import { useCallback, useEffect, useMemo, useState } from 'react'
import Head from 'next/head'
import { requestJson } from '../lib/api'
import {
  buildFactorScreenerPayload,
  buildSelectedFilterChips,
  codeToSymbol,
  flattenMetricDefinitions,
  formatFactorValue,
  getBoardLabel,
  getDynamicMetricColumns,
  getPageNumbers,
  removeFactorFilter,
  updateFactorFilter,
} from '../lib/factor-lab'

const BASE_COLUMNS = [
  { key: 'code', label: '代码', format: 'text', sortable: true, width: 86 },
  { key: 'name', label: '名称', format: 'text', sortable: true, width: 100 },
  { key: 'board', label: '板块', format: 'board', sortable: true, width: 76 },
  { key: 'close_price', label: '昨收价', format: 'price', sortable: true, width: 82 },
  { key: 'market_cap', label: '总市值', format: 'bigNumber', sortable: true, width: 100 },
  { key: 'pe', label: 'PE', format: 'number', sortable: true, width: 70 },
  { key: 'pb', label: 'PB', format: 'number', sortable: true, width: 70 },
]

const QUICK_RANGES = {
  pe: [{ label: '0-10', min: 0, max: 10 }, { label: '0-20', min: 0, max: 20 }, { label: '20-50', min: 20, max: 50 }],
  pb: [{ label: '0-1', min: 0, max: 1 }, { label: '0-2', min: 0, max: 2 }, { label: '2-5', min: 2, max: 5 }],
  dividend_yield: [{ label: '>2%', min: 2 }, { label: '>3%', min: 3 }, { label: '>5%', min: 5 }],
  roe: [{ label: '>10%', min: 10 }, { label: '>15%', min: 15 }, { label: '>20%', min: 20 }],
  beta_1y: [{ label: '<0.8', max: 0.8 }, { label: '<1', max: 1 }, { label: '<1.2', max: 1.2 }],
  volatility_1m: [{ label: '<15%', max: 15 }, { label: '<20%', max: 20 }, { label: '<30%', max: 30 }],
  market_cap: [{ label: '>100亿', min: 100e8 }, { label: '<50亿', max: 50e8 }, { label: '20-100亿', min: 20e8, max: 100e8 }],
}

