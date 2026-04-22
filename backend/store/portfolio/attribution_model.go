package portfolio

import "time"

const (
	AttributionRange7D     = "7D"
	AttributionRange30D    = "30D"
	AttributionRange90D    = "90D"
	AttributionRangeAll    = "ALL"
	AttributionRangeCustom = "CUSTOM"
)

// ── DB Records ──

type SecurityProfileRecord struct {
	Symbol        string    `gorm:"primaryKey;size:16"`
	Exchange      string    `gorm:"size:8;not null;default:'';index"`
	Name          string    `gorm:"size:128;not null;default:''"`
	SectorCode    string    `gorm:"size:32;not null;default:'';index"`
	SectorName    string    `gorm:"size:64;not null;default:'';index"`
	BenchmarkCode string    `gorm:"size:16;not null;default:''"`
	Source        string    `gorm:"size:24;not null;default:'system'"`
	CreatedAt     time.Time `gorm:"not null"`
	UpdatedAt     time.Time `gorm:"not null"`
}

func (SecurityProfileRecord) TableName() string {
	return "security_profiles"
}

type PortfolioPositionDailySnapshotRecord struct {
	ID                  string    `gorm:"primaryKey;size:36"`
	UserID              string    `gorm:"size:36;not null;index:idx_ppds_user_date_symbol,priority:1"`
	SnapshotDate        string    `gorm:"size:10;not null;index:idx_ppds_user_date_symbol,priority:2"`
	Symbol              string    `gorm:"size:16;not null;index:idx_ppds_user_date_symbol,priority:3"`
	Exchange            string    `gorm:"size:8;not null;default:'';index"`
	CurrencyCode        string    `gorm:"size:8;not null;default:''"`
	CurrencySymbol      string    `gorm:"size:8;not null;default:''"`
	Name                string    `gorm:"size:128;not null;default:''"`
	Shares              float64   `gorm:"not null;default:0"`
	AvgCostPrice        float64   `gorm:"not null;default:0"`
	TotalCostAmount     float64   `gorm:"not null;default:0"`
	ClosePrice          float64   `gorm:"not null;default:0"`
	PrevClosePrice      float64   `gorm:"not null;default:0"`
	MarketValueAmount   float64   `gorm:"not null;default:0"`
	UnrealizedPnlAmount float64   `gorm:"not null;default:0"`
	RealizedPnlCum      float64   `gorm:"not null;default:0"`
	PositionWeightRatio float64   `gorm:"not null;default:0"`
	SectorCode          string    `gorm:"size:32;not null;default:'';index"`
	SectorName          string    `gorm:"size:64;not null;default:'';index"`
	BenchmarkCode       string    `gorm:"size:16;not null;default:''"`
	CreatedAt           time.Time `gorm:"not null"`
	UpdatedAt           time.Time `gorm:"not null"`
}

func (PortfolioPositionDailySnapshotRecord) TableName() string {
	return "user_portfolio_position_daily_snapshots"
}

// ── Query ──

type PortfolioAttributionQuery struct {
	Scope               string
	Range               string
	StartDate           string
	EndDate             string
	Limit               int
	SortBy              string
	Refresh             bool
	IncludeUnclassified bool
	TimelineLimit       int
}

// ── Shared output ──

type PortfolioAttributionMeta struct {
	Scope         string `json:"scope"`
	Range         string `json:"range"`
	StartDate     string `json:"start_date"`
	EndDate       string `json:"end_date"`
	ComputedAt    string `json:"computed_at"`
	MixedCurrency bool   `json:"mixed_currency"`
	HasData       bool   `json:"has_data"`
	EmptyReason   string `json:"empty_reason"`
}

type PortfolioAttributionInsight struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Severity    string `json:"severity"`
}

type PortfolioAttributionSummaryCard struct {
	Key            string   `json:"key"`
	Label          string   `json:"label"`
	ValueAmount    *float64 `json:"value_amount,omitempty"`
	ValueRatio     *float64 `json:"value_ratio,omitempty"`
	CurrencyCode   string   `json:"currency_code"`
	CurrencySymbol string   `json:"currency_symbol"`
	Tone           string   `json:"tone"`
	Tooltip        string   `json:"tooltip"`
}

type PortfolioAttributionWaterfallItem struct {
	Key          string   `json:"key"`
	Label        string   `json:"label"`
	Type         string   `json:"type"`
	Amount       float64  `json:"amount"`
	Ratio        *float64 `json:"ratio,omitempty"`
	DisplayOrder int      `json:"display_order"`
	Tooltip      string   `json:"tooltip"`
}

