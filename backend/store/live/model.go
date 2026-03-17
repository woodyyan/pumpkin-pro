package live

import "time"

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
	Symbol      string  `json:"symbol"`
	Name        string  `json:"name"`
	LastPrice   float64 `json:"last_price"`
	ChangeRate  float64 `json:"change_rate"`
	Volume      float64 `json:"volume"`
	Turnover    float64 `json:"turnover"`
	Amplitude   float64 `json:"amplitude"`
	VolumeRatio float64 `json:"volume_ratio"`
	TS          string  `json:"ts"`
	Source      string  `json:"source"`
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

type WatchlistState struct {
	SessionState SessionState    `json:"session_state"`
	ActiveSymbol string          `json:"active_symbol,omitempty"`
	Items        []WatchlistItem `json:"items"`
}
