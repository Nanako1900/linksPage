package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Nanako1900/linksPage/internal/netx"
)

// MinSecretKeyBytes is the minimum accepted secret_key length.
const MinSecretKeyBytes = 32

var headerTokenRe = regexp.MustCompile("^[A-Za-z0-9!#$%&'*+.^_`|~-]+$")

var validSSLModes = map[string]bool{
	"disable": true, "allow": true, "prefer": true,
	"require": true, "verify-ca": true, "verify-full": true,
}

// Validate checks the configuration and returns warnings plus every
// error joined together. Error messages name key paths, never values.
func (c *Config) Validate() ([]string, error) {
	var errs []error
	var warns []string

	errs = append(errs, validateBaseURL(c.BaseURL))
	errs = append(errs, validateAddr(c.Server.Addr))
	dbWarns, dbErrs := validateDB(c.DB)
	warns = append(warns, dbWarns...)
	errs = append(errs, dbErrs...)
	if strings.TrimSpace(c.DataDir) == "" {
		errs = append(errs, errors.New("data_dir: must not be empty"))
	}
	if c.SecretKey.IsSet() && len(c.SecretKey.Reveal()) < MinSecretKeyBytes {
		errs = append(errs, fmt.Errorf("secret_key: must be at least %d bytes", MinSecretKeyBytes))
	}
	if _, err := netx.ParseTrustedProxies(c.TrustedProxies); err != nil {
		errs = append(errs, fmt.Errorf("trusted_proxies: %w", err))
	}
	if !headerTokenRe.MatchString(c.ClientIPHeader) {
		errs = append(errs, errors.New("client_ip_header: must be a valid HTTP header name"))
	}
	errs = append(errs, validateProxyURL(c.Providers.HTTPProxy.Reveal()))
	if c.HSTS && !strings.HasPrefix(c.BaseURL, "https://") {
		warns = append(warns, "hsts is enabled but base_url is not https; Strict-Transport-Security will not be sent")
	}
	if _, err := time.LoadLocation(c.Analytics.Timezone); err != nil || c.Analytics.Timezone == "" {
		errs = append(errs, errors.New("analytics.timezone: unknown time zone"))
	}
	errs = append(errs, validateLog(c.Log)...)

	authWarns, authErrs := validateAuth(c.Auth)
	warns = append(warns, authWarns...)
	errs = append(errs, authErrs...)
	return warns, errors.Join(errs...)
}

func validateBaseURL(raw string) error {
	if raw == "" {
		return errors.New("base_url: required (e.g. https://links.example.com)")
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Opaque != "" {
		return errors.New("base_url: must be an absolute http(s) URL")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.ForceQuery {
		return errors.New("base_url: must not contain credentials, query or fragment")
	}
	if u.Path == "/" {
		return errors.New("base_url: must not end with a slash")
	}
	if u.Path != "" {
		return errors.New("base_url: must be an origin without a path")
	}
	return nil
}

func validateAddr(addr string) error {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return errors.New("server.addr: must be host:port (e.g. :8080)")
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
		return errors.New("server.addr: invalid port")
	}
	return nil
}

func validateDB(db DB) ([]string, []error) {
	var warns []string
	var errs []error
	if db.Port < 1 || db.Port > 65535 {
		errs = append(errs, errors.New("db.port: must be between 1 and 65535"))
	}
	if !validSSLModes[db.SSLMode] {
		errs = append(errs, errors.New("db.sslmode: must be one of disable, allow, prefer, require, verify-ca, verify-full"))
	}
	for key, v := range map[string]string{"db.host": db.Host, "db.user": db.User, "db.name": db.Name} {
		if v == "" {
			warns = append(warns, key+" is not set; the server cannot connect to PostgreSQL")
		}
	}
	slices.Sort(warns)
	return warns, errs
}

func validateProxyURL(raw string) error {
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return errors.New("providers.http_proxy: invalid proxy URL")
	}
	switch u.Scheme {
	case "http", "https", "socks5", "socks5h":
		return nil
	default:
		return errors.New("providers.http_proxy: scheme must be http, https, socks5 or socks5h")
	}
}

func validateLog(l Log) []error {
	var errs []error
	switch l.Level {
	case "debug", "info", "warn", "error":
	default:
		errs = append(errs, errors.New("log.level: must be one of debug, info, warn, error"))
	}
	switch l.Format {
	case "json", "text":
	default:
		errs = append(errs, errors.New("log.format: must be json or text"))
	}
	return errs
}
