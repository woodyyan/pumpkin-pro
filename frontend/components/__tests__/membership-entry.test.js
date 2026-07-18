import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

import { buildNavigationState } from '../../lib/navigation.js'

const appSource = readFileSync(new URL('../../pages/_app.js', import.meta.url), 'utf8')
const membershipPageSource = readFileSync(new URL('../../pages/membership.js', import.meta.url), 'utf8')
const dialogSource = readFileSync(new URL('../MembershipComingSoonDialog.js', import.meta.url), 'utf8')
const settingsSource = readFileSync(new URL('../../pages/settings.js', import.meta.url), 'utf8')

describe('membership navigation entry', () => {
  it('adds 会员中心 under the 更多 group linking to /membership', () => {
    const state = buildNavigationState('/live-trading', 0)
    const moreGroup = state.groups.find((group) => group.key === 'more')
    const membershipItem = moreGroup.items.find((item) => item.key === 'membership')

    assert.ok(membershipItem, 'membership item must exist under 更多')
    assert.equal(membershipItem.href, '/membership')
    assert.equal(membershipItem.label, '会员中心')
  })

  it('marks 更多 active on the membership route', () => {
    const state = buildNavigationState('/membership', 0)
    const moreGroup = state.groups.find((group) => group.key === 'more')

    assert.equal(moreGroup.isActive, true)
    assert.equal(state.activeGroupKey, 'more')
  })

  it('keeps 更新日志 under 更多 alongside 会员中心', () => {
    const state = buildNavigationState('/changelog', 0)
    const moreGroup = state.groups.find((group) => group.key === 'more')

    assert.ok(moreGroup.items.some((item) => item.key === 'changelog'))
    assert.ok(moreGroup.items.some((item) => item.key === 'membership'))
  })
})

describe('account area membership entries (pages/_app.js)', () => {
  it('shows a low-key 开通会员 CTA for guests next to login/register', () => {
    assert.ok(appSource.includes('开通会员'))
    // 未登录态 CTA 必须链接到 /membership，而非触发支付
    assert.ok(appSource.includes('href="/membership"'))
  })

  it('shows persistent 开通会员 · ¥39/月 button for logged-in non-members', () => {
    assert.ok(appSource.includes('开通会员 · ¥39/月'))
  })

  it('drives entry visibility through resolveMembershipEntryState', () => {
    assert.ok(appSource.includes('resolveMembershipEntryState'))
    assert.ok(appSource.includes('buildMembershipMenuLabel'))
  })
})

describe('membership page (prelaunch, simplified)', () => {
  it('keeps pricing, compare table, FAQ and feedback sections only', () => {
    assert.ok(membershipPageSource.includes('MEMBERSHIP_PLANS'))
    assert.ok(membershipPageSource.includes('MEMBERSHIP_COMPARE_ROWS'))
    assert.ok(membershipPageSource.includes('MEMBERSHIP_FAQS'))
  })

  it('removes AI-tier relation, benefits and rules sections', () => {
    assert.ok(!membershipPageSource.includes('MEMBERSHIP_AI_RELATION_NOTES'))
    assert.ok(!membershipPageSource.includes('MEMBERSHIP_BENEFITS'))
    assert.ok(!membershipPageSource.includes('MEMBERSHIP_RULES'))
    assert.ok(!membershipPageSource.includes('会员与 AI 投研阶梯'))
  })

  it('links feedback to the settings feedback section instead of email', () => {
    assert.ok(membershipPageSource.includes('MEMBERSHIP_FEEDBACK_PATH'))
    assert.ok(!membershipPageSource.includes('mailto:'), 'page must not use email feedback')
    // 设置页必须存在对应锚点
    assert.ok(settingsSource.includes('id="feedback"'))
  })

  it('opens the placeholder dialog instead of real payment', () => {
    assert.ok(membershipPageSource.includes('MembershipComingSoonDialog'))
    assert.ok(!membershipPageSource.includes('stripe'), 'no payment integration in prelaunch')
    assert.ok(!dialogSource.includes('stripe'), 'dialog must not trigger payment')
  })

  it('dialog communicates 即将上线 and routes feedback to settings', () => {
    assert.ok(dialogSource.includes('即将上线'))
    assert.ok(dialogSource.includes('MEMBERSHIP_FEEDBACK_PATH'))
    assert.ok(!dialogSource.includes('mailto:'), 'dialog must not use email feedback')
  })
})
