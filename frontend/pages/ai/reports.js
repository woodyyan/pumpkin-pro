import Head from 'next/head'
import { useCallback, useEffect, useMemo, useState } from 'react'

import { useAuth } from '../../lib/auth-context'
import { requestJson } from '../../lib/api'
import {
  AI_REPORT_PRICING_PLANS,
  DEFAULT_AI_REPORT_DELIVERY_TEXT,
  DEFAULT_AI_REPORT_RISK_DISCLAIMER,
  getAIReportMarketLabel,
  normalizeAIReportItems,
} from '../../lib/ai-reports'

const FEATURE_CARDS = [
  ['覆盖 A 股与中国香港股票', '支持主流 A 股与中国香港股票个股研报定制。'],
  ['专业深度研报', '结合专业股票分析师框架、卧龙AI模型与市场数据生成。'],
  ['明确投资建议', '覆盖买入、卖出、观望、短线、长线等判断。'],
  ['多维度分析', '包含技术面、基本面、资金面、宏观市场等分析。'],
  ['目标位与止损位', '给出关键观察位、目标区间和风险控制参考。'],
  ['财报与事件解读', '解读财报、新闻、政策、产业事件对个股的影响。'],
]

const SERVICE_STEPS = [
  ['添加微信', '备注 AI研报 + 股票名称 / 代码。'],
  ['确认需求', '工作人员确认股票、市场和分析侧重点。'],
  ['生成研报', '分析师框架、卧龙AI模型与市场数据共同生成。'],
  ['微信交付', '大部分情况下 1 小时内完成交付。'],
]

function ReportCard({ report, isLoggedIn, onPreview }) {
  return (
    <article className="group overflow-hidden rounded-2xl border border-border bg-card transition hover:border-primary/40">
      <div className="relative aspect-[4/3] overflow-hidden bg-[var(--color-bg-hover)]">
        {report.thumbnailURL ? (
          <img src={report.thumbnailURL} alt={`${report.stockName} AI研报缩略图`} className="h-full w-full object-cover object-top transition duration-300 group-hover:scale-[1.02]" />
        ) : (
          <div className="flex h-full items-center justify-center text-sm text-foreground-dim">暂无缩略图</div>
        )}
        {!isLoggedIn && (
          <div className="absolute inset-x-3 bottom-3 rounded-xl border border-border bg-card/90 px-3 py-2 text-xs text-foreground-muted backdrop-blur">
            登录后可预览完整研报样例
          </div>
        )}
      </div>
      <div className="space-y-3 p-4">
        <div>
          <div className="flex items-center justify-between gap-3">
            <h3 className="text-base font-semibold text-foreground">{report.stockName}</h3>
            <span className="rounded-full border border-border px-2 py-0.5 text-xs text-foreground-muted">{getAIReportMarketLabel(report.exchange)}</span>
          </div>
          <div className="mt-1 text-sm text-foreground-muted">{report.symbol}</div>
        </div>
        <div className="text-xs text-foreground-dim">数据截至交易日：{report.sourceTradeDate || '--'}</div>
        <button type="button" onClick={() => onPreview(report)} className="w-full rounded-xl bg-primary px-4 py-2.5 text-sm font-semibold text-black transition hover:bg-primary/90">
          {isLoggedIn ? '预览研报' : '登录后预览'}
        </button>
      </div>
    </article>
  )
}

