package config

import (
	"fmt"
	"slices"
	"strings"
)

// Layer precedence (doc 4.4): defaults < config file < LP_* < LP_*_FILE.
const (
	rankUnset = iota
	rankFile
	rankEnv
	rankEnvFile
)

// layerSources records which layer set each key so that *_file
// companions can be ranked against the plain value they replace.
type layerSources struct {
	file    map[string]any // nested map from the YAML file
	env     map[string]any // flat keys from LP_* variables
	envFile map[string]any // flat keys from LP_*_FILE contents
}

func (l layerSources) rank(key string) int {
	if _, ok := l.envFile[key]; ok {
		return rankEnvFile
	}
	if _, ok := l.env[key]; ok {
		return rankEnv
	}
	if hasNestedKey(l.file, key) {
		return rankFile
	}
	return rankUnset
}

func hasNestedKey(m map[string]any, key string) bool {
	head, rest, nested := strings.Cut(key, ".")
	v, ok := m[head]
	if !ok || !nested {
		return ok
	}
	sub, isMap := v.(map[string]any)
	return isMap && hasNestedKey(sub, rest)
}

// dropShadowedFiles clears secret_key_file / db.password_file when the
// plain value was set by a higher-precedence layer (e.g. YAML
// db.password_file plus LP_DB__PASSWORD): the higher layer wins and the
// lower *_file is ignored with a warning. Within the same layer the *_file
// companion wins. The input is not modified.
func dropShadowedFiles(in *Config, src layerSources) (*Config, []string) {
	out := *in
	var warns []string
	shadowed := func(plain string) bool {
		if src.rank(plain) <= src.rank(plain+"_file") {
			return false
		}
		warns = append(warns, fmt.Sprintf("%s_file is ignored because %s is set by a higher-precedence source", plain, plain))
		return true
	}
	if out.SecretKeyFile != "" && shadowed("secret_key") {
		out.SecretKeyFile = ""
	}
	if out.DB.PasswordFile != "" && shadowed("db.password") {
		out.DB.PasswordFile = ""
	}
	return &out, warns
}

// resolveFiles reads every *_file companion into its secret field and
// returns a new Config; the input is not modified. Errors name key paths
// only.
func resolveFiles(in *Config, readFile func(string) ([]byte, error)) (*Config, []error) {
	out := *in
	var errs []error

	if out.SecretKeyFile != "" {
		v, err := readSecretFile(readFile, out.SecretKeyFile)
		if err != nil {
			errs = append(errs, fmt.Errorf("secret_key_file: %w", err))
		} else {
			out.SecretKey = Secret(v)
		}
	}

	if out.DB.PasswordFile != "" {
		v, err := readSecretFile(readFile, out.DB.PasswordFile)
		if err != nil {
			errs = append(errs, fmt.Errorf("db.password_file: %w", err))
		} else {
			out.DB.Password = Secret(v)
		}
	}

	users, userErrs := resolveLocalUsers(in.Auth.Local.Users, readFile)
	errs = append(errs, userErrs...)
	out.Auth.Local.Users = users

	providers, provErrs := resolveOAuthProviders(in.Auth.OAuth.Providers, readFile)
	errs = append(errs, provErrs...)
	out.Auth.OAuth.Providers = providers

	out.TrustedProxies = slices.Clone(in.TrustedProxies)
	out.Auth.OAuth.Admins = slices.Clone(in.Auth.OAuth.Admins)
	return &out, errs
}

func resolveLocalUsers(in []LocalUser, readFile func(string) ([]byte, error)) ([]LocalUser, []error) {
	out := make([]LocalUser, len(in))
	var errs []error
	for i, u := range in {
		path := fmt.Sprintf("auth.local.users[%d]", i)
		hasHash, hasFile := u.PasswordHash.IsSet(), u.PasswordHashFile != ""
		if hasHash == hasFile {
			errs = append(errs, fmt.Errorf("%s: set exactly one of password_hash or password_hash_file", path))
		}
		if hasFile && !hasHash {
			v, err := readSecretFile(readFile, u.PasswordHashFile)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s.password_hash_file: %w", path, err))
			} else {
				u.PasswordHash = Secret(v)
			}
		}
		out[i] = u
	}
	return out, errs
}

func resolveOAuthProviders(in []OAuthProvider, readFile func(string) ([]byte, error)) ([]OAuthProvider, []error) {
	out := make([]OAuthProvider, len(in))
	var errs []error
	for i, p := range in {
		path := fmt.Sprintf("auth.oauth.providers[%d]", i)
		hasSecret, hasFile := p.ClientSecret.IsSet(), p.ClientSecretFile != ""
		if hasSecret == hasFile {
			errs = append(errs, fmt.Errorf("%s: set exactly one of client_secret or client_secret_file", path))
		}
		if hasFile && !hasSecret {
			v, err := readSecretFile(readFile, p.ClientSecretFile)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s.client_secret_file: %w", path, err))
			} else {
				p.ClientSecret = Secret(v)
			}
		}
		p.Scopes = slices.Clone(p.Scopes)
		out[i] = p
	}
	return out, errs
}
