package analytics

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := db.AutoMigrate(&DeviceSnapshot{}); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
	return db
}

func seedDeviceSnapshots(t *testing.T, db *gorm.DB) {
	now := time.Now().UTC()
	snaps := []DeviceSnapshot{
		{UserID: "u1", VisitorID: "v1", Source: "page_view", DeviceType: "desktop", OSFamily: "macOS", BrowserFamily: "Chrome", CreatedAt: now.AddDate(0, 0, -1)},
		{UserID: "u1", VisitorID: "v1", Source: "page_view", DeviceType: "desktop", OSFamily: "macOS", BrowserFamily: "Chrome", CreatedAt: now.AddDate(0, 0, -2)},
		{UserID: "u2", VisitorID: "v2", Source: "page_view", DeviceType: "mobile", OSFamily: "iOS", BrowserFamily: "WeChat", CreatedAt: now.AddDate(0, 0, -1)},
		{UserID: "", VisitorID: "v3", Source: "page_view", DeviceType: "mobile", OSFamily: "Android", BrowserFamily: "Chrome", CreatedAt: now.AddDate(0, 0, -3)},
		{UserID: "u3", VisitorID: "v4", Source: "auth", DeviceType: "desktop", OSFamily: "Windows", BrowserFamily: "Edge", CreatedAt: now.AddDate(0, 0, -5)},
		{UserID: "u3", VisitorID: "v4", Source: "page_view", DeviceType: "desktop", OSFamily: "Windows", BrowserFamily: "Edge", CreatedAt: now.AddDate(0, 0, -4)},
	}
	for _, s := range snaps {
		if err := db.Create(&s).Error; err != nil {
			t.Fatalf("seed snapshot: %v", err)
		}
	}
}

func seedAuthAuditLogs(t *testing.T, db *gorm.DB) {
	now := time.Now().UTC()
	// We need to create the auth_audit_logs table for the top users query
	err := db.Exec(`CREATE TABLE IF NOT EXISTS auth_audit_logs (
		id TEXT PRIMARY KEY,
		user_id TEXT,
		email_masked TEXT,
		action TEXT,
		ip TEXT,
		user_agent TEXT,
		success INTEGER,
		message TEXT,
		created_at DATETIME
	)`).Error
	if err != nil {
		t.Fatalf("create auth_audit_logs: %v", err)
	}

	// Create users table for email lookup
	err = db.Exec(`CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY,
		email TEXT NOT NULL,
		password_hash TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'active',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	)`).Error
	if err != nil {
		t.Fatalf("create users: %v", err)
	}

	logs := []struct {
		ID        string
		UserID    string
		Action    string
		Success   int
		CreatedAt time.Time
	}{
		{"a1", "u1", "login", 1, now.AddDate(0, 0, -1)},
		{"a2", "u1", "login", 1, now.AddDate(0, 0, -2)},
		{"a3", "u1", "login", 1, now.AddDate(0, 0, -3)},
		{"a4", "u2", "login", 1, now.AddDate(0, 0, -1)},
		{"a5", "u2", "login", 1, now.AddDate(0, 0, -2)},
		{"a6", "u3", "login", 1, now.AddDate(0, 0, -5)},
		{"a7", "u3", "login", 1, now.AddDate(0, 0, -4)},
	}
	for _, l := range logs {
		err := db.Exec(`INSERT INTO auth_audit_logs (id, user_id, action, success, created_at) VALUES (?, ?, ?, ?, ?)`,
			l.ID, l.UserID, l.Action, l.Success, l.CreatedAt).Error
		if err != nil {
			t.Fatalf("seed auth log: %v", err)
		}
	}

	// Seed users with emails
	users := []struct {
		ID           string
		Email        string
		PasswordHash string
		CreatedAt    time.Time
		UpdatedAt    time.Time
	}{
		{"u1", "alice@example.com", "hash1", now, now},
		{"u2", "bob@example.com", "hash2", now, now},
		{"u3", "", "hash3", now, now},
	}
	for _, u := range users {
		err := db.Exec(`INSERT INTO users (id, email, password_hash, status, created_at, updated_at) VALUES (?, ?, ?, 'active', ?, ?)`,
			u.ID, u.Email, u.PasswordHash, u.CreatedAt, u.UpdatedAt).Error
		if err != nil {
			t.Fatalf("seed user: %v", err)
		}
	}
}

