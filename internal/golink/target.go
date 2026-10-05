package golink

import (
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/Nanako1900/linksPage/internal/provider"
)

// maxRedirectURLLength matches content.MaxURLLength and the DB CHECKs.
const maxRedirectURLLength = 2048

// Redirect scheme allow-lists. Community targets are join pages; links
// may also be mailto: (doc 9).
var (
	communitySchemes = []string{"https", "http"}
	linkSchemes      = []string{"https", "http", "mailto"}
)

// qqJoinHosts are the official QQ group join hosts accepted as a
// qq-group invite_url (doc 5.5; same hosts as the platforms.yaml
// url_pattern of qq-group).
var qqJoinHosts = []string{"qm.qq.com", "qun.qq.com"}

// QQJoinHosts returns a copy of the QQ join host allow-list.
func QQJoinHosts() []string { return slices.Clone(qqJoinHosts) }

// JoinURL returns the redirect target of a community at now ("" when
// there is none). It applies provider.JoinTarget after dropping a
// qq-group invite_url outside QQJoinHosts, and rejects targets that are
// not absolute http(s) URLs (defence in depth: the writers validate too).
func JoinURL(c CommunityTarget, now time.Time) string {
	join := c.Join
	if join.Card == provider.CardQQGroup && !isQQJoinURL(join.InviteURL) {
		join.InviteURL = ""
	}
	target, ok := safeRedirect(provider.JoinTarget(join, now), communitySchemes)
	if !ok {
		return ""
	}
	return target
}

// isQQJoinURL reports whether raw is an https URL on a QQ join host.
func isQQJoinURL(raw string) bool {
	u, ok := parseRedirect(raw, []string{"https"})
	if !ok {
		return false
	}
	return slices.Contains(qqJoinHosts, strings.ToLower(u.Hostname()))
}

// safeRedirect validates raw as a Location value: allowed scheme, no
// credentials, no whitespace or control characters, bounded length.
func safeRedirect(raw string, schemes []string) (string, bool) {
	if _, ok := parseRedirect(raw, schemes); !ok {
		return "", false
	}
	return raw, true
}

func parseRedirect(raw string, schemes []string) (*url.URL, bool) {
	if raw == "" || len(raw) > maxRedirectURLLength || strings.ContainsFunc(raw, unsafeRune) {
		return nil, false
	}
	u, err := url.Parse(raw)
	if err != nil || !slices.Contains(schemes, strings.ToLower(u.Scheme)) {
		return nil, false
	}
	if strings.EqualFold(u.Scheme, "mailto") {
		return u, u.Opaque != ""
	}
	if u.Host == "" || u.User != nil || u.Opaque != "" {
		return nil, false
	}
	return u, true
}

// unsafeRune rejects characters that must never appear in a Location
// header or that browsers normalise in surprising ways.
func unsafeRune(r rune) bool {
	return r == '\\' || unicode.IsSpace(r) || unicode.IsControl(r) || r == unicode.ReplacementChar
}
