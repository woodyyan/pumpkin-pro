package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/woodyyan/pumpkin-pro/backend/config"
	"github.com/woodyyan/pumpkin-pro/backend/store"
	"github.com/woodyyan/pumpkin-pro/backend/store/portfolio"
)

type cliOptions struct {
	DBPath        string
	Scope         string
	TargetDate    string
	UserID        string
	ScheduledTime string
	TriggerSource string
}

func main() {
	log.SetFlags(0)
	opts, err := parseOptions(os.Args[1:])
	if err != nil {
		log.Fatalf("参数错误: %v", err)
	}

	cfg := config.DBConfig{Type: "sqlite", Path: opts.DBPath}
	storeInstance, err := store.New(cfg)
	if err != nil {
		log.Fatalf("打开数据库失败: %v", err)
	}

	service := portfolio.NewService(portfolio.NewRepository(storeInstance.DB))
	ctx := context.Background()
	if strings.TrimSpace(opts.UserID) != "" {
		ok, rebuildErr := service.RebuildDailySnapshotForUser(ctx, opts.UserID, opts.Scope, opts.TargetDate, portfolio.PortfolioSnapshotSourceManualRebuild, "")
		if rebuildErr != nil {
			log.Fatalf("重建用户快照失败: %v", rebuildErr)
		}
		if ok {
			log.Printf("用户 %s %s %s 快照重建完成", opts.UserID, opts.Scope, opts.TargetDate)
		} else {
			log.Printf("用户 %s %s %s 无需写入快照", opts.UserID, opts.Scope, opts.TargetDate)
		}
		return
	}

	scheduledTime, err := parseScheduledTime(opts.ScheduledTime)
	if err != nil {
		log.Fatalf("scheduled-time 参数错误: %v", err)
	}
	jobRun, err := service.RunDailyMarketSnapshot(ctx, opts.Scope, opts.TargetDate, scheduledTime, opts.TriggerSource)
	if err != nil {
		log.Fatalf("执行市场快照任务失败: %v", err)
	}
	log.Printf("任务完成: id=%s scope=%s date=%s status=%s users=%d written=%d failed=%d", jobRun.ID, jobRun.Scope, jobRun.TargetDate, jobRun.Status, jobRun.UserCountTotal, jobRun.SnapshotCountWritten, jobRun.UserCountFailed)
	if strings.TrimSpace(jobRun.Message) != "" {
		log.Printf("任务消息: %s", jobRun.Message)
	}
}

func parseOptions(args []string) (cliOptions, error) {
	fs := flag.NewFlagSet("rebuild-portfolio-daily-snapshots", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var opts cliOptions
	fs.StringVar(&opts.DBPath, "db", "", "pumpkin.db 路径；不传时自动尝试 data/pumpkin.db 等常见位置")
	fs.StringVar(&opts.Scope, "scope", portfolio.PortfolioScopeAShare, "目标市场：ASHARE 或 HKEX")
	fs.StringVar(&opts.TargetDate, "date", "", "快照日期，格式 YYYY-MM-DD；默认当天（北京时间）")
	fs.StringVar(&opts.UserID, "user", "", "只重建单个用户；为空时执行市场级任务")
	fs.StringVar(&opts.ScheduledTime, "scheduled-time", "", "任务计划触发时间，支持 RFC3339；默认当前时间")
	fs.StringVar(&opts.TriggerSource, "trigger-source", portfolio.PortfolioSnapshotJobTriggerManual, "任务触发来源")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "用法：go run ./cmd/rebuild-portfolio-daily-snapshots [options]\n\n")
		fmt.Fprintf(fs.Output(), "示例：\n")
		fmt.Fprintf(fs.Output(), "  go run ./cmd/rebuild-portfolio-daily-snapshots --db ../data/pumpkin.db --scope ASHARE --date 2026-04-21\n")
		fmt.Fprintf(fs.Output(), "  go run ./cmd/rebuild-portfolio-daily-snapshots --db ../data/pumpkin.db --scope HKEX --date 2026-04-21 --user demo-user\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	resolvedDB, err := resolveDBPath(opts.DBPath)
	if err != nil {
		return opts, err
	}
	opts.DBPath = resolvedDB
	opts.Scope = normalizeScope(opts.Scope)
	if opts.Scope != portfolio.PortfolioScopeAShare && opts.Scope != portfolio.PortfolioScopeHK {
		return opts, fmt.Errorf("不支持的 scope: %s", opts.Scope)
	}
	if err := validateDate(opts.TargetDate); err != nil {
		return opts, fmt.Errorf("date 日期不合法: %w", err)
	}
	if strings.TrimSpace(opts.TriggerSource) == "" {
		opts.TriggerSource = portfolio.PortfolioSnapshotJobTriggerManual
	}
	return opts, nil
}

func validateDate(dateStr string) error {
	if strings.TrimSpace(dateStr) == "" {
		return nil
	}
	_, err := time.ParseInLocation("2006-01-02", dateStr, time.FixedZone("CST", 8*60*60))
	return err
}

func resolveDBPath(input string) (string, error) {
	candidates := []string{}
	if strings.TrimSpace(input) != "" {
		candidates = append(candidates, input)
	} else {
		candidates = append(candidates, "data/pumpkin.db", "../data/pumpkin.db", "backend/data/pumpkin.db")
	}
	for _, candidate := range candidates {
		abs, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		if _, statErr := os.Stat(abs); statErr == nil {
			return abs, nil
		}
	}
	if strings.TrimSpace(input) != "" {
		return "", fmt.Errorf("数据库文件不存在: %s", input)
	}
	return "", errors.New("未找到 pumpkin.db，请显式传 --db")
}

func normalizeScope(scope string) string {
	switch strings.ToUpper(strings.TrimSpace(scope)) {
	case "", portfolio.PortfolioScopeAShare, "SSE", "SZSE", "ASHARES":
		return portfolio.PortfolioScopeAShare
	case portfolio.PortfolioScopeHK, "HKSHARES":
		return portfolio.PortfolioScopeHK
	default:
		return strings.ToUpper(strings.TrimSpace(scope))
	}
}

func parseScheduledTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, err
	}
	return parsed, nil
}
