// Package gateinfer names a project's test command from its own files when
// nothing better is known (card GATE-INFER1). Red: the tests come first.
package gateinfer

import "slices"

// Inferred is a test command read from the project's own files.
type Inferred struct {
	Argv     []string
	Dir      string
	From     string
	More     int
	Declared bool
}

var commands [][]string

type manifest struct {
	file string
	argv []string
}

func manifests() []manifest { return nil }

type found struct {
	argv []string
	dir  string
}

func command(line string) (found, bool) { return found{}, false }

func hasPrefix(argv, row []string) bool {
	return len(argv) >= len(row) && slices.Equal(argv[:len(row)], row)
}

// Infer is the test command of the project at root, and whether one was
// found.
func Infer(root string) (Inferred, bool) {
	return Inferred{}, false
}
