package store

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pressly/goose/v3"

	"github.com/Nanako1900/linksPage/internal/config"
)

func TestPoolConfig(t *testing.T) {
	_, err := PoolConfig(config.DB{Port: 5432, SSLMode: "disable"})
	if !errors.Is(err, ErrNotConfigured) || !strings.Contains(err.Error(), "db.host, db.name, db.user") {
		t.Fatalf("err = %v", err)
	}

	pc, err := PoolConfig(config.DB{
		Host: "db", Port: 5433, User: "o'neil", Name: `we\ird`, SSLMode: "disable",
		Password: config.Secret("p@ss word'with$chars"),
	})
	if err != nil {
		t.Fatal(err)
	}
	cc := pc.ConnConfig
	if cc.Host != "db" || cc.Port != 5433 || cc.User != "o'neil" || cc.Database != `we\ird` {
		t.Errorf("unexpected conn config: %s %d %s %s", cc.Host, cc.Port, cc.User, cc.Database)
	}
	if cc.Password != "p@ss word'with$chars" {
		t.Error("password not applied verbatim")
	}
	if cc.RuntimeParams["application_name"] != "linkspage" || pc.MaxConns != defaultMaxConns {
		t.Error("pool defaults not applied")
	}

	if _, err := PoolConfig(config.DB{Host: "h", User: "u", Name: "n", Port: 5432, SSLMode: "bogus"}); err == nil {
		t.Error("invalid sslmode should fail")
	}
}

func TestCompareCompat(t *testing.T) {
	tests := []struct {
		app, minV string
		wantErr   bool
	}{
		{"dev", "9.9.9", false},
		{"v0.1.0", "0.0.0", false},
		{"0.2.0", "0.2.0", false},
		{"1.0.0-rc.1", "0.9.9", false},
		{"0.1.9", "0.2.0", true},
		{"v1.2.3", "1.2.4", true},
		{"1.2.3", "bogus", true},
		{"1.2", "0.0.0", false},
		{"1.x.3", "0.0.0", false},
	}
	for _, tt := range tests {
		if err := compareCompat(tt.app, tt.minV); (err != nil) != tt.wantErr {
			t.Errorf("compareCompat(%q, %q) = %v, wantErr %v", tt.app, tt.minV, err, tt.wantErr)
		}
	}
}

func TestMigrationsEmbedded(t *testing.T) {
	b, err := MigrationsFSReadFile("00001_init.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"-- +goose Up", "CREATE TABLE pages", "CREATE TABLE site_settings", "CREATE TABLE schema_meta", "-- +goose Down"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("migration missing %q", want)
		}
	}
}

func TestClassifyMigrationError(t *testing.T) {
	transient := errors.New("connection reset by peer")
	tests := []struct {
		name      string
		err       error
		permanent bool
	}{
		{"nil", nil, false},
		{"transient", transient, false},
		{"syntax", &pgconn.PgError{Code: "42601"}, true},
		{"privilege", &goose.PartialError{Failed: &goose.MigrationResult{}, Err: &pgconn.PgError{Code: "42501"}}, true},
		{"unsupported feature", fmt.Errorf("wrap: %w", &pgconn.PgError{Code: "0A000"}), true},
		{"unique violation", &pgconn.PgError{Code: "23505"}, true},
		{"server restarting", &pgconn.PgError{Code: "57P03"}, false},
		{"deadlock", &pgconn.PgError{Code: "40P01"}, false},
		{"no migrations", goose.ErrNoMigrations, true},
	}
	for _, tt := range tests {
		got := classifyMigrationError(tt.err)
		if errors.Is(got, ErrMigrationRejected) != tt.permanent {
			t.Errorf("%s: permanent = %v, want %v", tt.name, !tt.permanent, tt.permanent)
		}
		if tt.err != nil && !errors.Is(got, tt.err) {
			t.Errorf("%s: classified error must wrap the cause", tt.name)
		}
	}
}

func TestSentinelErrors(t *testing.T) {
	if err := compareCompat("0.1.0", "0.2.0"); !errors.Is(err, ErrIncompatibleApp) {
		t.Errorf("older app: %v", err)
	}
	if err := compareCompat("0.1.0", "bogus"); !errors.Is(err, ErrIncompatibleApp) {
		t.Errorf("bad schema meta: %v", err)
	}
}
