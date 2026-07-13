from __future__ import annotations

import logging
from typing import Iterable, List

from .errors import EmptyResponseError, TradeDateMismatchError, ValidationError
from .models import DailyBar

logger = logging.getLogger(__name__)


def validate_daily_bars(
    bars: Iterable[DailyBar],
    *,
    target_trade_date: str = "",
    require_exact_trade_date: bool = False,
) -> List[DailyBar]:
    result = sorted(list(bars), key=lambda item: item.trade_date)
    if not result:
        raise EmptyResponseError("日线返回空数据")

    # 单根 bar 级容错：丢弃脏 bar（open/close/high/low<=0 或 high<low 或缺日期），
    # 而非整只股票 reject，恢复旧直连逻辑的韧性。若全部脏数据则仍硬失败。
    cleaned: List[DailyBar] = []
    dropped: List[str] = []
    for bar in result:
        missing = []
        if not bar.trade_date:
            missing.append("trade_date")
        for field in ("open", "close", "high", "low"):
            if getattr(bar, field) <= 0:
                missing.append(field)
        if bar.high < bar.low:
            missing.append("high_low")
        if missing:
            dropped.append(f"{bar.symbol}/{bar.trade_date}:{','.join(missing)}")
            continue
        cleaned.append(bar)

    if not cleaned:
        raise ValidationError(f"日线全部字段无效，已丢弃所有 bar（样例: {dropped[:3]}）")

    if dropped:
        logger.warning(
            "[validate] 丢弃 %d 根无效日线（保留 %d 根）: %s",
            len(dropped), len(cleaned), dropped[:5],
        )

    if require_exact_trade_date and target_trade_date:
        if target_trade_date not in {item.trade_date for item in cleaned}:
            latest = cleaned[-1].trade_date if cleaned else ""
            raise TradeDateMismatchError(
                f"目标交易日 {target_trade_date} 不在返回日线中，最新返回 {latest}；禁止用其他日期价格兜底"
            )
    return cleaned
