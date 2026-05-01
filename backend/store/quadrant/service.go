package quadrant

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
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

const (
	recentSnapshotRepairLookbackDays = 14
	snapshotPriceHintMaxGapDays      = 3
)

type snapshotPriceHint struct {
	ClosePrice float64
	TradeDate  string
}

func rankingSnapshotDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.In(rankingSnapshotLocation).Format("2006-01-02")
}

func snapshotPriceHintKey(code, exchange string) string {
	return strings.ToUpper(strings.TrimSpace(exchange)) + "\x00" + strings.TrimSpace(code)
}

func validPriceTradeDate(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if _, err := time.ParseInLocation("2006-01-02", value, rankingSnapshotLocation); err != nil {
		return ""
	}
	return value
}

func priceHintUsableForSnapshot(hint snapshotPriceHint, snapshotDate string) (float64, string) {
	if hint.ClosePrice <= 0 {
		return 0, ""
	}
	tradeDate := validPriceTradeDate(hint.TradeDate)
	if tradeDate == "" {
		return 0, ""
	}
	snapDate, snapErr := time.ParseInLocation("2006-01-02", snapshotDate, rankingSnapshotLocation)
	hintDate, hintErr := time.ParseInLocation("2006-01-02", tradeDate, rankingSnapshotLocation)
	if snapErr != nil || hintErr != nil || hintDate.After(snapDate) {
		return 0, ""
	}
	gapDays := int(snapDate.Sub(hintDate).Hours() / 24)
	if gapDays > snapshotPriceHintMaxGapDays {
		return 0, ""
	}
	return hint.ClosePrice, tradeDate
}

