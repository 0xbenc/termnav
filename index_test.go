package termnav

// These tests are ported verbatim (renamed to the exported API) from passage's
// internal/ui/pathindex_test.go, pinning the navigation-index contract so the
// extraction cannot silently regress passage's path completion.

import (
	"reflect"
	"testing"
)

var fixturePaths = []string{
	"pp/alter-ego/proton",
	"pp/backup/key",
	"work/aws/db",
}

func TestBuildIndexFolders(t *testing.T) {
	idx := BuildIndex(fixturePaths)
	for _, f := range []string{"pp", "pp/alter-ego", "pp/backup", "work", "work/aws"} {
		if _, ok := idx.folders[f]; !ok {
			t.Errorf("expected folder %q in index", f)
		}
	}
	for _, notFolder := range []string{"pp/alter-ego/proton", "pp/backup/key", "work/aws/db"} {
		if _, ok := idx.folders[notFolder]; ok {
			t.Errorf("%q should be an entry, not a folder", notFolder)
		}
		if _, ok := idx.entries[notFolder]; !ok {
			t.Errorf("expected entry %q in index", notFolder)
		}
	}
}

func TestBuildIndexChildrenSortedFoldersFirst(t *testing.T) {
	idx := BuildIndex([]string{"pp/zeta/x", "pp/alpha", "pp/beta/y"})
	got := idx.children[""]
	want := []Node{{Name: "pp", IsFolder: true}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("root children = %#v, want %#v", got, want)
	}
	ppc := idx.children["pp"]
	wantPP := []Node{
		{Name: "beta", IsFolder: true},
		{Name: "zeta", IsFolder: true},
		{Name: "alpha", IsEntry: true},
	}
	if !reflect.DeepEqual(ppc, wantPP) {
		t.Fatalf("pp children = %#v, want %#v", ppc, wantPP)
	}
}

func TestBuildIndexFileFolderCollision(t *testing.T) {
	idx := BuildIndex([]string{"a/b", "a/b/c"})
	ac := idx.children["a"]
	if len(ac) != 1 {
		t.Fatalf("a children = %#v, want one node", ac)
	}
	if !ac[0].IsFolder || !ac[0].IsEntry || ac[0].Name != "b" {
		t.Fatalf("a/b should be folder AND entry: %#v", ac[0])
	}
	if k := idx.Classify("a/b"); k != LeafExistingEntry {
		t.Fatalf("Classify(a/b) = %v, want LeafExistingEntry", k)
	}
}

func TestBuildIndexEmpty(t *testing.T) {
	for _, in := range [][]string{nil, {}, {"", "  ", "/"}} {
		idx := BuildIndex(in)
		if len(idx.entries) != 0 || len(idx.folders) != 0 || len(idx.children) != 0 {
			t.Fatalf("empty input %#v built non-empty index", in)
		}
		if k := idx.Classify("anything/new"); k != LeafNew {
			t.Fatalf("empty store: Classify = %v, want LeafNew", k)
		}
	}
}

func TestSplitPath(t *testing.T) {
	cases := []struct{ in, dir, frag string }{
		{"", "", ""},
		{"p", "", "p"},
		{"pp/", "pp/", ""},
		{"pp/al", "pp/", "al"},
		{"pp/alter-ego/gm", "pp/alter-ego/", "gm"},
		{"/x", "/", "x"},
	}
	for _, c := range cases {
		dir, frag := SplitPath(c.in)
		if dir != c.dir || frag != c.frag {
			t.Errorf("SplitPath(%q) = (%q,%q), want (%q,%q)", c.in, dir, frag, c.dir, c.frag)
		}
	}
}

func TestCandidates(t *testing.T) {
	idx := BuildIndex(fixturePaths)

	all := idx.Candidates("", "")
	if len(all) != 2 {
		t.Fatalf("root candidates = %d, want 2", len(all))
	}
	for _, c := range all {
		if !c.Match || c.Positions != nil {
			t.Fatalf("empty frag candidate should match with no positions: %#v", c)
		}
	}

	got := idx.Candidates("", "p")
	if got[0].Node.Name != "pp" || !got[0].Match {
		t.Fatalf("first candidate = %#v, want matching pp", got[0])
	}
	if !reflect.DeepEqual(got[0].Positions, []int{0}) {
		t.Fatalf("pp positions = %#v, want [0]", got[0].Positions)
	}
	if got[1].Node.Name != "work" || got[1].Match {
		t.Fatalf("second candidate = %#v, want non-matching work", got[1])
	}

	sub := idx.Candidates("pp/", "")
	names := []string{sub[0].Node.Name, sub[1].Node.Name}
	if !reflect.DeepEqual(names, []string{"alter-ego", "backup"}) {
		t.Fatalf("pp/ children = %v, want [alter-ego backup]", names)
	}

	ci := idx.Candidates("", "WOR")
	if mc := MatchingCandidates(ci); len(mc) != 1 || mc[0].Node.Name != "work" {
		t.Fatalf("case-insensitive WOR should match work: %#v", mc)
	}
}

