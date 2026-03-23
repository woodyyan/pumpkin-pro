import logging
import os
import hashlib
import json
import queue
import threading
import time
from datetime import datetime
from functools import lru_cache
from typing import Dict, List, Optional, Tuple

import akshare as ak
import pandas as pd

from data.data_loader import DataLoader

logger = logging.getLogger(__name__)

# ---------------------------------------------------------------------------
# 请求节流 & 重试配置
# ---------------------------------------------------------------------------
_REQUEST_INTERVAL = 1.5  # 两次 AKShare 请求之间的最小间隔（秒）
_MAX_RETRIES = 2         # 每个数据源的最大重试次数
_RETRY_BASE_DELAY = 2.0  # 重试基础等待秒数（指数退避: delay * 2^attempt）
_last_request_ts: float = 0.0
_request_lock = threading.Lock()

# ---------------------------------------------------------------------------
# 本地缓存配置
# ---------------------------------------------------------------------------
_CACHE_DIR = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "..", ".cache", "stock_data")
_CACHE_TTL_SECONDS = 4 * 3600  # 缓存有效期 4 小时


def _throttle():
    """确保两次网络请求之间有足够间隔，避免触发东方财富反爬机制"""
    global _last_request_ts
    with _request_lock:
        elapsed = time.time() - _last_request_ts
        if elapsed < _REQUEST_INTERVAL:
            time.sleep(_REQUEST_INTERVAL - elapsed)
        _last_request_ts = time.time()


def _call_with_retry(fn, source_label: str, retries: int = _MAX_RETRIES):
    """
    带指数退避的重试包装器。
    成功返回 DataFrame；全部失败抛出最后一个异常。
    """
    last_exc = None
    for attempt in range(retries + 1):
        try:
            _throttle()
            result = fn()
            return result
        except Exception as exc:
            last_exc = exc
            if attempt < retries:
                delay = _RETRY_BASE_DELAY * (2 ** attempt)
                logger.warning(
                    "%s 第 %d 次请求失败，%.1f 秒后重试: %s",
                    source_label, attempt + 1, delay, exc,
                )
                time.sleep(delay)
    raise last_exc


# ---------------------------------------------------------------------------
# 本地磁盘缓存
# ---------------------------------------------------------------------------
def _cache_key(ticker: str, start_str: str, end_str: str, market: str) -> str:
    raw = f"{market}:{ticker}:{start_str}:{end_str}"
    return hashlib.md5(raw.encode()).hexdigest()


def _read_cache(ticker: str, start_str: str, end_str: str, market: str) -> Optional[Tuple[pd.DataFrame, str]]:
    """尝试从本地缓存读取数据。命中返回 (df, source_name)，否则 None。"""
    try:
        key = _cache_key(ticker, start_str, end_str, market)
        meta_path = os.path.join(_CACHE_DIR, f"{key}.json")
        data_path = os.path.join(_CACHE_DIR, f"{key}.csv")
        if not os.path.exists(meta_path) or not os.path.exists(data_path):
            return None
        with open(meta_path, "r", encoding="utf-8") as f:
            meta = json.load(f)
        if time.time() - meta.get("ts", 0) > _CACHE_TTL_SECONDS:
            return None
        df = pd.read_csv(data_path)
        if df is not None and not df.empty:
            logger.info("缓存命中: ticker=%s market=%s source=%s", ticker, market, meta.get("source"))
            return df, meta.get("source", "缓存") + " (缓存)"
    except Exception as exc:
        logger.debug("缓存读取跳过: %s", exc)
    return None


def _write_cache(ticker: str, start_str: str, end_str: str, market: str,
                 df: pd.DataFrame, source_name: str):
    """将成功获取的数据写入本地缓存"""
    try:
        os.makedirs(_CACHE_DIR, exist_ok=True)
        key = _cache_key(ticker, start_str, end_str, market)
        df.to_csv(os.path.join(_CACHE_DIR, f"{key}.csv"), index=False)
        with open(os.path.join(_CACHE_DIR, f"{key}.json"), "w", encoding="utf-8") as f:
            json.dump({"ts": time.time(), "source": source_name, "ticker": ticker, "market": market}, f)
    except Exception as exc:
        logger.debug("缓存写入跳过: %s", exc)


def _put_success(result_queue, dataframe: pd.DataFrame, source_name: str):
    result_queue.put(("success", dataframe, source_name))


