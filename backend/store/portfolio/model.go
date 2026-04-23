package portfolio

import "time"

const (
	EventTypeInit          = "init"
	EventTypeBuy           = "buy"
	EventTypeSell          = "sell"
	EventTypeAdjustAvgCost = "adjust_avg_cost"
	EventTypeSyncPosition  = "sync_position"
)

const (
	CostMethodWeightedAvg = "weighted_avg"
)

const (
	CostSourceSystem = "system"
	CostSourceManual = "manual"
)

const (
	PortfolioScopeAll    = "ALL"
	PortfolioScopeAShare = "ASHARE"
	PortfolioScopeHK     = "HKEX"
)

const (
	EventSourceManual    = "manual"
	EventSourceMigration = "migration"
)

// ── DB Record ──

type PortfolioRecord struct {
	ID              string     `gorm:"primaryKey;size:36"`
	UserID          string     `gorm:"size:36;not null;index;uniqueIndex:idx_portfolio_user_symbol,priority:1"`
	Symbol          string     `gorm:"size:16;not null;uniqueIndex:idx_portfolio_user_symbol,priority:2"`
	Shares          float64    `gorm:"not null;default:0"`
	AvgCostPrice    float64    `gorm:"not null;default:0"`
	TotalCostAmount float64    `gorm:"not null;default:0"`
	BuyDate         string     `gorm:"size:10;not null;default:''"`
	Note            string     `gorm:"type:text;not null;default:''"`
	CostMethod      string     `gorm:"size:24;not null;default:'weighted_avg'"`
	CostSource      string     `gorm:"size:24;not null;default:'system'"`
	LastTradeAt     *time.Time `gorm:"index"`
	LastEventID     string     `gorm:"size:36;not null;default:'';index"`
	CreatedAt       time.Time  `gorm:"not null"`
	UpdatedAt       time.Time  `gorm:"not null"`
}

func (PortfolioRecord) TableName() string {
	return "user_portfolios"
}

type PortfolioEventRecord struct {
	ID                 string    `gorm:"primaryKey;size:36"`
	UserID             string    `gorm:"size:36;not null;index:idx_portfolio_event_user_symbol_date,priority:1"`
	Symbol             string    `gorm:"size:16;not null;index:idx_portfolio_event_user_symbol_date,priority:2"`
	EventType          string    `gorm:"size:32;not null;index"`
	TradeDate          string    `gorm:"size:10;not null;index:idx_portfolio_event_user_symbol_date,priority:3"`
	EffectiveAt        time.Time `gorm:"not null;index"`
	Quantity           float64   `gorm:"not null;default:0"`
	Price              float64   `gorm:"not null;default:0"`
	FeeAmount          float64   `gorm:"not null;default:0"`
	ManualAvgCostPrice float64   `gorm:"not null;default:0"`
	Note               string    `gorm:"type:text;not null;default:''"`
	Source             string    `gorm:"size:24;not null;default:'manual'"`
	IsVoided           bool      `gorm:"not null;default:false;index"`
	VoidedByEventID    string    `gorm:"size:36;not null;default:''"`
	BeforeShares       float64   `gorm:"not null;default:0"`
	BeforeAvgCostPrice float64   `gorm:"not null;default:0"`
	BeforeTotalCost    float64   `gorm:"not null;default:0"`
	AfterShares        float64   `gorm:"not null;default:0"`
	AfterAvgCostPrice  float64   `gorm:"not null;default:0"`
	AfterTotalCost     float64   `gorm:"not null;default:0"`
	RealizedPnlAmount  float64   `gorm:"not null;default:0"`
	RealizedPnlPct     float64   `gorm:"not null;default:0"`
	CreatedAt          time.Time `gorm:"not null"`
	UpdatedAt          time.Time `gorm:"not null"`
}

func (PortfolioEventRecord) TableName() string {
	return "user_portfolio_events"
}

