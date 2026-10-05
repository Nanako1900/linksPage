package webui

import "strings"

// Content-Security-Policy builders (doc 12.4). Public pages use hashes,
// never nonces, so a cached body and its 304 revalidation always share
// the same policy.

// TrustedTypesReportOnly is sent as Content-Security-Policy-Report-Only
// on public pages until enforcement is enabled.
const TrustedTypesReportOnly = "require-trusted-types-for 'script'; trusted-types 'none'"

// AdminCSP is the policy for the admin shell.
const AdminCSP = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data: blob:; frame-src 'self'; frame-ancestors 'none'; base-uri 'self'; " +
	"form-action 'self'; object-src 'none'"

// PublicCSP builds the public page policy from inline-content hashes.
func PublicCSP(bootHash, criticalHash, themeHash string) string {
	return strings.Join([]string{
		"default-src 'self'",
		"script-src 'self' " + bootHash,
		"style-src 'self' " + criticalHash + " " + themeHash,
		"img-src 'self' data: blob:",
		"font-src 'self'",
		"connect-src 'self'",
		"frame-ancestors 'none'",
		"base-uri 'self'",
		"form-action 'self'",
		"object-src 'none'",
	}, "; ")
}
