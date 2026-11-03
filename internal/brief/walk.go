package brief

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// walkGo is the directories at most maxDepth below root that hold a .go
// file, found breadth first, skipping what the go tool skips (names starting
// with "." or "_", testdata) and vendor, and never following a symlink. It
// stops, capped, at MaxWalkEntries entries read or once clock reads
// WalkBudget past its first reading, checked before each read.
func walkGo(root string, clock func() time.Time) (dirs []string, entries int, capped bool) {
	start := clock()
	over := func() bool { return clock().Sub(start) >= WalkBudget }
	level := []string{root}
	for depth := 0; depth <= maxDepth && len(level) > 0; depth++ {
		var next []string
		for _, dir := range level {
			subs, hasGo, n, short := readGoDir(dir, MaxWalkEntries-entries, over)
			entries += n
			if hasGo {
				dirs = append(dirs, dir)
			}
			if short {
				return dirs, entries, true
			}
			next = append(next, subs...)
		}
		level = next
	}
	return dirs, entries, false
}

// readGoDir reads dir in chunks, at most left entries in all, asking over
// before each chunk: dir's subdirectories to walk, whether it holds a .go
// file, how many entries it read, and whether it stopped short of its end.
// A directory that cannot be opened or read is read as far as it can be.
func readGoDir(dir string, left int, over func() bool) (subs []string, hasGo bool, n int, short bool) {
	f, err := os.Open(dir)
	if err != nil {
		return nil, false, 0, false
	}
	defer f.Close() //nolint:errcheck // read-only
	for {
		if n >= left {
			// The cap is reached, not necessarily passed: the directory
			// is short only if one more entry is there to read.
			ents, _ := f.ReadDir(1)
			return subs, hasGo, n, len(ents) > 0
		}
		if over() {
			return subs, hasGo, n, true
		}
		ents, err := f.ReadDir(min(walkChunk, left-n))
		n += len(ents)
		for _, e := range ents {
			name := e.Name()
			switch {
			case name[0] == '.' || name[0] == '_' || name == "testdata" || name == "vendor":
			case e.IsDir():
				subs = append(subs, filepath.Join(dir, name))
			case e.Type().IsRegular() && strings.HasSuffix(name, ".go"):
				hasGo = true
			}
		}
		if err != nil {
			return subs, hasGo, n, false
		}
	}
}
