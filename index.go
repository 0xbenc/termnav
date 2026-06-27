package termnav

// This file is the navigation INDEX kernel, lifted from passage's
// internal/ui/pathindex.go and generalized with exported names. It is an
// immutable, point-in-time snapshot of a flat set of "entry paths" precomputed
// into the lookups a hierarchical path completer needs. Folders are implicit —
// there is no folder entity — so every leading path segment is treated as a
// folder. All fields are read-only after construction so a value-copied caller
// (passage's composerModel, copied on every keystroke) can share it safely.
//
// It is pure and total: a nil/empty input yields an index whose lookups all
// miss and whose children are empty. The same Index backs passage's
// new-entry path completion today and, via a FileSource Indexer, breadcrumb +
// shell-TAB completion on any browser tomorrow.

import (
	"sort"
	"strings"
	"unicode"
)

// Node is one immediate child of a folder in the implicit tree. A name can be
// both a folder and an entry at once (an "a/b" entry alongside an "a/b/c"
// entry), so the two flags are independent.
type Node struct {
	Name     string
	IsFolder bool
	IsEntry  bool
}

// Index is the precomputed snapshot. Construct it with BuildIndex.
type Index struct {
	entries     map[string]struct{} // exact full entry paths
	entriesFold map[string]string   // lower(path) -> canonical path, for case-collision detection
	folders     map[string]struct{} // every leading-prefix folder (no trailing slash): "pp", "pp/alter-ego"
	children    map[string][]Node
}

// BuildIndex derives the implicit folder tree from a snapshot of entry paths.
// It is pure and total.
func BuildIndex(entryPaths []string) *Index {
	idx := &Index{
		entries:     map[string]struct{}{},
		entriesFold: map[string]string{},
		folders:     map[string]struct{}{},
		children:    map[string][]Node{},
	}
	type agg struct{ isFolder, isEntry bool }
	childAgg := map[string]map[string]*agg{}
	addChild := func(dir, name string, isFolder bool) {
		m := childAgg[dir]
		if m == nil {
			m = map[string]*agg{}
			childAgg[dir] = m
		}
		a := m[name]
		if a == nil {
			a = &agg{}
			m[name] = a
		}
		if isFolder {
			a.isFolder = true
		} else {
			a.isEntry = true
		}
	}
	for _, raw := range entryPaths {
		p := strings.Trim(strings.TrimSpace(raw), "/")
		if p == "" {
			continue
		}
		if _, ok := idx.entries[p]; !ok {
			idx.entries[p] = struct{}{}
			if lower := strings.ToLower(p); idx.entriesFold[lower] == "" {
				idx.entriesFold[lower] = p
			}
		}
		parts := strings.Split(p, "/")
		for i, part := range parts {
			if part == "" { // defensive: a malformed "a//b" snapshot
				continue
			}
			parent := strings.Join(parts[:i], "/")
			isFolder := i < len(parts)-1
			addChild(parent, part, isFolder)
			if isFolder {
				idx.folders[strings.Join(parts[:i+1], "/")] = struct{}{}
			}
		}
	}
	for dir, m := range childAgg {
		nodes := make([]Node, 0, len(m))
		for name, a := range m {
			nodes = append(nodes, Node{Name: name, IsFolder: a.isFolder, IsEntry: a.isEntry})
		}
		sortNodes(nodes)
		idx.children[dir] = nodes
	}
	return idx
}

// sortNodes orders children folders-first, then by name. A name that is both a
// folder and an entry sorts as a folder.
func sortNodes(nodes []Node) {
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].IsFolder != nodes[j].IsFolder {
			return nodes[i].IsFolder
		}
		return nodes[i].Name < nodes[j].Name
	})
}

// SplitPath splits a path field into (dir, frag): dir is the text up to and
// including the last "/" ("" when there is none); frag is the active segment
// after it. "pp/al" -> ("pp/","al"); "pp" -> ("","pp"); "pp/" -> ("pp/","").
func SplitPath(field string) (dir, frag string) {
	if i := strings.LastIndex(field, "/"); i >= 0 {
		return field[:i+1], field[i+1:]
	}
	return "", field
}

// folderKey turns a dir (with trailing slash, as SplitPath returns) into the
// children-map key (no trailing slash; "" for root).
func folderKey(dir string) string {
	return strings.TrimSuffix(dir, "/")
}

// Candidate is one child of the current folder, tagged with whether it
// prefix-matches the active fragment (case-insensitively) and, if so, the rune
// positions to highlight in its name.
type Candidate struct {
	Node      Node
	Match     bool
	Positions []int
}

// Candidates returns the children of dir, matches first (preserving
// folder-first/name order within each group). Matching is case-insensitive
// prefix — the same lockstep relationship TAB uses — so everything listed as a
// match is TAB-completable.
func (ix *Index) Candidates(dir, frag string) []Candidate {
	children := ix.children[folderKey(dir)]
	out := make([]Candidate, 0, len(children))
	fragLower := strings.ToLower(frag)
	fragRunes := len([]rune(frag))
	for _, n := range children {
		match := fragLower == "" || strings.HasPrefix(strings.ToLower(n.Name), fragLower)
		c := Candidate{Node: n, Match: match}
		if match && fragRunes > 0 {
			pos := make([]int, fragRunes)
			for i := range pos {
				pos[i] = i
			}
			c.Positions = pos
		}
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Match != out[j].Match {
			return out[i].Match
		}
		return false
	})
	return out
}

