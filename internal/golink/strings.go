package golink

import (
	"strings"

	"github.com/Nanako1900/linksPage/internal/site"
)

// guideText holds the built-in guide page texts of one language. The
// fields named after site copy keys are only defaults: admin overrides
// in settings.copy win (see copyText).
type guideText struct {
	OpenInBrowser        string // copy key openInBrowser
	InviteUnavailable    string // copy key inviteUnavailable
	CommunityUnavailable string // copy key communityUnavailable

	OpenInBrowserLede string
	OpenInBrowserAlt  string
	LinkLabel         string
	CopyLink          string
	Copy              string
	Copied            string
	GroupNumberLabel  string
	CopyGroupNumber   string
	QQSearchHint      string
	QQInWeChatHint    string
	ScanQR            string
	QRAlt             string
	NoQR              string
	ContactLabel      string
	CopyContact       string
	OtherCommunities  string
	BackHome          string
	NotFoundTitle     string
	NotFoundLede      string
	ErrorTitle        string
	ErrorLede         string
}

var guideTexts = map[string]guideText{
	"zh": {
		OpenInBrowser:        "点击右上角 ··· → 在浏览器打开",
		InviteUnavailable:    "邀请暂不可用，可以先看看其他社区。",
		CommunityUnavailable: "这个社区暂时无法加入。",
		OpenInBrowserLede:    "微信和 QQ 内无法直接打开这个邀请。在浏览器中打开本页后会自动跳转。",
		OpenInBrowserAlt:     "箭头指向右上角的菜单按钮",
		LinkLabel:            "也可以复制链接，粘贴到浏览器地址栏打开",
		CopyLink:             "复制链接",
		Copy:                 "复制",
		Copied:               "已复制",
		GroupNumberLabel:     "QQ 群号",
		CopyGroupNumber:      "复制群号",
		QQSearchHint:         "打开 QQ，搜索群号即可申请加入。",
		QQInWeChatHint:       "微信内无法打开 QQ 加群链接。复制群号到 QQ 搜索，或长按识别二维码。",
		ScanQR:               "长按识别二维码",
		QRAlt:                "二维码",
		NoQR:                 "二维码暂未上传。",
		ContactLabel:         "群满或二维码失效时，加我拉你进群",
		CopyContact:          "复制微信号",
		OtherCommunities:     "其他社区",
		BackHome:             "返回首页",
		NotFoundTitle:        "链接不存在",
		NotFoundLede:         "这个跳转链接不存在或已被移除。",
		ErrorTitle:           "暂时无法打开",
		ErrorLede:            "服务暂时不可用，请稍后刷新重试。",
	},
	"en": {
		OpenInBrowser:        "Tap ··· in the top-right corner → Open in browser",
		InviteUnavailable:    "This invite is unavailable right now. Have a look at our other communities.",
		CommunityUnavailable: "This community cannot be joined right now.",
		OpenInBrowserLede:    "This invite cannot be opened inside WeChat or QQ. Open this page in your browser and it will continue automatically.",
		OpenInBrowserAlt:     "Arrow pointing at the menu button in the top-right corner",
		LinkLabel:            "Or copy the link and paste it into your browser",
		CopyLink:             "Copy link",
		Copy:                 "Copy",
		Copied:               "Copied",
		GroupNumberLabel:     "QQ group number",
		CopyGroupNumber:      "Copy number",
		QQSearchHint:         "Open QQ and search for the group number to join.",
		QQInWeChatHint:       "WeChat cannot open QQ join links. Copy the group number into QQ search, or press and hold the QR code.",
		ScanQR:               "Press and hold the QR code to scan it",
		QRAlt:                "QR code",
		NoQR:                 "The QR code has not been uploaded yet.",
		ContactLabel:         "Group full or QR code expired? Add me and I will invite you",
		CopyContact:          "Copy WeChat ID",
		OtherCommunities:     "Other communities",
		BackHome:             "Back to home",
		NotFoundTitle:        "Link not found",
		NotFoundLede:         "This link does not exist or was removed.",
		ErrorTitle:           "Temporarily unavailable",
		ErrorLede:            "The service is temporarily unavailable. Please try again shortly.",
	},
}

// textFor picks built-in texts by primary language subtag (English
// otherwise).
func textFor(locale string) guideText {
	primary, _, _ := strings.Cut(strings.ToLower(locale), "-")
	if t, ok := guideTexts[primary]; ok {
		return t
	}
	return guideTexts["en"]
}

// copyText returns the admin override for key (settings.copy, see
// site.CopyOverrides.Get) or the built-in text.
func copyText(s site.Settings, locale, key string, builtin guideText) string {
	if v := s.Copy.Get(locale, s.DefaultLocale, key); v != "" {
		return v
	}
	switch key {
	case site.CopyOpenInBrowser:
		return builtin.OpenInBrowser
	case site.CopyInviteUnavailable:
		return builtin.InviteUnavailable
	case site.CopyCommunityUnavailable:
		return builtin.CommunityUnavailable
	default:
		return ""
	}
}
