package acceptance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// The steer tests run the binary as Claude Code 2.1.280 on the CLI, attended
// (CI empty, whatever the machine running the suite has set), on a watched
// install with record scope v5 and both consent.json and the
// steer_disclosure marker at 2 (card 2.3 R11, A2, A3).
const stAgent = "AI_AGENT=claude-code_2-1-280_harness"

var (
	stGoTest  = map[string]any{"command": "go test ./..."}
	stGoBuild = map[string]any{"command": "go build ./..."}
	stEdit    = map[string]any{"file_path": "/tmp/project/internal/a.go", "old_string": "a", "new_string": "b"}
)

func newSteerEnv(t *testing.T) *env {
	t.Helper()
	e := newEnv(t)
	e.watched(testSession)
	raised := strings.Replace(string(e.consentBytes()), `"steer_disclosure":0`, `"steer_disclosure":2`, 1)
	if raised == string(e.consentBytes()) {
		t.Fatal("premise: the watch record does not carry steer_disclosure 0")
	}
	if err := os.WriteFile(filepath.Join(e.home, "consent.json"), []byte(raised), 0o600); err != nil {
		t.Fatal(err)
	}
	e.steerMarker("2\n")
	if b, err := os.ReadFile(filepath.Join(e.home, "record_scope")); err != nil || strings.TrimSpace(string(b)) != "v5" {
		t.Fatalf("premise: record_scope marker %q (%v), want v5", b, err)
	}
	return e
}

// steerMarker writes the steer_disclosure marker, or removes it for "".
func (e *env) steerMarker(content string) {
	e.t.Helper()
	path := filepath.Join(e.home, "steer_disclosure")
	if content == "" {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			e.t.Fatal(err)
		}
		return
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		e.t.Fatal(err)
	}
}

func stPayload(t *testing.T, event, id, tool string, input map[string]any, extra map[string]any) string {
	t.Helper()
	p := map[string]any{"hook_event_name": event, "session_id": testSession, "prompt_id": "p1",
		"transcript_path": "/tmp/transcripts/sess-1.jsonl", "cwd": "/tmp/project", "permission_mode": "default",
		"tool_name": tool, "tool_input": input, "tool_use_id": id}
	for k, v := range extra {
		p[k] = v
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// stCall runs one tool call through the binary, hook then post (a
// PostToolUseFailure post when exit is not 0), and returns the post's
// result. extraEnv follows the steer defaults and wins.
func (e *env) stCall(id, tool string, input map[string]any, exit int, extraEnv ...string) result {
	e.t.Helper()
	envs := append([]string{stAgent, "CI="}, extraEnv...)
	if res := e.hook(stPayload(e.t, "PreToolUse", id, tool, input, nil), envs...); res.exitCode != 0 || res.stdout != "" {
		e.t.Fatalf("hook %s: exit %d, stdout %q, stderr %q", id, res.exitCode, res.stdout, res.stderr)
	}
	event, more := "PostToolUse", map[string]any{"tool_response": map[string]any{"stdout": "", "stderr": "", "interrupted": false}}
	if exit != 0 {
		event, more = "PostToolUseFailure", map[string]any{"error": "Exit code " + strconv.Itoa(exit), "is_interrupt": false, "duration_ms": 30}
	}
	res := e.post(stPayload(e.t, event, id, tool, input, more), envs...)
	if res.exitCode != 0 {
		e.t.Fatalf("post %s: exit %d, stderr %q", id, res.exitCode, res.stderr)
	}
	return res
}

var stamp = regexp.MustCompile(`"recorded_at_unix_ms":(\d{13})`)

// backdate moves the recorded time of call id's declaration back by ms, in
// place and at the same length: the binary has no clock to set, and the
// minutes the note states are read from these times (card 2.3 R7).
func (e *env) backdate(id string, ms int64) {
	e.t.Helper()
	path := filepath.Join(e.home, "runs", testSession, "records.ndjson")
	b, err := os.ReadFile(path)
	if err != nil {
		e.t.Fatal(err)
	}
	lines := strings.SplitAfter(string(b), "\n")
	n := 0
	for i, l := range lines {
		if !strings.Contains(l, `"type":"declaration"`) || !strings.Contains(l, `"tool_use_id":"`+id+`"`) {
			continue
		}
		m := stamp.FindStringSubmatchIndex(l)
		if m == nil {
			e.t.Fatalf("declaration %s has no 13-digit recorded_at_unix_ms: %s", id, l)
		}
		at, _ := strconv.ParseInt(l[m[2]:m[3]], 10, 64)
		lines[i] = l[:m[2]] + strconv.FormatInt(at-ms, 10) + l[m[3]:]
		n++
	}
	if n != 1 {
		e.t.Fatalf("backdate: %d declarations of %s, want 1", n, id)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "")), 0o600); err != nil {
		e.t.Fatal(err)
	}
}

