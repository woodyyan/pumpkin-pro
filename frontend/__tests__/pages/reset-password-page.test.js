import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { createRequire } from 'node:module'
import { readFileSync } from 'node:fs'

const requireFromCwd = createRequire(process.cwd() + '/')
const { parse } = requireFromCwd('next/dist/compiled/babel/parser')
const pageSource = readFileSync(new URL('../../pages/reset-password.js', import.meta.url), 'utf8')

describe('reset-password page', () => {
  it('parses as valid JSX', () => {
    assert.doesNotThrow(() => parse(pageSource, { sourceType: 'module', plugins: ['jsx'] }))
  })

  it('checks token first and clears auth session after reset', () => {
    assert.match(pageSource, /\/api\/auth\/reset-password\/inspect\?token=/)
    assert.match(pageSource, /clearAuthSession\(\)/)
    assert.match(pageSource, /broadcastAuthSessionCleared\('password_reset'\)/)
  })
})
