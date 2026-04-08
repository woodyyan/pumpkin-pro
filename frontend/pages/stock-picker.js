import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { requestJson } from '../lib/api'
import { useAuth } from '../lib/auth-context'

// ─── sessionStorage 缓存 ─────────────────────────────────
const SCREENER_CACHE_KEY = 'pumpkin_screener_cache'
const SKELETON_ROWS = 10

function readScreenerCache() {
  try {
    const raw = sessionStorage.getItem(SCREENER_CACHE_KEY)
    if (!raw) return null
    return JSON.parse(raw)
  } catch { return null }
}

function writeScreenerCache(data) {
  try {
    if (!data) return
    sessionStorage.setItem(SCREENER_CACHE_KEY, JSON.stringify(data))
  } catch { /* quota exceeded, ignore */ }
}

// ─── AI 选股示例标签 ─────────────────────────────────────────
const AI_EXAMPLE_QUERIES = [
  '挣钱的蓝筹股',
  '低估值绩优股',
  '高增长小盘股',
  '最近跌多了的好公司',
  '科技行业龙头',
  '今天涨停的',
]

// ─── 筛选条件配置（单下拉 · 预设区间） ──────────────────────
// 每个选项自带 min/max，选中即生效，用户只需选一次
const FILTER_FIELDS = [
  {
    key: 'price', label: '最新价',
    options: [
      { label: '不限',       min: null, max: null },
      { label: '1 元以下',   min: null, max: 1 },
      { label: '1 - 3 元',   min: 1,    max: 3 },
      { label: '3 - 5 元',   min: 3,    max: 5 },
      { label: '5 - 10 元',  min: 5,    max: 10 },
      { label: '10 - 20 元', min: 10,   max: 20 },
      { label: '20 - 50 元', min: 20,   max: 50 },
      { label: '50 - 100 元',min: 50,   max: 100 },
      { label: '100 元以上',  min: 100,  max: null },
    ],
  },
  {
    key: 'change_pct', label: '涨跌幅',
    options: [
      { label: '不限',         min: null, max: null },
      { label: '涨停 (≥9.8%)', min: 9.8,  max: null },
      { label: '涨幅 >5%',     min: 5,    max: null },
      { label: '涨幅 3~5%',    min: 3,    max: 5 },
      { label: '涨幅 1~3%',    min: 1,    max: 3 },
      { label: '小幅波动 ±1%', min: -1,   max: 1 },
      { label: '跌幅 1~3%',    min: -3,   max: -1 },
      { label: '跌幅 3~5%',    min: -5,   max: -3 },
      { label: '跌幅 >5%',     min: null, max: -5 },
      { label: '跌停 (≤-9.8%)',min: null, max: -9.8 },
    ],
  },
  {
    key: 'industry', label: '行业', type: 'enum',
    options: [
      { label: '不限', value: '' },
    ],
  },
  {
    key: 'profit_growth_rate', label: '利润增长率',
    options: [
      { label: '不限',          min: null, max: null },
      { label: '下滑 (<0%)',    min: null, max: 0 },
      { label: '0 - 20%',      min: 0,    max: 20 },
      { label: '20 - 50%',     min: 20,   max: 50 },
      { label: '50 - 100%',    min: 50,   max: 100 },
      { label: '100 - 200%',   min: 100,  max: 200 },
      { label: '200% 以上',     min: 200,  max: null },
    ],
  },
  {
    key: 'total_mv', label: '总市值',
    options: [
      { label: '不限',           min: null,       max: null },
      { label: '微盘 (<20亿)',   min: null,       max: 20e8 },
      { label: '小盘 (20~50亿)', min: 20e8,       max: 50e8 },
      { label: '中盘 (50~200亿)',min: 50e8,       max: 200e8 },
      { label: '中大盘 (200~500亿)', min: 200e8,  max: 500e8 },
      { label: '大盘 (500~2000亿)',  min: 500e8,  max: 2000e8 },
      { label: '超大盘 (>2000亿)',   min: 2000e8, max: null },
    ],
  },
  {
    key: 'pe', label: 'PE（动态）',
    options: [
      { label: '不限',         min: null, max: null },
      { label: '亏损 (<0)',    min: null, max: 0 },
      { label: '0 - 10',      min: 0,    max: 10 },
      { label: '10 - 20',     min: 10,   max: 20 },
      { label: '20 - 30',     min: 20,   max: 30 },
      { label: '30 - 50',     min: 30,   max: 50 },
      { label: '50 - 100',    min: 50,   max: 100 },
      { label: '高估值 (>100)',min: 100,  max: null },
    ],
  },
  {
    key: 'pb', label: 'PB',
    options: [
      { label: '不限',          min: null, max: null },
      { label: '破净 (<1)',     min: null, max: 1 },
      { label: '1 - 2',        min: 1,    max: 2 },
      { label: '2 - 3',        min: 2,    max: 3 },
      { label: '3 - 5',        min: 3,    max: 5 },
      { label: '5 - 10',       min: 5,    max: 10 },
      { label: '高 PB (>10)',   min: 10,   max: null },
    ],
  },
  {
    key: 'turnover_rate', label: '换手率',
    options: [
      { label: '不限',          min: null, max: null },
      { label: '低 (<1%)',      min: null, max: 1 },
      { label: '1 - 3%',       min: 1,    max: 3 },
      { label: '3 - 5%',       min: 3,    max: 5 },
      { label: '5 - 10%',      min: 5,    max: 10 },
      { label: '10 - 20%',     min: 10,   max: 20 },
      { label: '高 (>20%)',     min: 20,   max: null },
    ],
  },
  {
    key: 'volume_ratio', label: '量比',
    options: [
      { label: '不限',           min: null, max: null },
      { label: '极度萎缩 (<0.5)',min: null, max: 0.5 },
      { label: '萎缩 (0.5~1)',   min: 0.5,  max: 1 },
      { label: '正常 (1~2)',     min: 1,    max: 2 },
      { label: '放量 (2~5)',     min: 2,    max: 5 },
      { label: '巨量 (>5)',      min: 5,    max: null },
    ],
  },
  {
    key: 'amplitude', label: '振幅',
    options: [
      { label: '不限',       min: null, max: null },
      { label: '小 (<2%)',   min: null, max: 2 },
      { label: '2 - 5%',    min: 2,    max: 5 },
      { label: '5 - 10%',   min: 5,    max: 10 },
      { label: '大 (>10%)',  min: 10,   max: null },
    ],
  },
  {
    key: 'turnover', label: '成交额',
    options: [
      { label: '不限',               min: null,    max: null },
      { label: '低 (<1000万)',       min: null,    max: 1000e4 },
      { label: '1000万 - 5000万',   min: 1000e4,  max: 5000e4 },
      { label: '5000万 - 1亿',      min: 5000e4,  max: 1e8 },
      { label: '1亿 - 5亿',         min: 1e8,     max: 5e8 },
      { label: '5亿 - 10亿',        min: 5e8,     max: 10e8 },
      { label: '大于 10亿',          min: 10e8,    max: null },
    ],
  },
  {
    key: 'change_pct_60d', label: '60日涨幅',
    options: [
      { label: '不限',           min: null, max: null },
      { label: '大跌 (<-30%)',   min: null, max: -30 },
      { label: '下跌 -30~-10%', min: -30,  max: -10 },
      { label: '小跌 -10~0%',   min: -10,  max: 0 },
      { label: '小涨 0~10%',    min: 0,    max: 10 },
      { label: '上涨 10~30%',   min: 10,   max: 30 },
      { label: '大涨 (>30%)',    min: 30,   max: null },
    ],
  },
  {
    key: 'change_pct_ytd', label: 'YTD涨幅',
    options: [
      { label: '不限',           min: null, max: null },
      { label: '大跌 (<-30%)',   min: null, max: -30 },
      { label: '下跌 -30~-10%', min: -30,  max: -10 },
      { label: '小跌 -10~0%',   min: -10,  max: 0 },
      { label: '小涨 0~10%',    min: 0,    max: 10 },
      { label: '上涨 10~30%',   min: 10,   max: 30 },
      { label: '大涨 (>30%)',    min: 30,   max: null },
    ],
  },
  {
    key: 'float_mv', label: '流通市值',
    options: [
      { label: '不限',           min: null,       max: null },
      { label: '微盘 (<20亿)',   min: null,       max: 20e8 },
      { label: '小盘 (20~50亿)', min: 20e8,       max: 50e8 },
      { label: '中盘 (50~200亿)',min: 50e8,       max: 200e8 },
      { label: '中大盘 (200~500亿)', min: 200e8,  max: 500e8 },
      { label: '大盘 (500~2000亿)',  min: 500e8,  max: 2000e8 },
      { label: '超大盘 (>2000亿)',   min: 2000e8, max: null },
    ],
  },
]

