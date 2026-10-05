package store

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Nanako1900/linksPage/internal/store/dbq"
)

// ErrIncompatibleApp is returned when this binary cannot run against the
// database schema. It is permanent: retrying cannot help.
var ErrIncompatibleApp = errors.New("application is incompatible with the database schema")

// CheckAppCompatibility verifies that appVersion is not older than the
// schema's min_compatible_app_version. Development builds (versions that
// are not semver, e.g. "dev") are always allowed.
func CheckAppCompatibility(ctx context.Context, q *dbq.Queries, appVersion string) error {
	minVersion, err := q.GetMinCompatibleAppVersion(ctx)
	if err != nil {
		return fmt.Errorf("read schema_meta: %w", err)
	}
	return compareCompat(appVersion, minVersion)
}

func compareCompat(appVersion, minVersion string) error {
	app, ok := parseSemver(appVersion)
	if !ok {
		return nil
	}
	minV, ok := parseSemver(minVersion)
	if !ok {
		return fmt.Errorf("%w: schema_meta.min_compatible_app_version %q is not a valid version", ErrIncompatibleApp, minVersion)
	}
	for i := range app {
		if app[i] != minV[i] {
			if app[i] < minV[i] {
				return fmt.Errorf("%w: this binary (%s) is older than the database schema supports (min %s); upgrade the image",
					ErrIncompatibleApp, appVersion, minVersion)
			}
			return nil
		}
	}
	return nil
}

// parseSemver parses "v1.2.3" / "1.2.3-rc.1" into [major, minor, patch].
func parseSemver(v string) ([3]int, bool) {
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	var out [3]int
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}
