// Package providertest holds helpers for loading recorded upstream fixtures
// used by provider tests.
package providertest

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/textproto"
	"os"
	"strconv"
	"strings"
)

// Recorded is a recorded HTTP response status line and header block.
type Recorded struct {
	Proto  string
	Status int
	Header http.Header
}

// ErrBadStatusLine is returned when the first line is not "HTTP/x NNN ...".
var ErrBadStatusLine = errors.New("providertest: malformed status line")

// ParseHeaders parses a curl -D style header dump: a status line followed by
// "Key: value" lines and an optional blank terminator line.
func ParseHeaders(r io.Reader) (Recorded, error) {
	br := bufio.NewReader(r)
	line, err := br.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return Recorded{}, fmt.Errorf("providertest: read status line: %w", err)
	}
	proto, status, err := parseStatusLine(strings.TrimRight(line, "\r\n"))
	if err != nil {
		return Recorded{}, err
	}
	mime, err := textproto.NewReader(br).ReadMIMEHeader()
	if err != nil && !errors.Is(err, io.EOF) {
		return Recorded{}, fmt.Errorf("providertest: read headers: %w", err)
	}
	return Recorded{Proto: proto, Status: status, Header: http.Header(mime)}, nil
}

// LoadHeaders reads and parses a header dump file.
func LoadHeaders(path string) (Recorded, error) {
	data, err := os.ReadFile(path) //nolint:gosec // test helper: path is a fixture under testdata chosen by the test
	if err != nil {
		return Recorded{}, fmt.Errorf("providertest: %w", err)
	}
	return ParseHeaders(bytes.NewReader(data))
}

func parseStatusLine(line string) (string, int, error) {
	fields := strings.Fields(line)
	if len(fields) < 2 || !strings.HasPrefix(fields[0], "HTTP/") {
		return "", 0, ErrBadStatusLine
	}
	code, err := strconv.Atoi(fields[1])
	if err != nil || code < 100 || code > 599 {
		return "", 0, ErrBadStatusLine
	}
	return fields[0], code, nil
}
