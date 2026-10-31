package main

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/lsp"
	"mayoi/internal/settings"
)

func TestFoldRangeAt(t *testing.T) {
	src := "func f() {\n\tx := 1\n\tif x > 0 {\n\t\tx++\n\t}\n\treturn x\n}\n"
	snap := ui.NewTextBuffer(src).Snapshot()
	if f, ok := foldRangeAt(snap, 0, 4); !ok || f.start != 0 || f.end != 6 {
		t.Errorf("func: %+v, %v, want 0..6", f, ok)
	}
	if f, ok := foldRangeAt(snap, 2, 4); !ok || f.start != 2 || f.end != 4 {
		t.Errorf("if: %+v, %v, want 2..4", f, ok)
	}
	if _, ok := foldRangeAt(snap, 5, 4); ok {
		t.Error("return folds, want not")
	}

	py := "def add(a, b):\n    return a + b\n\nafter = 1\n"
	snap = ui.NewTextBuffer(py).Snapshot()
	if f, ok := foldRangeAt(snap, 0, 4); !ok || f.start != 0 || f.end != 1 {
		t.Errorf("def: %+v, %v, want 0..1", f, ok)
	}

	imp := "package p\n\n// p does things.\n\n\nimport (\n\t\"context\"\n\t\"net/http\"\n\t\"os\"\n\t\"fmt\"\n\t\"strings\"\n)\n"
	snap = ui.NewTextBuffer(imp).Snapshot()
	if f, ok := foldRangeAt(snap, 5, 4); !ok || f.start != 5 || f.end != 11 {
		t.Errorf("import: %+v, %v, want 5..11", f, ok)
	}
}

func TestFoldGeometry(t *testing.T) {
	g := geometry{lineH: 20, n: 7, folds: []foldRange{{start: 2, end: 4}}}
	if got := g.top(5); got != 60 {
		t.Errorf("top 5: %v, want 60", got)
	}
	if got := g.height(); got != 100 {
		t.Errorf("height: %v, want 100", got)
	}
	if got := g.at(60); got != 5 {
		t.Errorf("at 60: %v, want 5", got)
	}
	if got := g.at(40); got != 2 {
		t.Errorf("at 40: %v, want 2", got)
	}
	if got := g.lineRow(2); got != 2 {
		t.Errorf("row 2: %v, want 2", got)
	}
	if got := g.lineRow(3); got != 5 {
		t.Errorf("row 3: %v, want 5", got)
	}
}

func TestFoldUnion(t *testing.T) {
	in := []foldRange{{start: 1, end: 10}, {start: 3, end: 5}, {start: 7, end: 8}, {start: 12, end: 14}}
	out := foldUnion(in)
	if len(out) != 2 || out[0] != (foldRange{start: 1, end: 10}) || out[1] != (foldRange{start: 12, end: 14}) {
		t.Errorf("union %+v, want [{1 10} {12 14}]", out)
	}
	if in[0].end != 10 {
		t.Errorf("union changed the folds: %+v", in)
	}
}

func TestFoldDepth(t *testing.T) {
	cands := []foldRange{{start: 0, end: 9}, {start: 3, end: 5}, {start: 7, end: 8}, {start: 11, end: 13}}
	for i, want := range []int{0, 1, 1, 0} {
		if got := foldDepth(cands, cands[i]); got != want {
			t.Errorf("depth of %+v = %d, want %d", cands[i], got, want)
		}
	}
}

func TestFoldRangesOf(t *testing.T) {
	got := foldRangesOf([]lsp.FoldingRange{
		{StartLine: 0, EndLine: 4},
		{StartLine: 0, EndLine: 2},
		{StartLine: 3, EndLine: 3},
		{StartLine: 4, EndLine: 99},
		{StartLine: -1, EndLine: 2},
	}, 10)
	if len(got) != 2 || got[0] != (foldRange{start: 0, end: 4}) || got[1] != (foldRange{start: 4, end: 9}) {
		t.Errorf("fold ranges %+v, want [{0 4} {4 9}]", got)
	}
}

