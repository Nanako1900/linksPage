package media

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
)

// UploadsDir is the directory under data_dir holding stored files.
const UploadsDir = "uploads"

// tmpPrefix marks in-flight writes; leftovers are removed on startup.
const tmpPrefix = ".tmp-"

// File and directory modes for stored media.
const (
	dirMode  fs.FileMode = 0o750
	fileMode fs.FileMode = 0o640
)

// Store persists content-addressed files.
type Store interface {
	// Put stores data with extension ext ("webp", "png", "jpg") and returns
	// its key; storing identical bytes again is a no-op.
	Put(ctx context.Context, data []byte, ext string) (string, error)
	// Open returns the file for key or ErrNotFound.
	Open(ctx context.Context, key string) (io.ReadSeekCloser, error)
}

// LocalStore stores files under {data_dir}/uploads through os.OpenRoot,
// so keys can never escape the directory. Writes go to a temporary file
// that is renamed into place.
type LocalStore struct {
	root *os.Root
}

// NewLocalStore opens (creating if needed) dir and returns the store.
// Temporary files left behind by an interrupted write are removed.
func NewLocalStore(dir string) (*LocalStore, error) {
	if dir == "" {
		return nil, errors.New("media: store directory is empty")
	}
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return nil, fmt.Errorf("media: create %s: %w", dir, err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("media: open %s: %w", dir, err)
	}
	s := &LocalStore{root: root}
	if err := s.removeTemps(); err != nil {
		_ = root.Close()
		return nil, err
	}
	return s, nil
}

func (s *LocalStore) removeTemps() error {
	entries, err := fs.ReadDir(s.root.FS(), ".")
	if err != nil {
		return fmt.Errorf("media: list store: %w", err)
	}
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), tmpPrefix) {
			continue
		}
		if err := s.root.Remove(e.Name()); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("media: remove stale %s: %w", e.Name(), err)
		}
	}
	return nil
}

// KeyFor returns the content-addressed key for data and ext:
// hex(sha256(data)[:16]) + "." + ext.
func KeyFor(data []byte, ext string) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:16]) + "." + ext
}

// Put implements Store.
func (s *LocalStore) Put(ctx context.Context, data []byte, ext string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if ContentTypeForExt(ext) == "" {
		return "", fmt.Errorf("media: unsupported extension %q", ext)
	}
	if len(data) == 0 {
		return "", errors.New("media: refusing to store an empty file")
	}
	key := KeyFor(data, ext)
	if _, err := s.root.Stat(key); err == nil {
		return key, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("media: stat %s: %w", key, err)
	}
	if err := s.writeAtomic(key, data); err != nil {
		return "", err
	}
	return key, nil
}

func (s *LocalStore) writeAtomic(key string, data []byte) error {
	tmp, err := tempName()
	if err != nil {
		return err
	}
	f, err := s.root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, fileMode)
	if err != nil {
		return fmt.Errorf("media: create temp file: %w", err)
	}
	_, werr := f.Write(data)
	if werr == nil {
		werr = f.Sync()
	}
	cerr := f.Close()
	if err := errors.Join(werr, cerr); err != nil {
		_ = s.root.Remove(tmp)
		return fmt.Errorf("media: write %s: %w", key, err)
	}
	if err := s.root.Rename(tmp, key); err != nil {
		_ = s.root.Remove(tmp)
		return fmt.Errorf("media: rename %s: %w", key, err)
	}
	return nil
}

func tempName() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("media: temp name: %w", err)
	}
	return tmpPrefix + hex.EncodeToString(b[:]), nil
}

// Open implements Store.
func (s *LocalStore) Open(ctx context.Context, key string) (io.ReadSeekCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !KeyRe.MatchString(key) {
		return nil, ErrNotFound
	}
	f, err := s.root.Open(key)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("media: open %s: %w", key, err)
	}
	return f, nil
}

// Close releases the root.
func (s *LocalStore) Close() error {
	if s.root == nil {
		return nil
	}
	return s.root.Close()
}

// readStored reads a whole stored file, bounded by limit bytes.
func readStored(ctx context.Context, store Store, key string, limit int64) ([]byte, error) {
	f, err := store.Open(ctx, key)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, fmt.Errorf("media: read %s: %w", key, err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("media: stored %s: %w", key, ErrTooLarge)
	}
	return data, nil
}