function ReportPreviewModal({ report, preview, loading, error, riskText, serviceConfig, onClose }) {
  if (!report) return null
  return (
    <div className="fixed inset-0 z-[70] flex items-stretch justify-center bg-black/70 p-0 sm:items-center sm:p-6" role="dialog" aria-modal="true">
      <section className="flex h-full w-full flex-col overflow-hidden bg-card text-foreground sm:h-[88vh] sm:max-w-5xl sm:rounded-3xl sm:border sm:border-border">
        <header className="flex items-start justify-between gap-4 border-b border-border px-4 py-4 sm:px-6">
          <div>
            <h2 className="text-lg font-semibold text-foreground">{report.stockName} AI研报</h2>
            <p className="mt-1 text-sm text-foreground-muted">{report.symbol} · {getAIReportMarketLabel(report.exchange)} · 数据截至 {report.sourceTradeDate || '--'}</p>
          </div>
          <button type="button" onClick={onClose} className="rounded-full border border-border px-3 py-1.5 text-sm text-foreground-muted transition hover:border-primary hover:text-foreground">关闭</button>
        </header>
        <div className="flex-1 overflow-auto bg-background p-4 sm:p-6">
          {loading ? (
            <div className="flex min-h-[50vh] items-center justify-center text-sm text-foreground-muted">正在加载研报预览...</div>
          ) : error ? (
            <div className="rounded-2xl border border-negative/25 bg-negative/10 p-4 text-sm text-negative">{error}</div>
          ) : preview?.preview_url ? (
            <img src={preview.preview_url} alt={`${report.stockName} AI研报预览`} className="mx-auto w-full max-w-3xl rounded-2xl border border-border bg-card" />
          ) : (
            <div className="rounded-2xl border border-border bg-card p-4 text-sm text-foreground-muted">暂无可预览图片</div>
          )}
        </div>
        <footer className="border-t border-border bg-card px-4 py-4 sm:px-6">
          <div className="grid gap-3 md:grid-cols-[1fr_auto] md:items-center">
            <p className="text-xs leading-5 text-foreground-dim">{riskText || DEFAULT_AI_REPORT_RISK_DISCLAIMER}</p>
            <div className="flex flex-col gap-1 text-sm text-foreground-muted md:text-right">
              <span>添加微信定制研报</span>
              <span className="font-semibold text-primary">{serviceConfig?.wechat_id || '请查看页面微信二维码'}</span>
            </div>
          </div>
        </footer>
      </section>
    </div>
  )
}

