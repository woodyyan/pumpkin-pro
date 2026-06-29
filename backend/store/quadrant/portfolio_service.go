package quadrant

// Legacy ranking-portfolio materialization pipeline retained only for historical
// audit and compatibility. The new /portfolio-tracking experience uses the
// sim_portfolio_* fact-table chain instead of extending this JSON-result path.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const rankingPortfolioAnnualTradingDays = 252

const rankingPortfolioTradeCostDisplayDigits = 6

const (
	rankingPortfolioEffectiveHour   = 9
	rankingPortfolioEffectiveMinute = 30
)

type rankingPortfolioDefinitionSpec struct {
	ID               string
	Code             string
	Name             string
	Exchange         string
	PortfolioVariant string
	BenchmarkCode    string
	BenchmarkName    string
	SelectionRule    string
	SelectionWindow  int
	ExcludedBoards   []string
	MethodNote       string
}

func defaultRankingPortfolioDefinitionSpecs() []rankingPortfolioDefinitionSpec {
	return []rankingPortfolioDefinitionSpec{
		{
			ID:               defaultRankingPortfolioDefinitionID,
			Code:             defaultRankingPortfolioDefinitionCode,
			Name:             defaultRankingPortfolioName,
			Exchange:         defaultRankingPortfolioExchange,
			PortfolioVariant: rankingPortfolioVariantA,
			BenchmarkCode:    defaultRankingPortfolioBenchmarkCode,
			BenchmarkName:    defaultRankingPortfolioBenchmarkName,
			SelectionRule:    rankingPortfolioSelectionRuleTop4,
			ExcludedBoards:   []string{aShareBoardStar},
			MethodNote:       "",
		},
		{
			ID:               "wolong_ai_top10_ex_star_by_streak_v1",
			Code:             "wolong-ai-top10-ex-star-by-streak",
			Name:             "模拟组合B",
			Exchange:         "ASHARE",
			PortfolioVariant: rankingPortfolioVariantB,
			BenchmarkCode:    "SHCI",
			BenchmarkName:    "上证指数",
			SelectionRule:    rankingPortfolioSelectionRuleTop10ByStreak,
			SelectionWindow:  10,
			ExcludedBoards:   []string{aShareBoardStar},
			MethodNote:       "",
		},
		{
			ID:               "wolong_ai_hk_top4_equal_v1",
			Code:             "wolong-ai-hk-top4-equal",
			Name:             "模拟组合A",
			Exchange:         "HKEX",
			PortfolioVariant: rankingPortfolioVariantA,
			BenchmarkCode:    "HSI",
			BenchmarkName:    "恒生指数",
			SelectionRule:    rankingPortfolioSelectionRuleTop4,
			MethodNote:       "",
		},
		{
			ID:               "wolong_ai_hk_top10_by_streak_v1",
			Code:             "wolong-ai-hk-top10-by-streak",
			Name:             "模拟组合B",
			Exchange:         "HKEX",
			PortfolioVariant: rankingPortfolioVariantB,
			BenchmarkCode:    "HSI",
			BenchmarkName:    "恒生指数",
			SelectionRule:    rankingPortfolioSelectionRuleTop10ByStreak,
			SelectionWindow:  10,
			MethodNote:       "",
		},
	}
}

func buildRankingPortfolioDefinitionRecord(spec rankingPortfolioDefinitionSpec, now time.Time) RankingPortfolioDefinition {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	methodNote := strings.TrimSpace(spec.MethodNote)
	if methodNote == "" {
		methodNote = rankingPortfolioMethodNote
	}
	return RankingPortfolioDefinition{
		ID:               spec.ID,
		Code:             spec.Code,
		Name:             spec.Name,
		Exchange:         spec.Exchange,
		PortfolioVariant: spec.PortfolioVariant,
		BenchmarkCode:    spec.BenchmarkCode,
		BenchmarkName:    spec.BenchmarkName,
		MaxHoldings:      defaultRankingPortfolioMaxHoldings,
		SelectionRule:    spec.SelectionRule,
		SelectionWindow:  spec.SelectionWindow,
		ExcludedBoards:   mustMarshal(spec.ExcludedBoards),
		WeightingMethod:  "equal",
		RebalanceRule:    rankingPortfolioRebalanceRuleClose,
		TradeCostRate:    defaultRankingPortfolioTradeCostRate,
		MethodNote:       methodNote,
		IsActive:         true,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
}

func defaultRankingPortfolioDefinitionRecord(now time.Time) RankingPortfolioDefinition {
	return buildRankingPortfolioDefinitionRecord(defaultRankingPortfolioDefinitionSpecs()[0], now)
}

func defaultRankingPortfolioDefinitionRecords(now time.Time) []RankingPortfolioDefinition {
	specs := defaultRankingPortfolioDefinitionSpecs()
	defs := make([]RankingPortfolioDefinition, 0, len(specs))
	for _, spec := range specs {
		defs = append(defs, buildRankingPortfolioDefinitionRecord(spec, now))
	}
	return defs
}

func hasExchangeRecords(records []QuadrantScoreRecord, exchange string) bool {
	normalized := strings.ToUpper(strings.TrimSpace(exchange))
	for _, record := range records {
		recordExchange := strings.ToUpper(strings.TrimSpace(record.Exchange))
		switch normalized {
		case "ASHARE":
			if recordExchange == "SSE" || recordExchange == "SZSE" {
				return true
			}
		case "HKEX":
			if recordExchange == "HKEX" {
				return true
			}
		}
	}
	return false
}

func listRankingPortfolioDefinitionsForRecords(records []QuadrantScoreRecord, now time.Time) []RankingPortfolioDefinition {
	defs := defaultRankingPortfolioDefinitionRecords(now)
	selected := make([]RankingPortfolioDefinition, 0, len(defs))
	for _, def := range defs {
		if hasExchangeRecords(records, def.Exchange) {
			selected = append(selected, def)
		}
	}
	return selected
}

func buildRankingPortfolioSnapshotVersion(snapshotDate string) string {
	return strings.TrimSpace(snapshotDate)
}

func buildRankingPortfolioBatchID(definitionID, snapshotVersion string) string {
	return strings.TrimSpace(definitionID) + ":" + strings.TrimSpace(snapshotVersion)
}

func buildRankingPortfolioEffectiveTime(computedAt time.Time) time.Time {
	if computedAt.IsZero() {
		return time.Time{}
	}
	local := computedAt.In(rankingSnapshotLocation)
	nextDay := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, rankingSnapshotLocation).AddDate(0, 0, 1)
	for nextDay.Weekday() == time.Saturday || nextDay.Weekday() == time.Sunday {
		nextDay = nextDay.AddDate(0, 0, 1)
	}
	return time.Date(
		nextDay.Year(),
		nextDay.Month(),
		nextDay.Day(),
		rankingPortfolioEffectiveHour,
		rankingPortfolioEffectiveMinute,
		0,
		0,
		rankingSnapshotLocation,
	).UTC()
}

func buildRankingPortfolioCurrentEffectiveTime(computedAt time.Time) time.Time {
	if computedAt.IsZero() {
		return time.Time{}
	}
	local := computedAt.In(rankingSnapshotLocation)
	effectiveToday := time.Date(
		local.Year(),
		local.Month(),
		local.Day(),
		rankingPortfolioEffectiveHour,
		rankingPortfolioEffectiveMinute,
		0,
		0,
		rankingSnapshotLocation,
	)
	if local.Weekday() != time.Saturday && local.Weekday() != time.Sunday && !local.After(effectiveToday) {
		return effectiveToday.UTC()
	}

	nextDay := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, rankingSnapshotLocation).AddDate(0, 0, 1)
	for nextDay.Weekday() == time.Saturday || nextDay.Weekday() == time.Sunday {
		nextDay = nextDay.AddDate(0, 0, 1)
	}
	return time.Date(
		nextDay.Year(),
		nextDay.Month(),
		nextDay.Day(),
		rankingPortfolioEffectiveHour,
		rankingPortfolioEffectiveMinute,
		0,
		0,
		rankingSnapshotLocation,
	).UTC()
}

func buildRankingPortfolioCurrentSourceDate(effectiveTime time.Time) string {
	if effectiveTime.IsZero() {
		return ""
	}
	local := effectiveTime.In(rankingSnapshotLocation)
	tradeDay := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, rankingSnapshotLocation).AddDate(0, 0, -1)
	for tradeDay.Weekday() == time.Saturday || tradeDay.Weekday() == time.Sunday {
		tradeDay = tradeDay.AddDate(0, 0, -1)
	}
	return tradeDay.Format("2006-01-02")
}

type rankingPortfolioPersistError struct {
	DefinitionID string
	Stage        string
	Reason       string
	Details      map[string]any
}

type rankingPortfolioRebuildPlan struct {
	Date            string
	SnapshotTime    time.Time
	Constituents    []RankingPortfolioConstituentItem
	MarketPrices    []RankingPortfolioMarketPrice
	HasShortfall    bool
	WarningText     string
	SourceTradeDate string
}

type rankingSnapshotSourceRow struct {
	ID             int64
	Code           string
	Name           string
	Exchange       string
	Rank           int
	Opportunity    float64
	Risk           float64
	ClosePrice     float64
	PriceTradeDate string
	SnapshotDate   string
}

type rankingPortfolioPriceLookup struct {
	ClosePrice float64
	TradeDate  string
}

func (s *Service) resolveRankingPortfolioMarketPrice(ctx context.Context, code string, exchange string, targetTradeDate string) rankingPortfolioPriceLookup {
	targetTradeDate = strings.TrimSpace(targetTradeDate)
	if targetTradeDate == "" {
		return rankingPortfolioPriceLookup{}
	}
	if s.priceResolver != nil {
		if closePrice := s.priceResolver(ctx, code, exchange, targetTradeDate); closePrice > 0 {
			return rankingPortfolioPriceLookup{ClosePrice: closePrice, TradeDate: targetTradeDate}
		}
	}
	if s.repo != nil {
		if closePrice, tradeDate, err := s.repo.GetLatestRankingSnapshotClosePriceOnOrBefore(ctx, code, exchange, targetTradeDate); err == nil && closePrice > 0 && strings.TrimSpace(tradeDate) != "" {
			return rankingPortfolioPriceLookup{ClosePrice: closePrice, TradeDate: strings.TrimSpace(tradeDate)}
		}
	}
	return rankingPortfolioPriceLookup{}
}

func (e *rankingPortfolioPersistError) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.DefinitionID) != "" {
		return fmt.Sprintf("%s[%s]", e.Reason, e.DefinitionID)
	}
	return e.Reason
}

func newRankingPortfolioPersistError(definitionID string, stage string, reason string, details map[string]any) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "unknown ranking portfolio error"
	}
	return &rankingPortfolioPersistError{
		DefinitionID: strings.TrimSpace(definitionID),
		Stage:        strings.TrimSpace(stage),
		Reason:       reason,
		Details:      details,
	}
}

func rankingPortfolioAutoRepairStatus(err error) string {
	if err == nil {
		return "success"
	}
	return "failed"
}

func rankingPortfolioErrorParts(err error) (string, string, map[string]any) {
	if err == nil {
		return "", "", nil
	}
	var persistErr *rankingPortfolioPersistError
	if errors.As(err, &persistErr) {
		return strings.TrimSpace(persistErr.Stage), strings.TrimSpace(persistErr.Reason), persistErr.Details
	}
	return "", strings.TrimSpace(err.Error()), nil
}

func rankingPortfolioDetailsJSON(details map[string]any) string {
	if len(details) == 0 {
		return "{}"
	}
	payload, err := json.Marshal(details)
	if err != nil {
		return "{}"
	}
	return string(payload)
}

func rankingPortfolioLagDays(latestRankingDate string, latestPortfolioDate string) int {
	latestRankingDate = strings.TrimSpace(latestRankingDate)
	latestPortfolioDate = strings.TrimSpace(latestPortfolioDate)
	if latestRankingDate == "" || latestPortfolioDate == "" {
		return 0
	}
	rankingAt, err1 := time.ParseInLocation("2006-01-02", latestRankingDate, rankingSnapshotLocation)
	portfolioAt, err2 := time.ParseInLocation("2006-01-02", latestPortfolioDate, rankingSnapshotLocation)
	if err1 != nil || err2 != nil || rankingAt.Before(portfolioAt) {
		return 0
	}
	return int(rankingAt.Sub(portfolioAt).Hours() / 24)
}

