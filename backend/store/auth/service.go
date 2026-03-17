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
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type ServiceConfig struct {
	JWTSecret  string
	AccessTTL  time.Duration
	RefreshTTL time.Duration
}

type AccessClaims struct {
	UserID    string `json:"uid"`
	Email     string `json:"email"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
}

type Service struct {
	repo *Repository
	cfg  ServiceConfig
}

func NewService(repo *Repository, cfg ServiceConfig) *Service {
	return &Service{repo: repo, cfg: cfg}
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
		ID:           uuid.NewString(),
		Email:        email,
		PasswordHash: string(hash),
		Status:       "active",
		CreatedAt:    now,
		UpdatedAt:    now,
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
	return s.repo.UpdatePasswordHash(ctx, userID, string(hash))
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
		UserID:    user.ID,
		Email:     user.Email,
		IssuedAt:  now.Unix(),
		ExpiresAt: expireAt.Unix(),
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
