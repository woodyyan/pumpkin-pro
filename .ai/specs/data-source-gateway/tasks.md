# Tasks: Quant Data Source Gateway

## Phase 1: Gateway 骨架 + daily/index bars

- [x] 新增 `quant/data_sources/models.py`。
- [x] 新增 `quant/data_sources/errors.py`。
- [x] 新增 `quant/data_sources/policy.py`，用代码常量维护 provider 顺序。
- [x] 新增 `quant/data_sources/registry.py`，声明 provider capability matrix。
- [x] 新增 `quant/data_sources/validators.py`，校验日线字段和精确交易日。
- [x] 新增 `quant/data_sources/normalizers/daily_bars.py`。
- [x] 新增 Tencent / EastMoney / AkShare provider adapter。
- [x] 新增 `DataSourceManager`，支持 fallback、skip unsupported provider、trace 和 partial failure。
- [x] 新增 `health.py`，记录最近 trace 供后续 Admin 使用。
- [x] 新增 `quant/tests/test_data_sources_gateway.py`。

## Phase 2: 资金星图迁移到 quant

- [ ] quant 新增 `/api/capital-map`。
- [ ] EastMoney provider 增加资金星图行情、估值、板块资金能力。
- [ ] 新增 `normalizers/capital_map.py`。
- [ ] 迁移资金星图 PE 选择、成交额排序、PoC 分箱和板块资金算法到 quant。
- [ ] backend `/api/capital-map` 改为 quant proxy + 30 秒缓存 + stale 降级。

## Phase 3: financials / dividends / company_profile

- [ ] 新增 financials、dividends、company_profile、quote_snapshot capabilities。
- [ ] 将 `quant/data/fundamentals.py` provider fallback 收敛到 Gateway。
- [ ] 将 `quant/data/company_profile.py` provider 调用收敛到 Gateway。
- [ ] 将 factor Phase0 financials/dividends 逐步收敛到 Gateway。

## Phase 4: 核心任务接入 trace

- [ ] 四象限 summary 输出 data source trace。
- [ ] Factor Phase0 每个 mode 输出 data source summary 和覆盖率。
- [ ] 模拟组合 v2 价格需求解析记录 provider / exact trade date trace。

## Phase 5: Admin 数据源健康区块

- [ ] quant 新增 data source health/capabilities API。
- [ ] backend 新增 admin proxy API。
- [ ] frontend `/admin/data` 新增只读数据源健康区块。
