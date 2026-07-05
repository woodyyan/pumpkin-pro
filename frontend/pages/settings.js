import { useEffect, useState } from 'react'

import { requestJson } from '../lib/api'
import { useAuth } from '../lib/auth-context'
import { isAuthRequiredError } from '../lib/auth-storage'
import CommunityQRCard from '../components/CommunityQRCard'
import {
  describeFeeRate,
  formatFeeRatePercent,
  getPortfolioDefaultFeeRate,
  getPortfolioSystemDefaultFeeRate,
  parseFeeRatePercentInput,
} from '../lib/portfolio-fee.js'
import {
  clearInvestmentProfileCache,
  dispatchInvestmentProfileUpdated,
  writeInvestmentProfileCache,
} from '../lib/investment-profile-storage.js'
import {
  isIOSSafari,
  isNotificationSupported,
  loadNotificationPreferences,
  saveNotificationPreferences,
} from '../lib/notification.js'
import Head from 'next/head'

const WEBHOOK_CHANNEL_OPTIONS = [
  {
    value: 'wecom',
    label: '企业微信',
    docLabel: '企业微信 Webhook 配置教程',
    docHref: 'https://open.work.weixin.qq.com/help2/pc/14931',
    payloadPreview: {
      msgtype: 'text',
      text: {
        content: '股票交易信号来啦！\n类型：正式信号\n股票：00700.HK\n方向：BUY\n时间：2026-03-30 18:00:00\n策略：均线金叉策略\n原因：策略触发原因说明',
      },
    },
  },
  {
    value: 'feishu',
    label: '飞书',
    docLabel: '飞书 Webhook 配置教程',
    docHref: 'https://open.feishu.cn/document/client-docs/bot-v3/add-custom-bot?lang=zh-CN',
    payloadPreview: {
      msg_type: 'text',
      content: {
        text: '股票交易信号来啦！\n类型：正式信号\n股票：00700.HK\n方向：BUY\n时间：2026-03-30 18:00:00\n策略：均线金叉策略\n原因：策略触发原因说明',
      },
    },
  },
]

function getWebhookChannelMeta(channel) {
  return WEBHOOK_CHANNEL_OPTIONS.find((item) => item.value === channel) || WEBHOOK_CHANNEL_OPTIONS[0]
}

function createInvestForm(profile = null) {
  return {
    total_capital: profile?.total_capital ? String(profile.total_capital) : '',
    risk_preference: profile?.risk_preference || '',
    investment_goal: profile?.investment_goal || '',
    investment_horizon: profile?.investment_horizon || '',
    max_drawdown_pct: profile?.max_drawdown_pct ? String(profile.max_drawdown_pct) : '',
    experience_level: profile?.experience_level || '',
    default_fee_rate_ashare_buy: formatFeeRatePercent(getPortfolioDefaultFeeRate({ exchange: 'ASHARE', action: 'buy', profile })),
    default_fee_rate_ashare_sell: formatFeeRatePercent(getPortfolioDefaultFeeRate({ exchange: 'ASHARE', action: 'sell', profile })),
    default_fee_rate_hk_buy: formatFeeRatePercent(getPortfolioDefaultFeeRate({ exchange: 'HKEX', action: 'buy', profile })),
    default_fee_rate_hk_sell: formatFeeRatePercent(getPortfolioDefaultFeeRate({ exchange: 'HKEX', action: 'sell', profile })),
    note: profile?.note || '',
  }
}