func TestGetDeviceAnalytics(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)
	seedDeviceSnapshots(t, db)

	ctx := context.Background()
	since := time.Now().UTC().AddDate(0, 0, -10)

	result, err := repo.GetDeviceAnalytics(ctx, since)
	if err != nil {
		t.Fatalf("GetDeviceAnalytics error: %v", err)
	}

	// Device types: desktop (v1, v4=2 distinct), mobile (v2, v3=2 distinct) => total 4
	if len(result.DeviceTypes) != 2 {
		t.Errorf("device types count = %d, want 2", len(result.DeviceTypes))
	}

	// OS families: macOS (v1), iOS (v2), Android (v3), Windows (v4) => 4 distinct
	if len(result.OSFamilies) != 4 {
		t.Errorf("os families count = %d, want 4", len(result.OSFamilies))
	}

	// Browser families: Chrome (v1, v3=2 distinct), WeChat (v2), Edge (v4) => 3
	if len(result.BrowserFamilies) != 3 {
		t.Errorf("browser families count = %d, want 3", len(result.BrowserFamilies))
	}

	// Check Chrome percentage (2 out of 4 = 50%)
	for _, b := range result.BrowserFamilies {
		if b.Category == "Chrome" && b.Count != 2 {
			t.Errorf("Chrome count = %d, want 2", b.Count)
		}
	}
}

func TestGetDeviceAnalyticsIgnoresAPIErrors(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)
	now := time.Now().UTC()

	// Insert real user page_view records
	snaps := []DeviceSnapshot{
		{UserID: "u1", VisitorID: "v1", Source: "page_view", DeviceType: "desktop", OSFamily: "macOS", BrowserFamily: "Chrome", CreatedAt: now.AddDate(0, 0, -1)},
		{UserID: "u2", VisitorID: "v2", Source: "auth", DeviceType: "mobile", OSFamily: "iOS", BrowserFamily: "Safari", CreatedAt: now.AddDate(0, 0, -1)},
	}
	for _, s := range snaps {
		if err := db.Create(&s).Error; err != nil {
			t.Fatalf("seed snapshot: %v", err)
		}
	}

	// Insert api_error noise that should be IGNORED
	noise := []DeviceSnapshot{
		{UserID: "", VisitorID: "v99", Source: "api_error", DeviceType: "unknown", OSFamily: "unknown", BrowserFamily: "node", CreatedAt: now.AddDate(0, 0, -1)},
		{UserID: "", VisitorID: "v98", Source: "api_error", DeviceType: "unknown", OSFamily: "unknown", BrowserFamily: "unknown", CreatedAt: now.AddDate(0, 0, -2)},
	}
	for _, s := range noise {
		if err := db.Create(&s).Error; err != nil {
			t.Fatalf("seed noise snapshot: %v", err)
		}
	}

	ctx := context.Background()
	since := now.AddDate(0, 0, -10)

	result, err := repo.GetDeviceAnalytics(ctx, since)
	if err != nil {
		t.Fatalf("GetDeviceAnalytics error: %v", err)
	}

	// Browser: only Chrome and Safari should appear; "node" must NOT appear
	var hasNode bool
	var chromeCount int64
	for _, b := range result.BrowserFamilies {
		if b.Category == "node" {
			hasNode = true
		}
		if b.Category == "Chrome" {
			chromeCount = b.Count
		}
	}
	if hasNode {
		t.Error("browser families should not contain 'node' from api_error sources")
	}
	if chromeCount != 1 {
		t.Errorf("Chrome count = %d, want 1", chromeCount)
	}

	// Device types: only desktop and mobile; no unknown from api_error
	for _, d := range result.DeviceTypes {
		if d.Category == "unknown" {
			t.Error("device types should not contain 'unknown' from api_error sources")
		}
	}

	// OS: only macOS and iOS; no unknown from api_error
	for _, o := range result.OSFamilies {
		if o.Category == "unknown" {
			t.Error("os families should not contain 'unknown' from api_error sources")
		}
	}
}

