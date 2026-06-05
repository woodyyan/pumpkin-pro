const AUTH_PASSWORD_MIN_LENGTH = 8

export function createAuthDialogError(message = '', code = '') {
  return { message, code }
}

function isEmailLike(value) {
  return value.includes('@')
}

export function validateAuthDialogInput({ mode, email, password, confirmPassword }) {
  const trimmedEmail = email.trim()
  const trimmedPassword = password.trim()

  if (mode !== 'register') {
    if (!trimmedEmail || !password) {
      return createAuthDialogError('请输入邮箱和密码')
    }
    return createAuthDialogError()
  }

  if (!trimmedEmail) {
    return createAuthDialogError('请输入注册邮箱', 'EMAIL_REQUIRED')
  }
  if (!isEmailLike(trimmedEmail)) {
    return createAuthDialogError('请输入正确的邮箱地址', 'INVALID_EMAIL')
  }
  if (!trimmedPassword) {
    return createAuthDialogError('请先设置登录密码', 'PASSWORD_REQUIRED')
  }
  if (trimmedPassword.length < AUTH_PASSWORD_MIN_LENGTH) {
    return createAuthDialogError(`密码至少需要 ${AUTH_PASSWORD_MIN_LENGTH} 位`, 'PASSWORD_TOO_SHORT')
  }
  if (password !== confirmPassword) {
    return createAuthDialogError('两次密码输入不一致', 'PASSWORD_MISMATCH')
  }

  return createAuthDialogError()
}

export function mapAuthDialogError(error, mode) {
  const fallbackMessage = mode === 'register' ? '注册失败' : '登录失败'
  const code = String(error?.code || '').toUpperCase()

  if (mode === 'register') {
    switch (code) {
      case 'EMAIL_EXISTS':
        return createAuthDialogError('这个邮箱已经注册过了。你可以直接登录，或者使用“找回密码”重新设置密码。', code)
      case 'EMAIL_REQUIRED':
        return createAuthDialogError('请输入注册邮箱', code)
      case 'INVALID_EMAIL':
        return createAuthDialogError('请输入正确的邮箱地址', code)
      case 'PASSWORD_REQUIRED':
        return createAuthDialogError('请先设置登录密码', code)
      case 'PASSWORD_TOO_SHORT':
        return createAuthDialogError(`密码至少需要 ${AUTH_PASSWORD_MIN_LENGTH} 位。建议同时包含字母和数字，更安全也更容易识别。`, code)
      default:
        break
    }
  }

  return createAuthDialogError(error?.message || fallbackMessage, code)
}

export function getRegisterPasswordGuide(password) {
  const trimmedPassword = password.trim()
  const hasLetters = /[A-Za-z]/.test(password)
  const hasNumbers = /\d/.test(password)

  return {
    minLengthMet: trimmedPassword.length >= AUTH_PASSWORD_MIN_LENGTH,
    remainingLength: Math.max(0, AUTH_PASSWORD_MIN_LENGTH - trimmedPassword.length),
    recommendedMixMet: hasLetters && hasNumbers,
  }
}
