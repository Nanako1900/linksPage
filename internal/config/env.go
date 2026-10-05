package config

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
)

// envLayers holds the values derived from the process environment.
type envLayers struct {
	// values are LP_* variables mapped to known keys (flat, "." delimited).
	values map[string]any
	// fileValues are LP_*_FILE variables whose contents were read into the
	// key without the _FILE suffix. They take precedence over values.
	fileValues map[string]any
	warnings   []string
	errs       []error
}

// envKey converts an environment variable name to a config key:
// LP_DB__AUTO_MIGRATE -> db.auto_migrate.
func envKey(name string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimPrefix(name, EnvPrefix)), "__", ".")
}

// parseEnv maps LP_* variables to config keys. Empty values are treated
// as unset so that compose files can pass optional variables through.
func parseEnv(environ []string, known map[string]bool, readFile func(string) ([]byte, error)) envLayers {
	out := envLayers{values: map[string]any{}, fileValues: map[string]any{}}
	sorted := slices.Clone(environ)
	slices.Sort(sorted)
	for _, kv := range sorted {
		name, value, ok := strings.Cut(kv, "=")
		if !ok || !strings.HasPrefix(name, EnvPrefix) || name == EnvConfigFile || value == "" {
			continue
		}
		key := envKey(name)
		switch {
		case known[key]:
			if strings.HasPrefix(value, "#") {
				out.errs = append(out.errs, hashValueError(name))
				continue
			}
			out.values[key] = value
		case strings.HasSuffix(key, "_file") && known[strings.TrimSuffix(key, "_file")]:
			content, err := readSecretFile(readFile, value)
			if err != nil {
				out.errs = append(out.errs, fmt.Errorf("%s: %w", name, err))
				continue
			}
			out.fileValues[strings.TrimSuffix(key, "_file")] = content
		default:
			out.warnings = append(out.warnings, fmt.Sprintf("unknown environment variable %s is ignored", name))
		}
	}
	return out
}

func hashValueError(where string) error {
	return fmt.Errorf("%s: value starts with '#'; inline comments are not supported in .env files, put comments on their own line", where)
}

// readSecretFile reads a *_FILE target and trims trailing whitespace.
// Errors never include the file contents.
func readSecretFile(readFile func(string) ([]byte, error), path string) (string, error) {
	b, err := readFile(path)
	if err != nil {
		return "", fmt.Errorf("cannot read file: %w", err)
	}
	return strings.TrimRight(string(b), " \t\r\n"), nil
}

// knownKeys returns every leaf key path of Config (including auth.*).
// Slice fields are leaves: they cannot be addressed element-wise.
func knownKeys() map[string]bool {
	keys := map[string]bool{}
	collectKeys(reflect.TypeFor[Config](), "", keys)
	return keys
}

func collectKeys(t reflect.Type, prefix string, keys map[string]bool) {
	for i := range t.NumField() {
		f := t.Field(i)
		tag := f.Tag.Get("koanf")
		if tag == "-" {
			tag = strings.Split(f.Tag.Get("json"), ",")[0]
		}
		if tag == "" {
			continue
		}
		path := prefix + tag
		ft := f.Type
		if ft.Kind() == reflect.Struct && ft.String() != "time.Duration" {
			collectKeys(ft, path+".", keys)
			continue
		}
		keys[path] = true
	}
}
