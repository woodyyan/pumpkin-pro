package store

import (
	"fmt"

	"github.com/woodyyan/pumpkin-pro/backend/config"
	"github.com/woodyyan/pumpkin-pro/backend/store/admin"
	"github.com/woodyyan/pumpkin-pro/backend/store/analytics"
	"github.com/woodyyan/pumpkin-pro/backend/store/auth"
	"github.com/woodyyan/pumpkin-pro/backend/store/backtest"
	"github.com/woodyyan/pumpkin-pro/backend/store/fundcache"
	"github.com/woodyyan/pumpkin-pro/backend/store/feedback"
	"github.com/woodyyan/pumpkin-pro/backend/store/live"
	"github.com/woodyyan/pumpkin-pro/backend/store/portfolio"
	"github.com/woodyyan/pumpkin-pro/backend/store/quadrant"
	"github.com/woodyyan/pumpkin-pro/backend/store/screener"
	"github.com/woodyyan/pumpkin-pro/backend/store/signal"
	"github.com/woodyyan/pumpkin-pro/backend/store/strategy"
	"gorm.io/gorm"
)

type Migrator interface {
	Name() string
	AutoMigrate(db *gorm.DB) error
}

type Store struct {
	DB *gorm.DB
}

func New(cfg config.DBConfig) (*Store, error) {
	db, err := openGormDB(cfg)
	if err != nil {
		return nil, fmt.Errorf("open gorm db failed: %w", err)
	}

	migrators := []Migrator{
		auth.NewMigrator(),
		strategy.NewMigrator(),
		live.NewMigrator(),
		signal.NewMigrator(),
		admin.NewMigrator(),
		backtest.NewMigrator(),
		portfolio.NewMigrator(),
		quadrant.NewMigrator(),
		screener.NewMigrator(),
		analytics.NewMigrator(),
		feedback.NewMigrator(),
		fundcache.NewMigrator(),
	}
	for _, migrator := range migrators {
		if err := migrator.AutoMigrate(db); err != nil {
			return nil, fmt.Errorf("auto migrate %s failed: %w", migrator.Name(), err)
		}
	}

	// AI 调用日志表（使用 raw SQL 建表，避免 glebarez/sqlite 在处理此 struct 时偶发 SIGSEGV）
	if err := createAICallLogsTable(db); err != nil {
		return nil, fmt.Errorf("auto migrate ai_call_logs failed: %w", err)
	}

	return &Store{DB: db}, nil
}
