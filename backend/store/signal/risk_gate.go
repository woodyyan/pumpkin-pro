package signal

import (
	"fmt"
	"strings"
)

// ──────────────────────────────────────────────────────────────
// 持仓感知风控门控（RiskGate）
//
// 设计约束（务必遵守）：
//  1. 本文件中的判定逻辑必须保持为「纯函数」：无 IO、无 ctx、无数据库访问。
//     所有数据在 PositionContext 装配阶段（position_context.go）准备完毕。
//     这样 11 条规则可以用结构体驱动做穷举单测，无需 mock 数据库。
//  2. 风控层优先级高于策略层，且可以否决策略（override）。
//  3. 三态决策：pass / suppress / override。
//     不能退化为布尔值，否则无法表达「策略没说卖但风控必须卖」（止损）。
//
// ⚠️ 明确不实现的规则（产品原则，禁止后续加回）：
//   - ❌「亏损时不提示卖出」/「低于成本价不卖」
//   - ❌「只有盈利才允许卖出」
//     原因：成本是沉没成本，市场不关心你的成本。按成本决定是否卖出会把
//     「处置效应（Disposition Effect）」——过早卖出盈利、死扛亏损——硬编码进产品，
//     这是散户长期跑输的主因之一。成本价只允许用于：盈亏展示、税费计算、
//     以及止损（以成本为锚，但作用是让用户「更早离场」而非「不离场」）。
// ──────────────────────────────────────────────────────────────

// 门控决策类型。
const (
	GateDecisionPass       = "pass"
	GateDecisionSuppressed = "suppressed"
	GateDecisionOverridden = "overridden"
)

// 信号方向常量（与 normalizeSide 支持的取值保持一致）。
const (
	SideBuy  = "BUY"
	SideSell = "SELL"
	SideHold = "HOLD"
)

// 持仓数据状态。关键：unknown ≠ 空仓，绝不能把「未录持仓」当作「确认空仓」来拦截信号。
const (
	PositionDataKnown   = "known"
	PositionDataUnknown = "unknown"
	PositionDataStale   = "stale"
)

// 语义标签（供前端展示与 webhook 文案）。
const (
	SemanticOpen     = "首次建仓"
	SemanticAdd      = "加仓"
	SemanticReduce   = "减仓"
	SemanticClose    = "清仓"
	SemanticStopLoss = "止损离场"
	SemanticTakeStop = "止盈离场"
	SemanticRaw      = "策略提示"
)

// 规则码。顺序即优先级（数字越小越先匹配）。
const (
	RulePersonalizationOff = "PERSONALIZATION_OFF"
	RulePositionUnknown    = "POSITION_UNKNOWN"
	RuleStopLoss           = "RISK_STOP_LOSS"
	RuleTrailingStop       = "RISK_TRAILING_STOP"
	RuleSellNoPosition     = "GATE_SELL_NO_POSITION"
	RuleBuyPositionFull    = "GATE_BUY_POSITION_FULL"
	RuleBuyExposureFull    = "GATE_BUY_EXPOSURE_FULL"
	RuleBuyAddTimes        = "GATE_BUY_ADD_TIMES"
	RuleNotTradable        = "GATE_NOT_TRADABLE"
	RuleSemanticAdd        = "SEMANTIC_ADD"
	RuleSemanticOpen       = "SEMANTIC_OPEN"
	RuleSemanticReduce     = "SEMANTIC_REDUCE"
	RulePositionStale      = "POSITION_STALE"
	RuleCapitalMissing     = "CAPITAL_MISSING"
	RuleGateDisabled       = "POSITION_AWARE_DISABLED"
	RuleContextUnavailable = "POSITION_CONTEXT_UNAVAILABLE"
)

