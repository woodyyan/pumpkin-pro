# 卧龙AI量化交易台（Wolong Pro）

> 官网直达：<https://wolongtrader.top/>

一个面向个人研究者与量化开发者的 AI 投研与量化交易工作台。项目已经不再只是“回测页面”，而是围绕 **研究选股 -> 个股分析 -> 策略构建 -> 历史回测 -> 持仓跟踪 -> 信号推送 -> 后台运维** 形成了完整闭环。

当前版本覆盖的核心场景包括：A 股 / 港股行情看板、AI 个股分析、自然语言选股、风险机会四象限、卧龙排行榜、策略库与 AI 策略生成、历史回测、持仓管理、因子实验室、Webhook 信号推送，以及管理员后台的 AI 配置、计算任务、反馈、备份和系统健康管理。

## 功能特性

### 1. 行情看板与个股研究

- 支持 A 股与港股关注池，按股票卡片查看行情快照
- 支持个股详情页查看 K 线、均线、MACD、布林带、成交量等技术面
- 支持支撑位 / 压力位识别、基础面指标、公司资料与相关新闻摘要
- 支持保存 AI 分析历史，并回看历史判断与后验表现

### 2. AI 投研与分析能力

- 支持对单只股票发起 AI 分析，综合行情、技术面、基础面、新闻和持仓上下文生成结论
- 支持自然语言 AI 选股，将口语化条件解析为结构化筛选条件
- 支持 AI 生成策略说明与参数建议，并联动回测验证
- 支持回测结果 AI 优化建议，帮助迭代参数与策略思路

### 3. 全市场筛选与机会发现

- 支持 A 股与港股两套选股器筛选维度与示例查询
- 支持价格、市值、PE、PB、换手率、量比、振幅、区间涨跌幅等多维过滤
- 支持保存和管理自定义自选表
- 支持风险机会四象限、卧龙排行榜、排行榜组合结果等机会发现入口

### 4. 策略库与历史回测

- 支持策略库管理，包含草稿 / 启用 / 归档等状态
- 内置多类策略模板：双均线、网格、布林均值回归、RSI、MACD、放量突破、双重确认、布林 + MACD 组合
- 支持在线行情、本地 CSV、示例行情三类回测数据源
- 支持查看总收益率、买入持有收益、超额收益、年化收益、最大回撤、夏普比率、胜率、手续费等指标
- 支持图表化查看 K 线叠加指标、买卖点、权益曲线、回撤和历史运行记录

### 5. 持仓管理与交易信号

- 支持按股票记录买入、卖出、调均价等持仓事件，并可撤销最近操作
- 支持组合总览、资产曲线、盈亏日历、仓位分布、收益归因与风险指标
- 支持按 A 股 / 港股 / 全部组合范围查看组合表现
- 支持为单只股票配置策略信号，并通过企业微信 / 飞书 Webhook 推送
- 支持信号冷却、交易时段限制、送达记录和最新投递状态追踪

### 6. 因子实验室与后台运维

- 提供因子实验室页面，基于预计算快照对 A 股股票池做多因子打分与排序
- 支持价值、股息率、成长、质量、动量、规模、低波动等因子权重组合
- 提供管理员后台，用于查看 AI 配置、AI 用量、设备分析、用户漏斗、反馈列表、系统健康、备份状态
- 支持后台触发四象限计算、因子流水线、公司资料刷新与数据库备份任务

### 7. 分层服务架构

- **`frontend/`**：Next.js 前端界面，承载研究、选股、策略、回测、持仓、设置和管理后台等页面
- **`backend/`**：Go API 网关与业务服务，负责认证鉴权、数据持久化、组合管理、信号投递、后台任务与管理接口
- **`quant/`**：Python FastAPI 量化服务，负责行情抓取、选股扫描、策略执行、回测计算、四象限与因子相关计算

## 技术栈

- **前端**：Next.js 14、React 18、Tailwind CSS、Lightweight Charts
- **后端**：Go、GORM、HTTP API
- **数据库**：SQLite（默认）/ PostgreSQL（可选，`DB_TYPE=postgres`）
- **量化服务**：Python、FastAPI、Pandas、NumPy、AkShare
- **测试**：Node `--test`、Go test、Pytest
- **部署方式**：Docker Compose / 本地开发模式

## 项目架构

```text
pumpkin-pro/
├── frontend/   # Next.js 前端，含行情、选股、策略、回测、持仓、后台页面
├── backend/    # Go API 网关与业务服务
├── quant/      # Python 量化与数据计算服务
├── data/       # SQLite、缓存、备份等运行时数据
├── docker-compose.yml
└── README.md
```

## 快速安装

### 环境要求

建议准备以下环境：

- Node.js 18+
- npm 9+
- Go 1.21+
- Python 3.10+
- Docker / Docker Compose（可选）

### 方式一：使用 Docker Compose 启动（双配置）

当前仓库同时提供两套 Compose 文件，分别用于“本地构建运行”和“拉取已发布镜像运行”。这两套文件可以并存使用，不冲突。

