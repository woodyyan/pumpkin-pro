package quadrant

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

// rankingPortfolioCutoverDefault is the default start date for sim portfolio
// recompute when no explicit from_date is provided. It matches the open-entry
// pricing cutover date (RANKING_PORTFOLIO_CUTOVER_DATE) to avoid scanning the
// entire ranking snapshot history back to the earliest available date.
const rankingPortfolioCutoverDefault = "2026-06-10"

type simPortfolioSignalItem struct {
	Rank            int
	Code            string
	Name            string
	Exchange        string
	Board           string
	ConsecutiveDays int
}

type simPortfolioHolding struct {
	Rank        int
	Code        string
	Name        string
	Exchange    string
	Weight      float64
	Shares      float64
	BuyPrice    float64
	TargetValue float64
}

type simPortfolioStatus struct {
	LatestSignalDate       string
	PendingSignalDate      string
	NextEntryTradeDate     string
	Status                 string
	StatusText             string
	MissingOpenPriceCount  int
	MissingClosePriceCount int
}

func resolveSimPortfolioExchangeCodes(exchange string) []string {
	switch strings.ToUpper(strings.TrimSpace(exchange)) {
	case "HKEX":
		return []string{"HKEX"}
	default:
		return []string{"SSE", "SZSE"}
	}
}

func inferSimPortfolioBoard(code string, exchange string) string {
	normalizedExchange := strings.ToUpper(strings.TrimSpace(exchange))
	normalizedCode := strings.TrimSpace(code)
	if normalizedExchange == "HKEX" {
		return "HK_MAIN"
	}
	if strings.HasPrefix(normalizedCode, "688") || strings.HasPrefix(normalizedCode, "689") {
		return aShareBoardStar
	}
	if strings.HasPrefix(normalizedCode, "300") || strings.HasPrefix(normalizedCode, "301") {
		return aShareBoardChiNext
	}
	if strings.HasPrefix(normalizedCode, "8") || strings.HasPrefix(normalizedCode, "4") {
		return "BSE"
	}
	return aShareBoardMain
}

func roundTo(value float64, digits int) float64 {
	factor := math.Pow10(digits)
	return math.Round(value*factor) / factor
}

func sameSimPortfolioTradeDate(values []string) string {
	resolved := ""
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return ""
		}
		if resolved == "" {
			resolved = value
			continue
		}
		if resolved != value {
			return ""
		}
	}
	return resolved
}

func decodeSimPortfolioExcludedBoards(raw string) map[string]struct{} {
	out := map[string]struct{}{}
	for key := range decodeRankingPortfolioExcludedBoards(raw) {
		out[strings.ToUpper(strings.TrimSpace(key))] = struct{}{}
	}
	return out
}

func (s *Service) selectSimPortfolioSignal(ctx context.Context, definition RankingPortfolioDefinition, signalDate string) ([]simPortfolioSignalItem, string, error) {
	limit := definition.SelectionWindow
	if limit < definition.MaxHoldings {
		limit = definition.MaxHoldings
	}
	if limit < 20 {
		limit = 20
	}
	rows, err := s.repo.ListRankingSnapshotsByDate(ctx, definition.Exchange, signalDate, limit)
	if err != nil {
		return nil, "", err
	}
	excludedBoards := decodeSimPortfolioExcludedBoards(definition.ExcludedBoards)
	items := make([]simPortfolioSignalItem, 0, len(rows))
	for _, row := range rows {
		board := inferSimPortfolioBoard(row.Code, row.Exchange)
		if len(excludedBoards) > 0 {
			if _, excluded := excludedBoards[strings.ToUpper(strings.TrimSpace(board))]; excluded {
				continue
			}
		}
		item := simPortfolioSignalItem{
			Rank:     row.Rank,
			Code:     strings.TrimSpace(row.Code),
			Name:     strings.TrimSpace(row.Name),
			Exchange: strings.ToUpper(strings.TrimSpace(row.Exchange)),
			Board:    board,
		}
		if definition.SelectionRule == rankingPortfolioSelectionRuleTop10ByStreak {
			days, streakErr := s.repo.GetConsecutiveDaysAsOf(ctx, item.Code, resolveSimPortfolioExchangeCodes(definition.Exchange), signalDate)
			if streakErr != nil {
				return nil, "", streakErr
			}
			item.ConsecutiveDays = days
		}
		items = append(items, item)
	}
	if definition.SelectionWindow > 0 && len(items) > definition.SelectionWindow {
		items = items[:definition.SelectionWindow]
	}
	selected := append([]simPortfolioSignalItem(nil), items...)
	if definition.SelectionRule == rankingPortfolioSelectionRuleTop10ByStreak {
		sort.SliceStable(selected, func(i, j int) bool {
			if selected[i].ConsecutiveDays == selected[j].ConsecutiveDays {
				if selected[i].Rank == selected[j].Rank {
					return selected[i].Code < selected[j].Code
				}
				return selected[i].Rank < selected[j].Rank
			}
			return selected[i].ConsecutiveDays > selected[j].ConsecutiveDays
		})
	}
	if len(selected) > definition.MaxHoldings {
		selected = selected[:definition.MaxHoldings]
	}
	if len(selected) < definition.MaxHoldings {
		return selected, defaultRankingPortfolioWarningText, nil
	}
	for index := range selected {
		selected[index].Rank = index + 1
	}
	return selected, "", nil
}

