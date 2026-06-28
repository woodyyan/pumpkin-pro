package capitalmap

import (
	"context"
	"errors"
	"testing"
	"time"
)

type mockFetcher struct {
	stockResult SnapshotResult
	stockErr    error
	sectorRows  []Sector
	sectorErr   error
	stockCalls  int
	sectorCalls int
}

func (m *mockFetcher) FetchAshareSnapshot(ctx context.Context) (SnapshotResult, error) {
	m.stockCalls++
	if m.stockErr != nil {
		return SnapshotResult{}, m.stockErr
	}
	return m.stockResult, nil
}

func (m *mockFetcher) FetchIndustrySectors(ctx context.Context) ([]Sector, error) {
	m.sectorCalls++
	if m.sectorErr != nil {
		return nil, m.sectorErr
	}
	return m.sectorRows, nil
}

func TestCalculatePOCKeepsOriginalBinLogic(t *testing.T) {
	stocks := []Stock{
		{Code: "000001", Name: "平安银行", PE: floatPtr(12.3), Amount: 300000000, AmountYi: floatPtr(3), PctChg: floatPtr(1.2)},
		{Code: "600000", Name: "浦发银行", PE: floatPtr(14.8), Amount: 500000000, AmountYi: floatPtr(5), PctChg: floatPtr(-0.5)},
		{Code: "300001", Name: "特锐德", PE: floatPtr(42.1), Amount: 900000000, AmountYi: floatPtr(9), PctChg: floatPtr(2.5)},
		{Code: "688001", Name: "无效高PE", PE: floatPtr(125), Amount: 1200000000, AmountYi: floatPtr(12), PctChg: floatPtr(3)},
		{Code: "002001", Name: "无效负PE", PE: floatPtr(-3), Amount: 600000000, AmountYi: floatPtr(6), PctChg: floatPtr(-1)},
	}

	poc, distribution := CalculatePOC(stocks, 5, 120)
	if poc == nil {
		t.Fatalf("expected poc")
	}
	if poc.Key != "40-45" {
		t.Fatalf("expected highest amount bin 40-45, got %s", poc.Key)
	}
	if len(distribution) != 2 {
		t.Fatalf("expected 2 valid bins, got %d", len(distribution))
	}
	if distribution[0].Key != "10-15" || distribution[0].StockCount != 2 {
		t.Fatalf("expected first bin 10-15 with 2 stocks, got %+v", distribution[0])
	}
	if distribution[0].TotalAmountYi == nil || *distribution[0].TotalAmountYi != 8 {
		t.Fatalf("expected first bin amount 8 yi, got %+v", distribution[0].TotalAmountYi)
	}
}

func TestBuildMarketPayloadSummarizesSampleAndSectors(t *testing.T) {
	now := time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC)
	stocks := []Stock{
		{Code: "000001", Symbol: "SZ000001", Name: "平安银行", Market: "SZ", PE: floatPtr(12), Amount: 300000000, AmountYi: floatPtr(3), PctChg: floatPtr(1)},
		{Code: "600000", Symbol: "SH600000", Name: "浦发银行", Market: "SH", PE: floatPtr(18), Amount: 700000000, AmountYi: floatPtr(7), PctChg: floatPtr(-2)},
		{Code: "300001", Symbol: "SZ300001", Name: "无效估值", Market: "SZ", PE: nil, Amount: 100000000, AmountYi: floatPtr(1), PctChg: floatPtr(0)},
	}
	sectors := []Sector{
		{Code: "BK1", Name: "银行", Amount: 500000000, AmountYi: floatPtr(5), MainNetInflow: 20000000, MainNetInflowYi: floatPtr(0.2)},
		{Code: "BK2", Name: "半导体", Amount: 250000000, AmountYi: floatPtr(2.5), MainNetInflow: 80000000, MainNetInflowYi: floatPtr(0.8)},
	}

	payload := BuildMarketPayload(stocks, sectors, SnapshotResult{TotalAvailable: 5000, SampleScope: "成交额前 3 只股票"}, now)
	if payload.UpdatedAt != "2026-06-28T12:00:00Z" {
		t.Fatalf("unexpected updatedAt %s", payload.UpdatedAt)
	}
	if payload.Market.StockCount != 5000 || payload.Market.SampleCount != 3 || payload.Market.PositivePECount != 2 {
		t.Fatalf("unexpected market summary %+v", payload.Market)
	}
	if payload.Market.TotalAmountYi == nil || *payload.Market.TotalAmountYi != 11 {
		t.Fatalf("expected total amount 11 yi, got %+v", payload.Market.TotalAmountYi)
	}
	if len(payload.Stocks) != 2 {
		t.Fatalf("expected only valid PE chart stocks, got %d", len(payload.Stocks))
	}
	if len(payload.Sectors) != 2 || payload.Sectors[0].Name != "银行" {
		t.Fatalf("expected sectors sorted by amount, got %+v", payload.Sectors)
	}
	if payload.InflowSectors[0].Name != "半导体" {
		t.Fatalf("expected inflow sectors sorted by main inflow, got %+v", payload.InflowSectors)
	}
}

func TestServiceUsesCacheAndReturnsStaleFallback(t *testing.T) {
	fetcher := &mockFetcher{
		stockResult: SnapshotResult{Stocks: []Stock{{Code: "000001", Symbol: "SZ000001", Name: "平安银行", PE: floatPtr(12), Amount: 300000000, AmountYi: floatPtr(3), PctChg: floatPtr(1)}}},
		sectorRows:  []Sector{{Code: "BK1", Name: "银行", Amount: 500000000, AmountYi: floatPtr(5), MainNetInflow: 20000000}},
	}
	baseTime := time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC)
	service := NewService(fetcher, 30*time.Second)
	service.now = func() time.Time { return baseTime }

	payload, err := service.GetPayload(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if payload.CacheStatus != "fresh" || fetcher.stockCalls != 1 || fetcher.sectorCalls != 1 {
		t.Fatalf("unexpected initial cache state payload=%+v calls=%d/%d", payload, fetcher.stockCalls, fetcher.sectorCalls)
	}

	_, err = service.GetPayload(context.Background())
	if err != nil {
		t.Fatalf("unexpected cached error: %v", err)
	}
	if fetcher.stockCalls != 1 || fetcher.sectorCalls != 1 {
		t.Fatalf("expected fresh cache to avoid refetch, calls=%d/%d", fetcher.stockCalls, fetcher.sectorCalls)
	}

	service.now = func() time.Time { return baseTime.Add(31 * time.Second) }
	fetcher.stockErr = errors.New("eastmoney unavailable")
	stale, err := service.GetPayload(context.Background())
	if err != nil {
		t.Fatalf("expected stale fallback, got err %v", err)
	}
	if stale.CacheStatus != "stale" || stale.LastError == "" {
		t.Fatalf("expected stale fallback with error, got %+v", stale)
	}
}

func floatPtr(value float64) *float64 {
	return &value
}
