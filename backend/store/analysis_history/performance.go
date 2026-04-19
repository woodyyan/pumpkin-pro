package analysis_history

import (
	"sort"
	"strings"
	"time"
)

const (
	PriceBasisAnalysis       = "analysis"
	PriceBasisEstimatedClose = "estimated_close"
	PriceBasisMixed          = "mixed"

	DirectionStatusAligned  = "aligned"
	DirectionStatusOpposite = "opposite"
	DirectionStatusNeutral  = "neutral"
)

type ResolvedPrice struct {
	Price float64
	Basis string
}

func BuildSignalPerformances(records []AnalysisHistoryRecord, resolved map[string]ResolvedPrice) map[string]*HistorySignalPerformance {
	if len(records) == 0 {
		return map[string]*HistorySignalPerformance{}
	}

	ordered := append([]AnalysisHistoryRecord(nil), records...)
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].CreatedAt.Before(ordered[j].CreatedAt)
	})

	type previousPoint struct {
		CreatedAt time.Time
		Price     ResolvedPrice
	}

	lastBySignal := map[string]previousPoint{}
	performances := make(map[string]*HistorySignalPerformance, len(ordered))

	for _, record := range ordered {
		signal := strings.TrimSpace(record.Signal)
		current, hasCurrent := resolved[record.ID]
		if !hasCurrent || current.Price <= 0 {
			continue
		}

		perf := &HistorySignalPerformance{
			AnalysisPrice: float64Ptr(current.Price),
			PriceBasis:    normalizePriceBasis(current.Basis),
		}

		if prev, ok := lastBySignal[signal]; ok && prev.Price.Price > 0 {
			returnPct := ((current.Price - prev.Price.Price) / prev.Price.Price) * 100
			perf.PreviousAnalysisAt = prev.CreatedAt.UTC().Format(time.RFC3339)
			perf.PreviousAnalysisPrice = float64Ptr(prev.Price.Price)
			perf.ReturnPct = float64Ptr(returnPct)
			perf.DirectionStatus = classifyDirectionStatus(signal, returnPct)
			perf.PriceBasis = mergePriceBasis(current.Basis, prev.Price.Basis)
		}

		performances[record.ID] = perf
		lastBySignal[signal] = previousPoint{CreatedAt: record.CreatedAt, Price: current}
	}

	return performances
}

func mergePriceBasis(current, previous string) string {
	current = normalizePriceBasis(current)
	previous = normalizePriceBasis(previous)
	if current == "" {
		return previous
	}
	if previous == "" {
		return current
	}
	if current == previous {
		return current
	}
	return PriceBasisMixed
}

func normalizePriceBasis(basis string) string {
	basis = strings.TrimSpace(basis)
	if basis == "" {
		return PriceBasisAnalysis
	}
	return basis
}

func classifyDirectionStatus(signal string, returnPct float64) string {
	switch strings.TrimSpace(signal) {
	case "buy":
		if returnPct >= 0 {
			return DirectionStatusAligned
		}
		return DirectionStatusOpposite
	case "sell":
		if returnPct <= 0 {
			return DirectionStatusAligned
		}
		return DirectionStatusOpposite
	default:
		return DirectionStatusNeutral
	}
}

func float64Ptr(value float64) *float64 {
	v := value
	return &v
}
