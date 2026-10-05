package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strings"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/v2"
)

// permissionHint is shown when the config file exists but cannot be read.
const permissionHint = "check permissions: chgrp 65532 config config/config.yaml secrets/admin_* && chmod 750 config && chmod 640 config/config.yaml secrets/admin_*"

// Options controls where configuration is read from. Zero values use the
// real process environment and file system.
type Options struct {
	Environ  []string
	ReadFile func(string) ([]byte, error)
	Stat     func(string) (fs.FileInfo, error)
}

// Loaded is the result of a successful Load.
type Loaded struct {
	Config *Config
	// Warnings are non-fatal findings that callers should log.
	Warnings []string
	// File is the config file path that was consulted.
	File string
	// FileFound reports whether the config file existed.
	FileFound bool
}

func (o Options) withDefaults() Options {
	if o.Environ == nil {
		o.Environ = os.Environ()
	}
	if o.ReadFile == nil {
		o.ReadFile = os.ReadFile
	}
	if o.Stat == nil {
		o.Stat = os.Stat
	}
	return o
}

// Load reads, decodes, resolves and validates the configuration. All
// validation errors are returned together via errors.Join.
func Load(opts Options) (*Loaded, error) {
	opts = opts.withDefaults()
	path := lookupEnv(opts.Environ, EnvConfigFile)
	if path == "" {
		path = DefaultConfigFile
	}
	out := &Loaded{File: path}

	k := koanf.New(".")
	if err := k.Load(confmap.Provider(defaults(), "."), nil); err != nil {
		return nil, fmt.Errorf("load defaults: %w", err)
	}

	fileMap, found, warns, err := readConfigFile(opts, path)
	if err != nil {
		return nil, err
	}
	out.FileFound = found
	out.Warnings = append(out.Warnings, warns...)
	var errs []error
	errs = append(errs, checkHashValues(fileMap, "")...)
	if err := k.Load(confmap.Provider(fileMap, ""), nil); err != nil {
		return nil, fmt.Errorf("merge config file: %w", err)
	}

	env := parseEnv(opts.Environ, knownKeys(), opts.ReadFile)
	out.Warnings = append(out.Warnings, env.warnings...)
	errs = append(errs, env.errs...)
	if err := k.Load(confmap.Provider(env.values, "."), nil); err != nil {
		return nil, fmt.Errorf("merge environment: %w", err)
	}
	if err := k.Load(confmap.Provider(env.fileValues, "."), nil); err != nil {
		return nil, fmt.Errorf("merge *_FILE environment: %w", err)
	}

	cfg, unknown, err := decodeAll(k.Raw())
	if err != nil {
		return nil, errors.Join(append(errs, err)...)
	}
	for _, key := range unknown {
		out.Warnings = append(out.Warnings, fmt.Sprintf("unknown config key %s is ignored", key))
	}

	cfg, shadowWarns := dropShadowedFiles(cfg, layerSources{file: fileMap, env: env.values, envFile: env.fileValues})
	out.Warnings = append(out.Warnings, shadowWarns...)
	resolved, resolveErrs := resolveFiles(cfg, opts.ReadFile)
	errs = append(errs, resolveErrs...)
	warns, err = resolved.Validate()
	out.Warnings = append(out.Warnings, warns...)
	if err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	out.Config = resolved
	return out, nil
}

func lookupEnv(environ []string, name string) string {
	for _, kv := range slices.Backward(environ) {
		if k, v, ok := strings.Cut(kv, "="); ok && k == name {
			return v
		}
	}
	return ""
}

// readConfigFile reads and parses the YAML config file. Only a missing
// file (ENOENT) is treated as empty; every other failure is fatal.
func readConfigFile(opts Options, path string) (map[string]any, bool, []string, error) {
	b, err := opts.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]any{}, false, nil, nil
	}
	if err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return nil, false, nil, fmt.Errorf("read config file %s: %w; %s", path, err, permissionHint)
		}
		return nil, false, nil, fmt.Errorf("read config file %s: %w", path, err)
	}
	m, err := yaml.Parser().Unmarshal(b)
	if err != nil {
		return nil, true, nil, fmt.Errorf("parse config file %s: %w", path, err)
	}
	if m == nil {
		m = map[string]any{}
	}
	var warns []string
	if hasInlinePasswordHash(m) {
		if fi, err := opts.Stat(path); err == nil && fi.Mode().Perm()&0o004 != 0 {
			warns = append(warns, fmt.Sprintf("config file %s contains password hashes and is world-readable; run: chmod 640 %s", path, path))
		}
	}
	return m, true, warns, nil
}

// hasInlinePasswordHash reports whether any auth.local.users entry sets
// password_hash directly in the file (comments do not count).
func hasInlinePasswordHash(m map[string]any) bool {
	auth, _ := m["auth"].(map[string]any)
	local, _ := auth["local"].(map[string]any)
	users, _ := local["users"].([]any)
	for _, u := range users {
		user, _ := u.(map[string]any)
		if h, _ := user["password_hash"].(string); h != "" {
			return true
		}
	}
	return false
}

// checkHashValues rejects string values that start with '#', which
// usually means an inline comment was pasted into a value.
func checkHashValues(v any, path string) []error {
	switch t := v.(type) {
	case string:
		if strings.HasPrefix(t, "#") {
			return []error{hashValueError(path)}
		}
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		var errs []error
		for _, k := range keys {
			errs = append(errs, checkHashValues(t[k], joinPath(path, k))...)
		}
		return errs
	case []any:
		var errs []error
		for i, e := range t {
			errs = append(errs, checkHashValues(e, fmt.Sprintf("%s[%d]", path, i))...)
		}
		return errs
	}
	return nil
}

func joinPath(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}

// decodeAll decodes the merged raw map. auth.* is decoded strictly: any
// unknown key there is an error. Unknown keys elsewhere are returned so
// they can be reported as warnings.
func decodeAll(raw map[string]any) (*Config, []string, error) {
	cfg := &Config{}
	unused, err := decode(raw, cfg, "")
	if err != nil {
		return nil, nil, err
	}
	unknown := make([]string, 0, len(unused))
	for _, k := range unused {
		if k != "auth" && !strings.HasPrefix(k, "auth.") {
			unknown = append(unknown, k)
		}
	}
	authRaw, ok := raw["auth"]
	if !ok {
		return cfg, unknown, nil
	}
	if _, isMap := authRaw.(map[string]any); !isMap {
		return nil, nil, errors.New("auth: must be a mapping")
	}
	var auth Auth
	authUnused, err := decode(authRaw, &auth, "auth.")
	if err != nil {
		return nil, nil, err
	}
	if len(authUnused) > 0 {
		return nil, nil, unknownAuthKeysError(authUnused)
	}
	cfg.Auth = auth
	return cfg, unknown, nil
}

func unknownAuthKeysError(keys []string) error {
	errs := make([]error, 0, len(keys))
	for _, k := range keys {
		if strings.HasPrefix(k, "auth.local.users[") && strings.HasSuffix(k, "].password") {
			errs = append(errs, fmt.Errorf("%s: plaintext passwords are not accepted; set password_hash instead: %s", k, hashPasswordHint))
			continue
		}
		errs = append(errs, fmt.Errorf("%s: unknown key", k))
	}
	return errors.Join(errs...)
}
