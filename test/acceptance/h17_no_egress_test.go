package acceptance

import (
	"os/exec"
	"strings"
	"testing"
)

// TestH17_NoNetworkInTheDependencyGraph is the structural half of H-17.
//
// Asserting at runtime that zero non-loopback sockets were opened requires
// watching syscalls. Asserting that the binary cannot open one is both stronger
// and cheaper: a program whose transitive imports contain no networking package
// has no code path to a socket, whatever its configuration says.
//
// os/exec is listed for the same reason. A recorder that can spawn a process
// can reach the network through one, and it has no need to spawn anything.
func TestH17_NoNetworkInTheDependencyGraph(t *testing.T) {
	forbidden := map[string]string{
		"net":                "opens sockets",
		"net/http":           "opens sockets",
		"crypto/tls":         "implies a network peer",
		"os/exec":            "can reach the network through a child process",
		"net/http/httptrace": "implies a network client",
	}

	cmd := exec.Command("go", "list", "-deps", "./cmd/attest")
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
