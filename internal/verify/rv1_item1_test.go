package verify

import (
	"slices"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// A final message's done claim is read only where the main agent acted in
// the turn the Stop closes. A call acts for this when it is the main agent's
// (no AgentID), in this prompt, at or before the Stop, of a class that writes
// or runs (write, execute, test, version control or package, a line that may
// write, or a record before may_write, which cannot say), and not proven
// outside every project:
// a project the record does not say counts (fail closed). An empty prompt id
// reads the turn as acted, as before. The session's writes still decide
// whether done is read at all (WroteRelevant); this narrows it to the turns
// that acted. Only the done class is narrowed: the other classes are read on
// every turn, as before.

// rv1Said is the final message of the done rows: one strict done claim and
// no other class.
const rv1Said = "The fix is done."

// Where a row's second-turn calls ran: in the repository at v5, with nothing
// recorded (v4 withholds the project and the cwd), or proven outside every
// repository.
const (
	rv1Repo    = "repo"
	rv1V4      = "v4"
	rv1Outside = "outside"
)

// rv1Session is a session whose prompt p1 edits a file go test covers, so
// the session has written a relevant file before prompt p2, which makes
// calls. Each call runs where place says; p1's edit runs in the repository
// unless place is v4.
func rv1Session(t *testing.T, place string, calls ...call) *store.Run {
	t.Helper()
	r := runOf(t, append([]call{edit(1, "/repo/internal/a.go")}, calls...)...)
	if place != rv1V4 {
		scAt(t, r, 1, "/repo", "/repo")
	}
	scPrompt(r, "p1", 1)
	for _, c := range calls {
		switch place {
		case rv1Repo:
			scAt(t, r, c.seq, "/repo", "/repo")
		case rv1Outside:
			scAt(t, r, c.seq, "", "/repo")
		}
		scPrompt(r, "p2", c.seq)
	}
	return r
}

// rv1Read reports a claim of class in the Stop's scan.
func rv1Read(v StopView, class ClaimClass) bool {
	return slices.ContainsFunc(v.Scan.Claims, func(c ClaimVerdict) bool { return c.Class == class })
}

// TestRV1_1a_ClaimsReadOnlyWhereTheMainAgentActed: after an earlier turn's
// covered edit, prompt p2's final message says "The fix is done."; the done
// claim is read only where p2's own calls acted. A coordinator's relays, a
// search, an ls, a subagent's edit and an edit proven outside every
// repository act on no repository in this turn; an Edit, a go test run, an
// unrecognised program (verb.go: execute), a git commit and a go build
// (verb.go: vcs and package, classes that cannot say the line wrote nothing),
// a read-class line that may write (a redirect), an edit whose project the
// record does not say (fail closed) and a record before may_write (it cannot
// say) do. The last row is the limit as ruled: a relay's tests-pass claim is
// still read. Break (RV-1_item1.py): drop the AgentID filter (the subagent
// row), read acts alone with no class or project filter (the coordinator,
// search, ls and outside rows), drop the PromptID filter (the text-only row),
// drop any one class (write, execute, test, or vcs and package: the Edit,
// program, go test, git and go build rows), drop may_write (the redirect
// row) or the record-before-may_write clause, count an act proven in no
// project or refuse one the record does not place, or read an empty prompt
// id as no act.
func TestRV1_1a_ClaimsReadOnlyWhereTheMainAgentActed(t *testing.T) {
	coordinator := []call{
		{seq: 2, tool: "SendMessage", class: shape.VerbUnknown, outcome: store.ExecOK},
		{seq: 3, tool: "Agent", class: shape.VerbAgent, outcome: store.ExecOK},
		{seq: 4, tool: "Workflow", class: shape.VerbUnknown, outcome: store.ExecOK},
	}
	subEdit := call{seq: 2, tool: "Edit", class: shape.VerbWrite, outcome: store.ExecOK, paths: []string{"/repo/internal/b.go"}, agent: "a1"}
	search := call{seq: 2, tool: "WebSearch", class: shape.VerbNetwork, outcome: store.ExecOK}
	ls := call{seq: 2, tool: "Bash", prog: "ls", class: shape.VerbRead, outcome: store.ExecOK}
	unknown := call{seq: 2, tool: "Bash", prog: "frobnicate", class: shape.VerbExecute, outcome: store.ExecOK}
	commit := call{seq: 2, tool: "Bash", prog: "git", class: shape.VerbVCS, outcome: store.ExecOK}
	build := call{seq: 2, tool: "Bash", prog: "go", class: shape.VerbPackage, outcome: store.ExecOK}
	mw := call{seq: 2, tool: "Bash", prog: "cat", class: shape.VerbRead, outcome: store.ExecOK, mayWrite: true}
	for _, c := range []struct {
		name, place, prompt, said string
		calls                     []call
		schema2                   bool
		class                     ClaimClass
		read                      bool
	}{
		{"a coordinator in a repository: SendMessage, Agent and Workflow", rv1Repo, "p2", rv1Said, coordinator, false, ClaimDone, false},
		{"the same coordinator at v4, where no project is recorded", rv1V4, "p2", rv1Said, coordinator, false, ClaimDone, false},
		{"a text-only turn after an earlier turn's edit", rv1Repo, "p2", rv1Said, nil, false, ClaimDone, false},
		{"a subagent's edit only", rv1Repo, "p2", rv1Said, []call{subEdit}, false, ClaimDone, false},
		{"a WebSearch only", rv1Repo, "p2", rv1Said, []call{search}, false, ClaimDone, false},
		{"an ls only", rv1Repo, "p2", rv1Said, []call{ls}, false, ClaimDone, false},
		{"an edit proven outside every repository", rv1Outside, "p2", rv1Said, []call{edit(2, "/ev/notes.txt")}, false, ClaimDone, false},
		{"an Edit", rv1Repo, "p2", rv1Said, []call{edit(2, "/repo/internal/b.go")}, false, ClaimDone, true},
		{"a go test run", rv1Repo, "p2", rv1Said, []call{gateCall(2, "go-test", store.ExecOK)}, false, ClaimDone, true},
		{"an unrecognised program", rv1Repo, "p2", rv1Said, []call{unknown}, false, ClaimDone, true},
		{"a git commit", rv1Repo, "p2", rv1Said, []call{commit}, false, ClaimDone, true},
		{"a go build", rv1Repo, "p2", rv1Said, []call{build}, false, ClaimDone, true},
		{"a read-class line that may write", rv1Repo, "p2", rv1Said, []call{mw}, false, ClaimDone, true},
		{"an Edit whose project the record does not say", rv1V4, "p2", rv1Said, []call{edit(2, "/repo/internal/b.go")}, false, ClaimDone, true},
		{"a Read in a record before may_write", rv1Repo, "p2", rv1Said, []call{scRead(2)}, true, ClaimDone, true},
		{"a text-only turn with no prompt id", rv1Repo, "", rv1Said, nil, false, ClaimDone, true},
		{"limit: a coordinator's tests-pass claim is still read", rv1Repo, "p2", "All tests pass.", coordinator, false, ClaimTestsPass, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := rv1Session(t, c.place, c.calls...)
			if c.schema2 {
				// runOf stamps call schema 5 and up; a record before
				// may_write (schema 3) is set here, after the build.
				r.Declarations[1].SchemaVersion = 2
			}
			v := scStop(r, c.prompt, c.said, "/repo", goTestAll)
			if got := rv1Read(v, c.class); got != c.read {
				t.Errorf("%s claim read %t, want %t; claims %+v", c.class, got, c.read, v.Scan.Claims)
			}
		})
	}
}

