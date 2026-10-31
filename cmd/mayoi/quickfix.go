package main

import (
	"context"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/bg"
	"mayoi/internal/lsp"
	"mayoi/locales"
)

// fixState is the changes the language server offers to fix the problems
// at the caret: the changes it offered, the one chosen, and the place the
// list shows at.
type fixState struct {
	open    bool
	actions []lsp.CodeAction
	sel     int
	doc     int
	line    int
}

// showQuickFix asks the language server what fixes the problems at the
// caret, and shows them under it.
func (a *app) showQuickFix() {
	d := a.activeEditor()
	if d == nil || d.path == "" || d.loading {
		return
	}
	if a.fix.open {
		a.closeFix()
		return
	}
	a.closeCompletion()
	a.closeHover()
	setup, srv := a.lspTarget(d.path)
	if setup == nil {
		a.tell(note{message: locales.LspOff()})
		return
	}
	if srv == nil || srv.client == nil {
		a.tell(note{message: locales.LspStarting()})
		return
	}
	snap := d.buf.Snapshot()
	caret := min(d.sel[1], snap.Len())
	line := snap.LineAt(caret)
	diags := a.lineDiags(d, line)
	if len(diags) == 0 {
		a.tell(note{message: locales.NoQuickFixes()})
		return
	}
	rng := diags[0].Range
	c, uri, id := srv.client, lsp.URI(d.path), d.id
	ver := snap.Version()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	bg.Go(func() {
		defer cancel()
		actions, err := c.CodeAction(ctx, uri, rng, diags)
		if err != nil {
			return
		}
		var kept []lsp.CodeAction
		for _, act := range actions {
			if act.Edit != nil {
				kept = append(kept, act)
			}
		}
		if len(kept) == 0 {
			return
		}
		a.post(func() {
			d := a.activeEditor()
			if d == nil || d.id != id || d.buf.Version() != ver {
				return
			}
			a.fix = fixState{open: true, actions: kept, doc: id, line: line}
			a.invalidate()
		})
	})
}

// closeFix hides the list of fixes.
func (a *app) closeFix() {
	if !a.fix.open {
		return
	}
	a.fix = fixState{}
	a.invalidate()
}

// lineDiags returns the problems of a document at a line.
func (a *app) lineDiags(d *editor, line int) []lsp.Diagnostic {
	var out []lsp.Diagnostic
	for _, dg := range a.lspDiags[lsp.Key(d.path)].diags {
		if dg.Range.Start.Line <= line && line <= dg.Range.End.Line {
			out = append(out, dg)
			if len(out) >= 20 {
				break
			}
		}
	}
	return out
}

// fixKey takes the keys the list of fixes uses, before the editor's own.
func (a *app) fixKey(d *editor, ev ui.InputEvent) bool {
	f := &a.fix
	if !f.open || f.doc != d.id {
		return false
	}
	switch ev.Kind {
	case ui.InputKeyDown:
		switch ev.Key {
		case ui.KeyEscape:
			a.closeFix()
			return true
		case ui.KeyUp:
			f.sel = max(f.sel-1, 0)
			a.invalidate()
			return true
		case ui.KeyDown:
			f.sel = min(f.sel+1, len(f.actions)-1)
			a.invalidate()
			return true
		case ui.KeyEnter:
			if ev.Mods == 0 {
				a.applyFix(d)
				return true
			}
		}
	case ui.InputPointerDown:
		a.closeFix()
	}
	return false
}

// applyFix makes the change chosen, in place of the problems.
func (a *app) applyFix(d *editor) {
	f := &a.fix
	if !f.open || len(f.actions) == 0 {
		a.closeFix()
		return
	}
	act := f.actions[min(f.sel, len(f.actions)-1)]
	edits := a.fixEdits(d, act.Edit)
	a.closeFix()
	if len(edits) == 0 {
		return
	}
	last := 0
	for _, e := range edits {
		last = max(last, e.start+utf8.RuneCountInString(e.text))
	}
	last = min(last, d.buf.Snapshot().Len())
	d.edit([2]int{last, last}, edits...)
}

// fixEdits returns a change's edits of the document shown, as the edits of
// the app: the edits of other documents wait for their tabs.
func (a *app) fixEdits(d *editor, edit *lsp.WorkspaceEdit) []textEdit {
	if edit == nil {
		return nil
	}
	var raw []lsp.TextEdit
	want := lsp.URI(d.path)
	for uri, edits := range edit.Changes {
		if uri == want || lsp.Key(lsp.Path(uri)) == lsp.Key(d.path) {
			raw = append(raw, edits...)
		}
	}
	for _, dc := range edit.DocumentChanges {
		if dc.TextDocument.URI == want || lsp.Key(lsp.Path(dc.TextDocument.URI)) == lsp.Key(d.path) {
			raw = append(raw, dc.Edits...)
		}
	}
	snap := d.buf.Snapshot()
	out := make([]textEdit, 0, len(raw))
	for _, e := range raw {
		start := lspOffset(snap, e.Range.Start)
		end := lspOffset(snap, e.Range.End)
		if end < start || start < 0 || start > snap.Len() {
			continue
		}
		out = append(out, textEdit{start: start, end: min(end, snap.Len()), text: e.NewText})
	}
	return out
}

// lspOffset returns where a position of the language server is, as a rune
// offset.
func lspOffset(snap ui.TextSnapshot, pos lsp.Position) int {
	line := min(max(pos.Line, 0), snap.LineCount()-1)
	return characterOffset(snap, line, pos.Character)
}

// fixView shows the fixes the language server offers, under the caret.
func (a *app) fixView(c *ui.Context, d *editor, lineTop func(int) float32, textX, lineH float32, view ui.Rect) {
	f := &a.fix
	if !f.open || f.doc != d.id {
		return
	}
	p := a.pal
	snap := d.buf.Snapshot()
	caret := min(d.sel[1], snap.Len())
	caretLine := snap.LineAt(caret)
	s, _ := snap.LineRange(caretLine)
	caretX := textX + a.textWidth(snap.Slice(s, caret))
	left := max(d.hscroll.X+caretX, 8)
	if view.W > 344 {
		left = min(left, d.hscroll.X+view.W-336)
	}
	height := float32(len(f.actions))*24 + 8
	y := lineTop(caretLine) + lineH + 2
	if view.H > 0 && y+height > view.H {
		y = lineTop(caretLine) - height - 2
	}
	ui.Column(c.Key("quickfix")).Absolute().Left(left).Top(max(y, 4)).
		Width(320).Padding(0, 4).Gap(0).Background(p.panel).Border(1, p.border).Radius(6).
		Shadow(0, 8, 24, 0, p.shadow).Children(func() {
		for i, act := range f.actions {
			on := i == min(f.sel, len(f.actions)-1)
			row := ui.ButtonBase(c.Key("quickfix:"+strconv.Itoa(i))).Height(24).Padding(0, 8).
				AlignItems(ui.Center).Radius(3).Label(act.Title)
			if on {
				row.Background(p.active)
			} else if row.Hovered() {
				row.Background(p.hover)
			}
			row.Children(func() {
				ui.Text(c, act.Title).FontSize(12).TextColor(p.text).SingleLine().Shrink(1).Grow(1).MinWidth(0)
			})
			if row.Hovered() {
				f.sel = i
			}
			if row.Clicked() {
				f.sel = i
				a.post(func() { a.applyFix(d) })
			}
		}
	})
}
