package auth

import (
	"context"
	_ "embed"
	"strconv"
	"strings"
	"time"
)

//go:embed templates/mail/password-reset.html
var passwordResetHTMLTemplate string

//go:embed templates/mail/password-reset.txt
var passwordResetTextTemplate string

type Mailer interface {
	Send(ctx context.Context, message MailMessage) error
}

type MailMessage struct {
	ToEmail      string
	Subject      string
	HTMLBody     string
	TextBody     string
	TemplateData map[string]any
	Tag          string
	RequestID    string
}

type PasswordResetConfig struct {
	PublicBaseURL     string
	TTL               time.Duration
	RateLimitWindow   time.Duration
	RateLimitPerIP    int
	RateLimitPerEmail int
	EmailCooldown     time.Duration
}

type PasswordResetTokenStatus struct {
	Valid      bool   `json:"valid"`
	Code       string `json:"code,omitempty"`
	Detail     string `json:"detail,omitempty"`
	ExpiresAt  string `json:"expires_at,omitempty"`
	ConsumedAt string `json:"consumed_at,omitempty"`
}

type PasswordResetMailTemplateData struct {
	Token         string
	ExpireMinutes int
	ProductName   string
}

func BuildPasswordResetMailTemplate(data PasswordResetMailTemplateData) (string, string) {
	productName := strings.TrimSpace(data.ProductName)
	if productName == "" {
		productName = "卧龙 Trader"
	}
	token := strings.TrimSpace(data.Token)
	expireMinutes := data.ExpireMinutes
	if expireMinutes <= 0 {
		expireMinutes = 30
	}

	replacements := map[string]string{
		"{{PRODUCT_NAME}}":   productName,
		"{{token}}":          token,
		"{{EXPIRE_MINUTES}}": strconv.Itoa(expireMinutes),
	}
	htmlBody := applyPasswordResetTemplate(passwordResetHTMLTemplate, replacements)
	textBody := applyPasswordResetTemplate(passwordResetTextTemplate, replacements)
	return htmlBody, textBody
}

func applyPasswordResetTemplate(template string, replacements map[string]string) string {
	result := template
	for placeholder, value := range replacements {
		result = strings.ReplaceAll(result, placeholder, value)
	}
	return result
}
