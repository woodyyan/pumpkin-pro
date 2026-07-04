package quadrant

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

type SimPortfolioV2Service struct {
	repo                *Repository
	calendar            *SimPortfolioV2CalendarService
	priceResolver       PriceResolver
	priceLookupResolver PriceLookupResolver
	openPriceResolver   OpenPriceResolver
	dailyBarFetcher     SimPortfolioV2DailyBarFetcher
}

func NewSimPortfolioV2Service(repo *Repository) *SimPortfolioV2Service {
	return &SimPortfolioV2Service{repo: repo, calendar: NewSimPortfolioV2CalendarService()}
}

func (s *SimPortfolioV2Service) SetPriceResolver(r PriceResolver) { s.priceResolver = r }
func (s *SimPortfolioV2Service) SetPriceLookupResolver(r PriceLookupResolver) {
	s.priceLookupResolver = r
}
func (s *SimPortfolioV2Service) SetOpenPriceResolver(r OpenPriceResolver) { s.openPriceResolver = r }
func (s *SimPortfolioV2Service) SetDailyBarFetcher(f SimPortfolioV2DailyBarFetcher) {
	s.dailyBarFetcher = f
}

type SimPortfolioV2DailyBar struct {
	Date  string
	Open  float64
	Close float64
}

type SimPortfolioV2DailyBarFetcher func(ctx context.Context, code string, exchange string, lookbackDays int) ([]SimPortfolioV2DailyBar, error)

type SimPortfolioV2RunRequest struct {
	Market   string   `json:"market"`
	FromDate string   `json:"from_date"`
	ToDate   string   `json:"to_date"`
	Stages   []string `json:"stages"`
	DryRun   bool     `json:"dry_run"`
}

type SimPortfolioV2RunResponse struct {
	OK             bool   `json:"ok"`
	Message        string `json:"message,omitempty"`
	RunID          string `json:"run_id,omitempty"`
	Status         string `json:"status"`
	ProcessedDays  int    `json:"processed_days"`
	BlockedDays    int    `json:"blocked_days"`
	GeneratedFacts int    `json:"generated_facts"`
}

type SimPortfolioV2PriceRepairRequest struct {
	Market      string `json:"market"`
	SignalDate  string `json:"signal_date"`
	PortfolioID string `json:"portfolio_id,omitempty"`
	PriceType   string `json:"price_type,omitempty"`
	OnlyMissing bool   `json:"only_missing"`
	Operator    string `json:"operator,omitempty"`
}

type SimPortfolioV2PriceBackfillRequest struct {
	Market       string `json:"market"`
	SignalDate   string `json:"signal_date"`
	PortfolioID  string `json:"portfolio_id,omitempty"`
	PriceType    string `json:"price_type,omitempty"`
	OnlyMissing  bool   `json:"only_missing"`
	LookbackDays int    `json:"lookback_days,omitempty"`
	Operator     string `json:"operator,omitempty"`
}

type SimPortfolioV2PriceOverrideRequest struct {
	Market      string  `json:"market"`
	SignalDate  string  `json:"signal_date"`
	PortfolioID string  `json:"portfolio_id,omitempty"`
	Code        string  `json:"code"`
	Exchange    string  `json:"exchange"`
	TradeDate   string  `json:"trade_date"`
	PriceType   string  `json:"price_type"`
	Price       float64 `json:"price"`
	Reason      string  `json:"reason"`
	Evidence    string  `json:"evidence"`
	Operator    string  `json:"operator,omitempty"`
	Confirm     bool    `json:"confirm"`
}

type SimPortfolioV2PriceRepairResponse struct {
	OK            bool     `json:"ok"`
	Action        string   `json:"action"`
	Status        string   `json:"status"`
	AuditID       int64    `json:"audit_id,omitempty"`
	Checked       int      `json:"checked"`
	Satisfied     int      `json:"satisfied"`
	StillMissing  int      `json:"still_missing"`
	Updated       int      `json:"updated"`
	Backfilled    int      `json:"backfilled,omitempty"`
	Message       string   `json:"message,omitempty"`
	MissingItems  []string `json:"missing_items,omitempty"`
	RequiresRerun bool     `json:"requires_rerun"`
}

type SimPortfolioV2OverviewResponse struct {
	Items []SimPortfolioV2MarketOverview `json:"items"`
	Runs  []SimPortfolioV2RunItem        `json:"runs,omitempty"`
}

type SimPortfolioV2MarketOverview struct {
	Market                  string `json:"market"`
	Status                  string `json:"status"`
	StatusText              string `json:"status_text"`
	LatestSignalDate        string `json:"latest_signal_date,omitempty"`
	LatestVerifiedTradeDate string `json:"latest_verified_trade_date,omitempty"`
	BlockingStage           string `json:"blocking_stage,omitempty"`
	BlockingMessage         string `json:"blocking_message,omitempty"`
}

type SimPortfolioV2RunItem struct {
	RunID      string `json:"run_id"`
	Market     string `json:"market"`
	Status     string `json:"status"`
	FromDate   string `json:"from_date,omitempty"`
	ToDate     string `json:"to_date,omitempty"`
	StartedAt  string `json:"started_at,omitempty"`
	FinishedAt string `json:"finished_at,omitempty"`
}

type SimPortfolioV2DaysResponse struct {
	Items []SimPortfolioV2DayStatusItem `json:"items"`
}
type SimPortfolioV2DayStatusItem struct {
	Market        string `json:"market"`
	TradeDate     string `json:"trade_date"`
	Stage         string `json:"stage"`
	Status        string `json:"status"`
	ExpectedCount int    `json:"expected_count"`
	ActualCount   int    `json:"actual_count"`
	MissingCount  int    `json:"missing_count"`
	Message       string `json:"message,omitempty"`
	ActionHint    string `json:"action_hint,omitempty"`
}

func (s *SimPortfolioV2Service) Initialize(ctx context.Context, market string) (*SimPortfolioV2RunResponse, error) {
	if err := s.repo.EnsureSimPortfolioV2Definitions(ctx); err != nil {
		return nil, err
	}
	return &SimPortfolioV2RunResponse{OK: true, Status: SimPortfolioV2StatusOK, Message: "Sim Portfolio v2 定义已初始化。"}, nil
}

func (s *SimPortfolioV2Service) Run(ctx context.Context, req SimPortfolioV2RunRequest) (*SimPortfolioV2RunResponse, error) {
	if err := s.repo.EnsureSimPortfolioV2Definitions(ctx); err != nil {
		return nil, err
	}
	market := normalizeSimPortfolioV2Market(req.Market)
	if strings.EqualFold(strings.TrimSpace(req.Market), "ALL") {
		market = "ALL"
	}
	fromDate, toDate := strings.TrimSpace(req.FromDate), strings.TrimSpace(req.ToDate)
	if fromDate == "" {
		fromDate = time.Now().In(beijingLocation()).Format("2006-01-02")
	}
	if toDate == "" {
		toDate = fromDate
	}
	runID := fmt.Sprintf("spv2-%d", time.Now().UnixNano())
	stagesJSON, _ := json.Marshal(req.Stages)
	now := time.Now().UTC()
	run := &SimPortfolioV2PipelineRun{ID: runID, TriggerType: SimPortfolioV2TriggerAdmin, Market: market, FromDate: fromDate, ToDate: toDate, StageScope: string(stagesJSON), Status: SimPortfolioV2StatusRunning, StartedAt: now, CreatedAt: now, UpdatedAt: now}
	if err := s.repo.CreateSimPortfolioV2Run(ctx, run); err != nil {
		return nil, err
	}
	resp := &SimPortfolioV2RunResponse{OK: true, RunID: runID, Status: SimPortfolioV2StatusOK}
	markets := []string{market}
	if market == "ALL" {
		markets = []string{SimPortfolioV2MarketAShare, SimPortfolioV2MarketHKEX}
	}
	for _, m := range markets {
		for _, date := range enumerateDates(fromDate, toDate) {
			resp.ProcessedDays++
			generated, blocked, err := s.runMarketDate(ctx, runID, m, date)
			if err != nil {
				blocked = true
			}
			if generated {
				resp.GeneratedFacts++
			}
			if blocked {
				resp.BlockedDays++
			}
		}
	}
	if resp.BlockedDays > 0 {
		resp.Status = SimPortfolioV2StatusBlocked
		resp.Message = fmt.Sprintf("Pipeline 已运行，%d 天存在阻断。", resp.BlockedDays)
	} else {
		resp.Message = "Pipeline 已运行完成。"
	}
	run.Status = resp.Status
	run.FinishedAt = time.Now().UTC()
	run.UpdatedAt = run.FinishedAt
	summary, _ := json.Marshal(resp)
	run.SummaryJSON = string(summary)
	_ = s.repo.UpdateSimPortfolioV2Run(ctx, run)
	return resp, nil
}

func enumerateDates(fromDate, toDate string) []string {
	from, ok := parseYMD(fromDate)
	if !ok {
		return nil
	}
	to, ok := parseYMD(toDate)
	if !ok || to.Before(from) {
		to = from
	}
	out := []string{}
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		out = append(out, formatYMD(d))
	}
	return out
}

