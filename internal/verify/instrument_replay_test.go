package verify_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/altrace-dev-role/rashomon/internal/harness"
	"github.com/altrace-dev-role/rashomon/internal/intent"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// The turn replay: a guard, not a measure. Over one recorded session it
// re-decides each last-turn claim note with the Stop's turn rule
// (verify.TurnActed) and sets the transcript's own account of that turn
// beside it (instrument_truth_test.go), as counts with two baselines: every
// claim read, as the build before the rule read them, and none read. It
// reports the prompts by origin kind against the frame reader
// (harness.Framed) for the task-preflight notes, each read on what its
// fire's hook received (rHookInput), and the notes whose model lines list
// only paths. Counts and prompt ids only; nothing of the transcript is
// copied out.

// What a prompt hook receives, measured on Claude Code 2.1.322 for a
// message from another session, idle and queued alike: the bare element,
// its opening tag alone on the first line with the sender's from= key, the
// body, and its closing tag alone on the last line. The transcript records
// an idle message as the host's display, a lead line before the element and
// a closing paragraph after it, and a queued one as an attachment holding
// the bare element. The hook's reader keeps a prompt's first
// intent.MaxPromptBytes, cut back to a character boundary, and says it cut.
const (
	rPeerOpen    = "<cross-session-message "
	rPeerFrom    = ` from="`
	rPeerClose   = "</cross-session-message>"
	rMissDisplay = "peer display not one element"
)

// rHookInput is the prompt a fire of kind read in text, as its hook
// received it: a peer message's element taken out of its display, then the
// reader's head, cut set where the head is short of the whole. why is
// rMissDisplay where a peer display holds no single element; no input is
// given then.
func rHookInput(kind, text string) (in string, cut bool, why string) {
	if kind == "peer" {
		el, ok := rPeerElement(text)
		if !ok {
			return "", false, rMissDisplay
		}
		text = el
	}
	in, cut = rHead(text)
	return in, cut, ""
}

// rPeerElement takes the one peer element out of a display: from the one
// line that, trimmed, is an opening tag with a from= key, through the one
// line after it that, trimmed, is the closing tag alone. A bare element is
// given back as it is. ok is false for any other display: no element, or
// more than one opening or closing line.
func rPeerElement(display string) (el string, ok bool) {
	lines := strings.Split(display, "\n")
	open, end, opens, ends := -1, -1, 0, 0
	for i, l := range lines {
		s := strings.TrimSpace(l)
		switch {
		case strings.HasPrefix(s, rPeerOpen) && strings.HasSuffix(s, ">") && strings.Contains(s, rPeerFrom):
			open, opens = i, opens+1
		case s == rPeerClose:
			end, ends = i, ends+1
		}
	}
	if opens != 1 || ends != 1 || end < open {
		return "", false
	}
	return strings.Join(lines[open:end+1], "\n"), true
}

// rHead is what the hook's reader keeps of a prompt: all of it, or its
// first intent.MaxPromptBytes cut back to a character boundary, with cut
// set.
func rHead(s string) (head string, cut bool) {
	if len(s) <= intent.MaxPromptBytes {
		return s, false
	}
	k := intent.MaxPromptBytes
	for k > 0 && !utf8.RuneStart(s[k]) {
		k--
	}
	return s[:k], true
}

// replayEnv names a directory holding a session's records.ndjson and
// steer.ndjson and CUT.txt: the session id, the transcript's path, a line
// count and the sha256 of that many lines. Unset, the replay skips.
const replayEnv = "RASHOMON_TURN_REPLAY"

// rVerify is the part of a verify record the replay reads.
type rVerify struct {
	Type       string            `json:"type"`
	Seq        int64             `json:"seq"`
	PromptID   *string           `json:"prompt_id"`
	PointSeq   int64             `json:"point_seq"`
	RecordedAt int64             `json:"recorded_at_unix_ms"`
	Background []json.RawMessage `json:"background_in_flight"`
}

