import Head from 'next/head'
import Link from 'next/link'
import { useRouter } from 'next/router'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { adminFetch, clearAdminResourceCache } from '../../lib/admin-data'
import {
  ADMIN_NAV_ITEMS,
  ADMIN_TAB_ROUTE_MAP,
  findAdminNavItem,
} from './navigation'
import { AdminLoginForm } from './AdminSections'

function AdminNavLink({ item, active, compact = false, onNavigate }) {
  return (
    <Link
      href={item.href}
      onClick={onNavigate}
      aria-current={active ? 'page' : undefined}
      className={`block rounded-2xl border px-4 py-3 transition ${
        active
          ? 'border-primary/30 bg-primary/10 text-foreground'
          : 'border-transparent bg-card text-foreground-muted hover:border-border hover:bg-[var(--color-bg-hover)] hover:text-foreground'
      }`}
    >
      <div className="flex items-center justify-between gap-3">
        <span className="text-sm font-semibold">{compact ? item.shortLabel : item.label}</span>
        {active ? <span className="text-[11px] text-primary">当前</span> : null}
      </div>
      {!compact ? <p className="mt-1 text-xs text-foreground-dim">{item.description}</p> : null}
    </Link>
  )
}

export default function AdminShell({ section = 'overview', children }) {
  const router = useRouter()
  const [session, setSession] = useState(null)
  const [ready, setReady] = useState(false)
  const [mobileNavOpen, setMobileNavOpen] = useState(false)

  const activeItem = useMemo(() => findAdminNavItem(section), [section])
  const adminEmail = session?.admin?.email || '管理员'

  const handleLogin = useCallback((result) => {
    clearAdminResourceCache()
    setSession(result)
  }, [])

  const handleLogout = useCallback(() => {
    clearAdminResourceCache()
    setSession(null)
    setMobileNavOpen(false)
  }, [])

  const logout = useCallback(() => {
    adminFetch('/api/admin/logout', { method: 'POST' }).catch(() => null).finally(() => handleLogout())
  }, [handleLogout])

  useEffect(() => {
    let active = true
    adminFetch('/api/admin/session')
      .then((data) => {
        if (active) setSession(data)
      })
      .catch(() => {
        if (active) {
          clearAdminResourceCache()
          setSession(null)
        }
      })
      .finally(() => {
        if (active) setReady(true)
      })
    return () => {
      active = false
    }
  }, [])

  useEffect(() => {
    setMobileNavOpen(false)
  }, [router.asPath])

  useEffect(() => {
    if (!router.isReady) return
    const legacyTab = typeof router.query?.tab === 'string' ? router.query.tab : ''
    const target = ADMIN_TAB_ROUTE_MAP[legacyTab]
    if (!target || router.pathname === target) return
    const nextQuery = { ...router.query }
    delete nextQuery.tab
    router.replace({ pathname: target, query: nextQuery }, undefined, { shallow: true }).catch(() => null)
  }, [router])

  if (!ready) {
    return (
      <>
        <Head>
          <title>管理后台 — Wolong Pro</title>
          <meta name="robots" content="noindex, nofollow" />
        </Head>
        <div className="min-h-screen bg-background" />
      </>
    )
  }

  if (!session) {
    return (
      <>
        <Head>
          <title>管理后台 — Wolong Pro</title>
          <meta name="robots" content="noindex, nofollow" />
        </Head>
        <AdminLoginForm onLogin={handleLogin} />
      </>
    )
  }

  return (
    <>
      <Head>
        <title>{activeItem.label} — 管理后台</title>
        <meta name="robots" content="noindex, nofollow" />
      </Head>

      <div className="min-h-screen bg-background text-foreground">
        <header className="sticky top-0 z-50 border-b border-border bg-background/90 backdrop-blur-md">
          <div className="mx-auto flex max-w-7xl items-center justify-between gap-3 px-4 py-3 sm:px-6">
            <div className="flex items-center gap-3">
              <button
                type="button"
                onClick={() => setMobileNavOpen((open) => !open)}
                className="inline-flex h-10 w-10 items-center justify-center rounded-xl border border-border bg-card text-foreground md:hidden"
                aria-label="切换管理导航"
                aria-expanded={mobileNavOpen}
              >
                <span className="text-lg">≡</span>
              </button>
              <img src="/logo.png" alt="卧龙" width={32} height={32} className="rounded" />
              <div>
                <div className="text-lg font-bold">Wolong Pro 管理后台</div>
                <div className="text-xs text-foreground-dim">{activeItem.description}</div>
              </div>
            </div>

            <div className="flex items-center gap-3 text-sm">
              <span className="hidden text-foreground-muted sm:inline">{adminEmail}</span>
              <button
                type="button"
                onClick={logout}
                className="rounded-lg border border-[var(--color-border-strong)] px-3 py-1.5 text-sm text-foreground-muted transition hover:border-[var(--color-border-strong)] hover:text-foreground"
              >
                退出
              </button>
            </div>
          </div>
        </header>

        {mobileNavOpen ? (
          <div className="border-b border-border bg-background px-4 py-4 md:hidden">
            <div className="space-y-2">
              {ADMIN_NAV_ITEMS.map((item) => (
                <AdminNavLink
                  key={item.key}
                  item={item}
                  active={item.key === activeItem.key}
                  onNavigate={() => setMobileNavOpen(false)}
                />
              ))}
            </div>
          </div>
        ) : null}

        <div className="mx-auto flex max-w-7xl gap-6 px-4 py-6 sm:px-6">
          <aside className="sticky top-[88px] hidden h-fit w-72 shrink-0 space-y-3 md:block">
            {ADMIN_NAV_ITEMS.map((item) => (
              <AdminNavLink key={item.key} item={item} active={item.key === activeItem.key} />
            ))}
          </aside>

          <main className="min-w-0 flex-1">
            <div className="mb-6 rounded-2xl border border-border bg-card px-4 py-4 sm:px-5">
              <div className="text-xs uppercase tracking-[0.2em] text-foreground-disabled">Admin</div>
              <h1 className="mt-2 text-2xl font-semibold text-foreground">{activeItem.label}</h1>
              <p className="mt-1 text-sm text-foreground-dim">{activeItem.description}</p>
            </div>
            {typeof children === 'function' ? children({ session, onUnauthorized: handleLogout }) : children}
          </main>
        </div>
      </div>
    </>
  )
}
