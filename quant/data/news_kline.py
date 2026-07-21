from __future__ import annotations

import copy
import hashlib
import json
import logging
import math
import re
import threading
import time
import urllib.parse
import urllib.request
from dataclasses import dataclass
from datetime import datetime, timedelta, timezone
from typing import Any, Callable, Dict, Iterable, List, Optional, Sequence, Tuple

logger = logging.getLogger(__name__)

NEWS_KLINE_CACHE_TTL_SECONDS = 30 * 60
NEWS_KLINE_HTTP_TIMEOUT_SECONDS = 5
NEWS_KLINE_HTTP_RETRIES = 2
NEWS_KLINE_INFO_PAGE_SIZE = 50
NEWS_KLINE_MAX_DAYS = 1000
NEWS_KLINE_MIN_DAYS = 60
NEWS_KLINE_MAX_PAGES = 5
NEWS_KLINE_MIN_PAGES = 1

_CST = timezone(timedelta(hours=8))
_UA = {
    "User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36",
    "Referer": "https://gu.qq.com/",
}
_REPORT_CACHE: Dict[str, Dict[str, Any]] = {}
_CACHE_LOCK = threading.Lock()

CATEGORY_META: Dict[str, Dict[str, str]] = {
    "财报业绩": {"color": "#8b5cf6", "label": "财报业绩"},
    "股权资本": {"color": "#f59e0b", "label": "股权资本"},
    "重大事项": {"color": "#3b82f6", "label": "重大事项"},
    "管理层治理": {"color": "#14b8a6", "label": "管理层治理"},
    "风险监管": {"color": "#ef4444", "label": "风险监管"},
    "行业市场": {"color": "#22c55e", "label": "行业/市场"},
    "研报评级": {"color": "#ec4899", "label": "研报评级"},
    "其他": {"color": "#9ca3af", "label": "其他"},
}

KEYWORDS: Sequence[Tuple[str, Sequence[str]]] = (
    ("风险监管", ("风险", "警示", "处罚", "立案", "调查", "ST", "退市", "监管", "问询", "关注函", "责令改正", "违法违规", "信息披露", "虚假陈述", "内幕交易", "操纵市场", "留置", "纪律审查", "监察", "违纪", "违法", "违规")),
    ("财报业绩", ("年报", "季报", "中报", "一季报", "三季报", "业绩", "净利", "营收", "利润", "分红", "派息", "送转", "权益分派", "预增", "预减", "预盈", "预亏", "扭亏", "每股收益", "EPS", "业绩快报", "业绩预告", "年度报告", "半年度报告", "季度报告", "主要经营数据", "审计报告", "审计", "会计")),
    ("股权资本", ("回购", "增持", "减持", "解禁", "定增", "非公开发行", "股权激励", "员工持股", "转债", "配股", "发行", "承销", "重组", "并购", "收购", "股权转让", "拍卖", "质押", "解押", "冻结", "司法", "要约", "停牌", "复牌", "限售")),
    ("重大事项", ("战略合作", "中标", "合同", "订单", "投资", "扩产", "项目", "建设", "投产", "专利", "诉讼", "仲裁", "赔偿", "签约", "竞得", "竞拍", "关联交易", "关联", "商标许可", "许可协议", "出资", "成立", "设立", "合资", "合作")),
    ("管理层治理", ("高管", "董事", "监事", "董事会", "股东会", "股东大会", "聘任", "辞职", "换届", "选举", "薪酬", "考核", "治理", "章程", "职工董事", "独董", "会计师事务所", "审计委员会", "内部控制", "内控")),
    ("研报评级", ("研报", "评级", "目标价", "买入", "增持", "中性", "减持", "卖出", "强烈推荐")),
)


@dataclass(frozen=True)
class NormalizedSymbol:
    input: str
    symbol: str
    quote_code: str
    exchange: str
    code: str


def clamp_int(value: Any, default: int, minimum: int, maximum: int) -> int:
    try:
        parsed = int(value)
    except Exception:
        return default
    return min(max(parsed, minimum), maximum)


