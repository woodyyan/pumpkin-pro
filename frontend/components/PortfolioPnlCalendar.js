import InfoTip, { LabelWithInfo } from './InfoTip'
import {
  formatCalendarCellPnlAmount,
  formatCalendarPnlAmount,
  formatCalendarPnlRate,
  getCalendarPnlColor,
} from '../lib/portfolio-pnl-calendar.js'

const WEEKDAY_LABELS = ['一', '二', '三', '四', '五', '六', '日']
const SCOPE_LABELS = {
  ASHARE: 'A股',
  HKEX: '港股',
}

const METRIC_OPTIONS = [
  { value: 'amount', label: '收益' },
  { value: 'rate', label: '收益率' },
]

const CALENDAR_TOOLTIP = '每日收益 = 当日持仓行情收益 + 当日卖出已实现收益；每日收益率 = 每日收益 / 日初持仓基数。日初持仓基数采用近似口径，分母不可用时显示 --。'

export function buildCalendarGrid(year, month, days = []) {
  const dayMap = new Map((Array.isArray(days) ? days : []).map((day) => [Number(day.day), day]))
  const firstDate = new Date(year, month - 1, 1)
  const dayCount = new Date(year, month, 0).getDate()
  const leading = (firstDate.getDay() + 6) % 7
  const cells = []

  for (let index = 0; index < 42; index += 1) {
    const day = index - leading + 1
    if (day < 1 || day > dayCount) {
      cells.push({ key: `empty-${index}`, inMonth: false, date: '', day: null, data: null })
      continue
    }
    const date = `${year}-${String(month).padStart(2, '0')}-${String(day).padStart(2, '0')}`
    const data = dayMap.get(day) || null
    cells.push({ key: date, inMonth: true, date, day, data })
  }

  return cells
}

export function getMonthInputValue(year, month) {
  if (!year || !month) return ''
  return `${year}-${String(month).padStart(2, '0')}`
}

export function parseMonthInputValue(value) {
  const match = String(value || '').match(/^(\d{4})-(\d{2})$/)
  if (!match) return null
  const year = Number(match[1])
  const month = Number(match[2])
  if (year < 2000 || year > 2100 || month < 1 || month > 12) return null
  return { year, month }
}

export function shiftCalendarMonth(year, month, delta) {
  const date = new Date(year, month - 1 + delta, 1)
  return { year: date.getFullYear(), month: date.getMonth() + 1 }
}

export function getSelectedDay(data, selectedDate) {
  const days = Array.isArray(data?.days) ? data.days : []
  return days.find((day) => day.date === selectedDate) || null
}

function CalendarMetricValue({ day, metric, scope }) {
  if (!day?.has_data) {
    return <span className="text-white/18">--</span>
  }
  const value = metric === 'rate' ? day.pnl_rate : day.pnl_amount
  const text = metric === 'rate' ? formatCalendarPnlRate(value) : formatCalendarCellPnlAmount(value)
  const title = metric === 'rate' ? text : formatCalendarPnlAmount(value, scope)
  return <span className={getCalendarPnlColor(value)} title={title}>{text}</span>
}

