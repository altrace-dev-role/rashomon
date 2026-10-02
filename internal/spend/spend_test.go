package spend

import (
	"bytes"
	"encoding"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// canary is planted in every fixture's message.content -- in a text block,
// a thinking block and a tool_use input -- so a test can assert that nothing
// this package renders ever carries a byte of it.
const canary = "SPEND-CONTENT-CANARY-7f3a"

// now is fixed so every window in these tests is exact.
var now = time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)

// resp is one API response as a fixture writes it.
type resp struct {
	id, model, session string
	at                 time.Time
	in, out, read      int64
	w5, w1h            int64
	stop               string // "" writes stop_reason null
	iters              []resp // usage.iterations, when set
	typ                string // an iteration entry's type; "" writes "message"
	category           string // a refusal's stop_details.category; "" writes null
	sidechain          bool
	noTimestamp        bool
	noSplit            bool   // write cache_creation_input_tokens without the TTL split
	speed              string // usage.speed, when set
	text               string // the text block's words, when not the default
	requestID          string // the line's top-level requestId, when set
}

func (r resp) usage() map[string]any {
	u := map[string]any{
		"input_tokens":                r.in,
		"output_tokens":               r.out,
		"cache_read_input_tokens":     r.read,
		"cache_creation_input_tokens": r.w5 + r.w1h,
		"service_tier":                "standard",
	}
	if r.speed != "" {
		u["speed"] = r.speed
	}
	if !r.noSplit {
		u["cache_creation"] = map[string]any{
			"ephemeral_5m_input_tokens": r.w5,
			"ephemeral_1h_input_tokens": r.w1h,
		}
	}
	if r.iters != nil {
		var its []map[string]any
		for _, it := range r.iters {
			m := it.usage()
			m["type"] = "message"
			if it.typ != "" {
				m["type"] = it.typ
			}
			if it.model != "" {
				m["model"] = it.model
			}
			its = append(its, m)
		}
		u["iterations"] = its
	}
	return u
}

