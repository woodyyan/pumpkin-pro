package auth

import (
	"context"
	"fmt"
	"html"
	"strings"
	"time"
)

type Mailer interface {
	Send(ctx context.Context, message MailMessage) error
}

type MailMessage struct {
	ToEmail   string
	Subject   string
	HTMLBody  string
	TextBody  string
	Tag       string
	RequestID string
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
	ResetURL      string
	ExpireMinutes int
	ProductName   string
}

func BuildPasswordResetMailTemplate(data PasswordResetMailTemplateData) (string, string) {
	productName := strings.TrimSpace(data.ProductName)
	if productName == "" {
		productName = "卧龙 Trader"
	}
	resetURL := strings.TrimSpace(data.ResetURL)
	expireMinutes := data.ExpireMinutes
	if expireMinutes <= 0 {
		expireMinutes = 30
	}

	escapedURL := html.EscapeString(resetURL)
	escapedProduct := html.EscapeString(productName)
	htmlBody := fmt.Sprintf(`<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>重置密码</title>
</head>
<body style="margin:0;padding:0;background:#f4f7fb;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;color:#111827;">
  <table role="presentation" width="100%%" cellspacing="0" cellpadding="0" style="background:#f4f7fb;padding:24px 0;">
    <tr>
      <td align="center">
        <table role="presentation" width="100%%" cellspacing="0" cellpadding="0" style="max-width:560px;background:#ffffff;border-radius:20px;overflow:hidden;box-shadow:0 16px 40px rgba(15,23,42,0.08);">
          <tr>
            <td style="padding:32px 32px 20px;background:linear-gradient(135deg,#0f172a,#1d4ed8);color:#ffffff;">
              <div style="font-size:14px;letter-spacing:0.08em;text-transform:uppercase;opacity:0.82;">%s</div>
              <h1 style="margin:12px 0 0;font-size:28px;line-height:1.3;">重置你的登录密码</h1>
            </td>
          </tr>
          <tr>
            <td style="padding:32px;">
              <p style="margin:0 0 16px;font-size:16px;line-height:1.75;">你刚刚发起了密码重置请求。点击下面的按钮即可继续操作。</p>
              <p style="margin:0 0 24px;font-size:16px;line-height:1.75;">链接有效期为 <strong>%d 分钟</strong>。如果不是你本人操作，可以直接忽略这封邮件。</p>
              <p style="margin:0 0 24px;">
                <a href="%s" style="display:inline-block;padding:14px 24px;border-radius:999px;background:#f59e0b;color:#111827;text-decoration:none;font-size:16px;font-weight:700;">立即重置密码</a>
              </p>
              <p style="margin:0 0 12px;font-size:13px;line-height:1.7;color:#6b7280;">如果按钮无法打开，请复制下面的链接到浏览器：</p>
              <p style="margin:0;padding:16px;border-radius:14px;background:#eff6ff;word-break:break-all;font-size:13px;line-height:1.7;color:#1d4ed8;">%s</p>
            </td>
          </tr>
        </table>
      </td>
    </tr>
  </table>
</body>
</html>`, escapedProduct, expireMinutes, escapedURL, escapedURL)

	textBody := fmt.Sprintf("%s\n\n你刚刚发起了密码重置请求。\n请在 %d 分钟内打开下面的链接完成操作：\n\n%s\n\n如果不是你本人操作，可以直接忽略这封邮件。", productName, expireMinutes, resetURL)
	return htmlBody, textBody
}
