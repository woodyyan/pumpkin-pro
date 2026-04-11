"""
A 股/港股全市场选股筛选器
- A 股主数据源：AKShare stock_zh_a_spot_em()（东方财富实时推送）
- A 股备用数据源：腾讯财经 qt.gtimg.cn（全天候可用）
- 港股主数据源：AKShare stock_hk_spot_em()（东方财富实时推送）
- 内存缓存 5 分钟
- 支持多维指标范围筛选 + 排序 + 分页
"""

import logging
import math
import threading
import time
from concurrent.futures import ThreadPoolExecutor, as_completed
from typing import Any, Dict, List, Optional, Tuple

import akshare as ak
import numpy as np
import pandas as pd
import requests

logger = logging.getLogger(__name__)

# ---------------------------------------------------------------------------
# 列名映射：AKShare 中文列名 → API 英文 key
# ---------------------------------------------------------------------------
COLUMN_MAP = {
    "代码": "code",
    "名称": "name",
    "最新价": "price",
    "涨跌幅": "change_pct",
    "涨跌额": "change_amt",
    "成交量": "volume",
    "成交额": "turnover",
    "振幅": "amplitude",
    "最高": "high",
    "最低": "low",
    "今开": "open",
    "昨收": "prev_close",
    "量比": "volume_ratio",
    "换手率": "turnover_rate",
    "市盈率-动态": "pe",
    "市净率": "pb",
    "总市值": "total_mv",
    "流通市值": "float_mv",
    "60日涨跌幅": "change_pct_60d",
    "年初至今涨跌幅": "change_pct_ytd",
}

FUNDAMENTAL_COLUMN_MAP = {
    "股票代码": "code",
    "所处行业": "industry",
    "净利润-同比增长": "profit_growth_rate",
}

# 可被用于范围筛选的数值列
FILTERABLE_COLUMNS = [
    "price", "change_pct", "total_mv", "pe", "pb",
    "turnover_rate", "volume_ratio", "amplitude",
    "turnover", "change_pct_60d", "change_pct_ytd", "float_mv",
    "volume", "change_amt", "profit_growth_rate",
]

# 需要转为 float 的列（排除 code / name）
NUMERIC_COLUMNS = [
    "price", "change_pct", "change_amt", "volume", "turnover",
    "amplitude", "high", "low", "open", "prev_close",
    "volume_ratio", "turnover_rate", "pe", "pb",
    "total_mv", "float_mv", "change_pct_60d", "change_pct_ytd",
    "profit_growth_rate",
]

# ---------------------------------------------------------------------------
# 内存缓存
# ---------------------------------------------------------------------------
_cache_lock = threading.Lock()
_cache_data: Optional[pd.DataFrame] = None
_cache_ts: float = 0.0
_CACHE_TTL = 900  # 15 分钟（选股场景不需要秒级实时性）

_fundamentals_cache_data: Optional[pd.DataFrame] = None
_fundamentals_cache_ts: float = 0.0
_fundamentals_cache_report_date: Optional[str] = None
_FUNDAMENTALS_CACHE_TTL = 86400  # 24 小时（业绩报表为季度级更新）
_MIN_COMPLETE_FUNDAMENTALS_ROWS = 4500


# ---------------------------------------------------------------------------
# 备用数据源：腾讯财经 qt.gtimg.cn
# ---------------------------------------------------------------------------
_QQ_BATCH_SIZE = 500  # 每批查询数量
_QQ_TIMEOUT = 15
_QQ_HEADERS = {
    "User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) "
                  "AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
    "Referer": "https://stockapp.finance.qq.com/",
}


def _code_to_qq(code: str) -> str:
    """将 6 位数字代码转为腾讯格式 shXXXXXX / szXXXXXX"""
    c = str(code).zfill(6)
    return f"sh{c}" if c.startswith(("6", "9")) else f"sz{c}"


def _safe_float(val: str) -> Optional[float]:
    """安全转 float，空串或非法值返回 None"""
    if not val or val == "":
        return None
    try:
        f = float(val)
        if math.isnan(f) or math.isinf(f):
            return None
        return f
    except (ValueError, TypeError):
        return None


