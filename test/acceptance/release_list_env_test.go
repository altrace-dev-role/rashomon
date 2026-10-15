package acceptance

import (
	"strings"
	"testing"
)

// envValue returns the LAST value env carries for key, Cmd.Env's own rule,
// or ok=false if key is absent.
func envValue(env []string, key string) (string, bool) {
	prefix := key + "="
	var val string
	var found bool
	for _, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			val, found = strings.TrimPrefix(kv, prefix), true
		}
	}
	return val, found
}

// TestReleaseListEnv_ClearsAmbientGOFLAGSAndGOEXPERIMENT: the matrix reader
// shells out to `go list` to build the release listing TRU-40 and RES-3
// judge. A developer's GOFLAGS=-tags=withnet, from a shell export or `go env
// -w`, would silently relax what a target reports as linked -- the exact
// premise TRU-40 exists to hold. GOEXPERIMENT can change which packages a
// build even compiles. Both must read as unset in the environment `go list`
// actually runs under, whatever the ambient process carries.
func TestReleaseListEnv_ClearsAmbientGOFLAGSAndGOEXPERIMENT(t *testing.T) {
	t.Setenv("GOFLAGS", "-tags=withnet")
	t.Setenv("GOEXPERIMENT", "rangefunc")

	env := releaseListEnv(releaseTarget{goos: "linux", goarch: "amd64"}, nil)

	for _, key := range []string{"GOFLAGS", "GOEXPERIMENT"} {
		got, ok := envValue(env, key)
		if !ok {
			t.Errorf("%s is absent from the listing env; the ambient shell's own value would "+
				"leak through to `go list`", key)
			continue
		}
		if got != "" {
			t.Errorf("%s = %q in the listing env, want \"\": the ambient value was not cleared", key, got)
		}
	}
}

// TestReleaseListEnv_ABuildMatrixLineStillWinsOverTheDefault: Cmd.Env keeps
// the LAST value for a duplicate key. extra -- .goreleaser.yml's env: line
// for a target -- is appended after the forced-empty defaults, so a line
// that genuinely needed to set GOFLAGS or GOEXPERIMENT would still take
// effect; this is what makes forcing them empty safe to do unconditionally.
func TestReleaseListEnv_ABuildMatrixLineStillWinsOverTheDefault(t *testing.T) {
	env := releaseListEnv(releaseTarget{goos: "linux", goarch: "amd64"}, []string{"GOFLAGS=-tags=cgo"})

	got, ok := envValue(env, "GOFLAGS")
	if !ok || got != "-tags=cgo" {
		t.Errorf("GOFLAGS = %q, ok=%v, want \"-tags=cgo\": a build matrix line's own value "+
			"must still win over the forced-empty default", got, ok)
	}
}

// TestReleaseListEnv_SetsGOOSAndGOARCHLast: the target's own GOOS and GOARCH
// must win over anything else in the environment, since the whole point of
// this listing is one target at a time.
func TestReleaseListEnv_SetsGOOSAndGOARCHLast(t *testing.T) {
	t.Setenv("GOOS", "darwin")
	t.Setenv("GOARCH", "arm64")

	env := releaseListEnv(releaseTarget{goos: "windows", goarch: "386"}, nil)

	if got, _ := envValue(env, "GOOS"); got != "windows" {
		t.Errorf("GOOS = %q, want windows", got)
	}
	if got, _ := envValue(env, "GOARCH"); got != "386" {
		t.Errorf("GOARCH = %q, want 386", got)
	}
}
