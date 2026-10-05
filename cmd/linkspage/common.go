package main

import (
	"fmt"
	"io"
	"log/slog"

	"github.com/Nanako1900/linksPage/internal/config"
)

// newLogger builds the process logger from config.
func newLogger(w io.Writer, l config.Log) *slog.Logger {
	var level slog.Level
	if err := level.UnmarshalText([]byte(l.Level)); err != nil {
		level = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: level}
	if l.Format == "text" {
		return slog.New(slog.NewTextHandler(w, opts))
	}
	return slog.New(slog.NewJSONHandler(w, opts))
}

// loadConfig loads configuration, printing errors to stderr.
func loadConfig(stderr io.Writer) (*config.Loaded, bool) {
	loaded, err := config.Load(config.Options{})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "configuration error:\n%v\n", err)
		return nil, false
	}
	return loaded, true
}

func logWarnings(logger *slog.Logger, warnings []string) {
	for _, w := range warnings {
		logger.Warn(w)
	}
}

func noArgs(name string, args []string, stderr io.Writer) bool {
	if len(args) > 0 {
		_, _ = fmt.Fprintf(stderr, "%s: unexpected arguments %v\n", name, args)
		return false
	}
	return true
}
