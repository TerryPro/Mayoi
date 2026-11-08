package main

import "slices"

// fileColumn chooses where an implicit file open goes. Explicit drops and
// session restoration use openFileIn and can still target a locked column.
func (a *app) fileColumn() *column {
	col := a.column()
	if !col.locked {
		return col
	}
	if a.fileCol != nil && !a.fileCol.locked && slices.Contains(a.cols, a.fileCol) {
		return a.fileCol
	}
	for i := a.col - 1; i >= 0; i-- {
		if !a.cols[i].locked {
			a.fileCol = a.cols[i]
			return a.fileCol
		}
	}
	for _, c := range a.cols {
		if !c.locked {
			a.fileCol = c
			return c
		}
	}
	// Even when every column is locked, files have a workspace to the left.
	a.fileCol = a.splitColumn(a.cols[0], false)
	return a.fileCol
}

func (a *app) toggleColumnLock(col *column) {
	if !slices.Contains(a.cols, col) {
		return
	}
	col.locked = !col.locked
	if col.locked {
		// Ensure there is a file workspace without changing keyboard focus.
		if a.fileCol == col {
			a.fileCol = nil
		}
		a.fileColumn()
	} else if len(col.tabs) == 0 {
		a.removeColumn(col)
		a.focusColumn()
	}
	a.rememberTabs()
}

// terminalToLockedRight reuses a terminal-only rightmost column, or makes
// one there, and moves the live session without restarting its process.
func (a *app) terminalToLockedRight(s *session) {
	if !slices.Contains(a.terms, s) {
		return
	}
	to := a.cols[len(a.cols)-1]
	from, _ := a.locate(s.tab)
	terminalOnly := true
	for _, t := range to.tabs {
		terminalOnly = terminalOnly && t.term != nil
	}
	reusable := terminalOnly && (to.locked || from == to)
	if !reusable {
		to = a.splitColumn(to, true)
	}
	if !to.locked {
		a.toggleColumnLock(to)
	}
	a.terminalToColumn(s, to, len(to.tabs))
	a.rememberTabs()
}

func (a *app) moveTerminalToLockedRight() {
	switch t := a.activeTab(); {
	case a.focus == paneTerminal && a.panelTerm != nil:
		a.terminalToLockedRight(a.panelTerm)
	case t != nil && t.term != nil:
		a.terminalToLockedRight(t.term)
	case a.panelTerm != nil:
		a.terminalToLockedRight(a.panelTerm)
	}
}

// terminalLocked is the locked rightmost column the terminal is in, if it
// is in one: the focused terminal's, or the panel's once it moved there.
// Terminal locks always sit in the rightmost column.
func (a *app) terminalLocked() *column {
	var s *session
	switch t := a.activeTab(); {
	case a.focus == paneTerminal && a.panelTerm != nil:
		s = a.panelTerm
	case t != nil && t.term != nil:
		s = t.term
	case a.panelTerm != nil:
		s = a.panelTerm
	}
	if s != nil && s.tab != nil {
		if col, _ := a.locate(s.tab); col != nil && col.locked && col == a.rightmostLockedColumn() {
			return col
		}
	}
	return a.lockedTerminalColumn()
}

// rightmostLockedColumn is the locked column furthest to the right, which
// is the only place the terminal lock keeps terminals.
func (a *app) rightmostLockedColumn() *column {
	for i := len(a.cols) - 1; i >= 0; i-- {
		if a.cols[i].locked {
			return a.cols[i]
		}
	}
	return nil
}

// lockedTerminalColumn is the rightmost locked column when it holds a
// terminal, for the layout menu when no terminal has the keyboard.
func (a *app) lockedTerminalColumn() *column {
	col := a.rightmostLockedColumn()
	if col == nil {
		return nil
	}
	for _, t := range col.tabs {
		if t.term != nil {
			return col
		}
	}
	return nil
}

// toggleTerminalLockRight locks the right column to the terminal, or lets
// the column go when the terminal is locked there already.
func (a *app) toggleTerminalLockRight() {
	if col := a.terminalLocked(); col != nil {
		a.toggleColumnLock(col)
		return
	}
	a.moveTerminalToLockedRight()
}
