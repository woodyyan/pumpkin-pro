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
		InvestmentProfileRecord{},
	)
	return NewRepository(db), context.Background()
}

func TestCreateAndGetPortfolioItem(t *testing.T) {
	repo, ctx := setupPortfolioDB(t)

	record := &PortfolioRecord{
		ID:           "pf-001",
		UserID:       "user-1",
		Symbol:       "000001",
		Shares:       1000,
		AvgCostPrice: 12.5,
		BuyDate:      "2025-01-15",
		Note:         "测试持仓",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	err := repo.Upsert(ctx, record)
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	found, err := repo.GetBySymbol(ctx, "user-1", "000001")
	if err != nil {
		t.Fatalf("GetBySymbol failed: %v", err)
	}
	if found.Shares != 1000 {
		t.Errorf("expected Shares=1000, got %v", found.Shares)
	}
	if found.Symbol != "000001" {
		t.Errorf("expected Symbol=000001, got %s", found.Symbol)
	}
}

func TestUpsertUpdatesExisting(t *testing.T) {
	repo, ctx := setupPortfolioDB(t)

	// Create
	repo.Upsert(ctx, &PortfolioRecord{
		ID: "pf-upsert", UserID: "user-2", Symbol: "600036",
		Shares: 500, AvgCostPrice: 10.0,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	})

	// Update with new values
	err := repo.Upsert(ctx, &PortfolioRecord{
		ID: "pf-upsert-new-id", UserID: "user-2", Symbol: "600036",
		Shares: 2000, AvgCostPrice: 11.5, BuyDate: "2025-02-01",
		Note: "updated note", UpdatedAt: time.Now().UTC(),
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
	if found.BuyDate != "2025-02-01" {
		t.Errorf("expected BuyDate=2025-02-01, got %s", found.BuyDate)
	}
}

func TestDeletePortfolio(t *testing.T) {
	repo, ctx := setupPortfolioDB(t)

	repo.Upsert(ctx, &PortfolioRecord{
		ID: "pf-del", UserID: "user-3", Symbol: "300750",
		Shares: 100, AvgCostPrice: 50.0,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	})

	err := repo.Delete(ctx, "user-3", "300750")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = repo.GetBySymbol(ctx, "user-3", "300750")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestDeleteNonexistent(t *testing.T) {
	repo, ctx := setupPortfolioDB(t)

	err := repo.Delete(ctx, "user-noexist", "NOEXIST")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound for nonexistent item, got %v", err)
	}
}

func TestListByUser(t *testing.T) {
	repo, ctx := setupPortfolioDB(t)

	// Create multiple items for same user
	for i, sym := range []string{"000001", "600036", "300750"} {
		repo.Upsert(ctx, &PortfolioRecord{
			ID: "pf-list-" + string(rune('A'+i)),
			UserID: "user-list",
			Symbol: sym,
			Shares:   float64(100 * (i + 1)),
			AvgCostPrice: float64(10.0 + float64(i)),
			CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
		})
	}

	items, err := repo.ListByUser(ctx, "user-list")
	if err != nil {
		t.Fatalf("ListByUser failed: %v", err)
	}
	if len(items) != 3 {
		t.Errorf("expected 3 items, got %d", len(items))
	}
}

func TestInvestmentProfileCRUD(t *testing.T) {
	repo, ctx := setupPortfolioDB(t)

	// Create
	record := &InvestmentProfileRecord{
		UserID:            "profile-user",
		TotalCapital:      100000,
		RiskPreference:    "moderate",
		InvestmentGoal:    "growth",
		InvestmentHorizon: "medium",
		MaxDrawdownPct:    15.0,
		ExperienceLevel:   "intermediate",
		Note:              "test profile",
		UpdatedAt:         time.Now().UTC(),
	}
	err := repo.UpsertInvestmentProfile(ctx, record)
	if err != nil {
		t.Fatalf("UpsertInvestmentProfile create failed: %v", err)
	}

	// Get
	found, err := repo.GetInvestmentProfile(ctx, "profile-user")
	if err != nil {
		t.Fatalf("GetInvestmentProfile failed: %v", err)
	}
	if found.TotalCapital != 100000 {
		t.Errorf("expected TotalCapital=100000, got %v", found.TotalCapital)
	}
	if found.RiskPreference != "moderate" {
		t.Errorf("expected RiskPreference=moderate, got %s", found.RiskPreference)
	}

	// Update (upsert again with different data)
	updated := &InvestmentProfileRecord{
		UserID:            "profile-user",
		TotalCapital:      200000,
		RiskPreference:    "aggressive",
		MaxDrawdownPct:    25.0,
		UpdatedAt:         time.Now().UTC(),
	}
	err = repo.UpsertInvestmentProfile(ctx, updated)
	if err != nil {
		t.Fatalf("UpsertInvestmentProfile update failed: %v", err)
	}

	found2, _ := repo.GetInvestmentProfile(ctx, "profile-user")
	if found2.TotalCapital != 200000 {
		t.Errorf("expected updated TotalCapital=200000, got %v", found2.TotalCapital)
	}
	if found2.MaxDrawdownPct != 25.0 {
		t.Errorf("expected updated MaxDrawdownPct=25.0, got %v", found2.MaxDrawdownPct)
	}
}
