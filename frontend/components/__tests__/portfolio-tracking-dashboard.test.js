import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const dashboardSource = readFileSync(new URL('../PortfolioTrackingDashboard.js', import.meta.url), 'utf8')
const adminSectionsSource = readFileSync(new URL('../admin/AdminSections.js', import.meta.url), 'utf8')

describe('portfolio tracking dashboard discoverability', () => {
  it('highlights that users need to select a portfolio card first', () => {
    assert.match(dashboardSource, /先选一个组合/)
    assert.match(dashboardSource, /当前查看/)
    assert.match(dashboardSource, /点击这张卡片，可切换下方净值曲线、指标、持仓和调仓记录/)
    assert.match(dashboardSource, /scrollIntoView/)
  })
})

describe('admin quadrant panel migration hints', () => {
  it('surfaces the new fact-table management entry and hides old buttons behind legacy notice', () => {
    assert.match(adminSectionsSource, /新口径模拟组合管理/)
    assert.match(adminSectionsSource, /同步最新事实表/)
    assert.match(adminSectionsSource, /管理后台不再展示旧按钮/)
  })
})
