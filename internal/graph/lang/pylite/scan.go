package pylite

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// def is one function definition. class is the dotted path of the classes
// whose bodies hold it, innermost last, and empty for a module def: a def in
// a class body is a method of that class.
type def struct {
	name  string
	class string
	sig   string
	min   int
	max   string
	line  int
}

// key is the def's name inside its class path, as Store.get or get: the
// name a call's caller and an SQL literal's def carry, so a method shares no
// key with a module def of its name.
func (d def) key() string { return qualify(d.class, d.name) }

// qualify is name inside the dotted class path outer, or name alone when
// outer is empty.
func qualify(outer, name string) string {
	if outer == "" {
		return name
	}
	return outer + "." + name
}

// scope is one class whose body is open: its dotted path and the indent of
// its class line.
type scope struct {
	path   string
	indent int
}

// constDef is one module-level constant: NAME = literal.
type constDef struct {
	name  string
	value string
	line  int
}

// importRef is one imported name: from module import attr, or import
// module (attr ""). A relative module keeps its leading dots until parse
// resolves it against the file's package.
type importRef struct {
	module string
	attr   string
}

// call is one call by name inside a def; caller is that def's key.
type call struct {
	caller string
	name   string
	argc   int
	kw     map[string]string
	line   int
}

// fileScan is what scan reads from one file.
type fileScan struct {
	defs    []def
	consts  []constDef
	imports map[string]importRef
	calls   []call
}

// importedModules is the distinct modules imported, sorted.
func (f fileScan) importedModules() []string {
	seen := map[string]bool{}
	for _, im := range f.imports {
		seen[im.module] = true
	}
	out := make([]string, 0, len(seen))
	for m := range seen {
		out = append(out, m)
	}
	sort.Strings(out)
	return out
}

// patternSet is the line patterns, built on first use and never at init
// (RES-2).
type patternSet struct {
	once                              sync.Once
	def, class, cnst, call, from, imp *regexp.Regexp
}

var patterns patternSet

