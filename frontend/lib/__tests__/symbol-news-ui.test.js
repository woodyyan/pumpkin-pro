import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

import { buildAINewsContext } from '../symbol-news-ui.js'

describe('symbol news ui helpers', () => {
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
