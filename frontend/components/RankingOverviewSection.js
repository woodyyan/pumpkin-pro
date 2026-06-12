import { useEffect, useState } from 'react'

import { requestJson } from '../lib/api'
import { buildQuadrantRankingUrl } from '../lib/quadrant-api'
import RankingPanel from './RankingPanel'

export default function RankingOverviewSection() {
  const [rankingData, setRankingData] = useState(null)
  const [rankingLoading, setRankingLoading] = useState(false)
  const [rankingExchange, setRankingExchange] = useState('ASHARE')

  useEffect(() => {
    let cancelled = false

    const loadRanking = async () => {
      try {
        setRankingLoading(true)
        const data = await requestJson(buildQuadrantRankingUrl(rankingExchange, 20))
        if (!cancelled) {
          setRankingData(data)
        }
      } catch {
        if (!cancelled) {
          setRankingData(null)
        }
      } finally {
        if (!cancelled) {
          setRankingLoading(false)
        }
      }
    }

    loadRanking()
    return () => {
      cancelled = true
    }
  }, [rankingExchange])

  return (
    <RankingPanel
      items={rankingData?.items}
      meta={rankingData?.meta}
      loading={rankingLoading}
      exchange={rankingExchange}
      onExchangeChange={setRankingExchange}
    />
  )
}