export default function FactorLabPage() {
  const [meta, setMeta] = useState(null)
  const [data, setData] = useState(null)
  const [filters, setFilters] = useState({})
  const [sortBy, setSortBy] = useState('code')
  const [sortOrder, setSortOrder] = useState('asc')
  const [page, setPage] = useState(1)
  const [pageSize] = useState(50)
  const [loadingMeta, setLoadingMeta] = useState(true)
  const [loadingData, setLoadingData] = useState(false)
  const [error, setError] = useState('')
  const [mobileFiltersOpen, setMobileFiltersOpen] = useState(false)

  const metricGroups = meta?.metrics || []
  const metricMap = useMemo(() => flattenMetricDefinitions(metricGroups), [metricGroups])
  const selectedChips = useMemo(() => buildSelectedFilterChips(filters, metricMap), [filters, metricMap])
  const dynamicMetricKeys = useMemo(() => getDynamicMetricColumns(filters, metricMap), [filters, metricMap])
  const columns = useMemo(() => {
    const existing = new Set(BASE_COLUMNS.map((col) => col.key))
    const dynamic = dynamicMetricKeys
      .filter((key) => !existing.has(key))
      .map((key) => ({ key, label: metricMap[key]?.label || key, format: metricMap[key]?.format || 'number', sortable: true, width: 96 }))
    return [...BASE_COLUMNS, ...dynamic]
  }, [dynamicMetricKeys, metricMap])

  const total = data?.total || 0
  const totalPages = Math.max(1, Math.ceil(total / pageSize))

  const fetchMeta = useCallback(async () => {
    setLoadingMeta(true)
    setError('')
    try {
      const result = await requestJson('/api/factor-lab/meta', undefined, '加载因子实验室失败')
      setMeta(result)
    } catch (err) {
      setError(err.message || '加载因子实验室失败')
    } finally {
      setLoadingMeta(false)
    }
  }, [])

  const fetchScreener = useCallback(async (next = {}) => {
    const currentFilters = next.filters || filters
    const currentSortBy = next.sortBy || sortBy
    const currentSortOrder = next.sortOrder || sortOrder
    const currentPage = next.page || page
    setLoadingData(true)
    setError('')
    try {
      const payload = buildFactorScreenerPayload({ filters: currentFilters, sortBy: currentSortBy, sortOrder: currentSortOrder, page: currentPage, pageSize })
      const result = await requestJson('/api/factor-lab/screener', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      }, '因子筛选失败')
      setData(result)
    } catch (err) {
      setError(err.message || '因子筛选失败')
    } finally {
      setLoadingData(false)
    }
  }, [filters, page, pageSize, sortBy, sortOrder])

  useEffect(() => { fetchMeta() }, [fetchMeta])

  useEffect(() => {
    if (meta?.has_snapshot) fetchScreener({ page: 1 })
  }, [meta?.has_snapshot])

  const handleRangeChange = (key, bound, value) => {
    const nextFilters = updateFactorFilter(filters, key, bound, value)
    setFilters(nextFilters)
    setPage(1)
    fetchScreener({ filters: nextFilters, page: 1 })
  }

  const handleApplyPreset = (key, preset) => {
    const nextFilters = { ...filters, [key]: { min: preset.min ?? '', max: preset.max ?? '' } }
    setFilters(nextFilters)
    setPage(1)
    fetchScreener({ filters: nextFilters, page: 1 })
  }

  const handleRemoveFilter = (key) => {
    const nextFilters = removeFactorFilter(filters, key)
    setFilters(nextFilters)
    setPage(1)
    fetchScreener({ filters: nextFilters, page: 1 })
  }

  const handleReset = () => {
    setFilters({})
    setPage(1)
    setSortBy('code')
    setSortOrder('asc')
    fetchScreener({ filters: {}, sortBy: 'code', sortOrder: 'asc', page: 1 })
  }

  const handleSort = (key) => {
    const nextOrder = sortBy === key && sortOrder === 'asc' ? 'desc' : 'asc'
    setSortBy(key)
    setSortOrder(nextOrder)
    setPage(1)
    fetchScreener({ sortBy: key, sortOrder: nextOrder, page: 1 })
  }

  const handlePageChange = (nextPage) => {
    setPage(nextPage)
    fetchScreener({ page: nextPage })
  }

  return (
    <div className="mx-auto max-w-7xl space-y-5 px-4 py-6 sm:px-6 lg:px-8">
      <Head>
        <title>因子实验室 — 卧龙AI量化交易台</title>
        <meta name="description" content="基于收盘后因子快照筛选 A 股股票，支持价值、成长、质量、动量、规模、低波动等指标范围筛选。" />
      </Head>

      <header className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">因子实验室</h1>
          <p className="mt-1 text-sm text-white/50">基于收盘后预计算快照筛选 A 股因子，当前为 P0 因子筛选。</p>
        </div>
        <button type="button" onClick={() => setMobileFiltersOpen(true)} className="rounded-xl border border-primary/30 bg-primary/10 px-4 py-2 text-sm text-primary lg:hidden">
          筛选条件 {selectedChips.length > 0 ? `(${selectedChips.length})` : ''}
        </button>
      </header>

      <SnapshotStatus meta={meta} loading={loadingMeta} total={total} />

      {error && <div className="rounded-xl border border-red-500/30 bg-red-500/10 px-4 py-3 text-sm text-red-300">{error}</div>}

      {!loadingMeta && meta && !meta.has_snapshot ? (
        <section className="rounded-2xl border border-border bg-card px-5 py-10 text-center">
          <h2 className="text-lg font-medium text-white">因子快照尚未生成</h2>
          <p className="mt-2 text-sm text-white/50">请先等待每日 Phase 1 预计算完成，或手动运行 compute_factor_lab_phase1.py。</p>
        </section>
      ) : (
        <main className="grid gap-5 lg:grid-cols-[330px_1fr]">
          <aside className="hidden lg:block">
            <FilterPanel groups={metricGroups} filters={filters} onChange={handleRangeChange} onPreset={handleApplyPreset} onReset={handleReset} />
          </aside>

          <section className="space-y-4">
            <SelectedChips chips={selectedChips} onRemove={handleRemoveFilter} onReset={handleReset} loading={loadingData} />
            <ResultTable data={data} columns={columns} metricMap={metricMap} sortBy={sortBy} sortOrder={sortOrder} onSort={handleSort} loading={loadingData} />
            <ResultCards data={data} metricMap={metricMap} dynamicMetricKeys={dynamicMetricKeys} loading={loadingData} />
            {total > 0 && <Pagination page={page} totalPages={totalPages} total={total} onPageChange={handlePageChange} />}
          </section>
        </main>
      )}

      {mobileFiltersOpen && (
        <div className="fixed inset-0 z-[80] bg-black/70 px-4 py-5 lg:hidden" onClick={() => setMobileFiltersOpen(false)}>
          <div className="max-h-[90vh] overflow-y-auto rounded-2xl border border-border bg-[#121317] p-4" onClick={(e) => e.stopPropagation()}>
            <div className="mb-3 flex items-center justify-between">
              <h2 className="text-base font-medium">因子筛选</h2>
              <button type="button" onClick={() => setMobileFiltersOpen(false)} className="rounded-full bg-white/10 px-3 py-1 text-sm text-white/60">关闭</button>
            </div>
            <FilterPanel groups={metricGroups} filters={filters} onChange={handleRangeChange} onPreset={handleApplyPreset} onReset={handleReset} />
          </div>
        </div>
      )}
    </div>
  )
}

