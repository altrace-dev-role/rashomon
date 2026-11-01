package recap

import (
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/altrace-dev-role/rashomon/internal/jsonscan"
	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// The end-of-turn value line (tech 2.2.11; card 2.4 R6, R8) states what the
// record shows about the turn's checks after its last edit: one sentence per
// row of verify's table, an optional AutoSteer clause counting the session's
// notes, findings and checks (clause.go), and the report pointer recap.Line
// uses. Every
// slot is a count, a call number or a gate's program word from the record;
// the templates hold none of FUN-15's words (TestFUN15b scans this file), so
// the sentences are built by concatenation, with no format verbs.

// lineColumns is the width each line of the value line is fitted to.
const lineColumns = 80

// maxIDName is the longest spec gate id the line names: with it the verified
// sentence, the one the id lengthens most, still fits lineColumns.
const maxIDName = 24

// specGatePrefix leads the id of a gate a trusted spec declares.
const specGatePrefix = "spec:"

// maxGateNames is how many programs a gate list names before `and <k> more`.
const maxGateNames = 3

// titles names each pattern a delivered note can carry in the clause (tech
// 2.2.11 Titles). integrity and model-switch are never delivered, and
// sequence-risk is reserved; none has a title. learned-fact reads "lesson",
// the word the learn verb and the consent screen use, where tech 2.2.11
// reads "known fix" (FD-10, signed by the owner 10-29, D175).
func titles() map[string]string {
	titlesOnce.Do(func() {
		titlesOnce.v = map[string]string{
			string(pattern.SubtaskGate):        "task check",
			string(pattern.RegressionLocation): "regression location",
			string(pattern.ContinuityBriefing): "briefing",
			string(pattern.TaskPreflight):      "preflight",
			string(pattern.DeclaredFact):       "known fix",
			string(pattern.LearnedFact):        "lesson",
			string(pattern.PlatformRule):       "platform rule",
			string(pattern.IdenticalRetry):     "repeat-call note",
			string(pattern.CredSpray):          "credentials note",
			string(pattern.OrderingRule):       "ordering rule",
			string(pattern.AgentInterference):  "interference note",
			string(pattern.SubagentClaim):      "subagent check",
			string(pattern.ClaimVsRecord):      "record check",
			string(pattern.MaskedStatus):       "hidden-status note",
			string(pattern.StaleEvidence):      "stale-gate note",
		}
	})
	return titlesOnce.v
}

// titlesOnce holds what titles returns, built on its first call and never at
// init (RES-2).
var titlesOnce struct {
	sync.Once
	v map[string]string
}

// whens is the clause's <when> by the delivering line's event (D78, D87):
// v0 delivers a PostToolUse note at PostToolUse itself (D54), so both
// PostToolUse and PostToolBatch read "after the call". SessionStart's
// compact and resume kinds are on no line, and the commit, push, pull-request
// and task phrases have no event, so those events render no <when>.
func whens() map[string]string {
	whensOnce.Do(func() {
		whensOnce.v = map[string]string{
			string(steer.EventPostToolUseFailure): "at the failure",
			string(steer.EventUserPromptSubmit):   "with your prompt",
			string(steer.EventSubagentStart):      "at the subagent start",
			string(steer.EventPostToolUse):        "after the call",
			string(steer.EventPostToolBatch):      "after the call",
		}
	})
	return whensOnce.v
}

// whensOnce holds what whens returns, built on its first call and never at
// init (RES-2).
var whensOnce struct {
	sync.Once
	v map[string]string
}

// readFromOutput closes a sentence about a run whose outcome was read from
// its output, because a pipe hid the exit status (D229): the provenance word,
// in plain words, so the line never reads as an exit status the record holds.
const readFromOutput = " (read from its output; the pipe hid the exit status)"

// prefixWords are the words that lead a shell line before its runner, as in
// `set -o pipefail; make test`: a gate run whose program word is one names
// no runner. From call schema 8 the record holds the runner's row, which the
// Stop view names (verify.StopView.Programs), so the sentence reaches for the
// gate's kind (D94) only on an older record, or a run with no runner row. A
// slice of constants, with no code run at init (RES-2).
var prefixWords = []string{"set", "export", "source", "eval", "exec", "env", "time"}

// Note is one note delivered to the agent in the prompt: its pattern, the
// event that delivered it, and the text the model received.
type Note struct {
	Pattern   string
	Event     string
	ModelText []byte
}

// noteKeys is the one KeySet a steer line is read with (RES-5's pattern: no
// decode of a whole line).
func noteKeys() *jsonscan.KeySet {
	noteKeysOnce.Do(func() {
		noteKeysOnce.v = jsonscan.NewKeySet("state", "prompt_id", "event", "pattern", "model_text")
	})
	return noteKeysOnce.v
}

// noteKeysOnce holds what noteKeys returns, built on its first call and never
// at init (RES-2).
var noteKeysOnce struct {
	sync.Once
	v *jsonscan.KeySet
}

const (
	nkState = iota
	nkPrompt
	nkEvent
	nkPattern
	nkModelText
	nkCount
)

// DeliveredNotes is the prompt's delivered notes among a run's steer lines:
// state delivered, prompt_id the prompt's, and a model text, which a flag
// lacks. A line that does not read is passed over; no prompt, no note.
func DeliveredNotes(lines [][]byte, promptID string) []Note {
	if promptID == "" {
		return nil
	}
	var notes []Note
	var out [nkCount]jsonscan.Span
	for _, b := range lines {
		if jsonscan.Top(b, noteKeys(), out[:]) != nil {
			continue
		}
		if text(b, out[nkState]) != string(steer.LineStateDelivered) || text(b, out[nkPrompt]) != promptID {
			continue
		}
		model := text(b, out[nkModelText])
		if model == "" {
			continue
		}
		notes = append(notes, Note{Pattern: text(b, out[nkPattern]), Event: text(b, out[nkEvent]), ModelText: []byte(model)})
	}
	return notes
}

// text is a string value's text, "" for any other kind or a bad escape.
func text(b []byte, sp jsonscan.Span) string {
	if sp.Kind != jsonscan.String {
		return ""
	}
	s, err := jsonscan.DecodeString(b, sp)
	if err != nil {
		return ""
	}
	return s
}

// ValueLine renders the turn's value line from the Stop view, or "" for
// RowNone, which prints nothing. Its state line names fewer gates while it is
// over lineColumns and more than one is named; past that it prints whole
// (D86). The clause fits the line by dropping detail (clause.go).
func ValueLine(v verify.StopView, im Impact, sessionID string, fromPlugin bool) string {
	if v.Summary.Row == verify.RowNone {
		return ""
	}
	sentence := fitted(v, prefix)
	if sentence == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString(prefix + sentence + ".\n")
	if c := clause(im); c != "" {
		b.WriteString(continuation() + c + "\n")
	}
	b.WriteString(pointer(sessionID, fromPlugin))
	return b.String()
}

// fitted is the row's sentence with as many gate names as fit after lead on
// one line, down to one; past that, with a gate whose run had other arguments
// named as name() names a gate without its id (maxIDName fits "passed", not
// "ran with other arguments and passed"; GATE-BIND1, D-c).
func fitted(v verify.StopView, lead string) string {
	s := sentence(v, maxGateNames, false)
	for k := maxGateNames - 1; k >= 1 && columns(lead+s+".") > lineColumns; k-- {
		s = sentence(v, k, false)
	}
	if columns(lead+s+".") > lineColumns {
		if short := sentence(v, 1, true); columns(short) < columns(s) {
			s = short
		}
	}
	return s
}

func columns(s string) int { return len([]rune(s)) }

// sentence is the row's sentence (tech 2.2.11's table, first match won in
// TurnSummary), naming at most k programs in a gate list, with "; advisory
// only" when the row was read from advisory gates; short as gateSet's.
func sentence(v verify.StopView, k int, short bool) string {
	s := rowSentence(v, k, short)
	if s != "" && v.Summary.Advisory() {
		s += "; advisory only"
	}
	return s
}

// rowSentence fills the row's template. <gate> is the first turn gate in the
// row's state, <seq> its last run; <gates> are the green turn gates'
// programs, and their <seq> the latest of their runs (D5, D6).
func rowSentence(v verify.StopView, k int, short bool) string {
	t := turnGates(v)
	t.short = short
	wrote := v.Turn.Writes > 0
	files := filesEdited(v.Turn.PathsWritten)
	switch v.Summary.Row {
	case verify.RowUnknown:
		if v.Snap.Recording != verify.Complete {
			return v.Summary.Sentence
		}
		g := t.first(verify.Unknown)
		if g == nil {
			return ""
		}
		if g.Reason == verify.OrderUnknown {
			return "the record does not order every " + t.name(g) + " run and edit in this turn; verification state unknown"
		}
		where, _ := when(v, g)
		return t.name(g) + " ran" + where + " (call " + seqOf(g) + "); its result is not in the record"
	case verify.RowRed:
		g := t.first(verify.Red)
		if g == nil {
			return ""
		}
		verb := "failed"
		if g.LastRun != nil && g.LastRun.ExitCode != nil {
			verb = "exited " + strconv.Itoa(*g.LastRun.ExitCode)
		}
		if g.LastRun != nil && g.LastRun.OutcomeFrom == verify.SourceOutput {
			verb = "failed"
		}
		where, since := when(v, g)
		s := t.name(g) + " " + verb + where + " (call " + seqOf(g) + ")" + fromOutput(g.LastRun)
		if since {
			s += " and has not run since"
		}
		if wrote {
			s = files + "; " + s
		}
		return s
	case verify.RowMasked:
		if g := t.first(verify.MaskedInconclusive); g != nil {
			where, _ := when(v, g)
			if stage := droppingStage(g.LastRun); stage != "" {
				return t.name(g) + " ran" + where + ", but " + stage + " hid its result (call " + seqOf(g) + ")"
			}
			return t.name(g) + " ran" + where + "; its exit status was hidden by its line (call " + seqOf(g) + ")"
		}
	case verify.RowBackgrounded:
		if g := t.first(verify.Backgrounded); g != nil {
			where, _ := when(v, g)
			return t.name(g) + " moved to the background" + where + "; its result is not in the record"
		}
	case verify.RowRanOnly:
		greens, _ := t.passed(k)
		return "no edits; " + greens + " (call " + t.greenSeq() + ")" + t.fromOutput()
	case verify.RowUnverified:
		return t.unverified(v, k, files)
	case verify.RowNoGate:
		if v.Index != nil && len(v.Index.Entries) > 0 {
			return files + "; no known test or build applies to these files"
		}
		return files + "; no test or build is known for this project yet"
	case verify.RowUnnamedWrite:
		greens, _ := t.passed(k)
		return files + "; " + greens + " (call " + t.greenSeq() + ")" + t.fromOutput() + "; call " +
			strconv.FormatInt(v.Turn.UnnamedWriteSeq, 10) + " may have written files after that"
	case verify.RowVerified:
		// The row is read only where the green runs follow the last edit, so
		// "verified" says it; a run with other arguments takes the columns
		// "after the last edit" held (GATE-BIND1, D-c).
		greens, other := t.passed(k)
		if !other {
			greens += " after the last edit"
		}
		return "verified - " + greens + t.fromOutput()
	case verify.RowFindings:
		greens, _ := t.passed(k)
		return files + "; " + greens + " after the last edit" + t.fromOutput() + "; " + v.Summary.Sentence
	}
	return ""
}

// unverified is the unverified row's sentence: a gate not run after the last
// edit beside the greens, or none run; with no write, a run that checked
// nothing or only part of the project; and, under the advisory cap, greens
// alone, which are not established gates (D38).
func (t gateSet) unverified(v verify.StopView, k int, files string) string {
	g := t.first(verify.Stale)
	if g == nil {
		g = t.first(verify.Absent)
	}
	if v.Turn.Writes == 0 && g != nil {
		where, _ := when(v, g)
		switch g.Reason {
		case verify.EmptyRun:
			return t.name(g) + " ran" + where + " and ran no checks (call " + seqOf(g) + ")"
		case verify.PartialOnly:
			return t.name(g) + " ran on part of the project" + where + " (call " + seqOf(g) + ")"
		}
	}
	greens, _ := t.passed(k)
	switch {
	case g == nil && greens != "":
		return files + "; " + greens + " after the last edit" + t.fromOutput()
	case g != nil && greens != "":
		return files + "; " + greens + " after the last edit" + t.fromOutput() + ", " + t.name(g) + " not run after it"
	}
	return files + ", no test or build run after the last edit"
}

// when places g's last run by seq (card 2.4r): after the turn's last edit, a
// tie too (writes count strictly after a run, gates.go), or before it; with no
// write, in this turn or, before its first seq, an earlier one; since if before.
func when(v verify.StopView, g *verify.GateState) (where string, since bool) {
	var seq int64
	if g.LastRun != nil {
		seq = g.LastRun.Seq
	}
	switch {
	case v.Turn.Writes > 0 && seq >= v.Turn.LastWriteSeq:
		return " after the last edit", false
	case v.Turn.Writes > 0:
		return " before the last edit", true
	case seq >= v.Turn.FirstSeq:
		return " in this turn", false
	}
	return " in an earlier turn", true
}

// gateSet is the turn gates of a view and the programs that name them.
// short names a gate whose last run had other arguments without its id
// (fitted's last step).
type gateSet struct {
	gates    []verify.GateState
	programs map[string]string
	short    bool
}

func turnGates(v verify.StopView) gateSet {
	advisory := verify.Advisory(v.Snap.Gates)
	t := gateSet{programs: v.Programs}
	for _, g := range v.Snap.Gates {
		if verify.TurnGate(g, advisory) {
			t.gates = append(t.gates, g)
		}
	}
	return t
}

// first is the first turn gate in state s, nil with none.
func (t gateSet) first(s verify.State) *verify.GateState {
	for i := range t.gates {
		if t.gates[i].State == s {
			return &t.gates[i]
		}
	}
	return nil
}

// name is the id of a gate a trusted spec declares, in the slot's quote;
// else g's program word, else its kind: with no program word, or with a
// prefix word in its place. With short, a gate whose last run had other
// arguments skips the id.
func (t gateSet) name(g *verify.GateState) string {
	if id, ok := strings.CutPrefix(g.Gate, specGatePrefix); ok && len(id) <= maxIDName && !(t.short && otherArgs(g)) {
		if p := slot.Program(id); p.Validate() == nil {
			return p.Text()
		}
	}
	if p := t.programs[g.Gate]; p != "" && !slices.Contains(prefixWords, p) {
		return p
	}
	return g.Kind
}

// fromOutput is the provenance clause where a green turn gate's last run had
// its outcome read from its output, else "". The sentence before it lists the
// green programs, so with every one of them read from output the clause stands
// alone (readFromOutput); with only some, it names them, and the others read
// as the exit status they had.
func (t gateSet) fromOutput() string {
	var read, all []string
	for i := range t.gates {
		g := &t.gates[i]
		if g.State != verify.Green {
			continue
		}
		n := t.name(g)
		if !contains(all, n) {
			all = append(all, n)
		}
		if g.LastRun != nil && g.LastRun.OutcomeFrom == verify.SourceOutput && !contains(read, n) {
			read = append(read, n)
		}
	}
	switch {
	case len(read) == 0:
		return ""
	case len(read) == len(all):
		return readFromOutput
	}
	return " (" + joinNames(read) + " " + strings.TrimPrefix(readFromOutput, " (")
}

// fromOutput is readFromOutput for a run that had its outcome from its output.
func fromOutput(r *verify.RunRef) string {
	if r != nil && r.OutcomeFrom == verify.SourceOutput {
		return readFromOutput
	}
	return ""
}

// droppingStage is the words that say which stage of a pipe hid a masked
// run's result, "the pipe to head" for a stage that drops the summary (head,
// grep, wc, sort, cut) and "a pipe" for one no list names; "" where the run
// names no stage (a record before schema 8, or a status hidden by something
// that is not a pipe) or the stage keeps the summary, whose output simply did
// not say.
func droppingStage(r *verify.RunRef) string {
	if r == nil || r.MaskedBy == 0 || shape.MaskedByKeeps(r.MaskedBy.String()) {
		return ""
	}
	if r.MaskedBy.String() == "other" {
		return "a pipe"
	}
	return "the pipe to " + r.MaskedBy.String()
}

// passed is the green turn gates as the sentence says they passed, in
// order: the distinct names of those whose last run was their own words, at
// most k named and then `and <n> more`, with "passed"; then those bound at
// their runner with other words (RunRef.OtherArgs) with "ran with other
// arguments and passed", each named and never folded into the count, so no
// line says the gate passed of a run that was not its own (GATE-BIND1, D-c).
// other reports a gate of the second kind; "" with no green gate.
func (t gateSet) passed(k int) (s string, other bool) {
	var own, others []string
	for i := range t.gates {
		g := &t.gates[i]
		if g.State != verify.Green {
			continue
		}
		switch n := t.name(g); {
		case otherArgs(g):
			if !contains(others, n) {
				others = append(others, n)
			}
		case !contains(own, n):
			own = append(own, n)
		}
	}
	var parts []string
	if k = max(k-len(others), 1); len(own) > k {
		parts = append(parts, strings.Join(own[:k], ", ")+" and "+strconv.Itoa(len(own)-k)+" more passed")
	} else if len(own) > 0 {
		parts = append(parts, joinNames(own)+" passed")
	}
	if len(others) > 0 {
		parts = append(parts, joinNames(others)+" ran with other arguments and passed")
	}
	return strings.Join(parts, ", "), len(others) > 0
}

// otherArgs reports g's last run as bound with other arguments.
func otherArgs(g *verify.GateState) bool {
	return g.LastRun != nil && g.LastRun.OtherArgs
}

// joinNames is names as a list in words: "a", "a and b", "a, b and c".
func joinNames(names []string) string {
	if len(names) == 1 {
		return names[0]
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

func contains(names []string, n string) bool {
	for _, m := range names {
		if m == n {
			return true
		}
	}
	return false
}

// greenSeq is the latest last run among the green turn gates.
func (t gateSet) greenSeq() string {
	var at int64
	for _, g := range t.gates {
		if g.State == verify.Green && g.LastRun != nil {
			at = max(at, g.LastRun.Seq)
		}
	}
	return strconv.FormatInt(at, 10)
}

func seqOf(g *verify.GateState) string {
	if g.LastRun == nil {
		return "0"
	}
	return strconv.FormatInt(g.LastRun.Seq, 10)
}

// filesEdited is the count slot with its noun in number (D88).
func filesEdited(n int) string {
	if n == 1 {
		return "1 file edited"
	}
	return strconv.Itoa(n) + " files edited"
}
