package seed

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/Nanako1900/linksPage/internal/media"
)

// fakeIngester stands in for media.Processor: it hashes the bytes into a
// well-formed key and never decodes anything.
type fakeIngester struct {
	mu    sync.Mutex
	err   error
	calls []media.ProcessOptions
}

func (f *fakeIngester) Ingest(_ context.Context, r io.Reader, opts media.ProcessOptions) (media.Result, error) {
	f.mu.Lock()
	f.calls = append(f.calls, opts)
	f.mu.Unlock()
	if f.err != nil {
		return media.Result{}, f.err
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return media.Result{}, err
	}
	key := func(ext string) string {
		sum := sha256.Sum256(append([]byte(string(opts.Kind)+ext), data...))
		return hex.EncodeToString(sum[:16]) + "." + ext
	}
	stored := func(ext, ct string) media.Stored {
		return media.Stored{Key: key(ext), ContentType: ct, Bytes: len(data) + 1, Width: 480, Height: 480}
	}
	switch opts.Kind {
	case media.KindQR:
		return media.Result{Primary: stored("png", "image/png")}, nil
	case media.KindOG:
		return media.Result{Primary: stored("jpg", "image/jpeg")}, nil
	}
	return media.Result{Primary: stored("webp", "image/webp"), Variants: map[string]media.Stored{"png": stored("png", "image/png")}}, nil
}

func writeFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

func newTestImporter(t *testing.T, db TxBeginner, ing Ingester) *Importer {
	t.Helper()
	im, err := NewImporter(Deps{DB: db, Media: ing, Logger: discard(), MaxUploadBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	return im
}

func TestIngestAll(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "images", "qr.png"), "qr-bytes")
	writeFile(t, filepath.Join(dir, "images", "icon.webp"), "icon-bytes")
	outside := filepath.Join(t.TempDir(), "secret.png")
	writeFile(t, outside, "secret")
	if err := os.Symlink(outside, filepath.Join(dir, "images", "link.png")); err != nil {
		t.Fatal(err)
	}
	ok := []imageRef{
		{path: filepath.Join("images", "qr.png"), kind: media.KindQR, field: "communities[0].qr.image"},
		{path: filepath.Join("images", "icon.webp"), kind: media.KindIcon, field: "links[0].icon"},
	}
	tests := []struct {
		name      string
		refs      []imageRef
		ingestErr error
		invalid   bool
		want      string
	}{
		{"ok", ok, nil, false, ""},
		{"missing file", []imageRef{{path: "images/none.png", kind: media.KindIcon, field: "links[1].icon"}}, nil, true, "links[1].icon: open image"},
		{"symlink escape", []imageRef{{path: filepath.Join("images", "link.png"), kind: media.KindIcon, field: "links[2].icon"}}, nil, true, "links[2].icon"},
		{"unsupported", ok[:1], media.ErrUnsupportedType, true, "communities[0].qr.image: media: only png"},
		{"busy is transient", ok[:1], media.ErrBusy, false, "busy"},
		{"too large", ok[:1], fmt.Errorf("decode: %w", media.ErrTooLarge), true, "too large"},
		{"dimensions", ok[:1], media.ErrDimensions, true, "4096"},
		{"16-bit png", ok[:1], media.Err16BitPNG, true, "16-bit"},
		{"corrupt image", ok[:1], media.ErrInvalidImage, true, "corrupt"},
		{"unknown kind", ok[:1], media.ErrUnknownKind, true, "unknown media kind"},
		{"disk full is transient", ok[:1], &os.PathError{Op: "write", Path: "/data/uploads/x.webp", Err: syscall.ENOSPC}, false, "no space"},
		{"unclassified error is transient", ok[:1], errors.New("rename failed"), false, "rename failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ing := &fakeIngester{err: tt.ingestErr}
			im := newTestImporter(t, failingDB{}, ing)
			got, err := im.ingestAll(context.Background(), dir, tt.refs)
			if tt.want == "" {
				if err != nil || len(got) != 2 || ing.calls[0].MaxBytes != 1<<20 || ing.calls[1].MaxSide != 256 {
					t.Fatalf("got %v err=%v calls=%+v", got, err, ing.calls)
				}
				return
			}
			if err == nil || errors.Is(err, ErrInvalidSeed) != tt.invalid || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want %q (invalid=%v)", err, tt.want, tt.invalid)
			}
		})
	}
	if _, err := newTestImporter(t, failingDB{}, &fakeIngester{}).ingestAll(context.Background(), filepath.Join(dir, "nope"), ok); !errors.Is(err, ErrInvalidSeed) {
		t.Errorf("missing seed dir: %v", err)
	}
}

// failingDB fails every transaction start.
type failingDB struct{}

func (failingDB) Begin(context.Context) (pgx.Tx, error) { return nil, errors.New("db down") }

func TestImportDatabaseErrorIsTransient(t *testing.T) {
	im := newTestImporter(t, failingDB{}, &fakeIngester{})
	_, err := im.Import(context.Background(), examplePath)
	if err == nil || errors.Is(err, ErrInvalidSeed) {
		t.Fatalf("err = %v, want a retryable error", err)
	}
	if _, err := im.write(context.Background(), &plan{}, nil); err == nil || errors.Is(err, ErrInvalidSeed) {
		t.Fatalf("write err = %v", err)
	}
}

func TestReadFile(t *testing.T) {
	dir := t.TempDir()
	big := filepath.Join(dir, "big.yaml")
	writeFile(t, big, "version: 1\n"+strings.Repeat("#", MaxSeedBytes))
	for _, path := range []string{"", filepath.Join(dir, "missing.yaml"), big, dir} {
		if _, err := readFile(path); !errors.Is(err, ErrInvalidSeed) {
			t.Errorf("readFile(%q) = %v", path, err)
		}
	}
	if f, err := readFile(examplePath); err != nil || f.Version != 1 {
		t.Errorf("example: %v", err)
	}
}

func TestMergeSettings(t *testing.T) {
	seeded := map[string]any{"title": map[string]string{"en": "Seeded"}}
	tests := []struct {
		name, stored, want string
	}{
		{"empty", ``, `{"title":{"en":"Seeded"}}`},
		{"keeps other keys", `{"appearance":"dark","title":{"en":"Old"}}`, `{"appearance":"dark","title":{"en":"Seeded"}}`},
		{"corrupt stored", `[`, `{"title":{"en":"Seeded"}}`},
		{"invalid merge", `{"appearance":"neon"}`, `{"title":{"en":"Seeded"}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mergeSettings([]byte(tt.stored), seeded)
			if err != nil || string(got) != tt.want {
				t.Errorf("merge = %s err=%v, want %s", got, err, tt.want)
			}
		})
	}
	if _, err := mergeSettings(nil, map[string]any{"x": make(chan int)}); err == nil {
		t.Error("unencodable settings must fail")
	}
}
