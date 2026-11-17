package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/hook"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// Card RV1, item 4: a task-preflight note whose only model line names the
// paths the request itself named tells the agent nothing it was not told, so
// it is a flag for the person and never a nudge. Where the prompt also builds
// the graph brief, the brief takes the prompt's nudge (outcome a); where the
// note holds a rule or gate line, the note keeps the nudge and the brief is
// superseded (outcome b); a paths-only note alone prints to the person and
// sends the model nothing (outcome c). A delivered flag's paths count as
// given; a candidate's do not.
//
// Each test asserts its premise first, from the steer line's slots, which
// every line keeps (a line that did not print keeps no text): the note it
// reads is the request's note, key note, and lists exactly the paths the row
// names and no rule, gate, symbol or lesson.

// rv1Head is a later prompt's preflight header and the paths line it opens.
const rv1Head = "rashomon record: preflight for this request from the repository; the request text is not kept.\npaths named: "

// rv1GraphHead opens the graph brief's model text.
const rv1GraphHead = "rashomon record: the code this request touches"

// rv1Line is the part of a task-preflight steer line these tests read.
type rv1Line struct {
	PromptID     *string                    `json:"prompt_id"`
	Pattern      string                     `json:"pattern"`
	Key          string                     `json:"key"`
	Level        string                     `json:"level"`
	State        string                     `json:"state"`
	SuppressedBy *string                    `json:"suppressed_by"`
	Slots        map[string]json.RawMessage `json:"slots"`
}

// by is the line's suppressed_by, "" for none.
func (l *rv1Line) by() string {
	if l == nil || l.SuppressedBy == nil {
		return ""
	}
	return *l.SuppressedBy
}

// row is the line's state, suppressed_by and level, as a failure prints it.
func (l *rv1Line) row() string {
	if l == nil {
		return "no line"
	}
	return fmt.Sprintf("state %s, suppressed_by %q, level %s", l.State, l.by(), l.Level)
}

// rv1Lines are the request's note and the graph brief's line at promptID,
// each nil when the prompt logged none.
func (e *lessonEnv) rv1Lines(sessionID, promptID string) (note, graphed *rv1Line) {
	e.t.Helper()
	st, err := store.Open(e.root)
	if err != nil {
		e.t.Fatal(err)
	}
	raw, _, err := st.ReadSteer(sessionID)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		e.t.Fatal(err)
	}
	for _, b := range raw {
		var l rv1Line
		if err := json.Unmarshal(b, &l); err != nil {
			e.t.Fatal(err)
		}
		if l.Pattern != "task-preflight" || l.PromptID == nil || *l.PromptID != promptID {
			continue
		}
		switch {
		case l.Key == "note" && note == nil:
			note = &l
		case l.Key == keyGraph && graphed == nil:
			graphed = &l
		case l.Key == "note" || l.Key == keyGraph:
			e.t.Fatalf("two task-preflight lines with key %q at %s", l.Key, promptID)
		}
	}
	return note, graphed
}

