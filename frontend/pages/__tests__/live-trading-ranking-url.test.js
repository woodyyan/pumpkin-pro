// ── Tests for live-trading.js loadRanking URL construction ──
// 回归测试：保证 A 股/港股 ranking API 的 query string 格式正确
//
// Bug: 旧代码 exchange=ASHARE 时 qs 为空字符串，
//      最终 URL 变成 /api/quadrant/ranking&limit=20（缺少 ? 前缀）
// Fix: 改用 URLSearchParams 构建

import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

// ═══════════════════════════════════════════════════
// 从 live-trading.js 提取的纯函数（保持同步）
// ═══════════════════════════════════════════════════

function buildRankingUrl(exchange) {
  const params = new URLSearchParams()
  params.set('limit', '20')
  if (exchange && exchange !== 'ASHARE') {
    params.set('exchange', exchange)
  }
  // ASHARE 不传 exchange，走后端默认值 (SSE+SZSE)
  return `/api/quadrant/ranking?${params.toString()}`
}

// ═══════════════════════════════════════════════════
// 测试用例
// ═══════════════════════════════════════════════════

describe('buildRankingUrl — URL construction', () => {

  it('ASHARE: URL 必须包含 ? 前缀（回归：旧 bug 会生成 &limit=20）', () => {
    const url = buildRankingUrl('ASHARE')
    assert.ok(url.startsWith('/api/quadrant/ranking?'), `URL 应以 ? 开头，实际: ${url}`)
    assert.ok(!url.includes('&limit'), `URL 不应含孤立的 &limit，实际: ${url}`)
    assert.ok(url.includes('limit=20'))
  })

  it('ASHARE: 不携带 exchange 参数（走后端默认 SSE+SZSE）', () => {
    const url = buildRankingUrl('ASHARE')
    assert.ok(!url.includes('exchange='), `ASHARE 不应传 exchange，实际: ${url}`)
  })

  it('HKEX: URL 正确带 exchange 和 limit 参数', () => {
    const url = buildRankingUrl('HKEX')
    assert.ok(url.includes('exchange=HKEX'), `缺少 exchange=HKEX，实际: ${url}`)
    assert.ok(url.includes('limit=20'), `缺少 limit=20，实际: ${url}`)
    assert.ok(url.includes('?exchange') || url.includes('?limit'), `URL 缺少 ? 分隔符，实际: ${url}`)
  })

  it('空字符串: 等同 ASHARE 行为（后端兜底）', () => {
    const url = buildRankingUrl('')
    assert.equal(url, '/api/quadrant/ranking?limit=20')
  })

  it('undefined/null: 等同 ASHARE 行为', () => {
    assert.equal(buildRankingUrl(undefined), '/api/quadrant/ranking?limit=20')
    assert.equal(buildRankingUrl(null), '/api/quadrant/ranking?limit=20')
  })

  it('所有合法 URL 都不以 & 符号直接跟在路径后面', () => {
    ;['ASHARE', 'HKEX', '', null, undefined].forEach((ex) => {
      const url = buildRankingUrl(ex)
      assert.ok(
        !url.includes('/ranking&'),
        `exchange=${JSON.stringify(ex)} 的 URL 格式错误: ${url}`
      )
    })
  })
})
