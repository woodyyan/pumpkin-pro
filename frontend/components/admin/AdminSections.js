import { useCallback, useEffect, useRef, useState } from 'react'
import MiniChart from '../MiniChart'
import AIReportsAdminPanel from './AIReportsAdminPanel'
import FactorIndexAdminPanel from './FactorIndexAdminPanel'
import {
  BACKUP_STATUS_COLORS,
  BACKUP_TRIGGER_LABELS,
  buildBackupHistoryNote,
  buildBackupJobBanner,
  buildBackupStatusCards,
  formatBackupBytes,
  formatBackupDuration,
  getBackupCosMeta,
  resolveBackupTriggerButton,
  shouldPollBackupStatus,
} from '../../lib/backup-ui'
import {
  adminFetch,
  handleAdminActionError,
  useAdminResource,
} from '../../lib/admin-data'
import {
  resolveAdminPaymentDraftForMethod,
  resolveAdminPaymentMethodMeta,
  resolveAdminPaymentMethodOptions,
  resolveAdminPaymentPollingState,
  resolveAdminSelectedPaymentId,
} from '../../lib/admin-payments'

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
            <span className="text-foreground-muted">{item.category}</span>
          </div>
          <div className="tabular-nums text-foreground-dim">
            {item.count} <span className="text-foreground-disabled">({(item.percentage || 0).toFixed(1)}%)</span>
          </div>
        </div>
      ))}
    </div>
  )
}

const REFRESH_INTERVAL = 60_000

// ── Login Form ──

export function AdminLoginForm({ onLogin }) {
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
      onLogin(result)
    } catch (err) {
      setError(err.message || '登录失败')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="min-h-screen bg-background flex items-center justify-center px-4">
      <div className="w-full max-w-md">
        <div className="text-center mb-8">
          <img src="/logo.png" alt="卧龙" width={56} height={56} className="mx-auto rounded" />
          <h1 className="mt-3 text-2xl font-bold text-foreground">Wolong Pro 管理后台</h1>
          <p className="mt-2 text-sm text-foreground-dim">仅限超级管理员访问</p>
        </div>

        <form
          onSubmit={submit}
          className="rounded-2xl border border-border bg-card/95 p-6 shadow-2xl"
        >
          <div className="space-y-4">
            <div>
              <label className="block text-xs text-foreground-dim mb-1.5">管理员邮箱</label>
              <input
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                autoComplete="email"
                className="w-full rounded-xl border border-border bg-background-alt px-4 py-2.5 text-sm text-foreground outline-none transition placeholder:text-foreground-disabled focus:border-primary focus:bg-[var(--color-bg-hover)]"
                placeholder="admin@example.com"
              />
            </div>
            <div>
              <label className="block text-xs text-foreground-dim mb-1.5">密码</label>
              <input
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                autoComplete="current-password"
                className="w-full rounded-xl border border-border bg-background-alt px-4 py-2.5 text-sm text-foreground outline-none transition placeholder:text-foreground-disabled focus:border-primary focus:bg-[var(--color-bg-hover)]"
                placeholder="••••••••"
              />
            </div>
          </div>

          {error && (
            <div className="mt-4 rounded-xl bg-negative/10 px-3 py-2 text-sm text-negative">{error}</div>
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
    <div className="rounded-xl border border-border bg-card px-4 py-3">
      <div className="text-xs text-foreground-dim mb-1">{label}</div>
      <div className="text-2xl font-bold text-foreground tabular-nums">{value ?? '--'}</div>
      {sub && <div className="mt-0.5 text-xs text-foreground-dim">{sub}</div>}
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

function formatPercentValue(value, digits = 1) {
  const num = Number(value)
  if (!Number.isFinite(num)) return '--'
  return `${(num * 100).toFixed(digits)}%`
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
    badgeClassName: 'border-emerald-400/25 bg-positive/10 text-positive',
  },
  invalid_auth: {
    label: '鉴权失败',
    badgeClassName: 'border-rose-400/25 bg-negative/10 text-negative',
  },
  invalid_model: {
    label: '模型不可用',
    badgeClassName: 'border-amber-500/30 bg-amber-50 text-amber-800 dark:border-amber-400/25 dark:bg-amber-500/12 dark:text-amber-100',
  },
  timeout: {
    label: '请求超时',
    badgeClassName: 'border-amber-500/30 bg-amber-50 text-amber-800 dark:border-amber-400/25 dark:bg-amber-500/12 dark:text-amber-100',
  },
  network_error: {
    label: '网络异常',
    badgeClassName: 'border-amber-500/30 bg-amber-50 text-amber-800 dark:border-amber-400/25 dark:bg-amber-500/12 dark:text-amber-100',
  },
  provider_error: {
    label: '服务异常',
    badgeClassName: 'border-[var(--color-border-strong)] bg-[var(--color-bg-hover)] text-foreground-muted',
  },
  disabled: {
    label: '已禁用',
    badgeClassName: 'border-[var(--color-border-strong)] bg-[var(--color-bg-hover)] text-foreground-dim',
  },
  unknown: {
    label: '未测试',
    badgeClassName: 'border-sky-400/20 bg-sky-500/10 text-sky-100',
  },
  unconfigured: {
    label: '未配置',
    badgeClassName: 'border-[var(--color-border-strong)] bg-[var(--color-bg-hover)] text-foreground-dim',
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
      timeZone: 'Asia/Shanghai',
      hour12: false,
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
    })
  } catch {
    return '--'
  }
}

const PAYMENT_STATUS_META = {
  initiated: { label: '已创建', className: 'border-slate-400/20 bg-slate-500/10 text-slate-200' },
  checkout_open: { label: '待支付', className: 'border-sky-400/20 bg-sky-500/10 text-sky-100' },
  processing: { label: '处理中', className: 'border-amber-400/25 bg-amber-500/12 text-amber-100' },
  succeeded: { label: '已成功', className: 'border-emerald-400/25 bg-positive/10 text-positive' },
  failed: { label: '已失败', className: 'border-rose-400/25 bg-negative/10 text-negative' },
  expired: { label: '已过期', className: 'border-[var(--color-border-strong)] bg-[var(--color-bg-hover)] text-foreground-dim' },
  refunded: { label: '已退款', className: 'border-fuchsia-400/25 bg-fuchsia-500/10 text-fuchsia-100' },
  partially_refunded: { label: '部分退款', className: 'border-fuchsia-400/25 bg-fuchsia-500/10 text-fuchsia-100' },
}

const PAYMENT_METHOD_LABELS = {
  card: '银行卡',
  alipay: '支付宝',
  wechat_pay: '微信支付',
}

function getPaymentStatusMeta(status) {
  return PAYMENT_STATUS_META[status] || { label: status || '--', className: 'border-[var(--color-border-strong)] bg-[var(--color-bg-hover)] text-foreground-dim' }
}

function formatPaymentMethodList(value) {
  if (!value) return '--'
  const items = Array.isArray(value)
    ? value
    : String(value).split(',').map((item) => item.trim()).filter(Boolean)
  if (!items.length) return '--'
  return items.map((item) => PAYMENT_METHOD_LABELS[item] || item).join(' / ')
}

function formatMinorAmount(amountMinor, currency = 'cny') {
  if (amountMinor == null || Number.isNaN(Number(amountMinor))) return '--'
  const normalizedCurrency = String(currency || 'cny').toUpperCase()
  const value = Number(amountMinor) / 100
  try {
    return new Intl.NumberFormat('zh-CN', {
      style: 'currency',
      currency: normalizedCurrency,
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    }).format(value)
  } catch {
    return `${value.toFixed(2)} ${normalizedCurrency}`
  }
}

function AIConfigMetric({ label, value, sub }) {
  return (
    <div className="rounded-2xl border border-border bg-card px-4 py-3">
      <div className="text-[11px] text-foreground-dim">{label}</div>
      <div className="mt-1 text-sm font-semibold text-foreground">{value || '--'}</div>
      {sub ? <div className="mt-1 text-[11px] text-foreground-dim">{sub}</div> : null}
    </div>
  )
}

