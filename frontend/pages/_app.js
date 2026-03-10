import '../styles/globals.css'
import Head from 'next/head'

function MyApp({ Component, pageProps }) {
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
          <div className="flex space-x-6 text-sm font-medium text-white/70">
            <a href="/" className="text-white hover:text-white transition">历史回测</a>
            <a href="#" className="hover:text-white transition">实盘交易</a>
            <a href="#" className="hover:text-white transition">策略中心</a>
            <a href="#" className="hover:text-white transition">系统设置</a>
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
