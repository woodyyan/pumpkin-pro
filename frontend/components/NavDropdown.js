import Link from 'next/link'

export default function NavDropdown({ items }) {
  return (
    <div className="absolute left-0 top-full z-20 pt-2">
      <div className="min-w-[176px] rounded-xl border border-border bg-card p-2 shadow-2xl">
        {items.map((item) => (
          <Link
            key={item.key}
            href={item.href}
            className={`flex items-center justify-between rounded-lg px-3 py-2 text-sm transition ${
              item.isActive
                ? 'bg-primary/15 text-foreground'
                : 'text-foreground-muted hover:bg-[var(--color-bg-hover)] hover:text-foreground'
            }`}
          >
            <span>{item.label}</span>
            {item.badge ? (
              <span className="ml-3 inline-flex items-center justify-center min-w-[18px] h-[18px] px-1 rounded-full bg-rose-500 text-[10px] font-bold text-white leading-none">
                {item.badge}
              </span>
            ) : null}
          </Link>
        ))}
      </div>
    </div>
  )
}