function SnapshotStatus({ meta, loading, total }) {
  if (loading) {
    return <section className="grid gap-3 sm:grid-cols-4">{Array.from({ length: 4 }).map((_, i) => <div key={i} className="h-20 animate-pulse rounded-2xl border border-border bg-card" />)}</section>
  }
  const cards = [
    { label: '快照日期', value: meta?.snapshot_date || '--' },
    { label: '股票池', value: meta?.universe?.total?.toLocaleString('zh-CN') || '--', suffix: '只' },
    { label: '匹配结果', value: total?.toLocaleString('zh-CN') || '0', suffix: '只' },
    { label: '预计算', value: meta?.last_run?.status || '--' },
  ]
  return (
    <section className="space-y-3">
      <div className="grid gap-3 sm:grid-cols-4">
        {cards.map((card) => <StatCard key={card.label} {...card} />)}
      </div>
      <div className="rounded-xl border border-white/10 bg-white/[0.03] px-4 py-2 text-xs text-white/45">
        股票池已排除 ST / 科创板 / 北交所 / 停牌股，包含上市未满一年新股。{meta?.stale ? '当前快照可能已过期，请关注每日预计算状态。' : ''}
      </div>
    </section>
  )
}

function StatCard({ label, value, suffix }) {
  return <div className="rounded-2xl border border-border bg-card px-4 py-3"><div className="text-xs text-white/40">{label}</div><div className="mt-2 text-xl font-semibold text-white">{value}<span className="ml-1 text-xs font-normal text-white/40">{suffix}</span></div></div>
}

function FilterPanel({ groups, filters, onChange, onPreset, onReset }) {
  return (
    <section className="rounded-2xl border border-border bg-card p-4">
      <div className="mb-3 flex items-center justify-between">
        <div className="text-sm font-medium text-white/80">指标范围</div>
        <button type="button" onClick={onReset} className="text-xs text-white/40 hover:text-white/70">重置</button>
      </div>
      <div className="space-y-4">
        {(groups || []).map((group) => <MetricGroup key={group.key} group={group} filters={filters} onChange={onChange} onPreset={onPreset} />)}
      </div>
    </section>
  )
}