func TestFoldFollowsEdits(t *testing.T) {
	hs, d := editHarness(t, "a.go", "package p\n\nfunc f() {\n\tx := 1\n\tx++\n}\n")
	hs.a.toggleFold(d, 2, d.buf.Snapshot())
	if _, ok := d.foldStarting(2); !ok {
		t.Fatalf("folds %+v, want one at line 2", d.folds)
	}
	hs.place(d, 0, 0)
	hs.key(0, ui.KeyEnter)
	// The line above shifted the fold down.
	if f, ok := d.foldStarting(3); !ok || f.end != 6 {
		t.Fatalf("folds %+v after a line above, want 3..6", d.folds)
	}
	// The start line itself changed: the fold stays, its text anew.
	snap := d.buf.Snapshot()
	_, e := snap.LineRange(3)
	hs.place(d, e, e)
	hs.tt.Type(" // f")
	hs.settle()
	if f, ok := d.foldStarting(3); !ok || f.end != 6 {
		t.Fatalf("folds %+v after an edit of the start line, want 3..6", d.folds)
	}
}

func TestFoldLevelsNest(t *testing.T) {
	hs, d := editHarness(t, "a.go", "package p\n\nfunc a() {\n\tif x {\n\t\ty()\n\t}\n}\n\nfunc b() {\n}\n")
	hs.a.foldLevel(1)
	if starts := foldStarts(d); !slices.Equal(starts, []int{2, 8}) {
		t.Fatalf("level 1 folds %v, want 2 and 8", starts)
	}
	hs.a.foldLevel(2)
	if starts := foldStarts(d); !slices.Equal(starts, []int{2, 3, 8}) {
		t.Fatalf("level 2 folds %v, want 2, 3 and 8", starts)
	}
	// Unfolding the outer leaves the inner folded, as VS Code does.
	hs.a.toggleFold(d, 2, d.buf.Snapshot())
	if starts := foldStarts(d); !slices.Equal(starts, []int{3, 8}) {
		t.Fatalf("after unfolding the func, folds are %v", starts)
	}
}

func foldStarts(d *editor) []int {
	out := make([]int, len(d.folds))
	for i, f := range d.folds {
		out[i] = f.start
	}
	return out
}

func TestFoldWhileWrapped(t *testing.T) {
	hs, d := editHarness(t, "a.go", "func f() {\n\tx := 1\n\ty := 2\n}\n")
	hs.a.settings.WordWrap, hs.a.settings.WrapGen = true, hs.a.settings.WrapGen+1
	hs.a.invalidate()
	hs.settle()
	hs.a.toggleFold(d, 0, d.buf.Snapshot())
	if f, ok := d.foldStarting(0); !ok || f.end != 3 {
		t.Fatalf("folded %+v, %v while wrapped", f, ok)
	}
}

func TestToggleFoldKeepsNested(t *testing.T) {
	hs, d := editHarness(t, "a.go", "func f() {\n\tif x > 0 {\n\t\tx++\n\t}\n}\n")
	snap := d.buf.Snapshot()
	hs.a.toggleFold(d, 1, snap)
	if len(d.folds) != 1 || d.folds[0].start != 1 {
		t.Fatalf("folds after folding the if: %+v", d.folds)
	}
	hs.a.toggleFold(d, 0, snap)
	if len(d.folds) != 2 {
		t.Fatalf("folds after folding the func: %+v, want the if inside it kept", d.folds)
	}
	hs.a.toggleFold(d, 0, snap)
	if len(d.folds) != 1 || d.folds[0].start != 1 {
		t.Fatalf("folds after unfolding the func: %+v, want the if folded", d.folds)
	}
}

func TestRefreshFoldsKeepsFoldsAsLinesStay(t *testing.T) {
	a := &app{settings: settings.Default()}
	doc := newDocument(1, "a.go", "func f() {\n\tx := 1\n}\n", nil)
	d := newEditor(doc)
	snap := doc.buf.Snapshot()
	d.setFolds([]foldRange{{start: 0, end: 2, sig: "func f() {"}})
	d.foldKind, d.foldLines = foldEvery, snap.LineCount()
	d.foldTab = tabSizeOf(a.settings.Prefs)
	// A key changes a line, not the number of them: the fold stays.
	s, _ := snap.LineRange(1)
	doc.buf.Replace(s, s+1, "\t ")
	a.refreshFolds(d, doc.buf.Snapshot())
	if len(d.folds) != 1 || d.folds[0].start != 0 || d.folds[0].end != 2 {
		t.Fatalf("folds after typing: %+v, want 0..2", d.folds)
	}

	// A line comes inside the block: the fold is found again, its end with
	// it.
	snap = doc.buf.Snapshot()
	s, _ = snap.LineRange(1)
	doc.buf.Replace(s, s, "extra\n")
	a.refreshFolds(d, doc.buf.Snapshot())
	if len(d.folds) != 1 || d.folds[0].start != 0 || d.folds[0].end != 3 {
		t.Fatalf("folds after a line came: %+v, want 0..3", d.folds)
	}
}
