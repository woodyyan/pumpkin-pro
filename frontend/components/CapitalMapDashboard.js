import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import Link from 'next/link'

import { requestJson } from '../lib/api'
import {
  CAPITAL_MAP_REFRESH_MS,
  buildCapitalMapUrl,
  buildStockDetailHref,
  changeClassName,
  chartPalette,
  formatBeijingTime,
  formatCompactNumber,
  formatNumber,
  formatPercent,
} from '../lib/capital-map'
import { useTheme } from '../lib/theme-context'

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

function inlineChangeColor(value, palette) {
  const number = Number(value)
  if (number > 0) return palette.red
  if (number < 0) return palette.green
  return palette.neutral
}

function StatCard({ label, value, helper, accent = 'default' }) {
  const accentClass = {
    default: 'border-border bg-card',
    gold: 'border-primary/30 bg-[var(--color-bg-hover)]',
    redgreen: 'border-border bg-card',
    blue: 'border-border bg-[var(--color-bg-hover)]',
  }[accent] || 'border-border bg-card'

  return (
    <div className={`rounded-3xl border px-5 py-5 ${accentClass}`}>
      <div className="text-xs font-medium uppercase tracking-[0.16em] text-foreground-dim">{label}</div>
      <div className="mt-3 text-2xl font-semibold tabular-nums text-foreground">{value}</div>
      <div className="mt-2 text-sm leading-6 text-foreground-muted">{helper}</div>
    </div>
  )
}

function EmptyState({ loading }) {
  return (
    <section className="rounded-3xl border border-border bg-card px-5 py-10 text-center md:px-8">
      <div className="text-xs font-medium uppercase tracking-[0.18em] text-foreground-dim">A-share capital map</div>
      <h1 className="mt-2 text-2xl font-semibold tracking-tight text-foreground md:text-3xl">资金星图正在接入行情源</h1>
      <p className="mx-auto mt-3 max-w-2xl text-sm leading-6 text-foreground-muted">
        {loading ? '正在计算高流动性样本 PE 分布、板块成交额占比和 PoC 估值锚。' : '暂无可展示数据，请稍后自动重试。'}
      </p>
    </section>
  )
}

function DataTable({ columns, children }) {
  return (
    <div className="overflow-hidden rounded-2xl border border-border/80">
      <div className="grid grid-cols-4 gap-3 bg-[var(--color-bg-hover)] px-4 py-3 text-xs font-medium text-foreground-dim">
        {columns.map((column) => <span key={column}>{column}</span>)}
      </div>
      <div className="divide-y divide-border/70">{children}</div>
    </div>
  )
}

