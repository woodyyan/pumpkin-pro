package auth

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/woodyyan/pumpkin-pro/backend/tests/testutil"
)

func setupAuthService(t *testing.T) (*Service, context.Context) {
	t.Helper()
	db := testutil.InMemoryDB(t)
	testutil.AutoMigrateModels(t, db,
		UserRecord{},
		UserProfileRecord{},
		UserSessionRecord{},
		AuthAuditRecord{},
	)
	repo := NewRepository(db)
	cfg := ServiceConfig{
		JWTSecret:  "test-jwt-secret-for-unit-tests",
		AccessTTL:  24 * time.Hour,
		RefreshTTL: 168 * time.Hour,
	}
	svc := NewService(repo, cfg)
	return svc, context.Background()
}

func TestServiceRegister(t *testing.T) {
	svc, ctx := setupAuthService(t)

	result, err := svc.Register(ctx, RegisterInput{
		Email:    "register@test.com",
		Password: "securepassword123",
	}, "127.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if result.User.Email != "register@test.com" {
		t.Errorf("expected register@test.com, got %s", result.User.Email)
	}
	if result.User.Nickname != "register" {
		t.Errorf("expected nickname 'register', got %s", result.User.Nickname)
	}
	if result.Tokens.AccessToken == "" {
		t.Error("expected non-empty access token")
	}
	if result.Tokens.RefreshToken == "" {
		t.Error("expected non-empty refresh token")
	}
}

func TestServiceRegisterInvalidInput(t *testing.T) {
	svc, ctx := setupAuthService(t)

	// Too short password
	_, err := svc.Register(ctx, RegisterInput{
		Email:    "short@test.com",
		Password: "1234567",
	}, "", "")
	if err != ErrInvalidInput {
		t.Errorf("expected ErrInvalidInput for short password, got %v", err)
	}

	// Invalid email
	_, err = svc.Register(ctx, RegisterInput{
		Email:    "notanemail",
		Password: "validpassword123",
	}, "", "")
	if err != ErrInvalidInput {
		t.Errorf("expected ErrInvalidInput for invalid email, got %v", err)
	}

	// Empty fields
	_, err = svc.Register(ctx, RegisterInput{
		Email:    "",
		Password: "",
	}, "", "")
	if err != ErrInvalidInput {
		t.Errorf("expected ErrInvalidInput for empty input, got %v", err)
	}
}

func TestServiceRegisterDuplicate(t *testing.T) {
	svc, ctx := setupAuthService(t)

	_, err := svc.Register(ctx, RegisterInput{
		Email:    "duplicate@test.com",
		Password: "password123456",
	}, "", "")
	if err != nil {
		t.Fatalf("first registration should succeed: %v", err)
	}

	_, err = svc.Register(ctx, RegisterInput{
		Email:    "duplicate@test.com",
		Password: "anotherpass123",
	}, "", "")
	if err == nil {
		t.Error("expected error on duplicate registration")
	}
}

func TestServiceLogin(t *testing.T) {
	svc, ctx := setupAuthService(t)

	// Register first
	_, err := svc.Register(ctx, RegisterInput{
		Email:    "login@test.com",
		Password: "mypassword123",
	}, "", "")
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Login with correct credentials
	result, err := svc.Login(ctx, LoginInput{
		Email:    "login@test.com",
		Password: "mypassword123",
	}, "127.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	if result.User.Email != "login@test.com" {
		t.Errorf("expected login@test.com, got %s", result.User.Email)
	}
	if result.Tokens.AccessToken == "" {
		t.Error("expected non-empty access token after login")
	}
}

func TestServiceLoginWrongPassword(t *testing.T) {
	svc, ctx := setupAuthService(t)

	_, _ = svc.Register(ctx, RegisterInput{
		Email:    "wrongpwd@test.com",
		Password: "correctpassword",
	}, "", "")

	_, err := svc.Login(ctx, LoginInput{
		Email:    "wrongpwd@test.com",
		Password: "wrongpassword",
	}, "", "")
	if err != ErrInvalidCredential {
		t.Errorf("expected ErrInvalidCredential for wrong password, got %v", err)
	}
}

func TestServiceLoginNonexistent(t *testing.T) {
	svc, ctx := setupAuthService(t)

	_, err := svc.Login(ctx, LoginInput{
		Email:    "nobody@test.com",
		Password: "doesnotmatter",
	}, "", "")
	if err != ErrInvalidCredential {
		t.Errorf("expected ErrInvalidCredential for nonexistent user, got %v", err)
	}
}

