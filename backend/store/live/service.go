package live

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	maxSampleSize            = 80
	maxEventBufferSize       = 160
	maxOverlaySampleSize     = 360
	defaultOverlayBenchmark  = "HSI"
	defaultOverlayWindowMins = 60
	maxOverlayWindowMins     = 240
	betaWarmupSamples        = 30
	rsWarmupSamples          = 10
)

type Service struct {
	repo            *Repository
	marketClient    *MarketClient
	warmupMinSample int

	mu           sync.Mutex
	sessionState SessionState
	activeSymbol string
	runtimes     map[string]*symbolRuntime
}

type symbolRuntime struct {
	LastSnapshot *SymbolSnapshot
	LastVolume   float64
	LastTurnover float64
	LastPrice    float64

	PriceSamples    []float64
	VolumeDeltas    []float64
	NetInflowSeries []float64
	OverlaySamples  []overlaySample

	PriceVolumeEvents []PriceVolumeAnomaly
	BlockFlowEvents   []BlockFlowAnomaly
}

type overlaySample struct {
	TS             time.Time
	StockPrice     float64
	BenchmarkPrice float64
}

func NewService(repo *Repository) *Service {
	return &Service{
		repo:            repo,
		marketClient:    NewMarketClient(),
		warmupMinSample: 20,
		sessionState:    SessionIdle,
		runtimes:        map[string]*symbolRuntime{},
	}
}

func (s *Service) ListWatchlist(ctx context.Context) (*WatchlistState, error) {
	items, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	active := ""
	for _, item := range items {
		if item.IsActive {
			active = item.Symbol
			break
		}
	}
	if active != "" {
		s.activeSymbol = active
		if _, ok := s.runtimes[active]; !ok {
			s.runtimes[active] = &symbolRuntime{}
		}
		if s.sessionState == SessionIdle || s.sessionState == SessionStopped {
			s.sessionState = SessionWarming
		}
	} else {
		s.activeSymbol = ""
		s.sessionState = SessionIdle
	}

	return &WatchlistState{
		SessionState: s.sessionState,
		ActiveSymbol: s.activeSymbol,
		Items:        items,
	}, nil
}

func (s *Service) AddWatchlist(ctx context.Context, symbol, name string) (*WatchlistItem, error) {
	normalized, err := normalizeHKSymbol(symbol)
	if err != nil {
		return nil, err
	}
	cleanName := strings.TrimSpace(name)
	if cleanName == "" {
		cleanName = normalized
	}
	item, err := s.repo.Create(ctx, normalized, cleanName)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	if _, ok := s.runtimes[normalized]; !ok {
		s.runtimes[normalized] = &symbolRuntime{}
	}
	if s.activeSymbol == "" {
		s.activeSymbol = normalized
		s.sessionState = SessionWarming
		item.IsActive = true
		_, _ = s.repo.SetActiveSymbol(ctx, normalized)
	}
	s.mu.Unlock()

	return item, nil
}

func (s *Service) DeleteWatchlist(ctx context.Context, symbol string) error {
	normalized, err := normalizeHKSymbol(symbol)
	if err != nil {
		return err
	}

	item, err := s.repo.GetBySymbol(ctx, normalized)
	if err != nil {
		return err
	}

	if err := s.repo.Delete(ctx, normalized); err != nil {
		return err
	}

	s.mu.Lock()
	delete(s.runtimes, normalized)
	if item.IsActive {
		items, listErr := s.repo.List(ctx)
		if listErr == nil && len(items) > 0 {
			nextSymbol := items[0].Symbol
			if _, setErr := s.repo.SetActiveSymbol(ctx, nextSymbol); setErr == nil {
				s.activeSymbol = nextSymbol
				s.sessionState = SessionWarming
			}
		} else {
			s.activeSymbol = ""
			s.sessionState = SessionIdle
		}
	}
	s.mu.Unlock()
	return nil
}

func (s *Service) ActivateSymbol(ctx context.Context, symbol string, resetWindow bool) (*ActivateResult, error) {
	normalized, err := normalizeHKSymbol(symbol)
	if err != nil {
		return nil, err
	}
	if _, err := s.repo.SetActiveSymbol(ctx, normalized); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	previous := s.activeSymbol
	s.activeSymbol = normalized
	s.sessionState = SessionWarming
	if resetWindow {
		s.runtimes[normalized] = &symbolRuntime{}
	} else if _, ok := s.runtimes[normalized]; !ok {
		s.runtimes[normalized] = &symbolRuntime{}
	}

	return &ActivateResult{
		PreviousSymbol:  previous,
		ActiveSymbol:    normalized,
		SessionState:    s.sessionState,
		WarmupMinSample: s.warmupMinSample,
	}, nil
}

