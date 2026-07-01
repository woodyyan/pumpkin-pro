package capitalmap

import (
	"context"
	"fmt"
	"log"
	"math"
	"sort"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

const (
	defaultPEBinSize  = 5
	defaultMaxPE      = 120
	chartStockLimit   = 1200
	sectorLimit       = 14
	inflowSectorLimit = 8
)

type Fetcher interface {
	FetchAshareSnapshot(ctx context.Context) (SnapshotResult, error)
	FetchIndustrySectors(ctx context.Context) ([]Sector, error)
}

// onDemandFetchTimeout caps the synchronous fetch that GetPayload triggers
// when the cache is empty (e.g. warm-up failed). This keeps a slow upstream
// from stalling the request while still allowing self-healing.
const onDemandFetchTimeout = 12 * time.Second

type Service struct {
	fetcher Fetcher
	now     func() time.Time

	mu sync.Mutex
	// cached holds the most recent successful payload. When refresh fails but
	// a previous payload exists, cached is kept and tagged stale (see
	// markStale) so callers still get data and can observe staleness via
	// CacheStatus / LastError.
	cached   *Payload
	cachedAt time.Time

	// inflight deduplicates concurrent on-demand fetches triggered by
	// GetPayload when the cache is empty, so a burst of first requests
	// triggers at most one upstream fetch.
	inflight singleflight.Group

	// lastErr / lastErrAt record the most recent refresh failure (warm-up or
	// background) for observability. Cleared on success.
	lastErr   error
	lastErrAt time.Time
}

func NewService(fetcher Fetcher) *Service {
	if fetcher == nil {
		fetcher = NewEastmoneyClient(nil)
	}
	return &Service{fetcher: fetcher, now: time.Now}
}

// StartBackgroundRefresh performs an immediate warm-up fetch, then continues
// refreshing the cache at the given interval in the background.
// The caller should pass a long-lived context (e.g. context.Background()).
// The goroutine exits when ctx is cancelled.
func (s *Service) StartBackgroundRefresh(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = DefaultCacheTTL
	}
	s.refreshAndRecord(ctx)
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.refreshAndRecord(ctx)
			}
		}
	}()
}

// refreshAndRecord runs refresh and records failures for observability.
func (s *Service) refreshAndRecord(ctx context.Context) {
	if err := s.refresh(ctx); err != nil {
		log.Printf("capital map background refresh failed: %v", err)
	}
}

// refresh fetches fresh data from the upstream source and updates the cache.
// On success the cache is marked "fresh". On failure the existing cache (if
// any) is preserved and tagged "stale" with the error; the error is returned
// so callers (warm-up, on-demand fetch) can react. The cache is never wiped
// by a failure, so a transient upstream outage never blanks the page.
func (s *Service) refresh(ctx context.Context) error {
	rctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()

	stockResult, stockErr := s.fetcher.FetchAshareSnapshot(rctx)
	if stockErr != nil {
		s.markStale(stockErr)
		return stockErr
	}
	sectors, sectorErr := s.fetcher.FetchIndustrySectors(rctx)
	if sectorErr != nil {
		s.markStale(sectorErr)
		return sectorErr
	}

	now := s.now()
	payload := BuildMarketPayload(stockResult.Stocks, sectors, stockResult, now)
	payload.CacheStatus = "fresh"
	payload.LastError = ""

	s.mu.Lock()
	s.cached = clonePayload(payload)
	s.cachedAt = now
	s.lastErr = nil
	s.lastErrAt = time.Time{}
	s.mu.Unlock()
	return nil
}

// markStale preserves the existing cache but flags it as stale, so callers
// still receive data during a transient upstream outage. If no cache exists
// yet, only the failure is recorded (for observability) and the cache stays
// nil.
func (s *Service) markStale(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cached != nil {
		s.cached.CacheStatus = "stale"
		s.cached.LastError = err.Error()
	}
	s.lastErr = err
	s.lastErrAt = s.now()
}

