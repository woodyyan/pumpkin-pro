#!/usr/bin/env python3
"""Probe dividend-yield related fields from AkShare, EastMoney, and Tencent.

This script is intentionally read-only: it does not write pumpkin.db. Use it to
inspect which upstream source returns usable dividend_yield / cash dividend
fields before changing Factor Lab production ingestion.
"""

from __future__ import annotations

import argparse
import json
import re
import sys
import traceback
from dataclasses import dataclass, asdict
from pathlib import Path
from typing import Any, Optional

PROJECT_ROOT = Path(__file__).resolve().parents[2]
QUANT_ROOT = PROJECT_ROOT / "quant"
if str(QUANT_ROOT) not in sys.path:
    sys.path.insert(0, str(QUANT_ROOT))

EASTMONEY_DATACENTER_URL = "https://datacenter-web.eastmoney.com/api/data/v1/get"
DEFAULT_CODES = ["603459", "600519", "000001", "601318", "000858"]
DIVIDEND_KEYWORDS = [
    "股息", "股利", "分红", "派息", "派现", "现金", "红利", "每股", "10派", "每10股",
    "dividend", "yield", "bonus", "cash", "dps", "rate", "ratio",
]


@dataclass
class CandidateField:
    field: str
    non_null_count: int
    samples: list[str]


@dataclass
class ParsedCandidate:
    field: str
    raw_value: str
    dividend_yield: Optional[float] = None
    cash_dividend_per_share: Optional[float] = None
    reason: str = ""


@dataclass
class SourceProbeResult:
    code: str
    source: str
    ok: bool
    row_count: int = 0
    columns: list[str] | None = None
    candidate_fields: list[CandidateField] | None = None
    parsed_candidates: list[ParsedCandidate] | None = None
    sample_rows: list[dict[str, Any]] | None = None
    error: str = ""