export function AIProviderConfigPanel({ onUnauthorized }) {
  const [draft, setDraft] = useState(() => createAIConfigDraft(null))
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState(false)
  const [banner, setBanner] = useState(null)
  const [testResult, setTestResult] = useState(null)
  const initializedRef = useRef(false)

  const handleUnauthorized = useCallback(() => {
    onUnauthorized?.()
  }, [onUnauthorized])

  const aiConfigResource = useAdminResource({
    key: 'admin:ai-config',
    request: () => adminFetch('/api/admin/ai-config'),
    staleMs: 30_000,
    minIntervalMs: 3_000,
    onUnauthorized: handleUnauthorized,
    errorMessage: '加载 AI 配置失败',
  })
  const view = aiConfigResource.data

  useEffect(() => {
    if (!view) return
    if (!initializedRef.current) {
      setDraft(createAIConfigDraft(view))
      initializedRef.current = true
    }
  }, [view])

  useEffect(() => {
    if (aiConfigResource.error) {
      setBanner({ tone: 'error', text: aiConfigResource.error })
    }
  }, [aiConfigResource.error])

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
      aiConfigResource.mutate(data)
      setDraft(createAIConfigDraft(data))
      setTestResult(null)
      setBanner({ tone: 'success', text: 'AI 配置已保存' })
    } catch (err) {
      const message = handleAdminActionError(err, handleUnauthorized, '保存 AI 配置失败')
      if (!message) {
        return
      }
      setBanner({ tone: 'error', text: message })
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
      const message = handleAdminActionError(err, handleUnauthorized, '测试连接失败')
      if (!message) {
        return
      }
      setBanner({ tone: 'error', text: message })
    } finally {
      setTesting(false)
    }
  }

  if (aiConfigResource.loading && !view) {
    return (
      <section className="rounded-2xl border border-border bg-card p-5">
        <div className="text-sm text-foreground-dim">加载 AI 配置中…</div>
      </section>
    )
  }

  return (
    <section className="rounded-2xl border border-border bg-card p-4 sm:p-5">
      <div className="flex flex-col gap-4 xl:flex-row xl:items-start xl:justify-between">
        <div>
          <h2 className="text-base font-semibold text-foreground-muted">🤖 AI 模型配置</h2>
          <p className="mt-1 text-xs text-foreground-dim">当前系统仅支持 OpenAI-compatible Chat Completions 接口</p>
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
              ? 'border-positive/20 bg-positive/10 text-emerald-100'
              : banner.tone === 'error'
                ? 'border-rose-400/20 bg-negative/10 text-negative'
                : 'border-sky-400/20 bg-sky-500/10 text-sky-100'
          }`}
        >
          {banner.text}
        </div>
      ) : null}

      <div className="mt-5 grid grid-cols-1 gap-4 lg:grid-cols-2">
        <div>
          <label className="mb-1.5 block text-xs text-foreground-dim">Base URL</label>
          <input
            type="text"
            value={draft.base_url}
            onChange={(e) => updateDraft('base_url', e.target.value)}
            className="w-full rounded-2xl border border-border bg-card px-4 py-3 text-sm text-foreground outline-none transition focus:border-primary/60 focus:bg-[var(--color-bg-hover)]"
            placeholder="https://api.openai.com/v1"
          />
        </div>
        <div>
          <label className="mb-1.5 block text-xs text-foreground-dim">Model ID</label>
          <input
            type="text"
            value={draft.model_id}
            onChange={(e) => updateDraft('model_id', e.target.value)}
            className="w-full rounded-2xl border border-border bg-card px-4 py-3 text-sm text-foreground outline-none transition focus:border-primary/60 focus:bg-[var(--color-bg-hover)]"
            placeholder="gpt-4o-mini"
          />
        </div>
      </div>

      <div className="mt-4">
        <label className="mb-1.5 block text-xs text-foreground-dim">API Key</label>
        <input
          type="password"
          value={draft.api_key}
          onChange={(e) => updateDraft('api_key', e.target.value)}
          className="w-full rounded-2xl border border-border bg-card px-4 py-3 text-sm text-foreground outline-none transition focus:border-primary/60 focus:bg-[var(--color-bg-hover)]"
          placeholder="留空表示保持当前 key"
        />
        <div className="mt-2 flex flex-wrap items-center gap-2 text-xs text-foreground-dim">
          <span className="rounded-full border border-border bg-[var(--color-bg-hover)] px-2.5 py-1">
            {view?.config?.has_api_key ? `已保存：${view.config.api_key_mask || '已保存'}` : '暂未保存后台 key'}
          </span>
          <span>只有更换 key 时才需要重新输入</span>
        </div>
      </div>

      <label className="mt-4 flex items-start gap-3 rounded-2xl border border-border bg-card px-4 py-3">
        <input
          type="checkbox"
          checked={draft.is_enabled}
          onChange={(e) => updateDraft('is_enabled', e.target.checked)}
          className="mt-0.5 h-4 w-4 rounded border-[var(--color-border-strong)] bg-transparent text-amber-400 focus:ring-amber-400"
        />
        <div>
          <div className="text-sm font-medium text-foreground">启用后台配置</div>
          <div className="mt-1 text-xs text-foreground-dim">启用后将优先使用这里保存的模型参数；关闭后自动回退到环境变量。</div>
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
          className="rounded-2xl border border-amber-500/30 bg-amber-50 px-4 py-3 text-sm font-medium text-amber-800 transition hover:bg-amber-100 dark:border-amber-400/30 dark:bg-amber-500/12 dark:text-amber-100 dark:hover:bg-amber-500/18 disabled:cursor-not-allowed disabled:opacity-60"
        >
          {saving ? '保存中…' : '保存配置'}
        </button>
        <button
          type="button"
          onClick={restoreSaved}
          disabled={!view}
          className="rounded-2xl border border-[var(--color-border-strong)] bg-[var(--color-bg-hover)] px-4 py-3 text-sm font-medium text-foreground/72 transition hover:bg-[var(--color-bg-secondary)]"
        >
          恢复已保存值
        </button>
      </div>

      <div className="mt-5 flex flex-wrap items-center gap-2 text-xs">
        <span className={`rounded-full border px-2.5 py-1 ${healthMeta.badgeClassName}`}>
          {healthMeta.label}
        </span>
        <span className="text-foreground/42">当前生效模型：{view?.effective?.model_id || '--'}</span>
        <span className="text-foreground/28">base URL：{view?.effective?.base_url || '--'}</span>
      </div>
    </section>
  )
}

// ── Dashboard ──

export function AdminOverviewPage({ onUnauthorized }) {
  const [deviceDays, setDeviceDays] = useState(30)
  const dashboardResource = useAdminResource({
    key: `admin:dashboard:${deviceDays}`,
    request: async () => {
      const [statsData, analyticsData, deviceData] = await Promise.all([
        adminFetch('/api/admin/stats'),
        adminFetch('/api/admin/analytics').catch(() => null),
        adminFetch(`/api/admin/device-analytics?days=${deviceDays}`).catch(() => null),
      ])
      return {
        stats: statsData,
        analytics: analyticsData,
        deviceAnalytics: deviceData,
      }
    },
    staleMs: 20_000,
    minIntervalMs: 5_000,
    pollMs: REFRESH_INTERVAL,
    onUnauthorized,
    errorMessage: '加载统计数据失败',
  })

  const stats = dashboardResource.data?.stats || null
  const analytics = dashboardResource.data?.analytics || null
  const deviceAnalytics = dashboardResource.data?.deviceAnalytics || null
  const error = dashboardResource.error
  const loading = dashboardResource.loading

  return (
    <div className="space-y-8">
        {error && (
          <div className="rounded-xl bg-negative/10 border border-rose-400/20 px-4 py-3 text-sm text-negative">
            {error}
          </div>
        )}

        {loading && !stats ? (
          <div className="py-20 text-center text-foreground-dim">加载中…</div>
        ) : stats ? (
          <>
            {/* Panel 1: Users */}
            <section>
              <h2 className="text-base font-semibold text-foreground-muted mb-3">👤 用户概览</h2>
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
                <div className="rounded-xl border border-border bg-card p-3">
                  <MiniChart data={stats.trends?.daily_registrations} label="每日注册" width={380} height={130} color="#22c55e" />
                </div>
                <div className="rounded-xl border border-border bg-card p-3">
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
              <h2 className="text-base font-semibold text-foreground-muted mb-3">🧩 功能使用</h2>
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
                <div className="mt-4 rounded-xl border border-border bg-card p-3">
                  <MiniChart data={stats.trends.daily_portfolio_ops} label="每日持仓操作（30天）" width={780} height={130} type="bar" color="#14b8a6" />
                </div>
              )}
            </section>

            {/* Panel 3: Strategies */}
            <section>
              <h2 className="text-base font-semibold text-foreground-muted mb-3">📊 策略使用</h2>
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
              <h2 className="text-base font-semibold text-foreground-muted mb-3">📈 自选股</h2>
              <div className="grid grid-cols-2 gap-3 sm:grid-cols-2">
                <StatCard label="关注池条目" value={stats.live.watchlist_items} />
                <StatCard label="有关注的用户" value={stats.live.users_with_watchlist} />
              </div>
            </section>

            {/* Panel 5: Company Profiles */}
            <CompanyProfilesAdminPanel onUnauthorized={onUnauthorized} />

            {/* Panel 6: Signals & Webhook */}
            <section>
              <h2 className="text-base font-semibold text-foreground-muted mb-3">🔔 信号与 Webhook</h2>
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
                <div className="mt-4 rounded-xl border border-border bg-card p-3">
                  <MiniChart data={stats.trends.daily_signal_events} label="每日信号事件" width={780} height={130} type="bar" color="#eab308" />
                </div>
              )}
            </section>

            {/* Panel 6: Device & Browser */}
            {deviceAnalytics && (
              <section>
                <div className="flex items-center justify-between gap-3 mb-3">
                  <h2 className="text-base font-semibold text-foreground-muted">📱 设备与浏览器</h2>
                  <div className="flex items-center gap-2">
                    {([7, 30, 0]).map((d) => (
                      <button
                        key={d}
                        onClick={() => setDeviceDays(d)}
                        className={`rounded-lg px-2.5 py-1 text-xs transition ${
                          deviceDays === d
                            ? 'bg-[var(--color-bg-hover)] text-foreground'
                            : 'text-foreground-dim hover:text-foreground-muted'
                        }`}
                      >
                        {d === 0 ? '全部' : `${d}日`}
                      </button>
                    ))}
                  </div>
                </div>

                <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                  {/* Device Type */}
                  <div className="rounded-xl border border-border bg-card p-4">
                    <div className="text-xs text-foreground-dim mb-3 text-center">设备类型</div>
                    {deviceAnalytics.device_types && deviceAnalytics.device_types.length > 0 ? (
                      <>
                        <DoughnutChart data={deviceAnalytics.device_types} />
                        <CategoryLegend data={deviceAnalytics.device_types} />
                      </>
                    ) : (
                      <p className="text-xs text-foreground-disabled text-center py-4">暂无数据</p>
                    )}
                  </div>

                  {/* OS */}
                  <div className="rounded-xl border border-border bg-card p-4">
                    <div className="text-xs text-foreground-dim mb-3 text-center">操作系统</div>
                    {deviceAnalytics.os_families && deviceAnalytics.os_families.length > 0 ? (
                      <>
                        <DoughnutChart data={deviceAnalytics.os_families} />
                        <CategoryLegend data={deviceAnalytics.os_families} />
                      </>
                    ) : (
                      <p className="text-xs text-foreground-disabled text-center py-4">暂无数据</p>
                    )}
                  </div>

                  {/* Browser */}
                  <div className="rounded-xl border border-border bg-card p-4">
                    <div className="text-xs text-foreground-dim mb-3 text-center">浏览器</div>
                    {deviceAnalytics.browser_families && deviceAnalytics.browser_families.length > 0 ? (
                      <>
                        <DoughnutChart data={deviceAnalytics.browser_families} />
                        <CategoryLegend data={deviceAnalytics.browser_families} />
                      </>
                    ) : (
                      <p className="text-xs text-foreground-disabled text-center py-4">暂无数据</p>
                    )}
                  </div>
                </div>

                {/* Top Active Users */}
                {deviceAnalytics.top_active_users && deviceAnalytics.top_active_users.length > 0 && (
                  <div className="mt-4 rounded-xl border border-border bg-card p-4">
                    <div className="text-xs text-foreground-dim mb-3">最活跃用户浏览器偏好（Top {deviceAnalytics.top_active_users.length}）</div>
                    <div className="overflow-x-auto">
                      <table className="w-full text-xs text-left">
                        <thead>
                          <tr className="border-b border-border text-foreground-dim">
                            <th className="pb-2 pr-4 font-medium">用户邮箱</th>
                            <th className="pb-2 pr-4 font-medium">活跃天数</th>
                            <th className="pb-2 pr-4 font-medium">最后活跃</th>
                            <th className="pb-2 pr-4 font-medium">浏览器</th>
                            <th className="pb-2 font-medium">操作系统</th>
                          </tr>
                        </thead>
                        <tbody className="text-foreground-muted">
                          {deviceAnalytics.top_active_users.map((u, i) => (
                            <tr key={`${u.user_id}-${i}`} className="border-b border-border last:border-0">
                              <td className="py-1.5 pr-4 text-foreground-muted">{u.email || u.user_id?.slice(0, 12) || '-'}</td>
                              <td className="py-1.5 pr-4 tabular-nums">{u.active_days} 天</td>
                              <td className="py-1.5 pr-4 text-foreground-dim whitespace-nowrap">{u.last_active_at ? formatTimeAgo(u.last_active_at) : '-'}</td>
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
                <h2 className="text-base font-semibold text-foreground-muted mb-3">🌐 访问统计</h2>
                <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-6 gap-3">
                  <StatCard label="今日 PV" value={analytics.today_pv} />
                  <StatCard label="今日 UV" value={analytics.today_uv} />
                  <StatCard label="7天 PV" value={analytics.week_pv} />
                  <StatCard label="7天 UV" value={analytics.week_uv} />
                  <StatCard label="30天 PV" value={analytics.month_pv} />
                  <StatCard label="30天 UV" value={analytics.month_uv} />
                </div>
                <div className="mt-4 grid grid-cols-1 md:grid-cols-2 gap-4">
                  <div className="rounded-xl border border-border bg-card p-3">
                    <MiniChart data={analytics.daily_pv} label="每日 PV" width={380} height={130} color="#a78bfa" />
                  </div>
                  <div className="rounded-xl border border-border bg-card p-3">
                    <MiniChart data={analytics.daily_uv} label="每日 UV" width={380} height={130} color="#34d399" />
                  </div>
                </div>
                {/* Top pages */}
                {analytics.top_pages && analytics.top_pages.length > 0 && (
                  <div className="mt-4 rounded-xl border border-border bg-card p-4">
                    <div className="text-xs text-foreground-dim mb-3">页面访问排行（30天）</div>
                    <div className="space-y-2">
                      {analytics.top_pages.map((p, i) => {
                        const maxCount = analytics.top_pages[0]?.count || 1
                        const pct = (p.count / maxCount) * 100
                        return (
                          <div key={i} className="flex items-center gap-3 text-sm">
                            <span className="w-28 truncate text-foreground-muted text-xs">{p.page_path}</span>
                            <div className="flex-1 h-4 rounded bg-[var(--color-bg-hover)] overflow-hidden">
                              <div className="h-full rounded bg-primary/30" style={{ width: `${pct}%` }} />
                            </div>
                            <span className="text-xs text-foreground-dim tabular-nums w-10 text-right">{p.count}</span>
                          </div>
                        )
                      })}
                    </div>
                  </div>
                )}
              </section>
            )}

            {/* Panel 7: Traffic Sources */}
            {stats.traffic && (
              <section>
                <h2 className="text-base font-semibold text-foreground-muted mb-3">🌍 流量来源</h2>
                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  {/* UTM Source breakdown (user registration source) */}
                  <div className="rounded-xl border border-border bg-card p-4">
                    <div className="text-xs text-foreground-dim mb-3">注册来源（UTM Source）</div>
                    {(stats.traffic.utm_sources || []).length === 0 ? (
                      <p className="text-xs text-foreground-disabled">暂无数据（推广链接加 ?utm_source=xxx 即可追踪）</p>
                    ) : (
                      <div className="space-y-2">
                        {stats.traffic.utm_sources.map((s, i) => {
                          const maxCount = stats.traffic.utm_sources[0]?.count || 1
                          const pct = (s.count / maxCount) * 100
                          return (
                            <div key={i} className="flex items-center gap-3 text-sm">
                              <span className="w-24 truncate text-foreground-muted text-xs">{s.source}</span>
                              <div className="flex-1 h-4 rounded bg-[var(--color-bg-hover)] overflow-hidden">
                                <div className="h-full rounded bg-emerald-500/30" style={{ width: `${pct}%` }} />
                              </div>
                              <span className="text-xs text-foreground-dim tabular-nums w-8 text-right">{s.count}</span>
                            </div>
                          )
                        })}
                      </div>
                    )}
                  </div>
                  {/* Referrer breakdown (pageview referrer) */}
                  <div className="rounded-xl border border-border bg-card p-4">
                    <div className="text-xs text-foreground-dim mb-3">访问来源（Referrer · 30天）</div>
                    {(stats.traffic.referrers || []).length === 0 ? (
                      <p className="text-xs text-foreground-disabled">暂无数据</p>
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
                              <span className="w-32 truncate text-foreground-muted text-xs" title={s.source}>{label}</span>
                              <div className="flex-1 h-4 rounded bg-[var(--color-bg-hover)] overflow-hidden">
                                <div className="h-full rounded bg-blue-500/30" style={{ width: `${pct}%` }} />
                              </div>
                              <span className="text-xs text-foreground-dim tabular-nums w-8 text-right">{s.count}</span>
                            </div>
                          )
                        })}
                      </div>
                    )}
                  </div>
                </div>
              </section>
            )}

            {/* Panel 10: Audit */}
            <section>
              <h2 className="text-base font-semibold text-foreground-muted mb-3">🛡️ 审计日志</h2>
              <div className="grid grid-cols-2 sm:grid-cols-3 gap-3">
                <StatCard label="今日登录次数" value={stats.audit.today_logins} />
                <StatCard label="今日注册次数" value={stats.audit.today_registrations} />
                <StatCard label="7天登录失败" value={stats.audit.failed_logins_7d} />
              </div>
            </section>

            <UserFunnelPanel onUnauthorized={onUnauthorized} />
          </>
        ) : null}
    </div>
  )
}

export function AIUsageAdminPanel({ onUnauthorized }) {
  const resource = useAdminResource({
    key: 'admin:ai-usage-dashboard',
    request: async () => {
      const [statsData, aiUsageData] = await Promise.all([
        adminFetch('/api/admin/stats'),
        adminFetch('/api/admin/ai-usage?days=30&limit=120').catch(() => null),
      ])
      return {
        stats: statsData,
        aiUsage: aiUsageData,
      }
    },
    staleMs: 20_000,
    minIntervalMs: 5_000,
    pollMs: REFRESH_INTERVAL,
    onUnauthorized,
    errorMessage: '加载 AI 使用统计失败',
  })

  const stats = resource.data?.stats || null
  const aiUsage = resource.data?.aiUsage || null

  if (resource.loading && !stats) return null

  return stats?.ai ? (
    <section>
      {resource.error ? (
        <div className="mb-3 rounded-xl border border-rose-400/20 bg-negative/10 px-4 py-2 text-xs text-negative">
          {resource.error}
        </div>
      ) : null}

      <h2 className="text-base font-semibold text-foreground-muted mb-3">🤖 AI 调用统计</h2>

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

      <div className="mt-3 rounded-xl border border-amber-500/20 bg-amber-50 px-4 py-3 text-xs leading-6 text-amber-800 dark:border-amber-400/15 dark:bg-amber-500/[0.06] dark:text-amber-100/85">
        当前 token 面板基于 `ai_call_logs` 的真实 usage 字段聚合；旧日志或 provider 未返回 usage 的请求会记为 0，所以这部分数据会从本次版本上线后逐步变完整。
      </div>

      <div className="mt-4 grid grid-cols-1 xl:grid-cols-2 gap-4">
        {stats.ai.by_feature && stats.ai.by_feature.length > 0 ? (
          <div className="rounded-xl border border-border bg-card p-4">
            <div className="text-xs text-foreground-dim mb-3">按功能分布（调用次数）</div>
            <div className="space-y-2">
              {stats.ai.by_feature.map((f) => {
                const maxCount = stats.ai.by_feature[0]?.count || 1
                const pct = (f.count / maxCount) * 100
                return (
                  <div key={f.feature_key} className="flex items-center gap-3 text-sm">
                    <span className="w-28 truncate text-foreground-muted text-xs">{f.feature_name}</span>
                    <div className="flex-1 h-4 rounded bg-[var(--color-bg-hover)] overflow-hidden">
                      <div className="h-full rounded bg-violet-500/40" style={{ width: `${pct}%` }} />
                    </div>
                    <span className="text-xs text-foreground-dim tabular-nums w-12 text-right">{fmt(f.count)}</span>
                  </div>
                )
              })}
            </div>
          </div>
        ) : null}

        {stats.ai.by_feature_tokens && stats.ai.by_feature_tokens.length > 0 ? (
          <div className="rounded-xl border border-border bg-card p-4">
            <div className="text-xs text-foreground-dim mb-3">按功能分布（Token 消耗）</div>
            <div className="space-y-2">
              {stats.ai.by_feature_tokens.map((f) => {
                const maxTokens = stats.ai.by_feature_tokens[0]?.total_tokens || 1
                const pct = (f.total_tokens / maxTokens) * 100
                return (
                  <div key={f.feature_key} className="flex items-center gap-3 text-sm">
                    <span className="w-28 truncate text-foreground-muted text-xs">{f.feature_name}</span>
                    <div className="flex-1 h-4 rounded bg-[var(--color-bg-hover)] overflow-hidden">
                      <div className="h-full rounded bg-fuchsia-500/45" style={{ width: `${pct}%` }} />
                    </div>
                    <span className="text-xs text-fuchsia-200/80 tabular-nums w-20 text-right">{fmt(f.total_tokens)}</span>
                  </div>
                )
              })}
            </div>
          </div>
        ) : null}
      </div>

      <div className="mt-4 grid grid-cols-1 xl:grid-cols-2 gap-4">
        {stats.ai.daily_trend && stats.ai.daily_trend.length > 1 ? (
          <div className="rounded-xl border border-border bg-card p-3">
            <MiniChart data={stats.ai.daily_trend} label="每日 AI 调用趋势（30天）" width={380} height={130} type="bar" color="#a78bfa" />
          </div>
        ) : null}
        {stats.ai.daily_token_trend && stats.ai.daily_token_trend.length > 1 ? (
          <div className="rounded-xl border border-border bg-card p-3">
            <MiniChart data={stats.ai.daily_token_trend} label="每日 Token 用量（30天）" width={380} height={130} type="bar" color="#ec4899" />
          </div>
        ) : null}
      </div>

      <div className="mt-4 grid grid-cols-1 xl:grid-cols-2 gap-4">
        {stats.ai.top_users && stats.ai.top_users.length > 0 ? (
          <div className="rounded-xl border border-border bg-card p-4">
            <div className="text-xs text-foreground-dim mb-3">TOP 调用用户（前 10）</div>
            <div className="overflow-x-auto">
              <table className="w-full text-xs text-left">
                <thead>
                  <tr className="border-b border-border text-foreground-dim">
                    <th className="pb-2 pr-4 font-medium">排名</th>
                    <th className="pb-2 pr-4 font-medium">用户</th>
                    <th className="pb-2 pr-4 font-medium text-right">调用次数</th>
                    <th className="pb-2 font-medium text-right">最近一次</th>
                  </tr>
                </thead>
                <tbody className="text-foreground-muted">
                  {stats.ai.top_users.map((u, i) => (
                    <tr key={`${u.user_id}-${i}`} className="border-b border-border last:border-0">
                      <td className="py-1.5 pr-4 tabular-nums text-foreground-dim">{i + 1}</td>
                      <td className="py-1.5 pr-4">
                        <div className="max-w-[180px] truncate" title={u.email || u.user_id}>{formatUserDisplay(u)}</div>
                        <div className="text-[10px] text-foreground-disabled font-mono">{u.user_id?.slice(0, 12)}…</div>
                      </td>
                      <td className="py-1.5 pr-4 text-right tabular-nums font-medium text-violet-300">{fmt(u.call_count)}</td>
                      <td className="py-1.5 text-right text-foreground-dim whitespace-nowrap">
                        {u.last_called_at ? new Date(u.last_called_at).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }) : '-'}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        ) : null}

        {stats.ai.top_token_users && stats.ai.top_token_users.length > 0 ? (
          <div className="rounded-xl border border-border bg-card p-4">
            <div className="text-xs text-foreground-dim mb-3">TOP Token 用户（前 10）</div>
            <div className="overflow-x-auto">
              <table className="w-full text-xs text-left">
                <thead>
                  <tr className="border-b border-border text-foreground-dim">
                    <th className="pb-2 pr-4 font-medium">排名</th>
                    <th className="pb-2 pr-4 font-medium">用户</th>
                    <th className="pb-2 pr-4 font-medium text-right">总 Token</th>
                    <th className="pb-2 pr-4 font-medium text-right">调用</th>
                    <th className="pb-2 font-medium text-right">最近一次</th>
                  </tr>
                </thead>
                <tbody className="text-foreground-muted">
                  {stats.ai.top_token_users.map((u, i) => (
                    <tr key={`${u.user_id}-${i}`} className="border-b border-border last:border-0">
                      <td className="py-1.5 pr-4 tabular-nums text-foreground-dim">{i + 1}</td>
                      <td className="py-1.5 pr-4">
                        <div className="max-w-[180px] truncate" title={u.email || u.user_id}>{formatUserDisplay(u)}</div>
                        <div className="text-[10px] text-foreground-disabled font-mono">{u.user_id?.slice(0, 12)}…</div>
                      </td>
                      <td className="py-1.5 pr-4 text-right tabular-nums font-medium text-fuchsia-300">{fmt(u.total_tokens)}</td>
                      <td className="py-1.5 pr-4 text-right tabular-nums text-foreground-dim">{fmt(u.call_count)}</td>
                      <td className="py-1.5 text-right text-foreground-dim whitespace-nowrap">
                        {u.last_called_at ? new Date(u.last_called_at).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }) : '-'}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        ) : null}
      </div>

      {aiUsage?.daily_users && aiUsage.daily_users.length > 0 ? (
        <div className="mt-4 rounded-xl border border-border bg-card p-4">
          <div className="flex items-center justify-between gap-3 mb-3">
            <div>
              <div className="text-xs text-foreground-dim">每日每用户 Token 用量（近 {aiUsage.days || 30} 天）</div>
              <div className="mt-1 text-[11px] text-foreground-disabled">按日期倒序、当日 Token 从高到低排序，便于快速识别高消耗用户。</div>
            </div>
            <div className="text-[11px] text-foreground-dim">共 {fmt(aiUsage.daily_users.length)} 条聚合记录</div>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full text-xs text-left">
              <thead>
                <tr className="border-b border-border text-foreground-dim">
                  <th className="pb-2 pr-4 font-medium">日期</th>
                  <th className="pb-2 pr-4 font-medium">用户</th>
                  <th className="pb-2 pr-4 font-medium text-right">总 Token</th>
                  <th className="pb-2 pr-4 font-medium text-right">输入</th>
                  <th className="pb-2 pr-4 font-medium text-right">输出</th>
                  <th className="pb-2 pr-4 font-medium text-right">调用</th>
                  <th className="pb-2 font-medium text-right">最后一次</th>
                </tr>
              </thead>
              <tbody className="text-foreground-muted">
                {aiUsage.daily_users.map((row, i) => (
                  <tr key={`${row.date}-${row.user_id}-${i}`} className="border-b border-border last:border-0 hover:bg-[var(--color-bg-hover)]">
                    <td className="py-2 pr-4 whitespace-nowrap text-foreground-dim tabular-nums">{row.date}</td>
                    <td className="py-2 pr-4">
                      <div className="max-w-[220px] truncate" title={row.email || row.user_id}>{formatUserDisplay(row)}</div>
                      <div className="text-[10px] text-foreground-disabled font-mono">{row.user_id?.slice(0, 12)}…</div>
                    </td>
                    <td className="py-2 pr-4 text-right tabular-nums font-medium text-fuchsia-300">{fmt(row.total_tokens)}</td>
                    <td className="py-2 pr-4 text-right tabular-nums text-foreground-dim">{fmt(row.prompt_tokens)}</td>
                    <td className="py-2 pr-4 text-right tabular-nums text-foreground-dim">{fmt(row.completion_tokens)}</td>
                    <td className="py-2 pr-4 text-right tabular-nums text-foreground-dim">{fmt(row.call_count)}</td>
                    <td className="py-2 text-right text-foreground-dim whitespace-nowrap">
                      {row.last_called_at ? new Date(row.last_called_at).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }) : '-'}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      ) : null}
    </section>
  ) : null
}

// ── Quadrant Overview + Compute Logs (Panel 8 enhanced) ──

const QUADRANT_LABELS = {
  opportunity_zone: { label: '机会', color: 'text-emerald-400 bg-positive/10 border-emerald-400/25' },
  crowded_zone: { label: '拥挤', color: 'text-amber-400 bg-amber-500/10 border-amber-400/25' },
  bubble_zone: { label: '泡沫', color: 'text-negative bg-negative/10 border-rose-400/25' },
  defensive_zone: { label: '防御', color: 'text-foreground-dim bg-[var(--color-bg-hover)] border-border' },
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

export function CompanyProfilesAdminPanel({ onUnauthorized }) {
  const [loading, setLoading] = useState(false)
  const [notice, setNotice] = useState('')
  const [actionError, setActionError] = useState('')
  const resource = useAdminResource({
    key: 'admin:company-profiles',
    request: () => adminFetch('/api/admin/company-profiles'),
    staleMs: 10_000,
    minIntervalMs: 5_000,
    pollMs: 15_000,
    onUnauthorized,
    errorMessage: '加载公司资料统计失败',
  })
  const data = resource.data
  const error = resource.error

  const triggerRefresh = async () => {
    setLoading(true)
    setNotice('')
    setActionError('')
    try {
      await adminFetch('/api/admin/company-profiles/refresh', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ exchange: 'ALL' }),
      })
      setNotice('已开始刷新公司静态资料，面板会自动更新进度。')
      await resource.refresh()
    } catch (err) {
      const message = handleAdminActionError(err, onUnauthorized, '触发刷新失败')
      setNotice('')
      if (message) {
        setActionError(message)
      }
    } finally {
      setLoading(false)
    }
  }

  const refresh = data?.refresh || {}
  return (
    <section>
      <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-base font-semibold text-foreground-muted">🏢 公司资料管理</h2>
          <p className="mt-1 text-xs text-foreground-dim">覆盖率、失败项与手动刷新。刷新会从 Quant 自动采集并写入本地库。</p>
        </div>
        <button
          type="button"
          disabled={loading || refresh.running}
          onClick={triggerRefresh}
          className="rounded-lg bg-primary px-4 py-2 text-xs font-semibold text-foreground transition hover:bg-primary/85 disabled:cursor-not-allowed disabled:opacity-50"
        >
          {refresh.running ? '刷新中...' : loading ? '触发中...' : '一键更新静态资料'}
        </button>
      </div>
      {notice && <div className="mb-3 rounded-xl border border-positive/20 bg-positive/10 px-4 py-2 text-xs text-positive">{notice}</div>}
      {actionError && <div className="mb-3 rounded-xl border border-rose-400/20 bg-negative/10 px-4 py-2 text-xs text-negative">{actionError}</div>}
      {error && <div className="mb-3 rounded-xl border border-rose-400/20 bg-negative/10 px-4 py-2 text-xs text-negative">{error}</div>}
      {refresh.error && <div className="mb-3 rounded-xl border border-rose-400/20 bg-negative/10 px-4 py-2 text-xs leading-5 text-negative">刷新失败：{refresh.error}</div>}
      {refresh.message && !refresh.error && <div className="mb-3 rounded-xl border border-border bg-[var(--color-bg-hover)] px-4 py-2 text-xs text-foreground-dim">{refresh.message}</div>}
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6">
        {(data?.coverage || []).map((row) => (
          <StatCard
            key={row.exchange}
            label={`${row.exchange}覆盖`}
            value={`${Math.round((row.coverage_rate || 0) * 100)}%`}
            sub={`${row.profile_count || 0}/${row.universe_count || 0} · 行业映射 ${row.mapped_count || 0}/${row.applicable_count || 0}`}
          />
        ))}
        <StatCard label="刷新状态" value={refresh.status || 'idle'} sub={refresh.running ? `进度 ${refresh.success_count || 0}/${refresh.total_count || 0}` : (refresh.finished_at ? `完成 ${formatAdminDateTime(refresh.finished_at)}` : '等待触发')} />
        <StatCard label="新股发现" value={refresh.new_count || 0} />
        <StatCard label="退市标记" value={refresh.delisted_count || 0} />
      </div>
      <div className="mt-4 rounded-xl border border-border bg-card p-4">
        <div className="mb-3 text-xs text-foreground-dim">失败项 / 待补全（最近 30 条）</div>
        {(data?.failures || []).length === 0 ? (
          <p className="text-xs text-foreground-disabled">暂无失败项。</p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs">
              <thead><tr className="border-b border-border text-foreground-dim"><th className="pb-2 pr-3">代码</th><th className="pb-2 pr-3">名称</th><th className="pb-2 pr-3">市场</th><th className="pb-2 pr-3">状态</th><th className="pb-2">标记</th></tr></thead>
              <tbody className="text-foreground-muted">
                {data.failures.map((item) => (
                  <tr key={item.symbol} className="border-b border-border last:border-0"><td className="py-2 pr-3 font-mono">{item.symbol}</td><td className="py-2 pr-3">{item.name || '--'}</td><td className="py-2 pr-3">{item.exchange}</td><td className="py-2 pr-3">{item.profile_status}</td><td className="py-2">{(item.quality_flags || []).join(', ') || '--'}</td></tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </section>
  )
}

export function FactorLabPipelinePanel({ onUnauthorized }) {
  const [triggering, setTriggering] = useState(false)
  const [actionError, setActionError] = useState('')
  const [manualPhase, setManualPhase] = useState('all')
  const [phase0Mode, setPhase0Mode] = useState('all')
  const [manualScope, setManualScope] = useState('incremental')
  const resource = useAdminResource({
    key: 'admin:factor-lab-pipeline',
    request: () => adminFetch('/api/admin/factor-lab/pipeline/status'),
    staleMs: 5_000,
    minIntervalMs: 3_000,
    pollMs: (payload) => payload?.worker?.running ? 5_000 : null,
    onUnauthorized,
    errorMessage: '加载因子流水线状态失败',
  })
  const data = resource.data
  const worker = data?.worker || {}
  const coverage = data?.coverage || {}
  const metadata = coverage.metadata || {}
  const industriesMeta = metadata.industries || {}
  const industriesHealth = metadata.industries_health || {}
  const dividendsMeta = metadata.dividends || {}
  const industriesSummary = industriesMeta.summary || {}
  const industriesWarning = Array.isArray(industriesSummary.warnings) ? industriesSummary.warnings[0] : null
  const phases = worker.current?.phases || []
  const history = worker.history || []
  const triggerPipeline = async (override = null) => {
    const payload = override || normalizeFactorRunSelection(manualPhase, phase0Mode, manualScope)
    if (payload.error) {
      setActionError(payload.error)
      return
    }
    if (!window.confirm(`确认运行因子流水线？\n阶段：${payload.phase}\nPhase0 模式：${payload.phase0_mode}\n范围：${payload.scope}\n任务将在 backend 容器内执行，并会先做数据库健康检查和备份。`)) return
    setTriggering(true)
    setActionError('')
    try {
      await adminFetch('/api/admin/factor-lab/pipeline/run', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      })
      await resource.refresh({ force: true, preferCache: false })
    } catch (err) {
      const message = handleAdminActionError(err, onUnauthorized, '触发因子流水线失败')
      if (message) setActionError(message)
    } finally {
      setTriggering(false)
    }
  }
  const statusClass = resolveFactorStatusClass(worker.running ? 'running' : worker.current?.status || (worker.last_error ? 'failed' : 'success'))
  return (
    <section>
      <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-base font-semibold text-foreground-muted">因子实验室计算</h2>
          <p className="mt-1 text-xs text-foreground-dim">每天 21:00 在 backend 容器内串行执行 Phase0 增量（默认不含 dividends）、Phase1 快照、Phase2 因子分。</p>
        </div>
        <button
          type="button"
          disabled={triggering || worker.running}
          onClick={() => triggerPipeline({ phase: 'all', phase0_mode: 'all', scope: 'incremental' })}
          className="rounded-lg bg-primary px-4 py-2 text-xs font-semibold text-foreground transition hover:bg-primary/85 disabled:cursor-not-allowed disabled:opacity-50"
        >
          {worker.running ? '运行中...' : triggering ? '触发中...' : '运行完整流水线'}
        </button>
      </div>
      {(resource.error || actionError) && <div className="mb-3 rounded-xl border border-rose-400/20 bg-negative/10 px-4 py-2 text-xs text-negative">{actionError || resource.error}</div>}
      {worker.last_error && <div className="mb-3 rounded-xl border border-rose-400/20 bg-negative/10 px-4 py-2 text-xs text-negative">最近错误：{worker.last_error}</div>}
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6">
        <StatCard label="流水线状态" value={worker.running ? 'running' : (worker.current?.status || '--')} sub={worker.schedule ? `每日 ${worker.schedule}` : ''} />
        <StatCard label="DB 健康" value={data?.db_health || '--'} />
        <StatCard label="最新快照" value={data?.latest_snapshot_date || '--'} />
        <StatCard label="股票池" value={formatNumber(coverage.universe)} sub={coverage.snapshot_date || '--'} />
        <StatCard label="下一次运行" value={formatAdminDateTime(worker.next_run_at)} />
        <StatCard label="当前阶段" value={worker.current?.current_phase || '--'} />
      </div>
      <div className="mt-4 rounded-xl border border-border bg-card p-4">
        <div className="mb-3 text-xs text-foreground-dim">手动运行</div>
        <div className="grid gap-3 md:grid-cols-4">
          <label className="text-xs text-foreground-dim">阶段<select value={manualPhase} onChange={(e) => setManualPhase(e.target.value)} className="mt-1 w-full rounded-lg border border-border bg-background-alt px-2 py-2 text-foreground outline-none transition focus:border-primary"><option value="all">完整流水线</option><option value="phase0">只跑 Phase0</option><option value="phase1">只跑 Phase1</option><option value="phase2">只跑 Phase2</option><option value="phase1_phase2">Phase1 + Phase2</option></select></label>
          <label className="text-xs text-foreground-dim">Phase0 mode<select value={phase0Mode} onChange={(e) => setPhase0Mode(e.target.value)} disabled={!['all', 'phase0'].includes(manualPhase)} className="mt-1 w-full rounded-lg border border-border bg-background-alt px-2 py-2 text-foreground outline-none transition focus:border-primary disabled:cursor-not-allowed disabled:text-foreground-disabled disabled:opacity-40"><option value="all">all</option><option value="securities">securities</option><option value="industries">industries</option><option value="daily-bars">daily-bars</option><option value="index-bars">index-bars</option><option value="financials">financials</option><option value="dividends">dividends</option></select></label>
          <label className="text-xs text-foreground-dim">范围<select value={manualScope} onChange={(e) => setManualScope(e.target.value)} disabled={!['all', 'phase0'].includes(manualPhase)} className="mt-1 w-full rounded-lg border border-border bg-background-alt px-2 py-2 text-foreground outline-none transition focus:border-primary disabled:cursor-not-allowed disabled:text-foreground-disabled disabled:opacity-40"><option value="incremental">incremental</option><option value="repair_missing_dividend_yield">修复股息率</option><option value="repair_missing_fcfm_inputs">修复自由现金流率</option><option value="full_refresh_dividends">全量刷新股息率</option></select></label>
          <button type="button" disabled={triggering || worker.running} onClick={() => triggerPipeline()} className="self-end rounded-lg bg-primary px-4 py-2 text-xs font-semibold text-foreground transition hover:bg-primary/85 disabled:cursor-not-allowed disabled:opacity-50">{triggering ? '触发中...' : '按选择运行'}</button>
        </div>
        <div className="mt-3 flex flex-wrap gap-2 text-xs">
          <button type="button" disabled={triggering || worker.running} onClick={() => triggerPipeline({ phase: 'phase0', phase0_mode: 'dividends', scope: 'repair_missing_dividend_yield' })} className="rounded-lg border border-amber-500/25 bg-amber-50 px-3 py-1.5 text-amber-800 dark:border-amber-400/25 dark:bg-amber-500/10 dark:text-amber-100 disabled:opacity-40">只修复股息率</button>
          <button type="button" disabled={triggering || worker.running} onClick={() => triggerPipeline({ phase: 'phase0', phase0_mode: 'dividends', scope: 'full_refresh_dividends' })} className="rounded-lg border border-amber-500/25 bg-amber-50 px-3 py-1.5 text-amber-800 dark:border-amber-400/25 dark:bg-amber-500/10 dark:text-amber-100 disabled:opacity-40">全量刷新股息率</button>
          <button type="button" disabled={triggering || worker.running} onClick={() => triggerPipeline({ phase: 'phase0', phase0_mode: 'financials', scope: 'repair_missing_fcfm_inputs' })} className="rounded-lg border border-blue-400/25 bg-blue-500/10 px-3 py-1.5 text-blue-100 disabled:opacity-40">只修复自由现金流率</button>
          <button type="button" disabled={triggering || worker.running} onClick={() => triggerPipeline({ phase: 'phase0', phase0_mode: 'industries', scope: 'incremental' })} className="rounded-lg border border-primary/30 bg-primary/10 px-3 py-1.5 text-primary disabled:opacity-40">只刷新行业</button>
          <button type="button" disabled={triggering || worker.running} onClick={() => triggerPipeline({ phase: 'phase1_phase2', phase0_mode: 'all', scope: 'incremental' })} className="rounded-lg border border-emerald-400/25 bg-positive/10 px-3 py-1.5 text-emerald-100 disabled:opacity-40">只重算 Phase1+2</button>
        </div>
      </div>
      <div className="mt-4 grid gap-4 lg:grid-cols-2">
        <div className="rounded-xl border border-border bg-card p-4">
          <div className="mb-2 text-sm font-semibold text-foreground-muted">行业刷新健康度</div>
          <div className="space-y-1 text-xs text-foreground-dim">
            <div>最近状态：{industriesMeta.status || '--'}</div>
            <div>最近完成：{formatAdminDateTime(industriesMeta.finished_at)}</div>
            <div>覆盖率：{industriesHealth.universe ? `${formatNumber(industriesHealth.covered)} / ${formatNumber(industriesHealth.universe)} · ${formatPercentValue(industriesHealth.coverage_ratio)}` : '--'}</div>
            <div>最近成功刷新：{industriesHealth.last_success_at ? formatAdminDateTime(industriesHealth.last_success_at) : '--'}</div>
            <div>最近 warning：{industriesWarning?.error || '--'}</div>
            <div>口径：自动链路允许 warning 放行，不再 hard fail。</div>
          </div>
        </div>
        <div className="rounded-xl border border-border bg-card p-4">
          <div className="mb-2 text-sm font-semibold text-foreground-muted">股息率刷新策略</div>
          <div className="space-y-1 text-xs text-foreground-dim">
            <div>自动链路：默认不跑 dividends。</div>
            <div>最近状态：{dividendsMeta.status || '--'}</div>
            <div>最近完成：{formatAdminDateTime(dividendsMeta.finished_at)}</div>
            <div>建议手动时间点：年报密集披露后、半年报密集披露后、分红预案集中期、覆盖率告警时。</div>
          </div>
        </div>
      </div>
      <div className="mt-4 grid gap-3 md:grid-cols-3">
        {['phase0_incremental', 'phase1', 'phase2'].map((name) => {
          const phase = phases.find((item) => item.name === name) || { name, status: worker.running ? 'pending' : 'idle' }
          return <PhaseCard key={name} phase={phase} />
        })}
      </div>
      {coverage.warnings?.length > 0 && <div className="mt-3 rounded-xl border border-amber-500/20 bg-amber-50 px-4 py-2 text-xs text-amber-800 dark:border-amber-400/20 dark:bg-amber-500/10 dark:text-amber-100">{coverage.warnings.join('；')}</div>}
      <div className="mt-4 grid gap-4 lg:grid-cols-2">
        <CoverageTable title="原始指标覆盖率" rows={coverage.raw_metrics} total={coverage.universe} />
        <CoverageTable title="因子得分覆盖率" rows={coverage.factors} total={coverage.universe} />
      </div>
      <div className="mt-4 rounded-xl border border-border bg-card p-4">
        <div className="mb-3 flex items-center justify-between text-xs"><span className="text-foreground-dim">最近 10 次流水线运行</span><span className={`rounded-full border px-2 py-0.5 ${statusClass}`}>{worker.running ? 'running' : (worker.current?.status || 'idle')}</span></div>
        {history.length === 0 ? <p className="text-xs text-foreground-disabled">暂无流水线运行历史。</p> : (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs">
              <thead><tr className="border-b border-border text-foreground-dim"><th className="pb-2 pr-3">触发</th><th className="pb-2 pr-3">请求</th><th className="pb-2 pr-3">状态</th><th className="pb-2 pr-3">开始</th><th className="pb-2 pr-3">耗时</th><th className="pb-2">错误</th></tr></thead>
              <tbody className="text-foreground-muted">
                {history.slice(0, 10).map((run) => <tr key={run.id} className="border-b border-border last:border-0"><td className="py-2 pr-3">{run.trigger_type}</td><td className="py-2 pr-3 text-foreground-dim">{run.request?.phase || '--'} / {run.request?.phase0_mode || '--'} / {run.request?.scope || '--'}</td><td className="py-2 pr-3">{run.status}</td><td className="py-2 pr-3 whitespace-nowrap">{formatAdminDateTime(run.started_at)}</td><td className="py-2 pr-3">{formatDurationSeconds(run.duration_seconds)}</td><td className="py-2 text-negative/70">{run.error_message || '--'}</td></tr>)}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </section>
  )
}

function normalizeFactorRunSelection(phase, phase0Mode, scope) {
  const payload = { phase, phase0_mode: phase0Mode, scope }
  if (!['all', 'phase0'].includes(phase)) {
    payload.phase0_mode = 'all'
    payload.scope = 'incremental'
    return payload
  }
  if (scope === 'repair_missing_dividend_yield' && phase0Mode !== 'dividends') {
    return { ...payload, error: '修复股息率时，Phase0 mode 必须选择 dividends。' }
  }
  if (scope === 'repair_missing_fcfm_inputs' && phase0Mode !== 'financials') {
    return { ...payload, error: '修复自由现金流率时，Phase0 mode 必须选择 financials。' }
  }
  if (scope === 'full_refresh_dividends' && phase0Mode !== 'dividends') {
    return { ...payload, error: '全量刷新股息率时，Phase0 mode 必须选择 dividends。' }
  }
  return payload
}

function PhaseCard({ phase }) {
  const logs = phase.log_tail || []
  return (
    <div className="rounded-xl border border-border bg-card p-4">
      <div className="flex items-center justify-between gap-2">
        <div className="text-sm font-semibold text-foreground-muted">{phaseLabel(phase.name)}</div>
        <span className={`rounded-full border px-2 py-0.5 text-[11px] ${resolveFactorStatusClass(phase.status)}`}>{phase.status}</span>
      </div>
      <div className="mt-2 grid grid-cols-2 gap-2 text-xs text-foreground-dim">
        <span>耗时：{formatDurationSeconds(phase.duration_seconds)}</span>
        <span>总数：{formatNumber(phase.total_count)}</span>
        <span className="text-positive/75">成功：{formatNumber(phase.success_count)}</span>
        <span className="text-negative/75">失败：{formatNumber(phase.failed_count)}</span>
        <span>跳过：{formatNumber(phase.skipped_count)}</span>
      </div>
      {phase.last_message && <div className="mt-2 truncate rounded-lg bg-[var(--color-bg-hover)] px-2 py-1.5 text-[11px] text-blue-200/80" title={phase.last_message}>最新：{phase.last_message}</div>}
      {phase.error_message && <div className="mt-2 rounded-lg border border-rose-400/20 bg-negative/10 px-2 py-1.5 text-xs text-negative/80">{phase.error_message}</div>}
      <div className="mt-3 rounded-lg border border-border bg-[var(--color-bg-hover)] p-2">
        <div className="mb-1 text-[11px] text-foreground-dim">实时日志（最近 {logs.length} 行）</div>
        {logs.length === 0 ? <p className="text-[11px] text-foreground-disabled">暂无日志。</p> : <pre className="max-h-44 overflow-auto whitespace-pre-wrap break-words text-[11px] leading-5 text-foreground-dim">{logs.slice(-80).join('\n')}</pre>}
      </div>
    </div>
  )
}

function CoverageTable({ title, rows, total }) {
  const entries = Object.entries(rows || {}).sort((a, b) => a[0].localeCompare(b[0]))
  return <div className="rounded-xl border border-border bg-card p-4"><div className="mb-3 text-xs text-foreground-dim">{title}</div>{entries.length === 0 ? <p className="text-xs text-foreground-disabled">暂无数据</p> : <div className="space-y-2">{entries.map(([key, count]) => { const pct = total > 0 ? Math.round((Number(count || 0) / total) * 100) : 0; return <div key={key} className="text-xs"><div className="mb-1 flex justify-between gap-3"><span className="truncate text-foreground-muted">{factorAdminLabel(key)}</span><span className={pct < 80 ? 'text-amber-700 dark:text-amber-200' : 'text-foreground-dim'}>{formatNumber(count)} / {pct}%</span></div><div className="h-1.5 overflow-hidden rounded-full bg-[var(--color-bg-hover)]"><div className={`h-full rounded-full ${pct < 80 ? 'bg-amber-400/60' : 'bg-primary/60'}`} style={{ width: `${Math.min(100, pct)}%` }} /></div></div> })}</div>}</div>
}

function factorAdminLabel(key) {
  const labels = {
    dividend_yield: '股息率',
    performance_1y: '近一年涨幅',
    operating_cf_margin: '自由现金流率 (FCFM)',
    free_cash_flow_margin: '自由现金流率 (FCFM)',
    fcf_margin: '自由现金流率 (FCFM)',
    fcfm: '自由现金流率 (FCFM)',
    value_score: '价值',
    dividend_yield_score: '股息率因子',
    growth_score: '成长',
    quality_score: '质量',
    momentum_score: '动量',
    size_score: '规模',
    low_volatility_score: '低波动',
  }
  return labels[key] || key
}

function phaseLabel(name) {
  return { phase0_incremental: 'Phase0 增量', phase1: 'Phase1 快照', phase2: 'Phase2 因子分' }[name] || name
}

function resolveFactorStatusClass(status) {
  if (status === 'success') return 'border-emerald-400/25 bg-positive/10 text-positive'
  if (status === 'failed') return 'border-rose-400/25 bg-negative/10 text-negative'
  if (status === 'running') return 'border-blue-400/25 bg-blue-500/12 text-blue-200'
  if (status === 'partial') return 'border-amber-500/30 bg-amber-50 text-amber-800 dark:border-amber-400/25 dark:bg-amber-500/12 dark:text-amber-100'
  return 'border-[var(--color-border-strong)] bg-[var(--color-bg-hover)] text-foreground-dim'
}

function formatDurationSeconds(value) {
  const seconds = Number(value || 0)
  if (!Number.isFinite(seconds) || seconds <= 0) return '--'
  if (seconds < 60) return `${Math.round(seconds)}秒`
  return `${Math.floor(seconds / 60)}分${Math.round(seconds % 60)}秒`
}

function AIPickerTraceBlock({ label, value, emptyText = '暂无内容。' }) {
  return (
    <div className="rounded-xl border border-border bg-background-alt p-3">
      <div className="mb-2 text-[11px] font-medium uppercase tracking-[0.12em] text-foreground-dim">{label}</div>
      {value ? (
        <pre className="max-h-72 overflow-auto whitespace-pre-wrap break-words text-xs leading-6 text-foreground-muted">{value}</pre>
      ) : (
        <p className="text-xs text-foreground-disabled">{emptyText}</p>
      )}
    </div>
  )
}

export function AIPickerAdminPanel({ onUnauthorized }) {
  const [generating, setGenerating] = useState(false)
  const [actionError, setActionError] = useState('')
  const [actionSuccess, setActionSuccess] = useState('')
  const resource = useAdminResource({
    key: 'admin:ai-picker',
    request: async () => {
      const [status, latestRun] = await Promise.all([
        adminFetch('/api/admin/ai-picker/status'),
        adminFetch('/api/admin/ai-picker/latest-run'),
      ])
      return {
        status,
        latest_run: latestRun,
      }
    },
    staleMs: 5_000,
    minIntervalMs: 3_000,
    onUnauthorized,
    errorMessage: '加载 AI 选股状态失败',
  })
  const data = resource.data || {}
  const statusData = data.status || {}
  const latestRun = data.latest_run || null
  const latestResult = statusData.latest_result || null
  const latestLog = statusData.latest_log || null
  const latestRunLog = latestRun?.latest_log || latestLog
  const latestTrace = latestRun?.trace || null
  const logs = statusData.logs || []

  const handleGenerate = async () => {
    if (!window.confirm('确认立即手动生成一份 AI 优选组合吗？这会直接覆盖今日展示结果。')) return
    setGenerating(true)
    setActionError('')
    setActionSuccess('')
    try {
      await adminFetch('/api/admin/ai-picker/generate', { method: 'POST' })
      setActionSuccess('已手动生成今日 AI 优选组合')
      await resource.refresh({ force: true, preferCache: false })
    } catch (err) {
      const message = handleAdminActionError(err, onUnauthorized, '手动生成 AI 选股失败')
      if (message) setActionError(message)
    } finally {
      setGenerating(false)
    }
  }

  return (
    <section>
      <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-base font-semibold text-foreground-muted">🪄 AI 选股手动生成</h2>
          <p className="mt-1 text-xs text-foreground-dim">当每日自动结果缺失或异常时，可在这里手动重生一份共享组合，并查看最近错误日志。</p>
        </div>
        <button
          type="button"
          onClick={handleGenerate}
          disabled={generating}
          className="rounded-lg bg-primary px-4 py-2 text-xs font-semibold text-foreground transition hover:bg-primary/85 disabled:cursor-not-allowed disabled:opacity-50"
        >
          {generating ? '生成中...' : '立即手动生成'}
        </button>
      </div>
      {(resource.error || actionError) ? <div className="mb-3 rounded-xl border border-rose-400/20 bg-negative/10 px-4 py-2 text-xs text-negative">{actionError || resource.error}</div> : null}
      {actionSuccess ? <div className="mb-3 rounded-xl border border-emerald-400/20 bg-positive/10 px-4 py-2 text-xs text-positive">{actionSuccess}</div> : null}
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <StatCard label="最近交易日" value={latestResult?.trade_date || '--'} sub={latestResult?.trigger || '--'} />
        <StatCard label="最近快照" value={latestResult?.snapshot_date || latestLog?.snapshot_date || '--'} />
        <StatCard label="最近候选池" value={latestLog?.candidate_pool != null ? formatNumber(latestLog.candidate_pool) : '--'} sub={latestLog?.model || '--'} />
        <StatCard label="最近状态" value={latestLog?.status || '--'} sub={formatAdminDateTime(latestLog?.created_at)} />
      </div>
      <div className="mt-4 rounded-xl border border-border bg-card p-4">
        <div className="mb-3 text-xs text-foreground-dim">最近结果</div>
        {!latestResult ? (
          <p className="text-xs text-foreground-disabled">暂无已保存结果。</p>
        ) : (
          <div className="grid gap-3 md:grid-cols-3 text-xs text-foreground-dim">
            <div>触发方式：<span className="text-foreground-muted">{latestResult.trigger || '--'}</span></div>
            <div>模型：<span className="text-foreground-muted">{latestResult.model || '--'}</span></div>
            <div>更新时间（北京时间）：<span className="text-foreground-muted">{formatAdminDateTime(latestResult.updated_at)}</span></div>
          </div>
        )}
      </div>
      <div className="mt-4 rounded-xl border border-border bg-card p-4">
        <div className="mb-3 flex flex-wrap items-start justify-between gap-3">
          <div>
            <div className="text-xs text-foreground-dim">最近一场生成详情</div>
            <p className="mt-1 text-xs text-foreground-disabled">展示最近一次实际发送给 AI 的完整 prompt、provider 返回的 reasoning 字段，以及 AI 原始返回内容。时间均按北京时间展示。</p>
          </div>
          <div className="text-right text-xs text-foreground-dim">
            <div>最近生成：<span className="text-foreground-muted">{formatAdminDateTime(latestRunLog?.created_at)}</span></div>
            <div className="mt-1">状态：<span className={latestRunLog?.status === 'failed' ? 'text-negative' : 'text-positive'}>{latestRunLog?.status || '--'}</span></div>
          </div>
        </div>
        {!latestRunLog ? (
          <p className="text-xs text-foreground-disabled">暂无最近一次生成记录。</p>
        ) : (
          <>
            <div className="mb-4 grid gap-3 text-xs text-foreground-dim md:grid-cols-4">
              <div>触发方式：<span className="text-foreground-muted">{latestRunLog.trigger || '--'}</span></div>
              <div>快照日期：<span className="text-foreground-muted">{latestRunLog.snapshot_date || '--'}</span></div>
              <div>模型：<span className="text-foreground-muted">{latestRunLog.model || '--'}</span></div>
              <div>完成原因：<span className="text-foreground-muted">{latestRunLog.finish_reason || '--'}</span></div>
            </div>
            <div className="grid gap-3 lg:grid-cols-2">
              <AIPickerTraceBlock label="System Prompt" value={latestTrace?.system_prompt} emptyText="未记录 system prompt。" />
              <AIPickerTraceBlock label="User Prompt" value={latestTrace?.user_prompt} emptyText="未记录 user prompt。" />
              <AIPickerTraceBlock label="AI 思考 / 推理过程" value={latestTrace?.assistant_reasoning} emptyText="当前 provider 未返回 reasoning 字段。" />
              <AIPickerTraceBlock label="AI 原始返回内容" value={latestTrace?.assistant_content} emptyText="未记录 AI 原始返回内容。" />
            </div>
          </>
        )}
      </div>
      <div className="mt-4 rounded-xl border border-border bg-card p-4">
        <div className="mb-3 text-xs text-foreground-dim">最近 10 条生成日志（北京时间）</div>
        {logs.length === 0 ? (
          <p className="text-xs text-foreground-disabled">暂无生成日志。</p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs">
              <thead>
                <tr className="border-b border-border text-foreground-dim">
                  <th className="pb-2 pr-3">时间</th>
                  <th className="pb-2 pr-3">状态</th>
                  <th className="pb-2 pr-3">触发</th>
                  <th className="pb-2 pr-3">快照</th>
                  <th className="pb-2 pr-3">候选池</th>
                  <th className="pb-2 pr-3">模型</th>
                  <th className="pb-2">日志</th>
                </tr>
              </thead>
              <tbody className="text-foreground-muted">
                {logs.map((item) => (
                  <tr key={item.id} className="border-b border-border last:border-0 align-top">
                    <td className="py-2 pr-3 whitespace-nowrap">{formatAdminDateTime(item.created_at)}</td>
                    <td className={`py-2 pr-3 ${item.status === 'failed' ? 'text-negative' : 'text-positive'}`}>{item.status}</td>
                    <td className="py-2 pr-3">{item.trigger || '--'}</td>
                    <td className="py-2 pr-3">{item.snapshot_date || '--'}</td>
                    <td className="py-2 pr-3">{item.candidate_pool ? formatNumber(item.candidate_pool) : '--'}</td>
                    <td className="py-2 pr-3">{item.model || '--'}</td>
                    <td className="py-2 break-words whitespace-pre-wrap text-foreground-dim">{item.message || '--'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </section>
  )
}

export function QuadrantAdminPanel({ onUnauthorized }) {
  const [expandedLog, setExpandedLog] = useState(null)
  const [triggering, setTriggering] = useState(false)
  const [syncingPortfolio, setSyncingPortfolio] = useState(false)
  const [backfillingOpenPrices, setBackfillingOpenPrices] = useState(false)
  const [verifyingPortfolio, setVerifyingPortfolio] = useState(false)
  const [verifyPortfolioResult, setVerifyPortfolioResult] = useState(null)
  const [recomputingPortfolio, setRecomputingPortfolio] = useState(false)
  const [recomputeConfirming, setRecomputeConfirming] = useState(false)
  const [portfolioActionNotice, setPortfolioActionNotice] = useState('')
  const [actionError, setActionError] = useState('')
  const resource = useAdminResource({
    key: 'admin:quadrant',
    request: async () => {
      const [overview, logsPayload, progress, portfolioTrackingStatus] = await Promise.all([
        adminFetch('/api/admin/quadrant-overview').catch(() => null),
        adminFetch('/api/admin/quadrant-logs').catch(() => ({ items: [] })),
        adminFetch('/api/admin/compute-status').catch(() => null),
        adminFetch('/api/admin/portfolio-tracking/status').catch(() => ({ items: [] })),
      ])
      return {
        overview,
        logs: logsPayload?.items || [],
        progress,
        portfolioTrackingStatus: portfolioTrackingStatus?.items || [],
      }
    },
    staleMs: 5_000,
    minIntervalMs: 3_000,
    pollMs: (payload) => {
      const anyRunning = Object.values(payload?.progress || {}).some((item) => item?.status === 'running')
      return anyRunning ? 5_000 : null
    },
    onUnauthorized,
    errorMessage: '加载四象限数据失败',
  })
  const overview = resource.data?.overview || null
  const logs = resource.data?.logs || null
  const progress = resource.data?.progress || null
  const portfolioTrackingStatus = resource.data?.portfolioTrackingStatus || []

  // ── Manual trigger ──
  const handleTrigger = async (exchange) => {
    setTriggering(true)
    setActionError('')
    try {
      await adminFetch('/api/admin/quadrant-trigger', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ exchange }),
      })
      await resource.refresh()
    } catch (err) {
      const message = handleAdminActionError(err, onUnauthorized, '触发四象限计算失败')
      if (message) {
        setActionError(message)
      }
    } finally {
      setTriggering(false)
    }
  }

  const handlePortfolioTrackingSync = async () => {
    setSyncingPortfolio(true)
    setActionError('')
    setPortfolioActionNotice('')
    try {
      const resp = await adminFetch('/api/admin/portfolio-tracking/sync', { method: 'POST' })
      setVerifyPortfolioResult(null)
      setPortfolioActionNotice(resp?.message || '模拟组合事实表已同步最新信号。')
      await resource.refresh()
    } catch (err) {
      const message = handleAdminActionError(err, onUnauthorized, '同步模拟组合事实表失败')
      if (message) setActionError(message)
    } finally {
      setSyncingPortfolio(false)
    }
  }

  const handlePortfolioTrackingBackfillOpenPrices = async () => {
    setBackfillingOpenPrices(true)
    setActionError('')
    setPortfolioActionNotice('')
    try {
      const resp = await adminFetch('/api/admin/portfolio-tracking/backfill-open-prices', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ latest_only: true }),
      })
      setPortfolioActionNotice(resp?.message || '建仓开盘价补齐完成。')
      await resource.refresh()
    } catch (err) {
      const message = handleAdminActionError(err, onUnauthorized, '补齐建仓开盘价失败')
      if (message) setActionError(message)
    } finally {
      setBackfillingOpenPrices(false)
    }
  }

  const handlePortfolioTrackingVerify = async () => {
    setVerifyingPortfolio(true)
    setActionError('')
    setPortfolioActionNotice('')
    try {
      const resp = await adminFetch('/api/admin/portfolio-tracking/verify', { method: 'POST' })
      setVerifyPortfolioResult(resp)
      const diffCount = Array.isArray(resp?.items) ? resp.items.filter((item) => item.status !== 'ok').length : 0
      setPortfolioActionNotice(diffCount > 0 ? `发现 ${diffCount} 条需要人工核查的事实表记录。` : '所有事实表记录校验通过。')
      await resource.refresh()
    } catch (err) {
      const message = handleAdminActionError(err, onUnauthorized, '验证模拟组合事实表失败')
      if (message) setActionError(message)
    } finally {
      setVerifyingPortfolio(false)
    }
  }

  const handlePortfolioTrackingRecompute = async () => {
    if (!recomputeConfirming) {
      setRecomputeConfirming(true)
      return
    }
    setRecomputeConfirming(false)
    setRecomputingPortfolio(true)
    setActionError('')
    setPortfolioActionNotice('')
    try {
      const resp = await adminFetch('/api/admin/portfolio-tracking/recompute', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ reset: true }),
      })
      setVerifyPortfolioResult(null)
      setPortfolioActionNotice(resp?.message || '模拟组合已从头重算。')
      await resource.refresh()
    } catch (err) {
      const message = handleAdminActionError(err, onUnauthorized, '从头重算模拟组合失败')
      if (message) setActionError(message)
    } finally {
      setRecomputingPortfolio(false)
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
    const isSkipped = p.status === 'skipped'
    const pct = Math.min(p.percent || 0, 100).toFixed(1)
    const statusIcon = isSuccess ? '✅' : isFailed ? '❌' : isTimeout ? '⏰' : isSkipped ? '⏭️' : isRunning ? '🔄' : '💤'
    const statusLabel = isSuccess ? '已完成' : isFailed ? '失败' : isTimeout ? '超时' : isSkipped ? '已跳过' : isRunning ? '计算中...' : '空闲'
    const elapsed = p.updated_at ? formatTimeAgo(p.updated_at) : ''
    const barColor = isSuccess ? 'bg-emerald-500' : isFailed ? 'bg-rose-500' : isTimeout ? 'bg-amber-500' : isSkipped ? 'bg-slate-400' : 'bg-blue-500'
    const barPulse = isRunning ? 'animate-pulse' : ''

    return (
      <div className="rounded-xl border border-border bg-card p-4">
        <div className="flex items-center justify-between mb-2">
          <span className="text-sm font-semibold text-foreground-muted">{label} 四象限</span>
          <span className="flex items-center gap-1.5 text-xs font-medium">
            <span>{statusIcon}</span>
            <span className={isSuccess ? 'text-emerald-400' : isFailed ? 'text-negative' : isTimeout ? 'text-amber-400' : isSkipped ? 'text-foreground-dim' : isRunning ? 'text-blue-400' : 'text-foreground-dim'}>
              {statusLabel}
            </span>
          </span>
        </div>
        {/* Progress bar */}
        <div className="w-full h-2 bg-[var(--color-bg-secondary)] rounded-full overflow-hidden mb-2">
          <div
            className={`h-full rounded-full transition-all duration-700 ease-out ${barColor} ${barPulse}`}
            style={{ width: `${isRunning ? Math.max(pct, 2) : (isSuccess ? 100 : 0)}%` }}
          />
        </div>
        {/* 阶段消息（running/skipped + 有 message 时显示） */}
        {(isRunning || isSkipped) && p.message && (
          <div className={`text-[11px] mb-1 truncate ${isSkipped ? 'text-foreground-dim' : 'text-blue-300/70'}`} title={p.message}>
            {p.message}
          </div>
        )}
        <div className="flex items-center justify-between text-[11px] text-foreground-dim">
          <span>
            {isRunning && p.total > 0 ? `${p.current.toLocaleString()} / ${p.total.toLocaleString()} (${pct}%)` :
             isRunning && !p.message ? '准备中...' :
             isSuccess && p.total > 0 ? `${p.total.toLocaleString()} 只 · 已落库` :
             isFailed ? (p.error_msg || '数据未写入后端（回调失败）') :
             isTimeout ? '计算超时' :
             isSkipped ? (p.message || '今日非交易日，已跳过自动执行') :
             '--'}
          </span>
          <span>{elapsed}</span>
        </div>
      </div>
    )
  }

  const portfolioVerifyDiffs = Array.isArray(verifyPortfolioResult?.items)
    ? verifyPortfolioResult.items.filter((item) => item.status !== 'ok')
    : []
  const getPortfolioTrackingStatusClass = (status) => {
    switch (status) {
      case 'completed':
        return 'text-positive'
      case 'pending_open_price':
      case 'pending_close_price':
      case 'seeded':
        return 'text-amber-700'
      case 'shortfall':
      case 'failed':
        return 'text-negative'
      default:
        return 'text-foreground-muted'
    }
  }

  if (!overview && !logs) return null

  return (
    <section>
      <div className="mb-3">
        <h2 className="text-base font-semibold text-foreground-muted">🔲 四象限数据总览</h2>
        <p className="mt-1 text-xs text-foreground-dim">自动执行时间为北京时间每日 20:00；按 A 股与中国香港交易日分别判断，非交易日自动跳过。</p>
      </div>
      {(resource.error || actionError) && (
        <div className="mb-3 rounded-xl border border-rose-400/20 bg-negative/10 px-4 py-2 text-xs text-negative">
          {actionError || resource.error}
        </div>
      )}

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
              className="px-3 py-1.5 rounded-lg text-xs font-medium bg-blue-600 hover:bg-blue-500 disabled:bg-blue-900/50 disabled:text-foreground-dim text-foreground transition cursor-pointer disabled:cursor-not-allowed"
            >
              🔄 立即计算 A 股
            </button>
            <button
              onClick={() => handleTrigger('HKEX')}
              disabled={triggering || progress.HKEX?.status === 'running'}
              className="px-3 py-1.5 rounded-lg text-xs font-medium bg-purple-600 hover:bg-purple-500 disabled:bg-purple-900/50 disabled:text-foreground-dim text-foreground transition cursor-pointer disabled:cursor-not-allowed"
            >
              🔄 立即计算港股
            </button>
            <button
              onClick={() => resource.refresh()}
              className="ml-auto px-3 py-1.5 rounded-lg text-xs font-medium border border-border hover:border-[var(--color-border-strong)] text-foreground-dim hover:text-foreground-muted transition"
            >
              刷新
            </button>
          </div>
        </div>
      )}

      {/* 新口径模拟组合管理 */}
      <div className="mb-5 rounded-xl border border-border bg-card p-4">
        <div className="mb-3 flex flex-wrap items-start justify-between gap-3">
          <div>
            <h3 className="text-sm font-semibold text-foreground-muted">新口径模拟组合管理</h3>
            <p className="mt-1 text-xs text-foreground-dim">`/portfolio-tracking` 已切到事实表链路。自动同步挂在四象限 daily bulk-save 成功之后，读取接口不再承担写库职责。</p>
          </div>
          <div className="flex flex-wrap items-center justify-end gap-2">
            <button
              onClick={handlePortfolioTrackingBackfillOpenPrices}
              disabled={backfillingOpenPrices || syncingPortfolio || recomputingPortfolio}
              className="rounded-lg border border-border bg-background px-3 py-1.5 text-xs font-medium text-foreground hover:border-[var(--color-border-strong)] hover:bg-background-alt disabled:cursor-not-allowed disabled:opacity-60"
            >
              {backfillingOpenPrices ? '补齐中…' : '补齐建仓开盘价'}
            </button>
            <button
              onClick={handlePortfolioTrackingSync}
              disabled={syncingPortfolio || recomputingPortfolio || backfillingOpenPrices}
              className="rounded-lg border border-border bg-background px-3 py-1.5 text-xs font-medium text-foreground hover:border-[var(--color-border-strong)] hover:bg-background-alt disabled:cursor-not-allowed disabled:opacity-60"
            >
              {syncingPortfolio ? '同步中…' : '同步最新事实表'}
            </button>
            <button
              onClick={handlePortfolioTrackingVerify}
              disabled={verifyingPortfolio || syncingPortfolio || recomputingPortfolio || backfillingOpenPrices}
              className="rounded-lg border border-border bg-background px-3 py-1.5 text-xs font-medium text-foreground hover:border-[var(--color-border-strong)] hover:bg-background-alt disabled:cursor-not-allowed disabled:opacity-60"
            >
              {verifyingPortfolio ? '验证中…' : '验证事实表一致性'}
            </button>
            {recomputeConfirming ? (
              <div className="flex flex-wrap items-center gap-2">
                <span className="text-xs text-amber-700">确认从头重算全部新口径组合？</span>
                <button
                  onClick={handlePortfolioTrackingRecompute}
                  disabled={recomputingPortfolio}
                  className="rounded-lg border border-amber-500/40 bg-amber-50 px-3 py-1.5 text-xs font-medium text-amber-800 transition hover:bg-amber-100 disabled:cursor-not-allowed disabled:opacity-60"
                >
                  确认重算
                </button>
                <button
                  onClick={() => setRecomputeConfirming(false)}
                  className="rounded-lg border border-border px-3 py-1.5 text-xs text-foreground-dim hover:bg-background-alt"
                >
                  取消
                </button>
              </div>
            ) : (
              <button
                onClick={handlePortfolioTrackingRecompute}
                disabled={recomputingPortfolio || syncingPortfolio || backfillingOpenPrices}
                className="rounded-lg border border-amber-500/30 bg-amber-50 px-3 py-1.5 text-xs font-medium text-amber-800 transition hover:bg-amber-100 disabled:cursor-not-allowed disabled:opacity-60"
              >
                {recomputingPortfolio ? '重算中…' : '从头重算全部组合'}
              </button>
            )}
          </div>
        </div>

        <div className="mb-3 rounded-xl border border-dashed border-border bg-background px-4 py-3 text-xs leading-6 text-foreground-muted">
          旧版「补齐开盘价并重算曲线（仅上线日后）」与「验证收益曲线」只服务 legacy JSON 结果表。
          当前主站页面已不再依赖那套结果，管理后台不再展示旧按钮；相关后端接口暂时保留，仅用于历史核查。
        </div>

        {portfolioActionNotice ? (
          <div className="mb-3 rounded-lg border border-positive/20 bg-positive/10 px-3 py-2 text-xs text-positive">
            {portfolioActionNotice}
          </div>
        ) : null}

        {portfolioTrackingStatus.length === 0 ? (
          <p className="text-xs text-foreground-dim">暂无新口径模拟组合状态。</p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-xs text-left">
              <thead>
                <tr className="border-b border-border text-foreground-dim">
                  <th className="pb-2 pr-4 font-medium">组合</th>
                  <th className="pb-2 pr-4 font-medium">市场</th>
                  <th className="pb-2 pr-4 font-medium">最新信号日</th>
                  <th className="pb-2 pr-4 font-medium">待执行信号</th>
                  <th className="pb-2 pr-4 font-medium">下一次开盘</th>
                  <th className="pb-2 pr-4 font-medium">最新估值日</th>
                  <th className="pb-2 pr-4 font-medium">缺开盘价</th>
                  <th className="pb-2 font-medium">状态</th>
                </tr>
              </thead>
              <tbody className="text-foreground-muted">
                {portfolioTrackingStatus.map((item) => (
                  <tr key={item.portfolio_id} className="border-b border-border last:border-0 align-top">
                    <td className="py-2 pr-4 font-medium text-foreground">{item.name}</td>
                    <td className="py-2 pr-4">{item.exchange === 'HKEX' ? '中国香港' : 'A股'}</td>
                    <td className="py-2 pr-4 tabular-nums">{item.latest_signal_date || '--'}</td>
                    <td className="py-2 pr-4 tabular-nums">{item.pending_signal_date || '--'}</td>
                    <td className="py-2 pr-4 tabular-nums">{item.next_entry_trade_date || '--'}</td>
                    <td className="py-2 pr-4 tabular-nums">{item.latest_trade_date || '--'}</td>
                    <td className="py-2 pr-4 tabular-nums">{item.missing_open_price_count > 0 ? <span className="text-amber-700">{item.missing_open_price_count}</span> : (item.missing_open_price_count === 0 ? '0' : '--')}</td>
                    <td className={`py-2 font-medium ${getPortfolioTrackingStatusClass(item.status)}`}>{item.status_text || item.status || '--'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}

        {verifyPortfolioResult ? (
          portfolioVerifyDiffs.length > 0 ? (
            <div className="mt-4 rounded-xl border border-negative/20 bg-negative/10 px-4 py-3">
              <div className="text-xs font-medium text-negative">发现 {portfolioVerifyDiffs.length} 条异常记录</div>
              <div className="mt-3 overflow-x-auto">
                <table className="w-full text-[11px] text-left">
                  <thead>
                    <tr className="border-b border-border text-foreground-dim">
                      <th className="pb-2 pr-3 font-medium">组合</th>
                      <th className="pb-2 pr-3 font-medium">日期</th>
                      <th className="pb-2 pr-3 font-medium">状态</th>
                      <th className="pb-2 pr-3 font-medium text-right">总资产</th>
                      <th className="pb-2 pr-3 font-medium text-right">持仓汇总</th>
                      <th className="pb-2 font-medium">说明</th>
                    </tr>
                  </thead>
                  <tbody className="text-foreground-muted">
                    {portfolioVerifyDiffs.map((item) => (
                      <tr key={`${item.portfolio_id}-${item.trade_date}-${item.status}`} className="border-b border-border last:border-0">
                        <td className="py-2 pr-3">{item.portfolio_id}</td>
                        <td className="py-2 pr-3 tabular-nums">{item.trade_date}</td>
                        <td className="py-2 pr-3 font-medium text-negative">{item.status}</td>
                        <td className="py-2 pr-3 text-right tabular-nums">{item.total_assets?.toFixed?.(2) || '--'}</td>
                        <td className="py-2 pr-3 text-right tabular-nums">{item.position_assets?.toFixed?.(2) || '--'}</td>
                        <td className="py-2">{item.message || '--'}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          ) : (
            <div className="mt-3 rounded-lg border border-positive/20 bg-positive/10 px-3 py-2 text-xs text-positive">
              ✅ 所有事实表记录一致。
            </div>
          )
        ) : null}
      </div>

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
              <div key={ex.exchange} className="rounded-xl border border-border bg-card p-4">
                <div className="flex items-center justify-between mb-3">
                  <span className="text-sm font-semibold text-foreground-muted">{ex.exchange} 象限分布</span>
                  <span className="text-xs text-foreground-dim">{ex.total_count.toLocaleString()} 只</span>
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
                        <span className="text-foreground-disabled ml-0.5">{Math.round(count / total * 100)}%</span>
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
          <h3 className="text-sm font-medium text-foreground-dim">计算历史</h3>
          {!logs && <span className="text-xs text-foreground-disabled">加载中…</span>}
        </div>
        {!logs || logs.length === 0 ? (
          <p className="text-xs text-foreground-dim">暂无计算记录</p>
        ) : (
          <div className="space-y-1.5">
            {logs.slice(0, 15).map((log) => {
              const report = (() => { try { return JSON.parse(log.ReportJSON || '{}') } catch { return {} } })()
              const isExp = expandedLog === log.ID
              const statusColor = log.Status === 'success' ? 'text-emerald-400' : log.Status === 'failed' ? 'text-negative' : 'text-amber-400'
              const qc = report.quadrant_counts || {}
              return (
                <div key={log.ID} className="rounded-lg border border-border bg-card px-3 py-2">
                  <div
                    className="flex flex-wrap items-center gap-x-4 gap-y-1 text-[11px] cursor-pointer hover:bg-[var(--color-bg-hover)] rounded transition"
                    onClick={() => setExpandedLog(isExp ? null : log.ID)}
                  >
                    <span className="text-foreground-dim tabular-nums">{new Date(log.ComputedAt).toLocaleString('zh-CN')}</span>
                    <span className={`font-medium ${statusColor}`}>{log.Status}</span>
                    <span className="text-foreground-dim">{log.Mode}</span>
                    <span className="text-foreground-dim">{log.StockCount} 只</span>
                    <span className="text-foreground-dim">{log.DurationSec.toFixed(0)}s</span>
                    {Object.keys(qc).length > 0 && (
                      <span className="text-foreground-disabled hidden sm:inline">
                        机:{qc['机会']||0}/挤:{qc['拥挤']||0}/泡:{qc['泡沫']||0}/防:{qc['防御']||0}/中:{qc['中性']||0}
                      </span>
                    )}
                    <span className="ml-auto text-foreground-disabled">{isExp ? '▼' : '▶'}</span>
                  </div>
                  {isExp && (
                    <pre className="mt-2 max-h-56 overflow-auto rounded bg-[var(--color-bg-overlay)] p-2 text-[10px] leading-relaxed text-foreground-dim font-mono">
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
const FB_STATUS_COLORS = { pending: 'text-amber-300 bg-amber-500/10 border-amber-400/30', resolved: 'text-positive bg-positive/10 border-positive/30', dismissed: 'text-foreground-dim bg-[var(--color-bg-hover)] border-border' }

export function FeedbackPanel({ onUnauthorized }) {
  const [updating, setUpdating] = useState(null)
  const [actionError, setActionError] = useState('')
  const resource = useAdminResource({
    key: 'admin:feedback',
    request: () => adminFetch('/api/admin/feedback?limit=50'),
    staleMs: 30_000,
    minIntervalMs: 3_000,
    onUnauthorized,
    errorMessage: '加载反馈列表失败',
  })
  const data = resource.data || { items: [], total: 0, stats: null }

  if (resource.loading && !resource.data) return null

  const stats = data.stats
  const items = data.items || []

  const handleUpdateStatus = async (id, status) => {
    setUpdating(id)
    setActionError('')
    try {
      await adminFetch(`/api/admin/feedback/${id}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ status }),
      })
      await resource.refresh()
    } catch (err) {
      const message = handleAdminActionError(err, onUnauthorized, '更新反馈状态失败')
      if (message) {
        setActionError(message)
      }
    } finally {
      setUpdating(null)
    }
  }

  return (
    <section>
      <h2 className="text-base font-semibold text-foreground-muted mb-3">💬 用户反馈</h2>
      {resource.error ? (
        <div className="mb-3 rounded-xl border border-rose-400/20 bg-negative/10 px-4 py-2 text-xs text-negative">
          {resource.error}
        </div>
      ) : null}
      {actionError ? (
        <div className="mb-3 rounded-xl border border-rose-400/20 bg-negative/10 px-4 py-2 text-xs text-negative">
          {actionError}
        </div>
      ) : null}
      {stats ? (
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 mb-4">
          <StatCard label="总反馈" value={stats.total} />
          <StatCard label="待处理" value={stats.pending} />
          <StatCard label="Bug" value={stats.bug_count} />
          <StatCard label="建议+许愿" value={(stats.feature_count || 0) + (stats.wish_count || 0)} />
        </div>
      ) : null}
      {items.length === 0 ? (
        <p className="text-xs text-foreground-dim">暂无用户反馈</p>
      ) : (
        <div className="space-y-2">
          {items.map((item) => (
            <div key={item.id} className="rounded-lg border border-border bg-card px-4 py-3">
              <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs">
                <span className="font-medium text-foreground-muted">{FB_CATEGORY_LABELS[item.category] || item.category}</span>
                <span className={`inline-flex rounded-full border px-2 py-0.5 text-[10px] font-medium ${FB_STATUS_COLORS[item.status] || FB_STATUS_COLORS.pending}`}>
                  {FB_STATUS_LABELS[item.status] || item.status}
                </span>
                <span className="text-foreground-dim">{new Date(item.created_at).toLocaleString('zh-CN')}</span>
                <span className="text-foreground-dim">{item.user_email || item.user_id}</span>
              </div>
              <div className="mt-2 text-sm leading-7 text-foreground-muted whitespace-pre-wrap">{item.content}</div>
              {item.contact ? (
                <div className="mt-1 text-xs text-foreground-dim">联系方式：{item.contact}</div>
              ) : null}
              {item.status === 'pending' ? (
                <div className="mt-2 flex gap-2">
                  <button
                    type="button"
                    disabled={updating === item.id}
                    onClick={() => handleUpdateStatus(item.id, 'resolved')}
                    className="rounded-lg border border-positive/30 bg-positive/10 px-2.5 py-1 text-[11px] font-medium text-positive transition hover:bg-emerald-500/20 disabled:opacity-50"
                  >
                    标记已处理
                  </button>
                  <button
                    type="button"
                    disabled={updating === item.id}
                    onClick={() => handleUpdateStatus(item.id, 'dismissed')}
                    className="rounded-lg border border-border bg-[var(--color-bg-hover)] px-2.5 py-1 text-[11px] font-medium text-foreground-dim transition hover:bg-[var(--color-bg-hover)] disabled:opacity-50"
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

function statusColor(code) {
  if (code >= 500) return 'text-negative bg-negative/10 border-rose-400/25'
  return 'text-amber-300 bg-amber-500/10 border-amber-400/25'
}

function formatMS(ms) {
  if (ms == null) return '--'
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(1)}s`
}

export function SystemHealthPanel({ onUnauthorized }) {
  const [logsExpanded, setLogsExpanded] = useState(false)
  const [logsData, setLogsData] = useState(null)
  const [actionError, setActionError] = useState('')
  const resource = useAdminResource({
    key: 'admin:system-health',
    request: () => adminFetch('/api/admin/system-health'),
    staleMs: 20_000,
    minIntervalMs: 5_000,
    pollMs: 60_000,
    onUnauthorized,
    errorMessage: '获取系统健康数据失败',
  })
  const data = resource.data

  const loadMoreLogs = async () => {
    setActionError('')
    try {
      const d = await adminFetch('/api/admin/system-health/logs?limit=200&offset=0')
      setLogsData(d)
      setLogsExpanded(true)
    } catch (err) {
      const message = handleAdminActionError(err, onUnauthorized, '加载错误日志失败')
      if (message) {
        setActionError(message)
      }
    }
  }

  const handlePurge = async () => {
    if (!window.confirm('确定要清理历史错误日志吗？（保留最近 30 天）')) return
    setActionError('')
    try {
      await adminFetch('/api/admin/system-health/purge', { method: 'POST' })
      await resource.refresh()
      setLogsData(null)
      setLogsExpanded(false)
    } catch (err) {
      const message = handleAdminActionError(err, onUnauthorized, '清理历史错误日志失败')
      if (message) {
        setActionError(message)
      }
    }
  }

  if (resource.loading && !data) return null

  const summary = data?.error_summary || {}
  const topEndpoints = data?.top_error_endpoints || []
  const recentErrors = data?.recent_errors || []
  const trends = data?.error_trends || []

  return (
    <section>
      <h2 className="text-base font-semibold text-foreground-muted mb-3">🖥️ 系统健康（错误监控）</h2>
      {resource.error ? (
        <div className="mb-3 rounded-xl border border-rose-400/20 bg-negative/10 px-4 py-2 text-xs text-negative">
          {resource.error}
        </div>
      ) : null}
      {actionError ? (
        <div className="mb-3 rounded-xl border border-rose-400/20 bg-negative/10 px-4 py-2 text-xs text-negative">
          {actionError}
        </div>
      ) : null}

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
            <div className="rounded-xl border border-border bg-card p-3">
              <MiniChart data={trends} label="错误趋势（14天）" width={380} height={130} type="bar" color="#ef4444" />
            </div>
          )}
          {/* Top Error Endpoints */}
          {topEndpoints.length > 0 && (
            <div className="rounded-xl border border-border bg-card p-4">
              <div className="text-xs text-foreground-dim mb-3">Top 出错接口（今日）</div>
              <div className="space-y-2">
                {topEndpoints.slice(0, 8).map((ep, i) => (
                  <div key={`${ep.path}-${ep.method}`} className="flex items-center gap-3 text-sm">
                    <span className={`text-[11px] font-mono px-1.5 py-0.5 rounded border ${ep.count > 20 ? statusColor(500) : statusColor(400)}`}>
                      {ep.method}
                    </span>
                    <span className="w-44 truncate text-foreground-muted text-xs font-mono">{ep.path}</span>
                    <div className="flex-1 h-4 rounded bg-[var(--color-bg-hover)] overflow-hidden">
                      <div
                        className="h-full rounded bg-rose-500/30"
                        style={{ width: `${Math.min((ep.count / (topEndpoints[0].count || 1)) * 100, 100)}%` }}
                      />
                    </div>
                    <span className="text-xs text-foreground-dim tabular-nums w-8 text-right">{ep.count}</span>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      )}

      {/* Recent Errors Table */}
      {recentErrors.length > 0 ? (
        <div className="mt-4 rounded-xl border border-border bg-card overflow-hidden">
          <div className="flex items-center justify-between px-4 py-2.5 border-b border-border">
            <div className="text-xs text-foreground-dim">
              最近报错日志（{data?.generated_at ? `更新于 ${new Date(data.generated_at).toLocaleTimeString('zh-CN')}` : ''}）
            </div>
            <div className="flex gap-2">
              {!logsExpanded && (
                <button
                  type="button"
                  onClick={loadMoreLogs}
                  className="rounded-lg border border-[var(--color-border-strong)] bg-[var(--color-bg-hover)] px-2.5 py-1 text-[11px] text-foreground-muted transition hover:bg-[var(--color-bg-hover)] hover:text-foreground"
                >
                  展开全部
                </button>
              )}
              <button
                type="button"
                onClick={handlePurge}
                className="rounded-lg border border-rose-400/20 bg-rose-500/8 px-2.5 py-1 text-[11px] text-negative transition hover:bg-negative/15"
              >
                清理旧数据
              </button>
            </div>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full text-xs text-left">
              <thead>
                <tr className="border-b border-border text-foreground-dim">
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
              <tbody className="text-foreground-muted">
                {(logsExpanded && logsData ? logsData.items : recentErrors).map((err) => (
                  <tr key={err.id} className="border-b border-border last:border-0 hover:bg-[var(--color-bg-hover)]">
                    <td className="py-1.5 pl-4 pr-3 whitespace-nowrap text-foreground-dim">
                      {new Date(err.created_at).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })}
                    </td>
                    <td className="py-1.5 px-3">
                      <span className="font-mono text-[11px] text-foreground-dim">{err.method}</span>
                    </td>
                    <td className="py-1.5 px-3 max-w-[220px]">
                      <span className="font-mono text-[11px] truncate block" title={err.path}>{err.path}</span>
                    </td>
                    <td className="py-1.5 px-3 text-center">
                      <span className={`inline-flex rounded-md border px-1.5 py-0.5 text-[11px] font-medium ${statusColor(err.status_code)}`}>
                        {err.status_code}
                      </span>
                    </td>
                    <td className="py-1.5 px-3 font-mono text-[11px] text-foreground-dim">{err.error_code || '-'}</td>
                    <td className="py-1.5 px-3 max-w-[240px] truncate text-foreground-dim" title={err.error_message}>{err.error_message || '-'}</td>
                    <td className="py-1.5 px-3 text-right tabular-nums text-foreground-dim whitespace-nowrap">{formatMS(err.duration_ms)}</td>
                    <td className="py-1.5 pr-4 pl-3 text-right text-foreground-disabled font-mono text-[11px]" title={err.client_ip}>
                      {err.client_ip ? err.client_ip.split('.').slice(0, 2).join('.') + '.*' : '-'}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      ) : (
        <div className="mt-3 text-xs text-foreground-dim p-3 rounded-xl border border-dashed border-border text-center">
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

export function UserFunnelPanel({ onUnauthorized }) {
  const resource = useAdminResource({
    key: 'admin:user-funnel',
    request: () => adminFetch('/api/admin/user-funnel'),
    staleMs: 60_000,
    minIntervalMs: 10_000,
    onUnauthorized,
    errorMessage: '获取用户漏斗数据失败',
  })
  const data = resource.data

  if (resource.loading && !data) return null
  if (resource.error && !data) {
    return (
      <section>
        <h2 className="text-base font-semibold text-foreground-muted mb-3">📊 用户转化漏斗</h2>
        <div className="rounded-xl border border-rose-400/20 bg-negative/10 px-4 py-3 text-sm text-negative">
          {resource.error}
        </div>
      </section>
    )
  }
  const steps = data?.steps || []
  if (steps.length === 0) return null

  const maxAll = Math.max(...steps.map(s => s.count_all), 1)

  return (
    <section>
      <h2 className="text-base font-semibold text-foreground-muted mb-3">📊 用户转化漏斗</h2>

      {/* Funnel Visualization */}
      <div className="rounded-xl border border-border bg-card p-5">
        <div className="flex flex-col gap-2">
          {steps.map((step, i) => {
            const w = Math.max((step.count_all / maxAll) * 100, i === 0 ? 4 : 2)
            const prev = i > 0 ? steps[i - 1].count_all : step.count_all
            return (
              <div key={step.label} className="flex items-center gap-3">
                {/* Label */}
                <div className="w-20 text-right text-xs font-medium text-foreground-muted shrink-0 pt-0.5">
                  {step.label}
                </div>
                {/* Bar */}
                <div className="flex-1 h-9 relative rounded-lg overflow-hidden bg-[var(--color-bg-hover)]">
                  <div
                    className={`h-full rounded-lg bg-gradient-to-r ${FUNNEL_COLORS[i]} transition-all duration-500 flex items-center justify-between px-3`}
                    style={{ width: `${w}%` }}
                  >
                    <span className="text-[11px] font-bold text-foreground/90 truncate drop-shadow-sm">
                      {fmt(step.count_all)}
                    </span>
                    <span className="text-[11px] font-medium text-foreground-muted tabular-nums">
                      {convRate(prev, step.count_all)}
                    </span>
                  </div>
                </div>
                {/* Time breakdown */}
                <div className="w-56 flex gap-2 shrink-0 text-[10px] text-foreground-dim tabular-nums">
                  <span title="今日">{fmt(step.count_today)}</span>
                  <span title="7天" className="text-foreground-disabled">7d:{fmt(step.count_7d)}</span>
                  <span title="30天" className="text-foreground-disabled">30d:{fmt(step.count_30d)}</span>
                </div>
              </div>
            )
          })}
        </div>

        {/* Summary table below funnel */}
        <div className="mt-5 overflow-x-auto">
          <table className="w-full text-xs text-left">
            <thead>
              <tr className="border-b border-border text-foreground-dim">
                <th className="py-2 pl-3 font-medium">阶段</th>
                <th className="py-2 px-3 text-right font-medium">全部</th>
                <th className="py-2 px-3 text-right font-medium">今日</th>
                <th className="py-2 px-3 text-right font-medium">7 天</th>
                <th className="py-2 px-3 text-right font-medium">30 天</th>
                <th className="py-2 px-3 text-right font-medium">层转化率</th>
              </tr>
            </thead>
            <tbody className="text-foreground-muted">
              {steps.map((step, i) => (
                <tr key={step.label} className="border-b border-border last:border-0 hover:bg-[var(--color-bg-hover)]">
                  <td className="py-1.5 pl-3">
                    <span className="inline-flex items-center gap-1.5">
                      <span className="w-2.5 h-2.5 rounded-sm bg-gradient-to-r shrink-0" style={{ background: `linear-gradient(to right, ${FUNNEL_COLORS[i].replace('from-', '').replace('to-', ', ')})` }} />
                      {step.label}
                    </span>
                  </td>
                  <td className="py-1.5 px-3 text-right tabular-nums font-medium text-foreground-muted">{fmt(step.count_all)}</td>
                  <td className="py-1.5 px-3 text-right tabular-nums text-foreground-dim">{fmt(step.count_today)}</td>
                  <td className="py-1.5 px-3 text-right tabular-nums text-foreground-dim">{fmt(step.count_7d)}</td>
                  <td className="py-1.5 px-3 text-right tabular-nums text-foreground-dim">{fmt(step.count_30d)}</td>
                  <td className="py-1.5 px-3 text-right tabular-nums text-positive/70">
                    {i > 0 ? convRate(steps[i - 1].count_all, step.count_all) : '—'}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        {/* Key insight: overall conversion */}
        <div className="mt-3 flex items-center justify-between text-[11px] text-foreground-dim">
          <span>整体转化（访客 → 用 AI）：{convRate(steps[0]?.count_all, steps[steps.length - 1]?.count_all)}</span>
          <span>{data?.generated_at ? `数据更新：${new Date(data.generated_at).toLocaleString('zh-CN')}` : ''}</span>
        </div>
      </div>
    </section>
  )
}

// ── Backup Panel (数据备份) ──

export function AdminPaymentsPanel({ onUnauthorized }) {
  const [createDraft, setCreateDraft] = useState({
    amount_minor: 100,
    currency: 'cny',
    payment_method: 'card',
    payment_method_types: ['card'],
    title: 'Stripe Admin Test Payment',
  })
  const [selectedPaymentId, setSelectedPaymentId] = useState('')
  const [creating, setCreating] = useState(false)
  const [expiring, setExpiring] = useState(false)
  const [actionError, setActionError] = useState('')
  const [actionSuccess, setActionSuccess] = useState('')
  const [autoPollPaymentId, setAutoPollPaymentId] = useState('')

  const paymentsResource = useAdminResource({
    key: 'admin:payments:list',
    request: async () => {
      const [config, listPayload] = await Promise.all([
        adminFetch('/api/admin/payments/config').catch(() => null),
        adminFetch('/api/admin/payments?purpose=admin_test&limit=20').catch(() => ({ items: [], total: 0 })),
      ])
      return {
        config,
        payments: listPayload || { items: [], total: 0 },
      }
    },
    staleMs: 5_000,
    minIntervalMs: 2_000,
    shouldPoll: (payload) => resolveAdminPaymentPollingState(payload, autoPollPaymentId) === 'poll',
    pollMs: 4_000,
    onUnauthorized,
    errorMessage: '加载支付测试数据失败',
  })

  const detailResource = useAdminResource({
    key: `admin:payments:detail:${selectedPaymentId || 'none'}`,
    enabled: Boolean(selectedPaymentId),
    request: async () => adminFetch(`/api/admin/payments/${selectedPaymentId}`).catch(() => null),
    staleMs: 5_000,
    minIntervalMs: 2_000,
    shouldPoll: () => selectedPaymentId === autoPollPaymentId && resolveAdminPaymentPollingState(paymentsResource.data, autoPollPaymentId) === 'poll',
    pollMs: 4_000,
    onUnauthorized,
    errorMessage: '加载支付详情失败',
  })

  const config = paymentsResource.data?.config || null
  const payments = paymentsResource.data?.payments?.items || []
  const detail = detailResource.data || null
  const selectedPayment = detail?.payment || payments.find((item) => item.id === selectedPaymentId) || null
  const selectedEvents = detail?.events || []
  const configReady = Boolean(config?.ready)
  const resourceError = paymentsResource.error || detailResource.error
  const paymentMethodOptions = resolveAdminPaymentMethodOptions(config)
  const selectedCreateMethodMeta = resolveAdminPaymentMethodMeta(config, createDraft.payment_method || createDraft.payment_method_types?.[0] || 'card')
  const selectedCreateCurrencies = (selectedCreateMethodMeta?.supported_currencies?.length ? selectedCreateMethodMeta.supported_currencies : ['cny']).map((item) => String(item).toLowerCase())

  useEffect(() => {
    const nextPaymentId = resolveAdminSelectedPaymentId(payments, selectedPaymentId)
    if (nextPaymentId !== selectedPaymentId) {
      setSelectedPaymentId(nextPaymentId)
    }
  }, [payments, selectedPaymentId])

  useEffect(() => {
    if (!autoPollPaymentId) return
    if (resolveAdminPaymentPollingState(paymentsResource.data, autoPollPaymentId) === 'stop') {
      setAutoPollPaymentId('')
    }
  }, [autoPollPaymentId, paymentsResource.data])

  useEffect(() => {
    const enabledMethod = paymentMethodOptions.find((item) => item?.enabled)
    if (!enabledMethod) return
    const currentMethod = createDraft.payment_method || createDraft.payment_method_types?.[0] || ''
    if (currentMethod !== enabledMethod.code && !paymentMethodOptions.some((item) => item?.code === currentMethod && item?.enabled)) {
      setCreateDraft((current) => resolveAdminPaymentDraftForMethod(current, config, enabledMethod.code))
      return
    }
    if (selectedCreateMethodMeta && !selectedCreateCurrencies.includes(String(createDraft.currency || '').toLowerCase())) {
      setCreateDraft((current) => resolveAdminPaymentDraftForMethod(current, config, currentMethod || enabledMethod.code))
    }
  }, [config, createDraft.currency, createDraft.payment_method, createDraft.payment_method_types, paymentMethodOptions, selectedCreateCurrencies, selectedCreateMethodMeta])

  const refreshPaymentsPanel = useCallback(async () => {
    await paymentsResource.refresh()
    if (selectedPaymentId) {
      await detailResource.refresh()
    }
  }, [detailResource, paymentsResource, selectedPaymentId])

  const updateDraft = (key, value) => {
    setCreateDraft((current) => ({ ...current, [key]: value }))
  }

  const handleSelectCreateMethod = (paymentMethod) => {
    setCreateDraft((current) => resolveAdminPaymentDraftForMethod(current, config, paymentMethod))
  }

  const handleCreate = async () => {
    setCreating(true)
    setActionError('')
    setActionSuccess('')
    try {
      const result = await adminFetch('/api/admin/payments/checkout-sessions', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(createDraft),
      })
      const createdMethod = result.payment_method || createDraft.payment_method || createDraft.payment_method_types?.[0] || 'card'
      const methodLabel = PAYMENT_METHOD_LABELS[createdMethod] || createdMethod
      setSelectedPaymentId(result.payment_id || '')
      setAutoPollPaymentId(result.payment_id || '')
      setActionSuccess(`测试支付已创建：${methodLabel}。${result.allowed_payment_note || '可直接打开 Stripe Hosted Checkout 继续验证。'}`)
      await paymentsResource.refresh()
      if (result.checkout_url) {
        window.open(result.checkout_url, '_blank', 'noopener,noreferrer')
      }
    } catch (err) {
      const message = handleAdminActionError(err, onUnauthorized, '创建测试支付失败')
      if (message) setActionError(message)
    } finally {
      setCreating(false)
    }
  }

  const handleOpenCheckout = () => {
    const url = selectedPayment?.checkout_url
    if (!url) return
    window.open(url, '_blank', 'noopener,noreferrer')
  }

  const handleExpire = async () => {
    if (!selectedPaymentId || !window.confirm('确定要将当前 Checkout Session 手动过期吗？')) return
    setExpiring(true)
    setActionError('')
    setActionSuccess('')
    try {
      setAutoPollPaymentId(selectedPaymentId)
      await adminFetch(`/api/admin/payments/${selectedPaymentId}/expire`, {
        method: 'POST',
      })
      setActionSuccess('当前测试支付已手动过期。')
      await refreshPaymentsPanel()
    } catch (err) {
      const message = handleAdminActionError(err, onUnauthorized, '手动过期失败')
      if (message) setActionError(message)
    } finally {
      setExpiring(false)
    }
  }

  if (!config && !payments.length && !paymentsResource.loading) return null

  const selectedStatusMeta = getPaymentStatusMeta(selectedPayment?.status)

  return (
    <section className="rounded-2xl border border-border bg-card p-4 sm:p-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h2 className="text-base font-semibold text-foreground-muted">💳 支付测试</h2>
          <p className="mt-1 text-xs leading-6 text-foreground-dim">
            当前仅用于 admin 内测 Stripe Hosted Checkout 一次性支付链路，不改公开站点。现已支持银行卡、支付宝、微信支付三种测试方式，最终状态仍以 webhook 回写为准。
          </p>
        </div>
        <button
          type="button"
          onClick={() => refreshPaymentsPanel()}
          className="rounded-lg border border-[var(--color-border-strong)] px-3 py-1.5 text-xs text-foreground-dim transition hover:text-foreground"
        >
          刷新支付状态
        </button>
      </div>

      {resourceError ? (
        <div className="mt-3 rounded-xl border border-rose-400/20 bg-negative/10 px-4 py-2 text-xs text-negative">
          {resourceError}
        </div>
      ) : null}
      {actionError ? (
        <div className="mt-3 rounded-xl border border-rose-400/20 bg-negative/10 px-4 py-2 text-xs text-negative">
          {actionError}
        </div>
      ) : null}
      {actionSuccess ? (
        <div className="mt-3 rounded-xl border border-emerald-400/20 bg-positive/10 px-4 py-2 text-xs text-positive">
          {actionSuccess}
        </div>
      ) : null}

      <div className="mt-4 grid grid-cols-2 gap-3 lg:grid-cols-5">
        <StatCard label="当前模式" value={(config?.mode || '--').toUpperCase()} sub={config?.mode === 'test' ? '一期限制为测试模式' : '非测试模式不可创建'} />
        <StatCard label="Secret Key" value={config?.secret_key_configured ? '已配置' : '未配置'} sub="服务端 Stripe 凭据" />
        <StatCard label="Webhook Secret" value={config?.webhook_secret_configured ? '已配置' : '未配置'} sub="Webhook 验签必需" />
        <StatCard label="默认币种" value={(config?.default_currency || 'cny').toUpperCase()} sub="admin 测试默认值" />
        <StatCard label="允许支付方式" value={formatPaymentMethodList(config?.allowed_payment_methods)} sub="通过 env 控制 admin 内测范围" />
      </div>

      <div className="mt-4 grid grid-cols-1 gap-3 lg:grid-cols-3">
        {paymentMethodOptions.map((item) => (
          <div key={item.code} className={`rounded-2xl border px-4 py-3 ${item.enabled ? 'border-border bg-background-alt/40' : 'border-border/70 bg-[var(--color-bg-hover)]/60'}`}>
            <div className="flex items-center justify-between gap-3">
              <div className="text-sm font-semibold text-foreground-muted">{item.label}</div>
              <span className={`rounded-full border px-2 py-1 text-[11px] ${item.enabled ? 'border-emerald-400/25 bg-positive/10 text-positive' : 'border-[var(--color-border-strong)] bg-[var(--color-bg-hover)] text-foreground-dim'}`}>
                {item.enabled ? '已启用' : '未启用'}
              </span>
            </div>
            <div className="mt-2 text-xs leading-6 text-foreground-dim">{item.description || '--'}</div>
            <div className="mt-2 text-[11px] text-foreground-disabled">币种：{(item.supported_currencies || []).map((currency) => String(currency).toUpperCase()).join(' / ') || '--'} · 形态：{item.checkout_flow || '--'}</div>
          </div>
        ))}
      </div>

      <div className="mt-4 grid grid-cols-1 gap-4 xl:grid-cols-[360px_minmax(0,1fr)]">
        <div className="rounded-2xl border border-border bg-background-alt/40 p-4">
          <div className="flex items-center justify-between gap-3">
            <h3 className="text-sm font-semibold text-foreground-muted">创建测试支付</h3>
            <span className="rounded-full border border-sky-400/20 bg-sky-500/10 px-2 py-1 text-[11px] text-sky-100">Stripe Hosted Checkout</span>
          </div>
          <div className="mt-3 space-y-3">
            <div>
              <label className="mb-1.5 block text-xs text-foreground-dim">测试金额（分）</label>
              <input
                type="number"
                min="1"
                step="1"
                value={createDraft.amount_minor}
                onChange={(event) => updateDraft('amount_minor', Number(event.target.value || 0))}
                className="w-full rounded-xl border border-border bg-card px-3 py-2 text-sm text-foreground outline-none transition focus:border-primary"
              />
              <p className="mt-1 text-[11px] text-foreground-disabled">例如 100 表示 ¥1.00</p>
            </div>
            <div>
              <label className="mb-1.5 block text-xs text-foreground-dim">支付方式</label>
              <select
                value={createDraft.payment_method || createDraft.payment_method_types[0] || 'card'}
                onChange={(event) => handleSelectCreateMethod(event.target.value)}
                className="w-full rounded-xl border border-border bg-card px-3 py-2 text-sm text-foreground outline-none transition focus:border-primary"
              >
                {paymentMethodOptions.filter((item) => item?.enabled).map((item) => (
                  <option key={item.code} value={item.code}>{item.label}</option>
                ))}
              </select>
              {selectedCreateMethodMeta ? (
                <p className="mt-1 text-[11px] text-foreground-disabled">{selectedCreateMethodMeta.description}</p>
              ) : null}
            </div>
            <div>
              <label className="mb-1.5 block text-xs text-foreground-dim">币种</label>
              <select
                value={createDraft.currency}
                onChange={(event) => updateDraft('currency', event.target.value)}
                className="w-full rounded-xl border border-border bg-card px-3 py-2 text-sm text-foreground outline-none transition focus:border-primary"
              >
                {selectedCreateCurrencies.map((currency) => (
                  <option key={currency} value={currency}>{String(currency).toUpperCase()}</option>
                ))}
              </select>
              <p className="mt-1 text-[11px] text-foreground-disabled">推荐币种：{String(selectedCreateMethodMeta?.recommended_currency || createDraft.currency || 'cny').toUpperCase()}</p>
            </div>
            <div>
              <label className="mb-1.5 block text-xs text-foreground-dim">测试标题</label>
              <input
                type="text"
                value={createDraft.title}
                onChange={(event) => updateDraft('title', event.target.value)}
                className="w-full rounded-xl border border-border bg-card px-3 py-2 text-sm text-foreground outline-none transition focus:border-primary"
              />
            </div>
            {selectedCreateMethodMeta ? (
              <div className="rounded-xl border border-sky-400/20 bg-sky-500/10 px-3 py-3 text-xs text-sky-100">
                <div className="font-medium">测试提示</div>
                <div className="mt-1 leading-6">{selectedCreateMethodMeta.testing_note || '创建后可直接打开 Stripe Hosted Checkout 继续验证。'}</div>
              </div>
            ) : null}
            <button
              type="button"
              disabled={creating || !configReady}
              onClick={handleCreate}
              className={`w-full rounded-xl px-4 py-2.5 text-sm font-semibold transition ${
                creating || !configReady
                  ? 'cursor-not-allowed bg-[var(--color-bg-hover)] text-foreground-disabled'
                  : 'bg-amber-500 text-black hover:bg-amber-400'
              }`}
            >
              {creating ? '创建中…' : '创建并打开测试支付'}
            </button>
          </div>
        </div>

        <div className="space-y-4">
          <div className="rounded-2xl border border-border bg-background-alt/40 p-4">
            <div className="flex items-center justify-between gap-3">
              <h3 className="text-sm font-semibold text-foreground-muted">最近支付记录</h3>
              <span className="text-xs text-foreground-dim">共 {paymentsResource.data?.payments?.total || 0} 笔</span>
            </div>
            {!payments.length ? (
              <p className="mt-3 rounded-xl border border-dashed border-border px-4 py-6 text-center text-xs text-foreground-dim">
                暂无测试支付记录。创建第一笔后，系统会在这里持续轮询状态与 webhook 回写结果。
              </p>
            ) : (
              <div className="mt-3 space-y-2">
                {payments.map((item) => {
                  const meta = getPaymentStatusMeta(item.status)
                  const active = selectedPaymentId === item.id
                  return (
                    <button
                      key={item.id}
                      type="button"
                      onClick={() => setSelectedPaymentId(item.id)}
                      className={`w-full rounded-xl border px-3 py-3 text-left transition ${
                        active ? 'border-amber-400/40 bg-amber-500/10' : 'border-border bg-card hover:bg-[var(--color-bg-hover)]'
                      }`}
                    >
                      <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
                        <div>
                          <div className="text-sm font-medium text-foreground">{item.title || 'Stripe Admin Test Payment'}</div>
                          <div className="mt-1 text-xs text-foreground-dim">{formatMinorAmount(item.amount_minor, item.currency)} · {formatPaymentMethodList(item.payment_method_selected || item.payment_method_request)}</div>
                        </div>
                        <div className="flex items-center gap-2">
                          <span className={`rounded-full border px-2 py-1 text-[11px] ${meta.className}`}>{meta.label}</span>
                          <span className="text-[11px] text-foreground-disabled">{formatAdminDateTime(item.updated_at || item.created_at)}</span>
                          <span className="text-[11px] text-amber-200">查看详情</span>
                        </div>
                      </div>
                    </button>
                  )
                })}
              </div>
            )}
          </div>

          {selectedPayment ? (
            <div className="rounded-2xl border border-border bg-background-alt/40 p-4">
              <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                <div>
                  <div className="flex items-center gap-2">
                    <h3 className="text-sm font-semibold text-foreground-muted">支付详情</h3>
                    <span className={`rounded-full border px-2 py-1 text-[11px] ${selectedStatusMeta.className}`}>{selectedStatusMeta.label}</span>
                  </div>
                  <p className="mt-1 text-xs text-foreground-dim">Payment ID：{selectedPayment.id}</p>
                </div>
                <div className="flex flex-wrap gap-2">
                  <button
                    type="button"
                    disabled={!selectedPayment.checkout_url}
                    onClick={handleOpenCheckout}
                    className={`rounded-lg px-3 py-1.5 text-xs font-medium transition ${
                      selectedPayment.checkout_url ? 'bg-sky-500/15 text-sky-100 hover:bg-sky-500/25' : 'bg-[var(--color-bg-hover)] text-foreground-disabled'
                    }`}
                  >
                    打开 Checkout
                  </button>
                  <button
                    type="button"
                    disabled={expiring || selectedPayment.status !== 'checkout_open'}
                    onClick={handleExpire}
                    className={`rounded-lg px-3 py-1.5 text-xs font-medium transition ${
                      expiring || selectedPayment.status !== 'checkout_open'
                        ? 'bg-[var(--color-bg-hover)] text-foreground-disabled'
                        : 'bg-amber-500/12 text-amber-100 hover:bg-amber-500/20'
                    }`}
                  >
                    {expiring ? '处理中…' : '手动过期'}
                  </button>
                </div>
              </div>

              <div className="mt-4 grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
                <AIConfigMetric label="金额" value={formatMinorAmount(selectedPayment.amount_minor, selectedPayment.currency)} sub="一次性支付测试" />
                <AIConfigMetric label="支付方式" value={formatPaymentMethodList(selectedPayment.payment_method_selected || selectedPayment.payment_method_request)} sub="以 Checkout 实际完成方式为准" />
                <AIConfigMetric label="Session ID" value={selectedPayment.checkout_session_id || '--'} sub="Stripe Checkout Session" />
                <AIConfigMetric label="PaymentIntent ID" value={selectedPayment.payment_intent_id || '--'} sub="便于定位 webhook" />
              </div>

              {(selectedPayment.last_error_message || selectedPayment.last_error_code) ? (
                <div className="mt-3 rounded-xl border border-rose-400/20 bg-negative/10 px-4 py-3 text-xs text-negative">
                  最近错误：{selectedPayment.last_error_code ? `${selectedPayment.last_error_code} · ` : ''}{selectedPayment.last_error_message || '--'}
                </div>
              ) : null}

              <div className="mt-4">
                <h4 className="text-xs font-semibold uppercase tracking-wide text-foreground-dim">事件时间线</h4>
                {!selectedEvents.length ? (
                  <p className="mt-2 text-xs text-foreground-dim">暂无事件。创建支付后，这里会展示 admin API 与 webhook 的状态迁移。</p>
                ) : (
                  <div className="mt-3 space-y-2">
                    {selectedEvents.map((event) => (
                      <div key={event.id} className="rounded-xl border border-border bg-card px-3 py-3">
                        <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
                          <div>
                            <div className="text-sm font-medium text-foreground">{event.event_type}</div>
                            <div className="mt-1 text-[11px] text-foreground-dim">
                              {event.status_before || '--'} → {event.status_after || '--'} · {event.source || '--'}
                            </div>
                          </div>
                          <div className="text-[11px] text-foreground-disabled">{formatAdminDateTime(event.received_at || event.occurred_at)}</div>
                        </div>
                        {event.error_message ? (
                          <div className="mt-2 text-[11px] text-negative">{event.error_message}</div>
                        ) : null}
                      </div>
                    ))}
                  </div>
                )}
              </div>
            </div>
          ) : null}
        </div>
      </div>
    </section>
  )
}

export function BackupPanel({ onUnauthorized }) {
  const [triggering, setTriggering] = useState(false)
  const [actionError, setActionError] = useState('')
  const resource = useAdminResource({
    key: 'admin:backup',
    request: async () => {
      const [status, historyPayload, stats] = await Promise.all([
        adminFetch('/api/admin/backup-status').catch(() => null),
        adminFetch('/api/admin/backup-history?limit=7').catch(() => ({ items: [] })),
        adminFetch('/api/admin/backup-stats').catch(() => null),
      ])
      return {
        status,
        history: historyPayload?.items || [],
        stats,
      }
    },
    staleMs: 15_000,
    minIntervalMs: 3_000,
    pollMs: (payload) => (shouldPollBackupStatus(payload?.status) ? 2_000 : 120_000),
    onUnauthorized,
    errorMessage: '加载备份数据失败',
  })
  const status = resource.data?.status || null
  const history = resource.data?.history || null
  const stats = resource.data?.stats || null

  const handleTrigger = async () => {
    if (!window.confirm('确定要立即执行一次备份吗？')) return
    setTriggering(true)
    setActionError('')
    try {
      await adminFetch('/api/admin/backup-trigger', { method: 'POST' })
      await resource.refresh()
    } catch (err) {
      const message = handleAdminActionError(err, onUnauthorized, '触发备份失败')
      if (message) {
        setActionError(message)
      }
    } finally {
      setTriggering(false)
    }
  }

  if (!status && !history) return null

  const cards = buildBackupStatusCards(status, stats)
  const jobBanner = buildBackupJobBanner(status)
  const triggerButton = resolveBackupTriggerButton({ triggering, status })
  const cosMeta = getBackupCosMeta(status?.cos_status)

  return (
    <section>
      <h2 className="text-base font-semibold text-foreground-muted mb-3">📦 数据备份</h2>
      {resource.error ? (
        <div className="mb-3 rounded-xl border border-rose-400/20 bg-negative/10 px-4 py-2 text-xs text-negative">
          {resource.error}
        </div>
      ) : null}
      {actionError ? (
        <div className="mb-3 rounded-xl border border-rose-400/20 bg-negative/10 px-4 py-2 text-xs text-negative">
          {actionError}
        </div>
      ) : null}

      {/* Status Cards */}
      <div className="grid grid-cols-2 sm:grid-cols-4 lg:grid-cols-6 gap-3">
        <StatCard label="状态" value={cards.overall.value} sub={cards.overall.sub} />
        <StatCard label="主库大小" value={cards.sizes.pumpkin} />
        <StatCard label="A 股缓存" value={cards.sizes.cacheA} />
        <StatCard label="港股缓存" value={cards.sizes.cacheHK} />
        <StatCard label="COS 同步" value={cards.cos.value} sub={cards.cos.sub} />
        <StatCard label="耗时" value={cards.duration} />
      </div>

      {jobBanner && (
        <div className={`mt-3 rounded-xl border px-3 py-2 text-xs ${
          jobBanner.tone === 'danger'
            ? 'bg-negative/10 border-rose-400/20 text-negative'
            : jobBanner.tone === 'warning'
              ? 'bg-amber-500/10 border-amber-400/20 text-amber-200'
              : jobBanner.tone === 'success'
                ? 'bg-positive/10 border-positive/20 text-positive'
                : 'bg-sky-500/10 border-sky-400/20 text-sky-100'
        }`}>
          {jobBanner.text}
        </div>
      )}

      {/* Error Message */}
      {status?.error_msg && (
        <div className="mt-3 rounded-xl bg-negative/10 border border-rose-400/20 px-3 py-2 text-xs text-negative">
          {status.error_msg}
        </div>
      )}

      {status?.cos_error_msg && status?.cos_status !== 'success' && (
        <div className={`mt-3 rounded-xl border px-3 py-2 text-xs ${cosMeta.tone} border-border bg-[var(--color-bg-hover)]`}>
          COS: {status.cos_error_msg}
        </div>
      )}

      {/* Storage Stats */}
      {stats && (
        <div className="mt-3 flex gap-6 text-xs text-foreground-dim">
          <span>本地: {formatBackupBytes(stats.local_total_bytes)} ({stats.local_file_count} 文件 · 保留{stats.local_retention_days}天)</span>
          {stats.cloud_enabled && (
            <span>云端: {formatBackupBytes(stats.cloud_total_bytes)} ({stats.cloud_file_count} 文件)</span>
          )}
        </div>
      )}
      {stats?.cloud_error_msg && (
        <div className="mt-2 text-xs text-amber-200">云端统计获取失败: {stats.cloud_error_msg}</div>
      )}

      {/* Manual Trigger */}
      <div className="mt-4 flex items-center justify-between">
        <h3 className="text-sm font-medium text-foreground-dim">最近备份记录</h3>
        <button
          type="button"
          disabled={triggerButton.disabled}
          onClick={handleTrigger}
          className={`rounded-lg border px-3 py-1.5 text-xs font-medium transition ${
            triggerButton.disabled
              ? 'border-border bg-[var(--color-bg-hover)] text-foreground-dim cursor-not-allowed'
              : 'border-positive/30 bg-emerald-500/8 text-positive hover:bg-emerald-500/15 hover:border-emerald-400/50'
          }`}
        >
          {triggerButton.label}
        </button>
      </div>

      {/* History Table */}
      {!history || history.length === 0 ? (
        <p className="mt-2 text-xs text-foreground-dim p-3 rounded-xl border border-dashed border-border text-center">
          暂无备份记录 — 系统将在每天凌晨自动执行备份
        </p>
      ) : (
        <div className="mt-2 rounded-xl border border-border bg-card overflow-hidden">
          <table className="w-full text-xs text-left">
            <thead>
              <tr className="border-b border-border text-foreground-dim">
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
            <tbody className="text-foreground-muted">
              {history.map((row) => (
                <tr key={row.id} className="border-b border-border last:border-0 hover:bg-[var(--color-bg-hover)]">
                  <td className="py-1.5 pl-4 whitespace-nowrap tabular-nums text-foreground-dim">{row.triggered_at}</td>
                  <td className="py-1.5 px-3 text-foreground-dim">{BACKUP_TRIGGER_LABELS[row.trigger_type] || row.trigger_type}</td>
                  <td className={`py-1.5 px-3 font-medium ${BACKUP_STATUS_COLORS[row.status] || ''}`}>{row.status}</td>
                  <td className="py-1.5 px-3 text-right tabular-nums">{formatBackupBytes(row.pumpkin_size_bytes)}</td>
                  <td className="py-1.5 px-3 text-right tabular-nums">{formatBackupBytes(row.cache_a_size_bytes + row.cache_hk_size_bytes)}</td>
                  <td className={`py-1.5 px-3 text-center ${getBackupCosMeta(row.cos_status).tone}`}>{getBackupCosMeta(row.cos_status).symbol}</td>
                  <td className="py-1.5 px-3 text-right tabular-nums text-foreground-dim">{formatBackupDuration(row.duration_ms)}</td>
                  <td className="py-1.5 pr-4 text-foreground-disabled max-w-[200px] truncate" title={row.error_msg}>
                    {buildBackupHistoryNote(row)}
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

export function AdminDataPage({ onUnauthorized }) {
  return (
    <div className="space-y-8">
      <CompanyProfilesAdminPanel onUnauthorized={onUnauthorized} />
      <FactorLabPipelinePanel onUnauthorized={onUnauthorized} />
      <FactorIndexAdminPanel onUnauthorized={onUnauthorized} />
      <QuadrantAdminPanel onUnauthorized={onUnauthorized} />
    </div>
  )
}

export function AdminAIPage({ onUnauthorized }) {
  return (
    <div className="space-y-8">
      <AIProviderConfigPanel onUnauthorized={onUnauthorized} />
      <AIUsageAdminPanel onUnauthorized={onUnauthorized} />
      <AIReportsAdminPanel onUnauthorized={onUnauthorized} />
      <AIPickerAdminPanel onUnauthorized={onUnauthorized} />
    </div>
  )
}

export function AdminOpsPage({ onUnauthorized }) {
  return (
    <div className="space-y-8">
      <AdminPaymentsPanel onUnauthorized={onUnauthorized} />
      <BackupPanel onUnauthorized={onUnauthorized} />
      <SystemHealthPanel onUnauthorized={onUnauthorized} />
      <FeedbackPanel onUnauthorized={onUnauthorized} />
    </div>
  )
}
