import { useEffect, useMemo, useRef, useState } from 'react'

import { requestJson } from '../lib/api'
import { useAuth } from '../lib/auth-context'
import { isAuthRequiredError } from '../lib/auth-storage'

const POLL_MS = 2000
const OVERLAY_WINDOW_MINUTES = 60
const SUPPORT_REFRESH_MS = 60 * 1000
const SUPPORT_LOOKBACK_DAYS = 120

export default function LiveTradingPage() {
  const { openAuthModal } = useAuth()
  const [watchlist, setWatchlist] = useState({ items: [], active_symbol: '', session_state: 'idle' })
  const [marketOverview, setMarketOverview] = useState(null)
  const [snapshotPayload, setSnapshotPayload] = useState(null)
  const [overlayPayload, setOverlayPayload] = useState(null)
  const [supportPayload, setSupportPayload] = useState(null)
  const [supportError, setSupportError] = useState('')
  const [priceVolumeEvents, setPriceVolumeEvents] = useState([])
  const [blockFlowEvents, setBlockFlowEvents] = useState([])
  const [symbolInput, setSymbolInput] = useState('00700.HK')
  const [nameInput, setNameInput] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')
  const [errorNeedsLogin, setErrorNeedsLogin] = useState(false)
  const [lastUpdateAt, setLastUpdateAt] = useState('')
  const supportRefreshRef = useRef({ symbol: '', refreshedAt: 0 })

  const activeSymbol = watchlist.active_symbol
  const sessionState = watchlist.session_state || 'idle'
  const supportSummary = supportPayload?.summary || null
  const supportLevels = Array.isArray(supportPayload?.levels) ? supportPayload.levels : []
  const supportStatusAccent = supportSummary?.status === '跌破支撑'
    ? 'down'
    : supportSummary?.status === '临近支撑' || supportSummary?.status === '回踩支撑'
      ? 'up'
      : 'normal'

  const updateError = (nextError, nextNeedsLogin = false) => {
    setError(nextError)
    setErrorNeedsLogin(nextNeedsLogin)
  }

  const applyRequestError = (err, fallbackText) => {
    updateError(err.message || fallbackText, isAuthRequiredError(err))
  }

  const sortedWatchlist = useMemo(() => {
    return [...(watchlist.items || [])].sort((a, b) => Number(b.is_active) - Number(a.is_active))
  }, [watchlist.items])

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
    const data = await requestJson('/api/live/market/overview')
    setMarketOverview(data)
  }

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

    await loadSupportLevels(symbol, { force: forceSupport })
  }

  const runPolling = async () => {
    try {
      updateError('')
      const watchState = await loadWatchlist()
      await loadMarketOverview()
      if (watchState.active_symbol) {
        await loadSymbolPanels(watchState.active_symbol, { forceSupport: true })
      } else {
        setSupportPayload(null)
        setSupportError('')
        supportRefreshRef.current = { symbol: '', refreshedAt: 0 }
      }
    } catch (err) {
      applyRequestError(err, '实时数据刷新失败')
    }
  }

  useEffect(() => {
    runPolling()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    const timer = setInterval(async () => {
      try {
        updateError('')
        await loadMarketOverview()
        if (activeSymbol) {
          await loadSymbolPanels(activeSymbol)
        }
      } catch (err) {
        applyRequestError(err, '实时数据刷新失败')
      }
    }, POLL_MS)

    return () => clearInterval(timer)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeSymbol])

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
      if (!nextWatchlist.active_symbol) {
        setSnapshotPayload(null)
        setOverlayPayload(null)
        setSupportPayload(null)
        setSupportError('')
        setPriceVolumeEvents([])
        setBlockFlowEvents([])
        supportRefreshRef.current = { symbol: '', refreshedAt: 0 }
      } else {
        await loadSymbolPanels(nextWatchlist.active_symbol, { forceSupport: true })
      }
    } catch (err) {
      applyRequestError(err, '删除关注失败')
    }
  }

  return (
    <div className="space-y-6">
      <section className="rounded-2xl border border-border bg-card p-6">
        <h1 className="text-2xl font-semibold tracking-tight">实盘监控</h1>
        <p className="mt-3 text-sm leading-7 text-white/65">
          当前仅提供实时监控与异动捕获，不触发任何下单行为。系统采用“关注池 + 激活标的”模型：可维护多只关注股票，但同一时刻只监控 1 只激活标的。
        </p>
      </section>

      <section className="grid gap-6 lg:grid-cols-[320px_1fr]">
        <div className="space-y-4 rounded-2xl border border-border bg-card p-5">
          <div>
            <h2 className="text-lg font-semibold text-white">关注股票池</h2>
            <p className="mt-1 text-xs text-white/50">仅港股代码（如 00700.HK）</p>
          </div>

          <form onSubmit={handleAddWatch} className="space-y-3">
            <input
              value={symbolInput}
              onChange={(event) => setSymbolInput(event.target.value.toUpperCase())}
              placeholder="00700.HK"
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
        </div>

        <div className="space-y-4">
          <div className="grid gap-4 md:grid-cols-4">
            <MetricCard label="会话状态" value={sessionStateLabel(sessionState)} />
            <MetricCard label="激活标的" value={activeSymbol || '未设置'} />
            <MetricCard label="最后刷新" value={lastUpdateAt ? formatDateTime(lastUpdateAt) : '--'} />
            <MetricCard label="行情来源" value={formatSource(snapshotPayload?.snapshot?.source)} />
          </div>

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

          <section className="rounded-2xl border border-border bg-card p-5">
            <h3 className="text-base font-semibold text-white">港股大盘概览</h3>
            <div className="mt-4 grid gap-3 md:grid-cols-3">
              {(marketOverview?.indexes || []).map((index) => (
                <div key={index.code} className="rounded-xl border border-border bg-black/20 p-3">
                  <div className="text-xs text-white/50">{formatMarketIndexTitle(index.name, index.code)}</div>
                  <div className="mt-1 text-lg font-semibold text-white">{formatNumber(index.last, 2)}</div>
                  <div className={`text-xs ${index.change_rate >= 0 ? 'text-emerald-300' : 'text-rose-300'}`}>
                    {formatPercent(index.change_rate)}
                  </div>
                </div>
              ))}
            </div>
          </section>

          <section className="rounded-2xl border border-border bg-card p-5">
            <h3 className="text-base font-semibold text-white">激活标的快照</h3>
            {!snapshotPayload?.snapshot ? (
              <div className="mt-3 rounded-xl border border-dashed border-border px-4 py-6 text-sm text-white/50">请先在左侧选择一个激活标的。</div>
            ) : (
              <div className="mt-4 grid gap-3 md:grid-cols-3">
                <MetricMini label="最新价" value={formatNumber(snapshotPayload.snapshot.last_price, 3)} />
                <MetricMini label="涨跌幅" value={formatPercent(snapshotPayload.snapshot.change_rate)} accent={snapshotPayload.snapshot.change_rate >= 0 ? 'up' : 'down'} />
                <MetricMini label="量比" value={formatNumber(snapshotPayload.snapshot.volume_ratio, 2)} />
                <MetricMini label="成交量" value={formatCompact(snapshotPayload.snapshot.volume)} />
                <MetricMini label="成交额(HKD)" value={formatCompact(snapshotPayload.snapshot.turnover)} />
                <MetricMini label="振幅" value={formatPercent(snapshotPayload.snapshot.amplitude)} />
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

function MetricCard({ label, value }) {
  return (
    <div className="rounded-xl border border-border bg-card px-4 py-3">
      <div className="text-xs text-white/55">{label}</div>
      <div className="mt-1 text-sm font-semibold text-white">{value}</div>
    </div>
  )
}

function MetricMini({ label, value, accent = 'normal', emphasis = false }) {
  const color = accent === 'up' ? 'text-emerald-300' : accent === 'down' ? 'text-rose-300' : 'text-white'
  const emphasisTone = accent === 'up'
    ? 'border-emerald-400/45 bg-emerald-500/10 ring-1 ring-emerald-300/20'
    : accent === 'down'
      ? 'border-rose-400/45 bg-rose-500/10 ring-1 ring-rose-300/20'
      : 'border-primary/45 bg-primary/10 ring-1 ring-primary/25'

  return (
    <div className={`rounded-xl border px-3 py-2 ${emphasis ? emphasisTone : 'border-border bg-black/20'}`}>
      <div className="text-xs text-white/50">{label}</div>
      <div className={`mt-1 text-sm font-semibold ${color}`}>{value}</div>
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
