package main

import (
	"os"
	"testing"
	"time"

	"mayoi/internal/git"
	"mayoi/internal/settings"
	"mayoi/locales"
)

// The versions of a timeline are the work tree's change first, then its
// commits, each with the version before it.
func TestBuildTimeline(t *testing.T) {
	second := time.Date(2026, 10, 31, 12, 0, 0, 0, time.Local)
	first := second.Add(-time.Hour)
	commits := []git.FileCommit{
		{Commit: git.Commit{Hash: "bbb", Subject: "second", Author: "Ada", Time: second}, Status: 'M', Path: "/repo/f.go"},
		{Commit: git.Commit{Hash: "aaa", Subject: "first", Author: "Ada", Time: first}, Status: 'A', Path: "/repo/f.go"},
	}
	entries := buildTimeline("/repo/f.go", 'M', second, commits)
	if len(entries) != 3 {
		t.Fatalf("%d entries", len(entries))
	}
	if !entries[0].working || entries[0].status != 'M' || entries[0].path != "/repo/f.go" {
		t.Errorf("the working tree's entry is %+v", entries[0])
	}
	if entries[1].commit.Subject != "second" || entries[1].older == nil || entries[1].older.Hash != "aaa" {
		t.Errorf("the newest commit is %+v", entries[1])
	}
	if entries[2].commit.Subject != "first" || entries[2].older != nil {
		t.Errorf("the oldest commit is %+v", entries[2])
	}
}

// Dragging the timeline leaves the outline at its height, and the tree's
// room moves with it; a folded tree leaves the sections more.
func TestExplorerSectionHeights(t *testing.T) {
	w := &settings.Window{}
	w.OutlineHeight, w.TimelineOpen, w.TimelineHeight = 220, true, 190
	const pane = float32(400)
	outline, timeline, _, timelineMax := explorerSectionHeights(pane, w, "")
	if outline != 220 || timeline != 156 {
		t.Fatalf("outline %v, timeline %v", outline, timeline)
	}
	w.TimelineHeight = 300
	outline2, _, _, _ := explorerSectionHeights(pane, w, "")
	if outline2 != outline {
		t.Errorf("the timeline's height moved the outline from %v to %v", outline, outline2)
	}
	w.OutlineHeight = 250
	_, timeline, _, timelineMax = explorerSectionHeights(pane, w, "")
	if timelineMax <= timelineMinH || timeline < timelineMinH {
		t.Errorf("a tall outline leaves the timeline %v of %v", timeline, timelineMax)
	}
	w.FilesClosed = true
	_, _, _, more := explorerSectionHeights(pane, w, "")
	if more <= timelineMax {
		t.Errorf("a folded tree leaves the timeline %v, no more than %v", more, timelineMax)
	}
}

// The folder's chevron folds the files, and shows them again.
func TestExplorerFoldsFiles(t *testing.T) {
	hs := newHarness(t, true)
	a := hs.a
	a.window.TimelineOpen = true
	hs.settle()
	if !hs.tt.HasText("internal") {
		t.Fatalf("the files do not show: %q", hs.tt.Texts())
	}
	if err := hs.tt.Click(locales.ExplorerHideFiles()); err != nil {
		t.Fatalf("clicking the folder's chevron: %v", err)
	}
	hs.settle()
	if !a.window.FilesClosed || hs.tt.HasText("internal") {
		t.Errorf("the files stay: closed %v, %q", a.window.FilesClosed, hs.tt.Texts())
	}
	if !hs.tt.HasText("TIMELINE") {
		t.Errorf("the timeline does not take the files' room: %q", hs.tt.Texts())
	}
	hs.shot("explorer-folded")
	if err := hs.tt.Click(locales.ExplorerShowFiles()); err != nil {
		t.Fatalf("clicking the folder's chevron again: %v", err)
	}
	hs.settle()
	if a.window.FilesClosed || !hs.tt.HasText("internal") {
		t.Errorf("the files do not come back: closed %v", a.window.FilesClosed)
	}
}

