import { useEffect, useMemo, useRef, useState } from 'react'

import AIAnalysisReportContent from './AIAnalysisReportContent'
import AIAnalysisShareCard from './AIAnalysisShareCard'
import { useTheme } from '../lib/theme-context'
import {
  buildAIAnalysisSharePayload,
  exportAIAnalysisShareImages,
} from '../lib/ai-analysis-share'
import {
  getNotificationPermission,
  requestNotificationPermission,
  sendNotification,
  NOTIFICATION_CATEGORIES,
} from '../lib/notification'

export function AIAnalysisPanel({ analyzing, result, error, onClose, onRetry, symbolName, elapsedSec, waitState, referenceItem, symbol, exchange, newsState, notifPromptVisible, onNotifPromptClose, closeLabel = '✕ 关闭' }) {
  const [logicExpanded, setLogicExpanded] = useState(true)
  const [shareBusy, setShareBusy] = useState(false)
  const [shareNotice, setShareNotice] = useState('')
  const [shareError, setShareError] = useState('')
  const shareCardWrapRef = useRef(null)
  const { resolvedTheme } = useTheme()
  const isDark = resolvedTheme !== 'light'

  const sharePayload = useMemo(() => buildAIAnalysisSharePayload({
    symbol,
    symbolName: symbolName || symbol,
    exchange,
    result,
  }), [exchange, result, symbol, symbolName])

  useEffect(() => {
    setShareNotice('')
    setShareError('')
  }, [result?.meta?.generated_at, result?.analysis?.data_timestamp, symbol])

  if (analyzing) {
    return (
      <AIAnalysisLoadingPanel
        symbolName={symbolName}
        symbol={symbol}
        elapsedSec={elapsedSec}
        waitState={waitState}
        referenceItem={referenceItem}
        newsState={newsState}
        notifPromptVisible={notifPromptVisible}
        onNotifPromptClose={onNotifPromptClose}
      />
    )
  }

  const analysis = result?.analysis
  if (!analysis) return null

  const handleShareImage = async () => {
    const shareElement = shareCardWrapRef.current?.querySelector('[data-share-card-root="true"]')
    if (!sharePayload?.result?.analysis || !shareElement) {
      setShareError('分享图内容尚未准备好，请稍后重试')
      return
    }

    setShareBusy(true)
    setShareNotice('')
    setShareError('')
    try {
      const exportResult = await exportAIAnalysisShareImages({
        element: shareElement,
        payload: sharePayload,
        isDark,
      })
      if (exportResult.action === 'shared') {
        setShareNotice(exportResult.total > 1 ? `已唤起系统分享，共 ${exportResult.total} 张图片。` : '已唤起系统分享。')
      } else if (exportResult.action === 'cancelled') {
        setShareNotice('已取消分享。')
      } else {
        setShareNotice(exportResult.total > 1 ? `已下载 ${exportResult.total} 张分享图片。` : '分享图片已开始下载。')
      }
    } catch (err) {
      setShareError(err?.message || '生成分享图片失败，请稍后重试')
    } finally {
      setShareBusy(false)
    }
  }

  return (
    <>
      <AIAnalysisReportContent
        result={result}
        logicExpanded={logicExpanded}
        onToggleLogic={() => setLogicExpanded((current) => !current)}
        actionSlot={(
          <>
            <button
              type="button"
              onClick={handleShareImage}
              disabled={shareBusy}
              className="rounded-lg border border-primary/35 bg-primary/10 px-2.5 py-1.5 text-xs text-primary transition hover:border-primary/60 hover:bg-primary/16 hover:text-primary disabled:cursor-not-allowed disabled:opacity-60"
            >
              {shareBusy ? '生成中...' : '🖼️ 生成图片'}
            </button>
            <button type="button" onClick={onRetry} className="rounded-lg border border-border px-2.5 py-1.5 text-xs text-foreground-muted transition hover:border-[var(--color-border-strong)] hover:text-foreground">
              🔄 重新分析
            </button>
            {onClose ? (
              <button type="button" onClick={onClose} className="rounded-lg border border-border px-2.5 py-1.5 text-xs text-foreground-dim transition hover:border-[var(--color-border-strong)] hover:text-foreground-muted">
                {closeLabel}
              </button>
            ) : null}
          </>
        )}
      />

      {shareNotice ? (
        <div className="mt-3 rounded-xl border border-sky-400/20 bg-sky-500/8 px-3.5 py-2.5 text-xs text-sky-100/85">
          {shareNotice}
        </div>
      ) : null}

      {shareError ? (
        <div className="mt-3 rounded-xl border border-rose-400/25 bg-negative/10 px-3.5 py-2.5 text-xs text-negative/85">
          {shareError}
        </div>
      ) : null}

      <div ref={shareCardWrapRef} aria-hidden className="pointer-events-none fixed left-[-20000px] top-0 z-[-1]">
        {sharePayload ? <AIAnalysisShareCard payload={sharePayload} isDark={isDark} /> : null}
      </div>
    </>
  )
}

