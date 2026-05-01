export const AI_ANALYSIS_SHARE_MAX_IMAGE_HEIGHT = 6000
export const AI_ANALYSIS_SHARE_PIXEL_RATIO = 2

const SHARE_FILENAME_TIME_FORMATTER = new Intl.DateTimeFormat('zh-CN', {
  timeZone: 'Asia/Shanghai',
  year: 'numeric',
  month: '2-digit',
  day: '2-digit',
  hour: '2-digit',
  minute: '2-digit',
  hour12: false,
})

function cloneResult(result) {
  if (!result || typeof result !== 'object') return { analysis: null, meta: {} }
  try {
    return JSON.parse(JSON.stringify(result))
  } catch {
    return result
  }
}

export function getAIAnalysisShareMarketLabel(exchange) {
  return exchange === 'SSE' || exchange === 'SZSE' ? 'A股' : '港股'
}

export function getAIAnalysisSharePrimaryTimestamp(payload) {
  return payload?.result?.meta?.generated_at || payload?.result?.analysis?.data_timestamp || ''
}

export function getAIAnalysisShareDataTimestamp(payload) {
  return payload?.result?.analysis?.data_timestamp || ''
}

export function buildAIAnalysisSharePayload(payload) {
  if (!payload) return null
  return {
    symbol: String(payload.symbol || '').toUpperCase(),
    symbolName: payload.symbolName || payload.symbol || '',
    exchange: payload.exchange || '',
    marketLabel: payload.marketLabel || getAIAnalysisShareMarketLabel(payload.exchange || ''),
    result: cloneResult(payload.result),
  }
}

function sanitizeFilenameSegment(value, fallback = 'AI分析') {
  const normalized = String(value || '')
    .replace(/[\\/:*?"<>|]+/g, '-')
    .replace(/\s+/g, '-')
    .replace(/-+/g, '-')
    .replace(/^-|-$/g, '')
  return normalized || fallback
}

function formatFilenameTime(value) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return 'unknown-time'

  const parts = SHARE_FILENAME_TIME_FORMATTER.formatToParts(date)
  const partMap = Object.fromEntries(parts.filter((part) => part.type !== 'literal').map((part) => [part.type, part.value]))

  const year = partMap.year
  const month = partMap.month
  const day = partMap.day
  const hours = partMap.hour
  const minutes = partMap.minute

  if (!year || !month || !day || !hours || !minutes) return 'unknown-time'
  return `${year}-${month}-${day}-${hours}${minutes}`
}

export function buildAIAnalysisShareFilename(payload, index = 0, total = 1) {
  const normalizedPayload = buildAIAnalysisSharePayload(payload) || {}
  const stockName = sanitizeFilenameSegment(normalizedPayload.symbolName || normalizedPayload.symbol || '股票', '股票')
  const stockCode = sanitizeFilenameSegment(normalizedPayload.symbol || 'UNKNOWN', 'UNKNOWN')
  const timestamp = formatFilenameTime(getAIAnalysisSharePrimaryTimestamp(normalizedPayload))
  const suffix = total > 1 ? `-${index + 1}of${total}` : ''
  return `AI分析-${stockName}-${stockCode}-${timestamp}${suffix}.png`
}

export function countAIAnalysisShareSlices(height, maxHeight = AI_ANALYSIS_SHARE_MAX_IMAGE_HEIGHT) {
  const safeHeight = Math.max(0, Number.isFinite(Number(height)) ? Number(height) : 0)
  const safeMaxHeight = Math.max(1, Number.isFinite(Number(maxHeight)) ? Number(maxHeight) : AI_ANALYSIS_SHARE_MAX_IMAGE_HEIGHT)
  return Math.max(1, Math.ceil(safeHeight / safeMaxHeight))
}

export function isSafariUserAgent(userAgent = '') {
  const ua = String(userAgent || '')
  return /Safari\//.test(ua) && !/Chrome\//.test(ua) && !/Chromium\//.test(ua) && !/Edg\//.test(ua) && !/OPR\//.test(ua) && !/CriOS\//.test(ua) && !/Android/.test(ua)
}

export function isIOSUserAgent(userAgent = '') {
  const ua = String(userAgent || '')
  return /iP(hone|ad|od)/.test(ua) || (/Macintosh/.test(ua) && /Mobile/.test(ua))
}

export function shouldUseServerShareFallback({ userAgent = '', elementHeight = 0, maxHeight = AI_ANALYSIS_SHARE_MAX_IMAGE_HEIGHT } = {}) {
  const safeHeight = Math.max(0, Number.isFinite(Number(elementHeight)) ? Number(elementHeight) : 0)
  return safeHeight > maxHeight || isIOSUserAgent(userAgent) || isSafariUserAgent(userAgent)
}

async function waitForShareCaptureReady() {
  if (typeof window === 'undefined') return
  if (document?.fonts?.ready) {
    try {
      await document.fonts.ready
    } catch {
      // ignore font readiness failure and continue
    }
  }
  await new Promise((resolve) => window.requestAnimationFrame(() => resolve()))
  await new Promise((resolve) => window.requestAnimationFrame(() => resolve()))
}

function canvasToBlob(canvas) {
  return new Promise((resolve, reject) => {
    canvas.toBlob((blob) => {
      if (blob) {
        resolve(blob)
        return
      }
      reject(new Error('无法生成分享图片'))
    }, 'image/png')
  })
}

export async function splitCanvasIntoShareFiles(canvas, payload, maxHeight = AI_ANALYSIS_SHARE_MAX_IMAGE_HEIGHT) {
  const total = countAIAnalysisShareSlices(canvas?.height || 0, maxHeight)
  const files = []
  for (let index = 0; index < total; index += 1) {
    const sliceHeight = Math.min(maxHeight, canvas.height - index * maxHeight)
    const outputCanvas = document.createElement('canvas')
    outputCanvas.width = canvas.width
    outputCanvas.height = sliceHeight
    const context = outputCanvas.getContext('2d')
    if (!context) throw new Error('无法切分分享图片')
    context.drawImage(canvas, 0, index * maxHeight, canvas.width, sliceHeight, 0, 0, canvas.width, sliceHeight)
    const blob = await canvasToBlob(outputCanvas)
    files.push({
      blob,
      filename: buildAIAnalysisShareFilename(payload, index, total),
    })
  }
  return files
}

function downloadBlob(blob, filename) {
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = filename
  document.body.appendChild(anchor)
  anchor.click()
  anchor.remove()
  window.setTimeout(() => URL.revokeObjectURL(url), 1000)
}

function canUseFileShare(files) {
  if (typeof navigator === 'undefined' || typeof navigator.share !== 'function' || typeof File === 'undefined') return false
  const shareFiles = files.map(({ blob, filename }) => new File([blob], filename, { type: blob.type || 'image/png' }))
  if (typeof navigator.canShare !== 'function') return shareFiles.length === 1
  return navigator.canShare({ files: shareFiles })
}

async function shareOrDownloadFiles(files, payload) {
  if (canUseFileShare(files)) {
    const shareFiles = files.map(({ blob, filename }) => new File([blob], filename, { type: blob.type || 'image/png' }))
    try {
      await navigator.share({
        files: shareFiles,
        title: `${payload?.symbolName || payload?.symbol || '股票'} AI分析`,
        text: `${payload?.symbolName || payload?.symbol || '股票'} AI 分析结果分享图`,
      })
      return { action: 'shared' }
    } catch (error) {
      if (error?.name === 'AbortError') return { action: 'cancelled' }
    }
  }

  files.forEach(({ blob, filename }) => downloadBlob(blob, filename))
  return { action: 'downloaded' }
}

function decodeBase64Image(base64) {
  const byteChars = atob(base64)
  const bytes = new Uint8Array(byteChars.length)
  for (let index = 0; index < byteChars.length; index += 1) {
    bytes[index] = byteChars.charCodeAt(index)
  }
  return new Blob([bytes], { type: 'image/png' })
}

async function requestServerShareFiles(payload, fallbackUrl) {
  const response = await fetch(fallbackUrl, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Accept: 'application/json',
    },
    body: JSON.stringify(payload),
  })

  const data = await response.json().catch(() => null)
  if (!response.ok) {
    throw new Error(data?.detail || '服务端生成分享图片失败')
  }

  return Array.isArray(data?.images)
    ? data.images.map((item) => ({
        filename: item.filename,
        blob: decodeBase64Image(item.base64 || ''),
      }))
    : []
}

