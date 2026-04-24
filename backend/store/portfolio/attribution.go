package portfolio

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/woodyyan/pumpkin-pro/backend/store/live"
)

type attributionHistoryReader interface {
	GetDailyBars(ctx context.Context, symbols []string, startDate, endDate string) (map[string][]DailyBarRecord, error)
}

type attributionBenchmarkReader interface {
	FetchBenchmarkDailyBars(ctx context.Context, benchmark string, lookbackDays int) ([]live.DailyBar, error)
}

type defaultAttributionBenchmarkReader struct {
	client *live.MarketClient
}

func (r *defaultAttributionBenchmarkReader) FetchBenchmarkDailyBars(ctx context.Context, benchmark string, lookbackDays int) ([]live.DailyBar, error) {
	return r.client.FetchBenchmarkDailyBars(ctx, benchmark, lookbackDays)
}

type attributionPositionState struct {
	OriginalSymbol   string
	NormalizedSymbol string
	Name             string
	Exchange         string
	CurrencyCode     string
	CurrencySymbol   string
	Shares           float64
	AvgCostPrice     float64
	TotalCostAmount  float64
	BuyDate          string
	CostSource       string
	SectorCode       string
	SectorName       string
	BenchmarkCode    string
}

type attributionSnapshotLite struct {
	Date                string
	ClosePrice          float64
	PrevClosePrice      float64
	MarketValueAmount   float64
	UnrealizedPnlAmount float64
	PositionWeightRatio float64
	TotalCostAmount     float64
	Shares              float64
}

type attributionTradeStats struct {
	RealizedPnlAmount      float64
	FeeAmount              float64
	TradeCount             int
	BuyCount               int
	SellCount              int
	SellAmount             float64
	WinningSellCount       int
	HoldingDaysBeforeSell  int
	HoldingDaysSampleCount int
	Timeline               []PortfolioAttributionTradingTimelineItem
}

type attributionSymbolAggregate struct {
	Symbol                    string
	Name                      string
	Exchange                  string
	CurrencyCode              string
	CurrencySymbol            string
	SectorCode                string
	SectorName                string
	BenchmarkCode             string
	StartWeightRatio          float64
	EndWeightRatio            float64
	AvgWeightRatio            float64
	RealizedPnlAmount         float64
	UnrealizedPnlChangeAmount float64
	TotalPnlAmount            float64
	HoldingReturnPct          float64
	BuyCount                  int
	SellCount                 int
	HoldingDays               int
	Snapshots                 []attributionSnapshotLite
	DetailURL                 string
}

type attributionScopeDailyTotal struct {
	Date              string
	Scope             string
	CurrencyCode      string
	CurrencySymbol    string
	MarketValueAmount float64
	UnrealizedPnl     float64
	RangeRealizedPnl  float64
	MaxWeightRatio    float64
}

type attributionScopeAggregate struct {
	Scope                       string
	ScopeLabel                  string
	CurrencyCode                string
	CurrencySymbol              string
	StartMarketValueAmount      float64
	EndMarketValueAmount        float64
	StartUnrealizedPnlAmount    float64
	EndUnrealizedPnlAmount      float64
	RealizedPnlAmount           float64
	UnrealizedPnlChangeAmount   float64
	FeeAmount                   float64
	TradingAlphaAmount          float64
	MarketContributionAmount    float64
	ExcessContributionAmount    float64
	TotalPnlAmount              float64
	TotalReturnPct              float64
	ShadowHoldPnlAmount         float64
	BenchmarkCode               string
	BenchmarkName               string
	BenchmarkReturnPct          float64
	SelectionContributionAmount float64
	DailyTotals                 []attributionScopeDailyTotal
	Series                      []PortfolioAttributionMarketSeriesPoint
	TradeStats                  attributionTradeStats
}

type attributionDataset struct {
	Meta            PortfolioAttributionMeta
	ComputedAt      time.Time
	Scopes          []string
	ScopeAggregates map[string]*attributionScopeAggregate
	SymbolsByScope  map[string][]attributionSymbolAggregate
}

func (s *Service) GetAttributionSummary(ctx context.Context, userID string, query PortfolioAttributionQuery) (*PortfolioAttributionSummaryPayload, error) {
	dataset, err := s.loadAttributionDataset(ctx, userID, query, nil, nil)
	if err != nil {
		return nil, err
	}
	payload := &PortfolioAttributionSummaryPayload{PortfolioAttributionMeta: dataset.Meta}
	if !dataset.Meta.HasData {
		return payload, nil
	}

	payload.MarketBlocks = make([]PortfolioAttributionMarketBlock, 0, len(dataset.Scopes))
	payload.WaterfallGroups = make([]PortfolioAttributionWaterfallGroup, 0, len(dataset.Scopes))
	payload.SummaryCards = buildAttributionSummaryCards(dataset)
	payload.Insights = buildAttributionSummaryInsights(dataset)
	payload.Headline = buildAttributionHeadline(dataset)

	for _, scope := range dataset.Scopes {
		agg := dataset.ScopeAggregates[scope]
		if agg == nil {
			continue
		}
		payload.MarketBlocks = append(payload.MarketBlocks, PortfolioAttributionMarketBlock{
			Scope:                     agg.Scope,
			ScopeLabel:                agg.ScopeLabel,
			CurrencyCode:              agg.CurrencyCode,
			CurrencySymbol:            agg.CurrencySymbol,
			StartMarketValueAmount:    round2(agg.StartMarketValueAmount),
			EndMarketValueAmount:      round2(agg.EndMarketValueAmount),
			RealizedPnlAmount:         round2(agg.RealizedPnlAmount),
			UnrealizedPnlChangeAmount: round2(agg.UnrealizedPnlChangeAmount),
			FeeAmount:                 round2(agg.FeeAmount),
			TradingAlphaAmount:        round2(agg.TradingAlphaAmount),
			MarketContributionAmount:  round2(agg.MarketContributionAmount),
			ExcessContributionAmount:  round2(agg.ExcessContributionAmount),
			TotalPnlAmount:            round2(agg.TotalPnlAmount),
			TotalReturnPct:            round4(agg.TotalReturnPct),
		})
		payload.WaterfallGroups = append(payload.WaterfallGroups, buildWaterfallGroup(agg))
	}
	return payload, nil
}

func (s *Service) GetAttributionStocks(ctx context.Context, userID string, query PortfolioAttributionQuery) (*PortfolioAttributionStocksPayload, error) {
	dataset, err := s.loadAttributionDataset(ctx, userID, query, nil, nil)
	if err != nil {
		return nil, err
	}
	payload := &PortfolioAttributionStocksPayload{PortfolioAttributionMeta: dataset.Meta}
	if !dataset.Meta.HasData {
		return payload, nil
	}
	payload.PositiveGroups = make([]PortfolioAttributionStockGroup, 0, len(dataset.Scopes))
	payload.NegativeGroups = make([]PortfolioAttributionStockGroup, 0, len(dataset.Scopes))
	payload.NetGroups = make([]PortfolioAttributionStockGroup, 0, len(dataset.Scopes))
	limit := query.Limit
	if limit <= 0 {
		limit = 5
	}
	for _, scope := range dataset.Scopes {
		items := append([]attributionSymbolAggregate(nil), dataset.SymbolsByScope[scope]...)
		agg := dataset.ScopeAggregates[scope]
		if agg == nil || len(items) == 0 {
			continue
		}
		sort.SliceStable(items, func(i, j int) bool { return items[i].TotalPnlAmount > items[j].TotalPnlAmount })
		allItems := toAttributionStockItems(items, agg.TotalPnlAmount)
		positive := make([]PortfolioAttributionStockItem, 0)
		negative := make([]PortfolioAttributionStockItem, 0)
		for _, item := range allItems {
			if item.TotalPnlAmount > 0 && len(positive) < limit {
				positive = append(positive, item)
			}
			if item.TotalPnlAmount < 0 && len(negative) < limit {
				negative = append(negative, item)
			}
		}
		payload.PositiveGroups = append(payload.PositiveGroups, PortfolioAttributionStockGroup{
			Scope:          scope,
			ScopeLabel:     agg.ScopeLabel,
			CurrencyCode:   agg.CurrencyCode,
			CurrencySymbol: agg.CurrencySymbol,
			Items:          positive,
		})
		payload.NegativeGroups = append(payload.NegativeGroups, PortfolioAttributionStockGroup{
			Scope:          scope,
			ScopeLabel:     agg.ScopeLabel,
			CurrencyCode:   agg.CurrencyCode,
			CurrencySymbol: agg.CurrencySymbol,
			Items:          negative,
		})
		if len(allItems) > limit {
			allItems = allItems[:limit]
		}
		payload.NetGroups = append(payload.NetGroups, PortfolioAttributionStockGroup{
			Scope:          scope,
			ScopeLabel:     agg.ScopeLabel,
			CurrencyCode:   agg.CurrencyCode,
			CurrencySymbol: agg.CurrencySymbol,
			Items:          allItems,
		})
	}
	return payload, nil
}

