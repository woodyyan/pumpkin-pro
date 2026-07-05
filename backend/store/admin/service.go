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
	"github.com/woodyyan/pumpkin-pro/backend/config"
	"golang.org/x/crypto/bcrypt"
)

type ServiceConfig struct {
	JWTSecret string
	AccessTTL time.Duration
	EnvAI     config.AIConfig
}

type Service struct {
	repo     *Repository
	cfg      ServiceConfig
	aiTester aiProviderTester
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
		ExpiresIn:   int64(expiresIn.Seconds()),
		AccessToken: token,
	}, nil
}

// ── Token ──

func (s *Service) buildAccessToken(record *SuperAdminRecord) (string, time.Duration, error) {
	now := time.Now().UTC()
	expireAt := now.Add(s.cfg.AccessTTL)
	claims := AdminAccessClaims{
		AdminID:   record.ID,
		Email:     record.Email,
		Nickname:  record.Nickname,
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

// ── Insert API Error (called by logging middleware) ──

func (s *Service) InsertAPIError(ctx context.Context, record APIErrorRecord) error {
	return s.repo.InsertAPIError(ctx, record)
}

// ── API Error Log Access (for admin panel) ──

func (s *Service) ListAPIErrors(ctx context.Context, limit, offset int) ([]APIErrorRecord, int64, error) {
	return s.repo.ListAPIErrors(ctx, limit, offset)
}

func (s *Service) PurgeOldAPIErrors(ctx context.Context, days int) (int64, error) {
	return s.repo.PurgeOldAPIErrors(ctx, days)
}

// ── Funnel ──

func (s *Service) GetFunnelStats(ctx context.Context) (*FunnelStats, error) {
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	sevenDaysAgo := now.AddDate(0, 0, -7)
	thirtyDaysAgo := now.AddDate(0, 0, -30)

	safeNoArg := func(fn func(context.Context) (int64, error)) int64 {
		v, err := fn(ctx)
		if err != nil {
			return 0
		}
		return v
	}
	since := func(fn func(context.Context, time.Time) (int64, error), t time.Time) int64 {
		v, err := fn(ctx, t)
		if err != nil {
			return 0
		}
		return v
	}

	steps := []FunnelStep{
		{
			Label:      "访客",
			CountAll:   since(s.repo.CountUVSince, time.Time{}),
			CountToday: since(s.repo.CountUVSince, today),
			Count7D:    since(s.repo.CountUVSince, sevenDaysAgo),
			Count30D:   since(s.repo.CountUVSince, thirtyDaysAgo),
		},
		{
			Label:      "注册",
			CountAll:   safeNoArg(s.repo.CountUsers),
			CountToday: since(s.repo.CountUsersSince, today),
			Count7D:    since(s.repo.CountUsersSince, sevenDaysAgo),
			Count30D:   since(s.repo.CountUsersSince, thirtyDaysAgo),
		},
		{
			Label:      "登录",
			CountAll:   since(s.repo.CountUniqueLoginsSince, time.Time{}),
			CountToday: since(s.repo.CountUniqueLoginsSince, today),
			Count7D:    since(s.repo.CountUniqueLoginsSince, sevenDaysAgo),
			Count30D:   since(s.repo.CountUniqueLoginsSince, thirtyDaysAgo),
		},
		{
			Label:      "加关注池",
			CountAll:   safeNoArg(s.repo.CountUsersWithWatchlist),
			CountToday: since(s.repo.CountUsersWithWatchlistSince, today),
			Count7D:    since(s.repo.CountUsersWithWatchlistSince, sevenDaysAgo),
			Count30D:   since(s.repo.CountUsersWithWatchlistSince, thirtyDaysAgo),
		},
		{
			Label:      "持仓管理",
			CountAll:   safeNoArg(s.repo.CountPortfolioEventUsers),
			CountToday: since(s.repo.CountPortfolioEventUsersSince, today),
			Count7D:    since(s.repo.CountPortfolioEventUsersSince, sevenDaysAgo),
			Count30D:   since(s.repo.CountPortfolioEventUsersSince, thirtyDaysAgo),
		},
		{
			Label:      "配置信号",
			CountAll:   since(s.repo.CountUsersWithSignalConfigsSince, time.Time{}),
			CountToday: since(s.repo.CountUsersWithSignalConfigsSince, today),
			Count7D:    since(s.repo.CountUsersWithSignalConfigsSince, sevenDaysAgo),
			Count30D:   since(s.repo.CountUsersWithSignalConfigsSince, thirtyDaysAgo),
		},
		{
			Label:      "跑回测",
			CountAll:   safeNoArg(s.repo.CountBacktestUsers),
			CountToday: since(s.repo.CountBacktestUsersSince, today),
			Count7D:    since(s.repo.CountBacktestUsersSince, sevenDaysAgo),
			Count30D:   since(s.repo.CountBacktestUsersSince, thirtyDaysAgo),
		},
		{
			Label:      "用 AI",
			CountAll:   safeNoArg(s.repo.AIUniqueUsers),
			CountToday: since(s.repo.CountAIUniqueUsersSince, today),
			Count7D:    since(s.repo.CountAIUniqueUsersSince, sevenDaysAgo),
			Count30D:   since(s.repo.CountAIUniqueUsersSince, thirtyDaysAgo),
		},
	}

	return &FunnelStats{
		Steps:       steps,
		GeneratedAt: now.Format(time.RFC3339),
	}, nil
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
		Features:    s.collectFeatureStats(ctx, today, sevenDaysAgo),
		Trends:      s.collectTrendStats(ctx),
		Retention:   s.collectRetentionStats(ctx),
		Traffic:     s.collectTrafficStats(ctx),
		AI:          s.collectAIStats(ctx, today, sevenDaysAgo),
		GeneratedAt: now.Format(time.RFC3339),
	}, nil
}

func (s *Service) collectFeatureStats(ctx context.Context, today, sevenDaysAgo time.Time) FeatureStats {
	btTotal, _ := s.repo.CountBacktestRuns(ctx)
	btToday, _ := s.repo.CountBacktestRunsSince(ctx, today)
	btUsers, _ := s.repo.CountBacktestUsers(ctx)
	pfRecords, _ := s.repo.CountPortfolioRecords(ctx)
	pfUsers, _ := s.repo.CountPortfolioUsers(ctx)
	pfActivePositions, _ := s.repo.CountActivePortfolioRecords(ctx)
	pfActiveUsers, _ := s.repo.CountActivePortfolioUsers(ctx)
	pfEventTotal, _ := s.repo.CountPortfolioEvents(ctx)
	pfEventToday, _ := s.repo.CountPortfolioEventsSince(ctx, today)
	pfEventUsers7D, _ := s.repo.CountPortfolioEventUsersSince(ctx, sevenDaysAgo)
	pfProfileUsers, _ := s.repo.CountPortfolioProfileUsers(ctx)
	scLists, _ := s.repo.CountScreenerWatchlists(ctx)
	scUsers, _ := s.repo.CountScreenerUsers(ctx)
	return FeatureStats{
		BacktestTotal:            btTotal,
		BacktestToday:            btToday,
		BacktestUsers:            btUsers,
		PortfolioRecords:         pfRecords,
		PortfolioUsers:           pfUsers,
		PortfolioActivePositions: pfActivePositions,
		PortfolioActiveUsers:     pfActiveUsers,
		PortfolioEventTotal:      pfEventTotal,
		PortfolioEventToday:      pfEventToday,
		PortfolioEventUsers7D:    pfEventUsers7D,
		PortfolioProfileUsers:    pfProfileUsers,
		ScreenerLists:            scLists,
		ScreenerUsers:            scUsers,
	}
}

func (s *Service) collectTrendStats(ctx context.Context) TrendStats {
	regTrend, _ := s.repo.DailyRegistrations(ctx, 30)
	dauTrend, _ := s.repo.DailyActiveUsers(ctx, 30)
	sigTrend, _ := s.repo.DailySignalEvents(ctx, 30)
	pfTrend, _ := s.repo.DailyPortfolioEvents(ctx, 30)
	if regTrend == nil {
		regTrend = []DailyCount{}
	}
	if dauTrend == nil {
		dauTrend = []DailyCount{}
	}
	if sigTrend == nil {
		sigTrend = []DailyCount{}
	}
	if pfTrend == nil {
		pfTrend = []DailyCount{}
	}
	return TrendStats{
		DailyRegistrations: regTrend,
		DailyActiveUsers:   dauTrend,
		DailySignalEvents:  sigTrend,
		DailyPortfolioOps:  pfTrend,
	}
}

func (s *Service) collectRetentionStats(ctx context.Context) RetentionStats {
	now := time.Now().UTC()
	sevenDaysAgo := now.AddDate(0, 0, -7)
	thirtyDaysAgo := now.AddDate(0, 0, -30)

	reg7, ret7, _ := s.repo.RetentionRate(ctx, sevenDaysAgo, 7)
	reg30, ret30, _ := s.repo.RetentionRate(ctx, thirtyDaysAgo, 30)

	rate7 := float64(0)
	if reg7 > 0 {
		rate7 = float64(ret7) / float64(reg7)
	}
	rate30 := float64(0)
	if reg30 > 0 {
		rate30 = float64(ret30) / float64(reg30)
	}

	return RetentionStats{Day7Rate: rate7, Day30Rate: rate30}
}

func (s *Service) collectTrafficStats(ctx context.Context) TrafficStats {
	utmSources, _ := s.repo.UTMSourceBreakdown(ctx)
	if utmSources == nil {
		utmSources = []SourceCount{}
	}

	thirtyDaysAgo := time.Now().UTC().AddDate(0, 0, -30)
	referrers, _ := s.repo.ReferrerBreakdown(ctx, thirtyDaysAgo)
	if referrers == nil {
		referrers = []SourceCount{}
	}

	return TrafficStats{
		UTMSources: utmSources,
		Referrers:  referrers,
	}
}

func (s *Service) GetAITokenUsage(ctx context.Context, days, limit int) (*AITokenUsageResult, error) {
	if days <= 0 {
		days = 30
	}
	if limit <= 0 {
		limit = 120
	}

	since := time.Now().UTC().AddDate(0, 0, -days)
	summary, err := s.repo.AISumUsage(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("query ai token summary: %w", err)
	}
	dailyUsers, err := s.repo.AIDailyUserTokenUsage(ctx, days, limit)
	if err != nil {
		return nil, fmt.Errorf("query ai daily user token usage: %w", err)
	}
	if dailyUsers == nil {
		dailyUsers = []DailyUserTokenUsage{}
	}

	return &AITokenUsageResult{
		Days:        days,
		Summary:     summary,
		DailyUsers:  dailyUsers,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// ── Helpers ──

func (s *Service) collectAIStats(ctx context.Context, today, sevenDaysAgo time.Time) AIStats {
	totalCalls, _ := s.repo.AITotalCalls(ctx)
	todayCalls, _ := s.repo.AICallsSince(ctx, today)
	calls7D, _ := s.repo.AICallsSince(ctx, sevenDaysAgo)
	successRate, _ := s.repo.AISuccessRate(ctx)
	avgMS, _ := s.repo.AIAvgResponseMS(ctx)
	uniqueUsers, _ := s.repo.AIUniqueUsers(ctx)
	byFeature, _ := s.repo.AIByFeatureBreakdown(ctx)
	byFeatureTokens, _ := s.repo.AIByFeatureTokenBreakdown(ctx)
	dailyTrend, _ := s.repo.AIDailyTrend(ctx, 30)
	dailyTokenTrend, _ := s.repo.AIDailyTokenTrend(ctx, 30)
	topUsers, _ := s.repo.AITopUsers(ctx, 10)
	topTokenUsers, _ := s.repo.AITopUsersByTokens(ctx, 10)
	totalUsage, _ := s.repo.AISumUsage(ctx, time.Time{})
	todayUsage, _ := s.repo.AISumUsage(ctx, today)
	usage7D, _ := s.repo.AISumUsage(ctx, sevenDaysAgo)

	if byFeature == nil {
		byFeature = []FeatureCount{}
	}
	if byFeatureTokens == nil {
		byFeatureTokens = []FeatureTokenCount{}
	}
	if dailyTrend == nil {
		dailyTrend = []DailyCount{}
	}
	if dailyTokenTrend == nil {
		dailyTokenTrend = []DailyCount{}
	}
	if topUsers == nil {
		topUsers = []TopAIUser{}
	}
	if topTokenUsers == nil {
		topTokenUsers = []TopAIUser{}
	}

	return AIStats{
		TotalCalls:       totalCalls,
		TodayCalls:       todayCalls,
		Last7DCalls:      calls7D,
		SuccessRate:      successRate,
		AvgResponseMS:    avgMS,
		UniqueUsers:      uniqueUsers,
		TotalTokens:      totalUsage.TotalTokens,
		TodayTokens:      todayUsage.TotalTokens,
		Last7DTokens:     usage7D.TotalTokens,
		AvgTokensPerCall: totalUsage.AvgTokensPerCall,
		ByFeature:        byFeature,
		ByFeatureTokens:  byFeatureTokens,
		DailyTrend:       dailyTrend,
		DailyTokenTrend:  dailyTokenTrend,
		TopUsers:         topUsers,
		TopTokenUsers:    topTokenUsers,
	}
}

func signPayload(payloadPart string, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payloadPart))
	sig := mac.Sum(nil)
	return base64.RawURLEncoding.EncodeToString(sig)
}

// ── Community QR Config ──

const (
	siteConfigKeyCommunityQR = "community_qr"

	communityQRTitleDefault       = "卧龙AI量化交流群"
	communityQRTitleMaxLen        = 50
	communityQRDescriptionMaxLen  = 200
	communityQRBase64MaxBytes     = 2 * 1024 * 1024 // 2 MB decoded
)

// communityQRStoredValue is the JSON shape persisted in site_config.value.
type communityQRStoredValue struct {
	IsEnabled     bool   `json:"is_enabled"`
	Title         string `json:"title"`
	Description   string `json:"description"`
	QRImageBase64 string `json:"qr_image_base64"`
}

// GetCommunityQRConfig returns the public-facing community QR config.
// Returns a zero-valued (disabled) config if not yet configured.
func (s *Service) GetCommunityQRConfig(ctx context.Context) (CommunityQRConfig, error) {
	raw, updatedAt, _, found, err := s.repo.GetSiteConfig(ctx, siteConfigKeyCommunityQR)
	if err != nil {
		return CommunityQRConfig{}, fmt.Errorf("get community qr config: %w", err)
	}
	if !found {
		return CommunityQRConfig{IsEnabled: false}, nil
	}

	var stored communityQRStoredValue
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		return CommunityQRConfig{IsEnabled: false}, nil
	}

	return CommunityQRConfig{
		IsEnabled:     stored.IsEnabled,
		Title:         stored.Title,
		Description:   stored.Description,
		QRImageBase64: stored.QRImageBase64,
		UpdatedAt:     formatAdminTimestamp(updatedAt),
	}, nil
}

