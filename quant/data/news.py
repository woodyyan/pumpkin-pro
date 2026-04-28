from __future__ import annotations

import copy
import hashlib
import logging
import re
import signal
import threading
from datetime import datetime, timezone, timedelta
from typing import Any, Dict, Iterable, List, Optional, Sequence, Tuple

import akshare as ak
import pandas as pd

from data.fundamentals import (
    first_non_empty,
    get_symbol_fundamentals,
    normalize_date,
    normalize_float,
    normalize_symbol,
)

logger = logging.getLogger(__name__)

_MEDIA_FUNCTION_CANDIDATES = (
    "stock_news_em",
    "stock_news_main_cx",
)
_A_SHARE_OFFICIAL_FUNCTION_CANDIDATES = (
    "stock_notice_report",
    "stock_zh_a_notice_report",
    "stock_notice_board",
)
_HK_OFFICIAL_FUNCTION_CANDIDATES = (
    "stock_hk_notice",
    "stock_hk_notice_report",
)
_NEWS_CACHE: Dict[str, Dict[str, Any]] = {}
_CACHE_LOCK = threading.Lock()
_CST = timezone(timedelta(hours=8))
_NEWS_TRADING_CACHE_TTL_SECONDS = 10 * 60
_NEWS_OFF_TRADING_CACHE_TTL_SECONDS = 30 * 60
_NEWS_IDLE_NIGHT_CACHE_TTL_SECONDS = 2 * 60 * 60

_TITLE_COLUMNS = ["新闻标题", "标题", "名称", "title", "Title"]
_SUMMARY_COLUMNS = ["摘要", "内容摘要", "新闻内容", "内容", "summary", "content", "正文摘要"]
_PUBLISHED_COLUMNS = ["发布时间", "时间", "发布日期", "publish_time", "pub_time", "date", "display_time"]
_SOURCE_COLUMNS = ["文章来源", "来源", "媒体", "媒体名称", "source", "publisher"]
_URL_COLUMNS = ["新闻链接", "链接", "网址", "url", "article_url", "新闻网址"]
_REPORT_PERIOD_COLUMNS = ["报告期", "report_period", "报告年度"]
_REPORT_TYPE_COLUMNS = ["公告类型", "类型", "report_type", "公告类别"]


def current_news_cache_ttl_seconds(now: Optional[datetime] = None) -> int:
    current = now or datetime.utcnow()
    cst = current.replace(tzinfo=timezone.utc).astimezone(_CST)
    total_minutes = cst.hour * 60 + cst.minute
    if total_minutes < 7 * 60:
        return _NEWS_IDLE_NIGHT_CACHE_TTL_SECONDS
    if is_market_news_window(cst):
        return _NEWS_TRADING_CACHE_TTL_SECONDS
    return _NEWS_OFF_TRADING_CACHE_TTL_SECONDS


def is_market_news_window(now: datetime) -> bool:
    if now.weekday() >= 5:
        return False
    total_minutes = now.hour * 60 + now.minute
    return (9 * 60 + 15 <= total_minutes <= 12 * 60) or (13 * 60 <= total_minutes <= 16 * 60 + 10)


class SourceTimeoutError(TimeoutError):
    pass


class time_limit:
    def __init__(self, seconds: int):
        self.seconds = max(1, int(seconds or 1))
        self._previous_handler = None

    def _handle_timeout(self, signum, frame):  # pragma: no cover - signal path
        raise SourceTimeoutError(f"source timed out after {self.seconds}s")

    def __enter__(self):
        if threading.current_thread() is not threading.main_thread():
            return self
        self._previous_handler = signal.getsignal(signal.SIGALRM)
        signal.signal(signal.SIGALRM, self._handle_timeout)
        signal.alarm(self.seconds)
        return self

    def __exit__(self, exc_type, exc, tb):
        if threading.current_thread() is not threading.main_thread():
            return False
        signal.alarm(0)
        signal.signal(signal.SIGALRM, self._previous_handler)
        return False


def collect_source_items(fetcher, timeout_seconds: int) -> Tuple[List[Dict[str, Any]], List[str]]:
    try:
        with time_limit(timeout_seconds):
            return fetcher()
    except SourceTimeoutError as exc:
        logger.warning("新闻源调用超时: %s", exc)
        return [], [str(exc)]


