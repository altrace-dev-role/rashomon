package acceptance

// SRS1, the hook's half (card SPEC-RULES-STOP1): the post keeps each
// forbid_command rule the call matched as a spec_event rule_hit before Route,
// at either post event, whatever Route then decides.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

var dghPush = map[string]any{"command": "git push --force origin main"}

// recordsOfType is the session's records.ndjson lines of type typ, decoded;
// "" is every line.
func (e *env) recordsOfType(typ string) []map[string]any {
	e.t.Helper()
	f, err := os.Open(filepath.Join(e.home, "runs", testSession, "records.ndjson"))
	if err != nil {
		e.t.Fatal(err)
	}
	defer f.Close()
	var out []map[string]any
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for sc.Scan() {
		var m map[string]any
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			e.t.Fatalf("records line %q: %v", sc.Text(), err)
		}
		if typ == "" || m["type"] == typ {
			out = append(out, m)
		}
	}
	if err := sc.Err(); err != nil {
		e.t.Fatal(err)
	}
	return out
}

// TestSRS1f_AHitIsKeptWhateverRouteDecides: with the user's steering switch
// off, Route clamps the rule's note to a flag for the user, so the agent is
// told nothing; on a lab store under the operator's forced off arm, Route
// prints nothing at all. Either way the forced push's hit is in the run and
// the ledger's no-force row is unmet.
func TestSRS1f_AHitIsKeptWhateverRouteDecides(t *testing.T) {
	for _, c := range []struct {
		name, file, content string
		env                 []string
		printsNothing       bool
	}{
		{"steering off", "config.json", `{"steer_user":"off"}`, nil, false},
		{"forced off arm", store.FileLab, "lab\n", []string{"RASHOMON_STEER_ARM=off"}, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := newSteerEnv(t)
			dir, path := dghProject(t, dgh5Spec)
			e.trust(path)
			if err := os.WriteFile(filepath.Join(e.home, c.file), []byte(c.content), 0o600); err != nil {
				t.Fatal(err)
			}
			out := e.dghCall(dir, "toolu_p", "Bash", dghPush, c.env...)
			if strings.Contains(out, "additionalContext") || c.printsNothing && out != "" {
				t.Errorf("post printed %q, want no note to the agent (and, under the off arm, nothing)", out)
			}
			if hits := e.recordsOfType("spec_event"); len(hits) != 1 || hits[0]["reason"] != "rule_hit" || hits[0]["rule_id"] != "spec:no-force" {
				t.Fatalf("spec_event records %v, want the one no-force hit", hits)
			}
			e.dghStop(dir)
			if row := e.dghLedgerRow("spec:no-force"); row == nil || row[4] != "unmet" {
				t.Errorf("ledger no-force row %v, want unmet", row)
			}
		})
	}
}

// postSyncs runs one push of a trusted no-force session, hook then post, at
// PostToolUse or (failed) PostToolUseFailure exit 128, with the sync
// recorder on both, and returns their syncs, each file by its base name, and
// the post's stdout.
func postSyncs(t *testing.T, command string, failed bool) ([]syncEvent, string) {
	t.Helper()
	e := newSteerEnv(t)
	dir, path := dghProject(t, dgh5Spec)
	e.trust(path)
	log := filepath.Join(t.TempDir(), "sync.log")
	in := map[string]any{"command": command}
	var out string
	if failed {
		out = e.dghFailedCall(dir, "toolu_p", "Bash", in, 128, syncLogEnv+"="+log)
	} else {
		out = e.dghCall(dir, "toolu_p", "Bash", in, syncLogEnv+"="+log)
	}
	events := readSyncLog(t, log)
	for i := range events {
		events[i].file = filepath.Base(events[i].file)
	}
	return events, out
}

