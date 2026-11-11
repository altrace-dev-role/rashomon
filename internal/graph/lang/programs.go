package lang

// programKinds are the extensions a request names a script or a source
// file by (W-M1.46), in the registration's order. They are file kinds a
// request can name, not languages the graph reads: a kind has an extractor
// only where a Resolver claims its files. The table is unexported and
// static, so a caller can ask whether an extension is one and has no way
// to name one, and reading it allocates nothing.
var programKinds = [...]string{".sh", ".bash", ".zsh", ".fish", ".ps1", ".psm1", ".bat", ".cmd", ".py", ".rb", ".pl",
	".exs", ".js", ".mjs", ".cjs", ".ts", ".lua"}

// ProgramKind reports whether ext, with its dot, is a program kind. It
// says what a word is when the caller holds no file of its kind; a caller
// that holds files of the kind reads such a word as it reads any path.
func ProgramKind(ext string) bool {
	for _, k := range programKinds {
		if k == ext {
			return true
		}
	}
	return false
}
