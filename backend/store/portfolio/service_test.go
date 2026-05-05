package portfolio

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/woodyyan/pumpkin-pro/backend/tests/testutil"
)

func setupPortfolioService(t *testing.T) (*Service, context.Context) {
	t.Helper()
	db := testutil.InMemoryDB(t)
	testutil.AutoMigrateModels(t, db,
		PortfolioRecord{},
		PortfolioEventRecord{},
		PortfolioDailySnapshotRecord{},
		PortfolioPositionDailySnapshotRecord{},
		SecurityProfileRecord{},
		InvestmentProfileRecord{},
	)
	repo := NewRepository(db)
	return NewService(repo), context.Background()
}

type stubHistoryReader struct {
	bars map[string][]DailyBarRecord
}

func (s *stubHistoryReader) GetDailyBars(ctx context.Context, symbols []string, startDate, endDate string) (map[string][]DailyBarRecord, error) {
	result := make(map[string][]DailyBarRecord, len(symbols))
	for _, symbol := range symbols {
		if bars, ok := s.bars[symbol]; ok {
			result[symbol] = bars
		}
	}
	return result, nil
}

func seedPortfolioProfile(t *testing.T, svc *Service, symbol, exchange, name string) {
	t.Helper()
	now := time.Now().UTC()
	if err := svc.repo.UpsertSecurityProfiles(context.Background(), []SecurityProfileRecord{{
		Symbol:        symbol,
		Exchange:      exchange,
		Name:          name,
		BenchmarkCode: resolveBenchmarkCode(exchange, ""),
		Source:        "test",
		CreatedAt:     now,
		UpdatedAt:     now,
	}}); err != nil {
		t.Fatalf("UpsertSecurityProfiles failed: %v", err)
	}
}

func buildStubBars(code string, closes map[string]float64) []DailyBarRecord {
	dates := make([]string, 0, len(closes))
	for date := range closes {
		dates = append(dates, date)
	}
	sort.Strings(dates)
	items := make([]DailyBarRecord, 0, len(dates))
	for _, date := range dates {
		items = append(items, DailyBarRecord{Code: code, Date: date, Close: closes[date]})
	}
	return items
}

func TestServiceCreateBuyEventBuildsWeightedAverage(t *testing.T) {
	svc, ctx := setupPortfolioService(t)

	item, event, err := svc.CreateEvent(ctx, "svc-user", "000001", CreatePortfolioEventInput{
		EventType: EventTypeBuy,
		TradeDate: "2026-04-20",
		Quantity:  100,
		Price:     10,
		FeeAmount: 0,
		Note:      "首次建仓",
	})
	if err != nil {
		t.Fatalf("CreateEvent buy failed: %v", err)
	}
	if item.Shares != 100 {
		t.Fatalf("expected shares 100, got %v", item.Shares)
	}
	if item.AvgCostPrice != 10 {
		t.Fatalf("expected avg cost 10, got %v", item.AvgCostPrice)
	}
	if event.AfterTotalCost != 1000 {
		t.Fatalf("expected after total cost 1000, got %v", event.AfterTotalCost)
	}

	item, _, err = svc.CreateEvent(ctx, "svc-user", "000001", CreatePortfolioEventInput{
		EventType: EventTypeBuy,
		TradeDate: "2026-04-21",
		Quantity:  200,
		Price:     13,
		FeeAmount: 0,
		Note:      "二次加仓",
	})
	if err != nil {
		t.Fatalf("CreateEvent second buy failed: %v", err)
	}
	if item.Shares != 300 {
		t.Fatalf("expected shares 300, got %v", item.Shares)
	}
	if item.AvgCostPrice != 12 {
		t.Fatalf("expected weighted avg 12, got %v", item.AvgCostPrice)
	}
	if item.TotalCostAmount != 3600 {
		t.Fatalf("expected total cost 3600, got %v", item.TotalCostAmount)
	}
}

func TestServiceSellEventKeepsAverageCost(t *testing.T) {
	svc, ctx := setupPortfolioService(t)
	if _, _, err := svc.CreateEvent(ctx, "sell-user", "600036", CreatePortfolioEventInput{
		EventType: EventTypeBuy,
		TradeDate: "2026-04-20",
		Quantity:  300,
		Price:     12,
		Note:      "建仓",
	}); err != nil {
		t.Fatalf("buy failed: %v", err)
	}

	item, event, err := svc.CreateEvent(ctx, "sell-user", "600036", CreatePortfolioEventInput{
		EventType: EventTypeSell,
		TradeDate: "2026-04-23",
		Quantity:  100,
		Price:     13,
		Note:      "减仓",
	})
	if err != nil {
		t.Fatalf("sell failed: %v", err)
	}
	if item.Shares != 200 {
		t.Fatalf("expected remaining shares 200, got %v", item.Shares)
	}
	if item.AvgCostPrice != 12 {
		t.Fatalf("expected avg cost remains 12, got %v", item.AvgCostPrice)
	}
	if item.TotalCostAmount != 2400 {
		t.Fatalf("expected total cost 2400, got %v", item.TotalCostAmount)
	}
	if event.RealizedPnlAmount != 100 {
		t.Fatalf("expected realized pnl 100, got %v", event.RealizedPnlAmount)
	}
}

