package config

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
)

func validPHC(m, t, p int) string {
	salt := base64.RawStdEncoding.EncodeToString([]byte("0123456789abcdef"))
	key := base64.RawStdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", m, t, p, salt, key)
}

func authYAML(body string) map[string]string {
	return map[string]string{"/etc/linkspage/config.yaml": "base_url: http://localhost:8080\nauth:\n" + body}
}

func TestAuthValidation(t *testing.T) {
	good := validPHC(47104, 1, 1)
	tests := []struct {
		name     string
		body     string
		wantErr  []string
		wantWarn []string
		leak     string
	}{
		{
			name:     "valid local owner",
			body:     "  local:\n    users:\n      - {username: nanako, password_hash: '" + good + "', role: owner}\n",
			wantWarn: nil,
		},
		{
			name:    "plaintext password",
			body:    "  local:\n    users:\n      - {username: a, password: hunter2-PLAINTEXT, role: owner}\n",
			wantErr: []string{"auth.local.users[0].password: plaintext passwords are not accepted", "argon2id PHC string"},
			leak:    "hunter2-PLAINTEXT",
		},
		{
			name:    "unknown key",
			body:    "  local:\n    users:\n      - {username: a, passwd: x, password_hash: '" + good + "', role: owner}\n  sesion: {}\n",
			wantErr: []string{"auth.local.users[0].passwd: unknown key", "auth.sesion: unknown key"},
		},
		{
			name:    "bad phc",
			body:    "  local:\n    users:\n      - {username: a, password_hash: '$2a$10$LEAKYBCRYPTHASH', role: owner}\n",
			wantErr: []string{"auth.local.users[0].password_hash: not a valid argon2id PHC string"},
			leak:    "LEAKYBCRYPTHASH",
		},
		{
			name:    "phc over limits",
			body:    "  local:\n    users:\n      - {username: a, password_hash: '" + validPHC(131072, 1, 1) + "', role: owner}\n",
			wantErr: []string{"auth.local.users[0].password_hash: argon2id parameters exceed"},
		},
		{
			name:     "weak phc warns",
			body:     "  local:\n    users:\n      - {username: a, password_hash: '" + validPHC(4096, 1, 1) + "', role: owner}\n",
			wantWarn: []string{"below the recommended strength"},
		},
		{
			name:    "hash and file",
			body:    "  local:\n    users:\n      - {username: a, password_hash: '" + good + "', password_hash_file: /x, role: owner}\n",
			wantErr: []string{"auth.local.users[0]: set exactly one of password_hash or password_hash_file"},
		},
		{
			name:    "neither hash nor file",
			body:    "  local:\n    users:\n      - {username: a, role: owner}\n",
			wantErr: []string{"set exactly one of password_hash"},
		},
		{
			name:    "missing hash file",
			body:    "  local:\n    users:\n      - {username: a, password_hash_file: /missing, role: owner}\n",
			wantErr: []string{"auth.local.users[0].password_hash_file: cannot read file"},
		},
		{
			name: "duplicate users and bad roles",
			body: "  local:\n    users:\n      - {username: Admin, password_hash: '" + good + "', role: owner}\n" +
				"      - {username: admin, password_hash: '" + good + "', role: root}\n      - {username: 'bad name', password_hash: '" + good + "', role: editor}\n",
			wantErr: []string{"users[1].username: duplicate", "users[1].role: must be owner or editor", "users[2].username: must match"},
		},
		{
			name:     "local disabled ignores users",
			body:     "  local:\n    enabled: false\n    users:\n      - {username: a, password_hash: 'garbage', role: owner}\n",
			wantWarn: []string{"no administrators"},
		},
		{
			name:     "editor only",
			body:     "  local:\n    users:\n      - {username: a, password_hash: '" + good + "', role: editor}\n",
			wantWarn: []string{"no administrator has the owner role"},
		},
		{
			name: "oauth valid",
			body: "  oauth:\n    providers:\n      - {id: gh, type: github, client_id: c, client_secret: s}\n" +
				"      - {id: sso, type: oidc, issuer: 'https://auth.example.com/app/', client_id: c, client_secret: s, scopes: [openid, email]}\n" +
				"    admins:\n      - {provider: gh, subject: '1', role: owner}\n      - {provider: sso, group: admins, role: editor}\n      - {provider: gh, email: a@b.c, role: editor}\n",
			wantWarn: []string{"admins[2]: email matching for github"},
		},
		{
			name: "oauth errors",
			body: "  oauth:\n    providers:\n      - {id: GH, type: gitlab, client_id: ''}\n      - {id: sso, type: oidc, issuer: 'http://auth.example.com', client_id: c, client_secret: s}\n" +
				"      - {id: sso, type: oidc, issuer: '::bad', client_id: c, client_secret: s}\n" +
				"    admins:\n      - {provider: nope, subject: '1', role: owner}\n      - {provider: sso, subject: '1', email: e@x, role: editor}\n" +
				"      - {provider: sso, subject: '2', role: owner}\n      - {provider: sso, subject: '2', role: boss}\n",
			wantErr: []string{
				"providers[0].id: must match", "providers[0].type: must be oidc", "providers[0].client_id: required",
				"providers[0]: set exactly one of client_secret", "providers[1].issuer: must use https",
				"providers[2].id: duplicate", "providers[2].issuer: must be an absolute URL",
				"admins[0].provider: does not reference", "admins[1]: set exactly one of subject",
				"admins[3]: duplicate admin identity", "admins[3].role: must be owner or editor",
			},
		},
		{
			name:    "session durations",
			body:    "  session: {idle_timeout: 0s, reauth_window: -1m}\n",
			wantErr: []string{"auth.session.idle_timeout: must be a positive duration", "auth.session.reauth_window"},
		},
		{
			name:    "auth not a map",
			body:    "  - x\n",
			wantErr: []string{"auth: must be a mapping"},
		},
		{
			name:    "bad type in auth",
			body:    "  local:\n    enabled: maybe-LEAK\n",
			wantErr: []string{"auth.local.enabled: invalid value type"},
			leak:    "maybe-LEAK",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l, err := load(t, newFakeFS(authYAML(tt.body)))
			if len(tt.wantErr) == 0 {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				for _, w := range tt.wantWarn {
					if !hasWarning(l.Warnings, w) {
						t.Errorf("missing warning %q in %v", w, l.Warnings)
					}
				}
				return
			}
			if err == nil {
				t.Fatal("expected error")
			}
			for _, w := range tt.wantErr {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error missing %q:\n%v", w, err)
				}
			}
			if tt.leak != "" && strings.Contains(err.Error(), tt.leak) {
				t.Errorf("error leaks value %q: %v", tt.leak, err)
			}
		})
	}
}