// ─── 表格列配置 ──────────────────────────────────────────────
const TABLE_COLUMNS = [
  { key: 'code',               label: '代码',         sortable: true,  width: 80,  format: 'code' },
  { key: 'name',               label: '名称',         sortable: true,  width: 90,  format: 'text' },
  { key: 'industry',           label: '行业',         sortable: true,  width: 120, format: 'industry' },
  { key: 'price',              label: '最新价',       sortable: true,  width: 80,  format: 'price' },
  { key: 'change_pct',         label: '涨跌幅',       sortable: true,  width: 80,  format: 'percent', colorize: true },
  { key: 'change_amt',         label: '涨跌额',       sortable: true,  width: 80,  format: 'price',   colorize: true },
  { key: 'volume',             label: '成交量(手)',   sortable: true,  width: 100, format: 'integer' },
  { key: 'turnover',           label: '成交额',       sortable: true,  width: 100, format: 'bigNumber' },
  { key: 'amplitude',          label: '振幅',         sortable: true,  width: 70,  format: 'percent' },
  { key: 'turnover_rate',      label: '换手率',       sortable: true,  width: 70,  format: 'percent' },
  { key: 'volume_ratio',       label: '量比',         sortable: true,  width: 60,  format: 'number' },
  { key: 'pe',                 label: 'PE',           sortable: true,  width: 70,  format: 'number' },
  { key: 'pb',                 label: 'PB',           sortable: true,  width: 60,  format: 'number' },
  { key: 'profit_growth_rate', label: '利润增长率',   sortable: true,  width: 95,  format: 'percent' },
  { key: 'total_mv',           label: '总市值',       sortable: true,  width: 100, format: 'bigNumber' },
  { key: 'float_mv',           label: '流通市值',     sortable: true,  width: 100, format: 'bigNumber' },
  { key: 'change_pct_60d',     label: '60日涨幅',     sortable: true,  width: 85,  format: 'percent', colorize: true },
  { key: 'change_pct_ytd',     label: 'YTD涨幅',      sortable: true,  width: 85,  format: 'percent', colorize: true },
]

// ─── 格式化工具 ──────────────────────────────────────────────
const INDUSTRY_LEVEL_SUFFIX_RE = /\s*[ⅠⅡⅢⅣⅤⅥⅦⅧⅨⅩ]+$/u

function formatIndustryLabel(value) {
  if (value === null || value === undefined || value === '') return '--'
  const raw = String(value).trim()
  const cleaned = raw.replace(INDUSTRY_LEVEL_SUFFIX_RE, '').trim()
  return cleaned || raw
}

function normalizeIndustryText(value) {
  if (value === null || value === undefined || value === '') return ''
  return formatIndustryLabel(value).replace(/\s+/g, '').toLowerCase()
}

