package source

import (
	"context"
	"strings"

	"github.com/0xbenc/termnav"
)

// TreeSource is a zero-IO FileSource over an in-memory navigation Index built
// from a flat snapshot of entry paths (passage's implicit pass-store tree). It
// implements FileSource, SyncLister, and Indexer — the last proving that
// path-completion and directory-browse are two renderings of one navigation
// index: the same snapshot that drives the composer's TAB completion also backs
// a full browser. Listing a node never blocks, so navigation is instant.
type TreeSource struct {
	idx *termnav.Index
	opt TreeOptions
}

// TreeOptions configures the cosmetics of a TreeSource's rows.
type TreeOptions struct {
	FolderSuffix                               string // appended to a folder's Title; default "/"
	UseRow                                     bool   // inject a "Use this folder" row
	UseTitle                                   string
	FolderGroup, EntryGroup, UseGroup, UpGroup string
	FolderBadge, EntryBadge, UseBadge, UpBadge string
	FolderKind, EntryKind, UseKind, UpKind     string
}

// NewTree builds a TreeSource from a flat list of entry paths.
func NewTree(entryPaths []string, opt TreeOptions) *TreeSource {
	return &TreeSource{idx: termnav.BuildIndex(entryPaths), opt: opt}
}

var (
	_ termnav.FileSource = (*TreeSource)(nil)
	_ termnav.SyncLister = (*TreeSource)(nil)
	_ termnav.Indexer    = (*TreeSource)(nil)
)

// Index exposes the underlying navigation index for breadcrumb / TAB completion.
func (s *TreeSource) Index() *termnav.Index { return s.idx }

// Resolve normalizes a pass folder path: trims surrounding slashes; the root is
// the empty string.
func (s *TreeSource) Resolve(_ context.Context, path string) (string, error) {
	return strings.Trim(strings.TrimSpace(path), "/"), nil
}

// List returns the children of the folder at dir (no trailing slash; "" root).
func (s *TreeSource) List(_ context.Context, dir string) (termnav.Listing, error) {
	return s.ListSync(dir)
}

// ListSync is the synchronous (instant) listing.
func (s *TreeSource) ListSync(dir string) (termnav.Listing, error) {
	dir = strings.Trim(strings.TrimSpace(dir), "/")
	suffix := s.opt.FolderSuffix
	if suffix == "" {
		suffix = "/"
	}
	children := s.idx.Children(dir)
	rows := make([]termnav.Row, 0, len(children))
	for _, n := range children {
		full := n.Name
		if dir != "" {
			full = dir + "/" + n.Name
		}
		if n.IsFolder {
			rows = append(rows, termnav.Row{
				Token:       full,
				Title:       n.Name + suffix,
				Group:       s.opt.FolderGroup,
				Badge:       s.opt.FolderBadge,
				Kind:        s.opt.FolderKind,
				Intent:      termnav.IntentDescend,
				Selectable:  true,
				IsContainer: true,
			})
			continue
		}
		rows = append(rows, termnav.Row{
			Token:      full,
			Title:      n.Name,
			Group:      s.opt.EntryGroup,
			Badge:      s.opt.EntryBadge,
			Kind:       s.opt.EntryKind,
			Intent:     termnav.IntentSelectLeaf,
			Selectable: true,
		})
	}
	parent := ""
	if i := strings.LastIndex(dir, "/"); i >= 0 {
		parent = dir[:i]
	}
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
	// At the root, parent == dir == "" so InjectNav suppresses "..". For a
	// non-root dir whose parent is the root, parent "" != dir, so ".." appears.
	if dir != "" && parent == "" {
		nav.Parent = rootSentinel
	}
	rows = termnav.InjectNav(dir, rows, nav)
	// Rewrite the sentinel parent token back to the real root ("").
	for i := range rows {
		if rows[i].Intent == termnav.IntentAscend && rows[i].Token == rootSentinel {
			rows[i].Token = ""
			rows[i].Description = ""
		}
	}
	return termnav.Listing{Dir: dir, Parent: parent, Rows: rows}, nil
}

// rootSentinel lets a non-root directory advertise a ".." back to the empty
// root without InjectNav suppressing it (parent "" == dir "" is the suppress
// rule). It is rewritten to "" before the listing is returned.
const rootSentinel = "\x00root"
