package aireport

import (
	"context"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	DefaultDeliveryTimeText = "研报生成时间通常为 10 分钟到 24 小时不等，大部分情况下会在 1 小时内完成交付。具体时间取决于股票复杂度、数据完整度和人工复核情况。"
	DefaultRiskDisclaimer   = "AI研报内容包含对个股的研究分析和投资建议，仅供投资研究参考，不构成收益承诺。证券市场存在风险，投资者应结合自身风险承受能力独立判断并审慎决策。"
)

// DefaultPreviewURLTTL 为预览/缩略图签名 URL 的默认有效期。
const DefaultPreviewURLTTL = 15 * time.Minute

// 数据万象（CI）图片处理参数：预览图走极智压缩 imageSlim（保持原格式、仅压缩体积），
// 缩略图缩放到 30%。这些参数会作为 query 原样附加到 COS 对象 URL 上。
const (
	previewImageProcess   = "imageSlim"
	thumbnailImageProcess = "imageMogr2/thumbnail/!30p"
)

// ImageURLSigner 生成 COS 对象的带签名临时 GET URL。
// 由上层（main.go）使用现有 COS 密钥配置注入具体实现，避免本包直接依赖 backup 包。
// process 为数据万象图片处理参数（如 imageMogr2/format/webp），空字符串表示不处理。
type ImageURLSigner interface {
	PresignGetURLWithProcess(objectKey string, expire time.Duration, process string) (string, error)
}

// imageVariant 表示一类图片用途，决定附加的数据万象处理参数。
type imageVariant int

const (
	variantOriginal imageVariant = iota
	variantPreview
	variantThumbnail
)

// processParam 返回该图片用途对应的数据万象处理参数（无则返回空串）。
func (v imageVariant) processParam() string {
	switch v {
	case variantPreview:
		return previewImageProcess
	case variantThumbnail:
		return thumbnailImageProcess
	default:
		return ""
	}
}

type ServiceConfig struct {
	COSBucket     string
	COSRegion     string
	PreviewURLTTL time.Duration
}

type Service struct {
	repo   *Repository
	cfg    ServiceConfig
	signer ImageURLSigner
	now    func() time.Time
}

func NewService(repo *Repository, cfg ServiceConfig) *Service {
	if cfg.PreviewURLTTL <= 0 {
		cfg.PreviewURLTTL = DefaultPreviewURLTTL
	}
	return &Service{repo: repo, cfg: cfg, now: time.Now}
}

// WithImageURLSigner 注入 COS 预签名器。注入后，纯对象 Key 会被解析为带签名的临时 URL。
func (s *Service) WithImageURLSigner(signer ImageURLSigner) *Service {
	s.signer = signer
	return s
}

func (s *Service) ListPublicReports(ctx context.Context) ([]ReportListItem, error) {
	records, err := s.repo.ListReports(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]ReportListItem, 0, len(records))
	for _, record := range records {
		items = append(items, ReportListItem{
			ID:              record.ID,
			StockName:       record.StockName,
			Symbol:          record.Symbol,
			Exchange:        record.Exchange,
			SourceTradeDate: record.SourceTradeDate,
			ThumbnailURL:    s.resolveImageURL(record.ImageThumbnailKey, variantThumbnail),
		})
	}
	return items, nil
}

func (s *Service) GetPreview(ctx context.Context, id string) (*ReportPreview, error) {
	record, err := s.repo.GetReport(ctx, strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}
	return &ReportPreview{
		ID:              record.ID,
		StockName:       record.StockName,
		Symbol:          record.Symbol,
		Exchange:        record.Exchange,
		SourceTradeDate: record.SourceTradeDate,
		PreviewURL:      s.resolveImageURL(record.ImagePreviewKey, variantPreview),
	}, nil
}

func (s *Service) ListAdminReports(ctx context.Context) ([]AdminReportItem, error) {
	records, err := s.repo.ListReports(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]AdminReportItem, 0, len(records))
	for _, record := range records {
		items = append(items, s.adminView(record))
	}
	return items, nil
}

