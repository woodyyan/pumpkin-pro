package signal

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/woodyyan/pumpkin-pro/backend/store/quadrant"
	"github.com/woodyyan/pumpkin-pro/backend/tests/testutil"
)

func setupSubscriptionTest(t *testing.T) (*Service, func()) {
	t.Helper()
	db := testutil.InMemoryDB(t)
	testutil.AutoMigrateModels(t, db,
		&WebhookEndpointRecord{},
		&SymbolSignalConfigRecord{},
		&SignalSubscriptionRecord{},
		&SignalEventRecord{},
		&WebhookDeliveryRecord{},
		&quadrant.QuadrantScoreRecord{},
	)
	return NewService(NewRepository(db), ServiceConfig{}), func() {}
}

func boolPtr(v bool) *bool       { return &v }
func intPtr(v int) *int          { return &v }
func float64Ptr(v float64) *float64 { return &v }

func TestCreateSubscription_DefaultsEnabled(t *testing.T) {
	svc, cleanup := setupSubscriptionTest(t)
	defer cleanup()
	ctx := context.Background()

	item, err := svc.CreateSubscription(ctx, "user-1", SignalSubscriptionInput{
		TemplateKey: "macd_cross",
		Symbol:      "600519",
	})
	if err != nil {
		t.Fatalf("CreateSubscription failed: %v", err)
	}
	if !item.IsEnabled {
		t.Error("subscription should be enabled by default (一键开启)")
	}
	if item.Symbol != "600519.SH" {
		t.Errorf("symbol should be normalized, got %s", item.Symbol)
	}
	if item.EvalIntervalSeconds != defaultSubscriptionEvalIntervalSeconds {
		t.Errorf("expected default eval interval %d, got %d", defaultSubscriptionEvalIntervalSeconds, item.EvalIntervalSeconds)
	}
	if item.TemplateName == "" || item.Category != TemplateCategoryIndicator {
		t.Errorf("template meta should be resolved, got name=%q category=%q", item.TemplateName, item.Category)
	}
	// 参数应合并模板默认值。
	if item.Params["fast_period"] != 12.0 {
		t.Errorf("expected default params merged, got %v", item.Params)
	}
}

