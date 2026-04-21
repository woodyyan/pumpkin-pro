import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

import { deriveAIAnalysisWaitState } from '../ai-analysis-wait.js'

describe('deriveAIAnalysisWaitState', () => {
  it('starts with prepare stage and market step active', () => {
    const state = deriveAIAnalysisWaitState(0)
    assert.equal(state.stage.key, 'prepare')
    assert.equal(state.stage.kicker, '阶段 1/5')
    assert.equal(state.progress, 8)
    assert.equal(state.steps[0].status, 'active')
    assert.equal(state.steps[1].status, 'pending')
  })

  it('advances into market stage and marks prior steps correctly', () => {
    const state = deriveAIAnalysisWaitState(10)
    assert.equal(state.stage.key, 'market')
    assert.ok(state.progress >= 12 && state.progress <= 28)
    assert.equal(state.steps[0].status, 'done')
    assert.equal(state.steps[1].status, 'active')
    assert.equal(state.steps[2].status, 'pending')
  })

  it('switches context copy based on whether the user has a position', () => {
    const withPosition = deriveAIAnalysisWaitState(36, { hasPosition: true })
    const withoutPosition = deriveAIAnalysisWaitState(36, { hasPosition: false })

    assert.equal(withPosition.stage.key, 'context')
    assert.match(withPosition.stage.title, /持仓上下文/)
    assert.match(withPosition.steps[4].description, /持仓盈亏/)

    assert.equal(withoutPosition.stage.key, 'context')
    assert.match(withoutPosition.stage.title, /风险偏好/)
    assert.match(withoutPosition.steps[4].description, /空仓视角/)
  })

  it('caps slow progress before completion and keeps conclusion active', () => {
    const state = deriveAIAnalysisWaitState(120)
    assert.equal(state.stage.key, 'slow')
    assert.equal(state.progress, 92)
    assert.equal(state.steps[4].status, 'done')
    assert.equal(state.steps[5].status, 'active')
  })

  it('clamps invalid elapsed input to a safe starting state', () => {
    const state = deriveAIAnalysisWaitState(-5)
    assert.equal(state.elapsedSec, 0)
    assert.equal(state.stage.key, 'prepare')
    assert.equal(state.progress, 8)
  })
})
