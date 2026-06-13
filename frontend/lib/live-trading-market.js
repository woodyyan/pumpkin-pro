const CORE_INDEX_CODES = ['000001', '399001', '399006', 'HSI', 'HSCEI', 'HSTECH']
const SECONDARY_INDEX_CODES = ['000300', '000688', '000016', '399905']
const INDEX_DISPLAY_MAP = {
  '000001': { title: '上证指数', market: 'A股', group: 'core', description: '大盘权重风向标', importance: '主板基准' },
  '399001': { title: '深证成指', market: 'A股', group: 'core', description: '深市宽基代表', importance: '深市总览' },
  '399006': { title: '创业板指', market: 'A股', group: 'core', description: '成长风格温度计', importance: '成长风格' },
  '000300': { title: '沪深300', market: 'A股', group: 'secondary', description: '机构偏好与核心资产', importance: '宽基补充' },
  '000688': { title: '科创50', market: 'A股', group: 'secondary', description: '硬科技与高研发资产', importance: '科技成长' },
  '000016': { title: '上证50', market: 'A股', group: 'secondary', description: '超大盘蓝筹表现', importance: '权重风格' },
  '399905': { title: '中证500', market: 'A股', group: 'secondary', description: '中盘活跃度参考', importance: '中盘风格' },
  HSI: { title: '恒生指数', market: '港股', group: 'core', description: '港股整体情绪锚点', importance: '港股宽基' },
  HSCEI: { title: '恒生中国企业指数', market: '港股', group: 'core', description: '中资核心资产表现', importance: '中资权重' },
  HSTECH: { title: '恒生科技指数', market: '港股', group: 'core', description: '港股科技风险偏好', importance: '科技主线' },
}
const INDEX_NAME_CODE_MAP = {
  '上证指数': '000001',
  '深证成指': '399001',
  '创业板指': '399006',
  '沪深300': '000300',
  '科创50': '000688',
  '上证50': '000016',
  '中证500': '399905',
  'Hang Seng Index': 'HSI',
  'Hang Seng China Enterprises Index': 'HSCEI',
  'Hang Seng TECH Index': 'HSTECH',
}

function buildMarketState(marketOverviewA, marketOverviewHK) {
  const indexes = [...(marketOverviewA?.indexes || []), ...(marketOverviewHK?.indexes || [])]
  const normalizedIndexes = indexes.map((index) => normalizeIndex(index)).filter(Boolean)
  const byCode = new Map(normalizedIndexes.map((item) => [item.code, item]))

  const coreIndexes = CORE_INDEX_CODES.map((code) => byCode.get(code)).filter(Boolean)
  const secondaryIndexes = SECONDARY_INDEX_CODES.map((code) => byCode.get(code)).filter(Boolean)

  const risingCount = normalizedIndexes.filter((item) => item.changeRate > 0).length
  const fallingCount = normalizedIndexes.filter((item) => item.changeRate < 0).length
  const strongest = [...normalizedIndexes].sort((a, b) => (b.changeRate || -Infinity) - (a.changeRate || -Infinity))[0]
  const weakest = [...normalizedIndexes].sort((a, b) => (a.changeRate || Infinity) - (b.changeRate || Infinity))[0]
  const total = normalizedIndexes.length
  const updatedAt = [marketOverviewA?.ts, marketOverviewHK?.ts].filter(Boolean).sort().at(-1) || ''
  const trendSummary = [marketOverviewA?.trend_summary, marketOverviewHK?.trend_summary].filter(Boolean).join('；')

  return {
    heroStats: [
      {
        label: '覆盖指数',
        value: total > 0 ? `${total} 个` : '--',
        description: '首屏核心 + 扩展风格指数一并观察。',
      },
      {
        label: '上涨 / 下跌',
        value: total > 0 ? `${risingCount} / ${fallingCount}` : '--',
        description: total > 0 ? '用于快速判断市场广度。' : '等待行情数据返回。',
      },
      {
        label: '最强指数',
        value: strongest ? strongest.title : '--',
        description: strongest ? `${formatPercent(strongest.changeRate)}，${strongest.description}` : '暂无可比较数据。',
      },
    ],
    coreIndexes,
    secondaryIndexes,
    insights: buildMarketInsights({ coreIndexes, secondaryIndexes, strongest, weakest, risingCount, total, trendSummary }),
    updatedAt,
    trendSummary,
  }
}