func TestCreateSubscription_MultipleTemplatesSameSymbol(t *testing.T) {
	svc, cleanup := setupSubscriptionTest(t)
	defer cleanup()
	ctx := context.Background()

	for _, key := range []string{"macd_cross", "rsi_range"} {
		if _, err := svc.CreateSubscription(ctx, "user-1", SignalSubscriptionInput{TemplateKey: key, Symbol: "600519.SH"}); err != nil {
			t.Fatalf("CreateSubscription %s failed: %v", key, err)
		}
	}
	items, err := svc.ListSubscriptions(ctx, "user-1")
	if err != nil {
		t.Fatalf("ListSubscriptions failed: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("一股应可配多个信号, got %d items", len(items))
	}
}

func TestCreateSubscription_DuplicateConflict(t *testing.T) {
	svc, cleanup := setupSubscriptionTest(t)
	defer cleanup()
	ctx := context.Background()

	input := SignalSubscriptionInput{TemplateKey: "macd_cross", Symbol: "600519.SH"}
	if _, err := svc.CreateSubscription(ctx, "user-1", input); err != nil {
		t.Fatalf("first create failed: %v", err)
	}
	if _, err := svc.CreateSubscription(ctx, "user-1", input); !errors.Is(err, ErrConflict) {
		t.Errorf("expected ErrConflict on duplicate, got %v", err)
	}
}

func TestCreateSubscription_Validation(t *testing.T) {
	svc, cleanup := setupSubscriptionTest(t)
	defer cleanup()
	ctx := context.Background()

	if _, err := svc.CreateSubscription(ctx, "user-1", SignalSubscriptionInput{TemplateKey: "nope", Symbol: "600519.SH"}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("unknown template should fail, got %v", err)
	}
	if _, err := svc.CreateSubscription(ctx, "user-1", SignalSubscriptionInput{TemplateKey: "strategy", Symbol: "600519.SH"}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("strategy template without strategy_id should fail, got %v", err)
	}
	if _, err := svc.CreateSubscription(ctx, "user-1", SignalSubscriptionInput{TemplateKey: "price_above", Symbol: "600519.SH"}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("price_above without price param should fail, got %v", err)
	}
	if _, err := svc.CreateSubscription(ctx, "", SignalSubscriptionInput{TemplateKey: "macd_cross", Symbol: "600519.SH"}); !errors.Is(err, ErrForbidden) {
		t.Errorf("empty user should fail, got %v", err)
	}
	// 非策略模板的 strategy_id 应被清空，避免脏引用。
	item, err := svc.CreateSubscription(ctx, "user-1", SignalSubscriptionInput{TemplateKey: "macd_cross", Symbol: "600519.SH", StrategyID: "stray"})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if item.StrategyID != "" {
		t.Errorf("non-strategy template should clear strategy_id, got %q", item.StrategyID)
	}
}

func TestUpdateSubscription_ToggleAndRiskFields(t *testing.T) {
	svc, cleanup := setupSubscriptionTest(t)
	defer cleanup()
	ctx := context.Background()

	item, err := svc.CreateSubscription(ctx, "user-1", SignalSubscriptionInput{TemplateKey: "rsi_range", Symbol: "00700.HK"})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	// 开关即点即生效：仅传 is_enabled。
	updated, err := svc.UpdateSubscription(ctx, "user-1", item.ID, SignalSubscriptionUpdateInput{IsEnabled: boolPtr(false)})
	if err != nil {
		t.Fatalf("toggle off failed: %v", err)
	}
	if updated.IsEnabled {
		t.Error("expected disabled after toggle")
	}

	// 显式 0/false 关闭风控规则必须落库（GORM 零值坑回归）。
	updated, err = svc.UpdateSubscription(ctx, "user-1", item.ID, SignalSubscriptionUpdateInput{
		PositionAwareEnabled: boolPtr(false),
		MaxPositionPct:       float64Ptr(0),
		StopLossPct:          float64Ptr(8),
	})
	if err != nil {
		t.Fatalf("risk update failed: %v", err)
	}
	if updated.PositionAwareEnabled {
		t.Error("position_aware_enabled=false must persist")
	}
	if updated.StopLossPct != 8 {
		t.Errorf("stop_loss_pct should be 8, got %v", updated.StopLossPct)
	}

	// 越界校验。
	if _, err := svc.UpdateSubscription(ctx, "user-1", item.ID, SignalSubscriptionUpdateInput{EvalIntervalSeconds: intPtr(60)}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("eval interval below min should fail, got %v", err)
	}
	if _, err := svc.UpdateSubscription(ctx, "user-1", item.ID, SignalSubscriptionUpdateInput{StopLossPct: float64Ptr(101)}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("stop_loss_pct > 100 should fail, got %v", err)
	}
	// 他人订阅不可改。
	if _, err := svc.UpdateSubscription(ctx, "user-2", item.ID, SignalSubscriptionUpdateInput{IsEnabled: boolPtr(true)}); !errors.Is(err, ErrNotFound) {
		t.Errorf("cross-user update should be not found, got %v", err)
	}
}

func TestDeleteSubscription(t *testing.T) {
	svc, cleanup := setupSubscriptionTest(t)
	defer cleanup()
	ctx := context.Background()

	item, _ := svc.CreateSubscription(ctx, "user-1", SignalSubscriptionInput{TemplateKey: "macd_cross", Symbol: "600519.SH"})
	if err := svc.DeleteSubscription(ctx, "user-1", item.ID); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	if err := svc.DeleteSubscription(ctx, "user-1", item.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("second delete should be not found, got %v", err)
	}
}

// TestCreateSubscription_PositionAwareFalsePreserved 覆盖 GORM default:true 首次创建坑。
func TestCreateSubscription_PositionAwareFalsePreserved(t *testing.T) {
	svc, cleanup := setupSubscriptionTest(t)
	defer cleanup()
	ctx := context.Background()

	item, err := svc.CreateSubscription(ctx, "user-1", SignalSubscriptionInput{TemplateKey: "macd_cross", Symbol: "600519.SH"})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	// 先创建（默认 true），再显式关闭，最后验证读回 false。
	if _, err := svc.UpdateSubscription(ctx, "user-1", item.ID, SignalSubscriptionUpdateInput{PositionAwareEnabled: boolPtr(false)}); err != nil {
		t.Fatalf("update failed: %v", err)
	}
	items, _ := svc.ListSubscriptions(ctx, "user-1")
	if len(items) != 1 || items[0].PositionAwareEnabled {
		t.Error("position_aware_enabled=false must survive a fresh read")
	}
}

// ── 迁移 ──

func TestMigrateSymbolConfigsToSubscriptions(t *testing.T) {
	svc, cleanup := setupSubscriptionTest(t)
	defer cleanup()
	ctx := context.Background()

	// 准备两条旧配置：一条启用带风控，一条停用。
	legacy := []SymbolSignalConfigRecord{
		{
			ID: "sc-1", UserID: "user-1", Symbol: "600519.SH", StrategyID: "strat-001",
			IsEnabled: true, CooldownSeconds: 3600, EvalIntervalSeconds: 1800,
			ThresholdsJSON: `{"note":"x"}`,
			PositionAwareEnabled: true, MaxPositionPct: 25, StopLossPct: 8,
			CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
		},
		{
			ID: "sc-2", UserID: "user-2", Symbol: "00700.HK", StrategyID: "",
			IsEnabled: false, CooldownSeconds: 300, EvalIntervalSeconds: 3600,
			ThresholdsJSON: "{}", PositionAwareEnabled: false,
			CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
		},
	}
	for _, record := range legacy {
		// 注意：GORM Create 会把 default 标签值回填进结构体字段（default:true 坑的另一半），
		// 必须先保存期望值，Create 后再用期望值显式回写。
		desiredPosAware := record.PositionAwareEnabled
		desiredEnabled := record.IsEnabled
		if err := svc.repo.db.Create(&record).Error; err != nil {
			t.Fatalf("seed legacy config failed: %v", err)
		}
		if err := svc.repo.db.Model(&SymbolSignalConfigRecord{}).Where("id = ?", record.ID).
			Updates(map[string]any{"position_aware_enabled": desiredPosAware, "is_enabled": desiredEnabled}).Error; err != nil {
			t.Fatalf("seed legacy config flags failed: %v", err)
		}
	}

	report, err := svc.MigrateSymbolConfigsToSubscriptions(ctx)
	if err != nil {
		t.Fatalf("migration failed: %v", err)
	}
	if report.LegacyTotal != 2 || report.LegacyEnabled != 1 {
		t.Errorf("reconcile counts wrong: %+v", report)
	}
	if report.Created != 2 || report.Skipped != 0 || report.Failed != 0 {
		t.Errorf("expected 2 created, got %+v", report)
	}

	// 字段保留校验。
	items, err := svc.ListSubscriptions(ctx, "user-1")
	if err != nil || len(items) != 1 {
		t.Fatalf("expected 1 migrated subscription, got %d err=%v", len(items), err)
	}
	migrated := items[0]
	if migrated.TemplateKey != "strategy" || migrated.StrategyID != "strat-001" {
		t.Errorf("expected strategy subscription, got %s/%s", migrated.TemplateKey, migrated.StrategyID)
	}
	if !migrated.IsEnabled || migrated.EvalIntervalSeconds != 1800 || migrated.MaxPositionPct != 25 || migrated.StopLossPct != 8 {
		t.Errorf("migration must preserve flags/risk fields, got %+v", migrated)
	}

	// user-2 的停用配置 + position_aware=false 必须保留。
	items2, _ := svc.ListSubscriptions(ctx, "user-2")
	if len(items2) != 1 || items2[0].IsEnabled || items2[0].PositionAwareEnabled {
		t.Errorf("migration must preserve disabled/false states, got %+v", items2)
	}

	// 幂等：再跑一次全部 skipped。
	report2, err := svc.MigrateSymbolConfigsToSubscriptions(ctx)
	if err != nil {
		t.Fatalf("second migration failed: %v", err)
	}
	if report2.Created != 0 || report2.Skipped != 2 {
		t.Errorf("migration must be idempotent, got %+v", report2)
	}
}

// ── 事件流 / 未读 / 已读 ──

func emitTestEvent(t *testing.T, svc *Service, userID, symbol, templateKey, barState string, isRead bool) {
	t.Helper()
	ctx := context.Background()
	_, err := svc.EmitSignal(ctx, EmitSignalInput{
		UserID:      userID,
		Symbol:      symbol,
		Side:        SideBuy,
		SignalScore: 1,
		Reason:      map[string]any{"message": "test"},
		EventTime:   time.Now().UTC(),
		TemplateKey: templateKey,
		BarState:    barState,
		Gate: &EmitGateInfo{
			RawSide:   SideBuy,
			FinalSide: SideBuy,
			Decision:  GateDecisionPass,
		},
	})
	if err != nil {
		t.Fatalf("EmitSignal failed: %v", err)
	}
}

func TestSignalEvents_FilterUnreadMarkRead(t *testing.T) {
	svc, cleanup := setupSubscriptionTest(t)
	defer cleanup()
	ctx := context.Background()

	// 不同 bar_state 的同向事件可以共存（指纹含 bar_state）。
	emitTestEvent(t, svc, "user-1", "600519.SH", "macd_cross", BarStateIntradayProvisional, false)
	emitTestEvent(t, svc, "user-1", "600519.SH", "macd_cross", BarStateCloseConfirmed, false)
	emitTestEvent(t, svc, "user-1", "00700.HK", "price_above", BarStateRealtime, false)

	// 过滤：bar_state。
	confirmed, err := svc.ListSignalEventsFiltered(ctx, "user-1", "", BarStateCloseConfirmed, "", 50)
	if err != nil || len(confirmed) != 1 {
		t.Fatalf("expected 1 close_confirmed event, got %d err=%v", len(confirmed), err)
	}
	if confirmed[0].BarState != BarStateCloseConfirmed || confirmed[0].TemplateName == "" {
		t.Errorf("event should carry bar_state and resolved template name, got %+v", confirmed[0])
	}

	// 过滤：symbol + side。
	bySymbol, err := svc.ListSignalEventsFiltered(ctx, "user-1", "600519", "", "buy", 50)
	if err != nil || len(bySymbol) != 2 {
		t.Errorf("expected 2 events for 600519, got %d err=%v", len(bySymbol), err)
	}

	// 非法 bar_state。
	if _, err := svc.ListSignalEventsFiltered(ctx, "user-1", "", "bogus", "", 50); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("invalid bar_state should fail, got %v", err)
	}

	// 未读数（测试信号不计入）。
	unread, err := svc.CountUnreadEvents(ctx, "user-1")
	if err != nil || unread != 3 {
		t.Errorf("expected 3 unread, got %d err=%v", unread, err)
	}

	// 按股票标记已读。
	marked, err := svc.MarkEventsRead(ctx, "user-1", "600519.SH")
	if err != nil || marked != 2 {
		t.Errorf("expected 2 marked, got %d err=%v", marked, err)
	}
	unread, _ = svc.CountUnreadEvents(ctx, "user-1")
	if unread != 1 {
		t.Errorf("expected 1 unread after mark, got %d", unread)
	}

	// 全部标记已读。
	if _, err := svc.MarkEventsRead(ctx, "user-1", ""); err != nil {
		t.Fatalf("mark all read failed: %v", err)
	}
	unread, _ = svc.CountUnreadEvents(ctx, "user-1")
	if unread != 0 {
		t.Errorf("expected 0 unread, got %d", unread)
	}
}