type PortfolioAttributionWaterfallGroup struct {
	Scope          string                             `json:"scope"`
	ScopeLabel     string                             `json:"scope_label"`
	CurrencyCode   string                             `json:"currency_code"`
	CurrencySymbol string                             `json:"currency_symbol"`
	Items          []PortfolioAttributionWaterfallItem `json:"items"`
}

type PortfolioAttributionMarketBlock struct {
	Scope                     string  `json:"scope"`
	ScopeLabel                string  `json:"scope_label"`
	CurrencyCode              string  `json:"currency_code"`
	CurrencySymbol            string  `json:"currency_symbol"`
	StartMarketValueAmount    float64 `json:"start_market_value_amount"`
	EndMarketValueAmount      float64 `json:"end_market_value_amount"`
	RealizedPnlAmount         float64 `json:"realized_pnl_amount"`
	UnrealizedPnlChangeAmount float64 `json:"unrealized_pnl_change_amount"`
	FeeAmount                 float64 `json:"fee_amount"`
	TradingAlphaAmount        float64 `json:"trading_alpha_amount"`
	MarketContributionAmount  float64 `json:"market_contribution_amount"`
	ExcessContributionAmount  float64 `json:"excess_contribution_amount"`
	TotalPnlAmount            float64 `json:"total_pnl_amount"`
	TotalReturnPct            float64 `json:"total_return_pct"`
}

type PortfolioAttributionSummaryPayload struct {
	PortfolioAttributionMeta
	Headline        string                              `json:"headline"`
	SummaryCards    []PortfolioAttributionSummaryCard   `json:"summary_cards"`
	WaterfallGroups []PortfolioAttributionWaterfallGroup `json:"waterfall_groups"`
	MarketBlocks    []PortfolioAttributionMarketBlock   `json:"market_blocks"`
	Insights        []PortfolioAttributionInsight       `json:"insights"`
}

type PortfolioAttributionStockItem struct {
	Symbol                    string  `json:"symbol"`
	Name                      string  `json:"name"`
	Exchange                  string  `json:"exchange"`
	ExchangeLabel             string  `json:"exchange_label"`
	SectorCode                string  `json:"sector_code"`
	SectorName                string  `json:"sector_name"`
	StartWeightRatio          float64 `json:"start_weight_ratio"`
	EndWeightRatio            float64 `json:"end_weight_ratio"`
	AvgWeightRatio            float64 `json:"avg_weight_ratio"`
	RealizedPnlAmount         float64 `json:"realized_pnl_amount"`
	UnrealizedPnlChangeAmount float64 `json:"unrealized_pnl_change_amount"`
	TotalPnlAmount            float64 `json:"total_pnl_amount"`
	ContributionRatio         float64 `json:"contribution_ratio"`
	HoldingReturnPct          float64 `json:"holding_return_pct"`
	BuyCount                  int     `json:"buy_count"`
	SellCount                 int     `json:"sell_count"`
	HoldingDays               int     `json:"holding_days"`
	DriverTag                 string  `json:"driver_tag"`
	DriverLabel               string  `json:"driver_label"`
	DetailURL                 string  `json:"detail_url"`
}

type PortfolioAttributionStockGroup struct {
	Scope          string                        `json:"scope"`
	ScopeLabel     string                        `json:"scope_label"`
	CurrencyCode   string                        `json:"currency_code"`
	CurrencySymbol string                        `json:"currency_symbol"`
	Items          []PortfolioAttributionStockItem `json:"items"`
}

type PortfolioAttributionStocksPayload struct {
	PortfolioAttributionMeta
	PositiveGroups []PortfolioAttributionStockGroup `json:"positive_groups"`
	NegativeGroups []PortfolioAttributionStockGroup `json:"negative_groups"`
	NetGroups      []PortfolioAttributionStockGroup `json:"net_groups"`
}

type PortfolioAttributionSectorItem struct {
	SectorCode                string  `json:"sector_code"`
	SectorName                string  `json:"sector_name"`
	StockCount                int     `json:"stock_count"`
	StartWeightRatio          float64 `json:"start_weight_ratio"`
	EndWeightRatio            float64 `json:"end_weight_ratio"`
	AvgWeightRatio            float64 `json:"avg_weight_ratio"`
	SectorReturnPct           float64 `json:"sector_return_pct"`
	RealizedPnlAmount         float64 `json:"realized_pnl_amount"`
	UnrealizedPnlChangeAmount float64 `json:"unrealized_pnl_change_amount"`
	TotalPnlAmount            float64 `json:"total_pnl_amount"`
	ContributionRatio         float64 `json:"contribution_ratio"`
	TopWinnerSymbol           string  `json:"top_winner_symbol"`
	TopLoserSymbol            string  `json:"top_loser_symbol"`
	DriverLabel               string  `json:"driver_label"`
}