// gateRuleMessages 规则码 → 面向用户的中文说明。
// 文案口径：统一使用「研究参考」措辞，不出现指令式表达（如"立即卖出"）。
var gateRuleMessages = map[string]string{
	RulePersonalizationOff: "你已关闭个性化，本提示为策略原始输出，未结合持仓做校验。",
	RulePositionUnknown:    "你尚未录入该股持仓，本提示未做持仓校验。建议先在「持仓管理」录入，提示会更贴合你的实际情况。",
	RulePositionStale:      "该股持仓信息较久未更新，本提示基于你最近一次录入的持仓，请留意是否已有变动。",
	RuleCapitalMissing:     "你尚未填写总资金，仓位比例类风控（单票上限、总敞口）暂未生效。",
	RuleStopLoss:           "已触发你设置的止损线，建议关注离场时机。",
	RuleTrailingStop:       "自持仓高点回撤已达你设置的移动止盈幅度，建议关注保护利润。",
	RuleSellNoPosition:     "你当前未持有该股，卖出提示已归档但未推送。",
	RuleBuyPositionFull:    "该股持仓占比已达你设置的单票上限，买入提示已归档但未推送。",
	RuleBuyExposureFull:    "你的总持仓占比已达设置上限，买入提示已归档但未推送。",
	RuleBuyAddTimes:        "该股加仓次数已达你设置的上限，买入提示已归档但未推送。",
	RuleNotTradable:        "该股当前停牌或不可交易，提示已归档但未推送。",
	RuleSemanticAdd:        "你已持有该股，策略提示为加仓方向。",
	RuleSemanticOpen:       "你当前空仓，策略提示为建仓方向。",
	RuleSemanticReduce:     "你当前持有该股，策略提示为减仓方向。",
	RuleGateDisabled:       "该股未启用持仓感知，本提示为策略原始输出。",
	RuleContextUnavailable: "持仓信息暂时读取失败，本提示未做持仓校验。",
}

// GateRuleMessage 返回规则码对应的用户可读说明；未知规则码返回空串。
func GateRuleMessage(code string) string {
	if code == "" {
		return ""
	}
	if msg, ok := gateRuleMessages[code]; ok {
		return msg
	}
	return ""
}

// EffectiveRiskConfig 合并后的有效风控参数（单票 > 账户 > 默认）。
// 所有 0 值统一表示「该规则关闭/不限制」。
type EffectiveRiskConfig struct {
	PositionAwareEnabled bool
	PersonalizationOn    bool
	MaxPositionPct       float64 // 单票市值占总资金上限(%)，0=不限
	MaxTotalExposurePct  float64 // 总敞口上限(%)，0=不限
	MaxAddTimes          int     // 最大加仓次数，0=不限
	StopLossPct          float64 // 止损线(%)，0=关闭
	TrailingStopPct      float64 // 移动止盈回撤(%)，0=关闭
	PositionLimitBuffer  float64 // 上限判定缓冲带(百分点)，抑制临界值反复翻转
}

// PositionSnapshot 决策时点的持仓与估值快照（会原样存入 signal_events 便于事后归因）。
type PositionSnapshot struct {
	DataStatus          string  `json:"data_status"`
	Shares              float64 `json:"shares"`
	AvgCostPrice        float64 `json:"avg_cost_price"`
	TotalCostAmount     float64 `json:"total_cost_amount"`
	BuyDate             string  `json:"buy_date,omitempty"`
	AddTimes            int     `json:"add_times"`
	LatestPrice         float64 `json:"latest_price"`
	MarketValue         float64 `json:"market_value"`
	UnrealizedPnlPct    float64 `json:"unrealized_pnl_pct"`
	PositionWeightPct   float64 `json:"position_weight_pct"`
	PeakPriceSinceBuy   float64 `json:"peak_price_since_buy,omitempty"`
	DrawdownFromPeakPct float64 `json:"drawdown_from_peak_pct,omitempty"`
	TotalCapital        float64 `json:"total_capital"`
	TotalExposurePct    float64 `json:"total_exposure_pct"`
	IsTradable          bool    `json:"is_tradable"`
	CapitalAvailable    bool    `json:"capital_available"`
}

// PositionContext RiskGate 的完整输入（纯数据，装配层负责填充）。
type PositionContext struct {
	Symbol   string
	Snapshot PositionSnapshot
	Config   EffectiveRiskConfig
	// ContextAvailable=false 表示持仓信息读取失败（fail-open：照发信号，仅标注未校验）。
	ContextAvailable bool
}

