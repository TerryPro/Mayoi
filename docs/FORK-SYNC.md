# Syncing with upstream

The fork lives on `feat/tab-indent-and-menubar`, and upstream's `main` is
merged into it; `main` is never where the fork's work goes.

## The routine

```sh
git fetch upstream
git switch -c sync/upstream-<version> feat/tab-indent-and-menubar
git merge upstream/main
# resolve conflicts, in the order below
go build ./...
go vet ./...
go test ./cmd/mayoi ./internal/...
git switch feat/tab-indent-and-menubar
git merge --ff-only sync/upstream-<version>
```

A sync branch that will not resolve is thrown away, and the fork's line is
untouched. `git config rerere.enabled` is on, so a conflict seen before is
resolved as it was last time.

## What conflicts, and what to do

1. `locales/catalog_gen.go` is generated: take upstream's copy and run
   `go tool mygo generate` afterwards. Never hand-merge it.
2. `locales/en.json`, `ja.json`, `zh-Hans.json`: take upstream's, then add
   the fork's keys back, the ones for the outline, timeline, explorer fold
   and language server panels.
3. `cmd/mayoi/editor.go`: keep upstream's code and restore the calls into
   `editorfork.go` — `editorForkHooks`, `editorForkKeys`,
   `editorForkOverlays`, `forkProblems`, `drawForkDiags`,
   `drawForkDiagMarks`, `drawForkFoldGutter`, `drawForkFoldBadges`,
   `lspForkOverlays`. The drawing and the fold checks live in the fork's
   file, not here.
4. `cmd/mayoi/diffview.go`: keep upstream's code and the fork's anchors —
   `forkCommitInfo`, `fileTitle`, `showsFile`, `deferSeek`, and the
   `titlePath` field, which sits in a block of its own.
5. `cmd/mayoi/sidebar.go`: keep upstream's switches and panes, then restore
   the outline switch, its title case, `sidebarTitleRoom` and the outline
   pane's branch. `explorerView` is in `explorerpane.go`.
6. `CHANGELOG.md`: take upstream's, then put the fork's bullets back at the
   top of its own `## <version>` section.
7. `docs/`: upstream's wording wins unless it describes a fork feature.

## When adding a feature

- Put it in a file of its own. Upstream only ever edits upstream's files, so
  a file it does not know never conflicts.
- Wire it into an upstream file with one call, kept apart from upstream's
  neighbouring lines by a blank line or a comment of its own.
- Put new fields of upstream's structs in their own blank-line separated
  block: `gofmt` then leaves upstream's fields and their alignment alone.
- Keep generated files out of hand edits; regenerate them after a merge.

The fork's own files: `folding.go`, `outline.go`, `timeline.go`, `lsp.go`,
`complete.go`, `quickfix.go`, `problems.go`, `menubar.go`, `editorfork.go`,
`diffcommitfile.go`, `explorerpane.go`, `termlist.go`, `termtab.go`.
