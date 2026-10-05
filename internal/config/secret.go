package config

import (
	"encoding/json"
	"log/slog"
	"net/url"
)

// redacted is the placeholder printed instead of secret values.
const redacted = "[REDACTED]"

// Secret holds a sensitive string. It never reveals its value through
// fmt, encoding/json or log/slog; call Reveal to obtain the raw value.
type Secret string

// Reveal returns the raw secret value.
func (s Secret) Reveal() string { return string(s) }

// IsSet reports whether the secret has a non-empty value.
func (s Secret) IsSet() bool { return s != "" }

// String implements fmt.Stringer and always redacts.
func (s Secret) String() string {
	if s == "" {
		return ""
	}
	return redacted
}

// GoString implements fmt.GoStringer so %#v also redacts.
func (s Secret) GoString() string { return s.String() }

// MarshalJSON implements json.Marshaler and always redacts.
func (s Secret) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

// MarshalText implements encoding.TextMarshaler and always redacts.
func (s Secret) MarshalText() ([]byte, error) {
	return []byte(s.String()), nil
}

// LogValue implements slog.LogValuer and always redacts.
func (s Secret) LogValue() slog.Value {
	return slog.StringValue(s.String())
}

// ProxyURL is an outbound proxy URL. Proxy URLs often carry credentials
// (http://user:pass@host:port), so the password is redacted whenever the
// value is printed, marshaled or logged; call Reveal for the raw URL.
type ProxyURL string

// Reveal returns the raw proxy URL.
func (p ProxyURL) Reveal() string { return string(p) }

// String implements fmt.Stringer with the password redacted.
func (p ProxyURL) String() string {
	if p == "" {
		return ""
	}
	u, err := url.Parse(string(p))
	if err != nil {
		return redacted
	}
	return u.Redacted()
}

// GoString implements fmt.GoStringer so %#v also redacts.
func (p ProxyURL) GoString() string { return p.String() }

// MarshalJSON implements json.Marshaler with the password redacted.
func (p ProxyURL) MarshalJSON() ([]byte, error) {
	return json.Marshal(p.String())
}

// MarshalText implements encoding.TextMarshaler with the password redacted.
func (p ProxyURL) MarshalText() ([]byte, error) {
	return []byte(p.String()), nil
}

// LogValue implements slog.LogValuer with the password redacted.
func (p ProxyURL) LogValue() slog.Value {
	return slog.StringValue(p.String())
}