type PortfolioDailySnapshotRecord struct {
	ID                  string    `gorm:"primaryKey;size:36"`
	UserID              string    `gorm:"index:idx_portfolio_daily_user_scope_date,priority:1;size:36;not null"`
	Scope               string    `gorm:"index:idx_portfolio_daily_user_scope_date,priority:2;size:16;not null"`
	SnapshotDate        string    `gorm:"index:idx_portfolio_daily_user_scope_date,priority:3;size:10;not null"`
	CurrencyCode        string    `gorm:"size:8;not null;default:''"`
	MarketValueAmount   float64   `gorm:"not null;default:0"`
	TotalCostAmount     float64   `gorm:"not null;default:0"`
	UnrealizedPnlAmount float64   `gorm:"not null;default:0"`
	RealizedPnlAmount   float64   `gorm:"not null;default:0"`
	TotalPnlAmount      float64   `gorm:"not null;default:0"`
	TodayPnlAmount      float64   `gorm:"not null;default:0"`
	PositionCount       int       `gorm:"not null;default:0"`
	CreatedAt           time.Time `gorm:"not null"`
	UpdatedAt           time.Time `gorm:"not null"`
}

func (PortfolioDailySnapshotRecord) TableName() string {
	return "user_portfolio_daily_snapshots"
}

// ── API Output ──

type PortfolioItem struct {
	Symbol          string  `json:"symbol"`
	Shares          float64 `json:"shares"`
	AvgCostPrice    float64 `json:"avg_cost_price"`
	TotalCostAmount float64 `json:"total_cost_amount"`
	BuyDate         string  `json:"buy_date"`
	Note            string  `json:"note"`
	CostMethod      string  `json:"cost_method"`
	CostSource      string  `json:"cost_source"`
	LastTradeAt     string  `json:"last_trade_at,omitempty"`
	LastEventID     string  `json:"last_event_id,omitempty"`
	UpdatedAt       string  `json:"updated_at"`
}

type PortfolioEventItem struct {
	ID                 string  `json:"id"`
	EventType          string  `json:"event_type"`
	TradeDate          string  `json:"trade_date"`
	EffectiveAt        string  `json:"effective_at"`
	Quantity           float64 `json:"quantity"`
	Price              float64 `json:"price"`
	FeeAmount          float64 `json:"fee_amount"`
	ManualAvgCostPrice float64 `json:"manual_avg_cost_price"`
	Note               string  `json:"note"`
	Source             string  `json:"source"`
	IsVoided           bool    `json:"is_voided"`
	BeforeShares       float64 `json:"before_shares"`
	BeforeAvgCostPrice float64 `json:"before_avg_cost_price"`
	BeforeTotalCost    float64 `json:"before_total_cost"`
	AfterShares        float64 `json:"after_shares"`
	AfterAvgCostPrice  float64 `json:"after_avg_cost_price"`
	AfterTotalCost     float64 `json:"after_total_cost"`
	RealizedPnlAmount  float64 `json:"realized_pnl_amount"`
	RealizedPnlPct     float64 `json:"realized_pnl_pct"`
}

type PortfolioDetail struct {
	Item           *PortfolioItem       `json:"item"`
	HistoryPreview []PortfolioEventItem `json:"history_preview"`
}

type UndoPortfolioEventResult struct {
	Item          *PortfolioItem `json:"item"`
	UndoneEventID string         `json:"undone_event_id"`
}

type PortfolioSummaryAmounts struct {
	CurrencyCode        string  `json:"currency_code"`
	CurrencySymbol      string  `json:"currency_symbol"`
	MarketValueAmount   float64 `json:"market_value_amount"`
	TotalCostAmount     float64 `json:"total_cost_amount"`
	UnrealizedPnlAmount float64 `json:"unrealized_pnl_amount"`
	RealizedPnlAmount   float64 `json:"realized_pnl_amount"`
	TotalPnlAmount      float64 `json:"total_pnl_amount"`
	TodayPnlAmount      float64 `json:"today_pnl_amount"`
}

