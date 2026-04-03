import { useRef, useEffect } from 'react'

/**
 * MiniChart — lightweight Canvas chart (line or bar).
 *
 * Props:
 *   data:    [{ date: "2026-04-01", count: 12 }, ...]
 *   width / height
 *   type:    "line" | "bar" (default "line")
 *   color:   stroke / fill color (default "#e67e22")
 *   areaColor: fill under line (default color + 15% opacity)
 *   label:   optional axis label
 */
const PAD = { top: 20, right: 12, bottom: 24, left: 40 }

export default function MiniChart({
  data = [],
  width = 320,
  height = 140,
  type = 'line',
  color = '#e67e22',
  areaColor,
  label = '',
}) {
  const canvasRef = useRef(null)

  useEffect(() => {
    const canvas = canvasRef.current
    if (!canvas || data.length === 0) return
    const ctx = canvas.getContext('2d')
    const dpr = window.devicePixelRatio || 1
    canvas.width = width * dpr
    canvas.height = height * dpr
    ctx.scale(dpr, dpr)
    ctx.clearRect(0, 0, width, height)

    const plotW = width - PAD.left - PAD.right
    const plotH = height - PAD.top - PAD.bottom
    const values = data.map((d) => d.count || 0)
    const maxVal = Math.max(...values, 1)
    const n = values.length

    // Y-axis ticks
    ctx.fillStyle = 'rgba(255,255,255,0.25)'
    ctx.font = '10px system-ui, sans-serif'
    ctx.textAlign = 'right'
    for (let i = 0; i <= 3; i++) {
      const v = Math.round((maxVal * i) / 3)
      const y = PAD.top + plotH - (plotH * i) / 3
      ctx.fillText(String(v), PAD.left - 6, y + 3)
      // Grid line
      ctx.strokeStyle = 'rgba(255,255,255,0.05)'
      ctx.lineWidth = 1
      ctx.beginPath()
      ctx.moveTo(PAD.left, y)
      ctx.lineTo(PAD.left + plotW, y)
      ctx.stroke()
    }

    // X-axis labels (show first, mid, last date)
    ctx.fillStyle = 'rgba(255,255,255,0.2)'
    ctx.font = '9px system-ui, sans-serif'
    ctx.textAlign = 'center'
    const showDates = [0, Math.floor(n / 2), n - 1]
    showDates.forEach((i) => {
      if (i >= 0 && i < data.length) {
        const x = PAD.left + (plotW * i) / Math.max(n - 1, 1)
        const dateLabel = (data[i].date || '').slice(5) // "04-01"
        ctx.fillText(dateLabel, x, height - 4)
      }
    })

    if (type === 'bar') {
      const barW = Math.max(2, plotW / n - 2)
      values.forEach((v, i) => {
        const x = PAD.left + (plotW * i) / n + 1
        const barH = (v / maxVal) * plotH
        const y = PAD.top + plotH - barH
        ctx.fillStyle = color
        ctx.globalAlpha = 0.7
        ctx.fillRect(x, y, barW, barH)
        ctx.globalAlpha = 1.0
      })
    } else {
      // Line + area
      const points = values.map((v, i) => ({
        x: PAD.left + (plotW * i) / Math.max(n - 1, 1),
        y: PAD.top + plotH - (v / maxVal) * plotH,
      }))

      // Area fill
      ctx.beginPath()
      ctx.moveTo(points[0].x, PAD.top + plotH)
      points.forEach((p) => ctx.lineTo(p.x, p.y))
      ctx.lineTo(points[points.length - 1].x, PAD.top + plotH)
      ctx.closePath()
      ctx.fillStyle = areaColor || (color + '20')
      ctx.fill()

      // Line
      ctx.beginPath()
      points.forEach((p, i) => (i === 0 ? ctx.moveTo(p.x, p.y) : ctx.lineTo(p.x, p.y)))
      ctx.strokeStyle = color
      ctx.lineWidth = 1.5
      ctx.stroke()

      // End dot
      const last = points[points.length - 1]
      ctx.fillStyle = color
      ctx.beginPath()
      ctx.arc(last.x, last.y, 3, 0, Math.PI * 2)
      ctx.fill()
    }

    // Label
    if (label) {
      ctx.fillStyle = 'rgba(255,255,255,0.35)'
      ctx.font = '10px system-ui, sans-serif'
      ctx.textAlign = 'left'
      ctx.fillText(label, PAD.left, 12)
    }
  }, [data, width, height, type, color, areaColor, label])

  if (!data || data.length === 0) {
    return (
      <div className="flex items-center justify-center text-xs text-white/20" style={{ width, height }}>
        暂无数据
      </div>
    )
  }

  return <canvas ref={canvasRef} style={{ width, height }} />
}
