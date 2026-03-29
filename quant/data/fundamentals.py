from __future__ import annotations

import copy
import logging
import threading
from datetime import datetime
from typing import Any, Dict, List, Optional, Tuple

import akshare as ak
import pandas as pd
import requests

logger = logging.getLogger(__name__)

_SYMBOL_CACHE: Dict[str, Dict[str, Any]] = {}
_REPORT_CACHE: Dict[str, pd.DataFrame] = {}
_CACHE_LOCK = threading.Lock()
_CACHE_DAY = ""

CODE_ALIASES = ["股票代码", "代码", "证券代码"]
REVENUE_ALIASES = ["营业总收入-营业总收入", "营业收入-营业收入", "营业总收入", "营业收入"]
NET_PROFIT_ALIASES = ["净利润-净利润", "归母净利润-净利润", "净利润", "归母净利润"]
GROSS_MARGIN_ALIASES = ["销售毛利率", "毛利率"]
ANNOUNCEMENT_ALIASES = ["最新公告日期", "公告日期", "业绩披露日期"]
QQ_HEADERS = {
    "User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 "
    "(KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
    "Referer": "https://stockapp.finance.qq.com/",
}


def get_symbol_fundamentals(symbol: str) -> Dict[str, Any]:
    normalized, exchange, code = normalize_symbol(symbol)
    cache_day = _cache_day()

    _reset_daily_cache_if_needed(cache_day)
    with _CACHE_LOCK:
        cache_entry = _SYMBOL_CACHE.get(normalized)
        if cache_entry and cache_entry.get("cache_day") == cache_day:
            return copy.deepcopy(cache_entry["payload"])

    if exchange == "HKEX":
        payload = build_hk_payload(normalized, code)
    else:
        payload = build_a_share_payload(normalized, exchange, code)

    with _CACHE_LOCK:
        _SYMBOL_CACHE[normalized] = {
            "cache_day": cache_day,
            "payload": copy.deepcopy(payload),
        }
    return copy.deepcopy(payload)



def normalize_symbol(symbol: str) -> Tuple[str, str, str]:
    raw = str(symbol or "").strip().upper()
    if not raw:
        raise ValueError("股票代码不能为空")

    if raw.endswith(".SH"):
        digits = raw[:-3]
        if not is_a_share_code(digits, expected_prefix="6"):
            raise ValueError("A 股代码格式无效")
        return f"{digits}.SH", "SSE", digits
    if raw.endswith(".SZ"):
        digits = raw[:-3]
        if not is_a_share_code(digits, expected_prefix=("0", "3")):
            raise ValueError("A 股代码格式无效")
        return f"{digits}.SZ", "SZSE", digits
    if raw.endswith(".HK"):
        digits = raw[:-3]
        if not digits.isdigit():
            raise ValueError("港股代码格式无效")
        return f"{digits.zfill(5)}.HK", "HKEX", digits.zfill(5)

    digits = "".join(ch for ch in raw if ch.isdigit())
    if len(digits) == 6:
        if digits.startswith("6"):
            return f"{digits}.SH", "SSE", digits
        if digits.startswith(("0", "3")):
            return f"{digits}.SZ", "SZSE", digits
    if len(digits) == 5:
        return f"{digits}.HK", "HKEX", digits
    raise ValueError("股票代码格式无效，A 股请传 600519.SH / 000001.SZ，港股请传 00700.HK")



def is_a_share_code(code: str, expected_prefix: Any) -> bool:
    if not str(code).isdigit() or len(str(code)) != 6:
        return False
    if isinstance(expected_prefix, tuple):
        return str(code).startswith(expected_prefix)
    return str(code).startswith(str(expected_prefix))



