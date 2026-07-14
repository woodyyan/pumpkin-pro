"""
A 股股票池预过滤模块 — 在四象限计算发起数据源请求之前，
先剔除 ST/退市、停牌代理、北交所股票。

设计原则：
1. 纯函数，不发起任何网络请求。
2. 只读取已拉取到的全市场快照本地字段（code/name/volume）。
3. 与 baostock 可用性完全解耦。
4. 保守原则：字段缺失时默认不过滤（宁可漏过滤，不可误伤正常股票）。
"""

from __future__ import annotations

import logging
from dataclasses import dataclass, field
from typing import Any, Dict, List, Optional, Tuple

logger = logging.getLogger(__name__)

# 过滤比例合理性熔断阈值：A 股 ST+北交所+日常停牌历史占比约 5%~10%，
# 超过此阈值说明上游数据可能异常，记录 ERROR 但不阻断流程。
MAX_FILTER_RATIO = 0.15


@dataclass(frozen=True)
class FilterOptions:
    """过滤开关，默认全部开启，可按场景单独关闭某一维度。"""

    exclude_bse: bool = True
    exclude_st: bool = True
    exclude_suspended: bool = True


@dataclass
class FilterStats:
    """过滤统计，用于进度上报和 Admin 监控。"""

    total_before: int = 0
    total_after: int = 0
    excluded_bse: int = 0
    excluded_st: int = 0
    excluded_suspended: int = 0
    # 同时命中多个条件的股票数（避免三个计数相加超过实际排除总数）
    excluded_overlap: int = 0
    filter_ratio: float = 0.0
    ratio_alert: bool = False

    def to_dict(self) -> Dict[str, Any]:
        return {
            "total_before": self.total_before,
            "total_after": self.total_after,
            "excluded_bse": self.excluded_bse,
            "excluded_st": self.excluded_st,
            "excluded_suspended": self.excluded_suspended,
            "excluded_overlap": self.excluded_overlap,
            "filter_ratio": round(self.filter_ratio, 4),
            "ratio_alert": self.ratio_alert,
        }


def is_bse_code(code: str) -> bool:
    """
    判断是否为北交所股票代码。

    规则与 Factor Lab classify_board() 的 BJ 分支保持一致：
    code.startswith(("8", "4", "920"))

    涵盖：
    - 8 开头（83x/87x/88x 等，北交所存量+新股）
    - 4 开头（430 系列，老三板/北交所遗留）
    - 920 开头（920 系列，北交所新代码段）
    """
    code = str(code or "").strip()
    if not code:
        return False
    return code.startswith(("8", "4", "920"))


def is_st_name(name: str) -> bool:
    """
    判断股票名称是否为 ST/*ST/SST/S*ST 或退市整理期股票。

    规则："ST" in name.upper().replace(" ", "") or "退" in name

    子串匹配天然覆盖 *ST / SST / S*ST 等变体；
    额外排除名称含"退"字的退市整理期股票（流动性更差）。
    """
    if name is None:
        return False
    name = str(name).strip()
    if not name:
        return False
    normalized = name.upper().replace(" ", "")
    return "ST" in normalized or "退" in name


def is_suspended_row(volume: Optional[float]) -> bool:
    """
    用当天成交量代理判断是否停牌。

    四象限在收盘后运行，正常交易股票 volume 必然 > 0；
    停牌股票全天无成交，volume 为 0 或 None。

    保守原则：volume 为 None 时返回 False（不过滤），
    避免上游字段缺失导致正常股票被误伤。
    """
    if volume is None:
        return False
    try:
        return float(volume) <= 0
    except (TypeError, ValueError):
        return False