// rNote is the part of a steer line the replay reads.
type rNote struct {
	Type     string `json:"type"`
	Pattern  string `json:"pattern"`
	Key      string `json:"key"`
	State    string `json:"state"`
	PromptID string `json:"prompt_id"`
	AfterSeq int64  `json:"after_seq"`
	AtMS     int64  `json:"at_unix_ms"`
	Slots    struct {
		Evidence []json.RawMessage `json:"evidence"`
		Gates    struct {
			Gates []json.RawMessage `json:"gates"`
		} `json:"gates"`
		Rules struct {
			IDs []json.RawMessage `json:"ids"`
		} `json:"rules"`
		Unknown []json.RawMessage `json:"unknown"`
	} `json:"slots"`
}

// rRow is one last-turn note as the two sides read it.
type rRow struct {
	truth, rule string // the prompt id each side names
	class       tClass
	called      bool   // the truth's turn held a main-agent call
	placed      bool   // the transcript places the note
	stop        bool   // a verify record written at or before the note exists
	acted       bool   // the rule's verdict
	by          string // the class of the call that made the rule's turn count
	windows     int    // background windows in flight at the rule's Stop
}

// rDecidedBy names the first call TurnActed counts, by what counted it: a
// record before may_write, a line that may write, or the call's verb class;
// with ", project not said" where the record does not place it. "" where no
// call counts; "no prompt id" for an empty one, which the rule reads as
// acted. rReplay checks it against TurnActed on every note.
func rDecidedBy(run *store.Run, promptID string, at int64) string {
	if promptID == "" {
		return "no prompt id"
	}
	for i := range run.Declarations {
		d := &run.Declarations[i]
		if d.PromptID == nil || *d.PromptID != promptID || d.Seq > at || d.AgentID != nil || !verify.WritesOrRuns(d) {
			continue
		}
		p := verify.ProjectOf(d)
		if verify.InNoProject(p) {
			continue
		}
		c := d.Shape.VerbClass
		switch {
		case d.SchemaVersion < 3:
			c = "before may_write"
		case d.Shape.MayWrite:
			c = "may-write"
		}
		if p == nil {
			c += ", project not said"
		}
		return c
	}
	return ""
}

// rReplay reads each last-turn claim note both ways. The note is built at
// the prompt after its Stop, with the run's last seq as its after_seq, and
// that seq counts no verify record (Run.LastSeq); the Stop's verify record is
// written after its point, so its own seq is above the note's after_seq and
// a later text-only turn's Stop shares the point. The rule's Stop is the
// last verify record written at or before the note, as the flagged Stop is
// the last one before the prompt that takes it; its prompt id and point
// decide the rule's turn.
func rReplay(t *testing.T, ls []tLine, run *store.Run, vs []rVerify, notes []rNote) []rRow {
	t.Helper()
	var rows []rRow
	for _, n := range notes {
		if n.Type != "steer" || n.Pattern != "claim-vs-record" || n.Key != "last-turn" {
			continue
		}
		var r rRow
		r.truth, r.class, r.called, r.placed = tTurn(ls, n.PromptID)
		var v *rVerify
		for i := range vs {
			if vs[i].Type == "verify" && vs[i].RecordedAt <= n.AtMS && (v == nil || vs[i].RecordedAt >= v.RecordedAt) {
				v = &vs[i]
			}
		}
		if v != nil {
			r.stop = true
			if v.PromptID != nil {
				r.rule = *v.PromptID
			}
			r.acted = verify.TurnActed(run, r.rule, v.PointSeq)
			r.by = rDecidedBy(run, r.rule, v.PointSeq)
			if (r.by != "") != r.acted {
				t.Errorf("the replay names %q as what counted prompt %s, and TurnActed reads %t", r.by, r.rule, r.acted)
			}
			r.windows = len(v.Background)
		}
		rows = append(rows, r)
	}
	return rows
}

// rTable is the replay's table: cells[truth class][0 rule acted, 1 not];
// apart, the notes the two sides name by different prompt ids (counted,
// never scored) and those either side cannot place.
type rTable struct {
	cells              [3][2]int
	mismatch, unplaced int
	calledNoAct        int            // no-act turns, in the cells, that held any main-agent call
	residual           map[string]int // notes left on a no-act turn, by what counted the turn
	bashBy             map[string]int // notes on a Bash-only turn the rule acts on, by what counted the turn
	droppedInWindow    int            // notes the rule drops on a no-act turn inside a background window
	windows            int            // the windows those notes sit in
}

