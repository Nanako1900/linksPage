package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/Nanako1900/linksPage/internal/store"
)

// migrateTimeout bounds a manual migration run.
const migrateTimeout = 10 * time.Minute

func cmdMigrate(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 || (args[0] != "up" && args[0] != "status") {
		_, _ = fmt.Fprintln(stderr, "usage: linkspage migrate up|status")
		return 2
	}
	loaded, ok := loadConfig(stderr)
	if !ok {
		return 1
	}
	logger := newLogger(stderr, loaded.Config.Log)
	logWarnings(logger, loaded.Warnings)

	ctx, cancel := context.WithTimeout(ctx, migrateTimeout)
	defer cancel()
	pool, err := store.Connect(ctx, loaded.Config.DB)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "migrate: %v\n", err)
		return 1
	}
	defer pool.Close()
	if err := store.CheckServerVersion(ctx, pool); err != nil {
		_, _ = fmt.Fprintf(stderr, "migrate: %v\n", err)
		return 1
	}
	m, err := store.NewMigrator(pool, logger)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "migrate: %v\n", err)
		return 1
	}
	defer func() { _ = m.Close() }()

	if args[0] == "up" {
		applied, err := m.Up(ctx)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "migrate up: %v\n", err)
			return 1
		}
		_, _ = fmt.Fprintf(stdout, "applied %d migration(s)\n", len(applied))
		return 0
	}
	status, err := m.Status(ctx)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "migrate status: %v\n", err)
		return 1
	}
	for _, s := range status {
		state := "pending"
		if s.Applied {
			state = "applied"
		}
		_, _ = fmt.Fprintf(stdout, "%05d  %-8s %s\n", s.Version, state, s.Name)
	}
	return 0
}
