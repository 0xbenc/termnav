# termnav

`termnav` is the shared file/namespace **navigation** engine for terminal TUIs —
a sibling to [`termtheme`](https://github.com/0xbenc/termtheme). It owns exactly
the must-agree navigation logic that [`passage`](https://github.com/0xbenc/passage)
and [`ssherpa`](https://github.com/0xbenc/ssherpa) had each been carrying a
near-identical copy of: the directory/list browse model, the fuzzy filter with a
relevance gate, the group-aware scroll window, cursor-snap over reference rows,
the shell-style path-completion index, and a first-class async
Loading/Ready/Error/Empty state machine.

It ships **none** of the parts that legitimately diverge per app — the transport
(local FS, remote SFTP, an in-memory namespace), the app's private row
vocabulary, badges/labels, dotfile/symlink policy, or the palette. Those are
supplied by the caller through a single injected `FileSource` seam and a thin
per-app shim, exactly as `termtheme` ships no palettes and takes the base from
`ThemeConfig.Resolve(base)`.

## Layers

```
termnav            L0  pure kernel — no bubbletea, no os, no net (stdlib only)
  Row, NavIntent, Listing, LoadState, Outcome
  Model + Update(Model, Event) -> (Model, []Effect)   the IO-free reducer
  Matcher (Fuzzy / Substring) + MatchFuzzy/Relevant
  Index (implicit-tree path completion) + Candidates/Classify/Breadcrumb
  Snap / ClampWindow / WindowContainsCursor / JumpSection   windowing
  FileSource + Closer/SyncLister/Indexer  + InjectNav

termnav/source     L1  concrete sources (imports os)
  LocalSource (os.ReadDir), TreeSource (zero-IO tree), StaticSource
  Conformance(t, src, start)  table-driven FileSource contract harness

termnav/teax       L2  Bubble Tea v2 adapter (the ONLY bubbletea importer)
  Model (embeddable) + Run (turnkey)  +  KeyMap / DefaultKey
  generation token + cancel of in-flight listings + SyncLister fast-path

termnav/render     L3  themed default render (the ONLY termtheme importer)
  Render(model, theme, Styler, Chrome)  — color keys off NavIntent, never Kind
  every name routed through termtheme.Sanitize at the render boundary
```

`L0`/`L1` depend on nothing outside the standard library; only `L2` pulls in
Bubble Tea and only `L3` pulls in `termtheme`. An app that wants the headless
core and its own rendering never compiles bubbletea-free code against a render
stack it doesn't use.

## The seam

```go
// The one interface an app must implement (or reuse source.LocalSource).
type FileSource interface {
	Resolve(ctx context.Context, path string) (start string, err error)
	List(ctx context.Context, dir string) (Listing, error)
}
// Optional, detected by type assertion:
//   Closer{ Close() }                 stateful session teardown (SFTP)
//   SyncLister{ ListSync(dir) }       instant reads skip the Loading flash
//   Indexer{ Index() *Index }         breadcrumb / shell-TAB completion
```

`List` returns one directory's children as `[]Row`, each tagged with a canonical
`NavIntent` (`Descend`, `Ascend`, `UseContainer`, `SelectLeaf`, `Reference`).
The kernel never calls `List` — only the `teax` adapter does, inside a
`tea.Cmd` — so the core is PTY-free and filesystem-free testable.

## Turnkey usage

```go
out, ok, err := teax.Run(ctx, teax.Config{
	Source: source.NewLocal(source.LocalOptions{SelectFiles: true}),
	Start:  ".",
	Render: myRenderFunc, // or render.Render via a small closure
}, termnav.Options{}, teax.ProgramIO{Input: os.Stdin, Output: os.Stderr})
if ok {
	chosen := out.Token()
}
```

`Run` replaces a bespoke browser program **and** the caller-driven
"list dir → browse → cd → repeat" loop: navigation, async listing, and
cancelation all happen inside one program, so the program is no longer torn down
and rebuilt on every directory change — and a slow remote listing shows a real
loading state and is abortable with Esc instead of freezing the terminal.

## Status

Extracted from the duplicated browsers in `passage` and `ssherpa`. The Index
parity tests are ported verbatim from `passage/internal/ui/pathindex_test.go`;
the windowing primitives are lifted from `ssherpa/internal/chrome/listview.go`;
the fuzzy matcher is the byte-identical copy both apps carried.

## License

MIT — see [LICENSE](LICENSE).
