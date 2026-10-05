package discord

import (
	"errors"
	"fmt"
	"time"
)

// Discord JSON error codes relevant to the public endpoints.
const (
	CodeUnknownGuild   = 10004
	CodeUnknownInvite  = 10006
	CodeWidgetDisabled = 50004
)

// Sentinel errors; use errors.Is against values returned by the parsers.
var (
	ErrUnknownGuild   = errors.New("discord: unknown guild")
	ErrUnknownInvite  = errors.New("discord: unknown invite")
	ErrWidgetDisabled = errors.New("discord: widget disabled")
	ErrRateLimited    = errors.New("discord: rate limited")
	ErrMalformed      = errors.New("discord: malformed response")
	ErrUpstream       = errors.New("discord: unexpected upstream response")
)

// APIError describes a non-2xx Discord response. Message is the upstream
// message and must not be persisted verbatim (doc 5.1 stores codes only).
type APIError struct {
	HTTPStatus int
	Code       int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("discord: http %d, code %d", e.HTTPStatus, e.Code)
}

// Unwrap maps known codes to sentinel errors.
func (e *APIError) Unwrap() error {
	switch e.Code {
	case CodeUnknownGuild:
		return ErrUnknownGuild
	case CodeUnknownInvite:
		return ErrUnknownInvite
	case CodeWidgetDisabled:
		return ErrWidgetDisabled
	default:
		return ErrUpstream
	}
}

// RateLimitError is returned for HTTP 429. RetryAfter is zero when the
// upstream gave no hint (e.g. Cloudflare-level bans).
type RateLimitError struct {
	RetryAfter time.Duration
	Global     bool
	Scope      string
	Bucket     string
	Cloudflare bool // HTML ban page instead of a JSON body
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("discord: rate limited (retry after %s, global=%t, scope=%q)", e.RetryAfter, e.Global, e.Scope)
}

// Unwrap lets callers match with errors.Is(err, ErrRateLimited).
func (e *RateLimitError) Unwrap() error {
	return ErrRateLimited
}

// ErrorCode returns a short stable code suitable for Snapshot.ErrCode.
func ErrorCode(err error) string {
	var apiErr *APIError
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrRateLimited):
		return "discord_429"
	case errors.As(err, &apiErr):
		return fmt.Sprintf("discord_%d_%d", apiErr.HTTPStatus, apiErr.Code)
	case errors.Is(err, ErrMalformed):
		return "discord_malformed"
	default:
		return "discord_error"
	}
}
