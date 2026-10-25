package pgraph

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func envMap(env []string) (map[string][]string, int) {
	m := map[string][]string{}
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		m[k] = append(m[k], v)
	}
	return m, len(env)
}

func TestPL9_LoaderEnvForced(t *testing.T) {
	hostile := []string{
		"PATH=/usr/bin", "HOME=/h",
		"GOPROXY=https://proxy.golang.org,direct", "GOTOOLCHAIN=auto", "GOSUMDB=sum.golang.org",
		"GOFLAGS=-mod=mod", "GOWORK=/x/go.work", "GONOSUMDB=", "GOENV=/e", "CGO_ENABLED=1",
		"GOOS=plan9", "GOARCH=386", "GOPROXY=direct",
	}
	m, _ := envMap(LoaderEnv(hostile, Config{GOOS: "linux", GOARCH: "amd64"}))
	for k, v := range map[string]string{
		"GOPROXY": "off", "GOTOOLCHAIN": "local", "GOSUMDB": "off", "GOFLAGS": "-mod=readonly",
		"GOWORK": "off", "GOENV": "off", "CGO_ENABLED": "0", "GOOS": "linux", "GOARCH": "amd64", "GO111MODULE": "on",
	} {
		if len(m[k]) != 1 || m[k][0] != v {
			t.Errorf("%s = %v; want exactly [%s]", k, m[k], v)
		}
	}
	// Negative: unrelated variables survive.
	if m["PATH"][0] != "/usr/bin" || m["HOME"][0] != "/h" {
		t.Errorf("unrelated env lost: %v", m)
	}
	if _, ok := m["GONOSUMDB"]; !ok {
		t.Error("variables outside the forced set must pass through")
	}
}

// A dependency that is not in the module cache must fail at once with the
// proxy-disabled error, not with a connection error: no attempt is made. The
// caller's environment asks for the public proxy; LoaderEnv overrides it.
func TestPL9_MissingModuleFailsWithoutDialing(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.test/off\n\ngo 1.25.0\n\nrequire example.invalid/nope v1.2.3\n"), 0o644)
	// A go.sum line that is present (and wrong) forces a module lookup.
	os.WriteFile(filepath.Join(dir, "go.sum"), []byte("example.invalid/nope v1.2.3 h1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=\nexample.invalid/nope v1.2.3/go.mod h1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "m.go"), []byte("package off\n\nimport _ \"example.invalid/nope\"\n"), 0o644)

	start := time.Now()
	if _, err := Load(t.Context(), dir, Config{}); err == nil {
		t.Fatal("load of an unfetchable dependency succeeded")
	}
	// go/packages does not surface go list's own message, so read it from the
	// tool in the same environment, starting from a hostile one.
	cmd := exec.CommandContext(t.Context(), "go", "list", "-e", "-json", "-deps", "./...")
	cmd.Dir = dir
	cmd.Env = LoaderEnv(append(os.Environ(), "GOPROXY=https://proxy.golang.org,direct", "GOFLAGS=-mod=mod", "GOTOOLCHAIN=auto"), Config{})
	out, _ := cmd.CombinedOutput()
	if !strings.Contains(string(out), "module lookup disabled by GOPROXY=off") {
		t.Errorf("go list did not report the proxy as off:\n%s", out)
	}
	for _, net := range []string{"dial tcp", "no such host", "i/o timeout", "connection refused", "TLS"} {
		if strings.Contains(string(out), net) {
			t.Errorf("network attempt visible: %q in\n%s", net, out)
		}
	}
	if d := time.Since(start); d > 30*time.Second {
		t.Errorf("took %v: something waited on the network", d)
	}
}

// Re-runs the loader tests inside an empty network namespace: any connection
// attempt would fail there, and the tests must still pass.
func TestPL9_LoaderRunsInNetworklessNamespace(t *testing.T) {
	if os.Getenv("PGRAPH_NETNS") != "" {
		t.Skip("already inside the namespace")
	}
	unshare, err := exec.LookPath("unshare")
	if err != nil || exec.Command(unshare, "-n", "true").Run() != nil {
		t.Skip("unshare -n unavailable here; the forced-environment tests above are the fallback")
	}
	cmd := exec.Command(unshare, "-n", os.Args[0], "-test.count=1", "-test.run", "^(TestPL1_ResolvedSetEqualsGoList|TestPL2_)")
	cmd.Env = append(os.Environ(), "PGRAPH_NETNS=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("under unshare -n: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "PASS") {
		t.Fatalf("no PASS in output:\n%s", out)
	}
}
