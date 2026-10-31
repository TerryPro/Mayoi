package main

import (
	"cmp"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/lsp"
	"mayoi/locales"
)

// problem is one of the language server's problems, as the problems panel
// lists it: where it is, how bad it is, and what it says.
type problem struct {
	path      string
	line      int
	character int
	severity  int
	message   string
	source    string
}

// problemsPanel is the problems panel's place: the filter of its severities
// and its text, the files it folds, and the row it chooses.
type problemsPanel struct {
	list        ui.ListState
	listView    ui.Handle
	input       ui.Handle
	row         int
	query       string
	severity    int // 0 for all, 1 for errors, 2 for warnings
	collapsed   map[string]bool
	collapseGen int
	focusList   bool
}

// problemsCache is what the problems panel last worked out, of the
// diagnostics of the language servers, which are worked out again only as
// they change, or as its filter does, rather than every frame.
type problemsCache struct {
	valid            bool
	gen              int
	all              []problem
	count            int
	errors, warnings int

	shownValid    bool
	shownGen      int
	shownSeq      int
	shownQuery    string
	shownSeverity int
	shown         []problem

	itemsValid    bool
	itemsGen      int
	itemsShown    int
	itemsCollapse int
	items         []problemItem
}

// problemItem is a row of the problems panel: a file's header, or one of
// its problems.
type problemItem struct {
	header bool
	path   string
	count  int
	pr     problem
}

// problems lists the language server's problems: a file's in the order of
// their place in it, and the files by their paths.
func (a *app) problems() []problem {
	c := &a.problemsCache
	if c.valid && c.gen == a.problemsGen {
		return c.all
	}
	var out []problem
	for _, f := range a.lspDiags {
		for _, d := range f.diags {
			out = append(out, problem{
				path: f.path, line: d.Range.Start.Line, character: d.Range.Start.Character,
				severity: d.Severity, message: d.Message, source: d.Source,
			})
		}
	}
	slices.SortFunc(out, func(x, y problem) int {
		if c := cmp.Compare(lsp.Key(x.path), lsp.Key(y.path)); c != 0 {
			return c
		}
		if c := cmp.Compare(x.line, y.line); c != 0 {
			return c
		}
		if c := cmp.Compare(x.character, y.character); c != 0 {
			return c
		}
		return cmp.Compare(x.message, y.message)
	})
	if out == nil {
		out = []problem{}
	}
	c.valid, c.gen, c.all, c.count = true, a.problemsGen, out, len(out)
	c.errors, c.warnings = 0, 0
	for _, pr := range out {
		switch pr.severity {
		case 1:
			c.errors++
		case 2:
			c.warnings++
		}
	}
	return out
}

// problemsShown lists the problems the panel's filters show.
func (a *app) problemsShown() []problem {
	ps := a.problems()
	s := &a.problemsUI
	c := &a.problemsCache
	if c.shownValid && c.shownGen == c.gen && c.shownQuery == s.query && c.shownSeverity == s.severity {
		return c.shown
	}
	query := strings.ToLower(strings.TrimSpace(s.query))
	out := make([]problem, 0, len(ps))
	for _, pr := range ps {
		switch s.severity {
		case 1:
			if pr.severity != 1 {
				continue
			}
		case 2:
			if pr.severity != 2 {
				continue
			}
		}
		if query != "" &&
			!strings.Contains(strings.ToLower(pr.message), query) &&
			!strings.Contains(strings.ToLower(filepath.ToSlash(pr.path)), query) &&
			!strings.Contains(strings.ToLower(pr.source), query) {
			continue
		}
		out = append(out, pr)
	}
	c.shownValid, c.shownGen, c.shownQuery, c.shownSeverity, c.shown = true, c.gen, s.query, s.severity, out
	c.shownSeq++
	return out
}

// problemCount counts the language server's problems, in every file.
func (a *app) problemCount() int {
	a.problems()
	return a.problemsCache.count
}

