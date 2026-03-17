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

  return (
    <div className="fixed inset-0 z-[80] flex items-center justify-center bg-black/65 px-4 backdrop-blur-sm">
      <div className="w-full max-w-md rounded-2xl border border-white/10 bg-slate-950 p-6 shadow-2xl">
        <div className="mb-5 flex items-start justify-between gap-4">
          <div>
            <h2 className="text-xl font-semibold text-white">{mode === 'register' ? '注册账号' : '登录账号'}</h2>
            <p className="mt-2 text-sm text-white/55">{reason || '登录后可使用策略创建、实盘关注池等用户功能。'}</p>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="rounded-lg border border-white/10 px-2 py-1 text-xs text-white/70 transition hover:border-white/25 hover:text-white"
          >
            关闭
          </button>
        </div>

        <div className="mb-4 flex items-center gap-2 rounded-xl border border-white/10 bg-black/25 p-1">
          <button
            type="button"
            onClick={() => onSwitchMode('login')}
            className={`flex-1 rounded-lg px-3 py-2 text-sm transition ${mode === 'login' ? 'bg-primary text-black font-semibold' : 'text-white/70 hover:text-white'}`}
          >
            登录
          </button>
          <button
            type="button"
            onClick={() => onSwitchMode('register')}
            className={`flex-1 rounded-lg px-3 py-2 text-sm transition ${mode === 'register' ? 'bg-primary text-black font-semibold' : 'text-white/70 hover:text-white'}`}
          >
            注册
          </button>
        </div>

        <form onSubmit={submit} className="space-y-3">
          <input
            value={email}
            onChange={(event) => setEmail(event.target.value)}
            type="email"
            autoComplete="email"
            placeholder="邮箱"
            className="w-full rounded-xl border border-border bg-black/20 px-4 py-2.5 text-sm text-white outline-none transition focus:border-primary"
          />
          <input
            value={password}
            onChange={(event) => setPassword(event.target.value)}
            type="password"
            autoComplete={mode === 'register' ? 'new-password' : 'current-password'}
            placeholder="密码"
            className="w-full rounded-xl border border-border bg-black/20 px-4 py-2.5 text-sm text-white outline-none transition focus:border-primary"
          />
          {mode === 'register' ? (
            <input
              value={confirmPassword}
              onChange={(event) => setConfirmPassword(event.target.value)}
              type="password"
              autoComplete="new-password"
              placeholder="确认密码"
              className="w-full rounded-xl border border-border bg-black/20 px-4 py-2.5 text-sm text-white outline-none transition focus:border-primary"
            />
          ) : null}

          {error ? <div className="rounded-lg border border-rose-400/40 bg-rose-500/10 px-3 py-2 text-sm text-rose-200">{error}</div> : null}

          <button
            type="submit"
            disabled={submitting}
            className="w-full rounded-xl bg-primary px-4 py-2.5 text-sm font-semibold text-black transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-65"
          >
            {submitting ? '提交中...' : mode === 'register' ? '注册并登录' : '登录'}
          </button>
        </form>
      </div>
    </div>
  )
}