// HasPosition 仅在「明确知道持仓且份额>0」时为 true。
func (c PositionContext) HasPosition() bool {
	return c.Snapshot.DataStatus != PositionDataUnknown && c.Snapshot.Shares > 0
}

// IsConfirmedEmpty 仅在「明确知道且份额<=0」时为 true。unknown 不算空仓。
func (c PositionContext) IsConfirmedEmpty() bool {
	return c.Snapshot.DataStatus != PositionDataUnknown && c.Snapshot.Shares <= 0
}

// GateDecision RiskGate 的输出。
type GateDecision struct {
	Decision         string
	FinalSide        string
	SemanticLabel    string
	SuppressedReason string
	MatchedRules     []string
	Notes            []string // 附加提示（如 unknown/stale/总资金缺失），不影响决策
}

// SkipDelivery 被拦截的信号只落库不推送。
func (d GateDecision) SkipDelivery() bool {
	return d.Decision == GateDecisionSuppressed
}

// Message 返回主导本次决策的用户可读说明。
func (d GateDecision) Message() string {
	if d.SuppressedReason != "" {
		return GateRuleMessage(d.SuppressedReason)
	}
	if len(d.MatchedRules) > 0 {
		return GateRuleMessage(d.MatchedRules[0])
	}
	return ""
}

