'use client'

import { createContext, useContext, useState, useEffect, useCallback, useMemo } from 'react'

const ThemeContext = createContext(undefined)

const THEME_KEY = 'wolong_theme'
const CLASS_LIGHT = 'light'
const CLASS_DARK = 'dark'

/**
 * Read persisted theme from localStorage.
 * Returns "light" | "dark" | "system" (system is the default when unset).
 */
function readStoredTheme() {
  if (typeof window === 'undefined') return 'system'
  try {
    const stored = localStorage.getItem(THEME_KEY)
    if (stored === 'light' || stored === 'dark' || stored === 'system') return stored
  } catch {
    // localStorage unavailable — ignore
  }
  return 'system'
}

/**
 * Resolve "system" to concrete "light" or "dark" using matchMedia.
 */
function resolveSystemTheme() {
  if (typeof window === 'undefined') return 'dark'
  try {
    return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
  } catch {
    return 'dark'
  }
}

function resolveTheme(theme) {
  if (theme === 'system') return resolveSystemTheme()
  return theme
}

function applyHtmlClass(resolved) {
  const html = document.documentElement
  html.classList.remove(CLASS_LIGHT, CLASS_DARK)
  html.classList.add(resolved === 'dark' ? CLASS_DARK : CLASS_LIGHT)
}

/**
 * ThemeProvider — wraps the app to provide theme state and toggle capability.
 *
 * @example
 * <ThemeProvider>
 *   <App />
 * </ThemeProvider>
 */
export function ThemeProvider({ children }) {
  const [theme, setThemeState] = useState('system') // "light" | "dark" | "system"
  const [resolvedTheme, setResolvedTheme] = useState('dark')
  const [mounted, setMounted] = useState(false)

  // On mount: read stored preference and sync with <html> class
  useEffect(() => {
    const stored = readStoredTheme()
    setThemeState(stored)
    const resolved = resolveTheme(stored)
    setResolvedTheme(resolved)
    applyHtmlClass(resolved)
    setMounted(true)
  }, [])

  // Listen for OS theme changes when in "system" mode
  useEffect(() => {
    if (typeof window === 'undefined') return

    let mql
    try {
      mql = window.matchMedia('(prefers-color-scheme: dark)')
    } catch {
      return
    }

    const handler = () => {
      setThemeState((prev) => {
        if (prev !== 'system') return prev
        const nextResolved = resolveTheme('system')
        setResolvedTheme(nextResolved)
        applyHtmlClass(nextResolved)
        return prev
      })
    }

    mql.addEventListener('change', handler)
    return () => {
      try { mql.removeEventListener('change', handler) } catch { /* ignore */ }
    }
  }, [])

  const setTheme = useCallback((nextTheme) => {
    if (nextTheme !== 'light' && nextTheme !== 'dark' && nextTheme !== 'system') return
    setThemeState(nextTheme)
    const resolved = resolveTheme(nextTheme)
    setResolvedTheme(resolved)
    applyHtmlClass(resolved)
    try {
      if (nextTheme === 'system') {
        localStorage.removeItem(THEME_KEY)
      } else {
        localStorage.setItem(THEME_KEY, nextTheme)
      }
    } catch {
      // localStorage unavailable — ignore
    }
  }, [])

  const value = useMemo(() => ({
    theme,
    resolvedTheme,
    setTheme,
    mounted,
  }), [theme, resolvedTheme, setTheme, mounted])

  return (
    <ThemeContext.Provider value={value}>
      {children}
    </ThemeContext.Provider>
  )
}

/**
 * useTheme — access theme context from any component.
 *
 * @returns {{ theme: string, resolvedTheme: string, setTheme: Function, mounted: boolean }}
 */
export function useTheme() {
  const ctx = useContext(ThemeContext)
  if (ctx === undefined) {
    throw new Error('useTheme must be used within a ThemeProvider')
  }
  return ctx
}
