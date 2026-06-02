# Tasks: 卧龙 AI 精选模拟组合指标改造

## 已完成

- [x] 前端收益图改为单曲线，tooltip 改为累计收益 / 单日收益 / 回撤。
- [x] 顶部指标改为累计收益、成立天数、昨日收益率、本月收益率、最大回撤、波动率、日胜率。
- [x] 当前成分股增加买入价、最新价与“较买入价”收益率。
- [x] 后端新增 summary metrics 推导逻辑。
- [x] 后端新增当前成分股 entry/latest 价格 enrich 逻辑。
- [x] ranking portfolio 后端实时链路移除 benchmark 收益计算。
- [x] ranking portfolio 重建命令移除 benchmark 收益计算与 benchmark price 表依赖。
- [x] 相关后端、前端测试更新并通过。

## 可选后续

- [ ] 如未来要支持盘中浮盈，再单独评估接入实时行情价与刷新频率。
- [ ] 如后续确认不再需要 benchmark 元信息，可再评估数据库迁移，移除结果表和快照表中的 benchmark 标识字段。
