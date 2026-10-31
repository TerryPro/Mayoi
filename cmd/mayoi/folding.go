package main

import (
	"slices"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/settings"
)

// foldRange is a fold of an editor: the lines after start through end are
// hidden, and start stays, with a marker. sig is the text of its start
// line, which finds it again after the lines above it change.
type foldRange struct {
	start, end int
	sig        string
}

// foldKind is how the folds of an editor were made: one at a time, all of
// them, or up to a level, which the next frame makes again as the text
// changes.
type foldKind int

const (
	foldCustom foldKind = iota // a fold toggled, or folded back, one at a time
	foldEvery                  // every fold, by Fold All
	foldLevels                 // the folds up to a level, by Fold Level
)

// foldHit is what folding a line hides, as the cache of it keeps it.
type foldHit struct {
	end int
	ok  bool
}

// maxFoldCandidates is the most folds folding all of a document makes, for
// a file too long to fold whole.
const maxFoldCandidates = 4096

// maxFoldScan is how far a fold looks for the line that ends its block. A
// block longer than this folds not, so that a file that never closes one
// is not read whole on each line.
const maxFoldScan = 5000

// foldFew is how few folds are found anew as the text changes: past that,
// typing keeps them where they are, rather than reading each block again
// on each key.
const foldFew = 8

// foldUnion returns the folds as ranges that do not overlap: the lines a
// fold inside another hides are the outer's anyway.
func foldUnion(folds []foldRange) []foldRange {
	if len(folds) < 2 {
		return folds
	}
	out := make([]foldRange, 0, len(folds))
	for _, f := range folds {
		if n := len(out); n > 0 && f.start <= out[n-1].end {
			out[n-1].end = max(out[n-1].end, f.end)
			continue
		}
		out = append(out, foldRange{start: f.start, end: f.end})
	}
	return out
}

// setFolds sets the folds of the editor, which hide the lines.
func (d *editor) setFolds(folds []foldRange) {
	d.folds, d.foldGen = folds, d.foldGen+1
}

// hides returns the folds as ranges that do not overlap, the union of
// them, which hides the same lines and places the rows below them.
func (d *editor) hides() []foldRange {
	if d.hideGen != d.foldGen {
		d.hide, d.hideGen = foldUnion(d.folds), d.foldGen
	}
	return d.hide
}

// foldRangeFor returns what folding line hides: the language server's fold
// for the text, or else the block of an opening bracket ending its code, or
// the lines indented under it.
func (a *app) foldRangeFor(d *editor, snap ui.TextSnapshot, line int) (foldRange, bool) {
	if line < 0 || line >= snap.LineCount()-1 {
		return foldRange{}, false
	}
	if f, ok := d.lspFoldAt(snap, line); ok {
		return f, true
	}
	return d.heuristicFold(snap, line, tabSizeOf(a.settings.Prefs))
}

// lspFoldAt returns the fold a language server found starting at line, of
// the text the folds are of.
func (d *editor) lspFoldAt(snap ui.TextSnapshot, line int) (foldRange, bool) {
	if len(d.lspFolds) == 0 || d.lspFoldVersion != snap.Version() {
		return foldRange{}, false
	}
	for _, f := range d.lspFolds {
		if f.start == line {
			return f, true
		}
		if f.start > line {
			break
		}
	}
	return foldRange{}, false
}

// heuristicFold returns what folding line hides by the brackets and the
// indentation of its code, as the text's version caches it.
func (d *editor) heuristicFold(snap ui.TextSnapshot, line, tab int) (foldRange, bool) {
	if d.foldHitsVersion != snap.Version() || d.foldHits == nil {
		d.foldHits, d.foldHitsVersion = map[int]foldHit{}, snap.Version()
	}
	if hit, ok := d.foldHits[line]; ok {
		if hit.ok {
			return foldRange{start: line, end: hit.end}, true
		}
		return foldRange{}, false
	}
	f, ok := foldRangeAt(snap, line, tab)
	d.foldHits[line] = foldHit{end: f.end, ok: ok}
	return f, ok
}

// folded reports whether line is hidden by a fold.
func (d *editor) folded(line int) bool {
	hides := d.hides()
	i := sort.Search(len(hides), func(i int) bool { return hides[i].end >= line })
	return i < len(hides) && line > hides[i].start && line <= hides[i].end
}

