import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const pageSource = readFileSync(new URL('../../pages/admin.js', import.meta.url), 'utf8')

describe('admin payments panel integration', () => {
  it('mounts the payments panel in the admin dashboard', () => {
    assert.match(pageSource, /AdminPaymentsPanel/)
    assert.match(pageSource, /💳 支付测试/)
    assert.match(pageSource, /<AdminPaymentsPanel onUnauthorized=\{onLogout\} \/>/)
  })

  it('wires payment admin APIs through the shared admin data layer', () => {
    assert.match(pageSource, /adminFetch\('\/api\/admin\/payments\/config'/)
    assert.match(pageSource, /adminFetch\('\/api\/admin\/payments\?purpose=admin_test&limit=20'/)
    assert.match(pageSource, /adminFetch\('\/api\/admin\/payments\/checkout-sessions'/)
    assert.match(pageSource, /adminFetch\(`\/api\/admin\/payments\/\$\{selectedPaymentId\}`\)/)
    assert.match(pageSource, /adminFetch\(`\/api\/admin\/payments\/\$\{selectedPaymentId\}\/expire`/)
    assert.match(pageSource, /handleAdminActionError/)
  })

  it('limits auto polling to the locally triggered payment flow instead of any historical record', () => {
    assert.match(pageSource, /const \[autoPollPaymentId, setAutoPollPaymentId\] = useState\(''\)/)
    assert.match(pageSource, /const paymentsResource = useAdminResource\(/)
    assert.match(pageSource, /const detailResource = useAdminResource\(/)
    assert.match(pageSource, /key: 'admin:payments:list'/)
    assert.match(pageSource, /key: `admin:payments:detail:\$\{selectedPaymentId \|\| 'none'\}`/)
    assert.match(pageSource, /shouldPoll: \(payload\) => resolveAdminPaymentPollingState\(payload, autoPollPaymentId\) === 'poll'/)
    assert.match(pageSource, /shouldPoll: \(\) => selectedPaymentId === autoPollPaymentId && resolveAdminPaymentPollingState\(paymentsResource\.data, autoPollPaymentId\) === 'poll'/)
    assert.match(pageSource, /setAutoPollPaymentId\(result\.payment_id \|\| ''\)/)
  })

  it('keeps historical failed records from forcing detail refresh on page load', () => {
    assert.match(pageSource, /const nextPaymentId = resolveAdminSelectedPaymentId\(payments, selectedPaymentId\)/)
    assert.match(pageSource, /const selectedPayment = detail\?\.payment \|\| payments\.find\(\(item\) => item\.id === selectedPaymentId\) \|\| null/)
    assert.match(pageSource, /await paymentsResource\.refresh\(\)/)
  })

  it('surfaces explicit test-mode, webhook and card-first guidance', () => {
    assert.match(pageSource, /当前模式/)
    assert.match(pageSource, /Webhook Secret/)
    assert.match(pageSource, /默认只先跑通银行卡支付测试/)
    assert.match(pageSource, /人民币（CNY）/)
    assert.match(pageSource, /Stripe Hosted Checkout/)
  })

  it('shows history and detail timeline affordances', () => {
    assert.match(pageSource, /最近支付记录/)
    assert.match(pageSource, /查看详情/)
    assert.match(pageSource, /事件时间线/)
    assert.match(pageSource, /手动过期/)
    assert.match(pageSource, /打开 Checkout/)
  })
})
