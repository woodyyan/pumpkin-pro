from typing import Any, Dict, List, Optional

from pydantic import BaseModel, Field


class StrategyParamDefinition(BaseModel):
    key: str
    label: str
    type: str = Field(default="number", description="integer/number/string/boolean")
    required: bool = True
    default: Optional[Any] = None
    min: Optional[float] = None
    max: Optional[float] = None
    step: Optional[float] = None
    description: str = ""
    options: List[Dict[str, Any]] = Field(default_factory=list)


class StrategyDefinition(BaseModel):
    id: str
    key: str
    name: str
    description: str = ""
    category: str = "通用"
    implementation_key: str
    status: str = "draft"
    version: int = 1
    created_at: str
    updated_at: str
    param_schema: List[StrategyParamDefinition] = Field(default_factory=list)
    default_params: Dict[str, Any] = Field(default_factory=dict)
    required_indicators: List[Dict[str, Any]] = Field(default_factory=list)
    chart_overlays: List[Dict[str, Any]] = Field(default_factory=list)
    ui_schema: Dict[str, Any] = Field(default_factory=dict)
    execution_options: Dict[str, Any] = Field(default_factory=dict)
    metadata: Dict[str, Any] = Field(default_factory=dict)


class StrategyLibraryDocument(BaseModel):
    version: int = 1
    updated_at: str
    items: List[StrategyDefinition] = Field(default_factory=list)
