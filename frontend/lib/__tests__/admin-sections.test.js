import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

import { resolveAdminSectionApis } from '../admin-sections.js'

describe('resolveAdminSectionApis()', () => {
  it('returns only overview APIs for overview page', () => {
    assert.deepEqual(resolveAdminSectionApis('overview'), [
      '/api/admin/stats',
      '/api/admin/analytics',
      '/api/admin/device-analytics',
      '/api/admin/user-funnel',
    ])
  })

  it('returns data job APIs for data page', () => {
    assert.deepEqual(resolveAdminSectionApis('data'), [
      '/api/admin/company-profiles',
      '/api/admin/factor-lab/pipeline/status',
      '/api/admin/quadrant-overview',
      '/api/admin/quadrant-logs',
      '/api/admin/compute-status',
      '/api/admin/ranking-portfolio-status',
    ])
  })

  it('returns ai APIs for ai page', () => {
    assert.deepEqual(resolveAdminSectionApis('ai'), [
      '/api/admin/ai-config',
      '/api/admin/stats',
      '/api/admin/ai-usage',
      '/api/admin/ai-picker/status',
    ])
  })

  it('returns ops APIs for ops page', () => {
    assert.deepEqual(resolveAdminSectionApis('ops'), [
      '/api/admin/payments/config',
      '/api/admin/payments',
      '/api/admin/backup-status',
      '/api/admin/backup-history',
      '/api/admin/backup-stats',
      '/api/admin/system-health',
      '/api/admin/feedback',
    ])
  })

  it('returns empty array for unknown sections', () => {
    assert.deepEqual(resolveAdminSectionApis('unknown'), [])
  })
})
