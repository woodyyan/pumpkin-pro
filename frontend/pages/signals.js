import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import Head from 'next/head'
import { useRouter } from 'next/router'

import SignalWebhookPanel from '../components/SignalWebhookPanel'
import { requestJson } from '../lib/api'
import { useAuth } from '../lib/auth-context'
import { isAuthRequiredError } from '../lib/auth-storage'
import {
  barStateMeta,
  buildCreateSubscriptionPayload,
  buildOverviewStats,
  buildParamsSummary,
  buildTemplateParamDraft,
  cstDateString,
  evalIntervalLabel,
  evalModeLabel,
  filterSignalEvents,
  groupSubscriptionsBySymbol,
  groupTemplatesByCategory,
  normalizeSignalEvents,
  normalizeSubscriptions,
  sideMeta,
  validateTemplateParamDraft,
} from '../lib/signal-center-ui'

const SEARCH_DEBOUNCE_MS = 300
const SEARCH_MIN_LEN = 2
const SEARCH_MAX_RESULTS = 8

const EVAL_INTERVAL_OPTIONS = [
  { value: 900, label: '每 15 分钟' },
  { value: 1800, label: '每 30 分钟' },
  { value: 3600, label: '每小时' },
  { value: 7200, label: '每 2 小时' },
  { value: 14400, label: '每 4 小时' },
]

