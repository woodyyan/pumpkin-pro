import json
from pathlib import Path
from typing import List, Optional

from .models import StrategyDefinition, StrategyLibraryDocument


def _model_to_dict(model):
    if hasattr(model, "model_dump"):
        return model.model_dump()
    return model.dict()


class StrategyRepository:
    def __init__(self, file_path: Optional[str] = None):
        default_path = Path(__file__).resolve().parent / "strategies.json"
        self.file_path = Path(file_path) if file_path else default_path

    def load_document(self) -> StrategyLibraryDocument:
        if not self.file_path.exists():
            raise FileNotFoundError(f"策略库文件不存在: {self.file_path}")

        with self.file_path.open("r", encoding="utf-8") as file:
            payload = json.load(file)
        return StrategyLibraryDocument(**payload)

    def save_document(self, document: StrategyLibraryDocument) -> StrategyLibraryDocument:
        self.file_path.parent.mkdir(parents=True, exist_ok=True)
        with self.file_path.open("w", encoding="utf-8") as file:
            json.dump(_model_to_dict(document), file, ensure_ascii=False, indent=2)
        return document

    def list_strategies(self) -> List[StrategyDefinition]:
        return self.load_document().items

    def get_strategy(self, strategy_id: str) -> StrategyDefinition:
        for strategy in self.list_strategies():
            if strategy.id == strategy_id:
                return strategy
        raise KeyError(f"未找到策略: {strategy_id}")