func (s *Service) persistRankingPortfolioFailureStatus(ctx context.Context, taskLogID string, err error, snapshotDate string, sourceTradeDate string, autoRepairTriggered bool, autoRepairMessage string) error {
	if strings.TrimSpace(taskLogID) == "" || err == nil {
		return nil
	}
	stage, reason, details := rankingPortfolioErrorParts(err)
	now := time.Now().UTC()
	for _, definition := range defaultRankingPortfolioDefinitionRecords(now) {
		item := RankingPortfolioJobStatus{
			TaskLogID:           strings.TrimSpace(taskLogID),
			DefinitionID:        definition.ID,
			DefinitionCode:      definition.Code,
			DefinitionName:      definition.Name,
			Exchange:            definition.Exchange,
			SnapshotDate:        strings.TrimSpace(snapshotDate),
			SourceTradeDate:     strings.TrimSpace(sourceTradeDate),
			Status:              "failed",
			FailureStage:        stage,
			FailureReason:       reason,
			DetailsJSON:         rankingPortfolioDetailsJSON(details),
			AutoRepairTriggered: autoRepairTriggered,
			AutoRepairStatus:    "",
			AutoRepairMessage:   strings.TrimSpace(autoRepairMessage),
			CreatedAt:           now,
			UpdatedAt:           now,
		}
		if err := s.repo.UpsertRankingPortfolioJobStatus(ctx, item); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) persistRankingPortfolioDefinitionStatus(ctx context.Context, taskLogID string, definition RankingPortfolioDefinition, snapshotDate string, sourceTradeDate string, persistErr error, autoRepairTriggered bool, autoRepairMessage string) error {
	if strings.TrimSpace(taskLogID) == "" {
		return nil
	}
	status := "success"
	failureStage := ""
	failureReason := ""
	var details map[string]any
	if persistErr != nil {
		status = "failed"
		failureStage, failureReason, details = rankingPortfolioErrorParts(persistErr)
	}
	now := time.Now().UTC()
	item := RankingPortfolioJobStatus{
		TaskLogID:           strings.TrimSpace(taskLogID),
		DefinitionID:        definition.ID,
		DefinitionCode:      definition.Code,
		DefinitionName:      definition.Name,
		Exchange:            definition.Exchange,
		SnapshotDate:        strings.TrimSpace(snapshotDate),
		SourceTradeDate:     strings.TrimSpace(sourceTradeDate),
		Status:              status,
		FailureStage:        failureStage,
		FailureReason:       failureReason,
		DetailsJSON:         rankingPortfolioDetailsJSON(details),
		AutoRepairTriggered: autoRepairTriggered,
		AutoRepairStatus:    "",
		AutoRepairMessage:   strings.TrimSpace(autoRepairMessage),
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	return s.repo.UpsertRankingPortfolioJobStatus(ctx, item)
}

func rankingPortfolioRepairTaskLogID(now time.Time) string {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return fmt.Sprintf("qrp-repair-%d", now.UnixMilli())
}

func (s *Service) RebuildLaggingRankingPortfolioResults(ctx context.Context, taskLogID string, markAutoRepair bool) error {
	return s.RebuildLaggingRankingPortfolioResultsFromDate(ctx, taskLogID, markAutoRepair, "")
}

// RebuildLaggingRankingPortfolioResultsFromDate rebuilds lagging portfolio results
// starting from cutoverDate (inclusive). Pass "" to use the default lookback window.
func (s *Service) RebuildLaggingRankingPortfolioResultsFromDate(ctx context.Context, taskLogID string, markAutoRepair bool, cutoverDate string) error {
	definitions := defaultRankingPortfolioDefinitionRecords(time.Now().UTC())
	var firstErr error
	for _, definition := range definitions {
		if err := s.rebuildLaggingRankingPortfolioResultForDefinitionFromDate(ctx, definition, taskLogID, markAutoRepair, cutoverDate); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (s *Service) rebuildLaggingRankingPortfolioResultForDefinition(ctx context.Context, definition RankingPortfolioDefinition, taskLogID string, markAutoRepair bool) error {
	return s.rebuildLaggingRankingPortfolioResultForDefinitionFromDate(ctx, definition, taskLogID, markAutoRepair, "")
}

func (s *Service) rebuildLaggingRankingPortfolioResultForDefinitionFromDate(ctx context.Context, definition RankingPortfolioDefinition, taskLogID string, markAutoRepair bool, cutoverDate string) error {
	latestRankingDate, err := s.repo.GetLatestRankingSnapshotDateByExchange(ctx, definition.Exchange)
	if err != nil {
		return err
	}
	latestRankingDate = strings.TrimSpace(latestRankingDate)
	if latestRankingDate == "" {
		return nil
	}
	latestPortfolioDate, err := s.repo.GetLatestRankingPortfolioResultDate(ctx, definition.ID)
	if err != nil {
		return err
	}
	latestPortfolioDate = strings.TrimSpace(latestPortfolioDate)
	if latestPortfolioDate != "" && latestPortfolioDate >= latestRankingDate {
		return nil
	}
	fromDate := latestPortfolioDate
	if fromDate == "" {
		fromDate = recentSnapshotRepairFromDate(latestRankingDate)
	}
	// D0 守卫：如果调用方传入了 cutoverDate，则 fromDate 不得早于 cutoverDate，
	// 防止补齐路径回溯到上线日之前的旧口径数据。
	if cutoverDate != "" && (fromDate == "" || fromDate < cutoverDate) {
		fromDate = cutoverDate
	}
	targetDates, err := s.repo.ListRankingSnapshotDatesByExchangeRange(ctx, definition.Exchange, fromDate, latestRankingDate)
	if err != nil {
		return err
	}
	if latestPortfolioDate != "" {
		filtered := make([]string, 0, len(targetDates))
		for _, snapshotDate := range targetDates {
			if strings.TrimSpace(snapshotDate) > latestPortfolioDate {
				filtered = append(filtered, strings.TrimSpace(snapshotDate))
			}
		}
		targetDates = filtered
	}
	if len(targetDates) == 0 {
		return nil
	}
	now := time.Now().UTC()
	autoRepairMessage := fmt.Sprintf("rebuild portfolio results from latest_ranking_date=%s latest_portfolio_date=%s", latestRankingDate, latestPortfolioDate)
	autoRepairStatus := ""
	lastAutoRepairAt := (*time.Time)(nil)
	if markAutoRepair {
		autoRepairStatus = "running"
		ts := now
		lastAutoRepairAt = &ts
	}
	rebuiltCount := 0
	for _, snapshotDate := range targetDates {
		if err := s.rebuildRankingPortfolioResultForSnapshot(ctx, definition, snapshotDate); err != nil {
			if strings.TrimSpace(taskLogID) != "" {
				stage, reason, details := rankingPortfolioErrorParts(err)
				failedAt := time.Now().UTC()
				item := RankingPortfolioJobStatus{
					TaskLogID:           strings.TrimSpace(taskLogID),
					DefinitionID:        definition.ID,
					DefinitionCode:      definition.Code,
					DefinitionName:      definition.Name,
					Exchange:            definition.Exchange,
					SnapshotDate:        snapshotDate,
					SourceTradeDate:     "",
					Status:              "failed",
					FailureStage:        stage,
					FailureReason:       reason,
					DetailsJSON:         rankingPortfolioDetailsJSON(details),
					AutoRepairTriggered: markAutoRepair,
					AutoRepairStatus:    rankingPortfolioAutoRepairStatus(err),
					AutoRepairMessage:   autoRepairMessage,
					LastAutoRepairAt:    &failedAt,
					CreatedAt:           failedAt,
					UpdatedAt:           failedAt,
				}
				_ = s.repo.UpsertRankingPortfolioJobStatus(ctx, item)
			}
			return err
		}
		var rebuilt RankingPortfolioResult
		if err := s.repo.db.WithContext(ctx).
			Where("definition_id = ? AND snapshot_date = ?", definition.ID, strings.TrimSpace(snapshotDate)).
			Order("id DESC").
			First(&rebuilt).Error; err == nil {
			rebuiltCount++
		}
	}
	if rebuiltCount == 0 {
		return nil
	}
	if strings.TrimSpace(taskLogID) != "" && markAutoRepair {
		runningAt := time.Now().UTC()
		item := RankingPortfolioJobStatus{
			TaskLogID:           strings.TrimSpace(taskLogID),
			DefinitionID:        definition.ID,
			DefinitionCode:      definition.Code,
			DefinitionName:      definition.Name,
			Exchange:            definition.Exchange,
			SnapshotDate:        targetDates[len(targetDates)-1],
			SourceTradeDate:     "",
			Status:              "success",
			FailureStage:        "",
			FailureReason:       "",
			DetailsJSON:         "{}",
			AutoRepairTriggered: true,
			AutoRepairStatus:    autoRepairStatus,
			AutoRepairMessage:   autoRepairMessage,
			LastAutoRepairAt:    &runningAt,
			CreatedAt:           runningAt,
			UpdatedAt:           runningAt,
		}
		if err := s.repo.UpsertRankingPortfolioJobStatus(ctx, item); err != nil {
			return err
		}
	}
	if strings.TrimSpace(taskLogID) != "" {
		latestResult, err := s.repo.GetLatestRankingPortfolioResultByDefinition(ctx, definition.ID)
		if err == nil && latestResult != nil {
			completedAt := time.Now().UTC()
			item := RankingPortfolioJobStatus{
				TaskLogID:           strings.TrimSpace(taskLogID),
				DefinitionID:        definition.ID,
				DefinitionCode:      definition.Code,
				DefinitionName:      definition.Name,
				Exchange:            definition.Exchange,
				SnapshotDate:        latestResult.SnapshotDate,
				SourceTradeDate:     latestResult.SourceTradeDate,
				Status:              "success",
				FailureStage:        "",
				FailureReason:       "",
				DetailsJSON:         "{}",
				AutoRepairTriggered: markAutoRepair,
				AutoRepairStatus:    map[bool]string{true: "success", false: ""}[markAutoRepair],
				AutoRepairMessage:   autoRepairMessage,
				LastAutoRepairAt:    lastAutoRepairAt,
				CreatedAt:           completedAt,
				UpdatedAt:           completedAt,
			}
			if markAutoRepair {
				item.LastAutoRepairAt = &completedAt
			}
			if err := s.repo.UpsertRankingPortfolioJobStatus(ctx, item); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) rebuildRankingPortfolioFromRankingSnapshot(ctx context.Context, definition RankingPortfolioDefinition, snapshotDate string) error {
	plan, err := s.buildRankingPortfolioRebuildPlan(ctx, definition, snapshotDate)
	if err != nil {
		return err
	}
	if plan == nil {
		return nil
	}
	return s.applyRankingPortfolioRebuildPlan(ctx, definition, *plan)
}

func (s *Service) buildRankingPortfolioRebuildPlan(ctx context.Context, definition RankingPortfolioDefinition, snapshotDate string) (*rankingPortfolioRebuildPlan, error) {
	snapshotDate = strings.TrimSpace(snapshotDate)
	sourceRows, err := s.loadRankingSnapshotSourceRows(ctx, snapshotDate, resolveRankingExchanges(definition.Exchange))
	if err != nil {
		return nil, newRankingPortfolioPersistError(definition.ID, "load_ranking_snapshots", fmt.Sprintf("load ranking snapshots: %v", err), map[string]any{"snapshot_date": snapshotDate})
	}
	if len(sourceRows) == 0 {
		return nil, nil
	}
	previousConstituents, err := s.loadLatestRankingPortfolioConstituentsBeforeDate(ctx, definition.ID, snapshotDate)
	if err != nil {
		return nil, newRankingPortfolioPersistError(definition.ID, "load_previous_constituents", fmt.Sprintf("load previous ranking portfolio constituents: %v", err), map[string]any{"snapshot_date": snapshotDate})
	}
	constituents, err := s.selectRankingPortfolioConstituentsFromSnapshotRows(ctx, definition, sourceRows)
	if err != nil {
		return nil, newRankingPortfolioPersistError(definition.ID, "select_constituents", err.Error(), map[string]any{"snapshot_date": snapshotDate})
	}
	hasShortfall := len(constituents) < definition.MaxHoldings
	warningText := ""
	if hasShortfall {
		warningText = defaultRankingPortfolioWarningText
	}
	sourceTradeDate := s.resolveRankingPortfolioSourceTradeDateFromRows(sourceRows, snapshotDate)
	if sourceTradeDate == "" {
		sourceTradeDate = snapshotDate
	}
	marketPrices, err := s.buildRankingPortfolioMarketPricesFromSnapshotRows(ctx, definition, snapshotDate, sourceTradeDate, constituents, previousConstituents, sourceRows)
	if err != nil {
		return nil, err
	}
	snapshotTime := time.Date(parseSnapshotDate(snapshotDate).Year(), parseSnapshotDate(snapshotDate).Month(), parseSnapshotDate(snapshotDate).Day(), 15, 0, 0, 0, rankingSnapshotLocation).UTC()
	return &rankingPortfolioRebuildPlan{
		Date:            snapshotDate,
		SnapshotTime:    snapshotTime,
		Constituents:    constituents,
		MarketPrices:    marketPrices,
		HasShortfall:    hasShortfall,
		WarningText:     warningText,
		SourceTradeDate: sourceTradeDate,
	}, nil
}

func (s *Service) applyRankingPortfolioRebuildPlan(ctx context.Context, definition RankingPortfolioDefinition, plan rankingPortfolioRebuildPlan) error {
	now := time.Now().UTC()
	return s.repo.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		definition.UpdatedAt = now
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"code", "name", "exchange", "portfolio_variant", "benchmark_code", "benchmark_name",
				"max_holdings", "selection_rule", "selection_window", "excluded_boards", "weighting_method",
				"rebalance_rule", "trade_cost_rate", "method_note", "is_active", "updated_at",
			}),
		}).Create(&definition).Error; err != nil {
			return newRankingPortfolioPersistError(definition.ID, "upsert_definition", fmt.Sprintf("upsert ranking portfolio definition: %v", err), map[string]any{"snapshot_date": plan.Date})
		}
		snapshotVersion := buildRankingPortfolioSnapshotVersion(plan.Date)
		if err := deleteRankingPortfolioSnapshotVersion(tx, definition.ID, snapshotVersion); err != nil {
			return newRankingPortfolioPersistError(definition.ID, "cleanup_snapshot_version", err.Error(), map[string]any{"snapshot_date": plan.Date})
		}
		snapshot := RankingPortfolioSnapshot{
			DefinitionID:          definition.ID,
			SnapshotVersion:       snapshotVersion,
			BatchID:               buildRankingPortfolioBatchID(definition.ID, snapshotVersion),
			SnapshotDate:          plan.Date,
			RankingTime:           plan.SnapshotTime,
			HoldingsEffectiveTime: buildRankingPortfolioEffectiveTime(plan.SnapshotTime.In(rankingSnapshotLocation)),
			NavAsOfTime:           plan.SnapshotTime,
			SourceTradeDate:       plan.SourceTradeDate,
			BenchmarkCode:         definition.BenchmarkCode,
			BenchmarkName:         definition.BenchmarkName,
			ConstituentsCount:     len(plan.Constituents),
			HasShortfall:          plan.HasShortfall,
			WarningText:           plan.WarningText,
			MethodNote:            definition.MethodNote,
			CreatedAt:             now,
			UpdatedAt:             now,
		}
		if err := tx.Create(&snapshot).Error; err != nil {
			return newRankingPortfolioPersistError(definition.ID, "insert_snapshot", fmt.Sprintf("insert ranking portfolio snapshot: %v", err), map[string]any{"snapshot_date": plan.Date})
		}
		constituentRows := make([]RankingPortfolioSnapshotConstituent, 0, len(plan.Constituents))
		for _, item := range plan.Constituents {
			constituentRows = append(constituentRows, RankingPortfolioSnapshotConstituent{DefinitionID: definition.ID, SnapshotVersion: snapshotVersion, SnapshotDate: plan.Date, Rank: item.Rank, Code: item.Code, Name: item.Name, Exchange: item.Exchange, Board: item.Board, SourceRank: item.SourceRank, ConsecutiveDays: item.ConsecutiveDays, Weight: item.Weight, RankingScore: item.RankingScore, Opportunity: item.Opportunity, Risk: item.Risk, CreatedAt: now, UpdatedAt: now})
		}
		if len(constituentRows) > 0 {
			if err := tx.Create(&constituentRows).Error; err != nil {
				return newRankingPortfolioPersistError(definition.ID, "insert_constituents", fmt.Sprintf("insert ranking portfolio constituents: %v", err), map[string]any{"snapshot_date": plan.Date})
			}
		}
		if len(plan.MarketPrices) > 0 {
			if err := tx.Create(&plan.MarketPrices).Error; err != nil {
				return newRankingPortfolioPersistError(definition.ID, "insert_market_prices", fmt.Sprintf("insert ranking portfolio market prices: %v", err), map[string]any{"snapshot_date": plan.Date})
			}
		}
		result, err := buildRankingPortfolioResult(tx, definition, snapshotVersion, now)
		if err != nil {
			return newRankingPortfolioPersistError(definition.ID, "build_result", err.Error(), map[string]any{"snapshot_date": plan.Date})
		}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "definition_id"}, {Name: "snapshot_version"}}, DoUpdates: clause.AssignmentColumns([]string{"batch_id", "snapshot_date", "ranking_time", "holdings_effective_time", "nav_as_of_time", "source_trade_date", "benchmark_code", "benchmark_name", "latest_nav", "latest_portfolio_return", "current_constituent_count", "has_shortfall", "warning_text", "method_note", "series_json", "constituents_json", "latest_rebalance_json", "updated_at"})}).Create(result).Error; err != nil {
			return newRankingPortfolioPersistError(definition.ID, "upsert_result", fmt.Sprintf("upsert ranking portfolio result: %v", err), map[string]any{"snapshot_date": plan.Date})
		}
		return nil
	})
}

