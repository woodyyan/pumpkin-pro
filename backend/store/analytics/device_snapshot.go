package analytics

import "time"

type DeviceSnapshot struct {
	ID              uint64    `gorm:"primaryKey;autoIncrement"`
	UserID          string    `gorm:"size:36;index"`
	VisitorID       string    `gorm:"size:64;index"`
	Source          string    `gorm:"size:32;not null"` // 'page_view' | 'auth' | 'api_error'
	SourceID        string    `gorm:"size:64"`
	DeviceType      string    `gorm:"size:16"` // desktop | mobile | tablet | unknown
	OSFamily        string    `gorm:"size:16;index"` // macOS | Windows | iOS | Android | Linux | unknown
	OSVersion       string    `gorm:"size:16"`
	BrowserFamily   string    `gorm:"size:16;index"` // Chrome | Safari | Edge | Firefox | WeChat | WeCom | DingTalk | unknown
	BrowserVersion  string    `gorm:"size:16"`
	RawUserAgent    string    `gorm:"size:512"`
	CreatedAt       time.Time `gorm:"not null;index"`
}

func (DeviceSnapshot) TableName() string {
	return "device_snapshots"
}
