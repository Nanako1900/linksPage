package webui

import (
	"strings"

	"github.com/Nanako1900/linksPage/internal/site"
)

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

	// Community card texts.
	Join            string
	JoinGroup       string
	OnlineMembers   string // fmt: online, members
	OnlineOnly      string // fmt: online
	MembersOnly     string // fmt: members
	UpdatedAt       string // fmt: UTC time
	Pending         string
	GroupNumber     string
	QRAlt           string // fmt: community name
	QRHint          string
	OnlineList      string
	ListSeparator   string
	MoreUsers       string // fmt: count
	PoweredBy       string
	CopyDefaults    map[string]string
	CommunityPinned string
}

const poweredByURL = "https://github.com/Nanako1900/linksPage"

var uiText = map[string]uiStrings{
	"zh": {
		Loading:       "正在加载…",
		NoScript:      "启用 JavaScript 后可查看实时在线人数和更多操作。",
		MissingBuild:  "前端资源尚未构建（运行 make web-dist）。",
		PrivacyTitle:  "隐私政策",
		NotFoundTitle: "页面不存在",
		NotFoundLede:  "你访问的地址不存在或已被移除。",
		BackHome:      "返回首页 →",
		AdminTitle:    "LinksPage 管理后台",
		AdminNoScript: "管理后台需要启用 JavaScript。",

		Join:          "加入 →",
		JoinGroup:     "加群 →",
		OnlineMembers: "● %d 在线 · %d 成员",
		OnlineOnly:    "● %d 在线",
		MembersOnly:   "%d 成员",
		UpdatedAt:     "数据更新于 %s",
		Pending:       "正在获取最新数据…",
		GroupNumber:   "群号",
		QRAlt:         "%s 二维码",
		QRHint:        "长按或扫码加入",
		OnlineList:    "在线：",
		ListSeparator: "、",
		MoreUsers:     " 等 %d 人",
		PoweredBy:     "由 LinksPage 驱动",
		CopyDefaults: map[string]string{
			site.CopyInviteUnavailable:    "邀请暂不可用，请稍后再试。",
			site.CopyCommunityUnavailable: "该社区暂不可用。",
		},
		CommunityPinned: "分享的社区",
	},
	"en": {
		Loading:       "Loading…",
		NoScript:      "Enable JavaScript for live member counts and more actions.",
		MissingBuild:  "Frontend assets are not built (run make web-dist).",
		PrivacyTitle:  "Privacy",
		NotFoundTitle: "Page not found",
		NotFoundLede:  "The page you are looking for does not exist or was removed.",
		BackHome:      "Back to home →",
		AdminTitle:    "LinksPage Admin",
		AdminNoScript: "The admin console requires JavaScript.",

		Join:          "Join →",
		JoinGroup:     "Join group →",
		OnlineMembers: "● %d online · %d members",
		OnlineOnly:    "● %d online",
		MembersOnly:   "%d members",
		UpdatedAt:     "Updated %s",
		Pending:       "Fetching the latest data…",
		GroupNumber:   "Group number",
		QRAlt:         "%s QR code",
		QRHint:        "Scan to join",
		OnlineList:    "Online: ",
		ListSeparator: ", ",
		MoreUsers:     " and %d more",
		PoweredBy:     "Powered by LinksPage",
		CopyDefaults: map[string]string{
			site.CopyInviteUnavailable:    "The invite is currently unavailable. Please try again later.",
			site.CopyCommunityUnavailable: "This community is currently unavailable.",
		},
		CommunityPinned: "Shared community",
	},
}

// textFor picks strings by primary language subtag, defaulting to English.
func textFor(locale string) uiStrings {
	if t, ok := uiText[primarySubtag(locale)]; ok {
		return t
	}
	return uiText["en"]
}

// copyText resolves an overridable copy key: copy[locale] → copy[default]
// → copy.en → built-in text.
func copyText(c site.CopyOverrides, locale, defaultLocale, key string, txt uiStrings) string {
	if v := c.Get(locale, defaultLocale, key); v != "" {
		return v
	}
	return txt.CopyDefaults[key]
}

// ogLocale converts a BCP-47 tag (zh-CN) to the OG form (zh_CN).
func ogLocale(locale string) string {
	return strings.ReplaceAll(locale, "-", "_")
}
