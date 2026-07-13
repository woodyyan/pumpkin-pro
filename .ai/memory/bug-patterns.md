# Bug Patterns

## BP-001: 跨地域大文件上传超时

**模式**: 低带宽 + 跨地域 + 大文件 → HTTP Client Timeout

**关键参数**:
- 服务器上行带宽: 4Mbps
- 文件大小: ~300MB（SQLite 备份，持续增长）
- 原超时: 60s
- 理论最低传输时间: 300MB / 4Mbps ≈ 600s

**教训**: HTTP Client Timeout 必须根据 **文件大小 / 可用带宽** 上限计算，不能用固定短超时。当文件会增长时，超时应留足余量或动态计算。

**适用场景**: 所有涉及大文件远端上传的功能（COS、S3、SFTP 等）。

## BP-002: 表有记录但关键数值字段全为空

**模式**: 离线数据源回填成功写入大量记录，但下游计算依赖的关键数值字段全为 NULL，导致因子覆盖率为 0。

**案例**: 因子实验室股息率。`factor_dividend_records` 有 5 万+ 分红记录，但 `cash_dividend_per_share` / `total_cash_dividend` 全为空；Phase1 无法计算 `dividend_yield`，Phase2 无法生成 `dividend_yield_score`。

**教训**:
- 回填成功不等于字段可用，必须在任务 summary/Admin 覆盖率中统计关键字段非空数量。
- 对字段名不稳定的数据源，先用独立 probe 脚本确认真实字段，再接入正式解析逻辑。
- 对直接可用指标应保留 source 字段，避免后续口径不可追溯。

**适用场景**: 财务、分红、行业、估值等所有外部源结构化回填。

## BP-003: 聚合接口混用了“最新排行榜”和“旧结果快照”

**模式**: 同一页面同时依赖实时排行榜和离线物化结果，但接口把“当前持仓/当前名单”直接取自旧快照，导致用户看到的当前列表与最新榜单不一致。

**案例**: 行情看板中的卧龙 AI 精选模拟组合。排行榜已经更新到新交易日，但 `quadrant_ranking_portfolio_results` 仍停留在旧批次；API 直接返回旧 `constituents_json`，于是“当前成分股”落后于排行榜一天以上。

**教训**:
- 任何聚合接口都要先区分字段语义：哪些是“历史批次结果”，哪些是“当前可执行状态”。
- 如果历史收益和当前名单来自不同刷新链路，接口必须显式分层返回时间字段，前端必须写清楚生效口径。
- 当“当前名单”已切到新批次但“最近一次调仓/收益曲线”仍是旧批次时，应隐藏或降级旧批次动作，避免用户把旧动作误读为当前建议。

**适用场景**: 榜单 + 模拟组合、信号面板 + 历史回测、缓存快照 + 实时状态等所有“实时视图 + 离线物化结果”混合展示场景。

## BP-004: 用 computed_at 充当用户口径日期

**模式**: 离线任务在凌晨计算收盘后数据，`computed_at` 落在 T+1 日（如 5/22 凌晨 02:00），但数据实际基于 T 日（5/21）收盘数据。前端直接展示 `computed_at` 的日期部分，导致用户误以为数据是 T+1 日收盘后的。

**案例**: 行情看板的风险机会全景图、卧龙 AI 精选排行榜、模拟组合当前成分股，均展示计算完成日期而非真实收盘日期。

**教训**:
- 用户口径日期 ≠ 计算完成时间。任何"日级收盘后产出"的数据，用户看到的日期必须是数据所基于的收盘交易日。
- 不能用 `computed_at - 1 day` 推算收盘日：周末、节假日、A 股/港股交易日差异均无法通过简单减天数处理。
- 必须引入 `source_trade_date` 作为批次级元数据，由交易日历服务按市场查询前一交易日。

**适用场景**: 所有"日级收盘后产出"的离线计算结果展示——策略、榜单、象限、组合、因子快照等。


## BP-005: Docker Compose 变量插值不等于注入容器环境

**模式**: 根目录 `.env` 已配置业务变量，但 Compose 服务未通过 `env_file` 或 `environment` 显式声明这些变量，容器内读取到的仍是代码默认值。

