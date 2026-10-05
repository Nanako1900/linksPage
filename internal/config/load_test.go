package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

const testBaseURL = "LP_BASE_URL=http://localhost:8080"

// fakeFS provides ReadFile/Stat backed by an in-memory map; paths are
// stored without the leading slash.
type fakeFS struct {
	files fstest.MapFS
	errs  map[string]error
}

func newFakeFS(files map[string]string) *fakeFS {
	m := fstest.MapFS{}
	for p, c := range files {
		m[strings.TrimPrefix(p, "/")] = &fstest.MapFile{Data: []byte(c), Mode: 0o640}
	}
	return &fakeFS{files: m, errs: map[string]error{}}
}

func (f *fakeFS) ReadFile(p string) ([]byte, error) {
	if err, ok := f.errs[p]; ok {
		return nil, err
	}
	return f.files.ReadFile(strings.TrimPrefix(p, "/"))
}

func (f *fakeFS) Stat(p string) (fs.FileInfo, error) {
	return f.files.Stat(strings.TrimPrefix(p, "/"))
}

func load(t *testing.T, f *fakeFS, env ...string) (*Loaded, error) {
	t.Helper()
	if f == nil {
		f = newFakeFS(nil)
	}
	return Load(Options{Environ: env, ReadFile: f.ReadFile, Stat: f.Stat})
}

func mustLoad(t *testing.T, f *fakeFS, env ...string) *Loaded {
	t.Helper()
	l, err := load(t, f, env...)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return l
}

func hasWarning(ws []string, sub string) bool {
	for _, w := range ws {
		if strings.Contains(w, sub) {
			return true
		}
	}
	return false
}

func TestLoadDefaults(t *testing.T) {
	l := mustLoad(t, nil, testBaseURL)
	c := l.Config
	if l.FileFound || l.File != DefaultConfigFile {
		t.Errorf("file = %q found=%v", l.File, l.FileFound)
	}
	checks := map[string][2]any{
		"base_url":         {c.BaseURL, "http://localhost:8080"},
		"server.addr":      {c.Server.Addr, ":8080"},
		"db.port":          {c.DB.Port, 5432},
		"db.sslmode":       {c.DB.SSLMode, "disable"},
		"db.auto_migrate":  {c.DB.AutoMigrate, true},
		"data_dir":         {c.DataDir, "/data"},
		"client_ip_header": {c.ClientIPHeader, "CF-Connecting-IP"},
		"timezone":         {c.Analytics.Timezone, "Asia/Shanghai"},
		"log.level":        {c.Log.Level, "info"},
		"log.format":       {c.Log.Format, "json"},
		"idle_timeout":     {c.Auth.Session.IdleTimeout, 24 * time.Hour},
		"local.enabled":    {c.Auth.Local.Enabled, true},
	}
	for name, v := range checks {
		if v[0] != v[1] {
			t.Errorf("%s = %v, want %v", name, v[0], v[1])
		}
	}
	if len(c.TrustedProxies) != 1 || c.TrustedProxies[0] != "172.31.255.2/32" {
		t.Errorf("trusted_proxies = %v", c.TrustedProxies)
	}
	if !hasWarning(l.Warnings, "no administrators") || !hasWarning(l.Warnings, "db.host is not set") {
		t.Errorf("warnings = %v", l.Warnings)
	}
}

