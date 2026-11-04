package pylite

import (
	"regexp"
	"sort"
	"strings"
	"sync"
)

// sqlRef is one SQL statement found in a string literal: the table it
// names, whether it reads or writes it, the columns it mentions, and the
// line and enclosing def.
type sqlRef struct {
	table  string
	write  bool
	cols   []string
	line   int
	caller string
}

type sqlPatterns struct {
	once                 sync.Once
	lit, stmt, ident, kw *regexp.Regexp
}

var sqlPats sqlPatterns

func sqlp() *sqlPatterns {
	sqlPats.once.Do(func() {
		sqlPats.lit = regexp.MustCompile(`"((?:[^"\\]|\\.)*)"|'((?:[^'\\]|\\.)*)'`)
		sqlPats.stmt = regexp.MustCompile(`(?i)\b(SELECT|INSERT\s+INTO|UPDATE|DELETE\s+FROM)\b`)
		sqlPats.ident = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)?`)
		sqlPats.kw = regexp.MustCompile(`(?i)^(select|from|where|and|or|not|insert|into|values|update|set|delete|join|on|as|count|now|interval|seconds|order|by|group|limit|null|is|in|like|desc|asc|true|false|s|d)$`)
	})
	return &sqlPats
}

// scanSQL finds the SQL statements in src's string literals, each
// attributed to the last def that starts above it.
func scanSQL(src string, defs []def) []sqlRef {
	p := sqlp()
	var out []sqlRef
	for i, line := range strings.Split(src, "\n") {
		for _, m := range p.lit.FindAllStringSubmatch(line, -1) {
			lit := m[1]
			if lit == "" {
				lit = m[2]
			}
			ref, ok := parseSQL(p, lit)
			if !ok {
				continue
			}
			ref.line = i + 1
			ref.caller = enclosing(defs, ref.line)
			out = append(out, ref)
		}
	}
	return out
}

// parseSQL reads one literal: the verb decides read or write, the word
// after FROM, INTO or UPDATE is the table, the other identifiers are
// columns.
func parseSQL(p *sqlPatterns, lit string) (sqlRef, bool) {
	m := p.stmt.FindStringSubmatchIndex(lit)
	if m == nil {
		return sqlRef{}, false
	}
	verb := strings.ToUpper(strings.Join(strings.Fields(lit[m[2]:m[3]]), " "))
	ref := sqlRef{write: verb != "SELECT"}
	words := p.ident.FindAllString(lit, -1)
	for i, w := range words {
		up := strings.ToUpper(w)
		if (up == "FROM" || up == "INTO" || up == "UPDATE" || up == "JOIN") && i+1 < len(words) && ref.table == "" {
			ref.table = words[i+1]
		}
	}
	if ref.table == "" {
		return sqlRef{}, false
	}
	seen := map[string]bool{}
	for _, w := range words {
		if w == ref.table || p.kw.MatchString(w) || seen[w] || !strings.ContainsAny(w[:1], "abcdefghijklmnopqrstuvwxyz") {
			continue
		}
		seen[w] = true
		ref.cols = append(ref.cols, w)
	}
	sort.Strings(ref.cols)
	return ref, true
}

// enclosing is the name of the last def starting at or above line, or ""
// for module level.
func enclosing(defs []def, line int) string {
	name := ""
	for _, d := range defs {
		if d.line <= line {
			name = d.name
		}
	}
	return name
}
