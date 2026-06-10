package quadrant

import (
	"context"
	"testing"
	"time"
)

// open→close 单日组合收益（等权）。
func TestCalculateRankingPortfolioPeriodReturn_OpenToClose(t *testing.T) {
	holdings := []RankingPortfolioConstituentItem{
		{Code: "600001", Exchange: "SSE", Weight: 0.5},
		{Code: "000001", Exchange: "SZSE", Weight: 0.5},
	}
	dayPrices := map[string]rankingPortfolioDayPrice{
		snapshotPriceHintKey("600001", "SSE"):  {Open: 10, Close: 11}, // +10%
		snapshotPriceHintKey("000001", "SZSE"): {Open: 20, Close: 19}, // -5%
	}
	got := calculateRankingPortfolioPeriodReturn(holdings, dayPrices)
	want := 0.025 // (0.10 + -0.05) / 2
	if abs(got-want) > 1e-9 {
		t.Fatalf("period return = %v, want %v", got, want)
	}
}

// 缺开盘价的成分股被跳过，剩余成分股重新归一权重。
func TestCalculateRankingPortfolioPeriodReturn_SkipsMissingOpen(t *testing.T) {
	holdings := []RankingPortfolioConstituentItem{
		{Code: "600001", Exchange: "SSE", Weight: 0.5},
		{Code: "000001", Exchange: "SZSE", Weight: 0.5},
	}
	dayPrices := map[string]rankingPortfolioDayPrice{
		snapshotPriceHintKey("600001", "SSE"):  {Open: 10, Close: 11}, // +10%
		snapshotPriceHintKey("000001", "SZSE"): {Open: 0, Close: 19},  // 开盘价缺失 → 跳过
	}
	got := calculateRankingPortfolioPeriodReturn(holdings, dayPrices)
	if abs(got-0.10) > 1e-9 {
		t.Fatalf("period return = %v, want 0.10 (re-normalized to single holding)", got)
	}
}

// 连续在仓的股票换手率为 0，不重复扣交易成本。
func TestCalculateRankingPortfolioTradeRatio_ContinuousHoldingZeroTurnover(t *testing.T) {
	holdings := []RankingPortfolioConstituentItem{
		{Code: "600001", Exchange: "SSE", Weight: 0.5},
		{Code: "000001", Exchange: "SZSE", Weight: 0.5},
	}
	ratio := calculateRankingPortfolioTradeRatio(holdings, holdings)
	if ratio != 0 {
		t.Fatalf("continuous holding turnover = %v, want 0", ratio)
	}
}

// 一进一出：换手率 = 卖出腿(0.5) + 买入腿(0.5) = 1。
func TestCalculateRankingPortfolioTradeRatio_OneInOneOut(t *testing.T) {
	previous := []RankingPortfolioConstituentItem{
		{Code: "600001", Exchange: "SSE", Weight: 0.5},
		{Code: "000001", Exchange: "SZSE", Weight: 0.5},
	}
	current := []RankingPortfolioConstituentItem{
		{Code: "600001", Exchange: "SSE", Weight: 0.5},
		{Code: "600002", Exchange: "SSE", Weight: 0.5},
	}
	ratio := calculateRankingPortfolioTradeRatio(previous, current)
	if ratio != 1 {
		t.Fatalf("turnover = %v, want 1 (sell 0.5 + buy 0.5)", ratio)
	}
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// ── 实时刷新时点（北京时间）────────────────────────────────────────────

func TestDefaultRefreshPoints_AShareAndHKCounts(t *testing.T) {
	a := defaultAShareRefreshPoints()
	if len(a) != 12 {
		t.Fatalf("A-share refresh points = %d, want 12", len(a))
	}
	if a[0] != "09:25" || a[len(a)-1] != "15:30" {
		t.Fatalf("A-share first/last = %s/%s, want 09:25/15:30", a[0], a[len(a)-1])
	}
	hk := defaultHKRefreshPoints()
	if len(hk) != 15 {
		t.Fatalf("HK refresh points = %d, want 15", len(hk))
	}
	if hk[0] != "09:25" || hk[len(hk)-1] != "16:30" {
		t.Fatalf("HK first/last = %s/%s, want 09:25/16:30", hk[0], hk[len(hk)-1])
	}
}

func TestParseRefreshPoint(t *testing.T) {
	cases := map[string]struct {
		mins int
		ok   bool
	}{
		"09:25": {565, true},
		"15:30": {930, true},
		"24:00": {0, false},
		"09:60": {0, false},
		"9:5":   {545, true},
		"abc":   {0, false},
		"0900":  {0, false},
	}
	for in, want := range cases {
		mins, ok := parseRefreshPoint(in)
		if ok != want.ok || (ok && mins != want.mins) {
			t.Fatalf("parseRefreshPoint(%q) = (%d,%v), want (%d,%v)", in, mins, ok, want.mins, want.ok)
		}
	}
}

func TestSanitizeRefreshPoints_SortsAndDedups(t *testing.T) {
	got := sanitizeRefreshPoints([]string{"15:30", "09:25", "bad", "09:25", "13:00"})
	want := []string{"09:25", "13:00", "15:30"}
	if len(got) != len(want) {
		t.Fatalf("sanitized = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sanitized[%d] = %s, want %s", i, got[i], want[i])
		}
	}
}

