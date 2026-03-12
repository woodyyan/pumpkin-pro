import logging
import os
from datetime import datetime
from typing import Any, Dict, List, Optional, Tuple

import numpy as np
import pandas as pd
import simplejson as json
import uvicorn
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel, Field

from data.data_loader import DataLoader, generate_sample_data
from data.scripts.akshare_loader import fetch_stock_data, resolve_stock_name_with_debug
from engine.backtest_engine import BacktestEngine
from result.metrics import PerformanceMetrics
from strategy_library.models import StrategyDefinition, StrategyParamDefinition
from strategy_library.registry import StrategyRegistry
from strategy_library.resolver import ResolvedStrategy, StrategyResolver
from strategy_library.service import StrategyService

logger = logging.getLogger(__name__)

app = FastAPI(title="Pumpkin Quant Service", description="Quantitative Backtesting Engine API")

SUPPORTED_DATA_SOURCES = ["online", "csv", "sample"]

strategy_registry = StrategyRegistry()
strategy_service = StrategyService(registry=strategy_registry)
strategy_resolver = StrategyResolver(service=strategy_service, registry=strategy_registry)


class SampleDataConfig(BaseModel):
    start_price: float = Field(default=100.0, gt=0)
    drift: float = Field(default=0.0005, ge=-0.2, le=0.2)
    volatility: float = Field(default=0.02, gt=0, le=0.5)
    seed: int = Field(default=42, ge=0)


class StrategyUpsertRequest(BaseModel):
    id: str
    key: str
    name: str
    description: str = ""
    category: str = "通用"
    implementation_key: str
    status: str = "draft"
    version: int = 1
    param_schema: List[StrategyParamDefinition] = Field(default_factory=list)
    default_params: Dict[str, Any] = Field(default_factory=dict)
    required_indicators: List[Dict[str, Any]] = Field(default_factory=list)
    chart_overlays: List[Dict[str, Any]] = Field(default_factory=list)
    ui_schema: Dict[str, Any] = Field(default_factory=dict)
    execution_options: Dict[str, Any] = Field(default_factory=dict)
    metadata: Dict[str, Any] = Field(default_factory=dict)


class BacktestRequest(BaseModel):
    data_source: str = Field(default="online", description="online/csv/sample")
    ticker: Optional[str] = Field(default=None, description="A股六位数字或港股五位数字")
    start_date: str = Field(..., description="YYYY-MM-DD")
    end_date: str = Field(..., description="YYYY-MM-DD")
    capital: float = Field(default=100000.0, gt=0)
    fee_pct: float = Field(default=0.001, ge=0, le=0.05)
    strategy_id: Optional[str] = Field(default=None, description="策略库中的策略 ID")
    strategy_name: Optional[str] = Field(default=None, description="兼容旧版请求的策略名称")
    strategy_params: Dict[str, Any] = Field(default_factory=dict)
    csv_content: Optional[str] = Field(default=None, description="上传的本地 CSV 文本")
    csv_filename: Optional[str] = Field(default=None)
    sample_config: SampleDataConfig = Field(default_factory=SampleDataConfig)


@app.get("/api/health")
def health_check():
    return {
        "status": "online",
        "service": "Pumpkin Quant Engine",
        "strategies": [strategy.name for strategy in strategy_service.list_strategies(active_only=True)],
        "data_sources": SUPPORTED_DATA_SOURCES,
    }


@app.get("/api/backtest/options")
def get_backtest_options():
    return {
        "strategies": [build_strategy_summary(strategy) for strategy in strategy_service.list_strategies(active_only=True)],
        "data_sources": SUPPORTED_DATA_SOURCES,
    }


@app.get("/api/strategies/active")
def get_active_strategies():
    return {
        "items": [strategy_to_dict(strategy) for strategy in strategy_service.list_strategies(active_only=True)]
    }


@app.get("/api/strategies")
def list_strategies():
    return {
        "items": [build_strategy_summary(strategy) for strategy in strategy_service.list_strategies()],
        "implementation_keys": strategy_service.list_implementation_keys(),
    }


@app.get("/api/strategies/{strategy_id}/definition")
def get_strategy_definition(strategy_id: str):
    try:
        strategy = strategy_service.get_strategy(strategy_id)
        return {"item": strategy_to_dict(strategy)}
    except KeyError as exc:
        raise HTTPException(status_code=404, detail=str(exc)) from exc


