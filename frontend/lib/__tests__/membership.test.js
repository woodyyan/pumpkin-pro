import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

import {
  MEMBERSHIP_COMPARE_ROWS,
  MEMBERSHIP_FAQS,
  MEMBERSHIP_FEEDBACK_PATH,
  MEMBERSHIP_PLANS,
  MEMBERSHIP_PRELAUNCH,
  buildMembershipMenuLabel,
  resolveMembershipEntryState,
} from '../membership.js'

describe('membership plans config', () => {
  it('offers exactly two plans: ¥39/month and ¥390/year, no other tiers', () => {
    assert.equal(MEMBERSHIP_PLANS.length, 2)

    const monthly = MEMBERSHIP_PLANS.find((plan) => plan.key === 'monthly')
    const yearly = MEMBERSHIP_PLANS.find((plan) => plan.key === 'yearly')

    assert.equal(monthly.price, 39)
    assert.equal(monthly.unit, '月')
    assert.equal(yearly.price, 390)
    assert.equal(yearly.unit, '年')
    assert.ok(yearly.badge, 'yearly plan must show discount badge')
  })

  it('tells members they get 5 AI reports per month in the compare table', () => {
    const reportRow = MEMBERSHIP_COMPARE_ROWS.find((row) => row.feature === 'AI 研报')
    assert.ok(reportRow, 'compare table must include AI 研报 row')
    assert.ok(reportRow.member.includes('5 份'), 'member cell must state 5 reports per month')
  })

  it('covers the full member benefit scope in the compare table', () => {
    const features = MEMBERSHIP_COMPARE_ROWS.map((row) => row.feature)
    assert.deepEqual(features, [
      'AI 研报',
      'AI 功能',
      'AI 选股',
      '模拟组合',
      '绩效归因分析',
      '组合风险分析',
      '因子实验室',
      '卧龙推荐',
      '交易信号配置',
    ])

    const rowByFeature = Object.fromEntries(MEMBERSHIP_COMPARE_ROWS.map((row) => [row.feature, row]))
    assert.ok(rowByFeature['AI 功能'].free.includes('每日 3 次'))
    assert.ok(rowByFeature['AI 功能'].member.includes('无限次'))
    assert.ok(rowByFeature['AI 选股'].member.includes('无限次'))
    assert.ok(rowByFeature['模拟组合'].member.includes('因子组合'))
    assert.ok(rowByFeature['绩效归因分析'].member.includes('高级版'))
    assert.ok(rowByFeature['组合风险分析'].free.includes('无此功能'))
    assert.ok(rowByFeature['因子实验室'].member.includes('因子选股'))
    assert.ok(rowByFeature['卧龙推荐'].free.includes('无此功能'))
    assert.ok(rowByFeature['交易信号配置'].member.includes('邮件推送'))
    assert.ok(rowByFeature['交易信号配置'].free.includes('3 个信号'))
  })

  it('mentions Beijing-time reset for free quota and keeps FAQ concise', () => {
    const text = MEMBERSHIP_COMPARE_ROWS.map((row) => row.free).join('\n')
    assert.ok(text.includes('北京时间'))
    assert.ok(MEMBERSHIP_FAQS.length >= 3)
    // 不再向用户解释 AI 投研阶梯关系
    const faqText = MEMBERSHIP_FAQS.map((faq) => faq.question + faq.answer).join('\n')
    assert.ok(!faqText.includes('投研阶梯'), 'FAQ must not explain the AI tier system')
  })

  it('points feedback to the settings feedback section', () => {
    assert.equal(MEMBERSHIP_FEEDBACK_PATH, '/settings#feedback')
  })
})

describe('resolveMembershipEntryState', () => {
  it('returns loading when auth is not ready', () => {
    assert.equal(resolveMembershipEntryState({ ready: false, isLoggedIn: true }), 'loading')
    assert.equal(resolveMembershipEntryState({ ready: false, isLoggedIn: false }), 'loading')
  })

  it('returns guest when not logged in', () => {
    assert.equal(resolveMembershipEntryState({ ready: true, isLoggedIn: false }), 'guest')
  })

  it('forces non-member for logged-in users during prelaunch', () => {
    assert.equal(MEMBERSHIP_PRELAUNCH, true)
    assert.equal(resolveMembershipEntryState({ ready: true, isLoggedIn: true, isMember: false }), 'non-member')
    // 即使未来状态源误传 isMember=true，预发布期也必须按非会员展示
    assert.equal(resolveMembershipEntryState({ ready: true, isLoggedIn: true, isMember: true }), 'non-member')
  })

  it('uses default params safely', () => {
    assert.equal(resolveMembershipEntryState(), 'guest')
    assert.equal(resolveMembershipEntryState({ ready: false }), 'loading')
  })
})

describe('buildMembershipMenuLabel', () => {
  it('shows plain label for non-member and guest', () => {
    assert.equal(buildMembershipMenuLabel('non-member'), '会员中心')
    assert.equal(buildMembershipMenuLabel('guest'), '会员中心')
  })

  it('shows expiry for member state when provided', () => {
    assert.equal(
      buildMembershipMenuLabel('member', { expiresAt: '2026-08-18' }),
      '会员中心 · 有效期至 2026-08-18'
    )
  })

  it('falls back to plain label for member without expiry', () => {
    assert.equal(buildMembershipMenuLabel('member'), '会员中心')
  })
})
