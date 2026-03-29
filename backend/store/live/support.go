package live

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

const (
	supportPeriodDaily         = "daily"
	defaultSupportLookbackDays = 120
	defaultMALookbackDays      = 240
	minSupportSampleCount      = 60
	minMASampleCount           = 200
	maxSupportLevels           = 3
	swingLookaround            = 3
)

type supportCandidate struct {
	Price           float64
	Source          string
	Weight          float64
	SourceScore     float64
	TouchCount      int
	LastValidatedAt time.Time
	Bounce          float64
}

type supportBand struct {
	Price           float64
	BandLow         float64
	BandHigh        float64
	WeightSum       float64
	SourceScore     float64
	TouchCount      int
	LastValidatedAt time.Time
	Bounce          float64
	Sources         map[string]struct{}
}

func (s *Service) GetSupportLevels(ctx context.Context, userID, symbol, period string, lookbackDays int) (*SupportLevelsPayload, error) {
	normalizedSymbol, _, err := NormalizeSymbol(symbol)
	if err != nil {
		return nil, err
	}

	normalizedPeriod := strings.ToLower(strings.TrimSpace(period))
	if normalizedPeriod == "" {
		normalizedPeriod = supportPeriodDaily
	}
	if normalizedPeriod != supportPeriodDaily {
		return nil, fmt.Errorf("%w: period only supports daily", ErrInvalidArgument)
	}

	if lookbackDays == 0 {
		lookbackDays = defaultSupportLookbackDays
	}
	if lookbackDays < 90 || lookbackDays > 240 {
		return nil, fmt.Errorf("%w: lookback_days must be between 90 and 240", ErrInvalidArgument)
	}

	bars, err := s.marketClient.FetchSymbolDailyBars(ctx, normalizedSymbol, lookbackDays)
	if err != nil {
		return nil, err
	}
	if len(bars) < minSupportSampleCount {
		return nil, ErrWarmupNotReady
	}

	lastBar := bars[len(bars)-1]
	if lastBar.Close <= 0 {
		return nil, ErrDataSourceDown
	}

	candidates := buildSupportCandidates(bars, lastBar.Close)
	if len(candidates) == 0 {
		return nil, ErrWarmupNotReady
	}

	bands := clusterSupportBands(candidates, lastBar.Close)
	if len(bands) == 0 {
		return nil, ErrWarmupNotReady
	}

	levels, summary := buildSupportLevelsAndSummary(bands, lastBar.Close, lastBar.Date)
	if len(levels) == 0 {
		return nil, ErrWarmupNotReady
	}

	sessionState := s.resolveSessionState(userID)
	now := time.Now().UTC().Format(time.RFC3339)

	return &SupportLevelsPayload{
		Symbol:       normalizedSymbol,
		Period:       supportPeriodDaily,
		LookbackDays: lookbackDays,
		AsOf:         lastBar.Date,
		PriceRef:     roundTo(lastBar.Close, 4),
		SessionState: sessionState,
		Summary:      summary,
		Levels:       levels,
		Meta: SupportMeta{
			Algorithm:          "support-v1-fusion-daily",
			SampleCount:        len(bars),
			MinRequiredSamples: minSupportSampleCount,
			IsWarmup:           false,
			UpdatedAt:          now,
		},
	}, nil
}

