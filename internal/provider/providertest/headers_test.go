package providertest

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseHeaders(t *testing.T) {
	tests := []struct {
		name       string
		in         string
		wantStatus int
		wantProto  string
		wantHeader map[string]string
		wantErr    error
	}{
		{
			name:       "crlf with terminator",
			in:         "HTTP/2 302 \r\nlocation: https://example.com/a\r\nContent-Type: text/plain\r\n\r\n",
			wantStatus: 302,
			wantProto:  "HTTP/2",
			wantHeader: map[string]string{"Location": "https://example.com/a", "Content-Type": "text/plain"},
		},
		{
			name:       "lf without terminator",
			in:         "HTTP/1.1 429 Too Many Requests\nretry-after: 5",
			wantStatus: 429,
			wantProto:  "HTTP/1.1",
			wantHeader: map[string]string{"Retry-After": "5"},
		},
		{name: "status only", in: "HTTP/2 200", wantStatus: 200, wantProto: "HTTP/2"},
		{name: "empty", in: "", wantErr: ErrBadStatusLine},
		{name: "not http", in: "FOO 200\n", wantErr: ErrBadStatusLine},
		{name: "non numeric", in: "HTTP/2 abc\n", wantErr: ErrBadStatusLine},
		{name: "out of range", in: "HTTP/2 999\n", wantErr: ErrBadStatusLine},
		{name: "missing code", in: "HTTP/2\n", wantErr: ErrBadStatusLine},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseHeaders(strings.NewReader(tc.in))
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Status != tc.wantStatus || got.Proto != tc.wantProto {
				t.Fatalf("got %s %d, want %s %d", got.Proto, got.Status, tc.wantProto, tc.wantStatus)
			}
			for k, v := range tc.wantHeader {
				if got.Header.Get(k) != v {
					t.Errorf("header %s = %q, want %q", k, got.Header.Get(k), v)
				}
			}
		})
	}
}

func TestParseHeadersMalformedHeaderLine(t *testing.T) {
	_, err := ParseHeaders(strings.NewReader("HTTP/2 200\n bad continuation\n\n"))
	if err == nil {
		t.Fatal("expected error for malformed header block")
	}
}

func TestLoadHeaders(t *testing.T) {
	rec, err := LoadHeaders(filepath.Join("..", "discord", "testdata", "widget_ok.headers"))
	if err != nil {
		t.Fatalf("LoadHeaders: %v", err)
	}
	if rec.Status != 200 {
		t.Fatalf("status = %d", rec.Status)
	}
	if _, err := LoadHeaders(filepath.Join("testdata", "does-not-exist")); err == nil {
		t.Fatal("expected error for missing file")
	}
}
