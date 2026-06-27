package termnav

import "testing"

func leafRow(token, title string) Row {
	return Row{Token: token, Title: title, Intent: IntentSelectLeaf, Selectable: true}
}
func dirRow(token, title string) Row {
	return Row{Token: token, Title: title, Intent: IntentDescend, Selectable: true, IsContainer: true}
}
func refRow(title string) Row {
	return Row{Title: title, Intent: IntentReference, Selectable: false}
}

func listing(dir, parent string, rows ...Row) Listing {
	return Listing{Dir: dir, Parent: parent, Rows: rows}
}

func TestLoadEmitsListEffect(t *testing.T) {
	m := New(Options{})
	m, effects := m.Load("/start")
	if len(effects) != 1 {
		t.Fatalf("first Load effects = %d, want 1 (no cancel on gen 0)", len(effects))
	}
	le, ok := effects[0].(ListDirEffect)
	if !ok || le.Dir != "/start" || le.Gen != 1 {
		t.Fatalf("effect = %#v, want ListDirEffect{Dir:/start,Gen:1}", effects[0])
	}
	if m.State() != Loading {
		t.Fatalf("state = %v, want Loading", m.State())
	}
}

func TestListLoadedReadyAndEmpty(t *testing.T) {
	m := New(Options{})
	m, _ = m.Load("/a")
	m, _ = Update(m, ListLoadedEvent{Gen: 1, Listing: listing("/a", "/", dirRow("/a/x", "x/"), leafRow("/a/f", "f"))})
	if m.State() != Ready || m.Cwd() != "/a" {
		t.Fatalf("state=%v cwd=%q, want Ready /a", m.State(), m.Cwd())
	}
	if len(m.Filtered()) != 2 {
		t.Fatalf("filtered = %d, want 2", len(m.Filtered()))
	}

	m2 := New(Options{})
	m2, _ = m2.Load("/empty")
	m2, _ = Update(m2, ListLoadedEvent{Gen: 1, Listing: listing("/empty", "/")})
	if m2.State() != Empty {
		t.Fatalf("state = %v, want Empty", m2.State())
	}
}

func TestStaleListingDropped(t *testing.T) {
	m := New(Options{})
	m, _ = m.Load("/a") // gen 1
	// user descends again before /a arrives -> gen 2
	m, _ = m.Load("/a/b")
	// the stale gen-1 listing must be ignored
	m, _ = Update(m, ListLoadedEvent{Gen: 1, Listing: listing("/a", "/", leafRow("/a/stale", "stale"))})
	if m.Cwd() == "/a" || len(m.Rows()) != 0 {
		t.Fatalf("stale gen-1 listing was applied: cwd=%q rows=%d", m.Cwd(), len(m.Rows()))
	}
	// the fresh gen-2 listing lands
	m, _ = Update(m, ListLoadedEvent{Gen: 2, Listing: listing("/a/b", "/a", leafRow("/a/b/f", "f"))})
	if m.Cwd() != "/a/b" {
		t.Fatalf("fresh listing not applied: cwd=%q", m.Cwd())
	}
}

func TestDescendBumpsGenAndCancels(t *testing.T) {
	m := New(Options{})
	m, _ = m.Load("/a")
	m, _ = Update(m, ListLoadedEvent{Gen: 1, Listing: listing("/a", "/", dirRow("/a/sub", "sub/"))})
	m, effects := Update(m, KeyEvent{Key: "enter"})
	// expect CancelListEffect{1} + ListDirEffect{2,/a/sub}
	if len(effects) != 2 {
		t.Fatalf("descend effects = %d, want 2", len(effects))
	}
	if c, ok := effects[0].(CancelListEffect); !ok || c.Gen != 1 {
		t.Fatalf("effect[0] = %#v, want CancelListEffect{1}", effects[0])
	}
	if l, ok := effects[1].(ListDirEffect); !ok || l.Dir != "/a/sub" || l.Gen != 2 {
		t.Fatalf("effect[1] = %#v, want ListDirEffect{/a/sub,2}", effects[1])
	}
}

