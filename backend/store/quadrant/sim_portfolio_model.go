package quadrant

import "time"

const (
	simPortfolioInitialAssets  = 1000000.0
	simPortfolioTargetWeight   = 0.25
	simPortfolioNavPrecision   = 6
	simPortfolioStatusSeeded   = "seeded"
	simPortfolioStatusComplete = "completed"
	simPortfolioStatusPending  = "pending"
	simPortfolioStatusFailed   = "failed"
)

const (
	simPortfolioActionBuy  = "BUY"
	simPortfolioActionSell = "SELL"
	simPortfolioActionHold = "HOLD"
)

const (
	simPortfolioReasonEnterTop4 = "enter_top4"
	simPortfolioReasonDropTop4  = "drop_out_top4"
	simPortfolioReasonStayTop4  = "stay_top4"
)

type SimPortfolioDaily struct {
	ID              int64     `gorm:"primaryKey;autoIncrement"`
	PortfolioID     string    `gorm:"size:64;not null;uniqueIndex:uidx_sim_portfolio_daily,priority:1;index:idx_sim_portfolio_daily_trade,priority:1"`
	TradeDate       string    `gorm:"size:10;not null;uniqueIndex:uidx_sim_portfolio_daily,priority:2;index:idx_sim_portfolio_daily_trade,priority:2"`
	SignalDate      string    `gorm:"size:10;not null;default:''"`
	SourceTradeDate string    `gorm:"size:10;not null;default:''"`
	NAV             float64   `gorm:"not null;default:1"`
	TotalAssets     float64   `gorm:"not null;default:0"`
	PreviousAssets  float64   `gorm:"not null;default:0"`
	DailyReturn     float64   `gorm:"not null;default:0"`
	TotalReturn     float64   `gorm:"not null;default:0"`
	PositionCount   int       `gorm:"not null;default:0"`
	Rebalance       bool      `gorm:"not null;default:false"`
	Status          string    `gorm:"size:24;not null;default:'completed'"`
	WarningText     string    `gorm:"type:text;not null;default:''"`
	ComputedAt      time.Time `gorm:"not null"`
	CreatedAt       time.Time `gorm:"not null"`
	UpdatedAt       time.Time `gorm:"not null"`
}

func (SimPortfolioDaily) TableName() string { return "portfolio_daily" }

type SimPortfolioPosition struct {
	ID              int64     `gorm:"primaryKey;autoIncrement"`
	PortfolioID     string    `gorm:"size:64;not null;uniqueIndex:uidx_sim_portfolio_position,priority:1;index:idx_sim_portfolio_position_trade,priority:1"`
	TradeDate       string    `gorm:"size:10;not null;uniqueIndex:uidx_sim_portfolio_position,priority:2;index:idx_sim_portfolio_position_trade,priority:2"`
	SignalDate      string    `gorm:"size:10;not null;default:''"`
	StockCode       string    `gorm:"size:16;not null;uniqueIndex:uidx_sim_portfolio_position,priority:3"`
	StockName       string    `gorm:"size:128;not null;default:''"`
	Exchange        string    `gorm:"size:8;not null;default:'';uniqueIndex:uidx_sim_portfolio_position,priority:4"`
	Rank            int       `gorm:"not null;default:0"`
	Weight          float64   `gorm:"not null;default:0"`
	TargetValue     float64   `gorm:"not null;default:0"`
	Shares          float64   `gorm:"not null;default:0"`
	BuyPrice        float64   `gorm:"not null;default:0"`
	ClosePrice      float64   `gorm:"not null;default:0"`
	MarketValue     float64   `gorm:"not null;default:0"`
	Profit          float64   `gorm:"not null;default:0"`
	ProfitRate      float64   `gorm:"not null;default:0"`
	SourceTradeDate string    `gorm:"size:10;not null;default:''"`
	CreatedAt       time.Time `gorm:"not null"`
	UpdatedAt       time.Time `gorm:"not null"`
}

func (SimPortfolioPosition) TableName() string { return "portfolio_position" }