// seqOf is the seq of call id's record of type typ.
func (e *env) seqOf(typ, id string) int64 {
	e.t.Helper()
	for _, r := range recordsOfType(e.read(testSession, "records.ndjson"), typ) {
		if r.str("tool_use_id") == id {
			if f, ok := r.fields["seq"].(float64); ok {
				return int64(f)
			}
		}
	}
	e.t.Fatalf("no %s with a seq for %s", typ, id)
	return 0
}

func (e *env) steerLines() []record {
	e.t.Helper()
	return e.read(testSession, "steer.ndjson")
}

// regression lands a green `go test ./...` three minutes before its red,
// with an Edit of internal/a.go two minutes before it, and returns the red
// post's result.
func (e *env) regression(redID string, extraEnv ...string) result {
	e.t.Helper()
	for _, c := range []struct {
		id    string
		tool  string
		input map[string]any
	}{{"toolu_g", "Bash", stGoTest}, {"toolu_e", "Edit", stEdit}} {
		if res := e.stCall(c.id, c.tool, c.input, 0, extraEnv...); res.stdout != "" {
			e.t.Fatalf("the post of %s printed %q, want nothing", c.id, res.stdout)
		}
	}
	e.backdate("toolu_g", 180_000)
	e.backdate("toolu_e", 120_000)
	return e.stCall(redID, "Bash", stGoTest, 1, extraEnv...)
}

func numbers(v any) []int64 {
	list, _ := v.([]any)
	out := make([]int64, 0, len(list))
	for _, x := range list {
		f, _ := x.(float64)
		out = append(out, int64(f))
	}
	return out
}

func strs(v any) []string {
	list, _ := v.([]any)
	out := make([]string, 0, len(list))
	for _, x := range list {
		s, _ := x.(string)
		out = append(out, s)
	}
	return out
}

// TestSTR43_RegressionLocationNamesWritesBetweenGreenAndRed: a green go
// test, an edit, then a red go test: the red's post decides the
// regression-location note. Under the shipped table PostToolUseFailure's
// model channel is unverified (D69), so it prints nothing and logs one
// suppressed line whose slots re-render to the card's note; the line keeps
// no text, as every undelivered line (RES-32). Under record scope v4 the
// write is unnamed and the slots are the degraded row's.
func TestSTR43_RegressionLocationNamesWritesBetweenGreenAndRed(t *testing.T) {
	check := func(t *testing.T, e *env, res result) map[string]any {
		t.Helper()
		if res.stdout != "" {
			t.Errorf("the red's post printed %q, want nothing (D69: the channel is unverified)", res.stdout)
		}
		lines := e.steerLines()
		if len(lines) != 1 {
			t.Fatalf("%d steer lines, want 1", len(lines))
		}
		l := lines[0]
		if l.str("pattern") != string(pattern.RegressionLocation) || l.str("state") != "suppressed" ||
			l.str("suppressed_by") != "unverified_channel" || l.str("level") != "nudge" ||
			l.str("event") != "PostToolUseFailure" || l.str("attendance") != "attended" ||
			l.str("model_text") != "" || l.str("prompt_id") != "p1" {
			t.Errorf("line = %s", l.raw)
		}
		if !strings.HasPrefix(l.str("key"), "learned:test:") {
			t.Errorf("key %q, want the go test gate's id", l.str("key"))
		}
		if after, _ := l.fields["after_seq"].(float64); int64(after) != e.seqOf("execution", "toolu_r") {
			t.Errorf("after_seq %v, want the red execution's seq %d", l.fields["after_seq"], e.seqOf("execution", "toolu_r"))
		}
		if got, want := numbers(l.fields["evidence_seqs"]), []int64{e.seqOf("declaration", "toolu_g"), e.seqOf("declaration", "toolu_r")}; !slices.Equal(got, want) {
			t.Errorf("evidence_seqs %v, want the two runs %v", got, want)
		}
		if v, _ := l.fields["renderer_version"].(float64); v != render.Version {
			t.Errorf("renderer_version %v, want %d", l.fields["renderer_version"], render.Version)
		}
		slots, _ := l.fields["slots"].(map[string]any)
		if slots["gate"] != "go" || slots["minutes"] != float64(3) || slots["exit"] != float64(1) {
			t.Errorf("slots %v, want gate go, minutes 3, exit 1", slots)
		}
		return slots
	}
	t.Run("v5", func(t *testing.T) {
		e := newSteerEnv(t)
		slots := check(t, e, e.regression("toolu_r"))
		paths := strs(slots["paths"])
		if !slices.Equal(paths, []string{"internal/a.go"}) || slots["more"] != float64(0) {
			t.Fatalf("slots %v, want paths [internal/a.go] and more 0", slots)
		}
		gate, _ := slots["gate"].(string)
		m, err := render.New(pattern.RegressionLocation, render.RegressionLocationBinding{
			Gate: slot.Gate(gate), Minutes: 3, Exit: 1, Paths: slot.NewPathList([]slot.Path{slot.Path(paths[0])}, 0)})
		want := "rashomon record: gate 'go' exited 0 earlier in this session (3 min ago) and exited 1 on this run. " +
			"Files written between the two runs: 'internal/a.go'."
		if err != nil || m.Text() != want {
			t.Errorf("the line re-renders to %q (%v), want %q", m.Text(), err, want)
		}
	})
	t.Run("v4", func(t *testing.T) {
		e := newSteerEnv(t)
		if err := os.Remove(filepath.Join(e.home, "record_scope")); err != nil {
			t.Fatal(err)
		}
		slots := check(t, e, e.regression("toolu_r"))
		if _, named := slots["paths"]; named || slots["count"] != float64(1) {
			t.Errorf("slots %v, want the degraded row's: count 1 and no paths (\"1 file writes\")", slots)
		}
		if l := e.steerLines()[0]; l.str("project") != "" {
			t.Errorf("project %q under v4, want \"\": the record holds none and the line follows it (A3)", l.str("project"))
		}
	})
}

