package hook

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// BenchmarkTreeDigest is the Stop's side of the tree binding at its cap (card
// P5A, D140; the B48 line: per claim, verify.MaxTreeFiles files of
// MaxHashBytes each is the worst case): TreeShas over 64 targets of 1 MiB,
// each stat, opened and hashed, 64 MiB read per op from the page cache.
// files_64 is the row test/perf/budgets.json gates; ns/op is ungated.
func BenchmarkTreeDigest(b *testing.B) {
	dir := b.TempDir()
	data := bytes.Repeat([]byte("x"), MaxHashBytes)
	targets := make([]verify.TreeTarget, verify.MaxTreeFiles)
	for i := range targets {
		name := fmt.Sprintf("f%02d.go", i)
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data, 0o644); err != nil {
			b.Fatal(err)
		}
		targets[i] = verify.TreeTarget{Path: name, File: path}
	}
	b.Run("files_64", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			if tree := TreeShas(targets); len(tree) != verify.MaxTreeFiles {
				b.Fatalf("%d entries, want %d", len(tree), verify.MaxTreeFiles)
			}
		}
	})
}