func TestLoadPrecedence(t *testing.T) {
	f := newFakeFS(map[string]string{
		"/etc/linkspage/config.yaml": "base_url: https://file.example.com\nserver:\n  addr: ':9000'\ndb:\n  host: filehost\n  user: u\n  name: n\nlog:\n  level: debug\n",
		"/run/secrets/pg":            "pg-secret\n\n",
		"/run/secrets/base":          "https://filevar.example.com \n",
	})
	l := mustLoad(t, f,
		"LP_DB__HOST=envhost",
		"LP_DB__PASSWORD=env-pass",
		"LP_DB__PASSWORD_FILE=/run/secrets/pg",
		"LP_BASE_URL=https://env.example.com",
		"LP_BASE_URL_FILE=/run/secrets/base",
		"LP_TRUSTED_PROXIES=10.0.0.1/32, cloudflare ,",
		"LP_DB__AUTO_MIGRATE=false",
		"LP_PROVIDERS__HTTP_PROXY=",
		"OTHER=ignored",
	)
	c := l.Config
	if !l.FileFound {
		t.Fatal("file not found")
	}
	if c.Server.Addr != ":9000" || c.Log.Level != "debug" {
		t.Errorf("file values not applied: %+v %+v", c.Server, c.Log)
	}
	if c.DB.Host != "envhost" {
		t.Errorf("db.host = %q, want env override", c.DB.Host)
	}
	if c.DB.Password.Reveal() != "pg-secret" {
		t.Errorf("db.password not read from password_file")
	}
	if c.BaseURL != "https://filevar.example.com" {
		t.Errorf("base_url = %q, want *_FILE override", c.BaseURL)
	}
	if got := strings.Join(c.TrustedProxies, "|"); got != "10.0.0.1/32|cloudflare" {
		t.Errorf("trusted_proxies = %q", got)
	}
	if c.DB.AutoMigrate {
		t.Error("auto_migrate should be false")
	}
	if c.Providers.HTTPProxy != "" {
		t.Error("empty env value should be treated as unset")
	}
}

func TestLoadCustomConfigPath(t *testing.T) {
	f := newFakeFS(map[string]string{"/cfg/c.yaml": "base_url: http://127.0.0.1:8080\n"})
	l := mustLoad(t, f, "LP_CONFIG_FILE=/cfg/c.yaml")
	if l.File != "/cfg/c.yaml" || l.Config.BaseURL != "http://127.0.0.1:8080" {
		t.Errorf("got %q %q", l.File, l.Config.BaseURL)
	}
}

func TestLoadFileErrors(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(f *fakeFS)
		wantSub string
	}{
		{"permission", func(f *fakeFS) { f.errs["/etc/linkspage/config.yaml"] = fs.ErrPermission }, "chmod 640"},
		{"io error", func(f *fakeFS) { f.errs["/etc/linkspage/config.yaml"] = errors.New("boom") }, "boom"},
		{"yaml", func(f *fakeFS) {
			f.files["etc/linkspage/config.yaml"] = &fstest.MapFile{Data: []byte("a: [\n")}
		}, "parse config file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeFS(nil)
			tt.setup(f)
			_, err := load(t, f, testBaseURL)
			if err == nil || !strings.Contains(err.Error(), tt.wantSub) {
				t.Fatalf("err = %v, want containing %q", err, tt.wantSub)
			}
		})
	}
}

func TestLoadEmptyYAML(t *testing.T) {
	f := newFakeFS(map[string]string{"/etc/linkspage/config.yaml": "# only a comment\n"})
	if l := mustLoad(t, f, testBaseURL); !l.FileFound {
		t.Error("expected file found")
	}
}

func TestLoadHashValues(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		env     []string
		wantSub string
	}{
		{"env", "", []string{testBaseURL, "LP_DB__HOST=# the db host"}, "LP_DB__HOST: value starts with '#'"},
		{"yaml", "db:\n  host: '#x'\n", []string{testBaseURL}, "db.host: value starts with '#'"},
		{"yaml list", "trusted_proxies: ['#c']\n", []string{testBaseURL}, "trusted_proxies[0]: value starts"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeFS(map[string]string{"/etc/linkspage/config.yaml": tt.file})
			_, err := load(t, f, tt.env...)
			if err == nil || !strings.Contains(err.Error(), tt.wantSub) {
				t.Fatalf("err = %v, want %q", err, tt.wantSub)
			}
		})
	}
}

func TestLoadUnknownKeysWarn(t *testing.T) {
	f := newFakeFS(map[string]string{"/etc/linkspage/config.yaml": "colour: red\ndb:\n  hots: x\n"})
	l := mustLoad(t, f, testBaseURL, "LP_NOPE=1", "LP_DB__HOTS=1")
	for _, want := range []string{"LP_NOPE", "LP_DB__HOTS", "unknown config key colour", "unknown config key db.hots"} {
		if !hasWarning(l.Warnings, want) {
			t.Errorf("missing warning %q in %v", want, l.Warnings)
		}
	}
}

