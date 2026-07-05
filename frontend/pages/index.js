import Link from 'next/link'
import { useState } from 'react'
import Head from 'next/head'

import CommunityQRCard from '../components/CommunityQRCard'
import { useAuth } from '../lib/auth-context'
import {
  CORE_SELLING_POINTS,
  FEATURE_CATEGORIES,
  HOME_HERO,
  QUICK_START_PATHS,
  TUTORIAL_GROUPS,
} from '../data/homepage'

function ArrowIcon() {
  return <span className="ml-1 transition-transform group-hover:translate-x-1">→</span>
}

function Badge({ children, tone = 'default' }) {
  const toneClassName = tone === 'primary'
    ? 'border-primary/25 bg-primary/10 text-primary'
    : 'border-border bg-[var(--color-bg-hover)] text-foreground-dim'

  return (
    <span className={`inline-flex rounded-full border px-2.5 py-1 text-xs font-medium ${toneClassName}`}>
      {children}
    </span>
  )
}

function Accordion({ items }) {
  const [openIdx, setOpenIdx] = useState(-1)
  return (
    <div className="space-y-2">
      {items.map((item, i) => {
        const isOpen = openIdx === i
        return (
          <div key={item.q} className="overflow-hidden rounded-xl border border-border bg-card">
            <button
              type="button"
              onClick={() => setOpenIdx(isOpen ? -1 : i)}
              className="flex w-full items-center justify-between gap-4 px-5 py-4 text-left text-sm font-medium text-foreground transition hover:bg-[var(--color-bg-hover)]"
            >
              <span>{item.q}</span>
              <span className={`text-foreground-dim transition-transform ${isOpen ? 'rotate-90' : ''}`}>▸</span>
            </button>
            <div className={`grid transition-all duration-300 ease-in-out ${isOpen ? 'grid-rows-[1fr]' : 'grid-rows-[0fr]'}`}>
              <div className="overflow-hidden">
                <div className="space-y-3 px-5 pb-5">
                  <ol className="list-decimal space-y-1.5 pl-5 text-sm leading-6 text-foreground-muted">
                    {item.steps.map((step) => <li key={step}>{step}</li>)}
                  </ol>
                  {item.tip ? (
                    <div className="rounded-lg border border-primary/15 bg-primary/10 px-3 py-2 text-xs leading-5 text-primary/90">
                      提示：{item.tip}
                    </div>
                  ) : null}
                  {item.href && item.cta ? (
                    <Link href={item.href} className="group inline-flex items-center text-sm font-medium text-primary transition hover:text-primary/80">
                      {item.cta}<ArrowIcon />
                    </Link>
                  ) : null}
                </div>
              </div>
            </div>
          </div>
        )
      })}
    </div>
  )
}

function CoreSellingPointCard({ item }) {
  return (
    <Link href={item.href} className="group flex h-full flex-col rounded-2xl border border-border bg-card p-6 transition hover:-translate-y-1 hover:border-primary/45 hover:shadow-card">
      <div className="flex items-center justify-between gap-3">
        <span className="text-xs font-medium text-primary">{item.label}</span>
        <Badge tone="primary">{item.status}</Badge>
      </div>
      <h3 className="mt-4 text-xl font-semibold tracking-tight text-foreground">{item.title}</h3>
      <p className="mt-3 flex-1 text-sm leading-7 text-foreground-muted">{item.summary}</p>
      <div className="mt-5 flex flex-wrap gap-2">
        {item.capabilities.map((capability) => <Badge key={capability}>{capability}</Badge>)}
      </div>
      <span className="mt-6 inline-flex items-center text-sm font-medium text-primary">
        {item.cta}<ArrowIcon />
      </span>
    </Link>
  )
}

