package live

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

const maxResistanceLevels = 3

type resistanceCandidate struct {
	Price           float64
	Source          string
	Weight          float64
	SourceScore     float64
	TouchCount      int
	LastValidatedAt time.Time
	Rejection       float64
}

type resistanceBand struct {
	Price           float64
	BandLow         float64
	BandHigh        float64
	WeightSum       float64
	SourceScore     float64
	TouchCount      int
	LastValidatedAt time.Time
	Rejection       float64
	Sources         map[string]struct{}
}

func (s *Service) GetResistanceLevels(ctx context.Context, userID, symbol, period string, lookbackDays int) (*ResistanceLevelsPayload, error) {
	normalizedSymbol, err := normalizeHKSymbol(symbol)
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

	candidates := buildResistanceCandidates(bars, lastBar.Close)
	if len(candidates) == 0 {
		return nil, ErrWarmupNotReady
	}

	bands := clusterResistanceBands(candidates, lastBar.Close)
	if len(bands) == 0 {
		return nil, ErrWarmupNotReady
	}

	levels, summary := buildResistanceLevelsAndSummary(bands, lastBar.Close, lastBar.Date)
	if len(levels) == 0 {
		return nil, ErrWarmupNotReady
	}

	sessionState := s.resolveSessionState(userID)
	now := time.Now().UTC().Format(time.RFC3339)

	return &ResistanceLevelsPayload{
		Symbol:       normalizedSymbol,
		Period:       supportPeriodDaily,
		LookbackDays: lookbackDays,
		AsOf:         lastBar.Date,
		PriceRef:     roundTo(lastBar.Close, 4),
		SessionState: sessionState,
		Summary:      summary,
		Levels:       levels,
		Meta: ResistanceMeta{
			Algorithm:          "resistance-v1-fusion-daily",
			SampleCount:        len(bars),
			MinRequiredSamples: minSupportSampleCount,
			IsWarmup:           false,
			UpdatedAt:          now,
		},
	}, nil
}