// problemSeverities counts the errors and warnings of every file.
func (a *app) problemSeverities() (errors, warnings int) {
	a.problems()
	return a.problemsCache.errors, a.problemsCache.warnings
}

// problemItems lists the panel's rows: a file's header, and its problems
// unless it is folded.
func (a *app) problemItems(ps []problem) []problemItem {
	s := &a.problemsUI
	c := &a.problemsCache
	if c.itemsValid && c.itemsGen == c.gen && c.itemsShown == c.shownSeq && c.itemsCollapse == s.collapseGen {
		return c.items
	}
	counts := map[string]int{}
	for _, pr := range ps {
		counts[lsp.Key(pr.path)]++
	}
	var out []problemItem
	last := ""
	for _, pr := range ps {
		key := lsp.Key(pr.path)
		if key != last {
			last = key
			out = append(out, problemItem{header: true, path: pr.path, count: counts[key]})
		}
		if s.collapsed[key] {
			continue
		}
		out = append(out, problemItem{path: pr.path, pr: pr})
	}
	c.itemsValid, c.itemsGen, c.itemsShown, c.itemsCollapse, c.items = true, c.gen, c.shownSeq, s.collapseGen, out
	return out
}

// openProblems shows the problems panel, and gives its list the keyboard.
func (a *app) openProblems() {
	a.panelProblems = true
	if !a.window.PanelOpen {
		a.window.PanelOpen = true
		a.settings.Changed()
	}
	a.problemsUI.focusList = true
	a.focus = paneProblems
	a.invalidate()
}

// toggleProblems shows the problems panel, or hides the panel when it
// shows them.
func (a *app) toggleProblems() {
	if a.window.PanelOpen && a.panelProblems {
		a.hidePanel()
		return
	}
	a.openProblems()
}

// problemsView lists the language server's problems, grouped by file, and
// the filters of its header.
func (a *app) problemsView(c *ui.Context) {
	p := a.pal
	if a.problemCount() == 0 {
		icon, col, msg := iconCheck, p.faint, locales.ProblemsNone()
		if !a.settings.LSP {
			icon, col, msg = iconTriangleAlert, p.muted, locales.LspOff()
		}
		ui.Column(c).Fill().Center().Padding(12).Gap(8).Children(func() {
			ui.Icon(c, icon).FontSize(18).TextColor(col)
			ui.Text(c, msg).FontSize(12).TextColor(p.faint).TextAlign(ui.Center).MaxWidth(420)
			if !a.settings.LSP {
				if ui.Button(c, locales.CmdFileSettings()).Clicked() {
					a.openSettingsDialog()
				}
			}
		})
		return
	}
	ps := a.problemsShown()
	ui.Column(c).Fill().MinHeight(0).Children(func() {
		a.problemsToolbar(c)
		if len(ps) == 0 {
			ui.Column(c).Grow(1).Center().Padding(12).Children(func() {
				ui.Text(c, locales.ProblemsNoMatch()).FontSize(12).TextColor(p.faint)
			})
			return
		}
		items := a.problemItems(ps)
		s := &a.problemsUI
		s.list.Selected = &s.row
		s.list.Key = func(i int) any {
			it := items[i]
			if it.header {
				return "file\x00" + lsp.Key(it.path)
			}
			return fmt.Sprintf("problem\x00%s\x00%d\x00%d\x00%s", lsp.Key(it.path), it.pr.line, it.pr.character, it.pr.message)
		}
		s.list.Header = func(i int) bool { return items[i].header }
		s.list.Label = func(i int) string {
			it := items[i]
			if it.header {
				return filepath.Base(it.path)
			}
			return it.pr.message
		}
		var action func()
		base := c.Theme()
		quiet := *base
		quiet.Accent, quiet.AccentText, quiet.SurfaceHover, quiet.Surface = p.active, p.text, p.hover, p.panel
		c.SetTheme(&quiet)
		out := ui.List(c.Key("problems"), &s.list, len(items), func(i int) {
			if it := items[i]; it.header {
				a.problemGroupRow(c, it, &action)
			} else {
				a.problemItemRow(c, it.pr)
			}
		}).Grow(1).MinHeight(0).Padding(0, 8, 8, 8).Bind(&s.listView)
		c.SetTheme(base)
		if s.focusList && s.list.Focus(c) {
			s.focusList = false
		}
		if s.list.Focused(c) && s.list.Shortcut(c, 0, ui.KeyEscape) {
			a.focusColumn()
		}
		if out.Submitted() {
			if pr, ok := problemAt(items, s.row); ok {
				a.post(func() { a.openProblem(pr, true) })
			}
		} else if out.Changed() {
			// A click or the arrow keys preview the problem, the keyboard
			// staying in the list.
			s.focusList = true
			if pr, ok := problemAt(items, s.row); ok {
				a.post(func() { a.openProblem(pr, false) })
			}
		}
		if action != nil {
			action()
		}
	})
}

