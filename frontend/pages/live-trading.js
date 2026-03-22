import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import { requestJson } from '../lib/api'
import { useAuth } from '../lib/auth-context'
import { isAuthRequiredError } from '../lib/auth-storage'

const POLL_MS = 2000
const OVERLAY_WINDOW_MINUTES = 60
const SUPPORT_REFRESH_MS = 60 * 1000
const SIGNAL_CENTER_REFRESH_MS = 15 * 1000
const SUPPORT_LOOKBACK_DAYS = 120
const MA_LOOKBACK_DAYS = 240
const SIGNAL_DISPATCH_INTERVAL_SECONDS = 2
const SIGNAL_MAX_ATTEMPTS = 4
const SIGNAL_BACKOFF_STEPS = ['1 分钟', '5 分钟', '15 分钟']
const SIGNAL_CONFIG_VIEWS = [
  { key: 'active', label: '激活标的' },
  { key: 'enabled', label: '已开启' },
  { key: 'all', label: '全部股票' },
]
const DEFAULT_WATCHLIST = { items: [], active_symbol: '', session_state: 'idle' }
const DEFAULT_WEBHOOK_CONFIG = {
  url: '',
  has_secret: false,
  is_enabled: true,
  timeout_ms: 3000,
  updated_at: '',
}

