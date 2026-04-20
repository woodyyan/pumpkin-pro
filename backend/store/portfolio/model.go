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
	UserID            string    `gorm:"primaryKey;size:36"`
	TotalCapital      float64   `gorm:"not null;default:0"`
	RiskPreference    string    `gorm:"size:32;not null;default:''"`
	InvestmentGoal    string    `gorm:"size:64;not null;default:''"`
	InvestmentHorizon string    `gorm:"size:32;not null;default:''"`
	MaxDrawdownPct    float64   `gorm:"not null;default:0"`
	ExperienceLevel   string    `gorm:"size:32;not null;default:''"`
	Note              string    `gorm:"type:text;not null;default:''"`
	UpdatedAt         time.Time `gorm:"not null"`
}

func (InvestmentProfileRecord) TableName() string {
	return "user_investment_profiles"
}

type InvestmentProfile struct {
	TotalCapital      float64 `json:"total_capital"`
	RiskPreference    string  `json:"risk_preference"`
	InvestmentGoal    string  `json:"investment_goal"`
	InvestmentHorizon string  `json:"investment_horizon"`
	MaxDrawdownPct    float64 `json:"max_drawdown_pct"`
	ExperienceLevel   string  `json:"experience_level"`
	Note              string  `json:"note"`
	UpdatedAt         string  `json:"updated_at"`
}

type UpsertInvestmentProfileInput struct {
	TotalCapital      float64 `json:"total_capital"`
	RiskPreference    string  `json:"risk_preference"`
	InvestmentGoal    string  `json:"investment_goal"`
	InvestmentHorizon string  `json:"investment_horizon"`
	MaxDrawdownPct    float64 `json:"max_drawdown_pct"`
	ExperienceLevel   string  `json:"experience_level"`
	Note              string  `json:"note"`
}

func (r InvestmentProfileRecord) toProfile() InvestmentProfile {
	return InvestmentProfile{
		TotalCapital:      r.TotalCapital,
		RiskPreference:    r.RiskPreference,
		InvestmentGoal:    r.InvestmentGoal,
		InvestmentHorizon: r.InvestmentHorizon,
		MaxDrawdownPct:    r.MaxDrawdownPct,
		ExperienceLevel:   r.ExperienceLevel,
		Note:              r.Note,
		UpdatedAt:         r.UpdatedAt.UTC().Format(time.RFC3339),
	}
}
