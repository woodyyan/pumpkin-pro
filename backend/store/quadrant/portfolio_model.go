package quadrant

import "time"

const (
	defaultRankingPortfolioDefinitionID   = "wolong_ai_top4_ex_star_equal_v1"
	defaultRankingPortfolioDefinitionCode = "wolong-ai-top4-ex-star-equal"
	defaultRankingPortfolioName           = "卧龙AI精选模拟组合"
	defaultRankingPortfolioExchange       = "ASHARE"
	defaultRankingPortfolioBenchmarkCode  = "SHCI"
	defaultRankingPortfolioBenchmarkName  = "上证指数"
	defaultRankingPortfolioMaxHoldings    = 4
	defaultRankingPortfolioTradeCostRate  = 0.0002
	defaultRankingPortfolioWarningText    = "当日有效成分股不足 4 只"
)

// RankingPortfolioDefinition stores the portfolio rule definition separately
// from the computed result batches.
type RankingPortfolioDefinition struct {
	ID              string    `gorm:"primaryKey;size:64"`
	Code            string    `gorm:"size:64;not null;uniqueIndex"`
	Name            string    `gorm:"size:128;not null"`
	Exchange        string    `gorm:"size:16;not null;default:'ASHARE'"`
	BenchmarkCode   string    `gorm:"size:16;not null;default:'SHCI'"`
	BenchmarkName   string    `gorm:"size:64;not null;default:'上证指数'"`
	MaxHoldings     int       `gorm:"not null;default:4"`
	ExcludedBoards  string    `gorm:"type:text;not null;default:'[]'"`
	WeightingMethod string    `gorm:"size:32;not null;default:'equal'"`
	RebalanceRule   string    `gorm:"size:64;not null;default:'t_close_generate_t1_open_rebalance'"`
	TradeCostRate   float64   `gorm:"not null;default:0.0002"`
	MethodNote      string    `gorm:"type:text;not null;default:''"`
	IsActive        bool      `gorm:"not null;default:true"`
	CreatedAt       time.Time `gorm:"not null"`
	UpdatedAt       time.Time `gorm:"not null"`
}

func (RankingPortfolioDefinition) TableName() string {
	return "quadrant_ranking_portfolio_definitions"
}

// RankingPortfolioSnapshot stores one versioned daily source snapshot.
type RankingPortfolioSnapshot struct {
	ID                    int64     `gorm:"primaryKey;autoIncrement"`
	DefinitionID          string    `gorm:"size:64;not null;uniqueIndex:uidx_qrps_def_ver,priority:1;index:idx_qrps_def_date,priority:1"`
	SnapshotVersion       string    `gorm:"size:64;not null;uniqueIndex:uidx_qrps_def_ver,priority:2"`
	BatchID               string    `gorm:"size:64;not null"`
	SnapshotDate          string    `gorm:"size:10;not null;index:idx_qrps_def_date,priority:2"`
	RankingTime           time.Time `gorm:"not null"`
	HoldingsEffectiveTime time.Time `gorm:"not null"`
	NavAsOfTime           time.Time `gorm:"not null"`
	BenchmarkCode         string    `gorm:"size:16;not null;default:'SHCI'"`
	BenchmarkName         string    `gorm:"size:64;not null;default:'上证指数'"`
	ConstituentsCount     int       `gorm:"not null;default:0"`
	HasShortfall          bool      `gorm:"not null;default:false"`
	WarningText           string    `gorm:"size:128;not null;default:''"`
	MethodNote            string    `gorm:"type:text;not null;default:''"`
	CreatedAt             time.Time `gorm:"not null;index:idx_qrps_def_date,priority:3"`
	UpdatedAt             time.Time `gorm:"not null"`
}

func (RankingPortfolioSnapshot) TableName() string {
	return "quadrant_ranking_portfolio_snapshots"
}

type RankingPortfolioSnapshotConstituent struct {
	ID              int64     `gorm:"primaryKey;autoIncrement"`
	DefinitionID    string    `gorm:"size:64;not null;uniqueIndex:uidx_qrpsc_def_ver_code,priority:1;index:idx_qrpsc_def_ver_rank,priority:1"`
	SnapshotVersion string    `gorm:"size:64;not null;uniqueIndex:uidx_qrpsc_def_ver_code,priority:2;index:idx_qrpsc_def_ver_rank,priority:2"`
	SnapshotDate    string    `gorm:"size:10;not null"`
	Rank            int       `gorm:"not null;index:idx_qrpsc_def_ver_rank,priority:3"`
	Code            string    `gorm:"size:10;not null;uniqueIndex:uidx_qrpsc_def_ver_code,priority:3"`
	Name            string    `gorm:"size:128;not null;default:''"`
	Exchange        string    `gorm:"size:8;not null;default:'SZSE'"`
	Board           string    `gorm:"size:16;not null;default:''"`
	Weight          float64   `gorm:"not null;default:0"`
	RankingScore    float64   `gorm:"not null;default:0"`
	Opportunity     float64   `gorm:"not null;default:0"`
	Risk            float64   `gorm:"not null;default:0"`
	CreatedAt       time.Time `gorm:"not null"`
	UpdatedAt       time.Time `gorm:"not null"`
}