// foldStarting returns the fold starting at line.
func (d *editor) foldStarting(line int) (foldRange, bool) {
	i := sort.Search(len(d.folds), func(i int) bool { return d.folds[i].start >= line })
	if i < len(d.folds) && d.folds[i].start == line {
		return d.folds[i], true
	}
	return foldRange{}, false
}

// foldHolding returns the fold holding line, as a line of its own counts.
func (d *editor) foldHolding(line int) (foldRange, bool) {
	for _, f := range d.folds {
		if line >= f.start && line <= f.end {
			return f, true
		}
	}
	return foldRange{}, false
}

// uiFolds is the editor's folds as its text area hides them.
func (d *editor) uiFolds() []ui.Fold {
	hides := d.hides()
	if len(hides) == 0 {
		return nil
	}
	out := make([]ui.Fold, len(hides))
	for i, f := range hides {
		out[i] = ui.Fold{Start: f.start, End: f.end}
	}
	return out
}

// maxSavedFolds is the most folds a tab keeps for its next session, for a
// file too long to fold whole.
const maxSavedFolds = 256

// savedFolds are the folds of an editor as the settings keep them, nil for
// too many to keep.
func savedFolds(d *editor) []settings.Fold {
	if len(d.folds) == 0 || len(d.folds) > maxSavedFolds {
		return nil
	}
	out := make([]settings.Fold, len(d.folds))
	for i, f := range d.folds {
		out[i] = settings.Fold{Start: f.start, End: f.end, Sig: f.sig}
	}
	return out
}

// restoreFolds puts back the folds a tab kept, as the text finds them.
func (a *app) restoreFolds(d *editor, saved []settings.Fold) {
	if len(saved) == 0 {
		return
	}
	snap := d.buf.Snapshot()
	n := snap.LineCount()
	var folds []foldRange
	used := map[int]bool{}
	for _, f := range saved {
		line, found := f.Start, false
		if f.Sig != "" {
			if l := findFoldStart(snap, f.Start, f.Sig, used); l >= 0 {
				line, found = l, true
			}
		}
		if line < 0 || line >= n-1 || used[line] {
			continue
		}
		end := min(f.End, n-1)
		if !found {
			// The start line's text changed: it folds only as it does now.
			r, ok := a.foldRangeFor(d, snap, line)
			if !ok {
				continue
			}
			end = r.end
		}
		if end <= line {
			continue
		}
		folds = append(folds, foldRange{start: line, end: end, sig: foldSig(snap, line)})
		used[line] = true
	}
	if len(folds) == 0 {
		return
	}
	slices.SortFunc(folds, func(x, y foldRange) int { return x.start - y.start })
	d.setFolds(folds)
	d.foldKind, d.foldVersion = foldCustom, snap.Version()
	d.foldLines, d.foldTab = snap.LineCount(), tabSizeOf(a.settings.Prefs)
}

// foldRangeAt returns what folding line hides, and whether it folds: the
// block of an opening bracket ending its code, or the lines indented under
// it.
func foldRangeAt(snap ui.TextSnapshot, line, tab int) (foldRange, bool) {
	if line < 0 || line >= snap.LineCount()-1 {
		return foldRange{}, false
	}
	text := lineText(snap, line)
	if strings.TrimSpace(text) == "" {
		return foldRange{}, false
	}
	code := strings.TrimRight(stripComment(text), " \t\r")
	if len(code) > 0 {
		if open := code[len(code)-1]; open == '{' || open == '[' || open == '(' {
			if end, ok := matchFoldBracket(snap, line, text, open); ok && end > line {
				return foldRange{start: line, end: end}, true
			}
		}
	}
	base := indentColumns(text, tab)
	end := line
	for i := line + 1; i < snap.LineCount() && i <= line+maxFoldScan; i++ {
		t := lineText(snap, i)
		if strings.TrimSpace(t) == "" {
			continue
		}
		if indentColumns(t, tab) <= base {
			break
		}
		end = i
	}
	if end > line {
		return foldRange{start: line, end: end}, true
	}
	return foldRange{}, false
}

