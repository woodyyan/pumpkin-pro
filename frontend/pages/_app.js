import '../styles/globals.css'
import Head from 'next/head'
import Link from 'next/link'
import { useRouter } from 'next/router'
import { useEffect, useMemo, useRef, useState } from 'react'

import { AuthProvider, useAuth } from '../lib/auth-context'
import changelogData from '../data/changelog.json'
import NavSearchBox from '../components/NavSearchBox'

const NAV_ITEMS = [
  { href: '/', label: '首页' },
  { href: '/strategies', label: '策略库' },
  { href: '/backtest', label: '回测引擎' },
  { href: '/live-trading', label: '行情看板' },
  { href: '/stock-picker', label: '选股器' },
  { href: '/changelog', label: '更新日志', badgeKey: 'changelog' },
]

// ── Changelog 未读检测 ──
const CL_KEY = 'wolong_changelog_seen'

function useChangelogUnread() {
  const [unreadCount, setUnreadCount] = useState(0)

  const lastUpdated = changelogData?.last_updated || ''

  useEffect(() => {
    const seenAt = localStorage.getItem(CL_KEY) || ''
    if (!lastUpdated || seenAt >= lastUpdated) {
      setUnreadCount(0)
      return
    }
    const items = Array.isArray(changelogData?.items) ? changelogData.items : []
    setUnreadCount(items.filter((it) => it?.visible !== false && (it?.date || '') > (seenAt || '')).length)
  }, [])

  // Cross-tab sync
  useEffect(() => {
    const onStorage = (e) => {
      if (e.key === CL_KEY) {
        const seenAt = e.newValue || ''
        if (!lastUpdated || seenAt >= lastUpdated) { setUnreadCount(0); return }
        const items = Array.isArray(changelogData?.items) ? changelogData.items : []
        setUnreadCount(items.filter((it) => it?.visible !== false && (it?.date || '') > seenAt).length)
      }
    }
    window.addEventListener('storage', onStorage)
    return () => window.removeEventListener('storage', onStorage)
  }, [lastUpdated])

  const markAsSeen = () => {
    const val = lastUpdated
    localStorage.setItem(CL_KEY, val)
    setUnreadCount(0)
  }

  return { unreadCount, markAsSeen }
}

function getVisitorId() {
  if (typeof window === 'undefined') return ''
  const key = 'wolong_visitor_id'
  let id = localStorage.getItem(key)
  if (!id) {
    id = 'v_' + Math.random().toString(36).slice(2) + Date.now().toString(36)
    localStorage.setItem(key, id)
  }
  return id
}

// Capture UTM parameters from URL on first visit and persist them
function captureUtmParams() {
  if (typeof window === 'undefined') return
  const UTM_KEY = 'wolong_utm'
  if (localStorage.getItem(UTM_KEY)) return // already captured
  const params = new URLSearchParams(window.location.search)
  const utm = {}
  for (const key of ['utm_source', 'utm_medium', 'utm_campaign']) {
    const val = (params.get(key) || '').trim()
    if (val) utm[key] = val
  }
  // Also capture raw referrer from the very first page load
  if (document.referrer) utm.referrer = document.referrer
  if (Object.keys(utm).length > 0) {
    localStorage.setItem(UTM_KEY, JSON.stringify(utm))
  }
}

function getStoredUtm() {
  if (typeof window === 'undefined') return {}
  try { return JSON.parse(localStorage.getItem('wolong_utm') || '{}') } catch { return {} }
}

function reportPageView(path) {
  if (typeof window === 'undefined') return
  captureUtmParams()
  fetch('/api/analytics/pageview', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      page_path: path,
      visitor_id: getVisitorId(),
      screen_width: window.innerWidth,
      referrer: document.referrer || '',
    }),
  }).catch(() => {})
}

