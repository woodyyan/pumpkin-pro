import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import Link from 'next/link'
import Head from 'next/head'
import InfoTip, { LabelWithInfo } from '../components/InfoTip'
import PortfolioAttributionSection from '../components/PortfolioAttributionSection'
import { requestJson } from '../lib/api'
import { useAuth } from '../lib/auth-context'
import {
  buildPortfolioDeleteConfirmText,
  createPortfolioEvent,
  fetchPortfolioDashboard,
  fetchPortfolioDetail,
  fetchPortfolioEventTimeline,
  fetchPortfolioRiskMetrics,
  fetchSymbolSnapshot,
  fetchSymbolDailyBars,
  findClosePriceByDate,
  formatMoney,
  formatCompactNumber,
  inferPortfolioTradeMarket,
  resolvePortfolioTradeSymbol,
  scopeLabel,
  exchangeTag,
  deletePortfolioHistory,
  undoPortfolioEvent,
} from '../lib/portfolio-dashboard.js'
import {
  fetchPortfolioAttributionSummary,
  fetchPortfolioAttributionStocks,
  fetchPortfolioAttributionSectors,
  fetchPortfolioAttributionTrading,
  fetchPortfolioAttributionMarket,
} from '../lib/portfolio-attribution.js'
import { getPortfolioPageViewState } from '../lib/portfolio-page-view.js'
import {
  readInvestmentProfileCache,
  subscribeInvestmentProfileUpdates,
} from '../lib/investment-profile-storage.js'
import {
  buildPortfolioEventPreview,
  createPortfolioActionForm,
  formatPortfolioEventHeadline,
  formatPortfolioEventSubline,
  getPortfolioEventAccent,
  isPortfolioPositionActive,
} from '../lib/portfolio-events.js'
import {
  describeFeeRate,
  describePortfolioFeeEstimate,
  formatPortfolioFeeAmount,
  formatFeeRatePercent,
  getPortfolioDefaultFeeRate,
} from '../lib/portfolio-fee.js'

// ── 常量 ──

const SCOPE_OPTIONS = [
  { value: 'ALL', label: '全部' },
  { value: 'ASHARE', label: 'A 股' },
  { value: 'HKEX', label: '港股' },
]

const SORT_OPTIONS = [
  { value: 'market_value', label: '市值' },
  { value: 'today_pnl', label: '今日盈亏' },
  { value: 'total_pnl', label: '累计盈亏' },
  { value: 'unrealized_pnl', label: '未实现盈亏' },
  { value: 'holding_days', label: '持仓天数' },
  { value: 'last_trade', label: '最近交易' },
]

const PNL_FILTER_OPTIONS = [
  { value: 'all', label: '全部' },
  { value: 'profit', label: '盈利' },
  { value: 'loss', label: '亏损' },
]

const CURVE_RANGE_OPTIONS = [
  { value: '7D', label: '7 天' },
  { value: '30D', label: '30 天' },
  { value: '90D', label: '90 天' },
  { value: 'ALL', label: '全部' },
]

const EMPTY_ATTRIBUTION = {
  summary: null,
  stocks: null,
  sectors: null,
  trading: null,
  market: null,
}

const EMPTY_ATTRIBUTION_DETAIL_LOADING = {
  stocks: false,
  sectors: false,
  trading: false,
  market: false,
}

const EMPTY_ATTRIBUTION_DETAIL_ERROR = {
  stocks: '',
  sectors: '',
  trading: '',
  market: '',
}

const ATTRIBUTION_FETCHERS = {
  stocks: fetchPortfolioAttributionStocks,
  sectors: fetchPortfolioAttributionSectors,
  trading: fetchPortfolioAttributionTrading,
  market: fetchPortfolioAttributionMarket,
}

const FIELD_TIPS = {
  manual_notice: '当前持仓管理数据需要你在个股详情页手动记录买入、卖出和调均价操作，系统暂时还不能直接同步券商真实持仓。后续会逐步支持券商数据对接。',
  position_count: '当前筛选范围内仍持有仓位的股票数量。只有持仓数量大于 0 的标的才会统计在内。',
  profit_loss_count: '按单只股票的累计盈亏划分：累计盈亏大于 0 计为盈利，小于 0 计为亏损。',
  max_position_weight: '单只股票在当前组合市值中的最高占比，用来观察仓位是否过于集中。',
  latest_trade: '最近一笔有效持仓交易的发生时间，包含买入、卖出和手动调均价。',
  total_capital: '来自设置页的“账户总资金”，表示你计划用于投资的总预算，目前用于辅助判断资金利用率。',
  market_value: '按最新行情估算的当前持仓总价值。持股数量 × 最新价格。',
  total_cost: '当前仍持有仓位对应的总投入成本，不含已经卖出的部分。',
  unrealized_pnl: '浮动盈亏。表示如果你现在按最新价格卖出当前持仓，理论上还能赚或亏多少。因为还没真正卖出，所以叫“未实现”。',
  realized_pnl: '已落袋的盈亏。只统计已经通过卖出真正确认下来的收益或亏损。',
  total_pnl: '累计盈亏 = 已实现盈亏 + 未实现盈亏，用来看这只股票或这个组合截至当前一共赚了还是亏了。',
  today_pnl: '按昨收和最新价估算的当日盈亏，用来看今天这一交易日你的持仓变动。',
  capital_usage: '资金利用率 = 当前持仓市值 / 设置页账户总资金。暂时只在人民币单市场口径下展示，避免跨币种误导。',
  equity_curve: '按每日持仓快照生成的组合市值变化曲线，用来观察整体资产走势。',
  allocation: '展示当前前几大持仓占组合市值的比例，方便快速判断仓位分散或集中情况。',
  position_detail: '按股票维度查看当前持仓、成本、现价、市值和盈亏表现，点击后会打开对应个股详情页。',
  shares: '你当前仍持有的股票数量。',
  avg_cost: '当前剩余仓位的加权平均持仓成本。买入会自动重算，卖出只减少数量不改剩余仓位成本。',
  last_price: '最近一次拉取到的市场价格，用于估算市值和浮动盈亏。',
  symbol_market_value: '该股票当前持仓数量按最新价格估算出来的总市值。',
  symbol_total_pnl: '这只股票截至当前的累计盈亏，等于已实现盈亏加未实现盈亏。',
  symbol_today_pnl: '这只股票相对昨收在今天贡献的盈亏。',
  recent_events: '最近记录的持仓操作流水，帮助你回看组合是如何一步步变化的。',
  trade_symbol: '支持直接输入完整股票代码，也支持输入裸代码后自动补齐市场后缀。例如 A 股输入 600519 会自动补成 600519.SH，港股输入 700 会自动补成 00700.HK。',
  trade_quantity: '交易数量按股为单位填写，买入和卖出都只记录你这次实际成交的数量。',
  trade_price: '按本次成交的实际单价填写，系统会据此计算新的持仓成本或已实现收益。',
  trade_fee: '手续费改为按默认费率自动估算，A股小额买卖若低于 5 元会按最低佣金 5 元估算；你仍可手动调整本次费率。',
  trade_adjust_reason: '调均价不会改动持仓数量，只会校准当前剩余仓位的平均成本，因此必须写清原因。',
  trade_preview: '结果预览会按当前持仓状态模拟这次操作后的剩余股数、均价和成本，不会提前真正写入。',
}

const TRADE_MARKET_OPTIONS = [
  { value: 'ASHARE', label: 'A股' },
  { value: 'HKEX', label: '港股' },
]

const TRADE_ACTION_COPY = {
  buy: {
    title: '买入 / 加仓',
    shortLabel: '买入',
    confirmLabel: '确认买入',
    description: '系统会按加权平均成本法自动重算最新均价。',
  },
  sell: {
    title: '卖出 / 减仓',
    shortLabel: '卖出',
    confirmLabel: '确认卖出',
    description: '卖出后只减少股数，剩余持仓均价保持不变。',
  },
  adjust: {
    title: '手动调整均价',
    shortLabel: '调均价',
    confirmLabel: '确认调整',
    description: '该操作不会改动持仓数量，只会校准当前买入均价。',
  },
}

function createTradeDrawerState(market = 'ASHARE', profile = null) {
  return {
    open: false,
    action: 'buy',
    market,
    symbolInput: '',
    lockedSymbol: false,
    form: createPortfolioActionForm('buy', null, { exchange: market, profile }),
    item: null,
    events: [],
    loading: false,
    saving: false,
    error: '',
    notice: '',
    deleteConfirmOpen: false,
    deleteConfirmValue: '',
  }
}

function mergeTradeContextItem(base, detail) {
  return {
    ...(base || {}),
    ...(detail || {}),
  }
}

function seedTradeContextFromPosition(position) {
  if (!position) return null
  return {
    symbol: position.symbol,
    name: position.name,
    exchange: position.exchange,
    shares: position.shares,
    avg_cost_price: position.avg_cost_price,
    total_cost_amount: position.total_cost_amount,
    buy_date: position.buy_date,
    note: position.note,
    cost_source: position.cost_source,
    last_trade_at: position.last_trade_at,
    last_event_id: position.last_event_id,
    last_price: position.last_price,
    market_value_amount: position.market_value_amount,
    total_pnl_amount: position.total_pnl_amount,
    total_pnl_pct: position.total_pnl_pct,
    today_pnl_amount: position.today_pnl_amount,
    can_sell: position.can_sell,
    can_adjust_avg_cost: position.can_adjust_avg_cost,
  }
}

function formatTradeDecimal(value, digits = 3) {
  if (typeof value !== 'number' || Number.isNaN(value)) return '--'
  return value.toLocaleString('zh-CN', {
    maximumFractionDigits: digits,
    minimumFractionDigits: digits,
  })
}

function formatTradeSignedMoney(value, exchange) {
  if (typeof value !== 'number' || Number.isNaN(value)) return '--'
  const prefix = value > 0 ? '+' : value < 0 ? '-' : ''
  return `${prefix}${formatMoney(Math.abs(value), exchange)}`
}

function buildTradeSuccessNotice(action) {
  if (action === 'sell') return '卖出记录已保存'
  if (action === 'adjust') return '均价调整已保存'
  return '买入记录已保存'
}

function eventTypeLabel(type) {
  switch (type) {
    case 'buy': return '买入'
    case 'sell': return '卖出'
    case 'adjust_avg_cost': return '均价调整'
    case 'init': return '初始化'
    case 'sync_position': return '校准'
    default: return '变动'
  }
}

function ManualMaintenanceNotice() {
  return (
    <div className="rounded-xl border border-white/8 bg-white/[0.025] px-4 py-3 sm:px-5">
      <div className="flex items-start gap-2.5 text-xs text-white/48">
        <div className="mt-0.5 rounded-full border border-white/10 bg-white/[0.03] p-1 text-white/35">
          <svg className="h-3.5 w-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8">
            <circle cx="12" cy="12" r="9" />
            <path d="M12 10v4" />
            <path d="M12 7.5h.01" />
          </svg>
        </div>
        <div className="min-w-0 leading-6">
          <div className="inline-flex items-center gap-1.5 text-white/58">
            <span>当前持仓数据仍以手动维护为主，暂未直连券商。</span>
            <InfoTip text={FIELD_TIPS.manual_notice} iconClassName="border-white/12 text-white/30 hover:border-white/25 hover:text-white/55" />
          </div>
          <p className="mt-0.5 text-[11px] text-white/32">
            你在个股详情页记录的买入、卖出和调均价，会同步汇总到这里；后续版本会逐步补上自动同步能力。
          </p>
        </div>
      </div>
    </div>
  )
}

// ── 空状态 ──

function EmptyPortfolio() {
  return (
    <div className="flex flex-col items-center justify-center py-20 text-center">
      <div className="w-16 h-16 rounded-full bg-white/5 flex items-center justify-center mb-4">
        <svg className="w-8 h-8 text-white/25" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
          <path d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
          <path d="M12 7v5l3 3" />
        </svg>
      </div>
      <h3 className="text-base font-medium text-white/60 mb-1">暂无持仓数据</h3>
      <p className="text-sm text-white/35 max-w-xs">
        在个股详情页添加持仓后，这里会展示你的组合全貌。
        <Link href="/live-trading" className="text-primary hover:text-primary/80 underline ml-1">去添加 →</Link>
      </p>
    </div>
  )
}

// ── 汇总卡片 ──