export default function AIReportsPage() {
  const { isLoggedIn, openAuthModal, ready } = useAuth()
  const [reports, setReports] = useState([])
  const [serviceConfig, setServiceConfig] = useState(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [selectedReport, setSelectedReport] = useState(null)
  const [preview, setPreview] = useState(null)
  const [previewLoading, setPreviewLoading] = useState(false)
  const [previewError, setPreviewError] = useState('')

  const deliveryText = serviceConfig?.delivery_time_text || DEFAULT_AI_REPORT_DELIVERY_TEXT
  const riskText = serviceConfig?.risk_disclaimer || DEFAULT_AI_REPORT_RISK_DISCLAIMER

  const loadPageData = useCallback(async () => {
    setLoading(true)
    try {
      const [reportPayload, configPayload] = await Promise.all([
        requestJson('/api/ai/reports', undefined, '加载 AI 研报失败'),
        requestJson('/api/ai/report-service-config', undefined, '加载 AI 研报服务配置失败'),
      ])
      setReports(normalizeAIReportItems(reportPayload?.items))
      setServiceConfig(configPayload || null)
      setError('')
    } catch (err) {
      setError(err.message || '页面加载失败')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    loadPageData()
  }, [loadPageData])

  const handlePreview = useCallback(async (report) => {
    if (!isLoggedIn) {
      openAuthModal('login', '登录后可预览完整 AI研报样例。当前页面仅展示缩略图。')
      return
    }
    setSelectedReport(report)
    setPreview(null)
    setPreviewError('')
    setPreviewLoading(true)
    try {
      const data = await requestJson(`/api/ai/reports/${encodeURIComponent(report.id)}/preview`, undefined, '加载研报预览失败')
      setPreview(data)
    } catch (err) {
      if (Number(err?.status) === 401) {
        openAuthModal('login', '登录后可预览完整 AI研报样例。')
        setSelectedReport(null)
        return
      }
      setPreviewError(err.message || '加载研报预览失败')
    } finally {
      setPreviewLoading(false)
    }
  }, [isLoggedIn, openAuthModal])

  const featuredReports = useMemo(() => reports.slice(0, 9), [reports])

  return (
    <>
      <Head>
        <title>AI研报 - 卧龙AI量化交易台</title>
        <meta name="description" content="AI研报覆盖 A 股与中国香港股票，结合专业股票分析师框架、卧龙AI模型与市场数据，生成个股深度研究报告。" />
      </Head>

      <main className="mx-auto w-full max-w-7xl px-4 py-10 sm:px-6 lg:px-8">
        <section className="grid gap-8 rounded-3xl border border-border bg-card p-6 sm:p-8 lg:grid-cols-[1.1fr_0.9fr] lg:items-center">
          <div>
            <div className="mb-4 inline-flex rounded-full border border-primary/25 bg-primary/10 px-3 py-1 text-xs font-medium text-primary">覆盖 A 股与中国香港股票</div>
            <h1 className="text-3xl font-bold tracking-tight text-foreground sm:text-5xl">AI研报，面向个股的深度投资研究报告</h1>
            <p className="mt-5 max-w-3xl text-base leading-8 text-foreground-muted">结合专业股票分析师框架、卧龙AI模型与市场数据，围绕技术面、基本面、资金面、宏观环境、财报与事件变化，生成包含操作建议、目标位、止损位与风险提示的个股研报。</p>
            <div className="mt-6 flex flex-col gap-3 sm:flex-row">
              <a href="#samples" className="rounded-xl bg-primary px-5 py-3 text-center text-sm font-semibold text-black transition hover:bg-primary/90">查看研报样例</a>
              <a href="#wechat" className="rounded-xl border border-border px-5 py-3 text-center text-sm font-semibold text-foreground transition hover:border-primary hover:text-primary">添加微信定制</a>
            </div>
            <p className="mt-4 text-xs leading-5 text-foreground-dim">研报内容可能包含投资建议，仅供研究参考，不构成收益承诺。市场有风险，投资需谨慎。</p>
          </div>
          <div className="rounded-3xl border border-border bg-background p-4">
            <div className="rounded-2xl border border-border bg-card p-4">
              <div className="text-sm font-semibold text-foreground">交付时效</div>
              <div className="mt-3 text-3xl font-bold text-primary">多数 1 小时内</div>
              <p className="mt-3 text-sm leading-6 text-foreground-muted">{deliveryText}</p>
            </div>
            <div className="mt-4 grid grid-cols-2 gap-3 text-sm">
              <div className="rounded-2xl border border-border bg-card p-4"><div className="text-foreground-dim">最快</div><div className="mt-1 font-semibold text-foreground">10 分钟</div></div>
              <div className="rounded-2xl border border-border bg-card p-4"><div className="text-foreground-dim">复杂情况</div><div className="mt-1 font-semibold text-foreground">24 小时内</div></div>
            </div>
          </div>
        </section>

        <section className="mt-10 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {FEATURE_CARDS.map(([title, desc]) => (
            <div key={title} className="rounded-2xl border border-border bg-card p-5">
              <h2 className="text-base font-semibold text-foreground">{title}</h2>
              <p className="mt-2 text-sm leading-6 text-foreground-muted">{desc}</p>
            </div>
          ))}
        </section>

        <section id="samples" className="mt-12">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
            <div>
              <h2 className="text-2xl font-bold text-foreground">研报样例</h2>
              <p className="mt-2 text-sm text-foreground-muted">未登录用户可查看缩略图；登录后可弹窗预览完整研报样例，不默认提供原图下载。</p>
            </div>
            {!ready || isLoggedIn ? null : <button type="button" onClick={() => openAuthModal('login', '登录后可预览完整 AI研报样例。')} className="rounded-xl border border-border px-4 py-2 text-sm font-semibold text-foreground hover:border-primary hover:text-primary">登录后预览</button>}
          </div>
          {loading ? (
            <div className="mt-6 rounded-2xl border border-border bg-card p-8 text-center text-sm text-foreground-muted">正在加载研报样例...</div>
          ) : error ? (
            <div className="mt-6 rounded-2xl border border-negative/25 bg-negative/10 p-4 text-sm text-negative">{error}</div>
          ) : featuredReports.length === 0 ? (
            <div className="mt-6 rounded-2xl border border-border bg-card p-8 text-center text-sm text-foreground-muted">研报样例正在整理中，可先添加工作人员微信查看最新案例。</div>
          ) : (
            <div className="mt-6 grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
              {featuredReports.map((report) => <ReportCard key={report.id} report={report} isLoggedIn={isLoggedIn} onPreview={handlePreview} />)}
            </div>
          )}
        </section>

        <section className="mt-12 grid gap-6 lg:grid-cols-[1fr_0.9fr]">
          <div>
            <h2 className="text-2xl font-bold text-foreground">套餐价格</h2>
            <div className="mt-5 grid gap-4 md:grid-cols-3 lg:grid-cols-1 xl:grid-cols-3">
              {AI_REPORT_PRICING_PLANS.map((plan) => (
                <article key={plan.key} className="relative rounded-2xl border border-border bg-card p-5">
                  {plan.badge && <span className="absolute right-4 top-4 rounded-full bg-primary/10 px-2.5 py-1 text-xs font-medium text-primary">{plan.badge}</span>}
                  <h3 className="text-base font-semibold text-foreground">{plan.name}</h3>
                  <div className="mt-4 text-3xl font-bold text-foreground">{plan.price}</div>
                  <div className="mt-2 text-sm text-foreground-muted">{plan.quota} · {plan.unitPrice}</div>
                  <p className="mt-3 text-sm leading-6 text-foreground-dim">{plan.description}</p>
                  <a href="#wechat" className="mt-5 block rounded-xl bg-primary px-4 py-2.5 text-center text-sm font-semibold text-black hover:bg-primary/90">添加微信购买</a>
                </article>
              ))}
            </div>
          </div>
          <div id="wechat" className="rounded-3xl border border-border bg-card p-6">
            <h2 className="text-2xl font-bold text-foreground">添加工作人员微信</h2>
            <p className="mt-3 text-sm leading-6 text-foreground-muted">添加时请备注：AI研报 + 股票名称 / 代码。工作人员会确认需求、付款方式和预计交付时间。</p>
            <div className="mt-5 grid gap-5 sm:grid-cols-[180px_1fr] sm:items-center">
              <div className="flex aspect-square items-center justify-center rounded-2xl border border-border bg-background p-3">
                {serviceConfig?.wechat_qr_image_url ? <img src={serviceConfig.wechat_qr_image_url} alt="工作人员微信二维码" className="h-full w-full rounded-xl object-contain" /> : <span className="text-center text-sm text-foreground-dim">后台暂未配置二维码</span>}
              </div>
              <div className="space-y-3 text-sm text-foreground-muted">
                <div>微信号：<span className="font-semibold text-primary">{serviceConfig?.wechat_id || '后台暂未配置'}</span></div>
                <div>交付时效：{deliveryText}</div>
                <div>服务方式：微信人工确认、人工交付。</div>
              </div>
            </div>
          </div>
        </section>

        <section className="mt-12 rounded-3xl border border-negative/25 bg-negative/10 p-6">
          <h2 className="text-xl font-bold text-foreground">风险提示与免责声明</h2>
          <p className="mt-3 text-sm leading-7 text-foreground-muted">{riskText}</p>
        </section>

        <section className="mt-12 grid gap-4 md:grid-cols-4">
          {SERVICE_STEPS.map(([title, desc], index) => (
            <div key={title} className="rounded-2xl border border-border bg-card p-5">
              <div className="text-xs text-primary">步骤 {index + 1}</div>
              <h3 className="mt-2 text-base font-semibold text-foreground">{title}</h3>
              <p className="mt-2 text-sm leading-6 text-foreground-muted">{desc}</p>
            </div>
          ))}
        </section>
      </main>

      <ReportPreviewModal report={selectedReport} preview={preview} loading={previewLoading} error={previewError} riskText={riskText} serviceConfig={serviceConfig} onClose={() => setSelectedReport(null)} />
    </>
  )
}
