package quadrant

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Service provides business logic for quadrant scores.
type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// BulkSave writes all quadrant scores from the Quant callback.
func (s *Service) BulkSave(ctx context.Context, input BulkSaveInput) (int, error) {
	computedAt := time.Now().UTC()
	if input.ComputedAt != "" {
		if parsed, err := time.Parse(time.RFC3339, input.ComputedAt); err == nil {
			computedAt = parsed.UTC()
		}
	}

	records := make([]QuadrantScoreRecord, 0, len(input.Items))
	for _, item := range input.Items {
		code := strings.TrimSpace(item.Code)
		if code == "" {
			continue
		}
		// Determine exchange: use item's explicit exchange, or default to SZSE (A-share)
		exchange := strings.TrimSpace(item.Exchange)
		if exchange == "" {
			exchange = "SZSE"
		}
		records = append(records, QuadrantScoreRecord{
			Code:        code,
			Name:        strings.TrimSpace(item.Name),
			Exchange:    exchange,
			Opportunity: item.Opportunity,
			Risk:        item.Risk,
			Quadrant:    strings.TrimSpace(item.Quadrant),
			Trend:       item.Trend,
			Flow:        item.Flow,
			Revision:    item.Revision,
			Volatility:  item.Volatility,
			Drawdown:    item.Drawdown,
			Crowding:    item.Crowding,
			ComputedAt:  computedAt,
		})
	}

	if len(records) == 0 {
		return 0, fmt.Errorf("no valid items to save")
	}

	if err := s.repo.BulkUpsert(ctx, records); err != nil {
		return 0, err
	}

	// Save compute log if report is present
	if input.Report != nil {
		s.saveComputeLog(ctx, computedAt, input.Report, len(records))
	}

	return len(records), nil
}

func (s *Service) saveComputeLog(ctx context.Context, computedAt time.Time, report map[string]any, stockCount int) {
	reportBytes, _ := json.Marshal(report)
	mode := "unknown"
	if m, ok := report["mode"].(string); ok {
		mode = m
	}
	durationSec := float64(0)
	if d, ok := report["duration_seconds"].(float64); ok {
		durationSec = d
	}
	status := "success"
	if st, ok := report["status"].(string); ok {
		status = st
	}
	errorMsg := ""
	if e, ok := report["error"].(string); ok {
		errorMsg = e
	}
	logID := fmt.Sprintf("qcl-%d", computedAt.UnixMilli())
	_ = s.repo.InsertComputeLog(ctx, ComputeLogRecord{
		ID:          logID,
		ComputedAt:  computedAt,
		Mode:        mode,
		DurationSec: durationSec,
		StockCount:  stockCount,
		ReportJSON:  string(reportBytes),
		Status:      status,
		ErrorMsg:    errorMsg,
	})
}

// GetAllWithWatchlist returns all scores (compact) + watchlist details for the given exchange.
// When exchanges is nil or empty, returns all records (no filter).
func (s *Service) GetAllWithWatchlist(ctx context.Context, exchanges []string, watchlistCodes []string) (*QuadrantResponse, error) {
	allRecords, err := s.repo.FindByExchange(ctx, exchanges)
	if err != nil {
		return nil, err
	}

	// Build compact list + summary
	allStocks := make([]QuadrantScoreCompact, 0, len(allRecords))
	summary := QuadrantSummary{}
	var latestComputedAt time.Time

	for _, r := range allRecords {
		allStocks = append(allStocks, r.ToCompact())

		switch r.Quadrant {
		case "机会":
			summary.OpportunityZone++
		case "拥挤":
			summary.CrowdedZone++
		case "泡沫":
			summary.BubbleZone++
		case "防御":
			summary.DefensiveZone++
		default:
			summary.NeutralZone++
		}

		if r.ComputedAt.After(latestComputedAt) {
			latestComputedAt = r.ComputedAt
		}
	}

	// Build watchlist details
	watchlistDetails := make([]QuadrantScoreDetail, 0, len(watchlistCodes))
	if len(watchlistCodes) > 0 {
		watchlistRecords, err := s.repo.FindBySymbols(ctx, watchlistCodes)
		if err == nil {
			for _, r := range watchlistRecords {
				watchlistDetails = append(watchlistDetails, r.ToDetail())
			}
		}
	}

	computedAtStr := ""
	if !latestComputedAt.IsZero() {
		computedAtStr = latestComputedAt.UTC().Format(time.RFC3339)
	}

	return &QuadrantResponse{
		Meta: QuadrantMeta{
			ComputedAt: computedAtStr,
			TotalCount: len(allRecords),
		},
		AllStocks:        allStocks,
		WatchlistDetails: watchlistDetails,
		Summary:          summary,
	}, nil
}

