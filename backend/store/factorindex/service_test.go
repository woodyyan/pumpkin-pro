package factorindex

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/woodyyan/pumpkin-pro/backend/store/factorlab"
	"github.com/woodyyan/pumpkin-pro/backend/tests/testutil"
)

func floatPtr(v float64) *float64 { return &v }

func setupFactorIndexService(t *testing.T) (*Service, *Repository) {
	t.Helper()
	db := testutil.InMemoryDB(t)
	if err := factorlab.NewMigrator().AutoMigrate(db); err != nil {
		t.Fatalf("migrate factorlab: %v", err)
	}
	if err := NewMigrator().AutoMigrate(db); err != nil {
		t.Fatalf("migrate factorindex: %v", err)
	}
	repo := NewRepository(db)
	return NewService(repo), repo
}

func seedFactorIndexDataset(t *testing.T, repo *Repository, missingThirdDay bool) {
	t.Helper()
	now := time.Now().UTC()
	securities := make([]factorlab.FactorSecurity, 0, 50)
	scores := make([]factorlab.FactorScore, 0, 50)
	bars := make([]factorlab.FactorDailyBar, 0, 150)
	for idx := 1; idx <= 50; idx++ {
		code := fmt.Sprintf("600%03d", idx)
		name := fmt.Sprintf("样本股%d", idx)
		securities = append(securities, factorlab.FactorSecurity{
			Code:      code,
			Symbol:    code + ".SH",
			Name:      name,
			Exchange:  "SSE",
			Board:     factorlab.BoardMain,
			IsActive:  true,
			Source:    "test",
			UpdatedAt: now,
		})
		score := float64(100 - idx)
		scores = append(scores, factorlab.FactorScore{
			SnapshotDate:       "2026-06-01",
			Code:               code,
			Symbol:             code + ".SH",
			Name:               name,
			Industry:           "测试行业",
			ClosePrice:         float64(100 + idx),
			ValueScore:         floatPtr(score),
			DividendYieldScore: floatPtr(score - 1),
			GrowthScore:        floatPtr(score - 2),
			QualityScore:       floatPtr(score - 3),
			MomentumScore:      floatPtr(score - 4),
			SizeScore:          floatPtr(score - 5),
			LowVolatilityScore: floatPtr(score - 6),
			CreatedAt:          now,
		})
		bars = append(bars,
			factorlab.FactorDailyBar{Code: code, TradeDate: "2026-06-01", Close: float64(100 + idx), Open: float64(100 + idx), High: float64(100 + idx), Low: float64(100 + idx), Adjusted: "qfq", Source: "test", UpdatedAt: now},
			factorlab.FactorDailyBar{Code: code, TradeDate: "2026-06-02", Close: float64(101 + idx), Open: float64(101 + idx), High: float64(101 + idx), Low: float64(101 + idx), Adjusted: "qfq", Source: "test", UpdatedAt: now},
		)
		if !(missingThirdDay && idx == 1) {
			bars = append(bars, factorlab.FactorDailyBar{Code: code, TradeDate: "2026-06-03", Close: float64(102 + idx), Open: float64(102 + idx), High: float64(102 + idx), Low: float64(102 + idx), Adjusted: "qfq", Source: "test", UpdatedAt: now})
		}
	}
	if err := repo.db.WithContext(context.Background()).Create(&securities).Error; err != nil {
		t.Fatalf("seed securities: %v", err)
	}
	if err := repo.db.WithContext(context.Background()).Create(&scores).Error; err != nil {
		t.Fatalf("seed scores: %v", err)
	}
	if err := repo.db.WithContext(context.Background()).Create(&bars).Error; err != nil {
		t.Fatalf("seed bars: %v", err)
	}
}

func TestFactorIndexSyncAllBuildsOverview(t *testing.T) {
	svc, repo := setupFactorIndexService(t)
	seedFactorIndexDataset(t, repo, false)

	if err := svc.SyncAll(context.Background()); err != nil {
		t.Fatalf("sync all: %v", err)
	}
	overview, err := svc.GetOverview(context.Background(), ExchangeAShare)
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if overview.SourceTradeDate != "2026-06-03" {
		t.Fatalf("expected latest source trade date 2026-06-03, got %s", overview.SourceTradeDate)
	}
	if len(overview.Items) != 7 {
		t.Fatalf("expected 7 factor index cards, got %d", len(overview.Items))
	}
	first := overview.Items[0]
	if first.FactorKey != "value" {
		t.Fatalf("expected first factor key value, got %s", first.FactorKey)
	}
	if first.ConstituentCount != 50 {
		t.Fatalf("expected 50 constituents, got %d", first.ConstituentCount)
	}
	if first.NAV == nil || *first.NAV <= defaultBaseNAV {
		t.Fatalf("expected nav above base, got %+v", first.NAV)
	}
	if first.Status != StatusCompleted {
		t.Fatalf("expected completed status, got %s", first.Status)
	}
	if len(first.TrendPoints) != 2 {
		t.Fatalf("expected two trend points, got %d", len(first.TrendPoints))
	}
}

