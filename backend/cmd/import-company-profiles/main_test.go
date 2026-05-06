package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/woodyyan/pumpkin-pro/backend/store/companyprofile"
	"gorm.io/gorm"
)

func TestParseOptionsRequiresInput(t *testing.T) {
	_, err := parseOptions([]string{"--db", "pumpkin.db"}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error when input paths are missing")
	}
}

func TestLoadCompanyProfilesJSONL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profiles.jsonl")
	content := `{"symbol":"600519.SH","exchange":"SSE","code":"600519","name":"贵州茅台","raw_industry_name":"食品饮料Ⅰ","industry_name":"食品饮料","listing_status":"LISTED","profile_status":"COMPLETE"}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write fixture failed: %v", err)
	}
	rows, err := loadCompanyProfiles(path)
	if err != nil {
		t.Fatalf("loadCompanyProfiles failed: %v", err)
	}
	if len(rows) != 1 || rows[0].Symbol != "600519.SH" || rows[0].IndustryName != "食品饮料" {
		t.Fatalf("unexpected rows: %#v", rows)
	}
	if rows[0].CreatedAt.IsZero() || rows[0].UpdatedAt.IsZero() {
		t.Fatal("expected timestamps to be filled")
	}
}

func TestRunImportsProfilesAndMappings(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "pumpkin.db")
	profilePath := filepath.Join(dir, "profiles.jsonl")
	mappingPath := filepath.Join(dir, "industry.jsonl")
	profile := `{"symbol":"00700.HK","exchange":"HKEX","code":"00700","name":"腾讯控股","industry_source":"eastmoney_hk","raw_industry_name":"软件服务","industry_name":"软件服务","listing_status":"LISTED","profile_status":"PARTIAL"}` + "\n"
	mapping := `{"source":"eastmoney_hk","source_industry_name":"软件服务","standard_industry_code":"software_services","standard_industry_name":"软件服务","standard_level":"l1"}` + "\n"
	if err := os.WriteFile(profilePath, []byte(profile), 0644); err != nil {
		t.Fatalf("write profile fixture failed: %v", err)
	}
	if err := os.WriteFile(mappingPath, []byte(mapping), 0644); err != nil {
		t.Fatalf("write mapping fixture failed: %v", err)
	}
	stdout := &bytes.Buffer{}
	if err := run([]string{"--db", dbPath, "--input", profilePath, "--industry-mapping", mappingPath, "--write"}, stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "imported 1 company profiles and 1 industry mappings") {
		t.Fatalf("unexpected stdout: %s", stdout.String())
	}
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db failed: %v", err)
	}
	repo := companyprofile.NewRepository(db)
	got, err := repo.GetBySymbol(context.Background(), "00700.HK")
	if err != nil {
		t.Fatalf("GetBySymbol failed: %v", err)
	}
	if got == nil || got.Name != "腾讯控股" {
		t.Fatalf("unexpected imported profile: %#v", got)
	}
	mappingRecord, err := repo.GetIndustryMapping(context.Background(), "eastmoney_hk", "软件服务")
	if err != nil {
		t.Fatalf("GetIndustryMapping failed: %v", err)
	}
	if mappingRecord == nil || mappingRecord.StandardIndustryCode != "software_services" {
		t.Fatalf("unexpected imported mapping: %#v", mappingRecord)
	}
}
