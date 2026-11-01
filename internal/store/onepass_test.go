package store

import (
	"bytes"
	"encoding/json"
	"reflect"
	"sort"
	"testing"
)

// readTwoPass is classifyRunLine as it was before card 2.3b: the head
// decoded first, then an accepted pair's record decoded again. It is the
// oracle the one-pass reader is compared against (R4,
// FuzzREC_OnePassEqualsTwoPass), and nothing else calls it.
func readTwoPass(run *Run, line []byte) {
	var head struct {
		Type          string `json:"type"`
		SchemaVersion int    `json:"schema_version"`
		Critical      *bool  `json:"critical"`
	}
	if json.Unmarshal(line, &head) != nil {
		run.Skipped++
		return
	}
	if !AcceptsKind(head.Type, head.SchemaVersion) {
		if head.Critical != nil && !*head.Critical {
			run.Ignored++
		} else {
			run.Skipped++
		}
		return
	}
	var err error
	switch head.Type {
	case TypeDeclaration:
		var rec Declaration
		if err = json.Unmarshal(line, &rec); err == nil {
			run.Declarations = append(run.Declarations, rec)
		}
	case TypeExecution:
		var rec Execution
		if err = json.Unmarshal(line, &rec); err == nil {
			run.Executions = append(run.Executions, rec)
		}
	case TypeTerminal:
		var rec Terminal
		if err = json.Unmarshal(line, &rec); err == nil {
			run.Terminals = append(run.Terminals, rec)
		}
	case TypeCoverage:
		var rec Coverage
		if err = json.Unmarshal(line, &rec); err == nil {
			run.Coverage = append(run.Coverage, rec)
		}
	case TypeSpecEvent:
		var rec SpecEvent
		if err = json.Unmarshal(line, &rec); err == nil {
			switch {
			case rec.Reason == SpecEventRuleHit:
				run.SpecEvents = append(run.SpecEvents, rec)
			case head.Critical != nil && !*head.Critical:
				run.Ignored++
			default:
				run.Skipped++
			}
		}
	case TypeAfterHit:
		var rec AfterHit
		if err = json.Unmarshal(line, &rec); err == nil {
			run.AfterHits = append(run.AfterHits, rec)
		}
	default:
		var rec EventHeader
		err = json.Unmarshal(line, &rec)
	}
	if err != nil {
		run.Skipped++
	}
}

const onePassSession = "sess-1p"

// writerRecord is a record of kind at version, in the type the store writes
// that kind as: the call records, coverage and gaps in their own structs,
// the spec_event as a rule_hit, every other kind of 1.2.0 as the header each
// of them starts with.
func writerRecord(kind string, version int) any {
	seq, at := int64(7), int64(1_800_000_000_000)
	switch kind {
	case TypeDeclaration:
		return Declaration{Type: kind, SchemaVersion: version, Seq: seq, RecordedAtMS: at, ToolUseID: "toolu_1p", SessionID: onePassSession}
	case TypeExecution:
		return Execution{Type: kind, SchemaVersion: version, Seq: &seq, RecordedAtMS: at, ToolUseID: "toolu_1p", SessionID: onePassSession, Outcome: ExecOK}
	case TypeTerminal:
		return Terminal{Type: kind, SchemaVersion: version, Seq: &seq, RecordedAtMS: at, ToolUseID: "toolu_1p", SessionID: onePassSession, Outcome: OutcomeOK}
	case TypeCoverage:
		return Coverage{Type: kind, SchemaVersion: version, RecordedAtMS: at, SessionID: onePassSession, Phase: PhasePost}
	case TypeGap:
		return Gap{Type: kind, SchemaVersion: version, RecordedAtMS: at, SessionID: onePassSession, Reason: "forget"}
	case TypeSpecEvent:
		e := NewRuleHit(onePassSession, "PostToolUse", "spec:no-force", "sha256:1p", 6, at)
		e.SchemaVersion, e.Seq = version, &seq
		return *e
	case TypeAfterHit:
		zero := 0
		e := NewAfterHit(onePassSession, "PostToolUse", "spec:test-first", "sha256:1p", 6, &zero, true, "cwd:1p", false, at)
		e.SchemaVersion, e.Seq = version, &seq
		return *e
	}
	return EventHeader{Type: kind, SchemaVersion: version, Seq: &seq, RecordedAtMS: at, SessionID: onePassSession}
}

// writerLine is rec as marshalLine writes it, without its newline: the form
// the one-pass reader reads its pair from (2.3b P2).
func writerLine(t testing.TB, rec any) []byte {
	t.Helper()
	line, err := marshalLine(rec)
	if err != nil {
		t.Fatal(err)
	}
	return bytes.TrimSuffix(line, []byte("\n"))
}

// withKey is line, an object, with one more key and value at its end.
func withKey(line []byte, kv string) []byte {
	out := append([]byte(nil), line[:len(line)-1]...)
	return append(append(out, ","+kv...), '}')
}

