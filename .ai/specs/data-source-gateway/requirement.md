# Requirement: Quant Data Source Gateway

## 背景

quant 中已有多个模块直接调用 akshare、东方财富、腾讯等外部数据源，导致数据源优先级、字段解析、降级和日期校验散落在四象限、因子脚本、基础面、资金星图等模块中。未来还会增加更多数据源和市场，需要统一入口。

## 目标

1. 在 quant 内部新增统一 Data Source Gateway，作为外部数据源的唯一出入口。
2. backend 保持纯粹，只调用 quant API，不直接理解 akshare / 东方财富 / 腾讯字段。
3. 数据源优先级按能力和市场独立管理，但第一期只使用代码常量，不新增 env、数据库配置或 Admin 可编辑配置。
4. 支持 provider fallback，允许返回 partial 数据，由上层业务决定是否采用。
5. 价格类数据必须精确匹配目标交易日，不允许用其他日期价格兜底。
6. 第一期先实现 Gateway 骨架与 `daily_bars` / `index_bars` 能力。

## 范围

### In Scope

- 新增 quant `data_sources` 模块：models、errors、registry、policy、manager、validators、providers、normalizers、health。
- `daily_bars` / `index_bars` 支持 A 股、港股。
- Provider 初始包含 Tencent、EastMoney、AkShare。
- Policy 使用代码常量：A 股/港股日线和指数日线默认 `tencent -> eastmoney -> akshare`。
- Validator 强制价格字段有效；传入 `target_trade_date` 时必须精确命中。
- 测试覆盖 policy、registry、fallback、unsupported provider skip、all failed partial、exact-date validation、价格字段 validation。

### Out of Scope

- 不迁移资金星图到 quant（后续 Phase 2）。
- 不迁移 financials / dividends / company_profile（后续 Phase 3）。
- 不新增 Admin 数据源健康区块（后续 Phase 5）。
- 不新增 env 配置或 Admin 可编辑数据源策略。
- 不做美股扩展。
