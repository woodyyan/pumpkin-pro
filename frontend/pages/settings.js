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

  const loadPage = async () => {
    try {
      setError('')
      await Promise.all([loadWebhookConfig(), loadDeliveries()])
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
          用户级能力统一在这里管理。Webhook 配置已从实盘页迁移到本页，实盘页仅保留股票级信号规则。
        </p>
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
