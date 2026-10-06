package media

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Nanako1900/linksPage/internal/store/dbq"
)

const testQRID = "0192f0c1-7a2b-7c3d-8e4f-001122334455"

type fakeMeta struct {
	media   map[string]dbq.Medium
	qr      map[[16]byte]dbq.GetQRCodeRow
	mediaEr error
	qrErr   error
}

func (f fakeMeta) GetMedia(_ context.Context, key string) (dbq.Medium, error) {
	if f.mediaEr != nil {
		return dbq.Medium{}, f.mediaEr
	}
	m, ok := f.media[key]
	if !ok {
		return dbq.Medium{}, pgx.ErrNoRows
	}
	return m, nil
}

func (f fakeMeta) GetQRCode(_ context.Context, id pgtype.UUID) (dbq.GetQRCodeRow, error) {
	if f.qrErr != nil {
		return dbq.GetQRCodeRow{}, f.qrErr
	}
	r, ok := f.qr[id.Bytes]
	if !ok {
		return dbq.GetQRCodeRow{}, pgx.ErrNoRows
	}
	return r, nil
}

type handlerFixture struct {
	store   *LocalStore
	pngKey  string
	qrKey   string
	ghost   string // row without file
	pngData []byte
}

func newHandlerFixture(t *testing.T) handlerFixture {
	t.Helper()
	s := newTestStore(t)
	data := pngBytes(t, patterned(3, 3))
	key, err := s.Put(context.Background(), data, ExtPNG)
	if err != nil {
		t.Fatal(err)
	}
	qrKey, err := s.Put(context.Background(), pngBytes(t, patterned(4, 4)), ExtPNG)
	if err != nil {
		t.Fatal(err)
	}
	return handlerFixture{store: s, pngKey: key, qrKey: qrKey, ghost: "ffffffffffffffffffffffffffffffff.webp", pngData: data}
}

func (f handlerFixture) meta() fakeMeta {
	id, _ := parseUUID(testQRID)
	return fakeMeta{
		media: map[string]dbq.Medium{
			f.pngKey: {Key: f.pngKey, Kind: string(KindAvatar)},
			f.qrKey:  {Key: f.qrKey, Kind: string(KindQR)},
			f.ghost:  {Key: f.ghost, Kind: string(KindIcon)},
		},
		qr: map[[16]byte]dbq.GetQRCodeRow{id.Bytes: {MediaKey: f.qrKey}},
	}
}

func serve(h http.Handler, pattern, target string, hdr map[string]string) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	mux.Handle(pattern, h)
	req := httptest.NewRequest(http.MethodGet, target, nil)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestUploadsHandler(t *testing.T) {
	t.Parallel()
	f := newHandlerFixture(t)
	etag := `"` + strings.TrimSuffix(f.pngKey, ".png") + `"`

	tests := []struct {
		name   string
		meta   fakeMeta
		store  Store
		key    string
		hdr    map[string]string
		status int
		cache  string
		ct     string
	}{
		{"ok", f.meta(), f.store, f.pngKey, nil, 200, CacheUploads, "image/png"},
		{"not modified", f.meta(), f.store, f.pngKey, map[string]string{"If-None-Match": etag}, 304, CacheUploads, ""},
		{"invalid key", f.meta(), f.store, "..%2fetc", nil, 404, CacheNoStore, ""},
		{"bad ext", f.meta(), f.store, strings.TrimSuffix(f.pngKey, ".png") + ".gif", nil, 404, CacheNoStore, ""},
		{"unknown row", f.meta(), f.store, "00000000000000000000000000000000.png", nil, 404, CacheNoStore, ""},
		{"qr hidden", f.meta(), f.store, f.qrKey, nil, 404, CacheNoStore, ""},
		{"row without file", f.meta(), f.store, f.ghost, nil, 404, CacheNoStore, ""},
		{"db error", fakeMeta{mediaEr: errBoom}, f.store, f.pngKey, nil, 500, CacheNoStore, ""},
		{"store error", f.meta(), failingStore{Store: f.store, err: errBoom}, f.pngKey, nil, 500, CacheNoStore, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := UploadsHandler(tt.store, tt.meta, nil)
			rec := serve(h, "GET /media/u/{key}", "/media/u/"+tt.key, tt.hdr)
			checkResponse(t, rec, tt.status, tt.cache, tt.ct)
			if tt.status == 200 {
				if rec.Body.String() != string(f.pngData) {
					t.Fatal("body mismatch")
				}
				if rec.Header().Get("ETag") != etag {
					t.Fatalf("ETag = %q", rec.Header().Get("ETag"))
				}
			}
		})
	}
}

