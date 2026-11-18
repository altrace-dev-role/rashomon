package session

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"maps"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/events"
	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// TestEvent_MergeOrder: a session's records and steer lines merge by (seq,
// record before line, at_unix_ms, id). A line's seq is its after_seq, the
// last record seq its decision saw, so it sorts after that record and before
// the next one, whatever the two clocks say.
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

// TestEvent_Decode: a record line decodes to its seq and type, a steer line
// to its after_seq, clock and id. A steer_evidence line has no after_seq and
// feeds no frontier field: not an event and not an error. A line the merge
// cannot place is an error.
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

// TestEvent_DecodeWrites: a declaration's written paths decode as verify
// reads a write: the project prefix first, else the cwd prefix, cleaned, and
// nothing outside the project; a declaration that names a path outside it is
// marked (WroteOutside), its inside paths kept. Only a declaration names
// writes (an instructions record's file_path is the file it loaded), and a
// Read's declaration names no path.
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

// TestInProject_DecidesAsVerify: inProject is a copy of verify's
// projectPath(relativise(...)) (its doc), so it must decide inside or outside
// as verify does. Each row pins the copy's decision and
// verify.RelevantUnder's, verify's exported reading of the same two rules, on
// .go paths, a class RelevantUnder counts, so its answer is the location's
// alone. Where the two differ the row says why: each is a difference inside
// verify, RelevantUnder against projectPath, and the copy follows
// projectPath, the reading verify's rule check uses. RelevantUnder reads
// absoluteness by filepath.IsAbs, the OS's rule, so its column holds off
// windows (the CI test job runs on linux); on windows only the copy's
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
// ledger writer exists yet, so every ledger line in this file is SYNTHETIC.
// Its rows are real verify.Obligation values marshalled by encoding/json, so
// a decode reads verify's own keys; tail adds keys after the rows.
func ledgerLineSynthetic(t *testing.T, seq int64, rows []verify.Obligation, tail string) []byte {
	t.Helper()
	b, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	return []byte(`{"type":"ledger","seq":` + strconv.FormatInt(seq, 10) + `,"point":"stop","obligations":` + string(b) + tail + `}`)
}

// TestCheckpoint_PointIsVerifys: the fold's Stop point is verify's, named in
// the fold because it imports no verify outside its tests.
func TestCheckpoint_PointIsVerifys(t *testing.T) {
	if stopPoint != verify.PointStop {
		t.Fatalf("stopPoint %q, verify.PointStop %q", stopPoint, verify.PointStop)
	}
}

// TestEvent_DecodeCheckpoint: a verify record at point stop is a Stop's
// checkpoint; a verify record at another point or with none, and any other
// record carrying the key, are not.
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

// TestEvent_DecodeVerdictSynthetic: a ledger line's done_flagged, unmet and
// checkpoint_seq decode as its verdict, a present false or 0 as a value; a
// line without any one of the three, or with one null, has no verdict, never
// false or 0. Only a ledger line carries one. SYNTHETIC lines.
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
// no writer records a session record yet, so every session record in this
// file is SYNTHETIC. It is a real store.Session marshalled by encoding/json,
// so a decode reads the store's own keys.
func sessionLineSynthetic(t *testing.T, seq int64, source string) []byte {
	t.Helper()
	b, err := json.Marshal(store.Session{EventHeader: store.EventHeader{Type: store.TypeSession, Seq: &seq}, Source: source})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestEvent_DecodeBoundarySynthetic: a session record whose SessionStart
// source is compact or resume is a boundary; startup, clear, no source, and
// any other record carrying the key are not. SYNTHETIC session records.
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

// TestEvent_DecodeLedgerSynthetic: a ledger line decodes to its rows' id,
// kind, unit and state by verify.Obligation's keys, with its two cut counts,
// 0 when absent. Scope is opaque: a scope no list names decodes as any other.
// Only a ledger line's rows are the ledger: a verify record or a declaration
// carrying the same keys is not one. SYNTHETIC lines.
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

// TestEvent_DecodeWithheld: a declaration whose write path the record
// withheld is marked, whatever the reason, so the frontier can say Touched is
// cut. A withheld cwd or project alone (a Read below record scope v5) is no
// write, and only a declaration's paths are writes. Below v5 a shell line's
// list is keyed withheld only when it named a path (store.GateDeclaration),
// so one that named none is not marked.
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

// TestEvent_HoldsNoWords: an event keeps the order key and the fields a rule
// reads, never a line's words (model_text, user_text). A string field not
// named here fails by name.
func TestEvent_HoldsNoWords(t *testing.T) {
	named := map[string]string{
		"ID":       "steer line id",
		"Type":     "record type or steer.LineType",
		"State":    "a closed steer.LineState name",
		"Key":      "a finding's dedupe key: a unit id and a digest",
		"Kind":     "a closed steer.Kind name, or empty",
		"PromptID": "a prompt id (a UUID), never the prompt's words",
		"Wrote[]":  "a project-relative path a call named as written, never a file's contents",

		"Stated[].Pred": "a compliance predicate's name, from steer.Compliance's closed set",
		"Stated[].Arg":  "a graph line's subject: the path its line holds (a code line's path, never a symbol), its keyed digest at record scope v4, or a decision's knowledge id",

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

// TestStep_Delivered: a steer line in state delivered becomes one LineRef at
// its after_seq with its id, key and kind. A line in any other state and a
// record leave Delivered as it was.
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

// TestStep_DeliveredOncePerID: one LineRef per steer line id, the uniqueness
// canon's order rests on; a line seen again keeps its first ref.
func TestStep_DeliveredOncePerID(t *testing.T) {
	f := newFrontier("s1")
	Step(&f, Event{Seq: 4, Steer: true, AtUnixMS: 5, ID: "d1", Type: "steer", State: "delivered", Key: "k1"})
	Step(&f, Event{Seq: 9, Steer: true, AtUnixMS: 9, ID: "d1", Type: "steer", State: "delivered", Key: "k1"})
	if want := []LineRef{{ID: "d1", Key: "k1", Seq: 4}}; !reflect.DeepEqual(f.Delivered, want) {
		t.Fatalf("Delivered\n got %+v\nwant %+v", f.Delivered, want)
	}
}

// TestStep_Intents: one IntentRef per prompt id in the steer log, in order of
// first appearance from 1, at its first line's after_seq, whatever that
// line's state; a briefing line (no prompt id) adds none.
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

// TestStep_Touched: every path a declaration wrote joins Touched as its
// graph.FileID, sorted and unique, so the same writes in any order fold to
// the same set; a steer line and a record without writes leave it as it was.
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

// TestStep_TouchedWithheld: each declaration whose write path was withheld
// counts once, so a frontier below record scope v5 says its empty Touched is
// cut, not that the session wrote nothing.
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

// TestStep_BoundarySynthetic: LastBoundary is the seq of the last boundary; a
// session record that is none (startup, clear) and any other record leave it
// as it was. SYNTHETIC events.
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

// TestStep_VerdictSynthetic: each Stop checkpoint is in StopsWithoutVerdict
// until a ledger line's verdict names it; a verdict is copied as its
// checkpoint's VerdictRef, one per checkpoint, the first kept; a ledger line
// with no verdict adds none; and each checkpoint is in Verdicts or
// StopsWithoutVerdict (conservation). SYNTHETIC events.
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

// TestStep_VerdictBindsBySeqSynthetic: a ledger line binds to the checkpoint
// its checkpoint_seq names, not to the record before it, since a record can
// land between a Stop's two appends: here a write declaration and a
// subagent_stop checkpoint. SYNTHETIC events.
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

// TestStep_VerdictWithoutCheckpointSynthetic: a ledger line whose
// checkpoint_seq names no stop checkpoint read before it, a verify record at
// another point or one at a later seq, adds no verdict, and its rows still
// apply. SYNTHETIC events.
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

// TestStep_VerdictLeavesCopies: a verdict cuts its checkpoint from
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

// TestStep_CheckpointWithoutLedgerSynthetic: a Stop whose checkpoint no
// ledger line follows (a kill, a lock timeout) keeps the obligations of the
// Stop before, as of that line's seq, but gets no verdict: a verdict belongs
// to one Stop, and the Stop is in StopsWithoutVerdict, never read as not
// done. SYNTHETIC events.
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

// TestStep_LedgerSynthetic: the last ledger line is the frontier's
// obligations, as of its seq, with its own cut counts, each line replacing
// the one before; one ref per id, the first kept; each state as written, so
// unknown stays unknown and open stays open; and a record that is no ledger
// line leaves them as they were. SYNTHETIC events: no ledger writer exists
// yet.
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

// TestStep_TouchedOutside: each declaration that named a path outside the
// project counts once, beside the inside paths it adds to Touched, and the
// two cut counts move apart.
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

// TestEvent_DecodeStatedSynthetic: a steer line's graph lines, in slots
// line1..line5, decode as Stated in slot order: a unit line opened_unit, a
// code line in role reuse called_symbol and in role consumer
// edited_or_verified, each on its path or its keyed digest when the record
// form left no path, and a decision line paths_untouched on its id. A code
// line with no role, a table, a lesson, an empty line, a line with no subject
// and a pattern's other slot keys state none. SYNTHETIC lines.
func TestEvent_DecodeStatedSynthetic(t *testing.T) {
	head := `{"type":"steer","id":"u1","at_unix_ms":5,"after_seq":7,"state":"delivered","key":"graph","kind":"","prompt_id":"p1"`
	unit := func(arg string) StatedReq { return StatedReq{Pred: steer.PredOpenedUnit, Arg: arg} }
	for _, c := range []struct {
		name  string
		slots string
		want  []StatedReq
	}{
		{"units in slot order", `,"slots":{"line1":{"kind":"unit","path":"internal/a","digest":""},"line2":{"kind":"code","path":"internal/b/x.go"},` +
			`"line3":{"kind":"","path":""},"line5":{"kind":"unit","path":"internal/e"},"gate":"go test"}`, []StatedReq{unit("internal/a"), unit("internal/e")}},
		{"a keyed digest at v4", `,"slots":{"line4":{"kind":"unit","path":"","digest":"` + keyedUnit + `"}}`, []StatedReq{unit(keyedUnit)}},
		{"a unit line with neither", `,"slots":{"line1":{"kind":"unit","path":"","digest":""}}`, nil},
		{"code lines by role", `,"slots":{"line1":{"kind":"code","path":"internal/r.go","role":"reuse"},` +
			`"line2":{"kind":"code","path":"","digest":"` + keyedUnit + `","role":"consumer"},"line3":{"kind":"code","path":"internal/n.go","role":""},` +
			`"line4":{"kind":"table","table":"users","path":""}}`,
			[]StatedReq{{Pred: steer.PredCalledSymbol, Arg: "internal/r.go"}, {Pred: steer.PredEditedOrVerified, Arg: keyedUnit}}},
		{"a decision line by its id", `,"slots":{"line1":{"kind":"lesson","id":"les:INC-1","path":"docs/l.md"},` +
			`"line2":{"kind":"decision","id":"dec:D145","path":"docs/adr/145.md"},"line3":{"kind":"decision","id":"","path":"docs/adr/146.md"}}`,
			[]StatedReq{{Pred: steer.PredPathsUntouched, Arg: "dec:D145"}}},
		{"no slots", ``, nil},
		{"null slots", `,"slots":null`, nil},
	} {
		ev, ok, err := SteerEvent([]byte(head + c.slots + "}"))
		if err != nil || !ok || !slices.Equal(ev.Stated, c.want) {
			t.Errorf("%s: Stated %+v, %v, %v; want %+v", c.name, ev.Stated, ok, err, c.want)
		}
	}
}

// TestStep_ReqsSynthetic: a delivered line states one opened_unit Req per
// unit, keyed Pred:Arg, at its after_seq; a suppressed line states none, and
// a unit stated again keeps its first StatedAt, its LastStatedAt the later
// line's. The lines name no prompt, so none supersedes. A Req is marked
// met_once at the first write after its line at its path or under it, and
// never again. The boundary in three cases, the events put in the merged
// order by Before: a write before the line (5) and a write at the line's own
// after_seq (10, a record sorts first) leave internal/early open; writes
// after it (14, 15) meet. A sibling that only shares the prefix (13) meets
// nothing. A Req of another predicate on the same path stays open: the fold
// marks only what it can see. SYNTHETIC events.
func TestStep_ReqsSynthetic(t *testing.T) {
	f := newFrontier("s1")
	other := ReqState{ID: steer.PredEditedOrVerified + ":internal/platform", Pred: steer.PredEditedOrVerified, Arg: "internal/platform", Status: ReqOpen, StatedAt: 3}
	f.Reqs = []ReqState{other}
	line := func(seq int64, id, state string, units ...string) Event {
		ev := Event{Seq: seq, Steer: true, AtUnixMS: seq, ID: id, Type: steer.LineType, State: state, Key: "graph"}
		for _, u := range units {
			ev.Stated = append(ev.Stated, StatedReq{Pred: steer.PredOpenedUnit, Arg: u})
		}
		return ev
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
	unit := func(arg string, status ReqStatus, last, met int64) ReqState {
		return ReqState{ID: steer.PredOpenedUnit + ":" + arg, Pred: steer.PredOpenedUnit, Arg: arg, Status: status, StatedAt: 10, LastStatedAt: last, MetAt: met}
	}
	want := []ReqState{other, unit("internal/platform", ReqMetOnce, 12, 15), unit("cmd/x", ReqMetOnce, 10, 14), unit("internal/early", ReqOpen, 10, 0)}
	if !slices.Equal(f.Reqs, want) {
		t.Errorf("Reqs:\n%+v\nwant\n%+v", f.Reqs, want)
	}
}

// TestStep_LeavesCopies: a Step leaves a Frontier copied before it as it was,
// byte for byte, while the stepped one gains the write's file in order and
// meets the Req. Touched has room past its length, so an insert that shifts
// in place would write into the copy's array. Likewise a later prompt's line
// supersedes a Req of a list with room past its length, and the copy keeps it
// open. SYNTHETIC events.
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
	if !slices.Equal(f.Touched, wantTouched) || f.Reqs[0].Status != ReqMetOnce || f.Reqs[0].MetAt != 2 {
		t.Errorf("stepped: Touched %q, Req %+v; want %q and met at 2", f.Touched, f.Reqs[0], wantTouched)
	}
	h := stepAll(unitLine(2, "l1", "p1", "d"))
	h.Reqs = append(make([]ReqState, 0, 8), h.Reqs...)
	k := h
	beforeK, err := MarshalFrontier(k)
	if err != nil {
		t.Fatal(err)
	}
	Step(&h, unitLine(5, "l2", "p2", "e"))
	if after, _ := MarshalFrontier(k); string(after) != string(beforeK) {
		t.Errorf("copy changed by a supersede:\n%s\nwant\n%s", after, beforeK)
	}
	if d := unitReq(h, "d"); d.Status != ReqSuperseded {
		t.Errorf("stepped: d %+v; want superseded", d)
	}
}

// TestStep_KnownHole_SubdirLookalikeMeets: a KNOWN HOLE, pinned so that its
// fix must flip this test. A slot path is relative to the hook's working
// directory, a written path to the project, and the steer line names no cwd.
// A line written from /p/sub names the unit /p/sub/pkg as "pkg": a write to
// the unit itself is sub/pkg/b.go and leaves the Req open, and a write to the
// lookalike /p/pkg/a.go meets it. Once the record keeps a project-relative
// path, the first write meets and the second does not. SYNTHETIC lines.
func TestStep_KnownHole_SubdirLookalikeMeets(t *testing.T) {
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
		{`{"type":"declaration","seq":4,"file_path":"/p/pkg/a.go","project":"/p","cwd":"/p/sub"}`, false, ReqMetOnce, 4},
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

// TestStep_LineStatesEachPredSynthetic: one delivered line with a unit, a
// reuse, a consumer and a decision graph line states four Reqs at its
// after_seq, keyed Pred:Arg, in slot order. Writes after it to every subject
// mark only opened_unit, met_once at the first; called_symbol,
// edited_or_verified and paths_untouched stay open, since the fold resolves
// none of them. SYNTHETIC lines.
func TestStep_LineStatesEachPredSynthetic(t *testing.T) {
	f := newFrontier("s1")
	for _, c := range []struct {
		line  string
		steer bool
	}{
		{`{"type":"steer","id":"q1","at_unix_ms":1,"after_seq":3,"state":"delivered","key":"graph","kind":"","prompt_id":"p1","slots":{` +
			`"line1":{"kind":"unit","path":"internal/u"},"line2":{"kind":"code","path":"internal/r.go","role":"reuse"},` +
			`"line3":{"kind":"code","path":"internal/c.go","role":"consumer"},"line4":{"kind":"decision","id":"dec:D1","path":"internal/d"}}}`, true},
		{`{"type":"declaration","seq":4,"file_path":"/p/internal/r.go","project":"/p","cwd":"/p"}`, false},
		{`{"type":"declaration","seq":5,"file_path":"/p/internal/c.go","project":"/p","cwd":"/p"}`, false},
		{`{"type":"declaration","seq":6,"file_path":"/p/internal/d/x.go","project":"/p","cwd":"/p"}`, false},
		{`{"type":"declaration","seq":7,"file_path":"/p/internal/u/a.go","project":"/p","cwd":"/p"}`, false},
		{`{"type":"declaration","seq":8,"file_path":"/p/internal/u/b.go","project":"/p","cwd":"/p"}`, false},
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
	}
	req := func(pred, arg string, status ReqStatus, met int64) ReqState {
		return ReqState{ID: pred + ":" + arg, Pred: pred, Arg: arg, Status: status, StatedAt: 3, LastStatedAt: 3, MetAt: met}
	}
	want := []ReqState{
		req(steer.PredOpenedUnit, "internal/u", ReqMetOnce, 7),
		req(steer.PredCalledSymbol, "internal/r.go", ReqOpen, 0),
		req(steer.PredEditedOrVerified, "internal/c.go", ReqOpen, 0),
		req(steer.PredPathsUntouched, "dec:D1", ReqOpen, 0),
	}
	if !slices.Equal(f.Reqs, want) {
		t.Errorf("Reqs:\n%+v\nwant\n%+v", f.Reqs, want)
	}
}

// unitLine is a delivered steer line of prompt p at after_seq seq, its graph
// lines stating one opened_unit Req per unit, in slot order; p empty is a
// line with no prompt (a briefing). SYNTHETIC.
func unitLine(seq int64, id, p string, units ...string) Event {
	ev := Event{Seq: seq, Steer: true, AtUnixMS: seq, ID: id, Type: steer.LineType, State: string(steer.LineStateDelivered), Key: "graph", PromptID: p}
	for _, u := range units {
		ev.Stated = append(ev.Stated, StatedReq{Pred: steer.PredOpenedUnit, Arg: u})
	}
	return ev
}

// stepAll steps a new frontier of session s1 through evs, in their order.
func stepAll(evs ...Event) Frontier {
	f := newFrontier("s1")
	for _, ev := range evs {
		Step(&f, ev)
	}
	return f
}

// unitReq is f's opened_unit Req on u, or the zero ReqState.
func unitReq(f Frontier, u string) ReqState {
	for _, r := range f.Reqs {
		if r.ID == steer.PredOpenedUnit+":"+u {
			return r
		}
	}
	return ReqState{}
}

// promptID is the IntentRef.ID of session s1's n-th prompt.
func promptID(n int) string { return string(graph.PromptNodeID("s1", n)) }

// supersededByWhileSuperseded is the invariant on every Req of f:
// SupersededBy is non-empty exactly when Status is superseded. It names the
// first Req that breaks it, or returns "".
func supersededByWhileSuperseded(f Frontier) string {
	for _, r := range f.Reqs {
		if (r.SupersededBy != "") != (r.Status == ReqSuperseded) {
			return r.ID + " " + string(r.Status) + " superseded_by " + strconv.Quote(r.SupersededBy)
		}
	}
	return ""
}

// TestStep_SupersedePerLineSynthetic: supersede takes a line whole, never
// slot by slot. Prompt p1 states b and d; p2's one line states a and b. b,
// named in the line beside a, stays open in either slot order, and d, which
// the line does not name, is superseded, so the line does supersede. The two
// slot orders fold to the same bytes. SYNTHETIC lines.
func TestStep_SupersedePerLineSynthetic(t *testing.T) {
	var got []string
	for _, units := range [][]string{{"a", "b"}, {"b", "a"}} {
		f := stepAll(unitLine(2, "l1", "p1", "b", "d"), unitLine(5, "l2", "p2", units...))
		if a, b, d := unitReq(f, "a"), unitReq(f, "b"), unitReq(f, "d"); a.Status != ReqOpen || b.Status != ReqOpen || b.SupersededBy != "" || d.Status != ReqSuperseded {
			t.Errorf("slots %q: a %+v, b %+v, d %+v; want a and b open, d superseded", units, a, b, d)
		}
		bs, err := MarshalFrontier(f)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, string(bs))
	}
	if got[0] != got[1] {
		t.Errorf("slot order changed the frontier:\n%s\n%s", got[0], got[1])
	}
}

// TestStep_SupersedeSparesPromptsOwnSynthetic: a Req a prompt names is that
// prompt's own. p1 states b and d; p2's first line states b again and
// supersedes d, which no line of p2 names; p2's second line states c and
// supersedes neither b, which p2's first line named at p2's StatedAt, nor c.
// b keeps its first StatedAt, its LastStatedAt p2's line. With
// TestStep_RestatementReopensSynthetic's case, a prompt's {b} then {c} and
// {c} then {b} both end with b open. SYNTHETIC lines.
func TestStep_SupersedeSparesPromptsOwnSynthetic(t *testing.T) {
	f := stepAll(unitLine(2, "l1", "p1", "b", "d"), unitLine(5, "l2", "p2", "b"), unitLine(6, "l3", "p2", "c"))
	b, c, d := unitReq(f, "b"), unitReq(f, "c"), unitReq(f, "d")
	if b.Status != ReqOpen || b.StatedAt != 2 || b.LastStatedAt != 5 || c.Status != ReqOpen || d.Status != ReqSuperseded {
		t.Errorf("b %+v, c %+v, d %+v; want b open, stated at 2, last at 5; c open; d superseded", b, c, d)
	}
}

// TestStep_RestatementReopensSynthetic: a superseded Req goes back to open
// only when a delivered line states the same Pred and Arg again. p1 states b
// and d; p2's first line states c and supersedes both. A suppressed line of
// p2 naming b and a delivered line of p2 naming c again leave b superseded;
// p2's line naming b reopens it: open, SupersededBy empty, StatedAt still
// p1's, LastStatedAt the reopening line's. d stays superseded, and c, p2's
// own, stays open. SYNTHETIC lines.
func TestStep_RestatementReopensSynthetic(t *testing.T) {
	suppressed := unitLine(6, "l3", "p2", "b")
	suppressed.State = string(steer.LineStateSuppressed)
	f := stepAll(unitLine(2, "l1", "p1", "b", "d"), unitLine(5, "l2", "p2", "c"), suppressed, unitLine(7, "l4", "p2", "c"))
	if b := unitReq(f, "b"); b.Status != ReqSuperseded || b.SupersededBy != promptID(2) {
		t.Fatalf("before the restatement: b %+v; want superseded by %s", b, promptID(2))
	}
	Step(&f, unitLine(8, "l5", "p2", "b"))
	b, c, d := unitReq(f, "b"), unitReq(f, "c"), unitReq(f, "d")
	if b.Status != ReqOpen || b.SupersededBy != "" || b.StatedAt != 2 || b.LastStatedAt != 8 || c.Status != ReqOpen || d.Status != ReqSuperseded {
		t.Errorf("b %+v, c %+v, d %+v; want b open, SupersededBy empty, stated at 2, last at 8; c open; d superseded", b, c, d)
	}
}

// TestStep_SupersededMeetsOnceSynthetic: evidence marks a superseded Req as
// it marks an open one and empties its SupersededBy: met_once is final and
// has one form. p1 states b; p2 states c and supersedes b; a write under b
// marks it met_once at the write. p2 stating b again and a second write leave
// it met_once at the first write. SYNTHETIC events.
func TestStep_SupersededMeetsOnceSynthetic(t *testing.T) {
	f := stepAll(unitLine(2, "l1", "p1", "b"), unitLine(5, "l2", "p2", "c"),
		Event{Seq: 6, Type: store.TypeDeclaration, Wrote: []string{"b/x.go"}},
		unitLine(7, "l3", "p2", "b"),
		Event{Seq: 8, Type: store.TypeDeclaration, Wrote: []string{"b/y.go"}})
	want := ReqState{ID: steer.PredOpenedUnit + ":b", Pred: steer.PredOpenedUnit, Arg: "b", Status: ReqMetOnce, StatedAt: 2, LastStatedAt: 7, MetAt: 6}
	if b := unitReq(f, "b"); b != want {
		t.Errorf("b %+v; want %+v", b, want)
	}
}

// TestStep_SupersededByIsPromptSynthetic: SupersededBy is the later prompt's
// IntentRef.ID, its prompt node id, never a sibling Req's key or the prompt's
// digest. p1 states b; p2 states c and supersedes b. A line with no prompt
// then states e and supersedes nothing: c stays open. SYNTHETIC lines.
func TestStep_SupersededByIsPromptSynthetic(t *testing.T) {
	f := stepAll(unitLine(2, "l1", "p1", "b"), unitLine(5, "l2", "p2", "c"), unitLine(9, "l3", "", "e"))
	b, c, e := unitReq(f, "b"), unitReq(f, "c"), unitReq(f, "e")
	if b.Status != ReqSuperseded || b.SupersededBy != promptID(2) || c.Status != ReqOpen || e.Status != ReqOpen {
		t.Errorf("b %+v, c %+v, e %+v; want b superseded by %s, c and e open", b, c, e, promptID(2))
	}
}

// TestStep_SupersedeLineBroughtOnlySynthetic: only a predicate a graph line
// states supersedes or is superseded, and only a Req of a predicate the line
// states. p1's line states opened_unit b, called_symbol r.go and gate_green
// go-test; p2's line states opened_unit c and gate_green go-build. b is
// superseded; r.go, of a predicate p2's line does not state, stays open, and
// so do both gate_green Reqs: a gate Req neither supersedes nor is
// superseded. SYNTHETIC events: no SteerEvent states gate_green, which a
// later change copies from a stored line.
func TestStep_SupersedeLineBroughtOnlySynthetic(t *testing.T) {
	const gate = "gate_green"
	l1 := unitLine(2, "l1", "p1", "b")
	l1.Stated = append(l1.Stated, StatedReq{Pred: steer.PredCalledSymbol, Arg: "r.go"}, StatedReq{Pred: gate, Arg: "go-test"})
	l2 := unitLine(5, "l2", "p2", "c")
	l2.Stated = append(l2.Stated, StatedReq{Pred: gate, Arg: "go-build"})
	f := stepAll(l1, l2)
	got := map[string]ReqStatus{}
	for _, r := range f.Reqs {
		got[r.ID] = r.Status
	}
	want := map[string]ReqStatus{
		steer.PredOpenedUnit + ":b":      ReqSuperseded,
		steer.PredOpenedUnit + ":c":      ReqOpen,
		steer.PredCalledSymbol + ":r.go": ReqOpen,
		gate + ":go-test":                ReqOpen,
		gate + ":go-build":               ReqOpen,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("statuses %v; want %v", got, want)
	}
}

// TestReq_MetOnceIsFinalSupersededReopensByLine: one Req through every
// transition. b is open at p1's line, superseded at p2's, open again when p2
// states it, superseded at p3's and met_once at a write, which empties its
// SupersededBy; then p3 stating it again, p4's line moving past it and a
// second write leave it met_once at the first write: met_once is final and
// never superseded. After every step SupersededBy is set exactly while Status
// is superseded. StatedAt stays p1's line throughout. SYNTHETIC events.
func TestReq_MetOnceIsFinalSupersededReopensByLine(t *testing.T) {
	f := newFrontier("s1")
	for _, c := range []struct {
		ev     Event
		status ReqStatus
		by     string
		met    int64
	}{
		{unitLine(2, "l1", "p1", "b"), ReqOpen, "", 0},
		{unitLine(4, "l2", "p2", "c"), ReqSuperseded, promptID(2), 0},
		{unitLine(5, "l3", "p2", "b"), ReqOpen, "", 0},
		{unitLine(7, "l4", "p3", "d"), ReqSuperseded, promptID(3), 0},
		{Event{Seq: 8, Type: store.TypeDeclaration, Wrote: []string{"b/x.go"}}, ReqMetOnce, "", 8},
		{unitLine(9, "l5", "p3", "b"), ReqMetOnce, "", 8},
		{unitLine(11, "l6", "p4", "e"), ReqMetOnce, "", 8},
		{Event{Seq: 12, Type: store.TypeDeclaration, Wrote: []string{"b/y.go"}}, ReqMetOnce, "", 8},
	} {
		Step(&f, c.ev)
		if bad := supersededByWhileSuperseded(f); bad != "" {
			t.Fatalf("after seq %d: %s", c.ev.Seq, bad)
		}
		if b := unitReq(f, "b"); b.Status != c.status || b.SupersededBy != c.by || b.MetAt != c.met || b.StatedAt != 2 {
			t.Fatalf("after seq %d: b %+v; want %s, superseded by %q, met at %d, stated at 2", c.ev.Seq, b, c.status, c.by, c.met)
		}
	}
}

// foldEventsSynthetic is one session's events in the merged order, at least
// one for every rule Step applies: a briefing line, three prompts' unit lines
// (a supersede, a restatement that reopens, met_once by writes), a suppressed
// line, writes inside, withheld and outside the project, a startup and a
// compact session record, three Stop checkpoints and three ledger lines, two
// with a verdict and one with a repeated row id. SYNTHETIC events.
func foldEventsSynthetic() []Event {
	suppressed := unitLine(6, "l3", "p2", "internal/x")
	suppressed.State, suppressed.AtUnixMS = string(steer.LineStateSuppressed), 7
	return []Event{
		unitLine(0, "b0", "", "internal/brief"),
		{Seq: 1, Type: store.TypeSession},
		unitLine(1, "l1", "p1", "internal/a", "internal/b"),
		{Seq: 2, Type: store.TypeDeclaration, Wrote: []string{"internal/a/x.go"}},
		{Seq: 3, Type: store.TypeDeclaration, WroteWithheld: true},
		{Seq: 4, Type: store.TypeDeclaration, Wrote: []string{"c.go"}, WroteOutside: true},
		{Seq: 5, Type: store.TypeVerify, Checkpoint: true},
		{Seq: 6, Type: ledgerType, Ledger: true, Obligations: []ObligationRef{
			{ID: "ob2", Kind: "expected_evidence", State: "unknown"},
			{ID: "ob1", Kind: "gate_after_write", Unit: "unit:a", State: "met"},
			{ID: "ob1", Kind: "gate_after_write", Unit: "unit:a", State: "open"},
		}, ObligationsOmitted: 1, Verdict: true, DoneFlagged: true, CheckpointSeq: 5},
		unitLine(6, "l2", "p2", "internal/c"),
		suppressed,
		{Seq: 7, Type: store.TypeSession, Boundary: true},
		unitLine(7, "l4", "p2", "internal/b"),
		{Seq: 8, Type: store.TypeDeclaration, Wrote: []string{"internal/c/y.go", "internal/b/z.go"}},
		{Seq: 9, Type: store.TypeVerify, Checkpoint: true},
		{Seq: 10, Type: ledgerType, Ledger: true, Obligations: []ObligationRef{{ID: "ob3", Kind: "gate_after_write", Unit: "unit:c", State: "open"}}, UnitRowsCut: 2},
		unitLine(10, "l5", "p3", "internal/d"),
		{Seq: 11, Type: store.TypeVerify, Checkpoint: true},
		{Seq: 12, Type: ledgerType, Ledger: true, Obligations: []ObligationRef{
			{ID: "ob4", Kind: "expected_evidence", State: "unmet"},
			{ID: "ob3", Kind: "gate_after_write", Unit: "unit:c", State: "unmet"},
		}, ObligationsOmitted: 3, UnitRowsCut: 1, Verdict: true, Unmet: 2, CheckpointSeq: 11},
	}
}

// foldByStep is what Fold means, spelled out without it: a new frontier of
// session s1 stepped through evs as given, which the caller has put in the
// merged order, Through the last one's place.
func foldByStep(evs []Event) Frontier {
	f := stepAll(evs...)
	if len(evs) > 0 {
		last := evs[len(evs)-1]
		f.Through = Mark{Seq: last.Seq, Steer: last.Steer, AtUnixMS: last.AtUnixMS, ID: last.ID}
	}
	return f
}

// mustMarshal is MarshalFrontier(f) as a string, failing t on an error.
func mustMarshal(t *testing.T, f Frontier) string {
	t.Helper()
	b, err := MarshalFrontier(f)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// inMergedOrder fails t unless evs are in the merged order.
func inMergedOrder(t *testing.T, evs []Event) {
	t.Helper()
	if !sort.SliceIsSorted(evs, func(i, j int) bool { return evs[i].Before(evs[j]) }) {
		t.Fatal("the synthetic events are not in the merged order")
	}
}

// TestFold_IncrementalEqualsFullSynthetic: for every cut j of one session's
// events and every cut k at or after it, the fold to j, saved and read back,
// then caught up by the events from j to k in a shuffled order, is byte-equal
// to the fold to k; and the fold to k is its events stepped in the merged
// order. Every fold to k keeps SupersededBy set exactly while Status is
// superseded. SYNTHETIC events.
func TestFold_IncrementalEqualsFullSynthetic(t *testing.T) {
	evs := foldEventsSynthetic()
	inMergedOrder(t, evs)
	full := make([]string, len(evs)+1)
	for k := range full {
		f := Fold("s1", evs[:k])
		if bad := supersededByWhileSuperseded(f); bad != "" {
			t.Fatalf("fold to %d: %s", k, bad)
		}
		full[k] = mustMarshal(t, f)
		if want := mustMarshal(t, foldByStep(evs[:k])); full[k] != want {
			t.Fatalf("fold to %d is not its events stepped in order:\n got %s\nwant %s", k, full[k], want)
		}
	}
	r := rand.New(rand.NewSource(1))
	for j := range full {
		saved, err := ParseFrontier([]byte(full[j]))
		if err != nil {
			t.Fatalf("read back the fold to %d: %v", j, err)
		}
		for k := j; k < len(full); k++ {
			tail := slices.Clone(evs[j:k])
			r.Shuffle(len(tail), func(a, b int) { tail[a], tail[b] = tail[b], tail[a] })
			g, ok := Catch(saved, tail)
			if !ok {
				t.Fatalf("catch from %d to %d missed", j, k)
			}
			if got := mustMarshal(t, g); got != full[k] {
				t.Fatalf("catch from %d to %d:\n got %s\nwant %s", j, k, got, full[k])
			}
		}
	}
}

// TestFold_IsDeterministicSynthetic: one session's events in 20 shuffled
// orders fold to the bytes of the events stepped in the merged order, and
// Fold leaves the order it was given. SYNTHETIC events.
func TestFold_IsDeterministicSynthetic(t *testing.T) {
	evs := foldEventsSynthetic()
	inMergedOrder(t, evs)
	want := mustMarshal(t, foldByStep(evs))
	in := slices.Clone(evs)
	r := rand.New(rand.NewSource(1))
	for round := 0; round < 20; round++ {
		r.Shuffle(len(in), func(i, j int) { in[i], in[j] = in[j], in[i] })
		was := slices.Clone(in)
		if got := mustMarshal(t, Fold("s1", in)); got != want {
			t.Fatalf("round %d:\n got %s\nwant %s", round, got, want)
		}
		if !reflect.DeepEqual(in, was) {
			t.Fatalf("round %d: Fold reordered the events it was given", round)
		}
	}
}

// TestFold_ThroughIsLastAppliedSynthetic: Through is the full place of the
// last event applied in the merged order, not of the last one given, and an
// empty fold's is the zero Mark. A line at a record's seq sorts after it, so
// a fold of the two ends at the line. Catch with no tail keeps Through; with
// one, it ends at the tail's last. SYNTHETIC events.
func TestFold_ThroughIsLastAppliedSynthetic(t *testing.T) {
	r3 := Event{Seq: 3, Type: store.TypeDeclaration}
	line := Event{Seq: 3, Steer: true, AtUnixMS: 40, ID: "t1", Type: steer.LineType, State: string(steer.LineStateSuppressed), Key: "k"}
	r7 := Event{Seq: 7, Type: store.TypeTerminal}
	atLine := Mark{Seq: 3, Steer: true, AtUnixMS: 40, ID: "t1"}
	for _, c := range []struct {
		name string
		evs  []Event
		want Mark
	}{
		{"empty", nil, Mark{}},
		{"a line given before its record", []Event{line, r3}, atLine},
		{"a record given first", []Event{r7, line, r3}, Mark{Seq: 7}},
	} {
		if got := Fold("s1", c.evs).Through; got != c.want {
			t.Errorf("%s: Through %+v, want %+v", c.name, got, c.want)
		}
	}
	f := Fold("s1", []Event{r3, line})
	if g, ok := Catch(f, nil); !ok || g.Through != atLine {
		t.Errorf("catch with no tail: Through %+v, %v; want %+v", g.Through, ok, atLine)
	}
	if g, ok := Catch(f, []Event{r7}); !ok || g.Through != (Mark{Seq: 7}) {
		t.Errorf("catch to seq 7: Through %+v, %v; want seq 7's record", g.Through, ok)
	}
}

// TestFold_LateLineRefoldsSynthetic: a frontier is continued only by events
// after its Through. A steer line written late, its after_seq below Through,
// misses even when it comes with a later record, and so does an event read
// again; a line at Through's own seq, after its record, continues and equals
// the fold over the whole log. SYNTHETIC events.
func TestFold_LateLineRefoldsSynthetic(t *testing.T) {
	head := []Event{{Seq: 3, Type: store.TypeDeclaration}, {Seq: 7, Type: store.TypeDeclaration}}
	f := Fold("s1", head)
	r9 := Event{Seq: 9, Type: store.TypeDeclaration, Wrote: []string{"a/x.go"}}
	if _, ok := Catch(f, []Event{r9, unitLine(5, "late", "p1", "a")}); ok {
		t.Error("a line at after_seq 5 continued a frontier through seq 7")
	}
	if _, ok := Catch(f, []Event{{Seq: 7, Type: store.TypeDeclaration}}); ok {
		t.Error("the record at Through, read again, continued the frontier")
	}
	at7 := unitLine(7, "on7", "p1", "a")
	g, ok := Catch(f, []Event{r9, at7})
	if !ok {
		t.Fatal("a line at Through's seq, after its record, missed")
	}
	if got, want := mustMarshal(t, g), mustMarshal(t, foldByStep(append(slices.Clone(head), at7, r9))); got != want {
		t.Errorf("caught up:\n got %s\nwant %s", got, want)
	}
}

// TestCatch_FromOneFrontierTwiceSynthetic: two Catches from one frontier
// share no list. The saved frontier has room past every length, as one read
// back does (canon's copies), and each tail appends an intent, a delivered
// line, a checkpoint and a verdict, so a Step's in-place append would write
// the other Catch's array. The first Catch's frontier is the same after the
// second, and each is the fold over its whole log. SYNTHETIC events.
func TestCatch_FromOneFrontierTwiceSynthetic(t *testing.T) {
	head := []Event{
		unitLine(1, "l1", "p1", "a"),
		{Seq: 2, Type: store.TypeVerify, Checkpoint: true},
		{Seq: 3, Type: ledgerType, Ledger: true, Verdict: true, CheckpointSeq: 2},
	}
	tail := func(id, p, unit string, done bool, unmet int) []Event {
		return []Event{
			unitLine(4, id, p, unit),
			{Seq: 5, Type: store.TypeVerify, Checkpoint: true},
			{Seq: 6, Type: ledgerType, Ledger: true, Verdict: true, DoneFlagged: done, Unmet: unmet, CheckpointSeq: 5},
		}
	}
	tailA, tailB := tail("la", "pa", "x", false, 1), tail("lb", "pb", "y", true, 0)
	saved := Fold("s1", head)
	saved.Intents = slices.Grow(saved.Intents, 8)
	saved.Reqs = slices.Grow(saved.Reqs, 8)
	saved.Delivered = slices.Grow(saved.Delivered, 8)
	saved.Verdicts = slices.Grow(saved.Verdicts, 8)
	saved.StopsWithoutVerdict = slices.Grow(saved.StopsWithoutVerdict, 8)
	a, okA := Catch(saved, tailA)
	first := mustMarshal(t, a)
	b, okB := Catch(saved, tailB)
	if !okA || !okB {
		t.Fatalf("catches missed: %v, %v", okA, okB)
	}
	if again := mustMarshal(t, a); again != first {
		t.Errorf("the second Catch changed the first's frontier:\n got %s\nwant %s", again, first)
	}
	if want := mustMarshal(t, foldByStep(append(slices.Clone(head), tailA...))); first != want {
		t.Errorf("first catch:\n got %s\nwant %s", first, want)
	}
	if got, want := mustMarshal(t, b), mustMarshal(t, foldByStep(append(slices.Clone(head), tailB...))); got != want {
		t.Errorf("second catch:\n got %s\nwant %s", got, want)
	}
}

// TestCatch_ClipsEveryList: every list of a caught-up frontier ends at its
// length, so the next Step's append allocates, and the frontier Catch was
// given keeps its room. The walk is over Frontier's fields by reflection, so
// a list added later is held to it too.
func TestCatch_ClipsEveryList(t *testing.T) {
	f := newFrontier("s1")
	v := reflect.ValueOf(&f).Elem()
	for i := 0; i < v.NumField(); i++ {
		if fv := v.Field(i); fv.Kind() == reflect.Slice {
			fv.Set(reflect.MakeSlice(fv.Type(), 1, 4))
		}
	}
	g, ok := Catch(f, nil)
	if !ok {
		t.Fatal("an empty tail missed")
	}
	gv := reflect.ValueOf(g)
	for i := 0; i < gv.NumField(); i++ {
		name := gv.Type().Field(i).Name
		if fv := gv.Field(i); fv.Kind() == reflect.Slice && fv.Cap() != fv.Len() {
			t.Errorf("%s: cap %d, len %d; want no room past its length", name, fv.Cap(), fv.Len())
		}
		if fv := v.Field(i); fv.Kind() == reflect.Slice && fv.Cap() != 4 {
			t.Errorf("%s of the frontier given: cap %d, want 4 as it was", name, fv.Cap())
		}
	}
}

// TestCatch_MissesOnAnotherKey: Catch continues only a frontier of this
// foldVersion and ledgerVersion. One of version 0, a Frontier built by hand,
// or of the next version misses, so its state is folded again under the
// current rules instead of continued.
func TestCatch_MissesOnAnotherKey(t *testing.T) {
	ev := []Event{{Seq: 1, Type: store.TypeDeclaration, Wrote: []string{"a.go"}}}
	if _, ok := Catch(newFrontier("s1"), ev); !ok {
		t.Fatal("a frontier of this key missed")
	}
	for _, c := range []struct {
		name         string
		fold, ledger int
	}{
		{"fold_version 0", 0, ledgerVersion},
		{"the next fold_version", foldVersion + 1, ledgerVersion},
		{"ledger_version 0", foldVersion, 0},
		{"the next ledger_version", foldVersion, ledgerVersion + 1},
	} {
		f := newFrontier("s1")
		f.FoldVersion, f.LedgerVersion = c.fold, c.ledger
		if _, ok := Catch(f, ev); ok {
			t.Errorf("%s: continued", c.name)
		}
	}
	if _, ok := Catch(Frontier{}, ev); ok {
		t.Error("a zero Frontier continued")
	}
}

// updateGolden writes the fold golden of this foldVersion, once: it refuses
// a set that exists, so a golden is never rewritten under its own version.
var updateGolden = flag.Bool("update", false, "write testdata/sessions/golden/fold-v<foldVersion>; refuses an existing set")

// goldenDir is the golden set of this foldVersion.
func goldenDir() string {
	return filepath.Join("testdata", "sessions", "golden", "fold-v"+strconv.Itoa(foldVersion))
}

// TestFold_GoldenTiesFoldVersion: the frontier the synthetic events fold to,
// and each recorded session's of testdata/sessions, is its golden of this
// foldVersion, byte for byte, so a change to what Step writes fails here
// until foldVersion is bumped and the next set written; a frontier saved
// under the old rules then misses. The golden root holds this version's set
// alone, and the set holds no golden without a source. -update writes the set
// once, into a directory that does not exist. The limit is foldVersion's: a
// Step change that neither the synthetic events nor the recorded sessions
// exercise passes unbumped, and the recorded ones hold no ledger line or
// session record (testdata/sessions/PROVENANCE.txt).
func TestFold_GoldenTiesFoldVersion(t *testing.T) {
	evs := foldEventsSynthetic()
	inMergedOrder(t, evs)
	sources := map[string]string{"synthetic.json": mustMarshal(t, Fold("s1", evs)) + "\n"}
	for _, run := range loadFixtures(t) {
		sources[run.Name+".json"] = mustMarshal(t, Fold(run.Session, run.Events)) + "\n"
	}
	dir := goldenDir()
	if *updateGolden {
		if _, err := os.Stat(dir); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("-update: %s exists or cannot be read (%v); a set is written once, under a new foldVersion", dir, err)
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for name, b := range sources {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(b), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		t.Logf("wrote %s: read it by hand before it is committed", dir)
		return
	}
	sets, err := os.ReadDir(filepath.Dir(dir))
	if err != nil {
		t.Fatalf("golden root: %v; write the set with -update", err)
	}
	for _, s := range sets {
		if s.Name() != filepath.Base(dir) {
			t.Errorf("%s: a golden set beside foldVersion %d's; one set lives", s.Name(), foldVersion)
		}
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("the golden set of foldVersion %d: %v", foldVersion, err)
	}
	for _, f := range files {
		if _, ok := sources[f.Name()]; !ok {
			t.Errorf("%s: a golden with no source", f.Name())
		}
	}
	for name, got := range sources {
		want, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if !bytes.Equal([]byte(got), want) {
			t.Errorf("%s changed while foldVersion stayed %d: bump foldVersion and write fold-v%d with -update; %s", name, foldVersion, foldVersion+1, firstDiff(got, string(want)))
		}
	}
}

// fixtureRun is one recorded session of testdata/sessions: the run's name,
// its session id and its events as its two files hold them, records first.
type fixtureRun struct {
	Name, Session string
	Events        []Event
}

// readFixtures reads the session fixtures under root: every file
// SHA256SUMS.txt lists, each held to its digest, and no file under stores/
// that it does not list, so a byte edited by hand or a file added fails until
// the set is regenerated (PROVENANCE.txt). Every records.ndjson line decodes
// to an event; a steer line may decode to none (steer_evidence). Runs come in
// name order.
func readFixtures(root string) ([]fixtureRun, error) {
	sums, err := os.ReadFile(filepath.Join(root, "SHA256SUMS.txt"))
	if err != nil {
		return nil, err
	}
	listed := map[string]string{}
	for _, ln := range strings.Split(strings.TrimSuffix(string(sums), "\n"), "\n") {
		sum, rel, ok := strings.Cut(ln, "  ")
		if !ok || len(sum) != 2*sha256.Size {
			return nil, fmt.Errorf("SHA256SUMS.txt: %q is not \"<sha256>  <path>\"", ln)
		}
		listed[rel] = sum
	}
	err = filepath.WalkDir(filepath.Join(root, "stores"), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err == nil && listed[filepath.ToSlash(rel)] == "" {
			err = fmt.Errorf("%s: a file SHA256SUMS.txt does not list", filepath.ToSlash(rel))
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	byRun := map[string]*fixtureRun{}
	for _, rel := range slices.Sorted(maps.Keys(listed)) {
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return nil, err
		}
		if sum := sha256.Sum256(b); hex.EncodeToString(sum[:]) != listed[rel] {
			return nil, fmt.Errorf("%s: its sha256 is not SHA256SUMS.txt's; regenerate the set, never edit it by hand (PROVENANCE.txt)", rel)
		}
		part := strings.Split(rel, "/")
		if len(part) != 5 || part[0] != "stores" || part[2] != "runs" || part[4] != "records.ndjson" && part[4] != "steer.ndjson" {
			return nil, fmt.Errorf("%s: not stores/<run>/runs/<session>/{records,steer}.ndjson", rel)
		}
		r := byRun[part[1]]
		if r == nil {
			r = &fixtureRun{Name: part[1], Session: part[3]}
			byRun[part[1]] = r
		}
		if r.Session != part[3] {
			return nil, fmt.Errorf("%s: a second session in run %s", rel, part[1])
		}
		for i, ln := range bytes.Split(bytes.TrimSuffix(b, []byte("\n")), []byte("\n")) {
			ev, ok, err := decodeLine(part[4], ln)
			if err != nil {
				return nil, fmt.Errorf("%s line %d: %w", rel, i+1, err)
			}
			if ok {
				r.Events = append(r.Events, ev)
			}
		}
	}
	runs := make([]fixtureRun, 0, len(byRun))
	for _, name := range slices.Sorted(maps.Keys(byRun)) {
		runs = append(runs, *byRun[name])
	}
	return runs, nil
}

// decodeLine decodes one line of a run's file named file: records.ndjson by
// RecordEvent, steer.ndjson by SteerEvent.
func decodeLine(file string, line []byte) (Event, bool, error) {
	if file == "records.ndjson" {
		return RecordEvent(line)
	}
	return SteerEvent(line)
}

// loadFixtures is readFixtures over testdata/sessions, failing t on an error.
func loadFixtures(t *testing.T) []fixtureRun {
	t.Helper()
	runs, err := readFixtures(filepath.Join("testdata", "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	return runs
}

// inOrder is a copy of evs in the merged order, sorted by Event.Before alone:
// the tests' own sort, so a fault in sortedCopy cannot hide in an oracle.
func inOrder(evs []Event) []Event {
	s := slices.Clone(evs)
	sort.SliceStable(s, func(i, j int) bool { return s[i].Before(s[j]) })
	return s
}

// steppedAs is foldByStep for session sid: a new frontier stepped through
// evs as given, which the caller has put in the merged order, Through the
// last one's place.
func steppedAs(sid string, evs []Event) Frontier {
	f := newFrontier(sid)
	for _, ev := range evs {
		Step(&f, ev)
	}
	if len(evs) > 0 {
		last := evs[len(evs)-1]
		f.Through = Mark{Seq: last.Seq, Steer: last.Steer, AtUnixMS: last.AtUnixMS, ID: last.ID}
	}
	return f
}

// firstDiff names the first byte where got and want part, with a little of
// each from there.
func firstDiff(got, want string) string {
	i := 0
	for i < len(got) && i < len(want) && got[i] == want[i] {
		i++
	}
	lo := max(i-40, 0)
	return fmt.Sprintf("first difference on byte %d:\n got ...%s\nwant ...%s", i, got[lo:min(i+80, len(got))], want[lo:min(i+80, len(want))])
}

// TestFixtures_SumsHold: the fixture set is the files SHA256SUMS.txt lists,
// byte for byte, and nothing else under stores/: 42 runs that decode to 12919
// record events and 406 steer line events (their 81 steer_evidence lines are
// none), so every fixture test reads the set PROVENANCE.txt describes. On a
// set of one SYNTHETIC run, a byte edited by hand fails, and so does a file
// the sums do not list.
func TestFixtures_SumsHold(t *testing.T) {
	runs := loadFixtures(t)
	var recs, lines int
	for _, r := range runs {
		for _, ev := range r.Events {
			if ev.Steer {
				lines++
			} else {
				recs++
			}
		}
	}
	if len(runs) != 42 || recs != 12919 || lines != 406 {
		t.Errorf("%d runs, %d record events, %d steer line events; want 42, 12919, 406", len(runs), recs, lines)
	}
	root := t.TempDir()
	write := func(rel string, b []byte) {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	rel := "stores/r1/runs/s1/records.ndjson"
	body := []byte(`{"type":"declaration","seq":1,"cwd":"/p","project":"/p","file_path":"/p/a.go"}` + "\n")
	sum := sha256.Sum256(body)
	write("SHA256SUMS.txt", []byte(hex.EncodeToString(sum[:])+"  "+rel+"\n"))
	write(rel, body)
	if got, err := readFixtures(root); err != nil || len(got) != 1 || len(got[0].Events) != 1 || !slices.Equal(got[0].Events[0].Wrote, []string{"a.go"}) {
		t.Fatalf("a set its sums hold: %v, %+v", err, got)
	}
	write(rel, bytes.Replace(body, []byte("a.go"), []byte("b.go"), 1))
	if _, err := readFixtures(root); err == nil || !strings.Contains(err.Error(), "sha256") {
		t.Errorf("a byte edited by hand: %v; want its sha256 refused", err)
	}
	write(rel, body)
	write("stores/r1/runs/s1/extra.ndjson", body)
	if _, err := readFixtures(root); err == nil || !strings.Contains(err.Error(), "does not list") {
		t.Errorf("a file the sums do not list: %v; want it refused", err)
	}
}

// jsonTagPaths appends to out the path of every leaf key typ decodes, after
// prefix, as keys.txt writes it: "." steps into an object's keys and "[]."
// into a list's rows. A field with no json name fails t, since encoding/json
// would then read a key named for the Go field.
func jsonTagPaths(t *testing.T, typ reflect.Type, prefix string, out *[]string) {
	t.Helper()
	for i := 0; i < typ.NumField(); i++ {
		fl := typ.Field(i)
		name, _, _ := strings.Cut(fl.Tag.Get("json"), ",")
		if name == "" || name == "-" || !fl.IsExported() {
			t.Fatalf("%s%s: a field with no json name", prefix, fl.Name)
		}
		ft := fl.Type
		for ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		el := ft
		if ft.Kind() == reflect.Slice {
			for el = ft.Elem(); el.Kind() == reflect.Pointer; el = el.Elem() {
			}
		}
		switch {
		case ft.Kind() == reflect.Struct:
			jsonTagPaths(t, ft, prefix+name+".", out)
		case ft.Kind() == reflect.Slice && el.Kind() == reflect.Struct:
			jsonTagPaths(t, el, prefix+name+"[].", out)
		default:
			*out = append(*out, prefix+name)
		}
	}
}

// TestFixtures_KeysAreTheDecodersTags: the keys the fixtures were cut to
// (testdata/sessions/keys.txt, also the generator's input) are the json tags
// recordHead and steerHead decode, both ways. A tag the decoders gain is a
// key the cut dropped from every fixture line, so the fixtures would test the
// new rule on nothing: regenerate testdata/sessions from the raw set
// (PROVENANCE.txt). A tag they lose is a key the fixtures keep that the fold
// no longer reads.
func TestFixtures_KeysAreTheDecodersTags(t *testing.T) {
	var got []string
	for _, h := range []struct {
		file string
		typ  reflect.Type
	}{{"records", reflect.TypeOf(recordHead{})}, {"steer", reflect.TypeOf(steerHead{})}} {
		var paths []string
		jsonTagPaths(t, h.typ, "", &paths)
		for _, p := range paths {
			got = append(got, h.file+" "+p)
		}
	}
	sort.Strings(got)
	b, err := os.ReadFile(filepath.Join("testdata", "sessions", "keys.txt"))
	if err != nil {
		t.Fatal(err)
	}
	var want []string
	for _, ln := range strings.Split(string(b), "\n") {
		if ln != "" && !strings.HasPrefix(ln, "#") {
			want = append(want, ln)
		}
	}
	if !slices.IsSorted(want) {
		t.Error("keys.txt is not sorted")
	}
	if !slices.Equal(got, want) {
		var gained, lost []string
		for _, k := range got {
			if !slices.Contains(want, k) {
				gained = append(gained, k)
			}
		}
		for _, k := range want {
			if !slices.Contains(got, k) {
				lost = append(lost, k)
			}
		}
		t.Errorf("the decoders' tags are not keys.txt's; regenerate testdata/sessions from the raw set (PROVENANCE.txt)\nonly the decoders: %q\nonly keys.txt: %q", gained, lost)
	}
}

// TestFixtures_DecodeAsTheRawSet, run where RASHOMON_SESSIONS_RAW names the
// raw set, which never enters the repository, and skipped elsewhere: the raw
// set is the one PROVENANCE.txt names by its SHA256SUMS.txt digest, each raw
// file held to that list; every fixture file has its raw twin and every raw
// file its fixture; and every fixture line decodes as its raw line does, so
// the cut (keys.txt's keys, paths outside the run's tree replaced, ids
// replaced) changed no event. The id map is rebuilt from the raw set by
// PROVENANCE.txt's rule (Ids): the ids the cut keeps are the session id of
// each raw path and the prompt_id of each raw steer line, their count is the
// one PROVENANCE.txt states, and the nth of them in byte order becomes
// 00000000-0000-4000-8000-<n in 12 hex digits>, n counted from 1. A fixture
// finds its raw twin through the map, and each raw line has its ids mapped
// before it is decoded, so the two events compare like with like. It names
// files by their fixture path and lines by number, and prints no line, no
// event and no real id: a raw line holds words and home paths, and a raw
// path a real id.
func TestFixtures_DecodeAsTheRawSet(t *testing.T) {
	raw := os.Getenv("RASHOMON_SESSIONS_RAW")
	if raw == "" {
		t.Skip("RASHOMON_SESSIONS_RAW is unset: the raw set lives outside the repository")
	}
	root := filepath.Join("testdata", "sessions")
	prov, err := os.ReadFile(filepath.Join(root, "PROVENANCE.txt"))
	if err != nil {
		t.Fatal(err)
	}
	_, rest, named := strings.Cut(string(prov), "Raw SHA256SUMS.txt sha256: ")
	want, _, _ := strings.Cut(rest, "\n")
	rawSums, err := os.ReadFile(filepath.Join(raw, "SHA256SUMS.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if sum := sha256.Sum256(rawSums); !named || hex.EncodeToString(sum[:]) != want {
		t.Fatal("RASHOMON_SESSIONS_RAW is not the raw set PROVENANCE.txt names")
	}
	rawListed := map[string]string{}
	for _, ln := range strings.Split(strings.TrimSuffix(string(rawSums), "\n"), "\n") {
		sum, rel, _ := strings.Cut(ln, "  ")
		rawListed[strings.TrimPrefix(rel, "./")] = sum
	}
	// Each raw file is read once and held to its sum, and the ids the cut
	// keeps are gathered from it. A message names a raw file by its place in
	// the sorted list, since its path holds a real id.
	rawData, kept := map[string][]byte{}, map[string]bool{}
	for n, rawRel := range slices.Sorted(maps.Keys(rawListed)) {
		part := strings.Split(rawRel, "/")
		if len(part) != 5 || part[1] != "store" || part[2] != "runs" {
			t.Fatalf("raw file %d: not <run>/store/runs/<session>/<file>", n+1)
		}
		rb, err := os.ReadFile(filepath.Join(raw, "runs", filepath.FromSlash(rawRel)))
		if err != nil {
			t.Fatalf("raw file %d: %v", n+1, errors.Unwrap(err))
		}
		if sum := sha256.Sum256(rb); hex.EncodeToString(sum[:]) != rawListed[rawRel] {
			t.Fatalf("raw file %d is not its SHA256SUMS.txt's", n+1)
		}
		rawData[rawRel] = rb
		kept[part[3]] = true
		if part[4] != "steer.ndjson" {
			continue
		}
		for i, ln := range bytes.Split(bytes.TrimSuffix(rb, []byte("\n")), []byte("\n")) {
			var h struct {
				PromptID *string `json:"prompt_id"`
			}
			if err := json.Unmarshal(ln, &h); err != nil {
				t.Fatalf("raw file %d line %d: %v", n+1, i+1, err)
			}
			if h.PromptID != nil {
				kept[*h.PromptID] = true
			}
		}
	}
	if !strings.Contains(strings.Join(strings.Fields(string(prov)), " "), fmt.Sprintf("the cut keeps %d:", len(kept))) {
		t.Fatalf("the raw set has %d ids the cut keeps, and PROVENANCE.txt states another count", len(kept))
	}
	realID, pairs := map[string]string{}, []string{}
	for n, id := range slices.Sorted(maps.Keys(kept)) {
		ph := fmt.Sprintf("00000000-0000-4000-8000-%012x", n+1)
		realID[ph] = id
		pairs = append(pairs, id, ph)
	}
	toPlaceholders := strings.NewReplacer(pairs...)
	cutSums, err := os.ReadFile(filepath.Join(root, "SHA256SUMS.txt"))
	if err != nil {
		t.Fatal(err)
	}
	files, lines := 0, 0
	for _, ln := range strings.Split(strings.TrimSuffix(string(cutSums), "\n"), "\n") {
		_, rel, _ := strings.Cut(ln, "  ")
		part := strings.Split(rel, "/")
		if len(part) != 5 {
			t.Fatalf("%s: not stores/<run>/runs/<session>/<file>", rel)
		}
		rawRel := part[1] + "/store/runs/" + realID[part[3]] + "/" + part[4]
		rb, ok := rawData[rawRel]
		if !ok {
			t.Errorf("%s: no raw twin", rel)
			continue
		}
		delete(rawData, rawRel)
		cb, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		rl := bytes.Split(bytes.TrimSuffix(rb, []byte("\n")), []byte("\n"))
		cl := bytes.Split(bytes.TrimSuffix(cb, []byte("\n")), []byte("\n"))
		if len(cl) != len(rl) {
			t.Errorf("%s: %d lines, its raw twin %d", rel, len(cl), len(rl))
			continue
		}
		for i := range cl {
			ce, cok, cerr := decodeLine(part[4], cl[i])
			re, rok, rerr := decodeLine(part[4], []byte(toPlaceholders.Replace(string(rl[i]))))
			if cok != rok || (cerr == nil) != (rerr == nil) || !reflect.DeepEqual(ce, re) {
				t.Errorf("%s line %d decodes otherwise than its raw line", rel, i+1)
			}
		}
		files, lines = files+1, lines+len(cl)
	}
	for _, rawRel := range slices.Sorted(maps.Keys(rawData)) {
		run, inStore, _ := strings.Cut(rawRel, "/store/")
		t.Errorf("stores/%s/%s: a raw file with no fixture", run, toPlaceholders.Replace(inStore))
	}
	t.Logf("%d fixture files, %d lines, decode as the raw set's", files, lines)
}

// TestFixtures_KnownHole_AliasWritesDecodeOutside: run t1c-r6's project is
// under /private/tmp, and 29 of its declarations, seq 154 to 422, each name
// one write through /tmp, that directory's alias: a path inside the project
// once /private is put back. inProject reads a path as text, as verify does,
// so today each of the 29 decodes outside: WroteOutside set, and the path not
// in Wrote. The test finds the 29 by its own reading of each line, not the
// decoder's. A fix that resolves the /tmp alias turns them inside, and must
// change this test by name, never a golden silently; the generator kept these
// paths verbatim for it.
func TestFixtures_KnownHole_AliasWritesDecodeOutside(t *testing.T) {
	dir := filepath.Join("testdata", "sessions", "stores", "t1c-r6", "runs")
	sids, err := os.ReadDir(dir)
	if err != nil || len(sids) != 1 {
		t.Fatalf("t1c-r6: %v, %d sessions; want one", err, len(sids))
	}
	b, err := os.ReadFile(filepath.Join(dir, sids[0].Name(), "records.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	var seqs []int64
	for i, ln := range bytes.Split(bytes.TrimSuffix(b, []byte("\n")), []byte("\n")) {
		var h struct {
			Type         string   `json:"type"`
			Seq          int64    `json:"seq"`
			Project      string   `json:"project"`
			FilePath     *string  `json:"file_path"`
			WrittenPaths []string `json:"written_paths"`
		}
		if err := json.Unmarshal(ln, &h); err != nil {
			t.Fatalf("line %d: %v", i+1, err)
		}
		if h.Type != store.TypeDeclaration || !strings.HasPrefix(h.Project, "/private/tmp/") {
			continue
		}
		paths := slices.Clone(h.WrittenPaths)
		if h.FilePath != nil {
			paths = append(paths, *h.FilePath)
		}
		var alias []string
		for _, p := range paths {
			if rel, ok := strings.CutPrefix("/private"+p, h.Project+"/"); ok && strings.HasPrefix(p, "/tmp/") {
				alias = append(alias, rel)
			}
		}
		if len(alias) == 0 {
			continue
		}
		ev, _, err := RecordEvent(ln)
		if err != nil {
			t.Fatalf("seq %d: %v", h.Seq, err)
		}
		if !ev.WroteOutside {
			t.Errorf("seq %d: a write through the /tmp alias left WroteOutside unset", h.Seq)
		}
		for _, rel := range alias {
			if slices.Contains(ev.Wrote, rel) {
				t.Errorf("seq %d: %s, written through the alias, is in Wrote", h.Seq, rel)
			}
		}
		if len(alias) != 1 {
			t.Errorf("seq %d: %d writes through the alias; want one", h.Seq, len(alias))
		}
		seqs = append(seqs, h.Seq)
	}
	if len(seqs) != 29 || seqs[0] != 154 || seqs[len(seqs)-1] != 422 {
		t.Errorf("writes through the alias on seqs %v; want 29, from 154 to 422", seqs)
	}
}

// TestFold_IncrementalEqualsFull: for each recorded session of
// testdata/sessions and every cut j of its events in the merged order, the
// frontier stepped to j, with Through at the last event stepped as its bound,
// saved and read back, then caught up by the events after j in a shuffled
// order, is byte-equal to Fold over the whole log, and Fold over the log as
// its files give it is its events stepped in the merged order. Every frontier
// stepped to a cut keeps SupersededBy set exactly while Status is superseded.
// The synthetic form, every pair of cuts, is
// TestFold_IncrementalEqualsFullSynthetic.
func TestFold_IncrementalEqualsFull(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for _, run := range loadFixtures(t) {
		t.Run(run.Name, func(t *testing.T) {
			evs := inOrder(run.Events)
			full := mustMarshal(t, Fold(run.Session, run.Events))
			if want := mustMarshal(t, steppedAs(run.Session, evs)); full != want {
				t.Fatalf("the fold is not its events stepped in the merged order; %s", firstDiff(full, want))
			}
			f := newFrontier(run.Session)
			for j := 0; j <= len(evs); j++ {
				if j > 0 {
					ev := evs[j-1]
					Step(&f, ev)
					f.Through = Mark{Seq: ev.Seq, Steer: ev.Steer, AtUnixMS: ev.AtUnixMS, ID: ev.ID}
				}
				if bad := supersededByWhileSuperseded(f); bad != "" {
					t.Fatalf("stepped to %d: %s", j, bad)
				}
				saved, err := ParseFrontier([]byte(mustMarshal(t, f)))
				if err != nil {
					t.Fatalf("read back the frontier stepped to %d: %v", j, err)
				}
				tail := slices.Clone(evs[j:])
				r.Shuffle(len(tail), func(a, b int) { tail[a], tail[b] = tail[b], tail[a] })
				g, ok := Catch(saved, tail)
				if !ok {
					t.Fatalf("catch from %d of %d missed", j, len(evs))
				}
				if got := mustMarshal(t, g); got != full {
					t.Fatalf("catch from %d of %d is not the fold over the whole log; %s", j, len(evs), firstDiff(got, full))
				}
			}
		})
	}
}

// TestFold_IsDeterministic: each recorded session's events, given in 5
// shuffled orders, fold to the bytes of its events stepped in the merged
// order. The synthetic form is TestFold_IsDeterministicSynthetic.
func TestFold_IsDeterministic(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for _, run := range loadFixtures(t) {
		want := mustMarshal(t, steppedAs(run.Session, inOrder(run.Events)))
		in := slices.Clone(run.Events)
		for round := 0; round < 5; round++ {
			r.Shuffle(len(in), func(i, j int) { in[i], in[j] = in[j], in[i] })
			if got := mustMarshal(t, Fold(run.Session, in)); got != want {
				t.Errorf("%s, round %d: %s", run.Name, round, firstDiff(got, want))
				break
			}
		}
	}
}
