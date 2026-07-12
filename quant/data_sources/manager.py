from __future__ import annotations

import time
from typing import Dict, List, Optional

from .errors import DataSourceError, UnsupportedCapabilityError
from .health import GLOBAL_HEALTH, DataSourceHealth
from .models import DataSourceRequest, DataSourceResponse, DailyBar, SourceTrace
from .policy import SourcePolicy, get_policy
from .providers import AkShareProvider, EastMoneyProvider, TencentProvider
from .registry import SourceRegistry
from .validators import validate_daily_bars


class DataSourceManager:
    def __init__(self, providers: Optional[Dict[str, object]] = None, registry: Optional[SourceRegistry] = None, health: Optional[DataSourceHealth] = None):
        self.providers = providers or {
            "tencent": TencentProvider(),
            "eastmoney": EastMoneyProvider(),
            "akshare": AkShareProvider(),
        }
        self.registry = registry or SourceRegistry()
        self.health = health or GLOBAL_HEALTH

    def fetch(self, request: DataSourceRequest) -> DataSourceResponse:
        policy = get_policy(request.capability, request.market)
        traces: List[SourceTrace] = []
        errors: List[str] = []
        for provider_name in policy.providers:
            if not self.registry.supports(provider_name, request.market, request.capability):
                traces.append(SourceTrace(
                    provider=provider_name,
                    capability=request.capability,
                    market=request.market,
                    status="skipped",
                    reason="provider does not support capability/market",
                ))
                continue
            provider = self.providers.get(provider_name)
            if provider is None:
                traces.append(SourceTrace(
                    provider=provider_name,
                    capability=request.capability,
                    market=request.market,
                    status="skipped",
                    reason="provider not registered",
                ))
                continue
            start = time.perf_counter()
            try:
                rows = provider.fetch(request)  # type: ignore[attr-defined]
                rows = self._validate(rows, request, policy)
                duration = (time.perf_counter() - start) * 1000
                trace = SourceTrace(
                    provider=provider_name,
                    capability=request.capability,
                    market=request.market,
                    status="success",
                    duration_ms=duration,
                    records_count=len(rows),
                    trade_date=rows[-1].trade_date if rows else "",
                )
                traces.append(trace)
                response = DataSourceResponse(
                    ok=True,
                    capability=request.capability,
                    market=request.market,
                    symbol=request.symbol,
                    data=rows,
                    used_sources=[provider_name],
                    trace=traces,
                    source_trade_date=request.target_trade_date or (rows[-1].trade_date if rows else ""),
                )
                self.health.record(traces)
                return response
            except Exception as exc:  # noqa: BLE001 - provider isolation is the fallback boundary
                duration = (time.perf_counter() - start) * 1000
                message = f"{provider_name}: {exc}"
                errors.append(message)
                traces.append(SourceTrace(
                    provider=provider_name,
                    capability=request.capability,
                    market=request.market,
                    status="failed",
                    reason=str(exc),
                    duration_ms=duration,
                ))

        self.health.record(traces)
        if not policy.allow_partial and not request.allow_partial:
            raise DataSourceError("所有数据源均失败: " + " | ".join(errors))
        return DataSourceResponse(
            ok=False,
            capability=request.capability,
            market=request.market,
            symbol=request.symbol,
            trace=traces,
            errors=errors,
            partial=request.allow_partial,
        )

    def fetch_daily_bars(self, *, symbol: str, market: str, start_date: str = "", end_date: str = "", target_trade_date: str = "", lookback_days: int = 120, adjust: str = "qfq") -> DataSourceResponse:
        from .models import Capability

        return self.fetch(DataSourceRequest(
            capability=Capability.DAILY_BARS,
            market=market,
            symbol=symbol,
            start_date=start_date,
            end_date=end_date,
            target_trade_date=target_trade_date,
            lookback_days=lookback_days,
            adjust=adjust,
            require_exact_trade_date=bool(target_trade_date),
        ))

    def fetch_index_bars(self, *, symbol: str, market: str, start_date: str = "", end_date: str = "", target_trade_date: str = "", lookback_days: int = 120, adjust: str = "qfq") -> DataSourceResponse:
        from .models import Capability

        return self.fetch(DataSourceRequest(
            capability=Capability.INDEX_BARS,
            market=market,
            symbol=symbol,
            start_date=start_date,
            end_date=end_date,
            target_trade_date=target_trade_date,
            lookback_days=lookback_days,
            adjust=adjust,
            require_exact_trade_date=bool(target_trade_date),
        ))

    @staticmethod
    def _validate(rows: List[DailyBar], request: DataSourceRequest, policy: SourcePolicy) -> List[DailyBar]:
        if request.capability not in {"daily_bars", "index_bars"}:
            raise UnsupportedCapabilityError(f"unsupported capability: {request.capability}")
        return validate_daily_bars(
            rows,
            target_trade_date=request.target_trade_date,
            require_exact_trade_date=policy.require_exact_trade_date or request.require_exact_trade_date,
        )
