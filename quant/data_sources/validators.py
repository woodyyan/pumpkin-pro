from __future__ import annotations

from typing import Iterable, List

from .errors import EmptyResponseError, TradeDateMismatchError, ValidationError
from .models import DailyBar


def validate_daily_bars(
    bars: Iterable[DailyBar],
    *,
    target_trade_date: str = "",
    require_exact_trade_date: bool = False,
) -> List[DailyBar]:
    result = sorted(list(bars), key=lambda item: item.trade_date)
    if not result:
        raise EmptyResponseError("日线返回空数据")

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
            raise ValidationError(f"日线字段无效 {bar.symbol} {bar.trade_date}: {','.join(missing)}")

    if require_exact_trade_date and target_trade_date:
        if target_trade_date not in {item.trade_date for item in result}:
            latest = result[-1].trade_date if result else ""
            raise TradeDateMismatchError(
                f"目标交易日 {target_trade_date} 不在返回日线中，最新返回 {latest}；禁止用其他日期价格兜底"
            )
    return result
