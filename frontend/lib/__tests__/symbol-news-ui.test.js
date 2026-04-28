import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

import {
  buildAINewsContext,
  buildNewsEmptyState,
  buildNewsHeadlineText,
  buildNewsSummaryBadges,
  filterSymbolNewsItems,
  formatNewsTypeLabel,
} from '../symbol-news-ui.js'

describe('symbol news ui helpers', () => {
  it('builds compact summary badges', () => {
    const badges = buildNewsSummaryBadges({ last_24h_count: 8, announcement_count: 2, filing_count: 1 })
    assert.deepEqual(badges, ['近24h 8条', '公告 2条', '财报 1份'])
  })

  it('falls back when no news is available', () => {
    assert.deepEqual(buildNewsSummaryBadges(null), [])
    assert.equal(buildNewsHeadlineText({}), '最近暂无高相关的新闻、公告或财报更新。')
    assert.equal(buildNewsEmptyState('filing'), '当前没有可展示的财报。')
  })

  it('filters by item type and formats labels', () => {
    const items = [{ type: 'news' }, { type: 'announcement' }, { type: 'filing' }]
    assert.equal(filterSymbolNewsItems(items, 'announcement').length, 1)
    assert.equal(formatNewsTypeLabel('filing'), '财报')
  })

  it('builds ai news context with capped items', () => {
    const context = buildAINewsContext({
      summary: { last_24h_count: 3, announcement_count: 1, filing_count: 1, latest_headline: '财报发布', highlight_tags: ['财报', '业绩'] },
      items: [
        { type: 'filing', source_name: 'HKEX', published_at: '2026-04-27T09:28:00Z', title: '2026Q1 财报', summary: '净利润增长', source_type: 'official' },
        { type: 'news', source_name: '财联社', published_at: '2026-04-27T08:41:00Z', title: '新品放量', summary: '订单增长', source_type: 'media' },
      ],
      maxItems: 1,
    })

    assert.equal(context._valid, true)
    assert.equal(context.summary.filing_count, 1)
    assert.equal(context.items.length, 1)
    assert.equal(context.items[0].official, true)
  })

  it('returns invalid context when neither summary nor items are useful', () => {
    assert.deepEqual(buildAINewsContext({ summary: {}, items: [] }), { _valid: false })
  })
})
