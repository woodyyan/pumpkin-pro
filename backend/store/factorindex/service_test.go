package factorindex

import (
	"context"
	"fmt"
	"math"
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

func seedFactorMarketMetric(t *testing.T, repo *Repository, code, tradeDate string, isSuspended bool) {
	t.Helper()
	metric := factorlab.FactorMarketMetric{
		Code:        code,
		TradeDate:   tradeDate,
		ClosePrice:  0,
		Volume:      0,
		IsSuspended: isSuspended,
		Source:      "test",
		UpdatedAt:   time.Now().UTC(),
	}
	if err := repo.db.WithContext(context.Background()).Create(&metric).Error; err != nil {
		t.Fatalf("seed market metric %s %s: %v", code, tradeDate, err)
	}
}

func valueIndexDaily(t *testing.T, repo *Repository, tradeDate string) *Daily {
	t.Helper()
	row, err := repo.GetDailyByTradeDate(context.Background(), "fi_value_ashare", tradeDate)
	if err != nil {
		t.Fatalf("get daily %s: %v", tradeDate, err)
	}
	return row
}

// TestSyncDailyRecomputesPartialRowsAfterBarsCatchUp 验证：日线补齐后再次同步，
// 之前的 partial 行会被重算为 completed，而不是永久跳过。
func TestSyncDailyRecomputesPartialRowsAfterBarsCatchUp(t *testing.T) {
	svc, repo := setupFactorIndexService(t)
	seedFactorIndexDataset(t, repo, true) // 600001 缺 2026-06-03 日线

	if err := svc.SyncAll(context.Background()); err != nil {
		t.Fatalf("first sync all: %v", err)
	}
	first := valueIndexDaily(t, repo, "2026-06-03")
	if first == nil || first.Status != StatusPartial {
		t.Fatalf("expected partial row on first sync, got %+v", first)
	}

	// 补齐缺失日线，模拟上游数据晚到
	now := time.Now().UTC()
	bar := factorlab.FactorDailyBar{Code: "600001", TradeDate: "2026-06-03", Close: 103, Open: 103, High: 103, Low: 103, Adjusted: "qfq", Source: "test", UpdatedAt: now}
	if err := repo.db.WithContext(context.Background()).Create(&bar).Error; err != nil {
		t.Fatalf("backfill bar: %v", err)
	}
	if err := svc.SyncAll(context.Background()); err != nil {
		t.Fatalf("second sync all: %v", err)
	}
	second := valueIndexDaily(t, repo, "2026-06-03")
	if second == nil {
		t.Fatal("expected daily row after recompute")
	}
	if second.Status != StatusCompleted {
		t.Fatalf("expected completed after bars catch up, got %s", second.Status)
	}
	if second.ValidPriceCount != 50 {
		t.Fatalf("expected 50 valid prices after catch up, got %d", second.ValidPriceCount)
	}
	if second.NAV <= first.NAV {
		t.Fatalf("expected NAV to increase after including missing stock return, before=%v after=%v", first.NAV, second.NAV)
	}
}

// TestSyncDailyCascadeRecomputesLaterCompletedRows 验证：中间日 partial 行被重算时，
// 其后基于污染 NAV 链的 completed 行也会被级联重算。
func TestSyncDailyCascadeRecomputesLaterCompletedRows(t *testing.T) {
	svc, repo := setupFactorIndexService(t)
	now := time.Now().UTC()
	// 50 只股票，06-01 ~ 06-04 四天日线；600001 缺 06-03。
	for idx := 1; idx <= 50; idx++ {
		code := fmt.Sprintf("600%03d", idx)
		security := factorlab.FactorSecurity{Code: code, Symbol: code + ".SH", Name: fmt.Sprintf("样本股%d", idx), Exchange: "SSE", Board: factorlab.BoardMain, IsActive: true, Source: "test", UpdatedAt: now}
		if err := repo.db.WithContext(context.Background()).Create(&security).Error; err != nil {
			t.Fatalf("seed security: %v", err)
		}
		score := float64(100 - idx)
		scoreRow := factorlab.FactorScore{SnapshotDate: "2026-06-01", Code: code, Symbol: code + ".SH", Name: fmt.Sprintf("样本股%d", idx), ClosePrice: float64(100 + idx), ValueScore: floatPtr(score), CreatedAt: now}
		if err := repo.db.WithContext(context.Background()).Create(&scoreRow).Error; err != nil {
			t.Fatalf("seed score: %v", err)
		}
		for dayIdx, date := range []string{"2026-06-01", "2026-06-02", "2026-06-03", "2026-06-04"} {
			if idx == 1 && date == "2026-06-03" {
				continue
			}
			price := float64(100 + idx + dayIdx)
			bar := factorlab.FactorDailyBar{Code: code, TradeDate: date, Close: price, Open: price, High: price, Low: price, Adjusted: "qfq", Source: "test", UpdatedAt: now}
			if err := repo.db.WithContext(context.Background()).Create(&bar).Error; err != nil {
				t.Fatalf("seed bar: %v", err)
			}
		}
	}

	if err := svc.SyncAll(context.Background()); err != nil {
		t.Fatalf("first sync all: %v", err)
	}
	partial0603 := valueIndexDaily(t, repo, "2026-06-03")
	if partial0603 == nil || partial0603.Status != StatusPartial {
		t.Fatalf("expected partial 06-03, got %+v", partial0603)
	}
	polluted0604 := valueIndexDaily(t, repo, "2026-06-04")
	if polluted0604 == nil || polluted0604.Status != StatusCompleted {
		t.Fatalf("expected completed 06-04, got %+v", polluted0604)
	}

	// 补齐 600001@06-03（收盘 103），再次同步
	bar := factorlab.FactorDailyBar{Code: "600001", TradeDate: "2026-06-03", Close: 103, Open: 103, High: 103, Low: 103, Adjusted: "qfq", Source: "test", UpdatedAt: now}
	if err := repo.db.WithContext(context.Background()).Create(&bar).Error; err != nil {
		t.Fatalf("backfill bar: %v", err)
	}
	if err := svc.SyncAll(context.Background()); err != nil {
		t.Fatalf("second sync all: %v", err)
	}

	fixed0603 := valueIndexDaily(t, repo, "2026-06-03")
	if fixed0603 == nil || fixed0603.Status != StatusCompleted {
		t.Fatalf("expected completed 06-03 after catch up, got %+v", fixed0603)
	}
	// 06-03 期望日收益：每只股票涨 1/(101+idx)，等权 2%
	expectedReturn0603 := 0.0
	for idx := 1; idx <= 50; idx++ {
		expectedReturn0603 += 0.02 * (1.0 / float64(101+idx))
	}
	if math.Abs(fixed0603.DailyReturn-roundFloat(expectedReturn0603, 6)) > 1e-6 {
		t.Fatalf("unexpected 06-03 daily return: got %v want %v", fixed0603.DailyReturn, expectedReturn0603)
	}

	fixed0604 := valueIndexDaily(t, repo, "2026-06-04")
	if fixed0604 == nil || fixed0604.Status != StatusCompleted {
		t.Fatalf("expected recomputed completed 06-04, got %+v", fixed0604)
	}
	// 06-04 期望日收益：600001 窗口为 [06-04, 06-03]，收益 104/103-1；
	// 其余股票收益 1/(102+idx)。
	expectedReturn0604 := 0.02 * (104.0/103.0 - 1.0)
	for idx := 2; idx <= 50; idx++ {
		expectedReturn0604 += 0.02 * (1.0 / float64(102+idx))
	}
	if math.Abs(fixed0604.DailyReturn-roundFloat(expectedReturn0604, 6)) > 1e-6 {
		t.Fatalf("06-04 was not cascade-recomputed: got %v want %v (polluted row had %v)", fixed0604.DailyReturn, expectedReturn0604, polluted0604.DailyReturn)
	}
	// NAV 链应从修复后的 06-03 继续推进
	expectedNAV0604 := roundFloat(fixed0603.NAV*(1+roundFloat(expectedReturn0604, 6)), 6)
	if math.Abs(fixed0604.NAV-expectedNAV0604) > 1e-4 {
		t.Fatalf("06-04 NAV not chained from fixed 06-03: got %v want ~%v", fixed0604.NAV, expectedNAV0604)
	}
}

// TestSyncDailySkipsTradeDateBelowCoverageRatio 验证：单日日线覆盖数低于基线比例时
// 跳过该日计算（等待补齐），调低门槛后可正常计算。
func TestSyncDailySkipsTradeDateBelowCoverageRatio(t *testing.T) {
	svc, repo := setupFactorIndexService(t)
	seedFactorIndexDataset(t, repo, false)
	// 删除 06-03 的 45 只股票日线，使当日覆盖 5/50 = 10% < 90% 门槛
	if err := repo.db.WithContext(context.Background()).Exec(
		"DELETE FROM factor_daily_bars WHERE trade_date = '2026-06-03' AND code <> '600001' AND code <> '600002' AND code <> '600003' AND code <> '600004' AND code <> '600005'",
	).Error; err != nil {
		t.Fatalf("delete bars: %v", err)
	}

	if err := svc.SyncAll(context.Background()); err != nil {
		t.Fatalf("sync all: %v", err)
	}
	if row := valueIndexDaily(t, repo, "2026-06-02"); row == nil {
		t.Fatal("expected 06-02 row to be computed")
	}
	if row := valueIndexDaily(t, repo, "2026-06-03"); row != nil {
		t.Fatalf("expected 06-03 to be skipped by coverage gate, got %+v", row)
	}

	// 调低门槛后应可计算（落库为 partial，45 只缺数）
	svc.MinBarCoverageRatio = 0.05
	if err := svc.SyncAll(context.Background()); err != nil {
		t.Fatalf("sync all with lower ratio: %v", err)
	}
	row := valueIndexDaily(t, repo, "2026-06-03")
	if row == nil {
		t.Fatal("expected 06-03 row after lowering coverage ratio")
	}
	if row.Status != StatusPartial || row.ValidPriceCount != 5 {
		t.Fatalf("expected partial with 5 valid prices, got status=%s valid=%d", row.Status, row.ValidPriceCount)
	}
}

// TestComputeDailySeparatesSuspendedFromMissing 验证停牌/退市股与数据缺失分级：
// 停牌股按 0 收益但不触发 partial；数据缺失仍触发 partial 并保留警告。
func TestComputeDailySeparatesSuspendedFromMissing(t *testing.T) {
	t.Run("suspended and missing are classified separately", func(t *testing.T) {
		svc, repo := setupFactorIndexService(t)
		seedFactorIndexDataset(t, repo, false)
		// 600001 停牌（无 06-03 日线 + metrics 停牌标记）；600002 缺数（无日线、无 metrics）
		if err := repo.db.WithContext(context.Background()).Exec("DELETE FROM factor_daily_bars WHERE trade_date = '2026-06-03' AND code IN ('600001','600002')").Error; err != nil {
			t.Fatalf("delete bars: %v", err)
		}
		seedFactorMarketMetric(t, repo, "600001", "2026-06-03", true)

		if err := svc.SyncAll(context.Background()); err != nil {
			t.Fatalf("sync all: %v", err)
		}
		row := valueIndexDaily(t, repo, "2026-06-03")
		if row == nil {
			t.Fatal("expected 06-03 row")
		}
		if row.Status != StatusPartial {
			t.Fatalf("expected partial (1 missing), got %s", row.Status)
		}
		if row.ValidPriceCount != 48 || row.SuspendedPriceCount != 1 {
			t.Fatalf("expected valid=48 suspended=1, got valid=%d suspended=%d", row.ValidPriceCount, row.SuspendedPriceCount)
		}
		if !strings.Contains(row.WarningText, "1/50 只成分股缺少完整日线，按 0 收益处理") {
			t.Fatalf("expected missing warning, got %q", row.WarningText)
		}
		if !strings.Contains(row.WarningText, "1/50 只成分股停牌或已退市，按 0 收益处理") {
			t.Fatalf("expected suspended warning, got %q", row.WarningText)
		}
	})

	t.Run("only suspended stays completed with informational warning", func(t *testing.T) {
		svc, repo := setupFactorIndexService(t)
		seedFactorIndexDataset(t, repo, false)
		if err := repo.db.WithContext(context.Background()).Exec("DELETE FROM factor_daily_bars WHERE trade_date = '2026-06-03' AND code = '600001'").Error; err != nil {
			t.Fatalf("delete bar: %v", err)
		}
		seedFactorMarketMetric(t, repo, "600001", "2026-06-03", true)

		if err := svc.SyncAll(context.Background()); err != nil {
			t.Fatalf("sync all: %v", err)
		}
		row := valueIndexDaily(t, repo, "2026-06-03")
		if row == nil {
			t.Fatal("expected 06-03 row")
		}
		if row.Status != StatusCompleted {
			t.Fatalf("expected completed (suspension is not a data gap), got %s", row.Status)
		}
		if row.ValidPriceCount != 49 || row.SuspendedPriceCount != 1 {
			t.Fatalf("expected valid=49 suspended=1, got valid=%d suspended=%d", row.ValidPriceCount, row.SuspendedPriceCount)
		}
		if row.WarningText != "1/50 只成分股停牌或已退市，按 0 收益处理" {
			t.Fatalf("unexpected warning text: %q", row.WarningText)
		}
	})

	t.Run("delisted stock classified via latest prior metrics row", func(t *testing.T) {
		svc, repo := setupFactorIndexService(t)
		seedFactorIndexDataset(t, repo, false)
		// 退市股特征：06-02 起 metrics 标记停牌，此后数据源停止推送快照与日线
		if err := repo.db.WithContext(context.Background()).Exec("DELETE FROM factor_daily_bars WHERE trade_date = '2026-06-03' AND code = '600001'").Error; err != nil {
			t.Fatalf("delete bar: %v", err)
		}
		seedFactorMarketMetric(t, repo, "600001", "2026-06-02", true)

		if err := svc.SyncAll(context.Background()); err != nil {
			t.Fatalf("sync all: %v", err)
		}
		row := valueIndexDaily(t, repo, "2026-06-03")
		if row == nil {
			t.Fatal("expected 06-03 row")
		}
		if row.Status != StatusCompleted || row.SuspendedPriceCount != 1 {
			t.Fatalf("expected completed with suspended=1, got status=%s suspended=%d", row.Status, row.SuspendedPriceCount)
		}
	})
}
