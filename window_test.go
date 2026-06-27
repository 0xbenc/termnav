package termnav

import "testing"

// noGroups is a groupAt callback for an ungrouped list of n rows.
func noGroups(n int) func(i int) (string, bool) {
	return func(i int) (string, bool) {
		if i < 0 || i >= n {
			return "", false
		}
		return "", true
	}
}

func TestWindowContainsCursor(t *testing.T) {
	n := 20
	// budget 5, scrolled to top: cursor 0..3 fit (one row reserved for "N more below")
	if !WindowContainsCursor(n, 0, 0, 5, noGroups(n)) {
		t.Fatal("cursor 0 should fit at top")
	}
	if WindowContainsCursor(n, 0, 19, 5, noGroups(n)) {
		t.Fatal("cursor 19 cannot fit in a top window of budget 5")
	}
}

func TestClampWindowAdvancesScroll(t *testing.T) {
	n := 20
	contains := func(start, cursor int) bool {
		return WindowContainsCursor(n, start, cursor, 5, noGroups(n))
	}
	cur, scroll := ClampWindow(n, 19, 0, contains)
	if cur != 19 {
		t.Fatalf("cursor = %d, want 19", cur)
	}
	if scroll == 0 || !contains(scroll, 19) {
		t.Fatalf("scroll = %d did not advance to make cursor 19 visible", scroll)
	}
}

func TestClampWindowEmpty(t *testing.T) {
	cur, scroll := ClampWindow(0, 5, 5, func(int, int) bool { return true })
	if cur != 0 || scroll != 0 {
		t.Fatalf("empty clamp = (%d,%d), want (0,0)", cur, scroll)
	}
}

func TestSnapSkipsReferenceRows(t *testing.T) {
	// rows: selectable at 0 and 3, reference at 1,2
	selectable := func(i int) bool { return i == 0 || i == 3 }
	if got := Snap(4, 1, 1, selectable); got != 3 {
		t.Fatalf("Snap forward from 1 = %d, want 3", got)
	}
	if got := Snap(4, 2, -1, selectable); got != 0 {
		t.Fatalf("Snap backward from 2 = %d, want 0", got)
	}
	if got := Snap(4, 1, 1, func(int) bool { return false }); got != -1 {
		t.Fatalf("Snap with nothing selectable = %d, want -1", got)
	}
}

func TestJumpSection(t *testing.T) {
	// groups: A A A B B C
	groups := []string{"A", "A", "A", "B", "B", "C"}
	groupAt := func(i int) string {
		if i < 0 || i >= len(groups) {
			return ""
		}
		return groups[i]
	}
	// from index 0 (group A), forward -> first B at index 3
	if got := JumpSection(len(groups), 0, 1, groupAt); got != 3 {
		t.Fatalf("jump forward from 0 = %d, want 3", got)
	}
	// from index 4 (group B), backward -> start of B at index 3
	if got := JumpSection(len(groups), 4, -1, groupAt); got != 3 {
		t.Fatalf("jump backward from 4 = %d, want 3 (start of current group)", got)
	}
}