// GetStatus returns the current computation status.
func (s *Service) GetStatus(ctx context.Context) (*QuadrantStatusResponse, error) {
	count, err := s.repo.Count(ctx)
	if err != nil {
		return nil, err
	}

	latestAt, err := s.repo.GetLatestComputedAt(ctx)
	if err != nil {
		return nil, err
	}

	computedAtStr := ""
	if latestAt != nil {
		computedAtStr = latestAt.UTC().Format(time.RFC3339)
	}

	resp := &QuadrantStatusResponse{
		LastComputedAt: computedAtStr,
		StockCount:     int(count),
		LastError:      "",
	}

	// Attach last compute report if available
	lastLog, _ := s.repo.GetLatestComputeLog(ctx)
	if lastLog != nil {
		var report map[string]any
		if err := json.Unmarshal([]byte(lastLog.ReportJSON), &report); err == nil {
			resp.LastReport = report
		}
	}

	return resp, nil
}

// ListComputeLogs returns recent compute history for admin dashboard.
func (s *Service) ListComputeLogs(ctx context.Context, limit int) ([]ComputeLogRecord, error) {
	if limit <= 0 {
		limit = 30
	}
	return s.repo.ListComputeLogs(ctx, limit)
}

// Search searches stocks by query string (code or name).
func (s *Service) Search(ctx context.Context, q string, limit int) ([]SearchResult, error) {
	if len(q) < 2 {
		return []SearchResult{}, nil
	}
	return s.repo.Search(ctx, q, limit)
}

// ── Admin Overview ──

// ExchangeOverview holds per-exchange quadrant statistics.
type ExchangeOverview struct {
	Exchange     string          `json:"exchange"`      // "ASHARE", "HKEX"
	TotalCount   int64           `json:"total_count"`
	LastComputed string          `json:"last_computed"`
	Summary      QuadrantSummary `json:"summary"`
}

// QuadrantOverviewResponse is the response for GET /api/admin/quadrant-overview.
type QuadrantOverviewResponse struct {
	Exchanges    []ExchangeOverview `json:"exchanges"`
	GrandTotal   int64             `json:"grand_total"`
	GrandSummary QuadrantSummary   `json:"grand_summary"`
}

// GetAdminOverview returns quadrant statistics grouped by exchange for the admin dashboard.
func (s *Service) GetAdminOverview(ctx context.Context) (*QuadrantOverviewResponse, error) {
	records, err := s.repo.FindAll(ctx)
	if err != nil {
		return nil, err
	}

	// Group by exchange
	m := make(map[string]exchangeAccum)
	var grandSummary QuadrantSummary

	for _, r := range records {
		ex := r.Exchange
		if ex == "SSE" || ex == "SZSE" {
			ex = "ASHARE"
		}

		e := m[ex]
		e.key = ex
		e.count++
		if r.ComputedAt.After(e.lastComputed) {
			e.lastComputed = r.ComputedAt
		}

		switch r.Quadrant {
		case "机会":
			e.summary.OpportunityZone++
			grandSummary.OpportunityZone++
		case "拥挤":
			e.summary.CrowdedZone++
			grandSummary.CrowdedZone++
		case "泡沫":
			e.summary.BubbleZone++
			grandSummary.BubbleZone++
		case "防御":
			e.summary.DefensiveZone++
			grandSummary.DefensiveZone++
		default:
			grandSummary.NeutralZone++
			e.summary.NeutralZone++
		}

		m[ex] = e
	}

	exchanges := []ExchangeOverview{
		buildExchangeOverview(m["ASHARE"], "A股"),
		buildExchangeOverview(m["HKEX"], "港股"),
	}

	grandTotal := int64(len(records))
	return &QuadrantOverviewResponse{
		Exchanges:    exchanges,
		GrandTotal:   grandTotal,
		GrandSummary: grandSummary,
	}, nil
}

// exchangeAccum is an internal accumulator for per-exchange stats.
type exchangeAccum struct {
	key          string
	lastComputed time.Time
	summary      QuadrantSummary
	count        int64
}

func buildExchangeOverview(e exchangeAccum, label string) ExchangeOverview {
	computedAtStr := ""
	if !e.lastComputed.IsZero() {
		computedAtStr = e.lastComputed.UTC().Format(time.RFC3339)
	}
	return ExchangeOverview{
		Exchange:     label,
		TotalCount:   e.count,
		LastComputed: computedAtStr,
		Summary:      e.summary,
	}
}
