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
		PasswordResetTokenRecord{},
		PasswordResetAttemptRecord{},
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

func TestPasswordResetTokenLifecycle(t *testing.T) {
	repo, ctx := setupAuthDB(t)
	now := nowUTC()
	user := UserRecord{ID: "reset-user", Email: "reset@example.com", PasswordHash: "hash", CredentialVersion: 1, Status: "active"}
	profile := UserProfileRecord{UserID: user.ID}
	if err := repo.CreateUser(ctx, user, profile); err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	token := PasswordResetTokenRecord{
		ID:        "prt-1",
		UserID:    user.ID,
		TokenHash: "token-hash-1",
		ExpiresAt: now.Add(30 * time.Minute),
		IP:        "127.0.0.1",
		CreatedAt: now,
	}
	if err := repo.CreatePasswordResetToken(ctx, token); err != nil {
		t.Fatalf("CreatePasswordResetToken failed: %v", err)
	}

	loaded, err := repo.GetPasswordResetTokenByHash(ctx, token.TokenHash)
	if err != nil {
		t.Fatalf("GetPasswordResetTokenByHash failed: %v", err)
	}
	if loaded.ID != token.ID {
		t.Fatalf("token ID = %s; want %s", loaded.ID, token.ID)
	}

	if err := repo.DeleteActivePasswordResetTokensByUserID(ctx, user.ID); err != nil {
		t.Fatalf("DeleteActivePasswordResetTokensByUserID failed: %v", err)
	}
	if _, err := repo.GetPasswordResetTokenByHash(ctx, token.TokenHash); err != ErrResetTokenNotFound {
		t.Fatalf("expected ErrResetTokenNotFound after delete, got %v", err)
	}
}

func TestPasswordResetAttemptsQueries(t *testing.T) {
	repo, ctx := setupAuthDB(t)
	now := nowUTC()
	attempts := []PasswordResetAttemptRecord{
		{ID: "a1", Email: "user@example.com", UserID: "u1", IP: "1.1.1.1", CreatedAt: now.Add(-50 * time.Minute)},
		{ID: "a2", Email: "user@example.com", UserID: "u1", IP: "1.1.1.1", CreatedAt: now.Add(-10 * time.Minute)},
		{ID: "a3", Email: "other@example.com", UserID: "u2", IP: "2.2.2.2", CreatedAt: now.Add(-5 * time.Minute)},
	}
	for _, item := range attempts {
		if err := repo.CreatePasswordResetAttempt(ctx, item); err != nil {
			t.Fatalf("CreatePasswordResetAttempt failed: %v", err)
		}
	}

	countEmail, err := repo.CountPasswordResetAttemptsByEmailSince(ctx, "user@example.com", now.Add(-1*time.Hour))
	if err != nil || countEmail != 2 {
		t.Fatalf("CountPasswordResetAttemptsByEmailSince = %d, %v; want 2, nil", countEmail, err)
	}
	countIP, err := repo.CountPasswordResetAttemptsByIPSince(ctx, "1.1.1.1", now.Add(-1*time.Hour))
	if err != nil || countIP != 2 {
		t.Fatalf("CountPasswordResetAttemptsByIPSince = %d, %v; want 2, nil", countIP, err)
	}
	latest, err := repo.GetLatestPasswordResetAttemptByEmail(ctx, "user@example.com")
	if err != nil || latest == nil || latest.ID != "a2" {
		t.Fatalf("latest attempt = %#v, err=%v; want a2", latest, err)
	}
	oldestEmail, err := repo.GetOldestPasswordResetAttemptByEmailSince(ctx, "user@example.com", now.Add(-1*time.Hour))
	if err != nil || oldestEmail == nil || oldestEmail.ID != "a1" {
		t.Fatalf("oldest email attempt = %#v, err=%v; want a1", oldestEmail, err)
	}
	oldestIP, err := repo.GetOldestPasswordResetAttemptByIPSince(ctx, "1.1.1.1", now.Add(-1*time.Hour))
	if err != nil || oldestIP == nil || oldestIP.ID != "a1" {
		t.Fatalf("oldest ip attempt = %#v, err=%v; want a1", oldestIP, err)
	}
}

func TestConsumePasswordResetTokenAndResetPassword(t *testing.T) {
	repo, ctx := setupAuthDB(t)
	now := nowUTC()
	user := UserRecord{ID: "reset-user-2", Email: "reset2@example.com", PasswordHash: "oldhash", CredentialVersion: 1, Status: "active"}
	profile := UserProfileRecord{UserID: user.ID}
	if err := repo.CreateUser(ctx, user, profile); err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	if err := repo.CreateSession(ctx, UserSessionRecord{ID: "sess-2", UserID: user.ID, RefreshTokenHash: "refresh-hash", ExpiresAt: now.Add(time.Hour), CreatedAt: now}); err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	if err := repo.CreatePasswordResetToken(ctx, PasswordResetTokenRecord{ID: "token-2", UserID: user.ID, TokenHash: "hash-2", ExpiresAt: now.Add(time.Hour), CreatedAt: now}); err != nil {
		t.Fatalf("CreatePasswordResetToken failed: %v", err)
	}

	if err := repo.ConsumePasswordResetTokenAndResetPassword(ctx, "token-2", user.ID, "newhash", now); err != nil {
		t.Fatalf("ConsumePasswordResetTokenAndResetPassword failed: %v", err)
	}

	updated, err := repo.GetUserByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetUserByID failed: %v", err)
	}
	if updated.PasswordHash != "newhash" {
		t.Fatalf("password hash = %s; want newhash", updated.PasswordHash)
	}
	if updated.CredentialVersion != 2 {
		t.Fatalf("credential version = %d; want 2", updated.CredentialVersion)
	}
	session, err := repo.GetSessionByRefreshHash(ctx, "refresh-hash")
	if err != nil {
		t.Fatalf("GetSessionByRefreshHash failed: %v", err)
	}
	if session.RevokedAt == nil {
		t.Fatalf("expected session revoked")
	}
	token, err := repo.GetPasswordResetTokenByHash(ctx, "hash-2")
	if err != nil {
		t.Fatalf("GetPasswordResetTokenByHash failed: %v", err)
	}
	if token.ConsumedAt == nil {
		t.Fatalf("expected token consumed")
	}
	if err := repo.ConsumePasswordResetTokenAndResetPassword(ctx, "token-2", user.ID, "again", now.Add(time.Minute)); err != ErrResetTokenConsumed {
		t.Fatalf("expected ErrResetTokenConsumed, got %v", err)
	}
}
