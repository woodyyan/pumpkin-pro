import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

import { buildForwardHeaders, FORWARDED_REQUEST_HEADERS } from '../../pages/api/[...path].js'

describe('api proxy forwarded headers', () => {
  it('forwards browser user-agent to backend instead of Node default UA', () => {
    const req = {
      headers: {
        'content-type': 'application/json',
        accept: 'application/json',
        authorization: 'Bearer token_123',
        'user-agent': 'Mozilla/5.0 (Macintosh; Intel Mac OS X 14_4) AppleWebKit/537.36 Chrome/124.0.0.0 Safari/537.36',
        'x-forwarded-for': '1.2.3.4',
      },
    }

    const headers = buildForwardHeaders(req)
    assert.equal(headers['user-agent'], req.headers['user-agent'])
    assert.equal(headers.authorization, 'Bearer token_123')
    assert.equal(headers['x-forwarded-for'], '1.2.3.4')
  })

  it('keeps the allowlist explicit and includes user-agent', () => {
    assert.ok(FORWARDED_REQUEST_HEADERS.includes('user-agent'))
    assert.ok(FORWARDED_REQUEST_HEADERS.includes('authorization'))
  })
})
