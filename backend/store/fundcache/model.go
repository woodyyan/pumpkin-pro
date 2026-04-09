package fundcache

import "time"

// FundamentalsCacheRow 基础面数据缓存行，symbol 为主键
type FundamentalsCacheRow struct {
	Symbol    string    `gorm:"primaryKey;size:20"`
	DataJSON  string    `gorm:"type:text;not null"` // 原始 Quant 返回的完整 JSON
	FetchedAt time.Time `gorm:"index;not null"`
}