func buildResistanceCandidates(bars []DailyBar, lastClose float64) []resistanceCandidate {
	candidates := make([]resistanceCandidate, 0, 16)
	candidates = append(candidates, buildSwingResistanceCandidates(bars)...)
	candidates = append(candidates, buildPivotResistanceCandidates(bars)...)
	candidates = append(candidates, buildMAResistanceCandidates(bars)...)

	filtered := make([]resistanceCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Price <= 0 || math.IsNaN(candidate.Price) || math.IsInf(candidate.Price, 0) {
			continue
		}
		if candidate.Price < lastClose*0.99 {
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

func buildSwingResistanceCandidates(bars []DailyBar) []resistanceCandidate {
	if len(bars) < swingLookaround*2+1 {
		return nil
	}
	candidates := make([]resistanceCandidate, 0, 10)
	for idx := swingLookaround; idx < len(bars)-swingLookaround; idx++ {
		currentHigh := bars[idx].High
		if currentHigh <= 0 {
			continue
		}
		isSwing := true
		for i := idx - swingLookaround; i <= idx+swingLookaround; i++ {
			if i == idx {
				continue
			}
			if bars[i].High >= currentHigh {
				isSwing = false
				break
			}
		}
		if !isSwing {
			continue
		}

		touchCount, lastValidatedAt := measureTouchesAndLastValidatedAt(bars, currentHigh, 0.005)
		rejection := calcPullbackFromIndex(bars, idx, 10)
		if lastValidatedAt.IsZero() {
			lastValidatedAt = parseBarDate(bars[idx].Date)
		}
		candidates = append(candidates, resistanceCandidate{
			Price:           currentHigh,
			Source:          "swing",
			Weight:          1.2 + math.Min(0.3, rejection),
			SourceScore:     100,
			TouchCount:      maxInt(touchCount, 1),
			LastValidatedAt: lastValidatedAt,
			Rejection:       rejection,
		})
	}
	return candidates
}

func buildPivotResistanceCandidates(bars []DailyBar) []resistanceCandidate {
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
	r1 := 2*pp - pivotBar.Low
	r2 := pp + (pivotBar.High - pivotBar.Low)

	pivotDate := parseBarDate(pivotBar.Date)
	cand := make([]resistanceCandidate, 0, 2)
	for _, price := range []float64{r1, r2} {
		if price <= 0 {
			continue
		}
		touchCount, lastValidatedAt := measureTouchesAndLastValidatedAt(bars, price, 0.005)
		rejection := calcBestPullbackAroundLevel(bars, price, 10)
		if lastValidatedAt.IsZero() {
			lastValidatedAt = pivotDate
		}
		cand = append(cand, resistanceCandidate{
			Price:           price,
			Source:          "pivot",
			Weight:          1,
			SourceScore:     70,
			TouchCount:      maxInt(touchCount, 1),
			LastValidatedAt: lastValidatedAt,
			Rejection:       rejection,
		})
	}
	return cand
}

func buildMAResistanceCandidates(bars []DailyBar) []resistanceCandidate {
	candidates := make([]resistanceCandidate, 0, 2)

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
		rejection := calcBestPullbackAroundLevel(bars, ma, 10)
		candidates = append(candidates, resistanceCandidate{
			Price:           ma,
			Source:          source,
			Weight:          0.9,
			SourceScore:     sourceScore,
			TouchCount:      maxInt(touchCount, 1),
			LastValidatedAt: lastValidatedAt,
			Rejection:       rejection,
		})
	}
	return candidates
}

func clusterResistanceBands(candidates []resistanceCandidate, lastClose float64) []resistanceBand {
	if len(candidates) == 0 {
		return nil
	}
	sortedCandidates := append([]resistanceCandidate(nil), candidates...)
	sort.Slice(sortedCandidates, func(i, j int) bool {
		return sortedCandidates[i].Price < sortedCandidates[j].Price
	})

	eps := math.Max(lastClose*0.006, 0.02)
	bands := make([]resistanceBand, 0, len(sortedCandidates))
	for _, candidate := range sortedCandidates {
		if len(bands) == 0 {
			bands = append(bands, resistanceBand{
				Price:           candidate.Price,
				BandLow:         candidate.Price,
				BandHigh:        candidate.Price,
				WeightSum:       candidate.Weight,
				SourceScore:     candidate.SourceScore,
				TouchCount:      candidate.TouchCount,
				LastValidatedAt: candidate.LastValidatedAt,
				Rejection:       candidate.Rejection,
				Sources:         map[string]struct{}{candidate.Source: {}},
			})
			continue
		}

		lastIdx := len(bands) - 1
		lastBand := &bands[lastIdx]
		if math.Abs(candidate.Price-lastBand.Price) > eps {
			bands = append(bands, resistanceBand{
				Price:           candidate.Price,
				BandLow:         candidate.Price,
				BandHigh:        candidate.Price,
				WeightSum:       candidate.Weight,
				SourceScore:     candidate.SourceScore,
				TouchCount:      candidate.TouchCount,
				LastValidatedAt: candidate.LastValidatedAt,
				Rejection:       candidate.Rejection,
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
		lastBand.Rejection = math.Max(lastBand.Rejection, candidate.Rejection)
		lastBand.SourceScore = math.Max(lastBand.SourceScore, candidate.SourceScore)
		if lastBand.Sources == nil {
			lastBand.Sources = map[string]struct{}{}
		}
		lastBand.Sources[candidate.Source] = struct{}{}
	}
	return bands
}

func buildResistanceLevelsAndSummary(bands []resistanceBand, priceRef float64, asOfDate string) ([]ResistanceLevel, ResistanceSummary) {
	sortedBands := append([]resistanceBand(nil), bands...)
	sort.Slice(sortedBands, func(i, j int) bool {
		di := math.Abs(priceRef - sortedBands[i].Price)
		dj := math.Abs(priceRef - sortedBands[j].Price)
		if di == dj {
			return sortedBands[i].Price < sortedBands[j].Price
		}
		return di < dj
	})
	if len(sortedBands) > maxResistanceLevels {
		sortedBands = sortedBands[:maxResistanceLevels]
	}

	levels := make([]ResistanceLevel, 0, len(sortedBands))
	for idx, band := range sortedBands {
		distancePct := 0.0
		if priceRef > 0 {
			distancePct = (band.Price - priceRef) / priceRef * 100
		}
		score := scoreResistanceBand(band)
		status := classifyResistanceStatus(priceRef, band.BandLow, band.BandHigh, distancePct)
		lastValidatedAt := asOfDate
		if !band.LastValidatedAt.IsZero() {
			lastValidatedAt = band.LastValidatedAt.UTC().Format("2006-01-02")
		}

		levels = append(levels, ResistanceLevel{
			Level:           fmt.Sprintf("R%d", idx+1),
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

	summary := ResistanceSummary{}
	if len(levels) > 0 {
		nearest := levels[0]
		summary = ResistanceSummary{
			NearestLevel: nearest.Level,
			NearestPrice: nearest.Price,
			DistancePct:  nearest.DistancePct,
			Strength:     nearest.Strength,
			Status:       nearest.Status,
		}
	}

	return levels, summary
}

func scoreResistanceBand(band resistanceBand) float64 {
	touchScore := math.Min(100, float64(maxInt(band.TouchCount, 1))*20)

	recencyScore := 20.0
	if !band.LastValidatedAt.IsZero() {
		days := time.Since(band.LastValidatedAt).Hours() / 24
		recencyScore = math.Max(0, 100-days*2)
	}

	rejectionScore := math.Min(100, math.Max(0, band.Rejection)*200)
	sourceScore := math.Max(40, band.SourceScore)

	score := touchScore*0.35 + recencyScore*0.25 + rejectionScore*0.25 + sourceScore*0.15
	if score < 0 {
		return 0
	}
	if score > 100 {
		return 100
	}
	return score
}

func classifyResistanceStatus(priceRef, bandLow, bandHigh, distancePct float64) string {
	if priceRef >= bandLow && priceRef <= bandHigh {
		return "回踩压力"
	}
	if priceRef > bandHigh {
		return "突破压力"
	}
	if distancePct <= 2 {
		return "临近压力"
	}
	if priceRef < bandLow {
		return "位于压力下方"
	}
	return "临近压力"
}

func calcPullbackFromIndex(bars []DailyBar, idx int, forward int) float64 {
	if idx < 0 || idx >= len(bars) {
		return 0
	}
	high := bars[idx].High
	if high <= 0 {
		return 0
	}
	end := idx + forward
	if end >= len(bars) {
		end = len(bars) - 1
	}
	minClose := bars[idx].Close
	for i := idx + 1; i <= end; i++ {
		minClose = math.Min(minClose, bars[i].Close)
	}
	if minClose >= high {
		return 0
	}
	return (high - minClose) / high
}

func calcBestPullbackAroundLevel(bars []DailyBar, level float64, forward int) float64 {
	if level <= 0 {
		return 0
	}
	tolerance := math.Max(level*0.005, 0.02)
	best := 0.0
	for idx := range bars {
		bar := bars[idx]
		if bar.Low <= level+tolerance && bar.High >= level-tolerance {
			best = math.Max(best, calcPullbackFromIndex(bars, idx, forward))
		}
	}
	return best
}