// matchBracket returns the line closing the bracket that ends the code of
// line, from the bracket at its end, past the brackets of strings and
// comments.
func matchFoldBracket(snap ui.TextSnapshot, line int, text string, open byte) (int, bool) {
	stack := []byte{open}
	inBlock := false
	inString := byte(0)
	start := strings.LastIndexByte(strings.TrimRight(stripComment(text), " \t\r"), open)
	for i := line; i < snap.LineCount() && i <= line+maxFoldScan; i++ {
		s := lineText(snap, i)
		from := 0
		if i == line {
			if start < 0 {
				return 0, false
			}
			from = start + 1
		}
		for j := from; j < len(s); j++ {
			c := s[j]
			if inBlock {
				if c == '*' && j+1 < len(s) && s[j+1] == '/' {
					inBlock, j = false, j+1
				}
				continue
			}
			if inString != 0 {
				if c == '\\' {
					j++
				} else if c == inString {
					inString = 0
				}
				continue
			}
			switch {
			case c == '/' && j+1 < len(s) && s[j+1] == '/':
				j = len(s)
			case c == '/' && j+1 < len(s) && s[j+1] == '*':
				inBlock, j = true, j+1
			case c == '#':
				j = len(s)
			case c == '\'' || c == '"' || c == '`':
				inString = c
			case c == '{' || c == '[' || c == '(':
				stack = append(stack, c)
			case c == '}' || c == ']' || c == ')':
				if len(stack) == 0 {
					continue
				}
				if matching(stack[len(stack)-1], c) {
					stack = stack[:len(stack)-1]
				}
			}
		}
		if len(stack) == 0 {
			return i, true
		}
	}
	return 0, false
}

// matching reports whether close closes open.
func matching(open, close byte) bool {
	return open == '{' && close == '}' || open == '[' && close == ']' || open == '(' && close == ')'
}

// lineText returns the text of a line, without its newline.
func lineText(snap ui.TextSnapshot, line int) string {
	s, e := snap.LineRange(line)
	return snap.Slice(s, e)
}

// stripComment returns the code of a line, before its // or # comment
// outside a string.
func stripComment(s string) string {
	inString := byte(0)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inString != 0 {
			if c == '\\' {
				i++
			} else if c == inString {
				inString = 0
			}
			continue
		}
		switch {
		case c == '\'' || c == '"' || c == '`':
			inString = c
		case c == '/' && i+1 < len(s) && s[i+1] == '/', c == '#':
			return s[:i]
		}
	}
	return s
}

// indentColumns counts a line's indentation, with its tabs to the next
// stop.
func indentColumns(s string, tab int) int {
	if tab <= 0 {
		tab = 4
	}
	col := 0
	for _, r := range s {
		switch r {
		case ' ':
			col++
		case '\t':
			col += tab - col%tab
		default:
			return col
		}
	}
	return col
}

// trackFoldHover notes the line of the gutter the pointer is on, for its
// fold marker.
func (a *app) trackFoldHover(d *editor, ev ui.InputEvent) {
	if ev.Kind != ui.InputPointerMove && ev.Kind != ui.InputPointerDown {
		return
	}
	gx := ev.X - d.hscroll.X
	if d.gutterW <= 0 || gx < d.gutterW-18 || gx > d.gutterW+2 {
		if d.foldGutter || d.foldHover != -1 {
			// Off the markers, they show only where a fold is.
			d.foldGutter, d.foldHover = false, -1
			a.invalidate()
		}
		return
	}
	snap := d.buf.Snapshot()
	line := a.geometry(d, snap.LineCount()).at(ev.Y + d.vscroll.Y - editorPadY)
	line = min(max(line, 0), snap.LineCount()-1)
	if d.folded(line) {
		line = -1
	} else if _, ok := a.foldRangeFor(d, snap, line); !ok {
		// Only a line that folds shows its own marker under the pointer.
		line = -1
	}
	if !d.foldGutter || d.foldHover != line {
		d.foldGutter, d.foldHover = true, line
		a.invalidate()
	}
}

// foldAtCaret folds the block at the caret, or unfolds it.
func (a *app) foldAtCaret() {
	d := a.activeEditor()
	if d == nil {
		return
	}
	snap := d.buf.Snapshot()
	a.toggleFold(d, snap.LineAt(min(d.caret, snap.Len())), snap)
}

// toggleFold folds or unfolds the fold starting at line.
func (a *app) toggleFold(d *editor, line int, snap ui.TextSnapshot) {
	if _, ok := d.foldStarting(line); ok {
		d.setFolds(slices.DeleteFunc(d.folds, func(f foldRange) bool { return f.start == line }))
		d.foldKind, d.foldVersion = foldCustom, snap.Version()
		a.invalidate()
		return
	}
	f, ok := a.foldRangeFor(d, snap, line)
	if !ok {
		return
	}
	f.sig = foldSig(snap, f.start)
	// Folds inside the new one stay folded inside it, as VS Code keeps
	// them; the others that meet it go.
	folds := slices.DeleteFunc(d.folds, func(o foldRange) bool {
		if o.start >= f.start && o.end <= f.end {
			return false
		}
		return o.start <= f.end && f.start <= o.end
	})
	folds = append(folds, f)
	slices.SortFunc(folds, func(x, y foldRange) int { return x.start - y.start })
	d.setFolds(folds)
	d.foldKind, d.foldVersion = foldCustom, snap.Version()
	a.clampFoldSelection(d)
	a.invalidate()
}

