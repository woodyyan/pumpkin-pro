import { useEffect, useMemo, useRef } from 'react'

import {
  buildPortfolioTrackingChart,
  buildPortfolioTrackingDetailHref,
  formatPortfolioTrackingCurrency,
  formatPortfolioTrackingDate,
  formatPortfolioTrackingNav,
  formatPortfolioTrackingPercent,
  formatPortfolioTrackingShares,
  getPortfolioTrackingPerformanceClass,
  getPortfolioTrackingStatusTone,
} from '../lib/portfolio-tracking'

function EmptyState({ title, description }) {
  return (
    <div className="rounded-2xl border border-dashed border-border bg-card px-5 py-8 text-center">
      <div className="text-sm font-medium text-foreground">{title}</div>
      <div className="mt-2 text-sm leading-6 text-foreground-muted">{description}</div>
    </div>
  )
}

function LoadingState() {
  return (
    <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
      {Array.from({ length: 4 }).map((_, index) => (
        <div key={index} className="h-36 animate-pulse rounded-2xl border border-border bg-card/70" />
      ))}
    </div>
  )
}

function StatusBadge({ status, text }) {
  return (
    <span className={`inline-flex items-center rounded-full border px-3 py-1 text-xs font-medium ${getPortfolioTrackingStatusTone(status)}`}>
      {text || '等待数据更新'}
    </span>
  )
}

function CompactInfoPanel({ selectedItem, metrics }) {
  const metricRows = [
    { label: '最新净值', value: formatPortfolioTrackingNav(selectedItem?.nav) },
    { label: '累计收益', value: formatPortfolioTrackingPercent(selectedItem?.total_return), valueClass: getPortfolioTrackingPerformanceClass(selectedItem?.total_return) },
    { label: '日收益', value: formatPortfolioTrackingPercent(selectedItem?.daily_return), valueClass: getPortfolioTrackingPerformanceClass(selectedItem?.daily_return) },
    { label: '总资产', value: formatPortfolioTrackingCurrency(selectedItem?.total_assets, selectedItem?.exchange) },
    { label: '最大回撤', value: formatPortfolioTrackingPercent(metrics?.max_drawdown ?? selectedItem?.max_drawdown) },
    { label: '年化波动率', value: formatPortfolioTrackingPercent(metrics?.volatility ?? selectedItem?.volatility) },
    { label: '胜率', value: formatPortfolioTrackingPercent(metrics?.win_rate ?? selectedItem?.win_rate) },
    { label: '换手率', value: formatPortfolioTrackingPercent(metrics?.turnover_rate ?? selectedItem?.turnover_rate) },
  ]
  return (
    <div className="rounded-2xl border border-border bg-card px-4 py-4">
      <div className="text-sm font-medium text-foreground">绩效指标</div>
      <div className="mt-3 grid grid-cols-2 gap-x-4 gap-y-2 text-sm">
        {metricRows.map((row) => (
          <div key={row.label} className="flex items-center justify-between gap-2">
            <span className="text-foreground-muted">{row.label}</span>
            <span className={`font-semibold ${row.valueClass || 'text-foreground'}`}>{row.value}</span>
          </div>
        ))}
      </div>
    </div>
  )
}

function buildPortfolioTrackingSelectionLabel(item) {
  if (!item) return '模拟组合'
  const marketLabel = String(item?.exchange || '').toUpperCase() === 'HKEX' ? '港股' : 'A股'
  return `${marketLabel} · ${item?.name || '模拟组合'}${item?.portfolio_variant ? ` · ${item.portfolio_variant}` : ''}`
}

