import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

import {
  findAdminPaymentById,
  isAdminPaymentPollable,
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
