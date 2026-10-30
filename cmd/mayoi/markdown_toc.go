package main

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"mayoi/locales"
)

// markdownTOCEntry is a heading of a Markdown preview's text: the block it
// heads, its level, and its words.
type markdownTOCEntry struct {
	block, level int
	text         string
}

// markdownTOC lists the headings of a preview, in the order the text has
// them.
func markdownTOC(m *markdownPreview) []markdownTOCEntry {
	var out []markdownTOCEntry
	for i, b := range m.blocks {
		if b.kind != markdownHeading {
			continue
		}
		var s strings.Builder
		for _, r := range b.runs {
			s.WriteString(r.text)
		}
		if text := strings.TrimSpace(s.String()); text != "" {
			out = append(out, markdownTOCEntry{block: i, level: b.level, text: text})
		}
	}
	return out
}

// markdownPreviewView shows a Markdown preview, and its contents beside it
// when the text has headings.
func (a *app) markdownPreviewView(c *ui.Context, v *viewer, name string) {
	if len(markdownTOC(v.markdown)) == 0 {
		a.markdownView(c, v, name)
		return
	}
	ui.Row(c.Key("markdown-preview")).Fill().AlignItems(ui.Stretch).MinHeight(0).Children(func() {
		ui.Column(c.Key("markdown-panes")).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
			a.markdownView(c, v, name)
		})
		a.markdownTOCView(c, v)
	})
}

// markdownTOCView is the preview's contents: the headings of the text, at
// the depth of their level, which a click scrolls to.
func (a *app) markdownTOCView(c *ui.Context, v *viewer) {
	m, p := v.markdown, a.pal
	ui.Column(c.Key("markdown-toc")).Width(220).Shrink(0).FillHeight().MinHeight(0).
		Background(p.chrome).BorderWidth(0, 0, 0, 1).BorderColor(p.border).Children(func() {
		ui.Row(c).Height(34).Padding(0, 12).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(p.border).Children(func() {
			ui.Text(c, locales.MarkdownContents()).FontSize(11).FontWeight(600).
				LetterSpacing(titleSpacing).TextColor(p.muted).SingleLine()
		})
		first, _ := m.list.Visible()
		active := -1
		for _, e := range markdownTOC(m) {
			if e.block <= first {
				active = e.block
			}
		}
		ui.Scroll(c.Key("markdown-toc-list")).Grow(1).Padding(6, 8).Children(func() {
			for _, e := range markdownTOC(m) {
				a.markdownTOCRow(c, m, e, e.block == active)
			}
		})
	})
}

// markdownTOCRow is a heading in the contents.
func (a *app) markdownTOCRow(c *ui.Context, m *markdownPreview, e markdownTOCEntry, active bool) {
	p := a.pal
	row := ui.ButtonBase(c.Key(fmt.Sprintf("markdown-toc:%d", e.block))).Height(24).Padding(2, 6, 2, 6+float32(e.level-1)*10).
		Radius(4).Justify(ui.Start).AlignItems(ui.Center).Label(e.text)
	col := p.muted
	switch {
	case active:
		row.Background(p.active)
		col = p.text
	case row.Hovered():
		row.Background(p.hover)
		col = p.text
	}
	row.Transition(hoverFade)
	row.Children(func() {
		ui.Text(c, e.text).FontSize(11.5).TextColor(col).SingleLine()
	})
	if row.Clicked() {
		m.list.ScrollTo(e.block, ui.Start)
	}
}