function PortfolioChart({ series, portfolioLabel }) {
  const chart = useMemo(() => buildPortfolioTrackingChart(series, 760, 220, 24), [series])

  if (!chart.path) {
    return <EmptyState title="暂无净值曲线" description="新的模拟组合还没有形成完整的收盘估值序列。" />
  }

  const latest = chart.points[chart.points.length - 1]
  return (
    <div className="rounded-2xl border border-border bg-card px-4 py-4">
      <div className="flex items-start justify-between gap-4">
        <div>
          <div className="text-sm font-medium text-foreground">净值曲线</div>
          <div className="mt-1 text-xs text-foreground-muted">当前查看：{portfolioLabel || '模拟组合'} · 按收盘总资产 / 初始资金计算，口径不含手续费。</div>
        </div>
        <div className="text-right">
          <div className="text-xs text-foreground-muted">最新净值</div>
          <div className="mt-1 text-lg font-semibold text-foreground">{formatPortfolioTrackingNav(latest?.nav)}</div>
        </div>
      </div>
      <div className="mt-4 overflow-x-auto text-negative">
        <svg viewBox="0 0 760 220" className="h-[220px] min-w-[640px] w-full">
          <line x1="24" x2="736" y1={chart.baselineY} y2={chart.baselineY} stroke="rgba(148,163,184,0.35)" strokeDasharray="6 6" />
          <path d={chart.path} fill="none" stroke="currentColor" strokeWidth="3" strokeLinecap="round" />
          {chart.points.map((point, index) => (
            <g key={point.trade_date}>
              <circle cx={point.x} cy={point.y} r={index === chart.points.length - 1 ? 4 : 2.5} fill="currentColor" />
            </g>
          ))}
          {chart.points.map((point, index) => {
            if (index !== 0 && index !== chart.points.length - 1) return null
            return (
              <text key={`${point.trade_date}-label`} x={point.x} y="210" textAnchor={index === 0 ? 'start' : 'end'} fill="var(--color-text-tertiary)" fontSize="11">
                {point.trade_date}
              </text>
            )
          })}
        </svg>
      </div>
    </div>
  )
}

function PortfolioTab({ item, active, onSelect }) {
  const isHK = String(item?.exchange || '').toUpperCase() === 'HKEX'
  const dotClass = isHK ? 'bg-sky-500' : 'bg-negative'
  return (
    <button
      type="button"
      aria-pressed={active}
      onClick={() => onSelect?.(item?.portfolio_id)}
      className={`flex-1 shrink-0 rounded-xl border px-4 py-3 text-left transition cursor-pointer lg:min-w-0 min-w-[140px] ${
        active
          ? 'border-negative/60 bg-negative/5 ring-2 ring-negative/15'
          : 'border-border bg-card hover:border-negative/30 hover:shadow-sm'
      }`}
    >
      <div className="flex items-center gap-1.5">
        <span className={`inline-block h-2 w-2 shrink-0 rounded-full ${dotClass}`} />
        <span className="truncate text-sm font-semibold text-foreground">{item?.name || '模拟组合'}</span>
      </div>
      <div className={`mt-1 text-base font-bold ${getPortfolioTrackingPerformanceClass(item?.total_return)}`}>
        {formatPortfolioTrackingPercent(item?.total_return)}
      </div>
      <div className="mt-0.5 text-xs text-foreground-muted">
        {formatPortfolioTrackingCurrency(item?.total_assets, item?.exchange)}
      </div>
    </button>
  )
}

function PortfolioTabBar({ items, selectedPortfolioId, onSelectPortfolio }) {
  return (
    <div className="flex gap-2 overflow-x-auto pb-1">
      {items.map((item) => (
        <PortfolioTab
          key={item.portfolio_id}
          item={item}
          active={item.portfolio_id === selectedPortfolioId}
          onSelect={onSelectPortfolio}
        />
      ))}
    </div>
  )
}

