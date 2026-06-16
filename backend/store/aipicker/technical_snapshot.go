package aipicker

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/woodyyan/pumpkin-pro/backend/store/live"
)

type TechnicalSnapshot struct {
	ID                uint      `gorm:"primaryKey" json:"id"`
	TradeDate         string    `gorm:"size:10;not null;index:idx_aipicker_technical_trade_code,unique" json:"trade_date"`
	Code              string    `gorm:"size:16;not null;index:idx_aipicker_technical_trade_code,unique" json:"code"`
	Symbol            string    `gorm:"size:20;not null;default:''" json:"symbol"`
	Name              string    `gorm:"size:128;not null;default:''" json:"name"`
	Industry          string    `gorm:"size:128;not null;default:'';index" json:"industry"`
	ClosePrice        float64   `gorm:"not null;default:0" json:"close_price"`
	MA5               float64   `gorm:"not null;default:0" json:"ma5"`
	MA20              float64   `gorm:"not null;default:0" json:"ma20"`
	MA60              float64   `gorm:"not null;default:0" json:"ma60"`
	MA200             float64   `gorm:"not null;default:0" json:"ma200"`
	DistanceToMA20Pct float64   `gorm:"not null;default:0" json:"distance_to_ma20_pct"`
	DistanceToMA60Pct float64   `gorm:"not null;default:0" json:"distance_to_ma60_pct"`
	RSI14             float64   `gorm:"not null;default:0" json:"rsi14"`
	RSI14Status       string    `gorm:"size:32;not null;default:''" json:"rsi14_status"`
	ChangePct20D      float64   `gorm:"not null;default:0" json:"change_pct_20d"`
	ChangePct60D      float64   `gorm:"not null;default:0" json:"change_pct_60d"`
	Volatility20D     float64   `gorm:"not null;default:0" json:"volatility_20d"`
	VolumeMA5ToMA20   float64   `gorm:"not null;default:0" json:"volume_ma5_to_ma20"`
	TrendStatus       string    `gorm:"size:32;not null;default:''" json:"trend_status"`
	TechTagsJSON      string    `gorm:"type:text;not null;default:'[]'" json:"tech_tags_json"`
	CreatedAt         time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt         time.Time `gorm:"not null" json:"updated_at"`
}

func (TechnicalSnapshot) TableName() string { return "aipicker_technical_snapshots" }

type TechnicalSnapshotRepository struct{ db *gorm.DB }

func NewTechnicalSnapshotRepository(db *gorm.DB) *TechnicalSnapshotRepository {
	return &TechnicalSnapshotRepository{db: db}
}

func (r *TechnicalSnapshotRepository) ExistsByTradeDate(ctx context.Context, tradeDate string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&TechnicalSnapshot{}).Where("trade_date = ?", strings.TrimSpace(tradeDate)).Count(&count).Error
	return count > 0, err
}

func (r *TechnicalSnapshotRepository) SaveBatch(ctx context.Context, items []TechnicalSnapshot) error {
	if len(items) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "trade_date"}, {Name: "code"}},
		DoUpdates: clause.AssignmentColumns([]string{"symbol", "name", "industry", "close_price", "ma5", "ma20", "ma60", "ma200", "distance_to_ma20_pct", "distance_to_ma60_pct", "rsi14", "rsi14_status", "change_pct_20d", "change_pct_60d", "volatility_20d", "volume_ma5_to_ma20", "trend_status", "tech_tags_json", "updated_at"}),
	}).Create(&items).Error
}

func (r *TechnicalSnapshotRepository) GetByTradeDateAndCodes(ctx context.Context, tradeDate string, codes []string) ([]TechnicalSnapshot, error) {
	if len(codes) == 0 {
		return nil, nil
	}
	var items []TechnicalSnapshot
	err := r.db.WithContext(ctx).Where("trade_date = ? AND code IN ?", strings.TrimSpace(tradeDate), codes).Find(&items).Error
	return items, err
}

type TechnicalSnapshotService struct {
	repo         *TechnicalSnapshotRepository
	marketClient *live.MarketClient
}

func NewTechnicalSnapshotService(repo *TechnicalSnapshotRepository) *TechnicalSnapshotService {
	return &TechnicalSnapshotService{repo: repo, marketClient: live.NewMarketClient()}
}

func (s *TechnicalSnapshotService) EnsureForCandidates(ctx context.Context, tradeDate string, candidates []FactorCandidate) error {
	if s == nil || s.repo == nil {
		return nil
	}
	exists, err := s.repo.ExistsByTradeDate(ctx, tradeDate)
	if err != nil || exists {
		return err
	}
	items := make([]TechnicalSnapshot, 0, len(candidates))
	for _, candidate := range candidates {
		item, ok := s.buildSnapshot(ctx, tradeDate, candidate)
		if ok {
			items = append(items, item)
		}
	}
	return s.repo.SaveBatch(ctx, items)
}

