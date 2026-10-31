package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// stcStore is a store at a fresh root, with the working directory moved to a
// fresh project directory, which it returns beside the store.
func stcStore(t *testing.T) (*store.Store, string) {
	t.Helper()
	root := t.TempDir()
	benchSetenv(t, root, t.TempDir())
	proj := t.TempDir()
	t.Chdir(proj)
	st, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	return st, proj
}

// stcDecision is a steer line a test appends.
type stcDecision struct {
	session, pattern, event, level, state, by, delivery string
	at                                                  int64
}

// stcAppend writes each decision as a steer line of its session, with a
// model text no verb may print.
func stcAppend(t *testing.T, st *store.Store, ds ...stcDecision) {
	t.Helper()
	for _, d := range ds {
		l := &steer.Line{Type: steer.LineType, V: steer.LineVersion, ID: fmt.Sprintf("%032d", d.at), SteerID: fmt.Sprintf("%032d", d.at),
			AtUnixMS: d.at, SessionID: d.session, Pattern: d.pattern, Level: d.level, Event: d.event,
			State: steer.LineState(d.state), ModelText: "NOTE-TEXT-NO-VERB-MAY-PRINT", UserText: "NOTE-TEXT-NO-VERB-MAY-PRINT"}
		if d.by != "" {
			by := steer.SuppressedBy(d.by)
			l.SuppressedBy = &by
		}
		if d.delivery != "" {
			dv := steer.Delivery(d.delivery)
			l.Delivery = &dv
		}
		if err := st.AppendSteer(d.session, func(io.ReaderAt) ([]store.SteerLine, error) { return []store.SteerLine{l}, nil }); err != nil {
			t.Fatal(err)
		}
	}
}

// stcRun is a verb's output.
func stcRun(t *testing.T, verb func([]string, io.Writer) error, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	if err := verb(args, &out); err != nil {
		t.Fatalf("%q: %v", args, err)
	}
	return out.String()
}

// stcFixture is the decisions the read verbs are tested on.
var stcFixture = []stcDecision{
	{"s1", "regression-location", "PostToolUseFailure", "nudge", "delivered", "", "delivered", 1_800_000_000_000},
	{"s1", "regression-location", "PostToolUseFailure", "nudge", "suppressed", "budget_pattern", "suppressed_budget", 1_800_000_001_000},
	{"s2", "task-preflight", "UserPromptSubmit", "flag", "delivered", "steer_off", "flag_only", 1_800_000_002_000},
	{"s2", "regression-location", "PostToolUseFailure", "flag", "delivered", "consent_missing", "flag_only", 1_800_000_003_000},
}

func TestSTC1_SteerOffUserWritesTheUsersKey(t *testing.T) {
	st, _ := stcStore(t)
	if out := stcRun(t, cmdSteer); !strings.HasPrefix(out, "steer: on (you: on; this project: on") {
		t.Errorf("bare steer = %q", out)
	}
	if out := stcRun(t, cmdSteer, "off"); !strings.HasPrefix(out, "steer: off (you: off;") {
		t.Errorf("steer off = %q", out)
	}
	c, err := store.ReadConfig(st.Root())
	if err != nil || !c.SteerOffUser() {
		t.Fatalf("config after steer off = %+v, %v", c, err)
	}
	if len(c.SteerProjects) != 0 {
		t.Errorf("steer off --user wrote a project key: %v", c.SteerProjects)
	}
	if out := stcRun(t, cmdSteer, "on", "--user"); !strings.HasPrefix(out, "steer: on (you: on;") {
		t.Errorf("steer on = %q", out)
	}
	if c, _ := store.ReadConfig(st.Root()); c.SteerOffUser() {
		t.Errorf("steer on left the user's key: %+v", c)
	}
}

func TestSTC1_SteerRefusesWhatItDoesNotKnow(t *testing.T) {
	stcStore(t)
	for _, args := range [][]string{{"off", "--user", "--project"}, {"off", "now"}, {"set"}, {"explain", "x"}, {"log", "--tail"}} {
		if err := cmdSteer(args, io.Discard); err == nil {
			t.Errorf("steer %q was accepted", args)
		}
	}
}

