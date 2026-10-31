package hook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

var cs1Key = []byte("cwd-symlink-test-key-0123456789ab")

// cs1Tree is a project whose .git sits in real/, reached through link, a
// symlink to real/ in the same temp tree. t.TempDir is itself under /var on
// macOS, a symlink to /private/var, so every spelling here already crosses one.
type cs1Tree struct{ spelled, physical, link string }

func newCS1Tree(t *testing.T) cs1Tree {
	t.Helper()
	base := t.TempDir()
	phys, err := filepath.EvalSymlinks(base)
	if err != nil {
		t.Fatal(err)
	}
	real := filepath.Join(base, "real")
	if err := os.MkdirAll(filepath.Join(real, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(real, "web"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	return cs1Tree{spelled: base, physical: phys, link: link}
}

func cs1Digest(t *testing.T, cmd, cwd string) string {
	t.Helper()
	in, _ := json.Marshal(map[string]string{"command": cmd})
	_, d := Locate(cs1Key, "Bash", in, cwd)
	return d
}

// TestCS1_LocateKeysTheDirectoryAsItIsOnDisk: a cwd reached through a symlink,
// through /var on macOS, or through both, digests the same as its physical
// spelling, with and without a leading cd.
func TestCS1_LocateKeysTheDirectoryAsItIsOnDisk(t *testing.T) {
	tr := newCS1Tree(t)
	want := shape.CWDDigest(cs1Key, filepath.Join(tr.physical, "real", "web"))
	for _, c := range []struct{ name, cmd, cwd string }{
		{"symlinked cwd, cd step", "cd web && go test ./...", tr.link},
		{"symlinked cwd, nested", "go test ./...", filepath.Join(tr.link, "web")},
		{"temp-dir spelling", "go test ./...", filepath.Join(tr.spelled, "real", "web")},
		{"physical spelling", "go test ./...", filepath.Join(tr.physical, "real", "web")},
		{"absolute cd through the link", "cd " + filepath.Join(tr.link, "web") + " && go test ./...", "/"},
	} {
		if got := cs1Digest(t, c.cmd, c.cwd); got != want {
			t.Errorf("%s: digest %s, want the physical directory's %s", c.name, got, want)
		}
	}
	if runtime.GOOS == "darwin" {
		if p, err := filepath.EvalSymlinks("/var"); err != nil || p != "/private/var" {
			t.Fatalf("/var resolves to %q, %v; the macOS case needs /private/var", p, err)
		}
		a, b := cs1Digest(t, "go test", "/var"), cs1Digest(t, "go test", "/private/var")
		if a != b {
			t.Errorf("/var and /private/var digest apart: %s, %s", a, b)
		}
	}
}

// TestCS1b_AnUnresolvableDirKeepsTheSpelledDigest: a directory that is not
// on disk digests as spelled, and an unknown one stays unknown.
func TestCS1b_AnUnresolvableDirKeepsTheSpelledDigest(t *testing.T) {
	tr := newCS1Tree(t)
	gone := filepath.Join(tr.link, "missing")
	if got, want := cs1Digest(t, "go test", gone), shape.CWDDigest(cs1Key, gone); got != want {
		t.Errorf("missing dir digest %s, want the spelled %s", got, want)
	}
	// A cd step into a directory that is not there: the fold's own
	// resolution fails, and the folded spelling is digested.
	if got, want := cs1Digest(t, "cd missing && go test", tr.link), shape.CWDDigest(cs1Key, gone); got != want {
		t.Errorf("cd into a missing dir digest %s, want the spelled %s", got, want)
	}
	if got, want := cs1Digest(t, "go test", ""), shape.CWDDigest(cs1Key, ""); got != want {
		t.Errorf("no cwd digest %s, want the unknown location %s", got, want)
	}
}

// TestCS1c_TheDigestAgreesAtBothScopes: through a symlinked cwd the hook
// records the same location digest whatever the record scope (each store has
// its own key, so each is compared with the digest of the physical directory
// under that store's key), and a path only at v5.
func TestCS1c_TheDigestAgreesAtBothScopes(t *testing.T) {
	tr := newCS1Tree(t)
	for _, scope := range []string{store.RecordScopeV4, store.RecordScopeV5} {
		f := newST(t)
		if err := store.WriteRecordScopeMarker(f.st.Root(), scope); err != nil {
			t.Fatal(err)
		}
		f.cwd = tr.link
		f.call("toolu_a", "Bash", goTest, 0)
		run, err := f.st.ReadRun(stSession)
		if err != nil || len(run.Declarations) == 0 {
			t.Fatalf("%s: %v", scope, err)
		}
		d := run.Declarations[0]
		if want := shape.CWDDigest(f.st.Key(), filepath.Join(tr.physical, "real")); d.CWDDigest != want {
			t.Errorf("%s: digest %s, want the physical project's %s", scope, d.CWDDigest, want)
		}
		if scope == store.RecordScopeV4 && (d.CWD != nil || d.Project != nil) {
			t.Errorf("v4 stored a path: cwd %v project %v", d.CWD, d.Project)
		}
		if scope == store.RecordScopeV5 && (d.Project == nil || *d.Project != filepath.Join(tr.physical, "real")) {
			t.Errorf("v5 project %v, want the physical %s", d.Project, filepath.Join(tr.physical, "real"))
		}
	}
}

// TestCS1_ASymlinkedProjectBindsItsDeclaredGate: a trusted spec's declared
// gate, in a project opened through a symlink, binds the run that ran in it:
// the gate reads green, not absent.
func TestCS1_ASymlinkedProjectBindsItsDeclaredGate(t *testing.T) {
	tr := newCS1Tree(t)
	f := newST(t)
	f.cwd = tr.link
	f.call("toolu_e", "Edit", map[string]any{"file_path": filepath.Join(tr.link, "a.go"), "old_string": "a", "new_string": "b"}, 0)
	f.call("toolu_g", "Bash", goTest, 0)
	run, err := f.st.ReadRun(stSession)
	if err != nil {
		t.Fatal(err)
	}
	yes := true
	sp := &spec.Compiled{
		Sources: []spec.Source{{Path: filepath.Join(tr.link, ".claude/rashomon/spec.json"), Digest: "sha256:0", Trust: spec.Trusted}},
		Trust:   spec.Trusted,
		Gates: []spec.Declared{{ID: "spec:unit", Gate: spec.Gate{ID: "unit", Kind: "test",
			Argv: []string{"go", "test", "./..."}, Match: "prefix", Required: &yes}}},
	}
	snap, _ := verify.EvaluateWithObligations(run, nil, sp, f.st.Key(),
		verify.Point{Kind: verify.PointStop, Seq: 1 << 40, SessionID: stSession}, time.UnixMilli(1_800_000_600_000))
	for _, g := range snap.Gates {
		if g.Gate == "spec:unit" {
			if g.State != verify.Green {
				t.Fatalf("spec:unit is %s (%s), want green: the run did not bind the gate", g.State, g.Reason)
			}
			return
		}
	}
	t.Fatalf("no spec:unit row in %+v", snap.Gates)
}

// TestCS1d_OnlyACdStepResolvesTheFoldedDirectory: a line with no cd step uses
// the cwd the hook already resolved for the project key, and resolves nothing
// more; a line with one resolves its folded directory, once.
func TestCS1d_OnlyACdStepResolvesTheFoldedDirectory(t *testing.T) {
	tr := newCS1Tree(t)
	calls := 0
	old := evalSymlinks
	evalSymlinks = func(p string) (string, error) { calls++; return old(p) }
	t.Cleanup(func() { evalSymlinks = old })
	for _, c := range []struct {
		cmd  string
		want int
	}{{"go test ./...", 0}, {"cd web && go test ./...", 1}} {
		calls = 0
		in, _ := json.Marshal(map[string]string{"command": c.cmd})
		locate(cs1Key, "Bash", in, tr.link, tr.link)
		if calls != c.want {
			t.Errorf("%q: %d resolutions, want %d", c.cmd, calls, c.want)
		}
	}
}