**案例**: 本地密码找回联调时，宿主机 `.env` 已设置 `MAIL_PROVIDER=tencent`，但 `backend` 容器 Compose 配置只映射了部分环境变量，未映射 `MAIL_PROVIDER` 和 `MAIL_TENCENT_*`，最终后端回退到 `mock` provider。

**教训**:
- Compose 的 `${VAR}` 只负责解析 YAML，不代表该变量会自动进入容器。
- 对配置项较多且会持续扩展的后端服务，优先给服务增加 `env_file: .env`，再用 `environment` 覆盖容器内专用值。
- 当日志显示代码命中了默认值，应优先核对“容器实际环境”而不是只看宿主机 `.env`。

**适用场景**: 所有通过 Docker Compose 启动、且依赖大量环境变量的后端服务。

## BP-006: 排行榜表现口径与连续上榜周期不一致

**模式**: 页面同时展示"连续上榜 N 日"与"上榜以来涨幅"，但后端涨幅按历史首次上榜快照计算，连续天数按最新连续快照链计算。股票断档后重新上榜时，用户会看到"连续上榜 1 日"但涨幅来自更早历史周期。

**案例**: 行情看板卧龙 AI 精选排行榜中，天岳先进在 2026-05-27 收盘后数据生成的榜单里显示连续上榜 1 日，但涨幅为 +35.8%。该数字来自 2026-05-11 历史首次快照价到 2026-05-27 收盘价的涨幅，而不是当前连续上榜周期。

**教训**:
- 同一 UI 区块内的周期类指标必须共用同一个时间窗口。
- 排行榜主指标 `return_pct` 应按当前连续上榜周期计算；历史首次入选以来涨幅如需展示，必须拆成独立字段和明确文案。
- 历史快照价格可能受盘中或缺失数据影响；正价格不一定正确，必要时使用历史日线回填命令刷新已有快照价格。

**适用场景**: 排行榜、模拟组合成分股、信号历史表现等所有同时展示"连续/当前周期"与"累计表现"的模块。

## BP-007: 股票池全量成功但附带行业字段严重截断

**模式**: `securities` 股票池刷新返回了完整证券数量，但外部源附带的行业字段只覆盖极小子集；如果直接复用该字段写入下游，行业表看起来“有数据”，实际覆盖率却接近 0。

**案例**: 因子实验室的 `factor_security_industries` 曾只有约 189 条记录，而同批 `factor_securities` 已有 5200+ 条，导致 Phase 2 列表绝大多数股票行业为空。

**教训**:
- 对行业这类关键维度，必须单独校验覆盖量，不能只看股票池总条数。
- `securities` 成功不代表行业成功；行业应有独立 Phase 0 步骤和独立最小覆盖阈值。
- 外部源如果只能返回局部行业，必须继续回退到更稳定的行业专用源，不能把局部结果直接当全量真值。

**适用场景**: 股票池、行业、概念板块等“主记录完整但附带维度缺失”的离线回填链路。

## BP-008: 外部财务接口分表字段漂移导致派生指标覆盖率为 0

**模式**: 财务回填表有大量记录，但外部源的利润表/资产负债表/现金流表字段名或报表名变化，导致跨表 join 的主键列、收入列或现金流列匹配失败；下游派生指标全为空。

**案例**: 因子实验室 FCFM。东方财富 direct API 中现金流表字段为 `NETCASH_OPERATE` / `CONSTRUCT_LONG_ASSET`，利润表当前可用报表为 `RPT_DMSK_FN_INCOME`，主键列为 `SECURITY_CODE`；旧解析仅覆盖中文列名和旧 `RPT_LICO_FN_CPD`，导致 `operating_cash_flow` / `capex` 没有写入，Admin FCFM 覆盖率持续 0%。

**教训**:
- 财务派生指标不能只验证单表字段，必须验证利润表、资产负债表、现金流表三表的主键列和关键数值列都能匹配。
- 外部 direct API 的 `reportName` 需要 fallback；主报表为空时应切换到当前有效报表，而不是直接判定数据源为空。
- `--require-fcfm-inputs` 等修复模式应记录关键字段非空数量，避免“任务成功但关键字段全空”。