type SimPortfolioTrade struct {
	ID          int64     `gorm:"primaryKey;autoIncrement"`
	PortfolioID string    `gorm:"size:64;not null;uniqueIndex:uidx_sim_portfolio_trade,priority:1;index:idx_sim_portfolio_trade_trade,priority:1"`
	TradeDate   string    `gorm:"size:10;not null;uniqueIndex:uidx_sim_portfolio_trade,priority:2;index:idx_sim_portfolio_trade_trade,priority:2"`
	SignalDate  string    `gorm:"size:10;not null;default:''"`
	StockCode   string    `gorm:"size:16;not null;uniqueIndex:uidx_sim_portfolio_trade,priority:3"`
	StockName   string    `gorm:"size:128;not null;default:''"`
	Exchange    string    `gorm:"size:8;not null;default:'';uniqueIndex:uidx_sim_portfolio_trade,priority:4"`
	Action      string    `gorm:"size:8;not null;default:''"`
	OldWeight   float64   `gorm:"not null;default:0"`
	NewWeight   float64   `gorm:"not null;default:0"`
	TradePrice  float64   `gorm:"not null;default:0"`
	TargetValue float64   `gorm:"not null;default:0"`
	OldShares   float64   `gorm:"not null;default:0"`
	NewShares   float64   `gorm:"not null;default:0"`
	ShareDelta  float64   `gorm:"not null;default:0"`
	Reason      string    `gorm:"size:32;not null;default:''"`
	CreatedAt   time.Time `gorm:"not null"`
	UpdatedAt   time.Time `gorm:"not null"`
}

func (SimPortfolioTrade) TableName() string { return "portfolio_trade" }

type SimPortfolioMetrics struct {
	ID              int64     `gorm:"primaryKey;autoIncrement"`
	PortfolioID     string    `gorm:"size:64;not null;uniqueIndex:uidx_sim_portfolio_metrics,priority:1;index:idx_sim_portfolio_metrics_trade,priority:1"`
	TradeDate       string    `gorm:"size:10;not null;uniqueIndex:uidx_sim_portfolio_metrics,priority:2;index:idx_sim_portfolio_metrics_trade,priority:2"`
	NAV             float64   `gorm:"not null;default:1"`
	AnnualReturn    float64   `gorm:"not null;default:0"`
	MaxDrawdown     float64   `gorm:"not null;default:0"`
	SharpeRatio     *float64  `gorm:"default:null"`
	Volatility      float64   `gorm:"not null;default:0"`
	WinRate         float64   `gorm:"not null;default:0"`
	TurnoverRate    float64   `gorm:"not null;default:0"`
	BenchmarkReturn *float64  `gorm:"default:null"`
	ExcessReturn    *float64  `gorm:"default:null"`
	CreatedAt       time.Time `gorm:"not null"`
	UpdatedAt       time.Time `gorm:"not null"`
}

func (SimPortfolioMetrics) TableName() string { return "portfolio_metrics" }

const (
	simPortfolioTrackingStartConfigKey  = "global_start_signal_date"
	simPortfolioTrackingJobApply        = "apply_start_date"
	simPortfolioTrackingJobStatusOK     = "success"
	simPortfolioTrackingJobStatusFailed = "failed"
	simPortfolioTrackingJobStatusReject = "rejected"
)

type SimPortfolioTrackingConfig struct {
	ID               int64     `gorm:"primaryKey;autoIncrement"`
	ConfigKey        string    `gorm:"size:64;not null;uniqueIndex"`
	StartSignalDate  string    `gorm:"size:10;not null;default:''"`
	LatestApplyJobID string    `gorm:"size:64;not null;default:''"`
	Status           string    `gorm:"size:24;not null;default:'active'"`
	AppliedBy        string    `gorm:"size:128;not null;default:''"`
	AppliedAt        time.Time `gorm:"not null"`
	Note             string    `gorm:"type:text;not null;default:''"`
	CreatedAt        time.Time `gorm:"not null"`
	UpdatedAt        time.Time `gorm:"not null"`
}

func (SimPortfolioTrackingConfig) TableName() string { return "sim_portfolio_tracking_config" }

type SimPortfolioTrackingJob struct {
	ID                       string    `gorm:"primaryKey;size:64"`
	JobType                  string    `gorm:"size:32;not null;index"`
	RequestedStartSignalDate string    `gorm:"size:10;not null;default:''"`
	EffectiveStartSignalDate string    `gorm:"size:10;not null;default:''"`
	Status                   string    `gorm:"size:24;not null;index"`
	Message                  string    `gorm:"type:text;not null;default:''"`
	RequestedBy              string    `gorm:"size:128;not null;default:''"`
	StartedAt                time.Time `gorm:"not null;index"`
	FinishedAt               time.Time `gorm:"not null"`
	PreviewJSON              string    `gorm:"type:text;not null;default:'{}'"`
	ResultJSON               string    `gorm:"type:text;not null;default:'{}'"`
	ErrorText                string    `gorm:"type:text;not null;default:''"`
	CreatedAt                time.Time `gorm:"not null"`
	UpdatedAt                time.Time `gorm:"not null"`
}

func (SimPortfolioTrackingJob) TableName() string { return "sim_portfolio_tracking_jobs" }

