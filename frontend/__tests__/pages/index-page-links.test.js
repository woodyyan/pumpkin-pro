import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { createRequire } from 'node:module'
import { readFileSync } from 'node:fs'

const requireFromCwd = createRequire(process.cwd() + '/')
const { parse } = requireFromCwd('next/dist/compiled/babel/parser')
const pageSource = readFileSync(new URL('../../pages/index.js', import.meta.url), 'utf8')

describe('home page quadrant entry points', () => {
  it('parses as valid JSX', () => {
    assert.doesNotThrow(() => {
      parse(pageSource, {
        sourceType: 'module',
        plugins: ['jsx'],
      })
    })
  })

  it('routes risk panorama feature and scenario to /quadrant', () => {
    assert.match(pageSource, /title: '风险全景', href: '\/quadrant'/)
    assert.match(pageSource, /title: '我想抄作业',[\s\S]*?href: '\/quadrant'/)
    assert.match(pageSource, /进入「四象限」页面，先看风险机会全景图/)
  })

  it('keeps home page SEO aligned with quadrant and ranking content', () => {
    assert.match(pageSource, /AI 个股诊断、四象限风险全景、卧龙AI精选、策略回测与信号推送/)
  })
})
