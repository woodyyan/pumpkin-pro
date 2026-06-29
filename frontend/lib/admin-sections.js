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
      '/api/admin/portfolio-tracking/status',
    ]
  }
  if (section === 'ai') {
    return [
      '/api/admin/ai-config',
      '/api/admin/stats',
      '/api/admin/ai-usage',
      '/api/admin/ai-reports',
      '/api/admin/ai-report-service-config',
      '/api/admin/ai-picker/status',
      '/api/admin/ai-picker/latest-run',
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
