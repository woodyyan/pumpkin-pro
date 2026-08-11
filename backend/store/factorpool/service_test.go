package factorpool

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/woodyyan/pumpkin-pro/backend/store/factorindex"
	"github.com/woodyyan/pumpkin-pro/backend/store/factorlab"
	"github.com/woodyyan/pumpkin-pro/backend/store/live"
	"github.com/woodyyan/pumpkin-pro/backend/tests/testutil"
)

type fakeSnapshotProvider struct {
	items []live.DetailedSymbolSnapshot
	err   error
	calls int
}

func (p *fakeSnapshotProvider) GetDetailedSnapshots(_ context.Context, _ []string) ([]live.DetailedSymbolSnapshot, error) {
	p.calls++
	return p.items, p.err
}

func setupPoolService(t *testing.T, provider SnapshotProvider) (*Service, *factorindex.Repository) {
	t.Helper()
	db := testutil.InMemoryDB(t)
	if err := factorlab.NewMigrator().AutoMigrate(db); err != nil {
		t.Fatalf("migrate factorlab: %v", err)
	}
	if err := factorindex.NewMigrator().AutoMigrate(db); err != nil {
		t.Fatalf("migrate factorindex: %v", err)
	}
	repo := factorindex.NewRepository(db)
	if err := repo.EnsureDefaultDefinitions(context.Background()); err != nil {
		t.Fatalf("seed definitions: %v", err)
	}
	return NewService(repo, provider), repo
}

func seedCurrentPool(t *testing.T, repo *factorindex.Repository) {
	t.Helper()
	now := time.Now().UTC()
	definitions, err := repo.ListActiveDefinitions(context.Background(), factorindex.ExchangeAShare)
	if err != nil {
		t.Fatalf("list definitions: %v", err)
	}
	for definitionIndex, definition := range definitions {
		rebalanceID := fmt.Sprintf("%s_20260803", definition.ID)
		rebalance := factorindex.Rebalance{
			ID:                 rebalanceID,
			IndexID:            definition.ID,
			SignalDate:         "2026-08-03",
			SourceTradeDate:    "2026-08-03",
			EffectiveStartDate: "2026-08-04",
			ConstituentCount:   50,
			Status:             factorindex.StatusCompleted,
			ComputedAt:         now,
			CreatedAt:          now,
			UpdatedAt:          now,
		}
		if err := repo.SaveRebalance(context.Background(), rebalance, makeConstituents(definition, rebalanceID, now)); err != nil {
			t.Fatalf("seed rebalance %s: %v", definition.ID, err)
		}
		if err := repo.SaveDaily(context.Background(), factorindex.Daily{
			IndexID:          definition.ID,
			TradeDate:        "2026-08-10",
			SourceTradeDate:  "2026-08-10",
			RebalanceID:      rebalanceID,
			NAV:              1000,
			ConstituentCount: 50,
			Status:           factorindex.StatusCompleted,
			ComputedAt:       now,
			CreatedAt:        now,
			UpdatedAt:        now,
		}); err != nil {
			t.Fatalf("seed daily %d: %v", definitionIndex, err)
		}
	}
}

func makeConstituents(definition factorindex.Definition, rebalanceID string, now time.Time) []factorindex.Constituent {
	items := make([]factorindex.Constituent, 0, 12)
	for rank := 1; rank <= 12; rank++ {
		code := fmt.Sprintf("%06d", 600000+rank)
		items = append(items, factorindex.Constituent{
			RebalanceID:      rebalanceID,
			IndexID:          definition.ID,
			StockCode:        code,
			StockName:        fmt.Sprintf("样本股%d", rank),
			Exchange:         "SSE",
			Rank:             rank,
			FactorScore:      float64(100-rank) + 0.5,
			Weight:           0.02,
			SignalClosePrice: float64(10 + rank),
			Industry:         "测试行业",
			CreatedAt:        now,
			UpdatedAt:        now,
		})
	}
	return items
}

func TestGetCurrentBuildsSevenListsAndLimitsEachToTopTen(t *testing.T) {
	provider := &fakeSnapshotProvider{items: []live.DetailedSymbolSnapshot{{
		Symbol: "600001.SH", LastPrice: 12.34, ChangeRate: 0.015, Turnover: 123456789, TS: "2026-08-11T06:30:00Z",
	}}}
	service, repo := setupPoolService(t, provider)
	seedCurrentPool(t, repo)

	payload, err := service.GetCurrent(context.Background(), "")
	if err != nil {
		t.Fatalf("get current: %v", err)
	}
	if payload.Exchange != factorindex.ExchangeAShare {
		t.Fatalf("expected ASHARE, got %s", payload.Exchange)
	}
	if payload.SourceTradeDate != "2026-08-10" {
		t.Fatalf("expected source trade date 2026-08-10, got %s", payload.SourceTradeDate)
	}
	if len(payload.Lists) != 7 {
		t.Fatalf("expected seven factor lists, got %d", len(payload.Lists))
	}
	if payload.Lists[0].FactorKey != "value" {
		t.Fatalf("expected value first, got %s", payload.Lists[0].FactorKey)
	}
	if len(payload.Lists[0].Items) != 10 {
		t.Fatalf("expected top ten items, got %d", len(payload.Lists[0].Items))
	}
	if payload.Lists[0].Items[0].Rank != 1 || payload.Lists[0].Items[9].Rank != 10 {
		t.Fatalf("expected ranks 1 through 10, got first=%d last=%d", payload.Lists[0].Items[0].Rank, payload.Lists[0].Items[9].Rank)
	}
	quote := payload.Lists[0].Items[0].Quote
	if quote.Status != "live" || quote.LastPrice == nil || *quote.LastPrice != 12.34 {
		t.Fatalf("expected quote enrichment, got %+v", quote)
	}
	if provider.calls != 1 {
		t.Fatalf("expected one batched quote request, got %d", provider.calls)
	}
}

func TestGetCurrentKeepsMonthlyFactsWhenQuotesFail(t *testing.T) {
	provider := &fakeSnapshotProvider{err: fmt.Errorf("quote source unavailable")}
	service, repo := setupPoolService(t, provider)
	seedCurrentPool(t, repo)

	payload, err := service.GetCurrent(context.Background(), factorindex.ExchangeAShare)
	if err != nil {
		t.Fatalf("get current should degrade on quote failure: %v", err)
	}
	if payload.QuoteStatus != "unavailable" {
		t.Fatalf("expected unavailable quote status, got %s", payload.QuoteStatus)
	}
	if len(payload.Lists) != 7 || len(payload.Lists[0].Items) != 10 {
		t.Fatalf("monthly factor facts should remain available: %+v", payload.Lists)
	}
	if payload.Lists[0].Items[0].Quote.Status != "unavailable" {
		t.Fatalf("expected unavailable item quote, got %+v", payload.Lists[0].Items[0].Quote)
	}
}
