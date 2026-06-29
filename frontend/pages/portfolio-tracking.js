import { useEffect, useMemo, useState } from 'react'

import dynamic from 'next/dynamic'
import Head from 'next/head'

import { requestJson } from '../lib/api'

const PortfolioTrackingDashboard = dynamic(() => import('../components/PortfolioTrackingDashboard'), { ssr: false })

function buildPortfolioTrackingUrl(pathname, params = {}) {
  const searchParams = new URLSearchParams()
  Object.entries(params).forEach(([key, value]) => {
    if (value === null || value === undefined || value === '') return
    searchParams.set(key, value)
  })
  const query = searchParams.toString()
  return query ? `${pathname}?${query}` : pathname
}

export default function PortfolioTrackingPage() {
  const [overview, setOverview] = useState(null)
  const [overviewLoading, setOverviewLoading] = useState(false)
  const [detailLoading, setDetailLoading] = useState(false)
  const [selectedPortfolioId, setSelectedPortfolioId] = useState('')
  const [daily, setDaily] = useState(null)
  const [positions, setPositions] = useState(null)
  const [trades, setTrades] = useState(null)
  const [metrics, setMetrics] = useState(null)

  useEffect(() => {
    let cancelled = false
    const loadOverview = async () => {
      try {
        setOverviewLoading(true)
        const data = await requestJson('/api/portfolio-tracking/overview')
        if (cancelled) return
        setOverview(data)
        const firstPortfolioId = Array.isArray(data?.items) && data.items.length ? data.items[0].portfolio_id : ''
        setSelectedPortfolioId((current) => current || firstPortfolioId)
      } catch {
        if (!cancelled) {
          setOverview(null)
        }
      } finally {
        if (!cancelled) {
          setOverviewLoading(false)
        }
      }
    }
    loadOverview()
    return () => {
      cancelled = true
    }
  }, [])

  const selectedItem = useMemo(() => {
    const items = Array.isArray(overview?.items) ? overview.items : []
    return items.find((item) => item.portfolio_id === selectedPortfolioId) || items[0] || null
  }, [overview, selectedPortfolioId])

  useEffect(() => {
    if (!selectedItem?.portfolio_id) {
      setDaily(null)
      setPositions(null)
      setTrades(null)
      setMetrics(null)
      return undefined
    }
    let cancelled = false
    const loadDetail = async () => {
      try {
        setDetailLoading(true)
        const latestTradeDate = selectedItem.latest_trade_date || ''
        const portfolioId = selectedItem.portfolio_id
        const [dailyData, positionsData, tradesData, metricsData] = await Promise.all([
          requestJson(buildPortfolioTrackingUrl(`/api/portfolio-tracking/${encodeURIComponent(portfolioId)}/daily`)),
          requestJson(buildPortfolioTrackingUrl(`/api/portfolio-tracking/${encodeURIComponent(portfolioId)}/positions`, { trade_date: latestTradeDate })),
          requestJson(buildPortfolioTrackingUrl(`/api/portfolio-tracking/${encodeURIComponent(portfolioId)}/trades`)),
          requestJson(buildPortfolioTrackingUrl(`/api/portfolio-tracking/${encodeURIComponent(portfolioId)}/metrics`)),
        ])
        if (cancelled) return
        setDaily(dailyData)
        setPositions(positionsData)
        setTrades(tradesData)
        setMetrics(metricsData)
      } catch {
        if (!cancelled) {
          setDaily(null)
          setPositions(null)
          setTrades(null)
          setMetrics(null)
        }
      } finally {
        if (!cancelled) {
          setDetailLoading(false)
        }
      }
    }
    loadDetail()
    return () => {
      cancelled = true
    }
  }, [selectedItem])

  return (
    <div className="space-y-6">
      <Head>
        <title>组合跟踪 — 卧龙AI量化交易台</title>
        <meta
          name="description"
          content="组合跟踪页展示新的模拟组合事实表口径：每日净值、当前持仓、实际理论成交与绩效指标。"
        />
        <link rel="canonical" href="https://wolongtrader.top/portfolio-tracking" />
      </Head>

      <PortfolioTrackingDashboard
        overview={overview}
        overviewLoading={overviewLoading}
        detailLoading={detailLoading}
        selectedPortfolioId={selectedItem?.portfolio_id || ''}
        onSelectPortfolio={setSelectedPortfolioId}
        daily={daily}
        positions={positions}
        trades={trades}
        metrics={metrics}
      />
    </div>
  )
}
