package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/woodyyan/pumpkin-pro/backend/store/companyprofile"
	"gorm.io/gorm"
)

type importOptions struct {
	DBPath            string
	InputPath         string
	IndustryInputPath string
	Write             bool
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "import-company-profiles failed: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	opts, err := parseOptions(args, stderr)
	if err != nil {
		return err
	}
	profiles, err := loadCompanyProfiles(opts.InputPath)
	if err != nil {
		return err
	}
	mappings, err := loadIndustryMappings(opts.IndustryInputPath)
	if err != nil {
		return err
	}
	if !opts.Write {
		fmt.Fprintf(stdout, "dry-run: loaded %d company profiles and %d industry mappings; pass --write to persist\n", len(profiles), len(mappings))
		return nil
	}
	if strings.TrimSpace(opts.DBPath) == "" {
		return errors.New("--db is required when --write is set")
	}
	db, err := gorm.Open(sqlite.Open(opts.DBPath), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	if err := companyprofile.NewMigrator().AutoMigrate(db); err != nil {
		return fmt.Errorf("migrate companyprofile tables: %w", err)
	}
	repo := companyprofile.NewRepository(db)
	ctx := context.Background()
	if err := repo.BulkUpsert(ctx, profiles); err != nil {
		return fmt.Errorf("upsert company profiles: %w", err)
	}
	if err := repo.UpsertIndustryMappings(ctx, mappings); err != nil {
		return fmt.Errorf("upsert industry mappings: %w", err)
	}
	fmt.Fprintf(stdout, "imported %d company profiles and %d industry mappings\n", len(profiles), len(mappings))
	return nil
}

func parseOptions(args []string, output io.Writer) (importOptions, error) {
	fs := flag.NewFlagSet("import-company-profiles", flag.ContinueOnError)
	fs.SetOutput(output)
	opts := importOptions{}
	fs.StringVar(&opts.DBPath, "db", "", "SQLite DB path, e.g. ../data/pumpkin.db")
	fs.StringVar(&opts.InputPath, "input", "", "Company profile JSON/JSONL file path")
	fs.StringVar(&opts.IndustryInputPath, "industry-mapping", "", "Optional industry mapping JSON/JSONL file path")
	fs.BoolVar(&opts.Write, "write", false, "Persist imported rows into SQLite")
	if err := fs.Parse(args); err != nil {
		return importOptions{}, err
	}
	if strings.TrimSpace(opts.InputPath) == "" && strings.TrimSpace(opts.IndustryInputPath) == "" {
		return importOptions{}, errors.New("--input or --industry-mapping is required")
	}
	return opts, nil
}

func loadCompanyProfiles(path string) ([]companyprofile.CompanyProfileRecord, error) {
	if strings.TrimSpace(path) == "" {
		return []companyprofile.CompanyProfileRecord{}, nil
	}
	rows, err := readJSONLinesOrArray[companyprofile.CompanyProfileRecord](path)
	if err != nil {
		return nil, fmt.Errorf("read company profiles: %w", err)
	}
	now := time.Now().UTC()
	for i := range rows {
		if rows[i].CreatedAt.IsZero() {
			rows[i].CreatedAt = now
		}
		if rows[i].UpdatedAt.IsZero() {
			rows[i].UpdatedAt = now
		}
	}
	return rows, nil
}

func loadIndustryMappings(path string) ([]companyprofile.IndustryMappingRecord, error) {
	if strings.TrimSpace(path) == "" {
		return []companyprofile.IndustryMappingRecord{}, nil
	}
	rows, err := readJSONLinesOrArray[companyprofile.IndustryMappingRecord](path)
	if err != nil {
		return nil, fmt.Errorf("read industry mappings: %w", err)
	}
	now := time.Now().UTC()
	for i := range rows {
		if rows[i].CreatedAt.IsZero() {
			rows[i].CreatedAt = now
		}
		if rows[i].UpdatedAt.IsZero() {
			rows[i].UpdatedAt = now
		}
	}
	return rows, nil
}

func readJSONLinesOrArray[T any](path string) ([]T, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader := bufio.NewReader(file)
	peek, err := reader.Peek(1)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return []T{}, nil
		}
		return nil, err
	}
	if len(peek) > 0 && peek[0] == '[' {
		var rows []T
		if err := json.NewDecoder(reader).Decode(&rows); err != nil {
			return nil, err
		}
		return rows, nil
	}
	rows := []T{}
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var row T
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return rows, nil
}