function buildMoneyMetric(value, exchange, { signed = false, compact = true } = {}) {
  if (typeof value !== 'number' || Number.isNaN(value)) {
    return { main: '--', detail: '', shortened: false }
  }

  const symbol = exchange === 'HKEX' ? 'HK$' : '¥'
  const abs = Math.abs(value)
  const sign = signed ? (value > 0 ? '+' : value < 0 ? '-' : '') : (value < 0 ? '-' : '')

  let compactText = `${sign}${symbol}${abs.toLocaleString('zh-CN', { maximumFractionDigits: 2, minimumFractionDigits: 2 })}`
  if (compact) {
    if (abs >= 1e8) {
      compactText = `${sign}${symbol}${(abs / 1e8).toFixed(abs >= 1e9 ? 1 : 2)}亿`
    } else if (abs >= 1e4) {
      compactText = `${sign}${symbol}${(abs / 1e4).toFixed(abs >= 1e6 ? 1 : 2)}万`
    }
  }

  const fullText = `${sign}${symbol}${abs.toLocaleString('zh-CN', { maximumFractionDigits: 2, minimumFractionDigits: 2 })}`
  return {
    main: compactText,
    detail: compactText === fullText ? '' : fullText,
    shortened: compactText !== fullText,
  }
}

function SummaryCard({ label, tooltip, value, footnote, accent, valueClassName = 'text-white/90', labelExtra }) {
  return (
    <div className={`rounded-2xl border p-4 sm:p-4.5 min-h-[112px] flex flex-col justify-between ${accent || 'border-white/10 bg-white/[0.03]'}`}>
      <div className="flex items-start justify-between gap-3">
        <div className="text-[11px] font-medium text-white/45 tracking-wide">
          <LabelWithInfo label={label} tooltip={tooltip} />
        </div>
        {labelExtra ? <div className="shrink-0">{labelExtra}</div> : null}
      </div>
      <div className={`mt-2 text-base sm:text-lg font-semibold leading-snug tracking-tight break-words ${valueClassName}`}>
        {value}
      </div>
      {footnote ? (
        <div className="mt-2 text-[11px] text-white/35 break-all leading-relaxed">{footnote}</div>
      ) : <div />}
    </div>
  )
}

function SummaryRow({ label, tooltip, value, valueClassName = 'text-white/85', footnote }) {
  return (
    <div className="rounded-xl bg-black/20 px-3 py-2">
      <div className="text-[11px] text-white/40">
        <LabelWithInfo label={label} tooltip={tooltip} />
      </div>
      <div className={`mt-1 text-sm font-medium break-words ${valueClassName}`}>{value}</div>
      {footnote ? <div className="mt-1 text-[10px] text-white/25 break-all">{footnote}</div> : null}
    </div>
  )
}

function MarketOverviewCard({ block }) {
  const marketValue = buildMoneyMetric(block.market_value_amount, block.scope)
  const totalCost = buildMoneyMetric(block.total_cost_amount, block.scope)
  const unrealized = buildMoneyMetric(block.unrealized_pnl_amount, block.scope, { signed: true })
  const realized = buildMoneyMetric(block.realized_pnl_amount, block.scope, { signed: true })
  const total = buildMoneyMetric(block.total_pnl_amount, block.scope, { signed: true })
  const today = buildMoneyMetric(block.today_pnl_amount, block.scope, { signed: true })

  return (
    <div className="rounded-2xl border border-white/10 bg-white/[0.03] p-4 sm:p-5">
      <div className="flex items-start justify-between gap-3">
        <div>
          <div className="text-xs font-semibold text-white/70">{block.scope_label} 持仓</div>
          <div className="mt-1 text-xl font-semibold text-white/95 tracking-tight">{marketValue.main}</div>
          <div className="mt-1 text-[11px] text-white/35 inline-flex items-center gap-1.5">
            <span>总市值 · {block.position_count || 0} 只标的</span>
            <InfoTip text={FIELD_TIPS.market_value} />
          </div>
        </div>
        <div className="rounded-full border border-white/10 px-2 py-0.5 text-[10px] text-white/45">
          {block.currency_code || (block.scope === 'HKEX' ? 'HKD' : 'CNY')}
        </div>
      </div>

      <div className="mt-4 grid grid-cols-2 gap-2.5">
        <SummaryRow label="总成本" tooltip={FIELD_TIPS.total_cost} value={totalCost.main} footnote={totalCost.detail} />
        <SummaryRow
          label="未实现盈亏"
          tooltip={FIELD_TIPS.unrealized_pnl}
          value={unrealized.main}
          footnote={unrealized.detail}
          valueClassName={block.unrealized_pnl_amount >= 0 ? 'text-rose-400' : 'text-emerald-400'}
        />
        <SummaryRow
          label="已实现盈亏"
          tooltip={FIELD_TIPS.realized_pnl}
          value={realized.main}
          footnote={realized.detail}
          valueClassName={block.realized_pnl_amount >= 0 ? 'text-rose-400' : 'text-emerald-400'}
        />
        <SummaryRow
          label="累计盈亏"
          tooltip={FIELD_TIPS.total_pnl}
          value={total.main}
          footnote={total.detail}
          valueClassName={block.total_pnl_amount >= 0 ? 'text-rose-400' : 'text-emerald-400'}
        />
        <SummaryRow
          label="今日盈亏"
          tooltip={FIELD_TIPS.today_pnl}
          value={today.main}
          footnote={today.detail}
          valueClassName={block.today_pnl_amount >= 0 ? 'text-rose-400' : 'text-emerald-400'}
        />
        <SummaryRow
          label="最大单仓占比"
          tooltip={FIELD_TIPS.max_position_weight}
          value={`${((block.max_position_weight_ratio || 0) * 100).toFixed(1)}%`}
          valueClassName={(block.max_position_weight_ratio || 0) >= 0.3 ? 'text-amber-400' : 'text-white/80'}
        />
      </div>
    </div>
  )
}

function SummarySection({ summary }) {
  if (!summary) return null

  const isMixed = summary.mixed_currency
  const singleAmounts = summary.amounts
  const marketBlocks = summary.amounts_by_market || []
  const totalCapital = typeof summary.total_capital_amount === 'number' ? buildMoneyMetric(summary.total_capital_amount, '') : null
  const latestTradeText = summary.latest_trade_at
    ? new Date(summary.latest_trade_at).toLocaleDateString('zh-CN')
    : '暂无'

  if (isMixed) {
    return (
      <section className="mb-6 space-y-4">
        <div className="grid grid-cols-2 lg:grid-cols-5 gap-3">
          <SummaryCard
            label="持仓标的"
            tooltip={FIELD_TIPS.position_count}
            value={`${summary.position_count || 0} 只`}
            footnote="全部市场统一统计"
          />
          <SummaryCard
            label="盈利 / 亏损"
            tooltip={FIELD_TIPS.profit_loss_count}
            value={<span><span className="text-rose-300">{summary.profit_position_count || 0}</span> / <span className="text-emerald-300">{summary.loss_position_count || 0}</span></span>}
            footnote="按单只股票累计盈亏口径"
          />
          <SummaryCard
            label="最大单仓占比"
            tooltip={FIELD_TIPS.max_position_weight}
            value={`${((summary.max_position_weight_ratio || 0) * 100).toFixed(1)}%`}
            valueClassName={(summary.max_position_weight_ratio || 0) >= 0.3 ? 'text-amber-400' : 'text-white/90'}
            footnote="跨市场不做汇率折算，仅作仓位集中度参考"
          />
          <SummaryCard
            label="最近交易"
            tooltip={FIELD_TIPS.latest_trade}
            value={latestTradeText}
            footnote="最近一笔有效交易时间"
          />
          <SummaryCard
            label="账户总资金（设置）"
            tooltip={FIELD_TIPS.total_capital}
            value={totalCapital ? totalCapital.main : '未设置'}
            footnote={totalCapital ? '来自设置页，作为组合预算参考，不参与 A/H 汇总折算' : <Link href="/settings" className="text-primary hover:text-primary/80 underline">去设置页补充账户总资金</Link>}
            accent="border-primary/20 bg-primary/[0.06]"
          />
        </div>

        <div className="rounded-2xl border border-white/10 bg-white/[0.03] p-4 sm:p-5">
          <div className="flex items-center justify-between gap-3 mb-4">
            <div>
              <h3 className="text-sm font-semibold text-white/80">分市场总览</h3>
              <p className="mt-1 text-xs text-white/35">选择「全部」时，金额类指标按市场分别展示，避免人民币与港币混排。</p>
            </div>
          </div>
          <div className="grid gap-3 xl:grid-cols-2">
            {marketBlocks.map((block) => (
              <MarketOverviewCard key={block.scope} block={block} />
            ))}
          </div>
        </div>
      </section>
    )
  }

  const exchange = summary.scope === 'HKEX' || singleAmounts?.currency_code === 'HKD' ? 'HKEX' : ''
  const marketValue = buildMoneyMetric(singleAmounts?.market_value_amount, exchange)
  const totalCost = buildMoneyMetric(singleAmounts?.total_cost_amount, exchange)
  const unrealized = buildMoneyMetric(singleAmounts?.unrealized_pnl_amount, exchange, { signed: true })
  const realized = buildMoneyMetric(singleAmounts?.realized_pnl_amount, exchange, { signed: true })
  const total = buildMoneyMetric(singleAmounts?.total_pnl_amount, exchange, { signed: true })
  const today = buildMoneyMetric(singleAmounts?.today_pnl_amount, exchange, { signed: true })

  return (
    <section className="mb-6 space-y-4">
      <div className="grid grid-cols-2 md:grid-cols-3 xl:grid-cols-6 gap-3">
        <SummaryCard label="总市值" tooltip={FIELD_TIPS.market_value} value={marketValue.main} footnote={marketValue.detail || `${summary.position_count || 0} 只标的`} />
        <SummaryCard label="总成本" tooltip={FIELD_TIPS.total_cost} value={totalCost.main} footnote={totalCost.detail} />
        <SummaryCard label="未实现盈亏" tooltip={FIELD_TIPS.unrealized_pnl} value={unrealized.main} footnote={unrealized.detail} valueClassName={(singleAmounts?.unrealized_pnl_amount || 0) >= 0 ? 'text-rose-400' : 'text-emerald-400'} />
        <SummaryCard label="已实现盈亏" tooltip={FIELD_TIPS.realized_pnl} value={realized.main} footnote={realized.detail} valueClassName={(singleAmounts?.realized_pnl_amount || 0) >= 0 ? 'text-rose-400' : 'text-emerald-400'} />
        <SummaryCard label="累计盈亏" tooltip={FIELD_TIPS.total_pnl} value={total.main} footnote={total.detail} valueClassName={(singleAmounts?.total_pnl_amount || 0) >= 0 ? 'text-rose-400' : 'text-emerald-400'} />
        <SummaryCard label="今日盈亏" tooltip={FIELD_TIPS.today_pnl} value={today.main} footnote={today.detail} valueClassName={(singleAmounts?.today_pnl_amount || 0) >= 0 ? 'text-rose-400' : 'text-emerald-400'} />
      </div>

      <div className="grid grid-cols-2 md:grid-cols-4 xl:grid-cols-5 gap-3">
        <SummaryCard
          label="盈利 / 亏损"
          tooltip={FIELD_TIPS.profit_loss_count}
          value={<span><span className="text-rose-300">{summary.profit_position_count || 0}</span> / <span className="text-emerald-300">{summary.loss_position_count || 0}</span></span>}
          footnote="按单只股票累计盈亏口径"
        />
        <SummaryCard
          label="最大单仓占比"
          tooltip={FIELD_TIPS.max_position_weight}
          value={`${((summary.max_position_weight_ratio || 0) * 100).toFixed(1)}%`}
          valueClassName={(summary.max_position_weight_ratio || 0) >= 0.3 ? 'text-amber-400' : 'text-white/90'}
          footnote="单市场口径"
        />
        <SummaryCard
          label="账户总资金（设置）"
          tooltip={FIELD_TIPS.total_capital}
          value={totalCapital ? totalCapital.main : '未设置'}
          footnote={totalCapital ? totalCapital.detail || '来自设置页，用于辅助评估资金使用情况' : <Link href="/settings" className="text-primary hover:text-primary/80 underline">去设置页补充账户总资金</Link>}
          accent="border-primary/20 bg-primary/[0.06]"
        />
        <SummaryCard
          label="资金利用率"
          tooltip={FIELD_TIPS.capital_usage}
          value={summary.capital_usage_ratio != null ? `${(summary.capital_usage_ratio * 100).toFixed(1)}%` : '—'}
          valueClassName={summary.capital_usage_ratio != null && summary.capital_usage_ratio >= 0.85 ? 'text-amber-400' : 'text-white/90'}
          footnote={summary.capital_usage_ratio != null ? '当前持仓市值 / 设置页账户总资金' : (exchange === 'HKEX' ? '港股视图暂不做汇率折算，因此不展示资金利用率' : '需先在设置页填写账户总资金')}
        />
        <SummaryCard label="最近交易" tooltip={FIELD_TIPS.latest_trade} value={latestTradeText} footnote="最近一笔有效交易时间" />
      </div>
    </section>
  )
}

// ── 持仓列表行 ──

