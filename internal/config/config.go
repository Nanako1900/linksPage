// Package config loads and validates LinksPage configuration.
//
// Sources are applied from lowest to highest precedence:
// built-in defaults, the YAML file (LP_CONFIG_FILE, default
// /etc/linkspage/config.yaml), LP_* environment variables ("__" separates
// nesting levels) and LP_*_FILE variables (file contents, trailing
// whitespace trimmed).
package config

import "time"

// Environment variable names that control loading itself.
const (
	EnvPrefix         = "LP_"
	EnvConfigFile     = "LP_CONFIG_FILE"
	DefaultConfigFile = "/etc/linkspage/config.yaml"
)

// M1 defaults.
const (
	DefaultDiscordAPIBase = "https://discord.com"
	DefaultKOOKAPIBase    = "https://www.kookapp.cn"
	DefaultUploadMaxBytes = 5 << 20
	MinUploadMaxBytes     = 64 << 10
	MaxUploadMaxBytes     = 20 << 20
	MinProxyAuthBytes     = 32
)

// Role is an administrator role.
type Role string

// Supported administrator roles.
const (
	RoleOwner  Role = "owner"
	RoleEditor Role = "editor"
)

// Config is the fully loaded, typed configuration.
type Config struct {
	BaseURL        string    `koanf:"base_url" json:"base_url"`
	Server         Server    `koanf:"server" json:"server"`
	DB             DB        `koanf:"db" json:"db"`
	DataDir        string    `koanf:"data_dir" json:"data_dir"`
	SecretKey      Secret    `koanf:"secret_key" json:"secret_key"`
	SecretKeyFile  string    `koanf:"secret_key_file" json:"secret_key_file,omitempty"`
	TrustedProxies []string  `koanf:"trusted_proxies" json:"trusted_proxies"`
	ClientIPHeader string    `koanf:"client_ip_header" json:"client_ip_header"`
	Providers      Providers `koanf:"providers" json:"providers"`
	Analytics      Analytics `koanf:"analytics" json:"analytics"`
	Log            Log       `koanf:"log" json:"log"`
	// SeedFile is a seed.yaml imported on first start when the content
	// tables are empty ("" disables seeding). Image paths inside it are
	// relative to its directory.
	SeedFile string  `koanf:"seed_file" json:"seed_file,omitempty"`
	Uploads  Uploads `koanf:"uploads" json:"uploads"`
	Edge     Edge    `koanf:"edge" json:"edge"`
	// HSTS makes the application send Strict-Transport-Security itself.
	// Only for https deployments that do not sit behind Cloudflare (which
	// sets HSTS at the edge).
	HSTS bool `koanf:"hsts" json:"hsts"`
	// Auth is decoded strictly in a separate pass (unknown keys are errors).
	Auth Auth `koanf:"-" json:"auth"`
}

// Server holds HTTP listener settings.
type Server struct {
	Addr string `koanf:"addr" json:"addr"`
}

// DB holds PostgreSQL connection settings.
type DB struct {
	Host         string `koanf:"host" json:"host"`
	Port         int    `koanf:"port" json:"port"`
	User         string `koanf:"user" json:"user"`
	Name         string `koanf:"name" json:"name"`
	Password     Secret `koanf:"password" json:"password"`
	PasswordFile string `koanf:"password_file" json:"password_file,omitempty"`
	SSLMode      string `koanf:"sslmode" json:"sslmode"`
	AutoMigrate  bool   `koanf:"auto_migrate" json:"auto_migrate"`
}

// Providers holds outbound settings for community providers.
type Providers struct {
	HTTPProxy ProxyURL    `koanf:"http_proxy" json:"http_proxy,omitempty"`
	Discord   ProviderAPI `koanf:"discord" json:"discord"`
	KOOK      ProviderAPI `koanf:"kook" json:"kook"`
}

// ProviderAPI overrides a provider's upstream API origin. Only meant for
// tests and mirrors; set via the config file or LP_PROVIDERS__*__API_BASE.
type ProviderAPI struct {
	APIBase string `koanf:"api_base" json:"api_base"`
}

// Uploads holds image upload limits.
type Uploads struct {
	MaxBytes int64 `koanf:"max_bytes" json:"max_bytes"`
}

// Edge holds topology C (Cloudflare Worker front end) settings.
type Edge struct {
	// ProxyAuth is the shared secret the Worker sends as X-LP-Proxy-Auth
	// (doc 12.3). Optional; when set, /api/v1/public/render requires it.
	ProxyAuth     Secret `koanf:"proxy_auth" json:"proxy_auth"`
	ProxyAuthFile string `koanf:"proxy_auth_file" json:"proxy_auth_file,omitempty"`
}

