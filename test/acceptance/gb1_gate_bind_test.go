package acceptance

// GB1: a trusted declared gate binds a run of its runner, not of its
// argument list (card GATE-BIND1, R47-R49). The runner is the row of the
// runner lists the line's words match (shape's RunnerName: `python3 -m
// pytest`, `go test`, `npm test`); a launcher row (`make`, `npm run`), whose
// next word names the work, keeps the exact words; match:exact keeps the
// whole line. A bound run whose words are not the gate's never reads as the
// gate passed: "ran with other arguments and passed", within 80 columns.

import (
	"strings"
	"testing"
)

// gb1Spec is a repository spec declaring one gate per entry: id, kind, the
// argv as a JSON array, and "exact" for match:exact.
func gb1Spec(gates ...[4]string) string {
	var b strings.Builder
	b.WriteString(`{"rashomon_spec": 1, "gates": [`)
	for i, g := range gates {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(`{"id": "` + g[0] + `", "kind": "` + g[1] + `", "argv": ` + g[2])
		if g[3] == "exact" {
			b.WriteString(`, "match": "exact"`)
		}
		b.WriteString("}")
	}
	b.WriteString("]}")
	return b.String()
}

// gb1Stop is the Stop line of a session in a project whose trusted spec is
// doc: one edit, then a green Bash call of each command.
func gb1Stop(t *testing.T, doc string, commands ...string) string {
	t.Helper()
	e := newSteerEnv(t)
	dir, path := dghProject(t, doc)
	e.trust(path)
	e.dghCall(dir, "toolu_e", "Edit", dghEdit(dir))
	for i, c := range commands {
		e.dghCall(dir, "toolu_g"+string(rune('0'+i)), "Bash", map[string]any{"command": c})
	}
	got := e.dghStop(dir)
	t.Logf("Stop line: %q", got)
	return got
}

// gb1Columns fails t for a line of out over 80 columns.
func gb1Columns(t *testing.T, out string) {
	t.Helper()
	for _, l := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if n := len([]rune(l)); n > 80 {
			t.Errorf("a line is %d columns, want at most 80: %q", n, l)
		}
	}
}

// TestGB1a_ARunOfTheGatesRunnerWithOtherArgumentsBindsIt: QA-2's 4b (e2e/
// live/bind4b.txt). Each run is the gate's runner with words the gate does
// not have: a flag, a path, a selector. Each binds the gate, and the line
// says the run's words were other than the gate's instead of a bare
// "passed".
// Break: key the gate by its exact words (today's RunnerDigest), or print a
// bound run whose words differ as "passed".
func TestGB1a_ARunOfTheGatesRunnerWithOtherArgumentsBindsIt(t *testing.T) {
	const want = "verified - 'unit' ran with other arguments and passed."
	for _, c := range []struct{ name, argv, run string }{
		{"python3 -m pytest with a flag and a path", `["python3", "-m", "pytest"]`, "python3 -m pytest -q -p no:cacheprovider tests/test_a.py"},
		{"pytest with a flag and a path", `["pytest"]`, "pytest -q tests/test_a.py"},
		{"go test with a flag before the path", `["go", "test", "./..."]`, "go test -count=1 ./..."},
		{"go test narrowed by -run", `["go", "test", "./..."]`, "go test -run TestX ./..."},
		{"npm test with arguments after --", `["npm", "test"]`, "npm test -- --watch=false"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := gb1Stop(t, gb1Spec([4]string{"unit", "test", c.argv, ""}), c.run)
			if !strings.Contains(got, want) {
				t.Errorf("Stop line %q, want it to contain %q", got, want)
			}
			if strings.Contains(got, "'unit' passed") {
				t.Errorf("Stop line %q says 'unit' passed for a run whose words are not the gate's", got)
			}
			gb1Columns(t, got)
		})
	}
}

// TestGB1b_ControlsTheGatesOwnWordsExactAndLaunchers: what must not change.
// The gate's own words read "passed" as before. A match:exact gate is not
// bound by its runner with a flag. A launcher row's gate (`make lint`, `npm
// run lint`) is not bound by the same launcher running other work (`make
// build`, `npm run build`): its next word names the work, and the record
// does not hold that word, so it keeps the exact words.
// Break: let exact fall through to the runner key, or key a launcher row by
// its name.
func TestGB1b_ControlsTheGatesOwnWordsExactAndLaunchers(t *testing.T) {
	t.Run("the gate's own words", func(t *testing.T) {
		got := gb1Stop(t, gb1Spec([4]string{"unit", "test", `["python3", "-m", "pytest"]`, ""}), "python3 -m pytest")
		if !strings.Contains(got, "verified - 'unit' passed after the last edit.") || strings.Contains(got, "other arguments") {
			t.Errorf("Stop line %q, want \"verified - 'unit' passed after the last edit.\" and no other arguments", got)
		}
	})
	for _, c := range []struct{ name, id, kind, argv, match, run string }{
		{"match:exact and a flag", "unit", "test", `["go", "test", "./..."]`, "exact", "go test -count=1 ./..."},
		{"make lint against make build", "lint", "lint", `["make", "lint"]`, "", "make build"},
		{"npm run lint against npm run build", "lint", "lint", `["npm", "run", "lint"]`, "", "npm run build"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := gb1Stop(t, gb1Spec([4]string{c.id, c.kind, c.argv, c.match}), c.run)
			if strings.Contains(got, "verified") || strings.Contains(got, "'"+c.id+"' passed") || strings.Contains(got, "'"+c.id+"' ran with") {
				t.Errorf("Stop line %q binds '%s' to %q", got, c.id, c.run)
			}
		})
	}
}

