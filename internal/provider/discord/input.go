package discord

import (
	"errors"
	"net/url"
	"regexp"
	"strings"

	"github.com/Nanako1900/linksPage/internal/provider"
)

// APIHost is the Discord REST host.
const APIHost = "discord.com"

var (
	guildIDRE    = regexp.MustCompile(`^\d{17,20}$`)
	inviteCodeRE = regexp.MustCompile(`^[A-Za-z0-9-]{2,32}$`)
)

// ErrInvalidInput is returned for guild ids or invite codes that fail the
// doc 5.1 patterns.
var ErrInvalidInput = errors.New("discord: invalid guild id or invite code")

// inviteHosts maps accepted invite link hosts to the path prefix before the code.
var inviteHosts = map[string]string{
	"discord.gg":         "/",
	"www.discord.gg":     "/",
	"discord.com":        "/invite/",
	"www.discord.com":    "/invite/",
	"discordapp.com":     "/invite/",
	"www.discordapp.com": "/invite/",
}

// ValidGuildID reports whether id is a Discord snowflake (17-20 digits).
func ValidGuildID(id string) bool {
	return guildIDRE.MatchString(id)
}

// ExtractInviteCode accepts a bare code or an invite link
// (discord.gg/<code>, discord.com/invite/<code>) and returns the code.
func ExtractInviteCode(input string) (string, error) {
	s := strings.TrimSpace(input)
	if inviteCodeRE.MatchString(s) {
		return s, nil
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil {
		return "", ErrInvalidInput
	}
	prefix, ok := inviteHosts[strings.ToLower(u.Hostname())]
	if !ok || u.Port() != "" || !strings.HasPrefix(u.Path, prefix) {
		return "", ErrInvalidInput
	}
	code := strings.TrimSuffix(strings.TrimPrefix(u.Path, prefix), "/")
	if !inviteCodeRE.MatchString(code) {
		return "", ErrInvalidInput
	}
	return code, nil
}

// WidgetURL builds the widget.json endpoint for a guild.
func WidgetURL(guildID string) (string, error) {
	if !ValidGuildID(guildID) {
		return "", ErrInvalidInput
	}
	u := url.URL{Scheme: "https", Host: APIHost, Path: "/api/guilds/" + url.PathEscape(guildID) + "/widget.json"}
	return u.String(), nil
}

// InviteURL builds the invite lookup endpoint including approximate counts.
func InviteURL(code string) (string, error) {
	if !inviteCodeRE.MatchString(code) {
		return "", ErrInvalidInput
	}
	u := url.URL{
		Scheme:   "https",
		Host:     APIHost,
		Path:     "/api/v10/invites/" + url.PathEscape(code),
		RawQuery: url.Values{"with_counts": {"true"}}.Encode(),
	}
	return u.String(), nil
}

// WidgetURLAt builds the widget.json endpoint under base (api_base).
func WidgetURLAt(base *url.URL, guildID string) (string, error) {
	if !ValidGuildID(guildID) {
		return "", ErrInvalidInput
	}
	return provider.EndpointURL(base, "/api/guilds/"+guildID+"/widget.json", nil), nil
}

// InviteURLAt builds the invite lookup endpoint (with counts) under base.
func InviteURLAt(base *url.URL, code string) (string, error) {
	if !inviteCodeRE.MatchString(code) {
		return "", ErrInvalidInput
	}
	return provider.EndpointURL(base, "/api/v10/invites/"+code, url.Values{"with_counts": {"true"}}), nil
}
