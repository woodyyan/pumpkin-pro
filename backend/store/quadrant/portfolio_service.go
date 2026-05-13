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

const defaultRankingPortfolioMethodNote = "模拟组合规则：每日收盘后取去除科创板后的卧龙AI精选 TOP4，下一交易日生效，收益按相邻有效交易日收盘价近似计算并扣除 0.02% 交易成本，不代表实际投资建议。"

func defaultRankingPortfolioDefinitionRecord(now time.Time) RankingPortfolioDefinition {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return RankingPortfolioDefinition{
		ID:              defaultRankingPortfolioDefinitionID,
		Code:            defaultRankingPortfolioDefinitionCode,
		Name:            defaultRankingPortfolioName,
		Exchange:        defaultRankingPortfolioExchange,
		BenchmarkCode:   defaultRankingPortfolioBenchmarkCode,
		BenchmarkName:   defaultRankingPortfolioBenchmarkName,
		MaxHoldings:     defaultRankingPortfolioMaxHoldings,
		ExcludedBoards:  mustMarshal([]string{aShareBoardStar}),
		WeightingMethod: "equal",
		RebalanceRule:   "t_close_generate_t1_open_rebalance",
		TradeCostRate:   defaultRankingPortfolioTradeCostRate,
		MethodNote:      defaultRankingPortfolioMethodNote,
		IsActive:        true,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

func buildRankingPortfolioSnapshotVersion(snapshotDate string) string {
	return strings.TrimSpace(snapshotDate)
}

func buildRankingPortfolioBatchID(definitionID, snapshotVersion string) string {
	return strings.TrimSpace(definitionID) + ":" + strings.TrimSpace(snapshotVersion)
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

	return s.repo.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		definition := defaultRankingPortfolioDefinitionRecord(now)
		definition.UpdatedAt = now
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"code", "name", "exchange", "benchmark_code", "benchmark_name",
				"max_holdings", "excluded_boards", "weighting_method", "rebalance_rule",
				"trade_cost_rate", "method_note", "is_active", "updated_at",
			}),
		}).Create(&definition).Error; err != nil {
			return fmt.Errorf("upsert ranking portfolio definition: %w", err)
		}

		snapshotVersion := buildRankingPortfolioSnapshotVersion(snapshotDate)
		if err := deleteRankingPortfolioSnapshotVersion(tx, definition.ID, snapshotVersion); err != nil {
			return err
		}

		currentConstituents := selectRankingPortfolioConstituents(records, definition.MaxHoldings)
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
		snapshot := RankingPortfolioSnapshot{
			DefinitionID:          definition.ID,
			SnapshotVersion:       snapshotVersion,
			BatchID:               batchID,
			SnapshotDate:          snapshotDate,
			RankingTime:           computedAt,
			HoldingsEffectiveTime: computedAt,
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
				"series_json", "constituents_json", "updated_at",
			}),
		}).Create(result).Error; err != nil {
			return fmt.Errorf("upsert ranking portfolio result: %w", err)
		}

		return nil
	})
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
			Rank:         row.Rank,
			Code:         row.Code,
			Name:         row.Name,
			Exchange:     row.Exchange,
			Board:        row.Board,
			Weight:       row.Weight,
			RankingScore: row.RankingScore,
			Opportunity:  row.Opportunity,
			Risk:         row.Risk,
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

func (s *Service) GetRankingPortfolio(ctx context.Context) (*RankingPortfolioResponse, error) {
	definition := defaultRankingPortfolioDefinitionRecord(time.Now().UTC())
	var result RankingPortfolioResult
	err := s.repo.db.WithContext(ctx).
		Where("definition_id = ?", definition.ID).
		Order("snapshot_date DESC, id DESC").
		First(&result).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &RankingPortfolioResponse{
				Meta: RankingPortfolioMeta{
					DefinitionID:       definition.ID,
					DefinitionCode:     definition.Code,
					Name:               definition.Name,
					BenchmarkCode:      definition.BenchmarkCode,
					BenchmarkName:      definition.BenchmarkName,
					LatestNav:          1,
					LatestBenchmarkNav: 1,
					MethodNote:         definition.MethodNote,
				},
				Series:       []RankingPortfolioSeriesPoint{},
				Constituents: []RankingPortfolioConstituentItem{},
			}, nil
		}
		return nil, err
	}

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

	return &RankingPortfolioResponse{
		Meta: RankingPortfolioMeta{
			DefinitionID:             definition.ID,
			DefinitionCode:           definition.Code,
			Name:                     definition.Name,
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
		Series:       series,
		Constituents: constituents,
	}, nil
}
