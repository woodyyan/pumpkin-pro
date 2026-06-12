import { useEffect, useRef, useState } from 'react'

import NavDropdown from './NavDropdown'
import { buildNavigationState } from '../lib/navigation'

function Chevron({ open }) {
  return (
    <svg
      width="14"
      height="14"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      className={`transition ${open ? 'rotate-180' : ''}`}
      aria-hidden="true"
    >
      <path d="M6 9l6 6 6-6" />
    </svg>
  )
}

export default function DesktopNavMenu({ currentPath, unreadCount }) {
  const { groups } = buildNavigationState(currentPath, unreadCount)
  const [openGroupKey, setOpenGroupKey] = useState(null)
  const navRef = useRef(null)

  useEffect(() => {
    setOpenGroupKey(null)
  }, [currentPath])

  useEffect(() => {
    if (!openGroupKey) return undefined

    const onClickOutside = (event) => {
      if (!navRef.current?.contains(event.target)) {
        setOpenGroupKey(null)
      }
    }

    const onKeyDown = (event) => {
      if (event.key === 'Escape') {
        setOpenGroupKey(null)
      }
    }

    window.addEventListener('mousedown', onClickOutside)
    window.addEventListener('keydown', onKeyDown)
    return () => {
      window.removeEventListener('mousedown', onClickOutside)
      window.removeEventListener('keydown', onKeyDown)
    }
  }, [openGroupKey])

  return (
    <div ref={navRef} className="hidden md:flex items-center gap-2 text-base font-medium">
      {groups.map((group) => {
        const isOpen = openGroupKey === group.key
        const triggerClassName = `inline-flex items-center gap-2 rounded-lg border px-3 py-1.5 transition ${
          group.isActive || isOpen
            ? 'border-primary/50 bg-primary/15 text-primary font-semibold'
            : 'border-transparent text-foreground-muted hover:border-border hover:bg-[var(--color-bg-hover)] hover:text-foreground'
        }`

        return (
          <div
            key={group.key}
            className="relative"
            onMouseEnter={() => setOpenGroupKey(group.key)}
            onMouseLeave={() => setOpenGroupKey((prev) => (prev === group.key ? null : prev))}
          >
            <button
              type="button"
              onClick={() => setOpenGroupKey((prev) => (prev === group.key ? null : group.key))}
              onFocus={() => setOpenGroupKey(group.key)}
              className={triggerClassName}
              aria-haspopup="menu"
              aria-expanded={isOpen}
            >
              <span>{group.label}</span>
              {group.badge ? (
                <span className="inline-flex items-center justify-center min-w-[18px] h-[18px] px-1 rounded-full bg-rose-500 text-[10px] font-bold text-white leading-none">
                  {group.badge}
                </span>
              ) : null}
              <Chevron open={isOpen} />
            </button>

            {isOpen ? <NavDropdown items={group.items} /> : null}
          </div>
        )
      })}
    </div>
  )
}
