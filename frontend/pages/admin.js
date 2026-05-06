import Head from 'next/head'
import { useCallback, useEffect, useRef, useState } from 'react'
import MiniChart from '../components/MiniChart'

// ── Simple Doughnut Chart (SVG) ──

function DoughnutChart({ data, size = 140, strokeWidth = 18 }) {
  if (!data || data.length === 0) return null
  const total = data.reduce((sum, d) => sum + (d.count || 0), 0)
  if (total === 0) return null

  const radius = (size - strokeWidth) / 2
  const circumference = 2 * Math.PI * radius
  let offset = 0
  const colors = ['#6366f1', '#ec4899', '#10b981', '#f59e0b', '#3b82f6', '#ef4444', '#8b5cf6', '#14b8a6']

  return (
    <div className="flex flex-col items-center">
      <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`}>
        {data.map((item, i) => {
          const pct = (item.count / total)
          const dash = pct * circumference
          const circle = (
            <circle
              key={i}
              cx={size / 2}
              cy={size / 2}
              r={radius}
              fill="none"
              stroke={colors[i % colors.length]}
              strokeWidth={strokeWidth}
              strokeDasharray={`${dash} ${circumference - dash}`}
              strokeDashoffset={-offset}
              strokeLinecap="butt"
              transform={`rotate(-90 ${size / 2} ${size / 2})`}
            />
          )
          offset += dash
          return circle
        })}
      </svg>
    </div>
  )
}

function CategoryLegend({ data }) {
  const colors = ['#6366f1', '#ec4899', '#10b981', '#f59e0b', '#3b82f6', '#ef4444', '#8b5cf6', '#14b8a6']
  if (!data || data.length === 0) return null
  return (
    <div className="mt-3 space-y-1.5">
      {data.map((item, i) => (
        <div key={i} className="flex items-center justify-between text-xs">
          <div className="flex items-center gap-2">
            <span className="inline-block h-2 w-2 rounded-full" style={{ backgroundColor: colors[i % colors.length] }} />
            <span className="text-white/60">{item.category}</span>
          </div>
          <div className="tabular-nums text-white/45">
            {item.count} <span className="text-white/25">({(item.percentage || 0).toFixed(1)}%)</span>
          </div>
        </div>
      ))}
    </div>
  )
}

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

function formatNumber(value) {
  if (value == null || Number.isNaN(Number(value))) return '--'
  return Number(value).toLocaleString('zh-CN')
}

function formatUserDisplay(user) {
  if (!user) return '--'
  if (user.email) return user.email
  if (user.user_id) return `${user.user_id.slice(0, 12)}…`
  return '--'
}

const AI_CONFIG_SOURCE_LABELS = {
  admin: '后台配置',
  env: '环境变量',
  none: '未配置',
}

const AI_CONFIG_STATUS_META = {
  available: {
    label: '可用',
    badgeClassName: 'border-emerald-400/25 bg-emerald-500/12 text-emerald-200',
  },
  invalid_auth: {
    label: '鉴权失败',
    badgeClassName: 'border-rose-400/25 bg-rose-500/12 text-rose-200',
  },
  invalid_model: {
    label: '模型不可用',
    badgeClassName: 'border-amber-400/25 bg-amber-500/12 text-amber-100',
  },
  timeout: {
    label: '请求超时',
    badgeClassName: 'border-amber-400/25 bg-amber-500/12 text-amber-100',
  },
  network_error: {
    label: '网络异常',
    badgeClassName: 'border-amber-400/25 bg-amber-500/12 text-amber-100',
  },
  provider_error: {
    label: '服务异常',
    badgeClassName: 'border-white/15 bg-white/7 text-white/75',
  },
  disabled: {
    label: '已禁用',
    badgeClassName: 'border-white/12 bg-white/6 text-white/55',
  },
  unknown: {
    label: '未测试',
    badgeClassName: 'border-sky-400/20 bg-sky-500/10 text-sky-100',
  },
  unconfigured: {
    label: '未配置',
    badgeClassName: 'border-white/12 bg-white/6 text-white/55',
  },
}

function createAIConfigDraft(view) {
  return {
    base_url: view?.config?.base_url || '',
    model_id: view?.config?.model_id || '',
    api_key: '',
    is_enabled: Boolean(view?.config?.is_enabled),
  }
}

function getAIConfigStatusMeta(status) {
  return AI_CONFIG_STATUS_META[status] || AI_CONFIG_STATUS_META.unknown
}

function formatAdminDateTime(value) {
  if (!value) return '--'
  try {
    return new Date(value).toLocaleString('zh-CN', {
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
    })
  } catch {
    return '--'
  }
}

function AIConfigMetric({ label, value, sub }) {
  return (
    <div className="rounded-2xl border border-white/8 bg-[#171a21] px-4 py-3">
      <div className="text-[11px] text-white/40">{label}</div>
      <div className="mt-1 text-sm font-semibold text-white">{value || '--'}</div>
      {sub ? <div className="mt-1 text-[11px] text-white/35">{sub}</div> : null}
    </div>
  )
}

function AIProviderConfigPanel({ onUnauthorized }) {
  const [view, setView] = useState(null)
  const [draft, setDraft] = useState(() => createAIConfigDraft(null))
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState(false)
  const [banner, setBanner] = useState(null)
  const [testResult, setTestResult] = useState(null)
  const initializedRef = useRef(false)

  const handleUnauthorized = useCallback(() => {
    writeAdminSession(null)
    onUnauthorized?.()
  }, [onUnauthorized])

  const loadConfig = useCallback(async ({ resetDraft = false } = {}) => {
    try {
      const data = await adminFetch('/api/admin/ai-config')
      setView(data)
      if (resetDraft || !initializedRef.current) {
        setDraft(createAIConfigDraft(data))
        initializedRef.current = true
      }
      if (resetDraft) {
        setBanner(null)
        setTestResult(null)
      }
    } catch (err) {
      if (err.status === 401) {
        handleUnauthorized()
        return
      }
      setBanner({ tone: 'error', text: err.message || '加载 AI 配置失败' })
    } finally {
      setLoading(false)
    }
  }, [handleUnauthorized])

  useEffect(() => {
    loadConfig({ resetDraft: true })
  }, [loadConfig])

  const health = testResult || view?.health
  const healthMeta = getAIConfigStatusMeta(health?.status)
  const sourceLabel = AI_CONFIG_SOURCE_LABELS[view?.effective?.source] || AI_CONFIG_SOURCE_LABELS.none

  const updateDraft = (key, value) => {
    setDraft((current) => ({ ...current, [key]: value }))
  }

  const restoreSaved = () => {
    setDraft(createAIConfigDraft(view))
    setBanner(null)
    setTestResult(null)
  }

  const handleSave = async () => {
    setSaving(true)
    setBanner(null)
    try {
      const data = await adminFetch('/api/admin/ai-config', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          base_url: draft.base_url.trim(),
          model_id: draft.model_id.trim(),
          api_key: draft.api_key,
          is_enabled: draft.is_enabled,
        }),
      })
      setView(data)
      setDraft(createAIConfigDraft(data))
      setTestResult(null)
      setBanner({ tone: 'success', text: 'AI 配置已保存' })
    } catch (err) {
      if (err.status === 401) {
        handleUnauthorized()
        return
      }
      setBanner({ tone: 'error', text: err.message || '保存 AI 配置失败' })
    } finally {
      setSaving(false)
    }
  }

  const handleTest = async () => {
    setTesting(true)
    setBanner(null)
    try {
      const matchesSaved =
        draft.base_url.trim() === (view?.config?.base_url || '').trim() &&
        draft.model_id.trim() === (view?.config?.model_id || '').trim() &&
        draft.is_enabled === Boolean(view?.config?.is_enabled) &&
        !draft.api_key.trim()

      const init = {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
      }
      if (!matchesSaved) {
        init.body = JSON.stringify({
          base_url: draft.base_url.trim(),
          model_id: draft.model_id.trim(),
          api_key: draft.api_key,
          is_enabled: draft.is_enabled,
        })
      }

      const result = await adminFetch('/api/admin/ai-config/test', init)
      setTestResult(result)
      setBanner({
        tone: result?.status === 'available' ? 'success' : 'info',
        text: result?.message || '测试完成',
      })
    } catch (err) {
      if (err.status === 401) {
        handleUnauthorized()
        return
      }
      setBanner({ tone: 'error', text: err.message || '测试连接失败' })
    } finally {
      setTesting(false)
    }
  }

  if (loading && !view) {
    return (
      <section className="rounded-2xl border border-white/8 bg-[#111318] p-5">
        <div className="text-sm text-white/40">加载 AI 配置中…</div>
      </section>
    )
  }

  return (
    <section className="rounded-2xl border border-white/8 bg-[#111318] p-4 sm:p-5">
      <div className="flex flex-col gap-4 xl:flex-row xl:items-start xl:justify-between">
        <div>
          <h2 className="text-base font-semibold text-white/85">🤖 AI 模型配置</h2>
          <p className="mt-1 text-xs text-white/35">当前系统仅支持 OpenAI-compatible Chat Completions 接口</p>
        </div>
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-4 xl:min-w-[520px]">
          <AIConfigMetric label="当前生效" value={sourceLabel} sub={view?.effective?.configured ? '运行中' : '待配置'} />
          <AIConfigMetric label="当前状态" value={healthMeta.label} sub={health?.message || '--'} />
          <AIConfigMetric label="最近延迟" value={health?.latency_ms ? `${health.latency_ms}ms` : '--'} sub="测试连接结果" />
          <AIConfigMetric label="最近测试" value={formatAdminDateTime(health?.checked_at)} sub={view?.effective?.model_id || '--'} />
        </div>
      </div>

      {banner ? (
        <div
          className={`mt-4 rounded-2xl border px-4 py-3 text-sm ${
            banner.tone === 'success'
              ? 'border-emerald-400/20 bg-emerald-500/10 text-emerald-100'
              : banner.tone === 'error'
                ? 'border-rose-400/20 bg-rose-500/10 text-rose-100'
                : 'border-sky-400/20 bg-sky-500/10 text-sky-100'
          }`}
        >
          {banner.text}
        </div>
      ) : null}

      <div className="mt-5 grid grid-cols-1 gap-4 lg:grid-cols-2">
        <div>
          <label className="mb-1.5 block text-xs text-white/45">Base URL</label>
          <input
            type="text"
            value={draft.base_url}
            onChange={(e) => updateDraft('base_url', e.target.value)}
            className="w-full rounded-2xl border border-white/10 bg-[#171a21] px-4 py-3 text-sm text-white outline-none transition focus:border-amber-400/60 focus:bg-[#1b1f28]"
            placeholder="https://api.openai.com/v1"
          />
        </div>
        <div>
          <label className="mb-1.5 block text-xs text-white/45">Model ID</label>
          <input
            type="text"
            value={draft.model_id}
            onChange={(e) => updateDraft('model_id', e.target.value)}
            className="w-full rounded-2xl border border-white/10 bg-[#171a21] px-4 py-3 text-sm text-white outline-none transition focus:border-amber-400/60 focus:bg-[#1b1f28]"
            placeholder="gpt-4o-mini"
          />
        </div>
      </div>

      <div className="mt-4">
        <label className="mb-1.5 block text-xs text-white/45">API Key</label>
        <input
          type="password"
          value={draft.api_key}
          onChange={(e) => updateDraft('api_key', e.target.value)}
          className="w-full rounded-2xl border border-white/10 bg-[#171a21] px-4 py-3 text-sm text-white outline-none transition focus:border-amber-400/60 focus:bg-[#1b1f28]"
          placeholder="留空表示保持当前 key"
        />
        <div className="mt-2 flex flex-wrap items-center gap-2 text-xs text-white/35">
          <span className="rounded-full border border-white/10 bg-white/[0.04] px-2.5 py-1">
            {view?.config?.has_api_key ? `已保存：${view.config.api_key_mask || '已保存'}` : '暂未保存后台 key'}
          </span>
          <span>只有更换 key 时才需要重新输入</span>
        </div>
      </div>

      <label className="mt-4 flex items-start gap-3 rounded-2xl border border-white/8 bg-[#171a21] px-4 py-3">
        <input
          type="checkbox"
          checked={draft.is_enabled}
          onChange={(e) => updateDraft('is_enabled', e.target.checked)}
          className="mt-0.5 h-4 w-4 rounded border-white/20 bg-transparent text-amber-400 focus:ring-amber-400"
        />
        <div>
          <div className="text-sm font-medium text-white">启用后台配置</div>
          <div className="mt-1 text-xs text-white/35">启用后将优先使用这里保存的模型参数；关闭后自动回退到环境变量。</div>
        </div>
      </label>

      <div className="mt-5 grid grid-cols-1 gap-3 sm:grid-cols-3">
        <button
          type="button"
          onClick={handleTest}
          disabled={testing}
          className="rounded-2xl border border-sky-400/25 bg-sky-500/10 px-4 py-3 text-sm font-medium text-sky-100 transition hover:bg-sky-500/16 disabled:cursor-not-allowed disabled:opacity-60"
        >
          {testing ? '测试中…' : '测试连接'}
        </button>
        <button
          type="button"
          onClick={handleSave}
          disabled={saving}
          className="rounded-2xl border border-amber-400/30 bg-amber-500/12 px-4 py-3 text-sm font-medium text-amber-100 transition hover:bg-amber-500/18 disabled:cursor-not-allowed disabled:opacity-60"
        >
          {saving ? '保存中…' : '保存配置'}
        </button>
        <button
          type="button"
          onClick={restoreSaved}
          className="rounded-2xl border border-white/12 bg-white/[0.04] px-4 py-3 text-sm font-medium text-white/72 transition hover:bg-white/[0.08]"
        >
          恢复已保存值
        </button>
      </div>

      <div className="mt-5 flex flex-wrap items-center gap-2 text-xs">
        <span className={`rounded-full border px-2.5 py-1 ${healthMeta.badgeClassName}`}>
          {healthMeta.label}
        </span>
        <span className="text-white/42">当前生效模型：{view?.effective?.model_id || '--'}</span>
        <span className="text-white/28">base URL：{view?.effective?.base_url || '--'}</span>
      </div>
    </section>
  )
}

// ── Dashboard ──

function AdminDashboard({ session, onLogout }) {
  const [stats, setStats] = useState(null)
  const [analytics, setAnalytics] = useState(null)
  const [aiUsage, setAiUsage] = useState(null)
  const [deviceAnalytics, setDeviceAnalytics] = useState(null)
  const [deviceDays, setDeviceDays] = useState(30)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  const [lastRefresh, setLastRefresh] = useState(null)
  const timerRef = useRef(null)

  const loadAll = useCallback(async () => {
    try {
      setError('')
      const [statsData, analyticsData, aiUsageData, deviceData] = await Promise.all([
        adminFetch('/api/admin/stats'),
        adminFetch('/api/admin/analytics').catch(() => null),
        adminFetch('/api/admin/ai-usage?days=30&limit=120').catch(() => null),
        adminFetch(`/api/admin/device-analytics?days=${deviceDays}`).catch(() => null),
      ])
      setStats(statsData)
      setAnalytics(analyticsData)
      setAiUsage(aiUsageData)
      setDeviceAnalytics(deviceData)
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
  }, [onLogout, deviceDays])

  useEffect(() => {
    loadAll()
    timerRef.current = setInterval(loadAll, REFRESH_INTERVAL)
    return () => clearInterval(timerRef.current)
  }, [loadAll])

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
              {/* Trend charts */}
              <div className="mt-4 grid grid-cols-1 md:grid-cols-2 gap-4">
                <div className="rounded-xl border border-white/8 bg-[#15171e] p-3">
                  <MiniChart data={stats.trends?.daily_registrations} label="每日注册" width={380} height={130} color="#22c55e" />
                </div>
                <div className="rounded-xl border border-white/8 bg-[#15171e] p-3">
                  <MiniChart data={stats.trends?.daily_active_users} label="DAU（日活跃）" width={380} height={130} color="#60a5fa" />
                </div>
              </div>
              {/* Retention */}
              {stats.retention && (
                <div className="mt-3 grid grid-cols-2 sm:grid-cols-4 gap-3">
                  <RateCard label="7天留存率" value={stats.retention.day_7_rate} />
                  <RateCard label="30天留存率" value={stats.retention.day_30_rate} />
                </div>
              )}
            </section>

            {/* Panel 2: Feature Usage */}
            <section>
              <h2 className="text-base font-semibold text-white/80 mb-3">🧩 功能使用</h2>
              <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-3">
                <StatCard label="回测总次数" value={stats.features?.backtest_total} />
                <StatCard label="今日回测" value={stats.features?.backtest_today} />
                <StatCard label="回测用户" value={stats.features?.backtest_users} />
                <StatCard label="持仓快照记录" value={stats.features?.portfolio_records} sub="user_portfolios" />
                <StatCard label="持仓用户累计" value={stats.features?.portfolio_users} sub="曾写入过持仓快照" />
                <StatCard label="当前持仓标的" value={stats.features?.portfolio_active_positions} sub="shares > 0" />
                <StatCard label="当前持仓用户" value={stats.features?.portfolio_active_users} sub="仍有持仓的用户" />
                <StatCard label="累计持仓操作" value={stats.features?.portfolio_event_total} sub="买入 / 卖出 / 调均价" />
                <StatCard label="今日持仓操作" value={stats.features?.portfolio_event_today} />
                <StatCard label="7天持仓活跃用户" value={stats.features?.portfolio_event_users_7d} />
                <StatCard label="已填投资画像" value={stats.features?.portfolio_profile_users} />
                <StatCard label="自选表" value={stats.features?.screener_lists} />
                <StatCard label="选股用户" value={stats.features?.screener_users} />
              </div>
              {stats.trends?.daily_portfolio_ops && stats.trends.daily_portfolio_ops.length > 0 && (
                <div className="mt-4 rounded-xl border border-white/8 bg-[#15171e] p-3">
                  <MiniChart data={stats.trends.daily_portfolio_ops} label="每日持仓操作（30天）" width={780} height={130} type="bar" color="#14b8a6" />
                </div>
              )}
            </section>

            {/* Panel 3: Strategies */}
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

            {/* Panel 4: Live */}
            <section>
              <h2 className="text-base font-semibold text-white/80 mb-3">📈 行情看板</h2>
              <div className="grid grid-cols-2 gap-3 sm:grid-cols-2">
                <StatCard label="关注池条目" value={stats.live.watchlist_items} />
                <StatCard label="有关注的用户" value={stats.live.users_with_watchlist} />
              </div>
            </section>

            {/* Panel 5: Company Profiles */}
            <CompanyProfilesAdminPanel />

            {/* Panel 6: Signals & Webhook */}
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
              {stats.trends?.daily_signal_events && stats.trends.daily_signal_events.length > 0 && (
                <div className="mt-4 rounded-xl border border-white/8 bg-[#15171e] p-3">
                  <MiniChart data={stats.trends.daily_signal_events} label="每日信号事件" width={780} height={130} type="bar" color="#eab308" />
                </div>
              )}
            </section>

            {/* Panel 6: Device & Browser */}
            {deviceAnalytics && (
              <section>
                <div className="flex items-center justify-between gap-3 mb-3">
                  <h2 className="text-base font-semibold text-white/80">📱 设备与浏览器</h2>
                  <div className="flex items-center gap-2">
                    {([7, 30, 0]).map((d) => (
                      <button
                        key={d}
                        onClick={() => setDeviceDays(d)}
                        className={`rounded-lg px-2.5 py-1 text-xs transition ${
                          deviceDays === d
                            ? 'bg-white/10 text-white'
                            : 'text-white/40 hover:text-white/70'
                        }`}
                      >
                        {d === 0 ? '全部' : `${d}日`}
                      </button>
                    ))}
                  </div>
                </div>

                <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                  {/* Device Type */}
                  <div className="rounded-xl border border-white/8 bg-[#15171e] p-4">
                    <div className="text-xs text-white/40 mb-3 text-center">设备类型</div>
                    {deviceAnalytics.device_types && deviceAnalytics.device_types.length > 0 ? (
                      <>
                        <DoughnutChart data={deviceAnalytics.device_types} />
                        <CategoryLegend data={deviceAnalytics.device_types} />
                      </>
                    ) : (
                      <p className="text-xs text-white/25 text-center py-4">暂无数据</p>
                    )}
                  </div>

                  {/* OS */}
                  <div className="rounded-xl border border-white/8 bg-[#15171e] p-4">
                    <div className="text-xs text-white/40 mb-3 text-center">操作系统</div>
                    {deviceAnalytics.os_families && deviceAnalytics.os_families.length > 0 ? (
                      <>
                        <DoughnutChart data={deviceAnalytics.os_families} />
                        <CategoryLegend data={deviceAnalytics.os_families} />
                      </>
                    ) : (
                      <p className="text-xs text-white/25 text-center py-4">暂无数据</p>
                    )}
                  </div>

                  {/* Browser */}
                  <div className="rounded-xl border border-white/8 bg-[#15171e] p-4">
                    <div className="text-xs text-white/40 mb-3 text-center">浏览器</div>
                    {deviceAnalytics.browser_families && deviceAnalytics.browser_families.length > 0 ? (
                      <>
                        <DoughnutChart data={deviceAnalytics.browser_families} />
                        <CategoryLegend data={deviceAnalytics.browser_families} />
                      </>
                    ) : (
                      <p className="text-xs text-white/25 text-center py-4">暂无数据</p>
                    )}
                  </div>
                </div>

                {/* Top Active Users */}
                {deviceAnalytics.top_active_users && deviceAnalytics.top_active_users.length > 0 && (
                  <div className="mt-4 rounded-xl border border-white/8 bg-[#15171e] p-4">
                    <div className="text-xs text-white/40 mb-3">最活跃用户浏览器偏好（Top {deviceAnalytics.top_active_users.length}）</div>
                    <div className="overflow-x-auto">
                      <table className="w-full text-xs text-left">
                        <thead>
                          <tr className="border-b border-white/8 text-white/35">
                            <th className="pb-2 pr-4 font-medium">用户邮箱</th>
                            <th className="pb-2 pr-4 font-medium">活跃天数</th>
                            <th className="pb-2 pr-4 font-medium">最后活跃</th>
                            <th className="pb-2 pr-4 font-medium">浏览器</th>
                            <th className="pb-2 font-medium">操作系统</th>
                          </tr>
                        </thead>
                        <tbody className="text-white/65">
                          {deviceAnalytics.top_active_users.map((u, i) => (
                            <tr key={`${u.user_id}-${i}`} className="border-b border-white/[0.04] last:border-0">
                              <td className="py-1.5 pr-4 text-white/70">{u.email || u.user_id?.slice(0, 12) || '-'}</td>
                              <td className="py-1.5 pr-4 tabular-nums">{u.active_days} 天</td>
                              <td className="py-1.5 pr-4 text-white/40 whitespace-nowrap">{u.last_active_at ? formatTimeAgo(u.last_active_at) : '-'}</td>
                              <td className="py-1.5 pr-4">{u.browser || 'unknown'}</td>
                              <td className="py-1.5">{u.os || 'unknown'}</td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  </div>
                )}
              </section>
            )}

            {/* Panel 7: Analytics (PV/UV) */}
            {analytics && (
              <section>
                <h2 className="text-base font-semibold text-white/80 mb-3">🌐 访问统计</h2>
                <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-6 gap-3">
                  <StatCard label="今日 PV" value={analytics.today_pv} />
                  <StatCard label="今日 UV" value={analytics.today_uv} />
                  <StatCard label="7天 PV" value={analytics.week_pv} />
                  <StatCard label="7天 UV" value={analytics.week_uv} />
                  <StatCard label="30天 PV" value={analytics.month_pv} />
                  <StatCard label="30天 UV" value={analytics.month_uv} />
                </div>
                <div className="mt-4 grid grid-cols-1 md:grid-cols-2 gap-4">
                  <div className="rounded-xl border border-white/8 bg-[#15171e] p-3">
                    <MiniChart data={analytics.daily_pv} label="每日 PV" width={380} height={130} color="#a78bfa" />
                  </div>
                  <div className="rounded-xl border border-white/8 bg-[#15171e] p-3">
                    <MiniChart data={analytics.daily_uv} label="每日 UV" width={380} height={130} color="#34d399" />
                  </div>
                </div>
                {/* Top pages */}
                {analytics.top_pages && analytics.top_pages.length > 0 && (
                  <div className="mt-4 rounded-xl border border-white/8 bg-[#15171e] p-4">
                    <div className="text-xs text-white/40 mb-3">页面访问排行（30天）</div>
                    <div className="space-y-2">
                      {analytics.top_pages.map((p, i) => {
                        const maxCount = analytics.top_pages[0]?.count || 1
                        const pct = (p.count / maxCount) * 100
                        return (
                          <div key={i} className="flex items-center gap-3 text-sm">
                            <span className="w-28 truncate text-white/60 text-xs">{p.page_path}</span>
                            <div className="flex-1 h-4 rounded bg-white/5 overflow-hidden">
                              <div className="h-full rounded bg-primary/30" style={{ width: `${pct}%` }} />
                            </div>
                            <span className="text-xs text-white/50 tabular-nums w-10 text-right">{p.count}</span>
                          </div>
                        )
                      })}
                    </div>
                  </div>
                )}
                {/* Device breakdown */}
                {analytics.devices && (analytics.devices.desktop + analytics.devices.mobile + analytics.devices.tablet > 0) && (
                  <div className="mt-3 grid grid-cols-3 gap-3">
                    <StatCard label="桌面端" value={analytics.devices.desktop} />
                    <StatCard label="移动端" value={analytics.devices.mobile} />
                    <StatCard label="平板" value={analytics.devices.tablet} />
                  </div>
                )}
              </section>
            )}

            {/* Panel 7: Traffic Sources */}
            {stats.traffic && (
              <section>
                <h2 className="text-base font-semibold text-white/80 mb-3">🌍 流量来源</h2>
                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  {/* UTM Source breakdown (user registration source) */}
                  <div className="rounded-xl border border-white/8 bg-[#15171e] p-4">
                    <div className="text-xs text-white/40 mb-3">注册来源（UTM Source）</div>
                    {(stats.traffic.utm_sources || []).length === 0 ? (
                      <p className="text-xs text-white/25">暂无数据（推广链接加 ?utm_source=xxx 即可追踪）</p>
                    ) : (
                      <div className="space-y-2">
                        {stats.traffic.utm_sources.map((s, i) => {
                          const maxCount = stats.traffic.utm_sources[0]?.count || 1
                          const pct = (s.count / maxCount) * 100
                          return (
                            <div key={i} className="flex items-center gap-3 text-sm">
                              <span className="w-24 truncate text-white/60 text-xs">{s.source}</span>
                              <div className="flex-1 h-4 rounded bg-white/5 overflow-hidden">
                                <div className="h-full rounded bg-emerald-500/30" style={{ width: `${pct}%` }} />
                              </div>
                              <span className="text-xs text-white/50 tabular-nums w-8 text-right">{s.count}</span>
                            </div>
                          )
                        })}
                      </div>
                    )}
                  </div>
                  {/* Referrer breakdown (pageview referrer) */}
                  <div className="rounded-xl border border-white/8 bg-[#15171e] p-4">
                    <div className="text-xs text-white/40 mb-3">访问来源（Referrer · 30天）</div>
                    {(stats.traffic.referrers || []).length === 0 ? (
                      <p className="text-xs text-white/25">暂无数据</p>
                    ) : (
                      <div className="space-y-2">
                        {stats.traffic.referrers.slice(0, 10).map((s, i) => {
                          const maxCount = stats.traffic.referrers[0]?.count || 1
                          const pct = (s.count / maxCount) * 100
                          // Try to extract domain from referrer URL
                          let label = s.source
                          try { label = new URL(s.source).hostname } catch {}
                          return (
                            <div key={i} className="flex items-center gap-3 text-sm">
                              <span className="w-32 truncate text-white/60 text-xs" title={s.source}>{label}</span>
                              <div className="flex-1 h-4 rounded bg-white/5 overflow-hidden">
                                <div className="h-full rounded bg-blue-500/30" style={{ width: `${pct}%` }} />
                              </div>
                              <span className="text-xs text-white/50 tabular-nums w-8 text-right">{s.count}</span>
                            </div>
                          )
                        })}
                      </div>
                    )}
                  </div>
                </div>
              </section>
            )}

            {/* Panel 8: Quadrant Overview + Compute History (enhanced) */}
            <QuadrantAdminPanel />

            {/* Panel 9: User Feedback */}
            <FeedbackPanel />

            {/* Panel 10: Audit */}
            <section>
              <h2 className="text-base font-semibold text-white/80 mb-3">🛡️ 审计日志</h2>
              <div className="grid grid-cols-2 sm:grid-cols-3 gap-3">
                <StatCard label="今日登录次数" value={stats.audit.today_logins} />
                <StatCard label="今日注册次数" value={stats.audit.today_registrations} />
                <StatCard label="7天登录失败" value={stats.audit.failed_logins_7d} />
              </div>
            </section>

            {/* Panel 12: System Health (Error Monitoring) */}
            <SystemHealthPanel />

            {/* Panel 13: User Funnel */}
            <UserFunnelPanel />

            {/* Panel 14: 数据备份 */}
            <BackupPanel />

            {/* Panel 11: AI 模型配置 */}
            <AIProviderConfigPanel onUnauthorized={onLogout} />

            {/* Panel 12: AI 调用统计 */}
            {stats.ai && (
              <section>
                <h2 className="text-base font-semibold text-white/80 mb-3">🤖 AI 调用统计</h2>

                <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-6 gap-3">
                  <StatCard label="总调用量" value={stats.ai.total_calls} />
                  <StatCard label="今日调用" value={stats.ai.today_calls} />
                  <StatCard label="近7天调用" value={stats.ai.last_7d_calls} />
                  <RateCard label="成功率" value={stats.ai.success_rate} />
                  <StatCard
                    label="平均响应(ms)"
                    value={stats.ai.avg_response_ms != null ? Math.round(stats.ai.avg_response_ms) : '--'}
                    sub="越低越好"
                  />
                  <StatCard label="使用用户数" value={stats.ai.unique_users} />
                </div>

                <div className="mt-4 grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-6 gap-3">
                  <StatCard label="累计 Token" value={fmt(stats.ai.total_tokens)} />
                  <StatCard label="今日 Token" value={fmt(stats.ai.today_tokens)} />
                  <StatCard label="近7天 Token" value={fmt(stats.ai.last_7d_tokens)} />
                  <StatCard
                    label="平均每次 Token"
                    value={stats.ai.avg_tokens_per_call != null ? fmt(Math.round(stats.ai.avg_tokens_per_call)) : '--'}
                    sub="总 Token / 总调用"
                  />
                  <StatCard label="近30天输入 Token" value={fmt(aiUsage?.summary?.prompt_tokens)} />
                  <StatCard label="近30天输出 Token" value={fmt(aiUsage?.summary?.completion_tokens)} />
                </div>

                <div className="mt-3 rounded-xl border border-amber-400/15 bg-amber-500/[0.06] px-4 py-3 text-xs leading-6 text-amber-100/85">
                  当前 token 面板基于 `ai_call_logs` 的真实 usage 字段聚合；旧日志或 provider 未返回 usage 的请求会记为 0，所以这部分数据会从本次版本上线后逐步变完整。
                </div>

                <div className="mt-4 grid grid-cols-1 xl:grid-cols-2 gap-4">
                  {/* 调用次数按功能分布 */}
                  {stats.ai.by_feature && stats.ai.by_feature.length > 0 && (
                    <div className="rounded-xl border border-white/8 bg-[#15171e] p-4">
                      <div className="text-xs text-white/40 mb-3">按功能分布（调用次数）</div>
                      <div className="space-y-2">
                        {stats.ai.by_feature.map((f) => {
                          const maxCount = stats.ai.by_feature[0]?.count || 1
                          const pct = (f.count / maxCount) * 100
                          return (
                            <div key={f.feature_key} className="flex items-center gap-3 text-sm">
                              <span className="w-28 truncate text-white/60 text-xs">{f.feature_name}</span>
                              <div className="flex-1 h-4 rounded bg-white/5 overflow-hidden">
                                <div className="h-full rounded bg-violet-500/40" style={{ width: `${pct}%` }} />
                              </div>
                              <span className="text-xs text-white/50 tabular-nums w-12 text-right">{fmt(f.count)}</span>
                            </div>
                          )
                        })}
                      </div>
                    </div>
                  )}

                  {/* Token 按功能分布 */}
                  {stats.ai.by_feature_tokens && stats.ai.by_feature_tokens.length > 0 && (
                    <div className="rounded-xl border border-white/8 bg-[#15171e] p-4">
                      <div className="text-xs text-white/40 mb-3">按功能分布（Token 消耗）</div>
                      <div className="space-y-2">
                        {stats.ai.by_feature_tokens.map((f) => {
                          const maxTokens = stats.ai.by_feature_tokens[0]?.total_tokens || 1
                          const pct = (f.total_tokens / maxTokens) * 100
                          return (
                            <div key={f.feature_key} className="flex items-center gap-3 text-sm">
                              <span className="w-28 truncate text-white/60 text-xs">{f.feature_name}</span>
                              <div className="flex-1 h-4 rounded bg-white/5 overflow-hidden">
                                <div className="h-full rounded bg-fuchsia-500/45" style={{ width: `${pct}%` }} />
                              </div>
                              <span className="text-xs text-fuchsia-200/80 tabular-nums w-20 text-right">{fmt(f.total_tokens)}</span>
                            </div>
                          )
                        })}
                      </div>
                    </div>
                  )}
                </div>

                <div className="mt-4 grid grid-cols-1 xl:grid-cols-2 gap-4">
                  {stats.ai.daily_trend && stats.ai.daily_trend.length > 1 && (
                    <div className="rounded-xl border border-white/8 bg-[#15171e] p-3">
                      <MiniChart data={stats.ai.daily_trend} label="每日 AI 调用趋势（30天）" width={380} height={130} type="bar" color="#a78bfa" />
                    </div>
                  )}
                  {stats.ai.daily_token_trend && stats.ai.daily_token_trend.length > 1 && (
                    <div className="rounded-xl border border-white/8 bg-[#15171e] p-3">
                      <MiniChart data={stats.ai.daily_token_trend} label="每日 Token 用量（30天）" width={380} height={130} type="bar" color="#ec4899" />
                    </div>
                  )}
                </div>

                <div className="mt-4 grid grid-cols-1 xl:grid-cols-2 gap-4">
                  {/* TOP 调用用户 */}
                  {stats.ai.top_users && stats.ai.top_users.length > 0 && (
                    <div className="rounded-xl border border-white/8 bg-[#15171e] p-4">
                      <div className="text-xs text-white/40 mb-3">TOP 调用用户（前 10）</div>
                      <div className="overflow-x-auto">
                        <table className="w-full text-xs text-left">
                          <thead>
                            <tr className="border-b border-white/8 text-white/35">
                              <th className="pb-2 pr-4 font-medium">排名</th>
                              <th className="pb-2 pr-4 font-medium">用户</th>
                              <th className="pb-2 pr-4 font-medium text-right">调用次数</th>
                              <th className="pb-2 font-medium text-right">最近一次</th>
                            </tr>
                          </thead>
                          <tbody className="text-white/65">
                            {stats.ai.top_users.map((u, i) => (
                              <tr key={`${u.user_id}-${i}`} className="border-b border-white/[0.04] last:border-0">
                                <td className="py-1.5 pr-4 tabular-nums text-white/40">{i + 1}</td>
                                <td className="py-1.5 pr-4">
                                  <div className="max-w-[180px] truncate" title={u.email || u.user_id}>{formatUserDisplay(u)}</div>
                                  <div className="text-[10px] text-white/25 font-mono">{u.user_id?.slice(0, 12)}…</div>
                                </td>
                                <td className="py-1.5 pr-4 text-right tabular-nums font-medium text-violet-300">{fmt(u.call_count)}</td>
                                <td className="py-1.5 text-right text-white/30 whitespace-nowrap">
                                  {u.last_called_at ? new Date(u.last_called_at).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }) : '-'}
                                </td>
                              </tr>
                            ))}
                          </tbody>
                        </table>
                      </div>
                    </div>
                  )}

                  {/* TOP Token 用户 */}
                  {stats.ai.top_token_users && stats.ai.top_token_users.length > 0 && (
                    <div className="rounded-xl border border-white/8 bg-[#15171e] p-4">
                      <div className="text-xs text-white/40 mb-3">TOP Token 用户（前 10）</div>
                      <div className="overflow-x-auto">
                        <table className="w-full text-xs text-left">
                          <thead>
                            <tr className="border-b border-white/8 text-white/35">
                              <th className="pb-2 pr-4 font-medium">排名</th>
                              <th className="pb-2 pr-4 font-medium">用户</th>
                              <th className="pb-2 pr-4 font-medium text-right">总 Token</th>
                              <th className="pb-2 pr-4 font-medium text-right">调用</th>
                              <th className="pb-2 font-medium text-right">最近一次</th>
                            </tr>
                          </thead>
                          <tbody className="text-white/65">
                            {stats.ai.top_token_users.map((u, i) => (
                              <tr key={`${u.user_id}-${i}`} className="border-b border-white/[0.04] last:border-0">
                                <td className="py-1.5 pr-4 tabular-nums text-white/40">{i + 1}</td>
                                <td className="py-1.5 pr-4">
                                  <div className="max-w-[180px] truncate" title={u.email || u.user_id}>{formatUserDisplay(u)}</div>
                                  <div className="text-[10px] text-white/25 font-mono">{u.user_id?.slice(0, 12)}…</div>
                                </td>
                                <td className="py-1.5 pr-4 text-right tabular-nums font-medium text-fuchsia-300">{fmt(u.total_tokens)}</td>
                                <td className="py-1.5 pr-4 text-right tabular-nums text-white/45">{fmt(u.call_count)}</td>
                                <td className="py-1.5 text-right text-white/30 whitespace-nowrap">
                                  {u.last_called_at ? new Date(u.last_called_at).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }) : '-'}
                                </td>
                              </tr>
                            ))}
                          </tbody>
                        </table>
                      </div>
                    </div>
                  )}
                </div>

                {/* 每日每用户 token 明细 */}
                {aiUsage?.daily_users && aiUsage.daily_users.length > 0 && (
                  <div className="mt-4 rounded-xl border border-white/8 bg-[#15171e] p-4">
                    <div className="flex items-center justify-between gap-3 mb-3">
                      <div>
                        <div className="text-xs text-white/40">每日每用户 Token 用量（近 {aiUsage.days || 30} 天）</div>
                        <div className="mt-1 text-[11px] text-white/25">按日期倒序、当日 Token 从高到低排序，便于快速识别高消耗用户。</div>
                      </div>
                      <div className="text-[11px] text-white/30">共 {fmt(aiUsage.daily_users.length)} 条聚合记录</div>
                    </div>
                    <div className="overflow-x-auto">
                      <table className="w-full text-xs text-left">
                        <thead>
                          <tr className="border-b border-white/8 text-white/35">
                            <th className="pb-2 pr-4 font-medium">日期</th>
                            <th className="pb-2 pr-4 font-medium">用户</th>
                            <th className="pb-2 pr-4 font-medium text-right">总 Token</th>
                            <th className="pb-2 pr-4 font-medium text-right">输入</th>
                            <th className="pb-2 pr-4 font-medium text-right">输出</th>
                            <th className="pb-2 pr-4 font-medium text-right">调用</th>
                            <th className="pb-2 font-medium text-right">最后一次</th>
                          </tr>
                        </thead>
                        <tbody className="text-white/65">
                          {aiUsage.daily_users.map((row, i) => (
                            <tr key={`${row.date}-${row.user_id}-${i}`} className="border-b border-white/[0.04] last:border-0 hover:bg-white/[0.02]">
                              <td className="py-2 pr-4 whitespace-nowrap text-white/40 tabular-nums">{row.date}</td>
                              <td className="py-2 pr-4">
                                <div className="max-w-[220px] truncate" title={row.email || row.user_id}>{formatUserDisplay(row)}</div>
                                <div className="text-[10px] text-white/25 font-mono">{row.user_id?.slice(0, 12)}…</div>
                              </td>
                              <td className="py-2 pr-4 text-right tabular-nums font-medium text-fuchsia-300">{fmt(row.total_tokens)}</td>
                              <td className="py-2 pr-4 text-right tabular-nums text-white/45">{fmt(row.prompt_tokens)}</td>
                              <td className="py-2 pr-4 text-right tabular-nums text-white/45">{fmt(row.completion_tokens)}</td>
                              <td className="py-2 pr-4 text-right tabular-nums text-white/45">{fmt(row.call_count)}</td>
                              <td className="py-2 text-right text-white/30 whitespace-nowrap">
                                {row.last_called_at ? new Date(row.last_called_at).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }) : '-'}
                              </td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  </div>
                )}
              </section>
            )}
          </>
        ) : null}
      </main>
    </div>
  )
}

// ── Quadrant Overview + Compute Logs (Panel 8 enhanced) ──

const QUADRANT_LABELS = {
  opportunity_zone: { label: '机会', color: 'text-emerald-400 bg-emerald-500/10 border-emerald-400/25' },
  crowded_zone: { label: '拥挤', color: 'text-amber-400 bg-amber-500/10 border-amber-400/25' },
  bubble_zone: { label: '泡沫', color: 'text-rose-400 bg-rose-500/10 border-rose-400/25' },
  defensive_zone: { label: '防御', color: 'text-white/50 bg-white/5 border-white/10' },
  neutral_zone: { label: '中性', color: 'text-blue-400 bg-blue-500/10 border-blue-400/25' },
}

function formatLastComputed(s) {
  if (!s) return '--'
  try {
    const d = new Date(s)
    const diff = Math.floor((Date.now() - d.getTime()) / 3600000)
    if (diff < 1) return '刚刚'
    if (diff < 24) return `${diff}小时前`
    return d.toLocaleDateString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })
  } catch { return '--' }
}

function formatTimeAgo(s) {
  if (!s) return ''
  try {
    const d = new Date(s)
    const diffSec = Math.floor((Date.now() - d.getTime()) / 1000)
    if (diffSec < 10) return '刚刚'
    if (diffSec < 60) return `${diffSec}秒前`
    const diffMin = Math.floor(diffSec / 60)
    if (diffMin < 60) return `${diffMin}分钟前`
    return `${Math.floor(diffMin / 60)}小时前`
  } catch { return '' }
}

function CompanyProfilesAdminPanel() {
  const [data, setData] = useState(null)
  const [loading, setLoading] = useState(false)
  const [notice, setNotice] = useState('')
  const [error, setError] = useState('')

  const loadData = useCallback(async () => {
    try {
      const d = await adminFetch('/api/admin/company-profiles')
      setData(d)
      setError('')
    } catch (err) {
      setError(err.message || '加载公司资料统计失败')
    }
  }, [])

  useEffect(() => {
    loadData()
    const timer = setInterval(loadData, 15000)
    return () => clearInterval(timer)
  }, [loadData])

  const triggerRefresh = async () => {
    setLoading(true)
    setNotice('')
    setError('')
    try {
      await adminFetch('/api/admin/company-profiles/refresh', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ exchange: 'ALL' }),
      })
      setNotice('已开始刷新公司静态资料，面板会自动更新进度。')
      await loadData()
    } catch (err) {
      setError(err.message || '触发刷新失败')
    } finally {
      setLoading(false)
    }
  }

  const refresh = data?.refresh || {}
  return (
    <section>
      <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-base font-semibold text-white/80">🏢 公司资料管理</h2>
          <p className="mt-1 text-xs text-white/40">覆盖率、失败项与手动刷新。刷新会从 Quant 自动采集并写入本地库。</p>
        </div>
        <button
          type="button"
          disabled={loading || refresh.running}
          onClick={triggerRefresh}
          className="rounded-lg bg-primary px-4 py-2 text-xs font-semibold text-white transition hover:bg-primary/85 disabled:cursor-not-allowed disabled:opacity-50"
        >
          {refresh.running ? '刷新中...' : loading ? '触发中...' : '一键更新静态资料'}
        </button>
      </div>
      {notice && <div className="mb-3 rounded-xl border border-emerald-400/20 bg-emerald-500/10 px-4 py-2 text-xs text-emerald-200">{notice}</div>}
      {error && <div className="mb-3 rounded-xl border border-rose-400/20 bg-rose-500/10 px-4 py-2 text-xs text-rose-200">{error}</div>}
      {refresh.error && <div className="mb-3 rounded-xl border border-rose-400/20 bg-rose-500/10 px-4 py-2 text-xs leading-5 text-rose-200">刷新失败：{refresh.error}</div>}
      {refresh.message && !refresh.error && <div className="mb-3 rounded-xl border border-white/10 bg-white/[0.04] px-4 py-2 text-xs text-white/55">{refresh.message}</div>}
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6">
        {(data?.coverage || []).map((row) => (
          <StatCard key={row.exchange} label={`${row.exchange}覆盖`} value={`${Math.round((row.coverage_rate || 0) * 100)}%`} sub={`${row.profile_count || 0}/${row.universe_count || 0}`} />
        ))}
        <StatCard label="刷新状态" value={refresh.status || 'idle'} sub={refresh.running ? `进度 ${refresh.success_count || 0}/${refresh.total_count || 0}` : (refresh.finished_at ? `完成 ${formatAdminDateTime(refresh.finished_at)}` : '等待触发')} />
        <StatCard label="新股发现" value={refresh.new_count || 0} />
        <StatCard label="退市标记" value={refresh.delisted_count || 0} />
      </div>
      <div className="mt-4 rounded-xl border border-white/8 bg-[#15171e] p-4">
        <div className="mb-3 text-xs text-white/40">失败项 / 待补全（最近 30 条）</div>
        {(data?.failures || []).length === 0 ? (
          <p className="text-xs text-white/25">暂无失败项。</p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs">
              <thead><tr className="border-b border-white/8 text-white/35"><th className="pb-2 pr-3">代码</th><th className="pb-2 pr-3">名称</th><th className="pb-2 pr-3">市场</th><th className="pb-2 pr-3">状态</th><th className="pb-2">标记</th></tr></thead>
              <tbody className="text-white/65">
                {data.failures.map((item) => (
                  <tr key={item.symbol} className="border-b border-white/[0.04] last:border-0"><td className="py-2 pr-3 font-mono">{item.symbol}</td><td className="py-2 pr-3">{item.name || '--'}</td><td className="py-2 pr-3">{item.exchange}</td><td className="py-2 pr-3">{item.profile_status}</td><td className="py-2">{(item.quality_flags || []).join(', ') || '--'}</td></tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </section>
  )
}

function QuadrantAdminPanel() {
  const [overview, setOverview] = useState(null)
  const [logs, setLogs] = useState(null)
  const [expandedLog, setExpandedLog] = useState(null)
  const [progress, setProgress] = useState(null)          // { ASHARE: {...}, HKEX: {...} }
  const [triggering, setTriggering] = useState(false)

  // ── Progress polling ──
  useEffect(() => {
    let timer = null
    const fetchProgress = async () => {
      try {
        const data = await adminFetch('/api/admin/compute-status')
        const prevStatus = progress ? { ASHARE: progress.ASHARE?.status, HKEX: progress.HKEX?.status } : null

        setProgress(data)

        // Auto-refresh overview + logs on terminal state transition
        if (prevStatus && data) {
          for (const ex of ['ASHARE', 'HKEX']) {
            const wasRunning = prevStatus[ex] === 'running'
            const isTerminal = data[ex]?.status === 'success' || data[ex]?.status === 'failed' || data[ex]?.status === 'timeout'
            if (wasRunning && isTerminal) {
              refreshAll()
              break  // only need one refreshAll call
            }
          }
        }
      } catch { /* silent */ }
    }
    fetchProgress()
    // Auto-poll every 5s when any exchange is running
    timer = setInterval(() => {
      if (progress) {
        const anyRunning = Object.values(progress).some(p => p.status === 'running')
        if (anyRunning) fetchProgress()
      }
    }, 5000)
    return () => clearInterval(timer)
  }, [progress?.ASHARE?.status, progress?.HKEX?.status])

  const refreshAll = useCallback(async () => {
    try {
      const [ov, lg, pr] = await Promise.all([
        adminFetch('/api/admin/quadrant-overview').catch(() => null),
        adminFetch('/api/admin/quadrant-logs').then((d) => d.items || []).catch(() => []),
        adminFetch('/api/admin/compute-status').catch(() => null),
      ])
      setOverview(ov)
      setLogs(lg)
      if (pr) setProgress(pr)
    } catch { /* silent */ }
  }, [])

  // ── Load initial data ──
  useEffect(() => { refreshAll() }, [refreshAll])

  // ── Manual trigger ──
  const handleTrigger = async (exchange) => {
    setTriggering(true)
    try {
      await adminFetch('/api/admin/quadrant-trigger', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ exchange }),
      })
      // Immediately refresh progress
      const pr = await adminFetch('/api/admin/compute-status').catch(() => null)
      if (pr) setProgress(pr)
    } catch (err) {
      alert('触发失败: ' + err.message)
    } finally {
      setTriggering(false)
    }
  }

  // Helper: render a single exchange progress bar (defined before any conditional return — Rules of Hooks)
  const renderProgressBar = (exKey, label) => {
    const p = progress?.[exKey]
    if (!p) return null

    const isRunning = p.status === 'running'
    const isSuccess = p.status === 'success'
    const isFailed = p.status === 'failed'
    const isTimeout = p.status === 'timeout'
    const pct = Math.min(p.percent || 0, 100).toFixed(1)
    const statusIcon = isSuccess ? '✅' : isFailed ? '❌' : isTimeout ? '⏰' : isRunning ? '🔄' : '💤'
    const statusLabel = isSuccess ? '已完成' : isFailed ? '失败' : isTimeout ? '超时' : isRunning ? '计算中...' : '空闲'
    const elapsed = p.updated_at ? formatTimeAgo(p.updated_at) : ''
    const barColor = isSuccess ? 'bg-emerald-500' : isFailed ? 'bg-rose-500' : isTimeout ? 'bg-amber-500' : 'bg-blue-500'
    const barPulse = isRunning ? 'animate-pulse' : ''

    return (
      <div className="rounded-xl border border-white/8 bg-[#15171e] p-4">
        <div className="flex items-center justify-between mb-2">
          <span className="text-sm font-semibold text-white/80">{label} 四象限</span>
          <span className="flex items-center gap-1.5 text-xs font-medium">
            <span>{statusIcon}</span>
            <span className={isSuccess ? 'text-emerald-400' : isFailed ? 'text-rose-400' : isTimeout ? 'text-amber-400' : isRunning ? 'text-blue-400' : 'text-white/40'}>
              {statusLabel}
            </span>
          </span>
        </div>
        {/* Progress bar */}
        <div className="w-full h-2 bg-white/8 rounded-full overflow-hidden mb-2">
          <div
            className={`h-full rounded-full transition-all duration-700 ease-out ${barColor} ${barPulse}`}
            style={{ width: `${isRunning ? Math.max(pct, 2) : (isSuccess ? 100 : 0)}%` }}
          />
        </div>
        {/* 阶段消息（running + 有 message 时显示） */}
        {isRunning && p.message && (
          <div className="text-[11px] text-blue-300/70 mb-1 truncate" title={p.message}>
            {p.message}
          </div>
        )}
        <div className="flex items-center justify-between text-[11px] text-white/35">
          <span>
            {isRunning && p.total > 0 ? `${p.current.toLocaleString()} / ${p.total.toLocaleString()} (${pct}%)` :
             isRunning && !p.message ? '准备中...' :
             isSuccess && p.total > 0 ? `${p.total.toLocaleString()} 只 · 已落库` :
             isFailed ? (p.error_msg || '数据未写入后端（回调失败）') :
             isTimeout ? '计算超时' :
             '--'}
          </span>
          <span>{elapsed}</span>
        </div>
      </div>
    )
  }

  if (!overview && !logs) return null

  return (
    <section>
      <h2 className="text-base font-semibold text-white/80 mb-3">🔲 四象限数据总览</h2>

      {/* ════════════════ PROGRESS PANEL ════════════════ */}
      {progress && (
        <div className="mb-5 space-y-3">
          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
            {renderProgressBar('ASHARE', 'A 股')}
            {renderProgressBar('HKEX', '港股')}
          </div>
          <div className="flex items-center gap-3">
            <button
              onClick={() => handleTrigger('ASHARE')}
              disabled={triggering || progress.ASHARE?.status === 'running'}
              className="px-3 py-1.5 rounded-lg text-xs font-medium bg-blue-600 hover:bg-blue-500 disabled:bg-blue-900/50 disabled:text-white/30 text-white transition cursor-pointer disabled:cursor-not-allowed"
            >
              🔄 立即计算 A 股
            </button>
            <button
              onClick={() => handleTrigger('HKEX')}
              disabled={triggering || progress.HKEX?.status === 'running'}
              className="px-3 py-1.5 rounded-lg text-xs font-medium bg-purple-600 hover:bg-purple-500 disabled:bg-purple-900/50 disabled:text-white/30 text-white transition cursor-pointer disabled:cursor-not-allowed"
            >
              🔄 立即计算港股
            </button>
            <button
              onClick={refreshAll}
              className="ml-auto px-3 py-1.5 rounded-lg text-xs font-medium border border-white/10 hover:border-white/20 text-white/45 hover:text-white/65 transition"
            >
              刷新
            </button>
          </div>
        </div>
      )}

      {/* Overview Cards */}
      {overview && (
        <>
          {/* Exchange summary cards */}
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 mb-4">
            <StatCard label="A 股总数" value={overview.exchanges?.[0]?.total_count ?? '--'} sub={formatLastComputed(overview.exchanges?.[0]?.last_computed)} />
            <StatCard label="港股总数" value={overview.exchanges?.[1]?.total_count ?? '--'} sub={formatLastComputed(overview.exchanges?.[1]?.last_computed)} />
            <StatCard label="合计股票" value={overview.grand_total} />
            <StatCard label="最后更新" value={formatLastComputed(
              (overview.exchanges?.[0]?.last_computed || '') > (overview.exchanges?.[1]?.last_computed || '')
                ? overview.exchanges?.[0]?.last_computed
                : overview.exchanges?.[1]?.last_computed
            )} />
          </div>

          {/* Per-exchange quadrant breakdown */}
          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
            {overview.exchanges.filter(e => e.total_count > 0).map(ex => (
              <div key={ex.exchange} className="rounded-xl border border-white/8 bg-[#15171e] p-4">
                <div className="flex items-center justify-between mb-3">
                  <span className="text-sm font-semibold text-white/80">{ex.exchange} 象限分布</span>
                  <span className="text-xs text-white/35">{ex.total_count.toLocaleString()} 只</span>
                </div>
                <div className="flex flex-wrap gap-2">
                  {[
                    ['opportunity_zone', ex.summary.opportunity_zone],
                    ['crowded_zone', ex.summary.crowded_zone],
                    ['bubble_zone', ex.summary.bubble_zone],
                    ['defensive_zone', ex.summary.defensive_zone],
                    ['neutral_zone', ex.summary.neutral_zone],
                  ].map(([key, count]) => {
                    const q = QUADRANT_LABELS[key]
                    const total = ex.total_count || 1
                    return (
                      <span key={key} className={`inline-flex items-center gap-1 rounded-md border px-2 py-0.5 text-[11px] font-medium ${q?.color}`}>
                        {q?.label}{count}
                        <span className="text-white/25 ml-0.5">{Math.round(count / total * 100)}%</span>
                      </span>
                    )
                  })}
                </div>
              </div>
            ))}
          </div>
        </>
      )}

      {/* Compute Logs */}
      <div className="mt-5">
        <div className="flex items-center justify-between mb-3">
          <h3 className="text-sm font-medium text-white/55">计算历史</h3>
          {!logs && <span className="text-xs text-white/25">加载中…</span>}
        </div>
        {!logs || logs.length === 0 ? (
          <p className="text-xs text-white/30">暂无计算记录</p>
        ) : (
          <div className="space-y-1.5">
            {logs.slice(0, 15).map((log) => {
              const report = (() => { try { return JSON.parse(log.ReportJSON || '{}') } catch { return {} } })()
              const isExp = expandedLog === log.ID
              const statusColor = log.Status === 'success' ? 'text-emerald-400' : log.Status === 'failed' ? 'text-rose-400' : 'text-amber-400'
              const qc = report.quadrant_counts || {}
              return (
                <div key={log.ID} className="rounded-lg border border-white/6 bg-[#15171e]/70 px-3 py-2">
                  <div
                    className="flex flex-wrap items-center gap-x-4 gap-y-1 text-[11px] cursor-pointer hover:bg-white/[0.02] rounded transition"
                    onClick={() => setExpandedLog(isExp ? null : log.ID)}
                  >
                    <span className="text-white/40 tabular-nums">{new Date(log.ComputedAt).toLocaleString('zh-CN')}</span>
                    <span className={`font-medium ${statusColor}`}>{log.Status}</span>
                    <span className="text-white/30">{log.Mode}</span>
                    <span className="text-white/30">{log.StockCount} 只</span>
                    <span className="text-white/30">{log.DurationSec.toFixed(0)}s</span>
                    {Object.keys(qc).length > 0 && (
                      <span className="text-white/20 hidden sm:inline">
                        机:{qc['机会']||0}/挤:{qc['拥挤']||0}/泡:{qc['泡沫']||0}/防:{qc['防御']||0}/中:{qc['中性']||0}
                      </span>
                    )}
                    <span className="ml-auto text-white/25">{isExp ? '▼' : '▶'}</span>
                  </div>
                  {isExp && (
                    <pre className="mt-2 max-h-56 overflow-auto rounded bg-black/40 p-2 text-[10px] leading-relaxed text-white/45 font-mono">
                      {JSON.stringify(report, null, 2)}
                    </pre>
                  )}
                </div>
              )
            })}
          </div>
        )}
      </div>
    </section>
  )
}

// ── Feedback Panel ──

const FB_CATEGORY_LABELS = { bug: '🐛 Bug', feature: '💡 功能建议', wish: '🌟 许愿池' }
const FB_STATUS_LABELS = { pending: '待处理', resolved: '已处理', dismissed: '已忽略' }
const FB_STATUS_COLORS = { pending: 'text-amber-300 bg-amber-500/10 border-amber-400/30', resolved: 'text-emerald-300 bg-emerald-500/10 border-emerald-400/30', dismissed: 'text-white/40 bg-white/5 border-white/10' }

function FeedbackPanel() {
  const [data, setData] = useState(null)
  const [updating, setUpdating] = useState(null)

  useEffect(() => {
    adminFetch('/api/admin/feedback?limit=50')
      .then((d) => setData(d))
      .catch(() => setData({ items: [], total: 0, stats: null }))
  }, [])

  if (!data) return null

  const stats = data.stats
  const items = data.items || []

  const handleUpdateStatus = async (id, status) => {
    setUpdating(id)
    try {
      await adminFetch(`/api/admin/feedback/${id}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ status }),
      })
      // Refresh
      const refreshed = await adminFetch('/api/admin/feedback?limit=50')
      setData(refreshed)
    } catch { /* silent */ }
    setUpdating(null)
  }

  return (
    <section>
      <h2 className="text-base font-semibold text-white/80 mb-3">💬 用户反馈</h2>
      {stats ? (
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 mb-4">
          <StatCard label="总反馈" value={stats.total} />
          <StatCard label="待处理" value={stats.pending} />
          <StatCard label="Bug" value={stats.bug_count} />
          <StatCard label="建议+许愿" value={(stats.feature_count || 0) + (stats.wish_count || 0)} />
        </div>
      ) : null}
      {items.length === 0 ? (
        <p className="text-xs text-white/40">暂无用户反馈</p>
      ) : (
        <div className="space-y-2">
          {items.map((item) => (
            <div key={item.id} className="rounded-lg border border-white/8 bg-[#15171e] px-4 py-3">
              <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs">
                <span className="font-medium text-white/80">{FB_CATEGORY_LABELS[item.category] || item.category}</span>
                <span className={`inline-flex rounded-full border px-2 py-0.5 text-[10px] font-medium ${FB_STATUS_COLORS[item.status] || FB_STATUS_COLORS.pending}`}>
                  {FB_STATUS_LABELS[item.status] || item.status}
                </span>
                <span className="text-white/35">{new Date(item.created_at).toLocaleString('zh-CN')}</span>
                <span className="text-white/30">{item.user_email || item.user_id}</span>
              </div>
              <div className="mt-2 text-sm leading-7 text-white/70 whitespace-pre-wrap">{item.content}</div>
              {item.contact ? (
                <div className="mt-1 text-xs text-white/40">联系方式：{item.contact}</div>
              ) : null}
              {item.status === 'pending' ? (
                <div className="mt-2 flex gap-2">
                  <button
                    type="button"
                    disabled={updating === item.id}
                    onClick={() => handleUpdateStatus(item.id, 'resolved')}
                    className="rounded-lg border border-emerald-400/30 bg-emerald-500/10 px-2.5 py-1 text-[11px] font-medium text-emerald-300 transition hover:bg-emerald-500/20 disabled:opacity-50"
                  >
                    标记已处理
                  </button>
                  <button
                    type="button"
                    disabled={updating === item.id}
                    onClick={() => handleUpdateStatus(item.id, 'dismissed')}
                    className="rounded-lg border border-white/10 bg-white/5 px-2.5 py-1 text-[11px] font-medium text-white/50 transition hover:bg-white/10 disabled:opacity-50"
                  >
                    忽略
                  </button>
                </div>
              ) : null}
            </div>
          ))}
        </div>
      )}
    </section>
  )
}