def get_symbol_news(symbol: str) -> Dict[str, Any]:
    normalized, exchange, code = normalize_symbol(symbol)
    now = datetime.utcnow()
    cache_ttl_seconds = current_news_cache_ttl_seconds(now)

    with _CACHE_LOCK:
        cached = _NEWS_CACHE.get(normalized)
        if cached and (now - cached["fetched_at"]).total_seconds() < cache_ttl_seconds:
            return copy.deepcopy(cached["payload"])

    warnings: List[str] = []
    items: List[Dict[str, Any]] = []

    media_items, media_warnings = collect_source_items(
        lambda: fetch_media_news(normalized, exchange, code),
        timeout_seconds=6,
    )
    items.extend(media_items)
    warnings.extend(media_warnings)

    official_items, official_warnings = collect_source_items(
        lambda: fetch_official_items(normalized, exchange, code),
        timeout_seconds=6,
    )
    items.extend(official_items)
    warnings.extend(official_warnings)

    filing_items, filing_warnings = collect_source_items(
        lambda: build_filing_items_from_fundamentals(normalized),
        timeout_seconds=8,
    )
    items.extend(filing_items)
    warnings.extend(filing_warnings)

    ranked = rank_and_dedupe_items(items)
    summary = build_news_summary(ranked)
    if not ranked and warnings:
        summary["latest_headline"] = "新闻源暂时不可用，已回退为空结果"

    payload = {
        "symbol": normalized,
        "exchange": exchange,
        "updated_at": now.isoformat() + "Z",
        "summary": summary,
        "items": ranked[:60],
        "meta": {
            "warnings": warnings,
            "cache_ttl_seconds": cache_ttl_seconds,
            "sources": sorted({str(item.get("source_name") or "").strip() for item in ranked if str(item.get("source_name") or "").strip()}),
        },
    }

    with _CACHE_LOCK:
        _NEWS_CACHE[normalized] = {"fetched_at": now, "payload": copy.deepcopy(payload)}
    return copy.deepcopy(payload)


def fetch_media_news(symbol: str, exchange: str, code: str) -> Tuple[List[Dict[str, Any]], List[str]]:
    items: List[Dict[str, Any]] = []
    warnings: List[str] = []
    attempts = build_function_attempts(code, symbol)
    for func_name in _MEDIA_FUNCTION_CANDIDATES:
        df = call_akshare_dataframe(func_name, attempts)
        if df is None or df.empty:
            continue
        normalized_items = normalize_news_frame(
            df=df,
            symbol=symbol,
            default_type="news",
            default_source_type="media",
            default_source_name="财经媒体",
        )
        if normalized_items:
            items.extend(normalized_items)
            break
    if not items:
        warnings.append(f"{exchange} 媒体新闻源暂不可用")
    return items, warnings


def fetch_official_items(symbol: str, exchange: str, code: str) -> Tuple[List[Dict[str, Any]], List[str]]:
    items: List[Dict[str, Any]] = []
    warnings: List[str] = []
    candidates = _HK_OFFICIAL_FUNCTION_CANDIDATES if exchange == "HKEX" else _A_SHARE_OFFICIAL_FUNCTION_CANDIDATES
    attempts = build_function_attempts(code, symbol)
    for func_name in candidates:
        df = call_akshare_dataframe(func_name, attempts)
        if df is None or df.empty:
            continue
        normalized_items = normalize_news_frame(
            df=df,
            symbol=symbol,
            default_type="announcement",
            default_source_type="official",
            default_source_name="官方披露",
        )
        if normalized_items:
            items.extend(normalized_items)
            break
    if not items:
        warnings.append(f"{exchange} 官方公告源暂不可用")
    return items, warnings


