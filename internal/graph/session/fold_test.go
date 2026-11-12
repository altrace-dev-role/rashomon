package session

import (
	"encoding/json"
	"math/rand"
	"reflect"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/events"
	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// TestEvent_MergeOrder (W-M4.2): a session's records and steer lines merge
// by (seq, record before line, at_unix_ms, id). A line's seq is its
// after_seq, the last record seq its decision saw, so it sorts after that
// record and before the next one, whatever the two clocks say.
func TestEvent_MergeOrder(t *testing.T) {
	want := []Event{
		{Seq: 0, Steer: true, AtUnixMS: 900, ID: "a0", Type: "steer"}, // the first prompt's line: no record yet
		{Seq: 1, Type: "declaration"},
		{Seq: 2, Type: "terminal"},
		{Seq: 2, Steer: true, AtUnixMS: 100, ID: "z9", Type: "steer"}, // an earlier clock than record 2's, still after it
		{Seq: 2, Steer: true, AtUnixMS: 200, ID: "a1", Type: "steer"},
		{Seq: 2, Steer: true, AtUnixMS: 200, ID: "b1", Type: "steer"},
		{Seq: 5, Type: "verify"},
		{Seq: 5, Steer: true, AtUnixMS: 50, ID: "c5", Type: "steer"},
		{Seq: 6, Type: "declaration"},
	}
	for _, e := range want {
		if e.Before(e) {
			t.Fatalf("%+v sorts before itself", e)
		}
	}
	r := rand.New(rand.NewSource(1))
	for round := 0; round < 50; round++ {
		got := append([]Event{}, want...)
		r.Shuffle(len(got), func(i, j int) { got[i], got[j] = got[j], got[i] })
		sort.Slice(got, func(i, j int) bool { return got[i].Before(got[j]) })
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("round %d: merged order\n got %+v\nwant %+v", round, got, want)
		}
	}
}

// TestEvent_Decode (W-M4.2): a record line decodes to its seq and type, a
// steer line to its after_seq, clock and id. A steer_evidence line has no
// after_seq and feeds no frontier field: not an event and not an error. A
// line the merge cannot place is an error.
func TestEvent_Decode(t *testing.T) {
	rec := `{"type":"declaration","schema_version":10,"seq":7,"recorded_at_unix_ms":1793897985945,"tool_use_id":"toolu_1","session_id":"s1","tool_name":"Read"}`
	ev, ok, err := RecordEvent([]byte(rec))
	if err != nil || !ok || !reflect.DeepEqual(ev, Event{Seq: 7, Type: "declaration"}) {
		t.Errorf("record: %+v, %v, %v", ev, ok, err)
	}
	line := `{"type":"steer","v":1,"id":"0a6742a0","at_unix_ms":1793897981372,"session_id":"s1","prompt_id":"6f1c2d3e-0000-4000-8000-00000000000a","kind":"unit","key":"example.com/m/internal/a@0a1b2c3d","after_seq":7,"model_text":"some words","user_text":"more words","state":"delivered"}`
	ev, ok, err = SteerEvent([]byte(line))
	if err != nil || !ok || !reflect.DeepEqual(ev, Event{Seq: 7, Steer: true, AtUnixMS: 1793897981372, ID: "0a6742a0", Type: "steer", State: "delivered", Key: "example.com/m/internal/a@0a1b2c3d", Kind: "unit", PromptID: "6f1c2d3e-0000-4000-8000-00000000000a"}) {
		t.Errorf("steer line: %+v, %v, %v", ev, ok, err)
	}
	evidence := `{"type":"steer_evidence","v":1,"at_unix_ms":1793897981400,"session_id":"s1","steer_id":"0a6742a0","event":"UserPromptSubmit"}`
	if ev, ok, err = SteerEvent([]byte(evidence)); err != nil || ok {
		t.Errorf("steer_evidence: %+v, %v, %v; want not an event, no error", ev, ok, err)
	}
	for name, c := range map[string]struct {
		steer bool
		line  string
	}{
		"record without seq":      {false, `{"type":"declaration","schema_version":10}`},
		"record without type":     {false, `{"seq":3}`},
		"record not json":         {false, `{"type":"declaration","seq":`},
		"steer without after_seq": {true, `{"type":"steer","id":"x","at_unix_ms":5}`},
		"steer without id":        {true, `{"type":"steer","at_unix_ms":5,"after_seq":3}`},
		"steer not json":          {true, `{"type":"steer",`},
	} {
		dec := RecordEvent
		if c.steer {
			dec = SteerEvent
		}
		if ev, ok, err := dec([]byte(c.line)); err == nil {
			t.Errorf("%s: %+v, %v, no error", name, ev, ok)
		}
	}
}

