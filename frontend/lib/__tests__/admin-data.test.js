import { beforeEach, describe, it } from 'node:test'
import assert from 'node:assert/strict'

import {
  clearAdminResourceCache,
  handleAdminActionError,
  isAdminUnauthorized,
  readFreshAdminResourceCache,
  resolveAdminErrorMessage,
  resolveAdminPollInterval,
  shouldThrottleAdminRequest,
  syncAdminResourceCache,
} from '../admin-data.js'

beforeEach(() => {
  clearAdminResourceCache()
})

describe('isAdminUnauthorized()', () => {
  it('detects 401 status from numbers and numeric strings', () => {
    assert.equal(isAdminUnauthorized({ status: 401 }), true)
    assert.equal(isAdminUnauthorized({ status: '401' }), true)
    assert.equal(isAdminUnauthorized({ status: 403 }), false)
    assert.equal(isAdminUnauthorized(null), false)
  })
})

describe('resolveAdminErrorMessage()', () => {
  it('prefers trimmed error message over fallback', () => {
    assert.equal(resolveAdminErrorMessage({ message: '  保存失败  ' }, 'fallback'), '保存失败')
  })

  it('falls back when message is missing or blank', () => {
    assert.equal(resolveAdminErrorMessage({ message: '   ' }, 'fallback'), 'fallback')
    assert.equal(resolveAdminErrorMessage({}, 'fallback'), 'fallback')
  })
})

describe('handleAdminActionError()', () => {
  it('triggers unauthorized handler and returns empty string on 401', () => {
    const calls = []
    const result = handleAdminActionError({ status: 401, message: 'expired' }, (error) => calls.push(error), 'fallback')
    assert.equal(result, '')
    assert.equal(calls.length, 1)
    assert.equal(calls[0].message, 'expired')
  })

  it('returns resolved error message for non-401 errors', () => {
    const result = handleAdminActionError({ status: 500, message: '服务异常' }, () => { throw new Error('should not run') }, 'fallback')
    assert.equal(result, '服务异常')
  })
})

describe('resource cache helpers', () => {
  it('stores and reads fresh cache entries', () => {
    syncAdminResourceCache('stats', { total: 3 }, 1_000)
    assert.deepEqual(readFreshAdminResourceCache('stats', 10_000, 5_000), { data: { total: 3 }, updatedAt: 1_000 })
  })

  it('treats expired entries as stale', () => {
    syncAdminResourceCache('stats', { total: 3 }, 1_000)
    assert.equal(readFreshAdminResourceCache('stats', 500, 2_000), null)
  })

  it('deletes cache entry when syncing null data', () => {
    syncAdminResourceCache('stats', { total: 3 }, 1_000)
    syncAdminResourceCache('stats', null, 2_000)
    assert.equal(readFreshAdminResourceCache('stats', 10_000, 2_000), null)
  })

  it('clears one key or the whole cache', () => {
    syncAdminResourceCache('stats', { total: 3 }, 1_000)
    syncAdminResourceCache('backup', { total: 7 }, 1_000)
    clearAdminResourceCache('stats')
    assert.equal(readFreshAdminResourceCache('stats', 10_000, 2_000), null)
    assert.deepEqual(readFreshAdminResourceCache('backup', 10_000, 2_000), { data: { total: 7 }, updatedAt: 1_000 })
    clearAdminResourceCache()
    assert.equal(readFreshAdminResourceCache('backup', 10_000, 2_000), null)
  })
})

describe('shouldThrottleAdminRequest()', () => {
  it('returns true only inside the configured interval', () => {
    assert.equal(shouldThrottleAdminRequest(10_000, 5_000, 12_000), true)
    assert.equal(shouldThrottleAdminRequest(10_000, 5_000, 15_500), false)
    assert.equal(shouldThrottleAdminRequest(0, 5_000, 12_000), false)
    assert.equal(shouldThrottleAdminRequest(10_000, 0, 12_000), false)
  })
})

describe('resolveAdminPollInterval()', () => {
  it('supports numeric and function poll values', () => {
    assert.equal(resolveAdminPollInterval(5_000, null), 5_000)
    assert.equal(resolveAdminPollInterval((data) => data.running ? 2_000 : null, { running: true }), 2_000)
  })

  it('normalizes invalid poll values to null', () => {
    assert.equal(resolveAdminPollInterval(0, null), null)
    assert.equal(resolveAdminPollInterval(-1, null), null)
    assert.equal(resolveAdminPollInterval(() => undefined, {}), null)
  })
})
