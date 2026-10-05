package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/Nanako1900/linksPage/internal/config"
	"github.com/Nanako1900/linksPage/internal/httpapi"
)

func cmdConfig(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "check" {
		_, _ = fmt.Fprintln(stderr, "usage: linkspage config check [-print]")
		return 2
	}
	fs := flag.NewFlagSet("config check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	printCfg := fs.Bool("print", false, "print the effective configuration (secrets redacted)")
	if err := fs.Parse(args[1:]); err != nil || !noArgs("config check", fs.Args(), stderr) {
		return 2
	}
	loaded, ok := loadConfig(stderr)
	if !ok {
		return 1
	}
	for _, w := range loaded.Warnings {
		_, _ = fmt.Fprintf(stderr, "WARN: %s\n", w)
	}
	source := "not found, using defaults and environment"
	if loaded.FileFound {
		source = "loaded"
	}
	_, _ = fmt.Fprintf(stdout, "configuration OK (config file %s: %s)\n", loaded.File, source)
	if *printCfg {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(loaded.Config); err != nil {
			_, _ = fmt.Fprintf(stderr, "print config: %v\n", err)
			return 1
		}
	}
	return 0
}

func cmdOpenAPI(args []string, stdout, stderr io.Writer) int {
	if !noArgs("openapi", args, stderr) {
		return 2
	}
	spec, err := httpapi.OpenAPISpec(version)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "openapi: %v\n", err)
		return 1
	}
	if _, err := stdout.Write(spec); err != nil {
		return 1
	}
	return 0
}

// healthcheckTimeout bounds the Docker HEALTHCHECK probe.
const healthcheckTimeout = 3 * time.Second

// cmdHealthcheck probes /healthz on the address the server listens on.
// It resolves only server.addr (default, config file, LP_SERVER__ADDR)
// rather than the full config, so it stays cheap and works even when
// secrets are unreadable to the probing process.
func cmdHealthcheck(ctx context.Context, args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("healthcheck", flag.ContinueOnError)
	fs.SetOutput(stderr)
	addr := fs.String("addr", config.ServerAddr(config.Options{}), "server listen address")
	if err := fs.Parse(args); err != nil || !noArgs("healthcheck", fs.Args(), stderr) {
		return 2
	}
	target, err := probeTarget(*addr)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "healthcheck: invalid address %q\n", *addr)
		return 1
	}
	if err := probe(ctx, "http://"+target+"/healthz"); err != nil {
		_, _ = fmt.Fprintf(stderr, "healthcheck: %v\n", err)
		return 1
	}
	return 0
}

func probe(ctx context.Context, url string) error {
	ctx, cancel := context.WithTimeout(ctx, healthcheckTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
	if resp.StatusCode != http.StatusOK {
		return errors.New("unhealthy: status " + resp.Status)
	}
	return nil
}

// probeTarget turns a listen address into a dialable host:port: wildcard
// hosts ("", 0.0.0.0, ::) are probed on loopback, specific hosts as-is.
func probeTarget(addr string) (string, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		return "", errors.New("invalid address")
	}
	if ip := net.ParseIP(host); host == "" || (ip != nil && ip.IsUnspecified()) {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port), nil
}
