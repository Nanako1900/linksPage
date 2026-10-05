// Command linkspage is the LinksPage server and admin CLI.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	_ "time/tzdata" // embed the time zone database for distroless images
)

// Set via -ldflags "-X main.version=... -X main.commit=...".
var (
	version = "dev"
	commit  = "none"
)

const usage = `usage: linkspage <command> [args]

commands:
  serve               run the HTTP server (default)
  migrate up|status   apply or list database migrations
  config check        validate configuration and exit (no database)
  openapi             print the OpenAPI 3.1 document as JSON
  healthcheck         probe http://127.0.0.1:<port>/healthz (exit 0/1)
  version             print version information
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	// After the first signal restore default handling so that a second
	// SIGINT/SIGTERM terminates immediately during graceful shutdown.
	go func() {
		<-ctx.Done()
		stop()
	}()
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

// run dispatches a subcommand and returns the process exit code.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	cmd, rest := "serve", args
	if len(args) > 0 {
		cmd, rest = args[0], args[1:]
	}
	switch cmd {
	case "serve":
		return cmdServe(ctx, rest, stderr)
	case "migrate":
		return cmdMigrate(ctx, rest, stdout, stderr)
	case "config":
		return cmdConfig(rest, stdout, stderr)
	case "openapi":
		return cmdOpenAPI(rest, stdout, stderr)
	case "healthcheck":
		return cmdHealthcheck(ctx, rest, stderr)
	case "version", "--version", "-v":
		_, _ = fmt.Fprintf(stdout, "linkspage %s (commit %s)\n", version, commit)
		return 0
	case "help", "--help", "-h":
		_, _ = io.WriteString(stdout, usage)
		return 0
	default:
		_, _ = fmt.Fprintf(stderr, "unknown command %q\n\n%s", cmd, usage)
		return 2
	}
}