func (s *SimPortfolioV2Service) writeStatus(ctx context.Context, runID, market, date, stage, status, message, hint string, expected, actual, missing int) {
	now := time.Now().UTC()
	_ = s.repo.UpsertSimPortfolioV2DayStatus(ctx, SimPortfolioV2PipelineDayStatus{Market: market, TradeDate: date, Stage: stage, Status: status, ExpectedCount: expected, ActualCount: actual, MissingCount: missing, Message: message, ActionHint: hint, RunID: runID, CreatedAt: now, UpdatedAt: now})
}

func (s *SimPortfolioV2Service) runMarketDate(ctx context.Context, runID, market, date string) (bool, bool, error) {
	cal := s.calendar.CalendarRow(market, date)
	_ = s.repo.UpsertMarketCalendar(ctx, cal)
	if !cal.IsTradingDay {
		s.writeStatus(ctx, runID, market, date, SimPortfolioV2StageCalendar, SimPortfolioV2StatusSkipped, "休市日，跳过模拟组合 pipeline。", "无需操作。", 0, 0, 0)
		return false, false, nil
	}
	s.writeStatus(ctx, runID, market, date, SimPortfolioV2StageCalendar, SimPortfolioV2StatusOK, "交易日。", "", 1, 1, 0)
	batch, err := s.buildSignalBatch(ctx, runID, market, date)
	if err != nil || batch == nil || batch.Status != SimPortfolioV2StatusOK {
		return false, true, err
	}
	defs, err := s.repo.ListActiveSimPortfolioV2Definitions(ctx)
	if err != nil {
		return false, true, err
	}
	generated := false
	for _, def := range defs {
		if def.Market != market {
			continue
		}
		ok, err := s.runDefinitionDate(ctx, runID, def, date)
		if err != nil {
			return generated, true, err
		}
		generated = generated || ok
	}
	return generated, false, nil
}

func (s *SimPortfolioV2Service) buildSignalBatch(ctx context.Context, runID, market, date string) (*SimPortfolioV2SignalBatch, error) {
	rows, err := s.repo.ListRankingSnapshotsByDate(ctx, market, date, 50)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	batchID := fmt.Sprintf("sig-%s-%s", strings.ToLower(market), date)
	items := []SimPortfolioV2SignalItem{}
	missingPrice := 0
	for _, row := range rows {
		if row.ClosePrice <= 0 || strings.TrimSpace(row.PriceTradeDate) != date {
			missingPrice++
		}
		items = append(items, SimPortfolioV2SignalItem{BatchID: batchID, Market: market, SourceTradeDate: date, Code: row.Code, Exchange: row.Exchange, Name: row.Name, Rank: row.Rank, Opportunity: row.Opportunity, Risk: row.Risk, ClosePrice: row.ClosePrice, PriceTradeDate: row.PriceTradeDate, Board: inferSimPortfolioBoard(row.Code, row.Exchange), CreatedAt: now})
	}
	status := SimPortfolioV2StatusOK
	message := "信号快照已就绪。"
	if len(items) == 0 {
		status = SimPortfolioV2StatusBlocked
		message = "缺少模拟组合信号快照。"
	}
	if missingPrice > 0 {
		status = SimPortfolioV2StatusBlocked
		message = "信号快照存在缺失或非精确收盘价。"
	}
	batch := SimPortfolioV2SignalBatch{ID: batchID, Market: market, SourceTradeDate: date, ComputedAt: now, Status: status, CandidateCount: len(rows), SignalCount: len(items), MissingPriceCount: missingPrice, Message: message, CreatedAt: now, UpdatedAt: now}
	if err := s.repo.ReplaceSimPortfolioV2SignalBatch(ctx, batch, items); err != nil {
		return nil, err
	}
	if status != SimPortfolioV2StatusOK {
		s.writeStatus(ctx, runID, market, date, SimPortfolioV2StageSignal, status, message, "请先重跑该市场该交易日四象限/排行榜快照。", 1, 0, 1)
	} else {
		s.writeStatus(ctx, runID, market, date, SimPortfolioV2StageSignal, status, message, "", 1, 1, 0)
	}
	return &batch, nil
}

func (s *SimPortfolioV2Service) runDefinitionDate(ctx context.Context, runID string, def SimPortfolioV2Definition, signalDate string) (bool, error) {
	items, err := s.selectV2Items(ctx, def, signalDate)
	if err != nil {
		return false, err
	}
	entryDate := s.calendar.NextTradingDay(def.Market, signalDate)
	now := time.Now().UTC()
	batchID := fmt.Sprintf("sel-%s-%s", def.ID, signalDate)
	status := SimPortfolioV2StatusOK
	warning := ""
	if len(items) < def.MaxHoldings {
		status = SimPortfolioV2StatusBlocked
		warning = defaultRankingPortfolioWarningText
	}
	selItems := []SimPortfolioV2SelectionItem{}
	for i, item := range items {
		selItems = append(selItems, SimPortfolioV2SelectionItem{SelectionBatchID: batchID, PortfolioID: def.ID, SignalDate: signalDate, EntryTradeDate: entryDate, Code: item.Code, Exchange: item.Exchange, Name: item.Name, Rank: i + 1, SourceRank: item.Rank, Weight: 1 / float64(def.MaxHoldings), Board: item.Board, CreatedAt: now})
	}
	batch := SimPortfolioV2SelectionBatch{ID: batchID, PortfolioID: def.ID, Market: def.Market, SignalDate: signalDate, EntryTradeDate: entryDate, Status: status, SelectedCount: len(selItems), WarningText: warning, CreatedAt: now, UpdatedAt: now}
	if err := s.repo.ReplaceSimPortfolioV2Selection(ctx, batch, selItems); err != nil {
		return false, err
	}
	if status != SimPortfolioV2StatusOK {
		s.writeStatus(ctx, runID, def.Market, signalDate, SimPortfolioV2StageSelection, status, warning, "检查选股规则或补齐信号快照。", def.MaxHoldings, len(selItems), def.MaxHoldings-len(selItems))
		return false, nil
	}
	s.writeStatus(ctx, runID, def.Market, signalDate, SimPortfolioV2StageSelection, SimPortfolioV2StatusOK, "选股完成。", "", def.MaxHoldings, len(selItems), 0)
	if entryDate == "" {
		s.writeStatus(ctx, runID, def.Market, signalDate, SimPortfolioV2StagePriceRequirements, SimPortfolioV2StatusBlocked, "缺少下一交易日。", "检查市场交易日历。", 1, 0, 1)
		return false, nil
	}
	reqs := s.buildPriceRequirements(def, signalDate, entryDate, selItems)
	if err := s.repo.ReplaceSimPortfolioV2PriceRequirements(ctx, def.ID, signalDate, reqs); err != nil {
		return false, err
	}
	s.writeStatus(ctx, runID, def.Market, signalDate, SimPortfolioV2StagePriceRequirements, SimPortfolioV2StatusOK, "价格需求已生成。", "", len(reqs), len(reqs), 0)
	missingOpen, missingClose, err := s.resolvePriceRequirements(ctx, def.ID, signalDate)
	if err != nil {
		return false, err
	}
	if missingOpen > 0 {
		s.writeStatus(ctx, runID, def.Market, signalDate, SimPortfolioV2StageEntryOpen, SimPortfolioV2StatusBlocked, "建仓开盘价未就绪。", "等待行情源更新后重新运行 pipeline。", len(selItems), len(selItems)-missingOpen, missingOpen)
		return false, nil
	}
	s.writeStatus(ctx, runID, def.Market, signalDate, SimPortfolioV2StageEntryOpen, SimPortfolioV2StatusOK, "建仓开盘价已就绪。", "", len(selItems), len(selItems), 0)
	if missingClose > 0 {
		s.writeStatus(ctx, runID, def.Market, signalDate, SimPortfolioV2StageValuationClose, SimPortfolioV2StatusBlocked, "估值收盘价未就绪。", "等待收盘行情源更新后重新运行 pipeline。", len(selItems), len(selItems)-missingClose, missingClose)
		return false, nil
	}
	s.writeStatus(ctx, runID, def.Market, signalDate, SimPortfolioV2StageValuationClose, SimPortfolioV2StatusOK, "估值收盘价已就绪。", "", len(selItems), len(selItems), 0)
	if err := s.generateFacts(ctx, def, signalDate, entryDate); err != nil {
		s.writeStatus(ctx, runID, def.Market, signalDate, SimPortfolioV2StageFacts, SimPortfolioV2StatusFailed, err.Error(), "检查价格需求和事实表一致性。", 1, 0, 1)
		return false, err
	}
	s.writeStatus(ctx, runID, def.Market, signalDate, SimPortfolioV2StageFacts, SimPortfolioV2StatusOK, "事实表已生成。", "", 1, 1, 0)
	s.writeStatus(ctx, runID, def.Market, signalDate, SimPortfolioV2StageVerify, SimPortfolioV2StatusOK, "验证通过。", "", 1, 1, 0)
	return true, nil
}

type v2SignalSelectionItem struct {
	Rank                        int
	Code, Name, Exchange, Board string
	ConsecutiveDays             int
}