func (s *Service) CreateReport(ctx context.Context, input SaveReportInput) (*AdminReportItem, error) {
	cleaned, err := normalizeReportInput(input)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	record := ResearchReportRecord{
		ID:                uuid.NewString(),
		StockName:         cleaned.StockName,
		Symbol:            cleaned.Symbol,
		Exchange:          cleaned.Exchange,
		SourceTradeDate:   cleaned.SourceTradeDate,
		ImageOriginalKey:  cleaned.ImageOriginalKey,
		ImagePreviewKey:   cleaned.ImagePreviewKey,
		ImageThumbnailKey: cleaned.ImageThumbnailKey,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := s.repo.CreateReport(ctx, record); err != nil {
		return nil, err
	}
	item := s.adminView(record)
	return &item, nil
}

func (s *Service) UpdateReport(ctx context.Context, id string, input SaveReportInput) (*AdminReportItem, error) {
	cleaned, err := normalizeReportInput(input)
	if err != nil {
		return nil, err
	}
	record, err := s.repo.GetReport(ctx, strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}
	record.StockName = cleaned.StockName
	record.Symbol = cleaned.Symbol
	record.Exchange = cleaned.Exchange
	record.SourceTradeDate = cleaned.SourceTradeDate
	record.ImageOriginalKey = cleaned.ImageOriginalKey
	record.ImagePreviewKey = cleaned.ImagePreviewKey
	record.ImageThumbnailKey = cleaned.ImageThumbnailKey
	record.UpdatedAt = s.now().UTC()
	if err := s.repo.UpdateReport(ctx, *record); err != nil {
		return nil, err
	}
	item := s.adminView(*record)
	return &item, nil
}

func (s *Service) DeleteReport(ctx context.Context, id string) error {
	return s.repo.DeleteReport(ctx, strings.TrimSpace(id))
}

func (s *Service) GetServiceConfig(ctx context.Context) (*ServiceConfigView, error) {
	record, err := s.repo.GetServiceConfig(ctx)
	if err != nil {
		if errors.Is(err, ErrReportNotFound) {
			return s.defaultServiceConfigView(), nil
		}
		return nil, err
	}
	return s.serviceConfigView(*record), nil
}

func (s *Service) SaveServiceConfig(ctx context.Context, input SaveServiceConfigInput) (*ServiceConfigView, error) {
	wechatID := strings.TrimSpace(input.WechatID)
	qrKey := strings.TrimSpace(input.WechatQRImageKey)
	deliveryText := strings.TrimSpace(input.DeliveryTimeText)
	if deliveryText == "" {
		deliveryText = DefaultDeliveryTimeText
	}
	riskDisclaimer := strings.TrimSpace(input.RiskDisclaimer)
	if riskDisclaimer == "" {
		riskDisclaimer = DefaultRiskDisclaimer
	}

	now := s.now().UTC()
	record, err := s.repo.GetServiceConfig(ctx)
	if err != nil && !errors.Is(err, ErrReportNotFound) {
		return nil, err
	}
	if record == nil {
		record = &ServiceConfigRecord{ID: uuid.NewString(), CreatedAt: now}
	}
	record.WechatID = wechatID
	record.WechatQRImageKey = qrKey
	record.DeliveryTimeText = deliveryText
	record.RiskDisclaimer = riskDisclaimer
	record.UpdatedAt = now
	if err := s.repo.SaveServiceConfig(ctx, *record); err != nil {
		return nil, err
	}
	return s.serviceConfigView(*record), nil
}

func (s *Service) adminView(record ResearchReportRecord) AdminReportItem {
	return AdminReportItem{
		ID:                record.ID,
		StockName:         record.StockName,
		Symbol:            record.Symbol,
		Exchange:          record.Exchange,
		SourceTradeDate:   record.SourceTradeDate,
		ImageOriginalKey:  record.ImageOriginalKey,
		ImagePreviewKey:   record.ImagePreviewKey,
		ImageThumbnailKey: record.ImageThumbnailKey,
		OriginalURL:       s.resolveImageURL(record.ImageOriginalKey, variantOriginal),
		PreviewURL:        s.resolveImageURL(record.ImagePreviewKey, variantPreview),
		ThumbnailURL:      s.resolveImageURL(record.ImageThumbnailKey, variantThumbnail),
		CreatedAt:         formatTime(record.CreatedAt),
		UpdatedAt:         formatTime(record.UpdatedAt),
	}
}

func (s *Service) defaultServiceConfigView() *ServiceConfigView {
	return &ServiceConfigView{
		DeliveryTimeText: DefaultDeliveryTimeText,
		RiskDisclaimer:   DefaultRiskDisclaimer,
	}
}

func (s *Service) serviceConfigView(record ServiceConfigRecord) *ServiceConfigView {
	return &ServiceConfigView{
		WechatID:         record.WechatID,
		WechatQRImageKey: record.WechatQRImageKey,
		WechatQRImageURL: s.resolveImageURL(record.WechatQRImageKey, variantOriginal),
		DeliveryTimeText: record.DeliveryTimeText,
		RiskDisclaimer:   record.RiskDisclaimer,
		UpdatedAt:        formatTime(record.UpdatedAt),
	}
}

func (s *Service) resolveImageURL(key string, variant imageVariant) string {
	text := strings.TrimSpace(key)
	if text == "" {
		return ""
	}
	// 已经是完整 URL（运营手填的外链）或站内绝对路径时原样返回，不再追加处理参数。
	if parsed, err := url.Parse(text); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		return text
	}
	if strings.HasPrefix(text, "/") {
		return text
	}
	objectKey := strings.TrimLeft(text, "/")
	process := variant.processParam()
	// 优先生成带签名的临时 URL；失败时回退到未签名公开直链，保证旧行为不被破坏。
	if s.signer != nil {
		if signed, err := s.signer.PresignGetURLWithProcess(objectKey, s.cfg.PreviewURLTTL, process); err == nil && signed != "" {
			return signed
		}
	}
	bucket := strings.TrimSpace(s.cfg.COSBucket)
	region := strings.TrimSpace(s.cfg.COSRegion)
	if bucket == "" || region == "" {
		return text
	}
	publicURL := "https://" + bucket + ".cos." + region + ".myqcloud.com/" + objectKey
	if process != "" {
		publicURL += "?" + process
	}
	return publicURL
}

var tradeDatePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

func normalizeReportInput(input SaveReportInput) (SaveReportInput, error) {
	cleaned := SaveReportInput{
		StockName:         strings.TrimSpace(input.StockName),
		Symbol:            strings.ToUpper(strings.TrimSpace(input.Symbol)),
		Exchange:          strings.ToUpper(strings.TrimSpace(input.Exchange)),
		SourceTradeDate:   strings.TrimSpace(input.SourceTradeDate),
		ImageOriginalKey:  strings.TrimSpace(input.ImageOriginalKey),
		ImagePreviewKey:   strings.TrimSpace(input.ImagePreviewKey),
		ImageThumbnailKey: strings.TrimSpace(input.ImageThumbnailKey),
	}
	if cleaned.StockName == "" || cleaned.Symbol == "" || cleaned.SourceTradeDate == "" || cleaned.ImageOriginalKey == "" || cleaned.ImagePreviewKey == "" || cleaned.ImageThumbnailKey == "" {
		return cleaned, ErrInvalidInput
	}
	if cleaned.Exchange != "SSE" && cleaned.Exchange != "SZSE" && cleaned.Exchange != "HKEX" {
		return cleaned, ErrInvalidInput
	}
	if !tradeDatePattern.MatchString(cleaned.SourceTradeDate) {
		return cleaned, ErrInvalidInput
	}
	if _, err := time.Parse("2006-01-02", cleaned.SourceTradeDate); err != nil {
		return cleaned, ErrInvalidInput
	}
	return cleaned, nil
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format("2006-01-02T15:04:05Z")
}
