package admin

import "time"

type SuperAdminRecord struct {
	ID           string    `gorm:"primaryKey;size:36"`
	Email        string    `gorm:"size:128;not null;uniqueIndex"`
	PasswordHash string    `gorm:"size:255;not null"`
	Nickname     string    `gorm:"size:64;not null;default:''"`
	Status       string    `gorm:"size:20;not null;default:'active'"`
	CreatedAt    time.Time `gorm:"not null"`
	UpdatedAt    time.Time `gorm:"not null"`
}

func (SuperAdminRecord) TableName() string {
	return "super_admins"
}

type AdminProfile struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Nickname string `json:"nickname"`
}

type AdminLoginInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type AdminLoginResult struct {
	Admin  AdminProfile `json:"admin"`
	Tokens AdminTokens  `json:"tokens"`
}

type AdminTokens struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

type AdminAccessClaims struct {
	AdminID   string `json:"aid"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
}

// ── Stats response types ──

type StatsResult struct {
	Users      UserStats      `json:"users"`
	Strategies StrategyStats  `json:"strategies"`
	Live       LiveStats      `json:"live"`
	Signals    SignalStats    `json:"signals"`
	Audit      AuditStats     `json:"audit"`
	GeneratedAt string        `json:"generated_at"`
}

type UserStats struct {
	Total          int64 `json:"total"`
	Today          int64 `json:"today"`
	Last7D         int64 `json:"last_7d"`
	Last30D        int64 `json:"last_30d"`
	Active7D       int64 `json:"active_7d"`
	ActiveSessions int64 `json:"active_sessions"`
}

type StrategyStats struct {
	Total       int64 `json:"total"`
	System      int64 `json:"system"`
	UserCreated int64 `json:"user_created"`
	Active      int64 `json:"active"`
	Referenced  int64 `json:"referenced"`
}

type LiveStats struct {
	WatchlistItems     int64 `json:"watchlist_items"`
	UsersWithWatchlist int64 `json:"users_with_watchlist"`
	ActiveSymbols      int64 `json:"active_symbols"`
}

type SignalStats struct {
	WebhookUsers        int64   `json:"webhook_users"`
	WebhookEnabledRate  float64 `json:"webhook_enabled_rate"`
	SignalConfigs       int64   `json:"signal_configs"`
	SignalConfigsEnabled int64  `json:"signal_configs_enabled"`
	TotalEvents         int64   `json:"total_events"`
	TodayEvents         int64   `json:"today_events"`
	DeliverySuccessRate float64 `json:"delivery_success_rate"`
	TodayDeliveries     int64   `json:"today_deliveries"`
}

type AuditStats struct {
	TodayLogins        int64 `json:"today_logins"`
	TodayRegistrations int64 `json:"today_registrations"`
	FailedLogins7D     int64 `json:"failed_logins_7d"`
}