export default function CapitalMapDashboard() {
  const { resolvedTheme } = useTheme()
  const palette = useMemo(() => chartPalette(resolvedTheme), [resolvedTheme])
  const [data, setData] = useState(null)
  const [loading, setLoading] = useState(true)
  const [lastLoadedAt, setLastLoadedAt] = useState(null)

  const galaxyRef = useRef(null)
  const sectorRef = useRef(null)
  const pocRef = useRef(null)

  const loadData = useCallback(async (silent = false) => {
    if (!silent) setLoading(true)

    try {
      const payload = await requestJson(buildCapitalMapUrl(), { cache: 'no-store' }, '资金星图数据加载失败')
      setData(payload)
      setLastLoadedAt(new Date().toISOString())
    } catch {
      // 刷新失败由后端记录日志；前端保留已有快照，不打扰用户。
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    loadData(false)
    const timer = window.setInterval(() => loadData(true), CAPITAL_MAP_REFRESH_MS)
    return () => window.clearInterval(timer)
  }, [loadData])

  const galaxyOption = useMemo(() => {
    if (!data?.stocks?.length) return null
    const scatterData = data.stocks.map((stock) => [
      stock.pe,
      stock.amountYi,
      stock.pctChg,
      stock.name,
      stock.code,
      stock.peSource,
      stock.turnoverRate,
      stock.totalMarketCapYi,
      stock.mainNetInflowYi,
    ])

    return {
      backgroundColor: 'transparent',
      animationDuration: 700,
      tooltip: {
        ...chartTooltipStyle(palette),
        formatter(params) {
          const d = params.data
          return [
            `<strong>${d[3]} ${d[4]}</strong>`,
            `${d[5]}: ${Number(d[0]).toFixed(2)} 倍`,
            `成交额: ${formatCompactNumber(d[1])} 亿`,
            `涨跌幅: <span style="color:${inlineChangeColor(d[2], palette)}">${formatPercent(d[2])}</span>`,
            `换手率: ${d[6] ?? '--'}%`,
            `总市值: ${formatCompactNumber(d[7] || 0)} 亿`,
            `主力净流入: ${formatCompactNumber(d[8] || 0)} 亿`,
          ].join('<br/>')
        },
      },
      grid: { left: 58, right: 34, top: 28, bottom: 54 },
      xAxis: {
        name: 'PE',
        nameTextStyle: { color: palette.axis },
        axisLine: { lineStyle: { color: palette.split } },
        axisLabel: { color: palette.axis },
        splitLine: { lineStyle: { color: palette.split, type: 'dashed' } },
      },
      yAxis: {
        name: '成交额/亿',
        nameTextStyle: { color: palette.axis },
        axisLine: { lineStyle: { color: palette.split } },
        axisLabel: { color: palette.axis },
        splitLine: { lineStyle: { color: palette.split, type: 'dashed' } },
      },
      visualMap: {
        min: -10,
        max: 10,
        dimension: 2,
        orient: 'horizontal',
        left: 20,
        bottom: 4,
        calculable: true,
        inRange: { color: [palette.green, palette.neutral, palette.red] },
        text: ['涨', '跌'],
        textStyle: { color: palette.axis },
      },
      series: [{
        type: 'scatter',
        data: scatterData,
        symbolSize(value) {
          return Math.max(7, Math.min(36, 6 + Math.sqrt(value[1]) * 2.1))
        },
        itemStyle: { opacity: 0.82 },
        emphasis: { focus: 'self', itemStyle: { borderColor: palette.gold, borderWidth: 1.5, opacity: 1 } },
      }],
    }
  }, [data, palette])

  const sectorOption = useMemo(() => {
    if (!data?.sectors?.length) return null
    return {
      backgroundColor: 'transparent',
      tooltip: {
        trigger: 'axis',
        axisPointer: { type: 'shadow' },
        ...chartTooltipStyle(palette),
        formatter(params) {
          const sector = data.sectors[params[0].dataIndex]
          return [
            `<strong>${sector.name}</strong>`,
            `样本成交额占比: ${sector.amountRatio ?? '--'}%`,
            `成交额: ${formatCompactNumber(sector.amountYi)} 亿`,
            `主力净流入: <span style="color:${inlineChangeColor(sector.mainNetInflowYi, palette)}">${formatCompactNumber(sector.mainNetInflowYi)} 亿</span>`,
            `净流强度: ${sector.netInflowIntensity ?? '--'}%`,
            `代表股: ${sector.leaderName || '--'}`,
          ].join('<br/>')
        },
      },
      legend: { top: 0, right: 0, textStyle: { color: palette.axis } },
      grid: { left: 48, right: 44, top: 42, bottom: 72 },
      xAxis: {
        type: 'category',
        data: data.sectors.map((sector) => sector.name),
        axisLabel: { color: palette.axis, rotate: 34, interval: 0, fontSize: 11 },
        axisLine: { lineStyle: { color: palette.split } },
      },
      yAxis: [
        {
          type: 'value',
          name: '样本占比',
          axisLabel: { color: palette.axis, formatter: '{value}%' },
          nameTextStyle: { color: palette.axis },
          splitLine: { lineStyle: { color: palette.split, type: 'dashed' } },
        },
        {
          type: 'value',
          name: '净流强度',
          axisLabel: { color: palette.axis, formatter: '{value}%' },
          nameTextStyle: { color: palette.axis },
          splitLine: { show: false },
        },
      ],
      series: [
        {
          name: '样本占比',
          type: 'bar',
          data: data.sectors.map((sector) => sector.amountRatio),
          barWidth: '42%',
          itemStyle: { borderRadius: [8, 8, 2, 2], color: palette.gold },
        },
        {
          name: '主力净流强度',
          type: 'line',
          yAxisIndex: 1,
          smooth: true,
          symbolSize: 8,
          data: data.sectors.map((sector) => sector.netInflowIntensity),
          lineStyle: { color: palette.blue, width: 2 },
          itemStyle: { color: palette.blue },
        },
      ],
    }
  }, [data, palette])

  const pocOption = useMemo(() => {
    if (!data?.pocDistribution?.length) return null
    return {
      backgroundColor: 'transparent',
      tooltip: {
        trigger: 'axis',
        axisPointer: { type: 'shadow' },
        ...chartTooltipStyle(palette),
        formatter(params) {
          const bin = data.pocDistribution[params[0].dataIndex]
          const leaders = bin.topStocks.slice(0, 4).map((stock) => `${stock.name} ${stock.amountYi}亿`).join('、')
          return [
            `<strong>PE ${bin.key} 倍</strong>`,
            `成交额: ${formatCompactNumber(bin.totalAmountYi)} 亿`,
            `股票数: ${bin.stockCount} 只`,
            `平均涨跌幅: ${formatPercent(bin.avgPctChg)}`,
            `活跃股: ${leaders || '--'}`,
          ].join('<br/>')
        },
      },
      grid: { left: 58, right: 22, top: 24, bottom: 54 },
      xAxis: {
        type: 'category',
        data: data.pocDistribution.map((bin) => bin.key),
        axisLabel: { color: palette.axis, rotate: 30, interval: 0, fontSize: 11 },
        axisLine: { lineStyle: { color: palette.split } },
      },
      yAxis: {
        name: '成交额/亿',
        nameTextStyle: { color: palette.axis },
        axisLabel: { color: palette.axis },
        splitLine: { lineStyle: { color: palette.split, type: 'dashed' } },
      },
      series: [{
        type: 'bar',
        data: data.pocDistribution.map((bin) => ({
          value: bin.totalAmountYi,
          itemStyle: {
            color: data.poc?.key === bin.key ? palette.gold : palette.bar,
            borderColor: data.poc?.key === bin.key ? palette.gold : palette.barBorder,
            borderWidth: data.poc?.key === bin.key ? 1 : 0,
          },
        })),
        barWidth: '56%',
      }],
    }
  }, [data, palette])

  useEchart(galaxyRef, galaxyOption)
  useEchart(sectorRef, sectorOption)
  useEchart(pocRef, pocOption)

  const topStocks = useMemo(() => {
    if (!data?.stocks) return []
    return data.stocks.slice().sort((a, b) => Number(b.amountYi || 0) - Number(a.amountYi || 0)).slice(0, 10)
  }, [data])

  if (!data) {
    return <EmptyState loading={loading} />
  }

  return (
    <div className="space-y-6">
      <section className="overflow-hidden rounded-3xl border border-border bg-card px-5 py-5 md:px-6 md:py-6">
        <div className="flex flex-col gap-5 lg:flex-row lg:items-end lg:justify-between">
          <div className="max-w-3xl">
            <div className="text-xs font-medium uppercase tracking-[0.18em] text-foreground-dim">A-share capital map</div>
            <h1 className="mt-2 text-2xl font-semibold tracking-tight text-foreground md:text-3xl">资金星图</h1>
            <p className="mt-3 text-sm leading-6 text-foreground-muted">
              横轴为 PE，纵轴为成交额，颜色遵循 A 股红涨绿跌，并实时计算高流动性样本内的板块成交额占比与 PoC 估值锚。
            </p>
            <div className="mt-4 flex flex-wrap gap-2 text-xs text-foreground-dim">
              <span className="rounded-full border border-border bg-[var(--color-bg-hover)] px-3 py-1.5">样本：{data?.sampleScope || '--'}</span>
              <span className="rounded-full border border-border bg-[var(--color-bg-hover)] px-3 py-1.5">刷新：{data?.refreshHintSeconds || 60} 秒</span>
              <span className="rounded-full border border-border bg-[var(--color-bg-hover)] px-3 py-1.5">快照：{formatBeijingTime(data?.updatedAt)}</span>
            </div>
          </div>
          <div className="rounded-2xl border border-border bg-[var(--color-bg-hover)] px-4 py-4 text-sm text-foreground-muted lg:min-w-[220px]">
            <div className="text-xs font-medium uppercase tracking-[0.16em] text-foreground-dim">Last loaded</div>
            <div className="mt-2 text-sm leading-6 text-foreground-muted">{formatBeijingTime(lastLoadedAt)}</div>
          </div>
        </div>
      </section>

      <section className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <StatCard label="全市场股票" value={`${formatNumber(data?.market?.stockCount || 0)} 只`} helper={`当前抓取样本 ${formatNumber(data?.market?.sampleCount || 0)} 只，有效 PE ${formatNumber(data?.market?.positivePeCount || 0)} 只`} />
        <StatCard label="样本成交额" value={`${formatCompactNumber(data?.market?.totalAmountYi || 0)} 亿`} helper="成交额前排股票样本合计" accent="gold" />
        <StatCard label="上涨 / 下跌" value={`${formatNumber(data?.market?.upCount || 0)} / ${formatNumber(data?.market?.downCount || 0)}`} helper={`上涨占比 ${data?.market?.upRatio ?? '--'}%`} accent="redgreen" />
        <StatCard label="PoC 估值锚" value={data?.poc ? `PE ${data.poc.key}x` : '--'} helper={data?.poc ? `${formatCompactNumber(data.poc.totalAmountYi)} 亿成交额聚集` : '等待计算'} accent="blue" />
      </section>

      <section className="rounded-3xl border border-border bg-card px-5 py-5">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <div className="text-xs font-medium uppercase tracking-[0.16em] text-foreground-dim">Valuation x liquidity</div>
            <h2 className="mt-1 text-xl font-semibold tracking-tight text-foreground">个股估值星图</h2>
          </div>
          <span className="rounded-full border border-border bg-[var(--color-bg-hover)] px-3 py-1.5 text-xs text-foreground-dim">展示成交额前 {formatNumber(data?.market?.chartStockCount || 0)} 只有效 PE 股票</span>
        </div>
        <div ref={galaxyRef} className="mt-4 h-[430px] w-full md:h-[560px]" aria-label="个股估值星图" />
      </section>

      <section className="grid gap-4 xl:grid-cols-2">
        <div className="rounded-3xl border border-border bg-card px-5 py-5">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <div className="text-xs font-medium uppercase tracking-[0.16em] text-foreground-dim">Sector flow</div>
              <h2 className="mt-1 text-xl font-semibold tracking-tight text-foreground">行业板块资金</h2>
            </div>
            <span className="rounded-full border border-border bg-[var(--color-bg-hover)] px-3 py-1.5 text-xs text-foreground-dim">成交额 Top 行业</span>
          </div>
          <div ref={sectorRef} className="mt-4 h-[380px] w-full" aria-label="行业板块资金图" />
        </div>

        <div className="rounded-3xl border border-border bg-card px-5 py-5">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <div className="text-xs font-medium uppercase tracking-[0.16em] text-foreground-dim">Point of control</div>
              <h2 className="mt-1 text-xl font-semibold tracking-tight text-foreground">PE 成交额分布</h2>
            </div>
            <span className="rounded-full border border-border bg-[var(--color-bg-hover)] px-3 py-1.5 text-xs text-foreground-dim">5 倍 PE 分箱</span>
          </div>
          <div ref={pocRef} className="mt-4 h-[320px] w-full" aria-label="PE 成交额分布图" />
          {data?.poc && (
            <div className="mt-3 flex flex-wrap gap-2 rounded-2xl border border-border/80 bg-[var(--color-bg-hover)] px-4 py-3 text-xs text-foreground-muted">
              <span className="font-medium text-foreground">PoC 活跃股</span>
              {data.poc.topStocks.slice(0, 5).map((stock) => <span key={stock.code}>{stock.name} {stock.amountYi}亿</span>)}
            </div>
          )}
        </div>
      </section>

      <section className="grid gap-4 xl:grid-cols-2">
        <div className="rounded-3xl border border-border bg-card px-5 py-5">
          <div className="mb-4">
            <div className="text-xs font-medium uppercase tracking-[0.16em] text-foreground-dim">Liquidity leaders</div>
            <h2 className="mt-1 text-xl font-semibold tracking-tight text-foreground">成交额 Top 个股</h2>
          </div>
          <DataTable columns={['股票', 'PE', '成交额', '涨跌幅']}>
            {topStocks.map((stock) => (
              <div className="grid grid-cols-4 gap-3 px-4 py-3 text-sm text-foreground-muted" key={stock.code}>
                <Link href={buildStockDetailHref(stock)} className="min-w-0 text-foreground transition hover:text-primary">
                  <span className="block truncate font-medium">{stock.name}</span>
                  <span className="block text-xs text-foreground-dim">{stock.code}</span>
                </Link>
                <span className="tabular-nums">{stock.pe ?? '--'}</span>
                <span className="tabular-nums">{formatCompactNumber(stock.amountYi)} 亿</span>
                <span className={`tabular-nums ${changeClassName(stock.pctChg)}`}>{formatPercent(stock.pctChg)}</span>
              </div>
            ))}
          </DataTable>
        </div>

        <div className="rounded-3xl border border-border bg-card px-5 py-5">
          <div className="mb-4">
            <div className="text-xs font-medium uppercase tracking-[0.16em] text-foreground-dim">Sector inflow</div>
            <h2 className="mt-1 text-xl font-semibold tracking-tight text-foreground">主力净流入 Top 板块</h2>
          </div>
          <DataTable columns={['板块', '成交额', '净流入', '代表股']}>
            {(data?.inflowSectors || []).map((sector) => (
              <div className="grid grid-cols-4 gap-3 px-4 py-3 text-sm text-foreground-muted" key={sector.code}>
                <span className="min-w-0">
                  <span className="block truncate font-medium text-foreground">{sector.name}</span>
                  <span className={`block text-xs ${changeClassName(sector.pctChg)}`}>{formatPercent(sector.pctChg)}</span>
                </span>
                <span className="tabular-nums">{formatCompactNumber(sector.amountYi)} 亿</span>
                <span className={`tabular-nums ${changeClassName(sector.mainNetInflowYi)}`}>{formatCompactNumber(sector.mainNetInflowYi)} 亿</span>
                <span className="truncate">{sector.leaderName || '--'}</span>
              </div>
            ))}
          </DataTable>
        </div>
      </section>

      <section className="rounded-3xl border border-border bg-card px-5 py-5 text-sm leading-7 text-foreground-muted">
        <strong className="font-medium text-foreground">数据口径说明：</strong>
        {data?.sourceNote || '当前按成交额排序抓取高流动性样本。主力净流入属于平台算法口径，不等同于交易所逐笔资金流。本页仅用于市场观察和产品验证，不构成投资建议。'}
      </section>
    </div>
  )
}
