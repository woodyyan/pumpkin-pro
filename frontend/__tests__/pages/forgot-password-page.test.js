import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { createRequire } from 'node:module'
import { readFileSync } from 'node:fs'

const requireFromCwd = createRequire(process.cwd() + '/')
const { parse } = requireFromCwd('next/dist/compiled/babel/parser')
const pageSource = readFileSync(new URL('../../pages/forgot-password.js', import.meta.url), 'utf8')

describe('forgot-password page', () => {
  it('parses as valid JSX', () => {
    assert.doesNotThrow(() => parse(pageSource, { sourceType: 'module', plugins: ['jsx'] }))
  })

  it('calls forgot-password endpoint and shows rate-limit state', () => {
    assert.match(pageSource, /requestJson\('\/api\/auth\/forgot-password'/)
    assert.match(pageSource, /RATE_LIMITED/)
    assert.match(pageSource, /retry_after_seconds/)
  })
})