function buildMarketInsights({ coreIndexes, secondaryIndexes, strongest, weakest, risingCount, total, trendSummary }) {
  const aCore = coreIndexes.filter((item) => item.market === 'A股')
  const hkCore = coreIndexes.filter((item) => item.market === '港股')
  const aAverage = averageChangeRate(aCore)
  const hkAverage = averageChangeRate(hkCore)
  const styleLeader = secondaryIndexes.length > 0 ? [...secondaryIndexes].sort((a, b) => (b.changeRate || -Infinity) - (a.changeRate || -Infinity))[0] : null

  return [
    {
      title: 'A/H 主市场对比',
      tag: aAverage >= hkAverage ? 'A股更强' : '港股更强',
      accentClass: aAverage >= hkAverage ? 'bg-negative/10 text-negative' : 'bg-positive/10 text-positive',
      description:
        total > 0
          ? `A 股核心指数均值 ${formatPercent(aAverage)}，港股核心指数均值 ${formatPercent(hkAverage)}。适合先看哪一侧的风险偏好更强。`
          : '等待核心指数数据后生成主市场强弱对比。',
    },
    {
      title: '最强风格线索',
      tag: styleLeader ? styleLeader.title : '待补充',
      accentClass: 'bg-primary/15 text-primary',
      description: styleLeader
        ? `${styleLeader.title} 当前 ${formatPercent(styleLeader.changeRate)}，说明 ${styleLeader.description}。这类指数最适合作为补充风格观察。`
        : '数据源暂未返回补充指数，建议优先接入沪深300、科创50、上证50、中证500。',
    },
    {
      title: '市场广度与压力',
      tag: strongest && weakest ? `${strongest.title} ↔ ${weakest.title}` : '等待数据',
      accentClass: 'bg-[var(--color-bg-hover)] text-foreground',
      description:
        trendSummary || total > 0
          ? `${trendSummary || `当前上涨指数 ${risingCount} 个`}。最强的是 ${strongest?.title || '--'}，最弱的是 ${weakest?.title || '--'}。有助于判断是普涨普跌还是结构分化。`
          : '暂无可用指数数据，无法生成广度摘要。',
    },
  ]
}

function normalizeIndex(index) {
  if (!index || (!index.code && !index.name)) return null
  const rawCode = String(index.code || '').trim().toUpperCase()
  const mappedCode = rawCode || INDEX_NAME_CODE_MAP[String(index.name || '').trim()] || ''
  if (!mappedCode) return null

  const config = INDEX_DISPLAY_MAP[mappedCode] || {
    title: formatMarketIndexTitle(index.name, mappedCode),
    market: mappedCode.startsWith('HS') ? '港股' : 'A股',
    group: 'secondary',
    description: '可继续补充说明',
    importance: '补充指数',
  }

  const last = toNumber(index.last)
  const changeRate = toNumber(index.change_rate)
  const changeAmount = pickChangeAmount(index, last, changeRate)
  const trend = normalizeTrendSeries(index)
  if (trend.length < 2) return null
  const chartMeta = buildChartMeta(trend)

  return {
    code: mappedCode,
    title: config.title,
    market: config.market,
    group: config.group,
    description: config.description,
    importance: config.importance,
    last,
    changeRate,
    changeAmount,
    pointLabel: '涨跌点',
    trend,
    chartMeta,
    ts: String(index.ts || '').trim(),
  }
}

function normalizeTrendSeries(index) {
  const candidates = [index.trend_points, index.trendPoints, index.series, index.chart_data, index.sparkline]
  for (const candidate of candidates) {
    const normalized = mapTrendPoints(candidate)
    if (normalized.length >= 2) {
      return normalized
    }
  }
  return []
}

