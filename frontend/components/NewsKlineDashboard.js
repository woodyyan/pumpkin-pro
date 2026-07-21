import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import Link from 'next/link'

import { requestJson } from '../lib/api'
import { useTheme } from '../lib/theme-context'
import {
  NEWS_KLINE_DEFAULT_DAYS,
  NEWS_KLINE_DEFAULT_PAGES,
  NEWS_KLINE_REFRESH_MS,
  buildInsightText,
  buildNewsKlineUrl,
  changeClassName,
  chartPalette,
  filterKlineByRange,
  formatNumber,
  formatPercent,
  normalizeRange,
  symbolFromSearchResult,
} from '../lib/news-kline'

const RANGE_OPTIONS = [
  { key: '1M', label: '1个月' },
  { key: '3M', label: '3个月' },
  { key: '6M', label: '6个月' },
  { key: '1Y', label: '1年' },
  { key: 'ALL', label: '全部' },
]

const CATEGORY_LABELS = {
  财报业绩: '财报业绩',
  股权资本: '股权资本',
  重大事项: '重大事项',
  管理层治理: '管理层治理',
  风险监管: '风险监管',
  行业市场: '行业/市场',
  研报评级: '研报评级',
  其他: '其他',
}

function useEchart(ref, option) {
  useEffect(() => {
    if (!ref.current || !option) return undefined
    let chart
    let disposed = false
    let handleResize = null

    import('echarts').then((echarts) => {
      if (disposed || !ref.current) return
      chart = echarts.init(ref.current, null, { renderer: 'canvas' })
      chart.setOption(option, true)
      handleResize = () => chart?.resize()
      window.addEventListener('resize', handleResize)
      window.requestAnimationFrame(handleResize)
    })

    return () => {
      disposed = true
      if (handleResize) window.removeEventListener('resize', handleResize)
      if (chart) chart.dispose()
    }
  }, [ref, option])
}

function chartTooltipStyle(palette) {
  return {
    borderWidth: 1,
    borderColor: palette.tooltipBorder,
    backgroundColor: palette.tooltipBg,
    textStyle: { color: palette.tooltipText },
  }
}

