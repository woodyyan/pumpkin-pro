package signal

import (
	"context"
	"errors"
	"testing"
	"time"
)

// ──────────────────────────────────────────────────────────────
// RiskGate 是纯函数，无需 mock 数据库，可对 11 条规则做穷举测试。
// ──────────────────────────────────────────────────────────────

// baseContext 返回一个"已知持仓、可交易、总资金可用、所有风控关闭"的基线上下文。
func baseContext() PositionContext {
	return PositionContext{
		Symbol:           "SZ000001",
		ContextAvailable: true,
		Snapshot: PositionSnapshot{
			DataStatus:        PositionDataKnown,
			Shares:            1000,
			AvgCostPrice:      100,
			TotalCostAmount:   100000,
			LatestPrice:       100,
			MarketValue:       100000,
			UnrealizedPnlPct:  0,
			PositionWeightPct: 10,
			TotalCapital:      1000000,
			TotalExposurePct:  10,
			IsTradable:        true,
			CapitalAvailable:  true,
		},
		Config: EffectiveRiskConfig{
			PositionAwareEnabled: true,
			PersonalizationOn:    true,
			MaxPositionPct:       0,
			MaxTotalExposurePct:  0,
			MaxAddTimes:          0,
			StopLossPct:          0,
			TrailingStopPct:      0,
			PositionLimitBuffer:  0,
		},
	}
}

func hasRule(rules []string, target string) bool {
	for _, r := range rules {
		if r == target {
			return true
		}
	}
	return false
}

// ── 优先级 0：透传闸门 ──

func TestGate_PersonalizationOff_PassesThrough(t *testing.T) {
	ctx := baseContext()
	ctx.Config.PersonalizationOn = false
	ctx.Snapshot.Shares = 0 // 明确空仓：正常会拦截 SELL

	got := EvaluateGate(SideSell, ctx)
	if got.Decision != GateDecisionPass {
		t.Fatalf("关闭个性化后应透传，got decision=%s", got.Decision)
	}
	if got.FinalSide != SideSell {
		t.Errorf("side 应保持 SELL，got %s", got.FinalSide)
	}
	if !hasRule(got.MatchedRules, RulePersonalizationOff) {
		t.Errorf("应命中 %s，got %v", RulePersonalizationOff, got.MatchedRules)
	}
}

func TestGate_PositionAwareDisabled_PassesThrough(t *testing.T) {
	ctx := baseContext()
	ctx.Config.PositionAwareEnabled = false
	ctx.Snapshot.Shares = 0

	got := EvaluateGate(SideSell, ctx)
	if got.Decision != GateDecisionPass {
		t.Fatalf("关闭持仓感知后应透传，got %s", got.Decision)
	}
	if !hasRule(got.MatchedRules, RuleGateDisabled) {
		t.Errorf("应命中 %s，got %v", RuleGateDisabled, got.MatchedRules)
	}
}

func TestGate_ContextUnavailable_FailOpen(t *testing.T) {
	ctx := baseContext()
	ctx.ContextAvailable = false
	ctx.Snapshot.Shares = 0

	got := EvaluateGate(SideSell, ctx)
	if got.Decision != GateDecisionPass {
		t.Fatalf("上下文不可用必须 fail-open 透传，got %s", got.Decision)
	}
	if !hasRule(got.MatchedRules, RuleContextUnavailable) {
		t.Errorf("应命中 %s，got %v", RuleContextUnavailable, got.MatchedRules)
	}
}

// 这是本功能最关键的边界：未录持仓 ≠ 空仓，绝不能拦截。
func TestGate_PositionUnknown_MustNotSuppressSell(t *testing.T) {
	ctx := baseContext()
	ctx.Snapshot.DataStatus = PositionDataUnknown
	ctx.Snapshot.Shares = 0

	got := EvaluateGate(SideSell, ctx)
	if got.Decision != GateDecisionPass {
		t.Fatalf("未录持仓必须透传而非拦截，got %s", got.Decision)
	}
	if !hasRule(got.MatchedRules, RulePositionUnknown) {
		t.Errorf("应命中 %s，got %v", RulePositionUnknown, got.MatchedRules)
	}
}

