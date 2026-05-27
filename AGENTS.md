# AGENTS.md

本文件是 AI 进入本仓库时的入口说明。开始任何改动前，先按本文读取 `.ai/` 知识库；不要把 `.ai/` 当作可忽略的备注目录。

## 项目概览

Pumpkin Pro / 卧龙 AI 量化交易台是一个投研与量化交易工作台：

- `frontend/`: Next.js 14 + React 18 + Tailwind CSS 前端。
- `backend/`: Go API 与业务服务。
- `quant/`: Python 量化、行情、因子与回测计算服务。
- `.ai/`: 给 AI 使用的项目知识库、长期约束、历史经验和功能规格。

## 必读顺序

### 1. 长期上下文：每次任务都先读

先读取 `.ai/context/` 下的文件。这些内容是长期有效的项目约束，优先级高于临时猜测。

- `.ai/context/architecture.md`: 项目架构、技术栈、关键前端页面与组件。
- `.ai/context/coding-standards.md`: 编码规范，尤其是前端主题、颜色 token 与图表颜色规则。
- `.ai/context/domain-knowledge.md`: 业务语义和领域规则，例如交易日历、收盘后数据日期、账号安全。
- `.ai/context/glossary.md`: 术语定义。遇到 `source_trade_date`、`computed_at`、`snapshot_date` 等字段时必须按这里解释。
- `.ai/context/decisions.md`: 已做出的设计决策。除非用户明确要求重新设计，否则不要违反这些决策。

### 2. 长期记忆：改动前按相关性读取

读取 `.ai/memory/` 中与任务相关的文件。当前至少包含：

- `.ai/memory/bug-patterns.md`: 历史 bug 模式和工程教训。

如果任务涉及以下场景，必须检查该文件：

- 离线批处理、快照、排行榜、因子、收盘后数据展示。
- Docker Compose 环境变量、邮件配置、部署配置。
- 外部数据源回填、字段覆盖率、关键数值字段可用性。
- 大文件上传、备份、远端存储。

### 3. 当前任务规格：按任务匹配读取

`.ai/specs/` 是按功能或需求拆分的任务包。它不是全部都要读；先根据用户需求和代码影响面匹配相关目录。

常见文件语义：

- `requirement.md`: 用户需求、目标、约束和验收口径。
- `design.md`: 方案设计、模块边界、数据流。
- `tasks.md`: 待办拆分，可作为实现 checklist。
- `review.md`: 评审摘要、关键风险和范围确认。
- `changelog.md`: 已落地变更和用户可见行为。

当前已有规格目录：

- `.ai/specs/password-reset/`: 邮箱找回密码、重置 token、邮件 provider、session 撤销。
- `.ai/specs/source-trade-date/`: 日级收盘后数据统一展示 `source_trade_date`，禁止用 `computed_at` 充当用户口径日期。
- `.ai/specs/theme-mode/`: 前端浅色/深色/跟随系统主题方案。
- `.ai/specs/factor-lab-manual-apply/`: 因子实验室百分比权重输入与手动应用。

如果用户请求明显属于某个 spec，先读该目录下所有 markdown，再改代码。如果用户请求是新功能且没有对应 spec，可以参考现有 spec 的格式补充需求文档，但只有在用户要求或任务确实需要沉淀时才新增。

## 优先级规则

当信息冲突时，按以下顺序处理：

1. 用户本轮明确指令。
2. `AGENTS.md` 中的入口规则。
3. `.ai/context/` 的长期规则和术语定义。
4. 匹配到的 `.ai/specs/<feature>/` 当前任务规格。
5. `.ai/memory/` 的历史经验。
6. 代码现状和局部惯例。
7. README 或其他通用文档。

如果用户指令与长期规则冲突，先指出冲突和可能影响，再按用户确认后的方向执行。

## 工作流程

1. 识别任务影响面：前端、后端、量化服务、部署配置、数据语义或文档。
2. 按“必读顺序”读取相关 `.ai/` 文件。
3. 用代码现状验证知识库信息，避免只依赖文档。
4. 小步修改，保持既有目录结构、命名和测试风格。
5. 根据影响面运行最小必要验证。
6. 如果发现新的长期规则、设计决策或 bug 模式，优先向用户说明；在用户同意或任务要求时，再更新 `.ai/`。

## 关键工程约束

### 前端

- 颜色和主题必须遵守 `.ai/context/coding-standards.md`。
- 默认使用语义化 Tailwind token，例如 `text-foreground-muted`、`bg-background`、`bg-card`、`border-border`。
- 不要新增大面积硬编码 `text-white/60`、`bg-slate-900`、`border-white/10` 等颜色。
- 图表颜色应根据 `useTheme().resolvedTheme` 在 JS 中选择。
- 主题切换使用 `darkMode: "class"` 与 CSS 变量方案，不要改回 `media` 或重复铺 `dark:` class。

### 数据日期与交易日

- 用户口径日期必须优先使用 `source_trade_date`。
- `computed_at` 是计算完成时间，只用于内部运维和排障。
- 不允许用 `computed_at - 1 day` 推算收盘日。
- 涉及 A 股、港股前一交易日时，必须考虑市场维度交易日历。

### 后端与配置

- Docker Compose 中 `.env` 变量插值不等于注入容器环境；新增后端环境变量时检查 `env_file` 或显式 `environment`。
- 邮件、账号安全、密码找回应遵守 `.ai/context/domain-knowledge.md` 和 `.ai/specs/password-reset/` 中的限制。
- 涉及 token 消费、密码更新、session 撤销等安全流程时，保持事务一致性和单次消费语义。

### 外部数据与离线计算

- 回填成功不等于字段可用；涉及外部源时要验证关键字段非空覆盖率。
- 聚合接口要区分“当前状态”和“历史批次结果”，不要把旧快照伪装成当前列表。
- 日级离线结果应携带批次级元数据，包括 `source_trade_date` 和 `computed_at`。

## 常用验证命令

按改动范围选择最小必要命令：

```bash
cd backend && go test ./...
```

```bash
cd frontend && npm test
```

```bash
cd frontend && npm run build
```

```bash
cd quant && pytest
```

如果依赖未安装或命令因环境限制失败，在最终回复中说明未验证项和原因。

## 修改 `.ai/` 的规则

- `.ai/context/`: 只放长期有效、跨任务复用的项目规则和知识。
- `.ai/memory/`: 只放从真实问题中总结出的 bug 模式、排障经验和工程教训。
- `.ai/specs/`: 放具体功能的需求、设计、任务拆分、评审和变更记录。
- 不要把一次性执行日志、临时思考、无复用价值的内容写进 `.ai/`。
- 更新 `.ai/` 时保持中文表达、结构化标题和可执行约束。

## 给 AI 的默认行为

- 不确定字段语义时，先查 `.ai/context/glossary.md`。
- 不确定业务口径时，先查 `.ai/context/domain-knowledge.md`。
- 不确定前端样式写法时，先查 `.ai/context/coding-standards.md`。
- 不确定是否已有类似坑时，先查 `.ai/memory/bug-patterns.md`。
- 不确定当前功能范围时，先在 `.ai/specs/` 中寻找匹配目录。
- 不要绕过 `.ai/` 直接按通用经验重构项目。
