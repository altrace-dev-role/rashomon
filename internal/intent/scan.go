package intent

import "sync"

// The tokenizer is a byte scanner over closed character classes, so the
// package imports no regexp (RES-2). A token is a run of path bytes (letters,
// digits and "._@+-/", the bytes a printable path segment may hold) or of
// bytes at or above 0x80; every other byte ends one. A token holding a byte
// at or above 0x80 is no path, identifier or class word, so a non-ASCII word
// never yields an ASCII fragment.

// scan calls fn with each token of b, trailing dots dropped (a sentence's
// full stop). With cut, b ends mid-request, so a token that reaches its end
// may be a fragment and is not passed.
func scan(b []byte, cut bool, fn func(tok []byte)) {
	for i := 0; i < len(b); {
		if !tokenByte(b[i]) {
			i++
			continue
		}
		j := i
		for j < len(b) && tokenByte(b[j]) {
			j++
		}
		tok := b[i:j]
		for len(tok) > 0 && tok[len(tok)-1] == '.' {
			tok = tok[:len(tok)-1]
		}
		if len(tok) > 0 && !(cut && j == len(b)) && ascii(tok) {
			fn(tok)
		}
		i = j
	}
}

func tokenByte(c byte) bool {
	return c >= 0x80 || letter(c) || digit(c) || c == '.' || c == '_' || c == '@' || c == '+' || c == '-' || c == '/'
}

func letter(c byte) bool { return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' }
func digit(c byte) bool  { return '0' <= c && c <= '9' }
func lower(c byte) bool  { return 'a' <= c && c <= 'z' }
func upper(c byte) bool  { return 'A' <= c && c <= 'Z' }

func ascii(tok []byte) bool {
	for _, c := range tok {
		if c >= 0x80 {
			return false
		}
	}
	return true
}

// extensions is the closed list of file extensions that make a token with
// no slash a path.
var extensions = []string{
	"c", "cc", "cfg", "conf", "cpp", "cs", "css", "go", "h", "hpp", "html", "ini", "java", "js", "json", "jsx",
	"kt", "lock", "md", "mjs", "mod", "php", "proto", "py", "rb", "rs", "scss", "sh", "sql", "sum", "swift",
	"toml", "ts", "tsx", "txt", "xml", "yaml", "yml",
}

// pathLike reports a token that names a path: it holds a slash, or ends in
// a listed extension after a name.
func pathLike(tok []byte) bool {
	dot := -1
	for i, c := range tok {
		switch c {
		case '/':
			return true
		case '.':
			dot = i
		}
	}
	if dot <= 0 {
		return false
	}
	ext := tok[dot+1:]
	for _, e := range extensions {
		if string(ext) == e {
			return true
		}
	}
	return false
}

// identifierLike reports a code identifier: 3 to 64 bytes of letters,
// digits, '_' and '.', holding a case step (readFile, EvaluateStop) or an
// underscore inside a word (prompt_class).
func identifierLike(tok []byte) bool {
	if len(tok) < 3 || len(tok) > 64 {
		return false
	}
	shaped := false
	for i, c := range tok {
		switch {
		case letter(c) || digit(c) || c == '.':
		case c == '_':
			shaped = shaped || i > 0 && i < len(tok)-1 && (letter(tok[i-1]) || digit(tok[i-1])) && (letter(tok[i+1]) || digit(tok[i+1]))
			continue
		default:
			return false
		}
		if i > 0 && lower(tok[i-1]) && upper(c) {
			shaped = true
		}
	}
	return shaped
}

// classWords is the closed verb table: each word, matched whole and
// case-insensitively, names its class.
func classWords() map[string]Verb {
	classWordsOnce.Do(func() { classWordsOnce.v = classTable() })
	return classWordsOnce.v
}

// classWordsOnce holds what classWords returns, built on its first call and
// never at init (RES-2).
var classWordsOnce struct {
	sync.Once
	v map[string]Verb
}

func classTable() map[string]Verb {
	return map[string]Verb{
		"add": VerbAdd, "adds": VerbAdd, "adding": VerbAdd, "create": VerbAdd, "implement": VerbAdd, "introduce": VerbAdd,
		"fix": VerbFix, "fixes": VerbFix, "fixing": VerbFix, "repair": VerbFix, "debug": VerbFix, "patch": VerbFix,
		"refactor": VerbRefactor, "refactoring": VerbRefactor, "rename": VerbRefactor, "restructure": VerbRefactor,
		"simplify": VerbRefactor, "extract": VerbRefactor,
		"test": VerbTest, "tests": VerbTest, "testing": VerbTest,
		"migrate": VerbMigrate, "migration": VerbMigrate, "upgrade": VerbMigrate, "port": VerbMigrate,
		"deploy": VerbDeploy, "deploying": VerbDeploy, "release": VerbDeploy, "ship": VerbDeploy, "publish": VerbDeploy,
		"document": VerbDocument, "documentation": VerbDocument, "docs": VerbDocument, "readme": VerbDocument,
	}
}

// verbOf is a class word's class, or "" for any other token.
func verbOf(tok []byte) Verb {
	if len(tok) > len("documentation") {
		return ""
	}
	var buf [len("documentation")]byte
	for i, c := range tok {
		if !letter(c) {
			return ""
		}
		buf[i] = c | 0x20
	}
	return classWords()[string(buf[:len(tok)])]
}
