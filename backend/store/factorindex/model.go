package factorindex

import "time"

const (
	ExchangeAShare = "ASHARE"

	StatusPending   = "pending"
	StatusCompleted = "completed"
	StatusPartial   = "partial"
	StatusFailed    = "failed"

	defaultBaseNAV = 1000.0
	defaultTopN    = 50
	defaultWeight  = 0.02
)

type Definition struct {
	ID        string    `gorm:"primaryKey;size:64"`
	FactorKey string    `gorm:"size:32;not null;uniqueIndex:uidx_factor_index_definition,priority:1"`
	Name      string    `gorm:"size:64;not null;default:''"`
	Exchange  string    `gorm:"size:16;not null;default:'ASHARE';uniqueIndex:uidx_factor_index_definition,priority:2;index"`
	BaseNAV   float64   `gorm:"not null;default:1000"`
	TopN      int       `gorm:"not null;default:50"`
	Weight    float64   `gorm:"not null;default:0.02"`
	IsActive  bool      `gorm:"not null;default:true;index"`
	CreatedAt time.Time `gorm:"not null"`
	UpdatedAt time.Time `gorm:"not null"`
}

func (Definition) TableName() string { return "factor_index_definitions" }

type Rebalance struct {
	ID                 string    `gorm:"primaryKey;size:64"`
	IndexID            string    `gorm:"size:64;not null;uniqueIndex:uidx_factor_index_rebalance,priority:1;index"`
	SignalDate         string    `gorm:"size:10;not null;uniqueIndex:uidx_factor_index_rebalance,priority:2;index"`
	SourceTradeDate    string    `gorm:"size:10;not null;default:''"`
	EffectiveStartDate string    `gorm:"size:10;not null;default:'';index"`
	EffectiveEndDate   string    `gorm:"size:10;not null;default:'';index"`
	ConstituentCount   int       `gorm:"not null;default:0"`
	Status             string    `gorm:"size:16;not null;default:'pending';index"`
	WarningText        string    `gorm:"type:text;not null;default:''"`
	ComputedAt         time.Time `gorm:"not null"`
	CreatedAt          time.Time `gorm:"not null"`
	UpdatedAt          time.Time `gorm:"not null"`
}

func (Rebalance) TableName() string { return "factor_index_rebalances" }

type Constituent struct {
	ID               int64     `gorm:"primaryKey;autoIncrement"`
	RebalanceID      string    `gorm:"size:64;not null;uniqueIndex:uidx_factor_index_constituent,priority:1;index"`
	IndexID          string    `gorm:"size:64;not null;index"`
	StockCode        string    `gorm:"size:16;not null;uniqueIndex:uidx_factor_index_constituent,priority:2"`
	StockName        string    `gorm:"size:128;not null;default:''"`
	Exchange         string    `gorm:"size:8;not null;default:'';uniqueIndex:uidx_factor_index_constituent,priority:3"`
	Rank             int       `gorm:"not null;default:0"`
	FactorScore      float64   `gorm:"not null;default:0"`
	Weight           float64   `gorm:"not null;default:0.02"`
	SignalClosePrice float64   `gorm:"not null;default:0"`
	Industry         string    `gorm:"size:128;not null;default:''"`
	CreatedAt        time.Time `gorm:"not null"`
	UpdatedAt        time.Time `gorm:"not null"`
}

func (Constituent) TableName() string { return "factor_index_constituents" }