// pats is the built pattern set.
func pats() *patternSet {
	patterns.once.Do(func() {
		patterns.def = regexp.MustCompile(`^(\s*)(?:async\s+)?def\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
		patterns.class = regexp.MustCompile(`^(\s*)class\s+([A-Za-z_][A-Za-z0-9_]*)\s*[(:]`)
		patterns.cnst = regexp.MustCompile(`^([A-Z_][A-Z0-9_]*)\s*(?::[^=]+)?=\s*(.+?)\s*$`)
		patterns.call = regexp.MustCompile(`(?:^|[^A-Za-z0-9_.])([A-Za-z_][A-Za-z0-9_.]*)\s*\(`)
		patterns.from = regexp.MustCompile(`^from\s+(\.+(?:[A-Za-z_][A-Za-z0-9_.]*)?|[A-Za-z_][A-Za-z0-9_.]*)\s+import\s+(.+)$`)
		patterns.imp = regexp.MustCompile(`^import\s+([A-Za-z_][A-Za-z0-9_.]*)(?:\s+as\s+([A-Za-z_][A-Za-z0-9_]*))?\s*$`)
	})
	return &patterns
}

// scan reads src line by line. A def's body is every following line more
// indented than the def, or inside an open bracket; calls are read from
// bodies only. The body rule comes before the test for a def: a def nested
// in a body is part of that body, so it is no symbol, the calls in it and
// after it are the outer def's, and an import indented in it is not the
// module's. Of a nested def's own line only the parameters are read for
// calls (a default is evaluated when the outer body runs). A class's body is
// every following line more indented than its class line, its bases read
// past when they span lines; a def in it is a method of the innermost class
// open at its line, and a class in a def's body is part of that body.
func scan(src string) (fileScan, error) {
	f := fileScan{imports: map[string]importRef{}}
	lines := strings.Split(src, "\n")
	cur := ""
	curIndent := -1
	var classes []scope
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		if cur != "" && indent > curIndent {
			from := 0
			if m := pats().def.FindStringIndex(line); m != nil {
				from = m[1] - 1
			}
			i = readCalls(&f, lines, i, from, cur)
			continue
		}
		cur = ""
		for len(classes) > 0 && indent <= classes[len(classes)-1].indent {
			classes = classes[:len(classes)-1]
		}
		outer := ""
		if len(classes) > 0 {
			outer = classes[len(classes)-1].path
		}
		if m := pats().def.FindStringSubmatch(line); m != nil {
			d, end, err := readDef(lines, i, m[2], len(m[0])-1)
			if err != nil {
				return fileScan{}, err
			}
			d.class = outer
			f.defs = append(f.defs, d)
			cur, curIndent = d.key(), len(m[1])
			i = end
			continue
		}
		if m := pats().class.FindStringSubmatch(line); m != nil {
			classes = append(classes, scope{path: qualify(outer, m[2]), indent: len(m[1])})
			if strings.HasSuffix(m[0], "(") {
				if _, end, _, ok := group(lines, i, len(m[0])-1); ok {
					i = end
				}
			}
			continue
		}
		if m := pats().from.FindStringSubmatch(trim); m != nil {
			readFrom(f.imports, m[1], m[2])
		} else if m := pats().imp.FindStringSubmatch(trim); m != nil {
			name := m[1]
			if m[2] != "" {
				name = m[2]
			}
			f.imports[name] = importRef{module: m[1]}
		} else if m := pats().cnst.FindStringSubmatch(line); m != nil {
			f.consts = append(f.consts, constDef{name: m[1], value: m[2], line: i + 1})
		}
	}
	return f, nil
}

// readFrom records from module import a, b as c.
func readFrom(imports map[string]importRef, module, names string) {
	names = strings.Trim(strings.TrimSpace(names), "()")
	for _, part := range strings.Split(names, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		attr, alias, has := strings.Cut(part, " as ")
		attr = strings.TrimSpace(attr)
		if !has {
			alias = attr
		}
		imports[strings.TrimSpace(alias)] = importRef{module: module, attr: attr}
	}
}

// readDef reads a def starting at lines[i] with its parameter list opening
// at column open, possibly over several lines, and returns the def and the
// index of the line the list closes on. A list that does not close, or a
// missing colon after the close, is a parse refusal at the def's line.
func readDef(lines []string, i int, name string, open int) (def, int, error) {
	text, end, closeAt, ok := group(lines, i, open)
	if !ok {
		return def{}, 0, parseError{line: i + 1, why: "a def's parameters do not close"}
	}
	rest := lines[end][closeAt+1:]
	if !strings.Contains(rest, ":") {
		return def{}, 0, parseError{line: i + 1, why: "a def with no colon after its parameters"}
	}
	params := splitTop(text)
	d := def{name: name, line: i + 1, max: "0"}
	var shown []string
	total := 0
	for _, p := range params {
		p = strings.TrimSpace(p)
		switch {
		case p == "" || p == "/" || p == "*":
		case strings.HasPrefix(p, "*"):
			d.max = "*"
		default:
			shown = append(shown, p)
			if p == "self" || p == "cls" {
				continue
			}
			total++
			if !strings.Contains(p, "=") {
				d.min++
			}
		}
	}
	if d.max != "*" {
		d.max = strconv.Itoa(total)
	}
	d.sig = "(" + strings.Join(shown, ", ") + ")"
	return d, end, nil
}

// readCalls records every call on lines[i] from column from on (and the
// lines a call's arguments span) inside caller, and returns the last line
// index read.
func readCalls(f *fileScan, lines []string, i, from int, caller string) int {
	line := lines[i]
	end := i
	for _, m := range pats().call.FindAllStringSubmatchIndex(line[from:], -1) {
		name := line[from+m[2] : from+m[3]]
		if reserved(name) {
			continue
		}
		text, e, _, ok := group(lines, i, from+m[1]-1)
		if !ok {
			continue
		}
		if e > end {
			end = e
		}
		c := call{caller: caller, name: name, line: i + 1, kw: map[string]string{}}
		for _, a := range splitTop(text) {
			a = strings.TrimSpace(a)
			if a == "" {
				continue
			}
			c.argc++
			if k, v, has := strings.Cut(a, "="); has && !strings.ContainsAny(k, "=<>!") && literal(strings.TrimSpace(v)) {
				c.kw[strings.TrimSpace(k)] = strings.TrimSpace(v)
			}
		}
		f.calls = append(f.calls, c)
	}
	return end
}

// group is the text inside the bracket at lines[i][open], across lines,
// with the index of the closing line and the column of the close on it.
func group(lines []string, i, open int) (text string, end, closeAt int, ok bool) {
	depth := 0
	var b strings.Builder
	for j := i; j < len(lines); j++ {
		line := lines[j]
		start := 0
		if j == i {
			start = open
		}
		for k := start; k < len(line); k++ {
			switch line[k] {
			case '(', '[', '{':
				depth++
				if depth == 1 {
					continue
				}
			case ')', ']', '}':
				depth--
				if depth == 0 {
					return b.String(), j, k, true
				}
			}
			b.WriteByte(line[k])
		}
		b.WriteByte(' ')
	}
	return "", 0, 0, false
}

// splitTop splits text on commas at bracket depth zero.
func splitTop(text string) []string {
	var out []string
	depth := 0
	start := 0
	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, text[start:i])
				start = i + 1
			}
		}
	}
	return append(out, text[start:])
}

// literal reports whether v is a number or a quoted string.
func literal(v string) bool {
	if v == "" {
		return false
	}
	if (v[0] == '\'' || v[0] == '"') && len(v) > 1 && v[len(v)-1] == v[0] {
		return true
	}
	for _, c := range v {
		if (c < '0' || c > '9') && c != '.' && c != '_' {
			return false
		}
	}
	return true
}

// reserved is a keyword or statement word that precedes a parenthesis.
func reserved(name string) bool {
	switch name {
	case "if", "elif", "while", "for", "return", "and", "or", "not", "in", "is", "with", "assert", "yield", "await", "lambda", "except", "del", "print":
		return true
	}
	return false
}