@app.get("/api/strategies/{strategy_id}")
def get_strategy_detail(strategy_id: str):
    try:
        strategy = strategy_service.get_strategy(strategy_id)
        return {
            "item": strategy_to_dict(strategy),
            "implementation_keys": strategy_service.list_implementation_keys(),
        }
    except KeyError as exc:
        raise HTTPException(status_code=404, detail=str(exc)) from exc


@app.post("/api/strategies")
def create_strategy(req: StrategyUpsertRequest):
    try:
        created = strategy_service.create_strategy(build_strategy_definition(req))
        return {"item": strategy_to_dict(created)}
    except ValueError as exc:
        raise HTTPException(status_code=400, detail=str(exc)) from exc


@app.put("/api/strategies/{strategy_id}")
def update_strategy(strategy_id: str, req: StrategyUpsertRequest):
    try:
        updated = strategy_service.update_strategy(strategy_id, build_strategy_definition(req, strategy_id=strategy_id))
        return {"item": strategy_to_dict(updated)}
    except KeyError as exc:
        raise HTTPException(status_code=404, detail=str(exc)) from exc
    except ValueError as exc:
        raise HTTPException(status_code=400, detail=str(exc)) from exc


@app.post("/api/backtest")
def run_backtest_api(req: BacktestRequest):
    try:
        start_dt, end_dt = parse_date_range(req.start_date, req.end_date)
        resolved_strategy = strategy_resolver.resolve(
            strategy_id=req.strategy_id,
            strategy_name=req.strategy_name,
            override_params=req.strategy_params,
        )

        raw_data, source_name, stock_name, stock_name_debug = load_market_data(req, start_dt, end_dt)
        if raw_data.empty:
            raise HTTPException(status_code=400, detail="可用行情数据为空，无法回测")

        results_df, trades_df, enriched_df = execute_backtest(raw_data, resolved_strategy, req.capital, req.fee_pct)
        metrics = calculate_metrics(results_df, trades_df, req.capital)
        signal_summary = build_signal_summary(enriched_df)

        response = {
            "status": "success",
            "source_used": source_name,
            "data_source": req.data_source,
            "data_summary": build_data_summary(raw_data, req, source_name, stock_name, stock_name_debug),
            "strategy": build_runtime_strategy_payload(resolved_strategy),
            "metrics": metrics,
            "signal_summary": signal_summary,
            "trades": df_to_json_safe(trades_df),
            "kline_data": df_to_json_safe(build_visual_dataset(results_df, resolved_strategy)),
            "analysis": {
                "equity_curve": df_to_json_safe(build_equity_curve(results_df)),
                "drawdown_curve": df_to_json_safe(build_drawdown_curve(results_df)),
                "monthly_returns": monthly_returns_to_json(build_monthly_returns(results_df)),
            },
        }
        return response
    except HTTPException:
        raise
    except KeyError as exc:
        raise HTTPException(status_code=404, detail=str(exc)) from exc
    except ValueError as exc:
        raise HTTPException(status_code=400, detail=str(exc)) from exc
    except Exception as exc:
        import traceback

        traceback.print_exc()
        raise HTTPException(status_code=500, detail=str(exc)) from exc


def build_strategy_definition(req: StrategyUpsertRequest, strategy_id: Optional[str] = None) -> StrategyDefinition:
    return StrategyDefinition(
        id=strategy_id or req.id,
        key=req.key,
        name=req.name,
        description=req.description,
        category=req.category,
        implementation_key=req.implementation_key,
        status=req.status,
        version=req.version,
        created_at="",
        updated_at="",
        param_schema=req.param_schema,
        default_params=req.default_params,
        required_indicators=req.required_indicators,
        chart_overlays=req.chart_overlays,
        ui_schema=req.ui_schema,
        execution_options=req.execution_options,
        metadata=req.metadata,
    )


def build_strategy_summary(strategy: StrategyDefinition) -> Dict[str, Any]:
    description = strategy.description or ""
    return {
        "id": strategy.id,
        "key": strategy.key,
        "name": strategy.name,
        "category": strategy.category,
        "status": strategy.status,
        "description": description,
        "description_summary": description[:72],
        "implementation_key": strategy.implementation_key,
        "version": strategy.version,
        "updated_at": strategy.updated_at,
    }


def build_runtime_strategy_payload(resolved_strategy: ResolvedStrategy) -> Dict[str, Any]:
    strategy = resolved_strategy.definition
    return {
        "id": strategy.id,
        "key": strategy.key,
        "name": strategy.name,
        "implementation_key": strategy.implementation_key,
        "params": resolved_strategy.params,
        "chart_overlays": resolved_strategy.adapter.get_overlay_columns(resolved_strategy.params),
    }


