import Head from 'next/head'
import { useCallback, useEffect, useMemo, useState } from 'react'

import { fetchAIPickerMeta, fetchDailyAIPicks, formatPct, formatPrice, convictionTone } from '../../lib/ai-picker'
import { markMarketLoadAttempted, shouldAutoLoadAIPickerMarket } from '../../lib/ai-picker-page-state'

const MARKETS = [
  { key: 'ASHARE', label: 'A股' },
  { key: 'HKEX', label: '港股' },
]

export default function AIPickerPage() {
  const [market, setMarket] = useState('ASHARE')
  const [attemptedByMarket, setAttemptedByMarket] = useState({})
  const [metaByMarket, setMetaByMarket] = useState({})
  const [resultByMarket, setResultByMarket] = useState({})
  const [loadingByMarket, setLoadingByMarket] = useState({})
  const [errorByMarket, setErrorByMarket] = useState({})

  const loadPage = useCallback(async (activeMarket) => {
    setAttemptedByMarket((prev) => markMarketLoadAttempted(prev, activeMarket))
    setLoadingByMarket((prev) => ({ ...prev, [activeMarket]: true }))
    setErrorByMarket((prev) => ({ ...prev, [activeMarket]: '' }))
    try {
      const nextMeta = await fetchAIPickerMeta(activeMarket)
      setMetaByMarket((prev) => ({ ...prev, [activeMarket]: nextMeta }))
      if (activeMarket === 'ASHARE' && nextMeta?.available) {
        try {
          const daily = await fetchDailyAIPicks(activeMarket)
          setResultByMarket((prev) => ({ ...prev, [activeMarket]: daily }))
        } catch (err) {
          setErrorByMarket((prev) => ({ ...prev, [activeMarket]: err.message || '今日 AI 选股尚未生成' }))
        }
      }
    } catch (err) {
      setMetaByMarket((prev) => ({ ...prev, [activeMarket]: null }))
      setErrorByMarket((prev) => ({ ...prev, [activeMarket]: err.message || '页面加载失败' }))
    } finally {
      setLoadingByMarket((prev) => ({ ...prev, [activeMarket]: false }))
    }
  }, [])

  useEffect(() => {
    if (shouldAutoLoadAIPickerMarket({ market, attemptedByMarket, loadingByMarket })) {
      loadPage(market)
    }
  }, [attemptedByMarket, loadPage, loadingByMarket, market])

  const meta = metaByMarket[market] || null
  const result = resultByMarket[market] || null
  const loading = !attemptedByMarket[market] || Boolean(loadingByMarket[market])
  const error = errorByMarket[market] || ''
  const analysis = result?.analysis || null
  const picks = analysis?.picks || []
  const allocation = analysis?.portfolio_allocation || null
  const candidatePoolSize = result?.meta?.candidate_pool_size
  const generatedAt = result?.meta?.generated_at

  const summaryChips = useMemo(() => {
    if (!allocation) return []
    return [
      `总仓位 ${formatPct(allocation.total_position_pct)}`,
      `现金 ${formatPct(allocation.cash_reserve_pct)}`,
      allocation.expected_style || '均衡风格',
    ]
  }, [allocation])

  return (
    <>
      <Head>
        <title>AI选股 - 卧龙AI</title>
        <meta name="description" content="每日自动生成 A 股 AI 优选组合，默认展示全站共享的最新结果。" />
      </Head>
      <main className="mx-auto flex w-full max-w-6xl flex-col gap-5 px-4 py-6 sm:px-6 lg:px-8">
        <section className="flex items-start gap-3 rounded-2xl border border-amber-500/30 bg-amber-500/10 px-4 py-4 text-sm text-amber-800 dark:text-amber-200">
          <span className="mt-0.5 shrink-0 text-base leading-none">⚠️</span>
          <div>
            <span className="font-semibold">内测公告：</span>
            当前选股结果仅供内部测试，<span className="font-medium">数据不可靠，请勿参考</span>。正式版本上线后我们将第一时间通知，感谢您的耐心等待。
          </div>
        </section>
        <section className="overflow-hidden rounded-[28px] border border-primary/20 bg-gradient-to-br from-primary/20 via-card to-card px-5 py-5 shadow-card sm:px-6">
          <div className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
            <div>
              <div className="inline-flex rounded-full border border-primary/20 bg-primary/10 px-3 py-1 text-xs font-medium text-primary">AI 选股</div>
              <h1 className="mt-3 text-2xl font-semibold tracking-tight">今日 AI 优选组合</h1>
            </div>
            <div className="flex flex-wrap items-center gap-2">
              {MARKETS.map((item) => (
                <button
                  key={item.key}
                  type="button"
                  onClick={() => setMarket(item.key)}
                  className={`rounded-full px-4 py-2 text-sm transition ${market === item.key ? 'bg-primary text-black' : 'border border-border bg-card text-foreground-dim hover:border-primary/30 hover:text-foreground'}`}
                >
                  {item.label}
                </button>
              ))}
            </div>
          </div>

          <div className="mt-4 flex flex-wrap gap-2 text-xs text-foreground-dim">
            <span className="rounded-full border border-border bg-card px-3 py-1">市场：{market === 'ASHARE' ? 'A股' : '港股'}</span>
            {candidatePoolSize ? <span className="rounded-full border border-border bg-card px-3 py-1">候选池：{candidatePoolSize} 只</span> : null}
            {generatedAt ? <span className="rounded-full border border-border bg-card px-3 py-1">生成：{formatBeijingTime(generatedAt)}</span> : null}
          </div>
        </section>

        {loading ? (
          <section className="rounded-2xl border border-border bg-card p-6 text-sm text-foreground-dim">正在加载今日 AI 选股...</section>
        ) : market === 'HKEX' ? (
          <section className="rounded-2xl border border-border bg-card p-8 text-center">
            <h2 className="text-lg font-medium text-foreground">港股 AI 选股即将上线</h2>

          </section>
        ) : !meta?.available ? (
          <section className="rounded-2xl border border-border bg-card p-8 text-center">
            <h2 className="text-lg font-medium text-foreground">因子数据未就绪</h2>
            <p className="mt-2 text-sm text-foreground-dim">{meta?.reason || '请等待每日因子计算完成后再查看 AI 选股结果。'}</p>
          </section>
        ) : error && !analysis ? (
          <section className="rounded-2xl border border-negative/35 bg-negative/10 px-4 py-4 text-sm text-negative">
            <div>{error}</div>
          </section>
        ) : analysis ? (
          <>
            <section className="grid gap-4 lg:grid-cols-[1.35fr_.8fr]">
              <div className="rounded-2xl border border-border bg-card p-5">
                <div className="flex items-center gap-2 text-xs text-primary">
                  <span className="rounded-full bg-primary/10 px-2.5 py-1">市场观点</span>
                  <span className="rounded-full bg-[var(--color-bg-hover)] px-2.5 py-1 text-foreground-dim">{analysis.selection_basis}</span>
                  <span className="rounded-full bg-[var(--color-bg-hover)] px-2.5 py-1 text-foreground-dim">{analysis.trigger}</span>
                </div>
                <h2 className="mt-3 text-lg font-medium">{analysis.market_view || '今日市场环境'}</h2>
                <p className="mt-3 text-sm leading-7 text-foreground-dim">{analysis.strategy_summary}</p>
                {allocation ? (
                  <div className="mt-4 flex flex-wrap gap-2">
                    {summaryChips.map((chip) => (
                      <span key={chip} className="rounded-full border border-border bg-[var(--color-bg-hover)] px-3 py-1 text-xs text-foreground-dim">{chip}</span>
                    ))}
                  </div>
                ) : null}
              </div>
              <div className="rounded-2xl border border-border bg-card p-5">
                <h2 className="text-base font-medium">组合概览</h2>
                <div className="mt-4 grid grid-cols-2 gap-3 text-sm">
                  <MetricCard label="总仓位" value={formatPct(allocation?.total_position_pct)} />
                  <MetricCard label="现金预留" value={formatPct(allocation?.cash_reserve_pct)} />
                  <MetricCard label="持仓数" value={picks.length ? `${picks.length} 只` : '--'} />
                  <MetricCard label="风格" value={allocation?.expected_style || '--'} />
                </div>
                <div className="mt-4 rounded-2xl bg-[var(--color-bg-hover)] p-3 text-sm text-foreground-dim">{allocation?.diversification_note || '保持行业分散与现金预留。'}</div>
              </div>
            </section>

            <section className="grid gap-4 lg:grid-cols-2">
              {picks.map((pick) => (
                <article key={`${pick.code}-${pick.rank}`} className="rounded-2xl border border-border bg-card p-5 shadow-card">
                  <div className="flex items-start justify-between gap-3">
                    <div>
                      <div className="flex items-center gap-2">
                        <span className="inline-flex size-7 items-center justify-center rounded-full bg-primary text-xs font-semibold text-black">#{pick.rank}</span>
                        <h3 className="text-lg font-medium text-foreground">{pick.name}</h3>
                        <span className="text-xs text-foreground-dim">{pick.code}</span>
                      </div>
                      <div className="mt-2 flex flex-wrap gap-2 text-xs text-foreground-dim">
                        <span className="rounded-full border border-border px-2.5 py-1">{pick.industry || '未知行业'}</span>
                        <span className={`rounded-full border px-2.5 py-1 ${convictionTone(pick.conviction)}`}>信心 {pick.conviction_score}</span>
                        <span className="rounded-full border border-border px-2.5 py-1">{pick.time_horizon}</span>
                      </div>
                    </div>
                    <div className="text-right">
                      <div className="text-3xl font-semibold text-primary">{pick.position_pct}%</div>
                      <div className="mt-1 text-xs text-foreground-dim">建议持仓占比</div>
                    </div>
                  </div>

                  <div className="mt-4 grid grid-cols-2 gap-3 sm:grid-cols-4">
                    <MetricCard label="现价" value={formatPrice(pick.current_price, pick.currency)} />
                    <MetricCard label="买入区间" value={`${formatPrice(pick.entry_zone?.low, pick.currency)} ~ ${formatPrice(pick.entry_zone?.high, pick.currency)}`} />
                    <MetricCard label="止损" value={formatPrice(pick.stop_loss?.price, pick.currency)} />
                    <MetricCard label="止盈" value={formatPrice(pick.take_profit?.price, pick.currency)} />
                  </div>

                  <div className="mt-4 rounded-2xl bg-[var(--color-bg-hover)] p-4 text-sm leading-7 text-foreground-dim">{pick.reason}</div>

                  {Array.isArray(pick.factor_highlights) && pick.factor_highlights.length ? (
                    <div className="mt-4 flex flex-wrap gap-2">
                      {pick.factor_highlights.map((item) => (
                        <span key={`${pick.code}-${item.key}`} className="rounded-full border border-primary/20 bg-primary/10 px-3 py-1 text-xs text-primary">
                          {item.label} {item.score === null || item.score === undefined ? '--' : formatScore(item.score)}
                        </span>
                      ))}
                      {pick.composite_score !== null && pick.composite_score !== undefined ? (
                        <span className="rounded-full border border-border px-3 py-1 text-xs text-foreground-dim">综合 {formatScore(pick.composite_score)}</span>
                      ) : null}
                    </div>
                  ) : null}

                  <div className="mt-4 rounded-xl border border-amber-500/20 bg-amber-500/10 px-3 py-2 text-xs text-amber-800 dark:text-amber-200">风险提示：{pick.risk_note}</div>
                </article>
              ))}
            </section>

            <section className="grid gap-4 lg:grid-cols-[1fr_1fr]">
              <div className="rounded-2xl border border-border bg-card p-5">
                <h2 className="text-base font-medium">组合关键风险</h2>
                <ul className="mt-3 space-y-2 text-sm text-foreground-dim">
                  {(analysis.key_risks || []).map((item, idx) => <li key={`${item}-${idx}`}>• {item}</li>)}
                </ul>
              </div>
              <div className="rounded-2xl border border-border bg-card p-5">
                <h2 className="text-base font-medium">免责声明</h2>
                <p className="mt-3 text-sm leading-7 text-foreground-dim">{analysis.disclaimer}</p>
                {error ? <p className="mt-3 text-xs text-amber-300">提示：{error}</p> : null}
              </div>
            </section>
          </>
        ) : (
          <section className="rounded-2xl border border-dashed border-border bg-card px-4 py-8 text-center text-sm text-foreground-dim">
            今日 AI 选股结果正在生成中，请稍后刷新查看。
          </section>
        )}
      </main>
    </>
  )
}

function MetricCard({ label, value }) {
  return (
    <div className="rounded-2xl border border-border bg-[var(--color-bg-hover)] px-3 py-3">
      <div className="text-xs text-foreground-dim">{label}</div>
      <div className="mt-1 text-sm font-medium text-foreground">{value || '--'}</div>
    </div>
  )
}

function formatScore(value) {
  const num = Number(value)
  if (!Number.isFinite(num)) return '--'
  return num.toFixed(1)
}

function formatBeijingTime(value) {
  if (!value) return '--'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return String(value)
  const parts = new Intl.DateTimeFormat('zh-CN', {
    timeZone: 'Asia/Shanghai',
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  }).formatToParts(date).reduce((acc, part) => {
    acc[part.type] = part.value
    return acc
  }, {})
  return `${parts.year}-${parts.month}-${parts.day} ${parts.hour}:${parts.minute}:${parts.second}`
}
