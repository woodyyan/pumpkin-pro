package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/woodyyan/pumpkin-pro/backend/store/live"
	"github.com/woodyyan/pumpkin-pro/backend/store/quadrant"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var shanghaiLocation = time.FixedZone("CST", 8*60*60)

type cliOptions struct {
	DBPath          string
	FromDate        string
	ToDate          string
	Exchange        string
	Limit           int
	Write           bool
	PaddingDays     int
	MaxTradeGapDays int
	TimeoutSeconds  int
	Verbose         bool
}

type dailyBarFetcher interface {
	FetchSymbolDailyBars(ctx context.Context, symbol string, lookbackDays int) ([]live.DailyBar, error)
}

type backfillPlan struct {
	SnapshotID       int64
	Code             string
	Exchange         string
	SnapshotDate     string
	MatchedTradeDate string
	Symbol           string
	OldPrice         float64
	NewPrice         float64
}

type unresolvedSnapshot struct {
	SnapshotID   int64
	Code         string
	Exchange     string
	SnapshotDate string
	Symbol       string
	Reason       string
}

type planSummary struct {
	Candidates    int
	Planned       int
	Unresolved    int
	Updated       int
	FetchFailures int
}

func main() {
	log.SetFlags(0)

	opts, err := parseOptions(os.Args[1:])
	if err != nil {
		log.Fatalf("参数错误: %v", err)
	}

	db, err := openSQLiteDB(opts.DBPath)
	if err != nil {
		log.Fatalf("打开数据库失败: %v", err)
	}

	ctx := context.Background()
	fetcher := live.NewMarketClient()
	now := time.Now().In(shanghaiLocation)

	plans, unresolved, summary, err := buildBackfillPlans(ctx, db, fetcher, now, opts)
	if err != nil {
		log.Fatalf("生成回填计划失败: %v", err)
	}

	printSummary(opts, summary, plans, unresolved)

	if !opts.Write {
		log.Println("当前为 dry-run（默认模式），未写入数据库。确认结果后可追加 --write 执行实际回填。")
		return
	}

	updated, err := applyBackfillPlans(ctx, db, plans)
	if err != nil {
		log.Fatalf("写入回填结果失败: %v", err)
	}

	log.Printf("回填完成：已更新 %d 条排行榜历史快照。", updated)
	if len(unresolved) > 0 {
		log.Printf("仍有 %d 条未解决，请结合 dry-run 输出继续排查。", len(unresolved))
	}
}

