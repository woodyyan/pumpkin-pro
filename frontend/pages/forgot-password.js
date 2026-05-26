import Head from 'next/head'
import { useState } from 'react'

import { requestJson } from '../lib/api'

const PAGE_THEME = {
  shell: 'min-h-screen bg-[radial-gradient(circle_at_top,#19324a,transparent_42%),linear-gradient(180deg,#09131d,#0d1823_55%,#13283a)] px-4 py-10 text-foreground',
  card: 'mx-auto w-full max-w-[30rem] rounded-[2rem] border border-border bg-[#0f1722]/90 p-6 shadow-[0_30px_80px_rgba(0,0,0,0.32)] backdrop-blur',
  input: 'w-full rounded-2xl border border-[var(--color-border-strong)] bg-[var(--color-bg-hover)] px-4 py-3 text-sm text-foreground outline-none transition placeholder:text-foreground-dim focus:border-primary focus:bg-[var(--color-bg-hover)]',
  button: 'w-full rounded-2xl bg-primary px-4 py-3 text-sm font-semibold text-black transition hover:bg-primary/90 disabled:cursor-not-allowed disabled:opacity-60',
}

export default function ForgotPasswordPage() {
  const [email, setEmail] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [retryAfter, setRetryAfter] = useState(0)

  const submit = async (event) => {
    event.preventDefault()
    setError('')
    setNotice('')
    setRetryAfter(0)

    if (!email.trim()) {
      setError('请输入注册邮箱')
      return
    }

    setSubmitting(true)
    try {
      const result = await requestJson('/api/auth/forgot-password', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email: email.trim() }),
      }, '发送重置邮件失败')
      setNotice(result?.message || '如该邮箱已注册，我们将发送重置邮件')
    } catch (err) {
      if (err?.code === 'RATE_LIMITED') {
        setRetryAfter(Number(err?.responseData?.retry_after_seconds || 0))
      }
      setError(err.message || '发送重置邮件失败')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <>
      <Head>
        <title>找回密码 | Wolong Trader</title>
      </Head>
      <main className={PAGE_THEME.shell}>
        <div className={PAGE_THEME.card}>
          <div className="mb-6 space-y-3">
            <span className="inline-flex rounded-full border border-primary/35 bg-primary/10 px-3 py-1 text-xs font-semibold uppercase tracking-[0.18em] text-primary">Password Reset</span>
            <h1 className="text-3xl font-semibold">通过邮箱重置密码</h1>
            <p className="text-sm leading-7 text-foreground-muted">输入你的注册邮箱后，我们会发送一封密码重置邮件。邮件里只包含重置链接和有效期说明，不会展示你的账号敏感信息。</p>
          </div>

          <form onSubmit={submit} className="space-y-4">
            <input
              type="email"
              autoComplete="email"
              className={PAGE_THEME.input}
              placeholder="注册邮箱"
              value={email}
              onChange={(event) => setEmail(event.target.value)}
            />

            {notice ? <div className="rounded-2xl border border-positive/20 bg-emerald-400/10 px-4 py-3 text-sm text-emerald-100">{notice}</div> : null}
            {error ? <div className="rounded-2xl border border-rose-400/20 bg-rose-400/10 px-4 py-3 text-sm text-negative">{error}{retryAfter > 0 ? `，请约 ${retryAfter} 秒后再试。` : ''}</div> : null}

            <button type="submit" disabled={submitting} className={PAGE_THEME.button}>
              {submitting ? '发送中...' : '发送重置邮件'}
            </button>
          </form>

          <div className="mt-6 flex items-center justify-between text-xs text-foreground-dim">
            <a href="/" className="transition hover:text-foreground">返回首页</a>
            <a href="javascript:history.back()" className="transition hover:text-foreground">返回登录</a>
          </div>
        </div>
      </main>
    </>
  )
}
