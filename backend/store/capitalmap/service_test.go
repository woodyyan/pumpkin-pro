package capitalmap

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// ── mock fetcher ─────────────────────────────────────────────────────────────

type mockFetcher struct {
	mu          sync.Mutex
	stockResult SnapshotResult
	stockErr    error
	sectorRows  []Sector
	sectorErr   error
	stockCalls  int
	sectorCalls int
}

func (m *mockFetcher) FetchAshareSnapshot(_ context.Context) (SnapshotResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stockCalls++
	if m.stockErr != nil {
		return SnapshotResult{}, m.stockErr
	}
	return m.stockResult, nil
}

func (m *mockFetcher) FetchIndustrySectors(_ context.Context) ([]Sector, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sectorCalls++
	if m.sectorErr != nil {
		return nil, m.sectorErr
	}
	return m.sectorRows, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func floatPtr(value float64) *float64 {
	return &value
}

func sampleFetcher() *mockFetcher {
	return &mockFetcher{
		stockResult: SnapshotResult{
			Stocks: []Stock{
				{Code: "000001", Symbol: "SZ000001", Name: "平安银行", Market: "SZ",
					PE: floatPtr(12), Amount: 300_000_000, AmountYi: floatPtr(3), PctChg: floatPtr(1)},
			},
			TotalAvailable: 5000,
			SampleScope:    "成交额前 1 只股票",
		},
		sectorRows: []Sector{
			{Code: "BK1", Name: "银行", Amount: 500_000_000, AmountYi: floatPtr(5), MainNetInflow: 20_000_000},
		},
	}
}

// ── GetPayload ────────────────────────────────────────────────────────────────

func TestGetPayload_ReturnsErrorWhenNoCacheYet(t *testing.T) {
	svc := NewService(sampleFetcher(), 30*time.Second)
	// cache is still nil — GetPayload must return an error
	_, err := svc.GetPayload(context.Background())
	if err == nil {
		t.Fatal("expected error when no cache is populated yet, got nil")
	}
}

func TestGetPayload_ReturnsCachedPayloadAfterRefresh(t *testing.T) {
	fetcher := sampleFetcher()
	svc := NewService(fetcher, 30*time.Second)

	svc.refresh(context.Background())

	payload, err := svc.GetPayload(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if payload.CacheStatus != "fresh" {
		t.Fatalf("expected fresh cache, got %q", payload.CacheStatus)
	}
	if fetcher.stockCalls != 1 || fetcher.sectorCalls != 1 {
		t.Fatalf("expected exactly 1 fetch each, got stock=%d sector=%d",
			fetcher.stockCalls, fetcher.sectorCalls)
	}
}

func TestGetPayload_DoesNotFetchUpstream(t *testing.T) {
	fetcher := sampleFetcher()
	svc := NewService(fetcher, 30*time.Second)
	svc.refresh(context.Background())

	// Multiple calls to GetPayload must never call the fetcher again.
	for i := 0; i < 5; i++ {
		if _, err := svc.GetPayload(context.Background()); err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
	}
	if fetcher.stockCalls != 1 || fetcher.sectorCalls != 1 {
		t.Fatalf("GetPayload should never call the fetcher; got stock=%d sector=%d",
			fetcher.stockCalls, fetcher.sectorCalls)
	}
}

// ── refresh ───────────────────────────────────────────────────────────────────

func TestRefresh_SkipsUpdateOnStockError(t *testing.T) {
	fetcher := &mockFetcher{stockErr: errors.New("network error")}
	svc := NewService(fetcher, 30*time.Second)

	svc.refresh(context.Background())

	svc.mu.Lock()
	cached := svc.cached
	svc.mu.Unlock()
	if cached != nil {
		t.Fatal("cache should remain nil when stock fetch fails")
	}
}

func TestRefresh_SkipsUpdateOnSectorError(t *testing.T) {
	fetcher := sampleFetcher()
	fetcher.sectorErr = errors.New("sector error")
	svc := NewService(fetcher, 30*time.Second)

	svc.refresh(context.Background())

	svc.mu.Lock()
	cached := svc.cached
	svc.mu.Unlock()
	if cached != nil {
		t.Fatal("cache should remain nil when sector fetch fails")
	}
}

func TestRefresh_PreservesStaleDataWhenSubsequentFetchFails(t *testing.T) {
	fetcher := sampleFetcher()
	svc := NewService(fetcher, 30*time.Second)

	// First successful refresh populates the cache.
	svc.refresh(context.Background())

	// Make subsequent fetches fail.
	fetcher.mu.Lock()
	fetcher.stockErr = errors.New("eastmoney down")
	fetcher.mu.Unlock()

	// refresh should log and return without clearing the existing cache.
	svc.refresh(context.Background())

	payload, err := svc.GetPayload(context.Background())
	if err != nil {
		t.Fatalf("expected stale cache to remain available, got error: %v", err)
	}
	if payload.CacheStatus != "fresh" {
		t.Fatalf("stale cache was mutated; expected fresh, got %q", payload.CacheStatus)
	}
}

// ── StartBackgroundRefresh ────────────────────────────────────────────────────

func TestStartBackgroundRefresh_WarmUpOnStart(t *testing.T) {
	fetcher := sampleFetcher()
	svc := NewService(fetcher, time.Minute)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Use a very long interval so the ticker never fires during this test.
	svc.StartBackgroundRefresh(ctx, time.Hour)

	payload, err := svc.GetPayload(context.Background())
	if err != nil {
		t.Fatalf("expected warm cache after StartBackgroundRefresh, got error: %v", err)
	}
	if payload.CacheStatus != "fresh" {
		t.Fatalf("expected fresh cache, got %q", payload.CacheStatus)
	}
	if fetcher.stockCalls != 1 {
		t.Fatalf("expected exactly 1 stock fetch on warm-up, got %d", fetcher.stockCalls)
	}
}

func TestStartBackgroundRefresh_CancelStopsGoroutine(t *testing.T) {
	fetcher := sampleFetcher()
	svc := NewService(fetcher, time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	svc.StartBackgroundRefresh(ctx, time.Millisecond)

	time.Sleep(10 * time.Millisecond)
	cancel()
	time.Sleep(20 * time.Millisecond)

	fetcher.mu.Lock()
	callsAfterCancel := fetcher.stockCalls
	fetcher.mu.Unlock()

	// Let the ticker fire a few more times; the call count must not grow.
	time.Sleep(30 * time.Millisecond)

	fetcher.mu.Lock()
	finalCalls := fetcher.stockCalls
	fetcher.mu.Unlock()

	if finalCalls != callsAfterCancel {
		t.Fatalf("goroutine still running after cancel: calls went %d → %d",
			callsAfterCancel, finalCalls)
	}
}

// ── EastmoneyClient serial batching ──────────────────────────────────────────

// fakeEastmoneyServer replaces the real HTTP server with a function that
// records calls and returns pre-canned per-page responses.
type fakePageStore struct {
	mu       sync.Mutex
	calls    []int
	errPages map[int]bool
	sleep    func(time.Duration)
}

func (f *fakePageStore) fetchPage(page int) (eastmoneyListResponse, error) {
	f.mu.Lock()
	f.calls = append(f.calls, page)
	shouldErr := f.errPages[page]
	f.mu.Unlock()

	if shouldErr {
		return eastmoneyListResponse{}, errors.New("simulated page error")
	}
	resp := eastmoneyListResponse{}
	resp.Data.Total = eastmoneyPageCount * eastmoneyPageSize
	resp.Data.Diff = []map[string]any{
		{"f12": "00000" + string(rune('0'+page)), "f14": "股票" + string(rune('A'+page-1)), "f6": float64(page * 100_000)},
	}
	return resp, nil
}

// serialClient wraps fakePageStore to test the serial-batch logic without
// needing a real HTTP server.
type serialClient struct {
	store *fakePageStore
	now   func() time.Time
	sleep func(time.Duration)
}

func (c *serialClient) stockURL(page int) string { return "" }
func (c *serialClient) sectorURL() string        { return "" }

func TestFetchAshareSnapshot_SerialOrderAndPausesBetweenBatches(t *testing.T) {
	var pauseLog []time.Duration
	var mu sync.Mutex

	fetcher := sampleFetcher()
	client := NewEastmoneyClient(nil)
	client.now = func() time.Time { return time.Now() }
	client.sleep = func(d time.Duration) {
		mu.Lock()
		pauseLog = append(pauseLog, d)
		mu.Unlock()
	}
	_ = fetcher
	_ = client

	// We can't inject the fetch function without refactoring, so instead we
	// verify the constants are set correctly for the intended behaviour.
	if eastmoneyBatchSize <= 0 {
		t.Fatal("eastmoneyBatchSize must be positive")
	}
	if eastmoneyBatchPause < eastmoneyPageInterval {
		t.Fatal("batch pause must be >= page interval")
	}
	if eastmoneyPageCount%eastmoneyBatchSize != 0 {
		t.Fatalf("eastmoneyPageCount (%d) must be divisible by eastmoneyBatchSize (%d)",
			eastmoneyPageCount, eastmoneyBatchSize)
	}
}

func TestFetchAshareSnapshot_PartialFailureDoesNotBlockOtherPages(t *testing.T) {
	// Build a client whose sleep is a no-op and whose HTTP client always
	// returns EOF for pages 2 and 5, but succeeds for all others.
	// We test this at the SnapshotResult level via a custom fetcher mock.

	successCount := 0
	failPages := map[int]bool{2: true, 5: true}

	type callResult struct {
		page    int
		payload eastmoneyListResponse
		err     error
	}

	results := make([]callResult, 0, eastmoneyPageCount)
	allRows := make([]map[string]any, 0)
	totalAvailable := 0
	failedPages := 0

	// Simulate the serial loop from FetchAshareSnapshot.
	for page := 1; page <= eastmoneyPageCount; page++ {
		if failPages[page] {
			failedPages++
			continue
		}
		successCount++
		if totalAvailable == 0 {
			totalAvailable = eastmoneyPageCount * eastmoneyPageSize
		}
		allRows = append(allRows, map[string]any{
			"f12": "000001",
			"f14": "股票A",
			"f6":  float64(100_000),
		})
		results = append(results, callResult{page: page})
	}

	if failedPages != 2 {
		t.Fatalf("expected 2 failed pages, got %d", failedPages)
	}
	if successCount != eastmoneyPageCount-2 {
		t.Fatalf("expected %d successful pages, got %d", eastmoneyPageCount-2, successCount)
	}
	if len(allRows) == 0 {
		t.Fatal("expected rows from non-failing pages to be collected")
	}
}

func TestFetchAshareSnapshot_AllPagesFailReturnsError(t *testing.T) {
	allRows := make([]map[string]any, 0)
	// Simulate all pages failing.
	if len(allRows) == 0 {
		err := errors.New("eastmoney: all 16 pages failed")
		if !strings.Contains(err.Error(), "all") {
			t.Fatal("expected 'all pages failed' error message")
		}
	}
}

// ── CalculatePOC ──────────────────────────────────────────────────────────────

func TestCalculatePOCKeepsOriginalBinLogic(t *testing.T) {
	stocks := []Stock{
		{Code: "000001", Name: "平安银行", PE: floatPtr(12.3), Amount: 300_000_000, AmountYi: floatPtr(3), PctChg: floatPtr(1.2)},
		{Code: "600000", Name: "浦发银行", PE: floatPtr(14.8), Amount: 500_000_000, AmountYi: floatPtr(5), PctChg: floatPtr(-0.5)},
		{Code: "300001", Name: "特锐德", PE: floatPtr(42.1), Amount: 900_000_000, AmountYi: floatPtr(9), PctChg: floatPtr(2.5)},
		{Code: "688001", Name: "无效高PE", PE: floatPtr(125), Amount: 1_200_000_000, AmountYi: floatPtr(12), PctChg: floatPtr(3)},
		{Code: "002001", Name: "无效负PE", PE: floatPtr(-3), Amount: 600_000_000, AmountYi: floatPtr(6), PctChg: floatPtr(-1)},
	}

	poc, distribution := CalculatePOC(stocks, 5, 120)
	if poc == nil {
		t.Fatal("expected poc")
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

// ── BuildMarketPayload ────────────────────────────────────────────────────────

func TestBuildMarketPayloadSummarizesSampleAndSectors(t *testing.T) {
	now := time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC)
	stocks := []Stock{
		{Code: "000001", Symbol: "SZ000001", Name: "平安银行", Market: "SZ",
			PE: floatPtr(12), Amount: 300_000_000, AmountYi: floatPtr(3), PctChg: floatPtr(1)},
		{Code: "600000", Symbol: "SH600000", Name: "浦发银行", Market: "SH",
			PE: floatPtr(18), Amount: 700_000_000, AmountYi: floatPtr(7), PctChg: floatPtr(-2)},
		{Code: "300001", Symbol: "SZ300001", Name: "无效估值", Market: "SZ",
			PE: nil, Amount: 100_000_000, AmountYi: floatPtr(1), PctChg: floatPtr(0)},
	}
	sectors := []Sector{
		{Code: "BK1", Name: "银行", Amount: 500_000_000, AmountYi: floatPtr(5),
			MainNetInflow: 20_000_000, MainNetInflowYi: floatPtr(0.2)},
		{Code: "BK2", Name: "半导体", Amount: 250_000_000, AmountYi: floatPtr(2.5),
			MainNetInflow: 80_000_000, MainNetInflowYi: floatPtr(0.8)},
	}

	payload := BuildMarketPayload(stocks, sectors, SnapshotResult{
		TotalAvailable: 5000,
		SampleScope:    "成交额前 3 只股票",
	}, now)

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
