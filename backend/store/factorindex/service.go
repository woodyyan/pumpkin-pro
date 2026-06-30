package factorindex

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) EnsureInitialized(ctx context.Context) error {
	if s == nil || s.repo == nil {
		return fmt.Errorf("factor index service is nil")
	}
	return s.repo.EnsureDefaultDefinitions(ctx)
}

func (s *Service) SyncAll(ctx context.Context) error {
	if err := s.EnsureInitialized(ctx); err != nil {
		return err
	}
	if err := s.SyncRebalances(ctx); err != nil {
		return err
	}
	return s.SyncDaily(ctx)
}

func (s *Service) SyncRebalances(ctx context.Context) error {
	definitions, err := s.repo.ListActiveDefinitions(ctx, ExchangeAShare)
	if err != nil {
		return err
	}
	snapshotDates, err := s.repo.ListSnapshotDates(ctx)
	if err != nil {
		return err
	}
	previousDate := ""
	for _, snapshotDate := range snapshotDates {
		if !isFirstTradeDateOfMonth(previousDate, snapshotDate) {
			previousDate = snapshotDate
			continue
		}
		for _, definition := range definitions {
			exists, existsErr := s.repo.RebalanceExists(ctx, definition.ID, snapshotDate)
			if existsErr != nil {
				return existsErr
			}
			if exists {
				continue
			}
			if err := s.createRebalanceForDate(ctx, definition, snapshotDate); err != nil {
				return err
			}
		}
		previousDate = snapshotDate
	}
	return nil
}

