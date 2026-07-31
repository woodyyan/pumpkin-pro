package signal

import (
	"context"
	"log"
	"math"
	"strings"
	"time"
)

// ──────────────────────────────────────────────────────────────
// PositionContext 装配层
//
// 职责：把所有 IO（持仓查询、投资画像查询）集中在这里完成，
// 使 risk_gate.go 的规则判定保持为可穷举单测的纯函数。
//
// 关键设计：
//  1. fail-open：任何数据读取失败都不拦截信号，仅标记 ContextAvailable=false，
//     降级为改造前行为。风控组件故障不应导致用户彻底收不到信号。
//  2. unknown ≠ 空仓：从未录入持仓时 DataStatus=unknown，规则层会直接透传。
//  3. 总资金未填时 CapitalAvailable=false，仓位比例类风控全部跳过（避免 0 做除数）。
// ──────────────────────────────────────────────────────────────

const (
	// 持仓超过该天数未更新则标记为 stale（仅提示，不拦截）。
	defaultPositionStaleDays = 30
	// 上限判定缓冲带（百分点）：抑制在临界值附近反复翻转。
	defaultPositionLimitBuffer = 0.5
	// 账户级默认值。
	defaultMaxSinglePositionPct = 20.0
	defaultMaxTotalExposurePct  = 100.0
)

// PositionReader 持仓数据读取接口（依赖倒置：signal 不直接依赖 portfolio 包实现）。
type PositionReader interface {
	GetPositionForSignal(ctx context.Context, userID, symbol string) (*SignalPositionData, error)
	GetAccountRiskProfileForSignal(ctx context.Context, userID string) (*SignalAccountRiskProfile, error)
	ListPositionsForSignal(ctx context.Context, userID string) ([]SignalPositionData, error)
}

// SignalPositionData 单个标的的持仓事实（由 portfolio 侧适配填充）。
type SignalPositionData struct {
	Symbol          string
	Found           bool
	Shares          float64
	AvgCostPrice    float64
	TotalCostAmount float64
	BuyDate         string
	AddTimes        int
	LastTradeAt     *time.Time
	UpdatedAt       time.Time
}

// SignalAccountRiskProfile 账户级风控与资金口径。
type SignalAccountRiskProfile struct {
	Found                bool
	TotalCapital         float64
	MaxSinglePositionPct float64
	MaxTotalExposurePct  float64
	DefaultStopLossPct   float64
	PersonalizationOn    bool
}

// PositionContextBuilder 装配 RiskGate 所需的全部输入。
type PositionContextBuilder struct {
	reader PositionReader
}

// NewPositionContextBuilder 创建装配器。reader 为 nil 时所有上下文都返回不可用（fail-open）。
func NewPositionContextBuilder(reader PositionReader) *PositionContextBuilder {
	return &PositionContextBuilder{reader: reader}
}

// BuildInput 装配所需的外部输入。
type BuildInput struct {
	UserID      string
	Symbol      string
	LatestPrice float64
	IsTradable  bool
	Config      SymbolSignalConfigRecord
	Now         time.Time
}

