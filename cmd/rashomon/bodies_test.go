package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// bodiesHere is a store under RASHOMON_HOME and a project the test has made
// its working directory, with the key and directory the probe would name it
// by (card 4.1).
func bodiesHere(t *testing.T) (st *store.Store, key, dir string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("RASHOMON_HOME", root)
	st, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	key = baseline.Key(cwd)
	return st, key, strings.TrimSuffix(baseline.FileName(key), baseline.FileExt)
}

// writeBody puts a file of n bytes under bodies/<dir>/ as a copy would be.
func writeBody(t *testing.T, st *store.Store, dir, name string, n int) {
	t.Helper()
	p := filepath.Join(st.Root(), store.DirBodies, dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, bytes.Repeat([]byte("x"), n), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestRC14i_BodiesOnOffStatus: on lists the working directory's project
// with the default cap, --max-bytes sets the cap, off removes this project
// and keeps every other, and status says which, counting copies and never a
// temporary file. Arguments it does not know, --help among them, are
// refused as forget refuses them (forget.go cmdForget's default case).
func TestRC14i_BodiesOnOffStatus(t *testing.T) {
	st, key, dir := bodiesHere(t)
	var out bytes.Buffer
	if err := cmdBodies([]string{"on"}, &out); err != nil {
		t.Fatalf("bodies on: %v", err)
	}
	set, ok, err := store.ReadBodies(st.Root())
	if !ok || err != nil || set.MaxBytes != store.DefaultBodiesMaxBytes || !slices.Equal(set.Projects, []string{key}) {
		t.Fatalf("after bodies on: %+v, %v, %v; want %s listed at the default cap", set, ok, err, key)
	}
	if err := cmdBodies([]string{"on", "--max-bytes", "4096"}, &out); err != nil {
		t.Fatalf("bodies on --max-bytes: %v", err)
	}
	if set, _, _ = store.ReadBodies(st.Root()); set.MaxBytes != 4096 || !slices.Equal(set.Projects, []string{key}) {
		t.Errorf("after --max-bytes 4096: %+v; want the cap 4096 and the key once", set)
	}

	writeBody(t, st, dir, "s1.jsonl", 100)
	writeBody(t, st, dir, store.BodiesTempPrefix+"left", 50)
	out.Reset()
	if err := cmdBodies([]string{"status"}, &out); err != nil {
		t.Fatalf("bodies status: %v", err)
	}
	if want := "bodies: 1 sessions, " + store.FormatBytes(100) + " of " + store.FormatBytes(4096) + " (" + key + " on)\n"; out.String() != want {
		t.Errorf("status while on = %q, want %q (copies only, no temporary file)", out.String(), want)
	}

	if err := store.WriteBodies(st.Root(), store.BodiesSetting{MaxBytes: 4096, Projects: []string{"/other", key}}); err != nil {
		t.Fatal(err)
	}
	if err := cmdBodies([]string{"off"}, &out); err != nil {
		t.Fatalf("bodies off: %v", err)
	}
	if set, _, _ = store.ReadBodies(st.Root()); !slices.Equal(set.Projects, []string{"/other"}) || set.MaxBytes != 4096 {
		t.Errorf("after bodies off: %+v; want /other alone, the cap kept", set)
	}
	out.Reset()
	if err := cmdBodies([]string{"status"}, &out); err != nil || !strings.HasSuffix(out.String(), " ("+key+" off)\n") {
		t.Errorf("status while off = (%q, %v), want the line ending (%s off)", out.String(), err, key)
	}

	for _, args := range [][]string{nil, {"--help"}, {"maybe"}, {"off", "--max-bytes", "1"}, {"on", "--max-bytes", "0"}, {"on", "--max-bytes", "lots"}} {
		if err := cmdBodies(args, &out); err == nil {
			t.Errorf("bodies %q was taken, want a refusal", args)
		}
	}
}

// TestRC14j_ForgetOneKindOnly: forget takes one kind, now one of three, and
// its refusal names all three; --bodies alone removes this project's copies
// and nothing else: not another project's, and not a run.
func TestRC14j_ForgetOneKindOnly(t *testing.T) {
	st, key, dir := bodiesHere(t)
	for _, args := range [][]string{{"--bodies", "--host", "example.com"}, {"--bodies", "--since", "1h"}, {"--host", "example.com", "--before", "1h"}} {
		err := cmdForget(args, &bytes.Buffer{})
		if err == nil {
			t.Fatalf("forget %q was taken, want a refusal", args)
		}
		for _, kind := range []string{"--host", "--since/--before", "--bodies"} {
			if !strings.Contains(err.Error(), kind) {
				t.Errorf("forget %q refused with %q, which does not name %s", args, err, kind)
			}
		}
	}
	if err := cmdForget(nil, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "--bodies") {
		t.Errorf("forget with no argument = %v, want a refusal naming --bodies too", err)
	}

	if err := st.AppendDeclaration(store.Declaration{Type: store.TypeDeclaration, SchemaVersion: store.SchemaVersion,
		ToolUseID: "toolu_kept", SessionID: "sess-kept"}); err != nil {
		t.Fatal(err)
	}
	writeBody(t, st, dir, "s1.jsonl", 10)
	writeBody(t, st, dir, "s2.jsonl", 10)
	writeBody(t, st, "other-project", "s3.jsonl", 10)
	var out bytes.Buffer
	if err := cmdForget([]string{"--bodies"}, &out); err != nil {
		t.Fatalf("forget --bodies: %v", err)
	}
	if want := "forgot 2 transcript copies of " + key + "\n"; out.String() != want {
		t.Errorf("forget --bodies said %q, want %q", out.String(), want)
	}
	if _, err := os.Stat(filepath.Join(st.Root(), store.DirBodies, dir)); !os.IsNotExist(err) {
		t.Errorf("this project's copies survived forget --bodies: %v", err)
	}
	if _, err := os.Stat(filepath.Join(st.Root(), store.DirBodies, "other-project", "s3.jsonl")); err != nil {
		t.Errorf("another project's copy went too: %v", err)
	}
	if _, err := os.Stat(st.RunDir("sess-kept")); err != nil {
		t.Errorf("forget --bodies removed a run: %v", err)
	}
}

// TestRC14k_StatusLineBesideStoreSize: status prints the copies' line on
// the line after the store's size.
func TestRC14k_StatusLineBesideStoreSize(t *testing.T) {
	root, cfg := t.TempDir(), t.TempDir()
	benchSetenv(t, root, cfg)
	if _, err := store.Open(root); err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	var out bytes.Buffer
	if err := cmdStatus(&out); err != nil {
		t.Fatalf("status: %v", err)
	}
	lines := strings.Split(out.String(), "\n")
	i := slices.IndexFunc(lines, func(l string) bool { return strings.HasPrefix(l, "  size: ") })
	if i < 0 || i+1 >= len(lines) || !strings.HasPrefix(lines[i+1], "  bodies: 0 sessions, ") {
		t.Errorf("status has no bodies line after its size line:\n%s", out.String())
	}
}

// TestRC14l_OverCapPrintsOneLine: a transcript larger than the whole cap is
// not copied, and the end probe says so in one fixed line, exiting 0 as it
// always does; one under the cap is copied and the probe says nothing.
func TestRC14l_OverCapPrintsOneLine(t *testing.T) {
	for _, c := range []struct {
		name     string
		maxBytes int64
		stderr   string
		copied   bool
	}{{"over", 16, bodyOverCapLine + "\n", false}, {"under", 1 << 20, "", true}} {
		t.Run(c.name, func(t *testing.T) {
			root, installID := benchStarted(t)
			if err := store.WriteBodies(root, store.BodiesSetting{MaxBytes: c.maxBytes, Projects: []string{baseline.Key(benchCWD)}}); err != nil {
				t.Fatal(err)
			}
			grantBodiesHere(t, root)
			transcript := filepath.Join(t.TempDir(), "t.jsonl")
			if err := os.WriteFile(transcript, bytes.Repeat([]byte("y"), 64), 0o600); err != nil {
				t.Fatal(err)
			}
			head := newBenchHead("SessionEnd")
			head.TranscriptPath = transcript
			end := benchJSON(t, struct {
				benchHead
				Reason string `json:"reason"`
			}{head, "exit"})
			var stderr bytes.Buffer
			if code := cmdProbe([]string{"end", install.Marker, installID}, bytes.NewReader(end), io.Discard, &stderr); code != exitOK || stderr.String() != c.stderr {
				t.Errorf("probe end: exit %d, stderr %q; want exit 0, stderr %q", code, stderr.String(), c.stderr)
			}
			st, err := store.Open(root)
			if err != nil {
				t.Fatal(err)
			}
			if n, _, err := st.BodiesTotal(); err != nil || (n == 1) != c.copied {
				t.Errorf("copies after the end probe = (%d, %v), want copied %v", n, err, c.copied)
			}
		})
	}
}
