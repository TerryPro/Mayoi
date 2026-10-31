package main

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"mayoi/internal/symbols"
)

// outlineTestSymbols is a namespace with a variable and a class, whose
// method has a local variable, and a function of the document.
var outlineTestSymbols = []symbols.Symbol{
	{Line: 0, Name: "App", Kind: "namespace", Depth: 0},
	{Line: 1, Name: "MAX", Kind: "variable", Depth: 2},
	{Line: 2, Name: "Widget", Kind: "class", Depth: 2},
	{Line: 3, Name: "draw", Kind: "method", Depth: 4},
	{Line: 4, Name: "cache", Kind: "variable", Depth: 6},
	{Line: 6, Name: "main", Kind: "function", Depth: 0},
}

// The outline puts a declaration under the one it is indented in, drops
// the variables of a function, and puts those of a kind hidden in its
// place.
func TestOutlineTree(t *testing.T) {
	roots := outlineNodes(outlineTestSymbols)
	if len(roots) != 2 || roots[0].sym.Name != "App" || roots[1].sym.Name != "main" {
		t.Fatalf("roots %+v", roots)
	}
	if len(roots[0].children) != 2 {
		t.Fatalf("App holds %d declarations", len(roots[0].children))
	}
	widget := roots[0].children[1]
	if widget.sym.Name != "Widget" || len(widget.children) != 1 || widget.children[0].sym.Name != "draw" {
		t.Fatalf("Widget holds %+v", widget.children)
	}
	if len(widget.children[0].children) != 1 {
		t.Fatalf("draw holds %d declarations", len(widget.children[0].children))
	}
	roots = pruneOutlineVariables(roots, false)
	if n := len(roots[0].children[1].children[0].children); n != 0 {
		t.Errorf("draw still holds %d locals", n)
	}
	filtered := filterOutlineNodes(outlineNodes(outlineTestSymbols), func(kind string) bool {
		return kind == "method"
	})
	if len(filtered) != 1 || filtered[0].sym.Name != "draw" {
		t.Errorf("a hidden class did not put its method in its place: %+v", filtered)
	}
}

// An outline ordered by name or by kind sorts each run of declarations.
func TestOutlineSort(t *testing.T) {
	for _, order := range []string{"name", "kind"} {
		roots := outlineNodes(outlineTestSymbols)
		sortOutlineNodes(roots, order)
		if roots[0].sym.Name != "App" {
			t.Errorf("%s: the namespace is not first: %+v", order, roots)
		}
		if order == "kind" {
			if roots[1].sym.Name != "main" {
				t.Errorf("kind: the function is not after the namespace: %+v", roots)
			}
		} else if roots[1].sym.Name != "main" {
			t.Errorf("name: main is not after App: %+v", roots)
		}
	}
}

// A folded declaration hides the rows of those it holds.
func TestOutlineFoldHidesRows(t *testing.T) {
	roots := outlineNodes(outlineTestSymbols)
	outlineAssignKeys(roots, "")
	rows := outlineTestRows(roots, nil)
	if len(rows) != 6 {
		t.Fatalf("%d rows open", len(rows))
	}
	widget := roots[0].children[1]
	if rows := outlineTestRows(roots, map[string]bool{"path\x00" + widget.key: true}); len(rows) != 4 {
		t.Errorf("%d rows with Widget folded", len(rows))
	}
}

// outlineTestRows flattens a tree as the outline shows it.
func outlineTestRows(nodes []*outlineNode, collapsed map[string]bool) []outlineRow {
	var rows []outlineRow
	outlineFlatten(nodes, "path", collapsed, 0, &rows)
	return rows
}

// The Explorer shows the outline of the file shown, folds its rows, and
// a click on a declaration goes to it.
func TestOutlineShowsFoldsAndJumps(t *testing.T) {
	hs := newHarness(t, true)
	a := hs.a
	path := hs.path("outline.py")
	src := "class Foo:\n    def bar(self):\n        pass\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	a.openFile(path, true)
	hs.settle()
	rows := a.outlineRows()
	if len(rows) != 2 || rows[0].node.sym.Name != "Foo" || rows[1].node.sym.Name != "bar" {
		t.Fatalf("rows %+v", rows)
	}
	if !hs.tt.HasText("OUTLINE") {
		t.Errorf("the outline's header does not show: %q", hs.tt.Texts())
	}
	foo := rows[0].node
	a.settings.OutlineKinds = []string{"method"}
	if rows := a.outlineRows(); len(rows) != 1 || rows[0].node.sym.Name != "bar" {
		t.Errorf("the kinds shown: %+v", rows)
	}
	a.settings.OutlineKinds = nil
	a.foldOutlineNode(foo, true)
	hs.settle()
	if rows := a.outlineRows(); len(rows) != 1 {
		t.Errorf("%d rows with Foo folded", len(rows))
	}
	a.foldOutlineNode(foo, false)
	hs.settle()
	rows = a.outlineRows()
	a.openOutlineNode(rows[1].node)
	hs.settle()
	d := a.activeEditor()
	if d == nil {
		t.Fatal("no editor")
	}
	s, _ := d.buf.Snapshot().LineRange(1)
	if line := d.buf.Snapshot().LineAt(d.caret); line != 1 || d.sel[0] < s {
		t.Errorf("the caret is on line %d at %d, want line 1 from %d", line, d.sel[0], s)
	}
}

// A document of many lines is read in the background, and its outline
// shows once it is ready.
func TestOutlineReadsLongFiles(t *testing.T) {
	hs := newHarness(t, true)
	a := hs.a
	var src strings.Builder
	for i := range 5000 {
		fmt.Fprintf(&src, "def run%d():\n    pass\n", i)
	}
	path := hs.path("long.py")
	if err := os.WriteFile(path, []byte(src.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	a.openFile(path, true)
	hs.settle()
	if rows := a.outlineRows(); len(rows) != 5000 {
		t.Errorf("%d rows of a long file", len(rows))
	}
}