// rTabulate fills the table from the rows.
func rTabulate(rows []rRow) rTable {
	tb := rTable{residual: map[string]int{}, bashBy: map[string]int{}}
	for _, r := range rows {
		switch {
		case !r.placed || !r.stop:
			tb.unplaced++
			continue
		case r.truth != r.rule:
			tb.mismatch++
			continue
		}
		col := 1
		if r.acted {
			col = 0
		}
		tb.cells[r.class][col]++
		if r.class == tBashOnly && r.acted {
			tb.bashBy[r.by]++
		}
		if r.class != tNoAct {
			continue
		}
		if r.called {
			tb.calledNoAct++
		}
		if r.acted {
			tb.residual[r.by]++
		} else if r.windows > 0 {
			tb.droppedInWindow++
			tb.windows += r.windows
		}
	}
	return tb
}

// rReport prints the table whole, each cell beside its two baselines.
func rReport(t *testing.T, notes int, tb rTable) {
	t.Helper()
	t.Logf("TURN REPLAY: a guard, not a measure. Counts only, no rate.")
	t.Logf("last-turn claim notes %d; placed by neither side or not by one %d; named by different prompt ids %d", notes, tb.unplaced, tb.mismatch)
	t.Logf("%-12s | %-11s | %-15s | %-26s | %s", "truth", "rule acted", "rule not acted", "every claim read: acted/not", "none read: acted/not")
	for c := tFileTool; c >= tNoAct; c-- {
		n := tb.cells[c][0] + tb.cells[c][1]
		t.Logf("%-12s | %-11d | %-15d | %-26s | %s", c, tb.cells[c][0], tb.cells[c][1], fmt.Sprintf("%d/0", n), fmt.Sprintf("0/%d", n))
	}
	t.Logf("no act: %d of %d turns held a main-agent tool call", tb.calledNoAct, tb.cells[tNoAct][0]+tb.cells[tNoAct][1])
	t.Logf("residual notes (truth no act, rule acted) %d; lost catches (truth a file tool, rule not acted) %d",
		tb.cells[tNoAct][0], tb.cells[tFileTool][1])
	for _, k := range rKeys(tb.residual) {
		t.Logf("residual notes counted by %s: %d", k, tb.residual[k])
	}
	for _, k := range rKeys(tb.bashBy) {
		t.Logf("Bash-only notes counted by %s: %d", k, tb.bashBy[k])
	}
	t.Logf("background windows (source: background_in_flight of the rule's Stop record): notes the rule drops on a no-act turn inside a window %d; windows %d", tb.droppedInWindow, tb.windows)
}

// rKeys is a count map's keys, sorted.
func rKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// rCnt is one kind's row: its notes, and of them those the frame reader
// reads framed and typed on the hook's input.
type rCnt struct{ notes, framed, typed int }

// rPre is the task-preflight count (rPreflightCount).
type rPre struct {
	by                   map[string]*rCnt
	miss                 map[string]int // notes with no record or no hook input, by reason
	early, late, lateOne int            // lateOne: late notes read on their one drained element
	maxEarly, minLate    int64          // the largest early gap and the smallest late gap, ms; -1 where none
	paths, pathsKept     int
	unknown, unknownKept int
	states               map[string]int
}