func checkResponse(t *testing.T, rec *httptest.ResponseRecorder, status int, cache, ct string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d (%s)", rec.Code, status, rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != cache {
		t.Fatalf("Cache-Control = %q, want %q", got, cache)
	}
	if ct != "" && rec.Header().Get("Content-Type") != ct {
		t.Fatalf("Content-Type = %q, want %q", rec.Header().Get("Content-Type"), ct)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("missing nosniff")
	}
	if rec.Header().Get("Content-Security-Policy") != MediaCSP {
		t.Fatal("missing sandbox CSP")
	}
}

func TestQRHandler(t *testing.T) {
	t.Parallel()
	f := newHandlerFixture(t)
	id, _ := parseUUID(testQRID)
	badKey := f.meta()
	badKey.qr = map[[16]byte]dbq.GetQRCodeRow{id.Bytes: {MediaKey: "../x"}}
	missingFile := f.meta()
	missingFile.qr = map[[16]byte]dbq.GetQRCodeRow{id.Bytes: {MediaKey: f.ghost}}

	tests := []struct {
		name   string
		meta   fakeMeta
		id     string
		status int
		cache  string
		ct     string
	}{
		{"ok", f.meta(), testQRID, 200, CacheQR, "image/png"},
		{"uppercase ok", f.meta(), strings.ToUpper(testQRID), 200, CacheQR, "image/png"},
		{"gone", f.meta(), "0192f0c1-7a2b-7c3d-8e4f-ffffffffffff", 410, CacheGone, "text/plain; charset=utf-8"},
		{"malformed", f.meta(), "not-a-uuid", 404, CacheNoStore, ""},
		{"db error", fakeMeta{qrErr: errBoom}, testQRID, 500, CacheNoStore, ""},
		{"bad media key", badKey, testQRID, 500, CacheNoStore, ""},
		{"file missing", missingFile, testQRID, 404, CacheNoStore, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := serve(QRHandler(f.store, tt.meta, nil), "GET /media/q/{id}", "/media/q/"+tt.id, nil)
			checkResponse(t, rec, tt.status, tt.cache, tt.ct)
		})
	}
}

func TestParseUUID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in string
		ok bool
	}{
		{testQRID, true},
		{"0192F0C1-7A2B-7C3D-8E4F-001122334455", true},
		{"0192f0c17a2b7c3d8e4f001122334455", false},
		{"0192f0c1-7a2b-7c3d-8e4f-00112233445", false},
		{"0192f0c1-7a2b-7c3d-8e4f-00112233445g", false},
		{"0192f0c1+7a2b-7c3d-8e4f-001122334455", false},
		{"", false},
	}
	for _, tt := range tests {
		u, ok := parseUUID(tt.in)
		if ok != tt.ok || u.Valid != tt.ok {
			t.Errorf("parseUUID(%q) = %v, %v", tt.in, u, ok)
		}
	}
}

func TestHandlersRequireDeps(t *testing.T) {
	t.Parallel()
	for name, fn := range map[string]func(){
		"uploads": func() { UploadsHandler(nil, fakeMeta{}, nil) },
		"qr":      func() { QRHandler(newTestStore(t), nil, nil) },
		"files":   func() { SiteFilesHandler(SiteFile(99), func() *SiteFiles { return nil }) },
		"nil src": func() { SiteFilesHandler(FileRobots, nil) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: want panic", name)
				}
			}()
			fn()
		}()
	}
}

func TestIsMissing(t *testing.T) {
	t.Parallel()
	if !isMissing(pgx.ErrNoRows) || !isMissing(ErrNotFound) || isMissing(errBoom) || isMissing(nil) {
		t.Fatal("isMissing")
	}
	if !errors.Is(errors.Join(errBoom, pgx.ErrNoRows), pgx.ErrNoRows) {
		t.Fatal("sanity")
	}
}

// TestHandlersUnderChi pins that chi (used by internal/httpapi) populates
// r.PathValue for the {key} and {id} patterns.
func TestHandlersUnderChi(t *testing.T) {
	t.Parallel()
	f := newHandlerFixture(t)
	r := chi.NewRouter()
	r.Get("/media/u/{key}", UploadsHandler(f.store, f.meta(), nil).ServeHTTP)
	r.Get("/media/q/{id}", QRHandler(f.store, f.meta(), nil).ServeHTTP)
	for target, want := range map[string]int{
		"/media/u/" + f.pngKey: http.StatusOK,
		"/media/q/" + testQRID: http.StatusOK,
	} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
		if rec.Code != want {
			t.Errorf("%s: status %d", target, rec.Code)
		}
	}
}
