package factorlab

import (
	"context"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const sqliteSafeBatchSize = 500

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) BulkUpsertSecurities(ctx context.Context, records []FactorSecurity) error {
	return bulkUpsert(ctx, r.db, cleanSecurities(records))
}

func (r *Repository) BulkUpsertDailyBars(ctx context.Context, records []FactorDailyBar) error {
	return bulkUpsert(ctx, r.db, cleanDailyBars(records))
}

func (r *Repository) BulkUpsertIndexDailyBars(ctx context.Context, records []FactorIndexDailyBar) error {
	return bulkUpsert(ctx, r.db, cleanIndexDailyBars(records))
}

func (r *Repository) BulkUpsertMarketMetrics(ctx context.Context, records []FactorMarketMetric) error {
	return bulkUpsert(ctx, r.db, cleanMarketMetrics(records))
}

func (r *Repository) BulkUpsertFinancialMetrics(ctx context.Context, records []FactorFinancialMetric) error {
	return bulkUpsert(ctx, r.db, cleanFinancialMetrics(records))
}

func (r *Repository) BulkUpsertDividendRecords(ctx context.Context, records []FactorDividendRecord) error {
	return bulkUpsert(ctx, r.db, cleanDividendRecords(records))
}

func (r *Repository) BulkUpsertSnapshots(ctx context.Context, records []FactorSnapshot) error {
	return bulkUpsert(ctx, r.db, cleanSnapshots(records))
}

