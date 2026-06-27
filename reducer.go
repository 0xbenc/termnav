package termnav

// reducer.go is the pure, IO-free heart of termnav: a value-returning state
// machine, Update(Model, Event) -> (Model, []Effect), with NO bubbletea and NO
// os/net import. It owns the must-agree navigation logic both apps duplicated —
// the filter pass, cursor-snap over reference rows, the group-aware scroll
// window (lifted from ssherpa's chrome.ListView), and a first-class async
// Loading/Ready/Error/Empty state machine with a generation token so a rapid cd
// drops stale listings and a slow host is abortable. The only place IO happens
// is the teax adapter, which runs the returned Effects inside a tea.Cmd and
// feeds results back as Events. Every transition here is unit-testable with no
// PTY and no filesystem.

// Validator is an optional post-select gate: given the row the user committed,
// it returns ok=false plus a notice to block the commit (e.g. passage's
// classifyLeaf overwrite gate), or ok=true to commit. A nil Validator always
// commits.
type Validator func(r Row) (ok bool, notice string)

// Options configures a browse. The zero value is a usable single-pane fuzzy
// browser; every field has a sane default.
type Options struct {
	// Matcher filters rows against the query. Default Fuzzy{} (fzf scoring with
	// the per-rune relevance gate) — the behavior both apps' list filters used.
	Matcher Matcher
	// MatchText returns the string a row is filtered against. Default Row.Title;
	// ssherpa joins Title/Description/Detail/Token/Group/Badge.
	MatchText func(Row) string
	// Validate gates a commit (Use/Leaf). nil means always commit.
	Validate Validator
	// ReserveRows is the chrome line count subtracted from height to get the
	// list viewport budget (title/steps/footer/meta). The app sets it to match
	// its frame so the scroll window stays byte-identical.
	ReserveRows int
	// MinRows floors the viewport budget. Default 1 (passage); ssherpa uses 4.
	MinRows int
	// KeepCursorOnFilter keeps the cursor index across a filter change (clamped),
	// matching ssherpa; the default snaps the cursor to the first selectable row,
	// matching passage.
	KeepCursorOnFilter bool
	// Start is the initial path passed to FileSource.Resolve.
	Start string
}

// Model is the pure browse state. It is a value type — no tea, no IO. Render
// reads it through the accessors at the bottom of this file.
type Model struct {
	opts Options

	cwd      string
	rows     []Row // assembled listing rows for the current directory
	prevRows []Row // retained during Loading for a dimmed last-frame render
	filtered []int // indices into rows; recomputed by applyFilter
	cursor   int   // index into filtered
	scroll   int   // start index into filtered (window top)
	query    string

	state  LoadState
	gen    uint64
	notice string

	width  int
	height int

	result   *Outcome
	canceled bool
}

// New returns an Idle model. Call Load (typically from the adapter's Init, after
// FileSource.Resolve) to request the first directory.
func New(opts Options) Model {
	if opts.MinRows <= 0 {
		opts.MinRows = 1
	}
	return Model{opts: opts, state: Idle, width: 80, height: 24}
}

// Load starts navigating to dir: it bumps the generation, enters Loading
// (retaining the prior rows for a dimmed frame), and asks the adapter to cancel
// any in-flight listing and fetch the new one. cwd is updated when the listing
// actually arrives (it is canonicalized by the source).
func (m Model) Load(dir string) (Model, []Effect) {
	old := m.gen
	m.gen++
	m.prevRows = m.rows
	m.state = Loading
	m.notice = ""
	effects := []Effect{}
	if old != 0 {
		effects = append(effects, CancelListEffect{Gen: old})
	}
	effects = append(effects, ListDirEffect{Gen: m.gen, Dir: dir})
	return m, effects
}

// Update advances the state machine. It returns the new model and any Effects
// the adapter must run (list a directory, cancel an in-flight list, close the
// source). Render then reads the model; the adapter quits when Done reports true.
func Update(m Model, ev Event) (Model, []Effect) {
	switch e := ev.(type) {
	case ResizeEvent:
		if e.W > 0 {
			m.width = e.W
		}
		if e.H > 0 {
			m.height = e.H
		}
		m.clampWindow()
		return m, nil
	case ListLoadedEvent:
		return m.onListLoaded(e)
	case KeyEvent:
		return m.onKey(e)
	default:
		return m, nil
	}
}

