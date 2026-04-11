"""
Phase 0 数据验证脚本 — 港股四象限数据源可用性检查

验证内容：
1. 港股全市场快照 (ak.stock_hk_spot_em) → 股票数量、字段完整性
2. 关键指标覆盖率（PE/PB/价格/成交额）
3. 单只股票日线拉取能力（00700 腾讯控股）
4. 基本面数据可用性（fundamentals.py build_hk_payload）

用法：
  cd quant && python scripts/hk_quadrant_validate.py
"""

import sys
import os
import time

# 确保项目根目录在路径中
sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

import akshare as ak
import pandas as pd
import numpy as np


def separator(title: str):
    print(f"\n{'='*60}")
    print(f"  {title}")
    print(f"{'='*60}")


def check_hk_snapshot():
    """检查港股全市场快照数据质量。"""
    print("\n[1/4] 拉取港股全市场快照 (ak.stock_hk_spot_em)...")
    start = time.time()

    df = None
    last_exc = None

    # 最多重试 3 次，每次间隔递增
    for attempt in range(3):
        if attempt > 0:
            wait = (attempt + 1) * 3
            print(f"  第 {attempt + 1} 次尝试 (等待 {wait}s)...")
            time.sleep(wait)
        try:
            df = ak.stock_hk_spot_em()
            if df is not None and not df.empty:
                break
        except Exception as exc:
            last_exc = exc

    if df is None or df.empty:
        print(f"  ❌ 所有重试均失败: {last_exc}")
        return None

    elapsed = time.time() - start
    total = len(df)
    print(f"  ✅ 成功: {total} 只股票, 耗时 {elapsed:.1f}s")
    print(f"  列名 ({len(df.columns)}): {list(df.columns)}")

    # 检查关键字段
    key_fields = {
        "代码": "code",
        "名称": "name",
        "最新价": "price",
        "涨跌幅": "change_pct",
        "成交量": "volume",
        "成交额": "turnover",
        "市盈率-动态": "pe",
        "市净率": "pb",
        "总市值": "total_mv",
        "换手率": "turnover_rate",
    }

    print("\n  字段完整性:")
    for cn, en in key_fields.items():
        if cn in df.columns:
            valid = df[cn].notna().sum()
            pct = valid / total * 100
            bar = "█" * int(pct / 5) + "░" * (20 - int(pct / 5))
            print(f"    {cn:10s} ({en}): {valid:5d}/{total} ({pct:5.1f}%) {bar}")
        else:
            # 尝试模糊匹配
            matches = [c for c in df.columns if cn[:2] in c]
            print(f"    {cn:10s} ({en}): ❌ 缺失 (相似列: {matches or '无'})")

    # 异常值检测
    print("\n  异常值检测:")
    price_col = "最新价"
    pe_col = "市盈率-动态"
    pb_col = "市净率"

    if price_col in df.columns:
        prices = pd.to_numeric(df[price_col], errors="coerce")
        non_zero = prices[prices > 0]
        zero_count = (prices <= 0).sum() + prices.isna().sum()
        print(f"    价格 > 0:   {len(non_zero)}/{total} (零/空: {zero_count})")
        if len(non_zero) > 0:
            print(f"    价格范围:   [{non_zero.min():.2f}, {non_zero.max():.2f}] 中位={non_zero.median():.2f}")

    if pe_col in df.columns:
        pes = pd.to_numeric(df[pe_col], errors="coerce")
        valid_pe = pes[pes > 0]
        neg_pe = (pes < 0).sum()
        print(f"    PE > 0:     {len(valid_pe)}/{total} (负PE: {neg_pe})")

    if pb_col in df.columns:
        pbs = pd.to_numeric(df[pb_col], errors="coerce")
        valid_pb = pbs[pbs > 0]
        neg_pb = (pbs < 0).sum()
        print(f"    PB > 0:     {len(valid_pb)}/{total} (负PB: {neg_pb})")

    return df


def check_hk_daily_bars():
    """测试单只港股日线拉取。"""
    separator("[2/4] 测试港股日线拉取 (00700 腾讯控股)")

    from data.scripts.akshare_loader import fetch_stock_data
    from datetime import datetime, timedelta

    end_date = datetime.now()
    start_date = end_date - timedelta(days=120)

    try:
        df, source = fetch_stock_data("00700", start_date, end_date)
        print(f"  ✅ 日线获取成功 | 来源: {source}")
        print(f"  记录数: {len(df)}, 列: {list(df.columns)}")
        if not df.empty and "date" in df.columns:
            print(f"  日期范围: {df['date'].min()} ~ {df['date'].max()}")
        return df
    except Exception as exc:
        print(f"  ❌ 日线失败: {exc}")
        return None


