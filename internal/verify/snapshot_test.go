package verify

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// jsonKeys lists a struct's json keys in field order: an unexported field,
// or one tagged "-", has none.
func jsonKeys(v any) []string {
	rt := reflect.TypeOf(v)
	keys := make([]string, 0, rt.NumField())
	for i := 0; i < rt.NumField(); i++ {
		if key := strings.Split(rt.Field(i).Tag.Get("json"), ",")[0]; rt.Field(i).IsExported() && key != "-" {
			keys = append(keys, key)
		}
	}
	return keys
}

// TestVER52d_PointKindsAreTheArchivesList pins Point's kinds and keys (archive 20.2.4).
func TestVER52d_PointKindsAreTheArchivesList(t *testing.T) {
	want := []string{"stop", "subagent_stop", "subtask", "commit", "push", "pr", "query"}
	if !slices.Equal(PointKinds, want) {
		t.Errorf("PointKinds %q, want %q", PointKinds, want)
	}
	if got := jsonKeys(Point{}); !slices.Equal(got, []string{"kind", "seq", "session_id", "prompt_id", "agent_id", "task_ref"}) {
		t.Errorf("Point keys %q", got)
	}
}

// TestVER11b_GateStateKeysAndTypes pins GateState's keys (2.2.4) and the
// archive's field types behind them (20.2.4).
func TestVER11b_GateStateKeysAndTypes(t *testing.T) {
	want := []string{
		"gate", "kind", "family", "source", "trust", "required", "state", "reason",
		"last_run", "last_green", "last_write", "writes_since_green", "writers_since_green",
		"writers_omitted", "unknown_target_writes", "partial_runs_since",
	}
	if got := jsonKeys(GateState{}); !slices.Equal(got, want) {
		t.Errorf("GateState keys\n got %q\nwant %q", got, want)
	}
	for _, c := range []struct {
		v    any
		want []string
	}{
		{RunRef{}, []string{"seq", "tool_use_id", "agent_id", "outcome", "exit_code", "status_masked", "gate_output", "scope"}},
		{WriteRef{}, []string{"seq", "agent_id", "path", "tool"}},
		{AgentCount{}, []string{"agent_id", "agent_type", "writes"}},
	} {
		if got := jsonKeys(c.v); !slices.Equal(got, c.want) {
			t.Errorf("%T keys %q, want %q", c.v, got, c.want)
		}
	}
	rt := reflect.TypeOf(GateState{})
	for name, want := range map[string]string{"LastRun": "*verify.RunRef", "LastGreen": "*verify.RunRef", "LastWrite": "*verify.WriteRef", "WritersSinceGreen": "[]verify.AgentCount"} {
		f, _ := rt.FieldByName(name)
		if f.Type.String() != want {
			t.Errorf("GateState.%s is %s, want %s", name, f.Type, want)
		}
	}
	// RunRef.GateOutput and ExecutionV5.GateOutput are the same key on two
	// records of the same call; one shape, not a string on one side and a
	// *store.GateOutput on the other.
	if f, _ := reflect.TypeOf(RunRef{}).FieldByName("GateOutput"); f.Type != reflect.TypeOf((*store.GateOutput)(nil)) {
		t.Errorf("RunRef.GateOutput is %s, want *store.GateOutput", f.Type)
	}
	if MaxWriters != 8 {
		t.Errorf("MaxWriters = %d, want 8", MaxWriters)
	}
	if RulesVersion != shape.RulesVersion {
		t.Errorf("RulesVersion = %d, want shape.RulesVersion %d: VERIFY carries no second counter", RulesVersion, shape.RulesVersion)
	}
}