// The timeline shows the history of a Markdown file in a narrow window,
// its header and rows within the sidebar.
func TestTimelineFitsANarrowWindow(t *testing.T) {
	hs := newHarness(t, true)
	hs.gitRepo()
	a := hs.a
	a.openFile(hs.path("README.md"), true)
	hs.settle()
	a.window.TimelineOpen = true
	hs.tt.SetSize(620, 520)
	a.invalidate()
	hs.settle()
	entries, msg := a.timelineEntries()
	if msg != "" || len(entries) != 1 {
		t.Fatalf("timeline %q %+v", msg, entries)
	}
	if !hs.tt.HasText("TIMELINE") {
		t.Errorf("the timeline's header does not show: %q", hs.tt.Texts())
	}
	if !hs.tt.HasText(entries[0].message) {
		t.Errorf("the timeline's row does not show: %q", hs.tt.Texts())
	}
	hs.shot("timeline")
	if err := hs.tt.Click(entries[0].message); err != nil {
		t.Fatalf("clicking the timeline row: %v", err)
	}
	hs.settle()
	if tab := a.activeTab(); tab == nil || tab.diff == nil {
		t.Errorf("a click on the timeline row did not open its changes")
	}
}

// The Explorer's timeline shows the history of the file shown, opens a
// commit's changes, and compares it with the version before.
func TestTimelineShowsAndCompares(t *testing.T) {
	hs := newHarness(t, true)
	gitCmd := hs.gitRepo()
	a := hs.a
	path := hs.path("main.go")
	if err := os.WriteFile(path, []byte("package main\n\nfunc main() {}\n\n// two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hs.path("README.md"), []byte("# Mayoi\n\nSecond.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCmd("add", ".")
	gitCmd("commit", "-q", "-m", "second")
	hs.settleGit()
	a.openFile(path, true)
	hs.settle()
	a.window.TimelineOpen = true
	a.invalidate()
	hs.settle()
	if !hs.tt.HasText("TIMELINE") {
		t.Errorf("the timeline's header does not show: %q", hs.tt.Texts())
	}
	entries, msg := a.timelineEntries()
	if msg != "" || len(entries) != 2 {
		t.Fatalf("timeline %q %+v", msg, entries)
	}
	if entries[0].commit.Subject != "second" || entries[0].older == nil || entries[0].older.Subject != "first" {
		t.Fatalf("the newest version is %+v", entries[0])
	}
	if err := os.WriteFile(path, []byte("package main\n\nfunc main() {}\n\n// three\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hs.settleGit()
	hs.settle()
	entries, msg = a.timelineEntries()
	if msg != "" || len(entries) != 3 {
		t.Fatalf("timeline with changes %q %+v", msg, entries)
	}
	if !entries[0].working || entries[0].status != 'M' {
		t.Errorf("the work tree's change is %+v", entries[0])
	}
	hs.shot("timeline-history")
	commit := entries[1]
	a.openTimelineEntry(commit)
	hs.settle()
	root := a.git.repo.Root
	key := commit.older.Hash + ".." + commit.commit.Hash
	want := "main.go (" + git.ShortHash(commit.older.Hash) + ") ↔ main.go (" + git.ShortHash(commit.commit.Hash) + ")"
	col, i := a.fileCommitTab(key, path, root)
	if col == nil {
		t.Errorf("the file's version is not open")
	} else {
		if name := col.tabs[i].name(); name != want {
			t.Errorf("the version's tab is %q, want %q", name, want)
		}
		if files := col.tabs[i].diff.files; len(files) != 1 || files[0].path != path {
			t.Errorf("the tab shows %d files, not only %q", len(files), path)
		}
		if cm := col.tabs[i].diff.commit; cm == nil || len(cm.files) != 2 {
			t.Errorf("the commit's files are not 2: %+v", cm)
		}
	}
	if a.activeEditor() != nil || a.shownPath() != path {
		t.Errorf("the diff does not show %q: %q", path, a.shownPath())
	}
	rows := a.outlineRows()
	if len(rows) != 1 || rows[0].node.sym.Name != "main" {
		t.Errorf("the outline of the diff's file is %+v", rows)
	}
	if got, msg := a.timelineEntries(); msg != "" || len(got) != 3 {
		t.Errorf("the timeline of the diff's file is %q %+v", msg, got)
	}
	hs.shot("timeline-diff")
	node := rows[0].node
	a.openOutlineNode(node)
	hs.settle()
	if d := a.activeEditor(); d == nil || d.path != path {
		t.Errorf("a click on the outline did not open %q", path)
	}
	a.compareTimelineEntry(commit)
	hs.settle()
	hs.shot("timeline-compare")
	if col, _ := a.fileCommitTab(key, path, root); col == nil {
		t.Errorf("the comparison is not open")
	}
}
