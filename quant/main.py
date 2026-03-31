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

from config import EXECUTION_PRICE
from data.data_loader import DataLoader, generate_sample_data
from data.fundamentals import get_symbol_fundamentals
from data.scripts.akshare_loader import fetch_stock_data, resolve_stock_name_with_debug
from engine.backtest_engine import BacktestEngine
from result.metrics import PerformanceMetrics
from strategy_library.models import StrategyDefinition, StrategyParamDefinition
from strategy_library.registry import StrategyRegistry
from strategy_library.resolver import ResolvedStrategy, StrategyResolver
from screener.scanner import (
    FILTERABLE_COLUMNS,
    apply_filters,
    df_to_records,
    get_a_share_snapshot,
    get_industry_options,
    sort_and_paginate,
)
from screener.quadrant import compute_all_quadrant_scores, get_cached_scores
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


class RuntimeStrategyPayload(BaseModel):
    id: str
    key: str
    name: str
    implementation_key: str
    params: Dict[str, Any] = Field(default_factory=dict)


class ScreenerFilterRange(BaseModel):
    min: Optional[float] = None
    max: Optional[float] = None


class ScreenerScanRequest(BaseModel):
    filters: Dict[str, ScreenerFilterRange] = Field(default_factory=dict)
    industry: Optional[str] = Field(default=None)
    sort_by: str = Field(default="code")
    sort_order: str = Field(default="asc")
    page: int = Field(default=1, ge=1)
    page_size: int = Field(default=50, ge=1, le=200)


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
    runtime_strategy: Optional[RuntimeStrategyPayload] = Field(default=None, description="由网关注入的运行态策略定义")
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


@app.post("/api/screener/scan")
def screener_scan(req: ScreenerScanRequest):
    """A 股全市场筛选：获取实时快照 → 多维指标范围过滤 → 排序 → 分页返回"""
    try:
        df = get_a_share_snapshot()

        # 将 Pydantic 模型转为 dict
        raw_filters = {}
        for key, bounds in req.filters.items():
            if key not in FILTERABLE_COLUMNS:
                continue
            entry = {}
            if bounds.min is not None:
                entry["min"] = bounds.min
            if bounds.max is not None:
                entry["max"] = bounds.max
            if entry:
                raw_filters[key] = entry

        industries = get_industry_options(df)
        df = apply_filters(df, raw_filters, industry=req.industry)
        page_df, total = sort_and_paginate(df, req.sort_by, req.sort_order, req.page, req.page_size)

        return {
            "total": total,
            "page": req.page,
            "page_size": req.page_size,
            "items": df_to_records(page_df),
            "industries": industries,
        }
    except RuntimeError as exc:
        raise HTTPException(status_code=503, detail=str(exc)) from exc
    except Exception as exc:
        logger.exception("选股筛选异常")
        raise HTTPException(status_code=500, detail=f"选股筛选失败: {exc}") from exc


@app.get("/api/fundamentals/{symbol}")
def get_fundamentals(symbol: str):
    try:
        return get_symbol_fundamentals(symbol)
    except ValueError as exc:
        raise HTTPException(status_code=400, detail=str(exc)) from exc
    except RuntimeError as exc:
        raise HTTPException(status_code=503, detail=str(exc)) from exc
    except Exception as exc:
        logger.exception("基础面接口异常 symbol=%s", symbol)
        raise HTTPException(status_code=500, detail=f"基础面加载失败: {exc}") from exc


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


def resolve_backtest_strategy(req: BacktestRequest) -> ResolvedStrategy:
    if req.runtime_strategy:
        runtime = req.runtime_strategy
        adapter = strategy_registry.get_adapter(runtime.implementation_key)
        params = runtime.params or {}
        adapter.validate_params(params)
        definition = StrategyDefinition(
            id=runtime.id,
            key=runtime.key,
            name=runtime.name,
            description="",
            category="",
            implementation_key=runtime.implementation_key,
            status="active",
            version=1,
            created_at="",
            updated_at="",
            param_schema=[],
            default_params=params,
            required_indicators=[],
            chart_overlays=[],
            ui_schema={},
            execution_options={},
            metadata={},
        )
        return ResolvedStrategy(definition=definition, params=params, adapter=adapter)

    return strategy_resolver.resolve(
        strategy_id=req.strategy_id,
        strategy_name=req.strategy_name,
        override_params=req.strategy_params,
    )