def normalize_symbol(raw: str) -> NormalizedSymbol:
    value = str(raw or "").strip()
    if not value:
        raise ValueError("股票代码不能为空")
    lowered = value.lower().replace(" ", "")
    upper = value.upper().replace(" ", "")

    if upper.endswith(".HK"):
        digits = re.sub(r"\D", "", upper[:-3]).zfill(5)[-5:]
        return NormalizedSymbol(value, f"{digits}.HK", f"hk{digits}", "HKEX", digits)
    if upper.endswith(".SH"):
        digits = re.sub(r"\D", "", upper[:-3]).zfill(6)[-6:]
        return NormalizedSymbol(value, f"{digits}.SH", f"sh{digits}", "SSE", digits)
    if upper.endswith(".SZ"):
        digits = re.sub(r"\D", "", upper[:-3]).zfill(6)[-6:]
        return NormalizedSymbol(value, f"{digits}.SZ", f"sz{digits}", "SZSE", digits)
    if upper.endswith(".BJ"):
        digits = re.sub(r"\D", "", upper[:-3]).zfill(6)[-6:]
        return NormalizedSymbol(value, f"{digits}.BJ", f"bj{digits}", "BJSE", digits)

    if lowered.startswith(("sh", "sz", "bj")) and len(re.sub(r"\D", "", lowered[2:])) >= 1:
        market = lowered[:2]
        digits = re.sub(r"\D", "", lowered[2:]).zfill(6)[-6:]
        suffix = {"sh": "SH", "sz": "SZ", "bj": "BJ"}[market]
        exchange = {"sh": "SSE", "sz": "SZSE", "bj": "BJSE"}[market]
        return NormalizedSymbol(value, f"{digits}.{suffix}", f"{market}{digits}", exchange, digits)
    if lowered.startswith("hk") and len(re.sub(r"\D", "", lowered[2:])) >= 1:
        digits = re.sub(r"\D", "", lowered[2:]).zfill(5)[-5:]
        return NormalizedSymbol(value, f"{digits}.HK", f"hk{digits}", "HKEX", digits)

    if value.isdigit():
        if len(value) <= 5:
            digits = value.zfill(5)[-5:]
            return NormalizedSymbol(value, f"{digits}.HK", f"hk{digits}", "HKEX", digits)
        digits = value.zfill(6)[-6:]
        market = detect_a_share_market(digits)
        suffix = {"sh": "SH", "sz": "SZ", "bj": "BJ"}[market]
        exchange = {"sh": "SSE", "sz": "SZSE", "bj": "BJSE"}[market]
        return NormalizedSymbol(value, f"{digits}.{suffix}", f"{market}{digits}", exchange, digits)

    raise ValueError(f"暂不支持的股票代码格式: {raw}")


def detect_a_share_market(code: str) -> str:
    if code.startswith(("60", "68", "9")):
        return "sh"
    if code.startswith(("43", "83", "87", "8", "4", "920")):
        return "bj"
    return "sz"


def _http_get(url: str, *, timeout: int = NEWS_KLINE_HTTP_TIMEOUT_SECONDS, retries: int = NEWS_KLINE_HTTP_RETRIES) -> str:
    last_error: Optional[Exception] = None
    for attempt in range(max(1, retries)):
        try:
            req = urllib.request.Request(url, headers=_UA)
            with urllib.request.urlopen(req, timeout=timeout) as response:
                text = response.read().decode("utf-8", "ignore")
            if text.strip():
                return text
            last_error = RuntimeError("empty response")
        except Exception as exc:  # pragma: no cover - real network path
            last_error = exc
        if attempt < retries - 1:
            time.sleep(0.25 * (attempt + 1))
    raise RuntimeError(f"数据源请求失败: {last_error}")


def get_news_kline_report(symbol: str, days: int = 500, pages: int = 3, force: bool = False) -> Dict[str, Any]:
    normalized = normalize_symbol(symbol)
    days = clamp_int(days, 500, NEWS_KLINE_MIN_DAYS, NEWS_KLINE_MAX_DAYS)
    pages = clamp_int(pages, 3, NEWS_KLINE_MIN_PAGES, NEWS_KLINE_MAX_PAGES)
    cache_key = f"{normalized.symbol}:{days}:{pages}"
    now = datetime.utcnow()

    if not force:
        cached = _get_cached_report(cache_key, now)
        if cached is not None:
            cached.setdefault("META", {})["cache_status"] = "hit"
            cached.setdefault("META", {})["cache_ttl_seconds"] = NEWS_KLINE_CACHE_TTL_SECONDS
            return cached

    try:
        payload = build_report(normalized, days=days, pages=pages)
    except Exception as exc:
        stale = _get_stale_report(cache_key)
        if stale is not None:
            meta = stale.setdefault("META", {})
            warnings = list(meta.get("warnings") or [])
            warnings.append(f"刷新失败，已返回最近一次缓存: {exc}")
            meta["warnings"] = warnings
            meta["cache_status"] = "stale"
            meta["last_error"] = str(exc)
            return stale
        raise

    payload.setdefault("META", {})["cache_status"] = "fresh"
    payload.setdefault("META", {})["cache_ttl_seconds"] = NEWS_KLINE_CACHE_TTL_SECONDS
    _set_cached_report(cache_key, payload, now)
    return copy.deepcopy(payload)


