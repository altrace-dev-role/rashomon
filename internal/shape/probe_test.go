package shape

import (
	"encoding/json"
	"testing"
)

// TestH52_AProbeIsReadFromEveryCommand (issue #52): a Bash line is a probe
// where every one of its commands asks whether a program is installed and
// runs none: which or type with a name, or command -v or -V with one. A line
// with any other command is none, since its exit may be that command's:
// `which pytest && pytest -q` exits 1 when a test failed. A line the parse
// cannot read whole is none. Break: read the first command only, accept a
// bare which or `command` without -v, or read a line in part.
func TestH52_AProbeIsReadFromEveryCommand(t *testing.T) {
	for _, c := range []struct {
		command string
		want    bool
	}{
		{"which pytest", true},
		{"which -a go python3", true},
		{"command -v pytest >/dev/null 2>&1", true},
		{"command -V pytest", true},
		{"type pytest", true},
		{"which jq; type -P yq", true},
		{"which pytest && pytest -q", false},
		{"which pytest || pip install pytest", false},
		{"which pytest; $RUNNER -q", false},
		{"command pytest -q", false},
		{"command -v", false},
		{"which", false},
		{"", false},
		{"make test", false},
		{"bash -c 'which x'", false},
	} {
		in, err := json.Marshal(map[string]string{"command": c.command})
		if err != nil {
			t.Fatal(err)
		}
		if s := Derive("Bash", in, []byte("k")); s.Probe != c.want {
			t.Errorf("Derive(Bash, %q).Probe = %v, want %v", c.command, s.Probe, c.want)
		}
	}
	for _, tool := range []string{"Read", "Grep"} {
		in, _ := json.Marshal(map[string]string{"command": "which pytest", "pattern": "which pytest"})
		if Derive(tool, in, []byte("k")).Probe {
			t.Errorf("Derive(%s).Probe is true: the bit is a Bash line's", tool)
		}
	}
}
