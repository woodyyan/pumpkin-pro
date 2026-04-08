import Link from 'next/link'
import { useState } from 'react'
import { useAuth } from '../lib/auth-context'

// ── Data ──

const HIGHLIGHTS = [
  { value: 'A 股 + 港股', desc: '双市场支持' },
  { value: 'AI 辅助决策', desc: '智能分析选股' },
  { value: '100+', desc: '技术指标' },
]

const FEATURES = [
  {
    icon: '🤖', title: 'AI 分析与决策', href: '/live-trading',
    points: ['整合平台所有 AI 能力：AI 个股智能决策、AI 自然语言选股、AI 策略生成与优化'],
    cta: '体验 AI',
  },
  {
    icon: '📊', title: '行情看板', href: '/live-trading',
    points: ['实时 / 延迟行情数据展示', '均线 / MACD / 布林带等技术指标', '支撑位与阻力位分析', '基本面数据（PE / PB / PEG 等）', '走势对比与异动检测', '持仓记录与实时盈亏计算', '投资画像配置与管理'],
    cta: '进入看板',
  },
  {
    icon: '🔍', title: '选股器', href: '/stock-picker',
    points: ['多维条件筛选（行业 + 财务 + 技术面）', '全市场 A 股扫描', '支持排序与分页', '自选表保存与加载'],
    cta: '去选股',
  },
  {
    icon: '🔬', title: '策略回测', href: '/backtest',
    points: ['基于历史数据验证策略表现', '收益率 / 最大回撤 / 胜率等指标', '可视化收益曲线与交易记录', '支持自定义策略参数', '历史运行记录自动保存'],
    cta: '开始回测',
  },
  {
    icon: '🔔', title: '信号推送', href: '/settings',
    points: ['策略自动评估（每 15 分钟）', 'Webhook 推送至企微 / 钉钉 / 飞书', '冷却时间 + 按交易日去重', '非交易时段自动暂停', '支持多只股票独立配置'],
    cta: '配置信号',
  },
  {
    icon: '🗺️', title: '风险全景', href: '/live-trading',
    points: ['四象限模型（机会 / 拥挤 / 泡沫 / 防御）', '全市场 A 股覆盖', '关注池股票高亮标注', '每日凌晨自动更新', '鼠标悬停查看个股详情'],
    cta: '查看全景',
  },
]


const SCENARIOS = [
  {
    icon: '📊',
    title: '我想研究一只股票',
    href: '/live-trading',
    cta: '去研究股票',
    value: '🎯 30 秒获得 AI 专业级诊断报告',
    recommended: true,
    steps: [
      '进入「行情看板」，添加你想研究的股票（输入代码或名称）',
      '点击卡片进入详情页，浏览实时行情和技术指标',
      '点击顶部「✨AI分析」按钮，等待 AI 综合诊断报告',
    ],
    aiPreview: [
      { icon: '🟢', text: '建议：观望', color: 'text-emerald-300', highlight: true },
      { text: '置信度：72%（中高）', color: 'text-white/60' },
      { icon: '⚠️', text: '风险提示：短期波动较大', color: 'text-amber-300/80' },
    ],
  },
  {
    icon: '🔍',
    title: '我想选股',
    href: '/stock-picker',
    cta: '去 AI 选股',
    value: "🎯 不懂指标？用说话就能从全市场筛选",
    recommended: false,
    steps: [
      '进入「选股器」页面',
      '在搜索框中用自然语言描述你的选股条件，如：「低估值绩优股，高增长小盘股，科技行业龙头」',
      'AI 自动解析条件并全市场扫描，展示匹配结果列表',
    ],
    aiPreview: [
      { icon: '📋', text: '找到 12 只匹配标的', color: 'text-white', highlight: true },
      { text: '已按收益率排序，可逐个查看详情', color: 'text-white/55' },
    ],
  },
  {
    icon: '⚙️',
    title: '我想验证策略',
    href: '/backtest',
    cta: '去验证策略',
    value: '🎯 用历史数据说话，不靠直觉猜',
    recommended: false,
    steps: [
      '进入「回测引擎」页面',
      '选择预设策略或使用「AI 生成策略」自动定制',
      '设置参数后运行回测，查看收益曲线和关键指标',
    ],
    aiPreview: [
      { icon: '📈', text: '年化 +23.4%', color: 'text-red-300', highlight: true },
      { text: '夏普比率 1.82 · 最大回撤 12%', color: 'text-white/60' },
    ],
  },
]

