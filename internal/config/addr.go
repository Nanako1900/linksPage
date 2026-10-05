package config

import (
	"github.com/knadh/koanf/parsers/yaml"
)

// ServerAddr resolves server.addr with the normal precedence (default <
// config file < LP_SERVER__ADDR) without validating anything or reading
// secrets. `linkspage healthcheck` uses it so that it probes the port the
// server really listens on while staying cheap; unreadable or invalid
// files fall back to the lower layers (the server itself reports those).
func ServerAddr(opts Options) string {
	opts = opts.withDefaults()
	addr, _ := defaults()["server.addr"].(string)
	path := lookupEnv(opts.Environ, EnvConfigFile)
	if path == "" {
		path = DefaultConfigFile
	}
	if b, err := opts.ReadFile(path); err == nil {
		if m, err := yaml.Parser().Unmarshal(b); err == nil {
			if srv, ok := m["server"].(map[string]any); ok {
				if a, ok := srv["addr"].(string); ok && a != "" {
					addr = a
				}
			}
		}
	}
	if v := lookupEnv(opts.Environ, EnvPrefix+"SERVER__ADDR"); v != "" {
		addr = v
	}
	return addr
}