// ── System Health Panel (Error Monitoring) ──

const STATUS_LABELS = { 400: 'Bad Request', 401: 'Unauthorized', 403: 'Forbidden', 404: 'Not Found', 409: 'Conflict', 429: 'Too Many Requests', 500: 'Internal Error', 502: 'Bad Gateway', 503: 'Service Unavailable', 504: 'Gateway Timeout' }

function statusColor(code) {
  if (code >= 500) return 'text-rose-400 bg-rose-500/10 border-rose-400/25'
  return 'text-amber-300 bg-amber-500/10 border-amber-400/25'
}

function formatMS(ms) {
  if (ms == null) return '--'
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(1)}s`
}

function SystemHealthPanel() {
  const [data, setData] = useState(null)
  const [loading, setLoading] = useState(true)
  const [logsExpanded, setLogsExpanded] = useState(false)
  const [logsData, setLogsData] = useState(null)

  useEffect(() => {
    adminFetch('/api/admin/system-health')
      .then((d) => { setData(d); setLoading(false) })
      .catch(() => { setLoading(false) })
  }, [])

  const loadMoreLogs = async () => {
    try {
      const d = await adminFetch('/api/admin/system-health/logs?limit=200&offset=0')
      setLogsData(d)
      setLogsExpanded(true)
    } catch { /* silent */ }
  }

  const handlePurge = async () => {
    if (!window.confirm('确定要清理历史错误日志吗？（保留最近 30 天）')) return
    try {
      await adminFetch('/api/admin/system-health/purge', { method: 'POST' })
      // Refresh
      const refreshed = await adminFetch('/api/admin/system-health')
      setData(refreshed)
      setLogsData(null)
      setLogsExpanded(false)
    } catch { /* silent */ }
  }

  if (loading && !data) return null

  const summary = data?.error_summary || {}
  const topEndpoints = data?.top_error_endpoints || []
  const recentErrors = data?.recent_errors || []
  const trends = data?.error_trends || []

  return (
    <section>
      <h2 className="text-base font-semibold text-white/80 mb-3">🖥️ 系统健康（错误监控）</h2>

      {/* Error Summary Cards */}
      <div className="grid grid-cols-2 sm:grid-cols-4 lg:grid-cols-6 gap-3">
        <StatCard label="今日错误总数" value={summary.today_total ?? '--'} />
        <StatCard label="客户端错误(4xx)" value={summary.client_errors ?? '--'} sub="请求参数/权限问题" />
        <StatCard label="服务端错误(5xx)" value={summary.server_errors ?? '--'} sub="系统内部错误" />
        <StatCard label="平均耗时" value={formatMS(summary.avg_duration)} sub="仅错误请求" />
        <StatCard label="Top 错误接口" value={topEndpoints.length || '--'} />
        <StatCard label="最新记录" value={recentErrors.length > 0 ? `${recentErrors[0]?.status_code}` : '--'} />
      </div>

      {/* Error Trend Chart + Top Endpoints */}
      {(trends.length > 1 || topEndpoints.length > 0) && (
        <div className="mt-4 grid grid-cols-1 md:grid-cols-2 gap-4">
          {/* Trend */}
          {trends.length > 1 && (
            <div className="rounded-xl border border-white/8 bg-[#15171e] p-3">
              <MiniChart data={trends} label="错误趋势（14天）" width={380} height={130} type="bar" color="#ef4444" />
            </div>
          )}
          {/* Top Error Endpoints */}
          {topEndpoints.length > 0 && (
            <div className="rounded-xl border border-white/8 bg-[#15171e] p-4">
              <div className="text-xs text-white/40 mb-3">Top 出错接口（今日）</div>
              <div className="space-y-2">
                {topEndpoints.slice(0, 8).map((ep, i) => (
                  <div key={`${ep.path}-${ep.method}`} className="flex items-center gap-3 text-sm">
                    <span className={`text-[11px] font-mono px-1.5 py-0.5 rounded border ${ep.count > 20 ? statusColor(500) : statusColor(400)}`}>
                      {ep.method}
                    </span>
                    <span className="w-44 truncate text-white/60 text-xs font-mono">{ep.path}</span>
                    <div className="flex-1 h-4 rounded bg-white/5 overflow-hidden">
                      <div
                        className="h-full rounded bg-rose-500/30"
                        style={{ width: `${Math.min((ep.count / (topEndpoints[0].count || 1)) * 100, 100)}%` }}
                      />
                    </div>
                    <span className="text-xs text-white/50 tabular-nums w-8 text-right">{ep.count}</span>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      )}

      {/* Recent Errors Table */}
      {recentErrors.length > 0 ? (
        <div className="mt-4 rounded-xl border border-white/8 bg-[#15171e] overflow-hidden">
          <div className="flex items-center justify-between px-4 py-2.5 border-b border-white/8">
            <div className="text-xs text-white/40">
              最近报错日志（{data?.generated_at ? `更新于 ${new Date(data.generated_at).toLocaleTimeString('zh-CN')}` : ''}）
            </div>
            <div className="flex gap-2">
              {!logsExpanded && (
                <button
                  type="button"
                  onClick={loadMoreLogs}
                  className="rounded-lg border border-white/12 bg-white/5 px-2.5 py-1 text-[11px] text-white/60 transition hover:bg-white/10 hover:text-white"
                >
                  展开全部
                </button>
              )}
              <button
                type="button"
                onClick={handlePurge}
                className="rounded-lg border border-rose-400/20 bg-rose-500/8 px-2.5 py-1 text-[11px] text-rose-300 transition hover:bg-rose-500/15"
              >
                清理旧数据
              </button>
            </div>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full text-xs text-left">
              <thead>
                <tr className="border-b border-white/[0.06] text-white/30">
                  <th className="py-2 pl-4 pr-3 font-medium">时间</th>
                  <th className="py-2 px-3 font-medium">方法</th>
                  <th className="py-2 px-3 font-medium">接口</th>
                  <th className="py-2 px-3 font-medium text-center">状态码</th>
                  <th className="py-2 px-3 font-medium">错误码</th>
                  <th className="py-2 px-3 font-medium">信息</th>
                  <th className="py-2 px-3 font-medium text-right">耗时</th>
                  <th className="py-2 pr-4 pl-3 font-medium text-right">IP</th>
                </tr>
              </thead>
              <tbody className="text-white/65">
                {(logsExpanded && logsData ? logsData.items : recentErrors).map((err) => (
                  <tr key={err.id} className="border-b border-white/[0.03] last:border-0 hover:bg-white/[0.02]">
                    <td className="py-1.5 pl-4 pr-3 whitespace-nowrap text-white/35">
                      {new Date(err.created_at).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })}
                    </td>
                    <td className="py-1.5 px-3">
                      <span className="font-mono text-[11px] text-white/45">{err.method}</span>
                    </td>
                    <td className="py-1.5 px-3 max-w-[220px]">
                      <span className="font-mono text-[11px] truncate block" title={err.path}>{err.path}</span>
                    </td>
                    <td className="py-1.5 px-3 text-center">
                      <span className={`inline-flex rounded-md border px-1.5 py-0.5 text-[11px] font-medium ${statusColor(err.status_code)}`}>
                        {err.status_code}
                      </span>
                    </td>
                    <td className="py-1.5 px-3 font-mono text-[11px] text-white/40">{err.error_code || '-'}</td>
                    <td className="py-1.5 px-3 max-w-[240px] truncate text-white/55" title={err.error_message}>{err.error_message || '-'}</td>
                    <td className="py-1.5 px-3 text-right tabular-nums text-white/35 whitespace-nowrap">{formatMS(err.duration_ms)}</td>
                    <td className="py-1.5 pr-4 pl-3 text-right text-white/25 font-mono text-[11px]" title={err.client_ip}>
                      {err.client_ip ? err.client_ip.split('.').slice(0, 2).join('.') + '.*' : '-'}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      ) : (
        <div className="mt-3 text-xs text-white/30 p-3 rounded-xl border border-dashed border-white/10 text-center">
          暂无错误记录 — 系统运行正常 ✅
        </div>
      )}
    </section>
  )
}

// ── User Funnel Panel (Conversion Funnel) ──

const FUNNEL_COLORS = [
  'from-blue-500 to-cyan-400',      // 访客
  'from-emerald-500 to-green-400',  // 注册
  'from-violet-500 to-purple-400',  // 登录
  'from-orange-500 to-amber-400',   // 关注池
  'from-teal-500 to-cyan-400',      // 持仓管理
  'from-pink-500 to-rose-400',      // 配置信号
  'from-indigo-500 to-blue-400',    // 跑回测
  'from-fuchsia-500 to-pink-400',   // 用 AI
]

function fmt(n) {
  if (n == null) return '--'
  if (n >= 1000000) return (n / 10000).toFixed(1) + '万'
  return n.toLocaleString()
}

function convRate(prev, curr) {
  if (!prev || prev === 0) return '--'
  return ((curr / prev) * 100).toFixed(1) + '%'
}

function UserFunnelPanel() {
  const [data, setData] = useState(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    adminFetch('/api/admin/user-funnel')
      .then((d) => { setData(d); setLoading(false) })
      .catch(() => { setLoading(false) })
  }, [])

  if (loading && !data) return null
  const steps = data?.steps || []
  if (steps.length === 0) return null

  const maxAll = Math.max(...steps.map(s => s.count_all), 1)

  return (
    <section>
      <h2 className="text-base font-semibold text-white/80 mb-3">📊 用户转化漏斗</h2>

      {/* Funnel Visualization */}
      <div className="rounded-xl border border-white/8 bg-[#15171e] p-5">
        <div className="flex flex-col gap-2">
          {steps.map((step, i) => {
            const w = Math.max((step.count_all / maxAll) * 100, i === 0 ? 4 : 2)
            const prev = i > 0 ? steps[i - 1].count_all : step.count_all
            return (
              <div key={step.label} className="flex items-center gap-3">
                {/* Label */}
                <div className="w-20 text-right text-xs font-medium text-white/60 shrink-0 pt-0.5">
                  {step.label}
                </div>
                {/* Bar */}
                <div className="flex-1 h-9 relative rounded-lg overflow-hidden bg-white/[0.04]">
                  <div
                    className={`h-full rounded-lg bg-gradient-to-r ${FUNNEL_COLORS[i]} transition-all duration-500 flex items-center justify-between px-3`}
                    style={{ width: `${w}%` }}
                  >
                    <span className="text-[11px] font-bold text-white/90 truncate drop-shadow-sm">
                      {fmt(step.count_all)}
                    </span>
                    <span className="text-[11px] font-medium text-white/70 tabular-nums">
                      {convRate(prev, step.count_all)}
                    </span>
                  </div>
                </div>
                {/* Time breakdown */}
                <div className="w-56 flex gap-2 shrink-0 text-[10px] text-white/35 tabular-nums">
                  <span title="今日">{fmt(step.count_today)}</span>
                  <span title="7天" className="text-white/25">7d:{fmt(step.count_7d)}</span>
                  <span title="30天" className="text-white/20">30d:{fmt(step.count_30d)}</span>
                </div>
              </div>
            )
          })}
        </div>

        {/* Summary table below funnel */}
        <div className="mt-5 overflow-x-auto">
          <table className="w-full text-xs text-left">
            <thead>
              <tr className="border-b border-white/[0.06] text-white/30">
                <th className="py-2 pl-3 font-medium">阶段</th>
                <th className="py-2 px-3 text-right font-medium">全部</th>
                <th className="py-2 px-3 text-right font-medium">今日</th>
                <th className="py-2 px-3 text-right font-medium">7 天</th>
                <th className="py-2 px-3 text-right font-medium">30 天</th>
                <th className="py-2 px-3 text-right font-medium">层转化率</th>
              </tr>
            </thead>
            <tbody className="text-white/65">
              {steps.map((step, i) => (
                <tr key={step.label} className="border-b border-white/[0.03] last:border-0 hover:bg-white/[0.02]">
                  <td className="py-1.5 pl-3">
                    <span className="inline-flex items-center gap-1.5">
                      <span className="w-2.5 h-2.5 rounded-sm bg-gradient-to-r shrink-0" style={{ background: `linear-gradient(to right, ${FUNNEL_COLORS[i].replace('from-', '').replace('to-', ', ')})` }} />
                      {step.label}
                    </span>
                  </td>
                  <td className="py-1.5 px-3 text-right tabular-nums font-medium text-white/80">{fmt(step.count_all)}</td>
                  <td className="py-1.5 px-3 text-right tabular-nums text-white/50">{fmt(step.count_today)}</td>
                  <td className="py-1.5 px-3 text-right tabular-nums text-white/40">{fmt(step.count_7d)}</td>
                  <td className="py-1.5 px-3 text-right tabular-nums text-white/30">{fmt(step.count_30d)}</td>
                  <td className="py-1.5 px-3 text-right tabular-nums text-emerald-300/70">
                    {i > 0 ? convRate(steps[i - 1].count_all, step.count_all) : '—'}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        {/* Key insight: overall conversion */}
        <div className="mt-3 flex items-center justify-between text-[11px] text-white/30">
          <span>整体转化（访客 → 用 AI）：{convRate(steps[0]?.count_all, steps[steps.length - 1]?.count_all)}</span>
          <span>{data?.generated_at ? `数据更新：${new Date(data.generated_at).toLocaleString('zh-CN')}` : ''}</span>
        </div>
      </div>
    </section>
  )
}

// ── Backup Panel (数据备份) ──

const BACKUP_TRIGGER_LABELS = {
  quadrant_callback: '四象限回调',
  scheduled_fallback: '保底定时',
  manual: '手动触发',
}

const BACKUP_STATUS_COLORS = {
  success: 'text-emerald-400',
  partial: 'text-amber-400',
  failed: 'text-rose-400',
  skipped: 'text-white/40',
  never: 'text-white/30',
}

function formatBytes(bytes) {
  if (bytes == null) return '--'
  if (bytes < 1024) return `${bytes}B`
  if (bytes < 1048576) return `${(bytes / 1024).toFixed(1)}KB`
  return `${(bytes / 1048576).toFixed(1)}MB`
}

function formatDuration(ms) {
  if (ms == null || ms === 0) return '--'
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(1)}s`
}