export function AIAnalysisLoadingPanel({ symbolName, symbol, elapsedSec, waitState, referenceItem, newsState, notifPromptVisible, onNotifPromptClose }) {
  const steps = waitState?.steps || []
  return (
    <section className="rounded-2xl border border-primary/20 bg-[linear-gradient(180deg,rgba(99,102,241,0.08),rgba(99,102,241,0.02))] p-5">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <div className="text-sm font-semibold text-primary">AI 正在分析 {symbolName || symbol}</div>
          <div className="mt-1 text-xs text-foreground-dim">{waitState?.stage?.kicker} · {waitState?.stage?.title}</div>
          <div className="mt-1 text-xs text-foreground-dim">{waitState?.stage?.hint}</div>
        </div>
        <div className="text-right text-xs text-foreground-dim">
          <div>已等待 {elapsedSec || 0} 秒</div>
          {referenceItem?.created_at ? <div className="mt-1">上次分析：{new Date(referenceItem.created_at).toLocaleString('zh-CN', { hour12: false })}</div> : null}
        </div>
      </div>
      <div className="mt-4 h-2 overflow-hidden rounded-full bg-[var(--color-bg-hover)]">
        <div className="h-full rounded-full bg-primary transition-all" style={{ width: `${waitState?.progress || 0}%` }} />
      </div>
      {notifPromptVisible ? (
        <div className="mt-4 rounded-xl border border-sky-400/20 bg-sky-500/8 px-4 py-3 text-sm text-sky-100/90">
          <div className="font-medium">分析完成后通过桌面通知提醒你</div>
          <div className="mt-1 text-xs text-sky-100/75">如果你准备切到别的页面，可以先开启通知，分析结束后会自动提醒。</div>
          <div className="mt-3 flex flex-wrap gap-2">
            <button
              type="button"
              onClick={async () => {
                await requestNotificationPermission()
                onNotifPromptClose?.()
              }}
              className="rounded-lg border border-sky-300/40 bg-sky-500/15 px-3 py-1.5 text-xs font-medium text-sky-100 transition hover:bg-sky-500/22"
            >
              开启通知
            </button>
            <button
              type="button"
              onClick={() => onNotifPromptClose?.()}
              className="rounded-lg border border-sky-300/20 px-3 py-1.5 text-xs text-sky-100/75 transition hover:bg-sky-500/10"
            >
              稍后再说
            </button>
          </div>
        </div>
      ) : null}
      <div className="mt-4 grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
        {steps.map((step) => (
          <div key={step.key} className="rounded-xl border border-border bg-card/60 px-3.5 py-3">
            <div className="flex items-center justify-between gap-3">
              <span className="text-xs font-medium text-foreground-muted">{step.label}</span>
              <span className={`rounded-full px-2 py-0.5 text-[10px] ${step.status === 'done' ? 'bg-sky-500/10 text-sky-700 dark:text-sky-200' : step.status === 'active' ? 'bg-primary/15 text-primary' : 'bg-[var(--color-bg-hover)] text-foreground-dim'}`}>
                {step.status === 'done' ? '已完成' : step.status === 'active' ? '处理中' : '待开始'}
              </span>
            </div>
            <div className="mt-2 text-[12px] leading-5 text-foreground-dim">{step.description}</div>
          </div>
        ))}
      </div>
      {newsState === 'error' ? <div className="mt-3 text-xs text-amber-300">新闻上下文暂不可用，已按无新闻上下文继续分析。</div> : null}
    </section>
  )
}

export function AIAnalysisCapabilityCards() {
  const cards = [
    { title: '看多 / 看空判断', desc: '从趋势、资金、预期和基本面综合给出方向判断。', icon: '📈' },
    { title: '交易建议', desc: '给出买入区间、止损位、目标位和仓位建议。', icon: '📋' },
    { title: '执行触发条件', desc: '明确哪些条件成立时更适合买入或卖出。', icon: '🎯' },
    { title: '风险提示', desc: '识别潜在风险和催化因素，并结合质量验证复盘。', icon: '⚠️' },
  ]
  return (
    <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
      {cards.map((card) => (
        <div key={card.title} className="rounded-2xl border border-border bg-card px-4 py-3.5">
          <div className="text-lg">{card.icon}</div>
          <div className="mt-2 text-sm font-semibold text-foreground">{card.title}</div>
          <div className="mt-1 text-xs leading-5 text-foreground-dim">{card.desc}</div>
        </div>
      ))}
    </div>
  )
}

