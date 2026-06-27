package source_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/0xbenc/termnav"
	"github.com/0xbenc/termnav/source"
)

func fixtureDir(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(root, "sub"), 0o755))
	must(os.MkdirAll(filepath.Join(root, "alpha"), 0o755))
	must(os.WriteFile(filepath.Join(root, "zeta.txt"), []byte("z"), 0o644))
	must(os.WriteFile(filepath.Join(root, ".hidden"), []byte("h"), 0o644))
	return root
}

func TestLocalSourceListsDirsFirstAndSuppressesHidden(t *testing.T) {
	root := fixtureDir(t)
	s := source.NewLocal(source.LocalOptions{SkipHidden: true, SelectFiles: true})
	l, err := s.ListSync(root)
	if err != nil {
		t.Fatal(err)
	}
	// ".." then alpha/, sub/ (dirs, alpha-sorted), then zeta.txt; .hidden omitted
	var titles []string
	for _, r := range l.Rows {
		titles = append(titles, r.Title)
	}
	want := []string{"..", "alpha/", "sub/", "zeta.txt"}
	if len(titles) != len(want) {
		t.Fatalf("titles = %v, want %v", titles, want)
	}
	for i := range want {
		if titles[i] != want[i] {
			t.Fatalf("titles = %v, want %v", titles, want)
		}
	}
}

func TestLocalSourceDirsOnlyAndUseRow(t *testing.T) {
	root := fixtureDir(t)
	s := source.NewLocal(source.LocalOptions{DirsOnly: true, UseRow: true})
	l, err := s.ListSync(root)
	if err != nil {
		t.Fatal(err)
	}
	if l.Rows[0].Intent != termnav.IntentUseContainer {
		t.Fatalf("first row intent = %v, want UseContainer", l.Rows[0].Intent)
	}
	for _, r := range l.Rows {
		if r.Intent == termnav.IntentSelectLeaf || r.Intent == termnav.IntentReference {
			t.Fatalf("DirsOnly listing contained a file row: %q", r.Title)
		}
	}
}

func TestLocalSourceConformance(t *testing.T) {
	root := fixtureDir(t)
	source.Conformance(t, source.NewLocal(source.LocalOptions{SelectFiles: true}), root)
}

func TestLocalSourceRootSuppressesParent(t *testing.T) {
	s := source.NewLocal(source.LocalOptions{})
	l, err := s.ListSync("/")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range l.Rows {
		if r.Intent == termnav.IntentAscend {
			t.Fatal("listing of / must not contain a '..' row")
		}
	}
}

func TestTreeSourceNavigates(t *testing.T) {
	s := source.NewTree([]string{"pp/alter-ego/proton", "pp/backup/key", "work/aws/db"}, source.TreeOptions{})
	// root: pp/, work/ (folders), no ".."
	root, _ := s.ListSync("")
	if len(root.Rows) != 2 {
		t.Fatalf("root rows = %d, want 2", len(root.Rows))
	}
	for _, r := range root.Rows {
		if r.Intent == termnav.IntentAscend {
			t.Fatal("tree root must not have a '..'")
		}
	}
	// descend into pp: ".." + alter-ego/ + backup/
	pp, _ := s.ListSync("pp")
	if pp.Rows[0].Intent != termnav.IntentAscend || pp.Rows[0].Token != "" {
		t.Fatalf("pp first row = %#v, want Ascend to root \"\"", pp.Rows[0])
	}
	// the deepest level holds the entry as a selectable leaf
	leaf, _ := s.ListSync("pp/alter-ego")
	var foundLeaf bool
	for _, r := range leaf.Rows {
		if r.Intent == termnav.IntentSelectLeaf && r.Token == "pp/alter-ego/proton" {
			foundLeaf = true
		}
	}
	if !foundLeaf {
		t.Fatalf("pp/alter-ego should list proton as a leaf: %#v", leaf.Rows)
	}
}

func TestTreeSourceIsIndexer(t *testing.T) {
	s := source.NewTree([]string{"a/b/c"}, source.TreeOptions{})
	var _ termnav.Indexer = s
	if s.Index() == nil {
		t.Fatal("TreeSource.Index() returned nil")
	}
	_ = context.Background()
}