func TestServiceAdjustAvgCost(t *testing.T) {
	svc, ctx := setupPortfolioService(t)
	if _, _, err := svc.CreateEvent(ctx, "adjust-user", "300750", CreatePortfolioEventInput{
		EventType: EventTypeBuy,
		TradeDate: "2026-04-20",
		Quantity:  200,
		Price:     10,
		Note:      "建仓",
	}); err != nil {
		t.Fatalf("buy failed: %v", err)
	}

	item, event, err := svc.CreateEvent(ctx, "adjust-user", "300750", CreatePortfolioEventInput{
		EventType:          EventTypeAdjustAvgCost,
		TradeDate:          "2026-04-25",
		ManualAvgCostPrice: 9.5,
		Note:               "补录手续费后修正",
	})
	if err != nil {
		t.Fatalf("adjust avg cost failed: %v", err)
	}
	if item.Shares != 200 {
		t.Fatalf("expected shares unchanged at 200, got %v", item.Shares)
	}
	if item.AvgCostPrice != 9.5 {
		t.Fatalf("expected avg cost 9.5, got %v", item.AvgCostPrice)
	}
	if item.CostSource != CostSourceManual {
		t.Fatalf("expected cost source manual, got %s", item.CostSource)
	}
	if event.AfterTotalCost != 1900 {
		t.Fatalf("expected total cost 1900, got %v", event.AfterTotalCost)
	}
}

func TestServiceEnsureInitEventFromSnapshot(t *testing.T) {
	svc, ctx := setupPortfolioService(t)
	now := time.Now().UTC()
	if err := svc.repo.Upsert(ctx, &PortfolioRecord{
		ID:              "legacy-1",
		UserID:          "legacy-user",
		Symbol:          "601318",
		Shares:          800,
		AvgCostPrice:    42.5,
		TotalCostAmount: 34000,
		BuyDate:         "2025-01-10",
		Note:            "旧版快照",
		CreatedAt:       now,
		UpdatedAt:       now,
	}); err != nil {
		t.Fatalf("seed legacy snapshot failed: %v", err)
	}

	if err := svc.EnsureInitEventFromSnapshot(ctx, "legacy-user", "601318"); err != nil {
		t.Fatalf("EnsureInitEventFromSnapshot failed: %v", err)
	}

	detail, err := svc.GetDetailBySymbol(ctx, "legacy-user", "601318")
	if err != nil {
		t.Fatalf("GetDetailBySymbol failed: %v", err)
	}
	if detail.Item == nil {
		t.Fatalf("expected portfolio item after migration")
	}
	if len(detail.HistoryPreview) != 1 {
		t.Fatalf("expected 1 migrated init event, got %d", len(detail.HistoryPreview))
	}
	if detail.HistoryPreview[0].EventType != EventTypeInit {
		t.Fatalf("expected init event, got %s", detail.HistoryPreview[0].EventType)
	}
}

func TestServiceUndoLatestEvent(t *testing.T) {
	svc, ctx := setupPortfolioService(t)
	_, firstEvent, err := svc.CreateEvent(ctx, "undo-user", "000001", CreatePortfolioEventInput{
		EventType: EventTypeBuy,
		TradeDate: "2026-04-20",
		Quantity:  100,
		Price:     10,
		Note:      "建仓",
	})
	if err != nil {
		t.Fatalf("buy failed: %v", err)
	}
	_, secondEvent, err := svc.CreateEvent(ctx, "undo-user", "000001", CreatePortfolioEventInput{
		EventType: EventTypeBuy,
		TradeDate: "2026-04-21",
		Quantity:  100,
		Price:     12,
		Note:      "加仓",
	})
	if err != nil {
		t.Fatalf("second buy failed: %v", err)
	}

	result, err := svc.UndoLatestEvent(ctx, "undo-user", "000001", secondEvent.ID)
	if err != nil {
		t.Fatalf("UndoLatestEvent failed: %v", err)
	}
	if result.Item == nil {
		t.Fatalf("expected updated item after undo")
	}
	if result.Item.Shares != 100 {
		t.Fatalf("expected shares restored to 100, got %v", result.Item.Shares)
	}
	if result.Item.AvgCostPrice != 10 {
		t.Fatalf("expected avg cost restored to 10, got %v", result.Item.AvgCostPrice)
	}
	if result.Item.LastEventID != firstEvent.ID {
		t.Fatalf("expected last event id restored to first event, got %s", result.Item.LastEventID)
	}
}

