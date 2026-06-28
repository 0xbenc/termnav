// Package render is termnav's themed default renderer, layered over termtheme.
// It is the ONLY layer that imports termtheme, and it imports no Bubble Tea, so
// it produces a plain frame string the adapter (or an app) wraps in a tea.View.
// Color and behavior key off the canonical NavIntent via a Styler — NEVER off an
// app's private Kind literal — and every Provider-derived name is routed through
// termtheme.Sanitize at the render boundary so a hostile remote filename can
// never emit ESC/C1/BEL. Apps with existing golden frames keep their own render
// during migration; this is the turnkey look for new surfaces.
package render

import (
	"fmt"
	"strings"

	"github.com/0xbenc/termnav"
	"github.com/0xbenc/termtheme"
)

// Styler maps a canonical NavIntent (plus selection state) to a termtheme Role,
// and supplies a row's badge text. The default covers both apps' conventions.
type Styler interface {
	Role(in termnav.NavIntent, selected bool) termtheme.Role
	Badge(r termnav.Row) string
}

// DefaultStyler is the shared intent->role mapping: descend/leaf read as
// primary, a "use this folder" as accent, ".." as muted, a reference row as
// subtle, and any selected row as the selection role.
type DefaultStyler struct{}

func (DefaultStyler) Role(in termnav.NavIntent, selected bool) termtheme.Role {
	if selected {
		return termtheme.RoleSelected
	}
	switch in {
	case termnav.IntentUseContainer:
		return termtheme.RoleAccent
	case termnav.IntentAscend:
		return termtheme.RoleMuted
	case termnav.IntentReference:
		return termtheme.RoleSubtle
	default:
		return termtheme.RolePrimary
	}
}

func (DefaultStyler) Badge(r termnav.Row) string {
	if r.Badge == "" {
		return ""
	}
	return "[" + strings.ToUpper(r.Badge) + "]"
}

// Chrome is the optional frame decoration around the list.
type Chrome struct {
	Title         string
	LocationLabel string // e.g. "PATH"; defaults to "PATH"
	Footer        string
}

// Render produces the full frame for a browse model: title, location, filter,
// the windowed list (with group headers, overflow markers, and match
// highlighting), and a footer — honoring the Loading/Empty/Error states.
func Render(m termnav.Model, th termtheme.Theme, st Styler, ch Chrome) string {
	if st == nil {
		st = DefaultStyler{}
	}
	width := m.Width()
	if width < 24 {
		width = 24
	}
	inner := width - 2

	var b strings.Builder
	writeLine := func(s string) { b.WriteString(termtheme.Truncate(s, inner)); b.WriteByte('\n') }

	if ch.Title != "" {
		writeLine(th.Style(termtheme.RoleTitle, strings.ToUpper(ch.Title)))
	}
	label := ch.LocationLabel
	if label == "" {
		label = "PATH"
	}
	loc := m.Cwd()
	if loc == "" {
		loc = "."
	}
	writeLine(th.Style(termtheme.RoleMuted, label+"  ") + termtheme.Sanitize(loc))

	// Filter line + counter.
	query := termtheme.Sanitize(m.Query())
	if query == "" {
		query = th.Style(termtheme.RoleMuted, "type to filter")
	} else {
		query = th.Style(termtheme.RoleSearch, query)
	}
	counter := th.Style(termtheme.RoleMuted, fmt.Sprintf("%d/%d", len(m.Filtered()), len(m.Rows())))
	writeLine("/" + query + "  " + counter)
	b.WriteByte('\n')

	switch m.State() {
	case termnav.Loading:
		writeLine(th.Style(termtheme.RoleMuted, "loading…"))
	case termnav.Error:
		writeLine(th.Style(termtheme.RoleDanger, "error: "+termtheme.Sanitize(m.Notice())))
	case termnav.Empty:
		writeLine(th.Style(termtheme.RoleWarning, "(empty)"))
	default:
		for _, line := range listLines(m, th, st, inner) {
			writeLine(line)
		}
	}

	if n := m.Notice(); n != "" && m.State() != termnav.Error {
		writeLine(th.Style(termtheme.RoleWarning, termtheme.Sanitize(n)))
	}
	if ch.Footer != "" {
		b.WriteByte('\n')
		writeLine(th.Style(termtheme.RoleMuted, ch.Footer))
	}
	return strings.TrimRight(b.String(), "\n")
}

