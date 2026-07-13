# 资金星图设计

## 架构

```text
frontend/pages/capital-map.js
  -> components/CapitalMapDashboard.js
  -> lib/capital-map.js
  -> backend /api/capital-map
  -> backend/store/capitalmap.ProxyService（30 秒缓存 + stale 降级）
  -> quant /api/capital-map
  -> quant/capital_map.Service
  -> DataSourceManager(capital_map, ASHARE)
  -> EastMoney provider qt/clist/get
```

## 后端

- `backend/store/capitalmap.ProxyService` 负责代理 quant `/api/capital-map`、30 秒内存缓存和 stale 降级。
- backend 不再直接理解东方财富字段，不再承载 PE / PoC / 板块资金算法。
- `GET /api/capital-map` 由 `main.go` 注册，公开访问。
- 响应头设置 `Cache-Control: s-maxage=30, stale-while-revalidate=60`。

## Quant

- 新增 `quant/capital_map/` 模块。
- `models.py` 定义资金星图股票、板块与快照模型。
- `normalizer.py` 负责东方财富字段归一、PE TTM 优先选择、市场前缀归一与金额单位转换。
- `service.py` 负责 payload 构造、成交额排序、PoC 分箱、板块资金排序。
- `DataSourceManager.fetch_capital_map()` 作为统一数据源入口，第一期固定 `ASHARE + EastMoney`。
- quant `GET /api/capital-map` 返回前端兼容 payload。

## 前端

- 新增 `/capital-map` 页面。
- 新增 `CapitalMapDashboard` 作为主组件。
- 引入 `echarts`，在浏览器端动态 import，避免 SSR 直接加载。
- 图表颜色通过 `useTheme().resolvedTheme` 选择浅色/深色 palette。
- A 股涨跌色固定为红涨绿跌。

## 数据口径

- 股票样本：东方财富按成交额排序分页抓取，首期保留原高流动性样本范围。
- PE：优先 PE TTM，缺失时回退动态 PE。
- PoC：只统计 `0 < PE <= 120`，5 倍 PE 分箱，分箱成交额最大者为 PoC。
- 板块资金：东方财富板块公开接口，主力净流入属于平台算法口径。
