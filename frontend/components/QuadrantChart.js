import { useRef, useEffect, useCallback, useState } from 'react'

/**
 * QuadrantChart — Canvas-based cross quadrant scatter plot.
 *
 * Props:
 *   allStocks:  [{ c, n, o, r, q }]   (compact, full market)
 *   watchlist:  [{ code, name, opportunity, risk, quadrant, ... }]  (detailed, user watchlist)
 *   width / height: canvas dimensions (default 600 x 500)
 *   onClickStock: (code) => void
 */

// Quadrant colours (semi-transparent fills)
const QUADRANT_COLOURS = {
  '机会': { bg: 'rgba(34,197,94,0.08)',  dot: 'rgba(34,197,94,0.18)',  label: '#22c55e' },
  '拥挤': { bg: 'rgba(234,179,8,0.08)',   dot: 'rgba(234,179,8,0.18)',   label: '#eab308' },
  '泡沫': { bg: 'rgba(239,68,68,0.08)',   dot: 'rgba(239,68,68,0.18)',   label: '#ef4444' },
  '防御': { bg: 'rgba(156,163,175,0.06)', dot: 'rgba(156,163,175,0.15)', label: '#9ca3af' },
  '中性': { bg: 'rgba(96,165,250,0.05)',  dot: 'rgba(96,165,250,0.12)',  label: '#60a5fa' },
}

const PADDING = { top: 40, right: 30, bottom: 50, left: 50 }

function mapToCanvas(opportunity, risk, plotW, plotH) {
  // Map 0~100 to plot area. x = opportunity, y = risk (inverted: high risk at top)
  const x = PADDING.left + (opportunity / 100) * plotW
  const y = PADDING.top + (1 - risk / 100) * plotH
  return { x, y }
}

function getQuadrantColor(q) {
  return QUADRANT_COLOURS[q] || QUADRANT_COLOURS['中性']
}

