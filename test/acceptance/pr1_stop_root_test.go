package acceptance

// PROJECT-RESOLVE1, the root (eng-2's finding, manager 07:4x): a call's
// project is the repository it ran in, but a judgment has one project, the
// one its spec and inferred gate were read from: the Stop's and the post's
// cwd. Taking the root from the latest declaration's Project moved the
// session's gates to whatever repository its last call touched.

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// pr1Stop is the Stop line of a session in repository a, inferred gate go
// test: an edit, a green go test ./..., then last (nil for none), whose calls
// name b, another repository; and its `report --ledger` rows, the kind,
// state and evidence of each, which the replay (laststop) derives.
func pr1Stop(t *testing.T, last func(e *env, a, b string)) (string, []string) {
	t.Helper()
	e := newSteerEnv(t)
	a := gi1Project(t, map[string]string{"go.mod": "module a\n"})
	b := gi1Project(t, map[string]string{"go.mod": "module b\n"})
	e.dghCall(a, "toolu_e", "Edit", dghEdit(a))
	e.dghCall(a, "toolu_g", "Bash", map[string]any{"command": "go test ./..."})
	if last != nil {
		last(e, a, b)
	}
	line := e.dghStop(a)
	t.Logf("Stop line: %q", line)
	res := e.run("", nil, "report", "--ledger", "--session", testSession)
	if res.exitCode != 0 {
		t.Fatalf("report --ledger: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	var rows []string
	for _, l := range strings.Split(res.stdout, "\n") {
		// A row's kind has an underscore; no header word of seven does.
		if f := strings.Fields(l); len(f) == 7 && strings.Contains(f[2], "_") {
			rows = append(rows, f[2]+" "+f[4]+" "+f[5])
		}
	}
	t.Logf("ledger:\n%s", res.stdout)
	return line, rows
}

// TestPR1e_TheStopJudgesTheProjectItsSpecCameFrom: a session in a whose
// green run binds a's inferred gate keeps that Stop line, and that ledger,
// when its last call is a `git -C b status`, or a Write of a file at b's
// root: the gate stays a's, where the Stop read it, and the run still meets
// it; the replay takes the root the Stop's checkpoint recorded. Today the
// root is the last declaration's Project, b, the gate is placed under b, and
// the line says go test was not run. Break: take the root from the
// declarations, at the Stop or in the replay.
func TestPR1e_TheStopJudgesTheProjectItsSpecCameFrom(t *testing.T) {
	want, wantRows := pr1Stop(t, nil)
	met := func(r string) bool { return strings.HasPrefix(r, "gate_after_write met ") }
	if !strings.Contains(want, "verified") || !slices.ContainsFunc(wantRows, met) {
		t.Fatalf("premise: the control's Stop line %q is not verified, or its ledger %q has no met gate_after_write", want, wantRows)
	}
	for _, c := range []struct {
		name string
		last func(e *env, a, b string)
	}{
		{"git -C b status", func(e *env, a, b string) {
			e.dghCall(a, "toolu_l", "Bash", map[string]any{"command": "git -C " + b + " status"})
		}},
		{"a Write at b's root", func(e *env, a, b string) {
			e.dghCall(a, "toolu_l", "Write", map[string]any{"file_path": filepath.Join(b, "README.md"), "content": "b\n"})
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, rows := pr1Stop(t, c.last)
			if got != want {
				t.Errorf("Stop line %q, want the control's %q", got, want)
			}
			if !slices.Equal(rows, wantRows) {
				t.Errorf("ledger rows %q, want the control's %q", rows, wantRows)
			}
		})
	}
}

// pr1Regressions is the keys of the session's regression-location steer
// lines: the note the post built, whether or not the client carried it.
func pr1Regressions(e *env) []string {
	var keys []string
	for _, l := range e.read(testSession, "steer.ndjson") {
		if l.str("pattern") == "regression-location" {
			keys = append(keys, l.str("key"))
		}
	}
	return keys
}

// TestPR1f_AnotherRepositorysRunsAreNoRegressionOfThisProjectsGate: from a,
// whose trusted spec declares gate unit (go test ./...), a green `cd b && go
// test ./...`, an edit in b and a red `cd b && go test ./...` build no
// regression note of unit at the red run's post: both runs are b's, and unit
// is a's (b's runs make a learned family of their own, whose note stands, as
// on main). Control: the same in a builds one. The steer log is read, since
// Claude Code 2.1.280 is not sent a note at PostToolUseFailure. Today the red
// run's post takes the root from its own declaration, b, places unit under b,
// binds both of b's runs to it and builds unit's regression note. Break: take
// the post's root from the declarations.
func TestPR1f_AnotherRepositorysRunsAreNoRegressionOfThisProjectsGate(t *testing.T) {
	e, a := dghEnv(t, true)
	e.dghCall(a, "toolu_g", "Bash", dghRun)
	e.dghCall(a, "toolu_e", "Edit", dghEdit(a))
	e.dghFailedCall(a, "toolu_r", "Bash", dghRun, 1)
	if keys := pr1Regressions(e); len(keys) != 1 || !strings.Contains(keys[0], "unit") {
		t.Fatalf("premise: a red run of unit after a green one and an edit in a: regression notes %v, want one of unit", keys)
	}
	e2, a2 := dghEnv(t, true)
	b := gi1Project(t, map[string]string{"go.mod": "module b\n"})
	inB := map[string]any{"command": "cd " + b + " && go test ./..."}
	e2.dghCall(a2, "toolu_g", "Bash", inB)
	e2.dghCall(a2, "toolu_e", "Edit", dghEdit(b))
	e2.dghFailedCall(a2, "toolu_r", "Bash", inB, 1)
	if keys := pr1Regressions(e2); slices.ContainsFunc(keys, func(k string) bool { return strings.Contains(k, "unit") }) {
		t.Errorf("b's red run after b's green one and an edit in b: regression notes %v, want none of a's gate unit", keys)
	}
}