// Analytics holds analytics settings.
type Analytics struct {
	Timezone string `koanf:"timezone" json:"timezone"`
}

// Log holds logging settings.
type Log struct {
	Level  string `koanf:"level" json:"level"`
	Format string `koanf:"format" json:"format"`
}

// Auth holds administrator authentication settings. Administrators and
// their roles are defined only here; the database never grants access.
type Auth struct {
	Session Session `koanf:"session" json:"session"`
	Local   Local   `koanf:"local" json:"local"`
	OAuth   OAuth   `koanf:"oauth" json:"oauth"`
}

// Session holds admin session timeouts.
type Session struct {
	IdleTimeout     time.Duration `koanf:"idle_timeout" json:"idle_timeout"`
	AbsoluteTimeout time.Duration `koanf:"absolute_timeout" json:"absolute_timeout"`
	AttrMaxAge      time.Duration `koanf:"attr_max_age" json:"attr_max_age"`
	ReauthWindow    time.Duration `koanf:"reauth_window" json:"reauth_window"`
}

// Local holds config-defined local accounts.
type Local struct {
	Enabled bool        `koanf:"enabled" json:"enabled"`
	Users   []LocalUser `koanf:"users" json:"users"`
}

// LocalUser is a local administrator account. Only argon2id PHC hashes
// are accepted.
type LocalUser struct {
	Username         string `koanf:"username" json:"username"`
	PasswordHash     Secret `koanf:"password_hash" json:"password_hash"`
	PasswordHashFile string `koanf:"password_hash_file" json:"password_hash_file,omitempty"`
	Role             Role   `koanf:"role" json:"role"`
}

// OAuth holds OAuth/OIDC providers and the admin allow-list.
type OAuth struct {
	Providers []OAuthProvider `koanf:"providers" json:"providers"`
	Admins    []OAuthAdmin    `koanf:"admins" json:"admins"`
}

// OAuthProvider configures one OAuth2/OIDC identity provider.
type OAuthProvider struct {
	ID               string   `koanf:"id" json:"id"`
	Type             string   `koanf:"type" json:"type"`
	DisplayName      string   `koanf:"display_name" json:"display_name,omitempty"`
	Issuer           string   `koanf:"issuer" json:"issuer,omitempty"`
	ClientID         string   `koanf:"client_id" json:"client_id"`
	ClientSecret     Secret   `koanf:"client_secret" json:"client_secret"`
	ClientSecretFile string   `koanf:"client_secret_file" json:"client_secret_file,omitempty"`
	Scopes           []string `koanf:"scopes" json:"scopes,omitempty"`
	PKCE             *bool    `koanf:"pkce" json:"pkce,omitempty"`
}

// OAuthAdmin grants a role to identities from one provider. Exactly one of
// Subject, Email or Group must be set.
type OAuthAdmin struct {
	Provider string `koanf:"provider" json:"provider"`
	Subject  string `koanf:"subject" json:"subject,omitempty"`
	Email    string `koanf:"email" json:"email,omitempty"`
	Group    string `koanf:"group" json:"group,omitempty"`
	Role     Role   `koanf:"role" json:"role"`
}

// defaults returns the built-in defaults as a flat key map.
func defaults() map[string]any {
	return map[string]any{
		"server.addr":                   ":8080",
		"db.port":                       5432,
		"db.sslmode":                    "disable",
		"db.auto_migrate":               true,
		"data_dir":                      "/data",
		"trusted_proxies":               []any{"172.31.255.2/32"},
		"client_ip_header":              "CF-Connecting-IP",
		"analytics.timezone":            "Asia/Shanghai",
		"log.level":                     "info",
		"log.format":                    "json",
		"hsts":                          false,
		"providers.discord.api_base":    DefaultDiscordAPIBase,
		"providers.kook.api_base":       DefaultKOOKAPIBase,
		"uploads.max_bytes":             DefaultUploadMaxBytes,
		"auth.local.enabled":            true,
		"auth.session.idle_timeout":     "24h",
		"auth.session.absolute_timeout": "168h",
		"auth.session.attr_max_age":     "12h",
		"auth.session.reauth_window":    "15m",
	}
}

// AdminCount returns the number of configured administrator entries that
// can actually authenticate (local users only count when local login is
// enabled).
func (c *Config) AdminCount() int {
	n := len(c.Auth.OAuth.Admins)
	if c.Auth.Local.Enabled {
		n += len(c.Auth.Local.Users)
	}
	return n
}
