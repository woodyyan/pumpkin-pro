import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { createRequire } from 'node:module'
import { readFileSync } from 'node:fs'

const requireFromCwd = createRequire(process.cwd() + '/')
const { parse } = requireFromCwd('next/dist/compiled/babel/parser')
const pageSource = readFileSync(new URL('../../pages/index.js', import.meta.url), 'utf8')
const homepageDataSource = readFileSync(new URL('../../data/homepage.js', import.meta.url), 'utf8')
const combinedSource = `${pageSource}\n${homepageDataSource}`

describe('home page information architecture', () => {
  it('parses page and homepage data as valid JSX/JS', () => {
    assert.doesNotThrow(() => {
      parse(pageSource, {
        sourceType: 'module',
        plugins: ['jsx'],
      })
    })
    assert.doesNotThrow(() => {
      parse(homepageDataSource, {
        sourceType: 'module',
      })
    })
  })

  it('positions the hero around the new AI research workflow', () => {
    assert.match(homepageDataSource, /AI投研 · 因子选股 · 组合跟踪/)
    assert.match(homepageDataSource, /覆盖 A 股与中国香港股票/)
    assert.match(pageSource, /AI投研、因子选股与组合跟踪工作台/)
  })

  it('uses the refreshed three core selling points', () => {
    assert.match(homepageDataSource, /AI 投研闭环/)
    assert.match(homepageDataSource, /因子驱动选股/)
    assert.match(homepageDataSource, /组合跟踪与复盘/)
    assert.doesNotMatch(homepageDataSource, /卧龙AI投研模型/)
    assert.doesNotMatch(homepageDataSource, /100\+/)
  })

  it('keeps quick-start paths aligned with confirmed priority workflows', () => {
    assert.match(homepageDataSource, /title: '我想快速判断一只股票',[\s\S]*?href: '\/ai\/analysis'/)
    assert.match(homepageDataSource, /title: '我想看一份更完整的个股研报',[\s\S]*?href: '\/ai\/reports'/)
    assert.match(homepageDataSource, /title: '我想每天看 AI 推荐的股票组合',[\s\S]*?href: '\/ai\/picker'/)
    assert.match(homepageDataSource, /title: '我想自己调因子找股票',[\s\S]*?href: '\/factor-lab'/)
    assert.match(homepageDataSource, /title: '我想看模拟组合表现',[\s\S]*?href: '\/portfolio-tracking'/)
    assert.match(homepageDataSource, /title: '我想管理自己的真实持仓',[\s\S]*?href: '\/portfolio'/)
  })

  it('lists all homepage feature categories and key user-facing routes', () => {
    for (const label of ['AI 投研', '市场与机会发现', '选股与策略研究', '跟踪与组合管理', '账户、服务与更新']) {
      assert.match(homepageDataSource, new RegExp(label))
    }

    for (const href of ['/ai/analysis', '/ai/reports', '/ai/picker', '/ai/backtest', '/live-trading', '/quadrant', '/watchlist', '/portfolio-tracking', '/portfolio', '/stock-picker', '/factor-lab', '/backtest', '/strategies', '/settings', '/changelog', '/disclaimer']) {
      assert.match(homepageDataSource, new RegExp(`href: '${href.replaceAll('/', '\\/')}'`))
    }
  })

  it('adds tutorials for AI research, factors, portfolios, and signal setup', () => {
    for (const text of ['如何使用 AI分析判断一只股票？', '如何预览和定制 AI研报？', '如何查看每日 AI选股？', '如何用因子实验室自定义选股？', '如何查看模拟组合表现？', '如何记录和复盘真实持仓？', '如何配置交易信号和 Webhook？']) {
      assert.match(combinedSource, new RegExp(text.replace(/[?？]/g, '[?？]')))
    }
  })

  it('keeps investment-risk copy visible on the refreshed home page', () => {
    assert.match(pageSource, /AI分析、AI研报、AI选股、因子排序、策略回测、模拟组合和交易信号仅用于辅助研究/)
    assert.match(pageSource, /不构成任何投资建议或收益承诺/)
  })
})
