package quadrant

import (
	"time"
)

// QuadrantScoreRecord maps to the `quadrant_scores` table.
type QuadrantScoreRecord struct {
	Code        string    `gorm:"primaryKey;size:10"`
	Name        string    `gorm:"size:128;not null"`
	Opportunity float64   `gorm:"not null"`
	Risk        float64   `gorm:"not null"`
	Quadrant    string    `gorm:"size:16;not null"`
	Trend       float64   `gorm:"not null;default:0"`
	Flow        float64   `gorm:"not null;default:0"`
	Revision    float64   `gorm:"not null;default:0"`
	Volatility  float64   `gorm:"not null;default:0"`
	Drawdown    float64   `gorm:"not null;default:0"`
	Crowding    float64   `gorm:"not null;default:0"`
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
	Volatility  float64 `json:"volatility"`
	Drawdown    float64 `json:"drawdown"`
	Crowding    float64 `json:"crowding"`
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
	Items      []BulkSaveItem `json:"items"`
	ComputedAt string         `json:"computed_at"`
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
	Volatility  float64 `json:"volatility"`
	Drawdown    float64 `json:"drawdown"`
	Crowding    float64 `json:"crowding"`
}

// QuadrantStatusResponse for GET /api/quadrant/status.
type QuadrantStatusResponse struct {
	LastComputedAt string `json:"last_computed_at"`
	StockCount     int    `json:"stock_count"`
	LastError      string `json:"last_error"`
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
		Volatility:  r.Volatility,
		Drawdown:    r.Drawdown,
		Crowding:    r.Crowding,
	}
}