function formatValue(value, format) {
  if (value === null || value === undefined || value === '') return '--'
  const num = Number(value)

  switch (format) {
    case 'code':
      return String(value).padStart(6, '0')
    case 'text':
      return String(value)
    case 'industry':
      return formatIndustryLabel(value)
    case 'price':
      return isNaN(num) ? '--' : num.toFixed(2)
    case 'percent':
      if (isNaN(num)) return '--'
      return (num >= 0 ? '+' : '') + num.toFixed(2) + '%'
    case 'integer':
      return isNaN(num) ? '--' : num.toLocaleString('zh-CN', { maximumFractionDigits: 0 })
    case 'bigNumber': {
      if (isNaN(num)) return '--'
      const absNum = Math.abs(num)
      if (absNum >= 1e8) return (num / 1e8).toFixed(2) + ' 亿'
      if (absNum >= 1e4) return (num / 1e4).toFixed(2) + ' 万'
      return num.toFixed(2)
    }
    case 'number':
      return isNaN(num) ? '--' : num.toFixed(2)
    default:
      return String(value)
  }
}

// A 股惯例：涨→红，跌→绿
function getColorClass(value) {
  if (value === null || value === undefined) return ''
  const num = Number(value)
  if (isNaN(num)) return ''
  if (num > 0) return 'text-red-500'
  if (num < 0) return 'text-green-500'
  return 'text-white/50'
}

// 6 位纯数字 code → 带交易所后缀的 symbol（用于跳转详情页）
function codeToSymbol(code) {
  const c = String(code).padStart(6, '0')
  return c.startsWith('6') || c.startsWith('9') ? `${c}.SH` : `${c}.SZ`
}

// ─── 防抖 Hook ───────────────────────────────────────────────
function useDebounce(value, delay) {
  const [debounced, setDebounced] = useState(value)
  useEffect(() => {
    const timer = setTimeout(() => setDebounced(value), delay)
    return () => clearTimeout(timer)
  }, [value, delay])
  return debounced
}

