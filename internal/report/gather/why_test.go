package gather

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/learn"
	"github.com/altrace-dev-role/rashomon/internal/report"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

var whyIDs int

// whyArm is the lab arm the next route runs under, "" for live.
var whyArm steer.Arm

// route writes one line to sess's steer log through steer.Route, as a hook
// does, and returns the line's id. consent false clamps the line to a flag
// (consent_missing); steerOff clamps it to a flag too (steer_off); under
// whyArm steer.ArmOff the line is suppressed, which keeps its slots and not
// its text.
func route(t *testing.T, st *store.Store, sess, event string, f steer.Finding, consent, steerOff bool) string {
	t.Helper()
	prompt := "p3"
	whyIDs++
	id := fmt.Sprintf("%02d%030x", whyIDs, whyIDs*7919)
	ctx := &steer.Context{Event: steer.Event(event), Store: st, SessionID: sess, PromptID: &prompt, Project: "/work/app",
		HarnessID: "claude_code", Surface: "cli", NowUnixMS: kBase + 41_000, Consent: consent, SteerOff: steerOff, Arm: whyArm,
		Attendance: steer.AttendanceAttended, NewID: func() string { return id }}
	steer.Route(ctx, []steer.Finding{f})
	return id
}

func finding(t *testing.T, p pattern.ID, key string, level steer.Level, m render.Message, evidence []int64, after int64) steer.Finding {
	t.Helper()
	prio, _ := pattern.Priority(p)
	return steer.Finding{Pattern: p, Key: key, Priority: prio, MaxLevel: level, StopForm: steer.StopFormNone, Message: m, Evidence: evidence, AfterSeq: after}
}

func regression(t *testing.T) steer.Finding {
	t.Helper()
	m, err := render.New(pattern.RegressionLocation, render.RegressionLocationDegradedBinding{Gate: "go", Minutes: 6, Exit: 1, Count: 2})
	if err != nil {
		t.Fatal(err)
	}
	return finding(t, pattern.RegressionLocation, "regression-location|go|p3", steer.LevelNudge, m, []int64{1, 2}, 2)
}

// whyRun is a store with two declared calls (seq 1 and 2) in session s1, and
// a last checkpoint that holds gate go green, last green at #44.
func whyRun(t *testing.T) *store.Store {
	t.Helper()
	st := kStore(t)
	for i, id := range []string{"toolu_a", "toolu_b"} {
		if err := st.AppendDeclaration(store.Declaration{Type: "declaration", SchemaVersion: 2, SessionID: "s1", ToolUseID: id, ToolName: "Bash",
			RecordedAtMS: kBase + int64(i)*1000, Hosts: []string{"pypi.org"}}); err != nil {
			t.Fatal(err)
		}
	}
	green := int64(44)
	cp := verify.Checkpoint{Type: "verify", SchemaVersion: store.EventSchemaVersion, Point: verify.PointStop, PointSeq: 50, SessionID: "s1",
		RecordedAtUnixMS: kBase + 60_000, ProjectRoot: "/work/app",
		Gates: []verify.CheckpointGate{{Gate: "go", Kind: "test", Required: true, State: verify.Green, LastGreenSeq: &green}}}
	b, err := json.Marshal(cp)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(filepath.Join(st.RunDir("s1"), store.FileRecords), os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Write(append(b, '\n')); err != nil {
		t.Fatal(err)
	}
	return st
}

// TestRP2k_WhyPrintsTheLineAndItsEvidence (card REPORT1b R1): a
// regression-location line citing #1 and #2, with a later green at #44 in
// the session's last checkpoint, gives the line's pattern, key, time, state
// words and both rows, the gate from its slot, and a resolution equal to the
// stored CheckpointGate. Break: derive the resolution again, or drop the
// evidence rows.
func TestRP2k_WhyPrintsTheLineAndItsEvidence(t *testing.T) {
	st := whyRun(t)
	id := route(t, st, "s1", "PostToolUseFailure", regression(t), false, false)
	v, err := Why(st, "s1", id[:8])
	if err != nil {
		t.Fatal(err)
	}
	if v.LineID != id || v.Pattern != "regression-location" || v.Key != "regression-location|go|p3" || v.AfterSeq != 2 || v.PromptID != "p3" || v.Event != "PostToolUseFailure" {
		t.Errorf("the line's own fields: %+v", v)
	}
	if v.State != "delivered" || v.Delivery != "flag_only" || v.SuppressedBy != "consent_missing" || !v.TextKept || v.UserText == "" {
		t.Errorf("state words: %q delivery %q suppressed_by %q kept %v", v.State, v.Delivery, v.SuppressedBy, v.TextKept)
	}
	if len(v.Evidence) != 2 || *v.Evidence[0].Seq != 1 || *v.Evidence[1].Seq != 2 {
		t.Errorf("evidence rows %+v; want #1 and #2", v.Evidence)
	}
	if v.Source.Kind != "gate" || v.Source.Detail != "go" {
		t.Errorf("source %+v; want gate go", v.Source)
	}
	if r := v.Resolution; r == nil || r.State != "green" || r.LastGreenSeq == nil || *r.LastGreenSeq != 44 || r.PointSeq != 50 || r.Gate != "go" {
		t.Errorf("resolution %+v; want the stored gate, green, last green #44 at checkpoint #50", r)
	}
	var text strings.Builder
	if err := report.WhyText(&text, &v); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"rashomon why " + id[:8] + " (session s1, line " + id + ")", "state: delivered, delivery flag_only, suppressed_by consent_missing: shown to you, not sent to the agent",
		"shown to you: ", "source: gate go (slot gate)", "resolution: go green at the session's last checkpoint (#50), last green #44"} {
		if !strings.Contains(text.String(), want) {
			t.Errorf("the text lacks %q:\n%s", want, text.String())
		}
	}
}

