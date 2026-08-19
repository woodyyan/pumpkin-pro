package companyprofile

import "time"

// CompanyProfileRecord preserves the shared company_profiles schema used by
// Factor Lab industry refresh and Phase 2 industry classification.
type CompanyProfileRecord struct {
	Symbol                string    `gorm:"primaryKey;size:16" json:"symbol"`
	Exchange              string    `gorm:"size:8;not null;default:'';index" json:"exchange"`
	Code                  string    `gorm:"size:16;not null;default:'';index" json:"code"`
	Name                  string    `gorm:"size:128;not null;default:''" json:"name"`
	FullName              string    `gorm:"size:256;not null;default:''" json:"full_name"`
	BoardCode             string    `gorm:"size:32;not null;default:'';index" json:"board_code"`
	BoardName             string    `gorm:"size:64;not null;default:''" json:"board_name"`
	RawIndustryName       string    `gorm:"size:128;not null;default:''" json:"raw_industry_name"`
	IndustryCode          string    `gorm:"size:64;not null;default:'';index" json:"industry_code"`
	IndustryName          string    `gorm:"size:128;not null;default:'';index" json:"industry_name"`
	IndustryLevel         string    `gorm:"size:32;not null;default:''" json:"industry_level"`
	IndustrySource        string    `gorm:"size:32;not null;default:''" json:"industry_source"`
	Website               string    `gorm:"size:512;not null;default:''" json:"website"`
	FoundedDate           string    `gorm:"size:10;not null;default:''" json:"founded_date"`
	FoundedDatePrecision  string    `gorm:"size:16;not null;default:'unknown'" json:"founded_date_precision"`
	IPODate               string    `gorm:"column:ipo_date;size:10;not null;default:''" json:"ipo_date"`
	ListingStatus         string    `gorm:"size:16;not null;default:'UNKNOWN';index" json:"listing_status"`
	DelistedDate          string    `gorm:"size:10;not null;default:''" json:"delisted_date"`
	BusinessScope         string    `gorm:"type:text;not null;default:''" json:"business_scope"`
	BusinessSummary       string    `gorm:"size:512;not null;default:''" json:"business_summary"`
	BusinessSummarySource string    `gorm:"size:32;not null;default:''" json:"business_summary_source"`
	Source                string    `gorm:"size:64;not null;default:''" json:"source"`
	SourceURL             string    `gorm:"column:source_url;size:512;not null;default:''" json:"source_url"`
	SourceUpdatedAt       time.Time `json:"source_updated_at"`
	ProfileStatus         string    `gorm:"size:16;not null;default:'PENDING';index" json:"profile_status"`
	QualityFlags          string    `gorm:"type:text;not null;default:'[]'" json:"quality_flags"`
	CreatedAt             time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt             time.Time `gorm:"not null" json:"updated_at"`
}

func (CompanyProfileRecord) TableName() string {
	return "company_profiles"
}

// IndustryMappingRecord stores an explicit mapping from source-specific
// industries to a maintained standard industry taxonomy. Phase 1 displays the
// cleaned source industry; this table is the Phase 2 foundation for later
// standardization without changing company_profiles rows in place.
type IndustryMappingRecord struct {
	ID                   uint      `gorm:"primaryKey" json:"id"`
	Source               string    `gorm:"size:32;not null;uniqueIndex:uidx_industry_mapping_source_raw,priority:1" json:"source"`
	SourceIndustryName   string    `gorm:"size:128;not null;uniqueIndex:uidx_industry_mapping_source_raw,priority:2" json:"source_industry_name"`
	StandardIndustryCode string    `gorm:"size:64;not null;default:'';index" json:"standard_industry_code"`
	StandardIndustryName string    `gorm:"size:128;not null;default:'';index" json:"standard_industry_name"`
	StandardLevel        string    `gorm:"size:32;not null;default:''" json:"standard_level"`
	ExchangeScope        string    `gorm:"size:16;not null;default:'ALL';index" json:"exchange_scope"`
	Note                 string    `gorm:"size:512;not null;default:''" json:"note"`
	CreatedAt            time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt            time.Time `gorm:"not null" json:"updated_at"`
}

func (IndustryMappingRecord) TableName() string {
	return "industry_mapping"
}
