package acceptance

import (
	"bytes"
	"strings"
	"testing"
)

// TestRubricERRORS10_WatchSaysWhereTheSettingsFileIsBroken runs watch over two
// settings files it has to refuse, and holds the message to what a person
// needs to fix them: the file, the line and column, and for a hook event the
// event's name. The file itself is left as it was found.
func TestRubricERRORS10_WatchSaysWhereTheSettingsFileIsBroken(t *testing.T) {
	for _, tc := range []struct {
		name, content, at string
		says              []string
	}{
		{
			// A trailing comma, which a hand edit leaves and JSON refuses.
			name:    "a syntax error",
			content: "{\n  \"permissions\": {\"allow\": []},\n}\n",
			at:      ":3:1: ",
			says:    []string{"invalid character '}'"},
		},
		{
			// Valid JSON that watch cannot edit: the event it installs into
			// is an object. The error surfaces inside the edit, past the
			// parse, and must still name the file and the place.
			name:    "a hook event that is not an array",
			content: "{\n  \"hooks\": {\n    \"PreToolUse\": {\"matcher\": \"*\"}\n  }\n}\n",
			at:      ":3:19: ",
			says:    []string{`hook event "PreToolUse" is not an array`},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			e.writeSettings(tc.content)
			res := e.watch()
			if res.exitCode == 0 {
				t.Fatalf("watch accepted a settings file it cannot edit; stdout %q", res.stdout)
			}
			want := e.settingsPath() + tc.at
			if !strings.Contains(res.stderr, want) {
				t.Errorf("stderr %q does not place the problem at %q", res.stderr, want)
			}
			if n := strings.Count(res.stderr, e.settingsPath()); n != 1 {
				t.Errorf("stderr names the file %d times, want once: %q", n, res.stderr)
			}
			for _, s := range tc.says {
				if !strings.Contains(res.stderr, s) {
					t.Errorf("stderr %q does not say %q", res.stderr, s)
				}
			}
			if got := e.settingsBytes(); !bytes.Equal(got, []byte(tc.content)) {
				t.Errorf("watch changed a file it refused:\n%s", got)
			}
		})
	}
}

// TestRubricERRORS10_DetachSaysWhereTheSettingsFileIsBroken: detach edits
// settings.json through the same re-reading edit watch writes with, and a
// file broken since watch installed is refused with its own name, line and
// column, once, and left as it was found.
func TestRubricERRORS10_DetachSaysWhereTheSettingsFileIsBroken(t *testing.T) {
	e := newEnv(t)
	if res := e.watch(); res.exitCode != 0 {
		t.Fatalf("watch: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	const broken = "{\n  \"permissions\": {\"allow\": []},\n}\n"
	e.writeSettings(broken)
	res := e.detach()
	if res.exitCode == 0 {
		t.Fatalf("detach accepted a settings file it cannot edit; stdout %q", res.stdout)
	}
	if want := e.settingsPath() + ":3:1: "; !strings.Contains(res.stderr, want) {
		t.Errorf("stderr %q does not place the problem at %q", res.stderr, want)
	}
	if n := strings.Count(res.stderr, e.settingsPath()); n != 1 {
		t.Errorf("stderr names the file %d times, want once: %q", n, res.stderr)
	}
	if got := e.settingsBytes(); !bytes.Equal(got, []byte(broken)) {
		t.Errorf("detach changed a file it refused:\n%s", got)
	}
}
