package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
		st, err := store.Open(storeRoot)
		if err != nil {
			b.Fatal(err)
		}
		var out countingWriter
		printBrief(args, payload, st, &out)
		if out.n == 0 {
			b.Fatal("printBrief wrote nothing")
		}
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			printBrief(args, payload, st, io.Discard)
		}
	})
	for _, row := range []struct {
		name string
		fix  benchPrevious
		want string
	}{
		{"previous_hit", benchPrevious{previous: true}, "last session: "},
		{"previous_bound", benchPrevious{previous: true, others: 900, big: true}, "last session: "},
		{"previous_2000", benchPrevious{previous: true, others: 2000}, "last session: "},
	} {
		b.Run(row.name, func(b *testing.B) {
			st, payload, args := benchPreviousFixture(b, row.fix)
			var out countingWriter
			printBrief(args, payload, st, &out)
			if out.n == 0 {
				b.Fatal("printBrief wrote nothing")
			}
			var msg strings.Builder
			printBrief(args, payload, st, &msg)
			if !strings.Contains(msg.String(), row.want) {
				b.Fatalf("the brief holds no %q: %s", row.want, msg.String())
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				printBrief(args, payload, st, io.Discard)
			}
		})
	}
}

// benchPreviousFixture is a project directory and a store, for the rows that
// read the previous session at startup (card REPORT1b). The line comes from
// the project's checkpoint file and the one checkpoint it names, so the rows
// show a cost that does not follow the store: previous_hit is the previous
// session alone; previous_bound adds 900 run directories of another project
// and a 20 MiB records file newer than all of them; previous_2000 adds 2,000
// run directories. Each wants the same line.
// benchPrevious is what benchPreviousFixture seeds: the project's previous
// session, others runs of another project written before it, and a 20 MiB
// records file newer than all of them.
type benchPrevious struct {
	previous bool
	others   int
	big      bool
}

func benchPreviousFixture(b *testing.B, fx benchPrevious) (*store.Store, *hookio.Payload, []string) {
	b.Helper()
	storeRoot, cfg, dir := b.TempDir(), b.TempDir(), b.TempDir()
	benchSetenv(b, storeRoot, cfg)
	st, err := store.Open(storeRoot)
	if err != nil {
		b.Fatal(err)
	}
	root, ok := briefRoot(dir)
	if !ok {
		b.Fatalf("%s is not a readable directory", dir)
	}
	line := func(session, project string) string {
		return fmt.Sprintf(`{"type":"verify","schema_version":%d,"recorded_at_unix_ms":1760000000000,"session_id":%q,"project_root":%q,`+
			`"point_seq":7,"record_green":true,"gates":[{"gate":"go","required":true,"state":"green"}],"gates_omitted":0,`+
			`"integrity":{"unresolved":[]},"integrity_omitted":0}`+"\n", store.EventSchemaVersion, session, project)
	}
	seed := func(session, body string, at time.Time) {
		d := st.RunDir(session)
		if err := os.MkdirAll(d, 0o700); err != nil {
			b.Fatal(err)
		}
		p := filepath.Join(d, store.FileRecords)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			b.Fatal(err)
		}
		if err := os.Chtimes(p, at, at); err != nil {
			b.Fatal(err)
		}
	}
	base := time.Now().Add(-time.Hour)
	if fx.previous {
		seed("previous", line("previous", root), base)
		// The Stop that cut its checkpoint also noted it in the project's
		// checkpoint file.
		c := &verify.Checkpoints{Last: &verify.LastCheckpoint{SessionID: "previous", PointSeq: 7, AtMS: 1760000000000}}
		if err := verify.WriteCheckpoints(verify.CheckpointsPath(storeRoot, root), c); err != nil {
			b.Fatal(err)
		}
	}
	{
		for i := range fx.others {
			seed(fmt.Sprintf("other-%04d", i), line("other", "/other/project"), base.Add(-time.Duration(i+1)*time.Second))
		}
	}
	if fx.big {
		big := line("big", root) + strings.Repeat(`{"type":"declaration","schema_version":5,"pad":"`+strings.Repeat("x", 4000)+`"}`+"\n", 20<<20/4050)
		seed("big", big, base.Add(time.Minute))
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
	return st, payload, []string{store.PhaseStart, install.Marker, "bench"}
}

// countingWriter counts the bytes written to it.
type countingWriter struct{ n int }

func (w *countingWriter) Write(p []byte) (int, error) {
	w.n += len(p)
	return len(p), nil
}
