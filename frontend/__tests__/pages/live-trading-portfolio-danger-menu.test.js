import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const pageSource = readFileSync(new URL('../../pages/live-trading/[symbol].js', import.meta.url), 'utf8')

function extractFunctionSource(signature) {
  const start = pageSource.indexOf(signature)
  if (start === -1) throw new Error(`missing signature: ${signature}`)
  let depth = 0
  let end = -1
  for (let i = start; i < pageSource.length; i += 1) {
    const ch = pageSource[i]
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
  return pageSource.slice(start, end)
}

const createPortfolioDangerMenuState = new Function(
  `${extractFunctionSource('export function createPortfolioDangerMenuState').replace('export function', 'function')}; return createPortfolioDangerMenuState;`
)()

const reducePortfolioDangerMenu = new Function(
  'createPortfolioDangerMenuState',
  `${extractFunctionSource('export function reducePortfolioDangerMenu').replace('export function', 'function')}; return reducePortfolioDangerMenu;`
)(createPortfolioDangerMenuState)

describe('live trading portfolio danger menu', () => {
  it('starts closed by default', () => {
    assert.deepEqual(createPortfolioDangerMenuState(), { open: false })
  })

  it('toggles open and closed through reducer actions', () => {
    const opened = reducePortfolioDangerMenu(createPortfolioDangerMenuState(), { type: 'toggle' })
    assert.deepEqual(opened, { open: true })

    const closed = reducePortfolioDangerMenu(opened, { type: 'close' })
    assert.deepEqual(closed, { open: false })
  })

  it('keeps delete action inside the folded more menu instead of the main toolbar', () => {
    assert.match(pageSource, /更多/)
    assert.match(pageSource, /危险操作已折叠到这里，避免干扰日常买卖与调仓。/)
    assert.doesNotMatch(pageSource, /<div className="text-sm font-semibold text-rose-100">危险操作<\/div>/)
  })
})
