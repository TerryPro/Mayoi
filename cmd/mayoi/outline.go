package main

import (
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/bg"
	"mayoi/internal/highlight"
	"mayoi/internal/symbols"
	"mayoi/locales"
)

// outlineNode is a declaration of the outline, and the declarations it
// holds.
type outlineNode struct {
	sym      symbols.Symbol
	key      string
	children []*outlineNode
}

// outlineRow is a row of the outline: a declaration, and how deep it shows
// under the ones above it.
type outlineRow struct {
	node  *outlineNode
	level int
}

// outlineState is the outline of the document shown: its symbols, the tree
// they make, and the rows of it that show.
type outlineState struct {
	// The symbols of the document of version doc and version, and whether
	// they are ready. A document over outlineSyncLines is read in the
	// background, and the outline shows the symbols of the version before
	// until it comes.
	doc     int
	version uint64
	symbols []symbols.Symbol
	ready   bool

	pendingDoc     int
	pendingVersion uint64
	pending        bool

	// parseGen counts the reads asked for, so that one of a document or a
	// version left behind is dropped.
	parseGen int

	// The tree, made for these symbols and these settings.
	built      bool
	builtDoc   int
	builtVer   uint64
	builtKinds string
	builtSort  string
	nodes      []*outlineNode
	treeGen    int

	// The rows shown, made for this tree and these folded nodes.
	rowsBuilt    int
	rowsCollapse int
	rowsDoc      int
	rows         []outlineRow

	collapsed   map[string]bool
	collapseGen int

	row      int
	list     ui.ListState
	listView ui.Handle
	focus    bool
}

// outlineSyncLines is how long a document may be for its outline to be
// read in the frame that changes it; longer ones are read in the
// background.
const outlineSyncLines = 4000

// outlineMinH is the shortest outline section, under the files, and
// outlineHeadH the height of its header.
const (
	outlineMinH  = 80
	outlineHeadH = 24
	outlineRowH  = 22
)

// outlineAllKinds are the kinds of declaration the outline knows, in the
// order it groups them.
var outlineAllKinds = []string{"namespace", "class", "interface", "type", "enum", "function", "method", "variable", "heading"}

// outlineKindGroup is a group of declaration kinds the settings show or
// hide together, and what it is called.
type outlineKindGroup struct {
	key   string
	kinds []string
	label func() string
}

// outlineKindGroups are the groups of kinds the settings turn on and off.
var outlineKindGroups = []outlineKindGroup{
	{"functions", []string{"function", "method"}, locales.OutlineKindFunctions},
	{"types", []string{"class", "interface", "type", "enum"}, locales.OutlineKindTypes},
	{"variables", []string{"variable"}, locales.OutlineKindVariables},
	{"namespaces", []string{"namespace"}, locales.OutlineKindNamespaces},
	{"headings", []string{"heading"}, locales.OutlineKindHeadings},
}

// outlineKindRank orders the declarations of an outline sorted by kind.
var outlineKindRank = map[string]int{"namespace": 0, "class": 1, "interface": 2, "type": 3, "enum": 4, "function": 5, "method": 6, "variable": 7, "heading": 8}

// outlineSymbols returns the symbols of the document shown, and whether
// they are ready: the ones of the version before while a long document is
// read again.
func (a *app) outlineSymbols() ([]symbols.Symbol, bool) {
	o := &a.outline
	d := a.activeEditor()
	if d == nil {
		return nil, true
	}
	ver := d.buf.Version()
	if o.doc == d.id && o.version == ver && o.ready {
		return o.symbols, true
	}
	if o.pending && o.pendingDoc == d.id && o.pendingVersion == ver {
		if o.doc == d.id {
			return o.symbols, o.ready
		}
		return nil, false
	}
	snap := d.buf.Snapshot()
	markdown := d.hl != nil && strings.EqualFold(highlight.Name(d.hl.Lexer()), "markdown")
	if snap.LineCount() > outlineSyncLines {
		if o.pending {
			// One read of a long document runs at a time: the next frame
			// reads the text as it is once that one is done.
			if o.doc == d.id {
				return o.symbols, o.ready
			}
			return nil, false
		}
		o.parseGen++
		gen := o.parseGen
		id := d.id
		o.pendingDoc, o.pendingVersion, o.pending = id, ver, true
		if o.doc != id {
			o.doc, o.version, o.symbols, o.ready = id, 0, nil, false
		}
		bg.Go(func() {
			found := symbols.Find(snap, markdown)
			a.post(func() {
				if o.parseGen != gen {
					return
				}
				o.doc, o.version, o.symbols, o.ready, o.pending = id, ver, found, true, false
				a.invalidate()
			})
		})
		return o.symbols, o.ready
	}
	o.parseGen++
	o.doc, o.version, o.symbols, o.ready, o.pending = d.id, ver, symbols.Find(snap, markdown), true, false
	return o.symbols, true
}