@app.post("/api/backtest")
def run_backtest_api(req: BacktestRequest):
    try:
        start_dt, end_dt = parse_date_range(req.start_date, req.end_date)
        resolved_strategy = resolve_backtest_strategy(req)

        raw_data, source_name, stock_name, stock_name_debug = load_market_data(req, start_dt, end_dt)
        if raw_data.empty:
            raise HTTPException(status_code=400, detail="可用行情数据为空，无法回测")

        results_df, trades_df, enriched_df = execute_backtest(raw_data, resolved_strategy, req.capital, req.fee_pct)
        buy_and_hold_curve = build_buy_and_hold_curve(results_df, req.capital, req.fee_pct)
        metrics = calculate_metrics(results_df, trades_df, req.capital, buy_and_hold_curve)
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
                "buy_and_hold_curve": df_to_json_safe(buy_and_hold_curve),
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


def calculate_metrics(
    results_df: pd.DataFrame,
    trades_df: pd.DataFrame,
    capital: float,
    buy_and_hold_curve: Optional[pd.DataFrame] = None,
) -> Dict:
    metrics_calc = PerformanceMetrics(
        portfolio_values=results_df["portfolio_value"].tolist(),
        daily_returns=results_df["daily_return"].tolist(),
        trades=trades_df.to_dict("records"),
        initial_capital=capital,
    )
    metrics = metrics_calc.calculate_all_metrics()

    if buy_and_hold_curve is not None and not buy_and_hold_curve.empty:
        buy_and_hold_final_capital = float(buy_and_hold_curve["portfolio_value"].iloc[-1])
        buy_and_hold_return_pct = (buy_and_hold_final_capital - capital) / capital * 100 if capital else 0.0
        buy_and_hold_max_drawdown_pct = calculate_max_drawdown_pct(buy_and_hold_curve["portfolio_value"])

        metrics["buy_and_hold_final_capital"] = buy_and_hold_final_capital
        metrics["buy_and_hold_return_pct"] = buy_and_hold_return_pct
        metrics["buy_and_hold_max_drawdown_pct"] = buy_and_hold_max_drawdown_pct
        metrics["excess_return_pct"] = metrics.get("total_return_pct", 0.0) - buy_and_hold_return_pct

    return dict_to_json_safe(metrics)


def build_buy_and_hold_curve(results_df: pd.DataFrame, capital: float, fee_pct: float) -> pd.DataFrame:
    required_columns = {"date", "close"}
    if results_df.empty or not required_columns.issubset(set(results_df.columns)):
        return pd.DataFrame(columns=["date", "portfolio_value", "cumulative_return"])

    buy_index = 0
    buy_price_column = "close"
    if EXECUTION_PRICE == "next_open" and "open" in results_df.columns and len(results_df) > 1:
        buy_index = 1
        buy_price_column = "open"

    buy_price = float(results_df.iloc[buy_index][buy_price_column]) if len(results_df) > buy_index else 0.0
    buy_and_hold_shares = 0
    buy_and_hold_cash = capital

    if buy_price > 0 and capital > 0:
        estimated_fee = capital * fee_pct
        investable_cash = capital - estimated_fee
        buy_and_hold_shares = int(investable_cash / buy_price)

        if buy_and_hold_shares > 0:
            buy_amount = buy_and_hold_shares * buy_price
            buy_fee = buy_amount * fee_pct
            buy_and_hold_cash = capital - buy_amount - buy_fee

    close_series = pd.to_numeric(results_df["close"], errors="coerce")
    curve_rows: List[Dict[str, Any]] = []
    last_valid_close = buy_price if buy_price > 0 else None

    for position, (_, row) in enumerate(results_df.iterrows()):
        close_price = close_series.iloc[position]
        if pd.notna(close_price):
            last_valid_close = float(close_price)

        if position < buy_index or buy_and_hold_shares <= 0:
            portfolio_value = capital
        else:
            reference_price = last_valid_close if last_valid_close is not None else 0.0
            portfolio_value = buy_and_hold_cash + buy_and_hold_shares * reference_price

        cumulative_return = (portfolio_value / capital - 1.0) if capital else 0.0
        curve_rows.append(
            {
                "date": row["date"],
                "portfolio_value": portfolio_value,
                "cumulative_return": cumulative_return,
            }
        )

    return pd.DataFrame(curve_rows)


def calculate_max_drawdown_pct(values: pd.Series) -> float:
    if values is None:
        return 0.0

    series = pd.Series(values).replace([np.inf, -np.inf], np.nan).dropna()
    if series.empty:
        return 0.0

    rolling_max = series.cummax()
    drawdown_series = (series - rolling_max) / rolling_max * 100
    drawdown_min = drawdown_series.min()
    return abs(float(drawdown_min)) if drawdown_min < 0 else 0.0


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