func TestGate_PositionUnknown_BuyAlsoPasses(t *testing.T) {
	ctx := baseContext()
	ctx.Snapshot.DataStatus = PositionDataUnknown
	ctx.Config.MaxPositionPct = 5 // 即使超限也不该拦（因为持仓未知）
	ctx.Snapshot.PositionWeightPct = 50

	got := EvaluateGate(SideBuy, ctx)
	if got.Decision != GateDecisionPass {
		t.Fatalf("未录持仓时买入应透传，got %s", got.Decision)
	}
}

// ── 优先级 1：硬止损（override）──

func TestGate_StopLoss_OverridesToSell(t *testing.T) {
	ctx := baseContext()
	ctx.Config.StopLossPct = 8
	ctx.Snapshot.UnrealizedPnlPct = -10 // 亏 10% > 止损 8%

	got := EvaluateGate(SideBuy, ctx) // 策略说买，风控必须否决为卖
	if got.Decision != GateDecisionOverridden {
		t.Fatalf("触发止损应 override，got %s", got.Decision)
	}
	if got.FinalSide != SideSell {
		t.Errorf("止损应强制改写为 SELL，got %s", got.FinalSide)
	}
	if got.SemanticLabel != SemanticStopLoss {
		t.Errorf("语义应为止损离场，got %s", got.SemanticLabel)
	}
	if got.SkipDelivery() {
		t.Error("override 的信号必须推送，不能被跳过")
	}
}

func TestGate_StopLoss_NotTriggeredWhenAboveLine(t *testing.T) {
	ctx := baseContext()
	ctx.Config.StopLossPct = 8
	ctx.Snapshot.UnrealizedPnlPct = -5 // 未到止损线

	got := EvaluateGate(SideSell, ctx)
	if got.Decision == GateDecisionOverridden {
		t.Fatal("未达止损线不应 override")
	}
}

func TestGate_StopLoss_SkippedWhenCostInvalid(t *testing.T) {
	ctx := baseContext()
	ctx.Config.StopLossPct = 8
	ctx.Snapshot.AvgCostPrice = 0 // 成本异常 → 无锚点
	ctx.Snapshot.UnrealizedPnlPct = -50

	got := EvaluateGate(SideSell, ctx)
	if got.Decision == GateDecisionOverridden {
		t.Fatal("成本无效时止损应跳过")
	}
}

func TestGate_StopLoss_SkippedWhenNoPosition(t *testing.T) {
	ctx := baseContext()
	ctx.Config.StopLossPct = 8
	ctx.Snapshot.Shares = 0
	ctx.Snapshot.UnrealizedPnlPct = -50

	got := EvaluateGate(SideSell, ctx)
	if got.Decision == GateDecisionOverridden {
		t.Fatal("空仓不应触发止损")
	}
}

func TestGate_StopLoss_DisabledByDefault(t *testing.T) {
	ctx := baseContext() // StopLossPct 默认 0 = 关闭
	ctx.Snapshot.UnrealizedPnlPct = -50

	got := EvaluateGate(SideSell, ctx)
	if got.Decision == GateDecisionOverridden {
		t.Fatal("止损默认关闭（0），不应触发 override")
	}
}

// ── 优先级 2：移动止盈（override）──

func TestGate_TrailingStop_OverridesToSell(t *testing.T) {
	ctx := baseContext()
	ctx.Config.TrailingStopPct = 5
	ctx.Snapshot.PeakPriceSinceBuy = 120
	ctx.Snapshot.DrawdownFromPeakPct = 8

	got := EvaluateGate(SideBuy, ctx)
	if got.Decision != GateDecisionOverridden {
		t.Fatalf("触发移动止盈应 override，got %s", got.Decision)
	}
	if got.FinalSide != SideSell || got.SemanticLabel != SemanticTakeStop {
		t.Errorf("应为止盈离场 SELL，got side=%s label=%s", got.FinalSide, got.SemanticLabel)
	}
}