// outlineTree returns the tree of the outline, and whether its symbols are
// ready, worked out again only as the text or its settings change.
func (a *app) outlineTree() ([]*outlineNode, bool) {
	if a.activeEditor() == nil {
		return nil, true
	}
	o := &a.outline
	syms, ready := a.outlineSymbols()
	if !ready {
		return nil, false
	}
	kinds := strings.Join(a.settings.OutlineKinds, "\x00")
	order := a.settings.OutlineSort
	if o.built && o.builtDoc == o.doc && o.builtVer == o.version && o.builtKinds == kinds && o.builtSort == order {
		return o.nodes, true
	}
	nodes := outlineNodes(syms)
	nodes = pruneOutlineVariables(nodes, false)
	if len(a.settings.OutlineKinds) > 0 {
		nodes = filterOutlineNodes(nodes, func(kind string) bool {
			return slices.Contains(a.settings.OutlineKinds, kind)
		})
	}
	outlineAssignKeys(nodes, "")
	sortOutlineNodes(nodes, order)
	o.built, o.builtDoc, o.builtVer, o.builtKinds, o.builtSort, o.nodes = true, o.doc, o.version, kinds, order, nodes
	o.treeGen++
	return nodes, true
}

// outlineNodes makes the tree of a document's symbols, each under the
// declaration it is indented in.
func outlineNodes(syms []symbols.Symbol) []*outlineNode {
	var roots []*outlineNode
	var stack []*outlineNode
	for _, sym := range syms {
		for len(stack) > 0 && stack[len(stack)-1].sym.Depth >= sym.Depth {
			stack = stack[:len(stack)-1]
		}
		n := &outlineNode{sym: sym}
		if len(stack) > 0 {
			parent := stack[len(stack)-1]
			parent.children = append(parent.children, n)
		} else {
			roots = append(roots, n)
		}
		stack = append(stack, n)
	}
	return roots
}

// pruneOutlineVariables drops the variables declared in a function, which
// are locals rather than declarations of the document.
func pruneOutlineVariables(nodes []*outlineNode, inFunc bool) []*outlineNode {
	out := make([]*outlineNode, 0, len(nodes))
	for _, n := range nodes {
		underFunc := inFunc || n.sym.Kind == "function" || n.sym.Kind == "method"
		if n.sym.Kind == "variable" && underFunc {
			continue
		}
		n.children = pruneOutlineVariables(n.children, underFunc)
		out = append(out, n)
	}
	return out
}

// filterOutlineNodes keeps the kinds keep says, and puts the declarations
// under one it says no to in its place, rather than hiding them with it.
func filterOutlineNodes(nodes []*outlineNode, keep func(string) bool) []*outlineNode {
	var out []*outlineNode
	for _, n := range nodes {
		kids := filterOutlineNodes(n.children, keep)
		if keep(n.sym.Kind) {
			n.children = kids
			out = append(out, n)
			continue
		}
		out = append(out, kids...)
	}
	return out
}

// outlineAssignKeys gives each declaration a key of its place, which
// folded state follows.
func outlineAssignKeys(nodes []*outlineNode, prefix string) {
	seen := map[string]int{}
	for _, n := range nodes {
		base := fmt.Sprintf("%s%s:%s", prefix, n.sym.Kind, n.sym.Name)
		i := seen[base]
		seen[base] = i + 1
		key := base
		if i > 0 {
			key = fmt.Sprintf("%s#%d", base, i)
		}
		n.key = key
		outlineAssignKeys(n.children, key+"/")
	}
}

