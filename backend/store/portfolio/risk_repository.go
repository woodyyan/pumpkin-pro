package portfolio

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// RiskDBRepository 风险数据库仓库（访问quadrant_cache和quadrant_cache_hk）
type RiskDBRepository struct {
	cacheDB   *gorm.DB // quadrant_cache.db（A股）
	hkCacheDB *gorm.DB // quadrant_cache_hk.db（港股）
}

// NewRiskDBRepository 创建风险数据库仓库
func NewRiskDBRepository() (*RiskDBRepository, error) {
	return NewRiskDBRepositoryFromPaths("data/quant", "data/quant")
}

// NewRiskDBRepositoryFromPaths 按显式缓存目录创建风险数据库仓库
func NewRiskDBRepositoryFromPaths(cacheADir string, cacheHKDir string) (*RiskDBRepository, error) {
	cacheADir = strings.TrimSpace(cacheADir)
	if cacheADir == "" {
		cacheADir = "data/quant"
	}
	cacheHKDir = strings.TrimSpace(cacheHKDir)
	if cacheHKDir == "" {
		cacheHKDir = cacheADir
	}

	cachePath := filepath.Join(cacheADir, "quadrant_cache.db")
	cacheDB, err := openCacheDB(cachePath)
	if err != nil {
		return nil, fmt.Errorf("打开A股缓存数据库失败: %w", err)
	}

	hkCachePath := filepath.Join(cacheHKDir, "quadrant_cache_hk.db")
	hkCacheDB, err := openCacheDB(hkCachePath)
	if err != nil {
		// 如果港股数据库不存在，可能是旧版本，只记录警告
		log.Printf("警告: 港股缓存数据库不存在，跳过港股数据: %v", err)
		hkCacheDB = nil
	}

	return &RiskDBRepository{
		cacheDB:   cacheDB,
		hkCacheDB: hkCacheDB,
	}, nil
}

func (r *RiskDBRepository) CacheDB() *gorm.DB {
	if r == nil {
		return nil
	}
	return r.cacheDB
}

func (r *RiskDBRepository) HKCacheDB() *gorm.DB {
	if r == nil {
		return nil
	}
	return r.hkCacheDB
}

// openCacheDB 打开只读SQLite数据库
func openCacheDB(dbPath string) (*gorm.DB, error) {
	// 只读模式：避免写操作
	dsn := dbPath + "?_pragma=journal_mode(OFF)&_pragma=synchronous(OFF)&_pragma=locking_mode(NORMAL)&mode=ro"

	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("打开数据库 %s 失败: %w", dbPath, err)
	}

	// 设置连接池为只读（最大连接数可适当增加）
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("获取底层数据库连接失败: %w", err)
	}
	sqlDB.SetMaxOpenConns(5)
	sqlDB.SetMaxIdleConns(2)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)

	log.Printf("[风险仓库] 缓存数据库已打开: %s", dbPath)
	return db, nil
}

// DailyBarRecord 日线数据记录
type DailyBarRecord struct {
	Code         string  `gorm:"column:code;size:10"`
	Date         string  `gorm:"column:date;size:10"`
	Open         float64 `gorm:"column:open"`
	Close        float64 `gorm:"column:close"`
	High         float64 `gorm:"column:high"`
	Low          float64 `gorm:"column:low"`
	Volume       float64 `gorm:"column:volume"`
	TurnoverRate float64 `gorm:"column:turnover_rate"`
}

func (DailyBarRecord) TableName() string {
	return "daily_bars"
}

