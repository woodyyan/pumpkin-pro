const INVESTMENT_PROFILE_STORAGE_KEY = 'pumpkin_pro_investment_profile'
const INVESTMENT_PROFILE_EVENT = 'pumpkin:investment-profile-updated'

function isBrowser() {
  return typeof window !== 'undefined' && !!window.localStorage
}

export function readInvestmentProfileCache() {
  if (!isBrowser()) return null
  const text = window.localStorage.getItem(INVESTMENT_PROFILE_STORAGE_KEY)
  if (!text) return null
  try {
    const parsed = JSON.parse(text)
    return parsed && typeof parsed === 'object' ? parsed : null
  } catch {
    return null
  }
}

export function writeInvestmentProfileCache(profile) {
  if (!isBrowser()) return
  if (!profile || typeof profile !== 'object') {
    window.localStorage.removeItem(INVESTMENT_PROFILE_STORAGE_KEY)
    return
  }
  window.localStorage.setItem(INVESTMENT_PROFILE_STORAGE_KEY, JSON.stringify(profile))
}

export function clearInvestmentProfileCache() {
  if (!isBrowser()) return
  window.localStorage.removeItem(INVESTMENT_PROFILE_STORAGE_KEY)
}

export function dispatchInvestmentProfileUpdated(profile) {
  if (typeof window === 'undefined') return
  window.dispatchEvent(new CustomEvent(INVESTMENT_PROFILE_EVENT, { detail: profile || null }))
}

export function subscribeInvestmentProfileUpdates(listener) {
  if (typeof window === 'undefined' || typeof listener !== 'function') return () => {}

  const handleCustomEvent = (event) => {
    listener(event?.detail || null)
  }

  const handleStorageEvent = (event) => {
    if (event.key !== INVESTMENT_PROFILE_STORAGE_KEY) return
    if (!event.newValue) {
      listener(null)
      return
    }
    try {
      const parsed = JSON.parse(event.newValue)
      listener(parsed && typeof parsed === 'object' ? parsed : null)
    } catch {
      listener(null)
    }
  }

  window.addEventListener(INVESTMENT_PROFILE_EVENT, handleCustomEvent)
  window.addEventListener('storage', handleStorageEvent)
  return () => {
    window.removeEventListener(INVESTMENT_PROFILE_EVENT, handleCustomEvent)
    window.removeEventListener('storage', handleStorageEvent)
  }
}

export {
  INVESTMENT_PROFILE_EVENT,
  INVESTMENT_PROFILE_STORAGE_KEY,
}
