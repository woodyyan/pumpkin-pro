package factorlab

import "time"

const (
	TaskTypeBackfill     = "backfill"
	TaskTypeDailyCompute = "daily_compute"

	TaskStatusRunning = "running"
	TaskStatusSuccess = "success"
	TaskStatusPartial = "partial"
	TaskStatusFailed  = "failed"

	BoardMain    = "MAIN"
	BoardChiNext = "CHINEXT"
	BoardSTAR    = "STAR"
	BoardBJ      = "BJ"
	BoardOther   = "OTHER"
)

type FactorSecurity struct {
	Code        string    `gorm:"primaryKey;size:16" json:"code"`
	Symbol      string    `gorm:"size:20;not null;default:'';index" json:"symbol"`
	Name        string    `gorm:"size:128;not null;default:''" json:"name"`
	Exchange    string    `gorm:"size:8;not null;default:'';index" json:"exchange"`
	Board       string    `gorm:"size:16;not null;default:'';index" json:"board"`
	ListingDate string    `gorm:"size:10;not null;default:'';index" json:"listing_date"`
	IsST        bool      `gorm:"not null;default:false;index" json:"is_st"`
	IsActive    bool      `gorm:"not null;default:true;index" json:"is_active"`
	Source      string    `gorm:"size:64;not null;default:''" json:"source"`
	UpdatedAt   time.Time `gorm:"not null" json:"updated_at"`
}

func (FactorSecurity) TableName() string { return "factor_securities" }

type FactorDailyBar struct {
	Code         string    `gorm:"primaryKey;size:16" json:"code"`
	TradeDate    string    `gorm:"primaryKey;size:10" json:"trade_date"`
	Open         float64   `gorm:"not null;default:0" json:"open"`
	Close        float64   `gorm:"not null;default:0" json:"close"`
	High         float64   `gorm:"not null;default:0" json:"high"`
	Low          float64   `gorm:"not null;default:0" json:"low"`
	Volume       float64   `gorm:"not null;default:0" json:"volume"`
	Amount       float64   `gorm:"not null;default:0" json:"amount"`
	TurnoverRate *float64  `json:"turnover_rate"`
	Adjusted     string    `gorm:"size:16;not null;default:'qfq'" json:"adjusted"`
	Source       string    `gorm:"size:64;not null;default:''" json:"source"`
	UpdatedAt    time.Time `gorm:"not null" json:"updated_at"`
}

func (FactorDailyBar) TableName() string { return "factor_daily_bars" }

type FactorIndexDailyBar struct {
	IndexCode string    `gorm:"primaryKey;size:16" json:"index_code"`
	TradeDate string    `gorm:"primaryKey;size:10" json:"trade_date"`
	Close     float64   `gorm:"not null;default:0" json:"close"`
	PctChange *float64  `json:"pct_change"`
	Source    string    `gorm:"size:64;not null;default:''" json:"source"`
	UpdatedAt time.Time `gorm:"not null" json:"updated_at"`
}

func (FactorIndexDailyBar) TableName() string { return "factor_index_daily_bars" }

type FactorMarketMetric struct {
	Code         string    `gorm:"primaryKey;size:16" json:"code"`
	TradeDate    string    `gorm:"primaryKey;size:10" json:"trade_date"`
	ClosePrice   float64   `gorm:"not null;default:0" json:"close_price"`
	MarketCap    *float64  `json:"market_cap"`
	PE           *float64  `gorm:"column:pe" json:"pe"`
	PB           *float64  `gorm:"column:pb" json:"pb"`
	Volume       float64   `gorm:"not null;default:0" json:"volume"`
	Amount       float64   `gorm:"not null;default:0" json:"amount"`
	TurnoverRate *float64  `json:"turnover_rate"`
	IsSuspended  bool      `gorm:"not null;default:false;index" json:"is_suspended"`
	Source       string    `gorm:"size:64;not null;default:''" json:"source"`
	UpdatedAt    time.Time `gorm:"not null" json:"updated_at"`
}

func (FactorMarketMetric) TableName() string { return "factor_market_metrics" }

type FactorFinancialMetric struct {
	Code              string    `gorm:"primaryKey;size:16" json:"code"`
	ReportPeriod      string    `gorm:"primaryKey;size:10" json:"report_period"`
	ReportDate        string    `gorm:"size:10;not null;default:'';index" json:"report_date"`
	Revenue           *float64  `json:"revenue"`
	RevenueYOY        *float64  `gorm:"column:revenue_yoy" json:"revenue_yoy"`
	NetProfit         *float64  `json:"net_profit"`
	NetProfitYOY      *float64  `gorm:"column:net_profit_yoy" json:"net_profit_yoy"`
	TotalAssets       *float64  `json:"total_assets"`
	TotalEquity       *float64  `json:"total_equity"`
	OperatingCashFlow *float64  `json:"operating_cash_flow"`
	Source            string    `gorm:"size:64;not null;default:''" json:"source"`
	UpdatedAt         time.Time `gorm:"not null" json:"updated_at"`
}