// rPreflightCount reads each task-preflight note on the record its fire
// read (tFireOf), hands the frame reader what that fire's hook received
// (rHookInput), and counts the notes whose model lines list only paths (no
// rule, gate or evidence line: a flag, which reaches the person and not the
// agent), with how many of those carry an unknowns line. A note with no
// record is listed under its miss reason; a note with a record and no hook
// input stays under its kind; neither is in a rule column, and both are in
// miss.
func rPreflightCount(ls []tLine, notes []rNote) rPre {
	p := rPre{by: map[string]*rCnt{}, miss: map[string]int{}, maxEarly: -1, minLate: -1, states: map[string]int{}}
	for _, n := range notes {
		if n.Type != "steer" || n.Pattern != "task-preflight" {
			continue
		}
		f := tFireOf(ls, n.PromptID, n.AtMS)
		switch {
		case f.miss == tMissNoPrompt:
		case f.late:
			p.late++
			if f.miss == "" {
				p.lateOne++
			}
			if f.gap >= 0 && (p.minLate < 0 || f.gap < p.minLate) {
				p.minLate = f.gap
			}
		default:
			p.early++
			if f.gap > p.maxEarly {
				p.maxEarly = f.gap
			}
		}
		kind := f.kind
		if f.miss != "" {
			kind = f.miss
			p.miss[f.miss]++
		}
		if p.by[kind] == nil {
			p.by[kind] = &rCnt{}
		}
		p.by[kind].notes++
		known, framed := f.miss == "", false
		if known {
			in, cut, why := rHookInput(f.kind, f.text)
			if why != "" {
				p.miss[why]++
				known = false
			} else if framed, _ = harness.Framed(in, cut); framed {
				p.by[kind].framed++
			} else {
				p.by[kind].typed++
			}
		}
		if len(n.Slots.Rules.IDs) == 0 && len(n.Slots.Gates.Gates) == 0 && len(n.Slots.Evidence) == 0 {
			p.paths++
			p.states[n.State]++
			u := len(n.Slots.Unknown) > 0
			if u {
				p.unknown++
			}
			if known && !framed {
				p.pathsKept++
				if u {
					p.unknownKept++
				}
			}
		}
	}
	return p
}

// rTakers counts the last-turn claim notes by the kind of the fire that
// took them, as the truth places it: typed (human), framed (a kind
// tFramedKinds names), or a miss (no record, or any other kind).
func rTakers(ls []tLine, notes []rNote) (typed, framed, miss int) {
	for _, n := range notes {
		if n.Type != "steer" || n.Pattern != "claim-vs-record" || n.Key != "last-turn" {
			continue
		}
		f := tFireOf(ls, n.PromptID, n.AtMS)
		switch {
		case f.miss != "":
			miss++
		case f.kind == "human":
			typed++
		case tFramedKinds[f.kind]:
			framed++
		default:
			miss++
		}
	}
	return typed, framed, miss
}

// rPreflight reports rPreflightCount: the notes by their record's origin
// kind against the frame reader, the early and late split with its gap
// cross-check, the misses by reason, and the paths-only notes.
func rPreflight(t *testing.T, ls []tLine, notes []rNote) rPre {
	t.Helper()
	kinds := tPromptKinds(ls)
	p := rPreflightCount(ls, notes)
	all := map[string]bool{}
	for k := range kinds {
		all[strings.TrimPrefix(k, "queued_command/")] = true
	}
	for k := range p.by {
		all[k] = true
	}
	names := make([]string, 0, len(all))
	for k := range all {
		names = append(names, k)
	}
	sort.Strings(names)
	t.Logf("PROMPTS BY ORIGIN KIND (truth: framed where the kind is peer, task-notification, queued_command, cron or auto-continuation; typed where human; any other kind is a miss and in neither). A note's kind is its fire's record's; the rule columns read what that fire's hook received.")
	t.Logf("%-18s | %-8s | %-8s | %-14s | %-17s | %s", "kind", "prompts", "queued", "preflight notes", "rule framed (cut)", "rule typed (kept)")
	for _, k := range names {
		c := p.by[k]
		if c == nil {
			c = &rCnt{}
		}
		tag := ""
		switch {
		case k == "human":
			tag = " (typed)"
		case tFramedKinds[k]:
			tag = " (framed)"
		default:
			tag = " (miss)"
		}
		t.Logf("%-18s | %-8d | %-8d | %-15d | %-17d | %d", k+tag, kinds[k], kinds["queued_command/"+k], c.notes, c.framed, c.typed)
	}
	t.Logf("fires: early %d (largest gap %d ms), late %d (smallest gap %d ms); the gaps are a cross-check, the rule is structural; late on one drained element %d",
		p.early, p.maxEarly, p.late, p.minLate, p.lateOne)
	var mk []string
	for _, k := range rKeys(p.miss) {
		mk = append(mk, k+" "+strconv.Itoa(p.miss[k]))
	}
	if mk == nil {
		mk = []string{"none"}
	}
	t.Logf("miss rows by reason: %s", strings.Join(mk, "; "))
	sk := make([]string, 0, len(p.states))
	for _, k := range rKeys(p.states) {
		sk = append(sk, k+" "+strconv.Itoa(p.states[k]))
	}
	t.Logf("paths-only notes %d (%s), with an unknowns line %d; of them on prompts the frame reader keeps %d, with an unknowns line %d",
		p.paths, strings.Join(sk, ", "), p.unknown, p.pathsKept, p.unknownKept)
	return p
}

