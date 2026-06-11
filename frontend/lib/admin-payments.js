const POLLABLE_PAYMENT_STATUSES = new Set(['initiated', 'checkout_open', 'processing'])

const FALLBACK_ADMIN_PAYMENT_METHODS = [
  {
    code: 'card',
    label: '银行卡',
    enabled: true,
    supported_currencies: ['cny', 'hkd'],
    recommended_currency: 'cny',
    checkout_flow: 'hosted_checkout',
    description: 'Stripe Hosted Checkout 直接收单，适合验证银行卡一次性支付链路。',
    testing_note: '银行卡支付会直接在 Stripe Hosted Checkout 内完成，不涉及跳转或扫码。',
  },
  {
    code: 'alipay',
    label: '支付宝',
    enabled: false,
    supported_currencies: ['cny', 'hkd'],
    recommended_currency: 'cny',
    checkout_flow: 'redirect',
    description: '支付宝在 Hosted Checkout 中走跳转授权，仅用于一次性支付内测。',
    testing_note: '选择支付宝后会从 Stripe Checkout 跳转到支付宝授权页，完成授权后再回到 admin。',
  },
  {
    code: 'wechat_pay',
    label: '微信支付',
    enabled: false,
    supported_currencies: ['cny', 'hkd'],
    recommended_currency: 'cny',
    checkout_flow: 'qr_code',
    description: '微信支付在 PC 端 Hosted Checkout 下展示二维码，适合 admin 内测扫码支付链路。',
    testing_note: 'PC 端会展示二维码，请使用手机微信扫码完成测试支付。',
  },
]

export function resolveAdminSelectedPaymentId(payments, currentPaymentId) {
  if (!Array.isArray(payments) || payments.length === 0) {
    return ''
  }
  if (currentPaymentId && payments.some((item) => item?.id === currentPaymentId)) {
    return currentPaymentId
  }
  return payments[0]?.id || ''
}

export function resolveAdminPaymentMethodOptions(config) {
  const configured = Array.isArray(config?.admin_test_payment_methods) ? config.admin_test_payment_methods.filter(Boolean) : []
  if (configured.length) {
    return configured
  }
  const allowed = new Set(
    Array.isArray(config?.allowed_payment_methods)
      ? config.allowed_payment_methods.map((item) => String(item || '').trim()).filter(Boolean)
      : ['card']
  )
  return FALLBACK_ADMIN_PAYMENT_METHODS.map((item) => ({ ...item, enabled: allowed.has(item.code) }))
}

export function resolveAdminPaymentMethodMeta(config, paymentMethod) {
  if (!paymentMethod) return null
  return resolveAdminPaymentMethodOptions(config).find((item) => item?.code === paymentMethod) || null
}

export function resolveAdminPaymentDraftForMethod(draft, config, paymentMethod) {
  const nextMethod = String(paymentMethod || '').trim() || 'card'
  const meta = resolveAdminPaymentMethodMeta(config, nextMethod)
  const supportedCurrencies = Array.isArray(meta?.supported_currencies) && meta.supported_currencies.length
    ? meta.supported_currencies
    : ['cny']
  const normalizedCurrentCurrency = String(draft?.currency || '').trim().toLowerCase()
  const nextCurrency = supportedCurrencies.includes(normalizedCurrentCurrency)
    ? normalizedCurrentCurrency
    : String(meta?.recommended_currency || supportedCurrencies[0] || 'cny').trim().toLowerCase()

  return {
    ...draft,
    payment_method: nextMethod,
    payment_method_types: [nextMethod],
    currency: nextCurrency,
  }
}

export function findAdminPaymentById(payload, paymentId) {
  if (!paymentId || !payload) return null
  const detailPayment = payload?.detail?.payment
  if (detailPayment?.id === paymentId) return detailPayment
  const items = payload?.payments?.items
  if (!Array.isArray(items)) return null
  return items.find((item) => item?.id === paymentId) || null
}

export function isAdminPaymentPollable(payment, now = Date.now()) {
  if (!payment || !POLLABLE_PAYMENT_STATUSES.has(String(payment.status || ''))) {
    return false
  }
  const expiresAt = payment.session_expires_at || payment.expires_at || ''
  if (!expiresAt) return true
  const expiresAtMs = Date.parse(expiresAt)
  if (Number.isNaN(expiresAtMs)) return true
  return expiresAtMs > now
}

export function resolveAdminPaymentPollingState(payload, paymentId, now = Date.now()) {
  if (!paymentId) return 'idle'
  const payment = findAdminPaymentById(payload, paymentId)
  if (!payment) return 'unknown'
  return isAdminPaymentPollable(payment, now) ? 'poll' : 'stop'
}