def strategy_to_dict(strategy: StrategyDefinition) -> Dict[str, Any]:
    if hasattr(strategy, "model_dump"):
        return strategy.model_dump()
    return strategy.dict()


def parse_date_range(start_date: str, end_date: str) -> Tuple[datetime, datetime]:
    try:
        start_dt = datetime.strptime(start_date, "%Y-%m-%d")
        end_dt = datetime.strptime(end_date, "%Y-%m-%d")
    except ValueError as exc:
        raise ValueError("日期格式必须为 YYYY-MM-DD") from exc

    if start_dt > end_dt:
        raise ValueError("开始日期不能晚于结束日期")
    return start_dt, end_dt


def model_to_dict(model) -> Dict:
    if hasattr(model, "model_dump"):
        return model.model_dump()
    return model.dict()


def load_market_data(
    req: BacktestRequest, start_dt: datetime, end_dt: datetime
) -> Tuple[pd.DataFrame, str, Optional[str], Optional[Dict[str, Any]]]:
    loader = DataLoader()

    if req.data_source not in SUPPORTED_DATA_SOURCES:
        raise ValueError(f"不支持的数据源: {req.data_source}")

    if req.data_source == "online":
        if not req.ticker:
            raise ValueError("在线下载模式必须填写股票代码")
        data, source_name = fetch_stock_data(req.ticker, start_dt, end_dt)
        stock_name, stock_name_debug = resolve_stock_name_with_debug(req.ticker)
        logger.info(
            "股票名称识别结果 ticker=%s status=%s source=%s message=%s",
            req.ticker,
            stock_name_debug.get("status") if stock_name_debug else "unknown",
            stock_name_debug.get("source") if stock_name_debug else None,
            stock_name_debug.get("message") if stock_name_debug else None,
        )
        return loader.prepare_dataframe(data), source_name, stock_name, stock_name_debug

    if req.data_source == "csv":
        if not req.csv_content:
            raise ValueError("本地 CSV 模式必须上传 CSV 文件内容")
        data = loader.prepare_csv_content(req.csv_content)
        filtered = filter_date_range(data, start_dt, end_dt)
        filename = req.csv_filename or "uploaded.csv"
        return filtered, f"本地CSV ({filename})", None, {
            "status": "skipped",
            "message": "当前为本地 CSV 模式，未执行股票名称识别",
            "errors": [],
        }

    sample_params = model_to_dict(req.sample_config)
    generated = generate_sample_data(
        start_date=req.start_date,
        end_date=req.end_date,
        start_price=sample_params["start_price"],
        drift=sample_params["drift"],
        volatility=sample_params["volatility"],
        seed=sample_params["seed"],
    )
    prepared = loader.prepare_dataframe(generated)
    return prepared, "系统示例行情", None, {
        "status": "skipped",
        "message": "当前为系统示例行情，未执行股票名称识别",
        "errors": [],
    }


def filter_date_range(data: pd.DataFrame, start_dt: datetime, end_dt: datetime) -> pd.DataFrame:
    filtered = data[(data["date"] >= pd.Timestamp(start_dt)) & (data["date"] <= pd.Timestamp(end_dt))].copy()
    if filtered.empty:
        raise ValueError("指定日期区间内没有可用行情数据")
    return filtered.reset_index(drop=True)


def execute_backtest(
    market_data: pd.DataFrame,
    resolved_strategy: ResolvedStrategy,
    capital: float,
    fee_pct: float,
) -> Tuple[pd.DataFrame, pd.DataFrame, pd.DataFrame]:
    enriched_data = resolved_strategy.adapter.attach_indicators(market_data, resolved_strategy.params)
    strategy = resolved_strategy.adapter.build_strategy(enriched_data, resolved_strategy.params)
    data_with_signals = strategy.generate_signals()

    engine = BacktestEngine(data_with_signals, capital, fee_pct)
    results_df = engine.run_backtest()
    trades_df = engine.get_trade_log()
    return results_df, trades_df, data_with_signals


def calculate_metrics(results_df: pd.DataFrame, trades_df: pd.DataFrame, capital: float) -> Dict:
    metrics_calc = PerformanceMetrics(
        portfolio_values=results_df["portfolio_value"].tolist(),
        daily_returns=results_df["daily_return"].tolist(),
        trades=trades_df.to_dict("records"),
        initial_capital=capital,
    )
    return dict_to_json_safe(metrics_calc.calculate_all_metrics())


