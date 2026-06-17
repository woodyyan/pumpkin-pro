import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const sectionsSource = readFileSync(new URL('../../components/admin/AdminSections.js', import.meta.url), 'utf8')

describe('admin lazy loading boundaries', () => {
  it('keeps overview page free of payments, backup, ai config and factor pipeline mounts', () => {
    const overviewBlock = sectionsSource.slice(
      sectionsSource.indexOf('export function AdminOverviewPage'),
      sectionsSource.indexOf('export function AIUsageAdminPanel')
    )

    assert.doesNotMatch(overviewBlock, /AdminPaymentsPanel/)
    assert.doesNotMatch(overviewBlock, /BackupPanel/)
    assert.doesNotMatch(overviewBlock, /AIProviderConfigPanel/)
    assert.doesNotMatch(overviewBlock, /FactorLabPipelinePanel/)
    assert.doesNotMatch(overviewBlock, /AIPickerAdminPanel/)
    assert.doesNotMatch(overviewBlock, /QuadrantAdminPanel/)
  })

  it('loads AI usage from a dedicated panel instead of overview fetch bundle', () => {
    const overviewBlock = sectionsSource.slice(
      sectionsSource.indexOf('export function AdminOverviewPage'),
      sectionsSource.indexOf('export function AIUsageAdminPanel')
    )
    const aiUsageBlock = sectionsSource.slice(
      sectionsSource.indexOf('export function AIUsageAdminPanel'),
      sectionsSource.indexOf('const QUADRANT_LABELS')
    )

    assert.doesNotMatch(overviewBlock, /\/api\/admin\/ai-usage\?days=30&limit=120/)
    assert.match(aiUsageBlock, /\/api\/admin\/ai-usage\?days=30&limit=120/)
  })
})
