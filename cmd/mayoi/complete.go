package main

import (
	"context"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/bg"
	"mayoi/internal/lsp"
	"mayoi/locales"
)

// completionState is the language server's words at the caret: the words
// it offered, the one chosen, and the place the list shows at.
type completionState struct {
	open    bool
	items   []lsp.CompletionItem
	sel     int
	doc     int
	line    int // the line asked about, which the list shows under
	prefix  int // the rune offset the word at the caret starts at
	version uint64
}

// hoverState is what the language server says of the symbol at the caret,
// shown in a box under it.
type hoverState struct {
	open bool
	text string
	doc  int
	line int
}

// maxCompletionRows is the most words the list shows at once.
const maxCompletionRows = 10

// showCompletion asks the language server for the words at the caret, and
// shows them under it.
func (a *app) showCompletion() {
	d := a.activeEditor()
	if d == nil || d.path == "" || d.loading {
		return
	}
	if a.settings.Vim && d.vim.mode != vimInsert {
		return
	}
	if a.fix.open {
		a.closeFix()
	}
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
	pos := lspPosition(snap, caret)
	c, uri, id := srv.client, lsp.URI(d.path), d.id
	ver, line := snap.Version(), snap.LineAt(caret)
	prefix := completionStart(snap, caret)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	bg.Go(func() {
		defer cancel()
		list, err := c.Completion(ctx, uri, pos)
		if err != nil || len(list.Items) == 0 {
			return
		}
		a.post(func() {
			d := a.activeEditor()
			if d == nil || d.id != id || d.buf.Version() != ver {
				return
			}
			a.complete = completionState{open: true, items: list.Items, doc: id, line: line, prefix: prefix, version: ver}
			a.invalidate()
		})
	})
}

// showHover asks the language server what it says of the symbol at the
// caret, and shows it under it.
func (a *app) showHover() {
	d := a.activeEditor()
	if d == nil || d.path == "" || d.loading {
		return
	}
	if a.hover.open {
		a.closeHover()
		return
	}
	a.closeCompletion()
	a.closeFix()
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
	pos := lspPosition(snap, caret)
	c, uri, id := srv.client, lsp.URI(d.path), d.id
	ver, line := snap.Version(), snap.LineAt(caret)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	bg.Go(func() {
		defer cancel()
		hover, err := c.Hover(ctx, uri, pos)
		if err != nil || hover == nil {
			return
		}
		text := hoverText(hover.Text())
		if text == "" {
			return
		}
		a.post(func() {
			d := a.activeEditor()
			if d == nil || d.id != id || d.buf.Version() != ver {
				return
			}
			a.hover = hoverState{open: true, text: text, doc: id, line: line}
			a.invalidate()
		})
	})
}

// closeCompletion hides the list of words.
func (a *app) closeCompletion() {
	if !a.complete.open {
		return
	}
	a.complete = completionState{}
	a.invalidate()
}

// closeHover hides the box of what the server says.
func (a *app) closeHover() {
	if !a.hover.open {
		return
	}
	a.hover = hoverState{}
	a.invalidate()
}

// completionStart returns where the word at caret starts: after the run
// of its letters and digits before it.
func completionStart(snap ui.TextSnapshot, caret int) int {
	s, _ := snap.LineRange(snap.LineAt(caret))
	i := caret
	for i > s {
		r, _ := utf8.DecodeLastRuneInString(snap.Slice(i-1, i))
		if !isWordRune(r) {
			break
		}
		i--
	}
	return i
}

// completionShown returns the words the text at the caret still matches.
func (a *app) completionShown(d *editor) []lsp.CompletionItem {
	cs := &a.complete
	snap := d.buf.Snapshot()
	caret := min(d.sel[1], snap.Len())
	start := min(max(cs.prefix, 0), caret)
	word := strings.ToLower(snap.Slice(start, caret))
	if word == "" {
		return cs.items
	}
	var out []lsp.CompletionItem
	for _, it := range cs.items {
		label := it.FilterText
		if label == "" {
			label = it.Label
		}
		if strings.HasPrefix(strings.ToLower(label), word) {
			out = append(out, it)
		}
	}
	return out
}

// completionKey takes the keys the list of words uses, before the editor's
// own: the arrows to choose one, Enter, Tab or a click to insert it, and
// Escape to leave it.
func (a *app) completionKey(d *editor, ev ui.InputEvent) bool {
	cs := &a.complete
	if ev.Kind == ui.InputKeyDown && ev.Key == ui.KeyEscape && a.hover.open {
		a.closeHover()
		return true
	}
	if ev.Kind == ui.InputPointerDown && a.hover.open {
		a.closeHover()
	}
	if !cs.open || cs.doc != d.id {
		return false
	}
	switch ev.Kind {
	case ui.InputKeyDown:
		items := a.completionShown(d)
		if len(items) == 0 {
			a.closeCompletion()
			return false
		}
		switch ev.Key {
		case ui.KeyEscape:
			a.closeCompletion()
			return true
		case ui.KeyUp:
			cs.sel = max(cs.sel-1, 0)
			a.invalidate()
			return true
		case ui.KeyDown:
			cs.sel = min(cs.sel+1, len(items)-1)
			a.invalidate()
			return true
		case ui.KeyEnter, ui.KeyTab:
			if ev.Mods == 0 {
				a.applyCompletion(d, items)
				return true
			}
		}
	case ui.InputPointerDown:
		// A click of the editor's own, not one of the list's buttons, which
		// the list does not take: it puts the caret where it goes, and the
		// list goes.
		a.closeCompletion()
	}
	return false
}