func TestFactorIndexSyncAllMarksPartialWhenDailyBarMissing(t *testing.T) {
	svc, repo := setupFactorIndexService(t)
	seedFactorIndexDataset(t, repo, true)

	if err := svc.SyncAll(context.Background()); err != nil {
		t.Fatalf("sync all: %v", err)
	}
	overview, err := svc.GetOverview(context.Background(), ExchangeAShare)
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	valueItem := overview.Items[0]
	if valueItem.Status != StatusPartial {
		t.Fatalf("expected partial status, got %s", valueItem.Status)
	}
	if !strings.Contains(valueItem.WarningText, "按 0 收益处理") {
		t.Fatalf("expected warning text for missing price, got %q", valueItem.WarningText)
	}
	if valueItem.NAV == nil || *valueItem.NAV <= 0 {
		t.Fatalf("expected nav to remain positive, got %+v", valueItem.NAV)
	}
}

// TestListTopScoresExcludesDelistedAndSTStocks verifies that ListTopScores does
// not select stocks that are marked is_active=0 (delisted) or is_st=1 (ST),
// even when their factor scores are very high.
//
// Note: SQLite stores Go bool false as NULL via GORM's default path, so we
// seed is_active=0 and is_st=1 via raw SQL to match the actual production
// storage format (same as the UPDATE fix applied to live DB).
func TestListTopScoresExcludesDelistedAndSTStocks(t *testing.T) {
	_, repo := setupFactorIndexService(t)
	now := time.Now().UTC()

	// Seed 48 normal active stocks with descending scores via GORM (is_active=1)
	scores := make([]factorlab.FactorScore, 0, 50)
	for idx := 1; idx <= 48; idx++ {
		code := fmt.Sprintf("600%03d", idx)
		if err := repo.db.WithContext(context.Background()).Exec(
			`INSERT INTO factor_securities (code,symbol,name,exchange,board,is_active,is_st,source,updated_at) VALUES (?,?,?,?,?,1,0,?,?)`,
			code, code+".SH", fmt.Sprintf("样本股%d", idx), "SSE", factorlab.BoardMain, "test", now,
		).Error; err != nil {
			t.Fatalf("seed security %s: %v", code, err)
		}
		score := float64(50 - idx)
		scores = append(scores, factorlab.FactorScore{
			SnapshotDate: "2026-06-01", Code: code, Symbol: code + ".SH",
			Name: fmt.Sprintf("样本股%d", idx), ClosePrice: float64(100 + idx),
			ValueScore: floatPtr(score), CreatedAt: now,
		})
	}

	// Seed 1 delisted stock with the highest score (is_active=0) — must NOT be selected
	const delistedCode = "600599"
	if err := repo.db.WithContext(context.Background()).Exec(
		`INSERT INTO factor_securities (code,symbol,name,exchange,board,is_active,is_st,source,updated_at) VALUES (?,?,?,?,?,0,0,?,?)`,
		delistedCode, delistedCode+".SH", "退市熊猫", "SSE", factorlab.BoardMain, "test", now,
	).Error; err != nil {
		t.Fatalf("seed delisted security: %v", err)
	}
	scores = append(scores, factorlab.FactorScore{
		SnapshotDate: "2026-06-01", Code: delistedCode, Symbol: delistedCode + ".SH",
		Name: "退市熊猫", ClosePrice: 0.44,
		ValueScore: floatPtr(99.0), // artificially high score
		CreatedAt:  now,
	})

	// Seed 1 ST stock with the second highest score (is_st=1) — must NOT be selected
	const stCode = "000001"
	if err := repo.db.WithContext(context.Background()).Exec(
		`INSERT INTO factor_securities (code,symbol,name,exchange,board,is_active,is_st,source,updated_at) VALUES (?,?,?,?,?,1,1,?,?)`,
		stCode, stCode+".SZ", "*ST 样本", "SZSE", factorlab.BoardMain, "test", now,
	).Error; err != nil {
		t.Fatalf("seed ST security: %v", err)
	}
	scores = append(scores, factorlab.FactorScore{
		SnapshotDate: "2026-06-01", Code: stCode, Symbol: stCode + ".SZ",
		Name: "*ST 样本", ClosePrice: 3.5,
		ValueScore: floatPtr(98.0), // second highest, still must be excluded
		CreatedAt:  now,
	})

	if err := repo.db.WithContext(context.Background()).Create(&scores).Error; err != nil {
		t.Fatalf("seed scores: %v", err)
	}

	rows, err := repo.ListTopScores(context.Background(), "2026-06-01", "value_score", 50)
	if err != nil {
		t.Fatalf("ListTopScores: %v", err)
	}

	// Only the 48 normal stocks should be returned (delisted + ST excluded)
	if len(rows) != 48 {
		t.Fatalf("expected 48 rows (delisted+ST excluded), got %d", len(rows))
	}
	for _, row := range rows {
		if row.Code == delistedCode {
			t.Errorf("delisted stock %s must not appear in top scores", delistedCode)
		}
		if row.Code == stCode {
			t.Errorf("ST stock %s must not appear in top scores", stCode)
		}
	}
}
