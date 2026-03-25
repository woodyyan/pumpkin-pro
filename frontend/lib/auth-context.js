import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react'

import { requestJson } from './api'
import {
  clearAuthSession,
  getRefreshToken,
  isAuthRequiredError,
  readAuthSession,
  writeAuthSession,
} from './auth-storage'

const AuthContext = createContext(null)

export function AuthProvider({ children }) {
  const [session, setSession] = useState(null)
  const [ready, setReady] = useState(false)
  const [modalState, setModalState] = useState({
    open: false,
    mode: 'login',
    reason: '',
  })

  const syncSession = useCallback((payload) => {
    if (!payload?.tokens?.access_token || !payload?.tokens?.refresh_token || !payload?.user) {
      return null
    }
    const next = {
      user: payload.user,
      tokens: payload.tokens,
    }
    setSession(next)
    writeAuthSession(next)
    return next
  }, [])

  const clearSession = useCallback(() => {
    setSession(null)
    clearAuthSession()
  }, [])

  const fetchMe = useCallback(async () => {
    try {
      const result = await requestJson('/api/user/me', undefined, '读取账号信息失败')
      if (!result?.user) return
      setSession((prev) => {
        if (!prev?.tokens) return prev
        const next = {
          ...prev,
          user: result.user,
        }
        writeAuthSession(next)
        return next
      })
    } catch (error) {
      if (isAuthRequiredError(error)) {
        clearSession()
      }
    }
  }, [clearSession])

  useEffect(() => {
    const cached = readAuthSession()
    if (cached?.tokens?.access_token && cached?.user) {
      setSession(cached)
      fetchMe().finally(() => setReady(true))
      return
    }
    setReady(true)
  }, [fetchMe])

  const openAuthModal = useCallback((mode = 'login', reason = '') => {
    setModalState({ open: true, mode, reason })
  }, [])

  const closeAuthModal = useCallback(() => {
    setModalState((prev) => ({ ...prev, open: false, reason: '' }))
  }, [])

  const login = useCallback(async ({ email, password }) => {
    const result = await requestJson(
      '/api/auth/login',
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email, password }),
      },
      '登录失败',
    )
    return syncSession(result)
  }, [syncSession])

  const register = useCallback(async ({ email, password }) => {
    const result = await requestJson(
      '/api/auth/register',
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email, password }),
      },
      '注册失败',
    )
    return syncSession(result)
  }, [syncSession])

  const logout = useCallback(async () => {
    const refreshToken = getRefreshToken()
    try {
      await requestJson(
        '/api/auth/logout',
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ refresh_token: refreshToken }),
        },
        '退出登录失败',
      )
    } catch {
      // 无论后端是否成功，都清理本地登录态
    }
    clearSession()
  }, [clearSession])

  const value = useMemo(() => ({
    user: session?.user || null,
    isLoggedIn: !!session?.user,
    ready,
    openAuthModal,
    closeAuthModal,
    login,
    register,
    logout,
  }), [closeAuthModal, login, logout, openAuthModal, ready, register, session?.user])

  return (
    <AuthContext.Provider value={value}>
      {children}
      <AuthDialog
        open={modalState.open}
        mode={modalState.mode}
        reason={modalState.reason}
        onClose={closeAuthModal}
        onSwitchMode={(mode) => setModalState((prev) => ({ ...prev, mode }))}
        onLogin={login}
        onRegister={register}
      />
    </AuthContext.Provider>
  )
}

export function useAuth() {
  const value = useContext(AuthContext)
  if (!value) {
    throw new Error('useAuth 必须在 AuthProvider 内使用')
  }
  return value
}