func (s *Service) createRebalanceForDate(ctx context.Context, definition Definition, snapshotDate string) error {
	cfg := definitionByFactorKey(definition.FactorKey)
	if cfg == nil {
		return fmt.Errorf("unsupported factor key: %s", definition.FactorKey)
	}
	rows, err := s.repo.ListTopScores(ctx, snapshotDate, cfg.ScoreField, definition.TopN)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	rebalanceID := fmt.Sprintf("%s_%s", definition.ID, strings.ReplaceAll(snapshotDate, "-", ""))
	status := StatusCompleted
	warningText := ""
	if len(rows) == 0 {
		status = StatusPending
		warningText = "调仓日未获取到有效成分股"
	} else if len(rows) < definition.TopN {
		status = StatusPartial
		warningText = fmt.Sprintf("调仓日仅生成 %d/%d 只成分股", len(rows), definition.TopN)
	}
	rebalance := Rebalance{
		ID:               rebalanceID,
		IndexID:          definition.ID,
		SignalDate:       snapshotDate,
		SourceTradeDate:  snapshotDate,
		ConstituentCount: len(rows),
		Status:           status,
		WarningText:      warningText,
		ComputedAt:       now,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	constituents := make([]Constituent, 0, len(rows))
	for index, row := range rows {
		constituents = append(constituents, Constituent{
			RebalanceID:      rebalanceID,
			IndexID:          definition.ID,
			StockCode:        strings.TrimSpace(row.Code),
			StockName:        strings.TrimSpace(row.Name),
			Exchange:         strings.ToUpper(strings.TrimSpace(row.Exchange)),
			Rank:             index + 1,
			FactorScore:      row.Score,
			Weight:           definition.Weight,
			SignalClosePrice: row.ClosePrice,
			Industry:         strings.TrimSpace(row.Industry),
			CreatedAt:        now,
			UpdatedAt:        now,
		})
	}
	return s.repo.SaveRebalance(ctx, rebalance, constituents)
}

func (s *Service) SyncDaily(ctx context.Context) error {
	definitions, err := s.repo.ListActiveDefinitions(ctx, ExchangeAShare)
	if err != nil {
		return err
	}
	tradeDates, err := s.repo.ListTradeDates(ctx)
	if err != nil {
		return err
	}
	for _, tradeDate := range tradeDates {
		for _, definition := range definitions {
			if err := s.computeDailyForTradeDate(ctx, definition, tradeDate); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) computeDailyForTradeDate(ctx context.Context, definition Definition, tradeDate string) error {
	existing, err := s.repo.GetDailyByTradeDate(ctx, definition.ID, tradeDate)
	if err != nil {
		return err
	}
	if existing != nil {
		return nil
	}
	rebalance, err := s.repo.LatestRebalanceBeforeTradeDate(ctx, definition.ID, tradeDate)
	if err != nil {
		return err
	}
	if rebalance == nil {
		return nil
	}
	prevDaily, err := s.repo.LatestDailyBeforeTradeDate(ctx, definition.ID, tradeDate)
	if err != nil {
		return err
	}
	if strings.TrimSpace(rebalance.EffectiveStartDate) == "" {
		if err := s.repo.UpdateRebalanceActivation(ctx, rebalance.ID, tradeDate); err != nil {
			return err
		}
		if prevDaily != nil && strings.TrimSpace(prevDaily.TradeDate) != "" {
			if err := s.repo.ClosePreviousRebalances(ctx, definition.ID, rebalance.ID, prevDaily.TradeDate); err != nil {
				return err
			}
		}
		rebalance.EffectiveStartDate = tradeDate
	}
	constituents, err := s.repo.ListConstituentsByRebalance(ctx, rebalance.ID)
	if err != nil {
		return err
	}
	if len(constituents) == 0 {
		return nil
	}
	codes := make([]string, 0, len(constituents))
	for _, item := range constituents {
		codes = append(codes, item.StockCode)
	}
	priceWindows, err := s.repo.ListPriceWindows(ctx, codes, tradeDate)
	if err != nil {
		return err
	}
	validPriceCount := 0
	missingCount := 0
	returnSum := 0.0
	for _, item := range constituents {
		rows := priceWindows[item.StockCode]
		dailyReturn, valid := resolveConstituentReturn(tradeDate, rows)
		if valid {
			validPriceCount++
		} else {
			missingCount++
		}
		returnSum += item.Weight * dailyReturn
	}
	prevNAV := definition.BaseNAV
	if prevDaily != nil && prevDaily.NAV > 0 {
		prevNAV = prevDaily.NAV
	}
	nav := roundFloat(prevNAV*(1+returnSum), 6)
	totalReturn := 0.0
	if definition.BaseNAV > 0 {
		totalReturn = nav/definition.BaseNAV - 1
	}
	historyRows, err := s.repo.ListRecentDailyRows(ctx, definition.ID, tradeDate, 121)
	if err != nil {
		return err
	}
	series := append(append([]Daily{}, historyRows...), Daily{TradeDate: tradeDate, NAV: nav})
	weekly := returnForWindow(series, 5)
	monthly := returnForWindow(series, 20)
	threeMonth := returnForWindow(series, 60)
	halfYear := returnForWindow(series, 120)
	status := StatusCompleted
	warningText := ""
	if missingCount > 0 {
		status = StatusPartial
		warningText = fmt.Sprintf("%d/%d 只成分股缺少完整日线，按 0 收益处理", missingCount, len(constituents))
	}
	now := time.Now().UTC()
	row := Daily{
		IndexID:          definition.ID,
		TradeDate:        tradeDate,
		SourceTradeDate:  tradeDate,
		RebalanceID:      rebalance.ID,
		NAV:              nav,
		DailyReturn:      roundFloat(returnSum, 6),
		TotalReturn:      roundFloat(totalReturn, 6),
		WeeklyReturn:     weekly,
		MonthlyReturn:    monthly,
		ThreeMonthReturn: threeMonth,
		HalfYearReturn:   halfYear,
		ConstituentCount: len(constituents),
		ValidPriceCount:  validPriceCount,
		Status:           status,
		WarningText:      warningText,
		ComputedAt:       now,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	return s.repo.SaveDaily(ctx, row)
}

func (s *Service) GetOverview(ctx context.Context, exchange string) (*OverviewResponse, error) {
	if err := s.EnsureInitialized(ctx); err != nil {
		return nil, err
	}
	definitions, err := s.repo.ListActiveDefinitions(ctx, ExchangeAShare)
	if err != nil {
		return nil, err
	}
	items := make([]OverviewItem, 0, len(defaultDefinitions))
	latestSourceTradeDate := ""
	definitionMap := make(map[string]Definition, len(definitions))
	for _, definition := range definitions {
		definitionMap[definition.FactorKey] = definition
	}
	for _, cfg := range defaultDefinitions {
		definition, ok := definitionMap[cfg.FactorKey]
		if !ok {
			items = append(items, OverviewItem{IndexID: cfg.ID, FactorKey: cfg.FactorKey, Name: cfg.Name, Exchange: ExchangeAShare, Status: StatusPending})
			continue
		}
		item, buildErr := s.buildOverviewItem(ctx, definition)
		if buildErr != nil {
			return nil, buildErr
		}
		items = append(items, item)
		if strings.TrimSpace(item.SourceTradeDate) > latestSourceTradeDate {
			latestSourceTradeDate = strings.TrimSpace(item.SourceTradeDate)
		}
	}
	return &OverviewResponse{Exchange: strings.TrimSpace(exchange), SourceTradeDate: latestSourceTradeDate, Items: items}, nil
}

func (s *Service) buildOverviewItem(ctx context.Context, definition Definition) (OverviewItem, error) {
	latest, err := s.repo.LatestDaily(ctx, definition.ID)
	if err != nil {
		return OverviewItem{}, err
	}
	latestRebalance, err := s.repo.LatestRebalanceBeforeTradeDate(ctx, definition.ID, "9999-12-31")
	if err != nil {
		return OverviewItem{}, err
	}
	item := OverviewItem{
		IndexID:          definition.ID,
		FactorKey:        definition.FactorKey,
		Name:             definition.Name,
		Exchange:         definition.Exchange,
		ConstituentCount: 0,
		Status:           StatusPending,
	}
	if latestRebalance != nil {
		item.RebalanceDate = latestRebalance.SignalDate
		item.EffectiveStartDate = latestRebalance.EffectiveStartDate
		item.ConstituentCount = latestRebalance.ConstituentCount
		if latest == nil {
			item.Status = latestRebalance.Status
			item.WarningText = latestRebalance.WarningText
		}
	}
	if latest == nil {
		return item, nil
	}
	item.NAV = ptrFloat(latest.NAV)
	item.DailyReturn = ptrFloat(latest.DailyReturn)
	item.TotalReturn = ptrFloat(latest.TotalReturn)
	item.WeeklyReturn = latest.WeeklyReturn
	item.MonthlyReturn = latest.MonthlyReturn
	item.ThreeMonthReturn = latest.ThreeMonthReturn
	item.HalfYearReturn = latest.HalfYearReturn
	item.LatestTradeDate = latest.TradeDate
	item.SourceTradeDate = latest.SourceTradeDate
	item.Status = latest.Status
	item.WarningText = latest.WarningText
	item.ConstituentCount = latest.ConstituentCount
	recentRows, err := s.repo.ListRecentDailyRows(ctx, definition.ID, latest.TradeDate, 20)
	if err != nil {
		return OverviewItem{}, err
	}
	item.TrendPoints = make([]TrendPoint, 0, len(recentRows))
	for _, row := range recentRows {
		item.TrendPoints = append(item.TrendPoints, TrendPoint{Date: row.TradeDate, Count: row.NAV})
	}
	return item, nil
}

func resolveConstituentReturn(tradeDate string, rows []priceWindowRow) (float64, bool) {
	if len(rows) == 0 {
		return 0, false
	}
	var current *priceWindowRow
	var previous *priceWindowRow
	for index := range rows {
		row := rows[index]
		if row.TradeDate == tradeDate {
			current = &row
			if index+1 < len(rows) {
				previous = &rows[index+1]
			}
			break
		}
	}
	if current == nil {
		return 0, false
	}
	if previous == nil || previous.Close <= 0 {
		return 0, false
	}
	return current.Close/previous.Close - 1, true
}

func returnForWindow(series []Daily, window int) *float64 {
	if window <= 0 || len(series) <= window {
		return nil
	}
	start := series[len(series)-1-window].NAV
	end := series[len(series)-1].NAV
	if start <= 0 {
		return nil
	}
	result := roundFloat(end/start-1, 6)
	return &result
}

func isFirstTradeDateOfMonth(previousDate, currentDate string) bool {
	currentDate = strings.TrimSpace(currentDate)
	if len(currentDate) < 7 {
		return false
	}
	previousDate = strings.TrimSpace(previousDate)
	if previousDate == "" || len(previousDate) < 7 {
		return true
	}
	return previousDate[:7] != currentDate[:7]
}

func roundFloat(value float64, digits int) float64 {
	factor := math.Pow10(digits)
	return math.Round(value*factor) / factor
}

func ptrFloat(value float64) *float64 {
	v := value
	return &v
}