func TestSTC1_NoStoreHoldsNoSwitch(t *testing.T) {
	root := filepath.Join(t.TempDir(), "none")
	benchSetenv(t, root, t.TempDir())
	t.Chdir(t.TempDir())
	if err := cmdSteer([]string{"off"}, io.Discard); err == nil {
		t.Error("steer off with no store was accepted")
	}
	for _, verb := range []func([]string, io.Writer) error{cmdSteer, cmdScorecard} {
		if out := stcRun(t, verb); out == "" {
			t.Error("a read verb with no store said nothing")
		}
	}
	for _, args := range [][]string{{"explain"}, {"log"}} {
		if out := stcRun(t, cmdSteer, args...); !strings.Contains(out, "nothing has been recorded here") {
			t.Errorf("steer %v with no store = %q", args, out)
		}
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Errorf("a verb made the store root: %v", err)
	}
}

func TestSTC1b_ProjectScopeOnlyThatProject(t *testing.T) {
	st, a := stcStore(t)
	out := stcRun(t, cmdSteer, "off", "--project")
	if !strings.Contains(out, "steer: off (you: on; this project: off") {
		t.Errorf("steer off --project = %q", out)
	}
	c, err := store.ReadConfig(st.Root())
	if err != nil {
		t.Fatal(err)
	}
	// The key the hook looks the project up by: baseline.Key of the payload's cwd.
	if !c.SteerOffFor(baseline.Key(a)) || c.SteerOffUser() {
		t.Errorf("config = %+v: the project is not off, or the user is", c)
	}
	b := t.TempDir()
	t.Chdir(b)
	if c.SteerOffFor(baseline.Key(b)) {
		t.Error("another project reads off")
	}
	if out := stcRun(t, cmdSteer); !strings.Contains(out, "steer: on (you: on; this project: on") {
		t.Errorf("steer in the other project = %q", out)
	}
	t.Chdir(a)
	if out := stcRun(t, cmdSteer, "on", "--project"); !strings.Contains(out, "steer: on (you: on; this project: on") {
		t.Errorf("steer on --project = %q", out)
	}
	if c, _ := store.ReadConfig(st.Root()); c.SteerOffFor(baseline.Key(a)) || len(c.SteerProjects) != 0 {
		t.Errorf("steer on --project left %v", c.SteerProjects)
	}
}

func TestSTC1_SwitchesKeepEachOthersKeys(t *testing.T) {
	st, _ := stcStore(t)
	stcRun(t, cmdSteer, "off", "--project")
	stcRun(t, cmdSummary, "off", "--project")
	stcRun(t, cmdSummary, "off")
	stcRun(t, cmdSteer, "off")
	c, err := store.ReadConfig(st.Root())
	if err != nil {
		t.Fatal(err)
	}
	if len(c.SteerProjects) != 1 || len(c.SummaryProjects) != 1 || c.SummaryOn() || !c.SteerOffUser() {
		t.Errorf("config = %+v", c)
	}
}

func TestSTC1_SummaryScopes(t *testing.T) {
	st, proj := stcStore(t)
	key := baseline.Key(proj)
	if out := stcRun(t, cmdSummary, "off", "--project"); !strings.HasPrefix(out, "summary: off for this project (") {
		t.Errorf("summary off --project = %q", out)
	}
	c, _ := store.ReadConfig(st.Root())
	if !c.SummaryOn() || c.SummaryOnFor(key) {
		t.Errorf("project off: user on = %v, project on = %v", c.SummaryOn(), c.SummaryOnFor(key))
	}
	if summaryOn(st.Root(), key) || !summaryOn(st.Root(), "/another") {
		t.Error("the Stop line's reader does not honour the project's key alone")
	}
	if out := stcRun(t, cmdSummary); out != "summary: on\n" {
		t.Errorf("summary = %q: a project's switch changed the user's", out)
	}
	stcRun(t, cmdSummary, "on", "--project")
	if c, _ := store.ReadConfig(st.Root()); !c.SummaryOnFor(key) {
		t.Error("summary on --project did not clear the project's key")
	}
	if err := cmdSummary([]string{"off", "--user", "--project"}, io.Discard); err == nil {
		t.Error("two scopes were accepted")
	}
}

