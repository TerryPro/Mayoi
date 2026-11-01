package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/bg"
	"mayoi/internal/git"
	"mayoi/internal/vfs"
	"mayoi/locales"
)

// timelineEntry is a version of the file shown: the working tree's, or a
// commit's, and the version before it for comparison.
type timelineEntry struct {
	path    string // the file's path at that version
	working bool
	commit  git.Commit
	older   *git.Commit // the version before this one, nil for the oldest
	status  byte        // A, M, D, R, C, T, U for untracked, G for a merge
	when    time.Time
	author  string
	message string
}

// timelineState is the timeline of the file shown: its versions, read for
// the file and the history they belong to.
type timelineState struct {
	// key is what entries are of, once read; keyReading is the one a read
	// running now is of.
	key        string
	keyReading string
	loading    bool
	failed     bool
	entries    []timelineEntry
	commits    int // how many commits the read returned
	count      int // how many commits were asked for
	path       string

	list     ui.ListState
	listView ui.Handle
	row      int
}

// timelinePage is how many commits a timeline reads at first, and more
// each time more are asked for.
const timelinePage = 50

// timelineMinH is the shortest timeline section, and timelineHeadH the
// height of its header.
const (
	timelineMinH  = 90
	timelineHeadH = 24
	timelineRowH  = 38
)

// timelineKey names what a timeline's entries are of: the file, the
// repository and the commit checked out, the file's state in the work tree,
// and how many commits are asked for.
func timelineKey(root, path, head string, state byte, count int) string {
	return fmt.Sprintf("%s\x00%s\x00%s\x00%c\x00%d", root, path, head, state, count)
}

// timelineEntries returns the timeline's entries and, when it has none to
// show, what to say instead.
func (a *app) timelineEntries() ([]timelineEntry, string) {
	s := &a.timeline
	path := a.shownPath()
	if path == "" {
		return nil, locales.TimelineNoFile()
	}
	ctx := a.repositoryForPath(path)
	if ctx.git.repo == nil {
		if ctx.git.looking || !ctx.git.looked {
			return nil, locales.TimelineLoading()
		}
		return nil, locales.TimelineNoRepo()
	}
	st := ctx.git.status
	if st == nil {
		return nil, locales.TimelineLoading()
	}
	var state byte
	if e, ok := ctx.git.entries[path]; ok {
		state = e.Letter()
	}
	count := s.count
	if count < timelinePage {
		count = timelinePage
	}
	key := timelineKey(ctx.git.repo.Root, path, st.Head, state, count)
	if key == s.key {
		if s.failed {
			return nil, locales.TimelineFailed()
		}
		if len(s.entries) == 0 {
			return nil, locales.TimelineNoHistory()
		}
		return s.entries, ""
	}
	if !s.loading {
		s.loading, s.keyReading = true, key
		a.readTimeline(ctx.git.repo, path, st.Head, state, count)
	}
	if s.loading && s.path == path && len(s.entries) > 0 {
		// More of the same file, or its history moved: the versions shown
		// stay until the new ones come.
		return s.entries, ""
	}
	return nil, locales.TimelineLoading()
}

// readTimeline reads the versions of a file in the background.
func (a *app) readTimeline(r *git.Repo, path, head string, state byte, count int) {
	when := time.Now()
	bg.Go(func() {
		commits, err := r.FileHistory(path, count)
		if state != 0 {
			if info, e := vfs.Stat(path); e == nil {
				when = info.ModTime()
			}
		}
		a.post(func() {
			s := &a.timeline
			if !s.loading || s.keyReading == "" {
				return
			}
			s.loading, s.keyReading = false, ""
			if s.path != path {
				s.row = -1
			}
			s.key, s.path, s.count, s.commits = timelineKey(r.Root, path, head, state, count), path, count, len(commits)
			if err != nil {
				s.failed, s.entries = true, nil
				a.invalidate()
				return
			}
			s.failed = false
			s.entries = buildTimeline(path, state, when, commits)
			a.invalidate()
		})
	})
}

// buildTimeline makes the versions of a timeline: the working tree's, when
// it differs, then its commits, newest first.
func buildTimeline(path string, state byte, when time.Time, commits []git.FileCommit) []timelineEntry {
	entries := make([]timelineEntry, 0, len(commits)+1)
	if state != 0 {
		entries = append(entries, timelineEntry{
			path: path, working: true, status: state, when: when,
			author: locales.TimelineYou(), message: locales.TimelineWorking(),
		})
	}
	for _, c := range commits {
		p := c.Path
		if p == "" {
			p = path
		}
		e := timelineEntry{path: p, commit: c.Commit, status: c.Status, when: c.Time, author: c.Author, message: c.Subject}
		if len(c.Parents) > 1 {
			e.status = 'G'
		}
		if e.message == "" {
			e.message = git.ShortHash(c.Hash)
		}
		entries = append(entries, e)
	}
	for i := range entries {
		if entries[i].working {
			continue
		}
		for j := i + 1; j < len(entries); j++ {
			if entries[j].working {
				continue
			}
			older := entries[j].commit
			entries[i].older = &older
			break
		}
	}
	return entries
}