function QuickStartCard({ item }) {
  return (
    <Link
      href={item.href}
      className={`group relative flex h-full flex-col rounded-2xl border bg-card p-6 transition hover:-translate-y-1 hover:shadow-card ${
        item.recommended ? 'border-primary/45' : 'border-border hover:border-primary/35'
      }`}
    >
      {item.recommended ? <span className="absolute right-4 top-4 rounded-full bg-primary px-2.5 py-1 text-xs font-medium text-black">推荐</span> : null}
      <h3 className="pr-12 text-lg font-semibold text-foreground">{item.title}</h3>
      <p className="mt-2 text-sm leading-6 text-foreground-muted">{item.value}</p>
      <div className="mt-5 space-y-2.5">
        {item.steps.map((step, index) => (
          <div key={step} className="flex items-start gap-2.5">
            <span className="mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-primary/10 text-xs font-medium text-primary">{index + 1}</span>
            <span className="text-sm leading-6 text-foreground-muted">{step}</span>
          </div>
        ))}
      </div>
      <div className="mt-5 rounded-xl border border-border bg-[var(--color-bg-hover)] px-4 py-3">
        <p className="text-xs leading-5 text-foreground-muted">{item.output}</p>
        <p className="mt-1 text-xs leading-5 text-foreground-dim">{item.nextAction}</p>
      </div>
      <span className="mt-5 inline-flex items-center text-sm font-medium text-primary">
        {item.cta}<ArrowIcon />
      </span>
    </Link>
  )
}

function FeatureCategory({ category }) {
  return (
    <section className="rounded-3xl border border-border bg-card p-5 md:p-6">
      <div className="flex flex-col gap-3 md:flex-row md:items-end md:justify-between">
        <div>
          <h3 className="text-xl font-semibold tracking-tight text-foreground">{category.title}</h3>
          <p className="mt-2 text-sm leading-6 text-foreground-muted">{category.description}</p>
        </div>
        <Badge>{category.items.length} 项能力</Badge>
      </div>
      <div className="mt-5 grid gap-3 md:grid-cols-2 xl:grid-cols-4">
        {category.items.map((feature) => (
          <Link key={`${category.key}-${feature.title}`} href={feature.href} className="group rounded-2xl border border-border bg-background px-4 py-4 transition hover:border-primary/35 hover:bg-[var(--color-bg-hover)]">
            <div className="flex items-start justify-between gap-3">
              <h4 className="text-base font-semibold text-foreground">{feature.title}</h4>
              <span className="shrink-0 rounded-full border border-primary/20 bg-primary/10 px-2 py-0.5 text-[11px] font-medium text-primary">{feature.status}</span>
            </div>
            <p className="mt-3 text-sm leading-6 text-foreground-muted">{feature.summary}</p>
            <span className="mt-4 inline-flex items-center text-xs font-medium text-primary">
              进入功能<ArrowIcon />
            </span>
          </Link>
        ))}
      </div>
    </section>
  )
}