type Daily struct {
	ID               int64     `gorm:"primaryKey;autoIncrement"`
	IndexID          string    `gorm:"size:64;not null;uniqueIndex:uidx_factor_index_daily,priority:1;index"`
	TradeDate        string    `gorm:"size:10;not null;uniqueIndex:uidx_factor_index_daily,priority:2;index"`
	SourceTradeDate  string    `gorm:"size:10;not null;default:''"`
	RebalanceID      string    `gorm:"size:64;not null;default:'';index"`
	NAV              float64   `gorm:"not null;default:1000"`
	DailyReturn      float64   `gorm:"not null;default:0"`
	TotalReturn      float64   `gorm:"not null;default:0"`
	WeeklyReturn     *float64  `gorm:"default:null"`
	MonthlyReturn    *float64  `gorm:"default:null"`
	ThreeMonthReturn *float64  `gorm:"default:null"`
	HalfYearReturn   *float64  `gorm:"default:null"`
	ConstituentCount int       `gorm:"not null;default:0"`
	ValidPriceCount  int       `gorm:"not null;default:0"`
	Status           string    `gorm:"size:16;not null;default:'pending';index"`
	WarningText      string    `gorm:"type:text;not null;default:''"`
	ComputedAt       time.Time `gorm:"not null"`
	CreatedAt        time.Time `gorm:"not null"`
	UpdatedAt        time.Time `gorm:"not null"`
}

func (Daily) TableName() string { return "factor_index_daily" }

type TrendPoint struct {
	Date  string  `json:"date"`
	Count float64 `json:"count"`
}

type OverviewItem struct {
	IndexID            string       `json:"index_id"`
	FactorKey          string       `json:"factor_key"`
	Name               string       `json:"name"`
	Exchange           string       `json:"exchange"`
	NAV                *float64     `json:"nav,omitempty"`
	DailyReturn        *float64     `json:"daily_return,omitempty"`
	TotalReturn        *float64     `json:"total_return,omitempty"`
	WeeklyReturn       *float64     `json:"weekly_return,omitempty"`
	MonthlyReturn      *float64     `json:"monthly_return,omitempty"`
	ThreeMonthReturn   *float64     `json:"three_month_return,omitempty"`
	HalfYearReturn     *float64     `json:"half_year_return,omitempty"`
	LatestTradeDate    string       `json:"latest_trade_date,omitempty"`
	SourceTradeDate    string       `json:"source_trade_date,omitempty"`
	RebalanceDate      string       `json:"rebalance_date,omitempty"`
	EffectiveStartDate string       `json:"effective_start_date,omitempty"`
	ConstituentCount   int          `json:"constituent_count"`
	Status             string       `json:"status"`
	WarningText        string       `json:"warning_text,omitempty"`
	TrendPoints        []TrendPoint `json:"trend_points,omitempty"`
}

type OverviewResponse struct {
	Exchange        string         `json:"exchange"`
	SourceTradeDate string         `json:"source_trade_date,omitempty"`
	Items           []OverviewItem `json:"items"`
}

type defaultDefinition struct {
	ID         string
	FactorKey  string
	Name       string
	ScoreField string
}

var defaultDefinitions = []defaultDefinition{
	{ID: "fi_value_ashare", FactorKey: "value", Name: "价值因子指数", ScoreField: "value_score"},
	{ID: "fi_dividend_yield_ashare", FactorKey: "dividend_yield", Name: "股息率因子指数", ScoreField: "dividend_yield_score"},
	{ID: "fi_growth_ashare", FactorKey: "growth", Name: "成长因子指数", ScoreField: "growth_score"},
	{ID: "fi_quality_ashare", FactorKey: "quality", Name: "质量因子指数", ScoreField: "quality_score"},
	{ID: "fi_momentum_ashare", FactorKey: "momentum", Name: "动量因子指数", ScoreField: "momentum_score"},
	{ID: "fi_size_ashare", FactorKey: "size", Name: "小市值因子指数", ScoreField: "size_score"},
	{ID: "fi_low_volatility_ashare", FactorKey: "low_volatility", Name: "低波动因子指数", ScoreField: "low_volatility_score"},
}

func definitionByFactorKey(key string) *defaultDefinition {
	for _, item := range defaultDefinitions {
		if item.FactorKey == key {
			copyItem := item
			return &copyItem
		}
	}
	return nil
}