func TestServiceDeleteRebuildsHistoricalSnapshots(t *testing.T) {
	svc, ctx := setupPortfolioService(t)
	seedPortfolioProfile(t, svc, "600519.SH", "SSE", "贵州茅台")
	seedPortfolioProfile(t, svc, "00700.HK", "HKEX", "腾讯控股")
	svc.historyReader = &stubHistoryReader{bars: map[string][]DailyBarRecord{
		"600519": buildStubBars("600519", map[string]float64{
			"2026-04-20": 10,
			"2026-04-21": 11,
			"2026-04-22": 12,
		}),
		"00700": buildStubBars("00700", map[string]float64{
			"2026-04-20": 20,
			"2026-04-21": 21,
			"2026-04-22": 22,
		}),
	}}

	if _, _, err := svc.CreateEvent(ctx, "delete-user", "600519.SH", CreatePortfolioEventInput{
		EventType: EventTypeBuy,
		TradeDate: "2026-04-20",
		Quantity:  100,
		Price:     10,
	}); err != nil {
		t.Fatalf("CreateEvent A-share buy failed: %v", err)
	}
	if _, _, err := svc.CreateEvent(ctx, "delete-user", "00700.HK", CreatePortfolioEventInput{
		EventType: EventTypeBuy,
		TradeDate: "2026-04-20",
		Quantity:  50,
		Price:     20,
	}); err != nil {
		t.Fatalf("CreateEvent HK buy failed: %v", err)
	}
	if _, _, err := svc.CreateEvent(ctx, "delete-user", "600519.SH", CreatePortfolioEventInput{
		EventType: EventTypeSell,
		TradeDate: "2026-04-21",
		Quantity:  40,
		Price:     11,
	}); err != nil {
		t.Fatalf("CreateEvent A-share sell failed: %v", err)
	}

	result, err := svc.Delete(ctx, "delete-user", "600519.SH")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if result.Symbol != "600519.SH" {
		t.Fatalf("expected symbol 600519.SH, got %s", result.Symbol)
	}
	if result.DeletedEventCount != 2 {
		t.Fatalf("expected 2 deleted events, got %d", result.DeletedEventCount)
	}
	if !result.CacheRebuilt {
		t.Fatal("expected cache rebuilt")
	}

	detail, err := svc.GetDetailBySymbol(ctx, "delete-user", "600519.SH")
	if err != nil {
		t.Fatalf("GetDetailBySymbol failed: %v", err)
	}
	if detail.Item != nil || len(detail.HistoryPreview) != 0 {
		t.Fatalf("expected deleted symbol detail to be empty, got %+v", detail)
	}

	daily, err := svc.repo.ListDailySnapshots(ctx, "delete-user", nil, "")
	if err != nil {
		t.Fatalf("ListDailySnapshots failed: %v", err)
	}
	if len(daily) == 0 {
		t.Fatal("expected rebuilt daily snapshots")
	}
	for _, item := range daily {
		if item.Scope != PortfolioScopeHK {
			t.Fatalf("expected only HK scope after delete, got %s", item.Scope)
		}
		if item.PositionCount != 1 {
			t.Fatalf("expected position_count=1, got %d", item.PositionCount)
		}
		if item.TotalCostAmount <= 0 || item.MarketValueAmount <= 0 {
			t.Fatalf("expected positive rebuilt snapshot values, got %+v", item)
		}
	}

	positions, err := svc.repo.ListPositionDailySnapshots(ctx, "delete-user", nil, "", "")
	if err != nil {
		t.Fatalf("ListPositionDailySnapshots failed: %v", err)
	}
	if len(positions) == 0 {
		t.Fatal("expected rebuilt position snapshots")
	}
	for _, item := range positions {
		if item.Symbol != "00700.HK" {
			t.Fatalf("expected only HK position snapshots, got %s", item.Symbol)
		}
		if item.AvgCostPrice != 20 {
			t.Fatalf("expected avg cost 20, got %v", item.AvgCostPrice)
		}
		if item.PositionWeightRatio != 1 {
			t.Fatalf("expected position weight 1, got %v", item.PositionWeightRatio)
		}
	}
}