// TestVER52b_SnapshotKeysAreTheVerifyJSONList pins `verify --json`'s keys and every
// nested shape against archive 20.2.14's golden, the checkpoint's keys
// (schema-5 header, six *_omitted counts), and the closed lists.
func TestVER52b_SnapshotKeysAreTheVerifyJSONList(t *testing.T) {
	for _, c := range []struct {
		v    any
		want []string
	}{
		{Snapshot{}, []string{"verify_version", "rules_version", "practice_version", "point", "recording", "project_root", "spec", "gates", "claims", "spec_verdicts", "integrity", "practice", "record_green", "record_green_at_seq", "subtasks", "notes"}},
		{SpecInfo{}, []string{"path", "digest", "trust", "errors"}},
		{ClaimVerdict{}, []string{"source", "class", "strict", "verdict", "gates", "quote_hmac"}},
		{RuleVerdict{}, []string{"rule", "kind", "verdict", "seqs", "hard_control"}},
		{Integrity{}, []string{"unresolved", "counts", "unknown_marks", "unread_bases"}}, // the last two D41, D42
		{IntegrityEntry{}, []string{"check", "marker", "seq"}},
		{IntegrityCounts{}, []string{"tests_delta", "skips_added", "suppressions_added"}},
		{Practice{}, []string{"practice_version", "score", "components", "not_applicable"}},
		{PracticeComponents{}, []string{"gates", "claims", "spec", "integrity", "hygiene"}},
		{Subtasks{}, []string{"boundaries", "reason", "items"}},
		{Subtask{}, []string{"ref", "from_seq", "to_seq", "ambiguous", "record_green"}},
		{CheckpointGate{}, []string{"gate", "kind", "source", "required", "state", "reason", "last_run_seq", "last_green_seq", "last_write_seq", "writes_since_green", "unknown_target_writes"}},
		{BackgroundRef{}, []string{"family", "task_ref", "seq"}},
		{CheckpointRuleVerdict{}, []string{"rule", "kind", "verdict", "seqs"}},
		{Checkpoint{}, []string{
			"type", "schema_version", "critical", "seq", "recorded_at_unix_ms", "session_id", "prompt_id", "agent_id",
			"point", "point_seq", "point_tool_use_id", "task_ref", "verify_version", "rules_version",
			"spec_digest", "spec_trust", "recording", "project_root", "gates", "gates_omitted",
			"claims", "claims_omitted", "spec_verdicts", "spec_verdicts_omitted", "integrity", "integrity_omitted",
			"background_in_flight", "background_omitted", "practice", "practice_omitted", "record_green", "truncated",
		}},
	} {
		if got := jsonKeys(c.v); !slices.Equal(got, c.want) {
			t.Errorf("%T keys\n got %q\nwant %q", c.v, got, c.want)
		}
	}
	if f, _ := reflect.TypeOf(Checkpoint{}).FieldByName("Seq"); f.Type.String() != "*int64" {
		t.Errorf("Checkpoint.Seq is %s, want *int64: a spilled checkpoint carries null", f.Type)
	}
	if CheckpointMaxBytes != 8<<10 {
		t.Errorf("CheckpointMaxBytes = %d, want 8 KiB", CheckpointMaxBytes)
	}
	wantKinds := []IntegrityKind{"test_removed", "skip_added", "assertions_removed", "suppression_added", "generated_edited", "tests_only_then_green", "gate_weakened", "gate_config_edited"}
	if !slices.Equal(IntegrityKinds, wantKinds) {
		t.Errorf("IntegrityKinds\n got %q\nwant %q", IntegrityKinds, wantKinds)
	}
	wantClasses := []ClaimClass{"tests_pass", "build_ok", "lint_clean", "typecheck_clean", "scan_clean", "done"}
	if !slices.Equal(ClaimClasses, wantClasses) {
		t.Errorf("ClaimClasses\n got %q\nwant %q", ClaimClasses, wantClasses)
	}
	if PracticeVersion != 1 || QuoteHMACPrefix != "hmac-sha256:" || SourceFinalMessage != "final_message" {
		t.Error("a constant drifted from archive 20.2.14")
	}
}