export default function SignalsPage() {
  const router = useRouter()
  const { isLoggedIn, openAuthModal, ready } = useAuth()
  const privateAccessReady = ready && isLoggedIn

  const [templates, setTemplates] = useState([])
  const [subscriptions, setSubscriptions] = useState([])
  const [events, setEvents] = useState([])
  const [unreadCount, setUnreadCount] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [errorNeedsLogin, setErrorNeedsLogin] = useState(false)
  const [notice, setNotice] = useState('')

  const [filters, setFilters] = useState({ symbol: '', side: '', barState: '' })
  const [modalState, setModalState] = useState({ open: false, mode: 'create', editing: null, presetSymbol: '' })
  const [deletingId, setDeletingId] = useState('')
  const [togglingId, setTogglingId] = useState('')

  const applyError = (err, fallbackText) => {
    setError(err.message || fallbackText)
    setErrorNeedsLogin(isAuthRequiredError(err))
  }

  const loadAll = useCallback(async () => {
    const [templatesData, subscriptionsData, eventsData, unreadData] = await Promise.all([
      requestJson('/api/signal/templates'),
      requestJson('/api/signal/subscriptions'),
      requestJson('/api/signal/events?limit=100'),
      requestJson('/api/signal/events/unread-count'),
    ])
    setTemplates(Array.isArray(templatesData?.items) ? templatesData.items : [])
    setSubscriptions(normalizeSubscriptions(subscriptionsData?.items))
    setEvents(normalizeSignalEvents(eventsData?.items))
    setUnreadCount(Number(unreadData?.unread_count) || 0)
  }, [])

  useEffect(() => {
    if (!ready) return
    if (!privateAccessReady) {
      setTemplates([])
      setSubscriptions([])
      setEvents([])
      setUnreadCount(0)
      setLoading(false)
      return
    }
    let cancelled = false
    setLoading(true)
    loadAll()
      .then(() => {
        if (cancelled) return
        setError('')
        // 进入信号中心即标记全部已读（与导航角标乐观清零配套）。
        requestJson('/api/signal/events/mark-read', { method: 'POST' }).catch(() => {})
      })
      .catch((err) => {
        if (!cancelled) applyError(err, '信号数据加载失败')
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ready, privateAccessReady, loadAll])

  // 从个股详情页/自选股跳入：?symbol=XXX 自动打开新增弹窗并预选股票。
  useEffect(() => {
    if (!privateAccessReady || loading) return
    const preset = String(router.query.symbol || '').trim()
    if (preset) {
      setModalState({ open: true, mode: 'create', editing: null, presetSymbol: preset })
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [privateAccessReady, loading, router.query.symbol])

  const templateMap = useMemo(() => {
    const map = {}
    for (const tpl of templates) map[tpl.key] = tpl
    return map
  }, [templates])

  const symbolGroups = useMemo(() => groupSubscriptionsBySymbol(subscriptions), [subscriptions])

  const overview = useMemo(
    () => buildOverviewStats(subscriptions, events, unreadCount, cstDateString(new Date())),
    [subscriptions, events, unreadCount],
  )

  const filteredEvents = useMemo(() => filterSignalEvents(events, filters), [events, filters])

  const filterSymbolOptions = useMemo(() => {
    const seen = new Map()
    for (const sub of subscriptions) {
      if (sub.symbol && !seen.has(sub.symbol)) seen.set(sub.symbol, sub.symbol_name || sub.symbol)
    }
    for (const event of events) {
      if (event.symbol && !seen.has(event.symbol)) seen.set(event.symbol, event.symbol)
    }
    return [...seen.entries()].map(([symbol, name]) => ({ symbol, name }))
  }, [subscriptions, events])

  // ── 订阅操作 ──

  const handleToggle = async (sub) => {
    setTogglingId(sub.id)
    setError('')
    // 乐观更新，失败回滚（开关即点即生效）。
    setSubscriptions((prev) => prev.map((item) => (item.id === sub.id ? { ...item, is_enabled: !sub.is_enabled } : item)))
    try {
      await requestJson(`/api/signal/subscriptions/${encodeURIComponent(sub.id)}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ is_enabled: !sub.is_enabled }),
      })
    } catch (err) {
      setSubscriptions((prev) => prev.map((item) => (item.id === sub.id ? { ...item, is_enabled: sub.is_enabled } : item)))
      applyError(err, '切换信号状态失败')
    } finally {
      setTogglingId('')
    }
  }

  const handleDelete = async (sub) => {
    if (!window.confirm(`确认删除「${sub.symbol} ${sub.template_name}」信号？删除后不再评估与提醒。`)) return
    setDeletingId(sub.id)
    setError('')
    try {
      await requestJson(`/api/signal/subscriptions/${encodeURIComponent(sub.id)}`, { method: 'DELETE' })
      setSubscriptions((prev) => prev.filter((item) => item.id !== sub.id))
      setNotice('信号已删除')
    } catch (err) {
      applyError(err, '删除信号失败')
    } finally {
      setDeletingId('')
    }
  }

  const openCreateModal = (presetSymbol = '') => {
    setModalState({ open: true, mode: 'create', editing: null, presetSymbol })
  }

  const openEditModal = (sub) => {
    setModalState({ open: true, mode: 'edit', editing: sub, presetSymbol: '' })
  }

  const handleModalSaved = async (saved, mode) => {
    setModalState({ open: false, mode: 'create', editing: null, presetSymbol: '' })
    setNotice(mode === 'create' ? `已为 ${saved.symbol} 开启「${saved.template_name}」` : '信号配置已更新')
    try {
      const data = await requestJson('/api/signal/subscriptions')
      setSubscriptions(normalizeSubscriptions(data?.items))
    } catch {
      // 列表刷新失败不阻断；下次进入页面会重新加载。
    }
  }

  return (
    <div className="mx-auto max-w-5xl space-y-6">
      <Head>
        <title>信号中心 — 卧龙AI量化交易台</title>
        <meta name="description" content="卧龙AI量化交易台信号中心：为你的股票配置价格提醒与指标信号，盘中试算 + 收盘确认双状态，触发即时提醒。" />
        <link rel="canonical" href="https://wolongtrader.top/signals" />
      </Head>

      {/* 页头 + 概览条 */}
      <section className="rounded-2xl border border-border bg-card px-5 py-5">
        <div className="flex flex-col gap-3 md:flex-row md:items-end md:justify-between">
          <div>
            <div className="text-xs font-medium uppercase tracking-[0.18em] text-foreground-dim">Signal Center</div>
            <h1 className="mt-2 text-2xl font-semibold tracking-tight text-foreground md:text-3xl">信号中心</h1>
            <p className="mt-2 max-w-3xl text-sm leading-6 text-foreground-muted">
              为你的股票配置价格提醒与指标信号。盘中试算即时提醒，收盘后再确认一次，触发记录全部留档可复盘。
            </p>
          </div>
          {privateAccessReady ? (
            <button
              type="button"
              onClick={() => openCreateModal()}
              className="inline-flex shrink-0 items-center justify-center rounded-xl bg-primary px-4 py-2 text-sm font-semibold text-black transition hover:opacity-90"
            >
              + 新增信号
            </button>
          ) : null}
        </div>

        {privateAccessReady && !loading ? (
          <div className="mt-4 grid grid-cols-3 gap-2 sm:gap-3">
            <OverviewMetric label="已开启信号" value={overview.enabledCount} />
            <OverviewMetric label="今日触发" value={overview.todayCount} />
            <OverviewMetric label="未读提醒" value={overview.unreadCount} accent={overview.unreadCount > 0} />
          </div>
        ) : null}
      </section>

      {error ? (
        <div className="rounded-xl border border-negative/40 bg-negative/10 px-4 py-3 text-sm text-negative">
          <div>{error}</div>
          {errorNeedsLogin ? (
            <button
              type="button"
              onClick={() => openAuthModal('login', '信号中心需要登录后才能继续。')}
              className="mt-2 inline-flex rounded-lg border border-negative/40 px-2.5 py-1 text-xs text-negative transition hover:bg-negative/15"
            >
              去登录
            </button>
          ) : null}
        </div>
      ) : null}

      {notice ? (
        <div className="rounded-xl border border-emerald-400/40 bg-positive/10 px-4 py-3 text-sm text-positive">{notice}</div>
      ) : null}

      {!ready || loading ? (
        <div className="rounded-2xl border border-dashed border-border bg-card px-6 py-12 text-center text-sm text-foreground-dim">
          {!ready ? '正在确认账号状态...' : '信号数据加载中...'}
        </div>
      ) : !privateAccessReady ? (
        <section className="rounded-2xl border border-dashed border-primary/30 bg-primary/10 p-6">
          <div className="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
            <div className="space-y-2">
              <div className="text-lg font-semibold text-foreground">登录后开启信号提醒</div>
              <p className="max-w-2xl text-sm leading-7 text-foreground-muted">
                登录后即可为你的股票配置价格提醒与指标信号：涨破/跌破目标价、MACD 金叉、RSI 超卖等，触发后站内即时提醒。
              </p>
            </div>
            <button
              type="button"
              onClick={() => openAuthModal('login', '登录后即可配置信号提醒。')}
              className="inline-flex shrink-0 items-center justify-center rounded-xl bg-primary px-4 py-2 text-sm font-semibold text-black transition hover:opacity-90"
            >
              登录后继续
            </button>
          </div>
        </section>
      ) : (
        <>
          {/* 推荐配置区（P0 静态引导，P1 接入规则推荐引擎） */}
          {subscriptions.length === 0 ? (
            <section className="rounded-2xl border border-primary/30 bg-primary/5 p-5">
              <h2 className="text-base font-semibold text-foreground">开启你的第一个信号</h2>
              <p className="mt-1.5 text-xs leading-6 text-foreground-muted">
                无需创建策略，选择股票和信号模板即可开启。建议从「涨破/跌破提醒」或「MACD 金叉死叉」开始。
                添加自选股后，后续版本还会按你的关注标的自动推荐合适的信号。
              </p>
              <button
                type="button"
                onClick={() => openCreateModal()}
                className="mt-3 rounded-xl bg-primary px-4 py-2 text-sm font-semibold text-black transition hover:opacity-90"
              >
                选择股票并开启
              </button>
            </section>
          ) : null}

          {/* 我的信号 */}
          <section className="rounded-2xl border border-border bg-card p-5">
            <div className="flex items-center justify-between">
              <h2 className="text-base font-semibold text-foreground">我的信号</h2>
              <span className="text-xs text-foreground-dim">{overview.totalCount} 个配置 · {symbolGroups.length} 只股票</span>
            </div>

            {symbolGroups.length === 0 ? (
              <div className="mt-4 rounded-xl border border-dashed border-border px-4 py-8 text-center text-sm text-foreground-dim">
                还没有配置任何信号。点击右上角「新增信号」开始。
              </div>
            ) : (
              <div className="mt-4 space-y-3">
                {symbolGroups.map((group) => (
                  <div key={group.symbol} className="rounded-xl border border-border bg-[var(--color-bg-hover)] p-4">
                    <div className="flex items-center justify-between gap-2">
                      <div className="min-w-0">
                        <span className="text-sm font-semibold text-foreground">{group.symbol_name || group.symbol}</span>
                        {group.symbol_name ? (
                          <span className="ml-2 font-mono text-xs text-foreground-dim">{group.symbol}</span>
                        ) : null}
                      </div>
                      <div className="flex shrink-0 items-center gap-2">
                        <span className="rounded-full border border-border bg-[var(--color-bg-overlay)] px-2 py-0.5 text-[10px] text-foreground-muted">
                          {group.enabled_count} 个开启中
                        </span>
                        <button
                          type="button"
                          onClick={() => openCreateModal(group.symbol)}
                          className="rounded-lg border border-border px-2 py-0.5 text-[11px] text-foreground-muted transition hover:border-primary hover:text-primary"
                        >
                          + 加信号
                        </button>
                      </div>
                    </div>

                    <ul className="mt-3 space-y-2">
                      {group.items.map((sub) => (
                        <li
                          key={sub.id}
                          className="flex flex-col gap-2 rounded-lg border border-border bg-[var(--color-bg-overlay)] px-3 py-2.5 sm:flex-row sm:items-center sm:justify-between"
                        >
                          <div className="min-w-0">
                            <div className="flex flex-wrap items-center gap-2">
                              <span className="text-sm font-medium text-foreground">{sub.template_name}</span>
                              <span className="text-[11px] text-foreground-dim">{evalModeLabel(sub)}</span>
                            </div>
                            <div className="mt-0.5 text-xs text-foreground-dim">
                              {buildParamsSummary(templateMap[sub.template_key], sub.params) || '默认参数'}
                            </div>
                          </div>
                          <div className="flex shrink-0 items-center gap-2">
                            <button
                              type="button"
                              role="switch"
                              aria-checked={sub.is_enabled}
                              disabled={togglingId === sub.id}
                              onClick={() => handleToggle(sub)}
                              className={`inline-flex items-center gap-1.5 rounded-lg border px-2.5 py-1 text-[11px] transition disabled:opacity-60 ${
                                sub.is_enabled
                                  ? 'border-emerald-500/50 bg-emerald-500/10 text-emerald-600 dark:text-emerald-300'
                                  : 'border-border text-foreground-muted hover:border-[var(--color-border-strong)]'
                              }`}
                            >
                              {togglingId === sub.id ? '切换中...' : sub.is_enabled ? '已开启' : '已关闭'}
                            </button>
                            <button
                              type="button"
                              onClick={() => openEditModal(sub)}
                              className="rounded-lg border border-border px-2.5 py-1 text-[11px] text-foreground-muted transition hover:border-primary hover:text-primary"
                            >
                              编辑
                            </button>
                            <button
                              type="button"
                              disabled={deletingId === sub.id}
                              onClick={() => handleDelete(sub)}
                              className="rounded-lg px-2 py-1 text-[11px] text-negative/60 transition hover:bg-negative/10 hover:text-negative disabled:opacity-60"
                            >
                              {deletingId === sub.id ? '删除中...' : '删除'}
                            </button>
                          </div>
                        </li>
                      ))}
                    </ul>
                  </div>
                ))}
              </div>
            )}
          </section>

          {/* 信号记录 */}
          <section className="rounded-2xl border border-border bg-card p-5">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <h2 className="text-base font-semibold text-foreground">信号记录</h2>
              <div className="flex flex-wrap items-center gap-2 text-xs">
                <select
                  value={filters.symbol}
                  onChange={(e) => setFilters((prev) => ({ ...prev, symbol: e.target.value }))}
                  className="rounded-lg border border-border bg-[var(--color-bg-overlay)] px-2 py-1.5 text-xs text-foreground outline-none"
                >
                  <option value="">全部股票</option>
                  {filterSymbolOptions.map((option) => (
                    <option key={option.symbol} value={option.symbol}>{option.name}</option>
                  ))}
                </select>
                <select
                  value={filters.side}
                  onChange={(e) => setFilters((prev) => ({ ...prev, side: e.target.value }))}
                  className="rounded-lg border border-border bg-[var(--color-bg-overlay)] px-2 py-1.5 text-xs text-foreground outline-none"
                >
                  <option value="">全部方向</option>
                  <option value="BUY">买入提示</option>
                  <option value="SELL">卖出提示</option>
                </select>
                <select
                  value={filters.barState}
                  onChange={(e) => setFilters((prev) => ({ ...prev, barState: e.target.value }))}
                  className="rounded-lg border border-border bg-[var(--color-bg-overlay)] px-2 py-1.5 text-xs text-foreground outline-none"
                >
                  <option value="">全部状态</option>
                  <option value="intraday_provisional">盘中试算</option>
                  <option value="close_confirmed">收盘确认</option>
                  <option value="realtime">实时触发</option>
                </select>
              </div>
            </div>

            {filteredEvents.length === 0 ? (
              <div className="mt-4 rounded-xl border border-dashed border-border px-4 py-8 text-center text-sm text-foreground-dim">
                暂无信号记录。信号触发后会出现在这里。
              </div>
            ) : (
              <ul className="mt-4 space-y-2">
                {filteredEvents.map((event) => (
                  <SignalEventRow key={event.event_id} event={event} />
                ))}
              </ul>
            )}
          </section>

          {/* 通知设置 */}
          <section className="rounded-2xl border border-border bg-card p-5">
            <h2 className="text-base font-semibold text-foreground">通知设置</h2>
            <div className="mt-3 rounded-xl border border-border bg-[var(--color-bg-hover)] p-4">
              <div className="flex items-center justify-between gap-2">
                <div>
                  <div className="text-sm font-medium text-foreground">站内提醒（默认开启）</div>
                  <p className="mt-1 text-xs leading-5 text-foreground-muted">
                    信号触发后写入「信号记录」，导航栏「信号中心」出现未读角标。无需任何配置。
                  </p>
                </div>
                <span className="rounded-full border border-emerald-500/40 bg-emerald-500/10 px-2.5 py-1 text-[11px] text-emerald-600 dark:text-emerald-300">
                  已开启
                </span>
              </div>
            </div>

            <details className="mt-3 rounded-xl border border-border bg-[var(--color-bg-hover)] p-4">
              <summary className="cursor-pointer text-sm font-medium text-foreground-muted">
                Webhook 机器人推送（进阶）
              </summary>
              <div className="mt-4">
                <SignalWebhookPanel />
              </div>
            </details>
          </section>

          <p className="px-1 text-[11px] leading-5 text-foreground-dim">
            风险提示：信号由你配置的规则自动生成，盘中试算基于未完成 K 线、收盘前可能反转，仅供投研参考，不构成投资建议。
          </p>
        </>
      )}

      {modalState.open ? (
        <SubscriptionModal
          mode={modalState.mode}
          editing={modalState.editing}
          presetSymbol={modalState.presetSymbol}
          templates={templates}
          onClose={() => setModalState({ open: false, mode: 'create', editing: null, presetSymbol: '' })}
          onSaved={handleModalSaved}
        />
      ) : null}
    </div>
  )
}

function OverviewMetric({ label, value, accent = false }) {
  return (
    <div className="rounded-xl border border-border bg-[var(--color-bg-hover)] px-3 py-2.5 text-center">
      <div className={`text-xl font-bold tracking-tight ${accent ? 'text-primary' : 'text-foreground'}`}>{value}</div>
      <div className="mt-0.5 text-[11px] text-foreground-dim">{label}</div>
    </div>
  )
}

function SignalEventRow({ event }) {
  const state = barStateMeta(event.bar_state)
  const side = sideMeta(event.side)
  const suppressed = event.gate_decision === 'suppressed'
  return (
    <li className="rounded-xl border border-border bg-[var(--color-bg-hover)] px-4 py-3">
      <div className="flex flex-wrap items-center gap-2">
        <span className={`text-sm font-semibold ${side.tone === 'rise' ? 'text-negative' : side.tone === 'fall' ? 'text-positive' : 'text-foreground-muted'}`}>
          {side.label}
        </span>
        <span className="font-mono text-xs text-foreground-muted">{event.symbol}</span>
        {state.label ? (
          <span className={`rounded-full border px-2 py-0.5 text-[10px] ${
            state.tone === 'warning'
              ? 'border-amber-400/40 bg-amber-500/10 text-amber-600 dark:text-amber-300'
              : state.tone === 'info'
                ? 'border-blue-400/40 bg-blue-500/10 text-blue-600 dark:text-blue-300'
                : 'border-border text-foreground-dim'
          }`}
          >
            {state.label}
          </span>
        ) : null}
        {event.template_name ? (
          <span className="rounded-full border border-border px-2 py-0.5 text-[10px] text-foreground-dim">{event.template_name}</span>
        ) : null}
        <span className="ml-auto text-[11px] text-foreground-dim">{formatEventTime(event.event_time)}</span>
      </div>
      {event.message ? (
        <div className="mt-1.5 text-xs leading-5 text-foreground-muted">{event.message}</div>
      ) : null}
      {event.semantic_label ? (
        <div className="mt-1 text-[11px] text-foreground-dim">持仓语义：{event.semantic_label}</div>
      ) : null}
      {suppressed ? (
        <div className="mt-1 text-[11px] text-amber-600 dark:text-amber-300/90">
          已归档未推送{event.suppressed_message ? `：${event.suppressed_message}` : ''}
        </div>
      ) : null}
      {event.bar_state_note ? (
        <div className="mt-1 text-[11px] text-foreground-dim">{event.bar_state_note}</div>
      ) : null}
    </li>
  )
}

function formatEventTime(isoTime) {
  if (!isoTime) return '--'
  const date = new Date(isoTime)
  if (Number.isNaN(date.getTime())) return '--'
  const shifted = new Date(date.getTime() + 8 * 3600 * 1000)
  const mm = String(shifted.getUTCMonth() + 1).padStart(2, '0')
  const dd = String(shifted.getUTCDate()).padStart(2, '0')
  const hh = String(shifted.getUTCHours()).padStart(2, '0')
  const mi = String(shifted.getUTCMinutes()).padStart(2, '0')
  return `${mm}-${dd} ${hh}:${mi}`
}

// ── 新增 / 编辑订阅弹窗 ──

function SubscriptionModal({ mode, editing, presetSymbol, templates, onClose, onSaved }) {
  const isEdit = mode === 'edit' && editing

  const [symbolQuery, setSymbolQuery] = useState(isEdit ? editing.symbol : (presetSymbol || ''))
  const [selectedSymbol, setSelectedSymbol] = useState(isEdit ? editing.symbol : '')
  const [searchResults, setSearchResults] = useState([])
  const [searching, setSearching] = useState(false)
  const [templateKey, setTemplateKey] = useState(isEdit ? editing.template_key : '')
  const [paramDraft, setParamDraft] = useState(() => (isEdit ? { ...editing.params } : {}))
  const [strategyId, setStrategyId] = useState(isEdit ? editing.strategy_id : '')
  const [strategies, setStrategies] = useState([])
  const [evalInterval, setEvalInterval] = useState(isEdit ? editing.eval_interval_seconds : 900)
  const [showAdvanced, setShowAdvanced] = useState(false)
  const [riskDraft, setRiskDraft] = useState(() => ({
    position_aware_enabled: isEdit ? editing.position_aware_enabled : true,
    stop_loss_pct: isEdit ? editing.stop_loss_pct : 0,
    trailing_stop_pct: isEdit ? editing.trailing_stop_pct : 0,
  }))
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')
  const debounceRef = useRef(null)

  const templateGroups = useMemo(() => groupTemplatesByCategory(templates), [templates])
  const selectedTemplate = useMemo(
    () => templates.find((tpl) => tpl.key === templateKey) || null,
    [templates, templateKey],
  )

  // 选择模板后用模板默认值初始化参数草稿。
  useEffect(() => {
    if (!isEdit && selectedTemplate) {
      setParamDraft(buildTemplateParamDraft(selectedTemplate))
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [templateKey])

  // 策略模板需要加载 active 策略列表。
  useEffect(() => {
    if (!selectedTemplate?.needs_strategy) return
    requestJson('/api/strategies/active')
      .then((data) => setStrategies(Array.isArray(data?.items) ? data.items : []))
      .catch(() => setStrategies([]))
  }, [selectedTemplate])

  // 股票搜索（复用 /api/search）。
  useEffect(() => {
    if (isEdit) return undefined
    if (debounceRef.current) clearTimeout(debounceRef.current)
    const query = symbolQuery.trim()
    if (query.length < SEARCH_MIN_LEN || query === selectedSymbol) {
      setSearchResults([])
      return undefined
    }
    debounceRef.current = setTimeout(async () => {
      setSearching(true)
      try {
        const data = await requestJson(`/api/search?q=${encodeURIComponent(query)}&limit=${SEARCH_MAX_RESULTS}`)
        setSearchResults(Array.isArray(data?.results) ? data.results : [])
      } catch {
        setSearchResults([])
      } finally {
        setSearching(false)
      }
    }, SEARCH_DEBOUNCE_MS)
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current)
    }
  }, [symbolQuery, selectedSymbol, isEdit])

  const handleSubmit = async () => {
    setError('')
    if (!isEdit && !selectedSymbol) {
      setError('请先搜索并选择股票')
      return
    }
    if (!selectedTemplate) {
      setError('请选择信号模板')
      return
    }
    if (selectedTemplate.needs_strategy && !strategyId) {
      setError('请选择要绑定的策略')
      return
    }
    const paramError = validateTemplateParamDraft(selectedTemplate, paramDraft)
    if (paramError) {
      setError(paramError)
      return
    }

    const params = {}
    for (const field of selectedTemplate.param_schema || []) {
      if (paramDraft[field.key] !== undefined && paramDraft[field.key] !== '') {
        params[field.key] = Number(paramDraft[field.key])
      }
    }

    setSubmitting(true)
    try {
      if (isEdit) {
        const body = {
          params,
          eval_interval_seconds: evalInterval,
          position_aware_enabled: riskDraft.position_aware_enabled,
          stop_loss_pct: Number(riskDraft.stop_loss_pct) || 0,
          trailing_stop_pct: Number(riskDraft.trailing_stop_pct) || 0,
        }
        const data = await requestJson(`/api/signal/subscriptions/${encodeURIComponent(editing.id)}`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(body),
        })
        onSaved(data?.item, 'edit')
      } else {
        const payload = buildCreateSubscriptionPayload({
          templateKey: selectedTemplate.key,
          symbol: selectedSymbol,
          strategyId: selectedTemplate.needs_strategy ? strategyId : '',
          params,
          evalIntervalSeconds: evalInterval,
        })
        const data = await requestJson('/api/signal/subscriptions', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        })
        onSaved(data?.item, 'create')
      }
    } catch (err) {
      setError(err.message || (isEdit ? '保存信号失败' : '创建信号失败'))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="fixed inset-0 z-[70] flex items-end justify-center bg-black/50 sm:items-center sm:p-4">
      <div className="max-h-[92vh] w-full max-w-lg overflow-y-auto rounded-t-2xl border border-border bg-card p-5 sm:rounded-2xl">
        <div className="flex items-center justify-between">
          <h3 className="text-base font-semibold text-foreground">{isEdit ? '编辑信号' : '新增信号'}</h3>
          <button type="button" onClick={onClose} className="text-sm text-foreground-dim transition hover:text-foreground">
            关闭
          </button>
        </div>

        {error ? (
          <div className="mt-3 rounded-lg border border-negative/40 bg-negative/10 px-3 py-2 text-xs text-negative">{error}</div>
        ) : null}

        {/* 第一步：选股票 */}
        <div className="mt-4">
          <div className="text-xs font-medium text-foreground-muted">1. 选择股票</div>
          {isEdit ? (
            <div className="mt-2 rounded-lg border border-border bg-[var(--color-bg-hover)] px-3 py-2 text-sm text-foreground">
              {editing.symbol_name ? `${editing.symbol_name} · ` : ''}{editing.symbol}
            </div>
          ) : (
            <div className="relative mt-2">
              <input
                value={symbolQuery}
                onChange={(event) => {
                  setSymbolQuery(event.target.value)
                  setSelectedSymbol('')
                }}
                placeholder="输入代码或名称搜索，如 600519 / 茅台"
                className="w-full rounded-lg border border-border bg-[var(--color-bg-overlay)] px-3 py-2 text-sm text-foreground outline-none transition focus:border-primary"
              />
              {searchResults.length > 0 && !selectedSymbol ? (
                <ul className="absolute left-0 right-0 top-full z-10 mt-1 max-h-56 overflow-y-auto rounded-xl border border-border bg-card shadow-2xl">
                  {searchResults.map((item) => (
                    <li key={item.code}>
                      <button
                        type="button"
                        onClick={() => {
                          setSelectedSymbol(item.code)
                          setSymbolQuery(item.code)
                          setSearchResults([])
                        }}
                        className="flex w-full items-center justify-between px-3 py-2 text-left text-sm text-foreground-muted transition hover:bg-primary/10"
                      >
                        <span>
                          <span className="font-mono font-semibold text-primary">{item.code}</span>
                          <span className="ml-2 text-foreground-dim">{item.name}</span>
                        </span>
                      </button>
                    </li>
                  ))}
                </ul>
              ) : null}
              {!searching && symbolQuery.trim().length >= SEARCH_MIN_LEN && searchResults.length === 0 && !selectedSymbol ? (
                <div className="mt-1 text-[11px] text-foreground-dim">未找到匹配股票，换个关键词试试</div>
              ) : null}
            </div>
          )}
        </div>

        {/* 第二步：选模板 */}
        <div className="mt-4">
          <div className="text-xs font-medium text-foreground-muted">2. 选择信号模板</div>
          {isEdit ? (
            <div className="mt-2 rounded-lg border border-border bg-[var(--color-bg-hover)] px-3 py-2 text-sm text-foreground">
              {editing.template_name}
            </div>
          ) : (
            <div className="mt-2 space-y-3">
              {templateGroups.map((group) => (
                <div key={group.key}>
                  <div className="mb-1.5 text-[11px] text-foreground-dim">{group.label}</div>
                  <div className="grid gap-2 sm:grid-cols-2">
                    {group.items.map((tpl) => (
                      <button
                        key={tpl.key}
                        type="button"
                        onClick={() => setTemplateKey(tpl.key)}
                        className={
                          templateKey === tpl.key
                            ? 'rounded-lg border border-primary bg-primary/10 px-3 py-2 text-left transition'
                            : 'rounded-lg border border-border bg-[var(--color-bg-hover)] px-3 py-2 text-left transition hover:border-[var(--color-border-strong)]'
                        }
                      >
                        <div className="text-sm font-medium text-foreground">{tpl.name}</div>
                        <div className="mt-0.5 line-clamp-2 text-[11px] leading-4 text-foreground-dim">{tpl.description}</div>
                      </button>
                    ))}
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>

        {/* 策略选择（策略模板） */}
        {selectedTemplate?.needs_strategy ? (
          <div className="mt-4">
            <div className="text-xs font-medium text-foreground-muted">绑定策略</div>
            {strategies.length === 0 ? (
              <div className="mt-2 rounded-lg border border-dashed border-border px-3 py-2 text-xs text-foreground-dim">
                你还没有已激活的策略，请先到「选股 → 策略库」创建并激活。
              </div>
            ) : (
              <select
                value={strategyId}
                onChange={(event) => setStrategyId(event.target.value)}
                disabled={isEdit}
                className="mt-2 w-full rounded-lg border border-border bg-[var(--color-bg-overlay)] px-3 py-2 text-sm text-foreground outline-none transition focus:border-primary disabled:opacity-60"
              >
                <option value="">请选择策略</option>
                {strategies.map((item) => (
                  <option key={item.id} value={item.id}>{item.name}</option>
                ))}
              </select>
            )}
          </div>
        ) : null}

        {/* 第三步：参数（schema 驱动） */}
        {selectedTemplate && (selectedTemplate.param_schema || []).length > 0 ? (
          <div className="mt-4">
            <div className="text-xs font-medium text-foreground-muted">3. 参数设置</div>
            <div className="mt-2 grid gap-3 sm:grid-cols-2">
              {selectedTemplate.param_schema.map((field) => (
                <label key={field.key} className="block">
                  <span className="text-xs text-foreground-dim">
                    {field.label}{field.unit ? `（${field.unit}）` : ''}
                  </span>
                  <input
                    type="number"
                    value={paramDraft[field.key] ?? ''}
                    min={field.min}
                    max={field.max}
                    step={field.step || 'any'}
                    onChange={(event) => setParamDraft((prev) => ({ ...prev, [field.key]: event.target.value }))}
                    className="mt-1 block w-full rounded-lg border border-border bg-[var(--color-bg-overlay)] px-3 py-2 text-sm text-foreground outline-none transition focus:border-primary"
                  />
                </label>
              ))}
            </div>
          </div>
        ) : null}

        {/* 评估频率（价格提醒固定实时，不展示） */}
        {selectedTemplate && selectedTemplate.category !== 'price_alert' ? (
          <div className="mt-4">
            <div className="text-xs font-medium text-foreground-muted">盘中评估频率</div>
            <select
              value={evalInterval}
              onChange={(event) => setEvalInterval(Number(event.target.value))}
              className="mt-2 w-full rounded-lg border border-border bg-[var(--color-bg-overlay)] px-3 py-2 text-sm text-foreground outline-none transition focus:border-primary"
            >
              {EVAL_INTERVAL_OPTIONS.map((option) => (
                <option key={option.value} value={option.value}>{option.label}</option>
              ))}
            </select>
            <p className="mt-1 text-[11px] text-foreground-dim">收盘后还会自动做一次收盘确认评估。</p>
          </div>
        ) : null}

        {/* 高级：持仓风控 */}
        <div className="mt-4">
          <button
            type="button"
            onClick={() => setShowAdvanced((prev) => !prev)}
            className="text-xs text-foreground-dim transition hover:text-foreground"
          >
            {showAdvanced ? '▾ 收起高级设置' : '▸ 高级设置（持仓风控）'}
          </button>
          {showAdvanced ? (
            <div className="mt-2 space-y-3 rounded-lg border border-border bg-[var(--color-bg-hover)] p-3">
              <label className="flex items-center gap-2 text-xs text-foreground-muted">
                <input
                  type="checkbox"
                  checked={riskDraft.position_aware_enabled}
                  onChange={(event) => setRiskDraft((prev) => ({ ...prev, position_aware_enabled: event.target.checked }))}
                />
                持仓感知（结合我的持仓过滤无意义提示）
              </label>
              <div className="grid gap-3 sm:grid-cols-2">
                <label className="block">
                  <span className="text-xs text-foreground-dim">止损线（%，0=关闭）</span>
                  <input
                    type="number"
                    min={0}
                    max={100}
                    step="any"
                    value={riskDraft.stop_loss_pct}
                    onChange={(event) => setRiskDraft((prev) => ({ ...prev, stop_loss_pct: event.target.value }))}
                    className="mt-1 block w-full rounded-lg border border-border bg-[var(--color-bg-overlay)] px-3 py-2 text-sm text-foreground outline-none transition focus:border-primary"
                  />
                </label>
                <label className="block">
                  <span className="text-xs text-foreground-dim">移动止盈回撤（%，0=关闭）</span>
                  <input
                    type="number"
                    min={0}
                    max={100}
                    step="any"
                    value={riskDraft.trailing_stop_pct}
                    onChange={(event) => setRiskDraft((prev) => ({ ...prev, trailing_stop_pct: event.target.value }))}
                    className="mt-1 block w-full rounded-lg border border-border bg-[var(--color-bg-overlay)] px-3 py-2 text-sm text-foreground outline-none transition focus:border-primary"
                  />
                </label>
              </div>
            </div>
          ) : null}
        </div>

        <div className="mt-5 flex items-center justify-end gap-2">
          <button
            type="button"
            onClick={onClose}
            className="rounded-lg border border-border px-4 py-2 text-xs text-foreground-muted transition hover:border-[var(--color-border-strong)]"
          >
            取消
          </button>
          <button
            type="button"
            disabled={submitting}
            onClick={handleSubmit}
            className="rounded-lg bg-primary px-4 py-2 text-xs font-semibold text-black transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
          >
            {submitting ? '提交中...' : isEdit ? '保存修改' : '开启信号'}
          </button>
        </div>
      </div>
    </div>
  )
}
