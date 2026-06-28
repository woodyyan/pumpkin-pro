# 资金星图设计

## 架构

```text
frontend/pages/capital-map.js
  -> components/CapitalMapDashboard.js
  -> lib/capital-map.js
  -> /api/capital-map
  -> backend/store/capitalmap.Service
  -> Eastmoney qt/clist/get
```

## 后端

- 新增 `backend/store/capitalmap` 包。
- `EastmoneyClient` 负责请求东方财富公开接口和字段解析。
- `Service` 负责 30 秒内存缓存、stale 快照降级和 payload 构造。
- `GET /api/capital-map` 由 `main.go` 注册，公开访问。
- 响应头设置 `Cache-Control: s-maxage=30, stale-while-revalidate=90`。

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
