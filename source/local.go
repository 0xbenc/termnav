// Package source ships the concrete FileSource implementations that are
// generically useful — a local-filesystem lister and a zero-IO in-memory tree —
// plus a conformance harness any adapter (including an app's own SFTP source)
// can run. This is the one place stdlib os enters the shipped module; the kernel
// stays os-free. A transport that legitimately diverges per app — ssherpa's
// SFTP over a multiplexed SSH socket — lives in that app's shim, the
// divergent-transport analogue of an app's builtin palette.
package source

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/0xbenc/termnav"
)

// LocalOptions configures a LocalSource. The zero value lists a directory's
// children with files selectable and no synthetic rows — a plain file browser.
type LocalOptions struct {
	SkipHidden  bool // omit dotfiles
	DirsOnly    bool // omit files entirely (a folder picker)
	SelectFiles bool // files are selectable leaves (file picker) vs dimmed reference rows
	UseRow      bool // inject a "Use this folder" row that commits the current dir

	// Cosmetic + app-Kind fields. Empty leaves the corresponding Row field empty.
	DirSuffix                              string // appended to a directory's Title (e.g. "/"); default "/"
	UseTitle                               string // default "Use this folder"
	DirGroup, FileGroup, UseGroup, UpGroup string
	DirBadge, FileBadge, UseBadge, UpBadge string
	DirKind, FileKind, UseKind, UpKind     string

	// Describe builds a file row's Description (size, mtime). nil leaves it empty.
	Describe func(os.FileInfo) string
	// Decorate is a final per-row hook to set any remaining app cosmetics; it
	// receives the built row and the entry it came from. nil is a no-op.
	Decorate func(termnav.Row, os.DirEntry, os.FileInfo) termnav.Row
}

// LocalSource lists the local filesystem with os.ReadDir. It implements
// FileSource and SyncLister (local reads are instant, so the adapter skips the
// Loading flash).
type LocalSource struct{ opt LocalOptions }

// NewLocal returns a LocalSource with the given options.
func NewLocal(opt LocalOptions) *LocalSource { return &LocalSource{opt: opt} }

var (
	_ termnav.FileSource = (*LocalSource)(nil)
	_ termnav.SyncLister = (*LocalSource)(nil)
)

// Resolve canonicalizes a start path: it expands a leading ~, makes it
// absolute, Cleans it, and steps to the containing directory if it is a file —
// the exact normalization ssherpa's expandLocalPath performed.
func (s *LocalSource) Resolve(_ context.Context, path string) (string, error) {
	dir, err := ExpandLocalPath(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(dir)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		dir = filepath.Dir(dir)
	}
	return dir, nil
}

// List reads one directory's children. It honors the context for cancelation by
// checking it up front (a local read is fast, but a slow network mount is not).
func (s *LocalSource) List(ctx context.Context, dir string) (termnav.Listing, error) {
	if err := ctx.Err(); err != nil {
		return termnav.Listing{}, err
	}
	return s.ListSync(dir)
}

// ListSync is the synchronous read used by SyncLister-aware adapters.
func (s *LocalSource) ListSync(dir string) (termnav.Listing, error) {
	dir, err := ExpandLocalPath(dir)
	if err != nil {
		return termnav.Listing{}, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return termnav.Listing{}, err
	}
	suffix := s.opt.DirSuffix
	if suffix == "" {
		suffix = "/"
	}

	type built struct {
		row   termnav.Row
		isDir bool
		name  string
	}
	rows := make([]built, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if s.opt.SkipHidden && strings.HasPrefix(name, ".") {
			continue
		}
		info, ierr := entry.Info()
		if ierr != nil {
			continue
		}
		path := filepath.Join(dir, name)
		isDir := info.IsDir()
		isLink := info.Mode()&os.ModeSymlink != 0
		if isLink {
			// Match current behavior: a symlink is shown but not followed for
			// dir-ness; it lists as a leaf unless it resolves to a directory and
			// the caller opts into following (not enabled here).
			if target, terr := os.Stat(path); terr == nil {
				isDir = target.IsDir()
			} else {
				isDir = false
			}
		}
		if isDir {
			r := termnav.Row{
				Token:       path,
				Title:       name + suffix,
				Description: path,
				Group:       s.opt.DirGroup,
				Badge:       s.opt.DirBadge,
				Kind:        s.opt.DirKind,
				Intent:      termnav.IntentDescend,
				Selectable:  true,
				IsContainer: true,
				IsSymlink:   isLink,
			}
			if s.opt.Decorate != nil {
				r = s.opt.Decorate(r, entry, info)
			}
			rows = append(rows, built{row: r, isDir: true, name: strings.ToLower(name)})
			continue
		}
		if s.opt.DirsOnly {
			continue
		}
		desc := ""
		if s.opt.Describe != nil {
			desc = s.opt.Describe(info)
		}
		r := termnav.Row{
			Token:       path,
			Title:       name,
			Description: desc,
			Detail:      path,
			Group:       s.opt.FileGroup,
			Badge:       s.opt.FileBadge,
			Kind:        s.opt.FileKind,
			Intent:      termnav.IntentSelectLeaf,
			Selectable:  s.opt.SelectFiles,
			IsSymlink:   isLink,
		}
		if !s.opt.SelectFiles {
			r.Intent = termnav.IntentReference
		}
		if s.opt.Decorate != nil {
			r = s.opt.Decorate(r, entry, info)
		}
		rows = append(rows, built{row: r, isDir: false, name: strings.ToLower(name)})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].isDir != rows[j].isDir {
			return rows[i].isDir
		}
		return rows[i].name < rows[j].name
	})
	children := make([]termnav.Row, len(rows))
	for i, b := range rows {
		children[i] = b.row
	}

	parent := filepath.Dir(dir)
	nav := termnav.Nav{
		Parent:   parent,
		UseRow:   s.opt.UseRow,
		UseTitle: s.opt.UseTitle,
		UseGroup: s.opt.UseGroup,
		UseBadge: s.opt.UseBadge,
		UseKind:  s.opt.UseKind,
		UpGroup:  s.opt.UpGroup,
		UpBadge:  s.opt.UpBadge,
		UpKind:   s.opt.UpKind,
	}
	return termnav.Listing{
		Dir:    dir,
		Parent: parent,
		Rows:   termnav.InjectNav(dir, children, nav),
	}, nil
}

// ExpandLocalPath expands a leading ~, makes the path absolute, and Cleans it —
// the shared normalization both apps' local pickers performed.
func ExpandLocalPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		path = "."
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if path == "~" {
			path = home
		} else {
			path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	if !filepath.IsAbs(path) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return "", err
		}
		path = abs
	}
	return filepath.Clean(path), nil
}
