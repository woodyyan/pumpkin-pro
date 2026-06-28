package aireport

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/woodyyan/pumpkin-pro/backend/tests/testutil"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	db := testutil.InMemoryDB(t)
	if err := NewMigrator().AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	svc := NewService(NewRepository(db), ServiceConfig{COSBucket: "bucket-123", COSRegion: "ap-guangzhou"})
	svc.now = func() time.Time { return time.Date(2026, 6, 27, 1, 2, 3, 0, time.UTC) }
	return svc
}

func TestCreateAndPreviewReport(t *testing.T) {
	svc := newTestService(t)
	item, err := svc.CreateReport(context.Background(), SaveReportInput{
		StockName:         "腾讯控股",
		Symbol:            "00700",
		Exchange:          "HKEX",
		SourceTradeDate:   "2026-06-26",
		ImageOriginalKey:  "ai-reports/original/2026/tencent.png",
		ImagePreviewKey:   "ai-reports/preview/2026/tencent.webp",
		ImageThumbnailKey: "ai-reports/thumb/2026/tencent.webp",
	})
	if err != nil {
		t.Fatalf("create report: %v", err)
	}
	if item.ID == "" {
		t.Fatalf("expected generated id")
	}
	if item.Exchange != "HKEX" || item.Symbol != "00700" {
		t.Fatalf("unexpected normalized fields: %+v", item)
	}
	if item.PreviewURL != "https://bucket-123.cos.ap-guangzhou.myqcloud.com/ai-reports/preview/2026/tencent.webp?imageSlim" {
		t.Fatalf("unexpected preview url: %s", item.PreviewURL)
	}
	if item.ThumbnailURL != "https://bucket-123.cos.ap-guangzhou.myqcloud.com/ai-reports/thumb/2026/tencent.webp?imageMogr2/thumbnail/!30p" {
		t.Fatalf("unexpected thumbnail url: %s", item.ThumbnailURL)
	}
	if item.OriginalURL != "https://bucket-123.cos.ap-guangzhou.myqcloud.com/ai-reports/original/2026/tencent.png" {
		t.Fatalf("unexpected original url (should have no process params): %s", item.OriginalURL)
	}

	publicItems, err := svc.ListPublicReports(context.Background())
	if err != nil {
		t.Fatalf("list public: %v", err)
	}
	if len(publicItems) != 1 {
		t.Fatalf("expected one public report, got %d", len(publicItems))
	}
	if publicItems[0].ThumbnailURL == "" {
		t.Fatalf("expected thumbnail url")
	}

	preview, err := svc.GetPreview(context.Background(), item.ID)
	if err != nil {
		t.Fatalf("get preview: %v", err)
	}
	if preview.PreviewURL != item.PreviewURL {
		t.Fatalf("unexpected preview payload: %+v", preview)
	}
}