function PositionRow({ item, onNavigate, onAction }) {
  const totalPnlColor = item.total_pnl_amount >= 0 ? 'text-rose-400' : item.total_pnl_amount < 0 ? 'text-emerald-400' : 'text-white/45'
  const todayPnlColor = item.today_pnl_amount >= 0 ? 'text-rose-400' : item.today_pnl_amount < 0 ? 'text-emerald-400' : 'text-white/45'

  const handleOpen = () => onNavigate(item.symbol)
  const handleKeyDown = (event) => {
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault()
      handleOpen()
    }
  }

  const handleActionClick = (action) => (event) => {
    event.stopPropagation()
    onAction?.(action, item)
  }

  return (
    <div
      role="button"
      tabIndex={0}
      onClick={handleOpen}
      onKeyDown={handleKeyDown}
      className="w-full cursor-pointer text-left rounded-lg border border-white/[0.07] hover:border-primary/30 hover:bg-white/[0.04] transition p-3 group"
    >
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2 mb-1">
            <span className="font-mono font-semibold text-sm text-white/90 truncate group-hover:text-primary transition">
              {item.symbol}
            </span>
            {exchangeTag(item.exchange)}
            <span className="truncate text-xs text-white/45 max-w-[120px]">{item.name}</span>
          </div>
          <div className="flex items-center gap-x-4 gap-y-1 flex-wrap text-xs text-white/40 mt-1">
            <span className="inline-flex items-center gap-1">持仓 <InfoTip text={FIELD_TIPS.shares} /> {Number(item.shares || 0).toLocaleString('zh-CN')} 股</span>
            <span className="inline-flex items-center gap-1">成本 <InfoTip text={FIELD_TIPS.avg_cost} /> {formatMoney(item.avg_cost_price, item.exchange)}</span>
            <span className="inline-flex items-center gap-1">现价 <InfoTip text={FIELD_TIPS.last_price} /> {item.last_price > 0 ? formatMoney(item.last_price, item.exchange) : '--'}</span>
            <span className="inline-flex items-center gap-1">市值 <InfoTip text={FIELD_TIPS.symbol_market_value} /> {item.market_value_amount > 0 ? formatCompactNumber(item.market_value_amount) : '--'}</span>
          </div>
        </div>

        <div className="text-right shrink-0 pt-0.5">
          <div className={`text-sm font-semibold ${totalPnlColor}`} title={FIELD_TIPS.symbol_total_pnl}>
            {item.total_pnl_amount != null ? formatCompactNumber(item.total_pnl_amount) : '--'}
          </div>
          <div className={`text-[11px] ${totalPnlColor}`}>
            {item.total_pnl_pct != null ? `${(item.total_pnl_pct * 100).toFixed(2)}%` : ''}
          </div>
          {item.today_pnl_amount !== 0 && (
            <div className={`text-[10px] mt-0.5 ${todayPnlColor}`} title={FIELD_TIPS.symbol_today_pnl}>
              今日 {item.today_pnl_amount > 0 ? '+' : ''}{formatCompactNumber(item.today_pnl_amount)}
            </div>
          )}
        </div>
      </div>

      <div className="mt-3 flex flex-wrap items-center justify-between gap-2 border-t border-white/[0.06] pt-3">
        <div className="text-[11px] text-white/30">
          {item.last_trade_at ? `最近变动：${new Date(item.last_trade_at).toLocaleDateString('zh-CN')}` : '点击整行可打开个股详情页'}
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <button
            type="button"
            onClick={handleActionClick('buy')}
            className="rounded-lg border border-primary/25 bg-primary/[0.08] px-2.5 py-1 text-[11px] font-medium text-primary transition hover:border-primary/45 hover:bg-primary/[0.14]"
          >
            买入
          </button>
          {item.can_sell ? (
            <button
              type="button"
              onClick={handleActionClick('sell')}
              className="rounded-lg border border-emerald-400/25 bg-emerald-500/[0.08] px-2.5 py-1 text-[11px] font-medium text-emerald-200 transition hover:border-emerald-300/45 hover:bg-emerald-500/[0.14]"
            >
              卖出
            </button>
          ) : null}
          {item.can_adjust_avg_cost ? (
            <button
              type="button"
              onClick={handleActionClick('adjust')}
              className="rounded-lg border border-sky-400/25 bg-sky-500/[0.08] px-2.5 py-1 text-[11px] font-medium text-sky-200 transition hover:border-sky-300/45 hover:bg-sky-500/[0.14]"
            >
              调均价
            </button>
          ) : null}
        </div>
      </div>
    </div>
  )
}

function PositionTable({ positions, onNavigate, onAction }) {
  if (!positions || positions.length === 0) {
    return (
      <div className="rounded-lg border border-white/[0.05] p-8 text-center text-sm text-white/30">
        当前筛选条件下无持仓记录
      </div>
    )
  }
  return (
    <div className="space-y-2">
      {positions.map((item) => (
        <PositionRow key={item.symbol} item={item} onNavigate={onNavigate} onAction={onAction} />
      ))}
    </div>
  )
}

function TradeMetric({ label, value, valueClassName = 'text-white/90', footnote }) {
  return (
    <div className="rounded-xl border border-white/8 bg-black/20 px-3 py-3">
      <div className="text-[11px] text-white/38">{label}</div>
      <div className={`mt-1 text-sm font-semibold break-words ${valueClassName}`}>{value}</div>
      {footnote ? <div className="mt-1 text-[10px] text-white/28 break-all">{footnote}</div> : null}
    </div>
  )
}