func parseSnapshotDate(snapshotDate string) time.Time {
	day, _ := time.ParseInLocation("2006-01-02", strings.TrimSpace(snapshotDate), rankingSnapshotLocation)
	return day
}

func (s *Service) loadRankingSnapshotSourceRows(ctx context.Context, snapshotDate string, exchanges []string) ([]rankingSnapshotSourceRow, error) {
	var rows []rankingSnapshotSourceRow
	query := s.repo.db.WithContext(ctx).Model(&RankingSnapshot{}).Select("id, code, name, exchange, rank, opportunity, risk, close_price, price_trade_date, snapshot_date").Where("snapshot_date = ?", strings.TrimSpace(snapshotDate))
	if len(exchanges) > 0 {
		query = query.Where("exchange IN ?", exchanges)
	}
	if err := query.Order("rank ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *Service) loadLatestRankingPortfolioConstituentsBeforeDate(ctx context.Context, definitionID string, snapshotDate string) ([]RankingPortfolioConstituentItem, error) {
	var previousSnapshot RankingPortfolioSnapshot
	if err := s.repo.db.WithContext(ctx).Where("definition_id = ? AND snapshot_date < ?", definitionID, strings.TrimSpace(snapshotDate)).Order("snapshot_date DESC, id DESC").First(&previousSnapshot).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	var rows []RankingPortfolioSnapshotConstituent
	if err := s.repo.db.WithContext(ctx).Where("definition_id = ? AND snapshot_version = ?", definitionID, previousSnapshot.SnapshotVersion).Order("rank ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]RankingPortfolioConstituentItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, RankingPortfolioConstituentItem{Rank: row.Rank, SourceRank: row.SourceRank, Code: row.Code, Name: row.Name, Exchange: row.Exchange, Board: row.Board, ConsecutiveDays: row.ConsecutiveDays, Weight: row.Weight, RankingScore: row.RankingScore, Opportunity: row.Opportunity, Risk: row.Risk})
	}
	return items, nil
}

func (s *Service) selectRankingPortfolioConstituentsFromSnapshotRows(ctx context.Context, definition RankingPortfolioDefinition, rows []rankingSnapshotSourceRow) ([]RankingPortfolioConstituentItem, error) {
	needsStreak := definition.SelectionRule == rankingPortfolioSelectionRuleTop10ByStreak
	rankingItems := make([]RankingItem, 0, len(rows))
	for _, row := range rows {
		item := RankingItem{Rank: row.Rank, Code: strings.TrimSpace(row.Code), Name: strings.TrimSpace(row.Name), Exchange: strings.ToUpper(strings.TrimSpace(row.Exchange)), Board: normalizeAShareRankingBoard(QuadrantScoreRecord{Code: strings.TrimSpace(row.Code), Board: "", Exchange: strings.ToUpper(strings.TrimSpace(row.Exchange))}), Opportunity: row.Opportunity, Risk: row.Risk}
		if needsStreak {
			days, err := s.repo.GetConsecutiveDays(ctx, item.Code, resolveRankingExchanges(definition.Exchange))
			if err != nil {
				return nil, err
			}
			item.ConsecutiveDays = days
		}
		rankingItems = append(rankingItems, item)
	}
	return buildRankingPortfolioConstituentItems(definition, rankingItems), nil
}

func (s *Service) resolveRankingPortfolioSourceTradeDateFromRows(rows []rankingSnapshotSourceRow, snapshotDate string) string {
	latest := ""
	for _, row := range rows {
		tradeDate := strings.TrimSpace(row.PriceTradeDate)
		if tradeDate == "" {
			tradeDate = strings.TrimSpace(snapshotDate)
		}
		if tradeDate == "" {
			continue
		}
		if latest == "" || tradeDate > latest {
			latest = tradeDate
		}
	}
	return latest
}

func (s *Service) buildRankingPortfolioMarketPricesFromSnapshotRows(ctx context.Context, definition RankingPortfolioDefinition, snapshotDate string, sourceTradeDate string, current []RankingPortfolioConstituentItem, previous []RankingPortfolioConstituentItem, rows []rankingSnapshotSourceRow) ([]RankingPortfolioMarketPrice, error) {
	snapshotByKey := make(map[string]rankingSnapshotSourceRow, len(rows))
	for _, row := range rows {
		snapshotByKey[snapshotPriceHintKey(row.Code, row.Exchange)] = row
	}
	needed := map[string]RankingPortfolioConstituentItem{}
	for _, item := range previous {
		needed[snapshotPriceHintKey(item.Code, item.Exchange)] = item
	}
	for _, item := range current {
		needed[snapshotPriceHintKey(item.Code, item.Exchange)] = item
	}
	prices := make([]RankingPortfolioMarketPrice, 0, len(needed))
	now := time.Now().UTC()
	snapshotVersion := buildRankingPortfolioSnapshotVersion(snapshotDate)
	for key, item := range needed {
		closePrice := 0.0
		priceTradeDate := sourceTradeDate
		if row, ok := snapshotByKey[key]; ok && row.ClosePrice > 0 {
			closePrice = row.ClosePrice
			if strings.TrimSpace(row.PriceTradeDate) != "" {
				priceTradeDate = strings.TrimSpace(row.PriceTradeDate)
			}
		}
		if closePrice <= 0 {
			lookup := s.resolveRankingPortfolioMarketPrice(ctx, item.Code, item.Exchange, sourceTradeDate)
			if lookup.ClosePrice > 0 {
				closePrice = lookup.ClosePrice
				priceTradeDate = lookup.TradeDate
			}
		}
		if closePrice <= 0 {
			return nil, newRankingPortfolioPersistError(definition.ID, "resolve_market_price", fmt.Sprintf("missing market close for %s(%s) on %s", item.Code, item.Exchange, sourceTradeDate), map[string]any{"snapshot_date": snapshotDate, "source_trade_date": sourceTradeDate, "code": item.Code, "exchange": item.Exchange})
		}
		prices = append(prices, RankingPortfolioMarketPrice{DefinitionID: definition.ID, SnapshotVersion: snapshotVersion, SnapshotDate: snapshotDate, Code: item.Code, Exchange: item.Exchange, ClosePrice: closePrice, PriceTradeDate: priceTradeDate, CreatedAt: now, UpdatedAt: now})
	}
	sort.Slice(prices, func(i, j int) bool {
		if prices[i].Exchange == prices[j].Exchange {
			return prices[i].Code < prices[j].Code
		}
		return prices[i].Exchange < prices[j].Exchange
	})
	return prices, nil
}

func (s *Service) rebuildRankingPortfolioResultForSnapshot(ctx context.Context, definition RankingPortfolioDefinition, snapshotDate string) error {
	snapshotDate = strings.TrimSpace(snapshotDate)
	var snapshot RankingPortfolioSnapshot
	if err := s.repo.db.WithContext(ctx).
		Where("definition_id = ? AND snapshot_date = ?", definition.ID, snapshotDate).
		Order("id DESC").
		First(&snapshot).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return s.rebuildRankingPortfolioFromRankingSnapshot(ctx, definition, snapshotDate)
		}
		return newRankingPortfolioPersistError(definition.ID, "load_snapshot", fmt.Sprintf("load ranking portfolio snapshot: %v", err), map[string]any{"snapshot_date": snapshotDate})
	}
	now := time.Now().UTC()
	return s.repo.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result, err := buildRankingPortfolioResult(tx, definition, snapshot.SnapshotVersion, now)
		if err != nil {
			return newRankingPortfolioPersistError(definition.ID, "build_result", err.Error(), map[string]any{"snapshot_date": snapshotDate, "snapshot_version": snapshot.SnapshotVersion})
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "definition_id"}, {Name: "snapshot_version"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"batch_id", "snapshot_date", "ranking_time", "holdings_effective_time", "nav_as_of_time", "source_trade_date",
				"benchmark_code", "benchmark_name", "latest_nav",
				"latest_portfolio_return",
				"current_constituent_count", "has_shortfall", "warning_text", "method_note",
				"series_json", "constituents_json", "latest_rebalance_json", "updated_at",
			}),
		}).Create(result).Error; err != nil {
			return newRankingPortfolioPersistError(definition.ID, "upsert_result", fmt.Sprintf("upsert ranking portfolio result: %v", err), map[string]any{"snapshot_date": snapshotDate, "snapshot_version": snapshot.SnapshotVersion})
		}
		return nil
	})
}