def fetch_data_worker(ticker, start_str, end_str, market, result_queue, start_date, end_date):
    """在独立线程中执行下载，包含多数据源轮询容灾 + 重试 + 缓存机制"""
    # ---- 优先查缓存 ----
    cached = _read_cache(ticker, start_str, end_str, market)
    if cached:
        _put_success(result_queue, cached[0], cached[1])
        return

    errors = []

    if market == "a_share":
        try:
            df = _call_with_retry(
                lambda: ak.stock_zh_a_hist(symbol=ticker, period="daily", start_date=start_str, end_date=end_str, adjust="qfq"),
                "东财-A股",
            )
            if df is not None and not df.empty:
                _write_cache(ticker, start_str, end_str, market, df, "东方财富 (EastMoney)")
                _put_success(result_queue, df, "东方财富 (EastMoney)")
                return
        except Exception as exc:
            errors.append(f"东财接口失败: {exc}")

        try:
            prefix = "sh" if ticker.startswith(("6", "5")) else "sz"
            symbol_sina = f"{prefix}{ticker}"
            df = _call_with_retry(
                lambda: ak.stock_zh_a_daily(symbol=symbol_sina, start_date=start_str, end_date=end_str, adjust="qfq"),
                "新浪-A股",
            )
            if df is not None and not df.empty:
                _write_cache(ticker, start_str, end_str, market, df, "新浪财经 (Sina Finance)")
                _put_success(result_queue, df, "新浪财经 (Sina Finance)")
                return
        except Exception as exc:
            errors.append(f"新浪接口失败: {exc}")

        try:
            df = _call_with_retry(
                lambda: ak.stock_zh_a_hist(symbol=ticker, period="daily", start_date=start_str, end_date=end_str, adjust=""),
                "东财-A股-不复权",
            )
            if df is not None and not df.empty:
                _write_cache(ticker, start_str, end_str, market, df, "备用通道")
                _put_success(result_queue, df, "备用通道")
                return
        except Exception as exc:
            errors.append(f"备用接口失败: {exc}")

    elif market == "hk":
        # ---- 港股第 1 层：东方财富 前复权 (带重试) ----
        try:
            df = _call_with_retry(
                lambda: ak.stock_hk_hist(symbol=ticker, period="daily", start_date=start_str, end_date=end_str, adjust="qfq"),
                "东财-港股",
            )
            if df is not None and not df.empty:
                _write_cache(ticker, start_str, end_str, market, df, "东方财富-港股 (EastMoney HK)")
                _put_success(result_queue, df, "东方财富-港股 (EastMoney HK)")
                return
        except Exception as exc:
            errors.append(f"港股主接口失败: {exc}")

        # ---- 港股第 2 层：新浪港股日线 (带重试) ----
        try:
            df = _call_with_retry(
                lambda: ak.stock_hk_daily(symbol=ticker),
                "新浪-港股",
            )
            if df is not None and not df.empty:
                df = df.reset_index()
                date_column = df.columns[0]
                df[date_column] = pd.to_datetime(df[date_column], errors="coerce")
                df = df[(df[date_column] >= pd.to_datetime(start_date)) & (df[date_column] <= pd.to_datetime(end_date))]
                if not df.empty:
                    _write_cache(ticker, start_str, end_str, market, df, "新浪财经-港股 (Sina HK)")
                    _put_success(result_queue, df, "新浪财经-港股 (Sina HK)")
                    return
        except Exception as exc:
            errors.append(f"港股备用接口失败: {exc}")

        # ---- 港股第 3 层：东方财富 不复权兜底 (带重试) ----
        try:
            df = _call_with_retry(
                lambda: ak.stock_hk_hist(symbol=ticker, period="daily", start_date=start_str, end_date=end_str, adjust=""),
                "东财-港股-不复权",
            )
            if df is not None and not df.empty:
                _write_cache(ticker, start_str, end_str, market, df, "东方财富-港股-不复权 (EastMoney HK Raw)")
                _put_success(result_queue, df, "东方财富-港股-不复权 (EastMoney HK Raw)")
                return
        except Exception as exc:
            errors.append(f"港股不复权兜底失败: {exc}")

    result_queue.put(("error", " | ".join(errors), None))


def _detect_market(ticker: str) -> str:
    if len(ticker) == 5 and ticker.isdigit():
        return "hk"
    if len(ticker) == 6 and ticker.isdigit():
        return "a_share"
    raise ValueError(f"无法识别的股票代码格式: {ticker}。A股请用6位数字，港股请用5位数字。")


def resolve_stock_name(ticker: str) -> Optional[str]:
    stock_name, _ = resolve_stock_name_with_debug(ticker)
    return stock_name


