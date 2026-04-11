"""
港股四象限全量计算与回写脚本

用法：
  # 全量计算 + 本地结果预览（不回调 Go）
  cd quant && python scripts/hk_quadrant_batch.py

  # 全量计算 + 回调 Go 后端
  cd quant && python scripts/hk_quadrant_batch.py --callback http://localhost:8080/api/quadrant/callback

  # 强制全量刷新（忽略缓存）
  cd quant && python scripts/hk_quadrant_batch.py --force

输出：
  - 控制台：象限分布、TOP10 机会/防御股
  - 文件：data/cache/hk_quadrant_results.json（完整评分数据）
"""

import argparse
import json
import os
import sys
import time
from datetime import datetime

# 确保项目根目录在路径中
sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

import pandas as pd


def main():
    parser = argparse.ArgumentParser(description="港股四象限全量批量计算")
    parser.add_argument("--callback", type=str, default=None, help="Go 后端回调 URL")
    parser.add_argument("--force", action="store_true", help="强制全量刷新缓存")
    args = parser.parse_args()

    print("=" * 60)
    print("  港股四象限 — 全量批量计算")
    print(f"  时间: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
    print(f"  模式: {'强制全量' if args.force else '自动(增量/全量)'}")
    if args.callback:
        print(f"  回调: {args.callback}")
    print("=" * 60)

    from screener.quadrant import compute_hk_quadrant_scores

    start = time.time()

    try:
        results = compute_hk_quadrant_scores(
            callback_url=args.callback,
            force_full=args.force,
        )
    except Exception as exc:
        print(f"\n❌ 计算失败: {exc}", file=sys.stderr)
        sys.exit(1)

    elapsed = time.time() - start

    if not results:
        print("\n⚠️ 结果为空，无有效股票")
        sys.exit(0)

    # ── 统计摘要 ──
    df = pd.DataFrame(results)

    print(f"\n✅ 计算完成: {len(results)} 只港股, 耗时 {elapsed:.1f}s")

    # 象限分布
    print(f"\n{'─' * 40}")
    print("  象限分布:")
    print(f"{'─' * 40}")
    q_counts = df["quadrant"].value_counts()
    total = len(df)
    for q in ["机会", "拥挤", "中性", "防御", "泡沫"]:
        count = q_counts.get(q, 0)
        pct = count / total * 100
        bar = "█" * int(pct / 2) + "░" * (50 - int(pct / 2))
        print(f"  {q}: {count:4d} ({pct:5.1f}%) {bar}")

    # 分数统计
    print(f"\n{'─' * 40}")
    print("  分数统计:")
    print(f"{'─' * 40}")
    for col in ["opportunity", "risk"]:
        s = df[col]
        print(f"  {col:12s}: min={s.min():.1f}  max={s.max():.1f}  "
              f"mean={s.mean():.1f}  median={s.median():.1f}")

    # TOP 10 机会区
    opp_top = df[df["quadrant"] == "机会"].nlargest(10, "opportunity")
    print(f"\n{'─' * 40}")
    print("  TOP 10 机会区 (高机会 + 低风险):")
    print(f"{'─' * 40}")
    print(f"  {'排名':>4s}  {'代码':>8s}  {'名称':>12s}  {'机会':>6s}  {'风险':>6s}  {'Trend':>6s}  {'Flow':>6s}  {'Rev':>6s}")
    for i, (_, row) in enumerate(opp_top.iterrows(), 1):
        print(f"  {i:4d}  {row['code']:>8s}  {str(row['name'])[:10]:>12s}  "
              f"{row['opportunity']:6.1f}  {row['risk']:6.1f}  "
              f"{row['trend']:6.1f}  {row['flow']:6.1f}  {row['revision']:6.1f}")

    # TOP 10 防御区（低机会 + 低风险，适合防守配置）
    def_top = df[df["quadrant"] == "防御"].nsmallest(10, "opportunity")
    if not def_top.empty:
        print(f"\n{'─' * 40}")
        print("  TOP 10 防御区 (低机会 + 低风险):")
        print(f"{'─' * 40}")
        print(f"  {'排名':>4s}  {'代码':>8s}  {'名称':>12s}  {'机会':>6s}  {'风险':>6s}  {'Vol':>6s}  {'DD':>6s}  {'Crowd':>6s}")
        for i, (_, row) in enumerate(def_top.iterrows(), 1):
            print(f"  {i:4d}  {row['code']:>8s}  {str(row['name'])[:10]:>12s}  "
                  f"{row['opportunity']:6.1f}  {row['risk']:6.1f}  "
                  f"{row['volatility']:6.1f}  {row['drawdown']:6.1f}  {row['crowding']:6.1f}")

    # ── 保存完整结果到本地 ──
    output_dir = os.path.join(os.path.dirname(__file__), "..", "data", "cache")
    os.makedirs(output_dir, exist_ok=True)
    output_path = os.path.join(output_dir, "hk_quadrant_results.json")

    with open(output_path, "w", encoding="utf-8") as f:
        json.dump({
            "computed_at": datetime.utcnow().isoformat() + "Z",
            "total": len(results),
            "elapsed_seconds": round(elapsed, 1),
            "quadrant_distribution": q_counts.to_dict(),
            "items": results,
        }, f, ensure_ascii=False, indent=2)

    print(f"\n📁 完整结果已保存至: {output_path}")
    print("\n✨ 港股四象限全量计算完成！")


if __name__ == "__main__":
    main()