func parseOptions(args []string) (cliOptions, error) {
	fs := flag.NewFlagSet("backfill-ranking-snapshots", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var opts cliOptions
	fs.StringVar(&opts.DBPath, "db", "", "pumpkin.db 路径；不传时自动尝试 data/pumpkin.db 等常见位置")
	fs.StringVar(&opts.FromDate, "from", "", "只处理大于等于该日期的快照（YYYY-MM-DD）")
	fs.StringVar(&opts.ToDate, "to", "", "只处理小于等于该日期的快照（YYYY-MM-DD）")
	fs.StringVar(&opts.Exchange, "exchange", "", "只处理指定市场：ASHARE / SSE / SZSE / HKEX")
	fs.IntVar(&opts.Limit, "limit", 0, "最多处理多少条快照（0 表示不限制）")
	fs.BoolVar(&opts.Write, "write", false, "实际写回数据库；默认仅 dry-run")
	fs.IntVar(&opts.PaddingDays, "padding-days", 10, "抓取历史日线时在最早快照日期基础上额外补的天数")
	fs.IntVar(&opts.MaxTradeGapDays, "max-trade-gap-days", 3, "快照日期无日线时，允许向前回退的最大交易日跨度")
	fs.IntVar(&opts.TimeoutSeconds, "timeout-seconds", 15, "单个 symbol 拉取日线的超时时间（秒）")
	fs.BoolVar(&opts.Verbose, "verbose", false, "输出更多 unresolved 明细")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "用法：go run ./cmd/backfill-ranking-snapshots [options]\n\n")
		fmt.Fprintf(fs.Output(), "示例：\n")
		fmt.Fprintf(fs.Output(), "  go run ./cmd/backfill-ranking-snapshots --db ../data/pumpkin.db --from 2026-04-16 --to 2026-04-16\n")
		fmt.Fprintf(fs.Output(), "  go run ./cmd/backfill-ranking-snapshots --db ../data/pumpkin.db --from 2026-04-18 --to 2026-04-18 --max-trade-gap-days 3\n")
		fmt.Fprintf(fs.Output(), "  go run ./cmd/backfill-ranking-snapshots --db ../data/pumpkin.db --exchange HKEX --write\n\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	if opts.PaddingDays < 0 {
		return opts, errors.New("padding-days 不能为负数")
	}
	if opts.MaxTradeGapDays < 0 {
		return opts, errors.New("max-trade-gap-days 不能为负数")
	}
	if opts.TimeoutSeconds < 0 {
		return opts, errors.New("timeout-seconds 不能为负数")
	}
	if opts.Limit < 0 {
		return opts, errors.New("limit 不能为负数")
	}
	if err := validateDate(opts.FromDate); err != nil {
		return opts, fmt.Errorf("from 日期不合法: %w", err)
	}
	if err := validateDate(opts.ToDate); err != nil {
		return opts, fmt.Errorf("to 日期不合法: %w", err)
	}
	if opts.FromDate != "" && opts.ToDate != "" && opts.FromDate > opts.ToDate {
		return opts, errors.New("from 不能晚于 to")
	}
	if opts.Exchange != "" {
		opts.Exchange = strings.ToUpper(strings.TrimSpace(opts.Exchange))
		switch opts.Exchange {
		case "ASHARE", "SSE", "SZSE", "HKEX":
		default:
			return opts, fmt.Errorf("不支持的 exchange: %s", opts.Exchange)
		}
	}

	resolvedDB, err := resolveDBPath(opts.DBPath)
	if err != nil {
		return opts, err
	}
	opts.DBPath = resolvedDB
	return opts, nil
}

func validateDate(dateStr string) error {
	if strings.TrimSpace(dateStr) == "" {
		return nil
	}
	_, err := time.ParseInLocation("2006-01-02", dateStr, shanghaiLocation)
	return err
}

