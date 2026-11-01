package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/pgraph"
)

// module writes files (slash paths to contents) under a fresh directory,
// symlink-resolved, and returns it.
func module(t *testing.T, files map[string]string) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		write(t, filepath.Join(dir, filepath.FromSlash(name)), body)
	}
	return dir
}

func write(t *testing.T, p, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// smallModule has an interface call CHA resolves, so a sync writes both
// tiers.
var smallModule = map[string]string{
	"go.mod":      "module example.com/m\n\ngo 1.25.0\n",
	"a/a.go":      "package a\n\ntype I interface{ M() }\n\ntype T struct{}\n\nfunc (T) M() {}\n\nfunc Call(i I) { i.M() }\n",
	"a/a_test.go": "package a\n",
	"b/b.go":      "package b\n\nimport \"example.com/m/a\"\n\nfunc G() { a.Call(a.T{}) }\n",
}

// sync runs `pgraph sync root` and returns its exit status, stdout and stderr.
func sync(root string, extra ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := run(append(append([]string{"sync"}, extra...), root), &out, &errb)
	return code, out.String(), errb.String()
}

func sha(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

// TestPLW1e_SyncWritesTheLogTheDigestsAndAnIgnore (card PL-WIRE1 a): `pgraph
// sync <root>` writes root/.rashomon/pgraph.log in Encode's format, which
// decodes to both tiers' facts, the digests of every compiled file (no test
// file), and a .gitignore of "*"; a .gitignore already there is left as it
// is. Break: skip SyncCHA's half, and no may-call fact is live.
func TestPLW1e_SyncWritesTheLogTheDigestsAndAnIgnore(t *testing.T) {
	root := module(t, smallModule)
	code, out, errs := sync(root)
	if code != 0 || !strings.HasPrefix(out, "pgraph: synced 2 packages in 1 module into .rashomon/pgraph.log\npgraph: facts PROVEN +") || errs != "" {
		t.Fatalf("sync: exit %d, stdout %q, stderr %q", code, out, errs)
	}
	dir := filepath.Join(root, ".rashomon")
	l, err := pgraph.DecodeLog(read(t, filepath.Join(dir, "pgraph.log")))
	if err != nil {
		t.Fatalf("the log does not decode: %v", err)
	}
	want := map[pgraph.Triple]pgraph.Tier{
		{Subject: "pkg:example.com/m/a", Predicate: pgraph.Declares, Object: "func:example.com/m/a.Call"}:       pgraph.Proven,
		{Subject: "func:example.com/m/b.G", Predicate: pgraph.Calls, Object: "func:example.com/m/a.Call"}:       pgraph.Proven,
		{Subject: "func:example.com/m/a.Call", Predicate: pgraph.MayCall, Object: "method:example.com/m/a.T.M"}: pgraph.Algorithmic,
	}
	for _, f := range l.Live() {
		if tier, ok := want[f.Triple()]; ok && tier == f.Tier() {
			delete(want, f.Triple())
		}
	}
	if len(want) > 0 {
		t.Errorf("facts not live after a sync: %v", want)
	}
	digests := "pgraph-digests v0\n" +
		"example.com/m/a\ta/a.go\t" + sha(read(t, filepath.Join(root, "a", "a.go"))) + "\n" +
		"example.com/m/b\tb/b.go\t" + sha(read(t, filepath.Join(root, "b", "b.go"))) + "\n"
	if got := string(read(t, filepath.Join(dir, "pgraph.digests"))); got != digests {
		t.Errorf("digests:\n%s\nwant:\n%s", got, digests)
	}
	if got := string(read(t, filepath.Join(dir, ".gitignore"))); got != "*\n" {
		t.Errorf(".gitignore %q, want \"*\\n\"", got)
	}
	write(t, filepath.Join(dir, ".gitignore"), "pgraph.log\n")
	if code, _, errs := sync(root); code != 0 || string(read(t, filepath.Join(dir, ".gitignore"))) != "pgraph.log\n" {
		t.Errorf("a second sync: exit %d (%s), and the person's .gitignore was rewritten", code, errs)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 3 {
		t.Errorf("%s holds %d entries, want the log, the digests and .gitignore (no temp file left)", dir, len(entries))
	}
}

// TestPLW1f_ResyncOnlyAppends: a sync of an unchanged tree leaves the log
// byte for byte; after an edit that drops a call the old log is a prefix of
// the new one, the call is retracted, and the digests name the new bytes.
// Break: start every sync from an empty log, and the history is rewritten.
func TestPLW1f_ResyncOnlyAppends(t *testing.T) {
	root := module(t, smallModule)
	logPath := filepath.Join(root, ".rashomon", "pgraph.log")
	if code, _, errs := sync(root); code != 0 {
		t.Fatalf("first sync: exit %d: %s", code, errs)
	}
	first := read(t, logPath)
	if code, out, errs := sync(root); code != 0 || !bytes.Equal(read(t, logPath), first) || !strings.HasSuffix(out, "\npgraph: facts PROVEN +0 -0, ALGORITHMIC +0 -0\n") {
		t.Fatalf("an unchanged resync: exit %d, %q %q; want the log as it was", code, out, errs)
	}
	write(t, filepath.Join(root, "b", "b.go"), "package b\n\nfunc G() {}\n")
	if code, _, errs := sync(root); code != 0 {
		t.Fatalf("resync after an edit: exit %d: %s", code, errs)
	}
	second := read(t, logPath)
	if !bytes.HasPrefix(second, first) || len(second) == len(first) {
		t.Fatal("the edit's sync did not append to the log it read")
	}
	if !strings.Contains(string(second[len(first):]), "-\tfunc:example.com/m/b.G\tcalls\tfunc:example.com/m/a.Call\tPROVEN\t") {
		t.Errorf("the dropped call has no tombstone in %q", second[len(first):])
	}
	if d := string(read(t, filepath.Join(root, ".rashomon", "pgraph.digests"))); !strings.Contains(d, "b/b.go\t"+sha([]byte("package b\n\nfunc G() {}\n"))) {
		t.Errorf("digests %q do not name b.go's new bytes", d)
	}
}

// TestPLW1g_AFailedSyncWritesNothing: a log that does not decode is refused
// and left as it was, with no digests written; a module that does not type
// check, and a root with no go.mod, write no .rashomon at all; a nested
// module's packages are synced with the root's. Break: start over on a log
// that does not decode, and its history is lost without a word.
func TestPLW1g_AFailedSyncWritesNothing(t *testing.T) {
	root := module(t, smallModule)
	corrupt := filepath.Join(root, ".rashomon", "pgraph.log")
	write(t, corrupt, "pgraph-log v0\n+\tnot a record\n")
	if code, _, errs := sync(root); code != 1 || !strings.Contains(errs, "does not decode") {
		t.Errorf("a corrupt log: exit %d, stderr %q; want 1 naming it", code, errs)
	}
	if string(read(t, corrupt)) != "pgraph-log v0\n+\tnot a record\n" {
		t.Error("the corrupt log was rewritten")
	}
	if _, err := os.Stat(filepath.Join(root, ".rashomon", "pgraph.digests")); err == nil {
		t.Error("a refused sync wrote digests")
	}
	for name, files := range map[string]map[string]string{
		"a type error": {"go.mod": "module example.com/m\n", "a/a.go": "package a\n\nvar X int = \"s\"\n"},
		"no go.mod":    {"a/a.go": "package a\n"},
	} {
		dir := module(t, files)
		if code, _, _ := sync(dir); code != 1 {
			t.Errorf("%s: exit %d, want 1", name, code)
		}
		if _, err := os.Stat(filepath.Join(dir, ".rashomon")); err == nil {
			t.Errorf("%s: .rashomon was written", name)
		}
	}
	nested := module(t, map[string]string{"go.mod": "module example.com/r\n", "r.go": "package r\n",
		"sub/go.mod": "module example.com/s\n", "sub/s.go": "package s\n"})
	if code, out, errs := sync(nested); code != 0 || !strings.HasPrefix(out, "pgraph: synced 2 packages in 2 modules into") {
		t.Errorf("a nested module: exit %d, %q %q; want both modules synced", code, out, errs)
	}
}

// TestPLW1h_UsageIsExitTwo: no verb, another verb, no root, two roots and a
// timeout that is not positive are usage errors and write nothing.
func TestPLW1h_UsageIsExitTwo(t *testing.T) {
	root := module(t, smallModule)
	for _, args := range [][]string{nil, {"load", root}, {"sync"}, {"sync", root, root}, {"sync", "-timeout", "0s", root}} {
		var out, errb bytes.Buffer
		if code := run(args, &out, &errb); code != 2 || !strings.Contains(errb.String(), "usage: pgraph sync") {
			t.Errorf("run(%q) = %d, stderr %q; want 2 and the usage", args, code, errb.String())
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".rashomon")); err == nil {
		t.Error("a usage error wrote .rashomon")
	}
}
