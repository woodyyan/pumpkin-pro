package portfolio

import (
	"context"
	"testing"

	"github.com/woodyyan/pumpkin-pro/backend/tests/testutil"
)

func setupPortfolioService(t *testing.T) (*Service, context.Context) {
	t.Helper()
	db := testutil.InMemoryDB(t)
	testutil.AutoMigrateModels(t, db,
		PortfolioRecord{},
		InvestmentProfileRecord{},
	)
	repo := NewRepository(db)
	return NewService(repo), context.Background()
}

func TestServiceUpsertAndList(t *testing.T) {
	svc, ctx := setupPortfolioService(t)

	item, err := svc.Upsert(ctx, "svc-user", "000001", UpsertPortfolioInput{
		Shares:       1500,
		AvgCostPrice: 13.8,
		BuyDate:      "2025-03-01",
		Note:         "service test holding",
	})
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}
	if item.Symbol != "000001" {
		t.Errorf("expected symbol 000001, got %s", item.Symbol)
	}
	if item.Shares != 1500 {
		t.Errorf("expected shares 1500, got %v", item.Shares)
	}

	list, err := svc.ListByUser(ctx, "svc-user")
	if err != nil {
		t.Fatalf("ListByUser failed: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 item, got %d", len(list))
	}
}

func TestServiceGetBySymbol(t *testing.T) {
	svc, ctx := setupPortfolioService(t)

	svc.Upsert(ctx, "get-user", "600036", UpsertPortfolioInput{
		Shares: 800, AvgCostPrice: 42.5, BuyDate: "2025-01-10",
	})

	item, err := svc.GetBySymbol(ctx, "get-user", "600036")
	if err != nil {
		t.Fatalf("GetBySymbol failed: %v", err)
	}
	if item.AvgCostPrice != 42.5 {
		t.Errorf("expected AvgCostPrice 42.5, got %v", item.AvgCostPrice)
	}
}

func TestServiceDelete(t *testing.T) {
	svc, ctx := setupPortfolioService(t)

	svc.Upsert(ctx, "del-user", "300750", UpsertPortfolioInput{
		Shares: 200, AvgCostPrice: 55.0,
	})

	err := svc.Delete(ctx, "del-user", "300750")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = svc.GetBySymbol(ctx, "del-user", "300750")
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestServiceValidation(t *testing.T) {
	svc, ctx := setupPortfolioService(t)

	// Negative shares
	_, err := svc.Upsert(ctx, "val-user", "BAD", UpsertPortfolioInput{
		Shares: -10, AvgCostPrice: 10.0,
	})
	if err == nil {
		t.Error("expected error for negative shares")
	}

	// Negative cost price
	_, err = svc.Upsert(ctx, "val-user", "BAD2", UpsertPortfolioInput{
		Shares: 100, AvgCostPrice: -5.0,
	})
	if err == nil {
		t.Error("expected error for negative avg_cost_price")
	}

	// Empty symbol
	_, err = svc.Upsert(ctx, "val-user", "", UpsertPortfolioInput{
		Shares: 100, AvgCostPrice: 10.0,
	})
	if err == nil {
		t.Error("expected error for empty symbol")
	}
}

func TestServiceSymbolUppercase(t *testing.T) {
	svc, ctx := setupPortfolioService(t)

	item, _ := svc.Upsert(ctx, "case-user", "sh601318", UpsertPortfolioInput{
		Shares: 100, AvgCostPrice: 30.0,
	})
	if item.Symbol != "SH601318" {
		t.Errorf("expected uppercase SH601318, got %s", item.Symbol)
	}

	// Should be able to retrieve by the stored uppercase form
	found, err := svc.GetBySymbol(ctx, "case-user", "SH601318")
	if err != nil {
		t.Fatalf("could not find by uppercase symbol: %v", err)
	}
	if found.Symbol != "SH601318" {
		t.Errorf("expected SH601318, got %s", found.Symbol)
	}
}

func TestServiceInvestmentProfile(t *testing.T) {
	svc, ctx := setupPortfolioService(t)

	// Create profile
	prof, err := svc.UpsertInvestmentProfile(ctx, "inv-user", UpsertInvestmentProfileInput{
		TotalCapital:      500000,
		RiskPreference:    "conservative",
		InvestmentGoal:    "income",
		InvestmentHorizon: "long",
		MaxDrawdownPct:    10.0,
		ExperienceLevel:   "beginner",
		Note:             "conservative investor",
	})
	if err != nil {
		t.Fatalf("UpsertInvestmentProfile failed: %v", err)
	}
	if prof.TotalCapital != 500000 {
		t.Errorf("expected TotalCapital 500000, got %v", prof.TotalCapital)
	}

	// Get profile
	got, err := svc.GetInvestmentProfile(ctx, "inv-user")
	if err != nil {
		t.Fatalf("GetInvestmentProfile failed: %v", err)
	}
	if got.InvestmentGoal != "income" {
		t.Errorf("expected investment goal 'income', got %s", got.InvestmentGoal)
	}

	// Update profile
	updated, err := svc.UpsertInvestmentProfile(ctx, "inv-user", UpsertInvestmentProfileInput{
		TotalCapital:      800000,
		RiskPreference:    "aggressive",
		MaxDrawdownPct:    30.0,
	})
	if err != nil {
		t.Fatalf("update UpsertInvestmentProfile failed: %v", err)
	}
	if updated.TotalCapital != 800000 {
		t.Errorf("expected updated TotalCapital 800000, got %v", updated.TotalCapital)
	}
}

func TestServiceProfileValidation(t *testing.T) {
	svc, ctx := setupPortfolioService(t)

	// Negative total capital
	_, err := svc.UpsertInvestmentProfile(ctx, "bad-user", UpsertInvestmentProfileInput{
		TotalCapital: -1000,
	})
	if err == nil {
		t.Error("expected error for negative total_capital")
	}

	// Max drawdown out of range (>100)
	_, err = svc.UpsertInvestmentProfile(ctx, "bad-user", UpsertInvestmentProfileInput{
		TotalCapital:   10000,
		MaxDrawdownPct: 150.0,
	})
	if err == nil {
		t.Error("expected error for max_drawdown_pct > 100")
	}

	// Max drawdown negative
	_, err = svc.UpsertInvestmentProfile(ctx, "bad-user", UpsertInvestmentProfileInput{
		TotalCapital:   10000,
		MaxDrawdownPct: -5.0,
	})
	if err == nil {
		t.Error("expected error for negative max_drawdown_pct")
	}
}