function PortfolioTradeDrawer({
  open,
  action,
  currentSymbol,
  market,
  symbolInput,
  symbolLocked,
  item,
  events,
  canDelete,
  form,
  preview,
  loading,
  saving,
  error,
  notice,
  priceAutoFilled,
  onPriceAutoFilledChange,
  onClose,
  onActionChange,
  onMarketChange,
  onSymbolInputChange,
  onFormChange,
  onSubmit,
  onUndo,
  onOpenDetail,
  onOpenDeleteConfirm,
}) {
  useEffect(() => {
    if (!open) return undefined
    const previousOverflow = document.body.style.overflow
    const handleKeyDown = (event) => {
      if (event.key === 'Escape') {
        onClose?.()
      }
    }
    document.body.style.overflow = 'hidden'
    document.addEventListener('keydown', handleKeyDown)
    return () => {
      document.body.style.overflow = previousOverflow
      document.removeEventListener('keydown', handleKeyDown)
    }
  }, [open, onClose])

  if (!open) return null

  const exchange = inferPortfolioTradeMarket(currentSymbol || symbolInput, market)
  const actionCopy = TRADE_ACTION_COPY[action] || TRADE_ACTION_COPY.buy
  const hasPosition = isPortfolioPositionActive(item)
  const totalPnlColor = (item?.total_pnl_amount || 0) >= 0 ? 'text-rose-400' : 'text-emerald-400'
  const detailHint = currentSymbol
    ? `将记录到 ${currentSymbol}`
    : '支持输入 600519 / 600519.SH / 700 / 00700.HK，系统会按市场自动补齐。'
  const hasTradeAmount = Number(form?.quantity) > 0 && Number(form?.price) > 0
  const feeHint = action !== 'adjust'
    ? (hasTradeAmount
      ? describePortfolioFeeEstimate({ exchange, feeEstimate: preview?.feeEstimate })
      : '填写数量和价格后，系统会自动估算本次手续费。')
    : ''

  return (
    <div className="fixed inset-0 z-[72]">
      <button
        type="button"
        aria-label="关闭持仓操作抽屉"
        onClick={onClose}
        className="absolute inset-0 bg-black/60 backdrop-blur-[2px]"
      />
      <div className="absolute inset-y-0 right-0 ml-auto flex h-full w-full max-w-2xl flex-col border-l border-white/10 bg-[#111419]/95 shadow-[-24px_0_64px_rgba(0,0,0,0.45)]">
        <div className="flex items-start justify-between gap-3 border-b border-white/8 px-5 py-4 sm:px-6">
          <div>
            <div className="text-lg font-semibold text-white">组合内直接记录持仓变动</div>
            <div className="mt-1 text-sm text-white/45">不用离开持仓页，就能完成买入、卖出和均价调整。</div>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="rounded-lg border border-white/10 bg-white/[0.03] px-2.5 py-1 text-xs text-white/55 transition hover:border-white/20 hover:text-white/85"
          >
            关闭
          </button>
        </div>

        <div className="flex-1 overflow-y-auto px-5 py-5 sm:px-6 custom-scrollbar">
          <div className="flex flex-wrap items-center gap-2">
            {Object.entries(TRADE_ACTION_COPY).map(([key, copy]) => {
              const disabled = saving || ((key === 'sell' || key === 'adjust') && !hasPosition)
              return (
                <button
                  key={key}
                  type="button"
                  disabled={disabled}
                  onClick={() => onActionChange?.(key)}
                  className={`rounded-full border px-3 py-1.5 text-xs font-medium transition ${
                    action === key
                      ? 'border-primary/35 bg-primary/[0.12] text-primary'
                      : 'border-white/10 bg-white/[0.03] text-white/55 hover:border-white/20 hover:text-white/80'
                  } ${disabled ? 'cursor-not-allowed opacity-35 hover:border-white/10 hover:text-white/55' : ''}`}
                >
                  {copy.shortLabel}
                </button>
              )
            })}
          </div>

          <div className="mt-4 rounded-2xl border border-white/10 bg-white/[0.03] p-4">
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div>
                <div className="text-sm font-semibold text-white">{actionCopy.title}</div>
                <div className="mt-1 text-xs leading-6 text-white/45">{actionCopy.description}</div>
              </div>
              {currentSymbol && canDelete ? (
                <button
                  type="button"
                  onClick={onOpenDetail}
                  className="rounded-lg border border-white/10 px-2.5 py-1 text-[11px] text-white/60 transition hover:border-white/20 hover:text-white"
                >
                  查看详情页
                </button>
              ) : null}
            </div>

            {!symbolLocked && action === 'buy' ? (
              <div className="mt-4 grid gap-3 md:grid-cols-[auto_1fr]">
                <div>
                  <div className="text-xs text-white/55">市场</div>
                  <div className="mt-1 flex items-center gap-1 rounded-xl border border-white/10 bg-black/20 p-1">
                    {TRADE_MARKET_OPTIONS.map((option) => (
                      <button
                        key={option.value}
                        type="button"
                        onClick={() => onMarketChange?.(option.value)}
                        className={`rounded-lg px-3 py-1.5 text-xs font-medium transition ${
                          market === option.value
                            ? 'bg-primary/[0.14] text-primary'
                            : 'text-white/50 hover:bg-white/[0.05] hover:text-white/85'
                        }`}
                      >
                        {option.label}
                      </button>
                    ))}
                  </div>
                </div>
                <label className="block">
                  <div className="text-xs text-white/55 inline-flex items-center gap-1.5">
                    股票代码
                    <InfoTip text={FIELD_TIPS.trade_symbol} />
                  </div>
                  <input
                    type="text"
                    value={symbolInput}
                    onChange={(event) => onSymbolInputChange?.(event.target.value)}
                    placeholder={market === 'HKEX' ? '例：700 或 00700.HK' : '例：600519 或 600519.SH'}
                    className="mt-1 block w-full rounded-xl border border-white/10 bg-black/25 px-3 py-2.5 text-sm text-white outline-none transition focus:border-primary/40"
                  />
                  <div className="mt-1 text-[11px] text-white/32">{detailHint}</div>
                </label>
              </div>
            ) : null}

            <div className="mt-4 rounded-xl border border-white/8 bg-black/20 px-4 py-4">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div>
                  <div className="flex items-center gap-2 text-sm font-semibold text-white/90">
                    <span className="font-mono">{currentSymbol || '--'}</span>
                    {currentSymbol ? exchangeTag(exchange) : null}
                  </div>
                  <div className="mt-1 text-xs text-white/38">{item?.name || (currentSymbol ? '当前未找到名称快照' : '先输入股票代码，再开始记录')}</div>
                </div>
                <div className={`rounded-full border px-2.5 py-0.5 text-[10px] ${hasPosition ? 'border-emerald-400/25 bg-emerald-500/[0.08] text-emerald-200' : 'border-white/10 bg-white/[0.04] text-white/45'}`}>
                  {hasPosition ? '当前有持仓' : '当前未持仓'}
                </div>
              </div>

              <div className="mt-4 grid gap-2.5 sm:grid-cols-2 xl:grid-cols-4">
                <TradeMetric label="当前持仓" value={`${Number(item?.shares || 0).toLocaleString('zh-CN')} 股`} />
                <TradeMetric label="当前均价" value={hasPosition ? formatMoney(item?.avg_cost_price || 0, exchange) : '--'} footnote={hasPosition ? `成本口径：${item?.cost_source === 'manual' ? '手动校准' : '加权平均'}` : ''} />
                <TradeMetric label="最新价" value={item?.last_price > 0 ? formatMoney(item.last_price, exchange) : '--'} />
                <TradeMetric
                  label="累计盈亏"
                  value={typeof item?.total_pnl_amount === 'number' ? formatTradeSignedMoney(item.total_pnl_amount, exchange) : '--'}
                  valueClassName={typeof item?.total_pnl_amount === 'number' ? totalPnlColor : 'text-white/90'}
                />
              </div>

              <div className="mt-3 text-[11px] leading-6 text-white/35">
                {currentSymbol
                  ? hasPosition
                    ? `最近变动：${item?.last_trade_at ? new Date(item.last_trade_at).toLocaleString('zh-CN', { hour12: false }) : '暂无'}${item?.note ? ` · 备注：${item.note}` : ''}`
                    : '当前没有剩余持仓，也可以直接补一笔新的买入记录重新建仓。'
                  : '还没有选定股票，先输入代码后系统会自动读取该标的的当前持仓状态。'}
              </div>
            </div>
          </div>

          {notice ? (
            <div className="mt-4 rounded-xl border border-emerald-400/25 bg-emerald-500/[0.08] px-4 py-3 text-sm text-emerald-200">
              {notice}
            </div>
          ) : null}

          {error ? (
            <div className="mt-4 rounded-xl border border-rose-400/25 bg-rose-500/[0.08] px-4 py-3 text-sm text-rose-200">
              {error}
            </div>
          ) : null}

          <div className="mt-4 rounded-2xl border border-white/10 bg-white/[0.03] p-4">
            <div className="text-sm font-semibold text-white">填写本次记录</div>
            <div className="mt-1 text-xs text-white/42">参数都按真实成交填写，保存后会即时刷新组合页总览、明细和最近交易。</div>

            {!currentSymbol ? (
              <div className="mt-4 rounded-xl border border-dashed border-white/10 px-4 py-5 text-sm text-white/40">
                先输入股票代码，再继续填写交易表单。
              </div>
            ) : (
              <>
                <div className="mt-4 grid gap-3 md:grid-cols-2">
                  <label className="block">
                    <span className="text-xs text-white/55">成交日期</span>
                    <input
                      type="date"
                      max={new Date().toISOString().split('T')[0]}
                      value={form.trade_date}
                      onChange={(event) => onFormChange?.({ trade_date: event.target.value })}
                      className="mt-1 block w-full rounded-lg border border-white/10 bg-black/25 px-3 py-2 text-sm text-white outline-none transition focus:border-primary/40"
                    />
                  </label>

                  {action !== 'adjust' ? (
                    <>
                      <label className="block">
                        <span className="text-xs text-white/55 inline-flex items-center gap-1.5">
                          {action === 'buy' ? '数量（股）' : '卖出数量（股）'}
                          <InfoTip text={FIELD_TIPS.trade_quantity} />
                        </span>
                        <input
                          type="number"
                          min="0"
                          step="any"
                          value={form.quantity}
                          onChange={(event) => onFormChange?.({ quantity: event.target.value })}
                          placeholder={action === 'buy' ? '例：200' : '例：100'}
                          className="mt-1 block w-full rounded-lg border border-white/10 bg-black/25 px-3 py-2 text-sm text-white outline-none transition focus:border-primary/40"
                        />
                      </label>
                      <label className="block">
                        <span className="text-xs text-white/55 inline-flex items-center gap-1.5">
                          成交价格
                          {priceAutoFilled ? (
                            <span className="rounded bg-primary/20 px-1.5 py-0.5 text-[10px] font-medium text-primary">自动</span>
                          ) : null}
                          <InfoTip text={FIELD_TIPS.trade_price} />
                        </span>
                        <input
                          type="number"
                          min="0"
                          step="any"
                          value={form.price}
                          onChange={(event) => {
                            onPriceAutoFilledChange?.(false)
                            onFormChange?.({ price: event.target.value })
                          }}
                          placeholder="例：12.35"
                          className="mt-1 block w-full rounded-lg border border-white/10 bg-black/25 px-3 py-2 text-sm text-white outline-none transition focus:border-primary/40"
                        />
                      </label>
                      <label className="block">
                        <span className="text-xs text-white/55 inline-flex items-center gap-1.5">
                          手续费率（%）
                          <span className="rounded-full border border-white/10 px-1.5 py-0.5 text-[10px] text-white/45">{describeFeeRate(preview?.feeRate ?? 0)}</span>
                          <InfoTip text={FIELD_TIPS.trade_fee} />
                        </span>
                        <input
                          type="number"
                          min="0"
                          step="any"
                          value={form.fee_rate}
                          onChange={(event) => onFormChange?.({ fee_rate: event.target.value })}
                          placeholder={action === 'buy' ? (exchange === 'HKEX' ? '默认 0.13' : '默认 0.03') : (exchange === 'HKEX' ? '默认 0.13' : '默认 0.08')}
                          className="mt-1 block w-full rounded-lg border border-white/10 bg-black/25 px-3 py-2 text-sm text-white outline-none transition focus:border-primary/40"
                        />
                        <div className="mt-1 text-[11px] leading-5 text-white/40">{feeHint}</div>
                      </label>
                    </>
                  ) : (
                    <label className="block md:col-span-2">
                      <span className="text-xs text-white/55 inline-flex items-center gap-1.5">
                        新的买入均价
                        <InfoTip text={FIELD_TIPS.trade_adjust_reason} />
                      </span>
                      <input
                        type="number"
                        min="0"
                        step="any"
                        value={form.avg_cost_price}
                        onChange={(event) => onFormChange?.({ avg_cost_price: event.target.value })}
                        placeholder="例：12.15"
                        className="mt-1 block w-full rounded-lg border border-white/10 bg-black/25 px-3 py-2 text-sm text-white outline-none transition focus:border-primary/40"
                      />
                    </label>
                  )}

                  <label className={`block ${action === 'adjust' ? 'md:col-span-2' : ''}`}>
                    <span className="text-xs text-white/55 inline-flex items-center gap-1.5">
                      {action === 'adjust' ? '调整原因（必填）' : '备注'}
                      {action === 'adjust' ? <InfoTip text={FIELD_TIPS.trade_adjust_reason} /> : null}
                    </span>
                    <input
                      type="text"
                      value={form.note}
                      onChange={(event) => onFormChange?.({ note: event.target.value })}
                      placeholder={action === 'adjust' ? '例：补录券商真实成本，已含手续费' : '例：盘中加仓'}
                      className="mt-1 block w-full rounded-lg border border-white/10 bg-black/25 px-3 py-2 text-sm text-white outline-none transition focus:border-primary/40"
                    />
                  </label>
                </div>

                <div className="mt-4 rounded-xl border border-white/8 bg-black/20 px-4 py-4">
                  <div className="text-xs font-medium text-white/65 inline-flex items-center gap-1.5">
                    结果预览
                    <InfoTip text={FIELD_TIPS.trade_preview} />
                  </div>
                  {preview?.errors?.length > 0 ? (
                    <div className="mt-2 text-xs text-amber-200">{preview.errors[0]}</div>
                  ) : (
                    <div className={`mt-3 grid gap-2.5 sm:grid-cols-2 ${action === 'adjust' ? 'xl:grid-cols-4' : 'xl:grid-cols-5'}`}>
                      <TradeMetric label="变动后股数" value={`${Number(preview?.nextShares || 0).toLocaleString('zh-CN')} 股`} />
                      <TradeMetric label="变动后均价" value={preview?.valid ? formatTradeDecimal(preview?.nextAvgCostPrice || 0, 3) : '--'} />
                      <TradeMetric label="变动后成本" value={preview?.valid ? formatMoney(preview?.nextTotalCostAmount || 0, exchange) : '--'} />
                      {action !== 'adjust' ? (
                        <TradeMetric
                          label="本次手续费"
                          value={preview?.valid ? formatPortfolioFeeAmount(preview?.feeAmount || 0, exchange) : '--'}
                          footnote={preview?.feeEstimate?.minimumApplied ? `低于最低佣金，按 ${formatPortfolioFeeAmount(preview?.feeEstimate?.minimumFeeAmount || 0, exchange)} 结算` : ''}
                        />
                      ) : null}
                      <TradeMetric
                        label={action === 'sell' ? '本次已实现收益' : '口径说明'}
                        value={action === 'sell'
                          ? formatTradeSignedMoney(preview?.realizedPnlAmount || 0, exchange)
                          : action === 'buy'
                            ? '买入后自动重算均价'
                            : '仅调整均价，不改股数'}
                        valueClassName={action === 'sell' ? ((preview?.realizedPnlAmount || 0) >= 0 ? 'text-rose-300' : 'text-emerald-300') : 'text-white/75'}
                      />
                    </div>
                  )}
                </div>
              </>
            )}
          </div>

          {currentSymbol ? (
            <div className="mt-4 rounded-2xl border border-white/10 bg-white/[0.03] p-4">
              <div className="flex items-center justify-between gap-3">
                <div>
                  <div className="text-sm font-semibold text-white">最近持仓变动</div>
                  <div className="mt-1 text-xs text-white/40">这里优先展示当前股票最近几笔操作，方便你在组合视角里快速回看和核对。</div>
                </div>
                {loading ? <div className="text-[11px] text-white/35">加载中...</div> : null}
              </div>

              {events && events.length > 0 ? (
                <div className="mt-3 space-y-2.5">
                  {events.slice(0, 6).map((event) => (
                    <div key={event.id} className="rounded-xl border border-white/8 bg-black/20 px-3 py-3">
                      <div className="flex flex-wrap items-start justify-between gap-2">
                        <div>
                          <div className="text-[11px] text-white/35">{event.trade_date}</div>
                          <div className={`mt-1 text-sm font-semibold ${getPortfolioEventAccent(event)}`}>{formatPortfolioEventHeadline(event, currentSymbol)}</div>
                          <div className="mt-1 text-xs leading-6 text-white/60">{formatPortfolioEventSubline(event, currentSymbol)}</div>
                        </div>
                        <div className="rounded-full border border-white/10 bg-black/20 px-2 py-0.5 text-[10px] text-white/55">
                          {eventTypeLabel(event.event_type)}
                        </div>
                      </div>
                      {(event.note || event.fee_amount > 0) ? (
                        <div className="mt-2 flex flex-wrap gap-3 text-[11px] text-white/35">
                          {event.note ? <span>备注：{event.note}</span> : null}
                          {event.fee_amount > 0 ? <span>手续费：{formatMoney(event.fee_amount, exchange)}</span> : null}
                        </div>
                      ) : null}
                    </div>
                  ))}
                </div>
              ) : (
                <div className="mt-3 rounded-xl border border-dashed border-white/10 px-4 py-4 text-sm text-white/35">
                  暂无该股票的持仓变动记录。
                </div>
              )}
            </div>
          ) : null}

          {currentSymbol && canDelete ? (
            <div className="mt-4 rounded-2xl border border-rose-400/20 bg-rose-500/[0.05] p-4">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div>
                  <div className="text-sm font-semibold text-rose-100">危险操作</div>
                  <div className="mt-1 text-xs leading-6 text-rose-100/70">
                    删除后，将清空该股票全部持仓记录，包括所有买入、卖出、调均价与初始化历史；该股票也会从当前持仓、最近交易、收益曲线和归因分析中移除，且无法恢复。
                  </div>
                </div>
                <button
                  type="button"
                  disabled={saving}
                  onClick={onOpenDeleteConfirm}
                  className="rounded-lg border border-rose-300/30 bg-rose-500/10 px-3 py-1.5 text-xs font-medium text-rose-200 transition hover:border-rose-200/50 hover:bg-rose-500/15 disabled:cursor-not-allowed disabled:opacity-50"
                >
                  删除该股票全部记录
                </button>
              </div>
            </div>
          ) : null}
        </div>

        <div className="border-t border-white/8 px-5 py-4 sm:px-6">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div className="flex flex-wrap items-center gap-2">
              {item?.last_event_id ? (
                <button
                  type="button"
                  disabled={saving}
                  onClick={onUndo}
                  className="rounded-lg border border-white/12 px-3 py-2 text-xs text-white/60 transition hover:border-white/25 hover:text-white disabled:cursor-not-allowed disabled:opacity-50"
                >
                  撤销最近一条
                </button>
              ) : null}
            </div>
            <div className="flex flex-wrap items-center gap-2">
              <button
                type="button"
                onClick={onClose}
                className="rounded-lg border border-white/12 px-4 py-2 text-xs text-white/60 transition hover:border-white/25 hover:text-white"
              >
                取消
              </button>
              <button
                type="button"
                disabled={saving || !currentSymbol}
                onClick={onSubmit}
                className="rounded-lg bg-primary px-4 py-2 text-xs font-medium text-white shadow-sm transition hover:bg-primary/85 disabled:cursor-not-allowed disabled:opacity-60"
              >
                {saving ? '保存中...' : actionCopy.confirmLabel}
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

// ── 最近交易 ──

function RecentEventsSection({ events }) {
  if (!events || events.length === 0) return null

  const eventTypeLabel = (type) => {
    switch (type) {
      case 'buy': return '买入'
      case 'sell': return '卖出'
      case 'adjust_avg_cost': return '调整均价'
      case 'init': return '初始化'
      default: return type
    }
  }

  const eventColor = (type) => {
    switch (type) {
      case 'buy': return 'bg-rose-500/15 text-rose-300 border-rose-500/20'
      case 'sell': return 'bg-emerald-500/15 text-emerald-300 border-emerald-500/20'
      case 'adjust_avg_cost': return 'bg-sky-500/15 text-sky-300 border-sky-500/20'
      default: return 'bg-white/[0.04] text-white/50 border-white/10'
    }
  }

  return (
    <section className="mt-8">
      <h3 className="text-sm font-semibold text-white/75 mb-3 flex items-center gap-2">
        <svg className="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M12 8v4l3 3"/><circle cx="12" cy="12" r="9"/></svg>
        <LabelWithInfo label="最近交易" tooltip={FIELD_TIPS.recent_events} />
        <span className="text-[10px] font-normal text-white/30">最近 {events.length} 条</span>
      </h3>
      <div className="space-y-1.5 max-h-[320px] overflow-y-auto pr-1 custom-scrollbar">
        {events.map((ev) => (
          <div key={ev.id} className="flex items-center gap-3 rounded-lg border border-white/[0.06] px-3 py-2 text-xs">
            <span className={`inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-medium border ${eventColor(ev.event_type)}`}>
              {eventTypeLabel(ev.event_type)}
            </span>
            <Link href={`/live-trading/${ev.symbol}`} className="font-mono font-semibold text-white/80 hover:text-primary transition">
              {ev.symbol}
            </Link>
            {ev.exchange_label && (
              <span className="text-[10px] text-white/30">{ev.exchange_label}</span>
            )}
            <span className="text-white/40 flex-1 truncate">
              {ev.quantity > 0 ? `${Number(ev.quantity).toLocaleString('zh-CN')} 股 @ ${ev.price}` : ''}
              {ev.note ? ` · ${ev.note}` : ''}
            </span>
            {ev.realized_pnl_amount !== 0 && (
              <span className={`${ev.realized_pnl_amount > 0 ? 'text-rose-400' : 'text-emerald-400'} shrink-0 font-mono`}>
                {ev.realized_pnl_amount > 0 ? '+' : ''}{ev.realized_pnl_amount.toLocaleString('zh-CN', { maximumFractionDigits: 2 })}
              </span>
            )}
            <span className="text-white/20 text-[10px] shrink-0">
              {new Date(ev.effective_at).toLocaleDateString('zh-CN')}
            </span>
          </div>
        ))}
      </div>
    </section>
  )
}

// ── 图表与分布 ──

function SimpleLineChart({ points, color = '#f59e0b' }) {
  if (!points || points.length === 0) {
    return <div className="flex items-center justify-center h-40 text-xs text-white/20">暂无数据</div>
  }

  const width = 100
  const height = 44
  const paddingX = 3
  const paddingY = 4
  const values = points.map((point) => point.market_value_amount || 0)
  const max = Math.max(...values, 1)
  const min = Math.min(...values, 0)
  const range = max - min || Math.max(Math.abs(max), 1)

  const coords = values.map((value, index) => {
    const x = paddingX + (index * (width - paddingX * 2)) / Math.max(values.length - 1, 1)
    const y = paddingY + ((max - value) / range) * (height - paddingY * 2)
    return { x, y }
  })

  const polyline = coords.map(({ x, y }) => `${x},${y}`).join(' ')
  const areaPath = [`M ${coords[0].x} ${height - paddingY}`, ...coords.map(({ x, y }) => `L ${x} ${y}`), `L ${coords[coords.length - 1].x} ${height - paddingY}`, 'Z'].join(' ')
  const last = coords[coords.length - 1]

  return (
    <svg viewBox={`0 0 ${width} ${height}`} className="w-full h-40 overflow-visible" preserveAspectRatio="none">
      <path d={areaPath} fill={color} opacity="0.12" />
      <line x1={paddingX} y1={height - paddingY} x2={width - paddingX} y2={height - paddingY} stroke="rgba(255,255,255,0.08)" strokeWidth="0.6" />
      <polyline fill="none" stroke={color} strokeWidth="2" strokeLinejoin="round" strokeLinecap="round" points={polyline} />
      <circle cx={last.x} cy={last.y} r="1.8" fill={color} />
    </svg>
  )
}

function EquityCurveCard({ series }) {
  const points = Array.isArray(series?.points) ? series.points : []
  if (points.length === 0) return null

  const exchange = series.scope === 'HKEX' ? 'HKEX' : ''
  const latest = points[points.length - 1]
  const first = points[0]
  const latestMetric = buildMoneyMetric(latest?.market_value_amount || 0, exchange)
  const deltaValue = (latest?.market_value_amount || 0) - (first?.market_value_amount || 0)
  const deltaMetric = buildMoneyMetric(deltaValue, exchange, { signed: true })
  const deltaClass = deltaValue >= 0 ? 'text-rose-400' : 'text-emerald-400'

  return (
    <div className="rounded-xl bg-black/20 p-3 sm:p-4">
      <div className="flex items-start justify-between gap-3 mb-2">
        <div>
          <div className="text-xs font-semibold text-white/70">{series.scope_label || scopeLabel(series.scope)}</div>
          <div className="mt-1 text-lg font-semibold text-white/95">{latestMetric.main}</div>
          <div className={`mt-1 text-xs ${deltaClass}`}>{deltaMetric.main}（区间变化）</div>
        </div>
        <div className="text-[10px] text-white/25 shrink-0">{series.currency_code || (series.scope === 'HKEX' ? 'HKD' : 'CNY')}</div>
      </div>
      <SimpleLineChart points={points} color={series.scope === 'HKEX' ? '#38bdf8' : '#f59e0b'} />
      <div className="mt-2 flex items-center justify-between text-[10px] text-white/30">
        <span>{first?.date || '--'}</span>
        <span>{latest?.date || '--'}</span>
      </div>
    </div>
  )
}

function EquityCurveSection({ curve, curveRange, setCurveRange }) {
  const series = Array.isArray(curve?.series) ? curve.series.filter((item) => Array.isArray(item?.points) && item.points.length > 0) : []
  if (series.length === 0) return null

  return (
    <section className="rounded-2xl border border-white/10 bg-white/[0.03] p-4 sm:p-5">
      <div className="mb-4 flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h3 className="text-sm font-semibold text-white/80 flex items-center gap-2">
            <svg className="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M3 17l6-6 4 4 7-8"/></svg>
            <LabelWithInfo label="资产曲线" tooltip={FIELD_TIPS.equity_curve} />
          </h3>
          <p className="mt-1 text-xs text-white/35">基于每日持仓快照生成，当前默认展示市值曲线。</p>
        </div>
        <div className="flex flex-col items-start gap-2 sm:items-end">
          <div className="inline-flex flex-wrap items-center gap-1 rounded-xl border border-white/10 bg-black/20 p-1">
            {CURVE_RANGE_OPTIONS.map((opt) => (
              <button
                key={opt.value}
                type="button"
                onClick={() => setCurveRange(opt.value)}
                className={`rounded-lg px-2.5 py-1 text-[11px] font-medium whitespace-nowrap transition ${
                  curveRange === opt.value
                    ? 'bg-primary/[0.14] text-primary'
                    : 'text-white/45 hover:bg-white/[0.05] hover:text-white/80'
                }`}
              >
                {opt.label}
              </button>
            ))}
          </div>
          {curve?.mixed_currency ? <div className="text-[10px] text-white/30 sm:text-right">分币种展示，不做汇率折算</div> : null}
        </div>
      </div>
      <div className={`grid gap-3 ${series.length > 1 ? 'xl:grid-cols-2' : ''}`}>
        {series.map((item) => (
          <EquityCurveCard key={item.scope} series={item} />
        ))}
      </div>
    </section>
  )
}

function AllocationBar({ allocationItems }) {
  if (!allocationItems || allocationItems.length === 0) return null

  const totalMarketValue = allocationItems.reduce((sum, it) => sum + (it.market_value_amount || 0), 0)

  return (
    <section className="rounded-2xl border border-white/10 bg-white/[0.03] p-4 sm:p-5">
      <h3 className="text-sm font-semibold text-white/80 mb-1 flex items-center gap-2">
        <svg className="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M18 20V10M12 20V4M6 20v-6"/></svg>
        <LabelWithInfo label="持仓分布" tooltip={FIELD_TIPS.allocation} />
      </h3>
      <p className="mb-4 text-xs text-white/35">按当前持仓市值估算前 {allocationItems.length} 大仓位权重。</p>
      <div className="space-y-2.5">
        {allocationItems.map((item) => {
          const ratio = totalMarketValue > 0 ? (item.market_value_amount || 0) / totalMarketValue : 0
          const pctStr = `${(ratio * 100).toFixed(1)}%`
          return (
            <div key={item.symbol}>
              <div className="flex items-center justify-between mb-1 text-xs">
                <div className="flex items-center gap-1.5 min-w-0">
                  <Link href={`/live-trading/${item.symbol}`} className="font-mono font-medium text-white/80 hover:text-primary transition truncate">
                    {item.symbol}
                  </Link>
                  <span className="text-white/30 truncate max-w-[80px]">{item.name}</span>
                  {exchangeTag(item.exchange)}
                </div>
                <div className="flex items-center gap-2 shrink-0 ml-2">
                  <span className="text-white/50 font-mono text-[10px]">{pctStr}</span>
                  <span className="text-white/65 text-[11px] w-20 text-right">{formatCompactNumber(item.market_value_amount)}</span>
                </div>
              </div>
              <div className="h-1.5 rounded-full bg-white/[0.05] overflow-hidden">
                <div
                  className="h-full rounded-full bg-gradient-to-r from-primary/70 to-primary/40 transition-all duration-500"
                  style={{ width: pctStr }}
                />
              </div>
            </div>
          )
        })}
      </div>
    </section>
  )
}

function PortfolioChartsSection({ curve, allocationItems, curveRange, setCurveRange }) {
  if ((!curve?.series || curve.series.length === 0) && (!allocationItems || allocationItems.length === 0)) {
    return null
  }

  return (
    <section className="grid gap-4 xl:grid-cols-[1.45fr_1fr]">
      <EquityCurveSection curve={curve} curveRange={curveRange} setCurveRange={setCurveRange} />
      <AllocationBar allocationItems={allocationItems} />
    </section>
  )
}

// ── 风险仪表盘 ──

function RiskSection({ riskMetrics }) {
  if (!riskMetrics) return null

  const formatPercent = (value) => typeof value === 'number' ? `${(value * 100).toFixed(1)}%` : '--'
  const formatScore = (value) => typeof value === 'number' ? value.toFixed(1) : '--'

  return (
    <section className="mb-6">
      <h3 className="text-sm font-semibold text-white/75 flex items-center gap-2 mb-3">
        <svg className="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
          <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/>
        </svg>
        组合风险分析
        <span className="text-[10px] font-normal text-white/30">
          {riskMetrics.scope === 'ALL' ? '全部市场' : riskMetrics.scope === 'ASHARE' ? 'A股' : '港股'} · 计算于 {new Date(riskMetrics.computed_at).toLocaleString('zh-CN', { hour12: false })}
        </span>
      </h3>

      {/* 总体风险评分 */}
      <div className="rounded-xl border border-white/10 bg-white/[0.03] p-4 mb-4">
        <div className="flex items-center justify-between">
          <div>
            <div className="text-sm font-medium text-white/60 mb-1">总体风险评分</div>
            <div className="text-xl font-semibold text-white/90">{(riskMetrics.overall_risk_score || 0).toFixed(1)} / 10</div>
            <div className="text-[10px] text-white/35 mt-1">
              评分越高风险越高，建议保持在 6 分以下
            </div>
          </div>
          <div className="relative">
            <div className="w-20 h-20 rounded-full border-4 border-white/10 flex items-center justify-center">
              <div className="text-xl font-semibold text-white/90">{(riskMetrics.overall_risk_score || 0).toFixed(1)}</div>
            </div>
            <div 
              className="absolute inset-0 w-20 h-20 rounded-full border-4 border-primary/60 clip-path-inset-0"
              style={{
                clipPath: `inset(0 ${100 - (riskMetrics.overall_risk_score || 0) * 10}% 0 0)`,
              }}
            />
          </div>
        </div>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-5 gap-3 mb-4">
        {/* 集中度风险 */}
        <div className="rounded-xl border border-white/10 bg-white/[0.03] p-3.5">
          <div className="text-[11px] font-medium text-white/45 tracking-wide mb-1">集中度风险</div>
          <div className="text-lg font-semibold text-white/90 mb-1">HHI: {formatScore(riskMetrics.concentration_risk?.herfindahl_index)}</div>
          <div className="text-[10px] text-white/35 space-y-0.5">
            <div>最大单股: {formatPercent(riskMetrics.concentration_risk?.single_stock_max_weight)}</div>
            <div>前三大: {formatPercent(riskMetrics.concentration_risk?.top3_weight)}</div>
            <div>前五大: {formatPercent(riskMetrics.concentration_risk?.top5_weight)}</div>
          </div>
          {riskMetrics.concentration_risk?.warnings?.length > 0 && (
            <div className="mt-2 text-[10px] text-amber-400">
              {riskMetrics.concentration_risk.warnings.map((w, i) => <div key={i}>⚠️ {w}</div>)}
            </div>
          )}
        </div>

        {/* 波动率风险 */}
        <div className="rounded-xl border border-white/10 bg-white/[0.03] p-3.5">
          <div className="text-[11px] font-medium text-white/45 tracking-wide mb-1">波动率风险</div>
          <div className="text-lg font-semibold text-white/90 mb-1">年化波动: {formatPercent(riskMetrics.volatility_risk?.annualized_volatility)}</div>
          <div className="text-[10px] text-white/35 space-y-0.5">
            <div>最大回撤: {formatPercent(riskMetrics.volatility_risk?.max_drawdown)}</div>
            <div>下跌概率: {formatPercent(riskMetrics.volatility_risk?.downside_probability)}</div>
            <div>单日VaR(95%): {formatPercent(riskMetrics.volatility_risk?.daily_var_95)}</div>
          </div>
          {riskMetrics.volatility_risk?.warnings?.length > 0 && (
            <div className="mt-2 text-[10px] text-amber-400">
              {riskMetrics.volatility_risk.warnings.map((w, i) => <div key={i}>⚠️ {w}</div>)}
            </div>
          )}
        </div>

        {/* 流动性风险 */}
        <div className="rounded-xl border border-white/10 bg-white/[0.03] p-3.5">
          <div className="text-[11px] font-medium text-white/45 tracking-wide mb-1">流动性风险</div>
          <div className="text-lg font-semibold text-white/90 mb-1">评分: {formatScore(riskMetrics.liquidity_risk?.liquidity_score)}</div>
          <div className="text-[10px] text-white/35 space-y-0.5">
            <div>日均成交额: {riskMetrics.liquidity_risk?.avg_daily_turnover?.toLocaleString('zh-CN') || '--'}</div>
            <div>换手率: {formatPercent(riskMetrics.liquidity_risk?.avg_turnover_rate)}</div>
            <div>低流动性标的: {riskMetrics.liquidity_risk?.illiquid_count || 0} 只</div>
          </div>
          {riskMetrics.liquidity_risk?.warnings?.length > 0 && (
            <div className="mt-2 text-[10px] text-amber-400">
              {riskMetrics.liquidity_risk.warnings.map((w, i) => <div key={i}>⚠️ {w}</div>)}
            </div>
          )}
        </div>

        {/* 尾部风险 */}
        <div className="rounded-xl border border-white/10 bg-white/[0.03] p-3.5">
          <div className="text-[11px] font-medium text-white/45 tracking-wide mb-1">尾部风险</div>
          <div className="text-lg font-semibold text-white/90 mb-1">ES(95%): {formatPercent(riskMetrics.tail_risk?.expected_shortfall_95)}</div>
          <div className="text-[10px] text-white/35 space-y-0.5">
            <div>单日VaR(95%): {formatPercent(riskMetrics.tail_risk?.var_95_one_day)}</div>
            <div>周度VaR(95%): {formatPercent(riskMetrics.tail_risk?.var_95_one_week)}</div>
            <div>最坏情况损失: {formatPercent(riskMetrics.tail_risk?.worst_case_loss)}</div>
          </div>
          {riskMetrics.tail_risk?.warnings?.length > 0 && (
            <div className="mt-2 text-[10px] text-amber-400">
              {riskMetrics.tail_risk.warnings.map((w, i) => <div key={i}>⚠️ {w}</div>)}
            </div>
          )}
        </div>

        {/* 相关性风险 */}
        <div className="rounded-xl border border-white/10 bg-white/[0.03] p-3.5">
          <div className="text-[11px] font-medium text-white/45 tracking-wide mb-1">相关性风险</div>
          <div className="text-lg font-semibold text-white/90 mb-1">平均相关性: {formatScore(riskMetrics.correlation_risk?.avg_correlation)}</div>
          <div className="text-[10px] text-white/35 space-y-0.5">
            <div>分散化评分: {formatScore(riskMetrics.correlation_risk?.diversification_score)}</div>
            <div>高相关性股票对: {riskMetrics.correlation_risk?.high_correlation_pairs?.length || 0} 对</div>
          </div>
          {riskMetrics.correlation_risk?.warnings?.length > 0 && (
            <div className="mt-2 text-[10px] text-amber-400">
              {riskMetrics.correlation_risk.warnings.map((w, i) => <div key={i}>⚠️ {w}</div>)}
            </div>
          )}
        </div>
      </div>
    </section>
  )
}

// ── 工具栏 ──

function Toolbar({
  scope,
  setScope,
  sortBy,
  setSortBy,
  sortOrder,
  setSortOrder,
  pnlFilter,
  setPnlFilter,
  keyword,
  setKeyword,
}) {
  return (
    <section className="mb-5 space-y-3">
      <div className="-mx-1 overflow-x-auto pb-1 sm:mx-0 sm:overflow-visible sm:pb-0">
        <div className="inline-flex min-w-full items-center gap-1 rounded-xl border border-white/10 bg-white/[0.03] p-1 sm:min-w-0">
          {SCOPE_OPTIONS.map((opt) => (
            <button
              key={opt.value}
              type="button"
              onClick={() => setScope(opt.value)}
              className={`flex-none whitespace-nowrap rounded-lg px-3.5 py-1.5 text-xs font-medium transition ${
                scope === opt.value
                  ? 'border border-primary/30 bg-primary/20 text-primary'
                  : 'text-white/50 hover:bg-white/[0.05] hover:text-white/80'
              }`}
            >
              {opt.label}
            </button>
          ))}
        </div>
      </div>

      <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
        <input
          type="text"
          placeholder="搜索代码或名称..."
          value={keyword}
          onChange={(e) => setKeyword(e.target.value)}
          className="w-full rounded-xl border border-white/10 bg-white/[0.04] px-3.5 py-2 text-sm text-white placeholder-white/25 outline-none transition focus:border-primary/40 focus:ring-1 focus:ring-primary/20 sm:max-w-[240px] sm:text-xs sm:py-1.5"
        />

        <div className="grid grid-cols-[minmax(0,1fr)_42px_minmax(0,1fr)] gap-2 sm:flex sm:items-center sm:gap-2">
          <select
            value={sortBy}
            onChange={(e) => setSortBy(e.target.value)}
            className="min-w-0 rounded-xl border border-white/10 bg-white/[0.04] px-3 py-2 text-sm text-white/75 outline-none transition focus:border-primary/40 cursor-pointer sm:text-xs sm:py-1.5"
          >
            {SORT_OPTIONS.map((opt) => (
              <option key={opt.value} value={opt.value}>{opt.label}</option>
            ))}
          </select>

          <button
            type="button"
            onClick={() => setSortOrder(sortOrder === 'desc' ? 'asc' : 'desc')}
            className="rounded-xl border border-white/10 bg-white/[0.04] px-0 py-2 text-sm text-white/60 transition hover:text-white/90 sm:w-10 sm:text-xs sm:py-1.5"
            title={sortOrder === 'desc' ? '当前降序，点击切换为升序' : '当前升序，点击切换为降序'}
            aria-label={sortOrder === 'desc' ? '切换为升序' : '切换为降序'}
          >
            {sortOrder === 'desc' ? '↓' : '↑'}
          </button>

          <select
            value={pnlFilter}
            onChange={(e) => setPnlFilter(e.target.value)}
            className="min-w-0 rounded-xl border border-white/10 bg-white/[0.04] px-3 py-2 text-sm text-white/75 outline-none transition focus:border-primary/40 cursor-pointer sm:text-xs sm:py-1.5"
          >
            {PNL_FILTER_OPTIONS.map((opt) => (
              <option key={opt.value} value={opt.value}>{opt.label}</option>
            ))}
          </select>
        </div>
      </div>
    </section>
  )
}

// ── 主页面 ──

export default function PortfolioPage() {
  const { isLoggedIn, ready } = useAuth()
  const [data, setData] = useState(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)
  const [riskMetrics, setRiskMetrics] = useState(null)
  const [attribution, setAttribution] = useState(EMPTY_ATTRIBUTION)
  const [attributionLoading, setAttributionLoading] = useState(true)
  const [attributionError, setAttributionError] = useState('')
  const [attributionDetailLoading, setAttributionDetailLoading] = useState(EMPTY_ATTRIBUTION_DETAIL_LOADING)
  const [attributionDetailError, setAttributionDetailError] = useState(EMPTY_ATTRIBUTION_DETAIL_ERROR)

  const [scope, setScope] = useState('ALL')
  const [sortBy, setSortBy] = useState('market_value')
  const [sortOrder, setSortOrder] = useState('desc')
  const [pnlFilter, setPnlFilter] = useState('all')
  const [curveRange, setCurveRange] = useState('30D')
  const [keyword, setKeyword] = useState('')
  const [investmentProfile, setInvestmentProfile] = useState(null)
  const [tradeDrawer, setTradeDrawer] = useState(() => createTradeDrawerState('ASHARE'))
  const [priceAutoFilled, setPriceAutoFilled] = useState(false)
  const [tradeDailyBars, setTradeDailyBars] = useState([])
  const [pageNotice, setPageNotice] = useState('')
  const tradeDailyBarsRef = useRef(tradeDailyBars)
  const attributionRequestKeyRef = useRef('')
  tradeDailyBarsRef.current = tradeDailyBars

  const defaultTradeMarket = scope === 'HKEX' ? 'HKEX' : 'ASHARE'

  const buildAttributionQuery = useCallback(() => ({
    scope,
    range: curveRange,
    limit: 5,
    timeline_limit: 8,
  }), [scope, curveRange])

  const loadInvestmentProfile = useCallback(async () => {
    try {
      const data = await requestJson('/api/investment-profile')
      setInvestmentProfile(data?.profile || null)
    } catch {
      setInvestmentProfile(null)
    }
  }, [])

  const load = useCallback(async () => {
    setLoading(true)
    setAttributionLoading(true)
    setError(null)
    setAttributionError('')
    setAttribution((prev) => (prev?.summary ? { ...EMPTY_ATTRIBUTION, summary: prev.summary } : EMPTY_ATTRIBUTION))
    setAttributionDetailLoading(EMPTY_ATTRIBUTION_DETAIL_LOADING)
    setAttributionDetailError(EMPTY_ATTRIBUTION_DETAIL_ERROR)

    const attributionQuery = buildAttributionQuery()
    const requestKey = JSON.stringify(attributionQuery)
    attributionRequestKeyRef.current = requestKey

    try {
      const [result, riskResult, summaryResult] = await Promise.all([
        fetchPortfolioDashboard({
          scope,
          sort_by: sortBy,
          sort_order: sortOrder,
          pnl_filter: pnlFilter,
          keyword,
          curve_range: curveRange,
        }),
        fetchPortfolioRiskMetrics({ scope }).catch((err) => {
          console.warn('风险指标加载失败:', err)
          return null
        }),
        fetchPortfolioAttributionSummary(attributionQuery).catch((err) => ({ __error: err })),
      ])

      setData(result)
      setRiskMetrics(riskResult)

      if (attributionRequestKeyRef.current === requestKey) {
        if (summaryResult?.__error) {
          setAttribution(EMPTY_ATTRIBUTION)
          setAttributionError(summaryResult.__error?.message || '加载绩效归因失败')
        } else {
          setAttribution({ ...EMPTY_ATTRIBUTION, summary: summaryResult || null })
        }
      }
    } catch (err) {
      setError(err.message || '加载失败')
    } finally {
      setLoading(false)
      setAttributionLoading(false)
    }
  }, [buildAttributionQuery, curveRange, keyword, pnlFilter, scope, sortBy, sortOrder])

  const ensureAttributionDetails = useCallback(async (keys = []) => {
    const requestedKeys = Array.from(new Set((Array.isArray(keys) ? keys : [keys]).filter((key) => ATTRIBUTION_FETCHERS[key])))
    const pendingKeys = requestedKeys.filter((key) => !attribution[key] && !attributionDetailLoading[key])
    if (!pendingKeys.length) return

    const requestKey = attributionRequestKeyRef.current
    const attributionQuery = buildAttributionQuery()

    setAttributionDetailLoading((prev) => {
      const next = { ...prev }
      pendingKeys.forEach((key) => {
        next[key] = true
      })
      return next
    })
    setAttributionDetailError((prev) => {
      const next = { ...prev }
      pendingKeys.forEach((key) => {
        next[key] = ''
      })
      return next
    })

    const results = await Promise.allSettled(
      pendingKeys.map((key) => ATTRIBUTION_FETCHERS[key](attributionQuery))
    )

    if (attributionRequestKeyRef.current !== requestKey) {
      return
    }

    setAttribution((prev) => {
      const next = { ...prev }
      results.forEach((result, index) => {
        const key = pendingKeys[index]
        if (result.status === 'fulfilled') {
          next[key] = result.value
        }
      })
      return next
    })

    setAttributionDetailError((prev) => {
      const next = { ...prev }
      results.forEach((result, index) => {
        const key = pendingKeys[index]
        next[key] = result.status === 'rejected' ? (result.reason?.message || '加载失败') : ''
      })
      return next
    })

    setAttributionDetailLoading((prev) => {
      const next = { ...prev }
      pendingKeys.forEach((key) => {
        next[key] = false
      })
      return next
    })
  }, [attribution, attributionDetailLoading, buildAttributionQuery])

  useEffect(() => {
    load()
  }, [load])

  useEffect(() => {
    if (!ready || !isLoggedIn) {
      setInvestmentProfile(null)
      return
    }
    const cached = readInvestmentProfileCache()
    if (cached) {
      setInvestmentProfile(cached)
    }
    loadInvestmentProfile()
  }, [ready, isLoggedIn, loadInvestmentProfile])

  useEffect(() => {
    if (!ready || !isLoggedIn) return undefined
    return subscribeInvestmentProfileUpdates((profile) => {
      setInvestmentProfile(profile)
      setTradeDrawer((prev) => {
        if (!prev.open || prev.action === 'adjust') return prev
        const nextExchange = inferPortfolioTradeMarket(prev.symbolInput, prev.market)
        return {
          ...prev,
          form: {
            ...prev.form,
            exchange: nextExchange,
            fee_rate: formatFeeRatePercent(getPortfolioDefaultFeeRate({ exchange: nextExchange, action: prev.action, profile })),
          },
        }
      })
    })
  }, [ready, isLoggedIn])

  const pageViewState = useMemo(() => getPortfolioPageViewState({ loading, data }), [loading, data])

  const currentTradeSymbol = useMemo(
    () => resolvePortfolioTradeSymbol(tradeDrawer.symbolInput, tradeDrawer.market),
    [tradeDrawer.symbolInput, tradeDrawer.market]
  )

  const tradeSnapshotItem = useMemo(() => {
    if (!currentTradeSymbol || !Array.isArray(data?.positions)) return null
    return data.positions.find((item) => item.symbol === currentTradeSymbol) || null
  }, [data?.positions, currentTradeSymbol])

  const tradeItem = useMemo(
    () => mergeTradeContextItem(tradeSnapshotItem, tradeDrawer.item),
    [tradeSnapshotItem, tradeDrawer.item]
  )

  const tradePreview = useMemo(() => {
    if (!tradeDrawer.open || !tradeDrawer.action) return null
    return buildPortfolioEventPreview(tradeDrawer.action, tradeItem, tradeDrawer.form)
  }, [tradeDrawer.open, tradeDrawer.action, tradeItem, tradeDrawer.form])

  const canDeleteTradeHistory = useMemo(() => (
    Boolean(tradeItem?.symbol) || (Array.isArray(tradeDrawer.events) && tradeDrawer.events.length > 0)
  ), [tradeDrawer.events, tradeItem?.symbol])

  const syncTradeContext = useCallback(async (symbol, { showLoading = true } = {}) => {
    const normalized = String(symbol || '').trim().toUpperCase()
    if (!normalized) {
      setTradeDrawer((prev) => ({
        ...prev,
        item: prev.lockedSymbol ? prev.item : null,
        events: [],
        loading: false,
        error: '',
      }))
      return { item: null, events: [] }
    }

    if (showLoading) {
      setTradeDrawer((prev) => {
        const activeSymbol = resolvePortfolioTradeSymbol(prev.symbolInput, prev.market)
        if (!prev.open || activeSymbol !== normalized) return prev
        return { ...prev, loading: true, error: '' }
      })
    }

    const [detail, timeline] = await Promise.all([
      fetchPortfolioDetail(normalized),
      fetchPortfolioEventTimeline(normalized, { limit: 6 }),
    ])
    const nextItem = detail?.item || null
    const nextEvents = Array.isArray(timeline?.items) ? timeline.items : []

    setTradeDrawer((prev) => {
      const activeSymbol = resolvePortfolioTradeSymbol(prev.symbolInput, prev.market)
      if (!prev.open || activeSymbol !== normalized) return prev
      return {
        ...prev,
        item: mergeTradeContextItem(prev.item, nextItem),
        events: nextEvents,
        loading: false,
        error: '',
      }
    })

    return { item: nextItem, events: nextEvents }
  }, [])

  useEffect(() => {
    if (!tradeDrawer.open) return
    if (!currentTradeSymbol) {
      setTradeDrawer((prev) => ({
        ...prev,
        item: prev.lockedSymbol ? prev.item : null,
        events: [],
        loading: false,
        error: '',
      }))
      return
    }

    const timer = setTimeout(() => {
      syncTradeContext(currentTradeSymbol).catch((err) => {
        setTradeDrawer((prev) => {
          const activeSymbol = resolvePortfolioTradeSymbol(prev.symbolInput, prev.market)
          if (!prev.open || activeSymbol !== currentTradeSymbol) return prev
          return {
            ...prev,
            loading: false,
            error: err.message || '加载该股票持仓信息失败',
          }
        })
      })
    }, tradeDrawer.lockedSymbol ? 0 : 250)

    return () => clearTimeout(timer)
  }, [tradeDrawer.open, tradeDrawer.lockedSymbol, currentTradeSymbol, syncTradeContext])

  // ── 自动填充成交价格（500ms 防抖）──
  useEffect(() => {
    if (!tradeDrawer.open || !currentTradeSymbol || !tradeDrawer.form.trade_date) return
    if (tradeDrawer.action === 'adjust') return // 调均价不需要成交价格

    const handler = (symbol, tradeDate) => {
      const today = new Date().toISOString().split('T')[0]
      if (tradeDate >= today) {
        // 今天或之后：用实时价
        fetchSymbolSnapshot(symbol).then(snapshot => {
          if (snapshot?.last_price > 0) {
            setTradeDrawer(prev => {
              if (!prev.open || resolvePortfolioTradeSymbol(prev.symbolInput, prev.market) !== symbol) return prev
              return { ...prev, form: { ...prev.form, price: String(Math.round(snapshot.last_price * 100) / 100) } }
            })
            setPriceAutoFilled(true)
          }
        }).catch(() => {})
      } else {
        // 历史日期：尝试从历史日线取收盘价
        const cached = tradeDailyBarsRef.current
        const needFetch = !cached || cached.length === 0
        const promise = needFetch
          ? fetchSymbolDailyBars(symbol, 260).then(bars => {
              setTradeDailyBars(bars)
              return bars
            })
          : Promise.resolve(cached)

        promise.then(bars => {
          const closePrice = findClosePriceByDate(bars, tradeDate)
          if (closePrice !== null) {
            setTradeDrawer(prev => {
              if (!prev.open || resolvePortfolioTradeSymbol(prev.symbolInput, prev.market) !== symbol) return prev
              return { ...prev, form: { ...prev.form, price: String(closePrice) } }
            })
            setPriceAutoFilled(true)
          }
        }).catch(() => {})
      }
    }

    const timer = setTimeout(() => {
      handler(currentTradeSymbol, tradeDrawer.form.trade_date)
    }, 500)

    return () => clearTimeout(timer)
  }, [tradeDrawer.open, tradeDrawer.action, currentTradeSymbol, tradeDrawer.form.trade_date])

  const openTradeDrawer = useCallback((action = 'buy', position = null) => {
    const market = position?.exchange === 'HKEX' ? 'HKEX' : defaultTradeMarket
    setPriceAutoFilled(false)
    setTradeDrawer({
      ...createTradeDrawerState(market, investmentProfile),
      open: true,
      action,
      market,
      symbolInput: position?.symbol || '',
      lockedSymbol: Boolean(position),
      form: createPortfolioActionForm(action, position || null, { exchange: market, profile: investmentProfile }),
      item: seedTradeContextFromPosition(position),
      loading: Boolean(position?.symbol),
    })
  }, [defaultTradeMarket, investmentProfile])

  const closeTradeDrawer = useCallback(() => {
    setTradeDrawer(createTradeDrawerState(defaultTradeMarket, investmentProfile))
  }, [defaultTradeMarket, investmentProfile])

  const handleOpenDeleteConfirm = useCallback(() => {
    if (!currentTradeSymbol) return
    setTradeDrawer((prev) => ({
      ...prev,
      deleteConfirmOpen: true,
      deleteConfirmValue: '',
      error: '',
      notice: '',
    }))
  }, [currentTradeSymbol])

  const handleDeleteConfirmValueChange = useCallback((value) => {
    setTradeDrawer((prev) => ({ ...prev, deleteConfirmValue: value }))
  }, [])

  const handleCloseDeleteConfirm = useCallback(() => {
    setTradeDrawer((prev) => ({
      ...prev,
      deleteConfirmOpen: false,
      deleteConfirmValue: '',
    }))
  }, [])

  const handleTradeActionChange = useCallback((nextAction) => {
    setTradeDrawer((prev) => ({
      ...prev,
      action: nextAction,
      form: createPortfolioActionForm(nextAction, tradeItem, { exchange: inferPortfolioTradeMarket(prev.symbolInput, prev.market), profile: investmentProfile }),
      error: '',
      notice: '',
    }))
  }, [investmentProfile, tradeItem])

  const handleTradeMarketChange = useCallback((nextMarket) => {
    setTradeDrawer((prev) => {
      const inferredExchange = inferPortfolioTradeMarket(prev.symbolInput, nextMarket)
      return {
        ...prev,
        market: nextMarket,
        form: prev.action === 'adjust'
          ? { ...prev.form, exchange: inferredExchange }
          : {
              ...prev.form,
              exchange: inferredExchange,
              fee_rate: formatFeeRatePercent(getPortfolioDefaultFeeRate({ exchange: inferredExchange, action: prev.action, profile: investmentProfile })),
            },
        error: '',
        notice: '',
      }
    })
  }, [investmentProfile])

  const handleTradeSymbolInputChange = useCallback((nextValue) => {
    setTradeDrawer((prev) => {
      const normalizedInput = String(nextValue || '').toUpperCase().trim()
      const inferredExchange = inferPortfolioTradeMarket(normalizedInput, prev.market)
      return {
        ...prev,
        symbolInput: normalizedInput,
        form: prev.action === 'adjust'
          ? { ...prev.form, exchange: inferredExchange }
          : {
              ...prev.form,
              exchange: inferredExchange,
              fee_rate: formatFeeRatePercent(getPortfolioDefaultFeeRate({ exchange: inferredExchange, action: prev.action, profile: investmentProfile })),
            },
        error: '',
        notice: '',
      }
    })
  }, [investmentProfile])

  const handleTradeFormChange = useCallback((patch) => {
    setTradeDrawer((prev) => ({
      ...prev,
      form: { ...prev.form, ...patch },
      error: '',
      notice: '',
    }))
  }, [])

  const handlePriceAutoFilledChange = useCallback((value) => {
    setPriceAutoFilled(value)
  }, [])

  const handleSubmitTrade = useCallback(async () => {
    if (!currentTradeSymbol) {
      setTradeDrawer((prev) => ({ ...prev, error: '请先输入股票代码' }))
      return
    }

    const preview = buildPortfolioEventPreview(tradeDrawer.action, tradeItem, tradeDrawer.form)
    if (!preview.valid) {
      setTradeDrawer((prev) => ({ ...prev, error: preview.errors[0] || '请先检查输入内容' }))
      return
    }

    const payload = {
      event_type: tradeDrawer.action === 'adjust' ? 'adjust_avg_cost' : tradeDrawer.action,
      trade_date: tradeDrawer.form.trade_date,
      note: tradeDrawer.form.note || '',
    }
    if (tradeDrawer.action === 'adjust') {
      payload.avg_cost_price = Number(tradeDrawer.form.avg_cost_price)
    } else {
      payload.quantity = Number(tradeDrawer.form.quantity)
      payload.price = Number(tradeDrawer.form.price)
      payload.fee_amount = Number(preview.feeAmount || 0)
    }

    setTradeDrawer((prev) => ({ ...prev, saving: true, error: '', notice: '' }))

    try {
      await createPortfolioEvent(currentTradeSymbol, payload)
      await load()
      const refreshed = await syncTradeContext(currentTradeSymbol, { showLoading: false })
      const mergedItem = mergeTradeContextItem(tradeItem, refreshed.item)
      const successNotice = buildTradeSuccessNotice(tradeDrawer.action)
      setTradeDrawer((prev) => ({
        ...prev,
        item: mergedItem,
        events: refreshed.events,
        form: createPortfolioActionForm(prev.action, mergedItem, { exchange: mergedItem?.exchange || prev.market, profile: investmentProfile }),
        saving: false,
        error: '',
        notice: successNotice,
      }))
    } catch (err) {
      setTradeDrawer((prev) => ({
        ...prev,
        saving: false,
        error: err.message || '保存失败',
      }))
    }
  }, [currentTradeSymbol, tradeDrawer.action, tradeDrawer.form, tradeItem, load, syncTradeContext])

  const handleUndoTrade = useCallback(async () => {
    if (!currentTradeSymbol || !tradeItem?.last_event_id) return
    if (!window.confirm('确定撤销最近一条持仓变动记录吗？')) return

    setTradeDrawer((prev) => ({ ...prev, saving: true, error: '', notice: '' }))
    try {
      await undoPortfolioEvent(currentTradeSymbol, tradeItem.last_event_id)
      await load()
      const refreshed = await syncTradeContext(currentTradeSymbol, { showLoading: false })
      const mergedItem = mergeTradeContextItem(tradeItem, refreshed.item)
      setTradeDrawer((prev) => ({
        ...prev,
        item: mergedItem,
        events: refreshed.events,
        form: createPortfolioActionForm(prev.action, mergedItem, { exchange: mergedItem?.exchange || prev.market, profile: investmentProfile }),
        saving: false,
        error: '',
        notice: '已撤销最近一条持仓变动记录',
      }))
    } catch (err) {
      setTradeDrawer((prev) => ({
        ...prev,
        saving: false,
        error: err.message || '撤销失败',
      }))
    }
  }, [currentTradeSymbol, tradeItem, load, syncTradeContext])

  const handleDeleteTradeHistory = useCallback(async () => {
    if (!currentTradeSymbol) return
    const expected = buildPortfolioDeleteConfirmText(currentTradeSymbol)
    if (tradeDrawer.deleteConfirmValue.trim().toUpperCase() !== expected) {
      setTradeDrawer((prev) => ({ ...prev, error: `请输入 ${expected} 以确认删除` }))
      return
    }

    setTradeDrawer((prev) => ({ ...prev, saving: true, error: '', notice: '' }))
    try {
      await deletePortfolioHistory(currentTradeSymbol)
      await load()
      setPageNotice(`${currentTradeSymbol} 的全部持仓记录已删除，相关持仓、最近交易、收益曲线与归因分析已同步刷新。`)
      setTradeDrawer(createTradeDrawerState(defaultTradeMarket, investmentProfile))
      setPriceAutoFilled(false)
    } catch (err) {
      setTradeDrawer((prev) => ({
        ...prev,
        saving: false,
        error: err.message || '删除失败',
      }))
    }
  }, [currentTradeSymbol, defaultTradeMarket, investmentProfile, load, tradeDrawer.deleteConfirmValue])

  function handleNavigate(symbol) {
    window.open(`/live-trading/${symbol}`, '_blank')
  }

  function handleOpenTradeDetail() {
    if (!currentTradeSymbol) return
    window.open(`/live-trading/${currentTradeSymbol}`, '_blank')
  }

  if (!ready) {
    return (
      <div className="flex items-center justify-center min-h-[50vh]">
        <div className="text-white/30 text-sm">加载中...</div>
      </div>
    )
  }

  if (!isLoggedIn) {
    return (
      <div className="flex flex-col items-center justify-center min-h-[50vh] gap-4">
        <div className="text-white/40 text-sm">请先登录以查看持仓管理</div>
      </div>
    )
  }

  return (
    <>
      <Head>
        <title>持仓管理 — 卧龙AI量化交易台</title>
        <meta name="description" content="统一查看所有股票持仓、交易记录、资产曲线和组合分析。" />
      </Head>

      <div className="max-w-6xl mx-auto space-y-6">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h1 className="text-xl font-bold text-white/90 tracking-tight">持仓管理</h1>
            <p className="text-xs text-white/35 mt-0.5">组合控制台 — 全部持仓一览</p>
          </div>
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={() => openTradeDrawer('buy')}
              className="rounded-lg bg-primary px-3.5 py-1.5 text-xs font-medium text-white shadow-sm transition hover:bg-primary/85"
            >
              买入股票
            </button>
            <button
              type="button"
              onClick={load}
              disabled={loading}
              className="rounded-lg border border-white/15 bg-white/[0.04] px-3 py-1.5 text-xs text-white/60 hover:text-white hover:bg-white/08 disabled:opacity-40 transition flex items-center gap-1.5"
            >
              <svg className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                <path d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
              </svg>
              刷新
            </button>
          </div>
        </div>

        {error && (
          <div className="rounded-lg border border-rose-500/25 bg-rose-500/[0.06] px-4 py-3 text-sm text-rose-300">
            {error}
            <button type="button" onClick={load} className="ml-3 underline text-rose-200 hover:text-white">重试</button>
          </div>
        )}

        {pageNotice && (
          <div className="rounded-lg border border-emerald-400/25 bg-emerald-500/[0.08] px-4 py-3 text-sm text-emerald-200">
            {pageNotice}
          </div>
        )}

        <ManualMaintenanceNotice />

        <Toolbar
          scope={scope} setScope={setScope}
          sortBy={sortBy} setSortBy={setSortBy}
          sortOrder={sortOrder} setSortOrder={setSortOrder}
          pnlFilter={pnlFilter} setPnlFilter={setPnlFilter}
          keyword={keyword} setKeyword={setKeyword}
        />

        {pageViewState.initialLoading && (
          <div className="space-y-4">
            <div className="grid grid-cols-2 md:grid-cols-4 lg:grid-cols-6 gap-3">
              {Array.from({ length: 6 }).map((_, i) => (
                <div key={i} className="rounded-xl border border-white/[0.06] bg-white/[0.02] h-24 animate-pulse" />
              ))}
            </div>
            {Array.from({ length: 5 }).map((_, i) => (
              <div key={i} className="rounded-lg border border-white/[0.06] bg-white/[0.02] h-16 animate-pulse" />
            ))}
          </div>
        )}

        {data?.summary && <SummarySection summary={data.summary} />}

        {pageViewState.hasDashboardData ? (
          <PortfolioChartsSection
            curve={data?.equity_curve_preview}
            allocationItems={data?.allocation_preview}
            curveRange={curveRange}
            setCurveRange={setCurveRange}
          />
        ) : null}

        <PortfolioAttributionSection
          loading={attributionLoading}
          error={attributionError}
          range={curveRange}
          onRangeChange={setCurveRange}
          summary={attribution.summary}
          stocks={attribution.stocks}
          sectors={attribution.sectors}
          trading={attribution.trading}
          market={attribution.market}
          detailLoading={attributionDetailLoading}
          detailError={attributionDetailError}
          onRequestDetails={ensureAttributionDetails}
        />

        {riskMetrics && <RiskSection riskMetrics={riskMetrics} />}

        {data?.positions && (
          <section>
            <div className="flex items-center justify-between mb-3 gap-3">
              <h3 className="text-sm font-semibold text-white/75 flex items-center gap-2">
                <svg className="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M19 14l-7 7m0 0l-7-7m7 7V3"/></svg>
                <LabelWithInfo label="持仓明细" tooltip={FIELD_TIPS.position_detail} />
                <span className="text-[10px] font-normal text-white/30">
                  {data.positions.length} 只{scopeLabel(scope)}
                </span>
              </h3>
              <div className="text-[11px] text-white/30 hidden sm:block">每一行都支持直接买入、卖出和调均价</div>
            </div>
            <PositionTable positions={data.positions} onNavigate={handleNavigate} onAction={openTradeDrawer} />
          </section>
        )}

        {data?.recent_events_preview && (
          <RecentEventsSection events={data.recent_events_preview} />
        )}

        {pageViewState.hasDashboardData && (!data.positions || data.positions.length === 0) && (!data.recent_events_preview || data.recent_events_preview.length === 0) && (
          <EmptyPortfolio />
        )}
      </div>

      <PortfolioTradeDrawer
        open={tradeDrawer.open}
        action={tradeDrawer.action}
        currentSymbol={currentTradeSymbol}
        market={tradeDrawer.market}
        symbolInput={tradeDrawer.symbolInput}
        symbolLocked={tradeDrawer.lockedSymbol}
        item={tradeItem}
        events={tradeDrawer.events}
        canDelete={canDeleteTradeHistory}
        form={tradeDrawer.form}
        preview={tradePreview}
        loading={tradeDrawer.loading}
        saving={tradeDrawer.saving}
        error={tradeDrawer.error}
        notice={tradeDrawer.notice}
        priceAutoFilled={priceAutoFilled}
        onPriceAutoFilledChange={handlePriceAutoFilledChange}
        onClose={closeTradeDrawer}
        onActionChange={handleTradeActionChange}
        onMarketChange={handleTradeMarketChange}
        onSymbolInputChange={handleTradeSymbolInputChange}
        onFormChange={handleTradeFormChange}
        onSubmit={handleSubmitTrade}
        onUndo={handleUndoTrade}
        onOpenDetail={handleOpenTradeDetail}
        onOpenDeleteConfirm={handleOpenDeleteConfirm}
      />

      {tradeDrawer.deleteConfirmOpen && currentTradeSymbol ? (
        <div className="fixed inset-0 z-[82]">
          <button
            type="button"
            aria-label="关闭删除确认"
            onClick={handleCloseDeleteConfirm}
            className="absolute inset-0 bg-black/70"
          />
          <div className="absolute inset-x-4 top-1/2 mx-auto w-full max-w-lg -translate-y-1/2 rounded-2xl border border-rose-400/20 bg-[#12151b] p-5 shadow-[0_24px_80px_rgba(0,0,0,0.45)]">
            <div className="text-lg font-semibold text-white">确认删除整只股票的全部持仓记录</div>
            <div className="mt-3 text-sm leading-7 text-white/70">
              你将删除 <span className="font-mono text-white">{currentTradeSymbol}</span> 的全部买入、卖出、调均价与初始化历史。删除后，该股票会从当前持仓、最近交易、收益曲线和归因分析中移除，且无法恢复。
            </div>
            <div className="mt-4 rounded-xl border border-rose-400/20 bg-rose-500/[0.08] px-3 py-3 text-xs leading-6 text-rose-100/85">
              为避免误删，请输入 <span className="font-mono text-rose-100">{buildPortfolioDeleteConfirmText(currentTradeSymbol)}</span> 后再确认。
            </div>
            <input
              type="text"
              value={tradeDrawer.deleteConfirmValue}
              onChange={(event) => handleDeleteConfirmValueChange(event.target.value)}
              placeholder={buildPortfolioDeleteConfirmText(currentTradeSymbol)}
              className="mt-4 block w-full rounded-xl border border-white/10 bg-black/30 px-3 py-2.5 text-sm text-white outline-none transition focus:border-rose-300/50"
            />
            <div className="mt-5 flex flex-wrap justify-end gap-2">
              <button
                type="button"
                onClick={handleCloseDeleteConfirm}
                className="rounded-lg border border-white/12 px-4 py-2 text-xs text-white/65 transition hover:border-white/25 hover:text-white"
              >
                取消
              </button>
              <button
                type="button"
                disabled={tradeDrawer.saving}
                onClick={handleDeleteTradeHistory}
                className="rounded-lg border border-rose-300/30 bg-rose-500/15 px-4 py-2 text-xs font-medium text-rose-100 transition hover:bg-rose-500/22 disabled:cursor-not-allowed disabled:opacity-60"
              >
                {tradeDrawer.saving ? '删除中...' : '确认永久删除'}
              </button>
            </div>
          </div>
        </div>
      ) : null}

      <style jsx>{`
        .custom-scrollbar::-webkit-scrollbar {
          width: 4px;
        }
        .custom-scrollbar::-webkit-scrollbar-track {
          background: transparent;
        }
        .custom-scrollbar::-webkit-scrollbar-thumb {
          background: rgba(255,255,255,0.12);
          border-radius: 999px;
        }
      `}</style>
    </>
  )
}