export default function HomePage() {
  const { isLoggedIn, openAuthModal } = useAuth()

  const handleCTA = () => {
    if (isLoggedIn) {
      window.location.href = HOME_HERO.primaryHref
    } else {
      openAuthModal('register')
    }
  }

  return (
    <div className="space-y-0">
      <Head>
        <title>卧龙AI量化交易台 — AI投研、因子选股与组合跟踪工作台</title>
        <meta name="description" content="卧龙AI量化交易台（Wolong Pro）面向个人投资者，提供 AI分析、AI研报、AI选股、因子实验室、组合跟踪、持仓管理、市场行情、回测引擎与交易信号能力，覆盖 A 股与中国香港股票投研场景。" />
        <link rel="canonical" href="https://wolongtrader.top/" />
      </Head>

      <section className="relative overflow-hidden px-4 pt-10 pb-16 text-center md:pt-16 md:pb-24">
        <div className="pointer-events-none absolute inset-x-0 top-0 mx-auto h-64 max-w-4xl rounded-full bg-primary/8" />
        <div className="relative mx-auto flex max-w-5xl flex-col items-center">
          <img src="/logo.png" alt="卧龙" width={100} height={100} className="mb-6 rounded-xl md:h-[120px] md:w-[120px]" />
          <Badge tone="primary">{HOME_HERO.eyebrow}</Badge>
          <h1 className="mt-5 text-3xl font-bold tracking-tight text-foreground md:text-5xl">
            {HOME_HERO.title}
          </h1>
          <p className="mt-4 text-base font-medium text-primary md:text-lg">
            {HOME_HERO.subtitle}
          </p>
          <p className="mt-4 max-w-3xl text-sm leading-7 text-foreground-muted md:text-base md:leading-8">
            {HOME_HERO.description}
          </p>

          <div className="mt-8 flex flex-col items-center gap-3 sm:flex-row">
            <button
              type="button"
              onClick={handleCTA}
              className="rounded-xl bg-primary px-7 py-3 text-sm font-semibold text-black shadow-card transition hover:bg-primary/90"
            >
              {HOME_HERO.primaryCta}
            </button>
            <Link href={HOME_HERO.secondaryHref} className="rounded-xl border border-border px-6 py-3 text-sm font-semibold text-foreground-muted transition hover:border-primary hover:text-primary">
              {HOME_HERO.secondaryCta}
            </Link>
            <a href={HOME_HERO.supportHref} className="rounded-xl border border-transparent px-5 py-3 text-sm font-medium text-foreground-dim transition hover:text-foreground">
              {HOME_HERO.supportCta}
            </a>
          </div>

          <div className="mt-8 flex max-w-3xl flex-wrap justify-center gap-2">
            {HOME_HERO.chips.map((chip) => <Badge key={chip}>{chip}</Badge>)}
          </div>
        </div>
      </section>

      <section className="mx-auto max-w-6xl px-4 py-14 md:py-20">
        <div className="mb-10 text-center">
          <h2 className="text-2xl font-bold tracking-tight md:text-3xl">三个核心卖点</h2>
          <p className="mt-3 text-sm text-foreground-dim">从旧的市场覆盖和指标数量，升级为完整的 AI 投研与组合复盘闭环。</p>
        </div>
        <div className="grid grid-cols-1 gap-5 lg:grid-cols-3">
          {CORE_SELLING_POINTS.map((item) => <CoreSellingPointCard key={item.key} item={item} />)}
        </div>
      </section>

      <section className="mx-auto max-w-6xl px-4 py-14 md:py-20">
        <div className="mb-10 text-center">
          <h2 className="text-2xl font-bold tracking-tight md:text-3xl">快速上手</h2>
          <p className="mt-3 text-sm text-foreground-dim">按你当前要完成的投研任务选择入口，3 步开始使用。</p>
        </div>
        <div className="grid grid-cols-1 gap-5 md:grid-cols-2 xl:grid-cols-3">
          {QUICK_START_PATHS.map((item) => <QuickStartCard key={item.key} item={item} />)}
        </div>
      </section>

      <section id="features" className="mx-auto max-w-7xl px-4 py-14 md:py-20">
        <div className="mb-10 text-center">
          <h2 className="text-2xl font-bold tracking-tight md:text-3xl">我们提供什么</h2>
          <p className="mt-3 text-sm text-foreground-dim">按「AI投研、市场机会、选股策略、跟踪组合、账户服务」分类展示全站能力。</p>
        </div>
        <div className="space-y-5">
          {FEATURE_CATEGORIES.map((category) => <FeatureCategory key={category.key} category={category} />)}
        </div>
      </section>

      <CommunityQRCard />

      <section className="mx-auto max-w-5xl px-4 py-14 md:py-20">
        <div className="mb-10 text-center">
          <h2 className="text-2xl font-bold tracking-tight md:text-3xl">使用教程</h2>
          <p className="mt-3 text-sm text-foreground-dim">覆盖 AI分析、AI研报、AI选股、因子实验室、组合跟踪、持仓管理和信号配置。</p>
        </div>
        <div className="space-y-6">
          {TUTORIAL_GROUPS.map((group) => (
            <section key={group.key} className="rounded-3xl border border-border bg-card p-5 md:p-6">
              <h3 className="text-lg font-semibold text-foreground">{group.title}</h3>
              <div className="mt-4">
                <Accordion items={group.items} />
              </div>
            </section>
          ))}
        </div>
      </section>

      <section className="mx-auto max-w-4xl px-4 py-14 text-center md:py-20">
        <div className="mb-10 rounded-2xl border border-negative/25 bg-negative/10 px-6 py-5 text-left">
          <p className="text-sm leading-7 text-foreground-muted">
            <strong className="text-negative">风险提示：</strong>
            本平台提供的数据分析、AI分析、AI研报、AI选股、因子排序、策略回测、模拟组合和交易信号仅用于辅助研究，不构成任何投资建议或收益承诺。股票市场有风险，投资需谨慎。详见
            <Link href="/disclaimer" className="mx-0.5 text-primary underline underline-offset-2 hover:text-primary/80">《免责声明》</Link>
          </p>
        </div>

        <button
          type="button"
          onClick={handleCTA}
          className="rounded-xl bg-primary px-8 py-3.5 text-base font-semibold text-black shadow-card transition hover:bg-primary/90"
        >
          从 AI分析开始
        </button>
      </section>
    </div>
  )
}
