import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const pageSource = readFileSync(new URL('../../pages/admin.js', import.meta.url), 'utf8')

describe('admin page theme integration', () => {
  it('uses semantic background tokens for page shell', () => {
    assert.match(pageSource, /min-h-screen bg-background text-foreground/)
    assert.match(pageSource, /bg-background\/90 backdrop-blur-md/)
    assert.doesNotMatch(pageSource, /bg-\[#0a0b0f\]/)
  })

  it('uses semantic card and input backgrounds instead of hardcoded dark colors', () => {
    assert.match(pageSource, /bg-card/)
    assert.match(pageSource, /bg-background-alt/)
    assert.match(pageSource, /focus:bg-\[var\(--color-bg-hover\)\]/) 
    assert.doesNotMatch(pageSource, /bg-\[#121317\]|bg-\[#15171e\]|bg-\[#171a21\]|bg-\[#111318\]|bg-\[#191d27\]|bg-\[#202633\]|bg-\[#1b1f28\]|bg-\[#0f1117\]/)
  })

  it('keeps amber warning surfaces readable in both light and dark mode', () => {
    assert.match(pageSource, /text-amber-800/)
    assert.match(pageSource, /dark:text-amber-100(?:\/\d+)?/)
    assert.match(pageSource, /bg-amber-50/)
  })
})