type PortfolioMarketAmountBlock struct {
	Scope                  string  `json:"scope"`
	ScopeLabel             string  `json:"scope_label"`
	CurrencyCode           string  `json:"currency_code"`
	CurrencySymbol         string  `json:"currency_symbol"`
	MarketValueAmount      float64 `json:"market_value_amount"`
	TotalCostAmount        float64 `json:"total_cost_amount"`
	UnrealizedPnlAmount    float64 `json:"unrealized_pnl_amount"`
	RealizedPnlAmount      float64 `json:"realized_pnl_amount"`
	TotalPnlAmount         float64 `json:"total_pnl_amount"`
	TodayPnlAmount         float64 `json:"today_pnl_amount"`
	PositionCount          int     `json:"position_count"`
	MaxPositionWeightRatio float64 `json:"max_position_weight_ratio"`
}

type PortfolioDashboardSummary struct {
	Scope                  string                       `json:"scope"`
	MixedCurrency          bool                         `json:"mixed_currency"`
	PositionCount          int                          `json:"position_count"`
	ProfitPositionCount    int                          `json:"profit_position_count"`
	LossPositionCount      int                          `json:"loss_position_count"`
	MaxPositionWeightRatio float64                      `json:"max_position_weight_ratio"`
	TotalCapitalAmount     *float64                     `json:"total_capital_amount,omitempty"`
	CapitalUsageRatio      *float64                     `json:"capital_usage_ratio,omitempty"`
	LatestTradeAt          string                       `json:"latest_trade_at,omitempty"`
	Amounts                *PortfolioSummaryAmounts     `json:"amounts,omitempty"`
	AmountsByMarket        []PortfolioMarketAmountBlock `json:"amounts_by_market,omitempty"`
}

type PortfolioPositionItem struct {
	Symbol              string  `json:"symbol"`
	Name                string  `json:"name"`
	Exchange            string  `json:"exchange"`
	ExchangeLabel       string  `json:"exchange_label"`
	CurrencyCode        string  `json:"currency_code"`
	CurrencySymbol      string  `json:"currency_symbol"`
	Shares              float64 `json:"shares"`
	AvgCostPrice        float64 `json:"avg_cost_price"`
	TotalCostAmount     float64 `json:"total_cost_amount"`
	LastPrice           float64 `json:"last_price"`
	PrevClosePrice      float64 `json:"prev_close_price"`
	MarketValueAmount   float64 `json:"market_value_amount"`
	UnrealizedPnlAmount float64 `json:"unrealized_pnl_amount"`
	UnrealizedPnlPct    float64 `json:"unrealized_pnl_pct"`
	RealizedPnlAmount   float64 `json:"realized_pnl_amount"`
	TotalPnlAmount      float64 `json:"total_pnl_amount"`
	TotalPnlPct         float64 `json:"total_pnl_pct"`
	TodayPnlAmount      float64 `json:"today_pnl_amount"`
	TodayPnlPct         float64 `json:"today_pnl_pct"`
	PositionWeightRatio float64 `json:"position_weight_ratio"`
	BuyDate             string  `json:"buy_date,omitempty"`
	FirstTradeDate      string  `json:"first_trade_date,omitempty"`
	HoldingDays         int     `json:"holding_days"`
	LastTradeAt         string  `json:"last_trade_at,omitempty"`
	LastEventType       string  `json:"last_event_type,omitempty"`
	BuyCount            int     `json:"buy_count"`
	SellCount           int     `json:"sell_count"`
	AdjustCount         int     `json:"adjust_count"`
	CostSource          string  `json:"cost_source"`
	Note                string  `json:"note,omitempty"`
	LastEventID         string  `json:"last_event_id,omitempty"`
	DetailURL           string  `json:"detail_url"`
	CanBuy              bool    `json:"can_buy"`
	CanSell             bool    `json:"can_sell"`
	CanAdjustAvgCost    bool    `json:"can_adjust_avg_cost"`
}

