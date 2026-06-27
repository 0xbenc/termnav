package teax

import (
	"context"
	"io"

	tea "charm.land/bubbletea/v2"

	"github.com/0xbenc/termnav"
)

// ProgramIO carries the terminal wiring for a standalone Run.
type ProgramIO struct {
	Input  io.Reader
	Output io.Writer
}

// Run drives a standalone browser to completion and returns the committed
// Outcome (ok=false on cancel). It is the drop-in replacement for an app's
// bespoke BrowseDir/BrowseTransfer plus its caller-driven re-list loop: the
// navigation, async listing, and cancelation all happen inside one program, so
// the program is no longer torn down and rebuilt on every cd.
func Run(ctx context.Context, cfg Config, opts termnav.Options, io ProgramIO) (termnav.Outcome, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	model := New(ctx, cfg, opts)

	programOptions := []tea.ProgramOption{tea.WithContext(ctx)}
	if io.Input != nil {
		programOptions = append(programOptions, tea.WithInput(io.Input))
	}
	if io.Output != nil {
		programOptions = append(programOptions, tea.WithOutput(io.Output))
	}

	final, err := tea.NewProgram(model, programOptions...).Run()
	if err != nil {
		return termnav.Outcome{}, false, err
	}
	m, ok := final.(Model)
	if !ok {
		return termnav.Outcome{}, false, nil
	}
	out, committed := m.nav.Outcome()
	return out, committed, nil
}
