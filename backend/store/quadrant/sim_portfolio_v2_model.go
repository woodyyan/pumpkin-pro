package quadrant

import "time"

const (
	SimPortfolioV2MarketAShare = "ASHARE"
	SimPortfolioV2MarketHKEX   = "HKEX"
)

const (
	SimPortfolioV2StageCalendar          = "calendar"
	SimPortfolioV2StageSignal            = "signal"
	SimPortfolioV2StageSelection         = "selection"
	SimPortfolioV2StagePriceRequirements = "price_requirements"
	SimPortfolioV2StageEntryOpen         = "entry_open"
	SimPortfolioV2StageValuationClose    = "valuation_close"
	SimPortfolioV2StageFacts             = "facts"
	SimPortfolioV2StageVerify            = "verify"
)

const (
	SimPortfolioV2StatusPending = "pending"
	SimPortfolioV2StatusRunning = "running"
	SimPortfolioV2StatusOK      = "ok"
	SimPortfolioV2StatusSkipped = "skipped"
	SimPortfolioV2StatusBlocked = "blocked"
	SimPortfolioV2StatusFailed  = "failed"
)

const (
	SimPortfolioV2PriceTypeEntryOpen      = "entry_open"
	SimPortfolioV2PriceTypeValuationClose = "valuation_close"
)

const (
	SimPortfolioV2PriceStatusPending   = "pending"
	SimPortfolioV2PriceStatusSatisfied = "satisfied"
	SimPortfolioV2PriceStatusMissing   = "missing"
	SimPortfolioV2PriceStatusSkipped   = "skipped"
	SimPortfolioV2PriceStatusFailed    = "failed"
)

const (
	SimPortfolioV2TriggerAdmin    = "admin"
	SimPortfolioV2TriggerSchedule = "schedule"
)

const (
	SimPortfolioV2PriceRepairRetryResolve     = "retry_resolve"
	SimPortfolioV2PriceRepairBackfillDailyBar = "backfill_daily_bars"
	SimPortfolioV2PriceRepairManualOverride   = "manual_override"
)

type MarketCalendar struct {
	ID            int64     `gorm:"primaryKey;autoIncrement"`
	Market        string    `gorm:"size:16;not null;uniqueIndex:uidx_market_calendar,priority:1;index:idx_market_calendar_market_date,priority:1"`
	TradeDate     string    `gorm:"size:10;not null;uniqueIndex:uidx_market_calendar,priority:2;index:idx_market_calendar_market_date,priority:2"`
	IsTradingDay  bool      `gorm:"not null;default:true"`
	PrevTradeDate string    `gorm:"size:10;not null;default:''"`
	NextTradeDate string    `gorm:"size:10;not null;default:''"`
	HolidayName   string    `gorm:"size:128;not null;default:''"`
	IsHalfDay     bool      `gorm:"not null;default:false"`
	Source        string    `gorm:"size:64;not null;default:'builtin'"`
	CreatedAt     time.Time `gorm:"not null"`
	UpdatedAt     time.Time `gorm:"not null"`
}

func (MarketCalendar) TableName() string { return "market_calendars" }

type SimPortfolioV2Definition struct {
	ID               string    `gorm:"primaryKey;size:64"`
	Code             string    `gorm:"size:64;not null;uniqueIndex"`
	Name             string    `gorm:"size:128;not null"`
	Market           string    `gorm:"size:16;not null;index"`
	PortfolioVariant string    `gorm:"size:8;not null;default:'A'"`
	MaxHoldings      int       `gorm:"not null;default:4"`
	SelectionRule    string    `gorm:"size:64;not null;default:'top4'"`
	SelectionWindow  int       `gorm:"not null;default:0"`
	ExcludedBoards   string    `gorm:"type:text;not null;default:'[]'"`
	WeightingMethod  string    `gorm:"size:32;not null;default:'equal'"`
	InitialAssets    float64   `gorm:"not null;default:1000000"`
	IsActive         bool      `gorm:"not null;default:true"`
	CreatedAt        time.Time `gorm:"not null"`
	UpdatedAt        time.Time `gorm:"not null"`
}

func (SimPortfolioV2Definition) TableName() string { return "sim_portfolio_v2_definitions" }