type PortfolioRecentEventItem struct {
	ID                 string  `json:"id"`
	Symbol             string  `json:"symbol"`
	Name               string  `json:"name"`
	Exchange           string  `json:"exchange"`
	ExchangeLabel      string  `json:"exchange_label"`
	CurrencyCode       string  `json:"currency_code"`
	CurrencySymbol     string  `json:"currency_symbol"`
	EventType          string  `json:"event_type"`
	TradeDate          string  `json:"trade_date"`
	EffectiveAt        string  `json:"effective_at"`
	Quantity           float64 `json:"quantity"`
	Price              float64 `json:"price"`
	FeeAmount          float64 `json:"fee_amount"`
	RealizedPnlAmount  float64 `json:"realized_pnl_amount"`
	BeforeShares       float64 `json:"before_shares"`
	AfterShares        float64 `json:"after_shares"`
	BeforeAvgCostPrice float64 `json:"before_avg_cost_price"`
	AfterAvgCostPrice  float64 `json:"after_avg_cost_price"`
	Note               string  `json:"note"`
	IsUndoable         bool    `json:"is_undoable"`
}

type PortfolioAllocationItem struct {
	Symbol            string  `json:"symbol"`
	Name              string  `json:"name"`
	Exchange          string  `json:"exchange"`
	ExchangeLabel     string  `json:"exchange_label"`
	CurrencyCode      string  `json:"currency_code"`
	CurrencySymbol    string  `json:"currency_symbol"`
	MarketValueAmount float64 `json:"market_value_amount"`
	WeightRatio       float64 `json:"weight_ratio"`
	TotalPnlAmount    float64 `json:"total_pnl_amount"`
}

type PortfolioCurvePoint struct {
	Date                string  `json:"date"`
	Scope               string  `json:"scope"`
	CurrencyCode        string  `json:"currency_code"`
	MarketValueAmount   float64 `json:"market_value_amount"`
	TotalCostAmount     float64 `json:"total_cost_amount"`
	UnrealizedPnlAmount float64 `json:"unrealized_pnl_amount"`
	RealizedPnlAmount   float64 `json:"realized_pnl_amount"`
	TotalPnlAmount      float64 `json:"total_pnl_amount"`
	TodayPnlAmount      float64 `json:"today_pnl_amount"`
	PositionCount       int     `json:"position_count"`
}

type PortfolioCurveSeries struct {
	Scope        string                `json:"scope"`
	ScopeLabel   string                `json:"scope_label"`
	CurrencyCode string                `json:"currency_code"`
	Points       []PortfolioCurvePoint `json:"points"`
}

type PortfolioCurvePayload struct {
	Scope         string                 `json:"scope"`
	MixedCurrency bool                   `json:"mixed_currency"`
	Series        []PortfolioCurveSeries `json:"series"`
}

type PortfolioDashboardFilters struct {
	Scope      string `json:"scope"`
	SortBy     string `json:"sort_by"`
	SortOrder  string `json:"sort_order"`
	PnlFilter  string `json:"pnl_filter"`
	Keyword    string `json:"keyword"`
	CurveRange string `json:"curve_range"`
}

type PortfolioAIContextMeta struct {
	Ready                bool `json:"ready"`
	PositionContextCount int  `json:"position_context_count"`
	RecentEventCount     int  `json:"recent_event_count"`
	ProfileReady         bool `json:"profile_ready"`
}

type PortfolioDashboardPayload struct {
	Summary             PortfolioDashboardSummary  `json:"summary"`
	Positions           []PortfolioPositionItem    `json:"positions"`
	RecentEventsPreview []PortfolioRecentEventItem `json:"recent_events_preview"`
	AllocationPreview   []PortfolioAllocationItem  `json:"allocation_preview"`
	EquityCurvePreview  PortfolioCurvePayload      `json:"equity_curve_preview"`
	Filters             PortfolioDashboardFilters  `json:"filters"`
	AIContextMeta       PortfolioAIContextMeta     `json:"ai_context_meta"`
}

