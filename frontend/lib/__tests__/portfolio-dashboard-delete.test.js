import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

function extractFunctionSource(source, signature) {
  const start = source.indexOf(signature)
  if (start === -1) throw new Error(`missing signature: ${signature}`)
  let depth = 0
  let end = -1
  for (let i = start; i < source.length; i++) {
    const ch = source[i]
    if (ch === '{') depth += 1
    if (ch === '}') {
      depth -= 1
      if (depth === 0) {
        end = i + 1
        break
      }
    }
  }
  if (end === -1) throw new Error(`unterminated function: ${signature}`)
  return source.slice(start, end)
}

const source = await import('node:fs/promises').then((fs) => fs.readFile(new URL('../portfolio-dashboard.js', import.meta.url), 'utf8'))

const buildPortfolioDeleteConfirmText = new Function(`${extractFunctionSource(source, 'export function buildPortfolioDeleteConfirmText').replace('export function', 'function')}; return buildPortfolioDeleteConfirmText;`)()

const deletePortfolioHistory = new Function(
  'requestJson',
  `${extractFunctionSource(source, 'export async function deletePortfolioHistory').replace('export async function', 'async function')}; return deletePortfolioHistory;`
)((path, init, fallback) => Promise.resolve({ path, init, fallback }))

describe('portfolio dashboard delete helpers', () => {
  it('builds delete confirmation text from normalized symbol', () => {
    assert.equal(buildPortfolioDeleteConfirmText(' 00700.hk '), 'DELETE 00700.HK')
  })

  it('builds DELETE request for portfolio history removal', async () => {
    const result = await deletePortfolioHistory('00700.hk')
    assert.deepEqual(result, {
      path: '/api/portfolio/00700.HK',
      init: { method: 'DELETE' },
      fallback: '删除持仓历史失败',
    })
  })
})
