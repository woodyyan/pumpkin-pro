package backtest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const maxRunsPerUser = 100

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// SaveRunAsync saves a backtest result asynchronously. Call from a goroutine.
func (s *Service) SaveRunAsync(userID string, request map[string]any, result map[string]any, durationMS int64, status string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := s.saveRun(ctx, userID, request, result, durationMS, status); err != nil {
		log.Printf("[backtest] save run failed: %v", err)
	}
}

func (s *Service) saveRun(ctx context.Context, userID string, request map[string]any, result map[string]any, durationMS int64, status string) error {
	dataSource := asString(request["data_source"])
	ticker := asString(request["ticker"])
	startDate := asString(request["start_date"])
	endDate := asString(request["end_date"])
	capital := asFloat64(request["capital"])
	feePct := asFloat64(request["fee_pct"])
	strategyID := asString(request["strategy_id"])
	strategyName := asString(request["strategy_name"])

	// Extract ticker info from result
	dataSummary := asMapNested(result, "data_summary")
	tickerName := asString(dataSummary["ticker_name"])
	tickerDisplay := asString(dataSummary["ticker_display"])
	if tickerDisplay == "" {
		tickerDisplay = ticker
	}

	// Also get strategy name from result if available
	resultStrategy := asMapNested(result, "strategy")
	if resultStrategyName := asString(resultStrategy["name"]); resultStrategyName != "" {
		strategyName = resultStrategyName
	}

	// Build title
	title := buildTitle(tickerDisplay, tickerName, strategyName, dataSource)

	// Build summary JSON (lightweight)
	summaryJSON, err := buildSummaryJSON(result)
	if err != nil {
		return fmt.Errorf("build summary json: %w", err)
	}

	// Build result JSON (full)
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal result json: %w", err)
	}

	record := &BacktestRunRecord{
		ID:           uuid.New().String(),
		UserID:       userID,
		Title:        title,
		DataSource:   dataSource,
		Ticker:       ticker,
		TickerName:   tickerName,
		StrategyID:   strategyID,
		StrategyName: strategyName,
		StartDate:    startDate,
		EndDate:      endDate,
		Capital:      capital,
		FeePct:       feePct,
		Status:       status,
		DurationMS:   durationMS,
		SummaryJSON:  summaryJSON,
		ResultJSON:   string(resultJSON),
		CreatedAt:    time.Now().UTC(),
	}

	if err := s.repo.Create(ctx, record); err != nil {
		return fmt.Errorf("create record: %w", err)
	}

	// Enforce per-user limit
	count, err := s.repo.CountByUser(ctx, userID)
	if err == nil && count > maxRunsPerUser {
		_ = s.repo.DeleteOldest(ctx, userID, maxRunsPerUser)
	}

	return nil
}

func (s *Service) List(ctx context.Context, userID string, limit, offset int) ([]RunListItem, int64, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	records, total, err := s.repo.ListByUser(ctx, userID, limit, offset)
	if err != nil {
		return nil, 0, err
	}

	items := make([]RunListItem, 0, len(records))
	for _, record := range records {
		items = append(items, record.toListItem())
	}

	return items, total, nil
}

func (s *Service) GetByID(ctx context.Context, userID, id string) (*RunDetail, error) {
	record, err := s.repo.GetByID(ctx, userID, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	detail := record.toDetail()
	return &detail, nil
}

func (s *Service) Delete(ctx context.Context, userID, id string) error {
	err := s.repo.Delete(ctx, userID, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

// ── helpers ──

func buildTitle(tickerDisplay, tickerName, strategyName, dataSource string) string {
	var parts []string

	switch {
	case tickerName != "" && tickerDisplay != "":
		parts = append(parts, fmt.Sprintf("%s (%s)", tickerName, tickerDisplay))
	case tickerDisplay != "":
		parts = append(parts, tickerDisplay)
	case dataSource == "csv":
		parts = append(parts, "CSV 数据")
	case dataSource == "sample":
		parts = append(parts, "示例行情")
	default:
		parts = append(parts, "回测")
	}

	if strategyName != "" {
		parts = append(parts, strategyName)
	}

	return strings.Join(parts, " · ")
}

func buildSummaryJSON(result map[string]any) (string, error) {
	metrics := asMapNested(result, "metrics")

	summary := map[string]any{
		"total_return_pct":  metrics["total_return_pct"],
		"annual_return_pct": metrics["annual_return_pct"],
		"max_drawdown_pct":  metrics["max_drawdown_pct"],
		"sharpe_ratio":      metrics["sharpe_ratio"],
		"total_trades":      metrics["total_trades"],
		"final_capital":     metrics["final_capital"],
		"win_rate_pct":      metrics["win_rate_pct"],
	}

	encoded, err := json.Marshal(summary)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func asString(v any) string {
	if v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return fmt.Sprintf("%v", v)
	}
	return strings.TrimSpace(s)
}

func asFloat64(v any) float64 {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case json.Number:
		f, _ := val.Float64()
		return f
	default:
		return 0
	}
}

func asMapNested(parent map[string]any, key string) map[string]any {
	if parent == nil {
		return map[string]any{}
	}
	v, ok := parent[key]
	if !ok || v == nil {
		return map[string]any{}
	}
	m, ok := v.(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return m
}
