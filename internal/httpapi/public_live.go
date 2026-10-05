package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/Nanako1900/linksPage/internal/site"
)

// Cache policies of the live endpoint (contract section 8).
const (
	CacheLive = "public, max-age=0, s-maxage=30"
	// liveETagHexLen is the number of hex digits of the live ETag.
	liveETagHexLen = 32
)

// LiveInput carries the conditional request header.
type LiveInput struct {
	IfNoneMatch string `header:"If-None-Match" doc:"ETag of a previous response"`
}

// LiveOutput is the response of GET /api/v1/public/live. Status is 304
// (without body) when If-None-Match matches.
type LiveOutput struct {
	Status       int
	CacheControl string `header:"Cache-Control"`
	ETag         string `header:"ETag"`
	Body         struct {
		Data site.LiveDTO `json:"data"`
	}
}

func registerLive(api huma.API, p publicAPI) {
	huma.Register(api, huma.Operation{
		OperationID: "getPublicLive",
		Method:      http.MethodGet,
		Path:        "/api/v1/public/live",
		Summary:     "Live community data",
		Description: "Frequently changing card data, polled every 60 s while the page is visible. " +
			"Send If-None-Match; 304 when unchanged. A different revision means the page must be reloaded.",
		Tags:   []string{"public"},
		Errors: []int{http.StatusServiceUnavailable, http.StatusTooManyRequests},
		Responses: map[string]*huma.Response{
			"304": {Description: "Not modified"},
		},
	}, p.live)
}

func (p publicAPI) live(_ context.Context, in *LiveInput) (*LiveOutput, error) {
	if err := p.checkReady(); err != nil {
		return nil, err
	}
	page := p.snapshots.Current().Public
	dto := page.Live()
	tag, err := p.liveTags.get(page, dto)
	if err != nil {
		return nil, err
	}
	out := &LiveOutput{Status: http.StatusOK, CacheControl: CacheLive, ETag: tag}
	if etagMatches(in.IfNoneMatch, tag) {
		out.Status = http.StatusNotModified
		return out, nil
	}
	out.Body.Data = dto
	return out, nil
}

// etagCache remembers the live ETag of the most recent page so it is
// computed once per snapshot.
type etagCache struct {
	mu   sync.Mutex
	page *site.PublicPage
	tag  string
}

func (c *etagCache) get(page *site.PublicPage, dto site.LiveDTO) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.page == page && c.tag != "" {
		return c.tag, nil
	}
	tag, err := LiveETag(dto)
	if err != nil {
		return "", err
	}
	c.page, c.tag = page, tag
	return tag, nil
}

// LiveETag is the strong ETag of a LiveDTO: the first 32 hex digits of the
// SHA-256 of its JSON without generatedAt.
func LiveETag(dto site.LiveDTO) (string, error) {
	dto.GeneratedAt = time.Time{}
	b, err := json.Marshal(dto)
	if err != nil {
		return "", fmt.Errorf("live etag: %w", err)
	}
	sum := sha256.Sum256(b)
	return `"` + hex.EncodeToString(sum[:])[:liveETagHexLen] + `"`, nil
}

// etagMatches implements the weak comparison of If-None-Match (RFC 9110
// 13.1.2): "*" or any listed tag equal to tag ignoring the W/ prefix.
func etagMatches(header, tag string) bool {
	if header == "" {
		return false
	}
	want := strings.TrimPrefix(tag, "W/")
	for part := range strings.SplitSeq(header, ",") {
		part = strings.TrimSpace(part)
		if part == "*" || strings.TrimPrefix(part, "W/") == want {
			return true
		}
	}
	return false
}