// line renders the response as one transcript line carrying one content
// block. Claude Code writes a response as one line per block, each with the
// full usage, which is the shape the dedupe exists for.
func (r resp) line(block string) string {
	var content []map[string]any
	switch block {
	case "thinking":
		content = []map[string]any{{"type": "thinking", "thinking": "thinking " + canary}}
	case "tool_use":
		content = []map[string]any{{"type": "tool_use", "id": "toolu_" + r.id, "name": "Bash",
			"input": map[string]any{"command": "echo " + canary}}}
	default:
		text := r.text
		if text == "" {
			text = "Done. " + canary
		}
		content = []map[string]any{{"type": "text", "text": text}}
	}
	var stop any
	if r.stop != "" {
		stop = r.stop
	}
	session := r.session
	if session == "" {
		session = "sess-a"
	}
	// stop_details is null for every stop reason but a refusal, whose
	// explanation is prose and carries the canary.
	var details any
	if r.stop == "refusal" {
		var category any
		if r.category != "" {
			category = r.category
		}
		details = map[string]any{"type": "refusal", "category": category, "explanation": "declined " + canary}
	}
	obj := map[string]any{
		"type":        "assistant",
		"sessionId":   session,
		"isSidechain": r.sidechain,
		"cwd":         "/home/someone/secret-project-" + canary,
		"message": map[string]any{
			"id":           r.id,
			"model":        r.model,
			"role":         "assistant",
			"stop_reason":  stop,
			"stop_details": details,
			"content":      content,
			"usage":        r.usage(),
		},
	}
	if !r.noTimestamp {
		obj["timestamp"] = r.at.UTC().Format(time.RFC3339Nano)
	}
	if r.requestID != "" {
		obj["requestId"] = r.requestID
	}
	b, err := json.Marshal(obj)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// userLine is a user line of a main transcript, in the shape Claude Code
// 2.1.285 writes: the typed prompt (toolResult false) or a tool_result
// answering one of the turn's calls, each carrying the turn's promptId. An
// empty promptID writes a line without the field, as an older version did.
func userLine(session, promptID string, at time.Time, toolResult bool) string {
	var content any = "a prompt " + canary
	if toolResult {
		content = []map[string]any{{"type": "tool_result", "tool_use_id": "toolu_x", "content": "out " + canary}}
	}
	obj := map[string]any{
		"type":        "user",
		"sessionId":   session,
		"isSidechain": false,
		"timestamp":   at.UTC().Format(time.RFC3339Nano),
		"message":     map[string]any{"role": "user", "content": content},
	}
	if promptID != "" {
		obj["promptId"] = promptID
	}
	b, err := json.Marshal(obj)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// config is a fake Claude Code configuration directory.
type config struct {
	t   *testing.T
	dir string
}

func newConfig(t *testing.T) *config { return &config{t: t, dir: t.TempDir()} }

// write puts lines in projects/<rel>, creating directories.
func (c *config) write(rel string, lines ...string) string {
	c.t.Helper()
	p := filepath.Join(c.dir, "projects", filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		c.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		c.t.Fatal(err)
	}
	return p
}

// summary reads the directory the way cmdSpend does, over `days` -- except
// that it hands Discover the zero time, so every file is read whatever its
// mtime and a fixture's own timestamps alone decide the window. A test of the
// mtime skip calls Discover itself.
func (c *config) summary(days int) *Summary {
	c.t.Helper()
	files, err := Discover(c.dir, time.Time{})
	if err != nil {
		c.t.Fatal(err)
	}
	sc, err := Read(files)
	if err != nil {
		c.t.Fatal(err)
	}
	return Build(sc, now, days)
}

// hasModel reports whether the summary has a by-model row for name.
func hasModel(s *Summary, name string) bool {
	for _, m := range s.ByModel {
		if m.Model == name {
			return true
		}
	}
	return false
}

func render(t *testing.T, s *Summary) (string, string) {
	t.Helper()
	var txt bytes.Buffer
	if err := Text(&txt, s); err != nil {
		t.Fatal(err)
	}
	j, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	return txt.String(), string(j)
}

// opus55 is Opus 5.5's rates in nanodollars per token, spelled out from the
// design's table ($4 / $20 / $0.20 per MTok) rather than read from the table
// under test.
const (
	opusIn    = 4000
	opusOut   = 20000
	opusRead  = 200
	opusW5    = 5000 // 1.25 x 4
	opusW1h   = 8000 // 2 x 4
	oneMinute = time.Minute
)

// TestDedupe_OneResponseWrittenOnSeveralLinesCountsOnce is the rule the
// design calls the most important one here: Claude Code writes one response
// as a line per content block, each carrying the SAME full usage, and summing
// lines overstated a real transcript's spend 2.1x.
func TestDedupe_OneResponseWrittenOnSeveralLinesCountsOnce(t *testing.T) {
	c := newConfig(t)
	r := resp{id: "msg_1", model: "claude-opus-5-5", at: now.Add(-time.Hour), in: 1000, out: 500, stop: "tool_use"}
	c.write("proj/sess-a.jsonl", r.line("thinking"), r.line("text"), r.line("tool_use"))

	s := c.summary(30)
	if s.Read.UsageLines != 3 {
		t.Fatalf("premise: usage lines = %d, want 3", s.Read.UsageLines)
	}
	if s.Responses != 1 {
		t.Errorf("responses = %d, want 1: three lines of one message.id are one response", s.Responses)
	}
	want := int64(1000*opusIn + 500*opusOut)
	if s.Total.Nano != want {
		t.Errorf("total = %d nanodollars, want %d (one response); three times that is the line-summing error", s.Total.Nano, want)
	}
	if s.Tokens.Output != 500 {
		t.Errorf("output tokens = %d, want 500", s.Tokens.Output)
	}
}

// TestDedupe_TheCompletedLineWinsOverTheStreamingPartial pins which line of
// a response is kept. A real subagent transcript writes a streaming line
// first (output 4, stop_reason null) and the completed one after it (output
// 107); keeping the first sighting undercounts that output 27x.
func TestDedupe_TheCompletedLineWinsOverTheStreamingPartial(t *testing.T) {
	for _, order := range []string{"partial first", "completed first"} {
		t.Run(order, func(t *testing.T) {
			c := newConfig(t)
			partial := resp{id: "msg_p", model: "claude-opus-5-5", at: now.Add(-time.Hour), in: 10, out: 4}
			done := partial
			done.out, done.stop, done.at = 107, "tool_use", partial.at.Add(400*time.Millisecond)
			done.iters = []resp{{in: 10, out: 107}}
			lines := []string{partial.line("thinking"), done.line("tool_use")}
			if order == "completed first" {
				lines[0], lines[1] = lines[1], lines[0]
			}
			c.write("proj/sess-a.jsonl", lines...)

			s := c.summary(30)
			if s.Responses != 1 {
				t.Fatalf("responses = %d, want 1", s.Responses)
			}
			if s.Tokens.Output != 107 {
				t.Errorf("output tokens = %d, want 107 from the completed line", s.Tokens.Output)
			}
			if got := s.window[0].StartMS; got != partial.at.UnixMilli() {
				t.Errorf("start = %d, want the earliest line's timestamp %d", got, partial.at.UnixMilli())
			}
		})
	}
	// What the completed line carries beyond its counts is kept with them:
	// a refusal's category, and a fallback's extra attempt and the model that
	// served it, which the streamed line does not have.
	for _, order := range []string{"partial first", "completed first"} {
		t.Run("a refusal and a fallback, "+order, func(t *testing.T) {
			c := newConfig(t)
			at := now.Add(-time.Hour)
			ref := resp{id: "msg_r", model: "claude-fable-5-1", at: at, in: 1000, out: 4}
			refDone := ref
			refDone.out, refDone.stop, refDone.category = 40, "refusal", "reasoning_extraction"
			fb := resp{id: "msg_f", model: "claude-fable-5", at: at, in: 412, out: 4}
			fbDone := fb
			fbDone.out, fbDone.stop = 264, "end_turn"
			fbDone.iters = []resp{{model: "claude-fable-5", in: 535, out: 50}, {typ: "fallback_message", model: "claude-opus-4-8", in: 412, out: 264}}
			lines := []string{ref.line("thinking"), refDone.line("text"), fb.line("thinking"), fbDone.line("text")}
			if order == "completed first" {
				lines = []string{refDone.line("text"), ref.line("thinking"), fbDone.line("text"), fb.line("thinking")}
			}
			c.write("proj/sess-a.jsonl", lines...)
			s := c.summary(30)
			if g := s.Refusals.ByCategory; len(g) != 1 || g[0].Category != "reasoning_extraction" || g[0].Model != "claude-fable-5-1" {
				t.Errorf("refusals by category = %+v, want reasoning_extraction on claude-fable-5-1", g)
			}
			if e := s.ExtraAttempts; e.FallbackServed != 1 || e.Attempts != 1 || e.Tokens.Total() != 585 {
				t.Errorf("extra attempts = %+v, want the fallback served and its 585-token declined attempt", e)
			}
			if !hasModel(s, "claude-opus-4-8") || hasModel(s, "claude-fable-5") {
				t.Errorf("by model = %+v, want the served response under claude-opus-4-8", s.ByModel)
			}
		})
	}
}

// TestDedupe_OneResponseInTwoFilesCountsOnce: a resumed session can carry
// earlier lines into a second file, and the id is the response either way.
func TestDedupe_OneResponseInTwoFilesCountsOnce(t *testing.T) {
	c := newConfig(t)
	r := resp{id: "msg_1", model: "claude-opus-5-5", at: now.Add(-time.Hour), in: 1000, stop: "end_turn"}
	c.write("proj/sess-a.jsonl", r.line("text"))
	c.write("other/sess-a.jsonl", r.line("text"))
	if s := c.summary(30); s.Responses != 1 || s.Total.Nano != 1000*opusIn {
		t.Errorf("responses = %d, total = %d; want 1 and %d", s.Responses, s.Total.Nano, 1000*opusIn)
	}
}

// TestWindow_DaysBoundsResponsesByTimestamp: the window is the response's own
// timestamp, and an undated response is excluded and counted, not guessed in.
func TestWindow_DaysBoundsResponsesByTimestamp(t *testing.T) {
	c := newConfig(t)
	old := resp{id: "msg_old", model: "claude-opus-5-5", at: now.Add(-31 * 24 * time.Hour), in: 1000, stop: "end_turn"}
	recent := resp{id: "msg_new", model: "claude-opus-5-5", at: now.Add(-29 * 24 * time.Hour), in: 2000, stop: "end_turn"}
	undated := resp{id: "msg_undated", model: "claude-opus-5-5", in: 3000, stop: "end_turn", noTimestamp: true}
	c.write("proj/sess-a.jsonl", old.line("text"), recent.line("text"), undated.line("text"))

	if s := c.summary(30); s.Total.Nano != 2000*opusIn || s.Responses != 1 {
		t.Errorf("30 days: total = %d over %d responses, want %d over 1", s.Total.Nano, s.Responses, 2000*opusIn)
	}
	s := c.summary(60)
	if s.Total.Nano != 3000*opusIn || s.Responses != 2 {
		t.Errorf("60 days: total = %d over %d responses, want %d over 2", s.Total.Nano, s.Responses, 3000*opusIn)
	}
	if s.Read.UndatedResponses != 1 {
		t.Errorf("undated = %d, want 1: an undated response is counted as excluded, not dropped silently", s.Read.UndatedResponses)
	}
	if txt, _ := render(t, s); !strings.Contains(txt, "carried no timestamp") {
		t.Errorf("the text does not say a response was left out:\n%s", txt)
	}
}

// TestWindow_AVeryLongWindowStillStartsBeforeNow: the window used to be a
// time.Duration of days x 24h, which overflows past 106,751 days and wrapped
// to a start AFTER now -- every response fell outside it and the answer was a
// confident $0.00. The longest window the command accepts must still hold a
// response from an hour ago.
func TestWindow_AVeryLongWindowStillStartsBeforeNow(t *testing.T) {
	c := newConfig(t)
	c.write("proj/sess-a.jsonl", resp{id: "r", model: "claude-opus-5-5", at: now.Add(-time.Hour), in: 1000, stop: "end_turn"}.line("text"))
	for _, days := range []int{MaxDays, 106752} {
		s := c.summary(days)
		if s.Responses != 1 || s.FromUnixMS >= now.UnixMilli() {
			t.Errorf("%d days: %d responses, window from %d (now %d); want the one response and a start before now",
				days, s.Responses, s.FromUnixMS, now.UnixMilli())
		}
	}
}

// TestWindow_AFutureDatedResponseIsNotTheLastNDays: a response dated after
// the run is not in "the last N days". It is counted as future-dated and
// said, not added to the total. One dated just after now -- a line the
// session being read wrote while spend ran -- still counts.
func TestWindow_AFutureDatedResponseIsNotTheLastNDays(t *testing.T) {
	c := newConfig(t)
	c.write("proj/sess-a.jsonl",
		resp{id: "r", model: "claude-opus-5-5", at: now.Add(-time.Hour), in: 1000, stop: "end_turn"}.line("text"),
		resp{id: "live", model: "claude-opus-5-5", at: now.Add(time.Minute), in: 10, stop: "end_turn"}.line("text"),
		resp{id: "future", model: "claude-opus-5-5", at: now.Add(48 * time.Hour), in: 1e6, stop: "end_turn"}.line("text"))
	s := c.summary(30)
	if s.Total.Nano != 1010*opusIn || s.Read.FutureDatedResponses != 1 {
		t.Errorf("total = %d with %d future-dated, want %d and 1", s.Total.Nano, s.Read.FutureDatedResponses, 1010*opusIn)
	}
	if txt, _ := render(t, s); !strings.Contains(txt, "1 response carried a timestamp after this run") {
		t.Errorf("the text does not say a future-dated response was left out:\n%s", txt)
	}
}

// TestWindow_OldTranscriptsAreNotNoTranscripts: every transcript was last
// written before the window. They were found; none was written in it. "no
// Claude Code transcripts were found" would send a reader looking for a
// misconfigured directory that is fine.
func TestWindow_OldTranscriptsAreNotNoTranscripts(t *testing.T) {
	c := newConfig(t)
	p := c.write("proj/old.jsonl", resp{id: "a", model: "claude-opus-5-5", at: now.Add(-40 * 24 * time.Hour), in: 1}.line("text"))
	past := time.Now().Add(-40 * 24 * time.Hour)
	if err := os.Chtimes(p, past, past); err != nil {
		t.Fatal(err)
	}
	found, err := Discover(c.dir, time.Now().Add(-30*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	sc, err := Read(found)
	if err != nil {
		t.Fatal(err)
	}
	txt, js := render(t, Build(sc, now, 30))
	if strings.Contains(txt, "no Claude Code transcripts were found") ||
		!strings.Contains(txt, "no Claude Code transcript was written in the last 30 days (1 older transcript last written before that was not read)") {
		t.Errorf("old transcripts are reported as none found:\n%s", txt)
	}
	if !strings.Contains(js, `"files_before_window":1`) {
		t.Errorf("the JSON does not count the old transcript:\n%s", js)
	}
}

// TestDiscover_ASymlinkedProjectFolderIsRead: a DirEntry reports a symlink as
// neither file nor directory, and a projects folder linked in from elsewhere
// was skipped without a word -- its spend an unflagged $0. It is read, and a
// record naming the transcript by its real path still covers it.
//
// Twice: once with every directory fully resolved, and once with the config
// directory and the link's target both under a symlinked ancestor -- a
// default macOS machine's TMPDIR, reached through /var -> /private/var. There
// the recorded path is spelled through the ancestor and the discovered one
// resolves past it, so neither spelling matched the other and the transcript
// read "not covered": the test failed on a Mac while CI, on Linux, was green.
//
// And a third time with a relative CLAUDE_CONFIG_DIR and a link with a
// relative target: the discovered path is relative, and resolving it as
// written gave a relative spelling no absolute recorded path could match.
func TestDiscover_ASymlinkedProjectFolderIsRead(t *testing.T) {
	for _, layout := range []string{"resolved", "under a symlinked ancestor", "relative config dir"} {
		t.Run(layout, func(t *testing.T) {
			base, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			root := base
			if layout == "under a symlinked ancestor" {
				if err := os.Mkdir(filepath.Join(base, "private"), 0o700); err != nil {
					t.Fatal(err)
				}
				root = filepath.Join(base, "var")
				if err := os.Symlink(filepath.Join(base, "private"), root); err != nil {
					t.Fatal(err)
				}
			}
			c := &config{t: t, dir: filepath.Join(root, "cfg")}
			if layout == "relative config dir" {
				t.Chdir(base)
				c.dir = "cfgdir"
			}
			real := filepath.Join(root, "elsewhere")
			if err := os.MkdirAll(filepath.Join(real, "sess-l", "subagents"), 0o700); err != nil {
				t.Fatal(err)
			}
			T := now.Add(-time.Hour)
			realMain := filepath.Join(real, "sess-l.jsonl")
			lines := userLine("sess-l", "p1", T, false) + "\n" +
				resp{id: "L1", model: "claude-opus-5-5", session: "sess-l", at: T, in: 100, stop: "end_turn"}.line("text") + "\n"
			if err := os.WriteFile(realMain, []byte(lines), 0o600); err != nil {
				t.Fatal(err)
			}
			sub := resp{id: "L2", model: "claude-opus-5-5", session: "sess-l", at: T, in: 20, stop: "end_turn", sidechain: true}.line("text")
			if err := os.WriteFile(filepath.Join(real, "sess-l", "subagents", "agent-1.jsonl"), []byte(sub+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(c.dir, "projects"), 0o700); err != nil {
				t.Fatal(err)
			}
			target := real
			if layout == "relative config dir" {
				target = filepath.Join("..", "..", "elsewhere")
			}
			if err := os.Symlink(target, filepath.Join(c.dir, "projects", "linked")); err != nil {
				t.Fatal(err)
			}
			rec := newRecorder(t)
			rec.transcript = realMain
			rec.call("sess-l", "p1", "toolu_l", T, T.Add(time.Second), store.ExecFailed)

			s := c.summary(30)
			if s.Responses != 2 || s.Total.Nano != 120*opusIn {
				t.Errorf("responses = %d, total = %d; want 2 and %d: the linked folder was not read", s.Responses, s.Total.Nano, 120*opusIn)
			}
			if err := s.Join(rec.st); err != nil {
				t.Fatal(err)
			}
			j := s.SilentFailureTurns
			if j.CoveredTranscripts != 1 || j.NotCoveredTranscripts != 0 {
				t.Errorf("covered %d, not covered %d; want 1, 0: a record naming the real path covers the linked transcript",
					j.CoveredTranscripts, j.NotCoveredTranscripts)
			}
			if j.Turns != 1 || j.Unjudged != 0 {
				t.Errorf("turns %d, unjudged %d; want 1 and 0: the recorded path names the linked transcript, so its failed turn is judged",
					j.Turns, j.Unjudged)
			}
		})
	}
}

// TestDiscover_OneTranscriptUnderTwoSpellingsIsReadOnce: after a repo moves,
// a common way to keep its history is to symlink the new project folder to
// its sibling, the old one. Discover then listed the transcript twice, once
// per spelling: two transcripts, one of them never named by a record, so the
// whole cost read not covered and a recorded conversation read partly
// recorded. A file whose resolved path was already kept is skipped, so it is
// one transcript, covered by a record naming either spelling, in either sort
// order of the two folder names.
func TestDiscover_OneTranscriptUnderTwoSpellingsIsReadOnce(t *testing.T) {
	for _, tc := range []struct{ old, link string }{
		{"-Users-me-old", "-Users-me-new"}, // the link sorts first
		{"-Users-me-old", "-Users-me-zzz"}, // the link sorts second
	} {
		for _, recordLink := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s recorded via link %v", tc.link, recordLink), func(t *testing.T) {
				c := newConfig(t)
				T := now.Add(-time.Hour)
				realMain := c.write(tc.old+"/sess-l.jsonl", userLine("sess-l", "p1", T, false),
					resp{id: "L1", model: "claude-opus-5-5", session: "sess-l", at: T, in: 100, stop: "end_turn"}.line("text"))
				if err := os.Symlink(tc.old, filepath.Join(c.dir, "projects", tc.link)); err != nil {
					t.Fatal(err)
				}
				rec := newRecorder(t)
				rec.transcript = realMain
				if recordLink {
					rec.transcript = filepath.Join(c.dir, "projects", tc.link, "sess-l.jsonl")
				}
				rec.call("sess-l", "p1", "toolu_l", T, T.Add(time.Second), store.ExecOK)

				s := c.summary(30)
				if s.Read.Files != 1 || s.Responses != 1 {
					t.Errorf("files %d, responses %d; want 1 and 1: one transcript was read under both spellings", s.Read.Files, s.Responses)
				}
				if err := s.Join(rec.st); err != nil {
					t.Fatal(err)
				}
				j := s.SilentFailureTurns
				if j.Transcripts != 1 || j.CoveredTranscripts != 1 || j.NotCoveredTranscripts != 0 || j.NotCoveredCost.Priced != 0 {
					t.Errorf("transcripts %d, covered %d, not covered %d (%+v); want 1, 1, 0: the second spelling read as an unrecorded copy",
						j.Transcripts, j.CoveredTranscripts, j.NotCoveredTranscripts, j.NotCoveredCost)
				}
				if len(s.PerSession) != 1 || s.PerSession[0].Coverage != CoverageRecorded {
					t.Errorf("per session = %+v, want sess-l recorded", s.PerSession)
				}
			})
		}
		// An old transcript under both spellings is one old transcript, not
		// two: Discover counted it once per spelling before the dedupe.
		t.Run(tc.link+" last written before the window", func(t *testing.T) {
			c := newConfig(t)
			p := c.write(tc.old+"/sess-l.jsonl",
				resp{id: "L1", model: "claude-opus-5-5", session: "sess-l", at: now.Add(-40 * 24 * time.Hour), in: 100, stop: "end_turn"}.line("text"))
			past := time.Now().Add(-40 * 24 * time.Hour)
			if err := os.Chtimes(p, past, past); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(tc.old, filepath.Join(c.dir, "projects", tc.link)); err != nil {
				t.Fatal(err)
			}
			found, err := Discover(c.dir, time.Now().Add(-30*24*time.Hour))
			if err != nil {
				t.Fatal(err)
			}
			sc, err := Read(found)
			if err != nil {
				t.Fatal(err)
			}
			txt, js := render(t, Build(sc, now, 30))
			if !strings.Contains(js, `"files_before_window":1`) ||
				!strings.Contains(txt, "(1 older transcript last written before that was not read)") {
				t.Errorf("one old transcript under two spellings is counted twice:\n%s\n%s", txt, js)
			}
		})
	}
}

// TestDiscover_AnUnreadableFolderIsCountedNotFatal: one folder that cannot be
// read used to abort the whole command. Here two cannot -- a symlink that
// loops and one that dangles -- and the rest is still counted, with a note
// saying how many folders were not.
func TestDiscover_AnUnreadableFolderIsCountedNotFatal(t *testing.T) {
	c := newConfig(t)
	c.write("proj/sess-a.jsonl", resp{id: "r", model: "claude-opus-5-5", at: now.Add(-time.Hour), in: 1000, stop: "end_turn"}.line("text"))
	loop := filepath.Join(c.dir, "projects", "loop")
	if err := os.Symlink(loop, loop); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(c.dir, "gone"), filepath.Join(c.dir, "projects", "proj", "sess-gone")); err != nil {
		t.Fatal(err)
	}
	s := c.summary(30)
	if s.Total.Nano != 1000*opusIn || s.Read.UnreadableDirs != 2 {
		t.Errorf("total = %d, unreadable dirs = %d; want %d and 2", s.Total.Nano, s.Read.UnreadableDirs, 1000*opusIn)
	}
	if txt, _ := render(t, s); !strings.Contains(txt, "note: 2 folders under projects/ could not be read") {
		t.Errorf("the text does not say folders were left out:\n%s", txt)
	}

	// And a folder the process may not list, where permissions apply.
	if os.Geteuid() == 0 {
		return
	}
	locked := filepath.Join(c.dir, "projects", "locked")
	if err := os.Mkdir(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })
	if s := c.summary(30); s.Read.UnreadableDirs != 3 || s.Total.Nano != 1000*opusIn {
		t.Errorf("with a locked folder: unreadable dirs = %d, total = %d; want 3 and %d", s.Read.UnreadableDirs, s.Total.Nano, 1000*opusIn)
	}
}