// MatchingCandidates is the subset of Candidates that prefix-match — the set
// TAB acts on.
func MatchingCandidates(cands []Candidate) []Candidate {
	out := make([]Candidate, 0, len(cands))
	for _, c := range cands {
		if c.Match {
			out = append(out, c)
		}
	}
	return out
}

// CommonPrefix is the longest common prefix of the matching candidate names,
// compared case-insensitively but emitted in the first match's canonical case —
// true shell first-TAB behavior.
func CommonPrefix(cands []Candidate) string {
	var first []rune
	var common []rune
	have := false
	for _, c := range cands {
		if !c.Match {
			continue
		}
		nr := []rune(c.Node.Name)
		if !have {
			first = nr
			common = nr
			have = true
			continue
		}
		n := minInt(len(common), len(nr))
		k := 0
		for k < n && foldEqualRune(common[k], nr[k]) {
			k++
		}
		common = common[:k]
	}
	if !have {
		return ""
	}
	return string(first[:len(common)])
}

// ApplyNode fills dir+node into a path field: a folder becomes "dir/name/" (and
// descends); an entry becomes "dir/name". A name that is both descends, since
// the user can always backspace to target the entry.
func ApplyNode(dir string, n Node) (field string, descended bool) {
	if n.IsFolder {
		return dir + n.Name + "/", true
	}
	return dir + n.Name, false
}

// LeafKind classifies a typed path for enter-validation and breadcrumb valence.
type LeafKind int

const (
	LeafEmpty         LeafKind = iota // nothing typed
	LeafTrailingSlash                 // ends with "/", no name yet
	LeafEmptySegment                  // "a//b" or "/a"
	LeafIsFolder                      // exact existing folder, no trailing name
	LeafExistingEntry                 // exact existing entry (overwrite)
	LeafCaseCollision                 // matches an existing entry only by case
	LeafNew                           // a fresh entry to create
)

// Classify is the single source of truth for whether enter may proceed and for
// the leaf's valence. Precedence: exact entry > exact folder > case collision >
// new.
func (ix *Index) Classify(field string) LeafKind {
	f := strings.TrimSpace(field)
	switch {
	case f == "":
		return LeafEmpty
	case strings.HasSuffix(f, "/"):
		return LeafTrailingSlash
	}
	for _, seg := range strings.Split(f, "/") {
		if seg == "" {
			return LeafEmptySegment
		}
	}
	if _, ok := ix.entries[f]; ok {
		return LeafExistingEntry
	}
	if _, ok := ix.folders[f]; ok {
		return LeafIsFolder
	}
	if canon, ok := ix.entriesFold[strings.ToLower(f)]; ok && canon != f {
		return LeafCaseCollision
	}
	return LeafNew
}

// CaseCanonical returns the canonical (stored) spelling of an entry that the
// field collides with only by case, for the notice message.
func (ix *Index) CaseCanonical(field string) string {
	return ix.entriesFold[strings.ToLower(strings.TrimSpace(field))]
}

// Crumb is one committed folder segment of the path for a read-only breadcrumb
// line, with a case-insensitive existence flag.
type Crumb struct {
	Name   string
	Exists bool
}

// Breadcrumb returns the committed folder segments (everything before the last
// "/") with existence flags, plus the active leaf and its kind — so render can
// color exists/missing/new without splicing into the cursored field.
func (ix *Index) Breadcrumb(field string) (segs []Crumb, leaf string, kind LeafKind) {
	kind = ix.Classify(field)
	dir, frag := SplitPath(field)
	leaf = frag
	key := folderKey(dir)
	if key == "" {
		return nil, leaf, kind
	}
	parts := strings.Split(key, "/")
	for i := range parts {
		if parts[i] == "" {
			continue
		}
		prefix := strings.Join(parts[:i+1], "/")
		_, exists := ix.folders[prefix]
		segs = append(segs, Crumb{Name: parts[i], Exists: exists})
	}
	return segs, leaf, kind
}

func foldEqualRune(a, b rune) bool {
	return a == b || unicode.ToLower(a) == unicode.ToLower(b)
}

// AscendPath drops the trailing path segment, for shift+tab: "pp/alter-ego/gm"
// -> "pp/alter-ego/" -> "pp/" -> "". A trailing slash is removed first.
func AscendPath(field string) string {
	field = strings.TrimSuffix(field, "/")
	if i := strings.LastIndex(field, "/"); i >= 0 {
		return field[:i+1]
	}
	return ""
}

// DisplayDir names a dir (with trailing slash, as SplitPath returns) for a
// notice; the root reads as "the store root".
func DisplayDir(dir string) string {
	if folderKey(dir) == "" {
		return "the store root"
	}
	return folderKey(dir)
}

// ChildCount is the number of immediate children under the child named `name`
// of `dir` (with trailing slash), used to show a folder's item count.
func (ix *Index) ChildCount(dir, name string) int {
	return len(ix.children[childPath(dir, name)])
}

// Children returns the immediate children of the folder at `folder` (no
// trailing slash; "" is the root), folders-first then by name. The returned
// slice must not be mutated.
func (ix *Index) Children(folder string) []Node {
	return ix.children[strings.TrimSuffix(folder, "/")]
}

// IsFolder reports whether `path` (no trailing slash) is a known implicit folder.
func (ix *Index) IsFolder(path string) bool {
	_, ok := ix.folders[strings.TrimSuffix(path, "/")]
	return ok
}

// childPath is the full path of a child name under dir (with trailing slash).
func childPath(dir, name string) string {
	if k := folderKey(dir); k != "" {
		return k + "/" + name
	}
	return name
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
