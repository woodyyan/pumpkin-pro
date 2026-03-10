from datetime import datetime
from typing import Dict, List, Optional, Tuple

import numpy as np
import pandas as pd
import simplejson as json
import uvicorn
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel, Field

from data.data_loader import DataLoader, generate_sample_data
from data.scripts.akshare_loader import fetch_stock_data, resolve_stock_name
from engine.backtest_engine import BacktestEngine
from indicators.technical_indicators import TechnicalIndicators
from result.metrics import PerformanceMetrics
from strategy.grid_strategy import GridStrategy
from strategy.mean_reversion_strategy import MeanReversionStrategy
from strategy.range_trading_strategy import RangeTradingStrategy
from strategy.trend_strategy import TrendStrategy

app = FastAPI(title="Pumpkin Quant Service", description="Quantitative Backtesting Engine API")

SUPPORTED_STRATEGIES = [
    "趋势跟踪(双均线)",
    "网格交易",
    "均值回归(布林带)",
    "区间交易(RSI)",
]

SUPPORTED_DATA_SOURCES = ["online", "csv", "sample"]


class StrategyParams(BaseModel):
    ma_short: int = Field(default=20, ge=2)
    ma_long: int = Field(default=60, ge=3)
    grid_count: int = Field(default=5, ge=2, le=20)
    grid_step: float = Field(default=0.05, gt=0.001, le=0.5)
    bb_period: int = Field(default=20, ge=5)
    bb_std: float = Field(default=2.0, gt=0.1, le=5.0)
    rsi_period: int = Field(default=14, ge=2)
    rsi_low: float = Field(default=30.0, ge=1, le=50)
    rsi_high: float = Field(default=70.0, ge=50, le=99)


class SampleDataConfig(BaseModel):
    start_price: float = Field(default=100.0, gt=0)
    drift: float = Field(default=0.0005, ge=-0.2, le=0.2)
    volatility: float = Field(default=0.02, gt=0, le=0.5)
    seed: int = Field(default=42, ge=0)


class BacktestRequest(BaseModel):
    data_source: str = Field(default="online", description="online/csv/sample")
    ticker: Optional[str] = Field(default=None, description="A股六位数字或港股五位数字")
    start_date: str = Field(..., description="YYYY-MM-DD")
    end_date: str = Field(..., description="YYYY-MM-DD")
    capital: float = Field(default=100000.0, gt=0)
    fee_pct: float = Field(default=0.001, ge=0, le=0.05)
    strategy_name: str = Field(default="趋势跟踪(双均线)")
    strategy_params: StrategyParams = Field(default_factory=StrategyParams)
    csv_content: Optional[str] = Field(default=None, description="上传的本地 CSV 文本")
    csv_filename: Optional[str] = Field(default=None)
    sample_config: SampleDataConfig = Field(default_factory=SampleDataConfig)


@app.get("/api/health")
def health_check():
    return {
        "status": "online",
        "service": "Pumpkin Quant Engine",
        "strategies": SUPPORTED_STRATEGIES,
        "data_sources": SUPPORTED_DATA_SOURCES,
    }


@app.get("/api/backtest/options")
def get_backtest_options():
    return {
        "strategies": SUPPORTED_STRATEGIES,
        "data_sources": SUPPORTED_DATA_SOURCES,
    }


@app.post("/api/backtest")
def run_backtest_api(req: BacktestRequest):
    try:
        start_dt, end_dt = parse_date_range(req.start_date, req.end_date)
        params = model_to_dict(req.strategy_params)

        raw_data, source_name, stock_name = load_market_data(req, start_dt, end_dt)
        if raw_data.empty:
            raise HTTPException(status_code=400, detail="可用行情数据为空，无法回测")

        results_df, trades_df, enriched_df = execute_backtest(raw_data, req.strategy_name, params, req.capital, req.fee_pct)
        metrics = calculate_metrics(results_df, trades_df, req.capital)
        signal_summary = build_signal_summary(enriched_df)

        response = {
            "status": "success",
            "source_used": source_name,
            "data_source": req.data_source,
            "data_summary": build_data_summary(raw_data, req, source_name, stock_name),
            "strategy": {
                "name": req.strategy_name,
                "params": params,
            },
            "metrics": metrics,
            "signal_summary": signal_summary,
            "trades": df_to_json_safe(trades_df),
            "kline_data": df_to_json_safe(build_visual_dataset(results_df, req.strategy_name, params)),
            "analysis": {
                "equity_curve": df_to_json_safe(build_equity_curve(results_df)),
                "drawdown_curve": df_to_json_safe(build_drawdown_curve(results_df)),
                "monthly_returns": monthly_returns_to_json(build_monthly_returns(results_df)),
            },
        }
        return response
    except HTTPException:
        raise
    except ValueError as exc:
        raise HTTPException(status_code=400, detail=str(exc)) from exc
    except Exception as exc:
        import traceback

        traceback.print_exc()
        raise HTTPException(status_code=500, detail=str(exc)) from exc


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