// TestRead_AnUnreadableTranscriptIsCountedAndSaid: a transcript that cannot
// be read to the end is spend this reader did not count, so it is counted in
// read.unreadable_files and said in a note, never dropped from the total
// without a word. Two ways a file fails: it cannot be opened (skipped as
// root, which permissions do not stop), and a line longer than maxLine, which
// the scanner cannot step over.
func TestRead_AnUnreadableTranscriptIsCountedAndSaid(t *testing.T) {
	for _, how := range []string{"not permitted", "a line past maxLine"} {
		t.Run(how, func(t *testing.T) {
			c := newConfig(t)
			c.write("proj/sess-a.jsonl", resp{id: "r", model: "claude-opus-5-5", at: now.Add(-time.Hour), in: 1000, stop: "end_turn"}.line("text"))
			bad := c.write("proj/sess-b.jsonl", resp{id: "b", model: "claude-opus-5-5", at: now.Add(-time.Hour), in: 7, stop: "end_turn"}.line("text"))
			switch how {
			case "not permitted":
				if os.Geteuid() == 0 {
					t.Skip("root reads a file whatever its mode")
				}
				if err := os.Chmod(bad, 0o000); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(bad, 0o600) })
			default:
				f, err := os.OpenFile(bad, os.O_APPEND|os.O_WRONLY, 0)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := f.WriteString(`{"usage":"` + strings.Repeat("x", maxLine) + "\"}\n"); err != nil {
					t.Fatal(err)
				}
				if err := f.Close(); err != nil {
					t.Fatal(err)
				}
			}
			s := c.summary(30)
			if s.scan.Unreadable != 1 || s.Read.UnreadableFiles != 1 {
				t.Errorf("unreadable = %d, read.unreadable_files = %d; want 1 and 1", s.scan.Unreadable, s.Read.UnreadableFiles)
			}
			txt, js := render(t, s)
			if !strings.Contains(js, `"unreadable_files":1`) {
				t.Errorf("the JSON does not count the unreadable transcript:\n%s", js)
			}
			if !strings.Contains(txt, "note: 1 transcript file could not be read to the end; what they hold past that point is not counted") {
				t.Errorf("the text does not say a transcript could not be read to the end:\n%s", txt)
			}
		})
	}
}

