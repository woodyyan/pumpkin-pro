export const BACKUP_TRIGGER_LABELS = {
  quadrant_callback: '四象限回调',
  scheduled_fallback: '保底定时',
  manual: '手动触发',
}

export const BACKUP_STATUS_COLORS = {
  success: 'text-emerald-400',
  partial: 'text-amber-400',
  failed: 'text-rose-400',
  skipped: 'text-white/40',
  never: 'text-white/30',
}

export const BACKUP_COS_STATUS_META = {
  disabled: { label: '未配置', tone: 'text-white/40', symbol: '⏸' },
  never: { label: '待首次同步', tone: 'text-white/45', symbol: '…' },
  pending: { label: '待上传', tone: 'text-sky-300', symbol: '…' },
  uploading: { label: '上传中', tone: 'text-sky-300', symbol: '⇪' },
  success: { label: '已同步', tone: 'text-emerald-400', symbol: '✅' },
  partial: { label: '部分同步', tone: 'text-amber-400', symbol: '⚠' },
  failed: { label: '同步失败', tone: 'text-rose-400', symbol: '✕' },
  skipped: { label: '已跳过', tone: 'text-white/40', symbol: '-' },
}

export function formatBackupBytes(bytes) {
  if (bytes == null) return '--'
  if (bytes < 1024) return `${bytes}B`
  if (bytes < 1048576) return `${(bytes / 1024).toFixed(1)}KB`
  if (bytes < 1073741824) return `${(bytes / 1048576).toFixed(1)}MB`
  return `${(bytes / 1073741824).toFixed(2)}GB`
}

export function formatBackupDuration(ms) {
  if (ms == null || ms === 0) return '--'
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(1)}s`
}

export function getBackupCosMeta(status) {
  return BACKUP_COS_STATUS_META[status] || { label: status || '--', tone: 'text-white/45', symbol: '·' }
}

export function buildBackupStatusCards(status, stats) {
  const cosMeta = getBackupCosMeta(status?.cos_status)
  return {
    overall: {
      value: status?.status ?? '--',
      sub: BACKUP_TRIGGER_LABELS[status?.last_trigger_type] || '',
    },
    sizes: {
      pumpkin: formatBackupBytes(status?.pumpkin_size_bytes),
      cacheA: formatBackupBytes(status?.cache_a_size_bytes),
      cacheHK: formatBackupBytes(status?.cache_hk_size_bytes),
    },
    cos: {
      value: `${cosMeta.symbol} ${cosMeta.label}`,
      sub: stats?.cloud_enabled ? '已配置' : '未配置',
      tone: cosMeta.tone,
    },
    duration: formatBackupDuration(status?.duration_ms),
  }
}

export function buildBackupJobBanner(status) {
  if (!status) return null
  const jobStatus = status.current_job_status || 'idle'
  if (jobStatus === 'idle') {
    if (status.next_allowed_at) {
      return { tone: 'muted', text: `冷却中，下一次可触发时间 ${status.next_allowed_at}` }
    }
    return null
  }
  if (jobStatus === 'queued' || jobStatus === 'running') {
    return {
      tone: 'info',
      text: `${BACKUP_TRIGGER_LABELS[status.current_job_trigger_type] || '后台任务'}进行中：${status.current_job_message || '正在执行'}`,
    }
  }
  if (jobStatus === 'success' || jobStatus === 'partial' || jobStatus === 'failed') {
    return {
      tone: jobStatus === 'success' ? 'success' : jobStatus === 'partial' ? 'warning' : 'danger',
      text: `${BACKUP_TRIGGER_LABELS[status.current_job_trigger_type] || '最近任务'}已结束：${status.current_job_message || jobStatus}`,
    }
  }
  return null
}

export function shouldPollBackupStatus(status) {
  const jobStatus = status?.current_job_status
  return jobStatus === 'queued' || jobStatus === 'running'
}

export function resolveBackupTriggerButton({ triggering = false, status = null } = {}) {
  if (triggering) {
    return { disabled: true, label: '提交中...' }
  }
  if (shouldPollBackupStatus(status)) {
    return { disabled: true, label: '后台执行中...' }
  }
  if (status?.next_allowed_at) {
    return { disabled: true, label: '冷却中...' }
  }
  return { disabled: false, label: '🔄 立即备份' }
}

export function buildBackupHistoryNote(row) {
  if (!row) return '-'
  if (row.error_msg) return row.error_msg
  if (row.cos_status && row.cos_status !== 'success' && row.cos_error_msg) return row.cos_error_msg
  if (row.integrity_check === 'ok') return '✅ 校验通过'
  if (row.integrity_check && row.integrity_check !== 'skipped') return `校验${row.integrity_check}`
  return '-'
}
