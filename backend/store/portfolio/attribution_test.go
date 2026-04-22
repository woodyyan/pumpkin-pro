package portfolio

import (
	"context"
	"testing"

	"github.com/woodyyan/pumpkin-pro/backend/store/live"
	"github.com/woodyyan/pumpkin-pro/backend/tests/testutil"
)

type stubAttributionSnapshotProvider struct {
	items map[string]portfolioMarketSnapshot
}

func (s stubAttributionSnapshotProvider) FetchDetailedSnapshots(ctx context.Context, symbols []string) (map[string]portfolioMarketSnapshot, error) {
	result := make(map[string]portfolioMarketSnapshot, len(symbols))
	for _, symbol := range symbols {
		if item, ok := s.items[symbol]; ok {
			result[symbol] = item
		}
	}
	return result, nil
}

type stubAttributionHistoryReader struct {
	data map[string][]DailyBarRecord
}

func (s stubAttributionHistoryReader) GetDailyBars(ctx context.Context, symbols []string, startDate, endDate string) (map[string][]DailyBarRecord, error) {
	result := make(map[string][]DailyBarRecord, len(symbols))
	for _, symbol := range symbols {
		bars := s.data[symbol]
		filtered := make([]DailyBarRecord, 0, len(bars))
		for _, bar := range bars {
			if startDate != "" && bar.Date < startDate {
				continue
			}
			if endDate != "" && bar.Date > endDate {
				continue
			}
			filtered = append(filtered, bar)
		}
		result[symbol] = filtered
	}
	return result, nil
}

type stubAttributionBenchmarkReader struct {
	data map[string][]live.DailyBar
}

func (s stubAttributionBenchmarkReader) FetchBenchmarkDailyBars(ctx context.Context, benchmark string, lookbackDays int) ([]live.DailyBar, error) {
	return s.data[benchmark], nil
}

func setupAttributionService(t *testing.T) (*Service, context.Context) {
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
	svc := NewService(repo)
	svc.snapshotProvider = stubAttributionSnapshotProvider{items: map[string]portfolioMarketSnapshot{
		"000001.SZ": {Name: "平安银行", Exchange: "SZSE"},
		"000002.SZ": {Name: "万科A", Exchange: "SZSE"},
	}}
	return svc, context.Background()
}

func TestLoadAttributionDatasetStartsFromRangeOpeningState(t *testing.T) {
	svc, ctx := setupAttributionService(t)

	if _, _, err := svc.CreateEvent(ctx, "attr-user", "000001", CreatePortfolioEventInput{
		EventType: EventTypeBuy,
		TradeDate: "2026-04-01",
		Quantity:  100,
		Price:     10,
		Note:      "建仓",
	}); err != nil {
		t.Fatalf("seed buy failed: %v", err)
	}
	if _, _, err := svc.CreateEvent(ctx, "attr-user", "000001", CreatePortfolioEventInput{
		EventType: EventTypeSell,
		TradeDate: "2026-04-03",
		Quantity:  50,
		Price:     12,
		Note:      "减仓",
	}); err != nil {
		t.Fatalf("seed sell failed: %v", err)
	}

	history := stubAttributionHistoryReader{data: map[string][]DailyBarRecord{
		"000001": {
			{Code: "000001", Date: "2026-04-01", Close: 10},
			{Code: "000001", Date: "2026-04-02", Close: 11},
			{Code: "000001", Date: "2026-04-03", Close: 12},
			{Code: "000001", Date: "2026-04-04", Close: 13},
		},
	}}
	benchmarks := stubAttributionBenchmarkReader{data: map[string][]live.DailyBar{
		"SHCI": {
			{Date: "2026-04-01", Close: 100},
			{Date: "2026-04-02", Close: 101},
			{Date: "2026-04-03", Close: 102},
			{Date: "2026-04-04", Close: 103},
		},
	}}

	dataset, err := svc.loadAttributionDataset(ctx, "attr-user", PortfolioAttributionQuery{
		Scope:     PortfolioScopeAShare,
		Range:     AttributionRangeCustom,
		StartDate: "2026-04-03",
		EndDate:   "2026-04-04",
	}, history, benchmarks)
	if err != nil {
		t.Fatalf("loadAttributionDataset failed: %v", err)
	}
	if !dataset.Meta.HasData {
		t.Fatal("expected attribution dataset to have data")
	}
	items := dataset.SymbolsByScope[PortfolioScopeAShare]
	if len(items) != 1 {
		t.Fatalf("expected 1 stock aggregate, got %d", len(items))
	}
	if len(items[0].Snapshots) != 2 {
		t.Fatalf("expected 2 snapshots in range, got %d", len(items[0].Snapshots))
	}
	if got := items[0].Snapshots[0].Shares; got != 50 {
		t.Fatalf("expected first in-range snapshot to reflect post-sell 50 shares, got %v", got)
	}
	if got := dataset.ScopeAggregates[PortfolioScopeAShare].RealizedPnlAmount; got <= 0 {
		t.Fatalf("expected positive realized pnl after sell, got %v", got)
	}
}

func TestLoadAttributionDatasetShadowHoldExcludesPositionsOpenedWithinRange(t *testing.T) {
	svc, ctx := setupAttributionService(t)

	if _, _, err := svc.CreateEvent(ctx, "shadow-user", "000001", CreatePortfolioEventInput{
		EventType: EventTypeBuy,
		TradeDate: "2026-04-01",
		Quantity:  100,
		Price:     10,
		Note:      "原有仓位",
	}); err != nil {
		t.Fatalf("seed first buy failed: %v", err)
	}
	if _, _, err := svc.CreateEvent(ctx, "shadow-user", "000002", CreatePortfolioEventInput{
		EventType: EventTypeBuy,
		TradeDate: "2026-04-03",
		Quantity:  100,
		Price:     20,
		Note:      "区间中新开仓",
	}); err != nil {
		t.Fatalf("seed second buy failed: %v", err)
	}

	history := stubAttributionHistoryReader{data: map[string][]DailyBarRecord{
		"000001": {
			{Code: "000001", Date: "2026-04-01", Close: 10},
			{Code: "000001", Date: "2026-04-02", Close: 11},
			{Code: "000001", Date: "2026-04-03", Close: 12},
			{Code: "000001", Date: "2026-04-04", Close: 13},
		},
		"000002": {
			{Code: "000002", Date: "2026-04-03", Close: 20},
			{Code: "000002", Date: "2026-04-04", Close: 22},
		},
	}}
	benchmarks := stubAttributionBenchmarkReader{data: map[string][]live.DailyBar{
		"SHCI": {
			{Date: "2026-04-02", Close: 100},
			{Date: "2026-04-03", Close: 101},
			{Date: "2026-04-04", Close: 102},
		},
	}}

	dataset, err := svc.loadAttributionDataset(ctx, "shadow-user", PortfolioAttributionQuery{
		Scope:     PortfolioScopeAShare,
		Range:     AttributionRangeCustom,
		StartDate: "2026-04-02",
		EndDate:   "2026-04-04",
	}, history, benchmarks)
	if err != nil {
		t.Fatalf("loadAttributionDataset failed: %v", err)
	}

	agg := dataset.ScopeAggregates[PortfolioScopeAShare]
	if agg == nil {
		t.Fatal("expected ashare aggregate")
	}
	if got := agg.ShadowHoldPnlAmount; got != 200 {
		t.Fatalf("expected shadow hold pnl 200 from opening position only, got %v", got)
	}
}