func (s *SimPortfolioV2Service) selectV2Items(ctx context.Context, def SimPortfolioV2Definition, signalDate string) ([]v2SignalSelectionItem, error) {
	batch, err := s.repo.GetSimPortfolioV2SignalBatch(ctx, def.Market, signalDate)
	if err != nil || batch == nil {
		return nil, err
	}
	rows, err := s.repo.ListSimPortfolioV2SignalItems(ctx, batch.ID)
	if err != nil {
		return nil, err
	}
	excluded := decodeSimPortfolioExcludedBoards(def.ExcludedBoards)
	items := []v2SignalSelectionItem{}
	for _, row := range rows {
		board := inferSimPortfolioBoard(row.Code, row.Exchange)
		if _, ok := excluded[strings.ToUpper(board)]; ok {
			continue
		}
		item := v2SignalSelectionItem{Rank: row.Rank, Code: row.Code, Name: row.Name, Exchange: row.Exchange, Board: board}
		if def.SelectionRule == rankingPortfolioSelectionRuleTop10ByStreak {
			days, err := s.repo.GetConsecutiveDaysAsOf(ctx, item.Code, resolveSimPortfolioExchangeCodes(def.Market), signalDate)
			if err != nil {
				return nil, err
			}
			item.ConsecutiveDays = days
		}
		items = append(items, item)
	}
	if def.SelectionWindow > 0 && len(items) > def.SelectionWindow {
		items = items[:def.SelectionWindow]
	}
	if def.SelectionRule == rankingPortfolioSelectionRuleTop10ByStreak {
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].ConsecutiveDays == items[j].ConsecutiveDays {
				return items[i].Rank < items[j].Rank
			}
			return items[i].ConsecutiveDays > items[j].ConsecutiveDays
		})
	}
	if len(items) > def.MaxHoldings {
		items = items[:def.MaxHoldings]
	}
	return items, nil
}

func (s *SimPortfolioV2Service) buildPriceRequirements(def SimPortfolioV2Definition, signalDate, entryDate string, items []SimPortfolioV2SelectionItem) []SimPortfolioV2PriceRequirement {
	now := time.Now().UTC()
	out := []SimPortfolioV2PriceRequirement{}
	for _, item := range items {
		for _, priceType := range []string{SimPortfolioV2PriceTypeEntryOpen, SimPortfolioV2PriceTypeValuationClose} {
			out = append(out, SimPortfolioV2PriceRequirement{PortfolioID: def.ID, Market: def.Market, SignalDate: signalDate, TradeDate: entryDate, Code: item.Code, Exchange: item.Exchange, PriceType: priceType, Required: true, Status: SimPortfolioV2PriceStatusPending, CreatedAt: now, UpdatedAt: now})
		}
	}
	return out
}

func (s *SimPortfolioV2Service) resolvePriceRequirements(ctx context.Context, portfolioID, signalDate string) (int, int, error) {
	reqs, err := s.repo.ListSimPortfolioV2PriceRequirements(ctx, portfolioID, signalDate)
	if err != nil {
		return 0, 0, err
	}
	missingOpen, missingClose := 0, 0
	for _, req := range reqs {
		s.resolveSinglePriceRequirement(ctx, &req)
		if err := s.repo.UpdateSimPortfolioV2PriceRequirement(ctx, req); err != nil {
			return 0, 0, err
		}
		if req.Status != SimPortfolioV2PriceStatusSatisfied {
			if req.PriceType == SimPortfolioV2PriceTypeEntryOpen {
				missingOpen++
			} else {
				missingClose++
			}
		}
	}
	return missingOpen, missingClose, nil
}

func (s *SimPortfolioV2Service) resolveSinglePriceRequirement(ctx context.Context, req *SimPortfolioV2PriceRequirement) {
	price := 0.0
	source := ""
	priceDate := req.TradeDate
	if override, _ := s.repo.GetSimPortfolioV2PriceOverride(ctx, req.Market, req.Code, req.Exchange, req.TradeDate, req.PriceType); override != nil && override.Price > 0 {
		price = override.Price
		priceDate = override.TradeDate
		source = "admin_override"
	} else if req.PriceType == SimPortfolioV2PriceTypeEntryOpen {
		if s.openPriceResolver != nil {
			price = s.openPriceResolver(ctx, req.Code, req.Exchange, req.TradeDate)
			source = "open_price_resolver"
		}
	} else {
		if s.priceLookupResolver != nil {
			lookup := s.priceLookupResolver(ctx, req.Code, req.Exchange, req.TradeDate)
			if lookup.ClosePrice > 0 && strings.TrimSpace(lookup.TradeDate) == req.TradeDate {
				price = lookup.ClosePrice
				priceDate = lookup.TradeDate
				source = "price_lookup_resolver"
			}
		}
		if price <= 0 && s.priceResolver != nil {
			price = s.priceResolver(ctx, req.Code, req.Exchange, req.TradeDate)
			source = "price_resolver"
		}
	}
	req.UpdatedAt = time.Now().UTC()
	if price > 0 {
		req.Price = price
		req.PriceTradeDate = priceDate
		req.Source = source
		req.Status = SimPortfolioV2PriceStatusSatisfied
		req.ResolvedAt = req.UpdatedAt
		req.MissingReason = ""
	} else {
		req.Price = 0
		req.PriceTradeDate = ""
		req.Source = ""
		req.Status = SimPortfolioV2PriceStatusMissing
		req.MissingReason = fmt.Sprintf("%s 在 %s 缺少 %s", req.Code, req.TradeDate, req.PriceType)
	}
}

func (s *SimPortfolioV2Service) generateFacts(ctx context.Context, def SimPortfolioV2Definition, signalDate, tradeDate string) error {
	items, err := s.repo.ListSimPortfolioV2SelectionItems(ctx, def.ID, signalDate)
	if err != nil {
		return err
	}
	reqs, err := s.repo.ListSimPortfolioV2PriceRequirements(ctx, def.ID, signalDate)
	if err != nil {
		return err
	}
	priceByKey := map[string]float64{}
	for _, req := range reqs {
		if req.Status != SimPortfolioV2PriceStatusSatisfied || req.Price <= 0 {
			return fmt.Errorf("price requirement not satisfied: %s %s %s", req.Code, req.TradeDate, req.PriceType)
		}
		priceByKey[req.Code+"\x00"+req.Exchange+"\x00"+req.PriceType] = req.Price
	}
	previous, err := s.repo.GetLatestSimPortfolioV2Daily(ctx, def.ID)
	if err != nil {
		return err
	}
	previousAssets := def.InitialAssets
	if previous != nil && previous.TotalAssets > 0 {
		previousAssets = previous.TotalAssets
	}
	targetValue := previousAssets / float64(def.MaxHoldings)
	now := time.Now().UTC()
	positions := []SimPortfolioV2Position{}
	trades := []SimPortfolioV2Trade{}
	total := 0.0
	for _, item := range items {
		open := priceByKey[item.Code+"\x00"+item.Exchange+"\x00"+SimPortfolioV2PriceTypeEntryOpen]
		closep := priceByKey[item.Code+"\x00"+item.Exchange+"\x00"+SimPortfolioV2PriceTypeValuationClose]
		shares := targetValue / open
		marketValue := shares * closep
		profit := marketValue - targetValue
		total += marketValue
		positions = append(positions, SimPortfolioV2Position{PortfolioID: def.ID, Market: def.Market, TradeDate: tradeDate, SignalDate: signalDate, Code: item.Code, Exchange: item.Exchange, Name: item.Name, Rank: item.Rank, Weight: item.Weight, TargetValue: targetValue, Shares: shares, BuyPrice: open, ClosePrice: closep, MarketValue: marketValue, Profit: profit, ProfitRate: profit / targetValue, CreatedAt: now, UpdatedAt: now})
		trades = append(trades, SimPortfolioV2Trade{PortfolioID: def.ID, Market: def.Market, TradeDate: tradeDate, SignalDate: signalDate, Code: item.Code, Exchange: item.Exchange, Name: item.Name, Action: simPortfolioActionBuy, NewWeight: item.Weight, TradePrice: open, TargetValue: targetValue, NewShares: shares, ShareDelta: shares, Reason: simPortfolioReasonEnterTop4, CreatedAt: now, UpdatedAt: now})
	}
	dailyReturn := 0.0
	if previousAssets > 0 {
		dailyReturn = total/previousAssets - 1
	}
	daily := SimPortfolioV2Daily{PortfolioID: def.ID, Market: def.Market, TradeDate: tradeDate, SignalDate: signalDate, SourceTradeDate: tradeDate, NAV: roundTo(total/def.InitialAssets, simPortfolioNavPrecision), TotalAssets: total, PreviousAssets: previousAssets, DailyReturn: dailyReturn, TotalReturn: total/def.InitialAssets - 1, PositionCount: len(positions), Rebalance: true, Status: "verified", ComputedAt: now, CreatedAt: now, UpdatedAt: now}
	metrics := SimPortfolioV2Metrics{PortfolioID: def.ID, Market: def.Market, TradeDate: tradeDate, NAV: daily.NAV, CreatedAt: now, UpdatedAt: now}
	return s.repo.ReplaceSimPortfolioV2FactDate(ctx, daily, positions, trades, metrics)
}

