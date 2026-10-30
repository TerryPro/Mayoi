package main

import (
	"math"
	"strings"

	"github.com/egoist/mygo/ui"

	"mayoi/locales"
)

// A terminal whose shell runs in herdr-core, or waits for one, says so:
// a dot at the corner of its icon, in its tab and its chip of the panel,
// and a pill over the terminal while it waits.

// termState is what a terminal's dot says.
type termState int

const (
	termHere     termState = iota // runs in Mayoi: no dot
	termKept                      // its shell runs in herdr-core
	termStarting                  // its shell starts in herdr-core
	termWaiting                   // waits for the connection or the machine that went
)

// termStatus is what a terminal's dot says about it.
func termStatus(s *session) termState {
	switch {
	case s == nil:
		return termHere
	case s.hcStarting:
		return termStarting
	case s.lost:
		return termWaiting
	case s.hc != "":
		return termKept
	}
	return termHere
}

// termWait is what the pill over a terminal says while it waits, "" when
// it doesn't.
func termWait(s *session) string {
	switch termStatus(s) {
	case termStarting:
		return locales.HcStarting()
	case termWaiting:
		if c := sessionConn(s); c != nil {
			return locales.TermWaiting(c.Host)
		}
		if machine, _, ok := strings.Cut(s.hc, "/"); ok {
			return locales.TermWaiting(machine)
		}
		return locales.TermWaiting("herdr-core")
	}
	return ""
}

// termIcon is a terminal's icon of size, with the dot of its state at its
// lower right corner: steady while its shell runs in herdr-core, pulsing
// while it starts or waits. The dot grows in and shrinks away as the
// state comes and goes.
func (a *app) termIcon(c *ui.Context, s *session, size float32, tint ui.Color) {
	p := a.pal
	st := termStatus(s)
	box := ui.Box(c).Size(size, size).Shrink(0)
	if st == termKept {
		box.Tooltip(locales.HcRunsIn(s.hc))
	}
	box.Children(func() {
		ui.Icon(c, iconTerminal).FontSize(size).TextColor(tint)
		if st == termHere {
			return
		}
		col := p.addedFg
		switch st {
		case termStarting:
			col = p.accent
		case termWaiting:
			col = p.modifiedFg
		}
		pulse := st != termKept
		d := float32(6)
		ui.Box(c.Key("dot")).Absolute().Right(-2).Bottom(-1).Size(d, d).Transition(dotPop).
			Draw(func(pt *ui.Painter, r ui.Rect) { statusDot(pt, r, col, pulse) })
	})
}

// statusDot paints a dot of col in r, and while pulse is set, a ring
// that swells from it and fades, as a beacon, at 30 frames a second,
// which the window paints without building the view.
func statusDot(pt *ui.Painter, r ui.Rect, col ui.Color, pulse bool) {
	if pulse {
		const period = 1400
		t := float32(pt.Now().UnixMilli()%period) / period
		grow := r.W * 0.9 * easeOutCubic(t)
		ring := ui.Rect{X: r.X - grow, Y: r.Y - grow, W: r.W + 2*grow, H: r.H + 2*grow}
		pt.Fill(ring, col.Alpha(0.45*(1-t)), ring.W/2)
		pt.AnimationFrameRate(30)
	}
	pt.Fill(r, col, r.W/2)
}

func easeOutCubic(t float32) float32 {
	return 1 - float32(math.Pow(float64(1-t), 3))
}

// hcMenu adds to the menu of a terminal whose shell runs in herdr-core
// an item that copies its address, for hc attach elsewhere.
func (a *app) hcMenu(m *ui.Menu, s *session) {
	if s.hc == "" {
		return
	}
	if m.Item(locales.TermCopyHC()).Chosen() {
		addr := s.hc
		a.post(func() {
			a.services.WriteClipboard(addr)
			a.tell(note{message: locales.CopiedText(addr)})
		})
	}
}

// waitPill is the pill over a terminal that waits, at its upper right:
// a spinner and what it waits for. It drops in and fades away.
func (a *app) waitPill(c *ui.Context, s *session) {
	msg := termWait(s)
	if msg == "" {
		return
	}
	p := a.pal
	box := ui.Row(c.Key("term-wait")).Absolute().Top(8).Right(14).Padding(4, 10, 4, 8).Gap(7).Radius(11).AlignItems(ui.Center)
	a.floating(c, box, false).Transition(popIn).Children(func() {
		ui.Spinner(c).Size(11, 11).TextColor(p.muted)
		ui.Text(c, msg).FontSize(11.5).TextColor(p.muted).SingleLine().MaxWidth(320)
	})
}
