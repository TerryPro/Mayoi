package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/bg"
	"mayoi/internal/lsp"
	"mayoi/internal/remote"
	"mayoi/internal/settings"
	"mayoi/locales"
)

// lspFile is the diagnostics of a file, and the path they are of.
type lspFile struct {
	path  string
	diags []lsp.Diagnostic
}

// lspSetup is a language server Mayoi knows: the language's id, the
// extensions of its files, its command, and the files that mark its
// project's root.
type lspSetup struct {
	name    string
	id      string
	exts    []string
	command string
	args    []string
	roots   []string
}

// lspBuiltin are the language servers Mayoi runs without a setting.
var lspBuiltin = map[string]lspSetup{
	"go": {id: "go", exts: []string{".go"}, command: "gopls", roots: []string{"go.mod", "go.work"}},
	"typescript": {id: "typescript", exts: []string{".ts", ".mts", ".cts"},
		command: "typescript-language-server", args: []string{"--stdio"},
		roots: []string{"tsconfig.json", "jsconfig.json", "package.json"}},
	"typescriptreact": {id: "typescriptreact", exts: []string{".tsx"},
		command: "typescript-language-server", args: []string{"--stdio"},
		roots: []string{"tsconfig.json", "jsconfig.json", "package.json"}},
	"javascript": {id: "javascript", exts: []string{".js", ".mjs", ".cjs"},
		command: "typescript-language-server", args: []string{"--stdio"},
		roots: []string{"jsconfig.json", "package.json"}},
	"javascriptreact": {id: "javascriptreact", exts: []string{".jsx"},
		command: "typescript-language-server", args: []string{"--stdio"},
		roots: []string{"jsconfig.json", "package.json"}},
	"rust": {id: "rust", exts: []string{".rs"}, command: "rust-analyzer", roots: []string{"Cargo.toml"}},
	"python": {id: "python", exts: []string{".py", ".pyi"},
		command: "pyright-langserver", args: []string{"--stdio"},
		roots: []string{"pyproject.toml", "setup.py", "setup.cfg", "pyrightconfig.json"}},
	"c": {id: "c", exts: []string{".c", ".h"}, command: "clangd",
		roots: []string{"compile_commands.json", "CMakeLists.txt", ".clangd"}},
	"cpp": {id: "cpp", exts: []string{".cc", ".cpp", ".cxx", ".hpp", ".hxx"}, command: "clangd",
		roots: []string{"compile_commands.json", "CMakeLists.txt", ".clangd"}},
}

// lspRegistry is the language servers of the settings, by language, and
// the one of each file extension.
type lspRegistry struct {
	byName map[string]*lspSetup
	byExt  map[string]*lspSetup
}

// newLSPRegistry makes the language servers of the preferences: the ones
// Mayoi knows, and those of lspServers, which a name of its own overrides.
func newLSPRegistry(p settings.Prefs) *lspRegistry {
	byName := map[string]*lspSetup{}
	var builtin []string
	for name, b := range lspBuiltin {
		s := b
		s.name = name
		if s.id == "" {
			s.id = name
		}
		byName[name] = &s
		builtin = append(builtin, name)
	}
	slices.Sort(builtin)
	var extra []string
	for name, u := range p.LSPServers {
		s := byName[name]
		if s == nil {
			s = &lspSetup{name: name, id: name}
			byName[name] = s
			extra = append(extra, name)
		}
		if exts := extKeys(u.Extensions); len(exts) > 0 {
			s.exts = exts
		}
		if u.LanguageID != "" {
			s.id = u.LanguageID
		}
		if cmd := strings.TrimSpace(u.Command); cmd != "" {
			s.command, s.args = cmd, u.Args
		}
		if len(u.Roots) > 0 {
			s.roots = u.Roots
		}
	}
	if cmd := strings.TrimSpace(p.LSPCommand); cmd != "" {
		if s := byName["go"]; s != nil {
			s.command, s.args = cmd, nil
		}
	}
	slices.Sort(extra)
	reg := &lspRegistry{byName: map[string]*lspSetup{}, byExt: map[string]*lspSetup{}}
	add := func(names []string) {
		for _, name := range names {
			s := byName[name]
			if s == nil || s.command == "" || len(s.exts) == 0 {
				continue
			}
			reg.byName[name] = s
			for _, e := range s.exts {
				if _, ok := reg.byExt[e]; !ok {
					reg.byExt[e] = s
				}
			}
		}
	}
	add(builtin)
	add(extra)
	return reg
}

