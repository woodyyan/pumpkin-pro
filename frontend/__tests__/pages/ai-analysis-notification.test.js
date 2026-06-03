import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const symbolPageSource = readFileSync(new URL('../../pages/live-trading/[symbol].js', import.meta.url), 'utf8')
const settingsPageSource = readFileSync(new URL('../../pages/settings.js', import.meta.url), 'utf8')

describe('AI analysis browser notification integration', () => {
  it('imports notification helpers in live-trading symbol page', () => {
    assert.match(symbolPageSource, /from ['"]\.\.\/\.\.\/lib\/notification['"]/)
    assert.match(symbolPageSource, /getNotificationPermission/)
    assert.match(symbolPageSource, /requestNotificationPermission/)
    assert.match(symbolPageSource, /sendNotification/)
    assert.match(symbolPageSource, /NOTIFICATION_CATEGORIES/)
  })

  it('has notification permission prompt state in AI analysis section', () => {
    assert.match(symbolPageSource, /aiNotifPromptVisible/)
    assert.match(symbolPageSource, /setAiNotifPromptVisible/)
  })

  it('checks notification permission on first AI analysis click', () => {
    const handleAIStart = symbolPageSource.indexOf('const handleAIAnalysis = async () => {')
    assert.notEqual(handleAIStart, -1)
    const handlerSegment = symbolPageSource.slice(handleAIStart, handleAIStart + 800)
    assert.match(handlerSegment, /getNotificationPermission\(\)/)
    assert.match(handlerSegment, /setAiNotifPromptVisible\(true\)/)
  })

  it('triggers sendNotification after successful AI analysis', () => {
    const successBlock = symbolPageSource.indexOf('setAiResult(result)')
    assert.notEqual(successBlock, -1)
    const segment = symbolPageSource.slice(successBlock, successBlock + 600)
    assert.match(segment, /sendNotification\(/)
    assert.match(segment, /NOTIFICATION_CATEGORIES\.AI_ANALYSIS/)
    assert.match(segment, /signal: result\?\.analysis\?\.signal/)
  })

  it('does not send notification on analysis error', () => {
    const catchBlock = symbolPageSource.indexOf('} catch (err) {')
    assert.notEqual(catchBlock, -1)
    const errSegment = symbolPageSource.slice(catchBlock, catchBlock + 400)
    assert.doesNotMatch(errSegment, /sendNotification/)
  })

  it('passes notification prompt props to AIAnalysisPanel', () => {
    const panelCall = symbolPageSource.indexOf('<AIAnalysisPanel')
    assert.notEqual(panelCall, -1)
    const panelSegment = symbolPageSource.slice(panelCall, panelCall + 600)
    assert.match(panelSegment, /notifPromptVisible/)
    assert.match(panelSegment, /onNotifPromptClose/)
  })

  it('AIAnalysisPanel passes prompt props to AIAnalysisLoadingPanel', () => {
    const panelDef = symbolPageSource.indexOf('function AIAnalysisPanel(')
    assert.notEqual(panelDef, -1)
    const panelDefSegment = symbolPageSource.slice(panelDef, panelDef + 300)
    assert.match(panelDefSegment, /notifPromptVisible/)
    assert.match(panelDefSegment, /onNotifPromptClose/)

    const loadingCall = symbolPageSource.indexOf('<AIAnalysisLoadingPanel')
    assert.notEqual(loadingCall, -1)
    const loadingSegment = symbolPageSource.slice(loadingCall, loadingCall + 400)
    assert.match(loadingSegment, /notifPromptVisible/)
    assert.match(loadingSegment, /onNotifPromptClose/)
  })

  it('AIAnalysisLoadingPanel receives prompt props and renders permission banner', () => {
    const loadingDef = symbolPageSource.indexOf('function AIAnalysisLoadingPanel(')
    assert.notEqual(loadingDef, -1)
    const loadingDefSegment = symbolPageSource.slice(loadingDef, loadingDef + 300)
    assert.match(loadingDefSegment, /notifPromptVisible/)
    assert.match(loadingDefSegment, /onNotifPromptClose/)

    const jsxStart = symbolPageSource.indexOf('return (', loadingDef)
    assert.notEqual(jsxStart, -1)
    const jsxSegment = symbolPageSource.slice(jsxStart, jsxStart + 1200)
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


describe('settings page webhook delivery history', () => {
  it('renders delivery history as collapsed details by default', () => {
    assert.match(settingsPageSource, /<details className="group rounded-xl border border-border bg-\[var\(--color-bg-hover\)\] p-4">/)
    assert.match(settingsPageSource, /默认收起，点开后查看最近的投递记录。/)
    assert.match(settingsPageSource, /group-open:hidden/)
    assert.match(settingsPageSource, /group-open:inline/)
  })
})


describe('settings page webhook channel support', () => {
  it('supports both wecom and feishu options in settings page', () => {
    assert.match(settingsPageSource, /WEBHOOK_CHANNEL_OPTIONS/)
    assert.match(settingsPageSource, /value: 'wecom'/)
    assert.match(settingsPageSource, /value: 'feishu'/)
    assert.match(settingsPageSource, /飞书 Webhook 配置教程/)
    assert.match(settingsPageSource, /webhookConfig\.channel/)
  })

  it('switches payload preview by selected channel', () => {
    assert.match(settingsPageSource, /msgtype: 'text'/)
    assert.match(settingsPageSource, /msg_type: 'text'/)
    assert.match(settingsPageSource, /JSON\.stringify\(webhookChannelMeta\.payloadPreview, null, 2\)/)
  })
})

describe('live-trading AI entry copy and history labeling', () => {
  it('renders separate desktop and mobile copy for the AI entry', () => {
    assert.match(symbolPageSource, /AI_ENTRY_COPY_DESKTOP = 'AI 会给出看多\/看空判断、交易建议、执行条件和风险提示'/)
    assert.match(symbolPageSource, /AI_ENTRY_COPY_MOBILE = '看方向、给建议、提条件、控风险'/)
    assert.match(symbolPageSource, /AI分析能看什么/)
    assert.match(symbolPageSource, /hidden text-\[12px\] leading-5 text-foreground-muted md:block/)
    assert.match(symbolPageSource, /text-\[12px\] leading-5 text-foreground-muted md:hidden/)
  })

  it('renames the history section to AI analysis history with the approved subtitle', () => {
    assert.match(symbolPageSource, /AI分析历史/)
    assert.match(symbolPageSource, /AI_HISTORY_SUBTITLE = '最近一次观点 \+ 5日验证'/)
    assert.match(symbolPageSource, /结果生成后会自动进入「AI分析历史」/)
  })

  it('uses darker light-mode text colors for AI suggestion and catalysts in history cards', () => {
    assert.match(symbolPageSource, /text-sky-800 dark:text-sky-200\/80">📋 交易建议/)
    assert.match(symbolPageSource, /text-sky-800 dark:text-sky-200\/70">✨ 潜在催化因素/)
    assert.match(symbolPageSource, /text-sky-800 dark:text-sky-200\/50 first:mt-0">💡 \{c\}/)
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
