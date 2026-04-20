package store

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"gorm.io/gorm"
)

// ── AI 调用日志（DB 写入器，由 strategy.LogAICall 回调触发）──

// AICallLog 记录每次 LLM 调用的详细信息（GORM model）
type AICallLog struct {
	ID               uint      `gorm:"primaryKey;autoIncrement" json:"-"`
	UserID           string    `gorm:"size:64;not null;default:'';index:idx_ai_call_user" json:"user_id"`
	FeatureKey       string    `gorm:"size:40;not null;index:idx_ai_call_feature" json:"feature_key"`
	FeatureName      string    `gorm:"size:64;not null;default:''" json:"feature_name"`
	Status           string    `gorm:"size:20;not null;default:'success'" json:"status"`
	ErrorMessage     string    `gorm:"size:512;default:''" json:"error_message,omitempty"`
	Model            string    `gorm:"size:128;default:''" json:"model,omitempty"`
	ResponseMS       int       `gorm:"not null;default:0" json:"response_ms"`
	PromptTokens     int       `gorm:"not null;default:0" json:"prompt_tokens"`
	CompletionTokens int       `gorm:"not null;default:0" json:"completion_tokens"`
	TotalTokens      int       `gorm:"not null;default:0" json:"total_tokens"`
	ExtraMeta        string    `gorm:"type:text;default:'{}'" json:"extra_meta,omitempty"`
	CreatedAt        time.Time `gorm:"not null;autoCreateTime" json:"created_at"`
}

func (AICallLog) TableName() string { return "ai_call_logs" }

type pragmaColumn struct {
	Name string `gorm:"column:name"`
}

// createAICallLogsTable 使用 raw SQL 创建/升级 ai_call_logs 表（避免 glebarez/sqlite AutoMigrate SIGSEGV）
func createAICallLogsTable(db *gorm.DB) error {
	createTableSQL := `CREATE TABLE IF NOT EXISTS ai_call_logs (` +
		`id INTEGER PRIMARY KEY AUTOINCREMENT, ` +
		`user_id TEXT NOT NULL DEFAULT '', ` +
		`feature_key TEXT NOT NULL, ` +
		`feature_name TEXT NOT NULL DEFAULT '', ` +
		`status TEXT NOT NULL DEFAULT 'success', ` +
		`error_message TEXT DEFAULT '', ` +
		`model TEXT DEFAULT '', ` +
		`response_ms INTEGER NOT NULL DEFAULT 0, ` +
		`prompt_tokens INTEGER NOT NULL DEFAULT 0, ` +
		`completion_tokens INTEGER NOT NULL DEFAULT 0, ` +
		`total_tokens INTEGER NOT NULL DEFAULT 0, ` +
		`extra_meta TEXT DEFAULT '{}', ` +
		`created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP` +
		`)`
	if err := db.Exec(createTableSQL).Error; err != nil {
		return fmt.Errorf("exec %q failed: %w", createTableSQL[:40], err)
	}

	var columns []pragmaColumn
	if err := db.Raw("PRAGMA table_info(ai_call_logs)").Scan(&columns).Error; err != nil {
		return fmt.Errorf("load ai_call_logs columns failed: %w", err)
	}
	columnSet := make(map[string]bool, len(columns))
	for _, col := range columns {
		columnSet[col.Name] = true
	}

	alterSQLs := []struct {
		column string
		sql    string
	}{
		{column: "prompt_tokens", sql: "ALTER TABLE ai_call_logs ADD COLUMN prompt_tokens INTEGER NOT NULL DEFAULT 0"},
		{column: "completion_tokens", sql: "ALTER TABLE ai_call_logs ADD COLUMN completion_tokens INTEGER NOT NULL DEFAULT 0"},
		{column: "total_tokens", sql: "ALTER TABLE ai_call_logs ADD COLUMN total_tokens INTEGER NOT NULL DEFAULT 0"},
	}
	for _, item := range alterSQLs {
		if columnSet[item.column] {
			continue
		}
		if err := db.Exec(item.sql).Error; err != nil {
			return fmt.Errorf("exec %q failed: %w", item.sql, err)
		}
	}

	indexSQLs := []string{
		"CREATE INDEX IF NOT EXISTS idx_ai_call_user ON ai_call_logs(user_id)",
		"CREATE INDEX IF NOT EXISTS idx_ai_call_feature ON ai_call_logs(feature_key)",
		"CREATE INDEX IF NOT EXISTS idx_ai_call_created_at ON ai_call_logs(created_at)",
		"CREATE INDEX IF NOT EXISTS idx_ai_call_user_created_at ON ai_call_logs(user_id, created_at)",
		"CREATE INDEX IF NOT EXISTS idx_ai_call_feature_created_at ON ai_call_logs(feature_key, created_at)",
	}
	for _, s := range indexSQLs {
		if err := db.Exec(s).Error; err != nil {
			return fmt.Errorf("exec %q failed: %w", s, err)
		}
	}
	return nil
}