// ─── 主页面组件 ──────────────────────────────────────────────
export default function StockPickerPage() {
  const { isLoggedIn, openAuthModal } = useAuth()

  const [filters, setFilters] = useState({})
  const [sortBy, setSortBy] = useState('code')
  const [sortOrder, setSortOrder] = useState('asc')
  const [page, setPage] = useState(1)
  const [pageSize] = useState(50)
  const [data, setData] = useState(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [filtersExpanded, setFiltersExpanded] = useState(true)
  const [industryOptions, setIndustryOptions] = useState([])

  // ── 关注池（用于判断是否已关注）──
  const [watchedSymbols, setWatchedSymbols] = useState(new Set())
  const [addingCode, setAddingCode] = useState(null) // 正在关注中的 code

  // 获取用户关注池
  const fetchWatchedSymbols = useCallback(async () => {
    if (!isLoggedIn) { setWatchedSymbols(new Set()); return }
    try {
      const res = await requestJson('/api/live/watchlist')
      const codes = new Set((res?.items || []).map((i) => i.symbol.split('.')[0].padStart(6, '0')))
      setWatchedSymbols(codes)
    } catch { /* non-critical */ }
  }, [isLoggedIn])

  useEffect(() => { fetchWatchedSymbols() }, [fetchWatchedSymbols])

  // 一键关注
  const handleAddWatch = async (code, name) => {
    if (!isLoggedIn) {
      openAuthModal('login', '登录后可关注股票')
      return
    }
    const symbol = codeToSymbol(code)
    setAddingCode(code)
    try {
      await requestJson('/api/live/watchlist', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ symbol, name: name || '' }),
      })
      setWatchedSymbols((prev) => new Set([...prev, String(code).padStart(6, '0')]))
    } catch { /* 静默：可能已存在 */ }
    setAddingCode(null)
  }

  // 跳转详情页（新标签页）
  const handleOpenDetail = (code) => {
    window.open(`/live-trading/${codeToSymbol(code)}`, '_blank')
  }

  // ── 自选表状态 ──
  const [watchlists, setWatchlists] = useState([])
  const [wlLoading, setWlLoading] = useState(false)
  const [activeWatchlistId, setActiveWatchlistId] = useState(null) // 当前加载的自选表
  const [saveDialogOpen, setSaveDialogOpen] = useState(false)
  const [saveName, setSaveName] = useState('')
  const [saving, setSaving] = useState(false)
  const [wlError, setWlError] = useState('')

  const debouncedFilters = useDebounce(filters, 500)
  const initialLoadRef = useRef(false)

  // ── AI 选股状态 ──
  const [aiQuery, setAiQuery] = useState('')
  const [aiParsing, setAiParsing] = useState(false)
  const [aiSummary, setAiSummary] = useState('')
  const [aiError, setAiError] = useState('')

  const filterFields = useMemo(() => {
    const resolvedIndustryOptions = [
      { label: '不限', value: '' },
      ...industryOptions.map((industry) => ({ label: formatIndustryLabel(industry), value: industry })),
    ]

    return FILTER_FIELDS.map((field) => (
      field.key === 'industry'
        ? { ...field, options: resolvedIndustryOptions }
        : field
    ))
  }, [industryOptions])

  // ── 自选表 API ──
  const fetchWatchlists = useCallback(async () => {
    if (!isLoggedIn) { setWatchlists([]); return }
    setWlLoading(true)
    try {
      const res = await requestJson('/api/screener/watchlists', undefined, '加载自选表失败')
      setWatchlists(res?.items || [])
    } catch {
      // 静默失败——列表不影响核心功能
    } finally {
      setWlLoading(false)
    }
  }, [isLoggedIn])

  // 登录状态变化时刷新自选表列表
  useEffect(() => { fetchWatchlists() }, [fetchWatchlists])

  const saveWatchlist = async () => {
    const trimmed = saveName.trim()
    if (!trimmed) { setWlError('请输入自选表名称'); return }
    const stocks = (data?.items || []).map((r) => ({ code: r.code, name: r.name }))
    if (stocks.length === 0) { setWlError('当前筛选结果为空，无法保存'); return }
    setSaving(true)
    setWlError('')
    try {
      await requestJson('/api/screener/watchlists', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: trimmed, stocks }),
      }, '保存自选表失败')
      setSaveDialogOpen(false)
      setSaveName('')
      fetchWatchlists()
    } catch (err) {
      setWlError(err.message || '保存失败')
    } finally {
      setSaving(false)
    }
  }

  const loadWatchlist = async (id) => {
    if (activeWatchlistId === id) { setActiveWatchlistId(null); return } // toggle off
    setLoading(true)
    setError('')
    setActiveWatchlistId(id)
    try {
      const detail = await requestJson(`/api/screener/watchlists/${id}`, undefined, '加载自选表失败')
      const wlDetail = detail?.item || {}
      // 将自选表的股票展示在表格中（只包含 code/name，其余字段为空）
      setData({ items: wlDetail.stocks || [], total: (wlDetail.stocks || []).length })
    } catch (err) {
      setError(err.message || '加载失败')
      setActiveWatchlistId(null)
    } finally {
      setLoading(false)
    }
  }

  const deleteWatchlist = async (id) => {
    try {
      await requestJson(`/api/screener/watchlists/${id}`, { method: 'DELETE' }, '删除失败')
      if (activeWatchlistId === id) setActiveWatchlistId(null)
      fetchWatchlists()
    } catch (err) {
      setError(err.message || '删除失败')
    }
  }

  // 退出自选表查看模式 → 重新用当前筛选条件查询
  const exitWatchlistView = () => {
    setActiveWatchlistId(null)
    fetchScreener(filters, sortBy, sortOrder, page)
  }

  // ── AI 自然语言选股 ──
  const handleAIParse = async () => {
    const trimmed = aiQuery.trim()
    if (!trimmed) return
    setAiParsing(true)
    setAiError('')
    setAiSummary('')
    try {
      const res = await requestJson('/api/screener/ai-parse', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ query: trimmed }),
      }, 'AI 解析失败')

      if (res?.summary) setAiSummary(res.summary)

      // 将 AI 返回的结构化条件映射回筛选选项索引
      const aiFilters = res?.filters || {}
      const newFilters = {}
      for (const [key, range] of Object.entries(aiFilters)) {
        const field = filterFields.find((f) => f.key === key)
        if (!field || field.type === 'enum') continue
        const bestIdx = findBestOptionIndex(field.options, range.min, range.max)
        if (bestIdx > 0) newFilters[key] = bestIdx
      }

      const aiIndustry = typeof res?.industry === 'string' ? res.industry.trim() : ''
      if (aiIndustry) {
        const industryField = filterFields.find((f) => f.key === 'industry')
        const industryIdx = findBestIndustryOptionIndex(industryField?.options || [], aiIndustry)
        if (industryIdx > 0) newFilters.industry = industryIdx
      }

      setFilters(newFilters)
      setActiveWatchlistId(null) // 退出自选表模式
      if (!filtersExpanded) setFiltersExpanded(true)
    } catch (err) {
      setAiError(err.message || 'AI 解析失败')
    } finally {
      setAiParsing(false)
    }
  }

  // 构建 API 请求参数：数值字段映射到 filters，枚举字段走顶层参数
  const buildScanPayload = useCallback((rawFilters) => {
    const payload = { filters: {} }

    for (const [key, optionIdx] of Object.entries(rawFilters)) {
      const field = filterFields.find((f) => f.key === key)
      if (!field) continue

      const opt = field.options[optionIdx]
      if (!opt) continue

      if (field.type === 'enum') {
        if (opt.value) payload[key] = opt.value
        continue
      }

      const entry = {}
      if (opt.min !== null && opt.min !== undefined) entry.min = opt.min
      if (opt.max !== null && opt.max !== undefined) entry.max = opt.max
      if (Object.keys(entry).length > 0) {
        payload.filters[key] = entry
      }
    }

    return payload
  }, [filterFields])

  const fetchScreener = useCallback(async (currentFilters, currentSortBy, currentSortOrder, currentPage) => {
    setLoading(true)
    setError('')
    try {
      const scanPayload = buildScanPayload(currentFilters)
      const result = await requestJson('/api/screener/scan', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          ...scanPayload,
          sort_by: currentSortBy,
          sort_order: currentSortOrder,
          page: currentPage,
          page_size: pageSize,
        }),
      }, '选股查询失败')
      if (Array.isArray(result?.industries)) {
        setIndustryOptions(result.industries)
      }
      setData(result)
      // Cache default view (no filters, page 1) for instant next visit
      if (Object.keys(currentFilters).length === 0 && currentPage === 1) {
        writeScreenerCache(result)
      }
    } catch (err) {
      setError(err.message || '查询失败')
    } finally {
      setLoading(false)
    }
  }, [buildScanPayload, pageSize])

  // 初始加载：先读缓存即时展示，再后台刷新最新数据
  useEffect(() => {
    if (!initialLoadRef.current) {
      initialLoadRef.current = true
      const cached = readScreenerCache()
      if (cached) {
        setData(cached)
        if (Array.isArray(cached?.industries)) {
          setIndustryOptions(cached.industries)
        }
        // 后台静默刷新（不显示 loading）
        ;(async () => {
          try {
            const result = await requestJson('/api/screener/scan', {
              method: 'POST',
              headers: { 'Content-Type': 'application/json' },
              body: JSON.stringify({ filters: {}, sort_by: 'code', sort_order: 'asc', page: 1, page_size: pageSize }),
            }, '选股查询失败')
            if (Array.isArray(result?.industries)) setIndustryOptions(result.industries)
            setData(result)
            writeScreenerCache(result)
          } catch { /* 静默失败，保留缓存数据 */ }
        })()
      } else {
        fetchScreener({}, 'code', 'asc', 1)
      }
    }
  }, [fetchScreener, pageSize])

  // 筛选条件变化时自动查询（防抖后）
  useEffect(() => {
    if (!initialLoadRef.current) return
    setPage(1)
    fetchScreener(debouncedFilters, sortBy, sortOrder, 1)
  }, [debouncedFilters, fetchScreener, sortBy, sortOrder])

  // 翻页
  const handlePageChange = (newPage) => {
    setPage(newPage)
    fetchScreener(filters, sortBy, sortOrder, newPage)
  }

  // 排序
  const handleSort = (columnKey) => {
    let newOrder = 'desc'
    if (sortBy === columnKey) {
      newOrder = sortOrder === 'desc' ? 'asc' : 'desc'
    }
    setSortBy(columnKey)
    setSortOrder(newOrder)
    setPage(1)
  }

  // 更新单个筛选字段（存选项索引，0 = 不限）
  const handleFilterChange = (key, optionIdx) => {
    setFilters((prev) => {
      const next = { ...prev }
      if (optionIdx === 0) {
        delete next[key]
      } else {
        next[key] = optionIdx
      }
      return next
    })
  }

  // 重置
  const handleReset = () => {
    setFilters({})
    setSortBy('code')
    setSortOrder('asc')
    setPage(1)
  }

  const total = data?.total || 0
  const items = data?.items || []
  const totalPages = Math.max(1, Math.ceil(total / pageSize))
  const hasActiveFilters = Object.keys(filters).length > 0

  return (
    <div className="space-y-4">
      {/* ─── Hero Section ─── */}
      <div className="flex items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">选股器</h1>
          <p className="mt-1 text-sm text-white/50">A 股全市场多维指标筛选，实时行情数据</p>
        </div>
        <div className="flex items-center gap-3 text-sm">
          <MiniStat label="全市场" value={total.toLocaleString('zh-CN')} suffix="只" />
          <MiniStat label="当前页" value={items.length} suffix="只" />
        </div>
      </div>

      {/* ─── AI 自然语言选股 ─── */}
      <section className="rounded-2xl border border-border bg-card px-5 py-3">
        <div className="mb-2 text-xs font-medium text-white/50">AI 一句话选股</div>
        <div className="flex items-center gap-3">
          <div className="relative flex-1">
            <input
              value={aiQuery}
              onChange={(e) => setAiQuery(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && !aiParsing && handleAIParse()}
              placeholder={'随便说，如"挣钱的蓝筹股""最近涨得好的科技股""帮我找便宜的小盘股"'}
              className="w-full rounded-xl border border-white/15 bg-white/5 py-2 pl-3 pr-20 text-sm text-white outline-none transition placeholder:text-white/25 focus:border-primary/50 focus:ring-1 focus:ring-primary/30"
            />
            <button
              type="button"
              onClick={handleAIParse}
              disabled={aiParsing || !aiQuery.trim()}
              className="absolute right-1.5 top-1/2 -translate-y-1/2 rounded-xl bg-gradient-to-r from-indigo-500 to-violet-500 px-3 py-1.5 text-xs font-semibold text-white shadow-[0_0_12px_rgba(99,102,241,0.3)] transition-all duration-300 hover:scale-[1.03] hover:shadow-[0_0_18px_rgba(99,102,241,0.45)] active:scale-[0.98] animate-ai-glow disabled:cursor-not-allowed disabled:opacity-40 disabled:hover:scale-100"
            >
              {aiParsing ? '解析中...' : '✨ AI 选股'}
            </button>
          </div>
        </div>
        <div className="mt-2 flex flex-wrap items-center gap-1.5">
          <span className="text-[11px] text-white/30">试试：</span>
          {AI_EXAMPLE_QUERIES.map((example) => (
            <button
              key={example}
              type="button"
              disabled={aiParsing}
              onClick={() => { setAiQuery(example); }}
              className="rounded-full border border-white/10 bg-white/[0.03] px-2.5 py-1 text-[11px] text-white/45 transition hover:border-primary/30 hover:bg-primary/10 hover:text-primary disabled:cursor-not-allowed disabled:opacity-40"
            >
              {example}
            </button>
          ))}
        </div>
        {aiSummary && (
          <p className="mt-2 text-xs text-primary/70">
            <span className="font-medium text-primary">AI 理解：</span>{aiSummary}
          </p>
        )}
        {aiError && (
          <p className="mt-2 text-xs text-red-300">{aiError}</p>
        )}
      </section>

      {/* ─── 筛选条件面板 ─── */}
      <section className="rounded-2xl border border-border bg-card">
        <button
          type="button"
          onClick={() => setFiltersExpanded((prev) => !prev)}
          className="flex w-full items-center justify-between px-5 py-3 text-left"
        >
          <span className="text-sm font-medium text-white/80">
            筛选条件
            {hasActiveFilters && (
              <span className="ml-2 rounded-full bg-primary/20 px-2 py-0.5 text-xs text-primary">
                {Object.keys(filters).length} 项
              </span>
            )}
          </span>
          <span className="text-xs text-white/40">{filtersExpanded ? '收起 ▲' : '展开 ▼'}</span>
        </button>

        {filtersExpanded && (
          <div className="border-t border-border px-5 pb-4 pt-3">
            <div className="grid grid-cols-2 gap-x-6 gap-y-3 sm:grid-cols-3 lg:grid-cols-4">
              {filterFields.map((field) => (
                <FilterSelect
                  key={field.key}
                  field={field}
                  selectedIdx={filters[field.key] ?? 0}
                  onChange={handleFilterChange}
                />
              ))}
            </div>
            <div className="mt-3 flex items-center justify-between">
              <button
                type="button"
                disabled={!hasActiveFilters}
                onClick={handleReset}
                className="rounded-lg border border-white/15 px-3 py-1.5 text-xs text-white/60 transition hover:border-white/25 hover:text-white/80 disabled:cursor-not-allowed disabled:opacity-40"
              >
                重置筛选
              </button>
              {loading && <span className="text-xs text-white/40 animate-pulse">正在查询...</span>}
            </div>
          </div>
        )}
      </section>

      {/* ─── 自选表工具栏 ─── */}
      <WatchlistToolbar
        isLoggedIn={isLoggedIn}
        openAuthModal={openAuthModal}
        watchlists={watchlists}
        wlLoading={wlLoading}
        activeWatchlistId={activeWatchlistId}
        items={items}
        onSave={() => { setSaveName(''); setWlError(''); setSaveDialogOpen(true) }}
        onLoad={loadWatchlist}
        onDelete={deleteWatchlist}
        onExit={exitWatchlistView}
      />

      {/* ─── 保存自选表弹窗 ─── */}
      {saveDialogOpen && (
        <SaveWatchlistDialog
          name={saveName}
          onNameChange={setSaveName}
          stockCount={items.length}
          saving={saving}
          error={wlError}
          onSave={saveWatchlist}
          onClose={() => setSaveDialogOpen(false)}
        />
      )}

      {/* ─── 错误提示 ─── */}
      {error && (
        <div className="rounded-xl border border-red-500/30 bg-red-500/10 px-4 py-3 text-sm text-red-300">
          {error}
        </div>
      )}

      {/* ─── 结果表格 ─── */}
      <section className="rounded-2xl border border-border bg-card overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border bg-white/[0.02]">
                {TABLE_COLUMNS.map((col) => (
                  <th
                    key={col.key}
                    className={`whitespace-nowrap px-3 py-2.5 text-left text-xs font-medium text-white/50 ${
                      col.sortable ? 'cursor-pointer select-none hover:text-white/80 transition' : ''
                    }`}
                    style={{ minWidth: col.width }}
                    onClick={() => col.sortable && handleSort(col.key)}
                  >
                    <span className="inline-flex items-center gap-1">
                      {col.label}
                      {col.sortable && sortBy === col.key && (
                        <span className="text-primary">{sortOrder === 'asc' ? '↑' : '↓'}</span>
                      )}
                    </span>
                  </th>
                ))}
                <th className="whitespace-nowrap px-3 py-2.5 text-center text-xs font-medium text-white/50" style={{ minWidth: 70 }}>
                  操作
                </th>
              </tr>
            </thead>
            <tbody>
              {loading && items.length === 0 ? (
                Array.from({ length: SKELETON_ROWS }).map((_, rowIdx) => (
                  <tr key={`skeleton-${rowIdx}`} className="border-b border-white/[0.04]">
                    {TABLE_COLUMNS.map((col) => (
                      <td key={col.key} className="px-3 py-2" style={{ minWidth: col.width }}>
                        <div className={`h-4 rounded bg-white/[0.06] animate-pulse ${col.key === 'name' ? 'w-16' : col.key === 'code' ? 'w-14' : 'w-12'}`} />
                      </td>
                    ))}
                    <td className="px-3 py-2" style={{ minWidth: 70 }}>
                      <div className="h-4 w-10 rounded bg-white/[0.06] animate-pulse mx-auto" />
                    </td>
                  </tr>
                ))
              ) : items.length === 0 ? (
                <tr>
                  <td colSpan={TABLE_COLUMNS.length + 1} className="py-16 text-center text-white/40">
                    {error ? '查询失败' : '无匹配结果'}
                  </td>
                </tr>
              ) : (
                items.map((row, idx) => {
                  const code6 = String(row.code || '').padStart(6, '0')
                  const isWatched = watchedSymbols.has(code6)
                  const isAdding = addingCode === row.code
                  return (
                    <tr
                      key={row.code || idx}
                      className="border-b border-white/[0.04] transition hover:bg-white/[0.03]"
                    >
                      {TABLE_COLUMNS.map((col) => {
                        const colorClass = col.colorize ? getColorClass(row[col.key]) : ''
                        // 代码和名称列可点击跳转详情页
                        if (col.key === 'code' || col.key === 'name') {
                          return (
                            <td
                              key={col.key}
                              className={`whitespace-nowrap px-3 py-2 ${colorClass}`}
                              style={{ minWidth: col.width }}
                            >
                              <button
                                type="button"
                                onClick={() => handleOpenDetail(row.code)}
                                className={`text-left transition ${
                                  col.key === 'code'
                                    ? 'font-mono text-primary/80 hover:text-primary hover:underline underline-offset-2'
                                    : 'text-white/90 hover:text-primary hover:underline underline-offset-2'
                                }`}
                              >
                                {formatValue(row[col.key], col.format)}
                              </button>
                            </td>
                          )
                        }
                        return (
                          <td
                            key={col.key}
                            className={`whitespace-nowrap px-3 py-2 ${colorClass}`}
                            style={{ minWidth: col.width }}
                          >
                            {formatValue(row[col.key], col.format)}
                          </td>
                        )
                      })}
                      <td className="whitespace-nowrap px-3 py-2 text-center" style={{ minWidth: 70 }}>
                        {isWatched ? (
                          <span className="inline-flex items-center gap-0.5 text-[11px] text-white/30">
                            <span>✓</span> 已关注
                          </span>
                        ) : (
                          <button
                            type="button"
                            disabled={isAdding}
                            onClick={() => handleAddWatch(row.code, row.name)}
                            className="inline-flex items-center gap-0.5 rounded-md border border-primary/30 bg-primary/8 px-2 py-0.5 text-[11px] font-medium text-primary transition hover:bg-primary/15 disabled:opacity-40"
                          >
                            {isAdding ? '...' : '+ 关注'}
                          </button>
                        )}
                      </td>
                    </tr>
                  )
                })
              )}
            </tbody>
          </table>
        </div>

        {/* ─── 分页 ─── */}
        {total > 0 && (
          <div className="flex items-center justify-between border-t border-border px-4 py-3 text-xs text-white/50">
            <span>
              共 {total.toLocaleString('zh-CN')} 只 · 第 {page}/{totalPages} 页
            </span>
            <div className="flex items-center gap-1">
              <PaginationButton disabled={page <= 1} onClick={() => handlePageChange(1)}>
                ««
              </PaginationButton>
              <PaginationButton disabled={page <= 1} onClick={() => handlePageChange(page - 1)}>
                «
              </PaginationButton>
              {getPageNumbers(page, totalPages).map((p, i) =>
                p === '...' ? (
                  <span key={`ellipsis-${i}`} className="px-2 text-white/30">
                    ...
                  </span>
                ) : (
                  <PaginationButton
                    key={p}
                    active={p === page}
                    onClick={() => handlePageChange(p)}
                  >
                    {p}
                  </PaginationButton>
                )
              )}
              <PaginationButton disabled={page >= totalPages} onClick={() => handlePageChange(page + 1)}>
                »
              </PaginationButton>
              <PaginationButton disabled={page >= totalPages} onClick={() => handlePageChange(totalPages)}>
                »»
              </PaginationButton>
            </div>
          </div>
        )}
      </section>
    </div>
  )
}

