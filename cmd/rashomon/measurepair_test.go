package main

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/measure"
	"github.com/altrace-dev-role/rashomon/internal/steer"
)

// TestMS1p_MeasurePairReadsItsArguments: two `--session ID --arm ARM` pairs
// and an optional --json; anything else is refused before the store opens.
func TestMS1p_MeasurePairReadsItsArguments(t *testing.T) {
	asks, asJSON, err := pairArgs([]string{"--session", "a", "--arm", "off", "--home", "/h1", "--config", "/c1", "--json",
		"--session", "b", "--arm", "steer"})
	want := [2]measure.Ask{{SessionID: "a", Arm: steer.ArmOff, Root: "/h1", ConfigDir: "/c1"}, {SessionID: "b", Arm: steer.ArmSteer}}
	if err != nil || !asJSON || asks != want {
		t.Errorf("pairArgs = %+v, %t, %v; want %+v, true", asks, asJSON, err, want)
	}
	for _, bad := range [][]string{
		nil,
		{"--session", "a", "--arm", "off"},
		{"--session", "a", "--arm", "live", "--session", "b", "--arm", "steer"},
		{"--session", "a", "--arm", "OFF", "--session", "b", "--arm", "steer"},
		{"--session", "a", "off", "--session", "b", "--arm", "steer"},
		{"--session", "a", "--arm"},
		{"--session", "a", "--arm", "off", "--session", "b", "--arm", "steer", "--session", "c", "--arm", "off"},
		{"--session", "a", "--arm", "off", "--session", "b", "--arm", "steer", "--days", "3"},
		{"--home", "/h", "--session", "a", "--arm", "off", "--session", "b", "--arm", "steer"},
		{"--session", "a", "--arm", "off", "--home", "/h", "--home", "/h2", "--session", "b", "--arm", "steer"},
		{"--session", "a", "--arm", "off", "--session", "b", "--arm", "steer", "--config"},
	} {
		if _, _, err := pairArgs(bad); err == nil {
			t.Errorf("pairArgs(%q) = nil error", bad)
		}
	}
	t.Setenv("RASHOMON_HOME", filepath.Join(t.TempDir(), "none"))
	var stdout, stderr bytes.Buffer
	if code := run([]string{"measure", "pair", "--session", "a", "--arm", "off", "--session", "b", "--arm", "steer"}, nil, &stdout, &stderr); code != exitFail || stdout.Len() != 0 {
		t.Errorf("measure pair with no store: exit %d, stdout %q", code, stdout.String())
	}
}
