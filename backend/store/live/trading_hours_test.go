package live

import (
	"testing"
	"time"
)

// ── A-Share Trading Hours ──

func TestIsAShareTradingHoursAt_Morning(t *testing.T) {
	// 09:30 CST weekday → true
	cst := fixedCST(2026, 4, 10, 9, 30, 0) // Friday
	if !IsAShareTradingHoursAt(cst) {
		t.Error("09:30 should be in A-share morning session")
	}
}

func TestIsAShareTradingHoursAt_MorningBoundaryStart(t *testing.T) {
	cst := fixedCST(2026, 4, 10, 9, 15, 0)
	if !IsAShareTradingHoursAt(cst) {
		t.Error("09:15 should be start of morning session")
	}
}

func TestIsAShareTradingHoursAt_BeforeMorning(t *testing.T) {
	cst := fixedCST(2026, 4, 10, 9, 14, 59)
	if IsAShareTradingHoursAt(cst) {
		t.Error("09:14:59 should NOT be in trading hours")
	}
}

func TestIsAShareTradingHoursAt_LunchBreak(t *testing.T) {
	cst := fixedCST(2026, 4, 10, 12, 0, 0)
	if IsAShareTradingHoursAt(cst) {
		t.Error("12:00 lunch break should NOT be trading hours")
	}
}

func TestIsAShareTradingHoursAt_Afternoon(t *testing.T) {
	cst := fixedCST(2026, 4, 10, 14, 30, 0)
	if !IsAShareTradingHoursAt(cst) {
		t.Error("14:30 should be in afternoon session")
	}
}

func TestIsAShareTradingHoursAt_AfternoonEnd(t *testing.T) {
	cst := fixedCST(2026, 4, 10, 15, 0, 0)
	if !IsAShareTradingHoursAt(cst) {
		t.Error("15:00 should be end of afternoon session (inclusive)")
	}
}

func TestIsAShareTradingHoursAt_AfterClose(t *testing.T) {
	cst := fixedCST(2026, 4, 10, 15, 1, 0) // 15:01 — next minute past close
	if IsAShareTradingHoursAt(cst) {
		t.Error("15:01 should NOT be trading hours")
	}
}

func TestIsAShareTradingHoursAt_Saturday(t *testing.T) {
	cst := fixedCST(2026, 4, 11, 10, 0, 0) // Saturday
	if IsAShareTradingHoursAt(cst) {
		t.Error("Saturday should NOT be trading day")
	}
}

func TestIsAShareTradingHoursAt_Sunday(t *testing.T) {
	cst := fixedCST(2026, 4, 12, 10, 0, 0) // Sunday
	if IsAShareTradingHoursAt(cst) {
		t.Error("Sunday should NOT be trading day")
	}
}

// ── HK Trading Hours ──

func TestIsHKTradingHoursAt_Morning(t *testing.T) {
	cst := fixedCST(2026, 4, 10, 10, 0, 0)
	if !IsHKTradingHoursAt(cst) {
		t.Error("10:00 should be in HK morning session")
	}
}

func TestIsHKTradingHoursAt_MorningStart(t *testing.T) {
	cst := fixedCST(2026, 4, 10, 9, 30, 0)
	if !IsHKTradingHoursAt(cst) {
		t.Error("09:30 should be start of HK session")
	}
}

func TestIsHKTradingHoursAt_BeforeOpen(t *testing.T) {
	cst := fixedCST(2026, 4, 10, 9, 29, 59)
	if IsHKTradingHoursAt(cst) {
		t.Error("09:29:59 should NOT be HK trading hours")
	}
}

func TestIsHKTradingHoursAt_LunchBreak(t *testing.T) {
	cst := fixedCST(2026, 4, 10, 12, 30, 0)
	if IsHKTradingHoursAt(cst) {
		t.Error("12:30 HK lunch break should NOT be trading hours")
	}
}

func TestIsHKTradingHoursAt_Afternoon(t *testing.T) {
	cst := fixedCST(2026, 4, 10, 14, 0, 0)
	if !IsHKTradingHoursAt(cst) {
		t.Error("14:00 should be in HK afternoon session")
	}
}

func TestIsHKTradingHoursAt_Close(t *testing.T) {
	cst := fixedCST(2026, 4, 10, 16, 0, 0)
	if !IsHKTradingHoursAt(cst) {
		t.Error("16:00 should be end of HK session (inclusive)")
	}
}

func TestIsHKTradingHoursAfterClose(t *testing.T) {
	cst := fixedCST(2026, 4, 10, 16, 1, 0) // 16:01 — next minute past close
	if IsHKTradingHoursAt(cst) {
		t.Error("16:01 should NOT be HK trading hours")
	}
}

func TestIsHKTradingHoursAt_Weekend(t *testing.T) {
	cst := fixedCST(2026, 4, 11, 10, 0, 0) // Saturday
	if IsHKTradingHoursAt(cst) {
		t.Error("Saturday should NOT be HK trading day")
	}
}

// ── TradeDate ──

func TestTradeDateAt_Format(t *testing.T) {
	cst := fixedCST(2026, 4, 10, 18, 30, 0)
	date := TradeDateAt(cst)
	if date != "2026-04-10" {
		t.Errorf("expected 2026-04-10, got %s", date)
	}
}

// ── IsTradingHoursAt dispatch ──

func TestIsTradingHoursAt_AShare(t *testing.T) {
	cst := fixedCST(2026, 4, 10, 10, 0, 0)
	if !IsTradingHoursAt("600519.SH", cst) {
		t.Error("A-share at 10:00 should be trading hours")
	}
}

func TestIsTradingHoursAt_HK(t *testing.T) {
	cst := fixedCST(2026, 4, 10, 10, 0, 0)
	if !IsTradingHoursAt("00700.HK", cst) {
		t.Error("HK at 10:00 should be trading hours")
	}
}

func TestIsTradingHoursAt_NonTrading(t *testing.T) {
	cst := fixedCST(2026, 4, 10, 12, 30, 0)
	if IsTradingHoursAt("600519.SH", cst) {
		t.Error("A-share at 12:30 (lunch) should NOT be trading hours")
	}
}

// ── Helper: create a fixed time in CST (UTC+8) ──

func fixedCST(year, month, day, hour, min, sec int) time.Time {
	// Construct as UTC then convert via In(cstLocation())
	utc := time.Date(year, time.Month(month), day, hour-8, min, sec, 0, time.UTC)
	return utc.In(cstLocation())
}
