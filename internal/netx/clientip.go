package netx

import (
	"net/http"
	"net/netip"
	"strings"
)

// Header names used by the trust model.
const (
	HeaderXForwardedFor = "X-Forwarded-For"
	HeaderCFCountry     = "CF-IPCountry"
)

// strippedHeaders are fine-grained location headers removed from every
// request so they can never reach logs (doc 4.12). CF-IPCountry is read
// first (when trusted) and then removed as well.
var strippedHeaders = []string{
	"CF-IPCity", "CF-IPLatitude", "CF-IPLongitude", "CF-IPContinent",
	"CF-Region", "CF-Region-Code", "CF-Metro-Code", "CF-Postal-Code",
	"CF-Timezone", HeaderCFCountry,
}

// ClientInfo describes the resolved client of a request.
type ClientInfo struct {
	// IP is the best-known client address (invalid if RemoteAddr was
	// unparseable).
	IP netip.Addr
	// Country is the ISO country from CF-IPCountry, only when the peer is
	// a trusted proxy.
	Country string
	// ViaTrustedProxy reports whether the direct peer is trusted.
	ViaTrustedProxy bool
	// MissingHeader reports that a trusted peer did not send a usable
	// client IP header; IP then falls back to the peer address.
	MissingHeader bool
}

// Resolver derives client addresses according to trusted_proxies and
// client_ip_header. It is safe for concurrent use.
type Resolver struct {
	trusted TrustedSet
	header  string
	isXFF   bool
}

// NewResolver builds a Resolver. header is the client IP header to trust
// (for example CF-Connecting-IP or X-Forwarded-For).
func NewResolver(trusted TrustedSet, header string) *Resolver {
	canon := http.CanonicalHeaderKey(header)
	return &Resolver{trusted: trusted, header: canon, isXFF: canon == HeaderXForwardedFor}
}

// Resolve returns the client information for r without modifying it.
func (rv *Resolver) Resolve(r *http.Request) ClientInfo {
	peer := parseRemoteAddr(r.RemoteAddr)
	info := ClientInfo{IP: peer}
	if !peer.IsValid() || !rv.trusted.Contains(peer) {
		return info
	}
	info.ViaTrustedProxy = true
	info.Country = normalizeCountry(r.Header.Get(HeaderCFCountry))
	var ip netip.Addr
	if rv.isXFF {
		ip = rv.fromXFF(r.Header.Values(HeaderXForwardedFor))
	} else {
		ip = singleIP(r.Header.Values(rv.header))
	}
	if !ip.IsValid() {
		info.MissingHeader = true
		return info
	}
	info.IP = ip
	return info
}

// fromXFF walks X-Forwarded-For right to left, skipping trusted proxies.
// The first untrusted address is the client. If every hop is trusted the
// leftmost one is used; an unparseable hop stops the walk.
func (rv *Resolver) fromXFF(values []string) netip.Addr {
	var hops []string
	for _, v := range values {
		hops = append(hops, strings.Split(v, ",")...)
	}
	var last netip.Addr
	for i := len(hops) - 1; i >= 0; i-- {
		a, err := parseHop(hops[i])
		if err != nil {
			return netip.Addr{}
		}
		if !rv.trusted.Contains(a) {
			return a
		}
		last = a
	}
	return last
}

// singleIP accepts exactly one address in exactly one header value.
func singleIP(values []string) netip.Addr {
	if len(values) != 1 {
		return netip.Addr{}
	}
	a, err := parseHop(values[0])
	if err != nil {
		return netip.Addr{}
	}
	return a
}

func parseHop(s string) (netip.Addr, error) {
	s = strings.TrimSpace(s)
	if ap, err := netip.ParseAddrPort(s); err == nil {
		return ap.Addr().Unmap(), nil
	}
	a, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Addr{}, err
	}
	return a.Unmap().WithZone(""), nil
}

func parseRemoteAddr(remote string) netip.Addr {
	if ap, err := netip.ParseAddrPort(remote); err == nil {
		return ap.Addr().Unmap().WithZone("")
	}
	if a, err := netip.ParseAddr(remote); err == nil {
		return a.Unmap().WithZone("")
	}
	return netip.Addr{}
}

func normalizeCountry(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	if len(s) != 2 || s[0] < 'A' || s[0] > 'Z' || s[1] < 'A' || s[1] > 'Z' {
		return ""
	}
	return s
}