// TestSTR44_RegressionLocationSilentOnExit126And127: a red that exited 127
// (not found) or 126 (not executable) did not run the gate, so it locates
// no regression. The control, exit 1 first, logs the line.
func TestSTR44_RegressionLocationSilentOnExit126And127(t *testing.T) {
	e := newSteerEnv(t)
	e.regression("toolu_r")
	if n := len(e.steerLines()); n != 1 {
		t.Fatalf("premise: the exit 1 control logged %d lines, want 1", n)
	}
	for _, exit := range []int{127, 126} {
		id := "toolu_x" + strconv.Itoa(exit)
		if res := e.stCall(id, "Bash", stGoTest, exit); res.stdout != "" {
			t.Errorf("exit %d printed %q", exit, res.stdout)
		}
		if n := len(e.steerLines()); n != 1 {
			t.Errorf("after exit %d: %d steer lines, want still 1", exit, n)
		}
	}
}

// TestSTR43i_ARedOfAnotherFamilyIsNotThisRun: the note says "on this run",
// so only the call that is the gate's red run raises it. A later failed
// `go build ./...`, a failed runner call of another family with no green of
// its own, logs no line for the go test gate. The control, the go test
// red itself, does.
func TestSTR43i_ARedOfAnotherFamilyIsNotThisRun(t *testing.T) {
	e := newSteerEnv(t)
	e.regression("toolu_r")
	if n := len(e.steerLines()); n != 1 {
		t.Fatalf("premise: the go test red logged %d lines, want 1", n)
	}
	if res := e.stCall("toolu_b", "Bash", stGoBuild, 1); res.stdout != "" {
		t.Errorf("the build's post printed %q", res.stdout)
	}
	if n := len(e.steerLines()); n != 1 {
		t.Errorf("after a red of another family: %d steer lines, want still 1", n)
	}
}

