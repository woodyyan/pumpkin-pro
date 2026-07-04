package quadrant

import (
	"strings"
	"time"
)

type SimPortfolioV2CalendarService struct{}

func NewSimPortfolioV2CalendarService() *SimPortfolioV2CalendarService {
	return &SimPortfolioV2CalendarService{}
}

func normalizeSimPortfolioV2Market(market string) string {
	switch strings.ToUpper(strings.TrimSpace(market)) {
	case "HK", "HKEX":
		return SimPortfolioV2MarketHKEX
	default:
		return SimPortfolioV2MarketAShare
	}
}

func parseYMD(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	parsed, err := time.ParseInLocation("2006-01-02", value, beijingLocation())
	return parsed, err == nil
}

func formatYMD(t time.Time) string { return t.In(beijingLocation()).Format("2006-01-02") }

func (c *SimPortfolioV2CalendarService) IsTradingDay(market string, date string) bool {
	day, ok := parseYMD(date)
	if !ok {
		return false
	}
	weekday := day.Weekday()
	if weekday == time.Saturday || weekday == time.Sunday {
		return false
	}
	market = normalizeSimPortfolioV2Market(market)
	date = formatYMD(day)
	if market == SimPortfolioV2MarketHKEX {
		if _, ok := hkexBuiltinHolidays[date]; ok {
			return false
		}
	} else {
		if _, ok := aShareBuiltinHolidays[date]; ok {
			return false
		}
	}
	return true
}

func (c *SimPortfolioV2CalendarService) HolidayName(market string, date string) string {
	market = normalizeSimPortfolioV2Market(market)
	if market == SimPortfolioV2MarketHKEX {
		return hkexBuiltinHolidays[strings.TrimSpace(date)]
	}
	return aShareBuiltinHolidays[strings.TrimSpace(date)]
}

func (c *SimPortfolioV2CalendarService) NextTradingDay(market string, date string) string {
	day, ok := parseYMD(date)
	if !ok {
		return ""
	}
	for i := 0; i < 370; i++ {
		day = day.AddDate(0, 0, 1)
		candidate := formatYMD(day)
		if c.IsTradingDay(market, candidate) {
			return candidate
		}
	}
	return ""
}

func (c *SimPortfolioV2CalendarService) PreviousTradingDay(market string, date string) string {
	day, ok := parseYMD(date)
	if !ok {
		return ""
	}
	for i := 0; i < 370; i++ {
		day = day.AddDate(0, 0, -1)
		candidate := formatYMD(day)
		if c.IsTradingDay(market, candidate) {
			return candidate
		}
	}
	return ""
}

func (c *SimPortfolioV2CalendarService) CalendarRow(market string, date string) MarketCalendar {
	market = normalizeSimPortfolioV2Market(market)
	isTrading := c.IsTradingDay(market, date)
	now := time.Now().UTC()
	row := MarketCalendar{Market: market, TradeDate: strings.TrimSpace(date), IsTradingDay: isTrading, HolidayName: c.HolidayName(market, date), Source: "builtin", CreatedAt: now, UpdatedAt: now}
	row.PrevTradeDate = c.PreviousTradingDay(market, date)
	row.NextTradeDate = c.NextTradingDay(market, date)
	return row
}

var hkexBuiltinHolidays = map[string]string{
	"2026-01-01": "The first day of January",
	"2026-02-17": "Lunar New Year",
	"2026-02-18": "Lunar New Year",
	"2026-02-19": "Lunar New Year",
	"2026-04-03": "Good Friday",
	"2026-04-06": "Easter Monday",
	"2026-04-07": "Ching Ming Festival observed",
	"2026-05-01": "Labour Day",
	"2026-05-25": "The Birthday of the Buddha",
	"2026-06-19": "Tuen Ng Festival",
	"2026-07-01": "Hong Kong SAR Establishment Day",
	"2026-09-26": "The day following Mid-Autumn Festival",
	"2026-10-01": "National Day",
	"2026-10-19": "Chung Yeung Festival",
	"2026-12-25": "Christmas Day",
}

var aShareBuiltinHolidays = map[string]string{
	"2026-01-01": "元旦",
	"2026-02-16": "春节",
	"2026-02-17": "春节",
	"2026-02-18": "春节",
	"2026-02-19": "春节",
	"2026-02-20": "春节",
	"2026-04-06": "清明节",
	"2026-05-01": "劳动节",
	"2026-05-04": "劳动节",
	"2026-05-05": "劳动节",
	"2026-06-19": "端午节",
	"2026-09-25": "中秋节",
	"2026-10-01": "国庆节",
	"2026-10-02": "国庆节",
	"2026-10-05": "国庆节",
	"2026-10-06": "国庆节",
	"2026-10-07": "国庆节",
}
