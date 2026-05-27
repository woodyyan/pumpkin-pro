import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const pageSource = readFileSync(new URL('../../pages/disclaimer.js', import.meta.url), 'utf8')

describe('disclaimer theme integration', () => {
  it('keeps both warning callouts on the same amber palette in light mode', () => {
    const amberCallouts = pageSource.match(/rounded-xl border border-amber-600\/30 dark:border-amber-400\/20 bg-amber-50 dark:bg-amber-500\/8 px-4 py-3 text-amber-800 dark:text-amber-200\/90/g) || []
    assert.equal(amberCallouts.length, 2)
  })
})
