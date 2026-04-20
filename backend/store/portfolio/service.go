package portfolio

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const portfolioHistoryPreviewLimit = 5

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
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
