package webui

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"strings"
)

// Manifest entry keys produced by the Vite build (web/).
const (
	ManifestPath = ".vite/manifest.json"
	PublicEntry  = "index.html"
	AdminEntry   = "admin/index.html"
	assetsPrefix = "assets/"
)

// ManifestChunk is one entry of a Vite manifest.
type ManifestChunk struct {
	File           string   `json:"file"`
	Name           string   `json:"name,omitempty"`
	Src            string   `json:"src,omitempty"`
	IsEntry        bool     `json:"isEntry,omitempty"`
	Imports        []string `json:"imports,omitempty"`
	DynamicImports []string `json:"dynamicImports,omitempty"`
	CSS            []string `json:"css,omitempty"`
	Assets         []string `json:"assets,omitempty"`
}

// Manifest is a parsed Vite manifest keyed by source path.
type Manifest map[string]ManifestChunk

// EntryAssets are the URLs an HTML page needs for one entry.
type EntryAssets struct {
	Script   string
	Styles   []string
	Preloads []string
}

// ParseManifest decodes a Vite manifest.
func ParseManifest(b []byte) (Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("parse vite manifest: %w", err)
	}
	return m, nil
}

// Entry resolves an entry's script, its CSS (including CSS of statically
// imported chunks) and the static import chain for modulepreload.
func (m Manifest) Entry(key string) (EntryAssets, error) {
	chunk, ok := m[key]
	if !ok {
		return EntryAssets{}, fmt.Errorf("vite manifest has no entry %q", key)
	}
	if !chunk.IsEntry {
		return EntryAssets{}, fmt.Errorf("vite manifest key %q is not an entry", key)
	}
	if err := validAssetPath(chunk.File); err != nil {
		return EntryAssets{}, err
	}
	out := EntryAssets{Script: "/" + chunk.File}
	seenCSS := map[string]bool{}
	if err := addCSS(&out, chunk.CSS, seenCSS); err != nil {
		return EntryAssets{}, err
	}
	visited := map[string]bool{key: true}
	if err := m.walkImports(chunk.Imports, visited, seenCSS, &out); err != nil {
		return EntryAssets{}, err
	}
	return out, nil
}

func (m Manifest) walkImports(imports []string, visited, seenCSS map[string]bool, out *EntryAssets) error {
	for _, imp := range imports {
		if visited[imp] {
			continue
		}
		visited[imp] = true
		c, ok := m[imp]
		if !ok {
			return fmt.Errorf("vite manifest import %q is missing", imp)
		}
		if err := validAssetPath(c.File); err != nil {
			return err
		}
		out.Preloads = append(out.Preloads, "/"+c.File)
		if err := addCSS(out, c.CSS, seenCSS); err != nil {
			return err
		}
		if err := m.walkImports(c.Imports, visited, seenCSS, out); err != nil {
			return err
		}
	}
	return nil
}

func addCSS(out *EntryAssets, css []string, seen map[string]bool) error {
	for _, c := range css {
		if err := validAssetPath(c); err != nil {
			return err
		}
		if !seen[c] {
			seen[c] = true
			out.Styles = append(out.Styles, "/"+c)
		}
	}
	return nil
}

func validAssetPath(p string) error {
	if !strings.HasPrefix(p, assetsPrefix) || !fs.ValidPath(p) || strings.ContainsAny(p, `"'<>\ `) {
		return fmt.Errorf("vite manifest file %q must be a clean path under %s", p, assetsPrefix)
	}
	return nil
}

// Assets is the loaded frontend build.
type Assets struct {
	fsys         fs.FS
	public       *EntryAssets
	admin        *EntryAssets
	manifestHash string
}

// LoadAssets loads the build from fsys (the dist root). A missing manifest
// puts Assets into fallback mode (no scripts); any other problem, including
// manifest files that do not exist in fsys, is an error.
func LoadAssets(fsys fs.FS) (*Assets, error) {
	b, err := fs.ReadFile(fsys, ManifestPath)
	if errors.Is(err, fs.ErrNotExist) {
		return &Assets{fsys: fsys, manifestHash: "none"}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read vite manifest: %w", err)
	}
	m, err := ParseManifest(b)
	if err != nil {
		return nil, err
	}
	pub, err := m.Entry(PublicEntry)
	if err != nil {
		return nil, err
	}
	adm, err := m.Entry(AdminEntry)
	if err != nil {
		return nil, err
	}
	for _, e := range []EntryAssets{pub, adm} {
		if err := checkExists(fsys, e); err != nil {
			return nil, err
		}
	}
	sum := sha256.Sum256(b)
	return &Assets{fsys: fsys, public: &pub, admin: &adm, manifestHash: hex.EncodeToString(sum[:8])}, nil
}

func checkExists(fsys fs.FS, e EntryAssets) error {
	all := append([]string{e.Script}, e.Styles...)
	all = append(all, e.Preloads...)
	for _, u := range all {
		if _, err := fs.Stat(fsys, strings.TrimPrefix(u, "/")); err != nil {
			return fmt.Errorf("vite manifest references missing file %s: %w", u, err)
		}
	}
	return nil
}

// Fallback reports whether no frontend build is available.
func (a *Assets) Fallback() bool { return a.public == nil }

// ManifestHash identifies the build (used in ETags).
func (a *Assets) ManifestHash() string { return a.manifestHash }