// TestVER52e_CheckpointKeepsNoHardControl: the rule verdicts the checkpoint
// stores carry the rule id, its kind, the verdict and the violation seqs
// (archive 20.2.14's checkpoint example), never the spec's hard_control
// text, which only the snapshot prints beside a no_recorded_violation
// (2.2.7). Checkpoint is walked by reflection, so a string field added to
// any type it stores fails here too.
func TestVER52e_CheckpointKeepsNoHardControl(t *testing.T) {
	f, _ := reflect.TypeOf(Checkpoint{}).FieldByName("SpecVerdicts")
	if got, want := f.Type, reflect.TypeOf([]CheckpointRuleVerdict(nil)); got != want {
		t.Errorf("Checkpoint.SpecVerdicts is %s, want %s", got, want)
	}
	var walk func(rt reflect.Type, path string, seen map[reflect.Type]bool)
	walk = func(rt reflect.Type, path string, seen map[reflect.Type]bool) {
		for rt.Kind() == reflect.Pointer || rt.Kind() == reflect.Slice {
			rt = rt.Elem()
		}
		if rt.Kind() != reflect.Struct || seen[rt] {
			return
		}
		seen[rt] = true
		for i := 0; i < rt.NumField(); i++ {
			fld := rt.Field(i)
			if tag := strings.Split(fld.Tag.Get("json"), ",")[0]; tag == "hard_control" {
				t.Errorf("the checkpoint stores hard_control at %s.%s", path, fld.Name)
			}
			walk(fld.Type, path+"."+fld.Name, seen)
		}
	}
	walk(reflect.TypeOf(Checkpoint{}), "Checkpoint", map[reflect.Type]bool{})

	// Decoded from the wire rather than built as a literal, so the check
	// reads the same whatever type the field holds.
	var cp Checkpoint
	in := `{"spec_verdicts":[{"rule":"no-shell-true","kind":"forbid_code","verdict":"violation","seqs":[44],"hard_control":"CI scanner job"}]}`
	if err := json.Unmarshal([]byte(in), &cp); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(cp)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "CI scanner job") {
		t.Errorf("checkpoint marshals the spec's hard_control text: %s", b)
	}
	if want := `"spec_verdicts":[{"rule":"no-shell-true","kind":"forbid_code","verdict":"violation","seqs":[44]}]`; !strings.Contains(string(b), want) {
		t.Errorf("checkpoint marshals %s, want it to hold %s", b, want)
	}
	if !slices.Contains(jsonKeys(RuleVerdict{}), "hard_control") {
		t.Error("RuleVerdict lost hard_control; the snapshot prints it beside no_recorded_violation")
	}
}

// TestVER52c_SnapshotMarshalsAsTheGolden: a populated Snapshot marshals with the
// golden's top-level keys and nested key sets (archive 20.2.14), so a reader
// written against the golden reads what Evaluate produces.
func TestVER52c_SnapshotMarshalsAsTheGolden(t *testing.T) {
	seq := int64(37)
	score := 60
	snap := Snapshot{
		VerifyVersion: Version, RulesVersion: RulesVersion, PracticeVersion: PracticeVersion,
		Point:     Point{Kind: PointStop, Seq: 212, SessionID: "s"},
		Recording: Complete, ProjectRoot: "/home/u/proj",
		Spec:         &SpecInfo{Path: "/home/u/proj/.claude/rashomon/spec.json", Digest: "sha256:ab", Trust: spec.Untrusted, Errors: []string{}},
		Gates:        []GateState{{Gate: "learned:go-test", Kind: "test", Source: "learned", Required: true, State: Stale, Reason: WritesAfterGreen, LastGreen: &RunRef{Seq: seq}}},
		Claims:       []ClaimVerdict{{Source: SourceFinalMessage, Class: ClaimTestsPass, Strict: true, Verdict: Verdict(Stale), Gates: []SlotID{"learned:go-test"}, QuoteHMAC: QuoteHMACPrefix + "00"}},
		SpecVerdicts: []RuleVerdict{{Rule: "no-shell-true", Kind: spec.ForbidCode, Verdict: Violation, Seqs: []int64{44}}},
		Integrity:    Integrity{Unresolved: []IntegrityEntry{{Check: SkipAdded, Marker: "go.t_skip", Seq: 40}}},
		Practice:     &Practice{Version: 1, Score: &score, NotApplicable: []string{}},
		Subtasks:     Subtasks{Boundaries: BoundariesNone, Reason: "no_subtask_boundaries", Items: []Subtask{}},
		Notes:        []string{"a recorded run is not a correct result"},
	}
	b, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	keysOf := func(raw json.RawMessage) []string {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("not an object: %s", raw)
		}
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		return keys
	}
	golden := map[string][]string{
		"point":     {"agent_id", "kind", "prompt_id", "seq", "session_id", "task_ref"},
		"spec":      {"digest", "errors", "path", "trust"},
		"integrity": {"counts", "unknown_marks", "unread_bases", "unresolved"}, // unknown_marks, unread_bases: D41, D42
		"practice":  {"components", "not_applicable", "practice_version", "score"},
		"subtasks":  {"boundaries", "items", "reason"},
	}
	for key, want := range golden {
		if got := keysOf(doc[key]); !slices.Equal(got, want) {
			t.Errorf("%s keys %q, want the golden's %q", key, got, want)
		}
	}
	if string(doc["point"]) == "" || strings.Contains(string(doc["point"]), "Kind") {
		t.Errorf("point marshals PascalCase keys: %s", doc["point"])
	}
	if !strings.Contains(string(b), `"subtasks":{"boundaries":"none","reason":"no_subtask_boundaries","items":[]}`) {
		t.Errorf("subtasks did not marshal as the golden: %s", b)
	}
}

