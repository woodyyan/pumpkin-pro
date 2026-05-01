package analytics

import (
	"testing"
)

func TestParseUserAgent(t *testing.T) {
	tests := []struct {
		name           string
		ua             string
		wantDeviceType string
		wantOSFamily   string
		wantBrowser    string
	}{
		{
			name:           "Chrome on macOS",
			ua:             "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.6367.60 Safari/537.36",
			wantDeviceType: "desktop",
			wantOSFamily:   "macOS",
			wantBrowser:    "Chrome",
		},
		{
			name:           "Safari on iOS",
			ua:             "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1",
			wantDeviceType: "mobile",
			wantOSFamily:   "iOS",
			wantBrowser:    "Safari",
		},
		{
			name:           "Edge on Windows",
			ua:             "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36 Edg/123.0.0.0",
			wantDeviceType: "desktop",
			wantOSFamily:   "Windows",
			wantBrowser:    "Edge",
		},
		{
			name:           "Firefox on Linux",
			ua:             "Mozilla/5.0 (X11; Linux x86_64; rv:125.0) Gecko/20100101 Firefox/125.0",
			wantDeviceType: "desktop",
			wantOSFamily:   "Linux",
			wantBrowser:    "Firefox",
		},
		{
			name:           "WeChat on iOS",
			ua:             "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148 MicroMessenger/8.0.38(0x18002628) NetType/WIFI Language/zh_CN",
			wantDeviceType: "mobile",
			wantOSFamily:   "iOS",
			wantBrowser:    "WeChat",
		},
		{
			name:           "WeChat on Android",
			ua:             "Mozilla/5.0 (Linux; Android 14; Pixel 7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Mobile Safari/537.36 MicroMessenger/8.0.38",
			wantDeviceType: "mobile",
			wantOSFamily:   "Android",
			wantBrowser:    "WeChat",
		},
		{
			name:           "WeCom",
			ua:             "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148 wxwork/4.1.0",
			wantDeviceType: "mobile",
			wantOSFamily:   "iOS",
			wantBrowser:    "WeCom",
		},
		{
			name:           "DingTalk",
			ua:             "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148 DingTalk/7.0.0",
			wantDeviceType: "mobile",
			wantOSFamily:   "iOS",
			wantBrowser:    "DingTalk",
		},
		{
			name:           "Empty UA",
			ua:             "",
			wantDeviceType: "unknown",
			wantOSFamily:   "unknown",
			wantBrowser:    "unknown",
		},
		{
			name:           "Whitespace only",
			ua:             "   ",
			wantDeviceType: "unknown",
			wantOSFamily:   "unknown",
			wantBrowser:    "unknown",
		},
		{
			name:           "Googlebot",
			ua:             "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)",
			wantDeviceType: "unknown",
			wantOSFamily:   "unknown",
			wantBrowser:    "unknown",
		},
		{
			name:           "Baiduspider",
			ua:             "Mozilla/5.0 (compatible; Baiduspider/2.0; +http://www.baidu.com/search/spider.html)",
			wantDeviceType: "unknown",
			wantOSFamily:   "unknown",
			wantBrowser:    "unknown",
		},
		{
			name:           "Headless Chrome",
			ua:             "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) HeadlessChrome/120.0.0.0 Safari/537.36",
			wantDeviceType: "unknown",
			wantOSFamily:   "unknown",
			wantBrowser:    "unknown",
		},
		{
			name:           "iPad tablet",
			ua:             "Mozilla/5.0 (iPad; CPU OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1",
			wantDeviceType: "tablet",
			wantOSFamily:   "iOS",
			wantBrowser:    "Safari",
		},
		{
			name:           "Android mobile",
			ua:             "Mozilla/5.0 (Linux; Android 14; SM-S918B) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Mobile Safari/537.36",
			wantDeviceType: "mobile",
			wantOSFamily:   "Android",
			wantBrowser:    "Chrome",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseUserAgent(tt.ua)
			if got.DeviceType != tt.wantDeviceType {
				t.Errorf("DeviceType = %q, want %q", got.DeviceType, tt.wantDeviceType)
			}
			if got.OSFamily != tt.wantOSFamily {
				t.Errorf("OSFamily = %q, want %q", got.OSFamily, tt.wantOSFamily)
			}
			if got.BrowserFamily != tt.wantBrowser {
				t.Errorf("BrowserFamily = %q, want %q", got.BrowserFamily, tt.wantBrowser)
			}
		})
	}
}

func TestExtractMajorVersion(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"124.0.6367.60", "124"},
		{"17.0.1", "17"},
		{"123", "123"},
		{"", ""},
		{"  ", ""},
		{"abc.def", "abc"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := extractMajorVersion(tt.input)
			if got != tt.want {
				t.Errorf("extractMajorVersion(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
