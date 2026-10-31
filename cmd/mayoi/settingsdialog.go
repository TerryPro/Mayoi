package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"mayoi/locales"
)

// settingsDialog is the box of the settings: a group of settings at its
// left, and the settings of that group at its right.
type settingsDialog struct {
	open bool
	tab  int
}

// settingsTabs are the groups of the settings, in the order the box shows
// them.
var settingsTabs = []struct {
	key   string
	label func() string
}{
	{"editor", locales.SettingsGroupEditor},
	{"appearance", locales.SettingsGroupAppearance},
	{"terminal", locales.SettingsGroupTerminal},
	{"git", locales.SettingsGroupGit},
	{"advanced", locales.SettingsGroupAdvanced},
}

// openSettingsDialog shows the settings, and closeSettingsDialog hides
// them.
func (a *app) openSettingsDialog() {
	a.settingsUI = settingsDialog{open: true}
	a.invalidate()
}

func (a *app) closeSettingsDialog() {
	if !a.settingsUI.open {
		return
	}
	a.settingsUI.open = false
	if a.focus != paneTerminal {
		a.focusColumn()
	}
}

// settingsDialogView shows the settings while they are open.
func (a *app) settingsDialogView(c *ui.Context) {
	sd := &a.settingsUI
	if !sd.open {
		return
	}
	p := a.pal
	ui.Overlay(c, func() {
		back := a.dialogBack(c, "settings").Justify(ui.Center)
		var box ui.Element
		back.Children(func() {
			box = a.floating(c, ui.Column(c).Width(760).MaxWidthPercent(94).MaxHeightPercent(84).Radius(10), true).Clip().
				Role(ui.RoleDialog).Label(locales.SettingsTitle())
			box.Children(func() {
				ui.Row(c).Height(48).Padding(0, 18).Gap(10).AlignItems(ui.Center).
					BorderWidth(0, 0, 1, 0).BorderColor(p.border).Children(func() {
					ui.Text(c, locales.SettingsTitle()).FontSize(15).FontWeight(600).TextColor(p.text)
					ui.Spacer(c)
					ui.Text(c, "settings.json").FontSize(11.5).TextColor(p.faint)
				})
				ui.Row(c).Grow(1).MinHeight(0).Children(func() {
					ui.Column(c).Width(180).Shrink(0).Padding(10, 8).Gap(3).Background(p.chrome).
						BorderWidth(0, 0, 0, 1).BorderColor(p.border).Children(func() {
						for i := range settingsTabs {
							a.settingsTab(c, i)
						}
					})
					ui.Scroll(c.Key("settings-items")).Grow(1).MinWidth(0).Padding(14, 18).Children(func() {
						ui.Text(c, settingsTabs[sd.tab].label()).FontSize(11).FontWeight(600).
							LetterSpacing(titleSpacing).TextColor(p.muted).Padding(0, 0, 10, 6)
						a.settingsItems(c, sd.tab)
					})
				})
				ui.Row(c).Height(30).Padding(0, 14).Gap(14).AlignItems(ui.Center).
					BorderWidth(1, 0, 0, 0).BorderColor(p.border).Children(func() {
					hint := func(keys, label string) {
						ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
							a.keycaps(c, keys)
							ui.Text(c, label).FontSize(11.5).TextColor(p.muted).SingleLine()
						})
					}
					hint("esc", locales.QuickClose())
				})
			})
		})
		if box.PressedOutside() || back.OverlayShortcut(0, ui.KeyEscape) {
			a.closeSettingsDialog()
		}
	})
}

// settingsTab is a group's button at the left of the settings.
func (a *app) settingsTab(c *ui.Context, i int) {
	sd := &a.settingsUI
	p := a.pal
	tab := settingsTabs[i]
	b := ui.ButtonBase(c.Key("settings-tab:"+tab.key)).Height(30).Padding(0, 10).Radius(6).
		Justify(ui.Start).AlignItems(ui.Center).Label(tab.label())
	col := p.muted
	switch {
	case i == sd.tab:
		b.Background(p.active)
		col = p.text
	case b.Hovered():
		b.Background(p.hover)
		col = p.text
	}
	b.Transition(hoverFade)
	b.Children(func() {
		ui.Text(c, tab.label()).FontSize(12.5).TextColor(col).SingleLine()
	})
	if b.Clicked() {
		sd.tab = i
	}
}