// TestGB1c_TheLineFitsWithALongIDAndBesideAPlainGate: the differing run's
// words fit 80 columns. A 24-rune id (recap's maxIDName) does not fit
// "verified - '<id>' ran with other arguments and passed.", so the gate is
// named as recap names it without the id; beside a gate whose run was its
// own words, each reads as it ran.
// Break: print the long id past 80 columns, or fold the two into one verb.
func TestGB1c_TheLineFitsWithALongIDAndBesideAPlainGate(t *testing.T) {
	t.Run("a 24-rune id", func(t *testing.T) {
		got := gb1Stop(t, gb1Spec([4]string{"unit-tests-of-the-module", "test", `["go", "test", "./..."]`, ""}), "go test -count=1 ./...")
		if !strings.Contains(got, "ran with other arguments and passed.") {
			t.Errorf("Stop line %q, want the run named as ran with other arguments and passed", got)
		}
		gb1Columns(t, got)
	})
	t.Run("beside a plain gate", func(t *testing.T) {
		doc := gb1Spec([4]string{"unit", "test", `["go", "test", "./..."]`, ""}, [4]string{"vet", "lint", `["go", "vet", "./..."]`, ""})
		got := gb1Stop(t, doc, "go vet ./...", "go test -count=1 ./...")
		const want = "verified - 'vet' passed, 'unit' ran with other arguments and passed."
		if !strings.Contains(got, want) {
			t.Errorf("Stop line %q, want it to contain %q", got, want)
		}
		gb1Columns(t, got)
	})
}

// TestGB1d_A2316StringFailureIsAFailedForegroundRun: pin (R48), Claude
// Code 2.1.316's failure of a Bash call (e2e/live/F, session c63d8c58):
// PostToolUseFailure with error "Exit code 1" and tool_response the string
// "Error: Exit code 1", where a PostToolUse carries an object. The run is
// recorded failed, in the foreground, with its exit status when error opens
// "Exit code " and none otherwise (never 0); the Stop speaks the turn's
// silent failure, which outranks the value line, and never a background
// move.
// Break: read a number out of any text in error, or take a string
// tool_response for a background call.
func TestGB1d_A2316StringFailureIsAFailedForegroundRun(t *testing.T) {
	for _, c := range []struct {
		name, err string
		exit      any
	}{
		{"error Exit code 1", "Exit code 1", float64(1)},
		{"error Error: Exit code 1", "Error: Exit code 1", nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := newSteerEnv(t)
			dir, path := dghProject(t, dghSpec)
			e.trust(path)
			e.dghCall(dir, "toolu_e", "Edit", dghEdit(dir))
			envs := []string{stAgent, "CI="}
			if res := e.hook(stPayload(t, "PreToolUse", "toolu_f", "Bash", dghRun, map[string]any{"cwd": dir}), envs...); res.exitCode != 0 || res.stdout != "" {
				t.Fatalf("hook: exit %d, stdout %q, stderr %q", res.exitCode, res.stdout, res.stderr)
			}
			more := map[string]any{"cwd": dir, "error": c.err, "is_interrupt": false, "duration_ms": 47, "tool_response": "Error: Exit code 1"}
			if res := e.post(stPayload(t, "PostToolUseFailure", "toolu_f", "Bash", dghRun, more), envs...); res.exitCode != 0 {
				t.Fatalf("post: exit %d, stderr %q", res.exitCode, res.stderr)
			}
			var run map[string]any
			for _, r := range e.executions(testSession) {
				if r.fields["tool_use_id"] == "toolu_f" {
					run = r.fields
				}
			}
			if run == nil || run["outcome"] != "failed" || run["exit_code"] != c.exit {
				t.Errorf("execution of toolu_f %v, want outcome failed and exit_code %v", run, c.exit)
			}
			got := e.dghStop(dir)
			t.Logf("Stop line: %q", got)
			if !strings.Contains(got, "1 recorded failure") || strings.Contains(got, "background") {
				t.Errorf("Stop line %q, want the turn's one recorded failure and no background move", got)
			}
		})
	}
}
