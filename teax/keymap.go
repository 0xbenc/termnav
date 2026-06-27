// Package teax is the Bubble Tea v2 adapter for termnav: the ONLY layer that
// imports bubbletea, so the kernel stays framework-free and trivially testable.
// It translates tea.Msg <-> termnav.Event through a per-app KeyMap, runs the
// reducer's []Effect as tea.Cmd (the only place IO happens), owns the generation
// token and cancelation of in-flight listings, and exposes BOTH an embeddable
// Model and a turnkey Run that replaces an app's bespoke browser program and
// its caller-driven "list dir -> browse -> cd -> repeat" loop.
package teax

import (
	tea "charm.land/bubbletea/v2"

	"github.com/0xbenc/termnav"
)

// KeyMap resolves a raw keypress to a normalized termnav.KeyEvent. ok=false
// means "ignore this key". It is the explicit seam for the divergent key
// profiles the two apps carry (ssherpa Q=cancel, passage ctrl+q=cancel): an app
// provides its own KeyMap that handles its specials and delegates the rest to
// DefaultKey.
type KeyMap func(msg tea.KeyPressMsg) (termnav.KeyEvent, bool)

// DefaultKey is the shared key profile: arrows/emacs motion, paging, section
// jumps, enter, backspace, esc/ctrl+c cancel, and printable text into the
// filter. An app wraps it to add its own cancel/extra keys.
func DefaultKey(msg tea.KeyPressMsg) (termnav.KeyEvent, bool) {
	key := msg.String()
	keystroke := msg.Key().Keystroke()
	switch {
	case key == "ctrl+c" || key == "esc":
		return termnav.KeyEvent{Key: "cancel"}, true
	case key == "enter":
		return termnav.KeyEvent{Key: "enter"}, true
	case key == "backspace":
		return termnav.KeyEvent{Key: "backspace"}, true
	case key == "home":
		return termnav.KeyEvent{Key: "home"}, true
	case key == "end":
		return termnav.KeyEvent{Key: "end"}, true
	case key == "pgup":
		return termnav.KeyEvent{Key: "pgup"}, true
	case key == "pgdown":
		return termnav.KeyEvent{Key: "pgdown"}, true
	case keystroke == "shift+up" || keystroke == "shift+left":
		return termnav.KeyEvent{Key: "section-up"}, true
	case keystroke == "shift+down" || keystroke == "shift+right":
		return termnav.KeyEvent{Key: "section-down"}, true
	case key == "up" || key == "ctrl+p":
		return termnav.KeyEvent{Key: "up"}, true
	case key == "down" || key == "ctrl+n":
		return termnav.KeyEvent{Key: "down"}, true
	case key == "left" || key == "right":
		return termnav.KeyEvent{}, false // ignore horizontal arrows
	}
	if text := msg.Text; text != "" && !isControl(text) {
		return termnav.KeyEvent{Text: text}, true
	}
	return termnav.KeyEvent{}, false
}

// isControl reports whether a key's text payload is a control sequence that
// should not be treated as filter input.
func isControl(text string) bool {
	for _, r := range text {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}
