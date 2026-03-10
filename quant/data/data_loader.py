"""
数据模块 - 负责读取和准备历史行情数据
"""

from io import StringIO
from pathlib import Path
from typing import Tuple
import warnings

import numpy as np
import pandas as pd

from config import DATA_PATH, DATE_COL, PRICE_COLS, VOLUME_COL


COLUMN_ALIASES = {
    "日期": "date",
    "时间": "date",
    "交易日期": "date",
    "Date": "date",
    "date": "date",
    "datetime": "date",
    "开盘": "open",
    "Open": "open",
    "open": "open",
    "最高": "high",
    "High": "high",
    "high": "high",
    "最低": "low",
    "Low": "low",
    "low": "low",
    "收盘": "close",
    "Close": "close",
    "close": "close",
    "成交量": "volume",
    "成交股数": "volume",
    "Volume": "volume",
    "volume": "volume",
}


class DataLoader:
    """数据加载器"""

    def __init__(self, data_path: str = None):
        self.data_path = Path(data_path or DATA_PATH)
        self.data = None

    def load_data(self) -> pd.DataFrame:
        """从本地文件加载 CSV 数据"""
        if not self.data_path.exists():
            raise FileNotFoundError(f"数据文件不存在: {self.data_path}")

        try:
            return pd.read_csv(self.data_path)
        except Exception as exc:
            raise ValueError(f"读取数据文件失败: {exc}") from exc

    def load_csv_content(self, csv_content: str) -> pd.DataFrame:
        """从上传的 CSV 文本加载数据"""
        if not csv_content or not csv_content.strip():
            raise ValueError("CSV 内容为空")

        try:
            return pd.read_csv(StringIO(csv_content))
        except Exception as exc:
            raise ValueError(f"解析 CSV 内容失败: {exc}") from exc

    def normalize_columns(self, data: pd.DataFrame) -> pd.DataFrame:
        """统一不同数据源的列名"""
        renamed = {}
        for column in data.columns:
            clean_name = str(column).strip()
            renamed[column] = COLUMN_ALIASES.get(clean_name, clean_name)

        normalized = data.rename(columns=renamed)

        if DATE_COL not in normalized.columns and normalized.index.name:
            index_name = str(normalized.index.name).strip()
            if COLUMN_ALIASES.get(index_name) == DATE_COL or index_name.lower() == DATE_COL:
                normalized = normalized.reset_index().rename(columns={index_name: DATE_COL})

        return normalized

    def validate_data(self, data: pd.DataFrame) -> Tuple[bool, str]:
        """验证数据完整性"""
        if data is None or len(data) == 0:
            return False, "数据为空"

        required_cols = [DATE_COL] + PRICE_COLS + [VOLUME_COL]
        missing_cols = [column for column in required_cols if column not in data.columns]
        if missing_cols:
            return False, f"缺少必需列: {missing_cols}"

        if data[DATE_COL].isnull().any():
            return False, "存在无法解析的日期"

        if data[required_cols].isnull().any().any():
            return False, "存在空值或无法转换的价格/成交量数据"

        duplicate_dates = int(data[DATE_COL].duplicated().sum())
        if duplicate_dates > 0:
            warnings.warn(f"⚠️ 数据中存在 {duplicate_dates} 个重复日期")

        price_issues = []
        for price_col in PRICE_COLS:
            if (data[price_col] <= 0).any():
                price_issues.append(f"{price_col} 有非正数值")

        if (data[VOLUME_COL] < 0).any():
            price_issues.append(f"{VOLUME_COL} 有负数值")

        if price_issues:
            return False, f"价格数据问题: {', '.join(price_issues)}"

        return True, "数据验证通过"

    def sort_data(self, data: pd.DataFrame) -> pd.DataFrame:
        """按时间升序排序"""
        if DATE_COL not in data.columns:
            raise ValueError(f"数据中缺少日期列: {DATE_COL}")

        return data.sort_values(by=DATE_COL, ascending=True).reset_index(drop=True)

    def prepare_dataframe(self, data: pd.DataFrame) -> pd.DataFrame:
        """标准化、清洗并验证输入 DataFrame"""
        prepared = self.normalize_columns(data.copy())

        required_cols = [DATE_COL] + PRICE_COLS + [VOLUME_COL]
        missing_cols = [column for column in required_cols if column not in prepared.columns]
        if missing_cols:
            raise ValueError(f"缺少必需列: {missing_cols}，需要至少包含 date/open/high/low/close/volume")

        prepared[DATE_COL] = pd.to_datetime(prepared[DATE_COL], errors="coerce")
        for column in PRICE_COLS + [VOLUME_COL]:
            prepared[column] = pd.to_numeric(prepared[column], errors="coerce")

        before_drop = len(prepared)
        prepared = prepared.dropna(subset=required_cols).copy()
        dropped_rows = before_drop - len(prepared)
        if dropped_rows > 0:
            warnings.warn(f"⚠️ 已移除 {dropped_rows} 行无效数据")

        duplicate_count = int(prepared[DATE_COL].duplicated().sum())
        if duplicate_count > 0:
            warnings.warn(f"⚠️ 已移除 {duplicate_count} 个重复交易日，保留最后一条记录")
            prepared = prepared.drop_duplicates(subset=[DATE_COL], keep="last")

        is_valid, message = self.validate_data(prepared)
        if not is_valid:
            raise ValueError(f"数据验证失败: {message}")

        extra_columns = [column for column in prepared.columns if column not in required_cols]
        prepared = prepared[required_cols + extra_columns]
        prepared = self.sort_data(prepared)
        self.data = prepared
        return prepared

    def get_data_summary(self, data: pd.DataFrame) -> dict:
        """获取数据摘要信息"""
        price_stats = {
            column: {
                "min": float(data[column].min()),
                "max": float(data[column].max()),
                "mean": float(data[column].mean()),
                "std": float(data[column].std()) if pd.notna(data[column].std()) else 0.0,
            }
            for column in PRICE_COLS
        }

        return {
            "total_records": int(len(data)),
            "start_date": data[DATE_COL].min().strftime("%Y-%m-%d"),
            "end_date": data[DATE_COL].max().strftime("%Y-%m-%d"),
            "total_days": int((data[DATE_COL].max() - data[DATE_COL].min()).days),
            "columns": list(data.columns),
            "missing_values": {key: int(value) for key, value in data.isnull().sum().to_dict().items()},
            "price_stats": price_stats,
        }

    def prepare_data(self) -> pd.DataFrame:
        """完整的数据准备流程（读取本地文件）"""
        raw_data = self.load_data()
        prepared = self.prepare_dataframe(raw_data)
        summary = self.get_data_summary(prepared)

        print(f"✅ 数据加载成功: {summary['total_records']} 条记录")
        print(f"📊 日期范围: {summary['start_date']} 到 {summary['end_date']}")
        return prepared

    def prepare_csv_content(self, csv_content: str) -> pd.DataFrame:
        """完整的数据准备流程（处理上传的 CSV 文本）"""
        raw_data = self.load_csv_content(csv_content)
        prepared = self.prepare_dataframe(raw_data)
        summary = self.get_data_summary(prepared)

        print(f"✅ 上传 CSV 解析成功: {summary['total_records']} 条记录")
        return prepared