func (s *Service) autoRepairRankingPortfolioLag(ctx context.Context, taskLogID string, snapshotDate string) (int, error) {
	definitions := defaultRankingPortfolioDefinitionRecords(time.Now().UTC())
	repaired := 0
	var firstErr error
	for _, definition := range definitions {
		latestRankingDate, err := s.repo.GetLatestRankingSnapshotDateByExchange(ctx, definition.Exchange)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		latestPortfolioDate, err := s.repo.GetLatestRankingPortfolioResultDate(ctx, definition.ID)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if strings.TrimSpace(latestRankingDate) == "" || latestRankingDate <= strings.TrimSpace(latestPortfolioDate) {
			continue
		}
		repaired++
		message := fmt.Sprintf("latest_ranking_date=%s latest_portfolio_date=%s", latestRankingDate, latestPortfolioDate)
		now := time.Now().UTC()
		item := RankingPortfolioJobStatus{
			TaskLogID:           strings.TrimSpace(taskLogID),
			DefinitionID:        definition.ID,
			DefinitionCode:      definition.Code,
			DefinitionName:      definition.Name,
			Exchange:            definition.Exchange,
			SnapshotDate:        strings.TrimSpace(snapshotDate),
			SourceTradeDate:     "",
			Status:              "success",
			FailureStage:        "",
			FailureReason:       "",
			DetailsJSON:         "{}",
			AutoRepairTriggered: true,
			AutoRepairStatus:    "pending",
			AutoRepairMessage:   message,
			LastAutoRepairAt:    &now,
			CreatedAt:           now,
			UpdatedAt:           now,
		}
		if err := s.repo.UpsertRankingPortfolioJobStatus(ctx, item); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return repaired, firstErr
}

func (s *Service) saveRankingPortfolioBestEffort(ctx context.Context, records []QuadrantScoreRecord, computedAt time.Time, priceHints map[string]snapshotPriceHint, taskLogID string) {
	snapshotDate := rankingSnapshotDate(computedAt)
	if err := s.saveRankingPortfolio(ctx, records, computedAt, priceHints, taskLogID); err != nil {
		log.Printf("[quadrant] ranking portfolio save skipped: %v", err)
		if taskLogID != "" {
			_ = s.persistRankingPortfolioFailureStatus(ctx, taskLogID, err, snapshotDate, collectLatestSourceTradeDate(records), false, "")
		}
		return
	}
	if snapshotDate == "" {
		return
	}
	repaired, repairErr := s.autoRepairRankingPortfolioLag(ctx, taskLogID, snapshotDate)
	if repairErr != nil {
		log.Printf("[quadrant] ranking portfolio auto repair failed: %v", repairErr)
	}
	if repaired > 0 {
		log.Printf("[quadrant] ranking portfolio auto repair done: repaired=%d snapshot_date=%s", repaired, snapshotDate)
	}
}

func (s *Service) saveRankingPortfolio(ctx context.Context, records []QuadrantScoreRecord, computedAt time.Time, priceHints map[string]snapshotPriceHint, taskLogID string) error {
	if len(records) == 0 {
		return nil
	}
	snapshotDate := rankingSnapshotDate(computedAt)
	if snapshotDate == "" {
		return nil
	}
	definitions := listRankingPortfolioDefinitionsForRecords(records, time.Now().UTC())
	if len(definitions) == 0 {
		return nil
	}

	now := time.Now().UTC()
	var firstErr error
	for _, definition := range definitions {
		if err := s.saveSingleRankingPortfolio(ctx, definition, records, computedAt, snapshotDate, priceHints, taskLogID, now); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func (s *Service) saveSingleRankingPortfolio(ctx context.Context, definition RankingPortfolioDefinition, records []QuadrantScoreRecord, computedAt time.Time, snapshotDate string, priceHints map[string]snapshotPriceHint, taskLogID string, now time.Time) error {
	err := s.repo.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		definition.UpdatedAt = now
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"code", "name", "exchange", "portfolio_variant", "benchmark_code", "benchmark_name",
				"max_holdings", "selection_rule", "selection_window", "excluded_boards", "weighting_method",
				"rebalance_rule", "trade_cost_rate", "method_note", "is_active", "updated_at",
			}),
		}).Create(&definition).Error; err != nil {
			return newRankingPortfolioPersistError(definition.ID, "upsert_definition", fmt.Sprintf("upsert ranking portfolio definition: %v", err), nil)
		}

		snapshotVersion := buildRankingPortfolioSnapshotVersion(snapshotDate)
		if err := deleteRankingPortfolioSnapshotVersion(tx, definition.ID, snapshotVersion); err != nil {
			return newRankingPortfolioPersistError(definition.ID, "cleanup_snapshot_version", err.Error(), nil)
		}

		currentConstituents, err := s.selectRankingPortfolioConstituents(ctx, definition, records)
		if err != nil {
			return newRankingPortfolioPersistError(definition.ID, "select_constituents", err.Error(), nil)
		}
		hasShortfall := len(currentConstituents) < definition.MaxHoldings
		warningText := ""
		if hasShortfall {
			warningText = defaultRankingPortfolioWarningText
		}

		var previousSnapshot RankingPortfolioSnapshot
		prevFound := false
		if err := tx.Where("definition_id = ?", definition.ID).
			Order("snapshot_date DESC, id DESC").
			First(&previousSnapshot).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return newRankingPortfolioPersistError(definition.ID, "load_previous_snapshot", fmt.Sprintf("load previous ranking portfolio snapshot: %v", err), nil)
			}
		} else {
			prevFound = true
		}

		previousConstituents := []RankingPortfolioConstituentItem{}
		if prevFound {
			if err := tx.Model(&RankingPortfolioSnapshotConstituent{}).
				Where("definition_id = ? AND snapshot_version = ?", definition.ID, previousSnapshot.SnapshotVersion).
				Order("rank ASC, id ASC").
				Find(&previousConstituents).Error; err != nil {
				return newRankingPortfolioPersistError(definition.ID, "load_previous_constituents", fmt.Sprintf("load previous ranking portfolio constituents: %v", err), nil)
			}
		}

		definitionPriceHints := filterSnapshotPriceHintsByDefinitionExchange(priceHints, definition.Exchange)
		sourceTradeDate := collectLatestSourceTradeDate(records)
		if sourceTradeDate == "" {
			sourceTradeDate = s.resolveSourceTradeDate(ctx, definition.Exchange, computedAt, definitionPriceHints)
		}
		batchID := buildRankingPortfolioBatchID(definition.ID, snapshotVersion)
		effectiveTime := buildRankingPortfolioEffectiveTime(computedAt)
		snapshot := RankingPortfolioSnapshot{
			DefinitionID:          definition.ID,
			SnapshotVersion:       snapshotVersion,
			BatchID:               batchID,
			SnapshotDate:          snapshotDate,
			RankingTime:           computedAt,
			HoldingsEffectiveTime: effectiveTime,
			NavAsOfTime:           computedAt,
			SourceTradeDate:       sourceTradeDate,
			BenchmarkCode:         definition.BenchmarkCode,
			BenchmarkName:         definition.BenchmarkName,
			ConstituentsCount:     len(currentConstituents),
			HasShortfall:          hasShortfall,
			WarningText:           warningText,
			MethodNote:            definition.MethodNote,
			CreatedAt:             now,
			UpdatedAt:             now,
		}
		if err := tx.Create(&snapshot).Error; err != nil {
			return newRankingPortfolioPersistError(definition.ID, "insert_snapshot", fmt.Sprintf("insert ranking portfolio snapshot: %v", err), nil)
		}

		constituentRows := make([]RankingPortfolioSnapshotConstituent, 0, len(currentConstituents))
		for _, item := range currentConstituents {
			constituentRows = append(constituentRows, RankingPortfolioSnapshotConstituent{
				DefinitionID:    definition.ID,
				SnapshotVersion: snapshotVersion,
				SnapshotDate:    snapshotDate,
				Rank:            item.Rank,
				Code:            item.Code,
				Name:            item.Name,
				Exchange:        item.Exchange,
				Board:           item.Board,
				SourceRank:      item.SourceRank,
				ConsecutiveDays: item.ConsecutiveDays,
				Weight:          item.Weight,
				RankingScore:    item.RankingScore,
				Opportunity:     item.Opportunity,
				Risk:            item.Risk,
				CreatedAt:       now,
				UpdatedAt:       now,
			})
		}
		if len(constituentRows) > 0 {
			if err := tx.Create(&constituentRows).Error; err != nil {
				return newRankingPortfolioPersistError(definition.ID, "insert_constituents", fmt.Sprintf("insert ranking portfolio constituents: %v", err), nil)
			}
		}

		marketPrices, priceErr := s.buildRankingPortfolioMarketPrices(ctx, definition, snapshotVersion, sourceTradeDate, currentConstituents, previousConstituents, now, priceHints)
		if priceErr != nil {
			return priceErr
		}
		if len(marketPrices) > 0 {
			if err := tx.Create(&marketPrices).Error; err != nil {
				return newRankingPortfolioPersistError(definition.ID, "insert_market_prices", fmt.Sprintf("insert ranking portfolio market prices: %v", err), nil)
			}
		}

		result, err := buildRankingPortfolioResult(tx, definition, snapshotVersion, now)
		if err != nil {
			return newRankingPortfolioPersistError(definition.ID, "build_result", err.Error(), nil)
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "definition_id"}, {Name: "snapshot_version"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"batch_id", "snapshot_date", "ranking_time", "holdings_effective_time", "nav_as_of_time", "source_trade_date",
				"benchmark_code", "benchmark_name", "latest_nav",
				"latest_portfolio_return",
				"current_constituent_count", "has_shortfall", "warning_text", "method_note",
				"series_json", "constituents_json", "latest_rebalance_json", "updated_at",
			}),
		}).Create(result).Error; err != nil {
			return newRankingPortfolioPersistError(definition.ID, "upsert_result", fmt.Sprintf("upsert ranking portfolio result: %v", err), nil)
		}
		return nil
	})
	statusErr := s.persistRankingPortfolioDefinitionStatus(ctx, taskLogID, definition, snapshotDate, collectLatestSourceTradeDate(records), err, false, "")
	if statusErr != nil {
		log.Printf("[quadrant] ranking portfolio status update failed: definition=%s err=%v", definition.ID, statusErr)
	}
	return err
}

func decodeRankingPortfolioExcludedBoards(value string) map[string]bool {
	boards := []string{}
	if strings.TrimSpace(value) != "" {
		_ = json.Unmarshal([]byte(value), &boards)
	}
	result := make(map[string]bool, len(boards))
	for _, board := range boards {
		normalized := strings.ToUpper(strings.TrimSpace(board))
		if normalized != "" {
			result[normalized] = true
		}
	}
	return result
}

func buildRankingPortfolioConstituentItems(definition RankingPortfolioDefinition, rankingItems []RankingItem) []RankingPortfolioConstituentItem {
	excludedBoards := decodeRankingPortfolioExcludedBoards(definition.ExcludedBoards)
	filtered := make([]RankingItem, 0, len(rankingItems))
	for _, item := range rankingItems {
		if len(excludedBoards) > 0 && excludedBoards[strings.ToUpper(strings.TrimSpace(item.Board))] {
			continue
		}
		filtered = append(filtered, item)
	}

	if definition.SelectionWindow > 0 && len(filtered) > definition.SelectionWindow {
		filtered = filtered[:definition.SelectionWindow]
	}

	selected := filtered
	if definition.SelectionRule == rankingPortfolioSelectionRuleTop10ByStreak {
		selected = append([]RankingItem(nil), filtered...)
		sort.SliceStable(selected, func(i, j int) bool {
			if selected[i].ConsecutiveDays == selected[j].ConsecutiveDays {
				if selected[i].Rank == selected[j].Rank {
					return rankingRecordKey(QuadrantScoreRecord{Exchange: selected[i].Exchange, Code: selected[i].Code}) < rankingRecordKey(QuadrantScoreRecord{Exchange: selected[j].Exchange, Code: selected[j].Code})
				}
				return selected[i].Rank < selected[j].Rank
			}
			return selected[i].ConsecutiveDays > selected[j].ConsecutiveDays
		})
	}

	if len(selected) > definition.MaxHoldings {
		selected = selected[:definition.MaxHoldings]
	}

	items := make([]RankingPortfolioConstituentItem, 0, len(selected))
	weight := 0.0
	if len(selected) > 0 {
		weight = 1 / float64(len(selected))
	}
	for i, item := range selected {
		items = append(items, RankingPortfolioConstituentItem{
			Rank:            i + 1,
			SourceRank:      item.Rank,
			Code:            item.Code,
			Name:            item.Name,
			Exchange:        item.Exchange,
			Board:           item.Board,
			ConsecutiveDays: item.ConsecutiveDays,
			Weight:          weight,
			RankingScore:    item.RankingScore,
			Opportunity:     item.Opportunity,
			Risk:            item.Risk,
		})
	}
	return items
}