**适用场景**: FCFM、ROE、PS、资产权益比、股息率等所有由多张外部财务/分红表组合得到的离线因子。

## BP-009: 修复脚本只校验 FCFM 输入，遗漏同比字段导致 Growth 覆盖率归零

**模式**: 为修复某个派生指标新增“缺字段修复”范围时，筛选条件只覆盖当前修复目标的直接输入列，没有把共用财务快照里的其他关键列一并纳入；后续整批回填会用新源覆盖旧记录，把原本可用的同比字段写成 NULL，导致其他因子覆盖率突然归零。

**案例**: 因子实验室 `repair_missing_fcfm_inputs` 修复 FCFM 后，`factor_financial_metrics` 最新报告期大量记录被 EastMoney `eastmoney:datacenter` 覆盖。该源已经补齐 `revenue`/`operating_cash_flow`/`capex`，但 `revenue_yoy` / `net_profit_yoy` 别名未覆盖 `TOTAL_OPERATE_INCOME_YOY` / `PARENT_NETPROFIT_YOY` 等当前字段；同时增量筛选只检查 FCFM 输入，导致 4361 只股票都被视为“已修复完成”。Phase 1 继续从最新财务行读取空同比值，Admin 中 `earning_growth` / `revenue_growth` 覆盖率直接变成 0%。

**教训**:
- 面向共享财务表的 repair scope 不能只检查单一指标的输入列，必须把同一批次会被覆盖的其他关键字段一并纳入“缺失”判断。
- 外部财务字段别名不仅要覆盖金额列，也要覆盖同比字段；否则“修好了 FCFM”可能顺手打坏 Growth。
- 每次修复财务回填后都要对 `factor_snapshots` 的 `earning_growth`、`revenue_growth`、`fcf_margin` 做批次级覆盖对比，避免单指标修复引入回归。

**适用场景**: 所有共享 `factor_financial_metrics` 或类似公共财务宽表的修复任务，尤其是按“缺字段修复”增量回填的脚本。


## BP-010: 开盘价未到时用收盘价兜底买入价，导致涨幅口径回退

**模式**: 模拟组合改为「T+1 开盘价建仓」后，若在 T+1 9:25 集合竞价开盘价尚未回填时，用最近收盘价兜底充当买入价，会让个股涨幅退回旧的收盘价口径，且与净值曲线口径不一致，用户在开盘前看到的「涨幅」是基于收盘价的假数据。

**案例**: 卧龙 AI 精选模拟组合当前成分股涨幅 = 实时价 / 开盘买入价 − 1。开盘价缺失时必须置 `entry_price_pending=true`、涨幅返回 null，而不是回退收盘价。

**教训**:
- 「建仓价」与「估值价」是不同口径，缺建仓价时必须显式 pending，不能用估值价（收盘价）兜底。
- 实时价可用于「涨幅展示」，但绝不能进净值曲线；曲线只在收盘后用收盘价结算，二者物理隔离。
- 展示日期统一用 `signal_date`/`entry_date`/`valuation_date`，不用 `computed_at`（见 BP-004）。

**适用场景**: 所有「下单价/成交价晚于信号产生」的模拟交易、回测建仓、组合换仓展示场景。

## BP-011: 写入函数依赖「关联表必须非空」作为前置条件，D0 边界触发漏写

**模式**: 某写入函数 A 先查关联表 B 取 latest 行，用于构造 WHERE 条件，再 UPDATE 目标表 C。当 B 表在系统初始化/清库后为空时，`ErrRecordNotFound → continue`，C 表永远不会被写入，不报错也无日志，极难排查。

**案例**: `FillRankingPortfolioEntryOpenPrice` 先查 `quadrant_ranking_portfolio_snapshots` 取 `latest.SnapshotVersion`，再 UPDATE `market_prices`。D0 清库后 snapshots 表为空 → 所有成分股 `open_price` 永远是 0 → 全天 pending。

**教训**:
- 写入函数的前置条件不应依赖「另一张表必须已有数据」，否则任何初始化场景都会静默失败。
- 如果目标表字段本身已能唯一标识行（如 `definition_id + code + exchange`），不要绕道关联表取中间 key。
- `ErrRecordNotFound → continue` 是危险模式：它让空表等价于「跳过所有行」，而不是「暂无数据，稍后重试」。

