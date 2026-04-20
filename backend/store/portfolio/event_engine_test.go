package portfolio

import "testing"

func TestComputePortfolioEventBuyWeightedAverage(t *testing.T) {
	current := portfolioPosition{Shares: 100, AvgCostPrice: 10, TotalCostAmount: 1000, BuyDate: "2026-04-10", CostMethod: CostMethodWeightedAvg, CostSource: CostSourceSystem}
	result, err := computePortfolioEvent(current, CreatePortfolioEventInput{
		EventType: EventTypeBuy,
		TradeDate: "2026-04-20",
		Quantity:  200,
		Price:     13,
	})
	if err != nil {
		t.Fatalf("computePortfolioEvent buy failed: %v", err)
	}
	if result.After.Shares != 300 {
		t.Fatalf("expected shares 300, got %v", result.After.Shares)
	}
	if result.After.AvgCostPrice != 12 {
		t.Fatalf("expected avg cost 12, got %v", result.After.AvgCostPrice)
	}
	if result.After.TotalCostAmount != 3600 {
		t.Fatalf("expected total cost 3600, got %v", result.After.TotalCostAmount)
	}
}

func TestComputePortfolioEventSellKeepsAverageCost(t *testing.T) {
	current := portfolioPosition{Shares: 300, AvgCostPrice: 12, TotalCostAmount: 3600, BuyDate: "2026-04-10", CostMethod: CostMethodWeightedAvg, CostSource: CostSourceSystem}
	result, err := computePortfolioEvent(current, CreatePortfolioEventInput{
		EventType: EventTypeSell,
		TradeDate: "2026-04-23",
		Quantity:  100,
		Price:     13,
	})
	if err != nil {
		t.Fatalf("computePortfolioEvent sell failed: %v", err)
	}
	if result.After.Shares != 200 {
		t.Fatalf("expected remaining shares 200, got %v", result.After.Shares)
	}
	if result.After.AvgCostPrice != 12 {
		t.Fatalf("expected avg cost unchanged at 12, got %v", result.After.AvgCostPrice)
	}
	if result.RealizedPnlAmount != 100 {
		t.Fatalf("expected realized pnl 100, got %v", result.RealizedPnlAmount)
	}
}

func TestComputePortfolioEventAdjustAvgCost(t *testing.T) {
	current := portfolioPosition{Shares: 200, AvgCostPrice: 10, TotalCostAmount: 2000, BuyDate: "2026-04-10", CostMethod: CostMethodWeightedAvg, CostSource: CostSourceSystem}
	result, err := computePortfolioEvent(current, CreatePortfolioEventInput{
		EventType:          EventTypeAdjustAvgCost,
		TradeDate:          "2026-04-25",
		ManualAvgCostPrice: 9.5,
		Note:               "补录手续费",
	})
	if err != nil {
		t.Fatalf("computePortfolioEvent adjust failed: %v", err)
	}
	if result.After.Shares != 200 {
		t.Fatalf("expected shares unchanged, got %v", result.After.Shares)
	}
	if result.After.AvgCostPrice != 9.5 {
		t.Fatalf("expected avg cost 9.5, got %v", result.After.AvgCostPrice)
	}
	if result.After.TotalCostAmount != 1900 {
		t.Fatalf("expected total cost 1900, got %v", result.After.TotalCostAmount)
	}
	if result.After.CostSource != CostSourceManual {
		t.Fatalf("expected manual cost source, got %s", result.After.CostSource)
	}
}
