import { useEffect, useState } from 'react'
import { requestJson } from '../lib/api'

/**
 * CommunityQRCard
 *
 * Fetches the community QR config from the public API and renders a
 * card with the QR code image + title + description.
 *
 * Renders nothing when the config is disabled or not yet configured,
 * or when the API request fails (silent degradation).
 */
export default function CommunityQRCard() {
  const [config, setConfig] = useState(null)
  const [loaded, setLoaded] = useState(false)

  useEffect(() => {
    let cancelled = false
    requestJson('/api/site-config/community')
      .then((data) => {
        if (cancelled) return
        setConfig(data)
      })
      .catch(() => {
        // Silent degradation — do not break the page.
      })
      .finally(() => {
        if (!cancelled) setLoaded(true)
      })
    return () => { cancelled = true }
  }, [])

  // Don't render anything until loaded (avoid flash of nothing → something).
  if (!loaded) return null

  // Don't render if disabled or missing QR image.
  if (!config || !config.is_enabled || !config.qr_image_base64) return null

  const title = config.title || '卧龙AI量化交流群'

  return (
    <section className="mx-auto max-w-5xl px-4 py-10 md:py-14">
      <div className="flex flex-col items-center gap-6 rounded-2xl border border-border bg-card p-6 md:flex-row md:gap-8 md:p-8">
        <img
          src={config.qr_image_base64}
          alt={title}
          width={160}
          height={160}
          className="h-[140px] w-[140px] shrink-0 rounded-xl object-contain md:h-[160px] md:w-[160px]"
        />
        <div className="text-center md:text-left">
          <h3 className="text-lg font-semibold tracking-tight text-foreground md:text-xl">{title}</h3>
          {config.description ? (
            <p className="mt-2 text-sm leading-7 text-foreground-muted md:text-base md:leading-7">
              {config.description}
            </p>
          ) : null}
        </div>
      </div>
    </section>
  )
}