// TestEvent_DecodeWrites (W-M4.2, D1): a declaration's written paths decode
// as verify reads a write: the project prefix first, else the cwd prefix,
// cleaned, and nothing outside the project; a declaration that names a path
// outside it is marked (WroteOutside), its inside paths kept. Only a
// declaration names writes (an instructions record's file_path is the file
// it loaded), and a Read's declaration names no path.
func TestEvent_DecodeWrites(t *testing.T) {
	for _, c := range []struct {
		name, line string
		want       []string
		outside    bool
	}{
		{"edit under the project", `{"type":"declaration","seq":3,"tool_name":"Edit","file_path":"/w/p/internal/a/x.go","project":"/w/p","cwd":"/w/p"}`, []string{"internal/a/x.go"}, false},
		{"project before cwd", `{"type":"declaration","seq":3,"tool_name":"Edit","file_path":"/w/p/sub/x.go","project":"/w/p","cwd":"/w/p/sub"}`, []string{"sub/x.go"}, false},
		{"cwd without a project", `{"type":"declaration","seq":3,"tool_name":"Write","file_path":"/w/p/x.go","project":null,"cwd":"/w/p"}`, []string{"x.go"}, false},
		{"relative, cleaned", `{"type":"declaration","seq":3,"tool_name":"Edit","file_path":"./a/../b/c.go","project":"/w/p","cwd":"/w/p"}`, []string{"b/c.go"}, false},
		{"climbs out", `{"type":"declaration","seq":3,"tool_name":"Edit","file_path":"../q/x.go","project":"/w/p","cwd":"/w/p"}`, nil, true},
		{"outside the project", `{"type":"declaration","seq":3,"tool_name":"Edit","file_path":"/etc/hosts","project":"/w/p","cwd":"/w/p"}`, nil, true},
		{"an alias of the project", `{"type":"declaration","seq":3,"tool_name":"Edit","file_path":"/tmp/t/x.go","project":"/private/tmp/t","cwd":"/private/tmp/t"}`, nil, true},
		{"a sibling is not below", `{"type":"declaration","seq":3,"tool_name":"Edit","file_path":"/w/pab/x.go","project":"/w/p","cwd":null}`, nil, true},
		{"the project itself", `{"type":"declaration","seq":3,"tool_name":"Edit","file_path":"/w/p","project":"/w/p","cwd":null}`, nil, true},
		{"the project's parent", `{"type":"declaration","seq":3,"tool_name":"Edit","file_path":"/w/p/..","project":"/w/p","cwd":null}`, nil, true},
		{"a project with a trailing slash", `{"type":"declaration","seq":3,"tool_name":"Edit","file_path":"/w/p/x.go","project":"/w/p/","cwd":null}`, []string{"x.go"}, false},
		{"the root is no prefix", `{"type":"declaration","seq":3,"tool_name":"Edit","file_path":"/etc/hosts","project":"/","cwd":null}`, nil, true},
		{"a drive letter without case", `{"type":"declaration","seq":3,"tool_name":"Edit","file_path":"c:\\r\\a\\b.go","project":"C:\\r","cwd":null}`, []string{"a/b.go"}, false},
		{"another drive", `{"type":"declaration","seq":3,"tool_name":"Edit","file_path":"D:\\o\\x.go","project":"C:\\r","cwd":null}`, nil, true},
		{"a shell line's paths", `{"type":"declaration","seq":3,"tool_name":"Bash","written_paths":["a.go","/w/p/sub/b.go","/etc/x"],"project":"/w/p","cwd":"/w/p"}`, []string{"a.go", "sub/b.go"}, true},
		{"a withheld path", `{"type":"declaration","seq":3,"tool_name":"Edit","file_path":null,"file_path_withheld":"no_consent","project":"/w/p","cwd":"/w/p"}`, nil, false},
		{"an instructions file is not a write", `{"type":"instructions","seq":3,"file_path":"/w/p/CLAUDE.md","project":"/w/p","cwd":"/w/p"}`, nil, false},
		{"a shell line inside only", `{"type":"declaration","seq":3,"tool_name":"Bash","written_paths":["a.go","/w/p/sub/b.go"],"project":"/w/p","cwd":"/w/p"}`, []string{"a.go", "sub/b.go"}, false},
		{"a shell line outside only", `{"type":"declaration","seq":3,"tool_name":"Bash","written_paths":["/tmp/build.out"],"project":"/w/p","cwd":"/w/p"}`, nil, true},
		{"an instructions file outside is not a write", `{"type":"instructions","seq":3,"file_path":"/etc/x.md","project":"/w/p","cwd":"/w/p"}`, nil, false},
	} {
		ev, ok, err := RecordEvent([]byte(c.line))
		if err != nil || !ok || !reflect.DeepEqual(ev.Wrote, c.want) || ev.WroteOutside != c.outside {
			t.Errorf("%s: Wrote %q, outside %v, %v, %v; want %q, outside %v", c.name, ev.Wrote, ev.WroteOutside, ok, err, c.want, c.outside)
		}
	}
}

// TestInProject_DecidesAsVerify (W-M4.2, ruling 11:37 4b): inProject is a
// copy of verify's projectPath(relativise(...)) (its doc), so it must decide
// inside or outside as verify does. Each row pins the copy's decision and
// verify.RelevantUnder's, verify's exported reading of the same two rules,
// on .go paths, a class RelevantUnder counts, so its answer is the
// location's alone. Where the two differ the row says why: each is a
// difference inside verify, RelevantUnder against projectPath, and the copy
// follows projectPath, the reading verify's rule check uses. RelevantUnder
// reads absoluteness by filepath.IsAbs, the OS's rule, so its column holds
// off windows (the CI test job runs on linux); on windows only the copy's
// decision is checked.
func TestInProject_DecidesAsVerify(t *testing.T) {
	for _, c := range []struct {
		name, path, project, cwd string
		inside                   bool
		verifyDiffers            string // why RelevantUnder decides the other way; "" when it agrees
	}{
		{"the project prefix", "/w/p/internal/a/x.go", "/w/p", "/w/p", true, ""},
		{"the cwd prefix", "/w/c/internal/x.go", "/w/p", "/w/c", true, ""},
		{"the project before the cwd", "/w/p/sub/x.go", "/w/p", "/w/p/sub", true, ""},
		{"a symlinked cwd", "/tmp/r/internal/x.go", "/private/tmp/r", "/tmp/r", true, ""},
		{"an alias under a resolved cwd", "/tmp/r/internal/x.go", "/private/tmp/r", "/private/tmp/r", false, ""},
		{"a leading ./", "./internal/x.go", "/w/p", "/w/p", true, ""},
		{"./ then ..", "./../x.go", "/w/p", "/w/p", false, ""},
		{"a leading ..", "../q/x.go", "/w/p", "/w/p", false, ""},
		{"a climb that stays in", "internal/../x.go", "/w/p", "/w/p", true, ""},
		{"a climb out mid-path", "internal/../../x.go", "/w/p", "/w/p", false, "RelevantUnder does not clean, so a .. past the first element reads inside"},
		{"an absolute path outside", "/etc/x.go", "/w/p", "/w/p", false, ""},
		{"a sibling is not below", "/w/pab/x.go", "/w/p", "/w/p", false, ""},
		{"a root project is no prefix", "/etc/x.go", "/", "", false, ""},
		{"the same drive without case", `c:\r\internal\x.go`, `C:\r`, "", true, ""},
		{"another drive", `D:\o\x.go`, `C:\r`, "", false, "RelevantUnder has no drive-letter test, and off windows filepath.IsAbs reads D:/o/x.go as relative"},
		{"backslash separators", `\w\p\internal\x.go`, "/w/p", "/w/p", true, ""},
		{"a UNC share", `\\srv\share\p\internal\x.go`, `\\srv\share\p`, "", true, ""},
		{"a relative path", "internal/x.go", "/w/p", "/w/p", true, ""},
	} {
		project, cwd := c.project, c.cwd
		if _, got := inProject(c.path, &project, &cwd); got != c.inside {
			t.Errorf("%s: inProject %v, want %v", c.name, got, c.inside)
		}
		if runtime.GOOS == "windows" {
			continue
		}
		want := c.inside != (c.verifyDiffers != "")
		if got := verify.RelevantUnder(c.path, c.project, c.cwd); got != want {
			t.Errorf("%s: verify.RelevantUnder %v, want %v (%s)", c.name, got, want, c.verifyDiffers)
		}
	}
}

