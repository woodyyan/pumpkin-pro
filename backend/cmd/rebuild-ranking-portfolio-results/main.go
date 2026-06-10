// Command rebuild-ranking-portfolio-results rebuilds materialized ranking
// portfolio results from historical snapshots.
//
// ⚠️ 口径说明（重要）：
// 本工具的净值序列(series)仍使用历史 close→close 口径（见 calculatePeriodReturn），
// 这是「开盘价建仓」改造之前的旧口径。新版模拟组合已改为「T+1 9:25 开盘价建仓、
// 当日收盘价估值」(open→close)，并采用 cut-over 策略：抛弃历史曲线、从新算法上线日
// D0 起重新计算。
//
// 因此：本工具仅用于历史数据的近似回算（plan B），不应作为新口径模拟组合的标准
// 重建路径。若产品明确需要展示 D0 之前的长历史曲线，再使用本工具按日线 Open 近似
// 回补，并在展示层标注「D0 之前为近似回算」。日常运维请勿用本工具刷新当前批次。
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/woodyyan/pumpkin-pro/backend/store/live"
	"github.com/woodyyan/pumpkin-pro/backend/store/quadrant"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

var (
	shanghaiLocation  = time.FixedZone("CST", 8*60*60)
	errDryRunRollback = errors.New("dry-run rollback")
)

const (
	defaultDefinitionID              = "wolong_ai_top4_ex_star_equal_v1"
	defaultDefinitionCode            = "wolong-ai-top4-ex-star-equal"
	defaultPortfolioName             = "模拟组合A"
	defaultMethodNote                = ""
	defaultWarningText               = "当日有效成分股不足 4 只"
	defaultMaxHoldings               = 4
	defaultTradeCostRate             = 0.0002
	rankingPortfolioTradeCostDigits  = 6
	defaultLookbackPaddingDays       = 10
	maxAllowedTradeGapDays           = 3
	rebuildSnapshotHourInShanghai    = 15
	rebuildEffectiveHourInShanghai   = 9
	rebuildEffectiveMinuteInShanghai = 30

	definitionIDAShareB = "wolong_ai_top10_ex_star_by_streak_v1"
	definitionIDHKA     = "wolong_ai_hk_top4_equal_v1"
	definitionIDHKB     = "wolong_ai_hk_top10_by_streak_v1"

	definitionCodeAShareB = "wolong-ai-top10-ex-star-by-streak"
	definitionCodeHKA     = "wolong-ai-hk-top4-equal"
	definitionCodeHKB     = "wolong-ai-hk-top10-by-streak"

	selectionRuleTop4          = "top4"
	selectionRuleTop10ByStreak = "top10_by_consecutive_days"
)

type cliOptions struct {
	DBPath       string
	FromDate     string
	ToDate       string
	Days         int
	DefinitionID string
	Write        bool
	Verbose      bool
}

type runStats struct {
	Candidates int
	Planned    int
	Previewed  int
	Written    int
	Failed     int
}

type rebuildPlan struct {
	Date         string
	SnapshotTime time.Time
	Constituents []quadrant.RankingPortfolioConstituentItem
	MarketPrices []marketPricePlan
	HasShortfall bool
	WarningText  string
	Progress     progressStatus
}

type progressStatus struct {
	Date             string
	SourceCount      int
	SelectedCount    int
	NeededPriceCount int
	SnapshotPriceHit int
	FetchedPriceHit  int
	MissingSymbols   []string
}

type marketPricePlan struct {
	Code           string
	Exchange       string
	ClosePrice     float64
	PriceTradeDate string
}

type sourceSnapshotRow struct {
	ID             int64
	Code           string
	Name           string
	Exchange       string
	Rank           int
	Opportunity    float64
	Risk           float64
	ClosePrice     float64
	PriceTradeDate string
	SnapshotDate   string
}

type priceLookupResult struct {
	ClosePrice float64
	TradeDate  string
}

type stockPriceResolver struct {
	client *live.MarketClient
	cache  map[string][]live.DailyBar
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
	definition, err := loadDefinition(ctx, db, opts.DefinitionID)
	if err != nil {
		log.Fatalf("加载组合定义失败: %v", err)
	}

	targetDates, err := loadTargetDates(ctx, db, resolveDefinitionExchanges(definition.Exchange), opts)
	if err != nil {
		log.Fatalf("加载待重建日期失败: %v", err)
	}
	if len(targetDates) == 0 {
		log.Printf("未找到可重建日期，definition_id=%s", definition.ID)
		return
	}

	marketClient := live.NewMarketClient()
	stockResolver := &stockPriceResolver{client: marketClient, cache: map[string][]live.DailyBar{}}

	log.Printf("开始生成重建计划，definition_id=%s，候选日期=%d，模式=%s", definition.ID, len(targetDates), runModeLabel(opts.Write))

	plans, stats, err := buildPlans(ctx, db, definition, targetDates, stockResolver, opts)
	if err != nil {
		log.Fatalf("生成重建计划失败: %v", err)
	}
	if stats.Failed > 0 {
		log.Printf("计划阶段发现 %d 个失败日期，本次未执行%s。", stats.Failed, actionLabel(opts.Write))
		return
	}
	if len(plans) == 0 {
		log.Println("没有可执行的重建计划。")
		return
	}

	err = applyPlans(ctx, db, definition, plans, opts)
	if err != nil && !errors.Is(err, errDryRunRollback) {
		log.Fatalf("执行重建失败: %v", err)
	}

	stats.Planned = len(plans)
	if opts.Write {
		stats.Written = len(plans)
		log.Printf("完成：候选=%d，计划成功=%d，已写入=%d，失败=%d", stats.Candidates, stats.Planned, stats.Written, stats.Failed)
		return
	}
	stats.Previewed = len(plans)
	log.Printf("完成：候选=%d，计划成功=%d，已预演=%d，失败=%d", stats.Candidates, stats.Planned, stats.Previewed, stats.Failed)
	log.Println("当前为 dry-run，数据库未落盘；确认输出无误后追加 --write 执行实际修复。")
}