func (s *Service) GetMarketOverview(ctx context.Context) (*MarketOverview, error) {
	overview, err := s.marketClient.FetchMarketOverview(ctx)
	if err != nil {
		s.setDegradedIfRunning()
		return nil, err
	}
	return overview, nil
}

func (s *Service) GetSymbolSnapshot(ctx context.Context, symbol string) (*SymbolSnapshot, bool, SessionState, error) {
	normalized, err := normalizeHKSymbol(symbol)
	if err != nil {
		return nil, false, SessionIdle, err
	}

	snapshot, err := s.marketClient.FetchSymbolSnapshot(ctx, normalized)
	if err != nil {
		s.setDegradedIfRunning()
		return nil, false, SessionDegraded, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	rt := s.ensureRuntime(normalized)
	s.processRuntimeUpdate(rt, snapshot, nil)
	isActive := normalized == s.activeSymbol
	if isActive {
		if len(rt.PriceSamples) >= s.warmupMinSample {
			s.sessionState = SessionRunning
		} else {
			s.sessionState = SessionWarming
		}
	}
	return snapshot, isActive, s.sessionState, nil
}

func (s *Service) GetOverlay(ctx context.Context, symbol string, windowMinutes int, benchmark string) (*OverlayPayload, error) {
	normalizedSymbol, err := normalizeHKSymbol(symbol)
	if err != nil {
		return nil, err
	}
	if windowMinutes <= 0 {
		windowMinutes = defaultOverlayWindowMins
	}
	if windowMinutes > maxOverlayWindowMins {
		windowMinutes = maxOverlayWindowMins
	}
	normalizedBenchmark := normalizeBenchmark(benchmark)

	symbolSnapshot, benchmarkSnapshot, err := s.marketClient.FetchOverlaySnapshot(ctx, normalizedSymbol, normalizedBenchmark)
	if err != nil {
		s.setDegradedIfRunning()
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	rt := s.ensureRuntime(normalizedSymbol)
	s.processRuntimeUpdate(rt, symbolSnapshot, benchmarkSnapshot)
	if normalizedSymbol == s.activeSymbol {
		if len(rt.PriceSamples) >= s.warmupMinSample {
			s.sessionState = SessionRunning
		} else {
			s.sessionState = SessionWarming
		}
	}

	windowSamples := windowOverlaySamples(rt.OverlaySamples, windowMinutes)
	series := buildOverlaySeries(windowSamples)
	metrics := buildOverlayMetrics(windowSamples)
	updatedAt := time.Now().UTC().Format(time.RFC3339)
	if len(windowSamples) > 0 {
		updatedAt = windowSamples[len(windowSamples)-1].TS.UTC().Format(time.RFC3339)
	}

	return &OverlayPayload{
		Symbol:        normalizedSymbol,
		Benchmark:     normalizedBenchmark,
		WindowMinutes: windowMinutes,
		SessionState:  s.sessionState,
		Series:        series,
		Metrics:       metrics,
		UpdatedAt:     updatedAt,
	}, nil
}

func (s *Service) ListPriceVolumeAnomalies(ctx context.Context, symbol string, since time.Time, limit int, types []string) ([]PriceVolumeAnomaly, SessionState, error) {
	normalized, err := normalizeHKSymbol(symbol)
	if err != nil {
		return nil, SessionIdle, err
	}
	if _, _, _, err := s.GetSymbolSnapshot(ctx, normalized); err != nil {
		return nil, SessionDegraded, err
	}

	typeSet := map[string]struct{}{}
	for _, item := range types {
		text := strings.TrimSpace(item)
		if text != "" {
			typeSet[text] = struct{}{}
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	rt := s.ensureRuntime(normalized)
	items := make([]PriceVolumeAnomaly, 0, len(rt.PriceVolumeEvents))
	for _, event := range rt.PriceVolumeEvents {
		t, err := time.Parse(time.RFC3339, event.DetectedAt)
		if err != nil {
			continue
		}
		if !since.IsZero() && !t.After(since) {
			continue
		}
		if len(typeSet) > 0 {
			if _, ok := typeSet[event.AnomalyType]; !ok {
				continue
			}
		}
		items = append(items, event)
	}
	if limit <= 0 {
		limit = 50
	}
	if len(items) > limit {
		items = items[:limit]
	}
	return items, s.sessionState, nil
}

func (s *Service) ListBlockFlowAnomalies(ctx context.Context, symbol string, since time.Time, limit int) ([]BlockFlowAnomaly, SessionState, error) {
	normalized, err := normalizeHKSymbol(symbol)
	if err != nil {
		return nil, SessionIdle, err
	}
	if _, _, _, err := s.GetSymbolSnapshot(ctx, normalized); err != nil {
		return nil, SessionDegraded, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	rt := s.ensureRuntime(normalized)
	items := make([]BlockFlowAnomaly, 0, len(rt.BlockFlowEvents))
	for _, event := range rt.BlockFlowEvents {
		t, err := time.Parse(time.RFC3339, event.DetectedAt)
		if err != nil {
			continue
		}
		if !since.IsZero() && !t.After(since) {
			continue
		}
		items = append(items, event)
	}
	if limit <= 0 {
		limit = 50
	}
	if len(items) > limit {
		items = items[:limit]
	}
	return items, s.sessionState, nil
}

func (s *Service) ensureRuntime(symbol string) *symbolRuntime {
	rt, ok := s.runtimes[symbol]
	if !ok {
		rt = &symbolRuntime{}
		s.runtimes[symbol] = rt
	}
	return rt
}

func (s *Service) processRuntimeUpdate(rt *symbolRuntime, snapshot *SymbolSnapshot, benchmark *BenchmarkSnapshot) {
	price := snapshot.LastPrice
	volume := snapshot.Volume
	turnover := snapshot.Turnover

	priceDelta := 0.0
	if rt.LastPrice > 0 {
		priceDelta = price - rt.LastPrice
	}
	volumeDelta := 0.0
	if rt.LastVolume > 0 {
		volumeDelta = math.Max(0, volume-rt.LastVolume)
	}
	turnoverDelta := 0.0
	if rt.LastTurnover > 0 {
		turnoverDelta = math.Max(0, turnover-rt.LastTurnover)
	}
	if rt.LastTurnover == 0 {
		turnoverDelta = turnover
	}

	rt.PriceSamples = appendWithCap(rt.PriceSamples, price, maxSampleSize)
	if volumeDelta > 0 {
		rt.VolumeDeltas = appendWithCap(rt.VolumeDeltas, volumeDelta, maxSampleSize)
	}

	netInflow := turnoverDelta
	if priceDelta < 0 {
		netInflow = -turnoverDelta
	}
	rt.NetInflowSeries = appendWithCap(rt.NetInflowSeries, netInflow, maxSampleSize)

	now, _ := time.Parse(time.RFC3339, snapshot.TS)
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if benchmark != nil && price > 0 && benchmark.Last > 0 {
		benchmarkTS, _ := time.Parse(time.RFC3339, benchmark.TS)
		if benchmarkTS.IsZero() {
			benchmarkTS = now
		}
		sampleTS := now
		if benchmarkTS.After(sampleTS) {
			sampleTS = benchmarkTS
		}
		rt.OverlaySamples = appendOverlaySample(rt.OverlaySamples, overlaySample{
			TS:             sampleTS.UTC(),
			StockPrice:     price,
			BenchmarkPrice: benchmark.Last,
		}, maxOverlaySampleSize)
	}

	newPriceEvents := detectPriceVolumeAnomalies(snapshot.Symbol, rt.PriceSamples, rt.VolumeDeltas, priceDelta, volumeDelta, now)
	if len(newPriceEvents) > 0 {
		rt.PriceVolumeEvents = append(newPriceEvents, rt.PriceVolumeEvents...)
		if len(rt.PriceVolumeEvents) > maxEventBufferSize {
			rt.PriceVolumeEvents = rt.PriceVolumeEvents[:maxEventBufferSize]
		}
	}

	newBlockEvents := detectBlockFlowAnomalies(snapshot.Symbol, netInflow, turnoverDelta, priceDelta, rt.NetInflowSeries, now)
	if len(newBlockEvents) > 0 {
		rt.BlockFlowEvents = append(newBlockEvents, rt.BlockFlowEvents...)
		if len(rt.BlockFlowEvents) > maxEventBufferSize {
			rt.BlockFlowEvents = rt.BlockFlowEvents[:maxEventBufferSize]
		}
	}

	rt.LastSnapshot = snapshot
	rt.LastVolume = volume
	rt.LastTurnover = turnover
	rt.LastPrice = price
}

func detectPriceVolumeAnomalies(symbol string, prices []float64, volumeDeltas []float64, priceDelta float64, volumeDelta float64, now time.Time) []PriceVolumeAnomaly {
	items := make([]PriceVolumeAnomaly, 0, 2)
	if len(volumeDeltas) >= 12 {
		window := volumeDeltas
		if len(window) > 20 {
			window = window[len(window)-20:]
		}
		base := median(window[:len(window)-1])
		latest := window[len(window)-1]
		if base > 0 && latest >= 2.5*base {
			score := math.Min(100, latest/base*25)
			items = append(items, PriceVolumeAnomaly{
				EventID:     fmt.Sprintf("pv-volume-%s-%d", symbol, now.UnixNano()),
				Symbol:      symbol,
				AnomalyType: "volume_spike",
				Score:       score,
				ThresholdSnapshot: map[string]any{
					"multiplier": 2.5,
					"baseline":   base,
					"current":    latest,
				},
				MetricsSnapshot: map[string]any{
					"price_delta":  priceDelta,
					"volume_delta": volumeDelta,
				},
				DetectedAt: now.UTC().Format(time.RFC3339),
			})
		}
	}

	if len(prices) >= 16 {
		recent := prices
		if len(recent) > 15 {
			recent = recent[len(recent)-15:]
		}
		current := recent[len(recent)-1]
		prev := recent[:len(recent)-1]
		maxPrev := maxFloat(prev)
		minPrev := minFloat(prev)
		if maxPrev > 0 && current >= maxPrev*1.008 {
			items = append(items, PriceVolumeAnomaly{
				EventID:     fmt.Sprintf("pv-breakout-up-%s-%d", symbol, now.UnixNano()),
				Symbol:      symbol,
				AnomalyType: "price_breakout_up",
				Score:       math.Min(100, (current/maxPrev-1)*10000),
				ThresholdSnapshot: map[string]any{
					"threshold_ratio": 0.008,
					"previous_high":   maxPrev,
					"current_price":   current,
				},
				MetricsSnapshot: map[string]any{"volume_delta": volumeDelta},
				DetectedAt:      now.UTC().Format(time.RFC3339),
			})
		}
		if minPrev > 0 && current <= minPrev*0.992 {
			items = append(items, PriceVolumeAnomaly{
				EventID:     fmt.Sprintf("pv-breakout-down-%s-%d", symbol, now.UnixNano()),
				Symbol:      symbol,
				AnomalyType: "price_breakout_down",
				Score:       math.Min(100, (1-current/minPrev)*10000),
				ThresholdSnapshot: map[string]any{
					"threshold_ratio": -0.008,
					"previous_low":    minPrev,
					"current_price":   current,
				},
				MetricsSnapshot: map[string]any{"volume_delta": volumeDelta},
				DetectedAt:      now.UTC().Format(time.RFC3339),
			})
		}
	}
	return items
}

func detectBlockFlowAnomalies(symbol string, netInflow float64, turnoverDelta float64, priceDelta float64, series []float64, now time.Time) []BlockFlowAnomaly {
	if turnoverDelta <= 0 {
		return nil
	}
	directionStrength := math.Min(1, math.Abs(netInflow)/math.Max(turnoverDelta, 1))
	buyAmount := turnoverDelta * (0.5 + directionStrength/2)
	sellAmount := turnoverDelta - buyAmount
	if netInflow < 0 {
		sellAmount = turnoverDelta * (0.5 + directionStrength/2)
		buyAmount = turnoverDelta - sellAmount
	}

	continuity := 0.0
	if len(series) >= 5 {
		recent := series
		if len(recent) > 5 {
			recent = recent[len(recent)-5:]
		}
		sameDirection := 0
		for _, value := range recent {
			if (netInflow >= 0 && value >= 0) || (netInflow < 0 && value < 0) {
				sameDirection++
			}
		}
		continuity = float64(sameDirection) / float64(len(recent))
	}

	if directionStrength < 0.4 && continuity < 0.7 {
		return nil
	}

	event := BlockFlowAnomaly{
		EventID:           fmt.Sprintf("bf-%s-%d", symbol, now.UnixNano()),
		Symbol:            symbol,
		NetInflow:         netInflow,
		BuyBlockAmount:    buyAmount,
		SellBlockAmount:   sellAmount,
		DirectionStrength: directionStrength,
		Continuity:        continuity,
		DetectedAt:        now.UTC().Format(time.RFC3339),
	}
	_ = priceDelta
	return []BlockFlowAnomaly{event}
}

func (s *Service) setDegradedIfRunning() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sessionState == SessionRunning || s.sessionState == SessionWarming {
		s.sessionState = SessionDegraded
	}
}

func appendWithCap(items []float64, value float64, capSize int) []float64 {
	items = append(items, value)
	if len(items) > capSize {
		items = items[len(items)-capSize:]
	}
	return items
}

func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	clone := append([]float64(nil), values...)
	sort.Float64s(clone)
	middle := len(clone) / 2
	if len(clone)%2 == 0 {
		return (clone[middle-1] + clone[middle]) / 2
	}
	return clone[middle]
}

func maxFloat(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	maxValue := values[0]
	for _, value := range values[1:] {
		if value > maxValue {
			maxValue = value
		}
	}
	return maxValue
}

func minFloat(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	minValue := values[0]
	for _, value := range values[1:] {
		if value < minValue {
			minValue = value
		}
	}
	return minValue
}

func appendOverlaySample(items []overlaySample, sample overlaySample, capSize int) []overlaySample {
	minuteTS := sample.TS.UTC().Truncate(time.Minute)
	sample.TS = minuteTS
	if len(items) > 0 {
		lastIdx := len(items) - 1
		if items[lastIdx].TS.Equal(minuteTS) {
			items[lastIdx] = sample
			return items
		}
	}

	items = append(items, sample)
	if len(items) > capSize {
		items = items[len(items)-capSize:]
	}
	return items
}

func windowOverlaySamples(items []overlaySample, windowMinutes int) []overlaySample {
	if len(items) == 0 {
		return nil
	}
	cutoff := time.Now().UTC().Add(-time.Duration(windowMinutes) * time.Minute)
	start := 0
	for idx, item := range items {
		if item.TS.After(cutoff) || item.TS.Equal(cutoff) {
			start = idx
			break
		}
		if idx == len(items)-1 {
			start = len(items)
		}
	}
	if start >= len(items) {
		return nil
	}
	window := make([]overlaySample, 0, len(items)-start)
	window = append(window, items[start:]...)
	return window
}

func buildOverlaySeries(samples []overlaySample) []OverlayPoint {
	if len(samples) == 0 {
		return []OverlayPoint{}
	}
	baseStock := samples[0].StockPrice
	baseBenchmark := samples[0].BenchmarkPrice
	if baseStock <= 0 {
		baseStock = 1
	}
	if baseBenchmark <= 0 {
		baseBenchmark = 1
	}

	points := make([]OverlayPoint, 0, len(samples))
	for _, sample := range samples {
		points = append(points, OverlayPoint{
			TS:             sample.TS.UTC().Format(time.RFC3339),
			StockPrice:     sample.StockPrice,
			BenchmarkPrice: sample.BenchmarkPrice,
			StockNorm:      sample.StockPrice / baseStock,
			BenchmarkNorm:  sample.BenchmarkPrice / baseBenchmark,
		})
	}
	return points
}

func buildOverlayMetrics(samples []overlaySample) OverlayMetrics {
	metrics := OverlayMetrics{
		Beta:             nil,
		RelativeStrength: nil,
		SampleCount:      len(samples),
		WarmupMinSamples: betaWarmupSamples,
		IsWarmup:         len(samples) < betaWarmupSamples,
	}
	if len(samples) < 2 {
		return metrics
	}

	stockReturns := make([]float64, 0, len(samples)-1)
	benchmarkReturns := make([]float64, 0, len(samples)-1)
	for i := 1; i < len(samples); i++ {
		prev := samples[i-1]
		curr := samples[i]
		if prev.StockPrice <= 0 || prev.BenchmarkPrice <= 0 {
			continue
		}
		stockReturns = append(stockReturns, curr.StockPrice/prev.StockPrice-1)
		benchmarkReturns = append(benchmarkReturns, curr.BenchmarkPrice/prev.BenchmarkPrice-1)
	}

	if len(samples) >= rsWarmupSamples {
		first := samples[0]
		last := samples[len(samples)-1]
		if first.StockPrice > 0 && first.BenchmarkPrice > 0 {
			rsValue := (last.StockPrice/first.StockPrice - 1) - (last.BenchmarkPrice/first.BenchmarkPrice - 1)
			metrics.RelativeStrength = &rsValue
		}
	}

	if len(stockReturns) < betaWarmupSamples-1 || len(benchmarkReturns) != len(stockReturns) {
		return metrics
	}

	meanStock := 0.0
	meanBenchmark := 0.0
	for i := range stockReturns {
		meanStock += stockReturns[i]
		meanBenchmark += benchmarkReturns[i]
	}
	meanStock /= float64(len(stockReturns))
	meanBenchmark /= float64(len(benchmarkReturns))

	cov := 0.0
	varBenchmark := 0.0
	for i := range stockReturns {
		ds := stockReturns[i] - meanStock
		db := benchmarkReturns[i] - meanBenchmark
		cov += ds * db
		varBenchmark += db * db
	}
	if varBenchmark > 0 {
		betaValue := cov / varBenchmark
		metrics.Beta = &betaValue
	}

	return metrics
}
