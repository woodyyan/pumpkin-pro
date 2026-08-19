package companyprofile

import (
	"testing"

	"github.com/woodyyan/pumpkin-pro/backend/tests/testutil"
)

func TestMigratorPreservesFactorLabIndustrySchema(t *testing.T) {
	db := testutil.InMemoryDB(t)
	if err := NewMigrator().AutoMigrate(db); err != nil {
		t.Fatalf("migrate company profile industry tables: %v", err)
	}

	for _, model := range []any{&CompanyProfileRecord{}, &IndustryMappingRecord{}} {
		if !db.Migrator().HasTable(model) {
			t.Fatalf("expected table for %T", model)
		}
	}
	for _, column := range []string{
		"symbol", "exchange", "code", "name", "board_code", "raw_industry_name",
		"industry_code", "industry_name", "industry_level", "industry_source",
		"listing_status", "profile_status", "quality_flags", "created_at", "updated_at",
	} {
		if !db.Migrator().HasColumn(&CompanyProfileRecord{}, column) {
			t.Fatalf("company_profiles missing required Factor Lab column %q", column)
		}
	}
}
