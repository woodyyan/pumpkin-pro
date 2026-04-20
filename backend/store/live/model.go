package live

import "time"

// ── Closing Snapshot Cache (for non-trading hours) ──

type ClosingSnapshotRecord struct {
	Symbol       string    `gorm:"size:32;primaryKey"`
	TradeDate    string    `gorm:"size:10;primaryKey"` // "2026-04-02"
	SnapshotJSON string    `gorm:"type:text;not null"`
	UpdatedAt    time.Time `gorm:"not null"`
}

func (ClosingSnapshotRecord) TableName() string {
	return "closing_snapshots"
}

// ── Watchlist ──

type WatchlistRecord struct {
	UserID    string    `gorm:"primaryKey;size:36"`
	Symbol    string    `gorm:"primaryKey;size:16"`
	Name      string    `gorm:"size:128;not null;default:''"`
	Exchange  string    `gorm:"size:16;not null;default:'HKEX'"`
	IsActive  bool      `gorm:"not null;default:false;index"`
	CreatedAt time.Time `gorm:"not null"`
	UpdatedAt time.Time `gorm:"not null"`
}

func (WatchlistRecord) TableName() string {
	return "live_watchlist_items"
}

type WatchlistItem struct {
	Symbol    string `json:"symbol"`
	Name      string `json:"name"`
	Exchange  string `json:"exchange"`
	IsActive  bool   `json:"is_active"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type SessionState string

const (
	SessionIdle     SessionState = "idle"
	SessionWarming  SessionState = "warming_up"
	SessionRunning  SessionState = "running"
	SessionDegraded SessionState = "degraded"
	SessionStopped  SessionState = "stopped"
)

type IndexSnapshot struct {
	Code       string  `json:"code"`
	Name       string  `json:"name"`
	Last       float64 `json:"last"`
	ChangeRate float64 `json:"change_rate"`
}

type BenchmarkSnapshot struct {
	Code       string  `json:"code"`
	Name       string  `json:"name"`
	Last       float64 `json:"last"`
	ChangeRate float64 `json:"change_rate"`
	TS         string  `json:"ts"`
}

type MarketOverview struct {
	TS             string          `json:"ts"`
	Indexes        []IndexSnapshot `json:"indexes"`
	MarketTurnover float64         `json:"market_turnover"`
	Advancers      int             `json:"advancers"`
	Decliners      int             `json:"decliners"`
}

type SymbolSnapshot struct {
	Symbol       string  `json:"symbol"`
	Name         string  `json:"name"`
	LastPrice    float64 `json:"last_price"`
	ChangeRate   float64 `json:"change_rate"`
	Volume       float64 `json:"volume"`
	Turnover     float64 `json:"turnover"`
	Amplitude    float64 `json:"amplitude"`
	VolumeRatio  float64 `json:"volume_ratio"`
	TurnoverRate float64 `json:"turnover_rate"`
	TS           string  `json:"ts"`
	Source       string  `json:"source"`
}

type DetailedSymbolSnapshot struct {
	Symbol         string  `json:"symbol"`
	Name           string  `json:"name"`
	Exchange       string  `json:"exchange"`
	CurrencyCode   string  `json:"currency_code"`
	CurrencySymbol string  `json:"currency_symbol"`
	LastPrice      float64 `json:"last_price"`
	PrevClosePrice float64 `json:"prev_close_price"`
	ChangeRate     float64 `json:"change_rate"`
	Volume         float64 `json:"volume"`
	Turnover       float64 `json:"turnover"`
	Amplitude      float64 `json:"amplitude"`
	VolumeRatio    float64 `json:"volume_ratio"`
	TurnoverRate   float64 `json:"turnover_rate"`
	TS             string  `json:"ts"`
	Source         string  `json:"source"`
}

type PriceVolumeAnomaly struct {
	EventID           string         `json:"event_id"`
	Symbol            string         `json:"symbol"`
	AnomalyType       string         `json:"anomaly_type"`
	Score             float64        `json:"score"`
	ThresholdSnapshot map[string]any `json:"threshold_snapshot"`
	MetricsSnapshot   map[string]any `json:"metrics_snapshot"`
	DetectedAt        string         `json:"detected_at"`
}

type BlockFlowAnomaly struct {
	EventID           string  `json:"event_id"`
	Symbol            string  `json:"symbol"`
	NetInflow         float64 `json:"net_inflow"`
	BuyBlockAmount    float64 `json:"buy_block_amount"`
	SellBlockAmount   float64 `json:"sell_block_amount"`
	DirectionStrength float64 `json:"direction_strength"`
	Continuity        float64 `json:"continuity"`
	DetectedAt        string  `json:"detected_at"`
}

type ActivateResult struct {
	PreviousSymbol  string       `json:"previous_symbol,omitempty"`
	ActiveSymbol    string       `json:"active_symbol"`
	SessionState    SessionState `json:"session_state"`
	WarmupMinSample int          `json:"warmup_min_samples"`
}

type OverlayPoint struct {
	TS             string  `json:"ts"`
	StockPrice     float64 `json:"stock_price"`
	BenchmarkPrice float64 `json:"benchmark_price"`
	StockNorm      float64 `json:"stock_norm"`
	BenchmarkNorm  float64 `json:"benchmark_norm"`
}

type OverlayMetrics struct {
	Beta             *float64 `json:"beta"`
	RelativeStrength *float64 `json:"relative_strength"`
	SampleCount      int      `json:"sample_count"`
	WarmupMinSamples int      `json:"warmup_min_samples"`
	IsWarmup         bool     `json:"is_warmup"`
}

type OverlayPayload struct {
	Symbol        string         `json:"symbol"`
	Benchmark     string         `json:"benchmark"`
	WindowMinutes int            `json:"window_minutes"`
	SessionState  SessionState   `json:"session_state"`
	Series        []OverlayPoint `json:"series"`
	Metrics       OverlayMetrics `json:"metrics"`
	UpdatedAt     string         `json:"updated_at"`
}

type DailyBar struct {
	Date   string  `json:"date"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume float64 `json:"volume"`
}

