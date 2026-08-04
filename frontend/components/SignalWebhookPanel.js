import { useEffect, useState } from 'react'

import { requestJson } from '../lib/api'

// 信号中心「通知设置」的 webhook 进阶面板。
// 自包含组件：自行加载/保存配置、发送测试、展示最近投递。
// 从设置页迁入（spec signal-center：webhook 为进阶通道，折叠在信号中心内）。

const WEBHOOK_CHANNEL_OPTIONS = [
  {
    value: 'wecom',
    label: '企业微信',
    docLabel: '企业微信 Webhook 配置教程',
    docHref: 'https://open.work.weixin.qq.com/help2/pc/14931',
  },
  {
    value: 'feishu',
    label: '飞书',
    docLabel: '飞书 Webhook 配置教程',
    docHref: 'https://open.feishu.cn/document/client-docs/bot-v3/add-custom-bot?lang=zh-CN',
  },
]

function emptyWebhookForm() {
  return { url: '', channel: 'wecom', has_secret: false, is_enabled: true, timeout_ms: 3000, updated_at: '' }
}

function normalizeWebhookItem(item) {
  if (!item) return emptyWebhookForm()
  return {
    url: item.url || '',
    channel: item.channel || 'wecom',
    has_secret: Boolean(item.has_secret),
    is_enabled: item.is_enabled !== false,
    timeout_ms: Number(item.timeout_ms) > 0 ? Number(item.timeout_ms) : 3000,
    updated_at: item.updated_at || '',
  }
}

function formatDeliveryStatus(status) {
  const map = { pending: '待发送', processing: '发送中', retrying: '重试中', delivered: '已送达', failed: '已失败' }
  return map[String(status || '').toLowerCase()] || status || '未知'
}