func TestLoadEnvFileErrors(t *testing.T) {
	_, err := load(t, nil, testBaseURL, "LP_SECRET_KEY_FILE=/missing", "LP_DB__PASSWORD_FILE=/nope")
	if err == nil || !strings.Contains(err.Error(), "secret_key_file") || !strings.Contains(err.Error(), "db.password_file") {
		t.Fatalf("err = %v", err)
	}
	_, err = load(t, nil, testBaseURL, "LP_LOG__LEVEL_FILE=/missing")
	if err == nil || !strings.Contains(err.Error(), "LP_LOG__LEVEL_FILE") {
		t.Fatalf("generic _FILE err = %v", err)
	}
}

func TestLoadDecodeErrorsHideValues(t *testing.T) {
	_, err := load(t, nil, testBaseURL, "LP_DB__PORT=notaport-SECRETISH")
	if err == nil || !strings.Contains(err.Error(), "db.port") {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "SECRETISH") {
		t.Errorf("error leaks value: %v", err)
	}
}

func TestLoadWorldReadableWarning(t *testing.T) {
	f := newFakeFS(map[string]string{"/etc/linkspage/config.yaml": "base_url: http://localhost:8080\nauth:\n  local:\n    users:\n      - {username: a, password_hash: '" + validPHC(47104, 1, 1) + "', role: owner}\n"})
	f.files["etc/linkspage/config.yaml"].Mode = 0o644
	l := mustLoad(t, f)
	if !hasWarning(l.Warnings, "world-readable") {
		t.Errorf("warnings = %v", l.Warnings)
	}
}

func TestLoadWorldReadableIgnoresCommentedHashes(t *testing.T) {
	f := newFakeFS(map[string]string{"/etc/linkspage/config.yaml": "base_url: http://localhost:8080\nauth:\n  local:\n    users: []\n    # - {username: a, password_hash: x, role: owner}\n"})
	f.files["etc/linkspage/config.yaml"].Mode = 0o644
	l := mustLoad(t, f)
	if hasWarning(l.Warnings, "world-readable") {
		t.Errorf("unexpected world-readable warning: %v", l.Warnings)
	}
}

func TestLoadSecretFiles(t *testing.T) {
	f := newFakeFS(map[string]string{
		"/etc/linkspage/config.yaml": `base_url: http://localhost:8080
secret_key_file: /run/secrets/key
auth:
  local:
    users:
      - username: helper
        password_hash_file: /run/secrets/h
        role: editor
      - username: boss
        password_hash: "` + validPHC(47104, 1, 1) + `"
        role: owner
  oauth:
    providers:
      - {id: gh, type: github, client_id: x, client_secret_file: /run/secrets/gh}
    admins:
      - {provider: gh, subject: "1", role: editor}
`,
		"/run/secrets/key": strings.Repeat("k", 40) + "\n",
		"/run/secrets/h":   validPHC(47104, 1, 1) + "\n",
		"/run/secrets/gh":  "ghsecret\n",
	})
	l := mustLoad(t, f)
	c := l.Config
	if c.SecretKey.Reveal() != strings.Repeat("k", 40) {
		t.Error("secret_key_file not applied")
	}
	if c.Auth.Local.Users[0].PasswordHash.Reveal() != validPHC(47104, 1, 1) {
		t.Error("password_hash_file not applied")
	}
	if c.Auth.OAuth.Providers[0].ClientSecret.Reveal() != "ghsecret" {
		t.Error("client_secret_file not applied")
	}
	if c.AdminCount() != 3 || hasWarning(l.Warnings, "owner") || hasWarning(l.Warnings, "no administrators") {
		t.Errorf("admins=%d warnings=%v", c.AdminCount(), l.Warnings)
	}
}