func TestParseArgon2idPHC(t *testing.T) {
	salt := base64.RawStdEncoding.EncodeToString([]byte("0123456789abcdef"))
	key := base64.RawStdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	tests := []struct {
		name    string
		in      string
		wantErr error
		weak    bool
	}{
		{"owasp default", validPHC(47104, 1, 1), nil, false},
		{"max limits", validPHC(65536, 5, 2), nil, false},
		{"alt owasp", validPHC(19456, 2, 1), nil, false},
		{"weak", validPHC(1024, 1, 1), nil, true},
		{"memory over", validPHC(65537, 1, 1), ErrPHCLimits, false},
		{"time over", validPHC(47104, 6, 1), ErrPHCLimits, false},
		{"threads over", validPHC(47104, 1, 3), ErrPHCLimits, false},
		{"argon2i", "$argon2i$v=19$m=47104,t=1,p=1$" + salt + "$" + key, ErrPHCFormat, false},
		{"old version", "$argon2id$v=16$m=47104,t=1,p=1$" + salt + "$" + key, ErrPHCFormat, false},
		{"missing version", "$argon2id$m=47104,t=1,p=1$" + salt + "$" + key, ErrPHCFormat, false},
		{"param order", "$argon2id$v=19$t=1,m=47104,p=1$" + salt + "$" + key, ErrPHCFormat, false},
		{"param count", "$argon2id$v=19$m=47104,t=1$" + salt + "$" + key, ErrPHCFormat, false},
		{"zero time", validPHC(47104, 0, 1), ErrPHCFormat, false},
		{"non numeric", "$argon2id$v=19$m=x,t=1,p=1$" + salt + "$" + key, ErrPHCFormat, false},
		{"memory below 8p", validPHC(8, 1, 2), ErrPHCFormat, false},
		{"threads overflow", validPHC(47104, 1, 256), ErrPHCFormat, false},
		{"short salt", "$argon2id$v=19$m=47104,t=1,p=1$YWJj$" + key, ErrPHCFormat, false},
		{"bad salt b64", "$argon2id$v=19$m=47104,t=1,p=1$!!!$" + key, ErrPHCFormat, false},
		{"short key", "$argon2id$v=19$m=47104,t=1,p=1$" + salt + "$YWJj", ErrPHCFormat, false},
		{"padded key", "$argon2id$v=19$m=47104,t=1,p=1$" + salt + "$" + key + "==", ErrPHCFormat, false},
		{"empty", "", ErrPHCFormat, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := ParseArgon2idPHC(tt.in)
			if tt.wantErr != nil {
				if err == nil || !errorsIs(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if p.BelowRecommended() != tt.weak {
				t.Errorf("BelowRecommended = %v, want %v", p.BelowRecommended(), tt.weak)
			}
		})
	}
}