type SimPortfolioOverviewResponse struct {
	AsOfTradeDate string                     `json:"as_of_trade_date,omitempty"`
	Items         []SimPortfolioOverviewItem `json:"items"`
}

type SimPortfolioOverviewItem struct {
	PortfolioID        string                     `json:"portfolio_id"`
	Code               string                     `json:"code"`
	Name               string                     `json:"name"`
	Exchange           string                     `json:"exchange"`
	PortfolioVariant   string                     `json:"portfolio_variant"`
	SelectionRule      string                     `json:"selection_rule"`
	SelectionWindow    int                        `json:"selection_window,omitempty"`
	InitialAssets      float64                    `json:"initial_assets"`
	PositionCount      int                        `json:"position_count"`
	LatestTradeDate    string                     `json:"latest_trade_date,omitempty"`
	LatestSignalDate   string                     `json:"latest_signal_date,omitempty"`
	PendingSignalDate  string                     `json:"pending_signal_date,omitempty"`
	NextEntryTradeDate string                     `json:"next_entry_trade_date,omitempty"`
	Status             string                     `json:"status"`
	StatusText         string                     `json:"status_text,omitempty"`
	NAV                float64                    `json:"nav"`
	TotalAssets        float64                    `json:"total_assets"`
	DailyReturn        float64                    `json:"daily_return"`
	TotalReturn        float64                    `json:"total_return"`
	MaxDrawdown        float64                    `json:"max_drawdown"`
	Volatility         float64                    `json:"volatility"`
	WinRate            float64                    `json:"win_rate"`
	TurnoverRate       float64                    `json:"turnover_rate"`
	CurrentPositions   []SimPortfolioPositionItem `json:"current_positions"`
	LatestTrades       []SimPortfolioTradeItem    `json:"latest_trades"`
}

type SimPortfolioDailyResponse struct {
	PortfolioID string                  `json:"portfolio_id"`
	Items       []SimPortfolioDailyItem `json:"items"`
}

type SimPortfolioDailyItem struct {
	TradeDate       string  `json:"trade_date"`
	SignalDate      string  `json:"signal_date,omitempty"`
	SourceTradeDate string  `json:"source_trade_date,omitempty"`
	NAV             float64 `json:"nav"`
	TotalAssets     float64 `json:"total_assets"`
	DailyReturn     float64 `json:"daily_return"`
	TotalReturn     float64 `json:"total_return"`
	PositionCount   int     `json:"position_count"`
	Rebalance       bool    `json:"rebalance"`
	Status          string  `json:"status"`
	WarningText     string  `json:"warning_text,omitempty"`
}

type SimPortfolioPositionsResponse struct {
	PortfolioID string                     `json:"portfolio_id"`
	TradeDate   string                     `json:"trade_date,omitempty"`
	TotalAssets float64                    `json:"total_assets"`
	Items       []SimPortfolioPositionItem `json:"items"`
}

type SimPortfolioPositionItem struct {
	Rank            int     `json:"rank"`
	StockCode       string  `json:"stock_code"`
	StockName       string  `json:"stock_name"`
	Exchange        string  `json:"exchange"`
	Weight          float64 `json:"weight"`
	TargetValue     float64 `json:"target_value"`
	Shares          float64 `json:"shares"`
	BuyPrice        float64 `json:"buy_price"`
	ClosePrice      float64 `json:"close_price"`
	MarketValue     float64 `json:"market_value"`
	Profit          float64 `json:"profit"`
	ProfitRate      float64 `json:"profit_rate"`
	SourceTradeDate string  `json:"source_trade_date,omitempty"`
}

type SimPortfolioTradesResponse struct {
	PortfolioID string                  `json:"portfolio_id"`
	Items       []SimPortfolioTradeItem `json:"items"`
}

type SimPortfolioTradeItem struct {
	TradeDate   string  `json:"trade_date"`
	SignalDate  string  `json:"signal_date,omitempty"`
	StockCode   string  `json:"stock_code"`
	StockName   string  `json:"stock_name"`
	Exchange    string  `json:"exchange"`
	Action      string  `json:"action"`
	OldWeight   float64 `json:"old_weight"`
	NewWeight   float64 `json:"new_weight"`
	TradePrice  float64 `json:"trade_price"`
	TargetValue float64 `json:"target_value"`
	OldShares   float64 `json:"old_shares"`
	NewShares   float64 `json:"new_shares"`
	ShareDelta  float64 `json:"share_delta"`
	Reason      string  `json:"reason"`
}

