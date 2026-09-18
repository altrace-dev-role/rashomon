package acceptance

// H-22 — `rashomon env` prints the variables that put the proxy on the path.
//
// It prints and does not export, because a command that modified the caller's
// environment would have to be a shell function rather than a binary. The
// operator runs `eval $(rashomon env)`, which is why every line has to be a
// valid shell assignment and nothing else may go to stdout.

import (
	"strings"
	"testing"
)

const (
	observePort   = "18080"
	wantNoProxy   = "localhost,127.0.0.1,::1,0.0.0.0,*.local"
	wantHTTPSUp   = "export HTTPS_PROXY=http://127.0.0.1:" + observePort
	wantHTTPSDown = "export https_proxy=http://127.0.0.1:" + observePort
)

func TestH22_EnvPrintsTheHTTPSVariablesOnly(t *testing.T) {
	e := newEnv(t)
	res := e.run("", nil, "env")

	if res.exitCode != 0 {
		t.Fatalf("env: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	for _, want := range []string{wantHTTPSUp, wantHTTPSDown, "export NO_PROXY=" + wantNoProxy} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("env does not print %q:\n%s", want, res.stdout)
		}
	}

	// Plain HTTP is not observed in this release. Printing HTTP_PROXY would
	// route traffic through a proxy that does not record it and then report
	// nothing -- silence read as zero, which is the one thing the coverage
	// rules forbid.
	if strings.Contains(res.stdout, "HTTP_PROXY") || strings.Contains(res.stdout, "http_proxy") {
		t.Errorf("env prints an HTTP_PROXY variable; plain HTTP is not observed:\n%s", res.stdout)
	}

	// The lowercase form is not a duplicate for tidiness: curl reads only
	// lowercase https_proxy, and curl is the first thing anyone tests with.
	if !strings.Contains(res.stdout, "https_proxy") {
		t.Errorf("env omits lowercase https_proxy, the only form curl reads:\n%s", res.stdout)
	}
}

// TestH22_EnvIsEvalSafe is the property that makes `eval $(rashomon env)`
// usable. Every line must be an assignment; one stray word of prose on stdout
// and eval tries to run it as a command.
func TestH22_EnvIsEvalSafe(t *testing.T) {
	e := newEnv(t)
	res := e.run("", nil, "env")

	for i, line := range strings.Split(strings.TrimSpace(res.stdout), "\n") {
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "export ") {
			t.Errorf("stdout line %d is not an export assignment: %q. `eval $(rashomon "+
				"env)` would try to run it as a command.", i+1, line)
		}
		if strings.ContainsAny(line, "`$(;&|") {
			t.Errorf("stdout line %d carries shell metacharacters: %q", i+1, line)
		}
	}
}

// TestH22_EnvNeedsNoStore keeps the command usable before anything has been
// recorded. It prints constants; requiring a store would make the first thing
// an operator runs depend on having already run something else.
func TestH22_EnvNeedsNoStore(t *testing.T) {
	e := newEnv(t)
	res := e.run("", nil, "env")
	if res.exitCode != 0 {
		t.Fatalf("env on a machine with no store: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if !strings.Contains(res.stdout, wantHTTPSUp) {
		t.Errorf("env printed nothing useful without a store:\n%s", res.stdout)
	}
}

// TestH22_EnvPortIsConfigurable covers the collision case. 18080 is the
// default because 8080 is the enforce profile's port and the most commonly
// occupied port on a developer machine, but a second observe proxy has to be
// addressable.
func TestH22_EnvPortIsConfigurable(t *testing.T) {
	e := newEnv(t)
	res := e.run("", nil, "env", "--port", "19090")

	if res.exitCode != 0 {
		t.Fatalf("env --port: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if !strings.Contains(res.stdout, "export HTTPS_PROXY=http://127.0.0.1:19090") {
		t.Errorf("env --port 19090 did not change the address:\n%s", res.stdout)
	}
	if strings.Contains(res.stdout, observePort) {
		t.Errorf("env --port 19090 still prints the default port:\n%s", res.stdout)
	}
}

// TestH22_EnvRejectsABadPort refuses rather than falling back. A silently
// ignored --port sends the operator's traffic to whatever is on 18080, which
// may be someone else's proxy.
func TestH22_EnvRejectsABadPort(t *testing.T) {
	for _, bad := range []string{"0", "-1", "70000", "eighty-eighty", "18080; rm -rf /"} {
		e := newEnv(t)
		res := e.run("", nil, "env", "--port", bad)
		if res.exitCode == 0 {
			t.Errorf("env --port %q exited 0; a bad port must be refused, not defaulted, "+
				"or the operator's traffic goes to whatever is on the default port:\n%s",
				bad, res.stdout)
		}
	}
}
