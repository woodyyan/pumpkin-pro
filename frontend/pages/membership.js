import { useState } from 'react'
import Head from 'next/head'
import Link from 'next/link'

import MembershipComingSoonDialog from '../components/MembershipComingSoonDialog'
import {
  MEMBERSHIP_COMPARE_ROWS,
  MEMBERSHIP_FAQS,
  MEMBERSHIP_FEEDBACK_PATH,
  MEMBERSHIP_FREE_QUOTA_NOTE,
  MEMBERSHIP_LAUNCH_NOTE,
  MEMBERSHIP_PLANS,
} from '../lib/membership'

function PricingCard({ plan, onSubscribe }) {
  return (
    <div
      className={`relative flex flex-col rounded-2xl border p-6 ${
        plan.highlight
          ? 'border-primary/50 bg-primary/10 shadow-[0_16px_50px_rgba(230,126,34,0.15)]'
          : 'border-border bg-[var(--color-bg-hover)]'
      }`}
    >
      {plan.badge ? (
        <span className="absolute -top-3 left-1/2 -translate-x-1/2 whitespace-nowrap rounded-full bg-primary px-3 py-1 text-xs font-bold text-black">
          {plan.badge}
        </span>
      ) : null}

      <h3 className="text-base font-semibold text-foreground">{plan.name}</h3>
      <div className="mt-3 flex items-baseline gap-1">
        <span className="text-3xl font-extrabold text-foreground">¥{plan.price}</span>
        <span className="text-sm text-foreground-dim">/ {plan.unit}</span>
      </div>
      <p className="mt-2 flex-1 text-sm text-foreground-muted">{plan.description}</p>

      <button
        type="button"
        onClick={onSubscribe}
        className={`mt-5 w-full rounded-lg px-4 py-2.5 text-sm font-semibold transition ${
          plan.highlight
            ? 'bg-primary text-black hover:opacity-90'
            : 'border border-border text-foreground-muted hover:bg-[var(--color-bg-overlay)] hover:text-foreground'
        }`}
      >
        开通会员
      </button>
    </div>
  )
}

export default function MembershipPage() {
  const [dialogOpen, setDialogOpen] = useState(false)
  const openDialog = () => setDialogOpen(true)

  return (
    <div className="mx-auto max-w-5xl space-y-10 pb-10">
      <Head>
        <title>会员中心 — 卧龙AI量化交易台</title>
        <meta name="description" content="卧龙AI量化交易台会员体系预发布 — ¥39/月或¥390/年，每月含 5 份 AI 研报额度。抢先了解价格与权益，欢迎反馈建议。" />
        <link rel="canonical" href="https://wolongtrader.top/membership" />
      </Head>

      {/* 首屏：定位 + 价格卡 */}
      <section className="relative overflow-hidden rounded-[28px] border border-border bg-[var(--color-bg-secondary)] dark:bg-[#111114] p-8 shadow-[0_24px_80px_rgba(0,0,0,0.08)] dark:shadow-[0_24px_80px_rgba(0,0,0,0.35)] lg:p-10">
        <div className="absolute inset-0 bg-[radial-gradient(circle_at_top_left,rgba(230,126,34,0.18),transparent_34%),radial-gradient(circle_at_85%_20%,rgba(56,189,248,0.14),transparent_24%)]" />
        <div className="relative">
          <div className="inline-flex rounded-full border border-primary/25 bg-primary/10 px-3 py-1 text-xs tracking-[0.22em] text-primary">
            会员体系 · 预发布
          </div>
          <h1 className="mt-4 text-3xl font-extrabold tracking-tight text-foreground lg:text-4xl">
            卧龙会员
          </h1>
          <p className="mt-3 max-w-2xl text-sm leading-relaxed text-foreground-muted lg:text-base">
            一个档位，全部权益。会员每月拥有 5 份 AI 研报额度，并解锁深度投研、高级选股、完整回测与组合风险管理能力。
          </p>
          <p className="mt-2 text-xs text-foreground-dim">
            本期为预发布展示，暂未开启支付；{MEMBERSHIP_LAUNCH_NOTE}
          </p>

          <div className="mt-8 grid gap-5 sm:grid-cols-2 lg:max-w-2xl">
            {MEMBERSHIP_PLANS.map((plan) => (
              <PricingCard key={plan.key} plan={plan} onSubscribe={openDialog} />
            ))}
          </div>
        </div>
      </section>

      {/* 免费 vs 会员 对比 */}
      <section>
        <h2 className="text-xl font-bold text-foreground">免费版 vs 会员</h2>
        <div className="mt-4 overflow-x-auto rounded-2xl border border-border">
          <table className="w-full min-w-[560px] border-collapse text-sm">
            <thead>
              <tr className="bg-[var(--color-bg-hover)] text-left text-foreground-dim">
                <th className="px-4 py-3 font-medium">功能</th>
                <th className="px-4 py-3 font-medium">免费版</th>
                <th className="px-4 py-3 font-medium text-primary">会员</th>
              </tr>
            </thead>
            <tbody>
              {MEMBERSHIP_COMPARE_ROWS.map((row) => (
                <tr key={row.feature} className="border-t border-border">
                  <td className="px-4 py-3 font-medium text-foreground">{row.feature}</td>
                  <td className="px-4 py-3 text-foreground-muted">{row.free}</td>
                  <td className="px-4 py-3 text-foreground">{row.member}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <p className="mt-2 text-xs text-foreground-dim">{MEMBERSHIP_FREE_QUOTA_NOTE}</p>
      </section>

      {/* FAQ */}
      <section>
        <h2 className="text-xl font-bold text-foreground">常见问题</h2>
        <div className="mt-4 space-y-3">
          {MEMBERSHIP_FAQS.map((faq) => (
            <details key={faq.question} className="group rounded-2xl border border-border bg-[var(--color-bg-hover)] px-5 py-4">
              <summary className="cursor-pointer list-none text-sm font-semibold text-foreground transition group-open:text-primary">
                {faq.question}
              </summary>
              <p className="mt-2 text-sm leading-relaxed text-foreground-muted">{faq.answer}</p>
            </details>
          ))}
        </div>
      </section>

      {/* 反馈 */}
      <section className="rounded-2xl border border-border bg-[var(--color-bg-hover)] p-6 text-center lg:p-8">
        <h2 className="text-lg font-bold text-foreground">您的意见决定会员的最终形态</h2>
        <p className="mx-auto mt-2 max-w-xl text-sm text-foreground-muted">
          本期为预发布，暂未开启支付。欢迎告诉我们您最看重的权益、对价格与规则的建议，正式上线前我们会结合反馈持续优化。
        </p>
        <div className="mt-5 flex flex-col items-center justify-center gap-3 sm:flex-row">
          <button
            type="button"
            onClick={openDialog}
            className="rounded-lg bg-primary px-6 py-2.5 text-sm font-semibold text-black transition hover:opacity-90"
          >
            开通会员
          </button>
          <Link
            href={MEMBERSHIP_FEEDBACK_PATH}
            className="rounded-lg border border-border px-6 py-2.5 text-sm text-foreground-muted transition hover:bg-[var(--color-bg-overlay)] hover:text-foreground"
          >
            反馈建议
          </Link>
        </div>
      </section>

      <MembershipComingSoonDialog open={dialogOpen} onClose={() => setDialogOpen(false)} />
    </div>
  )
}
