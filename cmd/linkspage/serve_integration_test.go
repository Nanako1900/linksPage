package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/Nanako1900/linksPage/internal/config"
)

func get(ctx context.Context, t *testing.T, u string) (int, string) {
	t.Helper()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err.Error()
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func waitStatus(ctx context.Context, t *testing.T, u string, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		code, _ := get(ctx, t, u)
		if code == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("GET %s = %d, want %d", u, code, want)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// TestIntegrationServe boots the real server against postgres:18-alpine
// with db.auto_migrate=false: it must stay unready until `migrate up` is
// run, then become ready and shut down gracefully.
func TestIntegrationServe(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in -short mode")
	}
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()
	ctr, err := postgres.Run(ctx, "postgres:18-alpine", postgres.WithDatabase("linkspage"),
		postgres.WithUsername("linkspage"), postgres.WithPassword("pw"), postgres.BasicWaitStrategies())
	testcontainers.CleanupContainer(t, ctr)
	if err != nil {
		t.Fatal(err)
	}
	dsn, err := ctr.ConnectionString(ctx)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(dsn)
	for k, v := range map[string]string{
		"LP_CONFIG_FILE": t.TempDir() + "/none.yaml", "LP_BASE_URL": "http://127.0.0.1",
		"LP_DATA_DIR": t.TempDir(),
		"LP_DB__HOST": u.Hostname(), "LP_DB__PORT": u.Port(), "LP_DB__USER": "linkspage",
		"LP_DB__NAME": "linkspage", "LP_DB__PASSWORD": "pw", "LP_DB__AUTO_MIGRATE": "false",
		"LP_LOG__LEVEL": "error",
	} {
		t.Setenv(k, v)
	}

	loaded, err := config.Load(config.Options{})
	if err != nil {
		t.Fatal(err)
	}
	// A pre-bound listener avoids the close-then-reuse port race.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	base := "http://" + ln.Addr().String()
	listen := func(context.Context, string) (net.Listener, error) { return ln, nil }

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var logs syncBuffer
	logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn}))
	done := make(chan error, 1)
	go func() { done <- serve(runCtx, loaded.Config, logger, listen) }()

	// Pending migrations: alive but not ready.
	waitStatus(ctx, t, base+"/healthz", http.StatusOK, 10*time.Second)
	for _, p := range []string{"/readyz", "/api/v1/public/bootstrap"} {
		if code, _ := get(ctx, t, base+p); code != http.StatusServiceUnavailable {
			t.Errorf("%s before migrations = %d, want 503", p, code)
		}
	}

	var out, errOut bytes.Buffer
	if code := run(ctx, []string{"migrate", "status"}, &out, &errOut); code != 0 || !strings.Contains(out.String(), "pending") {
		t.Fatalf("migrate status: %d %q %q", code, out.String(), errOut.String())
	}
	out.Reset()
	if code := run(ctx, []string{"migrate", "up"}, &out, &errOut); code != 0 || !strings.Contains(out.String(), "applied 2 migration(s)") {
		t.Fatalf("migrate up: %d %q %q", code, out.String(), errOut.String())
	}
	out.Reset()
	if code := run(ctx, []string{"migrate", "status"}, &out, &errOut); code != 0 || !strings.Contains(out.String(), "applied") || strings.Contains(out.String(), "pending") {
		t.Fatalf("migrate status after up: %d %q", code, out.String())
	}

	waitStatus(ctx, t, base+"/readyz", http.StatusOK, 60*time.Second)
	if code, body := get(ctx, t, base+"/api/v1/public/bootstrap"); code != 200 || len(body) < 20 {
		t.Errorf("bootstrap = %d %q", code, body)
	}
	if code, _ := get(ctx, t, base+"/"); code != 200 {
		t.Errorf("home = %d", code)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("serve: %v", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("serve did not shut down")
	}
}