func TestSelectLeafCommits(t *testing.T) {
	m := New(Options{})
	m, _ = m.Load("/a")
	m, _ = Update(m, ListLoadedEvent{Gen: 1, Listing: listing("/a", "/", leafRow("/a/f", "f"))})
	m, _ = Update(m, KeyEvent{Key: "enter"})
	out, ok := m.Outcome()
	if !ok || out.Token() != "/a/f" || out.Intent != IntentSelectLeaf {
		t.Fatalf("outcome = %#v ok=%v, want /a/f leaf", out, ok)
	}
	if !m.Done() {
		t.Fatal("model should be Done after a commit")
	}
}

func TestValidatorBlocksCommit(t *testing.T) {
	m := New(Options{Validate: func(r Row) (bool, string) { return false, "nope" }})
	m, _ = m.Load("/a")
	m, _ = Update(m, ListLoadedEvent{Gen: 1, Listing: listing("/a", "/", leafRow("/a/f", "f"))})
	m, _ = Update(m, KeyEvent{Key: "enter"})
	if _, ok := m.Outcome(); ok {
		t.Fatal("validator returned false but commit went through")
	}
	if m.Notice() != "nope" {
		t.Fatalf("notice = %q, want nope", m.Notice())
	}
}

func TestCancel(t *testing.T) {
	m := New(Options{})
	m, _ = m.Load("/a")
	m, _ = Update(m, KeyEvent{Key: "cancel"})
	if !m.Canceled() || !m.Done() {
		t.Fatal("cancel did not terminate the browse")
	}
}

func TestErrorState(t *testing.T) {
	m := New(Options{})
	m, _ = m.Load("/a")
	m, _ = Update(m, ListLoadedEvent{Gen: 1, Err: errString("permission denied")})
	if m.State() != Error || m.Notice() != "permission denied" {
		t.Fatalf("state=%v notice=%q, want Error/permission denied", m.State(), m.Notice())
	}
}

func TestCursorSnapsOverReferenceRows(t *testing.T) {
	m := New(Options{})
	m, _ = m.Load("/a")
	// dir (selectable), ref (not), ref (not), leaf (selectable)
	m, _ = Update(m, ListLoadedEvent{Gen: 1, Listing: listing("/a", "/",
		dirRow("/a/d", "d/"), refRow("r1"), refRow("r2"), leafRow("/a/f", "f"))})
	if m.Cursor() != 0 {
		t.Fatalf("initial cursor = %d, want 0 (first selectable)", m.Cursor())
	}
	// moving down once should skip both reference rows and land on the leaf (index 3)
	m, _ = Update(m, KeyEvent{Key: "down"})
	if m.Cursor() != 3 {
		t.Fatalf("after down, cursor = %d, want 3 (skipped reference rows)", m.Cursor())
	}
	r, _ := m.FocusedRow()
	if r.Token != "/a/f" {
		t.Fatalf("focused = %q, want /a/f", r.Token)
	}
}

func TestFilterNarrows(t *testing.T) {
	m := New(Options{Matcher: Substring{}})
	m, _ = m.Load("/a")
	m, _ = Update(m, ListLoadedEvent{Gen: 1, Listing: listing("/a", "/",
		leafRow("/a/apple", "apple"), leafRow("/a/banana", "banana"), leafRow("/a/grape", "grape"))})
	for _, ch := range "an" {
		m, _ = Update(m, KeyEvent{Text: string(ch)})
	}
	// "an" substring matches banana only
	if len(m.Filtered()) != 1 {
		t.Fatalf("filtered for 'an' = %d, want 1 (banana)", len(m.Filtered()))
	}
	r, _ := m.FocusedRow()
	if r.Title != "banana" {
		t.Fatalf("focused after filter = %q, want banana", r.Title)
	}
	// backspace widens again
	m, _ = Update(m, KeyEvent{Key: "backspace"})
	m, _ = Update(m, KeyEvent{Key: "backspace"})
	if len(m.Filtered()) != 3 {
		t.Fatalf("filtered after clearing = %d, want 3", len(m.Filtered()))
	}
}

type errString string

func (e errString) Error() string { return string(e) }
