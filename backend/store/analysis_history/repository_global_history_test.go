package analysis_history

import (
	"context"
	"testing"
	"time"
)

func TestListByUser_EmptyUser(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	items, total, err := repo.ListByUser(context.Background(), "", 10, 0)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if total != 0 || len(items) != 0 {
		t.Fatalf("expected empty result, got total=%d len=%d", total, len(items))
	}
}

func TestListByUser_NormalizesLimitAndOffset(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	base := time.Date(2026, 6, 13, 13, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		record := AnalysisHistoryRecord{
			ID:         genID(),
			UserID:     "u-normalize",
			Symbol:     "00700.HK",
			SymbolName: "腾讯控股",
			ResultJSON: "{}",
			MetaJSON:   "{}",
			CreatedAt:  base.Add(time.Duration(-i) * time.Minute),
		}
		if err := repo.Create(ctx, &record); err != nil {
			t.Fatalf("create: %v", err)
		}
	}

	items, total, err := repo.ListByUser(ctx, "u-normalize", 999, -1)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if total != 3 {
		t.Fatalf("total=%d", total)
	}
	if len(items) != 3 {
		t.Fatalf("len=%d", len(items))
	}
}