type SupportSummary struct {
	NearestLevel string  `json:"nearest_level"`
	NearestPrice float64 `json:"nearest_price"`
	DistancePct  float64 `json:"distance_pct"`
	Strength     string  `json:"strength"`
	Status       string  `json:"status"`
}

type SupportLevel struct {
	Level           string   `json:"level"`
	Price           float64  `json:"price"`
	BandLow         float64  `json:"band_low"`
	BandHigh        float64  `json:"band_high"`
	DistancePct     float64  `json:"distance_pct"`
	Strength        string   `json:"strength"`
	Score           float64  `json:"score"`
	Status          string   `json:"status"`
	Sources         []string `json:"sources"`
	TouchCount      int      `json:"touch_count"`
	LastValidatedAt string   `json:"last_validated_at"`
}

type SupportMeta struct {
	Algorithm          string `json:"algorithm"`
	SampleCount        int    `json:"sample_count"`
	MinRequiredSamples int    `json:"min_required_samples"`
	IsWarmup           bool   `json:"is_warmup"`
	UpdatedAt          string `json:"updated_at"`
}

type SupportLevelsPayload struct {
	Symbol       string         `json:"symbol"`
	Period       string         `json:"period"`
	LookbackDays int            `json:"lookback_days"`
	AsOf         string         `json:"as_of"`
	PriceRef     float64        `json:"price_ref"`
	SessionState SessionState   `json:"session_state"`
	Summary      SupportSummary `json:"summary"`
	Levels       []SupportLevel `json:"levels"`
	Meta         SupportMeta    `json:"meta"`
}

type ResistanceSummary struct {
	NearestLevel string  `json:"nearest_level"`
	NearestPrice float64 `json:"nearest_price"`
	DistancePct  float64 `json:"distance_pct"`
	Strength     string  `json:"strength"`
	Status       string  `json:"status"`
}

type ResistanceLevel struct {
	Level           string   `json:"level"`
	Price           float64  `json:"price"`
	BandLow         float64  `json:"band_low"`
	BandHigh        float64  `json:"band_high"`
	DistancePct     float64  `json:"distance_pct"`
	Strength        string   `json:"strength"`
	Score           float64  `json:"score"`
	Status          string   `json:"status"`
	Sources         []string `json:"sources"`
	TouchCount      int      `json:"touch_count"`
	LastValidatedAt string   `json:"last_validated_at"`
}

