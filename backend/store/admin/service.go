package admin

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type ServiceConfig struct {
	JWTSecret string
	AccessTTL time.Duration
}

type Service struct {
	repo *Repository
	cfg  ServiceConfig
}

func NewService(repo *Repository, cfg ServiceConfig) *Service {
	return &Service{repo: repo, cfg: cfg}
}

// ── Seed ──

func (s *Service) SeedAdmin(ctx context.Context, email, password string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	password = strings.TrimSpace(password)
	if email == "" || password == "" {
		return nil
	}

	exists, err := s.repo.AdminExists(ctx)
	if err != nil {
		return fmt.Errorf("check admin exists: %w", err)
	}
	if exists {
		log.Println("[admin] super admin already exists, skip seeding")
		return nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	now := time.Now().UTC()
	record := SuperAdminRecord{
		ID:           uuid.NewString(),
		Email:        email,
		PasswordHash: string(hash),
		Nickname:     "超级管理员",
		Status:       "active",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.repo.CreateAdmin(ctx, record); err != nil {
		return fmt.Errorf("create admin: %w", err)
	}

	log.Printf("[admin] seeded super admin: %s", email)
	return nil
}

// ── Login ──

func (s *Service) Login(ctx context.Context, input AdminLoginInput) (*AdminLoginResult, error) {
	email := strings.ToLower(strings.TrimSpace(input.Email))
	password := strings.TrimSpace(input.Password)
	if email == "" || password == "" {
		return nil, ErrInvalidInput
	}

	record, err := s.repo.GetAdminByEmail(ctx, email)
	if err != nil {
		return nil, ErrInvalidCredential
	}
	if record.Status != "active" {
		return nil, ErrForbidden
	}
	if err := bcrypt.CompareHashAndPassword([]byte(record.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCredential
	}

	token, expiresIn, err := s.buildAccessToken(record)
	if err != nil {
		return nil, fmt.Errorf("build access token: %w", err)
	}

	return &AdminLoginResult{
		Admin: AdminProfile{
			ID:       record.ID,
			Email:    record.Email,
			Nickname: record.Nickname,
		},
		Tokens: AdminTokens{
			AccessToken: token,
			ExpiresIn:   int64(expiresIn.Seconds()),
			TokenType:   "Bearer",
		},
	}, nil
}

// ── Token ──

func (s *Service) buildAccessToken(record *SuperAdminRecord) (string, time.Duration, error) {
	now := time.Now().UTC()
	expireAt := now.Add(s.cfg.AccessTTL)
	claims := AdminAccessClaims{
		AdminID:   record.ID,
		Email:     record.Email,
		Role:      "super_admin",
		IssuedAt:  now.Unix(),
		ExpiresAt: expireAt.Unix(),
	}
	payloadBytes, err := json.Marshal(claims)
	if err != nil {
		return "", 0, err
	}
	payloadPart := base64.RawURLEncoding.EncodeToString(payloadBytes)
	signaturePart := signPayload(payloadPart, s.cfg.JWTSecret)
	return payloadPart + "." + signaturePart, s.cfg.AccessTTL, nil
}

func (s *Service) ParseAdminToken(raw string) (*AdminAccessClaims, error) {
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

	expectedSig := signPayload(payloadPart, s.cfg.JWTSecret)
	if !hmac.Equal([]byte(signaturePart), []byte(expectedSig)) {
		return nil, ErrUnauthorized
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(payloadPart)
	if err != nil {
		return nil, ErrUnauthorized
	}

	claims := &AdminAccessClaims{}
	if err := json.Unmarshal(payloadBytes, claims); err != nil {
		return nil, ErrUnauthorized
	}
	if claims.AdminID == "" || claims.Role != "super_admin" || claims.ExpiresAt <= time.Now().UTC().Unix() {
		return nil, ErrUnauthorized
	}
	return claims, nil
}

// ── Stats ──

func (s *Service) GetStats(ctx context.Context) (*StatsResult, error) {
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	sevenDaysAgo := now.AddDate(0, 0, -7)
	thirtyDaysAgo := now.AddDate(0, 0, -30)

	// Users
	usersTotal, _ := s.repo.CountUsers(ctx)
	usersToday, _ := s.repo.CountUsersSince(ctx, today)
	users7D, _ := s.repo.CountUsersSince(ctx, sevenDaysAgo)
	users30D, _ := s.repo.CountUsersSince(ctx, thirtyDaysAgo)
	active7D, _ := s.repo.CountActiveUsers7D(ctx)
	activeSessions, _ := s.repo.CountActiveSessions(ctx)

	// Strategies
	stratTotal, stratSystem, stratUser, stratActive, _ := s.repo.CountStrategies(ctx)
	stratReferenced, _ := s.repo.CountReferencedStrategies(ctx)

	// Live
	watchlistItems, _ := s.repo.CountWatchlistItems(ctx)
	usersWithWatchlist, _ := s.repo.CountUsersWithWatchlist(ctx)
	activeSymbols, _ := s.repo.CountActiveSymbols(ctx)

	// Signals
	webhookUsers, _ := s.repo.CountWebhookUsers(ctx)
	webhookEnabled, _ := s.repo.CountWebhookEnabled(ctx)
	signalConfigsTotal, signalConfigsEnabled, _ := s.repo.CountSignalConfigs(ctx)
	totalEvents, _ := s.repo.CountSignalEvents(ctx)
	todayEvents, _ := s.repo.CountSignalEventsSince(ctx, today)
	deliveryTotal, deliveryDelivered, _ := s.repo.CountDeliveries(ctx)
	todayDeliveries, _ := s.repo.CountDeliveriesSince(ctx, today)

	webhookEnabledRate := float64(0)
	if webhookUsers > 0 {
		webhookEnabledRate = float64(webhookEnabled) / float64(webhookUsers)
	}
	deliverySuccessRate := float64(0)
	if deliveryTotal > 0 {
		deliverySuccessRate = float64(deliveryDelivered) / float64(deliveryTotal)
	}

	// Audit
	todayLogins, _ := s.repo.CountAuditLogins(ctx, today)
	todayRegistrations, _ := s.repo.CountAuditRegistrations(ctx, today)
	failedLogins7D, _ := s.repo.CountFailedLogins(ctx, sevenDaysAgo)

	return &StatsResult{
		Users: UserStats{
			Total:          usersTotal,
			Today:          usersToday,
			Last7D:         users7D,
			Last30D:        users30D,
			Active7D:       active7D,
			ActiveSessions: activeSessions,
		},
		Strategies: StrategyStats{
			Total:       stratTotal,
			System:      stratSystem,
			UserCreated: stratUser,
			Active:      stratActive,
			Referenced:  stratReferenced,
		},
		Live: LiveStats{
			WatchlistItems:     watchlistItems,
			UsersWithWatchlist: usersWithWatchlist,
			ActiveSymbols:      activeSymbols,
		},
		Signals: SignalStats{
			WebhookUsers:         webhookUsers,
			WebhookEnabledRate:   webhookEnabledRate,
			SignalConfigs:        signalConfigsTotal,
			SignalConfigsEnabled: signalConfigsEnabled,
			TotalEvents:          totalEvents,
			TodayEvents:          todayEvents,
			DeliverySuccessRate:  deliverySuccessRate,
			TodayDeliveries:      todayDeliveries,
		},
		Audit: AuditStats{
			TodayLogins:        todayLogins,
			TodayRegistrations: todayRegistrations,
			FailedLogins7D:     failedLogins7D,
		},
		Features:    s.collectFeatureStats(ctx, today),
		Trends:      s.collectTrendStats(ctx),
		Retention:   s.collectRetentionStats(ctx),
		GeneratedAt: now.Format(time.RFC3339),
	}, nil
}

func (s *Service) collectFeatureStats(ctx context.Context, today time.Time) FeatureStats {
	btTotal, _ := s.repo.CountBacktestRuns(ctx)
	btToday, _ := s.repo.CountBacktestRunsSince(ctx, today)
	btUsers, _ := s.repo.CountBacktestUsers(ctx)
	pfRecords, _ := s.repo.CountPortfolioRecords(ctx)
	pfUsers, _ := s.repo.CountPortfolioUsers(ctx)
	scLists, _ := s.repo.CountScreenerWatchlists(ctx)
	scUsers, _ := s.repo.CountScreenerUsers(ctx)
	return FeatureStats{
		BacktestTotal:    btTotal,
		BacktestToday:    btToday,
		BacktestUsers:    btUsers,
		PortfolioRecords: pfRecords,
		PortfolioUsers:   pfUsers,
		ScreenerLists:    scLists,
		ScreenerUsers:    scUsers,
	}
}

func (s *Service) collectTrendStats(ctx context.Context) TrendStats {
	regTrend, _ := s.repo.DailyRegistrations(ctx, 30)
	dauTrend, _ := s.repo.DailyActiveUsers(ctx, 30)
	sigTrend, _ := s.repo.DailySignalEvents(ctx, 30)
	if regTrend == nil { regTrend = []DailyCount{} }
	if dauTrend == nil { dauTrend = []DailyCount{} }
	if sigTrend == nil { sigTrend = []DailyCount{} }
	return TrendStats{
		DailyRegistrations: regTrend,
		DailyActiveUsers:   dauTrend,
		DailySignalEvents:  sigTrend,
	}
}

func (s *Service) collectRetentionStats(ctx context.Context) RetentionStats {
	now := time.Now().UTC()
	sevenDaysAgo := now.AddDate(0, 0, -7)
	thirtyDaysAgo := now.AddDate(0, 0, -30)

	reg7, ret7, _ := s.repo.RetentionRate(ctx, sevenDaysAgo, 7)
	reg30, ret30, _ := s.repo.RetentionRate(ctx, thirtyDaysAgo, 30)

	rate7 := float64(0)
	if reg7 > 0 { rate7 = float64(ret7) / float64(reg7) }
	rate30 := float64(0)
	if reg30 > 0 { rate30 = float64(ret30) / float64(reg30) }

	return RetentionStats{Day7Rate: rate7, Day30Rate: rate30}
}

// ── Helpers ──

func signPayload(payloadPart string, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payloadPart))
	sig := mac.Sum(nil)
	return base64.RawURLEncoding.EncodeToString(sig)
}
