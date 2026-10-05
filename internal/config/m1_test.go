package config

import (
	"bytes"
	"strings"
	"testing"
)

func TestM1Defaults(t *testing.T) {
	c := mustLoad(t, nil, testBaseURL).Config
	if c.Providers.Discord.APIBase != DefaultDiscordAPIBase || c.Providers.KOOK.APIBase != DefaultKOOKAPIBase {
		t.Errorf("api bases = %q %q", c.Providers.Discord.APIBase, c.Providers.KOOK.APIBase)
	}
	if c.Uploads.MaxBytes != DefaultUploadMaxBytes || c.SeedFile != "" || c.Edge.ProxyAuth.IsSet() {
		t.Errorf("uploads=%d seed=%q edge=%v", c.Uploads.MaxBytes, c.SeedFile, c.Edge.ProxyAuth.IsSet())
	}
	for _, k := range []string{
		"seed_file", "uploads.max_bytes", "edge.proxy_auth", "edge.proxy_auth_file",
		"providers.discord.api_base", "providers.kook.api_base",
	} {
		if !knownKeys()[k] {
			t.Errorf("missing known key %s", k)
		}
	}
}

func TestM1FromEnvAndFile(t *testing.T) {
	f := newFakeFS(map[string]string{
		"/etc/linkspage/config.yaml": "seed_file: /etc/linkspage/seed.yaml\nproviders:\n  kook:\n    api_base: http://127.0.0.1:9999\n",
		"/run/secrets/edge":          strings.Repeat("e", 40) + "\n",
	})
	c := mustLoad(t, f, testBaseURL,
		"LP_PROVIDERS__DISCORD__API_BASE=http://127.0.0.1:8888/mock",
		"LP_UPLOADS__MAX_BYTES=1048576",
		"LP_EDGE__PROXY_AUTH_FILE=/run/secrets/edge",
	).Config
	if c.SeedFile != "/etc/linkspage/seed.yaml" || c.Providers.KOOK.APIBase != "http://127.0.0.1:9999" ||
		c.Providers.Discord.APIBase != "http://127.0.0.1:8888/mock" || c.Uploads.MaxBytes != 1<<20 {
		t.Errorf("config = %+v", c)
	}
	if c.Edge.ProxyAuth.Reveal() != strings.Repeat("e", 40) {
		t.Error("edge.proxy_auth_file not resolved")
	}
	l := mustLoad(t, f, testBaseURL, "LP_EDGE__PROXY_AUTH="+strings.Repeat("p", 32))
	if l.Config.Edge.ProxyAuth.Reveal() != strings.Repeat("p", 32) {
		t.Error("plain edge.proxy_auth lost")
	}
}

func TestM1ShadowedProxyAuthFile(t *testing.T) {
	f := newFakeFS(map[string]string{
		"/etc/linkspage/config.yaml": "edge:\n  proxy_auth_file: /run/secrets/edge\n",
		"/run/secrets/edge":          strings.Repeat("f", 40),
	})
	l := mustLoad(t, f, testBaseURL, "LP_EDGE__PROXY_AUTH="+strings.Repeat("e", 32))
	if l.Config.Edge.ProxyAuth.Reveal() != strings.Repeat("e", 32) || !hasWarning(l.Warnings, "edge.proxy_auth_file is ignored") {
		t.Errorf("proxy auth = %v warnings=%v", l.Config.Edge.ProxyAuth, l.Warnings)
	}
	missing := newFakeFS(map[string]string{"/etc/linkspage/config.yaml": "edge:\n  proxy_auth_file: /nope\n"})
	if _, err := load(t, missing, testBaseURL); err == nil || !strings.Contains(err.Error(), "edge.proxy_auth_file") {
		t.Errorf("missing file err = %v", err)
	}
}

func TestM1Validation(t *testing.T) {
	cases := []struct{ name, env, want string }{
		{"api base scheme", "LP_PROVIDERS__DISCORD__API_BASE=ftp://x", "providers.discord.api_base: must be an absolute"},
		{"api base relative", "LP_PROVIDERS__KOOK__API_BASE=/api", "providers.kook.api_base"},
		{"api base slash", "LP_PROVIDERS__KOOK__API_BASE=https://x.example/", "trailing slash"},
		{"api base query", "LP_PROVIDERS__DISCORD__API_BASE=https://x.example?a=1", "query"},
		{"api base creds", "LP_PROVIDERS__DISCORD__API_BASE=https://u:p@x.example", "credentials"},
		{"uploads small", "LP_UPLOADS__MAX_BYTES=10", "uploads.max_bytes"},
		{"uploads big", "LP_UPLOADS__MAX_BYTES=999999999", "uploads.max_bytes"},
		{"seed ext", "LP_SEED_FILE=/etc/linkspage/seed.json", "seed_file"},
		{"proxy auth short", "LP_EDGE__PROXY_AUTH=short", "edge.proxy_auth: must be at least"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := load(t, nil, testBaseURL, tc.env)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
			if strings.Contains(err.Error(), "short") && tc.name == "proxy auth short" {
				t.Error("error leaks the secret value")
			}
		})
	}
	for _, ok := range []string{"LP_SEED_FILE=./seed.yml", "LP_SEED_FILE=/x/SEED.YAML"} {
		if _, err := load(t, nil, testBaseURL, ok); err != nil {
			t.Errorf("%s: %v", ok, err)
		}
	}
}

func TestDeriveKey(t *testing.T) {
	c := &Config{SecretKey: Secret(strings.Repeat("k", 32))}
	a, err := c.DeriveKey(KeyInfoMediaProxy, 32)
	if err != nil || len(a) != 32 {
		t.Fatalf("derive = %x err=%v", a, err)
	}
	b, _ := c.DeriveKey(KeyInfoMediaProxy, 32)
	v, _ := c.DeriveKey(KeyInfoVisitor, 32)
	if !bytes.Equal(a, b) || bytes.Equal(a, v) {
		t.Error("derivation must be deterministic and info-separated")
	}
	errCases := []struct {
		name string
		cfg  *Config
		info string
		size int
	}{
		{"no secret", &Config{}, KeyInfoDevice, 32},
		{"no info", c, "", 32},
		{"small", c, KeyInfoDevice, 8},
		{"large", c, KeyInfoDevice, 65},
	}
	for _, tc := range errCases {
		if _, err := tc.cfg.DeriveKey(tc.info, tc.size); err == nil {
			t.Errorf("%s: expected error", tc.name)
		}
	}
}
