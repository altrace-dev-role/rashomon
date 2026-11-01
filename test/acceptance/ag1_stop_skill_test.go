package acceptance

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestAGSK1_TheStopSkillsHandThePersonTheCommand: detach refuses an agent
// session (TestAGDP1_), and a `!` line typed into Claude Code is one, so a
// stop skill that runs detach itself only relays a refusal. Both copies hand
// the person the command, with the binary's full path, and the place every
// refusal names -- a terminal outside Claude Code -- and neither tells the
// agent to run a verb that refuses it. test/live/l-items.sh, which runs watch
// and detach itself, refuses the signal before it writes anything rather
// than stopping half set up.
//
// Break: put back "run `$RASHOMON detach`" in either skill, and "stop
// rashomon" ends in a refusal the agent can only relay.
func TestAGSK1_TheStopSkillsHandThePersonTheCommand(t *testing.T) {
	runs := regexp.MustCompile("(?i)\\brun\\s+`?\\$RASHOMON (detach|pause|purge|watch|consent|steer|summary|bodies|spec)\\b")
	block := regexp.MustCompile(`(?m)^ {4,}\$RASHOMON detach$`)
	for _, rel := range []string{"plugin/skills/stop/SKILL.md", "skills/rashomon-stop/SKILL.md"} {
		body, err := os.ReadFile(filepath.Join(moduleRoot, rel))
		if err != nil {
			t.Fatal(err)
		}
		text := string(body)
		if !block.MatchString(text) {
			t.Errorf("%s does not hand the person the command: no `$RASHOMON detach` on a line of its own", rel)
		}
		// The words are read as one line: a skill rewraps its prose.
		words := strings.Join(strings.Fields(text), " ")
		for _, want := range []string{"a terminal outside Claude Code", "the full path"} {
			if !strings.Contains(words, want) {
				t.Errorf("%s does not tell the agent where the person runs it: no %q", rel, want)
			}
		}
		if m := runs.FindString(words); m != "" {
			t.Errorf("%s tells the agent to %q, which refuses an agent session", rel, m)
		}
	}

	script, err := os.ReadFile(filepath.Join(moduleRoot, "test", "live", "l-items.sh"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(script)
	refuse, first := strings.Index(s, `if [ -n "${CLAUDECODE:-}" ]; then`), strings.Index(s, `rm -rf "$LIVE"`)
	if refuse < 0 || first < 0 || refuse > first {
		t.Errorf("l-items.sh does not refuse CLAUDECODE before it writes anything (refusal at %d, first write at %d)", refuse, first)
	}
}