// TestVER18b_AnEmptyRecordFailsClosed: a record with no coverage record reads
// incomplete (coverage_absent), so the snapshot has no record-green point and
// no gate, the comparator is silent and the value line reads unknown; the
// spec's identity still travels, the path being the repository spec's. A nil
// run is an empty one.
func TestVER18b_AnEmptyRecordFailsClosed(t *testing.T) {
	sp := &spec.Compiled{
		Sources: []spec.Source{{Path: "/etc/rashomon/spec.json", Digest: spec.DigestPrefix + "11", Trust: spec.Managed}, {Path: ".claude/rashomon/spec.json", Digest: spec.DigestPrefix + "22", Trust: spec.Trusted}},
		Digest:  spec.DigestPrefix + strings.Repeat("ab", 32),
		Trust:   spec.Trusted,
	}
	snap := Evaluate(&store.Run{Dir: "s1"}, &ProjectIndex{ProjectKey: "k"}, sp, Point{Kind: PointStop, Seq: 42, SessionID: "s1"}, time.Unix(0, 0))
	if snap.Recording != Incomplete {
		t.Errorf("empty record: recording %q, want %q", snap.Recording, Incomplete)
	}
	if snap.RecordGreen || snap.RecordGreenAtSeq != nil || len(snap.Gates) != 0 {
		t.Errorf("empty record: gates or a record-green point: %+v", snap)
	}
	if snap.Point.Seq != 42 || snap.VerifyVersion != Version || snap.RulesVersion != RulesVersion {
		t.Errorf("empty record: the point or versions lost: %+v", snap)
	}
	if snap.Spec == nil || snap.Spec.Digest != sp.Digest || snap.Spec.Trust != spec.Trusted || snap.Spec.Path != ".claude/rashomon/spec.json" {
		t.Errorf("empty record: the spec identity lost: %+v", snap.Spec)
	}
	if !slices.Equal(snap.Notes, []string{"coverage_absent"}) {
		t.Errorf("empty record: notes %q, want coverage_absent alone", snap.Notes)
	}
	if row := TurnSummary(snap, TurnCounts{Writes: 3, GateRuns: 1}).Row; row != RowUnknown {
		t.Errorf("value line over an empty record: row %q, want %q", row, RowUnknown)
	}
	if got := Evaluate(nil, nil, nil, Point{}, time.Time{}); got.Spec != nil || got.Recording != Incomplete || !slices.Equal(got.Notes, []string{"coverage_absent"}) {
		t.Errorf("nil inputs: %+v", got)
	}
	if spec.Lowest(spec.Managed, spec.Untrusted) != spec.Untrusted || spec.Lowest(spec.Trusted, spec.Managed) != spec.Trusted || spec.Lowest("x", spec.Managed) != spec.Untrusted {
		t.Error("spec.Lowest is not the lower trust")
	}
}
