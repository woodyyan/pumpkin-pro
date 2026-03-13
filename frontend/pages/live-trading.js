import { useEffect, useMemo, useRef, useState } from 'react'
import { requestJson } from '../lib/api'

const POLL_MS = 2000
const OVERLAY_WINDOW_MINUTES = 60

export default function LiveTradingPage() {
  const [watchlist, setWatchlist] = useState({ items: [], active_symbol: '', session_state: 'idle' })
  const [marketOverview, setMarketOverview] = useState(null)
  const [snapshotPayload, setSnapshotPayload] = useState(null)
  const [overlayPayload, setOverlayPayload] = useState(null)
  const [priceVolumeEvents, setPriceVolumeEvents] = useState([])
  const [blockFlowEvents, setBlockFlowEvents] = useState([])
  const [symbolInput, setSymbolInput] = useState('00700.HK')
  const [nameInput, setNameInput] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')
  const [lastUpdateAt, setLastUpdateAt] = useState('')

  const activeSymbol = watchlist.active_symbol
  const sessionState = watchlist.session_state || 'idle'

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

  const loadSymbolPanels = async (symbol) => {
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
  }

  const runPolling = async () => {
    try {
      setError('')
      const watchState = await loadWatchlist()
      await loadMarketOverview()
      if (watchState.active_symbol) {
        await loadSymbolPanels(watchState.active_symbol)
      }
    } catch (err) {
      setError(err.message || '实时数据刷新失败')
    }
  }

  useEffect(() => {
    runPolling()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    const timer = setInterval(async () => {
      try {
        setError('')
        await loadMarketOverview()
        if (activeSymbol) {
          await loadSymbolPanels(activeSymbol)
        }
      } catch (err) {
        setError(err.message || '实时数据刷新失败')
      }
    }, POLL_MS)

    return () => clearInterval(timer)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeSymbol])

  const handleAddWatch = async (event) => {
    event.preventDefault()
    setSubmitting(true)
    setError('')
    try {
      await requestJson('/api/live/watchlist', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ symbol: symbolInput, name: nameInput }),
      })
      setNameInput('')
      const nextWatchlist = await loadWatchlist()
      if (nextWatchlist.active_symbol) {
        await loadSymbolPanels(nextWatchlist.active_symbol)
      }
    } catch (err) {
      setError(err.message || '添加关注失败')
    } finally {
      setSubmitting(false)
    }
  }

  const handleActivate = async (symbol) => {
    setError('')
    try {
      await requestJson(`/api/live/watchlist/${encodeURIComponent(symbol)}/activate`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ reset_window: true }),
      })
      await loadWatchlist()
      await loadSymbolPanels(symbol)
    } catch (err) {
      setError(err.message || '切换激活标的失败')
    }
  }

  const handleDelete = async (symbol) => {
    setError('')
    try {
      await requestJson(`/api/live/watchlist/${encodeURIComponent(symbol)}`, { method: 'DELETE' })
      const nextWatchlist = await loadWatchlist()
      if (!nextWatchlist.active_symbol) {
        setSnapshotPayload(null)
        setOverlayPayload(null)
        setPriceVolumeEvents([])
        setBlockFlowEvents([])
      } else {
        await loadSymbolPanels(nextWatchlist.active_symbol)
      }
    } catch (err) {
      setError(err.message || '删除关注失败')
    }
  }

  return (
    <div className="space-y-6">
      <section className="rounded-2xl border border-border bg-card p-6">
        <h1 className="text-2xl font-semibold tracking-tight">实盘监控（阶段 A）</h1>
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
          <div className="grid gap-4 md:grid-cols-3">
            <MetricCard label="会话状态" value={sessionStateLabel(sessionState)} />
            <MetricCard label="激活标的" value={activeSymbol || '未设置'} />
            <MetricCard label="最后刷新" value={lastUpdateAt ? formatDateTime(lastUpdateAt) : '--'} />
          </div>

          {error && <div className="rounded-xl border border-rose-400/40 bg-rose-500/10 px-4 py-3 text-sm text-rose-200">{error}</div>}

          <section className="rounded-2xl border border-border bg-card p-5">
            <h3 className="text-base font-semibold text-white">港股大盘概览</h3>
            <div className="mt-4 grid gap-3 md:grid-cols-3">
              {(marketOverview?.indexes || []).map((index) => (
                <div key={index.code} className="rounded-xl border border-border bg-black/20 p-3">
                  <div className="text-xs text-white/50">{index.name || index.code}</div>
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

          <section className="grid gap-4 xl:grid-cols-2">
            <EventPanel title="量价异动（A3）" events={priceVolumeEvents} renderEvent={(item) => (
              <>
                <div className="font-medium text-white">{item.anomaly_type}</div>
                <div className="text-xs text-white/55">评分：{formatNumber(item.score, 1)} · {formatDateTime(item.detected_at)}</div>
              </>
            )} />

            <EventPanel title="大单流向（A4）" events={blockFlowEvents} renderEvent={(item) => (
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

      const stockData = []
      const benchmarkData = []
      for (const item of series) {
        const timestamp = Math.floor(new Date(item.ts).getTime() / 1000)
        if (!timestamp || Number.isNaN(timestamp)) continue
        if (item.stock_norm != null && !Number.isNaN(Number(item.stock_norm))) {
          stockData.push({ time: timestamp, value: Number(item.stock_norm) })
        }
        if (item.benchmark_norm != null && !Number.isNaN(Number(item.benchmark_norm))) {
          benchmarkData.push({ time: timestamp, value: Number(item.benchmark_norm) })
        }
      }

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

function MetricMini({ label, value, accent = 'normal' }) {
  const color = accent === 'up' ? 'text-emerald-300' : accent === 'down' ? 'text-rose-300' : 'text-white'
  return (
    <div className="rounded-xl border border-border bg-black/20 px-3 py-2">
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