function SearchBox({ query, results, loading, onQueryChange, onSelect }) {
  const wrapperRef = useRef(null)
  const [open, setOpen] = useState(false)
  const [activeIdx, setActiveIdx] = useState(-1)

  useEffect(() => {
    function handleClickOutside(event) {
      if (wrapperRef.current && !wrapperRef.current.contains(event.target)) {
        setOpen(false)
        setActiveIdx(-1)
      }
    }
    document.addEventListener('mousedown', handleClickOutside)
    return () => document.removeEventListener('mousedown', handleClickOutside)
  }, [])

  useEffect(() => {
    if (query.trim().length >= 2) setOpen(true)
  }, [query, results.length])

  function handleKeyDown(event) {
    if (!open || results.length === 0) return
    if (event.key === 'ArrowDown') {
      event.preventDefault()
      setActiveIdx((idx) => Math.min(idx + 1, results.length - 1))
    }
    if (event.key === 'ArrowUp') {
      event.preventDefault()
      setActiveIdx((idx) => Math.max(idx - 1, -1))
    }
    if (event.key === 'Enter' && activeIdx >= 0 && activeIdx < results.length) {
      event.preventDefault()
      onSelect(results[activeIdx])
      setOpen(false)
    }
    if (event.key === 'Escape') {
      setOpen(false)
      setActiveIdx(-1)
    }
  }

  return (
    <div ref={wrapperRef} className="relative w-full max-w-2xl">
      <div className="flex items-center rounded-2xl border border-border bg-card px-4 py-3 shadow-sm transition focus-within:border-primary/50">
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" className="mr-2 shrink-0 text-foreground-dim">
          <circle cx="11" cy="11" r="8" />
          <path d="M21 21l-4.35-4.35" />
        </svg>
        <input
          value={query}
          onChange={(event) => {
            onQueryChange(event.target.value)
            setActiveIdx(-1)
          }}
          onFocus={() => setOpen(query.trim().length >= 2)}
          onKeyDown={handleKeyDown}
          placeholder="搜索股票名称或代码，例如 贵州茅台 / 600519 / 腾讯"
          className="min-w-0 flex-1 bg-transparent text-sm text-foreground placeholder-foreground-disabled outline-none"
        />
        {loading && <span className="ml-2 h-4 w-4 rounded-full border border-border border-t-primary animate-spin" />}
      </div>
      {open && query.trim().length >= 2 && (
        <div className="absolute left-0 right-0 top-full z-30 mt-2 overflow-hidden rounded-2xl border border-border bg-card shadow-2xl">
          {results.length > 0 ? (
            <ul>
              {results.map((item, index) => {
                const symbol = symbolFromSearchResult(item)
                const active = activeIdx === index
                return (
                  <li key={`${item.exchange}-${item.code}`}>
                    <button
                      type="button"
                      onClick={() => {
                        onSelect(item)
                        setOpen(false)
                      }}
                      onMouseEnter={() => setActiveIdx(index)}
                      className={`w-full border-b border-border px-4 py-3 text-left transition last:border-b-0 ${active ? 'bg-primary/15' : 'hover:bg-[var(--color-bg-hover)]'}`}
                    >
                      <div className="flex items-center justify-between gap-3">
                        <div>
                          <span className="font-medium text-foreground">{item.name}</span>
                          <span className="ml-2 font-mono text-xs text-foreground-dim">{symbol}</span>
                        </div>
                        <span className="text-[11px] text-foreground-disabled">{item.exchange === 'HKEX' ? '中国香港' : 'A股'}</span>
                      </div>
                    </button>
                  </li>
                )
              })}
            </ul>
          ) : (
            <div className="px-4 py-4 text-center text-sm text-foreground-dim">未找到匹配股票</div>
          )}
        </div>
      )}
    </div>
  )
}

function StatCard({ label, value, helper, valueClass = 'text-foreground' }) {
  return (
    <div className="rounded-3xl border border-border bg-card px-5 py-5">
      <div className="text-xs font-medium uppercase tracking-[0.16em] text-foreground-dim">{label}</div>
      <div className={`mt-3 text-2xl font-semibold tabular-nums ${valueClass}`}>{value}</div>
      <div className="mt-2 text-sm leading-6 text-foreground-muted">{helper}</div>
    </div>
  )
}

function CategoryTag({ category, cats }) {
  const color = cats?.[category]?.color || '#94a3b8'
  return (
    <span className="inline-flex items-center rounded-full px-2 py-0.5 text-[11px] font-medium text-white" style={{ backgroundColor: color }}>
      {CATEGORY_LABELS[category] || category}
    </span>
  )
}