function AppLayout({ Component, pageProps }) {
  const router = useRouter()
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false)
  const mobileMenuRef = useRef(null)
  const { unreadCount, markAsSeen } = useChangelogUnread()

  // Page view tracking
  useEffect(() => {
    reportPageView(router.pathname)
    const onRouteChange = (url) => reportPageView(url)
    router.events.on('routeChangeComplete', onRouteChange)
    return () => router.events.off('routeChangeComplete', onRouteChange)
  }, [router])

  // Close mobile menu on route change
  useEffect(() => {
    setMobileMenuOpen(false)
  }, [router.pathname])

  // Auto-mark changelog as seen when visiting the page
  useEffect(() => {
    if (router.pathname === '/changelog' && unreadCount > 0) {
      markAsSeen()
    }
  }, [router.pathname, unreadCount, markAsSeen])

  // Close mobile menu on outside click
  useEffect(() => {
    if (!mobileMenuOpen) return
    const handler = (e) => {
      if (mobileMenuRef.current && !mobileMenuRef.current.contains(e.target)) {
        setMobileMenuOpen(false)
      }
    }
    window.addEventListener('mousedown', handler)
    return () => window.removeEventListener('mousedown', handler)
  }, [mobileMenuOpen])

  return (
    <>
      <Head>
        <title>卧龙AI量化交易台</title>
        <meta name="viewport" content="width=device-width, initial-scale=1" />
        <link rel="icon" type="image/png" href="/favicon.png" />
        <link rel="apple-touch-icon" href="/apple-touch-icon.png" />
        {/* 百度统计 */}
        <script dangerouslySetInnerHTML={{ __html: `
          var _hmt = _hmt || [];
          (function() {
            var hm = document.createElement("script");
            hm.src = "https://hm.baidu.com/hm.js?2dbedd62cd5d38840b696b0dfb4ea2d1";
            var s = document.getElementsByTagName("script")[0];
            s.parentNode.insertBefore(hm, s);
          })();
        `}} />
      </Head>
      <div className="min-h-screen bg-background text-foreground flex flex-col">
        <nav ref={mobileMenuRef} className="fixed top-0 left-0 right-0 bg-black/60 backdrop-blur-md border-b border-white/10 z-50">
          {/* ── Top bar ── */}
          <div className="h-16 flex items-center justify-between px-4 md:px-6 gap-3">
            {/* Logo + title */}
            <Link href="/" className="flex items-center space-x-2 min-w-0 shrink-0">
              <img src="/logo.png" alt="卧龙" width={36} height={36} className="rounded shrink-0" />
              <span className="text-lg font-bold tracking-tight truncate hidden sm:inline">卧龙AI量化交易台</span>
              <span className="text-lg font-bold tracking-tight sm:hidden">卧龙</span>
            </Link>

            {/* Desktop nav items */}
            <div className="hidden md:flex items-center space-x-2 text-base font-medium">
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
                    {item.badgeKey === 'changelog' && unreadCount > 0 && (
                      <span className="ml-1.5 inline-flex items-center justify-center min-w-[18px] h-[18px] px-1 rounded-full bg-rose-500 text-[10px] font-bold text-white leading-none">
                        {unreadCount > 99 ? '99+' : unreadCount}
                      </span>
                    )}
                  </Link>
                )
              })}
            </div>

            {/* Right side: search + hamburger (mobile) + account */}
            <div className="flex items-center gap-2 shrink-0">
              {/* Search — desktop only */}
              <div className="hidden md:block">
                <NavSearchBox />
              </div>
              {/* Hamburger button — mobile only */}
              <button
                type="button"
                onClick={() => setMobileMenuOpen((v) => !v)}
                className="md:hidden inline-flex items-center justify-center w-9 h-9 rounded-lg border border-white/15 text-white/70 transition hover:bg-white/10 hover:text-white"
                aria-label="菜单"
              >
                {mobileMenuOpen ? (
                  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><path d="M18 6L6 18M6 6l12 12" /></svg>
                ) : (
                  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><path d="M3 12h18M3 6h18M3 18h18" /></svg>
                )}
              </button>
              {/* Mobile search button */}
              <MobileSearchButton />
              <AccountEntry />
            </div>
          </div>

          {/* ── Mobile dropdown menu ── */}
          {mobileMenuOpen && (
            <div className="md:hidden border-t border-white/10 bg-black/80 backdrop-blur-md px-4 py-3 space-y-1">
              {NAV_ITEMS.map((item) => {
                const isActive = router.pathname === item.href
                return (
                  <Link
                    key={item.href}
                    href={item.href}
                    className={`block rounded-lg px-3 py-2.5 text-sm font-medium transition ${
                      isActive
                        ? 'bg-primary/15 text-white border-l-2 border-primary'
                        : 'text-white/60 hover:bg-white/5 hover:text-white'
                    }`}
                  >
                    <span className="inline-flex items-center">
                      {item.label}
                      {item.badgeKey === 'changelog' && unreadCount > 0 && (
                        <span className="ml-2 inline-flex items-center justify-center min-w-[18px] h-[18px] px-1 rounded-full bg-rose-500 text-[10px] font-bold text-white leading-none">
                          {unreadCount > 99 ? '99+' : unreadCount}
                        </span>
                      )}
                    </span>
                  </Link>
                )
              })}
            </div>
          )}
        </nav>

        <main className="flex-1 mt-16 p-4 md:p-6">
          <Component {...pageProps} />
        </main>

        <footer className="border-t border-white/8 py-6 px-6 text-center text-xs text-white/30 space-y-2">
          <div className="flex items-center justify-center gap-3">
            <Link href="/privacy" className="hover:text-white/60 transition">隐私政策</Link>
            <span>·</span>
            <Link href="/terms" className="hover:text-white/60 transition">用户协议</Link>
            <span>·</span>
            <Link href="/disclaimer" className="hover:text-white/60 transition">免责声明</Link>
          </div>
          <div className="flex items-center justify-center gap-3">
            <a href="mailto:easystudio@outlook.com" className="hover:text-white/60 transition">📧 easystudio@outlook.com</a>
            <span>·</span>
            <a href="https://weibo.com/u/5613355795" target="_blank" rel="noopener noreferrer" className="hover:text-white/60 transition">微博</a>
          </div>
          <p>© {new Date().getFullYear()} Easy Studio Inc. All rights reserved.</p>
        </footer>
      </div>
    </>
  )
}

