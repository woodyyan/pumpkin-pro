package aireport

import "time"

type ResearchReportRecord struct {
	ID                string    `gorm:"primaryKey;size:36" json:"id"`
	StockName         string    `gorm:"size:128;not null" json:"stock_name"`
	Symbol            string    `gorm:"size:32;not null;index" json:"symbol"`
	Exchange          string    `gorm:"size:16;not null;index" json:"exchange"`
	SourceTradeDate   string    `gorm:"size:10;not null;index" json:"source_trade_date"`
	ImageOriginalKey  string    `gorm:"size:1024;not null" json:"image_original_key"`
	ImagePreviewKey   string    `gorm:"size:1024;not null" json:"image_preview_key"`
	ImageThumbnailKey string    `gorm:"size:1024;not null" json:"image_thumbnail_key"`
	CreatedAt         time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt         time.Time `gorm:"not null" json:"updated_at"`
}

func (ResearchReportRecord) TableName() string {
	return "ai_research_reports"
}

type ServiceConfigRecord struct {
	ID               string    `gorm:"primaryKey;size:36" json:"id"`
	WechatID         string    `gorm:"size:128;not null;default:''" json:"wechat_id"`
	WechatQRImageKey string    `gorm:"size:1024;not null;default:''" json:"wechat_qr_image_key"`
	DeliveryTimeText string    `gorm:"type:text;not null" json:"delivery_time_text"`
	RiskDisclaimer   string    `gorm:"type:text;not null" json:"risk_disclaimer"`
	CreatedAt        time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt        time.Time `gorm:"not null" json:"updated_at"`
}

func (ServiceConfigRecord) TableName() string {
	return "ai_report_service_config"
}

type ReportListItem struct {
	ID              string `json:"id"`
	StockName       string `json:"stock_name"`
	Symbol          string `json:"symbol"`
	Exchange        string `json:"exchange"`
	SourceTradeDate string `json:"source_trade_date"`
	ThumbnailURL    string `json:"thumbnail_url"`
	CreatedAt       string `json:"created_at,omitempty"`
	UpdatedAt       string `json:"updated_at,omitempty"`
}

type ReportPreview struct {
	ID              string `json:"id"`
	StockName       string `json:"stock_name"`
	Symbol          string `json:"symbol"`
	Exchange        string `json:"exchange"`
	SourceTradeDate string `json:"source_trade_date"`
	PreviewURL      string `json:"preview_url"`
}

type AdminReportItem struct {
	ID                string `json:"id"`
	StockName         string `json:"stock_name"`
	Symbol            string `json:"symbol"`
	Exchange          string `json:"exchange"`
	SourceTradeDate   string `json:"source_trade_date"`
	ImageOriginalKey  string `json:"image_original_key"`
	ImagePreviewKey   string `json:"image_preview_key"`
	ImageThumbnailKey string `json:"image_thumbnail_key"`
	OriginalURL       string `json:"original_url"`
	PreviewURL        string `json:"preview_url"`
	ThumbnailURL      string `json:"thumbnail_url"`
	CreatedAt         string `json:"created_at"`
	UpdatedAt         string `json:"updated_at"`
}

type SaveReportInput struct {
	StockName         string `json:"stock_name"`
	Symbol            string `json:"symbol"`
	Exchange          string `json:"exchange"`
	SourceTradeDate   string `json:"source_trade_date"`
	ImageOriginalKey  string `json:"image_original_key"`
	ImagePreviewKey   string `json:"image_preview_key"`
	ImageThumbnailKey string `json:"image_thumbnail_key"`
}

type ServiceConfigView struct {
	WechatID         string `json:"wechat_id"`
	WechatQRImageKey string `json:"wechat_qr_image_key"`
	WechatQRImageURL string `json:"wechat_qr_image_url"`
	DeliveryTimeText string `json:"delivery_time_text"`
	RiskDisclaimer   string `json:"risk_disclaimer"`
	UpdatedAt        string `json:"updated_at,omitempty"`
}

type SaveServiceConfigInput struct {
	WechatID         string `json:"wechat_id"`
	WechatQRImageKey string `json:"wechat_qr_image_key"`
	DeliveryTimeText string `json:"delivery_time_text"`
	RiskDisclaimer   string `json:"risk_disclaimer"`
}