// GetPayload returns the most recently cached payload. If the cache is empty
// (warm-up failed or not yet run), it triggers one synchronous fetch
// deduplicated across concurrent callers, so the first request self-heals
// instead of hard erroring. An error is returned only when both the cache and
// the on-demand fetch are unavailable.
func (s *Service) GetPayload(ctx context.Context) (*Payload, error) {
	if s == nil {
		return nil, fmt.Errorf("capital map service is not initialized")
	}

	s.mu.Lock()
	cached := s.cached
	s.mu.Unlock()
	if cached != nil {
		return clonePayload(cached), nil
	}

	// Cache empty — deduplicate concurrent first-fetch attempts.
	_, err, _ := s.inflight.Do("fetch", func() (any, error) {
		// Re-check inside the leader: another caller may have populated the
		// cache between our first check and acquiring the singleflight slot.
		s.mu.Lock()
		if s.cached != nil {
			s.mu.Unlock()
			return nil, nil
		}
		s.mu.Unlock()

		fetchCtx, cancel := context.WithTimeout(ctx, onDemandFetchTimeout)
		defer cancel()
		return nil, s.refresh(fetchCtx)
	})
	if err != nil {
		return nil, fmt.Errorf("资金星图数据暂不可用: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cached == nil {
		return nil, fmt.Errorf("资金星图数据尚未就绪，请稍后重试")
	}
	return clonePayload(s.cached), nil
}

// ServiceStatus exposes the current cache state for observability (admin
// diagnostics). It never triggers a fetch.
type ServiceStatus struct {
	CacheAvailable bool      `json:"cacheAvailable"`
	CacheStatus    string    `json:"cacheStatus"`
	CachedAt       time.Time `json:"cachedAt"`
	LastError      string    `json:"lastError,omitempty"`
	LastErrAt      time.Time `json:"lastErrAt,omitempty"`
}

// Status returns a snapshot of the cache health.
func (s *Service) Status() ServiceStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := ServiceStatus{}
	if s.cached != nil {
		st.CacheAvailable = true
		st.CacheStatus = s.cached.CacheStatus
		st.CachedAt = s.cachedAt
		st.LastError = s.cached.LastError
	}
	if s.lastErr != nil {
		st.LastError = s.lastErr.Error()
		st.LastErrAt = s.lastErrAt
	}
	return st
}

func BuildMarketPayload(stocks []Stock, sectors []Sector, meta SnapshotResult, now time.Time) *Payload {
	positivePEStocks := make([]Stock, 0, len(stocks))
	var totalAmount float64
	upCount := 0
	downCount := 0

	for _, stock := range stocks {
		totalAmount += stock.Amount
		pct := deref(stock.PctChg)
		if pct > 0 {
			upCount++
		} else if pct < 0 {
			downCount++
		}
		if stock.PE != nil && *stock.PE > 0 && *stock.PE <= defaultMaxPE && stock.Amount > 0 {
			positivePEStocks = append(positivePEStocks, stock)
		}
	}
	flatCount := len(stocks) - upCount - downCount
	poc, distribution := CalculatePOC(stocks, defaultPEBinSize, defaultMaxPE)

	chartStocks := append([]Stock(nil), positivePEStocks...)
	sort.Slice(chartStocks, func(i, j int) bool { return chartStocks[i].Amount > chartStocks[j].Amount })
	if len(chartStocks) > chartStockLimit {
		chartStocks = chartStocks[:chartStockLimit]
	}
	for idx := range chartStocks {
		chartStocks[idx].PE = roundPtr(deref(chartStocks[idx].PE), 2)
	}

	topSectors := append([]Sector(nil), sectors...)
	sort.Slice(topSectors, func(i, j int) bool { return topSectors[i].Amount > topSectors[j].Amount })
	if len(topSectors) > sectorLimit {
		topSectors = topSectors[:sectorLimit]
	}
	for idx := range topSectors {
		if totalAmount > 0 {
			topSectors[idx].AmountRatio = roundPtr((topSectors[idx].Amount/totalAmount)*100, 2)
		}
	}

	inflowSectors := append([]Sector(nil), sectors...)
	sort.Slice(inflowSectors, func(i, j int) bool { return inflowSectors[i].MainNetInflow > inflowSectors[j].MainNetInflow })
	if len(inflowSectors) > inflowSectorLimit {
		inflowSectors = inflowSectors[:inflowSectorLimit]
	}

	sampleScope := meta.SampleScope
	if sampleScope == "" {
		sampleScope = fmt.Sprintf("成交额前 %d 只股票", len(stocks))
	}
	stockCount := meta.TotalAvailable
	if stockCount <= 0 {
		stockCount = len(stocks)
	}

	var upRatio *float64
	if len(stocks) > 0 {
		upRatio = roundPtr((float64(upCount)/float64(len(stocks)))*100, 2)
	}

	return &Payload{
		Source:             "东方财富公开行情接口",
		SourceNote:         "当前按成交额排序抓取高流动性样本。主力净流入属于平台算法口径，不等同于交易所逐笔资金流。本页仅用于市场观察和产品验证，不构成投资建议。",
		UpdatedAt:          now.UTC().Format(time.RFC3339),
		RefreshHintSeconds: DefaultRefreshHintSeconds,
		SampleScope:        sampleScope,
		CacheStatus:        "fresh",
		Market: MarketSummary{
			StockCount:      stockCount,
			SampleCount:     len(stocks),
			PositivePECount: len(positivePEStocks),
			ChartStockCount: len(chartStocks),
			TotalAmountYi:   roundPtr(totalAmount/100000000, 2),
			UpCount:         upCount,
			DownCount:       downCount,
			FlatCount:       flatCount,
			UpRatio:         upRatio,
		},
		Stocks:          chartStocks,
		Sectors:         topSectors,
		InflowSectors:   inflowSectors,
		POC:             poc,
		POCDistribution: distribution,
	}
}