// TestRV1_1b_OwesNothingFollowsTheTurn (STALE-COVER1's fixture 1 under this
// rule): a turn wrote one note outside every repository and said "QA-1's run
// is complete." Alone, the turn acted on no repository: the claim is not
// read and nothing is flagged. Beside an unrecognised program run in the
// repository it acted: the claim is read and answers to no gate, since go
// test never ran and covers nothing the turn wrote (owesNothing), so nothing
// is flagged either. Control: the note in the repository is a covered write
// and the claim is flagged. Break: read a gate that owes nothing as owed, or
// read the claim on a turn that acted on no repository.
func TestRV1_1b_OwesNothingFollowsTheTurn(t *testing.T) {
	const said = "QA-1's run is complete."
	run := call{seq: 3, tool: "Bash", prog: "frobnicate", class: shape.VerbExecute, outcome: store.ExecOK}
	for _, c := range []struct {
		name, path, project  string
		acted, read, flagged bool
	}{
		{"the note alone, outside every repository", "/ev/verdicts/PR-170.txt", "", false, false, false},
		{"the note outside, beside a program run in the repository", "/ev/verdicts/PR-170.txt", "", true, true, false},
		{"control: the note in the repository", "/repo/verdicts/PR-170.txt", "/repo", false, true, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			calls := []call{scRead(1), scWrite(2, c.path)}
			if c.acted {
				calls = append(calls, run)
			}
			r := runOf(t, calls...)
			ranIn(r, "/repo", "")
			scAt(t, r, 1, "/repo", "/repo")
			scAt(t, r, 2, c.project, "/repo")
			if c.acted {
				scAt(t, r, 3, "/repo", "/repo")
			}
			v := scStop(inPrompt(r, "p1"), "p1", said, "/repo", goTestAll)
			read := rv1Read(v, ClaimDone)
			if read != c.read || read && scFlagged(t, v) != c.flagged {
				t.Errorf("done read %t (want %t), flagged %t (want %t); claims %+v, gates %+v", read, c.read,
					read && scFlagged(t, v), c.flagged, v.Scan.Claims, v.Snap.Gates)
			}
		})
	}
}

// TestRV1_1c_ExpectedKeepsTheSessionsWrite: the rule narrows the claim scan
// only. On a text-only turn after an earlier turn's edit, under a task that
// expects a stated mechanism, the done claim is not read, and the
// expected-evidence row still reads the session's write (evidence.go,
// mechanismRow): met, not open. Break (mutant M1-4): hand the narrowed input
// to expected as well.
func TestRV1_1c_ExpectedKeepsTheSessionsWrite(t *testing.T) {
	r := rv1Session(t, rv1Repo)
	sp := evSpec([]string{"mechanism_stated"}, "the cache key omits the locale")
	pt := Point{Kind: PointStop, Seq: lastSeqOf(r), SessionID: "s1", PromptID: ptr("p2")}
	v := EvaluateStop(r, nil, sp, pt, time.Time{}, StopInput{PromptID: "p2", Key: dgKey, ProjectKey: "/repo",
		Root: "/repo", Inferred: goTestAll, Head: []byte(rv1Said)})
	if read := rv1Read(v, ClaimDone); read || len(v.Expected) != 1 || v.Expected[0].State != ObligationMet {
		t.Errorf("done read %t, expected %+v; want not read and one met row", read, v.Expected)
	}
}
