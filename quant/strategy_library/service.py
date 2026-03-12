from copy import deepcopy
from datetime import datetime, timezone
from typing import Any, Dict, List, Optional

from .models import StrategyDefinition, StrategyLibraryDocument, StrategyParamDefinition
from .registry import StrategyRegistry
from .repository import StrategyRepository

ALLOWED_STATUS = {"draft", "active", "archived"}
ALLOWED_PARAM_TYPES = {"integer", "number", "string", "boolean"}


class StrategyService:
    def __init__(self, repository: Optional[StrategyRepository] = None, registry: Optional[StrategyRegistry] = None):
        self.repository = repository or StrategyRepository()
        self.registry = registry or StrategyRegistry()

    def list_strategies(self, active_only: bool = False) -> List[StrategyDefinition]:
        strategies = self.repository.list_strategies()
        if active_only:
            strategies = [strategy for strategy in strategies if strategy.status == "active"]
        return strategies

    def list_implementation_keys(self) -> List[str]:
        return self.registry.list_implementation_keys()

    def get_strategy(self, strategy_id: str) -> StrategyDefinition:
        return self.repository.get_strategy(strategy_id)

    def get_strategy_by_name(self, strategy_name: str) -> StrategyDefinition:
        normalized_target = self._normalize_strategy_name(strategy_name)
        for strategy in self.list_strategies():
            aliases = strategy.metadata.get("aliases", []) if isinstance(strategy.metadata, dict) else []
            candidates = [strategy.name, *aliases]
            if any(self._normalize_strategy_name(candidate) == normalized_target for candidate in candidates if candidate):
                return strategy
        raise KeyError(f"未找到策略名称: {strategy_name}")

    def create_strategy(self, strategy: StrategyDefinition) -> StrategyDefinition:
        document = self.repository.load_document()
        prepared = self._prepare_strategy(strategy=strategy, existing=None, siblings=document.items)
        document.items.append(prepared)
        document.updated_at = self._utc_now_iso()
        self.repository.save_document(document)
        return prepared

    def update_strategy(self, strategy_id: str, strategy: StrategyDefinition) -> StrategyDefinition:
        document = self.repository.load_document()
        existing_index = next((index for index, item in enumerate(document.items) if item.id == strategy_id), None)
        if existing_index is None:
            raise KeyError(f"未找到策略: {strategy_id}")

        existing = document.items[existing_index]
        if strategy.id and strategy.id != strategy_id:
            raise ValueError("更新策略时不允许修改策略 ID")
        if strategy.implementation_key and strategy.implementation_key != existing.implementation_key:
            raise ValueError("更新策略时不允许修改策略类型")

        siblings = [item for item in document.items if item.id != strategy_id]
        prepared = self._prepare_strategy(strategy=strategy, existing=existing, siblings=siblings)
        document.items[existing_index] = prepared
        document.updated_at = self._utc_now_iso()
        self.repository.save_document(document)
        return prepared

    def merge_with_defaults(self, strategy: StrategyDefinition, override_params: Optional[Dict[str, Any]] = None) -> Dict[str, Any]:
        merged = dict(strategy.default_params)
        if override_params:
            merged.update(override_params)
        return self._validate_and_normalize_params(strategy, merged)

    def _prepare_strategy(
        self,
        strategy: StrategyDefinition,
        existing: Optional[StrategyDefinition],
        siblings: List[StrategyDefinition],
    ) -> StrategyDefinition:
        strategy_id = self._require_text(strategy.id, "策略 ID")
        key = self._require_text(strategy.key, "策略 key")
        name = self._require_text(strategy.name, "策略名称")
        implementation_key = self._require_text(strategy.implementation_key, "执行映射 key")
        status = strategy.status or "draft"
        if status not in ALLOWED_STATUS:
            raise ValueError(f"不支持的策略状态: {status}")

        self.registry.get_adapter(implementation_key)
        self._ensure_uniqueness(strategy_id, key, siblings)

        param_schema = self._normalize_param_schema(strategy.param_schema)
        prepared = StrategyDefinition(
            id=strategy_id,
            key=key,
            name=name,
            description=(strategy.description or "").strip(),
            category=(strategy.category or "通用").strip(),
            implementation_key=implementation_key,
            status=status,
            version=self._resolve_version(strategy.version, existing),
            created_at=existing.created_at if existing else self._utc_now_iso(),
            updated_at=self._utc_now_iso(),
            param_schema=param_schema,
            default_params={},
            required_indicators=deepcopy(strategy.required_indicators or []),
            chart_overlays=deepcopy(strategy.chart_overlays or []),
            ui_schema=deepcopy(strategy.ui_schema or {}),
            execution_options=deepcopy(strategy.execution_options or {}),
            metadata=deepcopy(strategy.metadata or {}),
        )

        default_params = self._validate_and_normalize_params(prepared, strategy.default_params or {})
        adapter = self.registry.get_adapter(implementation_key)
        adapter.validate_params(default_params)
        prepared.default_params = default_params
        return prepared

    def _normalize_param_schema(self, param_schema: List[StrategyParamDefinition]) -> List[StrategyParamDefinition]:
        seen_keys = set()
        normalized = []
        for item in param_schema or []:
            key = self._require_text(item.key, "参数 key")
            if key in seen_keys:
                raise ValueError(f"参数 key 重复: {key}")
            seen_keys.add(key)

            param_type = (item.type or "number").strip()
            if param_type not in ALLOWED_PARAM_TYPES:
                raise ValueError(f"不支持的参数类型: {param_type}")

            normalized.append(
                StrategyParamDefinition(
                    key=key,
                    label=(item.label or key).strip(),
                    type=param_type,
                    required=item.required,
                    default=item.default,
                    min=item.min,
                    max=item.max,
                    step=item.step,
                    description=(item.description or "").strip(),
                    options=deepcopy(item.options or []),
                )
            )
        return normalized

    def _validate_and_normalize_params(self, strategy: StrategyDefinition, params: Dict[str, Any]) -> Dict[str, Any]:
        schema_map = {item.key: item for item in strategy.param_schema}
        unknown_keys = sorted(set(params.keys()) - set(schema_map.keys()))
        if unknown_keys:
            raise ValueError(f"存在未定义的策略参数: {', '.join(unknown_keys)}")

        normalized: Dict[str, Any] = {}
        for item in strategy.param_schema:
            raw_value = params.get(item.key, item.default)
            if raw_value is None:
                if item.required:
                    raise ValueError(f"缺少必填参数: {item.label}")
                continue

            value = self._coerce_param_value(item, raw_value)
            if item.type in {"integer", "number"}:
                if item.min is not None and value < item.min:
                    raise ValueError(f"参数 {item.label} 不能小于 {item.min}")
                if item.max is not None and value > item.max:
                    raise ValueError(f"参数 {item.label} 不能大于 {item.max}")
            normalized[item.key] = value

        return normalized

    def _coerce_param_value(self, item: StrategyParamDefinition, value: Any):
        try:
            if item.type == "integer":
                if isinstance(value, bool):
                    raise ValueError
                if isinstance(value, float) and not value.is_integer():
                    raise ValueError
                return int(value)
            if item.type == "number":
                if isinstance(value, bool):
                    raise ValueError
                return float(value)
            if item.type == "boolean":
                if isinstance(value, bool):
                    return value
                if isinstance(value, str):
                    lowered = value.strip().lower()
                    if lowered in {"true", "1", "yes", "on"}:
                        return True
                    if lowered in {"false", "0", "no", "off"}:
                        return False
                raise ValueError
            return str(value)
        except (TypeError, ValueError) as exc:
            raise ValueError(f"参数 {item.label} 的值格式不正确") from exc

    def _ensure_uniqueness(self, strategy_id: str, key: str, siblings: List[StrategyDefinition]) -> None:
        if any(item.id == strategy_id for item in siblings):
            raise ValueError(f"策略 ID 已存在: {strategy_id}")
        if any(item.key == key for item in siblings):
            raise ValueError(f"策略 key 已存在: {key}")

    def _resolve_version(self, version: int, existing: Optional[StrategyDefinition]) -> int:
        requested = max(int(version or 1), 1)
        if existing is None:
            return requested
        return max(requested, existing.version + 1)

    @staticmethod
    def _utc_now_iso() -> str:
        return datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")

    @staticmethod
    def _require_text(value: str, field_name: str) -> str:
        normalized = (value or "").strip()
        if not normalized:
            raise ValueError(f"{field_name}不能为空")
        return normalized

    @staticmethod
    def _normalize_strategy_name(name: str) -> str:
        return (name or "").strip().replace("（", "(").replace("）", ")").replace(" ", "").lower()