func (s *Service) GetAttributionSectors(ctx context.Context, userID string, query PortfolioAttributionQuery) (*PortfolioAttributionSectorsPayload, error) {
	dataset, err := s.loadAttributionDataset(ctx, userID, query, nil, nil)
	if err != nil {
		return nil, err
	}
	payload := &PortfolioAttributionSectorsPayload{PortfolioAttributionMeta: dataset.Meta, ClassificationSource: "security_profiles"}
	if !dataset.Meta.HasData {
		return payload, nil
	}
	payload.Groups = make([]PortfolioAttributionSectorGroup, 0, len(dataset.Scopes))

	classifiedCount := 0
	totalCount := 0
	unclassifiedCount := 0
	for _, items := range dataset.SymbolsByScope {
		for _, item := range items {
			totalCount++
			if isUnclassifiedSector(item.SectorName) {
				unclassifiedCount++
			} else {
				classifiedCount++
			}
		}
	}
	if totalCount > 0 {
		payload.CoverageRatio = round4(float64(classifiedCount) / float64(totalCount))
	}
	payload.UnclassifiedStockCount = unclassifiedCount

	for _, scope := range dataset.Scopes {
		agg := dataset.ScopeAggregates[scope]
		if agg == nil {
			continue
		}
		sectorItems := buildSectorItems(dataset.SymbolsByScope[scope], agg.TotalPnlAmount, query.IncludeUnclassified, query.Limit)
		payload.Groups = append(payload.Groups, PortfolioAttributionSectorGroup{
			Scope:          scope,
			ScopeLabel:     agg.ScopeLabel,
			CurrencyCode:   agg.CurrencyCode,
			CurrencySymbol: agg.CurrencySymbol,
			Items:          sectorItems,
		})
	}
	return payload, nil
}

func (s *Service) GetAttributionTrading(ctx context.Context, userID string, query PortfolioAttributionQuery) (*PortfolioAttributionTradingPayload, error) {
	dataset, err := s.loadAttributionDataset(ctx, userID, query, nil, nil)
	if err != nil {
		return nil, err
	}
	payload := &PortfolioAttributionTradingPayload{PortfolioAttributionMeta: dataset.Meta}
	if !dataset.Meta.HasData {
		return payload, nil
	}
	payload.Groups = make([]PortfolioAttributionTradingGroup, 0, len(dataset.Scopes))
	limit := query.TimelineLimit
	if limit <= 0 {
		limit = 20
	}
	for _, scope := range dataset.Scopes {
		agg := dataset.ScopeAggregates[scope]
		if agg == nil {
			continue
		}
		timeline := append([]PortfolioAttributionTradingTimelineItem(nil), agg.TradeStats.Timeline...)
		sort.SliceStable(timeline, func(i, j int) bool {
			return math.Abs(timeline[i].TimingEffectAmount) > math.Abs(timeline[j].TimingEffectAmount)
		})
		if len(timeline) > limit {
			timeline = timeline[:limit]
		}
		avgHolding := 0.0
		if agg.TradeStats.HoldingDaysSampleCount > 0 {
			avgHolding = float64(agg.TradeStats.HoldingDaysBeforeSell) / float64(agg.TradeStats.HoldingDaysSampleCount)
		}
		winRatio := 0.0
		if agg.TradeStats.SellCount > 0 {
			winRatio = float64(agg.TradeStats.WinningSellCount) / float64(agg.TradeStats.SellCount)
		}
		payload.Groups = append(payload.Groups, PortfolioAttributionTradingGroup{
			Scope:                    agg.Scope,
			ScopeLabel:               agg.ScopeLabel,
			CurrencyCode:             agg.CurrencyCode,
			CurrencySymbol:           agg.CurrencySymbol,
			ActualTotalPnlAmount:     round2(agg.TotalPnlAmount),
			ShadowHoldPnlAmount:      round2(agg.ShadowHoldPnlAmount),
			TradingAlphaAmount:       round2(agg.TradingAlphaAmount),
			FeeAmount:                round2(agg.FeeAmount),
			TurnoverRatio:            round4(calcTurnoverRatio(agg)),
			TradeCount:               agg.TradeStats.TradeCount,
			BuyCount:                 agg.TradeStats.BuyCount,
			SellCount:                agg.TradeStats.SellCount,
			WinSellRatio:             round4(winRatio),
			AvgHoldingDaysBeforeSell: round2(avgHolding),
			Timeline:                 timeline,
			Insights:                 buildTradingInsights(agg),
		})
	}
	return payload, nil
}

func (s *Service) GetAttributionMarket(ctx context.Context, userID string, query PortfolioAttributionQuery) (*PortfolioAttributionMarketPayload, error) {
	dataset, err := s.loadAttributionDataset(ctx, userID, query, nil, nil)
	if err != nil {
		return nil, err
	}
	payload := &PortfolioAttributionMarketPayload{PortfolioAttributionMeta: dataset.Meta}
	if !dataset.Meta.HasData {
		return payload, nil
	}
	payload.Groups = make([]PortfolioAttributionMarketGroup, 0, len(dataset.Scopes))
	for _, scope := range dataset.Scopes {
		agg := dataset.ScopeAggregates[scope]
		if agg == nil {
			continue
		}
		payload.Groups = append(payload.Groups, PortfolioAttributionMarketGroup{
			Scope:                       agg.Scope,
			ScopeLabel:                  agg.ScopeLabel,
			CurrencyCode:                agg.CurrencyCode,
			CurrencySymbol:              agg.CurrencySymbol,
			BenchmarkCode:               agg.BenchmarkCode,
			BenchmarkName:               agg.BenchmarkName,
			PortfolioReturnPct:          round4(agg.TotalReturnPct),
			BenchmarkReturnPct:          round4(agg.BenchmarkReturnPct),
			ExcessReturnPct:             round4(agg.TotalReturnPct - agg.BenchmarkReturnPct),
			MarketContributionAmount:    round2(agg.MarketContributionAmount),
			SelectionContributionAmount: round2(agg.SelectionContributionAmount),
			TradingAlphaAmount:          round2(agg.TradingAlphaAmount),
			FeeAmount:                   round2(agg.FeeAmount),
			Series:                      agg.Series,
			Insights:                    buildMarketInsights(agg),
		})
	}
	return payload, nil
}