// ─── 子组件 ──────────────────────────────────────────────────

function MiniStat({ label, value, suffix }) {
  return (
    <div className="flex items-baseline gap-1 rounded-lg border border-border bg-card px-3 py-2">
      <span className="text-white/45">{label}</span>
      <span className="font-semibold text-white tabular-nums">{value}</span>
      {suffix && <span className="text-white/45">{suffix}</span>}
    </div>
  )
}

function FilterSelect({ field, selectedIdx, onChange }) {
  const isActive = selectedIdx > 0
  return (
    <div className="space-y-1">
      <label className={`block text-xs ${isActive ? 'text-primary font-medium' : 'text-white/45'}`}>
        {field.label}
        {isActive && <span className="ml-1 text-primary/60">●</span>}
      </label>
      <select
        value={selectedIdx}
        onChange={(e) => onChange(field.key, Number(e.target.value))}
        className={`w-full appearance-none rounded-md border bg-white/5 px-2 py-1.5 text-xs outline-none transition cursor-pointer ${
          isActive
            ? 'border-primary/40 text-primary'
            : 'border-white/10 text-white/60'
        } focus:border-primary/50 focus:ring-1 focus:ring-primary/30`}
      >
        {field.options.map((opt, idx) => (
          <option key={idx} value={idx} className="bg-[#1a1a2e] text-white">
            {opt.label}
          </option>
        ))}
      </select>
    </div>
  )
}

