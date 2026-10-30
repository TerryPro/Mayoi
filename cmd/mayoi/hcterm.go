package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"mayoi/internal/bg"
	"mayoi/locales"
)

// With terminalBackend "hc", a terminal's shell runs in a terminal of
// herdr-core, and the view runs hc attach on it: the shell outlives the
// window. Closing a terminal ends it there; closing the window or
// quitting only ends the attach, and the shell runs on.

// hcRun runs hc with args and returns what it printed, or the message
// of the error it printed instead. Tests replace it.
var hcRun = func(args ...string) ([]byte, error) {
	var stderr bytes.Buffer
	cmd := exec.Command("hc", args...)
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, hcError(stderr.Bytes(), err)
	}
	return out, nil
}

// hcError is the error hc printed, {"error": {"message": ...}}, or
// fallback when it printed none.
func hcError(out []byte, fallback error) error {
	var e struct {
		Error struct{ Message string } `json:"error"`
	}
	if json.Unmarshal(out, &e) == nil && e.Error.Message != "" {
		return errors.New(e.Error.Message)
	}
	if msg := strings.TrimSpace(string(out)); msg != "" {
		return errors.New(msg)
	}
	return fallback
}

// hcBackend reports whether new terminals run in herdr-core: the
// settings ask for it, and hc is there.
func (a *app) hcBackend() bool {
	if a.settings.TerminalBackend != "hc" {
		return false
	}
	_, err := lookPath("hc")
	return err == nil
}

// hcStart starts a shell in dir in a terminal of herdr-core, labeled
// with the window's folder, and returns its address. It is removed once
// its shell exits.
func (a *app) hcStart(dir string) (string, error) {
	out, err := hcRun("run", "--bg", "--json", "--no-parent", "--rm", "--cwd", dir, "--label", "mayoi.folder="+a.root)
	if err != nil {
		return "", err
	}
	var t struct{ Address string }
	if err := json.Unmarshal(out, &t); err != nil || t.Address == "" {
		return "", errors.New("hc run printed no address")
	}
	return t.Address, nil
}

// hcAttach is the command that shows the terminal of herdr-core at addr,
// with what scrolled off its screen when history is set. Tests replace
// it.
var hcAttach = func(addr string, history bool) []string {
	if history {
		return []string{"hc", "attach", "--history", "1000", addr}
	}
	return []string{"hc", "attach", addr}
}

// hcKill ends the shell of a terminal of herdr-core, in the background.
func hcKill(addr string) {
	bg.Go(func() { hcRun("kill", "--json", addr) })
}

// hcDetachKey reports whether a key is the one hc attach detaches with,
// Ctrl+\, which Mayoi takes so that a terminal can't come off its shell:
// the shell gets it from hc keys instead.
func hcDetachKey(s *session, mods ui.Modifiers, key ui.Key) bool {
	if s.hc == "" || mods != ui.Ctrl || key != ui.KeyBackslash {
		return false
	}
	addr := s.hc
	bg.Go(func() { hcRun("keys", "--json", addr, `ctrl+\`) })
	return true
}

// killTerminal ends a terminal the user closes: its shell in herdr-core
// too, which closing the window leaves running.
func (a *app) killTerminal(s *session) {
	if s.hc != "" {
		hcKill(s.hc)
	}
	a.removeTerminal(s)
}

// hcLive reports whether the terminal of herdr-core at addr still runs.
func hcLive(addr string) bool {
	out, err := hcRun("get", "--json", addr)
	var t struct{ State string }
	return err == nil && json.Unmarshal(out, &t) == nil && t.State == "live"
}

// hcExited takes the end of hc attach in a terminal: it closes once its
// shell ended, and attaches again while the shell runs on, as after
// herdr-core restarts. One that ends as soon as it attached closes, to
// not attach again and again.
func (a *app) hcExited(s *session, t *terminal.Terminal) {
	if !slices.Contains(a.terms, s) {
		return // closed here, or its window closed
	}
	addr, quick := s.hc, time.Since(s.attached) < time.Second
	bg.Go(func() {
		live := hcLive(addr)
		a.post(func() {
			if s.term != t || !slices.Contains(a.terms, s) {
				return
			}
			switch {
			case live && quick:
				a.fail("%s", locales.FailAttachHC(addr))
			case live:
				a.hcReattach(s)
				return
			}
			a.removeTerminal(s)
		})
	})
}

// hcReattach attaches a terminal again to its shell in herdr-core.
func (a *app) hcReattach(s *session) {
	old := s.term
	t, err := a.spawnTerminal(s, s.dir)
	if err != nil {
		a.fail("%s", locales.FailRestartTerminal(err.Error()))
		a.removeTerminal(s)
		return
	}
	s.term = t
	old.Close()
}

// hcTerms returns the terminals of herdr-core that run, by address, with
// the folder each is in: none when hc is not there.
func hcTerms() map[string]string {
	if _, err := lookPath("hc"); err != nil {
		return nil
	}
	out, err := hcRun("ls", "--json")
	if err != nil {
		return nil
	}
	var l struct {
		Terminals []struct{ Address, Cwd, State string }
	}
	if json.Unmarshal(out, &l) != nil {
		return nil
	}
	live := map[string]string{}
	for _, t := range l.Terminals {
		if t.State == "live" {
			live[t.Address] = t.Cwd
		}
	}
	return live
}

// hcShown reports whether a terminal here shows the shell at addr in
// herdr-core already.
func (a *app) hcShown(addr string) bool {
	return slices.ContainsFunc(a.terms, func(s *session) bool { return s.hc == addr })
}

// attachTerminal shows the shell at addr in herdr-core, in dir, in a
// terminal: one of a window before it closed.
func (a *app) attachTerminal(addr, dir string) *session {
	return a.addTerminal(&session{dir: dir, title: shellName(), hc: addr})
}

// restorePanel attaches the panel again to those of its terminals at
// addrs whose shells run on in herdr-core, showing the one at shown,
// and reports whether there were any. The panel stays as it was, shown
// or hidden.
func (a *app) restorePanel(addrs []string, shown int) bool {
	if len(addrs) == 0 {
		return false
	}
	live := hcTerms()
	var show *session
	for i, addr := range addrs {
		dir, ok := live[addr]
		if !ok || a.hcShown(addr) {
			continue
		}
		if s := a.attachTerminal(addr, dir); s != nil && (show == nil || i == shown) {
			show = s
		}
	}
	if show == nil {
		return false
	}
	a.panelTerm = show
	a.rememberTabs()
	return true
}
