package portfolio

import (
	"context"
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
		InvestmentProfileRecord{},
	)
	repo := NewRepository(db)
	return NewService(repo), context.Background()
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
		TotalCapital:      500000,
		RiskPreference:    "conservative",
		InvestmentGoal:    "income",
		InvestmentHorizon: "long",
		MaxDrawdownPct:    10.0,
		ExperienceLevel:   "beginner",
		Note:              "conservative investor",
	})
	if err != nil {
		t.Fatalf("UpsertInvestmentProfile failed: %v", err)
	}
	if prof.TotalCapital != 500000 {
		t.Errorf("expected TotalCapital 500000, got %v", prof.TotalCapital)
	}

	got, err := svc.GetInvestmentProfile(ctx, "inv-user")
	if err != nil {
		t.Fatalf("GetInvestmentProfile failed: %v", err)
	}
	if got.InvestmentGoal != "income" {
		t.Errorf("expected investment goal 'income', got %s", got.InvestmentGoal)
	}
}
