package quadrant

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"
)

// PriceResolver resolves a stock's closing price for the given trade date.
// Implemented by the live-store module; returns 0 if unavailable.
type PriceResolver func(ctx context.Context, code string, exchange string, tradeDate string) float64

// Service provides business logic for quadrant scores.
type Service struct {
	repo          *Repository
	priceResolver PriceResolver // optional, for snapshot close_price
	worker        *Worker       // optional, injected for manual trigger
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// SetPriceResolver injects the price resolution callback (called during init).
func (s *Service) SetPriceResolver(r PriceResolver) {
	s.priceResolver = r
}

var rankingSnapshotLocation = time.FixedZone("CST", 8*60*60)

func rankingSnapshotDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.In(rankingSnapshotLocation).Format("2006-01-02")
}

// BulkSave writes all quadrant scores from the Quant callback.
// It guarantees a compute log is written regardless of success or failure.
func (s *Service) BulkSave(ctx context.Context, input BulkSaveInput) (int, error) {
	computedAt := time.Now().UTC()
	if input.ComputedAt != "" {
		if parsed, err := time.Parse(time.RFC3339, input.ComputedAt); err == nil {
			computedAt = parsed.UTC()
		}
	}

	// Determine exchange from items
	exchange := detectExchange(input.Items)

	log.Printf("[quadrant] BulkSave start: exchange=%s, items=%d", exchange, len(input.Items))

	// ── Input validation: always write a log ──
	if len(input.Items) == 0 || allEmpty(input.Items) {
		log.Printf("[quadrant] BulkSave REJECT: no valid items (received %d)", len(input.Items))
		s.saveTaskLog(ctx, ComputeLogRecord{
			ID:         fmt.Sprintf("qcl-%d", computedAt.UnixMilli()),
			ComputedAt: computedAt,
			Mode:       "unknown",
			Status:     "failed",
			ErrorMsg:   "no valid items to save",
			Exchange:   exchange,
			StartedAt:  &computedAt,
			FinishedAt: &computedAt,
		})
		SetProgressTerminal(exchange, "failed", "收到空数据（0条有效股票）")
		return 0, fmt.Errorf("no valid items to save")
	}

	records := make([]QuadrantScoreRecord, 0, len(input.Items))
	for _, item := range input.Items {
		code := strings.TrimSpace(item.Code)
		if code == "" {
			continue
		}
		itemExchange := strings.TrimSpace(item.Exchange)
		if itemExchange == "" {
			itemExchange = "SZSE"
		}
		records = append(records, QuadrantScoreRecord{
			Code: code, Name: strings.TrimSpace(item.Name), Exchange: itemExchange,
			Opportunity: item.Opportunity, Risk: item.Risk, Quadrant: strings.TrimSpace(item.Quadrant),
			Trend: item.Trend, Flow: item.Flow, Revision: item.Revision,
			Liquidity: item.Liquidity, Volatility: item.Volatility,
			Drawdown: item.Drawdown, Crowding: item.Crowding,
			AvgAmount5d: item.AvgAmount5d, ComputedAt: computedAt,
		})
	}

	totalCount := len(records)

	// ── BulkUpsert (DB write) ──
	if err := s.repo.BulkUpsert(ctx, records); err != nil {
		// DB failure — write failed log and return
		now := time.Now().UTC()
		log.Printf("[quadrant] BulkSave DB ERROR: %v (exchange=%s)", err, exchange)
		s.saveTaskLog(ctx, ComputeLogRecord{
			ID:         fmt.Sprintf("qcl-%d", computedAt.UnixMilli()),
			ComputedAt: computedAt,
			Mode:       "unknown",
			Status:     "failed",
			ErrorMsg:   fmt.Sprintf("DB写入错误: %v", err),
			Exchange:   exchange,
			StartedAt:  &computedAt,
			FinishedAt: &now,
			TotalCount: totalCount,
		})
		SetProgressTerminal(exchange, "failed", fmt.Sprintf("DB写入错误: %v", err))
		return 0, err
	}

	log.Printf("[quadrant] BulkSave OK: wrote %d records (exchange=%s)", totalCount, exchange)

	// ── BulkUpsert succeeded — parse report and write final log ──
	status := "success"
	errorMsg := ""
	mode := "unknown"
	var totalSec float64
	var successCount, failedCount int

	if input.Report != nil {
		if m, ok := input.Report["mode"].(string); ok {
			mode = m
		}
		if st, ok := input.Report["status"].(string); ok && st != "" {
			status = st
		}
		if e, ok := input.Report["error"].(string); ok {
			errorMsg = e
		}
		if d, ok := input.Report["duration_seconds"].(float64); ok {
			totalSec = d
		}
		if sc, ok := input.Report["stock_count"].(float64); ok {
			successCount = int(sc)
		}
		if fc, ok := input.Report["daily_bars"].(map[string]any); ok {
			if f, ok := fc["failed"].(float64); ok {
				failedCount = int(f)
			}
		}
	} else {
		successCount = totalCount
	}

	finishedAt := time.Now().UTC()

	// Try to reuse an existing "running" log for this exchange (avoids orphan records)
	var logID string
	if runningLog, err := s.repo.FindLatestRunningLog(ctx, exchange); err == nil && runningLog != nil {
		logID = runningLog.ID
	} else {
		logID = fmt.Sprintf("qcl-%d", computedAt.UnixMilli())
	}

	taskLog := ComputeLogRecord{
		ID:           logID,
		ComputedAt:   computedAt,
		Mode:         mode,
		DurationSec:  totalSec,
		StockCount:   totalCount,
		ReportJSON:   mustMarshal(input.Report),
		Status:       status,
		ErrorMsg:     errorMsg,
		Exchange:     exchange,
		StartedAt:    &computedAt,
		FinishedAt:   &finishedAt,
		TotalCount:   totalCount,
		SuccessCount: successCount,
		FailedCount:  failedCount,
	}

	_ = s.repo.InsertComputeLog(ctx, taskLog)

	// Update progress to terminal state
	SetProgressTerminal(exchange, status, errorMsg)
	log.Printf("[quadrant] BulkSave COMPLETE: exchange=%s status=%s log_id=%s", exchange, status, logID)

	// Save ranking snapshots (best-effort; failure does not affect BulkSave result)
	s.saveRankingSnapshotsBestEffort(ctx, records, computedAt)

	return totalCount, nil
}

