import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const pageSource = readFileSync(new URL('../../pages/admin/ops.js', import.meta.url), 'utf8')
const sectionsSource = readFileSync(new URL('../../components/admin/AdminSections.js', import.meta.url), 'utf8')

describe('admin payments panel integration', () => {
  it('mounts the payments panel in the admin dashboard', () => {
    assert.match(pageSource, /AdminOpsPage/)
    assert.match(sectionsSource, /💳 支付测试/)
    assert.match(sectionsSource, /<AdminPaymentsPanel onUnauthorized=\{onUnauthorized\} \/>/)
  })

  it('wires payment admin APIs through the shared admin data layer', () => {
    assert.match(sectionsSource, /adminFetch\('\/api\/admin\/payments\/config'/)
    assert.match(sectionsSource, /adminFetch\('\/api\/admin\/payments\?purpose=admin_test&limit=20'/)
    assert.match(sectionsSource, /adminFetch\('\/api\/admin\/payments\/checkout-sessions'/)
    assert.match(sectionsSource, /adminFetch\(`\/api\/admin\/payments\/\$\{selectedPaymentId\}`\)/)
    assert.match(sectionsSource, /adminFetch\(`\/api\/admin\/payments\/\$\{selectedPaymentId\}\/expire`/)
    assert.match(sectionsSource, /handleAdminActionError/)
  })

  it('limits auto polling to the locally triggered payment flow instead of any historical record', () => {
    assert.match(sectionsSource, /const \[autoPollPaymentId, setAutoPollPaymentId\] = useState\(''\)/)
    assert.match(sectionsSource, /const paymentsResource = useAdminResource\(/)
    assert.match(sectionsSource, /const detailResource = useAdminResource\(/)
    assert.match(sectionsSource, /key: 'admin:payments:list'/)
    assert.match(sectionsSource, /key: `admin:payments:detail:\$\{selectedPaymentId \|\| 'none'\}`/)
    assert.match(sectionsSource, /shouldPoll: \(payload\) => resolveAdminPaymentPollingState\(payload, autoPollPaymentId\) === 'poll'/)
    assert.match(sectionsSource, /shouldPoll: \(\) => selectedPaymentId === autoPollPaymentId && resolveAdminPaymentPollingState\(paymentsResource\.data, autoPollPaymentId\) === 'poll'/)
    assert.match(sectionsSource, /setAutoPollPaymentId\(result\.payment_id \|\| ''\)/)
  })

  it('keeps historical failed records from forcing detail refresh on page load', () => {
    assert.match(sectionsSource, /const nextPaymentId = resolveAdminSelectedPaymentId\(payments, selectedPaymentId\)/)
    assert.match(sectionsSource, /const selectedPayment = detail\?\.payment \|\| payments\.find\(\(item\) => item\.id === selectedPaymentId\) \|\| null/)
    assert.match(sectionsSource, /await paymentsResource\.refresh\(\)/)
  })

  it('surfaces explicit test-mode and local wallet guidance', () => {
    assert.match(sectionsSource, /当前模式/)
    assert.match(sectionsSource, /Webhook Secret/)
    assert.match(sectionsSource, /现已支持银行卡、支付宝、微信支付三种测试方式/)
    assert.match(sectionsSource, /Stripe Hosted Checkout/)
    assert.match(sectionsSource, /支付宝/)
    assert.match(sectionsSource, /微信支付/)
  })

  it('shows history and detail timeline affordances', () => {
    assert.match(sectionsSource, /最近支付记录/)
    assert.match(sectionsSource, /查看详情/)
    assert.match(sectionsSource, /事件时间线/)
    assert.match(sectionsSource, /手动过期/)
    assert.match(sectionsSource, /打开 Checkout/)
  })

  it('renders config-driven local wallet choices and testing hints', () => {
    assert.match(sectionsSource, /resolveAdminPaymentMethodOptions\(config\)/)
    assert.match(sectionsSource, /resolveAdminPaymentMethodMeta\(config, createDraft\.payment_method/)
    assert.match(sectionsSource, /handleSelectCreateMethod\(event\.target\.value\)/)
    assert.match(sectionsSource, /payment_method: 'card'/)
    assert.match(sectionsSource, /payment_method_types: \['card'\]/)
    assert.match(sectionsSource, /测试提示/)
  })
})
