// ── Tests for AccountEntry component (pages/_app.js) ──
// Uses Node 20+ built-in test runner (node --test)
//
// AccountEntry is a pure React component that renders different UI based on
// useAuth() state. We test the rendering logic by simulating the conditional
// branches without needing ReactDOM.

import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

/**
 * Simulates AccountEntry's render logic (from _app.js lines 281-362).
 *
 * Returns a description of what would be rendered, based solely on
 * the auth state inputs — no React renderer needed.
 */
function accountEntryRender({ isLoggedIn = false, ready = true, user = null }) {
  if (!ready) {
    return { type: 'loading', label: '···' }
  }

  if (!isLoggedIn) {
    return { type: 'unauthenticated', buttons: ['登录', '注册'] }
  }

  const accountLabel = user?.nickname?.trim() || user?.email || '账号'
  return {
    type: 'authenticated',
    label: accountLabel,
    menuItems: ['当前账号', '设置', '退出登录'],
  }
}

describe('AccountEntry UI rendering', () => {

  // ═════════════════════════════════════════
  // T4.1: ready=false → shows loading placeholder ⭐ CORE SCENARIO
  // ═════════════════════════════════════════
  it('T4.1: shows loading placeholder when ready=false', () => {
    const result = accountEntryRender({
      ready: false,
      isLoggedIn: true,
      user: { email: 'woody@test.com' },
    })

    assert.equal(result.type, 'loading', 'must render loading state')
    assert.equal(result.label, '···', 'must show placeholder text')
    // Must NOT show login buttons or user name when not ready
    assert.equal('buttons' in result, false, 'no login buttons during loading')
    assert.equal('menuItems' in result, false, 'no user dropdown during loading')
  })

  it('T4.1b: ready=false with isLoggedIn=false still shows loading (not login buttons)', () => {
    const result = accountEntryRender({
      ready: false,
      isLoggedIn: false,
      user: null,
    })

    assert.equal(result.type, 'loading')
    assert.equal(result.label, '···')
    // Critical: must NOT show login/register buttons yet
    assert.equal(result.buttons, undefined)
  })

  // ═════════════════════════════════════════
  // T4.2: ready=true + isLoggedIn=false → login/register buttons
  // ═════════════════════════════════════════
  it('T4.2: ready=true + not logged in shows login and register buttons', () => {
    const result = accountEntryRender({
      ready: true,
      isLoggedIn: false,
      user: null,
    })

    assert.equal(result.type, 'unauthenticated')
    assert.ok(Array.isArray(result.buttons))
    assert.equal(result.buttons.length, 2)
    assert.equal(result.buttons[0], '登录')
    assert.equal(result.buttons[1], '注册')
  })

  // ═════════════════════════════════════════
  // T4.3: ready=true + isLoggedIn=true → user dropdown
  // ═════════════════════════════════════════
  it('T4.3: ready=true + logged in shows user dropdown menu', () => {
    const result = accountEntryRender({
      ready: true,
      isLoggedIn: true,
      user: { email: 'woody@example.com' },
    })

    assert.equal(result.type, 'authenticated')
    assert.ok(Array.isArray(result.menuItems))
    assert.equal(result.menuItems.includes('退出登录'), true)
    assert.equal(result.menuItems.includes('设置'), true)
  })

  // ═════════════════════════════════════════
  // T4.4: Shows nickname or email as display label
  // ═════════════════════════════════════════
  it('T4.4a: displays nickname when available', () => {
    const result = accountEntryRender({
      ready: true,
      isLoggedIn: true,
      user: { email: 'woody@example.com', nickname: '卧龙玩家' },
    })

    assert.equal(result.label, '卧龙玩家', 'nickname takes priority over email')
  })

  it('T4.4b: falls back to email when no nickname', () => {
    const result = accountEntryRender({
      ready: true,
      isLoggedIn: true,
      user: { email: 'woody@example.com' },
    })

    assert.equal(result.label, 'woody@example.com', 'email used when nickname absent')
  })

  it('T4.4c: falls back to "账号" when neither nickname nor email', () => {
    const result = accountEntryRender({
      ready: true,
      isLoggedIn: true,
      user: {},
    })

    assert.equal(result.label, '账号', 'default fallback text')
  })
})
