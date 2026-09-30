package spend

import (
	"encoding/json"
	"math"
	"path/filepath"
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

// TurnBound states how a turn's spend is found, wherever its number is.
const TurnBound = "a turn's spend is every response its main transcript ties to its prompt, and a subagent's when rashomon recorded a call the subagent made; a subagent with no recorded call is not counted, so this is a floor"

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
	Cost               Cost     `json:"cost"`
	Bound              string   `json:"bound"`
}

// MarshalJSON writes turns and cost as null when no transcript is covered --
// no store, a store that recorded none of these transcripts, or a Join that
// never ran. Nothing was checked then, and {"turns": 0, "cost": {"usd": 0}}
// would tell a JSON consumer "checked, and clean": the "$0 for unknown" the
// text rendering refuses by saying "unknown".
func (j SilentFailureTurns) MarshalJSON() ([]byte, error) {
	type plain SilentFailureTurns
	out := struct {
		plain
		Turns *int  `json:"turns"`
		Cost  *Cost `json:"cost"`
	}{plain: plain(j)}
	if j.NotCoveredSessions == nil {
		out.NotCoveredSessions = []string{}
	}
	if j.CoveredTranscripts > 0 {
		out.Turns, out.Cost = &j.Turns, &j.Cost
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
	// declarations (a subagent's declaration names its own file). They are
	// only ever compared against paths this package discovered itself;
	// see Join.
	transcripts map[string]bool
}

// turnsOf groups a run by prompt_id, the key digest groups a turn by -- and
// ONLY prompt_id, for digest's reason: a subagent's calls carry the parent's
// prompt_id, so they belong to the turn that spawned them. Executions and
// terminals carry no prompt_id and join through the tool_use_id of the
// declaration they answer, which is how digest builds the same small run.
// A declaration with no prompt_id (a record older than the field) belongs to
// no turn and is left out rather than guessed into one.
func turnsOf(run *store.Run) []turn {
	byPrompt := map[string]*turn{}
	owner := map[string]*turn{}
	span := func(t *turn, ms int64) {
		if ms < t.firstMS {
			t.firstMS = ms
		}
		if ms > t.lastMS {
			t.lastMS = ms
		}
	}
	for _, d := range run.Declarations {
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
		if t, ok := owner[x.ToolUseID]; ok {
			t.run.Executions = append(t.run.Executions, x)
			span(t, x.RecordedAtMS)
		}
	}
	for _, x := range run.Terminals {
		if t, ok := owner[x.ToolUseID]; ok {
			span(t, x.RecordedAtMS)
		}
	}
	out := make([]turn, 0, len(byPrompt))
	for _, t := range byPrompt {
		out = append(out, *t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].firstMS < out[j].firstMS })
	return out
}

