import Head from 'next/head'

import NewsKlineDashboard from '../components/NewsKlineDashboard'

export default function NewsKlinePage() {
  return (
    <>
      <Head>
        <title>新闻透视 — 卧龙AI量化交易台</title>
        <meta name="description" content="卧龙AI量化交易台新闻透视，将公告、新闻和财报催化剂映射到前复权 K 线，观察事件发生后的短期股价反应。" />
        <link rel="canonical" href="https://wolongtrader.top/news-kline" />
      </Head>
      <NewsKlineDashboard />
    </>
  )
}
