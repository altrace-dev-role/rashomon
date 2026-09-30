package spend

import (
	"encoding/json"
	"math"
	"path/filepath"
	"sort"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/report"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// What the silent-failure line was built from.
const (
	// StoreNotConsulted: Join was never called. Only a caller that skipped
	// it can produce this, and the renderer then says nothing about the line
	// rather than a zero.
	StoreNotConsulted = "not_consulted"
	// StoreNone: there is no rashomon store on this machine, so no session
	// is covered and the line is unknown, not zero.
	StoreNone = "none"
	// StoreRead: a store was read.
	StoreRead = "read"
)

// TurnBound states how a turn is placed in time, wherever its number is.
const TurnBound = "a turn spans its first to its last recorded call; the response that made the first call and the final reply fall outside that span, so this is a floor"

// SilentFailureTurns is the line only rashomon can draw: spend inside turns
// whose recorded calls failed while the turn's final message mentioned no
// failure at all.
//
// It covers only sessions rashomon recorded. The rest are COUNTED and their
// spend is shown as not covered -- never folded in as zero, because a session
// nobody recorded is a session nobody checked, and zero would read as
// "checked and clean".
type SilentFailureTurns struct {
	Store              string `json:"store"`
	Sessions           int    `json:"sessions"`
	CoveredSessions    int    `json:"covered_sessions"`
	NotCoveredSessions int    `json:"not_covered_sessions"`
	NotCoveredCost     Cost   `json:"not_covered_cost"`
	Turns              int    `json:"turns"`
	Cost               Cost   `json:"cost"`
	Bound              string `json:"bound"`
}

// MarshalJSON writes turns and cost as null when no session is covered --
// no store, a store that recorded none of these sessions, or a Join that
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
	if j.CoveredSessions > 0 {
		out.Turns, out.Cost = &j.Turns, &j.Cost
	}
	return json.Marshal(out)
}