export default function NewsKlineDashboard() {
  const { resolvedTheme } = useTheme()
  const palette = useMemo(() => chartPalette(resolvedTheme), [resolvedTheme])
  const chartRef = useRef(null)
  const [query, setQuery] = useState('')
  const [searchResults, setSearchResults] = useState([])
  const [searchLoading, setSearchLoading] = useState(false)
  const [selected, setSelected] = useState(null)
  const [report, setReport] = useState(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [range, setRange] = useState('1Y')

  const selectedSymbol = selected ? symbolFromSearchResult(selected) : ''

  useEffect(() => {
    const value = query.trim()
    if (value.length < 2) {
      setSearchResults([])
      return undefined
    }
    let cancelled = false
    const timer = window.setTimeout(async () => {
      setSearchLoading(true)
      try {
        const params = new URLSearchParams({ q: value, limit: '8' })
        const data = await requestJson(`/api/search?${params.toString()}`, { cache: 'no-store' }, '股票搜索失败')
        if (!cancelled) setSearchResults(data?.results || [])
      } catch {
        if (!cancelled) setSearchResults([])
      } finally {
        if (!cancelled) setSearchLoading(false)
      }
    }, 260)
    return () => {
      cancelled = true
      window.clearTimeout(timer)
    }
  }, [query])

  const loadReport = useCallback(async (symbol, force = false) => {
    if (!symbol) return
    setLoading(true)
    setError('')
    try {
      const payload = await requestJson(
        buildNewsKlineUrl({ symbol, days: NEWS_KLINE_DEFAULT_DAYS, pages: NEWS_KLINE_DEFAULT_PAGES, force }),
        { cache: 'no-store' },
        '新闻透视数据加载失败',
      )
      setReport(payload)
    } catch (err) {
      setError(err?.message || '新闻透视数据加载失败')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    if (!selectedSymbol) return undefined
    loadReport(selectedSymbol, false)
    const timer = window.setInterval(() => loadReport(selectedSymbol, false), NEWS_KLINE_REFRESH_MS)
    return () => window.clearInterval(timer)
  }, [loadReport, selectedSymbol])

  const filteredKline = useMemo(() => filterKlineByRange(report?.KLINE, range), [report, range])
  const eventsInRange = useMemo(() => {
    const dates = new Set(filteredKline.map((item) => item.date))
    return (report?.EVENTS || []).filter((event) => dates.has(event.trade_date || event.date))
  }, [report, filteredKline])

  const chartOption = useMemo(() => {
    if (!filteredKline.length) return null
    const labels = filteredKline.map((item) => item.date)
    const closeByDate = new Map(filteredKline.map((item) => [item.date, Number(item.close)]))
    const cats = report?.CATS || {}
    const grouped = new Map()
    eventsInRange.forEach((event) => {
      const category = event.category || '其他'
      if (!grouped.has(category)) grouped.set(category, [])
      const date = event.trade_date || event.date
      grouped.get(category).push({
        name: event.title,
        value: [date, closeByDate.get(date)],
        event,
      })
    })

    const candleData = filteredKline.map((item) => [Number(item.open), Number(item.close), Number(item.low), Number(item.high)])
    const series = [
      {
        name: '前复权K线',
        type: 'candlestick',
        data: candleData,
        itemStyle: {
          color: palette.red,
          color0: palette.green,
          borderColor: palette.red,
          borderColor0: palette.green,
        },
      },
    ]
    Array.from(grouped.entries()).forEach(([category, rows]) => {
      const color = cats?.[category]?.color || palette.neutral
      series.push({
        name: CATEGORY_LABELS[category] || category,
        type: 'scatter',
        symbol: 'circle',
        symbolSize(value, params) {
          const count = rows.filter((item) => item.value[0] === params.data.value[0]).length
          return Math.min(14, 7 + count)
        },
        data: rows,
        itemStyle: { color, opacity: 0.86 },
        emphasis: { itemStyle: { borderColor: palette.tooltipText, borderWidth: 1.5, opacity: 1 } },
        tooltip: {
          formatter(params) {
            const event = params.data.event || {}
            return [
              `<strong>${event.title || '--'}</strong>`,
              `事件日期: ${event.date || '--'}`,
              `交易日: ${event.trade_date || event.date || '--'}`,
              `分类: ${event.category || '--'}`,
              `当日: ${formatPercent(event.impact?.day_change)}`,
              `后3日: ${formatPercent(event.impact?.ret_3d)}`,
            ].join('<br/>')
          },
        },
      })
    })

    return {
      backgroundColor: 'transparent',
      animationDuration: 600,
      tooltip: {
        trigger: 'axis',
        axisPointer: { type: 'cross' },
        ...chartTooltipStyle(palette),
      },
      legend: { top: 0, right: 0, textStyle: { color: palette.axis } },
      grid: { left: 58, right: 32, top: 48, bottom: 54 },
      xAxis: {
        type: 'category',
        data: labels,
        boundaryGap: true,
        axisLine: { lineStyle: { color: palette.split } },
        axisLabel: { color: palette.axis },
        splitLine: { show: false },
      },
      yAxis: {
        scale: true,
        axisLine: { lineStyle: { color: palette.split } },
        axisLabel: { color: palette.axis },
        splitLine: { lineStyle: { color: palette.split, type: 'dashed' } },
      },
      dataZoom: [{ type: 'inside' }, { type: 'slider', height: 20, bottom: 12, borderColor: palette.split, textStyle: { color: palette.axis } }],
      series,
    }
  }, [filteredKline, eventsInRange, report, palette])

  useEchart(chartRef, chartOption)

  const rangeStats = useMemo(() => {
    if (!filteredKline.length) return null
    const first = filteredKline[0]
    const last = filteredKline[filteredKline.length - 1]
    const closes = filteredKline.map((item) => Number(item.close)).filter(Number.isFinite)
    const highs = filteredKline.map((item) => Number(item.high)).filter(Number.isFinite)
    const lows = filteredKline.map((item) => Number(item.low)).filter(Number.isFinite)
    return {
      change: Number(last.close) / Number(first.close) - 1,
      high: Math.max(...highs),
      low: Math.min(...lows),
      close: closes[closes.length - 1],
    }
  }, [filteredKline])

  return (
    <main className="min-h-screen bg-background px-4 py-6 text-foreground md:px-8 lg:px-10">
      <section className="mx-auto max-w-7xl">
        <div className="rounded-[2rem] border border-border bg-card px-5 py-6 shadow-sm md:px-8 md:py-8">
          <div className="flex flex-col gap-5 lg:flex-row lg:items-end lg:justify-between">
            <div>
              <div className="text-xs font-medium uppercase tracking-[0.2em] text-primary/80">News K-line Lens</div>
              <h1 className="mt-2 text-3xl font-semibold tracking-tight text-foreground md:text-4xl">新闻透视</h1>
              <p className="mt-3 max-w-3xl text-sm leading-6 text-foreground-muted">
                将公告、新闻和财报催化剂映射到前复权 K 线，观察事件发生后的短期股价反应。A 股与中国香港股票均使用前复权口径；研报分类首期隐藏空数据。
              </p>
            </div>
            {selectedSymbol && (
              <Link href={`/live-trading/${selectedSymbol}`} className="inline-flex items-center justify-center rounded-full border border-border px-4 py-2 text-sm text-foreground-muted transition hover:border-primary/40 hover:text-primary">
                打开个股详情
              </Link>
            )}
          </div>

          <div className="mt-6 flex flex-col gap-3 lg:flex-row lg:items-center">
            <SearchBox
              query={query}
              results={searchResults}
              loading={searchLoading}
              onQueryChange={setQuery}
              onSelect={(item) => {
                setSelected(item)
                setQuery(`${item.name} ${symbolFromSearchResult(item)}`)
                setSearchResults([])
                setRange('1Y')
              }}
            />
            {selectedSymbol && (
              <button
                type="button"
                onClick={() => loadReport(selectedSymbol, true)}
                className="inline-flex items-center justify-center rounded-2xl border border-border bg-[var(--color-bg-hover)] px-4 py-3 text-sm text-foreground-muted transition hover:border-primary/40 hover:text-primary"
              >
                强制刷新
              </button>
            )}
          </div>
        </div>

        {!selectedSymbol && (
          <section className="mt-6 rounded-3xl border border-border bg-card px-5 py-12 text-center md:px-8">
            <div className="text-xs font-medium uppercase tracking-[0.18em] text-foreground-dim">Search first</div>
            <h2 className="mt-2 text-2xl font-semibold text-foreground">先搜索一只股票开始透视</h2>
            <p className="mx-auto mt-3 max-w-2xl text-sm leading-6 text-foreground-muted">
              当前页面不会默认加载个股，避免无意义外部请求。请选择股票后，系统会拉取 500 个交易日 K 线和更深覆盖的公告/新闻事件。
            </p>
          </section>
        )}

        {selectedSymbol && (
          <>
            {error && (
              <div className="mt-6 rounded-2xl border border-negative/20 bg-negative/5 px-4 py-3 text-sm text-negative">{error}</div>
            )}

            <section className="mt-6 grid gap-4 md:grid-cols-2 xl:grid-cols-4">
              <StatCard label="当前股票" value={selected?.name || report?.META?.symbol || selectedSymbol} helper={`${selectedSymbol} · ${report?.META?.exchange === 'HKEX' ? '中国香港股票' : 'A股股票'}`} />
              <StatCard label="区间涨跌幅" value={rangeStats ? formatPercent(rangeStats.change) : loading ? '加载中' : '--'} helper="按当前区间首尾收盘价计算" valueClass={rangeStats ? changeClassName(rangeStats.change) : 'text-foreground'} />
              <StatCard label="区间最高 / 最低" value={rangeStats ? `${formatNumber(rangeStats.high)} / ${formatNumber(rangeStats.low)}` : '--'} helper="前复权价格口径" />
              <StatCard label="事件样本" value={report?.META?.n_events ?? '--'} helper={`缓存 TTL 30 分钟 · ${report?.META?.cache_status || '待加载'}`} />
            </section>

            <section className="mt-6 rounded-3xl border border-border bg-card px-5 py-5 md:px-6">
              <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
                <div>
                  <h2 className="text-xl font-semibold text-foreground">股价走势与催化剂时间轴</h2>
                  <p className="mt-1 text-sm leading-6 text-foreground-muted">
                    蜡烛图为前复权日 K；圆点为事件标记，颜色代表事件类型。公告若落在非交易日，会映射到下一交易日参与影响计算。
                  </p>
                </div>
                <div className="flex flex-wrap gap-2">
                  {RANGE_OPTIONS.map((item) => (
                    <button
                      key={item.key}
                      type="button"
                      onClick={() => setRange(normalizeRange(item.key))}
                      className={`rounded-full border px-3 py-1.5 text-xs transition ${range === item.key ? 'border-primary bg-primary/10 text-primary' : 'border-border text-foreground-muted hover:border-primary/40 hover:text-primary'}`}
                    >
                      {item.label}
                    </button>
                  ))}
                </div>
              </div>
              <div className="mt-5 h-[440px] w-full">
                {chartOption ? <div ref={chartRef} className="h-full w-full" /> : <div className="flex h-full items-center justify-center text-sm text-foreground-dim">{loading ? '正在加载新闻透视数据…' : '暂无可展示图表数据'}</div>}
              </div>
              <div className="mt-4 flex flex-wrap gap-3 text-xs text-foreground-muted">
                {Object.entries(report?.CATS || {}).map(([category, meta]) => (
                  <span key={category} className="inline-flex items-center gap-1.5">
                    <span className="h-2.5 w-2.5 rounded-full" style={{ backgroundColor: meta.color }} />
                    {meta.label || CATEGORY_LABELS[category] || category}
                  </span>
                ))}
              </div>
            </section>

            <section className="mt-6 grid gap-6 xl:grid-cols-[0.9fr_1.1fr]">
              <div className="rounded-3xl border border-border bg-card px-5 py-5 md:px-6">
                <h2 className="text-xl font-semibold text-foreground">事件影响解释力排行</h2>
                <p className="mt-1 text-sm leading-6 text-foreground-muted">按「平均 3 日绝对收益 × 方向稳定性」排序。空分类自动隐藏。</p>
                <div className="mt-4 overflow-hidden rounded-2xl border border-border/80">
                  <table className="w-full text-sm">
                    <thead className="bg-[var(--color-bg-hover)] text-xs text-foreground-dim">
                      <tr>
                        <th className="px-3 py-3 text-left font-medium">类型</th>
                        <th className="px-3 py-3 text-right font-medium">数量</th>
                        <th className="px-3 py-3 text-right font-medium">得分</th>
                        <th className="px-3 py-3 text-right font-medium">后3日</th>
                        <th className="px-3 py-3 text-right font-medium">胜率</th>
                      </tr>
                    </thead>
                    <tbody className="divide-y divide-border/70">
                      {(report?.STATS || []).map((item) => (
                        <tr key={item.category}>
                          <td className="px-3 py-3"><CategoryTag category={item.category} cats={report?.CATS} /></td>
                          <td className="px-3 py-3 text-right tabular-nums text-foreground-muted">{item.count}</td>
                          <td className="px-3 py-3 text-right tabular-nums text-foreground">{item.explain_score == null ? '--' : formatNumber(item.explain_score * 100, 1)}</td>
                          <td className={`px-3 py-3 text-right tabular-nums ${changeClassName(item.avg_3d)}`}>{formatPercent(item.avg_3d)}</td>
                          <td className="px-3 py-3 text-right tabular-nums text-foreground-muted">{formatPercent(item.win_3d, 0)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
                <div className="mt-4 rounded-2xl border border-primary/20 bg-primary/5 px-4 py-3 text-sm leading-6 text-foreground-muted">
                  {report ? buildInsightText({ ...report, META: { ...report.META, name: selected?.name || report.META?.name } }) : '选择股票后生成解释力摘要。'}
                </div>
              </div>

              <div className="rounded-3xl border border-border bg-card px-5 py-5 md:px-6">
                <h2 className="text-xl font-semibold text-foreground">事件明细</h2>
                <p className="mt-1 text-sm leading-6 text-foreground-muted">按事件交易日倒序展示，收益均基于前复权 K 线计算。</p>
                <div className="mt-4 max-h-[520px] overflow-auto rounded-2xl border border-border/80">
                  <table className="w-full min-w-[760px] text-sm">
                    <thead className="sticky top-0 bg-[var(--color-bg-hover)] text-xs text-foreground-dim">
                      <tr>
                        <th className="px-3 py-3 text-left font-medium">事件日</th>
                        <th className="px-3 py-3 text-left font-medium">分类</th>
                        <th className="px-3 py-3 text-left font-medium">标题</th>
                        <th className="px-3 py-3 text-right font-medium">当日</th>
                        <th className="px-3 py-3 text-right font-medium">后1日</th>
                        <th className="px-3 py-3 text-right font-medium">后3日</th>
                      </tr>
                    </thead>
                    <tbody className="divide-y divide-border/70">
                      {(report?.EVENTS || []).map((event) => (
                        <tr key={`${event.trade_date}-${event.id}`} className="hover:bg-[var(--color-bg-hover)]">
                          <td className="whitespace-nowrap px-3 py-3 text-foreground-muted">
                            <div>{event.date}</div>
                            {event.trade_date !== event.date && <div className="text-[11px] text-foreground-disabled">映射 {event.trade_date}</div>}
                          </td>
                          <td className="px-3 py-3"><CategoryTag category={event.category} cats={report?.CATS} /></td>
                          <td className="max-w-[360px] px-3 py-3 text-foreground">
                            <div className="truncate" title={event.title}>{event.title}</div>
                            <div className="mt-1 text-xs text-foreground-dim">{event.info_type_str || '资讯'} · {event.src || '来源未标注'}</div>
                          </td>
                          <td className={`px-3 py-3 text-right tabular-nums ${changeClassName(event.impact?.day_change)}`}>{formatPercent(event.impact?.day_change)}</td>
                          <td className={`px-3 py-3 text-right tabular-nums ${changeClassName(event.impact?.ret_1d)}`}>{formatPercent(event.impact?.ret_1d)}</td>
                          <td className={`px-3 py-3 text-right tabular-nums ${changeClassName(event.impact?.ret_3d)}`}>{formatPercent(event.impact?.ret_3d)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>
            </section>

            <section className="mt-6 rounded-3xl border border-border bg-card px-5 py-4 text-xs leading-6 text-foreground-muted md:px-6">
              <strong className="text-foreground">口径说明：</strong>
              K 线为腾讯公开行情前复权日 K；事件来源首期包括公告、新闻和财报类披露，不包含研报；事件分类基于标题关键词规则，仅用于辅助观察，不构成投资建议。数据刷新失败时会优先返回最近一次缓存快照。
            </section>
          </>
        )}
      </section>
    </main>
  )
}
