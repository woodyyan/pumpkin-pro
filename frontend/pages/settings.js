import { useEffect, useState } from 'react'

import { requestJson } from '../lib/api'
import { useAuth } from '../lib/auth-context'
import { isAuthRequiredError } from '../lib/auth-storage'

export default function SettingsPage() {
  const { openAuthModal, isLoggedIn, ready, user } = useAuth()
  const [webhookConfig, setWebhookConfig] = useState({
    url: '',
    has_secret: false,
    is_enabled: true,
    timeout_ms: 3000,
    updated_at: '',
  })
  const [secretInput, setSecretInput] = useState('')
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState(false)
  const [notice, setNotice] = useState('')
  const [error, setError] = useState('')
  const [errorNeedsLogin, setErrorNeedsLogin] = useState(false)
  const [latestDelivery, setLatestDelivery] = useState(null)
  const [deliveryItems, setDeliveryItems] = useState([])
  const [investProfile, setInvestProfile] = useState(null)
  const [investForm, setInvestForm] = useState({
    total_capital: '',
    risk_preference: '',
    investment_goal: '',
    investment_horizon: '',
    max_drawdown_pct: '',
    experience_level: '',
    note: '',
  })
  const [investSaving, setInvestSaving] = useState(false)
  const [investNotice, setInvestNotice] = useState('')
  const authIdentityKey = String(user?.id || user?.email || '')

  const applyError = (err, fallbackText) => {
    setNotice('')
    setError(err.message || fallbackText)
    setErrorNeedsLogin(isAuthRequiredError(err))
  }

  const loadWebhookConfig = async () => {
    const data = await requestJson('/api/webhook')
    const item = data?.item || null
    if (!item) {
      setWebhookConfig({
        url: '',
        has_secret: false,
        is_enabled: true,
        timeout_ms: 3000,
        updated_at: '',
      })
      return
    }

    setWebhookConfig({
      url: item.url || '',
      has_secret: Boolean(item.has_secret),
      is_enabled: item.is_enabled !== false,
      timeout_ms: Number(item.timeout_ms) > 0 ? Number(item.timeout_ms) : 3000,
      updated_at: item.updated_at || '',
    })
  }

  const loadDeliveries = async () => {
    const latestPromise = requestJson('/api/webhook-deliveries/latest').catch((err) => {
      if (err?.status === 404) {
        return { item: null }
      }
      throw err
    })

    const [latestData, listData] = await Promise.all([
      latestPromise,
      requestJson('/api/webhook-deliveries?limit=20'),
    ])
    setLatestDelivery(latestData?.item || null)
    setDeliveryItems(Array.isArray(listData?.items) ? listData.items : [])
  }

  const loadInvestmentProfile = async () => {
    try {
      const data = await requestJson('/api/investment-profile')
      const p = data?.profile || null
      setInvestProfile(p)
      if (p) {
        setInvestForm({
          total_capital: p.total_capital ? String(p.total_capital) : '',
          risk_preference: p.risk_preference || '',
          investment_goal: p.investment_goal || '',
          investment_horizon: p.investment_horizon || '',
          max_drawdown_pct: p.max_drawdown_pct ? String(p.max_drawdown_pct) : '',
          experience_level: p.experience_level || '',
          note: p.note || '',
        })
      }
    } catch {
      // non-critical
    }
  }

  const loadPage = async () => {
    try {
      setError('')
      await Promise.all([loadWebhookConfig(), loadDeliveries(), loadInvestmentProfile()])
    } catch (err) {
      applyError(err, '加载设置失败')
    }
  }

  useEffect(() => {
    if (!ready) return

    if (!isLoggedIn) {
      setWebhookConfig({
        url: '',
        has_secret: false,
        is_enabled: true,
        timeout_ms: 3000,
        updated_at: '',
      })
      setSecretInput('')
      setLatestDelivery(null)
      setDeliveryItems([])
      setInvestProfile(null)
      setInvestForm({ total_capital: '', risk_preference: '', investment_goal: '', investment_horizon: '', max_drawdown_pct: '', experience_level: '', note: '' })
      setInvestNotice('')
      setNotice('')
      setError('')
      setErrorNeedsLogin(false)
      return
    }

    loadPage()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ready, isLoggedIn, authIdentityKey])

  const handleSaveWebhook = async () => {
    setSaving(true)
    setNotice('')
    setError('')

    try {
      const result = await requestJson('/api/webhook', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          url: webhookConfig.url,
          secret: secretInput,
          is_enabled: webhookConfig.is_enabled,
          timeout_ms: Number(webhookConfig.timeout_ms) || 3000,
        }),
      })

      const item = result?.item || null
      if (item) {
        setWebhookConfig({
          url: item.url || '',
          has_secret: Boolean(item.has_secret),
          is_enabled: item.is_enabled !== false,
          timeout_ms: Number(item.timeout_ms) > 0 ? Number(item.timeout_ms) : 3000,
          updated_at: item.updated_at || '',
        })
      }
      setSecretInput('')
      setNotice('Webhook 配置已保存')
    } catch (err) {
      applyError(err, '保存 Webhook 配置失败')
    } finally {
      setSaving(false)
    }
  }

  const handleTestWebhook = async () => {
    setTesting(true)
    setNotice('')
    setError('')

    try {
      await requestJson('/api/webhook/test', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ symbol: '00700.HK', side: 'BUY' }),
      })
      await loadDeliveries()
      setNotice('测试 Webhook 已送达，请查看下方投递结果')
    } catch (err) {
      await loadDeliveries().catch(() => null)
      applyError(err, '测试 Webhook 未送达')
    } finally {
      setTesting(false)
    }
  }

  return (
    <div className="space-y-6">
      <section className="rounded-2xl border border-border bg-card p-8">
        <h1 className="text-2xl font-semibold tracking-tight">设置</h1>
        <p className="mt-3 text-sm leading-7 text-white/65">
          用户级能力统一在这里管理。
        </p>
      </section>

      {/* Investment Profile */}
      <section className="rounded-2xl border border-border bg-card p-5">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h2 className="text-base font-semibold text-white">投资画像</h2>
            <p className="mt-1 text-xs text-white/60">帮助系统了解你的投资风格，以便未来提供更精准的策略推荐和风险提示。</p>
          </div>
          {investProfile?.updated_at && (
            <div className="text-xs text-white/55">更新：{formatDateTime(investProfile.updated_at)}</div>
          )}
        </div>

        {investNotice && (
          <div className="mt-3 rounded-xl border border-emerald-400/40 bg-emerald-500/10 px-4 py-3 text-sm text-emerald-200">{investNotice}</div>
        )}

        <div className="mt-4 space-y-4 rounded-xl border border-border bg-black/20 p-4">
          <div className="grid gap-4 md:grid-cols-2">
            <label className="block">
              <span className="text-xs text-white/55">风险偏好</span>
              <select
                value={investForm.risk_preference}
                onChange={(e) => setInvestForm((f) => ({ ...f, risk_preference: e.target.value }))}
                className="mt-1 block w-full rounded-lg border border-border bg-black/30 px-3 py-2 text-sm text-white outline-none transition focus:border-primary"
              >
                <option value="">请选择</option>
                <option value="保守">保守 — 尽量不亏，收益低一点也可以</option>
                <option value="稳健">稳健 — 追求稳定增长，可接受小幅波动</option>
                <option value="积极">积极 — 愿意承受较大波动换取更高回报</option>
                <option value="激进">激进 — 高风险高回报，能承受大幅亏损</option>
              </select>
            </label>

            <label className="block">
              <span className="text-xs text-white/55">投资目标</span>
              <select
                value={investForm.investment_goal}
                onChange={(e) => setInvestForm((f) => ({ ...f, investment_goal: e.target.value }))}
                className="mt-1 block w-full rounded-lg border border-border bg-black/30 px-3 py-2 text-sm text-white outline-none transition focus:border-primary"
              >
                <option value="">请选择</option>
                <option value="长期增值">长期增值 — 买入好公司长期持有</option>
                <option value="价值投资">价值投资 — 寻找被低估的股票</option>
                <option value="分红收益">分红收益 — 以股息收入为主</option>
                <option value="波段交易">波段交易 — 中短线高抛低吸</option>
                <option value="短线交易">短线交易 — 日内或数日内快进快出</option>
              </select>
            </label>

            <label className="block">
              <span className="text-xs text-white/55">投资周期</span>
              <select
                value={investForm.investment_horizon}
                onChange={(e) => setInvestForm((f) => ({ ...f, investment_horizon: e.target.value }))}
                className="mt-1 block w-full rounded-lg border border-border bg-black/30 px-3 py-2 text-sm text-white outline-none transition focus:border-primary"
              >
                <option value="">请选择</option>
                <option value="短期">短期 — 1 年以内</option>
                <option value="中期">中期 — 1~3 年</option>
                <option value="长期">长期 — 3 年以上</option>
              </select>
            </label>

            <label className="block">
              <span className="text-xs text-white/55">投资经验</span>
              <select
                value={investForm.experience_level}
                onChange={(e) => setInvestForm((f) => ({ ...f, experience_level: e.target.value }))}
                className="mt-1 block w-full rounded-lg border border-border bg-black/30 px-3 py-2 text-sm text-white outline-none transition focus:border-primary"
              >
                <option value="">请选择</option>
                <option value="新手">新手 — 刚开始接触股票投资</option>
                <option value="进阶">进阶 — 有 1~3 年投资经验</option>
                <option value="资深">资深 — 有 3 年以上投资经验</option>
                <option value="专业">专业 — 金融从业或全职投资</option>
              </select>
            </label>

            <label className="block">
              <span className="text-xs text-white/55">账户总资金（元）</span>
              <input
                type="number"
                min="0"
                step="any"
                value={investForm.total_capital}
                onChange={(e) => setInvestForm((f) => ({ ...f, total_capital: e.target.value }))}
                className="mt-1 block w-full rounded-lg border border-border bg-black/30 px-3 py-2 text-sm text-white outline-none transition focus:border-primary"
                placeholder="选填，例：500000"
              />
            </label>

            <label className="block">
              <span className="text-xs text-white/55">可承受最大回撤（%）</span>
              <select
                value={investForm.max_drawdown_pct}
                onChange={(e) => setInvestForm((f) => ({ ...f, max_drawdown_pct: e.target.value }))}
                className="mt-1 block w-full rounded-lg border border-border bg-black/30 px-3 py-2 text-sm text-white outline-none transition focus:border-primary"
              >
                <option value="">请选择</option>
                <option value="5">5% — 几乎不能接受亏损</option>
                <option value="10">10% — 可接受小幅回撤</option>
                <option value="20">20% — 可接受中等回撤</option>
                <option value="30">30% — 可接受较大回撤</option>
                <option value="50">50% — 能承受大幅亏损</option>
              </select>
            </label>
          </div>

          <label className="block">
            <span className="text-xs text-white/55">补充说明（选填）</span>
            <textarea
              value={investForm.note}
              onChange={(e) => setInvestForm((f) => ({ ...f, note: e.target.value }))}
              rows={2}
              className="mt-1 block w-full resize-none rounded-lg border border-border bg-black/30 px-3 py-2 text-sm text-white outline-none transition focus:border-primary"
              placeholder="例：主要关注港股科技板块，偏好有回购计划的公司"
            />
          </label>

          <button
            type="button"
            disabled={investSaving}
            onClick={async () => {
              setInvestSaving(true)
              setInvestNotice('')
              try {
                const payload = {
                  total_capital: Number(investForm.total_capital) || 0,
                  risk_preference: investForm.risk_preference,
                  investment_goal: investForm.investment_goal,
                  investment_horizon: investForm.investment_horizon,
                  max_drawdown_pct: Number(investForm.max_drawdown_pct) || 0,
                  experience_level: investForm.experience_level,
                  note: investForm.note,
                }
                const result = await requestJson('/api/investment-profile', {
                  method: 'PUT',
                  headers: { 'Content-Type': 'application/json' },
                  body: JSON.stringify(payload),
                })
                if (result?.profile) setInvestProfile(result.profile)
                setInvestNotice('投资画像已保存')
              } catch (err) {
                applyError(err, '投资画像保存失败')
              } finally {
                setInvestSaving(false)
              }
            }}
            className="rounded-lg bg-primary px-4 py-1.5 text-xs font-medium text-white shadow-sm transition hover:bg-primary/85 disabled:cursor-not-allowed disabled:opacity-60"
          >
            {investSaving ? '保存中...' : '保存投资画像'}
          </button>
        </div>
      </section>

      <section className="rounded-2xl border border-border bg-card p-5">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h2 className="text-base font-semibold text-white">Webhook 推送配置</h2>
            <p className="mt-1 text-xs text-white/60">用于接收所有股票的交易信号。仅支持 HTTPS URL；Secret 为可选，配置后会附带签名头。测试按钮仅在真实送达后才会提示成功。</p>
          </div>
          <div className="text-xs text-white/55">
            {webhookConfig.updated_at ? `配置更新时间：${formatDateTime(webhookConfig.updated_at)}` : '未配置'}
          </div>
        </div>

        {error ? (
          <div className="mt-3 rounded-xl border border-rose-400/40 bg-rose-500/10 px-4 py-3 text-sm text-rose-200">
            <div>{error}</div>
            {errorNeedsLogin ? (
              <button
                type="button"
                onClick={() => openAuthModal('login', 'Webhook 配置需要登录后才能继续。')}
                className="mt-2 inline-flex rounded-lg border border-rose-300/40 px-2.5 py-1 text-xs text-rose-100 transition hover:bg-rose-500/15"
              >
                去登录
              </button>
            ) : null}
          </div>
        ) : null}

        {notice ? (
          <div className="mt-3 rounded-xl border border-emerald-400/40 bg-emerald-500/10 px-4 py-3 text-sm text-emerald-200">{notice}</div>
        ) : null}

        <div className="mt-4 space-y-3 rounded-xl border border-border bg-black/20 p-4">
          <input
            value={webhookConfig.url}
            onChange={(event) => setWebhookConfig((prev) => ({ ...prev, url: event.target.value.trim() }))}
            placeholder="https://example.com/webhook"
            className="w-full rounded-lg border border-border bg-black/30 px-3 py-2 text-sm text-white outline-none transition focus:border-primary"
          />

          <input
            value={secretInput}
            onChange={(event) => setSecretInput(event.target.value)}
            placeholder={webhookConfig.has_secret ? '留空表示不修改 Secret；输入可更新签名密钥' : '可选：输入 Secret 启用签名；留空则不签名发送'}
            className="w-full rounded-lg border border-border bg-black/30 px-3 py-2 text-sm text-white outline-none transition focus:border-primary"
          />

          <div className="text-xs text-white/55">
            当前签名状态：{webhookConfig.has_secret ? <span className="text-emerald-300">已启用（HMAC）</span> : <span className="text-amber-300">未启用（可选）</span>}
          </div>

          <div className="grid gap-3 md:grid-cols-2">
            <label className="text-xs text-white/70">
              超时（毫秒）
              <input
                type="number"
                min={1000}
                max={10000}
                value={webhookConfig.timeout_ms}
                onChange={(event) => setWebhookConfig((prev) => ({ ...prev, timeout_ms: Number(event.target.value) || 3000 }))}
                className="mt-1 w-full rounded-lg border border-border bg-black/30 px-3 py-2 text-sm text-white outline-none transition focus:border-primary"
              />
            </label>

            <label className="flex items-end gap-2 text-sm text-white/80">
              <input
                type="checkbox"
                checked={webhookConfig.is_enabled}
                onChange={(event) => setWebhookConfig((prev) => ({ ...prev, is_enabled: event.target.checked }))}
              />
              启用 Webhook 推送
            </label>
          </div>

          <div className="flex flex-wrap gap-2">
            <button
              type="button"
              disabled={saving}
              onClick={handleSaveWebhook}
              className="rounded-lg bg-primary px-3 py-1.5 text-xs font-medium text-black transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
            >
              {saving ? '保存中...' : '保存 Webhook'}
            </button>
            <button
              type="button"
              disabled={testing}
              onClick={handleTestWebhook}
              className="rounded-lg border border-border px-3 py-1.5 text-xs text-white/80 transition hover:border-primary hover:text-primary disabled:cursor-not-allowed disabled:opacity-60"
            >
              {testing ? '送达校验中...' : '验证 Webhook 送达'}
            </button>
          </div>
        </div>
      </section>

      <section className="rounded-2xl border border-border bg-card p-5">
        <h2 className="text-base font-semibold text-white">Webhook 投递可观测</h2>

        <div className="mt-4 grid gap-4 lg:grid-cols-[1fr_1fr]">
          <div className="space-y-3 rounded-xl border border-border bg-black/20 p-4">
            <div className="text-sm font-semibold text-white">最近发送状态</div>
            {!latestDelivery ? (
              <div className="rounded-lg border border-dashed border-border px-3 py-4 text-xs text-white/50">暂无投递记录</div>
            ) : (
              <div className="space-y-2 text-xs text-white/75">
                <div>标的：{latestDelivery.symbol || '--'}</div>
                <div>状态：<span className={deliveryStatusColor(latestDelivery.status)}>{formatDeliveryStatus(latestDelivery.status)}</span></div>
                <div>HTTP：{latestDelivery.http_status || '--'} · 耗时：{latestDelivery.latency_ms ?? '--'}ms</div>
                <div>时间：{formatDateTime(latestDelivery.updated_at)}</div>
                {latestDelivery.error_message ? <div className="text-rose-300">错误：{latestDelivery.error_message}</div> : null}
              </div>
            )}
          </div>

          <div className="space-y-2 rounded-xl border border-border bg-black/20 p-4">
            <div className="text-sm font-semibold text-white">最近 20 次投递</div>
            {!deliveryItems.length ? (
              <div className="rounded-lg border border-dashed border-border px-3 py-4 text-xs text-white/50">暂无投递记录</div>
            ) : (
              <div className="space-y-2">
                {deliveryItems.map((item) => (
                  <div key={`${item.event_id}-${item.updated_at}`} className="rounded-lg border border-border bg-black/30 px-3 py-2 text-xs text-white/75">
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <div className="font-medium text-white">{item.symbol || '--'} · {item.event_id}</div>
                      <div className={deliveryStatusColor(item.status)}>{formatDeliveryStatus(item.status)}</div>
                    </div>
                    <div className="mt-1">Attempt {item.attempt_no} · HTTP {item.http_status || '--'} · {item.latency_ms ?? '--'}ms · {formatDateTime(item.updated_at)}</div>
                    {item.error_message ? <div className="mt-1 text-rose-300">{item.error_message}</div> : null}
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>
      </section>
    </div>
  )
}

function formatDeliveryStatus(status) {
  const normalized = String(status || '').trim().toLowerCase()
  const labels = {
    pending: '待发送',
    processing: '发送中',
    retrying: '重试中',
    delivered: '已送达',
    failed: '已失败',
  }
  return labels[normalized] || normalized || '--'
}

function deliveryStatusColor(status) {
  const normalized = String(status || '').trim().toLowerCase()
  if (normalized === 'delivered') return 'text-emerald-300'
  if (normalized === 'failed') return 'text-rose-300'
  if (normalized === 'retrying') return 'text-amber-300'
  return 'text-white/75'
}

function formatDateTime(value) {
  if (!value) return '--'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString('zh-CN', { hour12: false })
}
