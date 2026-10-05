package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// ErrResponseTooLarge is returned when a body exceeds MaxResponseBytes.
var ErrResponseTooLarge = errors.New("provider: response too large")

// Response is a fully read upstream response.
type Response struct {
	Status int
	Header http.Header
	Body   []byte
}

// Get performs a GET with ctx and reads at most MaxResponseBytes of the
// body. Redirects are not followed when client comes from NewHTTPClient,
// so 3xx responses are returned as-is.
func Get(ctx context.Context, client *http.Client, rawURL string, accept string) (Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return Response{}, fmt.Errorf("provider: build request: %w", err)
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	resp, err := client.Do(req)
	if err != nil {
		return Response{}, fmt.Errorf("provider: request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBytes+1))
	if err != nil {
		return Response{}, fmt.Errorf("provider: read body: %w", err)
	}
	if len(body) > MaxResponseBytes {
		return Response{}, ErrResponseTooLarge
	}
	return Response{Status: resp.StatusCode, Header: resp.Header, Body: body}, nil
}