| 文件 | 适用场景 | 镜像来源 | 推荐命令 |
| --- | --- | --- | --- |
| `docker-compose.local.yml` | 本机开发、需要本地改代码后立即构建验证 | 本地 `Dockerfile`（`build`） | `docker compose -f docker-compose.local.yml up --build` |
| `docker-compose.yml` | 预发/生产或希望与 CI 发布镜像保持一致 | 腾讯云 TCR（`image`，生产模式启动） | `docker compose -f docker-compose.yml up -d` |

在首次启动前，建议先复制环境变量模板：

```bash
cp .env.example .env
```

`backend` 服务现在会通过 `env_file: .env` 直接把根目录 `.env` 注入容器。Compose 文件中的 `environment` 项仍保留用于覆盖容器内专用值，例如 `QUANT_SERVICE_URL=http://quant:8000` 与 `BACKEND_CALLBACK_URL=http://backend:8080`。

如果你使用本地构建模式，请在项目根目录执行：

```bash
docker compose -f docker-compose.local.yml up --build
```

如果你使用镜像拉取模式，请在项目根目录执行：

```bash
docker compose -f docker-compose.yml up -d
```

当 `docker-compose.yml` 指向私有 TCR 镜像时，需要先完成登录：

```bash
echo "$TCR_PASSWORD" | docker login ccr.ccs.tencentyun.com -u <tcr-username> --password-stdin
```

`docker-compose.yml` 默认会读取以下镜像变量（可在 `.env` 覆盖）：`IMAGE_REGISTRY`、`IMAGE_NAMESPACE`、`IMAGE_TAG`。前端容器会以 `NODE_ENV=production` 启动并执行 `next start`，默认开启 Next.js 压缩。默认数据库为 SQLite，数据库文件持久化在宿主机 `./data/pumpkin.db`。如果需要切换 PostgreSQL，只需在 `.env` 中设置 `DB_TYPE=postgres` 并补全连接参数。

无论采用哪套 Compose 文件，服务默认端口保持一致：前端 `http://localhost:3000`，后端 `http://localhost:8080`，量化服务 `http://localhost:8000`。

#### 服务端构建速度优化说明

本仓库已在 `frontend/`、`backend/`、`quant/` 下补充 `.dockerignore`，并优化了 Dockerfile 的依赖缓存层。首次在服务器构建仍可能较慢（冷缓存 + 依赖下载），但后续增量构建通常会明显加快。

如果你的目标是稳定且更快的服务器部署，建议优先使用 `docker-compose.yml` 的“拉取镜像模式”（CI 预构建后服务器仅 `pull`）；仅在需要现场编译验证时使用 `docker-compose.local.yml` 本地构建模式。

#### 本地镜像发布脚本（Phase 2）

仓库已提供本地发布入口 `ops/local/release.sh`，用于先在本机完成镜像构建，并按统一 tag 推送到 TCR。Phase 2 默认仍采用串行构建，但已经支持按服务选择发布，并对 `backend` 拆分为 `base image + app image` 两层结构。

其中：

- `backend/Dockerfile.base`：构建后端基础镜像 `pumpkin-base:1.0`
- `backend/Dockerfile`：基于 `pumpkin-base:1.0` 构建业务镜像 `pumpkin-pro-backend:<tag>`
- `backend/factorlab-requirements.txt`：沉淀后端稳定 Python 依赖，降低业务镜像构建成本

常用命令：

```bash
# 1) 先做 dry-run，检查参数、tag、builder、基础镜像与 manifest 输出
sh ops/local/release.sh --services backend --build-only --dry-run

# 2) 强制先构建 backend 基础镜像，再构建 backend 应用镜像
sh ops/local/release.sh --services backend --build-only --build-base

# 3) 构建并 push backend + frontend 到 TCR
export TCR_USERNAME="<your-tcr-username>"
export TCR_PASSWORD="<your-tcr-password>"
export IMAGE_NAMESPACE="pumpkin-pro"
sh ops/local/release.sh --services backend,frontend --push

# 4) 构建并 push 全部服务
sh ops/local/release.sh --all --push
```

脚本行为约定：

- 支持服务：`backend`、`frontend`、`quant`
- 默认 tag 规则：`release-时间-shortsha`
- 默认 registry：`ccr.ccs.tencentyun.com`
- 默认 namespace：`pumpkin-pro`
- 默认 builder：`default`
- manifest 输出目录：`.release/manifests/<tag>.json`
- 当前阶段支持 `--build-only`、`--push`、`--dry-run`、`--services`、`--all`
- 当前阶段支持 `--builder` 显式固定 buildx builder
- 当前阶段支持 `--build-base` 强制重建基础镜像，支持 `--skip-base-build` 跳过自动基础镜像构建
- 当前阶段 `--parallel` 仅保留参数，不启用并行构建

backend 基础镜像策略：