func (s *Service) GetMovingAverages(ctx context.Context, userID, symbol, period string, lookbackDays int) (*MovingAveragesPayload, error) {
	normalizedSymbol, _, err := NormalizeSymbol(symbol)
	if err != nil {
		return nil, err
	}

	normalizedPeriod := strings.ToLower(strings.TrimSpace(period))
	if normalizedPeriod == "" {
		normalizedPeriod = supportPeriodDaily
	}
	if normalizedPeriod != supportPeriodDaily {
		return nil, fmt.Errorf("%w: period only supports daily", ErrInvalidArgument)
	}

	if lookbackDays == 0 {
		lookbackDays = defaultMALookbackDays
	}
	if lookbackDays < minMASampleCount || lookbackDays > 480 {
		return nil, fmt.Errorf("%w: lookback_days must be between 200 and 480", ErrInvalidArgument)
	}

	bars, err := s.marketClient.FetchSymbolDailyBars(ctx, normalizedSymbol, lookbackDays)
	if err != nil {
		return nil, err
	}
	if len(bars) < minMASampleCount {
		return nil, ErrWarmupNotReady
	}

	lastBar := bars[len(bars)-1]
	if lastBar.Close <= 0 {
		return nil, ErrDataSourceDown
	}

	ma5 := movingAverageClose(bars, 5)
	ma20 := movingAverageClose(bars, 20)
	ma60 := movingAverageClose(bars, 60)
	ma200 := movingAverageClose(bars, 200)
	if ma20 <= 0 || ma200 <= 0 {
		return nil, ErrWarmupNotReady
	}

	distancePct := func(price, ma float64) float64 {
		if ma <= 0 {
			return 0
		}
		return (price - ma) / ma * 100
	}

	rsi14 := calculateRSI(bars, 14)

	return &MovingAveragesPayload{
		Symbol:             normalizedSymbol,
		Period:             supportPeriodDaily,
		LookbackDays:       lookbackDays,
		AsOf:               lastBar.Date,
		PriceRef:           roundTo(lastBar.Close, 4),
		MA5:                roundTo(ma5, 4),
		MA20:               roundTo(ma20, 4),
		MA60:               roundTo(ma60, 4),
		MA200:              roundTo(ma200, 4),
		DistanceToMA5Pct:   roundTo(distancePct(lastBar.Close, ma5), 2),
		DistanceToMA20Pct:  roundTo(distancePct(lastBar.Close, ma20), 2),
		DistanceToMA60Pct:  roundTo(distancePct(lastBar.Close, ma60), 2),
		DistanceToMA200Pct: roundTo(distancePct(lastBar.Close, ma200), 2),
		RSI14:              roundTo(math.Max(rsi14, 0), 2),
		RSI14Status:        classifyRSIStatus(rsi14),
		Status:             classifyMAStatus(lastBar.Close, ma20, ma200),
		SessionState:       s.resolveSessionState(userID),
		UpdatedAt:          time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func classifyMAStatus(priceRef, ma20, ma200 float64) string {
	if priceRef <= 0 || ma20 <= 0 || ma200 <= 0 {
		return "数据不足"
	}
	switch {
	case priceRef >= ma20 && priceRef >= ma200:
		return "双双站上"
	case priceRef < ma20 && priceRef < ma200:
		return "双双跌破"
	case priceRef >= ma20 && priceRef < ma200:
		return "站上MA20但低于MA200"
	default:
		return "跌破MA20但高于MA200"
	}
}

func (s *Service) resolveSessionState(userID string) SessionState {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.ensureUserState(userID)
	if state.sessionState == "" {
		return SessionIdle
	}
	return state.sessionState
}

func buildSupportCandidates(bars []DailyBar, lastClose float64) []supportCandidate {
	candidates := make([]supportCandidate, 0, 16)
	candidates = append(candidates, buildSwingCandidates(bars)...)
	candidates = append(candidates, buildPivotCandidates(bars)...)
	candidates = append(candidates, buildMACandidates(bars)...)

	filtered := make([]supportCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Price <= 0 || math.IsNaN(candidate.Price) || math.IsInf(candidate.Price, 0) {
			continue
		}
		if candidate.Price > lastClose*1.01 {
			continue
		}
		if candidate.TouchCount <= 0 {
			candidate.TouchCount = 1
		}
		if candidate.Weight <= 0 {
			candidate.Weight = 1
		}
		if candidate.SourceScore <= 0 {
			candidate.SourceScore = 50
		}
		filtered = append(filtered, candidate)
	}
	return filtered
}

func buildSwingCandidates(bars []DailyBar) []supportCandidate {
	if len(bars) < swingLookaround*2+1 {
		return nil
	}
	candidates := make([]supportCandidate, 0, 10)
	for idx := swingLookaround; idx < len(bars)-swingLookaround; idx++ {
		currentLow := bars[idx].Low
		if currentLow <= 0 {
			continue
		}
		isSwing := true
		for i := idx - swingLookaround; i <= idx+swingLookaround; i++ {
			if i == idx {
				continue
			}
			if bars[i].Low <= currentLow {
				isSwing = false
				break
			}
		}
		if !isSwing {
			continue
		}

		touchCount, lastValidatedAt := measureTouchesAndLastValidatedAt(bars, currentLow, 0.005)
		bounce := calcBounceFromIndex(bars, idx, 10)
		if lastValidatedAt.IsZero() {
			lastValidatedAt = parseBarDate(bars[idx].Date)
		}
		candidates = append(candidates, supportCandidate{
			Price:           currentLow,
			Source:          "swing",
			Weight:          1.2 + math.Min(0.3, bounce),
			SourceScore:     100,
			TouchCount:      maxInt(touchCount, 1),
			LastValidatedAt: lastValidatedAt,
			Bounce:          bounce,
		})
	}
	return candidates
}

func buildPivotCandidates(bars []DailyBar) []supportCandidate {
	if len(bars) == 0 {
		return nil
	}
	pivotBar := bars[len(bars)-1]
	if len(bars) >= 2 {
		pivotBar = bars[len(bars)-2]
	}
	if pivotBar.High <= 0 || pivotBar.Low <= 0 || pivotBar.Close <= 0 {
		return nil
	}

	pp := (pivotBar.High + pivotBar.Low + pivotBar.Close) / 3
	s1 := 2*pp - pivotBar.High
	s2 := pp - (pivotBar.High - pivotBar.Low)

	pivotDate := parseBarDate(pivotBar.Date)
	cand := make([]supportCandidate, 0, 2)
	for _, price := range []float64{s1, s2} {
		if price <= 0 {
			continue
		}
		touchCount, lastValidatedAt := measureTouchesAndLastValidatedAt(bars, price, 0.005)
		bounce := calcBestBounceAroundLevel(bars, price, 10)
		if lastValidatedAt.IsZero() {
			lastValidatedAt = pivotDate
		}
		cand = append(cand, supportCandidate{
			Price:           price,
			Source:          "pivot",
			Weight:          1,
			SourceScore:     70,
			TouchCount:      maxInt(touchCount, 1),
			LastValidatedAt: lastValidatedAt,
			Bounce:          bounce,
		})
	}
	return cand
}

func buildMACandidates(bars []DailyBar) []supportCandidate {
	candidates := make([]supportCandidate, 0, 2)

	for _, period := range []int{60, 120} {
		ma := movingAverageClose(bars, period)
		if ma <= 0 {
			continue
		}
		source := fmt.Sprintf("ma%d", period)
		sourceScore := 60.0
		if period >= 120 {
			sourceScore = 55
		}
		touchCount, lastValidatedAt := measureTouchesAndLastValidatedAt(bars, ma, 0.005)
		bounce := calcBestBounceAroundLevel(bars, ma, 10)
		candidates = append(candidates, supportCandidate{
			Price:           ma,
			Source:          source,
			Weight:          0.9,
			SourceScore:     sourceScore,
			TouchCount:      maxInt(touchCount, 1),
			LastValidatedAt: lastValidatedAt,
			Bounce:          bounce,
		})
	}
	return candidates
}

func clusterSupportBands(candidates []supportCandidate, lastClose float64) []supportBand {
	if len(candidates) == 0 {
		return nil
	}
	sortedCandidates := append([]supportCandidate(nil), candidates...)
	sort.Slice(sortedCandidates, func(i, j int) bool {
		return sortedCandidates[i].Price < sortedCandidates[j].Price
	})

	eps := math.Max(lastClose*0.006, 0.02)
	bands := make([]supportBand, 0, len(sortedCandidates))
	for _, candidate := range sortedCandidates {
		if len(bands) == 0 {
			bands = append(bands, supportBand{
				Price:           candidate.Price,
				BandLow:         candidate.Price,
				BandHigh:        candidate.Price,
				WeightSum:       candidate.Weight,
				SourceScore:     candidate.SourceScore,
				TouchCount:      candidate.TouchCount,
				LastValidatedAt: candidate.LastValidatedAt,
				Bounce:          candidate.Bounce,
				Sources:         map[string]struct{}{candidate.Source: {}},
			})
			continue
		}

		lastIdx := len(bands) - 1
		lastBand := &bands[lastIdx]
		if math.Abs(candidate.Price-lastBand.Price) > eps {
			bands = append(bands, supportBand{
				Price:           candidate.Price,
				BandLow:         candidate.Price,
				BandHigh:        candidate.Price,
				WeightSum:       candidate.Weight,
				SourceScore:     candidate.SourceScore,
				TouchCount:      candidate.TouchCount,
				LastValidatedAt: candidate.LastValidatedAt,
				Bounce:          candidate.Bounce,
				Sources:         map[string]struct{}{candidate.Source: {}},
			})
			continue
		}

		totalWeight := lastBand.WeightSum + candidate.Weight
		lastBand.Price = (lastBand.Price*lastBand.WeightSum + candidate.Price*candidate.Weight) / math.Max(totalWeight, 1)
		lastBand.WeightSum = totalWeight
		lastBand.BandLow = math.Min(lastBand.BandLow, candidate.Price)
		lastBand.BandHigh = math.Max(lastBand.BandHigh, candidate.Price)
		lastBand.TouchCount = maxInt(lastBand.TouchCount, candidate.TouchCount)
		if candidate.LastValidatedAt.After(lastBand.LastValidatedAt) {
			lastBand.LastValidatedAt = candidate.LastValidatedAt
		}
		lastBand.Bounce = math.Max(lastBand.Bounce, candidate.Bounce)
		lastBand.SourceScore = math.Max(lastBand.SourceScore, candidate.SourceScore)
		if lastBand.Sources == nil {
			lastBand.Sources = map[string]struct{}{}
		}
		lastBand.Sources[candidate.Source] = struct{}{}
	}
	return bands
}

func buildSupportLevelsAndSummary(bands []supportBand, priceRef float64, asOfDate string) ([]SupportLevel, SupportSummary) {
	sortedBands := append([]supportBand(nil), bands...)
	sort.Slice(sortedBands, func(i, j int) bool {
		di := math.Abs(priceRef - sortedBands[i].Price)
		dj := math.Abs(priceRef - sortedBands[j].Price)
		if di == dj {
			return sortedBands[i].Price > sortedBands[j].Price
		}
		return di < dj
	})
	if len(sortedBands) > maxSupportLevels {
		sortedBands = sortedBands[:maxSupportLevels]
	}

	levels := make([]SupportLevel, 0, len(sortedBands))
	for idx, band := range sortedBands {
		distancePct := 0.0
		if priceRef > 0 {
			distancePct = (priceRef - band.Price) / priceRef * 100
		}
		score := scoreSupportBand(band)
		status := classifySupportStatus(priceRef, band.BandLow, band.BandHigh, distancePct)
		lastValidatedAt := asOfDate
		if !band.LastValidatedAt.IsZero() {
			lastValidatedAt = band.LastValidatedAt.UTC().Format("2006-01-02")
		}

		levels = append(levels, SupportLevel{
			Level:           fmt.Sprintf("S%d", idx+1),
			Price:           roundTo(band.Price, 4),
			BandLow:         roundTo(band.BandLow, 4),
			BandHigh:        roundTo(band.BandHigh, 4),
			DistancePct:     roundTo(distancePct, 2),
			Strength:        labelSupportStrength(score),
			Score:           roundTo(score, 1),
			Status:          status,
			Sources:         sortSupportSources(band.Sources),
			TouchCount:      maxInt(band.TouchCount, 1),
			LastValidatedAt: lastValidatedAt,
		})
	}

	summary := SupportSummary{}
	if len(levels) > 0 {
		nearest := levels[0]
		summary = SupportSummary{
			NearestLevel: nearest.Level,
			NearestPrice: nearest.Price,
			DistancePct:  nearest.DistancePct,
			Strength:     nearest.Strength,
			Status:       nearest.Status,
		}
	}

	return levels, summary
}

func scoreSupportBand(band supportBand) float64 {
	touchScore := math.Min(100, float64(maxInt(band.TouchCount, 1))*20)

	recencyScore := 20.0
	if !band.LastValidatedAt.IsZero() {
		days := time.Since(band.LastValidatedAt).Hours() / 24
		recencyScore = math.Max(0, 100-days*2)
	}

	bounceScore := math.Min(100, math.Max(0, band.Bounce)*200)
	sourceScore := math.Max(40, band.SourceScore)

	score := touchScore*0.35 + recencyScore*0.25 + bounceScore*0.25 + sourceScore*0.15
	if score < 0 {
		return 0
	}
	if score > 100 {
		return 100
	}
	return score
}

func labelSupportStrength(score float64) string {
	switch {
	case score >= 75:
		return "强"
	case score >= 45:
		return "中"
	default:
		return "弱"
	}
}

func classifySupportStatus(priceRef, bandLow, bandHigh, distancePct float64) string {
	if priceRef >= bandLow && priceRef <= bandHigh {
		return "回踩支撑"
	}
	if priceRef < bandLow {
		return "跌破支撑"
	}
	if distancePct <= 2 {
		return "临近支撑"
	}
	if priceRef > bandHigh {
		return "位于支撑上方"
	}
	return "临近支撑"
}

func measureTouchesAndLastValidatedAt(bars []DailyBar, level float64, toleranceRatio float64) (int, time.Time) {
	if level <= 0 || len(bars) == 0 {
		return 0, time.Time{}
	}
	tolerance := math.Max(level*toleranceRatio, 0.02)
	count := 0
	lastAt := time.Time{}
	for _, bar := range bars {
		if bar.Low <= level+tolerance && bar.High >= level-tolerance {
			count++
			if t := parseBarDate(bar.Date); t.After(lastAt) {
				lastAt = t
			}
		}
	}
	return count, lastAt
}

func calcBounceFromIndex(bars []DailyBar, idx int, forward int) float64 {
	if idx < 0 || idx >= len(bars) {
		return 0
	}
	low := bars[idx].Low
	if low <= 0 {
		return 0
	}
	end := idx + forward
	if end >= len(bars) {
		end = len(bars) - 1
	}
	maxClose := bars[idx].Close
	for i := idx + 1; i <= end; i++ {
		maxClose = math.Max(maxClose, bars[i].Close)
	}
	if maxClose <= low {
		return 0
	}
	return (maxClose - low) / low
}

func calcBestBounceAroundLevel(bars []DailyBar, level float64, forward int) float64 {
	if level <= 0 {
		return 0
	}
	tolerance := math.Max(level*0.005, 0.02)
	best := 0.0
	for idx := range bars {
		bar := bars[idx]
		if bar.Low <= level+tolerance && bar.High >= level-tolerance {
			best = math.Max(best, calcBounceFromIndex(bars, idx, forward))
		}
	}
	return best
}

func movingAverageClose(bars []DailyBar, period int) float64 {
	if len(bars) < period || period <= 0 {
		return 0
	}
	sum := 0.0
	for _, bar := range bars[len(bars)-period:] {
		sum += bar.Close
	}
	return sum / float64(period)
}

func calculateRSI(bars []DailyBar, period int) float64 {
	if len(bars) < period+1 || period <= 0 {
		return -1 // not enough data
	}
	recent := bars[len(bars)-period-1:]
	avgGain := 0.0
	avgLoss := 0.0
	for i := 1; i < len(recent); i++ {
		delta := recent[i].Close - recent[i-1].Close
		if delta > 0 {
			avgGain += delta
		} else {
			avgLoss -= delta
		}
	}
	avgGain /= float64(period)
	avgLoss /= float64(period)
	if avgLoss == 0 {
		return 100
	}
	rs := avgGain / avgLoss
	return 100 - 100/(1+rs)
}

func classifyRSIStatus(rsi float64) string {
	if rsi < 0 {
		return "数据不足"
	}
	switch {
	case rsi >= 80:
		return "极度超买"
	case rsi >= 70:
		return "超买"
	case rsi <= 20:
		return "极度超卖"
	case rsi <= 30:
		return "超卖"
	default:
		return "中性"
	}
}

func parseBarDate(raw string) time.Time {
	text := strings.TrimSpace(raw)
	if text == "" {
		return time.Time{}
	}
	t, err := time.Parse("2006-01-02", text)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

func sortSupportSources(sourceSet map[string]struct{}) []string {
	if len(sourceSet) == 0 {
		return []string{}
	}
	items := make([]string, 0, len(sourceSet))
	for source := range sourceSet {
		items = append(items, source)
	}
	order := map[string]int{
		"swing": 1,
		"pivot": 2,
		"ma60":  3,
		"ma120": 4,
	}
	sort.Slice(items, func(i, j int) bool {
		left := order[items[i]]
		right := order[items[j]]
		if left == 0 {
			left = 999
		}
		if right == 0 {
			right = 999
		}
		if left == right {
			return items[i] < items[j]
		}
		return left < right
	})
	return items
}

func roundTo(value float64, digits int) float64 {
	factor := math.Pow10(maxInt(digits, 0))
	return math.Round(value*factor) / factor
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