// applyCompletion inserts the word chosen in place of what is typed of it.
func (a *app) applyCompletion(d *editor, items []lsp.CompletionItem) {
	cs := &a.complete
	if len(items) == 0 {
		a.closeCompletion()
		return
	}
	it := items[min(cs.sel, len(items)-1)]
	text := it.InsertText
	if text == "" && it.TextEdit != nil {
		text = it.TextEdit.NewText
	}
	if text == "" {
		text = it.Label
	}
	text = stripSnippet(text)
	snap := d.buf.Snapshot()
	caret := min(d.sel[1], snap.Len())
	start := min(max(cs.prefix, 0), caret)
	// The word's start is found again: a jump, or typing past it, moved the
	// offsets of the frame the list opened at.
	start = completionStart(snap, caret)
	a.closeCompletion()
	if text == "" || start > caret {
		return
	}
	sel := start + utf8.RuneCountInString(text)
	d.edit([2]int{sel, sel}, textEdit{start: start, end: caret, text: text})
}

// stripSnippet returns the text of a snippet, with its placeholders and
// their defaults as the text leaves them: $0 and ${1:name} go as name.
func stripSnippet(s string) string {
	if !strings.ContainsAny(s, "$") {
		return s
	}
	var out strings.Builder
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			if i+1 < len(s) && strings.ContainsRune("$}", rune(s[i+1])) {
				out.WriteByte(s[i+1])
				i++
			} else {
				out.WriteByte(s[i])
			}
		case '$':
			if i+1 < len(s) && s[i+1] == '{' {
				j := strings.IndexByte(s[i+2:], '}')
				if j < 0 {
					continue
				}
				inner := s[i+2 : i+2+j]
				if k := strings.IndexByte(inner, ':'); k >= 0 {
					out.WriteString(inner[k+1:])
				}
				i += 2 + j
			} else {
				j := i + 1
				for j < len(s) && s[j] >= '0' && s[j] <= '9' {
					j++
				}
				i = j - 1
			}
		default:
			out.WriteByte(s[i])
		}
	}
	return out.String()
}

// hoverText returns the text of a hover's contents, without the marks
// markdown spells and past its first lines.
func hoverText(s string) string {
	const maxLines = 12
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "![") {
			continue
		}
		t = strings.ReplaceAll(t, "**", "")
		t = strings.ReplaceAll(t, "`", "")
		if t == "" && len(lines) > 0 && lines[len(lines)-1] == "" {
			continue
		}
		lines = append(lines, t)
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > maxLines {
		lines = append(lines[:maxLines], "…")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// completionView shows the words the language server offers under the
// caret, and what it says of the symbol there.
func (a *app) completionView(c *ui.Context, d *editor, lineTop func(int) float32, textX, lineH float32, view ui.Rect) {
	p := a.pal
	snap := d.buf.Snapshot()
	caret := min(d.sel[1], snap.Len())
	caretLine := snap.LineAt(caret)
	s, _ := snap.LineRange(caretLine)
	caretX := textX + a.textWidth(snap.Slice(s, caret))
	left := max(d.hscroll.X+caretX, 8)
	if view.W > 360 {
		left = min(left, d.hscroll.X+view.W-352)
	}
	if a.hover.open && a.hover.doc == d.id {
		y := lineTop(a.hover.line) + lineH + 2
		ui.Column(c.Key("hover")).Absolute().Left(left).Top(max(y, 4)).
			Width(min(360, max(view.W-textX-24, 200))).Padding(8, 10).Gap(2).
			Background(p.panel).Border(1, p.border).Radius(6).Children(func() {
			ui.Text(c, a.hover.text).FontSize(11.5).TextColor(p.text).MaxWidth(320)
		})
	}
	cs := &a.complete
	if !cs.open || cs.doc != d.id {
		return
	}
	items := a.completionShown(d)
	if len(items) == 0 {
		a.complete = completionState{}
		return
	}
	sel := min(cs.sel, len(items)-1)
	first := max(0, min(sel-maxCompletionRows/2, len(items)-maxCompletionRows))
	last := min(len(items), first+maxCompletionRows)
	height := float32(last-first)*22 + 8
	y := lineTop(caretLine) + lineH + 2
	if view.H > 0 && y+height > view.H {
		y = lineTop(caretLine) - height - 2
	}
	inner := func() {
		for i := first; i < last; i++ {
			it := items[i]
			on := i == sel
			row := ui.ButtonBase(c.Key("completion:"+strconv.Itoa(i))).Height(22).Padding(0, 8).
				AlignItems(ui.Center).Radius(3).Label(it.Label)
			if on {
				row.Background(p.active)
			} else if row.Hovered() {
				row.Background(p.hover)
			}
			row.Children(func() {
				ui.Text(c, it.Label).FontSize(12).TextColor(p.text).SingleLine().Shrink(1).Grow(1).MinWidth(0)
				if it.Detail != "" {
					ui.Text(c, it.Detail).FontSize(10.5).TextColor(p.faint).SingleLine().MaxWidth(150)
				}
			})
			if row.Hovered() {
				cs.sel = i
			}
			if row.Clicked() {
				cs.sel = i
				a.post(func() { a.applyCompletion(d, a.completionShown(d)) })
			}
		}
	}
	ui.Column(c.Key("completion")).Absolute().Left(left).Top(max(y, 4)).
		Width(340).Padding(0, 4).Gap(0).Background(p.panel).Border(1, p.border).Radius(6).
		Shadow(0, 8, 24, 0, p.shadow).Children(inner)
}
