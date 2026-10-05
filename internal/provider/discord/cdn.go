package discord

import (
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// CDNHost serves guild icons, banners, splashes and widget avatars.
const CDNHost = "cdn.discordapp.com"

// ErrInvalidImage is returned for malformed image hashes or sizes.
var ErrInvalidImage = errors.New("discord: invalid image hash or size")

var imageHashRE = regexp.MustCompile(`^(a_)?[0-9a-f]{32}$`)

// ImageHosts returns the host -> allowed path prefixes used to vet URLs before
// they are registered with the media proxy (doc 5.2). A fresh map is returned
// on every call so callers cannot mutate shared state.
func ImageHosts() map[string][]string {
	return map[string][]string{
		CDNHost: {"/icons/", "/banners/", "/splashes/", "/widget-avatars/"},
	}
}

// IconURL builds a guild icon URL. Animated hashes ("a_" prefix) use .gif.
func IconURL(guildID, hash string, size int) (string, error) {
	return imageURL("icons", guildID, hash, size, true)
}

// BannerURL builds a guild banner URL. Animated hashes use .gif.
func BannerURL(guildID, hash string, size int) (string, error) {
	return imageURL("banners", guildID, hash, size, true)
}

// SplashURL builds an invite splash URL. Splashes are never animated.
func SplashURL(guildID, hash string, size int) (string, error) {
	return imageURL("splashes", guildID, hash, size, false)
}

func imageURL(kind, guildID, hash string, size int, mayAnimate bool) (string, error) {
	if !ValidGuildID(guildID) || !imageHashRE.MatchString(hash) || !validSize(size) {
		return "", ErrInvalidImage
	}
	animated := strings.HasPrefix(hash, "a_")
	if animated && !mayAnimate {
		return "", ErrInvalidImage
	}
	ext := ".png"
	if animated {
		ext = ".gif"
	}
	u := url.URL{
		Scheme:   "https",
		Host:     CDNHost,
		Path:     "/" + kind + "/" + guildID + "/" + hash + ext,
		RawQuery: url.Values{"size": {strconv.Itoa(size)}}.Encode(),
	}
	return u.String(), nil
}

// validSize accepts the power-of-two sizes Discord's CDN supports.
func validSize(size int) bool {
	return size >= 16 && size <= 4096 && size&(size-1) == 0
}
