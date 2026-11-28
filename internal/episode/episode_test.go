package episode

import (
	"encoding/json"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/trace"
)

// session builds one session's steps as any adapter would emit them, with no
// agent's words: an act is a call and its result.
type session_ struct {
	steps []trace.Step
	n     int
	at    time.Time
}

func newSession(root string) *session_ {
	s := &session_{at: time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)}
	s.steps = append(s.steps, trace.Step{Kind: trace.Entry, Session: "s1", At: s.at, Cwd: root})
	return s
}

func (s *session_) add(st trace.Step) int {
	s.at = s.at.Add(time.Second)
	st.Session, st.At = "s1", s.at
	s.steps = append(s.steps, st)
	return len(s.steps) - 1
}

func (s *session_) call(c trace.CallStep) string {
	s.n++
	c.ID = "c" + strconv.Itoa(s.n)
	in, _ := json.Marshal(map[string]any{"command": c.Command, "paths": c.Paths, "changes": c.Changes})
	c.Input = in
	s.add(trace.Step{Kind: trace.Call, Call: &c})
	return c.ID
}

func (s *session_) result(id string, r trace.ResultStep) int {
	r.CallID = id
	return s.add(trace.Step{Kind: trace.Result, Result: &r})
}

func (s *session_) exec(cmd, out string, ok bool) int {
	id := s.call(trace.CallStep{Tool: "shell", Class: trace.ActExec, Command: cmd})
	return s.result(id, trace.ResultStep{Failed: !ok, Output: out})
}

func (s *session_) edit(path, old, new string) int {
	id := s.call(trace.CallStep{Tool: "edit", Class: trace.ActEdit, Paths: []string{path},
		Changes: []trace.Change{{Path: path, Old: old, New: new}}})
	return s.result(id, trace.ResultStep{Output: "ok"})
}

func (s *session_) read(path string) int {
	id := s.call(trace.CallStep{Tool: "read", Class: trace.ActRead, Paths: []string{path}})
	return s.result(id, trace.ResultStep{Output: "contents"})
}

func (s *session_) said(text string, o trace.Origin, sig trace.Signal) int {
	return s.add(trace.Step{Kind: trace.Said, Text: text, Origin: o, Signal: sig})
}

func only(t *testing.T, es []Episode, kind Kind) []Episode {
	t.Helper()
	var out []Episode
	for _, e := range es {
		if e.Key.Kind == kind {
			out = append(out, e)
		}
	}
	return out
}

const pytestFail = `ImportError while importing test module '/w/py/tests/test_api.py'.
E   ModuleNotFoundError: No module named 'app'
=========================== short test summary info ============================
ERROR tests/test_api.py
!!!!!!!!!!!!!!!!!!!! Interrupted: 1 error during collection !!!!!!!!!!!!!!!!!!!!
=============================== 1 error in 0.12s ===============================`

// TestPython_ImportErrorResolvedByInstall: a pytest collection error the
// agent ran into, installed the package, and ran again: one act failure,
// keyed on the command and the error's own sentence, resolved by an act the
// session had not run before.
func TestPython_ImportErrorResolvedByInstall(t *testing.T) {
	s := newSession("/w/py")
	s.read("/w/py/tests/test_api.py")
	fail := s.exec("pytest tests/test_api.py", pytestFail, false)
	s.exec("pip install -e .", "Successfully installed app-0.1.0", true)
	pass := s.exec("pytest tests/test_api.py", "1 passed in 0.10s", true)

	es := Detect(s.steps, Options{})
	if len(es) != 1 {
		t.Fatalf("episodes %+v", es)
	}
	e := es[0]
	want := Key{V: KeyVersion, Kind: ActFailure, Act: ActSig{Class: trace.ActExec, Program: "pytest"},
		Scope: Scope{Dir: "tests", Ext: ".py"}, FP: e.Key.FP, FPV: FingerprintVersion}
	if e.Key != want || e.Key.FP == "" {
		t.Errorf("key %+v, want %+v", e.Key, want)
	}
	if e.Template != "E ModuleNotFoundError No module named <*>" {
		t.Errorf("template %q", e.Template)
	}
	if e.Evidence[0] != fail || e.To != pass {
		t.Errorf("span %d..%d evidence %v, want failure at %d and pass at %d", e.From, e.To, e.Evidence, fail, pass)
	}
	if r := e.Resolution; r == nil || !reflect.DeepEqual(r.Before, []ActSig{{Class: trace.ActExec, Program: "pip", Verb: "install"}}) {
		t.Errorf("resolution %+v", e.Resolution)
	}
}

const jestFail = `FAIL src/api/client.test.ts
  ● client › fetches the user
    TypeError: Cannot read properties of undefined (reading 'id')
      at Object.<anonymous> (src/api/client.ts:14:22)
Tests:       1 failed, 3 passed, 4 total
Time:        1.204 s`

const jestPass = `PASS src/api/client.test.ts
Tests:       4 passed, 4 total
Time:        1.101 s`