func (r *Repository) CreateTaskRun(ctx context.Context, run FactorTaskRun) error {
	run.ID = strings.TrimSpace(run.ID)
	if run.ID == "" {
		return gorm.ErrInvalidData
	}
	run.TaskType = strings.TrimSpace(run.TaskType)
	if run.TaskType == "" {
		run.TaskType = TaskTypeBackfill
	}
	run.SnapshotDate = strings.TrimSpace(run.SnapshotDate)
	run.Status = strings.TrimSpace(run.Status)
	if run.Status == "" {
		run.Status = TaskStatusRunning
	}
	if run.StartedAt.IsZero() {
		run.StartedAt = time.Now().UTC()
	}
	if strings.TrimSpace(run.ParamsJSON) == "" {
		run.ParamsJSON = "{}"
	}
	if strings.TrimSpace(run.SummaryJSON) == "" {
		run.SummaryJSON = "{}"
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(&run).Error
}

func (r *Repository) FinishTaskRun(ctx context.Context, id, status string, total, success, failed, skipped int, summaryJSON, errorMessage string) error {
	finishedAt := time.Now().UTC()
	updates := map[string]any{
		"status":        normalizeTaskStatus(status),
		"finished_at":   &finishedAt,
		"total_count":   maxInt(total, 0),
		"success_count": maxInt(success, 0),
		"failed_count":  maxInt(failed, 0),
		"skipped_count": maxInt(skipped, 0),
		"summary_json":  defaultJSONString(summaryJSON),
		"error_message": strings.TrimSpace(errorMessage),
	}
	return r.db.WithContext(ctx).Model(&FactorTaskRun{}).Where("id = ?", strings.TrimSpace(id)).Updates(updates).Error
}

func (r *Repository) BulkUpsertTaskItems(ctx context.Context, records []FactorTaskItem) error {
	return bulkUpsert(ctx, r.db, cleanTaskItems(records))
}

func (r *Repository) LatestSuccessfulSnapshotDate(ctx context.Context) (string, error) {
	var date string
	err := r.db.WithContext(ctx).
		Model(&FactorTaskRun{}).
		Select("COALESCE(MAX(snapshot_date), '')").
		Where("task_type = ? AND status IN ? AND snapshot_date <> ''", TaskTypeDailyCompute, []string{TaskStatusSuccess, TaskStatusPartial}).
		Scan(&date).Error
	if err != nil {
		return "", err
	}
	return date, nil
}

func bulkUpsert[T any](ctx context.Context, db *gorm.DB, records []T) error {
	if len(records) == 0 {
		return nil
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for start := 0; start < len(records); start += sqliteSafeBatchSize {
			end := start + sqliteSafeBatchSize
			if end > len(records) {
				end = len(records)
			}
			batch := records[start:end]
			if err := tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&batch).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func cleanSecurities(records []FactorSecurity) []FactorSecurity {
	now := time.Now().UTC()
	out := make([]FactorSecurity, 0, len(records))
	for _, record := range records {
		record.Code = normalizeCode(record.Code)
		if record.Code == "" {
			continue
		}
		record.Symbol = strings.ToUpper(strings.TrimSpace(record.Symbol))
		if record.Symbol == "" {
			record.Symbol = buildSymbol(record.Code, record.Exchange)
		}
		record.Exchange = normalizeExchange(record.Exchange, record.Code)
		record.Board = normalizeBoard(record.Board, record.Code)
		record.Name = strings.TrimSpace(record.Name)
		record.ListingDate = strings.TrimSpace(record.ListingDate)
		record.Source = strings.TrimSpace(record.Source)
		if record.UpdatedAt.IsZero() {
			record.UpdatedAt = now
		}
		out = append(out, record)
	}
	return out
}

func cleanDailyBars(records []FactorDailyBar) []FactorDailyBar {
	now := time.Now().UTC()
	out := make([]FactorDailyBar, 0, len(records))
	for _, record := range records {
		record.Code = normalizeCode(record.Code)
		record.TradeDate = strings.TrimSpace(record.TradeDate)
		if record.Code == "" || record.TradeDate == "" {
			continue
		}
		if record.Adjusted == "" {
			record.Adjusted = "qfq"
		}
		record.Source = strings.TrimSpace(record.Source)
		if record.UpdatedAt.IsZero() {
			record.UpdatedAt = now
		}
		out = append(out, record)
	}
	return out
}

func cleanIndexDailyBars(records []FactorIndexDailyBar) []FactorIndexDailyBar {
	now := time.Now().UTC()
	out := make([]FactorIndexDailyBar, 0, len(records))
	for _, record := range records {
		record.IndexCode = strings.ToUpper(strings.TrimSpace(record.IndexCode))
		record.TradeDate = strings.TrimSpace(record.TradeDate)
		if record.IndexCode == "" || record.TradeDate == "" {
			continue
		}
		record.Source = strings.TrimSpace(record.Source)
		if record.UpdatedAt.IsZero() {
			record.UpdatedAt = now
		}
		out = append(out, record)
	}
	return out
}

func cleanMarketMetrics(records []FactorMarketMetric) []FactorMarketMetric {
	now := time.Now().UTC()
	out := make([]FactorMarketMetric, 0, len(records))
	for _, record := range records {
		record.Code = normalizeCode(record.Code)
		record.TradeDate = strings.TrimSpace(record.TradeDate)
		if record.Code == "" || record.TradeDate == "" {
			continue
		}
		record.Source = strings.TrimSpace(record.Source)
		if record.UpdatedAt.IsZero() {
			record.UpdatedAt = now
		}
		out = append(out, record)
	}
	return out
}

func cleanFinancialMetrics(records []FactorFinancialMetric) []FactorFinancialMetric {
	now := time.Now().UTC()
	out := make([]FactorFinancialMetric, 0, len(records))
	for _, record := range records {
		record.Code = normalizeCode(record.Code)
		record.ReportPeriod = strings.TrimSpace(record.ReportPeriod)
		if record.Code == "" || record.ReportPeriod == "" {
			continue
		}
		record.ReportDate = strings.TrimSpace(record.ReportDate)
		record.Source = strings.TrimSpace(record.Source)
		if record.UpdatedAt.IsZero() {
			record.UpdatedAt = now
		}
		out = append(out, record)
	}
	return out
}

func cleanDividendRecords(records []FactorDividendRecord) []FactorDividendRecord {
	now := time.Now().UTC()
	out := make([]FactorDividendRecord, 0, len(records))
	for _, record := range records {
		record.Code = normalizeCode(record.Code)
		record.ReportPeriod = strings.TrimSpace(record.ReportPeriod)
		record.ExDividendDate = strings.TrimSpace(record.ExDividendDate)
		if record.Code == "" || record.ReportPeriod == "" {
			continue
		}
		if record.ExDividendDate == "" {
			record.ExDividendDate = "unknown"
		}
		record.Source = strings.TrimSpace(record.Source)
		if record.UpdatedAt.IsZero() {
			record.UpdatedAt = now
		}
		out = append(out, record)
	}
	return out
}

func cleanSnapshots(records []FactorSnapshot) []FactorSnapshot {
	now := time.Now().UTC()
	out := make([]FactorSnapshot, 0, len(records))
	for _, record := range records {
		record.Code = normalizeCode(record.Code)
		record.SnapshotDate = strings.TrimSpace(record.SnapshotDate)
		if record.Code == "" || record.SnapshotDate == "" {
			continue
		}
		record.Symbol = strings.ToUpper(strings.TrimSpace(record.Symbol))
		if record.Symbol == "" {
			record.Symbol = buildSymbol(record.Code, "")
		}
		record.Board = normalizeBoard(record.Board, record.Code)
		if strings.TrimSpace(record.DataQualityFlags) == "" {
			record.DataQualityFlags = "[]"
		}
		if record.CreatedAt.IsZero() {
			record.CreatedAt = now
		}
		out = append(out, record)
	}
	return out
}

func cleanTaskItems(records []FactorTaskItem) []FactorTaskItem {
	now := time.Now().UTC()
	out := make([]FactorTaskItem, 0, len(records))
	for _, record := range records {
		record.RunID = strings.TrimSpace(record.RunID)
		record.ItemType = strings.TrimSpace(record.ItemType)
		record.ItemKey = strings.TrimSpace(record.ItemKey)
		if record.RunID == "" || record.ItemType == "" || record.ItemKey == "" {
			continue
		}
		record.Status = normalizeTaskStatus(record.Status)
		record.ErrorMessage = strings.TrimSpace(record.ErrorMessage)
		if record.UpdatedAt.IsZero() {
			record.UpdatedAt = now
		}
		out = append(out, record)
	}
	return out
}

func normalizeCode(code string) string {
	code = strings.TrimSpace(code)
	if code == "" {
		return ""
	}
	if idx := strings.Index(code, "."); idx > 0 {
		code = code[:idx]
	}
	for len(code) < 6 {
		code = "0" + code
	}
	return code
}

func normalizeExchange(exchange, code string) string {
	exchange = strings.ToUpper(strings.TrimSpace(exchange))
	if exchange == "SSE" || exchange == "SZSE" {
		return exchange
	}
	code = normalizeCode(code)
	if strings.HasPrefix(code, "6") {
		return "SSE"
	}
	return "SZSE"
}

func normalizeBoard(board, code string) string {
	board = strings.ToUpper(strings.TrimSpace(board))
	switch board {
	case BoardMain, BoardChiNext, BoardSTAR, BoardBJ, BoardOther:
		return board
	}
	code = normalizeCode(code)
	if strings.HasPrefix(code, "688") || strings.HasPrefix(code, "689") {
		return BoardSTAR
	}
	if strings.HasPrefix(code, "300") || strings.HasPrefix(code, "301") {
		return BoardChiNext
	}
	if strings.HasPrefix(code, "8") || strings.HasPrefix(code, "4") || strings.HasPrefix(code, "920") {
		return BoardBJ
	}
	if strings.HasPrefix(code, "6") || strings.HasPrefix(code, "0") || strings.HasPrefix(code, "002") || strings.HasPrefix(code, "003") {
		return BoardMain
	}
	return BoardOther
}

func buildSymbol(code, exchange string) string {
	code = normalizeCode(code)
	if code == "" {
		return ""
	}
	if normalizeExchange(exchange, code) == "SSE" {
		return code + ".SH"
	}
	return code + ".SZ"
}

func normalizeTaskStatus(status string) string {
	switch strings.TrimSpace(status) {
	case TaskStatusSuccess, TaskStatusPartial, TaskStatusFailed, TaskStatusRunning:
		return strings.TrimSpace(status)
	default:
		return TaskStatusRunning
	}
}

func defaultJSONString(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "{}"
	}
	return value
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
