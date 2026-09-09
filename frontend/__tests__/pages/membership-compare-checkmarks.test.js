import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { createRequire } from 'node:module'
import { readFileSync } from 'node:fs'

const requireFromCwd = createRequire(process.cwd() + '/')
const { parse } = requireFromCwd('next/dist/compiled/babel/parser')

const pageSource = readFileSync(new URL('../../pages/membership.js', import.meta.url), 'utf8')

const parseJsx = (source) => parse(source, { sourceType: 'module', plugins: ['jsx'] })

describe('membership compare table member checkmarks', () => {
  it('keeps the page parseable as JSX', () => {
    assert.doesNotThrow(() => parseJsx(pageSource))
  })

  it('renders member benefits through the checkmark wrapper', () => {
    assert.match(pageSource, /function MemberBenefit\(\{ children \}\)/)
    assert.match(pageSource, /<MemberBenefit>\{row\.member\}<\/MemberBenefit>/)
  })

  it('marks the checkmark with the brand color token, not a hardcoded color', () => {
    const component = pageSource.slice(
      pageSource.indexOf('function MemberBenefit('),
      pageSource.indexOf('export default function MembershipPage')
    )

    assert.match(component, /text-primary/)
    // 遵循 .ai/context/coding-standards.md：禁止硬编码颜色
    assert.doesNotMatch(component, /text-\[#|text-orange-|text-white|text-slate-/)
  })

  it('hides the decorative checkmark from screen readers', () => {
    const component = pageSource.slice(
      pageSource.indexOf('function MemberBenefit('),
      pageSource.indexOf('export default function MembershipPage')
    )

    assert.match(component, /aria-hidden="true"/)
  })

  it('does not add checkmarks to the free plan column', () => {
    assert.match(pageSource, /text-foreground-muted">\{row\.free\}<\/td>/)
  })

  it('keeps the checkmark from shrinking or wrapping with long benefit text', () => {
    const component = pageSource.slice(
      pageSource.indexOf('function MemberBenefit('),
      pageSource.indexOf('export default function MembershipPage')
    )

    // 权益文案较长会换行，图标必须定宽且顶部对齐
    assert.match(component, /shrink-0/)
    assert.match(component, /items-start/)
  })

  it('uses a square viewBox and a closed filled checkmark path', () => {
    const viewBox = pageSource.match(/viewBox="0 0 (\d+) (\d+)"/)
    assert.ok(viewBox, 'checkmark svg must declare a viewBox')
    assert.equal(viewBox[1], viewBox[2], 'viewBox must be square so the icon scales evenly')

    const path = pageSource.match(/d="(M[^"]+)"/)
  assert.ok(path, 'checkmark svg must declare a path')

    // 该图标是 fill 填充的闭合折线（非 stroke 描边），
    // 因此必须成对提供 fillRule/clipRule，否则 evenodd 填充会出现镂空。
  assert.match(path[1], /^M/, 'path must start with an absolute moveto')
    assert.match(pageSource, /fillRule="evenodd"/)
    assert.match(pageSource, /clipRule="evenodd"/)
    assert.match(pageSource, /fill="currentColor"/)
  })

  it('draws the checkmark as a short tail plus a longer up-right stroke', () => {
    const path = pageSource.match(/d="(M[^"]+)"/)[1]

    // SVG 的 `l` 支持隐式重复坐标对（`l dx dy dx dy` 等价于两段 lineto），
    // 因此必须展开所有坐标对，不能假设每段都带显式 `l` 前缀。
    const segments = []
    for (const block of path.matchAll(/l([^a-zA-Z]+)/g)) {
    const nums = block[1].match(/-?\d*\.?\d+/g).map(Number)
      for (let i = 0; i + 1 < nums.length; i += 2) {
        segments.push({ dx: nums[i], dy: nums[i + 1] })
 }
    }

    assert.ok(segments.length >= 3, 'checkmark outline needs at least three line segments')

    // 对勾的辨识特征：存在一段「右上」长笔画，且它比「右下」短尾更长。
    const shortTail = segments.find((s) => s.dx > 0 && s.dy > 0)
    const longStroke = segments.find((s) => s.dx > 0 && s.dy < 0)

    assert.ok(shortTail, 'expected a down-right segment into the vertex')
    assert.ok(longStroke, 'expected an up-right segment out to the tip')
    assert.ok(
      Math.abs(longStroke.dy) > Math.abs(shortTail.dy),
      'the up-right stroke must be longer than the tail, otherwise it is a V not a check'
    )
  })
})