type SimPortfolioV2PipelineRun struct {
	ID          string    `gorm:"primaryKey;size:64"`
	TriggerType string    `gorm:"size:32;not null;index"`
	Operator    string    `gorm:"size:128;not null;default:''"`
	Market      string    `gorm:"size:16;not null;default:'';index"`
	FromDate    string    `gorm:"size:10;not null;default:''"`
	ToDate      string    `gorm:"size:10;not null;default:''"`
	StageScope  string    `gorm:"type:text;not null;default:'[]'"`
	Status      string    `gorm:"size:24;not null;index"`
	SummaryJSON string    `gorm:"type:text;not null;default:'{}'"`
	ErrorJSON   string    `gorm:"type:text;not null;default:'{}'"`
	StartedAt   time.Time `gorm:"not null;index"`
	FinishedAt  time.Time `gorm:"not null"`
	CreatedAt   time.Time `gorm:"not null"`
	UpdatedAt   time.Time `gorm:"not null"`
}

func (SimPortfolioV2PipelineRun) TableName() string { return "sim_portfolio_v2_pipeline_runs" }

type SimPortfolioV2PipelineDayStatus struct {
	ID            int64     `gorm:"primaryKey;autoIncrement"`
	Market        string    `gorm:"size:16;not null;uniqueIndex:uidx_spv2_day_stage,priority:1;index:idx_spv2_day_market_date,priority:1"`
	TradeDate     string    `gorm:"size:10;not null;uniqueIndex:uidx_spv2_day_stage,priority:2;index:idx_spv2_day_market_date,priority:2"`
	Stage         string    `gorm:"size:32;not null;uniqueIndex:uidx_spv2_day_stage,priority:3"`
	Status        string    `gorm:"size:24;not null;index"`
	ExpectedCount int       `gorm:"not null;default:0"`
	ActualCount   int       `gorm:"not null;default:0"`
	MissingCount  int       `gorm:"not null;default:0"`
	FailedCount   int       `gorm:"not null;default:0"`
	Message       string    `gorm:"type:text;not null;default:''"`
	ActionHint    string    `gorm:"type:text;not null;default:''"`
	RunID         string    `gorm:"size:64;not null;default:'';index"`
	CreatedAt     time.Time `gorm:"not null"`
	UpdatedAt     time.Time `gorm:"not null"`
}

func (SimPortfolioV2PipelineDayStatus) TableName() string {
	return "sim_portfolio_v2_pipeline_day_statuses"
}

type SimPortfolioV2SignalBatch struct {
	ID                  string    `gorm:"primaryKey;size:64"`
	Market              string    `gorm:"size:16;not null;uniqueIndex:uidx_spv2_signal_market_date,priority:1;index"`
	SourceTradeDate     string    `gorm:"size:10;not null;uniqueIndex:uidx_spv2_signal_market_date,priority:2;index"`
	ComputedAt          time.Time `gorm:"not null"`
	SourceQuadrantRunID string    `gorm:"size:64;not null;default:''"`
	Status              string    `gorm:"size:24;not null;index"`
	CandidateCount      int       `gorm:"not null;default:0"`
	SignalCount         int       `gorm:"not null;default:0"`
	MissingPriceCount   int       `gorm:"not null;default:0"`
	Message             string    `gorm:"type:text;not null;default:''"`
	CreatedAt           time.Time `gorm:"not null"`
	UpdatedAt           time.Time `gorm:"not null"`
}

func (SimPortfolioV2SignalBatch) TableName() string { return "sim_portfolio_v2_signal_batches" }

type SimPortfolioV2SignalItem struct {
	ID              int64     `gorm:"primaryKey;autoIncrement"`
	BatchID         string    `gorm:"size:64;not null;uniqueIndex:uidx_spv2_signal_item,priority:1;index"`
	Market          string    `gorm:"size:16;not null;index"`
	SourceTradeDate string    `gorm:"size:10;not null;index"`
	Code            string    `gorm:"size:16;not null;uniqueIndex:uidx_spv2_signal_item,priority:2"`
	Exchange        string    `gorm:"size:8;not null;uniqueIndex:uidx_spv2_signal_item,priority:3"`
	Name            string    `gorm:"size:128;not null;default:''"`
	Rank            int       `gorm:"not null;default:0"`
	Opportunity     float64   `gorm:"not null;default:0"`
	Risk            float64   `gorm:"not null;default:0"`
	ClosePrice      float64   `gorm:"not null;default:0"`
	PriceTradeDate  string    `gorm:"size:10;not null;default:''"`
	Board           string    `gorm:"size:16;not null;default:''"`
	CreatedAt       time.Time `gorm:"not null"`
}

