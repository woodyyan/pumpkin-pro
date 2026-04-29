const DEFAULT_NOTIFICATION_ICON = '/favicon.ico'
const NOTIFICATION_PREFERENCES_KEY = 'wolong_notification_prefs'
const MAX_NOTIFICATION_AGE_MS = 30000

export const NOTIFICATION_CATEGORIES = {
  AI_ANALYSIS: 'ai_analysis',
  SIGNAL_TRIGGER: 'signal_trigger',
  SYSTEM: 'system',
}

const SIGNAL_META = {
  buy: { label: '看多', emoji: '🔴', hint: '偏多配置' },
  sell: { label: '看空', emoji: '🟢', hint: '注意风险' },
  hold: { label: '观望', emoji: '🟡', hint: '持仓不变' },
}

const SIGNAL_FALLBACK_TEXT = {
  buy: '技术面与基础面共振，当前处于看多区间',
  sell: '多项指标提示风险，建议关注下行压力',
  hold: '多空因素交织，建议观望等待更明确信号',
}

export function isNotificationSupported() {
  return typeof window !== 'undefined' && 'Notification' in window
}

export function getNotificationPermission() {
  if (!isNotificationSupported()) return 'unsupported'
  return Notification.permission
}

export async function requestNotificationPermission() {
  if (!isNotificationSupported()) return 'unsupported'
  try {
    const permission = await Notification.requestPermission()
    return permission
  } catch {
    return 'denied'
  }
}

export function isIOSSafari() {
  if (typeof window === 'undefined' || typeof navigator === 'undefined') return false
  const ua = navigator.userAgent
  return /iPad|iPhone|iPod/.test(ua) && !window.MSStream && /Safari/.test(ua)
}

export function buildAIAnalysisNotification({ symbol, symbolName, signal, confidenceScore }) {
  const safeSymbol = symbolName || symbol || '--'
  const meta = SIGNAL_META[signal] || SIGNAL_META.hold
  const confidencePct = Math.min(100, Math.max(0, confidenceScore || 0))
  const confidenceText = confidencePct >= 70 ? '高置信度' : confidencePct >= 40 ? '中等置信度' : '低置信度'
  const body = `${meta.emoji} ${meta.label} — ${confidenceText} · ${SIGNAL_FALLBACK_TEXT[signal] || SIGNAL_FALLBACK_TEXT.hold}`

  return {
    title: `AI 分析完成 · ${safeSymbol}`,
    options: {
      body,
      tag: `ai-analysis-${symbol || 'unknown'}`,
      icon: DEFAULT_NOTIFICATION_ICON,
      silent: true,
      requireInteraction: false,
    },
  }
}

export function buildNotificationPayload(category, data) {
  switch (category) {
    case NOTIFICATION_CATEGORIES.AI_ANALYSIS:
      return buildAIAnalysisNotification(data)
    default:
      return null
  }
}

export function sendNotification(category, data) {
  if (!isNotificationSupported()) return false
  if (getNotificationPermission() !== 'granted') return false
  if (!isCategoryEnabled(category)) return false

  const payload = buildNotificationPayload(category, data)
  if (!payload) return false

  try {
    const notification = new Notification(payload.title, payload.options)
    notification.onclick = () => {
      if (typeof window !== 'undefined') {
        window.focus()
      }
    }
    return true
  } catch {
    return false
  }
}

export function closeNotificationByTag(tag) {
  if (!isNotificationSupported() || typeof navigator === 'undefined') return false
  try {
    navigator.notifications?.getNotifications?.({ tag }).then((notifications) => {
      notifications.forEach((n) => n.close())
    })
    return true
  } catch {
    return false
  }
}

export function loadNotificationPreferences() {
  if (typeof window === 'undefined') {
    return { aiAnalysis: true, signalTrigger: false, system: true }
  }
  try {
    const raw = window.localStorage.getItem(NOTIFICATION_PREFERENCES_KEY)
    if (!raw) return { aiAnalysis: true, signalTrigger: false, system: true }
    const parsed = JSON.parse(raw)
    return {
      aiAnalysis: parsed.aiAnalysis !== false,
      signalTrigger: parsed.signalTrigger === true,
      system: parsed.system !== false,
    }
  } catch {
    return { aiAnalysis: true, signalTrigger: false, system: true }
  }
}

export function saveNotificationPreferences(prefs) {
  if (typeof window === 'undefined') return false
  try {
    const normalized = {
      aiAnalysis: prefs?.aiAnalysis !== false,
      signalTrigger: prefs?.signalTrigger === true,
      system: prefs?.system !== false,
    }
    window.localStorage.setItem(NOTIFICATION_PREFERENCES_KEY, JSON.stringify(normalized))
    return true
  } catch {
    return false
  }
}

export function isCategoryEnabled(category) {
  const prefs = loadNotificationPreferences()
  switch (category) {
    case NOTIFICATION_CATEGORIES.AI_ANALYSIS:
      return prefs.aiAnalysis !== false
    case NOTIFICATION_CATEGORIES.SIGNAL_TRIGGER:
      return prefs.signalTrigger === true
    case NOTIFICATION_CATEGORIES.SYSTEM:
      return prefs.system !== false
    default:
      return false
  }
}
