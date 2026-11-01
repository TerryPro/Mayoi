package main

import (
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"

	"mayoi/locales"
)

// This file is the fork's share of the sidebar: the folders' own fold, the
// outline under or above the files, and the timeline under both. It is kept
// out of sidebar.go, which upstream also changes, so that a sync leaves the
// fork's own panes in place.

// sidebarTitleRoom returns the width the sidebar's title keeps, beside the
// switches and the actions of its row.
func (a *app) sidebarTitleRoom(actions int) float32 {
	switches := 1 // the outline's
	if a.root != "" {
		switches += 3
	}
	room := a.window.SidebarWidth - 1 - 14 - 6 - float32(actions)*28
	return room - float32(switches)*26 - float32(switches-1)*3
}

// explorerView is the Explorer's files, and the outline of the document
// shown under them, above them, or in a view of its own.
func (a *app) explorerView(c *ui.Context, hovered bool) {
	p, w := a.pal, a.window
	// The folder's row, with the explorer's actions while the pointer
	// is over the sidebar or the tree has the keyboard: on the title's
	// row, they would leave the title no room.
	ui.Row(c).Height(24).Padding(0, 4, 0, 8).Gap(4).AlignItems(ui.Center).ContextMenu(func(m *ui.Menu) {
		if m.Item(locales.CmdViewCompactFolders()).Checked(a.settings.ExplorerCompactFolders).Chosen() {
			a.toggleCompactFolders()
		}
	}).Children(func() {
		closed := w.FilesClosed
		label := locales.ExplorerHideFiles()
		if closed {
			label = locales.ExplorerShowFiles()
		}
		fold := ui.ButtonBase(c.Key("explorer-fold")).Size(16, 18).Radius(3).
			Justify(ui.Center).AlignItems(ui.Center).Label(label).Tooltip(label)
		fold.Children(func() { chevron(c, !closed).FontSize(11).TextColor(p.muted) })
		if fold.Hovered() {
			fold.Background(p.hover)
		}
		fold.Transition(hoverFade)
		if fold.Clicked() {
			w.FilesClosed = !closed
			a.settings.Changed()
			a.invalidate()
		}
		ui.Text(c, strings.ToUpper(filepath.Base(a.root))).FontSize(11).FontWeight(700).LetterSpacing(0.4).
			TextColor(p.text).SingleLine().Grow(1)
		if !hovered && !a.files.List.FocusWithin(c) {
			return
		}
		if a.iconButton(c, iconFilePlus, locales.NewFileEllipsis(), false).Clicked() {
			a.askNewEntry(a.targetDir(), false)
		}
		if a.iconButton(c, iconFolderPlus, locales.NewFolderEllipsis(), false).Clicked() {
			a.askNewEntry(a.targetDir(), true)
		}
		if a.iconButton(c, iconRefresh, locales.RefreshExplorer(), false).Clicked() {
			a.tree.Refresh()
			a.indexStale = true
		}
		if a.iconButton(c, iconCollapse, locales.CollapseFolders(), false).Clicked() {
			a.files.Open.Clear()
		}
	})
	closed := w.FilesClosed
	if !closed {
		if t := a.tree; t.Loaded(a.root) && len(t.Children(a.root)) == 0 {
			a.emptyFolder(c)
		}
		if a.conn != nil && !a.tree.Loaded(a.root) {
			a.remoteTreeWaits(c)
			return
		}
	}
	// The outline is beside the files unless it has a view of its own, and
	// the timeline is under them both.
	above := a.settings.OutlinePosition == "above"
	if a.settings.OutlinePosition == "view" {
		if !closed {
			a.fileTree(c)
		}
		a.timelineSection(c, closed)
		return
	}
	if above {
		a.outlineSection(c, true)
		if !closed {
			a.fileTree(c)
		}
		a.timelineSection(c, closed)
		return
	}
	if !closed {
		a.fileTree(c)
	}
	a.outlineSection(c, false)
	a.timelineSection(c, closed)
}