# ── Quadrant (四象限) ──


class QuadrantComputeRequest(BaseModel):
    callback_url: str = Field(default="", description="Go 后端回调 URL，为空则不回调")


@app.post("/api/quadrant/compute-all")
def quadrant_compute_all(req: QuadrantComputeRequest):
    """触发全市场四象限评分计算（异步后台执行）。

    Go 后端定时调用此端点，Quant 立即返回 accepted，
    计算完成后回调 callback_url 写入 DB。
    """
    import threading as _threading

    callback = req.callback_url.strip() if req.callback_url else None

    def _run():
        try:
            compute_all_quadrant_scores(callback_url=callback)
        except Exception as exc:
            logger.exception("[quadrant] compute-all 后台任务失败: %s", exc)

    _threading.Thread(target=_run, daemon=True).start()
    return {"status": "accepted", "message": "四象限计算已在后台启动"}


@app.get("/api/quadrant/scores")
def quadrant_get_scores():
    """返回最近一次缓存的全市场四象限评分。"""
    cached = get_cached_scores()
    if cached is None:
        raise HTTPException(status_code=404, detail="四象限数据尚未计算，请等待凌晨定时任务完成")
    return {
        "total": len(cached),
        "items": cached,
    }


# ── Signal Evaluation ──

class SignalEvaluateBar(BaseModel):
    date: str
    open: float
    high: float
    low: float
    close: float
    volume: float


class SignalEvaluateRequest(BaseModel):
    strategy_id: str = ""
    implementation_key: str = ""
    strategy_name: str = ""
    params: Optional[Dict[str, Any]] = None
    symbol: str = ""
    bars: List[SignalEvaluateBar] = Field(default_factory=list)
    snapshot_price: Optional[float] = None


# 每种策略对应的中文触发原因模板
SIGNAL_REASON_BUILDERS: Dict[str, Any] = {}


def _build_trend_reason(params: Dict, enriched: pd.DataFrame, signal: str) -> Dict[str, Any]:
    row = enriched.iloc[-1]
    short_col = f"MA{int(params['ma_short'])}"
    long_col = f"MA{int(params['ma_long'])}"
    ma_short_val = round(float(row.get(short_col, 0)), 2)
    ma_long_val = round(float(row.get(long_col, 0)), 2)
    if signal == "buy":
        return {
            "kind": "golden_cross",
            "message": f"短均线（MA{int(params['ma_short'])}）从下方向上穿越长均线（MA{int(params['ma_long'])}），形成金叉买入信号。当前 MA{int(params['ma_short'])}={ma_short_val}，MA{int(params['ma_long'])}={ma_long_val}。",
        }
    return {
        "kind": "death_cross",
        "message": f"短均线（MA{int(params['ma_short'])}）从上方向下跌破长均线（MA{int(params['ma_long'])}），形成死叉卖出信号。当前 MA{int(params['ma_short'])}={ma_short_val}，MA{int(params['ma_long'])}={ma_long_val}。",
    }


def _build_bollinger_reason(params: Dict, enriched: pd.DataFrame, signal: str) -> Dict[str, Any]:
    row = enriched.iloc[-1]
    close = round(float(row.get("close", 0)), 2)
    upper = round(float(row.get("BB_upper", 0)), 2)
    lower = round(float(row.get("BB_lower", 0)), 2)
    if signal == "buy":
        return {
            "kind": "bollinger_lower_break",
            "message": f"价格跌破布林带下轨，触发超卖买入信号。当前收盘价={close}，下轨={lower}。",
        }
    return {
        "kind": "bollinger_upper_break",
        "message": f"价格突破布林带上轨，触发超买卖出信号。当前收盘价={close}，上轨={upper}。",
    }


def _build_rsi_reason(params: Dict, enriched: pd.DataFrame, signal: str) -> Dict[str, Any]:
    row = enriched.iloc[-1]
    rsi_col = f"RSI_{int(params['rsi_period'])}"
    rsi_val = round(float(row.get(rsi_col, 0)), 2)
    if signal == "buy":
        return {
            "kind": "rsi_oversold_recovery",
            "message": f"RSI 从低位阈值 {params['rsi_low']} 向上突破，确认超卖修复。当前 RSI={rsi_val}。",
        }
    return {
        "kind": "rsi_overbought_pullback",
        "message": f"RSI 从高位阈值 {params['rsi_high']} 向下跌破，确认超买回落。当前 RSI={rsi_val}。",
    }


