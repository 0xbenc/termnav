package termnav

// Event is an input to the reducer. The teax adapter translates each tea.Msg
// into one of these; the pure core never sees a bubbletea type. The sealed
// isEvent marker keeps the set closed so Update's type switch is exhaustive.
type Event interface{ isEvent() }

// KeyEvent is a normalized keypress. Key is a SEMANTIC action the per-app KeyMap
// resolved to — "up", "down", "pgup", "pgdown", "home", "end", "section-up",
// "section-down", "enter", "backspace", "cancel" — or "" when the press is pure
// text input, in which case Text holds the printable runes to append to the
// filter. Separating semantics (here) from raw keys (the KeyMap) is what lets
// ssherpa's Q=cancel and passage's ctrl+q=cancel coexist without a core change.
type KeyEvent struct {
	Key  string
	Text string
}

// ListLoadedEvent delivers the result of a FileSource.List the adapter ran for
// generation Gen. A Gen that no longer matches the model is dropped as stale, so
// a rapid sequence of cd's always settles on the last one. Err non-nil puts the
// model in the Error state with the message as its notice.
type ListLoadedEvent struct {
	Gen     uint64
	Listing Listing
	Err     error
}

// ResizeEvent carries a new terminal size.
type ResizeEvent struct{ W, H int }

func (KeyEvent) isEvent()        {}
func (ListLoadedEvent) isEvent() {}
func (ResizeEvent) isEvent()     {}

// Effect is a side effect the reducer asks the adapter to perform. The core
// produces these as plain values and never executes them — the adapter runs
// each inside a tea.Cmd (the only place IO happens), then feeds results back as
// Events. The sealed isEffect marker keeps the set closed.
type Effect interface{ isEffect() }

// ListDirEffect asks the adapter to run FileSource.List(Dir) and report the
// result as a ListLoadedEvent tagged with Gen. Sync hints that a SyncLister may
// serve it inline (no Loading flash) — the adapter decides.
type ListDirEffect struct {
	Gen  uint64
	Dir  string
	Sync bool
}

// CancelListEffect asks the adapter to cancel the in-flight listing for an older
// generation (cancel its context, killing a slow sftp subprocess), so a fast
// host never blocks navigation away.
type CancelListEffect struct{ Gen uint64 }

// CloseEffect asks the adapter to Close a stateful FileSource (SFTP session
// teardown) when the browse ends.
type CloseEffect struct{}

func (ListDirEffect) isEffect()    {}
func (CancelListEffect) isEffect() {}
func (CloseEffect) isEffect()      {}
