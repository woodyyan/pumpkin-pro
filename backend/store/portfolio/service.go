package portfolio

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/woodyyan/pumpkin-pro/backend/store/live"
)

const portfolioHistoryPreviewLimit = 5

type portfolioMarketSnapshot struct {
	Symbol         string
	Name           string
	Exchange       string
	CurrencyCode   string
	CurrencySymbol string
	LastPrice      float64
	PrevClosePrice float64
}

type portfolioSnapshotProvider interface {
	FetchDetailedSnapshots(ctx context.Context, symbols []string) (map[string]portfolioMarketSnapshot, error)
}

type liveSnapshotProvider struct {
	client *live.MarketClient
}

func newLiveSnapshotProvider() *liveSnapshotProvider {
	return &liveSnapshotProvider{client: live.NewMarketClient()}
}

func (p *liveSnapshotProvider) FetchDetailedSnapshots(ctx context.Context, symbols []string) (map[string]portfolioMarketSnapshot, error) {
	items, err := p.client.FetchDetailedSymbolSnapshots(ctx, symbols)
	if err != nil {
		return nil, err
	}
	result := make(map[string]portfolioMarketSnapshot, len(items))
	for _, item := range items {
		result[item.Symbol] = portfolioMarketSnapshot{
			Symbol:         item.Symbol,
			Name:           item.Name,
			Exchange:       item.Exchange,
			CurrencyCode:   item.CurrencyCode,
			CurrencySymbol: item.CurrencySymbol,
			LastPrice:      item.LastPrice,
			PrevClosePrice: item.PrevClosePrice,
		}
	}
	return result, nil
}

type Service struct {
	repo             *Repository
	snapshotProvider portfolioSnapshotProvider
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo, snapshotProvider: newLiveSnapshotProvider()}
}

func (s *Service) ListByUser(ctx context.Context, userID string) ([]PortfolioItem, error) {
	records, err := s.repo.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	items := make([]PortfolioItem, 0, len(records))
	for _, r := range records {
		items = append(items, r.toItem())
	}
	return items, nil
}

func (s *Service) GetBySymbol(ctx context.Context, userID, symbol string) (*PortfolioItem, error) {
	record, err := s.repo.GetBySymbol(ctx, userID, normalizePortfolioSymbol(symbol))
	if err != nil {
		return nil, err
	}
	item := record.toItem()
	return &item, nil
}