func (m Model) onListLoaded(e ListLoadedEvent) (Model, []Effect) {
	if e.Gen != m.gen {
		return m, nil // stale: a newer navigation supersedes this listing
	}
	if e.Err != nil {
		m.state = Error
		m.notice = e.Err.Error()
		// keep prevRows visible (dimmed) so an error mid-navigation is recoverable
		return m, nil
	}
	m.cwd = e.Listing.Dir
	m.rows = e.Listing.Rows
	m.prevRows = nil
	m.notice = e.Listing.Notice
	m.query = ""
	m.cursor = 0
	m.scroll = 0
	m.applyFilter()
	if len(m.rows) == 0 {
		m.state = Empty
	} else {
		m.state = Ready
	}
	return m, nil
}

func (m Model) onKey(e KeyEvent) (Model, []Effect) {
	// Pure text input: extend the filter query.
	if e.Key == "" {
		if e.Text != "" {
			m.query += e.Text
			m.applyFilter()
		}
		return m, nil
	}
	switch e.Key {
	case "cancel":
		m.canceled = true
		return m, nil
	case "backspace":
		if m.query != "" {
			m.query = m.query[:len(m.query)-1]
			m.applyFilter()
		}
		return m, nil
	case "up":
		m.move(-1)
	case "down":
		m.move(1)
	case "pgup":
		m.move(-m.page())
	case "pgdown":
		m.move(m.page())
	case "home":
		if s := Snap(len(m.filtered), 0, 1, m.selectable); s >= 0 {
			m.cursor = s
		}
		m.clampWindow()
	case "end":
		if s := Snap(len(m.filtered), len(m.filtered)-1, -1, m.selectable); s >= 0 {
			m.cursor = s
		}
		m.clampWindow()
	case "section-up":
		m.jumpSection(-1)
	case "section-down":
		m.jumpSection(1)
	case "enter":
		return m.commit()
	}
	return m, nil
}

// commit acts on the focused row's intent: descend/ascend navigate (returning
// list effects); use/leaf run the Validate gate and either set the Outcome or a
// notice; a reference row is a no-op (Snap never rests on one).
func (m Model) commit() (Model, []Effect) {
	r, ok := m.focused()
	if !ok {
		return m, nil
	}
	switch r.Intent {
	case IntentDescend, IntentAscend:
		return m.Load(r.Token)
	case IntentUseContainer, IntentSelectLeaf:
		if m.opts.Validate != nil {
			if okCommit, notice := m.opts.Validate(r); !okCommit {
				m.notice = notice
				return m, nil
			}
		}
		out := Outcome{Rows: []Row{r}, Intent: r.Intent}
		m.result = &out
		return m, nil
	default:
		return m, nil
	}
}

func (m *Model) applyFilter() {
	matcher := m.matcher()
	prevCursor := m.cursor
	m.filtered = m.filtered[:0]
	for i, r := range m.rows {
		if _, ok := matcher.Match(m.query, m.matchText(r)); ok {
			m.filtered = append(m.filtered, i)
		}
	}
	if m.opts.KeepCursorOnFilter {
		if prevCursor >= len(m.filtered) {
			prevCursor = maxInt(0, len(m.filtered)-1)
		}
		m.cursor = prevCursor
		// keep the cursor on a selectable row if the current one became reference
		if !m.selectable(m.cursor) {
			if s := Snap(len(m.filtered), m.cursor, 1, m.selectable); s >= 0 {
				m.cursor = s
			}
		}
	} else {
		if s := Snap(len(m.filtered), 0, 1, m.selectable); s >= 0 {
			m.cursor = s
		} else {
			m.cursor = 0
		}
	}
	m.clampWindow()
}

func (m *Model) move(delta int) {
	if len(m.filtered) == 0 || delta == 0 {
		return
	}
	dir := 1
	if delta < 0 {
		dir = -1
	}
	if s := Snap(len(m.filtered), m.cursor+delta, dir, m.selectable); s >= 0 {
		m.cursor = s
	}
	m.clampWindow()
}

func (m *Model) jumpSection(delta int) {
	if next := JumpSection(len(m.filtered), m.cursor, delta, m.groupAtString); next != m.cursor {
		// land on a selectable row at/after the section boundary
		if s := Snap(len(m.filtered), next, delta, m.selectable); s >= 0 {
			m.cursor = s
		} else {
			m.cursor = next
		}
		m.clampWindow()
	}
}

func (m *Model) clampWindow() {
	budget := m.Budget()
	contains := func(start, cursor int) bool {
		return WindowContainsCursor(len(m.filtered), start, cursor, budget, m.groupAtOK)
	}
	m.cursor, m.scroll = ClampWindow(len(m.filtered), m.cursor, m.scroll, contains)
}