// ledgerLineSynthetic is a ledger line built by a test, never recorded: no
// ledger writer exists before the M2 merge (ledger condition 7), so every
// ledger line in this file is SYNTHETIC (condition 6). Its rows are real
// verify.Obligation values marshalled by encoding/json, so a decode reads
// verify's own keys; tail adds keys after the rows.
func ledgerLineSynthetic(t *testing.T, seq int64, rows []verify.Obligation, tail string) []byte {
	t.Helper()
	b, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	return []byte(`{"type":"ledger","seq":` + strconv.FormatInt(seq, 10) + `,"point":"stop","obligations":` + string(b) + tail + `}`)
}

// TestCheckpoint_PointIsVerifys (W-M4.2, ruling 12:03): the fold's Stop
// point is verify's, named in the fold because it imports no verify
// outside its tests.
func TestCheckpoint_PointIsVerifys(t *testing.T) {
	if stopPoint != verify.PointStop {
		t.Fatalf("stopPoint %q, verify.PointStop %q", stopPoint, verify.PointStop)
	}
}

// TestEvent_DecodeCheckpoint (W-M4.2, ruling 12:03): a verify record at
// point stop is a Stop's checkpoint; a verify record at another point or
// with none, and any other record carrying the key, are not.
func TestEvent_DecodeCheckpoint(t *testing.T) {
	for _, c := range []struct {
		name string
		line string
		want bool
	}{
		{"a stop checkpoint", `{"type":"verify","seq":12,"point":"stop","point_seq":11}`, true},
		{"a subagent_stop checkpoint", `{"type":"verify","seq":12,"point":"` + verify.PointSubagentStop + `","point_seq":11}`, false},
		{"no point", `{"type":"verify","seq":12}`, false},
		{"a ledger line is no checkpoint", `{"type":"ledger","seq":12,"point":"stop","obligations":[]}`, false},
		{"a declaration is no checkpoint", `{"type":"declaration","seq":12,"point":"stop"}`, false},
	} {
		ev, ok, err := RecordEvent([]byte(c.line))
		if err != nil || !ok || ev.Checkpoint != c.want {
			t.Errorf("%s: Checkpoint %v, %v, %v; want %v", c.name, ev.Checkpoint, ok, err, c.want)
		}
	}
}

// TestEvent_DecodeVerdictSynthetic (W-M4.2, rulings 12:03, 12:12): a ledger
// line's done_flagged, unmet and checkpoint_seq decode as its verdict, a
// present false or 0 as a value; a line without any one of the three, or
// with one null, has no verdict, never false or 0. Only a ledger line
// carries one. SYNTHETIC lines.
func TestEvent_DecodeVerdictSynthetic(t *testing.T) {
	rows := []verify.Obligation{{ID: "ob-a", Scope: verify.ScopeSession, Kind: verify.GateAfterWrite, Source: "spec", State: verify.ObligationOpen, Evidence: []int64{}}}
	type verdictOf struct {
		verdict, doneFlagged bool
		unmet                int
		at                   int64
	}
	for _, c := range []struct {
		name string
		line []byte
		want verdictOf
	}{
		{"a verdict", ledgerLineSynthetic(t, 11, rows, `,"checkpoint_seq":10,"done_flagged":true,"unmet":2`), verdictOf{true, true, 2, 10}},
		{"false and 0 are values", ledgerLineSynthetic(t, 11, rows, `,"checkpoint_seq":10,"done_flagged":false,"unmet":0`), verdictOf{true, false, 0, 10}},
		{"no done_flagged", ledgerLineSynthetic(t, 11, rows, `,"checkpoint_seq":10,"unmet":2`), verdictOf{}},
		{"no unmet", ledgerLineSynthetic(t, 11, rows, `,"checkpoint_seq":10,"done_flagged":true`), verdictOf{}},
		{"done_flagged null", ledgerLineSynthetic(t, 11, rows, `,"checkpoint_seq":10,"done_flagged":null,"unmet":2`), verdictOf{}},
		{"no checkpoint_seq", ledgerLineSynthetic(t, 11, rows, `,"done_flagged":true,"unmet":2`), verdictOf{}},
		{"an older line", ledgerLineSynthetic(t, 11, rows, ""), verdictOf{}},
		{"a verify record is no ledger line", []byte(`{"type":"verify","seq":11,"point":"stop","checkpoint_seq":10,"done_flagged":true,"unmet":2}`), verdictOf{}},
	} {
		ev, ok, err := RecordEvent(c.line)
		got := verdictOf{ev.Verdict, ev.DoneFlagged, ev.Unmet, ev.CheckpointSeq}
		if err != nil || !ok || got != c.want {
			t.Errorf("%s: %+v, %v, %v; want %+v", c.name, got, ok, err, c.want)
		}
	}
}