func (s *SimPortfolioV2Service) RetryResolvePrices(ctx context.Context, req SimPortfolioV2PriceRepairRequest) (*SimPortfolioV2PriceRepairResponse, error) {
	market, signalDate, priceType, err := validateSimPortfolioV2PriceRepairScope(req.Market, req.SignalDate, req.PriceType)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.ListSimPortfolioV2PriceRequirementsForRepair(ctx, market, signalDate, req.PortfolioID, priceType, req.OnlyMissing)
	if err != nil {
		return nil, err
	}
	audit := newSimPortfolioV2PriceRepairAudit(SimPortfolioV2PriceRepairRetryResolve, market, signalDate, req.PortfolioID, priceType, req.Operator)
	if err := s.repo.CreateSimPortfolioV2PriceRepairAudit(ctx, audit); err != nil {
		return nil, err
	}
	resp := &SimPortfolioV2PriceRepairResponse{OK: true, Action: SimPortfolioV2PriceRepairRetryResolve, Status: SimPortfolioV2StatusOK, AuditID: audit.ID, RequiresRerun: true}
	for _, row := range rows {
		before := row.Status
		s.resolveSinglePriceRequirement(ctx, &row)
		if err := s.repo.UpdateSimPortfolioV2PriceRequirement(ctx, row); err != nil {
			return nil, err
		}
		resp.Checked++
		if row.Status == SimPortfolioV2PriceStatusSatisfied {
			resp.Satisfied++
			if before != SimPortfolioV2PriceStatusSatisfied {
				resp.Updated++
			}
		} else {
			resp.StillMissing++
			resp.MissingItems = append(resp.MissingItems, simPortfolioV2MissingPriceText(row))
		}
	}
	if resp.StillMissing > 0 {
		resp.Status = SimPortfolioV2StatusBlocked
	}
	resp.Message = fmt.Sprintf("价格重新解析完成：检查 %d 条，满足 %d 条，仍缺 %d 条。请重新运行该日 pipeline 生成 facts。", resp.Checked, resp.Satisfied, resp.StillMissing)
	finishSimPortfolioV2PriceRepairAudit(audit, resp, "")
	_ = s.repo.UpdateSimPortfolioV2PriceRepairAudit(ctx, audit)
	return resp, nil
}

func (s *SimPortfolioV2Service) BackfillDailyBars(ctx context.Context, req SimPortfolioV2PriceBackfillRequest) (*SimPortfolioV2PriceRepairResponse, error) {
	market, signalDate, priceType, err := validateSimPortfolioV2PriceRepairScope(req.Market, req.SignalDate, req.PriceType)
	if err != nil {
		return nil, err
	}
	if s.dailyBarFetcher == nil {
		return nil, fmt.Errorf("历史日线重拉器未配置")
	}
	lookback := req.LookbackDays
	if lookback <= 0 {
		lookback = 90
	}
	rows, err := s.repo.ListSimPortfolioV2PriceRequirementsForRepair(ctx, market, signalDate, req.PortfolioID, priceType, req.OnlyMissing)
	if err != nil {
		return nil, err
	}
	audit := newSimPortfolioV2PriceRepairAudit(SimPortfolioV2PriceRepairBackfillDailyBar, market, signalDate, req.PortfolioID, priceType, req.Operator)
	if err := s.repo.CreateSimPortfolioV2PriceRepairAudit(ctx, audit); err != nil {
		return nil, err
	}
	resp := &SimPortfolioV2PriceRepairResponse{OK: true, Action: SimPortfolioV2PriceRepairBackfillDailyBar, Status: SimPortfolioV2StatusOK, AuditID: audit.ID, RequiresRerun: true}
	for _, row := range rows {
		bars, fetchErr := s.dailyBarFetcher(ctx, row.Code, row.Exchange, lookback)
		resp.Checked++
		if fetchErr == nil {
			if price := simPortfolioV2PriceFromBars(bars, row.TradeDate, row.PriceType); price > 0 {
				row.Price = price
				row.PriceTradeDate = row.TradeDate
				row.Source = "daily_bar_backfill"
				row.Status = SimPortfolioV2PriceStatusSatisfied
				row.MissingReason = ""
				now := time.Now().UTC()
				row.ResolvedAt = now
				row.UpdatedAt = now
				resp.Satisfied++
				resp.Updated++
				resp.Backfilled++
				if err := s.repo.UpdateSimPortfolioV2PriceRequirement(ctx, row); err != nil {
					return nil, err
				}
				continue
			}
		}
		row.Status = SimPortfolioV2PriceStatusMissing
		row.MissingReason = fmt.Sprintf("%s 在 %s 重拉历史日线后仍缺少 %s", row.Code, row.TradeDate, row.PriceType)
		row.UpdatedAt = time.Now().UTC()
		_ = s.repo.UpdateSimPortfolioV2PriceRequirement(ctx, row)
		resp.StillMissing++
		resp.MissingItems = append(resp.MissingItems, simPortfolioV2MissingPriceText(row))
	}
	if resp.StillMissing > 0 {
		resp.Status = SimPortfolioV2StatusBlocked
	}
	resp.Message = fmt.Sprintf("历史日线重拉完成：检查 %d 条，补齐 %d 条，仍缺 %d 条。请重新运行该日 pipeline 生成 facts。", resp.Checked, resp.Backfilled, resp.StillMissing)
	finishSimPortfolioV2PriceRepairAudit(audit, resp, "")
	_ = s.repo.UpdateSimPortfolioV2PriceRepairAudit(ctx, audit)
	return resp, nil
}

