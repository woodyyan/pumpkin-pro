# AI Picker P0 Design

## 架构
- 后端新增 `backend/store/aipicker`。
- 前端新增 `frontend/pages/ai/picker.js` 与 `frontend/lib/ai-picker.js`。
- 结果来源分为两类：
  1. `daily_auto`：每日定时生成并落库。
  2. `manual` / `by_direction`：用户实时生成，不覆盖 daily 页面读取逻辑。

## 后端链路
1. 读取 runtime AI 配置。
2. 调用 `factorLabService.Meta()` 校验最新快照存在且未过期。
3. 调用 `factorLabService.Screen()` 拉取综合得分候选全集，并按 7 因子独立 TopN recall + 行业 cap 收敛候选池。
4. 对候选池执行技术快照增强；技术指标为软依赖，缺失时保留股票，仅在 prompt 中标注“部分技术指标缺失”。
5. 将候选池压缩成固定 prompt，要求 LLM 严格输出 JSON。
6. 服务端执行 post-validation：
   - 仅允许候选池内股票
   - 强制补齐 4 只
   - 单票仓位夹紧到 10~35
   - 总仓位归一到 <= 80
   - 自动补齐止盈止损/理由/风险字段
7. `daily_auto` / `manual` 结果写入 `ai_picker_daily_results`，管理端生成日志写入 `ai_picker_generate_logs`。

## API
- `GET /api/ai/picker/meta`
- `GET /api/ai/picker/daily`
- `POST /api/ai/picker`

## 前端交互
- 默认加载 `meta + daily`。
- 港股页签显示即将上线。
- 登录后可点击“重新生成”。
- 高级模式可输入一句话方向。