// TestST_NoConsentPrintsAFlagOnly: with no steer_disclosure marker the
// nudge is clamped to a flag (consent_missing): stdout carries the
// systemMessage alone, which no channel gates, and no model-visible key.
// consent.json says 2 throughout: the hook reads the marker, never
// consent.json (D49, 2.2c). The threshold is the marker against
// steer.DisclosureVersion, 2.
func TestST_NoConsentPrintsAFlagOnly(t *testing.T) {
	for _, tc := range []struct {
		name, marker string
		flag         bool
	}{
		{"marker absent", "", true},
		{"marker 1", "1\n", true},
		{"control: marker 2", "2\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newSteerEnv(t)
			e.steerMarker(tc.marker)
			res := e.regression("toolu_r")
			lines := e.steerLines()
			if len(lines) != 1 {
				t.Fatalf("%d steer lines, want 1", len(lines))
			}
			l := lines[0]
			if !tc.flag {
				if res.stdout != "" || l.str("suppressed_by") != "unverified_channel" {
					t.Errorf("with consent: stdout %q, line %s; want nothing printed and unverified_channel", res.stdout, l.raw)
				}
				return
			}
			var out map[string]any
			if err := json.Unmarshal([]byte(res.stdout), &out); err != nil {
				t.Fatalf("stdout %q is not the hook's JSON: %v", res.stdout, err)
			}
			msg, _ := out["systemMessage"].(string)
			if _, model := out["hookSpecificOutput"]; model || len(out) != 1 ||
				!strings.Contains(msg, "rashomon record: gate 'go' exited 0 earlier in this session (3 min ago)") {
				t.Errorf("stdout %q, want the systemMessage alone, carrying the note", res.stdout)
			}
			if l.str("level") != "flag" || l.str("state") != "delivered" || l.str("suppressed_by") != "consent_missing" {
				t.Errorf("line %s, want a delivered flag with consent_missing", l.raw)
			}
		})
	}
}

// TestST_UnattendedRunLogsItsAttendance: under CI the line records the run
// as unattended. Nothing prints; the reason is the first clamp 2.2's decide
// reaches, unverified_channel, which it checks before no_mirror.
func TestST_UnattendedRunLogsItsAttendance(t *testing.T) {
	e := newSteerEnv(t)
	res := e.regression("toolu_r", "CI=1")
	if res.stdout != "" {
		t.Errorf("stdout %q under CI, want nothing", res.stdout)
	}
	lines := e.steerLines()
	if len(lines) != 1 || lines[0].str("attendance") != "unattended" || lines[0].str("suppressed_by") != "unverified_channel" {
		t.Fatalf("lines %v, want one unattended line suppressed by unverified_channel", lines)
	}
}