func (s *SimPortfolioV2Service) OverridePrice(ctx context.Context, req SimPortfolioV2PriceOverrideRequest) (*SimPortfolioV2PriceRepairResponse, error) {
	market, signalDate, priceType, err := validateSimPortfolioV2PriceRepairScope(req.Market, req.SignalDate, req.PriceType)
	if err != nil {
		return nil, err
	}
	if priceType == "" {
		return nil, fmt.Errorf("人工覆盖必须指定 price_type")
	}
	code := strings.TrimSpace(req.Code)
	exchange := strings.ToUpper(strings.TrimSpace(req.Exchange))
	tradeDate := strings.TrimSpace(req.TradeDate)
	if code == "" || exchange == "" || !isYMD(tradeDate) {
		return nil, fmt.Errorf("code/exchange/trade_date 必填且日期格式必须为 YYYY-MM-DD")
	}
	if req.Price <= 0 {
		return nil, fmt.Errorf("人工覆盖价格必须大于 0")
	}
	if strings.TrimSpace(req.Reason) == "" || strings.TrimSpace(req.Evidence) == "" {
		return nil, fmt.Errorf("人工覆盖必须填写 reason 和 evidence")
	}
	if !req.Confirm {
		return nil, fmt.Errorf("人工覆盖价格必须显式 confirm=true")
	}
	if tradeDate > time.Now().In(beijingLocation()).Format("2006-01-02") {
		return nil, fmt.Errorf("不能覆盖未来日期价格")
	}
	rows, err := s.repo.ListSimPortfolioV2PriceRequirementsForRepair(ctx, market, signalDate, req.PortfolioID, priceType, false)
	if err != nil {
		return nil, err
	}
	audit := newSimPortfolioV2PriceRepairAudit(SimPortfolioV2PriceRepairManualOverride, market, signalDate, req.PortfolioID, priceType, req.Operator)
	audit.Code, audit.Exchange, audit.TradeDate, audit.Price = code, exchange, tradeDate, req.Price
	audit.Reason, audit.Evidence = strings.TrimSpace(req.Reason), strings.TrimSpace(req.Evidence)
	if err := s.repo.CreateSimPortfolioV2PriceRepairAudit(ctx, audit); err != nil {
		return nil, err
	}
	override := SimPortfolioV2PriceOverride{Market: market, Code: code, Exchange: exchange, TradeDate: tradeDate, PriceType: priceType, Price: req.Price, Reason: strings.TrimSpace(req.Reason), Evidence: strings.TrimSpace(req.Evidence), Operator: defaultSimPortfolioV2Operator(req.Operator), AuditID: audit.ID, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := s.repo.UpsertSimPortfolioV2PriceOverride(ctx, override); err != nil {
		return nil, err
	}
	resp := &SimPortfolioV2PriceRepairResponse{OK: true, Action: SimPortfolioV2PriceRepairManualOverride, Status: SimPortfolioV2StatusOK, AuditID: audit.ID, RequiresRerun: true}
	for _, row := range rows {
		if strings.TrimSpace(row.Code) != code || strings.ToUpper(strings.TrimSpace(row.Exchange)) != exchange || row.TradeDate != tradeDate || row.PriceType != priceType {
			continue
		}
		row.Price = req.Price
		row.PriceTradeDate = tradeDate
		row.Source = "admin_override"
		row.Status = SimPortfolioV2PriceStatusSatisfied
		row.MissingReason = ""
		now := time.Now().UTC()
		row.ResolvedAt = now
		row.UpdatedAt = now
		if err := s.repo.UpdateSimPortfolioV2PriceRequirement(ctx, row); err != nil {
			return nil, err
		}
		resp.Checked++
		resp.Satisfied++
		resp.Updated++
	}
	resp.Message = fmt.Sprintf("人工价格覆盖已记录审计并更新 %d 条价格需求。请重新运行该日 pipeline 生成 facts。", resp.Updated)
	finishSimPortfolioV2PriceRepairAudit(audit, resp, "")
	_ = s.repo.UpdateSimPortfolioV2PriceRepairAudit(ctx, audit)
	return resp, nil
}

func validateSimPortfolioV2PriceRepairScope(market, signalDate, priceType string) (string, string, string, error) {
	market = normalizeSimPortfolioV2Market(market)
	signalDate = strings.TrimSpace(signalDate)
	priceType = strings.TrimSpace(priceType)
	if market != SimPortfolioV2MarketAShare && market != SimPortfolioV2MarketHKEX {
		return "", "", "", fmt.Errorf("market 仅支持 ASHARE/HKEX")
	}
	if !isYMD(signalDate) {
		return "", "", "", fmt.Errorf("signal_date 必须为 YYYY-MM-DD")
	}
	if priceType == "" {
		return market, signalDate, priceType, nil
	}
	if priceType != SimPortfolioV2PriceTypeEntryOpen && priceType != SimPortfolioV2PriceTypeValuationClose {
		return "", "", "", fmt.Errorf("price_type 仅支持 entry_open/valuation_close")
	}
	return market, signalDate, priceType, nil
}

func simPortfolioV2PriceFromBars(bars []SimPortfolioV2DailyBar, tradeDate, priceType string) float64 {
	for _, bar := range bars {
		if strings.TrimSpace(bar.Date) != tradeDate {
			continue
		}
		if priceType == SimPortfolioV2PriceTypeEntryOpen {
			return bar.Open
		}
		return bar.Close
	}
	return 0
}

func newSimPortfolioV2PriceRepairAudit(action, market, signalDate, portfolioID, priceType, operator string) *SimPortfolioV2PriceRepairAudit {
	now := time.Now().UTC()
	return &SimPortfolioV2PriceRepairAudit{Action: action, Market: market, SignalDate: signalDate, PortfolioID: strings.TrimSpace(portfolioID), PriceType: priceType, Status: SimPortfolioV2StatusRunning, Operator: defaultSimPortfolioV2Operator(operator), CreatedAt: now, UpdatedAt: now}
}

func finishSimPortfolioV2PriceRepairAudit(audit *SimPortfolioV2PriceRepairAudit, resp *SimPortfolioV2PriceRepairResponse, errMsg string) {
	audit.Status = resp.Status
	audit.ErrorMessage = errMsg
	audit.UpdatedAt = time.Now().UTC()
	summary, _ := json.Marshal(resp)
	audit.SummaryJSON = string(summary)
}

func defaultSimPortfolioV2Operator(operator string) string {
	if strings.TrimSpace(operator) == "" {
		return "admin"
	}
	return strings.TrimSpace(operator)
}

func simPortfolioV2MissingPriceText(row SimPortfolioV2PriceRequirement) string {
	return fmt.Sprintf("%s/%s %s %s", row.Code, row.Exchange, row.TradeDate, row.PriceType)
}

func isYMD(value string) bool {
	_, ok := parseYMD(strings.TrimSpace(value))
	return ok
}

func (s *SimPortfolioV2Service) GetAdminOverview(ctx context.Context) (*SimPortfolioV2OverviewResponse, error) {
	defs, err := s.repo.ListActiveSimPortfolioV2Definitions(ctx)
	if err != nil {
		return nil, err
	}
	statuses, _ := s.repo.ListSimPortfolioV2DayStatuses(ctx, "", "", "")
	latestBlock := map[string]SimPortfolioV2PipelineDayStatus{}
	for _, st := range statuses {
		if st.Status == SimPortfolioV2StatusBlocked || st.Status == SimPortfolioV2StatusFailed {
			if old, ok := latestBlock[st.Market]; !ok || st.TradeDate > old.TradeDate {
				latestBlock[st.Market] = st
			}
		}
	}
	_ = latestBlock
	resp := &SimPortfolioV2OverviewResponse{Items: []SimPortfolioV2MarketOverview{}}
	seen := map[string]bool{}
	for _, def := range defs {
		if seen[def.Market] {
			continue
		}
		seen[def.Market] = true
		item := SimPortfolioV2MarketOverview{Market: def.Market, Status: SimPortfolioV2StatusPending, StatusText: "等待初始化"}
		if latest, _ := s.repo.GetLatestSimPortfolioV2Daily(ctx, def.ID); latest != nil {
			item.Status = SimPortfolioV2StatusOK
			item.StatusText = "已有 verified 事实表"
			item.LatestVerifiedTradeDate = latest.TradeDate
			item.LatestSignalDate = latest.SignalDate
		}
		if block, ok := latestBlock[def.Market]; ok {
			item.Status = block.Status
			item.BlockingStage = block.Stage
			item.BlockingMessage = block.Message
			item.StatusText = block.Message
		}
		resp.Items = append(resp.Items, item)
	}
	runs, _ := s.repo.ListSimPortfolioV2Runs(ctx, 10)
	for _, run := range runs {
		resp.Runs = append(resp.Runs, SimPortfolioV2RunItem{RunID: run.ID, Market: run.Market, Status: run.Status, FromDate: run.FromDate, ToDate: run.ToDate, StartedAt: run.StartedAt.Format(time.RFC3339), FinishedAt: run.FinishedAt.Format(time.RFC3339)})
	}
	return resp, nil
}

func (s *SimPortfolioV2Service) GetAdminDays(ctx context.Context, market, fromDate, toDate string) (*SimPortfolioV2DaysResponse, error) {
	rows, err := s.repo.ListSimPortfolioV2DayStatuses(ctx, market, fromDate, toDate)
	if err != nil {
		return nil, err
	}
	resp := &SimPortfolioV2DaysResponse{}
	for _, row := range rows {
		resp.Items = append(resp.Items, SimPortfolioV2DayStatusItem{Market: row.Market, TradeDate: row.TradeDate, Stage: row.Stage, Status: row.Status, ExpectedCount: row.ExpectedCount, ActualCount: row.ActualCount, MissingCount: row.MissingCount, Message: row.Message, ActionHint: row.ActionHint})
	}
	return resp, nil
}

func (s *SimPortfolioV2Service) GetPortfolioOverview(ctx context.Context) (*SimPortfolioOverviewResponse, error) {
	if err := s.repo.EnsureSimPortfolioV2Definitions(ctx); err != nil {
		return nil, err
	}
	defs, err := s.repo.ListActiveSimPortfolioV2Definitions(ctx)
	if err != nil {
		return nil, err
	}
	resp := &SimPortfolioOverviewResponse{Items: []SimPortfolioOverviewItem{}}
	for _, def := range defs {
		latest, err := s.repo.GetLatestSimPortfolioV2Daily(ctx, def.ID)
		if err != nil {
			return nil, err
		}
		metrics, err := s.repo.GetLatestSimPortfolioV2Metrics(ctx, def.ID)
		if err != nil {
			return nil, err
		}
		positions := []SimPortfolioV2Position{}
		trades := []SimPortfolioV2Trade{}
		status, statusText := SimPortfolioV2StatusPending, "等待 Sim Portfolio v2 pipeline 生成 verified 数据"
		if latest != nil {
			status = simPortfolioStatusComplete
			statusText = "已完成最新 verified 收盘估值"
			positions, _ = s.repo.ListSimPortfolioV2PositionsByTradeDate(ctx, def.ID, latest.TradeDate)
			trades, _ = s.repo.ListSimPortfolioV2Trades(ctx, def.ID, "", "", "")
		}
		item := SimPortfolioOverviewItem{PortfolioID: def.ID, Code: def.Code, Name: def.Name, Exchange: def.Market, PortfolioVariant: def.PortfolioVariant, SelectionRule: def.SelectionRule, SelectionWindow: def.SelectionWindow, InitialAssets: def.InitialAssets, PositionCount: def.MaxHoldings, Status: status, StatusText: statusText, CurrentPositions: toV2PositionItems(positions), LatestTrades: toV2TradeItems(limitV2Trades(trades, 6))}
		if latest != nil {
			item.LatestTradeDate = latest.TradeDate
			item.LatestSignalDate = latest.SignalDate
			item.NAV = latest.NAV
			item.TotalAssets = latest.TotalAssets
			item.DailyReturn = latest.DailyReturn
			item.TotalReturn = latest.TotalReturn
			if latest.TradeDate > resp.AsOfTradeDate {
				resp.AsOfTradeDate = latest.TradeDate
			}
		}
		if metrics != nil {
			item.MaxDrawdown = metrics.MaxDrawdown
			item.Volatility = metrics.Volatility
			item.WinRate = metrics.WinRate
			item.TurnoverRate = metrics.TurnoverRate
		}
		resp.Items = append(resp.Items, item)
	}
	return resp, nil
}

func limitV2Trades(rows []SimPortfolioV2Trade, limit int) []SimPortfolioV2Trade {
	if len(rows) > limit {
		return rows[:limit]
	}
	return rows
}

func toV2PositionItems(rows []SimPortfolioV2Position) []SimPortfolioPositionItem {
	out := []SimPortfolioPositionItem{}
	for _, row := range rows {
		out = append(out, SimPortfolioPositionItem{Rank: row.Rank, StockCode: row.Code, StockName: row.Name, Exchange: row.Exchange, Weight: row.Weight, TargetValue: row.TargetValue, Shares: row.Shares, BuyPrice: row.BuyPrice, ClosePrice: row.ClosePrice, MarketValue: row.MarketValue, Profit: row.Profit, ProfitRate: row.ProfitRate})
	}
	return out
}
func toV2TradeItems(rows []SimPortfolioV2Trade) []SimPortfolioTradeItem {
	out := []SimPortfolioTradeItem{}
	for _, row := range rows {
		out = append(out, SimPortfolioTradeItem{TradeDate: row.TradeDate, SignalDate: row.SignalDate, StockCode: row.Code, StockName: row.Name, Exchange: row.Exchange, Action: row.Action, OldWeight: row.OldWeight, NewWeight: row.NewWeight, TradePrice: row.TradePrice, TargetValue: row.TargetValue, OldShares: row.OldShares, NewShares: row.NewShares, ShareDelta: row.ShareDelta, Reason: row.Reason})
	}
	return out
}

func (s *SimPortfolioV2Service) GetPortfolioDaily(ctx context.Context, portfolioID, fromDate, toDate string) (*SimPortfolioDailyResponse, error) {
	rows, err := s.repo.ListSimPortfolioV2DailyRange(ctx, portfolioID, fromDate, toDate)
	if err != nil {
		return nil, err
	}
	resp := &SimPortfolioDailyResponse{PortfolioID: portfolioID}
	for _, row := range rows {
		resp.Items = append(resp.Items, SimPortfolioDailyItem{TradeDate: row.TradeDate, SignalDate: row.SignalDate, SourceTradeDate: row.SourceTradeDate, NAV: row.NAV, TotalAssets: row.TotalAssets, DailyReturn: row.DailyReturn, TotalReturn: row.TotalReturn, PositionCount: row.PositionCount, Rebalance: row.Rebalance, Status: row.Status})
	}
	return resp, nil
}
func (s *SimPortfolioV2Service) GetPortfolioPositions(ctx context.Context, portfolioID, tradeDate string) (*SimPortfolioPositionsResponse, error) {
	if strings.TrimSpace(tradeDate) == "" {
		latest, _ := s.repo.GetLatestSimPortfolioV2Daily(ctx, portfolioID)
		if latest != nil {
			tradeDate = latest.TradeDate
		}
	}
	rows, err := s.repo.ListSimPortfolioV2PositionsByTradeDate(ctx, portfolioID, tradeDate)
	if err != nil {
		return nil, err
	}
	resp := &SimPortfolioPositionsResponse{PortfolioID: portfolioID, TradeDate: tradeDate, Items: toV2PositionItems(rows)}
	if latest, _ := s.repo.GetLatestSimPortfolioV2Daily(ctx, portfolioID); latest != nil && latest.TradeDate == tradeDate {
		resp.TotalAssets = latest.TotalAssets
	}
	return resp, nil
}
func (s *SimPortfolioV2Service) GetPortfolioTrades(ctx context.Context, portfolioID, fromDate, toDate, action string) (*SimPortfolioTradesResponse, error) {
	rows, err := s.repo.ListSimPortfolioV2Trades(ctx, portfolioID, fromDate, toDate, action)
	if err != nil {
		return nil, err
	}
	return &SimPortfolioTradesResponse{PortfolioID: portfolioID, Items: toV2TradeItems(rows)}, nil
}
func (s *SimPortfolioV2Service) GetPortfolioMetrics(ctx context.Context, portfolioID string) (*SimPortfolioMetricsResponse, error) {
	row, err := s.repo.GetLatestSimPortfolioV2Metrics(ctx, portfolioID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return &SimPortfolioMetricsResponse{PortfolioID: portfolioID}, nil
	}
	return &SimPortfolioMetricsResponse{PortfolioID: portfolioID, TradeDate: row.TradeDate, NAV: row.NAV, AnnualReturn: row.AnnualReturn, MaxDrawdown: row.MaxDrawdown, SharpeRatio: row.SharpeRatio, Volatility: row.Volatility, WinRate: row.WinRate, TurnoverRate: row.TurnoverRate}, nil
}

type SimPortfolioV2CalendarResponse struct {
	Month   string                         `json:"month"`
	Today   string                         `json:"today"`
	Markets []SimPortfolioV2MarketCalendar `json:"markets"`
}

type SimPortfolioV2MarketCalendar struct {
	Market                   string                      `json:"market"`
	Label                    string                      `json:"label"`
	StartSignalDate          string                      `json:"start_signal_date,omitempty"`
	LatestPublishedTradeDate string                      `json:"latest_published_trade_date,omitempty"`
	Days                     []SimPortfolioV2CalendarDay `json:"days"`
}

type SimPortfolioV2CalendarDay struct {
	Date                string                            `json:"date"`
	IsFuture            bool                              `json:"is_future"`
	IsTradingDay        bool                              `json:"is_trading_day"`
	HolidayName         string                            `json:"holiday_name,omitempty"`
	OverallStatus       string                            `json:"overall_status"`
	BlockingCount       int                               `json:"blocking_count"`
	CanSelectAsStart    bool                              `json:"can_select_as_start"`
	StartDisabledReason string                            `json:"start_disabled_reason,omitempty"`
	Portfolios          []SimPortfolioV2CalendarPortfolio `json:"portfolios,omitempty"`
}

type SimPortfolioV2CalendarPortfolio struct {
	PortfolioID          string `json:"portfolio_id"`
	Variant              string `json:"variant"`
	Status               string `json:"status"`
	SelectedCount        int    `json:"selected_count"`
	RequiredCount        int    `json:"required_count"`
	EntryOpenStatus      string `json:"entry_open_status"`
	ValuationCloseStatus string `json:"valuation_close_status"`
}

type SimPortfolioV2DayDetailResponse struct {
	Market            string                           `json:"market"`
	Date              string                           `json:"date"`
	IsFuture          bool                             `json:"is_future"`
	IsTradingDay      bool                             `json:"is_trading_day"`
	HolidayName       string                           `json:"holiday_name,omitempty"`
	Signal            SimPortfolioV2SignalDetail       `json:"signal"`
	Portfolios        []SimPortfolioV2PortfolioDetail  `json:"portfolios"`
	RepairSuggestions []SimPortfolioV2RepairSuggestion `json:"repair_suggestions,omitempty"`
}

type SimPortfolioV2SignalDetail struct {
	Status            string `json:"status"`
	CandidateCount    int    `json:"candidate_count"`
	SignalCount       int    `json:"signal_count"`
	MissingPriceCount int    `json:"missing_price_count"`
	Message           string `json:"message,omitempty"`
}

type SimPortfolioV2PortfolioDetail struct {
	PortfolioID       string                           `json:"portfolio_id"`
	Name              string                           `json:"name"`
	Variant           string                           `json:"variant"`
	Status            string                           `json:"status"`
	SelectedCount     int                              `json:"selected_count"`
	RequiredCount     int                              `json:"required_count"`
	EntryTradeDate    string                           `json:"entry_trade_date,omitempty"`
	EntryOpen         SimPortfolioV2PriceGroupDetail   `json:"entry_open"`
	ValuationClose    SimPortfolioV2PriceGroupDetail   `json:"valuation_close"`
	Facts             SimPortfolioV2FactsDetail        `json:"facts"`
	RepairSuggestions []SimPortfolioV2RepairSuggestion `json:"repair_suggestions,omitempty"`
}

type SimPortfolioV2PriceGroupDetail struct {
	Status         string                           `json:"status"`
	RequiredCount  int                              `json:"required_count"`
	SatisfiedCount int                              `json:"satisfied_count"`
	MissingCount   int                              `json:"missing_count"`
	MissingItems   []SimPortfolioV2MissingPriceItem `json:"missing_items,omitempty"`
}

type SimPortfolioV2MissingPriceItem struct {
	Code       string `json:"code"`
	Name       string `json:"name,omitempty"`
	Exchange   string `json:"exchange"`
	TradeDate  string `json:"trade_date"`
	PriceType  string `json:"price_type"`
	ReasonCode string `json:"reason_code"`
	Message    string `json:"message"`
}

type SimPortfolioV2FactsDetail struct {
	Status        string `json:"status"`
	DailyCount    int    `json:"daily_count"`
	PositionCount int    `json:"position_count"`
	TradeCount    int    `json:"trade_count"`
}

type SimPortfolioV2RepairSuggestion struct {
	Type  string `json:"type"`
	Label string `json:"label"`
	Hint  string `json:"hint,omitempty"`
}

type SimPortfolioV2StartDatePreviewRequest struct {
	Market          string `json:"market"`
	StartSignalDate string `json:"start_signal_date"`
}

type SimPortfolioV2StartDatePreviewResponse struct {
	Market             string                         `json:"market"`
	StartSignalDate    string                         `json:"start_signal_date"`
	CanApply           bool                           `json:"can_apply"`
	Message            string                         `json:"message"`
	AffectedPortfolios []string                       `json:"affected_portfolios"`
	Estimated          SimPortfolioV2StartEstimate    `json:"estimated"`
	BlockingReasons    []SimPortfolioV2BlockingReason `json:"blocking_reasons,omitempty"`
}

type SimPortfolioV2StartEstimate struct {
	SignalDays                 int    `json:"signal_days"`
	EntryDays                  int    `json:"entry_days"`
	DailyRows                  int    `json:"daily_rows"`
	PositionRows               int    `json:"position_rows"`
	LatestGeneratableTradeDate string `json:"latest_generatable_trade_date,omitempty"`
}

type SimPortfolioV2BlockingReason struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Action  string `json:"action,omitempty"`
}