export default function SignalWebhookPanel() {
  const [config, setConfig] = useState(emptyWebhookForm)
  const [secretInput, setSecretInput] = useState('')
  const [deliveries, setDeliveries] = useState([])
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState(false)
  const [notice, setNotice] = useState('')
  const [error, setError] = useState('')
  const [loaded, setLoaded] = useState(false)

  const load = async () => {
    const [webhookData, deliveriesData] = await Promise.all([
      requestJson('/api/webhook'),
      requestJson('/api/webhook-deliveries?limit=5').catch(() => ({ items: [] })),
    ])
    setConfig(normalizeWebhookItem(webhookData?.item))
    setDeliveries(Array.isArray(deliveriesData?.items) ? deliveriesData.items : [])
  }

  useEffect(() => {
    load()
      .catch((err) => setError(err.message || 'Webhook 配置加载失败'))
      .finally(() => setLoaded(true))
  }, [])

  const handleSave = async () => {
    setSaving(true)
    setNotice('')
    setError('')
    try {
      const result = await requestJson('/api/webhook', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          url: config.url,
          channel: config.channel,
          secret: secretInput,
          is_enabled: config.is_enabled,
          timeout_ms: Number(config.timeout_ms) || 3000,
        }),
      })
      setConfig(normalizeWebhookItem(result?.item))
      setSecretInput('')
      setNotice('Webhook 配置已保存')
    } catch (err) {
      setError(err.message || '保存 Webhook 配置失败')
    } finally {
      setSaving(false)
    }
  }

  const handleTest = async () => {
    setTesting(true)
    setNotice('')
    setError('')
    try {
      await requestJson('/api/webhook/test', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ symbol: '00700.HK', side: 'BUY' }),
      })
      await load().catch(() => null)
      setNotice('测试 Webhook 已送达，请查看最近投递')
    } catch (err) {
      await load().catch(() => null)
      setError(err.message || '测试 Webhook 未送达')
    } finally {
      setTesting(false)
    }
  }

  if (!loaded) {
    return <div className="text-xs text-foreground-dim">加载通知配置...</div>
  }

  const channelMeta = WEBHOOK_CHANNEL_OPTIONS.find((item) => item.value === config.channel) || WEBHOOK_CHANNEL_OPTIONS[0]

  return (
    <div className="space-y-3">
      {error ? (
        <div className="rounded-lg border border-negative/40 bg-negative/10 px-3 py-2 text-xs text-negative">{error}</div>
      ) : null}
      {notice ? (
        <div className="rounded-lg border border-emerald-400/40 bg-positive/10 px-3 py-2 text-xs text-positive">{notice}</div>
      ) : null}

      <div className="grid gap-2 sm:grid-cols-2">
        {WEBHOOK_CHANNEL_OPTIONS.map((option) => (
          <button
            key={option.value}
            type="button"
            onClick={() => setConfig((prev) => ({ ...prev, channel: option.value }))}
            className={
              config.channel === option.value
                ? 'rounded-lg border border-primary bg-primary/10 px-3 py-2 text-left text-sm text-foreground transition'
                : 'rounded-lg border border-border bg-[var(--color-bg-hover)] px-3 py-2 text-left text-sm text-foreground-muted transition hover:border-[var(--color-border-strong)]'
            }
          >
            {option.label}
          </button>
        ))}
      </div>

      <input
        value={config.url}
        onChange={(event) => setConfig((prev) => ({ ...prev, url: event.target.value.trim() }))}
        placeholder="https://example.com/webhook"
        className="w-full rounded-lg border border-border bg-[var(--color-bg-overlay)] px-3 py-2 text-sm text-foreground outline-none transition focus:border-primary"
      />

      <input
        value={secretInput}
        onChange={(event) => setSecretInput(event.target.value)}
        placeholder={config.has_secret ? '留空表示不修改 Secret；输入可更新签名密钥' : '可选：输入机器人签名密钥；留空则不启用签名'}
        className="w-full rounded-lg border border-border bg-[var(--color-bg-overlay)] px-3 py-2 text-sm text-foreground outline-none transition focus:border-primary"
      />

      <div className="flex flex-wrap items-center gap-4">
        <label className="text-xs text-foreground-muted">
          超时（毫秒）
          <input
            type="number"
            min={1000}
            max={10000}
            value={config.timeout_ms}
            onChange={(event) => setConfig((prev) => ({ ...prev, timeout_ms: Number(event.target.value) || 3000 }))}
            className="ml-2 w-24 rounded-lg border border-border bg-[var(--color-bg-overlay)] px-2 py-1 text-xs text-foreground outline-none transition focus:border-primary"
          />
        </label>
        <label className="flex items-center gap-2 text-xs text-foreground-muted">
          <input
            type="checkbox"
            checked={config.is_enabled}
            onChange={(event) => setConfig((prev) => ({ ...prev, is_enabled: event.target.checked }))}
          />
          启用 Webhook 推送
        </label>
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <button
          type="button"
          disabled={saving}
          onClick={handleSave}
          className="rounded-lg bg-primary px-3 py-1.5 text-xs font-medium text-black transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
        >
          {saving ? '保存中...' : '保存 Webhook'}
        </button>
        <button
          type="button"
          disabled={testing}
          onClick={handleTest}
          className="rounded-lg border border-border px-3 py-1.5 text-xs text-foreground-muted transition hover:border-primary hover:text-primary disabled:cursor-not-allowed disabled:opacity-60"
        >
          {testing ? '送达校验中...' : '验证 Webhook 送达'}
        </button>
        <a
          href={channelMeta.docHref}
          target="_blank"
          rel="noopener noreferrer"
          className="text-xs text-primary/80 underline underline-offset-2 hover:text-primary"
        >
          {channelMeta.docLabel}
        </a>
      </div>

      {deliveries.length > 0 ? (
        <div className="rounded-lg border border-border bg-[var(--color-bg-overlay)] p-3">
          <div className="mb-2 text-xs font-medium text-foreground-muted">最近投递</div>
          <ul className="space-y-1 text-[11px] text-foreground-dim">
            {deliveries.map((item, index) => (
              <li key={`${item.event_id}-${index}`} className="flex items-center justify-between gap-2">
                <span className="font-mono">{item.symbol || '--'}</span>
                <span className={item.status === 'delivered' ? 'text-positive' : item.status === 'failed' ? 'text-negative' : ''}>
                  {formatDeliveryStatus(item.status)}
                </span>
              </li>
            ))}
          </ul>
        </div>
      ) : null}
    </div>
  )
}