def build_hk_payload(symbol: str, code: str) -> Dict[str, Any]:
    warnings: List[str] = []
    now = datetime.utcnow().isoformat() + "Z"

    hk_metrics: Dict[str, Any] = {}
    try:
        hk_metrics = fetch_hk_core_metrics(code)
    except Exception as exc:  # pragma: no cover - source instability
        logger.warning("加载港股核心指标失败 symbol=%s error=%s", code, exc)
        warnings.append(f"港股核心指标暂不可用: {exc}")

    fallback_quote: Dict[str, Any] = {}
    if (
        not hk_metrics.get("name")
        or hk_metrics.get("market_cap") is None
        or hk_metrics.get("dividend_yield") is None
        or hk_metrics.get("pe_ttm") is None
    ):
        try:
            fallback_quote = fetch_qq_hk_fundamental_quote(code)
        except Exception as exc:  # pragma: no cover - source instability
            logger.warning("加载腾讯港股兜底失败 symbol=%s error=%s", code, exc)
            warnings.append(f"腾讯港股兜底暂不可用: {exc}")

    market_cap = hk_metrics.get("market_cap")
    if market_cap is None:
        market_cap = fallback_quote.get("market_cap")
    dividend_yield = hk_metrics.get("dividend_yield")
    if dividend_yield is None:
        dividend_yield = fallback_quote.get("dividend_yield")
    pe_ttm = hk_metrics.get("pe_ttm")
    if pe_ttm is None:
        pe_ttm = fallback_quote.get("pe_ttm")
    display_name = first_non_empty(hk_metrics.get("name"), fallback_quote.get("name"))

    items = {
        "market_cap": market_cap,
        "dividend_yield": dividend_yield,
        "pe_ttm": pe_ttm,
        "net_profit_fy": hk_metrics.get("net_profit_fy"),
        "revenue_fy": hk_metrics.get("revenue_fy"),
        "float_shares": hk_metrics.get("float_shares"),
        "gross_margin": None,
        "net_margin": hk_metrics.get("net_margin"),
    }

    if all(value is None for value in items.values()):
        raise RuntimeError("港股基础面暂不可用，请稍后重试")

    sources = []
    if hk_metrics:
        sources.append("eastmoney")
    if fallback_quote:
        sources.append("tencent")

    return {
        "symbol": symbol,
        "exchange": "HKEX",
        "name": display_name,
        "items": items,
        "meta": {
            "supported": True,
            "updated_at": now,
            "cache_day": _cache_day(),
            "source": "+".join(sources) if sources else "akshare",
            "fy_report_date": hk_metrics.get("fy_report_date"),
            "ttm_report_date": hk_metrics.get("ttm_report_date"),
            "warnings": warnings,
        },
    }



