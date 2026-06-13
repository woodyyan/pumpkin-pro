import Head from 'next/head'
import { useRouter } from 'next/router'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import { useAuth } from '../../lib/auth-context'
import { fetchSymbolSnapshot, fetchSymbolDailyBars } from '../../lib/portfolio-dashboard'
import { requestJson } from '../../lib/api'
import {
  AI_ANALYSIS_GLOBAL_HISTORY_PAGE_SIZE,
  buildAIAnalysisContext,
  createAIAnalysisInitialState,
  deriveControllerWaitState,
  fetchAIAnalysisNewsContext,
  fetchGlobalAIAnalysisHistory,
  mapAIAnalysisError,
  maybePromptNotification,
  resolveAnalysisTarget,
  runAIAnalysisRequest,
  searchAnalysisTargets,
} from '../../lib/ai-analysis-helpers'
import {
  AIAnalysisCapabilityCards,
  AIAnalysisEntryForm,
  AIAnalysisPanel,
  notifyAIAnalysisFinished,
} from '../../components/AIAnalysisWorkspace'
import { GlobalAIAnalysisHistorySection } from '../../components/AIAnalysisHistorySection'

function formatYiCurrency(value, prefix = '¥') {
  if (typeof value !== 'number' || !Number.isFinite(value)) return '--'
  return `${prefix}${(value / 1e8).toFixed(2)}亿`
}

function formatYiAmount(value, prefix = '¥') {
  if (typeof value !== 'number' || !Number.isFinite(value)) return '--'
  return `${prefix}${(value / 1e8).toFixed(2)}亿`
}

function formatYiShares(value) {
  if (typeof value !== 'number' || !Number.isFinite(value)) return '--'
  return `${(value / 1e8).toFixed(2)}亿股`
}

