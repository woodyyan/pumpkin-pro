export function resolveAdminSectionApis(section) {
  if (section === 'overview') {
    return [
      '/api/admin/stats',
      '/api/admin/analytics',
      '/api/admin/device-analytics',
      '/api/admin/user-funnel',
    ]
  }
  if (section === 'data') {
    return [
      '/api/admin/company-profiles',
      '/api/admin/factor-lab/pipeline/status',
      '/api/admin/quadrant-overview',
      '/api/admin/quadrant-logs',
      '/api/admin/compute-status',
      '/api/admin/ranking-portfolio-status',
    ]
  }
  if (section === 'ai') {
    return [
      '/api/admin/ai-config',
      '/api/admin/stats',
      '/api/admin/ai-usage',
      '/api/admin/ai-picker/status',
    ]
  }
  if (section === 'ops') {
    return [
      '/api/admin/payments/config',
      '/api/admin/payments',
      '/api/admin/backup-status',
      '/api/admin/backup-history',
      '/api/admin/backup-stats',
      '/api/admin/system-health',
      '/api/admin/feedback',
    ]
  }
  return []
}
