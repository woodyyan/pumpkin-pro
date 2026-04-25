package strategy

import (
	"context"
	"testing"

	"github.com/woodyyan/pumpkin-pro/backend/tests/testutil"
)

func setupStrategyTest(t *testing.T) (*Repository, func()) {
	t.Helper()
	db := testutil.InMemoryDB(t)
	testutil.AutoMigrateModels(t, db, &StrategyRecord{})
	repo := NewRepository(db)
	return repo, func() {}
}

func TestStrategyRepo_CreateAndGetByID(t *testing.T) {
	repo, cleanup := setupStrategyTest(t)
	defer cleanup()
	ctx := context.Background()

	payload := StrategyPayload{
		ID:                "strat-test-001",
		Key:               "test-ma-cross",
		Name:              "测试均线交叉",
		Description:       "基于MA金叉死叉的测试策略",
		Category:          "趋势",
		ImplementationKey: "trend_cross",
		Status:            "draft",
		Version:           1,
		ParamSchema:       []ParamSchemaItem{},
		DefaultParams:     map[string]any{"period": 20},
	}
	created, err := repo.Create(ctx, "user-1", payload)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if created.ID != payload.ID {
		t.Errorf("expected ID %s, got %s", payload.ID, created.ID)
	}
	if created.Name != payload.Name {
		t.Errorf("expected Name %s, got %s", payload.Name, created.Name)
	}

	got, err := repo.GetByID(ctx, payload.ID, "user-1", false)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.Name != payload.Name {
		t.Errorf("expected Name %s after GetByID, got %s", payload.Name, got.Name)
	}
}

func TestStrategyRepo_GetByID_NotFound(t *testing.T) {
	repo, cleanup := setupStrategyTest(t)
	defer cleanup()

	_, err := repo.GetByID(context.Background(), "nonexistent", "u1", false)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestStrategyRepo_GetByID_Forbidden(t *testing.T) {
	repo, cleanup := setupStrategyTest(t)
	defer cleanup()
	ctx := context.Background()

	payload := StrategyPayload{
		ID: "strat-forbid-01", Key: "k1", Name: "S1",
		Description: "D", Category: "C", ImplementationKey: "trend_cross",
		Status: "draft", Version: 1, ParamSchema: []ParamSchemaItem{},
	}
	_, _ = repo.Create(ctx, "owner-user", payload)

	_, err := repo.GetByID(ctx, "strat-forbid-01", "other-user", false)
	if err != ErrNotFound {
		t.Errorf("expecting ErrNotFound when accessing another user's strategy, got %v", err)
	}
}

func TestStrategyRepo_Create_DuplicateKey(t *testing.T) {
	repo, cleanup := setupStrategyTest(t)
	defer cleanup()
	ctx := context.Background()

	p := StrategyPayload{
		ID: "dup-key-01", Key: "same-key", Name: "First",
		Description: "D", Category: "C", ImplementationKey: "trend_cross",
		Status: "draft", Version: 1, ParamSchema: []ParamSchemaItem{},
	}
	_, _ = repo.Create(ctx, "user-dup", p)

	p2 := StrategyPayload{
		ID: "dup-key-02", Key: "same-key", Name: "Second",
		Description: "D", Category: "C", ImplementationKey: "trend_cross",
		Status: "draft", Version: 1, ParamSchema: []ParamSchemaItem{},
	}
	_, err := repo.Create(ctx, "user-dup", p2)
	if err != ErrConflict {
		t.Errorf("expected ErrConflict for duplicate key, got %v", err)
	}
}

func TestStrategyRepo_Create_SystemAndUserCanShareSemanticsButNotKey(t *testing.T) {
	repo, cleanup := setupStrategyTest(t)
	defer cleanup()
	ctx := context.Background()

	systemPayload := StrategyPayload{
		ID: "trend-ma-cross", Key: "trend_ma_cross", Name: "系统趋势策略",
		Description: "D", Category: "趋势", ImplementationKey: "trend_cross",
		Status: "active", Version: 1, ParamSchema: []ParamSchemaItem{},
	}
	if _, err := repo.Create(ctx, "", systemPayload); err != nil {
		t.Fatalf("create system strategy failed: %v", err)
	}

	userPayload := StrategyPayload{
		ID: "trend-strategy-abc123", Key: "trend_strategy_abc123", Name: "趋势跟踪策略 1",
		Description: "D", Category: "趋势", ImplementationKey: "trend_cross",
		Status: "draft", Version: 1, ParamSchema: []ParamSchemaItem{},
	}
	if _, err := repo.Create(ctx, "user-1", userPayload); err != nil {
		t.Fatalf("create user strategy failed: %v", err)
	}

	conflictPayload := StrategyPayload{
		ID: "other-id", Key: "trend_ma_cross", Name: "冲突策略",
		Description: "D", Category: "趋势", ImplementationKey: "trend_cross",
		Status: "draft", Version: 1, ParamSchema: []ParamSchemaItem{},
	}
	_, err := repo.Create(ctx, "", conflictPayload)
	if err != ErrConflict {
		t.Errorf("expected ErrConflict for duplicate system key, got %v", err)
	}
}

func TestStrategyRepo_List_Empty(t *testing.T) {
	repo, cleanup := setupStrategyTest(t)
	defer cleanup()

	items, err := repo.List(context.Background(), "user-x", true, true)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected empty list, got %d items", len(items))
	}
}

