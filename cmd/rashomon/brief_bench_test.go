package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/hookio"
	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// BenchmarkProbeStart is what card BRIEF1 adds to probe start. brief is
// printBrief at startup, on a payload already taken in: the source and cwd
// reads, the root's open and baseline.Key, spec discovery from the cwd, the
// four instruction Lstats, the thirteen manifest Lstats and go.mod's head,
// the project index read (24 families), the packages walk (a .git root,
// 32 package directories and cmd/app, 71 entries) and the JSON line. The
// user's Claude directory holds no CLAUDE.md and the managed settings path
// is absent (benchSetenv). The args are a settings install's, so the origin
// check stops at the --install marker; the plugin's entry adds
// install.PluginFires's reads, as the recap's pointer does.
func BenchmarkProbeStart(b *testing.B) {
	b.Run("brief", func(b *testing.B) {
		storeRoot, cfg, dir := b.TempDir(), b.TempDir(), b.TempDir()
		benchSetenv(b, storeRoot, cfg)
		files := map[string]string{"CLAUDE.md": "# rules\n", "go.mod": "module github.com/x/y\n\ngo 1.25\n",
			"cmd/app/main.go": "package main\n"}
		for i := range 32 {
			files[fmt.Sprintf("pkg/p%02d/x.go", i)] = "package p\n"
		}
		if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
			b.Fatal(err)
		}
		for name, body := range files {
			p := filepath.Join(dir, filepath.FromSlash(name))
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				b.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
				b.Fatal(err)
			}
		}
		root, ok := briefRoot(dir)
		if !ok {
			b.Fatalf("%s is not a readable directory", dir)
		}
		idx := &verify.ProjectIndex{ProjectKey: root}
		for i := range 24 {
			idx.Entries = append(idx.Entries, verify.IndexEntry{Family: fmt.Sprintf("%064x@%064x", i, i), Kind: "test",
				Sessions: 1 + i%3, LastSession: "s", FirstSeen: 1, LastSeen: 2})
		}
		if _, err := verify.WriteIndex(verify.IndexPath(storeRoot, root), idx); err != nil {
			b.Fatal(err)
		}
		head := newBenchHead("SessionStart")
		head.CWD = dir
		payload, err := hookio.IntakeBytes(benchJSON(b, struct {
			benchHead
			Source string `json:"source"`
		}{head, "startup"}))
		if err != nil {
			b.Fatal(err)
		}
		args := []string{store.PhaseStart, install.Marker, "bench"}
		var out countingWriter
		printBrief(args, payload, storeRoot, &out)
		if out.n == 0 {
			b.Fatal("printBrief wrote nothing")
		}
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			printBrief(args, payload, storeRoot, io.Discard)
		}
	})
}

// countingWriter counts the bytes written to it.
type countingWriter struct{ n int }

func (w *countingWriter) Write(p []byte) (int, error) {
	w.n += len(p)
	return len(p), nil
}
