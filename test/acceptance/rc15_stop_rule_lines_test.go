package acceptance

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

// rc15Fetch is B237's line, as TestRC15_ runs it.
func rc15Fetch(dir string) string {
	return "cd " + dir + " && git fetch -q 2>/dev/null; git status -sb | head -2"
}

// rc15Call is dghCall of a Bash command under promptID, the session's
// prompt for the call, and returns the post's stdout.
func (e *env) rc15Call(dir, promptID, id, command string) string {
	e.t.Helper()
	envs := []string{stAgent, "CI="}
	input := map[string]any{"command": command}
	if res := e.hook(stPayload(e.t, "PreToolUse", id, "Bash", input, map[string]any{"cwd": dir, "prompt_id": promptID}), envs...); res.exitCode != 0 || res.stdout != "" {
		e.t.Fatalf("hook %s: exit %d, stdout %q, stderr %q", id, res.exitCode, res.stdout, res.stderr)
	}
	more := map[string]any{"cwd": dir, "prompt_id": promptID, "tool_response": map[string]any{"stdout": "ok\n", "stderr": "", "interrupted": false}}
	res := e.post(stPayload(e.t, "PostToolUse", id, "Bash", input, more), envs...)
	if res.exitCode != 0 {
		e.t.Fatalf("post %s: exit %d, stderr %q", id, res.exitCode, res.stderr)
	}
	return res.stdout
}

// TestRC15g_TheStopNamesARuleInItsTurnAndWhereTheTurnSpeaks (card
// RECALL-15): the rule a call matched is named at the Stop of the call's
// turn, never at a later turn's, whose k still counts it (D40); an untrusted
// spec's rule binds nothing and is not named; with the summary switch off
// the line that names it still speaks, as the done flag does; and recap.Line's
// sentence carries it at the Stop, before the pointer, while StopFailure,
// which prints no value line (tech 2.2.11 condition 4), names none. Break:
// name the session's verdicts, an untrusted rule's, go quiet with the switch
// off, drop the lines from recap.Line's sentence, or print them at
// StopFailure, and this fails.
func TestRC15g_TheStopNamesARuleInItsTurnAndWhereTheTurnSpeaks(t *testing.T) {
	t.Run("a later turn", func(t *testing.T) {
		e := newSteerEnv(t)
		dir, path := dghProject(t, rc15Spec)
		e.trust(path)
		e.rc15Call(dir, "p1", "toolu_f", rc15Fetch(dir))
		if got := e.dghStop(dir); !strings.Contains(got, "spec rule no-fetch-all matched call ") {
			t.Fatalf("premise: the fetch's own turn ends with %q, want the rule named", got)
		}
		e.rc15Call(dir, "p2", "toolu_s", "cd "+dir+" && git status -sb | head -2")
		if got := e.dghStop(dir); strings.Contains(got, "no-fetch-all") {
			t.Errorf("the next turn's Stop line %q names the earlier turn's rule", got)
		}
	})
	t.Run("an untrusted spec", func(t *testing.T) {
		e := newSteerEnv(t)
		dir, _ := dghProject(t, rc15Spec)
		e.rc15Call(dir, "p1", "toolu_f", rc15Fetch(dir))
		if got := e.dghStop(dir); strings.Contains(got, "no-fetch-all") {
			t.Errorf("Stop line %q names a rule of a spec no one trusted", got)
		}
	})
	t.Run("the summary switch off", func(t *testing.T) {
		e := newSteerEnv(t)
		if res := e.run("", nil, "summary", "off"); res.exitCode != 0 {
			t.Fatalf("summary off: exit %d, stderr %q", res.exitCode, res.stderr)
		}
		dir, path := dghProject(t, rc15Spec)
		e.trust(path)
		e.rc15Call(dir, "p1", "toolu_f", rc15Fetch(dir))
		if got := e.dghStop(dir); !strings.Contains(got, "spec rule no-fetch-all matched call ") {
			t.Errorf("with the summary off the Stop line is %q, want the rule named", got)
		}
	})

	const lead = "※ rashomon: "
	pad := strings.Repeat(" ", len([]rune(lead)))
	pointer := pad + "→ rashomon report --session " + testSession
	for _, tc := range []struct {
		event string
		want  *regexp.Regexp
	}{
		{"Stop", regexp.MustCompile("^" + regexp.QuoteMeta(lead+"1 recorded failure.\n"+pad+"spec rule no-fetch-all matched call ") +
			"[0-9]+\n" + regexp.QuoteMeta(pointer) + "$")},
		{"StopFailure", regexp.MustCompile("^" + regexp.QuoteMeta(lead+"1 recorded failure.\n"+pointer) + "$")},
	} {
		t.Run(tc.event, func(t *testing.T) {
			e := newSteerEnv(t)
			dir, path := dghProject(t, rc15Spec)
			e.trust(path)
			e.rc15Call(dir, "p1", "toolu_f", rc15Fetch(dir))
			e.dghFailedCall(dir, "toolu_g", "Bash", dghRun, 1)
			res := e.recap(stopEvent(tc.event, "I fetched.", dir), stAgent, "CI=")
			var rec recapOutput
			if res.exitCode != 0 || json.Unmarshal([]byte(res.stdout), &rec) != nil {
				t.Fatalf("recap %s: exit %d, stdout %q, stderr %q", tc.event, res.exitCode, res.stdout, res.stderr)
			}
			if !tc.want.MatchString(rec.SystemMessage) {
				t.Errorf("the %s line:\n%s\nwant it to match:\n%s", tc.event, rec.SystemMessage, tc.want)
			}
		})
	}
}
