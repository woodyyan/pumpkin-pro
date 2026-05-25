package auth

import (
	"context"
	"errors"
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
		PasswordResetTokenRecord{},
		PasswordResetAttemptRecord{},
		UserSessionRecord{},
		AuthAuditRecord{},
	)
	repo := NewRepository(db)
	cfg := ServiceConfig{
		JWTSecret:  "test-jwt-secret-for-unit-tests",
		AccessTTL:  24 * time.Hour,
		RefreshTTL: 168 * time.Hour,
		PasswordReset: PasswordResetConfig{
			PublicBaseURL:     "https://wolongtrader.top",
			TTL:               30 * time.Minute,
			RateLimitWindow:   time.Hour,
			RateLimitPerIP:    10,
			RateLimitPerEmail: 3,
			EmailCooldown:     time.Minute,
		},
	}
	svc := NewService(repo, cfg)
	svc.SetMailer(&stubMailer{})
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

type stubMailer struct {
	messages []MailMessage
	fail     error
}

func (m *stubMailer) Send(_ context.Context, message MailMessage) error {
	if m.fail != nil {
		return m.fail
	}
	m.messages = append(m.messages, message)
	return nil
}

func TestServiceForgotPasswordAndReset(t *testing.T) {
	mailer := &stubMailer{}
	svc, ctx := setupAuthService(t)
	svc.SetMailer(mailer)
	result, err := svc.Register(ctx, RegisterInput{Email: "reset-flow@test.com", Password: "password12345"}, "", "")
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	if err := svc.ForgotPassword(ctx, ForgotPasswordInput{Email: "reset-flow@test.com"}, "127.0.0.1", "ua"); err != nil {
		t.Fatalf("ForgotPassword failed: %v", err)
	}
	if len(mailer.messages) != 1 {
		t.Fatalf("mail messages = %d; want 1", len(mailer.messages))
	}
	message := mailer.messages[0]
	if !strings.Contains(message.TextBody, "/reset-password?token=") {
		t.Fatalf("mail text body missing reset token link")
	}
	start := strings.LastIndex(message.TextBody, "https://wolongtrader.top/reset-password?token=")
	if start < 0 {
		t.Fatalf("reset URL not found in text body")
	}
	rawToken := strings.TrimSpace(message.TextBody[start:])
	if idx := strings.Index(rawToken, "\n"); idx >= 0 {
		rawToken = rawToken[:idx]
	}
	rawToken = strings.TrimPrefix(rawToken, "https://wolongtrader.top/reset-password?token=")

	status, err := svc.InspectPasswordResetToken(ctx, rawToken)
	if err != nil {
		t.Fatalf("InspectPasswordResetToken failed: %v", err)
	}
	if !status.Valid {
		t.Fatalf("token should be valid before reset: %#v", status)
	}

	if err := svc.ResetPassword(ctx, ResetPasswordInput{Token: rawToken, NewPassword: "newpassword123"}, "127.0.0.1", "ua"); err != nil {
		t.Fatalf("ResetPassword failed: %v", err)
	}
	if _, err := svc.Login(ctx, LoginInput{Email: "reset-flow@test.com", Password: "newpassword123"}, "", ""); err != nil {
		t.Fatalf("login with reset password failed: %v", err)
	}
	status, err = svc.InspectPasswordResetToken(ctx, rawToken)
	if err != nil {
		t.Fatalf("InspectPasswordResetToken after reset failed: %v", err)
	}
	if status.Valid || status.Code != "TOKEN_CONSUMED" {
		t.Fatalf("expected consumed token status, got %#v", status)
	}
	if _, err := svc.ParseAccessToken(result.Tokens.AccessToken); err != ErrUnauthorized {
		t.Fatalf("expected old access token invalid after reset, got %v", err)
	}
}

func TestServiceForgotPasswordRateLimited(t *testing.T) {
	svc, ctx := setupAuthService(t)
	svc.SetMailer(&stubMailer{})
	_, err := svc.Register(ctx, RegisterInput{Email: "limit@test.com", Password: "password12345"}, "", "")
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if err := svc.ForgotPassword(ctx, ForgotPasswordInput{Email: "limit@test.com"}, "127.0.0.1", "ua"); err != nil {
		t.Fatalf("first ForgotPassword failed: %v", err)
	}
	err = svc.ForgotPassword(ctx, ForgotPasswordInput{Email: "limit@test.com"}, "127.0.0.1", "ua")
	var rateErr *RateLimitError
	if !errors.As(err, &rateErr) {
		t.Fatalf("expected RateLimitError, got %v", err)
	}
	if rateErr.RetryAfter() <= 0 {
		t.Fatalf("expected positive retry after")
	}
}

func TestServiceForgotPasswordUnknownEmailIsSilent(t *testing.T) {
	svc, ctx := setupAuthService(t)
	svc.SetMailer(&stubMailer{})
	if err := svc.ForgotPassword(ctx, ForgotPasswordInput{Email: "missing@test.com"}, "127.0.0.1", "ua"); err != nil {
		t.Fatalf("ForgotPassword should not fail for unknown email: %v", err)
	}
}

func TestBuildPasswordResetMailTemplate(t *testing.T) {
	htmlBody, textBody := BuildPasswordResetMailTemplate(PasswordResetMailTemplateData{
		ResetURL:      "https://wolongtrader.top/reset-password?token=abc",
		ExpireMinutes: 30,
		ProductName:   "卧龙 Trader",
	})
	if !strings.Contains(htmlBody, `charset="UTF-8"`) {
		t.Fatalf("html template must declare utf-8")
	}
	if !strings.Contains(htmlBody, "https://wolongtrader.top/reset-password?token=abc") {
		t.Fatalf("html template missing reset url")
	}
	if !strings.Contains(textBody, "https://wolongtrader.top/reset-password?token=abc") {
		t.Fatalf("text template missing reset url")
	}
	if len([]byte(htmlBody)) >= 400*1024 {
		t.Fatalf("html template exceeds 400KB")
	}
}