type PortfolioAttributionSectorGroup struct {
	Scope          string                         `json:"scope"`
	ScopeLabel     string                         `json:"scope_label"`
	CurrencyCode   string                         `json:"currency_code"`
	CurrencySymbol string                         `json:"currency_symbol"`
	Items          []PortfolioAttributionSectorItem `json:"items"`
}

type PortfolioAttributionSectorsPayload struct {
	PortfolioAttributionMeta
	ClassificationSource string                           `json:"classification_source"`
	CoverageRatio        float64                          `json:"coverage_ratio"`
	UnclassifiedStockCount int                            `json:"unclassified_stock_count"`
	Groups               []PortfolioAttributionSectorGroup `json:"groups"`
}

type PortfolioAttributionTradingTimelineItem struct {
	EventID            string  `json:"event_id"`
	TradeDate          string  `json:"trade_date"`
	Symbol             string  `json:"symbol"`
	Name               string  `json:"name"`
	EventType          string  `json:"event_type"`
	FeeAmount          float64 `json:"fee_amount"`
	RealizedPnlAmount  float64 `json:"realized_pnl_amount"`
	TimingEffectAmount float64 `json:"timing_effect_amount"`
	Note               string  `json:"note"`
}

type PortfolioAttributionTradingGroup struct {
	Scope                    string                                 `json:"scope"`
	ScopeLabel               string                                 `json:"scope_label"`
	CurrencyCode             string                                 `json:"currency_code"`
	CurrencySymbol           string                                 `json:"currency_symbol"`
	ActualTotalPnlAmount     float64                                `json:"actual_total_pnl_amount"`
	ShadowHoldPnlAmount      float64                                `json:"shadow_hold_pnl_amount"`
	TradingAlphaAmount       float64                                `json:"trading_alpha_amount"`
	FeeAmount                float64                                `json:"fee_amount"`
	TurnoverRatio            float64                                `json:"turnover_ratio"`
	TradeCount               int                                    `json:"trade_count"`
	BuyCount                 int                                    `json:"buy_count"`
	SellCount                int                                    `json:"sell_count"`
	WinSellRatio             float64                                `json:"win_sell_ratio"`
	AvgHoldingDaysBeforeSell float64                                `json:"avg_holding_days_before_sell"`
	Timeline                 []PortfolioAttributionTradingTimelineItem `json:"timeline"`
	Insights                 []PortfolioAttributionInsight          `json:"insights"`
}

type PortfolioAttributionTradingPayload struct {
	PortfolioAttributionMeta
	Groups []PortfolioAttributionTradingGroup `json:"groups"`
}

type PortfolioAttributionMarketSeriesPoint struct {
	Date                    string  `json:"date"`
	PortfolioNav            float64 `json:"portfolio_nav"`
	BenchmarkNav            float64 `json:"benchmark_nav"`
	DailyPortfolioReturnPct float64 `json:"daily_portfolio_return_pct"`
	DailyBenchmarkReturnPct float64 `json:"daily_benchmark_return_pct"`
	DailyExcessReturnPct    float64 `json:"daily_excess_return_pct"`
	ActiveWeightRatio       float64 `json:"active_weight_ratio"`
}

type PortfolioAttributionMarketGroup struct {
	Scope                       string                           `json:"scope"`
	ScopeLabel                  string                           `json:"scope_label"`
	CurrencyCode                string                           `json:"currency_code"`
	CurrencySymbol              string                           `json:"currency_symbol"`
	BenchmarkCode               string                           `json:"benchmark_code"`
	BenchmarkName               string                           `json:"benchmark_name"`
	PortfolioReturnPct          float64                          `json:"portfolio_return_pct"`
	BenchmarkReturnPct          float64                          `json:"benchmark_return_pct"`
	ExcessReturnPct             float64                          `json:"excess_return_pct"`
	MarketContributionAmount    float64                          `json:"market_contribution_amount"`
	SelectionContributionAmount float64                          `json:"selection_contribution_amount"`
	TradingAlphaAmount          float64                          `json:"trading_alpha_amount"`
	FeeAmount                   float64                          `json:"fee_amount"`
	Series                      []PortfolioAttributionMarketSeriesPoint `json:"series"`
	Insights                    []PortfolioAttributionInsight    `json:"insights"`
}

type PortfolioAttributionMarketPayload struct {
	PortfolioAttributionMeta
	Groups []PortfolioAttributionMarketGroup `json:"groups"`
}
