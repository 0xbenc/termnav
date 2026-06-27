package termnav

// Row is one line in a navigation list: a child container to descend into, the
// parent (".."), a synthetic "use this folder", a selectable leaf, or a
// reference-only row. It is the shared currency that replaces passage's
// DirEntry{Title,Path,Kind} and ssherpa's Item{Kind,Token,...}. It is a
// superset (mirroring termtheme's universal role superset): passage populates a
// handful of fields, ssherpa fills the rich set, and unused fields render to
// nothing rather than erroring (forward-compat).
type Row struct {
	Token       string // navigable / return value: an absolute path or a pass path
	Title       string // display text
	Description string // secondary text (size, mtime, full path)
	Detail      string // tertiary text / canonical path for preview
	Group       string // grouping label ("Directories"/"Files"/...); "" disables grouping
	Badge       string // short tag ("dir","file","up","use"); cosmetic only
	Intent      NavIntent
	Selectable  bool  // may the cursor rest on and Enter commit this row
	IsContainer bool  // folder-ness, independent of Intent (a dual node is folder AND entry)
	Positions   []int // match-highlight rune indices into Title
	IsSymlink   bool
	LinkTarget  string
	// Kind preserves the app's private literal (passage "dir"/"file"/...,
	// ssherpa "file_dir"/"remote_file"/...) so a value round-trips losslessly
	// through the core. Theme/behavior NEVER key off Kind — only off Intent.
	Kind string
	// Meta carries an opaque app payload (e.g. os.FileInfo, a remote entry) the
	// app can recover after selection without the core understanding it.
	Meta any
}

// NavIntent is the canonical, app-independent meaning of a row. Every app's
// private Kind vocabulary folds into this 5-value superset; render and behavior
// switch on Intent ONLY, never on Kind. An unknown future intent tolerates and
// renders as a plain row, exactly as termtheme tolerates an unknown role.
type NavIntent uint8

const (
	IntentDescend      NavIntent = iota // open a child container (a directory)
	IntentAscend                        // go to the parent ("..")
	IntentUseContainer                  // commit the current folder (synthetic "Use this folder")
	IntentSelectLeaf                    // commit a leaf (a file / namespace entry)
	IntentReference                     // shown, dimmed, NOT selectable; the cursor skips it
)

// String renders an intent for diagnostics and logs (not for the UI).
func (in NavIntent) String() string {
	switch in {
	case IntentDescend:
		return "descend"
	case IntentAscend:
		return "ascend"
	case IntentUseContainer:
		return "use"
	case IntentSelectLeaf:
		return "leaf"
	case IntentReference:
		return "reference"
	default:
		return "unknown"
	}
}

// Listing is one directory's contents as returned by a FileSource. It is the
// only thing a transport must produce: the canonical resolved directory, its
// parent (for the ".." row; "" or == Dir means "at the root, suppress .."), the
// child rows, and an optional inline, recoverable notice (permission denied,
// partial listing) that is surfaced without aborting navigation.
type Listing struct {
	Dir    string
	Parent string
	Rows   []Row
	Notice string
}

// LoadState is the async lifecycle of the current directory, driving uniform
// spinner / error / empty feedback for every transport (instant local read,
// blocking SFTP round-trip, zero-IO in-memory tree).
type LoadState uint8

const (
	Idle    LoadState = iota // nothing requested yet
	Loading                  // a List is in flight
	Ready                    // a listing is shown
	Error                    // the last List failed (Notice holds why)
	Empty                    // the listing succeeded but has no rows
)

func (s LoadState) String() string {
	switch s {
	case Idle:
		return "idle"
	case Loading:
		return "loading"
	case Ready:
		return "ready"
	case Error:
		return "error"
	case Empty:
		return "empty"
	default:
		return "unknown"
	}
}

// Outcome is the terminal result of a browse: the committed row(s) and the
// intent that committed them, so the caller learns whether a leaf or a
// container was chosen without switching on Kind strings. Rows holds exactly
// one element for single-select; the slice shape is forward-compat for an
// opt-in MultiSelect.
type Outcome struct {
	Rows   []Row
	Intent NavIntent
	Notice string
}

// Token returns the chosen row's token (the navigable/return value), or "" when
// the outcome is empty.
func (o Outcome) Token() string {
	if len(o.Rows) == 0 {
		return ""
	}
	return o.Rows[0].Token
}

// Tokens returns every chosen row's token (one element for single-select).
func (o Outcome) Tokens() []string {
	out := make([]string, 0, len(o.Rows))
	for _, r := range o.Rows {
		out = append(out, r.Token)
	}
	return out
}
