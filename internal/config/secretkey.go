package config

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// SecretKeyFileName is the auto-generated key file inside data_dir.
const SecretKeyFileName = ".secret_key"

// EnsureSecretKey returns a copy of c whose SecretKey is set. When neither
// secret_key nor secret_key_file is configured, the key is read from
// {data_dir}/.secret_key, generating 32 random bytes (hex) with mode 0600
// on first use. generated reports whether a new key file was written.
func (c *Config) EnsureSecretKey() (*Config, bool, error) {
	out := *c
	if c.SecretKey.IsSet() {
		return &out, false, nil
	}
	path := filepath.Join(c.DataDir, SecretKeyFileName)
	key, err := readKeyFile(path)
	if err == nil {
		out.SecretKey = key
		return &out, false, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, false, err
	}
	key, err = writeNewKeyFile(path)
	if errors.Is(err, fs.ErrExist) {
		// Another process created it concurrently; use theirs.
		key, err = readKeyFile(path)
		if err != nil {
			return nil, false, err
		}
		out.SecretKey = key
		return &out, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	out.SecretKey = key
	return &out, true, nil
}

func readKeyFile(path string) (Secret, error) {
	if err := restrictKeyFileMode(path); err != nil {
		return "", err
	}
	b, err := os.ReadFile(path) //nolint:gosec // path is data_dir from trusted config
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	key := strings.TrimRight(string(b), " \t\r\n")
	if len(key) < MinSecretKeyBytes {
		return "", fmt.Errorf("%s: secret key must be at least %d bytes", path, MinSecretKeyBytes)
	}
	return Secret(key), nil
}

// restrictKeyFileMode makes sure the key file is not accessible by group
// or others (a restore or a manual copy can easily leave it 0644).
func restrictKeyFileMode(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if fi.Mode().Perm()&0o077 == 0 {
		return nil
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("%s is accessible by other users (mode %04o) and cannot be fixed (%w); run: chmod 600 %s",
			path, fi.Mode().Perm(), err, path)
	}
	return nil
}

// writeNewKeyFile generates a key and publishes it atomically: the key is
// written and synced to a temporary file which is then hard-linked to
// path. A crash can never leave a partial key behind, and a concurrent
// writer makes os.Link fail with fs.ErrExist.
func writeNewKeyFile(path string) (Secret, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate secret key: %w", err)
	}
	key := hex.EncodeToString(raw)
	tmp, err := os.CreateTemp(filepath.Dir(path), ".secret_key-*.tmp") // mode 0600
	if err != nil {
		return "", fmt.Errorf("create %s: %w", path, err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	_, werr := tmp.WriteString(key + "\n")
	if err := errors.Join(werr, tmp.Sync(), tmp.Close()); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}
	if err := os.Link(tmpName, path); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return "", fmt.Errorf("create %s: %w", path, err)
		}
		return "", fmt.Errorf("publish %s: %w", path, err)
	}
	return Secret(key), nil
}