func (s *Service) ensureSimPortfolioBaseline(ctx context.Context, definition RankingPortfolioDefinition, baselineDate string) error {
	baselineDate = strings.TrimSpace(baselineDate)
	if baselineDate == "" {
		return nil
	}
	existing, err := s.repo.GetSimPortfolioDailyByTradeDate(ctx, definition.ID, baselineDate)
	if err != nil || existing != nil {
		return err
	}
	now := time.Now().UTC()
	row := SimPortfolioDaily{
		PortfolioID:     definition.ID,
		TradeDate:       baselineDate,
		SignalDate:      "",
		SourceTradeDate: baselineDate,
		NAV:             1,
		TotalAssets:     simPortfolioInitialAssets,
		PreviousAssets:  simPortfolioInitialAssets,
		DailyReturn:     0,
		TotalReturn:     0,
		PositionCount:   0,
		Rebalance:       false,
		Status:          simPortfolioStatusSeeded,
		WarningText:     "",
		ComputedAt:      now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	return s.repo.db.WithContext(ctx).Create(&row).Error
}

func (s *Service) replaceSimPortfolioTradeDate(ctx context.Context, definitionID string, tradeDate string, daily SimPortfolioDaily, positions []SimPortfolioPosition, trades []SimPortfolioTrade) error {
	return s.repo.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("portfolio_id = ? AND trade_date = ?", definitionID, tradeDate).Delete(&SimPortfolioPosition{}).Error; err != nil {
			return err
		}
		if err := tx.Where("portfolio_id = ? AND trade_date = ?", definitionID, tradeDate).Delete(&SimPortfolioTrade{}).Error; err != nil {
			return err
		}
		if err := tx.Where("portfolio_id = ? AND trade_date = ?", definitionID, tradeDate).Delete(&SimPortfolioMetrics{}).Error; err != nil {
			return err
		}
		if err := tx.Where("portfolio_id = ? AND trade_date = ?", definitionID, tradeDate).Delete(&SimPortfolioDaily{}).Error; err != nil {
			return err
		}
		if err := tx.Create(&daily).Error; err != nil {
			return err
		}
		if len(positions) > 0 {
			if err := tx.Create(&positions).Error; err != nil {
				return err
			}
		}
		if len(trades) > 0 {
			if err := tx.Create(&trades).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func buildSimPortfolioMetrics(dailyRows []SimPortfolioDaily, trades []SimPortfolioTrade) SimPortfolioMetrics {
	metrics := SimPortfolioMetrics{NAV: 1}
	if len(dailyRows) == 0 {
		return metrics
	}
	latest := dailyRows[len(dailyRows)-1]
	metrics.NAV = latest.NAV
	tradeDailyReturns := make([]float64, 0, len(dailyRows))
	peak := 0.0
	maxDrawdown := 0.0
	positiveDays := 0
	turnoverByDate := map[string]float64{}
	for _, trade := range trades {
		turnoverByDate[trade.TradeDate] += math.Abs(trade.NewWeight - trade.OldWeight)
	}
	for index, row := range dailyRows {
		if row.NAV > peak {
			peak = row.NAV
		}
		if peak > 0 {
			drawdown := row.NAV/peak - 1
			if drawdown < maxDrawdown {
				maxDrawdown = drawdown
			}
		}
		if index == 0 || row.PositionCount == 0 {
			continue
		}
		tradeDailyReturns = append(tradeDailyReturns, row.DailyReturn)
		if row.DailyReturn > 0 {
			positiveDays++
		}
	}
	metrics.MaxDrawdown = maxDrawdown
	if len(tradeDailyReturns) > 0 {
		var sum float64
		for _, value := range tradeDailyReturns {
			sum += value
		}
		mean := sum / float64(len(tradeDailyReturns))
		var variance float64
		for _, value := range tradeDailyReturns {
			delta := value - mean
			variance += delta * delta
		}
		variance /= float64(len(tradeDailyReturns))
		metrics.Volatility = math.Sqrt(math.Max(variance, 0)) * math.Sqrt(rankingPortfolioAnnualTradingDays)
		metrics.WinRate = float64(positiveDays) / float64(len(tradeDailyReturns))
		metrics.AnnualReturn = math.Pow(latest.NAV, float64(rankingPortfolioAnnualTradingDays)/float64(len(tradeDailyReturns))) - 1
	}
	if len(turnoverByDate) > 0 {
		var totalTurnover float64
		for _, value := range turnoverByDate {
			totalTurnover += value / 2
		}
		metrics.TurnoverRate = totalTurnover / float64(len(turnoverByDate))
	}
	return metrics
}

func (s *Service) refreshSimPortfolioMetrics(ctx context.Context, definitionID string, tradeDate string) error {
	dailyRows, err := s.repo.ListAllSimPortfolioDaily(ctx, definitionID)
	if err != nil {
		return err
	}
	tradeRows, err := s.repo.ListAllSimPortfolioTrades(ctx, definitionID)
	if err != nil {
		return err
	}
	metrics := buildSimPortfolioMetrics(dailyRows, tradeRows)
	now := time.Now().UTC()
	metrics.PortfolioID = definitionID
	metrics.TradeDate = tradeDate
	metrics.CreatedAt = now
	metrics.UpdatedAt = now
	return s.repo.db.WithContext(ctx).Create(&metrics).Error
}

func (s *Service) computeAndPersistSimPortfolioDay(ctx context.Context, definition RankingPortfolioDefinition, signalDate string, tradeDate string, previousDaily *SimPortfolioDaily) error {
	if previousDaily == nil {
		return fmt.Errorf("missing previous daily for %s", definition.ID)
	}
	signalItems, warningText, err := s.selectSimPortfolioSignal(ctx, definition, signalDate)
	if err != nil {
		return err
	}
	if len(signalItems) < definition.MaxHoldings {
		return fmt.Errorf("signal shortfall on %s: %s", signalDate, warningText)
	}
	previousPositions, err := s.repo.ListSimPortfolioPositionsByTradeDate(ctx, definition.ID, previousDaily.TradeDate)
	if err != nil {
		return err
	}
	previousMap := make(map[string]SimPortfolioPosition, len(previousPositions))
	for _, item := range previousPositions {
		previousMap[snapshotPriceHintKey(item.StockCode, item.Exchange)] = item
	}
	currentAssets := previousDaily.TotalAssets
	if currentAssets <= 0 {
		currentAssets = simPortfolioInitialAssets
	}
	targetValue := currentAssets * simPortfolioTargetWeight
	positions := make([]SimPortfolioPosition, 0, len(signalItems))
	selectedMap := make(map[string]simPortfolioHolding, len(signalItems))
	sourceTradeDate := tradeDate
	now := time.Now().UTC()
	for _, item := range signalItems {
		openPrice, _, priceErr := s.repo.GetRankingPortfolioSelectionOpenPrice(ctx, definition.ID, signalDate, item.Code, item.Exchange)
		if priceErr != nil {
			return priceErr
		}
		if openPrice <= 0 && s.openPriceResolver != nil {
			openPrice = s.openPriceResolver(ctx, item.Code, item.Exchange, tradeDate)
		}
		if openPrice <= 0 {
			return fmt.Errorf("missing open price for %s on %s", item.Code, tradeDate)
		}
		closePrice, closeErr := s.repo.GetClosePriceByTradeDate(ctx, item.Code, item.Exchange, tradeDate)
		if closeErr != nil {
			return closeErr
		}
		if closePrice <= 0 {
			return fmt.Errorf("missing close price for %s on %s", item.Code, tradeDate)
		}
		shares := targetValue / openPrice
		marketValue := shares * closePrice
		profit := marketValue - targetValue
		profitRate := 0.0
		if targetValue > 0 {
			profitRate = profit / targetValue
		}
		positions = append(positions, SimPortfolioPosition{
			PortfolioID:     definition.ID,
			TradeDate:       tradeDate,
			SignalDate:      signalDate,
			StockCode:       item.Code,
			StockName:       item.Name,
			Exchange:        item.Exchange,
			Rank:            item.Rank,
			Weight:          simPortfolioTargetWeight,
			TargetValue:     targetValue,
			Shares:          shares,
			BuyPrice:        openPrice,
			ClosePrice:      closePrice,
			MarketValue:     marketValue,
			Profit:          profit,
			ProfitRate:      profitRate,
			SourceTradeDate: sourceTradeDate,
			CreatedAt:       now,
			UpdatedAt:       now,
		})
		selectedMap[snapshotPriceHintKey(item.Code, item.Exchange)] = simPortfolioHolding{
			Rank:        item.Rank,
			Code:        item.Code,
			Name:        item.Name,
			Exchange:    item.Exchange,
			Weight:      simPortfolioTargetWeight,
			Shares:      shares,
			BuyPrice:    openPrice,
			TargetValue: targetValue,
		}
	}
	trades := make([]SimPortfolioTrade, 0, len(previousMap)+len(selectedMap))
	keys := make([]string, 0, len(previousMap)+len(selectedMap))
	seen := map[string]struct{}{}
	for key := range previousMap {
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	for key := range selectedMap {
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		prev, hasPrev := previousMap[key]
		next, hasNext := selectedMap[key]
		stockCode := prev.StockCode
		stockName := prev.StockName
		exchange := prev.Exchange
		oldWeight := 0.0
		oldShares := 0.0
		if hasPrev {
			stockCode = prev.StockCode
			stockName = prev.StockName
			exchange = prev.Exchange
			oldWeight = prev.Weight
			oldShares = prev.Shares
		}
		newWeight := 0.0
		newShares := 0.0
		targetTradeValue := 0.0
		action := simPortfolioActionSell
		reason := simPortfolioReasonDropTop4
		if hasNext {
			stockCode = next.Code
			stockName = next.Name
			exchange = next.Exchange
			newWeight = next.Weight
			newShares = next.Shares
			targetTradeValue = next.TargetValue
			action = simPortfolioActionBuy
			reason = simPortfolioReasonEnterTop4
			if hasPrev {
				action = simPortfolioActionHold
				reason = simPortfolioReasonStayTop4
			}
		}
		tradePrice := 0.0
		if hasNext {
			tradePrice = next.BuyPrice
		}
		if tradePrice <= 0 && s.openPriceResolver != nil {
			tradePrice = s.openPriceResolver(ctx, stockCode, exchange, tradeDate)
		}
		trades = append(trades, SimPortfolioTrade{
			PortfolioID: definition.ID,
			TradeDate:   tradeDate,
			SignalDate:  signalDate,
			StockCode:   stockCode,
			StockName:   stockName,
			Exchange:    exchange,
			Action:      action,
			OldWeight:   oldWeight,
			NewWeight:   newWeight,
			TradePrice:  tradePrice,
			TargetValue: targetTradeValue,
			OldShares:   oldShares,
			NewShares:   newShares,
			ShareDelta:  newShares - oldShares,
			Reason:      reason,
			CreatedAt:   now,
			UpdatedAt:   now,
		})
	}
	todayAssets := 0.0
	for _, position := range positions {
		todayAssets += position.MarketValue
	}
	dailyReturn := 0.0
	if previousDaily.TotalAssets > 0 {
		dailyReturn = todayAssets/previousDaily.TotalAssets - 1
	}
	daily := SimPortfolioDaily{
		PortfolioID:     definition.ID,
		TradeDate:       tradeDate,
		SignalDate:      signalDate,
		SourceTradeDate: sourceTradeDate,
		NAV:             roundTo(todayAssets/simPortfolioInitialAssets, simPortfolioNavPrecision),
		TotalAssets:     todayAssets,
		PreviousAssets:  previousDaily.TotalAssets,
		DailyReturn:     dailyReturn,
		TotalReturn:     todayAssets/simPortfolioInitialAssets - 1,
		PositionCount:   len(positions),
		Rebalance:       len(trades) > 0,
		Status:          simPortfolioStatusComplete,
		WarningText:     "",
		ComputedAt:      now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := s.replaceSimPortfolioTradeDate(ctx, definition.ID, tradeDate, daily, positions, trades); err != nil {
		return err
	}
	return s.refreshSimPortfolioMetrics(ctx, definition.ID, tradeDate)
}

func (s *Service) recomputeSimPortfolioDefinition(ctx context.Context, definition RankingPortfolioDefinition, fromDate string, toDate string, reset bool) error {
	// When fromDate is empty, default to the cutover date to avoid scanning
	// the entire ranking snapshot history (which may go back to 2026-04-16).
	if strings.TrimSpace(fromDate) == "" {
		fromDate = rankingPortfolioCutoverDefault
	}
	dates, err := s.repo.ListRankingSnapshotDatesByExchangeRange(ctx, definition.Exchange, fromDate, toDate)
	if err != nil {
		return err
	}
	if len(dates) == 0 {
		return nil
	}
	if reset {
		if err := s.repo.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("portfolio_id = ?", definition.ID).Delete(&SimPortfolioPosition{}).Error; err != nil {
				return err
			}
			if err := tx.Where("portfolio_id = ?", definition.ID).Delete(&SimPortfolioTrade{}).Error; err != nil {
				return err
			}
			if err := tx.Where("portfolio_id = ?", definition.ID).Delete(&SimPortfolioMetrics{}).Error; err != nil {
				return err
			}
			if err := tx.Where("portfolio_id = ?", definition.ID).Delete(&SimPortfolioDaily{}).Error; err != nil {
				return err
			}
			return nil
		}); err != nil {
			return err
		}
	}
	if err := s.ensureSimPortfolioBaseline(ctx, definition, dates[0]); err != nil {
		return err
	}
	previousDaily, err := s.repo.GetLatestSimPortfolioDaily(ctx, definition.ID)
	if err != nil {
		return err
	}
	for index := 0; index < len(dates)-1; index++ {
		signalDate := dates[index]
		tradeDate := dates[index+1]
		if previousDaily != nil {
			if previousDaily.SignalDate != "" && previousDaily.SignalDate >= signalDate {
				continue
			}
			if previousDaily.SignalDate == "" && previousDaily.TradeDate > signalDate {
				continue
			}
		}
		if err := s.computeAndPersistSimPortfolioDay(ctx, definition, signalDate, tradeDate, previousDaily); err != nil {
			return err
		}
		previousDaily, err = s.repo.GetSimPortfolioDailyByTradeDate(ctx, definition.ID, tradeDate)
		if err != nil {
			return err
		}
	}
	return nil
}

func buildSimPortfolioSyncStatusText(generated int, status string) string {
	if generated > 0 {
		return fmt.Sprintf("已生成 %d 个估值日", generated)
	}
	switch status {
	case "initialized":
		return "已初始化基准资金，等待下一批信号形成 T+1 估值"
	case "up_to_date":
		return "事实表已是最新状态"
	case "no_source_snapshot":
		return "暂无可同步的排行榜快照"
	case "no_anchor":
		return "缺少同步锚点，需从头重算"
	case "blocked":
		return "同步被前置数据阻断"
	default:
		return "未生成新的估值日"
	}
}

func (s *Service) syncSimPortfolioDefinition(ctx context.Context, definition RankingPortfolioDefinition) (SimPortfolioSyncSummary, error) {
	summary := SimPortfolioSyncSummary{
		PortfolioID: definition.ID,
		Name:        definition.Name,
		Exchange:    definition.Exchange,
		Status:      "skipped",
	}
	latestSnapshotDate, err := s.repo.GetLatestRankingSnapshotDateByExchange(ctx, definition.Exchange)
	if err != nil {
		summary.Status = "failed"
		summary.Message = err.Error()
		return summary, err
	}
	latestSnapshotDate = strings.TrimSpace(latestSnapshotDate)
	summary.LatestSignalDate = latestSnapshotDate
	if latestSnapshotDate == "" {
		summary.Status = "no_source_snapshot"
		summary.Message = buildSimPortfolioSyncStatusText(0, summary.Status)
		return summary, nil
	}
	latestDaily, err := s.repo.GetLatestSimPortfolioDaily(ctx, definition.ID)
	if err != nil {
		summary.Status = "failed"
		summary.Message = err.Error()
		return summary, err
	}
	if latestDaily == nil {
		if err := s.ensureSimPortfolioBaseline(ctx, definition, latestSnapshotDate); err != nil {
			summary.Status = "failed"
			summary.Message = err.Error()
			return summary, err
		}
		summary.Status = "initialized"
		summary.AnchorDate = latestSnapshotDate
		summary.Message = buildSimPortfolioSyncStatusText(0, summary.Status)
		return summary, nil
	}
	anchorDate := latestDaily.SignalDate
	includeAnchor := false
	if anchorDate == "" {
		anchorDate = latestDaily.TradeDate
		includeAnchor = true
	}
	summary.AnchorDate = anchorDate
	if anchorDate == "" {
		summary.Status = "no_anchor"
		summary.Message = buildSimPortfolioSyncStatusText(0, summary.Status)
		return summary, nil
	}
	dates, err := s.repo.ListRankingSnapshotDatesByExchangeRange(ctx, definition.Exchange, anchorDate, latestSnapshotDate)
	if err != nil {
		summary.Status = "failed"
		summary.Message = err.Error()
		return summary, err
	}
	if len(dates) < 2 {
		summary.Status = "up_to_date"
		summary.Message = buildSimPortfolioSyncStatusText(0, summary.Status)
		return summary, nil
	}
	startIndex := 0
	for index, date := range dates {
		if date == anchorDate {
			startIndex = index
			break
		}
	}
	if !includeAnchor {
		startIndex++
	}
	if startIndex >= len(dates)-1 {
		summary.Status = "up_to_date"
		summary.Message = buildSimPortfolioSyncStatusText(0, summary.Status)
		return summary, nil
	}
	previousDaily := latestDaily
	for index := startIndex; index < len(dates)-1; index++ {
		signalDate := dates[index]
		tradeDate := dates[index+1]
		if previousDaily.SignalDate != "" && previousDaily.SignalDate >= signalDate {
			continue
		}
		if err := s.computeAndPersistSimPortfolioDay(ctx, definition, signalDate, tradeDate, previousDaily); err != nil {
			summary.Status = "blocked"
			summary.Message = err.Error()
			if strings.Contains(err.Error(), "missing open price") {
				summary.MissingOpenPriceCount = 1
			}
			if strings.Contains(err.Error(), "missing close price") {
				summary.MissingClosePriceCount = 1
			}
			return summary, err
		}
		summary.GeneratedDailyCount++
		summary.LastGeneratedTradeDate = tradeDate
		previousDaily, err = s.repo.GetSimPortfolioDailyByTradeDate(ctx, definition.ID, tradeDate)
		if err != nil {
			summary.Status = "failed"
			summary.Message = err.Error()
			return summary, err
		}
	}
	if summary.GeneratedDailyCount > 0 {
		summary.Status = "synced"
	} else {
		summary.Status = "up_to_date"
	}
	summary.Message = buildSimPortfolioSyncStatusText(summary.GeneratedDailyCount, summary.Status)
	return summary, nil
}

func (s *Service) syncSimPortfolios(ctx context.Context) (*SimPortfolioSyncResponse, error) {
	definitions, err := s.repo.ListActiveRankingPortfolioDefinitions(ctx)
	if err != nil {
		return nil, err
	}
	resp := &SimPortfolioSyncResponse{OK: true, Items: []SimPortfolioSyncSummary{}}
	totalGenerated := 0
	for _, definition := range definitions {
		summary, err := s.syncSimPortfolioDefinition(ctx, definition)
		resp.Items = append(resp.Items, summary)
		totalGenerated += summary.GeneratedDailyCount
		if err != nil {
			resp.OK = false
			resp.Message = fmt.Sprintf("%s 同步失败：%s", definition.Name, summary.Message)
			return resp, err
		}
	}
	if totalGenerated > 0 {
		resp.Message = fmt.Sprintf("模拟组合事实表已同步，新增 %d 个估值日。", totalGenerated)
	} else {
		resp.Message = "模拟组合事实表已检查，暂无新增估值日。"
	}
	return resp, nil
}

func (s *Service) SyncSimPortfolios(ctx context.Context) (*SimPortfolioSyncResponse, error) {
	s.simPortfolioMu.Lock()
	defer s.simPortfolioMu.Unlock()
	// Auto-backfill missing open prices before syncing fact tables.
	_ = s.backfillSimPortfolioOpenPrices(ctx, "", "", true)
	return s.syncSimPortfolios(ctx)
}

// BackfillSimPortfolioOpenPrices scans market-price rows whose open_price is
// still 0 and fills them from the openPriceResolver (daily-bar Open). It is
// idempotent: rows whose open_price is already > 0 are never touched.
//
// When latestOnly is true, only the most recent pending snapshot date per
// definition is scanned (lightweight, suitable for admin quick-fix).
//
// When portfolioID is non-empty, only that definition is processed.
// When exchange is non-empty ("ASHARE" or "HKEX"), only definitions for that
// market scope are processed.
func (s *Service) BackfillSimPortfolioOpenPrices(ctx context.Context, portfolioID string, exchange string, latestOnly bool) (*SimPortfolioBackfillOpenPriceResponse, error) {
	s.simPortfolioMu.Lock()
	defer s.simPortfolioMu.Unlock()
	resp := s.backfillSimPortfolioOpenPrices(ctx, portfolioID, exchange, latestOnly)
	return resp, nil
}

func (s *Service) backfillSimPortfolioOpenPrices(ctx context.Context, portfolioID string, exchange string, latestOnly bool) *SimPortfolioBackfillOpenPriceResponse {
	resp := &SimPortfolioBackfillOpenPriceResponse{
		OK:                 true,
		PortfolioSummaries: []SimPortfolioBackfillSummary{},
	}
	if s.openPriceResolver == nil {
		resp.OK = false
		resp.Message = "open price resolver not configured"
		return resp
	}

	definitions, err := s.repo.ListActiveRankingPortfolioDefinitions(ctx)
	if err != nil {
		resp.OK = false
		resp.Message = err.Error()
		return resp
	}

	totalSummary := SimPortfolioBackfillSummary{}
	for _, definition := range definitions {
		if strings.TrimSpace(portfolioID) != "" && definition.ID != strings.TrimSpace(portfolioID) {
			continue
		}
		if strings.TrimSpace(exchange) != "" && !strings.EqualFold(strings.TrimSpace(definition.Exchange), strings.TrimSpace(exchange)) {
			continue
		}
		summary := s.backfillOpenPricesForDefinition(ctx, definition, latestOnly)
		resp.PortfolioSummaries = append(resp.PortfolioSummaries, summary)
		totalSummary.ScannedCount += summary.ScannedCount
		totalSummary.FilledCount += summary.FilledCount
		totalSummary.StillPendingCount += summary.StillPendingCount
		totalSummary.FailedCount += summary.FailedCount
		totalSummary.SkippedBeforeCutover += summary.SkippedBeforeCutover
		totalSummary.MissingMarketPriceRows += summary.MissingMarketPriceRows
	}
	resp.Summary = totalSummary
	if resp.OK {
		if totalSummary.FilledCount > 0 {
			resp.Message = fmt.Sprintf("已补齐 %d 条建仓开盘价，仍有 %d 条待补。", totalSummary.FilledCount, totalSummary.StillPendingCount)
		} else if totalSummary.StillPendingCount > 0 {
			resp.Message = fmt.Sprintf("仍有 %d 条建仓开盘价待补（可能是停牌、日线源未更新或目标交易日尚未到达）。", totalSummary.StillPendingCount)
		} else {
			resp.Message = "暂无缺失的建仓开盘价。"
		}
	}
	return resp
}

func (s *Service) backfillOpenPricesForDefinition(ctx context.Context, definition RankingPortfolioDefinition, latestOnly bool) SimPortfolioBackfillSummary {
	summary := SimPortfolioBackfillSummary{}
	missingRows, err := s.repo.ListMarketPricesMissingOpenByDateRange(ctx, "")
	if err != nil {
		summary.FailedCount = -1
		return summary
	}

	// Filter rows for this definition.
	var targetRows []MissingOpenPriceRow
	for _, row := range missingRows {
		if row.DefinitionID != definition.ID {
			continue
		}
		targetRows = append(targetRows, row)
	}

	if latestOnly && len(targetRows) > 0 {
		// Find the latest snapshot date among missing rows.
		latestDate := ""
		for _, row := range targetRows {
			if row.SnapshotDate > latestDate {
				latestDate = row.SnapshotDate
			}
		}
		filtered := targetRows[:0]
		for _, row := range targetRows {
			if row.SnapshotDate == latestDate {
				filtered = append(filtered, row)
			}
		}
		targetRows = filtered
	}

	summary.ScannedCount = len(targetRows)
	for _, row := range targetRows {
		entryTradeDate, err := s.resolveSimPortfolioEntryTradeDate(ctx, definition, row.SnapshotDate)
		if err != nil || entryTradeDate == "" {
			summary.StillPendingCount++
			continue
		}
		openPrice := s.openPriceResolver(ctx, row.Code, row.Exchange, entryTradeDate)
		if openPrice <= 0 {
			summary.StillPendingCount++
			continue
		}
		if err := s.repo.SetRankingPortfolioMarketPriceOpen(ctx, row.DefinitionID, row.SnapshotVersion, row.Code, row.Exchange, openPrice, entryTradeDate); err != nil {
			summary.FailedCount++
			continue
		}
		summary.FilledCount++
	}
	return summary
}

// resolveSimPortfolioEntryTradeDate returns the T+1 trading date for a given
// snapshot/signal date. It queries the RankingSnapshot table (the authoritative
// ranking snapshots source) for the next snapshot date after the given date.
// If no successor exists (latest snapshot), it falls back to today in Beijing
// time, provided today is strictly later than the snapshot date.
func (s *Service) resolveSimPortfolioEntryTradeDate(ctx context.Context, definition RankingPortfolioDefinition, snapshotDate string) (string, error) {
	snapshotDate = strings.TrimSpace(snapshotDate)
	if snapshotDate == "" {
		return "", nil
	}
	// Query RankingSnapshot (the authoritative table with daily snapshots)
	// instead of the legacy RankingPortfolioSnapshot table, which may lag behind.
	exchangeCodes := resolveSimPortfolioExchangeCodes(definition.Exchange)
	var nextDate string
	query := s.repo.db.WithContext(ctx).
		Model(&RankingSnapshot{}).
		Distinct("snapshot_date").
		Where("snapshot_date > ?", snapshotDate).
		Where("exchange IN ?", exchangeCodes).
		Order("snapshot_date ASC").
		Limit(1)
	if err := query.Pluck("snapshot_date", &nextDate).Error; err != nil {
		return "", err
	}
	if nextDate != "" {
		return nextDate, nil
	}
	// No successor snapshot — this is the latest batch.
	// Fall back to today in Beijing time if the snapshot is from a previous day.
	todayBJ := time.Now().In(beijingLocation()).Format("2006-01-02")
	if todayBJ > snapshotDate {
		return todayBJ, nil
	}
	return "", nil
}

func buildSimPortfolioStatusText(status string) string {
	switch status {
	case "baseline_only":
		return "仅有初始化资金，尚未生成持仓"
	case "pending_fact_sync":
		return "前置数据已就绪，等待同步事实表"
	case "pending_open_price":
		return "等待下一交易日开盘价"
	case "pending_close_price":
		return "已建仓，等待收盘估值"
	case "shortfall":
		return "当日有效成分股不足 4 只"
	case simPortfolioStatusComplete:
		return "已完成最新收盘估值"
	case simPortfolioStatusSeeded:
		return "已初始化，等待下一次调仓"
	default:
		return "等待数据更新"
	}
}

func buildSimPortfolioAdminActionHint(status SimPortfolioAdminStatusItem) string {
	if status.BaselineOnly && status.CanSync {
		return "只有初始化资金，前置数据已具备；请点击“同步最新事实表”生成持仓、调仓和指标。"
	}
	if status.BaselineOnly {
		return "只有初始化资金，等待下一批排行榜快照形成 T+1 估值；如需回灌历史请点击“从头重算全部组合”。"
	}
	if status.MissingOpenPriceCount > 0 {
		return "建仓开盘价缺失；请先点击“补齐建仓开盘价”，再同步事实表。"
	}
	if status.MissingClosePriceCount > 0 {
		return "收盘估值价缺失；请先刷新四象限/行情数据，再同步事实表。"
	}
	if status.CanSync {
		return "已有新信号可推进；请点击“同步最新事实表”。"
	}
	return "无需操作；如页面数据异常可先验证事实表一致性。"
}

func (s *Service) resolveSimPortfolioStatus(ctx context.Context, definition RankingPortfolioDefinition, latestDaily *SimPortfolioDaily) (simPortfolioStatus, error) {
	status := simPortfolioStatus{Status: simPortfolioStatusSeeded}
	latestSignalDate, err := s.repo.GetLatestRankingSnapshotDateByExchange(ctx, definition.Exchange)
	if err != nil {
		return status, err
	}
	status.LatestSignalDate = latestSignalDate
	if latestSignalDate == "" {
		status.StatusText = buildSimPortfolioStatusText(status.Status)
		return status, nil
	}
	if latestDaily != nil && latestDaily.SignalDate == latestSignalDate {
		status.Status = simPortfolioStatusComplete
		status.StatusText = buildSimPortfolioStatusText(status.Status)
		return status, nil
	}
	signalItems, warningText, err := s.selectSimPortfolioSignal(ctx, definition, latestSignalDate)
	if err != nil {
		return status, err
	}
	if len(signalItems) < definition.MaxHoldings {
		status.PendingSignalDate = latestSignalDate
		status.Status = "shortfall"
		status.StatusText = warningText
		return status, nil
	}
	entryDates := make([]string, 0, len(signalItems))
	missingOpen := 0
	for _, item := range signalItems {
		openPrice, entryTradeDate, priceErr := s.repo.GetRankingPortfolioSelectionOpenPrice(ctx, definition.ID, latestSignalDate, item.Code, item.Exchange)
		if priceErr != nil {
			return status, priceErr
		}
		if openPrice <= 0 || entryTradeDate == "" {
			missingOpen++
			status.PendingSignalDate = latestSignalDate
			status.Status = "pending_open_price"
			status.StatusText = buildSimPortfolioStatusText(status.Status)
			continue
		}
		entryDates = append(entryDates, entryTradeDate)
	}
	status.MissingOpenPriceCount = missingOpen
	if missingOpen > 0 {
		return status, nil
	}
	status.PendingSignalDate = latestSignalDate
	status.NextEntryTradeDate = sameSimPortfolioTradeDate(entryDates)
	status.Status = "pending_close_price"
	status.StatusText = buildSimPortfolioStatusText(status.Status)
	if latestDaily != nil && latestDaily.SignalDate == latestSignalDate && latestDaily.TradeDate == status.NextEntryTradeDate {
		status.Status = simPortfolioStatusComplete
		status.StatusText = buildSimPortfolioStatusText(status.Status)
	}
	return status, nil
}

func toSimPortfolioPositionItems(rows []SimPortfolioPosition) []SimPortfolioPositionItem {
	items := make([]SimPortfolioPositionItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, SimPortfolioPositionItem{
			Rank:            row.Rank,
			StockCode:       row.StockCode,
			StockName:       row.StockName,
			Exchange:        row.Exchange,
			Weight:          row.Weight,
			TargetValue:     row.TargetValue,
			Shares:          row.Shares,
			BuyPrice:        row.BuyPrice,
			ClosePrice:      row.ClosePrice,
			MarketValue:     row.MarketValue,
			Profit:          row.Profit,
			ProfitRate:      row.ProfitRate,
			SourceTradeDate: row.SourceTradeDate,
		})
	}
	return items
}

func toSimPortfolioTradeItems(rows []SimPortfolioTrade) []SimPortfolioTradeItem {
	items := make([]SimPortfolioTradeItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, SimPortfolioTradeItem{
			TradeDate:   row.TradeDate,
			SignalDate:  row.SignalDate,
			StockCode:   row.StockCode,
			StockName:   row.StockName,
			Exchange:    row.Exchange,
			Action:      row.Action,
			OldWeight:   row.OldWeight,
			NewWeight:   row.NewWeight,
			TradePrice:  row.TradePrice,
			TargetValue: row.TargetValue,
			OldShares:   row.OldShares,
			NewShares:   row.NewShares,
			ShareDelta:  row.ShareDelta,
			Reason:      row.Reason,
		})
	}
	return items
}

func (s *Service) GetSimPortfolioOverview(ctx context.Context) (*SimPortfolioOverviewResponse, error) {
	definitions, err := s.repo.ListActiveRankingPortfolioDefinitions(ctx)
	if err != nil {
		return nil, err
	}
	resp := &SimPortfolioOverviewResponse{Items: []SimPortfolioOverviewItem{}}
	asOfTradeDate := ""
	for _, definition := range definitions {
		latestDaily, err := s.repo.GetLatestSimPortfolioDaily(ctx, definition.ID)
		if err != nil {
			return nil, err
		}
		latestMetrics, err := s.repo.GetLatestSimPortfolioMetrics(ctx, definition.ID)
		if err != nil {
			return nil, err
		}
		status, err := s.resolveSimPortfolioStatus(ctx, definition, latestDaily)
		if err != nil {
			return nil, err
		}
		positions := []SimPortfolioPosition{}
		latestTrades := []SimPortfolioTrade{}
		if latestDaily != nil && latestDaily.PositionCount > 0 {
			positions, err = s.repo.ListSimPortfolioPositionsByTradeDate(ctx, definition.ID, latestDaily.TradeDate)
			if err != nil {
				return nil, err
			}
			latestTrades, err = s.repo.ListLatestSimPortfolioTrades(ctx, definition.ID, 6)
			if err != nil {
				return nil, err
			}
			if asOfTradeDate == "" || latestDaily.TradeDate > asOfTradeDate {
				asOfTradeDate = latestDaily.TradeDate
			}
		}
		item := SimPortfolioOverviewItem{
			PortfolioID:        definition.ID,
			Code:               definition.Code,
			Name:               definition.Name,
			Exchange:           definition.Exchange,
			PortfolioVariant:   definition.PortfolioVariant,
			SelectionRule:      definition.SelectionRule,
			SelectionWindow:    definition.SelectionWindow,
			InitialAssets:      simPortfolioInitialAssets,
			PositionCount:      definition.MaxHoldings,
			LatestSignalDate:   status.LatestSignalDate,
			PendingSignalDate:  status.PendingSignalDate,
			NextEntryTradeDate: status.NextEntryTradeDate,
			Status:             status.Status,
			StatusText:         status.StatusText,
			CurrentPositions:   toSimPortfolioPositionItems(positions),
			LatestTrades:       toSimPortfolioTradeItems(latestTrades),
		}
		if latestDaily != nil {
			item.LatestTradeDate = latestDaily.TradeDate
			item.NAV = latestDaily.NAV
			item.TotalAssets = latestDaily.TotalAssets
			item.DailyReturn = latestDaily.DailyReturn
			item.TotalReturn = latestDaily.TotalReturn
		}
		if latestMetrics != nil {
			item.MaxDrawdown = latestMetrics.MaxDrawdown
			item.Volatility = latestMetrics.Volatility
			item.WinRate = latestMetrics.WinRate
			item.TurnoverRate = latestMetrics.TurnoverRate
		}
		resp.Items = append(resp.Items, item)
	}
	resp.AsOfTradeDate = asOfTradeDate
	return resp, nil
}

func (s *Service) GetSimPortfolioDaily(ctx context.Context, portfolioID string, fromDate string, toDate string) (*SimPortfolioDailyResponse, error) {
	rows, err := s.repo.ListSimPortfolioDailyRange(ctx, portfolioID, fromDate, toDate)
	if err != nil {
		return nil, err
	}
	resp := &SimPortfolioDailyResponse{PortfolioID: portfolioID, Items: []SimPortfolioDailyItem{}}
	for _, row := range rows {
		resp.Items = append(resp.Items, SimPortfolioDailyItem{
			TradeDate:       row.TradeDate,
			SignalDate:      row.SignalDate,
			SourceTradeDate: row.SourceTradeDate,
			NAV:             row.NAV,
			TotalAssets:     row.TotalAssets,
			DailyReturn:     row.DailyReturn,
			TotalReturn:     row.TotalReturn,
			PositionCount:   row.PositionCount,
			Rebalance:       row.Rebalance,
			Status:          row.Status,
			WarningText:     row.WarningText,
		})
	}
	return resp, nil
}

func (s *Service) GetSimPortfolioPositions(ctx context.Context, portfolioID string, tradeDate string) (*SimPortfolioPositionsResponse, error) {
	if strings.TrimSpace(tradeDate) == "" {
		latestDaily, err := s.repo.GetLatestSimPortfolioDaily(ctx, portfolioID)
		if err != nil {
			return nil, err
		}
		if latestDaily != nil {
			tradeDate = latestDaily.TradeDate
		}
	}
	daily, err := s.repo.GetSimPortfolioDailyByTradeDate(ctx, portfolioID, tradeDate)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.ListSimPortfolioPositionsByTradeDate(ctx, portfolioID, tradeDate)
	if err != nil {
		return nil, err
	}
	resp := &SimPortfolioPositionsResponse{
		PortfolioID: portfolioID,
		TradeDate:   tradeDate,
		Items:       toSimPortfolioPositionItems(rows),
	}
	if daily != nil {
		resp.TotalAssets = daily.TotalAssets
	}
	return resp, nil
}

func (s *Service) GetSimPortfolioTrades(ctx context.Context, portfolioID string, fromDate string, toDate string, action string) (*SimPortfolioTradesResponse, error) {
	rows, err := s.repo.ListSimPortfolioTradesRange(ctx, portfolioID, fromDate, toDate, action)
	if err != nil {
		return nil, err
	}
	return &SimPortfolioTradesResponse{PortfolioID: portfolioID, Items: toSimPortfolioTradeItems(rows)}, nil
}

func (s *Service) GetSimPortfolioMetrics(ctx context.Context, portfolioID string) (*SimPortfolioMetricsResponse, error) {
	row, err := s.repo.GetLatestSimPortfolioMetrics(ctx, portfolioID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return &SimPortfolioMetricsResponse{PortfolioID: portfolioID}, nil
	}
	return &SimPortfolioMetricsResponse{
		PortfolioID:     portfolioID,
		TradeDate:       row.TradeDate,
		NAV:             row.NAV,
		AnnualReturn:    row.AnnualReturn,
		MaxDrawdown:     row.MaxDrawdown,
		SharpeRatio:     row.SharpeRatio,
		Volatility:      row.Volatility,
		WinRate:         row.WinRate,
		TurnoverRate:    row.TurnoverRate,
		BenchmarkReturn: row.BenchmarkReturn,
		ExcessReturn:    row.ExcessReturn,
	}, nil
}

func (s *Service) GetSimPortfolioAdminStatus(ctx context.Context) (*SimPortfolioAdminStatusResponse, error) {
	definitions, err := s.repo.ListActiveRankingPortfolioDefinitions(ctx)
	if err != nil {
		return nil, err
	}
	resp := &SimPortfolioAdminStatusResponse{Items: []SimPortfolioAdminStatusItem{}}
	for _, definition := range definitions {
		latestDaily, err := s.repo.GetLatestSimPortfolioDaily(ctx, definition.ID)
		if err != nil {
			return nil, err
		}
		status, err := s.resolveSimPortfolioStatus(ctx, definition, latestDaily)
		if err != nil {
			return nil, err
		}
		dailyRows, err := s.repo.ListAllSimPortfolioDaily(ctx, definition.ID)
		if err != nil {
			return nil, err
		}
		positionCount, err := s.repo.CountSimPortfolioPositions(ctx, definition.ID)
		if err != nil {
			return nil, err
		}
		tradeCount, err := s.repo.CountSimPortfolioTrades(ctx, definition.ID)
		if err != nil {
			return nil, err
		}
		metricsCount, err := s.repo.CountSimPortfolioMetrics(ctx, definition.ID)
		if err != nil {
			return nil, err
		}
		completedDailyCount := 0
		for _, daily := range dailyRows {
			if daily.Status == simPortfolioStatusComplete && daily.PositionCount > 0 {
				completedDailyCount++
			}
		}

		item := SimPortfolioAdminStatusItem{
			PortfolioID:            definition.ID,
			Name:                   definition.Name,
			Exchange:               definition.Exchange,
			LatestSignalDate:       status.LatestSignalDate,
			PendingSignalDate:      status.PendingSignalDate,
			NextEntryTradeDate:     status.NextEntryTradeDate,
			Status:                 status.Status,
			StatusText:             status.StatusText,
			DailyRowCount:          len(dailyRows),
			CompletedDailyCount:    completedDailyCount,
			PositionRowCount:       positionCount,
			TradeRowCount:          tradeCount,
			MetricsRowCount:        metricsCount,
			MissingOpenPriceCount:  status.MissingOpenPriceCount,
			MissingClosePriceCount: status.MissingClosePriceCount,
		}
		if latestDaily != nil {
			item.LatestTradeDate = latestDaily.TradeDate
		}
		item.BaselineOnly = item.DailyRowCount > 0 && item.CompletedDailyCount == 0 && item.PositionRowCount == 0
		if item.BaselineOnly {
			item.Status = "baseline_only"
			item.StatusText = buildSimPortfolioStatusText(item.Status)
		}
		if latestDaily != nil {
			anchorDate := strings.TrimSpace(latestDaily.SignalDate)
			includeAnchor := false
			if anchorDate == "" {
				anchorDate = strings.TrimSpace(latestDaily.TradeDate)
				includeAnchor = true
			}
			if anchorDate != "" {
				dates, err := s.repo.ListRankingSnapshotDatesByExchangeRange(ctx, definition.Exchange, anchorDate, status.LatestSignalDate)
				if err != nil {
					return nil, err
				}
				startIndex := 0
				for index, date := range dates {
					if date == anchorDate {
						startIndex = index
						break
					}
				}
				if !includeAnchor {
					startIndex++
				}
				if startIndex < len(dates)-1 {
					item.CanSync = true
					item.NextSyncSignalDate = dates[startIndex]
					item.NextSyncTradeDate = dates[startIndex+1]
				}
			}
		}
		if item.Status == "pending_open_price" && item.MissingOpenPriceCount == 0 && item.CanSync {
			item.Status = "pending_fact_sync"
			item.StatusText = buildSimPortfolioStatusText(item.Status)
		}
		item.ActionHint = buildSimPortfolioAdminActionHint(item)
		resp.Items = append(resp.Items, item)
	}
	return resp, nil
}

func (s *Service) VerifySimPortfolios(ctx context.Context, portfolioID string) (*SimPortfolioVerifyResponse, error) {
	definitions, err := s.repo.ListActiveRankingPortfolioDefinitions(ctx)
	if err != nil {
		return nil, err
	}
	resp := &SimPortfolioVerifyResponse{Items: []SimPortfolioVerifyItem{}}
	for _, definition := range definitions {
		if strings.TrimSpace(portfolioID) != "" && definition.ID != strings.TrimSpace(portfolioID) {
			continue
		}
		dailyRows, err := s.repo.ListAllSimPortfolioDaily(ctx, definition.ID)
		if err != nil {
			return nil, err
		}
		for _, daily := range dailyRows {
			// Skip seeded baseline rows — they have no positions by design.
			if daily.Status == simPortfolioStatusSeeded || daily.PositionCount == 0 {
				resp.Items = append(resp.Items, SimPortfolioVerifyItem{
					PortfolioID:   definition.ID,
					TradeDate:     daily.TradeDate,
					Status:        "ok",
					PositionCount: 0,
					TotalAssets:   daily.TotalAssets,
					Message:       "seeded baseline, no positions",
				})
				continue
			}
			positions, err := s.repo.ListSimPortfolioPositionsByTradeDate(ctx, definition.ID, daily.TradeDate)
			if err != nil {
				return nil, err
			}
			positionAssets := 0.0
			missingOpenPrices := 0
			for _, position := range positions {
				positionAssets += position.MarketValue
				if position.BuyPrice <= 0 {
					missingOpenPrices++
				}
			}
			difference := positionAssets - daily.TotalAssets
			status := "ok"
			message := ""
			if daily.PositionCount != 0 && len(positions) != daily.PositionCount {
				status = "count_mismatch"
				message = "position_count 与持仓明细数量不一致"
			} else if missingOpenPrices > 0 {
				status = "missing_open_price"
				message = fmt.Sprintf("%d 只持仓的建仓开盘价缺失", missingOpenPrices)
			} else if math.Abs(difference) > 0.01 {
				status = "asset_mismatch"
				message = "portfolio_daily.total_assets 与持仓市值汇总不一致"
			}
			resp.Items = append(resp.Items, SimPortfolioVerifyItem{
				PortfolioID:    definition.ID,
				TradeDate:      daily.TradeDate,
				Status:         status,
				PositionCount:  len(positions),
				TotalAssets:    daily.TotalAssets,
				PositionAssets: positionAssets,
				Difference:     difference,
				Message:        message,
			})
		}
	}
	return resp, nil
}

func (s *Service) RecomputeSimPortfolios(ctx context.Context, portfolioID string, fromDate string, toDate string, reset bool) error {
	s.simPortfolioMu.Lock()
	defer s.simPortfolioMu.Unlock()
	// Auto-backfill missing open prices before recomputing fact tables.
	_ = s.backfillSimPortfolioOpenPrices(ctx, portfolioID, "", false)
	definitions, err := s.repo.ListActiveRankingPortfolioDefinitions(ctx)
	if err != nil {
		return err
	}
	for _, definition := range definitions {
		if strings.TrimSpace(portfolioID) != "" && definition.ID != strings.TrimSpace(portfolioID) {
			continue
		}
		if err := s.recomputeSimPortfolioDefinition(ctx, definition, fromDate, toDate, reset); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) ResetSimPortfolios(ctx context.Context, portfolioID string) error {
	s.simPortfolioMu.Lock()
	defer s.simPortfolioMu.Unlock()
	return s.repo.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, model := range []any{&SimPortfolioPosition{}, &SimPortfolioTrade{}, &SimPortfolioMetrics{}, &SimPortfolioDaily{}} {
			query := tx
			if strings.TrimSpace(portfolioID) != "" {
				query = query.Where("portfolio_id = ?", strings.TrimSpace(portfolioID))
			}
			if err := query.Delete(model).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
