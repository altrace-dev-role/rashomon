package episode

import (
	"path"
	"strings"
	"unicode"
)

// wrappers are words that run the command after them: the program is the
// word they wrap. Each is a POSIX or near-universal shell utility, not a
// language's tool.
var wrappers = map[string]bool{
	"env": true, "sudo": true, "time": true, "nice": true, "nohup": true,
	"command": true, "exec": true, "timeout": true, "stdbuf": true, "xargs": true,
}

// setup words open a shell segment that prepares the next one rather than
// doing the work.
var setup = map[string]bool{
	"cd": true, "pushd": true, "popd": true, "export": true, "source": true, ".": true,
	"set": true, "true": true, "unset": true, "ulimit": true, "umask": true,
}

// commandSig is a command's program and verb: the program is the base name of
// the first word of the first segment that does the work (after setup
// segments, variable assignments and wrappers); the verb is up to two
// word-shaped arguments after it, flags passed over, stopping at the first
// argument that is not word-shaped (a path, a number, a pattern).
//
//	"cd web && npx jest src/a.test.ts"  -> "npx", "jest"
//	"python -m pytest -q tests/"         -> "python", "pytest"
//	"terraform plan -out=x"              -> "terraform", "plan"
//	"npm run build"                      -> "npm", "run build"
func commandSig(cmd string) (program, verb string) {
	for _, seg := range segments(cmd) {
		words := unwrap(fields(seg))
		if len(words) == 0 || setup[words[0]] {
			continue
		}
		program = path.Base(words[0])
		var verbs []string
		for _, w := range words[1:] {
			if strings.HasPrefix(w, "-") {
				continue
			}
			if !wordShaped(w) || len(verbs) == 2 {
				break
			}
			verbs = append(verbs, w)
		}
		return program, strings.Join(verbs, " ")
	}
	return "", ""
}

// segments splits a command line on the shell's list and pipe operators,
// outside quotes.
func segments(cmd string) []string {
	var out []string
	var b strings.Builder
	var quote rune
	rs := []rune(cmd)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
			b.WriteRune(r)
		case r == '\'' || r == '"':
			quote = r
			b.WriteRune(r)
		case r == ';' || r == '|' || r == '&' || r == '\n':
			if s := strings.TrimSpace(b.String()); s != "" {
				out = append(out, s)
			}
			b.Reset()
		default:
			b.WriteRune(r)
		}
	}
	if s := strings.TrimSpace(b.String()); s != "" {
		out = append(out, s)
	}
	return out
}

// fields splits a segment into words, keeping quoted text whole without its
// quotes.
func fields(seg string) []string {
	var out []string
	var b strings.Builder
	var quote rune
	in := false
	for _, r := range seg {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				b.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote, in = r, true
		case unicode.IsSpace(r):
			if in {
				out = append(out, b.String())
				b.Reset()
				in = false
			}
		default:
			b.WriteRune(r)
			in = true
		}
	}
	if in {
		out = append(out, b.String())
	}
	return out
}

// unwrap passes over the variable assignments and wrappers in front of a
// segment's program, in any order (sudo env X=1 tool).
func unwrap(words []string) []string {
	for len(words) > 0 {
		switch {
		case isAssignment(words[0]):
			words = words[1:]
		case wrappers[path.Base(words[0])]:
			words = skipWrapperArgs(words[1:])
		default:
			return words
		}
	}
	return words
}

func isAssignment(w string) bool {
	i := strings.IndexByte(w, '=')
	return i > 0 && !strings.ContainsAny(w[:i], "/-.")
}

// skipWrapperArgs passes over a wrapper's own flags and, for one that takes a
// number first (timeout 30, nice -n 5), that number.
func skipWrapperArgs(words []string) []string {
	for len(words) > 0 && (strings.HasPrefix(words[0], "-") || isNumber(words[0])) {
		words = words[1:]
	}
	return words
}

func isNumber(w string) bool {
	return w != "" && strings.Trim(w, "0123456789.smhd") == ""
}

// wordShaped is a word that names something: letters first, then letters,
// digits, '-', '_' or ':'.
func wordShaped(w string) bool {
	for i, r := range w {
		switch {
		case unicode.IsLetter(r):
		case i > 0 && (unicode.IsDigit(r) || r == '-' || r == '_' || r == ':'):
		default:
			return false
		}
	}
	return w != ""
}

// pathWords are the words of a command that look like paths: they hold a
// separator or end in a file extension.
func pathWords(cmd string) []string {
	var out []string
	for _, seg := range segments(cmd) {
		for _, w := range fields(seg) {
			if strings.HasPrefix(w, "-") {
				if i := strings.IndexByte(w, '='); i > 0 {
					w = w[i+1:]
				} else {
					continue
				}
			}
			if strings.Contains(w, "/") || hasExt(w) {
				out = append(out, w)
			}
		}
	}
	return out
}

func hasExt(w string) bool {
	ext := path.Ext(w)
	return len(ext) > 1 && len(ext) <= 6 && wordShaped(ext[1:]) && !isNumber(strings.TrimSuffix(w, ext))
}