// Build 装配 PositionContext。永不返回 error：任何失败都降级为 ContextAvailable=false。
func (b *PositionContextBuilder) Build(ctx context.Context, input BuildInput) PositionContext {
	result := PositionContext{
		Symbol:           input.Symbol,
		ContextAvailable: false,
		Snapshot: PositionSnapshot{
			DataStatus:  PositionDataUnknown,
			LatestPrice: input.LatestPrice,
			IsTradable:  input.IsTradable,
		},
		Config: EffectiveRiskConfig{
			// 单票开关始终生效，即使上下文不可用也要如实反映用户设置。
			PositionAwareEnabled: input.Config.PositionAwareEnabled,
			PersonalizationOn:    true,
			PositionLimitBuffer:  defaultPositionLimitBuffer,
		},
	}

	if b == nil || b.reader == nil {
		return result
	}

	profile, profileErr := b.reader.GetAccountRiskProfileForSignal(ctx, input.UserID)
	if profileErr != nil {
		log.Printf("[signal-gate] read account risk profile failed user=%s: %v", input.UserID, profileErr)
		return result
	}
	position, posErr := b.reader.GetPositionForSignal(ctx, input.UserID, input.Symbol)
	if posErr != nil {
		log.Printf("[signal-gate] read position failed user=%s symbol=%s: %v", input.UserID, input.Symbol, posErr)
		return result
	}

	// 到此为止 IO 均成功，上下文可用。
	result.ContextAvailable = true
	result.Config = mergeRiskConfig(input.Config, profile)

	snapshot := PositionSnapshot{
		DataStatus:       PositionDataKnown,
		LatestPrice:      input.LatestPrice,
		IsTradable:       input.IsTradable,
		CapitalAvailable: profile != nil && profile.TotalCapital > 0,
	}
	if profile != nil {
		snapshot.TotalCapital = profile.TotalCapital
	}

	if position == nil || !position.Found {
		// 从未录入 → unknown（绝不等同于空仓）。
		snapshot.DataStatus = PositionDataUnknown
		result.Snapshot = snapshot
		return result
	}

	snapshot.Shares = position.Shares
	snapshot.AvgCostPrice = position.AvgCostPrice
	snapshot.TotalCostAmount = position.TotalCostAmount
	snapshot.BuyDate = position.BuyDate
	snapshot.AddTimes = position.AddTimes

	// 持仓过期判定：仅作提示，不影响拦截决策。
	if isPositionStale(position, input.Now, defaultPositionStaleDays) {
		snapshot.DataStatus = PositionDataStale
	}

	// 估值：无有效最新价时不计算市值/权重/浮盈亏（停牌等场景）。
	if input.LatestPrice > 0 && position.Shares > 0 {
		snapshot.MarketValue = position.Shares * input.LatestPrice
		if position.AvgCostPrice > 0 {
			snapshot.UnrealizedPnlPct = (input.LatestPrice/position.AvgCostPrice - 1) * 100
		}
		if snapshot.CapitalAvailable {
			snapshot.PositionWeightPct = snapshot.MarketValue / profile.TotalCapital * 100
		}
	}

	// 总敞口：需要遍历全部持仓，失败则视为不可用（跳过敞口规则）而非拦截。
	if snapshot.CapitalAvailable {
		if exposure, ok := b.computeTotalExposurePct(ctx, input, profile.TotalCapital); ok {
			snapshot.TotalExposurePct = exposure
		} else {
			// 敞口算不出来时，把敞口视作 0 并保留 CapitalAvailable，
			// 单票上限仍可判定，敞口上限因 0 < 上限而自然不触发。
			snapshot.TotalExposurePct = 0
		}
	}

	result.Snapshot = snapshot
	return result
}

// computeTotalExposurePct 计算总持仓市值占总资金比例。
// 说明：为控制单轮评估的开销，其余持仓统一按"成本价"估算市值，
// 仅当前标的使用实时价。这会带来一定偏差，但敞口上限是粗粒度风控，可接受。
func (b *PositionContextBuilder) computeTotalExposurePct(ctx context.Context, input BuildInput, totalCapital float64) (float64, bool) {
	if totalCapital <= 0 {
		return 0, false
	}
	positions, err := b.reader.ListPositionsForSignal(ctx, input.UserID)
	if err != nil {
		log.Printf("[signal-gate] list positions failed user=%s: %v", input.UserID, err)
		return 0, false
	}
	total := 0.0
	target := strings.ToUpper(strings.TrimSpace(input.Symbol))
	for _, p := range positions {
		if p.Shares <= 0 {
			continue
		}
		price := p.AvgCostPrice
		if strings.EqualFold(strings.TrimSpace(p.Symbol), target) && input.LatestPrice > 0 {
			price = input.LatestPrice
		}
		if price <= 0 {
			continue
		}
		total += p.Shares * price
	}
	return total / totalCapital * 100, true
}