type SimPortfolioV2StartDateApplyRequest struct {
	Market          string `json:"market"`
	StartSignalDate string `json:"start_signal_date"`
	Confirm         bool   `json:"confirm"`
	Note            string `json:"note"`
}

type SimPortfolioV2StartDateApplyResponse struct {
	OK              bool   `json:"ok"`
	JobID           string `json:"job_id"`
	Status          string `json:"status"`
	Message         string `json:"message"`
	StartSignalDate string `json:"start_signal_date"`
	Market          string `json:"market"`
}

func (s *SimPortfolioV2Service) GetAdminCalendars(ctx context.Context, month string) (*SimPortfolioV2CalendarResponse, error) {
	if err := s.repo.EnsureSimPortfolioV2Definitions(ctx); err != nil {
		return nil, err
	}
	month = strings.TrimSpace(month)
	if month == "" {
		month = time.Now().In(beijingLocation()).Format("2006-01")
	}
	start, ok := parseYMD(month + "-01")
	if !ok {
		return nil, fmt.Errorf("invalid month")
	}
	end := start.AddDate(0, 1, -1)
	today := time.Now().In(beijingLocation()).Format("2006-01-02")
	resp := &SimPortfolioV2CalendarResponse{Month: month, Today: today}
	for _, market := range []string{SimPortfolioV2MarketAShare, SimPortfolioV2MarketHKEX} {
		cal := SimPortfolioV2MarketCalendar{Market: market, Label: simPortfolioV2MarketLabel(market)}
		if cfg, _ := s.repo.GetSimPortfolioV2MarketConfig(ctx, market); cfg != nil {
			cal.StartSignalDate = cfg.StartSignalDate
			cal.LatestPublishedTradeDate = cfg.LatestPublishedTradeDate
		}
		for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
			date := formatYMD(d)
			day, err := s.buildCalendarDay(ctx, market, date, today)
			if err != nil {
				return nil, err
			}
			cal.Days = append(cal.Days, day)
		}
		resp.Markets = append(resp.Markets, cal)
	}
	return resp, nil
}