// TestST_AnEditPrintsNothingAndRunsNoStep: with the red state in the
// record, an Edit's post prints nothing and adds no steer line: the step
// runs on failed runner calls only (card 2.3 R3). The control, the red's
// own post, logs the line; consent is absent so a step would print.
func TestST_AnEditPrintsNothingAndRunsNoStep(t *testing.T) {
	e := newSteerEnv(t)
	e.steerMarker("")
	if res := e.regression("toolu_r"); res.stdout == "" {
		t.Fatal("premise: the red's post printed nothing; the control needs the flag")
	}
	before, err := os.ReadFile(filepath.Join(e.home, "runs", testSession, "steer.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	if res := e.stCall("toolu_e2", "Edit", stEdit, 0); res.stdout != "" {
		t.Errorf("the edit's post printed %q", res.stdout)
	}
	after, err := os.ReadFile(filepath.Join(e.home, "runs", testSession, "steer.ndjson"))
	if err != nil || string(after) != string(before) {
		t.Errorf("the steer log changed on an edit's post (%v)", err)
	}
}

// TestST_TheLineIsDurableBeforeItsNote: a post that prints a note makes its
// steer line durable first (REC-34; card 2.3 R4.7, R8). On darwin the line's
// append is ordered by a barrier inside its lock, and the step's full sync of
// the steer log follows it, outside every lock, as the process's last sync;
// off darwin the append's own sync is already full. This is REC-34's order
// proved against a process that prints, which that test's doc leaves to the
// steer output's card; the note printed is the consent flag, v0's printed
// path. The sync log is the red call's alone.
func TestST_TheLineIsDurableBeforeItsNote(t *testing.T) {
	e := newSteerEnv(t)
	e.steerMarker("")
	e.stCall("toolu_g", "Bash", stGoTest, 0)
	e.stCall("toolu_e", "Edit", stEdit, 0)
	log := filepath.Join(t.TempDir(), "sync.log")
	if res := e.stCall("toolu_r", "Bash", stGoTest, 1, syncLogEnv+"="+log); res.stdout == "" {
		t.Fatal("premise: the red's post printed nothing; it should print the consent flag")
	}
	events := readSyncLog(t, log)
	last := -1
	for i, ev := range events {
		if filepath.Base(ev.file) == "steer.ndjson" && ev.kind != "full" {
			last = i
		}
	}
	if last < 0 {
		t.Fatalf("no append to the steer log was synced: %v", events)
	}
	if flushesPerProcess() == 0 {
		if events[last].kind != "barrier-full" {
			t.Errorf("off darwin the steer line's append synced as %q, want barrier-full", events[last].kind)
		}
		return
	}
	tail := events[last+1:]
	if len(tail) != 1 || tail[0].kind != "full" || tail[0].locked || filepath.Base(tail[0].file) != "steer.ndjson" {
		t.Errorf("after the steer line's barrier: %v; want exactly one full sync of the steer log, outside every lock", tail)
	}
}

// TestST_FailedFlushCitesNothing: a post whose record flush failed cites
// nothing in the steer log (card 2.3 R1: a record that may be lost is not
// cited), by one of two mechanisms. At fail=1 the record's Flush fails and
// the flush_failed marker is written and synced, so the post prints nothing
// (REC-34) and the step runs: verify reads the call's spilled terminal as
// spilled_outcome, the run is Incomplete and nothing is found. At fail=2 the
// marker's own full sync fails too (a failed Flush ends the deferral), the
// error reaches cmdPost and its early return (flushLostLine) keeps the step
// from running. Without that return the call would still be uncited through
// spilled_outcome, because the marker's bytes landed (a failed sync is not
// cut back), so fail=2 does not separate it. Only where the marker could not
// be written at all (spill.ndjson read-only) does the run read Complete, and
// the early return alone keeps the step from appending a durable line it
// then cannot print: that subtest is the return's separating input. The
// log is the observable, since stdout is empty in every failure path. The
// control, the same red with no fault, prints the consent flag and logs the
// line. The fault is on the red's post alone. Darwin only
// (skipWithoutAFlush): elsewhere a deferred post issues no flush to fail,
// and a failed barrier lands no seq, so no step (R2).
func TestST_FailedFlushCitesNothing(t *testing.T) {
	skipWithoutAFlush(t)
	const lost = "rashomon: records may not be durable\n"
	failure := map[string]any{"error": "Exit code 1", "is_interrupt": false, "duration_ms": 30}
	for _, tc := range []struct {
		name, fault, stderr string
		readOnlySpill       bool
		lines               int
	}{
		{"flush", pointStoreFullSync + ":fail=1", "", false, 0},
		{"marker sync fails", pointStoreFullSync + ":fail=2", lost, false, 0},
		{"marker unwritable", pointStoreFullSync + ":fail=1", lost, true, 0},
		{"control: no fault", "", "", false, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newSteerEnv(t)
			e.steerMarker("")
			e.stCall("toolu_g", "Bash", stGoTest, 0)
			e.stCall("toolu_e", "Edit", stEdit, 0)
			envs := []string{stAgent, "CI="}
			if res := e.hook(stPayload(t, "PreToolUse", "toolu_r", "Bash", stGoTest, nil), envs...); res.exitCode != 0 || res.stdout != "" {
				t.Fatalf("hook toolu_r: exit %d, stdout %q, stderr %q", res.exitCode, res.stdout, res.stderr)
			}
			if tc.readOnlySpill {
				// The marker's append cannot open the file; the run still reads it.
				if err := os.WriteFile(filepath.Join(e.home, "runs", testSession, "spill.ndjson"), nil, 0o400); err != nil {
					t.Fatal(err)
				}
			}
			if tc.fault != "" {
				envs = append(envs, "RASHOMON_FAULT="+tc.fault)
			}
			res := e.post(stPayload(t, "PostToolUseFailure", "toolu_r", "Bash", stGoTest, failure), envs...)
			lines := e.steerLines()
			if tc.lines == 1 {
				if res.stdout == "" || len(lines) != 1 {
					t.Fatalf("control: stdout %q and %d steer lines; want the consent flag and one line", res.stdout, len(lines))
				}
				return
			}
			if res.exitCode != 0 || res.stdout != "" || res.stderr != tc.stderr {
				t.Errorf("exit %d, stdout %q, stderr %q; want 0, nothing on stdout, stderr %q", res.exitCode, res.stdout, res.stderr, tc.stderr)
			}
			if len(lines) != 0 {
				t.Errorf("%d steer lines after a failed record flush, want none: %s", len(lines), lines[0].raw)
			}
		})
	}
}
