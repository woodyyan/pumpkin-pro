package signal

import (
	"context"
	"testing"
	"time"

	"github.com/woodyyan/pumpkin-pro/backend/store/quadrant"
	"github.com/woodyyan/pumpkin-pro/backend/tests/testutil"
)

// ──────────────────────────────────────────────────────────────
// EmitSignal「生成 / 投递」解耦测试
//
// 改造前：无 webhook 直接返回 ErrWebhookMissing，一条记录都不写。
// 改造后（全量生成 + 推送门控）：
//   - Gate == nil  → 保持旧语义（要求 webhook，否则报错），兼容 SendTestSignal。
//   - Gate != nil  → 一律落库；仅当 !SkipDelivery 且 webhook 可用时才投递。
// ──────────────────────────────────────────────────────────────

func setupEmitTest(t *testing.T) (*Service, *Repository) {
	t.Helper()
	db := testutil.InMemoryDB(t)
	testutil.AutoMigrateModels(t, db,
		&WebhookEndpointRecord{},
		&SymbolSignalConfigRecord{},
		&SignalEventRecord{},
		&WebhookDeliveryRecord{},
		&quadrant.QuadrantScoreRecord{},
	)
	repo := NewRepository(db)
	return NewService(repo, ServiceConfig{}), repo
}

