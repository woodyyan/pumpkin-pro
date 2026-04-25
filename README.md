# 卧龙AI量化交易台（Wolong Pro）

> 官网直达：<https://wolongtrader.top/>

一个面向个人研究者与量化开发者的AI量化交易系统。项目提供统一的历史回测引擎、AI策略分析、风险机会四象限、卧龙股票排行榜、选股器、AI股票分析、持仓管理等功能。

当前版本重点覆盖 **历史回测工作台** 场景：你可以在网页端选择数据源、配置策略参数、发起回测，并查看收益曲线、回撤曲线、月度收益和交易明细。

## 功能特性

### 1. 历史回测工作台

- 支持在前端页面直接发起回测
- 支持设置回测区间、初始资金、手续费
- 支持查看核心绩效指标、资产曲线、回撤曲线、交易记录
- 支持在 K 线图上叠加策略指标与买卖点

### 2. 三类数据源

- **在线下载**：支持下载 A 股、港股历史行情数据
- **本地 CSV**：支持上传本地历史数据文件进行回测
- **示例行情**：支持生成模拟行情，用于快速体验系统流程

### 3. 四类内置策略

- **趋势跟踪（双均线）**：适合趋势型行情
- **网格交易**：适合震荡市场分层交易
- **均值回归（布林带）**：适合价格偏离后的回归交易
- **区间交易（RSI）**：适合区间震荡场景

### 4. 完整结果分析

- 总收益率、年化收益率、最大回撤、夏普比率
- 胜率、交易次数、最终资产、总手续费
- 月度收益统计
- 交易明细与信号统计
- K 线、资产曲线、回撤曲线、RSI 辅助图

### 5. 分层服务架构

- **`frontend/`**：Next.js 前端界面，负责参数配置、图表展示与交互体验
- **`backend/`**：Go API 网关，负责对前端暴露统一接口并转发请求到量化服务
- **`quant/`**：Python FastAPI 量化服务，负责数据加载、指标计算、策略执行与绩效分析

## 技术栈

- **前端**：Next.js 14、React 18、Lightweight Charts、Tailwind CSS
- **后端**：Go、GORM
- **数据库**：SQLite（默认）/ PostgreSQL（可选，`DB_TYPE=postgres`）
- **量化服务**：Python、FastAPI、Pandas、NumPy、AkShare
- **部署方式**：Docker Compose / 本地开发模式

## 项目架构

```text
pumpkin-pro/
├── frontend/   # 回测页面与可视化界面
├── backend/    # Go API 网关
├── quant/      # Python 量化回测服务
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

### `backend`

- **`GET /api/health`**：后端健康检查
- **`POST /api/backtest`**：统一回测入口，转发到量化服务

### `quant`

- **`GET /api/health`**：量化服务健康检查与能力说明
- **`GET /api/backtest/options`**：获取支持的策略和数据源
- **`POST /api/backtest`**：执行历史回测并返回结果

## 开发建议

- 若使用在线行情下载，请确认本机网络可访问外部数据源
- 若需要扩展策略，可在 `quant/strategy/` 中新增策略类，并在 `quant/main.py` 中注册

## 开源许可

本项目采用 **MIT License** 开源，你可以在保留原始版权声明的前提下自由使用、修改、分发和商用。

详细条款请参见根目录下的 `LICENSE` 文件。

## 免责声明

本项目仅用于学习、研究与工程实践演示，不构成任何投资建议。量化策略历史表现不代表未来收益，实盘使用前请自行完成风险评估与充分测试。
