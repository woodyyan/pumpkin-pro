package capitalmap

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"
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

type Service struct {
	fetcher  Fetcher
	cacheTTL time.Duration
	now      func() time.Time

	mu       sync.Mutex
	cached   *Payload
	cachedAt time.Time
}

func NewService(fetcher Fetcher, cacheTTL time.Duration) *Service {
	if fetcher == nil {
		fetcher = NewEastmoneyClient(nil)
	}
	if cacheTTL <= 0 {
		cacheTTL = DefaultCacheTTL
	}
	return &Service{fetcher: fetcher, cacheTTL: cacheTTL, now: time.Now}
}

func (s *Service) GetPayload(ctx context.Context) (*Payload, error) {
	if s == nil {
		return nil, fmt.Errorf("capital map service is not initialized")
	}
	now := s.now()
	s.mu.Lock()
	if s.cached != nil && now.Sub(s.cachedAt) < s.cacheTTL {
		cached := clonePayload(s.cached)
		s.mu.Unlock()
		return cached, nil
	}
	s.mu.Unlock()

	stockResult, stockErr := s.fetcher.FetchAshareSnapshot(ctx)
	if stockErr != nil {
		return s.cachedOrError(stockErr)
	}
	sectors, sectorErr := s.fetcher.FetchIndustrySectors(ctx)
	if sectorErr != nil {
		return s.cachedOrError(sectorErr)
	}

	payload := BuildMarketPayload(stockResult.Stocks, sectors, stockResult, now)
	payload.CacheStatus = "fresh"

	s.mu.Lock()
	s.cached = clonePayload(payload)
	s.cachedAt = now
	s.mu.Unlock()

	return payload, nil
}

func (s *Service) cachedOrError(err error) (*Payload, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cached == nil {
		return nil, err
	}
	cached := clonePayload(s.cached)
	cached.CacheStatus = "stale"
	cached.LastError = err.Error()
	return cached, nil
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
		SourceNote:         "公开网页接口，适合市场观察与产品验证；生产商用前需确认数据授权与稳定性。当前按成交额排序抓取高流动性样本，避免免费接口分页过多导致请求超时。",
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