def build_data_summary(
    data: pd.DataFrame,
    req: BacktestRequest,
    source_name: str,
    stock_name: Optional[str],
    stock_name_debug: Optional[Dict[str, Any]],
) -> Dict:
    loader = DataLoader()
    summary = loader.get_data_summary(data)
    ticker_display = build_ticker_display(req.data_source, req.ticker, stock_name, req.csv_filename)
    return {
        "source_used": source_name,
        "ticker": req.ticker,
        "ticker_name": stock_name,
        "ticker_name_debug": stock_name_debug,
        "ticker_display": ticker_display,
        "total_records": summary["total_records"],
        "start_date": summary["start_date"],
        "end_date": summary["end_date"],
        "total_days": summary["total_days"],
        "price_stats": summary["price_stats"],
    }


def build_ticker_display(data_source: str, ticker: Optional[str], stock_name: Optional[str], csv_filename: Optional[str]) -> str:
    if data_source == "sample":
        return "系统示例行情"
    if data_source == "csv":
        return f"本地 CSV / {csv_filename}" if csv_filename else "本地 CSV"
    if stock_name and ticker:
        return f"{stock_name} ({ticker})"
    return ticker or "未提供"


def build_signal_summary(data_with_signals: pd.DataFrame) -> Dict:
    if "signal" not in data_with_signals.columns:
        return {"buy": 0, "sell": 0, "hold": 0}

    counts = data_with_signals["signal"].fillna("hold").value_counts().to_dict()
    return {
        "buy": int(counts.get("buy", 0)),
        "sell": int(counts.get("sell", 0)),
        "hold": int(counts.get("hold", 0)),
    }


def build_visual_dataset(results_df: pd.DataFrame, resolved_strategy: ResolvedStrategy) -> pd.DataFrame:
    base_columns = [
        "date",
        "open",
        "high",
        "low",
        "close",
        "volume",
        "portfolio_value",
        "cash",
        "shares",
        "signal",
        "cumulative_return",
    ]
    overlay_columns = resolved_strategy.adapter.get_overlay_columns(resolved_strategy.params)
    existing_columns = [column for column in base_columns + overlay_columns if column in results_df.columns]
    return results_df[existing_columns].copy()


def build_equity_curve(results_df: pd.DataFrame) -> pd.DataFrame:
    columns = [column for column in ["date", "portfolio_value", "cash", "shares", "cumulative_return"] if column in results_df.columns]
    return results_df[columns].copy()


def build_drawdown_curve(results_df: pd.DataFrame) -> pd.DataFrame:
    if "portfolio_value" not in results_df.columns:
        return pd.DataFrame(columns=["date", "drawdown_pct"])

    drawdown_df = results_df[["date", "portfolio_value"]].copy()
    rolling_max = drawdown_df["portfolio_value"].cummax()
    drawdown_df["drawdown_pct"] = (drawdown_df["portfolio_value"] - rolling_max) / rolling_max * 100
    return drawdown_df[["date", "drawdown_pct"]]


def build_monthly_returns(results_df: pd.DataFrame) -> pd.Series:
    if "date" not in results_df.columns or "daily_return" not in results_df.columns:
        return pd.Series(dtype=float)

    monthly_df = results_df[["date", "daily_return"]].copy()
    monthly_df["date"] = pd.to_datetime(monthly_df["date"])
    monthly_df = monthly_df.set_index("date")
    return monthly_df["daily_return"].resample("M").apply(lambda values: (1 + values).prod() - 1)


def monthly_returns_to_json(monthly_returns: pd.Series) -> List[Dict]:
    if monthly_returns.empty:
        return []

    payload = []
    for index, value in monthly_returns.items():
        payload.append(
            {
                "month": pd.Timestamp(index).strftime("%Y-%m"),
                "return_pct": float(value * 100),
            }
        )
    return payload


def df_to_json_safe(df: pd.DataFrame) -> List[Dict]:
    if df is None or df.empty:
        return []

    safe_df = df.copy()
    for column in safe_df.columns:
        if pd.api.types.is_datetime64_any_dtype(safe_df[column]):
            safe_df[column] = safe_df[column].dt.strftime("%Y-%m-%d")

    safe_df = safe_df.replace([np.inf, -np.inf], np.nan)
    safe_df = safe_df.where(pd.notnull(safe_df), None)
    return json.loads(json.dumps(safe_df.to_dict("records"), ignore_nan=True))


def dict_to_json_safe(data: Dict) -> Dict:
    return json.loads(json.dumps(data, ignore_nan=True))


if __name__ == "__main__":
    uvicorn.run(app, host="0.0.0.0", port=int(os.getenv("PORT", "8000")))
