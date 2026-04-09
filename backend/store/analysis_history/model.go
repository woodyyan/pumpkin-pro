package analysis_history

import "time"

// AnalysisHistoryRecord AI 个股分析历史记录
type AnalysisHistoryRecord struct {
	ID              string    `gorm:"primaryKey;size:36"`
	UserID          string    `gorm:"size:64;not null;index:idx_ah_user_symbol,priority:1"`
	Symbol          string    `gorm:"size:20;not null;index:idx_ah_user_symbol,priority:2;index:idx_ah_symbol"`
	SymbolName      string    `gorm:"size:128;not null;default:''"`
	Signal          string    `gorm:"size:16;not null;default:''"`       // buy / sell / hold
	ConfidenceScore int       `gorm:"not null;default:0"`                // 0~100
	ResultJSON      string    `gorm:"type:text;not null"`                // 完整 StockAnalysisOutput JSON
	MetaJSON        string    `gorm:"type:text;not null;default:'{}'"`  // model/generated_at/data_completeness
	CreatedAt       time.Time `gorm:"not null;index:idx_ah_user_created,sort:desc"`
}

func (AnalysisHistoryRecord) TableName() string { return "stock_analysis_history" }

// 单用户上限
const MaxRecordsPerUser = 50

// API 输出：列表轻量项
type HistoryListItem struct {
	ID              string `json:"id"`
	Symbol          string `json:"symbol"`
	SymbolName      string `json:"symbol_name"`
	Signal          string `json:"signal"`
	ConfidenceScore int    `json:"confidence_score"`
	CreatedAt       string `json:"created_at"`
}

// API 输出：完整详情
type HistoryDetail struct {
	ID              string         `json:"id"`
	Symbol          string         `json:"symbol"`
	SymbolName      string         `json:"symbol_name"`
	Signal          string         `json:"signal"`
	ConfidenceScore int            `json:"confidence_score"`
	Result          map[string]any `json:"analysis"`
	Meta            map[string]any `json:"meta"`
	CreatedAt       string         `json:"created_at"`
}