// sortedKinds is kindVersion's kinds in name order.
func sortedKinds() []string {
	kinds := make([]string, 0, len(kindVersion))
	for k := range kindVersion {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	return kinds
}

// A onePassCase is one line of R4's fixture run. differs names the
// documented difference (D97) the line shows, or is empty when the two
// readers must agree on it exactly.
type onePassCase struct {
	name, differs string
	line          []byte
}

// escapedA is the letter a as a JSON escape: a kind name written this way
// decodes to its plain name but is not that name's bytes.
const escapedA = "\\u0061"

const (
	diffPair     = "pair"     // the prefix's pair and the decoded pair disagree: Skipped
	diffCritical = "critical" // a non-boolean critical on an accepted kind is not read
)

// onePassFixture is R4's run: every kind at every version this reader
// accepts, in the store's own form, and each line the one-pass reader must
// read as the two-pass reader did or differ from it only as documented.
func onePassFixture(t testing.TB) []onePassCase {
	t.Helper()
	var cases []onePassCase
	for _, kind := range sortedKinds() {
		for v := 1; v <= CallSchemaVersion+1; v++ {
			if AcceptsKind(kind, v) {
				cases = append(cases, onePassCase{name: kind + "_v" + string(rune('0'+v)), line: writerLine(t, writerRecord(kind, v))})
			}
		}
	}
	decl := writerLine(t, writerRecord(TypeDeclaration, CallSchemaVersion))
	spilled := writerRecord(TypeExecution, CallSchemaVersion).(Execution)
	spilled.Seq = nil
	reason, phase := ReasonFlushFailed, PhasePost
	marker := Terminal{Type: TypeTerminal, SchemaVersion: FlushFailedSchemaVersion, RecordedAtMS: 1, ToolUseID: "toolu_1p",
		SessionID: onePassSession, Outcome: OutcomeError, Reason: &reason, Phase: &phase}
	for _, c := range []onePassCase{
		{name: "spilled_exec", line: writerLine(t, spilled)},
		{name: "flush_marker", line: writerLine(t, marker)},
		{name: "unknown_noncrit", line: []byte(`{"type":"future_kind","schema_version":9,"critical":false}`)},
		{name: "unknown_crit", line: []byte(`{"type":"future_kind","schema_version":9,"critical":true}`)},
		{name: "refused_noncrit", line: []byte(`{"type":"declaration","schema_version":7,"critical":false}`)},
		{name: "refused_crit", line: []byte(`{"type":"declaration","schema_version":7,"critical":true}`)},
		{name: "refused_nokey", line: []byte(`{"type":"declaration","schema_version":7,"seq":1}`)},
		{name: "nil_line", line: nil},
		{name: "empty_line", line: []byte{}},
		{name: "not_json", line: []byte(`not json`)},
		{name: "foreign_order", line: []byte(`{"schema_version":6,"type":"declaration","seq":3,"tool_use_id":"toolu_f","session_id":"sess-1p"}`)},
		{name: "leading_space", line: append([]byte(" "), decl...)},
		{name: "escaped_kind", line: []byte(`{"type":"decl` + escapedA + `ration","schema_version":6,"seq":2,"tool_use_id":"toolu_e"}`)},
		{name: "no_comma", line: []byte(`{"type":"declaration","schema_version":6}`)},
		{name: "four_digits", line: []byte(`{"type":"declaration","schema_version":1000,"seq":1}`)},
		{name: "bad_body_type", line: []byte(`{"type":"declaration","schema_version":6,"seq":"x"}`)},
		{name: "bad_body_cut", line: []byte(`{"type":"declaration","schema_version":6,"seq":`)},
		{name: "bad_header", line: []byte(`{"type":"turn_end","schema_version":5,"critical":false,"seq":"one"}`)},
		{name: "trailing_bytes", line: append(append([]byte(nil), decl...), " x"...)},
		{name: "dup_type", differs: diffPair, line: withKey(decl, `"type":"terminal"`)},
		{name: "dup_version", differs: diffPair, line: []byte(`{"type":"declaration","schema_version":9,"critical":false,"seq":1,"schema_version":6}`)},
		{name: "dup_refused", differs: diffPair, line: []byte(`{"type":"declaration","schema_version":99,"critical":false,"type":"terminal"}`)},
		{name: "dup_default", differs: diffPair, line: []byte(`{"type":"turn_end","schema_version":5,"critical":false,"type":"prompt"}`)},
		{name: "case_type", differs: diffPair, line: withKey(decl, `"Type":"terminal"`)},
		{name: "crit_not_bool", differs: diffCritical, line: withKey(decl, `"critical":"no"`)},
	} {
		cases = append(cases, c)
	}
	return cases
}

// TestREC_OnePassEqualsTwoPass: over R4's fixture the one-pass reader and
// the two-pass reader it replaced agree on every count and every record,
// line by line, except on the documented differences, which are pinned both
// ways (D97). A line whose prefix pair and decoded pair disagree was read
// under its last key and is now Skipped: a repeated type or schema_version
// key, and, as case_type pins, a key that differs from type only in case,
// which encoding/json matches to the same field (a "Type" key after "type"
// was collected under it). A line of an accepted kind whose critical key is
// not a boolean was Skipped and is now read as the same line without that
// key.
func TestREC_OnePassEqualsTwoPass(t *testing.T) {
	for _, c := range onePassFixture(t) {
		t.Run(c.name, func(t *testing.T) {
			var old, got Run
			readTwoPass(&old, c.line)
			classifyRunLine(&got, c.line)
			switch c.differs {
			case diffPair:
				if old.Skipped != 0 {
					t.Errorf("two-pass skipped the line; the case no longer shows the difference")
				}
				if !reflect.DeepEqual(got, Run{Skipped: 1}) {
					t.Errorf("one-pass read %+v, want only Skipped 1 (D97)", got)
				}
			case diffCritical:
				var want Run
				readTwoPass(&want, bytes.Replace(c.line, []byte(`,"critical":"no"`), nil, 1))
				if !reflect.DeepEqual(old, Run{Skipped: 1}) {
					t.Errorf("two-pass read %+v, want only Skipped 1", old)
				}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("one-pass read %+v, want the line read as without its critical key: %+v", got, want)
				}
			default:
				if !reflect.DeepEqual(got, old) {
					t.Errorf("one-pass read %+v, two-pass %+v", got, old)
				}
			}
		})
	}
}

