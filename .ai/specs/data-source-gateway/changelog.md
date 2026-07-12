# Changelog: Quant Data Source Gateway

## 2026-07-11

- 新增 quant `data_sources` 统一数据源层骨架。
- 新增 `daily_bars` / `index_bars` 第一期能力。
- 数据源策略使用代码常量，不新增 env 或 Admin 可编辑配置。
- Provider 初始包含 Tencent、EastMoney、AkShare。
- Gateway 支持 fallback、unsupported provider skip、source trace 和 partial failure。
- 日线 validator 强制 OHLC 正数；传入 `target_trade_date` 时必须精确命中，禁止用其他日期价格兜底。
- 补充数据源层单元测试。
