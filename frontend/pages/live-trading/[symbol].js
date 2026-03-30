import Head from 'next/head'
import { useRouter } from 'next/router'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import { requestJson } from '../../lib/api'
import { useAuth } from '../../lib/auth-context'
import { isAuthRequiredError } from '../../lib/auth-storage'

const POLL_MS = 2000
const OVERLAY_WINDOW_MINUTES = 60
const SUPPORT_REFRESH_MS = 60 * 1000
const SIGNAL_CENTER_REFRESH_MS = 15 * 1000
const FUNDAMENTALS_REFRESH_MS = 24 * 60 * 60 * 1000
const SUPPORT_LOOKBACK_DAYS = 120
const MA_LOOKBACK_DAYS = 240
const SIGNAL_MAX_ATTEMPTS = 4
const SIGNAL_BACKOFF_STEPS = ['1 分钟', '5 分钟', '15 分钟']

export default function LiveTradingDetailPage() {
  const router = useRouter()
  const { symbol: rawSymbol } = router.query
  const symbol = rawSymbol ? decodeURIComponent(rawSymbol).toUpperCase() : ''

  const { isLoggedIn, openAuthModal, ready, user } = useAuth()

  const [dailyBars, setDailyBars] = useState([])
  const [allDailyBars, setAllDailyBars] = useState([])
  const [dailyRange, setDailyRange] = useState('6M')
  const [dailyLoading, setDailyLoading] = useState(false)
  const [snapshotPayload, setSnapshotPayload] = useState(null)
  const [fundamentalsPayload, setFundamentalsPayload] = useState(null)
  const [fundamentalsLoading, setFundamentalsLoading] = useState(false)
  const [fundamentalsError, setFundamentalsError] = useState('')
  const [overlayPayload, setOverlayPayload] = useState(null)
  const [supportPayload, setSupportPayload] = useState(null)
  const [supportError, setSupportError] = useState('')
  const [resistancePayload, setResistancePayload] = useState(null)
  const [resistanceError, setResistanceError] = useState('')
  const [movingAveragePayload, setMovingAveragePayload] = useState(null)
  const [movingAverageError, setMovingAverageError] = useState('')
  const [priceVolumeEvents, setPriceVolumeEvents] = useState([])
  const [blockFlowEvents, setBlockFlowEvents] = useState([])
  const [activeStrategies, setActiveStrategies] = useState([])
  const [signalConfig, setSignalConfig] = useState(null)
  const [webhookConfig, setWebhookConfig] = useState({ url: '', has_secret: false, is_enabled: true, timeout_ms: 3000, updated_at: '' })
  const [savingSignal, setSavingSignal] = useState(false)
  const [signalNotice, setSignalNotice] = useState('')
  const [signalError, setSignalError] = useState('')
  const [error, setError] = useState('')
  const [errorNeedsLogin, setErrorNeedsLogin] = useState(false)
  const [lastUpdateAt, setLastUpdateAt] = useState('')

  const supportRefreshRef = useRef({ symbol: '', refreshedAt: 0 })
  const resistanceRefreshRef = useRef({ symbol: '', refreshedAt: 0 })
  const movingAverageRefreshRef = useRef({ symbol: '', refreshedAt: 0 })
  const fundamentalsRefreshRef = useRef({ symbol: '', refreshedAt: 0 })
  const signalCenterRefreshRef = useRef(0)

  const privateAccessReady = ready && isLoggedIn
  const authIdentityKey = String(user?.id || user?.email || '')
  const exchange = detectExchange(symbol)
  const isAShare = exchange === 'SSE' || exchange === 'SZSE'

  const snapshot = snapshotPayload?.snapshot || null
  const symbolName = useMemo(() => {
    if (snapshot?.name && snapshot.name !== symbol) return snapshot.name
    return ''
  }, [snapshot, symbol])

  const pageTitle = symbolName ? `${symbolName}（${symbol}）- 行情看板` : symbol ? `${symbol} - 行情看板` : '行情看板'

  const updateError = (nextError, nextNeedsLogin = false) => {
    setError(nextError)
    setErrorNeedsLogin(nextNeedsLogin)
  }

  const applyRequestError = (err, fallbackText) => {
    updateError(err.message || fallbackText, isAuthRequiredError(err))
  }

  // ── Data loaders ──

  const loadSymbolPanels = useCallback(async (sym, { forceSupport = false } = {}) => {
    if (!sym) return
    const encoded = encodeURIComponent(sym)
    const [snapshotData, overlayData, pvData, blockData] = await Promise.all([
      requestJson(`/api/live/symbols/${encoded}/snapshot`),
      requestJson(`/api/live/symbols/${encoded}/overlay?window_minutes=${OVERLAY_WINDOW_MINUTES}`),
      requestJson(`/api/live/symbols/${encoded}/anomalies/price-volume?limit=20`),
      requestJson(`/api/live/symbols/${encoded}/anomalies/block-flow?limit=20`),
    ])

    setSnapshotPayload(snapshotData)
    setOverlayPayload(overlayData)
    setPriceVolumeEvents(pvData.items || [])
    setBlockFlowEvents(blockData.items || [])
    setLastUpdateAt(new Date().toISOString())

    // Load support/resistance/MA in parallel
    await Promise.all([
      loadSupportLevels(sym, { force: forceSupport }),
      loadResistanceLevels(sym, { force: forceSupport }),
      loadMovingAverages(sym, { force: forceSupport }),
    ])
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const loadSupportLevels = async (sym, { force = false } = {}) => {
    if (!sym) return
    const now = Date.now()
    const cache = supportRefreshRef.current
    if (!force && cache.symbol === sym && now - cache.refreshedAt < SUPPORT_REFRESH_MS) return
    try {
      const data = await requestJson(`/api/live/symbols/${encodeURIComponent(sym)}/support-levels?period=daily&lookback_days=${SUPPORT_LOOKBACK_DAYS}`)
      setSupportPayload(data)
      setSupportError('')
      supportRefreshRef.current = { symbol: sym, refreshedAt: now }
    } catch (err) {
      setSupportError(err.message || '支撑位数据暂不可用')
      if (force) setSupportPayload(null)
    }
  }

  const loadResistanceLevels = async (sym, { force = false } = {}) => {
    if (!sym) return
    const now = Date.now()
    const cache = resistanceRefreshRef.current
    if (!force && cache.symbol === sym && now - cache.refreshedAt < SUPPORT_REFRESH_MS) return
    try {
      const data = await requestJson(`/api/live/symbols/${encodeURIComponent(sym)}/resistance-levels?period=daily&lookback_days=${SUPPORT_LOOKBACK_DAYS}`)
      setResistancePayload(data)
      setResistanceError('')
      resistanceRefreshRef.current = { symbol: sym, refreshedAt: now }
    } catch (err) {
      setResistanceError(err.message || '压力位数据暂不可用')
      if (force) setResistancePayload(null)
    }
  }

  const loadMovingAverages = async (sym, { force = false } = {}) => {
    if (!sym) return
    const now = Date.now()
    const cache = movingAverageRefreshRef.current
    if (!force && cache.symbol === sym && now - cache.refreshedAt < SUPPORT_REFRESH_MS) return
    try {
      const data = await requestJson(`/api/live/symbols/${encodeURIComponent(sym)}/moving-averages?period=daily&lookback_days=${MA_LOOKBACK_DAYS}`)
      setMovingAveragePayload(data)
      setMovingAverageError('')
      movingAverageRefreshRef.current = { symbol: sym, refreshedAt: now }
    } catch (err) {
      setMovingAverageError(err.message || '均线指标暂不可用')
      if (force) setMovingAveragePayload(null)
    }
  }

  const loadFundamentals = async (sym, { force = false } = {}) => {
    if (!sym) return
    const now = Date.now()
    const cache = fundamentalsRefreshRef.current
    if (!force && cache.symbol === sym && now - cache.refreshedAt < FUNDAMENTALS_REFRESH_MS) return
    setFundamentalsLoading(true)
    try {
      const data = await requestJson(`/api/live/symbols/${encodeURIComponent(sym)}/fundamentals`)
      setFundamentalsPayload(data)
      setFundamentalsError('')
      fundamentalsRefreshRef.current = { symbol: sym, refreshedAt: now }
    } catch (err) {
      setFundamentalsError(err.message || '基础面数据暂不可用')
      if (force || cache.symbol !== sym) setFundamentalsPayload(null)
    } finally {
      setFundamentalsLoading(false)
    }
  }

  // ── Daily bars (history chart) ──

  const DAILY_RANGE_MAP = {
    '1D': 2, '1W': 7, '1M': 25, '3M': 65, '6M': 130,
    '1Y': 260, '2Y': 520, '5Y': 1300, '10Y': 1500, ALL: 1500,
  }
  const DAILY_RANGE_LABELS = ['1D','1W','1M','3M','6M','1Y','2Y','5Y','10Y','ALL']

  const loadDailyBars = useCallback(async (sym, range) => {
    if (!sym) return
    const lookback = DAILY_RANGE_MAP[range] || 130
    setDailyLoading(true)
    try {
      const data = await requestJson(`/api/live/symbols/${encodeURIComponent(sym)}/daily-bars?lookback_days=${lookback}`)
      setDailyBars(Array.isArray(data?.bars) ? data.bars : [])
    } catch (_) {
      setDailyBars([])
    } finally {
      setDailyLoading(false)
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const loadAllDailyBars = useCallback(async (sym) => {
    if (!sym) return
    try {
      const data = await requestJson(`/api/live/symbols/${encodeURIComponent(sym)}/daily-bars?lookback_days=1500`)
      setAllDailyBars(Array.isArray(data?.bars) ? data.bars : [])
    } catch (_) {
      setAllDailyBars([])
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const rangeReturns = useMemo(() => {
    if (!allDailyBars || allDailyBars.length < 2) return {}
    const last = allDailyBars[allDailyBars.length - 1]?.close
    if (!last || last <= 0) return {}
    const result = {}
    for (const key of DAILY_RANGE_LABELS) {
      const lookback = DAILY_RANGE_MAP[key] || allDailyBars.length
      const startIdx = Math.max(0, allDailyBars.length - lookback)
      const first = allDailyBars[startIdx]?.close
      if (first && first > 0) {
        result[key] = (last - first) / first
      }
    }
    return result
  }, [allDailyBars])


  // Load daily bars on mount and range change
  useEffect(() => {
    if (!ready || !symbol) return
    loadDailyBars(symbol, dailyRange)
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ready, symbol, dailyRange])

  // Load all daily bars once for range return calculation
  useEffect(() => {
    if (!ready || !symbol) return
    loadAllDailyBars(symbol)
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ready, symbol])

  const loadSignalCenter = async ({ force = false } = {}) => {
    const now = Date.now()
    if (!force && now - signalCenterRefreshRef.current < SIGNAL_CENTER_REFRESH_MS) return
    const [strategiesData, configsData, webhookData] = await Promise.all([
      requestJson('/api/strategies/active'),
      requestJson('/api/signal-configs'),
      requestJson('/api/webhook'),
    ])
    const strategies = Array.isArray(strategiesData?.items) ? strategiesData.items : []
    setActiveStrategies(strategies)

    const configs = Array.isArray(configsData?.items) ? configsData.items : []
    const matched = configs.find((c) => c?.symbol === symbol)
    setSignalConfig(matched || {
      symbol,
      strategy_id: strategies[0]?.id || '',
      is_enabled: false,
      cooldown_seconds: 3600,
      thresholds: {},
    })

    const wh = webhookData?.item || null
    if (wh) {
      setWebhookConfig({
        url: wh.url || '',
        has_secret: Boolean(wh.has_secret),
        is_enabled: wh.is_enabled !== false,
        timeout_ms: Number(wh.timeout_ms) > 0 ? Number(wh.timeout_ms) : 3000,
        updated_at: wh.updated_at || '',
      })
    }
    signalCenterRefreshRef.current = now
  }

  // ── Bootstrap & polling ──

  useEffect(() => {
    if (!ready || !symbol) return
    const bootstrap = async () => {
      try {
        await Promise.all([
          loadSymbolPanels(symbol, { forceSupport: true }),
          loadFundamentals(symbol),
        ])
        updateError('')
      } catch (err) {
        applyRequestError(err, '加载数据失败')
      }
      if (privateAccessReady) {
        try {
          await loadSignalCenter({ force: true })
        } catch (err) {
          setSignalError(err.message || '信号配置加载失败')
        }
      }
    }
    bootstrap()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ready, symbol, privateAccessReady, authIdentityKey])

  useEffect(() => {
    if (!ready || !symbol) return
    const timer = setInterval(async () => {
      try {
        await loadSymbolPanels(symbol)
        updateError('')
      } catch (err) {
        applyRequestError(err, '数据刷新失败')
      }
      if (privateAccessReady) {
        try { await loadSignalCenter() } catch (_) {}
      }
    }, POLL_MS)
    return () => clearInterval(timer)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ready, symbol, privateAccessReady, authIdentityKey])

  // ── Signal config handlers ──

  const updateLocalSignalConfig = (patch) => {
    setSignalConfig((prev) => ({
      ...(prev || { symbol, strategy_id: '', is_enabled: false, cooldown_seconds: 3600, thresholds: {} }),
      ...patch,
    }))
  }

  const handleSaveSignalConfig = async () => {
    if (!signalConfig) return
    setSavingSignal(true)
    setSignalNotice('')
    setSignalError('')
    try {
      const result = await requestJson(`/api/signal-configs/${encodeURIComponent(symbol)}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          strategy_id: signalConfig.strategy_id,
          is_enabled: Boolean(signalConfig.is_enabled),
          cooldown_seconds: Number(signalConfig.cooldown_seconds) || 3600,
          thresholds: signalConfig.thresholds || {},
        }),
      })
      if (result?.item) setSignalConfig(result.item)
      setSignalNotice('信号配置已保存')
    } catch (err) {
      setSignalError(err.message || '信号配置保存失败')
    } finally {
      setSavingSignal(false)
    }
  }

  const strategyByID = useMemo(() => {
    const map = {}
    activeStrategies.forEach((s) => { if (s?.id) map[s.id] = s })
    return map
  }, [activeStrategies])

  const selectedStrategy = signalConfig ? strategyByID[signalConfig.strategy_id] || null : null
  const webhookConfigured = Boolean(webhookConfig.url)
  const fundamentalsItems = fundamentalsPayload?.items || {}
  const fundamentalsMeta = fundamentalsPayload?.meta || null
  const fundamentalsReportLabel = buildFundamentalsReportLabel(fundamentalsMeta)
  const fundamentalsCurrencyCode = isAShare ? 'CNY' : 'HKD'
  const fundamentalsCurrencySymbol = isAShare ? '¥' : 'HK$'
  const fundamentalsMetaLine = buildFundamentalsMetaLine(fundamentalsMeta)
  const fundamentalsSupported = fundamentalsMeta?.supported !== false
  const supportSummary = supportPayload?.summary || null
  const supportLevels = Array.isArray(supportPayload?.levels) ? supportPayload.levels : []
  const resistanceSummary = resistancePayload?.summary || null
  const resistanceLevels = Array.isArray(resistancePayload?.levels) ? resistancePayload.levels : []
  const supportStatusAccent = supportSummary?.status === '跌破支撑' ? 'down' : supportSummary?.status === '临近支撑' || supportSummary?.status === '回踩支撑' ? 'up' : 'normal'
  const resistanceStatusAccent = resistanceSummary?.status === '突破压力' ? 'up' : resistanceSummary?.status === '临近压力' || resistanceSummary?.status === '回踩压力' ? 'down' : 'normal'
  const movingAverageStatusAccent = movingAveragePayload?.status === '双双站上' ? 'up' : movingAveragePayload?.status === '双双跌破' ? 'down' : 'normal'

  if (!symbol) {
    return (
      <div className="flex min-h-[40vh] items-center justify-center text-white/50">
        加载中...
      </div>
    )
  }

  return (
    <>
      <Head>
        <title>{pageTitle}</title>
      </Head>

      <div className="space-y-6">
        {/* Header */}
        <section className="rounded-2xl border border-border bg-card p-6">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <h1 className="text-2xl font-semibold tracking-tight text-white">
                {symbolName ? `${symbolName}（${symbol}）` : symbol}
              </h1>
              <div className="mt-1 flex items-center gap-3 text-xs text-white/55">
                <span>{detectExchangeLabel(symbol)}</span>
                {lastUpdateAt && <span>更新：{formatDateTime(lastUpdateAt)}</span>}
                <span>行情来源：{formatSource(snapshot?.source)}</span>
              </div>
            </div>
            <a
              href="/live-trading"
              className="inline-flex items-center gap-1 rounded-xl border border-border px-3 py-2 text-xs text-white/75 transition hover:border-primary hover:text-primary"
            >
              ← 返回概览
            </a>
          </div>
        </section>

        {error ? (
          <div className="rounded-xl border border-rose-400/40 bg-rose-500/10 px-4 py-3 text-sm text-rose-200">
            <div>{error}</div>
            {errorNeedsLogin ? (
              <button
                type="button"
                onClick={() => openAuthModal('login', '行情看板相关操作需要登录后才能继续。')}
                className="mt-2 inline-flex rounded-lg border border-rose-300/40 px-2.5 py-1 text-xs text-rose-100 transition hover:bg-rose-500/15"
              >
                去登录
              </button>
            ) : null}
          </div>
        ) : null}

        {/* Snapshot */}
        <section className="rounded-2xl border border-border bg-card p-5">
          <h3 className="text-base font-semibold text-white">实时快照</h3>
          {!snapshot ? (
            <div className="mt-3 rounded-xl border border-dashed border-border px-4 py-6 text-sm text-white/50">数据加载中...</div>
          ) : (
            <div className="mt-4 grid gap-3 md:grid-cols-4">
              <MetricMini
                label="最新价"
                value={formatNumber(snapshot.last_price, 3)}
                accent={snapshot.change_rate > 0 ? 'up' : snapshot.change_rate < 0 ? 'down' : 'normal'}
                featured
                marketAccent
              />
              <MetricMini label="涨跌幅" value={formatPercent(snapshot.change_rate)} accent={snapshot.change_rate >= 0 ? 'up' : 'down'} tooltip="今日价格相比昨日收盘价的变化百分比。红色表示上涨，绿色表示下跌。" />
              <MetricMini label="量比" value={formatNumber(snapshot.volume_ratio, 2)} tooltip="当前成交量与过去 5 日同时段平均成交量的比值。大于 1 说明今天比平时活跃，越大越异常。" />
              <MetricMini label="成交量" value={formatCompact(snapshot.volume)} tooltip="今日到目前为止的总成交股数（或手数），反映市场参与的活跃程度。" />
              <MetricMini label={`成交额(${isAShare ? 'CNY' : 'HKD'})`} value={formatCompact(snapshot.turnover)} tooltip="今日到目前为止的总成交金额，比成交量更能反映真实的资金参与规模。" />
              <MetricMini label="振幅" value={formatPercent(snapshot.amplitude)} tooltip="今日最高价与最低价之间的波动幅度占昨收价的百分比。振幅越大，说明今天价格波动越剧烈。" />
              <MetricMini label="换手率" value={formatTurnoverRate(snapshot.turnover_rate)} tooltip="今日成交股数占流通股总数的百分比。换手率越高，说明交易越活跃、筹码流动越快。" />
            </div>
          )}
        </section>

        {/* Fundamentals */}
        <section className="rounded-2xl border border-border bg-card p-5">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <h3 className="text-base font-semibold text-white">基础面概览</h3>
              {fundamentalsMetaLine ? (
                <p className="mt-1 text-xs text-white/55">{fundamentalsMetaLine}</p>
              ) : null}
            </div>
            {!fundamentalsSupported && fundamentalsMeta?.warning ? (
              <div className="rounded-full border border-amber-300/25 bg-amber-500/10 px-3 py-1 text-[11px] text-amber-200">
                {fundamentalsMeta.warning}
              </div>
            ) : null}
          </div>
          {fundamentalsError ? (
            <div className="mt-3 rounded-xl border border-amber-400/30 bg-amber-500/10 px-4 py-3 text-sm text-amber-200">{fundamentalsError}</div>
          ) : null}
          {!fundamentalsPayload && fundamentalsLoading ? (
            <div className="mt-3 rounded-xl border border-dashed border-border px-4 py-6 text-sm text-white/50">基础面数据加载中...</div>
          ) : (
            <div className="mt-4 grid gap-3 md:grid-cols-5">
              <MetricMini label={`市值(${fundamentalsCurrencyCode})`} value={formatYiCurrency(fundamentalsItems.market_cap, fundamentalsCurrencySymbol)} emphasis tooltip="公司所有流通股按当前价格计算的总价值。市值越大，公司规模越大。" />
              <MetricMini label="股息收益率" value={formatPercentMaybeNull(fundamentalsItems.dividend_yield)} tooltip="过去一年的每股分红金额占当前股价的比例。收益率越高，分红回报越好。" />
              <MetricMini label="市盈率(TTM)" value={formatMultiple(fundamentalsItems.pe_ttm)} tooltip="当前股价是过去 12 个月每股利润的多少倍。市盈率越低，可能越「便宜」；越高可能表示市场对未来增长的期望越大。" />
              <MetricMini label="市净率(PB)" value={formatMultiple(fundamentalsItems.pb_ttm)} tooltip="当前股价是每股净资产的多少倍。PB 越低，说明股价相对账面价值越「便宜」；PB < 1 意味着股价低于净资产（可能被低估，也可能反映经营困难）。" />
              <MetricMini label="PEG" value={formatPEG(fundamentalsItems.peg)} accent={pegAccent(fundamentalsItems.peg)} tooltip="市盈率与盈利增长率的比值（PEG = PE / 净利润增长率%）。PEG < 1 通常被认为低估（成长性好且估值合理），PEG > 2 可能偏贵。增长率为负时不可计算。" />
              <MetricMini label="流通股" value={formatYiShares(fundamentalsItems.float_shares)} tooltip="目前在市场上可以自由买卖的股票总数。流通股越少，股价越容易被大资金影响。" />
              <MetricMini label={`净利润(${fundamentalsReportLabel} · ${fundamentalsCurrencyCode})`} value={formatYiAmount(fundamentalsItems.net_profit_fy, fundamentalsCurrencySymbol)} tooltip="公司在报告期内扣除所有成本和税费后的最终利润。这是衡量公司赚钱能力的核心指标。" />
              <MetricMini label={`收入(${fundamentalsReportLabel} · ${fundamentalsCurrencyCode})`} value={formatYiAmount(fundamentalsItems.revenue_fy, fundamentalsCurrencySymbol)} tooltip="公司在报告期内的总营业收入（卖出产品或服务获得的钱）。收入增长通常意味着公司业务在扩张。" />
              <MetricMini label="毛利率" value={formatPercentDirect(fundamentalsItems.gross_margin)} tooltip="收入减去直接成本后剩余的利润占收入的比例。毛利率越高，说明产品或服务的附加值越大。" />
              <MetricMini label="净利率" value={formatPercentDirect(fundamentalsItems.net_margin)} tooltip="净利润占总收入的比例。净利率越高，说明公司每赚 1 块钱中能留下的利润越多。" />
            </div>
          )}
        </section>

        {/* Daily history chart */}
        <section className="rounded-2xl border border-border bg-card p-5">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <h3 className="text-base font-semibold text-white">历史走势</h3>
          </div>
          <div className="mt-3 flex flex-wrap gap-1.5">
            {DAILY_RANGE_LABELS.map((key) => {
              const ret = rangeReturns[key]
              const hasReturn = ret !== undefined && ret !== null
              const isUp = hasReturn && ret >= 0
              const retColor = hasReturn ? (isUp ? 'text-rose-300' : 'text-emerald-300') : 'text-white/30'
              return (
                <button
                  key={key}
                  type="button"
                  onClick={() => setDailyRange(key)}
                  className={`flex flex-col items-center rounded-lg px-2.5 py-1.5 text-xs font-medium transition min-w-[48px] ${
                    dailyRange === key
                      ? 'bg-primary text-white shadow-sm'
                      : 'bg-black/25 text-white/65 hover:bg-black/35 hover:text-white/85'
                  }`}
                >
                  <span>{key === 'ALL' ? '全部' : key.replace('D','天').replace('W','周').replace('M','月').replace('Y','年')}</span>
                  <span className={`mt-0.5 text-[10px] leading-tight font-semibold ${dailyRange === key ? (isUp ? 'text-white/90' : 'text-white/90') : retColor}`}>
                    {hasReturn ? `${isUp ? '+' : ''}${(ret * 100).toFixed(1)}%` : '--'}
                  </span>
                </button>
              )
            })}
          </div>
          {dailyLoading ? (
            <div className="mt-4 flex items-center justify-center rounded-xl border border-dashed border-border py-16 text-sm text-white/50">加载中...</div>
          ) : dailyBars.length === 0 ? (
            <div className="mt-4 rounded-xl border border-dashed border-border px-4 py-6 text-sm text-white/50">暂无历史数据。</div>
          ) : (
            <DailyHistoryChart bars={dailyBars} />
          )}
        </section>

        {/* Moving averages */}
        <section className="rounded-2xl border border-border bg-card p-5">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <h3 className="text-base font-semibold text-white">技术指标</h3>
              <p className="mt-1 text-xs text-white/60">均线 / RSI / MACD / 布林带，基于最近 {MA_LOOKBACK_DAYS} 个交易日收盘价。</p>
            </div>
            <div className="text-xs text-white/55">
              {movingAveragePayload?.updated_at ? `更新：${formatDateTime(movingAveragePayload.updated_at)}` : '等待数据'}
            </div>
          </div>
          {movingAverageError && <div className="mt-3 rounded-xl border border-amber-400/30 bg-amber-500/10 px-4 py-3 text-sm text-amber-200">{movingAverageError}</div>}
          {!movingAveragePayload ? (
            <div className="mt-3 rounded-xl border border-dashed border-border px-4 py-6 text-sm text-white/50">暂无均线数据。</div>
          ) : (
            <div className="mt-4 space-y-3">
              <div className="grid gap-3 md:grid-cols-4">
                <MetricMini label="MA5" value={formatNumber(movingAveragePayload.ma5, 3)} accent={maAccent(movingAveragePayload.price_ref, movingAveragePayload.ma5)} tooltip="最近 5 个交易日收盘价的平均值，反映超短线趋势。价格在 MA5 上方通常偏强。" />
                <MetricMini label="MA20" value={formatNumber(movingAveragePayload.ma20, 3)} accent={maAccent(movingAveragePayload.price_ref, movingAveragePayload.ma20)} tooltip="最近 20 个交易日（约 1 个月）收盘价均值，常用来判断短线趋势方向。" />
                <MetricMini label="MA60" value={formatNumber(movingAveragePayload.ma60, 3)} accent={maAccent(movingAveragePayload.price_ref, movingAveragePayload.ma60)} tooltip="最近 60 个交易日（约 3 个月）收盘价均值，反映中线趋势。常被称为「生命线」。" />
                <MetricMini label="MA200" value={formatNumber(movingAveragePayload.ma200, 3)} accent={maAccent(movingAveragePayload.price_ref, movingAveragePayload.ma200)} tooltip="最近 200 个交易日（约 1 年）收盘价均值，反映长线趋势。价格站上 MA200 通常被视为牛市信号。" />
              </div>
              <div className="grid gap-3 md:grid-cols-5">
                <MetricMini label="距 MA5" value={formatDistancePct(movingAveragePayload.distance_to_ma5_pct)} accent={movingAveragePayload.distance_to_ma5_pct >= 0 ? 'up' : 'down'} tooltip="当前价格偏离 MA5 的程度。正值说明价格在均线上方，负值说明在下方。偏离越大，回归的可能性越高。" />
                <MetricMini label="距 MA20" value={formatDistancePct(movingAveragePayload.distance_to_ma20_pct)} accent={movingAveragePayload.distance_to_ma20_pct >= 0 ? 'up' : 'down'} tooltip="当前价格偏离 MA20 的程度。偏离过大可能意味着短期涨跌过快，有修正的可能。" />
                <MetricMini label="距 MA60" value={formatDistancePct(movingAveragePayload.distance_to_ma60_pct)} accent={movingAveragePayload.distance_to_ma60_pct >= 0 ? 'up' : 'down'} tooltip="当前价格偏离 MA60 的程度。偏离 MA60 过远通常意味着中线超涨或超跌。" />
                <MetricMini label="距 MA200" value={formatDistancePct(movingAveragePayload.distance_to_ma200_pct)} accent={movingAveragePayload.distance_to_ma200_pct >= 0 ? 'up' : 'down'} tooltip="当前价格偏离 MA200 的程度。正值越大说明长线涨幅越多，负值越大说明长线偏弱。" />
                <MetricMini label="位置状态" value={formatMAStatus(movingAveragePayload.status)} accent={movingAverageStatusAccent} emphasis tooltip="当前价格相对 MA20 和 MA200 的位置组合。「双双站上」意味着短线和长线都偏强，「双双跌破」则都偏弱。" />
              </div>
              <div className="grid gap-3 md:grid-cols-2">
                <MetricMini label="RSI(14)" value={formatNumber(movingAveragePayload.rsi14, 2)} accent={rsiAccent(movingAveragePayload.rsi14)} tooltip="衡量最近 14 个交易日涨跌动能的指标，范围 0~100。≥70 为超买（可能回调），≤30 为超卖（可能反弹），50 附近为中性。" />
                <MetricMini label="RSI 状态" value={movingAveragePayload.rsi14_status || '--'} accent={rsiAccent(movingAveragePayload.rsi14)} emphasis tooltip="基于 RSI 数值的市场情绪判断。超买意味着短期涨太多可能要歇一歇，超卖意味着跌太多可能要反弹。" />
              </div>
              <div className="grid gap-3 md:grid-cols-3">
                <MetricMini label="MACD" value={formatNumber(movingAveragePayload.macd, 4)} accent={movingAveragePayload.macd >= 0 ? 'up' : 'down'} tooltip="快线（12日EMA）减慢线（26日EMA）的差值。MACD 为正说明短期趋势强于长期，为负则相反。MACD 从负转正叫「金叉」，是买入信号。" />
                <MetricMini label="信号线" value={formatNumber(movingAveragePayload.macd_signal, 4)} tooltip="MACD 线的 9 日平均值，用来平滑 MACD 的波动。当 MACD 线向上穿过信号线时为金叉（看涨），向下穿过时为死叉（看跌）。" />
                <MetricMini label="MACD 柱" value={formatNumber(movingAveragePayload.macd_histogram, 4)} accent={movingAveragePayload.macd_histogram >= 0 ? 'up' : 'down'} emphasis tooltip="MACD 线与信号线的差值。红柱（正值）表示多头动能在增强，绿柱（负值）表示空头动能在增强。柱子由短变长说明趋势在加速。" />
              </div>
              {movingAveragePayload.macd_series?.length > 0 && (
                <MACDChart series={movingAveragePayload.macd_series} />
              )}
              <div className="grid gap-3 md:grid-cols-4">
                <MetricMini label="布林上轨" value={formatNumber(movingAveragePayload.bollinger_upper, 3)} tooltip="布林带上轨 = MA20 + 2倍标准差。价格触及或突破上轨通常意味着短期涨幅较大，可能面临回调压力。" />
                <MetricMini label="布林下轨" value={formatNumber(movingAveragePayload.bollinger_lower, 3)} tooltip="布林带下轨 = MA20 - 2倍标准差。价格触及或跌破下轨通常意味着短期跌幅较大，可能有反弹机会。" />
                <MetricMini label="带宽" value={formatBollingerBW(movingAveragePayload.bollinger_bandwidth)} tooltip="上轨与下轨之间的宽度占中轨的百分比。带宽收窄说明波动率降低，往往预示即将出现大的方向性突破。" />
                <MetricMini label="%B 位置" value={formatPercentB(movingAveragePayload.bollinger_percent_b)} accent={percentBAccent(movingAveragePayload.bollinger_percent_b)} emphasis tooltip="价格在布林带中的相对位置。%B > 1 表示价格在上轨之上（超买区），%B < 0 表示在下轨之下（超卖区），0.5 表示正好在中轨。" />
              </div>
              {movingAveragePayload.bollinger_series?.length > 0 && (
                <BollingerChart series={movingAveragePayload.bollinger_series} />
              )}
            </div>
          )}
        </section>

        {/* Overlay intraday chart */}
        <section className="rounded-2xl border border-border bg-card p-5">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <h3 className="text-base font-semibold text-white">实时分时叠加（个股 vs 大盘）</h3>
            <div className="text-xs text-white/60">窗口：{OVERLAY_WINDOW_MINUTES} 分钟</div>
          </div>
          {!overlayPayload?.series?.length ? (
            <div className="mt-3 rounded-xl border border-dashed border-border px-4 py-6 text-sm text-white/50">分时数据预热中，请稍后。</div>
          ) : (
            <div className="mt-4 space-y-4">
              <OverlayIntradayChart series={overlayPayload.series} benchmark={overlayPayload.benchmark} symbol={overlayPayload.symbol} />
              <div className="grid gap-3 md:grid-cols-4">
                <MetricMini label="基准指数" value={overlayPayload.benchmark || 'HSI'} />
                <MetricMini label="Beta" value={formatNumberMaybeNull(overlayPayload?.metrics?.beta, 3)} accent={overlayPayload?.metrics?.beta != null && overlayPayload.metrics.beta >= 1 ? 'up' : 'normal'} tooltip="衡量该股票相对大盘的波动程度。Beta=1 表示与大盘同步；>1 表示波动比大盘大（更激进）；<1 表示波动比大盘小（更稳健）。" />
                <MetricMini label="Relative Strength" value={formatPercentMaybeNull(overlayPayload?.metrics?.relative_strength)} accent={overlayPayload?.metrics?.relative_strength != null && overlayPayload.metrics.relative_strength >= 0 ? 'up' : 'down'} tooltip="该股票在同时段内相对大盘的强弱表现。正值说明跑赢大盘，负值说明跑输大盘。" />
                <MetricMini label="样本状态" value={`${overlayPayload?.metrics?.sample_count || 0}/${overlayPayload?.metrics?.warmup_min_samples || 30} · ${overlayPayload?.metrics?.is_warmup ? '预热中' : '可用'}`} accent={overlayPayload?.metrics?.is_warmup ? 'normal' : 'up'} />
              </div>
            </div>
          )}
        </section>

        {/* Support levels */}
        <section className="rounded-2xl border border-border bg-card p-5">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <h3 className="text-base font-semibold text-white">支撑位（近{SUPPORT_LOOKBACK_DAYS}天）</h3>
            <div className="text-xs text-white/55">{supportPayload?.meta?.updated_at ? `更新：${formatDateTime(supportPayload.meta.updated_at)}` : '等待数据'}</div>
          </div>
          {supportError && <div className="mt-3 rounded-xl border border-amber-400/30 bg-amber-500/10 px-4 py-3 text-sm text-amber-200">{supportError}</div>}
          {!supportSummary ? (
            <div className="mt-3 rounded-xl border border-dashed border-border px-4 py-6 text-sm text-white/50">暂无支撑位数据。</div>
          ) : (
            <div className="mt-4 space-y-4">
              <div className="grid gap-3 md:grid-cols-4">
                <MetricMini label="最近支撑位" value={supportSummary.nearest_price ? formatNumber(supportSummary.nearest_price, 3) : '--'} accent={supportStatusAccent} emphasis tooltip="距离当前价格最近的一个支撑价位。历史上价格多次在这个位置附近止跌回升。" />
                <MetricMini label="距最近支撑位" value={formatDistancePct(supportSummary.distance_pct)} accent={supportSummary.distance_pct >= 0 ? 'normal' : 'down'} tooltip="当前价格与最近支撑位之间的距离百分比。负值说明已经跌破支撑。" />
                <MetricMini label="支撑强度" value={supportSummary.strength || '--'} accent={supportSummary.strength === '强' ? 'up' : supportSummary.strength === '弱' ? 'down' : 'normal'} tooltip="该支撑位的可靠程度。强度越高，价格在此位置止跌的可能性越大。" />
                <MetricMini label="支撑状态" value={formatSupportStatus(supportSummary.status)} accent={supportStatusAccent} emphasis tooltip="当前价格与支撑位的关系。例如「接近支撑区」说明价格快要触碰支撑，「跌破支撑区」说明支撑已失效。" />
              </div>
              <div className="space-y-2">
                {supportLevels.map((level, i) => (
                  <LevelCard key={level.level} level={level} index={i} type="support" />
                ))}
              </div>
            </div>
          )}
        </section>

        {/* Resistance levels */}
        <section className="rounded-2xl border border-border bg-card p-5">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <h3 className="text-base font-semibold text-white">压力位（近{SUPPORT_LOOKBACK_DAYS}天）</h3>
            <div className="text-xs text-white/55">{resistancePayload?.meta?.updated_at ? `更新：${formatDateTime(resistancePayload.meta.updated_at)}` : '等待数据'}</div>
          </div>
          {resistanceError && <div className="mt-3 rounded-xl border border-amber-400/30 bg-amber-500/10 px-4 py-3 text-sm text-amber-200">{resistanceError}</div>}
          {!resistanceSummary ? (
            <div className="mt-3 rounded-xl border border-dashed border-border px-4 py-6 text-sm text-white/50">暂无压力位数据。</div>
          ) : (
            <div className="mt-4 space-y-4">
              <div className="grid gap-3 md:grid-cols-4">
                <MetricMini label="最近压力位" value={resistanceSummary.nearest_price ? formatNumber(resistanceSummary.nearest_price, 3) : '--'} accent={resistanceStatusAccent} emphasis tooltip="距离当前价格最近的一个压力价位。历史上价格多次在这个位置附近遇阻回落。" />
                <MetricMini label="距最近压力位" value={formatDistancePct(resistanceSummary.distance_pct)} accent={resistanceSummary.distance_pct >= 0 ? 'normal' : 'up'} tooltip="当前价格与最近压力位之间的距离百分比。负值说明已经突破压力。" />
                <MetricMini label="压力强度" value={resistanceSummary.strength || '--'} accent={resistanceSummary.strength === '强' ? 'down' : resistanceSummary.strength === '弱' ? 'up' : 'normal'} tooltip="该压力位的阻力程度。强度越高，价格在此位置被压回的可能性越大。" />
                <MetricMini label="压力状态" value={formatResistanceStatus(resistanceSummary.status)} accent={resistanceStatusAccent} emphasis tooltip="当前价格与压力位的关系。例如「接近压力区」说明价格快要碰到天花板，「突破压力区」说明阻力已被突破。" />
              </div>
              <div className="space-y-2">
                {resistanceLevels.map((level, i) => (
                  <LevelCard key={level.level} level={level} index={i} type="resistance" />
                ))}
              </div>
            </div>
          )}
        </section>

        {/* Anomaly charts */}
        <section className="grid gap-4 xl:grid-cols-2">
          <PriceVolumeChart events={priceVolumeEvents} />
          <BlockFlowChart events={blockFlowEvents} />
        </section>

        {/* Signal config (login required) */}
        {privateAccessReady ? (
          <section className="rounded-2xl border border-border bg-card p-5">
            <div className="flex flex-wrap items-start justify-between gap-3">
              <h3 className="text-base font-semibold text-white">信号推送配置</h3>
            </div>

            {signalError && <div className="mt-3 rounded-xl border border-rose-400/40 bg-rose-500/10 px-4 py-3 text-sm text-rose-200">{signalError}</div>}
            {signalNotice && <div className="mt-3 rounded-xl border border-emerald-400/40 bg-emerald-500/10 px-4 py-3 text-sm text-emerald-200">{signalNotice}</div>}

            {signalConfig && (
              <div className="mt-4 rounded-xl border border-border bg-black/20 p-4">
                <div className="flex flex-wrap items-center justify-between gap-3">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="text-sm font-medium text-white">{symbol}</span>
                    <span className={`rounded-full px-2 py-0.5 text-[11px] ${webhookConfigured ? 'bg-emerald-500/15 text-emerald-200' : 'bg-amber-500/15 text-amber-200'}`}>
                      {webhookConfigured ? 'Webhook 已配置' : 'Webhook 未配置'}
                    </span>
                    <a href="/settings" className="text-[11px] text-white/45 underline decoration-white/20 underline-offset-2 transition hover:text-primary hover:decoration-primary/50">设置</a>
                  </div>
                  <button
                    type="button"
                    role="switch"
                    aria-checked={Boolean(signalConfig.is_enabled)}
                    onClick={() => updateLocalSignalConfig({ is_enabled: !signalConfig.is_enabled })}
                    className={`inline-flex items-center gap-2 rounded-lg border px-3 py-1.5 text-left text-xs transition focus:outline-none focus:ring-2 focus:ring-primary/40 ${
                      signalConfig.is_enabled
                        ? 'border-emerald-300/60 bg-emerald-500/18 text-emerald-50'
                        : 'border-amber-300/35 bg-amber-500/10 text-white/88 hover:border-amber-300/55'
                    }`}
                  >
                    <span className="font-medium">{signalConfig.is_enabled ? '信号已开启' : '信号未开启'}</span>
                    <span className={`relative inline-flex h-5 w-9 shrink-0 rounded-full border transition ${signalConfig.is_enabled ? 'border-emerald-200/60 bg-emerald-300/90' : 'border-amber-200/30 bg-black/25'}`}>
                      <span className={`absolute top-0.5 h-4 w-4 rounded-full bg-white shadow-sm transition-all ${signalConfig.is_enabled ? 'left-[18px]' : 'left-0.5'}`} />
                    </span>
                  </button>
                </div>

                {(!webhookConfigured || !webhookConfig.is_enabled) && (
                  <div className="mt-2.5 rounded-lg border border-amber-400/30 bg-amber-500/10 px-3 py-2 text-xs text-amber-200">
                    Webhook 未配置或未启用，信号不会发出。<a href="/settings" className="ml-1 underline underline-offset-2 hover:text-amber-100">去配置</a>
                  </div>
                )}

                <div className="mt-3 flex flex-wrap items-center gap-2">
                  <select
                    value={signalConfig.strategy_id || ''}
                    onChange={(e) => updateLocalSignalConfig({ strategy_id: e.target.value })}
                    className="min-w-[140px] flex-1 rounded-lg border border-border bg-black/30 px-2.5 py-1.5 text-xs text-white outline-none transition focus:border-primary"
                  >
                    <option value="">请选择策略</option>
                    {activeStrategies.map((s) => (
                      <option key={s.id} value={s.id}>{s.name}</option>
                    ))}
                  </select>
                  <button
                    type="button"
                    disabled={savingSignal}
                    onClick={handleSaveSignalConfig}
                    className="rounded-lg bg-primary px-4 py-1.5 text-xs font-medium text-white shadow-sm transition hover:bg-primary/85 disabled:cursor-not-allowed disabled:opacity-60"
                  >
                    {savingSignal ? '保存中...' : '保存配置'}
                  </button>
                </div>

                <p className="mt-2.5 text-[11px] leading-relaxed text-white/40">系统每小时自动评估一次策略，产生 BUY/SELL 信号时推送到 Webhook。</p>

                <details className="mt-3 rounded-lg border border-border/80 bg-black/30 p-3">
                  <summary className="cursor-pointer text-xs font-medium text-white/85">查看触发条件与 Payload 模板</summary>
                  <div className="mt-3 space-y-3 text-xs text-white/75">
                    <div className="space-y-1">
                      <div>失败重试：最多 {SIGNAL_MAX_ATTEMPTS} 次，退避间隔 {SIGNAL_BACKOFF_STEPS.join(' / ')}。</div>
                      <div>策略参数：{formatStrategyCycleHint(selectedStrategy)}</div>
                    </div>
                    <div>
                      <div className="mb-1 text-white/65">Payload 模板</div>
                      <pre className="overflow-x-auto rounded-lg border border-border/80 bg-black/50 p-2 text-[11px] leading-5 text-emerald-200">
                        {JSON.stringify(buildSignalPayloadTemplate(symbol, signalConfig.strategy_id), null, 2)}
                      </pre>
                    </div>
                  </div>
                </details>
              </div>
            )}
          </section>
        ) : null}
      </div>
    </>
  )
}

// ── Chart Components ──

function DailyHistoryChart({ bars }) {
  const containerRef = useRef(null)
  const chartRef = useRef(null)

  useEffect(() => {
    let cleanup = () => {}
    let cancelled = false
    const render = async () => {
      if (!containerRef.current || !Array.isArray(bars) || bars.length === 0) {
        if (chartRef.current) { chartRef.current.remove(); chartRef.current = null }
        return
      }
      const { createChart, ColorType } = await import('lightweight-charts')
      if (cancelled || !containerRef.current) return
      if (chartRef.current) { chartRef.current.remove(); chartRef.current = null }

      const chart = createChart(containerRef.current, {
        width: containerRef.current.clientWidth || 700,
        height: 320,
        layout: { background: { type: ColorType.Solid, color: 'rgba(9, 13, 24, 0.6)' }, textColor: '#E5E7EB' },
        rightPriceScale: { borderColor: 'rgba(148,163,184,0.35)' },
        timeScale: { borderColor: 'rgba(148,163,184,0.35)' },
        grid: { vertLines: { color: 'rgba(148,163,184,0.1)' }, horzLines: { color: 'rgba(148,163,184,0.1)' } },
        crosshair: { mode: 0 },
      })

      // Prepare data sorted by date
      const sorted = [...bars]
        .map((b) => ({ time: b.date, value: b.close }))
        .filter((b) => b.time && !Number.isNaN(b.value))
        .sort((a, b) => (a.time < b.time ? -1 : a.time > b.time ? 1 : 0))

      if (sorted.length === 0) {
        chart.remove()
        return
      }

      // Determine trend: rising (red) or falling (green) per Chinese convention
      const firstClose = sorted[0].value
      const lastClose = sorted[sorted.length - 1].value
      const isRising = lastClose >= firstClose

      const lineColor = isRising ? 'rgba(239, 68, 68, 0.9)' : 'rgba(34, 197, 94, 0.9)'
      const topAreaColor = isRising ? 'rgba(239, 68, 68, 0.28)' : 'rgba(34, 197, 94, 0.28)'
      const bottomAreaColor = isRising ? 'rgba(239, 68, 68, 0.02)' : 'rgba(34, 197, 94, 0.02)'

      const areaSeries = chart.addAreaSeries({
        lineColor,
        topColor: topAreaColor,
        bottomColor: bottomAreaColor,
        lineWidth: 2,
        priceFormat: { type: 'price', precision: 3, minMove: 0.001 },
      })
      areaSeries.setData(sorted)
      chart.timeScale().fitContent()
      chartRef.current = chart

      const onResize = () => {
        if (!containerRef.current || !chartRef.current) return
        chartRef.current.applyOptions({ width: containerRef.current.clientWidth || 700 })
        chartRef.current.timeScale().fitContent()
      }
      window.addEventListener('resize', onResize)
      cleanup = () => {
        window.removeEventListener('resize', onResize)
        if (chartRef.current) { chartRef.current.remove(); chartRef.current = null }
      }
    }
    render()
    return () => { cancelled = true; cleanup() }
  }, [bars])

  return <div ref={containerRef} className="mt-4 w-full overflow-hidden rounded-xl border border-border bg-black/20" />
}

function OverlayIntradayChart({ series, benchmark, symbol }) {
  const containerRef = useRef(null)
  const chartRef = useRef(null)

  useEffect(() => {
    let cleanup = () => {}
    let cancelled = false
    const renderChart = async () => {
      if (!containerRef.current || !Array.isArray(series) || series.length === 0) {
        if (chartRef.current) { chartRef.current.remove(); chartRef.current = null }
        return
      }
      const { createChart, ColorType } = await import('lightweight-charts')
      if (cancelled || !containerRef.current) return
      if (chartRef.current) { chartRef.current.remove(); chartRef.current = null }

      const chart = createChart(containerRef.current, {
        width: containerRef.current.clientWidth || 700,
        height: 280,
        layout: { background: { type: ColorType.Solid, color: 'rgba(9, 13, 24, 0.6)' }, textColor: '#E5E7EB' },
        rightPriceScale: { borderColor: 'rgba(148,163,184,0.35)' },
        timeScale: { borderColor: 'rgba(148,163,184,0.35)', timeVisible: true, secondsVisible: false },
        grid: { vertLines: { color: 'rgba(148,163,184,0.1)' }, horzLines: { color: 'rgba(148,163,184,0.1)' } },
      })

      const stockLine = chart.addLineSeries({ color: '#f59e0b', lineWidth: 2, title: `${symbol}（归一化）` })
      const benchmarkLine = chart.addLineSeries({ color: '#38bdf8', lineWidth: 2, title: `${benchmark || 'HSI'}（归一化）` })
      stockLine.setData(toAscendingSeriesData(series, 'stock_norm'))
      benchmarkLine.setData(toAscendingSeriesData(series, 'benchmark_norm'))
      chart.timeScale().fitContent()
      chartRef.current = chart

      const onResize = () => {
        if (!containerRef.current || !chartRef.current) return
        chartRef.current.applyOptions({ width: containerRef.current.clientWidth || 700 })
        chartRef.current.timeScale().fitContent()
      }
      window.addEventListener('resize', onResize)
      cleanup = () => { window.removeEventListener('resize', onResize); if (chartRef.current) { chartRef.current.remove(); chartRef.current = null } }
    }
    renderChart()
    return () => { cancelled = true; cleanup() }
  }, [benchmark, series, symbol])

  return <div ref={containerRef} className="w-full overflow-hidden rounded-xl border border-border bg-black/20" />
}

const ANOMALY_TYPE_META = {
  volume_spike: { label: '量能突增', color: '#f59e0b' },
  price_breakout_up: { label: '向上突破', color: '#ef4444' },
  price_breakout_down: { label: '向下突破', color: '#22c55e' },
}

function PriceVolumeChart({ events }) {
  const containerRef = useRef(null)
  const chartRef = useRef(null)

  useEffect(() => {
    let cleanup = () => {}
    let cancelled = false
    const render = async () => {
      if (!containerRef.current) return
      if (chartRef.current) { chartRef.current.remove(); chartRef.current = null }
      if (!events || events.length === 0) return
      const { createChart, ColorType } = await import('lightweight-charts')
      if (cancelled || !containerRef.current) return

      const chart = createChart(containerRef.current, {
        width: containerRef.current.clientWidth || 500, height: 220,
        layout: { background: { type: ColorType.Solid, color: 'rgba(9, 13, 24, 0.6)' }, textColor: '#E5E7EB' },
        rightPriceScale: { borderColor: 'rgba(148,163,184,0.35)' },
        timeScale: { borderColor: 'rgba(148,163,184,0.35)', timeVisible: true, secondsVisible: false },
        grid: { vertLines: { color: 'rgba(148,163,184,0.1)' }, horzLines: { color: 'rgba(148,163,184,0.1)' } },
      })

      const byType = {}
      for (const item of events) {
        const type = item.anomaly_type || 'unknown'
        if (!byType[type]) byType[type] = []
        const ts = Math.floor(new Date(item.detected_at).getTime() / 1000)
        if (!ts || Number.isNaN(ts)) continue
        byType[type].push({ time: ts, value: item.score ?? 0 })
      }
      for (const [type, points] of Object.entries(byType)) {
        const meta = ANOMALY_TYPE_META[type] || { color: '#94a3b8' }
        const deduped = deduplicateTimeSeries(points)
        const s = chart.addLineSeries({ color: meta.color, lineWidth: 2, title: '', crosshairMarkerRadius: 5, lastValueVisible: false, priceLineVisible: false })
        s.setData(deduped)
        s.setMarkers(deduped.map((p) => ({ time: p.time, position: 'inBar', shape: 'circle', color: meta.color, size: 1.5 })))
      }
      chart.timeScale().fitContent()
      chartRef.current = chart

      const onResize = () => {
        if (!containerRef.current || !chartRef.current) return
        chartRef.current.applyOptions({ width: containerRef.current.clientWidth || 500 })
        chartRef.current.timeScale().fitContent()
      }
      window.addEventListener('resize', onResize)
      cleanup = () => { window.removeEventListener('resize', onResize); if (chartRef.current) { chartRef.current.remove(); chartRef.current = null } }
    }
    render()
    return () => { cancelled = true; cleanup() }
  }, [events])

  const legendItems = Object.entries(ANOMALY_TYPE_META).map(([, meta]) => meta)
  return (
    <section className="rounded-2xl border border-border bg-card p-5">
      <h3 className="text-base font-semibold text-white">量价异动</h3>
      <p className="mt-1 text-xs text-white/55">按时间分布的异动事件，Y 轴为评分。</p>
      {!events || events.length === 0 ? (
        <div className="mt-3 rounded-xl border border-dashed border-border px-4 py-5 text-sm text-white/50">暂无事件</div>
      ) : (
        <>
          <div ref={containerRef} className="mt-3 w-full overflow-hidden rounded-xl border border-border bg-black/20" />
          <div className="mt-2 flex flex-wrap gap-3 text-xs text-white/55">
            {legendItems.map((item) => (
              <span key={item.label} className="inline-flex items-center gap-1.5">
                <span className="h-2 w-2 rounded-full" style={{ backgroundColor: item.color }} />{item.label}
              </span>
            ))}
          </div>
        </>
      )}
    </section>
  )
}

function BlockFlowChart({ events }) {
  const containerRef = useRef(null)
  const chartRef = useRef(null)

  useEffect(() => {
    let cleanup = () => {}
    let cancelled = false
    const render = async () => {
      if (!containerRef.current) return
      if (chartRef.current) { chartRef.current.remove(); chartRef.current = null }
      if (!events || events.length === 0) return
      const { createChart, ColorType } = await import('lightweight-charts')
      if (cancelled || !containerRef.current) return

      const chart = createChart(containerRef.current, {
        width: containerRef.current.clientWidth || 500, height: 220,
        layout: { background: { type: ColorType.Solid, color: 'rgba(9, 13, 24, 0.6)' }, textColor: '#E5E7EB' },
        rightPriceScale: { borderColor: 'rgba(148,163,184,0.35)' },
        timeScale: { borderColor: 'rgba(148,163,184,0.35)', timeVisible: true, secondsVisible: false },
        grid: { vertLines: { color: 'rgba(148,163,184,0.1)' }, horzLines: { color: 'rgba(148,163,184,0.1)' } },
      })

      const histSeries = chart.addHistogramSeries({ title: '', priceLineVisible: false, lastValueVisible: false, priceFormat: { type: 'volume' } })
      const rawHistData = events.map((item) => {
        const ts = Math.floor(new Date(item.detected_at).getTime() / 1000)
        if (!ts || Number.isNaN(ts)) return null
        return { time: ts, value: item.net_inflow ?? 0, color: (item.net_inflow ?? 0) >= 0 ? 'rgba(239, 68, 68, 0.75)' : 'rgba(34, 197, 94, 0.75)' }
      }).filter(Boolean)
      histSeries.setData(deduplicateTimeSeries(rawHistData))

      const strengthSeries = chart.addLineSeries({ color: '#f59e0b', lineWidth: 1.5, title: '', priceScaleId: 'strength', lastValueVisible: false, priceLineVisible: false })
      chart.priceScale('strength').applyOptions({ scaleMargins: { top: 0.05, bottom: 0.05 } })
      const rawStrengthData = events.map((item) => {
        const ts = Math.floor(new Date(item.detected_at).getTime() / 1000)
        if (!ts || Number.isNaN(ts)) return null
        return { time: ts, value: item.direction_strength ?? 0 }
      }).filter(Boolean)
      strengthSeries.setData(deduplicateTimeSeries(rawStrengthData))

      chart.timeScale().fitContent()
      chartRef.current = chart

      const onResize = () => {
        if (!containerRef.current || !chartRef.current) return
        chartRef.current.applyOptions({ width: containerRef.current.clientWidth || 500 })
        chartRef.current.timeScale().fitContent()
      }
      window.addEventListener('resize', onResize)
      cleanup = () => { window.removeEventListener('resize', onResize); if (chartRef.current) { chartRef.current.remove(); chartRef.current = null } }
    }
    render()
    return () => { cancelled = true; cleanup() }
  }, [events])

  return (
    <section className="rounded-2xl border border-border bg-card p-5">
      <h3 className="text-base font-semibold text-white">大单流向</h3>
      <p className="mt-1 text-xs text-white/55">柱状为净流向金额（红入绿出），折线为方向强度。</p>
      {!events || events.length === 0 ? (
        <div className="mt-3 rounded-xl border border-dashed border-border px-4 py-5 text-sm text-white/50">暂无事件</div>
      ) : (
        <>
          <div ref={containerRef} className="mt-3 w-full overflow-hidden rounded-xl border border-border bg-black/20" />
          <div className="mt-2 flex flex-wrap gap-3 text-xs text-white/55">
            <span className="inline-flex items-center gap-1.5"><span className="h-2 w-2 rounded-full bg-red-500" />资金流入</span>
            <span className="inline-flex items-center gap-1.5"><span className="h-2 w-2 rounded-full bg-green-500" />资金流出</span>
            <span className="inline-flex items-center gap-1.5"><span className="h-0.5 w-3 rounded-full bg-amber-500" />方向强度</span>
          </div>
        </>
      )}
    </section>
  )
}

// ── Shared sub-components ──

function LevelCard({ level, index, type }) {
  const isSupport = type === 'support'
  const levelLabel = isSupport ? formatSupportLevelLabel(level.level, index) : formatResistanceLevelLabel(level.level, index)
  const statusText = isSupport ? formatSupportStatus(level.status) : formatResistanceStatus(level.status)

  return (
    <div className="rounded-xl border border-border bg-black/20 px-3 py-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="text-sm font-semibold text-white">{levelLabel} · {formatNumber(level.price, 3)}</div>
        <div className="text-xs text-white/60">{statusText}</div>
      </div>
      <div className="mt-2 grid gap-2 text-xs text-white/70 md:grid-cols-2 xl:grid-cols-4">
        <div>{isSupport ? '支撑' : '压力'}区间：{formatNumber(level.band_low, 3)} ~ {formatNumber(level.band_high, 3)}</div>
        <div>距当前价：{formatDistancePct(level.distance_pct)}</div>
        <div>强度：{level.strength || '--'}（{formatNumber(level.score, 1)}）</div>
        <div>历史触达：{level.touch_count ?? '--'}</div>
      </div>
    </div>
  )
}

function MACDChart({ series }) {
  const containerRef = useRef(null)
  const chartRef = useRef(null)

  useEffect(() => {
    let cleanup = () => {}
    let cancelled = false
    const render = async () => {
      if (!containerRef.current || !Array.isArray(series) || series.length === 0) {
        if (chartRef.current) { chartRef.current.remove(); chartRef.current = null }
        return
      }
      const { createChart, ColorType } = await import('lightweight-charts')
      if (cancelled || !containerRef.current) return
      if (chartRef.current) { chartRef.current.remove(); chartRef.current = null }

      const chart = createChart(containerRef.current, {
        width: containerRef.current.clientWidth || 700,
        height: 260,
        layout: { background: { type: ColorType.Solid, color: 'rgba(9, 13, 24, 0.6)' }, textColor: '#E5E7EB' },
        rightPriceScale: { borderColor: 'rgba(148,163,184,0.35)' },
        timeScale: { borderColor: 'rgba(148,163,184,0.35)' },
        grid: { vertLines: { color: 'rgba(148,163,184,0.08)' }, horzLines: { color: 'rgba(148,163,184,0.08)' } },
        crosshair: { mode: 0 },
      })

      // Prepare data sorted by date
      const sorted = [...series]
        .filter((p) => p.date)
        .sort((a, b) => (a.date < b.date ? -1 : a.date > b.date ? 1 : 0))

      if (sorted.length === 0) { chart.remove(); return }

      // Histogram series (red/green bars)
      const histogramSeries = chart.addHistogramSeries({
        priceFormat: { type: 'price', precision: 4, minMove: 0.0001 },
        priceScaleId: 'right',
      })
      histogramSeries.setData(
        sorted.map((p) => ({
          time: p.date,
          value: p.histogram,
          color: p.histogram >= 0 ? 'rgba(239, 68, 68, 0.7)' : 'rgba(34, 197, 94, 0.7)',
        }))
      )

      // DIF line (blue)
      const difSeries = chart.addLineSeries({
        color: '#60a5fa',
        lineWidth: 2,
        title: 'DIF',
        priceFormat: { type: 'price', precision: 4, minMove: 0.0001 },
      })
      difSeries.setData(sorted.map((p) => ({ time: p.date, value: p.dif })))

      // Signal line (orange)
      const signalSeries = chart.addLineSeries({
        color: '#fb923c',
        lineWidth: 2,
        title: '信号线',
        priceFormat: { type: 'price', precision: 4, minMove: 0.0001 },
      })
      signalSeries.setData(sorted.map((p) => ({ time: p.date, value: p.signal })))

      // Detect golden cross / death cross and add markers
      const markers = []
      for (let i = 1; i < sorted.length; i++) {
        const prev = sorted[i - 1]
        const curr = sorted[i]
        if (prev.dif <= prev.signal && curr.dif > curr.signal) {
          markers.push({
            time: curr.date,
            position: 'belowBar',
            color: '#ef4444',
            shape: 'arrowUp',
            text: '金叉',
          })
        } else if (prev.dif >= prev.signal && curr.dif < curr.signal) {
          markers.push({
            time: curr.date,
            position: 'aboveBar',
            color: '#22c55e',
            shape: 'arrowDown',
            text: '死叉',
          })
        }
      }
      if (markers.length > 0) {
        difSeries.setMarkers(markers)
      }

      // Zero line
      const zeroLine = chart.addLineSeries({
        color: 'rgba(148,163,184,0.3)',
        lineWidth: 1,
        lineStyle: 2,
        priceFormat: { type: 'price', precision: 4, minMove: 0.0001 },
        crosshairMarkerVisible: false,
        lastValueVisible: false,
        priceLineVisible: false,
      })
      zeroLine.setData(sorted.map((p) => ({ time: p.date, value: 0 })))

      chart.timeScale().fitContent()
      chartRef.current = chart

      const onResize = () => {
        if (!containerRef.current || !chartRef.current) return
        chartRef.current.applyOptions({ width: containerRef.current.clientWidth || 700 })
        chartRef.current.timeScale().fitContent()
      }
      window.addEventListener('resize', onResize)
      cleanup = () => {
        window.removeEventListener('resize', onResize)
        if (chartRef.current) { chartRef.current.remove(); chartRef.current = null }
      }
    }
    render()
    return () => { cancelled = true; cleanup() }
  }, [series])

  return (
    <div>
      <div className="mb-2 flex items-center gap-4 text-[11px] text-white/50">
        <span className="flex items-center gap-1"><span className="inline-block h-0.5 w-4 rounded bg-[#60a5fa]" />DIF（快线）</span>
        <span className="flex items-center gap-1"><span className="inline-block h-0.5 w-4 rounded bg-[#fb923c]" />信号线</span>
        <span className="flex items-center gap-1"><span className="inline-block h-2 w-2 rounded-sm bg-red-500/70" />多头柱</span>
        <span className="flex items-center gap-1"><span className="inline-block h-2 w-2 rounded-sm bg-green-500/70" />空头柱</span>
        <span className="flex items-center gap-1"><span className="text-red-400">▲</span>金叉</span>
        <span className="flex items-center gap-1"><span className="text-green-400">▼</span>死叉</span>
      </div>
      <div ref={containerRef} className="w-full overflow-hidden rounded-xl border border-border bg-black/20" />
    </div>
  )
}

function BollingerChart({ series }) {
  const containerRef = useRef(null)
  const chartRef = useRef(null)

  useEffect(() => {
    let cleanup = () => {}
    let cancelled = false
    const render = async () => {
      if (!containerRef.current || !Array.isArray(series) || series.length === 0) {
        if (chartRef.current) { chartRef.current.remove(); chartRef.current = null }
        return
      }
      const { createChart, ColorType } = await import('lightweight-charts')
      if (cancelled || !containerRef.current) return
      if (chartRef.current) { chartRef.current.remove(); chartRef.current = null }

      const chart = createChart(containerRef.current, {
        width: containerRef.current.clientWidth || 700,
        height: 300,
        layout: { background: { type: ColorType.Solid, color: 'rgba(9, 13, 24, 0.6)' }, textColor: '#E5E7EB' },
        rightPriceScale: { borderColor: 'rgba(148,163,184,0.35)' },
        timeScale: { borderColor: 'rgba(148,163,184,0.35)' },
        grid: { vertLines: { color: 'rgba(148,163,184,0.08)' }, horzLines: { color: 'rgba(148,163,184,0.08)' } },
        crosshair: { mode: 0 },
      })

      const sorted = [...series]
        .filter((p) => p.date)
        .sort((a, b) => (a.date < b.date ? -1 : a.date > b.date ? 1 : 0))

      if (sorted.length === 0) { chart.remove(); return }

      // Upper band (filled area down to lower)
      const upperArea = chart.addAreaSeries({
        lineColor: 'rgba(139, 92, 246, 0.5)',
        topColor: 'rgba(139, 92, 246, 0.12)',
        bottomColor: 'rgba(139, 92, 246, 0.0)',
        lineWidth: 1,
        lineStyle: 2,
        priceFormat: { type: 'price', precision: 3, minMove: 0.001 },
        crosshairMarkerVisible: false,
        lastValueVisible: false,
        priceLineVisible: false,
      })
      upperArea.setData(sorted.map((p) => ({ time: p.date, value: p.upper })))

      // Lower band (filled area)
      const lowerArea = chart.addAreaSeries({
        lineColor: 'rgba(139, 92, 246, 0.5)',
        topColor: 'rgba(9, 13, 24, 0.0)',
        bottomColor: 'rgba(9, 13, 24, 0.0)',
        lineWidth: 1,
        lineStyle: 2,
        priceFormat: { type: 'price', precision: 3, minMove: 0.001 },
        crosshairMarkerVisible: false,
        lastValueVisible: false,
        priceLineVisible: false,
      })
      lowerArea.setData(sorted.map((p) => ({ time: p.date, value: p.lower })))

      // Middle band (MA20)
      const middleLine = chart.addLineSeries({
        color: 'rgba(251, 146, 60, 0.7)',
        lineWidth: 1,
        lineStyle: 2,
        priceFormat: { type: 'price', precision: 3, minMove: 0.001 },
        crosshairMarkerVisible: false,
        lastValueVisible: false,
        priceLineVisible: false,
      })
      middleLine.setData(sorted.map((p) => ({ time: p.date, value: p.middle })))

      // Close price line (main)
      const firstClose = sorted[0].close
      const lastClose = sorted[sorted.length - 1].close
      const isRising = lastClose >= firstClose
      const priceColor = isRising ? 'rgba(239, 68, 68, 0.9)' : 'rgba(34, 197, 94, 0.9)'

      const priceLine = chart.addLineSeries({
        color: priceColor,
        lineWidth: 2,
        priceFormat: { type: 'price', precision: 3, minMove: 0.001 },
      })
      priceLine.setData(sorted.map((p) => ({ time: p.date, value: p.close })))

      // Mark points where price touches upper/lower band
      const markers = []
      for (const pt of sorted) {
        if (pt.close >= pt.upper) {
          markers.push({ time: pt.date, position: 'aboveBar', color: '#ef4444', shape: 'circle', text: '' })
        } else if (pt.close <= pt.lower) {
          markers.push({ time: pt.date, position: 'belowBar', color: '#22c55e', shape: 'circle', text: '' })
        }
      }
      if (markers.length > 0) {
        priceLine.setMarkers(markers)
      }

      chart.timeScale().fitContent()
      chartRef.current = chart

      const onResize = () => {
        if (!containerRef.current || !chartRef.current) return
        chartRef.current.applyOptions({ width: containerRef.current.clientWidth || 700 })
        chartRef.current.timeScale().fitContent()
      }
      window.addEventListener('resize', onResize)
      cleanup = () => {
        window.removeEventListener('resize', onResize)
        if (chartRef.current) { chartRef.current.remove(); chartRef.current = null }
      }
    }
    render()
    return () => { cancelled = true; cleanup() }
  }, [series])

  return (
    <div>
      <div className="mb-2 flex items-center gap-4 text-[11px] text-white/50">
        <span className="flex items-center gap-1"><span className="inline-block h-0.5 w-4 rounded" style={{ background: 'rgba(239,68,68,0.9)' }} />收盘价</span>
        <span className="flex items-center gap-1"><span className="inline-block h-0.5 w-4 rounded border border-dashed" style={{ borderColor: 'rgba(139,92,246,0.5)' }} />上轨/下轨</span>
        <span className="flex items-center gap-1"><span className="inline-block h-0.5 w-4 rounded border border-dashed" style={{ borderColor: 'rgba(251,146,60,0.7)' }} />中轨(MA20)</span>
        <span className="flex items-center gap-1"><span className="inline-block h-1.5 w-1.5 rounded-full bg-red-400" />触及上轨</span>
        <span className="flex items-center gap-1"><span className="inline-block h-1.5 w-1.5 rounded-full bg-green-400" />触及下轨</span>
      </div>
      <div ref={containerRef} className="w-full overflow-hidden rounded-xl border border-border bg-black/20" />
    </div>
  )
}

function MetricMini({ label, value, accent = 'normal', emphasis = false, featured = false, marketAccent = false, tooltip = '' }) {
  const [showTip, setShowTip] = useState(false)
  const tipRef = useRef(null)
  const risingColor = marketAccent ? 'text-rose-300' : 'text-emerald-300'
  const fallingColor = marketAccent ? 'text-emerald-300' : 'text-rose-300'
  const color = accent === 'up' ? risingColor : accent === 'down' ? fallingColor : 'text-white'
  const emphasisTone = accent === 'up' ? 'border-emerald-400/45 bg-emerald-500/10 ring-1 ring-emerald-300/20' : accent === 'down' ? 'border-rose-400/45 bg-rose-500/10 ring-1 ring-rose-300/20' : 'border-primary/45 bg-primary/10 ring-1 ring-primary/25'
  const featuredTone = accent === 'up' ? 'border-rose-400/50 bg-rose-500/12 ring-1 ring-rose-300/25 shadow-[0_10px_30px_rgba(251,113,133,0.18)]' : accent === 'down' ? 'border-emerald-400/50 bg-emerald-500/12 ring-1 ring-emerald-300/25 shadow-[0_10px_30px_rgba(52,211,153,0.18)]' : 'border-primary/55 bg-primary/12 ring-1 ring-primary/30 shadow-[0_10px_30px_rgba(76,106,255,0.16)]'
  const containerTone = featured ? (marketAccent ? featuredTone : 'border-primary/55 bg-primary/12 ring-1 ring-primary/30 shadow-[0_10px_30px_rgba(76,106,255,0.16)]') : emphasis ? emphasisTone : 'border-border bg-black/20'
  const featuredLabelColor = marketAccent ? (accent === 'up' ? 'text-rose-200/90' : accent === 'down' ? 'text-emerald-200/90' : 'text-primary/85') : 'text-primary/85'

  useEffect(() => {
    if (!showTip) return
    const onClick = (e) => { if (tipRef.current && !tipRef.current.contains(e.target)) setShowTip(false) }
    document.addEventListener('mousedown', onClick)
    return () => document.removeEventListener('mousedown', onClick)
  }, [showTip])

  return (
    <div className={`relative rounded-xl border px-3 py-2 ${featured ? 'px-4 py-3' : ''} ${containerTone}`}>
      <div className={`flex items-center gap-1 text-xs ${featured ? featuredLabelColor : 'text-white/50'}`}>
        <span>{label}</span>
        {tooltip ? (
          <span
            ref={tipRef}
            className="relative cursor-help"
            onMouseEnter={() => setShowTip(true)}
            onMouseLeave={() => setShowTip(false)}
            onClick={(e) => { e.stopPropagation(); setShowTip((v) => !v) }}
          >
            <svg viewBox="0 0 16 16" fill="currentColor" className="h-3 w-3 opacity-40 hover:opacity-70 transition">
              <path d="M8 1a7 7 0 1 0 0 14A7 7 0 0 0 8 1Zm-.75 3.5a.75.75 0 0 1 1.5 0v.01a.75.75 0 0 1-1.5 0V4.5ZM7 7a1 1 0 0 1 2 0v4a1 1 0 0 1-2 0V7Z" />
            </svg>
            {showTip ? (
              <div className="absolute bottom-full left-1/2 z-50 mb-2 w-56 -translate-x-1/2 rounded-xl border border-white/10 bg-[#1a1d25]/95 px-3 py-2.5 text-[11px] leading-relaxed text-white/80 shadow-xl backdrop-blur-sm">
                {tooltip}
                <div className="absolute left-1/2 top-full -translate-x-1/2 border-4 border-transparent border-t-[#1a1d25]/95" />
              </div>
            ) : null}
          </span>
        ) : null}
      </div>
      <div className={`mt-1 font-semibold ${color} ${featured ? 'text-2xl leading-none tracking-tight' : 'text-sm'}`}>{value}</div>
    </div>
  )
}

// ── Utility functions ──

function toAscendingSeriesData(series, valueField) {
  if (!Array.isArray(series) || series.length === 0) return []
  const valueByTime = new Map()
  for (const item of series) {
    const timestamp = Math.floor(new Date(item.ts).getTime() / 1000)
    const value = Number(item?.[valueField])
    if (!timestamp || Number.isNaN(timestamp) || Number.isNaN(value)) continue
    valueByTime.set(timestamp, value)
  }
  return Array.from(valueByTime.entries()).sort((a, b) => a[0] - b[0]).map(([time, value]) => ({ time, value }))
}

function deduplicateTimeSeries(points) {
  if (!points || points.length === 0) return []
  const sorted = [...points].sort((a, b) => a.time - b.time)
  const result = [sorted[0]]
  for (let i = 1; i < sorted.length; i++) {
    if (sorted[i].time !== result[result.length - 1].time) result.push(sorted[i])
  }
  return result
}

function formatPercent(value) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return '--'
  const num = Number(value) * 100
  const sign = num > 0 ? '+' : ''
  return `${sign}${num.toFixed(2)}%`
}

function formatPercentMaybeNull(value) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return '--'
  return formatPercent(value)
}

function formatNumber(value, digits = 2) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return '--'
  return Number(value).toLocaleString('zh-CN', { maximumFractionDigits: digits, minimumFractionDigits: digits })
}

function formatNumberMaybeNull(value, digits = 2) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return '--'
  return formatNumber(value, digits)
}

function formatCompact(value) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return '--'
  return Number(value).toLocaleString('zh-CN', { maximumFractionDigits: 2 })
}

function formatTurnoverRate(value) {
  if (value === null || value === undefined || Number.isNaN(Number(value)) || Number(value) <= 0) return '--'
  return `${Number(value).toFixed(2)}%`
}

function formatPercentDirect(value) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return '--'
  return `${Number(value).toFixed(2)}%`
}

function formatYiAmount(value, currencySymbol = '') {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return '--'
  const num = Number(value)
  if (Math.abs(num) >= 1e8) return `${currencySymbol}${(num / 1e8).toLocaleString('zh-CN', { maximumFractionDigits: 2 })} 亿`
  return `${currencySymbol}${num.toLocaleString('zh-CN', { maximumFractionDigits: 2 })}`
}

function formatYiCurrency(value, currencySymbol = '¥') {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return '--'
  const num = Number(value)
  if (Math.abs(num) >= 1e8) return `${currencySymbol}${(num / 1e8).toLocaleString('zh-CN', { maximumFractionDigits: 2 })} 亿`
  return `${currencySymbol}${num.toLocaleString('zh-CN', { maximumFractionDigits: 2 })}`
}

function formatYiShares(value) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return '--'
  return `${(Number(value) / 1e8).toLocaleString('zh-CN', { maximumFractionDigits: 2 })} 亿股`
}

function formatMultiple(value) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return '--'
  return `${Number(value).toLocaleString('zh-CN', { maximumFractionDigits: 2 })} 倍`
}

function formatPEG(value) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return '--'
  return Number(value).toLocaleString('zh-CN', { maximumFractionDigits: 2 })
}

function pegAccent(value) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return 'normal'
  if (value < 1) return 'up'
  if (value > 2) return 'down'
  return 'normal'
}

function formatBollingerBW(value) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return '--'
  return `${Number(value).toFixed(2)}%`
}

function formatPercentB(value) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return '--'
  return Number(value).toFixed(2)
}

function percentBAccent(value) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return 'normal'
  if (value > 1) return 'down'
  if (value < 0) return 'up'
  return 'normal'
}

function buildFundamentalsReportLabel(meta) {
  return isAnnualReportDate(meta?.fy_report_date) ? 'FY' : '最近披露'
}

function buildFundamentalsMetaLine(meta) {
  if (!meta) return ''
  const parts = []
  if (isAnnualReportDate(meta.fy_report_date)) {
    const fyYear = extractFiscalYear(meta.fy_report_date)
    if (fyYear) parts.push(`FY ${fyYear}`)
  } else if (meta.fy_report_date) {
    parts.push(`最近披露截至 ${meta.fy_report_date}`)
  }
  if (meta.ttm_report_date && meta.ttm_report_date !== meta.fy_report_date) {
    parts.push(`TTM 截至 ${meta.ttm_report_date}`)
  }
  if (meta.updated_at) parts.push(`更新：${formatDateTime(meta.updated_at)}`)
  if (parts.length === 0 && meta.warning) return meta.warning
  return parts.join(' · ')
}

function isAnnualReportDate(value) {
  if (!value) return false
  return /(?:-|\/)?12(?:-|\/)?31$/.test(String(value).trim())
}

function extractFiscalYear(value) {
  if (!value) return ''
  const match = String(value).match(/^(\d{4})/)
  return match ? match[1] : ''
}

function formatDateTime(value) {
  if (!value) return '--'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString('zh-CN', { hour12: false })
}

function formatDistancePct(value) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return '--'
  const num = Number(value)
  const sign = num > 0 ? '+' : ''
  return `${sign}${num.toFixed(2)}%`
}

function formatSource(source) {
  const normalized = String(source || '').toLowerCase()
  if (normalized === 'tencent-qt') return '腾讯行情'
  return source || '腾讯行情'
}

function formatSupportStatus(status) {
  const map = { 临近支撑: '接近支撑区', 回踩支撑: '回踩支撑区', 位于支撑上方: '高于支撑区', 跌破支撑: '跌破支撑区' }
  return map[String(status || '').trim()] || String(status || '').trim() || '--'
}

function formatSupportLevelLabel(level, index = 0) {
  const map = { S1: '最近支撑位', S2: '第二支撑位', S3: '第三支撑位' }
  const key = String(level || '').trim().toUpperCase()
  if (map[key]) return map[key]
  return index === 0 ? '最近支撑位' : `第${index + 1}支撑位`
}

function formatResistanceStatus(status) {
  const map = { 临近压力: '接近压力区', 回踩压力: '回踩压力区', 位于压力下方: '位于压力区下方', 突破压力: '突破压力区' }
  return map[String(status || '').trim()] || String(status || '').trim() || '--'
}

function formatResistanceLevelLabel(level, index = 0) {
  const map = { R1: '最近压力位', R2: '第二压力位', R3: '第三压力位' }
  const key = String(level || '').trim().toUpperCase()
  if (map[key]) return map[key]
  return index === 0 ? '最近压力位' : `第${index + 1}压力位`
}

function formatMAStatus(status) {
  const map = { 双双站上: '价格高于 MA20 / MA200', 双双跌破: '价格低于 MA20 / MA200', '站上MA20但低于MA200': '短强长弱', '跌破MA20但高于MA200': '短弱长强' }
  return map[String(status || '').trim()] || String(status || '').trim() || '--'
}

function maAccent(priceRef, maValue) {
  if (!maValue || maValue <= 0) return 'normal'
  return priceRef >= maValue ? 'up' : 'down'
}

function rsiAccent(rsi) {
  if (rsi === null || rsi === undefined || rsi < 0) return 'normal'
  if (rsi >= 70) return 'down'
  if (rsi <= 30) return 'up'
  return 'normal'
}

function formatStrategyCycleHint(strategy) {
  if (!strategy) return '未选择策略'
  const schemaItems = Array.isArray(strategy.param_schema) ? strategy.param_schema : []
  const defaultParams = strategy.default_params && typeof strategy.default_params === 'object' ? strategy.default_params : {}
  const cycleItems = schemaItems.filter((item) => {
    const key = String(item?.key || '').toLowerCase()
    const label = String(item?.label || '')
    return /周期|窗口|回看/.test(label) || /period|window|lookback/.test(key)
  })
  if (cycleItems.length === 0) return '策略未声明固定周期参数'
  return cycleItems.map((item) => {
    const key = String(item?.key || '').trim()
    const label = String(item?.label || key || '参数').trim()
    const value = defaultParams[key] ?? item?.default
    return `${label}=${value ?? '--'}`
  }).join('，')
}

function buildSignalPayloadTemplate(sym, strategyID) {
  const lines = ['股票交易信号来啦！', '类型：正式信号', `股票：${sym || '--'}`, '方向：BUY', '时间：2026-03-19 18:00:00']
  if (strategyID) lines.push(`策略：${strategyID}`)
  lines.push('原因：策略触发原因说明')
  return { msgtype: 'text', text: { content: lines.join('\n') } }
}

function detectExchange(symbol) {
  if (!symbol) return 'HKEX'
  const upper = String(symbol).toUpperCase()
  if (upper.endsWith('.SH')) return 'SSE'
  if (upper.endsWith('.SZ')) return 'SZSE'
  if (upper.endsWith('.HK')) return 'HKEX'
  const digits = upper.replace(/\D/g, '')
  if (digits.length === 6) {
    if (digits[0] === '6') return 'SSE'
    if (digits[0] === '0' || digits[0] === '3') return 'SZSE'
  }
  return 'HKEX'
}

function detectExchangeLabel(symbol) {
  const ex = detectExchange(symbol)
  return { SSE: '沪市', SZSE: '深市', HKEX: '港股' }[ex] || ex
}