def _get_cached_report(cache_key: str, now: datetime) -> Optional[Dict[str, Any]]:
    with _CACHE_LOCK:
        entry = _REPORT_CACHE.get(cache_key)
        if not entry:
            return None
        fetched_at = entry.get("fetched_at")
        if not isinstance(fetched_at, datetime):
            return None
        if (now - fetched_at).total_seconds() >= NEWS_KLINE_CACHE_TTL_SECONDS:
            return None
        return copy.deepcopy(entry.get("payload"))


def _get_stale_report(cache_key: str) -> Optional[Dict[str, Any]]:
    with _CACHE_LOCK:
        entry = _REPORT_CACHE.get(cache_key)
        if not entry:
            return None
        return copy.deepcopy(entry.get("payload"))


def _set_cached_report(cache_key: str, payload: Dict[str, Any], now: datetime) -> None:
    with _CACHE_LOCK:
        _REPORT_CACHE[cache_key] = {"fetched_at": now, "payload": copy.deepcopy(payload)}


def build_report(symbol: NormalizedSymbol, *, days: int, pages: int) -> Dict[str, Any]:
    warnings: List[str] = []
    kline = fetch_kline(symbol, days=days)
    if not kline:
        raise RuntimeError("K线数据获取失败")

    raw_events: List[Dict[str, Any]] = []
    for info_type in (0, 2):
        items, item_warnings = fetch_info_items(symbol, info_type=info_type, pages=pages)
        raw_events.extend(items)
        warnings.extend(item_warnings)

    events = build_events(raw_events, kline)
    stats = build_stats(events)
    used_categories = {event["category"] for event in events}
    cats = {key: value for key, value in CATEGORY_META.items() if key in used_categories}
    latest = kline[-1]
    first = kline[0]
    meta = {
        "name": symbol.symbol,
        "symbol": symbol.symbol,
        "quote_code": symbol.quote_code,
        "exchange": symbol.exchange,
        "start": first["date"],
        "end": latest["date"],
        "source_trade_date": latest["date"],
        "n_events": len(events),
        "n_kline": len(kline),
        "generated_at": datetime.now(_CST).strftime("%Y-%m-%d %H:%M"),
        "adjustment": "qfq",
        "data_source": "tencent_gtimg",
        "event_sources": ["tencent_info_search:type0", "tencent_info_search:type2"],
        "warnings": unique_preserve_order(warnings),
    }
    return {"KLINE": kline, "EVENTS": events, "STATS": stats, "META": meta, "CATS": cats}


def fetch_kline(symbol: NormalizedSymbol, *, days: int) -> List[Dict[str, Any]]:
    window = max(days + 20, NEWS_KLINE_MIN_DAYS)
    if symbol.exchange == "HKEX":
        urls = [
            f"https://web.ifzq.gtimg.cn/appstock/app/hkfqkline/get?param={symbol.quote_code},day,,,{window},qfq",
            f"https://ifzq.gtimg.cn/appstock/app/hkfqkline/get?param={symbol.quote_code},day,,,{window},qfq",
        ]
    else:
        urls = [
            f"https://web.ifzq.gtimg.cn/appstock/app/fqkline/get?param={symbol.quote_code},day,,,{window},qfq",
            f"https://ifzq.gtimg.cn/appstock/app/fqkline/get?param={symbol.quote_code},day,,,{window},qfq",
        ]
    last_error: Optional[Exception] = None
    for url in urls:
        try:
            text = _http_get(url)
            payload = json.loads(text)
            node = (payload.get("data") or {}).get(symbol.quote_code) or next(iter((payload.get("data") or {}).values()), {})
            rows = node.get("qfqday") or node.get("day") or []
            bars = normalize_kline_rows(rows)
            if bars:
                return bars[-days:]
        except Exception as exc:
            last_error = exc
            logger.debug("新闻透视 K线源失败 symbol=%s url=%s error=%s", symbol.symbol, url, exc)
    raise RuntimeError(f"K线数据源不可用: {last_error}")