// settingsItems are the settings of a group.
func (a *app) settingsItems(c *ui.Context, tab int) {
	switch tab {
	case 0:
		a.settingsBool(c, "autoSave", locales.CmdFileAutoSave, &a.settings.AutoSave)
		a.settingsBool(c, "wordWrap", locales.CmdViewWordWrap, &a.settings.WordWrap)
		a.settingsShowFoldingControls(c)
		a.settingsOutlinePosition(c)
		a.settingsOutlineSort(c)
		a.settingsOutlineKinds(c)
		a.settingsBool(c, "vim", locales.CmdViewVim, &a.settings.Vim)
		a.settingsTabSize(c)
		a.settingsFloat(c, "fontSize", locales.SettingsFontSize,
			func() float64 { return float64(a.settings.FontSize) },
			func(v float64) { a.settings.FontSize = float32(v) }, 9, 28)
		a.settingsFloat(c, "lineHeight", locales.SettingsLineHeight,
			func() float64 { return float64(a.settings.LineHeight) },
			func(v float64) { a.settings.LineHeight = float32(v) }, 1, 3)
	case 1:
		a.settingsTheme(c)
		a.settingsAppearance(c)
		a.settingsBool(c, "menuBarExpanded", locales.CmdViewMenuBar, &a.settings.MenuBarExpanded)
		a.settingsBool(c, "sidebarRight", locales.CmdViewSidebarRight, &a.settings.SidebarRight)
		a.settingsPanelPosition(c)
	case 2:
		a.settingsBool(c, "terminalCopyOnSelect", locales.SettingsTerminalCopy, &a.settings.TerminalCopyOnSelect)
	case 3:
		a.settingsBool(c, "blame", locales.CmdGitBlame, &a.settings.Blame)
	case 4:
		a.settingsBool(c, "lsp", locales.SettingsLSP, &a.settings.LSP)
		a.settingsBool(c, "projectTabs", locales.CmdViewProjectTabs, &a.settings.ProjectTabs)
	}
}

// settingsRow shows a setting's name and its key, and the control at its
// right.
func (a *app) settingsRow(c *ui.Context, key string, label func() string, control func()) {
	p := a.pal
	row := ui.Row(c.Key("setting:"+key)).MinHeight(56).Padding(10, 12).Gap(20).
		AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(p.border)
	if row.Hovered() {
		row.Background(p.hover)
	}
	row.Transition(hoverFade)
	row.Children(func() {
		ui.Column(c).Grow(1).Shrink(1).Gap(2).Children(func() {
			ui.Text(c, label()).FontSize(13.5).FontWeight(600).TextColor(p.text).SingleLine()
			ui.Text(c, key).FontSize(11).TextColor(p.faint).SingleLine()
		})
		control()
	})
}

func (a *app) settingsBool(c *ui.Context, key string, label func() string, v *bool) {
	was := *v
	a.settingsRow(c, key, label, func() {
		ui.Switch(c.Key("setting:"+key+":switch"), v).Label(label())
	})
	if *v != was {
		a.settings.Changed()
		a.invalidate()
	}
}

// settingsPick shows a setting's value as a segmented choice, values the
// settings' and names what the choice shows.
func (a *app) settingsPick(c *ui.Context, key string, label func() string, v *string, values, names []string) {
	idx := 0
	for i, val := range values {
		if val == *v {
			idx = i
		}
	}
	was := idx
	a.settingsRow(c, key, label, func() {
		ui.Segmented(c.Key("setting:"+key+":pick"), &idx, names...).FontSize(12).Shrink(0)
	})
	if idx != was && idx >= 0 && idx < len(values) {
		*v = values[idx]
		a.settings.Changed()
		a.invalidate()
	}
}

// settingsFloat shows a number of the settings as a slider, the value
// rounded to a tenth.
func (a *app) settingsFloat(c *ui.Context, key string, label func() string, get func() float64, set func(float64), lo, hi float64) {
	v := get()
	was := v
	a.settingsRow(c, key, label, func() {
		ui.Row(c).Width(210).Shrink(0).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Slider(c.Key("setting:"+key+":slider"), &v, lo, hi).Grow(1)
			ui.Text(c, fmt.Sprintf("%.2f", v)).FontSize(11.5).TextColor(a.pal.muted).Width(34).SingleLine()
		})
	})
	if v != was {
		set(v)
		a.settings.Changed()
		a.invalidate()
	}
}

