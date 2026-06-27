package termnav

import "testing"

// This file pins the cross-app contract: the invariants passage and ssherpa
// both depend on, so a change in termnav that would silently break either app
// fails here first.

// TestNavIntentSupersetIsStable locks the canonical 5-value intent superset and
// its order. Both apps map their private Kind vocabularies onto these constants;
// renumbering or reordering them would silently remap every app's rows.
func TestNavIntentSupersetIsStable(t *testing.T) {
	want := []struct {
		intent NavIntent
		value  uint8
		name   string
	}{
		{IntentDescend, 0, "descend"},
		{IntentAscend, 1, "ascend"},
		{IntentUseContainer, 2, "use"},
		{IntentSelectLeaf, 3, "leaf"},
		{IntentReference, 4, "reference"},
	}
	for _, w := range want {
		if uint8(w.intent) != w.value {
			t.Errorf("%s intent = %d, want %d (reordering breaks every app's Kind map)", w.name, uint8(w.intent), w.value)
		}
		if w.intent.String() != w.name {
			t.Errorf("intent %d String() = %q, want %q", w.value, w.intent.String(), w.name)
		}
	}
}

// TestBehaviorKeysOffIntentNotKind asserts the core acts on a row's NavIntent,
// never on its app-private Kind literal: two leaves with different Kinds commit
// identically, and the Kind round-trips through the Outcome so an app can
// recover its own vocabulary.
func TestBehaviorKeysOffIntentNotKind(t *testing.T) {
	for _, kind := range []string{"file", "remote_file", "anything-new"} {
		m := New(Options{})
		m, _ = m.Load("/d")
		m, _ = Update(m, ListLoadedEvent{Gen: 1, Listing: Listing{Dir: "/d", Parent: "/", Rows: []Row{
			{Token: "/d/x", Title: "x", Intent: IntentSelectLeaf, Selectable: true, Kind: kind},
		}}})
		m, _ = Update(m, KeyEvent{Key: "enter"})
		out, ok := m.Outcome()
		if !ok || out.Intent != IntentSelectLeaf || out.Token() != "/d/x" {
			t.Fatalf("kind %q: leaf did not commit by intent: out=%#v ok=%v", kind, out, ok)
		}
		if out.Rows[0].Kind != kind {
			t.Errorf("kind %q did not round-trip through the Outcome (got %q)", kind, out.Rows[0].Kind)
		}
	}
}

// TestUnknownIntentToleratedNotPanicked mirrors termtheme's tolerate-and-warn
// rule: a row carrying an intent the current core does not special-case renders
// as a plain, non-committing row rather than crashing.
func TestUnknownIntentTolerated(t *testing.T) {
	m := New(Options{})
	m, _ = m.Load("/d")
	future := NavIntent(250)
	m, _ = Update(m, ListLoadedEvent{Gen: 1, Listing: Listing{Dir: "/d", Parent: "/", Rows: []Row{
		{Token: "/d/x", Title: "x", Intent: future, Selectable: true},
	}}})
	// Enter on an unknown intent is a no-op (no commit, no panic).
	m, _ = Update(m, KeyEvent{Key: "enter"})
	if _, ok := m.Outcome(); ok {
		t.Fatal("an unknown intent should not commit")
	}
	if future.String() != "unknown" {
		t.Errorf("unknown intent String() = %q, want unknown", future.String())
	}
}

// TestParentSuppressedAtRoot pins the root-suppression rule InjectNav enforces:
// a listing whose parent equals its dir (or is empty) has no ".." row.
func TestParentSuppressedAtRoot(t *testing.T) {
	atRoot := InjectNav("/", []Row{{Token: "/a", Title: "a", Intent: IntentDescend, Selectable: true}}, Nav{Parent: "/"})
	for _, r := range atRoot {
		if r.Intent == IntentAscend {
			t.Fatal("root listing (parent == dir) must not contain a '..' row")
		}
	}
	notRoot := InjectNav("/home", []Row{}, Nav{Parent: "/"})
	var sawAscend bool
	for _, r := range notRoot {
		if r.Intent == IntentAscend && r.Token == "/" {
			sawAscend = true
		}
	}
	if !sawAscend {
		t.Fatal("non-root listing must contain a '..' row pointing at the parent")
	}
}

// TestMatcherRelevanceGateIsShared pins the per-rune relevance floor both apps'
// list filters relied on, so a scattered subsequence is rejected.
func TestMatcherRelevanceGateIsShared(t *testing.T) {
	if _, ok := (Fuzzy{}).Match("redis", "internet midwest wifi password"); ok {
		t.Fatal("scattered subsequence must be rejected by the shared relevance gate")
	}
	if _, ok := (Fuzzy{}).Match("gh", "work/github"); !ok {
		t.Fatal("a boundary match must pass the relevance gate")
	}
}