func buildRankingItemsFromRecords(ctx context.Context, repo *Repository, definition RankingPortfolioDefinition, records []QuadrantScoreRecord, limit int) []RankingItem {
	if limit <= 0 {
		limit = 20
	}

	filtered := make([]QuadrantScoreRecord, 0, len(records))
	exchanges := resolveRankingExchanges(definition.Exchange)
	displayExchange := strings.ToUpper(strings.TrimSpace(definition.Exchange))

	hasLiquidityData := false
	hasRankingScore := false
	for _, record := range records {
		recordExchange := strings.ToUpper(strings.TrimSpace(record.Exchange))
		if displayExchange == "HKEX" {
			if recordExchange != "HKEX" {
				continue
			}
			if record.AvgAmount5d > 0 {
				hasLiquidityData = true
			}
			continue
		}
		if recordExchange != "SSE" && recordExchange != "SZSE" {
			continue
		}
		if record.AvgAmount5d > 0 {
			hasLiquidityData = true
		}
		if record.RankingScore > 0 {
			hasRankingScore = true
		}
	}

	minAmount := 0.0
	if hasLiquidityData {
		if displayExchange == "HKEX" {
			minAmount = 2000
		} else {
			minAmount = 5000
		}
	}

	for _, record := range records {
		recordExchange := strings.ToUpper(strings.TrimSpace(record.Exchange))
		if displayExchange == "HKEX" {
			if recordExchange != "HKEX" {
				continue
			}
			if record.Quadrant != "机会" || record.Opportunity <= 0 {
				continue
			}
		} else {
			if recordExchange != "SSE" && recordExchange != "SZSE" {
				continue
			}
			if hasRankingScore {
				if record.RankingScore <= 0 {
					continue
				}
			} else if record.Quadrant != "机会" || record.Opportunity <= 0 {
				continue
			}
		}
		if strings.Contains(strings.ToUpper(strings.TrimSpace(record.Name)), "ST") {
			continue
		}
		if minAmount > 0 && record.AvgAmount5d < minAmount {
			continue
		}
		filtered = append(filtered, record)
	}

	selected := filtered
	if displayExchange == "HKEX" {
		sort.SliceStable(selected, func(i, j int) bool {
			if selected[i].Opportunity == selected[j].Opportunity {
				if selected[i].Risk == selected[j].Risk {
					return rankingRecordKey(selected[i]) < rankingRecordKey(selected[j])
				}
				return selected[i].Risk < selected[j].Risk
			}
			return selected[i].Opportunity > selected[j].Opportunity
		})
		if len(selected) > limit {
			selected = selected[:limit]
		}
	} else if hasRankingScore {
		selected = selectAShareBalancedRanking(selected, limit)
	} else {
		sort.SliceStable(selected, func(i, j int) bool {
			if selected[i].Opportunity == selected[j].Opportunity {
				if selected[i].Risk == selected[j].Risk {
					return rankingRecordKey(selected[i]) < rankingRecordKey(selected[j])
				}
				return selected[i].Risk < selected[j].Risk
			}
			return selected[i].Opportunity > selected[j].Opportunity
		})
		if len(selected) > limit {
			selected = selected[:limit]
		}
	}

	items := make([]RankingItem, 0, len(selected))
	for i, record := range selected {
		item := RankingItem{
			Rank:             i + 1,
			Code:             record.Code,
			Name:             record.Name,
			Exchange:         record.Exchange,
			Opportunity:      record.Opportunity,
			Risk:             record.Risk,
			Quadrant:         record.Quadrant,
			Trend:            record.Trend,
			Flow:             record.Flow,
			Revision:         record.Revision,
			Liquidity:        record.Liquidity,
			AvgAmount5d:      record.AvgAmount5d,
			Board:            normalizeAShareRankingBoard(record),
			RankingScore:     record.RankingScore,
			GlobalRankScore:  record.GlobalRankScore,
			BoardRankScore:   record.BoardRankScore,
			TradabilityScore: record.TradabilityScore,
		}
		if repo != nil {
			days, _ := repo.GetConsecutiveDays(ctx, record.Code, exchanges)
			item.ConsecutiveDays = days
			firstDateStr, _ := repo.GetFirstAppearedDate(ctx, record.Code, exchanges)
			if firstDateStr != "" {
				startPrice, _, _ := repo.GetEarliestAvailableClosePrice(ctx, record.Code, exchanges, firstDateStr)
				currentPrice, _, _ := repo.GetLatestAvailableClosePrice(ctx, record.Code, exchanges)
				if startPrice > 0 && currentPrice > 0 {
					pct := (currentPrice - startPrice) / startPrice * 100
					item.ReturnPct = &pct
				}
			}
		}
		items = append(items, item)
	}

	return items
}

func (s *Service) selectRankingPortfolioConstituents(ctx context.Context, definition RankingPortfolioDefinition, records []QuadrantScoreRecord) ([]RankingPortfolioConstituentItem, error) {
	limit := definition.SelectionWindow
	if limit < definition.MaxHoldings {
		limit = definition.MaxHoldings
	}
	if limit < 20 {
		limit = 20
	}
	if len(records) > 0 {
		return buildRankingPortfolioConstituentItems(definition, buildRankingItemsFromRecords(ctx, s.repo, definition, records, limit)), nil
	}
	ranking, err := s.buildRankingResponse(ctx, definition.Exchange, limit)
	if err != nil {
		return nil, fmt.Errorf("load ranking portfolio candidates: %w", err)
	}
	return buildRankingPortfolioConstituentItems(definition, ranking.Items), nil
}

func deleteRankingPortfolioSnapshotVersion(tx *gorm.DB, definitionID string, snapshotVersion string) error {
	if err := tx.Where("definition_id = ? AND snapshot_version = ?", definitionID, snapshotVersion).
		Delete(&RankingPortfolioResult{}).Error; err != nil {
		return fmt.Errorf("delete ranking portfolio result: %w", err)
	}
	if err := tx.Where("definition_id = ? AND snapshot_version = ?", definitionID, snapshotVersion).
		Delete(&RankingPortfolioMarketPrice{}).Error; err != nil {
		return fmt.Errorf("delete ranking portfolio market prices: %w", err)
	}
	if err := tx.Where("definition_id = ? AND snapshot_version = ?", definitionID, snapshotVersion).
		Delete(&RankingPortfolioSnapshotConstituent{}).Error; err != nil {
		return fmt.Errorf("delete ranking portfolio constituents: %w", err)
	}
	if err := tx.Where("definition_id = ? AND snapshot_version = ?", definitionID, snapshotVersion).
		Delete(&RankingPortfolioSnapshot{}).Error; err != nil {
		return fmt.Errorf("delete ranking portfolio snapshot: %w", err)
	}
	return nil
}

func selectRankingPortfolioConstituents(records []QuadrantScoreRecord, limit int) []RankingPortfolioConstituentItem {
	if limit <= 0 {
		limit = defaultRankingPortfolioMaxHoldings
	}
	if len(records) == 0 {
		return nil
	}

	hasLiquidityData := false
	hasRankingScore := false
	for _, record := range records {
		if record.Exchange != "SSE" && record.Exchange != "SZSE" {
			continue
		}
		if record.AvgAmount5d > 0 {
			hasLiquidityData = true
		}
		if record.RankingScore > 0 {
			hasRankingScore = true
		}
	}

	minAmount := 0.0
	if hasLiquidityData {
		minAmount = 5000
	}

	filtered := make([]QuadrantScoreRecord, 0, len(records))
	for _, record := range records {
		if record.Exchange != "SSE" && record.Exchange != "SZSE" {
			continue
		}
		if strings.Contains(strings.ToUpper(strings.TrimSpace(record.Name)), "ST") {
			continue
		}
		if normalizeAShareRankingBoard(record) == aShareBoardStar {
			continue
		}
		if minAmount > 0 && record.AvgAmount5d < minAmount {
			continue
		}
		filtered = append(filtered, record)
	}

	if hasRankingScore {
		sortByRankingScore(filtered)
	} else {
		sort.SliceStable(filtered, func(i, j int) bool {
			if filtered[i].Opportunity == filtered[j].Opportunity {
				if filtered[i].Risk == filtered[j].Risk {
					return rankingRecordKey(filtered[i]) < rankingRecordKey(filtered[j])
				}
				return filtered[i].Risk < filtered[j].Risk
			}
			return filtered[i].Opportunity > filtered[j].Opportunity
		})
	}

	if len(filtered) > limit {
		filtered = filtered[:limit]
	}

	items := make([]RankingPortfolioConstituentItem, 0, len(filtered))
	weight := 0.0
	if len(filtered) > 0 {
		weight = 1 / float64(len(filtered))
	}
	for i, record := range filtered {
		items = append(items, RankingPortfolioConstituentItem{
			Rank:         i + 1,
			SourceRank:   i + 1,
			Code:         record.Code,
			Name:         record.Name,
			Exchange:     record.Exchange,
			Board:        normalizeAShareRankingBoard(record),
			Weight:       weight,
			RankingScore: record.RankingScore,
			Opportunity:  record.Opportunity,
			Risk:         record.Risk,
		})
	}
	return items
}

func (s *Service) buildRankingPortfolioMarketPrices(ctx context.Context, definition RankingPortfolioDefinition, snapshotVersion string, sourceTradeDate string, current []RankingPortfolioConstituentItem, previous []RankingPortfolioConstituentItem, now time.Time, priceHints map[string]snapshotPriceHint) ([]RankingPortfolioMarketPrice, error) {
	seen := map[string]RankingPortfolioConstituentItem{}
	for _, item := range current {
		seen[snapshotPriceHintKey(item.Code, item.Exchange)] = item
	}
	for _, item := range previous {
		seen[snapshotPriceHintKey(item.Code, item.Exchange)] = item
	}

	prices := make([]RankingPortfolioMarketPrice, 0, len(seen))
	for key, item := range seen {
		closePrice := 0.0
		priceTradeDate := ""
		if hint, ok := priceHints[key]; ok {
			tradeDate := validPriceTradeDate(hint.TradeDate)
			if hint.ClosePrice > 0 && tradeDate == sourceTradeDate {
				closePrice = hint.ClosePrice
				priceTradeDate = tradeDate
			}
		}
		if closePrice <= 0 || priceTradeDate == "" {
			lookup := s.resolveRankingPortfolioMarketPrice(ctx, item.Code, item.Exchange, sourceTradeDate)
			if lookup.ClosePrice > 0 && lookup.TradeDate != "" {
				closePrice = lookup.ClosePrice
				priceTradeDate = lookup.TradeDate
			}
		}
		if closePrice <= 0 || priceTradeDate == "" {
			return nil, newRankingPortfolioPersistError(definition.ID, "resolve_market_price", fmt.Sprintf("missing market close for %s(%s) on %s", item.Code, item.Exchange, sourceTradeDate), map[string]any{"code": item.Code, "exchange": item.Exchange, "source_trade_date": sourceTradeDate})
		}
		prices = append(prices, RankingPortfolioMarketPrice{
			DefinitionID:    definition.ID,
			SnapshotVersion: snapshotVersion,
			SnapshotDate:    sourceTradeDate,
			Code:            item.Code,
			Exchange:        item.Exchange,
			ClosePrice:      closePrice,
			PriceTradeDate:  priceTradeDate,
			CreatedAt:       now,
			UpdatedAt:       now,
		})
	}
	sort.SliceStable(prices, func(i, j int) bool {
		if prices[i].Exchange == prices[j].Exchange {
			return prices[i].Code < prices[j].Code
		}
		return prices[i].Exchange < prices[j].Exchange
	})
	return prices, nil
}

