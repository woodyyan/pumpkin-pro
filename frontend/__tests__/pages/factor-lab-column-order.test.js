import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const pageSource = readFileSync(new URL('../../pages/factor-lab.js', import.meta.url), 'utf8')

describe('factor lab result table columns', () => {
  it('places core stock identity columns before score columns', () => {
    assert.match(pageSource, /const ALL_COLUMNS = \[\.\.\.BASE_COLUMNS, \.\.\.SCORE_COLUMNS\]/)
  })
})