def _parse_qq_line(line: str) -> Optional[Dict[str, Any]]:
    """
    解析腾讯财经单行行情数据。
    格式: v_shXXXXXX="1~名称~代码~最新价~昨收~今开~...~";
    字段索引参考:
      1:名称  2:代码  3:最新价  4:昨收  5:今开
      6:成交量(手)  7:外盘  8:内盘  9:买一  10:买一量
      ...
      29:涨跌额  30:涨跌幅(已含%)  31:最高  32:最低  33:涨跌幅
      34:成交额(万)  35:成交量(手)  36:换手率
      37:市盈率  38:振幅  ...  44:流通市值(亿)  45:总市值(亿)  46:市净率
    """
    if "~" not in line:
        return None
    parts = line.split("~")
    if len(parts) < 50:
        return None

    price = _safe_float(parts[3])
    prev_close = _safe_float(parts[4])

    # 计算涨跌额和涨跌幅
    change_amt = None
    change_pct = None
    if price is not None and prev_close is not None and prev_close != 0:
        change_amt = round(price - prev_close, 3)
        change_pct = _safe_float(parts[32])  # 腾讯已算好的涨跌幅

    # 成交额：腾讯返回的是万元，需要转为元
    turnover_raw = _safe_float(parts[37])
    turnover = turnover_raw * 1e4 if turnover_raw is not None else None

    # 总市值 / 流通市值：腾讯返回的是亿元，需要转为元
    total_mv_raw = _safe_float(parts[45])
    total_mv = total_mv_raw * 1e8 if total_mv_raw is not None else None
    float_mv_raw = _safe_float(parts[44])
    float_mv = float_mv_raw * 1e8 if float_mv_raw is not None else None

    return {
        "code": str(parts[2]).zfill(6),
        "name": parts[1],
        "price": price,
        "change_pct": change_pct,
        "change_amt": change_amt,
        "volume": _safe_float(parts[36]),  # 成交量(手)
        "turnover": turnover,
        "amplitude": _safe_float(parts[43]),
        "high": _safe_float(parts[33]),
        "low": _safe_float(parts[34]),
        "open": _safe_float(parts[5]),
        "prev_close": prev_close,
        "volume_ratio": None,  # 腾讯接口无量比字段
        "turnover_rate": _safe_float(parts[38]),
        "pe": _safe_float(parts[39]),
        "pb": _safe_float(parts[46]),
        "total_mv": total_mv,
        "float_mv": float_mv,
        "change_pct_60d": None,  # 腾讯接口无 60 日涨幅
        "change_pct_ytd": None,  # 腾讯接口无 YTD 涨幅
    }


def _fetch_qq_batch(qq_codes: List[str]) -> List[Dict[str, Any]]:
    """查询一批腾讯行情数据"""
    url = f"http://qt.gtimg.cn/q={','.join(qq_codes)}"
    resp = requests.get(url, headers=_QQ_HEADERS, timeout=_QQ_TIMEOUT)
    resp.encoding = "gbk"
    results = []
    for line in resp.text.strip().split("\n"):
        record = _parse_qq_line(line)
        if record and record.get("price") is not None:
            results.append(record)
    return results


def _get_snapshot_via_qq() -> pd.DataFrame:
    """通过腾讯财经接口获取全市场快照（备用方案）"""
    logger.info("尝试腾讯财经备用数据源...")

    # 第一步：获取全部 A 股代码列表
    try:
        info_df = ak.stock_info_a_code_name()
    except Exception as exc:
        logger.error("获取 A 股代码列表失败: %s", exc)
        raise RuntimeError("获取 A 股代码列表失败") from exc

    all_codes = info_df["code"].astype(str).str.zfill(6).tolist()
    qq_codes = [_code_to_qq(c) for c in all_codes]
    logger.info("准备从腾讯财经拉取 %d 只股票行情...", len(qq_codes))

    # 第二步：分批并发拉取行情
    batches = [
        qq_codes[i:i + _QQ_BATCH_SIZE]
        for i in range(0, len(qq_codes), _QQ_BATCH_SIZE)
    ]

    all_records: List[Dict[str, Any]] = []
    with ThreadPoolExecutor(max_workers=4) as executor:
        futures = {executor.submit(_fetch_qq_batch, batch): idx for idx, batch in enumerate(batches)}
        for future in as_completed(futures):
            try:
                records = future.result()
                all_records.extend(records)
            except Exception as exc:
                logger.warning("腾讯行情批次 %d 拉取失败: %s", futures[future], exc)

    if not all_records:
        raise RuntimeError("腾讯财经数据源返回空数据")

    df = pd.DataFrame(all_records)

    # 数值列转 float
    for col in NUMERIC_COLUMNS:
        if col in df.columns:
            df[col] = pd.to_numeric(df[col], errors="coerce")

    logger.info("腾讯财经备用源加载完成: %d 只股票", len(df))
    return df