def build_a_share_payload(symbol: str, exchange: str, code: str) -> Dict[str, Any]:
    warnings: List[str] = []
    now = datetime.utcnow().isoformat() + "Z"

    info_map: Dict[str, Any] = {}
    try:
        info_map = fetch_individual_info(code)
    except Exception as exc:  # pragma: no cover - source instability
        logger.warning("加载个股信息失败 symbol=%s error=%s", code, exc)
        warnings.append(f"个股信息暂不可用: {exc}")

    financials = None
    try:
        financials = get_financial_metrics(code)
    except Exception as exc:  # pragma: no cover - source instability
        logger.warning("加载业绩报表失败 symbol=%s error=%s", code, exc)
        warnings.append(f"业绩报表暂不可用: {exc}")

    dividend = None
    try:
        dividend = get_dividend_metrics(code)
    except Exception as exc:  # pragma: no cover - source instability
        logger.warning("加载分红信息失败 symbol=%s error=%s", code, exc)
        warnings.append(f"分红信息暂不可用: {exc}")

    fallback_quote: Dict[str, Any] = {}
    market_cap = to_float(info_map.get("总市值"))
    float_shares = to_float(info_map.get("流通股"))
    display_name = first_non_empty(info_map.get("股票简称"), info_map.get("股票名称"), info_map.get("名称"))
    if market_cap is None or float_shares is None or not display_name:
        try:
            fallback_quote = fetch_qq_fundamental_quote(code)
        except Exception as exc:  # pragma: no cover - source instability
            logger.warning("加载腾讯行情兜底失败 symbol=%s error=%s", code, exc)
            warnings.append(f"腾讯行情兜底暂不可用: {exc}")

    market_cap = market_cap if market_cap is not None else fallback_quote.get("market_cap")
    float_shares = float_shares if float_shares is not None else fallback_quote.get("float_shares")
    display_name = display_name or fallback_quote.get("name")

    net_profit_ttm = financials.get("net_profit_ttm") if financials else None
    pe_ttm = None
    if market_cap and net_profit_ttm and net_profit_ttm > 0:
        pe_ttm = market_cap / net_profit_ttm

    items = {
        "market_cap": market_cap,
        "dividend_yield": dividend.get("dividend_yield") if dividend else None,
        "pe_ttm": normalize_float(pe_ttm),
        "net_profit_fy": financials.get("net_profit_fy") if financials else None,
        "revenue_fy": financials.get("revenue_fy") if financials else None,
        "float_shares": float_shares,
        "gross_margin": financials.get("gross_margin") if financials else None,
        "net_margin": financials.get("net_margin") if financials else None,
    }

    if all(value is None for value in items.values()):
        raise RuntimeError("A 股基础面数据暂不可用，请稍后重试")

    return {
        "symbol": symbol,
        "exchange": exchange,
        "name": display_name,
        "items": items,
        "meta": {
            "supported": True,
            "updated_at": now,
            "cache_day": _cache_day(),
            "source": "akshare",
            "fy_report_date": financials.get("fy_report_date") if financials else None,
            "ttm_report_date": financials.get("ttm_report_date") if financials else None,
            "dividend_report_date": dividend.get("report_date") if dividend else None,
            "warnings": warnings,
        },
    }



def fetch_individual_info(code: str) -> Dict[str, Any]:
    df = ak.stock_individual_info_em(symbol=code)
    if df is None or df.empty or not {"item", "value"}.issubset(df.columns):
        raise RuntimeError("东方财富个股信息返回为空")
    result: Dict[str, Any] = {}
    for _, row in df.iterrows():
        key = str(row.get("item") or "").strip()
        if not key:
            continue
        result[key] = row.get("value")
    return result



def fetch_qq_fundamental_quote(code: str) -> Dict[str, Any]:
    quote_code = f"sh{code}" if str(code).startswith("6") else f"sz{code}"
    url = f"http://qt.gtimg.cn/q={quote_code}"
    response = requests.get(url, headers=QQ_HEADERS, timeout=8)
    response.encoding = "gbk"
    body = str(response.text or "").strip()
    if "~" not in body:
        raise RuntimeError("腾讯行情返回异常")
    parts = body.split("~")
    if len(parts) < 46:
        raise RuntimeError("腾讯行情字段不足")

    price = normalize_float(parts[3])
    total_mv_raw = normalize_float(parts[45])
    float_mv_raw = normalize_float(parts[44])
    market_cap = total_mv_raw * 1e8 if total_mv_raw is not None else None
    float_market_cap = float_mv_raw * 1e8 if float_mv_raw is not None else None
    float_shares = None
    if float_market_cap is not None and price and price > 0:
        float_shares = float_market_cap / price

    return {
        "name": str(parts[1] or "").strip() or None,
        "market_cap": normalize_float(market_cap),
        "float_shares": normalize_float(float_shares),
    }