func parseOptions(args []string) (cliOptions, error) {
	fs := flag.NewFlagSet("rebuild-ranking-portfolio-results", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var opts cliOptions
	fs.StringVar(&opts.DBPath, "db", "", "pumpkin.db 路径；不传时自动尝试常见位置")
	fs.StringVar(&opts.FromDate, "from", "", "起始日期（YYYY-MM-DD）")
	fs.StringVar(&opts.ToDate, "to", "", "结束日期（YYYY-MM-DD）")
	fs.IntVar(&opts.Days, "days", 0, "按最近 N 个交易日倒推重建；与 --from/--to 二选一")
	fs.StringVar(&opts.DefinitionID, "definition-id", defaultDefinitionID, "组合 definition_id")
	fs.BoolVar(&opts.Write, "write", false, "实际写入数据库；默认仅 dry-run")
	fs.BoolVar(&opts.Verbose, "verbose", false, "输出更多诊断信息")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "用法：go run ./cmd/rebuild-ranking-portfolio-results [options]\n\n")
		fmt.Fprintf(fs.Output(), "示例：\n")
		fmt.Fprintf(fs.Output(), "  go run ./cmd/rebuild-ranking-portfolio-results --db ../data/pumpkin.db --days 5\n")
		fmt.Fprintf(fs.Output(), "  go run ./cmd/rebuild-ranking-portfolio-results --db ../data/pumpkin.db --from 2026-05-07 --to 2026-05-13 --write\n\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return opts, err
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
	if opts.Days < 0 {
		return opts, errors.New("days 不能为负数")
	}
	if opts.Days > 0 && (opts.FromDate != "" || opts.ToDate != "") {
		return opts, errors.New("days 与 from/to 不能同时使用")
	}
	if strings.TrimSpace(opts.DefinitionID) == "" {
		return opts, errors.New("definition-id 不能为空")
	}

	resolvedDB, err := resolveDBPath(opts.DBPath)
	if err != nil {
		return opts, err
	}
	opts.DBPath = resolvedDB
	opts.DefinitionID = strings.TrimSpace(opts.DefinitionID)
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

func loadDefinition(ctx context.Context, db *gorm.DB, definitionID string) (quadrant.RankingPortfolioDefinition, error) {
	var definition quadrant.RankingPortfolioDefinition
	err := db.WithContext(ctx).Where("id = ?", definitionID).First(&definition).Error
	if err == nil {
		return definition, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return quadrant.RankingPortfolioDefinition{}, err
	}

	if fallback, ok := fallbackDefinitionByID(definitionID, time.Now().UTC()); ok {
		return fallback, nil
	}
	return quadrant.RankingPortfolioDefinition{}, fmt.Errorf("组合定义不存在: %s", definitionID)
}

func fallbackDefinitionByID(definitionID string, now time.Time) (quadrant.RankingPortfolioDefinition, bool) {
	for _, definition := range fallbackDefinitions(now) {
		if definition.ID == strings.TrimSpace(definitionID) {
			return definition, true
		}
	}
	return quadrant.RankingPortfolioDefinition{}, false
}

func fallbackDefinitions(now time.Time) []quadrant.RankingPortfolioDefinition {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return []quadrant.RankingPortfolioDefinition{
		buildFallbackDefinition(defaultDefinitionID, defaultDefinitionCode, defaultPortfolioName, "ASHARE", "A", "SHCI", "上证指数", selectionRuleTop4, 0, []string{"STAR"}, now),
		buildFallbackDefinition(definitionIDAShareB, definitionCodeAShareB, "模拟组合B", "ASHARE", "B", "SHCI", "上证指数", selectionRuleTop10ByStreak, 10, []string{"STAR"}, now),
		buildFallbackDefinition(definitionIDHKA, definitionCodeHKA, "模拟组合A", "HKEX", "A", "HSI", "恒生指数", selectionRuleTop4, 0, nil, now),
		buildFallbackDefinition(definitionIDHKB, definitionCodeHKB, "模拟组合B", "HKEX", "B", "HSI", "恒生指数", selectionRuleTop10ByStreak, 10, nil, now),
	}
}

func buildFallbackDefinition(id string, code string, name string, exchange string, portfolioVariant string, benchmarkCode string, benchmarkName string, selectionRule string, selectionWindow int, excludedBoards []string, now time.Time) quadrant.RankingPortfolioDefinition {
	return quadrant.RankingPortfolioDefinition{
		ID:               id,
		Code:             code,
		Name:             name,
		Exchange:         exchange,
		PortfolioVariant: portfolioVariant,
		BenchmarkCode:    benchmarkCode,
		BenchmarkName:    benchmarkName,
		MaxHoldings:      defaultMaxHoldings,
		SelectionRule:    selectionRule,
		SelectionWindow:  selectionWindow,
		ExcludedBoards:   marshalStringSlice(excludedBoards),
		WeightingMethod:  "equal",
		RebalanceRule:    "t_close_generate_t1_open_rebalance",
		TradeCostRate:    defaultTradeCostRate,
		MethodNote:       defaultMethodNote,
		IsActive:         true,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
}

func marshalStringSlice(values []string) string {
	if len(values) == 0 {
		return "[]"
	}
	payload, err := json.Marshal(values)
	if err != nil {
		return "[]"
	}
	return string(payload)
}

func loadTargetDates(ctx context.Context, db *gorm.DB, exchanges []string, opts cliOptions) ([]string, error) {
	query := db.WithContext(ctx).
		Model(&quadrant.RankingSnapshot{}).
		Distinct("snapshot_date")

	if len(exchanges) > 0 {
		query = query.Where("exchange IN ?", exchanges)
	}

	if opts.FromDate != "" {
		query = query.Where("snapshot_date >= ?", opts.FromDate)
	}
	if opts.ToDate != "" {
		query = query.Where("snapshot_date <= ?", opts.ToDate)
	}

	var dates []string
	if opts.Days > 0 {
		if err := query.Order("snapshot_date DESC").Limit(opts.Days).Pluck("snapshot_date", &dates).Error; err != nil {
			return nil, err
		}
		sort.Strings(dates)
		return dates, nil
	}
	if err := query.Order("snapshot_date ASC").Pluck("snapshot_date", &dates).Error; err != nil {
		return nil, err
	}
	return dates, nil
}

func buildPlans(ctx context.Context, db *gorm.DB, definition quadrant.RankingPortfolioDefinition, targetDates []string, stockResolver *stockPriceResolver, opts cliOptions) ([]rebuildPlan, runStats, error) {
	stats := runStats{Candidates: len(targetDates)}
	plans := make([]rebuildPlan, 0, len(targetDates))

	previousConstituents, err := loadLatestPortfolioConstituentsBeforeDate(ctx, db, definition.ID, targetDates[0])
	if err != nil {
		return nil, stats, err
	}

	for i, snapshotDate := range targetDates {
		plan, currentConstituents, planErr := buildPlanForDate(ctx, db, definition, snapshotDate, previousConstituents, stockResolver)
		if planErr != nil {
			stats.Failed++
			log.Printf("[plan %d/%d] %s FAILED: %v", i+1, len(targetDates), snapshotDate, planErr)
			continue
		}
		plans = append(plans, plan)
		previousConstituents = cloneConstituents(currentConstituents)
		log.Printf(
			"[plan %d/%d] %s OK: source=%d selected=%d prices=%d/%d(snapshot=%d,fetched=%d) warning=%s",
			i+1,
			len(targetDates),
			snapshotDate,
			plan.Progress.SourceCount,
			plan.Progress.SelectedCount,
			plan.Progress.SnapshotPriceHit+plan.Progress.FetchedPriceHit,
			plan.Progress.NeededPriceCount,
			plan.Progress.SnapshotPriceHit,
			plan.Progress.FetchedPriceHit,
			warningLabel(plan.WarningText),
		)
		if opts.Verbose && len(plan.Progress.MissingSymbols) > 0 {
			log.Printf("[plan %d/%d] %s missing=%s", i+1, len(targetDates), snapshotDate, strings.Join(plan.Progress.MissingSymbols, ", "))
		}
	}

	return plans, stats, nil
}

func buildPlanForDate(ctx context.Context, db *gorm.DB, definition quadrant.RankingPortfolioDefinition, snapshotDate string, previousConstituents []quadrant.RankingPortfolioConstituentItem, stockResolver *stockPriceResolver) (rebuildPlan, []quadrant.RankingPortfolioConstituentItem, error) {
	repo := quadrant.NewRepository(db)
	sourceRows, err := loadSourceSnapshotRows(ctx, db, snapshotDate, resolveDefinitionExchanges(definition.Exchange))
	if err != nil {
		return rebuildPlan{}, nil, fmt.Errorf("加载 ranking snapshots 失败: %w", err)
	}
	if len(sourceRows) == 0 {
		return rebuildPlan{}, nil, fmt.Errorf("%s 没有可用的 ranking snapshots", snapshotDate)
	}

	constituents, err := selectPortfolioConstituentsFromSnapshots(ctx, repo, definition, sourceRows)
	if err != nil {
		return rebuildPlan{}, nil, fmt.Errorf("构建组合成分失败: %w", err)
	}
	hasShortfall := len(constituents) < definition.MaxHoldings
	warningText := ""
	if hasShortfall {
		warningText = defaultWarningText
	}

	marketPrices, snapshotHits, fetchedHits, missingSymbols, err := resolveMarketPricesForDate(ctx, snapshotDate, constituents, previousConstituents, sourceRows, stockResolver)
	if err != nil {
		return rebuildPlan{}, constituents, err
	}
	if len(missingSymbols) > 0 {
		return rebuildPlan{}, constituents, fmt.Errorf("%s 缺少股票收盘价: %s", snapshotDate, strings.Join(missingSymbols, ", "))
	}

	snapshotTime, err := rebuildSnapshotTime(snapshotDate)
	if err != nil {
		return rebuildPlan{}, constituents, err
	}

	return rebuildPlan{
		Date:         snapshotDate,
		SnapshotTime: snapshotTime,
		Constituents: constituents,
		MarketPrices: marketPrices,
		HasShortfall: hasShortfall,
		WarningText:  warningText,
		Progress: progressStatus{
			Date:             snapshotDate,
			SourceCount:      len(sourceRows),
			SelectedCount:    len(constituents),
			NeededPriceCount: len(marketPrices),
			SnapshotPriceHit: snapshotHits,
			FetchedPriceHit:  fetchedHits,
			MissingSymbols:   missingSymbols,
		},
	}, constituents, nil
}

func loadSourceSnapshotRows(ctx context.Context, db *gorm.DB, snapshotDate string, exchanges []string) ([]sourceSnapshotRow, error) {
	var rows []sourceSnapshotRow
	query := db.WithContext(ctx).
		Model(&quadrant.RankingSnapshot{}).
		Select("id, code, name, exchange, rank, opportunity, risk, close_price, price_trade_date, snapshot_date").
		Where("snapshot_date = ?", snapshotDate)
	if len(exchanges) > 0 {
		query = query.Where("exchange IN ?", exchanges)
	}
	err := query.Order("rank ASC, id ASC").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func selectPortfolioConstituentsFromSnapshots(ctx context.Context, repo *quadrant.Repository, definition quadrant.RankingPortfolioDefinition, rows []sourceSnapshotRow) ([]quadrant.RankingPortfolioConstituentItem, error) {
	maxHoldings := definition.MaxHoldings
	if maxHoldings <= 0 {
		maxHoldings = defaultMaxHoldings
	}

	exchanges := resolveDefinitionExchanges(definition.Exchange)
	exchangeSet := make(map[string]struct{}, len(exchanges))
	for _, exchange := range exchanges {
		exchangeSet[strings.ToUpper(strings.TrimSpace(exchange))] = struct{}{}
	}

	needsStreak := definition.SelectionRule == selectionRuleTop10ByStreak
	rankingItems := make([]quadrant.RankingItem, 0, len(rows))
	for _, row := range rows {
		normalizedExchange := strings.ToUpper(strings.TrimSpace(row.Exchange))
		if _, ok := exchangeSet[normalizedExchange]; !ok {
			continue
		}
		if strings.Contains(strings.ToUpper(strings.TrimSpace(row.Name)), "ST") {
			continue
		}

		item := quadrant.RankingItem{
			Rank:        row.Rank,
			Code:        strings.TrimSpace(row.Code),
			Name:        strings.TrimSpace(row.Name),
			Exchange:    normalizedExchange,
			Board:       normalizeBoardForExchange(normalizedExchange, row.Code),
			Opportunity: row.Opportunity,
			Risk:        row.Risk,
		}
		if needsStreak && repo != nil {
			days, err := repo.GetConsecutiveDays(ctx, item.Code, exchanges)
			if err != nil {
				return nil, err
			}
			item.ConsecutiveDays = days
		}
		rankingItems = append(rankingItems, item)
	}

	excludedBoards := decodeExcludedBoards(definition.ExcludedBoards)
	filtered := make([]quadrant.RankingItem, 0, len(rankingItems))
	for _, item := range rankingItems {
		if len(excludedBoards) > 0 && excludedBoards[strings.ToUpper(strings.TrimSpace(item.Board))] {
			continue
		}
		filtered = append(filtered, item)
	}

	if definition.SelectionWindow > 0 && len(filtered) > definition.SelectionWindow {
		filtered = filtered[:definition.SelectionWindow]
	}

	selected := filtered
	if needsStreak {
		selected = append([]quadrant.RankingItem(nil), filtered...)
		sort.SliceStable(selected, func(i, j int) bool {
			if selected[i].ConsecutiveDays == selected[j].ConsecutiveDays {
				if selected[i].Rank == selected[j].Rank {
					return snapshotRowKey(selected[i].Exchange, selected[i].Code) < snapshotRowKey(selected[j].Exchange, selected[j].Code)
				}
				return selected[i].Rank < selected[j].Rank
			}
			return selected[i].ConsecutiveDays > selected[j].ConsecutiveDays
		})
	}

	if len(selected) > maxHoldings {
		selected = selected[:maxHoldings]
	}

	weight := 0.0
	if len(selected) > 0 {
		weight = 1 / float64(len(selected))
	}
	items := make([]quadrant.RankingPortfolioConstituentItem, 0, len(selected))
	for i, item := range selected {
		items = append(items, quadrant.RankingPortfolioConstituentItem{
			Rank:            i + 1,
			SourceRank:      item.Rank,
			Code:            item.Code,
			Name:            item.Name,
			Exchange:        item.Exchange,
			Board:           item.Board,
			ConsecutiveDays: item.ConsecutiveDays,
			Weight:          weight,
			Opportunity:     item.Opportunity,
			Risk:            item.Risk,
		})
	}
	return items, nil
}

func resolveDefinitionExchanges(exchange string) []string {
	switch strings.ToUpper(strings.TrimSpace(exchange)) {
	case "HKEX":
		return []string{"HKEX"}
	case "ASHARE", "":
		fallthrough
	default:
		return []string{"SSE", "SZSE"}
	}
}

func decodeExcludedBoards(value string) map[string]bool {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	var boards []string
	_ = json.Unmarshal([]byte(value), &boards)
	result := make(map[string]bool, len(boards))
	for _, board := range boards {
		normalized := strings.ToUpper(strings.TrimSpace(board))
		if normalized != "" {
			result[normalized] = true
		}
	}
	return result
}

func normalizeBoardForExchange(exchange string, code string) string {
	switch strings.ToUpper(strings.TrimSpace(exchange)) {
	case "SSE", "SZSE":
		return normalizeBoardFromCode(code)
	default:
		return ""
	}
}

func snapshotRowKey(exchange string, code string) string {
	return strings.ToUpper(strings.TrimSpace(exchange)) + "\x00" + strings.TrimSpace(code)
}

func normalizeBoardFromCode(code string) string {
	trimmed := strings.TrimSpace(code)
	switch {
	case strings.HasPrefix(trimmed, "688"), strings.HasPrefix(trimmed, "689"):
		return "STAR"
	case strings.HasPrefix(trimmed, "300"), strings.HasPrefix(trimmed, "301"):
		return "CHINEXT"
	case strings.HasPrefix(trimmed, "600"), strings.HasPrefix(trimmed, "601"), strings.HasPrefix(trimmed, "603"), strings.HasPrefix(trimmed, "605"),
		strings.HasPrefix(trimmed, "000"), strings.HasPrefix(trimmed, "001"), strings.HasPrefix(trimmed, "002"), strings.HasPrefix(trimmed, "003"):
		return "MAIN"
	default:
		return "OTHER"
	}
}

func resolveMarketPricesForDate(ctx context.Context, snapshotDate string, current []quadrant.RankingPortfolioConstituentItem, previous []quadrant.RankingPortfolioConstituentItem, sourceRows []sourceSnapshotRow, resolver *stockPriceResolver) ([]marketPricePlan, int, int, []string, error) {
	sourceByKey := make(map[string]sourceSnapshotRow, len(sourceRows))
	for _, row := range sourceRows {
		sourceByKey[snapshotPriceKey(row.Code, row.Exchange)] = row
	}

	needed := map[string]quadrant.RankingPortfolioConstituentItem{}
	for _, item := range previous {
		needed[snapshotPriceKey(item.Code, item.Exchange)] = item
	}
	for _, item := range current {
		needed[snapshotPriceKey(item.Code, item.Exchange)] = item
	}

	keys := make([]string, 0, len(needed))
	for key := range needed {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	prices := make([]marketPricePlan, 0, len(keys))
	missing := make([]string, 0)
	snapshotHits := 0
	fetchedHits := 0

	for _, key := range keys {
		item := needed[key]
		lookup := priceLookupResult{}
		if row, ok := sourceByKey[key]; ok {
			lookup = sourceRowPrice(row, snapshotDate)
			if lookup.ClosePrice > 0 {
				snapshotHits++
			}
		}
		if lookup.ClosePrice <= 0 {
			resolved, err := resolver.Resolve(ctx, item.Code, item.Exchange, snapshotDate)
			if err != nil {
				return nil, 0, 0, nil, err
			}
			lookup = resolved
			if lookup.ClosePrice > 0 {
				fetchedHits++
			}
		}
		if lookup.ClosePrice <= 0 || lookup.TradeDate == "" {
			missing = append(missing, fmt.Sprintf("%s(%s)", item.Code, item.Exchange))
			continue
		}
		prices = append(prices, marketPricePlan{
			Code:           item.Code,
			Exchange:       item.Exchange,
			ClosePrice:     lookup.ClosePrice,
			PriceTradeDate: lookup.TradeDate,
		})
	}

	return prices, snapshotHits, fetchedHits, missing, nil
}

func sourceRowPrice(row sourceSnapshotRow, snapshotDate string) priceLookupResult {
	if row.ClosePrice <= 0 {
		return priceLookupResult{}
	}
	tradeDate := strings.TrimSpace(row.PriceTradeDate)
	if tradeDate == "" {
		tradeDate = snapshotDate
	}
	if !tradeDateUsableForSnapshot(tradeDate, snapshotDate) {
		return priceLookupResult{}
	}
	return priceLookupResult{ClosePrice: row.ClosePrice, TradeDate: tradeDate}
}

func (r *stockPriceResolver) Resolve(ctx context.Context, code string, exchange string, snapshotDate string) (priceLookupResult, error) {
	if r == nil || r.client == nil {
		return priceLookupResult{}, nil
	}
	symbol := buildSnapshotSymbol(code, exchange)
	if symbol == "" {
		return priceLookupResult{}, fmt.Errorf("无法为 %s(%s) 构造行情 symbol", code, exchange)
	}
	lookbackDays := calcLookbackDays(snapshotDate)
	bars, err := r.cachedBars(ctx, symbol, lookbackDays)
	if err != nil {
		return priceLookupResult{}, fmt.Errorf("拉取 %s 日线失败: %w", symbol, err)
	}
	return lookupCloseOnOrBefore(bars, snapshotDate), nil
}

func (r *stockPriceResolver) cachedBars(ctx context.Context, symbol string, lookbackDays int) ([]live.DailyBar, error) {
	cacheKey := fmt.Sprintf("%s|%d", symbol, lookbackDays)
	if bars, ok := r.cache[cacheKey]; ok {
		return bars, nil
	}
	bars, err := r.client.FetchSymbolDailyBars(ctx, symbol, lookbackDays)
	if err != nil {
		return nil, err
	}
	r.cache[cacheKey] = bars
	return bars, nil
}

func calcLookbackDays(snapshotDate string) int {
	target, err := time.ParseInLocation("2006-01-02", snapshotDate, shanghaiLocation)
	if err != nil {
		return defaultLookbackPaddingDays
	}
	today := time.Now().In(shanghaiLocation)
	gap := int(today.Sub(target).Hours()/24) + defaultLookbackPaddingDays
	if gap < defaultLookbackPaddingDays {
		return defaultLookbackPaddingDays
	}
	return gap
}

func buildSnapshotSymbol(code string, exchange string) string {
	normalizedCode := strings.ToUpper(strings.TrimSpace(code))
	if normalizedCode == "" {
		return ""
	}
	switch strings.ToUpper(strings.TrimSpace(exchange)) {
	case "SSE":
		return normalizedCode + ".SH"
	case "SZSE", "":
		return normalizedCode + ".SZ"
	case "HKEX":
		if len(normalizedCode) < 5 {
			normalizedCode = strings.Repeat("0", 5-len(normalizedCode)) + normalizedCode
		}
		return normalizedCode + ".HK"
	default:
		return ""
	}
}

func lookupCloseOnOrBefore(bars []live.DailyBar, snapshotDate string) priceLookupResult {
	for i := len(bars) - 1; i >= 0; i-- {
		tradeDate := strings.TrimSpace(bars[i].Date)
		if !tradeDateUsableForSnapshot(tradeDate, snapshotDate) {
			continue
		}
		if bars[i].Close > 0 {
			return priceLookupResult{ClosePrice: bars[i].Close, TradeDate: tradeDate}
		}
	}
	return priceLookupResult{}
}

func tradeDateUsableForSnapshot(tradeDate string, snapshotDate string) bool {
	tradeDate = strings.TrimSpace(tradeDate)
	snapshotDate = strings.TrimSpace(snapshotDate)
	if tradeDate == "" || snapshotDate == "" {
		return false
	}
	tradeAt, err1 := time.ParseInLocation("2006-01-02", tradeDate, shanghaiLocation)
	snapshotAt, err2 := time.ParseInLocation("2006-01-02", snapshotDate, shanghaiLocation)
	if err1 != nil || err2 != nil || tradeAt.After(snapshotAt) {
		return false
	}
	gapDays := int(snapshotAt.Sub(tradeAt).Hours() / 24)
	return gapDays <= maxAllowedTradeGapDays
}

func rebuildSnapshotTime(snapshotDate string) (time.Time, error) {
	day, err := time.ParseInLocation("2006-01-02", snapshotDate, shanghaiLocation)
	if err != nil {
		return time.Time{}, fmt.Errorf("解析日期失败 %s: %w", snapshotDate, err)
	}
	return time.Date(day.Year(), day.Month(), day.Day(), rebuildSnapshotHourInShanghai, 0, 0, 0, shanghaiLocation).UTC(), nil
}

func buildRebuildEffectiveTime(snapshotTime time.Time) time.Time {
	if snapshotTime.IsZero() {
		return time.Time{}
	}
	local := snapshotTime.In(shanghaiLocation)
	nextDay := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, shanghaiLocation).AddDate(0, 0, 1)
	for nextDay.Weekday() == time.Saturday || nextDay.Weekday() == time.Sunday {
		nextDay = nextDay.AddDate(0, 0, 1)
	}
	return time.Date(nextDay.Year(), nextDay.Month(), nextDay.Day(), rebuildEffectiveHourInShanghai, rebuildEffectiveMinuteInShanghai, 0, 0, shanghaiLocation).UTC()
}

func loadLatestPortfolioConstituentsBeforeDate(ctx context.Context, db *gorm.DB, definitionID string, snapshotDate string) ([]quadrant.RankingPortfolioConstituentItem, error) {
	var previousSnapshot quadrant.RankingPortfolioSnapshot
	err := db.WithContext(ctx).
		Where("definition_id = ? AND snapshot_date < ?", definitionID, snapshotDate).
		Order("snapshot_date DESC, id DESC").
		First(&previousSnapshot).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var rows []quadrant.RankingPortfolioSnapshotConstituent
	if err := db.WithContext(ctx).
		Where("definition_id = ? AND snapshot_version = ?", definitionID, previousSnapshot.SnapshotVersion).
		Order("rank ASC, id ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}

	items := make([]quadrant.RankingPortfolioConstituentItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, quadrant.RankingPortfolioConstituentItem{
			Rank:            row.Rank,
			SourceRank:      row.SourceRank,
			Code:            row.Code,
			Name:            row.Name,
			Exchange:        row.Exchange,
			Board:           row.Board,
			ConsecutiveDays: row.ConsecutiveDays,
			Weight:          row.Weight,
			RankingScore:    row.RankingScore,
			Opportunity:     row.Opportunity,
			Risk:            row.Risk,
		})
	}
	return items, nil
}

func cloneConstituents(items []quadrant.RankingPortfolioConstituentItem) []quadrant.RankingPortfolioConstituentItem {
	if len(items) == 0 {
		return nil
	}
	cloned := make([]quadrant.RankingPortfolioConstituentItem, len(items))
	copy(cloned, items)
	return cloned
}

func applyPlans(ctx context.Context, db *gorm.DB, definition quadrant.RankingPortfolioDefinition, plans []rebuildPlan, opts cliOptions) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for i, plan := range plans {
			result, err := applySinglePlanTx(ctx, tx, definition, plan)
			if err != nil {
				return fmt.Errorf("%s 写入失败: %w", plan.Date, err)
			}
			log.Printf(
				"[%s %d/%d] %s %s: portfolio=%0.2f%% constituents=%d series=%d",
				stageLabel(opts.Write),
				i+1,
				len(plans),
				plan.Date,
				strings.ToUpper(resultModeLabel(opts.Write)),
				result.LatestPortfolioReturn,
				result.CurrentConstituentCount,
				decodeSeriesLength(result.SeriesJSON),
			)
		}
		if !opts.Write {
			return errDryRunRollback
		}
		return nil
	})
}

func applySinglePlanTx(ctx context.Context, tx *gorm.DB, definition quadrant.RankingPortfolioDefinition, plan rebuildPlan) (*quadrant.RankingPortfolioResult, error) {
	now := time.Now().UTC()
	if err := upsertDefinitionTx(tx, definition, now); err != nil {
		return nil, err
	}

	snapshotVersion := plan.Date
	effectiveTime := buildRebuildEffectiveTime(plan.SnapshotTime)
	snapshot := quadrant.RankingPortfolioSnapshot{
		DefinitionID:          definition.ID,
		SnapshotVersion:       snapshotVersion,
		BatchID:               buildBatchID(definition.ID, snapshotVersion),
		SnapshotDate:          plan.Date,
		RankingTime:           plan.SnapshotTime,
		HoldingsEffectiveTime: effectiveTime,
		NavAsOfTime:           plan.SnapshotTime,
		BenchmarkCode:         definition.BenchmarkCode,
		BenchmarkName:         definition.BenchmarkName,
		ConstituentsCount:     len(plan.Constituents),
		HasShortfall:          plan.HasShortfall,
		WarningText:           plan.WarningText,
		MethodNote:            definition.MethodNote,
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	if err := tx.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "definition_id"}, {Name: "snapshot_version"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"batch_id", "snapshot_date", "ranking_time", "holdings_effective_time", "nav_as_of_time",
			"benchmark_code", "benchmark_name", "constituents_count", "has_shortfall", "warning_text", "method_note", "updated_at",
		}),
	}).Create(&snapshot).Error; err != nil {
		return nil, fmt.Errorf("upsert portfolio snapshot: %w", err)
	}

	if err := replaceConstituentsTx(ctx, tx, definition.ID, snapshotVersion, plan.Date, plan.Constituents, now); err != nil {
		return nil, err
	}
	if err := replaceMarketPricesTx(ctx, tx, definition.ID, snapshotVersion, plan.Date, plan.MarketPrices, now); err != nil {
		return nil, err
	}

	result, err := buildResultForDateTx(ctx, tx, definition, plan.Date, now)
	if err != nil {
		return nil, err
	}
	if err := tx.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "definition_id"}, {Name: "snapshot_version"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"batch_id", "snapshot_date", "ranking_time", "holdings_effective_time", "nav_as_of_time",
			"benchmark_code", "benchmark_name", "latest_nav",
			"latest_portfolio_return",
			"current_constituent_count", "has_shortfall", "warning_text", "method_note",
			"series_json", "constituents_json", "latest_rebalance_json", "updated_at",
		}),
	}).Create(result).Error; err != nil {
		return nil, fmt.Errorf("upsert portfolio result: %w", err)
	}

	return result, nil
}