// Join fills the silent-failure line from a store, or records that there is
// none when st is nil.
//
// st MUST come from store.OpenExisting (or be nil for ErrNoStore), never
// store.Open: spend is a question, and asking it must not mint an install
// identity -- H-87's rule, which every read-only command here follows.
//
// COVERAGE IS PER TRANSCRIPT. A windowed response's transcript is its main
// transcript (TranscriptFile.Main). That transcript is covered when a record
// of one of the window's sessions carries a transcript_path naming it, or
// naming a subagent transcript under it; every other transcript is not
// covered, however many run directories the store holds for its session id.
//
// Per recorded turn: the turn's own small run goes through
// report.BuildSilentFailures -- the very rule, and the very run shape, a
// turn's digest applies at Stop. The rule needs the turn's final message, and
// a turn with no recorded failure cannot fire whatever that message says, so
// transcripts are read only for turns with at least one, and each once for
// all of them (report.FinalAssistantTexts). Only the verdict is kept.
//
// A FIRING TURN'S SPEND IS KEYED BY ITS PROMPT, not by a span of recorded
// time. The main transcript ties each response to the promptId of the user
// line before it -- the prompt_id the hooks record -- so the turn's main-agent
// spend is exactly the responses tied to its prompt, including the one that
// made its first call and the final reply, which a record span left out. A
// subagent transcript carries no tie of its own that has been measured, so a
// subagent's spend joins the turn through the record: the prompt_id of the
// declarations whose transcript_path names that subagent's file. A span of
// time could also swallow a later turn's responses (a call whose execution
// was recorded after the next prompt began); a prompt key cannot.
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

	byTranscript := map[string][]*Response{}
	sessions := map[string]bool{}
	for _, r := range s.window {
		m := s.scan.Files[r.file].Main
		byTranscript[m] = append(byTranscript[m], r)
		sessions[r.SessionID] = true
	}
	j.Transcripts = len(byTranscript)

	covered := map[string]bool{}
	defer func() { s.markCoverage(byTranscript, covered) }()
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

	// A recorded transcript_path is only ever looked up here, against the
	// files Discover found: by its cleaned spelling, and a discovered file
	// also by its resolved one, so a projects directory reached through a
	// symlink still matches the real path Claude Code handed the hooks.
	known := map[string]int{}
	for i, f := range s.scan.Files {
		known[filepath.Clean(f.Path)] = i
		if real, err := filepath.EvalSymlinks(f.Path); err == nil {
			if _, ok := known[real]; !ok {
				known[real] = i
			}
		}
	}

	// subTurn is each recorded subagent file's prompt, "" when records of
	// more than one prompt name it and it cannot be split between them.
	subTurn := map[int]string{}
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
			i, ok := known[filepath.Clean(d.TranscriptPath)]
			if !ok || d.TranscriptPath == "" {
				continue
			}
			f := s.scan.Files[i]
			covered[f.Main] = true
			if f.Subagent && d.PromptID != nil && *d.PromptID != "" {
				if p, seen := subTurn[i]; !seen {
					subTurn[i] = *d.PromptID
				} else if p != *d.PromptID {
					subTurn[i] = ""
				}
			}
		}
		// Only a turn with a recorded failure can fire, whatever its final
		// message says; those alone need their transcripts read.
		for _, t := range turnsOf(run) {
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
	counted := map[*Response]bool{}
	for _, t := range turns {
		mains := s.namedMains(t.transcripts, known)
		if len(mains) == 0 {
			continue
		}
		byFile := make([]map[string]report.TurnFinal, len(mains))
		for k, p := range mains {
			if _, ok := finals[p]; !ok {
				finals[p] = report.FinalAssistantTexts(p, want)
			}
			byFile[k] = finals[p]
		}
		if !report.BuildSilentFailures(t.run, report.AccountFromMessage(lastSaid(byFile, t.prompt))).Fires {
			continue
		}
		j.Turns++
		for _, r := range s.window {
			if counted[r] || !s.inTurn(r, t.prompt, byFile, subTurn) {
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
func (s *Summary) markCoverage(byTranscript map[string][]*Response, covered map[string]bool) {
	j := &s.SilentFailureTurns
	type tally struct{ in, out int }
	bySession := map[string]*tally{}
	named := map[string]bool{}
	for m, rs := range byTranscript {
		if covered[m] {
			j.CoveredTranscripts++
		} else {
			j.NotCoveredTranscripts++
		}
		seen := map[string]bool{}
		for _, r := range rs {
			if !covered[m] {
				costOf(&j.NotCoveredCost, r)
			}
			if seen[r.SessionID] {
				continue
			}
			seen[r.SessionID] = true
			t := bySession[r.SessionID]
			if t == nil {
				t = &tally{}
				bySession[r.SessionID] = t
			}
			if covered[m] {
				t.in++
			} else {
				t.out++
				named[displaySession(r.SessionID)] = true
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

// namedMains is the discovered main transcripts among a turn's recorded
// transcript_paths, sorted.
func (s *Summary) namedMains(recorded map[string]bool, known map[string]int) []string {
	var out []string
	for p := range recorded {
		if i, ok := known[p]; ok && !s.scan.Files[i].Subagent {
			out = append(out, s.scan.Files[i].Path)
		}
	}
	sort.Strings(out)
	return out
}

// inTurn reports whether a response is the turn's spend: a main-agent
// response one of the turn's main transcripts ties to its prompt, or a
// response read from a subagent file whose records name that prompt alone.
func (s *Summary) inTurn(r *Response, prompt string, byFile []map[string]report.TurnFinal, subTurn map[int]string) bool {
	if s.scan.Files[r.file].Subagent {
		p, ok := subTurn[r.file]
		return ok && p == prompt
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
