package spend

import (
	"encoding/json"
	"math"
	"path/filepath"
	"slices"
	"sort"

	"github.com/altrace-dev-role/rashomon/internal/report"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// What the silent-failure line was built from.
const (
	// StoreNotConsulted: Join was never called. Only a caller that skipped
	// it can produce this, and the renderer then says nothing about the line
	// rather than a zero.
	StoreNotConsulted = "not_consulted"
	// StoreNone: there is no rashomon store on this machine, so no transcript
	// is covered and the line is unknown, not zero.
	StoreNone = "none"
	// StoreRead: a store was read.
	StoreRead = "read"
)

// TurnBound states how a turn's spend is found, wherever its number is, with
// every rule the readers apply. The unkeyed user lines that keep a tie: in
// the main transcript a tool result or an injected meta line
// (report.FinalAssistantTexts), in a subagent transcript only a meta line
// (readFile). A subagent's sidechain line in the main transcript ties its
// response to the prompt before it and, a user line included, never ends the
// tie. A line that cannot be decoded ends it: any such line in a subagent
// transcript (readFile), and in the main transcript, unless it is a sidechain
// line (report.DecodeHeader); a blank line is skipped. So only a response
// after another unkeyed user line or an undecodable line is left out -- and
// the bound says no more is left out than that.
const TurnBound = "a turn's spend is every response its main transcript, or a subagent transcript under it, ties to its prompt by the promptId on the user line before it; a subagent's sidechain response in the main transcript counts toward the prompt before it, and a sidechain user line does not end the tie; a response after a user line with no promptId that is not a meta line or, in the main transcript, a tool result, or after a line that cannot be decoded (any in a subagent transcript; in the main transcript, unless it is a sidechain line), is tied to no turn and not counted, so this is a floor"

// Per-session coverage, in per_session[].coverage once Join has run.
const (
	CoverageRecorded    = "recorded"
	CoveragePartly      = "partly_recorded"
	CoverageNotRecorded = "not_recorded"
)

// SilentFailureTurns is the line only rashomon can draw: spend inside turns
// whose recorded calls failed while the turn's final message mentioned no
// failure at all.
//
// It covers only TRANSCRIPTS rashomon recorded -- a main transcript some
// record's transcript_path names, with the subagent transcripts under it.
// The rest are COUNTED, their sessions NAMED, and their spend shown as not
// covered -- never folded in as zero, because a conversation nobody recorded
// is one nobody checked, and zero would read as "checked and clean".
//
// Per transcript, not per session id, because a session id is not a
// conversation. Measured on a real machine: the store held a run directory
// for every session id in the window, yet every record in it came from a
// headless test project; the interactive transcript that carried most of the
// spend was never recorded, and a per-session-id rule printed "8 of 8
// sessions were recorded".
type SilentFailureTurns struct {
	Store                 string `json:"store"`
	Transcripts           int    `json:"transcripts"`
	CoveredTranscripts    int    `json:"covered_transcripts"`
	NotCoveredTranscripts int    `json:"not_covered_transcripts"`
	NotCoveredCost        Cost   `json:"not_covered_cost"`
	// NotCoveredSessions names each session with a transcript that is not
	// covered, as displaySession prints it.
	NotCoveredSessions []string `json:"not_covered_sessions"`
	Turns              int      `json:"turns"`
	// Unjudged counts the recorded turns with a failed call that took no
	// verdict: no final message could be tied to their prompt (records that
	// name no discovered transcript, a transcript with no promptId on the
	// turn's lines, a prompt with no promptId after it that ends attribution,
	// a file not readable to the end). The rule cannot fire on no words, so
	// such a turn is neither in Turns nor a checked clean one, and folding it
	// into "none found" would claim a check that never happened.
	Unjudged int `json:"unjudged_turns"`
	// UndeclaredFailedCalls counts the failed calls recorded in the window
	// whose declaration was lost (turnsOf). No turn is known to hold them,
	// so none was checked, and like Unjudged they keep "none found" from
	// claiming every failure was checked.
	UndeclaredFailedCalls int    `json:"undeclared_failed_calls"`
	Cost                  Cost   `json:"cost"`
	Bound                 string `json:"bound"`
}

// MarshalJSON writes turns, unjudged_turns, undeclared_failed_calls and cost
// as null when no transcript is covered --
// no store, a store that recorded none of these transcripts, or a Join that
// never ran. Nothing was checked then, and {"turns": 0, "cost": {"usd": 0}}
// would tell a JSON consumer "checked, and clean": the "$0 for unknown" the
// text rendering refuses by saying "unknown".
func (j SilentFailureTurns) MarshalJSON() ([]byte, error) {
	type plain SilentFailureTurns
	out := struct {
		plain
		Turns      *int  `json:"turns"`
		Unjudged   *int  `json:"unjudged_turns"`
		Undeclared *int  `json:"undeclared_failed_calls"`
		Cost       *Cost `json:"cost"`
	}{plain: plain(j)}
	if j.NotCoveredSessions == nil {
		out.NotCoveredSessions = []string{}
	}
	if j.CoveredTranscripts > 0 {
		out.Turns, out.Unjudged, out.Undeclared, out.Cost = &j.Turns, &j.Unjudged, &j.UndeclaredFailedCalls, &j.Cost
	}
	return json.Marshal(out)
}

// turn is one prompt_id's records: the small run BuildSilentFailures takes,
// its span in recorded time, and the main transcripts its records name.
type turn struct {
	prompt  string
	run     *store.Run
	firstMS int64
	lastMS  int64
	// transcripts are the transcript_path values of the turn's MAIN-agent
	// declarations. They are only ever compared against paths this package
	// discovered itself; see Join.
	transcripts map[string]bool
}

// turnsOf groups a run by prompt_id, the key digest groups a turn by -- and
// ONLY prompt_id, for digest's reason: a subagent's calls carry the parent's
// prompt_id, so they belong to the turn that spawned them. Executions and
// terminals carry no prompt_id and join through the tool_use_id of the
// declaration they answer, which is how digest builds the same small run.
// A declaration with no prompt_id (a record older than the field) belongs to
// no turn and is left out rather than guessed into one.
//
// A FAILED EXECUTION WHOSE DECLARATION WAS LOST is in no turn. A
// declaration can be lost -- a lock timeout, a paused pre hook, a
// mid-session install -- while its execution is recorded. The execution
// then has no declaration to join through, and so no prompt: placing it by
// recorded time put it in whichever turn started before it, often a clean
// one, whose spend was then printed as silent-failure spend. The session
// report, which judges the whole run, counts such a failure; a turn's digest
// leaves it out, as this does. It is placed nowhere, and its recorded time is
// returned in lost, for Join to count once as a failed call that was not
// checked.
func turnsOf(run *store.Run) (turns []turn, lost []int64) {
	byPrompt := map[string]*turn{}
	owner := map[string]*turn{}
	declared := map[string]bool{}
	span := func(t *turn, ms int64) {
		if ms < t.firstMS {
			t.firstMS = ms
		}
		if ms > t.lastMS {
			t.lastMS = ms
		}
	}
	for _, d := range run.Declarations {
		declared[d.ToolUseID] = true
		if d.PromptID == nil || *d.PromptID == "" {
			continue
		}
		t, ok := byPrompt[*d.PromptID]
		if !ok {
			t = &turn{prompt: *d.PromptID, run: &store.Run{}, firstMS: math.MaxInt64, lastMS: math.MinInt64, transcripts: map[string]bool{}}
			byPrompt[*d.PromptID] = t
		}
		if (d.AgentID == nil || *d.AgentID == "") && d.TranscriptPath != "" {
			t.transcripts[filepath.Clean(d.TranscriptPath)] = true
		}
		t.run.Declarations = append(t.run.Declarations, d)
		owner[d.ToolUseID] = t
		span(t, d.RecordedAtMS)
	}
	for _, x := range run.Executions {
		t, ok := owner[x.ToolUseID]
		if !ok {
			if !declared[x.ToolUseID] && x.Outcome == store.ExecFailed {
				lost = append(lost, x.RecordedAtMS)
			}
			continue
		}
		t.run.Executions = append(t.run.Executions, x)
		span(t, x.RecordedAtMS)
	}
	for _, x := range run.Terminals {
		if t, ok := owner[x.ToolUseID]; ok {
			span(t, x.RecordedAtMS)
		}
	}
	turns = make([]turn, 0, len(byPrompt))
	for _, t := range byPrompt {
		turns = append(turns, *t)
	}
	sort.Slice(turns, func(i, j int) bool { return turns[i].firstMS < turns[j].firstMS })
	return turns, lost
}

// Join fills the silent-failure line from a store, or records that there is
// none when st is nil.
//
// st MUST come from store.OpenExisting (or be nil for ErrNoStore), never
// store.Open: spend is a question, and asking it must not mint an install
// identity -- H-87's rule, which every read-only command here follows.
//
// COVERAGE IS PER TRANSCRIPT. A windowed response's transcripts are the main
// transcripts (TranscriptFile.Main) of every file it was seen in. A
// transcript is covered when a record of one of the window's sessions
// carries a transcript_path naming it, or naming a subagent transcript under
// it; every other transcript is not covered, however many run directories
// the store holds for its session id. A response is not covered when ANY
// transcript holding it is not, and its cost is counted there once: a
// resumed conversation carries the original's responses into a second file,
// and pinning each response to whichever file sorted first reported an
// unrecorded original as recorded in one path order and not in the other.
// Claude Code makes such copies on /branch and --fork-session, which copy a
// transcript under a new session id.
//
// Per recorded turn: the turn's own small run goes through
// report.BuildSilentFailures -- the very rule, and the very run shape, a
// turn's digest applies at Stop. The rule needs the turn's final message, and
// a turn with no recorded failure cannot fire whatever that message says, so
// transcripts are read only for turns with at least one, and each once for
// all of them (report.FinalAssistantTexts). Only the verdict is kept. A turn
// with a failed call whose final message cannot be found takes no verdict,
// and is counted in Unjudged rather than read as clean. A failed call whose
// declaration was lost is in no turn (turnsOf) and is never placed in one by
// time: it is counted, once, in UndeclaredFailedCalls, and "none found" is
// then said only of the turns that could be checked.
//
// A FIRING TURN'S SPEND IS KEYED BY ITS PROMPT, not by a span of recorded
// time. The main transcript ties each response to the promptId of the user
// line before it -- the prompt_id the hooks record -- so the turn's main-agent
// spend is exactly the responses tied to its prompt, including the one that
// made its first call and the final reply, which a record span left out. A
// subagent transcript ties its responses the same way, by the promptId on its
// own user lines (Read keeps it per response): the record cannot, since the
// hooks name the main transcript for a subagent's calls too. A span of time
// could also swallow a later turn's responses (a call whose execution was
// recorded after the next prompt began); a prompt key cannot.
//
// ONE SESSION ID CAN NAME TWO CONVERSATIONS. Measured on this machine: a
// headless run started with --session-id reusing an interactive session's id
// wrote a second main transcript under another project directory. The hooks
// recorded which file each call belonged to (transcript_path), so a turn is
// read from, and priced in, the discovered main transcripts its records name
// and nothing else. A recorded path this package did not discover itself is
// never opened -- the read stays inside Claude Code's configuration directory
// -- and a turn whose records name no discovered transcript takes no verdict.
func (s *Summary) Join(st *store.Store) error {
	j := &s.SilentFailureTurns
	*j = SilentFailureTurns{Store: StoreNone, Bound: TurnBound, NotCoveredSessions: []string{}}
	defer s.buildSavings()

	// A pre-output refusal is in no figure, but its transcript is one of the
	// window's, and its session has a row to label.
	byTranscript := map[string][]*Response{}
	sessions := map[string]bool{}
	for _, rs := range [][]*Response{s.window, s.refused} {
		for _, r := range rs {
			for _, m := range s.mainsOf(r) {
				byTranscript[m] = append(byTranscript[m], r)
			}
			for _, f := range r.files {
				sessions[f.session] = true
			}
		}
	}
	j.Transcripts = len(byTranscript)

	covered := map[string]bool{}
	counted := map[*Response]bool{}
	defer func() { s.markCoverage(byTranscript, covered, counted) }()
	if st == nil {
		return nil
	}
	j.Store = StoreRead

	dirs, err := st.Runs()
	if err != nil {
		return err
	}
	recorded := map[string]bool{}
	for _, d := range dirs {
		recorded[d] = true
	}
	ids := make([]string, 0, len(sessions))
	for id := range sessions {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	known := newKnownPaths(s.scan.Files)

	var turns []turn
	want := map[string]bool{}
	for _, id := range ids {
		if !recorded[st.DirName(id)] {
			continue
		}
		run, err := st.ReadRun(id)
		if err != nil {
			return err
		}
		for _, d := range run.Declarations {
			i, ok := known.lookup(d.TranscriptPath)
			if !ok {
				continue
			}
			covered[s.scan.Files[i].Main] = true
		}
		// Only a turn with a recorded failure can fire, whatever its final
		// message says; those alone need their transcripts read. A failed
		// call whose declaration was lost is in no turn: it is counted once,
		// as not checked.
		recordedTurns, lost := turnsOf(run)
		for _, ms := range lost {
			if ms >= s.FromUnixMS {
				j.UndeclaredFailedCalls++
			}
		}
		for _, t := range recordedTurns {
			if t.lastMS < s.FromUnixMS {
				continue
			}
			if report.BuildSilentFailures(t.run, report.AccountFromMessage("")).Failed == 0 {
				continue
			}
			turns = append(turns, t)
			want[t.prompt] = true
		}
	}

	finals := map[string]map[string]report.TurnFinal{}
	for _, t := range turns {
		mains := s.namedMains(t.transcripts, known)
		if len(mains) == 0 {
			j.Unjudged++
			continue
		}
		byFile := make([]map[string]report.TurnFinal, len(mains))
		for k, p := range mains {
			if _, ok := finals[p]; !ok {
				finals[p] = report.FinalAssistantTexts(p, want)
			}
			byFile[k] = finals[p]
		}
		sf := report.BuildSilentFailures(t.run, report.AccountFromMessage(lastSaid(byFile, t.prompt)))
		if !sf.FinalMessageAvailable {
			j.Unjudged++
			continue
		}
		if !sf.Fires {
			continue
		}
		j.Turns++
		for _, r := range s.window {
			if counted[r] || !s.inTurn(r, t.prompt, mains, byFile) {
				continue
			}
			counted[r] = true
			costOf(&j.Cost, r)
		}
	}
	return nil
}

// markCoverage counts covered and not-covered transcripts, prices and names
// the not-covered ones, and marks each per_session row with its coverage.
// Deferred by Join so every exit, a nil store's included, fills the same
// fields the same way.
//
// Each transcript is tallied and named by its own session
// (TranscriptFile.Session), never by the session of whichever response it
// holds was seen first: a branched copy carrying the original's responses
// named the recorded copy as not covered in one path order. A row is tallied
// by its dollars too: each response it holds the cost of counts as covered,
// or as not when its cost is in NotCoveredCost. So a session with any
// transcript, or any dollar, not covered is not "recorded" -- an unrecorded
// copy is never reported as recorded, nor is a row holding not-covered
// dollars -- and a row whose session names no discovered file (a copy whose
// lines keep an absent original's id) still has a label.
func (s *Summary) markCoverage(byTranscript map[string][]*Response, covered map[string]bool, counted map[*Response]bool) {
	j := &s.SilentFailureTurns
	sessionOf := map[string]string{}
	for _, f := range s.scan.Files {
		sessionOf[f.Main] = f.Session
	}
	type tally struct{ in, out int }
	bySession := map[string]*tally{}
	tallyOf := func(id string) *tally {
		t := bySession[id]
		if t == nil {
			t = &tally{}
			bySession[id] = t
		}
		return t
	}
	named := map[string]bool{}
	for m := range byTranscript {
		id := sessionOf[m]
		t := tallyOf(id)
		if covered[m] {
			j.CoveredTranscripts++
			t.in++
		} else {
			j.NotCoveredTranscripts++
			t.out++
			named[displaySession(id)] = true
		}
	}
	// Once per response, however many transcripts hold it: not covered when
	// any of them is not -- unless a covered turn already counted it into the
	// figure, where printing it as not covered too put the same dollars on
	// both sides of the line.
	for _, r := range s.window {
		notCovered := false
		if !counted[r] {
			for _, m := range s.mainsOf(r) {
				if !covered[m] {
					notCovered = true
					break
				}
			}
		}
		if notCovered {
			costOf(&j.NotCoveredCost, r)
		}
		for _, id := range r.owners {
			if t := tallyOf(id); notCovered {
				t.out++
			} else {
				t.in++
			}
		}
	}
	for name := range named {
		j.NotCoveredSessions = append(j.NotCoveredSessions, name)
	}
	sort.Strings(j.NotCoveredSessions)
	for i := range s.PerSession {
		t := bySession[s.PerSession[i].id]
		switch {
		case t == nil:
		case t.out == 0:
			s.PerSession[i].Coverage = CoverageRecorded
		case t.in == 0:
			s.PerSession[i].Coverage = CoverageNotRecorded
		default:
			s.PerSession[i].Coverage = CoveragePartly
		}
	}
}

// mainsOf is the distinct main transcripts of every file a response was seen
// in, first sighting first.
func (s *Summary) mainsOf(r *Response) []string {
	var out []string
	for _, f := range r.files {
		if m := s.scan.Files[f.idx].Main; !slices.Contains(out, m) {
			out = append(out, m)
		}
	}
	return out
}

// knownPaths is the files Discover found, indexed so a recorded
// transcript_path can be matched against them -- and ONLY against them: a
// recorded path is looked up here and never opened.
//
// A path can be spelled two ways that name one file: through a symlinked
// ancestor, or resolved. Each discovered file is indexed by its absolute
// spelling and by the resolution of that absolute spelling -- resolving a
// relative one (a relative CLAUDE_CONFIG_DIR) gave a relative spelling no
// recorded path matched -- and a recorded path is tried as written and then
// resolved. Both sides need it. On a default macOS machine TMPDIR is
// reached through the /var -> /private/var link, so a projects folder linked
// in from there is discovered as <config>/projects/<link>/x.jsonl (resolving
// to /private/var/...) while the hooks may record /var/.../x.jsonl: neither
// spelling matched the other, and the transcript read "not covered", its
// failed turns unjudged. Resolving the recorded path stats its components; it
// does not open the file.
type knownPaths struct {
	index    map[string]int
	resolved map[string]string // recorded path -> its resolved spelling, or "" when it does not resolve
}

func newKnownPaths(files []TranscriptFile) *knownPaths {
	k := &knownPaths{index: map[string]int{}, resolved: map[string]string{}}
	add := func(p string, i int) {
		if _, ok := k.index[p]; !ok {
			k.index[p] = i
		}
	}
	for i, f := range files {
		p := filepath.Clean(f.Path)
		if abs, err := filepath.Abs(f.Path); err == nil {
			p = abs
		}
		add(p, i)
		if real, err := filepath.EvalSymlinks(p); err == nil {
			add(real, i)
		}
	}
	return k
}

// lookup finds the discovered file a recorded transcript_path names, as
// written or once resolved.
func (k *knownPaths) lookup(recorded string) (int, bool) {
	if recorded == "" {
		return 0, false
	}
	if i, ok := k.index[filepath.Clean(recorded)]; ok {
		return i, true
	}
	real, seen := k.resolved[recorded]
	if !seen {
		if r, err := filepath.EvalSymlinks(recorded); err == nil {
			real = r
		}
		k.resolved[recorded] = real
	}
	if real == "" {
		return 0, false
	}
	i, ok := k.index[real]
	return i, ok
}

// namedMains is the discovered main transcripts among a turn's recorded
// transcript_paths, sorted.
func (s *Summary) namedMains(recorded map[string]bool, known *knownPaths) []string {
	var out []string
	for p := range recorded {
		if i, ok := known.lookup(p); ok && !s.scan.Files[i].Subagent {
			out = append(out, s.scan.Files[i].Path)
		}
	}
	sort.Strings(out)
	return out
}

// inTurn reports whether a response is the turn's spend: a main-agent
// response one of the turn's main transcripts ties to its prompt, or a
// response a subagent file under one of them ties to that prompt.
func (s *Summary) inTurn(r *Response, prompt string, mains []string, byFile []map[string]report.TurnFinal) bool {
	if f := s.scan.Files[r.file]; f.Subagent {
		return r.prompt == prompt && slices.Contains(mains, f.Main)
	}
	for _, f := range byFile {
		if f[prompt].Responses[r.ID] {
			return true
		}
	}
	return false
}

// lastSaid is a turn's last assistant text across its main transcripts, ""
// when none of them attributes any text to its prompt. A session id can own
// more than one main file (a resumed or re-run session written under a second
// project directory), so the latest across all of them is the turn's last
// word.
func lastSaid(byFile []map[string]report.TurnFinal, prompt string) string {
	var best string
	var bestMS int64 = math.MinInt64
	for _, f := range byFile {
		if t, ok := f[prompt]; ok && t.Said && t.AtMS >= bestMS {
			best, bestMS = t.Text, t.AtMS
		}
	}
	return best
}