// --- helpers ---

func (m Model) matcher() Matcher {
	if m.opts.Matcher != nil {
		return m.opts.Matcher
	}
	return Fuzzy{}
}

func (m Model) matchText(r Row) string {
	if m.opts.MatchText != nil {
		return m.opts.MatchText(r)
	}
	return r.Title
}

// selectable reports whether filtered row i may hold the cursor.
func (m Model) selectable(i int) bool {
	if i < 0 || i >= len(m.filtered) {
		return false
	}
	idx := m.filtered[i]
	if idx < 0 || idx >= len(m.rows) {
		return false
	}
	return m.rows[idx].Selectable
}

func (m Model) groupAtString(i int) string {
	g, _ := m.groupAtOK(i)
	return g
}

func (m Model) groupAtOK(i int) (string, bool) {
	if i < 0 || i >= len(m.filtered) {
		return "", false
	}
	idx := m.filtered[i]
	if idx < 0 || idx >= len(m.rows) {
		return "", false
	}
	return m.rows[idx].Group, true
}

func (m Model) focused() (Row, bool) {
	if m.cursor < 0 || m.cursor >= len(m.filtered) {
		return Row{}, false
	}
	idx := m.filtered[m.cursor]
	if idx < 0 || idx >= len(m.rows) {
		return Row{}, false
	}
	return m.rows[idx], true
}

func (m Model) page() int {
	p := m.Budget() / 2
	if p < 1 {
		p = 1
	}
	return p
}

// --- accessors (read-only views for the adapter and render) ---

// Budget is the list viewport body-line budget: height minus the app's reserved
// chrome rows, floored at MinRows.
func (m Model) Budget() int {
	b := m.height - m.opts.ReserveRows
	if b < m.opts.MinRows {
		b = m.opts.MinRows
	}
	return b
}

// Outcome returns the committed selection, ok=true once a leaf or container has
// been chosen. ok=false while still navigating.
func (m Model) Outcome() (Outcome, bool) {
	if m.result == nil {
		return Outcome{}, false
	}
	return *m.result, true
}

// Done reports whether the browse has terminated — either a commit or a cancel.
func (m Model) Done() bool { return m.result != nil || m.canceled }

// Canceled reports whether the user aborted (esc / the app's cancel key).
func (m Model) Canceled() bool { return m.canceled }

// State is the async load state of the current directory.
func (m Model) State() LoadState { return m.state }

// Gen is the current navigation generation. The adapter tags each List with it
// so a stale result (from a superseded directory) is dropped.
func (m Model) Gen() uint64 { return m.gen }

// Loading reports whether a listing is in flight.
func (m Model) Loading() bool { return m.state == Loading }

// Cwd is the canonical current directory (set when a listing arrives).
func (m Model) Cwd() string { return m.cwd }

// Query is the current filter text.
func (m Model) Query() string { return m.query }

// Notice is the current inline message (error / permission / blocked commit), or "".
func (m Model) Notice() string { return m.notice }

// Width and Height are the last known terminal size.
func (m Model) Width() int  { return m.width }
func (m Model) Height() int { return m.height }

// Rows returns the assembled rows of the current directory (including the
// synthetic use/.. rows the source injected). Render must not mutate it.
func (m Model) Rows() []Row { return m.rows }

// PrevRows returns the prior directory's rows, retained during Loading so a
// render can dim the last frame instead of blanking. nil once a listing lands.
func (m Model) PrevRows() []Row { return m.prevRows }

// Filtered returns the indices into Rows that pass the current query, in order.
func (m Model) Filtered() []int { return m.filtered }

// Cursor is the index into Filtered the user is on.
func (m Model) Cursor() int { return m.cursor }

// Scroll is the Filtered index at the top of the viewport.
func (m Model) Scroll() int { return m.scroll }

// FocusedRow returns the row under the cursor, ok=false when the list is empty.
func (m Model) FocusedRow() (Row, bool) { return m.focused() }

// MatchTextOf exposes the configured match string for a row (for a render that
// wants to recompute highlight positions). Falls back to Row.Title.
func (m Model) MatchTextOf(r Row) string { return m.matchText(r) }

// GroupAt returns the group label of filtered row i and ok=false for an
// out-of-range index — the exact callback shape the windowing primitives and an
// app's group-header render loop both consume.
func (m Model) GroupAt(i int) (string, bool) { return m.groupAtOK(i) }
