import { useState } from 'react'

import {
  adminFetch,
  handleAdminActionError,
  useAdminResource,
} from '../../lib/admin-data'

const FACTOR_OPTIONS = [
  { value: '', label: '全部因子' },
  { value: 'value', label: '价值因子指数' },
  { value: 'dividend_yield', label: '股息率因子指数' },
  { value: 'growth', label: '成长因子指数' },
  { value: 'quality', label: '质量因子指数' },
  { value: 'momentum', label: '动量因子指数' },
  { value: 'size', label: '小市值因子指数' },
  { value: 'low_volatility', label: '低波动因子指数' },
]

const OPERATION_OPTIONS = [
  { value: 'sync_all', label: '全链路补算' },
  { value: 'sync_daily', label: '仅补算日净值' },
  { value: 'sync_rebalances', label: '仅补算月度调仓' },
]

function StatCard({ label, value, sub }) {
  return (
    <div className="rounded-xl border border-border bg-card p-4">
      <div className="text-xs text-foreground-dim">{label}</div>
      <div className="mt-2 text-lg font-semibold text-foreground-muted">{value || '--'}</div>
      {sub ? <div className="mt-1 text-xs text-foreground-dim">{sub}</div> : null}
    </div>
  )
}

function formatAdminDateTime(value) {
  if (!value) return '--'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '--'
  return new Intl.DateTimeFormat('zh-CN', {
    timeZone: 'Asia/Shanghai',
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  }).format(date)
}

function formatNumber(value, digits = 0) {
  const num = Number(value)
  if (!Number.isFinite(num)) return '--'
  return num.toLocaleString('zh-CN', {
    minimumFractionDigits: digits,
    maximumFractionDigits: digits,
  })
}

function formatPercent(value, digits = 2) {
  const num = Number(value)
  if (!Number.isFinite(num)) return '--'
  const prefix = num > 0 ? '+' : ''
  return `${prefix}${(num * 100).toFixed(digits)}%`
}

function formatDurationSeconds(value) {
  const seconds = Number(value || 0)
  if (!Number.isFinite(seconds) || seconds <= 0) return '--'
  if (seconds < 60) return `${Math.round(seconds)}秒`
  return `${Math.floor(seconds / 60)}分${Math.round(seconds % 60)}秒`
}

function resolveRunStatusClass(status) {
  if (status === 'completed') return 'border-emerald-400/25 bg-positive/10 text-positive'
  if (status === 'failed') return 'border-rose-400/25 bg-negative/10 text-negative'
  if (status === 'running') return 'border-blue-400/25 bg-blue-500/10 text-blue-700 dark:text-blue-200'
  return 'border-border bg-[var(--color-bg-hover)] text-foreground-dim'
}

function resolveItemStatusClass(status) {
  if (status === 'completed') return 'text-positive'
  if (status === 'partial') return 'text-amber-700 dark:text-amber-200'
  if (status === 'failed') return 'text-negative'
  return 'text-foreground-dim'
}

function buildSubmitSummary(form) {
  const operation = OPERATION_OPTIONS.find((item) => item.value === form.operation)?.label || form.operation
  const factor = FACTOR_OPTIONS.find((item) => item.value === form.factor_key)?.label || '全部因子'
  const range = form.from_date || form.to_date ? `${form.from_date || '--'} ~ ${form.to_date || '--'}` : '全历史范围'
  const resetText = form.reset ? '会先清空目标物化结果再重建。' : '仅补齐缺口或重算目标范围。'
  return `确认执行单因子指数运维任务？\n操作：${operation}\n因子：${factor}\n范围：${range}\n${resetText}`
}

function normalizeRequestPayload(form) {
  const payload = {
    operation: form.operation,
    factor_key: form.factor_key,
    from_date: form.from_date,
    to_date: form.to_date,
    reset: form.reset,
  }
  Object.keys(payload).forEach((key) => {
    if (payload[key] === '' || payload[key] == null) {
      delete payload[key]
    }
  })
  if (!('reset' in payload)) {
    payload.reset = false
  }
  return payload
}

