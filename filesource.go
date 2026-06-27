package termnav

import "context"

// FileSource is THE injected divergence seam — the direct analogue of
// termtheme's ThemeConfig.Resolve(base): just as termtheme ships no palettes and
// the host app supplies the base, termnav ships no transport and the host app
// supplies the lister. "List one directory" granularity fits a real-FS level,
// one SFTP round-trip, and one node of an in-memory tree alike, giving every
// transport a uniform async Loading/Error/Empty contract. The pure core NEVER
// calls List — only the teax adapter does, inside a tea.Cmd — so the kernel
// imports neither os nor net and stays PTY-free testable.
type FileSource interface {
	// Resolve normalizes the opening path once (~ -> abs, Clean, or the
	// namespace root) and returns the canonical starting directory.
	Resolve(ctx context.Context, path string) (start string, err error)
	// List returns ONE directory's children. It is the only IO method.
	List(ctx context.Context, dir string) (Listing, error)
}

// Closer is an optional capability (detected by type assertion): a stateful
// source — e.g. an SFTP session multiplexing over one authed socket — tears
// down here. Sources without session state simply omit it.
type Closer interface {
	Close() error
}

// SyncLister is an optional capability for zero-latency transports (local FS,
// in-memory tree): when present, the adapter lists inline and skips the Loading
// state entirely, so an instant read never flashes a spinner.
type SyncLister interface {
	ListSync(dir string) (Listing, error)
}

// Indexer is an optional capability exposing a navigation Index for breadcrumb
// and shell-TAB completion. A TreeSource exposes its whole index; a real-FS or
// SFTP source may expose a growing index merged as directories are visited.
type Indexer interface {
	Index() *Index
}

// Nav describes the synthetic navigation rows a source injects ahead of a
// directory's real children: an optional "Use this folder" row (to commit the
// container itself) and the ".." parent row (suppressed at the root). It is the
// small structural piece every transport shares; cosmetic fields stay
// app-supplied so grouping/badges match each app's existing frames.
type Nav struct {
	Parent   string // canonical parent dir; "" or == dir suppresses ".."
	UseRow   bool   // inject "Use this folder"
	UseTitle string // default "Use this folder"
	UseGroup string
	UseBadge string
	UseKind  string // app's private Kind literal for the use row
	UpTitle  string // default ".."
	UpGroup  string
	UpBadge  string
	UpKind   string // app's private Kind literal for the ".." row
}

// InjectNav prepends the synthetic [use][..] rows to a directory's children,
// matching the order both apps already produce (Current, then Directories).
// children are assumed already sorted (dirs first) by the source.
func InjectNav(dir string, children []Row, nav Nav) []Row {
	out := make([]Row, 0, len(children)+2)
	if nav.UseRow {
		title := nav.UseTitle
		if title == "" {
			title = "Use this folder"
		}
		out = append(out, Row{
			Token:       dir,
			Title:       title,
			Description: dir,
			Group:       nav.UseGroup,
			Badge:       nav.UseBadge,
			Kind:        nav.UseKind,
			Intent:      IntentUseContainer,
			Selectable:  true,
			IsContainer: true,
		})
	}
	if nav.Parent != "" && nav.Parent != dir {
		title := nav.UpTitle
		if title == "" {
			title = ".."
		}
		out = append(out, Row{
			Token:       nav.Parent,
			Title:       title,
			Description: nav.Parent,
			Group:       nav.UpGroup,
			Badge:       nav.UpBadge,
			Kind:        nav.UpKind,
			Intent:      IntentAscend,
			Selectable:  true,
			IsContainer: true,
		})
	}
	return append(out, children...)
}