const TUTORIALS = [
  {
    q: '如何添加股票到关注池？',
    steps: [
      '进入「行情看板」页面',
      '点击页面上方的「添加股票」按钮',
      '输入股票代码（如 600519）或名称关键字',
      '可选填自定义名称（如「茅台」）',
      '点击确认，股票会出现在关注池卡片列表中',
      '点击任意卡片可进入个股详情页，查看完整分析',
    ],
    tip: '支持 A 股（沪深）和港股代码（如 00700.HK）。',
  },
  {
    q: '如何创建自定义策略？',
    steps: [
      '进入「策略库」页面',
      '点击右上角「新建策略」按钮',
      '填写策略名称和说明',
      '选择策略类型（如趋势策略、均线交叉等）',
      '配置策略参数（每个参数都有单位标注）',
      '点击「保存」，策略会出现在你的策略列表中',
    ],
    tip: '系统也提供了多条预设策略模板，可以直接使用或基于它们修改。',
  },
  {
    q: '如何配置 Webhook 接收信号？',
    steps: [
      '进入「设置」页面',
      '找到「Webhook 配置」区域',
      '填入你的推送地址（支持企业微信、钉钉、飞书等）',
      '可选填签名密钥（用于验证消息来源）',
      '点击「保存」，然后点击「验证送达」测试是否连通',
      '返回行情看板个股详情页，开启信号开关并选择策略',
    ],
    tip: <>信号会以文本消息格式推送，包含股票代码、方向（买/卖）、策略名称和触发时间。不知道如何获取 Webhook 地址？可参考<a href="https://open.work.weixin.qq.com/help2/pc/14931" target="_blank" rel="noopener noreferrer" className="underline underline-offset-2 font-medium hover:text-primary">企业微信 Webhook 配置教程</a>。</>,
  },
  {
    q: '如何使用 AI 智能选股？',
    steps: [
      '进入「选股器」页面',
      '在顶部搜索框中用自然语言描述你的选股条件',
      '例如：「市盈率低于 20，净利润增长率大于 30% 的医药行业股票」',
      'AI 会自动解析条件并执行筛选',
      '结果以表格形式展示，支持排序和翻页',
    ],
    tip: '也可以使用手动筛选：选择行业 + 设置各指标范围。',
  },
  {
    q: '如何查看历史回测结果？',
    steps: [
      '进入「回测引擎」页面',
      '选择一只股票和一条策略',
      '设置回测时间范围和初始资金',
      '点击「开始回测」',
      '等待计算完成后，页面会展示收益曲线、交易记录、关键指标',
      '历史运行记录保存在页面下方「历史运行」面板中，可随时查看',
    ],
    tip: '单用户最多保存 100 条回测记录。',
  },
  {
    q: '如何查看个股技术指标？',
    steps: [
      '进入「行情看板」，点击任意关注股票卡片进入详情页',
      '页面中部「技术指标」区域展示了均线（MA5/MA10/MA20/MA60）数值和趋势',
      '向下滚动可看到 MACD 图表（DIF 线、信号线、红绿柱状图，自动标注金叉/死叉）',
      '继续向下是布林带图表（上轨/中轨/下轨通道 + 收盘价 + 触轨标记）',
      '支撑位和阻力位区域展示了近期的关键价格位',
    ],
    tip: '技术指标数据每 60 秒自动刷新（交易时段），基于最近 120-240 个交易日的日线计算。',
  },
  {
    q: '如何记录和跟踪持仓？',
    steps: [
      '进入行情看板中任意个股的详情页',
      '找到「我的持仓」区域（需登录）',
      '填写持仓数量、买入均价、买入日期',
      '系统会自动计算持仓市值和浮动盈亏',
      '盈亏会根据实时价格动态更新（红色为盈利，绿色为亏损）',
    ],
    tip: '你也可以在「设置」页面配置投资画像（风险偏好、投资目标等）。',
  },
  {
    q: '交易信号是怎么触发的？',
    steps: [
      '系统每 15 分钟自动扫描所有已开启信号的股票',
      '根据你选择的策略和参数，评估当前行情是否触发买入/卖出条件',
      '触发后，信号会通过你配置的 Webhook 推送到你的消息工具',
      '同一只股票同一方向，每个交易日最多触发一次（防重复）',
      '非交易时段（收盘后、周末）不会触发信号',
    ],
    tip: '冷却时间可在信号配置中自定义（10 秒 ~ 24 小时）。',
  },
]

// ── Components ──

