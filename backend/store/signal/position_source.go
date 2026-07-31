package signal

import (
	"context"
	"time"
)

// ──────────────────────────────────────────────────────────────
// 持仓数据源桥接层
//
// 为什么需要这一层：
// portfolio 包已提供结构相同但类型不同的返回值（Go 不做跨包结构体隐式转换）。
// 这里用一个窄接口 PositionSource 描述"我需要什么数据"，
// 再由 NewPortfolioPositionReader 适配成 PositionReader。
//
// 好处：
//  1. signal 不 import portfolio，portfolio 也不 import signal —— 零循环依赖。
//  2. 单测时可注入任意 fake 数据源，无需数据库。
// ──────────────────────────────────────────────────────────────

// RawPositionData 数据源返回的原始持仓字段（与 portfolio 侧字段一一对应）。
type RawPositionData struct {
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

// RawAccountRiskProfile 数据源返回的原始账户风控字段。
type RawAccountRiskProfile struct {
	Found                bool
	TotalCapital         float64
	MaxSinglePositionPct float64
	MaxTotalExposurePct  float64
	DefaultStopLossPct   float64
	PersonalizationOn    bool
}

// PositionSource 由 main.go 用闭包适配 portfolio.Service 实现。
type PositionSource struct {
	GetPosition    func(ctx context.Context, userID, symbol string) (RawPositionData, error)
	ListPositions  func(ctx context.Context, userID string) ([]RawPositionData, error)
	GetRiskProfile func(ctx context.Context, userID string) (RawAccountRiskProfile, error)
}

type portfolioPositionReader struct {
	source PositionSource
}

// NewPortfolioPositionReader 把 PositionSource 适配为 PositionReader。
func NewPortfolioPositionReader(source PositionSource) PositionReader {
	return &portfolioPositionReader{source: source}
}

func (r *portfolioPositionReader) GetPositionForSignal(ctx context.Context, userID, symbol string) (*SignalPositionData, error) {
	if r.source.GetPosition == nil {
		return &SignalPositionData{Symbol: symbol, Found: false}, nil
	}
	raw, err := r.source.GetPosition(ctx, userID, symbol)
	if err != nil {
		return nil, err
	}
	data := toSignalPositionData(raw)
	return &data, nil
}

func (r *portfolioPositionReader) ListPositionsForSignal(ctx context.Context, userID string) ([]SignalPositionData, error) {
	if r.source.ListPositions == nil {
		return nil, nil
	}
	rawItems, err := r.source.ListPositions(ctx, userID)
	if err != nil {
		return nil, err
	}
	items := make([]SignalPositionData, 0, len(rawItems))
	for _, raw := range rawItems {
		items = append(items, toSignalPositionData(raw))
	}
	return items, nil
}

func (r *portfolioPositionReader) GetAccountRiskProfileForSignal(ctx context.Context, userID string) (*SignalAccountRiskProfile, error) {
	if r.source.GetRiskProfile == nil {
		return &SignalAccountRiskProfile{Found: false, PersonalizationOn: true}, nil
	}
	raw, err := r.source.GetRiskProfile(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &SignalAccountRiskProfile{
		Found:                raw.Found,
		TotalCapital:         raw.TotalCapital,
		MaxSinglePositionPct: raw.MaxSinglePositionPct,
		MaxTotalExposurePct:  raw.MaxTotalExposurePct,
		DefaultStopLossPct:   raw.DefaultStopLossPct,
		PersonalizationOn:    raw.PersonalizationOn,
	}, nil
}

func toSignalPositionData(raw RawPositionData) SignalPositionData {
	return SignalPositionData{
		Symbol:          raw.Symbol,
		Found:           raw.Found,
		Shares:          raw.Shares,
		AvgCostPrice:    raw.AvgCostPrice,
		TotalCostAmount: raw.TotalCostAmount,
		BuyDate:         raw.BuyDate,
		AddTimes:        raw.AddTimes,
		LastTradeAt:     raw.LastTradeAt,
		UpdatedAt:       raw.UpdatedAt,
	}
}