func (s *SimPortfolioV2Service) buildCalendarDay(ctx context.Context, market, date, today string) (SimPortfolioV2CalendarDay, error) {
	row := s.calendar.CalendarRow(market, date)
	_ = s.repo.UpsertMarketCalendar(ctx, row)
	day := SimPortfolioV2CalendarDay{Date: date, IsFuture: date > today, IsTradingDay: row.IsTradingDay, HolidayName: row.HolidayName, OverallStatus: SimPortfolioV2StatusPending, CanSelectAsStart: row.IsTradingDay && date <= today}
	if day.IsFuture {
		day.OverallStatus = "future"
		day.CanSelectAsStart = false
		day.StartDisabledReason = "不能选择未来日期。"
		return day, nil
	}
	if !row.IsTradingDay {
		day.OverallStatus = SimPortfolioV2StatusSkipped
		day.CanSelectAsStart = false
		day.StartDisabledReason = "该市场休市，不能作为开始信号日。"
		return day, nil
	}
	defs, err := s.repo.ListActiveSimPortfolioV2Definitions(ctx)
	if err != nil {
		return day, err
	}
	batch, _ := s.repo.GetSimPortfolioV2SignalBatch(ctx, market, date)
	if batch == nil {
		day.OverallStatus = SimPortfolioV2StatusPending
	}
	if batch != nil && batch.Status == SimPortfolioV2StatusBlocked {
		day.OverallStatus = SimPortfolioV2StatusBlocked
		day.BlockingCount++
	}
	for _, def := range defs {
		if def.Market != market {
			continue
		}
		p, err := s.buildCalendarPortfolio(ctx, def, date)
		if err != nil {
			return day, err
		}
		if p.Status == SimPortfolioV2StatusBlocked || p.Status == SimPortfolioV2StatusFailed {
			day.BlockingCount++
		}
		day.Portfolios = append(day.Portfolios, p)
	}
	day.OverallStatus = aggregateSimPortfolioV2DayStatus(day.OverallStatus, day.Portfolios, day.BlockingCount)
	return day, nil
}

func (s *SimPortfolioV2Service) buildCalendarPortfolio(ctx context.Context, def SimPortfolioV2Definition, signalDate string) (SimPortfolioV2CalendarPortfolio, error) {
	p := SimPortfolioV2CalendarPortfolio{PortfolioID: def.ID, Variant: def.PortfolioVariant, Status: SimPortfolioV2StatusPending, RequiredCount: def.MaxHoldings, EntryOpenStatus: SimPortfolioV2StatusPending, ValuationCloseStatus: SimPortfolioV2StatusPending}
	items, err := s.repo.ListSimPortfolioV2SelectionItems(ctx, def.ID, signalDate)
	if err != nil {
		return p, err
	}
	p.SelectedCount = len(items)
	if p.SelectedCount > 0 && p.SelectedCount < def.MaxHoldings {
		p.Status = SimPortfolioV2StatusBlocked
	}
	reqs, err := s.repo.ListSimPortfolioV2PriceRequirements(ctx, def.ID, signalDate)
	if err != nil {
		return p, err
	}
	p.EntryOpenStatus = priceGroupStatus(reqs, SimPortfolioV2PriceTypeEntryOpen)
	p.ValuationCloseStatus = priceGroupStatus(reqs, SimPortfolioV2PriceTypeValuationClose)
	if p.EntryOpenStatus == SimPortfolioV2PriceStatusMissing || p.ValuationCloseStatus == SimPortfolioV2PriceStatusMissing {
		p.Status = SimPortfolioV2StatusBlocked
	}
	if count, _ := s.repo.CountSimPortfolioV2DailyByMarketSignalDate(ctx, def.Market, signalDate); count > 0 && p.Status != SimPortfolioV2StatusBlocked {
		p.Status = "verified"
	}
	return p, nil
}

func priceGroupStatus(reqs []SimPortfolioV2PriceRequirement, priceType string) string {
	total, satisfied, missing := 0, 0, 0
	for _, req := range reqs {
		if req.PriceType == priceType {
			total++
			if req.Status == SimPortfolioV2PriceStatusSatisfied {
				satisfied++
			} else if req.Status == SimPortfolioV2PriceStatusMissing || req.Status == SimPortfolioV2PriceStatusFailed {
				missing++
			}
		}
	}
	if total == 0 {
		return SimPortfolioV2StatusPending
	}
	if missing > 0 {
		return SimPortfolioV2PriceStatusMissing
	}
	if satisfied == total {
		return SimPortfolioV2StatusOK
	}
	return SimPortfolioV2StatusPending
}

func aggregateSimPortfolioV2DayStatus(seed string, portfolios []SimPortfolioV2CalendarPortfolio, blocking int) string {
	if blocking > 0 {
		return SimPortfolioV2StatusBlocked
	}
	verified := 0
	for _, p := range portfolios {
		if p.Status == "verified" {
			verified++
		}
	}
	if len(portfolios) > 0 && verified == len(portfolios) {
		return "verified"
	}
	if seed == SimPortfolioV2StatusOK {
		return SimPortfolioV2StatusOK
	}
	return seed
}

func (s *SimPortfolioV2Service) GetAdminCalendarDay(ctx context.Context, market, date string) (*SimPortfolioV2DayDetailResponse, error) {
	market = normalizeSimPortfolioV2Market(market)
	date = strings.TrimSpace(date)
	if _, ok := parseYMD(date); !ok {
		return nil, fmt.Errorf("invalid date")
	}
	today := time.Now().In(beijingLocation()).Format("2006-01-02")
	cal := s.calendar.CalendarRow(market, date)
	_ = s.repo.UpsertMarketCalendar(ctx, cal)
	resp := &SimPortfolioV2DayDetailResponse{Market: market, Date: date, IsFuture: date > today, IsTradingDay: cal.IsTradingDay, HolidayName: cal.HolidayName}
	batch, _ := s.repo.GetSimPortfolioV2SignalBatch(ctx, market, date)
	if batch != nil {
		resp.Signal = SimPortfolioV2SignalDetail{Status: batch.Status, CandidateCount: batch.CandidateCount, SignalCount: batch.SignalCount, MissingPriceCount: batch.MissingPriceCount, Message: batch.Message}
	} else if cal.IsTradingDay && date <= today {
		resp.Signal = SimPortfolioV2SignalDetail{Status: SimPortfolioV2StatusBlocked, Message: "缺少该市场该交易日四象限/排行榜快照。"}
		resp.RepairSuggestions = append(resp.RepairSuggestions, SimPortfolioV2RepairSuggestion{Type: "recompute_quadrant", Label: "重建该日四象限", Hint: "请在四象限板块按 market + source_trade_date 重建上游快照。"})
	} else {
		resp.Signal = SimPortfolioV2SignalDetail{Status: SimPortfolioV2StatusSkipped, Message: "休市或未来日期。"}
	}
	defs, err := s.repo.ListActiveSimPortfolioV2Definitions(ctx)
	if err != nil {
		return nil, err
	}
	for _, def := range defs {
		if def.Market == market {
			detail, err := s.buildPortfolioDetail(ctx, def, date)
			if err != nil {
				return nil, err
			}
			resp.Portfolios = append(resp.Portfolios, detail)
		}
	}
	return resp, nil
}