function PaginationButton({ children, disabled, active, onClick }) {
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={onClick}
      className={`min-w-[28px] rounded-md px-2 py-1 text-xs transition ${
        active
          ? 'border border-primary/40 bg-primary/15 text-primary font-medium'
          : disabled
          ? 'text-white/20 cursor-not-allowed'
          : 'text-white/55 hover:bg-white/10 hover:text-white/80'
      }`}
    >
      {children}
    </button>
  )
}

// ─── 分页页码生成 ─────────────────────────────────────────────
function getPageNumbers(current, total) {
  if (total <= 7) {
    return Array.from({ length: total }, (_, i) => i + 1)
  }
  const pages = []
  pages.push(1)
  if (current > 3) pages.push('...')
  for (let i = Math.max(2, current - 1); i <= Math.min(total - 1, current + 1); i++) {
    pages.push(i)
  }
  if (current < total - 2) pages.push('...')
  pages.push(total)
  return pages
}

// ─── AI 结果 → 选项索引匹配 ──────────────────────────────────
function findBestIndustryOptionIndex(options, aiIndustry) {
  const rawTarget = typeof aiIndustry === 'string' ? aiIndustry.trim() : ''
  if (!rawTarget || !Array.isArray(options) || options.length <= 1) return 0

  for (let i = 1; i < options.length; i++) {
    const opt = options[i]
    const optValue = typeof opt?.value === 'string' ? opt.value.trim() : ''
    const optLabel = typeof opt?.label === 'string' ? opt.label.trim() : ''
    if (rawTarget === optValue || rawTarget === optLabel) return i
  }

  const normalizedTarget = normalizeIndustryText(rawTarget)
  if (!normalizedTarget) return 0

  for (let i = 1; i < options.length; i++) {
    const opt = options[i]
    if (normalizedTarget === normalizeIndustryText(opt?.value) || normalizedTarget === normalizeIndustryText(opt?.label)) {
      return i
    }
  }

  for (let i = 1; i < options.length; i++) {
    const opt = options[i]
    const normalizedOption = normalizeIndustryText(opt?.value) || normalizeIndustryText(opt?.label)
    if (!normalizedOption) continue
    if (normalizedOption.includes(normalizedTarget) || normalizedTarget.includes(normalizedOption)) {
      return i
    }
  }

  return 0
}