type SimPortfolioMetricsResponse struct {
	PortfolioID     string   `json:"portfolio_id"`
	TradeDate       string   `json:"trade_date,omitempty"`
	NAV             float64  `json:"nav"`
	AnnualReturn    float64  `json:"annual_return"`
	MaxDrawdown     float64  `json:"max_drawdown"`
	SharpeRatio     *float64 `json:"sharpe_ratio,omitempty"`
	Volatility      float64  `json:"volatility"`
	WinRate         float64  `json:"win_rate"`
	TurnoverRate    float64  `json:"turnover_rate"`
	BenchmarkReturn *float64 `json:"benchmark_return,omitempty"`
	ExcessReturn    *float64 `json:"excess_return,omitempty"`
}

type SimPortfolioAdminStatusResponse struct {
	Items []SimPortfolioAdminStatusItem `json:"items"`
}

type SimPortfolioAdminStatusItem struct {
	PortfolioID            string `json:"portfolio_id"`
	Name                   string `json:"name"`
	Exchange               string `json:"exchange"`
	LatestTradeDate        string `json:"latest_trade_date,omitempty"`
	LatestSignalDate       string `json:"latest_signal_date,omitempty"`
	PendingSignalDate      string `json:"pending_signal_date,omitempty"`
	NextEntryTradeDate     string `json:"next_entry_trade_date,omitempty"`
	Status                 string `json:"status"`
	StatusText             string `json:"status_text,omitempty"`
	ActionHint             string `json:"action_hint,omitempty"`
	DailyRowCount          int    `json:"daily_row_count"`
	CompletedDailyCount    int    `json:"completed_daily_count"`
	PositionRowCount       int    `json:"position_row_count"`
	TradeRowCount          int    `json:"trade_row_count"`
	MetricsRowCount        int    `json:"metrics_row_count"`
	BaselineOnly           bool   `json:"baseline_only"`
	CanSync                bool   `json:"can_sync"`
	NextSyncSignalDate     string `json:"next_sync_signal_date,omitempty"`
	NextSyncTradeDate      string `json:"next_sync_trade_date,omitempty"`
	MissingOpenPriceCount  int    `json:"missing_open_price_count"`
	MissingClosePriceCount int    `json:"missing_close_price_count"`
}

type SimPortfolioSyncResponse struct {
	OK      bool                      `json:"ok"`
	Message string                    `json:"message,omitempty"`
	Items   []SimPortfolioSyncSummary `json:"items"`
}

type SimPortfolioSyncSummary struct {
	PortfolioID            string `json:"portfolio_id"`
	Name                   string `json:"name"`
	Exchange               string `json:"exchange"`
	Status                 string `json:"status"`
	Message                string `json:"message,omitempty"`
	AnchorDate             string `json:"anchor_date,omitempty"`
	LatestSignalDate       string `json:"latest_signal_date,omitempty"`
	GeneratedDailyCount    int    `json:"generated_daily_count"`
	LastGeneratedTradeDate string `json:"last_generated_trade_date,omitempty"`
	MissingOpenPriceCount  int    `json:"missing_open_price_count"`
	MissingClosePriceCount int    `json:"missing_close_price_count"`
}

type SimPortfolioVerifyResponse struct {
	Items []SimPortfolioVerifyItem `json:"items"`
}

type SimPortfolioVerifyItem struct {
	PortfolioID    string  `json:"portfolio_id"`
	TradeDate      string  `json:"trade_date"`
	Status         string  `json:"status"`
	PositionCount  int     `json:"position_count"`
	TotalAssets    float64 `json:"total_assets"`
	PositionAssets float64 `json:"position_assets"`
	Difference     float64 `json:"difference"`
	Message        string  `json:"message,omitempty"`
}

type SimPortfolioBackfillOpenPriceResponse struct {
	OK                 bool                          `json:"ok"`
	Message            string                        `json:"message,omitempty"`
	Summary            SimPortfolioBackfillSummary   `json:"summary"`
	PortfolioSummaries []SimPortfolioBackfillSummary `json:"portfolios"`
}

type SimPortfolioBackfillSummary struct {
	ScannedCount           int `json:"scanned_count"`
	FilledCount            int `json:"filled_count"`
	StillPendingCount      int `json:"still_pending_count"`
	FailedCount            int `json:"failed_count"`
	SkippedBeforeCutover   int `json:"skipped_before_cutover"`
	MissingMarketPriceRows int `json:"missing_market_price_rows"`
}

