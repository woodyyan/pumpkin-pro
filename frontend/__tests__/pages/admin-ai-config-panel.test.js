import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const pageSource = readFileSync(new URL('../../pages/admin/ai.js', import.meta.url), 'utf8')
const shellSource = readFileSync(new URL('../../components/admin/AdminShell.js', import.meta.url), 'utf8')
const sectionsSource = readFileSync(new URL('../../components/admin/AdminSections.js', import.meta.url), 'utf8')
const adminDataSource = readFileSync(new URL('../../lib/admin-data.js', import.meta.url), 'utf8')

describe('admin ai config panel integration', () => {
  it('renders the AI config panel ahead of AI usage stats', () => {
    assert.match(pageSource, /AdminAIPage/)
    assert.match(sectionsSource, /\uD83E\uDD16 AI 模型配置/)
    const aiPageBlock = sectionsSource.slice(
      sectionsSource.indexOf('export function AdminAIPage'),
      sectionsSource.indexOf('export function AdminOpsPage')
    )
    const configIndex = aiPageBlock.indexOf('AIProviderConfigPanel onUnauthorized={onUnauthorized}')
    const usageIndex = aiPageBlock.indexOf('AIUsageAdminPanel onUnauthorized={onUnauthorized}')
    assert.ok(configIndex >= 0, 'missing AI config panel mount')
    assert.ok(usageIndex >= 0, 'missing AI usage heading')
    assert.ok(configIndex < usageIndex, 'AI config panel should appear before usage stats')
  })

  it('wires load, save and test requests through the shared admin data layer', () => {
    assert.match(sectionsSource, /from '\.\.\/\.\.\/lib\/admin-data'/)
    assert.match(sectionsSource, /useAdminResource\(\{/)
    assert.match(sectionsSource, /adminFetch\('\/api\/admin\/ai-config'/)
    assert.match(sectionsSource, /adminFetch\('\/api\/admin\/ai-config\/test'/)
    assert.match(sectionsSource, /adminFetch\('\/api\/admin\/ai-picker\/latest-run'/)
    assert.match(sectionsSource, /handleAdminActionError/)
    assert.match(sectionsSource, /恢复已保存值/)
    assert.match(sectionsSource, /留空表示保持当前 key/)
    assert.match(adminDataSource, /credentials: 'same-origin'/)
    assert.match(adminDataSource, /export async function adminFetch/)
  })

  it('boots admin session from backend cookie instead of localStorage token', () => {
    assert.match(shellSource, /adminFetch\('\/api\/admin\/session'\)/)
    assert.match(shellSource, /adminFetch\('\/api\/admin\/logout', \{ method: 'POST' \}\)/)
    assert.doesNotMatch(shellSource, /localStorage\.getItem\(ADMIN_SESSION_KEY\)/)
    assert.doesNotMatch(shellSource, /Authorization', `Bearer \$\{session\.tokens\.access_token\}`/)
  })

  it('keeps device stats only in the dedicated device panel', () => {
    assert.doesNotMatch(sectionsSource, /analytics\.devices/)
    assert.doesNotMatch(sectionsSource, /StatCard label="桌面端"/)
    assert.doesNotMatch(sectionsSource, /StatCard label="移动端"/)
    assert.doesNotMatch(sectionsSource, /StatCard label="平板"/)
  })

  it('shows effective source and health states for admins', () => {
    assert.match(sectionsSource, /AI_CONFIG_SOURCE_LABELS/)
    assert.match(sectionsSource, /AI_CONFIG_STATUS_META/)
    assert.match(sectionsSource, /当前生效模型：/)
    assert.match(sectionsSource, /当前状态/)
    assert.match(sectionsSource, /最近测试/)
  })

  it('renders latest AI picker prompt and reasoning detail blocks', () => {
    assert.match(sectionsSource, /最近一场生成详情/)
    assert.match(sectionsSource, /System Prompt/)
    assert.match(sectionsSource, /User Prompt/)
    assert.match(sectionsSource, /AI 思考 \/ 推理过程/)
    assert.match(sectionsSource, /AI 原始返回内容/)
    assert.match(sectionsSource, /最近 10 条生成日志（北京时间）/)
    assert.match(sectionsSource, /更新时间（北京时间）：/)
  })
})
