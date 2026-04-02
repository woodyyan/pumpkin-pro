import Head from 'next/head'
import { useCallback, useEffect, useRef, useState } from 'react'

const ADMIN_SESSION_KEY = 'pumpkin_pro_admin_session'
const REFRESH_INTERVAL = 60_000

function readAdminSession() {
  if (typeof window === 'undefined') return null
  try {
    const raw = window.localStorage.getItem(ADMIN_SESSION_KEY)
    if (!raw) return null
    const parsed = JSON.parse(raw)
    if (!parsed?.tokens?.access_token || !parsed?.admin) return null
    return parsed
  } catch {
    return null
  }
}

function writeAdminSession(session) {
  if (typeof window === 'undefined') return
  if (!session) {
    window.localStorage.removeItem(ADMIN_SESSION_KEY)
    return
  }
  window.localStorage.setItem(ADMIN_SESSION_KEY, JSON.stringify(session))
}

async function adminFetch(path, init = {}) {
  const session = readAdminSession()
  const headers = new Headers(init?.headers || {})
  headers.set('Accept', 'application/json')
  if (session?.tokens?.access_token) {
    headers.set('Authorization', `Bearer ${session.tokens.access_token}`)
  }

  const res = await fetch(path, { ...init, headers })
  const text = await res.text()
  let data = null
  try {
    data = JSON.parse(text)
  } catch {
    data = text
  }

  if (!res.ok) {
    const err = new Error(data?.detail || '请求失败')
    err.status = res.status
    throw err
  }
  return data
}

// ── Login Form ──

function AdminLoginForm({ onLogin }) {
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const submit = async (e) => {
    e.preventDefault()
    setError('')
    if (!email.trim() || !password) {
      setError('请输入邮箱和密码')
      return
    }
    setLoading(true)
    try {
      const result = await adminFetch('/api/admin/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email: email.trim(), password }),
      })
      writeAdminSession(result)
      onLogin(result)
    } catch (err) {
      setError(err.message || '登录失败')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="min-h-screen bg-[#0a0b0f] flex items-center justify-center px-4">
      <div className="w-full max-w-md">
        <div className="text-center mb-8">
          <img src="/logo.png" alt="卧龙" width={56} height={56} className="mx-auto rounded" />
          <h1 className="mt-3 text-2xl font-bold text-white">Wolong Pro 管理后台</h1>
          <p className="mt-2 text-sm text-white/50">仅限超级管理员访问</p>
        </div>

        <form
          onSubmit={submit}
          className="rounded-2xl border border-white/10 bg-[#121317]/95 p-6 shadow-2xl"
        >
          <div className="space-y-4">
            <div>
              <label className="block text-xs text-white/50 mb-1.5">管理员邮箱</label>
              <input
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                autoComplete="email"
                className="w-full rounded-xl border border-[#303543] bg-[#191d27] px-4 py-2.5 text-sm text-white outline-none transition focus:border-amber-400 focus:bg-[#202633]"
                placeholder="admin@example.com"
              />
            </div>
            <div>
              <label className="block text-xs text-white/50 mb-1.5">密码</label>
              <input
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                autoComplete="current-password"
                className="w-full rounded-xl border border-[#303543] bg-[#191d27] px-4 py-2.5 text-sm text-white outline-none transition focus:border-amber-400 focus:bg-[#202633]"
                placeholder="••••••••"
              />
            </div>
          </div>

          {error && (
            <div className="mt-4 rounded-xl bg-rose-500/12 px-3 py-2 text-sm text-rose-200">{error}</div>
          )}

          <button
            type="submit"
            disabled={loading}
            className="mt-5 w-full rounded-xl bg-amber-500 px-4 py-3 text-sm font-semibold text-black transition hover:bg-amber-400 disabled:cursor-not-allowed disabled:opacity-60"
          >
            {loading ? '登录中...' : '登录管理后台'}
          </button>
        </form>
      </div>
    </div>
  )
}

// ── Stat Card ──

function StatCard({ label, value, sub }) {
  return (
    <div className="rounded-xl border border-white/8 bg-[#15171e] px-4 py-3">
      <div className="text-xs text-white/45 mb-1">{label}</div>
      <div className="text-2xl font-bold text-white tabular-nums">{value ?? '--'}</div>
      {sub && <div className="mt-0.5 text-xs text-white/40">{sub}</div>}
    </div>
  )
}

function RateCard({ label, value }) {
  const pct = value != null ? `${(value * 100).toFixed(1)}%` : '--'
  return <StatCard label={label} value={pct} />
}

// ── Dashboard ──

