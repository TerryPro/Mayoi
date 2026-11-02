package main

import (
	"fmt"
	"net"
	"testing"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

// The panel lists its terminals at its right: a row shows one, and the
// splitter on the list's left edge widens or narrows it.
func TestPanelTerminalList(t *testing.T) {
	if testing.Short() {
		t.Skip("loads libghostty-vt")
	}
	for _, dark := range []bool{false, true} {
		t.Run(fmt.Sprintf("dark=%v", dark), func(t *testing.T) {
			hs := newHarness(t, dark)
			a := hs.a
			t.Cleanup(a.closeTerminals)
			for i, title := range []string{"one", "two"} {
				conn, peer := net.Pipe()
				t.Cleanup(func() { peer.Close() })
				term, err := terminal.New(terminal.Options{Conn: conn})
				if err != nil {
					conn.Close()
					t.Fatal(err)
				}
				a.terms = append(a.terms, &session{id: i + 1, term: term, title: title})
			}
			first, second := a.terms[0], a.terms[1]
			a.panelTerm = second
			var terminalBox ui.Rect
			tt := ui.NewTester(func(c *ui.Context) {
				a.pal = a.applyTheme(c)
				a.panel(c)
				terminalBox = a.panelTerm.view.Bounds(c)
			}, 1000, 280)
			tt.SetDark(dark)
			hs.tt = tt
			for range 5 {
				step(tt)
			}
			if terminalBox.Y != panelHeadH+2 {
				t.Errorf("terminal starts at %v, want %v", terminalBox.Y, float32(panelHeadH+2))
			}
			click := func(s *session, label string) {
				t.Helper()
				box, ok := tt.Find(label)
				if !ok {
					t.Fatalf("no row for %q", label)
				}
				tt.ClickAt(box.X+box.W/2, box.Y+box.H/2)
				step(tt)
				if a.panelTerm != s {
					t.Errorf("clicking %q did not show it", label)
				}
			}
			click(first, "1: one")
			click(second, "2: two")
			// The splitter on the list's left edge widens it, the rows
			// moving left with it.
			rowX := func() float32 {
				box, ok := tt.Find("1: one")
				if !ok {
					t.Fatal("no row for \"1: one\"")
				}
				return box.X
			}
			x0 := rowX()
			// The splitter is the list's left edge, just left of a row.
			listX := x0 - 4
			hs.drag(listX+2, terminalBox.Y+40, listX-58, terminalBox.Y+40)
			step(tt)
			if got := a.window.TermListWidth; got < 190 || got > 210 {
				t.Errorf("the splitter set the list %v wide, want about 200", got)
			}
			if x1 := rowX(); x1 >= x0 {
				t.Errorf("widening the list left its rows at %v, from %v", x1, x0)
			}
			click(first, "1: one")
			hs.shot(fmt.Sprintf("terminal-list-%v", dark))
			a.closeTerminals()
		})
	}
}