type SimPortfolioTrackingStartStatusResponse struct {
	OK                     bool                            `json:"ok"`
	CurrentStartSignalDate string                          `json:"current_start_signal_date,omitempty"`
	AppliedAt              string                          `json:"applied_at,omitempty"`
	AppliedBy              string                          `json:"applied_by,omitempty"`
	Status                 string                          `json:"status,omitempty"`
	Note                   string                          `json:"note,omitempty"`
	LatestJob              *SimPortfolioTrackingJobSummary `json:"latest_job,omitempty"`
}

type SimPortfolioTrackingJobSummary struct {
	JobID      string `json:"job_id"`
	Status     string `json:"status"`
	Message    string `json:"message,omitempty"`
	StartedAt  string `json:"started_at,omitempty"`
	FinishedAt string `json:"finished_at,omitempty"`
}

type SimPortfolioTrackingStartPreviewResponse struct {
	OK               bool                                        `json:"ok"`
	StartSignalDate  string                                      `json:"start_signal_date"`
	CanApply         bool                                        `json:"can_apply"`
	Severity         string                                      `json:"severity"`
	Message          string                                      `json:"message,omitempty"`
	LatestSignalDate string                                      `json:"latest_signal_date,omitempty"`
	Markets          []SimPortfolioTrackingStartMarketPreview    `json:"markets"`
	Portfolios       []SimPortfolioTrackingStartPortfolioPreview `json:"portfolios"`
	BlockingReasons  []SimPortfolioTrackingStartReason           `json:"blocking_reasons"`
	Warnings         []SimPortfolioTrackingStartReason           `json:"warnings"`
}

type SimPortfolioTrackingStartMarketPreview struct {
	Exchange              string `json:"exchange"`
	Label                 string `json:"label"`
	HasSnapshot           bool   `json:"has_snapshot"`
	StartSignalDate       string `json:"start_signal_date"`
	NextEntryTradeDate    string `json:"next_entry_trade_date,omitempty"`
	LatestSignalDate      string `json:"latest_signal_date,omitempty"`
	SnapshotCountToLatest int    `json:"snapshot_count_to_latest"`
	Status                string `json:"status"`
	Message               string `json:"message,omitempty"`
}

type SimPortfolioTrackingStartPortfolioPreview struct {
	PortfolioID             string `json:"portfolio_id"`
	Name                    string `json:"name"`
	Exchange                string `json:"exchange"`
	Status                  string `json:"status"`
	SelectedCount           int    `json:"selected_count"`
	RequiredCount           int    `json:"required_count"`
	MissingOpenPriceCount   int    `json:"missing_open_price_count"`
	MissingClosePriceCount  int    `json:"missing_close_price_count"`
	FirstEntryTradeDate     string `json:"first_entry_trade_date,omitempty"`
	FirstValuationTradeDate string `json:"first_valuation_trade_date,omitempty"`
	Message                 string `json:"message,omitempty"`
}

type SimPortfolioTrackingStartReason struct {
	Scope       string `json:"scope"`
	Exchange    string `json:"exchange,omitempty"`
	PortfolioID string `json:"portfolio_id,omitempty"`
	Code        string `json:"code"`
	Message     string `json:"message"`
}

type SimPortfolioTrackingStartApplyResponse struct {
	OK              bool                                      `json:"ok"`
	JobID           string                                    `json:"job_id,omitempty"`
	StartSignalDate string                                    `json:"start_signal_date"`
	Message         string                                    `json:"message,omitempty"`
	Summary         SimPortfolioTrackingStartApplySummary     `json:"summary"`
	Portfolios      []SimPortfolioTrackingStartApplyPortfolio `json:"portfolios"`
	Verify          SimPortfolioTrackingStartVerifySummary    `json:"verify"`
}

type SimPortfolioTrackingStartApplySummary struct {
	PortfolioCount   int `json:"portfolio_count"`
	DailyRowCount    int `json:"daily_row_count"`
	PositionRowCount int `json:"position_row_count"`
	TradeRowCount    int `json:"trade_row_count"`
	MetricsRowCount  int `json:"metrics_row_count"`
}

type SimPortfolioTrackingStartApplyPortfolio struct {
	PortfolioID         string `json:"portfolio_id"`
	Name                string `json:"name"`
	Exchange            string `json:"exchange"`
	Status              string `json:"status"`
	StartSignalDate     string `json:"start_signal_date"`
	FirstTradeDate      string `json:"first_trade_date,omitempty"`
	LatestTradeDate     string `json:"latest_trade_date,omitempty"`
	GeneratedDailyCount int    `json:"generated_daily_count"`
	Message             string `json:"message,omitempty"`
}

type SimPortfolioTrackingStartVerifySummary struct {
	Status     string `json:"status"`
	IssueCount int    `json:"issue_count"`
}
