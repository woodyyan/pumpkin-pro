import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

import {
  buildBackupHistoryNote,
  buildBackupJobBanner,
  buildBackupStatusCards,
  formatBackupBytes,
  formatBackupDuration,
  getBackupCosMeta,
  resolveBackupTriggerButton,
  shouldPollBackupStatus,
} from '../backup-ui.js'

describe('formatBackupBytes()', () => {
  it('formats bytes into human readable units', () => {
    assert.equal(formatBackupBytes(null), '--')
    assert.equal(formatBackupBytes(512), '512B')
    assert.equal(formatBackupBytes(2048), '2.0KB')
    assert.equal(formatBackupBytes(3 * 1024 * 1024), '3.0MB')
  })
})

describe('formatBackupDuration()', () => {
  it('formats duration into ms or seconds', () => {
    assert.equal(formatBackupDuration(0), '--')
    assert.equal(formatBackupDuration(320), '320ms')
    assert.equal(formatBackupDuration(1830), '1.8s')
  })
})

describe('getBackupCosMeta()', () => {
  it('returns fallback for unknown status', () => {
    assert.deepEqual(getBackupCosMeta('unknown'), { label: 'unknown', tone: 'text-white/45', symbol: '·' })
  })
})

describe('buildBackupStatusCards()', () => {
  it('builds card values for split local and COS state', () => {
    const result = buildBackupStatusCards({
      status: 'partial',
      last_trigger_type: 'manual',
      pumpkin_size_bytes: 1024,
      cache_a_size_bytes: 2048,
      cache_hk_size_bytes: 3072,
      cos_status: 'partial',
      duration_ms: 2120,
    }, { cloud_enabled: true })

    assert.equal(result.overall.value, 'partial')
    assert.equal(result.overall.sub, '手动触发')
    assert.equal(result.sizes.pumpkin, '1.0KB')
    assert.equal(result.cos.value, '⚠ 部分同步')
    assert.equal(result.cos.sub, '已配置')
    assert.equal(result.duration, '2.1s')
  })
})

describe('buildBackupJobBanner()', () => {
  it('shows running banner when background job is active', () => {
    const result = buildBackupJobBanner({
      current_job_status: 'running',
      current_job_trigger_type: 'manual',
      current_job_message: '上传 3 个备份文件到 COS',
    })
    assert.equal(result.tone, 'info')
    assert.equal(result.text, '手动触发进行中：上传 3 个备份文件到 COS')
  })

  it('shows cooldown banner when idle but blocked', () => {
    const result = buildBackupJobBanner({ current_job_status: 'idle', next_allowed_at: '2026-05-07 18:00:00' })
    assert.equal(result.tone, 'muted')
    assert.equal(result.text, '冷却中，下一次可触发时间 2026-05-07 18:00:00')
  })
})

describe('shouldPollBackupStatus()', () => {
  it('polls only queued and running jobs', () => {
    assert.equal(shouldPollBackupStatus({ current_job_status: 'queued' }), true)
    assert.equal(shouldPollBackupStatus({ current_job_status: 'running' }), true)
    assert.equal(shouldPollBackupStatus({ current_job_status: 'success' }), false)
  })
})

describe('resolveBackupTriggerButton()', () => {
  it('disables button when triggering, running or cooldown applies', () => {
    assert.deepEqual(resolveBackupTriggerButton({ triggering: true }), { disabled: true, label: '提交中...' })
    assert.deepEqual(resolveBackupTriggerButton({ status: { current_job_status: 'running' } }), { disabled: true, label: '后台执行中...' })
    assert.deepEqual(resolveBackupTriggerButton({ status: { current_job_status: 'idle', next_allowed_at: '2026-05-07 18:00:00' } }), { disabled: true, label: '冷却中...' })
  })

  it('keeps button enabled when no blocker exists', () => {
    assert.deepEqual(resolveBackupTriggerButton({ status: { current_job_status: 'idle' } }), { disabled: false, label: '🔄 立即备份' })
  })
})

describe('buildBackupHistoryNote()', () => {
  it('prioritizes local and COS errors before integrity status', () => {
    assert.equal(buildBackupHistoryNote({ error_msg: 'pumpkin failed' }), 'pumpkin failed')
    assert.equal(buildBackupHistoryNote({ cos_status: 'failed', cos_error_msg: 'cos timeout' }), 'cos timeout')
    assert.equal(buildBackupHistoryNote({ integrity_check: 'ok' }), '✅ 校验通过')
  })
})