func (SimPortfolioV2SignalItem) TableName() string { return "sim_portfolio_v2_signal_items" }

type SimPortfolioV2SelectionBatch struct {
	ID             string    `gorm:"primaryKey;size:64"`
	PortfolioID    string    `gorm:"size:64;not null;uniqueIndex:uidx_spv2_sel_port_signal,priority:1;index"`
	Market         string    `gorm:"size:16;not null;index"`
	SignalDate     string    `gorm:"size:10;not null;uniqueIndex:uidx_spv2_sel_port_signal,priority:2;index"`
	EntryTradeDate string    `gorm:"size:10;not null;default:''"`
	Status         string    `gorm:"size:24;not null;index"`
	SelectedCount  int       `gorm:"not null;default:0"`
	WarningText    string    `gorm:"type:text;not null;default:''"`
	CreatedAt      time.Time `gorm:"not null"`
	UpdatedAt      time.Time `gorm:"not null"`
}

func (SimPortfolioV2SelectionBatch) TableName() string { return "sim_portfolio_v2_selection_batches" }

type SimPortfolioV2SelectionItem struct {
	ID               int64     `gorm:"primaryKey;autoIncrement"`
	SelectionBatchID string    `gorm:"size:64;not null;uniqueIndex:uidx_spv2_sel_item,priority:1;index"`
	PortfolioID      string    `gorm:"size:64;not null;index"`
	SignalDate       string    `gorm:"size:10;not null;index"`
	EntryTradeDate   string    `gorm:"size:10;not null;default:''"`
	Code             string    `gorm:"size:16;not null;uniqueIndex:uidx_spv2_sel_item,priority:2"`
	Exchange         string    `gorm:"size:8;not null;uniqueIndex:uidx_spv2_sel_item,priority:3"`
	Name             string    `gorm:"size:128;not null;default:''"`
	Rank             int       `gorm:"not null;default:0"`
	SourceRank       int       `gorm:"not null;default:0"`
	Weight           float64   `gorm:"not null;default:0"`
	Board            string    `gorm:"size:16;not null;default:''"`
	CreatedAt        time.Time `gorm:"not null"`
}

func (SimPortfolioV2SelectionItem) TableName() string { return "sim_portfolio_v2_selection_items" }

type SimPortfolioV2PriceRequirement struct {
	ID             int64     `gorm:"primaryKey;autoIncrement"`
	PortfolioID    string    `gorm:"size:64;not null;uniqueIndex:uidx_spv2_price_req,priority:1;index"`
	Market         string    `gorm:"size:16;not null;index"`
	SignalDate     string    `gorm:"size:10;not null;uniqueIndex:uidx_spv2_price_req,priority:2;index"`
	TradeDate      string    `gorm:"size:10;not null;uniqueIndex:uidx_spv2_price_req,priority:3;index"`
	Code           string    `gorm:"size:16;not null;uniqueIndex:uidx_spv2_price_req,priority:4"`
	Exchange       string    `gorm:"size:8;not null;uniqueIndex:uidx_spv2_price_req,priority:5"`
	PriceType      string    `gorm:"size:32;not null;uniqueIndex:uidx_spv2_price_req,priority:6;index"`
	Required       bool      `gorm:"not null;default:true"`
	Status         string    `gorm:"size:24;not null;index"`
	Price          float64   `gorm:"not null;default:0"`
	PriceTradeDate string    `gorm:"size:10;not null;default:''"`
	Source         string    `gorm:"size:64;not null;default:''"`
	MissingReason  string    `gorm:"type:text;not null;default:''"`
	ResolvedAt     time.Time `gorm:"not null"`
	CreatedAt      time.Time `gorm:"not null"`
	UpdatedAt      time.Time `gorm:"not null"`
}

func (SimPortfolioV2PriceRequirement) TableName() string {
	return "sim_portfolio_v2_price_requirements"
}

