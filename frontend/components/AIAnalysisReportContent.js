import InfoTip from './InfoTip'

const SIGNAL_MAP = {
  buy: { label: '看多', arrow: '↑', hint: '偏多配置', color: 'text-red-300', bg: 'bg-red-500/12', border: 'border-red-400/40', dot: '🔴' },
  sell: { label: '看空', arrow: '↓', hint: '注意风险', color: 'text-emerald-300', bg: 'bg-emerald-500/12', border: 'border-emerald-400/40', dot: '🟢' },
  hold: { label: '观望', arrow: '→', hint: '持仓不变', color: 'text-amber-300', bg: 'bg-amber-500/12', border: 'border-amber-400/40', dot: '🟡' },
}

function formatDateTime(value) {
  if (!value) return '--'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString('zh-CN', { hour12: false })
}

function buildCompletenessLabels(meta) {
  const dc = meta?.data_completeness || {}
  return {
    market: dc.market === 'complete' ? '实时' : '缺失',
    technical: dc.technical === 'complete' ? '可用' : '部分缺失',
    fundamentals: dc.fundamentals === 'complete' ? '昨日收盘' : '不可用',
    market_overview: dc.market_overview === 'complete' ? '实时' : '不可用',
    portfolio: dc.portfolio || '',
  }
}

function MetricMini({ label, value, accent = 'normal', emphasis = false, featured = false, marketAccent = false, tooltip = '' }) {
  const risingColor = marketAccent ? 'text-rose-300' : 'text-emerald-300'
  const fallingColor = marketAccent ? 'text-emerald-300' : 'text-rose-300'
  const color = accent === 'up' ? risingColor : accent === 'down' ? fallingColor : 'text-white'
  const emphasisTone = accent === 'up' ? 'border-emerald-400/45 bg-emerald-500/10 ring-1 ring-emerald-300/20' : accent === 'down' ? 'border-rose-400/45 bg-rose-500/10 ring-1 ring-rose-300/20' : 'border-primary/45 bg-primary/10 ring-1 ring-primary/25'
  const featuredTone = accent === 'up' ? 'border-rose-400/50 bg-rose-500/12 ring-1 ring-rose-300/25 shadow-[0_10px_30px_rgba(251,113,133,0.18)]' : accent === 'down' ? 'border-emerald-400/50 bg-emerald-500/12 ring-1 ring-emerald-300/25 shadow-[0_10px_30px_rgba(52,211,153,0.18)]' : 'border-primary/55 bg-primary/12 ring-1 ring-primary/30 shadow-[0_10px_30px_rgba(76,106,255,0.16)]'
  const containerTone = featured ? (marketAccent ? featuredTone : 'border-primary/55 bg-primary/12 ring-1 ring-primary/30 shadow-[0_10px_30px_rgba(76,106,255,0.16)]') : emphasis ? emphasisTone : 'border-border bg-black/20'
  const featuredLabelColor = marketAccent ? (accent === 'up' ? 'text-rose-200/90' : accent === 'down' ? 'text-emerald-200/90' : 'text-primary/85') : 'text-primary/85'

  return (
    <div className={`relative rounded-xl border px-3 py-2 ${featured ? 'px-4 py-3' : ''} ${containerTone}`}>
      <div className={`flex items-center gap-1 text-xs ${featured ? featuredLabelColor : 'text-white/50'}`}>
        <span>{label}</span>
        {tooltip ? (
          <InfoTip
            text={tooltip}
            placement="top"
            widthClassName="w-56"
            iconClassName="h-3 w-3 border-0 p-0 text-white/40 hover:text-white/70 focus:ring-0"
            panelClassName="rounded-xl bg-[#1a1d25]/95 px-3 py-2.5 text-[11px] leading-relaxed text-white/80"
          />
        ) : null}
      </div>
      <div className={`mt-1 font-semibold ${color} ${featured ? 'text-2xl leading-none tracking-tight' : 'text-sm'}`}>{value}</div>
    </div>
  )
}

