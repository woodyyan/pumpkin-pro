"""
Backtest 在线行情适配层。

职责：把 backtest「在线下载」模式的语义（datetime 起止、5位/6位裸代码、
中文数据源名）转换为 data_sources provider 降级体系的调用，并把体系返回的
DailyBar 列表还原为 backtest 引擎期望的 DataFrame。

设计要点：
- 不承担降级逻辑，降级由 DataSourceManager 负责（港股 tencent→eastmoney→akshare，
  A股 baostock→tencent→eastmoney→akshare）。
- 通过 manager.fetch(DataSourceRequest(..., extras={"caller": "backtest"})) 直连，
  以便透传 caller 归因与预留 providers_override 覆盖钩子。
- 返回 DataFrame 使用 DailyBar.to_dict() 的 date/open/high/low/close/volume 列，
  已被 DataLoader.COLUMN_ALIASES 直接识别，无需额外改列名。
"""

import logging
from datetime import datetime
from typing import List, Optional, Tuple

import pandas as pd

from data_sources import DataSourceManager, Market
from data_sources.models import Capability, DataSourceRequest

logger = logging.getLogger(__name__)


# provider 命中名 → 前端展示用中文数据源名。key 为 (provider, market)。
_SOURCE_LABELS = {
    ("baostock", Market.ASHARE): "BaoStock-A股 (BaoStock)",
    ("tencent", Market.ASHARE): "腾讯财经-A股 (Tencent)",
    ("tencent", Market.HKEX): "腾讯财经-港股 (Tencent HK)",
    ("eastmoney", Market.ASHARE): "东方财富-A股 (EastMoney)",
    ("eastmoney", Market.HKEX): "东方财富-港股 (EastMoney HK)",
    ("akshare", Market.ASHARE): "AKShare-A股 (AKShare)",
    ("akshare", Market.HKEX): "AKShare-港股 (AKShare HK)",
}


def detect_market(ticker: str) -> str:
    """把裸股票代码识别为 provider 体系的 Market 枚举。

    - 5 位纯数字 -> 港股 (HKEX)
    - 6 位纯数字 -> A股 (ASHARE)
    其余格式抛 ValueError，语义与 akshare_loader._detect_market 保持一致。
    """
    code = str(ticker or "").strip()
    if len(code) == 5 and code.isdigit():
        return Market.HKEX
    if len(code) == 6 and code.isdigit():
        return Market.ASHARE
    raise ValueError(f"无法识别的股票代码格式: {ticker}。A股请用6位数字，港股请用5位数字。")


def _source_label(provider: str, market: str) -> str:
    """把命中的 provider 名映射为中文数据源名，未知组合回退为 provider 原名。"""
    return _SOURCE_LABELS.get((provider, market), provider or "未知数据源")


def _summarize_errors(errors: List[str]) -> str:
    if not errors:
        return "无可用数据源返回结果"
    return " | ".join(str(e) for e in errors)


def fetch_online_bars(
    ticker: str,
    start_date: datetime,
    end_date: datetime,
    manager: DataSourceManager,
    adjust: str = "qfq",
) -> Tuple[pd.DataFrame, str]:
    """通过 provider 降级体系获取在线行情。

    Args:
        ticker: 裸股票代码（A股6位 / 港股5位）。
        start_date/end_date: 回测区间起止（datetime）。
        manager: 复用的 DataSourceManager 单例（由调用方注入，保证 health/配额一致）。
        adjust: 复权方式，默认前复权 qfq。

    Returns:
        (DataFrame, source_name)。DataFrame 至少含 date/open/high/low/close/volume。

    Raises:
        ValueError: 代码格式非法。
        RuntimeError: 所有数据源均失败或返回空数据。
    """
    if not ticker:
        raise ValueError("在线下载模式必须填写股票代码")

    market = detect_market(ticker)
    start_str = start_date.strftime("%Y%m%d")
    end_str = end_date.strftime("%Y%m%d")

    request = DataSourceRequest(
        capability=Capability.DAILY_BARS,
        market=market,
        symbol=ticker,
        start_date=start_str,
        end_date=end_str,
        adjust=adjust,
        # 拉区间、不锁定单一交易日：require_exact_trade_date 保持默认 False，
        # 避免 validators 对区间数据做「目标交易日必须存在」的强校验。
        extras={"caller": "backtest"},
    )

    response = manager.fetch(request)

    if not response.ok:
        detail = _summarize_errors(response.errors)
        logger.warning(
            "在线行情获取失败 ticker=%s market=%s detail=%s", ticker, market, detail
        )
        raise RuntimeError(f"所有数据源均连接失败。详细排查: {detail}")

    bars = response.data or []
    if not bars:
        raise RuntimeError(f"未能在任何数据源中找到 {ticker} 的交易记录。")

    df = pd.DataFrame([bar.to_dict() for bar in bars])

    provider = response.used_sources[0] if response.used_sources else ""
    source_name = _source_label(provider, market)
    logger.info(
        "在线行情获取成功 ticker=%s market=%s provider=%s rows=%d",
        ticker, market, provider, len(df),
    )
    return df, source_name
