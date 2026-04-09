package admin

import (
	"fmt"

	"gorm.io/gorm"
)

type Migrator struct{}

func NewMigrator() Migrator {
	return Migrator{}
}

func (Migrator) Name() string {
	return "admin"
}

func (Migrator) AutoMigrate(db *gorm.DB) error {
	if err := db.AutoMigrate(&SuperAdminRecord{}); err != nil {
		return err
	}
	// api_errors uses raw SQL to avoid potential GORM+SQLite issues with large tables
	if err := ensureAPIErrorsTable(db); err != nil {
		return err
	}
	return nil
}

// ensureAPIErrorsTable creates the api_errors table if it does not exist.
func ensureAPIErrorsTable(db *gorm.DB) error {
	// Suppress the CREATE TABLE log output on every startup
	sql := `
CREATE TABLE IF NOT EXISTS api_errors (
	id              INTEGER PRIMARY KEY AUTOINCREMENT,
	method          TEXT    NOT NULL DEFAULT '',
	path            TEXT    NOT NULL DEFAULT '',
	query_params    TEXT    DEFAULT '',
	status_code     INTEGER NOT NULL DEFAULT 0,
	error_code      TEXT    DEFAULT '',
	error_message   TEXT    DEFAULT '',
	duration_ms     INTEGER NOT NULL DEFAULT 0,
	client_ip       TEXT    DEFAULT '',
	user_agent      TEXT    DEFAULT '',
	user_id         TEXT    DEFAULT '',
	created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_api_errors_path ON api_errors(path);
CREATE INDEX IF NOT EXISTS idx_api_errors_status ON api_errors(status_code);
CREATE INDEX IF NOT EXISTS idx_api_errors_created ON api_errors(created_at);
CREATE INDEX IF NOT EXISTS idx_api_errors_code ON api_errors(error_code);
`
	if err := db.Exec(sql).Error; err != nil {
		return fmt.Errorf("create api_errors table: %w", err)
	}
	return nil
}