func TestStrategyRefs_CombinedCount(t *testing.T) {
	svc, cleanup := setupSubscriptionTest(t)
	defer cleanup()
	ctx := context.Background()

	// 旧配置引用。
	legacy := SymbolSignalConfigRecord{
		ID: "sc-9", UserID: "user-1", Symbol: "600519.SH", StrategyID: "strat-x",
		IsEnabled: true, ThresholdsJSON: "{}", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := svc.repo.db.Create(&legacy).Error; err != nil {
		t.Fatalf("seed legacy failed: %v", err)
	}
	// 新订阅引用。
	if _, err := svc.CreateSubscription(ctx, "user-1", SignalSubscriptionInput{
		TemplateKey: "strategy", StrategyID: "strat-x", Symbol: "00700.HK",
	}); err != nil {
		t.Fatalf("create strategy subscription failed: %v", err)
	}

	count, err := svc.CountSignalRefsByStrategy(ctx, "user-1", "strat-x")
	if err != nil || count != 2 {
		t.Errorf("expected combined ref count 2, got %d err=%v", count, err)
	}
	refs, err := svc.ListSignalRefs(ctx, "user-1", "strat-x")
	if err != nil || len(refs) != 2 {
		t.Errorf("expected 2 merged refs, got %d err=%v", len(refs), err)
	}
}
