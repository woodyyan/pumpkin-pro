import Head from 'next/head'

import CapitalMapDashboard from '../components/CapitalMapDashboard'

export default function CapitalMapPage() {
  return (
    <>
      <Head>
        <title>资金星图 — 卧龙AI量化交易台</title>
        <meta name="description" content="卧龙AI量化交易台资金星图，基于 A 股公开行情样本观察 PE、成交额、板块资金流和 PoC 估值锚。" />
        <link rel="canonical" href="https://wolongtrader.top/capital-map" />
      </Head>
      <CapitalMapDashboard />
    </>
  )
}
