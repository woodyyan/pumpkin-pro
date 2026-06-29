import { useCallback, useEffect, useMemo, useState } from 'react'

import dynamic from 'next/dynamic'

import { requestJson } from '../lib/api'
import { buildQuadrantUrl } from '../lib/quadrant-api'
import {
  buildQuadrantDetailSymbol,
  createQuadrantSearchState,
  findQuadrantStockByCode,
  getQuadrantSearchEntry,
  searchQuadrantStocks,
  updateQuadrantSearchEntry,
} from '../lib/quadrant-search'
import { formatCloseDateLabel, parseTradeDateLabelDate } from '../lib/trade-date-label'
import QuadrantSearchBox from './QuadrantSearchBox'

const QuadrantChart = dynamic(() => import('./QuadrantChart'), { ssr: false })

export default function QuadrantOverviewSection({ watchlistSymbols = [] }) {
  const [quadrantData, setQuadrantData] = useState(null)
  const [quadrantLoading, setQuadrantLoading] = useState(false)
  const [quadrantExchange, setQuadrantExchange] = useState('ASHARE')
  const [quadrantSearchState, setQuadrantSearchState] = useState(() => createQuadrantSearchState())

  const normalizedWatchlistSymbols = useMemo(() => {
    return Array.isArray(watchlistSymbols)
      ? [...new Set(watchlistSymbols.map((symbol) => String(symbol || '').trim()).filter(Boolean))]
      : []
  }, [watchlistSymbols])

  const currentExchangeWatchlistSymbols = useMemo(() => {
    return quadrantExchange === 'HKEX'
      ? normalizedWatchlistSymbols.filter((symbol) => symbol.toUpperCase().endsWith('.HK'))
      : normalizedWatchlistSymbols.filter((symbol) => !symbol.toUpperCase().endsWith('.HK'))
  }, [normalizedWatchlistSymbols, quadrantExchange])

  const currentExchangeWatchlistKey = useMemo(
    () => currentExchangeWatchlistSymbols.join(','),
    [currentExchangeWatchlistSymbols]
  )

  const currentQuadrantSearch = useMemo(
    () => getQuadrantSearchEntry(quadrantSearchState, quadrantExchange),
    [quadrantSearchState, quadrantExchange]
  )

  const currentQuadrantStocks = quadrantData?.all_stocks || []

  const currentQuadrantSearchResults = useMemo(
    () => searchQuadrantStocks(currentQuadrantStocks, currentQuadrantSearch.query, quadrantExchange),
    [currentQuadrantStocks, currentQuadrantSearch.query, quadrantExchange]
  )

  const highlightedQuadrantStock = useMemo(
    () => findQuadrantStockByCode(currentQuadrantStocks, currentQuadrantSearch.selectedCode, quadrantExchange),
    [currentQuadrantStocks, currentQuadrantSearch.selectedCode, quadrantExchange]
  )

  useEffect(() => {
    let cancelled = false

    const loadQuadrant = async () => {
      try {
        setQuadrantLoading(true)
        const data = await requestJson(buildQuadrantUrl({
          exchange: quadrantExchange,
          watchlistSymbols: currentExchangeWatchlistSymbols,
        }))
        if (!cancelled) {
          setQuadrantData(data)
        }
      } catch {
        if (!cancelled) {
          setQuadrantData(null)
        }
      } finally {
        if (!cancelled) {
          setQuadrantLoading(false)
        }
      }
    }

    loadQuadrant()
    return () => {
      cancelled = true
    }
  }, [quadrantExchange, currentExchangeWatchlistKey, currentExchangeWatchlistSymbols])

  useEffect(() => {
    if (!currentQuadrantSearch.selectedCode) return
    if (quadrantLoading) return
    if (currentQuadrantStocks.length === 0) return
    if (highlightedQuadrantStock) return

    setQuadrantSearchState((prev) => updateQuadrantSearchEntry(prev, quadrantExchange, { selectedCode: '' }))
  }, [
    currentQuadrantSearch.selectedCode,
    currentQuadrantStocks.length,
    highlightedQuadrantStock,
    quadrantExchange,
    quadrantLoading,
  ])

  const handleQuadrantSearchChange = useCallback((query) => {
    setQuadrantSearchState((prev) => updateQuadrantSearchEntry(prev, quadrantExchange, {
      query,
      selectedCode: '',
    }))
  }, [quadrantExchange])

  const handleQuadrantSearchSelect = useCallback((stock) => {
    if (!stock?.c) return
    setQuadrantSearchState((prev) => updateQuadrantSearchEntry(prev, quadrantExchange, {
      query: stock.n || stock.c,
      selectedCode: stock.c,
    }))
  }, [quadrantExchange])

  const handleQuadrantSearchClear = useCallback(() => {
    setQuadrantSearchState((prev) => updateQuadrantSearchEntry(prev, quadrantExchange, {
      query: '',
      selectedCode: '',
    }))
  }, [quadrantExchange])

  const handleOpenHighlightedQuadrantDetail = useCallback(() => {
    if (!highlightedQuadrantStock?.c) return
    const symbol = buildQuadrantDetailSymbol(highlightedQuadrantStock.c, quadrantExchange)
    if (!symbol) return
    window.open(`/live-trading/${symbol}`, '_blank')
  }, [highlightedQuadrantStock, quadrantExchange])

  const handleQuadrantExchangeChange = (newExchange) => {
    if (newExchange === quadrantExchange) return
    setQuadrantExchange(newExchange)
    setQuadrantData(null)
  }

  return (
    <section className="rounded-2xl border border-border bg-card p-5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h3 className="text-base font-semibold text-foreground">风险机会全景图</h3>
          {quadrantData?.meta?.computed_at && (
            <div className="mt-1 flex items-center gap-2 text-xs text-foreground-dim">
              <span>{formatCloseDateLabel(quadrantData.meta.source_trade_date, quadrantData.meta.computed_at)}</span>
              {quadrantData.meta.total_count > 0 && <span>· {quadrantData.meta.total_count.toLocaleString()} 只</span>}
              {(() => {
                const staleDate = parseTradeDateLabelDate(quadrantData.meta.source_trade_date) || parseTradeDateLabelDate(quadrantData.meta.computed_at)
                if (!staleDate) return null
                const daysDiff = Math.floor((Date.now() - new Date(staleDate).getTime()) / (1000 * 60 * 60 * 24))
                if (daysDiff > 3) {
                  return <span className="rounded bg-amber-200 px-1.5 py-0.5 text-xs font-medium text-amber-800 dark:bg-amber-500/15 dark:text-amber-200">数据已过期（{daysDiff} 天前）</span>
                }
                return null
              })()}
            </div>
          )}
        </div>

        <div className="flex items-center gap-1 rounded-lg bg-[var(--color-bg-hover)] p-0.5">
          {[
            { key: 'ASHARE', label: 'A 股' },
            { key: 'HKEX', label: '港股' },
          ].map((tab) => (
            <button
              key={tab.key}
              type="button"
              onClick={() => handleQuadrantExchangeChange(tab.key)}
              className={`rounded-md px-3 py-1 text-xs font-medium transition ${
                quadrantExchange === tab.key
                  ? 'bg-primary text-black'
                  : 'text-foreground-dim hover:bg-[var(--color-bg-hover)] hover:text-foreground-muted'
              }`}
            >
              {tab.label}
            </button>
          ))}
        </div>
      </div>

      <div className="mt-4">
        <QuadrantSearchBox
          market={quadrantExchange}
          query={currentQuadrantSearch.query}
          results={currentQuadrantSearchResults}
          selectedCode={currentQuadrantSearch.selectedCode}
          disabled={!quadrantData || currentQuadrantStocks.length === 0}
          onQueryChange={handleQuadrantSearchChange}
          onSelect={handleQuadrantSearchSelect}
          onClear={handleQuadrantSearchClear}
        />
      </div>

      {quadrantLoading && !quadrantData ? (
        <div className="mt-6 flex items-center justify-center py-12 text-sm text-foreground-dim">
          <span className="animate-pulse">加载四象限数据...</span>
        </div>
      ) : quadrantData && quadrantData.all_stocks && quadrantData.all_stocks.length > 0 ? (
        <>
          <div className="mt-4 w-full">
            <QuadrantChart
              allStocks={quadrantData.all_stocks}
              watchlist={quadrantData.watchlist_details || []}
              market={quadrantExchange}
              highlightCode={currentQuadrantSearch.selectedCode}
              autoOpenTooltip={Boolean(currentQuadrantSearch.selectedCode)}
            />
          </div>

          {highlightedQuadrantStock && (
            <div className="mt-4 flex flex-col gap-3 rounded-2xl border border-primary/20 bg-primary/5 px-4 py-3 md:flex-row md:items-center md:justify-between">
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2 text-sm text-foreground">
                  <span className="font-semibold text-primary">已定位</span>
                  <span className="font-medium">{highlightedQuadrantStock.n}</span>
                  <span className="font-mono text-xs text-foreground-dim">{highlightedQuadrantStock.c}</span>
                  <span className={`rounded-full px-2 py-0.5 text-[11px] ${quadrantBadgeClass(highlightedQuadrantStock.q)}`}>
                    {highlightedQuadrantStock.q}区
                  </span>
                </div>
                <div className="mt-1 text-xs text-foreground-dim">
                  机会 {Number(highlightedQuadrantStock.o || 0).toFixed(1)} / 风险 {Number(highlightedQuadrantStock.r || 0).toFixed(1)}
                </div>
              </div>
              <div className="flex shrink-0 items-center gap-2">
                <button
                  type="button"
                  onClick={handleOpenHighlightedQuadrantDetail}
                  className="inline-flex items-center justify-center rounded-xl border border-primary/35 bg-primary/10 px-3 py-2 text-xs font-medium text-primary transition hover:bg-primary/15"
                >
                  查看详情
                </button>
                <button
                  type="button"
                  onClick={handleQuadrantSearchClear}
                  className="inline-flex items-center justify-center rounded-xl border border-border px-3 py-2 text-xs text-foreground-muted transition hover:bg-[var(--color-bg-hover)] hover:text-foreground-muted"
                >
                  清除高亮
                </button>
              </div>
            </div>
          )}

          <div className="mt-4 space-y-3">
            <div className="flex flex-wrap gap-3 text-xs">
              <span className="inline-flex items-center gap-1.5 rounded-lg bg-positive/10 px-2.5 py-1 text-positive">
                <span className="inline-block h-2 w-2 rounded-full bg-emerald-400" />
                机会区 {quadrantData.summary?.opportunity_zone || 0}
              </span>
              <span className="inline-flex items-center gap-1.5 rounded-lg bg-amber-500/10 px-2.5 py-1 text-amber-300">
                <span className="inline-block h-2 w-2 rounded-full bg-amber-400" />
                拥挤区 {quadrantData.summary?.crowded_zone || 0}
              </span>
              <span className="inline-flex items-center gap-1.5 rounded-lg bg-negative/10 px-2.5 py-1 text-negative">
                <span className="inline-block h-2 w-2 rounded-full bg-rose-400" />
                泡沫区 {quadrantData.summary?.bubble_zone || 0}
              </span>
              <span className="inline-flex items-center gap-1.5 rounded-lg bg-[var(--color-bg-hover)] px-2.5 py-1 text-foreground-muted">
                <span className="inline-block h-2 w-2 rounded-full bg-white/40" />
                防御区 {quadrantData.summary?.defensive_zone || 0}
              </span>
              <span className="inline-flex items-center gap-1.5 rounded-lg bg-blue-500/10 px-2.5 py-1 text-blue-300">
                <span className="inline-block h-2 w-2 rounded-full bg-blue-400" />
                中性区 {quadrantData.summary?.neutral_zone || 0}
              </span>
            </div>

            {(quadrantData.watchlist_details || []).length > 0 && (
              <div className="rounded-xl border border-border/60 bg-[var(--color-bg-hover)] p-3">
                <div className="text-xs font-medium text-foreground-dim">我的关注（{quadrantData.watchlist_details.length} 只）</div>
                <div className="mt-2 space-y-1">
                  {quadrantData.watchlist_details.map((item) => (
                    <div key={item.code} className="flex items-center gap-2 text-xs">
                      <span className={`inline-block h-2 w-2 rounded-full ${watchlistQuadrantDotClass(item.quadrant)}`} />
                      <span className="font-medium text-foreground-muted">{item.name}</span>
                      <span className="text-foreground-dim">—</span>
                      <span className="text-foreground-muted">{item.quadrant}区</span>
                      <span className="text-foreground-dim">（机会 {item.opportunity.toFixed(0)} / 风险 {item.risk.toFixed(0)}）</span>
                    </div>
                  ))}
                </div>
              </div>
            )}
          </div>
        </>
      ) : (
        <div className="mt-4 rounded-xl border border-dashed border-border px-4 py-8 text-center text-sm text-foreground-dim">
          {quadrantData?.all_stocks?.length === 0
            ? '四象限数据尚未计算，请等待每日 20:00 收盘后自动计算完成。'
            : '加载四象限数据中...'}
        </div>
      )}
    </section>
  )
}

function quadrantBadgeClass(quadrant) {
  switch (quadrant) {
    case '机会':
      return 'bg-positive/10 text-positive'
    case '拥挤':
      return 'bg-amber-500/12 text-amber-300'
    case '泡沫':
      return 'bg-negative/10 text-negative'
    case '防御':
      return 'bg-[var(--color-bg-secondary)] text-foreground-muted'
    default:
      return 'bg-blue-500/12 text-blue-300'
  }
}

function watchlistQuadrantDotClass(quadrant) {
  switch (quadrant) {
    case '机会':
      return 'bg-emerald-400'
    case '拥挤':
      return 'bg-amber-400'
    case '泡沫':
      return 'bg-rose-400'
    case '防御':
      return 'bg-white/40'
    default:
      return 'bg-blue-400'
  }
}
