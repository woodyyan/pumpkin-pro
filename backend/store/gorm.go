package store

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/woodyyan/pumpkin-pro/backend/config"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openGormDB(cfg config.DBConfig) (*gorm.DB, error) {
	switch cfg.Type {
	case "", "sqlite":
		if err := ensureSQLiteDir(cfg.Path); err != nil {
			return nil, err
		}
		return gorm.Open(sqlite.Open(cfg.Path), &gorm.Config{Logger: logger.Default.LogMode(logger.Warn)})
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
