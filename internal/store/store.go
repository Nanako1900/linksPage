// Package store owns PostgreSQL access: the connection pool, embedded
// goose migrations and sqlc-generated queries (package dbq).
package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Nanako1900/linksPage/internal/config"
)

// MinServerVersionNum is the minimum PostgreSQL version (18, for native
// uuidv7()).
const MinServerVersionNum = 180000

// Pool defaults sized for a small single-instance deployment.
const (
	defaultMaxConns        = 10
	defaultMaxConnIdleTime = 5 * time.Minute
	connectTimeout         = 5 * time.Second
)

// ErrNotConfigured is returned when required connection settings are
// missing.
var ErrNotConfigured = errors.New("database connection is not configured")

// ErrUnsupportedServer is returned by CheckServerVersion when the server
// is older than MinServerVersionNum. It is permanent: retrying cannot help.
var ErrUnsupportedServer = errors.New("unsupported PostgreSQL version")

// PoolConfig builds a pgxpool configuration from cfg. It fails fast on
// missing or invalid settings and never logs or returns the password.
func PoolConfig(cfg config.DB) (*pgxpool.Config, error) {
	var missing []string
	for key, v := range map[string]string{"db.host": cfg.Host, "db.user": cfg.User, "db.name": cfg.Name} {
		if v == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		slices.Sort(missing)
		return nil, fmt.Errorf("%w: missing %s", ErrNotConfigured, strings.Join(missing, ", "))
	}
	dsn := strings.Join([]string{
		"host=" + quoteDSN(cfg.Host),
		"port=" + strconv.Itoa(cfg.Port),
		"user=" + quoteDSN(cfg.User),
		"dbname=" + quoteDSN(cfg.Name),
		"sslmode=" + quoteDSN(cfg.SSLMode),
		"application_name=linkspage",
		"connect_timeout=" + strconv.Itoa(int(connectTimeout.Seconds())),
	}, " ")
	pc, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse database config: %w", err)
	}
	// The password is set directly so it needs no DSN/URL escaping.
	pc.ConnConfig.Password = cfg.Password.Reveal()
	pc.MaxConns = defaultMaxConns
	pc.MaxConnIdleTime = defaultMaxConnIdleTime
	return pc, nil
}

// Connect creates the pool. It does not wait for the server to be
// reachable; use Ping for readiness.
func Connect(ctx context.Context, cfg config.DB) (*pgxpool.Pool, error) {
	pc, err := PoolConfig(cfg)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, pc)
	if err != nil {
		return nil, fmt.Errorf("create database pool: %w", err)
	}
	return pool, nil
}

// CheckServerVersion verifies the server is PostgreSQL 18 or newer.
func CheckServerVersion(ctx context.Context, pool *pgxpool.Pool) error {
	var num int
	if err := pool.QueryRow(ctx, "SELECT current_setting('server_version_num')::int").Scan(&num); err != nil {
		return fmt.Errorf("query server version: %w", err)
	}
	if num < MinServerVersionNum {
		return fmt.Errorf("%w: PostgreSQL %d.%d is not supported; version 18 or newer is required",
			ErrUnsupportedServer, num/10000, num%10000)
	}
	return nil
}

// quoteDSN quotes a libpq keyword/value DSN value.
func quoteDSN(v string) string {
	r := strings.NewReplacer(`\`, `\\`, `'`, `\'`)
	return "'" + r.Replace(v) + "'"
}
