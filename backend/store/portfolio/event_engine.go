package portfolio

import (
	"fmt"
	"strings"
	"time"
)

type portfolioPosition struct {
	Shares          float64
	AvgCostPrice    float64
	TotalCostAmount float64
	BuyDate         string
	Note            string
	CostMethod      string
	CostSource      string
}

type portfolioEventComputation struct {
	Before            portfolioPosition
	After             portfolioPosition
	RealizedPnlAmount float64
	RealizedPnlPct    float64
}

func shanghaiLocation() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("CST", 8*3600)
	}
	return loc
}

func normalizeTradeDate(raw string, now time.Time) (string, time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", time.Time{}, fmt.Errorf("trade_date is required")
	}
	loc := shanghaiLocation()
	t, err := time.ParseInLocation("2006-01-02", raw, loc)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("trade_date format must be YYYY-MM-DD")
	}
	return t.Format("2006-01-02"), t.UTC(), nil
}

func derivePositionFromRecord(record *PortfolioRecord) portfolioPosition {
	if record == nil {
		return portfolioPosition{
			CostMethod: CostMethodWeightedAvg,
			CostSource: CostSourceSystem,
		}
	}
	totalCost := record.TotalCostAmount
	if totalCost == 0 && record.Shares > 0 && record.AvgCostPrice > 0 {
		totalCost = record.Shares * record.AvgCostPrice
	}
	costMethod := strings.TrimSpace(record.CostMethod)
	if costMethod == "" {
		costMethod = CostMethodWeightedAvg
	}
	costSource := strings.TrimSpace(record.CostSource)
	if costSource == "" {
		costSource = CostSourceSystem
	}
	return portfolioPosition{
		Shares:          record.Shares,
		AvgCostPrice:    record.AvgCostPrice,
		TotalCostAmount: totalCost,
		BuyDate:         record.BuyDate,
		Note:            record.Note,
		CostMethod:      costMethod,
		CostSource:      costSource,
	}
}

func computePortfolioEvent(current portfolioPosition, input CreatePortfolioEventInput) (portfolioEventComputation, error) {
	before := current
	beforeTotalCost := before.TotalCostAmount
	if beforeTotalCost == 0 && before.Shares > 0 && before.AvgCostPrice > 0 {
		beforeTotalCost = before.Shares * before.AvgCostPrice
		before.TotalCostAmount = beforeTotalCost
	}

	after := before
	after.Note = strings.TrimSpace(input.Note)
	if after.CostMethod == "" {
		after.CostMethod = CostMethodWeightedAvg
	}
	if after.CostSource == "" {
		after.CostSource = CostSourceSystem
	}

	result := portfolioEventComputation{Before: before, After: after}
	quantity := input.Quantity
	price := input.Price
	feeAmount := input.FeeAmount
	manualAvg := input.ManualAvgCostPrice
	tradeDate := strings.TrimSpace(input.TradeDate)

	switch input.EventType {
	case EventTypeInit:
		if quantity < 0 {
			return portfolioEventComputation{}, fmt.Errorf("初始化持仓数量不能为负数")
		}
		if price < 0 {
			return portfolioEventComputation{}, fmt.Errorf("初始化均价不能为负数")
		}
		result.After.Shares = quantity
		result.After.AvgCostPrice = price
		result.After.TotalCostAmount = quantity * price
		result.After.CostSource = CostSourceManual
		if quantity > 0 {
			result.After.BuyDate = tradeDate
		} else {
			result.After.BuyDate = ""
		}

	case EventTypeBuy:
		if quantity <= 0 {
			return portfolioEventComputation{}, fmt.Errorf("买入数量必须大于 0")
		}
		if price <= 0 {
			return portfolioEventComputation{}, fmt.Errorf("买入价格必须大于 0")
		}
		if feeAmount < 0 {
			return portfolioEventComputation{}, fmt.Errorf("手续费不能为负数")
		}
		buyCost := quantity*price + feeAmount
		result.After.Shares = before.Shares + quantity
		result.After.TotalCostAmount = beforeTotalCost + buyCost
		if result.After.Shares > 0 {
			result.After.AvgCostPrice = result.After.TotalCostAmount / result.After.Shares
		}
		if before.Shares <= 0 || strings.TrimSpace(before.BuyDate) == "" {
			result.After.BuyDate = tradeDate
		}
		result.After.CostSource = CostSourceSystem

	case EventTypeSell:
		if before.Shares <= 0 {
			return portfolioEventComputation{}, fmt.Errorf("当前无持仓，无法卖出")
		}
		if quantity <= 0 {
			return portfolioEventComputation{}, fmt.Errorf("卖出数量必须大于 0")
		}
		if quantity > before.Shares {
			return portfolioEventComputation{}, fmt.Errorf("卖出数量不能超过当前持仓")
		}
		if price <= 0 {
			return portfolioEventComputation{}, fmt.Errorf("卖出价格必须大于 0")
		}
		if feeAmount < 0 {
			return portfolioEventComputation{}, fmt.Errorf("手续费不能为负数")
		}
		result.After.Shares = before.Shares - quantity
		if result.After.Shares > 0 {
			result.After.AvgCostPrice = before.AvgCostPrice
			result.After.TotalCostAmount = result.After.Shares * before.AvgCostPrice
			result.After.BuyDate = before.BuyDate
		} else {
			result.After.AvgCostPrice = 0
			result.After.TotalCostAmount = 0
			result.After.BuyDate = ""
		}
		result.After.CostSource = CostSourceSystem
		result.RealizedPnlAmount = quantity*(price-before.AvgCostPrice) - feeAmount
		if before.AvgCostPrice > 0 {
			result.RealizedPnlPct = ((price / before.AvgCostPrice) - 1) * 100
		}

	case EventTypeAdjustAvgCost:
		if before.Shares <= 0 {
			return portfolioEventComputation{}, fmt.Errorf("当前无持仓，无法调整均价")
		}
		if manualAvg <= 0 {
			return portfolioEventComputation{}, fmt.Errorf("调整后的均价必须大于 0")
		}
		if strings.TrimSpace(input.Note) == "" {
			return portfolioEventComputation{}, fmt.Errorf("调整均价请填写原因说明")
		}
		result.After.Shares = before.Shares
		result.After.AvgCostPrice = manualAvg
		result.After.TotalCostAmount = before.Shares * manualAvg
		result.After.BuyDate = before.BuyDate
		result.After.CostSource = CostSourceManual

	case EventTypeSyncPosition:
		if quantity < 0 {
			return portfolioEventComputation{}, fmt.Errorf("校准后的持仓数量不能为负数")
		}
		if manualAvg < 0 {
			return portfolioEventComputation{}, fmt.Errorf("校准后的均价不能为负数")
		}
		result.After.Shares = quantity
		result.After.AvgCostPrice = manualAvg
		result.After.TotalCostAmount = quantity * manualAvg
		if quantity > 0 {
			if before.Shares <= 0 {
				result.After.BuyDate = tradeDate
			}
		} else {
			result.After.BuyDate = ""
		}
		result.After.CostSource = CostSourceManual

	default:
		return portfolioEventComputation{}, fmt.Errorf("unsupported event_type")
	}

	if result.After.Shares < 0 {
		return portfolioEventComputation{}, fmt.Errorf("持仓数量不能为负数")
	}
	return result, nil
}