func TestNextRealtimeTriggerAt_BeijingTime(t *testing.T) {
	loc := beijingLocation()
	points := []string{"09:25", "09:30", "15:30"}

	// 周四 09:26（北京时间）→ 下一个点应为同日 09:30。
	now := time.Date(2026, 6, 4, 9, 26, 0, 0, loc)
	next := nextRealtimeTriggerAt(now, points)
	if next.Hour() != 9 || next.Minute() != 30 || next.Day() != 4 {
		t.Fatalf("next = %s, want 2026-06-04 09:30 CST", next.Format(time.RFC3339))
	}

	// 周四 15:31 收盘后 → 下一个点应为周五 09:25。
	now = time.Date(2026, 6, 4, 15, 31, 0, 0, loc)
	next = nextRealtimeTriggerAt(now, points)
	if next.Day() != 5 || next.Hour() != 9 || next.Minute() != 25 {
		t.Fatalf("next = %s, want 2026-06-05 09:25 CST", next.Format(time.RFC3339))
	}

	// 周五收盘后 → 跳过周末，下一个点应为下周一 09:25。
	now = time.Date(2026, 6, 5, 16, 0, 0, 0, loc)
	next = nextRealtimeTriggerAt(now, points)
	if next.Weekday() != time.Monday || next.Hour() != 9 || next.Minute() != 25 {
		t.Fatalf("next = %s, want next Monday 09:25 CST", next.Format(time.RFC3339))
	}

	// 即便传入的是 UTC 时间，也按北京时间换算。
	utcNow := time.Date(2026, 6, 4, 1, 26, 0, 0, time.UTC) // = 北京 09:26
	next = nextRealtimeTriggerAt(utcNow, points)
	if next.In(loc).Hour() != 9 || next.In(loc).Minute() != 30 {
		t.Fatalf("UTC input next = %s, want 09:30 CST", next.In(loc).Format(time.RFC3339))
	}
}

func TestIsOpenAuctionPoint(t *testing.T) {
	loc := beijingLocation()
	if !isOpenAuctionPoint(time.Date(2026, 6, 4, 9, 25, 0, 0, loc)) {
		t.Fatal("09:25 CST should be the open-auction point")
	}
	if isOpenAuctionPoint(time.Date(2026, 6, 4, 9, 30, 0, 0, loc)) {
		t.Fatal("09:30 CST should not be the open-auction point")
	}
	// UTC 01:25 = 北京 09:25。
	if !isOpenAuctionPoint(time.Date(2026, 6, 4, 1, 25, 0, 0, time.UTC)) {
		t.Fatal("UTC 01:25 (=09:25 CST) should be the open-auction point")
	}
}