function PositionTable({ item, loading, portfolioLabel }) {
  const positions = item?.current_positions || []
  if (loading) {
    return <div className="h-56 animate-pulse rounded-2xl border border-border bg-card/70" />
  }
  if (!positions.length) {
    return <EmptyState title="暂无持仓明细" description="当前组合还没有形成可估值的 4 只理论持仓。" />
  }
  return (
    <div className="rounded-2xl border border-border bg-card px-4 py-4">
      <div className="flex items-center justify-between gap-3">
        <div>
          <div className="text-sm font-medium text-foreground">当前持仓</div>
          <div className="mt-1 flex items-start gap-1.5 rounded-lg bg-amber-50 px-2.5 py-1.5 text-xs text-amber-700">
            <span className="mt-0.5 shrink-0 font-bold">ⓘ</span>
            <span>理论股数含小数以保证 25% 等权。本组合用于验证策略逻辑而非交易系统，100万资金下取整误差对收益可忽略。</span>
          </div>
        </div>
        <div className="text-xs text-foreground-muted">估值日：{formatPortfolioTrackingDate(item?.latest_trade_date)}</div>
      </div>
      <div className="mt-4 hidden overflow-x-auto lg:block">
        <table className="min-w-full text-left text-sm">
          <thead className="text-xs text-foreground-muted">
            <tr>
              <th className="pb-3 font-medium">股票</th>
              <th className="pb-3 font-medium">权重</th>
              <th className="pb-3 font-medium">理论股数</th>
              <th className="pb-3 font-medium">建仓价</th>
              <th className="pb-3 font-medium">收盘价</th>
              <th className="pb-3 font-medium">市值</th>
              <th className="pb-3 font-medium">盈亏</th>
            </tr>
          </thead>
          <tbody>
            {positions.map((position) => {
              const href = buildPortfolioTrackingDetailHref(position.stock_code, position.exchange)
              return (
                <tr key={`${position.exchange}-${position.stock_code}`} className="border-t border-border/80">
                  <td className="py-3">
                    {href ? (
                      <a href={href} target="_blank" rel="noreferrer" className="font-medium text-foreground hover:text-negative">
                        {position.stock_name}（{position.stock_code}）
                      </a>
                    ) : (
                      <span className="font-medium text-foreground">{position.stock_name}（{position.stock_code}）</span>
                    )}
                  </td>
                  <td className="py-3 text-foreground">{formatPortfolioTrackingPercent(position.weight, 0)}</td>
                  <td className="py-3 text-foreground">{formatPortfolioTrackingShares(position.shares)}</td>
                  <td className="py-3 text-foreground">{formatPortfolioTrackingCurrency(position.buy_price, position.exchange)}</td>
                  <td className="py-3 text-foreground">{formatPortfolioTrackingCurrency(position.close_price, position.exchange)}</td>
                  <td className="py-3 text-foreground">{formatPortfolioTrackingCurrency(position.market_value, position.exchange)}</td>
                  <td className={`py-3 ${getPortfolioTrackingPerformanceClass(position.profit_rate)}`}>
                    {formatPortfolioTrackingCurrency(position.profit, position.exchange)} / {formatPortfolioTrackingPercent(position.profit_rate)}
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
      <div className="mt-4 grid gap-3 lg:hidden">
        {positions.map((position) => {
          const href = buildPortfolioTrackingDetailHref(position.stock_code, position.exchange)
          return (
            <div key={`${position.exchange}-${position.stock_code}`} className="rounded-2xl border border-border bg-background px-4 py-4">
              <div className="flex items-start justify-between gap-3">
                <div>
                  {href ? (
                    <a href={href} target="_blank" rel="noreferrer" className="text-sm font-medium text-foreground hover:text-negative">
                      {position.stock_name}（{position.stock_code}）
                    </a>
                  ) : (
                    <div className="text-sm font-medium text-foreground">{position.stock_name}（{position.stock_code}）</div>
                  )}
                  <div className="mt-1 text-xs text-foreground-muted">权重 {formatPortfolioTrackingPercent(position.weight, 0)}</div>
                </div>
                <div className={`text-sm font-medium ${getPortfolioTrackingPerformanceClass(position.profit_rate)}`}>
                  {formatPortfolioTrackingPercent(position.profit_rate)}
                </div>
              </div>
              <div className="mt-3 grid grid-cols-2 gap-3 text-xs text-foreground-muted">
                <div>理论股数：<span className="text-foreground">{formatPortfolioTrackingShares(position.shares)}</span></div>
                <div>市值：<span className="text-foreground">{formatPortfolioTrackingCurrency(position.market_value, position.exchange)}</span></div>
                <div>建仓价：<span className="text-foreground">{formatPortfolioTrackingCurrency(position.buy_price, position.exchange)}</span></div>
                <div>收盘价：<span className="text-foreground">{formatPortfolioTrackingCurrency(position.close_price, position.exchange)}</span></div>
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}

function TradeTable({ trades, exchange, loading, portfolioLabel }) {
  if (loading) {
    return <div className="h-48 animate-pulse rounded-2xl border border-border bg-card/70" />
  }
  if (!Array.isArray(trades) || !trades.length) {
    return <EmptyState title="暂无调仓记录" description="等下一交易日开盘形成实际理论成交后，这里会展示 BUY / SELL / HOLD。" />
  }
  return (
    <div className="rounded-2xl border border-border bg-card px-4 py-4">
      <div className="text-sm font-medium text-foreground">最近调仓记录</div>
      <div className="mt-1 text-xs text-foreground-muted">当前查看：{portfolioLabel || '模拟组合'} · 这里只记录实际理论成交，不写未来计划调仓。</div>
      <div className="mt-4 overflow-x-auto">
        <table className="min-w-full text-left text-sm">
          <thead className="text-xs text-foreground-muted">
            <tr>
              <th className="pb-3 font-medium">日期</th>
              <th className="pb-3 font-medium">股票</th>
              <th className="pb-3 font-medium">动作</th>
              <th className="pb-3 font-medium">权重变化</th>
              <th className="pb-3 font-medium">成交价</th>
              <th className="pb-3 font-medium">目标金额</th>
              <th className="pb-3 font-medium">股数变化</th>
            </tr>
          </thead>
          <tbody>
            {trades.map((trade) => {
              const actionClass = trade.action === 'BUY' ? 'text-negative' : trade.action === 'SELL' ? 'text-positive' : 'text-foreground'
              return (
                <tr key={`${trade.trade_date}-${trade.exchange}-${trade.stock_code}-${trade.action}`} className="border-t border-border/80">
                  <td className="py-3 text-foreground">{formatPortfolioTrackingDate(trade.trade_date)}</td>
                  <td className="py-3 text-foreground">{trade.stock_name}（{trade.stock_code}）</td>
                  <td className={`py-3 font-medium ${actionClass}`}>{trade.action}</td>
                  <td className="py-3 text-foreground">{formatPortfolioTrackingPercent(trade.old_weight, 0)} → {formatPortfolioTrackingPercent(trade.new_weight, 0)}</td>
                  <td className="py-3 text-foreground">{formatPortfolioTrackingCurrency(trade.trade_price, exchange)}</td>
                  <td className="py-3 text-foreground">{formatPortfolioTrackingCurrency(trade.target_value, exchange)}</td>
                  <td className="py-3 text-foreground">{formatPortfolioTrackingShares(trade.old_shares)} → {formatPortfolioTrackingShares(trade.new_shares)}</td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
    </div>
  )
}

export default function PortfolioTrackingDashboard({
  overview,
  overviewLoading,
  detailLoading,
  selectedPortfolioId,
  onSelectPortfolio,
  daily,
  positions,
  trades,
  metrics,
}) {
  const items = Array.isArray(overview?.items) ? overview.items : []
  const selectedItem = items.find((item) => item.portfolio_id === selectedPortfolioId) || items[0] || null
  const detailSectionRef = useRef(null)
  const hasMountedSelectionRef = useRef(false)
  const selectedPortfolioLabel = buildPortfolioTrackingSelectionLabel(selectedItem)

  useEffect(() => {
    if (!selectedItem?.portfolio_id) return
    if (!hasMountedSelectionRef.current) {
      hasMountedSelectionRef.current = true
      return
    }
    detailSectionRef.current?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  }, [selectedItem?.portfolio_id])

  return (
    <div className="space-y-6">
      <section className="rounded-2xl border border-border bg-card px-5 py-5">
        <div className="max-w-4xl">
          <div className="flex flex-wrap items-center gap-3">
            <h1 className="text-xl font-semibold tracking-tight text-foreground">模拟组合跟踪</h1>
            <StatusBadge status={selectedItem?.status} text={selectedItem?.status_text} />
          </div>
          <p className="mt-2 text-sm leading-6 text-foreground-muted">
            组合按照「T日收盘生成信号 → 下一交易日开盘理论建仓 → 当日收盘估值」运行。
          </p>
          <div className="mt-3 flex flex-wrap gap-3 text-xs text-foreground-muted">
            <span>初始资金：¥1,000,000</span>
            <span>持仓规则：固定 4 只，每只 25%</span>
            <span>最新估值日：{formatPortfolioTrackingDate(overview?.as_of_trade_date)}</span>
          </div>
        </div>
      </section>

      {overviewLoading ? <LoadingState /> : null}
      {!overviewLoading && !items.length ? (
        <EmptyState title="模拟组合已切换到新口径" description="当前还没有完成第一笔 T+1 收盘估值。系统会从新的起点开始累计净值，不再展示旧 JSON 结果表。" />
      ) : null}

      {!overviewLoading && items.length ? (
        <PortfolioTabBar items={items} selectedPortfolioId={selectedItem?.portfolio_id} onSelectPortfolio={onSelectPortfolio} />
      ) : null}

      {selectedItem ? (
        <section ref={detailSectionRef} className="space-y-4">
          <div className="rounded-2xl border border-negative/20 bg-negative/5 px-5 py-4">
            <div className="flex flex-wrap items-start justify-between gap-4">
              <div>
                <div className="text-xs font-medium tracking-wide text-negative">当前查看</div>
                <div className="mt-1 text-lg font-semibold text-foreground">{selectedPortfolioLabel}</div>
                <div className="mt-1 text-sm leading-6 text-foreground-muted">下方净值曲线、绩效指标、持仓明细和调仓记录会随上方选中的组合切换。</div>
              </div>
              <div className="grid grid-cols-2 gap-2 text-xs">
                <div className="rounded-lg border border-border bg-background px-3 py-2">
                  <div className="text-foreground-muted">最新信号日</div>
                  <div className="mt-0.5 font-medium text-foreground">{formatPortfolioTrackingDate(selectedItem.latest_signal_date)}</div>
                </div>
                <div className="rounded-lg border border-border bg-background px-3 py-2">
                  <div className="text-foreground-muted">待执行信号</div>
                  <div className="mt-0.5 font-medium text-foreground">{formatPortfolioTrackingDate(selectedItem.pending_signal_date)}</div>
                </div>
                <div className="rounded-lg border border-border bg-background px-3 py-2">
                  <div className="text-foreground-muted">下一次开盘</div>
                  <div className="mt-0.5 font-medium text-foreground">{formatPortfolioTrackingDate(selectedItem.next_entry_trade_date)}</div>
                </div>
                <div className="rounded-lg border border-border bg-background px-3 py-2">
                  <div className="text-foreground-muted">最新估值日</div>
                  <div className="mt-0.5 font-medium text-foreground">{formatPortfolioTrackingDate(selectedItem.latest_trade_date)}</div>
                </div>
              </div>
            </div>
            {detailLoading ? <div className="mt-3 text-xs text-negative">详情更新中…</div> : null}
          </div>

          <div className="grid gap-4 xl:grid-cols-[minmax(0,2fr)_minmax(320px,1fr)]">
            <PortfolioChart series={daily?.items || []} portfolioLabel={selectedPortfolioLabel} />
            <CompactInfoPanel selectedItem={selectedItem} metrics={metrics} />
          </div>

          <PositionTable item={{ ...selectedItem, current_positions: positions?.items || selectedItem.current_positions || [] }} loading={detailLoading} portfolioLabel={selectedPortfolioLabel} />
          <TradeTable trades={trades?.items || selectedItem.latest_trades || []} exchange={selectedItem.exchange} loading={detailLoading} portfolioLabel={selectedPortfolioLabel} />
        </section>
      ) : null}
    </div>
  )
}