def _build_report_date_candidates(limit: int = 8) -> List[str]:
    """生成最近若干个季报/年报日期候选列表，按时间从近到远排序。"""
    today = pd.Timestamp.today().normalize()
    candidates: List[str] = []

    for year in range(today.year, today.year - 3, -1):
        for month, day in ((12, 31), (9, 30), (6, 30), (3, 31)):
            report_date = pd.Timestamp(year=year, month=month, day=day)
            if report_date > today:
                continue
            candidates.append(report_date.strftime("%Y%m%d"))
            if len(candidates) >= limit:
                return candidates

    return candidates


def _prepare_fundamentals_df(raw_df: Optional[pd.DataFrame]) -> pd.DataFrame:
    if raw_df is None or raw_df.empty:
        return pd.DataFrame(columns=["code", "industry", "profit_growth_rate"])

    available_columns = [column for column in FUNDAMENTAL_COLUMN_MAP if column in raw_df.columns]
    if "股票代码" not in available_columns:
        return pd.DataFrame(columns=["code", "industry", "profit_growth_rate"])

    df = raw_df[available_columns].copy().rename(columns=FUNDAMENTAL_COLUMN_MAP)
    df["code"] = df["code"].astype(str).str.zfill(6)

    if "industry" not in df.columns:
        df["industry"] = None
    else:
        df["industry"] = df["industry"].apply(
            lambda value: None if pd.isna(value) or not str(value).strip() else str(value).strip()
        )

    if "profit_growth_rate" not in df.columns:
        df["profit_growth_rate"] = np.nan
    else:
        df["profit_growth_rate"] = pd.to_numeric(df["profit_growth_rate"], errors="coerce")

    return df[["code", "industry", "profit_growth_rate"]].drop_duplicates(subset=["code"], keep="first")


def _load_latest_fundamentals() -> Tuple[pd.DataFrame, Optional[str]]:
    fallback_df = pd.DataFrame(columns=["code", "industry", "profit_growth_rate"])
    fallback_report_date: Optional[str] = None
    fallback_rows = 0

    for report_date in _build_report_date_candidates():
        try:
            raw_df = ak.stock_yjbb_em(date=report_date)
        except Exception as exc:
            logger.warning("加载业绩报表失败 report_date=%s error=%s", report_date, exc)
            continue

        prepared_df = _prepare_fundamentals_df(raw_df)
        row_count = len(prepared_df)
        if row_count == 0:
            continue

        logger.info("业绩报表加载成功 report_date=%s rows=%d", report_date, row_count)
        if row_count >= _MIN_COMPLETE_FUNDAMENTALS_ROWS:
            return prepared_df, report_date

        if row_count > fallback_rows:
            fallback_df = prepared_df
            fallback_report_date = report_date
            fallback_rows = row_count

    if fallback_rows > 0:
        logger.info(
            "使用覆盖率最高的业绩报表作为财务补充字段 report_date=%s rows=%d",
            fallback_report_date,
            fallback_rows,
        )
        return fallback_df, fallback_report_date

    return fallback_df, None


def _get_latest_fundamentals() -> Tuple[pd.DataFrame, Optional[str]]:
    global _fundamentals_cache_data, _fundamentals_cache_ts, _fundamentals_cache_report_date

    with _cache_lock:
        if _fundamentals_cache_data is not None and (time.time() - _fundamentals_cache_ts) < _FUNDAMENTALS_CACHE_TTL:
            logger.debug(
                "fundamentals 缓存命中，缓存年龄 %.1fs，report_date=%s",
                time.time() - _fundamentals_cache_ts,
                _fundamentals_cache_report_date,
            )
            return _fundamentals_cache_data.copy(), _fundamentals_cache_report_date

    fundamentals_df, report_date = _load_latest_fundamentals()

    with _cache_lock:
        _fundamentals_cache_data = fundamentals_df.copy()
        _fundamentals_cache_ts = time.time()
        _fundamentals_cache_report_date = report_date

    return fundamentals_df, report_date


def get_industry_options(df: pd.DataFrame) -> List[str]:
    if df is None or df.empty or "industry" not in df.columns:
        return []

    return sorted(
        {
            str(value).strip()
            for value in df["industry"].dropna().tolist()
            if str(value).strip()
        }
    )