// extKeys reads the extensions of a setting, each with a dot, in lower
// case.
func extKeys(exts []string) []string {
	out := make([]string, 0, len(exts))
	for _, e := range exts {
		e = strings.ToLower(strings.TrimSpace(e))
		if e == "" {
			continue
		}
		if !strings.HasPrefix(e, ".") {
			e = "." + e
		}
		out = append(out, e)
	}
	return out
}

// lspServer is a language server's process for a project root, and the
// documents it has open.
type lspServer struct {
	key     string
	root    string
	command string
	args    []string
	client  *lsp.Client
	open    map[string]uint64
	// ready is the version of each document the server was told of, by
	// didOpen or didChange: questions about it come after those.
	ready  map[string]uint64
	timers map[string]*bg.Timer
	// tried is set once it was started, and err why it could not be, or
	// why it ended.
	tried bool
	err   string
}

// lspServerKey is the key of a server: its command, arguments and root,
// which servers of two languages share when they run the same way in the
// same place, as TypeScript and JavaScript do.
func lspServerKey(setup *lspSetup, root string) string {
	return setup.command + "\x00" + strings.Join(setup.args, "\x00") + "\x00" + lsp.Key(root)
}

// lspTick starts the language servers of the documents open in the editor,
// each for the project of its own, and keeps their documents and
// diagnostics in step.
func (a *app) lspTick() {
	if a.lspRoot != a.root {
		a.resetLSP()
	}
	if !a.settings.LSP {
		if a.lspReg != nil || len(a.lspServers) > 0 {
			a.resetLSP()
		}
		return
	}
	if a.lspReg == nil || a.lspGen != a.settings.LSPGen {
		a.resetLSP()
		a.lspReg, a.lspGen = newLSPRegistry(a.settings.Prefs), a.settings.LSPGen
	}
	for _, d := range a.docs {
		if d.path == "" || remote.Is(d.path) {
			continue
		}
		setup := a.lspReg.byExt[strings.ToLower(filepath.Ext(d.path))]
		if setup == nil {
			continue
		}
		srv := a.lspServerFor(setup, a.lspRootOf(d.path, setup))
		if srv == nil || srv.client == nil {
			continue
		}
		uri, ver := lsp.URI(d.path), d.buf.Version()
		if sent, ok := srv.open[uri]; ok {
			if sent < ver {
				a.sendLSPChange(srv, d, uri, ver)
			}
			continue
		}
		a.openLSPDoc(srv, d, uri, ver, setup.id)
	}
	for _, srv := range a.lspServers {
		for uri := range srv.open {
			if !a.lspDocOpen(uri) {
				a.closeLSPDoc(srv, uri)
			}
		}
	}
}

// lspServerFor returns the server of a setup in a root, starting it the
// first time it is needed, nil while it has not started.
func (a *app) lspServerFor(setup *lspSetup, root string) *lspServer {
	if a.lspServers == nil {
		a.lspServers = map[string]*lspServer{}
	}
	key := lspServerKey(setup, root)
	if srv, ok := a.lspServers[key]; ok {
		return srv
	}
	srv := &lspServer{
		key: key, root: root, command: setup.command, args: setup.args, tried: true,
		open: map[string]uint64{}, ready: map[string]uint64{}, timers: map[string]*bg.Timer{},
	}
	a.lspServers[key] = srv
	a.startLSPServer(srv)
	return srv
}

