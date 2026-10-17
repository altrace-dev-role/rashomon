package acceptance

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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
// H-17, scoped to the RECORDER. TRU-40 makes the same claim of the whole
// binary now that nothing in it links a networking package; this one stays
// because it is the claim that matters even if the binary's ever changes.
//
// internal/hook is the right root. Every hook entry point lives in it --
// PreToolUse, PostToolUse, PostToolUseFailure and both probe phases -- so its
// transitive graph IS the set of code that can execute inside a tool call.
// That is the code whose freedom from sockets is a guarantee to the user: the
// recorder runs in the agent's own lifecycle, thousands of times a session,
// and must be unable to phone anywhere.
//
// The runtime halves below are stronger evidence still, but they are
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
// difference between a bound and a formality: transitive provenance excuses
// everything downstream of an import, whoever actually reached for it. What a
// reviewer wants to know is whether anyone in this module reached for one of
// these, and that is a question about import statements. It is also the
// question that would catch the temptation internal/baseline documents
// resisting, where walking up for .git replaced shelling out to
// `git rev-parse`.
//
// A list per package, because the specification plans two importers of
// os/exec: the supervisor, which starts this binary's own child for every
// hook entry and is the only one so far, and a notifier still to come.
// Nothing else will take an entry.
var sanctionedSource = map[string][]string{
	"os/exec": {rashomonModule + "/internal/supervise"},
}

// rashomonModule is this module's import path.
const rashomonModule = "github.com/altrace-dev-role/rashomon"

// moduleImports lists every package of this module with its own import
// statements: the non-test imports, which are what a binary can link.
func moduleImports(t *testing.T) map[string][]string {
	t.Helper()
	cmd := exec.Command("go", "list", "-f", "{{.ImportPath}} {{join .Imports \",\"}}", "./...")
	cmd.Dir = moduleRoot
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("listing direct imports: %v", err)
	}
	listing := map[string][]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || !strings.HasPrefix(fields[0], rashomonModule) {
			continue
		}
		var imports []string
		if len(fields) > 1 {
			imports = strings.Split(fields[1], ",")
		}
		listing[fields[0]] = imports
	}
	return listing
}

// isTestTree reports a package under the module's test/ tree.
func isTestTree(pkg string) bool {
	return strings.HasPrefix(pkg, rashomonModule+"/test/")
}

// shippedScope is the set of packages a shipped binary can link, the scope
// H-17 and TRU-40 hold (spec run 72): every package outside test/, plus any
// test/ package that a package outside test/ imports. A test/ package
// imported only from _test.go files is measurement code that no binary links,
// and TestH17_TestPackagesStayTestOnly keeps it that way.
func shippedScope(listing map[string][]string) map[string]bool {
	scope := map[string]bool{}
	for pkg, imports := range listing {
		if isTestTree(pkg) {
			continue
		}
		scope[pkg] = true
		for _, imp := range imports {
			if isTestTree(imp) {
				scope[imp] = true
			}
		}
	}
	return scope
}

// testOnlyViolations lists every import of a test/ package by a package
// outside test/, sorted: each one widens what a binary can link into code
// written as test-only.
func testOnlyViolations(listing map[string][]string) []string {
	var out []string
	for pkg, imports := range listing {
		if isTestTree(pkg) {
			continue
		}
		for _, imp := range imports {
			if isTestTree(imp) {
				out = append(out, pkg+" imports "+imp)
			}
		}
	}
	slices.Sort(out)
	return out
}