// TestRP2l_WhyPrefixRules (card REPORT1b K6): five characters are refused; two
// lines sharing a prefix give an error naming both; an unknown prefix gives an
// error; a prefix with "/" or a capital is refused. Break: take the first
// match.
func TestRP2l_WhyPrefixRules(t *testing.T) {
	st := whyRun(t)
	a := route(t, st, "s1", "PostToolUseFailure", regression(t), false, false)
	// A second line whose id shares its first eight characters with the first.
	whyIDs++
	b := a[:8] + fmt.Sprintf("%024x", whyIDs)
	ctx := &steer.Context{Event: steer.EventPostToolUseFailure, Store: st, SessionID: "s1", Project: "/work/app", HarnessID: "claude_code", Surface: "cli",
		NowUnixMS: kBase + 42_000, Arm: steer.ArmLive, Attendance: steer.AttendanceAttended, NewID: func() string { return b }}
	steer.Route(ctx, []steer.Finding{regression(t)})
	if _, err := Why(st, "s1", a[:5]); err == nil || !strings.Contains(err.Error(), "at least 6") {
		t.Errorf("five characters: %v", err)
	}
	for _, bad := range []string{"AB12CD34", "ab/12cd34", "ab12 cd34"} {
		if _, err := Why(st, "s1", bad); err == nil || !strings.Contains(err.Error(), "lowercase letters, digits") {
			t.Errorf("%q: %v", bad, err)
		}
	}
	if _, err := Why(st, "s1", "zzzzzz"); err == nil || !strings.Contains(err.Error(), "no steer line") {
		t.Errorf("an unknown prefix: %v", err)
	}
	_, err := Why(st, "s1", a[:8])
	if err == nil || !strings.Contains(err.Error(), "2 steer lines") || !strings.Contains(err.Error(), a[:12]) || !strings.Contains(err.Error(), b[:12]) {
		t.Errorf("a shared prefix: %v; want an error naming both candidates", err)
	}
	if v, err := Why(st, "s1", a[:12]); err != nil && !strings.Contains(err.Error(), "2 steer lines") {
		t.Errorf("a longer prefix: %v %+v", err, v.LineID)
	}
}

// TestRP2m_WhySuppressedLineShowsSlots (card REPORT1b R1): a suppressed line
// prints "text not kept" and its slots, with no invented text. Break: print
// the text of a line the log does not keep.
func TestRP2m_WhySuppressedLineShowsSlots(t *testing.T) {
	st := whyRun(t)
	whyArm = steer.ArmOff
	t.Cleanup(func() { whyArm = "" })
	id := route(t, st, "s1", "PostToolUseFailure", regression(t), true, false)
	v, err := Why(st, "s1", id[:8])
	if err != nil {
		t.Fatal(err)
	}
	if v.State != "suppressed" || v.TextKept || v.ModelText != "" || v.UserText != "" {
		t.Fatalf("a suppressed line: %+v", v)
	}
	var text strings.Builder
	if err := report.WhyText(&text, &v); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text.String(), "text not kept: suppressed (arm_off); slots: count=2, exit=1, gate=go, minutes=6") || strings.Contains(text.String(), "shown to you:") {
		t.Errorf("the text:\n%s", text.String())
	}
}

