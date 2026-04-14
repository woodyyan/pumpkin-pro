package screener

import (
	"encoding/json"
	"time"
)

// ── DB Records ──

type WatchlistRecord struct {
	ID        string    `gorm:"primaryKey;size:36"`
	UserID    string    `gorm:"size:36;not null;index:idx_screener_wl_user,priority:1;uniqueIndex:idx_screener_wl_user_name,priority:1"`
	Name      string    `gorm:"size:128;not null;uniqueIndex:idx_screener_wl_user_name,priority:2"`
	Exchange  string    `gorm:"size:8;not null;default:'ASHARE';index"` // ASHARE | HKEX
	CreatedAt time.Time `gorm:"not null"`
	UpdatedAt time.Time `gorm:"not null"`
}

func (WatchlistRecord) TableName() string {
	return "screener_watchlists"
}

type WatchlistStockRecord struct {
	ID          string    `gorm:"primaryKey;size:36"`
	WatchlistID string    `gorm:"size:36;not null;index:idx_screener_ws_wl;uniqueIndex:idx_screener_ws_wl_code,priority:1"`
	Code        string    `gorm:"size:16;not null;uniqueIndex:idx_screener_ws_wl_code,priority:2"`
	Name        string    `gorm:"size:128;not null;default:''"`
	CreatedAt   time.Time `gorm:"not null"`
}

func (WatchlistStockRecord) TableName() string {
	return "screener_watchlist_stocks"
}

// ── API Output ──

type Watchlist struct {
	ID         string           `json:"id"`
	Name       string           `json:"name"`
	Exchange   string           `json:"exchange"`
	StockCount int              `json:"stock_count"`
	CreatedAt  string           `json:"created_at"`
	UpdatedAt  string           `json:"updated_at"`
}

type WatchlistDetail struct {
	ID       string           `json:"id"`
	Name     string           `json:"name"`
	Exchange string           `json:"exchange"`
	Stocks   []WatchlistStock `json:"stocks"`
	CreatedAt string          `json:"created_at"`
	UpdatedAt string          `json:"updated_at"`
}

type WatchlistStock struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// ── API Input ──

type CreateWatchlistInput struct {
	Name     string           `json:"name"`
	Exchange string           `json:"exchange"` // ASHARE | HKEX
	Stocks   []WatchlistStock `json:"stocks"`
}

// ── Filter persistence (stored as JSON in a text column) ──

type FiltersJSON = map[string]any

func encodeJSON(v any) (string, error) {
	if v == nil {
		return "{}", nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func decodeJSON(raw string, target any) error {
	if raw == "" {
		raw = "{}"
	}
	return json.Unmarshal([]byte(raw), target)
}

// ── Record → API converters ──

func (r WatchlistRecord) toListItem(stockCount int) Watchlist {
	return Watchlist{
		ID:         r.ID,
		Name:       r.Name,
		Exchange:   r.Exchange,
		StockCount: stockCount,
		CreatedAt:  r.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:  r.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func (r WatchlistRecord) toDetail(stocks []WatchlistStockRecord) WatchlistDetail {
	items := make([]WatchlistStock, 0, len(stocks))
	for _, s := range stocks {
		items = append(items, WatchlistStock{Code: s.Code, Name: s.Name})
	}
	return WatchlistDetail{
		ID:        r.ID,
		Name:      r.Name,
		Exchange:  r.Exchange,
		Stocks:    items,
		CreatedAt: r.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: r.UpdatedAt.UTC().Format(time.RFC3339),
	}
}
