package webui

import "strings"

// uiStrings are the server-rendered fallback texts.
type uiStrings struct {
	Loading       string
	NoScript      string
	MissingBuild  string
	PrivacyTitle  string
	NotFoundTitle string
	NotFoundLede  string
	BackHome      string
	AdminTitle    string
	AdminNoScript string
}

var uiText = map[string]uiStrings{
	"zh": {
		Loading:       "正在加载…",
		NoScript:      "请启用 JavaScript 以查看完整内容。",
		MissingBuild:  "前端资源尚未构建（运行 make web-dist）。",
		PrivacyTitle:  "隐私政策",
		NotFoundTitle: "页面不存在",
		NotFoundLede:  "你访问的地址不存在或已被移除。",
		BackHome:      "返回首页 →",
		AdminTitle:    "LinksPage 管理后台",
		AdminNoScript: "管理后台需要启用 JavaScript。",
	},
	"en": {
		Loading:       "Loading…",
		NoScript:      "Enable JavaScript to see the full page.",
		MissingBuild:  "Frontend assets are not built (run make web-dist).",
		PrivacyTitle:  "Privacy",
		NotFoundTitle: "Page not found",
		NotFoundLede:  "The page you are looking for does not exist or was removed.",
		BackHome:      "Back to home →",
		AdminTitle:    "LinksPage Admin",
		AdminNoScript: "The admin console requires JavaScript.",
	},
}

// textFor picks strings by primary language subtag, defaulting to English.
func textFor(locale string) uiStrings {
	primary, _, _ := strings.Cut(strings.ToLower(locale), "-")
	if t, ok := uiText[primary]; ok {
		return t
	}
	return uiText["en"]
}

// ogLocale converts a BCP-47 tag (zh-CN) to the OG form (zh_CN).
func ogLocale(locale string) string {
	return strings.ReplaceAll(locale, "-", "_")
}
