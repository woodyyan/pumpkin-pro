package backup

import (
	"net/url"
	"testing"
	"time"
)

func TestPresignGetURLBuildsSignedURL(t *testing.T) {
	client := NewCOSCloudStorageClient("bucket-123", "ap-guangzhou", "ak-test", "sk-test")
	client.now = func() time.Time { return time.Date(2026, 6, 27, 1, 2, 3, 0, time.UTC) }

	signed, err := client.PresignGetURL("ai-reports/preview/2026/catl.webp", 15*time.Minute)
	if err != nil {
		t.Fatalf("presign: %v", err)
	}

	parsed, err := url.Parse(signed)
	if err != nil {
		t.Fatalf("parse signed url: %v", err)
	}
	if parsed.Scheme != "https" || parsed.Host != "bucket-123.cos.ap-guangzhou.myqcloud.com" {
		t.Fatalf("unexpected scheme/host: %s", signed)
	}
	if parsed.Path != "/ai-reports/preview/2026/catl.webp" {
		t.Fatalf("unexpected path: %s", parsed.Path)
	}

	q := parsed.Query()
	for _, key := range []string{"q-sign-algorithm", "q-ak", "q-sign-time", "q-key-time", "q-header-list", "q-signature"} {
		if q.Get(key) == "" && key != "q-header-list" {
			t.Fatalf("missing signature param %s in %s", key, signed)
		}
	}
	if q.Get("q-sign-algorithm") != "sha1" {
		t.Fatalf("unexpected algorithm: %s", q.Get("q-sign-algorithm"))
	}
	if q.Get("q-ak") != "ak-test" {
		t.Fatalf("unexpected ak: %s", q.Get("q-ak"))
	}
	if q.Get("q-header-list") != "host" {
		t.Fatalf("unexpected header list: %s", q.Get("q-header-list"))
	}
}

func TestPresignGetURLRequiresCredentials(t *testing.T) {
	client := NewCOSCloudStorageClient("bucket-123", "ap-guangzhou", "", "")
	if _, err := client.PresignGetURL("a/b.png", time.Minute); err == nil {
		t.Fatalf("expected error when credentials are missing")
	}
}

func TestPresignGetURLDefaultsTTL(t *testing.T) {
	client := NewCOSCloudStorageClient("bucket-123", "ap-guangzhou", "ak-test", "sk-test")
	fixed := time.Date(2026, 6, 27, 1, 2, 3, 0, time.UTC)
	client.now = func() time.Time { return fixed }

	signed, err := client.PresignGetURL("a/b.png", 0)
	if err != nil {
		t.Fatalf("presign: %v", err)
	}
	parsed, _ := url.Parse(signed)
	keyTime := parsed.Query().Get("q-key-time")
	if keyTime == "" {
		t.Fatalf("missing key time")
	}
}