func (s *Service) loadAttributionDataset(ctx context.Context, userID string, query PortfolioAttributionQuery, historyReader attributionHistoryReader, benchmarkReader attributionBenchmarkReader) (*attributionDataset, error) {
	normalized, err := s.normalizeAttributionQuery(ctx, userID, query)
	if err != nil {
		return nil, err
	}
	if err := s.ensureInitEventsForUser(ctx, userID); err != nil {
		return nil, err
	}
	events, err := s.repo.ListActiveEventsByUserAsc(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.loadAttributionDatasetFromEvents(ctx, userID, events, normalized, historyReader, benchmarkReader)
}

func (s *Service) loadAttributionDatasetFromEvents(ctx context.Context, userID string, events []PortfolioEventRecord, query PortfolioAttributionQuery, historyReader attributionHistoryReader, benchmarkReader attributionBenchmarkReader) (*attributionDataset, error) {
	normalized := query
	meta := PortfolioAttributionMeta{
		Scope:      normalized.Scope,
		Range:      normalized.Range,
		StartDate:  normalized.StartDate,
		EndDate:    normalized.EndDate,
		ComputedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if len(events) == 0 {
		meta.EmptyReason = "暂无持仓事件，暂时无法生成绩效归因分析。"
		return &attributionDataset{Meta: meta, ComputedAt: time.Now().UTC()}, nil
	}
	if historyReader == nil {
		historyRepo, hErr := NewRiskDBRepository()
		if hErr == nil {
			historyReader = historyRepo
		}
	}
	if benchmarkReader == nil {
		benchmarkReader = &defaultAttributionBenchmarkReader{client: live.NewMarketClient()}
	}

	profiles, err := s.ensureSecurityProfiles(ctx, events)
	if err != nil {
		return nil, err
	}
	profileBySymbol := make(map[string]SecurityProfileRecord, len(profiles))
	for _, item := range profiles {
		profileBySymbol[item.Symbol] = item
	}

	dateSet := map[string]struct{}{}
	symbolSet := map[string]struct{}{}
	preStates := map[string]*attributionPositionState{}
	tradeStatsByScope := map[string]*attributionTradeStats{}
	tradeStatsBySymbol := map[string]*attributionTradeStats{}
	scopeRealizedCum := map[string]float64{PortfolioScopeAShare: 0, PortfolioScopeHK: 0}
	eventsByDate := map[string][]PortfolioEventRecord{}

	for _, event := range events {
		normalizedSymbol, exchange := normalizeAttributionSymbol(event.Symbol)
		if normalizedSymbol == "" {
			continue
		}
		profile := profileBySymbol[normalizedSymbol]
		if strings.TrimSpace(exchange) == "" {
			exchange = profile.Exchange
		}
		if strings.TrimSpace(exchange) == "" {
			exchange = live.ExchangeFromSymbol(normalizedSymbol)
		}
		scope := exchangeToScope(exchange)
		if scope == "" {
			continue
		}
		state := preStates[normalizedSymbol]
		if state == nil {
			cc, cs := resolveCurrency(exchange)
			state = &attributionPositionState{
				OriginalSymbol:   event.Symbol,
				NormalizedSymbol: normalizedSymbol,
				Name:             coalesceString(profile.Name, normalizedSymbol),
				Exchange:         exchange,
				CurrencyCode:     cc,
				CurrencySymbol:   cs,
				CostSource:       CostSourceSystem,
				SectorCode:       profile.SectorCode,
				SectorName:       normalizeSectorName(profile.SectorName),
				BenchmarkCode:    resolveBenchmarkCode(exchange, profile.BenchmarkCode),
			}
			preStates[normalizedSymbol] = state
		}

		if event.TradeDate < normalized.StartDate {
			applyAttributionEventToState(state, event)
			continue
		}
		if event.TradeDate > normalized.EndDate {
			continue
		}

		dateSet[event.TradeDate] = struct{}{}
		eventsByDate[event.TradeDate] = append(eventsByDate[event.TradeDate], event)
		symbolSet[normalizedSymbol] = struct{}{}
		stats := ensureAttributionTradeStats(tradeStatsByScope, scope)
		symbolTrade := ensureAttributionTradeStats(tradeStatsBySymbol, normalizedSymbol)
		applyTradeStats(stats, symbolTrade, state, event, normalizedSymbol, coalesceString(profile.Name, normalizedSymbol))
	}

	shadowStartStates := cloneAttributionStates(preStates)
	for symbol, state := range preStates {
		if state.Shares > 0 {
			symbolSet[symbol] = struct{}{}
		}
	}
	if len(symbolSet) == 0 {
		meta.EmptyReason = "所选时间范围内没有可归因的持仓或交易记录。"
		return &attributionDataset{Meta: meta, ComputedAt: time.Now().UTC()}, nil
	}
	if historyReader == nil {
		meta.EmptyReason = "本地历史行情缓存不可用，暂时无法生成绩效归因分析。"
		return &attributionDataset{Meta: meta, ComputedAt: time.Now().UTC()}, nil
	}

	symbols := sortedStringKeys(symbolSet)
	barCodeToSymbol := map[string]string{}
	barCodes := make([]string, 0, len(symbols))
	for _, symbol := range symbols {
		code := historyCodeFromSymbol(symbol)
		if code == "" {
			continue
		}
		barCodeToSymbol[code] = symbol
		barCodes = append(barCodes, code)
	}
	lookupStart := shiftDateString(normalized.StartDate, -7)
	barsByCode, err := historyReader.GetDailyBars(ctx, barCodes, lookupStart, normalized.EndDate)
	if err != nil {
		return nil, fmt.Errorf("failed to compute portfolio attribution: %w", err)
	}
	barsBySymbol := map[string][]DailyBarRecord{}
	for code, bars := range barsByCode {
		symbol := barCodeToSymbol[code]
		if symbol == "" {
			continue
		}
		barsBySymbol[symbol] = bars
		for _, bar := range bars {
			if bar.Date >= normalized.StartDate && bar.Date <= normalized.EndDate {
				dateSet[bar.Date] = struct{}{}
			}
		}
	}

	requestedScopes := scopeListForAttribution(normalized.Scope, symbols)
	benchmarkBarsByScope := map[string][]live.DailyBar{}
	for _, scope := range requestedScopes {
		benchmarkCode := benchmarkCodeForScope(scope)
		bars, benchErr := benchmarkReader.FetchBenchmarkDailyBars(ctx, benchmarkCode, calcLookbackDays(normalized.StartDate, normalized.EndDate)+10)
		if benchErr == nil {
			benchmarkBarsByScope[scope] = bars
			for _, bar := range bars {
				if bar.Date >= normalized.StartDate && bar.Date <= normalized.EndDate {
					dateSet[bar.Date] = struct{}{}
				}
			}
		}
	}

	dates := sortedStringKeys(dateSet)
	if len(dates) == 0 {
		meta.EmptyReason = "所选时间范围内没有持仓快照或交易流水，暂时无法生成归因分析。"
		return &attributionDataset{Meta: meta, ComputedAt: time.Now().UTC()}, nil
	}

	rangeStates := cloneAttributionStates(shadowStartStates)
	allSnapshots := make([]PortfolioPositionDailySnapshotRecord, 0)
	scopeDailyTotals := map[string][]attributionScopeDailyTotal{}
	symbolSnapshots := map[string][]attributionSnapshotLite{}
	now := time.Now().UTC()

	for _, date := range dates {
		for _, event := range eventsByDate[date] {
			normalizedSymbol, exchange := normalizeAttributionSymbol(event.Symbol)
			if normalizedSymbol == "" {
				continue
			}
			state := rangeStates[normalizedSymbol]
			if state == nil {
				profile := profileBySymbol[normalizedSymbol]
				cc, cs := resolveCurrency(exchange)
				state = &attributionPositionState{
					OriginalSymbol:   event.Symbol,
					NormalizedSymbol: normalizedSymbol,
					Name:             coalesceString(profile.Name, normalizedSymbol),
					Exchange:         exchange,
					CurrencyCode:     cc,
					CurrencySymbol:   cs,
					SectorCode:       profile.SectorCode,
					SectorName:       normalizeSectorName(profile.SectorName),
					BenchmarkCode:    resolveBenchmarkCode(exchange, profile.BenchmarkCode),
				}
				rangeStates[normalizedSymbol] = state
			}
			applyAttributionEventToState(state, event)
			scope := exchangeToScope(state.Exchange)
			scopeRealizedCum[scope] += event.RealizedPnlAmount
		}

		daySnapshots := make([]*PortfolioPositionDailySnapshotRecord, 0)
		totalByScope := map[string]float64{}
		for _, symbol := range symbols {
			state := rangeStates[symbol]
			if state == nil || state.Shares <= 0 {
				continue
			}
			closePrice, prevClose, ok := lookupBarCloseForDate(barsBySymbol[symbol], date)
			if !ok || closePrice <= 0 {
				continue
			}
			scope := exchangeToScope(state.Exchange)
			marketValue := state.Shares * closePrice
			unrealized := marketValue - state.TotalCostAmount
			snapshot := &PortfolioPositionDailySnapshotRecord{
				ID:                  uuid.New().String(),
				UserID:              userID,
				SnapshotDate:        date,
				Symbol:              state.NormalizedSymbol,
				Exchange:            state.Exchange,
				CurrencyCode:        state.CurrencyCode,
				CurrencySymbol:      state.CurrencySymbol,
				Name:                state.Name,
				Shares:              state.Shares,
				AvgCostPrice:        state.AvgCostPrice,
				TotalCostAmount:     state.TotalCostAmount,
				ClosePrice:          closePrice,
				PrevClosePrice:      prevClose,
				MarketValueAmount:   marketValue,
				UnrealizedPnlAmount: unrealized,
				RealizedPnlCum:      scopeRealizedCum[scope],
				SectorCode:          state.SectorCode,
				SectorName:          coalesceString(state.SectorName, "未分类"),
				BenchmarkCode:       state.BenchmarkCode,
				CreatedAt:           now,
				UpdatedAt:           now,
			}
			daySnapshots = append(daySnapshots, snapshot)
			totalByScope[scope] += marketValue
		}
		byScope := map[string]*attributionScopeDailyTotal{}
		for _, snapshot := range daySnapshots {
			scope := exchangeToScope(snapshot.Exchange)
			if totalByScope[scope] > 0 {
				snapshot.PositionWeightRatio = snapshot.MarketValueAmount / totalByScope[scope]
			}
			allSnapshots = append(allSnapshots, *snapshot)
			symbolSnapshots[snapshot.Symbol] = append(symbolSnapshots[snapshot.Symbol], attributionSnapshotLite{
				Date:                snapshot.SnapshotDate,
				ClosePrice:          snapshot.ClosePrice,
				PrevClosePrice:      snapshot.PrevClosePrice,
				MarketValueAmount:   snapshot.MarketValueAmount,
				UnrealizedPnlAmount: snapshot.UnrealizedPnlAmount,
				PositionWeightRatio: snapshot.PositionWeightRatio,
				TotalCostAmount:     snapshot.TotalCostAmount,
				Shares:              snapshot.Shares,
			})
			item := byScope[scope]
			if item == nil {
				item = &attributionScopeDailyTotal{Date: date, Scope: scope, CurrencyCode: snapshot.CurrencyCode, CurrencySymbol: snapshot.CurrencySymbol}
				byScope[scope] = item
			}
			item.MarketValueAmount += snapshot.MarketValueAmount
			item.UnrealizedPnl += snapshot.UnrealizedPnlAmount
			item.RangeRealizedPnl = scopeRealizedCum[scope]
			if snapshot.PositionWeightRatio > item.MaxWeightRatio {
				item.MaxWeightRatio = snapshot.PositionWeightRatio
			}
		}
		for scope, item := range byScope {
			scopeDailyTotals[scope] = append(scopeDailyTotals[scope], *item)
		}
	}

	if len(allSnapshots) > 0 {
		if err := s.repo.DeletePositionDailySnapshotsInRange(ctx, userID, normalized.StartDate, normalized.EndDate); err == nil {
			_ = s.repo.UpsertPositionDailySnapshots(ctx, allSnapshots)
		}
	}

	scopeAggregates := map[string]*attributionScopeAggregate{}
	symbolsByScope := map[string][]attributionSymbolAggregate{}
	for _, scope := range requestedScopes {
		dailies := scopeDailyTotals[scope]
		if len(dailies) == 0 {
			continue
		}
		agg := &attributionScopeAggregate{
			Scope:          scope,
			ScopeLabel:     scopeLabel(scope),
			CurrencyCode:   dailies[0].CurrencyCode,
			CurrencySymbol: dailies[0].CurrencySymbol,
			DailyTotals:    dailies,
			TradeStats:     *ensureAttributionTradeStats(tradeStatsByScope, scope),
		}
		agg.StartMarketValueAmount = dailies[0].MarketValueAmount
		agg.EndMarketValueAmount = dailies[len(dailies)-1].MarketValueAmount
		agg.StartUnrealizedPnlAmount = dailies[0].UnrealizedPnl
		agg.EndUnrealizedPnlAmount = dailies[len(dailies)-1].UnrealizedPnl
		agg.RealizedPnlAmount = agg.TradeStats.RealizedPnlAmount
		agg.FeeAmount = agg.TradeStats.FeeAmount
		agg.UnrealizedPnlChangeAmount = agg.EndUnrealizedPnlAmount - agg.StartUnrealizedPnlAmount
		agg.TotalPnlAmount = agg.RealizedPnlAmount + agg.UnrealizedPnlChangeAmount
		if agg.StartMarketValueAmount > 0 {
			agg.TotalReturnPct = agg.TotalPnlAmount / agg.StartMarketValueAmount
		}
		agg.BenchmarkCode = benchmarkCodeForScope(scope)
		agg.BenchmarkName = benchmarkNameForCode(agg.BenchmarkCode)
		agg.BenchmarkReturnPct, agg.Series = buildMarketSeries(scope, dailies, benchmarkBarsByScope[scope])
		agg.MarketContributionAmount = agg.StartMarketValueAmount * agg.BenchmarkReturnPct
		agg.ShadowHoldPnlAmount = calcShadowHoldPnl(scope, shadowStartStates, barsBySymbol, normalized.StartDate, normalized.EndDate)
		agg.TradingAlphaAmount = agg.TotalPnlAmount - agg.ShadowHoldPnlAmount - agg.FeeAmount
		agg.SelectionContributionAmount = agg.TotalPnlAmount - agg.MarketContributionAmount - agg.TradingAlphaAmount + agg.FeeAmount
		agg.ExcessContributionAmount = agg.TotalPnlAmount - agg.MarketContributionAmount
		scopeAggregates[scope] = agg
	}

	for _, symbol := range symbols {
		snaps := symbolSnapshots[symbol]
		state := rangeStates[symbol]
		if len(snaps) == 0 && tradeStatsBySymbol[symbol] == nil {
			continue
		}
		if state == nil {
			continue
		}
		scope := exchangeToScope(state.Exchange)
		agg := attributionSymbolAggregate{
			Symbol:         symbol,
			Name:           state.Name,
			Exchange:       state.Exchange,
			CurrencyCode:   state.CurrencyCode,
			CurrencySymbol: state.CurrencySymbol,
			SectorCode:     state.SectorCode,
			SectorName:     coalesceString(state.SectorName, "未分类"),
			BenchmarkCode:  state.BenchmarkCode,
			Snapshots:      snaps,
			DetailURL:      fmt.Sprintf("/live-trading/%s", symbol),
		}
		if len(snaps) > 0 {
			agg.StartWeightRatio = snaps[0].PositionWeightRatio
			agg.EndWeightRatio = snaps[len(snaps)-1].PositionWeightRatio
			var weightSum float64
			for _, snap := range snaps {
				weightSum += snap.PositionWeightRatio
			}
			agg.AvgWeightRatio = weightSum / float64(len(snaps))
			agg.UnrealizedPnlChangeAmount = snaps[len(snaps)-1].UnrealizedPnlAmount - snaps[0].UnrealizedPnlAmount
			agg.HoldingDays = len(snaps)
			basis := snaps[0].TotalCostAmount
			if basis <= 0 {
				basis = snaps[len(snaps)-1].TotalCostAmount
			}
			agg.HoldingReturnPct = safeRatio(agg.UnrealizedPnlChangeAmount, basis)
		}
		if tradeStats := tradeStatsBySymbol[symbol]; tradeStats != nil {
			agg.RealizedPnlAmount = tradeStats.RealizedPnlAmount
			agg.BuyCount = tradeStats.BuyCount
			agg.SellCount = tradeStats.SellCount
		}
		agg.TotalPnlAmount = agg.RealizedPnlAmount + agg.UnrealizedPnlChangeAmount
		if len(snaps) > 0 {
			basis := snaps[0].TotalCostAmount
			if basis > 0 {
				agg.HoldingReturnPct = agg.TotalPnlAmount / basis
			}
		}
		symbolsByScope[scope] = append(symbolsByScope[scope], agg)
	}

	usedScopes := make([]string, 0, len(scopeAggregates))
	for _, scope := range requestedScopes {
		if scopeAggregates[scope] != nil {
			usedScopes = append(usedScopes, scope)
		}
	}
	meta.HasData = len(usedScopes) > 0
	meta.MixedCurrency = len(usedScopes) > 1
	if !meta.HasData {
		meta.EmptyReason = "所选时间范围内没有持仓快照或交易流水，暂时无法生成归因分析。"
	}
	return &attributionDataset{
		Meta:            meta,
		ComputedAt:      now,
		Scopes:          usedScopes,
		ScopeAggregates: scopeAggregates,
		SymbolsByScope:  symbolsByScope,
	}, nil
}

func (s *Service) normalizeAttributionQuery(ctx context.Context, userID string, query PortfolioAttributionQuery) (PortfolioAttributionQuery, error) {
	scope, err := normalizePortfolioScope(query.Scope)
	if err != nil {
		return PortfolioAttributionQuery{}, fmt.Errorf("invalid scope")
	}
	rangeLabel := strings.ToUpper(strings.TrimSpace(query.Range))
	if rangeLabel == "" {
		rangeLabel = AttributionRange30D
	}
	if rangeLabel != AttributionRange7D && rangeLabel != AttributionRange30D && rangeLabel != AttributionRange90D && rangeLabel != AttributionRangeAll && rangeLabel != AttributionRangeCustom {
		return PortfolioAttributionQuery{}, fmt.Errorf("invalid range")
	}
	result := query
	result.Scope = scope
	result.Range = rangeLabel
	result.IncludeUnclassified = query.IncludeUnclassified
	if result.Limit <= 0 {
		result.Limit = 5
	}
	if result.TimelineLimit <= 0 {
		result.TimelineLimit = 20
	}
	sortBy := strings.TrimSpace(strings.ToLower(query.SortBy))
	if sortBy != "" {
		switch sortBy {
		case "contribution", "total_pnl", "realized_pnl", "unrealized_pnl_change", "avg_weight", "return_pct":
		default:
			return PortfolioAttributionQuery{}, fmt.Errorf("invalid sort field")
		}
	}
	result.SortBy = sortBy

	nowDate := time.Now().In(shanghaiLocation()).Format("2006-01-02")
	result.EndDate = nowDate
	switch rangeLabel {
	case AttributionRangeCustom:
		if strings.TrimSpace(query.StartDate) == "" || strings.TrimSpace(query.EndDate) == "" {
			return PortfolioAttributionQuery{}, fmt.Errorf("custom range requires start_date and end_date")
		}
		if !isDateString(query.StartDate) || !isDateString(query.EndDate) || query.StartDate > query.EndDate {
			return PortfolioAttributionQuery{}, fmt.Errorf("invalid date range")
		}
		if calcLookbackDays(query.StartDate, query.EndDate) > 365 {
			return PortfolioAttributionQuery{}, fmt.Errorf("range exceeds maximum supported days")
		}
		result.StartDate = query.StartDate
		result.EndDate = query.EndDate
	case AttributionRangeAll:
		startDate, err := s.repo.GetEarliestActiveEventDate(ctx, userID)
		if err == nil && startDate != "" {
			result.StartDate = startDate
		} else {
			result.StartDate = nowDate
		}
	case AttributionRange7D:
		result.StartDate = shiftDateString(nowDate, -6)
	case AttributionRange90D:
		result.StartDate = shiftDateString(nowDate, -89)
	default:
		result.StartDate = shiftDateString(nowDate, -29)
	}
	return result, nil
}

func (s *Service) ensureInitEventsForUser(ctx context.Context, userID string) error {
	records, err := s.repo.ListByUser(ctx, userID)
	if err != nil {
		return err
	}
	for _, record := range records {
		if err := s.EnsureInitEventFromSnapshot(ctx, userID, record.Symbol); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) ensureSecurityProfiles(ctx context.Context, events []PortfolioEventRecord) ([]SecurityProfileRecord, error) {
	symbolSet := map[string]struct{}{}
	for _, event := range events {
		normalized, _ := normalizeAttributionSymbol(event.Symbol)
		if normalized != "" {
			symbolSet[normalized] = struct{}{}
		}
	}
	symbols := sortedStringKeys(symbolSet)
	existing, err := s.repo.ListSecurityProfilesBySymbols(ctx, symbols)
	if err != nil {
		return nil, err
	}
	existingMap := make(map[string]SecurityProfileRecord, len(existing))
	for _, item := range existing {
		existingMap[item.Symbol] = item
	}
	missing := make([]string, 0)
	for _, symbol := range symbols {
		if _, ok := existingMap[symbol]; !ok {
			missing = append(missing, symbol)
		}
	}
	if len(missing) == 0 {
		return existing, nil
	}
	snapshotMap := map[string]portfolioMarketSnapshot{}
	if s.snapshotProvider != nil {
		if got, snapErr := s.snapshotProvider.FetchDetailedSnapshots(ctx, missing); snapErr == nil {
			snapshotMap = got
		}
	}
	now := time.Now().UTC()
	upserts := make([]SecurityProfileRecord, 0, len(missing))
	for _, symbol := range missing {
		exchange := live.ExchangeFromSymbol(symbol)
		snapshot := snapshotMap[symbol]
		name := strings.TrimSpace(snapshot.Name)
		if name == "" {
			name = symbol
		}
		upserts = append(upserts, SecurityProfileRecord{
			Symbol:        symbol,
			Exchange:      coalesceString(snapshot.Exchange, exchange),
			Name:          name,
			SectorCode:    "",
			SectorName:    "",
			BenchmarkCode: resolveBenchmarkCode(exchange, ""),
			Source:        "system",
			CreatedAt:     now,
			UpdatedAt:     now,
		})
	}
	if err := s.repo.UpsertSecurityProfiles(ctx, upserts); err != nil {
		return nil, err
	}
	return s.repo.ListSecurityProfilesBySymbols(ctx, symbols)
}

func applyAttributionEventToState(state *attributionPositionState, event PortfolioEventRecord) {
	if state == nil {
		return
	}
	beforeShares := state.Shares
	state.Shares = event.AfterShares
	state.AvgCostPrice = event.AfterAvgCostPrice
	state.TotalCostAmount = event.AfterTotalCost
	state.CostSource = CostSourceSystem
	if event.EventType == EventTypeAdjustAvgCost || event.EventType == EventTypeSyncPosition {
		state.CostSource = CostSourceManual
	}
	switch event.EventType {
	case EventTypeInit:
		if event.AfterShares > 0 {
			state.BuyDate = event.TradeDate
		}
	case EventTypeBuy:
		if beforeShares <= 0 && event.AfterShares > 0 {
			state.BuyDate = event.TradeDate
		}
	case EventTypeSell:
		if event.AfterShares <= 0 {
			state.BuyDate = ""
		}
	case EventTypeSyncPosition:
		if event.AfterShares > 0 && beforeShares <= 0 {
			state.BuyDate = event.TradeDate
		}
		if event.AfterShares <= 0 {
			state.BuyDate = ""
		}
	}
}

func ensureAttributionTradeStats(target map[string]*attributionTradeStats, key string) *attributionTradeStats {
	item := target[key]
	if item == nil {
		item = &attributionTradeStats{}
		target[key] = item
	}
	return item
}

func applyTradeStats(scopeStats, symbolStats *attributionTradeStats, state *attributionPositionState, event PortfolioEventRecord, normalizedSymbol, name string) {
	apply := func(stats *attributionTradeStats) {
		stats.TradeCount++
		stats.FeeAmount += event.FeeAmount
		stats.RealizedPnlAmount += event.RealizedPnlAmount
		timingEffect := event.RealizedPnlAmount
		switch event.EventType {
		case EventTypeBuy:
			stats.BuyCount++
			timingEffect = -event.FeeAmount
		case EventTypeSell:
			stats.SellCount++
			stats.SellAmount += event.Quantity * event.Price
			if event.RealizedPnlAmount > 0 {
				stats.WinningSellCount++
			}
			if state != nil && state.BuyDate != "" && isDateString(state.BuyDate) && isDateString(event.TradeDate) {
				stats.HoldingDaysBeforeSell += calcLookbackDays(state.BuyDate, event.TradeDate)
				stats.HoldingDaysSampleCount++
			}
		case EventTypeAdjustAvgCost:
			timingEffect = 0
		}
		stats.Timeline = append(stats.Timeline, PortfolioAttributionTradingTimelineItem{
			EventID:            event.ID,
			TradeDate:          event.TradeDate,
			Symbol:             normalizedSymbol,
			Name:               name,
			EventType:          event.EventType,
			FeeAmount:          round2(event.FeeAmount),
			RealizedPnlAmount:  round2(event.RealizedPnlAmount),
			TimingEffectAmount: round2(timingEffect),
			Note:               event.Note,
		})
	}
	apply(scopeStats)
	apply(symbolStats)
}

func toAttributionStockItems(items []attributionSymbolAggregate, scopeTotalPnl float64) []PortfolioAttributionStockItem {
	result := make([]PortfolioAttributionStockItem, 0, len(items))
	for _, item := range items {
		contribution := 0.0
		if scopeTotalPnl != 0 {
			contribution = item.TotalPnlAmount / scopeTotalPnl
		}
		driverTag, driverLabel := resolveDriver(item)
		result = append(result, PortfolioAttributionStockItem{
			Symbol:                    item.Symbol,
			Name:                      item.Name,
			Exchange:                  item.Exchange,
			ExchangeLabel:             exchangeLabel(item.Exchange),
			SectorCode:                item.SectorCode,
			SectorName:                coalesceString(item.SectorName, "未分类"),
			StartWeightRatio:          round4(item.StartWeightRatio),
			EndWeightRatio:            round4(item.EndWeightRatio),
			AvgWeightRatio:            round4(item.AvgWeightRatio),
			RealizedPnlAmount:         round2(item.RealizedPnlAmount),
			UnrealizedPnlChangeAmount: round2(item.UnrealizedPnlChangeAmount),
			TotalPnlAmount:            round2(item.TotalPnlAmount),
			ContributionRatio:         round4(contribution),
			HoldingReturnPct:          round4(item.HoldingReturnPct),
			BuyCount:                  item.BuyCount,
			SellCount:                 item.SellCount,
			HoldingDays:               item.HoldingDays,
			DriverTag:                 driverTag,
			DriverLabel:               driverLabel,
			DetailURL:                 item.DetailURL,
		})
	}
	return result
}

func buildSectorItems(items []attributionSymbolAggregate, scopeTotalPnl float64, includeUnclassified bool, limit int) []PortfolioAttributionSectorItem {
	type bucket struct {
		PortfolioAttributionSectorItem
		winnerValue float64
		loserValue  float64
	}
	sectorMap := map[string]*bucket{}
	for _, item := range items {
		sectorName := coalesceString(item.SectorName, "未分类")
		if !includeUnclassified && isUnclassifiedSector(sectorName) {
			continue
		}
		key := item.SectorCode + "|" + sectorName
		entry := sectorMap[key]
		if entry == nil {
			entry = &bucket{PortfolioAttributionSectorItem: PortfolioAttributionSectorItem{SectorCode: item.SectorCode, SectorName: sectorName}}
			sectorMap[key] = entry
		}
		entry.StockCount++
		entry.StartWeightRatio += item.StartWeightRatio
		entry.EndWeightRatio += item.EndWeightRatio
		entry.AvgWeightRatio += item.AvgWeightRatio
		entry.RealizedPnlAmount += item.RealizedPnlAmount
		entry.UnrealizedPnlChangeAmount += item.UnrealizedPnlChangeAmount
		entry.TotalPnlAmount += item.TotalPnlAmount
		if item.TotalPnlAmount > entry.winnerValue || entry.TopWinnerSymbol == "" {
			entry.winnerValue = item.TotalPnlAmount
			entry.TopWinnerSymbol = item.Symbol
		}
		if item.TotalPnlAmount < entry.loserValue || entry.TopLoserSymbol == "" {
			entry.loserValue = item.TotalPnlAmount
			entry.TopLoserSymbol = item.Symbol
		}
	}
	result := make([]PortfolioAttributionSectorItem, 0, len(sectorMap))
	for _, entry := range sectorMap {
		if scopeTotalPnl != 0 {
			entry.ContributionRatio = round4(entry.TotalPnlAmount / scopeTotalPnl)
		}
		basis := entry.StartWeightRatio
		if basis > 0 {
			entry.SectorReturnPct = round4(entry.TotalPnlAmount / basis)
		}
		entry.StartWeightRatio = round4(entry.StartWeightRatio)
		entry.EndWeightRatio = round4(entry.EndWeightRatio)
		entry.AvgWeightRatio = round4(entry.AvgWeightRatio)
		entry.RealizedPnlAmount = round2(entry.RealizedPnlAmount)
		entry.UnrealizedPnlChangeAmount = round2(entry.UnrealizedPnlChangeAmount)
		entry.TotalPnlAmount = round2(entry.TotalPnlAmount)
		entry.DriverLabel = buildSectorDriverLabel(entry.TotalPnlAmount, entry.AvgWeightRatio)
		result = append(result, entry.PortfolioAttributionSectorItem)
	}
	sort.SliceStable(result, func(i, j int) bool { return math.Abs(result[i].TotalPnlAmount) > math.Abs(result[j].TotalPnlAmount) })
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result
}

func buildWaterfallGroup(agg *attributionScopeAggregate) PortfolioAttributionWaterfallGroup {
	marketRatio := safeRatio(agg.MarketContributionAmount, agg.TotalPnlAmount)
	selectionRatio := safeRatio(agg.SelectionContributionAmount, agg.TotalPnlAmount)
	tradingRatio := safeRatio(agg.TradingAlphaAmount, agg.TotalPnlAmount)
	feeRatio := safeRatio(-agg.FeeAmount, agg.TotalPnlAmount)
	totalRatio := 1.0
	return PortfolioAttributionWaterfallGroup{
		Scope:          agg.Scope,
		ScopeLabel:     agg.ScopeLabel,
		CurrencyCode:   agg.CurrencyCode,
		CurrencySymbol: agg.CurrencySymbol,
		Items: []PortfolioAttributionWaterfallItem{
			{Key: "market_move", Label: "市场行情", Type: valueToneType(agg.MarketContributionAmount), Amount: round2(agg.MarketContributionAmount), Ratio: ratioPtr(marketRatio), DisplayOrder: 1, Tooltip: "跟随对应市场基准上涨或下跌带来的收益变化。"},
			{Key: "stock_selection", Label: "选股超额", Type: valueToneType(agg.SelectionContributionAmount), Amount: round2(agg.SelectionContributionAmount), Ratio: ratioPtr(selectionRatio), DisplayOrder: 2, Tooltip: "组合相对基准跑赢或跑输的部分。"},
			{Key: "trading_alpha", Label: "调仓贡献", Type: valueToneType(agg.TradingAlphaAmount), Amount: round2(agg.TradingAlphaAmount), Ratio: ratioPtr(tradingRatio), DisplayOrder: 3, Tooltip: "实际调仓相对区间起始冻结组合的额外增益或拖累。"},
			{Key: "fee_drag", Label: "手续费", Type: valueToneType(-agg.FeeAmount), Amount: round2(-agg.FeeAmount), Ratio: ratioPtr(feeRatio), DisplayOrder: 4, Tooltip: "区间内全部有效交易产生的显式手续费。"},
			{Key: "end_pnl", Label: "区间总收益", Type: "total", Amount: round2(agg.TotalPnlAmount), Ratio: ratioPtr(totalRatio), DisplayOrder: 5, Tooltip: "该市场范围在当前区间内的总收益。"},
		},
	}
}

func buildAttributionSummaryCards(dataset *attributionDataset) []PortfolioAttributionSummaryCard {
	cards := make([]PortfolioAttributionSummaryCard, 0)
	if dataset.Meta.MixedCurrency {
		cards = append(cards,
			PortfolioAttributionSummaryCard{Key: "total_pnl", Label: "总收益", CurrencyCode: "", CurrencySymbol: "", Tone: overallTone(dataset), Tooltip: "A/H 混仓场景下不返回单一金额总值，请结合下方分市场卡片查看。"},
			PortfolioAttributionSummaryCard{Key: "fee_drag", Label: "手续费拖累", CurrencyCode: "", CurrencySymbol: "", Tone: "negative", Tooltip: "区间内全部有效交易产生的显式手续费。"},
		)
		return cards
	}
	if len(dataset.Scopes) == 0 {
		return cards
	}
	agg := dataset.ScopeAggregates[dataset.Scopes[0]]
	if agg == nil {
		return cards
	}
	total := round2(agg.TotalPnlAmount)
	realized := round2(agg.RealizedPnlAmount)
	unrealized := round2(agg.UnrealizedPnlChangeAmount)
	feeDrag := round2(-agg.FeeAmount)
	tradingAlpha := round2(agg.TradingAlphaAmount)
	excess := round2(agg.ExcessContributionAmount)
	cards = append(cards,
		PortfolioAttributionSummaryCard{Key: "total_pnl", Label: "总收益", ValueAmount: &total, CurrencyCode: agg.CurrencyCode, CurrencySymbol: agg.CurrencySymbol, Tone: toneForValue(agg.TotalPnlAmount), Tooltip: "当前区间内的总收益，等于已实现盈亏与未实现盈亏变化之和。"},
		PortfolioAttributionSummaryCard{Key: "realized_pnl", Label: "已实现盈亏", ValueAmount: &realized, CurrencyCode: agg.CurrencyCode, CurrencySymbol: agg.CurrencySymbol, Tone: toneForValue(agg.RealizedPnlAmount), Tooltip: "区间内卖出交易已经落袋的累计盈亏。"},
		PortfolioAttributionSummaryCard{Key: "unrealized_pnl_change", Label: "未实现盈亏变化", ValueAmount: &unrealized, CurrencyCode: agg.CurrencyCode, CurrencySymbol: agg.CurrencySymbol, Tone: toneForValue(agg.UnrealizedPnlChangeAmount), Tooltip: "区间起点到终点之间，仍在持有仓位的浮盈浮亏变化。"},
		PortfolioAttributionSummaryCard{Key: "fee_drag", Label: "手续费拖累", ValueAmount: &feeDrag, CurrencyCode: agg.CurrencyCode, CurrencySymbol: agg.CurrencySymbol, Tone: toneForValue(-agg.FeeAmount), Tooltip: "区间内全部有效交易产生的显式手续费。"},
		PortfolioAttributionSummaryCard{Key: "trading_alpha", Label: "调仓贡献", ValueAmount: &tradingAlpha, CurrencyCode: agg.CurrencyCode, CurrencySymbol: agg.CurrencySymbol, Tone: toneForValue(agg.TradingAlphaAmount), Tooltip: "实际调仓相对区间起始冻结组合的额外增益或拖累。"},
		PortfolioAttributionSummaryCard{Key: "market_excess", Label: "超额收益", ValueAmount: &excess, CurrencyCode: agg.CurrencyCode, CurrencySymbol: agg.CurrencySymbol, Tone: toneForValue(agg.ExcessContributionAmount), Tooltip: "组合相对市场基准跑赢或跑输的收益部分。"},
	)
	return cards
}

func buildAttributionSummaryInsights(dataset *attributionDataset) []PortfolioAttributionInsight {
	insights := make([]PortfolioAttributionInsight, 0)
	for _, scope := range dataset.Scopes {
		agg := dataset.ScopeAggregates[scope]
		if agg == nil {
			continue
		}
		if agg.TotalPnlAmount >= 0 {
			insights = append(insights, PortfolioAttributionInsight{
				Title:       fmt.Sprintf("%s收益为正", agg.ScopeLabel),
				Description: fmt.Sprintf("区间总收益 %s，市场贡献 %s，调仓贡献 %s。", formatCurrencyValue(agg.TotalPnlAmount, agg.CurrencySymbol), formatCurrencyValue(agg.MarketContributionAmount, agg.CurrencySymbol), formatCurrencyValue(agg.TradingAlphaAmount, agg.CurrencySymbol)),
				Severity:    "positive",
			})
		} else {
			insights = append(insights, PortfolioAttributionInsight{
				Title:       fmt.Sprintf("%s收益承压", agg.ScopeLabel),
				Description: fmt.Sprintf("区间总收益 %s，手续费 %s，调仓贡献 %s。", formatCurrencyValue(agg.TotalPnlAmount, agg.CurrencySymbol), formatCurrencyValue(-agg.FeeAmount, agg.CurrencySymbol), formatCurrencyValue(agg.TradingAlphaAmount, agg.CurrencySymbol)),
				Severity:    "warning",
			})
		}
	}
	return insights
}

func buildAttributionHeadline(dataset *attributionDataset) string {
	if len(dataset.Scopes) == 0 {
		return ""
	}
	best := dataset.ScopeAggregates[dataset.Scopes[0]]
	for _, scope := range dataset.Scopes[1:] {
		if dataset.ScopeAggregates[scope] != nil && dataset.ScopeAggregates[scope].TotalPnlAmount > best.TotalPnlAmount {
			best = dataset.ScopeAggregates[scope]
		}
	}
	if best == nil {
		return ""
	}
	if best.TotalPnlAmount >= 0 {
		return fmt.Sprintf("这段时间的收益主要来自%s，市场行情与选股超额共同拉动组合表现。", best.ScopeLabel)
	}
	return fmt.Sprintf("这段时间%s是主要拖累来源，建议重点复盘手续费与调仓节奏。", best.ScopeLabel)
}

func buildTradingInsights(agg *attributionScopeAggregate) []PortfolioAttributionInsight {
	if agg == nil {
		return nil
	}
	severity := "positive"
	title := "交易整体在加分"
	desc := "实际调仓相对冻结组合带来了额外正收益。"
	if agg.TradingAlphaAmount < 0 {
		severity = "warning"
		title = "交易整体略拖累"
		desc = "如果区间起始持仓不动，收益会更高；当前调仓未能完全抵消手续费与卖飞损失。"
	}
	return []PortfolioAttributionInsight{{Title: title, Description: desc, Severity: severity}}
}

func buildMarketInsights(agg *attributionScopeAggregate) []PortfolioAttributionInsight {
	if agg == nil {
		return nil
	}
	if agg.TotalReturnPct >= agg.BenchmarkReturnPct {
		return []PortfolioAttributionInsight{{
			Title:       fmt.Sprintf("%s组合跑赢基准", agg.ScopeLabel),
			Description: fmt.Sprintf("本区间组合收益率 %.2f%%，高于%s的 %.2f%%。", agg.TotalReturnPct*100, agg.BenchmarkName, agg.BenchmarkReturnPct*100),
			Severity:    "positive",
		}}
	}
	return []PortfolioAttributionInsight{{
		Title:       fmt.Sprintf("%s组合暂未跑赢基准", agg.ScopeLabel),
		Description: fmt.Sprintf("本区间组合收益率 %.2f%%，低于%s的 %.2f%%。", agg.TotalReturnPct*100, agg.BenchmarkName, agg.BenchmarkReturnPct*100),
		Severity:    "warning",
	}}
}

func resolveDriver(item attributionSymbolAggregate) (string, string) {
	if item.TotalPnlAmount >= 0 && item.RealizedPnlAmount > item.UnrealizedPnlChangeAmount {
		return "realized_gain", "已实现收益贡献更大"
	}
	if item.TotalPnlAmount >= 0 {
		return "trend_gain", "仓位较重且趋势收益明显"
	}
	if item.SellCount >= 2 {
		return "high_turnover", "交易频率较高但未形成正贡献"
	}
	return "rebound_loss", "区间表现拖累组合收益"
}

func buildSectorDriverLabel(totalPnl, avgWeight float64) string {
	if totalPnl >= 0 {
		if avgWeight >= 0.15 {
			return "重仓 + 板块上涨带来正贡献"
		}
		return "板块表现偏强，对组合形成正贡献"
	}
	if avgWeight >= 0.15 {
		return "仓位较重且板块走弱，是主要拖累来源"
	}
	return "板块表现偏弱，对组合形成负贡献"
}

func buildMarketSeries(scope string, totals []attributionScopeDailyTotal, benchmarkBars []live.DailyBar) (float64, []PortfolioAttributionMarketSeriesPoint) {
	if len(totals) == 0 {
		return 0, nil
	}
	firstBenchmark := 0.0
	lastBenchmark := 0.0
	firstEquity := totals[0].MarketValueAmount
	if firstEquity <= 0 {
		firstEquity = 1
	}
	prevEquity := firstEquity
	prevBenchmark := 0.0
	series := make([]PortfolioAttributionMarketSeriesPoint, 0, len(totals))
	for index, item := range totals {
		equity := totals[0].MarketValueAmount + item.RangeRealizedPnl + (item.UnrealizedPnl - totals[0].UnrealizedPnl)
		if equity <= 0 {
			equity = prevEquity
		}
		benchClose := lookupLiveBarCloseOnOrBefore(benchmarkBars, item.Date)
		if benchClose <= 0 {
			benchClose = prevBenchmark
		}
		if index == 0 {
			if benchClose > 0 {
				firstBenchmark = benchClose
			}
		}
		if benchClose > 0 {
			lastBenchmark = benchClose
		}
		portfolioNav := safeRatio(equity, firstEquity)
		benchmarkNav := 1.0
		if firstBenchmark > 0 && benchClose > 0 {
			benchmarkNav = benchClose / firstBenchmark
		}
		portfolioDaily := 0.0
		benchmarkDaily := 0.0
		if index > 0 && prevEquity > 0 {
			portfolioDaily = equity/prevEquity - 1
		}
		if index > 0 && prevBenchmark > 0 && benchClose > 0 {
			benchmarkDaily = benchClose/prevBenchmark - 1
		}
		series = append(series, PortfolioAttributionMarketSeriesPoint{
			Date:                    item.Date,
			PortfolioNav:            round4(portfolioNav),
			BenchmarkNav:            round4(benchmarkNav),
			DailyPortfolioReturnPct: round4(portfolioDaily),
			DailyBenchmarkReturnPct: round4(benchmarkDaily),
			DailyExcessReturnPct:    round4(portfolioDaily - benchmarkDaily),
			ActiveWeightRatio:       round4(item.MaxWeightRatio),
		})
		prevEquity = equity
		prevBenchmark = benchClose
	}
	benchmarkReturn := 0.0
	if firstBenchmark > 0 && lastBenchmark > 0 {
		benchmarkReturn = lastBenchmark/firstBenchmark - 1
	}
	return benchmarkReturn, series
}

func calcShadowHoldPnl(scope string, startStates map[string]*attributionPositionState, barsBySymbol map[string][]DailyBarRecord, startDate, endDate string) float64 {
	var total float64
	for symbol, state := range startStates {
		if state == nil || state.Shares <= 0 {
			continue
		}
		if exchangeToScope(state.Exchange) != scope {
			continue
		}
		startClose, ok := lookupFirstBarCloseOnOrAfterDate(barsBySymbol[symbol], startDate)
		if !ok || startClose <= 0 {
			continue
		}
		endClose, _, ok := lookupBarCloseForDate(barsBySymbol[symbol], endDate)
		if !ok || endClose <= 0 {
			endClose = startClose
		}
		total += state.Shares * (endClose - startClose)
	}
	return total
}

func calcTurnoverRatio(agg *attributionScopeAggregate) float64 {
	if agg == nil || agg.StartMarketValueAmount <= 0 {
		return 0
	}
	return agg.TradeStats.SellAmount / agg.StartMarketValueAmount
}

func lookupBarCloseForDate(bars []DailyBarRecord, date string) (float64, float64, bool) {
	if len(bars) == 0 {
		return 0, 0, false
	}
	idx := sort.Search(len(bars), func(i int) bool { return bars[i].Date > date })
	if idx == 0 {
		return 0, 0, false
	}
	closePrice := bars[idx-1].Close
	prevClose := closePrice
	if idx-2 >= 0 {
		prevClose = bars[idx-2].Close
		if bars[idx-1].Date == date {
			prevClose = bars[idx-2].Close
		}
	}
	return closePrice, prevClose, true
}

func lookupLiveBarCloseOnOrBefore(bars []live.DailyBar, date string) float64 {
	if len(bars) == 0 {
		return 0
	}
	idx := sort.Search(len(bars), func(i int) bool { return bars[i].Date > date })
	if idx == 0 {
		return 0
	}
	return bars[idx-1].Close
}

func lookupFirstBarCloseOnOrAfterDate(bars []DailyBarRecord, date string) (float64, bool) {
	if len(bars) == 0 {
		return 0, false
	}
	idx := sort.Search(len(bars), func(i int) bool { return bars[i].Date >= date })
	if idx >= len(bars) {
		return 0, false
	}
	if bars[idx].Close <= 0 {
		return 0, false
	}
	return bars[idx].Close, true
}

func benchmarkCodeForScope(scope string) string {
	if scope == PortfolioScopeHK {
		return "HSI"
	}
	return "SHCI"
}

func benchmarkNameForCode(code string) string {
	if strings.EqualFold(code, "HSI") {
		return "恒生指数"
	}
	if strings.EqualFold(code, "000300.SH") {
		return "沪深300"
	}
	return "上证指数"
}

func resolveBenchmarkCode(exchange, existing string) string {
	existing = strings.TrimSpace(existing)
	if existing != "" {
		if strings.EqualFold(existing, "000300.SH") {
			return "SHCI"
		}
		return strings.ToUpper(existing)
	}
	if exchange == "HKEX" {
		return "HSI"
	}
	return "SHCI"
}

func normalizeAttributionSymbol(input string) (string, string) {
	raw := strings.ToUpper(strings.TrimSpace(input))
	if raw == "" {
		return "", ""
	}
	if strings.HasPrefix(raw, "SH") && len(raw) > 2 {
		raw = raw[2:] + ".SH"
	}
	if strings.HasPrefix(raw, "SZ") && len(raw) > 2 {
		raw = raw[2:] + ".SZ"
	}
	if strings.HasPrefix(raw, "HK") && len(raw) > 2 && !strings.HasSuffix(raw, ".HK") {
		raw = raw[2:] + ".HK"
	}
	normalized, exchange, err := live.NormalizeSymbol(raw)
	if err != nil {
		return "", ""
	}
	return normalized, exchange
}

func historyCodeFromSymbol(symbol string) string {
	upper := strings.ToUpper(strings.TrimSpace(symbol))
	switch {
	case strings.HasSuffix(upper, ".HK"):
		return strings.TrimSuffix(upper, ".HK")
	case strings.HasSuffix(upper, ".SH"):
		return strings.TrimSuffix(upper, ".SH")
	case strings.HasSuffix(upper, ".SZ"):
		return strings.TrimSuffix(upper, ".SZ")
	default:
		return upper
	}
}

func scopeListForAttribution(scope string, symbols []string) []string {
	if scope == PortfolioScopeAShare || scope == PortfolioScopeHK {
		return []string{scope}
	}
	seen := map[string]struct{}{}
	result := make([]string, 0, 2)
	for _, symbol := range symbols {
		s := exchangeToScope(live.ExchangeFromSymbol(symbol))
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		result = append(result, s)
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i] < result[j] })
	if len(result) == 2 {
		return []string{PortfolioScopeAShare, PortfolioScopeHK}
	}
	return result
}

func sortedStringKeys[K ~string, V any](m map[K]V) []string {
	result := make([]string, 0, len(m))
	for key := range m {
		result = append(result, string(key))
	}
	sort.Strings(result)
	return result
}

func cloneAttributionStates(src map[string]*attributionPositionState) map[string]*attributionPositionState {
	result := make(map[string]*attributionPositionState, len(src))
	for key, item := range src {
		if item == nil {
			continue
		}
		copyItem := *item
		result[key] = &copyItem
	}
	return result
}

func shiftDateString(date string, days int) string {
	t, err := time.ParseInLocation("2006-01-02", date, shanghaiLocation())
	if err != nil {
		return date
	}
	return t.AddDate(0, 0, days).Format("2006-01-02")
}

func calcLookbackDays(startDate, endDate string) int {
	start, err1 := time.ParseInLocation("2006-01-02", startDate, shanghaiLocation())
	end, err2 := time.ParseInLocation("2006-01-02", endDate, shanghaiLocation())
	if err1 != nil || err2 != nil || end.Before(start) {
		return 30
	}
	return int(end.Sub(start).Hours()/24) + 1
}

func isDateString(input string) bool {
	_, err := time.ParseInLocation("2006-01-02", input, shanghaiLocation())
	return err == nil
}

func normalizeSectorName(input string) string {
	name := strings.TrimSpace(input)
	name = strings.TrimSuffix(name, "Ⅰ")
	name = strings.TrimSuffix(name, "Ⅱ")
	name = strings.TrimSuffix(name, "Ⅲ")
	name = strings.TrimSpace(name)
	return name
}

func isUnclassifiedSector(input string) bool {
	name := strings.TrimSpace(input)
	return name == "" || name == "未分类" || name == "待分类"
}

func toneForValue(value float64) string {
	if value > 0 {
		return "positive"
	}
	if value < 0 {
		return "negative"
	}
	return "neutral"
}

func valueToneType(value float64) string {
	if value > 0 {
		return "positive"
	}
	if value < 0 {
		return "negative"
	}
	return "base"
}

func overallTone(dataset *attributionDataset) string {
	if dataset == nil {
		return "neutral"
	}
	total := 0.0
	for _, scope := range dataset.Scopes {
		if agg := dataset.ScopeAggregates[scope]; agg != nil {
			total += agg.TotalPnlAmount
		}
	}
	return toneForValue(total)
}

func ratioPtr(value float64) *float64 {
	v := round4(value)
	return &v
}

func safeRatio(numerator, denominator float64) float64 {
	if denominator == 0 {
		return 0
	}
	return numerator / denominator
}

func round2(value float64) float64 {
	return math.Round(value*100) / 100
}

func round4(value float64) float64 {
	return math.Round(value*10000) / 10000
}

func coalesceString(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func formatCurrencyValue(value float64, symbol string) string {
	prefix := ""
	if value > 0 {
		prefix = "+"
	}
	return fmt.Sprintf("%s%s%.2f", prefix, symbol, value)
}