// 止损优先级高于移动止盈。
func TestGate_StopLoss_HasPriorityOverTrailingStop(t *testing.T) {
	ctx := baseContext()
	ctx.Config.StopLossPct = 8
	ctx.Snapshot.UnrealizedPnlPct = -10
	ctx.Config.TrailingStopPct = 5
	ctx.Snapshot.PeakPriceSinceBuy = 120
	ctx.Snapshot.DrawdownFromPeakPct = 20

	got := EvaluateGate(SideSell, ctx)
	if got.SemanticLabel != SemanticStopLoss {
		t.Fatalf("止损优先级应高于移动止盈，got %s", got.SemanticLabel)
	}
}

// ── 优先级 3：空仓收到卖出 → 拦截 ──

func TestGate_SellWithNoPosition_Suppressed(t *testing.T) {
	ctx := baseContext()
	ctx.Snapshot.Shares = 0

	got := EvaluateGate(SideSell, ctx)
	if got.Decision != GateDecisionSuppressed {
		t.Fatalf("空仓收到卖出应拦截，got %s", got.Decision)
	}
	if got.SuppressedReason != RuleSellNoPosition {
		t.Errorf("拦截原因应为 %s，got %s", RuleSellNoPosition, got.SuppressedReason)
	}
	if !got.SkipDelivery() {
		t.Error("被拦截的信号必须跳过推送")
	}
	if got.Message() == "" {
		t.Error("拦截必须有用户可读说明（静默必须可解释）")
	}
}

// ── 优先级 4/5/6：买入类上限拦截 ──

func TestGate_BuyWhenPositionFull_Suppressed(t *testing.T) {
	ctx := baseContext()
	ctx.Config.MaxPositionPct = 10
	ctx.Snapshot.PositionWeightPct = 12

	got := EvaluateGate(SideBuy, ctx)
	if got.Decision != GateDecisionSuppressed || got.SuppressedReason != RuleBuyPositionFull {
		t.Fatalf("超单票上限应拦截，got decision=%s reason=%s", got.Decision, got.SuppressedReason)
	}
}

func TestGate_BuyPositionLimit_SkippedWhenCapitalMissing(t *testing.T) {
	ctx := baseContext()
	ctx.Config.MaxPositionPct = 10
	ctx.Snapshot.PositionWeightPct = 50
	ctx.Snapshot.CapitalAvailable = false // 总资金未填 → 仓位类风控跳过

	got := EvaluateGate(SideBuy, ctx)
	if got.Decision == GateDecisionSuppressed {
		t.Fatal("总资金未填时仓位类风控必须跳过，不能拦截")
	}
	if !hasRule(got.Notes, RuleCapitalMissing) {
		t.Errorf("应附加总资金缺失提示，got notes=%v", got.Notes)
	}
}

func TestGate_BuyWhenExposureFull_Suppressed(t *testing.T) {
	ctx := baseContext()
	ctx.Config.MaxTotalExposurePct = 80
	ctx.Snapshot.TotalExposurePct = 85

	got := EvaluateGate(SideBuy, ctx)
	if got.Decision != GateDecisionSuppressed || got.SuppressedReason != RuleBuyExposureFull {
		t.Fatalf("超总敞口应拦截，got decision=%s reason=%s", got.Decision, got.SuppressedReason)
	}
}

func TestGate_BuyWhenAddTimesExceeded_Suppressed(t *testing.T) {
	ctx := baseContext()
	ctx.Config.MaxAddTimes = 3
	ctx.Snapshot.AddTimes = 3

	got := EvaluateGate(SideBuy, ctx)
	if got.Decision != GateDecisionSuppressed || got.SuppressedReason != RuleBuyAddTimes {
		t.Fatalf("超加仓次数应拦截，got decision=%s reason=%s", got.Decision, got.SuppressedReason)
	}
}