function MetricGroup({ group, filters, onChange, onPreset }) {
  return (
    <details open className="rounded-xl border border-white/10 bg-white/[0.02] p-3">
      <summary className="cursor-pointer text-sm font-medium text-white/70">{group.label}</summary>
      <div className="mt-3 space-y-3">
        {(group.items || []).map((metric) => <MetricControl key={metric.key} metric={metric} value={filters[metric.key] || {}} onChange={onChange} onPreset={onPreset} />)}
      </div>
    </details>
  )
}

function MetricControl({ metric, value, onChange, onPreset }) {
  const presets = QUICK_RANGES[metric.key] || []
  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between gap-2">
        <label className="text-xs text-white/55" title={metric.description}>{metric.label}<span className="ml-1 text-white/25">{metric.unit}</span></label>
        <span className="text-[11px] text-white/25">覆盖 {metric.coverage || 0}</span>
      </div>
      <div className="grid grid-cols-2 gap-2">
        <input value={value.min ?? ''} onChange={(e) => onChange(metric.key, 'min', e.target.value)} placeholder="最小" className="rounded-lg border border-white/10 bg-white/5 px-2 py-1.5 text-xs text-white outline-none focus:border-primary/50" />
        <input value={value.max ?? ''} onChange={(e) => onChange(metric.key, 'max', e.target.value)} placeholder="最大" className="rounded-lg border border-white/10 bg-white/5 px-2 py-1.5 text-xs text-white outline-none focus:border-primary/50" />
      </div>
      {presets.length > 0 && <div className="flex flex-wrap gap-1.5">{presets.map((preset) => <button key={preset.label} type="button" onClick={() => onPreset(metric.key, preset)} className="rounded-md border border-white/10 bg-white/5 px-2 py-0.5 text-[11px] text-white/45 hover:border-primary/30 hover:text-primary">{preset.label}</button>)}</div>}
    </div>
  )
}

function SelectedChips({ chips, onRemove, onReset, loading }) {
  return (
    <section className="rounded-2xl border border-border bg-card px-4 py-3">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex flex-wrap gap-2">
          {chips.length === 0 ? <span className="text-sm text-white/35">未设置因子条件，展示最新快照股票池。</span> : chips.map((chip) => <button key={chip.key} type="button" onClick={() => onRemove(chip.key)} className="rounded-full border border-primary/30 bg-primary/10 px-3 py-1 text-xs text-primary">{chip.text} ×</button>)}
        </div>
        <div className="flex items-center gap-3 text-xs text-white/35">{loading && <span className="animate-pulse">查询中...</span>}<button type="button" onClick={onReset} className="hover:text-white/70">重置</button></div>
      </div>
    </section>
  )
}

function ResultTable({ data, columns, metricMap, sortBy, sortOrder, onSort, loading }) {
  const items = data?.items || []
  return (
    <section className="hidden overflow-hidden rounded-2xl border border-border bg-card lg:block">
      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead><tr className="border-b border-border bg-white/[0.02]">{columns.map((col) => <th key={col.key} style={{ minWidth: col.width }} className="whitespace-nowrap px-3 py-2.5 text-left text-xs font-medium text-white/50"><button type="button" disabled={!col.sortable} onClick={() => onSort(col.key)} className="inline-flex items-center gap-1 hover:text-white/80">{col.label}{sortBy === col.key && <span className="text-primary">{sortOrder === 'asc' ? '↑' : '↓'}</span>}</button></th>)}<th className="px-3 py-2.5 text-center text-xs font-medium text-white/50">操作</th></tr></thead>
          <tbody>
            {loading && items.length === 0 ? Array.from({ length: 8 }).map((_, idx) => <tr key={idx} className="border-b border-white/[0.04]"><td colSpan={columns.length + 1} className="px-3 py-3"><div className="h-4 w-full animate-pulse rounded bg-white/[0.06]" /></td></tr>) : items.length === 0 ? <tr><td colSpan={columns.length + 1} className="py-16 text-center text-white/40">无匹配结果</td></tr> : items.map((row) => <tr key={row.code} className="border-b border-white/[0.04] hover:bg-white/[0.03]">{columns.map((col) => <td key={col.key} className="whitespace-nowrap px-3 py-2 text-white/75">{renderCell(row, col, metricMap)}</td>)}<td className="whitespace-nowrap px-3 py-2 text-center"><button type="button" onClick={() => window.open(`/live-trading/${row.symbol || codeToSymbol(row.code)}`, '_blank')} className="rounded-md border border-primary/30 px-2 py-0.5 text-xs text-primary hover:bg-primary/10">详情</button></td></tr>)}
          </tbody>
        </table>
      </div>
    </section>
  )
}