def fetch_hk_core_metrics(code: str) -> Dict[str, Any]:
    url = "https://datacenter.eastmoney.com/securities/api/data/v1/get"
    params = {
        "reportName": "RPT_CUSTOM_HKF10_FN_MAININDICATORMAX",
        "columns": "ORG_CODE,SECUCODE,SECURITY_CODE,SECURITY_NAME_ABBR,SECURITY_INNER_CODE,REPORT_DATE,BASIC_EPS,"
        "PER_NETCASH_OPERATE,BPS,BPS_NEDILUTED,COMMON_ACS,PER_SHARES,ISSUED_COMMON_SHARES,HK_COMMON_SHARES,"
        "TOTAL_MARKET_CAP,HKSK_MARKET_CAP,OPERATE_INCOME,OPERATE_INCOME_SQ,OPERATE_INCOME_QOQ,"
        "OPERATE_INCOME_QOQ_SQ,HOLDER_PROFIT,HOLDER_PROFIT_SQ,HOLDER_PROFIT_QOQ,HOLDER_PROFIT_QOQ_SQ,PE_TTM,"
        "PE_TTM_SQ,PB_TTM,PB_TTM_SQ,NET_PROFIT_RATIO,NET_PROFIT_RATIO_SQ,ROE_AVG,ROE_AVG_SQ,ROA,"
        "ROA_SQ,DIVIDEND_TTM,DIVIDEND_LFY,DIVI_RATIO,DIVIDEND_RATE,IS_CNY_CODE",
        "quoteColumns": "",
        "filter": f'(SECUCODE=\"{str(code).zfill(5)}.HK\")',
        "pageNumber": "1",
        "pageSize": "1",
        "sortTypes": "-1",
        "sortColumns": "REPORT_DATE",
        "source": "F10",
        "client": "PC",
        "v": "07945646099062258",
    }
    response = requests.get(url, params=params, timeout=10)
    data_json = response.json()
    rows = ((data_json or {}).get("result") or {}).get("data") or []
    if not rows:
        raise RuntimeError("东方财富港股指标返回为空")

    row = rows[0]
    report_date = normalize_date(row.get("REPORT_DATE"))
    return {
        "name": first_non_empty(row.get("SECURITY_NAME_ABBR")),
        "market_cap": normalize_float(row.get("TOTAL_MARKET_CAP")) or normalize_float(row.get("HKSK_MARKET_CAP")),
        "dividend_yield": normalize_dividend_yield(row.get("DIVIDEND_RATE")),
        "pe_ttm": normalize_float(row.get("PE_TTM")),
        "net_profit_fy": normalize_float(row.get("HOLDER_PROFIT")),
        "revenue_fy": normalize_float(row.get("OPERATE_INCOME")),
        "float_shares": normalize_float(row.get("HK_COMMON_SHARES")) or normalize_float(row.get("ISSUED_COMMON_SHARES")),
        "net_margin": normalize_float(row.get("NET_PROFIT_RATIO")),
        "fy_report_date": report_date,
        "ttm_report_date": report_date,
    }



def fetch_qq_hk_fundamental_quote(code: str) -> Dict[str, Any]:
    quote_code = f"hk{str(code).zfill(5)}"
    url = f"http://qt.gtimg.cn/q={quote_code}"
    response = requests.get(url, headers=QQ_HEADERS, timeout=8)
    response.encoding = "gbk"
    body = str(response.text or "").strip()
    if "=" in body:
        body = body.split("=", 1)[1]
    body = body.strip().strip(";").strip('"')
    if "~" not in body:
        raise RuntimeError("腾讯港股行情返回异常")
    parts = body.split("~")
    if len(parts) < 51:
        raise RuntimeError("腾讯港股行情字段不足")

    total_market_cap_raw = normalize_float(parts[39])
    market_cap = total_market_cap_raw * 1e8 if total_market_cap_raw is not None else None

    return {
        "name": str(parts[1] or "").strip() or None,
        "market_cap": normalize_float(market_cap),
        "dividend_yield": normalize_dividend_yield(parts[41]),
        "pe_ttm": normalize_float(parts[50]),
    }