func TestServiceParseAccessToken(t *testing.T) {
	svc, ctx := setupAuthService(t)

	// Register and login to get a valid token
	regResult, err := svc.Register(ctx, RegisterInput{
		Email:    "parsetest@test.com",
		Password: "tokenpass123",
	}, "", "")
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Parse valid access token
	claims, err := svc.ParseAccessToken(regResult.Tokens.AccessToken)
	if err != nil {
		t.Fatalf("ParseAccessToken failed: %v", err)
	}
	if claims.UserID != regResult.User.ID {
		t.Errorf("expected UserID %s, got %s", regResult.User.ID, claims.UserID)
	}
	if claims.Email != "parsetest@test.com" {
		t.Errorf("expected Email parsetest@test.com, got %s", claims.Email)
	}
	if claims.ExpiresAt <= claims.IssuedAt {
		t.Error("expected ExpiresAt > IssuedAt")
	}
}

func TestServiceParseInvalidTokens(t *testing.T) {
	svc, _ := setupAuthService(t)

	// Empty token
	_, err := svc.ParseAccessToken("")
	if err != ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized for empty token, got %v", err)
	}

	// Garbage token
	_, err = svc.ParseAccessToken("garbage.not-a-valid-token")
	if err != ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized for garbage token, got %v", err)
	}

	// Wrong signature: valid payload but wrong HMAC
	validPayload := `{"uid":"u1","email":"a@b.com","iat":` + formatUnix(nowUTC().Add(-time.Hour)) + `,"exp":` + formatUnix(nowUTC().Add(time.Hour)) + `}`
	wrongSig := "this-is-not-the-right-signature"
	badToken := base64Raw(validPayload) + "." + base64Raw(wrongSig)
	_, err = svc.ParseAccessToken(badToken)
	if err != ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized for wrong signature, got %v", err)
	}
}

func TestServiceChangePassword(t *testing.T) {
	svc, ctx := setupAuthService(t)

	regResult, _ := svc.Register(ctx, RegisterInput{
		Email:    "changepwd@test.com",
		Password: "oldpassword123",
	}, "", "")

	// Change with wrong current password
	err := svc.ChangePassword(ctx, regResult.User.ID, ChangePasswordInput{
		CurrentPassword: "wrongcurrent",
		NewPassword:     "newpassword123",
	})
	if err != ErrInvalidCredential {
		t.Errorf("expected ErrInvalidCredential for wrong current password, got %v", err)
	}

	// Change with too-short new password
	err = svc.ChangePassword(ctx, regResult.User.ID, ChangePasswordInput{
		CurrentPassword: "oldpassword123",
		NewPassword:     "short",
	})
	if err != ErrInvalidInput {
		t.Errorf("expected ErrInvalidInput for short new password, got %v", err)
	}

	// Successful change
	err = svc.ChangePassword(ctx, regResult.User.ID, ChangePasswordInput{
		CurrentPassword: "oldpassword123",
		NewPassword:     "newlongpassword123",
	})
	if err != nil {
		t.Fatalf("ChangePassword failed: %v", err)
	}

	// Should be able to login with new password
	loginResult, err := svc.Login(ctx, LoginInput{
		Email:    "changepwd@test.com",
		Password: "newlongpassword123",
	}, "", "")
	if err != nil {
		t.Fatalf("Login with new password failed: %v", err)
	}
	if loginResult.User.ID != regResult.User.ID {
		t.Errorf("expected same user ID, got %s vs %s", regResult.User.ID, loginResult.User.ID)
	}
}

func TestMaskEmail(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"test@example.com", "te***@example.com"},
		{"ab@example.com", "a*@example.com"},
		{"a@example.com", "a*@example.com"},
		{"", ""},
		{"invalid", "***"},
	}

	for _, tt := range tests {
		result := maskEmail(tt.input)
		if result != tt.expected {
			t.Errorf("maskEmail(%q) = %q; want %q", tt.input, result, tt.expected)
		}
	}
}

// ── Helpers ──

func formatUnix(t time.Time) string {
	return strings.Replace(t.Format("2006-01-02T15:04:05Z07:00"), "+00:00", "Z", 1)
}

func base64Raw(s string) string {
	// Simple base64url encoding without padding for test purposes only
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	var result []byte
	buf := []byte(s)
	for i := 0; i < len(buf); i += 3 {
		b0 := buf[i]
		var b1, b2 byte
		if i+1 < len(buf) {
			b1 = buf[i+1]
		}
		if i+2 < len(buf) {
			b2 = buf[i+2]
		}
		result = append(result, chars[b0>>2])
		result = append(result, chars[(b0&0x03)<<4|b1>>4])
		if i+1 < len(buf) {
			result = append(result, chars[(b1&0x0f)<<2|b2>>6])
		}
		if i+2 < len(buf) {
			result = append(result, chars[b2&0x3f])
		}
	}
	return string(result)
}
