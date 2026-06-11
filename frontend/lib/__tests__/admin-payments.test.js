import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

import {
  findAdminPaymentById,
  isAdminPaymentPollable,
  resolveAdminPaymentDraftForMethod,
  resolveAdminPaymentMethodMeta,
  resolveAdminPaymentMethodOptions,
  resolveAdminPaymentPollingState,
  resolveAdminSelectedPaymentId,
} from '../admin-payments.js'

describe('resolveAdminSelectedPaymentId()', () => {
  it('keeps the current selected id when it still exists in the latest list', () => {
    const payments = [
      { id: 'pay_failed', status: 'failed' },
      { id: 'pay_latest', status: 'succeeded' },
    ]

    assert.equal(resolveAdminSelectedPaymentId(payments, 'pay_latest'), 'pay_latest')
  })

  it('falls back to the first payment only when the current selection is missing', () => {
    const payments = [
      { id: 'pay_failed', status: 'failed' },
      { id: 'pay_latest', status: 'succeeded' },
    ]

    assert.equal(resolveAdminSelectedPaymentId(payments, ''), 'pay_failed')
    assert.equal(resolveAdminSelectedPaymentId(payments, 'pay_deleted'), 'pay_failed')
    assert.equal(resolveAdminSelectedPaymentId([], 'pay_latest'), '')
  })
})

describe('resolveAdminPaymentMethodOptions()', () => {
  it('prefers backend-provided admin test methods and preserves enablement metadata', () => {
    const config = {
      admin_test_payment_methods: [
        { code: 'card', enabled: true, supported_currencies: ['cny'] },
        { code: 'alipay', enabled: true, supported_currencies: ['cny', 'hkd'] },
        { code: 'wechat_pay', enabled: false, supported_currencies: ['cny'] },
      ],
    }

    const options = resolveAdminPaymentMethodOptions(config)
    assert.equal(options.length, 3)
    assert.equal(options[1].code, 'alipay')
    assert.equal(options[1].enabled, true)
    assert.equal(options[2].enabled, false)
  })

  it('falls back to local payment method metadata when config has no detailed list', () => {
    const options = resolveAdminPaymentMethodOptions({ allowed_payment_methods: ['card', 'wechat_pay'] })
    assert.equal(options.length, 3)
    assert.equal(options[0].enabled, true)
    assert.equal(options[1].enabled, false)
    assert.equal(options[2].enabled, true)
  })
})

describe('resolveAdminPaymentMethodMeta()', () => {
  it('returns the selected payment method description for UI guidance', () => {
    const config = {
      admin_test_payment_methods: [
        { code: 'card', enabled: true, testing_note: 'card note' },
        { code: 'wechat_pay', enabled: true, testing_note: 'wechat note', checkout_flow: 'qr_code' },
      ],
    }

    assert.deepEqual(resolveAdminPaymentMethodMeta(config, 'wechat_pay'), {
      code: 'wechat_pay',
      enabled: true,
      testing_note: 'wechat note',
      checkout_flow: 'qr_code',
    })
    assert.equal(resolveAdminPaymentMethodMeta(config, 'alipay'), null)
  })
})

describe('resolveAdminPaymentDraftForMethod()', () => {
  it('keeps the selected currency when it is supported by the chosen payment method', () => {
    const draft = resolveAdminPaymentDraftForMethod(
      { amount_minor: 100, currency: 'hkd', payment_method: 'card', payment_method_types: ['card'] },
      { admin_test_payment_methods: [{ code: 'alipay', enabled: true, supported_currencies: ['cny', 'hkd'], recommended_currency: 'cny' }] },
      'alipay'
    )

    assert.equal(draft.payment_method, 'alipay')
    assert.deepEqual(draft.payment_method_types, ['alipay'])
    assert.equal(draft.currency, 'hkd')
  })

  it('switches to the recommended currency when the current one is unsupported', () => {
    const draft = resolveAdminPaymentDraftForMethod(
      { amount_minor: 100, currency: 'usd', payment_method: 'card', payment_method_types: ['card'] },
      { admin_test_payment_methods: [{ code: 'wechat_pay', enabled: true, supported_currencies: ['cny', 'hkd'], recommended_currency: 'cny' }] },
      'wechat_pay'
    )

    assert.equal(draft.payment_method, 'wechat_pay')
    assert.deepEqual(draft.payment_method_types, ['wechat_pay'])
    assert.equal(draft.currency, 'cny')
  })
})

describe('findAdminPaymentById()', () => {
  it('prefers the selected payment detail before falling back to list items', () => {
    const payload = {
      detail: {
        payment: { id: 'pay_selected', status: 'processing' },
      },
      payments: {
        items: [
          { id: 'pay_selected', status: 'checkout_open' },
          { id: 'pay_other', status: 'succeeded' },
        ],
      },
    }

    assert.deepEqual(findAdminPaymentById(payload, 'pay_selected'), { id: 'pay_selected', status: 'processing' })
    assert.deepEqual(findAdminPaymentById(payload, 'pay_other'), { id: 'pay_other', status: 'succeeded' })
    assert.equal(findAdminPaymentById(payload, 'missing'), null)
  })
})

describe('isAdminPaymentPollable()', () => {
  it('treats in-flight statuses as pollable before session expiry', () => {
    assert.equal(isAdminPaymentPollable({ status: 'initiated' }, 1_000), true)
    assert.equal(isAdminPaymentPollable({ status: 'checkout_open', session_expires_at: '2026-06-05T10:10:00.000Z' }, Date.parse('2026-06-05T10:00:00.000Z')), true)
    assert.equal(isAdminPaymentPollable({ status: 'processing' }, 1_000), true)
  })

  it('stops polling for terminal or expired payments', () => {
    assert.equal(isAdminPaymentPollable({ status: 'succeeded' }, 1_000), false)
    assert.equal(isAdminPaymentPollable({ status: 'checkout_open', session_expires_at: '2026-06-05T10:00:00.000Z' }, Date.parse('2026-06-05T10:00:00.000Z')), false)
    assert.equal(isAdminPaymentPollable({ status: 'checkout_open', session_expires_at: 'invalid-date' }, 1_000), true)
  })
})

describe('resolveAdminPaymentPollingState()', () => {
  it('does not auto-poll historical records when no local polling target exists', () => {
    const payload = {
      payments: {
        items: [{ id: 'pay_1', status: 'checkout_open' }],
      },
    }

    assert.equal(resolveAdminPaymentPollingState(payload, ''), 'idle')
  })

  it('returns unknown until the newly created payment appears in resource data', () => {
    const payload = {
      payments: {
        items: [{ id: 'pay_existing', status: 'succeeded' }],
      },
    }

    assert.equal(resolveAdminPaymentPollingState(payload, 'pay_new'), 'unknown')
  })

  it('polls only the targeted in-flight payment and stops after completion', () => {
    const inFlightPayload = {
      detail: {
        payment: { id: 'pay_2', status: 'processing' },
      },
      payments: {
        items: [{ id: 'pay_2', status: 'checkout_open' }],
      },
    }
    const completedPayload = {
      detail: {
        payment: { id: 'pay_2', status: 'succeeded' },
      },
      payments: {
        items: [{ id: 'pay_2', status: 'succeeded' }],
      },
    }

    assert.equal(resolveAdminPaymentPollingState(inFlightPayload, 'pay_2'), 'poll')
    assert.equal(resolveAdminPaymentPollingState(completedPayload, 'pay_2'), 'stop')
  })
})
