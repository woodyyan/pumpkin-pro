import { useMemo, useState } from 'react'

import { requestJson } from '../lib/api'
import AIAnalysisReportContent from './AIAnalysisReportContent'

export const AI_HISTORY_SUBTITLE = '最近一次观点 + 5日验证'

function hasSignalPerformanceReturn(perf) {
  return typeof perf?.return_pct === 'number' && Number.isFinite(perf.return_pct)
}

function formatSignalPerformancePct(value) {
  if (typeof value !== 'number' || !Number.isFinite(value)) return '--'
  const prefix = value > 0 ? '+' : ''
  return `${prefix}${value.toFixed(1)}%`
}

function getSignalPerformanceReturnClass(value) {
  if (typeof value !== 'number' || !Number.isFinite(value)) return 'text-foreground-dim'
  return value >= 0 ? 'text-negative' : 'text-positive'
}

function buildSignalPerformanceSummary(signal, perf) {
  if (!hasSignalPerformanceReturn(perf)) return ''
  const pct = formatSignalPerformancePct(perf.return_pct)
  if (signal === 'buy') return `自上次看多以来，涨幅 ${pct}`
  if (signal === 'sell') return `自上次看空以来，变动 ${pct}`
  return `自上次分析以来，变动 ${pct}`
}

function buildSignalPerformanceStatus(signal, perf) {
  if (!perf?.direction_status || signal === 'hold') return ''
  return perf.direction_status === 'aligned' ? '与观点一致' : '与观点相反'
}

function getSignalPerformanceStatusClass(signal, perf) {
  if (!perf?.direction_status || signal === 'hold') return 'text-foreground-dim bg-[var(--color-bg-hover)] border-border'
  return perf.direction_status === 'aligned'
    ? 'text-sky-200 bg-sky-500/10 border-sky-400/25'
    : 'text-negative bg-negative/10 border-rose-400/25'
}

function isSignalPerformanceEstimated(perf) {
  return perf?.price_basis === 'estimated_close' || perf?.price_basis === 'mixed'
}

function hasQualityValidationReturn(validation) {
  return typeof validation?.primary_return_pct === 'number' && Number.isFinite(validation.primary_return_pct)
}

function buildQualityValidationHeadline(validation) {
  if (!validation) return ''
  const days = Math.max(1, validation.primary_window_days || 5)
  const availableDays = Math.max(0, validation.available_days || 0)
  if (validation.summary_status === 'pending') {
    return `${days}日验证中（${Math.min(availableDays, days)}/${days}）`
  }
  if (hasQualityValidationReturn(validation)) {
    return `${days}日验证：${formatSignalPerformancePct(validation.primary_return_pct)}`
  }
  return `${days}日验证`
}

function buildQualityValidationStatusLabel(validation) {
  return validation?.summary_label || ''
}

function getQualityValidationReturnClass(validation) {
  if (!hasQualityValidationReturn(validation)) return 'text-foreground-dim'
  return getSignalPerformanceReturnClass(validation.primary_return_pct)
}

function getQualityValidationStatusClass(validation) {
  switch (validation?.summary_status) {
    case 'hit':
      return 'text-sky-200 bg-sky-500/10 border-sky-400/25'
    case 'miss':
      return 'text-negative bg-negative/10 border-rose-400/25'
    case 'pending':
      return 'text-amber-200 bg-amber-500/10 border-amber-400/25'
    default:
      return 'text-foreground-dim bg-[var(--color-bg-hover)] border-border'
  }
}

function isQualityValidationEstimated(validation) {
  return validation?.price_basis === 'estimated_close' || validation?.price_basis === 'mixed'
}

function buildQualityWindowStatusLabel(window) {
  if (!window?.ready) return '验证中'
  if (window.direction_status === 'hit') return '命中'
  if (window.direction_status === 'miss') return '失准'
  return '区间变动'
}

function getQualityWindowStatusClass(window) {
  if (!window?.ready) return 'text-amber-200 bg-amber-500/10 border-amber-400/25'
  if (window.direction_status === 'hit') return 'text-sky-200 bg-sky-500/10 border-sky-400/25'
  if (window.direction_status === 'miss') return 'text-negative bg-negative/10 border-rose-400/25'
  return 'text-foreground-dim bg-[var(--color-bg-hover)] border-border'
}

