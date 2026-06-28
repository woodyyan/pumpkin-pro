import Head from 'next/head'
import { useRouter } from 'next/router'
import { useEffect, useState } from 'react'

import { requestJson } from '../lib/api'
import { broadcastAuthSessionCleared, clearAuthSession } from '../lib/auth-storage'

const PAGE_THEME = {
  shell: 'min-h-screen bg-[radial-gradient(circle_at_top,var(--color-radial-accent),transparent_42%)] bg-background px-4 py-10 text-foreground',
  card: 'mx-auto w-full max-w-[30rem] rounded-[2rem] border border-border bg-card p-6 shadow-[0_30px_80px_rgba(0,0,0,0.12)] backdrop-blur',
  input: 'w-full rounded-2xl border border-[var(--color-border-strong)] bg-[var(--color-bg-hover)] px-4 py-3 text-sm text-foreground outline-none transition placeholder:text-foreground-dim focus:border-primary focus:bg-[var(--color-bg-hover)]',
  button: 'w-full rounded-2xl bg-primary px-4 py-3 text-sm font-semibold text-black transition hover:bg-primary/90 disabled:cursor-not-allowed disabled:opacity-60',
}

export default function ResetPasswordPage() {
  const router = useRouter()
  const token = typeof router.query.token === 'string' ? router.query.token : ''
  const [password, setPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [checking, setChecking] = useState(true)
  const [tokenStatus, setTokenStatus] = useState({ valid: false, detail: '' })
  const [notice, setNotice] = useState('')
  const [error, setError] = useState('')

  useEffect(() => {
    if (!router.isReady) return
    if (!token) {
      setChecking(false)
      setTokenStatus({ valid: false, detail: '重置链接缺少 token 参数。' })
      return
    }

    let active = true
    setChecking(true)
    requestJson(`/api/auth/reset-password/inspect?token=${encodeURIComponent(token)}`, undefined, '校验重置链接失败')
      .then((result) => {
        if (!active) return
        setTokenStatus({
          valid: Boolean(result?.valid),
          detail: result?.detail || '',
        })
      })
      .catch((err) => {
        if (!active) return
        setTokenStatus({ valid: false, detail: err.message || '校验重置链接失败' })
      })
      .finally(() => {
        if (active) setChecking(false)
      })
    return () => {
      active = false
    }
  }, [router.isReady, token])

  const submit = async (event) => {
    event.preventDefault()
    setError('')
    setNotice('')

    if (!token) {
      setError('重置链接缺少 token 参数')
      return
    }
    if (!password || !confirmPassword) {
      setError('请输入并确认新密码')
      return
    }
    if (password !== confirmPassword) {
      setError('两次密码输入不一致')
      return
    }

    setSubmitting(true)
    try {
      const result = await requestJson('/api/auth/reset-password', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ token, new_password: password }),
      }, '重置密码失败')
      clearAuthSession()
      broadcastAuthSessionCleared('password_reset')
      setNotice(result?.message || '密码重置成功，请重新登录')
      setTokenStatus({ valid: false, detail: '该重置链接已使用完成。' })
      setPassword('')
      setConfirmPassword('')
    } catch (err) {
      setError(err.message || '重置密码失败')
      setTokenStatus((prev) => ({ ...prev, valid: false }))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <>
      <Head>
        <title>重置密码 | Wolong Trader</title>
      </Head>
      <main className={PAGE_THEME.shell}>
        <div className={PAGE_THEME.card}>
          <div className="mb-6 space-y-3">
            <span className="inline-flex rounded-full border border-primary/35 bg-primary/10 px-3 py-1 text-xs font-semibold uppercase tracking-[0.18em] text-primary">Secure Reset</span>
            <h1 className="text-3xl font-semibold">设置新的登录密码</h1>
            <p className="text-sm leading-7 text-foreground-muted">重置成功后，当前账号在其他标签页和设备上的登录态会同时失效，你需要用新密码重新登录。</p>
          </div>

          {checking ? <div className="rounded-2xl border border-border bg-[var(--color-bg-hover)] px-4 py-4 text-sm text-foreground-muted">正在校验重置链接...</div> : null}
          {!checking && !tokenStatus.valid ? <div className="rounded-2xl border border-amber-400/20 bg-amber-400/10 px-4 py-4 text-sm text-amber-100">{tokenStatus.detail || '该重置链接不可用，请重新申请。'}</div> : null}

          <form onSubmit={submit} className="mt-4 space-y-4">
            <input
              type="password"
              autoComplete="new-password"
              className={PAGE_THEME.input}
              placeholder="新密码（至少 8 位）"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              disabled={checking || !tokenStatus.valid || submitting}
            />
            <input
              type="password"
              autoComplete="new-password"
              className={PAGE_THEME.input}
              placeholder="确认新密码"
              value={confirmPassword}
              onChange={(event) => setConfirmPassword(event.target.value)}
              disabled={checking || !tokenStatus.valid || submitting}
            />

            {notice ? <div className="rounded-2xl border border-positive/20 bg-emerald-400/10 px-4 py-3 text-sm text-emerald-100">{notice}</div> : null}
            {error ? <div className="rounded-2xl border border-rose-400/20 bg-rose-400/10 px-4 py-3 text-sm text-negative">{error}</div> : null}

            <button type="submit" disabled={checking || !tokenStatus.valid || submitting} className={PAGE_THEME.button}>
              {submitting ? '重置中...' : '确认重置密码'}
            </button>
          </form>

          <div className="mt-6 flex items-center justify-between text-xs text-foreground-dim">
            <a href="/forgot-password" className="transition hover:text-foreground">重新申请链接</a>
            <a href="/" className="transition hover:text-foreground">返回首页</a>
          </div>
        </div>
      </main>
    </>
  )
}