const AUTH_DIALOG_THEME = {
  overlay: 'bg-black/70 backdrop-blur-[2px]',
  panel: 'w-full max-w-[25.5rem] rounded-[1.75rem] bg-[#121317]/95 p-6 shadow-[0_22px_68px_rgba(0,0,0,0.56)] ring-1 ring-primary/25 sm:min-w-[25.5rem]',
  closeBtn: 'bg-[#1f232d] text-slate-300 hover:bg-[#2a303c] hover:text-white',
  hintCard: 'rounded-2xl bg-[#1a1d25] px-4 py-3 text-sm leading-6 text-slate-200',
  modeWrap: 'mb-6 grid grid-cols-2 gap-2 rounded-2xl bg-[#1a1d25] p-1',
  modeActive: 'bg-primary text-black font-semibold',
  modeIdle: 'text-slate-300 hover:bg-white/5 hover:text-white',
  input: 'w-full rounded-2xl border border-[#303543] bg-[#191d27] px-4 py-2.5 text-sm text-white outline-none transition focus:border-primary focus:bg-[#202633]',
  submit: 'w-full rounded-2xl bg-primary px-4 py-3 text-sm font-semibold text-black transition hover:bg-primary/90 disabled:cursor-not-allowed disabled:opacity-65',
}

function AuthDialog({
  open,
  mode,
  reason,
  onClose,
  onSwitchMode,
  onLogin,
  onRegister,
}) {
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    if (!open) {
      setEmail('')
      setPassword('')
      setConfirmPassword('')
      setSubmitting(false)
      setError('')
    }
  }, [open])

  if (!open) return null

  const submit = async (event) => {
    event.preventDefault()
    setError('')

    if (!email.trim() || !password) {
      setError('请输入邮箱和密码')
      return
    }
    if (mode === 'register' && password !== confirmPassword) {
      setError('两次密码输入不一致')
      return
    }

    setSubmitting(true)
    try {
      if (mode === 'register') {
        await onRegister({ email: email.trim(), password })
      } else {
        await onLogin({ email: email.trim(), password })
      }
      onClose()
    } catch (err) {
      setError(err.message || (mode === 'register' ? '注册失败' : '登录失败'))
    } finally {
      setSubmitting(false)
    }
  }

  const theme = AUTH_DIALOG_THEME

  return (
    <div className={`fixed inset-0 z-[80] flex items-center justify-center px-4 py-8 ${theme.overlay}`}>
      <div className={theme.panel}>
        <div className="mb-5 flex items-start justify-between gap-4">
          <div className="space-y-1">
            <h2 className="text-xl font-semibold text-white">{mode === 'register' ? '注册账号' : '登录账号'}</h2>
            <p className="text-sm text-slate-300">{mode === 'register' ? '创建一个新账号，开始你的交易旅程。' : '欢迎回来，继续你的交易计划。'}</p>
          </div>
          <button
            type="button"
            onClick={onClose}
            className={`grid size-8 place-items-center rounded-full text-base leading-none transition ${theme.closeBtn}`}
            aria-label="关闭"
          >
            ×
          </button>
        </div>


        <div className={`mb-5 ${theme.hintCard}`}>
          {reason || '登录后可使用策略创建、行情关注池等用户功能。'}
        </div>

        <div className={theme.modeWrap}>
          <button
            type="button"
            onClick={() => onSwitchMode('login')}
            className={`rounded-xl px-3 py-2 text-sm transition ${mode === 'login' ? theme.modeActive : theme.modeIdle}`}
          >
            登录
          </button>
          <button
            type="button"
            onClick={() => onSwitchMode('register')}
            className={`rounded-xl px-3 py-2 text-sm transition ${mode === 'register' ? theme.modeActive : theme.modeIdle}`}
          >
            注册
          </button>
        </div>

        <form onSubmit={submit} className="space-y-4">
          <div className="space-y-3">
            <input
              value={email}
              onChange={(event) => setEmail(event.target.value)}
              type="email"
              autoComplete="email"
              placeholder="邮箱"
              className={theme.input}
            />
            <input
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              type="password"
              autoComplete={mode === 'register' ? 'new-password' : 'current-password'}
              placeholder="密码"
              className={theme.input}
            />
            {mode === 'register' ? (
              <input
                value={confirmPassword}
                onChange={(event) => setConfirmPassword(event.target.value)}
                type="password"
                autoComplete="new-password"
                placeholder="确认密码"
                className={theme.input}
              />
            ) : null}
          </div>

          {error ? <div className="rounded-xl bg-rose-500/12 px-3 py-2 text-sm text-rose-200">{error}</div> : null}

          <div className="pt-2">
            <button
              type="submit"
              disabled={submitting}
              className={theme.submit}
            >
              {submitting ? '提交中...' : mode === 'register' ? '注册并登录' : '登录'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