// TestREC_DuplicateTypeKeyIsSkipped: a line whose type or schema_version key
// comes twice with different values is not a record this store wrote, and
// is Skipped rather than collected under either pair (D97): on the accepted
// path, on the refused path (a refused first pair, an accepted last one) and
// in the default branch, and when the second key differs from the first only
// in case, which encoding/json matches all the same.
func TestREC_DuplicateTypeKeyIsSkipped(t *testing.T) {
	for _, c := range onePassFixture(t) {
		if c.differs != diffPair {
			continue
		}
		t.Run(c.name, func(t *testing.T) {
			var got Run
			classifyRunLine(&got, c.line)
			if !reflect.DeepEqual(got, Run{Skipped: 1}) {
				t.Errorf("read %+v, want only Skipped 1", got)
			}
		})
	}
}

// FuzzREC_OnePassEqualsTwoPass: on any line the one-pass reader agrees with
// the two-pass reader it replaced, or differs only as D97 documents: Skipped
// where the line gives type or schema_version twice, or read where a
// non-boolean critical key made the head fail. Which keys a line carries is
// read by encoding/json itself (documentedKeys), not by either reader. The
// seed corpus is R4's fixture.
func FuzzREC_OnePassEqualsTwoPass(f *testing.F) {
	for _, c := range onePassFixture(f) {
		f.Add(c.line)
	}
	f.Fuzz(func(t *testing.T, line []byte) {
		var old, got Run
		readTwoPass(&old, line)
		classifyRunLine(&got, line)
		if reflect.DeepEqual(old, got) {
			return
		}
		twice, badCritical := documentedKeys(line)
		switch {
		case twice && reflect.DeepEqual(got, Run{Skipped: 1}):
		case badCritical && reflect.DeepEqual(old, Run{Skipped: 1}) && got.Skipped == 0 && got.Ignored == 0:
		default:
			t.Errorf("%q: one-pass read %+v, two-pass %+v", line, got, old)
		}
	})
}

// documentedKeys reports which documented difference line can show: twice
// when keys that encoding/json decodes into type, or into schema_version,
// come more than once; badCritical when a key it decodes into critical
// holds anything but true, false or null. Each top-level key is matched by
// decoding it alone into a probe of the three fields, so the matching (case
// and all) is the library's, not either reader's.
func documentedKeys(line []byte) (twice, badCritical bool) {
	dec := json.NewDecoder(bytes.NewReader(line))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return false, false
	}
	types, versions := 0, 0
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		key, _ := tok.(string)
		var value json.RawMessage
		if dec.Decode(&value) != nil {
			break
		}
		one, err := json.Marshal(map[string]json.RawMessage{key: value})
		if err != nil {
			continue
		}
		var probe struct {
			Type          json.RawMessage `json:"type"`
			SchemaVersion json.RawMessage `json:"schema_version"`
			Critical      json.RawMessage `json:"critical"`
		}
		if json.Unmarshal(one, &probe) != nil {
			continue
		}
		if probe.Type != nil {
			types++
		}
		if probe.SchemaVersion != nil {
			versions++
		}
		if c := string(probe.Critical); c != "" && c != "true" && c != "false" && c != "null" {
			badCritical = true
		}
	}
	return types > 1 || versions > 1, badCritical
}
