# Changelog: Quant Data Source Gateway

## 2026-07-11

- 新增 quant `data_sources` 统一数据源层骨架。
- 新增 `daily_bars` / `index_bars` 第一期能力。
- 数据源策略使用代码常量，不新增 env 或 Admin 可编辑配置。
- Provider 初始包含 Tencent、EastMoney、AkShare。
- Gateway 支持 fallback、unsupported provider skip、source trace 和 partial failure。
- 日线 validator 强制 OHLC 正数；传入 `target_trade_date` 时必须精确命中，禁止用其他日期价格兜底。
- 补充数据源层单元测试。

## 2026-07-12

- 四象限 A 股 / 港股日线入口接入 `DataSourceManager.fetch_daily_bars`，保留原有缓存与计算流程。
- 四象限 A 股 / 港股 benchmark 60 日收益入口接入 `DataSourceManager.fetch_index_bars`，分别获取上证指数与恒生指数日线后计算。
- 新增四象限 Gateway 注入测试，覆盖 A 股日线、港股日线、A 股 benchmark、港股 benchmark。
- 四象限接入后仍不新增 env 或 Admin 可编辑配置，provider 顺序继续由 `data_sources/policy.py` 代码常量控制。
