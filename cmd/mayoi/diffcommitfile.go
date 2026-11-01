package main

import (
	"path/filepath"

	"mayoi/internal/git"
)

// This file is the fork's share of the diff view: a commit's changes in one
// file, the tab a timeline opens for it, and the names those tabs take. It
// is kept out of diffview.go, which upstream also changes, so that a sync
// leaves the fork's own functions untouched.

// forkCommitInfo makes the info of a comparison's commit: the one the key
// names, whose parent is the commit it starts from.
func forkCommitInfo(from, to string, info git.Commit) git.Commit {
	if info.Hash == "" {
		info = git.Commit{Hash: to}
	}
	info.Hash, info.Parents = to, []string{from}
	return info
}

// fileTitle names the file a timeline compares in a tab, at a commit and a
// comparison; "" when the tab names none.
func (v *diffView) fileTitle(from, to string) string {
	if v.titlePath == "" {
		return ""
	}
	name := filepath.Base(v.titlePath)
	if from == "" {
		return name + " (" + git.ShortHash(to) + ")"
	}
	return name + " (" + git.ShortHash(from) + ") ↔ " + name + " (" + git.ShortHash(to) + ")"
}

// showsFile reports whether a commit's diff lists a file, a timeline's
// showing only the file it was opened for.
func (cm *commitDiff) showsFile(e git.Entry) bool {
	return cm.only == "" || e.Path == cm.only || e.Orig == cm.only
}

// fileCommitTab returns the tab of a commit's changes in only one file,
// nil when none shows it.
func (a *app) fileCommitTab(hash, only string, roots ...string) (*column, int) {
	root := a.diffRoot(roots)
	for _, col := range a.cols {
		if col.locked {
			continue
		}
		for i, t := range col.tabs {
			if t.diff != nil && t.diff.commit != nil && t.diff.commit.only == only &&
				t.diff.commit.key() == hash && a.diffBelongs(t.diff, root) {
				return col, i
			}
		}
	}
	return nil, -1
}

// showFileCommit shows the changes a commit made to only one file in a tab
// of its own, against the commit's first parent, and returns its diff.
func (a *app) showFileCommit(c git.Commit, only string, focus bool, roots ...string) *diffView {
	key := c.Hash
	if len(c.Parents) > 0 {
		key = c.Parents[0] + ".." + c.Hash
	}
	root := a.diffRoot(roots)
	if col, i := a.fileCommitTab(key, only, root); col != nil {
		a.keepTab(col.tabs[i])
		a.activate(col, i, focus)
		return col.tabs[i].diff
	}
	col := a.fileColumn()
	at := min(col.active+1, len(col.tabs))
	v := newCommitView(parseDiffLayout(a.settings.DiffLayout), key, c)
	v.commit.only = only
	v.titlePath = only
	v.repoRoot = root
	a.addTab(col, at, &colTab{diff: v})
	a.activate(col, at, focus)
	a.rememberTabs()
	return v
}

// showComparison shows the changes from one commit to another in a tab of
// its own, of only one file when only is not "", and returns its diff.
func (a *app) showComparison(from, to git.Commit, only string, focus bool, roots ...string) *diffView {
	key := from.Hash + ".." + to.Hash
	root := a.diffRoot(roots)
	if only != "" {
		if col, i := a.fileCommitTab(key, only, root); col != nil {
			a.keepTab(col.tabs[i])
			a.activate(col, i, focus)
			return col.tabs[i].diff
		}
	} else if col, i := a.commitTab(key, root); col != nil {
		a.keepTab(col.tabs[i])
		a.activate(col, i, focus)
		return col.tabs[i].diff
	}
	col := a.fileColumn()
	at := min(col.active+1, len(col.tabs))
	v := newCommitView(parseDiffLayout(a.settings.DiffLayout), key, to)
	v.commit.only = only
	if only != "" {
		v.titlePath = only
	}
	v.repoRoot = root
	a.addTab(col, at, &colTab{diff: v})
	a.activate(col, at, focus)
	a.rememberTabs()
	return v
}

// deferSeek keeps the line a file is to open at until its text comes in,
// and reports whether it did.
func (a *app) deferSeek(d *editor, line int) bool {
	if !d.loading {
		return false
	}
	d.seekLine, d.seekChar = line, 0
	return true
}

// shownPath returns the path of the file a diff shows: the one at its
// cursor, the one at the top of a commit's files, or the one it is about
// to reveal when its files are not read yet.
func (v *diffView) shownPath() string {
	if v.cursor >= 0 && v.cursor < len(v.rows) {
		if i := v.rows[v.cursor].file; i >= 0 && i < len(v.files) {
			return v.files[i].path
		}
	}
	if v.tree.top != nil {
		return v.tree.top.path
	}
	if v.synced != nil {
		return v.synced.path
	}
	if v.reveal != nil {
		return v.reveal.path
	}
	if len(v.files) > 0 {
		return v.files[0].path
	}
	return ""
}
