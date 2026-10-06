package seed

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// examplePath is the shipped example (config/seed.example.yaml).
var examplePath = filepath.Join("..", "..", "config", "seed.example.yaml")

func parseExample(t *testing.T) *File {
	t.Helper()
	raw, err := os.ReadFile(examplePath)
	if err != nil {
		t.Fatal(err)
	}
	f, err := Parse(raw)
	if err != nil {
		t.Fatalf("seed.example.yaml: %v", err)
	}
	return f
}

func TestParseExample(t *testing.T) {
	f := parseExample(t)
	if len(f.Communities) != 4 || len(f.Links) != 3 || len(f.Blocks) != 9 {
		t.Errorf("counts = %d/%d/%d", len(f.Communities), len(f.Links), len(f.Blocks))
	}
	w := f.Communities[3]
	if w.Platform != "wechat-group" || w.QR == nil || w.QR.Image != "images/wechat-qr.png" || w.Contact.Value != "my_wechat_id" {
		t.Errorf("wechat = %+v", w)
	}
	if f.Communities[2].QQGroup != "123456789" || f.Communities[0].GuildID != "1114391825336250432" {
		t.Error("qq group or guild id lost")
	}
}

func TestParseErrors(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"unknown key", "version: 1\nsite:\n  titel: {}\n", "titel"},
		{"version", "version: 2\n", "version must be 1"},
		{"syntax", "version: [\n", "decode"},
		{"too large", strings.Repeat("#", MaxSeedBytes+1), "larger than"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.in))
			if err == nil || !strings.Contains(err.Error(), tc.want) || !errors.Is(err, ErrInvalidSeed) {
				t.Fatalf("err = %v, want %q wrapping ErrInvalidSeed", err, tc.want)
			}
		})
	}
}

func TestNewImporterRequiresDeps(t *testing.T) {
	if _, err := NewImporter(Deps{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}); err == nil {
		t.Error("missing deps should fail")
	}
}

func discard() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }
