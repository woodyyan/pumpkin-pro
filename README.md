# Pumpkin Trader Pro 🎃

A professional, high-performance, and scalable quantitative trading and backtesting system.

## 🏗️ Architecture (Monorepo)

This project adopts a modern microservices-inspired architecture separated into three distinct domains:

### 1. `backend/` (Golang)
The core engine. Responsible for:
- Connecting to Broker APIs (e.g., Futu, IB) via Websockets.
- Managing order state, risk control, and user accounts.
- Exposing REST/GraphQL APIs to the frontend.

### 2. `frontend/` (React + Next.js)
The visual terminal. Responsible for:
- Professional UI featuring `TradingView Lightweight Charts`.
- Real-time portfolio monitoring.
- Interactive strategy parameter configuration.

### 3. `quant/` (Python + FastAPI)
The brain. Responsible for:
- Holding the intellectual property: Trading Strategies (e.g., Dual Moving Average, Grid).
- Data processing via Pandas/Polars.
- Exposing a fast RPC/HTTP service that the Go backend queries to get Buy/Sell signals.

## 🚀 Quick Start
```bash
docker-compose up --build
```
