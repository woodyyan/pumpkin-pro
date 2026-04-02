import '../styles/globals.css'
import Head from 'next/head'
import Link from 'next/link'
import { useRouter } from 'next/router'
import { useEffect, useRef, useState } from 'react'

import { AuthProvider, useAuth } from '../lib/auth-context'

const NAV_ITEMS = [
  { href: '/strategies', label: '策略库' },
  { href: '/', label: '回测引擎' },
  { href: '/live-trading', label: '行情看板' },
  { href: '/stock-picker', label: '选股平台' },
  { href: '/settings', label: '设置' },
  { href: '/changelog', label: '更新日志' },
]

function AppLayout({ Component, pageProps }) {
  const router = useRouter()

  return (
    <>
      <Head>
        <title>卧龙AI量化交易台</title>
        <meta name="viewport" content="width=device-width, initial-scale=1" />
        <link rel="icon" type="image/png" href="/favicon.png" />
        <link rel="apple-touch-icon" href="/apple-touch-icon.png" />
      </Head>
      <div className="min-h-screen bg-background text-foreground flex flex-col">
        <nav className="fixed top-0 left-0 right-0 h-16 bg-black/60 backdrop-blur-md border-b border-white/10 z-50 flex items-center justify-between px-6 gap-4">
          <div className="flex items-center space-x-2.5 min-w-0">
            <img src="/logo.png" alt="卧龙" width={40} height={40} className="rounded" />
            <span className="text-xl font-bold tracking-tight truncate">卧龙AI量化交易台</span>
          </div>

          <div className="flex items-center space-x-2 text-base font-medium">
            {NAV_ITEMS.map((item) => {
              const isActive = router.pathname === item.href
              return (
                <Link
                  key={item.href}
                  href={item.href}
                  className={`inline-flex items-center rounded-lg border px-3 py-1.5 transition ${
                    isActive
                      ? 'border-primary/50 bg-primary/15 text-white font-semibold shadow-[0_0_8px_rgba(230,126,34,0.15)]'
                      : 'border-transparent text-white/60 hover:border-white/10 hover:bg-white/5 hover:text-white'
                  }`}
                >
                  {item.label}
                </Link>
              )
            })}
          </div>

          <AccountEntry />
        </nav>

        <main className="flex-1 mt-16 p-6">
          <Component {...pageProps} />
        </main>
      </div>
    </>
  )
}

function AccountEntry() {
  const { isLoggedIn, user, openAuthModal, logout } = useAuth()
  const [menuOpen, setMenuOpen] = useState(false)
  const menuRef = useRef(null)

  useEffect(() => {
    if (!menuOpen) return

    const onClickOutside = (event) => {
      if (!menuRef.current?.contains(event.target)) {
        setMenuOpen(false)
      }
    }

    window.addEventListener('mousedown', onClickOutside)
    return () => window.removeEventListener('mousedown', onClickOutside)
  }, [menuOpen])

  if (!isLoggedIn) {
    return (
      <div className="flex items-center gap-2 shrink-0">
        <button
          type="button"
          onClick={() => openAuthModal('login')}
          className="rounded-lg border border-white/20 px-3 py-1.5 text-sm text-white/85 transition hover:border-white/35 hover:bg-white/10 hover:text-white"
        >
          登录
        </button>
        <button
          type="button"
          onClick={() => openAuthModal('register')}
          className="rounded-lg bg-primary px-3 py-1.5 text-sm font-semibold text-black transition hover:opacity-90"
        >
          注册
        </button>
      </div>
    )
  }

  const accountLabel = user?.nickname?.trim() || user?.email || '账号'

  return (
    <div ref={menuRef} className="relative shrink-0">
      <button
        type="button"
        onClick={() => setMenuOpen((prev) => !prev)}
        className="inline-flex items-center gap-2 rounded-lg border border-white/20 px-3 py-1.5 text-sm text-white/85 transition hover:border-white/35 hover:bg-white/10 hover:text-white"
      >
        <span className="max-w-[180px] truncate">{accountLabel}</span>
        <span className="text-xs text-white/55">▼</span>
      </button>

      {menuOpen ? (
        <div className="absolute right-0 mt-2 w-64 rounded-xl border border-white/10 bg-slate-950 p-2 shadow-2xl">
          <div className="rounded-lg border border-white/10 bg-black/20 px-3 py-2">
            <div className="text-xs text-white/45">当前账号</div>
            <div className="mt-1 truncate text-sm text-white/90">{user?.email || '--'}</div>
          </div>

          <button
            type="button"
            onClick={async () => {
              setMenuOpen(false)
              await logout()
            }}
            className="mt-2 w-full rounded-lg border border-rose-400/35 bg-rose-500/10 px-3 py-2 text-left text-sm text-rose-200 transition hover:bg-rose-500/20"
          >
            退出登录
          </button>
        </div>
      ) : null}
    </div>
  )
}

export default function MyApp({ Component, pageProps, router }) {
  // /admin 使用独立布局，不显示主站导航
  if (router.pathname === '/admin') {
    return <Component {...pageProps} />
  }

  return (
    <AuthProvider>
      <AppLayout Component={Component} pageProps={pageProps} />
    </AuthProvider>
  )
}
