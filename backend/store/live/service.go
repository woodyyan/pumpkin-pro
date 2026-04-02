package live

import (
	"context"
	"encoding/json"
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

	mu     sync.Mutex
	states map[string]*userLiveState
}

type userLiveState struct {
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
		states:          map[string]*userLiveState{},
	}
}

func (s *Service) ListWatchlist(ctx context.Context, userID string) (*WatchlistState, error) {
	items, err := s.repo.List(ctx, userID)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.ensureUserState(userID)

	active := ""
	for _, item := range items {
		if item.IsActive {
			active = item.Symbol
			break
		}
	}
	if active != "" {
		state.activeSymbol = active
		if _, ok := state.runtimes[active]; !ok {
			state.runtimes[active] = &symbolRuntime{}
		}
		if state.sessionState == SessionIdle || state.sessionState == SessionStopped {
			state.sessionState = SessionWarming
		}
	} else {
		state.activeSymbol = ""
		state.sessionState = SessionIdle
	}

	return &WatchlistState{
		SessionState: state.sessionState,
		ActiveSymbol: state.activeSymbol,
		Items:        items,
	}, nil
}

func (s *Service) AddWatchlist(ctx context.Context, userID, symbol, name string) (*WatchlistItem, error) {
	normalized, exchange, err := NormalizeSymbol(symbol)
	if err != nil {
		return nil, err
	}

	// Verify the symbol actually exists by fetching a live snapshot.
	snapshot, fetchErr := s.marketClient.FetchSymbolSnapshot(ctx, normalized)
	if fetchErr != nil || snapshot == nil || snapshot.LastPrice <= 0 {
		return nil, ErrSymbolNotExist
	}

	cleanName := strings.TrimSpace(name)
	if cleanName == "" {
		if snapshot.Name != "" && snapshot.Name != normalized {
			cleanName = snapshot.Name
		}
	}
	if cleanName == "" {
		cleanName = normalized
	}
	item, err := s.repo.Create(ctx, userID, normalized, cleanName, exchange)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	state := s.ensureUserState(userID)
	if _, ok := state.runtimes[normalized]; !ok {
		state.runtimes[normalized] = &symbolRuntime{}
	}
	if state.activeSymbol == "" {
		state.activeSymbol = normalized
		state.sessionState = SessionWarming
		item.IsActive = true
		_, _ = s.repo.SetActiveSymbol(ctx, userID, normalized)
	}
	s.mu.Unlock()

	return item, nil
}

func (s *Service) DeleteWatchlist(ctx context.Context, userID, symbol string) error {
	normalized, _, err := NormalizeSymbol(symbol)
	if err != nil {
		return err
	}

	item, err := s.repo.GetBySymbol(ctx, userID, normalized)
	if err != nil {
		return err
	}

	if err := s.repo.Delete(ctx, userID, normalized); err != nil {
		return err
	}

	s.mu.Lock()
	state := s.ensureUserState(userID)
	delete(state.runtimes, normalized)
	if item.IsActive {
		items, listErr := s.repo.List(ctx, userID)
		if listErr == nil && len(items) > 0 {
			nextSymbol := items[0].Symbol
			if _, setErr := s.repo.SetActiveSymbol(ctx, userID, nextSymbol); setErr == nil {
				state.activeSymbol = nextSymbol
				state.sessionState = SessionWarming
			}
		} else {
			state.activeSymbol = ""
			state.sessionState = SessionIdle
		}
	}
	s.mu.Unlock()
	return nil
}

func (s *Service) ActivateSymbol(ctx context.Context, userID, symbol string, resetWindow bool) (*ActivateResult, error) {
	normalized, _, err := NormalizeSymbol(symbol)
	if err != nil {
		return nil, err
	}
	if _, err := s.repo.SetActiveSymbol(ctx, userID, normalized); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.ensureUserState(userID)
	previous := state.activeSymbol
	state.activeSymbol = normalized
	state.sessionState = SessionWarming
	if resetWindow {
		state.runtimes[normalized] = &symbolRuntime{}
	} else if _, ok := state.runtimes[normalized]; !ok {
		state.runtimes[normalized] = &symbolRuntime{}
	}

	return &ActivateResult{
		PreviousSymbol:  previous,
		ActiveSymbol:    normalized,
		SessionState:    state.sessionState,
		WarmupMinSample: s.warmupMinSample,
	}, nil
}

