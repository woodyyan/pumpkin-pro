import { existsSync } from 'node:fs'

import puppeteer from 'puppeteer-core'

import {
  AI_ANALYSIS_SHARE_MAX_IMAGE_HEIGHT,
  buildAIAnalysisShareFilename,
  buildAIAnalysisSharePayload,
  countAIAnalysisShareSlices,
} from '../../../lib/ai-analysis-share'

const PREVIEW_PATH = '/share/ai-analysis-preview'
const BROWSER_ARGS = ['--no-sandbox', '--disable-setuid-sandbox', '--font-render-hinting=medium']

function resolveChromeExecutablePath() {
  const candidates = [
    process.env.CHROME_EXECUTABLE_PATH,
    process.env.PUPPETEER_EXECUTABLE_PATH,
    '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
    '/usr/bin/google-chrome-stable',
    '/usr/bin/google-chrome',
    '/usr/bin/chromium-browser',
    '/usr/bin/chromium',
  ].filter(Boolean)

  return candidates.find((candidate) => existsSync(candidate)) || ''
}

function getBaseUrl(req) {
  const protoHeader = String(req.headers['x-forwarded-proto'] || '').split(',')[0].trim()
  const hostHeader = String(req.headers['x-forwarded-host'] || req.headers.host || '').split(',')[0].trim()
  const protocol = protoHeader || (hostHeader.includes('localhost') ? 'http' : 'https')
  return `${protocol}://${hostHeader}`
}

export default async function handler(req, res) {
  if (req.method !== 'POST') {
    res.setHeader('Allow', 'POST')
    res.status(405).json({ detail: 'Method Not Allowed' })
    return
  }

  const payload = buildAIAnalysisSharePayload(req.body)
  if (!payload?.result?.analysis) {
    res.status(400).json({ detail: '缺少 AI 分析结果，无法生成分享图片' })
    return
  }

  const executablePath = resolveChromeExecutablePath()
  if (!executablePath) {
    res.status(500).json({ detail: '未找到可用的 Chrome 浏览器，无法启用服务端兜底截图' })
    return
  }

  let browser
  try {
    browser = await puppeteer.launch({
      headless: true,
      executablePath,
      args: BROWSER_ARGS,
    })

    const page = await browser.newPage()
    await page.setViewport({ width: 1280, height: 900, deviceScaleFactor: 1 })
    await page.goto(`${getBaseUrl(req)}${PREVIEW_PATH}`, { waitUntil: 'networkidle0' })
    await page.evaluate((nextPayload) => {
      window.__AI_SHARE_PAYLOAD__ = nextPayload
      window.dispatchEvent(new CustomEvent('ai-analysis-share-payload', { detail: nextPayload }))
    }, payload)
    await page.waitForSelector('[data-share-ready="true"]', { timeout: 15000 })
    await page.evaluate(async () => {
      if (document?.fonts?.ready) {
        try {
          await document.fonts.ready
        } catch {
          // ignore font readiness errors for fallback screenshots
        }
      }
    })

    const cardHandle = await page.$('[data-share-card-root="true"]')
    if (!cardHandle) throw new Error('分享卡片未渲染完成')

    const box = await cardHandle.boundingBox()
    if (!box) throw new Error('无法计算分享卡片尺寸')

    const total = countAIAnalysisShareSlices(box.height, AI_ANALYSIS_SHARE_MAX_IMAGE_HEIGHT)
    const images = []

    for (let index = 0; index < total; index += 1) {
      const offsetY = index * AI_ANALYSIS_SHARE_MAX_IMAGE_HEIGHT
      const clipHeight = Math.min(AI_ANALYSIS_SHARE_MAX_IMAGE_HEIGHT, box.height - offsetY)
      const buffer = await page.screenshot({
        type: 'png',
        clip: {
          x: Math.max(0, Math.floor(box.x)),
          y: Math.max(0, Math.floor(box.y + offsetY)),
          width: Math.ceil(box.width),
          height: Math.ceil(clipHeight),
        },
      })
      images.push({
        filename: buildAIAnalysisShareFilename(payload, index, total),
        base64: buffer.toString('base64'),
      })
    }

    res.setHeader('Cache-Control', 'no-store')
    res.status(200).json({
      method: 'server',
      images,
      total,
    })
  } catch (error) {
    console.error('Failed to generate AI analysis share images', error)
    res.status(500).json({ detail: error?.message || '服务端生成分享图片失败' })
  } finally {
    if (browser) {
      await browser.close().catch(() => {})
    }
  }
}