function Accordion({ items }) {
  const [openIdx, setOpenIdx] = useState(-1)
  return (
    <div className="space-y-2">
      {items.map((item, i) => {
        const isOpen = openIdx === i
        return (
          <div key={i} className="rounded-xl border border-border bg-card overflow-hidden">
            <button
              type="button"
              onClick={() => setOpenIdx(isOpen ? -1 : i)}
              className="w-full flex items-center justify-between px-5 py-4 text-left text-sm font-medium text-white/90 hover:bg-white/3 transition"
            >
              <span>{item.q}</span>
              <span className={`text-white/40 transition-transform ${isOpen ? 'rotate-90' : ''}`}>▸</span>
            </button>
            <div className={`grid transition-all duration-300 ease-in-out ${isOpen ? 'grid-rows-[1fr]' : 'grid-rows-[0fr]'}`}>
              <div className="overflow-hidden">
                <div className="px-5 pb-4 space-y-3">
                  <ol className="list-decimal pl-5 space-y-1.5 text-sm text-white/65 leading-6">
                    {item.steps.map((s, j) => <li key={j}>{s}</li>)}
                  </ol>
                  {item.tip && (
                    <div className="rounded-lg bg-primary/8 border border-primary/15 px-3 py-2 text-xs text-primary/90">
                      💡 {item.tip}
                    </div>
                  )}
                </div>
              </div>
            </div>
          </div>
        )
      })}
    </div>
  )
}

// ── Page ──

