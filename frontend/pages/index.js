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
    icon: '📊', title: '行情看板', href: '/live-trading',
    points: ['实时 / 延迟行情数据展示', '均线 / MACD / 布林带等技术指标', '支撑位与阻力位分析', '基本面数据（PE / PB / PEG 等）', '走势对比与异动检测'],
    cta: '进入看板',
  },
  {
    icon: '🔬', title: '策略回测', href: '/backtest',
    points: ['基于历史数据验证策略表现', '收益率 / 最大回撤 / 胜率等指标', '可视化收益曲线与交易记录', '支持自定义策略参数', '历史运行记录自动保存'],
    cta: '开始回测',
  },
  {
    icon: '🔍', title: '选股平台', href: '/stock-picker',
    points: ['AI 自然语言智能选股', '多维条件筛选（行业 + 财务 + 技术面）', '全市场 A 股扫描', '支持排序与分页', '自选表保存与加载'],
    cta: '去选股',
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
  {
    icon: '📋', title: '持仓管理', href: '/live-trading',
    points: ['记录买入价格与数量', '实时盈亏计算（红盈绿亏）', '投资画像配置', '风险偏好与目标管理', '持仓市值动态更新'],
    cta: '管理持仓',
  },
]

const STEPS = [
  { icon: '👤', title: '注册账号', desc: '免费注册即可开始使用全部功能' },
  { icon: '📐', title: '配置策略', desc: '在策略库中选择预设或自建策略' },
  { icon: '📊', title: '进行回测', desc: '用历史数据验证策略是否有效' },
  { icon: '⭐', title: '添加关注股票', desc: '把感兴趣的股票加入关注池' },
  { icon: '🔔', title: '配置交易信号', desc: '配置 Webhook 接收买卖信号通知' },
]

const SCREENSHOTS = [
  { id: 'live', label: '行情看板', desc: '实时行情数据 + 技术指标 + 基本面分析' },
  { id: 'backtest', label: '策略回测', desc: '历史数据回测 + 收益曲线 + 交易记录' },
  { id: 'picker', label: '选股平台', desc: 'AI 智能选股 + 多维条件筛选' },
  { id: 'quadrant', label: '四象限', desc: '全市场风险机会全景散点图' },
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
      '进入「选股平台」页面',
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
  const [screenshotTab, setScreenshotTab] = useState(0)

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

      {/* ── Section 2: Features ── */}
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

      {/* ── Section 3: Screenshots ── */}
      <section className="px-4 py-16 md:py-24 max-w-5xl mx-auto">
        <div className="text-center mb-10">
          <h2 className="text-2xl md:text-3xl font-bold tracking-tight">产品一览</h2>
        </div>

        <div className="flex items-center justify-center gap-2 mb-6 flex-wrap">
          {SCREENSHOTS.map((s, i) => (
            <button
              key={s.id}
              type="button"
              onClick={() => setScreenshotTab(i)}
              className={`rounded-lg px-4 py-2 text-sm transition ${
                screenshotTab === i
                  ? 'bg-primary/15 text-white border border-primary/40 font-medium'
                  : 'text-white/45 border border-transparent hover:text-white/70'
              }`}
            >
              {s.label}
            </button>
          ))}
        </div>

        {/* Placeholder */}
        <div className="rounded-2xl border border-border bg-gradient-to-br from-[#12141a] to-[#0d0f14] flex items-center justify-center aspect-video max-h-[450px] w-full">
          <div className="text-center space-y-2">
            <div className="text-4xl opacity-20">📸</div>
            <p className="text-sm text-white/25">{SCREENSHOTS[screenshotTab].label}截图</p>
            <p className="text-xs text-white/15">即将更新</p>
          </div>
        </div>
        <p className="mt-4 text-center text-sm text-white/35">{SCREENSHOTS[screenshotTab].desc}</p>
      </section>

      {/* ── Section 4: Getting Started ── */}
      <section className="px-4 py-16 md:py-24 max-w-5xl mx-auto">
        <div className="text-center mb-12">
          <h2 className="text-2xl md:text-3xl font-bold tracking-tight">快速上手</h2>
          <p className="mt-3 text-sm text-white/40">5 步开始你的量化分析之旅</p>
        </div>

        {/* Desktop: horizontal */}
        <div className="hidden md:flex items-start justify-between gap-2">
          {STEPS.map((s, i) => (
            <div key={i} className="flex-1 flex flex-col items-center text-center relative">
              {/* Connector line */}
              {i < STEPS.length - 1 && (
                <div className="absolute top-6 left-[calc(50%+24px)] right-[calc(-50%+24px)] h-px bg-gradient-to-r from-primary/30 to-primary/10" />
              )}
              <div className="relative w-12 h-12 rounded-full bg-primary/15 border border-primary/30 flex items-center justify-center text-xl mb-3">
                {s.icon}
              </div>
              <div className="text-xs font-bold text-primary/70 mb-1">Step {i + 1}</div>
              <h4 className="text-sm font-semibold text-white mb-1">{s.title}</h4>
              <p className="text-xs text-white/40 leading-5 max-w-[140px]">{s.desc}</p>
            </div>
          ))}
        </div>

        {/* Mobile: vertical timeline */}
        <div className="md:hidden space-y-0">
          {STEPS.map((s, i) => (
            <div key={i} className="flex gap-4">
              <div className="flex flex-col items-center">
                <div className="w-10 h-10 rounded-full bg-primary/15 border border-primary/30 flex items-center justify-center text-lg shrink-0">
                  {s.icon}
                </div>
                {i < STEPS.length - 1 && <div className="w-px flex-1 bg-primary/15 my-1" />}
              </div>
              <div className="pb-6">
                <div className="text-xs font-bold text-primary/70">Step {i + 1}</div>
                <h4 className="text-sm font-semibold text-white">{s.title}</h4>
                <p className="text-xs text-white/40 leading-5 mt-0.5">{s.desc}</p>
              </div>
            </div>
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