// sortOutlineNodes sorts each run of declarations by the order of the
// settings: by name, or by kind.
func sortOutlineNodes(nodes []*outlineNode, order string) {
	switch order {
	case "name":
		sort.SliceStable(nodes, func(i, j int) bool {
			return strings.ToLower(nodes[i].sym.Name) < strings.ToLower(nodes[j].sym.Name)
		})
	case "kind":
		sort.SliceStable(nodes, func(i, j int) bool {
			x, y := outlineKindRank[nodes[i].sym.Kind], outlineKindRank[nodes[j].sym.Kind]
			if x != y {
				return x < y
			}
			return strings.ToLower(nodes[i].sym.Name) < strings.ToLower(nodes[j].sym.Name)
		})
	}
	for _, n := range nodes {
		sortOutlineNodes(n.children, order)
	}
}

// outlineRows returns the rows the outline shows, each declaration under
// the ones not folded.
func (a *app) outlineRows() []outlineRow {
	o := &a.outline
	nodes, ready := a.outlineTree()
	if !ready || nodes == nil {
		return nil
	}
	if o.rowsBuilt == o.treeGen && o.rowsCollapse == o.collapseGen && o.rowsDoc == o.doc {
		return o.rows
	}
	o.rows = o.rows[:0]
	outlineFlatten(nodes, a.outlineDocKey(), o.collapsed, 0, &o.rows)
	o.rowsBuilt, o.rowsCollapse, o.rowsDoc = o.treeGen, o.collapseGen, o.doc
	return o.rows
}

// outlineFlatten adds the rows of a run of declarations, and those of
// their children unless folded.
func outlineFlatten(nodes []*outlineNode, path string, collapsed map[string]bool, level int, out *[]outlineRow) {
	for _, n := range nodes {
		*out = append(*out, outlineRow{node: n, level: level})
		if len(n.children) > 0 && !collapsed[path+"\x00"+n.key] {
			outlineFlatten(n.children, path, collapsed, level+1, out)
		}
	}
}

// outlineDocKey names the document the outline shows, for the folded
// state of its declarations.
func (a *app) outlineDocKey() string {
	d := a.activeEditor()
	if d == nil {
		return ""
	}
	if d.path != "" {
		return d.path
	}
	return fmt.Sprintf("#%d", d.id)
}

// outlineView shows the outline of the document shown in a view of the
// sidebar of its own.
func (a *app) outlineView(c *ui.Context) {
	p := a.pal
	ui.Column(c.Key("outline-pane")).Fill().MinHeight(0).Children(func() {
		ui.Row(c).Height(26).Padding(0, 10, 0, 12).Gap(6).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(p.border).Children(func() {
			name := ""
			if d := a.activeEditor(); d != nil {
				name = d.title
				if d.path != "" {
					name = filepath.Base(d.path)
				}
			}
			if name == "" {
				ui.Spacer(c)
				return
			}
			ui.Text(c, name).FontSize(11.5).TextColor(p.faint).SingleLine().Shrink(1).Grow(1).MinWidth(0)
		})
		a.outlineBody(c)
	})
}

// outlineSection shows the outline under the files of the Explorer, or
// above them, as high as the splitter left it, or as its header alone
// while it is folded.
func (a *app) outlineSection(c *ui.Context, above bool) {
	p, w := a.pal, a.window
	open := !w.OutlineClosed
	_, windowH := c.Size()
	room := windowH - titleBarH - statusBarH - outlineHeadH
	height := clamp(w.OutlineHeight, outlineMinH, max(room-outlineMinH, outlineMinH))
	box := ui.Column(c.Key("outline-section")).Shrink(0).Background(p.chrome)
	if above {
		box.BorderWidth(0, 0, 1, 0)
	} else {
		box.BorderWidth(1, 0, 0, 0)
	}
	box.BorderColor(p.border)
	if open {
		box.Height(height)
	} else {
		box.Height(outlineHeadH)
	}
	box.Children(func() {
		a.outlineHeader(c, open)
		if !open {
			return
		}
		a.outlineBody(c)
		edge, drag := edgeTop, float32(-1)
		if above {
			edge, drag = edgeBottom, float32(1)
		}
		a.splitter(c, edge, func(d float32) {
			w.OutlineHeight = clamp(w.OutlineHeight+drag*d, outlineMinH, max(room-outlineMinH, outlineMinH))
		})
	})
}