function ResultCards({ data, metricMap, dynamicMetricKeys, loading }) {
  const items = data?.items || []
  if (loading && items.length === 0) return <section className="space-y-3 lg:hidden">{Array.from({ length: 5 }).map((_, idx) => <div key={idx} className="h-28 animate-pulse rounded-2xl border border-border bg-card" />)}</section>
  return (
    <section className="space-y-3 lg:hidden">
      {items.length === 0 ? <div className="rounded-2xl border border-border bg-card py-12 text-center text-sm text-white/40">无匹配结果</div> : items.map((row) => <article key={row.code} className="rounded-2xl border border-border bg-card p-4"><div className="flex items-start justify-between gap-3"><div><div className="font-medium text-white">{row.name}</div><div className="mt-1 text-xs text-white/40">{row.code} · {getBoardLabel(row.board)} {row.is_new_stock ? '· 新股' : ''}</div></div><button type="button" onClick={() => window.open(`/live-trading/${row.symbol || codeToSymbol(row.code)}`, '_blank')} className="rounded-md border border-primary/30 px-2 py-1 text-xs text-primary">详情</button></div><div className="mt-3 grid grid-cols-2 gap-2 text-xs">{['close_price', 'market_cap', ...dynamicMetricKeys.slice(0, 4)].map((key) => <div key={key} className="rounded-lg bg-white/[0.04] px-2 py-1.5"><div className="text-white/35">{metricMap[key]?.label || baseLabel(key)}</div><div className="mt-1 text-white/80">{formatFactorValue(row[key], metricMap[key]?.format || baseFormat(key))}</div></div>)}</div></article>)}
    </section>
  )
}

function renderCell(row, col, metricMap) {
  if (col.key === 'code') return <span className="font-mono text-primary/80">{String(row.code).padStart(6, '0')}</span>
  if (col.key === 'name') return <span className="text-white/90">{row.name || '--'}{row.is_new_stock && <span className="ml-1 rounded bg-primary/10 px-1 text-[10px] text-primary">新</span>}</span>
  if (col.key === 'board') return getBoardLabel(row.board)
  const format = col.format || metricMap[col.key]?.format || 'number'
  return formatFactorValue(row[col.key], format)
}

function baseLabel(key) {
  return BASE_COLUMNS.find((col) => col.key === key)?.label || key
}

function baseFormat(key) {
  return BASE_COLUMNS.find((col) => col.key === key)?.format || 'number'
}

function Pagination({ page, totalPages, total, onPageChange }) {
  return <div className="flex items-center justify-between rounded-2xl border border-border bg-card px-4 py-3 text-xs text-white/50"><span>共 {total.toLocaleString('zh-CN')} 只 · 第 {page}/{totalPages} 页</span><div className="flex items-center gap-1">{getPageNumbers(page, totalPages).map((p, idx) => p === '...' ? <span key={idx} className="px-2 text-white/25">...</span> : <PaginationButton key={p} active={p === page} onClick={() => onPageChange(p)}>{p}</PaginationButton>)}</div></div>
}

function PaginationButton({ children, active, onClick }) {
  return <button type="button" onClick={onClick} className={`min-w-[28px] rounded-md px-2 py-1 transition ${active ? 'border border-primary/40 bg-primary/15 text-primary' : 'text-white/55 hover:bg-white/10 hover:text-white/80'}`}>{children}</button>
}
