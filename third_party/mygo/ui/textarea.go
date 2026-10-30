package ui

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/internal/scene"
	"github.com/egoist/mygo/internal/text"
)

// area lays out and shows the text of a TextArea a paragraph at a time, as
// a List does its rows: each paragraph keeps its layout until an edit
// changes it, only those in view are laid out, and the heights of the
// others are estimated (heights) until they are. Text laid out whole, as
// text elements and single-line inputs are, makes the same lines, since
// the text system breaks lines a paragraph at a time too.
//
// The area scrolls as a scroll container does (flagScrollY): the wheel and
// the scroll bar move the state's offset, which the area keeps by a
// paragraph and how far into it the view starts (anchor, anchorOff), so
// that the view stays put as the heights above it are measured.
type area struct {
	hs heights
	// tabSize is how many cells a tab spans, 0 until the area's tab size is
	// set, and 4 then.
	tabSize int
	// params are those of the layouts, without their text; version is
	// the buffer's the heights are of, and line a line of an empty text.
	params  text.Params
	version uint64
	line    text.Line

	scroll     float64 // the content's y at the top of the view
	anchor     int
	anchorOff  float64
	lastScroll float64 // the state's offset after the last layout
	// reveal scrolls the caret into view at the next layout, after an
	// edit, a move of the caret or a press.
	reveal bool
	// first and last are the paragraphs the view shows; laid counts those
	// with a layout.
	first, last int
	laid        int
	// wrapped is the height of the whole text wrapped at a width, for a text
	// area that grows with it (Lines), and the params it is of.
	wrapped        float32
	wrappedParams  text.Params
	wrappedVersion uint64
	wrappedCompose string
}

// wrappedHeight returns the height of the text, with an input method's
// composition, wrapped at the content width cw.
func (a *area) wrappedHeight(e *node, ed *editor, cw float32) float32 {
	params := e.textParams(max(cw, 1))
	if ed.buf.indexed {
		params.KeepSpaces, params.MaxLines = true, 0
		if params != a.wrappedParams || a.wrappedVersion != ed.buf.version || a.wrappedCompose != ed.compose {
			empty := params
			empty.Text = ""
			limit := float32(ed.lines[1]) * textSystem().Layout(empty).Lines[0].Height
			var height float32
			for p := 0; p < len(ed.buf.paras) && height < limit; p++ {
				part := params
				part.Text = ed.buf.text(p)
				if ed.compose != "" && p == ed.buf.para(ed.caret) {
					at := runeOffset(part.Text, ed.caret-ed.buf.start(p))
					part.Text = part.Text[:at] + ed.compose + part.Text[at:]
				}
				part.Spans = ed.rangeKey(ed.buf.start(p), ed.buf.end(p))
				height += textSystem().Shape(part).Height
			}
			a.wrapped, a.wrappedParams, a.wrappedVersion = height, params, ed.buf.version
			a.wrappedCompose = ed.compose
		}
		return a.wrapped
	}
	params.Text = ed.displayText()
	params.KeepSpaces, params.MaxLines = true, 0
	// Bold runs take more room, so the text may wrap sooner.
	params.Spans = ed.rangeKey(0, ed.buf.n)
	if params != a.wrappedParams {
		a.wrappedParams = params
		a.wrapped = textSystem().Layout(params).Height
	}
	return a.wrapped
}

// maxLaid is how many paragraphs keep their layouts out of view, beyond
// those in view: those further from it give theirs up, keeping their
// heights.
const maxLaid = 512

// edited follows an edit of the buffer: the paragraphs from first, which
// were old, are now after others, without a layout.
func (a *area) edited(b *buffer, first int, old []paragraph, after int) {
	for _, o := range old {
		if o.layout != nil {
			a.laid--
		}
	}
	if a.version+1 == b.version {
		a.version = b.version
		if len(old) == after {
			for k, o := range old {
				if o.h > 0 {
					a.hs.add(first+k, -float64(o.h), 1)
				}
			}
		} else {
			a.hs.reset(b.paras)
		}
	}
	switch {
	case a.anchor >= first+len(old):
		a.anchor += after - len(old)
	case a.anchor > first:
		a.anchor, a.anchorOff = first, 0
	}
	a.reveal = true
}

// sync gets the heights in step with the buffer and the layouts' params,
// which a new width or style changes.
func (a *area) sync(b *buffer, params text.Params) {
	if params != a.params {
		for i := range b.paras {
			b.paras[i].layout, b.paras[i].h = nil, 0
		}
		a.laid = 0
		a.params = params
		a.version = 0
		empty := params
		empty.Text = ""
		a.line = textSystem().Layout(empty).Lines[0]
	}
	if a.version != b.version || len(a.hs.measured) != len(b.paras)+1 {
		if a.version != b.version {
			a.laid = 0 // a new text: its paragraphs have no layouts
		}
		a.hs.reset(b.paras)
		a.version = b.version
		a.anchor = min(a.anchor, len(b.paras)-1)
	}
	m, unknown := a.hs.totals()
	if n := len(b.paras) - unknown; n > 0 {
		a.hs.est = m / float64(n)
	} else {
		a.hs.est = float64(a.line.Height)
	}
}