func TestGetDeviceAnalyticsEmpty(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	ctx := context.Background()
	since := time.Now().UTC().AddDate(0, 0, -10)

	result, err := repo.GetDeviceAnalytics(ctx, since)
	if err != nil {
		t.Fatalf("GetDeviceAnalytics error: %v", err)
	}
	if len(result.DeviceTypes) != 0 {
		t.Errorf("empty device types count = %d, want 0", len(result.DeviceTypes))
	}
	if len(result.OSFamilies) != 0 {
		t.Errorf("empty os families count = %d, want 0", len(result.OSFamilies))
	}
	if len(result.BrowserFamilies) != 0 {
		t.Errorf("empty browser families count = %d, want 0", len(result.BrowserFamilies))
	}
}

func TestGetTopActiveUsersWithDevices(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)
	seedDeviceSnapshots(t, db)
	seedAuthAuditLogs(t, db)

	now := time.Now().UTC()
	// Insert api_error noise for u1 that is MORE recent than the page_view record.
	// This tests that GetTopActiveUsersWithDevices ignores api_error sources.
	noise := DeviceSnapshot{
		UserID:        "u1",
		VisitorID:     "v99",
		Source:        "api_error",
		DeviceType:    "unknown",
		OSFamily:      "unknown",
		BrowserFamily: "node",
		CreatedAt:     now.AddDate(0, 0, -1),
	}
	if err := db.Create(&noise).Error; err != nil {
		t.Fatalf("seed api_error noise: %v", err)
	}

	ctx := context.Background()
	since := now.AddDate(0, 0, -10)

	users, err := repo.GetTopActiveUsersWithDevices(ctx, since, 20)
	if err != nil {
		t.Fatalf("GetTopActiveUsersWithDevices error: %v", err)
	}
	if len(users) == 0 {
		t.Fatal("expected users, got none")
	}

	// u1 has 3 active days, u2 has 2, u3 has 2
	// u1 should be first
	if users[0].UserID != "u1" {
		t.Errorf("first user = %s, want u1", users[0].UserID)
	}
	if users[0].ActiveDays != 3 {
		t.Errorf("u1 active days = %d, want 3", users[0].ActiveDays)
	}
	// u1's email should be fetched from users table
	if users[0].Email != "alice@example.com" {
		t.Errorf("u1 email = %s, want alice@example.com", users[0].Email)
	}
	// u1's most recent device must come from page_view/auth, NOT the more recent api_error noise
	if users[0].Browser != "Chrome" {
		t.Errorf("u1 browser = %s, want Chrome (api_error 'node' should be ignored)", users[0].Browser)
	}
	if users[0].OS != "macOS" {
		t.Errorf("u1 os = %s, want macOS (api_error 'unknown' should be ignored)", users[0].OS)
	}
	// u3 has no email, should be empty string
	for _, u := range users {
		if u.UserID == "u3" && u.Email != "" {
			t.Errorf("u3 email = %s, want empty", u.Email)
		}
	}
}

func TestGetTopActiveUsersWithDevicesEmpty(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)
	// Create auth_audit_logs table so the query doesn't fail
	if err := db.Exec(`CREATE TABLE IF NOT EXISTS auth_audit_logs (id TEXT PRIMARY KEY, user_id TEXT, action TEXT, success INTEGER, created_at DATETIME)`).Error; err != nil {
		t.Fatalf("create auth_audit_logs: %v", err)
	}
	// Create users table for the LEFT JOIN
	if err := db.Exec(`CREATE TABLE IF NOT EXISTS users (id TEXT PRIMARY KEY, email TEXT NOT NULL, password_hash TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'active', created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL)`).Error; err != nil {
		t.Fatalf("create users: %v", err)
	}

	ctx := context.Background()
	since := time.Now().UTC().AddDate(0, 0, -10)

	users, err := repo.GetTopActiveUsersWithDevices(ctx, since, 20)
	if err != nil {
		t.Fatalf("GetTopActiveUsersWithDevices error: %v", err)
	}
	if len(users) != 0 {
		t.Errorf("empty users count = %d, want 0", len(users))
	}
}