// rLines decodes an ndjson file into one T per line.
func rLines[T any](t *testing.T, path string) []T {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out []T
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 0, 1<<20), 64<<20)
	for n := 1; sc.Scan(); n++ {
		var v T
		if err := json.Unmarshal(sc.Bytes(), &v); err != nil {
			t.Fatalf("%s line %d: %v", path, n, err)
		}
		out = append(out, v)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// TestTurnReplay_RecordedSession runs the replay over the session the
// directory replayEnv names. The transcript's prefix must hash as CUT.txt
// says, or the run fails; it never re-counts on a longer transcript.
func TestTurnReplay_RecordedSession(t *testing.T) {
	dir := os.Getenv(replayEnv)
	if dir == "" {
		t.Skip(replayEnv + " names no replay directory")
	}
	cut := map[string]string{}
	b, err := os.ReadFile(filepath.Join(dir, "CUT.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if k, v, ok := strings.Cut(l, " "); ok {
			cut[k] = v
		}
	}
	lines, err := strconv.Atoi(cut["lines"])
	if err != nil || cut["session"] == "" || cut["transcript"] == "" || len(cut["sha256"]) != 64 {
		t.Fatalf("CUT.txt names no session, transcript, line count or sha256: %q", cut)
	}
	f, err := os.Open(cut["transcript"])
	if err != nil {
		t.Fatal(err)
	}
	prefix, sum, err := tCut(f, lines)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	if sum != cut["sha256"] {
		t.Fatalf("the transcript's first %d lines hash %s; the cut says %s", lines, sum, cut["sha256"])
	}
	t.Logf("cut: %d transcript lines, sha256 %s (equal to CUT.txt)", lines, sum)
	ls, err := tRead(bytes.NewReader(prefix))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	rec, err := os.ReadFile(filepath.Join(dir, store.FileRecords))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(st.RunDir(cut["session"]), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(st.RunDir(cut["session"]), store.FileRecords), rec, 0o600); err != nil {
		t.Fatal(err)
	}
	run, err := st.ReadRunConsistent(cut["session"], time.Second)
	if err != nil {
		t.Fatal(err)
	}
	vs := rLines[rVerify](t, filepath.Join(dir, store.FileRecords))
	notes := rLines[rNote](t, filepath.Join(dir, "steer.ndjson"))
	t.Logf("records: %d declarations, %d skipped, %d ignored; steer lines %d", len(run.Declarations), run.Skipped, run.Ignored, len(notes))
	rows := rReplay(t, ls, run, vs, notes)
	tb := rTabulate(rows)
	rReport(t, len(rows), tb)
	if len(rows) > 0 && tb.unplaced+tb.mismatch == len(rows) {
		t.Errorf("none of the %d last-turn notes was scored: the replay measured nothing", len(rows))
	}
	typed, framed, miss := rTakers(ls, notes)
	t.Logf("last-turn claim notes by the kind of the fire that took them: typed %d, framed %d, miss %d", typed, framed, miss)
	p := rPreflight(t, ls, notes)
	got := [5]int{p.early, p.late, p.lateOne, p.miss[tMissDrains], p.miss[tMissNoDrain]}
	if want, ok := rPinned[sum]; !ok {
		t.Logf("no split pinned for this cut")
	} else if got != want {
		t.Errorf("the pinned split (early, late, late on one element, on two or more, on none) is %v; want %v", got, want)
	}
}

// rPinned is the task-preflight split a cut must give, keyed by the cut's
// sha256: fires early, late, late on one drained element, on two or more,
// and on none. It holds the one cut it was counted on, by a script that
// shares no code with this file.
var rPinned = map[string][5]int{
	"32e04eb5f2474a5a2e24d433c35e8742b48f202fa41599f8f507c3e1abe22e94": {33, 17, 13, 4, 0},
}
