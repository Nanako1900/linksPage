// Package uaclass classifies visitor user agents for the fixed in-app
// browser rules of doc 5.7. The same rules are mirrored by the frontend
// (web/src/shared/ua.ts); both table tests read the same UA samples
// (web/src/test/fixtures/user-agents.json).
//
// The rules are plain, case-sensitive substring checks so that the Go
// and TypeScript implementations cannot drift:
//
//	InWeChat: contains MarkerWeChat ("MicroMessenger")
//	InQQ:     contains MarkerQQ ("QQ/"); "MQQBrowser" alone is the
//	          standalone QQ Browser and is a normal browser
//	Mobile:   contains any of MobileMarkers
package uaclass

import "strings"

// Detection markers (exported so tests and docs reference one source).
const (
	// MarkerWeChat identifies WeChat WebViews (phone, tablet, desktop and
	// WeCom, which embeds the same token).
	MarkerWeChat = "MicroMessenger"
	// MarkerQQ identifies the QQ app WebView ("QQ/<version>"). It never
	// matches "MQQBrowser/" or "QQBrowser/" (the standalone browser).
	MarkerQQ = "QQ/"
)

// mobileMarkers are the substrings that mark a phone or tablet UA.
// iPadOS Safari in desktop mode sends a macOS UA and is classified as
// desktop; that is acceptable because desktop handling is a plain redirect.
var mobileMarkers = []string{"Mobi", "Android", "iPhone", "iPad", "iPod", "OpenHarmony"}

// MobileMarkers returns a copy of the mobile markers.
func MobileMarkers() []string {
	return append([]string(nil), mobileMarkers...)
}

// Class is the classification of one User-Agent string.
type Class struct {
	// InWeChat: the UA contains "MicroMessenger".
	InWeChat bool
	// InQQ: the UA contains "QQ/" (QQ app WebView). A UA with only
	// "MQQBrowser" is the standalone QQ Browser and is NOT InQQ.
	InQQ bool
	// Mobile: "Mobi", "Android", "iPhone", "iPad", "iPod" or "OpenHarmony".
	Mobile bool
}

// InApp reports whether the visitor is inside WeChat or QQ.
func (c Class) InApp() bool { return c.InWeChat || c.InQQ }

// Classify classifies userAgent. It never fails; an empty UA is a desktop
// browser outside any app.
func Classify(userAgent string) Class {
	return Class{
		InWeChat: strings.Contains(userAgent, MarkerWeChat),
		InQQ:     strings.Contains(userAgent, MarkerQQ),
		Mobile:   containsAny(userAgent, mobileMarkers),
	}
}

func containsAny(s string, subs []string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