// mergeRiskConfig 合并有效风控参数：单票 > 账户 > 系统默认。
// 约定：单票级 0 表示"未设置"，回退账户级；账户级 0 同样回退系统默认。
func mergeRiskConfig(symbolCfg SymbolSignalConfigRecord, profile *SignalAccountRiskProfile) EffectiveRiskConfig {
	cfg := EffectiveRiskConfig{
		PositionAwareEnabled: symbolCfg.PositionAwareEnabled,
		PersonalizationOn:    true,
		MaxAddTimes:          symbolCfg.MaxAddTimes,
		PositionLimitBuffer:  defaultPositionLimitBuffer,
	}

	accountMaxPosition := defaultMaxSinglePositionPct
	accountMaxExposure := defaultMaxTotalExposurePct
	accountStopLoss := 0.0
	if profile != nil && profile.Found {
		cfg.PersonalizationOn = profile.PersonalizationOn
		if profile.MaxSinglePositionPct > 0 {
			accountMaxPosition = profile.MaxSinglePositionPct
		}
		if profile.MaxTotalExposurePct > 0 {
			accountMaxExposure = profile.MaxTotalExposurePct
		}
		if profile.DefaultStopLossPct > 0 {
			accountStopLoss = profile.DefaultStopLossPct
		}
	}

	cfg.MaxPositionPct = accountMaxPosition
	if symbolCfg.MaxPositionPct > 0 {
		cfg.MaxPositionPct = symbolCfg.MaxPositionPct
	}
	cfg.MaxTotalExposurePct = accountMaxExposure

	// 止损默认关闭（0）：属于 override 会强制发卖出信号，必须用户显式设置。
	cfg.StopLossPct = accountStopLoss
	if symbolCfg.StopLossPct > 0 {
		cfg.StopLossPct = symbolCfg.StopLossPct
	}
	cfg.TrailingStopPct = symbolCfg.TrailingStopPct
	return cfg
}

func isPositionStale(position *SignalPositionData, now time.Time, staleDays int) bool {
	if position == nil || staleDays <= 0 {
		return false
	}
	reference := position.UpdatedAt
	if position.LastTradeAt != nil && position.LastTradeAt.After(reference) {
		reference = *position.LastTradeAt
	}
	if reference.IsZero() {
		return false
	}
	return now.Sub(reference) > time.Duration(staleDays)*24*time.Hour
}

// ToSnapshotMap 把快照转成可存库的 map（供 signal_events.position_snapshot_json）。
func ToSnapshotMap(snapshot PositionSnapshot) map[string]any {
	return map[string]any{
		"data_status":         snapshot.DataStatus,
		"shares":              roundFloat(snapshot.Shares, 4),
		"avg_cost_price":      roundFloat(snapshot.AvgCostPrice, 4),
		"total_cost_amount":   roundFloat(snapshot.TotalCostAmount, 2),
		"buy_date":            snapshot.BuyDate,
		"add_times":           snapshot.AddTimes,
		"latest_price":        roundFloat(snapshot.LatestPrice, 4),
		"market_value":        roundFloat(snapshot.MarketValue, 2),
		"unrealized_pnl_pct":  roundFloat(snapshot.UnrealizedPnlPct, 2),
		"position_weight_pct": roundFloat(snapshot.PositionWeightPct, 2),
		"total_capital":       roundFloat(snapshot.TotalCapital, 2),
		"total_exposure_pct":  roundFloat(snapshot.TotalExposurePct, 2),
		"is_tradable":         snapshot.IsTradable,
		"capital_available":   snapshot.CapitalAvailable,
	}
}

func roundFloat(value float64, decimals int) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	pow := math.Pow(10, float64(decimals))
	return math.Round(value*pow) / pow
}