def build_filing_items_from_fundamentals(symbol: str) -> Tuple[List[Dict[str, Any]], List[str]]:
    warnings: List[str] = []
    try:
        payload = get_symbol_fundamentals(symbol)
    except Exception as exc:  # pragma: no cover - upstream unstable
        logger.warning("构建财报新闻失败 symbol=%s error=%s", symbol, exc)
        return [], [f"财报摘要源暂不可用: {exc}"]

    meta = payload.get("meta") or {}
    items = payload.get("items") or {}
    report_date = first_non_empty(meta.get("ttm_report_date"), meta.get("fy_report_date"), meta.get("dividend_report_date"))
    if not report_date:
        return [], warnings

    report_period = build_report_period(report_date)
    report_type = build_report_type(report_date)
    title = f"{report_period or report_date} 财务披露更新"

    summary_parts: List[str] = []
    revenue = normalize_float(items.get("revenue_fy"))
    net_profit = normalize_float(items.get("net_profit_fy"))
    profit_growth = normalize_float(items.get("profit_growth_rate"))
    dividend_yield = normalize_float(items.get("dividend_yield"))

    if revenue is not None:
        summary_parts.append(f"营收 {format_large_amount(revenue)}")
    if net_profit is not None:
        summary_parts.append(f"净利润 {format_large_amount(net_profit)}")
    if profit_growth is not None:
        summary_parts.append(f"利润增速 {profit_growth:.2f}%")
    if dividend_yield is not None and dividend_yield >= 0:
        summary_parts.append(f"股息率 {dividend_yield * 100:.2f}%")

    if not summary_parts:
        summary_parts.append("最新财报与基础面数据已更新")

    source_name = "HKEX 披露易" if payload.get("exchange") == "HKEX" else "财务披露"
    item = build_news_item(
        symbol=symbol,
        item_type="filing",
        source_type="official",
        source_name=source_name,
        title=title,
        summary="；".join(summary_parts),
        published_at=report_date,
        url="",
        report_period=report_period,
        report_type=report_type,
    )
    item["importance_score"] = 98
    item["is_ai_relevant"] = True
    return [item], warnings


def call_akshare_dataframe(func_name: str, attempts: Sequence[Dict[str, str]]) -> Optional[pd.DataFrame]:
    func = getattr(ak, func_name, None)
    if not callable(func):
        return None
    for kwargs in attempts:
        try:
            result = func(**kwargs)
        except TypeError:
            continue
        except Exception as exc:  # pragma: no cover - upstream unstable
            logger.debug("新闻源函数调用失败 func=%s kwargs=%s error=%s", func_name, kwargs, exc)
            continue
        if isinstance(result, pd.DataFrame) and not result.empty:
            return result.copy()
    return None


def build_function_attempts(code: str, symbol: str) -> List[Dict[str, str]]:
    digits = re.sub(r"\D", "", code or symbol)
    return [
        {"symbol": code},
        {"symbol": symbol},
        {"stock": code},
        {"stock": symbol},
        {"code": code},
        {"code": digits},
        {"symbol": digits},
    ]


def normalize_news_frame(
    df: pd.DataFrame,
    symbol: str,
    default_type: str,
    default_source_type: str,
    default_source_name: str,
) -> List[Dict[str, Any]]:
    items: List[Dict[str, Any]] = []
    if df is None or df.empty:
        return items

    title_col = find_column(df, _TITLE_COLUMNS)
    if not title_col:
        return items
    summary_col = find_column(df, _SUMMARY_COLUMNS)
    published_col = find_column(df, _PUBLISHED_COLUMNS)
    source_col = find_column(df, _SOURCE_COLUMNS)
    url_col = find_column(df, _URL_COLUMNS)
    report_period_col = find_column(df, _REPORT_PERIOD_COLUMNS)
    report_type_col = find_column(df, _REPORT_TYPE_COLUMNS)

    limit = min(len(df.index), 40)
    for _, row in df.head(limit).iterrows():
        title = clean_text(row.get(title_col))
        if not title:
            continue
        summary = clean_text(row.get(summary_col)) if summary_col else ""
        published_at = parse_published_at(row.get(published_col)) if published_col else ""
        source_name = clean_text(row.get(source_col)) if source_col else default_source_name
        url = clean_text(row.get(url_col)) if url_col else ""
        report_period = clean_text(row.get(report_period_col)) if report_period_col else ""
        report_type = clean_text(row.get(report_type_col)) if report_type_col else ""

        item_type = infer_item_type(title, summary, default_type)
        if item_type == "filing" and not report_type:
            report_type = infer_report_type(title)
        if item_type == "filing" and not report_period:
            report_period = infer_report_period(title, published_at)

        item = build_news_item(
            symbol=symbol,
            item_type=item_type,
            source_type=default_source_type,
            source_name=source_name or default_source_name,
            title=title,
            summary=summary,
            published_at=published_at,
            url=url,
            report_period=report_period,
            report_type=report_type,
        )
        items.append(item)
    return items


