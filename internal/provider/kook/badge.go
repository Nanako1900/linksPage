// Package kook parses KOOK's token-less guild badge endpoint (doc 5.4).
//
// GET https://www.kookapp.cn/api/v3/badge/guild?guild_id=&style= answers with
// a 302 whose Location is an img.shields.io static badge. The only data we
// read is the "label" query parameter; the SVG itself is never fetched.
// The format is undocumented, so parsing is strict and fails closed.
package kook

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Style selects what the badge label carries.
type Style int

// Badge styles accepted by the endpoint. Unknown styles fall back to StyleName
// upstream (style=3 was observed to return the server name).
const (
	StyleName        Style = 0 // label = server name
	StyleOnline      Style = 1 // label = "<online> ONLINE"
	StyleOnlineTotal Style = 2 // label = "<online>/<total> ONLINE"
)

// Kind is the interpretation of a parsed label.
type Kind int

// Label kinds.
const (
	KindName Kind = iota + 1
	KindOnline
	KindOnlineTotal
)

const (
	// APIHost serves the badge endpoint.
	APIHost = "www.kookapp.cn"
	// BadgeHost is the redirect target host.
	BadgeHost = "img.shields.io"
	// badgePath is the redirect target path.
	badgePath = "/static/v1"
	// notPublicMarker is the label used for missing or private guilds.
	notPublicMarker = "服务器不存在或非公开"
	// maxLabelBytes bounds the label we are willing to interpret.
	maxLabelBytes = 512
)

// Errors returned by the parsers.
var (
	ErrInvalidGuildID   = errors.New("kook: invalid guild id")
	ErrInvalidStyle     = errors.New("kook: invalid badge style")
	ErrNotPublic        = errors.New("kook: guild does not exist or is not public")
	ErrMalformed        = errors.New("kook: malformed badge location")
	ErrUnexpectedStatus = errors.New("kook: unexpected badge status")
)

var (
	guildIDRE     = regexp.MustCompile(`^\d{1,20}$`)
	onlineRE      = regexp.MustCompile(`^(\d{1,9}) ONLINE$`)
	onlineTotalRE = regexp.MustCompile(`^(\d{1,9})/(\d{1,9}) ONLINE$`)
)

// Badge is the parsed badge label. Online and Total are meaningful only for
// KindOnline (Online) and KindOnlineTotal (both).
type Badge struct {
	Kind   Kind
	Label  string
	Name   string
	Online int
	Total  int
}

// ValidGuildID reports whether id matches the doc 5.1 pattern.
func ValidGuildID(id string) bool {
	return guildIDRE.MatchString(id)
}

// BadgeURL builds the badge endpoint URL for a guild and style.
func BadgeURL(guildID string, style Style) (string, error) {
	if !ValidGuildID(guildID) {
		return "", ErrInvalidGuildID
	}
	if style < StyleName || style > StyleOnlineTotal {
		return "", ErrInvalidStyle
	}
	q := url.Values{"guild_id": {guildID}, "style": {strconv.Itoa(int(style))}}
	u := url.URL{Scheme: "https", Host: APIHost, Path: "/api/v3/badge/guild", RawQuery: q.Encode()}
	return u.String(), nil
}

// ParseBadgeResponse validates a non-followed badge response and parses its
// Location for the requested style.
func ParseBadgeResponse(status int, header http.Header, style Style) (Badge, error) {
	switch status {
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther,
		http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
	default:
		return Badge{}, fmt.Errorf("%w: %d", ErrUnexpectedStatus, status)
	}
	return ParseBadgeLocationForStyle(header.Get("Location"), style)
}

// ParseBadgeLocationForStyle parses a Location and interprets the label
// strictly according to the style that was requested. Use this in the
// provider: a guild literally named "5 ONLINE" stays a name for StyleName.
func ParseBadgeLocationForStyle(location string, style Style) (Badge, error) {
	label, err := extractLabel(location)
	if err != nil {
		return Badge{}, err
	}
	switch style {
	case StyleName:
		return Badge{Kind: KindName, Label: label, Name: label}, nil
	case StyleOnline:
		b, ok := parseOnline(label)
		if !ok {
			return Badge{}, fmt.Errorf("%w: label does not match style 1", ErrMalformed)
		}
		return b, nil
	case StyleOnlineTotal:
		b, ok := parseOnlineTotal(label)
		if !ok {
			return Badge{}, fmt.Errorf("%w: label does not match style 2", ErrMalformed)
		}
		return b, nil
	default:
		return Badge{}, ErrInvalidStyle
	}
}

// ParseBadgeLocation parses a Location without knowing the requested style:
// count-shaped labels become KindOnline/KindOnlineTotal, anything else is
// treated as a server name.
func ParseBadgeLocation(location string) (Badge, error) {
	label, err := extractLabel(location)
	if err != nil {
		return Badge{}, err
	}
	if b, ok := parseOnlineTotal(label); ok {
		return b, nil
	}
	if b, ok := parseOnline(label); ok {
		return b, nil
	}
	return Badge{Kind: KindName, Label: label, Name: label}, nil
}

// extractLabel validates the redirect target and returns the decoded label.
func extractLabel(location string) (string, error) {
	if location == "" {
		return "", fmt.Errorf("%w: empty location", ErrMalformed)
	}
	u, err := url.Parse(location)
	if err != nil {
		return "", fmt.Errorf("%w: unparseable url", ErrMalformed)
	}
	if u.Scheme != "https" || u.Host != BadgeHost || u.Path != badgePath || u.User != nil {
		return "", fmt.Errorf("%w: unexpected redirect target", ErrMalformed)
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return "", fmt.Errorf("%w: bad query encoding", ErrMalformed)
	}
	label := strings.TrimSpace(q.Get("label"))
	if strings.Contains(label, notPublicMarker) || q.Get("message") == "404" {
		return "", ErrNotPublic
	}
	if label == "" {
		return "", fmt.Errorf("%w: missing label", ErrMalformed)
	}
	if len(label) > maxLabelBytes || !utf8.ValidString(label) {
		return "", fmt.Errorf("%w: label too long or not utf-8", ErrMalformed)
	}
	return label, nil
}

func parseOnline(label string) (Badge, bool) {
	m := onlineRE.FindStringSubmatch(label)
	if m == nil {
		return Badge{}, false
	}
	online, _ := strconv.Atoi(m[1]) // regex bounds digits to 9, cannot overflow
	return Badge{Kind: KindOnline, Label: label, Online: online}, true
}

func parseOnlineTotal(label string) (Badge, bool) {
	m := onlineTotalRE.FindStringSubmatch(label)
	if m == nil {
		return Badge{}, false
	}
	online, _ := strconv.Atoi(m[1]) // regex bounds digits to 9, cannot overflow
	total, _ := strconv.Atoi(m[2])
	if online > total {
		return Badge{}, false
	}
	return Badge{Kind: KindOnlineTotal, Label: label, Online: online, Total: total}, true
}
