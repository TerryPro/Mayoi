package main

import (
	"strconv"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/lsp"
)

// foldBadgeW is the width of a folded block's {…}.
const foldBadgeW = 36

// This file is the fork's share of the editor's view: the fold markers, the
// end of a folded block and the language server's problems. It is kept out
// of editor.go, which upstream also changes, so that the editor merges with
// a handful of calls rather than a block of drawing on every sync.

// editorForkHooks keeps the folds of an editor, and those the language
// server knows, right before its view is built.
func (a *app) editorForkHooks(d *editor, snap ui.TextSnapshot) {
	if len(d.folds) > 0 {
		a.refreshFolds(d, snap)
	}
	a.clampFoldSelection(d)
	a.requestLSPFolds(d, snap)
}

// editorForkKeys lets the language server take a key before the editor's
// own.
func (a *app) editorForkKeys(d *editor, ev ui.InputEvent) bool {
	if a.completionKey(d, ev) {
		return true
	}
	return a.fixKey(d, ev)
}

// forkProblems returns the problems the language server found in the file
// an editor shows.
func (a *app) forkProblems(d *editor) []lsp.Diagnostic {
	return a.lspDiags[lsp.Key(d.path)].diags
}

// editorForkOverlays puts a fold's marker under the hand that hovers it, and
// the end of each folded block in view, over the area, whose own press would
// move the caret.
func (a *app) editorForkOverlays(c *ui.Context, d *editor, snap ui.TextSnapshot, g geometry, view ui.Rect, gutterW, textX float32, f codeFont, lines int) {
	// A hand over the fold marker, which folds the block on a click,
	// over the area, whose own press would move the caret.
	if line := d.foldHover; line >= 0 && line < lines {
		_, folded := d.foldStarting(line)
		_, foldable := a.foldRangeFor(d, snap, line)
		if folded || foldable {
			b := ui.ButtonBase(c.Key("fold-chevron:" + strconv.Itoa(line))).Absolute().
				Left(d.hscroll.X + gutterW - 15).Top(editorPadY + g.top(line) - d.vscroll.Y).
				Width(14).Height(f.lineH).Cursor(ui.CursorPointer)
			if b.Clicked() {
				a.post(func() { a.toggleFold(d, line, snap) })
			}
		}
	}
	// The end of a folded block, which unfolds it as a click does. Wrapped
	// lines keep no place for it, where their rows end.
	for _, fold := range d.folds {
		if g.lines != nil || fold.start < 0 || fold.start >= lines || d.folded(fold.start) {
			continue
		}
		y := editorPadY + g.top(fold.start) - d.vscroll.Y
		if y < -f.lineH || view.H > 0 && y > view.H {
			continue
		}
		s, e := snap.LineRange(fold.start)
		text := snap.Slice(s, e)
		bx := d.hscroll.X + textX + f.width(text) + 4
		b := ui.ButtonBase(c.Key("fold-open:" + strconv.Itoa(fold.start))).Absolute().
			Left(bx).Top(y).Width(foldBadgeW).Height(f.lineH).Cursor(ui.CursorPointer)
		if b.Clicked() {
			line := fold.start
			a.post(func() { a.toggleFold(d, line, snap) })
		}
	}
}

// drawForkDiags underlines the problems the language server found, along the
// rows of the lines they are on.
func drawForkDiags(pt *ui.Painter, r ui.Rect, lineH float32, deco *decorations, p *palette,
	place func(ui.Rect, int, float32, float32, func(float32, float32, float32))) {
	for _, dm := range deco.diags {
		place(r, dm.line, dm.x0, dm.x1, func(y, x0, x1 float32) {
			wavyUnderline(pt, x0, x1, y+lineH-2, severityColor(p, dm.severity))
		})
	}
}

// drawForkDiagMarks marks the problems along the right edge, where the
// whole document shows.
func drawForkDiagMarks(pt *ui.Painter, r ui.Rect, right float32, lines int, diags []lsp.Diagnostic, p *palette) {
	for _, dg := range diags {
		line := dg.Range.Start.Line
		if line < 0 || line >= lines {
			continue
		}
		y := r.Y + float32(line)/float32(lines)*r.H
		pt.Fill(ui.Rect{X: right + 4, Y: y, W: 5, H: 2}, severityColor(p, dg.Severity), 1)
	}
}

// drawForkFoldGutter draws a fold's marker by the gutter of every foldable
// line, and the hand's own: over the markers, or as the settings always
// show them, as VS Code shows them.
func (a *app) drawForkFoldGutter(pt *ui.Painter, r ui.Rect, d *editor, snap ui.TextSnapshot, from, to int, x, gutterW float32,
	lineY func(ui.Rect, int) float32, lineH float32, p *palette) {
	for i := from; i <= to; i++ {
		if d.folded(i) {
			continue
		}
		show, folded := false, false
		if _, ok := d.foldStarting(i); ok {
			show, folded = true, true
		} else if d.foldGutter || a.settings.ShowFoldingControls == "always" {
			if _, ok := a.foldRangeFor(d, snap, i); ok {
				show = true
			}
		}
		if !show {
			continue
		}
		icon := iconChevronDown
		if folded {
			icon = iconChevronRight
		}
		side := float32(12)
		y := lineY(r, i)
		col := p.muted
		if d.foldHover == i {
			pt.Fill(ui.Rect{X: x + gutterW - 15, Y: y + 1, W: 14, H: lineH - 2}, p.hover, 3)
			col = p.text
		}
		pt.Icon(icon, ui.Rect{
			X: x + gutterW - 13, Y: y + (lineH-side)/2, W: side, H: side,
		}, col)
	}
}

// drawForkFoldBadges shows where each folded block in view ends, at the end
// of its first line. Wrapped lines keep no place for it.
func (a *app) drawForkFoldBadges(pt *ui.Painter, r ui.Rect, d *editor, snap ui.TextSnapshot, from, to int, textX float32,
	f codeFont, lineY func(ui.Rect, int) float32, lineH float32, p *palette) {
	for _, fold := range d.folds {
		if fold.start < from || fold.start > to || d.folded(fold.start) {
			continue
		}
		y := lineY(r, fold.start)
		s, e := snap.LineRange(fold.start)
		text := snap.Slice(s, e)
		bx := r.X + textX + f.width(text) + 4 - d.hscroll.X
		pt.Fill(ui.Rect{X: bx, Y: y + 1, W: foldBadgeW, H: lineH - 2}, p.active, 3)
		span := ui.Span{Text: "{…}", Font: f.family, Size: f.size, Color: p.muted}
		tw, th := pt.MeasureText(0, span)
		pt.RichText(bx+(foldBadgeW-tw)/2, y+(lineH-th)/2, 0, span)
	}
}

// lspForkOverlays draws the language server's words at the caret, and what
// it says of the symbol there, over the text and the area's own drawing.
func (a *app) lspForkOverlays(c *ui.Context, d *editor, g geometry, textX, lineH float32, view ui.Rect) {
	lineTop := func(line int) float32 {
		return editorPadY + g.top(line) - d.vscroll.Y
	}
	a.completionView(c, d, lineTop, textX, lineH, view)
	a.fixView(c, d, lineTop, textX, lineH, view)
}