func (FactorFinancialMetric) TableName() string { return "factor_financial_metrics" }

type FactorDividendRecord struct {
	Code                 string    `gorm:"primaryKey;size:16" json:"code"`
	ReportPeriod         string    `gorm:"primaryKey;size:10" json:"report_period"`
	ExDividendDate       string    `gorm:"primaryKey;size:10" json:"ex_dividend_date"`
	CashDividendPerShare *float64  `json:"cash_dividend_per_share"`
	TotalCashDividend    *float64  `json:"total_cash_dividend"`
	Source               string    `gorm:"size:64;not null;default:''" json:"source"`
	UpdatedAt            time.Time `gorm:"not null" json:"updated_at"`
}

func (FactorDividendRecord) TableName() string { return "factor_dividend_records" }

type FactorSnapshot struct {
	SnapshotDate            string    `gorm:"primaryKey;size:10;index" json:"snapshot_date"`
	Code                    string    `gorm:"primaryKey;size:16" json:"code"`
	Symbol                  string    `gorm:"size:20;not null;default:'';index" json:"symbol"`
	Name                    string    `gorm:"size:128;not null;default:''" json:"name"`
	Board                   string    `gorm:"size:16;not null;default:'';index" json:"board"`
	ListingAgeDays          *int      `json:"listing_age_days"`
	IsNewStock              bool      `gorm:"not null;default:false;index" json:"is_new_stock"`
	AvailableTradingDays    int       `gorm:"not null;default:0" json:"available_trading_days"`
	ClosePrice              float64   `gorm:"not null;default:0" json:"close_price"`
	MarketCap               *float64  `json:"market_cap"`
	PE                      *float64  `gorm:"column:pe" json:"pe"`
	PB                      *float64  `gorm:"column:pb" json:"pb"`
	PS                      *float64  `gorm:"column:ps" json:"ps"`
	DividendYield           *float64  `json:"dividend_yield"`
	EarningGrowth           *float64  `json:"earning_growth"`
	RevenueGrowth           *float64  `json:"revenue_growth"`
	Performance1Y           *float64  `gorm:"column:performance_1y" json:"performance_1y"`
	PerformanceSinceListing *float64  `json:"performance_since_listing"`
	Momentum1M              *float64  `gorm:"column:momentum_1m" json:"momentum_1m"`
	ROE                     *float64  `gorm:"column:roe" json:"roe"`
	OperatingCFMargin       *float64  `gorm:"column:operating_cf_margin" json:"operating_cf_margin"`
	AssetToEquity           *float64  `json:"asset_to_equity"`
	Volatility1M            *float64  `gorm:"column:volatility_1m" json:"volatility_1m"`
	Beta1Y                  *float64  `gorm:"column:beta_1y" json:"beta_1y"`
	DataQualityFlags        string    `gorm:"type:text;not null;default:'[]'" json:"data_quality_flags"`
	CreatedAt               time.Time `gorm:"not null" json:"created_at"`
}

func (FactorSnapshot) TableName() string { return "factor_snapshots" }

type FactorTaskRun struct {
	ID           string     `gorm:"primaryKey;size:36" json:"id"`
	TaskType     string     `gorm:"size:32;not null;index" json:"task_type"`
	SnapshotDate string     `gorm:"size:10;not null;default:'';index" json:"snapshot_date"`
	Status       string     `gorm:"size:16;not null;index" json:"status"`
	StartedAt    time.Time  `gorm:"not null;index" json:"started_at"`
	FinishedAt   *time.Time `json:"finished_at"`
	TotalCount   int        `gorm:"not null;default:0" json:"total_count"`
	SuccessCount int        `gorm:"not null;default:0" json:"success_count"`
	FailedCount  int        `gorm:"not null;default:0" json:"failed_count"`
	SkippedCount int        `gorm:"not null;default:0" json:"skipped_count"`
	ParamsJSON   string     `gorm:"type:text;not null;default:'{}'" json:"params_json"`
	SummaryJSON  string     `gorm:"type:text;not null;default:'{}'" json:"summary_json"`
	ErrorMessage string     `gorm:"type:text;not null;default:''" json:"error_message"`
}

func (FactorTaskRun) TableName() string { return "factor_task_runs" }

type FactorTaskItem struct {
	RunID        string    `gorm:"primaryKey;size:36;index" json:"run_id"`
	ItemType     string    `gorm:"primaryKey;size:32" json:"item_type"`
	ItemKey      string    `gorm:"primaryKey;size:64" json:"item_key"`
	Status       string    `gorm:"size:16;not null;index" json:"status"`
	ErrorMessage string    `gorm:"type:text;not null;default:''" json:"error_message"`
	UpdatedAt    time.Time `gorm:"not null" json:"updated_at"`
}

func (FactorTaskItem) TableName() string { return "factor_task_items" }