def _build_grid_reason(params: Dict, enriched: pd.DataFrame, signal: str) -> Dict[str, Any]:
    row = enriched.iloc[-1]
    close = round(float(row.get("close", 0)), 2)
    if signal == "buy":
        return {
            "kind": "grid_down_cross",
            "message": f"价格下穿网格线，触发逐级买入。当前价格={close}，网格步长={float(params['grid_step'])*100:.1f}%。",
        }
    return {
        "kind": "grid_up_cross",
        "message": f"价格上穿网格线，触发逐级卖出。当前价格={close}，网格步长={float(params['grid_step'])*100:.1f}%。",
    }


SIGNAL_REASON_BUILDERS = {
    "trend_cross": _build_trend_reason,
    "bollinger_reversion": _build_bollinger_reason,
    "rsi_range": _build_rsi_reason,
    "grid": _build_grid_reason,
}


@app.post("/api/signal/evaluate")
def evaluate_signal(req: SignalEvaluateRequest):
    """根据策略 + 历史 K 线评估最新信号，返回 side + reason + 策略信息。

    支持两种调用方式：
    1. 直传 implementation_key + params（推荐，支持用户自建策略）
    2. 传 strategy_id 从本地 JSON 解析（兼容预设策略）
    """
    try:
        if not req.bars or len(req.bars) < 2:
            raise ValueError("bars 数据不足，至少需要 2 根 K 线")

        # Resolve strategy: prefer direct implementation_key + params
        impl_key = req.implementation_key.strip() if req.implementation_key else ""
        direct_params = req.params or {}
        strategy_info_id = req.strategy_id or ""
        strategy_info_name = req.strategy_name or ""

        if impl_key and direct_params:
            # Direct mode: use implementation_key + params from request body
            adapter = strategy_registry.get_adapter(impl_key)
            adapter.validate_params(direct_params)
            used_params = direct_params
        elif req.strategy_id:
            # Fallback mode: resolve from local JSON
            resolved = strategy_resolver.resolve(strategy_id=req.strategy_id)
            impl_key = resolved.definition.implementation_key
            used_params = resolved.params
            strategy_info_id = resolved.definition.id
            strategy_info_name = resolved.definition.name
            adapter = resolved.adapter
        else:
            raise ValueError("必须提供 implementation_key + params 或 strategy_id")

        records = [{"date": b.date, "open": b.open, "high": b.high, "low": b.low, "close": b.close, "volume": b.volume} for b in req.bars]
        bars_df = pd.DataFrame(records)
        bars_df["date"] = pd.to_datetime(bars_df["date"])

        enriched = adapter.attach_indicators(bars_df, used_params)
        strategy_instance = adapter.build_strategy(enriched, used_params)
        data_with_signals = strategy_instance.generate_signals()

        latest_signal = str(data_with_signals["signal"].iloc[-1]).strip().lower()
        if latest_signal not in ("buy", "sell"):
            latest_signal = "hold"

        # 构建触发原因
        reason: Dict[str, Any] = {"kind": "strategy_signal", "message": f"策略评估结果: {latest_signal.upper()}"}
        reason_builder = SIGNAL_REASON_BUILDERS.get(impl_key)
        if reason_builder and latest_signal in ("buy", "sell"):
            try:
                reason = reason_builder(used_params, data_with_signals, latest_signal)
            except Exception:
                pass

        # 计算评分（简单：buy/sell=1.0, hold=0）
        score = 1.0 if latest_signal in ("buy", "sell") else 0.0

        return {
            "side": latest_signal.upper(),
            "score": score,
            "reason": reason,
            "strategy": {
                "id": strategy_info_id,
                "name": strategy_info_name,
                "implementation_key": impl_key,
                "params": used_params,
            },
            "bars_count": len(req.bars),
            "latest_date": req.bars[-1].date if req.bars else None,
            "latest_close": req.bars[-1].close if req.bars else None,
        }
    except KeyError as exc:
        raise HTTPException(status_code=404, detail=str(exc)) from exc
    except ValueError as exc:
        raise HTTPException(status_code=400, detail=str(exc)) from exc
    except Exception as exc:
        logger.exception("信号评估异常 strategy_id=%s symbol=%s", req.strategy_id, req.symbol)
        raise HTTPException(status_code=500, detail=f"信号评估失败: {exc}") from exc


if __name__ == "__main__":
    uvicorn.run(app, host="0.0.0.0", port=int(os.getenv("PORT", "8000")))