**适用场景**: 所有先查关联表取版本号/批次号再 UPDATE 的写入路径，尤其是定时 worker 的幂等写入。

## BP-012: 定时 worker 的 09:25 触发点在服务重启后永久错过

**模式**: Worker 启动时从当前时间算「下一个触发点」（strictly after now），如果服务在 09:25 之后重启，09:25 点已过，worker 直接跳到下一个时间点（09:30）。09:30 触发时 `isOpenAuctionPoint` 返回 false，开盘价永远不会被填入，当天全天 pending。

**案例**: `RealtimeWorker.scheduleLoop` 调用 `nextRealtimeTriggerAt(now, points)` 找严格大于 now 的第一个点。服务在 09:40 重启 → 09:25 点被跳过 → `fillOpen=false` → `FillRankingPortfolioEntryOpenPrice` 当天从不被调用。

**教训**:
- 对「只触发一次」的关键时间点，必须在 Worker.Start() 里加启动补偿：检测当天是否已错过该点且尚未填入数据，若是则立即补触发一次。
- 补偿逻辑应设置合理的时间窗口上限（如 10:30），超出窗口后市场价已变动较大，补填的「开盘价」意义不大。
- 补偿触发后立即 `markRun`（与 fetch 结果解耦），防止并发重复触发。

**适用场景**: 所有「每日仅在固定时间点触发一次、且触发结果影响全天展示」的 cron/ticker 任务。

## BP-013: 补齐函数依赖「后继数据行存在」来推断写入目标，初始态永远 pending

**模式**: 补齐函数需要推断「下一个交易日」，方法是查数据库里 `date > currentDate` 的下一条记录。当数据库里只有最新一条（D0、或某市场刚开始运行）时，没有后继行 → `ErrRecordNotFound → entryDate="" → 跳过`，每次触发都 pending，且不报错，极难感知。

**案例**: `resolveEntryTradeDateForSnapshot` 通过查 `snapshot_date > snapshotDate` 的后继 snapshot 来推断 T+1。D0 当天及港股只有一条 snapshot 时，后继不存在 → 所有成分股全天补不进开盘价。

**教训**:
- 「用后继行推断下一日期」是脆弱的：任何初始态、最新态都会触发 ErrRecordNotFound。
- 正确做法：加今日（北京时间）兜底——当没有后继且 `snapshotDate < today` 时，today 就是 T+1。
- 必须区分两种 not-found：「后继行还没生成，但 T+1 已经到了」vs「T+1 本身还没到」。用 `snapshotDate < todayBJ` 做守卫来区分。
- 同类场景：任何「用表中下一行推断时间偏移」的逻辑，都应该有「当前行是最新行时用系统时间兜底」的保护。

**适用场景**: 所有数据驱动的「下一个交易日」推断逻辑，尤其是在系统刚上线、数据刚初始化、或特定市场数据不足时。

## BP-014: 低频外部源仍放进 nightly 全量链路，导致 Phase0 反复超时或阻断

**模式**: 某些外部数据天然低频更新，但仍被放进 nightly 全量预计算链路；一旦接口变慢或单点失败，就会把本来高频必需的数据刷新一起拖慢甚至直接卡死。

**案例**: 因子实验室 `dividends` 每晚逐股全量刷新，数据更新频率却通常只有年报/半年报/分红实施窗口。结果 nightly Phase0 经常在 `dividends` 上耗尽 1800 秒超时预算，最新快照停在多天前。

**教训**:
- 先按业务更新频率给离线数据分层：高频必需的放 nightly，低频资料型数据改为手动或低频定时。
- 外部源失败时要区分“必须阻断”和“允许沿用旧结果”，不能把所有 Phase0 子任务都当成同等级 critical。
- admin 页面要同时暴露“最近状态”和“最近成功刷新时间”，否则运维只看到 exit code，看不到是否还能安全沿用旧数据。

**适用场景**: 分红、行业、财务附加维度、公司静态资料等依赖外部源但更新频率显著低于日线/指数/证券池的离线任务。

