package source

import (
	"context"

	"github.com/0xbenc/termnav"
)

// StaticSource serves a single pre-built listing and never navigates. It adapts
// the existing "caller assembled a []Row already" path (a flat picker over a
// fixed set) onto the FileSource seam, so the same browse model and rendering
// drive a non-navigable list. List ignores its dir argument.
type StaticSource struct{ listing termnav.Listing }

// NewStatic wraps a fixed set of rows as a FileSource. dir labels the location.
func NewStatic(dir string, rows []termnav.Row) *StaticSource {
	return &StaticSource{listing: termnav.Listing{Dir: dir, Rows: rows}}
}

var (
	_ termnav.FileSource = (*StaticSource)(nil)
	_ termnav.SyncLister = (*StaticSource)(nil)
)

func (s *StaticSource) Resolve(_ context.Context, _ string) (string, error) {
	return s.listing.Dir, nil
}

func (s *StaticSource) List(_ context.Context, _ string) (termnav.Listing, error) {
	return s.listing, nil
}

func (s *StaticSource) ListSync(_ string) (termnav.Listing, error) {
	return s.listing, nil
}