def get_financial_metrics(code: str) -> Dict[str, Any]:
    rows: Dict[str, Dict[str, Any]] = {}
    for report_date in build_report_date_candidates(limit=8):
        row = get_symbol_report_row(code, report_date)
        if row:
            rows[report_date] = row

    if not rows:
        raise RuntimeError("未找到该股票的业绩报表")

    ordered_dates = sorted(rows.keys(), reverse=True)
    latest_date = ordered_dates[0]
    latest_row = rows[latest_date]
    fy_date = next((date for date in ordered_dates if date.endswith("1231")), None)
    fy_row = rows.get(fy_date) if fy_date else None

    revenue_ttm = calculate_ttm(rows, latest_date, "revenue")
    net_profit_ttm = calculate_ttm(rows, latest_date, "net_profit")

    # 毛利率：直接从报告期取
    fy_gross_margin = normalize_float(fy_row.get("gross_margin")) if fy_row else normalize_float(latest_row.get("gross_margin"))

    # 净利率：从净利润/收入计算
    fy_revenue = fy_row.get("revenue") if fy_row else latest_row.get("revenue")
    fy_net_profit = fy_row.get("net_profit") if fy_row else latest_row.get("net_profit")
    fy_net_margin = None
    if fy_revenue and fy_net_profit and fy_revenue > 0:
        fy_net_margin = round(fy_net_profit / fy_revenue * 100, 2)

    return {
        "ttm_report_date": format_report_date(latest_date),
        "fy_report_date": format_report_date(fy_date) if fy_date else format_report_date(latest_date),
        "revenue_fy": fy_row.get("revenue") if fy_row else latest_row.get("revenue"),
        "net_profit_fy": fy_row.get("net_profit") if fy_row else latest_row.get("net_profit"),
        "revenue_ttm": revenue_ttm,
        "net_profit_ttm": net_profit_ttm,
        "gross_margin": fy_gross_margin,
        "net_margin": fy_net_margin,
        "announcement_date": latest_row.get("announcement_date"),
    }



def get_symbol_report_row(code: str, report_date: str) -> Optional[Dict[str, Any]]:
    df = get_report_frame(report_date)
    if df.empty:
        return None
    matched = df[df["code"] == str(code).zfill(6)]
    if matched.empty:
        return None
    row = matched.iloc[0]
    return {
        "report_date": report_date,
        "revenue": normalize_float(row.get("revenue")),
        "net_profit": normalize_float(row.get("net_profit")),
        "gross_margin": normalize_float(row.get("gross_margin")),
        "announcement_date": normalize_date(row.get("announcement_date")),
    }



def get_report_frame(report_date: str) -> pd.DataFrame:
    _reset_daily_cache_if_needed(_cache_day())
    with _CACHE_LOCK:
        cached = _REPORT_CACHE.get(report_date)
        if cached is not None:
            return cached

    raw_df = ak.stock_yjbb_em(date=report_date)
    if raw_df is None or raw_df.empty:
        prepared = pd.DataFrame(columns=["code", "revenue", "net_profit", "gross_margin", "announcement_date"])
    else:
        code_column = find_column(raw_df, CODE_ALIASES)
        revenue_column = find_column(raw_df, REVENUE_ALIASES)
        profit_column = find_column(raw_df, NET_PROFIT_ALIASES)
        gross_margin_column = find_column(raw_df, GROSS_MARGIN_ALIASES)
        announcement_column = find_column(raw_df, ANNOUNCEMENT_ALIASES)
        if not code_column:
            prepared = pd.DataFrame(columns=["code", "revenue", "net_profit", "gross_margin", "announcement_date"])
        else:
            frame = pd.DataFrame()
            frame["code"] = raw_df[code_column].astype(str).str.zfill(6)
            frame["revenue"] = pd.to_numeric(raw_df[revenue_column], errors="coerce") if revenue_column else pd.NA
            frame["net_profit"] = pd.to_numeric(raw_df[profit_column], errors="coerce") if profit_column else pd.NA
            frame["gross_margin"] = pd.to_numeric(raw_df[gross_margin_column], errors="coerce") if gross_margin_column else pd.NA
            frame["announcement_date"] = pd.to_datetime(raw_df[announcement_column], errors="coerce") if announcement_column else pd.NaT
            prepared = frame[["code", "revenue", "net_profit", "gross_margin", "announcement_date"]].drop_duplicates(subset=["code"], keep="first")

    with _CACHE_LOCK:
        _REPORT_CACHE[report_date] = prepared
    return prepared



