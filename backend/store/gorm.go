package store

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/glebarez/sqlite"
	"github.com/woodyyan/pumpkin-pro/backend/config"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openGormDB(cfg config.DBConfig) (*gorm.DB, error) {
	switch cfg.Type {
	case "", "sqlite":
		if err := ensureSQLiteDir(cfg.Path); err != nil {
			return nil, err
		}
		// WAL mode for concurrent reads, busy_timeout to avoid SQLITE_BUSY,
		// synchronous=NORMAL is safe under WAL and faster than FULL.
		dsn := cfg.Path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)"
		db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Warn)})
		if err != nil {
			return nil, err
		}
		// SQLite is single-writer; limit pool to 1 connection to serialise writes
		// and avoid lock contention between goroutines.
		sqlDB, sqlErr := db.DB()
		if sqlErr != nil {
			return nil, sqlErr
		}
		sqlDB.SetMaxOpenConns(1)
		sqlDB.SetMaxIdleConns(1)
		sqlDB.SetConnMaxLifetime(0)
		log.Printf("[store] SQLite opened: WAL mode, busy_timeout=5000ms, path=%s", cfg.Path)
		return db, nil
	case "postgres":
		dsn := fmt.Sprintf(
			"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s TimeZone=UTC",
			cfg.Host,
			cfg.Port,
			cfg.User,
			cfg.Password,
			cfg.Name,
			cfg.SSLMode,
		)
		return gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Warn)})
	default:
		return nil, fmt.Errorf("unsupported DB_TYPE: %s", cfg.Type)
	}
}

func ensureSQLiteDir(dbPath string) error {
	dir := filepath.Dir(dbPath)
	if dir == "" || dir == "." {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}