// EvaluateGate 按优先级执行门控规则，返回三态决策。
//
// 优先级（先匹配者决定 decision，与 combo_grid 的「熔断>止损>突破>网格>择时」同构）：
//
//	0  PERSONALIZATION_OFF / POSITION_AWARE_DISABLED / CONTEXT_UNAVAILABLE / POSITION_UNKNOWN → pass（透传）
//	1  RISK_STOP_LOSS        → override（强制 SELL）
//	2  RISK_TRAILING_STOP    → override（强制 SELL）
//	3  GATE_SELL_NO_POSITION → suppress
//	4  GATE_BUY_POSITION_FULL→ suppress
//	5  GATE_BUY_EXPOSURE_FULL→ suppress
//	6  GATE_BUY_ADD_TIMES    → suppress
//	7  GATE_NOT_TRADABLE     → suppress
//	8+ SEMANTIC_*            → pass（仅改写语义标签）
func EvaluateGate(rawSide string, ctx PositionContext) GateDecision {
	side := strings.ToUpper(strings.TrimSpace(rawSide))
	decision := GateDecision{
		Decision:     GateDecisionPass,
		FinalSide:    side,
		MatchedRules: []string{},
		Notes:        []string{},
	}

	// ── 优先级 0：透传闸门（不做任何持仓校验，等同改造前行为）──
	if !ctx.Config.PersonalizationOn {
		return passThrough(decision, RulePersonalizationOff)
	}
	if !ctx.Config.PositionAwareEnabled {
		return passThrough(decision, RuleGateDisabled)
	}
	if !ctx.ContextAvailable {
		// fail-open：风控组件不可用不应让用户彻底收不到信号。
		return passThrough(decision, RuleContextUnavailable)
	}
	if ctx.Snapshot.DataStatus == PositionDataUnknown {
		// 关键：未录持仓 ≠ 空仓，必须透传而非拦截。
		return passThrough(decision, RulePositionUnknown)
	}

	// 非决策性提示（不改变 decision，仅附加说明）。
	if ctx.Snapshot.DataStatus == PositionDataStale {
		decision.Notes = append(decision.Notes, RulePositionStale)
	}
	if !ctx.Snapshot.CapitalAvailable {
		decision.Notes = append(decision.Notes, RuleCapitalMissing)
	}

	// ── 优先级 1：硬止损（override，风控否决策略）──
	// 仅在成本有效（>0）且确有持仓时判定；成本异常则跳过（无锚点）。
	if ctx.Config.StopLossPct > 0 && ctx.HasPosition() && ctx.Snapshot.AvgCostPrice > 0 &&
		ctx.Snapshot.UnrealizedPnlPct <= -ctx.Config.StopLossPct {
		decision.Decision = GateDecisionOverridden
		decision.FinalSide = SideSell
		decision.SemanticLabel = SemanticStopLoss
		decision.MatchedRules = append(decision.MatchedRules, RuleStopLoss)
		return decision
	}

	// ── 优先级 2：移动止盈（override）──
	if ctx.Config.TrailingStopPct > 0 && ctx.HasPosition() &&
		ctx.Snapshot.PeakPriceSinceBuy > 0 &&
		ctx.Snapshot.DrawdownFromPeakPct >= ctx.Config.TrailingStopPct {
		decision.Decision = GateDecisionOverridden
		decision.FinalSide = SideSell
		decision.SemanticLabel = SemanticTakeStop
		decision.MatchedRules = append(decision.MatchedRules, RuleTrailingStop)
		return decision
	}

	// ── 优先级 7（前置）：不可交易时无论买卖都无意义 ──
	if !ctx.Snapshot.IsTradable {
		return suppress(decision, RuleNotTradable)
	}

	switch side {
	case SideSell:
		// ── 优先级 3：空仓收到卖出 → 拦截 ──
		if ctx.IsConfirmedEmpty() {
			return suppress(decision, RuleSellNoPosition)
		}
		// ── 优先级 10：减仓 / 清仓语义 ──
		decision.SemanticLabel = SemanticReduce
		decision.MatchedRules = append(decision.MatchedRules, RuleSemanticReduce)
		return decision

	case SideBuy:
		// ── 优先级 4：单票上限（带缓冲带，抑制临界反复翻转）──
		// 仅在总资金可用时判定，否则比例无分母。
		if ctx.Snapshot.CapitalAvailable && ctx.Config.MaxPositionPct > 0 &&
			ctx.Snapshot.PositionWeightPct >= ctx.Config.MaxPositionPct-ctx.Config.PositionLimitBuffer {
			return suppress(decision, RuleBuyPositionFull)
		}
		// ── 优先级 5：总敞口上限 ──
		if ctx.Snapshot.CapitalAvailable && ctx.Config.MaxTotalExposurePct > 0 &&
			ctx.Snapshot.TotalExposurePct >= ctx.Config.MaxTotalExposurePct-ctx.Config.PositionLimitBuffer {
			return suppress(decision, RuleBuyExposureFull)
		}
		// ── 优先级 6：加仓次数上限（仅对已有持仓的加仓行为生效）──
		if ctx.Config.MaxAddTimes > 0 && ctx.HasPosition() &&
			ctx.Snapshot.AddTimes >= ctx.Config.MaxAddTimes {
			return suppress(decision, RuleBuyAddTimes)
		}
		// ── 优先级 8/9：加仓 vs 首次建仓语义 ──
		if ctx.HasPosition() {
			decision.SemanticLabel = SemanticAdd
			decision.MatchedRules = append(decision.MatchedRules, RuleSemanticAdd)
		} else {
			decision.SemanticLabel = SemanticOpen
			decision.MatchedRules = append(decision.MatchedRules, RuleSemanticOpen)
		}
		return decision
	}

	// 其它 side（理论上不会到这里，evaluator 已过滤 HOLD）：原样透传。
	decision.SemanticLabel = SemanticRaw
	return decision
}

func passThrough(decision GateDecision, rule string) GateDecision {
	decision.Decision = GateDecisionPass
	decision.SemanticLabel = SemanticRaw
	decision.MatchedRules = append(decision.MatchedRules, rule)
	return decision
}

func suppress(decision GateDecision, rule string) GateDecision {
	decision.Decision = GateDecisionSuppressed
	decision.SuppressedReason = rule
	decision.MatchedRules = append(decision.MatchedRules, rule)
	return decision
}

// BuildGateSummary 生成一段简短的持仓上下文描述，用于信号推送文案。
func BuildGateSummary(ctx PositionContext) string {
	if ctx.Snapshot.DataStatus == PositionDataUnknown {
		return "未录入持仓"
	}
	if ctx.Snapshot.Shares <= 0 {
		return "当前空仓"
	}
	return fmt.Sprintf("持有 %.0f 股，成本 %.3f，浮动盈亏 %+.2f%%",
		ctx.Snapshot.Shares, ctx.Snapshot.AvgCostPrice, ctx.Snapshot.UnrealizedPnlPct)
}