// problemsToolbar is the row of the panel's filters: the severities it
// counts, and the text it searches for.
func (a *app) problemsToolbar(c *ui.Context) {
	p := a.pal
	errors, warnings := a.problemSeverities()
	ui.Row(c).Height(34).Padding(0, 8).Gap(6).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(p.border).Children(func() {
		a.problemFilter(c, "all", fmt.Sprintf("%s %d", locales.ProblemsAll(), a.problemCount()), a.problemsUI.severity == 0)
		a.problemFilter(c, "errors", fmt.Sprintf("%s %d", locales.ProblemsErrors(), errors), a.problemsUI.severity == 1)
		a.problemFilter(c, "warnings", fmt.Sprintf("%s %d", locales.ProblemsWarnings(), warnings), a.problemsUI.severity == 2)
		field := ui.Row(c).Grow(1).MinWidth(0).Height(26).Padding(0, 4, 0, 8).Gap(2).AlignItems(ui.Center).
			Radius(5).Background(p.editor).Border(1, p.border).Transition(fieldFocus)
		field.Children(func() {
			in := a.textKeys(ui.TextInputBase(c.Key("problems-query"), &a.problemsUI.query)).Grow(1).MinWidth(0).
				FontSize(12.5).Placeholder(locales.ProblemsSearch()).Label(locales.ProblemsSearch())
			in.Bind(&a.problemsUI.input)
			if in.Focused() {
				field.Border(1, p.accentDim)
			}
			if in.Shortcut(0, ui.KeyEscape) {
				a.post(func() {
					a.problemsUI.query = ""
					a.problemsUI.focusList = true
				})
			}
		})
	})
}

// problemFilter is a severity's button, which shows its count and chooses
// it as the only one shown.
func (a *app) problemFilter(c *ui.Context, key, label string, on bool) {
	p := a.pal
	b := ui.ButtonBase(c.Key("problems-filter:"+key)).Height(22).Padding(0, 8).Radius(4).
		AlignItems(ui.Center).Label(label)
	if on {
		b.Background(p.active)
	} else if b.Hovered() {
		b.Background(p.hover)
	}
	b.Transition(hoverFade)
	b.Children(func() {
		col := p.muted
		if on {
			col = p.text
		}
		ui.Text(c, label).FontSize(11).FontWeight(600).TextColor(col).SingleLine()
	})
	if b.Clicked() {
		severity := 0
		switch key {
		case "errors":
			severity = 1
		case "warnings":
			severity = 2
		}
		a.problemsUI.severity, a.problemsUI.row = severity, -1
	}
}

