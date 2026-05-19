package signal

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
)

const (
	WebhookChannelWeCom  = "wecom"
	WebhookChannelFeishu = "feishu"
)

type webhookChannelAdapter interface {
	BuildPayload(content string, timestamp string, secret string) map[string]any
	PrepareURL(rawURL string, timestamp string, secret string) (string, error)
}

type wecomWebhookAdapter struct{}

type feishuWebhookAdapter struct{}

func normalizeWebhookChannel(raw string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	if normalized == "" {
		return WebhookChannelWeCom, nil
	}
	switch normalized {
	case WebhookChannelWeCom, WebhookChannelFeishu:
		return normalized, nil
	default:
		return "", fmt.Errorf("%w: channel 仅支持 wecom 或 feishu", ErrInvalidInput)
	}
}

func getWebhookChannelAdapter(channel string) webhookChannelAdapter {
	if strings.EqualFold(strings.TrimSpace(channel), WebhookChannelFeishu) {
		return feishuWebhookAdapter{}
	}
	return wecomWebhookAdapter{}
}

func buildWebhookOfficialSignature(timestamp string, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("\n"))
	mac.Write([]byte(secret))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func (wecomWebhookAdapter) BuildPayload(content string, _ string, _ string) map[string]any {
	return map[string]any{
		"msgtype": "text",
		"text": map[string]any{
			"content": content,
		},
	}
}

func (wecomWebhookAdapter) PrepareURL(rawURL string, timestamp string, secret string) (string, error) {
	if strings.TrimSpace(secret) == "" {
		return rawURL, nil
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	query.Set("timestamp", timestamp)
	query.Set("sign", buildWebhookOfficialSignature(timestamp, secret))
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func (feishuWebhookAdapter) BuildPayload(content string, timestamp string, secret string) map[string]any {
	payload := map[string]any{
		"msg_type": "text",
		"content": map[string]any{
			"text": content,
		},
	}
	if strings.TrimSpace(secret) != "" {
		payload["timestamp"] = timestamp
		payload["sign"] = buildWebhookOfficialSignature(timestamp, secret)
	}
	return payload
}

func (feishuWebhookAdapter) PrepareURL(rawURL string, _ string, _ string) (string, error) {
	return rawURL, nil
}
