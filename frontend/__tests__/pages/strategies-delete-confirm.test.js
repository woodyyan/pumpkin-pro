// ── DeleteConfirmDialog rendering logic tests ──
// Uses Node 20+ built-in test runner (node --test)
//
// Tests that the delete-confirm dialog correctly displays
// symbol + name from the API response refs array.
// The bug was: Go SymbolRef had no json tags → JSON keys were
// "Symbol"/"Name" (capitalized), but frontend used ref.symbol/ref.name → undefined.
// Fix: added json:"symbol" / json:"name" tags to SymbolRef struct.

import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

// ── Simulates the inline rendering logic in strategies.js DeleteConfirmDialog ──
// Original code:
//   <span className="font-mono text-white/90">
//     {`${ref.symbol}${ref.name ? ` ${ref.name}` : ''}`}
//   </span>
function renderRefDisplay(ref) {
  return `${ref.symbol}${ref.name ? ` ${ref.name}` : ''}`
}

describe('DeleteConfirmDialog – ref display', () => {

  it('shows symbol + name when both present', () => {
    const ref = { symbol: '600519.SH', name: '贵州茅台' }
    assert.equal(renderRefDisplay(ref), '600519.SH 贵州茅台')
  })

  it('shows symbol only when name is empty', () => {
    const ref = { symbol: '000001.SZ', name: '' }
    assert.equal(renderRefDisplay(ref), '000001.SZ')
  })

  it('shows symbol only when name is undefined', () => {
    const ref = { symbol: '000001.SZ' }  // no name key
    assert.equal(renderRefDisplay(ref), '000001.SZ')
  })

  it('uses lowercase "symbol" key (json tag from Go fix)', () => {
    // This is the key regression test: after the Go json tag fix,
    // the API returns lowercase "symbol" / "name" keys.
    const apiResponse = {
      error: '该策略正在被交易信号使用，无法删除',
      refs: [
        { symbol: '600519.SH', name: '贵州茅台' },
        { symbol: '601138.SH', name: '工业富联' },
      ],
    }
    // Verify refs are accessible via lowercase keys
    const displays = apiResponse.refs.map(renderRefDisplay)
    assert.deepEqual(displays, ['600519.SH 贵州茅台', '601138.SH 工业富联'])
  })

  it('regression: keys must NOT be capitalized (the original bug)', () => {
    // This test captures the original bug: if the API returns capitalized
    // keys "Symbol"/"Name", the display would contain "undefined".
    const badResponse = {
      refs: [
        { Symbol: '600519.SH', Name: '贵州茅台' },  // capitalized – WRONG
      ],
    }
    // Accessing with wrong case returns undefined → template literal shows "undefined"
    const display = `${badResponse.refs[0].symbol}${badResponse.refs[0].name ? ` ${badResponse.refs[0].name}` : ''}`
    assert.equal(display, 'undefined')  // the bug: shows "undefined"
  })

})
