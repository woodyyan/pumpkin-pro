const DEFAULT_FEE_RATE_MAP = {
  ASHARE: {
    buy: 0.0003,
    sell: 0.0008,
  },
  HKEX: {
    buy: 0.0013,
    sell: 0.0013,
  },
}

function roundTo(value, digits = 2) {
  const factor = 10 ** digits
  return Math.round((Number(value) + Number.EPSILON) * factor) / factor
}

export function normalizePortfolioExchange(exchange) {
  const normalized = String(exchange || '').trim().toUpperCase()
  if (normalized === 'HKEX' || normalized === 'HK') return 'HKEX'
  if (normalized === 'SSE' || normalized === 'SZSE' || normalized === 'ASHARE' || normalized === 'SH' || normalized === 'SZ') {
    return 'ASHARE'
  }
  return 'ASHARE'
}

export function getPortfolioSystemDefaultFeeRate({ exchange, action } = {}) {
  const normalizedExchange = normalizePortfolioExchange(exchange)
  const normalizedAction = action === 'sell' ? 'sell' : 'buy'
  return DEFAULT_FEE_RATE_MAP[normalizedExchange]?.[normalizedAction] ?? 0
}

export function getPortfolioDefaultFeeRate({ exchange, action, profile } = {}) {
  const normalizedExchange = normalizePortfolioExchange(exchange)
  const normalizedAction = action === 'sell' ? 'sell' : 'buy'
  const fallback = getPortfolioSystemDefaultFeeRate({ exchange: normalizedExchange, action: normalizedAction })
  if (!profile) return fallback

  const profileFieldMap = {
    ASHARE: {
      buy: 'default_fee_rate_ashare_buy',
      sell: 'default_fee_rate_ashare_sell',
    },
    HKEX: {
      buy: 'default_fee_rate_hk_buy',
      sell: 'default_fee_rate_hk_sell',
    },
  }
  const field = profileFieldMap[normalizedExchange]?.[normalizedAction]
  const value = Number(profile?.[field])
  return Number.isFinite(value) && value > 0 ? value : fallback
}

export function getPortfolioMinimumCommission({ exchange, action } = {}) {
  const normalizedExchange = normalizePortfolioExchange(exchange)
  if (normalizedExchange === 'ASHARE' && (action === 'buy' || action === 'sell')) {
    return 5
  }
  return 0
}

export function calculatePortfolioFeeEstimate({ exchange, action, quantity, price, feeRate } = {}) {
  const normalizedQuantity = Number(quantity)
  const normalizedPrice = Number(price)
  const normalizedRate = Number(feeRate)
  const rawFeeBase = Number.isFinite(normalizedQuantity) && Number.isFinite(normalizedPrice) && Number.isFinite(normalizedRate)
    ? normalizedQuantity * normalizedPrice * normalizedRate
    : 0
  const rawFeeAmount = roundTo(Math.max(rawFeeBase, 0), 2)
  const minimumFeeAmount = roundTo(getPortfolioMinimumCommission({ exchange, action }), 2)
  const minimumApplied = rawFeeBase > 0 && minimumFeeAmount > 0 && rawFeeBase < minimumFeeAmount
  const finalFeeAmount = roundTo(minimumApplied ? minimumFeeAmount : rawFeeAmount, 2)

  return {
    rawFeeAmount,
    finalFeeAmount,
    minimumFeeAmount,
    minimumApplied,
  }
}

export function formatFeeRatePercent(rate) {
  const normalized = Number(rate)
  if (!Number.isFinite(normalized) || normalized < 0) return ''
  const percent = roundTo(normalized * 100, 4)
  if (percent === 0) return '0'
  return String(percent)
}

export function parseFeeRatePercentInput(text) {
  if (text === null || text === undefined) return null
  const normalizedText = String(text).trim()
  if (!normalizedText) return null
  const value = Number(normalizedText)
  if (!Number.isFinite(value)) return null
  return value / 100
}

export function describeFeeRate(rate) {
  const normalized = Number(rate)
  if (!Number.isFinite(normalized) || normalized < 0) return '约 0%'
  const basisPoints = normalized * 10000
  if (basisPoints > 0 && basisPoints < 10) {
    return `约万${roundTo(basisPoints, 2)}`
  }
  return `约 ${formatFeeRatePercent(normalized)}%`
}

export function formatPortfolioFeeAmount(amount, exchange) {
  const prefix = normalizePortfolioExchange(exchange) === 'HKEX' ? 'HK$' : '¥'
  const normalized = Number(amount)
  if (!Number.isFinite(normalized)) return `${prefix}0.00`
  return `${prefix}${normalized.toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`
}

export function describePortfolioFeeEstimate({ exchange, feeEstimate } = {}) {
  if (!feeEstimate) return '填写数量和价格后，系统会自动估算本次手续费。'
  if (feeEstimate.minimumApplied) {
    return `按费率估算约 ${formatPortfolioFeeAmount(feeEstimate.rawFeeAmount, exchange)}，低于最低佣金 ${formatPortfolioFeeAmount(feeEstimate.minimumFeeAmount, exchange)}，本次按 ${formatPortfolioFeeAmount(feeEstimate.finalFeeAmount, exchange)} 结算。`
  }
  return `按本次成交金额估算手续费：${formatPortfolioFeeAmount(feeEstimate.finalFeeAmount, exchange)}`
}
