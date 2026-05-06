from __future__ import annotations

import argparse
import json
from pathlib import Path
from typing import Iterable, List

from data.company_profile import fetch_a_share_company_profile, fetch_hk_company_profile, normalize_symbol


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Sync static company profiles into JSONL for Go import")
    parser.add_argument("--exchange", choices=["ASHARE", "HKEX", "ALL"], default="ALL")
    parser.add_argument("--symbols", default="", help="Comma-separated symbols, e.g. 600519.SH,00700.HK")
    parser.add_argument("--output", required=True, help="Output JSONL path")
    parser.add_argument("--limit", type=int, default=0, help="Limit symbol count for smoke runs")
    parser.add_argument("--dry-run", action="store_true", help="Print records without writing file")
    return parser


def parse_symbols(raw: str) -> List[str]:
    return [item.strip() for item in str(raw or "").split(",") if item.strip()]


def fetch_profile(symbol: str) -> dict:
    normalized, exchange, _ = normalize_symbol(symbol)
    if exchange == "HKEX":
        return fetch_hk_company_profile(normalized)
    return fetch_a_share_company_profile(normalized)


def sync_symbols(symbols: Iterable[str], limit: int = 0) -> List[dict]:
    records: List[dict] = []
    for symbol in symbols:
        if limit and len(records) >= limit:
            break
        records.append(fetch_profile(symbol))
    return records


def write_jsonl(path: str, records: Iterable[dict]) -> None:
    target = Path(path)
    target.parent.mkdir(parents=True, exist_ok=True)
    with target.open("w", encoding="utf-8") as fh:
        for record in records:
            fh.write(json.dumps(record, ensure_ascii=False, default=str) + "\n")


def main(argv: list[str] | None = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)
    symbols = parse_symbols(args.symbols)
    if not symbols:
        parser.error("--symbols is required in V1; full-market universe sync can be added after source coverage is validated")
    records = sync_symbols(symbols, args.limit)
    if args.dry_run:
        for record in records:
            print(json.dumps(record, ensure_ascii=False, default=str))
        return 0
    write_jsonl(args.output, records)
    print(f"wrote {len(records)} company profiles to {args.output}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
