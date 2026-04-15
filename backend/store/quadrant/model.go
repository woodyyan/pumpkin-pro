package quadrant

import (
	"time"
)

// QuadrantScoreRecord maps to the `quadrant_scores` table.
type QuadrantScoreRecord struct {
	Code        string    `gorm:"primaryKey;size:10"`
	Name        string    `gorm:"size:128;not null"`
	Exchange    string    `gorm:"size:8;not null;default:'SZSE';index:idx_quadrant_exchange"`
	Opportunity float64   `gorm:"not null"`
	Risk        float64   `gorm:"not null"`
	Quadrant    string    `gorm:"size:16;not null"`
	Trend       float64   `gorm:"not null;default:0"`
	Flow        float64   `gorm:"not null;default:0"`
	Revision    float64   `gorm:"not null;default:0"`
	Liquidity   float64   `gorm:"not null;default:0"`  // 流动性：近5日均成交额 percentile rank
	Volatility  float64   `gorm:"not null;default:0"`
	Drawdown    float64   `gorm:"not null;default:0"`
	Crowding    float64   `gorm:"not null;default:0"`
	AvgAmount5d float64   `gorm:"not null;default:0"` // 近5日均成交额（万元）
	ComputedAt  time.Time `gorm:"not null"`
}

func (QuadrantScoreRecord) TableName() string {
	return "quadrant_scores"
}

// QuadrantScoreCompact is the minimal JSON returned for all-market scatter plot.
type QuadrantScoreCompact struct {
	Code        string  `json:"c"`
	Name        string  `json:"n"`
	Opportunity float64 `json:"o"`
	Risk        float64 `json:"r"`
	Quadrant    string  `json:"q"`
}

// QuadrantScoreDetail is the full JSON returned for watchlist stocks.
type QuadrantScoreDetail struct {
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	Opportunity float64 `json:"opportunity"`
	Risk        float64 `json:"risk"`
	Quadrant    string  `json:"quadrant"`
	Trend       float64 `json:"trend"`
	Flow        float64 `json:"flow"`
	Revision    float64 `json:"revision"`
	Liquidity   float64 `json:"liquidity"`
	Volatility  float64 `json:"volatility"`
	Drawdown    float64 `json:"drawdown"`
	Crowding    float64 `json:"crowding"`
	AvgAmount5d float64 `json:"avg_amount_5d"` // 近5日均成交额（万元）
}

// QuadrantSummary holds per-quadrant counts.
type QuadrantSummary struct {
	OpportunityZone int `json:"opportunity_zone"`
	CrowdedZone     int `json:"crowded_zone"`
	BubbleZone      int `json:"bubble_zone"`
	DefensiveZone   int `json:"defensive_zone"`
	NeutralZone     int `json:"neutral_zone"`
}

// QuadrantMeta holds metadata about the computation.
type QuadrantMeta struct {
	ComputedAt string `json:"computed_at"`
	TotalCount int    `json:"total_count"`
}

// QuadrantResponse is the API response for GET /api/quadrant.
type QuadrantResponse struct {
	Meta             QuadrantMeta          `json:"meta"`
	AllStocks        []QuadrantScoreCompact `json:"all_stocks"`
	WatchlistDetails []QuadrantScoreDetail  `json:"watchlist_details"`
	Summary          QuadrantSummary        `json:"summary"`
}

// BulkSaveInput is the input from Quant callback.
type BulkSaveInput struct {
	Items      []BulkSaveItem         `json:"items"`
	ComputedAt string                 `json:"computed_at"`
	Report     map[string]any         `json:"report,omitempty"`
}

type BulkSaveItem struct {
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	Opportunity float64 `json:"opportunity"`
	Risk        float64 `json:"risk"`
	Quadrant    string  `json:"quadrant"`
	Trend       float64 `json:"trend"`
	Flow        float64 `json:"flow"`
	Revision    float64 `json:"revision"`
	Liquidity   float64 `json:"liquidity"`
	Volatility  float64 `json:"volatility"`
	Drawdown    float64 `json:"drawdown"`
	Crowding    float64 `json:"crowding"`
	AvgAmount5d float64 `json:"avg_amount_5d"` // 近5日均成交额（万元）
	Exchange    string  `json:"exchange,omitempty"` // "SSE"/"SZSE"(默认)/"HKEX"
}

// ComputeLogRecord stores quadrant compute history for observability.
type ComputeLogRecord struct {
	ID             string    `gorm:"primaryKey;size:36"`
	ComputedAt     time.Time `gorm:"not null;index"`
	Mode           string    `gorm:"size:16;not null"`
	DurationSec    float64   `gorm:"not null;default:0"`
	StockCount     int       `gorm:"not null;default:0"`
	ReportJSON     string    `gorm:"type:text;not null;default:'{}'"`
	Status         string    `gorm:"size:16;not null;default:'success'"`
	ErrorMsg       string    `gorm:"type:text;default:''"`
}

func (ComputeLogRecord) TableName() string {
	return "quadrant_compute_logs"
}

// QuadrantStatusResponse for GET /api/quadrant/status.
type QuadrantStatusResponse struct {
	LastComputedAt string         `json:"last_computed_at"`
	StockCount     int            `json:"stock_count"`
	LastError      string         `json:"last_error"`
	LastReport     map[string]any `json:"last_report,omitempty"`
}

// ── Ranking (卧龙AI精选) ──

// RankingItem is a single stock in the ranking list.
type RankingItem struct {
	Rank        int     `json:"rank"`
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	Exchange    string  `json:"exchange"`
	Opportunity float64 `json:"opportunity"`
	Risk        float64 `json:"risk"`
	Quadrant    string  `json:"quadrant"`
	Trend       float64 `json:"trend"`
	Flow        float64 `json:"flow"`
	Revision    float64 `json:"revision"`
	Liquidity   float64 `json:"liquidity"`
	AvgAmount5d float64 `json:"avg_amount_5d"` // 近5日均成交额（万元）
}

// RankingMeta holds ranking metadata.
type RankingMeta struct {
	ComputedAt    string `json:"computed_at"`
	TotalInZone   int    `json:"total_in_zone"`
	ReturnedCount int    `json:"returned_count"`
	Exchange      string `json:"exchange"`
}

// RankingResponse is the API response for GET /api/quadrant/ranking.
type RankingResponse struct {
	Meta  RankingMeta   `json:"meta"`
	Items []RankingItem `json:"items"`
}

func (r QuadrantScoreRecord) ToCompact() QuadrantScoreCompact {
	return QuadrantScoreCompact{
		Code:        r.Code,
		Name:        r.Name,
		Opportunity: r.Opportunity,
		Risk:        r.Risk,
		Quadrant:    r.Quadrant,
	}
}

func (r QuadrantScoreRecord) ToDetail() QuadrantScoreDetail {
	return QuadrantScoreDetail{
		Code:        r.Code,
		Name:        r.Name,
		Opportunity: r.Opportunity,
		Risk:        r.Risk,
		Quadrant:    r.Quadrant,
		Trend:       r.Trend,
		Flow:        r.Flow,
		Revision:    r.Revision,
		Liquidity:   r.Liquidity,
		Volatility:  r.Volatility,
		Drawdown:    r.Drawdown,
		Crowding:    r.Crowding,
		AvgAmount5d: r.AvgAmount5d,
	}
}