type PortfolioAIContextRiskFlags struct {
	PositionTooConcentrated  bool `json:"position_too_concentrated"`
	TooManyLosingPositions   bool `json:"too_many_losing_positions"`
	RecentOvertrading        bool `json:"recent_overtrading"`
	AveragingDownRisk        bool `json:"averaging_down_risk"`
	ManualCostAdjustFrequent bool `json:"manual_cost_adjust_frequent"`
	CapitalUsageTooHigh      bool `json:"capital_usage_too_high"`
}

type PortfolioAIContextPayload struct {
	Summary              PortfolioDashboardSummary    `json:"summary"`
	Positions            []PortfolioPositionItem      `json:"positions"`
	RecentEvents         []PortfolioRecentEventItem   `json:"recent_events"`
	Profile              *InvestmentProfile           `json:"profile"`
	RiskFlags            PortfolioAIContextRiskFlags  `json:"risk_flags"`
	MarketScopeBreakdown []PortfolioMarketAmountBlock `json:"market_scope_breakdown"`
}

// ── API Input ──

type UpsertPortfolioInput struct {
	Shares       float64 `json:"shares"`
	AvgCostPrice float64 `json:"avg_cost_price"`
	BuyDate      string  `json:"buy_date"`
	Note         string  `json:"note"`
}

type CreatePortfolioEventInput struct {
	EventType          string  `json:"event_type"`
	TradeDate          string  `json:"trade_date"`
	Quantity           float64 `json:"quantity"`
	Price              float64 `json:"price"`
	FeeAmount          float64 `json:"fee_amount"`
	ManualAvgCostPrice float64 `json:"avg_cost_price"`
	Note               string  `json:"note"`
}

type PortfolioDashboardQuery struct {
	Scope      string
	SortBy     string
	SortOrder  string
	PnlFilter  string
	Keyword    string
	CurveRange string
}

type PortfolioCurveQuery struct {
	Scope string
	Range string
}

type PortfolioRecentEventsQuery struct {
	Scope   string
	Keyword string
	Limit   int
	Offset  int
}

type PortfolioAllocationQuery struct {
	Scope   string
	Keyword string
	Limit   int
}

// ── Converters ──