func TestSTC1c_SteerLogPrintsClosedFields(t *testing.T) {
	st, _ := stcStore(t)
	stcAppend(t, st, stcFixture...)
	stcAppend(t, st, stcDecision{"s3", "ghp_NotAPatternAnyoneKnows", "PostToolUse", "nudge", "suppressed", "invented\nreason", "shadow", 1_800_000_004_000})
	out := stcRun(t, cmdSteer, "log")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 5 {
		t.Fatalf("steer log printed %d lines:\n%s", len(lines), out)
	}
	for _, want := range []string{"2027-01-15T08:00:00Z", "session s1", "PostToolUseFailure", "regression-location", "level nudge",
		"delivered", "by budget_pattern", "delivery suppressed_budget", "by steer_off", "delivery shadow", "level flag", "by consent_missing", "delivery flag_only"} {
		if !strings.Contains(out, want) {
			t.Errorf("steer log lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "NOTE-TEXT") || strings.Contains(out, "ghp_") || strings.Contains(out, "invented") {
		t.Errorf("steer log printed text outside its closed fields:\n%s", out)
	}
	if !strings.Contains(lines[4], "?  level nudge  suppressed  by ?  delivery shadow") {
		t.Errorf("a foreign pattern and reason did not print as ?: %q", lines[4])
	}
	var j struct {
		Decisions  []map[string]any `json:"decisions"`
		Unreadable int              `json:"unreadable"`
	}
	if err := json.Unmarshal([]byte(stcRun(t, cmdSteer, "log", "--json")), &j); err != nil || len(j.Decisions) != 5 {
		t.Fatalf("steer log --json: %v, %d decisions", err, len(j.Decisions))
	}
	for _, k := range []string{"at_unix_ms", "session", "event", "pattern", "level", "state", "suppressed_by", "delivery"} {
		if _, ok := j.Decisions[0][k]; !ok {
			t.Errorf("steer log --json lacks %q", k)
		}
	}
	if len(j.Decisions[0]) != 8 {
		t.Errorf("steer log --json carries %d fields, want the 8 closed ones: %v", len(j.Decisions[0]), j.Decisions[0])
	}
}

func TestSTC1c_SteerLogIsTheLastTwentyUnlessAll(t *testing.T) {
	st, _ := stcStore(t)
	for i := range 25 {
		stcAppend(t, st, stcDecision{"s1", "regression-location", "PostToolUse", "nudge", "delivered", "", "delivered", 1_800_000_000_000 + int64(i)*1000})
	}
	if n := strings.Count(stcRun(t, cmdSteer, "log"), "\n"); n != logTail {
		t.Errorf("steer log printed %d lines, want %d", n, logTail)
	}
	if n := strings.Count(stcRun(t, cmdSteer, "log", "--all"), "\n"); n != 25 {
		t.Errorf("steer log --all printed %d lines, want 25", n)
	}
}

func TestSTC1d_ScorecardCountsByPattern(t *testing.T) {
	st, _ := stcStore(t)
	stcAppend(t, st, stcFixture...)
	stcAppend(t, st,
		stcDecision{"s2", "regression-location", "PostToolUse", "nudge", "delivered", "", "holdout", 1_800_000_005_000},
		// No delivery recorded: a suppressed line still counts as suppressed, a candidate as other.
		stcDecision{"s2", "task-preflight", "PostToolUse", "nudge", "suppressed", "no_channel", "", 1_800_000_006_000},
		stcDecision{"s2", "task-preflight", "PostToolUse", "nudge", "candidate", "", "", 1_800_000_007_000})
	out := stcRun(t, cmdScorecard)
	if strings.Contains(out, "NOTE-TEXT") {
		t.Errorf("scorecard printed note text:\n%s", out)
	}
	var sc scorecard
	if err := json.Unmarshal([]byte(stcRun(t, cmdScorecard, "--json")), &sc); err != nil {
		t.Fatal(err)
	}
	want := map[string]scoreRow{
		"regression-location": {Pattern: "regression-location", Decisions: 4, Delivered: 1, FlagOnly: 1, Holdout: 1, Suppressed: 1},
		"task-preflight":      {Pattern: "task-preflight", Decisions: 3, FlagOnly: 1, Suppressed: 1, Other: 1},
	}
	if sc.Decisions != 7 || len(sc.ByPattern) != len(want) {
		t.Fatalf("scorecard = %+v", sc)
	}
	for _, r := range sc.ByPattern {
		if r != want[r.Pattern] {
			t.Errorf("%s = %+v, want %+v", r.Pattern, r, want[r.Pattern])
		}
	}
	if sc.ByPattern[0].Pattern != "regression-location" {
		t.Errorf("rows are not most decisions first: %+v", sc.ByPattern)
	}
	if sc.SuppressedBy["steer_off"] != 1 || sc.SuppressedBy["budget_pattern"] != 1 || sc.SuppressedBy["consent_missing"] != 1 {
		t.Errorf("suppressed_by = %v", sc.SuppressedBy)
	}
	if !strings.Contains(out, "scorecard: 7 decisions") || !strings.Contains(out, "regression-location") || !strings.Contains(out, "suppressed by: ") {
		t.Errorf("scorecard text:\n%s", out)
	}
	if err := cmdScorecard([]string{"--all"}, io.Discard); err == nil {
		t.Error("scorecard --all was accepted")
	}
}

func TestSTC1e_ExplainNamesTheLastDecision(t *testing.T) {
	st, _ := stcStore(t)
	stcAppend(t, st, stcFixture[:3]...)
	out := stcRun(t, cmdSteer, "explain")
	for _, want := range []string{"last decision: task-preflight, session s2", "state:         delivered", "suppressed_by: steer_off",
		"delivery:      flag_only", "shown to you as a flag, because you turned the notes to your agent off", "nothing reached the agent"} {
		if !strings.Contains(out, want) {
			t.Errorf("steer explain lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "NOTE-TEXT") {
		t.Errorf("steer explain printed note text:\n%s", out)
	}
	// A later delivered note is the last decision, and says who was told.
	stcAppend(t, st, stcDecision{"s2", "regression-location", "PostToolUse", "nudge", "delivered", "", "delivered", 1_800_000_009_000})
	if out := stcRun(t, cmdSteer, "explain"); !strings.Contains(out, "last decision: regression-location") || !strings.Contains(out, "the agent was told") {
		t.Errorf("steer explain after a delivery:\n%s", out)
	}
}

func TestSTC1e_EveryReasonIsExplained(t *testing.T) {
	for _, by := range steer.AllSuppressedBy() {
		if reasonMeaning[by] == "" {
			t.Errorf("suppressed_by %q has no sentence", by)
		}
	}
	if len(reasonMeaning) != len(steer.AllSuppressedBy()) {
		t.Errorf("%d sentences for %d reasons", len(reasonMeaning), len(steer.AllSuppressedBy()))
	}
}

func TestSTC1k_ReadVerbsWriteNothing(t *testing.T) {
	st, _ := stcStore(t)
	stcAppend(t, st, stcFixture...)
	before := stcTree(t, st.Root())
	for _, args := range [][]string{{}, {"explain"}, {"log"}, {"log", "--all", "--json"}} {
		stcRun(t, cmdSteer, args...)
	}
	stcRun(t, cmdScorecard)
	stcRun(t, cmdScorecard, "--json")
	stcRun(t, cmdSummary)
	if after := stcTree(t, st.Root()); after != before {
		t.Errorf("a read verb changed the store:\nbefore %s\nafter  %s", before, after)
	}
}

// stcTree is every file under root with its size and mtime.
func stcTree(t *testing.T, root string) string {
	t.Helper()
	var b strings.Builder
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		fmt.Fprintf(&b, "%s %d %d;", p, info.Size(), info.ModTime().UnixNano())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestSTC1_VerbsAreDispatchedAndHaveUsage(t *testing.T) {
	for _, v := range []string{"steer", "scorecard", "summary"} {
		if verbNamed(v) == nil {
			t.Errorf("verbNamed(%q) is nil", v)
		}
	}
	for _, line := range []string{"rashomon steer [off|on] [--user|--project]", "rashomon steer explain", "rashomon steer log [--all] [--json]", "rashomon scorecard [--json]"} {
		if !strings.Contains(verbUsage, line) {
			t.Errorf("verbUsage lacks %q", line)
		}
	}
}