func (s *Service) GetMarketOverview(ctx context.Context, exchange string) (*MarketOverview, error) {
	overview, err := s.marketClient.FetchMarketOverview(ctx, exchange)
	if err != nil {
		return nil, err
	}
	return overview, nil
}

func (s *Service) GetSymbolSnapshot(ctx context.Context, userID, symbol string) (*SymbolSnapshot, bool, SessionState, error) {
	normalized, _, err := NormalizeSymbol(symbol)
	if err != nil {
		return nil, false, SessionIdle, err
	}

	isTrading := IsAShareTradingHours()
	isAShare := IsAShare(normalized)

	// ── Non-trading hours: try closing snapshot cache first ──
	if isAShare && !isTrading {
		cached, cacheErr := s.loadClosingSnapshot(ctx, normalized)
		if cacheErr == nil && cached != nil {
			s.mu.Lock()
			defer s.mu.Unlock()
			state := s.ensureUserState(userID)
			isActive := normalized == state.activeSymbol
			return cached, isActive, state.sessionState, nil
		}
		// Cache miss → fall through to live fetch (first-time or no data yet)
	}

	snapshot, err := s.marketClient.FetchSymbolSnapshot(ctx, normalized)
	if err != nil {
		s.setDegradedIfRunning(userID)
		return nil, false, SessionDegraded, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.ensureUserState(userID)

	rt := s.ensureRuntime(state, normalized)
	s.processRuntimeUpdate(rt, snapshot, nil)
	isActive := normalized == state.activeSymbol
	if isActive {
		if len(rt.PriceSamples) >= s.warmupMinSample {
			state.sessionState = SessionRunning
		} else {
			state.sessionState = SessionWarming
		}
	}

	// ── Passive write: save snapshot for non-trading-hours cache ──
	if isAShare {
		go s.saveClosingSnapshotAsync(normalized, snapshot)
	}

	return snapshot, isActive, state.sessionState, nil
}

func (s *Service) GetOverlay(ctx context.Context, userID, symbol string, windowMinutes int, benchmark string) (*OverlayPayload, error) {
	normalizedSymbol, _, err := NormalizeSymbol(symbol)
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
		s.setDegradedIfRunning(userID)
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.ensureUserState(userID)

	rt := s.ensureRuntime(state, normalizedSymbol)
	s.processRuntimeUpdate(rt, symbolSnapshot, benchmarkSnapshot)
	if normalizedSymbol == state.activeSymbol {
		if len(rt.PriceSamples) >= s.warmupMinSample {
			state.sessionState = SessionRunning
		} else {
			state.sessionState = SessionWarming
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
		SessionState:  state.sessionState,
		Series:        series,
		Metrics:       metrics,
		UpdatedAt:     updatedAt,
	}, nil
}

func (s *Service) ListPriceVolumeAnomalies(ctx context.Context, userID, symbol string, since time.Time, limit int, types []string) ([]PriceVolumeAnomaly, SessionState, error) {
	normalized, _, err := NormalizeSymbol(symbol)
	if err != nil {
		return nil, SessionIdle, err
	}
	if _, _, _, err := s.GetSymbolSnapshot(ctx, userID, normalized); err != nil {
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
	state := s.ensureUserState(userID)
	rt := s.ensureRuntime(state, normalized)
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
	return items, state.sessionState, nil
}

func (s *Service) ListBlockFlowAnomalies(ctx context.Context, userID, symbol string, since time.Time, limit int) ([]BlockFlowAnomaly, SessionState, error) {
	normalized, _, err := NormalizeSymbol(symbol)
	if err != nil {
		return nil, SessionIdle, err
	}
	if _, _, _, err := s.GetSymbolSnapshot(ctx, userID, normalized); err != nil {
		return nil, SessionDegraded, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.ensureUserState(userID)
	rt := s.ensureRuntime(state, normalized)
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
	return items, state.sessionState, nil
}

func (s *Service) ensureUserState(userID string) *userLiveState {
	state, ok := s.states[userID]
	if !ok {
		state = &userLiveState{
			sessionState: SessionIdle,
			runtimes:     map[string]*symbolRuntime{},
		}
		s.states[userID] = state
	}
	return state
}

func (s *Service) ensureRuntime(state *userLiveState, symbol string) *symbolRuntime {
	rt, ok := state.runtimes[symbol]
	if !ok {
		rt = &symbolRuntime{}
		state.runtimes[symbol] = rt
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

func (s *Service) setDegradedIfRunning(userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.ensureUserState(userID)
	if state.sessionState == SessionRunning || state.sessionState == SessionWarming {
		state.sessionState = SessionDegraded
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
	if len(items) == 0 {
		return []overlaySample{sample}
	}

	lastIdx := len(items) - 1
	lastTS := items[lastIdx].TS
	if minuteTS.After(lastTS) {
		items = append(items, sample)
	} else if minuteTS.Equal(lastTS) {
		items[lastIdx] = sample
	} else {
		insertAt := sort.Search(len(items), func(i int) bool {
			return !items[i].TS.Before(minuteTS)
		})
		if insertAt < len(items) && items[insertAt].TS.Equal(minuteTS) {
			items[insertAt] = sample
		} else {
			items = append(items, overlaySample{})
			copy(items[insertAt+1:], items[insertAt:])
			items[insertAt] = sample
		}
	}

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

// GetDailyBars returns historical daily bars for a symbol.
// lookbackDays is clamped to [1, 2600] (roughly 10 years of trading days).
func (s *Service) GetDailyBars(ctx context.Context, symbol string, lookbackDays int) ([]DailyBar, error) {
	normalized, _, err := NormalizeSymbol(symbol)
	if err != nil {
		return nil, err
	}
	if lookbackDays <= 0 {
		lookbackDays = 130 // default ~6 months
	}
	if lookbackDays > 2600 {
		lookbackDays = 2600
	}
	bars, err := s.marketClient.FetchSymbolDailyBars(ctx, normalized, lookbackDays)
	if err != nil {
		return nil, err
	}
	return bars, nil
}

// GetDailyOverlay returns normalised daily close series for a stock vs its benchmark.
func (s *Service) GetDailyOverlay(ctx context.Context, symbol string, lookbackDays int, benchmark string) (*DailyOverlayPayload, error) {
	normalized, _, err := NormalizeSymbol(symbol)
	if err != nil {
		return nil, err
	}
	if lookbackDays <= 0 {
		lookbackDays = 60
	}
	if lookbackDays > 500 {
		lookbackDays = 500
	}
	if strings.TrimSpace(benchmark) == "" {
		benchmark = defaultBenchmarkForSymbol(normalized)
	}
	normalizedBenchmark := normalizeBenchmark(benchmark)

	stockBars, err := s.marketClient.FetchSymbolDailyBars(ctx, normalized, lookbackDays)
	if err != nil {
		return nil, err
	}
	benchBars, err := s.marketClient.FetchBenchmarkDailyBars(ctx, normalizedBenchmark, lookbackDays)
	if err != nil {
		return nil, err
	}

	// Build date→close map for benchmark
	benchByDate := make(map[string]float64, len(benchBars))
	for _, b := range benchBars {
		if b.Close > 0 {
			benchByDate[b.Date] = b.Close
		}
	}

	// Build aligned series: only dates present in both
	type aligned struct {
		Date       string
		StockClose float64
		BenchClose float64
	}
	pairs := make([]aligned, 0, len(stockBars))
	for _, sb := range stockBars {
		bc, ok := benchByDate[sb.Date]
		if !ok || sb.Close <= 0 || bc <= 0 {
			continue
		}
		pairs = append(pairs, aligned{Date: sb.Date, StockClose: sb.Close, BenchClose: bc})
	}

	if len(pairs) == 0 {
		return &DailyOverlayPayload{
			Symbol:       normalized,
			Benchmark:    normalizedBenchmark,
			LookbackDays: lookbackDays,
			Series:       []DailyOverlayPoint{},
			Metrics:      DailyOverlayMetrics{SampleDays: 0},
			UpdatedAt:    time.Now().UTC().Format(time.RFC3339),
		}, nil
	}

	baseStock := pairs[0].StockClose
	baseBench := pairs[0].BenchClose
	series := make([]DailyOverlayPoint, 0, len(pairs))
	stockReturns := make([]float64, 0, len(pairs)-1)
	benchReturns := make([]float64, 0, len(pairs)-1)
	for i, p := range pairs {
		series = append(series, DailyOverlayPoint{
			Date:      p.Date,
			StockClose: roundTo(p.StockClose, 4),
			BenchClose: roundTo(p.BenchClose, 4),
			StockNorm: roundTo(p.StockClose/baseStock, 6),
			BenchNorm: roundTo(p.BenchClose/baseBench, 6),
		})
		if i > 0 {
			stockReturns = append(stockReturns, p.StockClose/pairs[i-1].StockClose-1)
			benchReturns = append(benchReturns, p.BenchClose/pairs[i-1].BenchClose-1)
		}
	}

	metrics := DailyOverlayMetrics{SampleDays: len(pairs)}

	// Relative Strength
	if len(pairs) >= 2 {
		last := pairs[len(pairs)-1]
		rs := (last.StockClose/baseStock - 1) - (last.BenchClose/baseBench - 1)
		metrics.RelativeStrength = &rs
	}

	// Beta & Correlation
	if len(stockReturns) >= 10 && len(stockReturns) == len(benchReturns) {
		meanS, meanB := 0.0, 0.0
		for i := range stockReturns {
			meanS += stockReturns[i]
			meanB += benchReturns[i]
		}
		n := float64(len(stockReturns))
		meanS /= n
		meanB /= n

		cov, varS, varB := 0.0, 0.0, 0.0
		for i := range stockReturns {
			ds := stockReturns[i] - meanS
			db := benchReturns[i] - meanB
			cov += ds * db
			varS += ds * ds
			varB += db * db
		}
		if varB > 0 {
			beta := cov / varB
			metrics.Beta = &beta
		}
		if varS > 0 && varB > 0 {
			corr := cov / (math.Sqrt(varS) * math.Sqrt(varB))
			metrics.Correlation = &corr
		}
	}

	return &DailyOverlayPayload{
		Symbol:       normalized,
		Benchmark:    normalizedBenchmark,
		LookbackDays: lookbackDays,
		Series:       series,
		Metrics:      metrics,
		UpdatedAt:    time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// GetWatchlistSnapshots returns a snapshot for every item in the user's
// watchlist in a single batch call. The underlying MarketClient.fetchFields
// already supports multiple quote codes in one HTTP round-trip, so this avoids
// the N+1 request problem on the overview page.
func (s *Service) GetWatchlistSnapshots(ctx context.Context, userID string) ([]SymbolSnapshot, error) {
	items, err := s.repo.List(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return []SymbolSnapshot{}, nil
	}

	// Build quote-code → normalized-symbol mapping.
	type codeMapping struct {
		quoteCode  string
		normalized string
	}
	mappings := make([]codeMapping, 0, len(items))
	codeList := make([]string, 0, len(items))
	for _, item := range items {
		normalized, _, normErr := NormalizeSymbol(item.Symbol)
		if normErr != nil {
			continue
		}
		qc := quoteCodeFromSymbol(normalized)
		mappings = append(mappings, codeMapping{quoteCode: qc, normalized: normalized})
		codeList = append(codeList, qc)
	}

	if len(codeList) == 0 {
		return []SymbolSnapshot{}, nil
	}

	fields, err := s.marketClient.fetchFields(ctx, codeList)
	if err != nil {
		return nil, err
	}

	snapshots := make([]SymbolSnapshot, 0, len(mappings))
	for _, m := range mappings {
		raw, ok := fields[m.quoteCode]
		if !ok {
			continue
		}
		quote, parseErr := parseQuote(m.quoteCode, raw)
		if parseErr != nil {
			continue
		}
		snap := buildSymbolSnapshot(m.normalized, quote)
		snapshots = append(snapshots, *snap)
	}
	return snapshots, nil
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

// ── Closing Snapshot Cache helpers ──

func (s *Service) loadClosingSnapshot(ctx context.Context, symbol string) (*SymbolSnapshot, error) {
	tradeDate := TodayTradeDate()
	record, err := s.repo.GetClosingSnapshot(ctx, symbol, tradeDate)
	if err != nil || record == nil {
		return nil, err
	}
	var snapshot SymbolSnapshot
	if err := json.Unmarshal([]byte(record.SnapshotJSON), &snapshot); err != nil {
		return nil, err
	}
	return &snapshot, nil
}

func (s *Service) saveClosingSnapshotAsync(symbol string, snapshot *SymbolSnapshot) {
	if snapshot == nil || snapshot.LastPrice <= 0 {
		return
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		return
	}
	tradeDate := TodayTradeDate()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = s.repo.UpsertClosingSnapshot(ctx, ClosingSnapshotRecord{
		Symbol:       symbol,
		TradeDate:    tradeDate,
		SnapshotJSON: string(data),
		UpdatedAt:    time.Now().UTC(),
	})
}