func CalculatePOC(stocks []Stock, binSize float64, maxPE float64) (*POCBin, []POCBin) {
	if binSize <= 0 {
		binSize = defaultPEBinSize
	}
	if maxPE <= 0 {
		maxPE = defaultMaxPE
	}
	type workingBin struct {
		key         string
		left        float64
		right       float64
		totalAmount float64
		totalPctChg float64
		stocks      []Stock
	}
	bins := map[string]*workingBin{}

	for _, stock := range stocks {
		if stock.PE == nil || *stock.PE <= 0 || *stock.PE > maxPE || stock.Amount <= 0 {
			continue
		}
		left := math.Floor(*stock.PE/binSize) * binSize
		key := fmt.Sprintf("%.0f-%.0f", left, left+binSize)
		bin := bins[key]
		if bin == nil {
			bin = &workingBin{key: key, left: left, right: left + binSize}
			bins[key] = bin
		}
		bin.totalAmount += stock.Amount
		bin.totalPctChg += deref(stock.PctChg)
		bin.stocks = append(bin.stocks, stock)
	}

	distribution := make([]POCBin, 0, len(bins))
	for _, bin := range bins {
		binStocks := append([]Stock(nil), bin.stocks...)
		sort.Slice(binStocks, func(i, j int) bool { return binStocks[i].Amount > binStocks[j].Amount })
		if len(binStocks) > 8 {
			binStocks = binStocks[:8]
		}
		topStocks := make([]TopStock, 0, len(binStocks))
		for _, stock := range binStocks {
			topStocks = append(topStocks, TopStock{
				Code:     stock.Code,
				Symbol:   stock.Symbol,
				Name:     stock.Name,
				PE:       roundPtr(deref(stock.PE), 2),
				AmountYi: stock.AmountYi,
				PctChg:   stock.PctChg,
			})
		}
		distribution = append(distribution, POCBin{
			Key:           bin.key,
			Left:          bin.left,
			Right:         bin.right,
			StockCount:    len(bin.stocks),
			TotalAmount:   bin.totalAmount,
			TotalAmountYi: roundPtr(bin.totalAmount/100000000, 2),
			AvgPctChg:     roundPtr(bin.totalPctChg/float64(len(bin.stocks)), 2),
			TopStocks:     topStocks,
		})
	}
	sort.Slice(distribution, func(i, j int) bool { return distribution[i].Left < distribution[j].Left })

	var poc *POCBin
	for idx := range distribution {
		if poc == nil || distribution[idx].TotalAmount > poc.TotalAmount {
			current := distribution[idx]
			poc = &current
		}
	}
	return poc, distribution
}

func clonePayload(payload *Payload) *Payload {
	if payload == nil {
		return nil
	}
	clone := *payload
	clone.Stocks = append([]Stock(nil), payload.Stocks...)
	clone.Sectors = append([]Sector(nil), payload.Sectors...)
	clone.InflowSectors = append([]Sector(nil), payload.InflowSectors...)
	clone.POCDistribution = append([]POCBin(nil), payload.POCDistribution...)
	if payload.POC != nil {
		poc := *payload.POC
		poc.TopStocks = append([]TopStock(nil), payload.POC.TopStocks...)
		clone.POC = &poc
	}
	return &clone
}

func deref(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

func roundPtr(value float64, digits int) *float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return nil
	}
	base := math.Pow10(digits)
	rounded := math.Round(value*base) / base
	return &rounded
}
