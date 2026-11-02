package main

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"mayoi/locales"
)

// This file is the fork's share of the terminal panel: the list of its
// terminals beside the one it shows, as VS Code lists its terminal tabs,
// and the name a terminal takes when renamed. It is kept out of panel.go,
// which upstream also changes.

// termListWidth is the width the list beside the terminal shown takes: the
// width the layout keeps, 140 at first, within 46 and 420, and leaving the
// terminal room in a narrow panel.
func (a *app) termListWidth(panelWidth float32) float32 {
	size := a.window.TermListWidth
	if size <= 0 {
		size = 140
	}
	if panelWidth > 0 {
		size = min(size, maxTermListWidth(panelWidth))
	}
	return clamp(size, 46, 420)
}

// maxTermListWidth is the widest the list takes in a panel panelWidth wide,
// which keeps the terminal beside it 160 wide at least.
func maxTermListWidth(panelWidth float32) float32 {
	if panelWidth <= 0 {
		return 420
	}
	return min(420, max(panelWidth-160, 46))
}

// termListView lists the terminals of the panel, the one it shows chosen,
// each to show with a click and to act on through its menu, and a
// splitter on its left edge to widen or narrow it.
func (a *app) termListView(c *ui.Context, terms []*session, width, panelWidth float32) {
	p := a.pal
	list := ui.Column(c.Key("term-list")).Width(width).MinHeight(0).FillHeight().Background(p.panel).
		BorderWidth(0, 0, 0, 1).BorderColor(p.border)
	list.Children(func() {
		ui.Scroll(c.Key("term-list-scroll")).Grow(1).MinHeight(0).Padding(4).Children(func() {
			ui.Column(c).Gap(1).Children(func() {
				for i, s := range terms {
					a.termListRow(c, s, i+1, width < 80)
				}
			})
		})
		a.splitter(c, edgeLeft, func(d float32) {
			a.window.TermListWidth = clamp(width-d, 46, maxTermListWidth(panelWidth))
		})
	})
}

// termListRow is a terminal's row in the list: its icon and its name, the
// chosen one lit, and the cross that closes it under the pointer.
func (a *app) termListRow(c *ui.Context, s *session, n int, iconOnly bool) {
	p := a.pal
	on := s == a.panelTerm
	row := ui.ButtonBase(c.Key("term-row:" + fmt.Sprint(s.id))).FillWidth().Height(22).Gap(6).Radius(4).
		AlignItems(ui.Center).Label(fmt.Sprintf("%d: %s", n, termLabel(s))).Tooltip(termTabTip(s)).
		Drag(dragItem{term: s})
	if row.Dragging() {
		a.dragNow = true
	}
	if iconOnly {
		row.Justify(ui.Center)
	} else {
		row.Padding(0, 6)
	}
	under := p.panel
	switch {
	case on:
		row.Background(p.active)
		under = p.active.Over(p.panel)
	case row.Hovered():
		row.Background(p.hover)
		under = p.hover.Over(p.panel)
	}
	row.Transition(hoverFade)
	tint := p.muted
	if on {
		tint = p.text
	}
	row.Children(func() {
		a.termIcon(c, s, 14, tint, under)
		if iconOnly {
			return
		}
		ui.Text(c, termLabel(s)).FontSize(12).TextColor(tint).SingleLine().Grow(1)
		if row.Hovered() {
			x := ui.ButtonBase(c).Size(16, 16).Radius(4).Justify(ui.Center).AlignItems(ui.Center).Label(locales.TermKill())
			if x.Hovered() {
				x.Background(p.hover)
			}
			x.Transition(hoverFade)
			x.Children(func() {
				ui.Icon(c.Key("x"), iconX).FontSize(12).TextColor(p.muted)
			})
			if x.Clicked() {
				a.post(func() { a.killTerminal(s) })
			}
		}
	})
	if row.Clicked() {
		a.panelProblems = false
		a.panelTerm = s
		a.focusTerminal()
	}
	row.ContextMenu(func(m *ui.Menu) {
		m.Submenu(locales.CmdViewNewTerminal(), a.termProfileMenu)
		m.Separator()
		if m.Item(locales.RenameTitle()).Chosen() {
			a.post(func() { a.askRenameTerminal(s) })
		}
		if m.Item(locales.TermMoveLockedRight()).Chosen() {
			a.post(func() { a.terminalToLockedRight(s) })
		}
		if m.Item(locales.TermMoveEditorArea()).Chosen() {
			a.post(func() { a.terminalToColumn(s, a.column(), a.column().active+1) })
		}
		a.hcMenu(m, s)
		if m.Item(locales.TermKill()).Chosen() {
			a.post(func() { a.killTerminal(s) })
		}
	})
}

// askRenameTerminal asks for a name for a terminal, which its shell's title
// changes no longer replace; an empty name restores the shell's.
func (a *app) askRenameTerminal(s *session) {
	a.prompt = &prompt{title: locales.RenameTitle(), hint: termTabTip(s), value: termLabel(s), optional: true, done: func(v string) string {
		s.name = strings.TrimSpace(v)
		a.invalidate()
		return ""
	}}
}
