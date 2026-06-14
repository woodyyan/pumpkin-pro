package aipicker

import "time"

const (
	MarketAShare = "ASHARE"

	SelectionBasisFactorLab = "factor_lab"

	TriggerDailyAuto   = "daily_auto"
	TriggerManual      = "manual"
	TriggerByDirection = "by_direction"

	ConvictionHigh   = "high"
	ConvictionMedium = "medium"
	ConvictionLow    = "low"
)

type PickerRequest struct {
	Market    string `json:"market"`
	Direction string `json:"direction,omitempty"`
	Refresh   bool   `json:"refresh,omitempty"`
}

type DailyResult struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	Market         string    `gorm:"size:16;index;not null" json:"market"`
	TradeDate      string    `gorm:"size:10;index;not null" json:"trade_date"`
	Trigger        string    `gorm:"size:24;not null;default:'daily_auto'" json:"trigger"`
	SnapshotDate   string    `gorm:"size:10;not null;default:''" json:"snapshot_date"`
	SelectionBasis string    `gorm:"size:32;not null;default:'factor_lab'" json:"selection_basis"`
	Model          string    `gorm:"size:128;not null;default:''" json:"model"`
	PayloadJSON    string    `gorm:"type:text;not null" json:"payload_json"`
	CreatedAt      time.Time `gorm:"not null;index" json:"created_at"`
	UpdatedAt      time.Time `gorm:"not null" json:"updated_at"`
}

func (DailyResult) TableName() string { return "ai_picker_daily_results" }

type PickerResponse struct {
	Analysis *AnalysisPayload `json:"analysis"`
	Meta     map[string]any   `json:"meta"`
}

type AnalysisPayload struct {
	FormatVersion       string              `json:"format_version"`
	Market              string              `json:"market"`
	SnapshotDate        string              `json:"snapshot_date"`
	SelectionBasis      string              `json:"selection_basis"`
	Trigger             string              `json:"trigger"`
	MarketView          string              `json:"market_view"`
	StrategySummary     string              `json:"strategy_summary"`
	Picks               []PickItem          `json:"picks"`
	PortfolioAllocation PortfolioAllocation `json:"portfolio_allocation"`
	KeyRisks            []string            `json:"key_risks"`
	Disclaimer          string              `json:"disclaimer"`
	DataTimestamp       string              `json:"data_timestamp"`
}

type PickItem struct {
	Rank             int               `json:"rank"`
	Code             string            `json:"code"`
	Symbol           string            `json:"symbol"`
	Name             string            `json:"name"`
	Industry         string            `json:"industry"`
	CurrentPrice     float64           `json:"current_price"`
	Currency         string            `json:"currency"`
	PositionPct      int               `json:"position_pct"`
	Conviction       string            `json:"conviction"`
	ConvictionScore  int               `json:"conviction_score"`
	Reason           string            `json:"reason"`
	FactorHighlights []FactorHighlight `json:"factor_highlights"`
	CompositeScore   *float64          `json:"composite_score,omitempty"`
	EntryZone        PriceRange        `json:"entry_zone"`
	StopLoss         PricePoint        `json:"stop_loss"`
	TakeProfit       PricePoint        `json:"take_profit"`
	TimeHorizon      string            `json:"time_horizon"`
	RiskNote         string            `json:"risk_note"`
}

type FactorHighlight struct {
	Key   string   `json:"key"`
	Label string   `json:"label"`
	Score *float64 `json:"score,omitempty"`
}

type PortfolioAllocation struct {
	TotalPositionPct    int    `json:"total_position_pct"`
	CashReservePct      int    `json:"cash_reserve_pct"`
	DiversificationNote string `json:"diversification_note"`
	ExpectedStyle       string `json:"expected_style"`
}

type PriceRange struct {
	Low      float64 `json:"low"`
	High     float64 `json:"high"`
	Currency string  `json:"currency"`
}

type PricePoint struct {
	Price float64 `json:"price"`
	Pct   float64 `json:"pct"`
}

type Candidate struct {
	Code               string
	Symbol             string
	Name               string
	Industry           string
	ClosePrice         float64
	CompositeScore     *float64
	ValueScore         *float64
	DividendYieldScore *float64
	GrowthScore        *float64
	QualityScore       *float64
	MomentumScore      *float64
	SizeScore          *float64
	LowVolatilityScore *float64
}
