import AIAnalysisReportContent from './AIAnalysisReportContent'
import {
  buildAIAnalysisSharePayload,
  getAIAnalysisShareDataTimestamp,
  getAIAnalysisShareMarketLabel,
  getAIAnalysisSharePrimaryTimestamp,
} from '../lib/ai-analysis-share'

function formatDateTime(value) {
  if (!value) return '--'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString('zh-CN', { hour12: false })
}

export default function AIAnalysisShareCard({ payload }) {
  const normalizedPayload = buildAIAnalysisSharePayload(payload)
  if (!normalizedPayload?.result?.analysis) return null

  const stockName = normalizedPayload.symbolName || normalizedPayload.symbol || '股票'
  const stockCode = normalizedPayload.symbol || '--'
  const marketLabel = normalizedPayload.marketLabel || getAIAnalysisShareMarketLabel(normalizedPayload.exchange)
  const generatedAt = getAIAnalysisSharePrimaryTimestamp(normalizedPayload)
  const dataTimestamp = getAIAnalysisShareDataTimestamp(normalizedPayload)

  return (
    <div
      data-share-card-root="true"
      className="w-[1080px] overflow-hidden rounded-[32px] border border-white/10 bg-[#090b10] p-8 text-white shadow-[0_32px_120px_rgba(0,0,0,0.45)]"
    >
      <div className="rounded-[24px] border border-primary/20 bg-[radial-gradient(circle_at_top_left,_rgba(230,126,34,0.16),_transparent_34%),linear-gradient(135deg,rgba(255,255,255,0.05),rgba(255,255,255,0.02))] px-5 py-4">
        <div className="flex items-center justify-between gap-5">
          <div className="min-w-0 flex-1">
            <div className="inline-flex items-center rounded-full border border-primary/35 bg-primary/12 px-2.5 py-1 text-[11px] font-semibold text-primary/95">
              {marketLabel} AI 分析结果
            </div>
            <div className="mt-3 truncate text-[26px] font-bold leading-tight text-white">{stockName}</div>
            <div className="mt-1 text-sm font-medium tracking-[0.16em] text-white/55">{stockCode}</div>
          </div>
          <div className="min-w-[250px] rounded-xl border border-amber-300/20 bg-amber-500/8 px-3.5 py-2.5 text-right">
            <div className="text-[11px] font-semibold tracking-[0.18em] text-amber-200/72">分析时间</div>
            <div className="mt-1 text-base font-semibold text-amber-100">{formatDateTime(generatedAt)}</div>
            <div className="mt-1 text-[11px] text-amber-100/62">行情截至 {formatDateTime(dataTimestamp)}</div>
          </div>
        </div>
      </div>

      <AIAnalysisReportContent
        result={normalizedPayload.result}
        className="mt-5 rounded-[28px] border-white/10 bg-card/95"
        logicExpanded
        allowLogicToggle={false}
        hidePositionHint
        showAnalysisTime={false}
      />

      <div className="mt-5 rounded-[24px] border border-white/10 bg-white/[0.04] px-5 py-4">
        <div className="flex items-center justify-between gap-4">
          <div className="flex items-center gap-3">
            <img src="/logo.png" alt="卧龙" width="40" height="40" className="rounded-xl border border-white/10 bg-white/5 p-1" />
            <div>
              <div className="text-base font-semibold text-white">卧龙AI量化交易台</div>
            </div>
          </div>
          <div className="text-right">
            <div className="text-xs uppercase tracking-[0.28em] text-white/35">Website</div>
            <div className="mt-1 text-sm font-semibold text-white/70">wolongtrader.top</div>
          </div>
        </div>
      </div>
    </div>
  )
}
