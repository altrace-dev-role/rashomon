package shape

import (
	"encoding/json"
	"slices"
	"testing"
)

// TestGB1e_RunnerKeyIsTheRowForWorkAndTheWordsForALauncher (card
// GATE-BIND1): every launcherRows entry is a row of buildCommands, so the
// launcher test compares names the lists produce. A gate's argv and a run of
// the same work-naming row with other words share a key, the row's name; a
// launcher row keys by its words, so `make build` is not `make lint`; a run
// with no runner_name (before call schema 8) keys by its words; no runner,
// no key.
// Break: drop a launcher, key a launcher by its name, or let an argument
// into a work-naming row's key.
func TestGB1e_RunnerKeyIsTheRowForWorkAndTheWordsForALauncher(t *testing.T) {
	builds := rowNames(buildCommands)
	for _, l := range launcherRows {
		if !slices.Contains(builds, l) {
			t.Errorf("launcherRows names %q, which is no row of buildCommands", l)
		}
	}
	key := []byte("k")
	keyOf := func(line string) string {
		input, err := json.Marshal(map[string]string{"command": line})
		if err != nil {
			t.Fatal(err)
		}
		s := Derive("Bash", input, key)
		name := ""
		if s.RunnerName != nil {
			name = *s.RunnerName
		}
		k, ok := RunnerKey(name, s.RunnerDigest)
		if !ok {
			t.Fatalf("%q: no runner key", line)
		}
		return k
	}
	for _, c := range []struct {
		gate, run string
		same      bool
	}{
		{"python3 -m pytest", "python3 -u -m pytest -q tests/test_a.py", true},
		{"go test ./...", "go test -count=1 -run TestX ./internal/...", true},
		{"npm test", "npm test -- --watch=false", true},
		{"cargo test", "cargo +nightly test --release", true},
		{"make test", "make -j4 test", true},
		{"pytest", "python3 -m pytest", false},
		{"make lint", "make build", false},
		{"npm run lint", "npm run build", false},
		{"mvn verify", "mvn package", false},
		{"go test ./...", "go vet ./...", false},
	} {
		if got := keyOf(c.gate) == keyOf(c.run); got != c.same {
			t.Errorf("%q and %q share a key: %v, want %v", c.gate, c.run, got, c.same)
		}
	}
	d := "0123abcd"
	if k, ok := RunnerKey("", &d); !ok || k != d {
		t.Errorf("RunnerKey with no name = %q, %v; want the digest", k, ok)
	}
	if _, ok := RunnerKey("go test", nil); ok {
		t.Error("RunnerKey with no digest has a key, want none")
	}
}