## BP-016: LLM 调用方人为设置过小的 max_tokens，导致长 JSON 输出被截断

**模式**: 调用方在请求体中显式设置 `max_tokens` 固定值；当 schema / prompt 演进后输出天然变长，provider 正确执行限制触发 `finish_reason=length`，错误看起来像"模型失效"，实际是配置失效。

**案例**: AI Picker 设置 `max_tokens=4096`。候选池扩展至 60+ 条 × 每条 20+ 字段 + 每只股票要求 3-5 句中文理由后，完整 JSON 持续超过 4096 tokens，每次手动生成均报错"AI 输出超长被截断"。

**教训**:
- 对"每日一次 + 长结构化输出"的低频服务，优先选择"不设 max_tokens、交由 provider 默认"而非人为封顶。
- 提高 max_tokens 只是治标，根本出路是理解 provider 能力边界，并配套提高 HTTP timeout。
- finish_reason=length 的重试无意义：同一请求在同一 provider 下再试仍会被截断；此类错误应直接失败，附带可观测信息（completion_tokens / prompt_chars）便于人工研判。
- 发现 finish_reason=length 后，先核查"调用方是否设置了 max_tokens"，再考虑优化 schema。

**适用场景**: 所有 LLM 调用中使用固定 `max_tokens` 且输出内容会随业务演进而增长的场景（选股、报告生成、长文分析等）。



**模式**: 主候选池本身可正常生成，但后续增强步骤（如技术快照、外部画像、补充标签）失败后，代码把增强结果与主池做硬 join，导致所有候选项被一起过滤掉，最终报错与真实故障点不一致。

**案例**: AI Picker 因子 recall 已能收敛出 60+ 只候选股，但 `aipicker_technical_snapshots` 缺失或生成失败后，`buildCandidatePool()` 只保留有技术快照的股票，最终把真实问题表现成“候选池为空”。

**教训**:
- 增强型数据默认应是软依赖，除非产品明确要求为硬门槛。
- 如果增强步骤失败，日志和 prompt 中都要暴露“部分增强数据缺失”，不能把错误折叠成主链路为空。
- 当增强数据可空时，结构体字段也应支持 `nil`，避免用默认 0 值伪装成真实指标。

**适用场景**: 候选池 + 技术面、候选池 + 公司画像、实时列表 + 补充标签等所有"主结果 + 增强信息"组合链路。

## BP-015: 模拟组合事实表同步缺少开盘价补偿与失败可观测性

**模式**: 新口径 `/portfolio-tracking` 事实表链路依赖 legacy `quadrant_ranking_portfolio_market_prices.open_price` 判断建仓开盘价是否就绪；但开盘价填充只在 09:25 实时 worker 中执行，如果 worker 错过窗口或 market price 行未先生成，开盘价永远为 0，导致事实表只停留在 seeded baseline，页面永久显示「等待下一交易日开盘价」且不报错。

**案例**: 2026-07-01 排查发现 4 个模拟组合 `portfolio_daily` 只有 seeded baseline（2026-06-29），`portfolio_position/trade/metrics` 全空；`quadrant_ranking_portfolio_market_prices` 最新批次 `open_price` 全为 0，当前信号成分股无 market price 行；但没有任何失败状态或日志暴露这一缺口。

**教训**:
- 事实表同步链路不能只依赖外部条件成立才推进；sync/recompute 内部必须先尝试补齐缺失前置数据（如开盘价），再推进事实表写入。
- admin 状态必须同时暴露前置价格缺口和事实表完整度（missing_open_price_count / missing_close_price_count / baseline_only / fact row counts / can_sync），否则运维只看到 pending 或 seeded，却不知道是缺价格还是缺同步。
- 缺建仓价时必须显式 pending，不能用收盘价或实时价兜底（见 BP-010）。
- 补开盘价应作为独立 admin 运维动作，不能只藏在同步内部；运维需要先补价、再同步、最后校验的分步恢复路径，并明确“补价不生成持仓/调仓”。
- verify 校验不能只查资产一致性，还要检查 buy_price > 0 等关键字段完整性。

