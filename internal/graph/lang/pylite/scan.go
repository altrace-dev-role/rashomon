package pylite

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// def is one function definition.
type def struct {
	name string
	sig  string
	min  int
	max  string
	line int
}

// constDef is one module-level constant: NAME = literal.
type constDef struct {
	name  string
	value string
	line  int
}

// importRef is one imported name: from module import attr, or import
// module (attr "").
type importRef struct {
	module string
	attr   string
}

// call is one call by name inside a def.
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
	once                       sync.Once
	def, cnst, call, from, imp *regexp.Regexp
}

var patterns patternSet

// pats is the built pattern set.
func pats() *patternSet {
	patterns.once.Do(func() {
		patterns.def = regexp.MustCompile(`^(\s*)(?:async\s+)?def\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
		patterns.cnst = regexp.MustCompile(`^([A-Z_][A-Z0-9_]*)\s*(?::[^=]+)?=\s*(.+?)\s*$`)
		patterns.call = regexp.MustCompile(`(?:^|[^A-Za-z0-9_.])([A-Za-z_][A-Za-z0-9_.]*)\s*\(`)
		patterns.from = regexp.MustCompile(`^from\s+([A-Za-z_][A-Za-z0-9_.]*)\s+import\s+(.+)$`)
		patterns.imp = regexp.MustCompile(`^import\s+([A-Za-z_][A-Za-z0-9_.]*)(?:\s+as\s+([A-Za-z_][A-Za-z0-9_]*))?\s*$`)
	})
	return &patterns
}

// scan reads src line by line. A def's body is every following line more
// indented than the def, or inside an open bracket; calls are read from
// bodies only.
func scan(src string) (fileScan, error) {
	f := fileScan{imports: map[string]importRef{}}
	lines := strings.Split(src, "\n")
	cur := ""
	curIndent := -1
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		if m := pats().def.FindStringSubmatch(line); m != nil {
			d, end, err := readDef(lines, i, m[2], len(m[0])-1)
			if err != nil {
				return fileScan{}, err
			}
			f.defs = append(f.defs, d)
			cur, curIndent = d.name, len(m[1])
			i = end
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		if cur != "" && indent <= curIndent {
			cur = ""
		}
		if cur != "" {
			end := readCalls(&f, lines, i, cur)
			i = end
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
// index of the line the list closes on. A missing colon after the close is
// ErrParse.
func readDef(lines []string, i int, name string, open int) (def, int, error) {
	text, end, closeAt, ok := group(lines, i, open)
	if !ok {
		return def{}, 0, ErrParse
	}
	rest := lines[end][closeAt+1:]
	if !strings.Contains(rest, ":") {
		return def{}, 0, ErrParse
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

// readCalls records every call on lines[i] (and the lines a call's
// arguments span) inside caller, and returns the last line index read.
func readCalls(f *fileScan, lines []string, i int, caller string) int {
	line := lines[i]
	end := i
	for _, m := range pats().call.FindAllStringSubmatchIndex(line, -1) {
		name := line[m[2]:m[3]]
		if reserved(name) {
			continue
		}
		text, e, _, ok := group(lines, i, m[1]-1)
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
