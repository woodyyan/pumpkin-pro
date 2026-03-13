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
	maxSampleSize      = 80
	maxEventBufferSize = 160
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

	PriceVolumeEvents []PriceVolumeAnomaly
	BlockFlowEvents   []BlockFlowAnomaly
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
	s.processRuntimeUpdate(rt, snapshot)
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

func (s *Service) processRuntimeUpdate(rt *symbolRuntime, snapshot *SymbolSnapshot) {
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
