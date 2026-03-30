package portfolio

import "time"

// ── DB Record ──

type PortfolioRecord struct {
	ID           string    `gorm:"primaryKey;size:36"`
	UserID       string    `gorm:"size:36;not null;index;uniqueIndex:idx_portfolio_user_symbol,priority:1"`
	Symbol       string    `gorm:"size:16;not null;uniqueIndex:idx_portfolio_user_symbol,priority:2"`
	Shares       float64   `gorm:"not null;default:0"`
	AvgCostPrice float64   `gorm:"not null;default:0"`
	BuyDate      string    `gorm:"size:10;not null;default:''"`
	Note         string    `gorm:"type:text;not null;default:''"`
	CreatedAt    time.Time `gorm:"not null"`
	UpdatedAt    time.Time `gorm:"not null"`
}

func (PortfolioRecord) TableName() string {
	return "user_portfolios"
}

// ── API Output ──

type PortfolioItem struct {
	Symbol       string  `json:"symbol"`
	Shares       float64 `json:"shares"`
	AvgCostPrice float64 `json:"avg_cost_price"`
	BuyDate      string  `json:"buy_date"`
	Note         string  `json:"note"`
	UpdatedAt    string  `json:"updated_at"`
}

// ── API Input ──

type UpsertPortfolioInput struct {
	Shares       float64 `json:"shares"`
	AvgCostPrice float64 `json:"avg_cost_price"`
	BuyDate      string  `json:"buy_date"`
	Note         string  `json:"note"`
}

// ── Converters ──

func (r PortfolioRecord) toItem() PortfolioItem {
	return PortfolioItem{
		Symbol:       r.Symbol,
		Shares:       r.Shares,
		AvgCostPrice: r.AvgCostPrice,
		BuyDate:      r.BuyDate,
		Note:         r.Note,
		UpdatedAt:    r.UpdatedAt.UTC().Format(time.RFC3339),
	}
}