def get_dividend_metrics(code: str) -> Dict[str, Any]:
    df = ak.stock_fhps_detail_em(symbol=code)
    if df is None or df.empty or "现金分红-股息率" not in df.columns:
        raise RuntimeError("未找到分红数据")

    working = df.copy()
    if "报告期" in working.columns:
        working["报告期"] = pd.to_datetime(working["报告期"], errors="coerce")
    if "最新公告日期" in working.columns:
        working["最新公告日期"] = pd.to_datetime(working["最新公告日期"], errors="coerce")
    working["现金分红-股息率"] = pd.to_numeric(working["现金分红-股息率"], errors="coerce")
    working = working.dropna(subset=["现金分红-股息率"])
    if working.empty:
        raise RuntimeError("未找到有效股息率")

    sort_columns = [column for column in ["报告期", "最新公告日期"] if column in working.columns]
    if sort_columns:
        working = working.sort_values(sort_columns)
    latest = working.iloc[-1]
    dividend_yield = normalize_dividend_yield(latest.get("现金分红-股息率"))
    return {
        "dividend_yield": dividend_yield,
        "report_date": normalize_date(latest.get("报告期")) or normalize_date(latest.get("最新公告日期")),
    }



def calculate_ttm(rows: Dict[str, Dict[str, Any]], latest_date: str, field: str) -> Optional[float]:
    latest_row = rows.get(latest_date)
    if not latest_row:
        return None
    latest_value = normalize_float(latest_row.get(field))
    if latest_value is None:
        return None
    if latest_date.endswith("1231"):
        return latest_value

    year = int(latest_date[:4])
    month_day = latest_date[4:]
    prev_annual_date = f"{year - 1}1231"
    prev_same_date = f"{year - 1}{month_day}"
    prev_annual = normalize_float(rows.get(prev_annual_date, {}).get(field))
    prev_same = normalize_float(rows.get(prev_same_date, {}).get(field))
    if prev_annual is None or prev_same is None:
        return None
    return normalize_float(latest_value + prev_annual - prev_same)



def build_report_date_candidates(limit: int = 8) -> List[str]:
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



def find_column(df: pd.DataFrame, aliases: List[str]) -> Optional[str]:
    for alias in aliases:
        if alias in df.columns:
            return alias
    return None



def to_float(value: Any) -> Optional[float]:
    return normalize_float(value)



def normalize_float(value: Any) -> Optional[float]:
    if value is None or value is pd.NA:
        return None
    try:
        parsed = float(value)
    except (TypeError, ValueError):
        return None
    if pd.isna(parsed):
        return None
    return parsed



def normalize_dividend_yield(value: Any) -> Optional[float]:
    parsed = normalize_float(value)
    if parsed is None:
        return None
    if parsed > 1:
        return parsed / 100
    return parsed



def normalize_date(value: Any) -> Optional[str]:
    if value is None or value is pd.NaT:
        return None
    try:
        ts = pd.to_datetime(value, errors="coerce")
    except Exception:
        return None
    if ts is pd.NaT or pd.isna(ts):
        return None
    return ts.strftime("%Y-%m-%d")



def format_report_date(report_date: Optional[str]) -> Optional[str]:
    if not report_date:
        return None
    text = str(report_date)
    if len(text) != 8 or not text.isdigit():
        return None
    return f"{text[:4]}-{text[4:6]}-{text[6:]}"



def first_non_empty(*values: Any) -> Optional[str]:
    for value in values:
        text = str(value or "").strip()
        if text:
            return text
    return None



def _cache_day() -> str:
    return datetime.now().strftime("%Y-%m-%d")



def _reset_daily_cache_if_needed(cache_day: str) -> None:
    global _CACHE_DAY
    with _CACHE_LOCK:
        if _CACHE_DAY == cache_day:
            return
        _CACHE_DAY = cache_day
        _SYMBOL_CACHE.clear()
        _REPORT_CACHE.clear()