func (s *SimPortfolioV2Service) buildPortfolioDetail(ctx context.Context, def SimPortfolioV2Definition, signalDate string) (SimPortfolioV2PortfolioDetail, error) {
	out := SimPortfolioV2PortfolioDetail{PortfolioID: def.ID, Name: def.Name, Variant: def.PortfolioVariant, Status: SimPortfolioV2StatusPending, RequiredCount: def.MaxHoldings}
	items, err := s.repo.ListSimPortfolioV2SelectionItems(ctx, def.ID, signalDate)
	if err != nil {
		return out, err
	}
	out.SelectedCount = len(items)
	if len(items) > 0 {
		out.EntryTradeDate = items[0].EntryTradeDate
	}
	reqs, err := s.repo.ListSimPortfolioV2PriceRequirements(ctx, def.ID, signalDate)
	if err != nil {
		return out, err
	}
	nameByKey := map[string]string{}
	for _, item := range items {
		nameByKey[item.Code+"\x00"+item.Exchange] = item.Name
	}
	out.EntryOpen = buildPriceGroupDetail(reqs, SimPortfolioV2PriceTypeEntryOpen, nameByKey)
	out.ValuationClose = buildPriceGroupDetail(reqs, SimPortfolioV2PriceTypeValuationClose, nameByKey)
	factsCount, _ := s.repo.CountSimPortfolioV2DailyByMarketSignalDate(ctx, def.Market, signalDate)
	out.Facts.DailyCount = factsCount
	if factsCount > 0 {
		out.Facts.Status = "verified"
		out.Status = "verified"
	} else {
		out.Facts.Status = SimPortfolioV2StatusPending
	}
	if out.SelectedCount > 0 && out.SelectedCount < out.RequiredCount {
		out.Status = SimPortfolioV2StatusBlocked
		out.RepairSuggestions = append(out.RepairSuggestions, SimPortfolioV2RepairSuggestion{Type: "repair_selection", Label: "检查选股规则或补齐信号快照"})
	}
	if out.EntryOpen.MissingCount > 0 || out.ValuationClose.MissingCount > 0 {
		out.Status = SimPortfolioV2StatusBlocked
		out.Facts.Status = SimPortfolioV2StatusBlocked
		out.RepairSuggestions = append(out.RepairSuggestions, SimPortfolioV2RepairSuggestion{Type: "retry_price_resolve", Label: "重新解析该日价格", Hint: "先重新调用价格 resolver，不直接修改收益 facts。"}, SimPortfolioV2RepairSuggestion{Type: "backfill_daily_bars", Label: "重拉该日缺失价格", Hint: "补齐标准历史日线后再重新运行 pipeline。"})
	}
	return out, nil
}

func buildPriceGroupDetail(reqs []SimPortfolioV2PriceRequirement, priceType string, nameByKey map[string]string) SimPortfolioV2PriceGroupDetail {
	out := SimPortfolioV2PriceGroupDetail{Status: SimPortfolioV2StatusPending}
	for _, req := range reqs {
		if req.PriceType != priceType {
			continue
		}
		out.RequiredCount++
		if req.Status == SimPortfolioV2PriceStatusSatisfied {
			out.SatisfiedCount++
		} else {
			out.MissingCount++
			out.MissingItems = append(out.MissingItems, SimPortfolioV2MissingPriceItem{Code: req.Code, Name: nameByKey[req.Code+"\x00"+req.Exchange], Exchange: req.Exchange, TradeDate: req.TradeDate, PriceType: req.PriceType, ReasonCode: "source_not_loaded", Message: req.MissingReason})
		}
	}
	if out.RequiredCount == 0 {
		out.Status = SimPortfolioV2StatusPending
	} else if out.MissingCount > 0 {
		out.Status = SimPortfolioV2PriceStatusMissing
	} else {
		out.Status = SimPortfolioV2StatusOK
	}
	return out
}

func (s *SimPortfolioV2Service) PreviewStartDate(ctx context.Context, req SimPortfolioV2StartDatePreviewRequest) (*SimPortfolioV2StartDatePreviewResponse, error) {
	market := normalizeSimPortfolioV2Market(req.Market)
	date := strings.TrimSpace(req.StartSignalDate)
	if _, ok := parseYMD(date); !ok {
		return nil, fmt.Errorf("invalid start_signal_date")
	}
	today := time.Now().In(beijingLocation()).Format("2006-01-02")
	resp := &SimPortfolioV2StartDatePreviewResponse{Market: market, StartSignalDate: date, CanApply: true}
	if date > today {
		resp.CanApply = false
		resp.BlockingReasons = append(resp.BlockingReasons, SimPortfolioV2BlockingReason{Code: "future_date", Message: "不能选择未来日期。"})
	}
	cal := s.calendar.CalendarRow(market, date)
	if !cal.IsTradingDay {
		resp.CanApply = false
		resp.BlockingReasons = append(resp.BlockingReasons, SimPortfolioV2BlockingReason{Code: "market_closed", Message: "该市场当日休市，不能作为开始信号日。"})
	}
	batch, _ := s.repo.GetSimPortfolioV2SignalBatch(ctx, market, date)
	if batch == nil || batch.Status != SimPortfolioV2StatusOK {
		resp.CanApply = false
		resp.BlockingReasons = append(resp.BlockingReasons, SimPortfolioV2BlockingReason{Code: "missing_signal", Message: "缺少可用四象限/排行榜信号。", Action: "先重建该日四象限。"})
	}
	defs, _ := s.repo.ListActiveSimPortfolioV2Definitions(ctx)
	for _, def := range defs {
		if def.Market == market {
			resp.AffectedPortfolios = append(resp.AffectedPortfolios, def.ID)
		}
	}
	days := 0
	latest := ""
	for _, d := range enumerateDates(date, today) {
		if s.calendar.IsTradingDay(market, d) {
			days++
			latest = s.calendar.NextTradingDay(market, d)
		}
	}
	resp.Estimated.SignalDays = days
	resp.Estimated.EntryDays = days
	resp.Estimated.DailyRows = days * len(resp.AffectedPortfolios)
	if len(resp.AffectedPortfolios) > 0 {
		resp.Estimated.PositionRows = resp.Estimated.DailyRows * 4
	}
	resp.Estimated.LatestGeneratableTradeDate = latest
	if resp.CanApply {
		resp.Message = fmt.Sprintf("可以从该信号日重建%s模拟组合。", simPortfolioV2MarketLabel(market))
	} else {
		resp.Message = "该日期暂不能作为开始信号日。"
	}
	return resp, nil
}

func (s *SimPortfolioV2Service) ApplyStartDate(ctx context.Context, req SimPortfolioV2StartDateApplyRequest) (*SimPortfolioV2StartDateApplyResponse, error) {
	if !req.Confirm {
		return nil, fmt.Errorf("请确认后再执行市场起点重建")
	}
	preview, err := s.PreviewStartDate(ctx, SimPortfolioV2StartDatePreviewRequest{Market: req.Market, StartSignalDate: req.StartSignalDate})
	if err != nil {
		return nil, err
	}
	if !preview.CanApply {
		return nil, fmt.Errorf(preview.Message)
	}
	market := preview.Market
	jobID := fmt.Sprintf("spv2-rebuild-%s-%d", strings.ToLower(market), time.Now().UnixNano())
	if err := s.repo.DeleteSimPortfolioV2FactsForMarketFromSignalDate(ctx, market, preview.StartSignalDate); err != nil {
		return nil, err
	}
	runResp, err := s.Run(ctx, SimPortfolioV2RunRequest{Market: market, FromDate: preview.StartSignalDate, ToDate: time.Now().In(beijingLocation()).Format("2006-01-02")})
	if err != nil {
		return nil, err
	}
	status := runResp.Status
	latest := preview.Estimated.LatestGeneratableTradeDate
	now := time.Now().UTC()
	_ = s.repo.UpsertSimPortfolioV2MarketConfig(ctx, SimPortfolioV2MarketConfig{Market: market, StartSignalDate: preview.StartSignalDate, PublishedJobID: jobID, LatestPublishedTradeDate: latest, Status: status, UpdatedBy: "admin", CreatedAt: now, UpdatedAt: now})
	return &SimPortfolioV2StartDateApplyResponse{OK: true, JobID: jobID, Status: status, Market: market, StartSignalDate: preview.StartSignalDate, Message: "已按市场起点重建模拟组合 v2 pipeline。"}, nil
}

func simPortfolioV2MarketLabel(market string) string {
	if normalizeSimPortfolioV2Market(market) == SimPortfolioV2MarketHKEX {
		return "港股"
	}
	return "A 股"
}