function AdminDashboard({ session, onLogout }) {
  const [stats, setStats] = useState(null)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  const [lastRefresh, setLastRefresh] = useState(null)
  const timerRef = useRef(null)

  const loadStats = useCallback(async () => {
    try {
      setError('')
      const data = await adminFetch('/api/admin/stats')
      setStats(data)
      setLastRefresh(new Date())
    } catch (err) {
      if (err.status === 401) {
        writeAdminSession(null)
        onLogout()
        return
      }
      setError(err.message || '加载统计数据失败')
    } finally {
      setLoading(false)
    }
  }, [onLogout])

  useEffect(() => {
    loadStats()
    timerRef.current = setInterval(loadStats, REFRESH_INTERVAL)
    return () => clearInterval(timerRef.current)
  }, [loadStats])

  const adminEmail = session?.admin?.email || '管理员'

  return (
    <div className="min-h-screen bg-[#0a0b0f] text-white">
      {/* Header */}
      <header className="sticky top-0 z-50 border-b border-white/10 bg-[#0a0b0f]/90 backdrop-blur-md">
        <div className="mx-auto flex max-w-7xl items-center justify-between px-6 py-3">
          <div className="flex items-center gap-3">
            <img src="/logo.png" alt="卧龙" width={32} height={32} className="rounded" />
            <span className="text-lg font-bold">Wolong Pro 管理后台</span>
          </div>
          <div className="flex items-center gap-4 text-sm">
            {lastRefresh && (
              <span className="text-white/35">
                更新于 {lastRefresh.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit', second: '2-digit' })}
              </span>
            )}
            <span className="text-white/60">{adminEmail}</span>
            <button
              type="button"
              onClick={() => {
                writeAdminSession(null)
                onLogout()
              }}
              className="rounded-lg border border-white/15 px-3 py-1 text-sm text-white/70 transition hover:border-white/30 hover:text-white"
            >
              退出
            </button>
          </div>
        </div>
      </header>

      <main className="mx-auto max-w-7xl px-6 py-8 space-y-8">
        {error && (
          <div className="rounded-xl bg-rose-500/12 border border-rose-400/20 px-4 py-3 text-sm text-rose-200">
            {error}
          </div>
        )}

        {loading && !stats ? (
          <div className="py-20 text-center text-white/40">加载中…</div>
        ) : stats ? (
          <>
            {/* Panel 1: Users */}
            <section>
              <h2 className="text-base font-semibold text-white/80 mb-3">👤 用户概览</h2>
              <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-6 gap-3">
                <StatCard label="注册用户总数" value={stats.users.total} />
                <StatCard label="今日新增" value={stats.users.today} />
                <StatCard label="7天新增" value={stats.users.last_7d} />
                <StatCard label="30天新增" value={stats.users.last_30d} />
                <StatCard label="7天活跃用户" value={stats.users.active_7d} />
                <StatCard label="当前有效会话" value={stats.users.active_sessions} />
              </div>
            </section>

            {/* Panel 2: Strategies */}
            <section>
              <h2 className="text-base font-semibold text-white/80 mb-3">📊 策略使用</h2>
              <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-3">
                <StatCard label="策略总数" value={stats.strategies.total} />
                <StatCard label="系统策略" value={stats.strategies.system} />
                <StatCard label="用户自建" value={stats.strategies.user_created} />
                <StatCard label="启用策略" value={stats.strategies.active} />
                <StatCard label="被引用策略" value={stats.strategies.referenced} />
              </div>
            </section>

            {/* Panel 3: Live */}
            <section>
              <h2 className="text-base font-semibold text-white/80 mb-3">📈 行情看板</h2>
              <div className="grid grid-cols-2 sm:grid-cols-3 gap-3">
                <StatCard label="关注池条目" value={stats.live.watchlist_items} />
                <StatCard label="有关注的用户" value={stats.live.users_with_watchlist} />
                <StatCard label="已激活标的" value={stats.live.active_symbols} />
              </div>
            </section>

            {/* Panel 4: Signals & Webhook */}
            <section>
              <h2 className="text-base font-semibold text-white/80 mb-3">🔔 信号与 Webhook</h2>
              <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-3">
                <StatCard label="已配置 Webhook" value={stats.signals.webhook_users} sub="用户数" />
                <RateCard label="Webhook 启用率" value={stats.signals.webhook_enabled_rate} />
                <StatCard label="信号配置" value={stats.signals.signal_configs} />
                <StatCard label="已启用配置" value={stats.signals.signal_configs_enabled} />
                <StatCard label="累计信号事件" value={stats.signals.total_events} />
                <StatCard label="今日信号事件" value={stats.signals.today_events} />
                <RateCard label="投递成功率" value={stats.signals.delivery_success_rate} />
                <StatCard label="今日投递" value={stats.signals.today_deliveries} />
              </div>
            </section>

            {/* Panel 5: Audit */}
            <section>
              <h2 className="text-base font-semibold text-white/80 mb-3">🛡️ 审计日志</h2>
              <div className="grid grid-cols-2 sm:grid-cols-3 gap-3">
                <StatCard label="今日登录次数" value={stats.audit.today_logins} />
                <StatCard label="今日注册次数" value={stats.audit.today_registrations} />
                <StatCard label="7天登录失败" value={stats.audit.failed_logins_7d} />
              </div>
            </section>
          </>
        ) : null}
      </main>
    </div>
  )
}

// ── Page ──

export default function AdminPage() {
  const [session, setSession] = useState(null)
  const [ready, setReady] = useState(false)

  useEffect(() => {
    const cached = readAdminSession()
    if (cached) setSession(cached)
    setReady(true)
  }, [])

  if (!ready) {
    return (
      <>
        <Head>
          <title>管理后台 — Wolong Pro</title>
        </Head>
        <div className="min-h-screen bg-[#0a0b0f]" />
      </>
    )
  }

  return (
    <>
      <Head>
        <title>管理后台 — Wolong Pro</title>
      </Head>
      {session ? (
        <AdminDashboard session={session} onLogout={() => setSession(null)} />
      ) : (
        <AdminLoginForm onLogin={(result) => setSession(result)} />
      )}
    </>
  )
}
