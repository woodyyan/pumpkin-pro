package signal

import (
	"errors"
	"testing"
	"time"

	"github.com/woodyyan/pumpkin-pro/backend/store/live"
)

func TestListTemplates_ActiveOnlyAndOrdered(t *testing.T) {
	templates := ListTemplates()
	if len(templates) == 0 {
		t.Fatal("expected at least one active template")
	}
	for i := 1; i < len(templates); i++ {
		if templates[i].SortOrder < templates[i-1].SortOrder {
			t.Errorf("templates not ordered by sort_order at %d", i)
		}
		if !templates[i].IsActive {
			t.Errorf("inactive template %s should not be listed", templates[i].Key)
		}
	}

	// 首期必须包含的模板键。
	required := []string{"price_above", "price_below", "pct_change", "macd_cross", "rsi_range", "ma_breakout", "bollinger_reversion", "volume_breakout", "strategy"}
	for _, key := range required {
		if _, ok := GetTemplate(key); !ok {
			t.Errorf("required template %s missing", key)
		}
	}
}

func TestGetTemplate_UnknownKey(t *testing.T) {
	if _, ok := GetTemplate("not_exist"); ok {
		t.Error("unknown template key should return false")
	}
}

func TestValidateTemplateParams_MergesDefaults(t *testing.T) {
	tpl, _ := GetTemplate("macd_cross")
	params, err := ValidateTemplateParams(tpl, map[string]any{"fast_period": 10.0})
	if err != nil {
		t.Fatalf("ValidateTemplateParams failed: %v", err)
	}
	if params["fast_period"] != 10.0 {
		t.Errorf("expected fast_period override 10, got %v", params["fast_period"])
	}
	if params["slow_period"] != 26.0 || params["signal_period"] != 9.0 {
		t.Errorf("expected defaults merged, got %v", params)
	}
}

func TestValidateTemplateParams_RequiredMissing(t *testing.T) {
	tpl, _ := GetTemplate("price_above")
	if _, err := ValidateTemplateParams(tpl, map[string]any{}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for missing required price, got %v", err)
	}
}

func TestValidateTemplateParams_RangeEnforced(t *testing.T) {
	tpl, _ := GetTemplate("pct_change")
	if _, err := ValidateTemplateParams(tpl, map[string]any{"pct": 0.5}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected range violation error, got %v", err)
	}
	if _, err := ValidateTemplateParams(tpl, map[string]any{"pct": 25.0}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected range violation error for pct>20, got %v", err)
	}
	if _, err := ValidateTemplateParams(tpl, map[string]any{"pct": 5.0}); err != nil {
		t.Errorf("expected valid pct, got %v", err)
	}
}

func TestValidateTemplateParams_RejectsUnknownParam(t *testing.T) {
	tpl, _ := GetTemplate("macd_cross")
	if _, err := ValidateTemplateParams(tpl, map[string]any{"unknown_field": 1.0}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected unknown param rejection, got %v", err)
	}
}

func TestValidateTemplateParams_RejectsNonNumber(t *testing.T) {
	tpl, _ := GetTemplate("price_above")
	if _, err := ValidateTemplateParams(tpl, map[string]any{"price": "abc"}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected type error, got %v", err)
	}
}

func TestNormalizeScopeType(t *testing.T) {
	scope, err := normalizeScopeType("")
	if err != nil || scope != ScopeTypeSymbol {
		t.Errorf("empty scope should default to symbol, got %s err=%v", scope, err)
	}
	if _, err := normalizeScopeType("watchlist"); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("watchlist scope should be rejected in P0, got %v", err)
	}
	if _, err := normalizeScopeType("bogus"); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("bogus scope should be rejected, got %v", err)
	}
}

// ── 价格提醒纯函数 ──

func makeSnapshot(lastPrice, prevClose, changeRate float64) live.DetailedSymbolSnapshot {
	return live.DetailedSymbolSnapshot{
		Symbol:         "600519.SH",
		LastPrice:      lastPrice,
		PrevClosePrice: prevClose,
		ChangeRate:     changeRate,
	}
}

func TestEvaluatePriceAlert_PriceAbove(t *testing.T) {
	params := map[string]any{"price": 100.0}
	out := evaluatePriceAlert("price_above", params, makeSnapshot(101.5, 99, 2.5))
	if !out.triggered || out.side != SideBuy {
		t.Errorf("expected BUY trigger when price above target, got %+v", out)
	}
	out = evaluatePriceAlert("price_above", params, makeSnapshot(99.5, 99, 0.5))
	if out.triggered {
		t.Error("should not trigger below target")
	}
}

func TestEvaluatePriceAlert_PriceBelow(t *testing.T) {
	params := map[string]any{"price": 100.0}
	out := evaluatePriceAlert("price_below", params, makeSnapshot(98.5, 101, -2.5))
	if !out.triggered || out.side != SideSell {
		t.Errorf("expected SELL trigger when price below target, got %+v", out)
	}
	out = evaluatePriceAlert("price_below", params, makeSnapshot(100.5, 101, -0.5))
	if out.triggered {
		t.Error("should not trigger above target")
	}
}

