package render_test

import (
	"strings"
	"testing"

	"github.com/0xbenc/termnav"
	"github.com/0xbenc/termnav/render"
	"github.com/0xbenc/termtheme"
)

// readyModel drives a model into the Ready state with the given rows at an
// 80x24 size, using only the public termnav API.
func readyModel(rows ...termnav.Row) termnav.Model {
	m := termnav.New(termnav.Options{ReserveRows: 6})
	m, _ = m.Load("/x")
	m, _ = termnav.Update(m, termnav.ResizeEvent{W: 80, H: 24})
	m, _ = termnav.Update(m, termnav.ListLoadedEvent{Gen: 1, Listing: termnav.Listing{
		Dir: "/x", Parent: "/", Rows: rows,
	}})
	return m
}

func TestRenderBasics(t *testing.T) {
	m := readyModel(
		termnav.Row{Token: "/x/sub", Title: "sub/", Intent: termnav.IntentDescend, Selectable: true, IsContainer: true, Badge: "dir"},
		termnav.Row{Token: "/x/f", Title: "readme", Intent: termnav.IntentSelectLeaf, Selectable: true, Badge: "file"},
	)
	out := render.Render(m, termtheme.Theme{NoColor: true}, nil, render.Chrome{Title: "pick a file", Footer: "esc cancel"})
	for _, want := range []string{"PICK A FILE", "/x", "sub/", "readme", "esc cancel", "2/2"} {
		if !strings.Contains(out, want) {
			t.Errorf("render output missing %q\n---\n%s", want, out)
		}
	}
}

func TestRenderSanitizesHostileNames(t *testing.T) {
	// A remote filename carrying an escape sequence must never reach the terminal.
	hostile := "evil\x1b[31mname\x07"
	m := readyModel(termnav.Row{Token: "/x/e", Title: hostile, Intent: termnav.IntentSelectLeaf, Selectable: true})
	out := render.Render(m, termtheme.Theme{NoColor: true}, nil, render.Chrome{Title: "t"})
	if strings.ContainsAny(out, "\x1b\x07") {
		t.Fatalf("render leaked a control/escape byte from a hostile name:\n%q", out)
	}
	if !strings.Contains(out, "evil") || !strings.Contains(out, "name") {
		t.Fatalf("sanitized name lost its printable text:\n%s", out)
	}
}

func TestRenderLoadingAndEmpty(t *testing.T) {
	m := termnav.New(termnav.Options{})
	m, _ = m.Load("/x")
	m, _ = termnav.Update(m, termnav.ResizeEvent{W: 80, H: 24})
	if out := render.Render(m, termtheme.Theme{NoColor: true}, nil, render.Chrome{}); !strings.Contains(out, "loading") {
		t.Errorf("loading state should render a loading line:\n%s", out)
	}
	m, _ = termnav.Update(m, termnav.ListLoadedEvent{Gen: 1, Listing: termnav.Listing{Dir: "/x"}})
	if out := render.Render(m, termtheme.Theme{NoColor: true}, nil, render.Chrome{}); !strings.Contains(out, "empty") {
		t.Errorf("empty state should render an empty line:\n%s", out)
	}
}

func TestHighlightMatches(t *testing.T) {
	base := func(s string) string { return "[" + s + "]" }
	hl := func(s string) string { return "<" + s + ">" }
	cases := []struct {
		name      string
		display   string
		positions []int
		width     int
		want      string
	}{
		{"no positions", "hello", nil, 40, "[hello]"},
		{"all matched contiguous", "db", []int{0, 1}, 40, "<db>"},
		{"mixed segments", "abc", []int{1}, 40, "[a]<b>[c]"},
		{"truncation marker uses base, not hl", "abcdefghij", []int{0}, 4, "<a>[bc][~]"},
		{"positions beyond kept range ignored", "abcdefghij", []int{9}, 4, "[abc][~]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := render.HighlightMatches(tc.display, tc.positions, tc.width, base, hl); got != tc.want {
				t.Errorf("HighlightMatches(%q,%v,%d) = %q, want %q", tc.display, tc.positions, tc.width, got, tc.want)
			}
		})
	}
}