func (RankingPortfolioSnapshotConstituent) TableName() string {
	return "quadrant_ranking_portfolio_constituents"
}

// RankingPortfolioMarketPrice stores source stock closes used for offline recompute.
type RankingPortfolioMarketPrice struct {
	ID              int64     `gorm:"primaryKey;autoIncrement"`
	DefinitionID    string    `gorm:"size:64;not null;uniqueIndex:uidx_qrpmp_def_ver_code,priority:1;index:idx_qrpmp_def_date,priority:1"`
	SnapshotVersion string    `gorm:"size:64;not null;uniqueIndex:uidx_qrpmp_def_ver_code,priority:2"`
	SnapshotDate    string    `gorm:"size:10;not null;index:idx_qrpmp_def_date,priority:2"`
	Code            string    `gorm:"size:10;not null;uniqueIndex:uidx_qrpmp_def_ver_code,priority:3;index:idx_qrpmp_def_date,priority:3"`
	Exchange        string    `gorm:"size:8;not null;default:'SZSE'"`
	ClosePrice      float64   `gorm:"not null;default:0"`
	PriceTradeDate  string    `gorm:"size:10;not null;default:''"`
	CreatedAt       time.Time `gorm:"not null"`
	UpdatedAt       time.Time `gorm:"not null"`
}

func (RankingPortfolioMarketPrice) TableName() string {
	return "quadrant_ranking_portfolio_market_prices"
}

type RankingPortfolioBenchmarkPrice struct {
	ID              int64     `gorm:"primaryKey;autoIncrement"`
	DefinitionID    string    `gorm:"size:64;not null;uniqueIndex:uidx_qrpbp_def_ver,priority:1;index:idx_qrpbp_def_date,priority:1"`
	SnapshotVersion string    `gorm:"size:64;not null;uniqueIndex:uidx_qrpbp_def_ver,priority:2"`
	SnapshotDate    string    `gorm:"size:10;not null;index:idx_qrpbp_def_date,priority:2"`
	BenchmarkCode   string    `gorm:"size:16;not null;default:'SHCI'"`
	BenchmarkName   string    `gorm:"size:64;not null;default:'上证指数'"`
	ClosePrice      float64   `gorm:"not null;default:0"`
	PriceTradeDate  string    `gorm:"size:10;not null;default:''"`
	CreatedAt       time.Time `gorm:"not null"`
	UpdatedAt       time.Time `gorm:"not null"`
}

func (RankingPortfolioBenchmarkPrice) TableName() string {
	return "quadrant_ranking_portfolio_benchmark_prices"
}

// RankingPortfolioResult stores one fully materialized, batch-consistent view
// for frontend queries.
type RankingPortfolioResult struct {
	ID                      int64     `gorm:"primaryKey;autoIncrement"`
	DefinitionID            string    `gorm:"size:64;not null;uniqueIndex:uidx_qrpr_def_ver,priority:1;index:idx_qrpr_def_date,priority:1"`
	SnapshotVersion         string    `gorm:"size:64;not null;uniqueIndex:uidx_qrpr_def_ver,priority:2"`
	BatchID                 string    `gorm:"size:64;not null"`
	SnapshotDate            string    `gorm:"size:10;not null;index:idx_qrpr_def_date,priority:2"`
	RankingTime             time.Time `gorm:"not null"`
	HoldingsEffectiveTime   time.Time `gorm:"not null"`
	NavAsOfTime             time.Time `gorm:"not null"`
	BenchmarkCode           string    `gorm:"size:16;not null;default:'SHCI'"`
	BenchmarkName           string    `gorm:"size:64;not null;default:'上证指数'"`
	LatestNav               float64   `gorm:"not null;default:1"`
	LatestBenchmarkNav      float64   `gorm:"not null;default:1"`
	LatestPortfolioReturn   float64   `gorm:"not null;default:0"`
	LatestBenchmarkReturn   float64   `gorm:"not null;default:0"`
	LatestExcessReturnPct   float64   `gorm:"not null;default:0"`
	CurrentConstituentCount int       `gorm:"not null;default:0"`
	HasShortfall            bool      `gorm:"not null;default:false"`
	WarningText             string    `gorm:"size:128;not null;default:''"`
	MethodNote              string    `gorm:"type:text;not null;default:''"`
	SeriesJSON              string    `gorm:"type:text;not null;default:'[]'"`
	ConstituentsJSON        string    `gorm:"type:text;not null;default:'[]'"`
	LatestRebalanceJSON     string    `gorm:"type:text;not null;default:''"`
	CreatedAt               time.Time `gorm:"not null"`
	UpdatedAt               time.Time `gorm:"not null"`
}