// tabSize is how many cells a tab spans, to the next stop.
const tabSize = 4

// tabStop is a tab of a paragraph: the rune it is, how many spaces its
// layout shows it as, and the rune of the layout it starts at.
type tabStop struct {
	at, n, disp int
}

// expandTabs returns s with its tabs as spaces, to the next stop of size,
// and the tabs they replaced.
func expandTabs(s string, size int) (string, []tabStop) {
	if !strings.ContainsRune(s, '\t') {
		return s, nil
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	var stops []tabStop
	col, ri, extra := 0, 0, 0
	for _, r := range s {
		if r == '\t' {
			n := size - col%size
			stops = append(stops, tabStop{at: ri, n: n, disp: ri + extra})
			b.WriteString(strings.Repeat(" ", n))
			col += n
			extra += n - 1
		} else {
			b.WriteRune(r)
			col++
		}
		ri++
	}
	return b.String(), stops
}

// extraBefore returns how many more runes the layout of a paragraph has
// before rune at, over the text's own.
func extraBefore(stops []tabStop, at int) int {
	extra := 0
	for _, s := range stops {
		if s.at >= at {
			break
		}
		extra += s.n - 1
	}
	return extra
}

// originalOf returns the rune of the text that the rune of the layout at
// l shows, the tab's own rune for a space of one.
func originalOf(stops []tabStop, l int) int {
	orig := l
	for _, s := range stops {
		switch {
		case l < s.disp:
			return orig
		case l < s.disp+s.n:
			return s.at
		}
		orig -= s.n - 1
	}
	return orig
}

// remapSpans moves the ends of the spans of a paragraph past its tabs,
// which its layout shows longer.
func remapSpans(key string, stops []tabStop) string {
	if key == "" || len(stops) == 0 {
		return key
	}
	spans := text.DecodeSpans(key)
	for i := range spans {
		spans[i].End += extraBefore(stops, spans[i].End)
	}
	return text.EncodeSpans(spans)
}

// paraLayout returns the layout of paragraph p, with the composition of an
// input method in the paragraph of the caret. Its tab stops are those the
// text area's lines have, so that a tab-indented document shows where its
// indentation says.
func (a *area) paraLayout(ed *editor, p int) *text.Layout {
	b := &ed.buf
	pr := &b.paras[p]
	compose := ""
	if ed.compose != "" && p == b.para(ed.caret) {
		compose = ed.compose
	}
	key := ed.rangeKey(b.start(p), b.end(p))
	if pr.layout != nil && pr.compose == compose && pr.spans == key {
		return pr.layout
	}
	t := b.text(p)
	if compose != "" {
		at := runeOffset(t, ed.caret-b.start(p))
		t = t[:at] + compose + t[at:]
	}
	size := a.tabSize
	if size <= 0 {
		size = 4
	}
	t, tabs := expandTabs(t, size)
	params := a.params
	// A paragraph's layout outlives edits elsewhere. Its text must not
	// pin the old document when the buffer replaces it with a new string.
	if !b.indexed && len(t) < b.byteLen() {
		t = strings.Clone(t)
	}
	params.Text, params.Spans = t, remapSpans(key, tabs)
	pr.tabs = tabs
	l := textSystem().Shape(params)
	if pr.layout == nil {
		a.laid++
	}
	if pr.h > 0 {
		a.hs.add(p, float64(l.Height-pr.h), 0)
	} else {
		a.hs.add(p, float64(l.Height), -1)
	}
	pr.layout, pr.compose, pr.spans, pr.h = l, compose, key, l.Height
	return l
}

// local returns where rune i of paragraph p is in its layout, which holds
// the composition after the caret.
func (a *area) local(ed *editor, p, i int) int {
	l := i - ed.buf.start(p)
	l += extraBefore(ed.buf.paras[p].tabs, l)
	if ed.compose != "" && i > ed.caret && p == ed.buf.para(ed.caret) {
		l += utf8.RuneCountInString(ed.compose)
	}
	return l
}

// global returns the rune of the text at index l of the layout of
// paragraph p.
func (a *area) global(ed *editor, p, l int) int {
	start := ed.buf.start(p)
	tabs := ed.buf.paras[p].tabs
	if ed.compose != "" && p == ed.buf.para(ed.caret) {
		n, c := utf8.RuneCountInString(ed.compose), ed.caret-start
		c += extraBefore(tabs, c)
		switch {
		case l > c+n:
			l -= n
		case l > c:
			l = c
		}
	}
	l = originalOf(tabs, l)
	return min(start+l, ed.buf.end(p))
}

// caretAt returns the caret before rune i, extra runes into a
// composition there: its x, top and height in the content.
func (a *area) caretAt(ed *editor, i, extra int) (x float32, y float64, h float32) {
	p := ed.buf.para(i)
	l := a.paraLayout(ed, p)
	affinity := text.Downstream
	if i == ed.caret {
		affinity = ed.caretAffinity
	}
	x, ly, h := l.CaretAt(text.CaretPosition{Index: a.local(ed, p, i) + extra, Affinity: affinity})
	return x, a.hs.top(p) + float64(ly), h
}

// lineEdges returns the edges of the visual line holding rune i.
func (a *area) lineEdges(ed *editor, i int) (int, int) {
	p := ed.buf.para(i)
	l := a.paraLayout(ed, p)
	line := &l.Lines[l.LineAt(a.local(ed, p, i))]
	return a.global(ed, p, line.Start), a.global(ed, p, line.End)
}

// firstLine returns the first line of the text, or a line of an empty text
// while the first paragraph has no layout.
func (a *area) firstLine(b *buffer) *text.Line {
	if l := b.paras[0].layout; l != nil {
		return &l.Lines[0]
	}
	return &a.line
}

// indexAt returns the rune of the caret position closest to (x, y) in the
// content.
func (a *area) indexAt(ed *editor, x float32, y float64) int {
	return a.positionAt(ed, x, y).Index
}

func (a *area) positionAt(ed *editor, x float32, y float64) text.CaretPosition {
	return a.hitPosition(ed, x, y, false)
}

func (a *area) documentPositionAt(ed *editor, x float32, y float64) text.CaretPosition {
	return a.hitPosition(ed, x, y, true)
}

func (a *area) hitPosition(ed *editor, x float32, y float64, virtual bool) text.CaretPosition {
	n := len(ed.buf.paras)
	y = max(y, 0)
	p := a.hs.at(y)
	// Laying the paragraph out may show it is not as high as estimated.
	for range n {
		l := a.paraLayout(ed, p)
		top := a.hs.top(p)
		switch {
		case y < top && p > 0:
			p--
		case y >= top+float64(l.Height) && p < n-1:
			p++
		default:
			position := l.PositionAt(x, float32(y-top))
			if virtual {
				position.Index += ed.buf.start(p)
				if ed.compose != "" && p > ed.buf.para(ed.caret) {
					position.Index += utf8.RuneCountInString(ed.compose)
				}
			} else {
				position.Index = a.global(ed, p, position.Index)
			}
			return position
		}
	}
	return text.CaretPosition{Index: ed.buf.n, Affinity: text.Upstream}
}

// setScroll scrolls the content to y, keeping the place by the paragraph
// there.
func (a *area) setScroll(y float64) {
	a.scroll = y
	a.anchor = a.hs.at(y)
	a.anchorOff = y - a.hs.top(a.anchor)
}

// layout lays the text area out in a content box of cw×ch: the
// paragraphs in view, the caret's when it is to be revealed, and where the
// view is.
func (a *area) layout(e *node, cw, ch float32) {
	ed := e.st.editor
	b := &ed.buf
	if e.tabSize > 0 && a.tabSize != e.tabSize {
		// A tab spans other cells now: the layouts that showed it are old.
		a.tabSize = e.tabSize
		for i := range b.paras {
			b.paras[i].layout, b.paras[i].tabs = nil, nil
		}
		a.laid, a.version = 0, 0
	}
	params := e.textParams(cw)
	params.KeepSpaces, params.MaxLines = true, 0
	a.sync(b, params)

	st := e.st
	if st.scrollY != a.lastScroll {
		a.setScroll(st.scrollY) // the wheel or the scroll bar
	}
	// The text shows through the padding, inside the border.
	above, below := e.pad[0], e.pad[2]
	a.place(ed, ch, above, below)
	if a.reveal {
		a.reveal = false
		_, y, h := a.caretAt(ed, ed.caret, ed.composeCaret)
		switch {
		case y < a.scroll:
			a.setScroll(y)
		case y+float64(h) > a.scroll+float64(ch):
			// Measure the paragraphs above the caret's that the view will
			// show, so that the caret ends at its bottom whatever their
			// heights were estimated.
			p := b.para(ed.caret)
			for q, room := p-1, y+float64(h)-a.hs.top(p); q >= 0 && room < float64(ch); q-- {
				room += float64(a.paraLayout(ed, q).Height)
			}
			_, y, h = a.caretAt(ed, ed.caret, ed.composeCaret)
			a.setScroll(y + float64(h) - float64(ch))
		}
		a.place(ed, ch, above, below)
	}
	total := a.hs.top(len(b.paras))
	if s := max(0, min(a.scroll, total-float64(ch))); s != a.scroll {
		a.setScroll(s)
		a.place(ed, ch, above, below)
		total = a.hs.top(len(b.paras))
	}
	st.scrollTo(st.scrollX, a.scroll)
	a.lastScroll = a.scroll
	e.contentW, e.contentH = float64(e.w), total+float64(e.padY())
	if a.laid > maxLaid+a.last-a.first {
		a.forget(b)
	}
}

// place lays out the paragraphs in view, from the anchor down, with those
// showing in the bands above and below the view, and keeps the view where
// the anchor is.
func (a *area) place(ed *editor, ch, above, below float32) {
	n := len(ed.buf.paras)
	a.anchor = max(0, min(a.anchor, n-1))
	a.paraLayout(ed, a.anchor)
	a.first = a.anchor
	for a.first > 0 && a.hs.top(a.anchor)+a.anchorOff-a.hs.top(a.first) < float64(above) {
		a.first--
		a.paraLayout(ed, a.first)
	}
	// The heights from the first paragraph shown are known now: the place
	// is by the paragraph the offset falls in.
	a.setScroll(max(0, a.hs.top(a.anchor)+a.anchorOff))
	y := a.hs.top(a.first)
	p := a.first
	for ; p < n; p++ {
		y += float64(a.paraLayout(ed, p).Height)
		if y >= a.scroll+float64(ch+below) {
			break
		}
	}
	a.last = min(p, n-1)
}

// forget drops the layouts of the paragraphs far from view, keeping their
// heights.
func (a *area) forget(b *buffer) {
	lo, hi := a.first-maxLaid/4, a.last+maxLaid/4
	for i := range b.paras {
		if (i < lo || i > hi) && b.paras[i].layout != nil {
			b.paras[i].layout = nil
			a.laid--
		}
	}
}

// paint paints the paragraphs in view, the selection, the composition and
// the caret, with the content's top at oy.
func (a *area) paint(e *node, p *Painter, ox, oy float32) {
	ed := e.st.editor
	b := &ed.buf
	t := e.c.theme
	ts := e.resolvedText()
	focused := e.Focused()
	last := len(b.paras) - 1
	for i := a.first; i <= a.last; i++ {
		l := a.paraLayout(ed, i)
		y := oy + float32(a.hs.top(i)-a.scroll)
		start, end := b.start(i), b.end(i)
		_, sp := ed.rangeSpans(start, end)
		if sp != nil && len(b.paras[i].tabs) > 0 {
			// The spans of the ranges are the text's runes, which the layout
			// shows longer past a tab.
			for k := range sp.styles {
				sp.styles[k].End += extraBefore(b.paras[i].tabs, sp.styles[k].End)
			}
		}
		p.textBackgrounds(l, ox, y, ts, sp)
		if focused {
			for _, selected := range ed.selectedRanges() {
				sa, sz := selected.Start, selected.End
				if sa == sz || sa > end || sz < start || sz == start && i > 0 && sa < start {
					continue
				}
				from, to := a.local(ed, i, max(sa, start)), a.local(ed, i, min(sz, end))
				for _, r := range l.SelectionVisual(from, to, sz > end && i < last) {
					p.Fill(Rect{ox + r.X, y + r.Y, r.W, r.H}, ts.selectionColor(t), 0)
				}
			}
		}
		p.textForeground(l, ox, y, ts.color, ts, sp)
		if ed.compose != "" && b.para(ed.caret) == i {
			c := ed.caret - start
			for _, r := range l.Selection(c, c+utf8.RuneCountInString(ed.compose)) {
				p.Fill(Rect{ox + r.X, y + r.Y + r.H - 2, r.W, 1}, ts.color, 0)
			}
		}
	}
	if focused && !ed.readOnly {
		rt := e.c.rt
		phase := e.c.now.Sub(rt.blinkStart)
		const blink = 530 * time.Millisecond
		if (phase/blink)%2 == 0 {
			x, y, h := a.caretAt(ed, ed.caret, ed.composeCaret)
			p.add(scene.Op{Kind: scene.OpFill, Rect: p.snap(Rect{ox + x, oy + float32(y-a.scroll), 0, h}), Color: t.Accent.scene(), Wide: p.wide(t.Accent, Color{}, Color{}), Opacity: p.opacity})
			op := &p.s.Ops[len(p.s.Ops)-1]
			op.Rect.W = max(round(p.scale), 1)
		}
		if phase < 30*time.Second {
			e.c.After(blink - phase%blink)
		}
	}
}
