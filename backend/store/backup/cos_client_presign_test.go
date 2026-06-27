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

func TestPresignGetURLWithImageProcessParams(t *testing.T) {
	client := NewCOSCloudStorageClient("bucket-123", "ap-guangzhou", "ak-test", "sk-test")
	client.now = func() time.Time { return time.Date(2026, 6, 27, 1, 2, 3, 0, time.UTC) }

	params := url.Values{"imageMogr2/format/webp": []string{""}}
	signed, err := client.PresignGetURLWithParams("ai-reports/preview/2026/catl.webp", 15*time.Minute, params)
	if err != nil {
		t.Fatalf("presign with params: %v", err)
	}

	parsed, err := url.Parse(signed)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	q := parsed.Query()

	// 图片处理参数必须出现在最终 URL 上。
	if _, ok := q["imageMogr2/format/webp"]; !ok {
		t.Fatalf("expected image process param in url, got %s", signed)
	}
	// 该参数必须被声明进 q-url-param-list（小写、参与签名），否则严格校验桶会 403。
	if q.Get("q-url-param-list") != "imagemogr2/format/webp" {
		t.Fatalf("expected param declared in q-url-param-list, got %q (url=%s)", q.Get("q-url-param-list"), signed)
	}
	if q.Get("q-signature") == "" {
		t.Fatalf("missing signature")
	}
}

// 两次对同一对象、不同处理参数生成的签名必须不同（说明参数确实参与了签名计算）。
func TestPresignGetURLParamsAffectSignature(t *testing.T) {
	client := NewCOSCloudStorageClient("bucket-123", "ap-guangzhou", "ak-test", "sk-test")
	client.now = func() time.Time { return time.Date(2026, 6, 27, 1, 2, 3, 0, time.UTC) }

	a, err := client.PresignGetURLWithParams("a/b.webp", time.Minute, url.Values{"imageMogr2/format/webp": []string{""}})
	if err != nil {
		t.Fatalf("presign a: %v", err)
	}
	b, err := client.PresignGetURLWithParams("a/b.webp", time.Minute, url.Values{"imageMogr2/thumbnail/!30p": []string{""}})
	if err != nil {
		t.Fatalf("presign b: %v", err)
	}
	sigA, _ := url.Parse(a)
	sigB, _ := url.Parse(b)
	if sigA.Query().Get("q-signature") == sigB.Query().Get("q-signature") {
		t.Fatalf("expected different signatures for different process params")
	}
}