// outlineHeader is the section's header: the arrow that folds it, and its
// title.
func (a *app) outlineHeader(c *ui.Context, open bool) {
	p, w := a.pal, a.window
	b := ui.ButtonBase(c.Key("outline-header")).Height(outlineHeadH).Padding(0, 4, 0, 8).Gap(4).
		Justify(ui.Start).AlignItems(ui.Center).Label(locales.OutlineTitle())
	b.Children(func() {
		chevron(c, open).FontSize(11).TextColor(p.muted)
		ui.Text(c, strings.ToUpper(locales.OutlineTitle())).FontSize(11).FontWeight(600).
			LetterSpacing(titleSpacing).TextColor(p.muted).SingleLine()
		ui.Spacer(c)
	})
	if b.Hovered() {
		b.Background(p.hover)
	}
	b.Transition(hoverFade)
	if b.Clicked() {
		w.OutlineClosed = !w.OutlineClosed
		a.settings.Changed()
		a.invalidate()
	}
}

// outlineBody is what shows under a header: the rows of the outline, or
// why it has none.
func (a *app) outlineBody(c *ui.Context) {
	p := a.pal
	_, ready := a.outlineTree()
	if !ready {
		a.outlineNote(c, locales.OutlineLoading())
		return
	}
	rows := a.outlineRows()
	if len(rows) == 0 {
		msg := locales.OutlineNoSymbols()
		if a.activeEditor() == nil {
			msg = locales.OutlineNoFile()
		}
		a.outlineNote(c, msg)
		return
	}
	s := &a.outline
	doc := a.outlineDocKey()
	s.list.Selected = &s.row
	s.list.Key = func(i int) any { return doc + "\x00" + rows[i].node.key }
	s.list.Label = func(i int) string { return rows[i].node.sym.Name }
	base := c.Theme()
	quiet := *base
	quiet.Accent, quiet.AccentText, quiet.SurfaceHover = p.active, p.text, p.hover
	c.SetTheme(&quiet)
	out := ui.List(c.Key("outline"), &s.list, len(rows), func(i int) {
		a.outlineRowView(c, rows[i], i)
	}).Grow(1).MinHeight(0).Padding(0, 6, 6, 8).Bind(&s.listView)
	c.SetTheme(base)
	if s.focus && s.list.Focus(c) {
		s.focus = false
	}
	if s.list.Focused(c) {
		if s.list.Shortcut(c, 0, ui.KeyEscape) {
			a.focusColumn()
		}
		if s.row >= 0 && s.row < len(rows) {
			n := rows[s.row].node
			if s.list.Shortcut(c, 0, ui.KeyLeft) {
				a.foldOutlineNode(n, true)
			}
			if s.list.Shortcut(c, 0, ui.KeyRight) {
				a.foldOutlineNode(n, false)
			}
		}
	}
	if out.Submitted() && s.row >= 0 && s.row < len(rows) {
		n := rows[s.row].node
		a.post(func() { a.openOutlineNode(n) })
	}
}

// outlineNote says what the outline has none of.
func (a *app) outlineNote(c *ui.Context, msg string) {
	ui.Column(c).Grow(1).MinHeight(0).Padding(14, 12).AlignItems(ui.Start).Children(func() {
		ui.Text(c, msg).FontSize(11.5).TextColor(a.pal.faint).LineHeight(1.4)
	})
}

