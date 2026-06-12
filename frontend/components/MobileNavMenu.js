import Link from 'next/link'
import { useEffect, useState } from 'react'

import { buildNavigationState } from '../lib/navigation'

/**
 * 移动端导航抽屉（Drawer）。
 *
 * 交互：从屏幕左侧滑入，所有分组与子项「平铺」展示，点击任意子项直达，
 * 无需先展开一级菜单。点击右侧遮罩或子项链接均可关闭。
 *
 * 层叠设计：遮罩 (z-0) 在底层、面板 (z-10) 在上层，两者为兄弟节点且各自
 * 显式声明 z-index，避免依赖隐式层叠顺序（此前 overlay 方案在 iOS Safari /
 * Chrome 移动模拟下出现点击被遮罩拦截的根因）。
 *
 * 桌面端不渲染本组件（md:hidden），桌面导航由 DesktopNavMenu 负责，保持不变。
 */
export default function MobileNavMenu({ open, currentPath, unreadCount, onClose }) {
  const { groups } = buildNavigationState(currentPath, unreadCount)
  const [mounted, setMounted] = useState(false)

  // 延迟一帧再上滑入态，保证 transition 生效（先挂载在屏幕外，再过渡进来）。
  useEffect(() => {
    if (!open) {
      setMounted(false)
      return undefined
    }

    const raf = requestAnimationFrame(() => setMounted(true))
    return () => cancelAnimationFrame(raf)
  }, [open])

  // 支持 Esc 关闭。
  useEffect(() => {
    if (!open) return undefined

    const onKeyDown = (event) => {
      if (event.key === 'Escape') {
        onClose?.()
      }
    }

    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [open, onClose])

  if (!open) {
    return null
  }

  return (
    <div className="fixed inset-0 z-40 md:hidden">
      <button
        type="button"
        aria-label="关闭移动导航菜单"
        onClick={onClose}
        className={`absolute inset-0 z-0 bg-black/50 transition-opacity duration-200 ${
          mounted ? 'opacity-100' : 'opacity-0'
        }`}
      />

      <aside
        role="dialog"
        aria-modal="true"
        aria-label="移动导航菜单"
        className={`absolute inset-y-0 left-0 z-10 flex h-full w-[80vw] max-w-[320px] flex-col border-r border-border bg-[var(--color-bg-overlay)] shadow-2xl transition-transform duration-200 ease-out ${
          mounted ? 'translate-x-0' : '-translate-x-full'
        }`}
      >
        <div className="flex h-16 shrink-0 items-center justify-between border-b border-border px-4">
          <span className="text-base font-bold tracking-tight text-foreground">导航</span>
          <button
            type="button"
            onClick={onClose}
            aria-label="关闭"
            className="inline-flex h-9 w-9 items-center justify-center rounded-lg border border-border text-foreground-muted transition hover:bg-[var(--color-bg-hover)] hover:text-foreground"
          >
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" aria-hidden="true">
              <path d="M18 6L6 18M6 6l12 12" />
            </svg>
          </button>
        </div>

        <nav
          aria-label="移动导航"
          className="flex-1 overflow-y-auto px-3 py-3 pb-[max(1rem,env(safe-area-inset-bottom))]"
        >
          {groups.map((group) => (
            <div key={group.key} className="mb-4 last:mb-0">
              <div className="flex items-center gap-2 px-2 pb-1.5 text-xs font-semibold uppercase tracking-wide text-foreground-dim">
                <span>{group.label}</span>
                {group.badge ? (
                  <span className="inline-flex h-[18px] min-w-[18px] items-center justify-center rounded-full bg-rose-500 px-1 text-[10px] font-bold leading-none text-white">
                    {group.badge}
                  </span>
                ) : null}
              </div>

              <div className="space-y-1">
                {group.items.map((item) => (
                  <Link
                    key={item.key}
                    href={item.href}
                    onClick={onClose}
                    aria-current={item.isActive ? 'page' : undefined}
                    className={`flex items-center justify-between rounded-lg px-3 py-2.5 text-sm transition ${
                      item.isActive
                        ? 'border-l-2 border-primary bg-primary/15 font-medium text-foreground'
                        : 'text-foreground-muted hover:bg-[var(--color-bg-hover)] hover:text-foreground'
                    }`}
                  >
                    <span>{item.label}</span>
                    {item.badge ? (
                      <span className="ml-3 inline-flex h-[18px] min-w-[18px] items-center justify-center rounded-full bg-rose-500 px-1 text-[10px] font-bold leading-none text-white">
                        {item.badge}
                      </span>
                    ) : null}
                  </Link>
                ))}
              </div>
            </div>
          ))}
        </nav>
      </aside>
    </div>
  )
}
