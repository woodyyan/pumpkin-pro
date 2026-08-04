import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { createRequire } from 'node:module'
import { readFileSync } from 'node:fs'

const requireFromCwd = createRequire(process.cwd() + '/')
const { parse } = requireFromCwd('next/dist/compiled/babel/parser')
const pageSource = readFileSync(new URL('../../pages/signals.js', import.meta.url), 'utf8')
const panelSource = readFileSync(new URL('../../components/SignalWebhookPanel.js', import.meta.url), 'utf8')

describe('signals page syntax', () => {
  it('parses as valid JSX', () => {
    assert.doesNotThrow(() => {
      parse(pageSource, { sourceType: 'module', plugins: ['jsx'] })
      parse(panelSource, { sourceType: 'module', plugins: ['jsx'] })
    })
  })
})

describe('signals page structure', () => {
  it('uses the signal center API surface (templates/subscriptions/events)', () => {
    assert.ok(pageSource.includes("requestJson('/api/signal/templates')"))
    assert.ok(pageSource.includes("requestJson('/api/signal/subscriptions')"))
    assert.ok(pageSource.includes("requestJson('/api/signal/events?limit=100')"))
    assert.ok(pageSource.includes("requestJson('/api/signal/events/unread-count')"))
    assert.ok(pageSource.includes("requestJson('/api/signal/events/mark-read'"))
    // 不再依赖旧单股配置 API。
    assert.ok(!pageSource.includes('/api/signal-configs'))
  })

  it('keeps the five-section information architecture', () => {
    // 概览条
    assert.ok(pageSource.includes('已开启信号'))
    assert.ok(pageSource.includes('今日触发'))
    assert.ok(pageSource.includes('未读提醒'))
    // 推荐引导（P0 空状态第一触点）
    assert.ok(pageSource.includes('开启你的第一个信号'))
    // 我的信号 / 信号记录 / 通知设置
    assert.ok(pageSource.includes('我的信号'))
    assert.ok(pageSource.includes('信号记录'))
    assert.ok(pageSource.includes('通知设置'))
  })

  it('keeps dual-state semantics visible to users', () => {
    assert.ok(pageSource.includes('intraday_provisional'))
    assert.ok(pageSource.includes('close_confirmed'))
    assert.ok(pageSource.includes('收盘后还会自动做一次收盘确认评估'))
    assert.ok(pageSource.includes('盘中试算'))
  })

  it('supports quick enable via query param and instant toggle', () => {
    // ?symbol= 跳入自动打开新增弹窗（个股详情页/自选股入口）
    assert.ok(pageSource.includes('router.query.symbol'))
    // 开关即点即生效：乐观更新 + 失败回滚
    assert.ok(pageSource.includes('is_enabled: !sub.is_enabled'))
    assert.ok(pageSource.includes('切换信号状态失败'))
  })

  it('keeps compliance risk copy on page and provisional note', () => {
    assert.ok(pageSource.includes('不构成投资建议'))
    assert.ok(pageSource.includes('收盘前可能反转'))
  })

  it('drives the new-subscription form by template param schema', () => {
    assert.ok(pageSource.includes('param_schema.map'))
    assert.ok(pageSource.includes('validateTemplateParamDraft'))
    assert.ok(pageSource.includes('buildTemplateParamDraft'))
    // 策略模板走策略库 active 列表
    assert.ok(pageSource.includes("requestJson('/api/strategies/active')"))
  })
})
