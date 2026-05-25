package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type ServiceConfig struct {
	JWTSecret     string
	AccessTTL     time.Duration
	RefreshTTL    time.Duration
	PasswordReset PasswordResetConfig
}

type AccessClaims struct {
	UserID            string `json:"uid"`
	Email             string `json:"email"`
	CredentialVersion int    `json:"cv"`
	IssuedAt          int64  `json:"iat"`
	ExpiresAt         int64  `json:"exp"`
}

type Service struct {
	repo   *Repository
	cfg    ServiceConfig
	mailer Mailer
}

func NewService(repo *Repository, cfg ServiceConfig) *Service {
	return &Service{repo: repo, cfg: cfg}
}

func (s *Service) SetMailer(mailer Mailer) {
	s.mailer = mailer
}

func (s *Service) Register(ctx context.Context, input RegisterInput, ip string, userAgent string) (*AuthSessionResult, error) {
	email := normalizeEmail(input.Email)
	password := strings.TrimSpace(input.Password)
	if email == "" || !strings.Contains(email, "@") || len(password) < 8 {
		s.writeAudit(ctx, "register", "", email, ip, userAgent, false, "invalid input")
		return nil, ErrInvalidInput
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	user := UserRecord{
		ID:                uuid.NewString(),
		Email:             email,
		PasswordHash:      string(hash),
		CredentialVersion: 1,
		Status:            "active",
		UTMSource:         strings.TrimSpace(input.UTMSource),
		UTMMedium:         strings.TrimSpace(input.UTMMedium),
		UTMCampaign:       strings.TrimSpace(input.UTMCampaign),
		Referrer:          strings.TrimSpace(input.Referrer),
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	profile := UserProfileRecord{
		UserID:    user.ID,
		Nickname:  defaultNickname(email),
		AvatarURL: "",
		Timezone:  "Asia/Shanghai",
		UpdatedAt: now,
	}
	if err := s.repo.CreateUser(ctx, user, profile); err != nil {
		s.writeAudit(ctx, "register", "", email, ip, userAgent, false, err.Error())
		return nil, err
	}

	result, err := s.issueSession(ctx, &user, &profile)
	if err != nil {
		return nil, err
	}
	s.writeAudit(ctx, "register", user.ID, email, ip, userAgent, true, "ok")
	return result, nil
}

func (s *Service) Login(ctx context.Context, input LoginInput, ip string, userAgent string) (*AuthSessionResult, error) {
	email := normalizeEmail(input.Email)
	password := strings.TrimSpace(input.Password)
	if email == "" || password == "" {
		s.writeAudit(ctx, "login", "", email, ip, userAgent, false, "empty input")
		return nil, ErrInvalidCredential
	}

	user, err := s.repo.GetUserByEmail(ctx, email)
	if err != nil {
		s.writeAudit(ctx, "login", "", email, ip, userAgent, false, "user not found")
		return nil, ErrInvalidCredential
	}
	if user.Status != "active" {
		s.writeAudit(ctx, "login", user.ID, email, ip, userAgent, false, "user inactive")
		return nil, ErrForbidden
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		s.writeAudit(ctx, "login", user.ID, email, ip, userAgent, false, "password mismatch")
		return nil, ErrInvalidCredential
	}

	profile, err := s.repo.GetProfileByUserID(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	_ = s.repo.TouchLastLogin(ctx, user.ID)

	result, err := s.issueSession(ctx, user, profile)
	if err != nil {
		return nil, err
	}
	s.writeAudit(ctx, "login", user.ID, email, ip, userAgent, true, "ok")
	return result, nil
}

func (s *Service) Refresh(ctx context.Context, input RefreshInput, ip string, userAgent string) (*AuthSessionResult, error) {
	refreshToken := strings.TrimSpace(input.RefreshToken)
	if refreshToken == "" {
		return nil, ErrUnauthorized
	}

	hash := hashToken(refreshToken)
	session, err := s.repo.GetSessionByRefreshHash(ctx, hash)
	if err != nil {
		return nil, ErrUnauthorized
	}
	if session.RevokedAt != nil || session.ExpiresAt.Before(time.Now().UTC()) {
		return nil, ErrUnauthorized
	}

	user, err := s.repo.GetUserByID(ctx, session.UserID)
	if err != nil {
		return nil, ErrUnauthorized
	}
	profile, err := s.repo.GetProfileByUserID(ctx, session.UserID)
	if err != nil {
		return nil, ErrUnauthorized
	}

	if err := s.repo.RevokeSessionByID(ctx, session.ID); err != nil {
		return nil, err
	}

	result, err := s.issueSession(ctx, user, profile)
	if err != nil {
		return nil, err
	}
	s.writeAudit(ctx, "refresh", user.ID, user.Email, ip, userAgent, true, "ok")
	return result, nil
}

func (s *Service) Logout(ctx context.Context, userID string, input LogoutInput, ip string, userAgent string) error {
	if strings.TrimSpace(input.RefreshToken) != "" {
		hash := hashToken(strings.TrimSpace(input.RefreshToken))
		if err := s.repo.RevokeSessionByHash(ctx, hash); err != nil {
			return err
		}
	}
	s.writeAudit(ctx, "logout", userID, "", ip, userAgent, true, "ok")
	return nil
}

func (s *Service) GetProfile(ctx context.Context, userID string) (*UserProfile, error) {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	profile, err := s.repo.GetProfileByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	result := toProfile(user, profile)
	return &result, nil
}

func (s *Service) UpdateProfile(ctx context.Context, userID string, input UpdateProfileInput) (*UserProfile, error) {
	nickname := strings.TrimSpace(input.Nickname)
	avatarURL := strings.TrimSpace(input.AvatarURL)
	timezone := strings.TrimSpace(input.Timezone)
	if timezone == "" {
		timezone = "Asia/Shanghai"
	}

	if err := s.repo.UpsertProfile(ctx, UserProfileRecord{
		UserID:    userID,
		Nickname:  nickname,
		AvatarURL: avatarURL,
		Timezone:  timezone,
		UpdatedAt: time.Now().UTC(),
	}); err != nil {
		return nil, err
	}

	return s.GetProfile(ctx, userID)
}

func (s *Service) ChangePassword(ctx context.Context, userID string, input ChangePasswordInput) error {
	current := strings.TrimSpace(input.CurrentPassword)
	next := strings.TrimSpace(input.NewPassword)
	if len(next) < 8 || current == "" {
		return ErrInvalidInput
	}

	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(current)); err != nil {
		return ErrInvalidCredential
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(next), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.repo.UpdatePasswordHashAndBumpCredentialVersion(ctx, userID, string(hash))
}

func (s *Service) ForgotPassword(ctx context.Context, input ForgotPasswordInput, ip string, userAgent string) error {
	email := normalizeEmail(input.Email)
	if email == "" || !strings.Contains(email, "@") {
		s.writeAudit(ctx, "forgot_password", "", email, ip, userAgent, false, "invalid input")
		return ErrInvalidInput
	}

	if err := s.enforcePasswordResetLimits(ctx, email, ip); err != nil {
		s.writeAudit(ctx, "forgot_password", "", email, ip, userAgent, false, err.Error())
		return err
	}

	user, err := s.repo.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			_ = s.repo.CreatePasswordResetAttempt(ctx, PasswordResetAttemptRecord{
				ID:        uuid.NewString(),
				Email:     email,
				UserID:    "",
				IP:        strings.TrimSpace(ip),
				CreatedAt: nowUTC(),
			})
			s.writeAudit(ctx, "forgot_password", "", email, ip, userAgent, true, "accepted for unknown user")
			return nil
		}
		s.writeAudit(ctx, "forgot_password", "", email, ip, userAgent, false, err.Error())
		return err
	}
	if user.Status != "active" {
		s.writeAudit(ctx, "forgot_password", user.ID, email, ip, userAgent, false, "user inactive")
		return ErrForbidden
	}

	rawToken, err := generateToken(32)
	if err != nil {
		return err
	}
	now := nowUTC()
	record := PasswordResetTokenRecord{
		ID:        uuid.NewString(),
		UserID:    user.ID,
		TokenHash: hashToken(rawToken),
		ExpiresAt: now.Add(s.cfg.PasswordReset.TTL),
		IP:        strings.TrimSpace(ip),
		CreatedAt: now,
	}

	if err := s.repo.DeleteActivePasswordResetTokensByUserID(ctx, user.ID); err != nil {
		return err
	}
	if err := s.repo.CreatePasswordResetToken(ctx, record); err != nil {
		return err
	}
	if err := s.repo.CreatePasswordResetAttempt(ctx, PasswordResetAttemptRecord{
		ID:        uuid.NewString(),
		Email:     email,
		UserID:    user.ID,
		IP:        strings.TrimSpace(ip),
		CreatedAt: now,
	}); err != nil {
		_ = s.repo.DeletePasswordResetTokenByID(ctx, record.ID)
		return err
	}

	if s.mailer == nil {
		_ = s.repo.DeletePasswordResetTokenByID(ctx, record.ID)
		return fmt.Errorf("password reset mailer is not configured")
	}

	htmlBody, textBody := BuildPasswordResetMailTemplate(PasswordResetMailTemplateData{
		Token:         rawToken,
		ExpireMinutes: int(s.cfg.PasswordReset.TTL / time.Minute),
		ProductName:   "卧龙 Trader",
	})
	templateData := map[string]any{
		"PRODUCT_NAME":   "卧龙 Trader",
		"EXPIRE_MINUTES": int(s.cfg.PasswordReset.TTL / time.Minute),
		"token":          rawToken,
	}
	if err := s.mailer.Send(ctx, MailMessage{
		ToEmail:      email,
		Subject:      "重置你的卧龙 Trader 登录密码",
		HTMLBody:     htmlBody,
		TextBody:     textBody,
		TemplateData: templateData,
		Tag:          "password_reset",
		RequestID:    record.ID,
	}); err != nil {
		_ = s.repo.DeletePasswordResetTokenByID(ctx, record.ID)
		s.writeAudit(ctx, "forgot_password", user.ID, email, ip, userAgent, false, err.Error())
		return err
	}

	s.writeAudit(ctx, "forgot_password", user.ID, email, ip, userAgent, true, "ok")
	return nil
}

func (s *Service) InspectPasswordResetToken(ctx context.Context, rawToken string) (*PasswordResetTokenStatus, error) {
	token := strings.TrimSpace(rawToken)
	if token == "" {
		return &PasswordResetTokenStatus{Valid: false, Code: "TOKEN_NOT_FOUND", Detail: "重置链接无效或已失效"}, nil
	}

	record, err := s.repo.GetPasswordResetTokenByHash(ctx, hashToken(token))
	if err != nil {
		if errors.Is(err, ErrResetTokenNotFound) {
			return &PasswordResetTokenStatus{Valid: false, Code: "TOKEN_NOT_FOUND", Detail: "重置链接无效或已失效"}, nil
		}
		return nil, err
	}

	now := nowUTC()
	status := &PasswordResetTokenStatus{
		Valid:     true,
		ExpiresAt: record.ExpiresAt.Format(time.RFC3339),
	}
	if record.ConsumedAt != nil {
		status.Valid = false
		status.Code = "TOKEN_CONSUMED"
		status.Detail = "该重置链接已经使用过，请重新申请"
		status.ConsumedAt = record.ConsumedAt.Format(time.RFC3339)
		return status, nil
	}
	if !record.ExpiresAt.After(now) {
		status.Valid = false
		status.Code = "TOKEN_EXPIRED"
		status.Detail = "该重置链接已过期，请重新申请"
	}
	return status, nil
}

func (s *Service) ResetPassword(ctx context.Context, input ResetPasswordInput, ip string, userAgent string) error {
	token := strings.TrimSpace(input.Token)
	next := strings.TrimSpace(input.NewPassword)
	if token == "" || len(next) < 8 {
		s.writeAudit(ctx, "reset_password", "", "", ip, userAgent, false, "invalid input")
		return ErrInvalidInput
	}

	record, err := s.repo.GetPasswordResetTokenByHash(ctx, hashToken(token))
	if err != nil {
		s.writeAudit(ctx, "reset_password", "", "", ip, userAgent, false, err.Error())
		return err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(next), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	if err := s.repo.ConsumePasswordResetTokenAndResetPassword(ctx, record.ID, record.UserID, string(hash), nowUTC()); err != nil {
		s.writeAudit(ctx, "reset_password", record.UserID, "", ip, userAgent, false, err.Error())
		return err
	}

	s.writeAudit(ctx, "reset_password", record.UserID, "", ip, userAgent, true, "ok")
	return nil
}

func (s *Service) ParseAccessToken(raw string) (*AccessClaims, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return nil, ErrUnauthorized
	}
	parts := strings.Split(text, ".")
	if len(parts) != 2 {
		return nil, ErrUnauthorized
	}
	payloadPart := strings.TrimSpace(parts[0])
	signaturePart := strings.TrimSpace(parts[1])
	if payloadPart == "" || signaturePart == "" {
		return nil, ErrUnauthorized
	}

	expectedSig := signAccessPayload(payloadPart, s.cfg.JWTSecret)
	if !hmac.Equal([]byte(signaturePart), []byte(expectedSig)) {
		return nil, ErrUnauthorized
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(payloadPart)
	if err != nil {
		return nil, ErrUnauthorized
	}

	claims := &AccessClaims{}
	if err := json.Unmarshal(payloadBytes, claims); err != nil {
		return nil, ErrUnauthorized
	}
	if claims.UserID == "" || claims.ExpiresAt <= time.Now().UTC().Unix() {
		return nil, ErrUnauthorized
	}
	user, err := s.repo.GetUserByID(context.Background(), claims.UserID)
	if err != nil {
		return nil, ErrUnauthorized
	}
	if claims.CredentialVersion > 0 && user.CredentialVersion != claims.CredentialVersion {
		return nil, ErrUnauthorized
	}
	return claims, nil
}

func (s *Service) issueSession(ctx context.Context, user *UserRecord, profile *UserProfileRecord) (*AuthSessionResult, error) {
	accessToken, expiresIn, err := s.buildAccessToken(user)
	if err != nil {
		return nil, err
	}

	refreshToken, err := generateToken(48)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	session := UserSessionRecord{
		ID:               uuid.NewString(),
		UserID:           user.ID,
		RefreshTokenHash: hashToken(refreshToken),
		ExpiresAt:        now.Add(s.cfg.RefreshTTL),
		CreatedAt:        now,
	}
	if err := s.repo.CreateSession(ctx, session); err != nil {
		return nil, err
	}

	result := &AuthSessionResult{
		User: toProfile(user, profile),
		Tokens: AuthTokens{
			AccessToken:  accessToken,
			RefreshToken: refreshToken,
			ExpiresIn:    int64(expiresIn.Seconds()),
			TokenType:    "Bearer",
		},
	}
	return result, nil
}

func (s *Service) buildAccessToken(user *UserRecord) (string, time.Duration, error) {
	now := time.Now().UTC()
	expireAt := now.Add(s.cfg.AccessTTL)
	claims := AccessClaims{
		UserID:            user.ID,
		Email:             user.Email,
		CredentialVersion: user.CredentialVersion,
		IssuedAt:          now.Unix(),
		ExpiresAt:         expireAt.Unix(),
	}
	payloadBytes, err := json.Marshal(claims)
	if err != nil {
		return "", 0, err
	}
	payloadPart := base64.RawURLEncoding.EncodeToString(payloadBytes)
	signaturePart := signAccessPayload(payloadPart, s.cfg.JWTSecret)
	return payloadPart + "." + signaturePart, s.cfg.AccessTTL, nil
}

func toProfile(user *UserRecord, profile *UserProfileRecord) UserProfile {
	return UserProfile{
		ID:        user.ID,
		Email:     user.Email,
		Nickname:  profile.Nickname,
		AvatarURL: profile.AvatarURL,
		Timezone:  profile.Timezone,
	}
}

func (s *Service) writeAudit(ctx context.Context, action, userID, email, ip, userAgent string, success bool, message string) {
	s.repo.InsertAudit(ctx, AuthAuditRecord{
		ID:          uuid.NewString(),
		UserID:      strings.TrimSpace(userID),
		EmailMasked: maskEmail(email),
		Action:      action,
		IP:          strings.TrimSpace(ip),
		UserAgent:   strings.TrimSpace(userAgent),
		Success:     success,
		Message:     strings.TrimSpace(message),
		CreatedAt:   time.Now().UTC(),
	})
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func defaultNickname(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) > 0 && strings.TrimSpace(parts[0]) != "" {
		return parts[0]
	}
	return "用户"
}

func maskEmail(email string) string {
	email = normalizeEmail(email)
	if email == "" {
		return ""
	}
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return "***"
	}
	name := parts[0]
	domain := parts[1]
	if len(name) <= 2 {
		name = name[:1] + "*"
	} else {
		name = name[:2] + "***"
	}
	return name + "@" + domain
}

func (s *Service) enforcePasswordResetLimits(ctx context.Context, email string, ip string) error {
	now := nowUTC()
	if cooldown := s.cfg.PasswordReset.EmailCooldown; cooldown > 0 {
		latest, err := s.repo.GetLatestPasswordResetAttemptByEmail(ctx, email)
		if err != nil {
			return err
		}
		if latest != nil {
			retryAfter := latest.CreatedAt.Add(cooldown).Sub(now)
			if retryAfter > 0 {
				return &RateLimitError{RetryAfterSeconds: durationSecondsCeil(retryAfter)}
			}
		}
	}

	windowStart := now.Add(-s.cfg.PasswordReset.RateLimitWindow)
	if limit := s.cfg.PasswordReset.RateLimitPerEmail; limit > 0 {
		count, err := s.repo.CountPasswordResetAttemptsByEmailSince(ctx, email, windowStart)
		if err != nil {
			return err
		}
		if int(count) >= limit {
			oldest, err := s.repo.GetOldestPasswordResetAttemptByEmailSince(ctx, email, windowStart)
			if err != nil {
				return err
			}
			return &RateLimitError{RetryAfterSeconds: retryAfterFromOldest(oldest, s.cfg.PasswordReset.RateLimitWindow, now)}
		}
	}

	trimmedIP := strings.TrimSpace(ip)
	if limit := s.cfg.PasswordReset.RateLimitPerIP; limit > 0 && trimmedIP != "" {
		count, err := s.repo.CountPasswordResetAttemptsByIPSince(ctx, trimmedIP, windowStart)
		if err != nil {
			return err
		}
		if int(count) >= limit {
			oldest, err := s.repo.GetOldestPasswordResetAttemptByIPSince(ctx, trimmedIP, windowStart)
			if err != nil {
				return err
			}
			return &RateLimitError{RetryAfterSeconds: retryAfterFromOldest(oldest, s.cfg.PasswordReset.RateLimitWindow, now)}
		}
	}

	return nil
}

func (s *Service) buildPasswordResetURL(rawToken string) string {
	base := strings.TrimRight(strings.TrimSpace(s.cfg.PasswordReset.PublicBaseURL), "/")
	if base == "" {
		base = "https://wolongtrader.top"
	}
	return base + "/reset-password?token=" + rawToken
}

func retryAfterFromOldest(record *PasswordResetAttemptRecord, window time.Duration, now time.Time) int {
	if record == nil || window <= 0 {
		return 1
	}
	return durationSecondsCeil(record.CreatedAt.Add(window).Sub(now))
}

func durationSecondsCeil(d time.Duration) int {
	if d <= 0 {
		return 1
	}
	seconds := int(d / time.Second)
	if d%time.Second != 0 {
		seconds++
	}
	if seconds <= 0 {
		return 1
	}
	return seconds
}

func signAccessPayload(payloadPart string, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payloadPart))
	sig := mac.Sum(nil)
	return base64.RawURLEncoding.EncodeToString(sig)
}

func hashToken(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

func generateToken(length int) (string, error) {
	if length <= 0 {
		return "", errors.New("invalid token length")
	}
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
