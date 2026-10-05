package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func errorsIs(err, target error) bool { return errors.Is(err, target) }

func TestValidateFields(t *testing.T) {
	tests := []struct {
		name    string
		env     []string
		wantErr string
	}{
		{"missing base_url", nil, "base_url: required"},
		{"relative base_url", []string{"LP_BASE_URL=links.example.com"}, "base_url: must be an absolute"},
		{"ftp base_url", []string{"LP_BASE_URL=ftp://x.example.com"}, "base_url: must be an absolute"},
		{"trailing slash", []string{"LP_BASE_URL=https://x.example.com/"}, "must not end with a slash"},
		{"path", []string{"LP_BASE_URL=https://x.example.com/links"}, "without a path"},
		{"query", []string{"LP_BASE_URL=https://x.example.com?a=1"}, "must not contain credentials"},
		{"userinfo", []string{"LP_BASE_URL=https://u:p@x.example.com"}, "must not contain credentials"},
		{"addr", []string{testBaseURL, "LP_SERVER__ADDR=8080"}, "server.addr: must be host:port"},
		{"addr port", []string{testBaseURL, "LP_SERVER__ADDR=:99999"}, "server.addr: invalid port"},
		{"db port", []string{testBaseURL, "LP_DB__PORT=0"}, "db.port"},
		{"sslmode", []string{testBaseURL, "LP_DB__SSLMODE=maybe"}, "db.sslmode"},
		{"data_dir", []string{testBaseURL, "LP_DATA_DIR= "}, "data_dir"},
		{"short secret", []string{testBaseURL, "LP_SECRET_KEY=short"}, "secret_key: must be at least 32 bytes"},
		{"trusted none mix", []string{testBaseURL, "LP_TRUSTED_PROXIES=none,10.0.0.0/8"}, "trusted_proxies"},
		{"trusted bad", []string{testBaseURL, "LP_TRUSTED_PROXIES=dns:proxy"}, "trusted_proxies"},
		{"header", []string{testBaseURL, "LP_CLIENT_IP_HEADER=Bad Header"}, "client_ip_header"},
		{"proxy scheme", []string{testBaseURL, "LP_PROVIDERS__HTTP_PROXY=ftp://p:1"}, "providers.http_proxy: scheme"},
		{"proxy url", []string{testBaseURL, "LP_PROVIDERS__HTTP_PROXY=http://user:SECRET@"}, "providers.http_proxy: invalid proxy URL"},
		{"timezone", []string{testBaseURL, "LP_ANALYTICS__TIMEZONE=Mars/Base"}, "analytics.timezone"},
		{"log level", []string{testBaseURL, "LP_LOG__LEVEL=trace"}, "log.level"},
		{"log format", []string{testBaseURL, "LP_LOG__FORMAT=xml"}, "log.format"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := load(t, nil, tt.env...)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want %q", err, tt.wantErr)
			}
			if strings.Contains(err.Error(), "SECRET") {
				t.Errorf("error leaks value: %v", err)
			}
		})
	}
}

func TestValidateAcceptsVariants(t *testing.T) {
	envs := [][]string{
		{"LP_BASE_URL=https://links.example.com:8443", "LP_TRUSTED_PROXIES=none", "LP_PROVIDERS__HTTP_PROXY=socks5://127.0.0.1:1080", "LP_LOG__FORMAT=text", "LP_LOG__LEVEL=debug"},
		{testBaseURL, "LP_CLIENT_IP_HEADER=X-Forwarded-For", "LP_TRUSTED_PROXIES=172.31.255.1/32,cloudflare", "LP_DB__SSLMODE=verify-full"},
	}
	for i, env := range envs {
		if _, err := load(t, nil, env...); err != nil {
			t.Errorf("case %d: %v", i, err)
		}
	}
}

func TestSecretRedaction(t *testing.T) {
	s := Secret("super-secret-value")
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	logger.Info("x", "secret", s, "cfg", DB{Password: s})
	j, err := json.Marshal(struct{ S Secret }{s})
	if err != nil {
		t.Fatal(err)
	}
	txt, _ := s.MarshalText()
	outputs := []string{fmt.Sprint(s), fmt.Sprintf("%v %+v %#v %s", s, s, s, s), string(j), buf.String(), string(txt)}
	for _, out := range outputs {
		if strings.Contains(out, "super-secret-value") {
			t.Errorf("secret leaked: %s", out)
		}
	}
	if s.Reveal() != "super-secret-value" || !s.IsSet() {
		t.Error("Reveal broken")
	}
	if Secret("").String() != "" || Secret("").IsSet() {
		t.Error("empty secret should be empty")
	}
}

func TestEnsureSecretKey(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{DataDir: dir}

	c1, generated, err := cfg.EnsureSecretKey()
	if err != nil || !generated {
		t.Fatalf("first: generated=%v err=%v", generated, err)
	}
	if len(c1.SecretKey.Reveal()) != 64 || cfg.SecretKey.IsSet() {
		t.Fatalf("bad key or input mutated")
	}
	fi, err := os.Stat(filepath.Join(dir, SecretKeyFileName))
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v err=%v", fi.Mode(), err)
	}

	c2, generated, err := cfg.EnsureSecretKey()
	if err != nil || generated || c2.SecretKey != c1.SecretKey {
		t.Fatalf("second call should reuse key: generated=%v err=%v", generated, err)
	}

	preset := &Config{DataDir: dir, SecretKey: Secret(strings.Repeat("p", 32))}
	c3, generated, err := preset.EnsureSecretKey()
	if err != nil || generated || c3.SecretKey != preset.SecretKey {
		t.Fatal("configured key should be kept")
	}

	short := t.TempDir()
	if err := os.WriteFile(filepath.Join(short, SecretKeyFileName), []byte("abc\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := (&Config{DataDir: short}).EnsureSecretKey(); err == nil {
		t.Error("short key file should fail")
	}

	missing := &Config{DataDir: filepath.Join(dir, "does", "not", "exist")}
	if _, _, err := missing.EnsureSecretKey(); err == nil {
		t.Error("unwritable dir should fail")
	}
}

func TestEnsureSecretKeyFixesMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, SecretKeyFileName)
	if err := os.WriteFile(path, []byte(strings.Repeat("a", 64)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := (&Config{DataDir: dir}).EnsureSecretKey(); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v err=%v", fi.Mode(), err)
	}
}

func TestEnsureSecretKeyConcurrent(t *testing.T) {
	dir := t.TempDir()
	const n = 8
	keys := make([]Secret, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			c, _, err := (&Config{DataDir: dir}).EnsureSecretKey()
			errs[i] = err
			if err == nil {
				keys[i] = c.SecretKey
			}
		})
	}
	wg.Wait()
	for i := range n {
		if errs[i] != nil || keys[i] != keys[0] {
			t.Fatalf("instance %d: err=%v, keys must agree", i, errs[i])
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary files left behind: %v %v", entries, err)
	}
}

func TestKnownKeys(t *testing.T) {
	k := knownKeys()
	for _, want := range []string{"base_url", "db.password_file", "auth.local.enabled", "auth.local.users", "auth.session.idle_timeout", "trusted_proxies"} {
		if !k[want] {
			t.Errorf("missing known key %s", want)
		}
	}
	if k["db"] || k["auth"] {
		t.Error("struct keys should not be leaves")
	}
}
