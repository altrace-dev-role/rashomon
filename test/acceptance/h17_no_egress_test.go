package acceptance

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// forbiddenImports are the packages that give code a path to a socket.
//
// os/exec is listed for the same reason as the networking packages: code that
// can spawn a process can reach the network through one.
var forbiddenImports = map[string]string{
	"net":                "opens sockets",
	"net/http":           "opens sockets",
	"crypto/tls":         "implies a network peer",
	"os/exec":            "can reach the network through a child process",
	"net/http/httptrace": "implies a network client",
}

// deps lists the transitive imports of a package pattern.
func deps(t *testing.T, pattern string) []string {
	t.Helper()
	cmd := exec.Command("go", "list", "-deps", pattern)
	cmd.Dir = moduleRoot
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("listing dependencies of %s: %v", pattern, err)
	}
	var pkgs []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if p := strings.TrimSpace(line); p != "" {
			pkgs = append(pkgs, p)
		}
	}
	return pkgs
}

// TestH17_NoNetworkInTheRecordersDependencyGraph is the structural half of
// H-17, and it is scoped to the RECORDER rather than to the whole binary.
//
// It used to cover ./cmd/rashomon, and that assertion cannot survive the
// destinations reader: the report needs to read the proxy's SQLite store, the
// only pure-Go driver transitively imports net and os/exec through
// modernc.org/libc, and no arrangement of this program makes both true. So the
// claim is narrowed to the one that matters and the narrowing is made explicit
// rather than left as a deleted test.
//
// internal/hook is the right root. Every hook entry point lives in it --
// PreToolUse, PostToolUse, PostToolUseFailure and both probe phases -- so its
// transitive graph IS the set of code that can execute inside a tool call.
// That is the code whose freedom from sockets is a guarantee to the user: the
// recorder runs in the agent's own lifecycle, thousands of times a session,
// and must be unable to phone anywhere.
//
// The honest public sentence is now: "the recorder's import graph contains no
// networking package, checked on every build; the operator-invoked report
// links a SQLite driver whose dependency graph contains net, and opens no
// socket." The runtime halves below are stronger evidence still, but they are
// Linux-only and SKIP on macOS, so they cannot carry the claim.
func TestH17_NoNetworkInTheRecordersDependencyGraph(t *testing.T) {
	pkgs := deps(t, "./internal/hook")

	var recorderPkgs int
	for _, pkg := range pkgs {
		if why, bad := forbiddenImports[pkg]; bad {
			t.Errorf("the recorder depends on %s, which %s", pkg, why)
		}
		if strings.HasPrefix(pkg, "github.com/altrace-dev-role/rashomon/") {
			recorderPkgs++
		}
	}
	// Guard the premise twice. An empty listing would pass the loop silently,
	// and a listing that had stopped reaching this module's own packages would
	// mean the root is no longer the recorder.
	if len(pkgs) < 10 {
		t.Fatalf("dependency listing returned only %d packages; the command did not run as expected", len(pkgs))
	}
	if recorderPkgs < 6 {
		t.Fatalf("the recorder's graph contains only %d of this module's packages; "+
			"internal/hook no longer reaches the store, shape, settings, safe, fault "+
			"and host packages, so this test is guarding less than it claims", recorderPkgs)
	}
}

// TestH17_NetworkEntersOnlyThroughTheSQLiteDriver bounds the exception the test
// above carves out.
//
// Without this, "the recorder is clean" would be compatible with anything at
// all being added to the report path. It asserts the opposite direction: net
// and os/exec ARE in the binary, they are expected, and the only reason they
// are there is the SQLite driver's subtree. If they ever enter from somewhere
// else -- an HTTP health check, a telemetry client, a shell-out -- the driver
// stops being the explanation and this fails.
//
// It is deliberately a two-sided assertion. A test that only checked "no new
// forbidden imports" would pass if the driver were removed and something worse
// added in its place.
func TestH17_NetworkEntersOnlyThroughTheSQLiteDriver(t *testing.T) {
	binary := deps(t, "./cmd/rashomon")

	inBinary := map[string]bool{}
	for _, pkg := range binary {
		inBinary[pkg] = true
	}

	// While the driver is NOT linked into the binary, the original and stronger
	// assertion still holds and is the one to make: no forbidden import
	// anywhere in the program. This is not a fallback for convenience -- it
	// means the test ratchets. The exception only opens when the driver is
	// actually there, and closes again by itself if it is ever dropped.
	if !inBinary["modernc.org/libc"] {
		for pkg, why := range forbiddenImports {
			if inBinary[pkg] {
				t.Errorf("the binary depends on %s, which %s, and the SQLite driver is "+
					"not linked, so nothing sanctions it", pkg, why)
			}
		}
		return
	}

	// Everything the driver's subtree brings in, by itself.
	driver := map[string]bool{}
	for _, pkg := range deps(t, "modernc.org/sqlite") {
		driver[pkg] = true
	}

	for pkg, why := range forbiddenImports {
		if !inBinary[pkg] {
			continue
		}
		if !driver[pkg] {
			t.Errorf("the binary depends on %s (which %s) and the SQLite driver does "+
				"NOT, so something else introduced it. The driver is the only "+
				"sanctioned source of a networking import in this program.", pkg, why)
		}
	}
}