function BackupPanel() {
  const [status, setStatus] = useState(null)
  const [history, setHistory] = useState(null)
  const [stats, setStats] = useState(null)
  const [triggering, setTriggering] = useState(false)

  const loadData = useCallback(async () => {
    try {
      const [s, h, st] = await Promise.all([
        adminFetch('/api/admin/backup-status').catch(() => null),
        adminFetch('/api/admin/backup-history?limit=7').then(d => d.items || []).catch(() => []),
        adminFetch('/api/admin/backup-stats').catch(() => null),
      ])
      setStatus(s)
      setHistory(h)
      setStats(st)
    } catch { /* silent */ }
  }, [])

  useEffect(() => {
    loadData()
    const timer = setInterval(loadData, 120_000) // backup panel refreshes less frequently
    return () => clearInterval(timer)
  }, [loadData])

  const handleTrigger = async () => {
    if (!window.confirm('确定要立即执行一次备份吗？')) return
    setTriggering(true)
    try {
      await adminFetch('/api/admin/backup-trigger', { method: 'POST' })
      await loadData()
    } catch { /* silent */ }
    setTriggering(false)
  }

  if (!status && !history) return null

  return (
    <section>
      <h2 className="text-base font-semibold text-white/80 mb-3">📦 数据备份</h2>

      {/* Status Cards */}
      <div className="grid grid-cols-2 sm:grid-cols-4 lg:grid-cols-6 gap-3">
        <StatCard label="状态" value={status?.status ?? '--'} sub={BACKUP_TRIGGER_LABELS[status?.last_trigger_type] || ''} />
        <StatCard label="主库大小" value={formatBytes(status?.pumpkin_size_bytes)} />
        <StatCard label="A 股缓存" value={formatBytes(status?.cache_a_size_bytes)} />
        <StatCard label="港股缓存" value={formatBytes(status?.cache_hk_size_bytes)} />
        <StatCard label="COS 同步" value={status?.cos_uploaded ? '✅' : '⏸'} sub={stats?.cloud_enabled ? '已配置' : '未配置'} />
        <StatCard label="耗时" value={formatDuration(status?.duration_ms)} />
      </div>

      {/* Error Message */}
      {status?.error_msg && (
        <div className="mt-3 rounded-xl bg-rose-500/10 border border-rose-400/20 px-3 py-2 text-xs text-rose-200">
          {status.error_msg}
        </div>
      )}

      {/* Storage Stats */}
      {stats && (
        <div className="mt-3 flex gap-6 text-xs text-white/40">
          <span>本地: {formatBytes(stats.local_total_bytes)} ({stats.local_file_count} 文件 · 保留{stats.local_retention_days}天)</span>
          {stats.cloud_enabled && (
            <span>云端: {formatBytes(stats.cloud_total_bytes)} ({stats.cloud_file_count} 文件)</span>
          )}
        </div>
      )}

      {/* Manual Trigger */}
      <div className="mt-4 flex items-center justify-between">
        <h3 className="text-sm font-medium text-white/55">最近备份记录</h3>
        <button
          type="button"
          disabled={triggering}
          onClick={handleTrigger}
          className={`rounded-lg border px-3 py-1.5 text-xs font-medium transition ${
            triggering
              ? 'border-white/10 bg-white/5 text-white/30 cursor-not-allowed'
              : 'border-emerald-400/30 bg-emerald-500/8 text-emerald-300 hover:bg-emerald-500/15 hover:border-emerald-400/50'
          }`}
        >
          {triggering ? '备份中...' : '🔄 立即备份'}
        </button>
      </div>

      {/* History Table */}
      {!history || history.length === 0 ? (
        <p className="mt-2 text-xs text-white/30 p-3 rounded-xl border border-dashed border-white/10 text-center">
          暂无备份记录 — 系统将在每天凌晨自动执行备份
        </p>
      ) : (
        <div className="mt-2 rounded-xl border border-white/8 bg-[#15171e]/70 overflow-hidden">
          <table className="w-full text-xs text-left">
            <thead>
              <tr className="border-b border-white/[0.06] text-white/30">
                <th className="py-2 pl-4 font-medium">时间</th>
                <th className="py-2 px-3 font-medium">触发方式</th>
                <th className="py-2 px-3 font-medium">状态</th>
                <th className="py-2 px-3 font-medium text-right">主库</th>
                <th className="py-2 px-3 font-medium text-right">缓存</th>
                <th className="py-2 px-3 font-medium text-center">COS</th>
                <th className="py-2 px-3 font-medium text-right">耗时</th>
                <th className="py-2 pr-4 font-medium text-left">备注</th>
              </tr>
            </thead>
            <tbody className="text-white/65">
              {history.map((row) => (
                <tr key={row.id} className="border-b border-white/[0.03] last:border-0 hover:bg-white/[0.02]">
                  <td className="py-1.5 pl-4 whitespace-nowrap tabular-nums text-white/35">{row.triggered_at}</td>
                  <td className="py-1.5 px-3 text-white/50">{BACKUP_TRIGGER_LABELS[row.trigger_type] || row.trigger_type}</td>
                  <td className={`py-1.5 px-3 font-medium ${BACKUP_STATUS_COLORS[row.status] || ''}`}>{row.status}</td>
                  <td className="py-1.5 px-3 text-right tabular-nums">{formatBytes(row.pumpkin_size_bytes)}</td>
                  <td className="py-1.5 px-3 text-right tabular-nums">{formatBytes(row.cache_a_size_bytes + row.cache_hk_size_bytes)}</td>
                  <td className="py-1.5 px-3 text-center">{row.cos_uploaded ? '✅' : '-'}</td>
                  <td className="py-1.5 px-3 text-right tabular-nums text-white/35">{formatDuration(row.duration_ms)}</td>
                  <td className="py-1.5 pr-4 text-white/25 max-w-[200px] truncate" title={row.error_msg}>
                    {row.integrity_check !== 'ok' ? (row.error_msg || '-') : (row.integrity_check === 'ok' ? '✅ 校验通过' : '-')}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
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
          <meta name="robots" content="noindex, nofollow" />
        </Head>
        <div className="min-h-screen bg-[#0a0b0f]" />
      </>
    )
  }

  return (
    <>
      <Head>
        <title>管理后台 — Wolong Pro</title>
        <meta name="robots" content="noindex, nofollow" />
      </Head>
      {session ? (
        <AdminDashboard session={session} onLogout={() => setSession(null)} />
      ) : (
        <AdminLoginForm onLogin={(result) => setSession(result)} />
      )}
    </>
  )
}
