import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const pageSource = readFileSync(new URL('../../pages/admin.js', import.meta.url), 'utf8')

describe('admin ai config panel integration', () => {
  it('renders the AI config panel ahead of AI usage stats', () => {
    assert.match(pageSource, /AIProviderConfigPanel/)
    assert.match(pageSource, /\uD83E\uDD16 AI 模型配置/)
    const configIndex = pageSource.indexOf('AIProviderConfigPanel onUnauthorized={onLogout}')
    const usageIndex = pageSource.indexOf('🤖 AI 调用统计')
    assert.ok(configIndex >= 0, 'missing AI config panel mount')
    assert.ok(usageIndex >= 0, 'missing AI usage heading')
    assert.ok(configIndex < usageIndex, 'AI config panel should appear before usage stats')
  })

  it('wires load, save and test requests to dedicated admin endpoints', () => {
    assert.match(pageSource, /\/api\/admin\/ai-config/)
    assert.match(pageSource, /\/api\/admin\/ai-config\/test/)
    assert.match(pageSource, /method: 'PUT'/)
    assert.match(pageSource, /method: 'POST'/)
    assert.match(pageSource, /恢复已保存值/)
    assert.match(pageSource, /留空表示保持当前 key/)
  })

  it('shows effective source and health states for admins', () => {
    assert.match(pageSource, /AI_CONFIG_SOURCE_LABELS/)
    assert.match(pageSource, /AI_CONFIG_STATUS_META/)
    assert.match(pageSource, /当前生效模型：/)
    assert.match(pageSource, /当前状态/)
    assert.match(pageSource, /最近测试/)
  })
})
