package analysis_history

import (
	"sort"
	"strings"
	"time"

	"github.com/woodyyan/pumpkin-pro/backend/store/live"
)

const (
	PrimaryValidationWindowDays = 5

	QualitySummaryPending = "pending"
	QualitySummaryHit     = "hit"
	QualitySummaryMiss    = "miss"
	QualitySummaryStable  = "stable"
	QualitySummaryDrift   = "drift"
	QualitySummaryUnknown = "unknown"

	QualityLabelPending = "验证中"
	QualityLabelHit     = "命中"
	QualityLabelMiss    = "失准"
	QualityLabelStable  = "平稳"
	QualityLabelDrift   = "偏离"
	QualityLabelUnknown = "区间变动"
)

var validationWindows = []int{1, 3, 5, 10}

func BuildQualityValidations(records []AnalysisHistoryRecord, resolved map[string]ResolvedPrice, bars []live.DailyBar) map[string]*HistoryQualityValidation {
	validations := make(map[string]*HistoryQualityValidation, len(records))
	if len(records) == 0 || len(bars) == 0 {
		return validations
	}

	sortedBars := normalizeDailyBars(bars)
	if len(sortedBars) == 0 {
		return validations
	}

	for _, record := range records {
		entry, ok := resolved[record.ID]
		if !ok || entry.Price <= 0 {
			continue
		}
		validation := buildQualityValidation(record, entry, sortedBars)
		if validation == nil {
			continue
		}
		validations[record.ID] = validation
	}

	return validations
}

func buildQualityValidation(record AnalysisHistoryRecord, entry ResolvedPrice, bars []live.DailyBar) *HistoryQualityValidation {
	futureBars := collectFutureTradingBars(record.CreatedAt, bars)
	validation := &HistoryQualityValidation{
		PrimaryWindowDays: PrimaryValidationWindowDays,
		AvailableDays:     len(futureBars),
		PriceBasis:        normalizePriceBasis(entry.Basis),
		Windows:           make([]HistoryQualityWindow, 0, len(validationWindows)),
	}

	var primary *HistoryQualityWindow
	for _, horizon := range validationWindows {
		window := buildQualityWindow(record.Signal, entry.Price, futureBars, horizon)
		validation.Windows = append(validation.Windows, window)
		if horizon == PrimaryValidationWindowDays {
			windowCopy := window
			primary = &windowCopy
		}
	}

	applyQualitySummary(validation, strings.TrimSpace(record.Signal), primary)
	return validation
}

func applyQualitySummary(validation *HistoryQualityValidation, signal string, primary *HistoryQualityWindow) {
	if validation == nil {
		return
	}
	if primary == nil || !primary.Ready || primary.ReturnPct == nil {
		validation.SummaryStatus = QualitySummaryPending
		validation.SummaryLabel = QualityLabelPending
		validation.PrimaryReturnPct = nil
		return
	}

	validation.PrimaryReturnPct = float64Ptr(*primary.ReturnPct)
	switch strings.TrimSpace(signal) {
	case "buy", "sell":
		if primary.DirectionStatus == QualitySummaryHit {
			validation.SummaryStatus = QualitySummaryHit
			validation.SummaryLabel = QualityLabelHit
			return
		}
		validation.SummaryStatus = QualitySummaryMiss
		validation.SummaryLabel = QualityLabelMiss
	case "hold":
		validation.SummaryStatus = QualitySummaryUnknown
		validation.SummaryLabel = QualityLabelUnknown
	default:
		validation.SummaryStatus = QualitySummaryUnknown
		validation.SummaryLabel = QualityLabelUnknown
	}
}

func buildQualityWindow(signal string, entryPrice float64, futureBars []live.DailyBar, horizon int) HistoryQualityWindow {
	window := HistoryQualityWindow{HorizonDays: horizon}
	if entryPrice <= 0 || horizon <= 0 || len(futureBars) < horizon {
		window.DirectionStatus = QualitySummaryPending
		return window
	}

	horizonBars := futureBars[:horizon]
	endBar := horizonBars[horizon-1]
	if endBar.Close <= 0 {
		window.DirectionStatus = QualitySummaryPending
		return window
	}

	returnPct := ((endBar.Close - entryPrice) / entryPrice) * 100
	maxHigh, minLow := computeWindowExtremes(horizonBars)
	window.Ready = true
	window.EndDate = endBar.Date
	window.ClosePrice = float64Ptr(endBar.Close)
	window.ReturnPct = float64Ptr(returnPct)
	if maxHigh > 0 {
		window.MaxUpPct = float64Ptr(((maxHigh - entryPrice) / entryPrice) * 100)
	}
	if minLow > 0 {
		window.MaxDownPct = float64Ptr(((minLow - entryPrice) / entryPrice) * 100)
	}
	window.DirectionStatus = classifyQualityDirection(signal, returnPct)
	return window
}

func classifyQualityDirection(signal string, returnPct float64) string {
	switch strings.TrimSpace(signal) {
	case "buy":
		if returnPct >= 0 {
			return QualitySummaryHit
		}
		return QualitySummaryMiss
	case "sell":
		if returnPct <= 0 {
			return QualitySummaryHit
		}
		return QualitySummaryMiss
	case "hold":
		return QualitySummaryUnknown
	default:
		return QualitySummaryUnknown
	}
}

func collectFutureTradingBars(createdAt time.Time, bars []live.DailyBar) []live.DailyBar {
	if createdAt.IsZero() || len(bars) == 0 {
		return nil
	}
	targetDate := createdAt.In(historyMarketLocation()).Format("2006-01-02")
	future := make([]live.DailyBar, 0, len(bars))
	for _, bar := range bars {
		if strings.TrimSpace(bar.Date) == "" || bar.Close <= 0 {
			continue
		}
		if bar.Date > targetDate {
			future = append(future, bar)
		}
	}
	return future
}

func normalizeDailyBars(bars []live.DailyBar) []live.DailyBar {
	normalized := make([]live.DailyBar, 0, len(bars))
	for _, bar := range bars {
		if strings.TrimSpace(bar.Date) == "" || bar.Close <= 0 {
			continue
		}
		normalized = append(normalized, bar)
	}
	sort.Slice(normalized, func(i, j int) bool {
		return normalized[i].Date < normalized[j].Date
	})
	return normalized
}

func computeWindowExtremes(bars []live.DailyBar) (maxHigh float64, minLow float64) {
	for i, bar := range bars {
		high := bar.High
		if high <= 0 {
			high = bar.Close
		}
		low := bar.Low
		if low <= 0 {
			low = bar.Close
		}
		if i == 0 || high > maxHigh {
			maxHigh = high
		}
		if i == 0 || low < minLow {
			minLow = low
		}
	}
	return maxHigh, minLow
}

func historyMarketLocation() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("CST", 8*3600)
	}
	return loc
}