export function AIAnalysisEntryForm({
  query,
  onQueryChange,
  onSubmit,
  onPickCandidate,
  candidates,
  loading,
  resolving,
  selectedTarget,
  requireLogin,
  onLogin,
}) {
  const [activeIdx, setActiveIdx] = useState(-1)

  useEffect(() => {
    setActiveIdx(-1)
  }, [query, candidates.length])

  return (
    <section className="rounded-2xl border border-border bg-card p-4 sm:p-5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold text-foreground">AI 股票分析</h1>
        </div>
        {selectedTarget ? (
          <div className="rounded-xl border border-primary/20 bg-primary/8 px-3 py-2 text-xs text-primary">
            当前目标：{selectedTarget.symbolName}（{selectedTarget.symbol}）
          </div>
        ) : null}
      </div>
      <div className="mt-4 flex flex-col gap-3 sm:flex-row">
        <div className="relative min-w-0 flex-1">
          <input
            type="text"
            value={query}
            onChange={(event) => onQueryChange(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === 'ArrowDown' && candidates.length > 0) {
                event.preventDefault()
                setActiveIdx((current) => Math.min(current + 1, candidates.length - 1))
              } else if (event.key === 'ArrowUp' && candidates.length > 0) {
                event.preventDefault()
                setActiveIdx((current) => Math.max(current - 1, 0))
              } else if (event.key === 'Enter') {
                event.preventDefault()
                if (activeIdx >= 0 && candidates[activeIdx]) {
                  onPickCandidate(candidates[activeIdx])
                  return
                }
                onSubmit()
              }
            }}
            placeholder="输入股票代码或股票名称，例如 600519 / 腾讯控股"
            className="w-full rounded-2xl border border-border bg-[var(--color-bg-overlay)] px-4 py-3 text-sm text-foreground outline-none transition focus:border-primary"
          />
          {query.trim().length >= 2 && candidates.length > 0 ? (
            <div className="absolute left-0 right-0 top-[calc(100%+8px)] z-20 overflow-hidden rounded-2xl border border-border bg-card shadow-2xl">
              {candidates.map((item, index) => (
                <button
                  key={`${item.symbol}-${index}`}
                  type="button"
                  onClick={() => onPickCandidate(item)}
                  onMouseEnter={() => setActiveIdx(index)}
                  className={`flex w-full items-center justify-between px-4 py-3 text-left text-sm transition ${activeIdx === index ? 'bg-primary/10 text-primary' : 'text-foreground-muted hover:bg-[var(--color-bg-hover)]'}`}
                >
                  <span>
                    <span className="font-mono font-semibold text-primary/85">{item.code}</span>
                    <span className="ml-2">{item.symbolName}</span>
                  </span>
                  <span className="text-xs text-foreground-dim">{item.exchange === 'HKEX' ? '港股' : 'A股'}</span>
                </button>
              ))}
            </div>
          ) : null}
        </div>
        <button
          type="button"
          onClick={() => {
            if (requireLogin) {
              onLogin?.()
              return
            }
            onSubmit()
          }}
          disabled={loading || resolving}
          title="AI 综合分析该股票"
          className="inline-flex items-center gap-1.5 self-start rounded-xl bg-gradient-to-r from-indigo-500 to-violet-500 px-4 py-2 text-xs font-semibold text-white shadow-[0_0_16px_rgba(99,102,241,0.35)] transition-all duration-300 animate-ai-glow hover:scale-[1.03] hover:shadow-[0_0_24px_rgba(99,102,241,0.5)] active:scale-[0.98] disabled:cursor-not-allowed disabled:opacity-60 sm:self-auto"
        >
          {loading ? '✨ 分析中...' : resolving ? '✨ 匹配中...' : '✨ AI 分析'}
        </button>
      </div>
      {requireLogin ? (
        <div className="mt-3 rounded-xl border border-amber-400/25 bg-amber-500/8 px-4 py-3 text-sm text-amber-800 dark:text-amber-100/90">
          需要登录后才能发起 AI 分析和查看个人历史记录。
          <button type="button" onClick={onLogin} className="ml-2 font-medium text-amber-900 underline underline-offset-2 dark:text-inherit">去登录</button>
        </div>
      ) : null}
    </section>
  )
}

export function maybePromptNotification() {
  return getNotificationPermission() === 'default'
}

export function notifyAIAnalysisFinished({ symbol, symbolName, result }) {
  sendNotification(NOTIFICATION_CATEGORIES.AI_ANALYSIS, {
    symbol,
    symbolName: symbolName || symbol,
    signal: result?.analysis?.signal,
    confidenceScore: result?.analysis?.confidence_score,
  })
}
