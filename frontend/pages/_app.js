import '../styles/globals.css'
import Head from 'next/head'
import Link from 'next/link'
import { useRouter } from 'next/router'

const NAV_ITEMS = [
  { href: '/strategies', label: '策略库' },
  { href: '/', label: '回测引擎' },
  { href: '/live-trading', label: '实盘交易' },
  { href: '/stock-picker', label: '选股平台' },
  { href: '/settings', label: '设置' }
]

function MyApp({ Component, pageProps }) {
  const router = useRouter()

  return (
    <>
      <Head>
        <title>Pumpkin Trader Pro</title>
        <meta name="viewport" content="width=device-width, initial-scale=1" />
      </Head>
      <div className="min-h-screen bg-background text-foreground flex flex-col">
        <nav className="fixed top-0 left-0 right-0 h-16 bg-black/60 backdrop-blur-md border-b border-white/10 z-50 flex items-center justify-between px-6">
          <div className="flex items-center space-x-2">
            <span className="text-2xl">🎃</span>
            <span className="text-lg font-bold tracking-tight">Pumpkin Pro 南瓜交易系统</span>
          </div>
          <div className="flex items-center space-x-2 text-sm font-medium">
            {NAV_ITEMS.map((item) => {
              const isActive = router.pathname === item.href
              return (
                <Link
                  key={item.href}
                  href={item.href}
                  className={`inline-flex items-center rounded-lg border px-3 py-1.5 transition ${
                    isActive
                      ? 'border-white/25 bg-white/12 text-white shadow-[0_0_0_1px_rgba(255,255,255,0.08)]'
                      : 'border-transparent text-white/70 hover:border-white/10 hover:bg-white/5 hover:text-white'
                  }`}
                >
                  {item.label}
                </Link>
              )
            })}
          </div>
          <div className="w-8"></div>
        </nav>

        <main className="flex-1 mt-16 p-6">
          <Component {...pageProps} />
        </main>
      </div>
    </>
  )
}

export default MyApp