// listLines renders the visible window of the filtered list with group headers
// and overflow markers, reusing the model's scroll/cursor/group state so the
// window matches the windowing primitives exactly.
func listLines(m termnav.Model, th termtheme.Theme, st Styler, width int) []string {
	filtered := m.Filtered()
	rows := m.Rows()
	if len(filtered) == 0 {
		return []string{th.Style(termtheme.RoleWarning, "No matches")}
	}
	budget := m.Budget()
	start := m.Scroll()
	if start < 0 {
		start = 0
	}

	var lines []string
	if start > 0 {
		lines = append(lines, th.Style(termtheme.RoleMuted, fmt.Sprintf("  ↑ %d more", start)))
	}
	lastGroup := ""
	rendered := 0
	last := start
	for i := start; i < len(filtered); i++ {
		idx := filtered[i]
		if idx < 0 || idx >= len(rows) {
			continue
		}
		row := rows[idx]
		newGroup := row.Group != "" && row.Group != lastGroup
		cost := 0
		if newGroup {
			cost = 1
			if rendered > 0 {
				cost++
			}
		}
		reserve := 0
		if len(filtered)-i-1 > 0 {
			reserve = 1
		}
		if len(lines)+cost+1+reserve > budget {
			break
		}
		if newGroup {
			if rendered > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, th.Style(termtheme.RoleSecondary, row.Group))
			lastGroup = row.Group
		}
		lines = append(lines, rowLine(row, i == m.Cursor(), th, st, width))
		rendered++
		last = i + 1
	}
	if last < len(filtered) {
		lines = append(lines, th.Style(termtheme.RoleMuted, fmt.Sprintf("  ↓ %d more", len(filtered)-last)))
	}
	return lines
}

func rowLine(row termnav.Row, selected bool, th termtheme.Theme, st Styler, width int) string {
	cursor := "  "
	if selected {
		cursor = "> "
	}
	badge := st.Badge(row)
	prefix := cursor
	if badge != "" {
		prefix += termtheme.PadRight(badge, 7) + " "
	}
	title := termtheme.Sanitize(row.Title)
	role := st.Role(row.Intent, selected)
	body := termtheme.Truncate(prefix+title, width)
	if selected {
		body = termtheme.PadRight(body, width)
	}
	return th.Style(role, body)
}

// HighlightMatches cell-truncates display to width and styles it: runes at the
// given match positions are wrapped with hl(), the rest with base(). If
// truncation appended termtheme's "~" overflow marker, it is re-emitted via
// base() so the marker is never highlighted. base/hl let each app supply its own
// role styling (e.g. selected vs unselected row). Lifted verbatim from the
// byte-identical copies passage and ssherpa each carried.
func HighlightMatches(display string, positions []int, width int, base, hl func(string) string) string {
	truncated := termtheme.Truncate(display, width)
	if len(positions) == 0 {
		return base(truncated)
	}
	keptStr := truncated
	hasMarker := false
	if termtheme.VisibleWidth(display) > width && strings.HasSuffix(truncated, "~") {
		keptStr = strings.TrimSuffix(truncated, "~")
		hasMarker = true
	}
	runes := []rune(keptStr)
	matched := make([]bool, len(runes))
	for _, p := range positions {
		if p >= 0 && p < len(runes) {
			matched[p] = true
		}
	}
	var b strings.Builder
	for i := 0; i < len(runes); {
		j := i
		for j < len(runes) && matched[j] == matched[i] {
			j++
		}
		seg := string(runes[i:j])
		if matched[i] {
			b.WriteString(hl(seg))
		} else {
			b.WriteString(base(seg))
		}
		i = j
	}
	if hasMarker {
		b.WriteString(base("~"))
	}
	return b.String()
}
