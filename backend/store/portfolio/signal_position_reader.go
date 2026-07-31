package portfolio

import (
	"context"
	"strings"
	"time"
)

// ──────────────────────────────────────────────────────────────
// 交易信号持仓感知适配层
//
// 为 signal 包的 PositionReader 接口提供实现。
// 放在 portfolio 侧的原因：避免 signal 直接依赖 portfolio 的内部模型，
// 也避免两个包互相 import 造成循环依赖（signal 只声明接口，portfolio 提供实现）。
//
// 注意：这里的返回类型刻意使用「结构体值 + 独立字段」而非直接暴露 PortfolioRecord，
// 使 signal 侧不感知 portfolio 的表结构变化。
// ──────────────────────────────────────────────────────────────

// SignalPositionData 单标的持仓事实（字段与 signal.SignalPositionData 对齐）。
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

// SignalAccountRiskProfile 账户级风控与资金口径（字段与 signal 侧对齐）。
type SignalAccountRiskProfile struct {
	Found                bool
	TotalCapital         float64
	MaxSinglePositionPct float64
	MaxTotalExposurePct  float64
	DefaultStopLossPct   float64
	PersonalizationOn    bool
}

// GetPositionForSignal 读取单个标的的持仓事实。
// 未找到记录时返回 Found=false（signal 侧会据此标记 unknown，绝不等同于空仓）。
func (s *Service) GetPositionForSignal(ctx context.Context, userID, symbol string) (*SignalPositionData, error) {
	userID = strings.TrimSpace(userID)
	symbol = strings.TrimSpace(symbol)
	if userID == "" || symbol == "" {
		return &SignalPositionData{Symbol: symbol, Found: false}, nil
	}

	record, err := s.repo.GetBySymbol(ctx, userID, symbol)
	if err != nil {
		if err == ErrNotFound {
			return &SignalPositionData{Symbol: symbol, Found: false}, nil
		}
		return nil, err
	}
	if record == nil {
		return &SignalPositionData{Symbol: symbol, Found: false}, nil
	}

	data := &SignalPositionData{
		Symbol:          record.Symbol,
		Found:           true,
		Shares:          record.Shares,
		AvgCostPrice:    record.AvgCostPrice,
		TotalCostAmount: record.TotalCostAmount,
		BuyDate:         record.BuyDate,
		LastTradeAt:     record.LastTradeAt,
		UpdatedAt:       record.UpdatedAt,
	}
	// 加仓次数 = 有效买入事件数 - 1（首次建仓不计为加仓）。
	// 查询失败不阻断门控：加仓次数为 0 时该规则自然不触发。
	if addTimes, err := s.countAddTimes(ctx, userID, symbol); err == nil {
		data.AddTimes = addTimes
	}
	return data, nil
}

// countAddTimes 统计有效买入事件次数，换算为加仓次数。
func (s *Service) countAddTimes(ctx context.Context, userID, symbol string) (int, error) {
	events, err := s.repo.ListAllActiveEventsAsc(ctx, userID, symbol)
	if err != nil {
		return 0, err
	}
	buyCount := 0
	for _, event := range events {
		if event.IsVoided {
			continue
		}
		if event.EventType == EventTypeBuy {
			buyCount++
		}
	}
	if buyCount <= 1 {
		return 0, nil
	}
	return buyCount - 1, nil
}

// ListPositionsForSignal 读取用户全部持仓（用于计算总敞口）。
func (s *Service) ListPositionsForSignal(ctx context.Context, userID string) ([]SignalPositionData, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, nil
	}
	records, err := s.repo.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	items := make([]SignalPositionData, 0, len(records))
	for _, record := range records {
		items = append(items, SignalPositionData{
			Symbol:          record.Symbol,
			Found:           true,
			Shares:          record.Shares,
			AvgCostPrice:    record.AvgCostPrice,
			TotalCostAmount: record.TotalCostAmount,
			BuyDate:         record.BuyDate,
			LastTradeAt:     record.LastTradeAt,
			UpdatedAt:       record.UpdatedAt,
		})
	}
	return items, nil
}

// GetAccountRiskProfileForSignal 读取账户级风控参数。
// 记录不存在时返回 Found=false + PersonalizationOn=true（默认开启个性化）。
func (s *Service) GetAccountRiskProfileForSignal(ctx context.Context, userID string) (*SignalAccountRiskProfile, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return &SignalAccountRiskProfile{Found: false, PersonalizationOn: true}, nil
	}
	record, err := s.repo.GetInvestmentProfile(ctx, userID)
	if err != nil {
		if err == ErrNotFound {
			return &SignalAccountRiskProfile{Found: false, PersonalizationOn: true}, nil
		}
		return nil, err
	}
	if record == nil {
		return &SignalAccountRiskProfile{Found: false, PersonalizationOn: true}, nil
	}
	return &SignalAccountRiskProfile{
		Found:                true,
		TotalCapital:         record.TotalCapital,
		MaxSinglePositionPct: record.MaxSinglePositionPct,
		MaxTotalExposurePct:  record.MaxTotalExposurePct,
		DefaultStopLossPct:   record.DefaultStopLossPct,
		PersonalizationOn:    record.PersonalizationEnabled,
	}, nil
}
