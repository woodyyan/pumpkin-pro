import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const pageSource = readFileSync(new URL('../../pages/admin.js', import.meta.url), 'utf8')
const shellSource = readFileSync(new URL('../../components/admin/AdminShell.js', import.meta.url), 'utf8')
const sectionsSource = readFileSync(new URL('../../components/admin/AdminSections.js', import.meta.url), 'utf8')

describe('admin page theme integration', () => {
  it('uses semantic background tokens for page shell', () => {
    assert.match(shellSource, /min-h-screen bg-background text-foreground/)
    assert.match(shellSource, /bg-background\/90 backdrop-blur-md/)
    assert.doesNotMatch(shellSource, /bg-\[#0a0b0f\]/)
  })

  it('uses semantic card and input backgrounds instead of hardcoded dark colors', () => {
    assert.match(shellSource, /bg-card/)
    assert.match(sectionsSource, /bg-background-alt/)
    assert.match(sectionsSource, /focus:bg-\[var\(--color-bg-hover\)\]/)
    assert.doesNotMatch(shellSource + sectionsSource, /bg-\[#121317\]|bg-\[#15171e\]|bg-\[#171a21\]|bg-\[#111318\]|bg-\[#191d27\]|bg-\[#202633\]|bg-\[#1b1f28\]|bg-\[#0f1117\]/)
  })

  it('keeps amber warning surfaces readable in both light and dark mode', () => {
    assert.match(sectionsSource, /text-amber-800/)
    assert.match(sectionsSource, /dark:text-amber-100(?:\/\d+)?/)
    assert.match(sectionsSource, /bg-amber-50/)
  })

  it('routes admin entry through the shared shell', () => {
    assert.match(pageSource, /AdminShell/)
    assert.match(pageSource, /AdminOverviewPage/)
  })
})
