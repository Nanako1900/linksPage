package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
)

func runCmd(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(context.Background(), args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestRunBasics(t *testing.T) {
	if code, out, _ := runCmd(t, "version"); code != 0 || !strings.HasPrefix(out, "linkspage dev") {
		t.Errorf("version: %d %q", code, out)
	}
	if code, out, _ := runCmd(t, "help"); code != 0 || !strings.Contains(out, "migrate up|status") {
		t.Errorf("help: %d", code)
	}
	if code, _, errOut := runCmd(t, "bogus"); code != 2 || !strings.Contains(errOut, "unknown command") {
		t.Errorf("bogus: %d", code)
	}
	for _, args := range [][]string{{"migrate"}, {"migrate", "down"}, {"config"}, {"config", "check", "extra"}, {"openapi", "x"}, {"serve", "x"}, {"healthcheck", "x"}} {
		if code, _, _ := runCmd(t, args...); code != 2 {
			t.Errorf("%v: exit %d, want 2", args, code)
		}
	}
}

func TestOpenAPICommand(t *testing.T) {
	code, out, _ := runCmd(t, "openapi")
	var doc map[string]any
	if code != 0 || json.Unmarshal([]byte(out), &doc) != nil || doc["openapi"] != "3.1.0" {
		t.Fatalf("openapi: %d", code)
	}
}

func TestConfigCheckCommand(t *testing.T) {
	t.Setenv("LP_CONFIG_FILE", t.TempDir()+"/missing.yaml")
	t.Setenv("LP_BASE_URL", "http://localhost:8080")
	t.Setenv("LP_DB__PASSWORD", "very-secret-password")
	code, out, errOut := runCmd(t, "config", "check", "-print")
	if code != 0 || !strings.Contains(out, "configuration OK") || !strings.Contains(errOut, "WARN: no administrators") {
		t.Fatalf("config check: %d %q %q", code, out, errOut)
	}
	if strings.Contains(out, "very-secret-password") || !strings.Contains(out, "[REDACTED]") {
		t.Error("printed config must redact secrets")
	}
	t.Setenv("LP_BASE_URL", "")
	if code, _, errOut := runCmd(t, "config", "check"); code != 1 || !strings.Contains(errOut, "base_url: required") {
		t.Errorf("invalid config: %d %q", code, errOut)
	}
	if code, _, _ := runCmd(t, "serve"); code != 1 {
		t.Errorf("serve with invalid config: %d", code)
	}
	if code, _, _ := runCmd(t, "migrate", "status"); code != 1 {
		t.Errorf("migrate with invalid config: %d", code)
	}
}

func TestMigrateWithoutDatabaseConfig(t *testing.T) {
	t.Setenv("LP_CONFIG_FILE", t.TempDir()+"/missing.yaml")
	t.Setenv("LP_BASE_URL", "http://localhost:8080")
	if code, _, errOut := runCmd(t, "migrate", "status"); code != 1 || !strings.Contains(errOut, "db.host") {
		t.Errorf("migrate without db: %d %q", code, errOut)
	}
}

func TestHealthcheckCommand(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	if code, _, errOut := runCmd(t, "healthcheck", "-addr", ":"+u.Port()); code != 0 {
		t.Errorf("healthy: %d %q", code, errOut)
	}
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	t.Setenv("LP_SERVER__ADDR", net.JoinHostPort("", strconv.Itoa(port)))
	if code, _, _ := runCmd(t, "healthcheck"); code != 1 {
		t.Errorf("unreachable should fail: %d", code)
	}
	if code, _, _ := runCmd(t, "healthcheck", "-addr", "nonsense"); code != 1 {
		t.Errorf("bad addr: %d", code)
	}
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
	defer bad.Close()
	ub, _ := url.Parse(bad.URL)
	if code, _, _ := runCmd(t, "healthcheck", "-addr", ":"+ub.Port()); code != 1 {
		t.Errorf("503 should fail: %d", code)
	}
}

func TestHealthcheckReadsConfigFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	cfgFile := t.TempDir() + "/config.yaml"
	if err := os.WriteFile(cfgFile, []byte("server:\n  addr: '127.0.0.1:"+u.Port()+"'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LP_CONFIG_FILE", cfgFile)
	t.Setenv("LP_SERVER__ADDR", "")
	if code, _, errOut := runCmd(t, "healthcheck"); code != 0 {
		t.Errorf("healthcheck with server.addr from config file: %d %q", code, errOut)
	}
}

func TestProbeTarget(t *testing.T) {
	tests := map[string]string{
		":8080":          "127.0.0.1:8080",
		"0.0.0.0:9090":   "127.0.0.1:9090",
		"[::]:9090":      "127.0.0.1:9090",
		"10.0.0.5:8080":  "10.0.0.5:8080",
		"[::1]:8080":     "[::1]:8080",
		"localhost:8080": "localhost:8080",
	}
	for in, want := range tests {
		if got, err := probeTarget(in); err != nil || got != want {
			t.Errorf("probeTarget(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"nonsense", "host:"} {
		if _, err := probeTarget(bad); err == nil {
			t.Errorf("probeTarget(%q) should fail", bad)
		}
	}
}

func TestProbeDataDir(t *testing.T) {
	if err := probeDataDir(t.TempDir()); err != nil {
		t.Error(err)
	}
	if err := probeDataDir("/definitely/not/here"); err == nil {
		t.Error("missing dir should fail")
	}
}
