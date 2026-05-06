package companyprofile

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const sqliteSafeBatchSize = 500

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) GetBySymbol(ctx context.Context, symbol string) (*CompanyProfileRecord, error) {
	var record CompanyProfileRecord
	err := r.db.WithContext(ctx).First(&record, "symbol = ?", strings.ToUpper(strings.TrimSpace(symbol))).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}

func (r *Repository) Upsert(ctx context.Context, record CompanyProfileRecord) error {
	if strings.TrimSpace(record.Symbol) == "" {
		return nil
	}
	return r.BulkUpsert(ctx, []CompanyProfileRecord{record})
}

func (r *Repository) BulkUpsert(ctx context.Context, records []CompanyProfileRecord) error {
	if len(records) == 0 {
		return nil
	}
	now := time.Now().UTC()
	cleaned := make([]CompanyProfileRecord, 0, len(records))
	for _, record := range records {
		record.Symbol = strings.ToUpper(strings.TrimSpace(record.Symbol))
		if record.Symbol == "" {
			continue
		}
		record.Exchange = strings.ToUpper(strings.TrimSpace(record.Exchange))
		record.Code = strings.TrimSpace(record.Code)
		record.ListingStatus = normalizeListingStatus(record.ListingStatus)
		record.ProfileStatus = normalizeProfileStatus(record.ProfileStatus)
		if record.FoundedDatePrecision == "" {
			record.FoundedDatePrecision = "unknown"
		}
		if strings.TrimSpace(record.QualityFlags) == "" {
			record.QualityFlags = "[]"
		}
		if record.CreatedAt.IsZero() {
			record.CreatedAt = now
		}
		if record.UpdatedAt.IsZero() {
			record.UpdatedAt = now
		}
		cleaned = append(cleaned, record)
	}
	if len(cleaned) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for start := 0; start < len(cleaned); start += sqliteSafeBatchSize {
			end := start + sqliteSafeBatchSize
			if end > len(cleaned) {
				end = len(cleaned)
			}
			batch := cleaned[start:end]
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "symbol"}},
				DoUpdates: clause.AssignmentColumns([]string{
					"exchange", "code", "name", "full_name", "board_code", "board_name",
					"raw_industry_name", "industry_code", "industry_name", "industry_level", "industry_source",
					"website", "founded_date", "founded_date_precision", "ipo_date", "listing_status", "delisted_date",
					"business_scope", "business_summary", "business_summary_source", "source", "source_url", "source_updated_at",
					"profile_status", "quality_flags", "updated_at",
				}),
			}).Create(&batch).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *Repository) ListLatestUniverse(ctx context.Context, exchange string, limit int) ([]UniverseSecurity, error) {
	query := r.db.WithContext(ctx).
		Table("quadrant_scores AS qs").
		Select("qs.code, qs.name, qs.exchange, qs.board as board_code").
		Where("qs.computed_at = (SELECT MAX(q2.computed_at) FROM quadrant_scores q2 WHERE q2.exchange = qs.exchange)")
	exchange = strings.ToUpper(strings.TrimSpace(exchange))
	if exchange == "ASHARE" {
		query = query.Where("qs.exchange IN ?", []string{"SSE", "SZSE"})
	} else if exchange != "" && exchange != "ALL" {
		query = query.Where("qs.exchange = ?", exchange)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}
	var rows []struct{ Code, Name, Exchange, BoardCode string }
	if err := query.Order("qs.exchange ASC, qs.code ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]UniverseSecurity, 0, len(rows))
	for _, row := range rows {
		symbol := buildSymbol(row.Code, row.Exchange)
		if symbol == "" {
			continue
		}
		items = append(items, UniverseSecurity{Symbol: symbol, Code: row.Code, Name: row.Name, Exchange: row.Exchange, BoardCode: row.BoardCode})
	}
	return items, nil
}

func (r *Repository) ListProfiles(ctx context.Context) ([]CompanyProfileRecord, error) {
	var records []CompanyProfileRecord
	return records, r.db.WithContext(ctx).Find(&records).Error
}

func (r *Repository) MarkSymbolsDelisted(ctx context.Context, symbols []string) error {
	if len(symbols) == 0 {
		return nil
	}
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for start := 0; start < len(symbols); start += sqliteSafeBatchSize {
			end := start + sqliteSafeBatchSize
			if end > len(symbols) {
				end = len(symbols)
			}
			if err := tx.Model(&CompanyProfileRecord{}).
				Where("symbol IN ?", symbols[start:end]).
				Updates(map[string]any{"listing_status": ListingStatusDelisted, "updated_at": now}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *Repository) Coverage(ctx context.Context) ([]CoverageByExchange, error) {
	universe, err := r.ListLatestUniverse(ctx, "ALL", 0)
	if err != nil {
		return nil, err
	}
	profiles, err := r.ListProfiles(ctx)
	if err != nil {
		return nil, err
	}
	byExchange := map[string]*CoverageByExchange{}
	ensure := func(ex string) *CoverageByExchange {
		if strings.TrimSpace(ex) == "" {
			ex = "UNKNOWN"
		}
		row := byExchange[ex]
		if row == nil {
			row = &CoverageByExchange{Exchange: ex}
			byExchange[ex] = row
		}
		return row
	}
	for _, item := range universe {
		ensure(item.Exchange).UniverseCount++
	}
	for _, profile := range profiles {
		row := ensure(profile.Exchange)
		if profile.ListingStatus == ListingStatusDelisted {
			row.DelistedCount++
			continue
		}
		row.ProfileCount++
		switch normalizeProfileStatus(profile.ProfileStatus) {
		case ProfileStatusComplete:
			row.CompleteCount++
		case ProfileStatusFailed:
			row.FailedCount++
		default:
			row.PendingCount++
		}
	}
	result := make([]CoverageByExchange, 0, len(byExchange))
	for _, row := range byExchange {
		if row.UniverseCount > 0 {
			row.CoverageRate = float64(row.ProfileCount) / float64(row.UniverseCount)
		}
		result = append(result, *row)
	}
	return result, nil
}

func (r *Repository) ListFailureItems(ctx context.Context, limit int) ([]CompanyProfileFailureItem, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	var records []CompanyProfileRecord
	err := r.db.WithContext(ctx).Where("profile_status IN ? OR quality_flags <> ?", []string{ProfileStatusFailed, ProfileStatusPending}, "[]").Order("updated_at DESC").Limit(limit).Find(&records).Error
	if err != nil {
		return nil, err
	}
	items := make([]CompanyProfileFailureItem, 0, len(records))
	for _, r := range records {
		items = append(items, CompanyProfileFailureItem{Symbol: r.Symbol, Name: r.Name, Exchange: r.Exchange, ProfileStatus: r.ProfileStatus, QualityFlags: parseQualityFlags(r.QualityFlags), UpdatedAt: formatTime(r.UpdatedAt)})
	}
	return items, nil
}

func buildSymbol(code, exchange string) string {
	code = strings.TrimSpace(code)
	exchange = strings.ToUpper(strings.TrimSpace(exchange))
	if code == "" {
		return ""
	}
	switch exchange {
	case "HKEX":
		if len(code) < 5 {
			code = strings.Repeat("0", 5-len(code)) + code
		}
		return code + ".HK"
	case "SSE":
		return code + ".SH"
	case "SZSE":
		return code + ".SZ"
	default:
		return ""
	}
}

func (r *Repository) UpsertIndustryMappings(ctx context.Context, records []IndustryMappingRecord) error {
	if len(records) == 0 {
		return nil
	}
	now := time.Now().UTC()
	cleaned := make([]IndustryMappingRecord, 0, len(records))
	for _, record := range records {
		record.Source = strings.TrimSpace(record.Source)
		record.SourceIndustryName = strings.TrimSpace(record.SourceIndustryName)
		if record.Source == "" || record.SourceIndustryName == "" {
			continue
		}
		if strings.TrimSpace(record.ExchangeScope) == "" {
			record.ExchangeScope = "ALL"
		}
		if record.CreatedAt.IsZero() {
			record.CreatedAt = now
		}
		if record.UpdatedAt.IsZero() {
			record.UpdatedAt = now
		}
		cleaned = append(cleaned, record)
	}
	if len(cleaned) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for start := 0; start < len(cleaned); start += sqliteSafeBatchSize {
			end := start + sqliteSafeBatchSize
			if end > len(cleaned) {
				end = len(cleaned)
			}
			batch := cleaned[start:end]
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "source"}, {Name: "source_industry_name"}},
				DoUpdates: clause.AssignmentColumns([]string{
					"standard_industry_code", "standard_industry_name", "standard_level", "exchange_scope", "note", "updated_at",
				}),
			}).Create(&batch).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *Repository) GetIndustryMapping(ctx context.Context, source, sourceIndustryName string) (*IndustryMappingRecord, error) {
	var record IndustryMappingRecord
	err := r.db.WithContext(ctx).
		Where("source = ? AND source_industry_name = ?", strings.TrimSpace(source), strings.TrimSpace(sourceIndustryName)).
		First(&record).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}

func normalizeListingStatus(input string) string {
	switch strings.ToUpper(strings.TrimSpace(input)) {
	case ListingStatusListed:
		return ListingStatusListed
	case ListingStatusDelisted:
		return ListingStatusDelisted
	case ListingStatusSuspended:
		return ListingStatusSuspended
	default:
		return ListingStatusUnknown
	}
}

func normalizeProfileStatus(input string) string {
	switch strings.ToUpper(strings.TrimSpace(input)) {
	case ProfileStatusComplete:
		return ProfileStatusComplete
	case ProfileStatusPartial:
		return ProfileStatusPartial
	case ProfileStatusFailed:
		return ProfileStatusFailed
	default:
		return ProfileStatusPending
	}
}