// settingsTabSize shows the tab width as 2, 4 or 8.
func (a *app) settingsTabSize(c *ui.Context) {
	sizes := []int{2, 4, 8}
	idx := 1
	for i, n := range sizes {
		if n == a.settings.TabSize {
			idx = i
		}
	}
	was := idx
	a.settingsRow(c, "tabSize", locales.SettingsTabSize, func() {
		ui.Segmented(c.Key("setting:tabSize:pick"), &idx, "2", "4", "8").FontSize(12).Shrink(0)
	})
	if idx != was && idx >= 0 && idx < len(sizes) {
		a.settings.TabSize = sizes[idx]
		a.settings.Changed()
		a.invalidate()
	}
}

// settingsTheme shows the color theme as a row of its names.
func (a *app) settingsTheme(c *ui.Context) {
	names := make([]string, len(themeChoices))
	values := make([]string, len(themeChoices))
	for i, t := range themeChoices {
		names[i], values[i] = t.name(), t.id
	}
	was := a.settings.Theme
	a.settingsPick(c, "theme", locales.MenuTheme, &a.settings.Theme, values, names)
	if a.settings.Theme != was {
		a.previewTheme(a.settings.Theme, a.settings.Appearance)
		a.themeChanged()
	}
}

// settingsAppearance shows the appearance as the system's, light or dark.
func (a *app) settingsAppearance(c *ui.Context) {
	was := a.settings.Appearance
	a.settingsPick(c, "appearance", locales.SettingsAppearance, &a.settings.Appearance,
		[]string{"", "light", "dark"},
		[]string{locales.CmdViewAppearanceSystem(), locales.CmdViewAppearanceLight(), locales.CmdViewAppearanceDark()})
	if a.settings.Appearance != was {
		a.previewTheme(a.settings.Theme, a.settings.Appearance)
		a.themeChanged()
	}
}

// settingsShowFoldingControls shows when the fold markers show.
func (a *app) settingsShowFoldingControls(c *ui.Context) {
	a.settingsPick(c, "showFoldingControls", locales.SettingsFoldingControls, &a.settings.ShowFoldingControls,
		[]string{"", "always"},
		[]string{locales.FoldOnHover(), locales.FoldAlways()})
}

// settingsOutlinePosition shows where the outline is: under the files of
// the Explorer, above them, or in a sidebar view of its own.
func (a *app) settingsOutlinePosition(c *ui.Context) {
	was := a.settings.OutlinePosition
	a.settingsPick(c, "outlinePosition", locales.SettingsOutlinePosition, &a.settings.OutlinePosition,
		[]string{"", "above", "view"},
		[]string{locales.OutlineBelow(), locales.OutlineAbove(), locales.OutlineView()})
	if a.settings.OutlinePosition != was {
		a.invalidate()
	}
}

// settingsOutlineSort shows how each run of the outline is ordered.
func (a *app) settingsOutlineSort(c *ui.Context) {
	a.settingsPick(c, "outlineSort", locales.SettingsOutlineSort, &a.settings.OutlineSort,
		[]string{"", "name", "kind"},
		[]string{locales.OutlineSortPosition(), locales.OutlineSortName(), locales.OutlineSortKind()})
}

// settingsOutlineKinds shows the groups of declarations the outline shows.
func (a *app) settingsOutlineKinds(c *ui.Context) {
	p := a.pal
	a.settingsRow(c, "outlineKinds", locales.SettingsOutlineKinds, func() {
		ui.Row(c).Width(340).Shrink(0).Gap(4).Wrap().Children(func() {
			for _, g := range outlineKindGroups {
				on := a.outlineKindOn(g)
				b := ui.ButtonBase(c.Key("setting:outlineKinds:"+g.key)).Height(24).Padding(0, 8).Radius(4).
					Label(g.label())
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
					ui.Text(c, g.label()).FontSize(11.5).TextColor(col).SingleLine()
				})
				if b.Clicked() {
					a.toggleOutlineKind(g)
				}
			}
		})
	})
}

// settingsPanelPosition shows where the panel is.
func (a *app) settingsPanelPosition(c *ui.Context) {
	was := a.settings.PanelPosition
	a.settingsPick(c, "panelPosition", locales.MenuPanelPosition, &a.settings.PanelPosition,
		[]string{"", "left", "right"},
		[]string{locales.CmdViewPanelBottom(), locales.CmdViewPanelLeft(), locales.CmdViewPanelRight()})
	if a.settings.PanelPosition != was {
		a.invalidate()
	}
}
