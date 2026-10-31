package ui

import "slices"

// Fold is a run of lines a text area hides: the lines after Start through
// End, End included. Start stays, with whatever marker the app draws for
// it.
type Fold struct {
	Start, End int
}

// FoldLines has a text area hide the lines of folds, laying out the others
// as if they were alone: a folded block takes the height of its start
// line. The app moves the caret out of what a fold hides.
func (e Element) FoldLines(folds []Fold) Element {
	if n := e.node(); n != nil {
		n.folds = folds
	}
	return e
}

// setFolds hides the lines of folds in the text, and moves carets out of
// what they hide, to the end of the line folding them.
func (ed *editor) setFolds(folds []Fold) {
	if slices.Equal(ed.folds, folds) {
		return
	}
	ed.folds = slices.Clone(folds)
	ed.applyFolds()
	ed.caret, ed.anchor = ed.visible(ed.caret), ed.visible(ed.anchor)
}

// applyFolds marks the paragraphs the folds hide, whose layouts and heights
// go, and has the area measure the text anew.
func (ed *editor) applyFolds() {
	b := &ed.buf
	for i := range b.paras {
		p := &b.paras[i]
		p.hidden = false
		p.layout, p.h, p.tabs = nil, 0, nil
	}
	for _, f := range ed.folds {
		for i := max(0, f.Start+1); i <= f.End && i < len(b.paras); i++ {
			b.paras[i].hidden = true
		}
	}
	if ed.area != nil {
		ed.area.laid, ed.area.version = 0, 0
		ed.area.hs.reset(b.paras)
	}
}

// foldStart returns the fold hiding paragraph p, if one does.
func (ed *editor) foldStart(p int) (Fold, bool) {
	for _, f := range ed.folds {
		if p > f.Start && p <= f.End {
			return f, true
		}
	}
	return Fold{}, false
}

// visible returns off, or the end of the line folding its paragraph.
func (ed *editor) visible(off int) int {
	p := ed.buf.para(min(max(off, 0), ed.buf.n))
	if f, ok := ed.foldStart(p); ok {
		return ed.buf.end(f.Start)
	}
	return off
}
