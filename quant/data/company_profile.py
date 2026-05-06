from __future__ import annotations

import json
import re
from datetime import datetime
from typing import Any, Dict, Iterable, List, Optional, Tuple
from urllib.parse import urlparse

WEAK_INDUSTRY_VALUES = {"", "--", "-", "none", "null", "nan", "其他", "其它", "未知", "未分类"}
GENERIC_BUSINESS_TERMS = {"服务", "咨询", "投资", "管理", "销售", "贸易", "技术服务", "技术咨询"}
LEGAL_NOISE_PATTERNS = [
    r"依法须经批准的项目[，,、；;。\s]*经相关部门批准后方可开展经营活动[。；;]?",
    r"许可项目[:：]",
    r"一般项目[:：]",
    r"凭营业执照依法自主开展经营活动[。；;]?",
    r"货物进出口",
    r"技术进出口",
]


def normalize_symbol(symbol: str) -> Tuple[str, str, str]:
    raw = str(symbol or "").strip().upper()
    if not raw:
        raise ValueError("股票代码不能为空")
    if raw.endswith(".SH"):
        code = raw[:-3]
        if len(code) == 6 and code.isdigit() and code.startswith("6"):
            return raw, "SSE", code
        raise ValueError("A 股代码格式无效")
    if raw.endswith(".SZ"):
        code = raw[:-3]
        if len(code) == 6 and code.isdigit() and code.startswith(("0", "3")):
            return raw, "SZSE", code
        raise ValueError("A 股代码格式无效")
    if raw.endswith(".HK"):
        code = raw[:-3]
        if code.isdigit():
            code = code.zfill(5)
            return f"{code}.HK", "HKEX", code
        raise ValueError("港股代码格式无效")
    digits = "".join(ch for ch in raw if ch.isdigit())
    if len(digits) == 6:
        if digits.startswith("6"):
            return f"{digits}.SH", "SSE", digits
        if digits.startswith(("0", "3")):
            return f"{digits}.SZ", "SZSE", digits
    if 1 <= len(digits) <= 5:
        code = digits.zfill(5)
        return f"{code}.HK", "HKEX", code
    raise ValueError("股票代码格式无效，A 股请传 600519.SH / 000001.SZ，港股请传 00700.HK")


def normalize_industry_name(raw: Any) -> str:
    text = str(raw or "").strip()
    text = re.sub(r"\s+", " ", text)
    if text.lower() in WEAK_INDUSTRY_VALUES:
        return ""
    text = text.replace("（", "(").replace("）", ")")
    text = re.sub(r"\((申万|东财行业|东方财富|港交所|HKEX|GICS|来源[:：]?.*?)\)$", "", text, flags=re.IGNORECASE)
    text = re.sub(r"[ⅠⅡⅢ]+$", "", text).strip()
    text = re.sub(r"\b(?:I|II|III)\b$", "", text, flags=re.IGNORECASE).strip()
    text = re.sub(r"(行业|板块|概念)$", "", text).strip()
    for sep in ("-", "—", "/", "|"):
        if sep in text:
            text = text.split(sep, 1)[0].strip()
            break
    if text.lower() in WEAK_INDUSTRY_VALUES:
        return ""
    return text


def normalize_website(value: Any) -> str:
    text = str(value or "").strip()
    if not text:
        return ""
    if text.lower().startswith(("javascript:", "data:")):
        return ""
    candidate = text if text.startswith(("http://", "https://")) else f"https://{text}"
    parsed = urlparse(candidate)
    if parsed.scheme not in {"http", "https"} or not parsed.netloc:
        return ""
    return candidate


