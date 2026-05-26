'use client'

import { useState, useRef, useEffect } from 'react'
import { useTheme } from '../lib/theme-context'

/**
 * Icons for theme states (inline SVGs — no external dependencies).
 */
const SunIcon = () => (
  <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
    <circle cx="12" cy="12" r="5" />
    <line x1="12" y1="1" x2="12" y2="3" />
    <line x1="12" y1="21" x2="12" y2="23" />
    <line x1="4.22" y1="4.22" x2="5.64" y2="5.64" />
    <line x1="18.36" y1="18.36" x2="19.78" y2="19.78" />
    <line x1="1" y1="12" x2="3" y2="12" />
    <line x1="21" y1="12" x2="23" y2="12" />
    <line x1="4.22" y1="19.78" x2="5.64" y2="18.36" />
    <line x1="18.36" y1="5.64" x2="19.78" y2="4.22" />
  </svg>
)

const MoonIcon = () => (
  <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
    <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z" />
  </svg>
)

const SystemIcon = () => (
  <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
    <rect x="2" y="3" width="20" height="14" rx="2" ry="2" />
    <line x1="8" y1="21" x2="16" y2="21" />
    <line x1="12" y1="17" x2="12" y2="21" />
  </svg>
)

const THEME_OPTIONS = [
  { key: 'light', label: '浅色', icon: SunIcon },
  { key: 'dark', label: '深色', icon: MoonIcon },
  { key: 'system', label: '跟随系统', icon: SystemIcon },
]

/**
 * ThemeToggle button with optional dropdown for three-way selection.
 *
 * Renders in the top navbar. Clicking opens a small dropdown allowing the
 * user to pick between Light / Dark / System.
 */
export default function ThemeToggle() {
  const { theme, setTheme } = useTheme()
  const [open, setOpen] = useState(false)
  const menuRef = useRef(null)

  // Close dropdown on outside click
  useEffect(() => {
    if (!open) return
    const handler = (e) => {
      if (menuRef.current && !menuRef.current.contains(e.target)) {
        setOpen(false)
      }
    }
    window.addEventListener('mousedown', handler)
    return () => window.removeEventListener('mousedown', handler)
  }, [open])

  // Close on Escape key
  useEffect(() => {
    if (!open) return
    const handler = (e) => {
      if (e.key === 'Escape') setOpen(false)
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [open])

  const currentOption = THEME_OPTIONS.find((o) => o.key === theme) || THEME_OPTIONS[2]
  const CurrentIcon = currentOption.icon

  return (
    <div ref={menuRef} className="relative shrink-0">
      <button
        type="button"
        onClick={() => setOpen((prev) => !prev)}
        className="inline-flex items-center justify-center w-9 h-9 rounded-lg border border-border text-foreground-muted transition hover:border-border-strong hover:text-foreground hover:bg-[var(--color-bg-hover)]"
        aria-label={`主题切换，当前：${currentOption.label}`}
        title={`主题：${currentOption.label}`}
      >
        <CurrentIcon />
      </button>

      {open && (
        <div className="absolute right-0 mt-2 w-36 rounded-xl border border-border bg-[var(--color-bg-card)] shadow-2xl backdrop-blur p-1.5 z-50">
          {THEME_OPTIONS.map((opt) => {
            const Icon = opt.icon
            const isActive = theme === opt.key
            return (
              <button
                key={opt.key}
                type="button"
                onClick={() => { setTheme(opt.key); setOpen(false) }}
                className={`flex items-center w-full gap-2.5 rounded-lg px-3 py-2 text-sm font-medium transition ${
                  isActive
                    ? 'bg-primary/10 text-primary'
                    : 'text-foreground-muted hover:bg-[var(--color-bg-hover)] hover:text-foreground'
                }`}
              >
                <Icon />
                <span className="whitespace-nowrap">{opt.label}</span>
                {isActive && (
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" className="ml-auto">
                    <polyline points="20 6 9 17 4 12" />
                  </svg>
                )}
              </button>
            )
          })}
        </div>
      )}
    </div>
  )
}