func (RankingPortfolioResult) TableName() string {
	return "quadrant_ranking_portfolio_results"
}

type RankingPortfolioSeriesPoint struct {
	Date                    string  `json:"date"`
	Nav                     float64 `json:"nav"`
	BenchmarkNav            float64 `json:"benchmark_nav"`
	PortfolioReturnPct      float64 `json:"portfolio_return_pct"`
	BenchmarkReturnPct      float64 `json:"benchmark_return_pct"`
	ExcessReturnPct         float64 `json:"excess_return_pct"`
	DailyPortfolioReturnPct float64 `json:"daily_portfolio_return_pct"`
	DailyBenchmarkReturnPct float64 `json:"daily_benchmark_return_pct"`
	HoldingCount            int     `json:"holding_count"`
}

type RankingPortfolioConstituentItem struct {
	Rank         int     `json:"rank"`
	Code         string  `json:"code"`
	Name         string  `json:"name"`
	Exchange     string  `json:"exchange"`
	Board        string  `json:"board,omitempty"`
	Weight       float64 `json:"weight"`
	RankingScore float64 `json:"ranking_score,omitempty"`
	Opportunity  float64 `json:"opportunity,omitempty"`
	Risk         float64 `json:"risk,omitempty"`
}

type RankingPortfolioRebalanceItem struct {
	Action             string  `json:"action"`
	Code               string  `json:"code"`
	Name               string  `json:"name"`
	Exchange           string  `json:"exchange"`
	Board              string  `json:"board,omitempty"`
	FromWeight         float64 `json:"from_weight"`
	ToWeight           float64 `json:"to_weight"`
	ReferencePrice     float64 `json:"reference_price"`
	ReferenceCostPrice float64 `json:"reference_cost_price"`
	PriceTradeDate     string  `json:"price_trade_date,omitempty"`
}

type RankingPortfolioLatestRebalance struct {
	SnapshotDate  string                          `json:"snapshot_date"`
	RankingTime   string                          `json:"ranking_time"`
	EffectiveTime string                          `json:"effective_time"`
	TradeCostRate float64                         `json:"trade_cost_rate"`
	ChangeCount   int                             `json:"change_count"`
	Items         []RankingPortfolioRebalanceItem `json:"items"`
}

type RankingPortfolioMeta struct {
	DefinitionID             string  `json:"definition_id"`
	DefinitionCode           string  `json:"definition_code"`
	Name                     string  `json:"name"`
	BatchID                  string  `json:"batch_id"`
	SnapshotVersion          string  `json:"snapshot_version"`
	SnapshotDate             string  `json:"snapshot_date"`
	BenchmarkCode            string  `json:"benchmark_code"`
	BenchmarkName            string  `json:"benchmark_name"`
	RankingTime              string  `json:"ranking_time"`
	HoldingsEffectiveTime    string  `json:"holdings_effective_time"`
	NavAsOfTime              string  `json:"nav_as_of_time"`
	UpdatedAt                string  `json:"updated_at"`
	LatestNav                float64 `json:"latest_nav"`
	LatestBenchmarkNav       float64 `json:"latest_benchmark_nav"`
	LatestPortfolioReturnPct float64 `json:"latest_portfolio_return_pct"`
	LatestBenchmarkReturnPct float64 `json:"latest_benchmark_return_pct"`
	LatestExcessReturnPct    float64 `json:"latest_excess_return_pct"`
	CurrentConstituentCount  int     `json:"current_constituent_count"`
	HasShortfall             bool    `json:"has_shortfall"`
	WarningText              string  `json:"warning_text,omitempty"`
	MethodNote               string  `json:"method_note"`
}

type RankingPortfolioResponse struct {
	Meta            RankingPortfolioMeta              `json:"meta"`
	Series          []RankingPortfolioSeriesPoint     `json:"series"`
	Constituents    []RankingPortfolioConstituentItem `json:"constituents"`
	LatestRebalance *RankingPortfolioLatestRebalance  `json:"latest_rebalance,omitempty"`
}