func upsertDefinitionTx(tx *gorm.DB, definition quadrant.RankingPortfolioDefinition, now time.Time) error {
	definition.UpdatedAt = now
	if definition.CreatedAt.IsZero() {
		definition.CreatedAt = now
	}
	return tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"code", "name", "exchange", "portfolio_variant", "benchmark_code", "benchmark_name",
			"max_holdings", "selection_rule", "selection_window", "excluded_boards", "weighting_method", "rebalance_rule",
			"trade_cost_rate", "method_note", "is_active", "updated_at",
		}),
	}).Create(&definition).Error
}

func replaceConstituentsTx(ctx context.Context, tx *gorm.DB, definitionID string, snapshotVersion string, snapshotDate string, items []quadrant.RankingPortfolioConstituentItem, now time.Time) error {
	if err := tx.WithContext(ctx).
		Where("definition_id = ? AND snapshot_version = ?", definitionID, snapshotVersion).
		Delete(&quadrant.RankingPortfolioSnapshotConstituent{}).Error; err != nil {
		return fmt.Errorf("delete old constituents: %w", err)
	}
	if len(items) == 0 {
		return nil
	}

	rows := make([]quadrant.RankingPortfolioSnapshotConstituent, 0, len(items))
	for _, item := range items {
		rows = append(rows, quadrant.RankingPortfolioSnapshotConstituent{
			DefinitionID:    definitionID,
			SnapshotVersion: snapshotVersion,
			SnapshotDate:    snapshotDate,
			Rank:            item.Rank,
			Code:            item.Code,
			Name:            item.Name,
			Exchange:        item.Exchange,
			Board:           item.Board,
			SourceRank:      item.SourceRank,
			ConsecutiveDays: item.ConsecutiveDays,
			Weight:          item.Weight,
			RankingScore:    item.RankingScore,
			Opportunity:     item.Opportunity,
			Risk:            item.Risk,
			CreatedAt:       now,
			UpdatedAt:       now,
		})
	}
	if err := tx.WithContext(ctx).Create(&rows).Error; err != nil {
		return fmt.Errorf("insert constituents: %w", err)
	}
	return nil
}