// outlineRowView is a declaration in the outline: the arrow of the ones it
// holds, its mark, and its name.
func (a *app) outlineRowView(c *ui.Context, r outlineRow, i int) {
	p, n := a.pal, r.node
	row := ui.Row(c).Grow(1).Height(outlineRowH).AlignItems(ui.Center).Gap(3)
	row.Children(func() {
		if r.level > 0 {
			ui.Box(c).Width(float32(r.level) * 10).Shrink(0)
		}
		if len(n.children) > 0 {
			open := !a.outlineFolded(n)
			b := ui.ButtonBase(c.Key("outline-arrow:"+n.key)).Size(16, 18).Radius(3).
				Justify(ui.Center).AlignItems(ui.Center).Label(n.sym.Name)
			b.Children(func() { chevron(c, open).FontSize(11).TextColor(p.muted) })
			if b.Hovered() {
				b.Background(p.hover)
			}
			b.Transition(hoverFade)
			if b.Clicked() {
				a.outline.row = i
				a.foldOutlineNode(n, !open)
			}
		} else {
			ui.Box(c).Width(16).Shrink(0)
		}
		item := ui.ButtonBase(c.Key("outline-item:" + n.key)).Grow(1).MinWidth(0).Height(outlineRowH).Radius(3).
			Gap(6).Justify(ui.Start).AlignItems(ui.Center).Label(n.sym.Name)
		item.Children(func() {
			icon, col := a.symbolIcon(n.sym.Kind)
			ui.Icon(c, icon).FontSize(13).TextColor(col).Shrink(0)
			ui.Text(c, n.sym.Name).FontSize(12).TextColor(p.text).SingleLine().Shrink(1).Grow(1).MinWidth(0)
		})
		if item.Hovered() {
			item.Background(p.hover)
		}
		item.Transition(hoverFade)
		if item.Clicked() {
			a.outline.row = i
			a.post(func() { a.openOutlineNode(n) })
		}
	})
}

// outlineFolded reports whether a declaration's children are folded.
func (a *app) outlineFolded(n *outlineNode) bool {
	return a.outline.collapsed[a.outlineDocKey()+"\x00"+n.key]
}

// foldOutlineNode folds or unfolds the declarations under n, and says so
// in a new frame.
func (a *app) foldOutlineNode(n *outlineNode, folded bool) {
	if len(n.children) == 0 || a.outlineFolded(n) == folded {
		return
	}
	key := a.outlineDocKey() + "\x00" + n.key
	if a.outline.collapsed == nil {
		a.outline.collapsed = map[string]bool{}
	}
	if folded {
		a.outline.collapsed[key] = true
	} else {
		delete(a.outline.collapsed, key)
	}
	a.outline.collapseGen++
	a.invalidate()
}

// openOutlineNode shows the line of a declaration, and chooses its name.
func (a *app) openOutlineNode(n *outlineNode) {
	a.editActive(func(d *editor) {
		snap := d.buf.Snapshot()
		if snap.LineCount() == 0 {
			return
		}
		line := min(max(n.sym.Line, 0), snap.LineCount()-1)
		a.jumpToScope(d, line)
		s, e := snap.LineRange(line)
		text := snap.Slice(s, e)
		at := s + min(n.sym.Col, utf8.RuneCountInString(text))
		d.selectRange(at, at+utf8.RuneCountInString(n.sym.Name), false)
		d.lightLanding()
	})()
}

// showOutline shows the outline in a view of the sidebar of its own.
func (a *app) showOutline() {
	a.window.SidebarClosed, a.window.SidebarView = false, "outline"
	a.settings.Changed()
	a.outline.focus = true
	a.invalidate()
}

// outlineKindOn reports whether a group of kinds shows.
func (a *app) outlineKindOn(g outlineKindGroup) bool {
	for _, kind := range g.kinds {
		if a.outlineKindShown(kind) {
			return true
		}
	}
	return false
}

// outlineKindShown reports whether a kind of declaration shows: all do
// until the settings name those that do.
func (a *app) outlineKindShown(kind string) bool {
	if len(a.settings.OutlineKinds) == 0 {
		return true
	}
	return slices.Contains(a.settings.OutlineKinds, kind)
}

// toggleOutlineKind shows or hides a group of kinds.
func (a *app) toggleOutlineKind(g outlineKindGroup) {
	on := a.outlineKindOn(g)
	var kinds []string
	for _, kind := range outlineAllKinds {
		show := a.outlineKindShown(kind)
		if slices.Contains(g.kinds, kind) {
			show = !on
		}
		if show {
			kinds = append(kinds, kind)
		}
	}
	if len(kinds) >= len(outlineAllKinds) {
		kinds = nil
	}
	a.settings.OutlineKinds = kinds
	a.settings.Changed()
	a.invalidate()
}