function buildChartMeta(trend) {
  const points = Array.isArray(trend) ? trend : []
  const start = points[0]?.count
  const end = points[points.length - 1]?.count
  const rangePct = Number.isFinite(start) && start !== 0 && Number.isFinite(end) ? (end - start) / start : null
  const pointCount = points.length

  return {
    label: `真实走势 · ${pointCount} 点`,
    pointCount,
    rangePct,
    hasRealTrend: true,
  }
}

function mapTrendPoints(points) {
  if (!Array.isArray(points)) return []
  return points
    .map((point, idx) => {
      if (Array.isArray(point) && point.length >= 2) {
        const date = String(point[0] || '').trim() || `point-${idx + 1}`
        const count = toNumber(point[1])
        if (!Number.isFinite(count)) return null
        return { date, count }
      }
      if (point && typeof point === 'object') {
        const date = String(point.date || point.ts || point.label || '').trim() || `point-${idx + 1}`
        const count = toNumber(point.count ?? point.value ?? point.close ?? point.price ?? point.last)
        if (!Number.isFinite(count)) return null
        return { date, count }
      }
      const count = toNumber(point)
      if (!Number.isFinite(count)) return null
      return { date: `point-${idx + 1}`, count }
    })
    .filter(Boolean)
}

function pickChangeAmount(index, last, changeRate) {
  const explicit = toNumber(index.change_amount)
  if (Number.isFinite(explicit)) return explicit
  if (!Number.isFinite(last) || !Number.isFinite(changeRate)) return null
  const prev = changeRate === -1 ? last : last / (1 + changeRate)
  if (!Number.isFinite(prev)) return null
  return Number((last - prev).toFixed(2))
}

function inferExchange(index) {
  const rawCode = String(index?.code || '').trim().toUpperCase()
  if (rawCode.startsWith('HS')) return 'HKEX'
  return 'SSE'
}

function averageChangeRate(items) {
  const values = items.map((item) => item.changeRate).filter((value) => Number.isFinite(value))
  if (values.length === 0) return 0
  return values.reduce((sum, value) => sum + value, 0) / values.length
}

function toNumber(value) {
  const num = Number(value)
  return Number.isFinite(num) ? num : null
}

function formatPercent(value) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return '--'
  const num = Number(value) * 100
  const sign = num > 0 ? '+' : ''
  return `${sign}${num.toFixed(2)}%`
}

function formatNumber(value, digits = 2) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return '--'
  return Number(value).toLocaleString('zh-CN', { maximumFractionDigits: digits, minimumFractionDigits: digits })
}

function formatSignedNumber(value, digits = 2) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return '--'
  const num = Number(value)
  const sign = num > 0 ? '+' : ''
  return `${sign}${num.toLocaleString('zh-CN', { maximumFractionDigits: digits, minimumFractionDigits: digits })}`
}

function formatTime(value) {
  if (!value) return '--'
  const date = value instanceof Date ? value : new Date(value)
  if (!(date instanceof Date) || Number.isNaN(date.getTime())) return '--'
  return date.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false })
}

function formatMarketIndexTitle(name, code) {
  const rawName = String(name || '').trim()
  const upperCode = String(code || '').trim().toUpperCase()
  const nameMap = {
    'Hang Seng Index': '恒生指数',
    'Hang Seng China Enterprises Index': '恒生中国企业指数',
    'Hang Seng TECH Index': '恒生科技指数',
  }
  if (nameMap[rawName]) return nameMap[rawName]
  const codeMap = {
    HSI: '恒生指数',
    HSCEI: '恒生中国企业指数',
    HSTECH: '恒生科技指数',
    '000001': '上证指数',
    '399001': '深证成指',
    '399006': '创业板指',
    '000300': '沪深300',
    '000688': '科创50',
    '000016': '上证50',
    '399905': '中证500',
  }
  return codeMap[upperCode] || rawName || upperCode || '--'
}

module.exports = {
  buildMarketInsights,
  buildMarketState,
  formatMarketIndexTitle,
  formatNumber,
  formatPercent,
  formatSignedNumber,
  formatTime,
  inferExchange,
  mapTrendPoints,
  normalizeIndex,
  normalizeTrendSeries,
  pickChangeAmount,
}