def load_market_data(req: BacktestRequest, start_dt: datetime, end_dt: datetime) -> Tuple[pd.DataFrame, str, Optional[str]]:
    loader = DataLoader()

    if req.data_source not in SUPPORTED_DATA_SOURCES:
        raise ValueError(f"不支持的数据源: {req.data_source}")

    if req.data_source == "online":
        if not req.ticker:
            raise ValueError("在线下载模式必须填写股票代码")
        data, source_name = fetch_stock_data(req.ticker, start_dt, end_dt)
        stock_name = resolve_stock_name(req.ticker)
        return loader.prepare_dataframe(data), source_name, stock_name

    if req.data_source == "csv":
        if not req.csv_content:
            raise ValueError("本地 CSV 模式必须上传 CSV 文件内容")
        data = loader.prepare_csv_content(req.csv_content)
        filtered = filter_date_range(data, start_dt, end_dt)
        filename = req.csv_filename or "uploaded.csv"
        return filtered, f"本地CSV ({filename})", None

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
    return prepared, "系统示例行情", None


def filter_date_range(data: pd.DataFrame, start_dt: datetime, end_dt: datetime) -> pd.DataFrame:
    filtered = data[(data["date"] >= pd.Timestamp(start_dt)) & (data["date"] <= pd.Timestamp(end_dt))].copy()
    if filtered.empty:
        raise ValueError("指定日期区间内没有可用行情数据")
    return filtered.reset_index(drop=True)


def execute_backtest(
    market_data: pd.DataFrame,
    strategy_name: str,
    params: Dict,
    capital: float,
    fee_pct: float,
) -> Tuple[pd.DataFrame, pd.DataFrame, pd.DataFrame]:
    if strategy_name not in SUPPORTED_STRATEGIES:
        raise ValueError(f"未知策略: {strategy_name}")

    enriched_data = attach_indicators(market_data, strategy_name, params)
    strategy = build_strategy(strategy_name, enriched_data, params)
    data_with_signals = strategy.generate_signals()

    engine = BacktestEngine(data_with_signals, capital, fee_pct)
    results_df = engine.run_backtest()
    trades_df = engine.get_trade_log()
    return results_df, trades_df, data_with_signals


def attach_indicators(data: pd.DataFrame, strategy_name: str, params: Dict) -> pd.DataFrame:
    indicator_calc = TechnicalIndicators(data)
    enriched = indicator_calc.data.copy()

    if strategy_name == "趋势跟踪(双均线)":
        short_period = params["ma_short"]
        long_period = params["ma_long"]
        if short_period >= long_period:
            raise ValueError("双均线策略要求短均线周期小于长均线周期")
        enriched[f"MA{short_period}"] = indicator_calc.calculate_ma(short_period)
        enriched[f"MA{long_period}"] = indicator_calc.calculate_ma(long_period)

    elif strategy_name == "均值回归(布林带)":
        upper_band, mid_band, lower_band = indicator_calc.calculate_bollinger_bands(
            period=params["bb_period"], std_dev=params["bb_std"]
        )
        enriched["BB_upper"] = upper_band
        enriched["BB_mid"] = mid_band
        enriched["BB_lower"] = lower_band

    elif strategy_name == "区间交易(RSI)":
        if params["rsi_low"] >= params["rsi_high"]:
            raise ValueError("RSI 低阈值必须小于高阈值")
        enriched[f"RSI_{params['rsi_period']}"] = indicator_calc.calculate_rsi(period=params["rsi_period"])

    return enriched


def build_strategy(strategy_name: str, enriched_data: pd.DataFrame, params: Dict):
    if strategy_name == "趋势跟踪(双均线)":
        return TrendStrategy(enriched_data, ma_short=params["ma_short"], ma_long=params["ma_long"])
    if strategy_name == "网格交易":
        return GridStrategy(
            enriched_data,
            grid_count=params["grid_count"],
            grid_step_pct=params["grid_step"],
        )
    if strategy_name == "均值回归(布林带)":
        return MeanReversionStrategy(enriched_data, bb_period=params["bb_period"])
    return RangeTradingStrategy(
        enriched_data,
        rsi_period=params["rsi_period"],
        rsi_low=params["rsi_low"],
        rsi_high=params["rsi_high"],
    )


def calculate_metrics(results_df: pd.DataFrame, trades_df: pd.DataFrame, capital: float) -> Dict:
    metrics_calc = PerformanceMetrics(
        portfolio_values=results_df["portfolio_value"].tolist(),
        daily_returns=results_df["daily_return"].tolist(),
        trades=trades_df.to_dict("records"),
        initial_capital=capital,
    )
    return dict_to_json_safe(metrics_calc.calculate_all_metrics())


def build_data_summary(data: pd.DataFrame, req: BacktestRequest, source_name: str, stock_name: Optional[str]) -> Dict:
    loader = DataLoader()
    summary = loader.get_data_summary(data)
    ticker_display = build_ticker_display(req.data_source, req.ticker, stock_name, req.csv_filename)
    return {
        "source_used": source_name,
        "ticker": req.ticker,
        "ticker_name": stock_name,
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


def build_visual_dataset(results_df: pd.DataFrame, strategy_name: str, params: Dict) -> pd.DataFrame:
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

    overlay_columns: List[str] = []
    if strategy_name == "趋势跟踪(双均线)":
        overlay_columns = [f"MA{params['ma_short']}", f"MA{params['ma_long']}"]
    elif strategy_name == "均值回归(布林带)":
        overlay_columns = ["BB_upper", "BB_mid", "BB_lower"]
    elif strategy_name == "区间交易(RSI)":
        overlay_columns = [f"RSI_{params['rsi_period']}"]

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
    uvicorn.run(app, host="0.0.0.0", port=8000)