// sessionLineSynthetic is a session record built by a test, never recorded:
// no SessionStart writer exists before W-M4.3 and W-M4.4, so every session
// record in this file is SYNTHETIC. It is a real store.Session marshalled
// by encoding/json, so a decode reads the store's own keys.
func sessionLineSynthetic(t *testing.T, seq int64, source string) []byte {
	t.Helper()
	b, err := json.Marshal(store.Session{EventHeader: store.EventHeader{Type: store.TypeSession, Seq: &seq}, Source: source})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestEvent_DecodeBoundarySynthetic (W-M4.2, ruling 12:03): a session record
// whose SessionStart source is compact or resume is a boundary; startup,
// clear, no source, and any other record carrying the key are not.
// SYNTHETIC session records.
func TestEvent_DecodeBoundarySynthetic(t *testing.T) {
	for _, c := range []struct {
		name     string
		line     []byte
		boundary bool
	}{
		{"compact", sessionLineSynthetic(t, 4, events.Compact), true},
		{"resume", sessionLineSynthetic(t, 4, events.Resume), true},
		{"startup", sessionLineSynthetic(t, 4, "startup"), false},
		{"clear", sessionLineSynthetic(t, 4, events.Clear), false},
		{"no source", []byte(`{"type":"session","seq":4}`), false},
		{"source null", []byte(`{"type":"session","seq":4,"source":null}`), false},
		{"a compact record is no boundary", []byte(`{"type":"compact","seq":4,"trigger":"auto","source":"compact"}`), false},
		{"a declaration is no boundary", []byte(`{"type":"declaration","seq":4,"source":"resume"}`), false},
	} {
		ev, ok, err := RecordEvent(c.line)
		if err != nil || !ok || ev.Boundary != c.boundary || ev.Seq != 4 {
			t.Errorf("%s: Boundary %v, seq %d, %v, %v; want %v, 4", c.name, ev.Boundary, ev.Seq, ok, err, c.boundary)
		}
	}
}

// TestEvent_DecodeLedgerSynthetic (W-M4.2, D3): a ledger line decodes to its
// rows' id, kind, unit and state by verify.Obligation's keys, with its two
// cut counts, 0 when absent. Scope is opaque: a scope no list names decodes
// as any other. Only a ledger line's rows are the ledger: a verify record or
// a declaration carrying the same keys is not one. SYNTHETIC lines.
func TestEvent_DecodeLedgerSynthetic(t *testing.T) {
	rows := []verify.Obligation{
		{ID: "ob-a", Scope: verify.ScopeSession, Kind: verify.GateAfterWrite, Source: "spec", State: verify.ObligationMet, Evidence: []int64{4, 9}},
		{ID: "ob-b", Scope: "turn", Kind: verify.ExpectedEvidence, Source: "requirement", State: verify.ObligationUnknown, Evidence: []int64{}, Unit: "g:r1#unit:go:internal/a"},
		{ID: "ob-c", Scope: "a scope no list names", Kind: verify.PlatformImpact, Source: "graph", State: verify.ObligationOpen, Unit: "g:r1#unit:go:internal/b"},
		{ID: "ob-d", Scope: verify.ScopeSession, Kind: verify.ExitUnmasked, Source: "run", State: verify.ObligationUnmet},
	}
	var all []ObligationRef
	for _, r := range rows {
		all = append(all, ObligationRef{ID: r.ID, Kind: string(r.Kind), Unit: r.Unit, State: string(r.State)})
	}
	for _, c := range []struct {
		name              string
		line              []byte
		ledger            bool
		obs               []ObligationRef
		omitted, unitsCut int
	}{
		{"a ledger line", ledgerLineSynthetic(t, 12, rows, `,"obligations_omitted":2,"unit_rows_cut":5`), true, all, 2, 5},
		{"no cut keys", ledgerLineSynthetic(t, 12, rows[:1], ""), true, all[:1], 0, 0},
		{"no rows", ledgerLineSynthetic(t, 12, []verify.Obligation{}, ""), true, []ObligationRef{}, 0, 0},
		{"rows null", ledgerLineSynthetic(t, 12, nil, ""), true, []ObligationRef{}, 0, 0},
		{"a verify record is no ledger line", []byte(`{"type":"verify","seq":12,"point":"stop","obligations":[{"id":"ob-a","state":"met"}],"unit_rows_cut":5}`), false, nil, 0, 0},
		{"a declaration is no ledger line", []byte(`{"type":"declaration","seq":12,"obligations":[{"id":"ob-a","state":"met"}],"obligations_omitted":2}`), false, nil, 0, 0},
	} {
		ev, ok, err := RecordEvent(c.line)
		if err != nil || !ok || ev.Ledger != c.ledger || !reflect.DeepEqual(ev.Obligations, c.obs) || ev.ObligationsOmitted != c.omitted || ev.UnitRowsCut != c.unitsCut {
			t.Errorf("%s: Ledger %v, %+v, omitted %d, cut %d, %v, %v; want %v, %+v, %d, %d", c.name, ev.Ledger, ev.Obligations, ev.ObligationsOmitted, ev.UnitRowsCut, ok, err, c.ledger, c.obs, c.omitted, c.unitsCut)
		}
	}
}

// TestEvent_DecodeWithheld (W-M4.2, D1): a declaration whose write path the
// record withheld is marked, whatever the reason, so the frontier can say
// Touched is cut. A withheld cwd or project alone (a Read below record scope
// v5) is no write, and only a declaration's paths are writes. Below v5 a
// shell line's list is keyed withheld only when it named a path
// (store.GateDeclaration), so one that named none is not marked.
func TestEvent_DecodeWithheld(t *testing.T) {
	for _, c := range []struct {
		name, line string
		want       bool
		wrote      []string
	}{
		{"a file tool below v5", `{"type":"declaration","seq":3,"tool_name":"Edit","file_path":null,"file_path_withheld":"no_consent","project":null,"project_withheld":"no_consent","cwd":null,"cwd_withheld":"no_consent"}`, true, nil},
		{"a shell list below v5", `{"type":"declaration","seq":3,"tool_name":"Bash","written_paths":null,"written_paths_complete":false,"written_paths_withheld":"no_consent","project":null,"project_withheld":"no_consent"}`, true, nil},
		{"a shell line with no list below v5", `{"type":"declaration","seq":3,"tool_name":"Bash","command":null,"command_withheld":"no_consent","written_paths":null,"written_paths_complete":false,"project":null,"project_withheld":"no_consent","cwd":null,"cwd_withheld":"no_consent"}`, false, nil},
		{"an oversize path", `{"type":"declaration","seq":3,"tool_name":"Write","file_path":null,"file_path_withheld":"oversize","project":"/w/p","cwd":"/w/p"}`, true, nil},
		{"a path in clear", `{"type":"declaration","seq":3,"tool_name":"Edit","file_path":"/w/p/x.go","file_path_withheld":null,"project":"/w/p","cwd":"/w/p"}`, false, []string{"x.go"}},
		{"a read below v5", `{"type":"declaration","seq":3,"tool_name":"Read","project":null,"project_withheld":"no_consent","cwd":null,"cwd_withheld":"no_consent"}`, false, nil},
		{"an instructions file", `{"type":"instructions","seq":3,"file_path":null,"file_path_withheld":"oversize"}`, false, nil},
	} {
		ev, ok, err := RecordEvent([]byte(c.line))
		if err != nil || !ok || ev.WroteWithheld != c.want || !reflect.DeepEqual(ev.Wrote, c.wrote) {
			t.Errorf("%s: WroteWithheld %v, Wrote %q, %v, %v; want %v and %q", c.name, ev.WroteWithheld, ev.Wrote, ok, err, c.want, c.wrote)
		}
	}
}

// TestEvent_HoldsNoWords (W-M4.2, AC-M4.1): an event keeps the order key and
// the fields a rule reads, never a line's words (model_text, user_text). A
// string field not named here fails by name.
func TestEvent_HoldsNoWords(t *testing.T) {
	named := map[string]string{
		"ID":       "steer line id",
		"Type":     "record type or steer.LineType",
		"State":    "a closed steer.LineState name",
		"Key":      "a finding's dedupe key: a unit id and a digest",
		"Kind":     "a closed steer.Kind name, or empty",
		"PromptID": "a prompt id (a UUID), never the prompt's words",
		"Wrote[]":  "a project-relative path a call named as written, never a file's contents",
		"Units[]":  "a unit's path as a steer line's graph line holds it, or its keyed digest at record scope v4",

		"Obligations[].ID":    "an obligation id, verify.ObligationID's digest",
		"Obligations[].Kind":  "a closed verify.ObligationKind name",
		"Obligations[].Unit":  "a unit's global id, or its digest below record scope v5",
		"Obligations[].State": "a closed verify.ObligationState name",
	}
	var got []string
	stringLeaves(reflect.TypeOf(Event{}), "", &got)
	seen := map[string]bool{}
	for _, p := range got {
		seen[p] = true
		if _, ok := named[p]; !ok {
			t.Errorf("Event.%s: a string not named as a reference; name it or keep words out", p)
		}
	}
	for p := range named {
		if !seen[p] {
			t.Errorf("Event.%s: named but gone", p)
		}
	}
}

// TestStep_Delivered (W-M4.2): a steer line in state delivered becomes one
// LineRef at its after_seq with its id, key and kind. A line in any other
// state and a record leave Delivered as it was.
func TestStep_Delivered(t *testing.T) {
	f := newFrontier("s1")
	Step(&f, Event{Seq: 1, Type: "declaration"})
	Step(&f, Event{Seq: 1, Steer: true, AtUnixMS: 5, ID: "d1", Type: "steer", State: "delivered", Key: "k1"})
	for i, st := range steer.AllLineStates() {
		if st != steer.LineStateDelivered {
			Step(&f, Event{Seq: 2, Steer: true, AtUnixMS: int64(10 + i), ID: "n-" + string(st), Type: "steer", State: string(st), Key: "kn"})
		}
	}
	Step(&f, Event{Seq: 3, Steer: true, AtUnixMS: 30, ID: "d4", Type: "steer", State: "delivered", Key: "k4", Kind: "unit"})
	want := []LineRef{{ID: "d1", Key: "k1", Seq: 1}, {ID: "d4", Key: "k4", Kind: "unit", Seq: 3}}
	if !reflect.DeepEqual(f.Delivered, want) {
		t.Fatalf("Delivered\n got %+v\nwant %+v", f.Delivered, want)
	}
}

// TestStep_DeliveredOncePerID (W-M4.2): one LineRef per steer line id, the
// uniqueness canon's order rests on; a line seen again keeps its first ref.
func TestStep_DeliveredOncePerID(t *testing.T) {
	f := newFrontier("s1")
	Step(&f, Event{Seq: 4, Steer: true, AtUnixMS: 5, ID: "d1", Type: "steer", State: "delivered", Key: "k1"})
	Step(&f, Event{Seq: 9, Steer: true, AtUnixMS: 9, ID: "d1", Type: "steer", State: "delivered", Key: "k1"})
	if want := []LineRef{{ID: "d1", Key: "k1", Seq: 4}}; !reflect.DeepEqual(f.Delivered, want) {
		t.Fatalf("Delivered\n got %+v\nwant %+v", f.Delivered, want)
	}
}

// TestStep_Intents (W-M4.2, D2): one IntentRef per prompt id in the steer
// log, in order of first appearance from 1, at its first line's after_seq,
// whatever that line's state; a briefing line (no prompt id) adds none.
func TestStep_Intents(t *testing.T) {
	f := newFrontier("s1")
	for _, ev := range []Event{
		{Seq: 0, Steer: true, AtUnixMS: 1, ID: "b0", Type: "steer", State: "delivered", Key: "graph"},
		{Seq: 0, Steer: true, AtUnixMS: 2, ID: "a1", Type: "steer", State: "suppressed", Key: "k", PromptID: "p-one"},
		{Seq: 2, Steer: true, AtUnixMS: 3, ID: "a2", Type: "steer", State: "delivered", Key: "k2", PromptID: "p-one"},
		{Seq: 3, Type: "declaration"},
		{Seq: 5, Steer: true, AtUnixMS: 4, ID: "a3", Type: "steer", State: "superseded", Key: "k3", PromptID: "p-two"},
	} {
		Step(&f, ev)
	}
	want := []IntentRef{
		{ID: string(graph.PromptNodeID("s1", 1)), Digest: "p-one", StatedAt: 0, Status: ReqOpen},
		{ID: string(graph.PromptNodeID("s1", 2)), Digest: "p-two", StatedAt: 5, Status: ReqOpen},
	}
	if !reflect.DeepEqual(f.Intents, want) {
		t.Fatalf("Intents\n got %+v\nwant %+v", f.Intents, want)
	}
}

// TestStep_Touched (W-M4.2, D1): every path a declaration wrote joins
// Touched as its graph.FileID, sorted and unique, so the same writes in any
// order fold to the same set; a steer line and a record without writes
// leave it as it was.
func TestStep_Touched(t *testing.T) {
	evs := []Event{
		{Seq: 1, Type: "declaration", Wrote: []string{"internal/b/y.go"}},
		{Seq: 2, Type: "declaration", Wrote: []string{"a.go", "internal/b/y.go"}},
		{Seq: 2, Steer: true, AtUnixMS: 1, ID: "d1", Type: "steer", State: "delivered", Key: "k"},
		{Seq: 3, Type: "terminal"},
		{Seq: 4, Type: "declaration", Wrote: []string{"cmd/m/main.go"}},
	}
	want := []string{string(graph.FileID("a.go")), string(graph.FileID("cmd/m/main.go")), string(graph.FileID("internal/b/y.go"))}
	r := rand.New(rand.NewSource(1))
	for round := 0; round < 20; round++ {
		f := newFrontier("s1")
		for _, ev := range evs {
			Step(&f, ev)
		}
		if !reflect.DeepEqual(f.Touched, want) {
			t.Fatalf("round %d: Touched\n got %q\nwant %q", round, f.Touched, want)
		}
		r.Shuffle(len(evs), func(i, j int) { evs[i], evs[j] = evs[j], evs[i] })
	}
}

// TestStep_TouchedWithheld (W-M4.2, D1): each declaration whose write path
// was withheld counts once, so a frontier below record scope v5 says its
// empty Touched is cut, not that the session wrote nothing.
func TestStep_TouchedWithheld(t *testing.T) {
	f := newFrontier("s1")
	for _, ev := range []Event{
		{Seq: 1, Type: "declaration", WroteWithheld: true},
		{Seq: 2, Type: "declaration"},
		{Seq: 2, Steer: true, AtUnixMS: 1, ID: "d1", Type: "steer", State: "delivered", Key: "k"},
		{Seq: 3, Type: "declaration", WroteWithheld: true},
		{Seq: 4, Type: "declaration", Wrote: []string{"a.go"}},
	} {
		Step(&f, ev)
	}
	if f.TouchedWithheld != 2 || !reflect.DeepEqual(f.Touched, []string{string(graph.FileID("a.go"))}) {
		t.Fatalf("TouchedWithheld %d, Touched %q; want 2 and [file:a.go]", f.TouchedWithheld, f.Touched)
	}
}

// TestStep_BoundarySynthetic (W-M4.2, ruling 12:03): LastBoundary is the
// seq of the last boundary; a session record that is none (startup, clear)
// and any other record leave it as it was. SYNTHETIC events.
func TestStep_BoundarySynthetic(t *testing.T) {
	f := newFrontier("s1")
	for _, ev := range []Event{
		{Seq: 3, Type: store.TypeSession, Boundary: true},
		{Seq: 4, Type: "declaration", Wrote: []string{"a.go"}},
		{Seq: 5, Type: store.TypeSession},
		{Seq: 8, Type: store.TypeSession, Boundary: true},
		{Seq: 9, Type: store.TypeSession},
		{Seq: 10, Type: "verify"},
	} {
		Step(&f, ev)
	}
	if f.LastBoundary != 8 {
		t.Fatalf("LastBoundary %d, want 8", f.LastBoundary)
	}
}

// TestStep_VerdictSynthetic (W-M4.2, rulings 12:03, 12:12): each Stop
// checkpoint is in StopsWithoutVerdict until a ledger line's verdict names
// it; a verdict is copied as its checkpoint's VerdictRef, one per
// checkpoint, the first kept; a ledger line with no verdict adds none; and
// each checkpoint is in Verdicts or StopsWithoutVerdict (conservation).
// SYNTHETIC events.
func TestStep_VerdictSynthetic(t *testing.T) {
	rows := []ObligationRef{{ID: "ob-a", Kind: "gate_after_write", State: "open"}}
	f := newFrontier("s1")
	stops := 0
	for _, ev := range []Event{
		{Seq: 10, Type: "verify", Checkpoint: true},
		{Seq: 11, Type: ledgerType, Ledger: true, Obligations: rows, Verdict: true, DoneFlagged: false, Unmet: 1, CheckpointSeq: 10},
		{Seq: 20, Type: "verify", Checkpoint: true},
		{Seq: 21, Type: ledgerType, Ledger: true, Obligations: rows},
		{Seq: 30, Type: "verify", Checkpoint: true},
		{Seq: 31, Type: ledgerType, Ledger: true, Obligations: rows, Verdict: true, DoneFlagged: true, Unmet: 0, CheckpointSeq: 30},
		{Seq: 32, Type: ledgerType, Ledger: true, Obligations: rows, Verdict: true, DoneFlagged: false, Unmet: 5, CheckpointSeq: 30},
		{Seq: 40, Type: "verify", Checkpoint: true},
	} {
		Step(&f, ev)
		if ev.Checkpoint {
			stops++
		}
	}
	if len(f.Verdicts)+len(f.StopsWithoutVerdict) != stops {
		t.Errorf("%d verdicts + %d without one, want %d stop checkpoints", len(f.Verdicts), len(f.StopsWithoutVerdict), stops)
	}
	want := []VerdictRef{{Seq: 10, DoneFlagged: false, Unmet: 1}, {Seq: 30, DoneFlagged: true, Unmet: 0}}
	if !reflect.DeepEqual(f.Verdicts, want) || !reflect.DeepEqual(f.StopsWithoutVerdict, []int64{20, 40}) {
		t.Fatalf("Verdicts %+v, without %v; want %+v, [20 40]", f.Verdicts, f.StopsWithoutVerdict, want)
	}
}

// TestStep_VerdictBindsBySeqSynthetic (W-M4.2, ruling 12:12 3): a ledger
// line binds to the checkpoint its checkpoint_seq names, not to the record
// before it, since a record can land between a Stop's two appends: here a
// write declaration and a subagent_stop checkpoint. SYNTHETIC events.
func TestStep_VerdictBindsBySeqSynthetic(t *testing.T) {
	f := newFrontier("s1")
	for _, ev := range []Event{
		{Seq: 10, Type: "verify", Checkpoint: true},
		{Seq: 11, Type: "declaration"},
		{Seq: 12, Type: "verify"},
		{Seq: 13, Type: ledgerType, Ledger: true, Obligations: []ObligationRef{}, Verdict: true, DoneFlagged: true, Unmet: 2, CheckpointSeq: 10},
	} {
		Step(&f, ev)
	}
	if want := []VerdictRef{{Seq: 10, DoneFlagged: true, Unmet: 2}}; !reflect.DeepEqual(f.Verdicts, want) || len(f.StopsWithoutVerdict) != 0 {
		t.Fatalf("Verdicts %+v, without %v; want %+v, none", f.Verdicts, f.StopsWithoutVerdict, want)
	}
}

// TestStep_VerdictWithoutCheckpointSynthetic (W-M4.2, ruling 12:12 3): a
// ledger line whose checkpoint_seq names no stop checkpoint read before it,
// a verify record at another point or one at a later seq, adds no verdict,
// and its rows still apply. SYNTHETIC events.
func TestStep_VerdictWithoutCheckpointSynthetic(t *testing.T) {
	rows := []ObligationRef{{ID: "ob-a", Kind: "gate_after_write", State: "unmet"}}
	f := newFrontier("s1")
	for _, ev := range []Event{
		{Seq: 5, Type: "verify"},
		{Seq: 10, Type: ledgerType, Ledger: true, Obligations: rows, Verdict: true, DoneFlagged: true, Unmet: 1, CheckpointSeq: 5},
		{Seq: 20, Type: ledgerType, Ledger: true, Obligations: rows, Verdict: true, DoneFlagged: true, Unmet: 1, CheckpointSeq: 21},
		{Seq: 21, Type: "verify", Checkpoint: true},
	} {
		Step(&f, ev)
	}
	if len(f.Verdicts) != 0 || !reflect.DeepEqual(f.StopsWithoutVerdict, []int64{21}) {
		t.Errorf("Verdicts %+v, without %v; want none, [21]", f.Verdicts, f.StopsWithoutVerdict)
	}
	if !reflect.DeepEqual(f.Obligations, rows) || f.ObligationsAt != 20 {
		t.Errorf("Obligations %+v at %d; want %+v at 20: rows apply without a verdict", f.Obligations, f.ObligationsAt, rows)
	}
}

// TestStep_VerdictLeavesCopies (W-M4.2): a verdict cuts its checkpoint from
// StopsWithoutVerdict without writing into the list that a Frontier copied
// before the Step still holds. SYNTHETIC events.
func TestStep_VerdictLeavesCopies(t *testing.T) {
	f := newFrontier("s1")
	Step(&f, Event{Seq: 10, Type: "verify", Checkpoint: true})
	Step(&f, Event{Seq: 20, Type: "verify", Checkpoint: true})
	g := f
	Step(&f, Event{Seq: 21, Type: ledgerType, Ledger: true, Obligations: []ObligationRef{}, Verdict: true, Unmet: 1, CheckpointSeq: 10})
	if !reflect.DeepEqual(g.StopsWithoutVerdict, []int64{10, 20}) || !reflect.DeepEqual(f.StopsWithoutVerdict, []int64{20}) {
		t.Errorf("copy %v, stepped %v; want [10 20], [20]", g.StopsWithoutVerdict, f.StopsWithoutVerdict)
	}
}

// TestStep_CheckpointWithoutLedgerSynthetic (W-M4.2, rulings 12:03 3, 12:12
// 5): a Stop whose checkpoint no ledger line follows (a kill, a lock
// timeout) keeps the obligations of the Stop before, as of that line's seq
// (ledger condition 4), but gets no verdict: a verdict belongs to one Stop,
// and the Stop is in StopsWithoutVerdict, never read as not done.
// SYNTHETIC events.
func TestStep_CheckpointWithoutLedgerSynthetic(t *testing.T) {
	rows := []ObligationRef{{ID: "ob-a", Kind: "gate_after_write", State: "unmet"}}
	f := newFrontier("s1")
	for _, ev := range []Event{
		{Seq: 10, Type: "verify", Checkpoint: true},
		{Seq: 11, Type: ledgerType, Ledger: true, Obligations: rows, Verdict: true, DoneFlagged: false, Unmet: 1, CheckpointSeq: 10},
		{Seq: 20, Type: "verify", Checkpoint: true},
	} {
		Step(&f, ev)
	}
	if !reflect.DeepEqual(f.Obligations, rows) || f.ObligationsAt != 11 {
		t.Errorf("Obligations %+v at %d; want %+v at 11: they carry over", f.Obligations, f.ObligationsAt, rows)
	}
	if want := []VerdictRef{{Seq: 10, DoneFlagged: false, Unmet: 1}}; !reflect.DeepEqual(f.Verdicts, want) || !reflect.DeepEqual(f.StopsWithoutVerdict, []int64{20}) {
		t.Errorf("Verdicts %+v, without %v; want %+v, [20]: a verdict does not carry over", f.Verdicts, f.StopsWithoutVerdict, want)
	}
}

// TestStep_LedgerSynthetic (W-M4.2, D3): the last ledger line is the
// frontier's obligations, as of its seq, with its own cut counts, each line
// replacing the one before; one ref per id, the first kept; each state as
// written, so unknown stays unknown and open stays open; and a record that
// is no ledger line leaves them as they were (ledger condition 4).
// SYNTHETIC events: no ledger writer exists before the M2 merge.
func TestStep_LedgerSynthetic(t *testing.T) {
	first := []ObligationRef{{ID: "ob-a", Kind: "gate_after_write", State: "unmet"}}
	second := []ObligationRef{
		{ID: "ob-b", Kind: "expected_evidence", Unit: "g:r1#unit:go:internal/a", State: "unknown"},
		{ID: "ob-a", Kind: "gate_after_write", State: "met"},
		{ID: "ob-b", Kind: "expected_evidence", Unit: "g:r1#unit:go:internal/z", State: "met"},
		{ID: "ob-c", Kind: "platform_impact", State: "open"},
	}
	f := newFrontier("s1")
	for _, ev := range []Event{
		{Seq: 3, Type: ledgerType, Ledger: true, Obligations: first, ObligationsOmitted: 1, UnitRowsCut: 4},
		{Seq: 5, Type: ledgerType, Ledger: true, Obligations: second, UnitRowsCut: 3},
		{Seq: 6, Type: "declaration", Wrote: []string{"a.go"}},
		{Seq: 6, Steer: true, AtUnixMS: 1, ID: "d1", Type: "steer", State: "delivered", Key: "k"},
		{Seq: 7, Type: "verify"},
	} {
		Step(&f, ev)
	}
	want := []ObligationRef{second[0], second[1], second[3]}
	if !reflect.DeepEqual(f.Obligations, want) || f.ObligationsAt != 5 || f.ObligationsOmitted != 0 || f.UnitRowsCut != 3 {
		t.Fatalf("Obligations %+v at %d, omitted %d, cut %d; want %+v at 5, 0, 3", f.Obligations, f.ObligationsAt, f.ObligationsOmitted, f.UnitRowsCut, want)
	}
}

// TestStep_TouchedOutside (W-M4.2, ruling 11:37 3b): each declaration that
// named a path outside the project counts once, beside the inside paths it
// adds to Touched, and the two cut counts move apart.
func TestStep_TouchedOutside(t *testing.T) {
	f := newFrontier("s1")
	for _, ev := range []Event{
		{Seq: 1, Type: "declaration", WroteOutside: true},
		{Seq: 2, Type: "declaration", Wrote: []string{"a.go"}, WroteOutside: true},
		{Seq: 2, Steer: true, AtUnixMS: 1, ID: "d1", Type: "steer", State: "delivered", Key: "k"},
		{Seq: 3, Type: "declaration", Wrote: []string{"b.go"}},
		{Seq: 4, Type: "declaration", WroteWithheld: true},
	} {
		Step(&f, ev)
	}
	want := []string{string(graph.FileID("a.go")), string(graph.FileID("b.go"))}
	if f.TouchedOutside != 2 || f.TouchedWithheld != 1 || !reflect.DeepEqual(f.Touched, want) {
		t.Fatalf("TouchedOutside %d, TouchedWithheld %d, Touched %q; want 2, 1 and %q", f.TouchedOutside, f.TouchedWithheld, f.Touched, want)
	}
}

// keyedUnit is a unit path's keyed digest as the record form holds it at
// record scope v4: 64 lowercase hex, no path.
const keyedUnit = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// TestEvent_DecodeUnitsSynthetic (W-M4.2, D6, ruling 12:52): a steer line's
// unit graph lines, in slots line1..line5, decode as Units in slot order,
// each its path, or its keyed digest when the record form left no path.
// A code line, an empty line and a pattern's other slot keys are not
// units, and a unit line with neither names none. SYNTHETIC lines.
func TestEvent_DecodeUnitsSynthetic(t *testing.T) {
	head := `{"type":"steer","id":"u1","at_unix_ms":5,"after_seq":7,"state":"delivered","key":"graph","kind":"","prompt_id":"p1"`
	for _, c := range []struct {
		name  string
		slots string
		want  []string
	}{
		{"units in slot order", `,"slots":{"line1":{"kind":"unit","path":"internal/a","digest":""},"line2":{"kind":"code","path":"internal/b/x.go"},` +
			`"line3":{"kind":"","path":""},"line5":{"kind":"unit","path":"internal/e"},"gate":"go test"}`, []string{"internal/a", "internal/e"}},
		{"a keyed digest at v4", `,"slots":{"line4":{"kind":"unit","path":"","digest":"` + keyedUnit + `"}}`, []string{keyedUnit}},
		{"a unit line with neither", `,"slots":{"line1":{"kind":"unit","path":"","digest":""}}`, nil},
		{"no slots", ``, nil},
		{"null slots", `,"slots":null`, nil},
	} {
		ev, ok, err := SteerEvent([]byte(head + c.slots + "}"))
		if err != nil || !ok || !slices.Equal(ev.Units, c.want) {
			t.Errorf("%s: Units %q, %v, %v; want %q", c.name, ev.Units, ok, err, c.want)
		}
	}
}

// TestStep_ReqsSynthetic (W-M4.2, D6, rulings 12:52 (2), (3), 13:04 (3)): a
// delivered line states one opened_unit Req per unit, keyed Pred:Arg, at
// its after_seq; a suppressed line states none, and a unit stated again
// keeps its first StatedAt. A Req is met at the first write after its line
// at its path or under it, and never again. The boundary in three cases,
// the events put in the merged order by Before: a write before the line
// (5) and a write at the line's own after_seq (10, a record sorts first)
// leave internal/early open; writes after it (14, 15) meet. A sibling that
// only shares the prefix (13) meets nothing. A Req of another predicate on
// the same path stays open: the fold sets met only what it can see.
// SYNTHETIC events.
func TestStep_ReqsSynthetic(t *testing.T) {
	f := newFrontier("s1")
	other := ReqState{ID: steer.PredEditedOrVerified + ":internal/platform", Pred: steer.PredEditedOrVerified, Arg: "internal/platform", Status: ReqOpen, StatedAt: 3}
	f.Reqs = []ReqState{other}
	line := func(seq int64, id, state string, units ...string) Event {
		return Event{Seq: seq, Steer: true, AtUnixMS: seq, ID: id, Type: steer.LineType, State: state, Key: "graph", Units: units}
	}
	write := func(seq int64, p string) Event {
		return Event{Seq: seq, Type: store.TypeDeclaration, Wrote: []string{p}}
	}
	evs := []Event{
		write(5, "internal/early/a.go"),
		line(10, "l1", "delivered", "internal/platform", "cmd/x", "internal/early"),
		write(10, "internal/early/b.go"),
		line(11, "l2", "suppressed", "internal/other"),
		line(12, "l3", "delivered", "internal/platform"),
		write(13, "internal/platformer/x.go"),
		write(14, "cmd/x"),
		write(15, "internal/platform/b.go"),
		write(16, "internal/platform/c.go"),
	}
	sort.SliceStable(evs, func(i, j int) bool { return evs[i].Before(evs[j]) })
	for _, ev := range evs {
		Step(&f, ev)
	}
	unit := func(arg string, status ReqStatus, met int64) ReqState {
		return ReqState{ID: steer.PredOpenedUnit + ":" + arg, Pred: steer.PredOpenedUnit, Arg: arg, Status: status, StatedAt: 10, MetAt: met}
	}
	want := []ReqState{other, unit("internal/platform", ReqMet, 15), unit("cmd/x", ReqMet, 14), unit("internal/early", ReqOpen, 0)}
	if !slices.Equal(f.Reqs, want) {
		t.Errorf("Reqs:\n%+v\nwant\n%+v", f.Reqs, want)
	}
}

// TestStep_LeavesCopies (W-M4.2): a Step leaves a Frontier copied before it
// as it was, byte for byte, while the stepped one gains the write's file in
// order and meets the Req. Touched has room past its length, so an insert
// that shifts in place would write into the copy's array. SYNTHETIC events.
func TestStep_LeavesCopies(t *testing.T) {
	f := newFrontier("s1")
	f.Touched = append(make([]string, 0, 8), string(graph.FileID("a")), string(graph.FileID("c")))
	f.Reqs = []ReqState{{ID: steer.PredOpenedUnit + ":b", Pred: steer.PredOpenedUnit, Arg: "b", Status: ReqOpen, StatedAt: 1}}
	g := f
	before, err := MarshalFrontier(g)
	if err != nil {
		t.Fatal(err)
	}
	Step(&f, Event{Seq: 2, Type: store.TypeDeclaration, Wrote: []string{"b"}})
	if after, _ := MarshalFrontier(g); string(after) != string(before) {
		t.Errorf("copy changed:\n%s\nwant\n%s", after, before)
	}
	wantTouched := []string{string(graph.FileID("a")), string(graph.FileID("b")), string(graph.FileID("c"))}
	if !slices.Equal(f.Touched, wantTouched) || f.Reqs[0].Status != ReqMet || f.Reqs[0].MetAt != 2 {
		t.Errorf("stepped: Touched %q, Req %+v; want %q and met at 2", f.Touched, f.Reqs[0], wantTouched)
	}
}

// TestStep_KnownHoleT_SubdirLookalikeMeets (W-M4.2, debt (t), ruling 13:04
// (4)): a KNOWN HOLE, pinned so that its fix must flip this test. A slot
// path is relative to the hook's working directory, a written path to the
// project, and the steer line names no cwd. A line written from /p/sub
// names the unit /p/sub/pkg as "pkg": a write to the unit itself is
// sub/pkg/b.go and leaves the Req open, and a write to the lookalike
// /p/pkg/a.go meets it. Once the record keeps a project-relative path
// (debt (t), engine lane), the first write meets and the second does not.
// SYNTHETIC lines.
func TestStep_KnownHoleT_SubdirLookalikeMeets(t *testing.T) {
	f := newFrontier("s1")
	for _, c := range []struct {
		line   string
		steer  bool
		status ReqStatus
		met    int64
	}{
		{`{"type":"steer","id":"h1","at_unix_ms":1,"after_seq":2,"state":"delivered","key":"graph","kind":"","prompt_id":"p1","project":"/p",` +
			`"slots":{"line1":{"kind":"unit","path":"pkg","digest":""}}}`, true, ReqOpen, 0},
		{`{"type":"declaration","seq":3,"file_path":"/p/sub/pkg/b.go","project":"/p","cwd":"/p/sub"}`, false, ReqOpen, 0},
		{`{"type":"declaration","seq":4,"file_path":"/p/pkg/a.go","project":"/p","cwd":"/p/sub"}`, false, ReqMet, 4},
	} {
		decode := RecordEvent
		if c.steer {
			decode = SteerEvent
		}
		ev, ok, err := decode([]byte(c.line))
		if err != nil || !ok {
			t.Fatalf("%s: %v, %v", c.line, ok, err)
		}
		Step(&f, ev)
		if len(f.Reqs) != 1 || f.Reqs[0].Arg != "pkg" || f.Reqs[0].Status != c.status || f.Reqs[0].MetAt != c.met {
			t.Fatalf("after seq %d: Reqs %+v; want one on pkg, %s at %d", ev.Seq, f.Reqs, c.status, c.met)
		}
	}
}
