package factorlab

import (
	"context"
	"testing"
	"time"

	"github.com/woodyyan/pumpkin-pro/backend/tests/testutil"
)

func setupRepo(t *testing.T) *Repository {
	t.Helper()
	db := testutil.InMemoryDB(t)
	if err := NewMigrator().AutoMigrate(db); err != nil {
		t.Fatalf("migrate factorlab models: %v", err)
	}
	return NewRepository(db)
}

func TestBulkUpsertSecuritiesNormalizesAndIsIdempotent(t *testing.T) {
	repo := setupRepo(t)
	ctx := context.Background()

	records := []FactorSecurity{
		{Code: "688001", Name: "科创样本", IsActive: true, Source: "test"},
		{Code: "300001", Name: "创业样本", IsActive: true, Source: "test"},
		{Code: "", Name: "空代码"},
	}
	if err := repo.BulkUpsertSecurities(ctx, records); err != nil {
		t.Fatalf("upsert securities: %v", err)
	}
	if err := repo.BulkUpsertSecurities(ctx, []FactorSecurity{{Code: "300001.SZ", Name: "创业样本更新", IsActive: true, Source: "test2"}}); err != nil {
		t.Fatalf("upsert securities again: %v", err)
	}

	var securities []FactorSecurity
	if err := repo.db.Order("code ASC").Find(&securities).Error; err != nil {
		t.Fatalf("query securities: %v", err)
	}
	if len(securities) != 2 {
		t.Fatalf("expected 2 securities, got %d", len(securities))
	}
	if securities[0].Code != "300001" || securities[0].Board != BoardChiNext || securities[0].Exchange != "SZSE" {
		t.Fatalf("unexpected chinext security: %+v", securities[0])
	}
	if securities[1].Code != "688001" || securities[1].Board != BoardSTAR || securities[1].Exchange != "SSE" {
		t.Fatalf("unexpected star security: %+v", securities[1])
	}
	if securities[0].Name != "创业样本更新" || securities[0].Source != "test2" {
		t.Fatalf("expected idempotent update, got %+v", securities[0])
	}
}

func TestBulkUpsertDailyBarsAndMarketMetrics(t *testing.T) {
	repo := setupRepo(t)
	ctx := context.Background()
	turnoverRate := 1.23
	marketCap := 100_000_000.0
	pe := 12.5
	pb := 1.1

	if err := repo.BulkUpsertDailyBars(ctx, []FactorDailyBar{
		{Code: "1", TradeDate: "2026-05-08", Open: 10, Close: 11, High: 12, Low: 9, Volume: 1000, Amount: 11000, TurnoverRate: &turnoverRate, Source: "test"},
	}); err != nil {
		t.Fatalf("upsert daily bars: %v", err)
	}
	if err := repo.BulkUpsertMarketMetrics(ctx, []FactorMarketMetric{
		{Code: "000001", TradeDate: "2026-05-08", ClosePrice: 11, MarketCap: &marketCap, PE: &pe, PB: &pb, Volume: 1000, Amount: 11000, TurnoverRate: &turnoverRate, Source: "test"},
	}); err != nil {
		t.Fatalf("upsert market metrics: %v", err)
	}

	var bar FactorDailyBar
	if err := repo.db.First(&bar, "code = ? AND trade_date = ?", "000001", "2026-05-08").Error; err != nil {
		t.Fatalf("query daily bar: %v", err)
	}
	if bar.Adjusted != "qfq" || bar.Amount != 11000 || bar.TurnoverRate == nil || *bar.TurnoverRate != turnoverRate {
		t.Fatalf("unexpected daily bar: %+v", bar)
	}

	var metric FactorMarketMetric
	if err := repo.db.First(&metric, "code = ? AND trade_date = ?", "000001", "2026-05-08").Error; err != nil {
		t.Fatalf("query market metric: %v", err)
	}
	if metric.MarketCap == nil || *metric.MarketCap != marketCap || metric.PE == nil || *metric.PE != pe || metric.PB == nil || *metric.PB != pb {
		t.Fatalf("unexpected market metric: %+v", metric)
	}
}