func replaceMarketPricesTx(ctx context.Context, tx *gorm.DB, definitionID string, snapshotVersion string, snapshotDate string, prices []marketPricePlan, now time.Time) error {
	if err := tx.WithContext(ctx).
		Where("definition_id = ? AND snapshot_version = ?", definitionID, snapshotVersion).
		Delete(&quadrant.RankingPortfolioMarketPrice{}).Error; err != nil {
		return fmt.Errorf("delete old market prices: %w", err)
	}
	if len(prices) == 0 {
		return nil
	}

	rows := make([]quadrant.RankingPortfolioMarketPrice, 0, len(prices))
	for _, price := range prices {
		rows = append(rows, quadrant.RankingPortfolioMarketPrice{
			DefinitionID:    definitionID,
			SnapshotVersion: snapshotVersion,
			SnapshotDate:    snapshotDate,
			Code:            price.Code,
			Exchange:        price.Exchange,
			ClosePrice:      price.ClosePrice,
			PriceTradeDate:  price.PriceTradeDate,
			CreatedAt:       now,
			UpdatedAt:       now,
		})
	}
	if err := tx.WithContext(ctx).Create(&rows).Error; err != nil {
		return fmt.Errorf("insert market prices: %w", err)
	}
	return nil
}

func buildResultForDateTx(ctx context.Context, tx *gorm.DB, definition quadrant.RankingPortfolioDefinition, snapshotDate string, now time.Time) (*quadrant.RankingPortfolioResult, error) {
	var snapshots []quadrant.RankingPortfolioSnapshot
	if err := tx.WithContext(ctx).
		Where("definition_id = ? AND snapshot_date <= ?", definition.ID, snapshotDate).
		Order("snapshot_date ASC, id ASC").
		Find(&snapshots).Error; err != nil {
		return nil, fmt.Errorf("list portfolio snapshots: %w", err)
	}
	if len(snapshots) == 0 {
		return nil, fmt.Errorf("%s 之前没有 portfolio snapshots", snapshotDate)
	}

	constituentsByVersion := map[string][]quadrant.RankingPortfolioConstituentItem{}
	var constituentRows []quadrant.RankingPortfolioSnapshotConstituent
	if err := tx.WithContext(ctx).
		Where("definition_id = ? AND snapshot_date <= ?", definition.ID, snapshotDate).
		Order("snapshot_date ASC, rank ASC, id ASC").
		Find(&constituentRows).Error; err != nil {
		return nil, fmt.Errorf("list portfolio constituents: %w", err)
	}
	for _, row := range constituentRows {
		constituentsByVersion[row.SnapshotVersion] = append(constituentsByVersion[row.SnapshotVersion], quadrant.RankingPortfolioConstituentItem{
			Rank:            row.Rank,
			SourceRank:      row.SourceRank,
			Code:            row.Code,
			Name:            row.Name,
			Exchange:        row.Exchange,
			Board:           row.Board,
			ConsecutiveDays: row.ConsecutiveDays,
			Weight:          row.Weight,
			RankingScore:    row.RankingScore,
			Opportunity:     row.Opportunity,
			Risk:            row.Risk,
		})
	}

	priceByVersion := map[string]map[string]float64{}
	var priceRows []quadrant.RankingPortfolioMarketPrice
	if err := tx.WithContext(ctx).
		Where("definition_id = ? AND snapshot_date <= ?", definition.ID, snapshotDate).
		Order("snapshot_date ASC, exchange ASC, code ASC, id ASC").
		Find(&priceRows).Error; err != nil {
		return nil, fmt.Errorf("list portfolio market prices: %w", err)
	}
	for _, row := range priceRows {
		if _, ok := priceByVersion[row.SnapshotVersion]; !ok {
			priceByVersion[row.SnapshotVersion] = map[string]float64{}
		}
		priceByVersion[row.SnapshotVersion][snapshotPriceKey(row.Code, row.Exchange)] = row.ClosePrice
	}

	series := make([]quadrant.RankingPortfolioSeriesPoint, 0, len(snapshots))
	firstSnapshot := snapshots[0]
	series = append(series, quadrant.RankingPortfolioSeriesPoint{
		Date:                    firstSnapshot.SnapshotDate,
		SourceTradeDate:         firstSnapshot.SourceTradeDate,
		Nav:                     1,
		PortfolioReturnPct:      0,
		DailyPortfolioReturnPct: 0,
		DrawdownPct:             0,
		HoldingCount:            0,
	})

	activeHoldings := []quadrant.RankingPortfolioConstituentItem{}
	peakNav := 1.0
	for i := 1; i < len(snapshots); i++ {
		prevSnapshot := snapshots[i-1]
		currentSnapshot := snapshots[i]
		nextHoldings := constituentsByVersion[prevSnapshot.SnapshotVersion]
		portfolioReturn := calculatePeriodReturn(nextHoldings, priceByVersion[prevSnapshot.SnapshotVersion], priceByVersion[currentSnapshot.SnapshotVersion])
		tradeRatio := calculateTradeRatio(activeHoldings, nextHoldings)
		costRatio := definition.TradeCostRate * tradeRatio
		netDailyReturn := (1-costRatio)*(1+portfolioReturn) - 1

		prevPoint := series[len(series)-1]
		nav := prevPoint.Nav * (1 + netDailyReturn)
		if nav > peakNav {
			peakNav = nav
		}
		drawdownPct := 0.0
		if peakNav > 0 {
			drawdownPct = roundPct((nav/peakNav - 1) * 100)
		}

		series = append(series, quadrant.RankingPortfolioSeriesPoint{
			Date:                    currentSnapshot.SnapshotDate,
			SourceTradeDate:         currentSnapshot.SourceTradeDate,
			Nav:                     roundFloat(nav),
			PortfolioReturnPct:      roundPct((nav - 1) * 100),
			DailyPortfolioReturnPct: roundPct(netDailyReturn * 100),
			DrawdownPct:             drawdownPct,
			HoldingCount:            len(nextHoldings),
		})
		activeHoldings = cloneConstituents(nextHoldings)
	}

	latestSnapshot := snapshots[len(snapshots)-1]
	latestConstituents := constituentsByVersion[latestSnapshot.SnapshotVersion]
	latestPoint := series[len(series)-1]
	previousConstituents := []quadrant.RankingPortfolioConstituentItem{}
	if len(snapshots) > 1 {
		previousConstituents = constituentsByVersion[snapshots[len(snapshots)-2].SnapshotVersion]
	}
	latestPriceByKey := map[string]quadrant.RankingPortfolioMarketPrice{}
	for _, row := range priceRows {
		if row.SnapshotVersion != latestSnapshot.SnapshotVersion {
			continue
		}
		latestPriceByKey[snapshotPriceKey(row.Code, row.Exchange)] = row
	}
	latestRebalanceJSON := mustMarshal(buildLatestRebalance(definition, latestSnapshot, latestConstituents, previousConstituents, latestPriceByKey))

	return &quadrant.RankingPortfolioResult{
		DefinitionID:            definition.ID,
		SnapshotVersion:         latestSnapshot.SnapshotVersion,
		BatchID:                 buildBatchID(definition.ID, latestSnapshot.SnapshotVersion),
		SnapshotDate:            latestSnapshot.SnapshotDate,
		RankingTime:             latestSnapshot.RankingTime,
		HoldingsEffectiveTime:   latestSnapshot.HoldingsEffectiveTime,
		NavAsOfTime:             latestSnapshot.NavAsOfTime,
		BenchmarkCode:           latestSnapshot.BenchmarkCode,
		BenchmarkName:           latestSnapshot.BenchmarkName,
		LatestNav:               latestPoint.Nav,
		LatestPortfolioReturn:   latestPoint.PortfolioReturnPct,
		CurrentConstituentCount: len(latestConstituents),
		HasShortfall:            latestSnapshot.HasShortfall,
		WarningText:             latestSnapshot.WarningText,
		MethodNote:              latestSnapshot.MethodNote,
		SeriesJSON:              mustMarshal(series),
		ConstituentsJSON:        mustMarshal(latestConstituents),
		LatestRebalanceJSON:     latestRebalanceJSON,
		CreatedAt:               now,
		UpdatedAt:               now,
	}, nil
}