**适用场景**: 所有依赖"前置数据就绪才能推进"的离线事实表链路，尤其是建仓价/收盘价/信号数据驱动的模拟组合、回测建仓、策略换仓场景。

## BP-017: 用已有快照序列推断交易日导致缺口静默跳过

**模式**: 模拟组合用 `snapshot_date > currentDate` 的下一条排行榜快照推断 T+1 交易日。当某交易日四象限或排行榜快照失败时，该日期不会出现在快照序列中，系统会把缺失交易日静默跳过；当市场休市时，又可能把自然日或错误快照日当作交易日，误报缺开盘价/收盘价。

**教训**:
- 交易日必须来自市场交易日历，不得由已有业务快照反推。
- 缺少应有快照是 pipeline 阶段错误，必须显式标记为 `missing_signal` / `blocked`，不能从日期序列中消失。
- 休市日应标记 `skipped`，不应生成价格需求，也不应报缺价格。

**适用场景**: 所有按交易日推进的离线策略、模拟组合、回测建仓、排行榜和收盘后事实表链路。

## BP-018: 重建市场丢弃已修复价格需求导致回退为缺失

**模式**: `ReplaceSimPortfolioV2PriceRequirements` 在重建市场（`ApplyStartDate` → `Run`）时执行破坏性 `DELETE + INSERT`，把之前通过 `BackfillDailyBars` 或 `RetryResolvePrices` 修复为 satisfied 的价格需求行全部丢弃，重新插入 pending 行。同时 `resolveSinglePriceRequirement` 解析链不含 `dailyBarFetcher`，无法从腾讯 API 兜底，导致重建后价格回退为 missing。

**案例**: Admin 模拟组合板块选择 7 月 1 日作为市场开始信号日 → 7 月 2 日显示 `688220 valuation_close` 缺失 → 点击「重拉该日缺失价格」价格变正常 → 点击「确认应用并重建该市场」→ 7 月 2 日同一股票同一价格类型再次变为缺失。

**根因链**:
1. `ReplaceSimPortfolioV2PriceRequirements`（repository.go:132）盲删所有行再重新插入 pending 行，不保留已 resolved 的 satisfied 行。
2. `resolveSinglePriceRequirement`（service.go:442）解析链：admin_override → priceLookupResolver → priceResolver，三者都查本地 `closing_snapshots` 表，不含 `dailyBarFetcher`（腾讯日线 API）。
3. `BackfillDailyBars` 写入的是临时性 `price_requirements` 行（会被 Replace 重建），而非持久性 `price_overrides` 表（重建后仍生效）。

**修复**:
1. `ReplaceSimPortfolioV2PriceRequirements` 改为先查出已有 satisfied 行 → 删除全部 → 合并插入（satisfied 行保留原状态，新行插入 pending）。
2. `resolveSinglePriceRequirement` 在 priceResolver 之后增加 `dailyBarFetcher` 兜底，source 标记为 `daily_bar_fallback`。

**教训**:
- 任何 `Replace*` / DELETE+INSERT 模式在涉及"已修复/已解析"状态时，必须保留 satisfied 状态的行，不能盲删。
- 修复写入路径（BackfillDailyBars）和正常解析路径（resolveSinglePriceRequirement）的数据源必须一致或可互通，否则修复结果会在下一次正常流程中丢失。
- 临时性数据（price_requirements）和持久性数据（price_overrides）要区分清楚：需要跨重建生效的修复结果应写入持久表，或确保重建逻辑保留临时表中的已修复行。

**适用场景**: 所有含"修复 → 重建"循环的 pipeline，尤其是价格需求管理、信号快照重建、事实表重算场景。

## BP-019: 旧链路重写为新结构体时，字段迁移遗漏（隐式默认值掩盖错误）

**模式**: 从旧数据结构（`rankingPortfolioDefinitionSpec` + `[]string` 字段）重写为新结构体字面量（`SimPortfolioV2Definition{...}`）时，逐字段手工搬运，遗漏了非核心字段；由于该字段在 Go 里零值为空字符串，且数据库列有 `default:'[]'`，问题不会在编译期或首次运行时报错，而是悄悄以"空排除名单"的形式落库，长期不被发现。