// SaveTaskLog is a convenience wrapper that writes a compute log record.
func (s *Service) SaveTaskLog(ctx context.Context, log ComputeLogRecord) error {
	return s.repo.InsertComputeLog(ctx, log)
}

// saveTaskLog is an internal convenience that discards errors (best-effort).
func (s *Service) saveTaskLog(ctx context.Context, log ComputeLogRecord) {
	_ = s.repo.InsertComputeLog(ctx, log)
}

// TriggerComputeAShare manually triggers A-share quadrant computation.
// It delegates to the worker's triggerCompute logic (writes running log + progress).
func (s *Service) TriggerComputeAShare() {
	if s.worker == nil {
		return
	}
	s.worker.TriggerComputeAShare()
}

// TriggerComputeHK manually triggers HK quadrant computation.
func (s *Service) TriggerComputeHK() {
	if s.worker == nil {
		return
	}
	s.worker.TriggerComputeHK()
}

// SetWorker injects the worker reference for manual trigger support.
func (s *Service) SetWorker(w *Worker) {
	s.worker = w
}

// allEmpty returns true if every item has an empty code.
func allEmpty(items []BulkSaveItem) bool {
	for _, it := range items {
		if strings.TrimSpace(it.Code) != "" {
			return false
		}
	}
	return true
}

// detectExchange infers the exchange from the first non-empty item's exchange field.
func detectExchange(items []BulkSaveItem) string {
	for _, it := range items {
		if ex := strings.TrimSpace(it.Exchange); ex != "" {
			return ex
		}
	}
	return "SZSE" // default fallback
}

// mustMarshal JSON-encodes a value; returns "{}" on error (never panics).
func mustMarshal(v any) string {
	b, _ := json.Marshal(v)
	if len(b) == 0 {
		return "{}"
	}
	return string(b)
}

// saveRankingSnapshotsBestEffort saves top-50 opportunity-zone records as daily ranking snapshots.
// This is best-effort: errors are logged but not propagated to the caller.
func (s *Service) saveRankingSnapshotsBestEffort(ctx context.Context, records []QuadrantScoreRecord, computedAt time.Time) {
	// Extract opportunity-zone records only
	var oppRecords []QuadrantScoreRecord
	for _, r := range records {
		if r.Quadrant == "机会" && r.Opportunity > 0 {
			oppRecords = append(oppRecords, r)
		}
	}
	if len(oppRecords) == 0 {
		return
	}

	// Sort by opportunity DESC (same as ranking order)
	sort.Slice(oppRecords, func(i, j int) bool {
		return oppRecords[i].Opportunity > oppRecords[j].Opportunity
	})
	if len(oppRecords) > 50 {
		oppRecords = oppRecords[:50]
	}

	now := time.Now().UTC()
	dateStr := rankingSnapshotDate(computedAt)
	snaps := make([]RankingSnapshot, 0, len(oppRecords))
	for i, r := range oppRecords {
		closePrice := 0.0
		if s.priceResolver != nil {
			closePrice = s.priceResolver(ctx, r.Code, r.Exchange, dateStr)
		}
		snaps = append(snaps, RankingSnapshot{
			Code:         r.Code,
			Name:         r.Name,
			Exchange:     r.Exchange,
			Rank:         i + 1,
			Opportunity:  r.Opportunity,
			Risk:         r.Risk,
			ClosePrice:   closePrice,
			SnapshotDate: dateStr,
			CreatedAt:    now,
		})
	}
	if err := s.repo.UpsertSnapshots(ctx, snaps); err != nil {
		fmt.Printf("[quadrant] WARNING: failed to save ranking snapshots: %v\n", err)
	}
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
	Exchange     string          `json:"exchange"` // "ASHARE", "HKEX"
	TotalCount   int64           `json:"total_count"`
	LastComputed string          `json:"last_computed"`
	Summary      QuadrantSummary `json:"summary"`
}