- 默认基础镜像名：`ccr.ccs.tencentyun.com/<namespace>/pumpkin-base:1.0`
- 当构建 `backend` 且本地不存在该基础镜像时，脚本会自动先构建基础镜像
- 如果你想显式重建基础镜像，使用 `--build-base`
- 如果你已提前准备好基础镜像并希望跳过自动构建，使用 `--skip-base-build`

建议测试顺序：

1. 先执行 `--build-only --dry-run`，确认镜像名、tag、builder、manifest 路径符合预期
2. 再执行 `--build-only --build-base`，确认 backend base image 与 app image 都能成功构建
3. 最后执行 `--push`，验证 TCR 登录与上传

如果你已经提前 `docker login` 过 TCR，脚本也支持复用现有登录态；否则请通过 `TCR_USERNAME` / `TCR_PASSWORD` 或同名命令参数显式传入。

### 方式二：本地开发模式启动

#### 1. 启动量化服务

```bash
cd quant
python3 -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
python3 main.py
```

#### 2. 启动 Go 后端

```bash
cd backend
# 默认 SQLite（data/pumpkin.db）
go run main.go

# 如需切换 PostgreSQL
# DB_TYPE=postgres DB_HOST=127.0.0.1 DB_PORT=5432 DB_USER=postgres DB_PASSWORD=postgres DB_NAME=pumpkin_pro DB_SSLMODE=disable go run main.go
```

#### 3. 启动前端

```bash
cd frontend
npm install
npm run dev
```

前端统一请求同源路径 `/api/*`，由 Next.js 转发到后端。

- 本地开发默认转发到 `http://localhost:8080`
- 如果前端服务端与后端不在同一地址，可为前端进程设置 `BACKEND_API_URL`
- Docker Compose 已默认配置为 `http://backend:8080`

## 使用说明

### 典型使用路径

1. 打开前端页面 `http://localhost:3000`
2. 注册 / 登录账号后，将感兴趣股票加入关注池
3. 在 `行情看板` 与个股详情页查看技术面、基础面、新闻、公司资料和 AI 分析
4. 在 `AI 选股器`、`四象限`、`卧龙排行榜` 或 `因子实验室` 中寻找候选标的
5. 在 `策略库` 中维护策略，或直接使用 AI 生成策略并做回测验证
6. 在 `持仓管理` 中记录交易事件、查看组合归因与盈亏日历
7. 在 `设置` 中配置 Webhook，接收策略信号推送

### 发起一次历史回测

1. 打开前端页面 `http://localhost:3000`
2. 选择数据源：在线下载、本地 CSV 或示例行情
3. 选择策略并调整参数
4. 设置开始日期、结束日期、初始资金和手续费
5. 点击执行回测
6. 在结果区查看图表、绩效指标、月度收益和交易记录

### 在线下载模式说明

- **A 股**：输入 6 位股票代码，例如 `600519`
- **港股**：输入 5 位股票代码，例如 `00700`

### CSV 数据格式建议

推荐包含以下字段：

- `date`
- `open`
- `high`
- `low`
- `close`
- `volume`

系统会对日期和数值字段做基础清洗与标准化处理。

## API 概览

### `backend` 主要接口

- **认证与用户**：`/api/auth/*`、`/api/user/*`
- **回测与策略**：`/api/backtest`、`/api/backtest/runs`、`/api/strategies`、`/api/strategies/ai-generate`
- **实时行情与研究**：`/api/live/watchlist`、`/api/live/market/overview`、`/api/live/symbols/*`
- **组合与设置**：`/api/portfolio/*`、`/api/investment-profile`、`/api/webhook`、`/api/signal-configs`
- **市场机会**：`/api/quadrant*`、`/api/screener/*`、`/api/factor-lab/*`
- **管理后台**：`/api/admin/*`

### `quant` 主要接口

- **健康检查**：`GET /api/health`
- **回测与策略定义**：`GET /api/backtest/options`、`POST /api/backtest`、`GET /api/strategies*`
- **全市场扫描**：`POST /api/screener/scan`
- **基础数据**：`GET /api/fundamentals/{symbol}`、`GET /api/news/{symbol}`、`POST /api/company-profiles/sync`
- **计算任务**：`POST /api/quadrant/compute-all`、`POST /api/quadrant/compute-hk-all`
- **信号评估**：`POST /api/signal/evaluate`

## 开发建议

- 若使用在线行情下载、新闻、基础面或公司资料同步，请确认本机网络可访问外部数据源
- 若需要扩展策略，可优先在 `quant/strategy_library/` 与 `backend/store/strategy/` 中同步补充定义与注册
- 若需要扩展前端功能入口，可优先查看 `frontend/pages/live-trading.js`、`frontend/pages/live-trading/[symbol].js`、`frontend/pages/backtest.js`、`frontend/pages/portfolio.js`

## 开源许可

本项目采用 **MIT License** 开源，你可以在保留原始版权声明的前提下自由使用、修改、分发和商用。

详细条款请参见根目录下的 `LICENSE` 文件。

## 免责声明

本项目仅用于学习、研究与工程实践演示，不构成任何投资建议。量化策略历史表现不代表未来收益，实盘使用前请自行完成风险评估与充分测试。
