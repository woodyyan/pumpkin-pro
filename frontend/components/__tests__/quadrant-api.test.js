import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

import { buildQuadrantRankingUrl, buildQuadrantUrl } from '../../lib/quadrant-api.js'

describe('quadrant api url builders', () => {
  it('builds default A-share quadrant url without query string', () => {
    assert.equal(buildQuadrantUrl(), '/api/quadrant')
    assert.equal(buildQuadrantUrl({ exchange: 'ASHARE' }), '/api/quadrant')
  })

  it('builds HK quadrant url and includes deduped watchlist symbols', () => {
    assert.equal(
      buildQuadrantUrl({ exchange: 'HKEX', watchlistSymbols: ['00700.HK', '00700.HK', ' 09988.HK '] }),
      '/api/quadrant?exchange=HKEX&watchlist_symbols=00700.HK%2C09988.HK'
    )
  })

  it('skips empty watchlist symbols when building quadrant url', () => {
    assert.equal(
      buildQuadrantUrl({ exchange: 'ASHARE', watchlistSymbols: ['', '  ', '600519.SH'] }),
      '/api/quadrant?watchlist_symbols=600519.SH'
    )
  })

  it('builds ranking url with default limit and optional HK exchange', () => {
    assert.equal(buildQuadrantRankingUrl('ASHARE'), '/api/quadrant/ranking?limit=20')
    assert.equal(buildQuadrantRankingUrl('HKEX'), '/api/quadrant/ranking?limit=20&exchange=HKEX')
  })

  it('falls back to limit=20 for invalid limits', () => {
    assert.equal(buildQuadrantRankingUrl('HKEX', 0), '/api/quadrant/ranking?limit=20&exchange=HKEX')
    assert.equal(buildQuadrantRankingUrl('ASHARE', -3), '/api/quadrant/ranking?limit=20')
  })
})
