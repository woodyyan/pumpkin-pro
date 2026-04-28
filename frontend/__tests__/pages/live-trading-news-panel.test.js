import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const pageSource = readFileSync(new URL('../../pages/live-trading/[symbol].js', import.meta.url), 'utf8')
const panelSource = readFileSync(new URL('../../components/SymbolNewsPanel.js', import.meta.url), 'utf8')

describe('live trading news panel integration', () => {
  it('mounts the news panel with refresh and filter controls', () => {
    assert.match(pageSource, /<SymbolNewsPanel/)
    assert.match(pageSource, /onRefresh=\{refreshNewsPanel\}/)
    assert.match(pageSource, /setNewsFilter\(nextType\)/)
    assert.match(panelSource, /FILTERS = \[/)
    assert.match(panelSource, /全部/)
    assert.match(panelSource, /公告/)
    assert.match(panelSource, /财报/)
  })

  it('uses overlay presentation instead of lengthening the main page', () => {
    assert.match(panelSource, /fixed inset-0 z-\[70\]/)
    assert.match(panelSource, /md:w-\[460px\]/)
    assert.match(panelSource, /rounded-t-\[28px\]/)
  })
})
