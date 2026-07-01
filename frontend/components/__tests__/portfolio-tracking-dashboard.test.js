import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const dashboardSource = readFileSync(new URL('../PortfolioTrackingDashboard.js', import.meta.url), 'utf8')
const adminSectionsSource = readFileSync(new URL('../admin/AdminSections.js', import.meta.url), 'utf8')

describe('portfolio tracking dashboard discoverability', () => {
  it('keeps clickable card affordance and uses 港股 naming', () => {
    assert.doesNotMatch(dashboardSource, /先选一个组合/)
    assert.match(dashboardSource, /当前查看/)
    assert.match(dashboardSource, /点击这张卡片，可切换下方净值曲线、指标、持仓和调仓记录/)
    assert.match(dashboardSource, /'港股'/)
    assert.doesNotMatch(dashboardSource, /中国香港组合/)
    assert.match(dashboardSource, /scrollIntoView/)
  })
})

describe('admin quadrant panel migration hints', () => {
  it('surfaces the new fact-table management entry and hides old buttons behind legacy notice', () => {
    assert.match(adminSectionsSource, /新口径模拟组合管理/)
    assert.match(adminSectionsSource, /同步最新事实表/)
    assert.match(adminSectionsSource, /运维顺序/)
    assert.match(adminSectionsSource, /只补前置价格，不生成持仓/)
    assert.match(adminSectionsSource, /事实表/)
    assert.match(adminSectionsSource, /可同步/)
    assert.match(adminSectionsSource, /建议动作/)
    assert.match(adminSectionsSource, /最近一次事实表同步结果/)
    assert.match(adminSectionsSource, /全局开始跟踪日期/)
    assert.match(adminSectionsSource, /榜单信号日 \/ 收盘日/)
    assert.match(adminSectionsSource, /严格共同日期/)
    assert.match(adminSectionsSource, /按该日期重置并重算/)
  })
})