// startLSPServer runs a language server, and starts its session in the
// background.
func (a *app) startLSPServer(srv *lspServer) {
	server, err := serverCommand(srv.command)
	if err != nil {
		srv.err = err.Error()
		return
	}
	c, err := lsp.Start(server, srv.args, srv.root, func(uri string, diagnostics []lsp.Diagnostic) {
		path := lsp.Path(uri)
		if path == "" {
			return
		}
		key := lsp.Key(path)
		a.post(func() {
			if a.lspDiags == nil {
				a.lspDiags = map[string]lspFile{}
			}
			a.lspDiags[key] = lspFile{path: path, diags: diagnostics}
			a.invalidate()
		})
	})
	if err != nil {
		srv.err = err.Error()
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	root := srv.root
	bg.Go(func() {
		defer cancel()
		if err := c.Initialize(ctx, root); err != nil {
			c.Close()
			a.post(func() {
				if a.lspServers[srv.key] == srv {
					srv.err = err.Error()
					a.invalidate()
				}
			})
			return
		}
		a.post(func() {
			if a.lspServers[srv.key] != srv {
				c.Close()
				return
			}
			srv.client = c
			srv.err = ""
			a.invalidate()
		})
	})
}

// resetLSP stops every language server, and forgets their documents and
// diagnostics.
func (a *app) resetLSP() {
	for _, srv := range a.lspServers {
		for _, t := range srv.timers {
			t.Stop()
		}
		if c := srv.client; c != nil {
			bg.Go(func() { c.Close() })
		}
	}
	a.lspServers = map[string]*lspServer{}
	a.lspDiags = map[string]lspFile{}
	a.lspRoots = map[string]string{}
	a.lspReg, a.lspGen = nil, -1
	a.lspRoot = a.root
	// The folds the servers found were of them.
	for _, doc := range a.docs {
		for _, d := range doc.views {
			d.lspFolds, d.lspFoldVersion, d.lspFoldPending, d.lspFoldUnsupported = nil, 0, false, false
		}
	}
}

// lspRootOf returns the folder of a document's project: the nearest, from
// the document's own up to the window's, that holds one of the setup's
// root markers.
func (a *app) lspRootOf(path string, setup *lspSetup) string {
	dir := filepath.Dir(path)
	key := lsp.Key(dir) + "\x00" + setup.name
	if root, ok := a.lspRoots[key]; ok {
		return root
	}
	root := a.lspProjectRoot(dir, setup.roots)
	if a.lspRoots == nil {
		a.lspRoots = map[string]string{}
	}
	a.lspRoots[key] = root
	return root
}

// lspProjectRoot finds the project a folder is of, by the markers of its
// root, up to the window's folder, or the folder itself when it is outside
// the window's, or none holds a marker.
func (a *app) lspProjectRoot(dir string, markers []string) string {
	start := filepath.Clean(dir)
	for {
		for _, m := range markers {
			if info, err := os.Stat(filepath.Join(dir, m)); err == nil && !info.IsDir() {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		if a.root != "" && sameDir(dir, a.root) {
			break
		}
		if a.root != "" && !withinDir(a.root, dir) {
			break
		}
		dir = parent
	}
	if a.root != "" && withinDir(a.root, start) {
		return a.root
	}
	return start
}

// sameDir reports whether two folders are the same, whatever the case of
// their letters on Windows.
func sameDir(x, y string) bool {
	return lsp.Key(filepath.Clean(x)) == lsp.Key(filepath.Clean(y))
}

// withinDir reports whether a folder is one of another's, itself
// included.
func withinDir(root, dir string) bool {
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return false
	}
	return rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// lspTarget returns the setup and server of a document, nil when its
// language has none, or its server has not been started yet.
func (a *app) lspTarget(path string) (*lspSetup, *lspServer) {
	if a.lspReg == nil {
		return nil, nil
	}
	setup := a.lspReg.byExt[strings.ToLower(filepath.Ext(path))]
	if setup == nil {
		return nil, nil
	}
	return setup, a.lspServers[lspServerKey(setup, a.lspRootOf(path, setup))]
}

// openLSPDoc tells a server of a document opened in the editor.
func (a *app) openLSPDoc(srv *lspServer, d *document, uri string, version uint64, id string) {
	text := d.buf.Snapshot().String()
	c := srv.client
	srv.open[uri] = version
	bg.Go(func() {
		_ = c.DidOpen(uri, id, text)
		a.post(func() {
			if a.lspServers[srv.key] == srv {
				srv.ready[uri] = version
			}
		})
	})
}

// sendLSPChange tells a server of a document's text, once it stops
// changing for a moment.
func (a *app) sendLSPChange(srv *lspServer, d *document, uri string, version uint64) {
	if t := srv.timers[uri]; t != nil {
		t.Stop()
	}
	srv.open[uri] = version
	c := srv.client
	srv.timers[uri] = bg.AfterFunc(250*time.Millisecond, func() {
		_ = c.DidChange(uri, int(version), d.buf.Snapshot().String())
		a.post(func() {
			delete(srv.timers, uri)
			// The server has the text now, and its folds are asked for.
			if a.lspServers[srv.key] == srv {
				srv.ready[uri] = version
			}
			a.invalidate()
		})
	})
}

// closeLSPDoc tells a server of a document closed in the editor.
func (a *app) closeLSPDoc(srv *lspServer, uri string) {
	delete(srv.open, uri)
	delete(srv.ready, uri)
	if path := lsp.Path(uri); path != "" {
		delete(a.lspDiags, lsp.Key(path))
	}
	if t := srv.timers[uri]; t != nil {
		t.Stop()
		delete(srv.timers, uri)
	}
	c := srv.client
	bg.Go(func() { _ = c.DidClose(uri) })
}

// lspDocOpen reports whether a document of the folder is open in the
// editor.
func (a *app) lspDocOpen(uri string) bool {
	path := lsp.Path(uri)
	if path == "" {
		return false
	}
	key := lsp.Key(path)
	for _, d := range a.docs {
		if d.path != "" && lsp.Key(d.path) == key {
			return true
		}
	}
	return false
}

// requestLSPFolds asks a document's language server for the folds of its
// text, once for each version, while the server has the text open.
func (a *app) requestLSPFolds(d *editor, snap ui.TextSnapshot) {
	if !a.settings.LSP || d.path == "" || d.lspFoldPending || d.lspFoldUnsupported || d.lspFoldVersion == snap.Version() {
		return
	}
	setup, srv := a.lspTarget(d.path)
	if setup == nil || srv == nil || srv.client == nil {
		return
	}
	uri, ver := lsp.URI(d.path), snap.Version()
	if ready, ok := srv.ready[uri]; !ok || ready < ver {
		// The server has not the text yet: the folds it would find are of
		// the text before, or of none.
		return
	}
	c := srv.client
	d.lspFoldPending = true
	bg.Go(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		ranges, err := c.FoldingRange(ctx, uri)
		a.post(func() {
			d.lspFoldPending = false
			if a.lspServers[srv.key] != srv || d.buf.Snapshot().Version() != ver {
				// Another server, or other text since: the folds wait for
				// the next frame to ask again.
				return
			}
			d.lspFoldVersion = ver
			if err != nil {
				if errors.Is(err, lsp.ErrUnsupported) {
					d.lspFoldUnsupported = true
				}
				return
			}
			d.lspFolds = foldRangesOf(ranges, snap.LineCount())
			// Folds made of every fold, or of a level, are made again of
			// the server's, which may fold more than the brackets do.
			switch d.foldKind {
			case foldEvery:
				a.foldAllIn(d)
			case foldLevels:
				a.foldLevelIn(d, d.foldLevel)
			}
			a.invalidate()
		})
	})
}

// foldRangesOf returns the folds a server found, without the lines beyond
// the text, and without those that hide none: one range per start line, the
// longest.
func foldRangesOf(ranges []lsp.FoldingRange, lines int) []foldRange {
	out := make([]foldRange, 0, len(ranges))
	for _, r := range ranges {
		start, end := r.StartLine, r.EndLine
		if start < 0 || end < start || lines <= 0 {
			continue
		}
		start, end = min(start, lines-1), min(end, lines-1)
		if end <= start {
			continue
		}
		out = append(out, foldRange{start: start, end: end})
	}
	if len(out) == 0 {
		return nil
	}
	slices.SortFunc(out, func(x, y foldRange) int {
		if x.start != y.start {
			return x.start - y.start
		}
		return y.end - x.end
	})
	kept := out[:0]
	for i, f := range out {
		if i > 0 && f.start == out[i-1].start {
			continue
		}
		kept = append(kept, f)
	}
	return kept
}

// lspProblems counts the errors and warnings of a document, as the servers
// last sent them.
func (a *app) lspProblems(path string) (errors, warnings int) {
	for _, d := range a.lspDiags[lsp.Key(path)].diags {
		switch d.Severity {
		case 1:
			errors++
		case 2:
			warnings++
		}
	}
	return errors, warnings
}

// serverCommand resolves the command of a language server: the one of the
// settings, or of those Mayoi knows, also in Go's bin folder, which go
// install fills and a shell may not have on PATH.
func serverCommand(cmd string) (string, error) {
	if path, err := exec.LookPath(cmd); err == nil {
		return path, nil
	}
	if strings.ContainsAny(cmd, `/\`) {
		return "", fmt.Errorf("%s: %w", cmd, exec.ErrNotFound)
	}
	var dirs []string
	if bin := os.Getenv("GOBIN"); bin != "" {
		dirs = append(dirs, bin)
	}
	for _, gopath := range filepath.SplitList(os.Getenv("GOPATH")) {
		if gopath != "" {
			dirs = append(dirs, filepath.Join(gopath, "bin"))
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, "go", "bin"))
	}
	for _, dir := range dirs {
		for _, name := range []string{cmd, cmd + ".exe"} {
			p := filepath.Join(dir, name)
			if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() {
				return p, nil
			}
		}
	}
	return "", fmt.Errorf("%s: %w", cmd, exec.ErrNotFound)
}

// gotoDefinition opens the definition of the symbol at the caret, as the
// document's language server finds it.
func (a *app) gotoDefinition() {
	d := a.activeEditor()
	if d == nil || d.path == "" {
		return
	}
	setup, srv := a.lspTarget(d.path)
	if setup == nil {
		return
	}
	if srv == nil || srv.client == nil {
		a.tell(note{message: locales.LspOff()})
		return
	}
	snap := d.buf.Snapshot()
	caret := min(d.caret, snap.Len())
	pos := lspPosition(snap, caret)
	c, uri := srv.client, lsp.URI(d.path)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	bg.Go(func() {
		defer cancel()
		locs, err := c.Definition(ctx, uri, pos)
		if err != nil || len(locs) == 0 {
			a.post(func() { a.tell(note{message: locales.LspNoDefinition()}) })
			return
		}
		a.post(func() { a.openLocation(locs[0]) })
	})
}

// openLocation shows the place a definition is at, at the line and
// character of its range.
func (a *app) openLocation(loc lsp.Location) { a.revealLocation(loc, true) }

// revealLocation shows the place a location is at, keeping the keyboard
// where it was unless focus is set.
func (a *app) revealLocation(loc lsp.Location, focus bool) {
	path := lsp.Path(loc.URI)
	if path == "" {
		return
	}
	a.openFile(path, focus)
	d := a.activeEditor()
	if d == nil {
		return
	}
	snap := d.buf.Snapshot()
	if snap.LineCount() == 0 {
		return
	}
	line := min(max(loc.Range.Start.Line, 0), snap.LineCount()-1)
	offset := characterOffset(snap, line, loc.Range.Start.Character)
	d.selectRange(offset, offset, true)
}

// lspPosition returns where offset is, as the language server spells
// positions: the line, and the character in UTF-16 units.
func lspPosition(snap ui.TextSnapshot, offset int) lsp.Position {
	line := snap.LineAt(offset)
	s, _ := snap.LineRange(line)
	return lsp.Position{Line: line, Character: snap.UTF16Offset(offset) - snap.UTF16Offset(s)}
}

// characterOffset returns the rune offset of a line where its character
// character stands, in UTF-16 units.
func characterOffset(snap ui.TextSnapshot, line, character int) int {
	s, e := snap.LineRange(line)
	return min(snap.RuneOffset(snap.UTF16Offset(s)+max(character, 0)), e)
}
