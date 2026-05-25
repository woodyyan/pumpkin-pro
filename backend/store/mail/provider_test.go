package mail

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/woodyyan/pumpkin-pro/backend/config"
	"github.com/woodyyan/pumpkin-pro/backend/store/auth"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestTencentCloudProviderSendUsesTemplatePayload(t *testing.T) {
	type requestCapture struct {
		Authorization string
		ContentType   string
		Action        string
		Version       string
		Region        string
		Language      string
		Token         string
		Host          string
		RawBody       string
		Body          tencentPayload
	}
	var captured requestCapture

	provider := &TencentCloudProvider{
		cfg: config.MailConfig{
			FromEmail:         "no-reply@wolongtrader.top",
			TencentSecretID:   "secret-id",
			TencentSecretKey:  "secret-key",
			TencentToken:      "session-token",
			TencentRegion:     "ap-hongkong",
			TencentEndpoint:   "https://ses.tencentcloudapi.com",
			TencentAPIVersion: "2020-10-02",
			TencentAPIAction:  "SendEmail",
			TencentLanguage:   "zh-CN",
			TencentTemplateID: 179710,
		},
		client: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			captured.Authorization = r.Header.Get("Authorization")
			captured.ContentType = r.Header.Get("Content-Type")
			captured.Action = r.Header.Get("X-TC-Action")
			captured.Version = r.Header.Get("X-TC-Version")
			captured.Region = r.Header.Get("X-TC-Region")
			captured.Language = r.Header.Get("X-TC-Language")
			captured.Token = r.Header.Get("X-TC-Token")
			captured.Host = r.Host
			defer r.Body.Close()
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read request body: %v", err)
			}
			captured.RawBody = string(body)
			if err := json.Unmarshal(body, &captured.Body); err != nil {
				t.Fatalf("decode request body: %v", err)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"Response":{"RequestId":"req-1"}}`)),
			}, nil
		})},
	}

	err := provider.Send(context.Background(), auth.MailMessage{
		ToEmail:      "colorguitar@hotmail.com",
		Subject:      "测试",
		Tag:          "password_reset",
		TemplateData: map[string]any{"PRODUCT_NAME": "卧龙AI量化交易台", "EXPIRE_MINUTES": 30, "token": "abc"},
	})
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if captured.Body.FromEmailAddress != "no-reply@wolongtrader.top" {
		t.Fatalf("FromEmailAddress = %s", captured.Body.FromEmailAddress)
	}
	if captured.Body.ReplyToAddresses != "no-reply@wolongtrader.top" {
		t.Fatalf("ReplyToAddresses = %s", captured.Body.ReplyToAddresses)
	}
	if captured.Body.Subject != "测试" {
		t.Fatalf("Subject = %s", captured.Body.Subject)
	}
	if len(captured.Body.Destination) != 1 || captured.Body.Destination[0] != "colorguitar@hotmail.com" {
		t.Fatalf("Destination = %#v", captured.Body.Destination)
	}
	if captured.Body.Template.TemplateID != 179710 {
		t.Fatalf("TemplateID = %d", captured.Body.Template.TemplateID)
	}
	var templateData map[string]any
	if err := json.Unmarshal([]byte(captured.Body.Template.TemplateData), &templateData); err != nil {
		t.Fatalf("unmarshal TemplateData: %v", err)
	}
	if templateData["PRODUCT_NAME"] != "卧龙AI量化交易台" {
		t.Fatalf("PRODUCT_NAME = %#v", templateData["PRODUCT_NAME"])
	}
	if templateData["token"] != "abc" {
		t.Fatalf("token = %#v", templateData["token"])
	}
	if captured.ContentType != "application/json" {
		t.Fatalf("Content-Type = %s", captured.ContentType)
	}
	if captured.Action != "SendEmail" || captured.Version != "2020-10-02" || captured.Region != "ap-hongkong" {
		t.Fatalf("unexpected tc headers: action=%s version=%s region=%s", captured.Action, captured.Version, captured.Region)
	}
	if captured.Language != "zh-CN" {
		t.Fatalf("X-TC-Language = %s", captured.Language)
	}
	if captured.Token != "session-token" {
		t.Fatalf("X-TC-Token = %s", captured.Token)
	}
	if !strings.Contains(captured.Authorization, "SignedHeaders=content-type;host") {
		t.Fatalf("Authorization = %s", captured.Authorization)
	}
	if captured.Host != "ses.tencentcloudapi.com" {
		t.Fatalf("Host = %s", captured.Host)
	}
	if strings.Contains(captured.RawBody, "TagName") {
		t.Fatalf("request body must not include TagName: %s", captured.RawBody)
	}
}

func TestTencentCloudProviderSendRequiresTemplateID(t *testing.T) {
	provider := &TencentCloudProvider{
		cfg: config.MailConfig{
			FromEmail:        "no-reply@wolongtrader.top",
			TencentSecretID:  "secret-id",
			TencentSecretKey: "secret-key",
		},
		client: http.DefaultClient,
	}

	err := provider.Send(context.Background(), auth.MailMessage{
		ToEmail:      "user@example.com",
		Subject:      "测试",
		TemplateData: map[string]any{"token": "abc"},
	})
	if err == nil || !strings.Contains(err.Error(), "template id") {
		t.Fatalf("expected template id error, got %v", err)
	}
}
