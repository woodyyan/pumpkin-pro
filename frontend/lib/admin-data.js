import { useCallback, useEffect, useRef, useState } from 'react'

const adminResourceCache = new Map()

export async function adminFetch(path, init = {}) {
  const headers = new Headers(init?.headers || {})
  headers.set('Accept', 'application/json')

  const res = await fetch(path, { ...init, headers, credentials: 'same-origin' })
  const text = await res.text()
  let data = null
  try {
    data = JSON.parse(text)
  } catch {
    data = text
  }

  if (!res.ok) {
    const err = new Error(data?.detail || '请求失败')
    err.status = res.status
    err.code = data?.code || ''
    err.payload = data
    throw err
  }
  return data
}

export function isAdminUnauthorized(error) {
  return Number(error?.status) === 401
}

export function resolveAdminErrorMessage(error, fallbackMessage = '请求失败') {
  const text = typeof error?.message === 'string' ? error.message.trim() : ''
  return text || fallbackMessage
}

export function handleAdminActionError(error, onUnauthorized, fallbackMessage) {
  if (isAdminUnauthorized(error)) {
    onUnauthorized?.(error)
    return ''
  }
  return resolveAdminErrorMessage(error, fallbackMessage)
}

export function readFreshAdminResourceCache(key, staleMs, now = Date.now()) {
  if (!key) return null
  const entry = adminResourceCache.get(key)
  if (!entry) return null
  if (staleMs > 0 && now - entry.updatedAt > staleMs) return null
  return entry
}

export function shouldThrottleAdminRequest(lastLoadedAt, minIntervalMs, now = Date.now()) {
  if (!minIntervalMs || minIntervalMs <= 0 || !lastLoadedAt) return false
  return now - lastLoadedAt < minIntervalMs
}

export function resolveAdminPollInterval(pollMs, data) {
  const value = typeof pollMs === 'function' ? pollMs(data) : pollMs
  if (!value || value <= 0) return null
  return value
}

export function syncAdminResourceCache(key, data, updatedAt = Date.now()) {
  if (!key) return
  if (data == null) {
    adminResourceCache.delete(key)
    return
  }
  adminResourceCache.set(key, { data, updatedAt })
}

export function clearAdminResourceCache(key) {
  if (!key) {
    adminResourceCache.clear()
    return
  }
  adminResourceCache.delete(key)
}

export function useAdminResource({
  key,
  request,
  enabled = true,
  initialData = null,
  staleMs = 0,
  pollMs = null,
  minIntervalMs = 0,
  onUnauthorized,
  errorMessage = '加载失败',
  shouldPoll,
}) {
  const initialCache = enabled ? readFreshAdminResourceCache(key, staleMs) : null
  const initialSnapshot = initialCache?.data ?? initialData
  const initialLoadedAt = initialCache?.updatedAt || 0
  const [data, setData] = useState(() => initialSnapshot)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(() => enabled && initialCache == null && initialData == null)
  const [refreshing, setRefreshing] = useState(false)
  const [loadedAt, setLoadedAt] = useState(() => initialLoadedAt)
  const inFlightRef = useRef(null)
  const dataRef = useRef(initialSnapshot)
  const loadedAtRef = useRef(initialLoadedAt)
  const requestVersionRef = useRef(0)

  useEffect(() => {
    dataRef.current = data
  }, [data])

  const setDataAndCache = useCallback((nextData, updatedAt = Date.now()) => {
    dataRef.current = nextData
    loadedAtRef.current = nextData == null ? 0 : updatedAt
    setData(nextData)
    setLoadedAt(nextData == null ? 0 : updatedAt)
    syncAdminResourceCache(key, nextData, updatedAt)
  }, [key])

  useEffect(() => {
    requestVersionRef.current += 1
    inFlightRef.current = null
    const cached = enabled ? readFreshAdminResourceCache(key, staleMs) : null
    const nextData = cached?.data ?? initialData
    const nextLoadedAt = cached?.updatedAt || 0
    dataRef.current = nextData
    loadedAtRef.current = nextLoadedAt
    setData(nextData)
    setLoadedAt(nextLoadedAt)
    setError('')
    setRefreshing(false)
    setLoading(enabled && cached == null && initialData == null)
  }, [enabled, initialData, key, staleMs])

  const load = useCallback(async ({ preferCache = true, silent = false, force = false } = {}) => {
    if ((!enabled && !force) || typeof request !== 'function') {
      return dataRef.current
    }

    const now = Date.now()
    if (preferCache && !force) {
      const cached = readFreshAdminResourceCache(key, staleMs, now)
      if (cached) {
        if (cached.updatedAt !== loadedAtRef.current || cached.data !== dataRef.current) {
          dataRef.current = cached.data
          loadedAtRef.current = cached.updatedAt
          setData(cached.data)
          setLoadedAt(cached.updatedAt)
        }
        setError('')
        setLoading(false)
        return cached.data
      }
    }

    if (!force && shouldThrottleAdminRequest(loadedAtRef.current, minIntervalMs, now) && dataRef.current != null) {
      return dataRef.current
    }

    if (inFlightRef.current) {
      return inFlightRef.current
    }

    if (!silent) {
      if (dataRef.current == null) {
        setLoading(true)
      } else {
        setRefreshing(true)
      }
    }

    const requestVersion = requestVersionRef.current
    const task = (async () => {
      try {
        const result = await request()
        if (requestVersionRef.current !== requestVersion) {
          return dataRef.current
        }
        const updatedAt = Date.now()
        setDataAndCache(result, updatedAt)
        setError('')
        return result
      } catch (err) {
        if (requestVersionRef.current !== requestVersion) {
          return dataRef.current
        }
        if (isAdminUnauthorized(err)) {
          setError('')
          onUnauthorized?.(err)
          return null
        }
        setError(resolveAdminErrorMessage(err, errorMessage))
        return null
      } finally {
        if (requestVersionRef.current === requestVersion) {
          inFlightRef.current = null
          setLoading(false)
          setRefreshing(false)
        }
      }
    })()

    inFlightRef.current = task
    return task
  }, [enabled, errorMessage, key, minIntervalMs, onUnauthorized, request, setDataAndCache, staleMs])

  useEffect(() => {
    if (!enabled) return
    load({ preferCache: true, silent: dataRef.current != null }).catch(() => null)
  }, [enabled, load])

  useEffect(() => {
    if (!enabled) return
    if (shouldPoll && !shouldPoll(data)) return
    const interval = resolveAdminPollInterval(pollMs, data)
    if (!interval) return
    const timer = setTimeout(() => {
      load({ preferCache: false, silent: true, force: true }).catch(() => null)
    }, interval)
    return () => clearTimeout(timer)
  }, [data, enabled, load, pollMs, shouldPoll])

  const refresh = useCallback(() => load({ preferCache: false, force: true }), [load])
  const mutate = useCallback((next) => {
    const nextData = typeof next === 'function' ? next(dataRef.current) : next
    setDataAndCache(nextData)
  }, [setDataAndCache])

  return {
    data,
    error,
    loading,
    refreshing,
    loadedAt,
    refresh,
    mutate,
  }
}