func (s *Service) GetDetailBySymbol(ctx context.Context, userID, symbol string) (*PortfolioDetail, error) {
	symbol = normalizePortfolioSymbol(symbol)
	if symbol == "" {
		return nil, fmt.Errorf("symbol is required")
	}
	if err := s.EnsureInitEventFromSnapshot(ctx, userID, symbol); err != nil {
		return nil, err
	}

	detail := &PortfolioDetail{HistoryPreview: []PortfolioEventItem{}}
	record, err := s.repo.GetBySymbol(ctx, userID, symbol)
	if err == nil {
		item := record.toItem()
		detail.Item = &item
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	events, err := s.repo.ListEventsBySymbol(ctx, userID, symbol, portfolioHistoryPreviewLimit)
	if err != nil {
		return nil, err
	}
	if len(events) > 0 {
		detail.HistoryPreview = make([]PortfolioEventItem, 0, len(events))
		for _, event := range events {
			detail.HistoryPreview = append(detail.HistoryPreview, event.toItem())
		}
	}
	return detail, nil
}

func (s *Service) CreateEvent(ctx context.Context, userID, symbol string, input CreatePortfolioEventInput) (*PortfolioItem, *PortfolioEventItem, error) {
	symbol = normalizePortfolioSymbol(symbol)
	if symbol == "" {
		return nil, nil, fmt.Errorf("symbol is required")
	}
	input.EventType = strings.TrimSpace(input.EventType)
	input.Note = strings.TrimSpace(input.Note)
	tradeDate, effectiveAt, err := normalizeTradeDate(input.TradeDate, time.Now().UTC())
	if err != nil {
		return nil, nil, err
	}
	input.TradeDate = tradeDate

	var savedItem *PortfolioItem
	var savedEvent *PortfolioEventItem
	now := time.Now().UTC()

	err = s.repo.InTx(ctx, func(txRepo *Repository) error {
		if err := s.ensureInitEventFromSnapshotTx(ctx, txRepo, userID, symbol, now); err != nil {
			return err
		}

		currentRecord, err := txRepo.GetBySymbol(ctx, userID, symbol)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		if errors.Is(err, ErrNotFound) {
			currentRecord = nil
		}

		current := derivePositionFromRecord(currentRecord)
		computation, err := computePortfolioEvent(current, input)
		if err != nil {
			return err
		}

		eventRecord := &PortfolioEventRecord{
			ID:                 uuid.New().String(),
			UserID:             userID,
			Symbol:             symbol,
			EventType:          input.EventType,
			TradeDate:          input.TradeDate,
			EffectiveAt:        effectiveAt,
			Quantity:           input.Quantity,
			Price:              input.Price,
			FeeAmount:          input.FeeAmount,
			ManualAvgCostPrice: input.ManualAvgCostPrice,
			Note:               input.Note,
			Source:             EventSourceManual,
			BeforeShares:       computation.Before.Shares,
			BeforeAvgCostPrice: computation.Before.AvgCostPrice,
			BeforeTotalCost:    computation.Before.TotalCostAmount,
			AfterShares:        computation.After.Shares,
			AfterAvgCostPrice:  computation.After.AvgCostPrice,
			AfterTotalCost:     computation.After.TotalCostAmount,
			RealizedPnlAmount:  computation.RealizedPnlAmount,
			RealizedPnlPct:     computation.RealizedPnlPct,
			CreatedAt:          now,
			UpdatedAt:          now,
		}
		if err := txRepo.CreateEvent(ctx, eventRecord); err != nil {
			return err
		}

		summaryRecord := buildPortfolioSummaryRecord(currentRecord, userID, symbol, computation.After, input.Note, eventRecord.ID, effectiveAt, now)
		if err := txRepo.Upsert(ctx, summaryRecord); err != nil {
			return err
		}

		summaryItem := summaryRecord.toItem()
		eventItem := eventRecord.toItem()
		savedItem = &summaryItem
		savedEvent = &eventItem
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return savedItem, savedEvent, nil
}

func (s *Service) ListEvents(ctx context.Context, userID, symbol string, limit int) ([]PortfolioEventItem, error) {
	symbol = normalizePortfolioSymbol(symbol)
	if symbol == "" {
		return nil, fmt.Errorf("symbol is required")
	}
	if err := s.EnsureInitEventFromSnapshot(ctx, userID, symbol); err != nil {
		return nil, err
	}
	records, err := s.repo.ListEventsBySymbol(ctx, userID, symbol, limit)
	if err != nil {
		return nil, err
	}
	items := make([]PortfolioEventItem, 0, len(records))
	for _, record := range records {
		items = append(items, record.toItem())
	}
	return items, nil
}

func (s *Service) UndoLatestEvent(ctx context.Context, userID, symbol, eventID string) (*UndoPortfolioEventResult, error) {
	symbol = normalizePortfolioSymbol(symbol)
	if symbol == "" {
		return nil, fmt.Errorf("symbol is required")
	}
	eventID = strings.TrimSpace(eventID)
	if eventID == "" {
		return nil, fmt.Errorf("event id is required")
	}
	now := time.Now().UTC()
	var result *UndoPortfolioEventResult

	err := s.repo.InTx(ctx, func(txRepo *Repository) error {
		if err := s.ensureInitEventFromSnapshotTx(ctx, txRepo, userID, symbol, now); err != nil {
			return err
		}

		target, err := txRepo.FindEventByID(ctx, userID, eventID)
		if err != nil {
			return err
		}
		latest, err := txRepo.GetLatestActiveEventBySymbol(ctx, userID, symbol)
		if err != nil {
			return err
		}
		if latest.ID != target.ID {
			return fmt.Errorf("仅支持撤销最后一条持仓变动记录")
		}
		if err := txRepo.VoidEvent(ctx, userID, eventID, uuid.New().String()); err != nil {
			return err
		}

		activeEvents, err := txRepo.ListAllActiveEventsAsc(ctx, userID, symbol)
		if err != nil {
			return err
		}
		position, err := rebuildPositionFromEvents(activeEvents)
		if err != nil {
			return err
		}
		currentRecord, err := txRepo.GetBySymbol(ctx, userID, symbol)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		if errors.Is(err, ErrNotFound) {
			currentRecord = nil
		}
		var latestActive *PortfolioEventRecord
		if len(activeEvents) > 0 {
			latestActive = &activeEvents[len(activeEvents)-1]
		}
		summaryRecord := buildPortfolioSummaryRecordFromState(currentRecord, userID, symbol, position, latestActive, now)
		if err := txRepo.Upsert(ctx, summaryRecord); err != nil {
			return err
		}
		summaryItem := summaryRecord.toItem()
		result = &UndoPortfolioEventResult{
			Item:          &summaryItem,
			UndoneEventID: eventID,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) Upsert(ctx context.Context, userID, symbol string, input UpsertPortfolioInput) (*PortfolioItem, error) {
	symbol = normalizePortfolioSymbol(symbol)
	if symbol == "" {
		return nil, fmt.Errorf("symbol is required")
	}
	if input.Shares < 0 {
		return nil, fmt.Errorf("shares must be >= 0")
	}
	if input.AvgCostPrice < 0 {
		return nil, fmt.Errorf("avg_cost_price must be >= 0")
	}

	now := time.Now().UTC()
	tradeDate := strings.TrimSpace(input.BuyDate)
	var lastTradeAt *time.Time
	if tradeDate != "" {
		if _, effectiveAt, err := normalizeTradeDate(tradeDate, now); err == nil {
			lastTradeAt = &effectiveAt
		}
	}
	record := &PortfolioRecord{
		ID:              uuid.New().String(),
		UserID:          userID,
		Symbol:          symbol,
		Shares:          input.Shares,
		AvgCostPrice:    input.AvgCostPrice,
		TotalCostAmount: input.Shares * input.AvgCostPrice,
		BuyDate:         tradeDate,
		Note:            strings.TrimSpace(input.Note),
		CostMethod:      CostMethodWeightedAvg,
		CostSource:      CostSourceManual,
		LastTradeAt:     lastTradeAt,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := s.repo.Upsert(ctx, record); err != nil {
		return nil, err
	}
	if err := s.EnsureInitEventFromSnapshot(ctx, userID, symbol); err != nil {
		return nil, err
	}

	saved, err := s.repo.GetBySymbol(ctx, userID, symbol)
	if err != nil {
		return nil, err
	}
	item := saved.toItem()
	return &item, nil
}

func (s *Service) Delete(ctx context.Context, userID, symbol string) error {
	return s.repo.Delete(ctx, userID, normalizePortfolioSymbol(symbol))
}

func (s *Service) EnsureInitEventFromSnapshot(ctx context.Context, userID, symbol string) error {
	symbol = normalizePortfolioSymbol(symbol)
	if symbol == "" {
		return fmt.Errorf("symbol is required")
	}
	return s.repo.InTx(ctx, func(txRepo *Repository) error {
		return s.ensureInitEventFromSnapshotTx(ctx, txRepo, userID, symbol, time.Now().UTC())
	})
}

func (s *Service) ensureInitEventFromSnapshotTx(ctx context.Context, txRepo *Repository, userID, symbol string, now time.Time) error {
	record, err := txRepo.GetBySymbol(ctx, userID, symbol)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return err
	}
	hasEvents, err := txRepo.HasActiveEventsBySymbol(ctx, userID, symbol)
	if err != nil {
		return err
	}
	if hasEvents {
		return nil
	}
	tradeDate, effectiveAt := buildInitTradeDate(record, now)
	position := derivePositionFromRecord(record)
	eventRecord := &PortfolioEventRecord{
		ID:                 uuid.New().String(),
		UserID:             userID,
		Symbol:             symbol,
		EventType:          EventTypeInit,
		TradeDate:          tradeDate,
		EffectiveAt:        effectiveAt,
		Quantity:           position.Shares,
		Price:              position.AvgCostPrice,
		ManualAvgCostPrice: position.AvgCostPrice,
		Note:               buildInitEventNote(record.Note),
		Source:             EventSourceMigration,
		BeforeShares:       0,
		BeforeAvgCostPrice: 0,
		BeforeTotalCost:    0,
		AfterShares:        position.Shares,
		AfterAvgCostPrice:  position.AvgCostPrice,
		AfterTotalCost:     position.TotalCostAmount,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := txRepo.CreateEvent(ctx, eventRecord); err != nil {
		return err
	}
	record.TotalCostAmount = position.TotalCostAmount
	record.CostMethod = CostMethodWeightedAvg
	record.CostSource = CostSourceManual
	record.LastTradeAt = &effectiveAt
	record.LastEventID = eventRecord.ID
	record.UpdatedAt = now
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	return txRepo.Upsert(ctx, record)
}

// ── Investment Profile ──

func (s *Service) GetInvestmentProfile(ctx context.Context, userID string) (*InvestmentProfile, error) {
	record, err := s.repo.GetInvestmentProfile(ctx, userID)
	if err != nil {
		return nil, err
	}
	profile := record.toProfile()
	return &profile, nil
}

func (s *Service) UpsertInvestmentProfile(ctx context.Context, userID string, input UpsertInvestmentProfileInput) (*InvestmentProfile, error) {
	if input.TotalCapital < 0 {
		return nil, fmt.Errorf("total_capital must be >= 0")
	}
	if input.MaxDrawdownPct < 0 || input.MaxDrawdownPct > 100 {
		return nil, fmt.Errorf("max_drawdown_pct must be between 0 and 100")
	}

	now := time.Now().UTC()
	record := &InvestmentProfileRecord{
		UserID:            userID,
		TotalCapital:      input.TotalCapital,
		RiskPreference:    strings.TrimSpace(input.RiskPreference),
		InvestmentGoal:    strings.TrimSpace(input.InvestmentGoal),
		InvestmentHorizon: strings.TrimSpace(input.InvestmentHorizon),
		MaxDrawdownPct:    input.MaxDrawdownPct,
		ExperienceLevel:   strings.TrimSpace(input.ExperienceLevel),
		Note:              strings.TrimSpace(input.Note),
		UpdatedAt:         now,
	}

	if err := s.repo.UpsertInvestmentProfile(ctx, record); err != nil {
		return nil, err
	}

	saved, err := s.repo.GetInvestmentProfile(ctx, userID)
	if err != nil {
		return nil, err
	}
	profile := saved.toProfile()
	return &profile, nil
}

type portfolioEventStats struct {
	RealizedPnlAmount float64
	BuyCount          int
	SellCount         int
	AdjustCount       int
	FirstTradeDate    string
	LastTradeAt       time.Time
	LastEventType     string
	LastEventID       string
}

type portfolioBaseData struct {
	Profile      *InvestmentProfile
	EventStats   map[string]*portfolioEventStats
	NameBySymbol map[string]string
	Positions    []PortfolioPositionItem
	RecentEvents []PortfolioRecentEventItem
}

func (s *Service) GetDashboard(ctx context.Context, userID string, query PortfolioDashboardQuery) (*PortfolioDashboardPayload, error) {
	query, err := normalizeDashboardQuery(query)
	if err != nil {
		return nil, err
	}
	base, err := s.loadPortfolioBaseData(ctx, userID)
	if err != nil {
		return nil, err
	}
	if err := s.persistDailySnapshots(ctx, userID, base.Positions); err != nil {
		return nil, err
	}
	positions := applyPositionFilters(base.Positions, query.Scope, query.Keyword, query.PnlFilter)
	sortPortfolioPositions(positions, query.SortBy, query.SortOrder)
	recentEvents := filterRecentEvents(base.RecentEvents, query.Scope, query.Keyword)
	if len(recentEvents) > 10 {
		recentEvents = recentEvents[:10]
	}
	allocation := buildAllocationItems(positions, 10)
	curve, err := s.buildEquityCurvePayload(ctx, userID, PortfolioCurveQuery{Scope: query.Scope, Range: query.CurveRange})
	if err != nil {
		return nil, err
	}
	summary := buildDashboardSummary(positions, base.EventStats, base.NameBySymbol, base.Profile, query.Scope, query.Keyword)
	return &PortfolioDashboardPayload{
		Summary:             summary,
		Positions:           positions,
		RecentEventsPreview: recentEvents,
		AllocationPreview:   allocation,
		EquityCurvePreview:  *curve,
		Filters: PortfolioDashboardFilters{
			Scope:      query.Scope,
			SortBy:     query.SortBy,
			SortOrder:  query.SortOrder,
			PnlFilter:  query.PnlFilter,
			Keyword:    query.Keyword,
			CurveRange: query.CurveRange,
		},
		AIContextMeta: PortfolioAIContextMeta{
			Ready:                len(base.Positions) > 0 || len(base.RecentEvents) > 0 || base.Profile != nil,
			PositionContextCount: len(positions),
			RecentEventCount:     len(recentEvents),
			ProfileReady:         base.Profile != nil,
		},
	}, nil
}

func (s *Service) GetEquityCurve(ctx context.Context, userID string, query PortfolioCurveQuery) (*PortfolioCurvePayload, error) {
	query.Scope, _ = normalizePortfolioScope(query.Scope)
	query.Range = normalizeCurveRange(query.Range)
	base, err := s.loadPortfolioBaseData(ctx, userID)
	if err != nil {
		return nil, err
	}
	if err := s.persistDailySnapshots(ctx, userID, base.Positions); err != nil {
		return nil, err
	}
	return s.buildEquityCurvePayload(ctx, userID, query)
}

func (s *Service) ListRecentEventsByUser(ctx context.Context, userID string, query PortfolioRecentEventsQuery) ([]PortfolioRecentEventItem, error) {
	scope, err := normalizePortfolioScope(query.Scope)
	if err != nil {
		return nil, err
	}
	base, err := s.loadPortfolioBaseData(ctx, userID)
	if err != nil {
		return nil, err
	}
	items := filterRecentEvents(base.RecentEvents, scope, strings.TrimSpace(query.Keyword))
	limit := query.Limit
	if limit <= 0 {
		limit = 20
	}
	offset := query.Offset
	if offset < 0 {
		offset = 0
	}
	if offset >= len(items) {
		return []PortfolioRecentEventItem{}, nil
	}
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	return items[offset:end], nil
}

func (s *Service) GetAllocation(ctx context.Context, userID string, query PortfolioAllocationQuery) ([]PortfolioAllocationItem, error) {
	scope, err := normalizePortfolioScope(query.Scope)
	if err != nil {
		return nil, err
	}
	base, err := s.loadPortfolioBaseData(ctx, userID)
	if err != nil {
		return nil, err
	}
	positions := applyPositionFilters(base.Positions, scope, query.Keyword, "all")
	return buildAllocationItems(positions, query.Limit), nil
}

func (s *Service) GetAIContext(ctx context.Context, userID string, scope string) (*PortfolioAIContextPayload, error) {
	normalizedScope, err := normalizePortfolioScope(scope)
	if err != nil {
		return nil, err
	}
	dashboard, err := s.GetDashboard(ctx, userID, PortfolioDashboardQuery{Scope: normalizedScope, SortBy: "market_value", SortOrder: "desc", PnlFilter: "all", CurveRange: "30D"})
	if err != nil {
		return nil, err
	}
	base, err := s.loadPortfolioBaseData(ctx, userID)
	if err != nil {
		return nil, err
	}
	recentEvents := filterRecentEvents(base.RecentEvents, normalizedScope, "")
	if len(recentEvents) > 20 {
		recentEvents = recentEvents[:20]
	}
	flags := PortfolioAIContextRiskFlags{}
	if dashboard.Summary.MaxPositionWeightRatio >= 0.35 {
		flags.PositionTooConcentrated = true
	}
	for _, block := range dashboard.Summary.AmountsByMarket {
		if block.MaxPositionWeightRatio >= 0.35 {
			flags.PositionTooConcentrated = true
		}
	}
	flags.TooManyLosingPositions = dashboard.Summary.LossPositionCount > 0 && dashboard.Summary.LossPositionCount >= dashboard.Summary.ProfitPositionCount
	flags.AveragingDownRisk = hasAveragingDownRisk(dashboard.Positions)
	flags.ManualCostAdjustFrequent = hasFrequentManualAdjust(dashboard.Positions)
	flags.RecentOvertrading = countRecentTrades(recentEvents, 7) >= 5
	flags.CapitalUsageTooHigh = dashboard.Summary.CapitalUsageRatio != nil && *dashboard.Summary.CapitalUsageRatio >= 0.85
	marketBreakdown := dashboard.Summary.AmountsByMarket
	if len(marketBreakdown) == 0 && dashboard.Summary.Amounts != nil {
		marketBreakdown = buildMarketBreakdownFromSingleSummary(dashboard.Summary, dashboard.Positions)
	}
	return &PortfolioAIContextPayload{
		Summary:              dashboard.Summary,
		Positions:            dashboard.Positions,
		RecentEvents:         recentEvents,
		Profile:              base.Profile,
		RiskFlags:            flags,
		MarketScopeBreakdown: marketBreakdown,
	}, nil
}

func (s *Service) loadPortfolioBaseData(ctx context.Context, userID string) (*portfolioBaseData, error) {
	records, err := s.repo.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	events, err := s.repo.ListActiveEventsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	var profile *InvestmentProfile
	if got, err := s.repo.GetInvestmentProfile(ctx, userID); err == nil {
		converted := got.toProfile()
		profile = &converted
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	stats := buildEventStats(events)
	symbols := uniquePortfolioSymbols(records, events)
	snapshotMap := map[string]portfolioMarketSnapshot{}
	if s.snapshotProvider != nil {
		if snapshots, err := s.snapshotProvider.FetchDetailedSnapshots(ctx, symbols); err == nil {
			snapshotMap = snapshots
		}
	}
	nameBySymbol := map[string]string{}
	positions := buildPortfolioPositionItems(records, stats, snapshotMap)
	for _, item := range positions {
		nameBySymbol[item.Symbol] = item.Name
	}
	recentEvents := buildRecentEventItems(events, stats, snapshotMap)
	for _, item := range recentEvents {
		if strings.TrimSpace(item.Name) != "" {
			nameBySymbol[item.Symbol] = item.Name
		}
	}
	return &portfolioBaseData{
		Profile:      profile,
		EventStats:   stats,
		NameBySymbol: nameBySymbol,
		Positions:    positions,
		RecentEvents: recentEvents,
	}, nil
}

func (s *Service) persistDailySnapshots(ctx context.Context, userID string, positions []PortfolioPositionItem) error {
	today := time.Now().In(shanghaiLocation()).Format("2006-01-02")
	blocks := map[string]*PortfolioMarketAmountBlock{}
	for _, item := range positions {
		scope := exchangeToScope(item.Exchange)
		if scope == "" {
			continue
		}
		block := blocks[scope]
		if block == nil {
			block = &PortfolioMarketAmountBlock{
				Scope:          scope,
				ScopeLabel:     scopeLabel(scope),
				CurrencyCode:   item.CurrencyCode,
				CurrencySymbol: item.CurrencySymbol,
			}
			blocks[scope] = block
		}
		block.MarketValueAmount += item.MarketValueAmount
		block.TotalCostAmount += item.TotalCostAmount
		block.UnrealizedPnlAmount += item.UnrealizedPnlAmount
		block.RealizedPnlAmount += item.RealizedPnlAmount
		block.TodayPnlAmount += item.TodayPnlAmount
		block.PositionCount++
		if item.MarketValueAmount > block.MaxPositionWeightRatio {
			block.MaxPositionWeightRatio = item.MarketValueAmount
		}
	}
	for _, block := range blocks {
		if block.MarketValueAmount > 0 && block.MaxPositionWeightRatio > 0 {
			block.MaxPositionWeightRatio = block.MaxPositionWeightRatio / block.MarketValueAmount
		}
		block.TotalPnlAmount = block.RealizedPnlAmount + block.UnrealizedPnlAmount
		record := &PortfolioDailySnapshotRecord{
			ID:                  uuid.New().String(),
			UserID:              userID,
			Scope:               block.Scope,
			SnapshotDate:        today,
			CurrencyCode:        block.CurrencyCode,
			MarketValueAmount:   block.MarketValueAmount,
			TotalCostAmount:     block.TotalCostAmount,
			UnrealizedPnlAmount: block.UnrealizedPnlAmount,
			RealizedPnlAmount:   block.RealizedPnlAmount,
			TotalPnlAmount:      block.TotalPnlAmount,
			TodayPnlAmount:      block.TodayPnlAmount,
			PositionCount:       block.PositionCount,
			CreatedAt:           time.Now().UTC(),
			UpdatedAt:           time.Now().UTC(),
		}
		if err := s.repo.UpsertDailySnapshot(ctx, record); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) buildEquityCurvePayload(ctx context.Context, userID string, query PortfolioCurveQuery) (*PortfolioCurvePayload, error) {
	scope, err := normalizePortfolioScope(query.Scope)
	if err != nil {
		return nil, err
	}
	rangeLabel := normalizeCurveRange(query.Range)
	scopes := curveScopesForQuery(scope)
	fromDate := curveFromDate(rangeLabel)
	records, err := s.repo.ListDailySnapshots(ctx, userID, scopes, fromDate)
	if err != nil {
		return nil, err
	}
	seriesMap := map[string][]PortfolioCurvePoint{}
	currencyByScope := map[string]string{}
	for _, record := range records {
		seriesMap[record.Scope] = append(seriesMap[record.Scope], PortfolioCurvePoint{
			Date:                record.SnapshotDate,
			Scope:               record.Scope,
			CurrencyCode:        record.CurrencyCode,
			MarketValueAmount:   record.MarketValueAmount,
			TotalCostAmount:     record.TotalCostAmount,
			UnrealizedPnlAmount: record.UnrealizedPnlAmount,
			RealizedPnlAmount:   record.RealizedPnlAmount,
			TotalPnlAmount:      record.TotalPnlAmount,
			TodayPnlAmount:      record.TodayPnlAmount,
			PositionCount:       record.PositionCount,
		})
		currencyByScope[record.Scope] = record.CurrencyCode
	}
	orderedScopes := orderedScopes(seriesMap)
	series := make([]PortfolioCurveSeries, 0, len(orderedScopes))
	for _, itemScope := range orderedScopes {
		series = append(series, PortfolioCurveSeries{
			Scope:        itemScope,
			ScopeLabel:   scopeLabel(itemScope),
			CurrencyCode: currencyByScope[itemScope],
			Points:       seriesMap[itemScope],
		})
	}
	return &PortfolioCurvePayload{
		Scope:         scope,
		MixedCurrency: len(series) > 1,
		Series:        series,
	}, nil
}

func buildEventStats(events []PortfolioEventRecord) map[string]*portfolioEventStats {
	stats := make(map[string]*portfolioEventStats)
	for _, event := range events {
		item := stats[event.Symbol]
		if item == nil {
			item = &portfolioEventStats{}
			stats[event.Symbol] = item
		}
		item.RealizedPnlAmount += event.RealizedPnlAmount
		switch event.EventType {
		case EventTypeBuy:
			item.BuyCount++
		case EventTypeSell:
			item.SellCount++
		case EventTypeAdjustAvgCost:
			item.AdjustCount++
		}
		if item.FirstTradeDate == "" || (event.TradeDate != "" && event.TradeDate < item.FirstTradeDate) {
			item.FirstTradeDate = event.TradeDate
		}
		if item.LastTradeAt.IsZero() || event.EffectiveAt.After(item.LastTradeAt) {
			item.LastTradeAt = event.EffectiveAt
			item.LastEventType = event.EventType
			item.LastEventID = event.ID
		}
	}
	return stats
}

func uniquePortfolioSymbols(records []PortfolioRecord, events []PortfolioEventRecord) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(records)+len(events))
	for _, record := range records {
		if _, ok := seen[record.Symbol]; ok {
			continue
		}
		seen[record.Symbol] = struct{}{}
		result = append(result, record.Symbol)
	}
	for _, event := range events {
		if _, ok := seen[event.Symbol]; ok {
			continue
		}
		seen[event.Symbol] = struct{}{}
		result = append(result, event.Symbol)
	}
	return result
}

func buildPortfolioPositionItems(records []PortfolioRecord, stats map[string]*portfolioEventStats, snapshotMap map[string]portfolioMarketSnapshot) []PortfolioPositionItem {
	positions := make([]PortfolioPositionItem, 0, len(records))
	marketTotals := map[string]float64{}
	for _, record := range records {
		exchange := live.ExchangeFromSymbol(record.Symbol)
		snapshot := snapshotMap[record.Symbol]
		if snapshot.Exchange != "" {
			exchange = snapshot.Exchange
		}
		currencyCode, currencySymbol := resolveCurrency(exchange)
		if snapshot.CurrencyCode != "" {
			currencyCode = snapshot.CurrencyCode
		}
		if snapshot.CurrencySymbol != "" {
			currencySymbol = snapshot.CurrencySymbol
		}
		name := strings.TrimSpace(snapshot.Name)
		if name == "" {
			name = record.Symbol
		}
		lastPrice := snapshot.LastPrice
		prevClose := snapshot.PrevClosePrice
		marketValue := record.Shares * lastPrice
		unrealized := marketValue - record.TotalCostAmount
		realized := 0.0
		buyCount := 0
		sellCount := 0
		adjustCount := 0
		firstTradeDate := record.BuyDate
		lastTradeAt := ""
		lastEventType := ""
		lastEventID := record.LastEventID
		if stat := stats[record.Symbol]; stat != nil {
			realized = stat.RealizedPnlAmount
			buyCount = stat.BuyCount
			sellCount = stat.SellCount
			adjustCount = stat.AdjustCount
			if strings.TrimSpace(firstTradeDate) == "" {
				firstTradeDate = stat.FirstTradeDate
			}
			if !stat.LastTradeAt.IsZero() {
				lastTradeAt = stat.LastTradeAt.UTC().Format(time.RFC3339)
			}
			lastEventType = stat.LastEventType
			if stat.LastEventID != "" {
				lastEventID = stat.LastEventID
			}
		}
		todayPnl := 0.0
		todayPnlPct := 0.0
		if prevClose > 0 && record.Shares > 0 {
			todayPnl = (lastPrice - prevClose) * record.Shares
			base := prevClose * record.Shares
			if base > 0 {
				todayPnlPct = todayPnl / base
			}
		}
		totalPnl := realized + unrealized
		unrealizedPct := 0.0
		totalPnlPct := 0.0
		if record.TotalCostAmount > 0 {
			unrealizedPct = unrealized / record.TotalCostAmount
			totalPnlPct = totalPnl / record.TotalCostAmount
		}
		positions = append(positions, PortfolioPositionItem{
			Symbol:              record.Symbol,
			Name:                name,
			Exchange:            exchange,
			ExchangeLabel:       exchangeLabel(exchange),
			CurrencyCode:        currencyCode,
			CurrencySymbol:      currencySymbol,
			Shares:              record.Shares,
			AvgCostPrice:        record.AvgCostPrice,
			TotalCostAmount:     record.TotalCostAmount,
			LastPrice:           lastPrice,
			PrevClosePrice:      prevClose,
			MarketValueAmount:   marketValue,
			UnrealizedPnlAmount: unrealized,
			UnrealizedPnlPct:    unrealizedPct,
			RealizedPnlAmount:   realized,
			TotalPnlAmount:      totalPnl,
			TotalPnlPct:         totalPnlPct,
			TodayPnlAmount:      todayPnl,
			TodayPnlPct:         todayPnlPct,
			BuyDate:             record.BuyDate,
			FirstTradeDate:      firstTradeDate,
			HoldingDays:         computeHoldingDays(record.BuyDate, firstTradeDate),
			LastTradeAt:         lastTradeAt,
			LastEventType:       lastEventType,
			BuyCount:            buyCount,
			SellCount:           sellCount,
			AdjustCount:         adjustCount,
			CostSource:          record.CostSource,
			Note:                record.Note,
			LastEventID:         lastEventID,
			DetailURL:           fmt.Sprintf("/live-trading/%s", record.Symbol),
			CanBuy:              true,
			CanSell:             record.Shares > 0,
			CanAdjustAvgCost:    record.Shares > 0,
		})
		marketTotals[exchange] += marketValue
	}
	for i := range positions {
		total := marketTotals[positions[i].Exchange]
		if total > 0 {
			positions[i].PositionWeightRatio = positions[i].MarketValueAmount / total
		}
	}
	return positions
}

func buildRecentEventItems(events []PortfolioEventRecord, stats map[string]*portfolioEventStats, snapshotMap map[string]portfolioMarketSnapshot) []PortfolioRecentEventItem {
	items := make([]PortfolioRecentEventItem, 0, len(events))
	for _, event := range events {
		snapshot := snapshotMap[event.Symbol]
		exchange := live.ExchangeFromSymbol(event.Symbol)
		if snapshot.Exchange != "" {
			exchange = snapshot.Exchange
		}
		currencyCode, currencySymbol := resolveCurrency(exchange)
		if snapshot.CurrencyCode != "" {
			currencyCode = snapshot.CurrencyCode
		}
		if snapshot.CurrencySymbol != "" {
			currencySymbol = snapshot.CurrencySymbol
		}
		name := strings.TrimSpace(snapshot.Name)
		if name == "" {
			name = event.Symbol
		}
		isUndoable := false
		if stat := stats[event.Symbol]; stat != nil && stat.LastEventID == event.ID {
			isUndoable = true
		}
		items = append(items, PortfolioRecentEventItem{
			ID:                 event.ID,
			Symbol:             event.Symbol,
			Name:               name,
			Exchange:           exchange,
			ExchangeLabel:      exchangeLabel(exchange),
			CurrencyCode:       currencyCode,
			CurrencySymbol:     currencySymbol,
			EventType:          event.EventType,
			TradeDate:          event.TradeDate,
			EffectiveAt:        event.EffectiveAt.UTC().Format(time.RFC3339),
			Quantity:           event.Quantity,
			Price:              event.Price,
			FeeAmount:          event.FeeAmount,
			RealizedPnlAmount:  event.RealizedPnlAmount,
			BeforeShares:       event.BeforeShares,
			AfterShares:        event.AfterShares,
			BeforeAvgCostPrice: event.BeforeAvgCostPrice,
			AfterAvgCostPrice:  event.AfterAvgCostPrice,
			Note:               event.Note,
			IsUndoable:         isUndoable,
		})
	}
	return items
}

func buildDashboardSummary(positions []PortfolioPositionItem, eventStats map[string]*portfolioEventStats, nameBySymbol map[string]string, profile *InvestmentProfile, scope, keyword string) PortfolioDashboardSummary {
	summary := PortfolioDashboardSummary{Scope: scope}
	if profile != nil && profile.TotalCapital > 0 {
		totalCapital := profile.TotalCapital
		summary.TotalCapitalAmount = &totalCapital
	}
	marketBlocks := map[string]*PortfolioMarketAmountBlock{}
	maxMarketValueByScope := map[string]float64{}
	for _, item := range positions {
		block := ensureMarketBlock(marketBlocks, exchangeToScope(item.Exchange), item.CurrencyCode, item.CurrencySymbol)
		block.MarketValueAmount += item.MarketValueAmount
		block.TotalCostAmount += item.TotalCostAmount
		block.UnrealizedPnlAmount += item.UnrealizedPnlAmount
		block.TodayPnlAmount += item.TodayPnlAmount
		block.PositionCount++
		summary.PositionCount++
		if item.TotalPnlAmount > 0 {
			summary.ProfitPositionCount++
		} else if item.TotalPnlAmount < 0 {
			summary.LossPositionCount++
		}
		if item.MarketValueAmount > maxMarketValueByScope[block.Scope] {
			maxMarketValueByScope[block.Scope] = item.MarketValueAmount
		}
		if latestText(summary.LatestTradeAt, item.LastTradeAt) != summary.LatestTradeAt {
			summary.LatestTradeAt = item.LastTradeAt
		}
	}
	for symbol, stat := range eventStats {
		exchange := live.ExchangeFromSymbol(symbol)
		if !scopeMatchesExchange(scope, exchange) || !matchesKeyword(symbol, nameBySymbol[symbol], keyword) {
			continue
		}
		cc, cs := resolveCurrency(exchange)
		block := ensureMarketBlock(marketBlocks, exchangeToScope(exchange), cc, cs)
		block.RealizedPnlAmount += stat.RealizedPnlAmount
		latest := ""
		if !stat.LastTradeAt.IsZero() {
			latest = stat.LastTradeAt.UTC().Format(time.RFC3339)
		}
		if latestText(summary.LatestTradeAt, latest) != summary.LatestTradeAt {
			summary.LatestTradeAt = latest
		}
	}
	blocks := make([]PortfolioMarketAmountBlock, 0, len(marketBlocks))
	for _, scopeKey := range []string{PortfolioScopeAShare, PortfolioScopeHK} {
		block := marketBlocks[scopeKey]
		if block == nil {
			continue
		}
		if block.MarketValueAmount > 0 && maxMarketValueByScope[scopeKey] > 0 {
			block.MaxPositionWeightRatio = maxMarketValueByScope[scopeKey] / block.MarketValueAmount
		}
		block.TotalPnlAmount = block.RealizedPnlAmount + block.UnrealizedPnlAmount
		if block.PositionCount == 0 && block.RealizedPnlAmount == 0 && block.MarketValueAmount == 0 && block.TotalCostAmount == 0 {
			continue
		}
		blocks = append(blocks, *block)
	}
	summary.MixedCurrency = len(blocks) > 1
	if len(blocks) == 1 {
		block := blocks[0]
		summary.Amounts = &PortfolioSummaryAmounts{
			CurrencyCode:        block.CurrencyCode,
			CurrencySymbol:      block.CurrencySymbol,
			MarketValueAmount:   block.MarketValueAmount,
			TotalCostAmount:     block.TotalCostAmount,
			UnrealizedPnlAmount: block.UnrealizedPnlAmount,
			RealizedPnlAmount:   block.RealizedPnlAmount,
			TotalPnlAmount:      block.TotalPnlAmount,
			TodayPnlAmount:      block.TodayPnlAmount,
		}
		summary.MaxPositionWeightRatio = block.MaxPositionWeightRatio
		if profile != nil && profile.TotalCapital > 0 && strings.EqualFold(block.CurrencyCode, "CNY") {
			ratio := block.MarketValueAmount / profile.TotalCapital
			summary.CapitalUsageRatio = &ratio
		}
	} else if len(blocks) > 1 {
		summary.AmountsByMarket = blocks
	}
	return summary
}

func buildAllocationItems(positions []PortfolioPositionItem, limit int) []PortfolioAllocationItem {
	if limit <= 0 {
		limit = 10
	}
	items := make([]PortfolioPositionItem, len(positions))
	copy(items, positions)
	sort.Slice(items, func(i, j int) bool {
		if items[i].MarketValueAmount == items[j].MarketValueAmount {
			return items[i].TotalPnlAmount > items[j].TotalPnlAmount
		}
		return items[i].MarketValueAmount > items[j].MarketValueAmount
	})
	if len(items) > limit {
		items = items[:limit]
	}
	result := make([]PortfolioAllocationItem, 0, len(items))
	for _, item := range items {
		result = append(result, PortfolioAllocationItem{
			Symbol:            item.Symbol,
			Name:              item.Name,
			Exchange:          item.Exchange,
			ExchangeLabel:     item.ExchangeLabel,
			CurrencyCode:      item.CurrencyCode,
			CurrencySymbol:    item.CurrencySymbol,
			MarketValueAmount: item.MarketValueAmount,
			WeightRatio:       item.PositionWeightRatio,
			TotalPnlAmount:    item.TotalPnlAmount,
		})
	}
	return result
}

func applyPositionFilters(items []PortfolioPositionItem, scope, keyword, pnlFilter string) []PortfolioPositionItem {
	result := make([]PortfolioPositionItem, 0, len(items))
	for _, item := range items {
		if !scopeMatchesExchange(scope, item.Exchange) {
			continue
		}
		if !matchesKeyword(item.Symbol, item.Name, keyword) {
			continue
		}
		switch pnlFilter {
		case "profit":
			if item.TotalPnlAmount <= 0 {
				continue
			}
		case "loss":
			if item.TotalPnlAmount >= 0 {
				continue
			}
		}
		result = append(result, item)
	}
	return result
}

func filterRecentEvents(items []PortfolioRecentEventItem, scope, keyword string) []PortfolioRecentEventItem {
	result := make([]PortfolioRecentEventItem, 0, len(items))
	for _, item := range items {
		if !scopeMatchesExchange(scope, item.Exchange) {
			continue
		}
		if !matchesKeyword(item.Symbol, item.Name, keyword) {
			continue
		}
		result = append(result, item)
	}
	return result
}

func sortPortfolioPositions(items []PortfolioPositionItem, sortBy, sortOrder string) {
	desc := sortOrder != "asc"
	sort.SliceStable(items, func(i, j int) bool {
		left := items[i]
		right := items[j]
		less := false
		switch sortBy {
		case "today_pnl":
			less = left.TodayPnlAmount < right.TodayPnlAmount
		case "total_pnl":
			less = left.TotalPnlAmount < right.TotalPnlAmount
		case "last_trade":
			less = latestText(left.LastTradeAt, right.LastTradeAt) == right.LastTradeAt
		case "holding_days":
			less = left.HoldingDays < right.HoldingDays
		case "unrealized_pnl":
			less = left.UnrealizedPnlAmount < right.UnrealizedPnlAmount
		default:
			less = left.MarketValueAmount < right.MarketValueAmount
		}
		if desc {
			return !less
		}
		return less
	})
}

func normalizeDashboardQuery(query PortfolioDashboardQuery) (PortfolioDashboardQuery, error) {
	scope, err := normalizePortfolioScope(query.Scope)
	if err != nil {
		return PortfolioDashboardQuery{}, err
	}
	sortBy := strings.TrimSpace(strings.ToLower(query.SortBy))
	switch sortBy {
	case "today_pnl", "total_pnl", "last_trade", "holding_days", "unrealized_pnl", "market_value":
	default:
		sortBy = "market_value"
	}
	sortOrder := strings.TrimSpace(strings.ToLower(query.SortOrder))
	if sortOrder != "asc" {
		sortOrder = "desc"
	}
	pnlFilter := strings.TrimSpace(strings.ToLower(query.PnlFilter))
	if pnlFilter != "profit" && pnlFilter != "loss" {
		pnlFilter = "all"
	}
	return PortfolioDashboardQuery{
		Scope:      scope,
		SortBy:     sortBy,
		SortOrder:  sortOrder,
		PnlFilter:  pnlFilter,
		Keyword:    strings.TrimSpace(query.Keyword),
		CurveRange: normalizeCurveRange(query.CurveRange),
	}, nil
}

func normalizePortfolioScope(raw string) (string, error) {
	scope := strings.ToUpper(strings.TrimSpace(raw))
	switch scope {
	case "", PortfolioScopeAll:
		return PortfolioScopeAll, nil
	case PortfolioScopeAShare, "SSE", "SZSE":
		return PortfolioScopeAShare, nil
	case PortfolioScopeHK:
		return PortfolioScopeHK, nil
	default:
		return "", fmt.Errorf("invalid portfolio scope: %s", raw)
	}
}

func normalizeCurveRange(raw string) string {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "7D", "30D", "90D", "ALL":
		return strings.ToUpper(strings.TrimSpace(raw))
	default:
		return "30D"
	}
}

func curveScopesForQuery(scope string) []string {
	if scope == PortfolioScopeHK {
		return []string{PortfolioScopeHK}
	}
	if scope == PortfolioScopeAShare {
		return []string{PortfolioScopeAShare}
	}
	return []string{PortfolioScopeAShare, PortfolioScopeHK}
}

func curveFromDate(rangeLabel string) string {
	if rangeLabel == "ALL" {
		return ""
	}
	days := 30
	if rangeLabel == "7D" {
		days = 7
	} else if rangeLabel == "90D" {
		days = 90
	}
	return time.Now().In(shanghaiLocation()).AddDate(0, 0, -(days - 1)).Format("2006-01-02")
}

func orderedScopes(seriesMap map[string][]PortfolioCurvePoint) []string {
	result := make([]string, 0, len(seriesMap))
	for _, scope := range []string{PortfolioScopeAShare, PortfolioScopeHK} {
		if len(seriesMap[scope]) > 0 {
			result = append(result, scope)
		}
	}
	return result
}

func scopeMatchesExchange(scope, exchange string) bool {
	switch scope {
	case PortfolioScopeHK:
		return exchange == "HKEX"
	case PortfolioScopeAShare:
		return exchange == "SSE" || exchange == "SZSE"
	default:
		return exchange == "HKEX" || exchange == "SSE" || exchange == "SZSE"
	}
}

func exchangeToScope(exchange string) string {
	if exchange == "HKEX" {
		return PortfolioScopeHK
	}
	if exchange == "SSE" || exchange == "SZSE" {
		return PortfolioScopeAShare
	}
	return ""
}

func scopeLabel(scope string) string {
	switch scope {
	case PortfolioScopeAShare:
		return "A股"
	case PortfolioScopeHK:
		return "港股"
	default:
		return "全部"
	}
}

func exchangeLabel(exchange string) string {
	switch exchange {
	case "HKEX":
		return "港股"
	case "SSE", "SZSE":
		return "A股"
	default:
		return exchange
	}
}

func resolveCurrency(exchange string) (string, string) {
	if exchange == "HKEX" {
		return "HKD", "HK$"
	}
	return "CNY", "¥"
}

func ensureMarketBlock(blocks map[string]*PortfolioMarketAmountBlock, scope string, currency ...string) *PortfolioMarketAmountBlock {
	block := blocks[scope]
	if block != nil {
		return block
	}
	code := "CNY"
	symbol := "¥"
	if len(currency) >= 2 {
		code = currency[0]
		symbol = currency[1]
	}
	block = &PortfolioMarketAmountBlock{Scope: scope, ScopeLabel: scopeLabel(scope), CurrencyCode: code, CurrencySymbol: symbol}
	blocks[scope] = block
	return block
}

func matchesKeyword(symbol, name, keyword string) bool {
	keyword = strings.TrimSpace(strings.ToLower(keyword))
	if keyword == "" {
		return true
	}
	symbol = strings.ToLower(strings.TrimSpace(symbol))
	name = strings.ToLower(strings.TrimSpace(name))
	return strings.Contains(symbol, keyword) || strings.Contains(name, keyword)
}

func computeHoldingDays(primaryDate, fallbackDate string) int {
	tradeDate := strings.TrimSpace(primaryDate)
	if tradeDate == "" {
		tradeDate = strings.TrimSpace(fallbackDate)
	}
	if tradeDate == "" {
		return 0
	}
	date, err := time.ParseInLocation("2006-01-02", tradeDate, shanghaiLocation())
	if err != nil {
		return 0
	}
	now := time.Now().In(shanghaiLocation())
	if now.Before(date) {
		return 0
	}
	return int(now.Sub(date).Hours()/24) + 1
}

func latestText(current, candidate string) string {
	if candidate == "" {
		return current
	}
	if current == "" || candidate > current {
		return candidate
	}
	return current
}

func hasAveragingDownRisk(positions []PortfolioPositionItem) bool {
	for _, item := range positions {
		if item.UnrealizedPnlAmount < 0 && item.BuyCount >= 2 {
			return true
		}
	}
	return false
}

func hasFrequentManualAdjust(positions []PortfolioPositionItem) bool {
	for _, item := range positions {
		if item.AdjustCount >= 3 {
			return true
		}
	}
	return false
}

func countRecentTrades(events []PortfolioRecentEventItem, days int) int {
	if days <= 0 {
		return 0
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -days)
	count := 0
	for _, item := range events {
		if item.EffectiveAt == "" {
			continue
		}
		ts, err := time.Parse(time.RFC3339, item.EffectiveAt)
		if err != nil {
			continue
		}
		if ts.After(cutoff) {
			count++
		}
	}
	return count
}

func buildMarketBreakdownFromSingleSummary(summary PortfolioDashboardSummary, positions []PortfolioPositionItem) []PortfolioMarketAmountBlock {
	if summary.Amounts == nil || len(positions) == 0 {
		return nil
	}
	scope := exchangeToScope(positions[0].Exchange)
	return []PortfolioMarketAmountBlock{{
		Scope:                  scope,
		ScopeLabel:             scopeLabel(scope),
		CurrencyCode:           summary.Amounts.CurrencyCode,
		CurrencySymbol:         summary.Amounts.CurrencySymbol,
		MarketValueAmount:      summary.Amounts.MarketValueAmount,
		TotalCostAmount:        summary.Amounts.TotalCostAmount,
		UnrealizedPnlAmount:    summary.Amounts.UnrealizedPnlAmount,
		RealizedPnlAmount:      summary.Amounts.RealizedPnlAmount,
		TotalPnlAmount:         summary.Amounts.TotalPnlAmount,
		TodayPnlAmount:         summary.Amounts.TodayPnlAmount,
		PositionCount:          summary.PositionCount,
		MaxPositionWeightRatio: summary.MaxPositionWeightRatio,
	}}
}

func normalizePortfolioSymbol(symbol string) string {
	return strings.TrimSpace(strings.ToUpper(symbol))
}

func buildInitTradeDate(record *PortfolioRecord, now time.Time) (string, time.Time) {
	if record != nil {
		if tradeDate := strings.TrimSpace(record.BuyDate); tradeDate != "" {
			if normalized, effectiveAt, err := normalizeTradeDate(tradeDate, now); err == nil {
				return normalized, effectiveAt
			}
		}
		if !record.UpdatedAt.IsZero() {
			local := record.UpdatedAt.In(shanghaiLocation())
			tradeDate := local.Format("2006-01-02")
			if normalized, effectiveAt, err := normalizeTradeDate(tradeDate, record.UpdatedAt); err == nil {
				return normalized, effectiveAt
			}
			return tradeDate, record.UpdatedAt.UTC()
		}
	}
	local := now.In(shanghaiLocation())
	tradeDate := local.Format("2006-01-02")
	normalized, effectiveAt, _ := normalizeTradeDate(tradeDate, now)
	return normalized, effectiveAt
}

func buildInitEventNote(existing string) string {
	existing = strings.TrimSpace(existing)
	if existing == "" {
		return "由旧版持仓快照迁移生成"
	}
	return fmt.Sprintf("%s（由旧版持仓快照迁移生成）", existing)
}

func buildPortfolioSummaryRecord(existing *PortfolioRecord, userID, symbol string, after portfolioPosition, eventNote, eventID string, effectiveAt, now time.Time) *PortfolioRecord {
	createdAt := now
	id := uuid.New().String()
	note := strings.TrimSpace(eventNote)
	if existing != nil {
		if existing.ID != "" {
			id = existing.ID
		}
		if !existing.CreatedAt.IsZero() {
			createdAt = existing.CreatedAt
		}
		if note == "" {
			note = existing.Note
		}
	}
	lastTradeAt := effectiveAt
	return &PortfolioRecord{
		ID:              id,
		UserID:          userID,
		Symbol:          symbol,
		Shares:          after.Shares,
		AvgCostPrice:    after.AvgCostPrice,
		TotalCostAmount: after.TotalCostAmount,
		BuyDate:         after.BuyDate,
		Note:            note,
		CostMethod:      CostMethodWeightedAvg,
		CostSource:      after.CostSource,
		LastTradeAt:     &lastTradeAt,
		LastEventID:     eventID,
		CreatedAt:       createdAt,
		UpdatedAt:       now,
	}
}

func buildPortfolioSummaryRecordFromState(existing *PortfolioRecord, userID, symbol string, state portfolioPosition, latestActive *PortfolioEventRecord, now time.Time) *PortfolioRecord {
	createdAt := now
	id := uuid.New().String()
	note := strings.TrimSpace(state.Note)
	var lastTradeAt *time.Time
	lastEventID := ""
	if existing != nil {
		if existing.ID != "" {
			id = existing.ID
		}
		if !existing.CreatedAt.IsZero() {
			createdAt = existing.CreatedAt
		}
		if note == "" {
			note = existing.Note
		}
	}
	if latestActive != nil {
		t := latestActive.EffectiveAt.UTC()
		lastTradeAt = &t
		lastEventID = latestActive.ID
		if strings.TrimSpace(note) == "" {
			note = strings.TrimSpace(latestActive.Note)
		}
	}
	costSource := state.CostSource
	if costSource == "" {
		costSource = CostSourceSystem
	}
	return &PortfolioRecord{
		ID:              id,
		UserID:          userID,
		Symbol:          symbol,
		Shares:          state.Shares,
		AvgCostPrice:    state.AvgCostPrice,
		TotalCostAmount: state.TotalCostAmount,
		BuyDate:         state.BuyDate,
		Note:            note,
		CostMethod:      CostMethodWeightedAvg,
		CostSource:      costSource,
		LastTradeAt:     lastTradeAt,
		LastEventID:     lastEventID,
		CreatedAt:       createdAt,
		UpdatedAt:       now,
	}
}

func rebuildPositionFromEvents(events []PortfolioEventRecord) (portfolioPosition, error) {
	state := portfolioPosition{
		CostMethod: CostMethodWeightedAvg,
		CostSource: CostSourceSystem,
	}
	for _, event := range events {
		computation, err := computePortfolioEvent(state, CreatePortfolioEventInput{
			EventType:          event.EventType,
			TradeDate:          event.TradeDate,
			Quantity:           event.Quantity,
			Price:              event.Price,
			FeeAmount:          event.FeeAmount,
			ManualAvgCostPrice: event.ManualAvgCostPrice,
			Note:               event.Note,
		})
		if err != nil {
			return portfolioPosition{}, err
		}
		state = computation.After
	}
	return state, nil
}