// 给定 AI 返回的 min/max，在字段的 options 里找到最接近的选项索引
function findBestOptionIndex(options, aiMin, aiMax) {
  let bestIdx = 0
  let bestScore = -Infinity

  for (let i = 1; i < options.length; i++) {
    const opt = options[i]
    const oMin = opt.min
    const oMax = opt.max

    // 计算匹配分数：完全包含 AI 范围的选项得分最高
    let score = 0

    // 方向匹配
    if (aiMin !== null && aiMin !== undefined) {
      if (oMin !== null && oMin !== undefined) {
        // 两者都有下界 → 越接近越好
        score -= Math.abs(aiMin - oMin)
      } else {
        score -= 1000 // 选项没有下界但 AI 需要
      }
    }
    if (aiMax !== null && aiMax !== undefined) {
      if (oMax !== null && oMax !== undefined) {
        score -= Math.abs(aiMax - oMax)
      } else {
        score -= 1000
      }
    }

    // 如果 AI 只有一个方向，优先选只有同方向的选项
    if (aiMin !== null && aiMin !== undefined && (aiMax === null || aiMax === undefined)) {
      if (oMin !== null && (oMax === null || oMax === undefined)) score += 500
    }
    if (aiMax !== null && aiMax !== undefined && (aiMin === null || aiMin === undefined)) {
      if (oMax !== null && (oMin === null || oMin === undefined)) score += 500
    }

    // 完全匹配加分
    if (oMin === aiMin && oMax === aiMax) score += 10000

    if (score > bestScore) {
      bestScore = score
      bestIdx = i
    }
  }

  return bestIdx
}