func seedEnabledWebhook(t *testing.T, repo *Repository, userID string) {
	t.Helper()
	record := WebhookEndpointRecord{
		ID:        "wh-" + userID,
		UserID:    userID,
		URL:       "https://example.com/hook",
		Channel:   "wecom",
		IsEnabled: true,
		TimeoutMS: 3000,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if _, err := repo.SaveWebhookEndpoint(context.Background(), record); err != nil {
		t.Fatalf("seed webhook failed: %v", err)
	}
}

func countDeliveries(t *testing.T, repo *Repository, eventID string) int64 {
	t.Helper()
	var count int64
	if err := repo.db.Model(&WebhookDeliveryRecord{}).Where("event_id = ?", eventID).Count(&count).Error; err != nil {
		t.Fatalf("count deliveries failed: %v", err)
	}
	return count
}

// 旧语义必须保持：非门控模式 + 无 webhook → 报错且不落库。
func TestEmitSignal_NoGate_NoWebhook_KeepsLegacyError(t *testing.T) {
	svc, repo := setupEmitTest(t)
	ctx := context.Background()

	_, err := svc.EmitSignal(ctx, EmitSignalInput{
		UserID: "u-legacy", Symbol: "000001.SZ", Side: "BUY",
		EventTime: time.Now().UTC(),
	})
	if err != ErrWebhookMissing {
		t.Fatalf("非门控模式无 webhook 应返回 ErrWebhookMissing，got %v", err)
	}

	var count int64
	if err := repo.db.Model(&SignalEventRecord{}).Count(&count).Error; err != nil {
		t.Fatalf("count events failed: %v", err)
	}
	if count != 0 {
		t.Errorf("旧语义下不应落库，got %d 条", count)
	}
}

// 门控模式 + 无 webhook → 仍然落库（保留可复盘性），但不投递。
func TestEmitSignal_Gated_NoWebhook_StillPersists(t *testing.T) {
	svc, repo := setupEmitTest(t)
	ctx := context.Background()

	event, err := svc.EmitSignal(ctx, EmitSignalInput{
		UserID: "u-gated", Symbol: "000001.SZ", Side: "BUY",
		EventTime: time.Now().UTC(),
		Gate: &EmitGateInfo{
			RawSide: "BUY", FinalSide: "BUY", Decision: GateDecisionPass,
			PositionDataStatus: PositionDataUnknown,
		},
	})
	if err != nil {
		t.Fatalf("门控模式下无 webhook 不应报错，got %v", err)
	}
	if event == nil {
		t.Fatal("应返回已落库的事件")
	}
	if event.IsDelivered {
		t.Error("无 webhook 时 IsDelivered 应为 false")
	}
	if countDeliveries(t, repo, event.EventID) != 0 {
		t.Error("无 webhook 时不应创建投递记录")
	}

	saved, err := repo.GetSignalEventByEventID(ctx, event.EventID)
	if err != nil {
		t.Fatalf("事件应已落库: %v", err)
	}
	if saved.PositionDataStatus != PositionDataUnknown {
		t.Errorf("门控字段应落库，got %s", saved.PositionDataStatus)
	}
}

// 被门控拦截 → 落库但不投递，且门控字段完整。
func TestEmitSignal_Suppressed_PersistsWithoutDelivery(t *testing.T) {
	svc, repo := setupEmitTest(t)
	ctx := context.Background()
	seedEnabledWebhook(t, repo, "u-sup")

	event, err := svc.EmitSignal(ctx, EmitSignalInput{
		UserID: "u-sup", Symbol: "000001.SZ", Side: "SELL",
		EventTime: time.Now().UTC(),
		Gate: &EmitGateInfo{
			RawSide:            "SELL",
			FinalSide:          "SELL",
			Decision:           GateDecisionSuppressed,
			SuppressedReason:   RuleSellNoPosition,
			MatchedRules:       []string{RuleSellNoPosition},
			PositionDataStatus: PositionDataKnown,
			PositionSnapshot:   map[string]any{"shares": 0.0},
			ReferencePrice:     12.34,
			SkipDelivery:       true,
		},
	})
	if err != nil {
		t.Fatalf("被拦截信号仍应成功落库，got %v", err)
	}
	if event.IsDelivered {
		t.Error("被拦截的信号 IsDelivered 应为 false")
	}
	if countDeliveries(t, repo, event.EventID) != 0 {
		t.Error("被拦截的信号不应创建投递记录（即使 webhook 可用）")
	}
	if event.SuppressedReason != RuleSellNoPosition {
		t.Errorf("应记录拦截原因，got %s", event.SuppressedReason)
	}
	if event.SuppressedMessage == "" {
		t.Error("应带用户可读的拦截说明（静默必须可解释）")
	}
	if event.ReferencePrice != 12.34 {
		t.Errorf("参考价应落库，got %v", event.ReferencePrice)
	}
	if len(event.MatchedRules) != 1 {
		t.Errorf("命中规则应落库，got %v", event.MatchedRules)
	}
}

// 门控通过 + webhook 可用 → 正常投递。
func TestEmitSignal_GatedPass_CreatesDelivery(t *testing.T) {
	svc, repo := setupEmitTest(t)
	ctx := context.Background()
	seedEnabledWebhook(t, repo, "u-pass")

	event, err := svc.EmitSignal(ctx, EmitSignalInput{
		UserID: "u-pass", Symbol: "000001.SZ", Side: "BUY",
		EventTime: time.Now().UTC(),
		Gate: &EmitGateInfo{
			RawSide: "BUY", FinalSide: "BUY", Decision: GateDecisionPass,
			SemanticLabel: SemanticAdd, SkipDelivery: false,
		},
	})
	if err != nil {
		t.Fatalf("EmitSignal failed: %v", err)
	}
	if !event.IsDelivered {
		t.Error("门控通过且 webhook 可用时 IsDelivered 应为 true")
	}
	if countDeliveries(t, repo, event.EventID) != 1 {
		t.Error("应创建 1 条投递记录")
	}
	if event.SemanticLabel != SemanticAdd {
		t.Errorf("语义标签应落库，got %s", event.SemanticLabel)
	}
}

// override（止损）必须推送，且 raw/final 方向被正确记录为不同值。
func TestEmitSignal_Overridden_DeliversWithRewrittenSide(t *testing.T) {
	svc, repo := setupEmitTest(t)
	ctx := context.Background()
	seedEnabledWebhook(t, repo, "u-ovr")

	event, err := svc.EmitSignal(ctx, EmitSignalInput{
		UserID: "u-ovr", Symbol: "000001.SZ",
		Side:      "SELL", // 已被风控改写
		EventTime: time.Now().UTC(),
		Gate: &EmitGateInfo{
			RawSide:       "BUY", // 策略原本说买
			FinalSide:     "SELL",
			Decision:      GateDecisionOverridden,
			SemanticLabel: SemanticStopLoss,
			MatchedRules:  []string{RuleStopLoss},
			SkipDelivery:  false,
		},
	})
	if err != nil {
		t.Fatalf("EmitSignal failed: %v", err)
	}
	if !event.IsDelivered {
		t.Error("override 的风控信号必须推送")
	}
	if event.RawSide != "BUY" || event.FinalSide != "SELL" {
		t.Errorf("应同时保留策略原始方向与风控改写后方向，got raw=%s final=%s", event.RawSide, event.FinalSide)
	}
	if event.Side != "SELL" {
		t.Errorf("Side 应等于 FinalSide 以兼容既有逻辑，got %s", event.Side)
	}
	if countDeliveries(t, repo, event.EventID) != 1 {
		t.Error("override 应创建投递记录")
	}
}

// webhook 存在但被禁用：门控模式下落库不投递，不报错。
//
// 注意：这里先以启用状态创建、再 UPDATE 为禁用。
// 原因是 WebhookEndpointRecord.IsEnabled 带 `default:true` 标签，
// GORM 的 Create 会跳过 false 零值而落库成 true（既有行为，与本次改造无关）。
func TestEmitSignal_Gated_DisabledWebhook_PersistsOnly(t *testing.T) {
	svc, repo := setupEmitTest(t)
	ctx := context.Background()
	seedEnabledWebhook(t, repo, "u-off")
	if err := repo.db.Model(&WebhookEndpointRecord{}).
		Where("user_id = ?", "u-off").
		Update("is_enabled", false).Error; err != nil {
		t.Fatalf("disable webhook failed: %v", err)
	}

	event, err := svc.EmitSignal(ctx, EmitSignalInput{
		UserID: "u-off", Symbol: "000001.SZ", Side: "BUY",
		EventTime: time.Now().UTC(),
		Gate:      &EmitGateInfo{RawSide: "BUY", FinalSide: "BUY", Decision: GateDecisionPass},
	})
	if err != nil {
		t.Fatalf("webhook 禁用时门控模式不应报错，got %v", err)
	}
	if event.IsDelivered {
		t.Error("webhook 禁用时不应标记为已投递")
	}
	if countDeliveries(t, repo, event.EventID) != 0 {
		t.Error("webhook 禁用时不应创建投递")
	}
}

// 合规：每条信号都必须内联免责声明。
func TestEmitSignal_AlwaysCarriesComplianceDisclaimer(t *testing.T) {
	svc, repo := setupEmitTest(t)
	ctx := context.Background()
	seedEnabledWebhook(t, repo, "u-comp")

	event, err := svc.EmitSignal(ctx, EmitSignalInput{
		UserID: "u-comp", Symbol: "000001.SZ", Side: "BUY",
		EventTime: time.Now().UTC(),
		Gate:      &EmitGateInfo{RawSide: "BUY", FinalSide: "BUY", Decision: GateDecisionPass},
	})
	if err != nil {
		t.Fatalf("EmitSignal failed: %v", err)
	}
	if event.Compliance == nil || event.Compliance.Disclaimer == "" {
		t.Fatal("每条信号必须内联免责声明")
	}
	if event.Compliance.Disclaimer != SignalComplianceDisclaimer {
		t.Errorf("免责声明文案应统一，got %s", event.Compliance.Disclaimer)
	}
}

// ── 单票风控配置读写 ──

func TestUpsertSymbolConfig_RiskDefaults(t *testing.T) {
	svc, _ := setupEmitTest(t)
	ctx := context.Background()

	cfg, err := svc.UpsertSymbolConfig(ctx, "u-cfg", "000001.SZ", SymbolSignalConfigInput{
		StrategyID: "s1",
	})
	if err != nil {
		t.Fatalf("UpsertSymbolConfig failed: %v", err)
	}
	if !cfg.PositionAwareEnabled {
		t.Error("持仓感知应默认开启")
	}
	if cfg.StopLossPct != 0 {
		t.Errorf("止损应默认关闭（0），got %v", cfg.StopLossPct)
	}
	if cfg.TrailingStopPct != 0 {
		t.Errorf("移动止盈应默认关闭（0），got %v", cfg.TrailingStopPct)
	}
}

func TestUpsertSymbolConfig_RiskFieldsPersistAndKeepOnNil(t *testing.T) {
	svc, _ := setupEmitTest(t)
	ctx := context.Background()

	maxPos := 15.0
	stopLoss := 8.0
	addTimes := 3
	if _, err := svc.UpsertSymbolConfig(ctx, "u-cfg2", "000001.SZ", SymbolSignalConfigInput{
		StrategyID:     "s1",
		MaxPositionPct: &maxPos,
		StopLossPct:    &stopLoss,
		MaxAddTimes:    &addTimes,
	}); err != nil {
		t.Fatalf("first upsert failed: %v", err)
	}

	// 第二次只改 cooldown，风控字段未传 → 必须保持原值。
	cfg, err := svc.UpsertSymbolConfig(ctx, "u-cfg2", "000001.SZ", SymbolSignalConfigInput{
		StrategyID:      "s1",
		CooldownSeconds: 1800,
	})
	if err != nil {
		t.Fatalf("second upsert failed: %v", err)
	}
	if cfg.MaxPositionPct != 15 || cfg.StopLossPct != 8 || cfg.MaxAddTimes != 3 {
		t.Errorf("未传的风控字段应保持原值，got maxPos=%v stopLoss=%v addTimes=%v",
			cfg.MaxPositionPct, cfg.StopLossPct, cfg.MaxAddTimes)
	}
}

func TestUpsertSymbolConfig_ExplicitZeroDisablesRule(t *testing.T) {
	svc, _ := setupEmitTest(t)
	ctx := context.Background()

	stopLoss := 8.0
	if _, err := svc.UpsertSymbolConfig(ctx, "u-cfg3", "000001.SZ", SymbolSignalConfigInput{
		StrategyID: "s1", StopLossPct: &stopLoss,
	}); err != nil {
		t.Fatalf("first upsert failed: %v", err)
	}

	zero := 0.0
	cfg, err := svc.UpsertSymbolConfig(ctx, "u-cfg3", "000001.SZ", SymbolSignalConfigInput{
		StrategyID: "s1", StopLossPct: &zero,
	})
	if err != nil {
		t.Fatalf("second upsert failed: %v", err)
	}
	if cfg.StopLossPct != 0 {
		t.Errorf("显式传 0 应关闭该规则，got %v", cfg.StopLossPct)
	}
}

func TestUpsertSymbolConfig_RejectsOutOfRangeRiskValues(t *testing.T) {
	svc, _ := setupEmitTest(t)
	ctx := context.Background()

	bad := 150.0
	if _, err := svc.UpsertSymbolConfig(ctx, "u-cfg4", "000001.SZ", SymbolSignalConfigInput{
		StrategyID: "s1", MaxPositionPct: &bad,
	}); err == nil {
		t.Fatal("超范围的 max_position_pct 应被拒绝")
	}

	badStop := -5.0
	if _, err := svc.UpsertSymbolConfig(ctx, "u-cfg4", "000001.SZ", SymbolSignalConfigInput{
		StrategyID: "s1", StopLossPct: &badStop,
	}); err == nil {
		t.Fatal("负数 stop_loss_pct 应被拒绝")
	}
}