// turn is one prompt_id's records: the small run BuildSilentFailures takes,
// its span in recorded time, and the main transcript its records name.
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
// Per recorded session, per turn: the turn's own small run goes through
// report.BuildSilentFailures -- the very rule, and the very run shape, a
// turn's digest applies at Stop. The rule needs the turn's final message, and
// a turn with no recorded failure cannot fire whatever that message says, so
// the message is looked up only for turns with at least one: it is the last
// assistant text the session's main transcript attributes to the turn's
// prompt_id (report.FinalAssistantTexts, which says why the turn is keyed by
// its prompt and not by a span of time, and why that read lives in report).
// Only the verdict is kept.
//
// A firing turn's spend is every windowed response of that session, main or
// subagent, that started within the turn's first-to-last record span -- the
// design's join, which is a floor (TurnBound).
//
// ONE SESSION ID CAN NAME TWO CONVERSATIONS. Measured on this machine: a
// headless run started with --session-id reusing an interactive session's id
// wrote a second main transcript under another project directory, and the
// latest assistant text "across the session" was then the OTHER
// conversation's reply -- the verdict was taken on the wrong words. The
// hooks recorded which file each call belonged to (transcript_path), so when
// that path is one of the session's discovered main transcripts, the turn is
// scoped to it: the final message is read from that file alone, and only
// responses from it and from the subagent files under it are priced. A
// recorded path this package did not discover itself is never opened -- the
// read stays inside Claude Code's configuration directory -- and the turn
// then falls back to every main transcript of the session.
func (s *Summary) Join(st *store.Store) error {
	j := &s.SilentFailureTurns
	*j = SilentFailureTurns{Store: StoreNone, Bound: TurnBound, Sessions: s.Sessions}
	defer s.buildSavings()

	bySession := map[string][]*Response{}
	for _, r := range s.window {
		bySession[r.SessionID] = append(bySession[r.SessionID], r)
	}
	ids := make([]string, 0, len(bySession))
	for id := range bySession {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	notCovered := func(id string) {
		j.NotCoveredSessions++
		for _, r := range bySession[id] {
			costOf(&j.NotCoveredCost, r)
		}
	}
	if st == nil {
		for _, id := range ids {
			notCovered(id)
		}
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
	mains := s.mainTranscripts()

	for _, id := range ids {
		if !recorded[st.DirName(id)] {
			notCovered(id)
			continue
		}
		j.CoveredSessions++
		run, err := st.ReadRun(id)
		if err != nil {
			return err
		}
		// Only a turn with a recorded failure can fire, whatever its final
		// message says; those alone need their words read, and all of the
		// session's are read in one pass over each main file (finals).
		var turns []turn
		want := map[string]bool{}
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
		finals := map[string]map[string]report.TurnFinal{}
		counted := map[*Response]bool{}
		for _, t := range turns {
			files := scoped(mains[id], t.transcripts)
			final := finalMessage(finals, files, want, t.prompt)
			if !report.BuildSilentFailures(t.run, report.AccountFromMessage(final)).Fires {
				continue
			}
			j.Turns++
			for _, r := range bySession[id] {
				if !s.inConversation(r, files) {
					continue
				}
				if r.StartMS >= t.firstMS && r.StartMS <= t.lastMS && !counted[r] {
					counted[r] = true
					costOf(&j.Cost, r)
				}
			}
		}
	}
	return nil
}

// scoped is the session's discovered main transcripts narrowed to those the
// turn's records name, or all of them when the records name none of them.
func scoped(mains []string, recorded map[string]bool) []string {
	var out []string
	for _, p := range mains {
		if recorded[filepath.Clean(p)] {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return mains
	}
	return out
}

// inConversation reports whether a response was read from one of the given
// main transcripts or from a subagent transcript under one of them
// (<main without .jsonl>/subagents/...).
func (s *Summary) inConversation(r *Response, mains []string) bool {
	path := s.scan.Files[r.file].Path
	for _, m := range mains {
		if path == m || strings.HasPrefix(path, strings.TrimSuffix(m, ".jsonl")+string(filepath.Separator)+"subagents"+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// finalMessage is a turn's last assistant text across the given main
// transcripts, "" when none of them attributes any text to its prompt. A
// session id can own more than one main file (a resumed or re-run session
// written under a second project directory), so the latest across all of
// them is the turn's last word.
//
// finals caches each file's per-prompt texts, read once for every wanted
// prompt of the session: reading the file again per turn cost minutes on a
// long session with many failed turns, where the rest of spend took under a
// second.
func finalMessage(finals map[string]map[string]report.TurnFinal, paths []string, want map[string]bool, prompt string) string {
	var best string
	var bestMS int64 = math.MinInt64
	for _, p := range paths {
		byPrompt, ok := finals[p]
		if !ok {
			byPrompt = report.FinalAssistantTexts(p, want)
			finals[p] = byPrompt
		}
		if f, ok := byPrompt[prompt]; ok && f.AtMS >= bestMS {
			best, bestMS = f.Text, f.AtMS
		}
	}
	return best
}

// mainTranscripts maps a session id to its main (non-subagent) transcript
// files: those named for it, and those any of its responses were read from.
func (s *Summary) mainTranscripts() map[string][]string {
	out := map[string][]string{}
	seen := map[string]map[int]bool{}
	add := func(id string, i int) {
		if seen[id] == nil {
			seen[id] = map[int]bool{}
		}
		if seen[id][i] {
			return
		}
		seen[id][i] = true
		out[id] = append(out[id], s.scan.Files[i].Path)
	}
	for i, f := range s.scan.Files {
		if !f.Subagent {
			add(f.Session, i)
		}
	}
	for _, r := range s.scan.Responses {
		if !s.scan.Files[r.file].Subagent {
			add(r.SessionID, r.file)
		}
	}
	for id := range out {
		sort.Strings(out[id])
	}
	return out
}