func TestStrategyRepo_List_WithItems(t *testing.T) {
	repo, cleanup := setupStrategyTest(t)
	defer cleanup()
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		payload := StrategyPayload{
			ID:                t.Name() + "-item-" + itoa(i),
			Key:               t.Name() + "-" + itoa(i),
			Name:              "策略" + itoa(i),
			Description:       "描述" + itoa(i),
			Category:          "趋势",
			ImplementationKey: "trend_cross",
			Status:            "active",
			Version:           1,
			ParamSchema:       []ParamSchemaItem{},
		}
		if _, err := repo.Create(ctx, "list-user", payload); err != nil {
			t.Fatalf("create item %d failed: %v", i, err)
		}
	}

	items, err := repo.List(ctx, "list-user", false, false)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(items) != 3 {
		t.Errorf("expected 3 items, got %d", len(items))
	}
}

func TestStrategyRepo_Delete(t *testing.T) {
	repo, cleanup := setupStrategyTest(t)
	defer cleanup()
	ctx := context.Background()

	payload := StrategyPayload{
		ID: "strat-del-01", Key: "del-k1", Name: "DeleteMe",
		Description: "D", Category: "C", ImplementationKey: "rsi_range",
		Status: "active", Version: 1, ParamSchema: []ParamSchemaItem{},
	}
	_, _ = repo.Create(ctx, "del-owner", payload)

	err := repo.Delete(ctx, "strat-del-01", "del-owner")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = repo.GetByID(ctx, "strat-del-01", "del-owner", false)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestStrategyRepo_Delete_SystemForbidden(t *testing.T) {
	repo, cleanup := setupStrategyTest(t)
	defer cleanup()
	ctx := context.Background()

	sysPayload := StrategyPayload{
		ID: "sys-strat-01", Key: "sys-k", Name: "SystemStrat",
		Description: "D", Category: "C", ImplementationKey: "grid",
		Status: "active", Version: 1, ParamSchema: []ParamSchemaItem{},
	}
	_, _ = repo.Create(ctx, "", sysPayload)

	err := repo.Delete(ctx, "sys-strat-01", "some-user")
	if err != ErrForbidden {
		t.Errorf("expected ErrForbidden deleting system strategy, got %v", err)
	}
}

func TestStrategyRepo_Update(t *testing.T) {
	repo, cleanup := setupStrategyTest(t)
	defer cleanup()
	ctx := context.Background()

	payload := StrategyPayload{
		ID: "strat-upd-01", Key: "upd-k1", Name: "OriginalName",
		Description: "Original desc", Category: "趋势",
		ImplementationKey: "bollinger_reversion",
		Status: "draft", Version: 1, ParamSchema: []ParamSchemaItem{},
		DefaultParams: map[string]any{"period": 5},
	}
	_, _ = repo.Create(ctx, "upd-owner", payload)

	update := StrategyPayload{
		ID: "strat-upd-01", Key: "upd-k1", Name: "UpdatedName",
		Description: "Updated desc", Category: "均值回归",
		ImplementationKey: "bollinger_reversion",
		Status: "active", Version: 2,
		ParamSchema:       []ParamSchemaItem{},
		DefaultParams: map[string]any{"period": 10},
	}
	updated, err := repo.Update(ctx, "strat-upd-01", "upd-owner", update)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if updated.Name != "UpdatedName" {
		t.Errorf("expected UpdatedName, got %s", updated.Name)
	}
}

func TestStrategyRepo_CountSystem(t *testing.T) {
	repo, cleanup := setupStrategyTest(t)
	defer cleanup()
	ctx := context.Background()

	count, err := repo.CountSystem(ctx)
	if err != nil {
		t.Fatalf("CountSystem failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 system strategies initially, got %d", count)
	}

	sysPayload := StrategyPayload{
		ID: "sys-count-01", Key: "sys-count-k", Name: "Sys",
		Description: "D", Category: "C", ImplementationKey: "macd_cross",
		Status: "active", Version: 1, ParamSchema: []ParamSchemaItem{},
	}
	_, _ = repo.Create(ctx, "", sysPayload)

	count, _ = repo.CountSystem(ctx)
	if count != 1 {
		t.Errorf("expected 1 system strategy after insert, got %d", count)
	}
}

// ── Helpers ──

func itoa(i int) string {
	switch i {
	case 0:
		return "0"
	case 1:
		return "1"
	case 2:
		return "2"
	default:
		return "N"
	}
}