def normalize_kline_rows(rows: Sequence[Sequence[Any]]) -> List[Dict[str, Any]]:
    bars: List[Dict[str, Any]] = []
    for row in rows:
        if len(row) < 6:
            continue
        date = clean_text(row[0])
        if not re.match(r"^20\d{2}-\d{2}-\d{2}$", date):
            continue
        open_price = parse_float(row[1])
        close = parse_float(row[2])
        high = parse_float(row[3])
        low = parse_float(row[4])
        volume = parse_float(row[5])
        if open_price is None or close is None or high is None or low is None or close <= 0:
            continue
        bars.append({
            "date": date,
            "open": open_price,
            "close": close,
            "high": max(high, low),
            "low": min(high, low),
            "volume": max(volume or 0, 0),
        })
    bars.sort(key=lambda item: item["date"])
    deduped: Dict[str, Dict[str, Any]] = {bar["date"]: bar for bar in bars}
    return [deduped[key] for key in sorted(deduped)]


def fetch_info_items(symbol: NormalizedSymbol, *, info_type: int, pages: int) -> Tuple[List[Dict[str, Any]], List[str]]:
    items: List[Dict[str, Any]] = []
    warnings: List[str] = []
    label = {0: "公告", 2: "新闻"}.get(info_type, "资讯")
    for page in range(1, pages + 1):
        url = (
            "https://ifzq.gtimg.cn/appstock/news/info/search?"
            + urllib.parse.urlencode({"symbol": symbol.quote_code, "page": page, "n": NEWS_KLINE_INFO_PAGE_SIZE, "type": info_type})
        )
        try:
            text = _http_get(url)
            payload = json.loads(text)
            data = ((payload.get("data") or {}).get("data") or [])
        except Exception as exc:
            warnings.append(f"{label}第{page}页暂不可用")
            logger.debug("新闻透视资讯源失败 symbol=%s type=%s page=%s error=%s", symbol.symbol, info_type, page, exc)
            break
        if not data:
            break
        for row in data:
            published = clean_text(row.get("time"))
            title = clean_text(row.get("title"))
            if not published or not title:
                continue
            items.append({
                "id": clean_text(row.get("id")) or build_event_id(symbol.symbol, info_type, title, published),
                "title": title,
                "time": published,
                "date": published[:10],
                "url": clean_text(row.get("url")),
                "src": clean_text(row.get("src")),
                "info_type": info_type,
                "info_type_str": clean_text(row.get("typeStr")) or label,
            })
        time.sleep(0.08)
    return items, warnings


def build_events(raw_events: Sequence[Dict[str, Any]], kline: Sequence[Dict[str, Any]]) -> List[Dict[str, Any]]:
    trade_dates = [bar["date"] for bar in kline]
    events: List[Dict[str, Any]] = []
    seen = set()
    for raw in raw_events:
        title = clean_text(raw.get("title"))
        event_date = clean_text(raw.get("date"))[:10]
        if not title or not event_date:
            continue
        trade_date = resolve_effective_trade_date(trade_dates, event_date)
        if not trade_date:
            continue
        key = f"{trade_date}|{event_date}|{title}"
        if key in seen:
            continue
        seen.add(key)
        category = classify_event(title)
        impact = impact_for(kline, trade_date)
        event = {
            "id": clean_text(raw.get("id")) or build_event_id("", raw.get("info_type"), title, event_date),
            "title": title,
            "date": event_date,
            "trade_date": trade_date,
            "time": clean_text(raw.get("time")),
            "url": clean_text(raw.get("url")),
            "src": clean_text(raw.get("src")),
            "info_type": raw.get("info_type"),
            "info_type_str": clean_text(raw.get("info_type_str")),
            "category": category,
            "impact": impact,
        }
        if trade_date != event_date:
            event["date_note"] = "non_trading_day_mapped_to_next_trade_date"
        events.append(event)
    events.sort(key=lambda item: (item["trade_date"], item.get("time") or item["date"]), reverse=True)
    return events


def resolve_effective_trade_date(trade_dates: Sequence[str], event_date: str) -> str:
    if event_date in trade_dates:
        return event_date
    for date in trade_dates:
        if date >= event_date:
            return date
    return ""


def classify_event(title: str) -> str:
    haystack = title.upper()
    for category, words in KEYWORDS:
        for word in words:
            if word.upper() in haystack:
                return category
    return "行业市场" if any(keyword in title for keyword in ("行业", "政策", "市场", "板块", "指数")) else "其他"