func TestGate_AddTimesLimit_NotAppliedWhenEmpty(t *testing.T) {
	ctx := baseContext()
	ctx.Config.MaxAddTimes = 1
	ctx.Snapshot.Shares = 0 // 空仓：首次建仓不算加仓
	ctx.Snapshot.AddTimes = 5

	got := EvaluateGate(SideBuy, ctx)
	if got.Decision == GateDecisionSuppressed {
		t.Fatal("空仓首次建仓不应受加仓次数限制")
	}
}

// 缓冲带：抑制临界值反复翻转。
func TestGate_PositionLimitBuffer_TriggersSlightlyEarly(t *testing.T) {
	ctx := baseContext()
	ctx.Config.MaxPositionPct = 20
	ctx.Config.PositionLimitBuffer = 0.5
	ctx.Snapshot.PositionWeightPct = 19.6 // 落在缓冲带内

	got := EvaluateGate(SideBuy, ctx)
	if got.Decision != GateDecisionSuppressed {
		t.Fatalf("缓冲带内应提前拦截以抑制震荡，got %s", got.Decision)
	}
}

// ── 优先级 7：不可交易 ──

func TestGate_NotTradable_Suppressed(t *testing.T) {
	ctx := baseContext()
	ctx.Snapshot.IsTradable = false

	for _, side := range []string{SideBuy, SideSell} {
		got := EvaluateGate(side, ctx)
		if got.Decision != GateDecisionSuppressed || got.SuppressedReason != RuleNotTradable {
			t.Errorf("停牌时 %s 应拦截，got decision=%s reason=%s", side, got.Decision, got.SuppressedReason)
		}
	}
}

// 止损优先级高于停牌拦截（风控最高优先）。
func TestGate_StopLoss_HasPriorityOverNotTradable(t *testing.T) {
	ctx := baseContext()
	ctx.Snapshot.IsTradable = false
	ctx.Config.StopLossPct = 8
	ctx.Snapshot.UnrealizedPnlPct = -10

	got := EvaluateGate(SideSell, ctx)
	if got.Decision != GateDecisionOverridden {
		t.Fatalf("止损优先级应高于停牌拦截，got %s", got.Decision)
	}
}

// ── 优先级 8/9/10：语义改写 ──

func TestGate_BuyWithPosition_LabeledAsAdd(t *testing.T) {
	ctx := baseContext()

	got := EvaluateGate(SideBuy, ctx)
	if got.Decision != GateDecisionPass {
		t.Fatalf("正常买入应通过，got %s", got.Decision)
	}
	if got.SemanticLabel != SemanticAdd {
		t.Errorf("已有持仓买入应标记为加仓，got %s", got.SemanticLabel)
	}
}

func TestGate_BuyWithoutPosition_LabeledAsOpen(t *testing.T) {
	ctx := baseContext()
	ctx.Snapshot.Shares = 0

	got := EvaluateGate(SideBuy, ctx)
	if got.Decision != GateDecisionPass || got.SemanticLabel != SemanticOpen {
		t.Fatalf("空仓买入应为首次建仓，got decision=%s label=%s", got.Decision, got.SemanticLabel)
	}
}

func TestGate_SellWithPosition_LabeledAsReduce(t *testing.T) {
	ctx := baseContext()

	got := EvaluateGate(SideSell, ctx)
	if got.Decision != GateDecisionPass || got.SemanticLabel != SemanticReduce {
		t.Fatalf("有持仓卖出应为减仓，got decision=%s label=%s", got.Decision, got.SemanticLabel)
	}
}

// ── 处置效应防护：这是产品原则，必须有测试守住 ──

// 亏损时收到卖出信号，必须照常推送，绝不能因为"亏着"而拦截。
func TestGate_LosingPosition_SellMustNotBeSuppressed(t *testing.T) {
	ctx := baseContext()
	ctx.Snapshot.AvgCostPrice = 100
	ctx.Snapshot.LatestPrice = 50
	ctx.Snapshot.UnrealizedPnlPct = -50 // 巨亏

	got := EvaluateGate(SideSell, ctx)
	if got.Decision == GateDecisionSuppressed {
		t.Fatal("亏损中的卖出信号必须照常推送——按成本决定是否卖出即处置效应，明确禁止")
	}
	if got.SemanticLabel != SemanticReduce {
		t.Errorf("应正常标记为减仓，got %s", got.SemanticLabel)
	}
}

