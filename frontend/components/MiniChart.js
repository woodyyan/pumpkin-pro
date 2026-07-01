import { useRef, useEffect } from 'react'
import { buildYAxisScale, formatTickValue } from '../lib/mini-chart-scale'

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
 *   yAxisMode:       "auto" | "zero-based" (default "auto")
 *                    auto = Y 轴按数据 min/max ± padding 自适应，放大微小波动
 *                    zero-based = 旧行为，0 到 max，适合必须从 0 起算的柱状图
 *   valuePrecision:  Y 轴刻度小数位；不传时按量级自适应（≥100 取整，否则 2 位）
 *   baselineValue:   可选基准线值（如起始净值），在图上画一条虚线参考
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
  yAxisMode = 'auto',
  valuePrecision,
  baselineValue,
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
    const n = values.length

    const scale = buildYAxisScale(values, {
      mode: yAxisMode,
      tickCount: 3,
      valuePrecision,
    })
    const yMin = scale ? scale.yMin : 0
    const yMax = scale ? scale.yMax : 1
    const ticks = scale ? scale.ticks : [0, 0.33, 0.67, 1]
    const precision = scale ? scale.valuePrecision : 0
    const span = yMax - yMin || 1

    const mapY = (v) => PAD.top + plotH - ((v - yMin) / span) * plotH

    // Y-axis ticks
    ctx.fillStyle = 'rgba(255,255,255,0.25)'
    ctx.font = '10px system-ui, sans-serif'
    ctx.textAlign = 'right'
    ticks.forEach((v, i) => {
      const y = PAD.top + plotH - (plotH * i) / (ticks.length - 1)
      ctx.fillText(formatTickValue(v, precision), PAD.left - 6, y + 3)
      // Grid line
      ctx.strokeStyle = 'rgba(255,255,255,0.05)'
      ctx.lineWidth = 1
      ctx.beginPath()
      ctx.moveTo(PAD.left, y)
      ctx.lineTo(PAD.left + plotW, y)
      ctx.stroke()
    })

    // Optional baseline reference line
    if (
      baselineValue != null &&
      Number.isFinite(baselineValue) &&
      baselineValue >= yMin &&
      baselineValue <= yMax
    ) {
      const y = mapY(baselineValue)
      ctx.save()
      ctx.strokeStyle = 'rgba(255,255,255,0.3)'
      ctx.setLineDash([4, 4])
      ctx.lineWidth = 1
      ctx.beginPath()
      ctx.moveTo(PAD.left, y)
      ctx.lineTo(PAD.left + plotW, y)
      ctx.stroke()
      ctx.restore()
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
        const barH = ((v - yMin) / span) * plotH
        const y = PAD.top + plotH - barH
        ctx.fillStyle = color
        ctx.globalAlpha = 0.7
        ctx.fillRect(x, y, barW, Math.max(barH, 0))
        ctx.globalAlpha = 1.0
      })
    } else {
      // Line + area
      const points = values.map((v, i) => ({
        x: PAD.left + (plotW * i) / Math.max(n - 1, 1),
        y: mapY(v),
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
  }, [
    data,
    width,
    height,
    type,
    color,
    areaColor,
    label,
    yAxisMode,
    valuePrecision,
    baselineValue,
  ])

  if (!data || data.length === 0) {
    return (
      <div className="flex items-center justify-center text-xs text-foreground-disabled" style={{ width, height }}>
        暂无数据
      </div>
    )
  }

  return <canvas ref={canvasRef} style={{ width, height }} />
}
