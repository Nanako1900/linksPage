package discord

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// maxRetryAfter caps upstream-provided back-off hints so a bogus value cannot
// park the scheduler indefinitely.
const maxRetryAfter = time.Hour

type errorBody struct {
	Code       *int     `json:"code"`
	Message    string   `json:"message"`
	RetryAfter *float64 `json:"retry_after"`
	Global     bool     `json:"global"`
}

// ParseWidget decodes a widget.json response. Non-200 responses are mapped to
// *APIError or *RateLimitError.
func ParseWidget(status int, header http.Header, body []byte) (Widget, error) {
	if status != http.StatusOK {
		return Widget{}, parseError(status, header, body)
	}
	var w Widget
	if err := json.Unmarshal(body, &w); err != nil {
		return Widget{}, fmt.Errorf("%w: widget: %w", ErrMalformed, err)
	}
	if w.ID == "" {
		return Widget{}, fmt.Errorf("%w: widget: missing id", ErrMalformed)
	}
	return w, nil
}

// ParseInvite decodes an invite lookup response. Non-200 responses are mapped
// to *APIError or *RateLimitError.
func ParseInvite(status int, header http.Header, body []byte) (Invite, error) {
	if status != http.StatusOK {
		return Invite{}, parseError(status, header, body)
	}
	var inv Invite
	if err := json.Unmarshal(body, &inv); err != nil {
		return Invite{}, fmt.Errorf("%w: invite: %w", ErrMalformed, err)
	}
	if inv.Code == "" || inv.Guild == nil || inv.Guild.ID == "" {
		return Invite{}, fmt.Errorf("%w: invite: missing code or guild", ErrMalformed)
	}
	return inv, nil
}

// parseError maps a non-200 response to a typed error. The body is never
// included in the returned error string.
func parseError(status int, header http.Header, body []byte) error {
	var eb errorBody
	jsonErr := json.Unmarshal(body, &eb)

	if status == http.StatusTooManyRequests {
		return rateLimitError(header, eb, jsonErr == nil)
	}
	apiErr := &APIError{HTTPStatus: status}
	if jsonErr == nil && eb.Code != nil {
		apiErr.Code = *eb.Code
		apiErr.Message = eb.Message
	}
	return apiErr
}

func rateLimitError(header http.Header, eb errorBody, isJSON bool) *RateLimitError {
	rl := &RateLimitError{
		Scope:      header.Get("X-RateLimit-Scope"),
		Bucket:     header.Get("X-RateLimit-Bucket"),
		Global:     eb.Global || strings.EqualFold(header.Get("X-RateLimit-Global"), "true"),
		Cloudflare: !isJSON,
	}
	switch {
	case isJSON && eb.RetryAfter != nil:
		rl.RetryAfter = secondsToDuration(*eb.RetryAfter)
	default:
		rl.RetryAfter = parseRetryAfterHeader(header.Get("Retry-After"))
	}
	return rl
}

// parseRetryAfterHeader accepts integer or fractional seconds. HTTP-date
// values are not used by Discord and are treated as absent.
func parseRetryAfterHeader(v string) time.Duration {
	if v == "" {
		return 0
	}
	secs, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil {
		return 0
	}
	return secondsToDuration(secs)
}

func secondsToDuration(secs float64) time.Duration {
	if math.IsNaN(secs) || secs <= 0 {
		return 0
	}
	if secs >= maxRetryAfter.Seconds() {
		return maxRetryAfter
	}
	// Round to milliseconds: Discord reports at most millisecond precision and
	// float multiplication would otherwise drift (64.57s -> 1m4.569999999s).
	return time.Duration(math.Round(secs*1000)) * time.Millisecond
}