func (r PortfolioRecord) toItem() PortfolioItem {
	item := PortfolioItem{
		Symbol:          r.Symbol,
		Shares:          r.Shares,
		AvgCostPrice:    r.AvgCostPrice,
		TotalCostAmount: r.TotalCostAmount,
		BuyDate:         r.BuyDate,
		Note:            r.Note,
		CostMethod:      r.CostMethod,
		CostSource:      r.CostSource,
		LastEventID:     r.LastEventID,
		UpdatedAt:       r.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if r.LastTradeAt != nil && !r.LastTradeAt.IsZero() {
		item.LastTradeAt = r.LastTradeAt.UTC().Format(time.RFC3339)
	}
	return item
}

func (r PortfolioEventRecord) toItem() PortfolioEventItem {
	return PortfolioEventItem{
		ID:                 r.ID,
		EventType:          r.EventType,
		TradeDate:          r.TradeDate,
		EffectiveAt:        r.EffectiveAt.UTC().Format(time.RFC3339),
		Quantity:           r.Quantity,
		Price:              r.Price,
		FeeAmount:          r.FeeAmount,
		ManualAvgCostPrice: r.ManualAvgCostPrice,
		Note:               r.Note,
		Source:             r.Source,
		IsVoided:           r.IsVoided,
		BeforeShares:       r.BeforeShares,
		BeforeAvgCostPrice: r.BeforeAvgCostPrice,
		BeforeTotalCost:    r.BeforeTotalCost,
		AfterShares:        r.AfterShares,
		AfterAvgCostPrice:  r.AfterAvgCostPrice,
		AfterTotalCost:     r.AfterTotalCost,
		RealizedPnlAmount:  r.RealizedPnlAmount,
		RealizedPnlPct:     r.RealizedPnlPct,
	}
}

// ── Investment Profile ──

type InvestmentProfileRecord struct {
	UserID                   string    `gorm:"primaryKey;size:36"`
	TotalCapital             float64   `gorm:"not null;default:0"`
	RiskPreference           string    `gorm:"size:32;not null;default:''"`
	InvestmentGoal           string    `gorm:"size:64;not null;default:''"`
	InvestmentHorizon        string    `gorm:"size:32;not null;default:''"`
	MaxDrawdownPct           float64   `gorm:"not null;default:0"`
	ExperienceLevel          string    `gorm:"size:32;not null;default:''"`
	DefaultFeeRateAShareBuy  float64   `gorm:"column:default_fee_rate_ashare_buy;not null;default:0"`
	DefaultFeeRateAShareSell float64   `gorm:"column:default_fee_rate_ashare_sell;not null;default:0"`
	DefaultFeeRateHKBuy      float64   `gorm:"column:default_fee_rate_hk_buy;not null;default:0"`
	DefaultFeeRateHKSell     float64   `gorm:"column:default_fee_rate_hk_sell;not null;default:0"`
	Note                     string    `gorm:"type:text;not null;default:''"`
	UpdatedAt                time.Time `gorm:"not null"`
}

func (InvestmentProfileRecord) TableName() string {
	return "user_investment_profiles"
}

type InvestmentProfile struct {
	TotalCapital             float64 `json:"total_capital"`
	RiskPreference           string  `json:"risk_preference"`
	InvestmentGoal           string  `json:"investment_goal"`
	InvestmentHorizon        string  `json:"investment_horizon"`
	MaxDrawdownPct           float64 `json:"max_drawdown_pct"`
	ExperienceLevel          string  `json:"experience_level"`
	DefaultFeeRateAShareBuy  float64 `json:"default_fee_rate_ashare_buy"`
	DefaultFeeRateAShareSell float64 `json:"default_fee_rate_ashare_sell"`
	DefaultFeeRateHKBuy      float64 `json:"default_fee_rate_hk_buy"`
	DefaultFeeRateHKSell     float64 `json:"default_fee_rate_hk_sell"`
	Note                     string  `json:"note"`
	UpdatedAt                string  `json:"updated_at"`
}

type UpsertInvestmentProfileInput struct {
	TotalCapital             float64 `json:"total_capital"`
	RiskPreference           string  `json:"risk_preference"`
	InvestmentGoal           string  `json:"investment_goal"`
	InvestmentHorizon        string  `json:"investment_horizon"`
	MaxDrawdownPct           float64 `json:"max_drawdown_pct"`
	ExperienceLevel          string  `json:"experience_level"`
	DefaultFeeRateAShareBuy  float64 `json:"default_fee_rate_ashare_buy"`
	DefaultFeeRateAShareSell float64 `json:"default_fee_rate_ashare_sell"`
	DefaultFeeRateHKBuy      float64 `json:"default_fee_rate_hk_buy"`
	DefaultFeeRateHKSell     float64 `json:"default_fee_rate_hk_sell"`
	Note                     string  `json:"note"`
}

func (r InvestmentProfileRecord) toProfile() InvestmentProfile {
	return InvestmentProfile{
		TotalCapital:             r.TotalCapital,
		RiskPreference:           r.RiskPreference,
		InvestmentGoal:           r.InvestmentGoal,
		InvestmentHorizon:        r.InvestmentHorizon,
		MaxDrawdownPct:           r.MaxDrawdownPct,
		ExperienceLevel:          r.ExperienceLevel,
		DefaultFeeRateAShareBuy:  r.DefaultFeeRateAShareBuy,
		DefaultFeeRateAShareSell: r.DefaultFeeRateAShareSell,
		DefaultFeeRateHKBuy:      r.DefaultFeeRateHKBuy,
		DefaultFeeRateHKSell:     r.DefaultFeeRateHKSell,
		Note:                     r.Note,
		UpdatedAt:                r.UpdatedAt.UTC().Format(time.RFC3339),
	}
}