func TestTaskRunLifecycleAndItems(t *testing.T) {
	repo := setupRepo(t)
	ctx := context.Background()
	run := FactorTaskRun{ID: "run-1", TaskType: TaskTypeBackfill, ParamsJSON: `{"mode":"securities"}`}
	if err := repo.CreateTaskRun(ctx, run); err != nil {
		t.Fatalf("create task run: %v", err)
	}
	if err := repo.BulkUpsertTaskItems(ctx, []FactorTaskItem{
		{RunID: "run-1", ItemType: "security", ItemKey: "000001", Status: TaskStatusSuccess},
		{RunID: "run-1", ItemType: "security", ItemKey: "000002", Status: TaskStatusFailed, ErrorMessage: "failed"},
	}); err != nil {
		t.Fatalf("upsert task items: %v", err)
	}
	if err := repo.FinishTaskRun(ctx, "run-1", TaskStatusPartial, 2, 1, 1, 0, `{"coverage":50}`, ""); err != nil {
		t.Fatalf("finish task run: %v", err)
	}

	var saved FactorTaskRun
	if err := repo.db.First(&saved, "id = ?", "run-1").Error; err != nil {
		t.Fatalf("query task run: %v", err)
	}
	if saved.Status != TaskStatusPartial || saved.FinishedAt == nil || saved.TotalCount != 2 || saved.SuccessCount != 1 || saved.FailedCount != 1 {
		t.Fatalf("unexpected task run: %+v", saved)
	}

	var items []FactorTaskItem
	if err := repo.db.Order("item_key ASC").Find(&items, "run_id = ?", "run-1").Error; err != nil {
		t.Fatalf("query task items: %v", err)
	}
	if len(items) != 2 || items[1].Status != TaskStatusFailed || items[1].UpdatedAt.IsZero() {
		t.Fatalf("unexpected task items: %+v", items)
	}
}

func TestLatestSuccessfulSnapshotDate(t *testing.T) {
	repo := setupRepo(t)
	ctx := context.Background()
	date, err := repo.LatestSuccessfulSnapshotDate(ctx)
	if err != nil {
		t.Fatalf("latest successful snapshot: %v", err)
	}
	if date != "" {
		t.Fatalf("expected empty date, got %q", date)
	}
	if err := repo.CreateTaskRun(ctx, FactorTaskRun{ID: "run-old", TaskType: TaskTypeDailyCompute, SnapshotDate: "2026-05-07", Status: TaskStatusSuccess}); err != nil {
		t.Fatalf("create old run: %v", err)
	}
	if err := repo.CreateTaskRun(ctx, FactorTaskRun{ID: "run-new", TaskType: TaskTypeDailyCompute, SnapshotDate: "2026-05-08", Status: TaskStatusPartial}); err != nil {
		t.Fatalf("create new run: %v", err)
	}
	date, err = repo.LatestSuccessfulSnapshotDate(ctx)
	if err != nil {
		t.Fatalf("latest successful snapshot after insert: %v", err)
	}
	if date != "2026-05-08" {
		t.Fatalf("expected latest date, got %q", date)
	}
}

func TestSnapshotUpsertDefaultsQualityFlags(t *testing.T) {
	repo := setupRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()
	if err := repo.BulkUpsertSnapshots(ctx, []FactorSnapshot{{SnapshotDate: "2026-05-08", Code: "000001", Name: "平安银行", ClosePrice: 11, CreatedAt: now}}); err != nil {
		t.Fatalf("upsert snapshots: %v", err)
	}
	var snapshot FactorSnapshot
	if err := repo.db.First(&snapshot, "snapshot_date = ? AND code = ?", "2026-05-08", "000001").Error; err != nil {
		t.Fatalf("query snapshot: %v", err)
	}
	if snapshot.Symbol != "000001.SZ" || snapshot.DataQualityFlags != "[]" || snapshot.CreatedAt.IsZero() {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
}
