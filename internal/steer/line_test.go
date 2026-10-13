package steer

import (
	"encoding/json"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
)

// wantLineJSONTags is tech 4.2.3's Line key list: every key the example
// line shows, every key the "Also:" sentence adds, `outcome` (its own bullet:
// "closed, null on every other line") and `mirror_failure` (tech 6.2.1). A change to Line's JSON shape that does not also change
// this list is exactly the drift this test exists to catch.
var wantLineJSONTags = []string{
	"type", "v", "id", "at_unix_ms", "session_id", "prompt_id", "agent_id",
	"project", "pattern", "kind", "pattern_version", "level", "channel",
	"event", "attendance", "arm", "after_seq", "evidence_seqs", "slots",
	"model_text", "user_text", "state", "suppressed_by", "invocation_id",
	"renderer_version", "rules_version", "steer_id", "label_rule",
	"delivery", "mirrored_to_user", "facts", "records_offset", "harness_id",
	"surface", "mirror", "mirror_failure", "outcome",
}

// TestLine_JSONTagsMatchSpec is a construction-truth test: it reads Line's
// tags by reflection rather than trusting this file's prose, so a field
// renamed without updating its tag -- or added without updating this list
// -- fails here, not in a reader three packages away.
func TestLine_JSONTagsMatchSpec(t *testing.T) {
	typ := reflect.TypeOf(Line{})
	got := make([]string, 0, typ.NumField())
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		tag := f.Tag.Get("json")
		if tag == "" {
			t.Errorf("field %s has no json tag", f.Name)
			continue
		}
		got = append(got, tag)
	}
	sortedGot := append([]string(nil), got...)
	sortedWant := append([]string(nil), wantLineJSONTags...)
	sort.Strings(sortedGot)
	sort.Strings(sortedWant)
	if !reflect.DeepEqual(sortedGot, sortedWant) {
		t.Errorf("Line JSON tags = %v, want %v", sortedGot, sortedWant)
	}
}

func TestFacts_JSONTagsMatchSpec(t *testing.T) {
	want := []string{"program", "runner_digest", "target_id", "mechanism_id", "class_id", "rule_id"}
	typ := reflect.TypeOf(Facts{})
	if typ.NumField() != len(want) {
		t.Fatalf("Facts has %d fields, want %d", typ.NumField(), len(want))
	}
	for i, w := range want {
		f := typ.Field(i)
		if f.Tag.Get("json") != w {
			t.Errorf("Facts field %d tag = %q, want %q", i, f.Tag.Get("json"), w)
		}
		if f.Type.Kind() != reflect.Ptr {
			t.Errorf("Facts.%s is not a pointer, want nullable *string", f.Name)
		}
	}
}

// TestLine_NullableFieldsMarshalNull pins that the spec's independently
// nullable top-level fields serialise as explicit JSON null -- not an
// omitted key -- on a zero Line, matching the example line's own
// "agent_id":null and "suppressed_by":null.
func TestLine_NullableFieldsMarshalNull(t *testing.T) {
	var l Line
	b, err := json.Marshal(l)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	for _, key := range []string{"agent_id", "suppressed_by", "label_rule", "delivery", "outcome", "mirror_failure"} {
		v, ok := got[key]
		if !ok {
			t.Errorf("zero Line is missing key %q", key)
			continue
		}
		if v != nil {
			t.Errorf("zero Line's %q = %v, want null", key, v)
		}
	}
}

