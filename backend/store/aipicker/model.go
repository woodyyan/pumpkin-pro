package aipicker

import (
	"time"

	"github.com/woodyyan/pumpkin-pro/backend/store/factorlab"
)

const (
	MarketAShare = "ASHARE"

	SelectionBasisFactorLab = "factor_lab"

	TriggerDailyAuto = "daily_auto"
	TriggerManual    = "manual"

	ConvictionHigh   = "high"
	ConvictionMedium = "medium"
	ConvictionLow    = "low"
)

type PickerRequest struct {
	Market  string `json:"market"`
	Refresh bool   `json:"refresh,omitempty"`
}

type DailyResult struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	Market         string    `gorm:"size:16;not null;uniqueIndex:idx_aipicker_daily_market_date_trigger" json:"market"`
	TradeDate      string    `gorm:"size:10;not null;uniqueIndex:idx_aipicker_daily_market_date_trigger" json:"trade_date"`
	Trigger        string    `gorm:"size:24;not null;default:'daily_auto';uniqueIndex:idx_aipicker_daily_market_date_trigger" json:"trigger"`
	SnapshotDate   string    `gorm:"size:10;not null;default:''" json:"snapshot_date"`
	SelectionBasis string    `gorm:"size:32;not null;default:'factor_lab'" json:"selection_basis"`
	Model          string    `gorm:"size:128;not null;default:''" json:"model"`
	PayloadJSON    string    `gorm:"type:text;not null" json:"payload_json"`
	CreatedAt      time.Time `gorm:"not null;index" json:"created_at"`
	UpdatedAt      time.Time `gorm:"not null" json:"updated_at"`
}

func (DailyResult) TableName() string { return "ai_picker_daily_results" }

const (
	GenerateLogStatusSuccess = "success"
	GenerateLogStatusFailed  = "failed"
)

type GenerateLogRecord struct {
	ID            uint   `gorm:"primaryKey" json:"id"`
	TradeDate     string `gorm:"size:10;index;not null" json:"trade_date"`
	Trigger       string `gorm:"size:24;index;not null" json:"trigger"`
	Status        string `gorm:"size:16;index;not null" json:"status"`
	SnapshotDate  string `gorm:"size:10;not null;default:''" json:"snapshot_date"`
	CandidatePool int    `gorm:"not null;default:0" json:"candidate_pool"`
	Model         string `gorm:"size:128;not null;default:''" json:"model"`
	Message       string `gorm:"type:text;not null;default:''" json:"message"`
	UserID        string `gorm:"size:64;not null;default:''" json:"user_id"`
	// LLM 调用可观测字段（provider 无 max_tokens 时的监控依据）
	FinishReason     string    `gorm:"size:32;not null;default:''" json:"finish_reason"`
	PromptChars      int       `gorm:"not null;default:0" json:"prompt_chars"`
	CompletionTokens int       `gorm:"not null;default:0" json:"completion_tokens"`
	TimeoutSeconds   int       `gorm:"not null;default:0" json:"timeout_seconds"`
	ResponseMS       int       `gorm:"not null;default:0" json:"response_ms"`
	CreatedAt        time.Time `gorm:"not null;index" json:"created_at"`
}

func (GenerateLogRecord) TableName() string { return "ai_picker_generate_logs" }

type GenerateTraceRecord struct {
	ID                 uint      `gorm:"primaryKey" json:"id"`
	GenerateLogID      uint      `gorm:"not null;uniqueIndex" json:"generate_log_id"`
	SystemPrompt       string    `gorm:"type:text;not null;default:''" json:"system_prompt"`
	UserPrompt         string    `gorm:"type:text;not null;default:''" json:"user_prompt"`
	AssistantReasoning string    `gorm:"type:text;not null;default:''" json:"assistant_reasoning"`
	AssistantContent   string    `gorm:"type:text;not null;default:''" json:"assistant_content"`
	CreatedAt          time.Time `gorm:"not null;index" json:"created_at"`
}

func (GenerateTraceRecord) TableName() string { return "ai_picker_generate_traces" }

type AdminGenerateStatus struct {
	LatestResult *DailyResult        `json:"latest_result,omitempty"`
	LatestLog    *GenerateLogRecord  `json:"latest_log,omitempty"`
	Logs         []GenerateLogRecord `json:"logs"`
}

type AdminLatestGenerateRun struct {
	LatestLog *GenerateLogRecord   `json:"latest_log,omitempty"`
	Trace     *GenerateTraceRecord `json:"trace,omitempty"`
}

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
	Code                  string
	Symbol                string
	Name                  string
	Industry              string
	ClosePrice            float64
	CompositeScore        *float64
	ValueScore            *float64
	DividendYieldScore    *float64
	GrowthScore           *float64
	QualityScore          *float64
	MomentumScore         *float64
	SizeScore             *float64
	LowVolatilityScore    *float64
	HitFactors            []string
	FactorTags            []string
	TechnicalTags         []string
	ChangePct20D          *float64
	DistanceToMA20Pct     *float64
	DistanceToMA60Pct     *float64
	RSI14                 *float64
	Volatility20D         *float64
	VolumeMA5ToMA20       *float64
	TechnicalDataComplete bool
}

type FactorCandidate struct {
	factorlab.FactorScreenerItem
	HitFactors map[string]struct{}
}