func (s *TechnicalSnapshotService) buildSnapshot(ctx context.Context, tradeDate string, candidate FactorCandidate) (TechnicalSnapshot, bool) {
	bars, err := s.marketClient.FetchSymbolDailyBars(ctx, candidate.Symbol, 240)
	if err != nil || len(bars) < 200 {
		return TechnicalSnapshot{}, false
	}
	last := bars[len(bars)-1]
	ma5 := avgClose(bars, 5)
	ma20 := avgClose(bars, 20)
	ma60 := avgClose(bars, 60)
	ma200 := avgClose(bars, 200)
	change20 := changePctBars(bars, 20)
	change60 := changePctBars(bars, 60)
	vol20 := volatilityBars(bars, 20)
	volRatio := volumeRatioBars(bars, 5, 20)
	rsi14 := rsiBars(bars, 14)
	item := TechnicalSnapshot{
		TradeDate:         tradeDate,
		Code:              candidate.Code,
		Symbol:            candidate.Symbol,
		Name:              candidate.Name,
		Industry:          candidate.Industry,
		ClosePrice:        last.Close,
		MA5:               ma5,
		MA20:              ma20,
		MA60:              ma60,
		MA200:             ma200,
		DistanceToMA20Pct: pctDistance(last.Close, ma20),
		DistanceToMA60Pct: pctDistance(last.Close, ma60),
		RSI14:             round2Tech(rsi14),
		RSI14Status:       classifyRSIStatusLocal(rsi14),
		ChangePct20D:      change20,
		ChangePct60D:      change60,
		Volatility20D:     vol20,
		VolumeMA5ToMA20:   volRatio,
		TrendStatus:       classifyTrendStatus(change20),
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}
	buf, _ := json.Marshal(buildTechnicalTags(item))
	item.TechTagsJSON = string(buf)
	return item, true
}

func avgClose(bars []live.DailyBar, n int) float64 {
	if len(bars) < n || n <= 0 {
		return 0
	}
	sum := 0.0
	for _, bar := range bars[len(bars)-n:] {
		sum += bar.Close
	}
	return round2Tech(sum / float64(n))
}

func changePctBars(bars []live.DailyBar, days int) float64 {
	if len(bars) <= days || days <= 0 {
		return 0
	}
	prev := bars[len(bars)-days-1].Close
	last := bars[len(bars)-1].Close
	if prev <= 0 {
		return 0
	}
	return round2Tech((last - prev) / prev * 100)
}

func volatilityBars(bars []live.DailyBar, n int) float64 {
	if len(bars) <= n || n <= 1 {
		return 0
	}
	returns := make([]float64, 0, n)
	for i := len(bars) - n; i < len(bars); i++ {
		if i == 0 || bars[i-1].Close <= 0 {
			continue
		}
		returns = append(returns, (bars[i].Close-bars[i-1].Close)/bars[i-1].Close)
	}
	if len(returns) == 0 {
		return 0
	}
	mean := 0.0
	for _, r := range returns {
		mean += r
	}
	mean /= float64(len(returns))
	variance := 0.0
	for _, r := range returns {
		d := r - mean
		variance += d * d
	}
	variance /= float64(len(returns))
	return round2Tech(math.Sqrt(variance) * math.Sqrt(252) * 100)
}

func volumeRatioBars(bars []live.DailyBar, shortN, longN int) float64 {
	if len(bars) < longN || shortN <= 0 || longN <= 0 || shortN > longN {
		return 0
	}
	shortSum := 0.0
	for _, bar := range bars[len(bars)-shortN:] {
		shortSum += bar.Volume
	}
	longSum := 0.0
	for _, bar := range bars[len(bars)-longN:] {
		longSum += bar.Volume
	}
	shortAvg := shortSum / float64(shortN)
	longAvg := longSum / float64(longN)
	if longAvg <= 0 {
		return 0
	}
	return round2Tech(shortAvg / longAvg)
}

func rsiBars(bars []live.DailyBar, period int) float64 {
	if len(bars) <= period {
		return 0
	}
	gain := 0.0
	loss := 0.0
	for i := len(bars) - period; i < len(bars); i++ {
		change := bars[i].Close - bars[i-1].Close
		if change > 0 {
			gain += change
		} else {
			loss -= change
		}
	}
	avgGain := gain / float64(period)
	avgLoss := loss / float64(period)
	if avgLoss == 0 {
		return 100
	}
	rs := avgGain / avgLoss
	return 100 - (100 / (1 + rs))
}

func pctDistance(price, ma float64) float64 {
	if ma <= 0 {
		return 0
	}
	return round2Tech((price - ma) / ma * 100)
}

func classifyRSIStatusLocal(v float64) string {
	switch {
	case v >= 70:
		return "超买"
	case v >= 55:
		return "偏强"
	case v >= 45:
		return "中性"
	default:
		return "偏弱"
	}
}

func classifyTrendStatus(change20 float64) string {
	switch {
	case change20 >= 5:
		return "近一月上涨"
	case change20 <= -5:
		return "近一月走弱"
	default:
		return "近一月震荡"
	}
}

func buildTechnicalTags(item TechnicalSnapshot) []string {
	tags := []string{item.TrendStatus}
	if item.DistanceToMA20Pct >= 0 {
		tags = append(tags, "站上MA20")
	} else {
		tags = append(tags, "跌破MA20")
	}
	if item.DistanceToMA20Pct >= 0 && item.DistanceToMA60Pct >= 0 {
		tags = append(tags, "中期趋势偏强")
	} else if item.DistanceToMA20Pct < 0 && item.DistanceToMA60Pct < 0 {
		tags = append(tags, "中期趋势偏弱")
	}
	if item.RSI14 >= 70 {
		tags = append(tags, "接近超买")
	} else if item.RSI14 >= 55 {
		tags = append(tags, "动能偏强")
	}
	if item.VolumeMA5ToMA20 >= 1.2 {
		tags = append(tags, "量能温和放大")
	} else if item.VolumeMA5ToMA20 < 0.8 {
		tags = append(tags, "量能不足")
	}
	if len(tags) > 4 {
		tags = tags[:4]
	}
	return tags
}

func decodeTechTags(raw string) []string {
	var tags []string
	_ = json.Unmarshal([]byte(strings.TrimSpace(raw)), &tags)
	return tags
}

func round2Tech(v float64) float64 { return math.Round(v*100) / 100 }
