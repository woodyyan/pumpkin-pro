# 前端主题模式 — 需求 & 设计

## 需求
- 网站支持浅色模式（默认深色模式保持不变）
- 用户可自由在浅色/深色/跟随系统 三态间切换
- 切换即刻生效，刷新保持偏好
- 全部页面 UI 支持浅色模式

## 方案要点
1. **CSS 变量**: `:root` 定义浅色变量，`.dark` 定义深色变量
2. **Tailwind class 模式**: `darkMode: "class"`，colors 引用 `var(--xxx)`
3. **ThemeProvider**: React Context 管理状态 + localStorage 持久化
4. **ThemeToggle**: 导航栏右侧三态选择按钮
5. **FOUC 防护**: `_document.js` 阻塞脚本

详见: [design.md](design.md)