func TestEvaluatePriceAlert_PctChange(t *testing.T) {
	params := map[string]any{"pct": 5.0}
	up := evaluatePriceAlert("pct_change", params, makeSnapshot(105, 100, 5.2))
	if !up.triggered || up.side != SideBuy {
		t.Errorf("expected BUY trigger on surge, got %+v", up)
	}
	down := evaluatePriceAlert("pct_change", params, makeSnapshot(95, 100, -6.0))
	if !down.triggered || down.side != SideSell {
		t.Errorf("expected SELL trigger on drop, got %+v", down)
	}
	flat := evaluatePriceAlert("pct_change", params, makeSnapshot(102, 100, 2.0))
	if flat.triggered {
		t.Error("should not trigger within threshold")
	}
}

func TestEvaluatePriceAlert_InvalidParamsNeverTrigger(t *testing.T) {
	if out := evaluatePriceAlert("price_above", map[string]any{}, makeSnapshot(101, 99, 2)); out.triggered {
		t.Error("missing price param should never trigger")
	}
	if out := evaluatePriceAlert("unknown_template", map[string]any{"price": 1.0}, makeSnapshot(101, 99, 2)); out.triggered {
		t.Error("unknown template should never trigger")
	}
}

// ── 收盘确认调度纯函数 ──

func newTestCloseConfirmer() *CloseConfirmer {
	return NewCloseConfirmer(nil, CloseConfirmerConfig{
		Enabled:      true,
		AShareHour:   15,
		AShareMinute: 10,
		HKHour:       16,
		HKMinute:     10,
	})
}

func cstTime(year int, month time.Month, day, hour, minute int) time.Time {
	return time.Date(year, month, day, hour, minute, 0, 0, cstLocation())
}

func TestDueMarkets_BeforePoints(t *testing.T) {
	c := newTestCloseConfirmer()
	// 2026-08-03 是周一，15:05 早于 A 股 15:10。
	due := c.dueMarkets(cstTime(2026, 8, 3, 15, 5), map[string]string{})
	if len(due) != 0 {
		t.Errorf("expected no due markets before points, got %v", due)
	}
}

func TestDueMarkets_AfterASharePoint(t *testing.T) {
	c := newTestCloseConfirmer()
	due := c.dueMarkets(cstTime(2026, 8, 3, 15, 10), map[string]string{})
	if len(due) != 1 || due[0] != MarketAShare {
		t.Errorf("expected only ashare due at 15:10, got %v", due)
	}
}

func TestDueMarkets_AfterHKPoint(t *testing.T) {
	c := newTestCloseConfirmer()
	due := c.dueMarkets(cstTime(2026, 8, 3, 16, 11), map[string]string{})
	if len(due) != 2 {
		t.Errorf("expected both markets due after 16:10, got %v", due)
	}
}

func TestDueMarkets_SameDayDedup(t *testing.T) {
	c := newTestCloseConfirmer()
	lastRun := map[string]string{MarketAShare: "2026-08-03"}
	due := c.dueMarkets(cstTime(2026, 8, 3, 16, 11), lastRun)
	if len(due) != 1 || due[0] != MarketHK {
		t.Errorf("expected only hk due when ashare already ran, got %v", due)
	}
}

func TestDueMarkets_WeekendSkipped(t *testing.T) {
	c := newTestCloseConfirmer()
	// 2026-08-01 是周六。
	due := c.dueMarkets(cstTime(2026, 8, 1, 16, 30), map[string]string{})
	if len(due) != 0 {
		t.Errorf("expected no due markets on weekend, got %v", due)
	}
}

func TestCloseConfirmerConfig_Defaults(t *testing.T) {
	c := NewCloseConfirmer(nil, CloseConfirmerConfig{Enabled: true})
	if c.cfg.AShareHour != 15 || c.cfg.AShareMinute != 10 {
		t.Errorf("expected ashare default 15:10, got %02d:%02d", c.cfg.AShareHour, c.cfg.AShareMinute)
	}
	if c.cfg.HKHour != 16 || c.cfg.HKMinute != 10 {
		t.Errorf("expected hk default 16:10, got %02d:%02d", c.cfg.HKHour, c.cfg.HKMinute)
	}
}

// ── 指纹与 bar 状态 ──

func TestNormalizeBarState(t *testing.T) {
	if got := normalizeBarState("", true); got != BarStateTest {
		t.Errorf("test event should force bar_state=test, got %q", got)
	}
	if got := normalizeBarState(BarStateIntradayProvisional, false); got != BarStateIntradayProvisional {
		t.Errorf("valid bar state should pass through, got %q", got)
	}
	if got := normalizeBarState("bogus", false); got != "" {
		t.Errorf("invalid bar state should normalize to empty (legacy semantics), got %q", got)
	}
}

func TestFingerprint_DualStateCoexist(t *testing.T) {
	now := time.Date(2026, 8, 3, 15, 30, 0, 0, time.UTC)
	provisional := buildFingerprint("u1", "600519.SH", "", "macd_cross", "BUY", BarStateIntradayProvisional, now, false, "e1")
	confirmed := buildFingerprint("u1", "600519.SH", "", "macd_cross", "BUY", BarStateCloseConfirmed, now, false, "e2")
	if provisional == confirmed {
		t.Error("provisional and confirmed events of the same day must coexist (different fingerprints)")
	}

	same := buildFingerprint("u1", "600519.SH", "", "macd_cross", "BUY", BarStateIntradayProvisional, now, false, "e3")
	if provisional != same {
		t.Error("same day+state+side should dedup to the same fingerprint")
	}

	otherDay := buildFingerprint("u1", "600519.SH", "", "macd_cross", "BUY", BarStateIntradayProvisional, now.Add(48*time.Hour), false, "e4")
	if provisional == otherDay {
		t.Error("different trade dates must not collide")
	}
}
