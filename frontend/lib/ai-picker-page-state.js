export function markMarketLoadAttempted(prev, market) {
  if (!market || prev?.[market]) {
    return prev || {}
  }
  return {
    ...(prev || {}),
    [market]: true,
  }
}

export function shouldAutoLoadAIPickerMarket({ market, attemptedByMarket, loadingByMarket }) {
  if (!market) return false
  if (loadingByMarket?.[market]) return false
  return !attemptedByMarket?.[market]
}