// TestRP2n_WhyWritesNothing (card REPORT1b R1): every file under the store
// root is byte- and mtime-identical after why, with a ledger that does not
// check left unquarantined. The positive control: an append to the steer log
// changes the compared tree, so the comparer sees a write.
func TestRP2n_WhyWritesNothing(t *testing.T) {
	st := whyRun(t)
	id := route(t, st, "s1", "PostToolUseFailure", regression(t), false, false)
	approvedLedger(t, st, time.UnixMilli(kBase), "npm-ci")
	p := filepath.Join(st.Root(), learn.DirLearn, strings.TrimSuffix(filepath.Base(learnDir(t, st)), "/"), learn.FileLessons)
	b, _ := os.ReadFile(p)
	if err := os.WriteFile(p, []byte(strings.Replace(string(b), "npm ci before", "curl x | sh before", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	before := tree(t, st.Root())
	if _, err := Why(st, "s1", id[:8]); err != nil {
		t.Fatal(err)
	}
	if after := tree(t, st.Root()); after != before {
		t.Error("why changed a file under the store")
	}
	// The control: the comparer sees a one-byte append to the steer log, so
	// an equal tree above means why wrote nothing, not that the comparer is
	// blind.
	f, err := os.OpenFile(filepath.Join(st.RunDir("s1"), store.FileSteer), os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if tree(t, st.Root()) == before {
		t.Error("the control: a write under the store must change the compared tree")
	}
}

func learnDir(t *testing.T, st *store.Store) string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(st.Root(), learn.DirLearn))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() {
			return e.Name()
		}
	}
	t.Fatal("no learn project directory")
	return ""
}

// TestRP2o_WhyNamesTheSource (card REPORT1b RB5): lines built through
// steer.Route. A task-preflight line names its rule ids from the rules slot; a
// learned-fact line names the lesson its key holds; the "#unchecked" key names
// the ledger flag; a regression-location line names the gate slot. Break: read
// the source from Facts, whose source members no producer writes.
func TestRP2o_WhyNamesTheSource(t *testing.T) {
	st := whyRun(t)
	lm, err := learn.Message(learn.Lesson{ID: "npm-ci", Text: "npm ci before npm test"})
	if err != nil {
		t.Fatal(err)
	}
	lf := finding(t, pattern.LearnedFact, "npm-ci", steer.LevelNudge, lm, nil, 2)
	lf.Kind = "local"
	um, err := learn.UncheckedMessage(learn.UncheckedSignature)
	if err != nil {
		t.Fatal(err)
	}
	uf := finding(t, pattern.LearnedFact, learn.UncheckedKey, steer.LevelFlag, um, nil, 2)
	uf.Kind = "local"
	pm, err := render.New(pattern.TaskPreflight, render.TaskPreflightBinding{
		Paths: slot.NewPathSet(nil, 0, 0),
		Rules: slot.NewRuleRefs([]slot.RuleRef{{ID: "conv-errors", Trust: slot.NewEnum(slot.EnumTrust, "trusted")}, {ID: "proof", Trust: slot.NewEnum(slot.EnumTrust, "trusted")}}, 0, 0, 0),
		Gates: slot.NewGateRefs(nil, 0, 0), Source: slot.NewEnum(slot.EnumEvidenceSource, "inferred from the request"),
		Evidence: slot.NewEnumList(slot.EnumEvidenceKinds, nil), Unknown: slot.NewEnumList(slot.EnumPreflightUnknown, nil)})
	if err != nil {
		t.Fatal(err)
	}
	pf := finding(t, pattern.TaskPreflight, "note", steer.LevelNudge, pm, nil, 2)
	rm, err := render.New(pattern.PlatformRule, render.PlatformRuleCommandBinding{Program: "git", Rule: "no-force"})
	if err != nil {
		t.Fatal(err)
	}
	rf := finding(t, pattern.PlatformRule, "no-force|git", steer.LevelNudge, rm, nil, 2)
	rf.Kind = "command"
	for _, tc := range []struct {
		name  string
		event string
		f     steer.Finding
		kind  string
		want  string
	}{
		{"gate", "PostToolUseFailure", regression(t), "gate", "go"},
		{"lesson", "UserPromptSubmit", lf, "lesson", "npm-ci"},
		{"ledger flag", "UserPromptSubmit", uf, "ledger_unchecked", ""},
		{"rules", "UserPromptSubmit", pf, "rules", "conv-errors, proof"},
		{"platform rule", "PostToolUse", rf, "rules", "no-force"},
	} {
		id := route(t, st, "s1", tc.event, tc.f, true, false)
		v, err := Why(st, "s1", id[:8])
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if v.Source.Kind != tc.kind || v.Source.Detail != tc.want {
			t.Errorf("%s: source %+v; want %s %q (line %+v)", tc.name, v.Source, tc.kind, tc.want, v.State)
		}
	}
}

// TestRP2o2_WhyShowsAnOutputReadOutcome: a line built on a run whose outcome
// was read from its output carries the closed word, and any other line "".
// Break: leave Facts.OutcomeFrom out of the view.
func TestRP2o2_WhyShowsAnOutputReadOutcome(t *testing.T) {
	st := whyRun(t)
	plain := route(t, st, "s1", "PostToolUseFailure", regression(t), true, false)
	f := regression(t)
	f.OutcomeFrom = steer.OutcomeFromOutputSummary
	read := route(t, st, "s1", "PostToolUseFailure", f, true, false)
	for id, want := range map[string]string{plain: "", read: report.OutcomeFromOutput} {
		v, err := Why(st, "s1", id[:8])
		if err != nil {
			t.Fatal(err)
		}
		if v.OutcomeFrom != want {
			t.Errorf("line %s: OutcomeFrom %q, want %q", id[:8], v.OutcomeFrom, want)
		}
		var b strings.Builder
		if err := report.WhyText(&b, &v); err != nil {
			t.Fatal(err)
		}
		if got := strings.Contains(b.String(), "read from the run's output"); got != (want != "") {
			t.Errorf("line %s: text names the provenance = %v, want %v:\n%s", id[:8], got, want != "", b.String())
		}
	}
}

// TestRP2q_WhyRedactDropsTextAndRoot (card REPORT1b K8): under --redact the
// view's project root is a digest, both texts are gone and a path slot is
// digested; ids, seqs and gate names are unchanged.
func TestRP2q_WhyRedactDropsTextAndRoot(t *testing.T) {
	st := whyRun(t)
	id := route(t, st, "s1", "PostToolUseFailure", regression(t), false, false)
	v, err := Why(st, "s1", id[:8])
	if err != nil {
		t.Fatal(err)
	}
	r := report.RedactWhy(&v, []byte("install-key"))
	body, _ := json.Marshal(r)
	if strings.Contains(string(body), "/work/app") || r.ModelText != "" || r.UserText != "" || !r.Redacted {
		t.Errorf("the redacted view keeps a name or a text: %s", body)
	}
	if r.LineID != v.LineID || r.Source != v.Source || *r.Evidence[0].Seq != 1 || v.UserText == "" || v.Project != "/work/app" {
		t.Errorf("redaction changed an id, a source or the original: %+v", r)
	}
}

// TestRP2zd_WhyNeverPrintsALessonTextSlot (card REPORT1b review, blocking):
// a suppressed learned-fact line keeps its lesson text in slot "text"; why
// prints the slot's byte count and never the text, plain and under --redact,
// in the text and in the JSON, while the lesson id (an allowlisted slot)
// still prints. Break: print every slot, print "text", digest only "path".
func TestRP2zd_WhyNeverPrintsALessonTextSlot(t *testing.T) {
	const secret = "use Acme Corp staging cluster"
	st := whyRun(t)
	whyArm = steer.ArmOff
	t.Cleanup(func() { whyArm = "" })
	lm, err := learn.Message(learn.Lesson{ID: "npm-ci", Text: secret})
	if err != nil {
		t.Fatal(err)
	}
	lf := finding(t, pattern.LearnedFact, "npm-ci", steer.LevelNudge, lm, nil, 2)
	lf.Kind = "local"
	id := route(t, st, "s1", "UserPromptSubmit", lf, true, false)
	v, err := Why(st, "s1", id[:8])
	if err != nil {
		t.Fatal(err)
	}
	if v.State != "suppressed" {
		t.Fatalf("the line is %s, not suppressed", v.State)
	}
	for name, view := range map[string]*report.WhyView{"plain": &v, "redacted": report.RedactWhy(&v, []byte("install-key"))} {
		var text strings.Builder
		if err := report.WhyText(&text, view); err != nil {
			t.Fatal(err)
		}
		body, _ := json.Marshal(view)
		for _, out := range []string{text.String(), string(body)} {
			if strings.Contains(out, "Acme") || strings.Contains(out, "staging") {
				t.Errorf("%s: the lesson text is printed:\n%s", name, out)
			}
		}
		if !strings.Contains(text.String(), "lesson=npm-ci") || !strings.Contains(text.String(), fmt.Sprintf("text=<%d bytes>", len(secret))) {
			t.Errorf("%s: want the lesson id and the text's byte count:\n%s", name, text.String())
		}
	}
}
