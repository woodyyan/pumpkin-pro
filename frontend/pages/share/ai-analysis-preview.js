import Head from 'next/head'
import { useEffect, useState } from 'react'

import AIAnalysisShareCard from '../../components/AIAnalysisShareCard'
import { buildAIAnalysisSharePayload } from '../../lib/ai-analysis-share'

function readSharePayloadFromWindow() {
  if (typeof window === 'undefined') return null
  return buildAIAnalysisSharePayload(window.__AI_SHARE_PAYLOAD__ || null)
}

export default function AIAnalysisSharePreviewPage() {
  const [payload, setPayload] = useState(null)

  useEffect(() => {
    const applyPayload = (nextPayload) => {
      const normalizedPayload = buildAIAnalysisSharePayload(nextPayload)
      if (normalizedPayload?.result?.analysis) {
        setPayload(normalizedPayload)
      }
    }

    applyPayload(readSharePayloadFromWindow())

    const handlePayload = (event) => {
      applyPayload(event?.detail || null)
    }

    window.addEventListener('ai-analysis-share-payload', handlePayload)
    return () => window.removeEventListener('ai-analysis-share-payload', handlePayload)
  }, [])

  return (
    <>
      <Head>
        <title>AI 分析分享图预览</title>
        <meta name="robots" content="noindex,nofollow" />
      </Head>
      <div className="min-h-screen bg-[#05070b] px-8 py-10">
        <div className="mx-auto w-fit" data-share-ready={payload ? 'true' : 'false'}>
          {payload ? (
            <AIAnalysisShareCard payload={payload} />
          ) : (
            <div className="rounded-2xl border border-white/10 bg-white/[0.03] px-6 py-5 text-sm text-white/45">
              等待分享内容...
            </div>
          )}
        </div>
      </div>
    </>
  )
}
