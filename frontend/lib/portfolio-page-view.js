export function getPortfolioPageViewState({ loading = false, data = null } = {}) {
  const hasDashboardData = Boolean(data)

  return {
    hasDashboardData,
    initialLoading: Boolean(loading && !hasDashboardData),
    refreshing: Boolean(loading && hasDashboardData),
  }
}