function buildQualityWindowValue(window, validation) {
  if (window?.ready && typeof window?.return_pct === 'number' && Number.isFinite(window.return_pct)) {
    return formatSignalPerformancePct(window.return_pct)
  }
  const horizon = Math.max(1, window?.horizon_days || 0)
  const available = Math.max(0, validation?.available_days || 0)
  return `已完成 ${Math.min(available, horizon)}/${horizon}`
}

function formatTimeAgo(isoStr) {
  if (!isoStr) return ''
  const d = new Date(isoStr)
  const diff = Date.now() - d.getTime()
  const mins = Math.floor(diff / 60000)
  const hours = Math.floor(diff / 3600000)
  const days = Math.floor(diff / 86400000)
  if (mins < 1) return '刚刚'
  if (mins < 60) return `${mins} 分钟前`
  if (hours < 24) return `${hours} 小时前`
  return `${days} 天前 · ${d.toLocaleDateString('zh-CN')} ${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
}

function isStale(isoStr) {
  if (!isoStr) return false
  return Date.now() - new Date(isoStr).getTime() > 86400000
}

function getSignalLabel(signal) {
  return signal === 'buy' ? '看多' : signal === 'sell' ? '看空' : '观望'
}

function HistoryQualitySummary({ validation }) {
  if (!validation) return null
  const windows = Array.isArray(validation.windows) ? validation.windows.slice(0, 3) : []
  return (
    <div className="mt-2 flex flex-wrap items-center gap-2">
      <span className={`rounded-full border px-2 py-0.5 text-[10px] ${getQualityValidationStatusClass(validation)}`}>
        {buildQualityValidationStatusLabel(validation) || '质量验证'}
      </span>
      <span className={`text-[11px] font-medium ${getQualityValidationReturnClass(validation)}`}>
        {buildQualityValidationHeadline(validation)}
      </span>
      {isQualityValidationEstimated(validation) ? (
        <span className="text-[10px] text-foreground-dim">按收盘价估算</span>
      ) : null}
      {windows.map((window) => (
        <span key={`${window.horizon_days}-${window.end_date || 'pending'}`} className={`rounded-full border px-2 py-0.5 text-[10px] ${getQualityWindowStatusClass(window)}`}>
          {window.horizon_days}日 {buildQualityWindowStatusLabel(window)} · {buildQualityWindowValue(window, validation)}
        </span>
      ))}
    </div>
  )
}

function HistoryCard({ item, expanded, onToggle, detailLoading, detailData, allowDetail = true, onDelete }) {
  const signalMap = {
    buy: { label: '看多', arrow: '↑', color: 'text-negative', dot: '🔴', bg: 'bg-red-500/12', border: 'border-red-400/40' },
    sell: { label: '看空', arrow: '↓', color: 'text-positive', dot: '🟢', bg: 'bg-positive/10', border: 'border-emerald-400/40' },
    hold: { label: '观望', arrow: '→', color: 'text-amber-300', dot: '🟡', bg: 'bg-amber-500/12', border: 'border-amber-400/40' },
  }
  const sig = signalMap[item.signal] || signalMap.hold
  const stale = isStale(item.created_at)
  const signalPerformance = (expanded ? detailData?.signal_performance : null) || item.signal_performance || null
  const qualityValidation = (expanded ? detailData?.quality_validation : null) || item.quality_validation || null
  const performanceSummary = buildSignalPerformanceSummary(item.signal, signalPerformance)
  const performanceStatus = buildSignalPerformanceStatus(item.signal, signalPerformance)
  const analysis = detailData?.analysis || {}

  return (
    <div>
      <div
        className={`group flex items-start justify-between gap-3 rounded-xl border px-3.5 py-2.5 transition ${allowDetail ? 'cursor-pointer' : ''} ${stale ? 'border-border bg-[var(--color-bg-hover)]' : 'border-primary/15 bg-primary/[0.04]'} ${expanded ? `${sig.border} ${sig.bg} ring-1 ring-inset ring-border` : ''}`}
        onClick={allowDetail ? onToggle : undefined}
      >
        <div className="min-w-0 flex-1">
          <div className="flex min-w-0 items-center gap-2">
            <span className="shrink-0 text-sm">{sig.dot}</span>
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2">
                <span className={`text-sm font-semibold ${sig.color}`}>{item.symbol_name || item.symbol}</span>
                <span className="font-mono text-[11px] text-foreground-dim">{item.symbol}</span>
                <span className={`rounded-full border px-2 py-0.5 text-[10px] ${sig.border} ${sig.bg} ${sig.color}`}>{sig.label}</span>
                <span className="text-[11px] text-foreground-dim">置信度 {item.confidence_score ?? '--'}%</span>
              </div>
              <div className="mt-1 flex flex-wrap items-center gap-2 text-[11px] text-foreground-dim">
                <span>{formatTimeAgo(item.created_at)}</span>
                {performanceSummary ? <span className={getSignalPerformanceReturnClass(signalPerformance?.return_pct)}>{performanceSummary}</span> : null}
                {performanceStatus ? <span className={`rounded-full border px-2 py-0.5 ${getSignalPerformanceStatusClass(item.signal, signalPerformance)}`}>{performanceStatus}</span> : null}
                {isSignalPerformanceEstimated(signalPerformance) ? <span>按收盘价估算</span> : null}
              </div>
              <HistoryQualitySummary validation={qualityValidation} />
            </div>
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          {onDelete ? (
            <button
              type="button"
              onClick={(event) => {
                event.stopPropagation()
                onDelete(item.id)
              }}
              className="rounded-lg border border-rose-400/25 px-2 py-1 text-[11px] text-negative/80 transition hover:bg-negative/10"
            >
              删除
            </button>
          ) : null}
          {allowDetail ? <span className={`text-foreground-dim transition-transform ${expanded ? 'rotate-180' : ''}`}>▼</span> : null}
        </div>
      </div>
      {allowDetail && expanded ? (
        <div className="mt-2 rounded-xl border border-border bg-[var(--color-bg-hover)] p-3.5">
          {detailLoading ? (
            <div className="text-sm text-foreground-dim">详情加载中...</div>
          ) : detailData?.analysis ? (
            <div className="space-y-3">
              <AIAnalysisReportContent result={detailData} allowLogicToggle={false} hidePositionHint showAnalysisTime={false} />
              {analysis?.trading_suggestions?.action_suggestion ? (
                <div className="rounded-xl border border-sky-400/15 bg-sky-500/[0.04] px-3 py-2 text-[12px] leading-6 text-sky-800 dark:text-sky-200/80">
                  <div className="mb-1 text-xs font-semibold text-sky-800 dark:text-sky-200/80">📋 交易建议</div>
                  {analysis.trading_suggestions.action_suggestion}
                </div>
              ) : null}
              {Array.isArray(analysis?.key_catalysts) && analysis.key_catalysts.length > 0 ? (
                <div className="rounded-xl border border-sky-400/12 bg-sky-500/[0.03] px-3 py-2 text-[12px] leading-6 text-sky-800 dark:text-sky-200/70">
                  <div className="mb-1 text-xs font-semibold text-sky-800 dark:text-sky-200/70">✨ 潜在催化因素</div>
                  {analysis.key_catalysts.map((c, idx) => <div key={idx} className="mt-1 first:mt-0">💡 {c}</div>)}
                </div>
              ) : null}
            </div>
          ) : (
            <div className="text-sm text-foreground-dim">暂无详情</div>
          )}
        </div>
      ) : null}
    </div>
  )
}

export function SymbolAIAnalysisHistorySection({ items, expanded, onToggleExpand, symbol, onDelete }) {
  const [expandedId, setExpandedId] = useState(null)
  const [detailLoading, setDetailLoading] = useState(false)
  const [detailData, setDetailData] = useState(null)

  const handleToggleDetail = async (id) => {
    if (expandedId === id) {
      setExpandedId(null)
      setDetailData(null)
      return
    }
    setExpandedId(id)
    setDetailLoading(true)
    setDetailData(null)
    try {
      const data = await requestJson(`/api/live/symbols/${encodeURIComponent(symbol)}/analysis-history?id=${id}`)
      setDetailData(data || null)
    } catch {
      setDetailData(null)
    } finally {
      setDetailLoading(false)
    }
  }

  return (
    <section className="rounded-2xl border border-border bg-card p-4">
      <button type="button" onClick={onToggleExpand} className="flex w-full items-center justify-between text-left">
        <div className="flex items-start gap-2">
          <span className="mt-0.5 text-sm">📋</span>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <span className="text-[13px] font-medium text-foreground-muted">AI分析历史</span>
              <span className="rounded-full bg-[var(--color-bg-hover)] px-2 py-0.5 text-[10px] text-foreground-dim">{items.length} 条</span>
            </div>
            <p className="mt-1 text-[11px] text-foreground-dim">{AI_HISTORY_SUBTITLE}</p>
          </div>
        </div>
        <span className={`text-foreground-dim transition-transform duration-200 ${expanded ? 'rotate-180' : ''}`}>▼</span>
      </button>
      {expanded ? (
        <div className="mt-3 space-y-2">
          {items.slice(0, 5).map((item) => (
            <HistoryCard
              key={item.id}
              item={item}
              expanded={expandedId === item.id}
              onToggle={() => handleToggleDetail(item.id)}
              detailLoading={detailLoading && expandedId === item.id}
              detailData={expandedId === item.id ? detailData : null}
              onDelete={onDelete}
            />
          ))}
        </div>
      ) : null}
    </section>
  )
}

export function GlobalAIAnalysisHistorySection({ items, loading, error, page, pageSize, total, onPageChange }) {
  const totalPages = Math.max(1, Math.ceil((Number(total) || 0) / pageSize))
  const canPrev = page > 1
  const canNext = page < totalPages
  const summaryText = useMemo(() => {
    if (!total) return '暂无分析历史'
    const start = (page - 1) * pageSize + 1
    const end = Math.min(page * pageSize, total)
    return `最近分析历史 ${start}-${end} / ${total}`
  }, [page, pageSize, total])

  return (
    <section className="rounded-2xl border border-border bg-card p-4 sm:p-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <div className="text-sm font-semibold text-foreground">最近 AI 分析历史</div>
          <p className="mt-1 text-xs text-foreground-dim">展示用户最近分析过的全局历史，默认每页 10 条，含 5 日验证等质量结果。</p>
        </div>
        <div className="text-xs text-foreground-dim">{summaryText}</div>
      </div>
      {loading ? <div className="mt-4 text-sm text-foreground-dim">历史加载中...</div> : null}
      {error ? <div className="mt-4 rounded-xl border border-negative/35 bg-negative/10 px-3 py-2 text-sm text-negative">{error}</div> : null}
      {!loading && !error && items.length === 0 ? <div className="mt-4 rounded-xl border border-dashed border-border px-4 py-6 text-sm text-foreground-dim">暂无历史记录，完成一次 AI 分析后会显示在这里。</div> : null}
      {!loading && items.length > 0 ? (
        <div className="mt-4 space-y-3">
          {items.map((item) => (
            <HistoryCard key={item.id} item={item} expanded={false} allowDetail={false} />
          ))}
        </div>
      ) : null}
      {totalPages > 1 ? (
        <div className="mt-4 flex items-center justify-end gap-2 text-xs">
          <button
            type="button"
            disabled={!canPrev}
            onClick={() => onPageChange(page - 1)}
            className="rounded-lg border border-border px-3 py-1.5 text-foreground-muted transition hover:border-[var(--color-border-strong)] hover:text-foreground disabled:cursor-not-allowed disabled:opacity-40"
          >
            上一页
          </button>
          <span className="text-foreground-dim">第 {page} / {totalPages} 页</span>
          <button
            type="button"
            disabled={!canNext}
            onClick={() => onPageChange(page + 1)}
            className="rounded-lg border border-border px-3 py-1.5 text-foreground-muted transition hover:border-[var(--color-border-strong)] hover:text-foreground disabled:cursor-not-allowed disabled:opacity-40"
          >
            下一页
          </button>
        </div>
      ) : null}
    </section>
  )
}
