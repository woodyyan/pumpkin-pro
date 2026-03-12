from dataclasses import dataclass
from typing import Any, Dict, Optional

from .models import StrategyDefinition
from .registry import StrategyExecutionAdapter, StrategyRegistry
from .service import StrategyService


@dataclass
class ResolvedStrategy:
    definition: StrategyDefinition
    params: Dict[str, Any]
    adapter: StrategyExecutionAdapter


class StrategyResolver:
    def __init__(self, service: Optional[StrategyService] = None, registry: Optional[StrategyRegistry] = None):
        self.service = service or StrategyService(registry=registry)
        self.registry = registry or self.service.registry

    def resolve(
        self,
        strategy_id: Optional[str] = None,
        strategy_name: Optional[str] = None,
        override_params: Optional[Dict[str, Any]] = None,
    ) -> ResolvedStrategy:
        if strategy_id:
            definition = self.service.get_strategy(strategy_id)
        elif strategy_name:
            definition = self.service.get_strategy_by_name(strategy_name)
        else:
            raise ValueError("回测请求必须提供 strategy_id 或 strategy_name")

        if definition.status != "active":
            raise ValueError(f"策略 {definition.name} 当前不是启用状态，无法用于回测")

        params = self.service.merge_with_defaults(definition, override_params or {})
        adapter = self.registry.get_adapter(definition.implementation_key)
        adapter.validate_params(params)
        return ResolvedStrategy(definition=definition, params=params, adapter=adapter)