// GetDailyBars 获取股票的历史日线数据
func (r *RiskDBRepository) GetDailyBars(ctx context.Context, symbols []string, startDate, endDate string) (map[string][]DailyBarRecord, error) {
	result := make(map[string][]DailyBarRecord)

	// 分批查询A股和港股
	var ashareSymbols []string
	var hkSymbols []string

	for _, symbol := range symbols {
		// 简单判断：港股代码通常是5位数字，或以"hk"开头
		// 实际应根据exchange字段判断，这里简化处理
		if len(symbol) == 5 && isNumeric(symbol) {
			hkSymbols = append(hkSymbols, symbol)
		} else {
			ashareSymbols = append(ashareSymbols, symbol)
		}
	}

	// 查询A股数据
	if len(ashareSymbols) > 0 && r.cacheDB != nil {
		ashareBars, err := r.queryDailyBars(ctx, r.cacheDB, ashareSymbols, startDate, endDate)
		if err != nil {
			return nil, fmt.Errorf("查询A股日线失败: %w", err)
		}
		for k, v := range ashareBars {
			result[k] = v
		}
	}

	// 查询港股数据
	if len(hkSymbols) > 0 && r.hkCacheDB != nil {
		hkBars, err := r.queryDailyBars(ctx, r.hkCacheDB, hkSymbols, startDate, endDate)
		if err != nil {
			return nil, fmt.Errorf("查询港股日线失败: %w", err)
		}
		for k, v := range hkBars {
			result[k] = v
		}
	}

	return result, nil
}

// queryDailyBars 在指定数据库中查询日线数据
func (r *RiskDBRepository) queryDailyBars(ctx context.Context, db *gorm.DB, symbols []string, startDate, endDate string) (map[string][]DailyBarRecord, error) {
	if len(symbols) == 0 {
		return nil, nil
	}

	var records []DailyBarRecord
	query := db.WithContext(ctx).
		Where("code IN (?)", symbols).
		Order("code, date")

	if startDate != "" {
		query = query.Where("date >= ?", startDate)
	}
	if endDate != "" {
		query = query.Where("date <= ?", endDate)
	}

	if err := query.Find(&records).Error; err != nil {
		return nil, fmt.Errorf("数据库查询失败: %w", err)
	}

	// 按股票代码分组
	result := make(map[string][]DailyBarRecord)
	for _, record := range records {
		result[record.Code] = append(result[record.Code], record)
	}

	return result, nil
}

// GetLatestDailyBar 获取股票的最新日线数据
func (r *RiskDBRepository) GetLatestDailyBar(ctx context.Context, symbol string) (*DailyBarRecord, error) {
	// 判断是A股还是港股
	var db *gorm.DB
	if len(symbol) == 5 && isNumeric(symbol) {
		db = r.hkCacheDB
	} else {
		db = r.cacheDB
	}

	if db == nil {
		return nil, fmt.Errorf("数据库未初始化")
	}

	var record DailyBarRecord
	err := db.WithContext(ctx).
		Where("code = ?", symbol).
		Order("date DESC").
		First(&record).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("查询最新日线失败: %w", err)
	}

	return &record, nil
}

// GetDailyBarsForPeriod 获取指定时间段内的日线数据（默认最近252个交易日）
func (r *RiskDBRepository) GetDailyBarsForPeriod(ctx context.Context, symbols []string, days int) (map[string][]DailyBarRecord, error) {
	if days <= 0 {
		days = 252 // 默认一年交易日
	}

	// 计算开始日期（近似）
	endDate := time.Now().Format("2006-01-02")
	startDate := time.Now().AddDate(0, 0, -days*2).Format("2006-01-02") // 宽松一些

	return r.GetDailyBars(ctx, symbols, startDate, endDate)
}

// isNumeric 判断字符串是否全为数字
func isNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// Close 关闭数据库连接
func (r *RiskDBRepository) Close() error {
	var errs []error

	if r.cacheDB != nil {
		sqlDB, err := r.cacheDB.DB()
		if err == nil {
			if err := sqlDB.Close(); err != nil {
				errs = append(errs, fmt.Errorf("关闭A股数据库失败: %w", err))
			}
		}
	}

	if r.hkCacheDB != nil {
		sqlDB, err := r.hkCacheDB.DB()
		if err == nil {
			if err := sqlDB.Close(); err != nil {
				errs = append(errs, fmt.Errorf("关闭港股数据库失败: %w", err))
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("关闭风险数据库时发生错误: %v", errs)
	}
	return nil
}