def resolve_stock_name_with_debug(ticker: str) -> Tuple[Optional[str], Dict]:
    market = _detect_market(ticker)
    errors: List[str] = []

    if market == "a_share":
        try:
            info_df = ak.stock_individual_info_em(symbol=ticker)
            if info_df is not None and not info_df.empty and {"item", "value"}.issubset(info_df.columns):
                matched = info_df[info_df["item"].astype(str).isin(["股票简称", "股票名称", "名称"])]
                if not matched.empty:
                    name = str(matched.iloc[0]["value"]).strip()
                    if name:
                        return name, {
                            "status": "success",
                            "market": market,
                            "source": "东方财富-个股信息",
                            "message": f"已通过东方财富-个股信息识别股票名称：{name}",
                            "errors": [],
                        }
            errors.append("东方财富-个股信息未返回可用名称")
        except Exception as exc:
            logger.warning("A股名称识别失败 ticker=%s source=%s error=%s", ticker, "东方财富-个股信息", exc)
            errors.append(f"东方财富-个股信息异常: {exc}")

        try:
            a_share_spot_df = _get_a_share_spot_snapshot()
            if a_share_spot_df is not None and not a_share_spot_df.empty and {"代码", "名称"}.issubset(a_share_spot_df.columns):
                matched = a_share_spot_df[a_share_spot_df["代码"].astype(str).str.zfill(6) == ticker]
                if not matched.empty:
                    name = str(matched.iloc[0]["名称"]).strip()
                    if name:
                        return name, {
                            "status": "success",
                            "market": market,
                            "source": "东方财富-A股实时行情",
                            "message": f"已通过东方财富-A股实时行情识别股票名称：{name}",
                            "errors": errors,
                        }
            errors.append("东方财富-A股实时行情未匹配到股票名称")
        except Exception as exc:
            logger.warning("A股名称识别失败 ticker=%s source=%s error=%s", ticker, "东方财富-A股实时行情", exc)
            errors.append(f"东方财富-A股实时行情异常: {exc}")
    else:
        try:
            hk_spot_df = _get_hk_spot_snapshot()
            if hk_spot_df is not None and not hk_spot_df.empty and {"代码", "名称"}.issubset(hk_spot_df.columns):
                matched = hk_spot_df[hk_spot_df["代码"].astype(str).str.zfill(5) == ticker]
                if not matched.empty:
                    name = str(matched.iloc[0]["名称"]).strip()
                    if name:
                        return name, {
                            "status": "success",
                            "market": market,
                            "source": "东方财富-港股实时行情",
                            "message": f"已通过东方财富-港股实时行情识别股票名称：{name}",
                            "errors": [],
                        }
            errors.append("东方财富-港股实时行情未匹配到股票名称")
        except Exception as exc:
            logger.warning("港股名称识别失败 ticker=%s source=%s error=%s", ticker, "东方财富-港股实时行情", exc)
            errors.append(f"东方财富-港股实时行情异常: {exc}")

    message = "；".join(errors) if errors else "未匹配到股票名称"
    logger.warning("股票名称未识别 ticker=%s market=%s detail=%s", ticker, market, message)
    return None, {
        "status": "failed",
        "market": market,
        "source": None,
        "message": message,
        "errors": errors,
    }


@lru_cache(maxsize=1)
def _get_a_share_spot_snapshot() -> pd.DataFrame:
    return ak.stock_zh_a_spot_em()


@lru_cache(maxsize=1)
def _get_hk_spot_snapshot() -> pd.DataFrame:
    return ak.stock_hk_spot_em()


def fetch_stock_data(ticker: str, start_date: datetime, end_date: datetime) -> Tuple[pd.DataFrame, str]:
    """
    Fetch stock data from Akshare using multiple data sources as fallbacks.
    Returns (DataFrame, source_name)
    """
    start_str = start_date.strftime("%Y%m%d")
    end_str = end_date.strftime("%Y%m%d")
    market = _detect_market(ticker)

    result_queue = queue.Queue()
    thread = threading.Thread(
        target=fetch_data_worker,
        args=(ticker, start_str, end_str, market, result_queue, start_date, end_date),
        daemon=True,
    )
    thread.start()
    # 加入重试机制后，最坏情况需要更长时间：3 个数据源 × (3 次尝试 × ~6s) ≈ 54s
    thread.join(timeout=90.0)

    if thread.is_alive():
        raise TimeoutError("请求超时(>90秒)。所有数据源连接均已放弃，请检查网络后重试。")

    if result_queue.empty():
        raise RuntimeError("获取数据时发生未知异常，未返回结果。")

    status, result, source_used = result_queue.get()
    if status == "error":
        raise RuntimeError(f"所有数据源均连接失败。详细排查: {result}")

    if result is None or result.empty:
        raise ValueError(f"未能在任何数据源中找到 {ticker} 的交易记录。")

    loader = DataLoader()
    prepared = loader.prepare_dataframe(result)
    return prepared, source_used