func buildRankingPortfolioLatestRebalance(
	definition RankingPortfolioDefinition,
	currentSnapshot RankingPortfolioSnapshot,
	current []RankingPortfolioConstituentItem,
	previous []RankingPortfolioConstituentItem,
	priceByKey map[string]RankingPortfolioMarketPrice,
	latestEntryOpenPriceByKey map[string]float64,
) *RankingPortfolioLatestRebalance {
	currentByKey := make(map[string]RankingPortfolioConstituentItem, len(current))
	for _, item := range current {
		currentByKey[snapshotPriceHintKey(item.Code, item.Exchange)] = item
	}
	previousByKey := make(map[string]RankingPortfolioConstituentItem, len(previous))
	for _, item := range previous {
		previousByKey[snapshotPriceHintKey(item.Code, item.Exchange)] = item
	}

	keys := make([]string, 0, len(currentByKey)+len(previousByKey))
	seen := make(map[string]struct{}, len(currentByKey)+len(previousByKey))
	for key := range currentByKey {
		keys = append(keys, key)
		seen[key] = struct{}{}
	}
	for key := range previousByKey {
		if _, ok := seen[key]; ok {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	items := make([]RankingPortfolioRebalanceItem, 0, len(keys))
	for _, key := range keys {
		currentItem, hasCurrent := currentByKey[key]
		previousItem, hasPrevious := previousByKey[key]
		fromWeight := 0.0
		toWeight := 0.0
		baseItem := currentItem
		if hasPrevious {
			fromWeight = previousItem.Weight
			baseItem = previousItem
		}
		if hasCurrent {
			toWeight = currentItem.Weight
			baseItem = currentItem
		}
		weightDiff := fromWeight - toWeight
		if weightDiff < 0 {
			weightDiff = -weightDiff
		}
		if weightDiff < 1e-9 {
			continue
		}

		action := "buy"
		costMultiplier := 1 + definition.TradeCostRate
		if toWeight < fromWeight {
			action = "sell"
			costMultiplier = 1 - definition.TradeCostRate
		}

		priceRow := priceByKey[key]
		// 模拟买入/换仓参考价使用 T+1 开盘价（集合竞价），含成本价按单边 0.02% 计。
		referencePrice := roundRankingPortfolioFloat(priceRow.OpenPrice)
		referenceTradeDate := priceRow.EntryTradeDate
		if referenceTradeDate == "" {
			referenceTradeDate = priceRow.PriceTradeDate
		}
		referenceCostPrice := 0.0
		if referencePrice > 0 {
			referenceCostPrice = roundRankingPortfolioFloat(referencePrice * costMultiplier)
		}

		rebalanceItem := RankingPortfolioRebalanceItem{
			Action:             action,
			Code:               baseItem.Code,
			Name:               baseItem.Name,
			Exchange:           baseItem.Exchange,
			Board:              baseItem.Board,
			FromWeight:         roundRankingPortfolioFloat(fromWeight),
			ToWeight:           roundRankingPortfolioFloat(toWeight),
			ReferencePrice:     referencePrice,
			ReferenceCostPrice: referenceCostPrice,
			PriceTradeDate:     referenceTradeDate,
		}

		// For sell items, compute holding-period return using the most recent
		// entry open price (the price at which this stock was last bought into
		// the portfolio). Net sell proceeds = referencePrice * (1 - costRate).
		// Earlier entry prices from prior holding periods are excluded by the
		// caller, which supplies only the latest valid entry open price per key.
		if action == "sell" {
			if entryOpen, ok := latestEntryOpenPriceByKey[key]; ok && entryOpen > 0 && referencePrice > 0 {
				sellNet := referencePrice * (1 - definition.TradeCostRate)
				pct := roundRankingPortfolioPct((sellNet/entryOpen - 1) * 100)
				rebalanceItem.EntryOpenPrice = roundRankingPortfolioFloat(entryOpen)
				rebalanceItem.SoldReturnPct = &pct
			}
		}

		items = append(items, rebalanceItem)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Action != items[j].Action {
			return items[i].Action == "sell"
		}
		if items[i].Exchange != items[j].Exchange {
			return items[i].Exchange < items[j].Exchange
		}
		return items[i].Code < items[j].Code
	})

	return &RankingPortfolioLatestRebalance{
		SnapshotDate:    currentSnapshot.SnapshotDate,
		SourceTradeDate: currentSnapshot.SourceTradeDate,
		RankingTime:     currentSnapshot.RankingTime.UTC().Format(time.RFC3339),
		EffectiveTime:   currentSnapshot.HoldingsEffectiveTime.UTC().Format(time.RFC3339),
		TradeCostRate:   roundRankingPortfolioCostRate(definition.TradeCostRate),
		ChangeCount:     len(items),
		Items:           items,
	}
}

// rankingPortfolioDayPrice carries both the T+1 open (entry) price and the
// valuation-day close price for a single symbol within one snapshot version.
type rankingPortfolioDayPrice struct {
	Open  float64
	Close float64
}

func buildRankingPortfolioResult(tx *gorm.DB, definition RankingPortfolioDefinition, latestSnapshotVersion string, now time.Time) (*RankingPortfolioResult, error) {
	var snapshots []RankingPortfolioSnapshot
	if err := tx.Where("definition_id = ?", definition.ID).
		Order("snapshot_date ASC, id ASC").
		Find(&snapshots).Error; err != nil {
		return nil, fmt.Errorf("list ranking portfolio snapshots: %w", err)
	}
	if len(snapshots) == 0 {
		return nil, fmt.Errorf("ranking portfolio snapshots unavailable")
	}

	constituentsByVersion := map[string][]RankingPortfolioConstituentItem{}
	var constituentRows []RankingPortfolioSnapshotConstituent
	if err := tx.Where("definition_id = ?", definition.ID).
		Order("snapshot_date ASC, rank ASC, id ASC").
		Find(&constituentRows).Error; err != nil {
		return nil, fmt.Errorf("list ranking portfolio constituents: %w", err)
	}
	for _, row := range constituentRows {
		constituentsByVersion[row.SnapshotVersion] = append(constituentsByVersion[row.SnapshotVersion], RankingPortfolioConstituentItem{
			Rank:            row.Rank,
			SourceRank:      row.SourceRank,
			Code:            row.Code,
			Name:            row.Name,
			Exchange:        row.Exchange,
			Board:           row.Board,
			ConsecutiveDays: row.ConsecutiveDays,
			Weight:          row.Weight,
			RankingScore:    row.RankingScore,
			Opportunity:     row.Opportunity,
			Risk:            row.Risk,
		})
	}

	priceByVersion := map[string]map[string]rankingPortfolioDayPrice{}
	var priceRows []RankingPortfolioMarketPrice
	if err := tx.Where("definition_id = ?", definition.ID).
		Order("snapshot_date ASC, exchange ASC, code ASC, id ASC").
		Find(&priceRows).Error; err != nil {
		return nil, fmt.Errorf("list ranking portfolio market prices: %w", err)
	}
	for _, row := range priceRows {
		if _, ok := priceByVersion[row.SnapshotVersion]; !ok {
			priceByVersion[row.SnapshotVersion] = map[string]rankingPortfolioDayPrice{}
		}
		priceByVersion[row.SnapshotVersion][snapshotPriceHintKey(row.Code, row.Exchange)] = rankingPortfolioDayPrice{
			Open:  row.OpenPrice,
			Close: row.ClosePrice,
		}
	}

	series := make([]RankingPortfolioSeriesPoint, 0, len(snapshots))
	firstSnapshot := snapshots[0]
	series = append(series, RankingPortfolioSeriesPoint{
		Date:                    firstSnapshot.SnapshotDate,
		SourceTradeDate:         firstSnapshot.SourceTradeDate,
		Nav:                     1,
		PortfolioReturnPct:      0,
		DailyPortfolioReturnPct: 0,
		DrawdownPct:             0,
		HoldingCount:            0,
	})

	activeHoldings := []RankingPortfolioConstituentItem{}
	peakNav := 1.0
	for i := 1; i < len(snapshots); i++ {
		prevSnapshot := snapshots[i-1]
		currentSnapshot := snapshots[i]
		// nextHoldings 是在 prevSnapshot（信号日 T）收盘后选出的成分股，
		// 它们在 currentSnapshot（建仓/估值日 T+1）当天开盘买入、收盘估值。
		nextHoldings := constituentsByVersion[prevSnapshot.SnapshotVersion]
		// 当日收益按 open→close 计算（开盘买入价 → 当日收盘价），等权。
		portfolioReturn := calculateRankingPortfolioPeriodReturn(nextHoldings, priceByVersion[currentSnapshot.SnapshotVersion])
		// 交易成本只在买入腿与卖出腿发生：新建仓买入 + 被淘汰卖出；
		// 连续在仓的股票权重差为 0，不重复扣成本。
		turnover := calculateRankingPortfolioTradeRatio(activeHoldings, nextHoldings)
		costRatio := definition.TradeCostRate * turnover
		netDailyReturn := (1-costRatio)*(1+portfolioReturn) - 1

		prevPoint := series[len(series)-1]
		nav := prevPoint.Nav * (1 + netDailyReturn)
		if nav > peakNav {
			peakNav = nav
		}
		drawdownPct := 0.0
		if peakNav > 0 {
			drawdownPct = roundRankingPortfolioPct((nav/peakNav - 1) * 100)
		}

		series = append(series, RankingPortfolioSeriesPoint{
			Date:                    currentSnapshot.SnapshotDate,
			SourceTradeDate:         currentSnapshot.SourceTradeDate,
			Nav:                     roundRankingPortfolioFloat(nav),
			PortfolioReturnPct:      roundRankingPortfolioPct((nav - 1) * 100),
			DailyPortfolioReturnPct: roundRankingPortfolioPct(netDailyReturn * 100),
			DrawdownPct:             drawdownPct,
			HoldingCount:            len(nextHoldings),
		})
		activeHoldings = append([]RankingPortfolioConstituentItem(nil), nextHoldings...)
	}

	latestSnapshot := snapshots[len(snapshots)-1]
	latestConstituents := constituentsByVersion[latestSnapshot.SnapshotVersion]
	seriesJSON := mustMarshal(series)
	constituentsJSON := mustMarshal(latestConstituents)
	latestPoint := series[len(series)-1]
	previousConstituents := []RankingPortfolioConstituentItem{}
	if len(snapshots) > 1 {
		previousConstituents = constituentsByVersion[snapshots[len(snapshots)-2].SnapshotVersion]
	}
	latestPriceByKey := map[string]RankingPortfolioMarketPrice{}
	for _, row := range priceRows {
		if row.SnapshotVersion != latestSnapshot.SnapshotVersion {
			continue
		}
		latestPriceByKey[snapshotPriceHintKey(row.Code, row.Exchange)] = row
	}

	// Build a map of the most-recent entry open price per symbol across all
	// historical snapshots. This is used to compute holding-period return for
	// sold stocks. We walk priceRows in DESC order of entry_trade_date (already
	// loaded ASC above, so we reverse) and keep only the first valid hit per key.
	latestEntryOpenPriceByKey := map[string]float64{}
	for i := len(priceRows) - 1; i >= 0; i-- {
		row := priceRows[i]
		if row.OpenPrice <= 0 || row.EntryTradeDate == "" {
			continue
		}
		k := snapshotPriceHintKey(row.Code, row.Exchange)
		if _, exists := latestEntryOpenPriceByKey[k]; !exists {
			latestEntryOpenPriceByKey[k] = row.OpenPrice
		}
	}

	latestRebalanceJSON := mustMarshal(buildRankingPortfolioLatestRebalance(definition, latestSnapshot, latestConstituents, previousConstituents, latestPriceByKey, latestEntryOpenPriceByKey))

	return &RankingPortfolioResult{
		DefinitionID:            definition.ID,
		SnapshotVersion:         latestSnapshotVersion,
		BatchID:                 buildRankingPortfolioBatchID(definition.ID, latestSnapshotVersion),
		SnapshotDate:            latestSnapshot.SnapshotDate,
		RankingTime:             latestSnapshot.RankingTime,
		HoldingsEffectiveTime:   latestSnapshot.HoldingsEffectiveTime,
		NavAsOfTime:             latestSnapshot.NavAsOfTime,
		SourceTradeDate:         latestSnapshot.SourceTradeDate,
		BenchmarkCode:           latestSnapshot.BenchmarkCode,
		BenchmarkName:           latestSnapshot.BenchmarkName,
		LatestNav:               latestPoint.Nav,
		LatestPortfolioReturn:   latestPoint.PortfolioReturnPct,
		CurrentConstituentCount: len(latestConstituents),
		HasShortfall:            latestSnapshot.HasShortfall,
		WarningText:             latestSnapshot.WarningText,
		MethodNote:              latestSnapshot.MethodNote,
		SeriesJSON:              seriesJSON,
		ConstituentsJSON:        constituentsJSON,
		LatestRebalanceJSON:     latestRebalanceJSON,
		CreatedAt:               now,
		UpdatedAt:               now,
	}, nil
}

type rankingPortfolioSummaryMetrics struct {
	InceptionTradeDate    string
	InceptionDays         int
	LatestDailyReturnPct  *float64
	CurrentMonthReturnPct *float64
	MaxDrawdownPct        *float64
	VolatilityPct         *float64
	DailyWinRatePct       *float64
}

func buildRankingPortfolioSummaryMetrics(series []RankingPortfolioSeriesPoint) rankingPortfolioSummaryMetrics {
	if len(series) == 0 {
		return rankingPortfolioSummaryMetrics{}
	}
	firstTradeDate := rankingPortfolioSeriesTradeDate(series[0])
	latestTradeDate := rankingPortfolioSeriesTradeDate(series[len(series)-1])
	metrics := rankingPortfolioSummaryMetrics{InceptionTradeDate: firstTradeDate}
	if firstTradeDate != "" && latestTradeDate != "" {
		metrics.InceptionDays = rankingPortfolioInclusiveDays(firstTradeDate, latestTradeDate)
	}
	if len(series) > 2 {
		// 「昨日收益率」固定取「最新结算交易日的前一天」(T-1)。
		// series[0] 为建仓起点（NAV=1，日收益恒为 0，不是真实交易日收益），
		// 因此至少需要 3 个点（起点 + T-1 + T）才有真实的 T-1 日收益。
		// series[len-1] 为最新结算日 T，series[len-2] 即 T-1 的日收益。
		yesterdayDaily := roundRankingPortfolioPct(series[len(series)-2].DailyPortfolioReturnPct)
		metrics.LatestDailyReturnPct = &yesterdayDaily
	}

	maxDrawdown := 0.0
	dailyReturns := make([]float64, 0, max(len(series)-1, 0))
	winDays := 0
	for i := 1; i < len(series); i++ {
		dailyReturn := series[i].DailyPortfolioReturnPct / 100
		dailyReturns = append(dailyReturns, dailyReturn)
		if series[i].DailyPortfolioReturnPct > 0 {
			winDays++
		}
		if series[i].DrawdownPct < maxDrawdown {
			maxDrawdown = series[i].DrawdownPct
		}
	}
	maxDrawdown = roundRankingPortfolioPct(maxDrawdown)
	metrics.MaxDrawdownPct = &maxDrawdown
	if len(dailyReturns) > 0 {
		winRate := roundRankingPortfolioPct(float64(winDays) / float64(len(dailyReturns)) * 100)
		metrics.DailyWinRatePct = &winRate
	}
	if len(dailyReturns) >= 2 {
		volatility := roundRankingPortfolioPct(calculateRankingPortfolioAnnualizedVolatility(dailyReturns) * 100)
		metrics.VolatilityPct = &volatility
	}
	if monthReturn, ok := calculateRankingPortfolioCurrentMonthReturn(series); ok {
		monthReturn = roundRankingPortfolioPct(monthReturn)
		metrics.CurrentMonthReturnPct = &monthReturn
	}
	return metrics
}

func rankingPortfolioSeriesTradeDate(point RankingPortfolioSeriesPoint) string {
	if normalized := normalizeSourceTradeDate(point.SourceTradeDate); normalized != "" {
		return normalized
	}
	return normalizeSourceTradeDate(point.Date)
}

func rankingPortfolioInclusiveDays(startDate string, endDate string) int {
	startAt, err1 := time.ParseInLocation("2006-01-02", strings.TrimSpace(startDate), rankingSnapshotLocation)
	endAt, err2 := time.ParseInLocation("2006-01-02", strings.TrimSpace(endDate), rankingSnapshotLocation)
	if err1 != nil || err2 != nil || endAt.Before(startAt) {
		return 0
	}
	return int(endAt.Sub(startAt).Hours()/24) + 1
}

func calculateRankingPortfolioCurrentMonthReturn(series []RankingPortfolioSeriesPoint) (float64, bool) {
	if len(series) == 0 {
		return 0, false
	}
	latestTradeDate := rankingPortfolioSeriesTradeDate(series[len(series)-1])
	latestAt, err := time.ParseInLocation("2006-01-02", latestTradeDate, rankingSnapshotLocation)
	if err != nil {
		return 0, false
	}
	firstIndexOfMonth := -1
	for i, point := range series {
		tradeDate := rankingPortfolioSeriesTradeDate(point)
		tradeAt, parseErr := time.ParseInLocation("2006-01-02", tradeDate, rankingSnapshotLocation)
		if parseErr != nil {
			continue
		}
		if tradeAt.Year() == latestAt.Year() && tradeAt.Month() == latestAt.Month() {
			firstIndexOfMonth = i
			break
		}
	}
	if firstIndexOfMonth < 0 {
		return 0, false
	}
	latestNav := series[len(series)-1].Nav
	if latestNav <= 0 {
		return 0, false
	}
	if firstIndexOfMonth == 0 {
		return (latestNav - 1) * 100, true
	}
	baseNav := series[firstIndexOfMonth-1].Nav
	if baseNav <= 0 {
		return 0, false
	}
	return (latestNav/baseNav - 1) * 100, true
}

func calculateRankingPortfolioAnnualizedVolatility(dailyReturns []float64) float64 {
	if len(dailyReturns) < 2 {
		return 0
	}
	mean := 0.0
	for _, item := range dailyReturns {
		mean += item
	}
	mean /= float64(len(dailyReturns))
	variance := 0.0
	for _, item := range dailyReturns {
		diff := item - mean
		variance += diff * diff
	}
	variance /= float64(len(dailyReturns) - 1)
	if variance < 0 {
		variance = 0
	}
	return math.Sqrt(variance) * math.Sqrt(rankingPortfolioAnnualTradingDays)
}

// calculateRankingPortfolioPeriodReturn computes the equal-weighted single-day
// portfolio return using the open→close move of each holding on its valuation day.
// open = T+1 集合竞价买入价，close = T+1 收盘估值价。缺价的成分股被跳过并对剩余
// 成分股重新归一权重。
func calculateRankingPortfolioPeriodReturn(holdings []RankingPortfolioConstituentItem, dayPrices map[string]rankingPortfolioDayPrice) float64 {
	if len(holdings) == 0 || len(dayPrices) == 0 {
		return 0
	}
	weightedSum := 0.0
	weightSum := 0.0
	for _, holding := range holdings {
		key := snapshotPriceHintKey(holding.Code, holding.Exchange)
		price, ok := dayPrices[key]
		if !ok || price.Open <= 0 || price.Close <= 0 {
			continue
		}
		weightedSum += holding.Weight * (price.Close/price.Open - 1)
		weightSum += holding.Weight
	}
	if weightSum <= 0 {
		return 0
	}
	return weightedSum / weightSum
}

func calculateRankingPortfolioTradeRatio(previous []RankingPortfolioConstituentItem, current []RankingPortfolioConstituentItem) float64 {
	weights := map[string]float64{}
	for _, item := range previous {
		weights[snapshotPriceHintKey(item.Code, item.Exchange)] -= item.Weight
	}
	for _, item := range current {
		weights[snapshotPriceHintKey(item.Code, item.Exchange)] += item.Weight
	}
	turnover := 0.0
	for _, diff := range weights {
		if diff < 0 {
			diff = -diff
		}
		turnover += diff
	}
	return turnover
}

func roundRankingPortfolioFloat(value float64) float64 {
	return mathRound(value, 6)
}

func roundRankingPortfolioPct(value float64) float64 {
	return mathRound(value, 4)
}

func roundRankingPortfolioCostRate(value float64) float64 {
	return mathRound(value, rankingPortfolioTradeCostDisplayDigits)
}

func mathRound(value float64, digits int) float64 {
	if digits < 0 {
		return value
	}
	shift := 1.0
	for i := 0; i < digits; i++ {
		shift *= 10
	}
	if value >= 0 {
		return float64(int(value*shift+0.5)) / shift
	}
	return float64(int(value*shift-0.5)) / shift
}

func buildEmptyRankingPortfolioResponse(definition RankingPortfolioDefinition) RankingPortfolioResponse {
	return RankingPortfolioResponse{
		Meta: RankingPortfolioMeta{
			DefinitionID:      definition.ID,
			DefinitionCode:    definition.Code,
			Name:              definition.Name,
			Exchange:          definition.Exchange,
			PortfolioVariant:  definition.PortfolioVariant,
			SelectionRule:     definition.SelectionRule,
			SelectionWindow:   definition.SelectionWindow,
			RebalanceRule:     definition.RebalanceRule,
			CalculationMethod: rankingPortfolioCalculationMethodOpen,
			PriceBasis:        rankingPortfolioPriceBasisOpen,
			LatestNav:         1,
			MethodNote:        "",
		},
		Series:          []RankingPortfolioSeriesPoint{},
		Constituents:    []RankingPortfolioConstituentItem{},
		LatestRebalance: nil,
	}
}

func (s *Service) buildCurrentRankingPortfolioSelection(ctx context.Context, definition RankingPortfolioDefinition) ([]RankingPortfolioConstituentItem, time.Time, string, error) {
	limit := definition.SelectionWindow
	if limit < definition.MaxHoldings {
		limit = definition.MaxHoldings
	}
	if limit < 20 {
		limit = 20
	}
	ranking, err := s.buildRankingResponse(ctx, definition.Exchange, limit)
	if err != nil {
		return nil, time.Time{}, "", err
	}
	if len(ranking.Items) == 0 {
		return nil, time.Time{}, "", nil
	}
	latestComputedAt := time.Time{}
	if strings.TrimSpace(ranking.Meta.ComputedAt) != "" {
		if parsed, parseErr := time.Parse(time.RFC3339, ranking.Meta.ComputedAt); parseErr == nil {
			latestComputedAt = parsed.UTC()
		}
	}
	return buildRankingPortfolioConstituentItems(definition, ranking.Items), latestComputedAt, normalizeSourceTradeDate(ranking.Meta.SourceTradeDate), nil
}

func (s *Service) applyCurrentRankingPortfolioSelection(ctx context.Context, definition RankingPortfolioDefinition, item *RankingPortfolioResponse, resultRankingTime time.Time) error {
	currentConstituents, currentComputedAt, currentSourceTradeDate, err := s.buildCurrentRankingPortfolioSelection(ctx, definition)
	if err != nil {
		return err
	}
	if len(currentConstituents) == 0 {
		return nil
	}

	item.Constituents = currentConstituents
	item.Meta.CurrentConstituentCount = len(currentConstituents)
	item.Meta.CurrentConstituentComputedAt = currentComputedAt.UTC().Format(time.RFC3339)
	item.Meta.HasShortfall = len(currentConstituents) < definition.MaxHoldings
	if item.Meta.HasShortfall {
		item.Meta.WarningText = defaultRankingPortfolioWarningText
	} else {
		item.Meta.WarningText = ""
	}

	if !currentComputedAt.IsZero() {
		effectiveTime := buildRankingPortfolioCurrentEffectiveTime(currentComputedAt)
		item.Meta.CurrentConstituentEffectiveTime = effectiveTime.UTC().Format(time.RFC3339)
		item.Meta.CurrentConstituentSourceDate = currentSourceTradeDate
		if item.Meta.CurrentConstituentSourceDate == "" {
			item.Meta.CurrentConstituentSourceDate = buildRankingPortfolioCurrentSourceDate(effectiveTime)
		}
		if item.Meta.SourceTradeDate != "" && item.Meta.CurrentConstituentSourceDate != "" {
			item.Meta.IsSameBatchAsPerformance = item.Meta.SourceTradeDate == item.Meta.CurrentConstituentSourceDate
		}
		if !item.Meta.IsSameBatchAsPerformance {
			item.Meta.BatchMismatchReason = "当前成分股已按最新收盘榜单更新，收益曲线仍展示上一已物化批次。"
			if currentComputedAt.After(resultRankingTime) {
				item.LatestRebalance = nil
			}
		}
	}

	enrichSourceTradeDate := item.Meta.CurrentConstituentSourceDate
	if enrichSourceTradeDate == "" {
		enrichSourceTradeDate = item.Meta.SourceTradeDate
	}
	if err := s.enrichRankingPortfolioCurrentConstituents(ctx, definition, item.Constituents, enrichSourceTradeDate); err != nil {
		return err
	}

	// 汇总当前成分股的建仓日(entry_date)与最近实时刷新时间(realtime_as_of)。
	latestEntryDate := ""
	latestRealtime := ""
	for _, c := range item.Constituents {
		if c.EntryTradeDate != "" && c.EntryTradeDate > latestEntryDate {
			latestEntryDate = c.EntryTradeDate
		}
		if c.LatestQuoteTime != "" && c.LatestQuoteTime > latestRealtime {
			latestRealtime = c.LatestQuoteTime
		}
	}
	item.Meta.EntryDate = latestEntryDate
	item.Meta.RealtimeAsOf = latestRealtime

	return nil
}

func (s *Service) enrichRankingPortfolioCurrentConstituents(ctx context.Context, definition RankingPortfolioDefinition, items []RankingPortfolioConstituentItem, latestSourceTradeDate string) error {
	latestSourceTradeDate = normalizeSourceTradeDate(latestSourceTradeDate)
	if len(items) == 0 || latestSourceTradeDate == "" || s.repo == nil {
		return nil
	}

	var snapshots []RankingPortfolioSnapshot
	if err := s.repo.db.WithContext(ctx).
		Where("definition_id = ?", definition.ID).
		Order("snapshot_date ASC, id ASC").
		Find(&snapshots).Error; err != nil {
		return fmt.Errorf("list ranking portfolio snapshots for constituent enrichment: %w", err)
	}

	membershipByVersion := map[string]map[string]struct{}{}
	if len(snapshots) > 0 {
		var rows []RankingPortfolioSnapshotConstituent
		if err := s.repo.db.WithContext(ctx).
			Where("definition_id = ?", definition.ID).
			Find(&rows).Error; err != nil {
			return fmt.Errorf("list ranking portfolio constituents for enrichment: %w", err)
		}
		for _, row := range rows {
			if _, ok := membershipByVersion[row.SnapshotVersion]; !ok {
				membershipByVersion[row.SnapshotVersion] = map[string]struct{}{}
			}
			membershipByVersion[row.SnapshotVersion][snapshotPriceHintKey(row.Code, row.Exchange)] = struct{}{}
		}
	}

	for index := range items {
		entryTradeDate := latestSourceTradeDate
		key := snapshotPriceHintKey(items[index].Code, items[index].Exchange)
		for i := len(snapshots) - 1; i >= 0; i-- {
			membership := membershipByVersion[snapshots[i].SnapshotVersion]
			if _, ok := membership[key]; !ok {
				continue
			}
			entryTradeDate = normalizeSourceTradeDate(snapshots[i].SourceTradeDate)
			if entryTradeDate == "" {
				entryTradeDate = normalizeSourceTradeDate(snapshots[i].SnapshotDate)
			}
			for j := i - 1; j >= 0; j-- {
				previousMembership := membershipByVersion[snapshots[j].SnapshotVersion]
				if _, ok := previousMembership[key]; !ok {
					break
				}
				entryTradeDate = normalizeSourceTradeDate(snapshots[j].SourceTradeDate)
				if entryTradeDate == "" {
					entryTradeDate = normalizeSourceTradeDate(snapshots[j].SnapshotDate)
				}
			}
			break
		}

		// 买入价 = 当前连续在仓周期对应 entry_date(T+1) 的 9:25 集合竞价开盘价。
		// entryTradeDate 此处为该周期建仓信号日(T)；开盘价按 entry_trade_date >= T 取首个已回填值。
		entryPrice, resolvedEntryTradeDate, err := s.repo.GetRankingPortfolioEntryOpenPrice(ctx, definition.ID, items[index].Code, items[index].Exchange, entryTradeDate)
		if err != nil {
			return fmt.Errorf("resolve constituent entry open price for %s(%s): %w", items[index].Code, items[index].Exchange, err)
		}

		// 最新价 = 每半小时刷新的实时价；非交易时段沿用最近一次。
		// 实时价缺失（如盘前尚未刷新）时，回退到最新收盘价用于展示。
		latestPrice := 0.0
		latestQuoteTime := ""
		realtimePrice, quoteAt, rtErr := s.repo.GetRankingPortfolioRealtimePrice(ctx, items[index].Code, items[index].Exchange)
		if rtErr != nil {
			return fmt.Errorf("resolve constituent realtime price for %s(%s): %w", items[index].Code, items[index].Exchange, rtErr)
		}
		latestClosePrice, resolvedLatestTradeDate, err := s.repo.GetLatestRankingSnapshotClosePriceByTradeDateOnOrBefore(ctx, items[index].Code, items[index].Exchange, latestSourceTradeDate)
		if err != nil {
			return fmt.Errorf("resolve constituent latest close price for %s(%s): %w", items[index].Code, items[index].Exchange, err)
		}
		if realtimePrice > 0 {
			latestPrice = realtimePrice
			if !quoteAt.IsZero() {
				latestQuoteTime = quoteAt.UTC().Format(time.RFC3339)
			}
		} else if latestClosePrice > 0 {
			latestPrice = latestClosePrice
		}

		items[index].EntryTradeDate = normalizeSourceTradeDate(resolvedEntryTradeDate)
		items[index].EntryPrice = roundRankingPortfolioFloat(entryPrice)
		items[index].LatestTradeDate = normalizeSourceTradeDate(resolvedLatestTradeDate)
		items[index].LatestClosePrice = roundRankingPortfolioFloat(latestClosePrice)
		items[index].LatestPrice = roundRankingPortfolioFloat(latestPrice)
		items[index].LatestQuoteTime = latestQuoteTime

		// 开盘价未到（T+1 9:25 前）→ 买入价未知，涨幅置空、标记 pending，
		// 绝不用收盘价兜底买入价。
		if entryPrice <= 0 {
			items[index].EntryPricePending = true
			items[index].LatestReturnPct = nil
			continue
		}
		if latestPrice > 0 {
			latestReturnPct := roundRankingPortfolioPct((latestPrice/entryPrice - 1) * 100)
			items[index].LatestReturnPct = &latestReturnPct
		}
	}

	return nil
}

func buildRankingPortfolioResponse(definition RankingPortfolioDefinition, result RankingPortfolioResult) (*RankingPortfolioResponse, error) {
	series := []RankingPortfolioSeriesPoint{}
	if strings.TrimSpace(result.SeriesJSON) != "" {
		if err := json.Unmarshal([]byte(result.SeriesJSON), &series); err != nil {
			return nil, fmt.Errorf("decode ranking portfolio series: %w", err)
		}
	}
	constituents := []RankingPortfolioConstituentItem{}
	if strings.TrimSpace(result.ConstituentsJSON) != "" {
		if err := json.Unmarshal([]byte(result.ConstituentsJSON), &constituents); err != nil {
			return nil, fmt.Errorf("decode ranking portfolio constituents: %w", err)
		}
	}
	var latestRebalance *RankingPortfolioLatestRebalance
	if strings.TrimSpace(result.LatestRebalanceJSON) != "" {
		if err := json.Unmarshal([]byte(result.LatestRebalanceJSON), &latestRebalance); err != nil {
			return nil, fmt.Errorf("decode ranking portfolio latest rebalance: %w", err)
		}
	}
	summaryMetrics := buildRankingPortfolioSummaryMetrics(series)

	return &RankingPortfolioResponse{
		Meta: RankingPortfolioMeta{
			DefinitionID:             definition.ID,
			DefinitionCode:           definition.Code,
			Name:                     definition.Name,
			Exchange:                 definition.Exchange,
			PortfolioVariant:         definition.PortfolioVariant,
			SelectionRule:            definition.SelectionRule,
			SelectionWindow:          definition.SelectionWindow,
			RebalanceRule:            definition.RebalanceRule,
			CalculationMethod:        rankingPortfolioCalculationMethodOpen,
			PriceBasis:               rankingPortfolioPriceBasisOpen,
			BatchID:                  result.BatchID,
			SnapshotVersion:          result.SnapshotVersion,
			SnapshotDate:             result.SnapshotDate,
			SignalDate:               normalizeSourceTradeDate(result.SourceTradeDate),
			SourceTradeDate:          result.SourceTradeDate,
			RankingTime:              result.RankingTime.UTC().Format(time.RFC3339),
			HoldingsEffectiveTime:    result.HoldingsEffectiveTime.UTC().Format(time.RFC3339),
			NavAsOfTime:              result.NavAsOfTime.UTC().Format(time.RFC3339),
			UpdatedAt:                result.UpdatedAt.UTC().Format(time.RFC3339),
			LatestNav:                result.LatestNav,
			LatestPortfolioReturnPct: result.LatestPortfolioReturn,
			InceptionTradeDate:       summaryMetrics.InceptionTradeDate,
			InceptionDays:            summaryMetrics.InceptionDays,
			LatestDailyReturnPct:     summaryMetrics.LatestDailyReturnPct,
			CurrentMonthReturnPct:    summaryMetrics.CurrentMonthReturnPct,
			MaxDrawdownPct:           summaryMetrics.MaxDrawdownPct,
			VolatilityPct:            summaryMetrics.VolatilityPct,
			DailyWinRatePct:          summaryMetrics.DailyWinRatePct,
			CurrentConstituentCount:  result.CurrentConstituentCount,
			HasShortfall:             result.HasShortfall,
			WarningText:              result.WarningText,
			MethodNote:               "",
		},
		Series:          series,
		Constituents:    constituents,
		LatestRebalance: latestRebalance,
	}, nil
}

func (s *Service) GetRankingPortfolio(ctx context.Context) (*RankingPortfolioCollectionResponse, error) {
	definitions := defaultRankingPortfolioDefinitionRecords(time.Now().UTC())
	items := make([]RankingPortfolioResponse, 0, len(definitions))

	for _, definition := range definitions {
		var result RankingPortfolioResult
		err := s.repo.db.WithContext(ctx).
			Where("definition_id = ?", definition.ID).
			Order("snapshot_date DESC, id DESC").
			First(&result).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				item := buildEmptyRankingPortfolioResponse(definition)
				if err := s.applyCurrentRankingPortfolioSelection(ctx, definition, &item, time.Time{}); err != nil {
					return nil, err
				}
				items = append(items, item)
				continue
			}
			return nil, err
		}

		item, err := buildRankingPortfolioResponse(definition, result)
		if err != nil {
			return nil, err
		}
		if err := s.applyCurrentRankingPortfolioSelection(ctx, definition, item, result.RankingTime); err != nil {
			return nil, err
		}
		items = append(items, *item)
	}

	return &RankingPortfolioCollectionResponse{Items: items}, nil
}

