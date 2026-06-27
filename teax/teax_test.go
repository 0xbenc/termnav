package teax

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/0xbenc/termnav"
)

// asyncSource is a FileSource that is NOT a SyncLister, so the adapter must
// route its listings through the Loading state and a cancelable command.
type asyncSource struct{ listing termnav.Listing }

func (a asyncSource) Resolve(_ context.Context, p string) (string, error) { return p, nil }
func (a asyncSource) List(_ context.Context, _ string) (termnav.Listing, error) {
	return a.listing, nil
}

// syncSource additionally implements SyncLister.
type syncSource struct{ listing termnav.Listing }

func (s syncSource) Resolve(_ context.Context, p string) (string, error) { return p, nil }
func (s syncSource) List(_ context.Context, _ string) (termnav.Listing, error) {
	return s.listing, nil
}
func (s syncSource) ListSync(_ string) (termnav.Listing, error) { return s.listing, nil }

func sampleListing() termnav.Listing {
	return termnav.Listing{Dir: "/x", Parent: "/", Rows: []termnav.Row{
		{Token: "/x/f", Title: "f", Intent: termnav.IntentSelectLeaf, Selectable: true},
	}}
}

func TestDefaultKeyMapping(t *testing.T) {
	cases := []struct {
		msg  tea.KeyPressMsg
		key  string
		text string
		ok   bool
	}{
		{tea.KeyPressMsg{Code: tea.KeyEnter}, "enter", "", true},
		{tea.KeyPressMsg{Code: tea.KeyEsc}, "cancel", "", true},
		{tea.KeyPressMsg{Code: tea.KeyUp}, "up", "", true},
		{tea.KeyPressMsg{Code: tea.KeyDown}, "down", "", true},
		{tea.KeyPressMsg{Code: tea.KeyBackspace}, "backspace", "", true},
		{tea.KeyPressMsg{Code: 'a', Text: "a"}, "", "a", true},
	}
	for _, c := range cases {
		ev, ok := DefaultKey(c.msg)
		if ok != c.ok || ev.Key != c.key || ev.Text != c.text {
			t.Errorf("DefaultKey(%q) = (%+v,%v), want key=%q text=%q ok=%v",
				c.msg.String(), ev, ok, c.key, c.text, c.ok)
		}
	}
}

func TestSyncFastPathSkipsLoading(t *testing.T) {
	m := New(context.Background(), Config{Source: syncSource{sampleListing()}, Start: "/x"}, termnav.Options{})
	tm, _ := m.Update(startMsg{dir: "/x"})
	m = tm.(Model)
	// A SyncLister source applies the listing inline: never stuck in Loading.
	if m.Nav().State() != termnav.Ready {
		t.Fatalf("sync source state = %v, want Ready (no Loading flash)", m.Nav().State())
	}
	if m.Nav().Cwd() != "/x" {
		t.Fatalf("cwd = %q, want /x", m.Nav().Cwd())
	}
}

func TestAsyncListCmdAndCancel(t *testing.T) {
	src := asyncSource{sampleListing()}
	m := New(context.Background(), Config{Source: src, Start: "/x"}, termnav.Options{})
	tm, _ := m.Update(startMsg{dir: "/x"})
	m = tm.(Model)
	// Async source must enter Loading and register an in-flight cancel.
	if m.Nav().State() != termnav.Loading {
		t.Fatalf("async source state = %v, want Loading", m.Nav().State())
	}
	if len(m.inflight) != 1 {
		t.Fatalf("inflight = %d, want 1 cancelable listing", len(m.inflight))
	}
	// The list command produces a listLoadedMsg for the right generation.
	cmd := m.listCmd(m.Nav().Gen(), "/x")
	msg, ok := cmd().(listLoadedMsg)
	if !ok || msg.gen != m.Nav().Gen() {
		t.Fatalf("listCmd msg = %#v, want listLoadedMsg gen %d", msg, m.Nav().Gen())
	}
	tm, _ = m.Update(msg)
	m = tm.(Model)
	if m.Nav().State() != termnav.Ready {
		t.Fatalf("after listLoaded, state = %v, want Ready", m.Nav().State())
	}
}

func TestCancelKeyTerminates(t *testing.T) {
	m := New(context.Background(), Config{Source: syncSource{sampleListing()}, Start: "/x"}, termnav.Options{})
	tm, _ := m.Update(startMsg{dir: "/x"})
	m = tm.(Model)
	tm, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = tm.(Model)
	if !m.Nav().Canceled() || !m.Nav().Done() {
		t.Fatal("esc did not cancel the browse")
	}
	if cmd == nil {
		t.Fatal("a terminated browse should return a Quit command")
	}
}
