# 前端编码规范

## 颜色使用规范

### ❌ 禁止的写法
```jsx
// 禁止硬编码不透明颜色
<div className="text-white/60 bg-slate-900 border-white/10" />
```

### ✅ 正确的写法
```jsx
// 使用语义化 token
<div className="text-foreground-muted bg-background border-border" />
```

### 语义化 Token 速查表

| Token | 浅色模式 | 深色模式 | 用途 |
|---|---|---|---|
| `text-foreground` | `#1a1a2e` | `#ededed` | 主文字 |
| `text-foreground-muted` | `#4a4a6a` | `rgba(255,255,255,0.6)` | 次级文字 |
| `text-foreground-dim` | `rgba(0,0,0,0.5)` | `rgba(255,255,255,0.35)` | 三级文字 |
| `text-foreground-disabled` | `rgba(0,0,0,0.25)` | `rgba(255,255,255,0.18)` | 禁用态文字 |
| `bg-background` | `#ffffff` | `#0a0a0b` | 页面背景 |
| `bg-card` | `#ffffff` | `#161618` | 卡片背景 |
| `border-border` | `rgba(0,0,0,0.08)` | `rgba(255,255,255,0.08)` | 边框 |
| `text-primary` | `#e67e22` | `#e67e22` | 品牌色 |
| `text-positive` | `#16a34a` | `#22c55e` | 上涨绿 |
| `text-negative` | `#dc2626` | `#ef4444` | 下跌红 |

### 特殊情况
- 模态弹窗遮罩: 可以用 `bg-black/70`（深浅模式都需要黑色半透明）
- 固定颜色指示器: 可以用 `bg-white/40` 等固定值
- 内联渐变中的特殊颜色: 使用 `var(--color-bg-hover)` 或 `var(--color-border)` 替代

### 自定义 CSS 变量
```css
/* 引用 CSS 变量时使用方括号语法 */
bg-[var(--color-bg-hover)]
bg-[var(--color-bg-overlay)]
bg-[var(--color-bg-secondary)]
border-[var(--color-border-strong)]
```

### 图表颜色
图表组件的颜色应在 JS 中根据 `useTheme().resolvedTheme` 选择对应的配色方案，不要硬编码在 JSX className 中。
