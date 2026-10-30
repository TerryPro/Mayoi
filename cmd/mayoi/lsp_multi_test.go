package main

import (
	"os"
	"path/filepath"
	"testing"

	"mayoi/internal/settings"
)

func TestLSPRegistry(t *testing.T) {
	reg := newLSPRegistry(settings.Prefs{})
	for ext, want := range map[string]string{
		".go": "gopls", ".ts": "typescript-language-server", ".tsx": "typescript-language-server",
		".rs": "rust-analyzer", ".py": "pyright-langserver", ".cpp": "clangd",
	} {
		s := reg.byExt[ext]
		if s == nil || s.command != want {
			t.Errorf("%s: %+v, want %s", ext, s, want)
		}
	}
	if s := reg.byExt[".rb"]; s != nil {
		t.Errorf(".rb: %+v, want none", s)
	}

	p := settings.Prefs{LSPServers: map[string]settings.LSPServer{
		"ruby": {Extensions: []string{"rb"}, LanguageID: "ruby", Command: "ruby-lsp", Roots: []string{"Gemfile"}},
	}}
	reg = newLSPRegistry(p)
	if s := reg.byExt[".rb"]; s == nil || s.id != "ruby" || s.command != "ruby-lsp" {
		t.Errorf(".rb: %+v, want ruby-lsp", s)
	}
	if reg.byExt[".go"] == nil {
		t.Error("providing a server dropped Go")
	}

	p = settings.Prefs{LSPServers: map[string]settings.LSPServer{
		"typescript": {Command: "my-ts-server"},
	}}
	reg = newLSPRegistry(p)
	if s := reg.byExt[".ts"]; s == nil || s.command != "my-ts-server" || s.id != "typescript" {
		t.Errorf(".ts: %+v, want my-ts-server", s)
	}
}

func TestLSPLanguageIDs(t *testing.T) {
	reg := newLSPRegistry(settings.Prefs{})
	for ext, want := range map[string]string{
		".go": "go", ".ts": "typescript", ".tsx": "typescriptreact", ".js": "javascript",
		".jsx": "javascriptreact", ".rs": "rust", ".py": "python", ".c": "c", ".cpp": "cpp",
	} {
		s := reg.byExt[ext]
		if s == nil || s.id != want {
			t.Errorf("%s: %+v, want language %s", ext, s, want)
		}
	}
	root := filepath.Join("some", "project")
	ts, js, goSetup := reg.byExt[".ts"], reg.byExt[".js"], reg.byExt[".go"]
	if lspServerKey(ts, root) != lspServerKey(js, root) {
		t.Error("TypeScript and JavaScript do not share a server")
	}
	if lspServerKey(ts, root) == lspServerKey(goSetup, root) {
		t.Error("TypeScript and Go share a server")
	}
}

func TestLSPGen(t *testing.T) {
	s := settings.Default()
	gen := s.LSPGen
	s.SetPrefs(settings.Prefs{LSP: true})
	if s.LSPGen != gen+1 {
		t.Errorf("turning the servers on: gen %d, want %d", s.LSPGen, gen+1)
	}
	s.SetPrefs(settings.Prefs{LSP: true})
	if s.LSPGen != gen+1 {
		t.Errorf("no change: gen %d, want %d", s.LSPGen, gen+1)
	}
	p := settings.Prefs{LSP: true, LSPServers: map[string]settings.LSPServer{
		"ruby": {Command: "ruby-lsp"},
	}}
	s.SetPrefs(p)
	if s.LSPGen != gen+2 {
		t.Errorf("adding a server: gen %d, want %d", s.LSPGen, gen+2)
	}
}

func TestLSPProjectRoot(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "build", "lsp-test")
	if err := os.MkdirAll(filepath.Join(nested, "cmd"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{filepath.Join(root, "go.mod"), filepath.Join(nested, "go.mod")} {
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a := &app{root: root}
	if got := a.lspProjectRoot(filepath.Join(nested, "cmd"), []string{"go.mod"}); !sameDir(got, nested) {
		t.Errorf("nested root %q, want %q", got, nested)
	}
	if got := a.lspProjectRoot(filepath.Join(root, "cmd"), []string{"go.mod"}); !sameDir(got, root) {
		t.Errorf("module root %q, want %q", got, root)
	}
	outside := t.TempDir()
	if got := a.lspProjectRoot(outside, []string{"go.mod"}); !sameDir(got, outside) {
		t.Errorf("outside root %q, want %q", got, outside)
	}
}