func TestCommonPrefix(t *testing.T) {
	idx := BuildIndex([]string{"x/alpha/a", "x/alto/b", "x/beta/c"})
	cands := idx.Candidates("x/", "al")
	if got := CommonPrefix(cands); got != "al" {
		t.Fatalf("common prefix of alpha,alto = %q, want al", got)
	}
	if got := CommonPrefix(idx.Candidates("x/", "be")); got != "beta" {
		t.Fatalf("single-match prefix = %q, want beta", got)
	}
	if got := CommonPrefix(idx.Candidates("x/", "zzz")); got != "" {
		t.Fatalf("no-match prefix = %q, want empty", got)
	}
	ci := BuildIndex([]string{"Apple", "Apricot"})
	if got := CommonPrefix(ci.Candidates("", "ap")); got != "Ap" {
		t.Fatalf("canonical-case prefix = %q, want Ap", got)
	}
}

func TestClassify(t *testing.T) {
	idx := BuildIndex([]string{"pp/alter-ego/proton", "Work/github"})
	cases := []struct {
		field string
		want  LeafKind
	}{
		{"", LeafEmpty},
		{"   ", LeafEmpty},
		{"pp/", LeafTrailingSlash},
		{"pp//x", LeafEmptySegment},
		{"/x", LeafEmptySegment},
		{"pp", LeafIsFolder},
		{"pp/alter-ego", LeafIsFolder},
		{"pp/alter-ego/proton", LeafExistingEntry},
		{"work/github", LeafCaseCollision},
		{"Work/github", LeafExistingEntry},
		{"pp/alter-ego/gmail", LeafNew},
	}
	for _, c := range cases {
		if got := idx.Classify(c.field); got != c.want {
			t.Errorf("Classify(%q) = %v, want %v", c.field, got, c.want)
		}
	}
	if canon := idx.CaseCanonical("work/github"); canon != "Work/github" {
		t.Errorf("CaseCanonical = %q, want Work/github", canon)
	}
}

func TestApplyNode(t *testing.T) {
	f, descended := ApplyNode("pp/", Node{Name: "alter-ego", IsFolder: true})
	if f != "pp/alter-ego/" || !descended {
		t.Fatalf("folder apply = (%q,%v), want (pp/alter-ego/,true)", f, descended)
	}
	f, descended = ApplyNode("pp/alter-ego/", Node{Name: "proton", IsEntry: true})
	if f != "pp/alter-ego/proton" || descended {
		t.Fatalf("entry apply = (%q,%v), want (pp/alter-ego/proton,false)", f, descended)
	}
	f, descended = ApplyNode("a/", Node{Name: "b", IsFolder: true, IsEntry: true})
	if f != "a/b/" || !descended {
		t.Fatalf("dual node apply = (%q,%v), want (a/b/,true)", f, descended)
	}
}

func TestBreadcrumb(t *testing.T) {
	idx := BuildIndex(fixturePaths)
	segs, leaf, kind := idx.Breadcrumb("pp/alter-ego/gmail")
	want := []Crumb{{Name: "pp", Exists: true}, {Name: "alter-ego", Exists: true}}
	if !reflect.DeepEqual(segs, want) {
		t.Fatalf("segs = %#v, want %#v", segs, want)
	}
	if leaf != "gmail" || kind != LeafNew {
		t.Fatalf("leaf=%q kind=%v, want gmail/LeafNew", leaf, kind)
	}
	segs, _, _ = idx.Breadcrumb("pp/ghost/x")
	if len(segs) != 2 || segs[1].Name != "ghost" || segs[1].Exists {
		t.Fatalf("ghost folder should be flagged missing: %#v", segs)
	}
	segs, leaf, _ = idx.Breadcrumb("pp")
	if segs != nil || leaf != "pp" {
		t.Fatalf("root leaf: segs=%#v leaf=%q, want nil/pp", segs, leaf)
	}
}
