// Package symbols finds the declarations of a text, in most languages,
// which go to symbol lists.
package symbols

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
)

// Symbol is a declaration of a document, as go to symbol lists it: the
// line it is on, its name, which quick open matches, and its kind.
type Symbol struct {
	Line   int
	Name   string
	Detail string // the rest of the declaration, shortened
	Kind   string
	Depth  int // how far it is indented, in columns
	Col    int // where the name starts on its line, in runes
}

// declaration finds the lines that declare something, in most languages:
// a keyword, after modifiers, then a name.
var declaration = regexp.MustCompile(`^(\s*)(?:(?:export|pub(?:\([^)]*\))?|public|private|protected|internal|static|async|abstract|final|default|override|open|sealed|data|inline|unsafe|extern|declare|mut|readonly)\s+)*(func|fun|def|class|type|fn|function|interface|struct|enum|trait|impl|module|object|protocol|extension|union|macro|namespace|mod|var|let|const|val)\b\s*(.*)$`)

// mdHeading finds a heading of Markdown.
var mdHeading = regexp.MustCompile(`^(#{1,6})\s+(.+?)\s*#*\s*$`)

// maxLines is the longest document whose symbols are listed.
const maxLines = 200_000

// Find lists the declarations of a text, or, in Markdown, its headings.
func Find(snap ui.TextSnapshot, markdown bool) []Symbol {
	if snap.LineCount() > maxLines {
		return nil
	}
	var out []Symbol
	// outer are the declarations above the line read, to tell a local
	// variable from one of a class or a namespace.
	var outer []Symbol
	fence := false
	// The text is read whole, which is a copy, rather than line by line,
	// which walks the buffer for each: a document of many lines is found
	// faster so.
	text := snap.String()
	lines := strings.Split(text, "\n")
	for i, text := range lines {
		if len(text) > 400 {
			continue
		}
		if markdown {
			if strings.HasPrefix(text, "```") {
				fence = !fence
			}
			if m := mdHeading.FindStringSubmatch(text); m != nil && !fence {
				out = append(out, Symbol{Line: i, Name: m[2], Detail: m[1], Kind: "heading", Depth: 2 * (len(m[1]) - 1)})
			}
			continue
		}
		if !startsDeclaration(text) {
			continue
		}
		sym, ok := declare(text)
		if !ok {
			continue
		}
		for len(outer) > 0 && outer[len(outer)-1].Depth >= sym.Depth {
			outer = outer[:len(outer)-1]
		}
		if sym.Kind == "variable" && len(outer) > 0 {
			last := outer[len(outer)-1].Kind
			if last == "function" || last == "method" {
				// A variable of a function is a local, which lists of the
				// document's declarations leave out.
				continue
			}
		}
		sym.Line = i
		if at := strings.Index(text, sym.Name); at >= 0 {
			sym.Col = utf8.RuneCountInString(text[:at])
		}
		outer = append(outer, sym)
		out = append(out, sym)
	}
	return out
}

// keywordStarts are the words a declaration or its modifiers start with,
// which a line may declare something by.
var keywordStarts = map[string]bool{
	"func": true, "fun": true, "def": true, "class": true, "type": true,
	"fn": true, "function": true, "interface": true, "struct": true,
	"enum": true, "trait": true, "impl": true, "module": true,
	"object": true, "protocol": true, "extension": true, "union": true,
	"macro": true, "namespace": true, "mod": true, "var": true, "let": true,
	"const": true, "val": true,
	"export": true, "pub": true, "public": true, "private": true,
	"protected": true, "internal": true, "static": true, "async": true,
	"abstract": true, "final": true, "default": true, "override": true,
	"open": true, "sealed": true, "data": true, "inline": true,
	"unsafe": true, "extern": true, "declare": true, "mut": true,
	"readonly": true,
}

// startsDeclaration reports whether a line begins with a word that may
// declare something, which most lines of a document do not, so that the
// declaration's pattern is not tried on each of them.
func startsDeclaration(text string) bool {
	i := 0
	for i < len(text) && (text[i] == ' ' || text[i] == '\t') {
		i++
	}
	start := i
	for i < len(text) && isWordByte(text[i]) {
		i++
	}
	return keywordStarts[text[start:i]]
}

// isWordByte reports whether b may be in a keyword: letters, digits and
// underscores.
func isWordByte(b byte) bool {
	return b == '_' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9'
}

// declare reads a line declaring something: its name and kind.
func declare(text string) (Symbol, bool) {
	m := declaration.FindStringSubmatch(text)
	if m == nil {
		return Symbol{}, false
	}
	kw, rest := m[2], m[3]
	// A method's receiver comes first in Go: func (a *app) name(…).
	receiver := ""
	if kw == "func" && strings.HasPrefix(rest, "(") {
		if end := strings.IndexByte(rest, ')'); end > 0 {
			if f := strings.Fields(rest[1:end]); len(f) > 0 {
				receiver = strings.TrimLeft(f[len(f)-1], "*")
			}
			rest = strings.TrimSpace(rest[end+1:])
		}
	}
	name := rest
	if end := strings.IndexFunc(rest, func(r rune) bool { return !isWordRune(r) && r != '$' && r != '.' && r != '!' }); end >= 0 {
		name = rest[:end]
	}
	if name == "" || kw == "type" && rest != "" && rest[0] == '(' {
		return Symbol{}, false // an anonymous function, or a group of types
	}
	after := strings.TrimSpace(rest[len(name):])
	if kw == "const" && after != "" && !strings.HasPrefix(after, "=") && !strings.HasPrefix(after, ":") &&
		after[0] != ';' && after[0] != ',' {
		// A constant needs its value next, or its type: C's const char *
		// names a type, not a declaration.
		return Symbol{}, false
	}
	// The rest of the declaration, as a function's parameters, and the
	// type a method is of.
	detail := strings.TrimSpace(strings.TrimRight(after, "{:;"))
	if len(detail) > 100 {
		detail = detail[:97] + "…"
	}
	if receiver != "" {
		detail = strings.TrimSpace(detail + " · " + receiver)
	}
	kind := kw
	switch kw {
	case "def", "fn", "fun", "function", "func":
		kind = "function"
		if strings.Contains(m[0], "func (") || len(m[1]) > 0 && kw == "def" {
			kind = "method"
		}
	case "struct", "union", "object", "class", "impl", "extension":
		kind = "class"
	case "interface", "trait", "protocol":
		kind = "interface"
	case "namespace", "module", "mod":
		kind = "namespace"
	case "var", "let", "const", "val":
		kind = "variable"
	case "type":
		// Go's types say what they are after their name.
		switch f := strings.Fields(detail); {
		case len(f) > 0 && f[0] == "struct":
			kind = "class"
		case len(f) > 0 && f[0] == "interface":
			kind = "interface"
		}
	}
	return Symbol{Name: name, Detail: detail, Kind: kind, Depth: columns(m[1])}, true
}

func isWordRune(r rune) bool { return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) }

// tabWidth is the columns of a tab, as the editor shows it.
const tabWidth = 4

// columns is how wide s is, in columns, with its tabs.
func columns(s string) int {
	col := 0
	for _, r := range s {
		if r == '\t' {
			col += tabWidth - col%tabWidth
		} else {
			col++
		}
	}
	return col
}
