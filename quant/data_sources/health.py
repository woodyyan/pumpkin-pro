from __future__ import annotations

from collections import deque
from threading import Lock
from typing import Any, Deque, Dict, List

from .models import SourceTrace


class DataSourceHealth:
    def __init__(self, max_events: int = 200):
        self._events: Deque[SourceTrace] = deque(maxlen=max_events)
        self._lock = Lock()

    def record(self, traces: List[SourceTrace]) -> None:
        with self._lock:
            self._events.extend(traces)

    def snapshot(self) -> Dict[str, Any]:
        with self._lock:
            events = list(self._events)
        capabilities: Dict[str, Dict[str, Any]] = {}
        providers: Dict[str, Dict[str, Any]] = {}
        for event in events:
            entry = providers.setdefault(event.provider, {"success": 0, "failed": 0, "skipped": 0})
            entry[event.status] = entry.get(event.status, 0) + 1
            entry["last_status"] = event.status
            entry["last_reason"] = event.reason
            cap = capabilities.setdefault(event.capability, {"success": 0, "failed": 0, "skipped": 0})
            cap[event.status] = cap.get(event.status, 0) + 1
            cap["last_provider"] = event.provider
            cap["last_status"] = event.status
            cap["last_market"] = event.market
        return {
            "providers": providers,
            "capabilities": capabilities,
            "total_events": len(events),
            "recent": [event.__dict__ for event in events[-20:]],
        }


GLOBAL_HEALTH = DataSourceHealth()