def parse_args(argv: list[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Probe dividend-yield fields from upstream data sources")
    parser.add_argument("--codes", default=",".join(DEFAULT_CODES), help="逗号分隔股票代码，默认包含 603459/600519/000001/601318/000858")
    parser.add_argument("--sources", default="akshare,eastmoney,tencent", help="逗号分隔数据源：akshare,eastmoney,tencent")
    parser.add_argument("--sample-rows", type=int, default=3, help="每个数据源打印多少条样例行")
    parser.add_argument("--timeout", type=int, default=15, help="HTTP 超时时间秒")
    parser.add_argument("--json-output", default="", help="可选：把完整探测结果写入 JSON 文件")
    parser.add_argument("--verbose", action="store_true", help="失败时输出 traceback")
    return parser.parse_args(argv)


def split_csv(value: str) -> list[str]:
    return [item.strip() for item in str(value or "").split(",") if item.strip()]


def normalize_code(raw: Any) -> str:
    digits = "".join(ch for ch in str(raw or "") if ch.isdigit())
    return digits.zfill(6)[-6:] if digits else ""


def infer_symbol(code: str) -> str:
    code = normalize_code(code)
    if code.startswith("6"):
        return f"{code}.SH"
    return f"{code}.SZ"


def safe_float(value: Any) -> Optional[float]:
    if value is None:
        return None
    text = str(value).strip().replace(",", "")
    if not text or text.lower() in {"none", "nan", "null", "--", "-"}:
        return None
    match = re.search(r"[-+]?\d+(?:\.\d+)?", text)
    if not match:
        return None
    try:
        return float(match.group(0))
    except ValueError:
        return None


def normalize_dividend_yield(value: Any) -> Optional[float]:
    parsed = safe_float(value)
    if parsed is None:
        return None
    text = str(value or "")
    if "%" in text or parsed > 1:
        return parsed / 100
    return parsed


def parse_cash_dividend_per_share(value: Any) -> Optional[float]:
    text = str(value or "").strip()
    if not text:
        return None
    patterns = [
        r"(?:每)?10股[^\d-]*派\s*([-+]?\d+(?:\.\d+)?)",
        r"10\s*派\s*([-+]?\d+(?:\.\d+)?)",
        r"派\s*([-+]?\d+(?:\.\d+)?)\s*元",
    ]
    for pattern in patterns:
        match = re.search(pattern, text)
        if match:
            try:
                return float(match.group(1)) / 10.0
            except ValueError:
                return None
    return None


def is_candidate_field(name: str) -> bool:
    lowered = str(name or "").lower()
    return any(keyword.lower() in lowered for keyword in DIVIDEND_KEYWORDS)


def dataframe_to_records(df: Any, limit: int) -> list[dict[str, Any]]:
    if df is None or getattr(df, "empty", True):
        return []
    rows = df.head(max(limit, 0)).to_dict(orient="records")
    return [{str(k): stringify_value(v) for k, v in row.items()} for row in rows]


def stringify_value(value: Any) -> str:
    if value is None:
        return ""
    try:
        import pandas as pd  # type: ignore
        if pd.isna(value):
            return ""
    except Exception:
        pass
    return str(value)


def inspect_dataframe(code: str, source: str, df: Any, sample_rows: int) -> SourceProbeResult:
    columns = [str(item) for item in list(getattr(df, "columns", []))]
    candidate_columns = [col for col in columns if is_candidate_field(col)]
    candidate_fields: list[CandidateField] = []
    parsed_candidates: list[ParsedCandidate] = []
    if df is not None and not getattr(df, "empty", True):
        for col in candidate_columns:
            series = df[col]
            non_null = [stringify_value(item) for item in series.tolist() if stringify_value(item).strip()]
            samples = non_null[:5]
            candidate_fields.append(CandidateField(field=col, non_null_count=len(non_null), samples=samples))
            for raw in samples[:3]:
                parsed_yield = normalize_dividend_yield(raw) if ("率" in col or "yield" in col.lower() or "rate" in col.lower() or "ratio" in col.lower()) else None
                parsed_cash = parse_cash_dividend_per_share(raw)
                if parsed_yield is not None or parsed_cash is not None:
                    parsed_candidates.append(ParsedCandidate(field=col, raw_value=raw, dividend_yield=parsed_yield, cash_dividend_per_share=parsed_cash, reason="field/name heuristic"))
    return SourceProbeResult(
        code=code,
        source=source,
        ok=True,
        row_count=0 if df is None else int(len(df)),
        columns=columns,
        candidate_fields=candidate_fields,
        parsed_candidates=parsed_candidates,
        sample_rows=dataframe_to_records(df, sample_rows),
    )


def probe_akshare(code: str, args: argparse.Namespace) -> SourceProbeResult:
    import akshare as ak  # type: ignore
    df = ak.stock_fhps_detail_em(symbol=code)
    return inspect_dataframe(code, "akshare:stock_fhps_detail_em", df, args.sample_rows)


def probe_eastmoney(code: str, args: argparse.Namespace) -> SourceProbeResult:
    import pandas as pd  # type: ignore
    import requests  # type: ignore
    rows: list[dict[str, Any]] = []
    page = 1
    while True:
        params = {
            "sortColumns": "EX_DIVIDEND_DATE",
            "sortTypes": "-1",
            "pageSize": 100,
            "pageNumber": page,
            "reportName": "RPT_SHAREBONUS_DET",
            "columns": "ALL",
            "filter": f'(SECURITY_CODE="{code}")',
            "source": "WEB",
            "client": "WEB",
        }
        resp = requests.get(EASTMONEY_DATACENTER_URL, params=params, timeout=args.timeout)
        resp.raise_for_status()
        payload = resp.json()
        result = payload.get("result") or {}
        data = result.get("data") or []
        if not data:
            break
        rows.extend(data)
        pages = int(result.get("pages") or 1)
        if page >= pages:
            break
        page += 1
    df = pd.DataFrame(rows)
    return inspect_dataframe(code, "eastmoney:RPT_SHAREBONUS_DET", df, args.sample_rows)


def flatten_dict(prefix: str, value: Any, output: dict[str, Any]) -> None:
    if isinstance(value, dict):
        for key, child in value.items():
            flatten_dict(f"{prefix}.{key}" if prefix else str(key), child, output)
    elif isinstance(value, list):
        for idx, child in enumerate(value[:5]):
            flatten_dict(f"{prefix}[{idx}]", child, output)
    else:
        output[prefix] = value


def probe_tencent(code: str, args: argparse.Namespace) -> SourceProbeResult:
    from data.fundamentals import get_symbol_fundamentals  # type: ignore
    payload = get_symbol_fundamentals(infer_symbol(code))
    flattened: dict[str, Any] = {}
    flatten_dict("", payload, flattened)
    columns = sorted(flattened.keys())
    candidate_fields: list[CandidateField] = []
    parsed_candidates: list[ParsedCandidate] = []
    for field in columns:
        if not is_candidate_field(field):
            continue
        raw = stringify_value(flattened.get(field))
        samples = [raw] if raw else []
        candidate_fields.append(CandidateField(field=field, non_null_count=1 if raw else 0, samples=samples))
        if raw:
            parsed_yield = normalize_dividend_yield(raw) if ("yield" in field.lower() or "股息" in field or "率" in field) else None
            parsed_cash = parse_cash_dividend_per_share(raw)
            if parsed_yield is not None or parsed_cash is not None:
                parsed_candidates.append(ParsedCandidate(field=field, raw_value=raw, dividend_yield=parsed_yield, cash_dividend_per_share=parsed_cash, reason="flattened payload heuristic"))
    return SourceProbeResult(
        code=code,
        source="tencent:get_symbol_fundamentals",
        ok=True,
        row_count=1,
        columns=columns,
        candidate_fields=candidate_fields,
        parsed_candidates=parsed_candidates,
        sample_rows=[{key: stringify_value(value) for key, value in flattened.items() if is_candidate_field(key)}],
    )


def run_probe(code: str, source: str, args: argparse.Namespace) -> SourceProbeResult:
    try:
        if source == "akshare":
            return probe_akshare(code, args)
        if source == "eastmoney":
            return probe_eastmoney(code, args)
        if source == "tencent":
            return probe_tencent(code, args)
        return SourceProbeResult(code=code, source=source, ok=False, error=f"unsupported source: {source}")
    except Exception as exc:  # noqa: BLE001 - probe should continue across sources
        if args.verbose:
            traceback.print_exc()
        return SourceProbeResult(code=code, source=source, ok=False, error=f"{type(exc).__name__}: {exc}")


def print_result(result: SourceProbeResult) -> None:
    status = "OK" if result.ok else "FAILED"
    print(f"\n== {result.code} | {result.source} | {status} ==", flush=True)
    if not result.ok:
        print(f"error: {result.error}", flush=True)
        return
    print(f"rows: {result.row_count}", flush=True)
    print(f"columns({len(result.columns or [])}): {', '.join(result.columns or [])}", flush=True)
    if result.candidate_fields:
        print("candidate fields:", flush=True)
        for item in result.candidate_fields:
            print(f"  - {item.field}: non_null={item.non_null_count}, samples={item.samples}", flush=True)
    else:
        print("candidate fields: none", flush=True)
    if result.parsed_candidates:
        print("parsed candidates:", flush=True)
        for item in result.parsed_candidates:
            print(f"  - {item.field}: raw={item.raw_value!r}, dividend_yield={item.dividend_yield}, cash_per_share={item.cash_dividend_per_share}", flush=True)
    else:
        print("parsed candidates: none", flush=True)
    if result.sample_rows:
        print("sample rows:", flush=True)
        for row in result.sample_rows:
            print("  " + json.dumps(row, ensure_ascii=False, default=str), flush=True)


def main(argv: list[str]) -> int:
    args = parse_args(argv)
    codes = [normalize_code(item) for item in split_csv(args.codes)]
    codes = [item for item in codes if item]
    sources = split_csv(args.sources)
    results: list[SourceProbeResult] = []
    for code in codes:
        for source in sources:
            result = run_probe(code, source, args)
            results.append(result)
            print_result(result)
    if args.json_output:
        output_path = Path(args.json_output)
        output_path.parent.mkdir(parents=True, exist_ok=True)
        output_path.write_text(json.dumps([asdict(item) for item in results], ensure_ascii=False, indent=2, default=str), encoding="utf-8")
        print(f"\njson output written: {output_path}", flush=True)
    failures = [item for item in results if not item.ok]
    print(f"\nsummary: total={len(results)} ok={len(results)-len(failures)} failed={len(failures)}", flush=True)
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
