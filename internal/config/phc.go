package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Argon2id parameter limits. Hashes above these limits are rejected so a
// single login cannot exhaust the container memory budget.
const (
	MaxArgon2Memory  = 65536 // KiB (64 MiB)
	MaxArgon2Time    = 5
	MaxArgon2Threads = 2

	minArgon2SaltLen = 8
	minArgon2KeyLen  = 16
	maxArgon2KeyLen  = 64
)

// Errors returned by ParseArgon2idPHC. Messages never include the hash.
var (
	ErrPHCFormat = errors.New("not a valid argon2id PHC string")
	ErrPHCLimits = errors.New("argon2id parameters exceed the allowed limits (m<=65536, t<=5, p<=2)")
)

// Argon2Params are the parameters encoded in an argon2id PHC string.
type Argon2Params struct {
	Memory  uint32 // KiB
	Time    uint32
	Threads uint8
	Salt    []byte
	Key     []byte
}

// recommendedCosts lists OWASP-equivalent (memory KiB, iterations) pairs.
var recommendedCosts = []struct{ m, t uint32 }{
	{47104, 1}, {19456, 2}, {12288, 3}, {9216, 4}, {7168, 5},
}

// BelowRecommended reports whether the parameters are weaker than every
// OWASP-recommended argon2id configuration.
func (p Argon2Params) BelowRecommended() bool {
	for _, rc := range recommendedCosts {
		if p.Memory >= rc.m && p.Time >= rc.t {
			return false
		}
	}
	return true
}

// ParseArgon2idPHC parses "$argon2id$v=19$m=..,t=..,p=..$salt$hash" and
// enforces the configured limits.
func ParseArgon2idPHC(s string) (Argon2Params, error) {
	parts := strings.Split(s, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != "v=19" {
		return Argon2Params{}, ErrPHCFormat
	}
	p, err := parseArgon2Costs(parts[3])
	if err != nil {
		return Argon2Params{}, err
	}
	salt, err := base64.RawStdEncoding.Strict().DecodeString(parts[4])
	if err != nil || len(salt) < minArgon2SaltLen {
		return Argon2Params{}, ErrPHCFormat
	}
	key, err := base64.RawStdEncoding.Strict().DecodeString(parts[5])
	if err != nil || len(key) < minArgon2KeyLen || len(key) > maxArgon2KeyLen {
		return Argon2Params{}, ErrPHCFormat
	}
	p.Salt, p.Key = salt, key
	if p.Memory > MaxArgon2Memory || p.Time > MaxArgon2Time || p.Threads > MaxArgon2Threads {
		return Argon2Params{}, ErrPHCLimits
	}
	return p, nil
}

func parseArgon2Costs(s string) (Argon2Params, error) {
	fields := strings.Split(s, ",")
	if len(fields) != 3 {
		return Argon2Params{}, ErrPHCFormat
	}
	var vals [3]uint32
	for i, prefix := range []string{"m=", "t=", "p="} {
		raw, ok := strings.CutPrefix(fields[i], prefix)
		if !ok {
			return Argon2Params{}, ErrPHCFormat
		}
		v, err := strconv.ParseUint(raw, 10, 32)
		if err != nil || v == 0 || v > math.MaxUint32 {
			return Argon2Params{}, ErrPHCFormat
		}
		vals[i] = uint32(v)
	}
	m, t, p := vals[0], vals[1], vals[2]
	if p > math.MaxUint8 || uint64(m) < 8*uint64(p) {
		return Argon2Params{}, fmt.Errorf("%w: memory must be at least 8*p KiB", ErrPHCFormat)
	}
	return Argon2Params{Memory: m, Time: t, Threads: uint8(p)}, nil
}
