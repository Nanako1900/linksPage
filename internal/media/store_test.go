package media

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKeyFor(t *testing.T) {
	t.Parallel()
	got := KeyFor([]byte("abc"), ExtPNG)
	// sha256("abc") = ba7816bf8f01cfea414140de5dae2223...
	if got != "ba7816bf8f01cfea414140de5dae2223.png" {
		t.Fatalf("KeyFor = %q", got)
	}
	if !KeyRe.MatchString(got) {
		t.Fatalf("key %q does not match KeyRe", got)
	}
}

func TestContentTypeForKey(t *testing.T) {
	t.Parallel()
	tests := []struct {
		key, want string
	}{
		{"ba7816bf8f01cfea414140de5dae2223.png", "image/png"},
		{"ba7816bf8f01cfea414140de5dae2223.webp", "image/webp"},
		{"ba7816bf8f01cfea414140de5dae2223.jpg", "image/jpeg"},
		{"ba7816bf8f01cfea414140de5dae2223.gif", ""},
		{"../etc/passwd", ""},
		{"", ""},
	}
	for _, tt := range tests {
		if got := ContentTypeForKey(tt.key); got != tt.want {
			t.Errorf("ContentTypeForKey(%q) = %q, want %q", tt.key, got, tt.want)
		}
	}
	if (Stored{Key: "k.png"}).URL() != "/media/u/k.png" {
		t.Error("Stored.URL")
	}
}

func TestNewLocalStore(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(dir, "up")
	if err := os.MkdirAll(stale, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stale, tmpPrefix+"abc"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stale, "keep.png"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		dir     string
		wantErr bool
	}{
		{"empty", "", true},
		{"file in the way", filepath.Join(file, "sub"), true},
		{"creates nested", filepath.Join(dir, "a", "b"), false},
		{"removes temps", stale, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := NewLocalStore(tt.dir)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil {
				_ = s.Close()
			}
		})
	}
	if _, err := os.Stat(filepath.Join(stale, tmpPrefix+"abc")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("stale temp not removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(stale, "keep.png")); err != nil {
		t.Errorf("non-temp file removed: %v", err)
	}
}

func TestLocalStorePutOpen(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := newTestStore(t)
	data := []byte("\x89PNG fake")
	key, err := s.Put(ctx, data, ExtPNG)
	if err != nil {
		t.Fatal(err)
	}
	if key != KeyFor(data, ExtPNG) {
		t.Fatalf("key = %q", key)
	}
	again, err := s.Put(ctx, data, ExtPNG)
	if err != nil || again != key {
		t.Fatalf("second Put = %q, %v", again, err)
	}
	if got := readKey(t, s, key); string(got) != string(data) {
		t.Fatalf("read back %q", got)
	}
	entries, err := os.ReadDir(filepath.Dir(filepath.Join(s.root.Name(), key)))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), tmpPrefix) {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}

func TestLocalStoreErrors(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	ctx := context.Background()

	putTests := []struct {
		name string
		ctx  context.Context
		data []byte
		ext  string
	}{
		{"bad ext", ctx, []byte("x"), "gif"},
		{"traversal ext", ctx, []byte("x"), "png/../../x"},
		{"empty", ctx, nil, ExtPNG},
		{"cancelled", cancelled, []byte("x"), ExtPNG},
	}
	for _, tt := range putTests {
		if _, err := s.Put(tt.ctx, tt.data, tt.ext); err == nil {
			t.Errorf("Put %s: want error", tt.name)
		}
	}

	openTests := []struct {
		name string
		ctx  context.Context
		key  string
		want error
	}{
		{"invalid", ctx, "../../etc/passwd", ErrNotFound},
		{"missing", ctx, "00000000000000000000000000000000.png", ErrNotFound},
		{"cancelled", cancelled, "00000000000000000000000000000000.png", context.Canceled},
	}
	for _, tt := range openTests {
		if _, err := s.Open(tt.ctx, tt.key); !errors.Is(err, tt.want) {
			t.Errorf("Open %s: err = %v, want %v", tt.name, err, tt.want)
		}
	}
	if err := (&LocalStore{}).Close(); err != nil {
		t.Errorf("Close on zero store: %v", err)
	}
}

func TestLocalStoreReadOnlyDir(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root ignores permissions")
	}
	dir := t.TempDir()
	s, err := NewLocalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o750) })
	if _, err := s.Put(context.Background(), []byte("data"), ExtPNG); err == nil {
		t.Fatal("Put into read-only dir: want error")
	}
}

func TestReadStored(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := newTestStore(t)
	key, err := s.Put(ctx, []byte("0123456789"), ExtPNG)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := readStored(ctx, s, key, 5); !errors.Is(err, ErrTooLarge) {
		t.Errorf("limit: err = %v", err)
	}
	if b, err := readStored(ctx, s, key, 10); err != nil || len(b) != 10 {
		t.Errorf("read = %d, %v", len(b), err)
	}
	if _, err := readStored(ctx, s, "00000000000000000000000000000000.png", 10); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing: err = %v", err)
	}
}
