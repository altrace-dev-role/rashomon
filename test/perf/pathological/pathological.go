// Package pathological is the set of Bash command lines that have driven the
// shape of a call past its memory, shared by the tests that bound them at two
// levels: internal/shape's, which bound each reading of a line by allocation,
// and cmd/rashomon's, which run the hook entry on each one and bound its peak
// RSS (RES-29). One table, so a shape added at one level is measured at both.
//
// It imports nothing: internal/shape's own tests import it, and anything that
// imported shape would be a cycle.
package pathological

// Line is a command line built as Prefix, then Unit repeated as many times as
// the caller's size allows, then Suffix. A unit holding a backslash, a newline
// or a quote costs more encoded than its length, so a caller sizing a payload
// measures a unit's encoded cost rather than counting its bytes.
type Line struct {
	Name, Prefix, Unit, Suffix string
}

// Lines is every shape, each one unit repeated behind echo where the unit is
// not a command of its own.
func Lines() []Line {
	return []Line{
		{"short words", "echo", " a", ""},
		{"backslash-newlines", "echo ", "a\\\n", ""},
		{"one word of $a", "echo ", "$a", ""},
		{"escaped blanks", "echo ", "\\ ", ""},
		{"quoted blanks", "echo ", "'' ", ""},
		{"double-quoted blanks", "echo ", `"" `, ""},
		{"one double-quoted word", "echo \"", "a", "\""},
		{"redirections", "", "<", ""},
		{"cd a&&", "", "cd a&&", ""},
		{"a here-document", "python3 - <<'E'\n", "w1 w2 w3 w4 w5 w6 w7 w8 w9 w0\n", "E\n"},
		{"substitutions", "echo ", "$(a) ", ""},
		{"assignments", "", "X_TOKEN=abc ", ""},
		{"empty lines", "echo", "\n", ""},
		{"short lines", "echo", "\na", ""},
		{"ssh lines", "", "ssh a\n", ""},
		{"urls", "curl", " https://a.b/", ""},
		{"git@ scheme runs", "", "git@", ""},
		{"unterminated bracket hosts", "", "https://[", ""},
	}
}
