package config

import (
	"crypto/hkdf"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

// validateM1 checks the keys added for milestone M1.
func validateM1(c *Config) []error {
	var errs []error
	errs = append(errs, validateAPIBase("providers.discord.api_base", c.Providers.Discord.APIBase))
	errs = append(errs, validateAPIBase("providers.kook.api_base", c.Providers.KOOK.APIBase))
	if c.Uploads.MaxBytes < MinUploadMaxBytes || c.Uploads.MaxBytes > MaxUploadMaxBytes {
		errs = append(errs, fmt.Errorf("uploads.max_bytes: must be between %d and %d", MinUploadMaxBytes, MaxUploadMaxBytes))
	}
	if c.SeedFile != "" {
		ext := strings.ToLower(filepath.Ext(c.SeedFile))
		if (ext != ".yaml" && ext != ".yml") || strings.ContainsRune(c.SeedFile, 0) {
			errs = append(errs, errors.New("seed_file: must be a .yaml or .yml file path"))
		}
	}
	if c.Edge.ProxyAuth.IsSet() && len(c.Edge.ProxyAuth.Reveal()) < MinProxyAuthBytes {
		errs = append(errs, fmt.Errorf("edge.proxy_auth: must be at least %d bytes", MinProxyAuthBytes))
	}
	return errs
}

// validateAPIBase accepts an absolute http(s) origin (optionally with a
// path prefix) without credentials, query, fragment or trailing slash.
func validateAPIBase(key, raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.Opaque != "" {
		return fmt.Errorf("%s: must be an absolute http(s) URL", key)
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.ForceQuery || strings.HasSuffix(raw, "/") {
		return fmt.Errorf("%s: must not contain credentials, query, fragment or a trailing slash", key)
	}
	return nil
}

// HKDF info strings for keys derived from secret_key (doc 4.13).
const (
	KeyInfoMediaProxy = "media-proxy"
	KeyInfoVisitor    = "visitor"
	KeyInfoDevice     = "device"
)

// DeriveKey derives a size-byte subkey from secret_key with
// HKDF-SHA256(info). EnsureSecretKey must have run first.
func (c *Config) DeriveKey(info string, size int) ([]byte, error) {
	if !c.SecretKey.IsSet() {
		return nil, errors.New("derive key: secret_key is not set")
	}
	if info == "" || size < 16 || size > 64 {
		return nil, errors.New("derive key: info must be set and size between 16 and 64")
	}
	k, err := hkdf.Key(sha256.New, []byte(c.SecretKey.Reveal()), nil, info, size)
	if err != nil {
		return nil, fmt.Errorf("derive key: %w", err)
	}
	return k, nil
}
