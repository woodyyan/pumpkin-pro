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
  const [error, setError] = useState('')
  const [errorNeedsLogin, setErrorNeedsLogin] = useState(false)
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

  const applyError = (err, fallbackText) => {
    setError(err.message || fallbackText)
    setErrorNeedsLogin(isAuthRequiredError(err))
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
      await loadInvestmentProfile()
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
      setInvestProfile(null)
      setInvestForm(createInvestForm())
      clearInvestmentProfileCache()
      setInvestNotice('')
      setError('')
      setErrorNeedsLogin(false)
      return
    }

    loadPage()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ready, isLoggedIn, authIdentityKey])

  return (
    <div className="space-y-6">
      <Head>
        <title>设置 — 卧龙AI量化交易台</title>
        <meta name="description" content="卧龙AI量化交易台设置 — 管理信号通知偏好、投资画像等个人设置。机器人推送配置已迁入信号中心。" />
        <link rel="canonical" href="https://wolongtrader.top/settings" />
      </Head>
      <section className="rounded-2xl border border-border bg-card p-8">
        <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <h1 className="text-2xl font-semibold tracking-tight">设置</h1>
          <CommunityQRCard variant="inline" />
        </div>
      </section>

      {error ? (
        <div className="rounded-xl border border-negative/40 bg-negative/10 px-4 py-3 text-sm text-negative">
          <div>{error}</div>
          {errorNeedsLogin ? (
            <button
              type="button"
              onClick={() => openAuthModal('login', '该操作需要登录后才能继续。')}
              className="mt-2 inline-flex rounded-lg border border-negative/40 px-2.5 py-1 text-xs text-negative transition hover:bg-negative/15"
            >
              去登录
            </button>
          ) : null}
        </div>
      ) : null}

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

      {/* Webhook 推送已迁入信号中心（spec signal-center） */}
      <section className="rounded-2xl border border-border bg-card p-5">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h2 className="text-base font-semibold text-foreground">信号通知</h2>
            <p className="mt-1 text-xs text-foreground-muted">
              站内提醒默认开启，无需配置；企业微信/飞书机器人推送与投递记录已迁入「信号中心 → 通知设置」。
            </p>
          </div>
          <a
            href="/signals"
            className="inline-flex shrink-0 items-center justify-center rounded-lg bg-primary px-3 py-1.5 text-xs font-medium text-black transition hover:opacity-90"
          >
            前往信号中心
          </a>
        </div>
      </section>

      <section id="feedback" className="rounded-2xl border border-border bg-card p-5 scroll-mt-20">
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

function formatDateTime(value) {
  if (!value) return '--'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString('zh-CN', { hour12: false })
}
