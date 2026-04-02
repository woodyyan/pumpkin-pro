package live

import "time"

// cstLocation returns the Asia/Shanghai timezone, falling back to a fixed UTC+8.
func cstLocation() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("CST", 8*3600)
	}
	return loc
}

// IsAShareTradingHours returns true if the current time falls within A-share
// trading sessions: weekdays 09:15–11:30 and 13:00–15:00 CST.
func IsAShareTradingHours() bool {
	return IsAShareTradingHoursAt(time.Now())
}

// IsAShareTradingHoursAt checks whether a given time falls within A-share trading hours.
func IsAShareTradingHoursAt(t time.Time) bool {
	cst := t.In(cstLocation())
	weekday := cst.Weekday()
	if weekday == time.Saturday || weekday == time.Sunday {
		return false
	}
	h, m, _ := cst.Clock()
	mins := h*60 + m
	// Morning session: 09:15 – 11:30
	if mins >= 9*60+15 && mins <= 11*60+30 {
		return true
	}
	// Afternoon session: 13:00 – 15:00
	if mins >= 13*60 && mins <= 15*60 {
		return true
	}
	return false
}

// TodayTradeDate returns today's date string in "2006-01-02" format (CST).
func TodayTradeDate() string {
	return time.Now().In(cstLocation()).Format("2006-01-02")
}
