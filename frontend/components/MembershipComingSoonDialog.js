import { useEffect, useRef } from 'react'
import Link from 'next/link'

import {
  MEMBERSHIP_FEEDBACK_PATH,
  MEMBERSHIP_LAUNCH_NOTE,
  MEMBERSHIP_PLANS,
} from '../lib/membership'

/**
 * 开通会员占位弹层（预发布）。
 *
 * 本期不发起真实支付：展示价格与核心权益，引导用户到设置页「反馈与建议」
 * 留下意见，无额外后端依赖。
 */
export default function MembershipComingSoonDialog({ open, onClose }) {
  const dialogRef = useRef(null)

  useEffect(() => {
    if (!open) return undefined

    const onKeyDown = (event) => {
      if (event.key === 'Escape') onClose?.()
    }

    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [open, onClose])

  if (!open) return null

  return (
    <div className="fixed inset-0 z-[70] flex items-center justify-center px-4">
      <button
        type="button"
        aria-label="关闭"
        onClick={onClose}
        className="absolute inset-0 z-0 bg-black/50"
      />

      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-label="会员即将上线"
        className="relative z-10 w-full max-w-md rounded-2xl border border-border bg-card p-6 shadow-2xl"
      >
        <div className="inline-flex rounded-full border border-primary/25 bg-primary/10 px-3 py-1 text-xs tracking-[0.2em] text-primary">
          即将上线
        </div>

        <h2 className="mt-3 text-xl font-bold text-foreground">会员体系即将上线，敬请期待</h2>

        <p className="mt-2 text-sm leading-relaxed text-foreground-muted">
          正式收费尚未开启。您现在可以抢先了解会员价格与权益，并告诉我们您最看重的能力。
        </p>

        <ul className="mt-4 space-y-2 text-sm text-foreground-muted">
          {MEMBERSHIP_PLANS.map((plan) => (
            <li key={plan.key} className="flex items-center justify-between rounded-lg border border-border bg-[var(--color-bg-hover)] px-3 py-2">
              <span>{plan.name}</span>
              <span className="font-semibold text-primary">{plan.priceLabel}</span>
            </li>
          ))}
          <li className="rounded-lg border border-primary/25 bg-primary/10 px-3 py-2 text-primary">
            每月 5 份 AI 研报额度
          </li>
        </ul>

        <p className="mt-3 text-xs text-foreground-dim">{MEMBERSHIP_LAUNCH_NOTE}</p>

        <div className="mt-5 flex flex-col gap-2 sm:flex-row">
          <Link
            href={MEMBERSHIP_FEEDBACK_PATH}
            onClick={onClose}
            className="inline-flex flex-1 items-center justify-center rounded-lg bg-primary px-4 py-2 text-sm font-semibold text-black transition hover:opacity-90"
          >
            反馈我的建议
          </Link>
          <button
            type="button"
            onClick={onClose}
            className="inline-flex flex-1 items-center justify-center rounded-lg border border-border px-4 py-2 text-sm text-foreground-muted transition hover:bg-[var(--color-bg-hover)] hover:text-foreground"
          >
            我知道了
          </button>
        </div>
      </div>
    </div>
  )
}