// unfoldFold unfolds the fold starting at line, and those holding it.
func (a *app) unfoldFold(d *editor, line int) {
	if len(d.folds) == 0 {
		return
	}
	snap := d.buf.Snapshot()
	d.setFolds(slices.DeleteFunc(d.folds, func(f foldRange) bool {
		return f.start == line || line > f.start && line <= f.end
	}))
	d.foldKind, d.foldVersion = foldCustom, snap.Version()
	a.invalidate()
}

// unfoldAtCaret unfolds the fold at the caret.
func (a *app) unfoldAtCaret() {
	d := a.activeEditor()
	if d == nil {
		return
	}
	snap := d.buf.Snapshot()
	a.unfoldFold(d, snap.LineAt(min(d.caret, snap.Len())))
}

// foldAll folds every fold of the editor.
func (a *app) foldAll() {
	d := a.activeEditor()
	if d == nil {
		return
	}
	a.foldAllIn(d)
}

// foldAllIn folds every fold of the editor, those inside others included,
// as VS Code keeps them folded inside.
func (a *app) foldAllIn(d *editor) {
	snap := d.buf.Snapshot()
	d.setFolds(a.foldCandidates(d, snap))
	d.foldKind, d.foldLevel, d.foldVersion = foldEvery, 0, snap.Version()
	d.foldLines, d.foldTab = snap.LineCount(), tabSizeOf(a.settings.Prefs)
	a.clampFoldSelection(d)
	a.invalidate()
}

// foldLevel folds the folds of the editor up to level: the outermost for
// 1, and those of each level inside them for more.
func (a *app) foldLevel(level int) {
	d := a.activeEditor()
	if d == nil {
		return
	}
	a.foldLevelIn(d, level)
}

// foldLevelIn folds the folds of the editor up to level.
func (a *app) foldLevelIn(d *editor, level int) {
	snap := d.buf.Snapshot()
	cands := a.foldCandidates(d, snap)
	var folds []foldRange
	for _, f := range cands {
		if foldDepth(cands, f) < max(level, 1) {
			folds = append(folds, f)
		}
	}
	d.setFolds(folds)
	d.foldKind, d.foldLevel, d.foldVersion = foldLevels, level, snap.Version()
	d.foldLines, d.foldTab = snap.LineCount(), tabSizeOf(a.settings.Prefs)
	a.clampFoldSelection(d)
	a.invalidate()
}

// foldCandidates returns the folds the editor's text makes, by their start
// line: the language server's, and for the lines it did not name, those of
// the brackets and the indentation.
func (a *app) foldCandidates(d *editor, snap ui.TextSnapshot) []foldRange {
	var out []foldRange
	for line := 0; line < snap.LineCount()-1 && len(out) < maxFoldCandidates; line++ {
		f, ok := a.foldRangeFor(d, snap, line)
		if !ok {
			continue
		}
		f.sig = foldSig(snap, f.start)
		out = append(out, f)
	}
	return out
}

// foldDepth returns how many of the folds hold the start line of one: how
// deep it nestles.
func foldDepth(folds []foldRange, f foldRange) int {
	depth := 0
	for _, o := range folds {
		if o.start < f.start && f.start <= o.end {
			depth++
		}
	}
	return depth
}

// unfoldAll unfolds every fold of the editor.
func (a *app) unfoldAll() {
	d := a.activeEditor()
	if d == nil {
		return
	}
	a.unfoldAllIn(d)
}

// unfoldAllIn unfolds every fold of the editor.
func (a *app) unfoldAllIn(d *editor) {
	d.foldKind = foldCustom
	if len(d.folds) == 0 {
		return
	}
	d.setFolds(nil)
	d.foldVersion = d.buf.Snapshot().Version()
	d.foldLines, d.foldTab = d.buf.Snapshot().LineCount(), tabSizeOf(a.settings.Prefs)
	a.invalidate()
}

// foldSig is the text a fold's start line is found by.
func foldSig(snap ui.TextSnapshot, line int) string {
	return strings.TrimSpace(lineText(snap, line))
}