// 盈利时的卖出同样正常，不做任何"盈利才卖"的偏向。
func TestGate_WinningPosition_SellPassesNormally(t *testing.T) {
	ctx := baseContext()
	ctx.Snapshot.UnrealizedPnlPct = 80

	got := EvaluateGate(SideSell, ctx)
	if got.Decision != GateDecisionPass {
		t.Fatalf("盈利卖出应正常通过，got %s", got.Decision)
	}
}

// ── stale 提示 ──

func TestGate_StalePosition_AddsNoteButDoesNotSuppress(t *testing.T) {
	ctx := baseContext()
	ctx.Snapshot.DataStatus = PositionDataStale

	got := EvaluateGate(SideSell, ctx)
	if got.Decision != GateDecisionPass {
		t.Fatalf("持仓过期只提示不拦截，got %s", got.Decision)
	}
	if !hasRule(got.Notes, RulePositionStale) {
		t.Errorf("应附加持仓过期提示，got %v", got.Notes)
	}
}

// ── 辅助函数 ──

func TestGateRuleMessage_AllRulesHaveText(t *testing.T) {
	rules := []string{
		RulePersonalizationOff, RulePositionUnknown, RulePositionStale, RuleCapitalMissing,
		RuleStopLoss, RuleTrailingStop, RuleSellNoPosition, RuleBuyPositionFull,
		RuleBuyExposureFull, RuleBuyAddTimes, RuleNotTradable,
		RuleSemanticAdd, RuleSemanticOpen, RuleSemanticReduce,
		RuleGateDisabled, RuleContextUnavailable,
	}
	for _, rule := range rules {
		if GateRuleMessage(rule) == "" {
			t.Errorf("规则 %s 缺少用户可读文案", rule)
		}
	}
	if GateRuleMessage("") != "" || GateRuleMessage("NOT_A_RULE") != "" {
		t.Error("未知规则码应返回空串")
	}
}

func TestBuildGateSummary(t *testing.T) {
	ctx := baseContext()
	if got := BuildGateSummary(ctx); got == "" {
		t.Error("有持仓时应返回摘要")
	}

	ctx.Snapshot.DataStatus = PositionDataUnknown
	if got := BuildGateSummary(ctx); got != "未录入持仓" {
		t.Errorf("未知持仓摘要应为「未录入持仓」，got %s", got)
	}

	ctx.Snapshot.DataStatus = PositionDataKnown
	ctx.Snapshot.Shares = 0
	if got := BuildGateSummary(ctx); got != "当前空仓" {
		t.Errorf("空仓摘要应为「当前空仓」，got %s", got)
	}
}

func TestPositionContext_HasPositionAndConfirmedEmpty(t *testing.T) {
	ctx := baseContext()
	if !ctx.HasPosition() || ctx.IsConfirmedEmpty() {
		t.Error("已知持仓 1000 股：HasPosition 应为 true，IsConfirmedEmpty 应为 false")
	}

	ctx.Snapshot.Shares = 0
	if ctx.HasPosition() || !ctx.IsConfirmedEmpty() {
		t.Error("已知空仓：HasPosition 应为 false，IsConfirmedEmpty 应为 true")
	}

	// unknown 既不算持仓也不算确认空仓——这是最关键的语义。
	ctx.Snapshot.DataStatus = PositionDataUnknown
	if ctx.HasPosition() || ctx.IsConfirmedEmpty() {
		t.Error("未知持仓：HasPosition 与 IsConfirmedEmpty 都应为 false")
	}
}

// ──────────────────────────────────────────────────────────────
// PositionContextBuilder 装配层测试（用 fake 数据源，无需数据库）
// ──────────────────────────────────────────────────────────────

type fakePositionReader struct {
	position    *SignalPositionData
	positionErr error
	profile     *SignalAccountRiskProfile
	profileErr  error
	list        []SignalPositionData
	listErr     error
}

