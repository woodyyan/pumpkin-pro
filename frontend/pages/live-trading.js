import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import { requestJson } from '../lib/api'
import { useAuth } from '../lib/auth-context'
import { isAuthRequiredError } from '../lib/auth-storage'

const POLL_MS = 5000
const MARKET_OVERVIEW_POLL_MS = 5000

export default function LiveTradingOverviewPage() {
  const { isLoggedIn, openAuthModal, ready, user } = useAuth()
  const [watchlist, setWatchlist] = useState({ items: [], active_symbol: '', session_state: 'idle' })
  const [snapshots, setSnapshots] = useState([])
  const [marketOverviewA, setMarketOverviewA] = useState(null)
  const [marketOverviewHK, setMarketOverviewHK] = useState(null)
  const [symbolInput, setSymbolInput] = useState('')
  const [nameInput, setNameInput] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')
  const [errorNeedsLogin, setErrorNeedsLogin] = useState(false)
  const [lastUpdateAt, setLastUpdateAt] = useState('')

  const privateAccessReady = ready && isLoggedIn

  const resetPrivateState = useCallback(() => {
    setWatchlist({ items: [], active_symbol: '', session_state: 'idle' })
    setSnapshots([])
    setError('')
    setErrorNeedsLogin(false)
    setLastUpdateAt('')
  }, [])

  const updateError = (nextError, nextNeedsLogin = false) => {
    setError(nextError)
    setErrorNeedsLogin(nextNeedsLogin)
  }

  const applyRequestError = (err, fallbackText) => {
    updateError(err.message || fallbackText, isAuthRequiredError(err))
  }

  // Build snapshot lookup by symbol
  const snapshotBySymbol = useMemo(() => {
    const map = {}
    snapshots.forEach((s) => {
      if (s?.symbol) map[s.symbol] = s
    })
    return map
  }, [snapshots])

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

  const loadSnapshots = async () => {
    const data = await requestJson('/api/live/watchlist/snapshots')
    const items = Array.isArray(data?.items) ? data.items : []
    setSnapshots(items)
    setLastUpdateAt(new Date().toISOString())
    return items
  }

  const loadMarketOverview = async () => {
    const [aRes, hkRes] = await Promise.allSettled([
      requestJson('/api/live/market/overview?exchange=SSE'),
      requestJson('/api/live/market/overview'),
    ])
    if (aRes.status === 'fulfilled') setMarketOverviewA(aRes.value)
    if (hkRes.status === 'fulfilled') setMarketOverviewHK(hkRes.value)
  }

  const loadPrivateData = async ({ bootstrap = false } = {}) => {
    try {
      if (bootstrap) {
        await loadWatchlist()
      }
      await loadSnapshots()
      updateError('')
    } catch (err) {
      applyRequestError(err, '实时数据刷新失败')
    }
  }

  const loadPublicData = async () => {
    try {
      await loadMarketOverview()
    } catch (err) {
      // Market overview failure is non-critical
    }
  }

  // Bootstrap
  useEffect(() => {
    if (!ready) return
    loadPublicData()
    if (privateAccessReady) {
      loadPrivateData({ bootstrap: true })
    } else {
      resetPrivateState()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ready, privateAccessReady])

  // Polling
  useEffect(() => {
    if (!ready) return
    const timer = setInterval(() => {
      loadPublicData()
      if (privateAccessReady) {
        loadPrivateData()
      }
    }, POLL_MS)
    return () => clearInterval(timer)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ready, privateAccessReady])

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
      setSymbolInput('')
      setNameInput('')
      await loadWatchlist()
      await loadSnapshots()
    } catch (err) {
      applyRequestError(err, '添加关注失败')
    } finally {
      setSubmitting(false)
    }
  }

  const handleDelete = async (symbol) => {
    updateError('')
    try {
      await requestJson(`/api/live/watchlist/${encodeURIComponent(symbol)}`, { method: 'DELETE' })
      await loadWatchlist()
      await loadSnapshots()
    } catch (err) {
      applyRequestError(err, '删除关注失败')
    }
  }

  const handleOpenDetail = (symbol) => {
    window.open(`/live-trading/${encodeURIComponent(symbol)}`, '_blank')
  }

  const sortedWatchlist = useMemo(() => {
    return [...(watchlist.items || [])].sort((a, b) => Number(b.is_active) - Number(a.is_active))
  }, [watchlist.items])

  return (
    <div className="space-y-6">
      <section className="rounded-2xl border border-border bg-card p-6">
        <h1 className="text-2xl font-semibold tracking-tight">实盘监控</h1>
        <p className="mt-2 text-sm leading-7 text-white/60">
          关注池股票概览，点击卡片可在新标签页打开独立的实时详情页。
        </p>
      </section>

      {/* Market overview — A shares + HK side by side */}
      <section className="grid gap-4 md:grid-cols-2">
        <div className="rounded-2xl border border-border bg-card p-5">
          <h3 className="text-base font-semibold text-white">A 股大盘</h3>
          <div className="mt-4 grid gap-3">
            {(marketOverviewA?.indexes || []).length > 0 ? (
              marketOverviewA.indexes.map((index) => (
                <div key={index.code} className="rounded-xl border border-border bg-black/20 p-3">
                  <div className="text-xs text-white/50">{formatMarketIndexTitle(index.name, index.code)}</div>
                  <div className="mt-1 text-lg font-semibold text-white">{formatNumber(index.last, 2)}</div>
                  <div className={`text-xs ${index.change_rate >= 0 ? 'text-rose-300' : 'text-emerald-300'}`}>
                    {formatPercent(index.change_rate)}
                  </div>
                </div>
              ))
            ) : (
              <div className="text-xs text-white/40">加载中...</div>
            )}
          </div>
        </div>
        <div className="rounded-2xl border border-border bg-card p-5">
          <h3 className="text-base font-semibold text-white">港股大盘</h3>
          <div className="mt-4 grid gap-3">
            {(marketOverviewHK?.indexes || []).length > 0 ? (
              marketOverviewHK.indexes.map((index) => (
                <div key={index.code} className="rounded-xl border border-border bg-black/20 p-3">
                  <div className="text-xs text-white/50">{formatMarketIndexTitle(index.name, index.code)}</div>
                  <div className="mt-1 text-lg font-semibold text-white">{formatNumber(index.last, 2)}</div>
                  <div className={`text-xs ${index.change_rate >= 0 ? 'text-rose-300' : 'text-emerald-300'}`}>
                    {formatPercent(index.change_rate)}
                  </div>
                </div>
              ))
            ) : (
              <div className="text-xs text-white/40">加载中...</div>
            )}
          </div>
        </div>
      </section>

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
        <>
          {/* Add stock form */}
          <section className="rounded-2xl border border-border bg-card p-5">
            <h3 className="text-base font-semibold text-white">添加关注股票</h3>
            <form onSubmit={handleAddWatch} className="mt-3 flex flex-wrap items-end gap-3">
              <input
                value={symbolInput}
                onChange={(e) => setSymbolInput(e.target.value.toUpperCase())}
                placeholder="股票代码，如 00700 或 600519"
                className="w-48 rounded-xl border border-border bg-black/20 px-3 py-2 text-sm text-white outline-none transition focus:border-primary"
              />
              <input
                value={nameInput}
                onChange={(e) => setNameInput(e.target.value)}
                placeholder="备注名称（可选）"
                className="w-40 rounded-xl border border-border bg-black/20 px-3 py-2 text-sm text-white outline-none transition focus:border-primary"
              />
              <button
                type="submit"
                disabled={submitting || !symbolInput.trim()}
                className="rounded-xl bg-primary px-4 py-2 text-sm font-medium text-black transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
              >
                {submitting ? '添加中...' : '添加'}
              </button>
            </form>
          </section>

          {/* Stock cards grid */}
          {sortedWatchlist.length === 0 ? (
            <div className="rounded-2xl border border-dashed border-border bg-card px-6 py-12 text-center text-sm text-white/50">
              暂无关注股票，请先在上方添加。
            </div>
          ) : (
            <section className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
              {sortedWatchlist.map((item) => {
                const snap = snapshotBySymbol[item.symbol]
                const displayName = snap?.name && snap.name !== item.symbol
                  ? snap.name
                  : item.name && item.name !== item.symbol
                    ? item.name
                    : ''
                const changeRate = snap?.change_rate ?? null
                const isUp = changeRate !== null && changeRate > 0
                const isDown = changeRate !== null && changeRate < 0
                const borderAccent = isUp
                  ? 'border-rose-400/30 hover:border-rose-400/50'
                  : isDown
                    ? 'border-emerald-400/30 hover:border-emerald-400/50'
                    : 'border-border hover:border-primary/50'

                return (
                  <div
                    key={item.symbol}
                    className={`group cursor-pointer rounded-2xl border bg-card p-4 transition hover:shadow-lg ${borderAccent}`}
                    onClick={() => handleOpenDetail(item.symbol)}
                    role="button"
                    tabIndex={0}
                    onKeyDown={(e) => { if (e.key === 'Enter') handleOpenDetail(item.symbol) }}
                  >
                    {/* Header */}
                    <div className="flex items-start justify-between gap-2">
                      <div className="min-w-0 flex-1">
                        <div className="truncate text-sm font-semibold text-white">
                          {displayName ? `${displayName}` : item.symbol}
                        </div>
                        <div className="mt-0.5 text-xs text-white/45">
                          {displayName ? item.symbol : ''} · {detectExchangeLabel(item.symbol)}
                        </div>
                      </div>
                      {item.is_active && (
                        <span className="shrink-0 rounded-full bg-emerald-500/20 px-2 py-0.5 text-[10px] text-emerald-300">
                          激活
                        </span>
                      )}
                    </div>

                    {/* Price section */}
                    {snap ? (
                      <div className="mt-3">
                        <div className={`text-xl font-bold tracking-tight ${isUp ? 'text-rose-300' : isDown ? 'text-emerald-300' : 'text-white'}`}>
                          {formatNumber(snap.last_price, snap.last_price >= 100 ? 2 : 3)}
                        </div>
                        <div className="mt-1 flex items-center gap-3 text-xs">
                          <span className={isUp ? 'text-rose-300' : isDown ? 'text-emerald-300' : 'text-white/60'}>
                            {formatPercent(changeRate)}
                          </span>
                          {snap.volume_ratio > 0 && (
                            <span className="text-white/45">量比 {formatNumber(snap.volume_ratio, 2)}</span>
                          )}
                        </div>
                        <div className="mt-2 grid grid-cols-2 gap-x-4 gap-y-1 text-[11px] text-white/50">
                          <div>成交量 <span className="text-white/70">{formatCompact(snap.volume)}</span></div>
                          <div>成交额 <span className="text-white/70">{formatCompact(snap.turnover)}</span></div>
                          <div>振幅 <span className="text-white/70">{formatPercent(snap.amplitude)}</span></div>
                        </div>
                      </div>
                    ) : (
                      <div className="mt-3 text-xs text-white/40">加载中...</div>
                    )}

                    {/* Footer actions */}
                    <div className="mt-3 flex items-center justify-between border-t border-white/5 pt-3">
                      <span className="text-[11px] text-white/40 transition group-hover:text-primary">
                        点击查看详情 →
                      </span>
                      <button
                        type="button"
                        onClick={(e) => {
                          e.stopPropagation()
                          handleDelete(item.symbol)
                        }}
                        className="rounded-lg px-2 py-1 text-[11px] text-rose-300/60 transition hover:bg-rose-500/10 hover:text-rose-300"
                      >
                        删除
                      </button>
                    </div>
                  </div>
                )
              })}
            </section>
          )}

          {lastUpdateAt && (
            <div className="text-right text-xs text-white/35">
              最后更新：{formatDateTime(lastUpdateAt)}
            </div>
          )}
        </>
      ) : (
        <section className="rounded-2xl border border-dashed border-primary/30 bg-primary/10 p-6">
          <div className="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
            <div className="space-y-2">
              <div className="text-lg font-semibold text-white">
                {ready ? '登录后开启实盘监控' : '正在确认账号状态'}
              </div>
              <p className="max-w-2xl text-sm leading-7 text-white/65">
                {ready
                  ? '登录后可管理关注池、查看实时行情快照和独立股票详情页。'
                  : '正在检查你的登录状态，确认后会自动加载数据。'
                }
              </p>
            </div>
            <button
              type="button"
              disabled={!ready}
              onClick={ready ? () => openAuthModal('login', '登录后即可管理关注池与实盘监控。') : undefined}
              className="inline-flex shrink-0 items-center justify-center rounded-xl bg-primary px-4 py-2 text-sm font-semibold text-black transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
            >
              {ready ? '登录后继续' : '请稍候'}
            </button>
          </div>
        </section>
      )}
    </div>
  )
}

// ── Utility functions ──

function formatPercent(value) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return '--'
  const num = Number(value) * 100
  const sign = num > 0 ? '+' : ''
  return `${sign}${num.toFixed(2)}%`
}

function formatNumber(value, digits = 2) {
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
  const digits = upper.replace(/\D/g, '')
  if (digits.length === 6) {
    if (digits[0] === '6') return 'SSE'
    if (digits[0] === '0' || digits[0] === '3') return 'SZSE'
  }
  return 'HKEX'
}

function detectExchangeLabel(symbol) {
  const ex = detectExchange(symbol)
  const labels = { SSE: '沪市', SZSE: '深市', HKEX: '港股' }
  return labels[ex] || ex
}

function formatMarketIndexTitle(name, code) {
  const rawName = String(name || '').trim()
  const upperCode = String(code || '').trim().toUpperCase()
  const nameMap = {
    'Hang Seng Index': '恒生指数',
    'Hang Seng China Enterprises Index': '恒生中国企业指数',
    'Hang Seng TECH Index': '恒生科技指数',
  }
  if (nameMap[rawName]) return nameMap[rawName]
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
