package source

import (
	"context"
	"testing"

	"github.com/0xbenc/termnav"
)

// Conformance is a table-driven harness that asserts a FileSource honors the
// contract every transport must keep, so a new source (an app's SFTP lister,
// say) verifies itself with one call. It resolves start, lists the result, and
// checks the structural invariants the reducer relies on. It performs real IO,
// so point it at a fixture directory the test controls.
func Conformance(t *testing.T, s termnav.FileSource, start string) {
	t.Helper()
	ctx := context.Background()

	dir, err := s.Resolve(ctx, start)
	if err != nil {
		t.Fatalf("Resolve(%q): %v", start, err)
	}

	listing, err := s.List(ctx, dir)
	if err != nil {
		t.Fatalf("List(%q): %v", dir, err)
	}
	if listing.Dir == "" {
		t.Errorf("List(%q): Listing.Dir is empty; a source must report its canonical cwd", dir)
	}

	// Parent-suppression-at-root: a "" or self parent must not yield a ".." row.
	if listing.Parent == "" || listing.Parent == listing.Dir {
		for _, r := range listing.Rows {
			if r.Intent == termnav.IntentAscend {
				t.Errorf("List(%q): emitted a '..' (Ascend) row at the root (Parent=%q Dir=%q)", dir, listing.Parent, listing.Dir)
			}
		}
	}

	for i, r := range listing.Rows {
		if r.Token == "" && r.Intent != termnav.IntentReference {
			t.Errorf("row %d (%q): selectable/navigable row has an empty Token", i, r.Title)
		}
		switch r.Intent {
		case termnav.IntentDescend, termnav.IntentAscend, termnav.IntentUseContainer,
			termnav.IntentSelectLeaf, termnav.IntentReference:
		default:
			t.Errorf("row %d (%q): unknown NavIntent %v", i, r.Title, r.Intent)
		}
		if r.Intent == termnav.IntentReference && r.Selectable {
			t.Errorf("row %d (%q): a reference row must not be Selectable", i, r.Title)
		}
	}

	// SyncLister, when present, must agree with List on the row count.
	if sl, ok := s.(termnav.SyncLister); ok {
		sync, err := sl.ListSync(dir)
		if err != nil {
			t.Fatalf("ListSync(%q): %v", dir, err)
		}
		if len(sync.Rows) != len(listing.Rows) {
			t.Errorf("ListSync/List row count mismatch: %d vs %d", len(sync.Rows), len(listing.Rows))
		}
	}
}
