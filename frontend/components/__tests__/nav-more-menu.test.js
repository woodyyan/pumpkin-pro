import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

import { buildNavigationState } from '../../lib/navigation.js'

const navDropdownSource = readFileSync(new URL('../NavDropdown.js', import.meta.url), 'utf8')

describe('desktop and overflow navigation state', () => {
  it('keeps changelog under 更多 and mirrors the unread badge to the group trigger', () => {
    const state = buildNavigationState('/live-trading', 12)
    const moreGroup = state.groups.find((group) => group.key === 'more')
    const changelogItem = moreGroup.items.find((item) => item.key === 'changelog')

    assert.equal(moreGroup.label, '更多')
    assert.equal(moreGroup.badge, '12')
    assert.equal(changelogItem.badge, '12')
  })

  it('marks 更多 active on the changelog route and caps large unread counts', () => {
    const state = buildNavigationState('/changelog', 120)
    const moreGroup = state.groups.find((group) => group.key === 'more')

    assert.equal(moreGroup.isActive, true)
    assert.equal(state.activeGroupKey, 'more')
    assert.equal(moreGroup.badge, '99+')
  })

  it('keeps stock research tools under 选股', () => {
    const state = buildNavigationState('/factor-lab', 0)
    const screeningGroup = state.groups.find((group) => group.key === 'screening')

    assert.deepEqual(
      screeningGroup.items.map((item) => item.label),
      ['选股器', '因子实验室', '回测引擎', '策略库']
    )
    assert.equal(screeningGroup.isActive, true)
  })

  it('keeps a hover bridge between the trigger and submenu while preserving the visual gap', () => {
    assert.ok(navDropdownSource.includes('className="absolute left-0 top-full z-20 pt-2"'))
    assert.ok(navDropdownSource.includes('className="min-w-[176px] rounded-xl border border-border bg-card p-2 shadow-2xl"'))
    assert.ok(!navDropdownSource.includes('mt-2 min-w-[176px]'))
  })
})
