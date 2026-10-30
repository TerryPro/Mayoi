package main

import (
	"testing"

	"mayoi/internal/lsp"
)

func TestProblemsFilterAndFold(t *testing.T) {
	a := &app{lspDiags: map[string]lspFile{
		"a": {path: "a.go", diags: []lsp.Diagnostic{
			{Severity: 1, Message: "bad value"},
			{Severity: 2, Message: "unused"},
		}},
		"b": {path: "b.go", diags: []lsp.Diagnostic{{Severity: 1, Message: "other file"}}},
	}}
	if n := len(a.problemsShown()); n != 3 {
		t.Errorf("%d problems, want 3", n)
	}
	a.problemsUI.severity = 1
	if ps := a.problemsShown(); len(ps) != 2 {
		t.Errorf("%d errors, want 2", len(ps))
	}
	a.problemsUI.severity = 0
	a.problemsUI.query = "other"
	if ps := a.problemsShown(); len(ps) != 1 || ps[0].path != "b.go" {
		t.Errorf("search: %+v, want b.go's one problem", ps)
	}
	a.problemsUI.query = ""
	a.problemsUI.collapsed = map[string]bool{"a.go": true}
	items := a.problemItems(a.problemsShown())
	if len(items) != 3 || !items[0].header || items[0].count != 2 || !items[1].header || items[2].header {
		t.Errorf("folded rows: %+v", items)
	}
}