func recentSnapshotRepairFromDate(snapshotDate string) string {
	parsed, err := time.ParseInLocation("2006-01-02", snapshotDate, rankingSnapshotLocation)
	if err != nil {
		return ""
	}
	return parsed.AddDate(0, 0, -recentSnapshotRepairLookbackDays).Format("2006-01-02")
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
	priceHints := make(map[string]snapshotPriceHint, len(input.Items))
	for _, item := range input.Items {
		code := strings.TrimSpace(item.Code)
		if code == "" {
			continue
		}
		itemExchange := strings.TrimSpace(item.Exchange)
		if itemExchange == "" {
			itemExchange = "SZSE"
		}
		if item.ClosePrice > 0 {
			priceHints[snapshotPriceHintKey(code, itemExchange)] = snapshotPriceHint{
				ClosePrice: item.ClosePrice,
				TradeDate:  validPriceTradeDate(item.PriceTradeDate),
			}
		}
		records = append(records, QuadrantScoreRecord{
			Code: code, Name: strings.TrimSpace(item.Name), Exchange: itemExchange,
			Opportunity: item.Opportunity, Risk: item.Risk, Quadrant: strings.TrimSpace(item.Quadrant),
			Trend: item.Trend, Flow: item.Flow, Revision: item.Revision,
			Liquidity: item.Liquidity, Volatility: item.Volatility,
			Drawdown: item.Drawdown, Crowding: item.Crowding,
			AvgAmount5d: item.AvgAmount5d,
			Board:       strings.TrimSpace(item.Board), RankingScore: item.RankingScore,
			GlobalRankScore: item.GlobalRankScore, BoardRankScore: item.BoardRankScore,
			TradabilityScore: item.TradabilityScore, RiskAdjustedMomentum60d: item.RiskAdjustedMomentum60d,
			ComputedAt: computedAt,
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
	s.saveRankingSnapshotsBestEffortWithHints(ctx, records, computedAt, priceHints)

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

func applyBoardCap(records []QuadrantScoreRecord, limit int, maxShare float64) []QuadrantScoreRecord {
	if limit <= 0 {
		limit = 20
	}
	if len(records) == 0 {
		return []QuadrantScoreRecord{}
	}
	if maxShare <= 0 || maxShare > 1 {
		maxShare = 0.5
	}
	capPerBoard := int(math.Ceil(float64(limit) * maxShare))
	if capPerBoard < 1 {
		capPerBoard = 1
	}

	selected := make([]QuadrantScoreRecord, 0, limit)
	skipped := make([]QuadrantScoreRecord, 0)
	counts := map[string]int{}

	for _, record := range records {
		board := strings.TrimSpace(record.Board)
		if board == "" {
			board = "OTHER"
		}
		if counts[board] >= capPerBoard {
			skipped = append(skipped, record)
			continue
		}
		selected = append(selected, record)
		counts[board]++
		if len(selected) == limit {
			return selected
		}
	}

	for _, record := range skipped {
		selected = append(selected, record)
		if len(selected) == limit {
			break
		}
	}
	return selected
}

func isAShareRecords(records []QuadrantScoreRecord) bool {
	if len(records) == 0 {
		return false
	}
	for _, record := range records {
		if record.Exchange != "SSE" && record.Exchange != "SZSE" {
			return false
		}
	}
	return true
}

func hasNonZeroRankingScore(records []QuadrantScoreRecord) bool {
	for _, record := range records {
		if record.RankingScore > 0 {
			return true
		}
	}
	return false
}

func selectSnapshotRecords(records []QuadrantScoreRecord, limit int) []QuadrantScoreRecord {
	if limit <= 0 {
		limit = 50
	}
	if len(records) == 0 {
		return nil
	}

	if isAShareRecords(records) {
		filtered := make([]QuadrantScoreRecord, 0, len(records))
		for _, record := range records {
			if strings.Contains(strings.ToUpper(strings.TrimSpace(record.Name)), "ST") {
				continue
			}
			if record.AvgAmount5d < 5000 {
				continue
			}
			filtered = append(filtered, record)
		}
		if hasNonZeroRankingScore(filtered) {
			sort.Slice(filtered, func(i, j int) bool {
				if filtered[i].RankingScore == filtered[j].RankingScore {
					if filtered[i].Risk == filtered[j].Risk {
						return filtered[i].AvgAmount5d > filtered[j].AvgAmount5d
					}
					return filtered[i].Risk < filtered[j].Risk
				}
				return filtered[i].RankingScore > filtered[j].RankingScore
			})
			return applyBoardCap(filtered, limit, 0.5)
		}
		records = filtered
	}

	oppRecords := make([]QuadrantScoreRecord, 0, len(records))
	for _, record := range records {
		if record.Quadrant == "机会" && record.Opportunity > 0 {
			oppRecords = append(oppRecords, record)
		}
	}
	sort.Slice(oppRecords, func(i, j int) bool {
		if oppRecords[i].Opportunity == oppRecords[j].Opportunity {
			return oppRecords[i].Risk < oppRecords[j].Risk
		}
		return oppRecords[i].Opportunity > oppRecords[j].Opportunity
	})
	if len(oppRecords) > limit {
		oppRecords = oppRecords[:limit]
	}
	return oppRecords
}

// saveRankingSnapshotsBestEffort saves daily ranking snapshots using the active market-specific ranking rules.
// This is best-effort: errors are logged but not propagated to the caller.
func (s *Service) saveRankingSnapshotsBestEffort(ctx context.Context, records []QuadrantScoreRecord, computedAt time.Time) {
	s.saveRankingSnapshotsBestEffortWithHints(ctx, records, computedAt, nil)
}

func (s *Service) saveRankingSnapshotsBestEffortWithHints(ctx context.Context, records []QuadrantScoreRecord, computedAt time.Time, priceHints map[string]snapshotPriceHint) {
	selectedRecords := selectSnapshotRecords(records, 50)
	if len(selectedRecords) == 0 {
		return
	}

	now := time.Now().UTC()
	dateStr := rankingSnapshotDate(computedAt)
	snaps := make([]RankingSnapshot, 0, len(selectedRecords))
	missingPrices := 0
	for i, r := range selectedRecords {
		closePrice := 0.0
		priceTradeDate := ""
		if hint, ok := priceHints[snapshotPriceHintKey(r.Code, r.Exchange)]; ok && hint.ClosePrice > 0 {
			closePrice = hint.ClosePrice
			priceTradeDate = hint.TradeDate
		}
		if closePrice <= 0 && s.priceResolver != nil {
			closePrice = s.priceResolver(ctx, r.Code, r.Exchange, dateStr)
			if closePrice > 0 {
				priceTradeDate = dateStr
			}
		}
		if closePrice <= 0 {
			missingPrices++
		}
		snaps = append(snaps, RankingSnapshot{
			Code:           r.Code,
			Name:           r.Name,
			Exchange:       r.Exchange,
			Rank:           i + 1,
			Opportunity:    r.Opportunity,
			Risk:           r.Risk,
			ClosePrice:     closePrice,
			PriceTradeDate: priceTradeDate,
			SnapshotDate:   dateStr,
			CreatedAt:      now,
		})
	}
	if err := s.repo.UpsertSnapshots(ctx, snaps); err != nil {
		fmt.Printf("[quadrant] WARNING: failed to save ranking snapshots: %v\n", err)
		return
	}
	log.Printf("[quadrant] ranking snapshots saved: date=%s count=%d missing_close_price=%d", dateStr, len(snaps), missingPrices)
	s.repairRecentMissingSnapshotPricesBestEffort(ctx, selectedRecords, dateStr, priceHints)
}

func (s *Service) repairRecentMissingSnapshotPricesBestEffort(ctx context.Context, records []QuadrantScoreRecord, snapshotDate string, priceHints map[string]snapshotPriceHint) {
	if len(records) == 0 || strings.TrimSpace(snapshotDate) == "" {
		return
	}
	allowedCodes := make(map[string]bool, len(records))
	exchangeSet := map[string]bool{}
	for _, record := range records {
		allowedCodes[strings.TrimSpace(record.Code)] = true
		exchangeSet[strings.TrimSpace(record.Exchange)] = true
	}
	exchanges := make([]string, 0, len(exchangeSet))
	for exchange := range exchangeSet {
		if exchange != "" {
			exchanges = append(exchanges, exchange)
		}
	}
	fromDate := recentSnapshotRepairFromDate(snapshotDate)
	missing, err := s.repo.FindMissingSnapshotPrices(ctx, exchanges, fromDate, 500)
	if err != nil || len(missing) == 0 {
		if err != nil {
			log.Printf("[quadrant] ranking snapshot repair scan failed: %v", err)
		}
		return
	}

	updated := 0
	stillMissing := 0
	for _, snap := range missing {
		if !allowedCodes[strings.TrimSpace(snap.Code)] {
			continue
		}
		closePrice := 0.0
		priceTradeDate := ""
		if hint, ok := priceHints[snapshotPriceHintKey(snap.Code, snap.Exchange)]; ok {
			if hint.TradeDate == "" && snap.SnapshotDate == snapshotDate && hint.ClosePrice > 0 {
				closePrice = hint.ClosePrice
			} else {
				closePrice, priceTradeDate = priceHintUsableForSnapshot(hint, snap.SnapshotDate)
			}
		}
		if closePrice <= 0 && s.priceResolver != nil {
			closePrice = s.priceResolver(ctx, snap.Code, snap.Exchange, snap.SnapshotDate)
			if closePrice > 0 {
				priceTradeDate = snap.SnapshotDate
			}
		}
		if closePrice <= 0 {
			stillMissing++
			continue
		}
		if err := s.repo.UpdateSnapshotPrice(ctx, snap.ID, closePrice, priceTradeDate); err != nil {
			log.Printf("[quadrant] ranking snapshot repair update failed: id=%d code=%s err=%v", snap.ID, snap.Code, err)
			continue
		}
		updated++
	}
	if updated > 0 || stillMissing > 0 {
		log.Printf("[quadrant] ranking snapshot repair done: date=%s updated=%d still_missing=%d", snapshotDate, updated, stillMissing)
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

// GetRanking returns the market-specific ranking list.
// HKEX keeps the legacy opportunity-zone logic; ASHARE prefers ranking_score when available.
func (s *Service) GetRanking(ctx context.Context, exchange string, limit int) (*RankingResponse, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	displayExchange := strings.ToUpper(strings.TrimSpace(exchange))
	if displayExchange != "HKEX" {
		displayExchange = "ASHARE"
	}
	exchanges := resolveRankingExchanges(exchange)

	// Liquidity hard-filter thresholds (万元 / 万HKD)
	defaultMinAmount := 5000.0 // A-share default
	if displayExchange == "HKEX" {
		defaultMinAmount = 2000.0
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

	var records []QuadrantScoreRecord
	var totalInZone int64
	if displayExchange == "HKEX" {
		records, totalInZone, err = s.repo.FindOpportunityZone(ctx, exchanges, limit, minAmount)
		if err != nil {
			return nil, err
		}
	} else {
		var hasRankingScore bool
		records, totalInZone, hasRankingScore, err = s.repo.FindAShareRankingCandidates(ctx, limit, minAmount)
		if err != nil {
			return nil, err
		}
		if hasRankingScore {
			records = applyBoardCap(records, limit, 0.5)
		} else if len(records) > limit {
			records = records[:limit]
		}
	}

	items := make([]RankingItem, 0, len(records))
	var latestComputedAt time.Time

	for i, r := range records {
		item := RankingItem{
			Rank:             i + 1,
			Code:             r.Code,
			Name:             r.Name,
			Exchange:         r.Exchange,
			Opportunity:      r.Opportunity,
			Risk:             r.Risk,
			Quadrant:         r.Quadrant,
			Trend:            r.Trend,
			Flow:             r.Flow,
			Revision:         r.Revision,
			Liquidity:        r.Liquidity,
			AvgAmount5d:      r.AvgAmount5d,
			Board:            r.Board,
			RankingScore:     r.RankingScore,
			GlobalRankScore:  r.GlobalRankScore,
			BoardRankScore:   r.BoardRankScore,
			TradabilityScore: r.TradabilityScore,
		}
		if r.ComputedAt.After(latestComputedAt) {
			latestComputedAt = r.ComputedAt
		}

		// Enrich with consecutive days and return since first appearance
		days, _ := s.repo.GetConsecutiveDays(ctx, r.Code, exchanges)
		item.ConsecutiveDays = days

		firstDateStr, _ := s.repo.GetFirstAppearedDate(ctx, r.Code, exchanges)
		if firstDateStr != "" {
			startPrice, _, _ := s.repo.GetEarliestAvailableClosePrice(ctx, r.Code, exchanges, firstDateStr)
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