export default function LiveTradingPage() {
  const { isLoggedIn, openAuthModal, ready, user } = useAuth()
  const [watchlist, setWatchlist] = useState(DEFAULT_WATCHLIST)
  const [marketOverview, setMarketOverview] = useState(null)
  const [snapshotPayload, setSnapshotPayload] = useState(null)
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
  const [signalConfigBySymbol, setSignalConfigBySymbol] = useState({})
  const [signalConfigView, setSignalConfigView] = useState('active')
  const [webhookConfig, setWebhookConfig] = useState(DEFAULT_WEBHOOK_CONFIG)
  const [savingSignalSymbol, setSavingSignalSymbol] = useState('')
  const [signalNotice, setSignalNotice] = useState('')
  const [signalError, setSignalError] = useState('')
  const [signalErrorNeedsLogin, setSignalErrorNeedsLogin] = useState(false)
  const [symbolInput, setSymbolInput] = useState('')
  const [nameInput, setNameInput] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')
  const [errorNeedsLogin, setErrorNeedsLogin] = useState(false)
  const [lastUpdateAt, setLastUpdateAt] = useState('')
  const supportRefreshRef = useRef({ symbol: '', refreshedAt: 0 })
  const resistanceRefreshRef = useRef({ symbol: '', refreshedAt: 0 })
  const movingAverageRefreshRef = useRef({ symbol: '', refreshedAt: 0 })
  const signalCenterRefreshRef = useRef(0)

  const activeSymbol = watchlist.active_symbol
  const activeExchange = detectExchange(activeSymbol)
  const isActiveAShare = activeExchange === 'SSE' || activeExchange === 'SZSE'
  const sessionState = watchlist.session_state || 'idle'
  const supportSummary = supportPayload?.summary || null
  const supportLevels = Array.isArray(supportPayload?.levels) ? supportPayload.levels : []
  const resistanceSummary = resistancePayload?.summary || null
  const resistanceLevels = Array.isArray(resistancePayload?.levels) ? resistancePayload.levels : []
  const supportStatusAccent = supportSummary?.status === '跌破支撑'
    ? 'down'
    : supportSummary?.status === '临近支撑' || supportSummary?.status === '回踩支撑'
      ? 'up'
      : 'normal'
  const resistanceStatusAccent = resistanceSummary?.status === '突破压力'
    ? 'up'
    : resistanceSummary?.status === '临近压力' || resistanceSummary?.status === '回踩压力'
      ? 'down'
      : 'normal'
  const movingAverageStatusAccent = movingAveragePayload?.status === '双双站上'
    ? 'up'
    : movingAveragePayload?.status === '双双跌破'
      ? 'down'
      : 'normal'
  const privateAccessReady = ready && isLoggedIn
  const authIdentityKey = String(user?.id || user?.email || '')
  const webhookConfigured = Boolean(webhookConfig.url)

  const resetPrivateState = useCallback(() => {
    setWatchlist(DEFAULT_WATCHLIST)
    setSnapshotPayload(null)
    setOverlayPayload(null)
    setSupportPayload(null)
    setSupportError('')
    setResistancePayload(null)
    setResistanceError('')
    setMovingAveragePayload(null)
    setMovingAverageError('')
    setPriceVolumeEvents([])
    setBlockFlowEvents([])
    setActiveStrategies([])
    setSignalConfigBySymbol({})
    setSignalConfigView('active')
    setWebhookConfig(DEFAULT_WEBHOOK_CONFIG)
    setSavingSignalSymbol('')
    setSignalNotice('')
    setSignalError('')
    setSignalErrorNeedsLogin(false)
    setError('')
    setErrorNeedsLogin(false)
    setLastUpdateAt('')
    supportRefreshRef.current = { symbol: '', refreshedAt: 0 }
    resistanceRefreshRef.current = { symbol: '', refreshedAt: 0 }
    movingAverageRefreshRef.current = { symbol: '', refreshedAt: 0 }
    signalCenterRefreshRef.current = 0
  }, [])

  const updateError = (nextError, nextNeedsLogin = false) => {
    setError(nextError)
    setErrorNeedsLogin(nextNeedsLogin)
  }

  const applyRequestError = (err, fallbackText) => {
    updateError(err.message || fallbackText, isAuthRequiredError(err))
  }

  const applySignalError = (err, fallbackText) => {
    setSignalNotice('')
    setSignalError(err.message || fallbackText)
    setSignalErrorNeedsLogin(isAuthRequiredError(err))
  }

  const updateLocalSignalConfig = (symbol, patch) => {
    setSignalConfigBySymbol((prev) => {
      const previous = prev[symbol] || {
        symbol,
        strategy_id: activeStrategies[0]?.id || '',
        is_enabled: false,
        cooldown_seconds: 300,
        thresholds: {},
      }
      return {
        ...prev,
        [symbol]: {
          ...previous,
          ...patch,
        },
      }
    })
  }

  const sortedWatchlist = useMemo(() => {
    return [...(watchlist.items || [])].sort((a, b) => Number(b.is_active) - Number(a.is_active))
  }, [watchlist.items])

  const strategyByID = useMemo(() => {
    const mapped = {}
    activeStrategies.forEach((strategy) => {
      if (strategy?.id) {
        mapped[strategy.id] = strategy
      }
    })
    return mapped
  }, [activeStrategies])

  const loadWatchlist = async () => {
    const data = await requestJson('/api/live/watchlist')
    const nextState = {
      items: data.items || [],
      active_symbol: data.active_symbol || '',
      session_state: data.session_state || 'idle',
    }
    setWatchlist(nextState)
    return nextState
  }

  const loadMarketOverview = async () => {
    const exchangeParam = isActiveAShare ? 'SSE' : ''
    const qs = exchangeParam ? `?exchange=${exchangeParam}` : ''
    const data = await requestJson(`/api/live/market/overview${qs}`)
    setMarketOverview(data)
  }

  const loadActiveStrategies = async () => {
    const data = await requestJson('/api/strategies/active')
    const items = Array.isArray(data?.items) ? data.items : []
    setActiveStrategies(items)
    return items
  }

  const loadSignalConfigs = async () => {
    const data = await requestJson('/api/signal-configs')
    const items = Array.isArray(data?.items) ? data.items : []
    const mapped = {}
    items.forEach((item) => {
      if (item?.symbol) {
        mapped[item.symbol] = item
      }
    })
    setSignalConfigBySymbol(mapped)
    return mapped
  }

  const loadWebhookConfig = async () => {
    const data = await requestJson('/api/webhook')
    const item = data?.item || null
    if (!item) {
      setWebhookConfig(DEFAULT_WEBHOOK_CONFIG)
      return null
    }
    setWebhookConfig({
      url: item.url || '',
      has_secret: Boolean(item.has_secret),
      is_enabled: item.is_enabled !== false,
      timeout_ms: Number(item.timeout_ms) > 0 ? Number(item.timeout_ms) : 3000,
      updated_at: item.updated_at || '',
    })
    return item
  }

  const loadSignalCenter = async ({ force = false } = {}) => {
    const now = Date.now()
    if (!force && now - signalCenterRefreshRef.current < SIGNAL_CENTER_REFRESH_MS) {
      return
    }
    const [strategies] = await Promise.all([
      loadActiveStrategies(),
      loadSignalConfigs(),
      loadWebhookConfig(),
    ])

    setSignalConfigBySymbol((prev) => {
      const next = { ...prev }
      const defaultStrategyID = strategies[0]?.id || ''
      sortedWatchlist.forEach((item) => {
        if (!next[item.symbol]) {
          next[item.symbol] = {
            symbol: item.symbol,
            strategy_id: defaultStrategyID,
            is_enabled: false,
            cooldown_seconds: 300,
            thresholds: {},
          }
        }
      })
      return next
    })

    signalCenterRefreshRef.current = now
  }

  const getSignalConfigForSymbol = (symbol) => {
    return signalConfigBySymbol[symbol] || {
      symbol,
      strategy_id: activeStrategies[0]?.id || '',
      is_enabled: false,
      cooldown_seconds: 300,
      thresholds: {},
    }
  }

  const visibleSignalWatchlist = useMemo(() => {
    if (signalConfigView === 'all') {
      return sortedWatchlist
    }
    if (signalConfigView === 'enabled') {
      return sortedWatchlist.filter((item) => Boolean(signalConfigBySymbol[item.symbol]?.is_enabled))
    }
    if (!activeSymbol) {
      return []
    }
    return sortedWatchlist.filter((item) => item.symbol === activeSymbol)
  }, [activeSymbol, signalConfigBySymbol, signalConfigView, sortedWatchlist])

  const loadSupportLevels = async (symbol, { force = false } = {}) => {
    if (!symbol) return

    const now = Date.now()
    const cache = supportRefreshRef.current
    const hitRefreshWindow = !force && cache.symbol === symbol && now - cache.refreshedAt < SUPPORT_REFRESH_MS
    if (hitRefreshWindow) {
      return
    }

    try {
      const encoded = encodeURIComponent(symbol)
      const supportData = await requestJson(
        `/api/live/symbols/${encoded}/support-levels?period=daily&lookback_days=${SUPPORT_LOOKBACK_DAYS}`
      )
      setSupportPayload(supportData)
      setSupportError('')
      supportRefreshRef.current = { symbol, refreshedAt: now }
    } catch (err) {
      setSupportError(err.message || '支撑位数据暂不可用')
      if (force) {
        setSupportPayload(null)
      }
    }
  }

  const loadResistanceLevels = async (symbol, { force = false } = {}) => {
    if (!symbol) return

    const now = Date.now()
    const cache = resistanceRefreshRef.current
    const hitRefreshWindow = !force && cache.symbol === symbol && now - cache.refreshedAt < SUPPORT_REFRESH_MS
    if (hitRefreshWindow) {
      return
    }

    try {
      const encoded = encodeURIComponent(symbol)
      const resistanceData = await requestJson(
        `/api/live/symbols/${encoded}/resistance-levels?period=daily&lookback_days=${SUPPORT_LOOKBACK_DAYS}`
      )
      setResistancePayload(resistanceData)
      setResistanceError('')
      resistanceRefreshRef.current = { symbol, refreshedAt: now }
    } catch (err) {
      setResistanceError(err.message || '压力位数据暂不可用')
      if (force) {
        setResistancePayload(null)
      }
    }
  }

  const loadMovingAverages = async (symbol, { force = false } = {}) => {
    if (!symbol) return

    const now = Date.now()
    const cache = movingAverageRefreshRef.current
    const hitRefreshWindow = !force && cache.symbol === symbol && now - cache.refreshedAt < SUPPORT_REFRESH_MS
    if (hitRefreshWindow) {
      return
    }

    try {
      const encoded = encodeURIComponent(symbol)
      const movingAverageData = await requestJson(
        `/api/live/symbols/${encoded}/moving-averages?period=daily&lookback_days=${MA_LOOKBACK_DAYS}`
      )
      setMovingAveragePayload(movingAverageData)
      setMovingAverageError('')
      movingAverageRefreshRef.current = { symbol, refreshedAt: now }
    } catch (err) {
      setMovingAverageError(err.message || '均线指标暂不可用')
      if (force) {
        setMovingAveragePayload(null)
      }
    }
  }

  const loadSymbolPanels = async (symbol, { forceSupport = false } = {}) => {
    if (!symbol) return
    const encoded = encodeURIComponent(symbol)
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

    setWatchlist((prev) => ({
      ...prev,
      session_state: snapshotData.session_state || prev.session_state,
    }))

    await Promise.all([
      loadSupportLevels(symbol, { force: forceSupport }),
      loadResistanceLevels(symbol, { force: forceSupport }),
      loadMovingAverages(symbol, { force: forceSupport }),
    ])
  }

  const loadPublicPanels = async () => {
    try {
      await loadMarketOverview()
      updateError('')
    } catch (err) {
      applyRequestError(err, '实时数据刷新失败')
    }
  }

  const loadPrivatePanels = async ({ bootstrap = false } = {}) => {
    try {
      if (bootstrap) {
        const watchState = await loadWatchlist()
        if (watchState.active_symbol) {
          await loadSymbolPanels(watchState.active_symbol, { forceSupport: true })
        } else {
          setSnapshotPayload(null)
          setOverlayPayload(null)
          setSupportPayload(null)
          setSupportError('')
          setResistancePayload(null)
          setResistanceError('')
          setMovingAveragePayload(null)
          setMovingAverageError('')
          setPriceVolumeEvents([])
          setBlockFlowEvents([])
          setLastUpdateAt('')
          supportRefreshRef.current = { symbol: '', refreshedAt: 0 }
          resistanceRefreshRef.current = { symbol: '', refreshedAt: 0 }
          movingAverageRefreshRef.current = { symbol: '', refreshedAt: 0 }
        }
      } else if (activeSymbol) {
        await loadSymbolPanels(activeSymbol)
      }

      if (bootstrap || activeSymbol) {
        updateError('')
      }
    } catch (err) {
      applyRequestError(err, '实时数据刷新失败')
    }

    try {
      await loadSignalCenter({ force: bootstrap })
      setSignalError('')
      setSignalErrorNeedsLogin(false)
    } catch (err) {
      applySignalError(err, '信号中心数据刷新失败')
    }
  }

  useEffect(() => {
    if (!ready) return

    loadPublicPanels()
    if (privateAccessReady) {
      loadPrivatePanels({ bootstrap: true })
    } else {
      resetPrivateState()
    }

    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ready, privateAccessReady, authIdentityKey])

  useEffect(() => {
    if (!ready) return

    const timer = setInterval(() => {
      loadPublicPanels()
      if (privateAccessReady) {
        loadPrivatePanels()
      }
    }, POLL_MS)

    return () => clearInterval(timer)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeSymbol, ready, privateAccessReady, authIdentityKey])

  const handleAddWatch = async (event) => {
    event.preventDefault()
    setSubmitting(true)
    updateError('')
    try {
      await requestJson('/api/live/watchlist', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ symbol: symbolInput, name: nameInput }),
      })
      setNameInput('')
      const nextWatchlist = await loadWatchlist()
      if (nextWatchlist.active_symbol) {
        await loadSymbolPanels(nextWatchlist.active_symbol, { forceSupport: true })
      }
    } catch (err) {
      applyRequestError(err, '添加关注失败')
    } finally {
      setSubmitting(false)
    }
  }

  const handleActivate = async (symbol) => {
    updateError('')
    try {
      await requestJson(`/api/live/watchlist/${encodeURIComponent(symbol)}/activate`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ reset_window: true }),
      })
      await loadWatchlist()
      await loadSymbolPanels(symbol, { forceSupport: true })
    } catch (err) {
      applyRequestError(err, '切换激活标的失败')
    }
  }

  const handleDelete = async (symbol) => {
    updateError('')
    try {
      await requestJson(`/api/live/watchlist/${encodeURIComponent(symbol)}`, { method: 'DELETE' })
      const nextWatchlist = await loadWatchlist()
      setSignalConfigBySymbol((prev) => {
        const next = { ...prev }
        delete next[symbol]
        return next
      })
      if (!nextWatchlist.active_symbol) {
        setSnapshotPayload(null)
        setOverlayPayload(null)
        setSupportPayload(null)
        setSupportError('')
        setResistancePayload(null)
        setResistanceError('')
        setMovingAveragePayload(null)
        setMovingAverageError('')
        setPriceVolumeEvents([])
        setBlockFlowEvents([])
        supportRefreshRef.current = { symbol: '', refreshedAt: 0 }
        resistanceRefreshRef.current = { symbol: '', refreshedAt: 0 }
        movingAverageRefreshRef.current = { symbol: '', refreshedAt: 0 }
      } else {
        await loadSymbolPanels(nextWatchlist.active_symbol, { forceSupport: true })
      }
    } catch (err) {
      applyRequestError(err, '删除关注失败')
    }
  }

  const handleSaveSymbolSignalConfig = async (symbol) => {
    const config = getSignalConfigForSymbol(symbol)
    setSavingSignalSymbol(symbol)
    setSignalNotice('')
    setSignalError('')
    try {
      const result = await requestJson(`/api/signal-configs/${encodeURIComponent(symbol)}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          strategy_id: config.strategy_id,
          is_enabled: Boolean(config.is_enabled),
          cooldown_seconds: Number(config.cooldown_seconds) || 300,
          thresholds: config.thresholds || {},
        }),
      })
      if (result?.item?.symbol) {
        setSignalConfigBySymbol((prev) => ({
          ...prev,
          [result.item.symbol]: result.item,
        }))
      }
      setSignalNotice(`${symbol} 信号配置已保存`)
    } catch (err) {
      applySignalError(err, `${symbol} 信号配置保存失败`)
    } finally {
      setSavingSignalSymbol('')
    }
  }

  return (
    <div className="space-y-6">
      <section className="rounded-2xl border border-border bg-card p-6">
        <h1 className="text-2xl font-semibold tracking-tight">实盘监控</h1>
        <p className="mt-3 text-sm leading-7 text-white/65">
          当前仅提供实时监控与异动捕获，不触发任何下单行为。监控面板采用“关注池 + 激活标的”模型（同一时刻展示 1 只激活标的），信号推送支持对关注池内多只股票分别配置与发送。
        </p>
      </section>

      <section className="grid gap-6 lg:grid-cols-[320px_1fr]">
        <div className="space-y-4 rounded-2xl border border-border bg-card p-5">
          {privateAccessReady ? (
            <>
              <div>
                <h2 className="text-lg font-semibold text-white">关注股票池</h2>
                <p className="mt-1 text-xs text-white/50">港股（00700）或 A 股（600519）</p>
              </div>

              <form onSubmit={handleAddWatch} className="space-y-3">
                <input
                  value={symbolInput}
                  onChange={(event) => setSymbolInput(event.target.value.toUpperCase())}
                  placeholder="股票代码，如 00700 或 600519"
                  className="w-full rounded-xl border border-border bg-black/20 px-3 py-2 text-sm text-white outline-none transition focus:border-primary"
                />
                <input
                  value={nameInput}
                  onChange={(event) => setNameInput(event.target.value)}
                  placeholder="备注名称（可选）"
                  className="w-full rounded-xl border border-border bg-black/20 px-3 py-2 text-sm text-white outline-none transition focus:border-primary"
                />
                <button
                  type="submit"
                  disabled={submitting}
                  className="w-full rounded-xl bg-primary px-4 py-2 text-sm font-medium text-black transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
                >
                  {submitting ? '添加中...' : '添加到关注池'}
                </button>
              </form>

              <div className="space-y-2">
                {sortedWatchlist.length === 0 ? (
                  <div className="rounded-xl border border-dashed border-border px-3 py-4 text-center text-xs text-white/50">暂无关注股票</div>
                ) : (
                  sortedWatchlist.map((item) => (
                    <div key={item.symbol} className="rounded-xl border border-border bg-black/20 p-3">
                      <div className="flex items-center justify-between gap-2">
                        <div>
                          <div className="text-sm font-medium text-white">{item.symbol}</div>
                          <div className="text-xs text-white/55">{item.name || '未命名'}</div>
                        </div>
                        {item.is_active && <span className="rounded-full bg-emerald-500/20 px-2 py-1 text-[11px] text-emerald-300">激活中</span>}
                      </div>
                      <div className="mt-3 flex gap-2">
                        {!item.is_active && (
                          <button
                            onClick={() => handleActivate(item.symbol)}
                            className="flex-1 rounded-lg border border-border px-2 py-1 text-xs text-white/80 transition hover:border-primary hover:text-primary"
                          >
                            设为激活
                          </button>
                        )}
                        <button
                          onClick={() => handleDelete(item.symbol)}
                          className="rounded-lg border border-rose-400/40 px-2 py-1 text-xs text-rose-300 transition hover:bg-rose-500/10"
                        >
                          删除
                        </button>
                      </div>
                    </div>
                  ))
                )}
              </div>
            </>
          ) : (
            <AccessPromptCard
              title={ready ? '登录后管理关注股票池' : '正在确认账号状态'}
              description={
                ready
                  ? '关注池、激活标的切换和异动跟踪都属于账号级工作台。未登录时页面只保留公共行情，不再持续请求这些私有数据。'
                  : '正在检查你的登录状态，确认后会自动决定是否加载关注池与信号配置。'
              }
              buttonLabel={ready ? '登录后继续' : '请稍候'}
              onAction={ready ? () => openAuthModal('login', '登录后即可管理关注池、激活标的与实盘信号配置。') : undefined}
              disabled={!ready}
            />
          )}
        </div>

        <div className="space-y-4">
          {privateAccessReady ? (
            <div className="grid gap-4 md:grid-cols-4">
              <MetricCard label="会话状态" value={sessionStateLabel(sessionState)} />
              <MetricCard label="激活标的" value={activeSymbol || '未设置'} />
              <MetricCard label="最后刷新" value={lastUpdateAt ? formatDateTime(lastUpdateAt) : '--'} />
              <MetricCard label="行情来源" value={formatSource(snapshotPayload?.snapshot?.source)} />
            </div>
          ) : (
            <AccessPromptCard
              title={ready ? '登录后开启完整实盘工作台' : '正在确认账号状态'}
              description={
                ready
                  ? '未登录时只刷新公共行情，关注池、激活标的快照、信号配置与异动监控都会在登录后再加载，界面也不会再被私有接口的 401 轮询打闪。'
                  : '正在检查你的登录状态，确认后会自动切换到对应的数据视图。'
              }
              buttonLabel={ready ? '登录查看完整数据' : '请稍候'}
              onAction={ready ? () => openAuthModal('login', '登录后即可查看关注池、激活标的快照与交易信号配置。') : undefined}
              disabled={!ready}
            />
          )}

          {error ? (
            <div className="rounded-xl border border-rose-400/40 bg-rose-500/10 px-4 py-3 text-sm text-rose-200">
              <div>{error}</div>
              {errorNeedsLogin ? (
                <button
                  type="button"
                  onClick={() => openAuthModal('login', '实盘交易相关操作需要登录后才能继续。')}
                  className="mt-2 inline-flex rounded-lg border border-rose-300/40 px-2.5 py-1 text-xs text-rose-100 transition hover:bg-rose-500/15"
                >
                  去登录
                </button>
              ) : null}
            </div>
          ) : null}

          {privateAccessReady ? (
            <section className="rounded-2xl border border-border bg-card p-5">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div>
                  <h3 className="text-base font-semibold text-white">交易信号推送（Webhook）</h3>
                  <p className="mt-1 text-xs text-white/60">设置页统一管理 Webhook；本页只看配置摘要并配置各股票信号。</p>
                </div>
                <div className="text-xs text-white/55">
                  {webhookConfig.updated_at ? `更新于 ${formatDateTime(webhookConfig.updated_at)}` : '未配置'}
                </div>
              </div>

              {signalError ? (
                <div className="mt-3 rounded-xl border border-rose-400/40 bg-rose-500/10 px-4 py-3 text-sm text-rose-200">
                  <div>{signalError}</div>
                  {signalErrorNeedsLogin ? (
                    <button
                      type="button"
                      onClick={() => openAuthModal('login', '信号推送配置需要登录后才能继续。')}
                      className="mt-2 inline-flex rounded-lg border border-rose-300/40 px-2.5 py-1 text-xs text-rose-100 transition hover:bg-rose-500/15"
                    >
                      去登录
                    </button>
                  ) : null}
                </div>
              ) : null}

              {signalNotice ? (
                <div className="mt-3 rounded-xl border border-emerald-400/40 bg-emerald-500/10 px-4 py-3 text-sm text-emerald-200">{signalNotice}</div>
              ) : null}

              <div className="mt-4">
                <div className="space-y-3 rounded-xl border border-border bg-black/20 p-4">
                  <div className="flex flex-wrap items-center gap-2 text-xs">
                    <span className={`rounded-full px-2.5 py-1 ${webhookConfigured ? 'bg-emerald-500/15 text-emerald-200' : 'bg-amber-500/15 text-amber-200'}`}>
                      {webhookConfigured ? '已配置 URL' : '未配置 URL'}
                    </span>
                    <span className={`rounded-full px-2.5 py-1 ${webhookConfig.is_enabled ? 'bg-emerald-500/15 text-emerald-200' : 'bg-rose-500/15 text-rose-200'}`}>
                      {webhookConfig.is_enabled ? '已启用发送' : '已禁用发送'}
                    </span>
                  </div>
                  {!webhookConfigured || !webhookConfig.is_enabled ? (
                    <div className="rounded-lg border border-amber-400/30 bg-amber-500/10 px-3 py-2 text-xs text-amber-200">
                      未配置或未启用时，股票信号不会发出。
                    </div>
                  ) : null}
                  <a
                    href="/settings"
                    className="inline-flex rounded-lg border border-border px-3 py-1.5 text-xs text-white/85 transition hover:border-primary hover:text-primary"
                  >
                    去设置页
                  </a>
                </div>
              </div>

              <div className="mt-4 space-y-3">
                <div className="flex flex-wrap items-end justify-between gap-3">
                  <div>
                    <div className="text-sm font-semibold text-white">股票级信号配置</div>
                  </div>
                  {sortedWatchlist.length > 0 ? (
                    <div className="rounded-full border border-border bg-black/20 px-3 py-1 text-[11px] text-white/65">
                      当前激活：{activeSymbol || '未设置'}
                    </div>
                  ) : null}
                </div>

                {sortedWatchlist.length > 1 ? (
                  <div className="flex flex-wrap gap-2">
                    {SIGNAL_CONFIG_VIEWS.map((view) => {
                      const isActiveView = signalConfigView === view.key
                      const count = view.key === 'all'
                        ? sortedWatchlist.length
                        : view.key === 'enabled'
                          ? sortedWatchlist.filter((item) => Boolean(getSignalConfigForSymbol(item.symbol).is_enabled)).length
                          : activeSymbol
                            ? 1
                            : 0

                      return (
                        <button
                          key={view.key}
                          type="button"
                          onClick={() => setSignalConfigView(view.key)}
                          className={`inline-flex items-center gap-2 rounded-full border px-3 py-1.5 text-xs transition ${
                            isActiveView
                              ? 'border-primary bg-primary/12 text-primary'
                              : 'border-white/15 bg-black/15 text-white/65 hover:border-white/30 hover:text-white'
                          }`}
                        >
                          <span>{view.label}</span>
                          <span className={`rounded-full px-1.5 py-0.5 text-[10px] ${isActiveView ? 'bg-primary/20 text-primary' : 'bg-white/10 text-white/60'}`}>
                            {count}
                          </span>
                        </button>
                      )
                    })}
                  </div>
                ) : null}

                {sortedWatchlist.length === 0 ? (
                  <div className="rounded-xl border border-dashed border-border px-4 py-4 text-xs text-white/50">请先添加关注股票，再配置信号。</div>
                ) : signalConfigView === 'active' && !activeSymbol ? (
                  <div className="rounded-xl border border-dashed border-border px-4 py-4 text-xs text-white/50">当前还没有激活标的，请先在关注池里设置激活股票。</div>
                ) : visibleSignalWatchlist.length === 0 ? (
                  <div className="rounded-xl border border-dashed border-border px-4 py-4 text-xs text-white/50">
                    {signalConfigView === 'enabled' ? '当前没有已开启信号的股票。' : '当前视图下暂无可配置股票。'}
                  </div>
                ) : (
                  visibleSignalWatchlist.map((item) => {
                    const config = getSignalConfigForSymbol(item.symbol)
                    const selectedStrategy = strategyByID[config.strategy_id] || null
                    const payloadTemplate = buildSignalPayloadTemplate(item.symbol, config.strategy_id)
                    return (
                      <div key={`signal-config-${item.symbol}`} className="rounded-xl border border-border bg-black/20 p-3">
                        <div className="flex flex-wrap items-center justify-between gap-3">
                          <div>
                            <div className="flex items-center gap-2 text-sm font-medium text-white">
                              <span>{item.symbol}</span>
                              {item.is_active ? <span className="rounded-full bg-emerald-500/15 px-2 py-0.5 text-[11px] text-emerald-200">激活标的</span> : null}
                            </div>
                            <div className="mt-1 text-xs text-white/55">
                              {Boolean(config.is_enabled)
                                ? '该股票信号已开启，满足策略条件后会进入发送流程。'
                                : '该股票信号当前关闭，开启后才会按策略推送。'}
                            </div>
                          </div>
                          <button
                            type="button"
                            role="switch"
                            aria-checked={Boolean(config.is_enabled)}
                            aria-label={`${item.symbol} 股票信号开关`}
                            onClick={() => updateLocalSignalConfig(item.symbol, { is_enabled: !Boolean(config.is_enabled) })}
                            className={`inline-flex min-w-[260px] items-center justify-between gap-3 rounded-2xl border px-4 py-3 text-left text-sm transition focus:outline-none focus:ring-2 focus:ring-primary/40 ${
                              config.is_enabled
                                ? 'border-emerald-300/60 bg-emerald-500/18 text-emerald-50 shadow-[0_12px_30px_rgba(16,185,129,0.22)]'
                                : 'border-amber-300/35 bg-amber-500/10 text-white/88 shadow-[0_10px_26px_rgba(245,158,11,0.12)] hover:border-amber-300/55 hover:bg-amber-500/14'
                            }`}
                          >
                            <span className="min-w-0 flex-1">
                              <span className="flex items-center gap-2">
                                <span className="font-semibold">{config.is_enabled ? '股票信号已开启' : '股票信号未开启'}</span>
                                <span
                                  className={`rounded-full px-2 py-0.5 text-[11px] font-semibold uppercase tracking-[0.12em] ${
                                    config.is_enabled
                                      ? 'bg-emerald-950/45 text-emerald-100'
                                      : 'bg-amber-950/45 text-amber-100'
                                  }`}
                                >
                                  {config.is_enabled ? 'ON' : 'OFF'}
                                </span>
                              </span>
                              <span className={`mt-1 block text-xs ${config.is_enabled ? 'text-emerald-100/80' : 'text-amber-100/75'}`}>
                                {config.is_enabled ? '满足策略条件后会进入正式推送。' : '点击开启后，才会按所选策略推送。'}
                              </span>
                            </span>
                            <span
                              className={`relative inline-flex h-8 w-14 shrink-0 rounded-full border transition ${
                                config.is_enabled ? 'border-emerald-200/60 bg-emerald-300/90' : 'border-amber-200/30 bg-black/25'
                              }`}
                            >
                              <span
                                className={`absolute top-1 h-6 w-6 rounded-full bg-white shadow-[0_4px_12px_rgba(15,23,42,0.35)] transition-all ${
                                  config.is_enabled ? 'left-7' : 'left-1'
                                }`}
                              />
                            </span>
                          </button>
                        </div>
                        <div className="mt-3 grid gap-2 md:grid-cols-[1.2fr_1fr]">
                          <select
                            value={config.strategy_id || ''}
                            onChange={(event) => updateLocalSignalConfig(item.symbol, { strategy_id: event.target.value })}
                            className="rounded-lg border border-border bg-black/30 px-2 py-1.5 text-xs text-white outline-none transition focus:border-primary"
                          >
                            <option value="">请选择策略</option>
                            {activeStrategies.map((strategy) => (
                              <option key={strategy.id} value={strategy.id}>{strategy.name}</option>
                            ))}
                          </select>
                          <input
                            type="number"
                            min={10}
                            max={3600}
                            value={config.cooldown_seconds ?? 300}
                            onChange={(event) => updateLocalSignalConfig(item.symbol, { cooldown_seconds: Number(event.target.value) || 300 })}
                            className="rounded-lg border border-border bg-black/30 px-2 py-1.5 text-xs text-white outline-none transition focus:border-primary"
                          />
                        </div>
                        <div className="mt-2 flex flex-wrap items-center justify-between gap-2">
                          <div className="text-[11px] text-white/50">冷却时间：秒（10~3600）。保存会同时提交开关、策略和冷却时间。</div>
                          <button
                            type="button"
                            disabled={savingSignalSymbol === item.symbol}
                            onClick={() => handleSaveSymbolSignalConfig(item.symbol)}
                            className="rounded-lg border border-border px-3 py-1.5 text-xs text-white/80 transition hover:border-primary hover:text-primary disabled:cursor-not-allowed disabled:opacity-60"
                          >
                            {savingSignalSymbol === item.symbol ? '保存中...' : '保存该股票配置'}
                          </button>
                        </div>

                        <details className="mt-3 rounded-lg border border-border/80 bg-black/30 p-3">
                          <summary className="cursor-pointer text-xs font-medium text-white/85">查看触发条件与 Payload 模板</summary>
                          <div className="mt-3 space-y-3 text-xs text-white/75">
                            <div className="space-y-1">
                              <div>交易信号何时触发：启用该股票信号后，只要所选策略在后台判定满足条件，就会创建正式信号并投递到 Webhook。</div>
                              <div>后台投递节奏：约每 {SIGNAL_DISPATCH_INTERVAL_SECONDS} 秒扫描待发送队列。</div>
                              <div>失败重试：最多 {SIGNAL_MAX_ATTEMPTS} 次（含首发），退避间隔 {SIGNAL_BACKOFF_STEPS.join(' / ')}。</div>
                              <div>该股冷却时间：{Number(config.cooldown_seconds) || 300} 秒（同一股票重复信号抑制）。</div>
                              <div>策略参数线索：{formatStrategyCycleHint(selectedStrategy)}</div>
                              {selectedStrategy?.description ? <div>策略说明：{selectedStrategy.description}</div> : null}
                            </div>

                            <div>
                              <div className="mb-1 text-white/65">Webhook Headers</div>
                              <ul className="list-disc space-y-0.5 pl-4 text-white/70">
                                <li>Content-Type: application/json</li>
                                <li>X-Pumpkin-Event-Id: sig_xxx</li>
                                <li>X-Pumpkin-Timestamp: Unix 秒时间戳</li>
                                <li>X-Pumpkin-Signature: 仅配置 Secret 时附带（HMAC-SHA256）</li>
                              </ul>
                            </div>

                            <div>
                              <div className="mb-1 text-white/65">Payload 模板（text 消息）</div>
                              <pre className="overflow-x-auto rounded-lg border border-border/80 bg-black/50 p-2 text-[11px] leading-5 text-emerald-200">{JSON.stringify(payloadTemplate, null, 2)}</pre>
                            </div>
                          </div>
                        </details>
                      </div>
                    )
                  })
                )}
              </div>

            </section>
          ) : null}

          <section className="rounded-2xl border border-border bg-card p-5">
            <h3 className="text-base font-semibold text-white">{isActiveAShare ? 'A 股大盘概览' : '港股大盘概览'}</h3>
            <div className="mt-4 grid gap-3 md:grid-cols-3">
              {(marketOverview?.indexes || []).map((index) => (
                <div key={index.code} className="rounded-xl border border-border bg-black/20 p-3">
                  <div className="text-xs text-white/50">{formatMarketIndexTitle(index.name, index.code)}</div>
                  <div className="mt-1 text-lg font-semibold text-white">{formatNumber(index.last, 2)}</div>
                  <div className={`text-xs ${index.change_rate >= 0 ? 'text-rose-300' : 'text-emerald-300'}`}>
                    {formatPercent(index.change_rate)}
                  </div>
                </div>
              ))}
            </div>
          </section>

          {privateAccessReady ? (
            <>
              <section className="rounded-2xl border border-border bg-card p-5">
                <h3 className="text-base font-semibold text-white">激活标的快照</h3>
            {!snapshotPayload?.snapshot ? (
              <div className="mt-3 rounded-xl border border-dashed border-border px-4 py-6 text-sm text-white/50">请先在左侧选择一个激活标的。</div>
            ) : (
              <div className="mt-4 grid gap-3 md:grid-cols-3">
                <MetricMini
                  label="最新价"
                  value={formatNumber(snapshotPayload.snapshot.last_price, 3)}
                  accent={
                    snapshotPayload.snapshot.change_rate > 0
                      ? 'up'
                      : snapshotPayload.snapshot.change_rate < 0
                        ? 'down'
                        : 'normal'
                  }
                  featured
                  marketAccent
                />
                <MetricMini label="涨跌幅" value={formatPercent(snapshotPayload.snapshot.change_rate)} accent={snapshotPayload.snapshot.change_rate >= 0 ? 'up' : 'down'} />
                <MetricMini label="量比" value={formatNumber(snapshotPayload.snapshot.volume_ratio, 2)} />
                <MetricMini label="成交量" value={formatCompact(snapshotPayload.snapshot.volume)} />
                <MetricMini label={`成交额(${isActiveAShare ? 'CNY' : 'HKD'})`} value={formatCompact(snapshotPayload.snapshot.turnover)} />
                <MetricMini label="振幅" value={formatPercent(snapshotPayload.snapshot.amplitude)} />
              </div>
            )}
          </section>

          <section className="rounded-2xl border border-border bg-card p-5">
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div>
                <h3 className="text-base font-semibold text-white">均线指标（MA20 / MA200）</h3>
                <p className="mt-1 text-xs text-white/60">基于最近 {MA_LOOKBACK_DAYS} 个交易日收盘价计算。</p>
              </div>
              <div className="text-xs text-white/55">
                {movingAveragePayload?.updated_at ? `更新时间：${formatDateTime(movingAveragePayload.updated_at)}` : '等待数据'}
              </div>
            </div>

            {movingAverageError ? (
              <div className="mt-3 rounded-xl border border-amber-400/30 bg-amber-500/10 px-4 py-3 text-sm text-amber-200">{movingAverageError}</div>
            ) : null}

            {!movingAveragePayload ? (
              <div className="mt-3 rounded-xl border border-dashed border-border px-4 py-6 text-sm text-white/50">
                暂无可用均线数据（可能仍在预热或样本不足）。
              </div>
            ) : (
              <div className="mt-4 space-y-4">
                <div className="grid gap-3 md:grid-cols-5">
                  <MetricMini label="MA20" value={formatNumber(movingAveragePayload.ma20, 3)} accent={movingAveragePayload.price_ref >= movingAveragePayload.ma20 ? 'up' : 'down'} />
                  <MetricMini label="MA200" value={formatNumber(movingAveragePayload.ma200, 3)} accent={movingAveragePayload.price_ref >= movingAveragePayload.ma200 ? 'up' : 'down'} />
                  <MetricMini label="距 MA20" value={formatDistancePct(movingAveragePayload.distance_to_ma20_pct)} accent={movingAveragePayload.distance_to_ma20_pct >= 0 ? 'up' : 'down'} />
                  <MetricMini label="距 MA200" value={formatDistancePct(movingAveragePayload.distance_to_ma200_pct)} accent={movingAveragePayload.distance_to_ma200_pct >= 0 ? 'up' : 'down'} />
                  <MetricMini label="位置状态" value={formatMAStatus(movingAveragePayload.status)} accent={movingAverageStatusAccent} emphasis />
                </div>

                <div className="rounded-xl border border-border bg-black/20 p-3">
                  <div className="text-xs text-white/55">字段说明</div>
                  <ul className="mt-2 space-y-1 text-xs text-white/70">
                    <li>• MA20 / MA200：分别表示 20 日与 200 日日均收盘价。</li>
                    <li>• 距 MA：当前价相对均线的偏离百分比（正数=当前价在均线上方）。</li>
                  </ul>
                </div>
              </div>
            )}
          </section>

          <section className="rounded-2xl border border-border bg-card p-5">
            <div className="flex flex-wrap items-center justify-between gap-3">
              <h3 className="text-base font-semibold text-white">实时分时叠加（个股 vs 大盘）</h3>
              <div className="text-xs text-white/60">默认窗口：{OVERLAY_WINDOW_MINUTES} 分钟</div>
            </div>

            {!overlayPayload?.series?.length ? (
              <div className="mt-3 rounded-xl border border-dashed border-border px-4 py-6 text-sm text-white/50">分时数据预热中，请稍后。</div>
            ) : (
              <div className="mt-4 space-y-4">
                <OverlayIntradayChart series={overlayPayload.series} benchmark={overlayPayload.benchmark} symbol={overlayPayload.symbol} />
                <div className="grid gap-3 md:grid-cols-4">
                  <MetricMini label="基准指数" value={overlayPayload.benchmark || 'HSI'} />
                  <MetricMini
                    label="Beta"
                    value={formatNumberMaybeNull(overlayPayload?.metrics?.beta, 3)}
                    accent={overlayPayload?.metrics?.beta != null && overlayPayload.metrics.beta >= 1 ? 'up' : 'normal'}
                  />
                  <MetricMini
                    label="Relative Strength"
                    value={formatPercentMaybeNull(overlayPayload?.metrics?.relative_strength)}
                    accent={overlayPayload?.metrics?.relative_strength != null && overlayPayload.metrics.relative_strength >= 0 ? 'up' : 'down'}
                  />
                  <MetricMini
                    label="样本状态"
                    value={`${overlayPayload?.metrics?.sample_count || 0}/${overlayPayload?.metrics?.warmup_min_samples || 30} · ${overlayPayload?.metrics?.is_warmup ? '预热中' : '可用'}`}
                    accent={overlayPayload?.metrics?.is_warmup ? 'normal' : 'up'}
                  />
                </div>
              </div>
            )}
          </section>

          <section className="rounded-2xl border border-border bg-card p-5">
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div>
                <h3 className="text-base font-semibold text-white">支撑位（近{SUPPORT_LOOKBACK_DAYS}天）</h3>
                <p className="mt-1 text-xs text-white/60">
                  基于最近 {SUPPORT_LOOKBACK_DAYS} 个交易日，综合价格形态计算出的支撑参考区间。
                </p>
              </div>
              <div className="text-xs text-white/55">
                {supportPayload?.meta?.updated_at ? `更新时间：${formatDateTime(supportPayload.meta.updated_at)}` : '等待数据'}
              </div>
            </div>

            {supportError ? (
              <div className="mt-3 rounded-xl border border-amber-400/30 bg-amber-500/10 px-4 py-3 text-sm text-amber-200">{supportError}</div>
            ) : null}

            {!supportSummary ? (
              <div className="mt-3 rounded-xl border border-dashed border-border px-4 py-6 text-sm text-white/50">
                暂无可用支撑位数据（可能仍在预热或样本不足）。
              </div>
            ) : (
              <div className="mt-4 space-y-4">
                <div className="grid gap-3 md:grid-cols-4">
                  <MetricMini
                    label="最近支撑位"
                    value={supportSummary.nearest_price ? formatNumber(supportSummary.nearest_price, 3) : '--'}
                    accent={supportStatusAccent}
                    emphasis
                  />
                  <MetricMini
                    label="距最近支撑位"
                    value={formatDistancePct(supportSummary.distance_pct)}
                    accent={supportSummary.distance_pct >= 0 ? 'normal' : 'down'}
                  />
                  <MetricMini
                    label="支撑强度"
                    value={supportSummary.strength || '--'}
                    accent={supportSummary.strength === '强' ? 'up' : supportSummary.strength === '弱' ? 'down' : 'normal'}
                  />
                  <MetricMini
                    label="支撑状态"
                    value={formatSupportStatus(supportSummary.status)}
                    accent={supportStatusAccent}
                    emphasis
                  />
                </div>

                <div className="rounded-xl border border-border bg-black/20 p-3">
                  <div className="text-xs text-white/55">字段说明</div>
                  <ul className="mt-2 space-y-1 text-xs text-white/70">
                    <li>• 支撑位：历史上价格多次止跌或反弹的参考价位（区间），用于判断下方承接力度，不代表一定反弹。</li>
                    <li>• 距最近支撑位：当前价与最近支撑位的百分比距离（正数=当前价在支撑位上方）。</li>
                    <li>• 支撑强度：综合历史触达次数、最近验证时间、反弹幅度得到的分级。</li>
                    <li>• 支撑状态：接近支撑区 / 回踩支撑区 / 高于支撑区 / 跌破支撑区。</li>
                  </ul>
                </div>

                <div className="space-y-2">
                  {supportLevels.length === 0 ? (
                    <div className="rounded-xl border border-dashed border-border px-3 py-4 text-center text-xs text-white/50">暂无支撑位明细</div>
                  ) : (
                    supportLevels.map((level, index) => {
                      const levelLabel = formatSupportLevelLabel(level.level, index)
                      return (
                        <div key={level.level} className="rounded-xl border border-border bg-black/20 px-3 py-3">
                          <div className="flex flex-wrap items-center justify-between gap-2">
                            <div className="text-sm font-semibold text-white">{levelLabel} · {formatNumber(level.price, 3)}</div>
                            <div className="text-xs text-white/60">{formatSupportStatus(level.status)}</div>
                          </div>
                          <div className="mt-2 grid gap-2 text-xs text-white/70 md:grid-cols-2 xl:grid-cols-4">
                            <div>支撑区间：{formatNumber(level.band_low, 3)} ~ {formatNumber(level.band_high, 3)}</div>
                            <div>距当前价：{formatDistancePct(level.distance_pct)}</div>
                            <div>强度：{level.strength || '--'}（{formatNumber(level.score, 1)}）</div>
                            <div>历史触达次数：{level.touch_count ?? '--'}</div>
                            <div>来源：{formatSupportSources(level.sources)}</div>
                            <div>最近验证：{level.last_validated_at || '--'}</div>
                          </div>
                        </div>
                      )
                    })
                  )}
                </div>
              </div>
            )}
          </section>

          <section className="rounded-2xl border border-border bg-card p-5">
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div>
                <h3 className="text-base font-semibold text-white">压力位（近{SUPPORT_LOOKBACK_DAYS}天）</h3>
                <p className="mt-1 text-xs text-white/60">
                  基于最近 {SUPPORT_LOOKBACK_DAYS} 个交易日，综合价格形态计算出的压力参考区间。
                </p>
              </div>
              <div className="text-xs text-white/55">
                {resistancePayload?.meta?.updated_at ? `更新时间：${formatDateTime(resistancePayload.meta.updated_at)}` : '等待数据'}
              </div>
            </div>

            {resistanceError ? (
              <div className="mt-3 rounded-xl border border-amber-400/30 bg-amber-500/10 px-4 py-3 text-sm text-amber-200">{resistanceError}</div>
            ) : null}

            {!resistanceSummary ? (
              <div className="mt-3 rounded-xl border border-dashed border-border px-4 py-6 text-sm text-white/50">
                暂无可用压力位数据（可能仍在预热或样本不足）。
              </div>
            ) : (
              <div className="mt-4 space-y-4">
                <div className="grid gap-3 md:grid-cols-4">
                  <MetricMini
                    label="最近压力位"
                    value={resistanceSummary.nearest_price ? formatNumber(resistanceSummary.nearest_price, 3) : '--'}
                    accent={resistanceStatusAccent}
                    emphasis
                  />
                  <MetricMini
                    label="距最近压力位"
                    value={formatDistancePct(resistanceSummary.distance_pct)}
                    accent={resistanceSummary.distance_pct >= 0 ? 'normal' : 'up'}
                  />
                  <MetricMini
                    label="压力强度"
                    value={resistanceSummary.strength || '--'}
                    accent={resistanceSummary.strength === '强' ? 'down' : resistanceSummary.strength === '弱' ? 'up' : 'normal'}
                  />
                  <MetricMini
                    label="压力状态"
                    value={formatResistanceStatus(resistanceSummary.status)}
                    accent={resistanceStatusAccent}
                    emphasis
                  />
                </div>

                <div className="rounded-xl border border-border bg-black/20 p-3">
                  <div className="text-xs text-white/55">字段说明</div>
                  <ul className="mt-2 space-y-1 text-xs text-white/70">
                    <li>• 压力位：历史上价格多次受阻回落的参考价位（区间），用于判断上方抛压，不代表一定下跌。</li>
                    <li>• 距最近压力位：当前价与最近压力位的百分比距离（正数=当前价在压力位下方）。</li>
                    <li>• 压力强度：综合历史触达次数、最近验证时间、回落幅度得到的分级。</li>
                    <li>• 压力状态：接近压力区 / 回踩压力区 / 位于压力区下方 / 突破压力区。</li>
                  </ul>
                </div>

                <div className="space-y-2">
                  {resistanceLevels.length === 0 ? (
                    <div className="rounded-xl border border-dashed border-border px-3 py-4 text-center text-xs text-white/50">暂无压力位明细</div>
                  ) : (
                    resistanceLevels.map((level, index) => {
                      const levelLabel = formatResistanceLevelLabel(level.level, index)
                      return (
                        <div key={level.level} className="rounded-xl border border-border bg-black/20 px-3 py-3">
                          <div className="flex flex-wrap items-center justify-between gap-2">
                            <div className="text-sm font-semibold text-white">{levelLabel} · {formatNumber(level.price, 3)}</div>
                            <div className="text-xs text-white/60">{formatResistanceStatus(level.status)}</div>
                          </div>
                          <div className="mt-2 grid gap-2 text-xs text-white/70 md:grid-cols-2 xl:grid-cols-4">
                            <div>压力区间：{formatNumber(level.band_low, 3)} ~ {formatNumber(level.band_high, 3)}</div>
                            <div>距当前价：{formatDistancePct(level.distance_pct)}</div>
                            <div>强度：{level.strength || '--'}（{formatNumber(level.score, 1)}）</div>
                            <div>历史触达次数：{level.touch_count ?? '--'}</div>
                            <div>来源：{formatSupportSources(level.sources)}</div>
                            <div>最近验证：{level.last_validated_at || '--'}</div>
                          </div>
                        </div>
                      )
                    })
                  )}
                </div>
              </div>
            )}
          </section>

              <section className="grid gap-4 xl:grid-cols-2">
                <EventPanel title="量价异动" events={priceVolumeEvents} renderEvent={(item) => (
                  <>
                    <div className="font-medium text-white">{item.anomaly_type}</div>
                    <div className="text-xs text-white/55">评分：{formatNumber(item.score, 1)} · {formatDateTime(item.detected_at)}</div>
                  </>
                )} />

                <EventPanel title="大单流向" events={blockFlowEvents} renderEvent={(item) => (
                  <>
                    <div className="font-medium text-white">净流向：{formatCompact(item.net_inflow)}</div>
                    <div className="text-xs text-white/55">
                      强度 {formatPercent(item.direction_strength)} · 连续性 {formatPercent(item.continuity)} · {formatDateTime(item.detected_at)}
                    </div>
                  </>
                )} />
              </section>
            </>
          ) : null}
        </div>
      </section>
    </div>
  )
}

function OverlayIntradayChart({ series, benchmark, symbol }) {
  const containerRef = useRef(null)
  const chartRef = useRef(null)

  useEffect(() => {
    let cleanup = () => {}
    let cancelled = false

    const renderChart = async () => {
      if (!containerRef.current || !Array.isArray(series) || series.length === 0) {
        if (chartRef.current) {
          chartRef.current.remove()
          chartRef.current = null
        }
        return
      }

      const { createChart, ColorType } = await import('lightweight-charts')
      if (cancelled || !containerRef.current) return

      if (chartRef.current) {
        chartRef.current.remove()
        chartRef.current = null
      }

      const chart = createChart(containerRef.current, {
        width: containerRef.current.clientWidth || 700,
        height: 280,
        layout: {
          background: { type: ColorType.Solid, color: 'rgba(9, 13, 24, 0.6)' },
          textColor: '#E5E7EB',
        },
        rightPriceScale: { borderColor: 'rgba(148,163,184,0.35)' },
        timeScale: {
          borderColor: 'rgba(148,163,184,0.35)',
          timeVisible: true,
          secondsVisible: false,
        },
        grid: {
          vertLines: { color: 'rgba(148,163,184,0.1)' },
          horzLines: { color: 'rgba(148,163,184,0.1)' },
        },
      })

      const stockLine = chart.addLineSeries({
        color: '#f59e0b',
        lineWidth: 2,
        title: `${symbol}（归一化）`,
      })
      const benchmarkLine = chart.addLineSeries({
        color: '#38bdf8',
        lineWidth: 2,
        title: `${benchmark || 'HSI'}（归一化）`,
      })

      const stockData = toAscendingSeriesData(series, 'stock_norm')
      const benchmarkData = toAscendingSeriesData(series, 'benchmark_norm')

      stockLine.setData(stockData)
      benchmarkLine.setData(benchmarkData)
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
        if (chartRef.current) {
          chartRef.current.remove()
          chartRef.current = null
        }
      }
    }

    renderChart()
    return () => {
      cancelled = true
      cleanup()
    }
  }, [benchmark, series, symbol])

  return <div ref={containerRef} className="w-full overflow-hidden rounded-xl border border-border bg-black/20" />
}

function toAscendingSeriesData(series, valueField) {
  if (!Array.isArray(series) || series.length === 0) return []

  const valueByTime = new Map()
  for (const item of series) {
    const timestamp = Math.floor(new Date(item.ts).getTime() / 1000)
    const value = Number(item?.[valueField])
    if (!timestamp || Number.isNaN(timestamp) || Number.isNaN(value)) continue
    valueByTime.set(timestamp, value)
  }

  return Array.from(valueByTime.entries())
    .sort((a, b) => a[0] - b[0])
    .map(([time, value]) => ({ time, value }))
}

function EventPanel({ title, events, renderEvent }) {
  return (
    <section className="rounded-2xl border border-border bg-card p-5">
      <h3 className="text-base font-semibold text-white">{title}</h3>
      <div className="mt-3 space-y-2">
        {events.length === 0 ? (
          <div className="rounded-xl border border-dashed border-border px-4 py-5 text-sm text-white/50">暂无事件</div>
        ) : (
          events.map((item) => (
            <div key={item.event_id} className="rounded-xl border border-border bg-black/20 px-3 py-2">
              {renderEvent(item)}
            </div>
          ))
        )}
      </div>
    </section>
  )
}

function AccessPromptCard({ title, description, buttonLabel, onAction, disabled = false }) {
  return (
    <div className="rounded-2xl border border-dashed border-primary/30 bg-primary/10 p-5">
      <div className="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
        <div className="space-y-2">
          <div className="text-lg font-semibold text-white">{title}</div>
          <p className="max-w-2xl text-sm leading-7 text-white/65">{description}</p>
        </div>
        <button
          type="button"
          disabled={disabled || typeof onAction !== 'function'}
          onClick={onAction}
          className="inline-flex shrink-0 items-center justify-center rounded-xl bg-primary px-4 py-2 text-sm font-semibold text-black transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
        >
          {buttonLabel}
        </button>
      </div>
    </div>
  )
}

function MetricCard({ label, value }) {
  return (
    <div className="rounded-xl border border-border bg-card px-4 py-3">
      <div className="text-xs text-white/55">{label}</div>
      <div className="mt-1 text-sm font-semibold text-white">{value}</div>
    </div>
  )
}

function MetricMini({ label, value, accent = 'normal', emphasis = false, featured = false, marketAccent = false }) {
  const risingColor = marketAccent ? 'text-rose-300' : 'text-emerald-300'
  const fallingColor = marketAccent ? 'text-emerald-300' : 'text-rose-300'
  const color = accent === 'up' ? risingColor : accent === 'down' ? fallingColor : 'text-white'
  const emphasisTone = accent === 'up'
    ? 'border-emerald-400/45 bg-emerald-500/10 ring-1 ring-emerald-300/20'
    : accent === 'down'
      ? 'border-rose-400/45 bg-rose-500/10 ring-1 ring-rose-300/20'
      : 'border-primary/45 bg-primary/10 ring-1 ring-primary/25'
  const featuredTone = accent === 'up'
    ? 'border-rose-400/50 bg-rose-500/12 ring-1 ring-rose-300/25 shadow-[0_10px_30px_rgba(251,113,133,0.18)]'
    : accent === 'down'
      ? 'border-emerald-400/50 bg-emerald-500/12 ring-1 ring-emerald-300/25 shadow-[0_10px_30px_rgba(52,211,153,0.18)]'
      : 'border-primary/55 bg-primary/12 ring-1 ring-primary/30 shadow-[0_10px_30px_rgba(76,106,255,0.16)]'
  const containerTone = featured
    ? marketAccent
      ? featuredTone
      : 'border-primary/55 bg-primary/12 ring-1 ring-primary/30 shadow-[0_10px_30px_rgba(76,106,255,0.16)]'
    : emphasis
      ? emphasisTone
      : 'border-border bg-black/20'
  const featuredLabelColor = marketAccent
    ? accent === 'up'
      ? 'text-rose-200/90'
      : accent === 'down'
        ? 'text-emerald-200/90'
        : 'text-primary/85'
    : 'text-primary/85'

  return (
    <div className={`rounded-xl border px-3 py-2 ${featured ? 'px-4 py-3' : ''} ${containerTone}`}>
      <div className={`text-xs ${featured ? featuredLabelColor : 'text-white/50'}`}>{label}</div>
      <div className={`mt-1 font-semibold ${color} ${featured ? 'text-2xl leading-none tracking-tight' : 'text-sm'}`}>{value}</div>
    </div>
  )
}

function sessionStateLabel(state) {
  const labels = {
    idle: '空闲',
    warming_up: '预热中',
    running: '运行中',
    degraded: '降级',
    stopped: '已停止',
  }
  return labels[state] || state
}

function formatSource(source) {
  const normalized = String(source || '').toLowerCase()
  if (normalized === 'tencent-qt') {
    return '腾讯行情（qt.gtimg.cn）'
  }
  return source || '腾讯行情（qt.gtimg.cn）'
}

function formatMarketIndexTitle(name, code) {
  const rawName = String(name || '').trim()
  const upperCode = String(code || '').trim().toUpperCase()

  const nameMap = {
    'Hang Seng Index': '恒生指数',
    'Hang Seng China Enterprises Index': '恒生中国企业指数',
    'Hang Seng TECH Index': '恒生科技指数',
  }

  if (nameMap[rawName]) {
    return nameMap[rawName]
  }

  const codeMap = {
    HSI: '恒生指数',
    HSCEI: '恒生中国企业指数',
    HSTECH: '恒生科技指数',
    '000001': '上证指数',
    '399001': '深证成指',
    '399006': '创业板指',
  }

  return codeMap[upperCode] || rawName || upperCode || '--'
}

function formatDistancePct(value) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return '--'
  const num = Number(value)
  const sign = num > 0 ? '+' : ''
  return `${sign}${num.toFixed(2)}%`
}

function formatSupportStatus(status) {
  const normalized = String(status || '').trim()
  const statusMap = {
    临近支撑: '接近支撑区',
    回踩支撑: '回踩支撑区',
    位于支撑上方: '高于支撑区',
    跌破支撑: '跌破支撑区',
  }
  return statusMap[normalized] || normalized || '--'
}

function formatSupportLevelLabel(level, index = 0) {
  const normalized = String(level || '').trim().toUpperCase()
  const labelMap = {
    S1: '最近支撑位',
    S2: '第二支撑位',
    S3: '第三支撑位',
  }

  if (labelMap[normalized]) {
    return labelMap[normalized]
  }

  return index === 0 ? '最近支撑位' : `第${index + 1}支撑位`
}

function formatResistanceStatus(status) {
  const normalized = String(status || '').trim()
  const statusMap = {
    临近压力: '接近压力区',
    回踩压力: '回踩压力区',
    位于压力下方: '位于压力区下方',
    突破压力: '突破压力区',
  }
  return statusMap[normalized] || normalized || '--'
}

function formatResistanceLevelLabel(level, index = 0) {
  const normalized = String(level || '').trim().toUpperCase()
  const labelMap = {
    R1: '最近压力位',
    R2: '第二压力位',
    R3: '第三压力位',
  }

  if (labelMap[normalized]) {
    return labelMap[normalized]
  }

  return index === 0 ? '最近压力位' : `第${index + 1}压力位`
}

function formatMAStatus(status) {
  const normalized = String(status || '').trim()
  const statusMap = {
    双双站上: '价格高于 MA20 / MA200',
    双双跌破: '价格低于 MA20 / MA200',
    '站上MA20但低于MA200': '短强长弱（上 MA20 下 MA200）',
    '跌破MA20但高于MA200': '短弱长强（下 MA20 上 MA200）',
  }
  return statusMap[normalized] || normalized || '--'
}

function formatStrategyCycleHint(strategy) {
  if (!strategy) return '未选择策略，无法判断策略周期'

  const schemaItems = Array.isArray(strategy.param_schema) ? strategy.param_schema : []
  const defaultParams = strategy.default_params && typeof strategy.default_params === 'object'
    ? strategy.default_params
    : {}
  const cycleItems = schemaItems.filter((item) => {
    const key = String(item?.key || '').toLowerCase()
    const label = String(item?.label || '')
    return /周期|窗口|回看/.test(label) || /period|window|lookback/.test(key)
  })

  if (cycleItems.length === 0) {
    return '策略未声明固定周期参数（由策略实现实时判定）'
  }

  return cycleItems.map((item) => {
    const key = String(item?.key || '').trim()
    const label = String(item?.label || key || '参数').trim()
    const value = defaultParams[key] ?? item?.default
    return `${label}=${value ?? '--'}`
  }).join('，')
}

function buildSignalPayloadTemplate(symbol, strategyID) {
  const lines = [
    '股票交易信号来啦！',
    '类型：正式信号',
    `股票：${symbol || '--'}`,
    '方向：BUY',
    '时间：2026-03-19 18:00:00',
  ]

  if (strategyID) {
    lines.push(`策略：${strategyID}`)
  }
  lines.push('原因：策略触发原因说明')

  return {
    msgtype: 'text',
    text: {
      content: lines.join('\n'),
    },
  }
}


function formatSupportSources(sources) {
  if (!Array.isArray(sources) || sources.length === 0) return '--'
  const map = {
    swing: 'Swing',
    pivot: 'Pivot',
    ma60: 'MA60',
    ma120: 'MA120',
  }
  return sources.map((item) => map[item] || String(item || '').toUpperCase()).join(' + ')
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
  return Number(value).toLocaleString('zh-CN', { maximumFractionDigits: digits, minimumFractionDigits: digits })
}

function formatCompact(value) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return '--'
  return Number(value).toLocaleString('zh-CN', { maximumFractionDigits: 2 })
}

function formatDateTime(value) {
  if (!value) return '--'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString('zh-CN', { hour12: false })
}

function detectExchange(symbol) {
  if (!symbol) return 'HKEX'
  const upper = String(symbol).toUpperCase()
  if (upper.endsWith('.SH')) return 'SSE'
  if (upper.endsWith('.SZ')) return 'SZSE'
  if (upper.endsWith('.HK')) return 'HKEX'
  // Bare digits: 6-digit starting with 6 → SSE, 0/3 → SZSE, else HK
  const digits = upper.replace(/\D/g, '')
  if (digits.length === 6) {
    if (digits[0] === '6') return 'SSE'
    if (digits[0] === '0' || digits[0] === '3') return 'SZSE'
  }
  return 'HKEX'
}
