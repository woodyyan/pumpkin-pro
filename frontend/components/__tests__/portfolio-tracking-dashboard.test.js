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
  it('surfaces the calendar-driven v2 pipeline entry and removes legacy repair buttons', () => {
    assert.match(adminSectionsSource, /模拟组合 Pipeline/)
    assert.match(adminSectionsSource, /交易日历驱动的严格模式链路/)
    assert.match(adminSectionsSource, /市场日历驾驶舱/)
    assert.match(adminSectionsSource, /设为该市场开始信号日/)
    assert.match(adminSectionsSource, /确认应用并重建该市场/)
    assert.match(adminSectionsSource, /初始化 v2 定义/)
    assert.match(adminSectionsSource, /运行 A 股 Pipeline/)
    assert.match(adminSectionsSource, /运行港股 Pipeline/)
    assert.match(adminSectionsSource, /休市日会标记为 skipped/)
    assert.match(adminSectionsSource, /最近运行日志/)
    assert.doesNotMatch(adminSectionsSource, /补齐建仓开盘价/)
    assert.doesNotMatch(adminSectionsSource, /补齐收盘价/)
    assert.doesNotMatch(adminSectionsSource, /同步最新事实表/)
    assert.doesNotMatch(adminSectionsSource, /全局开始跟踪日期/)
  })
})