def normalize_date(value: Any) -> Tuple[str, str]:
    if value is None:
        return "", "unknown"
    if hasattr(value, "strftime"):
        return value.strftime("%Y-%m-%d"), "day"
    text = str(value).strip()
    if not text or text.lower() in WEAK_INDUSTRY_VALUES:
        return "", "unknown"
    text = text.replace("/", "-").replace(".", "-")
    if re.fullmatch(r"\d{8}", text):
        return f"{text[:4]}-{text[4:6]}-{text[6:8]}", "day"
    if re.fullmatch(r"\d{4}-\d{1,2}-\d{1,2}", text):
        parts = [int(p) for p in text.split("-")]
        return f"{parts[0]:04d}-{parts[1]:02d}-{parts[2]:02d}", "day"
    if re.fullmatch(r"\d{4}-\d{1,2}", text):
        year, month = [int(p) for p in text.split("-")]
        return f"{year:04d}-{month:02d}-01", "month"
    if re.fullmatch(r"\d{4}", text):
        return f"{int(text):04d}-01-01", "year"
    try:
        parsed = datetime.fromisoformat(text[:10])
        return parsed.strftime("%Y-%m-%d"), "day"
    except Exception:
        return "", "unknown"


def clean_business_text(text: Any) -> str:
    cleaned = str(text or "").strip()
    cleaned = re.sub(r"\s+", "", cleaned)
    for pattern in LEGAL_NOISE_PATTERNS:
        cleaned = re.sub(pattern, "", cleaned)
    cleaned = re.sub(r"(注册地址|注册资本|法定代表人|股票发行).*?(。|；|;)", "", cleaned)
    cleaned = cleaned.strip("，,；;。 ")
    return cleaned


def extract_core_business_phrases(text: Any, limit: int = 4) -> List[str]:
    cleaned = clean_business_text(text)
    if not cleaned:
        return []
    markers = ["主要从事", "主营业务为", "主营业务是", "主要业务包括", "业务涵盖", "是一家从事"]
    for marker in markers:
        if marker in cleaned:
            cleaned = cleaned.split(marker, 1)[1]
            break
    cleaned = re.split(r"[。.]", cleaned, maxsplit=1)[0]
    parts = re.split(r"[、,，；;]", cleaned)
    phrases: List[str] = []
    for part in parts:
        phrase = part.strip("包括以及和等的 ")
        if not phrase or phrase in GENERIC_BUSINESS_TERMS:
            continue
        if len(phrase) < 3:
            continue
        if phrase not in phrases:
            phrases.append(phrase)
        if len(phrases) >= limit:
            break
    return phrases


def build_business_summary(profile: Dict[str, Any]) -> Tuple[str, str, List[str]]:
    name = str(profile.get("name") or profile.get("short_name") or "该公司").strip() or "该公司"
    industry = normalize_industry_name(profile.get("industry_name") or profile.get("raw_industry_name"))
    board = str(profile.get("board_name") or "").strip()
    flags: List[str] = []

    for key in ("main_business", "主营业务", "主要产品及业务"):
        phrases = extract_core_business_phrases(profile.get(key))
        if phrases:
            summary = f"{name}主要从事{'、'.join(phrases[:3])}"
            if industry:
                summary += f"，所属行业为{industry}"
            return _trim_sentence(summary), "source_extract", flags

    intro = clean_business_text(profile.get("company_intro") or profile.get("公司简介"))
    if intro and ("是一家" in intro or "主要从事" in intro):
        first_sentence = re.split(r"[。.]", intro, maxsplit=1)[0]
        return _trim_sentence(first_sentence), "source_extract", flags

    phrases = extract_core_business_phrases(profile.get("business_scope") or profile.get("经营范围"))
    if phrases:
        return _trim_sentence(f"{name}业务涉及{'、'.join(phrases[:3])}等"), "rule_template", flags

    flags.append("summary_fallback")
    if industry and board:
        return f"{name}属于{industry}行业，在{board}上市交易，具体主营业务资料暂待补全。", "fallback", flags
    if industry:
        return f"{name}属于{industry}行业，具体主营业务资料暂待补全。", "fallback", flags
    return "该公司的业务介绍暂待补全。", "fallback", flags


def _trim_sentence(text: str, max_len: int = 90) -> str:
    text = re.sub(r"(低估|推荐|值得关注|有望受益|最强|唯一|第一|龙头)", "", text)
    text = text.strip("，,；;。 ")
    if len(text) > max_len:
        text = text[:max_len].rstrip("，,；;、 ")
    return text + "。"


