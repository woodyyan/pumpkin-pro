import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { createRequire } from 'node:module'
import { readFileSync } from 'node:fs'

const requireFromCwd = createRequire(process.cwd() + '/')
const { parse } = requireFromCwd('next/dist/compiled/babel/parser')
const pageSource = readFileSync(new URL('../../pages/live-trading/[symbol].js', import.meta.url), 'utf8')

describe('live trading stock detail latest UI adjustments', () => {
  it('parses the stock detail page as valid JSX', () => {
    assert.doesNotThrow(() => {
      parse(pageSource, {
        sourceType: 'module',
        plugins: ['jsx'],
      })
    })
  })

  it('removes the recent viewpoint copy from the overview AI entry', () => {
    assert.doesNotMatch(pageSource, /最近观点：已生成，可在下方 AI 分析历史中查看详情。/)
    assert.doesNotMatch(pageSource, /最近观点：/)
  })

  it('places AI analysis history below the realtime snapshot in overview', () => {
    const snapshotIndex = pageSource.indexOf('实时快照')
    const historyIndex = pageSource.indexOf('AI 分析历史（登录后展示，默认折叠）')

    assert.notEqual(snapshotIndex, -1)
    assert.notEqual(historyIndex, -1)
    assert.ok(historyIndex > snapshotIndex)
  })
})
