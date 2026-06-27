package teax

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/0xbenc/termnav"
)

// RenderFunc turns the pure browse model into a Bubble Tea view. An app supplies
// its own (capturing theme, title, steps, preview) so its frames stay
// byte-identical through the migration; the render package offers a shared
// default for new surfaces.
type RenderFunc func(termnav.Model) tea.View

// Config configures a teax browser.
type Config struct {
	Source termnav.FileSource
	Start  string // initial path; passed to Source.Resolve
	Render RenderFunc
	KeyMap KeyMap // nil uses DefaultKey
}

// Model is the embeddable Bubble Tea adapter. Use it directly to host a browser
// inside a larger program (passage's gap), or via Run for a standalone one.
type Model struct {
	nav      termnav.Model
	src      termnav.FileSource
	render   RenderFunc
	keymap   KeyMap
	baseCtx  context.Context
	start    string
	inflight map[uint64]context.CancelFunc
}

// listLoadedMsg carries a FileSource.List result back into the program.
type listLoadedMsg struct {
	gen     uint64
	listing termnav.Listing
	err     error
}

// startMsg carries the resolved start directory after the initial Resolve.
type startMsg struct {
	dir string
	err error
}

// New builds an embeddable browser model. opts configures the pure reducer;
// ctx bounds every listing (cancel it to abort all IO).
func New(ctx context.Context, cfg Config, opts termnav.Options) Model {
	if ctx == nil {
		ctx = context.Background()
	}
	km := cfg.KeyMap
	if km == nil {
		km = DefaultKey
	}
	return Model{
		nav:      termnav.New(opts),
		src:      cfg.Source,
		render:   cfg.Render,
		keymap:   km,
		baseCtx:  ctx,
		start:    cfg.Start,
		inflight: map[uint64]context.CancelFunc{},
	}
}

// Nav exposes the underlying pure model (e.g. to read the Outcome after the
// program ends).
func (m Model) Nav() termnav.Model { return m.nav }

func (m Model) Init() tea.Cmd {
	src := m.src
	base := m.baseCtx
	start := m.start
	return tea.Batch(tea.RequestWindowSize, func() tea.Msg {
		dir, err := src.Resolve(base, start)
		return startMsg{dir: dir, err: err}
	})
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		nav, _ := termnav.Update(m.nav, termnav.ResizeEvent{W: msg.Width, H: msg.Height})
		m.nav = nav
		return m, nil
	case startMsg:
		if msg.err != nil {
			// Surface the resolve failure as a load error and stop.
			nav, effects := m.nav.Load(msg.dir)
			m.nav = nav
			cmds := m.applyEffects(effects)
			nav, _ = termnav.Update(m.nav, termnav.ListLoadedEvent{Gen: m.nav.Gen(), Err: msg.err})
			m.nav = nav
			return m, tea.Batch(cmds...)
		}
		nav, effects := m.nav.Load(msg.dir)
		m.nav = nav
		return m, tea.Batch(m.applyEffects(effects)...)
	case listLoadedMsg:
		delete(m.inflight, msg.gen)
		nav, effects := termnav.Update(m.nav, termnav.ListLoadedEvent{Gen: msg.gen, Listing: msg.listing, Err: msg.err})
		m.nav = nav
		return m, tea.Batch(m.applyEffects(effects)...)
	case tea.KeyPressMsg:
		ev, ok := m.keymap(msg)
		if !ok {
			return m, nil
		}
		nav, effects := termnav.Update(m.nav, ev)
		m.nav = nav
		cmds := m.applyEffects(effects)
		if m.nav.Done() {
			cmds = append(cmds, m.finish())
		}
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

func (m Model) View() tea.View {
	if m.render != nil {
		return m.render(m.nav)
	}
	return tea.NewView("")
}

// applyEffects runs the reducer's effects: it lists directories (inline for a
// SyncLister so an instant read never flashes a spinner; otherwise in a
// cancelable goroutine), cancels superseded in-flight listings, and closes a
// stateful source. It may mutate m.nav (the sync fast-path applies the listing
// immediately).
func (m *Model) applyEffects(effects []termnav.Effect) []tea.Cmd {
	var cmds []tea.Cmd
	for _, e := range effects {
		switch ef := e.(type) {
		case termnav.ListDirEffect:
			if sl, ok := m.src.(termnav.SyncLister); ok {
				listing, err := sl.ListSync(ef.Dir)
				nav, more := termnav.Update(m.nav, termnav.ListLoadedEvent{Gen: ef.Gen, Listing: listing, Err: err})
				m.nav = nav
				cmds = append(cmds, m.applyEffects(more)...)
				continue
			}
			cmds = append(cmds, m.listCmd(ef.Gen, ef.Dir))
		case termnav.CancelListEffect:
			if cancel, ok := m.inflight[ef.Gen]; ok {
				cancel()
				delete(m.inflight, ef.Gen)
			}
		case termnav.CloseEffect:
			if c, ok := m.src.(termnav.Closer); ok {
				_ = c.Close()
			}
		}
	}
	return cmds
}

// listCmd returns a cancelable command that runs FileSource.List in the
// background and reports the result. The per-generation cancel func is stored so
// a CancelListEffect (a rapid cd away) aborts the in-flight, network-latent
// listing.
func (m *Model) listCmd(gen uint64, dir string) tea.Cmd {
	ctx, cancel := context.WithCancel(m.baseCtx)
	m.inflight[gen] = cancel
	src := m.src
	return func() tea.Msg {
		listing, err := src.List(ctx, dir)
		return listLoadedMsg{gen: gen, listing: listing, err: err}
	}
}

// finish cancels any in-flight listings, closes a stateful source, and quits.
func (m *Model) finish() tea.Cmd {
	for gen, cancel := range m.inflight {
		cancel()
		delete(m.inflight, gen)
	}
	if c, ok := m.src.(termnav.Closer); ok {
		_ = c.Close()
	}
	return tea.Quit
}
