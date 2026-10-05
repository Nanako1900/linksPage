// Package netx implements the client IP trust model: which peers are
// trusted proxies and how the real client address is derived from them.
package netx

import (
	"bufio"
	_ "embed"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strings"
)

// Special trusted_proxies entries.
const (
	ProxiesNone       = "none"
	ProxiesCloudflare = "cloudflare"
)

//go:embed cloudflare_ips.txt
var cloudflareIPs string

// cloudflarePrefixes is parsed once from the embedded list.
var cloudflarePrefixes = mustParsePrefixList(cloudflareIPs)

// CloudflarePrefixes returns a copy of the built-in Cloudflare ranges.
func CloudflarePrefixes() []netip.Prefix {
	return slices.Clone(cloudflarePrefixes)
}

func mustParsePrefixList(s string) []netip.Prefix {
	var out []netip.Prefix
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, netip.MustParsePrefix(line))
	}
	return out
}

// TrustedSet is an immutable set of trusted proxy networks.
type TrustedSet struct {
	prefixes []netip.Prefix
}

// ParseTrustedProxies parses trusted_proxies entries: CIDRs, bare IPs,
// "cloudflare" (built-in Cloudflare ranges) or a lone "none".
func ParseTrustedProxies(specs []string) (TrustedSet, error) {
	if len(specs) == 0 {
		return TrustedSet{}, errors.New(`must not be empty; use "none" to trust no proxy`)
	}
	var prefixes []netip.Prefix
	for i, raw := range specs {
		spec := strings.TrimSpace(raw)
		switch strings.ToLower(spec) {
		case ProxiesNone:
			if len(specs) != 1 {
				return TrustedSet{}, errors.New(`"none" cannot be combined with other entries`)
			}
			return TrustedSet{}, nil
		case ProxiesCloudflare:
			prefixes = append(prefixes, cloudflarePrefixes...)
			continue
		}
		p, err := parsePrefixOrAddr(spec)
		if err != nil {
			return TrustedSet{}, fmt.Errorf("entry %d: must be a CIDR, an IP address, %q or %q", i, ProxiesCloudflare, ProxiesNone)
		}
		prefixes = append(prefixes, p)
	}
	return TrustedSet{prefixes: prefixes}, nil
}

func parsePrefixOrAddr(s string) (netip.Prefix, error) {
	if strings.Contains(s, "/") {
		p, err := netip.ParsePrefix(s)
		if err != nil {
			return netip.Prefix{}, err
		}
		return netip.PrefixFrom(p.Addr().Unmap(), unmappedBits(p)).Masked(), nil
	}
	a, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Prefix{}, err
	}
	a = a.Unmap()
	return netip.PrefixFrom(a, a.BitLen()), nil
}

// unmappedBits converts an IPv4-mapped IPv6 prefix length to IPv4 bits.
func unmappedBits(p netip.Prefix) int {
	if p.Addr().Is4In6() {
		return max(p.Bits()-96, 0)
	}
	return p.Bits()
}

// Contains reports whether addr belongs to a trusted network.
func (t TrustedSet) Contains(addr netip.Addr) bool {
	addr = addr.Unmap()
	for _, p := range t.prefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// Len returns the number of trusted networks.
func (t TrustedSet) Len() int { return len(t.prefixes) }