// TestH17_CaptureIsIdenticalWithNetworkDenied is the runtime half: a full
// capture cycle inside a network namespace with no interfaces behaves exactly
// as one outside it.
func TestH17_CaptureIsIdenticalWithNetworkDenied(t *testing.T) {
	if _, err := exec.LookPath("unshare"); err != nil {
		t.Skip("unshare is not available; the network-denied half of H-17 cannot run here")
	}
	if out, err := exec.Command("unshare", "-n", "true").CombinedOutput(); err != nil {
		t.Skipf("unshare -n is not permitted here (%v: %s); the network-denied half of H-17 cannot run", err, out)
	}

	e := newEnv(t)
	e.watched(testSession)

	connected := defaultPayload()
	connected.ToolUseID = "toolu_connected"
	e.mustHook(connected.build(t))

	denied := defaultPayload()
	denied.ToolUseID = "toolu_denied"
	cmd := exec.Command("unshare", "-n", rashomonBin, "hook")
	cmd.Stdin = strings.NewReader(denied.build(t))
	cmd.Env = e.environ()
	cmd.Dir = e.cwd
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	_ = cmd.Run()
	if code := cmd.ProcessState.ExitCode(); code != 0 {
		t.Fatalf("with network denied the hook exited %d, want 0 (stderr %q)", code, stderr.String())
	}

	decls := e.declarations(testSession)
	if len(decls) != 2 {
		t.Fatalf("got %d declarations, want 2", len(decls))
	}
	a, b := decls[0], decls[1]
	// Everything except the identifiers and the clock must match exactly.
	for _, key := range []string{"tool_use_id", "seq", "recorded_at_unix_ms"} {
		delete(a.fields, key)
		delete(b.fields, key)
	}
	aj, _ := jsonCanonical(a.fields)
	bj, _ := jsonCanonical(b.fields)
	if !bytes.Equal(aj, bj) {
		t.Errorf("capture differs with network denied:\n  connected: %s\n  denied:    %s", aj, bj)
	}
	assertCallCoverage(t, e, testSession, "verified", "")
}

// TestH17_HandlerOpensNoSockets traces the handler's syscalls and asserts that
// no socket is created at all -- not merely no non-loopback one, since the
// program has no reason to open any.
func TestH17_HandlerOpensNoSockets(t *testing.T) {
	if _, err := exec.LookPath("strace"); err != nil {
		t.Skip("strace is not available; the socket half of H-17 cannot run here")
	}
	if out, err := exec.Command("strace", "-e", "trace=none", "true").CombinedOutput(); err != nil {
		t.Skipf("strace cannot trace here (%v: %s); the socket half of H-17 cannot run", err, out)
	}

	e := newEnv(t)
	e.watched(testSession)
	trace := filepath.Join(t.TempDir(), "trace")

	cmd := exec.Command("strace", "-f", "-e", "trace=socket,connect,sendto,sendmsg", "-o", trace, rashomonBin, "hook")
	cmd.Stdin = strings.NewReader(defaultPayload().build(t))
	cmd.Env = e.environ()
	cmd.Dir = e.cwd
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("strace run failed: %v: %s", err, out)
	}

	body, err := os.ReadFile(trace)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(body), "\n") {
		if strings.Contains(line, "socket(") || strings.Contains(line, "connect(") {
			t.Errorf("handler made a network syscall: %s", line)
		}
	}
	// Guard the premise: strace must have seen the process at all.
	if !strings.Contains(string(body), "+++ exited") {
		t.Fatalf("trace does not show the process exiting; strace did not attach:\n%s", body)
	}
	if got := e.declarations(testSession); len(got) != 1 {
		t.Errorf("traced run recorded %d declarations, want 1", len(got))
	}
}