function AccountEntry() {
  const { isLoggedIn, user, openAuthModal, logout, ready } = useAuth()
  const [menuOpen, setMenuOpen] = useState(false)
  const menuRef = useRef(null)

  // ⚠️ Hooks must be called unconditionally before any early returns (Rules of Hooks)
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

  // Don't expose login state until initial fetchMe resolves.
  // Prevents flash of "logged-in name → login button" during page load.
  if (!ready) {
    return (
      <div className="flex items-center gap-2 shrink-0">
        <span className="text-sm text-white/30">···</span>
      </div>
    )
  }

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

          <Link
            href="/settings"
            onClick={() => setMenuOpen(false)}
            className="mt-2 block w-full rounded-lg border border-white/10 bg-white/5 px-3 py-2 text-left text-sm text-white/75 transition hover:bg-white/10 hover:text-white"
          >
            ⚙️ 设置
          </Link>

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

function MobileSearchButton() {
  const [open, setOpen] = useState(false)

  return (
    <>
      <button
        type="button"
        onClick={() => setOpen(true)}
        className="md:hidden inline-flex items-center justify-center w-9 h-9 rounded-lg border border-white/15 text-white/70 transition hover:bg-white/10 hover:text-white"
        aria-label="搜索股票"
      >
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><circle cx="11" cy="11" r="8" /><path d="M21 21l-4.35-4.35" /></svg>
      </button>

      {/* Full-screen overlay */}
      {open && (
        <div className="fixed inset-0 z-[60] bg-black/90 backdrop-blur-sm md:hidden flex items-start pt-20 px-4">
          <div className="w-full max-w-lg mx-auto relative">
            <div className="flex items-center bg-white/10 border border-primary/30 rounded-xl px-4 py-3 focus-within:border-primary/60 transition">
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" className="shrink-0 text-white/40 mr-3">
                <circle cx="11" cy="11" r="8" /><path d="M21 21l-4.35-4.35" />
              </svg>
              <MobileSearchInner onSelect={(code) => { setOpen(false); window.open(`/live-trading/${code}`, '_blank') }} />
            </div>
            <button
              type="button"
              onClick={() => setOpen(false)}
              className="absolute -top-12 right-0 text-white/50 hover:text-white text-sm flex items-center gap-1"
            >
              取消
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M18 6L6 18M6 6l12 12" /></svg>
            </button>
          </div>
        </div>
      )}
    </>
  )
}

// Extracted inner search logic for reuse
function MobileSearchInner({ onSelect }) {
  const [query, setQuery] = useState('')
  const [results, setResults] = useState([])
  const [isLoading, setIsLoading] = useState(false)
  const debounceRef = useRef(null)
  const inputRef = useRef(null)

  useEffect(() => {
    if (inputRef.current) inputRef.current.focus()
  }, [])

  useEffect(() => {
    if (debounceRef.current) clearTimeout(debounceRef.current)
    if (query.length < 2) { setResults([]); return }
    debounceRef.current = setTimeout(async () => {
      setIsLoading(true)
      try {
        const res = await fetch(`/api/search?q=${encodeURIComponent(query)}&limit=${MAX_RESULTS}`)
        const data = await res.json()
        setResults(data.results || [])
      } catch { setResults([]) }
      finally { setIsLoading(false) }
    }, DEBOUNCE_MS)
    return () => { if (debounceRef.current) clearTimeout(debounceRef.current) }
  }, [query])

  return (
    <>
      <input
        ref={inputRef}
        type="text"
        value={query}
        onChange={e => setQuery(e.target.value)}
        placeholder="输入代码或名称搜索股票..."
        className="flex-1 bg-transparent text-base text-white placeholder-white/30 outline-none min-w-0"
      />
      {results.length > 0 && (
        <ul className="absolute top-full left-0 right-0 mt-2 bg-slate-900/95 border border-white/10 rounded-xl shadow-2xl z-10 max-h-[60vh] overflow-y-auto">
          {results.map(item => (
            <li key={item.code}>
              <button
                type="button"
                onClick={() => onSelect(item.code)}
                className="w-full flex items-center justify-between px-4 py-3 text-left text-sm text-white/80 hover:bg-primary/15 transition border-b border-white/5 last:border-b-0"
              >
                <span>
                  <span className="font-mono font-semibold text-primary">{item.code}</span>
                  <span className="ml-2 text-white/50">{item.name}</span>
                  {item.exchange === 'HKEX' && <span className="ml-1.5 inline-flex items-center px-1 rounded text-[10px] font-medium bg-blue-500/20 text-blue-300">HK</span>}
                </span>
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" className="shrink-0 opacity-30"><path d="M7 17L17 7M7 7h10v10" /></svg>
              </button>
            </li>
          ))}
        </ul>
      )}
      {!isLoading && query.length >= 2 && results.length === 0 && (
        <div className="mt-2 text-center text-sm text-white/30 py-3">未找到匹配股票</div>
      )}
    </>
  )
}

const DEBOUNCE_MS = 300
const MIN_QUERY_LEN = 2
const MAX_RESULTS = 8

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
