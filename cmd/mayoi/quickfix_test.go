package main

import (
	"path/filepath"
	"testing"

	"mayoi/internal/lsp"
)

func TestFixEdits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.go")
	doc := newDocument(1, path, "fmt.Println(x)\n", nil)
	d := newEditor(doc)
	a := &app{}
	edit := &lsp.WorkspaceEdit{
		Changes: map[string][]lsp.TextEdit{
			lsp.URI(path): {{
				Range:   lsp.Range{Start: lsp.Position{Line: 0, Character: 0}, End: lsp.Position{Line: 0, Character: 3}},
				NewText: "fmt",
			}},
		},
		DocumentChanges: []lsp.TextDocumentEdit{{
			TextDocument: struct {
				URI string `json:"uri"`
			}{URI: lsp.URI(path)},
			Edits: []lsp.TextEdit{{
				Range:   lsp.Range{Start: lsp.Position{Line: 0, Character: 13}, End: lsp.Position{Line: 0, Character: 14}},
				NewText: "y",
			}},
		}},
	}
	edits := a.fixEdits(d, edit)
	if len(edits) != 2 || edits[0].start != 0 || edits[0].end != 3 || edits[0].text != "fmt" ||
		edits[1].start != 13 || edits[1].end != 14 || edits[1].text != "y" {
		t.Errorf("fixEdits: %+v", edits)
	}
}

func TestLineDiags(t *testing.T) {
	doc := newDocument(1, "a.go", "x\n", nil)
	d := newEditor(doc)
	a := &app{lspDiags: map[string]lspFile{
		lsp.Key("a.go"): {path: "a.go", diags: []lsp.Diagnostic{
			{Range: lsp.Range{Start: lsp.Position{Line: 0}, End: lsp.Position{Line: 0}}},
			{Range: lsp.Range{Start: lsp.Position{Line: 2}, End: lsp.Position{Line: 2}}},
		}},
	}}
	if got := a.lineDiags(d, 0); len(got) != 1 {
		t.Errorf("line 0: %d diagnostics, want 1", len(got))
	}
	if got := a.lineDiags(d, 1); len(got) != 0 {
		t.Errorf("line 1: %d diagnostics, want 0", len(got))
	}
}
