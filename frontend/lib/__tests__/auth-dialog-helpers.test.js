import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

import {
  createAuthDialogError,
  getRegisterPasswordGuide,
  mapAuthDialogError,
  validateAuthDialogInput,
} from '../auth-dialog-helpers.js'

describe('validateAuthDialogInput', () => {
  it('requires email and password in login mode', () => {
    assert.deepEqual(
      validateAuthDialogInput({ mode: 'login', email: '', password: '', confirmPassword: '' }),
      createAuthDialogError('请输入邮箱和密码'),
    )
  })

  it('returns register-specific email guidance', () => {
    assert.deepEqual(
      validateAuthDialogInput({ mode: 'register', email: '', password: 'password123', confirmPassword: 'password123' }),
      createAuthDialogError('请输入注册邮箱', 'EMAIL_REQUIRED'),
    )

    assert.deepEqual(
      validateAuthDialogInput({ mode: 'register', email: 'invalid-email', password: 'password123', confirmPassword: 'password123' }),
      createAuthDialogError('请输入正确的邮箱地址', 'INVALID_EMAIL'),
    )
  })

  it('returns register-specific password guidance', () => {
    assert.deepEqual(
      validateAuthDialogInput({ mode: 'register', email: 'user@test.com', password: '', confirmPassword: '' }),
      createAuthDialogError('请先设置登录密码', 'PASSWORD_REQUIRED'),
    )

    assert.deepEqual(
      validateAuthDialogInput({ mode: 'register', email: 'user@test.com', password: '1234567', confirmPassword: '1234567' }),
      createAuthDialogError('密码至少需要 8 位', 'PASSWORD_TOO_SHORT'),
    )
  })

  it('requires matching confirmation in register mode', () => {
    assert.deepEqual(
      validateAuthDialogInput({ mode: 'register', email: 'user@test.com', password: 'password123', confirmPassword: 'password321' }),
      createAuthDialogError('两次密码输入不一致', 'PASSWORD_MISMATCH'),
    )
  })

  it('returns empty error when register form is valid', () => {
    assert.deepEqual(
      validateAuthDialogInput({ mode: 'register', email: 'user@test.com', password: 'password123', confirmPassword: 'password123' }),
      createAuthDialogError(),
    )
  })
})

describe('mapAuthDialogError', () => {
  it('maps duplicate email to actionable register guidance', () => {
    const mapped = mapAuthDialogError({ code: 'EMAIL_EXISTS', message: '邮箱已被注册' }, 'register')
    assert.equal(mapped.code, 'EMAIL_EXISTS')
    assert.match(mapped.message, /已经注册过了/)
    assert.match(mapped.message, /找回密码/)
  })

  it('maps short password error to friendlier explanation', () => {
    const mapped = mapAuthDialogError({ code: 'PASSWORD_TOO_SHORT', message: '密码至少需要 8 位' }, 'register')
    assert.equal(mapped.code, 'PASSWORD_TOO_SHORT')
    assert.match(mapped.message, /至少需要 8 位/)
    assert.match(mapped.message, /字母和数字/)
  })

  it('falls back to backend message for unhandled login errors', () => {
    const mapped = mapAuthDialogError({ code: 'INVALID_CREDENTIAL', message: '邮箱或密码错误' }, 'login')
    assert.deepEqual(mapped, createAuthDialogError('邮箱或密码错误', 'INVALID_CREDENTIAL'))
  })
})

describe('getRegisterPasswordGuide', () => {
  it('tracks missing length and recommended mix separately', () => {
    assert.deepEqual(getRegisterPasswordGuide('1234567'), {
      minLengthMet: false,
      remainingLength: 1,
      recommendedMixMet: false,
    })
  })

  it('marks recommendation complete when letters and digits are both present', () => {
    assert.deepEqual(getRegisterPasswordGuide('abc12345'), {
      minLengthMet: true,
      remainingLength: 0,
      recommendedMixMet: true,
    })
  })
})