func TestLine_PopulatedRoundTrip(t *testing.T) {
	agent := "sub-1"
	sb := SuppressedByDeadline
	rule := LabelRuleGateRed
	delivery := DeliveryDelivered
	outcome := OutcomeAskUnattended
	l := Line{
		Type:         LineType,
		V:            LineVersion,
		ID:           "0123456789abcdef0123456789abcdef",
		SessionID:    "sess-1",
		PromptID:     "prompt-1",
		AgentID:      &agent,
		Project:      "/abs/root",
		Pattern:      "regression-location",
		Channel:      ChannelPostToolBatch,
		Event:        string(EventPostToolBatch),
		Attendance:   AttendanceAttended,
		Arm:          ArmLive,
		AfterSeq:     57,
		EvidenceSeqs: []int64{37, 52},
		Slots: map[string]json.RawMessage{
			"gate": json.RawMessage(`"go test"`), "exit": json.RawMessage(`1`),
			"paths": json.RawMessage(`["internal/a.go"]`),
		},
		ModelText:       "rashomon record: something",
		UserText:        "※ rashomon told the agent: \"rashomon record: something\"",
		State:           LineStateDelivered,
		SuppressedBy:    &sb,
		InvocationID:    "fedcba9876543210fedcba9876543210",
		RendererVersion: 1,
		RulesVersion:    4,
		SteerID:         "0123456789abcdef0123456789abcdef",
		LabelRule:       &rule,
		Outcome:         &outcome,
		Delivery:        &delivery,
		MirroredToUser:  true,
		Mirror:          MirrorSystemMessage,
	}
	b, err := json.Marshal(l)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var round Line
	if err := json.Unmarshal(b, &round); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(l, round) {
		t.Errorf("round trip mismatch:\n got %+v\nwant %+v", round, l)
	}
	if l.SteerID != l.ID {
		t.Errorf("SteerID = %q, want equal to ID %q", l.SteerID, l.ID)
	}
}

// specExampleLine is tech 4.2.3's example steer.ndjson line, byte for byte
// as the spec prints it (placeholders and line breaks included).
const specExampleLine = `{"type":"steer","v":1,"id":"<32 hex: 128 random bits>","at_unix_ms":1760000000000,
 "session_id":"…","prompt_id":"…","agent_id":null,"project":"/abs/root",
 "pattern":"regression-location","kind":"","pattern_version":1,
 "level":"nudge","channel":"post_tool_batch","event":"PostToolBatch",
 "attendance":"attended","arm":"live","after_seq":57,"evidence_seqs":[37,52],
 "slots":{"gate":"go test","exit":1,"paths":["internal/a.go","internal/b.go"],"more":0,"minutes":6},
 "model_text":"rashomon record: …","user_text":"※ rashomon told the agent: \"rashomon record: …\"",
 "state":"delivered","suppressed_by":null,"invocation_id":"<32 hex: the supervisor's id for the child that appended this line, G5>","renderer_version":1,"rules_version":4}`

// TestLine_DecodesTheSpecsExampleLine: a reader built on Line can read the
// line the spec shows, number and array slots included, and every key the
// example carries is a Line field.
func TestLine_DecodesTheSpecsExampleLine(t *testing.T) {
	dec := json.NewDecoder(strings.NewReader(specExampleLine))
	dec.DisallowUnknownFields()
	var l Line
	if err := dec.Decode(&l); err != nil {
		t.Fatalf("decoding the spec's example line: %v", err)
	}
	if l.Pattern != "regression-location" || l.Channel != ChannelPostToolBatch || l.State != LineStateDelivered {
		t.Errorf("decoded pattern/channel/state = %q/%q/%q", l.Pattern, l.Channel, l.State)
	}
	want := map[string]string{
		"gate": `"go test"`, "exit": `1`, "paths": `["internal/a.go","internal/b.go"]`, "more": `0`, "minutes": `6`,
	}
	if len(l.Slots) != len(want) {
		t.Fatalf("slots = %v, want the example's %d keys", l.Slots, len(want))
	}
	for k, v := range want {
		if got := string(l.Slots[k]); got != v {
			t.Errorf("slots[%q] = %s, want %s", k, got, v)
		}
	}
	if l.AgentID != nil || l.SuppressedBy != nil || l.Outcome != nil || l.LabelRule != nil {
		t.Errorf("the example's null and absent nullable keys decoded non-nil: %+v", l)
	}
}

// TestLabelRule_IsMeasuresTable pins label_rule's values to MEASURE's rule
// table (tech 5.2.4 B), in the table's order, as literals.
func TestLabelRule_IsMeasuresTable(t *testing.T) {
	want := []LabelRule{
		"gate_red", "gate_unverified", "claim_vs_record", "known_fix", "needs_human",
		"policy_match", "repeat_failure", "absence", "none",
	}
	if got := AllLabelRules(); !slices.Equal(got, want) {
		t.Errorf("AllLabelRules() = %q, want %q", got, want)
	}
	for _, label := range []string{"confirmed", "refuted", "unresolved"} {
		if slices.Contains(AllLabelRules(), LabelRule(label)) {
			t.Errorf("label value %q is in the rule set: a label is not a rule", label)
		}
	}
}
