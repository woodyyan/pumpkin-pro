import { useEffect, useMemo, useState } from 'react'

import { AI_REPORT_MARKET_OPTIONS, DEFAULT_AI_REPORT_DELIVERY_TEXT, DEFAULT_AI_REPORT_RISK_DISCLAIMER, getAIReportMarketLabel } from '../../lib/ai-reports'
import { adminFetch, handleAdminActionError, useAdminResource } from '../../lib/admin-data'

const EMPTY_REPORT_FORM = {
  stock_name: '',
  symbol: '',
  exchange: 'SZSE',
  source_trade_date: '',
  image_original_key: '',
  image_preview_key: '',
  image_thumbnail_key: '',
}

function AdminField({ label, children, hint }) {
  return (
    <label className="block">
      <span className="text-xs text-foreground-dim">{label}</span>
      <div className="mt-1.5">{children}</div>
      {hint && <span className="mt-1 block text-xs text-foreground-disabled">{hint}</span>}
    </label>
  )
}

function inputClass() {
  return 'w-full rounded-xl border border-border bg-background-alt px-3 py-2 text-sm text-foreground outline-none transition placeholder:text-foreground-disabled focus:border-primary focus:bg-[var(--color-bg-hover)]'
}

function textareaClass() {
  return `${inputClass()} min-h-[96px] resize-y leading-6`
}