**案例**: `sim_portfolio_v2_repository.go` 的 `defaultSimPortfolioV2Definitions()` 在 v2 重构首个提交（`0b3db1d`，2026-07-04）就只搬运了 `SelectionRule`/`SelectionWindow`/`MaxHoldings`，遗漏了 A 股组合 A/B 的 `ExcludedBoards: mustMarshal([]string{aShareBoardStar})`（科创板排除名单）。旧版 `portfolio_service.go` 里这个字段是写了的。此后 4 次相关提交（日历驾驶舱、价格修复等）都没有再碰这段定义代码，缺陷持续到被人工排查发现（2026-07-06）。修复方式：为 `spv2_ashare_a`/`spv2_ashare_b` 显式补上 `ExcludedBoards`；港股两个组合本身不涉及科创板，保持不变。

**教训**:
- 结构体重写/新旧链路切换时，必须显式对照"旧字段 → 新字段"迁移映射表，而不是凭记忆手写字面量，尤其是非"看起来核心"的字段（如排除名单、白名单类配置）。
- 对有零值 + DB 默认值兜底的字段，团队应养成"关键配置一律显式赋值，不依赖隐式默认"的习惯，防止遗漏被悄悄"正确掩盖"。
- 新链路上线后应有一个针对"默认配置定义"本身的快照/断言测试（不依赖 pipeline 运行，只校验 `defaultXxxDefinitions()` 返回值），能在 CI 阶段直接捕获此类遗漏。

**适用场景**: 任何"v1 → v2 全量重写数据定义/配置" 的重构任务，尤其是选股规则、白名单/黑名单、排除条件等业务口径类配置字段。

## BP-020: 单连接池 + 长排他操作导致连接池饿死

**模式**: SQLite（或任何单写多读数据库）连接池 `MaxOpenConns=1` 时，如果有一个操作长时间持有该唯一连接（如 `VACUUM INTO` 791MB 数据库耗时 30s+），所有其他 DB 查询将排队等待，API 请求因无法获取 DB 连接而超时，最终被反向代理（如 Cloudflare 524）边缘超时。

**案例**: 2026-07-08 生产事故。四象限计算（`QUADRANT_COMPUTE_HOUR=20`）完成后异步触发 backup，backup 的 `VACUUM INTO` 通过共享的 `s.db` 连接执行，独占唯一连接 30s+。所有 API 请求排队等待 ~300s，Cloudflare 边缘 524 超时。事故窗口 UTC 12:55~13:02（北京 20:55~21:02）与 quadrant 计算时间吻合。

**根因链**:
1. `MaxOpenConns=1`：连接池只有 1 个连接，任何长操作都会阻塞所有其他操作。
2. VACUUM 复用主连接：`hotBackupPumpkin` 使用 `s.db`（共享连接）执行 `VACUUM INTO`，而非创建独立连接。
3. 无 HTTP 超时：`http.ListenAndServe` 无显式超时，被阻塞的请求无限期等待，goroutine 堆积。
4. 异步任务无超时：异步 goroutine 使用 `context.Background()`，无超时上限。

**修复**:
1. VACUUM 改用独立 `gorm.Open` 连接（`store/backup/service.go`）。
2. `MaxOpenConns` 1→4，`MaxIdleConns` 1→2，`busy_timeout` 5s→15s（`store/gorm.go`）。
3. HTTP Server 加显式超时：`ReadHeaderTimeout=10s`、`ReadTimeout=30s`、`WriteTimeout=120s`、`IdleTimeout=120s`（`main.go`）。
4. 异步 goroutine 加 `context.WithTimeout(..., 10*time.Minute)`（`store/quadrant/service.go`）。

**教训**:
- 任何全库级排他操作（VACUUM、DDL、大批量迁移）绝不能在共享连接池上执行，必须使用独立连接。
- `MaxOpenConns=1` 对 WAL 模式的 SQLite 过于保守——WAL 支持并发读，应设 `MaxOpenConns>=4` 以允许读操作在写事务进行时继续。
- HTTP Server 必须设置显式超时，否则被阻塞的请求会无限堆积 goroutine，把 DB 问题放大为 OOM。
- 异步 goroutine 即使脱离请求 context，也必须有自己的超时，防止外部依赖故障导致永久阻塞。
- 定时任务（如 backup、quadrant compute）的时间窗口可能重叠，必须确保它们不会通过共享资源（连接池、文件锁）互相饿死。

