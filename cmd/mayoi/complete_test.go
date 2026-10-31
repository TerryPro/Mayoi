package main

import (
	"testing"

	"mayoi/internal/lsp"
)

func TestStripSnippet(t *testing.T) {
	cases := []struct{ in, want string }{
		{"plain", "plain"},
		{"foo(${1:bar, baz})", "foo(bar, baz)"},
		{"foo($0)", "foo()"},
		{"a\\$b", "a$b"},
		{"${1:x}$2", "x"},
	}
	for _, c := range cases {
		if got := stripSnippet(c.in); got != c.want {
			t.Errorf("stripSnippet(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCompletionStart(t *testing.T) {
	src := "x := fo"
	doc := newDocument(1, "a.go", src, nil)
	snap := doc.buf.Snapshot()
	if got := completionStart(snap, 7); got != 5 {
		t.Errorf("completionStart = %d, want 5", got)
	}
	if got := completionStart(snap, 0); got != 0 {
		t.Errorf("completionStart at 0 = %d, want 0", got)
	}
}

func TestCompletionShownFiltersByWord(t *testing.T) {
	doc := newDocument(1, "a.go", "fmt.Pri", nil)
	d := newEditor(doc)
	d.sel = [2]int{7, 7}
	a := &app{docs: []*document{doc}}
	a.complete = completionState{
		open: true, doc: 1, prefix: 4,
		items: []lsp.CompletionItem{
			{Label: "Println", InsertText: "Println"},
			{Label: "Printf", InsertText: "Printf"},
			{Label: "Sprintf", InsertText: "Sprintf"},
		},
	}
	items := a.completionShown(d)
	if len(items) != 2 || items[0].Label != "Println" || items[1].Label != "Printf" {
		t.Errorf("shown: %+v, want Println and Printf", items)
	}
}