// ── 异步批量写入器 ──

type aiLogWriter struct {
	db   *gorm.DB
	ch   chan aiLogEntryInternal
	once sync.Once
}

type aiLogEntryInternal struct {
	UserID           string
	FeatureKey       string
	FeatureName      string
	Status           string
	ErrorMessage     string
	Model            string
	ResponseMS       int
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	ExtraMeta        map[string]any
}

var globalAIWriter *aiLogWriter

// InitAILogger 初始化全局 AI 日志写入器（main.go 启动时调用）
func InitAILogger(db *gorm.DB) {
	w := &aiLogWriter{
		db: db,
		ch: make(chan aiLogEntryInternal, 256),
	}
	globalAIWriter = w
	go w.loop()
	log.Println("[ai-log] logger initialized")
}

// WriteAICallBatch 内部方法：将 entry 放入缓冲通道（由 strategy.LogAICall 调用）
func WriteAICallBatch(userID, featureKey, featureName, status, errMsg, model string, responseMs, promptTokens, completionTokens, totalTokens int, extraMeta map[string]any) {
	if globalAIWriter == nil {
		return
	}
	select {
	case globalAIWriter.ch <- aiLogEntryInternal{
		UserID:           userID,
		FeatureKey:       featureKey,
		FeatureName:      featureName,
		Status:           status,
		ErrorMessage:     errMsg,
		Model:            model,
		ResponseMS:       responseMs,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
		ExtraMeta:        extraMeta,
	}:
	default:
		log.Printf("[ai-log] warning: buffer full, dropping entry for feature=%s", featureKey)
	}
}

func (w *aiLogWriter) loop() {
	batch := make([]aiLogEntryInternal, 0, 32)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case entry, ok := <-w.ch:
			if !ok {
				w.flush(batch)
				return
			}
			batch = append(batch, entry)
			if len(batch) >= 32 {
				w.flush(batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				w.flush(batch)
				batch = batch[:0]
			}
		}
	}
}

func (w *aiLogWriter) flush(batch []aiLogEntryInternal) {
	if len(batch) == 0 {
		return
	}
	logs := make([]AICallLog, len(batch))
	for i, e := range batch {
		metaJSON := "{}"
		if e.ExtraMeta != nil {
			if b, err := json.Marshal(e.ExtraMeta); err == nil {
				metaJSON = string(b)
			}
		}
		status := e.Status
		if status == "" {
			status = "success"
		}
		logs[i] = AICallLog{
			UserID:           e.UserID,
			FeatureKey:       e.FeatureKey,
			FeatureName:      e.FeatureName,
			Status:           status,
			ErrorMessage:     e.ErrorMessage,
			Model:            e.Model,
			ResponseMS:       e.ResponseMS,
			PromptTokens:     e.PromptTokens,
			CompletionTokens: e.CompletionTokens,
			TotalTokens:      e.TotalTokens,
			ExtraMeta:        metaJSON,
		}
	}
	if err := w.db.Create(&logs).Error; err != nil {
		log.Printf("[ai-log] batch insert failed (%d entries): %v", len(logs), err)
	} else {
		log.Printf("[ai-log] flushed %d entries", len(logs))
	}
}