**适用场景**: 所有使用 SQLite WAL 模式 + 连接池 + 定时长操作（backup、VACUUM、批处理）+ HTTP API 的生产架构。

## BP-021: Python 函数默认参数在定义时绑定，导致 monkeypatch 配置不生效

**模式**: 类方法 `def __init__(self, db_path: str = CACHE_DB_PATH)` 的默认值 `CACHE_DB_PATH` 在**模块加载（函数定义）时**即被求值并冻结。后续即使测试/运行时 `monkeypatch.setattr(mod, "CACHE_DB_PATH", tmp)` 改了模块属性，`super().__init__()`（不带参数）仍会使用旧的、定义在导入时刻的真实路径。表现：测试本想用临时 db 做隔离，实际悄悄读写生产/真实 db，造成测试间数据污染与「缓存看似有数据」的假象。

**案例**: 2026-07-12 修复四象限日线缓存降级测试时发现：`test_a_share_full_refresh_raises_with_dominant_reason_when_cache_empty` 期望空缓存报错，却误读真实 db（含上一测试残留的 8 只股票）→ 缓存覆盖率 80% → 走降级而非报错。

**教训**:
- 配置型默认值（路径、阈值、开关）不要在签名里直接写模块全局，改为 `def __init__(self, db_path: Optional[str] = None): self.db_path = db_path or CACHE_DB_PATH`，在**调用时**解析。
- 任何依赖 monkeypatch 做路径隔离的测试，都要先确认被 patch 的全局真的会被读路径使用（而非被冻结的默认值）。
- 此类 bug 在 pytest 下极易被「恰好真实 db 有数据」掩盖，CI 换机器/空 db 时才会暴露。

**适用场景**: 所有在 `__init__`/函数签名里引用模块级配置常量（路径、env、全局开关）的 Python 代码，尤其是带缓存/DB 连接的类。

## BP-022: SQLite WAL 模式下「同进程独立连接」数据不可见 + 空缓存全量刷新死循环

**模式**（两个相关联的坑，均出现在四象限 `DailyBarCache`）：
1. **WAL 跨连接不可见（实测）**: 同一进程内，一个 `sqlite3.connect` 连接 `commit()` 写入的数据，另一个**独立打开**的连接（即使同一 db 文件、WAL 模式）在读取时可能看不到（daily_bars 行数返回 0）。`meta` 表相对更易见，但不可依赖。生产无影响（每个运行用单一 cache 实例、进程间运行不共享）。**测试不能靠「先开连接 A 写、再开连接 B 读」来验证**。
2. **空缓存全量刷新死循环**: 旧 `needs_full_refresh()` 逻辑是 `count==0 → return True`，而失败的全量刷新从不写缓存 → 下次又是空缓存 → 又全量 → 又被限流。腾讯限流场景下形成永久死循环。

**案例**: 2026-07-12。admin 手动跑四象限，腾讯主源限流，全量刷新成功率 17%(A)/0%(HK)，旧逻辑直接 `RuntimeError` 且缓存从未写入，每次都重新全量。

**教训**:
- 测试需要预置缓存数据时，**在被测函数自身使用的同一个 cache 实例内**写（让 cache 子类在 `__init__` 里 `set_stock_bars` + `save`，并捕获该实例验证副作用），不要另开连接预置。
- 全量刷新失败也要 `cache.save()` + `cache.mark_full_refresh()`，把「已尝试全量」记下来；`needs_full_refresh()` 应尊重「近期已标记全量」→ 返回 `False` 走增量，逐步补齐缺口而非反复全量。
- 增量模式不要因部分失败就强制回退全量（同样会死循环）。
- 阈值不足时：缓存覆盖 ≥80% → 降级继续（缺失股票中性分 50 参与）；覆盖仍不足 → 报错文案带「主数据源异常」+ dominant provider reason，便于定位。

**适用场景**: 任何带本地 SQLite 缓存、需要 full/incremental 刷新、且上游可能限流的离线计算任务。
