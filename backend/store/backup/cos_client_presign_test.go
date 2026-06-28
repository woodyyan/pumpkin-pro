package backup

import (
	"net/url"
	"strings"
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

func TestPresignGetURLWithProcessKeepsParamRaw(t *testing.T) {
	client := NewCOSCloudStorageClient("bucket-123", "ap-guangzhou", "ak-test", "sk-test")
	client.now = func() time.Time { return time.Date(2026, 6, 27, 1, 2, 3, 0, time.UTC) }

	signed, err := client.PresignGetURLWithProcess("ai-reports/preview/2026/catl.webp", 15*time.Minute, "imageMogr2/thumbnail/!30p")
	if err != nil {
		t.Fatalf("presign with process: %v", err)
	}

	// 处理参数必须以原始字符串出现，斜杠/叹号不被转义，且只出现一次。
	if !strings.Contains(signed, "imageMogr2/thumbnail/!30p") {
		t.Fatalf("expected raw process param in url, got %s", signed)
	}
	if strings.Contains(signed, "imageMogr2%2F") || strings.Contains(signed, "%2130p") {
		t.Fatalf("process param should not be percent-encoded, got %s", signed)
	}
	if strings.Count(signed, "imageMogr2/thumbnail/!30p") != 1 {
		t.Fatalf("process param should appear exactly once, got %s", signed)
	}

	parsed, err := url.Parse(signed)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	q := parsed.Query()
	// 下载时处理参数不参与签名，q-url-param-list 为空。
	if q.Get("q-url-param-list") != "" {
		t.Fatalf("expected empty q-url-param-list, got %q", q.Get("q-url-param-list"))
	}
	if q.Get("q-signature") == "" {
		t.Fatalf("missing signature")
	}
	if q.Get("q-ak") != "ak-test" {
		t.Fatalf("unexpected ak: %s", q.Get("q-ak"))
	}
}

// 不同处理参数下，签名本身不变（签名只覆盖 host），但处理参数原样附加在 URL 上。
func TestPresignGetURLProcessNotInSignature(t *testing.T) {
	client := NewCOSCloudStorageClient("bucket-123", "ap-guangzhou", "ak-test", "sk-test")
	client.now = func() time.Time { return time.Date(2026, 6, 27, 1, 2, 3, 0, time.UTC) }

	a, err := client.PresignGetURLWithProcess("a/b.webp", time.Minute, "imageMogr2/format/webp")
	if err != nil {
		t.Fatalf("presign a: %v", err)
	}
	b, err := client.PresignGetURLWithProcess("a/b.webp", time.Minute, "imageMogr2/thumbnail/!30p")
	if err != nil {
		t.Fatalf("presign b: %v", err)
	}
	sigA, _ := url.Parse(a)
	sigB, _ := url.Parse(b)
	if sigA.Query().Get("q-signature") != sigB.Query().Get("q-signature") {
		t.Fatalf("download-time process should not change signature")
	}
}