def check_hk_fundamentals():
    """测试港股基本面数据。"""
    separator("[3/4] 测试港股基本面数据 (00700)")

    try:
        from data.fundamentals import get_symbol_fundamentals
        payload = get_symbol_fundamentals("00700.HK")
        print(f"  ✅ 基本面获取成功")
        print(f"  名称: {payload.get('name')}")
        print(f"  Exchange: {payload.get('exchange')}")
        items = payload.get("items", {})
        meta = payload.get("meta", {})
        print(f"  来源: {meta.get('source')}")
        print(f"  警告: {meta.get('warnings', [])}")
        print(f"  字段:")
        for k, v in items.items():
            label = f"{v:.2f}" if isinstance(v, (int, float)) else str(v or "N/A")
            print(f"    {k:20s}: {label}")
        return payload
    except Exception as exc:
        print(f"  ❌ 基本面失败: {exc}")
        return None


def check_benchmark_index():
    """测试恒生指数获取能力。"""
    separator("[4/4] 测试基准指数 (恒生指数 HSI)")

    try:
        import requests

        headers = {
            "User-Agent": (
                "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) "
                "AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
            ),
            "Referer": "https://stockapp.finance.qq.com/",
        }
        # 尝试多种恒生指数代码格式
        hsi_codes = ["hkHSI", "hkHSCEI", "hkHS_TECH"]
        hsi_names = ["恒生指数", "国企指数", "科技指数"]

        url = "https://web.ifzq.gtimg.cn/appstock/app/fqkline/get"
        end_date = pd.Timestamp.now().strftime("%Y-%m-%d")
        start_date = (pd.Timestamp.now() - pd.Timedelta(days=180)).strftime("%Y-%m-%d")

        for code, name in zip(hsi_codes, hsi_names):
            try:
                params = {
                    "param": f"{code},day,{start_date},{end_date},180",
                }
                resp = requests.get(url, headers=headers, timeout=15)
                resp.raise_for_status()
                data = resp.json()
                idx_data = data.get("data", {}).get(code, {})
                klines = idx_data.get("day") or idx_data.get("qfqday") or []
                if klines:
                    closes = [float(k[2]) for k in klines if len(k) >= 3]
                    try:
                        closes = [c for c in closes if c > 0]
                    except (ValueError, TypeError):
                        closes = []
                    if len(closes) >= 2:
                        lookback = min(60, len(closes))
                        ret_60d = (closes[-1] / closes[-lookback] - 1) * 100
                        print(f"  ✅ {name} ({code}) 日线成功")
                        print(f"  K线数: {len(klines)}, 最新: {closes[-1]:.2f}")
                        print(f"  近60日收益: {ret_60d:.2f}%")
                        return True
                    else:
                        print(f"  ⚠️ {name} ({code}): 无有效收盘价")
                else:
                    print(f"  ⚠️ {name} ({code}): 返回空 klines")
            except Exception as exc:
                print(f"  ⚠️ {name} ({code}): {exc}")

        # 兜底：用 AKShare 获取恒生指数
        print("\n  尝试 AKShare 兜底...")
        try:
            hsi_df = ak.stock_zh_index_spot_em()
            if hsi_df is not None:
                hsi_row = hsi_df[hsi_df["名称"].str.contains("恒生")]
                if not hsi_row.empty:
                    print(f"  ✅ AKShare 指数列表成功: {len(hsi_row)} 条恒生相关指数")
                    print(hsi_row[["代码", "名称"]].head(5).to_string(index=False))
                    return True
        except Exception as exc:
            print(f"  ❌ AKShare 也失败: {exc}")

        return False
    except Exception as exc:
        print(f"  ❌ 基准指数模块异常: {exc}")
        return False


def main():
    print("=" * 60)
    print("  港股四象限 Phase 0 — 数据源验证")
    print(f"  时间: {pd.Timestamp.now().strftime('%Y-%m-%d %H:%M:%S')}")
    print("=" * 60)

    results = {}

    # 1. 快照
    snapshot_df = check_hk_snapshot()
    results["snapshot"] = snapshot_df is not None and not snapshot_df.empty

    # 2. 日线
    daily_df = check_hk_daily_bars()
    results["daily_bars"] = daily_df is not None and not daily_df.empty

    # 3. 基本面
    fund_payload = check_hk_fundamentals()
    results["fundamentals"] = fund_payload is not None

    # 4. 基准指数
    benchmark_ok = check_benchmark_index()
    results["benchmark"] = benchmark_ok

    # 总结
    separator("验证总结")
    all_pass = all(results.values())
    status = "✅ 全部通过，可以进入 P1 开发" if all_pass else "⚠️ 部分未通过，需处理"

    print(f"\n  总体状态: {status}\n")
    for name, ok in results.items():
        icon = "✅" if ok else "❌"
        print(f"    {icon} {name}")

    # 如果有快照数据，给出统计摘要
    if snapshot_df is not None and not snapshot_df.empty:
        print(f"\n  快照数据可用于四象限评分的股票筛选:")
        price_col = [c for c in snapshot_df.columns if "价" in c]
        if price_col:
            pc = price_col[0]
            valid = snapshot_df[pd.to_numeric(snapshot_df[pc], errors="coerce") > 0]
            print(f"    有有效价格: {len(valid)} 只")

    sys.exit(0 if all_pass else 1)


if __name__ == "__main__":
    main()
