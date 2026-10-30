package main

import (
	"errors"
	"net"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"mayoi/internal/settings"
)

// fakeHC stands in for hc: it records the commands run and answers them
// with reply, by the command's name.
type fakeHC struct {
	mu    sync.Mutex
	calls [][]string
	reply func(args []string) ([]byte, error)
}

func useFakeHC(t *testing.T, reply func(args []string) ([]byte, error)) *fakeHC {
	f := &fakeHC{reply: reply}
	oldRun, oldLook := hcRun, lookPath
	t.Cleanup(func() { hcRun, lookPath = oldRun, oldLook })
	hcRun = func(args ...string) ([]byte, error) {
		f.mu.Lock()
		f.calls = append(f.calls, args)
		f.mu.Unlock()
		if f.reply == nil {
			return []byte("{}"), nil
		}
		return f.reply(args)
	}
	lookPath = func(name string) (string, error) {
		if name == "hc" {
			return "/usr/local/bin/hc", nil
		}
		return oldLook(name)
	}
	return f
}

// ran returns the commands run whose name is cmd, waiting a moment for
// those run in the background.
func (f *fakeHC) ran(cmd string) [][]string {
	var out [][]string
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(time.Millisecond) {
		f.mu.Lock()
		out = nil
		for _, c := range f.calls {
			if c[0] == cmd {
				out = append(out, c)
			}
		}
		f.mu.Unlock()
		if len(out) > 0 {
			break
		}
	}
	return out
}

// connTerminal is a terminal of no program, for a session that never
// starts one.
func connTerminal(t *testing.T) *terminal.Terminal {
	near, far := net.Pipe()
	t.Cleanup(func() { far.Close() })
	term, err := terminal.New(terminal.Options{Conn: near})
	if err != nil {
		t.Fatal(err)
	}
	return term
}

// hcSession adds a terminal of the panel whose shell runs in herdr-core
// at addr.
func hcSession(t *testing.T, a *app, addr string) *session {
	a.nextTerm++
	s := &session{id: a.nextTerm, dir: a.root, hc: addr, term: connTerminal(t)}
	a.terms = append(a.terms, s)
	a.panelTerm = s
	return s
}