// 实时 worker：09:25 落开盘买入价 + 实时价；后续点只更新实时价。
func TestRealtimeWorker_RunOnceFillsOpenAndRealtime(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	svc := NewService(repo)
	ctx := context.Background()
	now := time.Now().UTC()

	// 默认 A 股组合定义需要落库，FillRankingPortfolioEntryOpenPrice 才能定位最新批次。
	def := buildRankingPortfolioDefinitionRecord(defaultRankingPortfolioDefinitionSpecs()[0], now)
	if err := repo.db.WithContext(ctx).Create(&def).Error; err != nil {
		t.Fatalf("seed definition failed: %v", err)
	}
	snap := RankingPortfolioSnapshot{DefinitionID: def.ID, SnapshotVersion: "2026-06-03", SnapshotDate: "2026-06-03", SourceTradeDate: "2026-06-03", CreatedAt: now, UpdatedAt: now}
	if err := repo.db.WithContext(ctx).Create(&snap).Error; err != nil {
		t.Fatalf("seed snapshot failed: %v", err)
	}
	mp := RankingPortfolioMarketPrice{DefinitionID: def.ID, SnapshotVersion: "2026-06-03", SnapshotDate: "2026-06-03", Code: "600001", Exchange: "SSE", ClosePrice: 9.5, PriceTradeDate: "2026-06-03", CreatedAt: now, UpdatedAt: now}
	if err := repo.db.WithContext(ctx).Create(&mp).Error; err != nil {
		t.Fatalf("seed market price failed: %v", err)
	}

	fetcher := func(ctx context.Context, symbols []RealtimeSymbol) ([]RealtimeQuote, error) {
		out := make([]RealtimeQuote, 0, len(symbols))
		for _, s := range symbols {
			out = append(out, RealtimeQuote{Code: s.Code, Exchange: s.Exchange, LastPrice: 10})
		}
		return out, nil
	}
	worker := NewRealtimeWorker(svc, fetcher, RealtimeWorkerConfig{Enabled: true})

	// 直接对单一标的执行落库（绕过 buildCurrentRankingPortfolioSelection 的榜单依赖）。
	quotes := []RealtimeQuote{{Code: "600001", Exchange: "SSE", LastPrice: 10, IsOpen: true}}
	openAt := time.Date(2026, 6, 4, 9, 25, 0, 0, beijingLocation())
	if err := svc.persistRealtimeQuotes(ctx, "ASHARE", quotes, true, openAt); err != nil {
		t.Fatalf("persist open quotes failed: %v", err)
	}

	openPrice, entryDate, err := repo.GetRankingPortfolioEntryOpenPrice(ctx, def.ID, "600001", "SSE", "2026-06-04")
	if err != nil {
		t.Fatalf("get entry open price failed: %v", err)
	}
	if openPrice != 10 || entryDate != "2026-06-04" {
		t.Fatalf("entry open = (%v,%s), want (10,2026-06-04)", openPrice, entryDate)
	}
	rt, _, err := repo.GetRankingPortfolioRealtimePrice(ctx, "600001", "SSE")
	if err != nil {
		t.Fatalf("get realtime price failed: %v", err)
	}
	if rt != 10 {
		t.Fatalf("realtime = %v, want 10", rt)
	}

	// 盘中后续点（非 09:25）只更新实时价，不改开盘价。
	intradayAt := time.Date(2026, 6, 4, 10, 30, 0, 0, beijingLocation())
	if err := svc.persistRealtimeQuotes(ctx, "ASHARE", []RealtimeQuote{{Code: "600001", Exchange: "SSE", LastPrice: 12}}, false, intradayAt); err != nil {
		t.Fatalf("persist intraday quotes failed: %v", err)
	}
	rt2, _, _ := repo.GetRankingPortfolioRealtimePrice(ctx, "600001", "SSE")
	if rt2 != 12 {
		t.Fatalf("realtime after intraday = %v, want 12", rt2)
	}
	openPrice2, _, _ := repo.GetRankingPortfolioEntryOpenPrice(ctx, def.ID, "600001", "SSE", "2026-06-04")
	if openPrice2 != 10 {
		t.Fatalf("open price after intraday = %v, want unchanged 10", openPrice2)
	}
	_ = worker
}
