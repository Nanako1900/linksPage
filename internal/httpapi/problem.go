package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// Stable error codes (also used as i18n keys by the frontend).
const (
	CodeInvalidRequest   = "invalid_request"
	CodeUnauthorized     = "unauthorized"
	CodeForbidden        = "forbidden"
	CodeNotFound         = "not_found"
	CodeMethodNotAllowed = "method_not_allowed"
	CodeNotAcceptable    = "not_acceptable"
	CodeConflict         = "conflict"
	CodePrecondition     = "precondition_failed"
	CodeTooLarge         = "payload_too_large"
	CodeUnsupportedMedia = "unsupported_media_type"
	CodeRateLimited      = "rate_limited"
	CodeCrossOrigin      = "cross_origin_forbidden"
	CodeNotReady         = "not_ready"
	CodeInternal         = "internal"
)

var codeByStatus = map[int]string{
	http.StatusBadRequest:            CodeInvalidRequest,
	http.StatusUnauthorized:          CodeUnauthorized,
	http.StatusForbidden:             CodeForbidden,
	http.StatusNotFound:              CodeNotFound,
	http.StatusMethodNotAllowed:      CodeMethodNotAllowed,
	http.StatusNotAcceptable:         CodeNotAcceptable,
	http.StatusConflict:              CodeConflict,
	http.StatusPreconditionFailed:    CodePrecondition,
	http.StatusRequestEntityTooLarge: CodeTooLarge,
	http.StatusUnsupportedMediaType:  CodeUnsupportedMedia,
	http.StatusUnprocessableEntity:   CodeInvalidRequest,
	http.StatusTooManyRequests:       CodeRateLimited,
	http.StatusServiceUnavailable:    CodeNotReady,
}

// ProblemContentType is the RFC 9457 media type.
const ProblemContentType = "application/problem+json"

// Problem is an RFC 9457 problem document extended with a stable code
// and the request ID.
type Problem struct {
	huma.ErrorModel
	Code      string `json:"code" doc:"Stable machine-readable error code"`
	RequestID string `json:"requestId,omitempty" doc:"Request ID for correlating logs"`
}

// codeFor returns the default code for an HTTP status.
func codeFor(status int) string {
	if status >= http.StatusInternalServerError && status != http.StatusServiceUnavailable {
		return CodeInternal
	}
	if c, ok := codeByStatus[status]; ok {
		return c
	}
	return CodeInvalidRequest
}

// NewProblem builds a problem with an explicit code. For statuses >= 500
// (except 503) the detail and code are replaced so internals never leak.
func NewProblem(status int, code, detail string, errs ...*huma.ErrorDetail) *Problem {
	if status >= http.StatusInternalServerError && status != http.StatusServiceUnavailable {
		return &Problem{ErrorModel: huma.ErrorModel{Status: status, Title: http.StatusText(status)}, Code: CodeInternal}
	}
	return &Problem{
		ErrorModel: huma.ErrorModel{Status: status, Title: http.StatusText(status), Detail: detail, Errors: errs},
		Code:       code,
	}
}

// newHumaError is installed as huma.NewError.
func newHumaError(status int, msg string, errs ...error) huma.StatusError {
	details := make([]*huma.ErrorDetail, 0, len(errs))
	for _, e := range errs {
		if e == nil {
			continue
		}
		if d, ok := e.(huma.ErrorDetailer); ok {
			details = append(details, d.ErrorDetail())
			continue
		}
		details = append(details, &huma.ErrorDetail{Message: e.Error()})
	}
	return NewProblem(status, codeFor(status), msg, details...)
}

// newHumaErrorWithContext is installed as huma.NewErrorWithContext. It
// adds the request ID and logs the original errors of 5xx responses.
func newHumaErrorWithContext(ctx huma.Context, status int, msg string, errs ...error) huma.StatusError {
	p := newHumaError(status, msg, errs...).(*Problem)
	if ctx == nil {
		return p
	}
	reqCtx := ctx.Context()
	p.RequestID = RequestIDFrom(reqCtx)
	if status >= http.StatusInternalServerError {
		loggerFrom(reqCtx).Error("request failed", slog.Int("status", status), slog.String("message", msg),
			slog.Any("error", errors.Join(errs...)), slog.String("request_id", p.RequestID))
	}
	return p
}

func init() {
	huma.NewError = newHumaError
	huma.NewErrorWithContext = newHumaErrorWithContext
}

// problemTransformer fills in the request ID for problems created without
// a context (for example huma.Error404NotFound).
func problemTransformer(ctx huma.Context, _ string, v any) (any, error) {
	if p, ok := v.(*Problem); ok && p.RequestID == "" {
		cp := *p
		cp.RequestID = RequestIDFrom(ctx.Context())
		return &cp, nil
	}
	return v, nil
}

// writeProblem writes a problem response outside of huma (router-level
// 404/405, panics, cross-origin denials).
func writeProblem(w http.ResponseWriter, r *http.Request, status int, code, detail string) {
	p := NewProblem(status, code, detail)
	p.RequestID = RequestIDFrom(r.Context())
	h := w.Header()
	h.Set("Content-Type", ProblemContentType)
	h.Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if r.Method == http.MethodHead {
		return
	}
	if err := json.NewEncoder(w).Encode(p); err != nil {
		loggerFrom(r.Context()).Debug("write problem", slog.Any("error", err))
	}
}
