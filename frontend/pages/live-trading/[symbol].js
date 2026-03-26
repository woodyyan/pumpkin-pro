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
const SIGNAL_DISPATCH_INTERVAL_SECONDS = 2
const SIGNAL_MAX_ATTEMPTS = 4
const SIGNAL_BACKOFF_STEPS = ['1 分钟', '5 分钟', '15 分钟']

export default function LiveTradingDetailPage() {
  const router = useRouter()
  const { symbol: rawSymbol } = router.query
  const symbol = rawSymbol ? decodeURIComponent(rawSymbol).toUpperCase() : ''

  const { isLoggedIn, openAuthModal, ready, user } = useAuth()

  const [dailyBars, setDailyBars] = useState([])
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
    '1Y': 260, '2Y': 520, '5Y': 1300, '10Y': 2600, ALL: 9999,
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

  // Load daily bars on mount and range change
  useEffect(() => {
    if (!ready || !symbol) return
    loadDailyBars(symbol, dailyRange)
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ready, symbol, dailyRange])

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
      cooldown_seconds: 300,
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
      ...(prev || { symbol, strategy_id: '', is_enabled: false, cooldown_seconds: 300, thresholds: {} }),
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
          cooldown_seconds: Number(signalConfig.cooldown_seconds) || 300,
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
            <div className="mt-4 grid gap-3 md:grid-cols-3">
              <MetricMini
                label="最新价"
                value={formatNumber(snapshot.last_price, 3)}
                accent={snapshot.change_rate > 0 ? 'up' : snapshot.change_rate < 0 ? 'down' : 'normal'}
                featured
                marketAccent
              />
              <MetricMini label="涨跌幅" value={formatPercent(snapshot.change_rate)} accent={snapshot.change_rate >= 0 ? 'up' : 'down'} />
              <MetricMini label="量比" value={formatNumber(snapshot.volume_ratio, 2)} />
              <MetricMini label="成交量" value={formatCompact(snapshot.volume)} />
              <MetricMini label={`成交额(${isAShare ? 'CNY' : 'HKD'})`} value={formatCompact(snapshot.turnover)} />
              <MetricMini label="振幅" value={formatPercent(snapshot.amplitude)} />
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
            <div className="mt-4 grid gap-3 md:grid-cols-3">
              <MetricMini label={`市值(${fundamentalsCurrencyCode})`} value={formatYiCurrency(fundamentalsItems.market_cap, fundamentalsCurrencySymbol)} emphasis />
              <MetricMini label="股息收益率" value={formatPercentMaybeNull(fundamentalsItems.dividend_yield)} />
              <MetricMini label="市盈率(TTM)" value={formatMultiple(fundamentalsItems.pe_ttm)} />
              <MetricMini label={`净利润(${fundamentalsReportLabel} · ${fundamentalsCurrencyCode})`} value={formatYiAmount(fundamentalsItems.net_profit_fy, fundamentalsCurrencySymbol)} />
              <MetricMini label={`收入(${fundamentalsReportLabel} · ${fundamentalsCurrencyCode})`} value={formatYiAmount(fundamentalsItems.revenue_fy, fundamentalsCurrencySymbol)} />
              <MetricMini label="流通股" value={formatYiShares(fundamentalsItems.float_shares)} />
            </div>
          )}
        </section>

        {/* Daily history chart */}
        <section className="rounded-2xl border border-border bg-card p-5">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <h3 className="text-base font-semibold text-white">历史走势</h3>
          </div>
          <div className="mt-3 flex flex-wrap gap-1.5">
            {DAILY_RANGE_LABELS.map((key) => (
              <button
                key={key}
                type="button"
                onClick={() => setDailyRange(key)}
                className={`rounded-lg px-2.5 py-1 text-xs font-medium transition ${
                  dailyRange === key
                    ? 'bg-primary text-white shadow-sm'
                    : 'bg-black/25 text-white/65 hover:bg-black/35 hover:text-white/85'
                }`}
              >
                {key === 'ALL' ? '全部' : key.replace('D','天').replace('W','周').replace('M','月').replace('Y','年')}
              </button>
            ))}
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
              <h3 className="text-base font-semibold text-white">均线指标（MA20 / MA200）</h3>
              <p className="mt-1 text-xs text-white/60">基于最近 {MA_LOOKBACK_DAYS} 个交易日收盘价计算。</p>
            </div>
            <div className="text-xs text-white/55">
              {movingAveragePayload?.updated_at ? `更新：${formatDateTime(movingAveragePayload.updated_at)}` : '等待数据'}
            </div>
          </div>
          {movingAverageError && <div className="mt-3 rounded-xl border border-amber-400/30 bg-amber-500/10 px-4 py-3 text-sm text-amber-200">{movingAverageError}</div>}
          {!movingAveragePayload ? (
            <div className="mt-3 rounded-xl border border-dashed border-border px-4 py-6 text-sm text-white/50">暂无均线数据。</div>
          ) : (
            <div className="mt-4 grid gap-3 md:grid-cols-5">
              <MetricMini label="MA20" value={formatNumber(movingAveragePayload.ma20, 3)} accent={movingAveragePayload.price_ref >= movingAveragePayload.ma20 ? 'up' : 'down'} />
              <MetricMini label="MA200" value={formatNumber(movingAveragePayload.ma200, 3)} accent={movingAveragePayload.price_ref >= movingAveragePayload.ma200 ? 'up' : 'down'} />
              <MetricMini label="距 MA20" value={formatDistancePct(movingAveragePayload.distance_to_ma20_pct)} accent={movingAveragePayload.distance_to_ma20_pct >= 0 ? 'up' : 'down'} />
              <MetricMini label="距 MA200" value={formatDistancePct(movingAveragePayload.distance_to_ma200_pct)} accent={movingAveragePayload.distance_to_ma200_pct >= 0 ? 'up' : 'down'} />
              <MetricMini label="位置状态" value={formatMAStatus(movingAveragePayload.status)} accent={movingAverageStatusAccent} emphasis />
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
                <MetricMini label="Beta" value={formatNumberMaybeNull(overlayPayload?.metrics?.beta, 3)} accent={overlayPayload?.metrics?.beta != null && overlayPayload.metrics.beta >= 1 ? 'up' : 'normal'} />
                <MetricMini label="Relative Strength" value={formatPercentMaybeNull(overlayPayload?.metrics?.relative_strength)} accent={overlayPayload?.metrics?.relative_strength != null && overlayPayload.metrics.relative_strength >= 0 ? 'up' : 'down'} />
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
                <MetricMini label="最近支撑位" value={supportSummary.nearest_price ? formatNumber(supportSummary.nearest_price, 3) : '--'} accent={supportStatusAccent} emphasis />
                <MetricMini label="距最近支撑位" value={formatDistancePct(supportSummary.distance_pct)} accent={supportSummary.distance_pct >= 0 ? 'normal' : 'down'} />
                <MetricMini label="支撑强度" value={supportSummary.strength || '--'} accent={supportSummary.strength === '强' ? 'up' : supportSummary.strength === '弱' ? 'down' : 'normal'} />
                <MetricMini label="支撑状态" value={formatSupportStatus(supportSummary.status)} accent={supportStatusAccent} emphasis />
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
                <MetricMini label="最近压力位" value={resistanceSummary.nearest_price ? formatNumber(resistanceSummary.nearest_price, 3) : '--'} accent={resistanceStatusAccent} emphasis />
                <MetricMini label="距最近压力位" value={formatDistancePct(resistanceSummary.distance_pct)} accent={resistanceSummary.distance_pct >= 0 ? 'normal' : 'up'} />
                <MetricMini label="压力强度" value={resistanceSummary.strength || '--'} accent={resistanceSummary.strength === '强' ? 'down' : resistanceSummary.strength === '弱' ? 'up' : 'normal'} />
                <MetricMini label="压力状态" value={formatResistanceStatus(resistanceSummary.status)} accent={resistanceStatusAccent} emphasis />
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
              <div>
                <h3 className="text-base font-semibold text-white">信号推送配置</h3>
                <p className="mt-1 text-xs text-white/60">Webhook 配置在设置页统一管理，此处仅配置该股票的策略与推送开关。</p>
              </div>
              <div className="text-xs text-white/55">
                {webhookConfig.updated_at ? `Webhook 更新于 ${formatDateTime(webhookConfig.updated_at)}` : 'Webhook 未配置'}
              </div>
            </div>

            {signalError && <div className="mt-3 rounded-xl border border-rose-400/40 bg-rose-500/10 px-4 py-3 text-sm text-rose-200">{signalError}</div>}
            {signalNotice && <div className="mt-3 rounded-xl border border-emerald-400/40 bg-emerald-500/10 px-4 py-3 text-sm text-emerald-200">{signalNotice}</div>}

            <div className="mt-4 space-y-3 rounded-xl border border-border bg-black/20 p-4">
              <div className="flex flex-wrap items-center gap-2 text-xs">
                <span className={`rounded-full px-2.5 py-1 ${webhookConfigured ? 'bg-emerald-500/15 text-emerald-200' : 'bg-amber-500/15 text-amber-200'}`}>
                  {webhookConfigured ? '已配置 URL' : '未配置 URL'}
                </span>
                <span className={`rounded-full px-2.5 py-1 ${webhookConfig.is_enabled ? 'bg-emerald-500/15 text-emerald-200' : 'bg-rose-500/15 text-rose-200'}`}>
                  {webhookConfig.is_enabled ? '已启用发送' : '已禁用发送'}
                </span>
              </div>
              {(!webhookConfigured || !webhookConfig.is_enabled) && (
                <div className="rounded-lg border border-amber-400/30 bg-amber-500/10 px-3 py-2 text-xs text-amber-200">
                  未配置或未启用时，股票信号不会发出。
                </div>
              )}
              <a href="/settings" className="inline-flex rounded-lg border border-border px-3 py-1.5 text-xs text-white/85 transition hover:border-primary hover:text-primary">去设置页</a>
            </div>

            {signalConfig && (
              <div className="mt-4 rounded-xl border border-border bg-black/20 p-4">
                <div className="flex flex-wrap items-center justify-between gap-3">
                  <div className="text-sm font-medium text-white">{symbol} 信号配置</div>
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

                <div className="mt-3 grid gap-2 md:grid-cols-[1.2fr_1fr]">
                  <select
                    value={signalConfig.strategy_id || ''}
                    onChange={(e) => updateLocalSignalConfig({ strategy_id: e.target.value })}
                    className="rounded-lg border border-border bg-black/30 px-2 py-1.5 text-xs text-white outline-none transition focus:border-primary"
                  >
                    <option value="">请选择策略</option>
                    {activeStrategies.map((s) => (
                      <option key={s.id} value={s.id}>{s.name}</option>
                    ))}
                  </select>
                  <input
                    type="number"
                    min={10}
                    max={3600}
                    value={signalConfig.cooldown_seconds ?? 300}
                    onChange={(e) => updateLocalSignalConfig({ cooldown_seconds: Number(e.target.value) || 300 })}
                    className="rounded-lg border border-border bg-black/30 px-2 py-1.5 text-xs text-white outline-none transition focus:border-primary"
                  />
                </div>
                <div className="mt-2 flex flex-wrap items-center justify-between gap-2">
                  <div className="text-[11px] text-white/50">推送间隔：秒（10~3600），该间隔内重复信号不会推送。</div>
                  <button
                    type="button"
                    disabled={savingSignal}
                    onClick={handleSaveSignalConfig}
                    className="rounded-lg bg-primary px-4 py-1.5 text-xs font-medium text-white shadow-sm transition hover:bg-primary/85 disabled:cursor-not-allowed disabled:opacity-60"
                  >
                    {savingSignal ? '保存中...' : '保存配置'}
                  </button>
                </div>

                <details className="mt-3 rounded-lg border border-border/80 bg-black/30 p-3">
                  <summary className="cursor-pointer text-xs font-medium text-white/85">查看触发条件与 Payload 模板</summary>
                  <div className="mt-3 space-y-3 text-xs text-white/75">
                    <div className="space-y-1">
                      <div>启用信号后，后台约每 {SIGNAL_DISPATCH_INTERVAL_SECONDS} 秒扫描待发送队列。</div>
                      <div>失败重试：最多 {SIGNAL_MAX_ATTEMPTS} 次，退避间隔 {SIGNAL_BACKOFF_STEPS.join(' / ')}。</div>
                      <div>冷却时间：{Number(signalConfig.cooldown_seconds) || 300} 秒。</div>
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

function MetricMini({ label, value, accent = 'normal', emphasis = false, featured = false, marketAccent = false }) {
  const risingColor = marketAccent ? 'text-rose-300' : 'text-emerald-300'
  const fallingColor = marketAccent ? 'text-emerald-300' : 'text-rose-300'
  const color = accent === 'up' ? risingColor : accent === 'down' ? fallingColor : 'text-white'
  const emphasisTone = accent === 'up' ? 'border-emerald-400/45 bg-emerald-500/10 ring-1 ring-emerald-300/20' : accent === 'down' ? 'border-rose-400/45 bg-rose-500/10 ring-1 ring-rose-300/20' : 'border-primary/45 bg-primary/10 ring-1 ring-primary/25'
  const featuredTone = accent === 'up' ? 'border-rose-400/50 bg-rose-500/12 ring-1 ring-rose-300/25 shadow-[0_10px_30px_rgba(251,113,133,0.18)]' : accent === 'down' ? 'border-emerald-400/50 bg-emerald-500/12 ring-1 ring-emerald-300/25 shadow-[0_10px_30px_rgba(52,211,153,0.18)]' : 'border-primary/55 bg-primary/12 ring-1 ring-primary/30 shadow-[0_10px_30px_rgba(76,106,255,0.16)]'
  const containerTone = featured ? (marketAccent ? featuredTone : 'border-primary/55 bg-primary/12 ring-1 ring-primary/30 shadow-[0_10px_30px_rgba(76,106,255,0.16)]') : emphasis ? emphasisTone : 'border-border bg-black/20'
  const featuredLabelColor = marketAccent ? (accent === 'up' ? 'text-rose-200/90' : accent === 'down' ? 'text-emerald-200/90' : 'text-primary/85') : 'text-primary/85'

  return (
    <div className={`rounded-xl border px-3 py-2 ${featured ? 'px-4 py-3' : ''} ${containerTone}`}>
      <div className={`text-xs ${featured ? featuredLabelColor : 'text-white/50'}`}>{label}</div>
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
