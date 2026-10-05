package provider

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
)

// ErrCodeRe is the provider_snapshots.err_code format.
var ErrCodeRe = regexp.MustCompile(`^[a-z0-9_]{1,64}$`)

// Generic error codes used when a provider gives no specific one.
const (
	CodeFetchFailed   = "fetch_failed"
	CodeConfigInvalid = "config_invalid"
	CodeUnknownKind   = "provider_unknown"
)

// FetchError tags a transient Fetch failure with a short stable code that
// the refresh job stores in provider_snapshots.err_code (never upstream
// bodies).
type FetchError struct {
	Code string
	Err  error
}

func (e *FetchError) Error() string {
	if e.Err == nil {
		return "provider: " + e.Code
	}
	return "provider: " + e.Code + ": " + e.Err.Error()
}

func (e *FetchError) Unwrap() error { return e.Err }

// ErrCode returns the code of the first *FetchError in err's chain, or
// CodeFetchFailed. Invalid codes are replaced with CodeFetchFailed so the
// database CHECK can never fail.
func ErrCode(err error) string {
	var fe *FetchError
	if errors.As(err, &fe) && ErrCodeRe.MatchString(fe.Code) {
		return fe.Code
	}
	return CodeFetchFailed
}

// ValidateEmptyConfig accepts an empty value, null or {} for
// communities.config (always {} in M1); anything else wraps
// ErrInvalidConfig.
func ValidateEmptyConfig(raw json.RawMessage) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.DisallowUnknownFields()
	var empty struct{}
	if err := dec.Decode(&empty); err != nil {
		return fmt.Errorf("%w: config must be an empty object", ErrInvalidConfig)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: trailing data in config", ErrInvalidConfig)
	}
	return nil
}