// problemGroupRow is a file's row above its problems: its name, its folder,
// how many problems it has, and a click that folds it.
func (a *app) problemGroupRow(c *ui.Context, it problemItem, action *func()) {
	p := a.pal
	s := &a.problemsUI
	key := lsp.Key(it.path)
	row := ui.Row(c).Grow(1).Height(26).Padding(0, 6).Gap(6).AlignItems(ui.Center).Background(p.chrome)
	clicked := row.Clicked()
	row.Children(func() {
		chevron(c, !s.collapsed[key]).FontSize(12).TextColor(p.muted)
		icon, col := fileIcon(p, filepath.Base(it.path))
		ui.Icon(c, icon).FontSize(13).TextColor(col)
		ui.Text(c, filepath.Base(it.path)).FontSize(11.5).FontWeight(600).TextColor(p.text).SingleLine().MaxWidth(240)
		if dir := a.problemDir(it.path); dir != "" {
			ui.Text(c, dir).FontSize(10.5).TextColor(p.faint).SingleLine().Shrink(1).Grow(1).MinWidth(0)
		} else {
			ui.Spacer(c)
		}
		ui.Text(c, strconv.Itoa(it.count)).FontSize(10.5).TextColor(p.faint).SingleLine()
	})
	if clicked {
		*action = func() {
			if s.collapsed == nil {
				s.collapsed = map[string]bool{}
			}
			s.collapsed[key] = !s.collapsed[key]
			s.collapseGen++
			s.focusList = true
			a.invalidate()
		}
	}
}

// problemItemRow is one problem: its severity, its message, and where it is.
func (a *app) problemItemRow(c *ui.Context, pr problem) {
	p := a.pal
	icon, col := severityIcon(p, pr.severity)
	ui.Row(c).Grow(1).Height(24).Padding(0, 6, 0, 22).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, icon).FontSize(12).TextColor(col)
		ui.Text(c, strings.ReplaceAll(pr.message, "\n", " ")).FontSize(11.5).TextColor(p.text).
			SingleLine().Shrink(1).Grow(1).MinWidth(0)
		if pr.source != "" {
			ui.Text(c, pr.source).FontSize(10.5).TextColor(p.faint).SingleLine()
		}
		ui.Text(c, fmt.Sprintf("%d:%d", pr.line+1, pr.character+1)).FontSize(10.5).TextColor(p.faint).SingleLine()
	})
}

// problemAt returns the problem of a row, false for a file's header.
func problemAt(items []problemItem, row int) (problem, bool) {
	if row < 0 || row >= len(items) || items[row].header {
		return problem{}, false
	}
	return items[row].pr, true
}

// openProblem shows a problem's place, and gives it the keyboard with
// focus, as Enter does.
func (a *app) openProblem(pr problem, focus bool) {
	a.revealLocation(lsp.Location{
		URI: lsp.URI(pr.path),
		Range: lsp.Range{
			Start: lsp.Position{Line: pr.line, Character: pr.character},
			End:   lsp.Position{Line: pr.line, Character: pr.character},
		},
	}, focus)
}

// problemDir is the folder of a problem's file, from the folder, to show
// beside its name.
func (a *app) problemDir(path string) string {
	rel := path
	if a.root != "" {
		if r, err := filepath.Rel(a.root, path); err == nil &&
			r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator)) {
			rel = r
		}
	}
	dir := filepath.ToSlash(filepath.Dir(rel))
	if dir == "." || dir == "" || dir == "/" {
		return ""
	}
	return dir
}

// severityIcon is the icon of a problem's severity, and its color.
func severityIcon(p *palette, severity int) (*ui.SVG, ui.Color) {
	switch severity {
	case 1:
		return iconOctagonAlert, p.dangerFg
	case 2:
		return iconTriangleAlert, p.ansi[3]
	case 3:
		return iconInfo, p.ansi[4]
	}
	return iconLightbulb, p.muted
}

// severityColor is the color of a problem's severity, as an underline and
// a mark of the overview strip show it.
func severityColor(p *palette, severity int) ui.Color {
	switch severity {
	case 1:
		return p.dangerFg
	case 2:
		return p.ansi[3]
	case 3:
		return p.ansi[4]
	}
	return p.muted
}