# ---------------------------------------------------------------------------
# 主入口：先试东财，失败则回退腾讯
# ---------------------------------------------------------------------------
def get_a_share_snapshot() -> pd.DataFrame:
    """获取 A 股全市场实时快照，5 分钟内存缓存"""
    global _cache_data, _cache_ts

    with _cache_lock:
        if _cache_data is not None and (time.time() - _cache_ts) < _CACHE_TTL:
            logger.debug("screener 缓存命中，缓存年龄 %.1fs", time.time() - _cache_ts)
            return _cache_data.copy()

    logger.info("screener 缓存未命中，正在拉取全市场快照...")
    start = time.time()

    df = None

    # ---- 主数据源：东方财富（AKShare） ----
    try:
        raw_df = ak.stock_zh_a_spot_em()
        if raw_df is not None and not raw_df.empty:
            available_columns = [col for col in COLUMN_MAP if col in raw_df.columns]
            df = raw_df[available_columns].copy()
            df = df.rename(columns=COLUMN_MAP)
            if "code" in df.columns:
                df["code"] = df["code"].astype(str).str.zfill(6)
            for col in NUMERIC_COLUMNS:
                if col in df.columns:
                    df[col] = pd.to_numeric(df[col], errors="coerce")
            logger.info("东财主数据源加载成功: %d 只股票", len(df))
    except Exception as exc:
        logger.warning("东财主数据源失败: %s，切换腾讯备用源", exc)

    # ---- 备用数据源：腾讯财经 ----
    if df is None or df.empty:
        try:
            df = _get_snapshot_via_qq()
        except Exception as exc:
            logger.error("腾讯备用数据源也失败: %s", exc)
            raise RuntimeError("获取 A 股行情数据失败，请稍后重试") from exc

    fundamentals_df, report_date = _get_latest_fundamentals()
    if not fundamentals_df.empty:
        df = df.merge(fundamentals_df, on="code", how="left")
        logger.info(
            "已合并财务字段 report_date=%s matched=%d/%d",
            report_date,
            int(df["industry"].notna().sum()),
            len(df),
        )
    else:
        df["industry"] = None
        df["profit_growth_rate"] = np.nan
        logger.warning("未获取到财务字段补充数据，行业与利润增长率将为空")

    df["profit_growth_rate"] = pd.to_numeric(df["profit_growth_rate"], errors="coerce")

    elapsed = time.time() - start
    logger.info("全市场快照加载完成: %d 只股票, 耗时 %.2fs", len(df), elapsed)

    with _cache_lock:
        _cache_data = df.copy()
        _cache_ts = time.time()

    return df


# ---------------------------------------------------------------------------
# 筛选
# ---------------------------------------------------------------------------
def apply_filters(
    df: pd.DataFrame,
    filters: Dict[str, Dict[str, Any]],
    industry: Optional[str] = None,
) -> pd.DataFrame:
    """
    先按行业精确匹配，再按 min/max 范围过滤 DataFrame。
    filters 示例:
        {"price": {"min": 10, "max": 100}, "pe": {"max": 30}}
    """
    if industry and str(industry).strip():
        if "industry" not in df.columns:
            return df.iloc[0:0]
        df = df[df["industry"] == str(industry).strip()]

    if not filters:
        return df

    for key, bounds in filters.items():
        if key not in df.columns:
            continue
        if not isinstance(bounds, dict):
            continue

        min_val = bounds.get("min")
        max_val = bounds.get("max")

        if min_val is not None:
            try:
                df = df[df[key] >= float(min_val)]
            except (ValueError, TypeError):
                pass

        if max_val is not None:
            try:
                df = df[df[key] <= float(max_val)]
            except (ValueError, TypeError):
                pass

    return df


# ---------------------------------------------------------------------------
# 排序 + 分页
# ---------------------------------------------------------------------------
SORTABLE_COLUMNS = set(COLUMN_MAP.values()) | {"industry", "profit_growth_rate"}


def sort_and_paginate(
    df: pd.DataFrame,
    sort_by: str = "code",
    sort_order: str = "asc",
    page: int = 1,
    page_size: int = 50,
) -> Tuple[pd.DataFrame, int]:
    """排序并分页，返回 (page_df, total)"""
    # 白名单校验排序列
    if sort_by not in SORTABLE_COLUMNS:
        sort_by = "code"

    ascending = sort_order != "desc"

    try:
        df = df.sort_values(by=sort_by, ascending=ascending, na_position="last")
    except KeyError:
        df = df.sort_values(by="code", ascending=True, na_position="last")

    total = len(df)

    # 分页
    start = (page - 1) * page_size
    end = start + page_size
    page_df = df.iloc[start:end]

    return page_df, total