// refreshFolds keeps the folds of an editor as its text changed, finding
// each start line near where it was by its text, and its block anew.
//
// Typing changes the text without moving its lines: the folds of Fold All
// and Fold Level then stay where they are, as their start lines do, rather
// than the whole document being folded again on each key.
func (a *app) refreshFolds(d *editor, snap ui.TextSnapshot) {
	if d.foldVersion == snap.Version() {
		return
	}
	tab := tabSizeOf(a.settings.Prefs)
	retab := d.foldTab != tab
	if retab {
		// The folds were found with another tab width: their blocks are
		// found again, which the width changes.
		d.foldHits, d.foldTab = nil, tab
	}
	if !retab && len(d.folds) >= foldFew && d.foldLines == snap.LineCount() && d.foldsHold(snap) {
		d.foldVersion = snap.Version()
		return
	}
	var kept []foldRange
	used := map[int]bool{}
	for _, f := range d.folds {
		line := findFoldStart(snap, f.start, f.sig, used)
		if line < 0 && f.start < snap.LineCount() && !used[f.start] {
			// The start line's own text may have changed: it stays where it
			// is, if it still folds.
			line = f.start
		}
		if line < 0 {
			continue
		}
		r, ok := a.foldRangeFor(d, snap, line)
		if !ok {
			continue
		}
		// The line's text may have changed: its sig is what finds it the
		// next time.
		r.sig = foldSig(snap, r.start)
		kept = append(kept, r)
		used[r.start] = true
	}
	slices.SortFunc(kept, func(x, y foldRange) int { return x.start - y.start })
	d.setFolds(kept)
	d.foldVersion, d.foldLines = snap.Version(), snap.LineCount()
}

// foldsHold reports whether the start line of each fold is still the one
// it was: the folds stay where they are while the lines do not move.
func (d *editor) foldsHold(snap ui.TextSnapshot) bool {
	for _, f := range d.folds {
		if f.start < 0 || f.start >= snap.LineCount() || foldSig(snap, f.start) != f.sig {
			return false
		}
	}
	return true
}

// findFoldStart finds the line near line whose text is sig, unused.
func findFoldStart(snap ui.TextSnapshot, line int, sig string, used map[int]bool) int {
	if sig == "" {
		return -1
	}
	n := snap.LineCount()
	for d := 0; d <= 200; d++ {
		for _, i := range []int{line - d, line + d} {
			if i < 0 || i >= n || used[i] {
				continue
			}
			if foldSig(snap, i) == sig {
				return i
			}
			if d == 0 {
				break
			}
		}
	}
	return -1
}

// clampFoldSelection moves the caret and the selection out of the lines a
// fold hides, down past the fold or up to its start, as the caret moved.
func (a *app) clampFoldSelection(d *editor) {
	if len(d.folds) == 0 {
		d.foldCaretRow = -1
		return
	}
	snap := d.buf.Snapshot()
	if d.pending != nil {
		// A jump to a line a fold hides, as a search's or a problem's,
		// unfolds it.
		folds := d.folds
		for _, off := range d.pending {
			line := snap.LineAt(min(max(off, 0), snap.Len()))
			folds = slices.DeleteFunc(folds, func(f foldRange) bool {
				return line > f.start && line <= f.end
			})
		}
		if len(folds) != len(d.folds) {
			d.setFolds(folds)
			d.foldKind = foldCustom
			a.invalidate()
		}
	}
	caretLine := snap.LineAt(min(max(d.caret, 0), snap.Len()))
	down := d.foldCaretRow >= 0 && caretLine > d.foldCaretRow
	fix := func(off int, down bool) int {
		line := snap.LineAt(min(max(off, 0), snap.Len()))
		f, ok := d.foldHolding(line)
		if !ok || line <= f.start {
			// A fold's start line shows, with its marker.
			return off
		}
		if down && f.end+1 < snap.LineCount() {
			s, _ := snap.LineRange(f.end + 1)
			return s
		}
		_, e := snap.LineRange(f.start)
		return e
	}
	// The caret decides where a selection in hidden lines goes.
	d.sel[1] = fix(d.sel[1], down)
	d.sel[0] = fix(d.sel[0], down)
	d.caret = fix(d.caret, down)
	if d.pending != nil {
		d.pending[1] = fix(d.pending[1], down)
		d.pending[0] = fix(d.pending[0], down)
	}
	d.foldCaretRow = snap.LineAt(min(max(d.caret, 0), snap.Len()))
}
