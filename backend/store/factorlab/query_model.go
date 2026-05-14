package factorlab

import "time"

type MetricDirection string

const (
	DirectionLowerBetter  MetricDirection = "lower_better"
	DirectionHigherBetter MetricDirection = "higher_better"
	DirectionNeutral      MetricDirection = "neutral"
)

type FactorMetricDefinition struct {
	Key         string          `json:"key"`
	Label       string          `json:"label"`
	Unit        string          `json:"unit"`
	Format      string          `json:"format"`
	Direction   MetricDirection `json:"direction"`
	Description string          `json:"description"`
	Coverage    int64           `json:"coverage"`
}

type FactorMetricGroup struct {
	Key   string                   `json:"key"`
	Label string                   `json:"label"`
	Items []FactorMetricDefinition `json:"items"`
}

type FactorFilterRange struct {
	Min *float64 `json:"min,omitempty"`
	Max *float64 `json:"max,omitempty"`
}

type FactorScreenerRequest struct {
	SnapshotDate string                       `json:"snapshot_date,omitempty"`
	Filters      map[string]FactorFilterRange `json:"filters,omitempty"`
	SortBy       string                       `json:"sort_by,omitempty"`
	SortOrder    string                       `json:"sort_order,omitempty"`
	Page         int                          `json:"page,omitempty"`
	PageSize     int                          `json:"page_size,omitempty"`
}

type FactorScreenerResponse struct {
	SnapshotDate string               `json:"snapshot_date"`
	Total        int64                `json:"total"`
	Page         int                  `json:"page"`
	PageSize     int                  `json:"page_size"`
	Items        []FactorScreenerItem `json:"items"`
}

type FactorScreenerItem struct {
	SnapshotDate            string   `json:"snapshot_date"`
	Code                    string   `json:"code"`
	Symbol                  string   `json:"symbol"`
	Name                    string   `json:"name"`
	Board                   string   `json:"board"`
	ListingAgeDays          *int     `json:"listing_age_days"`
	IsNewStock              bool     `json:"is_new_stock"`
	AvailableTradingDays    int      `json:"available_trading_days"`
	ClosePrice              float64  `json:"close_price"`
	MarketCap               *float64 `json:"market_cap"`
	PE                      *float64 `json:"pe"`
	PB                      *float64 `json:"pb"`
	PS                      *float64 `json:"ps"`
	DividendYield           *float64 `json:"dividend_yield"`
	EarningGrowth           *float64 `json:"earning_growth"`
	RevenueGrowth           *float64 `json:"revenue_growth"`
	Performance1Y           *float64 `json:"performance_1y"`
	PerformanceSinceListing *float64 `json:"performance_since_listing"`
	Momentum1M              *float64 `json:"momentum_1m"`
	ROE                     *float64 `json:"roe"`
	OperatingCFMargin       *float64 `json:"operating_cf_margin"`
	AssetToEquity           *float64 `json:"asset_to_equity"`
	Volatility1M            *float64 `json:"volatility_1m"`
	Beta1Y                  *float64 `json:"beta_1y"`
	DataQualityFlags        []string `json:"data_quality_flags"`
}

type FactorLabUniverseMeta struct {
	Total         int64 `json:"total"`
	NewStockCount int64 `json:"new_stock_count"`
}

type FactorTaskRunMeta struct {
	ID           string     `json:"id,omitempty"`
	Status       string     `json:"status,omitempty"`
	SnapshotDate string     `json:"snapshot_date,omitempty"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
	ErrorMessage string     `json:"error_message,omitempty"`
}

type FactorLabMetaResponse struct {
	HasSnapshot  bool                  `json:"has_snapshot"`
	SnapshotDate string                `json:"snapshot_date"`
	Stale        bool                  `json:"stale"`
	Universe     FactorLabUniverseMeta `json:"universe"`
	Coverage     map[string]int64      `json:"coverage"`
	Metrics      []FactorMetricGroup   `json:"metrics"`
	LastRun      FactorTaskRunMeta     `json:"last_run"`
}

type SnapshotStats struct {
	Total         int64
	NewStockCount int64
}

type ScanInput struct {
	SnapshotDate string
	Filters      map[string]FactorFilterRange
	SortBy       string
	SortOrder    string
	Page         int
	PageSize     int
}

type ScanResult struct {
	Total int64
	Items []FactorSnapshot
}