func (f *fakePositionReader) GetPositionForSignal(ctx context.Context, userID, symbol string) (*SignalPositionData, error) {
	return f.position, f.positionErr
}

func (f *fakePositionReader) ListPositionsForSignal(ctx context.Context, userID string) ([]SignalPositionData, error) {
	return f.list, f.listErr
}

func (f *fakePositionReader) GetAccountRiskProfileForSignal(ctx context.Context, userID string) (*SignalAccountRiskProfile, error) {
	return f.profile, f.profileErr
}

func defaultBuildInput() BuildInput {
	return BuildInput{
		UserID:      "u1",
		Symbol:      "SZ000001",
		LatestPrice: 110,
		IsTradable:  true,
		Config:      SymbolSignalConfigRecord{PositionAwareEnabled: true},
		Now:         time.Now().UTC(),
	}
}

func TestBuilder_NilReader_ContextUnavailable(t *testing.T) {
	builder := NewPositionContextBuilder(nil)
	got := builder.Build(context.Background(), defaultBuildInput())
	if got.ContextAvailable {
		t.Fatal("reader 为 nil 时上下文应不可用（fail-open）")
	}
}

func TestBuilder_ReaderError_FailOpen(t *testing.T) {
	builder := NewPositionContextBuilder(&fakePositionReader{
		profileErr: errors.New("db down"),
	})
	got := builder.Build(context.Background(), defaultBuildInput())
	if got.ContextAvailable {
		t.Fatal("读取失败时应标记上下文不可用，交由门控 fail-open")
	}
	// 并且门控必须放行
	if EvaluateGate(SideSell, got).Decision != GateDecisionPass {
		t.Error("上下文不可用时门控必须放行")
	}
}

func TestBuilder_PositionNotFound_MarkedUnknown(t *testing.T) {
	builder := NewPositionContextBuilder(&fakePositionReader{
		position: &SignalPositionData{Symbol: "SZ000001", Found: false},
		profile:  &SignalAccountRiskProfile{Found: true, TotalCapital: 1000000, PersonalizationOn: true},
	})
	got := builder.Build(context.Background(), defaultBuildInput())
	if !got.ContextAvailable {
		t.Fatal("IO 成功时上下文应可用")
	}
	if got.Snapshot.DataStatus != PositionDataUnknown {
		t.Errorf("未找到持仓应标记 unknown（而非空仓），got %s", got.Snapshot.DataStatus)
	}
}

func TestBuilder_ComputesValuation(t *testing.T) {
	builder := NewPositionContextBuilder(&fakePositionReader{
		position: &SignalPositionData{
			Symbol: "SZ000001", Found: true, Shares: 1000,
			AvgCostPrice: 100, TotalCostAmount: 100000, UpdatedAt: time.Now().UTC(),
		},
		profile: &SignalAccountRiskProfile{Found: true, TotalCapital: 1000000, PersonalizationOn: true},
		list: []SignalPositionData{
			{Symbol: "SZ000001", Shares: 1000, AvgCostPrice: 100},
		},
	})
	got := builder.Build(context.Background(), defaultBuildInput())

	if got.Snapshot.MarketValue != 110000 {
		t.Errorf("市值应为 1000*110=110000，got %v", got.Snapshot.MarketValue)
	}
	if got.Snapshot.UnrealizedPnlPct < 9.99 || got.Snapshot.UnrealizedPnlPct > 10.01 {
		t.Errorf("浮盈应为 +10%%，got %v", got.Snapshot.UnrealizedPnlPct)
	}
	if got.Snapshot.PositionWeightPct < 10.99 || got.Snapshot.PositionWeightPct > 11.01 {
		t.Errorf("权重应为 11%%，got %v", got.Snapshot.PositionWeightPct)
	}
	if !got.Snapshot.CapitalAvailable {
		t.Error("总资金已填时 CapitalAvailable 应为 true")
	}
}

