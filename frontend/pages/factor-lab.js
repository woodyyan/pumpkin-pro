import { useCallback, useEffect, useMemo, useState } from 'react'
import Head from 'next/head'
import { requestJson } from '../lib/api'
import {
  FACTOR_DEFINITIONS,
  buildFactorScreenerPayload,
  codeToSymbol,
  factorWeightChipText,
  flattenFactorDefinitions,
  formatFactorValue,
  formatWeight,
  getActiveFactorScoreKeys,
  getPageNumbers,
  getScoreTone,
  isScoreColumnActive,
  validateFactorWeights,
} from '../lib/factor-lab'

const SCORE_COLUMNS = [
  { key: 'composite_score', label: '综合得分', format: 'score', sortable: true, width: 92, primary: true },
  { key: 'value_score', label: '价值', format: 'score', sortable: true, width: 76 },
  { key: 'dividend_yield_score', label: '股息率', format: 'score', sortable: true, width: 82 },
  { key: 'growth_score', label: '成长', format: 'score', sortable: true, width: 76 },
  { key: 'quality_score', label: '质量', format: 'score', sortable: true, width: 76 },
  { key: 'momentum_score', label: '动量', format: 'score', sortable: true, width: 76 },
  { key: 'size_score', label: '规模', format: 'score', sortable: true, width: 76 },
  { key: 'low_volatility_score', label: '低波动', format: 'score', sortable: true, width: 86 },
]

const BASE_COLUMNS = [
  { key: 'code', label: '代码', format: 'text', sortable: true, width: 86 },
  { key: 'name', label: '名称', format: 'text', sortable: true, width: 100 },
  { key: 'industry', label: '行业', format: 'text', sortable: true, width: 92 },
  { key: 'close_price', label: '昨收价', format: 'price', sortable: true, width: 82 },
]

const ALL_COLUMNS = [...SCORE_COLUMNS, ...BASE_COLUMNS]
const SCORE_KEYS = new Set(SCORE_COLUMNS.map((col) => col.key))