// rv1Lists is how many items the line's preflight slot lists: a path set's
// items and more, a rule list's ids and more, a gate list's gates and more; 0
// for a slot the line does not hold. A count given earlier, withheld or
// learned lists nothing, as the renderer reads it (slot.Lister).
func rv1Lists(t *testing.T, l *rv1Line, name string) int {
	t.Helper()
	raw, ok := l.Slots[name]
	if !ok {
		return 0
	}
	var v struct {
		Items []string `json:"items"`
		IDs   []string `json:"ids"`
		Gates []string `json:"gates"`
		More  int      `json:"more"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("slot %s = %s: %v", name, raw, err)
	}
	return len(v.Items) + len(v.IDs) + len(v.Gates) + v.More
}

// rv1Holds fails the test unless note is the request's note and its model
// slots list paths paths, rules rules and gates gates, and no symbol or
// lesson: the premise each row stands on.
func rv1Holds(t *testing.T, note *rv1Line, paths, rules, gates int) {
	t.Helper()
	if note == nil {
		t.Fatal("premise: no task-preflight line with key note")
	}
	for _, c := range []struct {
		name string
		want int
	}{{"paths", paths}, {"rules", rules}, {"gates", gates}} {
		if got := rv1Lists(t, note, c.name); got != c.want {
			t.Fatalf("premise: the note's %s slot lists %d, want %d (slots %v)", c.name, got, c.want, note.Slots)
		}
	}
	for _, name := range []string{"symbols", "lesson"} {
		if _, ok := note.Slots[name]; ok {
			t.Fatalf("premise: the note holds a %s slot (slots %v)", name, note.Slots)
		}
	}
}

// rv1Is reports a mismatch between the line's row and the row wanted.
func rv1Is(t *testing.T, what string, l *rv1Line, state steer.LineState, by string, level steer.Level) {
	t.Helper()
	if l == nil || l.State != string(state) || l.by() != by || l.Level != level.String() {
		t.Errorf("%s: %s; want state %s, suppressed_by %q, level %s", what, l.row(), state, by, level)
	}
}

// rv1GraphAsks are a later request naming 0, 1 and 2 files of the auth unit
// by their paths from the project root, where the hook runs. The graph reads
// a path with a directory at its index form, the path from the root, and the
// note reads it from the cwd and then from the root, so from the root both
// read the same word. The first names the file by its bare name, which the
// graph reads as the one file of that name and the note, finding no such file
// at the root, does not: the graph brief is built and the note lists no path.
var rv1GraphAsks = []struct {
	paths int
	text  string
}{
	{0, "make session expiry in sessions.py respect the org-level timeout override"},
	{1, "make session expiry in services/auth/auth/sessions.py respect the org-level timeout override"},
	{2, "make session expiry in services/auth/auth/sessions.py and services/auth/auth/db.py respect the org-level timeout override"},
}

// rv1Graphed is a session over the graph specimen with its cwd at the
// project root, the spec text bound as trusted where it is not "", after a
// first prompt that names nothing and one call, so the next prompt is a
// later one. A repository spec is looked for from the cwd up to the git
// root, by the cwd's path with its symlinks resolved, so the specimen gets a
// .git once its index is built and the spec is bound under the resolved
// path.
func rv1Graphed(t *testing.T, specText string, arm func(e *lessonEnv)) *lessonEnv {
	t.Helper()
	e := newLessonEnv(t)
	graphProject(t, e.cwd)
	project, err := filepath.EvalSymlinks(e.cwd)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if specText != "" {
		cfBindSpecText(t, e.root, project, specText)
	}
	if arm != nil {
		arm(e)
	}
	e.rv1Later("s1")
	return e
}

// rv1Later starts the session and gives it a first prompt that names nothing
// and one call.
func (e *lessonEnv) rv1Later(sessionID string) {
	e.t.Helper()
	e.start(sessionID)
	e.prompt(sessionID, "p1", "good morning")
	e.call(sessionID, "p1", "tu1")
}

// rv1ArmOff forces the steer arm off on a lab store: every print is
// withheld and every line is decided and logged as under the steer arm.
func rv1ArmOff(e *lessonEnv) {
	e.t.Setenv(hook.EnvSteerArm, "off")
	if err := store.WriteLab(e.root); err != nil {
		e.t.Fatal(err)
	}
}

// TestRV1_4a_PathsOnlyYieldsToTheGraphBrief: at a later prompt that builds
// both the graph brief and a note listing only the paths the request named,
// the graph brief is delivered as the prompt's nudge and the note is a flag
// left a candidate for the turn's end, so nothing tells the agent "paths
// named". At 0 paths the note, if any, already lists nothing for the model,
// so the brief is delivered at the base too: the row the fixture can pass.
// Under arm off nothing prints and the lines read as the steer arm decided
// them: the brief withheld by the arm, the note a candidate flag.
func TestRV1_4a_PathsOnlyYieldsToTheGraphBrief(t *testing.T) {
	for _, ask := range rv1GraphAsks {
		t.Run(fmt.Sprintf("paths_%d", ask.paths), func(t *testing.T) {
			e := rv1Graphed(t, "", nil)
			out := e.prompt("s1", "p2", ask.text)
			note, graphed := e.rv1Lines("s1", "p2")
			if graphed == nil {
				t.Fatal("premise: no task-preflight line with the graph key at p2")
			}
			if ask.paths > 0 {
				rv1Holds(t, note, ask.paths, 0, 0)
			} else if note != nil && rv1Lists(t, note, "paths") != 0 {
				t.Fatalf("premise: the 0 row's note lists paths (slots %v)", note.Slots)
			}
			t.Logf("graph line: %s; note line: %s", graphed.row(), note.row())
			rv1Is(t, "graph line", graphed, steer.LineStateDelivered, "", steer.LevelNudge)
			if note != nil {
				rv1Is(t, "note line", note, steer.LineStateCandidate, "", steer.LevelFlag)
			}
			if !strings.HasPrefix(out.context, rv1GraphHead) {
				t.Errorf("additionalContext is not the graph brief:\n%s", out.context)
			}
			if strings.Contains(out.context+out.system, "paths named:") {
				t.Errorf("a paths line printed: context %q, system %q", out.context, out.system)
			}
		})
	}
	t.Run("arm_off", func(t *testing.T) {
		e := rv1Graphed(t, "", rv1ArmOff)
		out := e.prompt("s1", "p2", rv1GraphAsks[1].text)
		note, graphed := e.rv1Lines("s1", "p2")
		if graphed == nil {
			t.Fatal("premise: no task-preflight line with the graph key at p2")
		}
		rv1Holds(t, note, 1, 0, 0)
		t.Logf("arm off: graph line: %s; paths line: %s", graphed.row(), note.row())
		if out.stdout != "" {
			t.Errorf("arm off printed %q", out.stdout)
		}
		rv1Is(t, "graph line", graphed, steer.LineStateSuppressed, string(steer.SuppressedByArmOff), steer.LevelNudge)
		rv1Is(t, "paths line", note, steer.LineStateCandidate, "", steer.LevelFlag)
	})
}

// rv1Rule and rv1Gate are the spec items 4b's rows bind: p0 covers no file
// the request names, so a spec is bound in every row and only the listed
// items differ; r1 covers the named file and r2 its directory's Python
// files; g1 is a required test gate, listed for any request but a document
// one.
const (
	rv1RuleNone     = `{"id": "p0", "kind": "protect_path", "paths": ["none/*.go"], "why": "w", "hard_control": "h"}`
	rv1RuleSessions = `{"id": "r1", "kind": "protect_path", "paths": ["services/auth/auth/sessions.py"], "why": "w", "hard_control": "h"}`
	rv1RuleAuth     = `{"id": "r2", "kind": "protect_path", "paths": ["services/auth/auth/*.py"], "why": "w", "hard_control": "h"}`
	rv1Gate         = `{"id": "g1", "kind": "test", "argv": ["go", "test", "./..."]}`
)

// rv1Spec is a spec of p0 and rules, with gates where it is not "".
func rv1Spec(gates string, rules ...string) string {
	text := `{"rashomon_spec": 1, "rules": [` + strings.Join(append([]string{rv1RuleNone}, rules...), ", ") + `]`
	if gates != "" {
		text += `, "gates": [` + gates + `]`
	}
	return text + "}"
}

// TestRV1_4b_RuleLineHoldsItsPlaceOverTheGraphBrief: where the request's
// note lists a spec rule or a gate beside its paths, the note keeps the
// prompt's nudge and the graph brief is superseded in the same call, at 1
// and 2 rules and at 1 gate. At 0 rules and 0 gates the note lists paths
// only, and the brief takes the nudge (4a's outcome, under a bound spec).
func TestRV1_4b_RuleLineHoldsItsPlaceOverTheGraphBrief(t *testing.T) {
	for _, row := range []struct {
		name         string
		spec         string
		rules, gates int
		line         string
	}{
		{"rules_0", rv1Spec(""), 0, 0, ""},
		{"rules_1", rv1Spec("", rv1RuleSessions), 1, 0, "\nspec rules that apply: "},
		{"rules_2", rv1Spec("", rv1RuleSessions, rv1RuleAuth), 2, 0, "\nspec rules that apply: "},
		{"gates_1", rv1Spec(rv1Gate), 0, 1, "\ngates before done: "},
	} {
		t.Run(row.name, func(t *testing.T) {
			e := rv1Graphed(t, row.spec, nil)
			out := e.prompt("s1", "p2", rv1GraphAsks[1].text)
			note, graphed := e.rv1Lines("s1", "p2")
			if graphed == nil {
				t.Fatal("premise: no task-preflight line with the graph key at p2")
			}
			rv1Holds(t, note, 1, row.rules, row.gates)
			t.Logf("graph line: %s; note line: %s", graphed.row(), note.row())
			if row.line == "" {
				rv1Is(t, "graph line", graphed, steer.LineStateDelivered, "", steer.LevelNudge)
				rv1Is(t, "note line", note, steer.LineStateCandidate, "", steer.LevelFlag)
				return
			}
			rv1Is(t, "note line", note, steer.LineStateDelivered, "", steer.LevelNudge)
			rv1Is(t, "graph line", graphed, steer.LineStateSuperseded, string(steer.SuppressedByBudgetBatch), steer.LevelNudge)
			if !strings.HasPrefix(out.context, rv1Head) || !strings.Contains(out.context, row.line) {
				t.Errorf("additionalContext is not the note with its %q line:\n%s", row.line, out.context)
			}
		})
	}
}

// rv1PlainFiles are files of a project with no graph index.
var rv1PlainFiles = []string{"internal/reader/read.go", "internal/reader/open.go"}

// rv1PlainAsks are a later request naming 0, 1 and 2 of rv1PlainFiles; the
// first names a path the project does not hold.
var rv1PlainAsks = []struct {
	paths int
	text  string
}{
	{0, "make the reader in internal/nosuch/read.go take a context"},
	{1, "make the reader in internal/reader/read.go take a context"},
	{2, "make the reader in internal/reader/read.go and internal/reader/open.go take a context"},
}

// rv1Plain is a session over a project with no graph index that holds
// rv1PlainFiles, after a first prompt that names nothing and one call.
func rv1Plain(t *testing.T, arm func(e *lessonEnv)) *lessonEnv {
	t.Helper()
	e := newLessonEnv(t)
	for _, rel := range rv1PlainFiles {
		p := filepath.Join(e.cwd, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("package reader\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if arm != nil {
		arm(e)
	}
	e.rv1Later("s1")
	return e
}

// TestRV1_4c_PathsOnlyAloneIsAFlagWithNoModelText: in a project with no
// graph index, a later request's note that lists only the named paths is
// delivered as a flag: the person reads the note itself, header and paths
// line first, never the wrapper that says the agent was told, and the model
// is sent nothing. At 0 paths no paths line prints at all. Under arm off
// nothing prints and the note reads as a flag the arm withheld.
func TestRV1_4c_PathsOnlyAloneIsAFlagWithNoModelText(t *testing.T) {
	for _, ask := range rv1PlainAsks {
		t.Run(fmt.Sprintf("paths_%d", ask.paths), func(t *testing.T) {
			e := rv1Plain(t, nil)
			out := e.prompt("s1", "p2", ask.text)
			note, graphed := e.rv1Lines("s1", "p2")
			if graphed != nil {
				t.Fatalf("premise: a graph line with no index: %s", graphed.row())
			}
			if out.context != "" {
				t.Errorf("the model was sent %q", out.context)
			}
			if ask.paths == 0 {
				if strings.Contains(out.system, "paths named:") {
					t.Errorf("a paths line printed for a path the project does not hold: %q", out.system)
				}
				return
			}
			rv1Holds(t, note, ask.paths, 0, 0)
			t.Logf("note line: %s", note.row())
			rv1Is(t, "note line", note, steer.LineStateDelivered, "", steer.LevelFlag)
			if !strings.HasPrefix(out.system, rv1Head) {
				t.Errorf("systemMessage does not open with the note's header and paths line:\n%s", out.system)
			}
			for _, rel := range rv1PlainFiles[:ask.paths] {
				if !strings.Contains(out.system, rel) {
					t.Errorf("systemMessage does not name %s:\n%s", rel, out.system)
				}
			}
		})
	}
	t.Run("arm_off", func(t *testing.T) {
		e := rv1Plain(t, rv1ArmOff)
		out := e.prompt("s1", "p2", rv1PlainAsks[1].text)
		note, _ := e.rv1Lines("s1", "p2")
		rv1Holds(t, note, 1, 0, 0)
		t.Logf("arm off: paths line: %s", note.row())
		if out.stdout != "" {
			t.Errorf("arm off printed %q", out.stdout)
		}
		rv1Is(t, "paths line", note, steer.LineStateSuppressed, string(steer.SuppressedByArmOff), steer.LevelFlag)
	})
}

// TestRV1_4d_DeliveredFlagPathsCountAsGiven: a paths-only note delivered as a
// flag counts its paths as given, so the next request naming the same file
// builds no note; a paths-only note left a candidate behind the graph brief
// does not, so the next request naming the same file lists it again.
func TestRV1_4d_DeliveredFlagPathsCountAsGiven(t *testing.T) {
	t.Run("delivered_flag", func(t *testing.T) {
		e := rv1Plain(t, nil)
		e.prompt("s1", "p2", rv1PlainAsks[1].text)
		note, _ := e.rv1Lines("s1", "p2")
		rv1Holds(t, note, 1, 0, 0)
		if note.State != string(steer.LineStateDelivered) {
			t.Fatalf("premise: p2's note %s, want delivered", note.row())
		}
		e.call("s1", "p2", "tu2")
		out := e.prompt("s1", "p3", rv1PlainAsks[1].text)
		if again, _ := e.rv1Lines("s1", "p3"); again != nil || strings.Contains(out.system+out.context, "paths named:") {
			t.Errorf("p3 named a path p2's delivered note gave: line %s, system %q, context %q", again.row(), out.system, out.context)
		}
	})
	t.Run("candidate", func(t *testing.T) {
		e := rv1Graphed(t, "", nil)
		e.prompt("s1", "p2", rv1GraphAsks[1].text)
		note, graphed := e.rv1Lines("s1", "p2")
		if graphed == nil {
			t.Fatal("premise: no task-preflight line with the graph key at p2")
		}
		rv1Holds(t, note, 1, 0, 0)
		e.call("s1", "p2", "tu2")
		e.prompt("s1", "p3", rv1GraphAsks[1].text)
		again, _ := e.rv1Lines("s1", "p3")
		t.Logf("p2 graph line: %s; p2 note line: %s; p3 note line: %s", graphed.row(), note.row(), again.row())
		if again == nil || rv1Lists(t, again, "paths") != 1 {
			t.Errorf("p3's note %s: want the path p2's candidate did not give listed again", again.row())
		}
	})
}