async function renderShareFilesClient(element, payload) {
  const { toCanvas } = await import('html-to-image')
  const canvas = await toCanvas(element, {
    backgroundColor: '#090b10',
    pixelRatio: AI_ANALYSIS_SHARE_PIXEL_RATIO,
    cacheBust: true,
  })
  return splitCanvasIntoShareFiles(canvas, payload)
}

export async function exportAIAnalysisShareImages({ element, payload, fallbackUrl = '/api/share/ai-analysis-image' }) {
  const normalizedPayload = buildAIAnalysisSharePayload(payload)
  if (!normalizedPayload?.result?.analysis) throw new Error('分析结果尚未准备好')
  if (!element) throw new Error('分享图内容尚未渲染完成')

  await waitForShareCaptureReady()

  const elementHeight = Math.max(element.scrollHeight || 0, element.offsetHeight || 0, Math.ceil(element.getBoundingClientRect?.().height || 0))
  const userAgent = typeof navigator !== 'undefined' ? navigator.userAgent || '' : ''

  let files = []
  let method = 'client'

  if (shouldUseServerShareFallback({ userAgent, elementHeight })) {
    files = await requestServerShareFiles(normalizedPayload, fallbackUrl)
    method = 'server'
  } else {
    try {
      files = await renderShareFilesClient(element, normalizedPayload)
    } catch {
      files = await requestServerShareFiles(normalizedPayload, fallbackUrl)
      method = 'server'
    }
  }

  if (!files.length) throw new Error('分享图片生成失败')

  const delivery = await shareOrDownloadFiles(files, normalizedPayload)
  return {
    method,
    files,
    total: files.length,
    ...delivery,
  }
}
