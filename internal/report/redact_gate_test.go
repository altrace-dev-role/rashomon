package report

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestRP2zm_RedactDigestsAGateIdEverywhereItReaches (census g3 W1, STEP-15): a
// trusted spec's gate id is a hostname-shaped word, and besides the autosteer
// gate name and Stop sentence it reaches a finding's gate slot, a check's gate
// key and its reason, and the knowledge block's gate keys. Under --redact none
// of them carries it and the words around it stay; the original report keeps
// it. Break: skip any one field's digest (the RP-2 mutants name each).
func TestRP2zm_RedactDigestsAGateIdEverywhereItReaches(t *testing.T) {
	const id = "a.example.com"
	gate := "spec:" + id
	rep := &Report{Sessions: []Session{{SessionID: "s1",
		Autosteer: &Autosteer{
			Stop:     &AutosteerStop{Seq: 6, Row: "verified", Sentence: "verified - '" + id + "' passed"},
			Gates:    []AutosteerGate{{Name: "'" + id + "'", Kind: "test", State: "green"}},
			Findings: []AutosteerFinding{{ID: "n1", Pattern: "stale_evidence", Title: "stale", Gate: id, State: "open"}},
			Checks: []AutosteerCheck{{Kind: "real_data_run", State: "unmet", Gate: gate,
				Reason: "no green run of " + gate + " since the last edit"},
				{Kind: "cross_check", State: "met", Gate: "learned:go-test"}},
		},
		Knowledge: &Knowledge{GateStates: []GateStates{
			{Gate: gate, Changes: 2, LastFrom: "red", LastTo: "green", LastSeq: 9},
			{Gate: "learned:go-test", Changes: 1, LastFrom: "red", LastTo: "green", LastSeq: 4}}},
	}}}
	red := Redact(rep, []byte("install-key"))
	body, err := json.Marshal(red)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), id) {
		t.Errorf("gate id survived --redact:\n%s", locate(string(body), id))
	}
	a, k := red.Sessions[0].Autosteer, red.Sessions[0].Knowledge
	if !strings.HasPrefix(a.Checks[0].Gate, "spec:") || !strings.HasPrefix(k.GateStates[0].Gate, "spec:") {
		t.Errorf("a gate key lost its spec: prefix: %q, %q", a.Checks[0].Gate, k.GateStates[0].Gate)
	}
	if want := "no green run of " + a.Checks[0].Gate + " since the last edit"; a.Checks[0].Reason != want {
		t.Errorf("reason = %q, want %q", a.Checks[0].Reason, want)
	}
	if a.Checks[1].Gate != "learned:go-test" || k.GateStates[1].Gate != "learned:go-test" {
		t.Errorf("a learned gate key was digested: %q, %q", a.Checks[1].Gate, k.GateStates[1].Gate)
	}
	if a.Findings[0].Pattern != "stale_evidence" || a.Findings[0].Gate == "" {
		t.Errorf("a closed word changed or the gate was dropped: %+v", a.Findings[0])
	}
	orig, _ := json.Marshal(rep)
	if strings.Count(string(orig), id) != 6 {
		t.Errorf("the original report lost the id: %s", orig)
	}
}

// TestRP2zm_RedactWhyDigestsAGateId: a why view's gate slot, gate source and
// resolution carry the same id and are digested too.
func TestRP2zm_RedactWhyDigestsAGateId(t *testing.T) {
	const id = "a.example.com"
	v := &WhyView{Slots: []WhySlot{{Name: "gate", Value: id}, {Name: "exit", Value: "1"}},
		Source:     WhySource{Kind: "gate", Detail: id},
		Resolution: &WhyResolution{Gate: "spec:" + id, State: "green", PointSeq: 3}}
	red := RedactWhy(v, []byte("install-key"))
	body, _ := json.Marshal(red)
	if strings.Contains(string(body), id) {
		t.Errorf("gate id survived why --redact:\n%s", body)
	}
	if red.Slots[1].Value != "1" || red.Resolution.State != "green" {
		t.Errorf("the rest changed: %+v", red)
	}
	if o, _ := json.Marshal(v); strings.Count(string(o), id) != 3 {
		t.Errorf("the original lost the id: %s", o)
	}
}