// timelineSection shows the timeline in the Explorer, under the outline.
func (a *app) timelineSection(c *ui.Context, fill bool) {
	p, w := a.pal, a.window
	open := w.TimelineOpen
	_, height, _, maxH := explorerSectionHeights(explorerPaneHeight(c), w, a.settings.OutlinePosition)
	box := ui.Column(c.Key("timeline-section")).Shrink(0).Background(p.chrome).
		BorderWidth(1, 0, 0, 0).BorderColor(p.border)
	switch {
	case !open:
		box.Height(timelineHeadH)
	case fill:
		// Without the files, the timeline takes the room they leave.
		box.Grow(1).MinHeight(timelineMinH)
	default:
		box.Height(height)
	}
	box.Children(func() {
		a.timelineHeader(c, open)
		if !open {
			return
		}
		a.timelineBody(c)
		if !fill {
			a.splitter(c, edgeTop, func(d float32) {
				w.TimelineHeight = clamp(w.TimelineHeight-d, timelineMinH, maxH)
			})
		}
	})
}

// timelineHeader is the section's header: the arrow that folds it, and its
// title.
func (a *app) timelineHeader(c *ui.Context, open bool) {
	p, w := a.pal, a.window
	b := ui.ButtonBase(c.Key("timeline-header")).Height(timelineHeadH).Padding(0, 4, 0, 8).Gap(4).
		Justify(ui.Start).AlignItems(ui.Center).Label(locales.TimelineTitle())
	b.Children(func() {
		sidebarArrow(c, open, p.muted)
		ui.Text(c, strings.ToUpper(locales.TimelineTitle())).FontSize(11).FontWeight(600).
			LetterSpacing(titleSpacing).TextColor(p.muted).SingleLine()
		ui.Spacer(c)
	})
	if b.Hovered() {
		b.Background(p.hover)
	}
	b.Transition(hoverFade)
	if b.Clicked() {
		w.TimelineOpen = !w.TimelineOpen
		a.settings.Changed()
		a.invalidate()
	}
}

// timelineBody is what shows under the header: the versions of the file,
// or why it has none.
func (a *app) timelineBody(c *ui.Context) {
	p := a.pal
	entries, msg := a.timelineEntries()
	if msg != "" {
		ui.Column(c).Grow(1).MinHeight(0).Padding(14, 12).AlignItems(ui.Start).Children(func() {
			ui.Text(c, msg).FontSize(11.5).TextColor(p.faint).LineHeight(1.4)
		})
		return
	}
	s := &a.timeline
	s.list.Selected = &s.row
	s.list.Key = func(i int) any { return timelineEntryKey(entries[i]) }
	s.list.Label = func(i int) string { return entries[i].message }
	base := c.Theme()
	quiet := *base
	quiet.Accent, quiet.AccentText, quiet.SurfaceHover = p.active, p.text, p.hover
	c.SetTheme(&quiet)
	out := ui.List(c.Key("timeline"), &s.list, len(entries), func(i int) {
		a.timelineRow(c, entries[i], i)
	}).Grow(1).MinHeight(0).Padding(0, 4, 4, 24).Bind(&s.listView)
	c.SetTheme(base)
	if s.list.Focused(c) && s.list.Shortcut(c, 0, ui.KeyEscape) {
		a.focusColumn()
	}
	if out.Submitted() && s.row >= 0 && s.row < len(entries) {
		e := entries[s.row]
		a.post(func() { a.openTimelineEntry(e) })
	}
	if s.commits >= s.count {
		a.timelineMore(c)
	}
}

// timelineMore shows a button that reads more of the file's history.
func (a *app) timelineMore(c *ui.Context) {
	p := a.pal
	ui.Row(c).Height(26).Padding(0, 6).Gap(6).AlignItems(ui.Center).
		BorderWidth(1, 0, 0, 0).BorderColor(p.border).Children(func() {
		b := ui.ButtonBase(c.Key("timeline-more")).Height(22).Padding(0, 8).Radius(4).
			Justify(ui.Start).AlignItems(ui.Center).Label(locales.TimelineShowMore())
		if b.Hovered() {
			b.Background(p.hover)
		}
		b.Transition(hoverFade)
		b.Children(func() {
			ui.Text(c, locales.TimelineShowMore()).FontSize(11.5).TextColor(p.muted).SingleLine()
		})
		if b.Clicked() {
			a.timeline.count += timelinePage
			a.invalidate()
		}
	})
}

// timelineEntryKey names a version of the timeline.
func timelineEntryKey(e timelineEntry) string {
	if e.working {
		return "work"
	}
	return e.commit.Hash
}

