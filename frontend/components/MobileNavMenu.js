import Link from 'next/link'
import { useEffect, useState } from 'react'

import { buildNavigationState } from '../lib/navigation'

function Chevron({ open }) {
  return (
    <svg
      width="16"
      height="16"
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

export default function MobileNavMenu({ open, currentPath, unreadCount }) {
  const { groups, activeGroupKey } = buildNavigationState(currentPath, unreadCount)
  const [expandedGroupKey, setExpandedGroupKey] = useState(activeGroupKey)

  useEffect(() => {
    if (open) {
      setExpandedGroupKey(activeGroupKey || groups[0]?.key || null)
    }
  }, [activeGroupKey, groups, open])

  if (!open) {
    return null
  }

  return (
    <div className="md:hidden border-t border-border bg-[var(--color-bg-overlay)] backdrop-blur-md px-4 py-3 space-y-2">
      {groups.map((group) => {
        const isExpanded = expandedGroupKey === group.key

        return (
          <div key={group.key} className="rounded-xl border border-border bg-card/80 p-2">
            <button
              type="button"
              onClick={() => setExpandedGroupKey((prev) => (prev === group.key ? null : group.key))}
              className={`flex w-full items-center justify-between rounded-lg px-3 py-2 text-sm font-medium transition ${
                group.isActive
                  ? 'bg-primary/15 text-foreground'
                  : 'text-foreground-muted hover:bg-[var(--color-bg-hover)] hover:text-foreground'
              }`}
              aria-expanded={isExpanded}
            >
              <span className="inline-flex items-center gap-2">
                <span>{group.label}</span>
                {group.badge ? (
                  <span className="inline-flex items-center justify-center min-w-[18px] h-[18px] px-1 rounded-full bg-rose-500 text-[10px] font-bold text-white leading-none">
                    {group.badge}
                  </span>
                ) : null}
              </span>
              <Chevron open={isExpanded} />
            </button>

            {isExpanded ? (
              <div className="mt-2 ml-3 space-y-1 border-l border-border pl-3">
                {group.items.map((item) => (
                  <Link
                    key={item.key}
                    href={item.href}
                    className={`flex items-center justify-between rounded-lg px-3 py-2 text-sm transition ${
                      item.isActive
                        ? 'bg-primary/15 text-foreground border-l-2 border-primary'
                        : 'text-foreground-muted hover:bg-[var(--color-bg-hover)] hover:text-foreground'
                    }`}
                  >
                    <span>{item.label}</span>
                    {item.badge ? (
                      <span className="ml-3 inline-flex items-center justify-center min-w-[18px] h-[18px] px-1 rounded-full bg-rose-500 text-[10px] font-bold text-white leading-none">
                        {item.badge}
                      </span>
                    ) : null}
                  </Link>
                ))}
              </div>
            ) : null}
          </div>
        )
      })}
    </div>
  )
}