export default function SettingsPage() {
  const { openAuthModal, isLoggedIn, ready, user } = useAuth()
  const [webhookConfig, setWebhookConfig] = useState({
    url: '',
    channel: 'wecom',
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
  const [investForm, setInvestForm] = useState(() => createInvestForm())
  const [investSaving, setInvestSaving] = useState(false)
  const [investNotice, setInvestNotice] = useState('')
  const [fbCategory, setFbCategory] = useState('bug')
  const [fbContent, setFbContent] = useState('')
  const [fbContact, setFbContact] = useState('')
  const [fbSaving, setFbSaving] = useState(false)
  const [fbNotice, setFbNotice] = useState('')
  const [fbError, setFbError] = useState('')
  const [notifPrefs, setNotifPrefs] = useState(() => loadNotificationPreferences())
  const [notifSupported, setNotifSupported] = useState(false)
  const authIdentityKey = String(user?.id || user?.email || '')
  const webhookChannelMeta = getWebhookChannelMeta(webhookConfig.channel)

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
        channel: 'wecom',
        has_secret: false,
        is_enabled: true,
        timeout_ms: 3000,
        updated_at: '',
      })
      return
    }

    setWebhookConfig({
      url: item.url || '',
      channel: item.channel || 'wecom',
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
      setInvestForm(createInvestForm(p))
      if (p) writeInvestmentProfileCache(p)
      else clearInvestmentProfileCache()
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
    setNotifSupported(isNotificationSupported())
  }, [])

  useEffect(() => {
    if (!ready) return

    if (!isLoggedIn) {
      setWebhookConfig({
        url: '',
        channel: 'wecom',
        has_secret: false,
        is_enabled: true,
        timeout_ms: 3000,
        updated_at: '',
      })
      setSecretInput('')
      setLatestDelivery(null)
      setDeliveryItems([])
      setInvestProfile(null)
      setInvestForm(createInvestForm())
      clearInvestmentProfileCache()
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
          channel: webhookConfig.channel,
          secret: secretInput,
          is_enabled: webhookConfig.is_enabled,
          timeout_ms: Number(webhookConfig.timeout_ms) || 3000,
        }),
      })

      const item = result?.item || null
      if (item) {
        setWebhookConfig({
          url: item.url || '',
          channel: item.channel || 'wecom',
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
      <Head>
        <title>设置 — 卧龙AI量化交易台</title>
        <meta name="description" content="卧龙AI量化交易台设置 — 管理 Webhook 推送配置、信号通知偏好、投资画像等个人设置。" />
        <link rel="canonical" href="https://wolongtrader.top/settings" />
      </Head>
      <section className="rounded-2xl border border-border bg-card p-8">
        <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <h1 className="text-2xl font-semibold tracking-tight">设置</h1>
          <CommunityQRCard variant="inline" />
        </div>
      </section>

      {/* Investment Profile */}
      <section className="rounded-2xl border border-border bg-card p-5">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h2 className="text-base font-semibold text-foreground">投资画像</h2>
            <p className="mt-1 text-xs text-foreground-muted">帮助系统了解你的投资风格，以便AI分析提供更精准的策略结果和风险提示。</p>
          </div>
          {investProfile?.updated_at && (
            <div className="text-xs text-foreground-dim">更新：{formatDateTime(investProfile.updated_at)}</div>
          )}
        </div>

        {investNotice && (
          <div className="mt-3 rounded-xl border border-emerald-400/40 bg-positive/10 px-4 py-3 text-sm text-positive">{investNotice}</div>
        )}

        <div className="mt-4 space-y-4 rounded-xl border border-border bg-[var(--color-bg-hover)] p-4">
          <div className="grid gap-4 md:grid-cols-2">
            <label className="block">
              <span className="text-xs text-foreground-dim">风险偏好</span>
              <select
                value={investForm.risk_preference}
                onChange={(e) => setInvestForm((f) => ({ ...f, risk_preference: e.target.value }))}
                className="mt-1 block w-full rounded-lg border border-border bg-[var(--color-bg-overlay)] px-3 py-2 text-sm text-foreground outline-none transition focus:border-primary"
              >
                <option value="">请选择</option>
                <option value="保守">保守 — 尽量不亏，收益低一点也可以</option>
                <option value="稳健">稳健 — 追求稳定增长，可接受小幅波动</option>
                <option value="积极">积极 — 愿意承受较大波动换取更高回报</option>
                <option value="激进">激进 — 高风险高回报，能承受大幅亏损</option>
              </select>
            </label>

            <label className="block">
              <span className="text-xs text-foreground-dim">投资目标</span>
              <select
                value={investForm.investment_goal}
                onChange={(e) => setInvestForm((f) => ({ ...f, investment_goal: e.target.value }))}
                className="mt-1 block w-full rounded-lg border border-border bg-[var(--color-bg-overlay)] px-3 py-2 text-sm text-foreground outline-none transition focus:border-primary"
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
              <span className="text-xs text-foreground-dim">投资周期</span>
              <select
                value={investForm.investment_horizon}
                onChange={(e) => setInvestForm((f) => ({ ...f, investment_horizon: e.target.value }))}
                className="mt-1 block w-full rounded-lg border border-border bg-[var(--color-bg-overlay)] px-3 py-2 text-sm text-foreground outline-none transition focus:border-primary"
              >
                <option value="">请选择</option>
                <option value="短期">短期 — 1 年以内</option>
                <option value="中期">中期 — 1~3 年</option>
                <option value="长期">长期 — 3 年以上</option>
              </select>
            </label>

            <label className="block">
              <span className="text-xs text-foreground-dim">投资经验</span>
              <select
                value={investForm.experience_level}
                onChange={(e) => setInvestForm((f) => ({ ...f, experience_level: e.target.value }))}
                className="mt-1 block w-full rounded-lg border border-border bg-[var(--color-bg-overlay)] px-3 py-2 text-sm text-foreground outline-none transition focus:border-primary"
              >
                <option value="">请选择</option>
                <option value="新手">新手 — 刚开始接触股票投资</option>
                <option value="进阶">进阶 — 有 1~3 年投资经验</option>
                <option value="资深">资深 — 有 3 年以上投资经验</option>
                <option value="专业">专业 — 金融从业或全职投资</option>
              </select>
            </label>

            <label className="block">
              <span className="text-xs text-foreground-dim">账户总资金（元）</span>
              <input
                type="number"
                min="0"
                step="any"
                value={investForm.total_capital}
                onChange={(e) => setInvestForm((f) => ({ ...f, total_capital: e.target.value }))}
                className="mt-1 block w-full rounded-lg border border-border bg-[var(--color-bg-overlay)] px-3 py-2 text-sm text-foreground outline-none transition focus:border-primary"
                placeholder="选填，例：500000"
              />
            </label>

            <label className="block">
              <span className="text-xs text-foreground-dim">可承受最大回撤（%）</span>
              <select
                value={investForm.max_drawdown_pct}
                onChange={(e) => setInvestForm((f) => ({ ...f, max_drawdown_pct: e.target.value }))}
                className="mt-1 block w-full rounded-lg border border-border bg-[var(--color-bg-overlay)] px-3 py-2 text-sm text-foreground outline-none transition focus:border-primary"
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

          <div className="rounded-xl border border-border bg-[var(--color-bg-hover)] p-4">
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div>
                <div className="text-sm font-semibold text-foreground">默认手续费率</div>
                <div className="mt-1 text-xs leading-6 text-foreground-dim">买入 / 卖出表单会自动带出这里的费率。A 股小额买卖若按费率估算低于 ¥5.00，会按最低佣金 ¥5.00 估算。</div>
              </div>
              <div className="rounded-full border border-border px-2 py-0.5 text-[10px] text-foreground-dim">可随时手动修改</div>
            </div>

            <div className="mt-4 grid gap-4 lg:grid-cols-2">
              <div className="rounded-xl border border-border bg-[var(--color-bg-hover)] p-4">
                <div className="text-xs font-semibold uppercase tracking-[0.2em] text-foreground-dim">A股</div>
                <div className="mt-3 grid gap-3 sm:grid-cols-2">
                  <label className="block">
                    <span className="text-xs text-foreground-dim">买入默认费率（%）</span>
                    <input
                      type="number"
                      min="0"
                      step="any"
                      value={investForm.default_fee_rate_ashare_buy}
                      onChange={(e) => setInvestForm((f) => ({ ...f, default_fee_rate_ashare_buy: e.target.value }))}
                      className="mt-1 block w-full rounded-lg border border-border bg-[var(--color-bg-overlay)] px-3 py-2 text-sm text-foreground outline-none transition focus:border-primary"
                      placeholder="默认 0.03"
                    />
                    <div className="mt-1 text-[11px] text-foreground-dim">{describeFeeRate(parseFeeRatePercentInput(investForm.default_fee_rate_ashare_buy) ?? 0)}</div>
                  </label>
                  <label className="block">
                    <span className="text-xs text-foreground-dim">卖出默认费率（%）</span>
                    <input
                      type="number"
                      min="0"
                      step="any"
                      value={investForm.default_fee_rate_ashare_sell}
                      onChange={(e) => setInvestForm((f) => ({ ...f, default_fee_rate_ashare_sell: e.target.value }))}
                      className="mt-1 block w-full rounded-lg border border-border bg-[var(--color-bg-overlay)] px-3 py-2 text-sm text-foreground outline-none transition focus:border-primary"
                      placeholder="默认 0.08"
                    />
                    <div className="mt-1 text-[11px] text-foreground-dim">{describeFeeRate(parseFeeRatePercentInput(investForm.default_fee_rate_ashare_sell) ?? 0)}</div>
                  </label>
                </div>
              </div>

              <div className="rounded-xl border border-border bg-[var(--color-bg-hover)] p-4">
                <div className="text-xs font-semibold uppercase tracking-[0.2em] text-foreground-dim">港股</div>
                <div className="mt-3 grid gap-3 sm:grid-cols-2">
                  <label className="block">
                    <span className="text-xs text-foreground-dim">买入默认费率（%）</span>
                    <input
                      type="number"
                      min="0"
                      step="any"
                      value={investForm.default_fee_rate_hk_buy}
                      onChange={(e) => setInvestForm((f) => ({ ...f, default_fee_rate_hk_buy: e.target.value }))}
                      className="mt-1 block w-full rounded-lg border border-border bg-[var(--color-bg-overlay)] px-3 py-2 text-sm text-foreground outline-none transition focus:border-primary"
                      placeholder="默认 0.13"
                    />
                    <div className="mt-1 text-[11px] text-foreground-dim">{describeFeeRate(parseFeeRatePercentInput(investForm.default_fee_rate_hk_buy) ?? 0)}</div>
                  </label>
                  <label className="block">
                    <span className="text-xs text-foreground-dim">卖出默认费率（%）</span>
                    <input
                      type="number"
                      min="0"
                      step="any"
                      value={investForm.default_fee_rate_hk_sell}
                      onChange={(e) => setInvestForm((f) => ({ ...f, default_fee_rate_hk_sell: e.target.value }))}
                      className="mt-1 block w-full rounded-lg border border-border bg-[var(--color-bg-overlay)] px-3 py-2 text-sm text-foreground outline-none transition focus:border-primary"
                      placeholder="默认 0.13"
                    />
                    <div className="mt-1 text-[11px] text-foreground-dim">{describeFeeRate(parseFeeRatePercentInput(investForm.default_fee_rate_hk_sell) ?? 0)}</div>
                  </label>
                </div>
              </div>
            </div>
          </div>

          <label className="block">
            <span className="text-xs text-foreground-dim">补充说明（选填）</span>
            <textarea
              value={investForm.note}
              onChange={(e) => setInvestForm((f) => ({ ...f, note: e.target.value }))}
              rows={2}
              className="mt-1 block w-full resize-none rounded-lg border border-border bg-[var(--color-bg-overlay)] px-3 py-2 text-sm text-foreground outline-none transition focus:border-primary"
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
                  default_fee_rate_ashare_buy: parseFeeRatePercentInput(investForm.default_fee_rate_ashare_buy) ?? getPortfolioSystemDefaultFeeRate({ exchange: 'ASHARE', action: 'buy' }),
                  default_fee_rate_ashare_sell: parseFeeRatePercentInput(investForm.default_fee_rate_ashare_sell) ?? getPortfolioSystemDefaultFeeRate({ exchange: 'ASHARE', action: 'sell' }),
                  default_fee_rate_hk_buy: parseFeeRatePercentInput(investForm.default_fee_rate_hk_buy) ?? getPortfolioSystemDefaultFeeRate({ exchange: 'HKEX', action: 'buy' }),
                  default_fee_rate_hk_sell: parseFeeRatePercentInput(investForm.default_fee_rate_hk_sell) ?? getPortfolioSystemDefaultFeeRate({ exchange: 'HKEX', action: 'sell' }),
                  note: investForm.note,
                }
                const result = await requestJson('/api/investment-profile', {
                  method: 'PUT',
                  headers: { 'Content-Type': 'application/json' },
                  body: JSON.stringify(payload),
                })
                if (result?.profile) {
                  setInvestProfile(result.profile)
                  setInvestForm(createInvestForm(result.profile))
                  writeInvestmentProfileCache(result.profile)
                  dispatchInvestmentProfileUpdated(result.profile)
                }
                setInvestNotice('投资画像已保存')
              } catch (err) {
                applyError(err, '投资画像保存失败')
              } finally {
                setInvestSaving(false)
              }
            }}
            className="rounded-lg bg-primary px-4 py-1.5 text-xs font-medium text-foreground shadow-sm transition hover:bg-primary/85 disabled:cursor-not-allowed disabled:opacity-60"
          >
            {investSaving ? '保存中...' : '保存投资画像'}
          </button>
        </div>
      </section>

      {/* Desktop Notifications */}
      {notifSupported && (
        <section className="rounded-2xl border border-border bg-card p-5">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <h2 className="text-base font-semibold text-foreground">桌面通知</h2>
              <p className="mt-1 text-xs text-foreground-muted">控制是否通过浏览器系统弹窗接收通知。</p>
            </div>
          </div>

          <div className="mt-4 space-y-3 rounded-xl border border-border bg-[var(--color-bg-hover)] p-4">
            <label className="flex cursor-pointer items-center gap-3">
              <input
                type="checkbox"
                checked={notifPrefs.aiAnalysis}
                onChange={(e) => {
                  const next = { ...notifPrefs, aiAnalysis: e.target.checked }
                  setNotifPrefs(next)
                  saveNotificationPreferences(next)
                }}
                className="h-4 w-4 rounded border-border bg-[var(--color-bg-overlay)] text-primary accent-primary"
              />
              <div>
                <div className="text-sm text-foreground-muted">AI 分析完成时通知我</div>
                <div className="text-[11px] text-foreground-dim">分析完成后通过桌面弹窗提醒，即使切换到其他标签页也能收到</div>
              </div>
            </label>

            {isIOSSafari() && (
              <div className="rounded-xl border border-amber-400/20 bg-amber-500/8 px-3.5 py-2.5 text-[12px] leading-6 text-amber-200/80">
                📱 iOS 用户：添加到主屏幕后可在后台接收通知
              </div>
            )}
          </div>
        </section>
      )}

      <section className="rounded-2xl border border-border bg-card p-5">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h2 className="text-base font-semibold text-foreground">Webhook 推送配置</h2>
            <p className="mt-1 text-xs text-foreground-muted">
              用于接收所有股票的网站信号。当前支持企业微信和飞书机器人。Secret签名为可选。
              不知道如何获取 Webhook 地址？可参考
              <a href="https://open.work.weixin.qq.com/help2/pc/14931" target="_blank" rel="noopener noreferrer" className="ml-0.5 text-primary/80 underline underline-offset-2 hover:text-primary">企业微信 Webhook 配置教程</a>
              {' '}或
              <a href="https://open.feishu.cn/document/client-docs/bot-v3/add-custom-bot?lang=zh-CN" target="_blank" rel="noopener noreferrer" className="ml-0.5 text-primary/80 underline underline-offset-2 hover:text-primary">飞书 Webhook 配置教程</a>。
            </p>
          </div>
          <div className="text-xs text-foreground-dim">
            {webhookConfig.updated_at ? `配置更新时间：${formatDateTime(webhookConfig.updated_at)}` : '未配置'}
          </div>
        </div>

        {error ? (
          <div className="mt-3 rounded-xl border border-negative/40 bg-negative/10 px-4 py-3 text-sm text-negative">
            <div>{error}</div>
            {errorNeedsLogin ? (
              <button
                type="button"
                onClick={() => openAuthModal('login', 'Webhook 配置需要登录后才能继续。')}
                className="mt-2 inline-flex rounded-lg border border-negative/40 px-2.5 py-1 text-xs text-negative transition hover:bg-negative/15"
              >
                去登录
              </button>
            ) : null}
          </div>
        ) : null}

        {notice ? (
          <div className="mt-3 rounded-xl border border-emerald-400/40 bg-positive/10 px-4 py-3 text-sm text-positive">{notice}</div>
        ) : null}

        <div className="mt-4 space-y-3 rounded-xl border border-border bg-[var(--color-bg-hover)] p-4">
          <div>
            <div className="mb-2 text-xs text-foreground-dim">推送渠道</div>
            <div className="grid gap-2 sm:grid-cols-2">
              {WEBHOOK_CHANNEL_OPTIONS.map((option) => (
                <button
                  key={option.value}
                  type="button"
                  onClick={() => setWebhookConfig((prev) => ({ ...prev, channel: option.value }))}
                  className={
                    webhookConfig.channel === option.value
                      ? 'rounded-lg border border-primary bg-primary/10 px-3 py-2 text-left transition shadow-[0_0_0_1px_rgba(230,126,34,0.2)]'
                      : 'rounded-lg border border-border bg-[var(--color-bg-hover)] px-3 py-2 text-left transition hover:border-[var(--color-border-strong)]'
                  }
                >
                  <div className="text-sm font-medium text-foreground">{option.label}</div>
                </button>
              ))}
            </div>
          </div>

          <input
            value={webhookConfig.url}
            onChange={(event) => setWebhookConfig((prev) => ({ ...prev, url: event.target.value.trim() }))}
            placeholder="https://example.com/webhook"
            className="w-full rounded-lg border border-border bg-[var(--color-bg-overlay)] px-3 py-2 text-sm text-foreground outline-none transition focus:border-primary"
          />

          <input
            value={secretInput}
            onChange={(event) => setSecretInput(event.target.value)}
            placeholder={webhookConfig.has_secret ? '留空表示不修改 Secret；输入可更新签名密钥' : '可选：输入机器人签名密钥；留空则不启用签名'}
            className="w-full rounded-lg border border-border bg-[var(--color-bg-overlay)] px-3 py-2 text-sm text-foreground outline-none transition focus:border-primary"
          />

          <div className="text-xs text-foreground-dim">
            当前签名状态：{webhookConfig.has_secret ? <span className="text-positive">已启用</span> : <span className="text-amber-300">未启用（可选）</span>}
          </div>

          <div className="grid gap-3 md:grid-cols-2">
            <label className="text-xs text-foreground-muted">
              超时（毫秒）
              <input
                type="number"
                min={1000}
                max={10000}
                value={webhookConfig.timeout_ms}
                onChange={(event) => setWebhookConfig((prev) => ({ ...prev, timeout_ms: Number(event.target.value) || 3000 }))}
                className="mt-1 w-full rounded-lg border border-border bg-[var(--color-bg-overlay)] px-3 py-2 text-sm text-foreground outline-none transition focus:border-primary"
              />
            </label>

            <label className="flex items-end gap-2 text-sm text-foreground-muted">
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
              className="rounded-lg border border-border px-3 py-1.5 text-xs text-foreground-muted transition hover:border-primary hover:text-primary disabled:cursor-not-allowed disabled:opacity-60"
            >
              {testing ? '送达校验中...' : '验证 Webhook 送达'}
            </button>
          </div>

          <details className="mt-3 rounded-lg border border-border/80 bg-[var(--color-bg-overlay)] p-3">
            <summary className="cursor-pointer text-xs font-medium text-foreground-muted">查看触发条件与 Payload 模板</summary>
            <div className="mt-3 space-y-3 text-xs text-foreground-muted">
              <div className="space-y-1">
                <div>推送渠道：当前为 {webhookChannelMeta.label} 机器人。</div>
                <div>评估周期：系统每小时自动评估一次已开启信号的股票策略。</div>
                <div>失败重试：最多 4 次，退避间隔 1 分钟 / 5 分钟 / 15 分钟。</div>
              </div>
              <div>
                <div className="mb-1 text-foreground-muted">Payload 模板（系统自动生成）</div>
                <pre className="overflow-x-auto rounded-lg border border-border/80 bg-[var(--color-bg-overlay)] p-2 text-[11px] leading-5 text-positive">
                  {JSON.stringify(webhookChannelMeta.payloadPreview, null, 2)}
                </pre>
              </div>
            </div>
          </details>
        </div>
      </section>

      <section className="rounded-2xl border border-border bg-card p-5">
        <h2 className="text-base font-semibold text-foreground">Webhook 投递可观测</h2>

        <div className="mt-4 grid gap-4 lg:grid-cols-[1fr_1fr]">
          <div className="space-y-3 rounded-xl border border-border bg-[var(--color-bg-hover)] p-4">
            <div className="text-sm font-semibold text-foreground">最近发送状态</div>
            {!latestDelivery ? (
              <div className="rounded-lg border border-dashed border-border px-3 py-4 text-xs text-foreground-dim">暂无投递记录</div>
            ) : (
              <div className="space-y-2 text-xs text-foreground-muted">
                <div>标的：{latestDelivery.symbol || '--'}</div>
                <div>状态：<span className={deliveryStatusColor(latestDelivery.status)}>{formatDeliveryStatus(latestDelivery.status)}</span></div>
                <div>HTTP：{latestDelivery.http_status || '--'} · 耗时：{latestDelivery.latency_ms ?? '--'}ms</div>
                <div>时间：{formatDateTime(latestDelivery.updated_at)}</div>
                {latestDelivery.error_message ? <div className="text-negative">错误：{latestDelivery.error_message}</div> : null}
              </div>
            )}
          </div>

          {!deliveryItems.length ? (
            <div className="space-y-2 rounded-xl border border-border bg-[var(--color-bg-hover)] p-4">
              <div className="text-sm font-semibold text-foreground">最近 20 次投递</div>
              <div className="rounded-lg border border-dashed border-border px-3 py-4 text-xs text-foreground-dim">暂无投递记录</div>
            </div>
          ) : (
            <details className="group rounded-xl border border-border bg-[var(--color-bg-hover)] p-4">
              <summary className="flex cursor-pointer list-none items-start justify-between gap-3 [&::-webkit-details-marker]:hidden">
                <div>
                  <div className="text-sm font-semibold text-foreground">最近 20 次投递</div>
                  <div className="mt-1 text-xs text-foreground-dim">默认收起，点开后查看最近的投递记录。</div>
                </div>
                <div className="flex items-center gap-2 text-xs text-foreground-dim">
                  <span className="rounded-full border border-border px-2 py-0.5 text-[11px] text-foreground-muted">{deliveryItems.length} 条</span>
                  <span className="group-open:hidden">展开</span>
                  <span className="hidden group-open:inline">收起</span>
                </div>
              </summary>

              <div className="mt-3 space-y-2 border-t border-border pt-3">
                {deliveryItems.map((item) => (
                  <div key={`${item.event_id}-${item.updated_at}`} className="rounded-lg border border-border bg-[var(--color-bg-overlay)] px-3 py-2 text-xs text-foreground-muted">
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <div className="font-medium text-foreground">{item.symbol || '--'} · {item.event_id}</div>
                      <div className={deliveryStatusColor(item.status)}>{formatDeliveryStatus(item.status)}</div>
                    </div>
                    <div className="mt-1">Attempt {item.attempt_no} · HTTP {item.http_status || '--'} · {item.latency_ms ?? '--'}ms · {formatDateTime(item.updated_at)}</div>
                    {item.error_message ? <div className="mt-1 text-negative">{item.error_message}</div> : null}
                  </div>
                ))}
              </div>
            </details>
          )}
        </div>
      </section>

      <section className="rounded-2xl border border-border bg-card p-5">
        <div>
          <h2 className="text-base font-semibold text-foreground">反馈与建议</h2>
          <p className="mt-1 text-xs text-foreground-muted">遇到问题或有想法？我们很想听到你的声音。</p>
        </div>

        {!isLoggedIn ? (
          <div className="mt-4 rounded-xl border border-dashed border-border bg-[var(--color-bg-hover)] px-4 py-6 text-center">
            <span className="text-sm text-foreground-dim">
              <button type="button" onClick={() => openAuthModal('login', '登录后可提交反馈和建议。')} className="text-primary hover:underline">登录</button>
              {' '}后可提交反馈和建议
            </span>
          </div>
        ) : (
          <div className="mt-4 space-y-4 rounded-xl border border-border bg-[var(--color-bg-hover)] p-4">
            <div>
              <div className="mb-2 text-xs text-foreground-dim">反馈类型</div>
              <div className="flex flex-wrap gap-2">
                {[
                  { value: 'bug', label: '🐛 Bug', desc: '系统报错或功能异常' },
                  { value: 'feature', label: '💡 功能建议', desc: '改进现有功能' },
                  { value: 'wish', label: '🌟 许愿池', desc: '想要全新功能' },
                ].map((opt) => (
                  <button
                    key={opt.value}
                    type="button"
                    onClick={() => setFbCategory(opt.value)}
                    className={`rounded-xl border px-3 py-2 text-left transition ${
                      fbCategory === opt.value
                        ? 'border-primary bg-primary/10 shadow-[0_0_0_1px_rgba(230,126,34,0.2)]'
                        : 'border-border bg-[var(--color-bg-hover)] hover:border-[var(--color-border-strong)]'
                    }`}
                  >
                    <div className="text-xs font-medium text-foreground">{opt.label}</div>
                    <div className="mt-0.5 text-[10px] text-foreground-dim">{opt.desc}</div>
                  </button>
                ))}
              </div>
            </div>

            <label className="block">
              <span className="text-xs text-foreground-dim">详细描述 *</span>
              <textarea
                value={fbContent}
                onChange={(e) => setFbContent(e.target.value)}
                rows={4}
                maxLength={2000}
                className="mt-1 block w-full resize-none rounded-lg border border-border bg-[var(--color-bg-overlay)] px-3 py-2 text-sm text-foreground outline-none transition placeholder:text-foreground-disabled focus:border-primary"
                placeholder="请描述你遇到的问题、期望的功能、或想要的改进..."
              />
              <div className="mt-1 text-right text-[10px] text-foreground-dim">{fbContent.length}/2000</div>
            </label>

            <label className="block">
              <span className="text-xs text-foreground-dim">联系方式（选填）</span>
              <input
                value={fbContact}
                onChange={(e) => setFbContact(e.target.value)}
                maxLength={128}
                className="mt-1 block w-full rounded-lg border border-border bg-[var(--color-bg-overlay)] px-3 py-2 text-sm text-foreground outline-none transition placeholder:text-foreground-disabled focus:border-primary"
                placeholder="微信号、邮箱或其他联系方式，方便我们跟进"
              />
            </label>

            {fbError ? (
              <div className="rounded-xl border border-negative/40 bg-negative/10 px-4 py-3 text-sm text-negative">{fbError}</div>
            ) : null}

            {fbNotice ? (
              <div className="rounded-xl border border-emerald-400/40 bg-positive/10 px-4 py-3 text-sm text-positive">{fbNotice}</div>
            ) : null}

            <button
              type="button"
              disabled={fbSaving || !fbContent.trim()}
              onClick={async () => {
                setFbSaving(true)
                setFbError('')
                setFbNotice('')
                try {
                  await requestJson('/api/feedback', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ category: fbCategory, content: fbContent.trim(), contact: fbContact.trim() }),
                  })
                  setFbNotice('反馈已提交，感谢你的宝贵意见！')
                  setFbContent('')
                  setFbContact('')
                } catch (err) {
                  setFbError(err.message || '提交反馈失败，请稍后重试')
                } finally {
                  setFbSaving(false)
                }
              }}
              className="w-full rounded-lg bg-primary px-4 py-2 text-sm font-medium text-foreground shadow-sm transition hover:bg-primary/85 disabled:cursor-not-allowed disabled:opacity-60 sm:w-auto"
            >
              {fbSaving ? '提交中...' : '提交反馈'}
            </button>
          </div>
        )}
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
  if (normalized === 'delivered') return 'text-positive'
  if (normalized === 'failed') return 'text-negative'
  if (normalized === 'retrying') return 'text-amber-300'
  return 'text-foreground-muted'
}

function formatDateTime(value) {
  if (!value) return '--'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString('zh-CN', { hour12: false })
}
