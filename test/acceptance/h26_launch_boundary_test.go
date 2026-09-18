package acceptance

// H-26 — the launcher's imports stay out of the recorder.
//
// `rashomon run` has to spawn a process and install a signal handler. Both are
// forbidden in the recorder's dependency graph: code that executes inside an
// agent's tool calls, thousands of times a session, must not be able to spawn a
// child, because a child is a path to the network.
//
// So the launcher lives in its own package and these two tests bound it. The
// first asserts the recorder still cannot reach it. The second asserts that
// os/exec enters the binary only from the launcher and net only from the SQLite
// driver -- naming both sanctioned sources, so a third one added later fails
// rather than hiding behind the exception the first two opened.

import (
	"strings"
	"testing"
)

const (
	recorderRoot = "./internal/hook"
	launcherPkg  = "github.com/altrace-dev-role/rashomon/internal/launch"
	driverPkg    = "modernc.org/sqlite"
)

// TestH26_RecorderCannotReachTheLauncher is the import-direction assertion.
//
// Without it, "the recorder has no exec" holds only until somebody imports the
// launcher from a hook path for a reason that looks good at the time. The
// dependency graph is the only thing that can state the rule in a way a future
// change has to argue with.
func TestH26_RecorderCannotReachTheLauncher(t *testing.T) {
	for _, pkg := range deps(t, recorderRoot) {
		if pkg == launcherPkg {
			t.Errorf("%s reaches %s. The launcher imports os/exec and installs a signal "+
				"handler; neither belongs on a path that runs inside a tool call.",
				recorderRoot, launcherPkg)
		}
	}
}

// TestH26_ForbiddenImportsComeOnlyFromTheirSanctionedSource names both
// exceptions explicitly.
//
// A test that only said "no NEW forbidden imports" would pass if the driver
// were swapped for something worse, or if a second package quietly acquired
// os/exec. Naming the source for each means the exception cannot widen without
// this failing.
func TestH26_ForbiddenImportsComeOnlyFromTheirSanctionedSource(t *testing.T) {
	binary := map[string]bool{}
	for _, pkg := range deps(t, "./cmd/rashomon") {
		binary[pkg] = true
	}

	// Premise: both sanctioned sources are actually linked. If either is gone,
	// the corresponding ban should be restored over the whole binary rather
	// than this narrower test being kept.
	if !binary[launcherPkg] {
		t.Fatalf("%s is not in the binary; if the launcher was removed, restore the "+
			"whole-binary ban on os/exec instead of keeping this test", launcherPkg)
	}
	if !binary["modernc.org/libc"] {
		t.Fatal("the SQLite driver is not in the binary; if it was removed, restore the " +
			"whole-binary ban on net instead of keeping this test")
	}

	sanctioned := map[string]string{
		"os/exec": launcherPkg,
		"net":     driverPkg,
	}
	for forbidden, source := range sanctioned {
		if !binary[forbidden] {
			continue
		}
		var fromSource bool
		for _, pkg := range deps(t, source) {
			if pkg == forbidden {
				fromSource = true
				break
			}
		}
		if !fromSource {
			t.Errorf("the binary imports %s and its sanctioned source %s does NOT, so "+
				"something else introduced it", forbidden, source)
		}
	}

	// And nothing else forbidden crept in under cover of those two.
	for pkg, why := range forbiddenImports {
		if !binary[pkg] {
			continue
		}
		if _, ok := sanctioned[pkg]; ok {
			continue
		}
		var explained bool
		for _, source := range []string{launcherPkg, driverPkg} {
			for _, dep := range deps(t, source) {
				if dep == pkg {
					explained = true
					break
				}
			}
		}
		if !explained {
			t.Errorf("the binary imports %s (which %s) and neither sanctioned source "+
				"explains it", pkg, why)
		}
	}
}

// TestH26_LauncherDoesNotReachTheReportOrTheStoreWriters keeps the blast radius
// of the exec package small: it launches a command and forwards signals, and
// has no business holding a store writer or a renderer.
func TestH26_LauncherDoesNotReachTheReportOrTheStoreWriters(t *testing.T) {
	for _, pkg := range deps(t, launcherPkg) {
		if strings.HasSuffix(pkg, "/internal/report") || strings.HasSuffix(pkg, "/internal/wire") {
			t.Errorf("the launcher reaches %s; it spawns a command and forwards signals, "+
				"and pulling the renderer in with it widens what an exec-capable "+
				"package can touch", pkg)
		}
	}
}
