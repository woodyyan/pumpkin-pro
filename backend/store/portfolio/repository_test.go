package portfolio

import (
	"context"
	"testing"
	"time"

	"github.com/woodyyan/pumpkin-pro/backend/tests/testutil"
)

func setupPortfolioDB(t *testing.T) (*Repository, context.Context) {
	t.Helper()
	db := testutil.InMemoryDB(t)
	testutil.AutoMigrateModels(t, db,
		PortfolioRecord{},
		PortfolioEventRecord{},
		PortfolioDailySnapshotRecord{},
		PortfolioPositionDailySnapshotRecord{},
		InvestmentProfileRecord{},
	)
	return NewRepository(db), context.Background()
}

func TestCreateAndGetPortfolioItem(t *testing.T) {
	repo, ctx := setupPortfolioDB(t)
	now := time.Now().UTC()
	record := &PortfolioRecord{
		ID:              "pf-001",
		UserID:          "user-1",
		Symbol:          "000001",
		Shares:          1000,
		AvgCostPrice:    12.5,
		TotalCostAmount: 12500,
		BuyDate:         "2025-01-15",
		Note:            "测试持仓",
		CostMethod:      CostMethodWeightedAvg,
		CostSource:      CostSourceSystem,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := repo.Upsert(ctx, record); err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	found, err := repo.GetBySymbol(ctx, "user-1", "000001")
	if err != nil {
		t.Fatalf("GetBySymbol failed: %v", err)
	}
	if found.Shares != 1000 {
		t.Errorf("expected Shares=1000, got %v", found.Shares)
	}
	if found.TotalCostAmount != 12500 {
		t.Errorf("expected TotalCostAmount=12500, got %v", found.TotalCostAmount)
	}
}

func TestUpsertUpdatesExisting(t *testing.T) {
	repo, ctx := setupPortfolioDB(t)
	now := time.Now().UTC()
	if err := repo.Upsert(ctx, &PortfolioRecord{
		ID: "pf-upsert", UserID: "user-2", Symbol: "600036",
		Shares: 500, AvgCostPrice: 10.0, TotalCostAmount: 5000,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("initial Upsert failed: %v", err)
	}

	updatedAt := now.Add(time.Hour)
	lastTradeAt := updatedAt
	err := repo.Upsert(ctx, &PortfolioRecord{
		ID:              "pf-upsert-new-id",
		UserID:          "user-2",
		Symbol:          "600036",
		Shares:          2000,
		AvgCostPrice:    11.5,
		TotalCostAmount: 23000,
		BuyDate:         "2025-02-01",
		Note:            "updated note",
		CostMethod:      CostMethodWeightedAvg,
		CostSource:      CostSourceManual,
		LastTradeAt:     &lastTradeAt,
		LastEventID:     "evt-1",
		UpdatedAt:       updatedAt,
	})
	if err != nil {
		t.Fatalf("update Upsert failed: %v", err)
	}

	found, _ := repo.GetBySymbol(ctx, "user-2", "600036")
	if found.Shares != 2000 {
		t.Errorf("expected updated Shares=2000, got %v", found.Shares)
	}
	if found.AvgCostPrice != 11.5 {
		t.Errorf("expected updated AvgCostPrice=11.5, got %v", found.AvgCostPrice)
	}
	if found.LastEventID != "evt-1" {
		t.Errorf("expected LastEventID=evt-1, got %s", found.LastEventID)
	}
}

func TestDeletePortfolioRemovesEvents(t *testing.T) {
	repo, ctx := setupPortfolioDB(t)
	now := time.Now().UTC()
	if err := repo.Upsert(ctx, &PortfolioRecord{
		ID: "pf-del", UserID: "user-3", Symbol: "300750",
		Shares: 100, AvgCostPrice: 50.0, TotalCostAmount: 5000,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}
	if err := repo.CreateEvent(ctx, &PortfolioEventRecord{
		ID: "evt-del", UserID: "user-3", Symbol: "300750", EventType: EventTypeBuy,
		TradeDate: "2025-01-01", EffectiveAt: now,
		Quantity: 100, Price: 50, AfterShares: 100, AfterAvgCostPrice: 50, AfterTotalCost: 5000,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}

	if err := repo.Delete(ctx, "user-3", "300750"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if _, err := repo.GetBySymbol(ctx, "user-3", "300750"); err != ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
	events, err := repo.ListEventsBySymbol(ctx, "user-3", "300750", 10)
	if err != nil {
		t.Fatalf("ListEventsBySymbol failed: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected 0 events after delete, got %d", len(events))
	}
}

func TestDeletePortfolioWithEventsAllowsEventOnlySymbol(t *testing.T) {
	repo, ctx := setupPortfolioDB(t)
	now := time.Now().UTC()
	if err := repo.CreateEvent(ctx, &PortfolioEventRecord{
		ID: "evt-only", UserID: "user-event-only", Symbol: "00700.HK", EventType: EventTypeSell,
		TradeDate: "2025-01-02", EffectiveAt: now,
		Quantity: 100, Price: 400, RealizedPnlAmount: 1200,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}

	count, err := repo.DeletePortfolioWithEvents(ctx, "user-event-only", "00700.HK")
	if err != nil {
		t.Fatalf("DeletePortfolioWithEvents failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 deleted event, got %d", count)
	}
	items, err := repo.ListEventsBySymbol(ctx, "user-event-only", "00700.HK", 10)
	if err != nil {
		t.Fatalf("ListEventsBySymbol failed: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected events removed, got %d", len(items))
	}
}

func TestDeleteUserSnapshotsHelpers(t *testing.T) {
	repo, ctx := setupPortfolioDB(t)
	now := time.Now().UTC()
	if err := repo.UpsertDailySnapshot(ctx, &PortfolioDailySnapshotRecord{
		ID: "ds-1", UserID: "user-snap", Scope: PortfolioScopeAShare, SnapshotDate: "2026-04-20", CurrencyCode: "CNY", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("UpsertDailySnapshot failed: %v", err)
	}
	if err := repo.UpsertPositionDailySnapshots(ctx, []PortfolioPositionDailySnapshotRecord{{
		ID: "ps-1", UserID: "user-snap", SnapshotDate: "2026-04-20", Symbol: "600519.SH", Exchange: "SSE", CurrencyCode: "CNY", CurrencySymbol: "¥", Name: "贵州茅台", CreatedAt: now, UpdatedAt: now,
	}}); err != nil {
		t.Fatalf("UpsertPositionDailySnapshots failed: %v", err)
	}
	if err := repo.DeleteDailySnapshotsByUser(ctx, "user-snap"); err != nil {
		t.Fatalf("DeleteDailySnapshotsByUser failed: %v", err)
	}
	if err := repo.DeletePositionDailySnapshotsByUser(ctx, "user-snap"); err != nil {
		t.Fatalf("DeletePositionDailySnapshotsByUser failed: %v", err)
	}
	daily, err := repo.ListDailySnapshots(ctx, "user-snap", nil, "")
	if err != nil {
		t.Fatalf("ListDailySnapshots failed: %v", err)
	}
	if len(daily) != 0 {
		t.Fatalf("expected 0 daily snapshots, got %d", len(daily))
	}
	positions, err := repo.ListPositionDailySnapshots(ctx, "user-snap", nil, "", "")
	if err != nil {
		t.Fatalf("ListPositionDailySnapshots failed: %v", err)
	}
	if len(positions) != 0 {
		t.Fatalf("expected 0 position snapshots, got %d", len(positions))
	}
}

func TestListByUserFiltersZeroShares(t *testing.T) {
	repo, ctx := setupPortfolioDB(t)
	now := time.Now().UTC()
	for _, record := range []*PortfolioRecord{
		{ID: "pf-a", UserID: "user-list", Symbol: "000001", Shares: 100, AvgCostPrice: 10, TotalCostAmount: 1000, CreatedAt: now, UpdatedAt: now},
		{ID: "pf-b", UserID: "user-list", Symbol: "600036", Shares: 0, AvgCostPrice: 0, TotalCostAmount: 0, CreatedAt: now, UpdatedAt: now},
	} {
		if err := repo.Upsert(ctx, record); err != nil {
			t.Fatalf("Upsert failed: %v", err)
		}
	}

	items, err := repo.ListByUser(ctx, "user-list")
	if err != nil {
		t.Fatalf("ListByUser failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 visible item, got %d", len(items))
	}
	if items[0].Symbol != "000001" {
		t.Fatalf("expected symbol 000001, got %s", items[0].Symbol)
	}
}

func TestEventRepositoryLifecycle(t *testing.T) {
	repo, ctx := setupPortfolioDB(t)
	now := time.Now().UTC()
	events := []*PortfolioEventRecord{
		{ID: "evt-1", UserID: "user-evt", Symbol: "600036", EventType: EventTypeBuy, TradeDate: "2025-01-01", EffectiveAt: now, Quantity: 100, Price: 10, AfterShares: 100, AfterAvgCostPrice: 10, AfterTotalCost: 1000, CreatedAt: now, UpdatedAt: now},
		{ID: "evt-2", UserID: "user-evt", Symbol: "600036", EventType: EventTypeSell, TradeDate: "2025-01-02", EffectiveAt: now.Add(time.Hour), Quantity: 40, Price: 11, BeforeShares: 100, BeforeAvgCostPrice: 10, BeforeTotalCost: 1000, AfterShares: 60, AfterAvgCostPrice: 10, AfterTotalCost: 600, RealizedPnlAmount: 40, CreatedAt: now.Add(time.Hour), UpdatedAt: now.Add(time.Hour)},
	}
	for _, event := range events {
		if err := repo.CreateEvent(ctx, event); err != nil {
			t.Fatalf("CreateEvent failed: %v", err)
		}
	}

	hasEvents, err := repo.HasActiveEventsBySymbol(ctx, "user-evt", "600036")
	if err != nil {
		t.Fatalf("HasActiveEventsBySymbol failed: %v", err)
	}
	if !hasEvents {
		t.Fatalf("expected active events")
	}

	latest, err := repo.GetLatestActiveEventBySymbol(ctx, "user-evt", "600036")
	if err != nil {
		t.Fatalf("GetLatestActiveEventBySymbol failed: %v", err)
	}
	if latest.ID != "evt-2" {
		t.Fatalf("expected latest event evt-2, got %s", latest.ID)
	}

	listDesc, err := repo.ListEventsBySymbol(ctx, "user-evt", "600036", 10)
	if err != nil {
		t.Fatalf("ListEventsBySymbol failed: %v", err)
	}
	if len(listDesc) != 2 || listDesc[0].ID != "evt-2" {
		t.Fatalf("expected desc order evt-2 first, got %+v", listDesc)
	}

	listAsc, err := repo.ListAllActiveEventsAsc(ctx, "user-evt", "600036")
	if err != nil {
		t.Fatalf("ListAllActiveEventsAsc failed: %v", err)
	}
	if len(listAsc) != 2 || listAsc[0].ID != "evt-1" {
		t.Fatalf("expected asc order evt-1 first, got %+v", listAsc)
	}

	if err := repo.VoidEvent(ctx, "user-evt", "evt-2", "void-op"); err != nil {
		t.Fatalf("VoidEvent failed: %v", err)
	}
	latestAfterVoid, err := repo.GetLatestActiveEventBySymbol(ctx, "user-evt", "600036")
	if err != nil {
		t.Fatalf("GetLatestActiveEventBySymbol after void failed: %v", err)
	}
	if latestAfterVoid.ID != "evt-1" {
		t.Fatalf("expected latest active event evt-1 after void, got %s", latestAfterVoid.ID)
	}
}

func TestInvestmentProfileCRUD(t *testing.T) {
	repo, ctx := setupPortfolioDB(t)
	record := &InvestmentProfileRecord{
		UserID:                   "profile-user",
		TotalCapital:             100000,
		RiskPreference:           "moderate",
		InvestmentGoal:           "growth",
		InvestmentHorizon:        "medium",
		MaxDrawdownPct:           15.0,
		ExperienceLevel:          "intermediate",
		DefaultFeeRateAShareBuy:  0.0003,
		DefaultFeeRateAShareSell: 0.0008,
		DefaultFeeRateHKBuy:      0.0013,
		DefaultFeeRateHKSell:     0.0013,
		Note:                     "test profile",
		UpdatedAt:                time.Now().UTC(),
	}
	if err := repo.UpsertInvestmentProfile(ctx, record); err != nil {
		t.Fatalf("UpsertInvestmentProfile create failed: %v", err)
	}

	found, err := repo.GetInvestmentProfile(ctx, "profile-user")
	if err != nil {
		t.Fatalf("GetInvestmentProfile failed: %v", err)
	}
	if found.TotalCapital != 100000 {
		t.Errorf("expected TotalCapital=100000, got %v", found.TotalCapital)
	}
	if found.DefaultFeeRateAShareBuy != 0.0003 || found.DefaultFeeRateAShareSell != 0.0008 {
		t.Errorf("expected A股 default fee rates saved, got buy=%v sell=%v", found.DefaultFeeRateAShareBuy, found.DefaultFeeRateAShareSell)
	}
	if found.DefaultFeeRateHKBuy != 0.0013 || found.DefaultFeeRateHKSell != 0.0013 {
		t.Errorf("expected 港股 default fee rates saved, got buy=%v sell=%v", found.DefaultFeeRateHKBuy, found.DefaultFeeRateHKSell)
	}

	updated := &InvestmentProfileRecord{
		UserID:                   "profile-user",
		TotalCapital:             200000,
		RiskPreference:           "aggressive",
		MaxDrawdownPct:           25.0,
		DefaultFeeRateAShareBuy:  0.0005,
		DefaultFeeRateAShareSell: 0.0009,
		DefaultFeeRateHKBuy:      0.0011,
		DefaultFeeRateHKSell:     0.0012,
		UpdatedAt:                time.Now().UTC(),
	}
	if err := repo.UpsertInvestmentProfile(ctx, updated); err != nil {
		t.Fatalf("UpsertInvestmentProfile update failed: %v", err)
	}

	found2, _ := repo.GetInvestmentProfile(ctx, "profile-user")
	if found2.TotalCapital != 200000 {
		t.Errorf("expected updated TotalCapital=200000, got %v", found2.TotalCapital)
	}
	if found2.MaxDrawdownPct != 25.0 {
		t.Errorf("expected updated MaxDrawdownPct=25.0, got %v", found2.MaxDrawdownPct)
	}
	if found2.DefaultFeeRateAShareBuy != 0.0005 || found2.DefaultFeeRateAShareSell != 0.0009 {
		t.Errorf("expected updated A股 default fee rates, got buy=%v sell=%v", found2.DefaultFeeRateAShareBuy, found2.DefaultFeeRateAShareSell)
	}
	if found2.DefaultFeeRateHKBuy != 0.0011 || found2.DefaultFeeRateHKSell != 0.0012 {
		t.Errorf("expected updated 港股 default fee rates, got buy=%v sell=%v", found2.DefaultFeeRateHKBuy, found2.DefaultFeeRateHKSell)
	}
}