// collectCurrentConstituentSymbols returns the de-duplicated set of current
// constituent symbols across all active definitions of the given market scope.
func (s *Service) collectCurrentConstituentSymbols(ctx context.Context, scope string) ([]RealtimeSymbol, error) {
	scope = strings.ToUpper(strings.TrimSpace(scope))
	definitions := defaultRankingPortfolioDefinitionRecords(time.Now().UTC())
	seen := map[string]struct{}{}
	out := make([]RealtimeSymbol, 0, 16)
	for _, definition := range definitions {
		if strings.ToUpper(strings.TrimSpace(definition.Exchange)) != scope {
			continue
		}
		constituents, _, _, err := s.buildCurrentRankingPortfolioSelection(ctx, definition)
		if err != nil {
			return nil, err
		}
		for _, c := range constituents {
			key := snapshotPriceHintKey(c.Code, c.Exchange)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, RealtimeSymbol{Code: c.Code, Exchange: c.Exchange})
		}
	}
	return out, nil
}

// persistRealtimeQuotes upserts the latest intraday quote for every symbol, and
// when fillOpen is true (09:25 call auction) also fills the simulated entry open
// price for the current trading day's batch. The entry trade date is the Beijing
// date of `at`.
func (s *Service) persistRealtimeQuotes(ctx context.Context, scope string, quotes []RealtimeQuote, fillOpen bool, at time.Time) error {
	if s.repo == nil || len(quotes) == 0 {
		return nil
	}
	entryTradeDate := at.In(beijingLocation()).Format("2006-01-02")
	for _, q := range quotes {
		if q.LastPrice <= 0 {
			continue
		}
		if err := s.repo.UpsertRankingPortfolioRealtimePrice(ctx, RankingPortfolioRealtimePrice{
			Code:      q.Code,
			Exchange:  q.Exchange,
			LastPrice: q.LastPrice,
			QuoteTime: at.UTC(),
		}); err != nil {
			return err
		}
		// On the 09:25 call auction, the open price is the simulated buy price.
		if fillOpen && q.IsOpen {
			if err := s.repo.FillRankingPortfolioEntryOpenPrice(ctx, scope, q.Code, q.Exchange, q.LastPrice, entryTradeDate); err != nil {
				return err
			}
		}
	}
	return nil
}

