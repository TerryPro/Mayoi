package main

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mayoi/locales"
)

// aboutDialog is the About dialog, open while it shows.
type aboutDialog struct {
	open bool
}

// showAbout opens the About dialog, which names the version this window runs.
func (a *app) showAbout() {
	a.about.open = true
}

// aboutView is the About dialog: the app's name and the version it runs,
// over the window, closed by its button, a click outside or Escape.
func (a *app) aboutView(c *ui.Context) {
	if !a.about.open {
		return
	}
	p := a.pal
	ui.Overlay(c, func() {
		back := a.dialogBack(c)
		back.Children(func() {
			box := a.floating(c, ui.Column(c).Width(340).MaxWidthPercent(92).Margin(titleBarH+8, 0, 0, 0).
				Padding(18, 20).Gap(6).Radius(12), true).Role(ui.RoleDialog).Label(locales.CmdHelpAbout())
			box.Children(func() {
				ui.Text(c, mygo.App.Name()).FontSize(19).FontWeight(650).TextColor(p.text)
				ui.Text(c, locales.AboutVersion(appVersion())).FontSize(12.5).TextColor(p.muted)
				ui.Row(c).Justify(ui.End).Margin(10, 0, 0, 0).Children(func() {
					if ui.PrimaryButton(c, locales.ButtonClose()).AutoFocus().Clicked() {
						a.about.open = false
					}
				})
			})
			if box.PressedOutside() || back.OverlayShortcut(0, ui.KeyEscape) {
				a.about.open = false
			}
		})
	})
}
