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

type FactorDefinition struct {
	Key           string  `json:"key"`
	Label         string  `json:"label"`
	Format        string  `json:"format"`
	Description   string  `json:"description"`
	Coverage      int64   `json:"coverage"`
	DefaultWeight float64 `json:"default_weight"`
}

type FactorFilterRange struct {
	Min *float64 `json:"min,omitempty"`
	Max *float64 `json:"max,omitempty"`
}

type FactorScreenerRequest struct {
	SnapshotDate  string                       `json:"snapshot_date,omitempty"`
	Filters       map[string]FactorFilterRange `json:"filters,omitempty"`
	FactorWeights map[string]float64           `json:"factor_weights,omitempty"`
	SortBy        string                       `json:"sort_by,omitempty"`
	SortOrder     string                       `json:"sort_order,omitempty"`
	Page          int                          `json:"page,omitempty"`
	PageSize      int                          `json:"page_size,omitempty"`
}

type FactorScreenerResponse struct {
	SnapshotDate string               `json:"snapshot_date"`
	Total        int64                `json:"total"`
	Page         int                  `json:"page"`
	PageSize     int                  `json:"page_size"`
	Items        []FactorScreenerItem `json:"items"`
}

type FactorScreenerItem struct {
	SnapshotDate       string   `json:"snapshot_date"`
	Code               string   `json:"code"`
	Symbol             string   `json:"symbol"`
	Name               string   `json:"name"`
	Industry           string   `json:"industry"`
	IsNewStock         bool     `json:"is_new_stock"`
	ClosePrice         float64  `json:"close_price"`
	CompositeScore     *float64 `json:"composite_score"`
	ValueScore         *float64 `json:"value_score"`
	DividendYieldScore *float64 `json:"dividend_yield_score"`
	GrowthScore        *float64 `json:"growth_score"`
	QualityScore       *float64 `json:"quality_score"`
	MomentumScore      *float64 `json:"momentum_score"`
	SizeScore          *float64 `json:"size_score"`
	LowVolatilityScore *float64 `json:"low_volatility_score"`
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
	Factors      []FactorDefinition    `json:"factors"`
	LastRun      FactorTaskRunMeta     `json:"last_run"`
}

type SnapshotStats struct {
	Total         int64
	NewStockCount int64
}

type ScanInput struct {
	SnapshotDate  string
	FactorWeights map[string]float64
	SortBy        string
	SortOrder     string
	Page          int
	PageSize      int
}

type ScanResult struct {
	Total int64
	Items []FactorScore
}

type FactorCoverageResponse struct {
	SnapshotDate string           `json:"snapshot_date"`
	Universe     int64            `json:"universe"`
	RawMetrics   map[string]int64 `json:"raw_metrics"`
	Factors      map[string]int64 `json:"factors"`
	Warnings     []string         `json:"warnings"`
}

type FactorPipelineAdminStatus struct {
	Worker         WorkerStatus           `json:"worker"`
	DBHealth       string                 `json:"db_health"`
	LatestSnapshot string                 `json:"latest_snapshot_date"`
	Coverage       FactorCoverageResponse `json:"coverage"`
}
