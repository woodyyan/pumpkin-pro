package analysis_history

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Create 写入一条分析历史记录，超过上限时自动淘汰最旧记录
func (r *Repository) Create(ctx context.Context, record *AnalysisHistoryRecord) error {
	// 先检查是否超限，超限则淘汰最旧
	var count int64
	r.db.WithContext(ctx).Model(&AnalysisHistoryRecord{}).
		Where("user_id = ?", record.UserID).Count(&count)

	if count >= MaxRecordsPerUser {
		// 删除最旧的 (count - MaxRecordsPerUser + 1) 条，为新记录腾位置
		r.db.WithContext(ctx).
			Where("user_id = ? AND id NOT IN (SELECT id FROM stock_analysis_history WHERE user_id = ? ORDER BY created_at DESC LIMIT ?)",
				record.UserID, record.UserID, MaxRecordsPerUser-1).
			Delete(&AnalysisHistoryRecord{})
	}

	return r.db.WithContext(ctx).Create(record).Error
}

// ListBySymbol 获取某用户在某股票的分析历史（倒序，limit 限制条数）
func (r *Repository) ListBySymbol(ctx context.Context, userID, symbol string, limit int) ([]AnalysisHistoryRecord, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	var records []AnalysisHistoryRecord
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND symbol = ?", userID, symbol).
		Order("created_at DESC").
		Limit(limit).
		Find(&records).Error
	return records, err
}

// GetLatestBySymbol 获取某用户在某股票的最新一条分析
func (r *Repository) GetLatestBySymbol(ctx context.Context, userID, symbol string) (*AnalysisHistoryRecord, error) {
	var record AnalysisHistoryRecord
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND symbol = ?", userID, symbol).
		Order("created_at DESC").
		First(&record).Error
	if err != nil {
		return nil, err
	}
	return &record, nil
}

// GetByID 获取单条详情
func (r *Repository) GetByID(ctx context.Context, userID, id string) (*AnalysisHistoryRecord, error) {
	var record AnalysisHistoryRecord
	err := r.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, userID).
		First(&record).Error
	if err != nil {
		return nil, err
	}
	return &record, nil
}

// Delete 删除单条记录
func (r *Repository) Delete(ctx context.Context, userID, id string) error {
	return r.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, userID).
		Delete(&AnalysisHistoryRecord{}).Error
}

// ── 转换方法 ──

func (r AnalysisHistoryRecord) ToListItem() HistoryListItem {
	return HistoryListItem{
		ID:              r.ID,
		Symbol:          r.Symbol,
		SymbolName:      r.SymbolName,
		Signal:          r.Signal,
		ConfidenceScore: r.ConfidenceScore,
		CreatedAt:       r.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func (r AnalysisHistoryRecord) ToDetail() (HistoryDetail, error) {
	var result map[string]any
	if r.ResultJSON != "" {
		_ = json.Unmarshal([]byte(r.ResultJSON), &result)
	}
	if result == nil {
		result = map[string]any{}
	}

	var meta map[string]any
	if r.MetaJSON != "" && r.MetaJSON != "{}" {
		_ = json.Unmarshal([]byte(r.MetaJSON), &meta)
	}
	if meta == nil {
		meta = map[string]any{}
	}

	return HistoryDetail{
		ID:              r.ID,
		Symbol:          r.Symbol,
		SymbolName:      r.SymbolName,
		Signal:          r.Signal,
		ConfidenceScore: r.ConfidenceScore,
		Result:          result,
		Meta:            meta,
		CreatedAt:       r.CreatedAt.UTC().Format(time.RFC3339),
	}, nil
}

// SaveFromAPIResponse 从 AI 分析 API 响应中构造并保存记录。
// symbol / symbolName 由调用方显式传入，不依赖响应体中的 symbol_meta。
func (r *Repository) SaveFromAPIResponse(ctx context.Context, userID, symbol, symbolName string, respBytes []byte) error {
	// 解析完整响应
	var apiResp struct {
		Analysis *json.RawMessage `json:"analysis"`
		Meta     map[string]any   `json:"meta"`
	}
	if err := json.Unmarshal(respBytes, &apiResp); err != nil {
		return fmt.Errorf("parse api response: %w", err)
	}
	if apiResp.Analysis == nil {
		return fmt.Errorf("empty analysis in response")
	}

	// 解析 analysis 提取 signal / confidence_score
	var analysis struct {
		Signal          string `json:"signal"`
		ConfidenceScore int    `json:"confidence_score"`
	}
	if err := json.Unmarshal(*apiResp.Analysis, &analysis); err != nil {
		return fmt.Errorf("parse analysis: %w", err)
	}

	record := &AnalysisHistoryRecord{
		ID:              generateUUID(),
		UserID:          userID,
		Symbol:          symbol,
		SymbolName:      symbolName,
		Signal:          analysis.Signal,
		ConfidenceScore: analysis.ConfidenceScore,
		ResultJSON:      string(*apiResp.Analysis),
		MetaJSON:        mustMarshal(apiResp.Meta),
		CreatedAt:       time.Now().UTC(),
	}

	return r.Create(ctx, record)
}

// generateUUID 简易 UUID v4（不需要引入额外依赖）
func generateUUID() string {
	b := make([]byte, 16)
	for i := range b {
		b[i] = byte(time.Now().UnixNano()>>uint(i*7) & 0xff)
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func mustMarshal(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