type ResistanceMeta struct {
	Algorithm          string `json:"algorithm"`
	SampleCount        int    `json:"sample_count"`
	MinRequiredSamples int    `json:"min_required_samples"`
	IsWarmup           bool   `json:"is_warmup"`
	UpdatedAt          string `json:"updated_at"`
}

type ResistanceLevelsPayload struct {
	Symbol       string            `json:"symbol"`
	Period       string            `json:"period"`
	LookbackDays int               `json:"lookback_days"`
	AsOf         string            `json:"as_of"`
	PriceRef     float64           `json:"price_ref"`
	SessionState SessionState      `json:"session_state"`
	Summary      ResistanceSummary `json:"summary"`
	Levels       []ResistanceLevel `json:"levels"`
	Meta         ResistanceMeta    `json:"meta"`
}

type MovingAveragesPayload struct {
	Symbol             string       `json:"symbol"`
	Period             string       `json:"period"`
	LookbackDays       int          `json:"lookback_days"`
	AsOf               string       `json:"as_of"`
	PriceRef           float64      `json:"price_ref"`
	MA5                float64      `json:"ma5"`
	MA20               float64      `json:"ma20"`
	MA60               float64      `json:"ma60"`
	MA200              float64      `json:"ma200"`
	DistanceToMA5Pct   float64      `json:"distance_to_ma5_pct"`
	DistanceToMA20Pct  float64      `json:"distance_to_ma20_pct"`
	DistanceToMA60Pct  float64      `json:"distance_to_ma60_pct"`
	DistanceToMA200Pct float64      `json:"distance_to_ma200_pct"`
	RSI14              float64      `json:"rsi14"`
	RSI14Status        string       `json:"rsi14_status"`
	MACD               float64      `json:"macd"`
	MACDSignal         float64      `json:"macd_signal"`
	MACDHistogram      float64      `json:"macd_histogram"`
	MACDSeries         []MACDPoint      `json:"macd_series,omitempty"`
	BollingerUpper     float64          `json:"bollinger_upper"`
	BollingerLower     float64          `json:"bollinger_lower"`
	BollingerBandwidth float64          `json:"bollinger_bandwidth"`
	BollingerPercentB  float64          `json:"bollinger_percent_b"`
	BollingerSeries    []BollingerPoint `json:"bollinger_series,omitempty"`
	ChangePct60D       float64          `json:"change_pct_60d"`
	Volatility20D      float64          `json:"volatility_20d"`
	VolumeMA5toMA20    float64          `json:"volume_ma5_to_ma20"`
	Status             string           `json:"status"`
	SessionState       SessionState `json:"session_state"`
	UpdatedAt          string       `json:"updated_at"`
}

type MACDPoint struct {
	Date      string  `json:"date"`
	DIF       float64 `json:"dif"`
	Signal    float64 `json:"signal"`
	Histogram float64 `json:"histogram"`
}

type BollingerPoint struct {
	Date   string  `json:"date"`
	Close  float64 `json:"close"`
	Upper  float64 `json:"upper"`
	Middle float64 `json:"middle"`
	Lower  float64 `json:"lower"`
}

type WatchlistState struct {
	SessionState SessionState    `json:"session_state"`
	ActiveSymbol string          `json:"active_symbol,omitempty"`
	Items        []WatchlistItem `json:"items"`
}

type DailyOverlayPoint struct {
	Date          string  `json:"date"`
	StockClose    float64 `json:"stock_close"`
	BenchClose    float64 `json:"bench_close"`
	StockNorm     float64 `json:"stock_norm"`
	BenchNorm     float64 `json:"bench_norm"`
}

type DailyOverlayMetrics struct {
	RelativeStrength *float64 `json:"relative_strength"`
	Beta             *float64 `json:"beta"`
	Correlation      *float64 `json:"correlation"`
	SampleDays       int      `json:"sample_days"`
}

type DailyOverlayPayload struct {
	Symbol       string              `json:"symbol"`
	Benchmark    string              `json:"benchmark"`
	LookbackDays int                 `json:"lookback_days"`
	Series       []DailyOverlayPoint `json:"series"`
	Metrics      DailyOverlayMetrics `json:"metrics"`
	UpdatedAt    string              `json:"updated_at"`
}

type WatchlistSnapshotsPayload struct {
	Items []SymbolSnapshot `json:"items"`
}
