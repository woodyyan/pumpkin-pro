package signal

import (
	"context"
	"log"
	"sync"
	"time"
)

// ──────────────────────────────────────────────────────────────
// 收盘确认器（CloseConfirmer）
//
// 职责：在收盘后最后一根日 K 定型的时点，对指标/策略类订阅做一次
// 「收盘确认」评估（bar_state=close_confirmed）。
//
// 调度：A 股默认 15:10、港股默认 16:10（北京时间，配置化）。
// 市场日历门控隐含在评估层：休市时最后一根 bar 不是当日，
// requireCurrentTradeDate 会自动跳过，无需额外依赖日历服务。
//
// 与盘中评估器共享同一条评估链路（evaluator.runPass），仅调度与
// bar_state 不同；cooldown 与指纹均按 bar_state 隔离，互不阻塞。
// ──────────────────────────────────────────────────────────────

const (
	closeConfirmerTickInterval = 30 * time.Second
	MarketAShare               = "ashare"
	MarketHK                   = "hk"
)

// CloseConfirmerConfig 收盘确认调度配置。
type CloseConfirmerConfig struct {
	Enabled      bool
	AShareHour   int
	AShareMinute int
	HKHour       int
	HKMinute     int
}

// CloseConfirmer 收盘确认 worker。
type CloseConfirmer struct {
	evaluator *Evaluator
	cfg       CloseConfirmerConfig

	mu           sync.Mutex
	lastRunByDay map[string]string // market -> 已执行的 CST 日期
}

func NewCloseConfirmer(evaluator *Evaluator, cfg CloseConfirmerConfig) *CloseConfirmer {
	if cfg.AShareHour <= 0 && cfg.AShareMinute <= 0 {
		cfg.AShareHour, cfg.AShareMinute = 15, 10
	}
	if cfg.HKHour <= 0 && cfg.HKMinute <= 0 {
		cfg.HKHour, cfg.HKMinute = 16, 10
	}
	return &CloseConfirmer{
		evaluator:    evaluator,
		cfg:          cfg,
		lastRunByDay: map[string]string{},
	}
}

// Start 启动调度循环；启动时做一次当日 catch-up（进程重启在时点之后不会错过确认）。
func (c *CloseConfirmer) Start(ctx context.Context) {
	if c == nil || !c.cfg.Enabled {
		return
	}
	go func() {
		c.runDue(time.Now())
		ticker := time.NewTicker(closeConfirmerTickInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				c.runDue(now)
			}
		}
	}()
	log.Printf("[signal-close-confirmer] started, ashare=%02d:%02d hk=%02d:%02d (CST)",
		c.cfg.AShareHour, c.cfg.AShareMinute, c.cfg.HKHour, c.cfg.HKMinute)
}

// runDue 检查两个市场是否到达确认时点且当日未执行，到达则执行。
func (c *CloseConfirmer) runDue(now time.Time) {
	for _, market := range c.dueMarkets(now, c.snapshotLastRun()) {
		c.markRun(market, cstDateString(now))
		log.Printf("[signal-close-confirmer] run market=%s", market)
		c.evaluator.RunCloseConfirm(context.Background(), market)
	}
}

// dueMarkets 纯函数：返回当前时刻到达确认时点且当日未执行的市场列表。
// lastRun 为 market -> CST 日期串（"2006-01-02"）。
// 周末两市必休市，直接不触发（节假日由评估层的当日 bar 校验兜底）。
func (c *CloseConfirmer) dueMarkets(now time.Time, lastRun map[string]string) []string {
	weekday := now.In(cstLocation()).Weekday()
	if weekday == time.Saturday || weekday == time.Sunday {
		return nil
	}
	today := cstDateString(now)
	due := make([]string, 0, 2)
	for _, point := range c.marketPoints() {
		if lastRun[point.market] == today {
			continue
		}
		if cstMinutesOfDay(now) >= point.hour*60+point.minute {
			due = append(due, point.market)
		}
	}
	return due
}

type marketPoint struct {
	market string
	hour   int
	minute int
}

func (c *CloseConfirmer) marketPoints() []marketPoint {
	return []marketPoint{
		{market: MarketAShare, hour: c.cfg.AShareHour, minute: c.cfg.AShareMinute},
		{market: MarketHK, hour: c.cfg.HKHour, minute: c.cfg.HKMinute},
	}
}

func (c *CloseConfirmer) snapshotLastRun() map[string]string {
	c.mu.Lock()
	defer c.mu.Unlock()
	copied := make(map[string]string, len(c.lastRunByDay))
	for key, value := range c.lastRunByDay {
		copied[key] = value
	}
	return copied
}

func (c *CloseConfirmer) markRun(market, day string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastRunByDay[market] = day
}

func cstDateString(t time.Time) string {
	return t.In(cstLocation()).Format("2006-01-02")
}

func cstMinutesOfDay(t time.Time) int {
	cst := t.In(cstLocation())
	h, m, _ := cst.Clock()
	return h*60 + m
}
