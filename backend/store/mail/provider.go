package mail

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/woodyyan/pumpkin-pro/backend/config"
	"github.com/woodyyan/pumpkin-pro/backend/store/auth"
)

type Provider interface {
	Send(ctx context.Context, message auth.MailMessage) error
}

type providerAdapter struct {
	provider Provider
}

func (a providerAdapter) Send(ctx context.Context, message auth.MailMessage) error {
	return a.provider.Send(ctx, message)
}

func New(cfg config.MailConfig) (auth.Mailer, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case "", "mock":
		return providerAdapter{provider: &MockProvider{cfg: cfg}}, nil
	case "tencent", "tencent_ses", "tencentcloud_ses":
		return providerAdapter{provider: &TencentCloudProvider{cfg: cfg, client: &http.Client{Timeout: 15 * time.Second}}}, nil
	default:
		return nil, fmt.Errorf("unsupported mail provider: %s", cfg.Provider)
	}
}

type MockProvider struct {
	cfg config.MailConfig
}

func (p *MockProvider) Send(_ context.Context, message auth.MailMessage) error {
	if p.cfg.MockFailDelivery {
		return fmt.Errorf("mock mail delivery failure")
	}
	captured := strings.TrimSpace(p.cfg.MockCaptureRecipient)
	if captured == "" {
		captured = "dev-null@local.invalid"
	}
	log.Printf("[mail:mock] to=%s captured_as=%s subject=%s tag=%s request_id=%s", message.ToEmail, captured, message.Subject, message.Tag, message.RequestID)
	if p.cfg.MockLogBodies {
		log.Printf("[mail:mock:text] %s", message.TextBody)
	}
	return nil
}

type TencentCloudProvider struct {
	cfg    config.MailConfig
	client *http.Client
}

type tencentPayload struct {
	FromEmailAddress string          `json:"FromEmailAddress"`
	Destination      []string        `json:"Destination"`
	Subject          string          `json:"Subject"`
	ReplyToAddresses string          `json:"ReplyToAddresses,omitempty"`
	Template         tencentTemplate `json:"Template"`
}

type tencentTemplate struct {
	TemplateID   int    `json:"TemplateID"`
	TemplateData string `json:"TemplateData"`
}

type tencentResponse struct {
	Response struct {
		RequestID string `json:"RequestId"`
		Error     *struct {
			Code    string `json:"Code"`
			Message string `json:"Message"`
		} `json:"Error,omitempty"`
	} `json:"Response"`
}

func (p *TencentCloudProvider) Send(ctx context.Context, message auth.MailMessage) error {
	if strings.TrimSpace(p.cfg.TencentSecretID) == "" || strings.TrimSpace(p.cfg.TencentSecretKey) == "" {
		return fmt.Errorf("tencent mail credentials are not configured")
	}
	if strings.TrimSpace(p.cfg.FromEmail) == "" {
		return fmt.Errorf("tencent mail from email is not configured")
	}
	if p.cfg.TencentTemplateID <= 0 {
		return fmt.Errorf("tencent mail template id is not configured")
	}
	if len(message.TemplateData) == 0 {
		return fmt.Errorf("tencent mail template data is required")
	}

	templateData, err := json.Marshal(message.TemplateData)
	if err != nil {
		return fmt.Errorf("marshal tencent mail template data: %w", err)
	}
	payload := tencentPayload{
		FromEmailAddress: strings.TrimSpace(p.cfg.FromEmail),
		Destination:      []string{strings.TrimSpace(message.ToEmail)},
		Subject:          strings.TrimSpace(message.Subject),
		ReplyToAddresses: strings.TrimSpace(p.cfg.FromEmail),
		Template: tencentTemplate{
			TemplateID:   p.cfg.TencentTemplateID,
			TemplateData: string(templateData),
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	endpoint := strings.TrimSpace(p.cfg.TencentEndpoint)
	if endpoint == "" {
		endpoint = "https://ses.tencentcloudapi.com"
	}
	version := strings.TrimSpace(p.cfg.TencentAPIVersion)
	if version == "" {
		version = "2020-10-02"
	}
	action := strings.TrimSpace(p.cfg.TencentAPIAction)
	if action == "" {
		action = "SendEmail"
	}
	region := strings.TrimSpace(p.cfg.TencentRegion)
	if region == "" {
		region = "ap-hongkong"
	}
	language := strings.TrimSpace(p.cfg.TencentLanguage)
	if language == "" {
		language = "zh-CN"
	}

	parsedEndpoint, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("parse tencent endpoint: %w", err)
	}
	host := parsedEndpoint.Host
	if host == "" {
		return fmt.Errorf("invalid tencent endpoint: missing host")
	}
	canonicalPath := parsedEndpoint.EscapedPath()
	if canonicalPath == "" {
		canonicalPath = "/"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Host = host
	service := "ses"
	timestamp := time.Now().UTC()
	authorization := buildTC3Authorization(p.cfg.TencentSecretID, p.cfg.TencentSecretKey, service, host, canonicalPath, timestamp, body)
	req.Header.Set("Authorization", authorization)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-TC-Action", action)
	req.Header.Set("X-TC-Version", version)
	req.Header.Set("X-TC-Region", region)
	req.Header.Set("X-TC-Language", language)
	req.Header.Set("X-TC-Timestamp", fmt.Sprintf("%d", timestamp.Unix()))
	if token := strings.TrimSpace(p.cfg.TencentToken); token != "" {
		req.Header.Set("X-TC-Token", token)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("tencent mail http %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var decoded tencentResponse
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		return fmt.Errorf("decode tencent mail response: %w", err)
	}
	if decoded.Response.Error != nil {
		return fmt.Errorf("tencent mail %s: %s", decoded.Response.Error.Code, decoded.Response.Error.Message)
	}
	return nil
}

func buildTC3Authorization(secretID, secretKey, service, host, canonicalPath string, timestamp time.Time, body []byte) string {
	hashedPayload := sha256Hex(body)
	canonicalHeaders := "content-type:application/json\nhost:" + host + "\n"
	signedHeaders := "content-type;host"
	canonicalRequest := strings.Join([]string{
		http.MethodPost,
		canonicalPath,
		"",
		canonicalHeaders,
		signedHeaders,
		hashedPayload,
	}, "\n")
	date := timestamp.UTC().Format("2006-01-02")
	credentialScope := fmt.Sprintf("%s/%s/tc3_request", date, service)
	stringToSign := strings.Join([]string{
		"TC3-HMAC-SHA256",
		fmt.Sprintf("%d", timestamp.Unix()),
		credentialScope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")
	secretDate := hmacSHA256([]byte("TC3"+secretKey), date)
	secretService := hmacSHA256(secretDate, service)
	secretSigning := hmacSHA256(secretService, "tc3_request")
	signature := hex.EncodeToString(hmacSHA256(secretSigning, stringToSign))
	return fmt.Sprintf("TC3-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s", secretID, credentialScope, signedHeaders, signature)
}

func hmacSHA256(key []byte, message string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(message))
	return mac.Sum(nil)
}

func sha256Hex(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
