package quadrant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const rankingPortfolioTradeCostDisplayDigits = 6

const (
	rankingPortfolioEffectiveHour   = 9
	rankingPortfolioEffectiveMinute = 30
)

type rankingPortfolioDefinitionSpec struct {
	ID              string
	Code            string
	Name            string
	Exchange        string
	PortfolioVariant string
	BenchmarkCode   string
	BenchmarkName   string
	SelectionRule   string
	SelectionWindow int
	ExcludedBoards  []string
	MethodNote      string
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
		RebalanceRule:    "t_close_generate_t1_open_rebalance",
		TradeCostRate:    defaultRankingPortfolioTradeCostRate,
		MethodNote:       spec.MethodNote,
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

func (s *Service) saveRankingPortfolioBestEffort(ctx context.Context, records []QuadrantScoreRecord, computedAt time.Time, priceHints map[string]snapshotPriceHint) {
	if err := s.saveRankingPortfolio(ctx, records, computedAt, priceHints); err != nil {
		log.Printf("[quadrant] ranking portfolio save skipped: %v", err)
	}
}

func (s *Service) saveRankingPortfolio(ctx context.Context, records []QuadrantScoreRecord, computedAt time.Time, priceHints map[string]snapshotPriceHint) error {
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

	return s.repo.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		for _, definition := range definitions {
			definition.UpdatedAt = now
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "id"}},
				DoUpdates: clause.AssignmentColumns([]string{
					"code", "name", "exchange", "portfolio_variant", "benchmark_code", "benchmark_name",
					"max_holdings", "selection_rule", "selection_window", "excluded_boards", "weighting_method",
					"rebalance_rule", "trade_cost_rate", "method_note", "is_active", "updated_at",
				}),
			}).Create(&definition).Error; err != nil {
				return fmt.Errorf("upsert ranking portfolio definition: %w", err)
			}

			snapshotVersion := buildRankingPortfolioSnapshotVersion(snapshotDate)
			if err := deleteRankingPortfolioSnapshotVersion(tx, definition.ID, snapshotVersion); err != nil {
				return err
			}

			currentConstituents, err := s.selectRankingPortfolioConstituents(ctx, definition, records)
			if err != nil {
				return err
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
					return fmt.Errorf("load previous ranking portfolio snapshot: %w", err)
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
					return fmt.Errorf("load previous ranking portfolio constituents: %w", err)
				}
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
				return fmt.Errorf("insert ranking portfolio snapshot: %w", err)
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
					return fmt.Errorf("insert ranking portfolio constituents: %w", err)
				}
			}

			marketPrices := s.buildRankingPortfolioMarketPrices(ctx, definition, snapshotVersion, snapshotDate, currentConstituents, previousConstituents, now, priceHints)
			if len(marketPrices) > 0 {
				if err := tx.Create(&marketPrices).Error; err != nil {
					return fmt.Errorf("insert ranking portfolio market prices: %w", err)
				}
			}

			benchmarkClose, benchmarkTradeDate := s.resolveRankingPortfolioBenchmarkClose(ctx, definition.BenchmarkCode, snapshotDate)
			if benchmarkClose <= 0 {
				return fmt.Errorf("missing benchmark close for %s on %s", definition.BenchmarkCode, snapshotDate)
			}
			benchmarkRow := RankingPortfolioBenchmarkPrice{
				DefinitionID:    definition.ID,
				SnapshotVersion: snapshotVersion,
				SnapshotDate:    snapshotDate,
				BenchmarkCode:   definition.BenchmarkCode,
				BenchmarkName:   definition.BenchmarkName,
				ClosePrice:      benchmarkClose,
				PriceTradeDate:  benchmarkTradeDate,
				CreatedAt:       now,
				UpdatedAt:       now,
			}
			if err := tx.Create(&benchmarkRow).Error; err != nil {
				return fmt.Errorf("insert ranking portfolio benchmark price: %w", err)
			}

			result, err := buildRankingPortfolioResult(tx, definition, snapshotVersion, now)
			if err != nil {
				return err
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "definition_id"}, {Name: "snapshot_version"}},
				DoUpdates: clause.AssignmentColumns([]string{
					"batch_id", "snapshot_date", "ranking_time", "holdings_effective_time", "nav_as_of_time",
					"benchmark_code", "benchmark_name", "latest_nav", "latest_benchmark_nav",
					"latest_portfolio_return", "latest_benchmark_return", "latest_excess_return_pct",
					"current_constituent_count", "has_shortfall", "warning_text", "method_note",
					"series_json", "constituents_json", "latest_rebalance_json", "updated_at",
				}),
			}).Create(result).Error; err != nil {
				return fmt.Errorf("upsert ranking portfolio result: %w", err)
			}
		}

		return nil
	})
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
		Delete(&RankingPortfolioBenchmarkPrice{}).Error; err != nil {
		return fmt.Errorf("delete ranking portfolio benchmark price: %w", err)
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

func (s *Service) buildRankingPortfolioMarketPrices(ctx context.Context, definition RankingPortfolioDefinition, snapshotVersion string, snapshotDate string, current []RankingPortfolioConstituentItem, previous []RankingPortfolioConstituentItem, now time.Time, priceHints map[string]snapshotPriceHint) []RankingPortfolioMarketPrice {
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
		if hint, ok := priceHints[key]; ok && hint.ClosePrice > 0 {
			closePrice = hint.ClosePrice
			priceTradeDate = validPriceTradeDate(hint.TradeDate)
		}
		if closePrice <= 0 && s.priceResolver != nil {
			closePrice = s.priceResolver(ctx, item.Code, item.Exchange, snapshotDate)
			if closePrice > 0 {
				priceTradeDate = snapshotDate
			}
		}
		prices = append(prices, RankingPortfolioMarketPrice{
			DefinitionID:    definition.ID,
			SnapshotVersion: snapshotVersion,
			SnapshotDate:    snapshotDate,
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
	return prices
}

func (s *Service) resolveRankingPortfolioBenchmarkClose(ctx context.Context, benchmarkCode string, snapshotDate string) (float64, string) {
	if s.benchmarkPriceResolver == nil {
		return 0, ""
	}
	closePrice, tradeDate := s.benchmarkPriceResolver(ctx, benchmarkCode, snapshotDate)
	if closePrice <= 0 {
		return 0, ""
	}
	return closePrice, validPriceTradeDate(tradeDate)
}

func buildRankingPortfolioLatestRebalance(
	definition RankingPortfolioDefinition,
	currentSnapshot RankingPortfolioSnapshot,
	current []RankingPortfolioConstituentItem,
	previous []RankingPortfolioConstituentItem,
	priceByKey map[string]RankingPortfolioMarketPrice,
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
		referencePrice := roundRankingPortfolioFloat(priceRow.ClosePrice)
		referenceCostPrice := 0.0
		if referencePrice > 0 {
			referenceCostPrice = roundRankingPortfolioFloat(referencePrice * costMultiplier)
		}

		items = append(items, RankingPortfolioRebalanceItem{
			Action:             action,
			Code:               baseItem.Code,
			Name:               baseItem.Name,
			Exchange:           baseItem.Exchange,
			Board:              baseItem.Board,
			FromWeight:         roundRankingPortfolioFloat(fromWeight),
			ToWeight:           roundRankingPortfolioFloat(toWeight),
			ReferencePrice:     referencePrice,
			ReferenceCostPrice: referenceCostPrice,
			PriceTradeDate:     priceRow.PriceTradeDate,
		})
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
		SnapshotDate:  currentSnapshot.SnapshotDate,
		RankingTime:   currentSnapshot.RankingTime.UTC().Format(time.RFC3339),
		EffectiveTime: currentSnapshot.HoldingsEffectiveTime.UTC().Format(time.RFC3339),
		TradeCostRate: roundRankingPortfolioCostRate(definition.TradeCostRate),
		ChangeCount:   len(items),
		Items:         items,
	}
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

	priceByDate := map[string]map[string]float64{}
	var priceRows []RankingPortfolioMarketPrice
	if err := tx.Where("definition_id = ?", definition.ID).
		Order("snapshot_date ASC, exchange ASC, code ASC, id ASC").
		Find(&priceRows).Error; err != nil {
		return nil, fmt.Errorf("list ranking portfolio market prices: %w", err)
	}
	for _, row := range priceRows {
		if _, ok := priceByDate[row.SnapshotDate]; !ok {
			priceByDate[row.SnapshotDate] = map[string]float64{}
		}
		priceByDate[row.SnapshotDate][snapshotPriceHintKey(row.Code, row.Exchange)] = row.ClosePrice
	}

	benchmarkByDate := map[string]float64{}
	var benchmarkRows []RankingPortfolioBenchmarkPrice
	if err := tx.Where("definition_id = ?", definition.ID).
		Order("snapshot_date ASC, id ASC").
		Find(&benchmarkRows).Error; err != nil {
		return nil, fmt.Errorf("list ranking portfolio benchmark prices: %w", err)
	}
	for _, row := range benchmarkRows {
		benchmarkByDate[row.SnapshotDate] = row.ClosePrice
	}

	series := make([]RankingPortfolioSeriesPoint, 0, len(snapshots))
	firstSnapshot := snapshots[0]
	series = append(series, RankingPortfolioSeriesPoint{
		Date:                    firstSnapshot.SnapshotDate,
		Nav:                     1,
		BenchmarkNav:            1,
		PortfolioReturnPct:      0,
		BenchmarkReturnPct:      0,
		ExcessReturnPct:         0,
		DailyPortfolioReturnPct: 0,
		DailyBenchmarkReturnPct: 0,
		HoldingCount:            0,
	})

	activeHoldings := []RankingPortfolioConstituentItem{}
	for i := 1; i < len(snapshots); i++ {
		prevSnapshot := snapshots[i-1]
		currentSnapshot := snapshots[i]
		nextHoldings := constituentsByVersion[prevSnapshot.SnapshotVersion]
		portfolioReturn := calculateRankingPortfolioPeriodReturn(nextHoldings, priceByDate[prevSnapshot.SnapshotDate], priceByDate[currentSnapshot.SnapshotDate])
		tradeRatio := calculateRankingPortfolioTradeRatio(activeHoldings, nextHoldings)
		costRatio := definition.TradeCostRate * tradeRatio

		prevPoint := series[len(series)-1]
		nav := prevPoint.Nav * (1 - costRatio) * (1 + portfolioReturn)
		benchmarkReturn := calculateRankingPortfolioBenchmarkReturn(benchmarkByDate[prevSnapshot.SnapshotDate], benchmarkByDate[currentSnapshot.SnapshotDate])
		benchmarkNav := prevPoint.BenchmarkNav * (1 + benchmarkReturn)

		series = append(series, RankingPortfolioSeriesPoint{
			Date:                    currentSnapshot.SnapshotDate,
			Nav:                     roundRankingPortfolioFloat(nav),
			BenchmarkNav:            roundRankingPortfolioFloat(benchmarkNav),
			PortfolioReturnPct:      roundRankingPortfolioPct((nav - 1) * 100),
			BenchmarkReturnPct:      roundRankingPortfolioPct((benchmarkNav - 1) * 100),
			ExcessReturnPct:         roundRankingPortfolioPct((nav - benchmarkNav) * 100),
			DailyPortfolioReturnPct: roundRankingPortfolioPct(portfolioReturn * 100),
			DailyBenchmarkReturnPct: roundRankingPortfolioPct(benchmarkReturn * 100),
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
	latestRebalanceJSON := mustMarshal(buildRankingPortfolioLatestRebalance(definition, latestSnapshot, latestConstituents, previousConstituents, latestPriceByKey))

	return &RankingPortfolioResult{
		DefinitionID:            definition.ID,
		SnapshotVersion:         latestSnapshotVersion,
		BatchID:                 buildRankingPortfolioBatchID(definition.ID, latestSnapshotVersion),
		SnapshotDate:            latestSnapshot.SnapshotDate,
		RankingTime:             latestSnapshot.RankingTime,
		HoldingsEffectiveTime:   latestSnapshot.HoldingsEffectiveTime,
		NavAsOfTime:             latestSnapshot.NavAsOfTime,
		BenchmarkCode:           latestSnapshot.BenchmarkCode,
		BenchmarkName:           latestSnapshot.BenchmarkName,
		LatestNav:               latestPoint.Nav,
		LatestBenchmarkNav:      latestPoint.BenchmarkNav,
		LatestPortfolioReturn:   latestPoint.PortfolioReturnPct,
		LatestBenchmarkReturn:   latestPoint.BenchmarkReturnPct,
		LatestExcessReturnPct:   latestPoint.ExcessReturnPct,
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

func calculateRankingPortfolioPeriodReturn(holdings []RankingPortfolioConstituentItem, prevPrices map[string]float64, currentPrices map[string]float64) float64 {
	if len(holdings) == 0 || len(prevPrices) == 0 || len(currentPrices) == 0 {
		return 0
	}
	weightedSum := 0.0
	weightSum := 0.0
	for _, holding := range holdings {
		key := snapshotPriceHintKey(holding.Code, holding.Exchange)
		prevClose := prevPrices[key]
		currentClose := currentPrices[key]
		if prevClose <= 0 || currentClose <= 0 {
			continue
		}
		weightedSum += holding.Weight * (currentClose/prevClose - 1)
		weightSum += holding.Weight
	}
	if weightSum <= 0 {
		return 0
	}
	return weightedSum / weightSum
}

func calculateRankingPortfolioBenchmarkReturn(prevClose, currentClose float64) float64 {
	if prevClose <= 0 || currentClose <= 0 {
		return 0
	}
	return currentClose/prevClose - 1
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
			DefinitionID:       definition.ID,
			DefinitionCode:     definition.Code,
			Name:               definition.Name,
			Exchange:           definition.Exchange,
			PortfolioVariant:   definition.PortfolioVariant,
			SelectionRule:      definition.SelectionRule,
			SelectionWindow:    definition.SelectionWindow,
			BenchmarkCode:      definition.BenchmarkCode,
			BenchmarkName:      definition.BenchmarkName,
			LatestNav:          1,
			LatestBenchmarkNav: 1,
			MethodNote:         definition.MethodNote,
		},
		Series:          []RankingPortfolioSeriesPoint{},
		Constituents:    []RankingPortfolioConstituentItem{},
		LatestRebalance: nil,
	}
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

	return &RankingPortfolioResponse{
		Meta: RankingPortfolioMeta{
			DefinitionID:             definition.ID,
			DefinitionCode:           definition.Code,
			Name:                     definition.Name,
			Exchange:                 definition.Exchange,
			PortfolioVariant:         definition.PortfolioVariant,
			SelectionRule:            definition.SelectionRule,
			SelectionWindow:          definition.SelectionWindow,
			BatchID:                  result.BatchID,
			SnapshotVersion:          result.SnapshotVersion,
			SnapshotDate:             result.SnapshotDate,
			BenchmarkCode:            result.BenchmarkCode,
			BenchmarkName:            result.BenchmarkName,
			RankingTime:              result.RankingTime.UTC().Format(time.RFC3339),
			HoldingsEffectiveTime:    result.HoldingsEffectiveTime.UTC().Format(time.RFC3339),
			NavAsOfTime:              result.NavAsOfTime.UTC().Format(time.RFC3339),
			UpdatedAt:                result.UpdatedAt.UTC().Format(time.RFC3339),
			LatestNav:                result.LatestNav,
			LatestBenchmarkNav:       result.LatestBenchmarkNav,
			LatestPortfolioReturnPct: result.LatestPortfolioReturn,
			LatestBenchmarkReturnPct: result.LatestBenchmarkReturn,
			LatestExcessReturnPct:    result.LatestExcessReturnPct,
			CurrentConstituentCount:  result.CurrentConstituentCount,
			HasShortfall:             result.HasShortfall,
			WarningText:              result.WarningText,
			MethodNote:               result.MethodNote,
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
				items = append(items, buildEmptyRankingPortfolioResponse(definition))
				continue
			}
			return nil, err
		}

		item, err := buildRankingPortfolioResponse(definition, result)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}

	return &RankingPortfolioCollectionResponse{Items: items}, nil
}