// TestSRS1i_AHitAddsOneAppendAndAtMostOneFullSync (G1): a failed push's
// post with a hit syncs what one with none does, then the hit's ordered
// append (its seq counter and its line, each inside the lock) and, on
// darwin, one full sync of the records outside every lock, last. At
// PostToolUse the hit's append comes before the steer line's, and the post
// still ends in exactly one full sync after it, the steer log's: the hit
// adds no full sync of its own there.
func TestSRS1i_AHitAddsOneAppendAndAtMostOneFullSync(t *testing.T) {
	clean, _ := postSyncs(t, "git push origin main", true)
	for _, ev := range clean {
		if ev.file == "steer.ndjson" {
			t.Fatalf("premise: the clean failure's post wrote a steer line: %v", clean)
		}
	}
	hit, _ := postSyncs(t, "git push --force origin main", true)
	ordering := "barrier"
	if flushesPerProcess() == 0 {
		ordering = "barrier-full"
	}
	want := append(slices.Clone(clean),
		syncEvent{kind: ordering, file: "seq", locked: true}, syncEvent{kind: ordering, file: "records.ndjson", locked: true})
	if flushesPerProcess() == 1 {
		want = append(want, syncEvent{kind: "full", file: "records.ndjson"})
	}
	if !slices.Equal(hit, want) {
		t.Errorf("failure post syncs with a hit\n%v\nwant the clean post's and then %v", hit, want[len(clean):])
	}

	post, out := postSyncs(t, "git push --force origin main", false)
	if out == "" {
		t.Fatal("premise: the forced push's post printed no note")
	}
	hitAt, steerAt := -1, -1
	for i, ev := range post {
		if ev.file == "steer.ndjson" && ev.kind != "full" {
			steerAt = i
		}
	}
	for i := steerAt - 1; i >= 0 && hitAt < 0; i-- {
		if post[i].file == "records.ndjson" && post[i].kind != "full" {
			hitAt = i
		}
	}
	if steerAt < 0 || hitAt < 0 {
		t.Fatalf("PostToolUse syncs %v: want the hit's append on the records before the steer line's", post)
	}
	for _, ev := range post[hitAt+1 : steerAt] {
		if ev.kind == "full" {
			t.Errorf("PostToolUse syncs %v: a full sync between the hit and the steer line", post)
		}
	}
	tail := post[steerAt+1:]
	if flushesPerProcess() == 1 && (len(tail) != 1 || tail[0] != (syncEvent{kind: "full", file: "steer.ndjson"})) {
		t.Errorf("after the steer line's barrier: %v; want exactly one full sync of the steer log, outside every lock", tail)
	}
}

// TestSRS1k_AFailedSteerFlushLeavesTheExecutionAsIs (G4): with the steer
// step's flush failed, the post prints nothing and the hit stays in the file
// unmarked; the push has its one execution and no record is marked
// flush_failed, as with no fault: the record phase's flush succeeded.
func TestSRS1k_AFailedSteerFlushLeavesTheExecutionAsIs(t *testing.T) {
	e := newSteerEnv(t)
	dir, path := dghProject(t, dgh5Spec)
	e.trust(path)
	if out := e.dghCall(dir, "toolu_p", "Bash", dghPush, "RASHOMON_FAULT=steer.flush:fail=1000000"); out != "" {
		t.Errorf("post printed %q with the steer flush failed, want nothing", out)
	}
	if hits := e.recordsOfType("spec_event"); len(hits) != 1 {
		t.Errorf("spec_event records %v, want the hit", hits)
	}
	execs := 0
	for _, x := range e.recordsOfType("execution") {
		if x["tool_use_id"] == "toolu_p" {
			execs++
		}
	}
	if execs != 1 {
		t.Errorf("%d executions of the push, want 1", execs)
	}
	for _, r := range e.recordsOfType("") {
		if r["reason"] == store.ReasonFlushFailed {
			t.Errorf("record %v marked %s, want none: the record phase's flush succeeded", r, store.ReasonFlushFailed)
		}
	}
}

// TestH16b_ConcurrentHitsAllLand (G3): forced pushes among concurrent posts
// of one session. Every ordered record has a distinct seq, the seqs run 1 to
// n with no hole, and each forced push's execution has its one hit, naming
// that execution's seq.
func TestH16b_ConcurrentHitsAllLand(t *testing.T) {
	e := newSteerEnv(t)
	dir, path := dghProject(t, dgh5Spec)
	e.trust(path)
	const n = 16
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		cmd := "git status"
		if i%2 == 0 {
			cmd = "git push --force origin main"
		}
		id := fmt.Sprintf("toolu_%02d", i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			e.dghCall(dir, id, "Bash", map[string]any{"command": cmd})
		}()
	}
	wg.Wait()
	seqs := map[float64]bool{}
	forced := map[float64]bool{}
	for _, r := range e.recordsOfType("") {
		s, ok := r["seq"].(float64)
		if !ok {
			continue
		}
		if seqs[s] {
			t.Errorf("seq %v appears twice", s)
		}
		seqs[s] = true
		var i int
		if _, err := fmt.Sscanf(fmt.Sprint(r["tool_use_id"]), "toolu_%02d", &i); r["type"] == "execution" && err == nil && i%2 == 0 {
			forced[s] = true
		}
	}
	for s := 1; s <= len(seqs); s++ {
		if !seqs[float64(s)] {
			t.Errorf("seq %d missing: the ordered seqs have a hole", s)
		}
	}
	hit := map[float64]bool{}
	for _, h := range e.recordsOfType("spec_event") {
		landed, _ := h["landed_seq"].(float64)
		if !forced[landed] || hit[landed] {
			t.Errorf("hit %v names no forced push's execution, or repeats one", h)
		}
		hit[landed] = true
	}
	if len(forced) != n/2 || len(hit) != len(forced) {
		t.Errorf("%d hits for %d forced pushes' executions, want %d each", len(hit), len(forced), n/2)
	}
}
