import { useCallback, useEffect, useMemo, useState } from 'react'

import dynamic from 'next/dynamic'
import Head from 'next/head'
import { requestJson } from '../lib/api'
import { useAuth } from '../lib/auth-context'
import { isAuthRequiredError } from '../lib/auth-storage'

const RankingPortfolioPanel = dynamic(() => import('../components/RankingPortfolioPanel'), { ssr: false })

export default function LiveTradingOverviewPage() {
  const { isLoggedIn, openAuthModal, ready } = useAuth()
  const [watchlist, setWatchlist] = useState({ items: [], active_symbol: '', session_state: 'idle' })
  const [snapshots, setSnapshots] = useState([])
  const [marketOverviewA, setMarketOverviewA] = useState(null)
  const [marketOverviewHK, setMarketOverviewHK] = useState(null)
  const [symbolInput, setSymbolInput] = useState('')
  const [nameInput, setNameInput] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')
  const [errorNeedsLogin, setErrorNeedsLogin] = useState(false)
  const [signalConfigMap, setSignalConfigMap] = useState({})
  const [rankingPortfolioData, setRankingPortfolioData] = useState(null)
  const [rankingPortfolioLoading, setRankingPortfolioLoading] = useState(false)

  const privateAccessReady = ready && isLoggedIn

  const resetPrivateState = useCallback(() => {
    setWatchlist({ items: [], active_symbol: '', session_state: 'idle' })
    setSnapshots([])
    setSignalConfigMap({})
    setError('')
    setErrorNeedsLogin(false)
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
    return items
  }

  const loadSignalConfigs = async () => {
    try {
      const data = await requestJson('/api/signal-configs')
      const items = Array.isArray(data?.items) ? data.items : []
      const map = {}
      for (const cfg of items) {
        if (cfg?.symbol) map[cfg.symbol] = cfg
      }
      setSignalConfigMap(map)
    } catch {
      // Signal config loading is non-critical for overview
    }
  }

  const loadRankingPortfolio = async () => {
    try {
      setRankingPortfolioLoading(true)
      const data = await requestJson('/api/quadrant/ranking-portfolio')
      setRankingPortfolioData(data)
    } catch {
      // Ranking portfolio loading is non-critical
    } finally {
      setRankingPortfolioLoading(false)
    }
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
        await loadSignalConfigs()
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

  const refreshRealtimeSections = useCallback(() => {
    loadPublicData()
    if (privateAccessReady) {
      loadPrivateData()
    }
  }, [privateAccessReady])

  // Bootstrap
  useEffect(() => {
    if (!ready) return
    loadPublicData()
    loadRankingPortfolio()
    if (privateAccessReady) {
      loadPrivateData({ bootstrap: true })
    } else {
      resetPrivateState()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ready, privateAccessReady])

  useEffect(() => {
    if (!ready) return undefined

    const intervalId = window.setInterval(() => {
      refreshRealtimeSections()
    }, 10000)

    return () => window.clearInterval(intervalId)
  }, [ready, refreshRealtimeSections])

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
    return [...(watchlist.items || [])]
  }, [watchlist.items])

  return (
    <div className="space-y-6">
      <Head>
        <title>行情看板 — 卧龙AI量化交易台</title>
        <meta name="description" content="卧龙AI量化交易台行情看板 — 大盘指数、卧龙AI精选模拟组合与关注股票列表。支持登录后维护关注池并进入个股详情页。" />
        <link rel="canonical" href="https://wolongtrader.top/live-trading" />
      </Head>

      {/* Market overview — compact index bar */}
      <section className="rounded-2xl border border-border bg-card px-5 py-3">
        <div className="flex flex-wrap items-center gap-x-5 gap-y-2">
          <span className="text-xs font-medium text-foreground-dim">大盘</span>
          {[...(marketOverviewA?.indexes || []), ...(marketOverviewHK?.indexes || [])].map((index) => (
            <div key={index.code} className="flex items-baseline gap-1.5">
              <span className="text-xs text-foreground-dim">{formatMarketIndexTitle(index.name, index.code)}</span>
              <span className="text-sm font-semibold tabular-nums text-foreground">{formatNumber(index.last, 2)}</span>
              <span className={`text-xs font-medium tabular-nums ${index.change_rate >= 0 ? 'text-negative' : 'text-positive'}`}>
                {formatPercent(index.change_rate)}
              </span>
            </div>
          ))}
          {(marketOverviewA?.indexes || []).length === 0 && (marketOverviewHK?.indexes || []).length === 0 && (
            <span className="text-xs text-foreground-dim animate-pulse">加载中...</span>
          )}
        </div>
      </section>

      <RankingPortfolioPanel data={rankingPortfolioData} loading={rankingPortfolioLoading} />

      {error ? (
        <div className="rounded-xl border border-negative/40 bg-negative/10 px-4 py-3 text-sm text-negative">
          <div>{error}</div>
          {errorNeedsLogin ? (
            <button
              type="button"
              onClick={() => openAuthModal('login', '行情看板相关操作需要登录后才能继续。')}
              className="mt-2 inline-flex rounded-lg border border-negative/40 px-2.5 py-1 text-xs text-negative transition hover:bg-negative/15"
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
            <h3 className="text-base font-semibold text-foreground">添加关注股票</h3>
            <form onSubmit={handleAddWatch} className="mt-3 flex flex-wrap items-end gap-3">
              <input
                value={symbolInput}
                onChange={(e) => setSymbolInput(e.target.value.toUpperCase())}
                placeholder="股票代码，如 00700 或 600519"
                className="w-48 rounded-xl border border-border bg-[var(--color-bg-hover)] px-3 py-2 text-sm text-foreground outline-none transition focus:border-primary"
              />
              <input
                value={nameInput}
                onChange={(e) => setNameInput(e.target.value)}
                placeholder="备注名称（可选）"
                className="w-40 rounded-xl border border-border bg-[var(--color-bg-hover)] px-3 py-2 text-sm text-foreground outline-none transition focus:border-primary"
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
            <div className="rounded-2xl border border-dashed border-border bg-card px-6 py-12 text-center text-sm text-foreground-dim">
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
                  ? 'border-negative/30 hover:border-rose-400/50'
                  : isDown
                    ? 'border-positive/30 hover:border-emerald-400/50'
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
                        <div className="truncate text-sm font-semibold text-foreground">
                          {displayName ? `${displayName}` : item.symbol}
                        </div>
                        <div className="mt-0.5 text-xs text-foreground-dim">
                          {displayName ? item.symbol : ''} · {detectExchangeLabel(item.symbol)}
                        </div>
                      </div>
                      {signalConfigMap[item.symbol]?.is_enabled && (
                        <span className="mt-0.5 inline-flex shrink-0 items-center gap-1 rounded-full border border-positive/30 bg-positive/10 px-2 py-0.5 text-[10px] font-medium text-positive">
                          <span className="inline-block h-1.5 w-1.5 rounded-full bg-emerald-400 animate-pulse" />
                          信号
                        </span>
                      )}
                    </div>

                    {/* Price section */}
                    {snap ? (
                      <div className="mt-3">
                        <div className={`text-xl font-bold tracking-tight ${isUp ? 'text-negative' : isDown ? 'text-positive' : 'text-foreground'}`}>
                          {formatNumber(snap.last_price, snap.last_price >= 100 ? 2 : 3)}
                        </div>
                        <div className="mt-1 flex items-center gap-3 text-xs">
                          <span className={isUp ? 'text-negative' : isDown ? 'text-positive' : 'text-foreground-muted'}>
                            {formatPercent(changeRate)}
                          </span>
                          {snap.volume_ratio > 0 && (
                            <span className="text-foreground-dim">量比 {formatNumber(snap.volume_ratio, 2)}</span>
                          )}
                        </div>
                        <div className="mt-2 grid grid-cols-2 gap-x-4 gap-y-1 text-[11px] text-foreground-dim">
                          <div>成交量 <span className="text-foreground-muted">{formatCompact(snap.volume)}</span></div>
                          <div>成交额 <span className="text-foreground-muted">{formatCompact(snap.turnover)}</span></div>
                          <div>振幅 <span className="text-foreground-muted">{formatPercent(snap.amplitude)}</span></div>
                        </div>
                      </div>
                    ) : (
                      <div className="mt-3 text-xs text-foreground-dim">加载中...</div>
                    )}

                    {/* Footer actions */}
                    <div className="mt-3 flex items-center justify-between border-t border-border pt-3">
                      <span className="text-[11px] text-foreground-dim transition group-hover:text-primary">
                        点击查看详情 →
                      </span>
                      <button
                        type="button"
                        onClick={(e) => {
                          e.stopPropagation()
                          handleDelete(item.symbol)
                        }}
                        className="rounded-lg px-2 py-1 text-[11px] text-negative/60 transition hover:bg-negative/10 hover:text-negative"
                      >
                        删除
                      </button>
                    </div>
                  </div>
                )
              })}
            </section>
          )}
        </>
      ) : (
        <section className="rounded-2xl border border-dashed border-primary/30 bg-primary/10 p-6">
          <div className="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
            <div className="space-y-2">
              <div className="text-lg font-semibold text-foreground">
                {ready ? '登录后开启行情看板' : '正在确认账号状态'}
              </div>
              <p className="max-w-2xl text-sm leading-7 text-foreground-muted">
                {ready
                  ? '登录后可管理关注池、查看实时行情快照和独立股票详情页。'
                  : '正在检查你的登录状态，确认后会自动加载数据。'
                }
              </p>
            </div>
            <button
              type="button"
              disabled={!ready}
              onClick={ready ? () => openAuthModal('login', '登录后即可管理关注池与行情看板。') : undefined}
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
