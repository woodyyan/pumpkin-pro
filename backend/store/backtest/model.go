package backtest

import (
	"encoding/json"
	"time"
)

// ── DB Record ──

type BacktestRunRecord struct {
	ID           string    `gorm:"primaryKey;size:36"`
	UserID       string    `gorm:"size:36;not null;index:idx_backtest_runs_user_created,priority:1"`
	Title        string    `gorm:"size:255;not null"`
	DataSource   string    `gorm:"size:32;not null"`
	Ticker       string    `gorm:"size:32;not null;default:''"`
	TickerName   string    `gorm:"size:128;not null;default:''"`
	StrategyID   string    `gorm:"size:128;not null;default:''"`
	StrategyName string    `gorm:"size:255;not null;default:''"`
	StartDate    string    `gorm:"size:10;not null"`
	EndDate      string    `gorm:"size:10;not null"`
	Capital      float64   `gorm:"not null;default:0"`
	FeePct       float64   `gorm:"not null;default:0"`
	Status       string    `gorm:"size:16;not null;default:'success'"`
	DurationMS   int64     `gorm:"not null;default:0"`
	SummaryJSON  string    `gorm:"type:text;not null"`
	ResultJSON   string    `gorm:"type:text;not null"`
	CreatedAt    time.Time `gorm:"not null;index:idx_backtest_runs_user_created,priority:2,sort:desc"`
}

func (BacktestRunRecord) TableName() string {
	return "backtest_runs"
}

// ── API Output: list item (lightweight) ──

type RunListItem struct {
	ID             string         `json:"id"`
	Title          string         `json:"title"`
	DataSource     string         `json:"data_source"`
	Ticker         string         `json:"ticker"`
	TickerName     string         `json:"ticker_name"`
	StrategyName   string         `json:"strategy_name"`
	StartDate      string         `json:"start_date"`
	EndDate        string         `json:"end_date"`
	Capital        float64        `json:"capital"`
	Status         string         `json:"status"`
	DurationMS     int64          `json:"duration_ms"`
	CreatedAt      string         `json:"created_at"`
	MetricsSummary map[string]any `json:"metrics_summary"`
}

// ── API Output: full detail ──

type RunDetail struct {
	ID           string         `json:"id"`
	Title        string         `json:"title"`
	DataSource   string         `json:"data_source"`
	Ticker       string         `json:"ticker"`
	TickerName   string         `json:"ticker_name"`
	StrategyID   string         `json:"strategy_id"`
	StrategyName string         `json:"strategy_name"`
	StartDate    string         `json:"start_date"`
	EndDate      string         `json:"end_date"`
	Capital      float64        `json:"capital"`
	FeePct       float64        `json:"fee_pct"`
	Status       string         `json:"status"`
	DurationMS   int64          `json:"duration_ms"`
	CreatedAt    string         `json:"created_at"`
	Result       map[string]any `json:"result"`
}

func (r BacktestRunRecord) toListItem() RunListItem {
	var summary map[string]any
	if r.SummaryJSON != "" {
		_ = json.Unmarshal([]byte(r.SummaryJSON), &summary)
	}
	if summary == nil {
		summary = map[string]any{}
	}

	return RunListItem{
		ID:             r.ID,
		Title:          r.Title,
		DataSource:     r.DataSource,
		Ticker:         r.Ticker,
		TickerName:     r.TickerName,
		StrategyName:   r.StrategyName,
		StartDate:      r.StartDate,
		EndDate:        r.EndDate,
		Capital:        r.Capital,
		Status:         r.Status,
		DurationMS:     r.DurationMS,
		CreatedAt:      r.CreatedAt.UTC().Format(time.RFC3339),
		MetricsSummary: summary,
	}
}

func (r BacktestRunRecord) toDetail() RunDetail {
	var result map[string]any
	if r.ResultJSON != "" {
		_ = json.Unmarshal([]byte(r.ResultJSON), &result)
	}
	if result == nil {
		result = map[string]any{}
	}

	return RunDetail{
		ID:           r.ID,
		Title:        r.Title,
		DataSource:   r.DataSource,
		Ticker:       r.Ticker,
		TickerName:   r.TickerName,
		StrategyID:   r.StrategyID,
		StrategyName: r.StrategyName,
		StartDate:    r.StartDate,
		EndDate:      r.EndDate,
		Capital:      r.Capital,
		FeePct:       r.FeePct,
		Status:       r.Status,
		DurationMS:   r.DurationMS,
		CreatedAt:    r.CreatedAt.UTC().Format(time.RFC3339),
		Result:       result,
	}
}
