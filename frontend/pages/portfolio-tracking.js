import { useEffect, useState } from 'react'

import dynamic from 'next/dynamic'
import Head from 'next/head'

import { requestJson } from '../lib/api'

const RankingPortfolioPanel = dynamic(() => import('../components/RankingPortfolioPanel'), { ssr: false })

export default function PortfolioTrackingPage() {
  const [rankingPortfolioData, setRankingPortfolioData] = useState(null)
  const [rankingPortfolioLoading, setRankingPortfolioLoading] = useState(false)

  useEffect(() => {
    let cancelled = false

    const loadRankingPortfolio = async () => {
      try {
        setRankingPortfolioLoading(true)
        const data = await requestJson('/api/quadrant/ranking-portfolio')
        if (!cancelled) {
          setRankingPortfolioData(data)
        }
      } catch {
        if (!cancelled) {
          setRankingPortfolioData(null)
        }
      } finally {
        if (!cancelled) {
          setRankingPortfolioLoading(false)
        }
      }
    }

    loadRankingPortfolio()

    return () => {
      cancelled = true
    }
  }, [])

  return (
    <div className="space-y-6">
      <Head>
        <title>组合跟踪 — 卧龙AI量化交易台</title>
        <meta
          name="description"
          content="组合跟踪页集中展示卧龙AI精选模拟组合的收益曲线、风险指标、当前成分股与最近一次调仓。"
        />
        <link rel="canonical" href="https://wolongtrader.top/portfolio-tracking" />
      </Head>

      <section className="rounded-2xl border border-border bg-card px-5 py-5">
        <div className="max-w-3xl">
          <h1 className="text-xl font-semibold tracking-tight text-foreground">组合跟踪</h1>
          <p className="mt-2 text-sm leading-6 text-foreground-muted">
            这里会展示卧龙量化因子选股出来的结果，并持续跟踪他们的收益，方便大家参考。
          </p>
        </div>
      </section>

      <RankingPortfolioPanel data={rankingPortfolioData} loading={rankingPortfolioLoading} />
    </div>
  )
}