// timelineRow is a version of the timeline: its mark, its message, and what
// it did, who did it, and when.
func (a *app) timelineRow(c *ui.Context, e timelineEntry, i int) {
	p := a.pal
	row := ui.Row(c).Grow(1).Height(timelineRowH).Gap(6).AlignItems(ui.Center)
	row.Children(func() {
		icon, col := a.timelineIcon(e)
		ui.Icon(c, icon).FontSize(14).TextColor(col).Shrink(0)
		item := ui.ButtonBase(c.Key("timeline-item:" + timelineEntryKey(e))).Grow(1).MinWidth(0).Height(timelineRowH).
			Radius(4).Justify(ui.Start).AlignItems(ui.Center).Label(e.message)
		item.Children(func() {
			detail := a.timelineDetail(e)
			ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
				ui.Text(c, e.message).FontSize(12).TextColor(p.text).SingleLine().Shrink(1)
				ui.Text(c, detail).FontSize(10.5).TextColor(p.faint).SingleLine().Shrink(1).MinWidth(0).
					Tooltip(timelineFullWhen(e.when))
			})
		})
		if item.Hovered() {
			item.Background(p.hover)
		}
		item.Transition(hoverFade)
		item.ContextMenu(func(m *ui.Menu) {
			if m.Item(locales.TimelineCompare()).Disabled(e.working || e.older == nil).Chosen() {
				a.post(func() { a.compareTimelineEntry(e) })
			}
		})
		if item.Clicked() {
			a.timeline.row = i
			a.post(func() { a.openTimelineEntry(e) })
		}
		a.timelineCompareButton(c, e)
	})
}

// timelineCompareButton is the button of a version that compares it with
// the one before.
func (a *app) timelineCompareButton(c *ui.Context, e timelineEntry) {
	p := a.pal
	if e.working || e.older == nil {
		ui.Box(c).Width(20).Shrink(0)
		return
	}
	b := ui.ButtonBase(c.Key("timeline-compare:"+timelineEntryKey(e))).Size(20, 20).Radius(4).
		Justify(ui.Center).AlignItems(ui.Center).Label(locales.TimelineCompare())
	b.Children(func() { ui.Icon(c, iconDiff).FontSize(12).TextColor(p.muted) })
	if b.Hovered() {
		b.Background(p.hover)
	}
	b.Tooltip(locales.TimelineCompare())
	b.Transition(hoverFade)
	if b.Clicked() {
		a.post(func() { a.compareTimelineEntry(e) })
	}
}

// timelineIcon is the mark of a version, and its color.
func (a *app) timelineIcon(e timelineEntry) (*ui.SVG, ui.Color) {
	p := a.pal
	if e.working {
		return iconPencil, p.modifiedFg
	}
	switch e.status {
	case 'A', 'U':
		return iconFilePlus, p.addedFg
	case 'D':
		return iconTrash, p.removedFg
	case 'R':
		return iconArrowRight, p.muted
	case 'C':
		return iconCopy, p.muted
	case 'G':
		return iconMerge, p.muted
	}
	return iconCommit, p.modifiedFg
}

// timelineDetail is what a version did, who did it, and when.
func (a *app) timelineDetail(e timelineEntry) string {
	kind := a.timelineKind(e)
	author := e.author
	if author == "" {
		author = git.ShortHash(e.commit.Hash)
	}
	when := timelineWhen(e.when)
	if author == "" {
		return kind
	}
	if when == "" {
		return kind + " · " + author
	}
	return kind + " · " + author + " · " + when
}

// timelineKind is what a version did to the file.
func (a *app) timelineKind(e timelineEntry) string {
	switch e.status {
	case 'A', 'U':
		return locales.TimelineAdded()
	case 'D':
		return locales.TimelineDeleted()
	case 'R':
		return locales.TimelineRenamed()
	case 'C':
		if e.working {
			return locales.TimelineModified()
		}
		return locales.TimelineCopied()
	case 'G':
		return locales.TimelineMerge()
	}
	return locales.TimelineModified()
}

// timelineWhen returns when a version was made, short.
func timelineWhen(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	if t.Year() == time.Now().Year() {
		return t.Format("01-02 15:04")
	}
	return t.Format("2006-01-02")
}

// timelineFullWhen returns when a version was made, whole.
func timelineFullWhen(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02 15:04:05")
}

// openTimelineEntry shows the changes of a version, and scrolls its diff to
// the file.
func (a *app) openTimelineEntry(e timelineEntry) {
	ctx := a.repositoryForPath(e.path)
	var root string
	if ctx.git.repo != nil {
		root = ctx.git.repo.Root
	}
	if e.working {
		a.showChanges(diffTarget{path: e.path, any: true}, true, root)
		return
	}
	// Only this file's change in this commit shows, as VS Code's timeline
	// does; not every file the commit changed.
	a.showFileCommit(e.commit, e.path, true, root)
}

// compareTimelineEntry shows the changes from the version before to the
// one chosen, and scrolls its diff to the file.
func (a *app) compareTimelineEntry(e timelineEntry) {
	if e.working || e.older == nil {
		return
	}
	ctx := a.repositoryForPath(e.path)
	var root string
	if ctx.git.repo != nil {
		root = ctx.git.repo.Root
	}
	v := a.showComparison(*e.older, e.commit, e.path, true, root)
	v.changed()
}
