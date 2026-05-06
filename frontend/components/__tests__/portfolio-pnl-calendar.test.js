import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const source = readFileSync(new URL('../PortfolioPnlCalendar.js', import.meta.url), 'utf8')

function extractFunctionSource(signature) {
  const start = source.indexOf(signature)
  if (start === -1) throw new Error(`missing signature: ${signature}`)
  let depth = 0
  let end = -1
  for (let i = start; i < source.length; i += 1) {
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
  return source.slice(start, end).replace('export function', 'function')
}

const helpers = new Function(`
  ${extractFunctionSource('export function buildCalendarGrid')}
  ${extractFunctionSource('export function getMonthInputValue')}
  ${extractFunctionSource('export function parseMonthInputValue')}
  ${extractFunctionSource('export function shiftCalendarMonth')}
  ${extractFunctionSource('export function getSelectedDay')}
  return { buildCalendarGrid, getMonthInputValue, parseMonthInputValue, shiftCalendarMonth, getSelectedDay };
`)()

describe('PortfolioPnlCalendar helpers', () => {
  it('builds a fixed 42-cell Monday-first calendar grid', () => {
    const grid = helpers.buildCalendarGrid(2026, 5, [{ day: 5, date: '2026-05-05', has_data: true }])
    assert.equal(grid.length, 42)
    assert.equal(grid[0].inMonth, false)
    assert.equal(grid[4].date, '2026-05-01')
    assert.equal(grid[8].data.date, '2026-05-05')
  })

  it('formats and parses month input values', () => {
    assert.equal(helpers.getMonthInputValue(2026, 5), '2026-05')
    assert.deepEqual(helpers.parseMonthInputValue('2026-12'), { year: 2026, month: 12 })
    assert.equal(helpers.parseMonthInputValue('1999-12'), null)
  })

  it('shifts months across year boundaries', () => {
    assert.deepEqual(helpers.shiftCalendarMonth(2026, 1, -1), { year: 2025, month: 12 })
    assert.deepEqual(helpers.shiftCalendarMonth(2026, 12, 1), { year: 2027, month: 1 })
  })

  it('finds selected day from payload', () => {
    const data = { days: [{ date: '2026-05-05', pnl_amount: 10 }] }
    assert.deepEqual(helpers.getSelectedDay(data, '2026-05-05'), { date: '2026-05-05', pnl_amount: 10 })
    assert.equal(helpers.getSelectedDay(data, '2026-05-06'), null)
  })

  it('keeps mobile calendar numbers readable without truncation', () => {
    assert.doesNotMatch(source, /truncate/)
    assert.match(source, /formatCalendarCellPnlAmount/)
    assert.match(source, /whitespace-normal break-words text-center/)
    assert.match(source, /min-h-\[68px\]/)
  })
})
