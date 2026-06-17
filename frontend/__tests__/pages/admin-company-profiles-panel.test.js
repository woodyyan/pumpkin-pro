import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const pageSource = readFileSync(new URL('../../pages/admin/data.js', import.meta.url), 'utf8')
const sectionsSource = readFileSync(new URL('../../components/admin/AdminSections.js', import.meta.url), 'utf8')

describe('admin company profiles panel integration', () => {
  it('renders company profile management panel in dashboard', () => {
    assert.match(pageSource, /AdminDataPage/)
    assert.match(sectionsSource, /公司资料管理/)
    assert.match(sectionsSource, /一键更新静态资料/)
  })

  it('wires overview, refresh and status endpoints', () => {
    assert.match(sectionsSource, /\/api\/admin\/company-profiles'/)
    assert.match(sectionsSource, /\/api\/admin\/company-profiles\/refresh'/)
    assert.match(sectionsSource, /refresh\.running/)
    assert.match(sectionsSource, /刷新失败：\{refresh\.error\}/)
  })

  it('shows coverage and failure items', () => {
    assert.match(sectionsSource, /coverage_rate/)
    assert.match(sectionsSource, /mapped_count/)
    assert.match(sectionsSource, /applicable_count/)
    assert.match(sectionsSource, /失败项 \/ 待补全/)
    assert.match(sectionsSource, /quality_flags/)
  })

  it('does not render the removed recent factor task details block', () => {
    assert.doesNotMatch(sectionsSource, /最近 10 条任务明细/)
    assert.doesNotMatch(sectionsSource, /recent_task_runs/)
  })
})
