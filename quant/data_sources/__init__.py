"""Unified external data source gateway for Quant.

Business modules should request normalized market data through DataSourceManager
instead of calling Tencent/EastMoney/AkShare directly.
"""

from .manager import DataSourceManager
from .models import (
    Capability,
    DataSourceRequest,
    DataSourceResponse,
    DailyBar,
    Market,
    SourceTrace,
)

__all__ = [
    "Capability",
    "DataSourceManager",
    "DataSourceRequest",
    "DataSourceResponse",
    "DailyBar",
    "Market",
    "SourceTrace",
]