// GetCommunityQRConfigAdminView returns the admin view including updated_by.
func (s *Service) GetCommunityQRConfigAdminView(ctx context.Context) (CommunityQRConfigAdminView, error) {
	raw, updatedAt, updatedBy, found, err := s.repo.GetSiteConfig(ctx, siteConfigKeyCommunityQR)
	if err != nil {
		return CommunityQRConfigAdminView{}, fmt.Errorf("get community qr config (admin): %w", err)
	}
	if !found {
		return CommunityQRConfigAdminView{
			CommunityQRConfig: CommunityQRConfig{IsEnabled: false},
			UpdatedBy:         "",
		}, nil
	}

	var stored communityQRStoredValue
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		return CommunityQRConfigAdminView{
			CommunityQRConfig: CommunityQRConfig{IsEnabled: false},
			UpdatedBy:         updatedBy,
		}, nil
	}

	return CommunityQRConfigAdminView{
		CommunityQRConfig: CommunityQRConfig{
			IsEnabled:     stored.IsEnabled,
			Title:         stored.Title,
			Description:   stored.Description,
			QRImageBase64: stored.QRImageBase64,
			UpdatedAt:     formatAdminTimestamp(updatedAt),
		},
		UpdatedBy: updatedBy,
	}, nil
}

// SaveCommunityQRConfig validates, persists, and returns the updated admin view.
func (s *Service) SaveCommunityQRConfig(ctx context.Context, input SaveCommunityQRConfigInput) (CommunityQRConfigAdminView, error) {
	// Resolve admin email for audit.
	adminEmail := ""
	if admin, ok := CurrentAdminFromContext(ctx); ok {
		adminEmail = admin.Email
	}

	// ── Validation ──
	title := strings.TrimSpace(input.Title)
	if title == "" {
		title = communityQRTitleDefault
	}
	if len([]rune(title)) > communityQRTitleMaxLen {
		return CommunityQRConfigAdminView{}, fmt.Errorf("%w: 标题不能超过 %d 字符", ErrSiteConfigInvalid, communityQRTitleMaxLen)
	}

	description := strings.TrimSpace(input.Description)
	if len([]rune(description)) > communityQRDescriptionMaxLen {
		return CommunityQRConfigAdminView{}, fmt.Errorf("%w: 描述不能超过 %d 字符", ErrSiteConfigInvalid, communityQRDescriptionMaxLen)
	}

	qrImage := strings.TrimSpace(input.QRImageBase64)

	// If enabling, the QR image must be present and valid.
	if input.IsEnabled {
		if qrImage == "" {
			return CommunityQRConfigAdminView{}, fmt.Errorf("%w: 启用展示时必须上传二维码图片", ErrSiteConfigInvalid)
		}
		if err := validateQRBase64Image(qrImage); err != nil {
			return CommunityQRConfigAdminView{}, fmt.Errorf("%w: %s", ErrSiteConfigInvalid, err.Error())
		}
	}

	// If the image is provided (even when disabled), validate it.
	if qrImage != "" {
		if err := validateQRBase64Image(qrImage); err != nil {
			return CommunityQRConfigAdminView{}, fmt.Errorf("%w: %s", ErrSiteConfigInvalid, err.Error())
		}
	}

	// If no new image is provided but we are saving, preserve the existing one.
	if qrImage == "" {
		existingRaw, _, _, found, _ := s.repo.GetSiteConfig(ctx, siteConfigKeyCommunityQR)
		if found {
			var existing communityQRStoredValue
			if err := json.Unmarshal([]byte(existingRaw), &existing); err == nil {
				qrImage = existing.QRImageBase64
			}
		}
	}

	stored := communityQRStoredValue{
		IsEnabled:     input.IsEnabled,
		Title:         title,
		Description:   description,
		QRImageBase64: qrImage,
	}

	payloadBytes, err := json.Marshal(stored)
	if err != nil {
		return CommunityQRConfigAdminView{}, fmt.Errorf("marshal community qr config: %w", err)
	}

	if err := s.repo.UpsertSiteConfig(ctx, siteConfigKeyCommunityQR, string(payloadBytes), adminEmail); err != nil {
		return CommunityQRConfigAdminView{}, fmt.Errorf("save community qr config: %w", err)
	}

	return s.GetCommunityQRConfigAdminView(ctx)
}

// validateQRBase64Image validates the data URL prefix and decoded size.
func validateQRBase64Image(dataURL string) error {
	// Must start with data:image/ prefix.
	if !strings.HasPrefix(dataURL, "data:image/") {
		return fmt.Errorf("二维码图片必须为 data:image/ 开头的 base64 格式")
	}

	// Extract the base64 portion after the comma.
	commaIdx := strings.Index(dataURL, ",")
	if commaIdx < 0 {
		return fmt.Errorf("二维码图片 base64 数据格式不正确")
	}
	b64Data := dataURL[commaIdx+1:]

	// Decode to check actual size.
	decoded, err := base64.StdEncoding.DecodeString(b64Data)
	if err != nil {
		return fmt.Errorf("二维码图片 base64 解码失败")
	}
	if len(decoded) > communityQRBase64MaxBytes {
		return fmt.Errorf("二维码图片不能超过 %d MB", communityQRBase64MaxBytes/(1024*1024))
	}
	return nil
}

// formatAdminTimestamp formats a time value for API response (ISO 8601 UTC).
func formatAdminTimestamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02T15:04:05Z")
}
