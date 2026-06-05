const POLLABLE_PAYMENT_STATUSES = new Set(['initiated', 'checkout_open', 'processing'])

export function resolveAdminSelectedPaymentId(payments, currentPaymentId) {
  if (!Array.isArray(payments) || payments.length === 0) {
    return ''
  }
  if (currentPaymentId && payments.some((item) => item?.id === currentPaymentId)) {
    return currentPaymentId
  }
  return payments[0]?.id || ''
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