type SimPortfolioV2PriceRepairAudit struct {
	ID           int64     `gorm:"primaryKey;autoIncrement"`
	Action       string    `gorm:"size:32;not null;index"`
	Market       string    `gorm:"size:16;not null;index"`
	PortfolioID  string    `gorm:"size:64;not null;default:'';index"`
	SignalDate   string    `gorm:"size:10;not null;default:'';index"`
	TradeDate    string    `gorm:"size:10;not null;default:'';index"`
	Code         string    `gorm:"size:16;not null;default:'';index"`
	Exchange     string    `gorm:"size:8;not null;default:''"`
	PriceType    string    `gorm:"size:32;not null;default:'';index"`
	Price        float64   `gorm:"not null;default:0"`
	Source       string    `gorm:"size:64;not null;default:''"`
	Reason       string    `gorm:"type:text;not null;default:''"`
	Evidence     string    `gorm:"type:text;not null;default:''"`
	Status       string    `gorm:"size:24;not null;index"`
	SummaryJSON  string    `gorm:"type:text;not null;default:'{}'"`
	ErrorMessage string    `gorm:"type:text;not null;default:''"`
	Operator     string    `gorm:"size:128;not null;default:'admin'"`
	CreatedAt    time.Time `gorm:"not null;index"`
	UpdatedAt    time.Time `gorm:"not null"`
}

func (SimPortfolioV2PriceRepairAudit) TableName() string {
	return "sim_portfolio_v2_price_repair_audits"
}

type SimPortfolioV2PriceOverride struct {
	ID        int64     `gorm:"primaryKey;autoIncrement"`
	Market    string    `gorm:"size:16;not null;uniqueIndex:uidx_spv2_price_override,priority:1;index"`
	Code      string    `gorm:"size:16;not null;uniqueIndex:uidx_spv2_price_override,priority:2"`
	Exchange  string    `gorm:"size:8;not null;uniqueIndex:uidx_spv2_price_override,priority:3"`
	TradeDate string    `gorm:"size:10;not null;uniqueIndex:uidx_spv2_price_override,priority:4;index"`
	PriceType string    `gorm:"size:32;not null;uniqueIndex:uidx_spv2_price_override,priority:5;index"`
	Price     float64   `gorm:"not null"`
	Reason    string    `gorm:"type:text;not null"`
	Evidence  string    `gorm:"type:text;not null"`
	Operator  string    `gorm:"size:128;not null;default:'admin'"`
	AuditID   int64     `gorm:"not null;default:0"`
	CreatedAt time.Time `gorm:"not null"`
	UpdatedAt time.Time `gorm:"not null"`
}

func (SimPortfolioV2PriceOverride) TableName() string {
	return "sim_portfolio_v2_price_overrides"
}

type SimPortfolioV2Daily struct {
	ID              int64     `gorm:"primaryKey;autoIncrement"`
	PortfolioID     string    `gorm:"size:64;not null;uniqueIndex:uidx_spv2_daily,priority:1;index"`
	Market          string    `gorm:"size:16;not null;index"`
	TradeDate       string    `gorm:"size:10;not null;uniqueIndex:uidx_spv2_daily,priority:2;index"`
	SignalDate      string    `gorm:"size:10;not null;default:''"`
	SourceTradeDate string    `gorm:"size:10;not null;default:''"`
	NAV             float64   `gorm:"not null;default:1"`
	TotalAssets     float64   `gorm:"not null;default:0"`
	PreviousAssets  float64   `gorm:"not null;default:0"`
	DailyReturn     float64   `gorm:"not null;default:0"`
	TotalReturn     float64   `gorm:"not null;default:0"`
	PositionCount   int       `gorm:"not null;default:0"`
	Rebalance       bool      `gorm:"not null;default:false"`
	Status          string    `gorm:"size:24;not null;default:'verified';index"`
	ComputedAt      time.Time `gorm:"not null"`
	CreatedAt       time.Time `gorm:"not null"`
	UpdatedAt       time.Time `gorm:"not null"`
}

func (SimPortfolioV2Daily) TableName() string { return "sim_portfolio_v2_daily" }

type SimPortfolioV2Position struct {
	ID          int64     `gorm:"primaryKey;autoIncrement"`
	PortfolioID string    `gorm:"size:64;not null;uniqueIndex:uidx_spv2_position,priority:1;index"`
	Market      string    `gorm:"size:16;not null;index"`
	TradeDate   string    `gorm:"size:10;not null;uniqueIndex:uidx_spv2_position,priority:2;index"`
	SignalDate  string    `gorm:"size:10;not null;default:''"`
	Code        string    `gorm:"size:16;not null;uniqueIndex:uidx_spv2_position,priority:3"`
	Exchange    string    `gorm:"size:8;not null;uniqueIndex:uidx_spv2_position,priority:4"`
	Name        string    `gorm:"size:128;not null;default:''"`
	Rank        int       `gorm:"not null;default:0"`
	Weight      float64   `gorm:"not null;default:0"`
	TargetValue float64   `gorm:"not null;default:0"`
	Shares      float64   `gorm:"not null;default:0"`
	BuyPrice    float64   `gorm:"not null;default:0"`
	ClosePrice  float64   `gorm:"not null;default:0"`
	MarketValue float64   `gorm:"not null;default:0"`
	Profit      float64   `gorm:"not null;default:0"`
	ProfitRate  float64   `gorm:"not null;default:0"`
	CreatedAt   time.Time `gorm:"not null"`
	UpdatedAt   time.Time `gorm:"not null"`
}

