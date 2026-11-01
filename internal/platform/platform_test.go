package platform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/verify"
)

const (
	pkgA = "example.com/m/a"
	pkgB = "example.com/m/b"
	cfg  = "linux/amd64/cgo=0/tags="
)

// fixtureLog is testdata/facts.log, pgraph's pinned Encode of its fixture
// graph (TestPL8h_FixturePinned; TestPL8j_FixtureIsPgraphs pins the copy).
func fixtureLog(t testing.TB) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "facts.log"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// project is a module root, example.com/m, holding log as its pgraph log.
func project(t testing.TB, log []byte) (root, logPath string) {
	t.Helper()
	root = t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/m\n\ngo 1.25\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	logPath = filepath.Join(root, "facts.log")
	if err := os.WriteFile(logPath, log, 0o600); err != nil {
		t.Fatal(err)
	}
	return root, logPath
}

// viewHash is the SHA-256 of view lines, each "subject predicate object" at
// PROVEN under cfg, given here in the view's order.
func viewHash(triples ...string) string {
	h := sha256.New()
	for _, tr := range triples {
		h.Write([]byte(strings.ReplaceAll(tr, " ", "\t") + "\tPROVEN\t" + cfg + "\n"))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// The fixture's two families: a's facts are the package's two declares, its
// type's method and the method's call; b's are its live declare and its
// function's call, the retracted var gone.
var (
	hashA = viewHash(
		"method:example.com/m/a.T.M calls func:example.com/m/a.F",
		"pkg:example.com/m/a declares func:example.com/m/a.F",
		"pkg:example.com/m/a declares type:example.com/m/a.T",
		"type:example.com/m/a.T declares method:example.com/m/a.T.M")
	hashB = viewHash(
		"func:example.com/m/b.G calls func:example.com/m/a.F",
		"pkg:example.com/m/b declares func:example.com/m/b.G")
)

func impactOf(t *testing.T, root, logPath string, paths ...string) []Family {
	t.Helper()
	fams, err := Impact(context.Background(), root, logPath, paths)
	if err != nil {
		t.Fatalf("Impact(%v): %v", paths, err)
	}
	return fams
}

func sameFamilies(a, b []Family) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestPL8a_ImpactNamesTheFamiliesOfAChangedPath: a written file names its
// package's family, a test file its package's too, one family per package
// however many of its files were written, sorted by package; each Hash is the
// view lines of the package and the symbols it declares, a retracted fact
// gone.
func TestPL8a_ImpactNamesTheFamiliesOfAChangedPath(t *testing.T) {
	root, lp := project(t, fixtureLog(t))
	if got, want := impactOf(t, root, lp, "a/a.go", "a/a_test.go"), []Family{{pkgA, hashA}}; !sameFamilies(got, want) {
		t.Errorf("a/a.go and a/a_test.go: %+v, want %+v", got, want)
	}
	if got, want := impactOf(t, root, lp, "b/b.go", "a/a.go"), []Family{{pkgA, hashA}, {pkgB, hashB}}; !sameFamilies(got, want) {
		t.Errorf("b/b.go and a/a.go: %+v, want %+v", got, want)
	}
	if got, want := impactOf(t, root, lp, filepath.Join(root, "b", "b.go")), []Family{{pkgB, hashB}}; !sameFamilies(got, want) {
		t.Errorf("an absolute path under root: %+v, want %+v", got, want)
	}
}

// TestPL8b_UnchangedPathHasNoFamily: a package none of whose files was
// written has no family, nor does a path whose package the log does not hold,
// a path under testdata, a path outside root, or a file at the module root
// when the log holds no root package.
func TestPL8b_UnchangedPathHasNoFamily(t *testing.T) {
	root, lp := project(t, fixtureLog(t))
	if got, want := impactOf(t, root, lp, "b/b.go"), []Family{{pkgB, hashB}}; !sameFamilies(got, want) {
		t.Errorf("b/b.go alone: %+v, want b's family alone", got)
	}
	for _, p := range []string{"c/c.go", "a/testdata/x.go", "../outside/a/a.go", "README.md"} {
		if got := impactOf(t, root, lp, p); got == nil || len(got) != 0 {
			t.Errorf("%s: %+v, want no family (and an empty, not nil, list)", p, got)
		}
	}
}

// synthLog is a valid v0 log of the fixture's records and n more: package
// pI declaring five functions, each calling a.F, ten records a package.
func synthLog(t testing.TB, n int) []byte {
	t.Helper()
	var b strings.Builder
	b.Write(fixtureLog(t))
	for i := 0; i < n; i++ {
		pkg, fn := fmt.Sprintf("example.com/m/p%d", i/10), fmt.Sprintf("F%d", i%5)
		if i%10 < 5 {
			fmt.Fprintf(&b, "+\tpkg:%s\tdeclares\tfunc:%s.%s\tPROVEN\t%s\tc1\tgotypes@v2\n", pkg, pkg, fn, cfg)
		} else {
			fmt.Fprintf(&b, "+\tfunc:%s.%s\tcalls\tfunc:example.com/m/a.F\tPROVEN\t%s\tc1\tgotypes@v2\n", pkg, fn, cfg)
		}
	}
	return []byte(b.String())
}

// stall makes every record cost a millisecond, so a 5,000-record decode takes
// seconds against a 20 ms Deadline; the hook does not look at any context, so
// only the decoder's own looks can stop it.
func stall(t *testing.T) {
	t.Helper()
	recordHook = func() { time.Sleep(time.Millisecond) }
	t.Cleanup(func() { recordHook = nil })
}

// hang makes the first record wait two seconds, or until the test ends, so a
// decode past Deadline is one only the caller's deadline can cut short: an
// Obligations that waits for its worker returns after two seconds.
func hang(t *testing.T) {
	t.Helper()
	release := make(chan struct{})
	timer := time.AfterFunc(2*time.Second, func() { close(release) })
	recordHook = func() { <-release }
	t.Cleanup(func() {
		recordHook = nil
		if timer.Stop() {
			close(release)
		}
	})
}

// TestPL8c_DeadlineYieldsUnknownNotPartial: a decode held past Deadline is
// one unknown platform_impact row and no family, returned at the deadline,
// not when the decode would have finished.
func TestPL8c_DeadlineYieldsUnknownNotPartial(t *testing.T) {
	root, lp := project(t, synthLog(t, 5000))
	hang(t)
	start := time.Now()
	rows := Obligations(RunView{Root: root, WrittenPaths: []string{"a/a.go"}, LogPath: lp})
	elapsed := time.Since(start)
	want := verify.Obligation{ID: verify.ObligationID(verify.ScopeSession, verify.PlatformImpact, ""), Scope: verify.ScopeSession,
		Kind: verify.PlatformImpact, Source: SourcePlatform, State: verify.ObligationUnknown}
	if len(rows) != 1 || rows[0].ID != want.ID || rows[0].State != want.State || rows[0].Kind != want.Kind || len(rows[0].Evidence) != 0 {
		t.Errorf("a stalled decode: %+v, want the one unknown row %+v", rows, want)
	}
	if elapsed > time.Second {
		t.Errorf("Obligations returned after %v; Deadline is %v", elapsed, Deadline)
	}
}

// TestPL8c2_DeadlineCancelsTheWork: each timed-out call's worker sees its
// context done and exits; after fifty timeouts as many goroutines run as
// before.
func TestPL8c2_DeadlineCancelsTheWork(t *testing.T) {
	root, lp := project(t, synthLog(t, 5000))
	stall(t)
	before := runtime.NumGoroutine()
	for i := 0; i < 50; i++ {
		if rows := Obligations(RunView{Root: root, WrittenPaths: []string{"a/a.go"}, LogPath: lp}); len(rows) != 1 || rows[0].State != verify.ObligationUnknown {
			t.Fatalf("timeout %d: %+v, want one unknown row", i, rows)
		}
	}
	deadline := time.Now().Add(3 * time.Second)
	for runtime.NumGoroutine() > before && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if n := runtime.NumGoroutine(); n > before {
		t.Errorf("%d goroutines run after fifty timeouts and 3 s, %d before: the workers did not stop", n, before)
	}
}

// TestPL8d_ObligationsCarryPlatformImpactKindOnly: each family is one open
// platform_impact row in the session scope, source platform, no evidence (an
// empty list, never null), its id the verify ledger's over the package and
// its Hash; no writes, or no log, owe no row.
func TestPL8d_ObligationsCarryPlatformImpactKindOnly(t *testing.T) {
	root, lp := project(t, fixtureLog(t))
	// The shape is under test, not the deadline (TestPL8c*): a minute, so a
	// loaded runner cannot turn the answer into the timeout's unknown row.
	rows := obligationsWithin(RunView{Root: root, WrittenPaths: []string{"b/b.go", "a/a.go"}, LogPath: lp}, time.Minute)
	want := []string{pkgA + "@" + hashA, pkgB + "@" + hashB}
	if len(rows) != len(want) {
		t.Fatalf("rows %+v, want %d", rows, len(want))
	}
	for i, r := range rows {
		if r.Kind != verify.PlatformImpact || r.Scope != verify.ScopeSession || r.Source != SourcePlatform || r.State != verify.ObligationOpen ||
			r.Evidence == nil || len(r.Evidence) != 0 || r.ID != verify.ObligationID(verify.ScopeSession, verify.PlatformImpact, want[i]) {
			t.Errorf("row %d is %+v, want an open platform_impact row for %s", i, r, want[i])
		}
	}
	for _, run := range []RunView{{Root: root, LogPath: lp}, {Root: root, WrittenPaths: []string{"a/a.go"}}} {
		if rows := Obligations(run); rows == nil || len(rows) != 0 {
			t.Errorf("Obligations(%+v) = %+v, want none (an empty list)", run, rows)
		}
	}
}

// TestPL8i_PackageOfFollowsGosRule (D132): a file's package is the nearest
// go.mod's module path joined with its directory relative to that go.mod; a
// nested module is its own, never the outer's (its path here is not the
// outer's join, so the two readings differ); testdata and dot or underscore
// directories hold no package; a go.mod's comments are skipped and a quoted
// path read; no go.mod, or a nearest one without a module line, is no
// package.
func TestPL8i_PackageOfFollowsGosRule(t *testing.T) {
	root := t.TempDir()
	write := func(rel, s string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "// the outer module\n\nmodule example.com/m // trailing\n\ngo 1.25\n")
	write("pgraph/go.mod", "module example.com/pgraph\n")
	write("quoted/go.mod", "module \"example.com/q\"\n")
	write("nomod/go.mod", "go 1.25\n")
	for _, c := range []struct{ path, want string }{
		{"main.go", "example.com/m"},
		{"a/a.go", "example.com/m/a"},
		{"a/a_test.go", "example.com/m/a"},
		{"a/b/c.go", "example.com/m/a/b"},
		{filepath.Join(root, "a", "a.go"), "example.com/m/a"},
		{"pgraph/codec.go", "example.com/pgraph"},
		{"pgraph/x/y.go", "example.com/pgraph/x"},
		{"quoted/q.go", "example.com/q"},
		{"nomod/n.go", ""},
		{"nomod/sub/n.go", ""},
		{"a/testdata/fix.go", ""},
		{"pgraph/testdata/mod/a/a.go", ""},
		{".github/w.go", ""},
		{"_tools/t.go", ""},
		{"../outside.go", ""},
		{filepath.Join(filepath.Dir(root), "x.go"), ""},
	} {
		if got := packageOf(root, c.path, map[string]goMod{}); got != c.want {
			t.Errorf("packageOf(%q) = %q, want %q", c.path, got, c.want)
		}
	}
	if got := packageOf(t.TempDir(), "a/a.go", map[string]goMod{}); got != "" {
		t.Errorf("a root with no go.mod: %q, want no package", got)
	}
}

// TestPL8j_FixtureIsPgraphs: testdata/facts.log is byte for byte pgraph's
// pinned fixture, the other half of TestPL8h_FixturePinned: a codec change
// that rewrites pgraph's copy fails here until this one follows.
func TestPL8j_FixtureIsPgraphs(t *testing.T) {
	theirs, err := os.ReadFile(filepath.Join("..", "..", "pgraph", "testdata", "platform", "facts.log"))
	if err != nil {
		t.Fatal(err)
	}
	if ours := fixtureLog(t); string(ours) != string(theirs) {
		t.Errorf("testdata/facts.log differs from pgraph/testdata/platform/facts.log:\nours   %q\ntheirs %q", ours, theirs)
	}
}

// BenchmarkImpact is one written file's family read from a log of 1,000 and
// of 50,000 records, the decode and the index included. The names are
// literals so the perf gate reads them without running the benchmark
// (test/perf/gates_test.go).
func BenchmarkImpact(b *testing.B) {
	for _, c := range []struct {
		name string
		n    int
	}{{"records_1000", 1000}, {"records_50000", 50000}} {
		b.Run(c.name, func(b *testing.B) {
			root, lp := project(b, synthLog(b, c.n))
			paths := []string{"a/a.go"}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if _, err := Impact(context.Background(), root, lp, paths); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
