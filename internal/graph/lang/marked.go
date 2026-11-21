package lang

// Marked names a language whose test files the edit-marks table reads
// (W-M3.7). It is a file kind a table keys its rows by, not a language the
// graph reads: a Marked language has an extractor only where a Resolver
// claims its files.
type Marked uint8

const (
	// NotMarked is any file no table reads.
	NotMarked Marked = iota
	MarkedGo
	MarkedPython
)

// MarkedOf is the Marked language of the file path by its suffix, in any
// case, and NotMarked for any other. It allocates nothing.
func MarkedOf(path []byte) Marked {
	switch {
	case hasSuffixFold(path, ".go"):
		return MarkedGo
	case hasSuffixFold(path, ".py"):
		return MarkedPython
	}
	return NotMarked
}

func hasSuffixFold(b []byte, suffix string) bool {
	if len(b) < len(suffix) {
		return false
	}
	for i := 0; i < len(suffix); i++ {
		c := b[len(b)-len(suffix)+i]
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c != suffix[i] {
			return false
		}
	}
	return true
}