func TestHCError(t *testing.T) {
	fallback := errors.New("exit status 1")
	for in, want := range map[string]string{
		`{"error":{"kind":"spawn_failed","message":"cannot start \"/bin/zsh\""}}`: `cannot start "/bin/zsh"`,
		"hc: no core\n": "hc: no core",
		"":              "exit status 1",
	} {
		if got := hcError([]byte(in), fallback).Error(); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}

// With terminalBackend "hc", a terminal starts a shell in herdr-core,
// labeled with the folder, and attaches to it; without hc, or without
// the setting, it runs here.
func TestHCStart(t *testing.T) {
	hs := newHarness(t, true)
	a := hs.a
	f := useFakeHC(t, func(args []string) ([]byte, error) {
		return []byte(`{"address":"t9","state":"live"}`), nil
	})
	if a.hcBackend() {
		t.Fatal("hc without terminalBackend")
	}
	a.settings.TerminalBackend = "hc"
	if !a.hcBackend() {
		t.Fatal("no hc with terminalBackend hc")
	}
	addr, err := a.hcStart(hs.root)
	if err != nil || addr != "t9" {
		t.Fatalf("hcStart: %q, %v", addr, err)
	}
	run := f.ran("run")
	want := []string{"run", "--bg", "--json", "--no-parent", "--rm", "--cwd", hs.root, "--label", "mayoi.folder=" + hs.root}
	if len(run) != 1 || !slices.Equal(run[0], want) {
		t.Errorf("ran %q, want %q", run, want)
	}
	if got := hcAttach("t9", false); !slices.Equal(got, []string{"hc", "attach", "t9"}) {
		t.Errorf("attach command %q", got)
	}
	if got := hcAttach("t9", true); !slices.Equal(got, []string{"hc", "attach", "--history", "1000", "t9"}) {
		t.Errorf("attach command with history %q", got)
	}
	f.reply = func([]string) ([]byte, error) { return nil, errors.New("no core") }
	if _, err := a.hcStart(hs.root); err == nil || err.Error() != "no core" {
		t.Errorf("hcStart with no core: %v", err)
	}
	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	if a.hcBackend() {
		t.Error("hc backend without hc")
	}
}

// Closing a terminal ends its shell in herdr-core; closing the window
// leaves it running there.
func TestHCKill(t *testing.T) {
	hs := newHarness(t, true)
	a := hs.a
	f := useFakeHC(t, nil)
	s := hcSession(t, a, "t4")
	a.killTerminal(s)
	if kill := f.ran("kill"); len(kill) != 1 || kill[0][len(kill[0])-1] != "t4" {
		t.Fatalf("closing ran %q", kill)
	}
	if slices.Contains(a.terms, s) {
		t.Error("the closed terminal stays")
	}
	hcSession(t, a, "t5")
	a.closeTerminals()
	hs.settle()
	if kill := f.ran("kill"); len(kill) != 1 {
		t.Errorf("closing the window ran %q", kill)
	}
}

// Ctrl+\ in a terminal of herdr-core goes to its shell through hc keys,
// rather than detaching hc attach; other keys, and terminals of Mayoi's,
// are left alone.
func TestHCDetachKey(t *testing.T) {
	f := useFakeHC(t, nil)
	here := &session{}
	if hcDetachKey(here, ui.Ctrl, ui.KeyBackslash) {
		t.Error("took Ctrl+\\ of a terminal of Mayoi's")
	}
	s := &session{hc: "t3"}
	if hcDetachKey(s, ui.Ctrl|ui.Shift, ui.KeyBackslash) || hcDetachKey(s, ui.Ctrl, ui.KeyC) {
		t.Error("took another key")
	}
	if !hcDetachKey(s, ui.Ctrl, ui.KeyBackslash) {
		t.Fatal("left Ctrl+\\ to hc attach")
	}
	keys := f.ran("keys")
	if len(keys) != 1 || !slices.Equal(keys[0], []string{"keys", "--json", "t3", `ctrl+\`}) {
		t.Errorf("ran %q", keys)
	}
}

// As hc attach ends, its terminal closes once the shell ended, and one
// whose attach ended as soon as it began closes with an error rather
// than attach again and again.
func TestHCExited(t *testing.T) {
	hs := newHarness(t, true)
	a := hs.a
	state := "finished"
	useFakeHC(t, func(args []string) ([]byte, error) {
		if args[0] == "get" && state == "" {
			return nil, errors.New("no terminal")
		}
		return []byte(`{"state":"` + state + `"}`), nil
	})
	for _, st := range []string{"finished", ""} {
		state = st
		s := hcSession(t, a, "t6")
		s.attached = time.Now().Add(-time.Minute)
		a.terminalExited(s, s.term, 0)
		hs.settle()
		if slices.Contains(a.terms, s) {
			t.Errorf("the terminal of a shell %q stays", st)
		}
	}
	state = "live"
	fails := a.fails
	s := hcSession(t, a, "t7")
	s.attached = time.Now()
	a.terminalExited(s, s.term, 0)
	hs.settle()
	if slices.Contains(a.terms, s) || a.fails != fails+1 {
		t.Errorf("an attach that ended at once: open %v, %d errors", slices.Contains(a.terms, s), a.fails-fails)
	}
	// One closed here is not attached again.
	s = hcSession(t, a, "t8")
	t8 := s.term
	a.killTerminal(s)
	a.terminalExited(s, t8, 0)
	hs.settle()
	if slices.Contains(a.terms, s) {
		t.Error("a closed terminal came back")
	}
}

// A shell running on in herdr-core as hc attach ends, as after the core
// restarts, gets its terminal attached again, in the same tab.
func TestHCReattach(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a program")
	}
	hs := newHarness(t, true)
	a := hs.a
	useFakeHC(t, func([]string) ([]byte, error) { return []byte(`{"state":"live"}`), nil })
	attached := fakeAttach(t)
	s := hcSession(t, a, "t9")
	first := s.term
	s.attached = time.Now().Add(-time.Minute)
	a.terminalExited(s, first, 0)
	hs.settle()
	if !slices.Contains(a.terms, s) || s.term == first || !slices.Equal(*attached, []string{"t9+history"}) {
		t.Fatalf("not attached again: open %v, attached %q", slices.Contains(a.terms, s), *attached)
	}
	a.closeTerminals()
}

// The settings keep the addresses of the terminals whose shells run in
// herdr-core, of tabs and of the panel, with the one the panel shows.
func TestHCRemembered(t *testing.T) {
	hs := newHarness(t, true)
	a := hs.a
	useFakeHC(t, nil)
	hcSession(t, a, "t3")
	shown := hcSession(t, a, "t4")
	here := &session{id: 99, dir: a.root, term: connTerminal(t)}
	a.terms = append(a.terms, here)
	tab := hcSession(t, a, "t5")
	a.panelTerm = shown
	tab.tab = &colTab{term: tab}
	a.addTab(a.column(), len(a.column().tabs), tab.tab)
	a.rememberTabs()
	if w := a.window; !slices.Equal(w.PanelTerms, []string{"t3", "t4"}) || w.PanelTerm != 1 {
		t.Errorf("panel %q, showing %d", w.PanelTerms, w.PanelTerm)
	}
	tabs := a.window.Columns[0].Tabs
	if ts := tabs[len(tabs)-1]; !ts.Terminal || ts.HC != "t5" {
		t.Errorf("tab %+v", ts)
	}
}

// fakeAttach has terminals of herdr-core run a program that waits, and
// returns the addresses attached to, with history or not.
func fakeAttach(t *testing.T) *[]string {
	old := hcAttach
	t.Cleanup(func() { hcAttach = old })
	var attached []string
	hcAttach = func(addr string, history bool) []string {
		if history {
			addr += "+history"
		}
		attached = append(attached, addr)
		return []string{"sleep", "30"}
	}
	return &attached
}

// Starting again, a window attaches to the terminals whose shells still
// run in herdr-core, tabs and the panel's, with what they showed before,
// and starts a new shell for a tab whose shell ended.
func TestHCRestore(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a program")
	}
	hs := newHarness(t, true)
	a := hs.a
	t.Cleanup(a.closeTerminals)
	sub := hs.path("web")
	useFakeHC(t, func(args []string) ([]byte, error) {
		return []byte(`{"terminals":[{"address":"t3","cwd":"` + sub + `","state":"live"},` +
			`{"address":"t4","state":"finished"},{"address":"t6","state":"live"},{"address":"t7","state":"live"}]}`), nil
	})
	attached := fakeAttach(t)
	a.window.Columns = []settings.Column{{Tabs: []settings.Tab{{Terminal: true, HC: "t3"}, {Terminal: true, HC: "t4"}}}}
	a.restoreTabs()
	hs.settle()
	var terms []*session
	for _, tab := range a.column().tabs {
		if tab.term != nil {
			terms = append(terms, tab.term)
		}
	}
	if len(terms) != 2 || terms[0].hc != "t3" || terms[0].dir != sub || terms[1].hc != "" {
		t.Fatalf("restored %d terminal tabs", len(terms))
	}
	if !slices.Equal(*attached, []string{"t3+history"}) {
		t.Errorf("attached %q", *attached)
	}
	// The panel: t5 ended, t3 shows already, and the one it showed was t7.
	if !a.restorePanel([]string{"t5", "t6", "t3", "t7"}, 3) {
		t.Fatal("the panel attached to none")
	}
	var panel []string
	for _, s := range a.panelTerms() {
		panel = append(panel, s.hc)
	}
	if !slices.Equal(panel, []string{"t6", "t7"}) || a.panelTerm == nil || a.panelTerm.hc != "t7" {
		t.Errorf("panel %q", panel)
	}
	if a.restorePanel([]string{"t5"}, 0) {
		t.Error("attached to a shell that ended")
	}
}
