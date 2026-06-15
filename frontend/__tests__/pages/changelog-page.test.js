import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { createRequire } from 'node:module'
import { readFileSync } from 'node:fs'

const requireFromCwd = createRequire(process.cwd() + '/')
const { parse } = requireFromCwd('next/dist/compiled/babel/parser')
const pageSource = readFileSync(new URL('../../pages/changelog.js', import.meta.url), 'utf8')

describe('changelog page', () => {
  it('parses as valid JSX', () => {
    assert.doesNotThrow(() => parse(pageSource, { sourceType: 'module', plugins: ['jsx'] }))
  })

  it('continues to render changelog entries from the shared json data source', () => {
    assert.match(pageSource, /import changelogData from '..\/data\/changelog.json'/)
    assert.match(pageSource, /filter\(\(item\) => item\?\.visible !== false\)/)
    assert.match(pageSource, /sort\(\(left, right\) => String\(right.date \|\| ''\)\.localeCompare\(String\(left.date \|\| ''\)\)\)/)
  })
})