// ─── 自选表工具栏 ─────────────────────────────────────────────
function WatchlistToolbar({
  isLoggedIn, openAuthModal, watchlists, wlLoading,
  activeWatchlistId, items, onSave, onLoad, onDelete, onExit,
}) {
  const [confirmDeleteId, setConfirmDeleteId] = useState(null)

  return (
    <section className="rounded-2xl border border-border bg-card px-5 py-3">
      <div className="flex items-center justify-between gap-4">
        <div className="flex items-center gap-2 text-sm text-white/70">
          <span className="text-xs font-medium text-white/50">我的自选表</span>
          {!isLoggedIn && (
            <button
              type="button"
              onClick={() => openAuthModal('login', '登录后可保存和管理自选表')}
              className="text-xs text-primary hover:text-primary/80 transition"
            >
              登录使用
            </button>
          )}
        </div>

        {isLoggedIn && (
          <button
            type="button"
            disabled={!items.length}
            onClick={onSave}
            className="inline-flex items-center gap-1.5 rounded-lg bg-primary/15 px-3 py-1.5 text-xs font-medium text-primary transition hover:bg-primary/25 disabled:cursor-not-allowed disabled:opacity-40"
          >
            <span>+</span> 保存当前结果
          </button>
        )}
      </div>

      {/* 自选表列表 */}
      {isLoggedIn && (
        <div className="mt-2">
          {wlLoading ? (
            <span className="text-xs text-white/30 animate-pulse">加载中...</span>
          ) : watchlists.length === 0 ? (
            <span className="text-xs text-white/25">暂无保存的自选表</span>
          ) : (
            <div className="flex flex-wrap gap-2">
              {watchlists.map((wl) => {
                const isActive = activeWatchlistId === wl.id
                const isDeleting = confirmDeleteId === wl.id
                return (
                  <div
                    key={wl.id}
                    className={`group inline-flex items-center gap-1 rounded-lg border px-2.5 py-1.5 text-xs transition ${
                      isActive
                        ? 'border-primary/40 bg-primary/10 text-primary'
                        : 'border-white/10 bg-white/5 text-white/60 hover:border-white/20 hover:text-white/80'
                    }`}
                  >
                    <button type="button" onClick={() => onLoad(wl.id)} className="flex items-center gap-1">
                      <span>{wl.name}</span>
                      <span className="text-[10px] opacity-50">({wl.stock_count})</span>
                    </button>
                    {isDeleting ? (
                      <span className="ml-1 flex items-center gap-1">
                        <button
                          type="button"
                          onClick={() => { onDelete(wl.id); setConfirmDeleteId(null) }}
                          className="text-red-400 hover:text-red-300"
                          title="确认删除"
                        >
                          ✓
                        </button>
                        <button
                          type="button"
                          onClick={() => setConfirmDeleteId(null)}
                          className="text-white/40 hover:text-white/60"
                          title="取消"
                        >
                          ✗
                        </button>
                      </span>
                    ) : (
                      <button
                        type="button"
                        onClick={() => setConfirmDeleteId(wl.id)}
                        className="ml-0.5 text-white/20 hover:text-red-400 transition opacity-0 group-hover:opacity-100"
                        title="删除"
                      >
                        ×
                      </button>
                    )}
                  </div>
                )
              })}
            </div>
          )}

          {/* 自选表查看模式提示 */}
          {activeWatchlistId && (
            <div className="mt-2 flex items-center gap-2 text-xs text-primary/70">
              <span>正在查看自选表内容</span>
              <button
                type="button"
                onClick={onExit}
                className="rounded border border-primary/30 px-2 py-0.5 text-primary hover:bg-primary/10 transition"
              >
                返回筛选
              </button>
            </div>
          )}
        </div>
      )}
    </section>
  )
}

// ─── 保存自选表弹窗 ──────────────────────────────────────────
function SaveWatchlistDialog({ name, onNameChange, stockCount, saving, error, onSave, onClose }) {
  return (
    <div className="fixed inset-0 z-[70] flex items-center justify-center bg-black/60 backdrop-blur-[2px] px-4">
      <div className="w-full max-w-sm rounded-2xl border border-border bg-[#121317]/95 p-5 shadow-xl ring-1 ring-primary/20">
        <div className="mb-4 flex items-center justify-between">
          <h3 className="text-base font-semibold text-white">保存为自选表</h3>
          <button
            type="button"
            onClick={onClose}
            className="grid size-7 place-items-center rounded-full bg-white/5 text-white/40 hover:bg-white/10 hover:text-white/70 transition"
          >
            ×
          </button>
        </div>
        <div className="space-y-3">
          <input
            value={name}
            onChange={(e) => onNameChange(e.target.value)}
            placeholder="输入自选表名称"
            maxLength={64}
            autoFocus
            onKeyDown={(e) => e.key === 'Enter' && !saving && onSave()}
            className="w-full rounded-xl border border-white/15 bg-white/5 px-3 py-2 text-sm text-white outline-none transition focus:border-primary/50 focus:ring-1 focus:ring-primary/30"
          />
          <p className="text-xs text-white/40">
            将当前页 {stockCount} 只股票保存到自选表（单表最多 500 只）
          </p>
          {error && (
            <p className="text-xs text-red-300">{error}</p>
          )}
          <div className="flex justify-end gap-2 pt-1">
            <button
              type="button"
              onClick={onClose}
              className="rounded-lg border border-white/15 px-3 py-1.5 text-xs text-white/60 transition hover:border-white/25 hover:text-white/80"
            >
              取消
            </button>
            <button
              type="button"
              onClick={onSave}
              disabled={saving || !name.trim()}
              className="rounded-lg bg-primary px-4 py-1.5 text-xs font-medium text-black transition hover:bg-primary/90 disabled:cursor-not-allowed disabled:opacity-50"
            >
              {saving ? '保存中...' : '保存'}
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