export default function PortfolioPnlCalendar({
  data,
  loading,
  error,
  scope = 'ASHARE',
  availableScopes = ['ASHARE'],
  selectedDate,
  displayMetric = 'amount',
  onScopeChange,
  onMonthChange,
  onDateSelect,
  onDisplayMetricChange,
  onRetry,
}) {
  const year = data?.year || new Date().getFullYear()
  const month = data?.month || (new Date().getMonth() + 1)
  const grid = buildCalendarGrid(year, month, data?.days)
  const monthInputValue = getMonthInputValue(year, month)
  const monthAmountColor = getCalendarPnlColor(data?.month_pnl_amount)
  const monthRateColor = getCalendarPnlColor(data?.month_pnl_rate)
  const scopes = (Array.isArray(availableScopes) && availableScopes.length > 0 ? availableScopes : ['ASHARE'])
    .filter((item) => item === 'ASHARE' || item === 'HKEX')
  const visibleScopes = scopes.length > 0 ? Array.from(new Set(scopes)) : ['ASHARE']

  const handlePrevMonth = () => onMonthChange?.(shiftCalendarMonth(year, month, -1))
  const handleNextMonth = () => onMonthChange?.(shiftCalendarMonth(year, month, 1))
  const handleMonthInput = (event) => {
    const parsed = parseMonthInputValue(event.target.value)
    if (parsed) onMonthChange?.(parsed)
  }

  return (
    <section className="rounded-2xl border border-white/10 bg-white/[0.03] p-4 sm:p-5">
      <div className="mb-4 flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
        <div>
          <h3 className="flex items-center gap-2 text-sm font-semibold text-white/80">
            <svg className="h-4 w-4 text-primary" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <rect x="3" y="4" width="18" height="17" rx="2" />
              <path d="M8 2v4M16 2v4M3 10h18" />
            </svg>
            <LabelWithInfo label="盈亏日历" tooltip={CALENDAR_TOOLTIP} />
          </h3>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <div className="inline-flex items-center rounded-xl border border-white/10 bg-black/20 p-1">
            {METRIC_OPTIONS.map((option) => (
              <button
                key={option.value}
                type="button"
                onClick={() => onDisplayMetricChange?.(option.value)}
                className={`rounded-lg px-2.5 py-1 text-[11px] font-medium transition ${
                  displayMetric === option.value
                    ? 'bg-primary/[0.14] text-primary'
                    : 'text-white/45 hover:bg-white/[0.05] hover:text-white/80'
                }`}
              >
                {option.label}
              </button>
            ))}
          </div>
          <InfoTip text={CALENDAR_TOOLTIP} iconClassName="border-white/12 text-white/30 hover:border-white/25 hover:text-white/55" />
        </div>
      </div>

      <div className="mb-4 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex flex-wrap items-center gap-2">
          {visibleScopes.length > 1 ? (
            <div className="inline-flex items-center rounded-xl border border-white/10 bg-black/20 p-1">
              {visibleScopes.map((item) => (
                <button
                  key={item}
                  type="button"
                  onClick={() => onScopeChange?.(item)}
                  className={`rounded-lg px-3 py-1.5 text-xs font-medium transition ${
                    scope === item
                      ? 'bg-primary/[0.14] text-primary'
                      : 'text-white/45 hover:bg-white/[0.05] hover:text-white/80'
                  }`}
                >
                  {SCOPE_LABELS[item] || item}
                </button>
              ))}
            </div>
          ) : (
            <div className="rounded-xl border border-white/10 bg-black/20 px-3 py-1.5 text-xs text-white/55">
              {SCOPE_LABELS[scope] || 'A股'}
            </div>
          )}

          <div className="inline-flex items-center gap-1 rounded-xl border border-white/10 bg-black/20 p-1">
            <button type="button" onClick={handlePrevMonth} className="rounded-lg px-2 py-1 text-xs text-white/45 transition hover:bg-white/[0.05] hover:text-white/85">上一月</button>
            <input
              type="month"
              value={monthInputValue}
              onChange={handleMonthInput}
              className="rounded-lg border border-white/10 bg-black/25 px-2 py-1 text-xs text-white/70 outline-none transition focus:border-primary/40"
            />
            <button type="button" onClick={handleNextMonth} className="rounded-lg px-2 py-1 text-xs text-white/45 transition hover:bg-white/[0.05] hover:text-white/85">下一月</button>
          </div>
        </div>

        {data ? (
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-[11px] text-white/35 sm:justify-end">
            <span className={monthAmountColor}>本月收益 {formatCalendarPnlAmount(data.month_pnl_amount || 0, scope)}</span>
            <span className="text-white/20">·</span>
            <span className={monthRateColor}>本月收益率 {formatCalendarPnlRate(data.month_pnl_rate)}</span>
            <span className="text-white/20">·</span>
            <span>{data.currency_code || (scope === 'HKEX' ? 'HKD' : 'CNY')}</span>
          </div>
        ) : null}
      </div>

      {error ? (
        <div className="rounded-xl border border-rose-400/25 bg-rose-500/[0.08] px-4 py-3 text-sm text-rose-200">
          {error}
          <button type="button" onClick={onRetry} className="ml-3 underline decoration-rose-200/40 underline-offset-4 transition hover:text-white">
            重试
          </button>
        </div>
      ) : null}

      <div className={`relative ${loading ? 'pointer-events-none opacity-55' : ''}`}>
        <div className="grid grid-cols-7 gap-1.5 text-center text-[10px] text-white/28 sm:gap-2">
          {WEEKDAY_LABELS.map((label) => (
            <div key={label} className="py-1 font-medium">{label}</div>
          ))}
        </div>

        <div className="mt-1 grid grid-cols-7 gap-1.5 sm:gap-2">
          {grid.map((cell) => {
            if (!cell.inMonth) {
              return <div key={cell.key} className="min-h-[68px] rounded-xl border border-white/[0.03] bg-white/[0.01] sm:min-h-[72px]" />
            }
            const day = cell.data || { date: cell.date, day: cell.day, has_data: false }
            const isSelected = selectedDate === cell.date
            return (
              <button
                key={cell.key}
                type="button"
                onClick={() => onDateSelect?.(cell.date)}
                className={`min-h-[68px] rounded-xl border p-1 text-left transition sm:min-h-[72px] sm:p-2 ${
                  isSelected
                    ? 'border-primary/45 bg-primary/[0.10] shadow-[0_0_0_1px_rgba(245,158,11,0.12)]'
                    : day.is_today
                      ? 'border-primary/20 bg-primary/[0.04]'
                      : 'border-white/[0.06] bg-black/15 hover:border-white/14 hover:bg-white/[0.035]'
                }`}
              >
                <div className="flex items-center justify-between gap-1">
                  <span className={`text-[11px] font-medium ${day.is_today ? 'text-primary' : 'text-white/55'}`}>{cell.day}</span>
                  {day.is_today ? <span className="rounded-full bg-primary/15 px-1 text-[9px] text-primary">今</span> : null}
                </div>
                <div className="mt-2 whitespace-normal break-words text-center text-[10px] font-semibold leading-tight tabular-nums sm:text-[11px]">
                  <CalendarMetricValue day={day} metric={displayMetric} scope={scope} />
                </div>
              </button>
            )
          })}
        </div>

        {loading ? (
          <div className="absolute inset-0 flex items-center justify-center rounded-2xl bg-black/20 backdrop-blur-[1px]">
            <div className="rounded-full border border-white/10 bg-black/50 px-3 py-1.5 text-xs text-white/45">加载日历中...</div>
          </div>
        ) : null}
      </div>

    </section>
  )
}
