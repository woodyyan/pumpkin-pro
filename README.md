# 南瓜交易系统（Pumpkin Trader Pro）

一个面向个人研究者与量化开发者的多服务交易与回测系统。项目采用 `frontend + backend + quant` 三层架构，提供统一的历史回测入口、可视化结果分析界面，以及可扩展的策略计算服务。

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
- **后端**：Go
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

### 方式一：使用 Docker Compose 启动

在项目根目录执行：

```bash
docker compose up --build
```

启动后默认端口：

- **前端**：`http://localhost:3000`
- **后端**：`http://localhost:8080`
- **量化服务**：`http://localhost:8000`

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
go run main.go
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
