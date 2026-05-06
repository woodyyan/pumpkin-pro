package companyprofile

import "time"

const (
	ListingStatusListed    = "LISTED"
	ListingStatusDelisted  = "DELISTED"
	ListingStatusSuspended = "SUSPENDED"
	ListingStatusUnknown   = "UNKNOWN"

	ProfileStatusComplete = "COMPLETE"
	ProfileStatusPartial  = "PARTIAL"
	ProfileStatusPending  = "PENDING"
	ProfileStatusFailed   = "FAILED"

	SummarySourceExtract      = "source_extract"
	SummarySourceRuleTemplate = "rule_template"
	SummarySourceFallback     = "fallback"
	SummarySourceModelOffline = "model_offline"
)

// CompanyProfileRecord stores static company/security master data used by the
// public "About" card on the symbol detail page.
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

type CompanyAboutProfile struct {
	Name                  string `json:"name"`
	FullName              string `json:"full_name"`
	BoardCode             string `json:"board_code"`
	BoardName             string `json:"board_name"`
	RawIndustryName       string `json:"raw_industry_name"`
	IndustryCode          string `json:"industry_code"`
	IndustryName          string `json:"industry_name"`
	IndustryLevel         string `json:"industry_level"`
	IndustrySource        string `json:"industry_source"`
	Website               string `json:"website"`
	FoundedDate           string `json:"founded_date"`
	FoundedDatePrecision  string `json:"founded_date_precision"`
	IPODate               string `json:"ipo_date"`
	ListingStatus         string `json:"listing_status"`
	DelistedDate          string `json:"delisted_date"`
	BusinessSummary       string `json:"business_summary"`
	BusinessSummarySource string `json:"business_summary_source"`
	BusinessScope         string `json:"business_scope"`
}

type CompanyAboutMeta struct {
	ProfileStatus   string   `json:"profile_status"`
	Source          string   `json:"source,omitempty"`
	SourceURL       string   `json:"source_url,omitempty"`
	SourceUpdatedAt string   `json:"source_updated_at,omitempty"`
	UpdatedAt       string   `json:"updated_at,omitempty"`
	QualityFlags    []string `json:"quality_flags"`
	Message         string   `json:"message,omitempty"`
}

type CompanyAboutPayload struct {
	Symbol     string               `json:"symbol"`
	Exchange   string               `json:"exchange"`
	HasProfile bool                 `json:"has_profile"`
	Profile    *CompanyAboutProfile `json:"profile"`
	Meta       CompanyAboutMeta     `json:"meta"`
}

type UniverseSecurity struct {
	Symbol    string `json:"symbol"`
	Code      string `json:"code"`
	Name      string `json:"name"`
	Exchange  string `json:"exchange"`
	BoardCode string `json:"board_code"`
}

type CoverageByExchange struct {
	Exchange      string  `json:"exchange"`
	UniverseCount int64   `json:"universe_count"`
	ProfileCount  int64   `json:"profile_count"`
	CompleteCount int64   `json:"complete_count"`
	PendingCount  int64   `json:"pending_count"`
	FailedCount   int64   `json:"failed_count"`
	DelistedCount int64   `json:"delisted_count"`
	CoverageRate  float64 `json:"coverage_rate"`
}

type CompanyProfileFailureItem struct {
	Symbol        string   `json:"symbol"`
	Name          string   `json:"name"`
	Exchange      string   `json:"exchange"`
	ProfileStatus string   `json:"profile_status"`
	QualityFlags  []string `json:"quality_flags"`
	UpdatedAt     string   `json:"updated_at"`
}

type AdminCompanyProfileOverview struct {
	Coverage  []CoverageByExchange        `json:"coverage"`
	Failures  []CompanyProfileFailureItem `json:"failures"`
	Refresh   CompanyProfileRefreshStatus `json:"refresh"`
	UpdatedAt string                      `json:"updated_at"`
}

type CompanyProfileRefreshStatus struct {
	Running       bool   `json:"running"`
	Status        string `json:"status"`
	StartedAt     string `json:"started_at,omitempty"`
	FinishedAt    string `json:"finished_at,omitempty"`
	TotalCount    int    `json:"total_count"`
	SuccessCount  int    `json:"success_count"`
	FailedCount   int    `json:"failed_count"`
	NewCount      int    `json:"new_count"`
	DelistedCount int    `json:"delisted_count"`
	Message       string `json:"message,omitempty"`
	Error         string `json:"error,omitempty"`
}

type CompanyProfileRefreshRequest struct {
	Exchange string `json:"exchange"`
	Limit    int    `json:"limit"`
}