// TestH17_NoPackageOfOursReachesForANetworkImport is the provenance assertion
// the prose has been making all along, over the packages a shipped binary can
// link (shippedScope).
//
// Every forbidden package is either imported by nobody in that scope, or by
// exactly the one package sanctioned to.
func TestH17_NoPackageOfOursReachesForANetworkImport(t *testing.T) {
	listing := moduleImports(t)
	scope := shippedScope(listing)
	packages := 0
	for pkg, imports := range listing {
		if !scope[pkg] {
			continue
		}
		packages++
		for _, imp := range imports {
			why, forbidden := forbiddenImports[imp]
			if !forbidden {
				continue
			}
			sources, sanctioned := sanctionedSource[imp]
			if slices.Contains(sources, pkg) {
				continue
			}
			allowed := "Nothing in this module may"
			if sanctioned {
				allowed = "Only " + strings.Join(sources, " and ") + " may, each a separate " +
					"package for exactly this reason"
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
// two ways it would go quiet: an entry naming a package that no longer imports
// the thing it is excused for excuses nothing, and the next reader would read
// it as a live bound; and a forbidden package nothing sanctions, which must
// then be absent from the whole binary on every release target, not only
// from this module's own import statements, or a dependency could bring it
// in with nobody here having written the import. A sanctioned package is
// held to its importers on every target by TRU-40 instead.
func TestH17_TheSanctionedSourcesAreNotVacuous(t *testing.T) {
	for forbidden, sources := range sanctionedSource {
		if len(sources) == 0 {
			t.Errorf("%s has an entry naming no importer; remove it", forbidden)
		}
		for _, source := range sources {
			imports := directImports(t, source)
			if !imports[forbidden] {
				t.Errorf("%s is sanctioned to import %s and no longer does. Remove the entry "+
					"and let the whole-module ban cover it, rather than leaving an exception "+
					"that reads as though something still needs excusing.", source, forbidden)
			}
		}
	}
	binary := map[string]bool{}
	for _, pkg := range releaseDeps(t) {
		binary[pkg] = true
	}
	for pkg, why := range forbiddenImports {
		if binary[pkg] && len(sanctionedSource[pkg]) == 0 {
			t.Errorf("the binary depends on %s, which %s, and nothing sanctions it", pkg, why)
		}
	}
}

// rawSyscalls are the calls that reach a socket or start a process WITHOUT
// going through one of forbiddenImports' packages, so the import-based scan
// above cannot see them at all: syscall and os are themselves legitimately
// imported here (internal/store's file locks, internal/hook's signal
// handling), and banning either import would ban code this module needs.
// This is a scan for the specific calls, not the packages.
var rawSyscalls = []string{
	"syscall.Socket(", "syscall.Connect(", "syscall.ForkExec(", "syscall.Exec(",
	"os.StartProcess(",
}

// TestH17_NoSourceCallsASocketOrStartsAProcessOutsideASyscallImport backs the
// README's "no network code of its own" over the one path the import scan
// cannot cover. Every *.go file in this module, not only the recorder's: the
// README's claim is about the whole binary, and TestH17_HandlerOpensNoSockets
// above already covers one process at runtime, which is narrower.
func TestH17_NoSourceCallsASocketOrStartsAProcessOutsideASyscallImport(t *testing.T) {
	var files int
	err := filepath.WalkDir(moduleRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		files++
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(moduleRoot, path)
		if err != nil {
			rel = path
		}
		for _, call := range rawSyscalls {
			if strings.Contains(string(body), call) {
				t.Errorf("%s calls %s, which reaches a socket or starts a process "+
					"without going through an import the scan above would catch", rel, call)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// The failure mode of every source-scanning check is to scan nothing.
	if files < 50 {
		t.Fatalf("scanned only %d non-test .go files; this test is not looking at the "+
			"module any more", files)
	}
}

// TestRawSyscalls_CatchesEachListedCall pins rawSyscalls to its own list,
// independent of whatever this tree's source happens to contain today: a
// call dropped from the list, or a pattern narrowed so a real call site no
// longer matches it, is caught here even on a tree with no such call
// anywhere in it.
func TestRawSyscalls_CatchesEachListedCall(t *testing.T) {
	fixtures := []string{
		"	syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0)",
		"	syscall.Connect(fd, addr)",
		"	syscall.ForkExec(path, args, attr)",
		"	syscall.Exec(path, args, env)",
		"	os.StartProcess(path, args, attr)",
	}
	for _, src := range fixtures {
		var caught bool
		for _, call := range rawSyscalls {
			if strings.Contains(src, call) {
				caught = true
			}
		}
		if !caught {
			t.Errorf("rawSyscalls catches none of its own list against %q", src)
		}
	}
	// A benign, legitimate use of both packages -- exactly what store's file
	// locks and hook's signal handling do -- must not match any entry.
	safe := "	syscall.Flock(fd, syscall.LOCK_EX)\n\tos.Getpid()"
	for _, call := range rawSyscalls {
		if strings.Contains(safe, call) {
			t.Errorf("rawSyscalls entry %q matches legitimate syscall/os usage", call)
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