def build_news_item(
    symbol: str,
    item_type: str,
    source_type: str,
    source_name: str,
    title: str,
    summary: str,
    published_at: str,
    url: str,
    report_period: str = "",
    report_type: str = "",
) -> Dict[str, Any]:
    title_text = clean_text(title)
    summary_text = truncate_text(clean_text(summary), 140)
    published_text = published_at or ""
    dedupe_key = build_dedupe_key(title_text, published_text, url)
    importance = compute_importance(item_type, source_type, title_text)
    return {
        "id": hashlib.md5(f"{symbol}|{item_type}|{dedupe_key}".encode("utf-8")).hexdigest(),
        "type": item_type,
        "source_type": source_type,
        "source_name": source_name,
        "title": title_text,
        "summary": summary_text,
        "published_at": published_text,
        "url": url,
        "report_period": report_period,
        "report_type": report_type,
        "importance_score": importance,
        "is_ai_relevant": importance >= 60,
        "dedupe_key": dedupe_key,
    }


def build_news_summary(items: Sequence[Dict[str, Any]]) -> Dict[str, Any]:
    now = datetime.utcnow()
    last_24h_count = 0
    announcement_count = 0
    filing_count = 0
    latest_headline = ""
    highlight_tags: List[str] = []

    for idx, item in enumerate(items):
        if idx == 0:
            latest_headline = str(item.get("title") or "").strip()
        item_type = str(item.get("type") or "").strip()
        if item_type == "announcement":
            announcement_count += 1
        if item_type == "filing":
            filing_count += 1
        published_at = parse_datetime(item.get("published_at"))
        if published_at is None or (now - published_at).total_seconds() <= 24 * 3600:
            last_24h_count += 1
        highlight_tags.extend(extract_highlight_tags(item))

    return {
        "last_24h_count": last_24h_count,
        "announcement_count": announcement_count,
        "filing_count": filing_count,
        "latest_headline": latest_headline,
        "highlight_tags": unique_preserve_order(highlight_tags)[:4],
    }


def rank_and_dedupe_items(items: Iterable[Dict[str, Any]]) -> List[Dict[str, Any]]:
    best_by_key: Dict[str, Dict[str, Any]] = {}
    for item in items:
        title = str(item.get("title") or "").strip()
        if not title:
            continue
        key = str(item.get("dedupe_key") or build_dedupe_key(title, str(item.get("published_at") or ""), str(item.get("url") or "")))
        existing = best_by_key.get(key)
        if existing is None or score_item(item) > score_item(existing):
            best_by_key[key] = item
    ranked = list(best_by_key.values())
    ranked.sort(key=lambda item: (score_item(item), parse_datetime(item.get("published_at")) or datetime.min), reverse=True)
    return ranked


def score_item(item: Dict[str, Any]) -> float:
    importance = normalize_float(item.get("importance_score")) or 0
    source_bonus = 20 if item.get("source_type") == "official" else 0
    return importance + source_bonus


def compute_importance(item_type: str, source_type: str, title: str) -> int:
    base = 55
    if item_type == "filing":
        base = 95
    elif item_type == "announcement":
        base = 82
    if source_type == "official":
        base += 8
    lowered = title.lower()
    if any(keyword in title for keyword in ["年报", "季报", "中报", "业绩预告", "业绩快报", "盈喜", "盈警"]):
        base += 8
    if any(keyword in title for keyword in ["回购", "增持", "减持", "停牌", "复牌", "并购", "分红"]):
        base += 6
    if "research" in lowered or "product" in lowered:
        base += 2
    return min(base, 100)


def infer_item_type(title: str, summary: str, default_type: str) -> str:
    haystack = f"{title} {summary}"
    if any(keyword in haystack for keyword in ["年报", "中报", "季报", "财报", "业绩预告", "业绩快报", "盈喜", "盈警"]):
        return "filing"
    if any(keyword in haystack for keyword in ["公告", "停牌", "复牌", "回购", "分红", "增持", "减持", "股东大会"]):
        return "announcement"
    return default_type


def infer_report_type(title: str) -> str:
    for keyword in ["年报", "中报", "季报", "财报", "业绩预告", "业绩快报", "盈喜", "盈警"]:
        if keyword in title:
            return keyword
    return "财务披露"