func TestLoadFilePrecedence(t *testing.T) {
	yaml := "base_url: http://localhost:8080\ndb:\n  host: h\n  user: u\n  name: n\n  password_file: /nonexistent\nsecret_key_file: /nonexistent\n"
	f := newFakeFS(map[string]string{
		"/etc/linkspage/config.yaml": yaml,
		"/run/secrets/pg":            "file-pass\n",
	})
	// Env plain values beat *_file companions from the lower YAML layer.
	l := mustLoad(t, f, "LP_DB__PASSWORD=envpw", "LP_SECRET_KEY="+strings.Repeat("s", 40))
	if l.Config.DB.Password.Reveal() != "envpw" || l.Config.DB.PasswordFile != "" {
		t.Errorf("db.password = %q file=%q", l.Config.DB.Password.Reveal(), l.Config.DB.PasswordFile)
	}
	if l.Config.SecretKey.Reveal() != strings.Repeat("s", 40) {
		t.Error("env secret_key must win over YAML secret_key_file")
	}
	if !hasWarning(l.Warnings, "db.password_file is ignored") || !hasWarning(l.Warnings, "secret_key_file is ignored") {
		t.Errorf("warnings = %v", l.Warnings)
	}

	// A higher-layer *_file still beats a lower-layer plain value.
	f = newFakeFS(map[string]string{
		"/etc/linkspage/config.yaml": "base_url: http://localhost:8080\ndb:\n  host: h\n  user: u\n  name: n\n  password: yamlpw\n",
		"/run/secrets/pg":            "file-pass\n",
	})
	l = mustLoad(t, f, "LP_DB__PASSWORD_FILE=/run/secrets/pg")
	if l.Config.DB.Password.Reveal() != "file-pass" || hasWarning(l.Warnings, "is ignored") {
		t.Errorf("db.password = %q warnings=%v", l.Config.DB.Password.Reveal(), l.Warnings)
	}

	// LP_*_FILE contents of the plain key rank above everything.
	l = mustLoad(t, f, "LP_DB__PASSWORD_FILE=/run/secrets/pg", "LP_DB__PASSWORD=envpw")
	if l.Config.DB.Password.Reveal() != "file-pass" {
		t.Errorf("same-layer *_FILE must win: %q", l.Config.DB.Password.Reveal())
	}
}

func TestProxyURLRedaction(t *testing.T) {
	f := newFakeFS(map[string]string{"/etc/linkspage/config.yaml": "base_url: http://localhost:8080\n"})
	l := mustLoad(t, f, "LP_PROVIDERS__HTTP_PROXY=http://user:hunter2@proxy.local:7890")
	p := l.Config.Providers.HTTPProxy
	if p.Reveal() != "http://user:hunter2@proxy.local:7890" {
		t.Fatalf("raw proxy = %q", p.Reveal())
	}
	b, err := json.Marshal(l.Config)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{string(b), p.String(), fmt.Sprintf("%v %#v", p, p)} {
		if strings.Contains(s, "hunter2") {
			t.Errorf("proxy password leaked: %s", s)
		}
	}
	if !strings.Contains(p.String(), "proxy.local:7890") || ProxyURL("").String() != "" || ProxyURL("http://[::1").String() != redacted {
		t.Errorf("unexpected redaction: %q", p.String())
	}
	if txt, _ := p.MarshalText(); strings.Contains(string(txt), "hunter2") || strings.Contains(p.LogValue().String(), "hunter2") {
		t.Error("text/log forms must redact")
	}
}

func TestHSTSWarning(t *testing.T) {
	f := newFakeFS(map[string]string{"/etc/linkspage/config.yaml": "base_url: http://localhost:8080\nhsts: true\n"})
	if l := mustLoad(t, f); !l.Config.HSTS || !hasWarning(l.Warnings, "hsts is enabled but base_url is not https") {
		t.Errorf("warnings = %v", l.Warnings)
	}
}

func TestServerAddr(t *testing.T) {
	file := newFakeFS(map[string]string{"/etc/linkspage/config.yaml": "server:\n  addr: ':9090'\n"})
	bad := newFakeFS(map[string]string{"/etc/linkspage/config.yaml": "server: [\n"})
	tests := []struct {
		name string
		fs   *fakeFS
		env  []string
		want string
	}{
		{"default", newFakeFS(nil), nil, ":8080"},
		{"file", file, nil, ":9090"},
		{"env wins", file, []string{"LP_SERVER__ADDR=127.0.0.1:7000"}, "127.0.0.1:7000"},
		{"invalid file falls back", bad, nil, ":8080"},
		{"custom file path", file, []string{"LP_CONFIG_FILE=/missing.yaml"}, ":8080"},
	}
	for _, tt := range tests {
		env := append([]string{}, tt.env...) // never nil: nil means os.Environ
		got := ServerAddr(Options{Environ: env, ReadFile: tt.fs.ReadFile, Stat: tt.fs.Stat})
		if got != tt.want {
			t.Errorf("%s: ServerAddr = %q, want %q", tt.name, got, tt.want)
		}
	}
}