export default function HomePage() {
  const { isLoggedIn, openAuthModal } = useAuth()

  const handleCTA = () => {
    if (isLoggedIn) {
      window.location.href = '/live-trading'
    } else {
      openAuthModal('register')
    }
  }

  return (
    <div className="space-y-0">

      {/* ── Section 1: Hero ── */}
      <section className="relative flex flex-col items-center text-center px-4 pt-8 pb-16 md:pt-16 md:pb-24">
        {/* Subtle radial gradient */}
        <div className="absolute inset-0 bg-[radial-gradient(ellipse_at_center,rgba(230,126,34,0.06)_0%,transparent_70%)] pointer-events-none" />

        <img src="/logo.png" alt="卧龙" width={100} height={100} className="rounded-xl mb-6 relative md:w-[120px] md:h-[120px]" />

        <h1 className="relative text-3xl md:text-5xl font-bold tracking-tight bg-gradient-to-r from-amber-200 via-primary to-amber-500 bg-clip-text text-transparent">
          卧龙 AI 量化交易台
        </h1>
        <p className="relative mt-3 text-base md:text-lg text-white/50 font-medium">
          智能分析 · 策略回测 · 信号推送
        </p>
        <p className="relative mt-4 max-w-xl text-sm md:text-base text-white/40 leading-7">
          面向个人投资者的 AI 量化分析平台，帮你用数据和策略看清市场，做出更理性的投资决策。
        </p>

        <div className="relative flex flex-col sm:flex-row items-center gap-3 mt-8">
          <button
            type="button"
            onClick={handleCTA}
            className="rounded-xl bg-gradient-to-r from-amber-500 to-primary px-7 py-3 text-sm font-semibold text-black shadow-lg shadow-primary/20 transition hover:opacity-90"
          >
            免费开始使用
          </button>
          <a
            href="#features"
            className="rounded-xl border border-white/15 px-6 py-3 text-sm text-white/60 transition hover:border-white/30 hover:text-white"
          >
            查看功能介绍 ↓
          </a>
        </div>

        <div className="relative grid grid-cols-1 sm:grid-cols-3 gap-4 mt-12 w-full max-w-2xl">
          {HIGHLIGHTS.map((h, i) => (
            <div key={i} className="rounded-xl border border-white/8 bg-white/[0.03] backdrop-blur-sm px-5 py-4 text-center">
              <div className="text-xl md:text-2xl font-bold bg-gradient-to-r from-amber-300 to-primary bg-clip-text text-transparent">
                {h.value}
              </div>
              <div className="mt-1 text-xs text-white/40">{h.desc}</div>
            </div>
          ))}
        </div>
      </section>

      {/* ── Section 2: 场景化快速上手 ── */}
      <section className="px-4 py-16 md:py-24 max-w-5xl mx-auto">
        <div className="text-center mb-12">
          <h2 className="text-2xl md:text-3xl font-bold tracking-tight">🚀 快速上手</h2>
          <p className="mt-3 text-sm text-white/40">选择你的使用场景，3 步开始体验</p>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-3 gap-5">
          {SCENARIOS.map((s, i) => (
            <Link
              key={i}
              href={s.href}
              className={`group relative rounded-2xl border bg-card p-6 transition hover:-translate-y-1 hover:shadow-lg ${
                s.recommended
                  ? 'border-primary/40 shadow-primary/[0.04]'
                  : 'border-border hover:border-white/15'
              }`}
            >
              {s.recommended && (
                <span className="absolute right-4 top-4 rounded-full bg-primary/15 px-2.5 py-0.5 text-[11px] font-medium text-primary">推荐</span>
              )}

              <div className="flex items-center gap-2.5 mb-4">
                <span className="text-2xl">{s.icon}</span>
                <h3 className="text-lg font-semibold text-white">{s.title}</h3>
              </div>

              {/* Steps */}
              <div className="space-y-2 mb-5">
                {s.steps.map((step, j) => (
                  <div key={j} className="flex items-start gap-2.5">
                    <span className="mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-primary/10 text-[11px] font-bold text-primary">{j + 1}</span>
                    <span className="text-[13px] leading-relaxed text-white/65">{step}</span>
                  </div>
                ))}
              </div>

              {/* AI Preview */}
              {s.aiPreview && (
                <div className="rounded-xl bg-gradient-to-r from-primary/[0.07] to-transparent border border-primary/12 p-3.5 mb-4">
                  <div className="space-y-1.5">
                    {s.aiPreview.map((line, k) => (
                      <div key={k} className={`text-[13px] ${line.highlight ? 'font-semibold text-white' : line.color || 'text-white/55'}`}>
                        {line.icon && <span className="mr-1.5">{line.icon}</span>}
                        {line.text}
                      </div>
                    ))}
                  </div>
                </div>
              )}

              {/* Value tag */}
              <p className="mb-4 text-xs text-white/35 italic">{s.value}</p>

              {/* CTA */}
              <span className="inline-flex items-center text-sm font-medium text-primary group-hover:text-primary transition">
                {s.cta} <span className="ml-1 transition-transform group-hover:translate-x-1">→</span>
              </span>
            </Link>
          ))}
        </div>
      </section>

      {/* ── Section 3: Features ── */}
      <section id="features" className="px-4 py-16 md:py-24 max-w-6xl mx-auto">
        <div className="text-center mb-12">
          <h2 className="text-2xl md:text-3xl font-bold tracking-tight">我们提供什么</h2>
          <p className="mt-3 text-sm text-white/40">一站式量化分析工具，覆盖投研全流程</p>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-5">
          {FEATURES.map((f, i) => (
            <Link
              key={i}
              href={f.href}
              className="group rounded-2xl border border-border bg-card p-6 transition hover:-translate-y-1 hover:border-white/15 hover:shadow-lg"
            >
              <div className="text-4xl mb-4">{f.icon}</div>
              <h3 className="text-lg font-semibold text-white mb-3">{f.title}</h3>
              <ul className="space-y-1.5 text-sm text-white/50 leading-6 mb-4">
                {f.points.map((p, j) => <li key={j} className="flex items-start gap-2"><span className="text-primary/60 mt-0.5">•</span>{p}</li>)}
              </ul>
              <span className="inline-flex items-center text-sm text-primary/80 font-medium group-hover:text-primary transition">
                {f.cta} <span className="ml-1 transition-transform group-hover:translate-x-1">→</span>
              </span>
            </Link>
          ))}
        </div>
      </section>

      {/* ── Section 5: Tutorials ── */}
      <section className="px-4 py-16 md:py-24 max-w-3xl mx-auto">
        <div className="text-center mb-10">
          <h2 className="text-2xl md:text-3xl font-bold tracking-tight">使用教程</h2>
          <p className="mt-3 text-sm text-white/40">点击展开查看详细操作步骤</p>
        </div>
        <Accordion items={TUTORIALS} />
      </section>

      {/* ── Section 6: Risk Disclaimer + CTA ── */}
      <section className="px-4 py-16 md:py-20 max-w-3xl mx-auto text-center">
        <div className="rounded-2xl border border-amber-400/15 bg-amber-500/[0.04] px-6 py-5 mb-10">
          <p className="text-sm text-amber-200/70 leading-6">
            <strong className="text-amber-200/90">⚠️ 风险提示：</strong>本平台仅提供数据分析和策略回测工具，不构成任何投资建议。股票市场有风险，投资需谨慎。详见
            <Link href="/disclaimer" className="underline underline-offset-2 hover:text-amber-100 mx-0.5">《免责声明》</Link>
          </p>
        </div>

        <button
          type="button"
          onClick={handleCTA}
          className="rounded-xl bg-gradient-to-r from-amber-500 to-primary px-8 py-3.5 text-base font-semibold text-black shadow-lg shadow-primary/20 transition hover:opacity-90"
        >
          免费开始使用
        </button>
      </section>
    </div>
  )
}