func TestServiceGetPnlCalendarBuildsMonthDays(t *testing.T) {
	svc, ctx := setupPortfolioService(t)
	now := time.Now().UTC()
	if err := svc.repo.UpsertDailySnapshot(ctx, &PortfolioDailySnapshotRecord{
		ID: "calendar-day", UserID: "calendar-user", Scope: PortfolioScopeAShare, SnapshotDate: "2026-05-05",
		CurrencyCode: "CNY", MarketValueAmount: 11000, TotalCostAmount: 10000, TodayPnlAmount: 1000, PositionCount: 1,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("UpsertDailySnapshot failed: %v", err)
	}

	payload, err := svc.GetPnlCalendar(ctx, "calendar-user", PortfolioPnlCalendarQuery{Scope: PortfolioScopeAShare, Year: 2026, Month: 5})
	if err != nil {
		t.Fatalf("GetPnlCalendar failed: %v", err)
	}
	if len(payload.Days) != 31 {
		t.Fatalf("expected 31 days, got %d", len(payload.Days))
	}
	if payload.Days[0].Date != "2026-05-01" || payload.Days[30].Date != "2026-05-31" {
		t.Fatalf("expected full May date range, got first=%s last=%s", payload.Days[0].Date, payload.Days[30].Date)
	}
	if payload.CurrencyCode != "CNY" || payload.Scope != PortfolioScopeAShare {
		t.Fatalf("unexpected payload identity: %+v", payload)
	}
}

func TestServiceGetPnlCalendarCombinesHoldingAndRealizedPnl(t *testing.T) {
	svc, ctx := setupPortfolioService(t)
	now := time.Now().UTC()
	if err := svc.repo.UpsertDailySnapshot(ctx, &PortfolioDailySnapshotRecord{
		ID: "calendar-combine", UserID: "calendar-combine-user", Scope: PortfolioScopeAShare, SnapshotDate: "2026-05-10",
		CurrencyCode: "CNY", MarketValueAmount: 10100, TotalCostAmount: 10000, TodayPnlAmount: 100, PositionCount: 1,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("UpsertDailySnapshot failed: %v", err)
	}
	if err := svc.repo.CreateEvent(ctx, &PortfolioEventRecord{
		ID: "calendar-realized", UserID: "calendar-combine-user", Symbol: "600519.SH", EventType: EventTypeSell,
		TradeDate: "2026-05-10", EffectiveAt: now, RealizedPnlAmount: 50, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}

	payload, err := svc.GetPnlCalendar(ctx, "calendar-combine-user", PortfolioPnlCalendarQuery{Scope: PortfolioScopeAShare, Year: 2026, Month: 5})
	if err != nil {
		t.Fatalf("GetPnlCalendar failed: %v", err)
	}
	day := payload.Days[9]
	if day.Date != "2026-05-10" {
		t.Fatalf("expected May 10 at index 9, got %s", day.Date)
	}
	if day.PnlAmount != 150 || day.HoldingPnlAmount != 100 || day.RealizedPnlAmount != 50 {
		t.Fatalf("expected combined pnl 150 from 100 + 50, got %+v", day)
	}
}

func TestServiceGetPnlCalendarCalculatesRateFromBaseAmount(t *testing.T) {
	svc, ctx := setupPortfolioService(t)
	now := time.Now().UTC()
	if err := svc.repo.UpsertDailySnapshot(ctx, &PortfolioDailySnapshotRecord{
		ID: "calendar-rate", UserID: "calendar-rate-user", Scope: PortfolioScopeAShare, SnapshotDate: "2026-05-12",
		CurrencyCode: "CNY", MarketValueAmount: 11000, TotalCostAmount: 9500, TodayPnlAmount: 1000, PositionCount: 1,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("UpsertDailySnapshot failed: %v", err)
	}

	payload, err := svc.GetPnlCalendar(ctx, "calendar-rate-user", PortfolioPnlCalendarQuery{Scope: PortfolioScopeAShare, Year: 2026, Month: 5})
	if err != nil {
		t.Fatalf("GetPnlCalendar failed: %v", err)
	}
	day := payload.Days[11]
	if day.BaseAmount != 10000 {
		t.Fatalf("expected base 10000, got %v", day.BaseAmount)
	}
	if day.PnlRate == nil || *day.PnlRate != 0.1 {
		t.Fatalf("expected pnl rate 0.1, got %+v", day.PnlRate)
	}
}

func TestServiceGetPnlCalendarRejectsAllScope(t *testing.T) {
	svc, ctx := setupPortfolioService(t)
	if _, err := svc.GetPnlCalendar(ctx, "calendar-invalid", PortfolioPnlCalendarQuery{Scope: PortfolioScopeAll, Year: 2026, Month: 5}); err == nil {
		t.Fatal("expected ALL scope to be rejected")
	}
}

func TestServiceDeleteReturnsNotFoundWhenMissing(t *testing.T) {
	svc, ctx := setupPortfolioService(t)
	if _, err := svc.Delete(ctx, "missing-user", "600519.SH"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestServiceValidation(t *testing.T) {
	svc, ctx := setupPortfolioService(t)
	if _, _, err := svc.CreateEvent(ctx, "val-user", "BAD", CreatePortfolioEventInput{
		EventType: EventTypeSell,
		TradeDate: "2026-04-20",
		Quantity:  10,
		Price:     10,
	}); err == nil {
		t.Fatal("expected error for selling without position")
	}
	if _, _, err := svc.CreateEvent(ctx, "val-user", "BAD", CreatePortfolioEventInput{
		EventType: EventTypeBuy,
		TradeDate: "2026-04-20",
		Quantity:  -1,
		Price:     10,
	}); err == nil {
		t.Fatal("expected error for negative quantity")
	}
	if _, _, err := svc.CreateEvent(ctx, "val-user", "BAD", CreatePortfolioEventInput{
		EventType:          EventTypeAdjustAvgCost,
		TradeDate:          "2026-04-20",
		ManualAvgCostPrice: 10,
	}); err == nil {
		t.Fatal("expected error for adjusting avg cost without position")
	}
}

func TestServiceUpsertAndListRemainCompatible(t *testing.T) {
	svc, ctx := setupPortfolioService(t)
	item, err := svc.Upsert(ctx, "compat-user", "sh601318", UpsertPortfolioInput{
		Shares:       100,
		AvgCostPrice: 30,
		BuyDate:      "2026-04-20",
		Note:         "兼容写入",
	})
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}
	if item.Symbol != "SH601318" {
		t.Fatalf("expected uppercase symbol, got %s", item.Symbol)
	}
	list, err := svc.ListByUser(ctx, "compat-user")
	if err != nil {
		t.Fatalf("ListByUser failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 listed item, got %d", len(list))
	}
}

func TestServiceInvestmentProfile(t *testing.T) {
	svc, ctx := setupPortfolioService(t)
	prof, err := svc.UpsertInvestmentProfile(ctx, "inv-user", UpsertInvestmentProfileInput{
		TotalCapital:             500000,
		RiskPreference:           "conservative",
		InvestmentGoal:           "income",
		InvestmentHorizon:        "long",
		MaxDrawdownPct:           10.0,
		ExperienceLevel:          "beginner",
		DefaultFeeRateAShareBuy:  0.0003,
		DefaultFeeRateAShareSell: 0.0008,
		DefaultFeeRateHKBuy:      0.0013,
		DefaultFeeRateHKSell:     0.0013,
		Note:                     "conservative investor",
	})
	if err != nil {
		t.Fatalf("UpsertInvestmentProfile failed: %v", err)
	}
	if prof.TotalCapital != 500000 {
		t.Errorf("expected TotalCapital 500000, got %v", prof.TotalCapital)
	}
	if prof.DefaultFeeRateAShareBuy != 0.0003 || prof.DefaultFeeRateAShareSell != 0.0008 {
		t.Errorf("expected A股 default fee rates saved, got buy=%v sell=%v", prof.DefaultFeeRateAShareBuy, prof.DefaultFeeRateAShareSell)
	}
	if prof.DefaultFeeRateHKBuy != 0.0013 || prof.DefaultFeeRateHKSell != 0.0013 {
		t.Errorf("expected 港股 default fee rates saved, got buy=%v sell=%v", prof.DefaultFeeRateHKBuy, prof.DefaultFeeRateHKSell)
	}

	got, err := svc.GetInvestmentProfile(ctx, "inv-user")
	if err != nil {
		t.Fatalf("GetInvestmentProfile failed: %v", err)
	}
	if got.InvestmentGoal != "income" {
		t.Errorf("expected investment goal 'income', got %s", got.InvestmentGoal)
	}
	if _, err := svc.UpsertInvestmentProfile(ctx, "inv-user", UpsertInvestmentProfileInput{DefaultFeeRateAShareBuy: -0.01}); err == nil {
		t.Fatal("expected error for negative A股 buy fee rate")
	}
	if _, err := svc.UpsertInvestmentProfile(ctx, "inv-user", UpsertInvestmentProfileInput{DefaultFeeRateHKSell: 0.06}); err == nil {
		t.Fatal("expected error for oversized 港股 sell fee rate")
	}
}
