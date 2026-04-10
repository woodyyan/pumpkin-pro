package auth

import (
	"context"
	"testing"
	"time"

	"github.com/woodyyan/pumpkin-pro/backend/tests/testutil"
)

func setupAuthDB(t *testing.T) (*Repository, context.Context) {
	t.Helper()
	db := testutil.InMemoryDB(t)
	testutil.AutoMigrateModels(t, db,
		UserRecord{},
		UserProfileRecord{},
		UserSessionRecord{},
		AuthAuditRecord{},
	)
	repo := NewRepository(db)
	return repo, context.Background()
}

func TestCreateUserAndGetByEmail(t *testing.T) {
	repo, ctx := setupAuthDB(t)

	user := UserRecord{
		ID:           "user-001",
		Email:        "test@example.com",
		PasswordHash: "$2a$10$hashedpassword",
		Status:       "active",
	}
	profile := UserProfileRecord{
		UserID:   user.ID,
		Nickname: "testuser",
	}

	err := repo.CreateUser(ctx, user, profile)
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	found, err := repo.GetUserByEmail(ctx, "test@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail failed: %v", err)
	}
	if found.ID != "user-001" {
		t.Errorf("expected user-001, got %s", found.ID)
	}
	if found.Email != "test@example.com" {
		t.Errorf("expected email test@example.com, got %s", found.Email)
	}
}

func TestGetUserByID(t *testing.T) {
	repo, ctx := setupAuthDB(t)

	user := UserRecord{
		ID:           "user-002",
		Email:        "idlookup@example.com",
		PasswordHash: "$2a$10$hash",
		Status:       "active",
	}
	profile := UserProfileRecord{
		UserID: user.ID,
	}
	_ = repo.CreateUser(ctx, user, profile)

	found, err := repo.GetUserByID(ctx, "user-002")
	if err != nil {
		t.Fatalf("GetUserByID failed: %v", err)
	}
	if found.Email != "idlookup@example.com" {
		t.Errorf("expected idlookup@example.com, got %s", found.Email)
	}
}

func TestGetUserNotFound(t *testing.T) {
	repo, ctx := setupAuthDB(t)

	_, err := repo.GetUserByEmail(ctx, "nonexist@example.com")
	if err != ErrUserNotFound {
		t.Errorf("expected ErrUserNotFound, got %v", err)
	}

	_, err = repo.GetUserByID(ctx, "nonexistent-id")
	if err != ErrUserNotFound {
		t.Errorf("expected ErrUserNotFound, got %v", err)
	}
}

func TestDuplicateEmail(t *testing.T) {
	repo, ctx := setupAuthDB(t)

	user1 := UserRecord{
		ID:           "user-a",
		Email:        "dup@example.com",
		PasswordHash: "$2a$10$hash1",
		Status:       "active",
	}
	profile1 := UserProfileRecord{UserID: "user-a"}
	_ = repo.CreateUser(ctx, user1, profile1)

	user2 := UserRecord{
		ID:           "user-b",
		Email:        "dup@example.com",
		PasswordHash: "$2a$10$hash2",
		Status:       "active",
	}
	profile2 := UserProfileRecord{UserID: "user-b"}
	err := repo.CreateUser(ctx, user2, profile2)
	if err != ErrEmailAlreadyExists {
		t.Errorf("expected ErrEmailAlreadyExists for duplicate email, got %v", err)
	}
}

func TestUpdatePasswordHash(t *testing.T) {
	repo, ctx := setupAuthDB(t)

	user := UserRecord{
		ID:           "pw-user",
		Email:        "pw@example.com",
		PasswordHash: "old-hash",
		Status:       "active",
	}
	profile := UserProfileRecord{UserID: "pw-user"}
	_ = repo.CreateUser(ctx, user, profile)

	newHash := "new-hash-updated"
	err := repo.UpdatePasswordHash(ctx, "pw-user", newHash)
	if err != nil {
		t.Fatalf("UpdatePasswordHash failed: %v", err)
	}

	updated, _ := repo.GetUserByID(ctx, "pw-user")
	if updated.PasswordHash != newHash {
		t.Errorf("expected password hash %s, got %s", newHash, updated.PasswordHash)
	}
}

func TestSessionLifecycle(t *testing.T) {
	repo, ctx := setupAuthDB(t)

	session := UserSessionRecord{
		ID:               "sess-001",
		UserID:           "session-user",
		RefreshTokenHash: "hash-token-xyz",
		ExpiresAt:        nowUTC().Add(168 * time.Hour),
	}

	// Create session
	err := repo.CreateSession(ctx, session)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Get by hash
	found, err := repo.GetSessionByRefreshHash(ctx, "hash-token-xyz")
	if err != nil {
		t.Fatalf("GetSessionByRefreshHash failed: %v", err)
	}
	if found.ID != "sess-001" {
		t.Errorf("expected sess-001, got %s", found.ID)
	}

	// Revoke by ID
	err = repo.RevokeSessionByID(ctx, "sess-001")
	if err != nil {
		t.Fatalf("RevokeSessionByID failed: %v", err)
	}

	// After revoke, the session should still be findable (revoked_at is set but record exists)
	revoked, _ := repo.GetSessionByRefreshHash(ctx, "hash-token-xyz")
	if revoked.RevokedAt == nil {
		t.Error("expected RevokedAt to be set after revocation")
	}
}
