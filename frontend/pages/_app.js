import '../styles/globals.css'
import Head from 'next/head'
import Link from 'next/link'
import { useRouter } from 'next/router'
import { useEffect, useRef, useState } from 'react'

import DesktopNavMenu from '../components/DesktopNavMenu'
import MobileNavMenu from '../components/MobileNavMenu'
import NavSearchBox from '../components/NavSearchBox'
import ThemeToggle from '../components/ThemeToggle'
import { AuthProvider, useAuth } from '../lib/auth-context'
import changelogData from '../data/changelog.json'
import { buildPageViewHeaders } from '../lib/pageview'
import { ThemeProvider } from '../lib/theme-context'

const DEBOUNCE_MS = 300
const MIN_QUERY_LEN = 2
const MAX_RESULTS = 8

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
  }, [lastUpdated])

  useEffect(() => {
    const onStorage = (event) => {
      if (event.key !== CL_KEY) return

      const seenAt = event.newValue || ''
      if (!lastUpdated || seenAt >= lastUpdated) {
        setUnreadCount(0)
        return
      }

      const items = Array.isArray(changelogData?.items) ? changelogData.items : []
      setUnreadCount(items.filter((it) => it?.visible !== false && (it?.date || '') > seenAt).length)
    }

    window.addEventListener('storage', onStorage)
    return () => window.removeEventListener('storage', onStorage)
  }, [lastUpdated])

  const markAsSeen = () => {
    const value = lastUpdated
    localStorage.setItem(CL_KEY, value)
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

function captureUtmParams() {
  if (typeof window === 'undefined') return

  const utmKey = 'wolong_utm'
  if (localStorage.getItem(utmKey)) return

  const params = new URLSearchParams(window.location.search)
  const utm = {}
  for (const key of ['utm_source', 'utm_medium', 'utm_campaign']) {
    const value = (params.get(key) || '').trim()
    if (value) utm[key] = value
  }

  if (document.referrer) utm.referrer = document.referrer

  if (Object.keys(utm).length > 0) {
    localStorage.setItem(utmKey, JSON.stringify(utm))
  }
}

function reportPageView(path) {
  if (typeof window === 'undefined') return

  captureUtmParams()
  fetch('/api/analytics/pageview', {
    method: 'POST',
    headers: buildPageViewHeaders(),
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
  const currentPath = router.asPath
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false)
  const { unreadCount, markAsSeen } = useChangelogUnread()

  useEffect(() => {
    reportPageView(router.pathname)
    const onRouteChange = (url) => reportPageView(url)
    router.events.on('routeChangeComplete', onRouteChange)
    return () => router.events.off('routeChangeComplete', onRouteChange)
  }, [router])

  useEffect(() => {
    setMobileMenuOpen(false)
  }, [router.asPath])

  useEffect(() => {
    if (router.pathname === '/changelog' && unreadCount > 0) {
      markAsSeen()
    }
  }, [router.pathname, unreadCount, markAsSeen])

  useEffect(() => {
    if (!mobileMenuOpen) return undefined

    const previousBodyOverflow = document.body.style.overflow
    const previousHtmlOverflow = document.documentElement.style.overflow

    document.body.style.overflow = 'hidden'
    document.documentElement.style.overflow = 'hidden'

    return () => {
      document.body.style.overflow = previousBodyOverflow
      document.documentElement.style.overflow = previousHtmlOverflow
    }
  }, [mobileMenuOpen])

  return (
    <>
      <Head>
        <title>卧龙AI量化交易台</title>
        <meta name="description" content="卧龙AI量化交易台（Wolong Pro）— 面向个人投资者的 AI 量化分析平台。支持 A 股+港股双市场，提供 AI 个股诊断、策略回测、四象限风险全景、信号推送等一站式投研工具。" />
        <meta name="keywords" content="卧龙AI,量化交易,A股分析,港股分析,股票策略,策略回测,选股器,AI选股,四象限分析,技术指标,量化平台,Wolong Pro" />
        <meta property="og:type" content="website" />
        <meta property="og:site_name" content="卧龙AI量化交易台" />
        <meta property="og:title" content="卧龙AI量化交易台 — AI驱动的量化分析平台" />
        <meta property="og:description" content="面向个人投资者的 AI 量化分析平台，支持 A 股+港股双市场，AI 个股诊断、策略回测、信号推送一站式投研工具。" />
        <meta property="og:image" content="https://wolongtrader.top/logo.png" />
        <meta property="og:url" content="https://wolongtrader.top" />
        <meta property="og:locale" content="zh_CN" />
        <meta name="twitter:card" content="summary_large_image" />
        <meta name="twitter:title" content="卧龙AI量化交易台 — AI驱动的量化分析平台" />
        <meta name="twitter:description" content="面向个人投资者的 AI 量化分析平台，支持 A 股+港股双市场，AI 个股诊断、策略回测、信号推送一站式投研工具。" />
        <meta name="twitter:image" content="https://wolongtrader.top/logo.png" />
        <link rel="canonical" href="https://wolongtrader.top" />
        <meta name="viewport" content="width=device-width, initial-scale=1" />
        <link rel="icon" type="image/png" href="/favicon.png" />
        <link rel="apple-touch-icon" href="/apple-touch-icon.png" />
        <script
          dangerouslySetInnerHTML={{
            __html: `
          var _hmt = _hmt || [];
          (function() {
            var hm = document.createElement("script");
            hm.src = "https://hm.baidu.com/hm.js?2dbedd62cd5d38840b696b0dfb4ea2d1";
            var s = document.getElementsByTagName("script")[0];
            s.parentNode.insertBefore(hm, s);
          })();
        `,
          }}
        />
      </Head>
      <div className="min-h-screen bg-background text-foreground flex flex-col">
        <nav className="fixed top-0 left-0 right-0 bg-[var(--color-bg-overlay)] backdrop-blur-md border-b border-border z-50">
          <div className="h-16 flex items-center justify-between px-4 md:px-6 gap-3">
            <Link href="/" className="flex items-center space-x-2 min-w-0 shrink-0">
              <img src="/logo.png" alt="卧龙" width={36} height={36} className="rounded shrink-0" />
              <span className="text-lg font-bold tracking-tight truncate hidden sm:inline">卧龙AI量化交易台</span>
              <span className="text-lg font-bold tracking-tight sm:hidden">卧龙</span>
            </Link>

            <DesktopNavMenu currentPath={currentPath} unreadCount={unreadCount} />

            <div className="flex items-center gap-2 shrink-0">
              <div className="hidden md:block">
                <NavSearchBox />
              </div>
              <ThemeToggle />
              <button
                type="button"
                onClick={() => setMobileMenuOpen((value) => !value)}
                className="md:hidden inline-flex items-center justify-center w-9 h-9 rounded-lg border border-border text-foreground-muted transition hover:bg-[var(--color-bg-hover)] hover:text-foreground"
                aria-label="菜单"
                aria-expanded={mobileMenuOpen}
              >
                {mobileMenuOpen ? (
                  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round">
                    <path d="M18 6L6 18M6 6l12 12" />
                  </svg>
                ) : (
                  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round">
                    <path d="M3 12h18M3 6h18M3 18h18" />
                  </svg>
                )}
              </button>
              <MobileSearchButton />
              <AccountEntry />
            </div>
          </div>
        </nav>

        <MobileNavMenu open={mobileMenuOpen} currentPath={currentPath} unreadCount={unreadCount} onClose={() => setMobileMenuOpen(false)} />

        <main className="flex-1 mt-16 p-4 md:p-6">
          <Component {...pageProps} />
        </main>

        <footer className="border-t border-border py-6 px-6 text-center text-xs text-foreground-dim space-y-2">
          <div className="flex items-center justify-center gap-3">
            <Link href="/privacy" className="hover:text-foreground-muted transition">隐私政策</Link>
            <span>·</span>
            <Link href="/terms" className="hover:text-foreground-muted transition">用户协议</Link>
            <span>·</span>
            <Link href="/disclaimer" className="hover:text-foreground-muted transition">免责声明</Link>
          </div>
          <div className="flex items-center justify-center gap-3">
            <a href="mailto:easystudio@outlook.com" className="hover:text-foreground-muted transition">📧 easystudio@outlook.com</a>
            <span>·</span>
            <a href="https://weibo.com/u/5613355795" target="_blank" rel="noopener noreferrer" className="hover:text-foreground-muted transition">微博</a>
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

  useEffect(() => {
    if (!menuOpen) return undefined

    const onClickOutside = (event) => {
      if (!menuRef.current?.contains(event.target)) {
        setMenuOpen(false)
      }
    }

    window.addEventListener('mousedown', onClickOutside)
    return () => window.removeEventListener('mousedown', onClickOutside)
  }, [menuOpen])

  if (!ready) {
    return (
      <div className="flex items-center gap-2 shrink-0">
        <span className="text-sm text-foreground-dim">···</span>
      </div>
    )
  }

  if (!isLoggedIn) {
    return (
      <div className="flex items-center gap-2 shrink-0">
        <button
          type="button"
          onClick={() => openAuthModal('login')}
          className="rounded-lg border border-border px-3 py-1.5 text-sm text-foreground-muted transition hover:border-border-strong hover:bg-[var(--color-bg-hover)] hover:text-foreground"
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
        className="inline-flex items-center gap-2 rounded-lg border border-border px-3 py-1.5 text-sm text-foreground-muted transition hover:border-border-strong hover:bg-[var(--color-bg-hover)] hover:text-foreground"
      >
        <span className="max-w-[180px] truncate">{accountLabel}</span>
        <span className="text-xs text-foreground-dim">▼</span>
      </button>

      {menuOpen ? (
        <div className="absolute right-0 mt-2 w-64 rounded-xl border border-border bg-card p-2 shadow-2xl">
          <div className="rounded-lg border border-border bg-[var(--color-bg-hover)] px-3 py-2">
            <div className="text-xs text-foreground-dim">当前账号</div>
            <div className="mt-1 truncate text-sm text-foreground">{user?.email || '--'}</div>
          </div>

          <Link
            href="/settings"
            onClick={() => setMenuOpen(false)}
            className="mt-2 block w-full rounded-lg border border-border bg-[var(--color-bg-hover)] px-3 py-2 text-left text-sm text-foreground-muted transition hover:bg-[var(--color-bg-hover)] hover:text-foreground"
          >
            ⚙️ 设置
          </Link>

          <button
            type="button"
            onClick={async () => {
              setMenuOpen(false)
              await logout()
            }}
            className="mt-2 w-full rounded-lg border border-negative/35 bg-negative/10 px-3 py-2 text-left text-sm text-negative transition hover:bg-negative/20"
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
        className="md:hidden inline-flex items-center justify-center w-9 h-9 rounded-lg border border-border text-foreground-muted transition hover:bg-[var(--color-bg-hover)] hover:text-foreground"
        aria-label="搜索股票"
      >
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
          <circle cx="11" cy="11" r="8" />
          <path d="M21 21l-4.35-4.35" />
        </svg>
      </button>

      {open ? (
        <div className="fixed inset-0 z-[60] bg-[var(--color-bg-overlay)] backdrop-blur-sm md:hidden flex items-start pt-20 px-4">
          <div className="w-full max-w-lg mx-auto relative">
            <div className="flex items-center bg-[var(--color-bg-hover)] border border-primary/30 rounded-xl px-4 py-3 focus-within:border-primary/60 transition">
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" className="shrink-0 text-foreground-dim mr-3">
                <circle cx="11" cy="11" r="8" />
                <path d="M21 21l-4.35-4.35" />
              </svg>
              <MobileSearchInner
                onSelect={(code) => {
                  setOpen(false)
                  window.open(`/live-trading/${code}`, '_blank')
                }}
              />
            </div>
            <button
              type="button"
              onClick={() => setOpen(false)}
              className="absolute -top-12 right-0 text-foreground-dim hover:text-foreground text-sm flex items-center gap-1"
            >
              取消
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                <path d="M18 6L6 18M6 6l12 12" />
              </svg>
            </button>
          </div>
        </div>
      ) : null}
    </>
  )
}

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

    if (query.length < MIN_QUERY_LEN) {
      setResults([])
      return undefined
    }

    debounceRef.current = setTimeout(async () => {
      setIsLoading(true)
      try {
        const response = await fetch(`/api/search?q=${encodeURIComponent(query)}&limit=${MAX_RESULTS}`)
        const data = await response.json()
        setResults(data.results || [])
      } catch {
        setResults([])
      } finally {
        setIsLoading(false)
      }
    }, DEBOUNCE_MS)

    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current)
    }
  }, [query])

  return (
    <>
      <input
        ref={inputRef}
        type="text"
        value={query}
        onChange={(event) => setQuery(event.target.value)}
        placeholder="输入代码或名称搜索股票..."
        className="flex-1 bg-transparent text-base text-foreground placeholder-foreground-disabled outline-none min-w-0"
      />
      {results.length > 0 ? (
        <ul className="absolute top-full left-0 right-0 mt-2 bg-card border border-border rounded-xl shadow-2xl z-10 max-h-[60vh] overflow-y-auto">
          {results.map((item) => (
            <li key={item.code}>
              <button
                type="button"
                onClick={() => onSelect(item.code)}
                className="w-full flex items-center justify-between px-4 py-3 text-left text-sm text-foreground-muted hover:bg-primary/15 transition border-b border-border last:border-b-0"
              >
                <span>
                  <span className="font-mono font-semibold text-primary">{item.code}</span>
                  <span className="ml-2 text-foreground-dim">{item.name}</span>
                  {item.exchange === 'HKEX' ? (
                    <span className="ml-1.5 inline-flex items-center px-1 rounded text-[10px] font-medium bg-blue-500/20 text-blue-300">HK</span>
                  ) : null}
                </span>
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" className="shrink-0 opacity-30">
                  <path d="M7 17L17 7M7 7h10v10" />
                </svg>
              </button>
            </li>
          ))}
        </ul>
      ) : null}
      {!isLoading && query.length >= MIN_QUERY_LEN && results.length === 0 ? (
        <div className="mt-2 text-center text-sm text-foreground-dim py-3">未找到匹配股票</div>
      ) : null}
    </>
  )
}

export default function MyApp({ Component, pageProps, router }) {
  if (router.pathname === '/admin' || router.pathname === '/share/ai-analysis-preview') {
    return <Component {...pageProps} />
  }

  return (
    <ThemeProvider>
      <AuthProvider>
        <AppLayout Component={Component} pageProps={pageProps} />
      </AuthProvider>
    </ThemeProvider>
  )
}