func (SimPortfolioV2Position) TableName() string { return "sim_portfolio_v2_positions" }

type SimPortfolioV2Trade struct {
	ID          int64     `gorm:"primaryKey;autoIncrement"`
	PortfolioID string    `gorm:"size:64;not null;uniqueIndex:uidx_spv2_trade,priority:1;index"`
	Market      string    `gorm:"size:16;not null;index"`
	TradeDate   string    `gorm:"size:10;not null;uniqueIndex:uidx_spv2_trade,priority:2;index"`
	SignalDate  string    `gorm:"size:10;not null;default:''"`
	Code        string    `gorm:"size:16;not null;uniqueIndex:uidx_spv2_trade,priority:3"`
	Exchange    string    `gorm:"size:8;not null;uniqueIndex:uidx_spv2_trade,priority:4"`
	Name        string    `gorm:"size:128;not null;default:''"`
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

func (SimPortfolioV2Trade) TableName() string { return "sim_portfolio_v2_trades" }

type SimPortfolioV2Metrics struct {
	ID           int64     `gorm:"primaryKey;autoIncrement"`
	PortfolioID  string    `gorm:"size:64;not null;uniqueIndex:uidx_spv2_metrics,priority:1;index"`
	Market       string    `gorm:"size:16;not null;index"`
	TradeDate    string    `gorm:"size:10;not null;uniqueIndex:uidx_spv2_metrics,priority:2;index"`
	NAV          float64   `gorm:"not null;default:1"`
	AnnualReturn float64   `gorm:"not null;default:0"`
	MaxDrawdown  float64   `gorm:"not null;default:0"`
	SharpeRatio  *float64  `gorm:"default:null"`
	Volatility   float64   `gorm:"not null;default:0"`
	WinRate      float64   `gorm:"not null;default:0"`
	TurnoverRate float64   `gorm:"not null;default:0"`
	CreatedAt    time.Time `gorm:"not null"`
	UpdatedAt    time.Time `gorm:"not null"`
}

func (SimPortfolioV2Metrics) TableName() string { return "sim_portfolio_v2_metrics" }

type SimPortfolioV2MarketConfig struct {
	ID                       int64     `gorm:"primaryKey;autoIncrement"`
	Market                   string    `gorm:"size:16;not null;uniqueIndex"`
	StartSignalDate          string    `gorm:"size:10;not null;default:''"`
	PublishedJobID           string    `gorm:"size:64;not null;default:''"`
	LatestPublishedTradeDate string    `gorm:"size:10;not null;default:''"`
	Status                   string    `gorm:"size:24;not null;default:'pending'"`
	UpdatedBy                string    `gorm:"size:128;not null;default:''"`
	CreatedAt                time.Time `gorm:"not null"`
	UpdatedAt                time.Time `gorm:"not null"`
}

func (SimPortfolioV2MarketConfig) TableName() string { return "sim_portfolio_v2_market_configs" }

type SimPortfolioV2Watermark struct {
	ID                      int64     `gorm:"primaryKey;autoIncrement"`
	PortfolioID             string    `gorm:"size:64;not null;uniqueIndex"`
	Market                  string    `gorm:"size:16;not null;index"`
	StartSignalDate         string    `gorm:"size:10;not null;default:''"`
	LatestSignalDate        string    `gorm:"size:10;not null;default:''"`
	LatestTradeDate         string    `gorm:"size:10;not null;default:''"`
	LatestVerifiedTradeDate string    `gorm:"size:10;not null;default:''"`
	Status                  string    `gorm:"size:24;not null;default:'pending'"`
	CreatedAt               time.Time `gorm:"not null"`
	UpdatedAt               time.Time `gorm:"not null"`
}

func (SimPortfolioV2Watermark) TableName() string { return "sim_portfolio_v2_watermarks" }