// sanctionedSource names, per forbidden package, the ONLY of this module's own
// packages allowed to import it DIRECTLY.
//
// Direct imports, not transitive ones, and that distinction is the whole
// difference between a bound and a formality. The membership test above asks
// whether the forbidden package is in the SQLite driver's dependency set -- and
// the driver's set contains net AND os/exec, so its predicate is satisfied
// whoever actually reached for them. Transitive provenance is no better: every
// package that touches the store inherits the driver's whole subtree, so
// internal/report and internal/wire "have an explanation" for os/exec without
// anybody having written a subprocess.
//
// What a reviewer actually wants to know is whether anyone in this module
// reached for one of these, and that is a question about import statements. It
// is also the question that would catch the temptation internal/baseline
// documents resisting, where walking up for .git replaced shelling out to
// `git rev-parse`.
var sanctionedSource = map[string]string{
	"os/exec": "github.com/altrace-dev-role/rashomon/internal/launch",
}

// TestH17_NoPackageOfOursReachesForANetworkImport is the provenance assertion
// the prose has been making all along.
//
// Every forbidden package is either imported by nobody here, or by exactly the
// one package sanctioned to. Nothing is said about the driver's own subtree:
// that code is not ours, it is the reason the exception exists, and the test
// above is what keeps the exception tied to the driver actually being present.
func TestH17_NoPackageOfOursReachesForANetworkImport(t *testing.T) {
	const modulePrefix = "github.com/altrace-dev-role/rashomon"

	cmd := exec.Command("go", "list", "-f", "{{.ImportPath}} {{join .Imports \",\"}}", "./...")
	cmd.Dir = moduleRoot
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("listing direct imports: %v", err)
	}

	packages := 0
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		pkg := fields[0]
		if !strings.HasPrefix(pkg, modulePrefix) {
			continue
		}
		packages++
		if len(fields) < 2 {
			continue
		}
		for _, imp := range strings.Split(fields[1], ",") {
			why, forbidden := forbiddenImports[imp]
			if !forbidden {
				continue
			}
			source, sanctioned := sanctionedSource[imp]
			if sanctioned && source == pkg {
				continue
			}
			allowed := "Nothing in this module may"
			if sanctioned {
				allowed = "Only " + source + " may, and it is a separate package for " +
					"exactly this reason"
			}
			t.Errorf("%s imports %s directly, which %s. %s: code that runs inside every "+
				"tool call must not be able to acquire a socket or a child process.",
				pkg, imp, why, allowed)
		}
	}

	// The failure mode of every source-scanning check is to scan nothing.
	if packages < 8 {
		t.Fatalf("inspected only %d packages of our own; this test is not looking at "+
			"the module any more", packages)
	}
}

// TestH17_TheSanctionedSourcesAreNotVacuous guards the map above against the
// way it would go quiet: an entry naming a package that no longer imports the
// thing it is excused for excuses nothing, and the next reader would read it as
// a live bound.
func TestH17_TheSanctionedSourcesAreNotVacuous(t *testing.T) {
	for forbidden, source := range sanctionedSource {
		imports := directImports(t, source)
		if !imports[forbidden] {
			t.Errorf("%s is sanctioned to import %s and no longer does. Remove the entry "+
				"and let the whole-module ban cover it, rather than leaving an exception "+
				"that reads as though something still needs excusing.", source, forbidden)
		}
	}
}

// directImports returns one package's own import statements.
func directImports(t *testing.T, pattern string) map[string]bool {
	t.Helper()
	cmd := exec.Command("go", "list", "-f", "{{join .Imports \",\"}}", pattern)
	cmd.Dir = moduleRoot
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("listing direct imports of %s: %v", pattern, err)
	}
	set := map[string]bool{}
	for _, imp := range strings.Split(strings.TrimSpace(string(out)), ",") {
		if p := strings.TrimSpace(imp); p != "" {
			set[p] = true
		}
	}
	return set
}