func resolveDBPath(input string) (string, error) {
	candidates := []string{}
	if strings.TrimSpace(input) != "" {
		candidates = append(candidates, input)
	} else {
		candidates = append(candidates,
			"data/pumpkin.db",
			"../data/pumpkin.db",
			"backend/data/pumpkin.db",
		)
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

func openSQLiteDB(dbPath string) (*gorm.DB, error) {
	dsn := dbPath + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)"
	return gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
}

func buildBackfillPlans(ctx context.Context, db *gorm.DB, fetcher dailyBarFetcher, now time.Time, opts cliOptions) ([]backfillPlan, []unresolvedSnapshot, planSummary, error) {
	rows, err := querySnapshotCandidates(ctx, db, opts)
	if err != nil {
		return nil, nil, planSummary{}, err
	}

	summary := planSummary{Candidates: len(rows)}
	if len(rows) == 0 {
		return nil, nil, summary, nil
	}

	groups := make(map[string][]quadrant.RankingSnapshot)
	unresolved := make([]unresolvedSnapshot, 0)
	for _, row := range rows {
		symbol, buildErr := buildSnapshotSymbol(row.Code, row.Exchange)
		if buildErr != nil {
			unresolved = append(unresolved, unresolvedSnapshot{
				SnapshotID:   row.ID,
				Code:         row.Code,
				Exchange:     row.Exchange,
				SnapshotDate: row.SnapshotDate,
				Reason:       buildErr.Error(),
			})
			continue
		}
		groups[symbol] = append(groups[symbol], row)
	}

	symbols := make([]string, 0, len(groups))
	for symbol := range groups {
		symbols = append(symbols, symbol)
	}
	sort.Strings(symbols)

	plans := make([]backfillPlan, 0, len(rows))
	for _, symbol := range symbols {
		groupRows := groups[symbol]
		fetchCtx := ctx
		cancel := func() {}
		if opts.TimeoutSeconds > 0 {
			fetchCtx, cancel = context.WithTimeout(ctx, time.Duration(opts.TimeoutSeconds)*time.Second)
		}

		groupPlans, groupUnresolved, groupErr := buildPlansForSymbol(fetchCtx, fetcher, symbol, groupRows, now, opts.PaddingDays, opts.MaxTradeGapDays)
		cancel()
		if groupErr != nil {
			summary.FetchFailures += len(groupRows)
			for _, row := range groupRows {
				unresolved = append(unresolved, unresolvedSnapshot{
					SnapshotID:   row.ID,
					Code:         row.Code,
					Exchange:     row.Exchange,
					SnapshotDate: row.SnapshotDate,
					Symbol:       symbol,
					Reason:       groupErr.Error(),
				})
			}
			continue
		}
		plans = append(plans, groupPlans...)
		unresolved = append(unresolved, groupUnresolved...)
	}

	summary.Planned = len(plans)
	summary.Unresolved = len(unresolved)
	return plans, unresolved, summary, nil
}

func querySnapshotCandidates(ctx context.Context, db *gorm.DB, opts cliOptions) ([]quadrant.RankingSnapshot, error) {
	query := db.WithContext(ctx).
		Model(&quadrant.RankingSnapshot{}).
		Where("close_price <= ?", 0).
		Order("snapshot_date ASC, exchange ASC, rank ASC, id ASC")

	if opts.FromDate != "" {
		query = query.Where("snapshot_date >= ?", opts.FromDate)
	}
	if opts.ToDate != "" {
		query = query.Where("snapshot_date <= ?", opts.ToDate)
	}
	if opts.Exchange != "" {
		switch opts.Exchange {
		case "ASHARE":
			query = query.Where("exchange IN ?", []string{"SSE", "SZSE"})
		default:
			query = query.Where("exchange = ?", opts.Exchange)
		}
	}
	if opts.Limit > 0 {
		query = query.Limit(opts.Limit)
	}

	var rows []quadrant.RankingSnapshot
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func buildSnapshotSymbol(code, exchange string) (string, error) {
	normalizedCode := strings.ToUpper(strings.TrimSpace(code))
	normalizedExchange := strings.ToUpper(strings.TrimSpace(exchange))
	if normalizedCode == "" {
		return "", errors.New("空股票代码")
	}

	switch normalizedExchange {
	case "HKEX":
		if len(normalizedCode) < 5 {
			normalizedCode = strings.Repeat("0", 5-len(normalizedCode)) + normalizedCode
		}
		return normalizedCode + ".HK", nil
	case "SSE":
		return normalizedCode + ".SH", nil
	case "SZSE":
		return normalizedCode + ".SZ", nil
	case "":
		symbol, _, err := live.NormalizeSymbol(normalizedCode)
		return symbol, err
	default:
		return "", fmt.Errorf("不支持的交易所: %s", normalizedExchange)
	}
}

func buildPlansForSymbol(ctx context.Context, fetcher dailyBarFetcher, symbol string, rows []quadrant.RankingSnapshot, now time.Time, paddingDays int, maxTradeGapDays int) ([]backfillPlan, []unresolvedSnapshot, error) {
	lookbackDays, err := computeLookbackDays(rows, now, paddingDays)
	if err != nil {
		return nil, nil, err
	}

	bars, err := fetcher.FetchSymbolDailyBars(ctx, symbol, lookbackDays)
	if err != nil {
		return nil, nil, fmt.Errorf("拉取 %s 日线失败: %w", symbol, err)
	}

	closeByDate := make(map[string]float64, len(bars))
	for _, bar := range bars {
		if bar.Close > 0 {
			closeByDate[bar.Date] = bar.Close
		}
	}

	plans := make([]backfillPlan, 0, len(rows))
	unresolved := make([]unresolvedSnapshot, 0)
	for _, row := range rows {
		matchedTradeDate, closePrice := resolveClosePrice(row.SnapshotDate, closeByDate, maxTradeGapDays)
		if closePrice <= 0 {
			unresolved = append(unresolved, unresolvedSnapshot{
				SnapshotID:   row.ID,
				Code:         row.Code,
				Exchange:     row.Exchange,
				SnapshotDate: row.SnapshotDate,
				Symbol:       symbol,
				Reason:       fmt.Sprintf("未在历史日线中找到可用收盘价（向前回退 %d 天内）", maxTradeGapDays),
			})
			continue
		}
		plans = append(plans, backfillPlan{
			SnapshotID:       row.ID,
			Code:             row.Code,
			Exchange:         row.Exchange,
			SnapshotDate:     row.SnapshotDate,
			MatchedTradeDate: matchedTradeDate,
			Symbol:           symbol,
			OldPrice:         row.ClosePrice,
			NewPrice:         closePrice,
		})
	}
	return plans, unresolved, nil
}

func resolveClosePrice(snapshotDate string, closeByDate map[string]float64, maxTradeGapDays int) (string, float64) {
	date, err := time.ParseInLocation("2006-01-02", snapshotDate, shanghaiLocation)
	if err != nil {
		return "", 0
	}
	for gap := 0; gap <= maxTradeGapDays; gap++ {
		candidateDate := date.AddDate(0, 0, -gap).Format("2006-01-02")
		if closePrice := closeByDate[candidateDate]; closePrice > 0 {
			return candidateDate, closePrice
		}
	}
	return "", 0
}

func computeLookbackDays(rows []quadrant.RankingSnapshot, now time.Time, paddingDays int) (int, error) {
	if len(rows) == 0 {
		return 0, errors.New("空快照列表")
	}

	var minDate time.Time
	for i, row := range rows {
		date, err := time.ParseInLocation("2006-01-02", row.SnapshotDate, shanghaiLocation)
		if err != nil {
			return 0, fmt.Errorf("解析 snapshot_date=%s 失败: %w", row.SnapshotDate, err)
		}
		if i == 0 || date.Before(minDate) {
			minDate = date
		}
	}

	diffDays := int(math.Ceil(now.Sub(minDate).Hours()/24.0)) + paddingDays + 1
	if diffDays < 120 {
		diffDays = 120
	}
	if diffDays > 5000 {
		diffDays = 5000
	}
	return diffDays, nil
}

func applyBackfillPlans(ctx context.Context, db *gorm.DB, plans []backfillPlan) (int, error) {
	if len(plans) == 0 {
		return 0, nil
	}

	updated := 0
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, plan := range plans {
			result := tx.Model(&quadrant.RankingSnapshot{}).
				Where("id = ?", plan.SnapshotID).
				Update("close_price", plan.NewPrice)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected > 0 {
				updated++
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return updated, nil
}

func printSummary(opts cliOptions, summary planSummary, plans []backfillPlan, unresolved []unresolvedSnapshot) {
	mode := "dry-run"
	if opts.Write {
		mode = "write"
	}
	log.Printf("模式: %s", mode)
	log.Printf("数据库: %s", opts.DBPath)
	if opts.FromDate != "" || opts.ToDate != "" {
		log.Printf("日期过滤: from=%s to=%s", emptyAsAll(opts.FromDate), emptyAsAll(opts.ToDate))
	}
	if opts.Exchange != "" {
		log.Printf("市场过滤: %s", opts.Exchange)
	}
	if opts.Limit > 0 {
		log.Printf("数量限制: %d", opts.Limit)
	}
	log.Printf("候选快照: %d，计划回填: %d，未解决: %d，抓取失败: %d", summary.Candidates, summary.Planned, summary.Unresolved, summary.FetchFailures)

	for i, plan := range plans {
		if i >= 8 {
			break
		}
		tradeDateNote := plan.MatchedTradeDate
		if tradeDateNote == "" {
			tradeDateNote = plan.SnapshotDate
		}
		log.Printf("PLAN[%d] %s %s %s %s (trade=%s): %.2f -> %.2f", i+1, plan.SnapshotDate, plan.Exchange, plan.Code, plan.Symbol, tradeDateNote, plan.OldPrice, plan.NewPrice)
	}

	if len(unresolved) > 0 {
		maxLines := 8
		if opts.Verbose {
			maxLines = len(unresolved)
		}
		for i, item := range unresolved {
			if i >= maxLines {
				break
			}
			log.Printf("MISS[%d] %s %s %s %s: %s", i+1, item.SnapshotDate, item.Exchange, item.Code, item.Symbol, item.Reason)
		}
		if !opts.Verbose && len(unresolved) > maxLines {
			log.Printf("还有 %d 条 unresolved 未展开；如需全部查看请加 --verbose", len(unresolved)-maxLines)
		}
	}
}

func emptyAsAll(v string) string {
	if strings.TrimSpace(v) == "" {
		return "ALL"
	}
	return v
}