export default function QuadrantChart({
  allStocks = [],
  watchlist = [],
  width: propWidth,
  height: propHeight,
}) {
  const containerRef = useRef(null)
  const canvasRef = useRef(null)
  // Spatial index for hover detection
  const gridRef = useRef(null)
  const hoveredRef = useRef(null)

  // Responsive sizing: fill parent container, fallback to props
  const [size, setSize] = useState({ w: propWidth || 600, h: propHeight || 500 })

  useEffect(() => {
    const el = containerRef.current
    if (!el) return
    const measure = () => {
      const rect = el.getBoundingClientRect()
      const w = Math.floor(rect.width) || propWidth || 600
      // Maintain roughly 4:3 aspect ratio, min height 360, max 700
      const h = propHeight || Math.min(700, Math.max(360, Math.floor(w * 0.65)))
      setSize({ w, h })
    }
    measure()
    const ro = typeof ResizeObserver !== 'undefined' ? new ResizeObserver(measure) : null
    if (ro) ro.observe(el)
    return () => { if (ro) ro.disconnect() }
  }, [propWidth, propHeight])

  const width = size.w
  const height = size.h

  const plotW = width - PADDING.left - PADDING.right
  const plotH = height - PADDING.top - PADDING.bottom

  // Build spatial grid (20x20)
  const buildGrid = useCallback(() => {
    const GRID_SIZE = 20
    const cellW = plotW / GRID_SIZE
    const cellH = plotH / GRID_SIZE
    const grid = Array.from({ length: GRID_SIZE }, () =>
      Array.from({ length: GRID_SIZE }, () => [])
    )

    const allItems = allStocks.map((s) => ({
      code: s.c,
      name: s.n,
      opportunity: s.o,
      risk: s.r,
      quadrant: s.q,
      isWatchlist: false,
    }))

    // Add watchlist items (will be drawn on top)
    const watchlistCodes = new Set(watchlist.map((w) => w.code))
    watchlist.forEach((w) => {
      allItems.push({
        code: w.code,
        name: w.name,
        opportunity: w.opportunity,
        risk: w.risk,
        quadrant: w.quadrant,
        isWatchlist: true,
        detail: w,
      })
    })

    allItems.forEach((item) => {
      const { x, y } = mapToCanvas(item.opportunity, item.risk, plotW, plotH)
      const gx = Math.min(GRID_SIZE - 1, Math.max(0, Math.floor((x - PADDING.left) / cellW)))
      const gy = Math.min(GRID_SIZE - 1, Math.max(0, Math.floor((y - PADDING.top) / cellH)))
      grid[gy][gx].push({ ...item, cx: x, cy: y })
    })

    gridRef.current = { grid, cellW, cellH, GRID_SIZE }
  }, [allStocks, watchlist, plotW, plotH])

  const draw = useCallback(() => {
    const canvas = canvasRef.current
    if (!canvas) return
    const ctx = canvas.getContext('2d')
    const dpr = window.devicePixelRatio || 1
    canvas.width = width * dpr
    canvas.height = height * dpr
    ctx.scale(dpr, dpr)
    ctx.clearRect(0, 0, width, height)

    // ── L0: Background quadrants ──
    const midX = PADDING.left + plotW * 0.5
    const midY = PADDING.top + plotH * 0.5

    // Top-left: 泡沫 (high risk, low opportunity)
    ctx.fillStyle = QUADRANT_COLOURS['泡沫'].bg
    ctx.fillRect(PADDING.left, PADDING.top, plotW * 0.5, plotH * 0.5)
    // Top-right: 拥挤 (high risk, high opportunity)
    ctx.fillStyle = QUADRANT_COLOURS['拥挤'].bg
    ctx.fillRect(midX, PADDING.top, plotW * 0.5, plotH * 0.5)
    // Bottom-left: 防御 (low risk, low opportunity)
    ctx.fillStyle = QUADRANT_COLOURS['防御'].bg
    ctx.fillRect(PADDING.left, midY, plotW * 0.5, plotH * 0.5)
    // Bottom-right: 机会 (low risk, high opportunity)
    ctx.fillStyle = QUADRANT_COLOURS['机会'].bg
    ctx.fillRect(midX, midY, plotW * 0.5, plotH * 0.5)

    // Cross lines
    ctx.strokeStyle = 'rgba(255,255,255,0.12)'
    ctx.lineWidth = 1
    ctx.beginPath()
    ctx.moveTo(midX, PADDING.top)
    ctx.lineTo(midX, PADDING.top + plotH)
    ctx.moveTo(PADDING.left, midY)
    ctx.lineTo(PADDING.left + plotW, midY)
    ctx.stroke()

    // Axis labels
    ctx.fillStyle = 'rgba(255,255,255,0.4)'
    ctx.font = '11px system-ui, sans-serif'
    ctx.textAlign = 'center'
    ctx.fillText('Opportunity →', PADDING.left + plotW / 2, height - 8)
    ctx.save()
    ctx.translate(14, PADDING.top + plotH / 2)
    ctx.rotate(-Math.PI / 2)
    ctx.fillText('Risk →', 0, 0)
    ctx.restore()

    // Quadrant labels (with dark background pill for readability)
    const quadrantLabels = [
      { text: '泡沫区', color: QUADRANT_COLOURS['泡沫'].label, x: PADDING.left + 10, y: PADDING.top + 22, align: 'left' },
      { text: '拥挤区', color: QUADRANT_COLOURS['拥挤'].label, x: PADDING.left + plotW - 10, y: PADDING.top + 22, align: 'right' },
      { text: '防御区', color: QUADRANT_COLOURS['防御'].label, x: PADDING.left + 10, y: PADDING.top + plotH - 10, align: 'left' },
      { text: '机会区', color: QUADRANT_COLOURS['机会'].label, x: PADDING.left + plotW - 10, y: PADDING.top + plotH - 10, align: 'right' },
    ]
    ctx.font = 'bold 13px system-ui, sans-serif'
    for (const lbl of quadrantLabels) {
      const metrics = ctx.measureText(lbl.text)
      const pw = metrics.width + 12
      const ph = 20
      const px = lbl.align === 'right' ? lbl.x - pw : lbl.x
      const py = lbl.y - 14
      // Dark pill background
      ctx.globalAlpha = 0.55
      ctx.fillStyle = '#0d0f14'
      ctx.beginPath()
      ctx.roundRect(px, py, pw, ph, 4)
      ctx.fill()
      // Colored text
      ctx.globalAlpha = 0.85
      ctx.fillStyle = lbl.color
      ctx.textAlign = lbl.align
      ctx.fillText(lbl.text, lbl.align === 'right' ? lbl.x - 6 : lbl.x + 6, lbl.y)
    }
    ctx.globalAlpha = 1.0

    // Tick marks on axes
    ctx.fillStyle = 'rgba(255,255,255,0.25)'
    ctx.font = '10px system-ui, sans-serif'
    ctx.textAlign = 'center'
    for (let v = 0; v <= 100; v += 20) {
      const { x } = mapToCanvas(v, 0, plotW, plotH)
      ctx.fillText(String(v), x, PADDING.top + plotH + 16)
    }
    ctx.textAlign = 'right'
    for (let v = 0; v <= 100; v += 20) {
      const { y } = mapToCanvas(0, v, plotW, plotH)
      ctx.fillText(String(v), PADDING.left - 8, y + 4)
    }

    // ── L1: All-market dots (tiny, semi-transparent) ──
    for (const stock of allStocks) {
      const { x, y } = mapToCanvas(stock.o, stock.r, plotW, plotH)
      const colour = getQuadrantColor(stock.q)
      ctx.fillStyle = colour.dot
      ctx.beginPath()
      ctx.arc(x, y, 1.8, 0, Math.PI * 2)
      ctx.fill()
    }

    // ── L3: Watchlist dots (large, with labels) ──
    const watchlistCodes = new Set(watchlist.map((w) => w.code))
    for (const w of watchlist) {
      const { x, y } = mapToCanvas(w.opportunity, w.risk, plotW, plotH)
      const colour = getQuadrantColor(w.quadrant)

      // Outer ring
      ctx.strokeStyle = colour.label
      ctx.lineWidth = 2
      ctx.beginPath()
      ctx.arc(x, y, 7, 0, Math.PI * 2)
      ctx.stroke()

      // Inner fill
      ctx.fillStyle = colour.label
      ctx.globalAlpha = 0.85
      ctx.beginPath()
      ctx.arc(x, y, 5, 0, Math.PI * 2)
      ctx.fill()
      ctx.globalAlpha = 1.0

      // Name label
      ctx.fillStyle = 'rgba(255,255,255,0.85)'
      ctx.font = 'bold 11px system-ui, sans-serif'
      ctx.textAlign = 'left'
      ctx.fillText(w.name, x + 10, y + 4)
    }
  }, [allStocks, watchlist, width, height, plotW, plotH])

  // Draw + build grid when data changes
  useEffect(() => {
    draw()
    buildGrid()
  }, [draw, buildGrid])

  // Tooltip state (interactive card instead of pointer-events-none text)
  const [activeTooltip, setActiveTooltip] = useState(null) // { name, code, opportunity, risk, quadrant, x, y }

  // Hover handling
  const handleMouseMove = useCallback((e) => {
    const canvas = canvasRef.current
    if (!canvas || !gridRef.current) return

    const rect = canvas.getBoundingClientRect()
    const mx = e.clientX - rect.left
    const my = e.clientY - rect.top

    const { grid, cellW, cellH, GRID_SIZE } = gridRef.current
    const gx = Math.min(GRID_SIZE - 1, Math.max(0, Math.floor((mx - PADDING.left) / cellW)))
    const gy = Math.min(GRID_SIZE - 1, Math.max(0, Math.floor((my - PADDING.top) / cellH)))

    let closest = null
    let closestDist = 15

    for (let dy = -1; dy <= 1; dy++) {
      for (let dx = -1; dx <= 1; dx++) {
        const ny = gy + dy
        const nx = gx + dx
        if (ny < 0 || ny >= GRID_SIZE || nx < 0 || nx >= GRID_SIZE) continue
        for (const item of grid[ny][nx]) {
          const dist = Math.hypot(item.cx - mx, item.cy - my)
          if (dist < closestDist) {
            closestDist = dist
            closest = item
          }
        }
      }
    }

    hoveredRef.current = closest
    canvas.style.cursor = closest ? 'pointer' : 'default'
  }, [])

  const handleClick = useCallback((e) => {
    const item = hoveredRef.current
    if (!item) {
      setActiveTooltip(null)
      return
    }
    const canvas = canvasRef.current
    if (!canvas) return
    const rect = canvas.getBoundingClientRect()
    const mx = e.clientX - rect.left
    const my = e.clientY - rect.top
    setActiveTooltip({
      name: item.name,
      code: item.code,
      opportunity: item.opportunity,
      risk: item.risk,
      quadrant: item.quadrant || '',
      isWatchlist: item.isWatchlist,
      x: mx,
      y: my,
    })
  }, [])

  const handleMouseLeave = useCallback(() => {
    // Don't clear tooltip on mouse leave — user may want to click the button
  }, [])

  // Code → display format: HK=5-digit, A-share=6-digit
  const formatCodeDisplay = (code) => {
    if (/^\d{5}$/.test(String(code))) return String(code).padStart(5, '0')
    return String(code).padStart(6, '0')
  }

  // Code → symbol helper
  const codeToSymbol = (code) => {
    const c = String(code).padStart(6, '0')
    // HK codes are 5-digit (e.g. 00700)
    if (/^\d{5}$/.test(String(code))) return `${c}.HK`
    return c.startsWith('6') || c.startsWith('9') ? `${c}.SH` : `${c}.SZ`
  }

  return (
    <div ref={containerRef} className="relative w-full" onClick={(e) => {
      // Close tooltip when clicking outside it
      if (e.target === canvasRef.current) return // canvas click handled by handleClick
      if (e.target.closest('[data-quadrant-tooltip]')) return
      setActiveTooltip(null)
    }}>
      <canvas
        ref={canvasRef}
        style={{ width, height }}
        onMouseMove={handleMouseMove}
        onClick={handleClick}
        onMouseLeave={handleMouseLeave}
      />
      {activeTooltip && (
        <div
          data-quadrant-tooltip
          className="absolute z-10 rounded-lg border border-border bg-card/95 px-3 py-2.5 text-xs text-white shadow-xl backdrop-blur-sm"
          style={{
            left: Math.min(activeTooltip.x + 12, width - 180),
            top: Math.max(activeTooltip.y - 10, 0),
          }}
        >
          <div className="font-medium">{activeTooltip.name} ({formatCodeDisplay(activeTooltip.code)})</div>
          <div className="mt-1 text-white/60">
            机会: {activeTooltip.opportunity.toFixed(1)} | 风险: {activeTooltip.risk.toFixed(1)}
            {activeTooltip.quadrant && <span> | {activeTooltip.quadrant}</span>}
          </div>
          <button
            type="button"
            onClick={(e) => {
              e.stopPropagation()
              window.open(`/live-trading/${codeToSymbol(activeTooltip.code)}`, '_blank')
              setActiveTooltip(null)
            }}
            className="mt-2 inline-flex w-full items-center justify-center gap-1 rounded-md border border-primary/40 bg-primary/10 px-2 py-1 text-[11px] font-medium text-primary transition hover:bg-primary/20"
          >
            查看详情 →
          </button>
        </div>
      )}
    </div>
  )
}