func TestBuilder_NoCapital_MarksCapitalUnavailable(t *testing.T) {
	builder := NewPositionContextBuilder(&fakePositionReader{
		position: &SignalPositionData{Symbol: "SZ000001", Found: true, Shares: 1000, AvgCostPrice: 100, UpdatedAt: time.Now().UTC()},
		profile:  &SignalAccountRiskProfile{Found: true, TotalCapital: 0, PersonalizationOn: true},
	})
	got := builder.Build(context.Background(), defaultBuildInput())
	if got.Snapshot.CapitalAvailable {
		t.Fatal("总资金为 0 时 CapitalAvailable 必须为 false（避免 0 做除数）")
	}
	if got.Snapshot.PositionWeightPct != 0 {
		t.Errorf("总资金缺失时不应计算权重，got %v", got.Snapshot.PositionWeightPct)
	}
}

func TestBuilder_StalePosition(t *testing.T) {
	old := time.Now().UTC().Add(-60 * 24 * time.Hour)
	builder := NewPositionContextBuilder(&fakePositionReader{
		position: &SignalPositionData{Symbol: "SZ000001", Found: true, Shares: 1000, AvgCostPrice: 100, UpdatedAt: old},
		profile:  &SignalAccountRiskProfile{Found: true, TotalCapital: 1000000, PersonalizationOn: true},
	})
	got := builder.Build(context.Background(), defaultBuildInput())
	if got.Snapshot.DataStatus != PositionDataStale {
		t.Errorf("超过 30 天未更新应标记 stale，got %s", got.Snapshot.DataStatus)
	}
}

func TestMergeRiskConfig_SymbolOverridesAccount(t *testing.T) {
	symbolCfg := SymbolSignalConfigRecord{
		PositionAwareEnabled: true,
		MaxPositionPct:       15,
		StopLossPct:          6,
		MaxAddTimes:          2,
	}
	profile := &SignalAccountRiskProfile{
		Found: true, MaxSinglePositionPct: 25, MaxTotalExposurePct: 90,
		DefaultStopLossPct: 10, PersonalizationOn: true,
	}
	got := mergeRiskConfig(symbolCfg, profile)

	if got.MaxPositionPct != 15 {
		t.Errorf("单票级应覆盖账户级，got %v", got.MaxPositionPct)
	}
	if got.StopLossPct != 6 {
		t.Errorf("单票止损应覆盖账户默认，got %v", got.StopLossPct)
	}
	if got.MaxTotalExposurePct != 90 {
		t.Errorf("总敞口应取账户级，got %v", got.MaxTotalExposurePct)
	}
	if got.MaxAddTimes != 2 {
		t.Errorf("加仓次数应取单票级，got %v", got.MaxAddTimes)
	}
}

func TestMergeRiskConfig_FallsBackToDefaults(t *testing.T) {
	got := mergeRiskConfig(SymbolSignalConfigRecord{PositionAwareEnabled: true}, nil)
	if got.MaxPositionPct != defaultMaxSinglePositionPct {
		t.Errorf("无账户配置应回退系统默认单票上限，got %v", got.MaxPositionPct)
	}
	if got.MaxTotalExposurePct != defaultMaxTotalExposurePct {
		t.Errorf("应回退系统默认总敞口，got %v", got.MaxTotalExposurePct)
	}
	if got.StopLossPct != 0 {
		t.Errorf("止损必须默认关闭（0），got %v", got.StopLossPct)
	}
	if !got.PersonalizationOn {
		t.Error("个性化应默认开启")
	}
}

func TestToSnapshotMap_ContainsKeyFields(t *testing.T) {
	snapshot := baseContext().Snapshot
	got := ToSnapshotMap(snapshot)
	for _, key := range []string{
		"data_status", "shares", "avg_cost_price", "latest_price",
		"market_value", "unrealized_pnl_pct", "position_weight_pct",
		"total_capital", "total_exposure_pct", "is_tradable", "capital_available",
	} {
		if _, ok := got[key]; !ok {
			t.Errorf("快照缺少字段 %s", key)
		}
	}
}