export default function AIReportsAdminPanel({ onUnauthorized }) {
  const reportsResource = useAdminResource({
    key: 'admin-ai-reports',
    request: () => adminFetch('/api/admin/ai-reports'),
    onUnauthorized,
    errorMessage: '加载 AI研报列表失败',
  })
  const configResource = useAdminResource({
    key: 'admin-ai-report-service-config',
    request: () => adminFetch('/api/admin/ai-report-service-config'),
    onUnauthorized,
    errorMessage: '加载 AI研报服务配置失败',
  })

  const [form, setForm] = useState(EMPTY_REPORT_FORM)
  const [editingId, setEditingId] = useState('')
  const [reportError, setReportError] = useState('')
  const [reportSaving, setReportSaving] = useState(false)
  const [configDraft, setConfigDraft] = useState({
    wechat_id: '',
    wechat_qr_image_key: '',
    delivery_time_text: DEFAULT_AI_REPORT_DELIVERY_TEXT,
    risk_disclaimer: DEFAULT_AI_REPORT_RISK_DISCLAIMER,
  })
  const [configError, setConfigError] = useState('')
  const [configSaving, setConfigSaving] = useState(false)

  const reports = useMemo(() => Array.isArray(reportsResource.data?.items) ? reportsResource.data.items : [], [reportsResource.data])

  useEffect(() => {
    if (!configResource.data) return
    setConfigDraft({
      wechat_id: configResource.data.wechat_id || '',
      wechat_qr_image_key: configResource.data.wechat_qr_image_key || '',
      delivery_time_text: configResource.data.delivery_time_text || DEFAULT_AI_REPORT_DELIVERY_TEXT,
      risk_disclaimer: configResource.data.risk_disclaimer || DEFAULT_AI_REPORT_RISK_DISCLAIMER,
    })
  }, [configResource.data])

  const updateForm = (key, value) => setForm((current) => ({ ...current, [key]: value }))
  const updateConfig = (key, value) => setConfigDraft((current) => ({ ...current, [key]: value }))

  const resetForm = () => {
    setForm(EMPTY_REPORT_FORM)
    setEditingId('')
    setReportError('')
  }

  const editReport = (item) => {
    setEditingId(item.id)
    setForm({
      stock_name: item.stock_name || '',
      symbol: item.symbol || '',
      exchange: item.exchange || 'SZSE',
      source_trade_date: item.source_trade_date || '',
      image_original_key: item.image_original_key || '',
      image_preview_key: item.image_preview_key || '',
      image_thumbnail_key: item.image_thumbnail_key || '',
    })
    setReportError('')
  }

  const saveReport = async (event) => {
    event.preventDefault()
    setReportSaving(true)
    setReportError('')
    try {
      const path = editingId ? `/api/admin/ai-reports/${encodeURIComponent(editingId)}` : '/api/admin/ai-reports'
      await adminFetch(path, {
        method: editingId ? 'PUT' : 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(form),
      })
      resetForm()
      reportsResource.refresh()
    } catch (err) {
      setReportError(handleAdminActionError(err, onUnauthorized, '保存 AI研报失败'))
    } finally {
      setReportSaving(false)
    }
  }

  const deleteReport = async (item) => {
    if (!window.confirm(`确认删除 ${item.stock_name} 的 AI研报样例？`)) return
    setReportError('')
    try {
      await adminFetch(`/api/admin/ai-reports/${encodeURIComponent(item.id)}`, { method: 'DELETE' })
      reportsResource.refresh()
    } catch (err) {
      setReportError(handleAdminActionError(err, onUnauthorized, '删除 AI研报失败'))
    }
  }

  const saveConfig = async (event) => {
    event.preventDefault()
    setConfigSaving(true)
    setConfigError('')
    try {
      await adminFetch('/api/admin/ai-report-service-config', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(configDraft),
      })
      configResource.refresh()
    } catch (err) {
      setConfigError(handleAdminActionError(err, onUnauthorized, '保存 AI研报服务配置失败'))
    } finally {
      setConfigSaving(false)
    }
  }

  return (
    <section className="rounded-2xl border border-border bg-card p-4 sm:p-5">
      <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h2 className="text-base font-semibold text-foreground-muted">AI研报管理</h2>
          <p className="mt-1 text-xs text-foreground-dim">三个字段都填同一张原图的 COS 对象 Key（如 ai-reports/2026/xxx.png），无需完整 URL。系统会用后台 COS 密钥生成带签名的临时链接，并自动追加数据万象处理参数：预览图走 imageSlim 压缩、缩略图缩放至 30%。未登录只看缩略图，登录用户弹窗预览，不提供原图下载。</p>
        </div>
        <button type="button" onClick={() => reportsResource.refresh()} className="rounded-xl border border-border px-3 py-2 text-xs font-medium text-foreground-muted hover:border-primary hover:text-primary">刷新</button>
      </div>

      <div className="mt-5 grid gap-5 xl:grid-cols-[minmax(0,1fr)_360px]">
        <div className="overflow-hidden rounded-2xl border border-border">
          {reportsResource.loading ? (
            <div className="p-5 text-sm text-foreground-muted">正在加载 AI研报列表...</div>
          ) : reportsResource.error ? (
            <div className="p-5 text-sm text-negative">{reportsResource.error}</div>
          ) : reports.length === 0 ? (
            <div className="p-5 text-sm text-foreground-muted">暂无研报样例。请先新增一条记录。</div>
          ) : (
            <div className="overflow-x-auto">
              <table className="min-w-full text-left text-xs">
                <thead className="border-b border-border bg-[var(--color-bg-hover)] text-foreground-dim">
                  <tr>
                    <th className="px-3 py-2">缩略图</th>
                    <th className="px-3 py-2">股票</th>
                    <th className="px-3 py-2">市场</th>
                    <th className="px-3 py-2">数据截至</th>
                    <th className="px-3 py-2">更新时间</th>
                    <th className="px-3 py-2">操作</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-border text-foreground-muted">
                  {reports.map((item) => (
                    <tr key={item.id}>
                      <td className="px-3 py-2">
                        <div className="h-14 w-20 overflow-hidden rounded-lg border border-border bg-background">
                          {item.thumbnail_url ? <img src={item.thumbnail_url} alt="" className="h-full w-full object-cover object-top" /> : null}
                        </div>
                      </td>
                      <td className="px-3 py-2"><div className="font-medium text-foreground">{item.stock_name}</div><div>{item.symbol}</div></td>
                      <td className="px-3 py-2">{getAIReportMarketLabel(item.exchange)}</td>
                      <td className="px-3 py-2">{item.source_trade_date || '--'}</td>
                      <td className="px-3 py-2">{item.updated_at || '--'}</td>
                      <td className="px-3 py-2">
                        <div className="flex flex-wrap gap-2">
                          <button type="button" onClick={() => editReport(item)} className="rounded-lg border border-border px-2 py-1 hover:border-primary hover:text-primary">编辑</button>
                          <button type="button" onClick={() => deleteReport(item)} className="rounded-lg border border-negative/25 px-2 py-1 text-negative hover:bg-negative/10">删除</button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>

        <form onSubmit={saveReport} className="rounded-2xl border border-border bg-background p-4">
          <div className="mb-4 flex items-center justify-between">
            <h3 className="text-sm font-semibold text-foreground">{editingId ? '编辑研报' : '新增研报'}</h3>
            {editingId && <button type="button" onClick={resetForm} className="text-xs text-foreground-dim hover:text-primary">取消编辑</button>}
          </div>
          <div className="space-y-3">
            <AdminField label="股票名称"><input className={inputClass()} value={form.stock_name} onChange={(e) => updateForm('stock_name', e.target.value)} placeholder="例如：宁德时代" /></AdminField>
            <div className="grid gap-3 sm:grid-cols-2">
              <AdminField label="股票代码"><input className={inputClass()} value={form.symbol} onChange={(e) => updateForm('symbol', e.target.value)} placeholder="300750" /></AdminField>
              <AdminField label="市场"><select className={inputClass()} value={form.exchange} onChange={(e) => updateForm('exchange', e.target.value)}>{AI_REPORT_MARKET_OPTIONS.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</select></AdminField>
            </div>
            <AdminField label="数据截至交易日"><input type="date" className={inputClass()} value={form.source_trade_date} onChange={(e) => updateForm('source_trade_date', e.target.value)} /></AdminField>
            <AdminField label="原图 COS Key" hint="仅后台管理和人工交付使用，用户侧不返回原图地址。填 COS 对象 Key 即可，无需完整 URL。"><input className={inputClass()} value={form.image_original_key} onChange={(e) => updateForm('image_original_key', e.target.value)} placeholder="ai-reports/2026/xxx.png" /></AdminField>
            <AdminField label="预览图 COS Key" hint="填原图 COS Key 即可，系统自动追加 imageSlim 压缩，无需填处理参数或完整 URL。"><input className={inputClass()} value={form.image_preview_key} onChange={(e) => updateForm('image_preview_key', e.target.value)} placeholder="ai-reports/2026/xxx.png" /></AdminField>
            <AdminField label="缩略图 COS Key" hint="填原图 COS Key 即可，系统自动追加 imageMogr2/thumbnail/!30p，无需填处理参数或完整 URL。"><input className={inputClass()} value={form.image_thumbnail_key} onChange={(e) => updateForm('image_thumbnail_key', e.target.value)} placeholder="ai-reports/2026/xxx.png" /></AdminField>
          </div>
          {reportError && <div className="mt-3 rounded-xl bg-negative/10 px-3 py-2 text-xs text-negative">{reportError}</div>}
          <button type="submit" disabled={reportSaving} className="mt-4 w-full rounded-xl bg-primary px-4 py-2.5 text-sm font-semibold text-black hover:bg-primary/90 disabled:opacity-60">{reportSaving ? '保存中...' : editingId ? '保存修改' : '新增研报'}</button>
        </form>
      </div>

      <form onSubmit={saveConfig} className="mt-6 rounded-2xl border border-border bg-background p-4">
        <h3 className="text-sm font-semibold text-foreground">微信二维码与服务配置</h3>
        <div className="mt-4 grid gap-4 lg:grid-cols-[1fr_180px]">
          <div className="space-y-3">
            <AdminField label="工作人员微信号"><input className={inputClass()} value={configDraft.wechat_id} onChange={(e) => updateConfig('wechat_id', e.target.value)} placeholder="请输入微信号" /></AdminField>
            <AdminField label="微信二维码 COS Key" hint="填 COS 对象 Key 即可，无需完整 URL。"><input className={inputClass()} value={configDraft.wechat_qr_image_key} onChange={(e) => updateConfig('wechat_qr_image_key', e.target.value)} placeholder="ai-reports/service/wechat-qr.png" /></AdminField>
            <AdminField label="交付时效说明"><textarea className={textareaClass()} value={configDraft.delivery_time_text} onChange={(e) => updateConfig('delivery_time_text', e.target.value)} /></AdminField>
            <AdminField label="风险提示与免责声明"><textarea className={textareaClass()} value={configDraft.risk_disclaimer} onChange={(e) => updateConfig('risk_disclaimer', e.target.value)} /></AdminField>
          </div>
          <div className="rounded-2xl border border-border bg-card p-3">
            <div className="flex aspect-square items-center justify-center rounded-xl border border-border bg-background p-2">
              {configResource.data?.wechat_qr_image_url ? <img src={configResource.data.wechat_qr_image_url} alt="微信二维码预览" className="h-full w-full object-contain" /> : <span className="text-center text-xs text-foreground-dim">保存后显示二维码预览</span>}
            </div>
            <div className="mt-3 text-xs leading-5 text-foreground-dim">用户侧会展示该二维码和微信号。页面不展示额度核销、订单和支付状态。</div>
          </div>
        </div>
        {configError && <div className="mt-3 rounded-xl bg-negative/10 px-3 py-2 text-xs text-negative">{configError}</div>}
        <button type="submit" disabled={configSaving} className="mt-4 rounded-xl bg-primary px-4 py-2.5 text-sm font-semibold text-black hover:bg-primary/90 disabled:opacity-60">{configSaving ? '保存中...' : '保存服务配置'}</button>
      </form>
    </section>
  )
}