// OpenPriceBackfillResult summarises the outcome of BackfillMissingEntryOpenPrices.
type OpenPriceBackfillResult struct {
	// FilledCount is the number of market-price rows that had their open_price set.
	FilledCount int
	// StillPendingCount is the number of rows where the open-price resolver
	// returned 0 (e.g. data not yet available or stock was suspended).
	StillPendingCount int
	// SkippedBeforeCutover is the number of rows skipped because their
	// snapshot_date is before the cutover date (D0).
	SkippedBeforeCutover int
}

// BackfillMissingEntryOpenPrices scans market_price rows whose open_price is 0
// and snapshot_date >= cutoverDate, then fills them from the openPriceResolver.
// It is idempotent: rows that already have open_price > 0 are never touched.
// Returns a summary and any hard error (resolver returning 0 is not an error).
func (s *Service) BackfillMissingEntryOpenPrices(ctx context.Context, cutoverDate string) (OpenPriceBackfillResult, error) {
	cutoverDate = strings.TrimSpace(cutoverDate)
	var result OpenPriceBackfillResult

	if s.openPriceResolver == nil {
		return result, fmt.Errorf("open price resolver not configured")
	}

	rows, err := s.repo.ListMarketPricesMissingOpenByDateRange(ctx, cutoverDate)
	if err != nil {
		return result, fmt.Errorf("list missing open prices: %w", err)
	}
	if len(rows) == 0 {
		return result, nil
	}

	// Determine the T+1 entry trade date for each snapshot. The entry date is
	// the next trading day after the snapshot date. We approximate this as the
	// snapshot version itself when available, falling back to listing the
	// snapshot record to read HoldingsEffectiveTime or using snapshot_date+1.
	// In practice, snapshot_version == snapshot_date for this system.
	for _, row := range rows {
		if cutoverDate != "" && strings.TrimSpace(row.SnapshotDate) < cutoverDate {
			result.SkippedBeforeCutover++
			continue
		}
		// The entry (T+1) trade date for this snapshot is the next trading day.
		// We resolve it by looking up an existing snapshot with a later date or
		// by querying the repo for the next snapshot date after this one.
		entryTradeDate, err := s.resolveEntryTradeDateForSnapshot(ctx, row.DefinitionID, row.SnapshotDate)
		if err != nil || entryTradeDate == "" {
			// Cannot determine T+1 date yet (e.g. next snapshot hasn't arrived);
			// treat as still pending, not an error.
			result.StillPendingCount++
			continue
		}

		openPrice := s.openPriceResolver(ctx, row.Code, row.Exchange, entryTradeDate)
		if openPrice <= 0 {
			result.StillPendingCount++
			continue
		}

		if err := s.repo.SetRankingPortfolioMarketPriceOpen(ctx,
			row.DefinitionID, row.SnapshotVersion, row.Code, row.Exchange,
			openPrice, entryTradeDate); err != nil {
			return result, fmt.Errorf("set open price for %s(%s) on %s: %w",
				row.Code, row.Exchange, entryTradeDate, err)
		}
		result.FilledCount++
	}
	return result, nil
}

// resolveEntryTradeDateForSnapshot returns the T+1 trading date for a given
// snapshot date. It does so by finding the next snapshot in the DB that has a
// later snapshot_date (data-driven, same approach used by the daily pipeline).
//
// Fallback (D0 / latest snapshot): when no successor snapshot exists yet, the
// snapshot is the most recent one, so T+1 is today in Beijing time — provided
// today is strictly later than the snapshot date (i.e. the snapshot belongs to
// a previous trading day). This covers the cutover day and the HK portfolio
// case where only one snapshot has been persisted so far.
func (s *Service) resolveEntryTradeDateForSnapshot(ctx context.Context, definitionID, snapshotDate string) (string, error) {
	snapshotDate = strings.TrimSpace(snapshotDate)
	if snapshotDate == "" {
		return "", nil
	}
	var next RankingPortfolioSnapshot
	if err := s.repo.db.WithContext(ctx).
		Where("definition_id = ? AND snapshot_date > ?", definitionID, snapshotDate).
		Order("snapshot_date ASC, id ASC").
		First(&next).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// No successor snapshot yet — this is the latest batch.
			// If the snapshot was taken on a previous day, T+1 is today
			// (Beijing time). If the snapshot date equals today, we cannot
			// determine T+1 yet (it hasn't started).
			todayBJ := time.Now().In(beijingLocation()).Format("2006-01-02")
			if todayBJ > snapshotDate {
				return todayBJ, nil
			}
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(next.SnapshotDate), nil
}
