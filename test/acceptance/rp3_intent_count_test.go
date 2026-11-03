package acceptance

// REPORT-REQS1: the session report's count line, "mistakes against intent: k
// caught at the act, p reached the person, r found later" (INTENT-MISTAKES),
// read from what the rows record and never a score. A mistake the record
// holds is a finding line, delivered or withheld; each is classified once, by
// the first rule that applies, and whether the agent was told is read from
// delivery, never from the finding's state: delivered at a Stop, it reached
// the person (the Stop speaks to the person alone); naming no gate, the record
// cannot settle it; told and then resolved by a green run, it was caught at
// the act; withheld and then resolved, it was fixed with no line; told, open
// at the last Stop and counted by the clause the Stop printed, it reached the
// person; otherwise it was missed. Nothing records a later find yet
// (LOOP-TRACE1), so "found later" reads unknown, never 0.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// rp3Line appends one finding line to sess-1's steer log: its id, the event
// that carried it, its pattern, the gate it names ("" for none, with no slot
// and no key) and its evidence seq. withheld "" delivers it; "holdout" keeps
// it delivered with neither text, as decide's holdOut writes one; any other
// value is the reason it was suppressed (the prompt's budget, or a client
// below the event's floor), with no model text.
func (e *env) rp3Line(t *testing.T, id, event, pat, gate string, seq int, withheld string) {
	t.Helper()
	l := map[string]any{"type": "steer", "id": id, "state": "delivered", "prompt_id": "prompt-2", "event": event,
		"pattern": pat, "level": "nudge", "model_text": "note " + id, "after_seq": seq, "evidence_seqs": []int{seq}}
	switch withheld {
	case "":
	case "holdout":
		l["model_text"], l["user_text"], l["delivery"] = "", "", "holdout"
	default:
		l["state"], l["suppressed_by"], l["model_text"] = "suppressed", withheld, ""
	}
	if gate != "" {
		l["slots"], l["key"] = map[string]any{"gate": gate}, e.rpGateID(t)
	}
	b, err := json.Marshal(l)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(filepath.Join(e.home, "runs", testSession, "steer.ndjson"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Write(append(b, '\n')); err != nil {
		t.Fatal(err)
	}
}

// rp3Session is rp1's session (a failed go test, its finding, a green turn)
// with five more findings: one delivered at a Stop that the later green would
// resolve, one withheld by the budget that it does resolve, one naming no
// gate, and three after the last green: one told, one withheld below the
// event's floor and one held out. The Stop then prints its clause, which
// counts the told one.
func (e *env) rp3Session(t *testing.T) {
	t.Helper()
	e.watched(testSession)
	e.rpCall(t, "prompt-1", "toolu_red", goTestRun("failed"))
	e.rpFinding(t, "prompt-1", 1)
	e.rp3Line(t, "st-at-stop", "Stop", "claim-vs-record", "go", 1, "")
	e.rp3Line(t, "st-withheld-fixed", "PostToolUseFailure", "regression-location", "go", 1, "budget_prompt")
	e.rpGreen(t, "prompt-2")
	e.rp3Line(t, "st-no-gate", "PreToolUse", "identical-retry", "", 2, "")
	e.rp3Line(t, "st-open", "PostToolUseFailure", "regression-location", "go", 99, "")
	e.rp3Line(t, "st-withheld-open", "PostToolUseFailure", "regression-location", "go", 98, "unverified_channel")
	e.rp3Line(t, "st-held-out", "PostToolUseFailure", "regression-location", "go", 97, "holdout")
	if line := e.mustSpeak(stopEvent("Stop", "Updated main.go.", e.cwd)); !strings.Contains(line, "AutoSteer:") {
		t.Fatalf("premise: the Stop printed %q, with no clause to carry the open finding", line)
	}
}

func TestRP3_CountLineReadsWhatTheRowsRecord(t *testing.T) {
	e := newEnv(t)
	e.rp3Session(t)
	res := e.run("", nil, "report", "--session", testSession, "--json")
	if res.exitCode != 0 {
		t.Fatalf("report --json: exit %d, %s", res.exitCode, res.stderr)
	}
	var rep struct {
		Sessions []struct {
			Autosteer struct {
				Findings []struct {
					ID, State string
					Seq       int64
				}
				Mistakes map[string]json.RawMessage
			}
		}
	}
	if err := json.Unmarshal([]byte(res.stdout), &rep); err != nil || len(rep.Sessions) != 1 {
		t.Fatalf("report --json: %v, %d sessions", err, len(rep.Sessions))
	}
	a := rep.Sessions[0].Autosteer
	var ids []string
	for _, f := range a.Findings {
		ids = append(ids, f.ID+"="+f.State)
	}
	slices.Sort(ids)
	want := []string{"st-at-stop=resolved", "st-held-out=open", "st-no-gate=unchecked", "st-open=open", "st-prompt-1=resolved"}
	if !slices.Equal(ids, want) {
		t.Fatalf("premise: the delivered findings are %v, want %v (the Stop must add none)", ids, want)
	}

	// The JSON: every finding in exactly one list, by id, and later null.
	if a.Mistakes == nil {
		t.Fatal("report --json carries no autosteer.mistakes")
	}
	buckets := map[string][]string{}
	for _, k := range []string{"caught", "reached", "missed", "unsettled", "fixed_unprompted"} {
		var rows []struct{ ID string }
		if err := json.Unmarshal(a.Mistakes[k], &rows); err != nil {
			t.Fatalf("mistakes.%s is %s: %v", k, a.Mistakes[k], err)
		}
		for _, r := range rows {
			buckets[k] = append(buckets[k], r.ID)
		}
	}
	for k, want := range map[string][]string{
		"caught":           {"st-prompt-1"},
		"reached":          {"st-at-stop", "st-open"},
		"missed":           {"st-withheld-open", "st-held-out"},
		"unsettled":        {"st-no-gate"},
		"fixed_unprompted": {"st-withheld-fixed"},
	} {
		if !slices.Equal(buckets[k], want) {
			t.Errorf("mistakes.%s = %v, want %v", k, buckets[k], want)
		}
	}
	n := 0
	for _, b := range buckets {
		n += len(b)
	}
	if n != len(a.Findings)+2 {
		t.Errorf("the lists hold %d findings, want the %d delivered-state ones and the 2 withheld, each once", n, len(a.Findings))
	}
	if got := strings.TrimSpace(string(a.Mistakes["later"])); got != "null" {
		t.Errorf("mistakes.later = %q, want null: nothing records a later find, which is unknown and never 0", got)
	}

	// The text: the owner's line with each term's seqs, the unknown spelled out.
	text := e.run("", nil, "report", "--session", testSession)
	if text.exitCode != 0 {
		t.Fatalf("report: exit %d, %s", text.exitCode, text.stderr)
	}
	line := "    mistakes against intent: 1 caught at the act (at #1), 2 reached the person (at #1, #99), 2 missed (at #98, #97); " +
		"found later unknown (no trace is recorded); 1 not settled (at #2); 1 fixed with no line (at #1)\n"
	if !strings.Contains(text.stdout, line) {
		t.Errorf("the report lacks %q:\n%s", line, text.stdout)
	}
}
