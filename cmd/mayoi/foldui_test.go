package main

import (
	"testing"

	"mayoi/locales"
)

// TestFoldGutterHoverAndClicks checks that the fold markers show while the
// pointer is over the gutter, that a click folds the block with its badge,
// and that a click on the badge unfolds it.
func TestFoldGutterHoverAndClicks(t *testing.T) {
	hs, d := editHarness(t, "a.go", "func a() {\n\tone()\n\ttwo()\n}\n\nfunc b() {\n\tthree()\n}\n")
	hs.settle()
	r, ok := hs.tt.Find(locales.EditorLabel(d.name()))
	if !ok {
		t.Fatal("no editor")
	}
	lineH := hs.a.font.lineH
	g := hs.a.geometry(d, d.buf.Snapshot().LineCount())
	lineY := func(line int) float32 {
		return r.Y + editorPadY + g.top(line) - d.vscroll.Y + lineH/2
	}
	barX := r.X + d.hscroll.X + d.gutterW - 8
	hs.tt.Move(barX, lineY(0))
	hs.settle()
	if !d.foldGutter || d.foldHover != 0 {
		t.Fatalf("over the gutter: foldGutter %v, foldHover %d", d.foldGutter, d.foldHover)
	}
	hs.tt.Move(r.X+r.W-40, r.Y+r.H/2)
	hs.settle()
	if d.foldGutter || d.foldHover != -1 {
		t.Fatalf("off the gutter: foldGutter %v, foldHover %d", d.foldGutter, d.foldHover)
	}
	hs.tt.Move(barX, lineY(0))
	hs.tt.ClickAt(barX, lineY(0))
	hs.settle()
	f, ok := d.foldStarting(0)
	if !ok || f.end != 3 || !d.folded(1) {
		t.Fatalf("click on the marker folded %+v, %v", f, ok)
	}
	text := lineText(d.buf.Snapshot(), 0)
	badgeX := r.X + d.hscroll.X + d.gutterW + gutterGap + hs.a.font.width(text) + 4 + foldBadgeW/2
	hs.tt.ClickAt(badgeX, lineY(0))
	hs.settle()
	if _, ok := d.foldStarting(0); ok {
		t.Fatalf("click on the badge left %+v", d.folds)
	}
}