# ---------------------------------------------------------------------------
# 港股列名映射
# ---------------------------------------------------------------------------
HK_COLUMN_MAP = {
    "代码": "code",
    "名称": "name",
    "最新价": "price",
    "涨跌幅": "change_pct",
    "涨跌额": "change_amt",
    "成交量": "volume",
    "成交额": "turnover",
    "振幅": "amplitude",
    "最高": "high",
    "最低": "low",
    "今开": "open",
    "昨收": "prev_close",
    "量比": "volume_ratio",
    "换手率": "turnover_rate",
    "市盈率-动态": "pe",
    "市净率": "pb",
    "总市值": "total_mv",
    "流通市值": "float_mv",
}

HK_NUMERIC_COLUMNS = [
    "price", "change_pct", "change_amt", "volume", "turnover",
    "amplitude", "high", "low", "open", "prev_close",
    "volume_ratio", "turnover_rate", "pe", "pb",
    "total_mv", "float_mv",
]

_hk_cache_data: Optional[pd.DataFrame] = None
_hk_cache_ts: float = 0.0
_HK_CACHE_TTL = 900  # 15 分钟


# ---------------------------------------------------------------------------
# 港股全市场快照
# ---------------------------------------------------------------------------
def get_hk_snapshot() -> pd.DataFrame:
    """获取港股全市场实时快照，15 分钟内存缓存（东财主源，腾讯兜底）"""
    global _hk_cache_data, _hk_cache_ts

    with _cache_lock:
        if _hk_cache_data is not None and (time.time() - _hk_cache_ts) < _HK_CACHE_TTL:
            logger.debug("港股 screener 缓存命中，缓存年龄 %.1fs", time.time() - _hk_cache_ts)
            return _hk_cache_data.copy()

    logger.info("港股 screener 缓存未命中，正在拉取全市场快照...")
    start = time.time()

    df = None

    # ---- 主数据源：东方财富港股 ----
    try:
        raw_df = ak.stock_hk_spot_em()
        if raw_df is not None and not raw_df.empty:
            available_columns = [col for col in HK_COLUMN_MAP if col in raw_df.columns]
            df = raw_df[available_columns].copy()
            df = df.rename(columns=HK_COLUMN_MAP)
            if "code" in df.columns:
                df["code"] = df["code"].astype(str).str.zfill(5)
            for col in HK_NUMERIC_COLUMNS:
                if col in df.columns:
                    df[col] = pd.to_numeric(df[col], errors="coerce")
            logger.info("东财港股主数据源加载成功: %d 只股票", len(df))
    except Exception as exc:
        logger.warning("东财港股主数据源失败: %s，尝试腾讯备用源...", exc)

    # ---- 备用数据源：腾讯财经港股 ----
    if df is None or df.empty:
        try:
            df = _get_hk_snapshot_via_qq()
        except Exception as exc:
            logger.error("腾讯港股备用源也失败: %s", exc)
            raise RuntimeError("获取港股行情数据失败，请稍后重试") from exc

    elapsed = time.time() - start
    logger.info("港股全市场快照加载完成: %d 只股票, 耗时 %.2fs", len(df), elapsed)

    with _cache_lock:
        _hk_cache_data = df.copy()
        _hk_cache_ts = time.time()

    return df