def filter_a_share_universe(
    snapshot_df: Any,
    options: FilterOptions = FilterOptions(),
) -> Tuple[Any, FilterStats]:
    """
    对全市场快照执行预过滤，返回过滤后的 DataFrame 和统计信息。

    Args:
        snapshot_df: 全市场快照 DataFrame，需含 code/name/volume 列。
        options: 过滤开关，默认全部开启。

    Returns:
        (filtered_df, FilterStats)
    """
    stats = FilterStats()

    if snapshot_df is None or (hasattr(snapshot_df, "empty") and snapshot_df.empty):
        return snapshot_df, stats

    stats.total_before = len(snapshot_df)

    df = snapshot_df.copy()

    # 确保所需列存在
    has_code = "code" in df.columns
    has_name = "name" in df.columns
    has_volume = "volume" in df.columns

    if not has_code:
        logger.warning("[universe_filter] 快照缺少 code 列，跳过过滤")
        stats.total_after = len(df)
        return df, stats

    # 逐行判断三个独立条件（任一命中即排除）
    bse_flags: List[bool] = []
    st_flags: List[bool] = []
    suspended_flags: List[bool] = []
    name_missing = False
    volume_missing = False

    for _, row in df.iterrows():
        code = str(row.get("code", "") or "").strip()

        # 北交所判断（基于 code，不受 name/volume 缺失影响）
        if options.exclude_bse:
            bse_flags.append(is_bse_code(code))
        else:
            bse_flags.append(False)

        # ST 判断（基于 name）
        if options.exclude_st and has_name:
            name = row.get("name")
            if name is None or (isinstance(name, float) and name != name):  # NaN check
                st_flags.append(False)
                name_missing = True
            else:
                st_flags.append(is_st_name(str(name)))
        else:
            st_flags.append(False)

        # 停牌判断（基于 volume）
        if options.exclude_suspended and has_volume:
            vol = row.get("volume")
            if vol is None or (isinstance(vol, float) and vol != vol):  # NaN check
                suspended_flags.append(False)
                volume_missing = True
            else:
                suspended_flags.append(is_suspended_row(vol))
        else:
            suspended_flags.append(False)

    if name_missing:
        logger.warning(
            "[universe_filter] 部分股票 name 字段缺失/NaN，ST 过滤对这些股票降级为不过滤"
        )
    if volume_missing:
        logger.warning(
            "[universe_filter] 部分股票 volume 字段缺失/NaN，停牌过滤对这些股票降级为不过滤"
        )

    # 合并：任一条件命中即排除
    exclude_mask = [
        b or s or sus
        for b, s, sus in zip(bse_flags, st_flags, suspended_flags)
    ]

    # 统计各维度命中数和重叠数
    bse_count = sum(bse_flags)
    st_count = sum(st_flags)
    suspended_count = sum(suspended_flags)
    total_excluded = sum(exclude_mask)
    overlap = bse_count + st_count + suspended_count - total_excluded

    stats.excluded_bse = bse_count
    stats.excluded_st = st_count
    stats.excluded_suspended = suspended_count
    stats.excluded_overlap = max(0, overlap)

    # 应用过滤
    filtered_df = df[[not e for e in exclude_mask]].copy()
    stats.total_after = len(filtered_df)
    stats.filter_ratio = (
        (stats.total_before - stats.total_after) / stats.total_before
        if stats.total_before > 0
        else 0.0
    )
    stats.ratio_alert = stats.filter_ratio > MAX_FILTER_RATIO

    if stats.ratio_alert:
        logger.error(
            "[universe_filter] 过滤比例 %.1f%% 超过阈值 %.0f%%（ST=%d, 停牌=%d, 北交所=%d, 重叠=%d），"
            "可能为上游数据异常，仍按过滤结果继续计算",
            stats.filter_ratio * 100,
            MAX_FILTER_RATIO * 100,
            stats.excluded_st,
            stats.excluded_suspended,
            stats.excluded_bse,
            stats.excluded_overlap,
        )
    else:
        logger.info(
            "[universe_filter] 过滤完成: %d → %d（排除 ST=%d, 停牌=%d, 北交所=%d, 重叠=%d, 比例=%.1f%%）",
            stats.total_before,
            stats.total_after,
            stats.excluded_st,
            stats.excluded_suspended,
            stats.excluded_bse,
            stats.excluded_overlap,
            stats.filter_ratio * 100,
        )

    return filtered_df, stats
