package lang

import "path"

// nonSourceKinds is the kinds table (LANG-REG1): the file kinds known to
// hold no program a language reader parses, so an edit of one that no
// resolver claims says nothing. The rule a row follows: its files are
// prose (.md .rst .txt), data or configuration (.json .yaml .yml .toml .ini
// .cfg .csv), a build tool's manifest, lock or checksum (.mod .work .sum
// .lock), an image (.png .jpg .gif .svg), or the repository's own metadata
// (.gitignore LICENSE CODEOWNERS). A kind whose files are a program in some
// language (.sh, .ts, .sql, Makefile, Dockerfile) is never a row: a reader
// of it may exist, and where none is here an edit of one says so, so an
// unknown kind is disclosed and never silent. A program whose name ends in
// a row's extension is keyed by its name (namedPrograms), so it is no row
// either. A row is keyed as KindKey keys a path, case and all. The table is
// unexported and static, like programKinds, so reading it allocates nothing
// and costs no init.
var nonSourceKinds = [...]string{".md", ".rst", ".txt", ".json", ".yaml", ".yml", ".toml", ".ini", ".cfg", ".csv",
	".mod", ".work", ".sum", ".lock", ".png", ".jpg", ".gif", ".svg", ".gitignore", "LICENSE", "CODEOWNERS"}

// namedPrograms are the base names whose files are a program though their
// extension is a row's: CMakeLists.txt is CMake's build program. KindKey
// keys one by its name, which is no row, so an edit of it is disclosed
// under that name like any program kind, never passed over as .txt. Static,
// like the table, so a read allocates nothing and costs no init.
var namedPrograms = [...]string{"CMakeLists.txt"}

// KindKey is the key the kinds table files rel under, rel a slash path:
// its base name when that is a named program, else its extension with the
// dot, else its base name.
func KindKey(rel string) string {
	base := path.Base(rel)
	for _, n := range namedPrograms {
		if base == n {
			return n
		}
	}
	if ext := path.Ext(rel); ext != "" {
		return ext
	}
	return base
}

// NonSourceKinds is the kinds table's rows in order, for a caller outside
// lang that ranges over every row. It returns the array, which is copied
// on return: the caller cannot change the table, and a read allocates
// nothing and costs no init.
func NonSourceKinds() [len(nonSourceKinds)]string { return nonSourceKinds }

// NonSource reports whether rel's kind is a row of the kinds table.
func NonSource(rel string) bool {
	k := KindKey(rel)
	for _, row := range nonSourceKinds {
		if row == k {
			return true
		}
	}
	return false
}
