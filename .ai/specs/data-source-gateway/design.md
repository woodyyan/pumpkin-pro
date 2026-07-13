# Design: Quant Data Source Gateway

## 总体架构

```text
backend
  -> quant API
    -> business service
      -> data_sources.DataSourceManager
        -> policy
        -> registry
        -> providers
        -> normalizers
        -> validators
        -> health trace
```

## 模块职责

| 模块 | 职责 |
|---|---|
| `models.py` | 定义 Capability、Market、DataSourceRequest、DataSourceResponse、DailyBar、SourceTrace |
| `errors.py` | 定义统一错误类型，如 `TradeDateMismatchError`、`ValidationError` |
| `policy.py` | 代码常量维护 capability + market 的 provider 顺序和策略 |
| `registry.py` | 声明 provider 支持的 market + capability 矩阵 |
| `manager.py` | Gateway 主入口，执行 policy 解析、provider fallback、validation、trace 生成 |
| `validators.py` | 校验日线非空、OHLC 正数、high/low 合法、目标交易日精确匹配 |
| `providers/` | Tencent、EastMoney、AkShare adapter，只负责拉取源数据 |
| `normalizers/` | 将 provider 原始数据转为标准 `DailyBar` |
| `health.py` | 记录最近 provider trace，供后续 Admin 健康区块使用 |

## 当前已落地能力

- `daily_bars`
- `index_bars`
- `capital_map`
- `fundamentals`
- `financials`
- `dividends`

## 当前策略

| Capability | Market | Provider 顺序 | 规则 |
|---|---|---|---|
| `daily_bars` | `ASHARE` | Tencent → EastMoney → AkShare | 价格必须精确交易日 |
| `daily_bars` | `HKEX` | Tencent → EastMoney → AkShare | 价格必须精确交易日 |
| `index_bars` | `ASHARE` | Tencent → EastMoney → AkShare | 价格必须精确交易日 |
| `index_bars` | `HKEX` | Tencent → EastMoney → AkShare | EastMoney/AkShare 未声明时由 registry skip |
| `capital_map` | `ASHARE` | EastMoney | 仅 A 股，返回整页资金星图快照 |
| `fundamentals` | `ASHARE` | AkShare → EastMoney → Tencent | 统一返回基础面 payload |
| `fundamentals` | `HKEX` | EastMoney → Tencent → AkShare | 统一返回基础面 payload |
| `financials` | `ASHARE` | AkShare → EastMoney → Tencent | 当前先复用既有财报抓取函数，Gateway 负责编排 |
| `dividends` | `ASHARE` | AkShare → EastMoney → Tencent | 当前先复用既有分红抓取函数，Gateway 负责编排 |

## 关键规则

1. Provider adapter 不写业务算法，只负责获取外部数据。
2. Normalizer 统一字段和单位，不决定业务是否可用。
3. Validator 是防止“假成功”的边界：日线为空、价格非正、日期不匹配均不能算成功。
4. Manager 允许 fallback；所有 provider 失败时返回 `ok=false, partial=true`，由上层业务决定是否阻断。
5. 第一阶段 policy 是代码常量，禁止新增 env 或 Admin 编辑，避免运维变重。
6. 对 Phase0 结构化表迁移，允许先保留原有行结构与解析逻辑，只把 provider 顺序、fallback、trace 收敛到 Gateway，避免一次性重写财报/分红字段映射。
