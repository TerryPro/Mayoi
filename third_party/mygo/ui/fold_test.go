package ui

import "testing"

func TestFoldLinesHide(t *testing.T) {
	b := NewTextBuffer("a\nb\nc\nd\ne\n")
	tt := coreNewTester(func(c *context) {
		e := coreTextAreaBufferBase(c, b)
		e.Fill()
		e.folds = []Fold{{Start: 1, End: 3}}
	}, 300, 300)
	var ed *editor
	for _, st := range tt.rt.states {
		if st.editor != nil && st.editor.multiline {
			ed = st.editor
		}
	}
	if ed == nil {
		t.Fatal("no text area")
	}
	for i, want := range []bool{false, false, true, true, false, false} {
		if i < len(ed.buf.paras) && ed.buf.paras[i].hidden != want {
			t.Errorf("line %d hidden %v, want %v", i, ed.buf.paras[i].hidden, want)
		}
	}
	if ed.area == nil {
		t.Fatal("no area")
	}
	line := ed.area.hs.top(1) - ed.area.hs.top(0)
	if got, want := ed.area.hs.top(len(ed.buf.paras)), 4*line; got < want-1 || got > want+1 {
		t.Errorf("text is %v high, want %v: two lines hidden", got, want)
	}
}

// TestFoldLinesWrap checks that a line a fold hides takes no height as the
// text area wraps the others: the lines below it move up by its rows.
func TestFoldLinesWrap(t *testing.T) {
	tall := coreNewTester(func(c *context) {
		e := coreTextAreaBufferBase(c, NewTextBuffer("a long line that wraps over several rows here\nsecond\nthird\n"))
		e.Fill()
	}, 300, 200)
	tt := coreNewTester(func(c *context) {
		e := coreTextAreaBufferBase(c, NewTextBuffer("a long line that wraps over several rows here\nsecond\nthird\n"))
		e.Fill()
		e.folds = []Fold{{Start: 0, End: 1}}
	}, 300, 200)
	areaOf := func(tt *Tester) (*area, *editor) {
		var ed *editor
		for _, st := range tt.rt.states {
			if st.editor != nil && st.editor.multiline {
				ed = st.editor
			}
		}
		if ed == nil || ed.area == nil {
			t.Fatal("no text area")
		}
		return ed.area, ed
	}
	ua, _ := areaOf(tall)
	fa, fed := areaOf(tt)
	folded := fa.hs.top(2) - fa.hs.top(1)
	row := ua.hs.top(2) - ua.hs.top(1)
	if folded != 0 {
		t.Errorf("the line below a hidden one is %v below it, want 0", folded)
	}
	if got, want := fa.hs.top(len(fed.buf.paras)), ua.hs.top(len(fed.buf.paras))-row; got < want-1 || got > want+1 {
		t.Errorf("folded text is %v high, want %v", got, want)
	}
}
