package main

import (
	"runtime"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mayoi/locales"
)

// menuBarShown reports whether the window shows its menus across the
// title bar, as VS Code does, rather than behind the menu button beside
// it. macOS has the system's menu bar, which it always shows.
func (a *app) menuBarShown() bool {
	return runtime.GOOS != "darwin" && a.settings.MenuBarExpanded
}

// menuBar is the menus of the window, File through Help, side by side at
// the left of the title bar. Each opens its own menu below it, with the
// items the menu bar has, in the language the window shows.
func (a *app) menuBar(c *ui.Context) {
	p := a.pal
	ui.Row(c.Key("menu-bar")).Height(24).AlignItems(ui.Center).Shrink(1).Children(func() {
		for _, e := range a.menuBarEntries() {
			b := ui.ButtonBase(c.Key("menu-bar:"+e.key)).Height(24).Padding(0, 8).Radius(4).Label(e.label)
			col := p.muted
			switch {
			case b.Pressed():
				b.Background(p.hover)
				col = p.text
			case b.Hovered():
				b.Background(p.hover)
				col = p.text
			}
			b.Transition(revealFade)
			pressPose(b)
			b.Children(func() {
				ui.Text(c, e.label).FontSize(12).FontWeight(500).TextColor(col).SingleLine()
			})
			b.Menu(e.fill)
		}
	})
}

// menuBarEntry is a menu of the menu bar: the key of its button, its
// name, and the items its menu shows.
type menuBarEntry struct {
	key, label string
	fill       func(m *ui.Menu)
}

// menuBarEntries are the menus of the bar, in the order the menu itself
// keeps them.
func (a *app) menuBarEntries() []menuBarEntry {
	names := []struct{ key, label string }{
		{"file", locales.MenuFile()},
		{"edit", locales.MenuEdit()},
		{"selection", locales.MenuSelection()},
		{"view", locales.MenuView()},
		{"go", locales.MenuGo()},
		{"git", locales.MenuGit()},
		{"help", locales.MenuHelp()},
	}
	out := make([]menuBarEntry, 0, len(names))
	for _, n := range names {
		out = append(out, menuBarEntry{key: n.key, label: n.label, fill: a.menuBarMenu(n.label)})
	}
	return out
}

// menuBarMenu fills the menu of the item labelled label, which the menu
// bar shows, with the items the menu itself has under that label. The
// menu is built as it opens, as the menu button's is, so that the recent
// folders and the state of the items are those of the moment.
func (a *app) menuBarMenu(label string) func(m *ui.Menu) {
	return func(m *ui.Menu) {
		menu := a.menu(false, func(id string, _ *mygo.Window) {
			a.post(func() { a.run(id) })
		})
		for _, it := range menu.Items() {
			if it.Label == label && len(it.Submenu) > 0 {
				a.populateUIMenu(m, it.Submenu)
				return
			}
		}
	}
}

// menuBarItems is the View menu's item that shows the menu bar or the
// menu button, and the line after it: none on macOS, which has the
// system's menu bar.
func (a *app) menuBarItems() []*mygo.MenuItem {
	if runtime.GOOS == "darwin" {
		return nil
	}
	return []*mygo.MenuItem{
		{
			Label:   locales.CmdViewMenuBar(),
			Type:    mygo.MenuItemCheckbox,
			Checked: a.settings.MenuBarExpanded,
			Click: func(_ *mygo.MenuItem, _ *mygo.Window) {
				a.post(a.toggleMenuBar)
			},
		},
		mygo.Separator(),
	}
}

// toggleMenuBar shows the menus across the title bar, or behind the
// menu button, and keeps the choice in the settings.
func (a *app) toggleMenuBar() {
	if runtime.GOOS == "darwin" {
		return
	}
	a.settings.MenuBarExpanded = !a.settings.MenuBarExpanded
	a.settings.Changed()
	a.host.menuChanged()
	a.invalidate()
}