func TestCreateReportRejectsInvalidInput(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.CreateReport(context.Background(), SaveReportInput{
		StockName:         "贵州茅台",
		Symbol:            "600519",
		Exchange:          "NYSE",
		SourceTradeDate:   "2026-06-26",
		ImageOriginalKey:  "original.png",
		ImagePreviewKey:   "preview.webp",
		ImageThumbnailKey: "thumb.webp",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestServiceConfigDefaultsAndSave(t *testing.T) {
	svc := newTestService(t)
	view, err := svc.GetServiceConfig(context.Background())
	if err != nil {
		t.Fatalf("get default config: %v", err)
	}
	if view.DeliveryTimeText != DefaultDeliveryTimeText || view.RiskDisclaimer != DefaultRiskDisclaimer {
		t.Fatalf("unexpected defaults: %+v", view)
	}

	saved, err := svc.SaveServiceConfig(context.Background(), SaveServiceConfigInput{
		WechatID:         "wolong-ai",
		WechatQRImageKey: "ai-reports/service/wechat.png",
	})
	if err != nil {
		t.Fatalf("save config: %v", err)
	}
	if saved.WechatID != "wolong-ai" {
		t.Fatalf("unexpected wechat id: %+v", saved)
	}
	if saved.WechatQRImageURL != "https://bucket-123.cos.ap-guangzhou.myqcloud.com/ai-reports/service/wechat.png" {
		t.Fatalf("unexpected qr url: %s", saved.WechatQRImageURL)
	}
	if saved.RiskDisclaimer == "" || saved.DeliveryTimeText == "" {
		t.Fatalf("expected default texts to be preserved: %+v", saved)
	}
}

type fakeSigner struct {
	expire      time.Duration
	lastKey     string
	failOn      string
	callKeys    []string
	lastProcess string
}

func (f *fakeSigner) PresignGetURLWithProcess(objectKey string, expire time.Duration, process string) (string, error) {
	f.lastKey = objectKey
	f.expire = expire
	f.lastProcess = process
	f.callKeys = append(f.callKeys, objectKey)
	if f.failOn != "" && objectKey == f.failOn {
		return "", errors.New("forced signer failure")
	}
	signed := "https://signed.example.com/" + objectKey + "?q-signature=test"
	if strings.TrimSpace(process) != "" {
		signed += "&" + process
	}
	return signed, nil
}

func TestPreviewUsesSignedURLWhenSignerPresent(t *testing.T) {
	svc := newTestService(t)
	signer := &fakeSigner{}
	svc.WithImageURLSigner(signer)

	item, err := svc.CreateReport(context.Background(), SaveReportInput{
		StockName:         "宁德时代",
		Symbol:            "300750",
		Exchange:          "SZSE",
		SourceTradeDate:   "2026-06-26",
		ImageOriginalKey:  "ai-reports/original/2026/catl.png",
		ImagePreviewKey:   "ai-reports/preview/2026/catl.webp",
		ImageThumbnailKey: "ai-reports/thumb/2026/catl.webp",
	})
	if err != nil {
		t.Fatalf("create report: %v", err)
	}

	preview, err := svc.GetPreview(context.Background(), item.ID)
	if err != nil {
		t.Fatalf("get preview: %v", err)
	}
	want := "https://signed.example.com/ai-reports/preview/2026/catl.webp?q-signature=test&imageSlim"
	if preview.PreviewURL != want {
		t.Fatalf("expected signed preview url with imageSlim param, got %s", preview.PreviewURL)
	}
	if signer.expire != DefaultPreviewURLTTL {
		t.Fatalf("expected default ttl %v, got %v", DefaultPreviewURLTTL, signer.expire)
	}
	// 预览必须把图片处理参数透传给签名器。
	if signer.lastProcess != previewImageProcess {
		t.Fatalf("expected process %s passed to signer, got %q", previewImageProcess, signer.lastProcess)
	}

	publicItems, err := svc.ListPublicReports(context.Background())
	if err != nil {
		t.Fatalf("list public: %v", err)
	}
	wantThumb := "https://signed.example.com/ai-reports/thumb/2026/catl.webp?q-signature=test&imageMogr2/thumbnail/!30p"
	if len(publicItems) != 1 || publicItems[0].ThumbnailURL != wantThumb {
		t.Fatalf("expected signed thumbnail url with thumbnail param, got %+v", publicItems)
	}
}


func TestPreviewFallsBackToPublicURLWhenSignerFails(t *testing.T) {
	svc := newTestService(t)
	signer := &fakeSigner{failOn: "ai-reports/preview/2026/catl.webp"}
	svc.WithImageURLSigner(signer)

	item, err := svc.CreateReport(context.Background(), SaveReportInput{
		StockName:         "宁德时代",
		Symbol:            "300750",
		Exchange:          "SZSE",
		SourceTradeDate:   "2026-06-26",
		ImageOriginalKey:  "ai-reports/original/2026/catl.png",
		ImagePreviewKey:   "ai-reports/preview/2026/catl.webp",
		ImageThumbnailKey: "ai-reports/thumb/2026/catl.webp",
	})
	if err != nil {
		t.Fatalf("create report: %v", err)
	}

	preview, err := svc.GetPreview(context.Background(), item.ID)
	if err != nil {
		t.Fatalf("get preview: %v", err)
	}
	want := "https://bucket-123.cos.ap-guangzhou.myqcloud.com/ai-reports/preview/2026/catl.webp?imageSlim"
	if preview.PreviewURL != want {
		t.Fatalf("expected fallback public url with process param, got %s", preview.PreviewURL)
	}
}

func TestResolveImageURLKeepsFullURLAndEmpty(t *testing.T) {
	svc := newTestService(t)
	svc.WithImageURLSigner(&fakeSigner{})

	if got := svc.resolveImageURL("", variantPreview); got != "" {
		t.Fatalf("expected empty url for empty key, got %q", got)
	}
	// 已是完整 URL 时原样返回，不追加处理参数。
	full := "https://cdn.example.com/a/b.png"
	if got := svc.resolveImageURL(full, variantThumbnail); got != full {
		t.Fatalf("expected full url returned as-is, got %q", got)
	}
}