export default function FactorLabPage() {
  const [meta, setMeta] = useState(null)
  const [data, setData] = useState(null)
  const [factorWeights, setFactorWeights] = useState({})
  const [sortBy, setSortBy] = useState('composite_score')
  const [sortOrder, setSortOrder] = useState('desc')
  const [page, setPage] = useState(1)
  const [pageSize] = useState(50)
  const [loadingMeta, setLoadingMeta] = useState(true)
  const [loadingData, setLoadingData] = useState(false)
  const [error, setError] = useState('')
  const [mobileFiltersOpen, setMobileFiltersOpen] = useState(false)

  const factors = meta?.factors?.length ? meta.factors : FACTOR_DEFINITIONS
  const factorMap = useMemo(() => flattenFactorDefinitions(factors.map((factor) => ({ ...factor, scoreKey: factor.scoreKey || `${factor.key}_score` }))), [factors])
  const weightStatus = useMemo(() => validateFactorWeights(factorWeights), [factorWeights])
  const activeScoreKeys = useMemo(() => getActiveFactorScoreKeys(factorWeights, factorMap), [factorWeights, factorMap])
  const columns = useMemo(() => ALL_COLUMNS.map((col) => ({ ...col, inactive: !isScoreColumnActive(col.key, activeScoreKeys) })), [activeScoreKeys])
  const chips = useMemo(() => factorWeightChipText(factorWeights, factorMap), [factorWeights, factorMap])
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

  const fetchScreener = useCallback(async () => {
    if (!weightStatus.valid) return
    setLoadingData(true)
    setError('')
    try {
      const payload = buildFactorScreenerPayload({ factorWeights, sortBy, sortOrder, page, pageSize })
      const result = await requestJson('/api/factor-lab/screener', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      }, '因子排序失败')
      setData(result)
    } catch (err) {
      setError(err.message || '因子排序失败')
    } finally {
      setLoadingData(false)
    }
  }, [factorWeights, page, pageSize, sortBy, sortOrder, weightStatus.valid])

  useEffect(() => { fetchMeta() }, [fetchMeta])

  useEffect(() => {
    if (!meta?.has_snapshot || !weightStatus.valid) return undefined
    const timer = setTimeout(() => { fetchScreener() }, 500)
    return () => clearTimeout(timer)
  }, [fetchScreener, meta?.has_snapshot, weightStatus.valid])

  const handleToggleFactor = (key, checked) => {
    setPage(1)
    setSortBy('composite_score')
    setSortOrder('desc')
    setFactorWeights((current) => {
      const next = { ...current }
      if (checked) next[key] = next[key] || ''
      else delete next[key]
      return next
    })
  }

  const handleWeightChange = (key, value) => {
    setPage(1)
    setSortBy('composite_score')
    setSortOrder('desc')
    setFactorWeights((current) => ({ ...current, [key]: value }))
  }

  const handleReset = () => {
    setFactorWeights({})
    setPage(1)
    setSortBy('composite_score')
    setSortOrder('desc')
  }

  const handleSort = (key) => {
    const defaultOrder = SCORE_KEYS.has(key) ? 'desc' : 'asc'
    const nextOrder = sortBy === key ? (sortOrder === 'asc' ? 'desc' : 'asc') : defaultOrder
    setSortBy(key)
    setSortOrder(nextOrder)
    setPage(1)
  }

  const handlePageChange = (nextPage) => {
    setPage(nextPage)
  }

  return (
    <div className="mx-auto max-w-7xl space-y-5 px-4 py-6 sm:px-6 lg:px-8">
      <Head>
        <title>因子实验室 — 卧龙AI量化交易台</title>
        <meta name="description" content="基于收盘后预计算快照，将 A 股指标转换为排名分，并按价值、成长、质量、动量等因子排序。" />
      </Head>

      <header className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">因子实验室</h1>
          <p className="mt-1 text-sm text-white/50">基于排名分的 7 因子排序，支持自定义因子权重生成综合得分。</p>
        </div>
        <button type="button" onClick={() => setMobileFiltersOpen(true)} className="rounded-xl border border-primary/30 bg-primary/10 px-4 py-2 text-sm text-primary lg:hidden">
          因子权重 {Object.keys(factorWeights).length > 0 ? `(${Object.keys(factorWeights).length})` : ''}
        </button>
      </header>

      <SnapshotStatus meta={meta} loading={loadingMeta} total={total} />

      {error && <div className="rounded-xl border border-red-500/30 bg-red-500/10 px-4 py-3 text-sm text-red-300">{error}</div>}

      {!loadingMeta && meta && !meta.has_snapshot ? (
        <section className="rounded-2xl border border-border bg-card px-5 py-10 text-center">
          <h2 className="text-lg font-medium text-white">因子得分快照尚未生成</h2>
          <p className="mt-2 text-sm text-white/50">请先等待每日 Phase 1 + Phase 2 预计算完成，或手动运行 compute_factor_lab_phase2.py。</p>
        </section>
      ) : (
        <main className="grid gap-5 lg:grid-cols-[300px_1fr]">
          <aside className="hidden lg:block">
            <FactorWeightPanel factors={factors} weights={factorWeights} status={weightStatus} onToggle={handleToggleFactor} onChange={handleWeightChange} onReset={handleReset} />
          </aside>

          <section className="space-y-4">
            <SelectedWeights chips={chips} status={weightStatus} onReset={handleReset} loading={loadingData} />
            <ResultTable data={data} columns={columns} sortBy={sortBy} sortOrder={sortOrder} onSort={handleSort} loading={loadingData} />
            <ResultCards data={data} factorWeights={factorWeights} factorMap={factorMap} loading={loadingData} />
            {total > 0 && <Pagination page={page} totalPages={totalPages} total={total} onPageChange={handlePageChange} />}
          </section>
        </main>
      )}

      {mobileFiltersOpen && (
        <div className="fixed inset-0 z-[80] bg-black/70 px-4 py-5 lg:hidden" onClick={() => setMobileFiltersOpen(false)}>
          <div className="max-h-[90vh] overflow-y-auto rounded-2xl border border-border bg-[#121317] p-4" onClick={(e) => e.stopPropagation()}>
            <div className="mb-3 flex items-center justify-between">
              <h2 className="text-base font-medium">因子与权重</h2>
              <button type="button" onClick={() => setMobileFiltersOpen(false)} className="rounded-full bg-white/10 px-3 py-1 text-sm text-white/60">关闭</button>
            </div>
            <FactorWeightPanel factors={factors} weights={factorWeights} status={weightStatus} onToggle={handleToggleFactor} onChange={handleWeightChange} onReset={handleReset} />
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
    { label: '当前结果', value: total?.toLocaleString('zh-CN') || '0', suffix: '只' },
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

function FactorWeightPanel({ factors, weights, status, onToggle, onChange, onReset }) {
  const selectedCount = Object.keys(weights || {}).length
  return (
    <section className="rounded-2xl border border-border bg-card p-4">
      <div className="mb-3 flex items-center justify-between">
        <div className="text-sm font-medium text-white/80">因子与权重</div>
        <button type="button" onClick={onReset} className="text-xs text-white/40 hover:text-white/70">重置</button>
      </div>
      <div className="space-y-2">
        {(factors || []).map((factor) => {
          const checked = Object.prototype.hasOwnProperty.call(weights, factor.key)
          return <FactorWeightRow key={factor.key} factor={factor} checked={checked} value={weights[factor.key] || ''} onToggle={onToggle} onChange={onChange} />
        })}
      </div>
      <div className={`mt-4 rounded-xl border px-3 py-2 text-xs ${status.valid ? 'border-primary/20 bg-primary/5 text-primary/80' : 'border-red-500/30 bg-red-500/10 text-red-300'}`}>
        <div className="flex items-center justify-between"><span>已选 {selectedCount || '默认'} 个因子</span><span>权重合计 {formatWeight(status.sum)}</span></div>
        <div className="mt-1 text-white/45">{status.message}</div>
      </div>
    </section>
  )
}

function FactorWeightRow({ factor, checked, value, onToggle, onChange }) {
  return (
    <div className={`rounded-xl border px-3 py-2 ${checked ? 'border-primary/30 bg-primary/10' : 'border-white/10 bg-white/[0.02]'}`}>
      <div className="flex items-center gap-3">
        <input type="checkbox" checked={checked} onChange={(e) => onToggle(factor.key, e.target.checked)} className="h-4 w-4 accent-primary" />
        <label className="flex-1 text-sm text-white/75">{factor.label}</label>
        <input
          value={value}
          disabled={!checked}
          onChange={(e) => onChange(factor.key, e.target.value)}
          inputMode="decimal"
          placeholder="0.00"
          className="w-20 rounded-lg border border-white/10 bg-white/5 px-2 py-1.5 text-right text-xs text-white outline-none disabled:cursor-not-allowed disabled:opacity-30 focus:border-primary/50"
        />
      </div>
    </div>
  )
}

function SelectedWeights({ chips, status, onReset, loading }) {
  return (
    <section className="rounded-2xl border border-border bg-card px-4 py-3">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex flex-wrap gap-2">
          {chips.map((chip) => <span key={chip} className="rounded-full border border-primary/30 bg-primary/10 px-3 py-1 text-xs text-primary">{chip}</span>)}
        </div>
        <div className="flex items-center gap-3 text-xs text-white/35">{loading && <span className="animate-pulse">排序中...</span>}<span className={status.valid ? 'text-primary/70' : 'text-red-300'}>{status.valid ? '自动应用' : '待修正'}</span><button type="button" onClick={onReset} className="hover:text-white/70">重置</button></div>
      </div>
    </section>
  )
}

function ResultTable({ data, columns, sortBy, sortOrder, onSort, loading }) {
  const items = data?.items || []
  return (
    <section className="hidden overflow-hidden rounded-2xl border border-border bg-card lg:block">
      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead><tr className="border-b border-border bg-white/[0.02]">{columns.map((col) => <th key={col.key} style={{ minWidth: col.width }} className={`whitespace-nowrap px-3 py-2.5 text-left text-xs font-medium ${col.inactive ? 'text-white/20' : 'text-white/50'}`}><button type="button" disabled={!col.sortable} onClick={() => onSort(col.key)} className={`inline-flex items-center gap-1 ${col.inactive ? 'hover:text-white/35' : 'hover:text-white/80'}`}>{col.label}{col.inactive && <span className="rounded bg-white/[0.04] px-1 text-[10px] text-white/25">未参与</span>}{sortBy === col.key && <span className="text-primary">{sortOrder === 'asc' ? '↑' : '↓'}</span>}</button></th>)}<th className="px-3 py-2.5 text-center text-xs font-medium text-white/50">操作</th></tr></thead>
          <tbody>
            {loading && items.length === 0 ? Array.from({ length: 8 }).map((_, idx) => <tr key={idx} className="border-b border-white/[0.04]"><td colSpan={columns.length + 1} className="px-3 py-3"><div className="h-4 w-full animate-pulse rounded bg-white/[0.06]" /></td></tr>) : items.length === 0 ? <tr><td colSpan={columns.length + 1} className="py-16 text-center text-white/40">无匹配结果</td></tr> : items.map((row) => <tr key={row.code} className="border-b border-white/[0.04] hover:bg-white/[0.03]">{columns.map((col) => <td key={col.key} className={`whitespace-nowrap px-3 py-2 ${col.inactive ? 'text-white/25' : 'text-white/75'}`}>{renderCell(row, col)}</td>)}<td className="whitespace-nowrap px-3 py-2 text-center"><button type="button" onClick={() => window.open(`/live-trading/${row.symbol || codeToSymbol(row.code)}`, '_blank')} className="rounded-md border border-primary/30 px-2 py-0.5 text-xs text-primary hover:bg-primary/10">详情</button></td></tr>)}
          </tbody>
        </table>
      </div>
    </section>
  )
}

function ResultCards({ data, factorWeights, factorMap, loading }) {
  const items = data?.items || []
  const selectedKeys = Object.keys(factorWeights || {})
  const scoreKeys = selectedKeys.length > 0 ? selectedKeys.map((key) => factorMap[key]?.scoreKey || `${key}_score`) : ['value_score', 'growth_score', 'quality_score']
  if (loading && items.length === 0) return <section className="space-y-3 lg:hidden">{Array.from({ length: 5 }).map((_, idx) => <div key={idx} className="h-28 animate-pulse rounded-2xl border border-border bg-card" />)}</section>
  return (
    <section className="space-y-3 lg:hidden">
      {items.length === 0 ? <div className="rounded-2xl border border-border bg-card py-12 text-center text-sm text-white/40">无匹配结果</div> : items.map((row) => <article key={row.code} className="rounded-2xl border border-border bg-card p-4"><div className="flex items-start justify-between gap-3"><div><div className="flex items-center gap-2"><ScoreBadge value={row.composite_score} /><div className="font-medium text-white">{row.name}</div></div><div className="mt-1 text-xs text-white/40">{row.code} · {row.industry || '行业--'} · 昨收 {formatFactorValue(row.close_price, 'price')} {row.is_new_stock ? '· 新股' : ''}</div></div><button type="button" onClick={() => window.open(`/live-trading/${row.symbol || codeToSymbol(row.code)}`, '_blank')} className="rounded-md border border-primary/30 px-2 py-1 text-xs text-primary">详情</button></div><div className="mt-3 grid grid-cols-3 gap-2 text-xs">{scoreKeys.slice(0, 3).map((key) => <div key={key} className="rounded-lg bg-white/[0.04] px-2 py-1.5"><div className="text-white/35">{factorMap[key]?.label || scoreLabel(key)}</div><div className="mt-1 text-white/80">{formatFactorValue(row[key], 'score')}</div></div>)}</div></article>)}
    </section>
  )
}

function renderCell(row, col) {
  if (col.key === 'code') return <span className="font-mono text-primary/80">{String(row.code).padStart(6, '0')}</span>
  if (col.key === 'name') return <span className="text-white/90">{row.name || '--'}{row.is_new_stock && <span className="ml-1 rounded bg-primary/10 px-1 text-[10px] text-primary">新</span>}</span>
  if (col.key === 'industry') return row.industry || '--'
  if (SCORE_KEYS.has(col.key)) return <ScoreBadge value={row[col.key]} inactive={col.inactive} />
  return formatFactorValue(row[col.key], col.format || 'number')
}

function ScoreBadge({ value, inactive = false }) {
  const tone = getScoreTone(value)
  const className = inactive ? 'border-white/10 bg-white/[0.02] text-white/25' : {
    strong: 'border-red-500/30 bg-red-500/10 text-red-300',
    good: 'border-primary/30 bg-primary/10 text-primary',
    neutral: 'border-white/15 bg-white/5 text-white/70',
    weak: 'border-green-500/30 bg-green-500/10 text-green-300',
    muted: 'border-white/10 bg-white/[0.03] text-white/35',
  }[tone]
  return <span className={`inline-flex min-w-[48px] justify-center rounded-md border px-2 py-0.5 text-xs ${className}`}>{formatFactorValue(value, 'score')}</span>
}

function scoreLabel(key) {
  return SCORE_COLUMNS.find((col) => col.key === key)?.label || key
}

function Pagination({ page, totalPages, total, onPageChange }) {
  return <div className="flex items-center justify-between rounded-2xl border border-border bg-card px-4 py-3 text-xs text-white/50"><span>共 {total.toLocaleString('zh-CN')} 只 · 第 {page}/{totalPages} 页</span><div className="flex items-center gap-1">{getPageNumbers(page, totalPages).map((p, idx) => p === '...' ? <span key={idx} className="px-2 text-white/25">...</span> : <PaginationButton key={p} active={p === page} onClick={() => onPageChange(p)}>{p}</PaginationButton>)}</div></div>
}

function PaginationButton({ children, active, onClick }) {
  return <button type="button" onClick={onClick} className={`min-w-[28px] rounded-md px-2 py-1 transition ${active ? 'border border-primary/40 bg-primary/15 text-primary' : 'text-white/55 hover:bg-white/10 hover:text-white/80'}`}>{children}</button>
}