def generate_sample_data(
    start_date: str = "2023-01-01",
    end_date: str = "2023-12-31",
    start_price: float = 100.0,
    drift: float = 0.0005,
    volatility: float = 0.02,
    seed: int = 42,
) -> pd.DataFrame:
    """生成示例行情数据"""
    dates = pd.date_range(start=start_date, end=end_date, freq="B")
    if len(dates) == 0:
        raise ValueError("示例数据日期范围无有效交易日")

    rng = np.random.default_rng(seed)
    returns = rng.normal(drift, volatility, len(dates))
    close_prices = start_price * np.exp(np.cumsum(returns))

    open_prices = close_prices * (1 + rng.normal(0, 0.008, len(dates)))
    high_prices = np.maximum(open_prices, close_prices) * (1 + np.abs(rng.normal(0.01, 0.01, len(dates))))
    low_prices = np.minimum(open_prices, close_prices) * (1 - np.abs(rng.normal(0.01, 0.01, len(dates))))
    volumes = rng.integers(800000, 5000000, len(dates))

    data = pd.DataFrame(
        {
            DATE_COL: dates,
            "open": open_prices,
            "high": high_prices,
            "low": low_prices,
            "close": close_prices,
            "volume": volumes,
        }
    )

    data["high"] = data[["open", "high", "close"]].max(axis=1)
    data["low"] = data[["open", "low", "close"]].min(axis=1)
    return data


def create_sample_data(output_path: str = "data/stock.csv", **kwargs):
    """创建并保存示例股票数据"""
    data = generate_sample_data(**kwargs)
    output = Path(output_path)
    output.parent.mkdir(parents=True, exist_ok=True)
    data.to_csv(output, index=False)
    print(f"✅ 示例数据已创建: {output} ({len(data)} 条记录)")
    return data


if __name__ == "__main__":
    loader = DataLoader()

    if not loader.data_path.exists():
        print("📝 数据文件不存在，创建示例数据...")
        create_sample_data(str(loader.data_path))

    prepared_data = loader.prepare_data()
    print("\n📋 数据前5行:")
    print(prepared_data.head())
    print("\n📋 数据后5行:")
    print(prepared_data.tail())