def infer_report_period(title: str, published_at: str) -> str:
    match = re.search(r"(20\d{2})\s*[年\-/]?\s*(Q?[1-4])", title, re.IGNORECASE)
    if match:
        year = match.group(1)
        quarter = match.group(2).upper().replace("Q", "Q")
        return f"{year}{quarter if quarter.startswith('Q') else 'Q' + quarter}"
    date_match = re.search(r"(20\d{2})[-/](\d{2})[-/](\d{2})", title)
    if date_match:
        return f"{date_match.group(1)}-{date_match.group(2)}-{date_match.group(3)}"
    return build_report_period(published_at)


def build_report_period(report_date: str) -> str:
    text = str(report_date or "").strip()
    if not text:
        return ""
    normalized = normalize_date(text) or text
    match = re.match(r"(20\d{2})-(\d{2})-(\d{2})", normalized)
    if not match:
        return normalized
    year = match.group(1)
    month = match.group(2)
    day = match.group(3)
    if month == "12" and day == "31":
        return f"FY {year}"
    quarter_map = {"03": "Q1", "06": "Q2", "09": "Q3", "12": "Q4"}
    if month in quarter_map:
        return f"{year}{quarter_map[month]}"
    return normalized


def build_report_type(report_date: str) -> str:
    text = normalize_date(report_date) or str(report_date or "")
    if text.endswith("-12-31"):
        return "年报"
    if text.endswith("-03-31"):
        return "一季报"
    if text.endswith("-06-30"):
        return "中报"
    if text.endswith("-09-30"):
        return "三季报"
    return "财务披露"


def extract_highlight_tags(item: Dict[str, Any]) -> List[str]:
    title = str(item.get("title") or "")
    tags: List[str] = []
    item_type = str(item.get("type") or "")
    if item_type == "filing":
        tags.append("财报")
    elif item_type == "announcement":
        tags.append("公告")
    for keyword, tag in [
        ("回购", "回购"),
        ("分红", "分红"),
        ("停牌", "停牌"),
        ("复牌", "复牌"),
        ("调研", "机构调研"),
        ("业绩", "业绩"),
        ("新品", "新品"),
    ]:
        if keyword in title:
            tags.append(tag)
    return tags


def find_column(df: pd.DataFrame, aliases: Sequence[str]) -> Optional[str]:
    column_map = {str(col).strip().lower(): col for col in df.columns}
    for alias in aliases:
        col = column_map.get(str(alias).strip().lower())
        if col is not None:
            return col
    for col in df.columns:
        lowered = str(col).strip().lower()
        for alias in aliases:
            alias_lower = str(alias).strip().lower()
            if alias_lower and alias_lower in lowered:
                return col
    return None


def clean_text(value: Any) -> str:
    text = str(value or "").strip()
    text = re.sub(r"\s+", " ", text)
    return text


def truncate_text(text: str, max_len: int) -> str:
    if len(text) <= max_len:
        return text
    return text[: max_len - 1].rstrip() + "…"


def parse_published_at(value: Any) -> str:
    if value is None:
        return ""
    dt = parse_datetime(value)
    if dt is None:
        normalized = normalize_date(value)
        return normalized or clean_text(value)
    return dt.strftime("%Y-%m-%dT%H:%M:%SZ")


def parse_datetime(value: Any) -> Optional[datetime]:
    if value is None or value == "":
        return None
    try:
        dt = pd.to_datetime(value, errors="coerce")
    except Exception:
        return None
    if dt is pd.NaT or pd.isna(dt):
        return None
    if hasattr(dt, "to_pydatetime"):
        dt = dt.to_pydatetime()
    if not isinstance(dt, datetime):
        return None
    return dt.replace(tzinfo=None)


def build_dedupe_key(title: str, published_at: str, url: str) -> str:
    base = re.sub(r"\s+", "", title)
    if published_at:
        base += "|" + published_at[:16]
    if url:
        base += "|" + url.split("?", 1)[0]
    return hashlib.md5(base.encode("utf-8")).hexdigest()


def format_large_amount(value: Optional[float]) -> str:
    if value is None:
        return "--"
    num = float(value)
    if abs(num) >= 1e8:
        return f"{num / 1e8:.2f}亿"
    if abs(num) >= 1e4:
        return f"{num / 1e4:.2f}万"
    return f"{num:.2f}"


def unique_preserve_order(items: Iterable[str]) -> List[str]:
    result: List[str] = []
    seen = set()
    for item in items:
        text = str(item or "").strip()
        if not text or text in seen:
            continue
        seen.add(text)
        result.append(text)
    return result
