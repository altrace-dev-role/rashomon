package acceptance

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestH17_NoNetworkInTheDependencyGraph is the structural half of H-17.
//
// A program whose transitive imports contain no networking package has no code
// path to a socket, whatever its configuration says. os/exec is listed for the
// same reason: a recorder that can spawn a process can reach the network
// through one, and it has no need to spawn anything.
func TestH17_NoNetworkInTheDependencyGraph(t *testing.T) {
	forbidden := map[string]string{
		"net":                "opens sockets",
		"net/http":           "opens sockets",
		"crypto/tls":         "implies a network peer",
		"os/exec":            "can reach the network through a child process",
		"net/http/httptrace": "implies a network client",
	}

	cmd := exec.Command("go", "list", "-deps", "./cmd/rashomon")
	cmd.Dir = moduleRoot
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("listing dependencies: %v", err)
	}

	var deps int
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		pkg := strings.TrimSpace(line)
		if pkg == "" {
			continue
		}
		deps++
		if why, bad := forbidden[pkg]; bad {
			t.Errorf("the binary depends on %s, which %s", pkg, why)
		}
	}
	// Guard the premise: an empty listing would pass the loop above silently.
	if deps < 10 {
		t.Fatalf("dependency listing returned only %d packages; the command did not run as expected", deps)
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