func buildLatestRebalance(
	definition quadrant.RankingPortfolioDefinition,
	currentSnapshot quadrant.RankingPortfolioSnapshot,
	current []quadrant.RankingPortfolioConstituentItem,
	previous []quadrant.RankingPortfolioConstituentItem,
	priceByKey map[string]quadrant.RankingPortfolioMarketPrice,
) *quadrant.RankingPortfolioLatestRebalance {
	currentByKey := make(map[string]quadrant.RankingPortfolioConstituentItem, len(current))
	for _, item := range current {
		currentByKey[snapshotPriceKey(item.Code, item.Exchange)] = item
	}
	previousByKey := make(map[string]quadrant.RankingPortfolioConstituentItem, len(previous))
	for _, item := range previous {
		previousByKey[snapshotPriceKey(item.Code, item.Exchange)] = item
	}

	keys := make([]string, 0, len(currentByKey)+len(previousByKey))
	seen := make(map[string]struct{}, len(currentByKey)+len(previousByKey))
	for key := range currentByKey {
		keys = append(keys, key)
		seen[key] = struct{}{}
	}
	for key := range previousByKey {
		if _, ok := seen[key]; ok {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	items := make([]quadrant.RankingPortfolioRebalanceItem, 0, len(keys))
	for _, key := range keys {
		currentItem, hasCurrent := currentByKey[key]
		previousItem, hasPrevious := previousByKey[key]
		fromWeight := 0.0
		toWeight := 0.0
		baseItem := currentItem
		if hasPrevious {
			fromWeight = previousItem.Weight
			baseItem = previousItem
		}
		if hasCurrent {
			toWeight = currentItem.Weight
			baseItem = currentItem
		}
		weightDiff := fromWeight - toWeight
		if weightDiff < 0 {
			weightDiff = -weightDiff
		}
		if weightDiff < 1e-9 {
			continue
		}

		action := "buy"
		costMultiplier := 1 + definition.TradeCostRate
		if toWeight < fromWeight {
			action = "sell"
			costMultiplier = 1 - definition.TradeCostRate
		}

		priceRow := priceByKey[key]
		referencePrice := roundFloat(priceRow.ClosePrice)
		referenceCostPrice := 0.0
		if referencePrice > 0 {
			referenceCostPrice = roundFloat(referencePrice * costMultiplier)
		}

		items = append(items, quadrant.RankingPortfolioRebalanceItem{
			Action:             action,
			Code:               baseItem.Code,
			Name:               baseItem.Name,
			Exchange:           baseItem.Exchange,
			Board:              baseItem.Board,
			FromWeight:         roundFloat(fromWeight),
			ToWeight:           roundFloat(toWeight),
			ReferencePrice:     referencePrice,
			ReferenceCostPrice: referenceCostPrice,
			PriceTradeDate:     priceRow.PriceTradeDate,
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Action != items[j].Action {
			return items[i].Action == "sell"
		}
		if items[i].Exchange != items[j].Exchange {
			return items[i].Exchange < items[j].Exchange
		}
		return items[i].Code < items[j].Code
	})

	return &quadrant.RankingPortfolioLatestRebalance{
		SnapshotDate:  currentSnapshot.SnapshotDate,
		RankingTime:   currentSnapshot.RankingTime.UTC().Format(time.RFC3339),
		EffectiveTime: currentSnapshot.HoldingsEffectiveTime.UTC().Format(time.RFC3339),
		TradeCostRate: roundTo(definition.TradeCostRate, rankingPortfolioTradeCostDigits),
		ChangeCount:   len(items),
		Items:         items,
	}
}

func calculatePeriodReturn(holdings []quadrant.RankingPortfolioConstituentItem, prevPrices map[string]float64, currentPrices map[string]float64) float64 {
	if len(holdings) == 0 || len(prevPrices) == 0 || len(currentPrices) == 0 {
		return 0
	}
	weightedSum := 0.0
	weightSum := 0.0
	for _, holding := range holdings {
		key := snapshotPriceKey(holding.Code, holding.Exchange)
		prevClose := prevPrices[key]
		currentClose := currentPrices[key]
		if prevClose <= 0 || currentClose <= 0 {
			continue
		}
		weightedSum += holding.Weight * (currentClose/prevClose - 1)
		weightSum += holding.Weight
	}
	if weightSum <= 0 {
		return 0
	}
	return weightedSum / weightSum
}

func calculateTradeRatio(previous []quadrant.RankingPortfolioConstituentItem, current []quadrant.RankingPortfolioConstituentItem) float64 {
	weights := map[string]float64{}
	for _, item := range previous {
		weights[snapshotPriceKey(item.Code, item.Exchange)] -= item.Weight
	}
	for _, item := range current {
		weights[snapshotPriceKey(item.Code, item.Exchange)] += item.Weight
	}
	turnover := 0.0
	for _, diff := range weights {
		if diff < 0 {
			diff = -diff
		}
		turnover += diff
	}
	return turnover
}

func snapshotPriceKey(code string, exchange string) string {
	return strings.ToUpper(strings.TrimSpace(exchange)) + "\x00" + strings.TrimSpace(code)
}

func buildBatchID(definitionID string, snapshotVersion string) string {
	return strings.TrimSpace(definitionID) + ":" + strings.TrimSpace(snapshotVersion)
}

func roundFloat(value float64) float64 {
	return roundTo(value, 6)
}

func roundPct(value float64) float64 {
	return roundTo(value, 4)
}

func roundTo(value float64, digits int) float64 {
	shift := 1.0
	for i := 0; i < digits; i++ {
		shift *= 10
	}
	if value >= 0 {
		return float64(int(value*shift+0.5)) / shift
	}
	return float64(int(value*shift-0.5)) / shift
}

func mustMarshal(v any) string {
	b, _ := json.Marshal(v)
	if len(b) == 0 {
		return "[]"
	}
	return string(b)
}

func decodeSeriesLength(raw string) int {
	if strings.TrimSpace(raw) == "" {
		return 0
	}
	var series []quadrant.RankingPortfolioSeriesPoint
	_ = json.Unmarshal([]byte(raw), &series)
	return len(series)
}

func warningLabel(warningText string) string {
	if strings.TrimSpace(warningText) == "" {
		return "none"
	}
	return warningText
}

func runModeLabel(write bool) string {
	if write {
		return "write"
	}
	return "dry-run"
}

func actionLabel(write bool) string {
	if write {
		return "写入"
	}
	return "预演"
}

func resultModeLabel(write bool) string {
	if write {
		return "written"
	}
	return "previewed"
}

func stageLabel(write bool) string {
	if write {
		return "write"
	}
	return "preview"
}