export default function AIAnalysisReportContent({
  result,
  className = '',
  logicExpanded = true,
  onToggleLogic,
  allowLogicToggle = true,
  actionSlot = null,
  hidePositionHint = false,
  showAnalysisTime = true,
}) {
  const analysis = result?.analysis
  if (!analysis) return null

  const sig = SIGNAL_MAP[analysis.signal] || SIGNAL_MAP.hold
  const confidencePct = Math.min(100, Math.max(0, analysis.confidence_score || 0))
  const confidenceLabel = analysis.confidence_level || 'medium'
  const completenessLabels = buildCompletenessLabels(result?.meta)
  const ts = analysis.trading_suggestions || {}
  const entryZone = ts.entry_zone || {}
  const stopLoss = ts.stop_loss || {}
  const takeProfit = ts.take_profit || {}
  const logicLines = String(analysis.logic_summary || '')
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean)

  return (
    <section className={`rounded-2xl border ${sig.border} ${sig.bg} p-5 ${className}`}>
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div className="flex items-center gap-3">
          <span className="text-xl">{sig.dot}</span>
          <div>
            <div className={`text-lg font-bold ${sig.color}`}>{sig.label} <span className="text-base">{sig.arrow}</span></div>
            <div className="mt-0.5 text-[11px] text-white/40">{sig.hint}</div>
            <div className="mt-1.5 flex items-center gap-2">
              <span className="text-xs text-white/50">置信度</span>
              <div className="h-2 w-32 overflow-hidden rounded-full bg-white/10">
                <div
                  className={`h-full rounded-full transition-all ${confidencePct >= 70 ? 'bg-red-400' : confidencePct >= 40 ? 'bg-amber-400' : 'bg-gray-500'}`}
                  style={{ width: `${confidencePct}%` }}
                />
              </div>
              <span className={`text-xs font-medium ${confidencePct >= 70 ? 'text-red-300' : confidencePct >= 40 ? 'text-amber-300' : 'text-gray-400'}`}>{confidencePct}%（{confidenceLabel}）</span>
            </div>
          </div>
        </div>
        {actionSlot ? <div className="flex items-center gap-2">{actionSlot}</div> : null}
      </div>

      <div className="mt-3 flex flex-wrap gap-x-4 gap-y-1 text-[11px] text-white/35">
        <span>数据时效：</span>
        <span>行情 {completenessLabels.market}</span>
        <span>· 技术 {completenessLabels.technical}</span>
        <span>· 基础面 {completenessLabels.fundamentals}</span>
        <span>· 大盘 {completenessLabels.market_overview}</span>
      </div>

      {analysis.layer_scores && Object.keys(analysis.layer_scores).length > 0 && (
        <div className="mt-4 rounded-xl border border-white/8 bg-black/20 px-4 py-3.5">
          <div className="mb-3 flex items-center gap-2">
            <span className="text-xs font-semibold text-white/70">📊 卧龙模型评分</span>
            {analysis.market_state && (
              <span className={`rounded-full px-2.5 py-0.5 text-[11px] font-medium ${analysis.market_state === 'trend' ? 'bg-red-500/15 text-red-300' : analysis.market_state === 'speculative' ? 'bg-purple-500/15 text-purple-300' : analysis.market_state === 'bubble' ? 'bg-orange-500/15 text-orange-300' : analysis.market_state === 'decline' ? 'bg-emerald-500/15 text-emerald-300' : 'bg-sky-500/15 text-sky-300'}`}>
                🏷️ {analysis.market_state_label || analysis.market_state}
              </span>
            )}
          </div>
          {(['narrative', 'liquidity', 'expectation', 'fundamental']).map((key) => {
            const ls = analysis.layer_scores[key]
            if (!ls) return null
            const layerMeta = {
              narrative: { label: '叙事层', icon: '📖', color: '#a78bfa', weight: '25%' },
              liquidity: { label: '资金层', icon: '💧', color: '#38bdf8', weight: '25%' },
              expectation: { label: '预期层', icon: '🎯', color: '#f472b6', weight: '30%' },
              fundamental: { label: '基本面', icon: '📈', color: '#34d399', weight: '20%' },
            }
            const meta = layerMeta[key]
            const barPct = Math.max(0, Math.min(100, ((ls.score + 2) / 4) * 100))
            const dirLabel = { bullish: '看多', neutral: '中性', bearish: '看空' }[ls.direction] || '中性'
            const dirColor = ls.direction === 'bullish' ? '#ef4444' : ls.direction === 'bearish' ? '#22c55e' : '#9ca3af'
            return (
              <div key={key} className="mt-2 first:mt-0">
                <div className="mb-1 flex items-center justify-between">
                  <div className="flex items-center gap-1.5">
                    <span className="text-[13px]">{meta.icon}</span>
                    <span className="text-[12px] font-medium text-white/80">{meta.label}</span>
                    <span className="text-[10px] text-white/30">({meta.weight})</span>
                  </div>
                  <div className="flex items-center gap-2">
                    <span className="text-[11px] font-semibold" style={{ color: dirColor }}>{dirLabel}</span>
                    <span className="text-[11px] font-mono text-white/50">{ls.score > 0 ? '+' : ''}{ls.score}</span>
                    <span className="text-[10px] text-white/30">置信度 {(ls.confidence * 100).toFixed(0)}%</span>
                  </div>
                </div>
                <div className="h-1.5 w-full overflow-hidden rounded-full bg-white/8">
                  <div className="h-full rounded-full transition-all" style={{ width: `${barPct}%`, backgroundColor: meta.color }} />
                </div>
                {ls.reason ? <p className="mt-1 text-[11px] leading-relaxed text-white/40">{ls.reason}</p> : null}
              </div>
            )
          })}
          {analysis.total_score != null && (
            <div className="mt-3 flex items-center justify-between border-t border-white/8 pt-3">
              <span className="text-[11px] text-white/40">加权综合评分</span>
              <span className={`text-sm font-bold font-mono ${analysis.total_score >= 0.5 ? 'text-red-400' : analysis.total_score <= -0.5 ? 'text-emerald-400' : 'text-amber-400'}`}>
                {analysis.total_score > 0 ? '+' : ''}{analysis.total_score.toFixed(2)}
              </span>
            </div>
          )}
        </div>
      )}

      <div className="mt-4 overflow-hidden rounded-xl border border-white/8 bg-black/20">
        {allowLogicToggle ? (
          <button
            type="button"
            onClick={onToggleLogic}
            className="flex w-full items-center justify-between px-4 py-2.5 text-left"
          >
            <span className="text-xs font-medium text-white/70">▾ 分析逻辑{!logicExpanded && '（点击展开）'}</span>
            <span className="text-[11px] text-white/35">{logicExpanded ? '收起' : '展开'}</span>
          </button>
        ) : (
          <div className="px-4 py-2.5 text-xs font-medium text-white/70">▾ 分析逻辑</div>
        )}
        {logicExpanded ? (
          <div className="px-4 pb-4">
            {logicLines.map((line, index) => (
              <p key={index} className="mt-2 text-[13px] leading-relaxed text-white/75 first:mt-0">• {line.replace(/^•\s*/, '')}</p>
            ))}
          </div>
        ) : null}
      </div>

      {Array.isArray(analysis.risk_warnings) && analysis.risk_warnings.length > 0 && (
        <div className="mt-4 rounded-xl border border-rose-400/25 bg-rose-500/8 px-4 py-3">
          <div className="mb-2 text-xs font-semibold text-rose-200/90">⚠️ 风险提示</div>
          {analysis.risk_warnings.map((warning, index) => (
            <p key={index} className="mt-1.5 text-[12px] leading-relaxed text-rose-200/70 first:mt-0">⚠️ {warning}</p>
          ))}
        </div>
      )}

      {ts.action_suggestion && (
        <div className="mt-4 rounded-xl border border-sky-400/20 bg-sky-500/5 px-4 py-3">
          <div className="mb-2 text-xs font-semibold text-sky-200/90">📋 交易建议</div>
          <p className="text-[13px] leading-relaxed text-white/80">{ts.action_suggestion}</p>
          <div className="mt-3 grid grid-cols-2 gap-x-6 gap-y-2 md:grid-cols-4">
            <MetricMini label="建议买价" value={`${entryZone.low ?? '--'} ~ ${entryZone.high ?? '--'}`} emphasis tooltip="建议的买入价格区间" />
            <MetricMini label="止损位" value={`${stopLoss.price || '--'}${stopLoss.pct != null ? `(${stopLoss.pct}%)` : ''}`} accent="down" tooltip="跌破此价位应考虑止损" />
            <MetricMini label="目标位" value={`${takeProfit.price || '--'}${takeProfit.pct != null ? `(+${takeProfit.pct}%)` : ''}`} accent="up" tooltip="预期盈利目标价位" />
            <MetricMini label="仓位建议" value={`${ts.position_size_pct || '--'}`} tooltip="占总资金的比例建议" />
          </div>
          {ts.time_horizon ? <div className="mt-2 text-[11px] text-white/45">投资周期：{ts.time_horizon}</div> : null}
        </div>
      )}

      {analysis.action_trigger && (analysis.action_trigger.buy_trigger || analysis.action_trigger.sell_trigger) && (
        <div className="mt-4 rounded-xl border border-amber-400/20 bg-amber-500/5 px-4 py-3">
          <div className="mb-2 text-xs font-semibold text-amber-200/90">🎯 执行触发条件</div>
          {analysis.action_trigger.buy_trigger ? (
            <div className="mt-1.5 flex items-start gap-2 first:mt-0">
              <span className="mt-0.5 text-xs">🟢</span>
              <div>
                <span className="text-[11px] font-medium text-white/50">买入触发</span>
                <p className="mt-0.5 text-[13px] leading-relaxed text-red-300/80">{analysis.action_trigger.buy_trigger}</p>
              </div>
            </div>
          ) : null}
          {analysis.action_trigger.sell_trigger ? (
            <div className="mt-2.5 flex items-start gap-2 first:mt-0">
              <span className="mt-0.5 text-xs">🔴</span>
              <div>
                <span className="text-[11px] font-medium text-white/50">卖出触发</span>
                <p className="mt-0.5 text-[13px] leading-relaxed text-emerald-300/80">{analysis.action_trigger.sell_trigger}</p>
              </div>
            </div>
          ) : null}
        </div>
      )}

      {Array.isArray(analysis.key_catalysts) && analysis.key_catalysts.length > 0 && (
        <div className="mt-4 rounded-xl border border-sky-400/15 bg-sky-500/[0.04] px-4 py-3">
          <div className="mb-2 text-xs font-semibold text-sky-200/90">✨ 潜在催化因素</div>
          {analysis.key_catalysts.map((item, index) => (
            <p key={index} className="mt-1.5 text-[12px] leading-relaxed text-sky-200/65 first:mt-0">💡 {item}</p>
          ))}
        </div>
      )}

      {!hidePositionHint && completenessLabels.portfolio === 'has_position' ? (
        <div className="mt-3 rounded-lg border border-emerald-400/15 bg-emerald-500/5 px-3.5 py-2.5 text-[12px] text-emerald-200/80">
          💡 你当前持有该股票，AI 已结合持仓盈亏给出针对性建议。
        </div>
      ) : null}

      <div className="mt-4 rounded-lg bg-black/20 px-3.5 py-2.5 text-center text-[11px] text-white/30">
        ⚠️ AI 分析仅供参考，不构成任何投资建议。市场有风险，投资需谨慎。
      </div>

      {showAnalysisTime && analysis.data_timestamp ? (
        <div className="mt-2 text-center text-[10px] text-white/20">分析时间：{formatDateTime(analysis.data_timestamp)}</div>
      ) : null}
    </section>
  )
}
