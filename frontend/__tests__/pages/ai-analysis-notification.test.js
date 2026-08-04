import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const symbolPageSource = readFileSync(new URL('../../pages/live-trading/[symbol].js', import.meta.url), 'utf8')
const workspaceSource = readFileSync(new URL('../../components/AIAnalysisWorkspace.js', import.meta.url), 'utf8')
const analysisPageSource = readFileSync(new URL('../../pages/ai/analysis.js', import.meta.url), 'utf8')
const helperSource = readFileSync(new URL('../../lib/ai-analysis-helpers.js', import.meta.url), 'utf8')
const settingsPageSource = readFileSync(new URL('../../pages/settings.js', import.meta.url), 'utf8')
const webhookPanelSource = readFileSync(new URL('../../components/SignalWebhookPanel.js', import.meta.url), 'utf8')

describe('AI analysis browser notification integration', () => {
  it('extracts notification helpers into the shared AI workspace', () => {
    assert.match(workspaceSource, /from ['"]\.\.\/lib\/notification['"]/) 
    assert.match(workspaceSource, /getNotificationPermission/)
    assert.match(workspaceSource, /requestNotificationPermission/)
    assert.match(workspaceSource, /sendNotification/)
    assert.match(workspaceSource, /NOTIFICATION_CATEGORIES/)
  })

  it('uses shared notification prompt state on both live-trading and AI analysis pages', () => {
    assert.match(symbolPageSource, /aiNotifPromptVisible/)
    assert.match(symbolPageSource, /onNotifPromptClose/)
    assert.match(analysisPageSource, /notifPromptVisible: maybePromptNotification\(\)/)
    assert.match(analysisPageSource, /onNotifPromptClose/)
  })

  it('checks notification permission through the shared helper before analysis starts', () => {
    assert.match(symbolPageSource, /getNotificationPermission\(\)/)
    assert.match(analysisPageSource, /if \(maybePromptNotification\(\)\)/)
    assert.match(helperSource, /export function maybePromptNotification\(\)/)
    assert.match(helperSource, /Notification\.permission === 'default'/)
  })

  it('triggers shared notification dispatch after successful AI analysis', () => {
    assert.match(symbolPageSource, /notifyAIAnalysisFinished\(\{ symbol, symbolName: symbolName \|\| symbol, result \}\)/)
    assert.match(analysisPageSource, /notifyAIAnalysisFinished\(\{ symbol: target\.symbol, symbolName: target\.symbolName, result \}\)/)
    assert.match(workspaceSource, /sendNotification\(NOTIFICATION_CATEGORIES\.AI_ANALYSIS, \{/)
    assert.match(workspaceSource, /signal: result\?\.analysis\?\.signal/)
  })

  it('does not send notification on analysis error branches', () => {
    const liveCatchStart = symbolPageSource.indexOf('} catch (err) {', symbolPageSource.indexOf('const handleAIAnalysis = async () => {'))
    const liveCatchSegment = symbolPageSource.slice(liveCatchStart, liveCatchStart + 300)
    const pageCatchStart = analysisPageSource.indexOf('} catch (err) {', analysisPageSource.indexOf('const handleSubmit = useCallback(async () => {'))
    const pageCatchSegment = analysisPageSource.slice(pageCatchStart, pageCatchStart + 300)
    assert.doesNotMatch(liveCatchSegment, /notifyAIAnalysisFinished|sendNotification/)
    assert.doesNotMatch(pageCatchSegment, /notifyAIAnalysisFinished|sendNotification/)
  })

  it('AIAnalysisPanel passes prompt props to AIAnalysisLoadingPanel', () => {
    const panelDef = workspaceSource.indexOf('export function AIAnalysisPanel(')
    assert.notEqual(panelDef, -1)
    const panelDefSegment = workspaceSource.slice(panelDef, panelDef + 500)
    assert.match(panelDefSegment, /notifPromptVisible/)
    assert.match(panelDefSegment, /onNotifPromptClose/)

    const loadingCall = workspaceSource.indexOf('<AIAnalysisLoadingPanel')
    assert.notEqual(loadingCall, -1)
    const loadingSegment = workspaceSource.slice(loadingCall, loadingCall + 400)
    assert.match(loadingSegment, /notifPromptVisible/)
    assert.match(loadingSegment, /onNotifPromptClose/)
  })

  it('AIAnalysisLoadingPanel receives prompt props and renders permission banner', () => {
    const loadingDef = workspaceSource.indexOf('export function AIAnalysisLoadingPanel(')
    assert.notEqual(loadingDef, -1)
    const loadingDefSegment = workspaceSource.slice(loadingDef, loadingDef + 300)
    assert.match(loadingDefSegment, /notifPromptVisible/)
    assert.match(loadingDefSegment, /onNotifPromptClose/)

    const jsxStart = workspaceSource.indexOf('return (', loadingDef)
    assert.notEqual(jsxStart, -1)
    const jsxSegment = workspaceSource.slice(jsxStart, jsxStart + 2200)
    assert.match(jsxSegment, /notifPromptVisible/)
    assert.match(jsxSegment, /分析完成后通过桌面通知提醒你/)
    assert.match(jsxSegment, /开启通知/)
    assert.match(jsxSegment, /稍后再说/)
    assert.match(jsxSegment, /requestNotificationPermission/)
  })
})

describe('settings page notification preferences', () => {
  it('imports notification helpers in settings page', () => {
    assert.match(settingsPageSource, /from ['"]\.\.\/lib\/notification\.js['"]/) 
    assert.match(settingsPageSource, /isIOSSafari/)
    assert.match(settingsPageSource, /isNotificationSupported/)
    assert.match(settingsPageSource, /loadNotificationPreferences/)
    assert.match(settingsPageSource, /saveNotificationPreferences/)
  })

  it('renders desktop notification section conditionally', () => {
    assert.match(settingsPageSource, /notifSupported &&/)
    assert.match(settingsPageSource, /桌面通知/)
  })

  it('has AI analysis notification toggle', () => {
    assert.match(settingsPageSource, /AI 分析完成时通知我/)
    assert.match(settingsPageSource, /notifPrefs\.aiAnalysis/)
    assert.match(settingsPageSource, /saveNotificationPreferences/)
  })

  it('shows iOS hint conditionally', () => {
    assert.match(settingsPageSource, /isIOSSafari\(\)/)
    assert.match(settingsPageSource, /iOS 用户/)
    assert.match(settingsPageSource, /添加到主屏幕后可在后台接收通知/)
  })
})

describe('webhook panel moved into signal center', () => {
  it('settings page links to the signal center for webhook management', () => {
    assert.match(settingsPageSource, /信号通知/)
    assert.match(settingsPageSource, /href="\/signals"/)
    assert.match(settingsPageSource, /前往信号中心/)
    // 旧的内嵌 webhook 配置与投递可观测已迁出设置页。
    assert.doesNotMatch(settingsPageSource, /WEBHOOK_CHANNEL_OPTIONS/)
    assert.doesNotMatch(settingsPageSource, /Webhook 推送配置/)
    assert.doesNotMatch(settingsPageSource, /Webhook 投递可观测/)
  })

  it('signal webhook panel keeps wecom and feishu channel options', () => {
    assert.match(webhookPanelSource, /WEBHOOK_CHANNEL_OPTIONS/)
    assert.match(webhookPanelSource, /value: 'wecom'/)
    assert.match(webhookPanelSource, /value: 'feishu'/)
    assert.match(webhookPanelSource, /飞书 Webhook 配置教程/)
  })

  it('signal webhook panel keeps save, test and recent deliveries', () => {
    assert.match(webhookPanelSource, /保存 Webhook/)
    assert.match(webhookPanelSource, /验证 Webhook 送达/)
    assert.match(webhookPanelSource, /最近投递/)
    assert.match(webhookPanelSource, /\/api\/webhook-deliveries/)
    assert.match(webhookPanelSource, /formatDeliveryStatus/)
  })
})

describe('live-trading AI entry copy and history labeling', () => {
  it('uses stronger light-mode colors for waiting panel hold and pending badges', () => {
    assert.match(symbolPageSource, /hold: 'text-amber-800 dark:text-amber-200 bg-amber-500\/10 border-amber-400\/25'/)
    assert.match(symbolPageSource, /return 'text-amber-800 dark:text-amber-200 bg-amber-500\/10 border-amber-400\/25'/)
  })

  it('renders separate desktop and mobile copy for the AI entry', () => {
    assert.match(symbolPageSource, /AI_ENTRY_COPY_DESKTOP = 'AI 会给出看多\/看空判断、交易建议、执行条件和风险提示'/)
    assert.match(symbolPageSource, /AI_ENTRY_COPY_MOBILE = '看方向、给建议、提条件、控风险'/)
    assert.match(symbolPageSource, /AI分析能看什么/)
    assert.match(symbolPageSource, /hidden text-\[12px\] leading-5 text-foreground-muted md:block/)
    assert.match(symbolPageSource, /text-\[12px\] leading-5 text-foreground-muted md:hidden/)
  })

  it('keeps the approved AI analysis history subtitle on the detail page', () => {
    assert.match(symbolPageSource, /AI_HISTORY_SUBTITLE = '最近一次观点 \+ 5日验证'/)
    assert.match(symbolPageSource, /AnalysisHistoryPanel/)
  })

  it('uses darker light-mode text colors for AI suggestion and catalysts in history cards', () => {
    const historySource = readFileSync(new URL('../../components/AIAnalysisHistorySection.js', import.meta.url), 'utf8')
    assert.match(historySource, /text-sky-800 dark:text-sky-200\/80">📋 交易建议/)
    assert.match(historySource, /text-sky-800 dark:text-sky-200\/70">✨ 潜在催化因素/)
    assert.match(historySource, /analysis\.key_catalysts\.map\(\(c, idx\) => <div key=\{idx\} className="mt-1 first:mt-0">💡 \{c\}<\/div>\)/)
  })
})

describe('AIAnalysisReportContent light mode contrast', () => {
  const reportContentSource = readFileSync(new URL('../../components/AIAnalysisReportContent.js', import.meta.url), 'utf8')

  it('uses darker light-mode text colors for trade suggestion and catalyst blocks', () => {
    assert.match(reportContentSource, /text-sky-800 dark:text-sky-200\/90">📋 交易建议/)
    assert.match(reportContentSource, /text-sky-800 dark:text-sky-200\/90">✨ 潜在催化因素/)
    assert.match(reportContentSource, /text-sky-800 dark:text-sky-200\/65 first:mt-0">💡 \{item\}/)
  })
})
