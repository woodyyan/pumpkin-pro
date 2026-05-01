package analytics

import (
	"strings"

	"github.com/mileusna/useragent"
)

// ParsedDeviceInfo holds structured UA parsing results.
type ParsedDeviceInfo struct {
	DeviceType     string
	OSFamily       string
	OSVersion      string
	BrowserFamily  string
	BrowserVersion string
}

// ParseUserAgent parses a raw UA string into structured device info.
// It handles special cases for WeChat, WeCom, DingTalk before falling back to the parser library.
func ParseUserAgent(raw string) ParsedDeviceInfo {
	ua := strings.TrimSpace(raw)
	if ua == "" {
		return ParsedDeviceInfo{
			DeviceType:    "unknown",
			OSFamily:      "unknown",
			BrowserFamily: "unknown",
		}
	}

	parsed := useragent.Parse(ua)

	// Filter out crawlers/bots using the library's built-in detection
	if parsed.Bot {
		return ParsedDeviceInfo{
			DeviceType:    "unknown",
			OSFamily:      "unknown",
			BrowserFamily: "unknown",
		}
	}

	info := ParsedDeviceInfo{
		DeviceType:    normalizeDeviceType(parsed),
		OSFamily:      normalizeOSFamily(parsed),
		OSVersion:     extractMajorVersion(parsed.OSVersion),
		BrowserFamily: normalizeBrowserFamily(ua, parsed),
	}
	info.BrowserVersion = extractMajorVersion(parsed.Version)

	return info
}

func normalizeDeviceType(ua useragent.UserAgent) string {
	switch {
	case ua.Tablet:
		return "tablet"
	case ua.Mobile:
		return "mobile"
	case ua.Desktop:
		return "desktop"
	default:
		return "unknown"
	}
}

func normalizeOSFamily(ua useragent.UserAgent) string {
	switch {
	case ua.IsMacOS():
		return "macOS"
	case ua.IsWindows():
		return "Windows"
	case ua.IsIOS():
		return "iOS"
	case ua.IsAndroid():
		return "Android"
	case ua.IsLinux():
		return "Linux"
	default:
		return "unknown"
	}
}

func normalizeBrowserFamily(uaString string, ua useragent.UserAgent) string {
	// Check special Chinese app browsers first (they embed system WebView)
	if strings.Contains(uaString, "MicroMessenger") {
		return "WeChat"
	}
	if strings.Contains(uaString, "wxwork") {
		return "WeCom"
	}
	if strings.Contains(uaString, "DingTalk") {
		return "DingTalk"
	}

	if ua.Name != "" {
		return ua.Name
	}
	return "unknown"
}

func extractMajorVersion(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	parts := strings.SplitN(v, ".", 2)
	return parts[0]
}
