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


## 2026-07-13

- Gateway 新增 `company_profile` capability，A 股 / 港股公司资料入口改为通过 `DataSourceManager` 编排 provider 顺序与 fallback。
- `company_profile` 第一阶段继续复用既有 `data/company_profile.py` 抓取逻辑作为 legacy adapter，先收敛入口、trace 与降级，不一次性重写字段解析。
- quant 新增 `/api/data-sources/health`，输出最近 provider/capability trace、聚合计数与最近事件。
- backend 新增 `/api/admin/data-source-health` 与 `/api/admin/company-profiles/refresh`，admin 数据页可查看 Gateway 健康并手动触发公司资料刷新。
- frontend `/admin/data` 新增“数据源健康”区块，展示公司资料同步状态、覆盖率、失败项，以及 Gateway 的 provider/capability 最近状态。
- Phase 2 资金星图迁移到 quant：新增 `capital_map` capability，A 股资金星图由 EastMoney provider 统一出入口提供。
- 新增 quant `capital_map` 模块，承载字段归一、PE 选择、成交额排序、PoC 分箱、板块资金排序和 `/api/capital-map` payload 构造。
- backend `/api/capital-map` 从直接请求东方财富改为 quant proxy，并保留 30 秒缓存与 stale 降级。
- Gateway 新增 `fundamentals`、`financials`、`dividends` capability。
- 因子 Phase0 的 `daily-bars`、`financials`、`dividends` 改为通过 Gateway 统一编排 provider 顺序与 fallback，同时保持原有表结构和 CLI 参数兼容。
- `fundamentals` / `financials` / `dividends` 第一阶段复用既有基础面/Phase0 抓取逻辑作为 adapter，避免一次性重写财报与分红字段映射。
- 补充 Gateway 与 Phase0 接入测试；`cd quant && pytest -q` 通过。

## 2026-07-29

- 背景：生产（香港服务器）backtest 港股在线回测报「所有数据源均连接失败」，根因是旧 `data.scripts.akshare_loader.fetch_stock_data` 仅依赖东财+新浪，东财对境外出口 IP 在 TCP 层 RST（RemoteDisconnected），且新浪层静默失效未兜住。
- Backtest「在线下载」模式（`main.py::load_market_data` online 分支）迁移到 Gateway：新增薄适配层 `quant/data/scripts/online_market_data.py::fetch_online_bars`，把 datetime 起止 / 5位6位裸代码 / 中文源名 转换为 `DataSourceManager.fetch()` 调用（`DataSourceRequest(capability=DAILY_BARS, extras={"caller":"backtest"})`）。港股天然获得 `tencent→eastmoney→akshare` 降级链（tencent 带 UA+429退避，境外最稳），A股获得 `baostock→tencent→eastmoney→akshare`。
- 输出对齐：`[bar.to_dict() for bar in resp.data]` → DataFrame（列 date/open/high/low/close/volume 已被 `DataLoader.COLUMN_ALIASES` 直接识别），失败判 `resp.ok`（policy allow_partial=True 不抛异常），命中源 `resp.used_sources[0]` 映射为中文 source_name。
- 复用 `main.py` 已有 `data_source_manager` 单例（注入 adapter），保证 health 账本 / baostock 长连接 / 配额守卫统一；不改 manager/policy/registry。
- 附带方案二：`data_sources/providers/eastmoney.py` 新增 `HEADERS`（浏览器 UA + `Referer: https://quote.eastmoney.com/`），给 kline/clist/sector 三处 `requests.get` 加 `headers=HEADERS`，惠及四象限/资金星图所有东财调用，降低境外风控概率。
- 名称解析降级（决策已确认）：`resolve_stock_name_with_debug` 用 `_resolve_stock_name_safe` 包裹，任何异常返回 (None, debug)，只显示代码不阻断回测（港股名依赖东财 spot，境外同样可能失败）。
- 放弃旧 akshare_loader 的 4h 磁盘缓存（回测低频，不迁移，避免缓存口径分裂）。旧 `fetch_stock_data`/`fetch_data_worker` 暂保留为 legacy，观察稳定后再删。
- 测试：新增 `quant/tests/test_online_market_data.py`（14 用例：market 识别、源名映射、港股成功、tencent→eastmoney fallback、全失败抛可读异常、A股链、空代码校验）全通过；港股 00700 真实端到端命中 Tencent HK 42 行，`prepare_dataframe` 兼容通过。仓库预存在的 pydantic/matplotlib 缺失导致的失败与本次改动无关（已用 git stash 基线比对确认）。
- 已知后续：港股「股票名」二期可作为独立 capability 纳入 Gateway，彻底摆脱 akshare 名称解析的境外脆弱性。
