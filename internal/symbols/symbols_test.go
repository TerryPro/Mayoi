package symbols

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestDeclare(t *testing.T) {
	for _, tc := range []struct {
		line, name, kind, detail string
	}{
		{"func main() {", "main", "function", "()"},
		{"func (a *app) editorView(c *ui.Context, d *document) {", "editorView", "method", "(c *ui.Context, d *document) · app"},
		{"type palette struct {", "palette", "class", "struct"},
		{"export async function load(url: string): Promise<void> {", "load", "function", "(url: string): Promise<void>"},
		{"    def run(self):", "run", "method", "(self)"},
		{"class Foo(Base):", "Foo", "class", "(Base)"},
		{"pub(crate) fn parse<'a>(s: &'a str) -> Result<()> {", "parse", "function", "<'a>(s: &'a str) -> Result<()>"},
		{"interface Props {", "Props", "interface", ""},
		{"var count int", "count", "variable", "int"},
		{"const MAX = 3", "MAX", "variable", "= 3"},
		{"pub const LIMIT: u32 = 10;", "LIMIT", "variable", ": u32 = 10"},
		{"namespace App {", "App", "namespace", ""},
		{"pub mod parser;", "parser", "namespace", ""},
		{"fun run() {", "run", "function", "()"},
	} {
		sym, ok := declare(tc.line)
		if !ok || sym.Name != tc.name || sym.Kind != tc.kind || sym.Detail != tc.detail {
			t.Errorf("%q: %+v %v", tc.line, sym, ok)
		}
	}
	for _, line := range []string{"x := func() {}", "	func() {", "type (", "// func comment()", "	return function() {"} {
		if sym, ok := declare(line); ok {
			t.Errorf("%q declares %+v", line, sym)
		}
	}
}

// Find lists a class's variable and a namespace, and leaves out the
// variables of a function, which are locals.
func TestFindLeavesOutLocals(t *testing.T) {
	src := `namespace App {
  const MAX = 3
  function load() {
    const cache = []
    var request = fetch()
  }
}
`
	syms := Find(ui.NewTextBuffer(src).Snapshot(), false)
	want := []struct{ name, kind string }{
		{"App", "namespace"},
		{"MAX", "variable"},
		{"load", "function"},
	}
	if len(syms) != len(want) {
		t.Fatalf("symbols %+v", syms)
	}
	for i, w := range want {
		if syms[i].Name != w.name || syms[i].Kind != w.kind {
			t.Errorf("symbol %d is %+v, want %s %s", i, syms[i], w.kind, w.name)
		}
	}
	for i, s := range syms {
		if s.Line != []int{0, 1, 2}[i] {
			t.Errorf("%s is on line %d", s.Name, s.Line)
		}
	}
}

// BenchmarkFind measures the scan of a document of 30,000 lines, which
// the outline reads in the background.
func BenchmarkFind(b *testing.B) {
	src := strings.Repeat("func alpha() {\n\treturn\n}\n", 10_000)
	snap := ui.NewTextBuffer(src).Snapshot()
	b.ResetTimer()
	for range b.N {
		Find(snap, false)
	}
}