def find_index(kline: Sequence[Dict[str, Any]], date: str) -> Optional[int]:
    for index, bar in enumerate(kline):
        if bar.get("date") == date:
            return index
    return None


def pct(current: float, base: float) -> Optional[float]:
    if not base:
        return None
    return current / base - 1


def impact_for(kline: Sequence[Dict[str, Any]], trade_date: str) -> Optional[Dict[str, Optional[float]]]:
    idx = find_index(kline, trade_date)
    if idx is None:
        return None
    close = float(kline[idx]["close"])
    prev_close = float(kline[idx - 1]["close"]) if idx > 0 else close
    result: Dict[str, Optional[float]] = {
        "day_change": pct(close, prev_close) if idx > 0 else None,
        "open_close": pct(close, float(kline[idx]["open"])),
    }
    for days in (1, 3, 5, 10):
        end_idx = min(idx + days, len(kline) - 1)
        window = kline[idx:end_idx + 1]
        result[f"ret_{days}d"] = pct(float(kline[end_idx]["close"]), close)
        result[f"max_up_{days}d"] = pct(max(float(item["high"]) for item in window), close) if window else None
        result[f"max_down_{days}d"] = pct(min(float(item["low"]) for item in window), close) if window else None
    return result


def build_stats(events: Sequence[Dict[str, Any]]) -> List[Dict[str, Any]]:
    groups: Dict[str, List[Dict[str, Any]]] = {}
    for event in events:
        groups.setdefault(event["category"], []).append(event)
    stats: List[Dict[str, Any]] = []
    for category, rows in groups.items():
        impacts = [event.get("impact") for event in rows if event.get("impact")]
        ret3 = [impact.get("ret_3d") for impact in impacts]
        ret1 = [impact.get("ret_1d") for impact in impacts]
        day = [impact.get("day_change") for impact in impacts]
        abs3 = [abs(value) for value in ret3 if is_number(value)]
        stat = {
            "category": category,
            "count": len(rows),
            "avg_day": safe_mean(day),
            "avg_1d": safe_mean(ret1),
            "avg_3d": safe_mean(ret3),
            "win_3d": win_rate(ret3),
            "abs_3d": safe_mean(abs3),
            "max_up": safe_max([impact.get("max_up_3d") for impact in impacts]),
            "max_down": safe_min([impact.get("max_down_3d") for impact in impacts]),
            "color": CATEGORY_META.get(category, CATEGORY_META["其他"])["color"],
        }
        if is_number(stat["abs_3d"]) and is_number(stat["win_3d"]):
            stat["explain_score"] = stat["abs_3d"] * (0.5 + abs(stat["win_3d"] - 0.5))
        else:
            stat["explain_score"] = None
        stats.append(stat)
    stats.sort(key=lambda item: item["explain_score"] if item["explain_score"] is not None else -1, reverse=True)
    return stats


def is_number(value: Any) -> bool:
    try:
        return value is not None and not math.isnan(float(value)) and math.isfinite(float(value))
    except Exception:
        return False


def safe_mean(values: Iterable[Any]) -> Optional[float]:
    nums = [float(value) for value in values if is_number(value)]
    return sum(nums) / len(nums) if nums else None


def safe_max(values: Iterable[Any]) -> Optional[float]:
    nums = [float(value) for value in values if is_number(value)]
    return max(nums) if nums else None


def safe_min(values: Iterable[Any]) -> Optional[float]:
    nums = [float(value) for value in values if is_number(value)]
    return min(nums) if nums else None


def win_rate(values: Iterable[Any]) -> Optional[float]:
    nums = [float(value) for value in values if is_number(value)]
    if not nums:
        return None
    return sum(1 for value in nums if value > 0) / len(nums)


def clean_text(value: Any) -> str:
    return re.sub(r"\s+", " ", str(value or "").strip())


def parse_float(value: Any) -> Optional[float]:
    try:
        parsed = float(str(value).strip())
    except Exception:
        return None
    if not math.isfinite(parsed):
        return None
    return parsed


def build_event_id(symbol: str, info_type: Any, title: str, published: str) -> str:
    raw = f"{symbol}|{info_type}|{title}|{published}"
    return hashlib.md5(raw.encode("utf-8")).hexdigest()


def unique_preserve_order(items: Iterable[str]) -> List[str]:
    result: List[str] = []
    seen = set()
    for item in items:
        text = clean_text(item)
        if not text or text in seen:
            continue
        seen.add(text)
        result.append(text)
    return result