def _first_non_empty(mapping: Dict[str, Any], keys: Iterable[str]) -> str:
    for key in keys:
        value = str(mapping.get(key) or "").strip()
        if value and value.lower() not in WEAK_INDUSTRY_VALUES:
            return value
    return ""


def _akshare_individual_info(code: str) -> Dict[str, Any]:
    import akshare as ak

    df = ak.stock_individual_info_em(symbol=code)
    if df is None or df.empty:
        return {}
    result: Dict[str, Any] = {}
    for _, row in df.iterrows():
        key = str(row.get("item") or "").strip()
        if key:
            result[key] = row.get("value")
    return result


def fetch_a_share_company_profile(symbol: str) -> Dict[str, Any]:
    normalized, exchange, code = normalize_symbol(symbol)
    info = _akshare_individual_info(code)
    raw_industry = _first_non_empty(info, ["行业", "所属行业", "东财行业", "申万行业"])
    industry = normalize_industry_name(raw_industry)
    founded_date, founded_precision = normalize_date(_first_non_empty(info, ["成立时间", "成立日期"]))
    ipo_date, _ = normalize_date(_first_non_empty(info, ["上市时间", "上市日期", "IPO日期"]))
    summary, summary_source, flags = build_business_summary({
        "name": _first_non_empty(info, ["股票简称", "股票名称", "名称"]) or normalized,
        "industry_name": industry,
        "board_name": _classify_a_share_board_name(code),
        "business_scope": _first_non_empty(info, ["经营范围", "主营业务", "公司简介"]),
    })
    return {
        "symbol": normalized,
        "exchange": exchange,
        "code": code,
        "name": _first_non_empty(info, ["股票简称", "股票名称", "名称"]) or normalized,
        "full_name": _first_non_empty(info, ["公司名称", "股票名称"]),
        "board_code": _classify_a_share_board_code(code),
        "board_name": _classify_a_share_board_name(code),
        "raw_industry_name": raw_industry,
        "industry_name": industry,
        "industry_source": "eastmoney",
        "website": normalize_website(_first_non_empty(info, ["官方网址", "网站", "官网"])),
        "founded_date": founded_date,
        "founded_date_precision": founded_precision,
        "ipo_date": ipo_date,
        "listing_status": "LISTED",
        "business_scope": _first_non_empty(info, ["经营范围", "主营业务", "公司简介"]),
        "business_summary": summary,
        "business_summary_source": summary_source,
        "profile_status": "COMPLETE" if summary_source != "fallback" else "PARTIAL",
        "quality_flags": json.dumps(flags, ensure_ascii=False),
        "source": "eastmoney",
    }


def fetch_hk_company_profile(symbol: str) -> Dict[str, Any]:
    normalized, exchange, code = normalize_symbol(symbol)
    # V1 uses the available spot/fundamental fields as a conservative shell.
    # Full HKEX/F10 enrichment can be added without changing the output schema.
    summary, summary_source, flags = build_business_summary({
        "name": normalized,
        "industry_name": "",
        "board_name": "港股主板",
    })
    return {
        "symbol": normalized,
        "exchange": exchange,
        "code": code,
        "name": normalized,
        "board_code": "HK_MAIN",
        "board_name": "港股主板",
        "listing_status": "LISTED",
        "business_summary": summary,
        "business_summary_source": summary_source,
        "profile_status": "PENDING",
        "quality_flags": json.dumps(flags + ["hk_profile_source_pending"], ensure_ascii=False),
        "source": "system",
    }


def _classify_a_share_board_code(code: str) -> str:
    if code.startswith("688") or code.startswith("689"):
        return "STAR"
    if code.startswith(("300", "301")):
        return "CHINEXT"
    return "MAIN"


def _classify_a_share_board_name(code: str) -> str:
    mapping = {"STAR": "科创板", "CHINEXT": "创业板", "MAIN": "主板"}
    return mapping.get(_classify_a_share_board_code(code), "主板")