export default function FactorIndexAdminPanel({ onUnauthorized }) {
  const [submitting, setSubmitting] = useState(false)
  const [actionError, setActionError] = useState('')
  const [actionSuccess, setActionSuccess] = useState('')
  const [form, setForm] = useState({
    operation: 'sync_all',
    factor_key: '',
    from_date: '',
    to_date: '',
    reset: false,
  })
  const resource = useAdminResource({
    key: 'admin:factor-index',
    request: () => adminFetch('/api/admin/factor-index/status'),
    staleMs: 5_000,
    minIntervalMs: 3_000,
    pollMs: (payload) => payload?.worker?.running ? 5_000 : null,
    onUnauthorized,
    errorMessage: '加载单因子指数运维状态失败',
  })

  const data = resource.data || {}
  const worker = data.worker || {}
  const items = Array.isArray(data.items) ? data.items : []
  const latestRun = worker.current || null
  const history = Array.isArray(worker.history) ? worker.history : []

  const handleFieldChange = (key, value) => {
    setForm((prev) => ({ ...prev, [key]: value }))
  }

  const submitRequest = async (override = null) => {
    const nextForm = override ? { ...form, ...override } : form
    if (nextForm.operation === 'sync_daily' && !nextForm.from_date && !nextForm.to_date && !nextForm.factor_key) {
      if (!window.confirm('当前将扫描全部因子的所有交易日净值缺口。确定继续吗？')) return
    } else if (!window.confirm(buildSubmitSummary(nextForm))) {
      return
    }
    setSubmitting(true)
    setActionError('')
    setActionSuccess('')
    try {
      const payload = normalizeRequestPayload(nextForm)
      const resp = await adminFetch('/api/admin/factor-index/recompute', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      })
      setActionSuccess(resp?.message || '已触发单因子指数补算任务。')
      await resource.refresh({ force: true, preferCache: false })
    } catch (err) {
      const message = handleAdminActionError(err, onUnauthorized, '触发单因子指数补算失败')
      if (message) setActionError(message)
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <section>
      <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-base font-semibold text-foreground-muted">单因子指数运维</h2>
          <p className="mt-1 text-xs text-foreground-dim">管理 7 条单因子指数的月度调仓与日净值物化，支持查看最近运行状态并手动补算。</p>
        </div>
        <div className="flex flex-wrap gap-2">
          <button
            type="button"
            disabled={submitting || worker.running}
            onClick={() => submitRequest({ operation: 'sync_all', factor_key: '', from_date: '', to_date: '', reset: false })}
            className="rounded-lg border border-primary/30 bg-primary/10 px-3 py-2 text-xs font-semibold text-primary transition hover:bg-primary/15 disabled:cursor-not-allowed disabled:opacity-50"
          >
            {worker.running ? '任务运行中...' : '补齐全部缺口'}
          </button>
          <button
            type="button"
            disabled={submitting || worker.running}
            onClick={() => submitRequest({ operation: 'sync_all', factor_key: '', from_date: '', to_date: '', reset: true })}
            className="rounded-lg border border-rose-400/25 bg-negative/10 px-3 py-2 text-xs font-semibold text-negative transition hover:bg-negative/15 disabled:cursor-not-allowed disabled:opacity-50"
          >
            从头重建全部指数
          </button>
        </div>
      </div>

      {(resource.error || actionError) ? (
        <div className="mb-3 rounded-xl border border-rose-400/20 bg-negative/10 px-4 py-2 text-xs text-negative">
          {actionError || resource.error}
        </div>
      ) : null}
      {actionSuccess ? (
        <div className="mb-3 rounded-xl border border-emerald-400/20 bg-positive/10 px-4 py-2 text-xs text-positive">
          {actionSuccess}
        </div>
      ) : null}

      {latestRun ? (
        <div className="mb-4 rounded-xl border border-blue-400/20 bg-blue-500/10 px-4 py-3 text-xs text-blue-700 dark:text-blue-200">
          当前任务：{latestRun.trigger_type || '--'} / {latestRun.request?.operation || '--'}，开始于 {formatAdminDateTime(latestRun.started_at)}。
        </div>
      ) : null}

      <div className="grid grid-cols-2 gap-3 lg:grid-cols-6">
        <StatCard label="最新因子快照" value={data.latest_snapshot_date || '--'} />
        <StatCard label="最新净值日期" value={data.latest_trade_date || '--'} />
        <StatCard label="调仓调度" value={worker.rebalance_schedule || '--'} sub={formatAdminDateTime(worker.next_rebalance_at)} />
        <StatCard label="日净值调度" value={worker.daily_schedule || '--'} sub={formatAdminDateTime(worker.next_daily_at)} />
        <StatCard label="最近运行" value={formatAdminDateTime(worker.last_run_at)} sub={worker.last_error ? '最近一次失败' : '最近一次已完成'} />
        <StatCard label="最近成功" value={formatAdminDateTime(worker.last_success_at)} sub={worker.last_error || '暂无错误'} />
      </div>

      <div className="mt-4 rounded-xl border border-border bg-card p-4">
        <div className="mb-3 text-xs text-foreground-dim">手动补算</div>
        <div className="grid gap-3 md:grid-cols-5">
          <label className="text-xs text-foreground-dim">操作
            <select value={form.operation} onChange={(e) => handleFieldChange('operation', e.target.value)} className="mt-1 w-full rounded-lg border border-border bg-background-alt px-2 py-2 text-foreground outline-none transition focus:border-primary">
              {OPERATION_OPTIONS.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
            </select>
          </label>
          <label className="text-xs text-foreground-dim">因子
            <select value={form.factor_key} onChange={(e) => handleFieldChange('factor_key', e.target.value)} className="mt-1 w-full rounded-lg border border-border bg-background-alt px-2 py-2 text-foreground outline-none transition focus:border-primary">
              {FACTOR_OPTIONS.map((option) => <option key={option.value || 'all'} value={option.value}>{option.label}</option>)}
            </select>
          </label>
          <label className="text-xs text-foreground-dim">起始日期
            <input type="date" value={form.from_date} onChange={(e) => handleFieldChange('from_date', e.target.value)} className="mt-1 w-full rounded-lg border border-border bg-background-alt px-2 py-2 text-foreground outline-none transition focus:border-primary" />
          </label>
          <label className="text-xs text-foreground-dim">结束日期
            <input type="date" value={form.to_date} onChange={(e) => handleFieldChange('to_date', e.target.value)} className="mt-1 w-full rounded-lg border border-border bg-background-alt px-2 py-2 text-foreground outline-none transition focus:border-primary" />
          </label>
          <button
            type="button"
            disabled={submitting || worker.running}
            onClick={() => submitRequest()}
            className="self-end rounded-lg bg-primary px-4 py-2 text-xs font-semibold text-foreground transition hover:bg-primary/85 disabled:cursor-not-allowed disabled:opacity-50"
          >
            {submitting ? '提交中...' : '执行补算'}
          </button>
        </div>
        <label className="mt-3 flex items-center gap-2 text-xs text-foreground-dim">
          <input type="checkbox" checked={form.reset} onChange={(e) => handleFieldChange('reset', e.target.checked)} />
          <span>执行前先清空目标物化结果（reset）</span>
        </label>
        <p className="mt-2 text-xs text-foreground-dim">说明：日期留空表示按当前操作扫描全历史；“从头重建全部指数”会清空全部调仓与净值后重算。</p>
      </div>

      <div className="mt-4 rounded-xl border border-border bg-card p-4">
        <div className="mb-3 flex items-center justify-between gap-3 text-xs">
          <span className="text-foreground-dim">最近运行历史</span>
          <span className={`rounded-full border px-2 py-0.5 ${resolveRunStatusClass(worker.running ? 'running' : history[0]?.status || 'idle')}`}>
            {worker.running ? 'running' : history[0]?.status || 'idle'}
          </span>
        </div>
        {history.length === 0 ? (
          <p className="text-xs text-foreground-disabled">暂无运行历史。</p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs">
              <thead>
                <tr className="border-b border-border text-foreground-dim">
                  <th className="pb-2 pr-3">触发</th>
                  <th className="pb-2 pr-3">操作</th>
                  <th className="pb-2 pr-3">因子</th>
                  <th className="pb-2 pr-3">范围</th>
                  <th className="pb-2 pr-3">状态</th>
                  <th className="pb-2 pr-3">开始</th>
                  <th className="pb-2 pr-3">耗时</th>
                  <th className="pb-2">错误</th>
                </tr>
              </thead>
              <tbody className="text-foreground-muted">
                {history.map((run) => (
                  <tr key={run.id} className="border-b border-border last:border-0 align-top">
                    <td className="py-2 pr-3 whitespace-nowrap">{run.trigger_type || '--'}</td>
                    <td className="py-2 pr-3 whitespace-nowrap">{run.request?.operation || '--'}{run.request?.reset ? ' / reset' : ''}</td>
                    <td className="py-2 pr-3 whitespace-nowrap">{run.request?.factor_key || '全部'}</td>
                    <td className="py-2 pr-3 whitespace-nowrap">{run.request?.from_date || run.request?.to_date ? `${run.request?.from_date || '--'} ~ ${run.request?.to_date || '--'}` : '全历史'}</td>
                    <td className={`py-2 pr-3 ${resolveItemStatusClass(run.status)}`}>{run.status}</td>
                    <td className="py-2 pr-3 whitespace-nowrap">{formatAdminDateTime(run.started_at)}</td>
                    <td className="py-2 pr-3 whitespace-nowrap">{formatDurationSeconds(run.duration_seconds)}</td>
                    <td className="py-2 break-words whitespace-pre-wrap text-negative/80">{run.error_message || '--'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      <div className="mt-4 rounded-xl border border-border bg-card p-4">
        <div className="mb-3 text-xs text-foreground-dim">指数物化状态</div>
        {items.length === 0 ? (
          <p className="text-xs text-foreground-disabled">暂无单因子指数状态。</p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs">
              <thead>
                <tr className="border-b border-border text-foreground-dim">
                  <th className="pb-2 pr-3">因子指数</th>
                  <th className="pb-2 pr-3">最新净值日</th>
                  <th className="pb-2 pr-3">NAV</th>
                  <th className="pb-2 pr-3">最近调仓</th>
                  <th className="pb-2 pr-3">生效日</th>
                  <th className="pb-2 pr-3">成分股</th>
                  <th className="pb-2 pr-3">状态</th>
                  <th className="pb-2 pr-3">日净值计算</th>
                  <th className="pb-2">提示</th>
                </tr>
              </thead>
              <tbody className="text-foreground-muted">
                {items.map((item) => (
                  <tr key={item.index_id} className="border-b border-border last:border-0 align-top">
                    <td className="py-2 pr-3">
                      <div className="font-medium text-foreground-muted">{item.name}</div>
                      <div className="mt-1 text-[11px] text-foreground-dim">{item.factor_key}</div>
                    </td>
                    <td className="py-2 pr-3 whitespace-nowrap">{item.latest_trade_date || '--'}</td>
                    <td className="py-2 pr-3 whitespace-nowrap">{item.nav != null ? formatNumber(item.nav, 2) : '--'}</td>
                    <td className="py-2 pr-3 whitespace-nowrap">{item.rebalance_date || '--'}</td>
                    <td className="py-2 pr-3 whitespace-nowrap">{item.effective_start_date || '--'}</td>
                    <td className="py-2 pr-3 whitespace-nowrap">{formatNumber(item.constituent_count)}</td>
                    <td className="py-2 pr-3">
                      <div className={resolveItemStatusClass(item.status)}>{item.status || '--'}</div>
                      {item.rebalance_status && item.rebalance_status !== item.status ? <div className="mt-1 text-[11px] text-foreground-dim">调仓：{item.rebalance_status}</div> : null}
                    </td>
                    <td className="py-2 pr-3 whitespace-nowrap">{formatAdminDateTime(item.daily_computed_at)}</td>
                    <td className="py-2 break-words whitespace-pre-wrap text-foreground-dim">{item.warning_text || item.rebalance_warning || '--'}</td>
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
