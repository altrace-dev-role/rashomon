package report

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"
)

// keysOf is every JSON key path of v, arrays as "[]", sorted.
func keysOf(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var doc any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	set := map[string]bool{}
	var walk func(prefix string, x any)
	walk = func(prefix string, x any) {
		switch x := x.(type) {
		case map[string]any:
			for k, v := range x {
				p := k
				if prefix != "" {
					p = prefix + "." + k
				}
				set[p] = true
				walk(p, v)
			}
		case []any:
			for _, v := range x {
				walk(prefix+"[]", v)
			}
		}
	}
	walk("", doc)
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return strings.Join(out, "\n")
}

// TestRP2r_TheKnowledgeAndWhyKeySetsAreClosed (card REPORT1b Standard): the
// keys of `knowledge` and of `why --json` are exactly these, so a key added
// later is a decision, with the schema's bump rule read first (a new key
// keeps report_version at 2).
func TestRP2r_TheKnowledgeAndWhyKeySetsAreClosed(t *testing.T) {
	k := sampleKnowledge()
	k.Invalidated = []Change{{What: "x", Source: "index"}}
	wantK := `baseline
baseline.point_seq
baseline.recorded_at_unix_ms
baseline.session_id
discovered
discovered[].at_unix_ms
discovered[].from
discovered[].point_seq
discovered[].source
discovered[].to
discovered[].what
gate_states
gate_states[].changes
gate_states[].gate
gate_states[].last_from
gate_states[].last_seq
gate_states[].last_to
gates_cut
invalidated
invalidated[].at_unix_ms
invalidated[].from
invalidated[].point_seq
invalidated[].source
invalidated[].to
invalidated[].what
project
retained
retained.bindings_unsigned
retained.facts
retained.ledger_unchecked
retained.lessons
retained.lessons[].id
retained.lessons[].tier
retained.prior
retained.required_families
retained.spec_at_last_stop
retained.spec_at_last_stop.compiled
retained.spec_at_last_stop.point_seq
retained.spec_at_last_stop.trust
retained.specs
retained.specs[].bound_at_unix_ms
retained.specs[].file_digest
retained.specs[].path`
	if got := keysOf(t, k); got != wantK {
		t.Errorf("knowledge keys:\n%s\nwant\n%s", got, wantK)
	}
	seq, green := int64(1), int64(44)
	w := &WhyView{Slots: []WhySlot{{Name: "gate", Value: "go"}}, Evidence: []TimelineCall{{Seq: &seq}}, Resolution: &WhyResolution{LastGreenSeq: &green}}
	for _, key := range strings.Split(keysOf(t, w), "\n") {
		if strings.HasPrefix(key, "evidence[].") {
			continue // the timeline row's own closed set is pinned by its tests
		}
		if !strings.Contains(wantWhy, "\n"+key+"\n") {
			t.Errorf("why key %q is not in the declared set", key)
		}
	}
	for _, want := range strings.Split(strings.Trim(wantWhy, "\n"), "\n") {
		if !strings.Contains("\n"+keysOf(t, w)+"\n", "\n"+want+"\n") {
			t.Errorf("why key %q is declared and not written", want)
		}
	}
}

const wantWhy = `
after_seq
at_unix_ms
delivery
event
evidence
kind
level
line_id
model_text
pattern
prefix
project
prompt_id
redacted
resolution
resolution.gate
resolution.last_green_seq
resolution.point_seq
resolution.state
session_id
slots
slots[].name
slots[].value
source
source.detail
source.kind
state
suppressed_by
text_kept
title
user_text
key
`
