package termnav

import (
	"reflect"
	"testing"
)

func TestMatchFuzzyBasics(t *testing.T) {
	if _, ok := MatchFuzzy("", "anything"); !ok {
		t.Fatal("empty query should match")
	}
	r, ok := MatchFuzzy("gh", "github")
	if !ok || len(r.Positions) != 2 || r.Positions[0] != 0 {
		t.Fatalf("gh/github = %#v ok=%v, want leading match", r, ok)
	}
	if _, ok := MatchFuzzy("xyz", "github"); ok {
		t.Fatal("xyz should not match github")
	}
}

func TestRelevanceGate(t *testing.T) {
	// A boundary match clears the gate; a scattered one does not.
	boundary, ok := MatchFuzzy("gh", "work/github")
	if !ok || !Relevant(boundary, 2) {
		t.Fatalf("boundary match should be relevant: %#v", boundary)
	}
	// Fuzzy{} folds Match + Relevant into one call.
	if _, ok := (Fuzzy{}).Match("redis", "internet midwest wifi password"); ok {
		t.Fatal("scattered subsequence should be rejected by the relevance gate")
	}
}

func TestSubstringMatcher(t *testing.T) {
	if _, ok := (Substring{}).Match("AN", "banana"); !ok {
		t.Fatal("substring should be case-insensitive")
	}
	if _, ok := (Substring{}).Match("zz", "banana"); ok {
		t.Fatal("zz is not a substring of banana")
	}
	if _, ok := (Substring{}).Match("", "x"); !ok {
		t.Fatal("empty query matches")
	}
}

func TestFuzzyPositionsAreRuneIndices(t *testing.T) {
	// Highlight positions must be rune indices (not byte offsets) for width-aware
	// rendering after truncation.
	r, ok := MatchFuzzy("é", "café")
	if !ok {
		t.Fatal("é should match café")
	}
	if !reflect.DeepEqual(r.Positions, []int{3}) {
		t.Fatalf("positions = %#v, want rune index [3]", r.Positions)
	}
}
