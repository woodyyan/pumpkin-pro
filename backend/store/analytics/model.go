package analytics

import "time"

type PageViewRecord struct {
	ID          string    `gorm:"primaryKey;size:36"`
	VisitorID   string    `gorm:"size:64;not null;index"`
	UserID      string    `gorm:"size:36;index"`
	PagePath    string    `gorm:"size:256;not null;index"`
	Referrer    string    `gorm:"size:512;not null;default:''"`
	UserAgent   string    `gorm:"size:512;not null;default:''"`
	ScreenWidth int       `gorm:"not null;default:0"`
	CreatedAt   time.Time `gorm:"not null;index"`
}

func (PageViewRecord) TableName() string {
	return "page_views"
}

type PageViewInput struct {
	PagePath    string `json:"page_path"`
	VisitorID   string `json:"visitor_id"`
	ScreenWidth int    `json:"screen_width"`
}

// ── Stats types for admin dashboard ──

type DailyCount struct {
	Date  string `json:"date"`
	Count int64  `json:"count"`
}

type PageRank struct {
	PagePath string `json:"page_path"`
	Count    int64  `json:"count"`
}

type DeviceStats struct {
	Desktop int64 `json:"desktop"`
	Mobile  int64 `json:"mobile"`
	Tablet  int64 `json:"tablet"`
}