export default function AIAnalysisPage() {
  const router = useRouter()
  const { isLoggedIn, openAuthModal, ready } = useAuth()
  const [query, setQuery] = useState('')
  const [selectedTarget, setSelectedTarget] = useState(null)
  const [candidates, setCandidates] = useState([])
  const [resolving, setResolving] = useState(false)
  const [controller, setController] = useState(() => createAIAnalysisInitialState())
  const [historyItems, setHistoryItems] = useState([])
  const [historyLoading, setHistoryLoading] = useState(false)
  const [historyError, setHistoryError] = useState('')
  const [historyPage, setHistoryPage] = useState(1)
  const [historyTotal, setHistoryTotal] = useState(0)
  const [snapshotPayload, setSnapshotPayload] = useState(null)
  const [movingAveragePayload, setMovingAveragePayload] = useState(null)
  const [fundamentalsItems, setFundamentalsItems] = useState(null)
  const [portfolioData, setPortfolioData] = useState(null)
  const [lastUpdateAt, setLastUpdateAt] = useState('')
  const debounceRef = useRef(null)

  const waitState = useMemo(() => deriveControllerWaitState(controller, { hasPosition: Boolean(portfolioData?.shares > 0) }), [controller, portfolioData])

  useEffect(() => {
    if (!controller.analyzing || !controller.waitStartedAt) return undefined
    const timer = setInterval(() => {
      setController((current) => ({
        ...current,
        waitElapsedSec: Math.max(0, Math.floor((Date.now() - current.waitStartedAt) / 1000)),
      }))
    }, 1000)
    return () => clearInterval(timer)
  }, [controller.analyzing, controller.waitStartedAt])

  const loadHistory = useCallback(async (page = 1) => {
    if (!isLoggedIn) {
      setHistoryItems([])
      setHistoryTotal(0)
      setHistoryError('')
      return
    }
    setHistoryLoading(true)
    try {
      const data = await fetchGlobalAIAnalysisHistory({ page, pageSize: AI_ANALYSIS_GLOBAL_HISTORY_PAGE_SIZE })
      setHistoryItems(Array.isArray(data?.items) ? data.items : [])
      setHistoryTotal(Number(data?.total) || 0)
      setHistoryError('')
    } catch (err) {
      setHistoryItems([])
      setHistoryError(err.message || '历史加载失败')
    } finally {
      setHistoryLoading(false)
    }
  }, [isLoggedIn])

  useEffect(() => {
    loadHistory(historyPage)
  }, [historyPage, loadHistory])

  useEffect(() => {
    if (!ready || !router.isReady) return
    const urlSymbol = String(router.query.symbol || '').trim()
    if (!urlSymbol) return
    resolveAnalysisTarget(urlSymbol, { limit: 8 })
      .then(({ selected }) => {
        if (selected) {
          setSelectedTarget(selected)
          setQuery(selected.symbol)
        }
      })
      .catch(() => {})
  }, [ready, router.isReady, router.query.symbol])

  useEffect(() => {
    const text = query.trim()
    if (text.length < 2) {
      setCandidates([])
      if (!selectedTarget || selectedTarget.symbol !== text) {
        setSelectedTarget((current) => (current && (current.symbol === text || current.code === text) ? current : current))
      }
      return undefined
    }
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(async () => {
      setResolving(true)
      try {
        const items = await searchAnalysisTargets(text)
        setCandidates(items)
        const exact = items.find((item) => item.symbol === text.toUpperCase() || item.code === text.toUpperCase())
        if (exact) setSelectedTarget(exact)
      } catch {
        setCandidates([])
      } finally {
        setResolving(false)
      }
    }, 250)
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current)
    }
  }, [query])

  const loadAnalysisDependencies = useCallback(async (target) => {
    const symbol = target.symbol
    const [snapshotData, dailyBarsData, fundamentalsData, portfolioRes] = await Promise.all([
      fetchSymbolSnapshot(symbol),
      fetchSymbolDailyBars(symbol, 240),
      requestJson(`/api/live/symbols/${encodeURIComponent(symbol)}/fundamentals`).catch(() => null),
      isLoggedIn ? requestJson(`/api/portfolio/${encodeURIComponent(symbol)}`).catch(() => null) : Promise.resolve(null),
    ])
    setSnapshotPayload(snapshotData)
    setLastUpdateAt(new Date().toISOString())
    const bars = Array.isArray(dailyBarsData?.bars) ? dailyBarsData.bars : []
    if (bars.length > 0) {
      const recent = bars.slice(-60)
      const closes = recent.map((item) => Number(item.close) || 0).filter((value) => value > 0)
      const sum = (arr) => arr.reduce((acc, value) => acc + value, 0)
      const calcMA = (days) => {
        const subset = closes.slice(-days)
        return subset.length ? Number((sum(subset) / subset.length).toFixed(3)) : null
      }
      setMovingAveragePayload({
        price_ref: closes.at(-1) || 0,
        ma5: calcMA(5),
        ma20: calcMA(20),
        ma60: calcMA(60),
        status: 'derived',
      })
    } else {
      setMovingAveragePayload(null)
    }
    setFundamentalsItems(fundamentalsData?.items || fundamentalsData?.fundamentals || null)
    setPortfolioData(portfolioRes?.item || null)
  }, [isLoggedIn])

  const buildMarketOverview = useCallback(async (exchange) => {
    try {
      const exParam = exchange === 'SSE' ? '?exchange=SSE' : ''
      const mktRes = await requestJson(`/api/live/market/overview${exParam}`)
      if (Array.isArray(mktRes?.indexes)) {
        const upCount = mktRes.indexes.filter((i) => (i.change_pct || 0) >= 0).length
        const totalCount = mktRes.indexes.length
        let trendSummary = `${totalCount} 指数`
        if (upCount === totalCount) trendSummary += '全部上涨'
        else if (upCount === 0) trendSummary += '全部下跌'
        else if (upCount > totalCount / 2) trendSummary += `多数上涨（${upCount}/${totalCount}）`
        else trendSummary += `偏弱（${upCount}/${totalCount} 涨）`
        return {
          indexes: mktRes.indexes.slice(0, 3).map((i) => ({ name: i.name || '', last: i.last ?? 0, change_pct: i.change_pct ?? 0 })),
          trend_summary: trendSummary,
          _valid: true,
        }
      }
    } catch {}
    return { _valid: false }
  }, [])

  const handleSubmit = useCallback(async () => {
    if (!isLoggedIn) {
      openAuthModal('login', '登录后可使用 AI 分析功能')
      return
    }
    let target = selectedTarget
    const text = query.trim()
    if (!target && text.length >= 2) {
      setResolving(true)
      try {
        const resolved = await resolveAnalysisTarget(text, { limit: 8 })
        setCandidates(resolved.items)
        target = resolved.selected
        if (target) setSelectedTarget(target)
      } finally {
        setResolving(false)
      }
    }
    if (!target) {
      setController((current) => ({ ...current, error: text ? '请输入有效的股票代码或名称，并从候选列表中选择目标股票' : '请输入股票代码或名称', showPanel: true }))
      return
    }

    const requestId = Date.now()
    if (maybePromptNotification()) {
      setController((current) => ({ ...current, notifPromptVisible: true }))
    }
    setController({
      analyzing: true,
      result: null,
      error: '',
      showPanel: true,
      waitStartedAt: Date.now(),
      waitElapsedSec: 0,
      newsContextState: 'loading',
      notifPromptVisible: maybePromptNotification(),
      requestId,
    })

    try {
      await loadAnalysisDependencies(target)
      const context = await buildAIAnalysisContext({
        symbol: target.symbol,
        symbolName: target.symbolName,
        exchange: target.exchange,
        snapshotPayload: snapshotPayload || (await fetchSymbolSnapshot(target.symbol)),
        lastUpdateAt: lastUpdateAt || new Date().toISOString(),
        movingAveragePayload,
        fundamentalsItems,
        portfolioData,
        buildMarketOverview,
        fetchNewsContext: fetchAIAnalysisNewsContext,
        formatYiCurrency,
        formatYiAmount,
        formatYiShares,
      })
      setController((current) => current.requestId !== requestId ? current : { ...current, newsContextState: context.newsState || 'idle' })
      const result = await runAIAnalysisRequest({ symbol: target.symbol, payload: context.payload })
      setController((current) => {
        if (current.requestId !== requestId) return current
        return { ...current, analyzing: false, result, error: '', showPanel: true }
      })
      notifyAIAnalysisFinished({ symbol: target.symbol, symbolName: target.symbolName, result })
      loadHistory(historyPage)
    } catch (err) {
      setController((current) => {
        if (current.requestId !== requestId) return current
        return { ...current, analyzing: false, error: mapAIAnalysisError(err), showPanel: true }
      })
    }
  }, [isLoggedIn, selectedTarget, query, openAuthModal, loadAnalysisDependencies, snapshotPayload, lastUpdateAt, movingAveragePayload, fundamentalsItems, portfolioData, buildMarketOverview, loadHistory, historyPage])

  return (
    <>
      <Head>
        <title>AI分析 - 卧龙AI</title>
      </Head>
      <main className="mx-auto flex w-full max-w-6xl flex-col gap-5 px-4 py-6 sm:px-6 lg:px-8">
        <AIAnalysisEntryForm
          query={query}
          onQueryChange={(value) => {
            setQuery(value)
            if (selectedTarget && value.trim().toUpperCase() !== selectedTarget.symbol.toUpperCase() && value.trim() !== selectedTarget.code) {
              setSelectedTarget(null)
            }
          }}
          onSubmit={handleSubmit}
          onPickCandidate={(item) => {
            setSelectedTarget(item)
            setQuery(item.symbol)
            setCandidates([])
          }}
          candidates={candidates}
          loading={controller.analyzing}
          resolving={resolving}
          selectedTarget={selectedTarget}
          requireLogin={!isLoggedIn}
          onLogin={() => openAuthModal('login', '登录后可使用 AI 分析功能')}
        />

        <AIAnalysisCapabilityCards />

        {controller.showPanel ? (
          controller.error && !controller.analyzing && !controller.result ? (
            <section className="rounded-2xl border border-negative/35 bg-negative/10 px-4 py-4 text-sm text-negative">
              <div>{controller.error}</div>
              <button type="button" onClick={handleSubmit} className="mt-3 rounded-lg border border-negative/30 px-3 py-1.5 text-xs transition hover:bg-negative/10">
                重试分析
              </button>
            </section>
          ) : (
            <AIAnalysisPanel
              analyzing={controller.analyzing}
              result={controller.result}
              error={controller.error}
              symbol={selectedTarget?.symbol || ''}
              exchange={selectedTarget?.exchange || ''}
              symbolName={selectedTarget?.symbolName || selectedTarget?.symbol || ''}
              elapsedSec={controller.waitElapsedSec}
              waitState={waitState}
              referenceItem={historyItems[0] || null}
              newsState={controller.newsContextState}
              notifPromptVisible={controller.notifPromptVisible}
              onNotifPromptClose={() => setController((current) => ({ ...current, notifPromptVisible: false }))}
              onClose={() => setController((current) => ({ ...current, showPanel: false, result: null, error: '' }))}
              onRetry={handleSubmit}
              closeLabel="清空结果"
            />
          )
        ) : (
          <section className="rounded-2xl border border-dashed border-border bg-card px-4 py-8 text-center text-sm text-foreground-dim">
            选择一只股票后开始 AI 分析，结果会展示在这里。
          </section>
        )}

        <GlobalAIAnalysisHistorySection
          items={historyItems}
          loading={historyLoading}
          error={historyError}
          page={historyPage}
          pageSize={AI_ANALYSIS_GLOBAL_HISTORY_PAGE_SIZE}
          total={historyTotal}
          onPageChange={setHistoryPage}
        />
      </main>
    </>
  )
}
