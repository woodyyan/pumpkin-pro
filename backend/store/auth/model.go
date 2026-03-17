package auth

import "time"

type UserRecord struct {
	ID           string    `gorm:"primaryKey;size:36"`
	Email        string    `gorm:"size:128;not null;uniqueIndex"`
	PasswordHash string    `gorm:"size:255;not null"`
	Status       string    `gorm:"size:20;not null;default:'active'"`
	CreatedAt    time.Time `gorm:"not null"`
	UpdatedAt    time.Time `gorm:"not null"`
}

func (UserRecord) TableName() string {
	return "users"
}

type UserProfileRecord struct {
	UserID    string    `gorm:"primaryKey;size:36"`
	Nickname  string    `gorm:"size:64;not null;default:''"`
	AvatarURL string    `gorm:"size:512;not null;default:''"`
	Timezone  string    `gorm:"size:64;not null;default:'Asia/Shanghai'"`
	UpdatedAt time.Time `gorm:"not null"`
}

func (UserProfileRecord) TableName() string {
	return "user_profiles"
}

type UserSessionRecord struct {
	ID               string     `gorm:"primaryKey;size:36"`
	UserID           string     `gorm:"size:36;not null;index"`
	RefreshTokenHash string     `gorm:"size:64;not null;uniqueIndex"`
	ExpiresAt        time.Time  `gorm:"not null;index"`
	RevokedAt        *time.Time `gorm:"index"`
	CreatedAt        time.Time  `gorm:"not null"`
}

func (UserSessionRecord) TableName() string {
	return "user_sessions"
}

type AuthAuditRecord struct {
	ID          string    `gorm:"primaryKey;size:36"`
	UserID      string    `gorm:"size:36;index"`
	EmailMasked string    `gorm:"size:128;not null;default:''"`
	Action      string    `gorm:"size:32;not null;index"`
	IP          string    `gorm:"size:64;not null;default:''"`
	UserAgent   string    `gorm:"size:512;not null;default:''"`
	Success     bool      `gorm:"not null;default:false"`
	Message     string    `gorm:"type:text;not null;default:''"`
	CreatedAt   time.Time `gorm:"not null;index"`
}

func (AuthAuditRecord) TableName() string {
	return "auth_audit_logs"
}

type UserProfile struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Nickname  string `json:"nickname"`
	AvatarURL string `json:"avatar_url"`
	Timezone  string `json:"timezone"`
}

type AuthTokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

type AuthSessionResult struct {
	User   UserProfile `json:"user"`
	Tokens AuthTokens  `json:"tokens"`
}

type RegisterInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type RefreshInput struct {
	RefreshToken string `json:"refresh_token"`
}

type LogoutInput struct {
	RefreshToken string `json:"refresh_token"`
}

type UpdateProfileInput struct {
	Nickname  string `json:"nickname"`
	AvatarURL string `json:"avatar_url"`
	Timezone  string `json:"timezone"`
}

type ChangePasswordInput struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}