// TestUnparsed_AMalformedOrImplausibleLineIsCountedNotPriced: a token count
// written as a string or a timestamp as a number made the whole line fail to
// decode, and it was dropped without a word; a negative count was priced as
// negative dollars, and an absurd one as an absurd bill. Each is left out of
// every figure and counted, and the text says so.
func TestUnparsed_AMalformedOrImplausibleLineIsCountedNotPriced(t *testing.T) {
	c := newConfig(t)
	good := resp{id: "ok", model: "claude-opus-5-5", at: now.Add(-time.Hour), in: 1000, stop: "end_turn"}.line("text")
	bend := func(id string, edit func(map[string]any)) string {
		var obj map[string]any
		if err := json.Unmarshal([]byte(resp{id: id, model: "claude-opus-5-5", at: now.Add(-time.Hour), in: 5, stop: "end_turn"}.line("text")), &obj); err != nil {
			t.Fatal(err)
		}
		edit(obj)
		b, err := json.Marshal(obj)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	usageOf := func(o map[string]any) map[string]any { return o["message"].(map[string]any)["usage"].(map[string]any) }
	c.write("proj/sess-a.jsonl", good,
		bend("str", func(o map[string]any) { usageOf(o)["input_tokens"] = "500" }),
		bend("num", func(o map[string]any) { o["timestamp"] = 1790683200 }),
		bend("neg", func(o map[string]any) { usageOf(o)["output_tokens"] = -2000000 }),
		bend("huge", func(o map[string]any) { usageOf(o)["cache_read_input_tokens"] = int64(1) << 50 }),
		bend("iter", func(o map[string]any) {
			usageOf(o)["iterations"] = []map[string]any{{"input_tokens": -7}, {"input_tokens": 5}}
		}),
		// No message.id to deduplicate by: with tokens it is counted as a
		// line that could not be; with none (nothing billed) it is not.
		resp{model: "claude-opus-5-5", at: now.Add(-time.Hour), in: 5_000_000, stop: "end_turn"}.line("text"),
		resp{model: "<synthetic>", at: now.Add(-time.Hour), stop: "end_turn"}.line("text"))
	// A subagent's user line with a numeric timestamp does not decode either,
	// but it carries no usage: it is not an unparsed usage line.
	c.write("proj/sess-a/subagents/agent-u.jsonl",
		strings.Replace(subUserLine("sess-a", "p1", now.Add(-time.Hour)), `"timestamp":"`, `"timestamp":5,"x":"`, 1))
	s := c.summary(30)
	if s.Total.Nano != 1000*opusIn || s.Responses != 1 {
		t.Errorf("total = %d over %d responses, want %d over 1: a malformed or implausible line was priced",
			s.Total.Nano, s.Responses, 1000*opusIn)
	}
	if s.Read.UnparsedUsageLines != 6 {
		t.Errorf("unparsed usage lines = %d, want 6", s.Read.UnparsedUsageLines)
	}
	if s.Read.UsageLines != 4 {
		t.Errorf("usage lines = %d, want 4: an id-less line is not one the dedupe measured", s.Read.UsageLines)
	}
	if txt, _ := render(t, s); !strings.Contains(txt, "note: 6 transcript lines that may carry usage could not be read") {
		t.Errorf("the text does not say lines were left out:\n%s", txt)
	}
}

// TestDiscover_SkipsFilesLastWrittenBeforeTheWindow: a file whose mtime is
// older than the window cannot hold a response inside it.
func TestDiscover_SkipsFilesLastWrittenBeforeTheWindow(t *testing.T) {
	c := newConfig(t)
	oldPath := c.write("proj/old.jsonl", resp{id: "a", model: "claude-opus-5-5", at: now.Add(-40 * 24 * time.Hour), in: 1}.line("text"))
	c.write("proj/new.jsonl", resp{id: "b", model: "claude-opus-5-5", at: now.Add(-time.Hour), in: 1}.line("text"))
	past := time.Now().Add(-40 * 24 * time.Hour)
	if err := os.Chtimes(oldPath, past, past); err != nil {
		t.Fatal(err)
	}
	files, err := Discover(c.dir, time.Now().Add(-30*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(files.Files) != 1 || filepath.Base(files.Files[0].Path) != "new.jsonl" || files.Stale != 1 {
		t.Errorf("discovered %+v, want only new.jsonl and the old one counted as stale", files)
	}
}

// TestDiscover_AFileThatCannotBeStatedIsNotDropped: in a folder that can be
// listed but not searched, every Stat fails, and Discover dropped each
// transcript as if it were old -- no count, no note, "no Claude Code
// transcripts were found to read". Only a file that no longer exists is
// dropped; any other failure keeps the file, for Read to open or count as
// unreadable. The failure is injected: root searches any folder.
func TestDiscover_AFileThatCannotBeStatedIsNotDropped(t *testing.T) {
	c := newConfig(t)
	locked := c.write("proj/locked.jsonl", resp{id: "a", model: "claude-opus-5-5", at: now.Add(-time.Hour), in: 1}.line("text"))
	gone := c.write("proj/gone.jsonl", resp{id: "b", model: "claude-opus-5-5", at: now.Add(-time.Hour), in: 1}.line("text"))
	t.Cleanup(func() { statFile = os.Stat })
	statFile = func(p string) (os.FileInfo, error) {
		switch p {
		case locked:
			return nil, &fs.PathError{Op: "stat", Path: p, Err: fs.ErrPermission}
		case gone:
			return nil, &fs.PathError{Op: "stat", Path: p, Err: fs.ErrNotExist}
		}
		return os.Stat(p)
	}
	found, err := Discover(c.dir, time.Now().Add(-30*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(found.Files) != 1 || found.Files[0].Path != locked || found.Stale != 0 {
		t.Errorf("discovered %+v, stale %d; want only locked.jsonl, kept for Read, and nothing counted stale", found.Files, found.Stale)
	}
}

// TestAgent_SubagentTranscriptsAreSubagentSpend: main vs subagent is a file-
// path fact, at any depth under subagents/ (a workflow's agents write two
// levels down), and a main-file line marked isSidechain is a subagent's too.
func TestAgent_SubagentTranscriptsAreSubagentSpend(t *testing.T) {
	c := newConfig(t)
	at := now.Add(-time.Hour)
	c.write("proj/sess-a.jsonl",
		resp{id: "m1", model: "claude-opus-5-5", at: at, in: 1000, stop: "end_turn"}.line("text"),
		resp{id: "m2", model: "claude-opus-5-5", at: at, in: 10, stop: "end_turn", sidechain: true}.line("text"))
	c.write("proj/sess-a/subagents/agent-a1.jsonl",
		resp{id: "s1", model: "claude-opus-5-5", at: at, in: 200, stop: "end_turn", sidechain: true}.line("text"))
	c.write("proj/sess-a/subagents/workflows/run-1/agent-a2.jsonl",
		// isSidechain false on purpose: the file's place under subagents/ is
		// the fact, and a line's own flag is only the fallback.
		resp{id: "s2", model: "claude-opus-5-5", at: at, in: 300, stop: "end_turn"}.line("text"))
	c.write("proj/sess-a/subagents/not-an-agent.jsonl",
		resp{id: "x", model: "claude-opus-5-5", at: at, in: 99999, stop: "end_turn"}.line("text"))

	s := c.summary(30)
	if s.ByAgent.Main.Nano != 1000*opusIn {
		t.Errorf("main = %d, want %d", s.ByAgent.Main.Nano, 1000*opusIn)
	}
	if want := int64((10 + 200 + 300) * opusIn); s.ByAgent.Subagents.Nano != want {
		t.Errorf("subagents = %d, want %d (sidechain line, agent-a1 and the workflow's agent-a2)", s.ByAgent.Subagents.Nano, want)
	}
	if s.Sessions != 1 || len(s.PerSession) != 1 || s.PerSession[0].SessionID != "sess-a" {
		t.Errorf("sessions = %d %+v, want the one session", s.Sessions, s.PerSession)
	}
}

// TestAgent_NoShareBesideAnUnknown: the subagents ran on a model the table
// does not know, with fifty times the main agent's tokens. A share of the
// priced part would print main 100% and subagents 0%; the record supports
// neither, so no share is printed.
func TestAgent_NoShareBesideAnUnknown(t *testing.T) {
	c := newConfig(t)
	at := now.Add(-time.Hour)
	c.write("proj/sess-a.jsonl",
		resp{id: "m1", model: "claude-opus-5-5", at: at, in: 1_000_000, stop: "end_turn"}.line("text"))
	c.write("proj/sess-a/subagents/agent-a.jsonl",
		resp{id: "s1", model: "claude-mystery-9", at: at, in: 50_000_000, stop: "end_turn", sidechain: true}.line("text"))
	txt, _ := render(t, c.summary(30))
	if !strings.Contains(txt, "main $4.00   subagents unknown\n") {
		t.Errorf("the by-agent line claims a share beside an unknown:\n%s", txt)
	}
	if strings.Contains(txt, "%)") {
		t.Errorf("a percentage was printed beside an unknown:\n%s", txt)
	}
}

// TestAgent_SharesSumTo100AndNeverCallANonZeroSide0: each share rounded on
// its own printed 100% and 1% (101%) for a 99.5/0.5 split, and a side under
// half a percent printed 0% beside its own non-zero dollars.
func TestAgent_SharesSumTo100AndNeverCallANonZeroSide0(t *testing.T) {
	for _, tc := range []struct {
		main, sub int64
		want      string
	}{
		{995, 5, "(>99%)   subagents <$0.01 (<1%)"},
		{1999, 1, "(>99%)   subagents <$0.01 (<1%)"},
		{25, 75, "(25%)   subagents <$0.01 (75%)"},
		{1015, 985, "(51%)   subagents <$0.01 (49%)"},
		{1, 2, "(33%)   subagents <$0.01 (67%)"},
	} {
		c := newConfig(t)
		at := now.Add(-time.Hour)
		c.write("proj/sess-a.jsonl", resp{id: "m", model: "claude-opus-5-5", at: at, in: tc.main, stop: "end_turn"}.line("text"))
		c.write("proj/sess-a/subagents/agent-a.jsonl",
			resp{id: "s", model: "claude-opus-5-5", at: at, in: tc.sub, stop: "end_turn", sidechain: true}.line("text"))
		txt, _ := render(t, c.summary(30))
		if !strings.Contains(txt, tc.want) {
			t.Errorf("main %d, sub %d tokens: by-agent line lacks %q:\n%s", tc.main, tc.sub, tc.want, txt)
		}
	}
}

// TestAgent_SharesDoNotOverflow: a*100 overflowed an int64 once an amount
// passed about 9e16 nanodollars, and shares(1e17, 1) printed "-84%". Only a
// fabricated or corrupted transcript that passes the plausibility check gets
// there, but a percentage that is negative is never an answer.
func TestAgent_SharesDoNotOverflow(t *testing.T) {
	for _, tc := range []struct {
		a, b   int64
		pa, pb string
	}{
		{1e17, 1, ">99%", "<1%"},
		{4e18, 4e18, "50%", "50%"},
		{3e18, 1e18, "75%", "25%"},
		{1, 9e18, "<1%", ">99%"},
	} {
		if pa, pb := shares(tc.a, tc.b); pa != tc.pa || pb != tc.pb {
			t.Errorf("shares(%d, %d) = %s, %s; want %s, %s", tc.a, tc.b, pa, pb, tc.pa, tc.pb)
		}
	}
}

// TestPricing_TheTableIsTheDesignsTable prices one million tokens of each
// kind on every model and compares with the design's own figures, written
// here in dollars so a reader can hold them against the table.
func TestPricing_TheTableIsTheDesignsTable(t *testing.T) {
	type row struct{ in, out, read float64 }
	want := map[string]row{
		"claude-fable-5-1":  {10, 50, 0.25},
		"claude-opus-5-5":   {4, 20, 0.20},
		"claude-opus-5":     {5, 25, 0.5},
		"claude-opus-4-8":   {5, 25, 0.5},
		"claude-opus-4-7":   {5, 25, 0.5},
		"claude-opus-4-6":   {5, 25, 0.5},
		"claude-sonnet-5-5": {2, 10, 0.20},
		"claude-sonnet-5":   {2, 10, 0.20},
		"claude-sonnet-4-6": {3, 15, 0.3},
		"claude-haiku-4-5":  {1, 5, 0.1},
		// Published on the same page, and priced as unknown until they had
		// rows: three current models, and four retired but still served on
		// Bedrock or Google Cloud (Opus 4: Google Cloud only).
		"claude-opus-4-5":   {5, 25, 0.5},
		"claude-sonnet-4-5": {3, 15, 0.3},
		"claude-fable-5":    {10, 50, 1},
		"claude-opus-4-1":   {15, 75, 1.5},
		"claude-opus-4":     {15, 75, 1.5},
		"claude-sonnet-4":   {3, 15, 0.3},
		"claude-3-5-haiku":  {0.8, 4, 0.08},
		// The two Mythos models, at their Fable twins' rates -- and so with
		// different cache reads: one row for both would price one model's
		// cache reads 4x off.
		"claude-mythos-5-1": {10, 50, 0.25},
		"claude-mythos-5":   {10, 50, 1},
	}
	if len(want) != len(table) {
		t.Errorf("table has %d rows, the design has %d", len(table), len(want))
	}
	const M = 1_000_000
	for model, w := range want {
		r := &Response{Model: model, Tokens: Tokens{Input: M, Output: M, CacheRead: M, CacheWrite5m: M, CacheWrite1h: M}}
		p := price(r)
		if !p.ok {
			t.Errorf("%s is not priced", model)
			continue
		}
		dollars := func(n int64) float64 { return float64(n) / 1e9 }
		if dollars(p.input) != w.in || dollars(p.output) != w.out || dollars(p.cacheRead) != w.read {
			t.Errorf("%s: in/out/read = %v/%v/%v per MTok, want %v/%v/%v",
				model, dollars(p.input), dollars(p.output), dollars(p.cacheRead), w.in, w.out, w.read)
		}
		if got, wantW := dollars(p.cacheWrite), w.in*1.25+w.in*2; got != wantW {
			t.Errorf("%s: a 5m plus a 1h write of 1 MTok each = $%v, want $%v (1.25x and 2x input)", model, got, wantW)
		}
	}
}

// TestPricing_CacheWritesArePricedByTTL: 5m at 1.25x, 1h at 2x, and a total
// with no TTL split at the API's default 5m.
func TestPricing_CacheWritesArePricedByTTL(t *testing.T) {
	c := newConfig(t)
	at := now.Add(-time.Hour)
	c.write("proj/sess-a.jsonl",
		resp{id: "a", model: "claude-opus-5-5", at: at, w5: 1000, stop: "end_turn"}.line("text"),
		resp{id: "b", model: "claude-opus-5-5", at: at, w1h: 1000, stop: "end_turn"}.line("text"),
		resp{id: "c", model: "claude-opus-5-5", at: at, w5: 1000, noSplit: true, stop: "end_turn"}.line("text"))
	s := c.summary(30)
	if want := int64(1000*opusW5 + 1000*opusW1h + 1000*opusW5); s.ByKind.CacheWrite.Nano != want {
		t.Errorf("cache write = %d, want %d", s.ByKind.CacheWrite.Nano, want)
	}
}

// TestPriceKey_MatchesConservatively: an exact id or one dated suffix, and
// nothing looser. The prefix shortcut would price Opus 5.5 as Opus 5.
func TestPriceKey_MatchesConservatively(t *testing.T) {
	for model, want := range map[string]string{
		"claude-haiku-4-5-20251001":  "claude-haiku-4-5",
		"claude-opus-5-5":            "claude-opus-5-5",
		"claude-opus-5":              "claude-opus-5",
		"claude-opus-5-20260101":     "claude-opus-5",
		"claude-opus-4-20250514":     "claude-opus-4",
		"claude-opus-4-5-20251101":   "claude-opus-4-5",
		"claude-3-5-haiku-20241022":  "claude-3-5-haiku",
		"claude-opus-5-5-preview":    "",
		"claude-opus-5-5[1m]":        "",
		"claude-opus-5-5-2026010":    "",
		"claude-opus-5-5-2026010x":   "",
		"us.anthropic.claude-opus-5": "",
		"claude-opus-9":              "",
		"<synthetic>":                "",
		"":                           "",
	} {
		got, ok := PriceKey(model)
		if got != want || ok != (want != "") {
			t.Errorf("PriceKey(%q) = %q, %v; want %q", model, got, ok, want)
		}
	}
}

// TestUnknownModel_IsUnknownNeverZero: tokens shown, cost unknown, and not a
// "$0.00" anywhere in either rendering.
func TestUnknownModel_IsUnknownNeverZero(t *testing.T) {
	c := newConfig(t)
	c.write("proj/sess-a.jsonl",
		resp{id: "u1", model: "claude-mystery-9", at: now.Add(-time.Hour), in: 1234, out: 5678, stop: "end_turn"}.line("text"))
	s := c.summary(30)
	if s.Total.Known() || s.Total.Wholly() {
		t.Errorf("total = %+v: an unpriced model must leave the total unknown", s.Total)
	}
	if len(s.ByModel) != 1 || s.ByModel[0].Priced || s.ByModel[0].Tokens.Total() != 1234+5678 {
		t.Errorf("by model = %+v, want one unpriced row carrying its tokens", s.ByModel)
	}
	txt, js := render(t, s)
	if strings.Contains(txt, "$0.00") {
		t.Errorf("an unpriced model rendered as $0.00:\n%s", txt)
	}
	if !strings.Contains(txt, "claude-mystery-9 6,912 tokens, cost unknown") || !strings.Contains(txt, "cost unknown (6,912 tokens") {
		t.Errorf("text does not show the unpriced tokens as cost unknown:\n%s", txt)
	}
	var doc struct {
		Total struct {
			USD      *float64 `json:"usd"`
			Unpriced int      `json:"unpriced_responses"`
		} `json:"total"`
	}
	if err := json.Unmarshal([]byte(js), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Total.USD != nil || doc.Total.Unpriced != 1 {
		t.Errorf("json total = %+v, want usd null and 1 unpriced response", doc.Total)
	}
}

// TestUnknownModel_ANonClaudeIdIsNotPrinted: the model string is whatever a
// deployment put there, and a provider ARN carries an account number.
func TestUnknownModel_ANonClaudeIdIsNotPrinted(t *testing.T) {
	c := newConfig(t)
	arn := "arn:aws:bedrock:us-east-1:123456789012:inference-profile/x"
	c.write("proj/sess-a.jsonl",
		resp{id: "u1", model: arn, at: now.Add(-time.Hour), in: 2, stop: "end_turn"}.line("text"),
		// claude-shaped at the front, and still carrying something that is not
		// a model name.
		resp{id: "u2", model: "claude-custom/123456789012", at: now.Add(-time.Hour), in: 3, stop: "end_turn"}.line("text"))
	txt, js := render(t, c.summary(30))
	for _, out := range []string{txt, js} {
		if strings.Contains(out, "123456789012") {
			t.Errorf("a non-claude model id reached the output:\n%s", out)
		}
	}
	if !strings.Contains(txt, "other 5 tokens, cost unknown") {
		t.Errorf("the unpriced model is not shown as other:\n%s", txt)
	}
}

// TestSessionID_OnlyAClosedShapeIsPrinted: a transcript's sessionId is a
// string a line carries, and a malformed or hostile one can be a path. It is
// printed only in the shape a Claude Code session id has -- the rule model
// ids follow -- and otherwise as "other".
func TestSessionID_OnlyAClosedShapeIsPrinted(t *testing.T) {
	c := newConfig(t)
	leak := "../../home/someone/secret-" + canary
	c.write("proj/sess-a.jsonl",
		resp{id: "r1", model: "claude-opus-5-5", session: leak, at: now.Add(-time.Hour), in: 2, stop: "end_turn"}.line("text"),
		resp{id: "r2", model: "claude-opus-5-5", session: "0b6a6a4e-8d5c-4b8e-9d52-0c1f6f2a3b4c", at: now.Add(-time.Hour), in: 3, stop: "end_turn"}.line("text"))
	s := c.summary(30)
	if err := s.Join(nil); err != nil {
		t.Fatal(err)
	}
	txt, js := render(t, s)
	for _, out := range []string{txt, js} {
		if strings.Contains(out, "secret-") || strings.Contains(out, "../") {
			t.Errorf("a path-shaped session id reached the output:\n%s", out)
		}
	}
	if !strings.Contains(js, `"session_id":"other"`) || !strings.Contains(js, `"session_id":"0b6a6a4e-8d5c-4b8e-9d52-0c1f6f2a3b4c"`) {
		t.Errorf("per_session does not name the uuid and fold the path into other:\n%s", js)
	}
}

// TestSessions_TheTextNamesTheCostliestAndTheirCoverage: per-session spend
// was JSON-only. The text lists the costliest sessions with their split and,
// once the store was consulted, which ones rashomon did not record; the rest
// are counted, and --json has every one.
func TestSessions_TheTextNamesTheCostliestAndTheirCoverage(t *testing.T) {
	c := newConfig(t)
	for i := 1; i <= 7; i++ {
		id := "sess-" + string(rune('a'+i-1))
		c.write("proj/"+id+".jsonl", resp{id: "r" + id, model: "claude-opus-5-5", session: id,
			at: now.Add(-time.Hour), in: int64(i) * 1_000_000, stop: "end_turn"}.line("text"))
	}
	c.write("proj/sess-g/subagents/agent-1.jsonl", resp{id: "sub", model: "claude-opus-5-5", session: "sess-g",
		at: now.Add(-time.Hour), in: 500_000, stop: "end_turn", sidechain: true}.line("text"))
	s := c.summary(30)
	if err := s.Join(nil); err != nil {
		t.Fatal(err)
	}
	txt, _ := render(t, s)
	for _, want := range []string{
		"by session    sess-g $30.00 (main $28.00, subagents $2.00), not recorded by rashomon\n",
		"              sess-c $12.00 (main $12.00, subagents none), not recorded by rashomon\n",
		"              and 2 more (--json lists every session)\n",
	} {
		if !strings.Contains(txt, want) {
			t.Errorf("text lacks %q:\n%s", want, txt)
		}
	}
	if strings.Contains(txt, "sess-a $") {
		t.Errorf("the text lists more than the costliest five:\n%s", txt)
	}
}

// TestZeroTokenResponses_AreNotCounted: Claude Code's "<synthetic>" lines
// carry an all-zero usage; they were not billed and are not a model to list.
func TestZeroTokenResponses_AreNotCounted(t *testing.T) {
	c := newConfig(t)
	c.write("proj/sess-a.jsonl",
		resp{id: "syn", model: "<synthetic>", at: now.Add(-time.Hour), stop: "stop_sequence"}.line("text"),
		resp{id: "r", model: "claude-opus-5-5", at: now.Add(-time.Hour), in: 1, stop: "end_turn"}.line("text"))
	s := c.summary(30)
	if len(s.ByModel) != 1 || !s.Total.Known() || s.Responses != 1 {
		t.Errorf("by model = %+v, total = %+v: a zero-token response was counted", s.ByModel, s.Total)
	}
}

// TestSubCentAmounts_AreNotRenderedAsZero.
func TestSubCentAmounts_AreNotRenderedAsZero(t *testing.T) {
	c := newConfig(t)
	c.write("proj/sess-a.jsonl", resp{id: "r", model: "claude-opus-5-5", at: now.Add(-time.Hour), in: 100, stop: "end_turn"}.line("text"))
	txt, _ := render(t, c.summary(30))
	if !strings.Contains(txt, "est. <$0.01 at API list prices") {
		t.Errorf("a $0.0004 total is not shown as under a cent:\n%s", txt)
	}
}

// TestCacheExpiry_WriteAfterTheTTLIsCold walks the heuristic's cases in one
// transcript, and a subagent's first write, which follows nothing in its own
// file, is a cold start and never an expiry.
func TestCacheExpiry_WriteAfterTheTTLIsCold(t *testing.T) {
	c := newConfig(t)
	t0 := now.Add(-5 * time.Hour)
	c.write("proj/sess-a.jsonl",
		resp{id: "r1", model: "claude-opus-5-5", at: t0, w5: 50, stop: "tool_use"}.line("text"),                        // first: no predecessor
		resp{id: "r2", model: "claude-opus-5-5", at: t0.Add(4 * oneMinute), w5: 200, stop: "tool_use"}.line("text"),    // 4m: warm
		resp{id: "r3", model: "claude-opus-5-5", at: t0.Add(10 * oneMinute), w5: 200, stop: "tool_use"}.line("text"),   // 6m: 5m write cold
		resp{id: "r4", model: "claude-opus-5-5", at: t0.Add(30 * oneMinute), w1h: 400, stop: "tool_use"}.line("text"),  // 20m: 1h write warm
		resp{id: "r5", model: "claude-opus-5-5", at: t0.Add(150 * oneMinute), w1h: 400, stop: "end_turn"}.line("text")) // 2h: 1h write cold
	// Two hours after the main transcript's last response: a subagent's first
	// write. If files were merged it would read as a cold 5m write.
	c.write("proj/sess-a/subagents/agent-a1.jsonl",
		resp{id: "s1", model: "claude-opus-5-5", at: t0.Add(270 * oneMinute), w5: 1000, stop: "end_turn", sidechain: true}.line("text"))
	// And a second subagent's first write, fifteen minutes after the first
	// subagent's: another file, another cold start, never an expiry.
	c.write("proj/sess-a/subagents/agent-a2.jsonl",
		resp{id: "s2", model: "claude-opus-5-5", at: t0.Add(285 * oneMinute), w5: 2000, stop: "end_turn", sidechain: true}.line("text"))

	s := c.summary(30)
	if s.CacheExpiry.Responses != 2 || s.CacheExpiry.Tokens != 600 {
		t.Errorf("cold = %d responses, %d tokens; want 2 and 600 (r3's 5m write, r5's 1h write)",
			s.CacheExpiry.Responses, s.CacheExpiry.Tokens)
	}
	if want := int64(200*(opusW5-opusRead) + 400*(opusW1h-opusRead)); s.CacheExpiry.Cost.Nano != want {
		t.Errorf("cold cost = %d, want %d: the writes over a cache read of the same tokens", s.CacheExpiry.Cost.Nano, want)
	}
	txt, _ := render(t, s)
	if !strings.Contains(txt, "heuristic") {
		t.Errorf("the cache expiry figure is not labelled a heuristic:\n%s", txt)
	}
}

// TestCacheExpiry_ThePreviousResponseIsTheSameAgents: a main transcript also
// carries its subagents' sidechain lines. Taken as one stream, a subagent's
// response during the main agent's pause made the main agent's next 1h write
// look warm (20 minutes after the subagent) and the subagent's own first
// write look like an expiry (50 minutes after the main agent): the cold
// figure named the wrong write. By agent, the main agent's write follows its
// own previous response by 70 minutes, and the subagent's first write
// follows nothing.
func TestCacheExpiry_ThePreviousResponseIsTheSameAgents(t *testing.T) {
	c := newConfig(t)
	t0 := now.Add(-5 * time.Hour)
	c.write("proj/sess-a.jsonl",
		resp{id: "m1", model: "claude-opus-5-5", at: t0, w1h: 300, stop: "tool_use"}.line("text"),
		resp{id: "s1", model: "claude-opus-5-5", at: t0.Add(50 * oneMinute), w5: 1000, stop: "end_turn", sidechain: true}.line("text"),
		resp{id: "m2", model: "claude-opus-5-5", at: t0.Add(70 * oneMinute), w1h: 300, stop: "end_turn"}.line("text"))
	s := c.summary(30)
	if s.CacheExpiry.Responses != 1 || s.CacheExpiry.Tokens1h != 300 || s.CacheExpiry.Tokens5m != 0 {
		t.Errorf("cold = %+v; want m2's 1h write alone", s.CacheExpiry)
	}
}

// TestCacheExpiry_IsAFigureNotASaving: the re-write figure is a heuristic
// that errs low, so it is shown as a figure and offered as no saving, with
// no TTL advice: on real data the re-writes were already 1h writes after
// gaps of hours to days, and advice no figure supports is not given.
func TestCacheExpiry_IsAFigureNotASaving(t *testing.T) {
	c := newConfig(t)
	t0 := now.Add(-10 * time.Hour)
	c.write("proj/sess-a.jsonl",
		resp{id: "a", model: "claude-opus-5-5", at: t0, w1h: 100, stop: "end_turn"}.line("text"),
		resp{id: "b", model: "claude-opus-5-5", at: t0.Add(3 * time.Hour), w1h: 50000, stop: "end_turn"}.line("text"))
	c.write("proj/sess-b.jsonl",
		resp{id: "c", model: "claude-opus-5-5", at: t0, w5: 100, stop: "end_turn"}.line("text"),
		resp{id: "d", model: "claude-opus-5-5", at: t0.Add(20 * oneMinute), w5: 50000, stop: "end_turn"}.line("text"))
	s := c.summary(30)
	if s.CacheExpiry.Responses != 2 {
		t.Fatalf("premise: cold = %+v, want b's and d's re-writes", s.CacheExpiry)
	}
	txt, js := render(t, s)
	if len(s.Savings) != 0 || strings.Contains(txt, "savings") || !strings.Contains(js, `"savings":[]`) {
		t.Errorf("the re-write figure is offered as a saving: %+v\n%s", s.Savings, txt)
	}
	for _, bad := range []string{"1h TTL", "keep the session warm", "went cold"} {
		if strings.Contains(txt, bad) {
			t.Errorf("text says %q about a heuristic re-write figure:\n%s", bad, txt)
		}
	}
	if !strings.Contains(txt, "re-written after a gap longer than its TTL (heuristic: ") {
		t.Errorf("text does not call it a heuristic re-write after a gap:\n%s", txt)
	}
}

// TestCacheExpiry_ThePredecessorMayLieOutsideTheWindow: the window is
// applied after ordering, or the first response inside it loses the gap.
func TestCacheExpiry_ThePredecessorMayLieOutsideTheWindow(t *testing.T) {
	c := newConfig(t)
	c.write("proj/sess-a.jsonl",
		resp{id: "r1", model: "claude-opus-5-5", at: now.Add(-3 * 24 * time.Hour), w5: 100, stop: "end_turn"}.line("text"),
		resp{id: "r2", model: "claude-opus-5-5", at: now.Add(-time.Hour), w5: 100, stop: "end_turn"}.line("text"))
	if s := c.summary(1); s.CacheExpiry.Tokens != 100 {
		t.Errorf("cold tokens = %d, want 100", s.CacheExpiry.Tokens)
	}
}

// TestCacheExpiry_AnUnbilledLineWarmsNothing: Claude Code writes a
// zero-usage "<synthetic>" line for a local error. Nothing was sent for it,
// so it refreshed no cache and is not a predecessor: the write after it
// still follows twenty minutes with no billed request.
func TestCacheExpiry_AnUnbilledLineWarmsNothing(t *testing.T) {
	c := newConfig(t)
	t0 := now.Add(-time.Hour)
	c.write("proj/sess-a.jsonl",
		resp{id: "a", model: "claude-opus-5-5", at: t0, w5: 50000, stop: "end_turn"}.line("text"),
		resp{id: "syn", model: "<synthetic>", at: t0.Add(20 * oneMinute), stop: "end_turn"}.line("text"),
		resp{id: "b", model: "claude-opus-5-5", at: t0.Add(20*oneMinute + time.Second), w5: 50000, stop: "end_turn"}.line("text"))
	s := c.summary(30)
	if s.CacheExpiry.Responses != 1 || s.CacheExpiry.Cost.Nano != 50000*(opusW5-opusRead) {
		t.Errorf("cold = %d responses, %d nanodollars; want b's write, 1 and %d",
			s.CacheExpiry.Responses, s.CacheExpiry.Cost.Nano, 50000*(opusW5-opusRead))
	}
}

// TestCacheExpiry_AWriteOnAWarmCacheIsNotCold: a response that read back
// everything the previous one cached found it warm, whatever the gap before
// it, and its write only added the tokens after the cached prefix. Counting
// it whole priced a 1k write on a warm cache as an expiry. And a cold write
// is priced as the write over a cache read of the same tokens -- the
// alternative was a read, not nothing -- so the full write rate overstated
// the figure. The cold write re-writes all 6k the warm response cached: a
// smaller request is not the same prompt, and is never counted.
func TestCacheExpiry_AWriteOnAWarmCacheIsNotCold(t *testing.T) {
	c := newConfig(t)
	t0 := now.Add(-2 * time.Hour)
	c.write("proj/sess-a.jsonl",
		resp{id: "a", model: "claude-opus-5-5", at: t0, w5: 100, stop: "end_turn"}.line("text"),
		resp{id: "warm", model: "claude-opus-5-5", at: t0.Add(20 * oneMinute), read: 5000, w5: 1000, stop: "end_turn"}.line("text"),
		resp{id: "cold", model: "claude-opus-5-5", at: t0.Add(40 * oneMinute), w5: 6000, stop: "end_turn"}.line("text"))
	s := c.summary(30)
	if s.CacheExpiry.Responses != 1 || s.CacheExpiry.Tokens != 6000 {
		t.Errorf("cold = %d responses, %d tokens; want the write that read nothing, 1 and 6000",
			s.CacheExpiry.Responses, s.CacheExpiry.Tokens)
	}
	if want := int64(6000 * (opusW5 - opusRead)); s.CacheExpiry.Cost.Nano != want {
		t.Errorf("cold cost = %d, want %d: the write rate minus the read rate", s.CacheExpiry.Cost.Nano, want)
	}
	txt, js := render(t, s)
	if !strings.Contains(txt, "over cache reads") || !strings.Contains(js, "counting only the shortfall") {
		t.Errorf("the figure does not say it is the write over a cache read, counting only the shortfall:\n%s\n%s", txt, js)
	}
}

// TestCacheExpiry_APartialExpiryIsCounted: a response can read a prefix that
// is still warm -- a 1h breakpoint, or one a parallel session kept warm --
// and re-write the expired rest of the conversation. Any cache read made a
// write not cold, so that re-write was never counted, and it is the case the
// 1h-TTL advice is about. Here the previous response read a 20k prefix and
// cached 100k after it; twenty minutes later the 20k prefix is read again and
// the expired 100k is re-written: the shortfall, 100k, is cold. A write
// beyond the previous response's cache is new content, not an expiry.
func TestCacheExpiry_APartialExpiryIsCounted(t *testing.T) {
	c := newConfig(t)
	t0 := now.Add(-2 * time.Hour)
	c.write("proj/sess-a.jsonl",
		resp{id: "a", model: "claude-opus-5-5", at: t0, read: 20000, w5: 100000, stop: "end_turn"}.line("text"),
		resp{id: "b", model: "claude-opus-5-5", at: t0.Add(20 * oneMinute), read: 20000, w5: 100000 + 3000, stop: "end_turn"}.line("text"))
	s := c.summary(30)
	if s.CacheExpiry.Responses != 1 || s.CacheExpiry.Tokens5m != 100000 {
		t.Errorf("cold = %+v; want b's re-write of the expired 100k, and not its 3k of new content", s.CacheExpiry)
	}
	if want := int64(100000 * (opusW5 - opusRead)); s.CacheExpiry.Cost.Nano != want {
		t.Errorf("cold cost = %d, want %d", s.CacheExpiry.Cost.Nano, want)
	}
}

// TestCacheExpiry_AWriteOfUnknownCostIsNotPriced: a refusal before any
// output re-writes a cache that expired. Its cost is unknown -- whether it
// was billed depends on its category -- so its cold write is counted in
// tokens with the cost unknown, never priced at its model's rates.
func TestCacheExpiry_AWriteOfUnknownCostIsNotPriced(t *testing.T) {
	c := newConfig(t)
	t0 := now.Add(-2 * time.Hour)
	c.write("proj/sess-a.jsonl",
		resp{id: "a", model: "claude-opus-5-5", at: t0, w5: 1000, stop: "end_turn"}.line("text"),
		resp{id: "b", model: "claude-opus-5-5", at: t0.Add(20 * oneMinute), w5: 1000, stop: "refusal", category: "bio"}.line("text"))
	s := c.summary(30)
	if s.Refusals.BeforeOutput != 1 {
		t.Fatalf("premise: refusals = %+v, want b before any output", s.Refusals)
	}
	if want := (Cost{Unpriced: 1, UnpricedTokens: 1000}); s.CacheExpiry.Responses != 1 || s.CacheExpiry.Cost != want {
		t.Errorf("cold = %+v; want b's 1,000-token write with the cost unknown, %+v", s.CacheExpiry, want)
	}
}

// TestCacheExpiry_NewContentIsNotARewrite: the shortfall rule assumes each
// request's prompt extends the previous one, so a write up to the shortfall
// re-writes what the previous response cached. That is false after
// compaction, after a model switch (opusplan switches within a session, and
// one model's cache never held the other's conversation), and when a second
// sidechain agent shares the main file's stream: each wrote new content, and
// each was counted cold. A response on another model than the previous one,
// or whose whole prompt is smaller than what the previous one cached, is not
// judged: the figure errs low.
func TestCacheExpiry_NewContentIsNotARewrite(t *testing.T) {
	t0 := now.Add(-2 * time.Hour)
	at10 := t0.Add(10 * oneMinute)
	for _, tc := range []struct {
		name  string
		lines []string
	}{
		{"compaction after 10m, the system prompt read", []string{
			resp{id: "a", model: "claude-opus-5-5", at: t0, read: 50000, w5: 2000, stop: "end_turn"}.line("text"),
			resp{id: "b", model: "claude-opus-5-5", at: at10, in: 100, read: 3000, w5: 9000, stop: "end_turn"}.line("text")}},
		{"a model switch after 10m, the new model's system prompt warm", []string{
			resp{id: "a", model: "claude-opus-5-5", at: t0, read: 80000, w5: 2000, stop: "end_turn"}.line("text"),
			resp{id: "b", model: "claude-sonnet-4-6", at: at10, read: 3000, w5: 82000, stop: "end_turn"}.line("text")}},
		{"two sidechain agents in one main file", []string{
			resp{id: "a", model: "claude-opus-5-5", at: t0, read: 40000, w5: 1000, stop: "end_turn", sidechain: true}.line("text"),
			resp{id: "b", model: "claude-opus-5-5", at: at10, read: 3000, w5: 5000, stop: "end_turn", sidechain: true}.line("text")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newConfig(t)
			c.write("proj/sess-a.jsonl", tc.lines...)
			if s := c.summary(30); s.CacheExpiry.Responses != 0 || s.CacheExpiry.Tokens != 0 {
				t.Errorf("cold = %+v, want 0 tokens: b wrote new content", s.CacheExpiry)
			}
		})
	}
}

// TestCacheExpiry_TheCheaperTTLIsColdFirst pins the split of a cold write by
// TTL: the shortfall goes to the 5m write first and the 1h write only after
// it, so the figure errs low. The predecessor cached 1,000 tokens; two hours
// later a response reading none writes 1,000 at each TTL: the 1,000 cold
// tokens are the 5m write's.
func TestCacheExpiry_TheCheaperTTLIsColdFirst(t *testing.T) {
	c := newConfig(t)
	t0 := now.Add(-3 * time.Hour)
	c.write("proj/sess-a.jsonl",
		resp{id: "a", model: "claude-opus-5-5", at: t0, w5: 1000, stop: "end_turn"}.line("text"),
		resp{id: "b", model: "claude-opus-5-5", at: t0.Add(2 * time.Hour), w5: 1000, w1h: 1000, stop: "end_turn"}.line("text"))
	s := c.summary(30)
	if e := s.CacheExpiry; e.Responses != 1 || e.Tokens5m != 1000 || e.Tokens1h != 0 {
		t.Errorf("cold = %+v; want 1,000 tokens of b's 5m write and none of its 1h write", e)
	}
}

// TestCacheExpiry_AResponseIsJudgedInTheFileItWasFirstSeenIn: a copied
// transcript can hold a response without the predecessor it had where it was
// first written. In proj1, B follows A by a minute: warm. A copy in proj2
// holds B after only X, an hour earlier. B is judged once, in proj1, so no
// write is cold; judged in every file, the copy's gap made it cold.
func TestCacheExpiry_AResponseIsJudgedInTheFileItWasFirstSeenIn(t *testing.T) {
	c := newConfig(t)
	t0 := now.Add(-2 * time.Hour)
	a := resp{id: "A", model: "claude-opus-5-5", at: t0, w5: 1000, stop: "end_turn"}
	b := resp{id: "B", model: "claude-opus-5-5", at: t0.Add(oneMinute), w5: 1000, stop: "end_turn"}
	x := resp{id: "X", model: "claude-opus-5-5", at: t0.Add(-time.Hour), w5: 1000, stop: "end_turn"}
	c.write("proj1/sess-a.jsonl", a.line("text"), b.line("text"))
	c.write("proj2/sess-b.jsonl", x.line("text"), b.line("text"))
	if s := c.summary(30); s.CacheExpiry.Responses != 0 {
		t.Errorf("cold = %+v, want none: B followed A by a minute where it was written", s.CacheExpiry)
	}
}

// TestNoSavingsWithoutAFigure: nothing cold, nothing failed, no suggestion.
func TestNoSavingsWithoutAFigure(t *testing.T) {
	c := newConfig(t)
	c.write("proj/sess-a.jsonl", resp{id: "r", model: "claude-opus-5-5", at: now.Add(-time.Hour), in: 1e6, stop: "end_turn"}.line("text"))
	s := c.summary(30)
	txt, _ := render(t, s)
	if len(s.Savings) != 0 || strings.Contains(txt, "savings") {
		t.Errorf("a suggestion was made with no figure under it: %+v\n%s", s.Savings, txt)
	}
}

// TestRefusalsAndExtraAttempts: a refusal that produced output is counted
// and priced like any response; the extra attempts are counted in TOKENS,
// with the dollars unknown, left out of the total, and said to be. Neither is
// a saving.
func TestRefusalsAndExtraAttempts(t *testing.T) {
	c := newConfig(t)
	at := now.Add(-time.Hour)
	declined1 := resp{in: 100, out: 10}
	declined2 := resp{in: 200, out: 20, read: 5}
	final := resp{in: 1000, out: 300}
	c.write("proj/sess-a.jsonl",
		resp{id: "ref", model: "claude-fable-5-1", at: at, in: 1000, out: 40, stop: "refusal"}.line("text"),
		resp{id: "fb", model: "claude-opus-5-5", at: at, in: 1000, out: 300, stop: "end_turn",
			iters: []resp{declined1, declined2, final}}.line("text"),
		resp{id: "one", model: "claude-opus-5-5", at: at, in: 1, stop: "end_turn", iters: []resp{{in: 1}}}.line("text"))

	s := c.summary(30)
	if s.Refusals.Responses != 1 {
		t.Errorf("refusals = %d, want 1", s.Refusals.Responses)
	}
	if want := int64(1000*10000 + 40*50000); s.Refusals.Cost.Nano != want {
		t.Errorf("refusal cost = %d, want %d at Fable 5.1's rates", s.Refusals.Cost.Nano, want)
	}
	if s.ExtraAttempts.Responses != 1 || s.ExtraAttempts.Attempts != 2 {
		t.Errorf("extra attempts = %+v, want 1 response with 2 extra attempts", s.ExtraAttempts)
	}
	if got := s.ExtraAttempts.Tokens.Total(); got != 100+10+200+20+5 {
		t.Errorf("extra-attempt tokens = %d, want %d: the earlier attempts only, never the final one the top level already counts",
			got, 100+10+200+20+5)
	}
	// The total, its tokens and every breakdown are the returned attempts
	// only: the extra attempts' output does not make them priced.
	if want := int64(1000*10000+40*50000) + int64(1000*opusIn+300*opusOut) + int64(1*opusIn); s.Total.Nano != want || !s.Total.Known() {
		t.Errorf("total = %+v, want %d", s.Total, want)
	}
	if got := s.Tokens.Total(); got != 1040+1300+1 {
		t.Errorf("token total = %d, want %d: the extra attempts are out of it", got, 1040+1300+1)
	}
	for _, m := range s.ByModel {
		if m.Model == "claude-opus-5-5" && m.Tokens.Total() != 1301 {
			t.Errorf("claude-opus-5-5 row = %+v, want the returned attempts' 1,301 tokens alone", m)
		}
	}
	txt, js := render(t, s)
	if !strings.Contains(txt, "1 response ended in a refusal, $0.01") {
		t.Errorf("text does not carry the refusal line:\n%s", txt)
	}
	if !strings.Contains(txt, "\nretries       1 response carried 2 extra attempts, 335 tokens with the cost unknown, not in the total\n") {
		t.Errorf("text does not carry the extra attempts in tokens with the cost unknown:\n%s", txt)
	}
	// The headline's total leaves the extra attempts out, and says so: a
	// wholly known "est. $X" beside tokens nobody priced read as complete.
	if !strings.Contains(txt, "\n       the total leaves out 335 tokens on 2 extra attempts (cost unknown)\n") {
		t.Errorf("the headline does not say the total excludes the extra attempts:\n%s", txt)
	}
	if !strings.Contains(js, `"extra_attempts":{"responses":1,"attempts":2,"tokens":{"input":300,"output":30,"cache_read":5,"cache_write_5m":0,"cache_write_1h":0},"fallback_served":0}`) {
		t.Errorf("the JSON does not carry the extra attempts as tokens:\n%s", js)
	}
	if len(s.Savings) != 0 || strings.Contains(txt, "savings") {
		t.Errorf("a refusal or an extra attempt is offered as a saving: %+v\n%s", s.Savings, txt)
	}
}

// fallbackExample is the refusals-and-fallback page's own example of a
// response a fallback served, verbatim
// (https://platform.claude.com/docs/en/build-with-claude/refusals-and-fallback,
// "What the response contains"): Fable 5 declined before any output, and
// default routing served the turn on Opus 4.8. No real transcript with a
// fallback has been captured yet; when one is, it belongs beside this.
const fallbackExample = `{
  "id": "msg_01XFUDYJgAACzvnptvVoYEL",
  "type": "message",
  "role": "assistant",
  "model": "claude-opus-4-8",
  "content": [
    {
      "type": "fallback",
      "from": { "model": "claude-fable-5" },
      "to": { "model": "claude-opus-4-8" }
    },
    { "type": "text", "text": "Hi! How can I help you today?" }
  ],
  "stop_reason": "end_turn",
  "stop_details": null,
  "usage": {
    "input_tokens": 412,
    "output_tokens": 264,
    "cache_read_input_tokens": 0,
    "cache_creation_input_tokens": 0,
    "iterations": [
      {
        "type": "message",
        "model": "claude-fable-5",
        "input_tokens": 535,
        "output_tokens": 0,
        "cache_read_input_tokens": 0,
        "cache_creation_input_tokens": 0
      },
      {
        "type": "fallback_message",
        "model": "claude-opus-4-8",
        "input_tokens": 412,
        "output_tokens": 264,
        "cache_read_input_tokens": 0,
        "cache_creation_input_tokens": 0
      }
    ]
  }
}`

// transcriptLine wraps an API message as Claude Code writes it into a
// transcript, optionally editing it first.
func transcriptLine(t *testing.T, msg string, at time.Time, edit func(map[string]any)) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(msg), &m); err != nil {
		t.Fatal(err)
	}
	if edit != nil {
		edit(m)
	}
	b, err := json.Marshal(map[string]any{
		"type": "assistant", "sessionId": "sess-a", "isSidechain": false,
		"timestamp": at.UTC().Format(time.RFC3339Nano), "message": m,
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// iterationsOf is the example's usage.iterations, to edit.
func iterationsOf(m map[string]any) []any {
	return m["usage"].(map[string]any)["iterations"].([]any)
}

// Opus 4.8's and Fable 5's rates in nanodollars per token, from the design's
// table ($5 / $25 and $10 / $50 per MTok).
const (
	opus48In   = 5000
	opus48Out  = 25000
	fable5In   = 10000
	fable5Out  = 50000
	fable5Read = 1000
)

// TestExtraAttempts_TheFallbackPagesExample: the attempt Fable 5 declined is
// tokens with the cost unknown, left out of the total, and said to be --
// whether it produced output or not -- and the response counts as one a
// fallback model served, priced at the model that served it.
func TestExtraAttempts_TheFallbackPagesExample(t *testing.T) {
	served := int64(412*opus48In + 264*opus48Out)
	for _, tc := range []struct {
		name   string
		edit   func(map[string]any)
		tokens int64
	}{
		{"as the page has it", nil, 535},
		{"the declined attempt with output", func(m map[string]any) {
			first := iterationsOf(m)[0].(map[string]any)
			first["output_tokens"] = 50
			first["cache_read_input_tokens"] = 100
		}, 685},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newConfig(t)
			c.write("proj/sess-a.jsonl", transcriptLine(t, fallbackExample, now.Add(-time.Hour), tc.edit))
			s := c.summary(30)
			if s.Total.Nano != served || !s.Total.Known() || s.Tokens.Total() != 412+264 {
				t.Errorf("total = %+v over %d tokens, want %d over 676: the served attempt alone", s.Total, s.Tokens.Total(), served)
			}
			e := s.ExtraAttempts
			if e.Responses != 1 || e.Attempts != 1 || e.Tokens.Total() != tc.tokens || e.FallbackServed != 1 {
				t.Errorf("extra attempts = %+v, want the one declined attempt, %d tokens, and the fallback served", e, tc.tokens)
			}
			if len(s.ByModel) != 1 || s.ByModel[0].Model != "claude-opus-4-8" || s.ByModel[0].Cost.Nano != served {
				t.Errorf("by model = %+v, want claude-opus-4-8 alone at %d", s.ByModel, served)
			}
			txt, js := render(t, s)
			for _, want := range []string{
				fmt.Sprintf("\n       the total leaves out %s tokens on 1 extra attempt (cost unknown)\n", thousands(tc.tokens)),
				fmt.Sprintf("\nretries       1 response carried 1 extra attempt, %s tokens with the cost unknown, not in the total; 1 response was served by a fallback model\n", thousands(tc.tokens)),
				"\nby model      claude-opus-4-8 <$0.01\n",
			} {
				if !strings.Contains(txt, want) {
					t.Errorf("text lacks %q:\n%s", want, txt)
				}
			}
			for name, out := range map[string]string{"text": txt, "json": js} {
				if strings.Contains(out, "How can I help") || strings.Contains(out, "claude-fable-5") {
					t.Errorf("the example's content, or the declined attempt's model, reached the %s output:\n%s", name, out)
				}
			}
		})
	}

	// A refusal served the same way: it produced output, so it is priced
	// like any response, at the model that served it, and nothing was served.
	c := newConfig(t)
	c.write("proj/sess-a.jsonl", transcriptLine(t, fallbackExample, now.Add(-time.Hour), func(m map[string]any) {
		m["stop_reason"] = "refusal"
		m["stop_details"] = map[string]any{"type": "refusal", "category": "cyber"}
		iterationsOf(m)[0].(map[string]any)["output_tokens"] = 50
	}))
	s := c.summary(30)
	want := Cost{Nano: served, Priced: 1}
	if s.Refusals.Cost != want || s.Total != want || s.ExtraAttempts.FallbackServed != 0 {
		t.Errorf("refusals = %+v, total = %+v, fallback served %d; want the served response alone, %+v, and none served",
			s.Refusals, s.Total, s.ExtraAttempts.FallbackServed, want)
	}
}

// TestExtraAttempts_AStickyRoutedResponseIsReported: after a fallback, the
// API sends later turns of the conversation straight to the fallback model.
// Such a response carries only a "fallback_message" entry. It was served by a
// fallback, and is counted as one, with no extra attempt.
func TestExtraAttempts_AStickyRoutedResponseIsReported(t *testing.T) {
	c := newConfig(t)
	c.write("proj/sess-a.jsonl", transcriptLine(t, fallbackExample, now.Add(-time.Hour), func(m map[string]any) {
		u := m["usage"].(map[string]any)
		u["iterations"] = iterationsOf(m)[1:]
	}))
	s := c.summary(30)
	if e := s.ExtraAttempts; e.FallbackServed != 1 || e.Responses != 0 {
		t.Errorf("extra attempts = %+v, want no extra attempt and one response a fallback served", e)
	}
	txt, _ := render(t, s)
	if !strings.Contains(txt, "\nretries       none (no response carried more than one attempt); 1 response was served by a fallback model\n") ||
		strings.Contains(txt, "leaves out") {
		t.Errorf("text does not report the sticky-routed response:\n%s", txt)
	}
}

// TestRefusals_APreOutputRefusalWithoutUsageIsCounted: a zero-usage refusal
// line that reports no response with usage (refusalMessage) was dropped by
// Build with every zero-token response -- so a transcript holding a refusal
// printed "refusals none". It is counted, a count only, and the header says
// the total leaves it out. With no other response, the line is still
// printed.
func TestRefusals_APreOutputRefusalWithoutUsageIsCounted(t *testing.T) {
	at := now.Add(-time.Hour)
	synthetic := func(id string) string { return refusalMessage(id, "", "", at) }
	for _, tc := range []struct {
		name  string
		lines []string
		n     int
		want  []string
	}{
		{"beside a billed response", []string{
			resp{id: "r", model: "claude-opus-5-5", at: at, in: 10, stop: "end_turn"}.line("text"), synthetic("z1"), synthetic("z2")}, 2,
			[]string{
				"\n       the total leaves out 2 pre-output refusals written without usage (cost unknown: whether a refusal before any output was billed depends on its category)\n",
				"refusals      2 pre-output refusals were written without usage\n" +
					"              uncategorized (model not recorded): 2 without usage\n"}},
		{"alone", []string{synthetic("z1")}, 1,
			[]string{
				"\n       the total leaves out 1 pre-output refusal written without usage (cost unknown: whether a refusal before any output was billed depends on its category)\n",
				"refusals      1 pre-output refusal was written without usage\n" +
					"              uncategorized (model not recorded): 1 without usage\n"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newConfig(t)
			c.write("proj/sess-a.jsonl", tc.lines...)
			s := c.summary(30)
			txt, js := render(t, s)
			for _, want := range tc.want {
				if !strings.Contains(txt, want) || strings.Contains(txt, "refusals      none") {
					t.Errorf("text lacks %q:\n%s", want, txt)
				}
			}
			if !strings.Contains(js, fmt.Sprintf(`"without_usage":%d`, tc.n)) || s.Refusals.WithoutUsage != tc.n {
				t.Errorf("without_usage = %d:\n%s", s.Refusals.WithoutUsage, js)
			}
			if strings.Contains(txt, "savings") {
				t.Errorf("the savings output names a refusal:\n%s", txt)
			}
		})
	}
}

// TestExtraAttempts_TheServedModelIsTheFallbackEntrys: for a mid-output
// fallback the page says message_start "already named the requested model,
// so read the serving model from the fallback block's to.model and from the
// fallback_message entry in the final message_delta's usage.iterations". A
// streamed line whose message.model is the model asked priced the served
// attempt at that model's rates and showed no Opus 4.8 row. The served model
// is the last entry's when it names one.
func TestExtraAttempts_TheServedModelIsTheFallbackEntrys(t *testing.T) {
	c := newConfig(t)
	c.write("proj/sess-a.jsonl", transcriptLine(t, fallbackExample, now.Add(-time.Hour), func(m map[string]any) {
		m["model"] = "claude-fable-5"
		iterationsOf(m)[0].(map[string]any)["output_tokens"] = 50
	}))
	s := c.summary(30)
	served := int64(412*opus48In + 264*opus48Out)
	if s.Total.Nano != served || s.Total.Nano != 8_660_000 {
		t.Errorf("total = %d, want %d: the served attempt at Opus 4.8's rates", s.Total.Nano, served)
	}
	if len(s.ByModel) != 1 || s.ByModel[0].Model != "claude-opus-4-8" || s.ByModel[0].Cost.Nano != served {
		t.Errorf("by model = %+v, want the served attempt under claude-opus-4-8", s.ByModel)
	}
	if s.ExtraAttempts.FallbackServed != 1 {
		t.Errorf("fallback served = %d, want 1", s.ExtraAttempts.FallbackServed)
	}
}

// TestExtraAttempts_AModelLessFallbackLineKeepsAKnownModel: keep took the
// model of any fallback line it kept, so a completed line naming no model --
// no message.model, and a fallback_message entry with none -- replaced the
// model the streamed line before it named, and the response was priced as
// unknown. A line that names no model never replaces one.
func TestExtraAttempts_AModelLessFallbackLineKeepsAKnownModel(t *testing.T) {
	c := newConfig(t)
	at := now.Add(-time.Hour)
	partial := resp{id: "msg_f", model: "claude-opus-4-8", at: at, in: 412, out: 4}
	done := resp{id: "msg_f", at: at, in: 412, out: 264, stop: "end_turn",
		iters: []resp{{model: "claude-fable-5", in: 535}, {typ: "fallback_message", in: 412, out: 264}}}
	c.write("proj/sess-a.jsonl", partial.line("thinking"), done.line("text"))
	s := c.summary(30)
	if want := (Cost{Nano: 412*opus48In + 264*opus48Out, Priced: 1}); s.Total != want {
		t.Errorf("total = %+v, want %+v: the response at the model its earlier line named", s.Total, want)
	}
	if !hasModel(s, "claude-opus-4-8") || s.ExtraAttempts.FallbackServed != 1 || s.Tokens.Output != 264 {
		t.Errorf("by model = %+v, extra attempts = %+v, tokens = %+v; want claude-opus-4-8, served by a fallback, from the completed line",
			s.ByModel, s.ExtraAttempts, s.Tokens)
	}
}

// TestExtraAttempts_AnAllDeclinedChainIsNotServed: when every model in the
// chain declines, the page says the response is the last model's refusal,
// with a fallback_message entry last. It was reported as served. Nothing was
// served; the earlier attempt is an extra attempt in tokens.
func TestExtraAttempts_AnAllDeclinedChainIsNotServed(t *testing.T) {
	c := newConfig(t)
	c.write("proj/sess-a.jsonl", transcriptLine(t, fallbackExample, now.Add(-time.Hour), func(m map[string]any) {
		m["stop_reason"] = "refusal"
		m["stop_details"] = map[string]any{"type": "refusal", "category": "bio"}
		iterationsOf(m)[0].(map[string]any)["output_tokens"] = 50
	}))
	s := c.summary(30)
	if e := s.ExtraAttempts; e.FallbackServed != 0 || e.Attempts != 1 || e.Tokens.Total() != 585 {
		t.Errorf("extra attempts = %+v, want none served and the 585-token declined attempt", e)
	}
	txt, _ := render(t, s)
	if !strings.Contains(txt, "\nretries       1 response carried 1 extra attempt, 585 tokens with the cost unknown, not in the total\n") ||
		strings.Contains(txt, "served by a fallback") {
		t.Errorf("an all-declined chain reads as served:\n%s", txt)
	}
}

// refusalMessage is the zero-usage line Claude Code writes after a refusal
// with no fallback: model "<synthetic>", stop_reason "refusal", the
// response's stop_details, and the same requestId as the real response. The
// shape is Claude Code's own refusal-message builder's, read from Claude Code
// 2.1.280's compiled code (no transcript with one has been captured yet); it
// writes this line after a mid-stream refusal too, not only before any
// output.
func refusalMessage(id, requestID, category string, at time.Time) string {
	return resp{id: id, model: "<synthetic>", at: at, stop: "refusal", category: category, requestID: requestID}.line("text")
}

// TestRefusals_AMidStreamRefusalIsCountedOnce: a mid-stream refusal is the
// response that streamed, with its usage, and then Claude Code's zero-usage
// "<synthetic>" line for the same request. Every zero-usage refusal line was
// counted as a pre-output refusal, so this one refusal printed twice -- once
// priced, once "written without usage", with a not-computed line beside it
// and a bio group "on other" saying the amount was not in the transcript. The
// synthetic line is folded into the response with the same requestId in the
// same file -- also when the streamed line carries no stop_reason of its own
// -- so it is one refusal, priced at the rates of the model that ran it.
func TestRefusals_AMidStreamRefusalIsCountedOnce(t *testing.T) {
	at := now.Add(-time.Hour)
	for _, stop := range []string{"refusal", ""} {
		t.Run("response stop_reason "+stop, func(t *testing.T) {
			c := newConfig(t)
			c.write("proj/sess-a.jsonl",
				resp{id: "msg_r", model: "claude-fable-5-1", at: at, in: 50000, out: 300, stop: stop, category: "bio",
					requestID: "req_011A"}.line("text"),
				refusalMessage("syn_1", "req_011A", "bio", at.Add(time.Second)))
			s := c.summary(30)
			r := s.Refusals
			want := Cost{Nano: 50000*10000 + 300*50000, Priced: 1}
			if r.Responses != 1 || r.WithoutUsage != 0 || r.Cost != want {
				t.Errorf("refusals = %+v; want one refusal at %+v and none without usage", r, want)
			}
			if len(r.ByCategory) != 1 || r.ByCategory[0].Category != "bio" || r.ByCategory[0].Model != "claude-fable-5-1" {
				t.Errorf("by category = %+v, want bio on claude-fable-5-1 alone", r.ByCategory)
			}
			txt, js := render(t, s)
			if strings.Contains(txt, "without usage") || strings.Contains(txt, "leaves out") || !strings.Contains(js, `"without_usage":0`) {
				t.Errorf("the refusal is counted again as a pre-output one:\n%s\n%s", txt, js)
			}
		})
	}
	// Folded into a response with usage and no output whose own line has no
	// stop_reason, the synthetic line makes it a pre-output refusal: its
	// tokens shown with the cost unknown, out of the total. Folding has to
	// come before the pre-output marking, or the response is priced.
	t.Run("a pre-output response with no stop_reason", func(t *testing.T) {
		c := newConfig(t)
		c.write("proj/sess-a.jsonl",
			resp{id: "msg_r", model: "claude-opus-5-5", at: at, in: 5000, requestID: "req_011A"}.line("text"),
			refusalMessage("syn_1", "req_011A", "cyber", at.Add(time.Second)))
		s := c.summary(30)
		r := s.Refusals
		if r.BeforeOutput != 1 || r.BeforeOutputTokens != 5000 || r.Responses != 0 || r.WithoutUsage != 0 {
			t.Errorf("refusals = %+v; want one before any output, 5,000 tokens", r)
		}
		if s.Total != (Cost{}) || s.Responses != 0 {
			t.Errorf("total = %+v over %d responses, want nothing: a pre-output refusal is out of the total", s.Total, s.Responses)
		}
	})
	// A synthetic line whose requestId matches nothing in its own file, or
	// is not a request id at all, is a pre-output refusal of its own.
	c := newConfig(t)
	c.write("proj/sess-a.jsonl",
		resp{id: "msg_r", model: "claude-fable-5-1", at: at, in: 50000, out: 300, stop: "refusal", category: "bio", requestID: "req_011A"}.line("text"),
		resp{id: "msg_q", model: "claude-fable-5-1", at: at, in: 50000, out: 300, stop: "refusal", category: "bio", requestID: "not/a-request-id"}.line("text"),
		refusalMessage("syn_1", "req_011B", "bio", at.Add(time.Second)),
		refusalMessage("syn_2", "not/a-request-id", "bio", at.Add(time.Second)))
	c.write("other/sess-b.jsonl", refusalMessage("syn_3", "req_011A", "bio", at.Add(time.Second)))
	if s := c.summary(30); s.Refusals.Responses != 2 || s.Refusals.WithoutUsage != 3 {
		t.Errorf("refusals = %+v; want 2 with usage and 3 without", s.Refusals)
	}
}

// TestRefusals_APreOutputRefusalIsLeftOutOfTheTotal: whether a refusal
// before any output was billed depends on its category, which this read does
// not price by. Pre-output is read from output_tokens == 0, never from the
// line's shape, and such a refusal -- in any category, a billed one, an
// unbilled one or one this read does not know -- shows its tokens with the
// cost unknown and stays out of the total and every breakdown; the header
// says how many, and why their cost is unknown. A window holding only a
// refusal without usage still says the total leaves it out.
func TestRefusals_APreOutputRefusalIsLeftOutOfTheTotal(t *testing.T) {
	at := now.Add(-time.Hour)
	t.Run("with usage and no output", func(t *testing.T) {
		c := newConfig(t)
		c.write("proj/sess-a.jsonl",
			resp{id: "r", model: "claude-opus-5-5", at: at, in: 10, stop: "end_turn"}.line("text"),
			resp{id: "cy", model: "claude-opus-5-5", at: at, in: 10000, stop: "refusal", category: "cyber"}.line("text"),
			resp{id: "bi", model: "claude-opus-5-5", at: at, in: 2000, read: 34, stop: "refusal", category: "bio"}.line("text"),
			resp{id: "o", model: "claude-opus-5-5", at: at, in: 200, stop: "refusal", category: "a_new_category"}.line("text"))
		s := c.summary(30)
		if s.Total != (Cost{Nano: 10 * opusIn, Priced: 1}) || s.Responses != 1 || s.Tokens.Total() != 10 {
			t.Errorf("total = %+v over %d responses, want %d over 1: a pre-output refusal is in the total", s.Total, s.Responses, 10*opusIn)
		}
		if len(s.ByModel) != 1 || s.ByModel[0].Responses != 1 || s.ByKind.Input != (Cost{Nano: 10 * opusIn, Priced: 1}) {
			t.Errorf("by model = %+v, by kind = %+v: a pre-output refusal is in a breakdown", s.ByModel, s.ByKind)
		}
		r := s.Refusals
		if r.BeforeOutput != 3 || r.BeforeOutputTokens != 12234 || r.Responses != 0 || r.Cost != (Cost{}) {
			t.Errorf("refusals = %+v, want three before any output, 12,234 tokens", r)
		}
		if len(s.Savings) != 0 {
			t.Errorf("savings = %+v, want none", s.Savings)
		}
		txt, _ := render(t, s)
		for _, want := range []string{
			"\n       the total leaves out 12,234 tokens on 3 pre-output refusals (cost unknown: whether a refusal before any output was billed depends on its category)\n",
			"\nrefusals      3 refusals before any output, 12,234 tokens with the cost unknown, not in the total\n",
			"              bio on claude-opus-5-5: 1 before any output\n",
			"              cyber on claude-opus-5-5: 1 before any output\n",
			"              other on claude-opus-5-5: 1 before any output\n",
		} {
			if !strings.Contains(txt, want) {
				t.Errorf("text lacks %q:\n%s", want, txt)
			}
		}
		for _, bad := range []string{"not billed (", "was not billed", "billed before any output in", "billing unknown"} {
			if strings.Contains(txt, bad) {
				t.Errorf("text says %q about a refusal it does not price by category:\n%s", bad, txt)
			}
		}
	})
	t.Run("only a pre-output refusal without usage", func(t *testing.T) {
		c := newConfig(t)
		c.write("proj/sess-a.jsonl", refusalMessage("z1", "", "bio", at))
		txt, _ := render(t, c.summary(30))
		if !strings.Contains(txt, "est. $0.00 at API list prices") ||
			!strings.Contains(txt, "\n       the total leaves out 1 pre-output refusal written without usage (cost unknown: whether a refusal before any output was billed depends on its category)\n") {
			t.Errorf("the header prints $0.00 beside a pre-output refusal with no caveat:\n%s", txt)
		}
	})
}

// TestRefusals_AreSplitByCategoryAndModel: stop_details.category was never
// read, so every refusal landed in one total and one line, and nothing told
// a classifier decline in a named category from the rest. The category is
// read as a closed word -- a null is "uncategorized", a word the page does
// not name is "other" -- and the refusals are counted by category and model.
func TestRefusals_AreSplitByCategoryAndModel(t *testing.T) {
	c := newConfig(t)
	at := now.Add(-time.Hour)
	synthetic := func(id, category string) string { return refusalMessage(id, "", category, at) }
	c.write("proj/sess-a.jsonl",
		resp{id: "f", model: "claude-fable-5-1", at: at, in: 1000, out: 40, stop: "refusal", category: "cyber"}.line("text"),
		resp{id: "o", model: "claude-opus-5-5", at: at, in: 10, stop: "refusal", category: "cyber"}.line("text"),
		synthetic("z1", "bio"), synthetic("z2", "bio"), synthetic("z3", "cyber"), synthetic("z4", ""), synthetic("z5", "a_new_category"),
		synthetic("z6", "frontier_llm"), synthetic("z7", "reasoning_extraction"), synthetic("z8", "general_harms"))
	s := c.summary(30)
	nr := ModelNotRecorded
	want := []RefusalGroup{
		{Category: "bio", Model: nr, WithoutUsage: 2},
		{Category: "cyber", Model: "claude-fable-5-1", Responses: 1},
		{Category: "cyber", Model: "claude-opus-5-5", BeforeOutput: 1},
		{Category: "cyber", Model: nr, WithoutUsage: 1},
		{Category: "frontier_llm", Model: nr, WithoutUsage: 1},
		{Category: "general_harms", Model: nr, WithoutUsage: 1},
		{Category: "other", Model: nr, WithoutUsage: 1},
		{Category: "reasoning_extraction", Model: nr, WithoutUsage: 1},
		{Category: "uncategorized", Model: nr, WithoutUsage: 1},
	}
	if !reflect.DeepEqual(s.Refusals.ByCategory, want) {
		t.Errorf("by category = %+v\nwant %+v", s.Refusals.ByCategory, want)
	}
	txt, js := render(t, s)
	for _, line := range []string{
		"refusals      1 response ended in a refusal, $0.01; 1 refusal before any output, 10 tokens with the cost unknown, not in the total; 8 pre-output refusals were written without usage\n",
		"              bio (model not recorded): 2 without usage\n",
		"              cyber on claude-fable-5-1: 1 response\n",
		"              cyber on claude-opus-5-5: 1 before any output\n",
		"              cyber (model not recorded): 1 without usage\n",
		"              frontier_llm (model not recorded): 1 without usage\n",
		"              general_harms (model not recorded): 1 without usage\n",
		"              other (model not recorded): 1 without usage\n",
		"              reasoning_extraction (model not recorded): 1 without usage\n",
		"              uncategorized (model not recorded): 1 without usage\n",
	} {
		if !strings.Contains(txt, line) {
			t.Errorf("text lacks %q:\n%s", line, txt)
		}
	}
	if !strings.Contains(js, `{"category":"bio","model":"not_recorded","responses":0,"before_output":0,"without_usage":2}`) {
		t.Errorf("the JSON does not count the refusals by category:\n%s", js)
	}
	if strings.Contains(txt+js, "a_new_category") {
		t.Errorf("a category outside the closed vocabulary was printed as read:\n%s\n%s", txt, js)
	}
}

// TestFastMode_IsCountedAndSaidToBePricedAtStandardRates: usage.speed marks a
// fast-mode response, which bills at a premium the table does not hold. The
// price.go rationale said no field identifies fast mode, and such responses
// were priced at standard rates without a word. They still are, and the
// output now counts them and says so.
func TestFastMode_IsCountedAndSaidToBePricedAtStandardRates(t *testing.T) {
	c := newConfig(t)
	at := now.Add(-time.Hour)
	fast := resp{id: "f", model: "claude-opus-5-5", at: at, in: 1000, stop: "end_turn", speed: "fast"}
	partial := fast
	partial.out, partial.stop = 1, ""
	c.write("proj/sess-a.jsonl", partial.line("thinking"), fast.line("text"),
		resp{id: "s", model: "claude-opus-5-5", at: at, in: 1000, stop: "end_turn", speed: "standard"}.line("text"))
	s := c.summary(30)
	if s.FastMode.Responses != 1 {
		t.Errorf("fast-mode responses = %d, want 1", s.FastMode.Responses)
	}
	if s.Total.Nano != 2000*opusIn {
		t.Errorf("total = %d, want %d: fast mode is priced at standard rates", s.Total.Nano, 2000*opusIn)
	}
	txt, js := render(t, s)
	if !strings.Contains(txt, "note: 1 response ran in fast mode, which bills at a premium; it is priced at standard rates") {
		t.Errorf("the text does not say the fast-mode response is priced at standard rates:\n%s", txt)
	}
	if !strings.Contains(js, `"fast_mode":{"responses":1,"pricing":"`+FastModePricing+`"}`) {
		t.Errorf("the JSON does not count fast mode:\n%s", js)
	}
	if !strings.Contains(txt, "web-search fees ($10 per 1,000 searches)") {
		t.Errorf("the out-of-scope line does not name web-search fees at the dated page's rate:\n%s", txt)
	}
}

// TestOutput_StatesTheBasisOfEveryFigure: estimated, at list prices, as of
// the snapshot date, and not what a plan subscriber was charged.
func TestOutput_StatesTheBasisOfEveryFigure(t *testing.T) {
	c := newConfig(t)
	c.write("proj/sess-a.jsonl", resp{id: "r", model: "claude-opus-5-5", at: now.Add(-time.Hour), in: 1e6, stop: "end_turn"}.line("text"))
	txt, js := render(t, c.summary(30))
	for _, want := range []string{"est. $4.00 at API list prices (2026-09-30)", "not billed per token", "not what was charged"} {
		if !strings.Contains(txt, want) {
			t.Errorf("text lacks %q:\n%s", want, txt)
		}
	}
	if !strings.Contains(js, `"snapshot":"2026-09-30"`) || !strings.Contains(js, "not billed per token") {
		t.Errorf("json lacks the snapshot or the plan note:\n%s", js)
	}
}

// TestContentNeverReachesTheOutput: the canary sits in every fixture's text,
// thinking and tool_use blocks and in cwd, and neither rendering carries it.
// It also sits in the decoded fields that are printed only as closed words,
// or not at all: an iteration entry's type and model, and a refusal's
// category (and the explanation beside it, which is not read).
func TestContentNeverReachesTheOutput(t *testing.T) {
	c := newConfig(t)
	at := now.Add(-time.Hour)
	r := resp{id: "msg_1", model: "claude-opus-5-5", at: at, in: 1000, out: 500, w5: 10, stop: "tool_use"}
	c.write("proj/sess-a.jsonl", r.line("thinking"), r.line("text"), r.line("tool_use"),
		resp{id: "msg_2", model: "claude-opus-5-5", at: at.Add(10 * oneMinute), w5: 100, stop: "refusal", category: canary}.line("text"),
		resp{id: "msg_6", model: "arn:aws:bedrock:" + canary, at: at.Add(11 * oneMinute), stop: "refusal", category: "bio-" + canary}.line("text"),
		resp{id: "msg_4", model: "claude-opus-5-5", at: at.Add(20 * oneMinute), in: 10, out: 5, stop: "end_turn", iters: []resp{
			{model: "arn:aws:bedrock:" + canary, in: 4, out: 2},
			{typ: canary, model: "claude-" + canary, in: 3, out: 1},
			{typ: "fallback_message", model: "claude-opus-5-5", in: 10, out: 5}}}.line("text"),
		resp{id: "msg_5", model: "claude-opus-5-5", at: at.Add(30 * oneMinute), in: 10, stop: "end_turn", iters: []resp{
			{typ: "fallback_message", model: "claude-opus-5-5", in: 10}}}.line("text"))
	c.write("proj/sess-a/subagents/agent-1.jsonl",
		resp{id: "msg_3", model: "claude-mystery-1", at: at, in: 7, stop: "end_turn"}.line("tool_use"))
	s := c.summary(30)
	s.SilentFailureTurns = SilentFailureTurns{Store: StoreNone, Transcripts: s.Sessions}
	txt, js := render(t, s)
	for name, out := range map[string]string{"text": txt, "json": js} {
		if strings.Contains(out, canary) {
			t.Errorf("the content canary reached the %s output:\n%s", name, out)
		}
		if strings.Contains(out, "secret-project") {
			t.Errorf("cwd reached the %s output:\n%s", name, out)
		}
	}
}

// TestContentHasNoFieldToLandIn holds the decoded shape: no field anywhere
// in it is tagged "content", so encoding/json skips message.content without
// ever making a value of it. A field added here for any reason turns the
// content read on, and this names the rule it breaks.
//
// Tags alone are not enough: a field under an allowed tag typed as a
// json.RawMessage, an interface or a map holds whatever bytes are there --
// a "message" of type any would hold the whole message, content and all --
// so those kinds fail before the struct walk.
func TestContentHasNoFieldToLandIn(t *testing.T) {
	var walk func(reflect.Type, string)
	seen := map[reflect.Type]bool{}
	raw := reflect.TypeOf(json.RawMessage{})
	unmarshaler := reflect.TypeOf((*json.Unmarshaler)(nil)).Elem()
	textUnmarshaler := reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()
	walk = func(ty reflect.Type, path string) {
		for ty.Kind() == reflect.Pointer || ty.Kind() == reflect.Slice || ty.Kind() == reflect.Array {
			if ty == raw {
				t.Errorf("%s is a json.RawMessage: it holds the bytes it spans as a value", path)
				return
			}
			ty = ty.Elem()
		}
		// A type with its own decoder is handed the raw bytes, whatever its
		// fields say.
		if pt := reflect.PointerTo(ty); pt.Implements(unmarshaler) || pt.Implements(textUnmarshaler) {
			t.Errorf("%s decodes itself (UnmarshalJSON or UnmarshalText): it is handed the bytes it spans", path)
			return
		}
		if k := ty.Kind(); k == reflect.Interface || k == reflect.Map {
			t.Errorf("%s is a %s: it holds whatever it is handed", path, k)
			return
		}
		if ty.Kind() != reflect.Struct || seen[ty] {
			return
		}
		seen[ty] = true
		for i := 0; i < ty.NumField(); i++ {
			f := ty.Field(i)
			tag := strings.Split(f.Tag.Get("json"), ",")[0]
			allowed := map[string]bool{
				"timestamp": true, "sessionId": true, "isSidechain": true, "message": true,
				"id": true, "model": true, "stop_reason": true, "usage": true,
				"input_tokens": true, "output_tokens": true, "cache_read_input_tokens": true,
				"cache_creation_input_tokens": true, "cache_creation": true,
				"ephemeral_5m_input_tokens": true, "ephemeral_1h_input_tokens": true,
				"iterations": true, "speed": true,
				// A refusal's category, read as a closed word
				// (refusalCategory); its explanation is not read.
				"stop_details": true, "category": true,
				// A subagent user line's header: a closed word, a flag and
				// the promptId key that ties a response to its turn.
				"type": true, "isMeta": true, "promptId": true,
				// The request id a "<synthetic>" refusal line shares with
				// the response it reports, read as a closed shape.
				"requestId": true,
			}
			if !f.Anonymous && !allowed[tag] {
				t.Errorf("%s.%s decodes %q, which is outside spend's read path", path, f.Name, tag)
			}
			walk(f.Type, path+"."+f.Name)
		}
	}
	walk(reflect.TypeOf(line{}), "line")
}