// QuadrantOverviewResponse is the response for GET /api/admin/quadrant-overview.
type QuadrantOverviewResponse struct {
	Exchanges    []ExchangeOverview `json:"exchanges"`
	GrandTotal   int64              `json:"grand_total"`
	GrandSummary QuadrantSummary    `json:"grand_summary"`
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

// ── Ranking (卧龙AI精选) ─_

// resolveRankingExchanges converts an API exchange param to DB-level exchange codes.
func resolveRankingExchanges(exchange string) []string {
	switch strings.ToUpper(strings.TrimSpace(exchange)) {
	case "HKEX":
		return []string{"HKEX"}
	case "ASHARE", "":
		fallthrough
	default:
		return []string{"SSE", "SZSE"}
	}
}

// GetRanking returns the top-N stocks from the opportunity zone (机会区),
// ordered by opportunity DESC + risk ASC.
// Applies a minimum liquidity filter (avg_amount_5d threshold) per market:
//
//	A-share: 5000 万元, HKEX: 2000 万 HKD.
//
// Backward compatibility: if no records have non-zero avg_amount5d (i.e.
// data was computed before the liquidity field was introduced), the filter is
// automatically disabled so old datasets still show results.
func (s *Service) GetRanking(ctx context.Context, exchange string, limit int) (*RankingResponse, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	exchanges := resolveRankingExchanges(exchange)

	// Liquidity hard-filter thresholds (万元 / 万HKD)
	defaultMinAmount := 5000.0 // A-share default
	for _, ex := range exchanges {
		if ex == "HKEX" {
			defaultMinAmount = 2000.0
		}
	}

	// Check if the data has been recomputed with liquidity field.
	// If all avg_amount5d are zero/NULL, skip the filter (backward compat).
	hasLiquidityData, err := s.repo.HasNonZeroLiquidity(ctx, exchanges)
	if err != nil {
		return nil, fmt.Errorf("check liquidity data: %w", err)
	}
	minAmount := defaultMinAmount
	if !hasLiquidityData {
		minAmount = 0
	}

	records, totalInZone, err := s.repo.FindOpportunityZone(ctx, exchanges, limit, minAmount)
	if err != nil {
		return nil, err
	}

	items := make([]RankingItem, 0, len(records))
	var latestComputedAt time.Time

	for i, r := range records {
		item := RankingItem{
			Rank:        i + 1,
			Code:        r.Code,
			Name:        r.Name,
			Exchange:    r.Exchange,
			Opportunity: r.Opportunity,
			Risk:        r.Risk,
			Quadrant:    r.Quadrant,
			Trend:       r.Trend,
			Flow:        r.Flow,
			Revision:    r.Revision,
			Liquidity:   r.Liquidity,
			AvgAmount5d: r.AvgAmount5d,
		}
		if r.ComputedAt.After(latestComputedAt) {
			latestComputedAt = r.ComputedAt
		}

		// Enrich with consecutive days and return since first appearance
		days, _ := s.repo.GetConsecutiveDays(ctx, r.Code, exchanges)
		item.ConsecutiveDays = days

		firstDateStr, _ := s.repo.GetFirstAppearedDate(ctx, r.Code, exchanges)
		if firstDateStr != "" {
			startPrice, _ := s.repo.GetClosePriceOnDate(ctx, r.Code, firstDateStr)
			currentPrice, _, _ := s.repo.GetLatestAvailableClosePrice(ctx, r.Code, exchanges)
			if startPrice > 0 && currentPrice > 0 {
				pct := (currentPrice - startPrice) / startPrice * 100
				item.ReturnPct = &pct
			}
		}

		items = append(items, item)
	}

	computedAtStr := ""
	if !latestComputedAt.IsZero() {
		computedAtStr = latestComputedAt.UTC().Format(time.RFC3339)
	}

	displayExchange := strings.ToUpper(strings.TrimSpace(exchange))
	if displayExchange == "" || displayExchange != "HKEX" {
		displayExchange = "ASHARE"
	}

	return &RankingResponse{
		Meta: RankingMeta{
			ComputedAt:    computedAtStr,
			TotalInZone:   int(totalInZone),
			ReturnedCount: len(items),
			Exchange:      displayExchange,
		},
		Items: items,
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