def _parse_qq_hk_line(line: str) -> Optional[Dict[str, Any]]:
    """
    解析腾讯财经港股单行行情数据。
    港股格式与 A 股类似但字段索引不同：
      1:名称  2:代码  3:最新价  4:昨收
      ...
      34:成交额  35:成交量  36:换手率(%)  37:PE  38:振幅
      39:总市值(亿)  ...  50:市盈率(TTM)
    """
    if "~" not in line:
        return None
    parts = line.split("~")
    if len(parts) < 51:
        return None

    price = _safe_float(parts[3])
    prev_close = _safe_float(parts[4])

    change_pct = None
    if price is not None and prev_close is not None and prev_close != 0:
        change_pct = round((price - prev_close) / prev_close * 100, 3)

    turnover_raw = _safe_float(parts[37])  # 成交额（万？）
    turnover = turnover_raw * 1e4 if turnover_raw is not None else None

    total_mv_raw = _safe_float(parts[39])   # 总市值（亿）
    total_mv = total_mv_raw * 1e8 if total_mv_raw is not None else None

    # 从代码中提取纯数字：hk00700 → 00700
    code_raw = str(parts[2] or "").replace("hk", "").replace("HK", "")
    hk_code = code_raw.zfill(5) if code_raw.isdigit() else code_raw

    pe_val = _safe_float(parts[50])

    return {
        "code": hk_code,
        "name": parts[1],
        "price": price,
        "change_pct": change_pct,
        "change_amt": round(price - prev_close, 3) if (price and prev_close) else None,
        "volume": _safe_float(parts[36]),
        "turnover": turnover,
        "amplitude": _safe_float(parts[38]),
        "high": None,
        "low": None,
        "open": None,
        "prev_close": prev_close,
        "volume_ratio": None,
        "turnover_rate": _safe_float(parts[36]),  # 换手率近似
        "pe": pe_val,
        "pb": None,
        "total_mv": total_mv,
        "float_mv": None,
    }


def _get_hk_snapshot_via_qq() -> pd.DataFrame:
    """通过腾讯财经接口获取港股全市场快照（备用方案）。

    不依赖 AKShare 获取代码列表，直接用 5 位数字代码段批量查询腾讯。
    腾讯返回空/无效的代码会被自动跳过。
    """
    logger.info("尝试腾讯财经港股备用数据源...")

    # 港股代码范围：00001-09999（约 2000+ 只活跃股票）
    # 分批查询，每批 500 个，腾讯自动跳过无效代码
    hk_codes = [str(i).zfill(5) for i in range(1, 10000)]
    qq_codes = [f"hk{c}" for c in hk_codes]
    logger.info("准备从腾讯财经拉取 %d 个潜在港股代码...", len(qq_codes))

    def _fetch_qq_batch_hk(qq_codes_batch: List[str]) -> List[Dict[str, Any]]:
        """查询一批腾讯港股行情数据"""
        url = f"http://qt.gtimg.cn/q={','.join(qq_codes_batch)}"
        resp = requests.get(url, headers=_QQ_HEADERS, timeout=_QQ_TIMEOUT)
        resp.encoding = "gbk"
        results = []
        for line in resp.text.strip().split("\n"):
            record = _parse_qq_hk_line(line)
            if record and record.get("price") is not None:
                results.append(record)
        return results

    batches = [
        qq_codes[i:i + _QQ_BATCH_SIZE]
        for i in range(0, len(qq_codes), _QQ_BATCH_SIZE)
    ]

    all_records: List[Dict[str, Any]] = []
    with ThreadPoolExecutor(max_workers=4) as executor:
        futures = {executor.submit(_fetch_qq_batch_hk, batch): idx for idx, batch in enumerate(batches)}
        done_count = 0
        total_batches = len(batches)
        for future in as_completed(futures):
            try:
                records = future.result()
                all_records.extend(records)
            except Exception as exc:
                logger.warning("港股腾讯行情批次 %d 拉取失败: %s", futures[future], exc)
            done_count += 1
            if done_count % 4 == 0:
                logger.info("港股腾讯行情进度: %d/%d 批次, 已获取 %d 只",
                            done_count, total_batches, len(all_records))

    if not all_records:
        raise RuntimeError("腾讯财经港股数据源返回空数据")

    df = pd.DataFrame(all_records)
    for col in HK_NUMERIC_COLUMNS:
        if col in df.columns:
            df[col] = pd.to_numeric(df[col], errors="coerce")
    df = df.drop_duplicates(subset=["code"], keep="first").reset_index(drop=True)

    logger.info("腾讯财经港股备用源加载完成: %d 只股票", len(df))
    return df


# ---------------------------------------------------------------------------
# DataFrame → JSON-safe list[dict]
# ---------------------------------------------------------------------------
def df_to_records(df: pd.DataFrame) -> List[Dict[str, Any]]:
    """将 DataFrame 转为 JSON 安全的 list[dict]"""
    if df is None or df.empty:
        return []

    safe_df = df.copy()
    safe_df = safe_df.replace([np.inf, -np.inf], np.nan)
    safe_df = safe_df.where(pd.notnull(safe_df), None)

    records = safe_df.to_dict("records")

    # 确保 NaN 变成 None
    for record in records:
        for key, value in record.items():
            if isinstance(value, float) and (value != value):  # NaN check
                record[key] = None

    return records