// TestTypeScript_RegressionAfterEdit: a test command that passed, an edit,
// the same command failing, then a fix: a check regression, resolved by the
// edit to the file the failure names.
func TestTypeScript_RegressionAfterEdit(t *testing.T) {
	s := newSession("/w/web")
	s.exec("cd /w/web && npx jest src/api", jestPass, true)
	s.edit("/w/web/src/api/client.ts", "user.id", "user.profile.id")
	s.exec("cd /w/web && npx jest src/api", jestFail, false)
	s.edit("/w/web/src/api/client.ts", "user.profile.id", "user.profile?.id")
	s.exec("cd /w/web && npx jest src/api", jestPass, true)

	b := Baseline{}
	b.Learn(s.steps[:3], "")
	es := only(t, Detect(s.steps, Options{Baseline: b}), CheckRegression)
	if len(es) != 1 {
		t.Fatalf("regressions %+v", es)
	}
	e := es[0]
	if e.Key.Act != (ActSig{Class: trace.ActExec, Program: "npx", Verb: "jest"}) || e.Key.Scope != (Scope{Dir: "src/api"}) {
		t.Errorf("key %+v", e.Key)
	}
	if e.Template != "TypeError Cannot read properties of undefined reading <*>" {
		t.Errorf("template %q", e.Template)
	}
	if r := e.Resolution; r == nil || !reflect.DeepEqual(r.Edited, []string{"src/api/client.ts"}) {
		t.Errorf("resolution %+v", e.Resolution)
	}
}

const tfFail = `Error: Unsupported argument

  on main.tf line 12, in resource "aws_s3_bucket" "logs":
  12:   acl = "private"

An argument named "acl" is not expected here.`

// TestTerraform_LoopCorrectionsAndRevert: a plan retried unchanged after the
// same failure (a loop), the person refusing an apply and a rule blocking
// one, the person stopping the agent, and an edit undone.
func TestTerraform_LoopCorrectionsAndRevert(t *testing.T) {
	s := newSession("/w/infra")
	s.read("/w/infra/main.tf")
	for i := 0; i < 3; i++ {
		s.exec("terraform plan", tfFail, false)
	}
	s.edit("/w/infra/main.tf", `acl = "private"`, "")
	s.exec("terraform plan", "No changes. Your infrastructure matches the configuration.", true)
	rej := s.call(trace.CallStep{Tool: "shell", Class: trace.ActExec, Command: "terraform apply -auto-approve"})
	s.result(rej, trace.ResultStep{Failed: true, Denied: trace.DeniedByPerson})
	blk := s.call(trace.CallStep{Tool: "shell", Class: trace.ActExec, Command: "terraform destroy"})
	s.result(blk, trace.ResultStep{Failed: true, Denied: trace.DeniedByRule})
	s.edit("/w/infra/variables.tf", "default = 1", "default = 3")
	s.edit("/w/infra/variables.tf", "default = 3", "default = 1")
	s.said("[stop]", trace.OriginUnknown, trace.SignalInterrupt)
	s.said("[stop]", trace.OriginUnknown, trace.SignalInterrupt) // no act since: not counted
	s.said("no, leave the variables alone", trace.OriginPerson, trace.SignalNone)
	s.edit("/w/infra/outputs.tf", "a", "b")
	s.said("no, revert that", trace.OriginMachine, trace.SignalNone) // not the person's words
	s.read("/w/infra/outputs.tf")
	s.said("no, from another session", trace.OriginUnknown, trace.SignalNone) // not marked as typed

	es := Detect(s.steps, Options{Pushback: func(text string) bool { return text[:2] == "no" }})
	loopsFound := only(t, es, RetryLoop)
	if len(loopsFound) != 1 || len(loopsFound[0].Evidence) != 3 || loopsFound[0].Template != "An argument named <*> is not expected here" {
		t.Errorf("loops %+v", loopsFound)
	}
	var details []string
	for _, e := range only(t, es, Correction) {
		details = append(details, e.Key.Detail+" "+e.Key.Act.Verb)
	}
	if want := []string{"reject apply", "block destroy", "interrupt "}; !reflect.DeepEqual(details, want) {
		t.Errorf("corrections %q, want %q", details, want)
	}
	if r := only(t, es, SelfRevert); len(r) != 1 || r[0].Key.Scope != (Scope{Ext: ".tf"}) {
		t.Errorf("reverts %+v", r)
	}
	if f := only(t, es, ActFailure); len(f) != 1 || f[0].Resolution == nil || !reflect.DeepEqual(f[0].Resolution.Edited, []string{"main.tf"}) {
		t.Errorf("failures %+v", f)
	}
}

// TestDetect_NoEpisodeForAOneOffOrASubagent: a failure the session never
// returns to, and a subagent's acts and lines, make no episode, even when the
// main session later runs what the subagent failed at.
func TestDetect_NoEpisodeForAOneOffOrASubagent(t *testing.T) {
	s := newSession("/w")
	s.exec("make lint", "lint: 3 problems found in the module", false)
	s.exec("tool build", "build broke: the input set is empty", false)
	s.steps[len(s.steps)-2].Sidechain = true
	s.steps[len(s.steps)-1].Sidechain = true
	s.exec("tool build", "built the whole thing", true)
	s.read("/w/a.c")
	id := s.call(trace.CallStep{Tool: "shell", Class: trace.ActExec, Command: "make"})
	s.steps[len(s.steps)-1].Sidechain = true
	s.result(id, trace.ResultStep{Failed: true, Denied: trace.DeniedByPerson})
	s.steps[len(s.steps)-1].Sidechain = true
	if es := Detect(s.steps, Options{}); len(es) != 0 {
		t.Errorf("episodes %+v", es)
	}
}

// TestRetryLoop_NeedsTwoRepeats: one identical retry after a failure is an
// act failure, not yet a loop; a second is.
func TestRetryLoop_NeedsTwoRepeats(t *testing.T) {
	s := newSession("/w")
	s.exec("tool check", "check failed: missing field in the record", false)
	s.exec("tool check", "check failed: missing field in the record", false)
	s.exec("tool check", "all good, nothing to report", true)
	es := Detect(s.steps, Options{})
	if len(only(t, es, RetryLoop)) != 0 || len(only(t, es, ActFailure)) != 1 {
		t.Errorf("episodes %+v", es)
	}
}
