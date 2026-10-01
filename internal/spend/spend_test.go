package spend

import (
	"bytes"
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
		resp{id: "r2", model: "claude-opus-5-5", at: t0.Add(4 * oneMinute), w5: 100, stop: "tool_use"}.line("text"),    // 4m: warm
		resp{id: "r3", model: "claude-opus-5-5", at: t0.Add(10 * oneMinute), w5: 200, stop: "tool_use"}.line("text"),   // 6m: 5m write cold
		resp{id: "r4", model: "claude-opus-5-5", at: t0.Add(30 * oneMinute), w1h: 300, stop: "tool_use"}.line("text"),  // 20m: 1h write warm
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
	if len(s.Savings) != 1 || s.Savings[0].Kind != SavingColdCache {
		t.Errorf("savings = %+v, want the cold-cache line resting on its figure", s.Savings)
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
		resp{id: "m1", model: "claude-opus-5-5", at: t0, in: 1, stop: "tool_use"}.line("text"),
		resp{id: "s1", model: "claude-opus-5-5", at: t0.Add(50 * oneMinute), w5: 1000, stop: "end_turn", sidechain: true}.line("text"),
		resp{id: "m2", model: "claude-opus-5-5", at: t0.Add(70 * oneMinute), w1h: 300, stop: "end_turn"}.line("text"))
	s := c.summary(30)
	if s.CacheExpiry.Responses != 1 || s.CacheExpiry.Tokens1h != 300 || s.CacheExpiry.Tokens5m != 0 {
		t.Errorf("cold = %+v; want m2's 1h write alone", s.CacheExpiry)
	}
}

// TestCacheExpiry_NoLongerTTLAdviceForA1hWrite: every re-write here was
// already made with the 1h TTL, after a gap of hours -- the shape of the real
// data. Advising the 1h TTL would be advice the figure does not support; the
// line says what happened and nothing more. A 5m re-write does support it.
func TestCacheExpiry_NoLongerTTLAdviceForA1hWrite(t *testing.T) {
	c := newConfig(t)
	t0 := now.Add(-10 * time.Hour)
	c.write("proj/sess-a.jsonl",
		resp{id: "a", model: "claude-opus-5-5", at: t0, w1h: 100, stop: "end_turn"}.line("text"),
		resp{id: "b", model: "claude-opus-5-5", at: t0.Add(3 * time.Hour), w1h: 50000, stop: "end_turn"}.line("text"))
	s := c.summary(30)
	txt, _ := render(t, s)
	if len(s.Savings) != 1 || s.Savings[0].Hint != "" {
		t.Errorf("savings = %+v, want the re-write with no TTL hint", s.Savings)
	}
	for _, bad := range []string{"1h TTL", "keep the session warm", "went cold"} {
		if strings.Contains(txt, bad) {
			t.Errorf("text says %q about a write that was already 1h:\n%s", bad, txt)
		}
	}
	if !strings.Contains(txt, "re-written after a gap longer than its TTL") {
		t.Errorf("text does not call it a re-write after a gap:\n%s", txt)
	}

	c.write("proj/sess-b.jsonl",
		resp{id: "c", model: "claude-opus-5-5", at: t0, w5: 100, stop: "end_turn"}.line("text"),
		resp{id: "d", model: "claude-opus-5-5", at: t0.Add(20 * oneMinute), w5: 50000, stop: "end_turn"}.line("text"))
	s = c.summary(30)
	txt, _ = render(t, s)
	if len(s.Savings) != 1 || s.Savings[0].Hint != SavingHintLongerTTL || !strings.Contains(txt, "the 1h TTL keeps a cache") {
		t.Errorf("a 5m re-write does not carry the 1h TTL hint: %+v\n%s", s.Savings, txt)
	}
}

// TestCacheExpiry_ThePredecessorMayLieOutsideTheWindow: the window is
// applied after ordering, or the first response inside it loses the gap.
func TestCacheExpiry_ThePredecessorMayLieOutsideTheWindow(t *testing.T) {
	c := newConfig(t)
	c.write("proj/sess-a.jsonl",
		resp{id: "r1", model: "claude-opus-5-5", at: now.Add(-3 * 24 * time.Hour), w5: 10, stop: "end_turn"}.line("text"),
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
		resp{id: "a", model: "claude-opus-5-5", at: t0, w5: 1000, stop: "end_turn"}.line("text"),
		resp{id: "syn", model: "<synthetic>", at: t0.Add(20 * oneMinute), stop: "end_turn"}.line("text"),
		resp{id: "b", model: "claude-opus-5-5", at: t0.Add(20*oneMinute + time.Second), w5: 50000, stop: "end_turn"}.line("text"))
	s := c.summary(30)
	if s.CacheExpiry.Responses != 1 || s.CacheExpiry.Cost.Nano != 50000*(opusW5-opusRead) {
		t.Errorf("cold = %d responses, %d nanodollars; want b's write, 1 and %d",
			s.CacheExpiry.Responses, s.CacheExpiry.Cost.Nano, 50000*(opusW5-opusRead))
	}
}

// TestCacheExpiry_AWriteOnAWarmCacheIsNotCold: a response that read from the
// cache found it warm, whatever the gap before it, and its write only added
// the tokens after the cached prefix. Counting it whole priced a 1k write on a
// warm cache as an expiry. And a cold write is priced as the write over a
// cache read of the same tokens -- the alternative was a read, not nothing --
// so the full write rate overstated the figure.
func TestCacheExpiry_AWriteOnAWarmCacheIsNotCold(t *testing.T) {
	c := newConfig(t)
	t0 := now.Add(-2 * time.Hour)
	c.write("proj/sess-a.jsonl",
		resp{id: "a", model: "claude-opus-5-5", at: t0, w5: 100, stop: "end_turn"}.line("text"),
		resp{id: "warm", model: "claude-opus-5-5", at: t0.Add(20 * oneMinute), read: 5000, w5: 1000, stop: "end_turn"}.line("text"),
		resp{id: "cold", model: "claude-opus-5-5", at: t0.Add(40 * oneMinute), w5: 2000, stop: "end_turn"}.line("text"))
	s := c.summary(30)
	if s.CacheExpiry.Responses != 1 || s.CacheExpiry.Tokens != 2000 {
		t.Errorf("cold = %d responses, %d tokens; want the write that read nothing, 1 and 2000",
			s.CacheExpiry.Responses, s.CacheExpiry.Tokens)
	}
	if want := int64(2000 * (opusW5 - opusRead)); s.CacheExpiry.Cost.Nano != want {
		t.Errorf("cold cost = %d, want %d: the write rate minus the read rate", s.CacheExpiry.Cost.Nano, want)
	}
	txt, js := render(t, s)
	if !strings.Contains(txt, "over cache reads") || !strings.Contains(js, "read nothing from the cache") {
		t.Errorf("the figure does not say it is the write over a cache read of a cache that read nothing:\n%s\n%s", txt, js)
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

// TestRefusalsAndExtraAttempts: refusals are counted and priced; an extra
// attempt whose entry names no model the table knows is counted in TOKENS,
// with the dollars unknown and the total said to leave it out.
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
		t.Errorf("extra attempts = %+v, want 1 response with 2 declined attempts", s.ExtraAttempts)
	}
	if got := s.ExtraAttempts.Tokens.Total(); got != 100+10+200+20+5 {
		t.Errorf("extra-attempt tokens = %d, want %d: the declined attempts only, never the final one the top level already counts",
			got, 100+10+200+20+5)
	}
	// The top level is still the returned attempt only.
	if want := int64(1000*10000+40*50000) + int64(1000*opusIn+300*opusOut) + int64(1*opusIn); s.Total.Nano != want {
		t.Errorf("total = %d, want %d", s.Total.Nano, want)
	}
	txt, js := render(t, s)
	if !strings.Contains(txt, "1 response ended in a refusal, $0.01") {
		t.Errorf("text does not carry the refusal line:\n%s", txt)
	}
	if !strings.Contains(txt, "1 response carried 2 extra attempts, 335 tokens; 335 tokens on 2 attempts cost unknown") || strings.Contains(txt, "\ndeclined ") {
		t.Errorf("text does not carry the extra attempts in tokens with the cost unknown:\n%s", txt)
	}
	if !strings.Contains(js, `"cost_unknown_reason":"`+AttemptsUnpriced+`"`) {
		t.Errorf("json does not state why the attempts are unpriced:\n%s", js)
	}
	// The headline's total leaves the extra attempts out, and says so: a
	// wholly known "est. $X" beside tokens nobody priced read as complete.
	if !strings.Contains(txt, "\n       the total leaves out 335 tokens on 2 extra attempts whose cost is unknown (see retries)\n") {
		t.Errorf("the headline does not say the total excludes the extra attempts:\n%s", txt)
	}
	// The priced refusal is a saving with its figure, by category and
	// model; the interim "not computed" line is gone with nothing left out.
	if !strings.Contains(txt, "savings       $0.01 on uncategorized refusals on claude-fable-5-1\n") ||
		!strings.Contains(js, `"savings_not_computed":[]`) || strings.Contains(txt, "not computed") {
		t.Errorf("the savings output does not carry the billed refusal:\n%s\n%s", txt, js)
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

// TestExtraAttempts_TheFallbackPagesExample: iteration entries were decoded
// into counts alone, so the attempt Fable 5 declined was never priced or
// placed, the served response showed only Opus 4.8, and the routing was
// invisible. The entries' type and model are read: the response is reported
// as claude-fable-5 -> claude-opus-4-8, and the declined attempt -- which
// produced no output, so was billed only if its refusal category is billed,
// which no entry records -- is tokens with the cost unknown, left out of the
// total, and said to be.
func TestExtraAttempts_TheFallbackPagesExample(t *testing.T) {
	c := newConfig(t)
	c.write("proj/sess-a.jsonl", transcriptLine(t, fallbackExample, now.Add(-time.Hour), nil))
	s := c.summary(30)
	if want := int64(412*opus48In + 264*opus48Out); s.Total.Nano != want || !s.Total.Known() {
		t.Errorf("total = %+v, want %d: the served attempt, with the no-output declined one left out", s.Total, want)
	}
	e := s.ExtraAttempts
	if e.Responses != 1 || e.Attempts != 1 || e.Tokens.Total() != 535 || e.Cost.Unpriced != 1 || e.Cost.UnpricedTokens != 535 || e.Cost.Priced != 0 {
		t.Errorf("extra attempts = %+v, want the one declined attempt, 535 tokens, cost unknown", e)
	}
	wantD := []DeclinedAttempts{{Model: "claude-fable-5", Attempts: 1, Tokens: Tokens{Input: 535}, Cost: AttemptCost{Cost{Unpriced: 1, UnpricedTokens: 535}},
		NoOutput: 1, NoOutputTokens: 535}}
	if !reflect.DeepEqual(e.Declined, wantD) {
		t.Errorf("declined = %+v, want %+v", e.Declined, wantD)
	}
	wantR := []FallbackRoute{{Requested: "claude-fable-5", Served: "claude-opus-4-8", Responses: 1}}
	if !reflect.DeepEqual(e.Fallback, wantR) {
		t.Errorf("fallback = %+v, want %+v", e.Fallback, wantR)
	}
	txt, js := render(t, s)
	for _, want := range []string{
		"\n       the total leaves out 535 tokens on 1 extra attempt whose cost is unknown (see retries)\n",
		"\nfallback      claude-fable-5 -> claude-opus-4-8 on 1 response\n",
		"\ndeclined      claude-fable-5 1 attempt (535 tokens on 1 attempt with no output, billed only in some refusal categories, which the transcript does not record)\n",
		"\nby model      claude-opus-4-8 <$0.01   claude-fable-5 535 tokens, cost unknown\n",
	} {
		if !strings.Contains(txt, want) {
			t.Errorf("text lacks %q:\n%s", want, txt)
		}
	}
	if !strings.Contains(js, `"fallback_served":[{"requested":"claude-fable-5","served":"claude-opus-4-8","sticky":false,"responses":1}]`) {
		t.Errorf("the JSON does not carry the route:\n%s", js)
	}
	for name, out := range map[string]string{"text": txt, "json": js} {
		if strings.Contains(out, "How can I help") {
			t.Errorf("the example's content reached the %s output", name)
		}
	}
}

// TestExtraAttempts_AnAttemptWithOutputIsPricedAtItsOwnModelsRates: the page
// bills "every attempt that produced output, including one that declined
// partway through its response" at the rates of the model that ran it, and a
// non-streaming mid-output decline keeps the declined attempt's output tokens
// in usage.iterations. The example with output on the declined Fable 5
// attempt: priced at Fable 5's rates into the total, its by-model row, its
// kinds and the declined figure.
func TestExtraAttempts_AnAttemptWithOutputIsPricedAtItsOwnModelsRates(t *testing.T) {
	c := newConfig(t)
	c.write("proj/sess-a.jsonl", transcriptLine(t, fallbackExample, now.Add(-time.Hour), func(m map[string]any) {
		first := iterationsOf(m)[0].(map[string]any)
		first["output_tokens"] = 50
		first["cache_read_input_tokens"] = 100
	}))
	s := c.summary(30)
	declined := int64(535*fable5In + 50*fable5Out + 100*fable5Read)
	served := int64(412*opus48In + 264*opus48Out)
	if s.Total.Nano != served+declined || !s.Total.Known() {
		t.Errorf("total = %+v, want %d: the declined attempt at Fable 5's rates, beside the served one", s.Total, served+declined)
	}
	if s.ExtraAttempts.Cost.Nano != declined || s.ExtraAttempts.Cost.Unpriced != 0 {
		t.Errorf("extra-attempt cost = %+v, want %d", s.ExtraAttempts.Cost, declined)
	}
	if len(s.ExtraAttempts.Declined) != 1 || s.ExtraAttempts.Declined[0].Cost.Nano != declined || s.ExtraAttempts.Declined[0].NoOutput != 0 {
		t.Errorf("declined = %+v, want Fable 5's attempt at %d", s.ExtraAttempts.Declined, declined)
	}
	models := map[string]int64{}
	for _, m := range s.ByModel {
		models[m.Model] = m.Cost.Nano
	}
	if models["claude-fable-5"] != declined || models["claude-opus-4-8"] != served {
		t.Errorf("by model = %v, want each attempt under the model that ran it", models)
	}
	k := s.ByKind
	if k.Input.Nano+k.Output.Nano+k.CacheRead.Nano+k.CacheWrite.Nano != s.Total.Nano || k.CacheRead.Nano != 100*fable5Read {
		t.Errorf("by kind = %+v, want it to sum to the total with the attempt's kinds in it", k)
	}
	if s.ByAgent.Main.Nano != s.Total.Nano || s.PerSession[0].Main.Nano != s.Total.Nano {
		t.Errorf("by agent = %+v, per session = %+v: the attempt is missing from the split", s.ByAgent, s.PerSession)
	}
	txt, _ := render(t, s)
	if strings.Contains(txt, "the total leaves out") || !strings.Contains(txt, "1 response carried 1 extra attempt, 685 tokens: <$0.01 at the rates of the models that ran them, in the total\n") {
		t.Errorf("the retries line does not price the attempt into the total:\n%s", txt)
	}
}

// TestExtraAttempts_AStickyRoutedResponseIsReported: after a fallback, the
// API sends later turns of the conversation straight to the fallback model.
// Such a response carries only a "fallback_message" entry and no "message"
// entry for the model asked. It was served by a fallback, and is reported as
// one, with the model asked said to be absent rather than guessed.
func TestExtraAttempts_AStickyRoutedResponseIsReported(t *testing.T) {
	c := newConfig(t)
	c.write("proj/sess-a.jsonl", transcriptLine(t, fallbackExample, now.Add(-time.Hour), func(m map[string]any) {
		u := m["usage"].(map[string]any)
		u["iterations"] = iterationsOf(m)[1:]
	}))
	s := c.summary(30)
	wantR := []FallbackRoute{{Served: "claude-opus-4-8", Sticky: true, Responses: 1}}
	if !reflect.DeepEqual(s.ExtraAttempts.Fallback, wantR) || s.ExtraAttempts.Responses != 0 {
		t.Errorf("extra attempts = %+v, want no extra attempt and the sticky route %+v", s.ExtraAttempts, wantR)
	}
	txt, _ := render(t, s)
	if !strings.Contains(txt, "\nfallback      (sticky routing: the model asked is not in the transcript) -> claude-opus-4-8 on 1 response\n") {
		t.Errorf("text does not report the sticky-routed response:\n%s", txt)
	}
}

// TestRefusals_APreOutputRefusalWithoutUsageIsCounted: a zero-usage refusal
// line that reports no response with usage (refusalMessage) was dropped by
// Build with every zero-token response -- so a transcript holding a refusal
// printed "refusals none". It is counted, a count only, and the text says
// what the page says of its billing. With no other response, the line is
// still printed.
func TestRefusals_APreOutputRefusalWithoutUsageIsCounted(t *testing.T) {
	at := now.Add(-time.Hour)
	synthetic := func(id string) string { return refusalMessage(id, "", "", at) }
	for _, tc := range []struct {
		name  string
		lines []string
		n     int
		want  string
	}{
		{"beside a billed response", []string{
			resp{id: "r", model: "claude-opus-5-5", at: at, in: 10, stop: "end_turn"}.line("text"), synthetic("z1"), synthetic("z2")}, 2,
			"refusals      2 pre-output refusals were written without usage\n" +
				"              uncategorized (model not recorded): 2 without usage, not billed (a pre-output refusal in this category is not)\n"},
		{"alone", []string{synthetic("z1")}, 1,
			"refusals      1 pre-output refusal was written without usage\n" +
				"              uncategorized (model not recorded): 1 without usage, not billed (a pre-output refusal in this category is not)\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newConfig(t)
			c.write("proj/sess-a.jsonl", tc.lines...)
			s := c.summary(30)
			txt, js := render(t, s)
			if !strings.Contains(txt, tc.want) || strings.Contains(txt, "refusals      none") {
				t.Errorf("text lacks %q:\n%s", tc.want, txt)
			}
			if !strings.Contains(js, fmt.Sprintf(`"without_usage":%d`, tc.n)) || s.Refusals.WithoutUsage != tc.n {
				t.Errorf("without_usage = %d:\n%s", s.Refusals.WithoutUsage, js)
			}
			// Uncategorized pre-output refusals are not billed: nothing to
			// save, and nothing left uncomputed.
			if strings.Contains(txt, "savings") || !strings.Contains(js, `"savings_not_computed":[]`) {
				t.Errorf("the savings output names an unbilled refusal:\n%s\n%s", txt, js)
			}
		})
	}
}

// TestExtraAttempts_TheServedModelIsTheFallbackEntrys: for a mid-output
// fallback the page says message_start "already named the requested model,
// so read the serving model from the fallback block's to.model and from the
// fallback_message entry in the final message_delta's usage.iterations". A
// streamed line whose message.model is the model asked priced the served
// attempt at that model's rates (25,170,000 nanodollars against 16,510,000)
// and printed "claude-fable-5 -> claude-fable-5" with no Opus 4.8 row. The
// served model is the last entry's when it names one.
func TestExtraAttempts_TheServedModelIsTheFallbackEntrys(t *testing.T) {
	c := newConfig(t)
	c.write("proj/sess-a.jsonl", transcriptLine(t, fallbackExample, now.Add(-time.Hour), func(m map[string]any) {
		m["model"] = "claude-fable-5"
		iterationsOf(m)[0].(map[string]any)["output_tokens"] = 50
	}))
	s := c.summary(30)
	served := int64(412*opus48In + 264*opus48Out)
	declined := int64(535*fable5In + 50*fable5Out)
	if s.Total.Nano != served+declined || s.Total.Nano != 16_510_000 {
		t.Errorf("total = %d, want %d: the served attempt at Opus 4.8's rates", s.Total.Nano, served+declined)
	}
	models := map[string]int64{}
	for _, m := range s.ByModel {
		models[m.Model] = m.Cost.Nano
	}
	if models["claude-opus-4-8"] != served || models["claude-fable-5"] != declined {
		t.Errorf("by model = %v, want the served attempt under claude-opus-4-8", models)
	}
	wantR := []FallbackRoute{{Requested: "claude-fable-5", Served: "claude-opus-4-8", Responses: 1}}
	if !reflect.DeepEqual(s.ExtraAttempts.Fallback, wantR) {
		t.Errorf("fallback = %+v, want %+v", s.ExtraAttempts.Fallback, wantR)
	}
}

// TestExtraAttempts_TheDeclinedLineSaysWhichTokensHadNoOutput: the declined
// line printed every unpriced token as "on N attempt with no output", so on a
// model the table lacks, the billed tokens of an attempt that produced output
// were called no-output tokens and the "not in the price table" reason was
// hidden by an else-if. Each clause now carries its own tokens. The counts in
// extra_attempts.cost and declined[].cost are attempts, and are named so; and
// every attempt's tokens, priced or not, are in the token total and its
// model's row, which counts its attempts.
func TestExtraAttempts_TheDeclinedLineSaysWhichTokensHadNoOutput(t *testing.T) {
	for _, tc := range []struct {
		name   string
		its    []resp
		line   string
		tokens int64
		js     string
	}{
		{"unpriced, with and without output",
			[]resp{{model: "claude-mystery-9", in: 535}, {model: "claude-mystery-9", in: 1000, out: 70}},
			"claude-mystery-9 2 attempts (535 tokens on 1 attempt with no output, billed only in some refusal categories, which the transcript does not record; 1,070 tokens, cost unknown: not in the price table)",
			1605,
			`"declined":[{"model":"claude-mystery-9","attempts":2,"tokens":{"input":1535,"output":70,"cache_read":0,"cache_write_5m":0,"cache_write_1h":0},"cost":{"usd":null,"unpriced_attempts":2,"unpriced_tokens":1605},"no_output":1,"no_output_tokens":535}]`},
		{"unpriced, with output only",
			[]resp{{model: "claude-mystery-9", in: 1000, out: 70}},
			"claude-mystery-9 1 attempt (1,070 tokens, cost unknown: not in the price table)",
			1070,
			`"cost":{"usd":null,"unpriced_attempts":1,"unpriced_tokens":1070},"no_output":0,"no_output_tokens":0}]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newConfig(t)
			its := append(tc.its, resp{typ: "fallback_message", model: "claude-opus-4-8", in: 412, out: 264})
			c.write("proj/sess-a.jsonl", resp{id: "fb", model: "claude-opus-4-8", at: now.Add(-time.Hour), in: 412, out: 264,
				stop: "end_turn", iters: its}.line("text"))
			s := c.summary(30)
			txt, js := render(t, s)
			if !strings.Contains(txt, "\ndeclined      "+tc.line+"\n") {
				t.Errorf("text lacks %q:\n%s", tc.line, txt)
			}
			if !strings.Contains(js, tc.js) || !strings.Contains(js, `"extra_attempts":{"responses":1,"attempts":`) ||
				strings.Contains(js, `"declined":[{"model":"claude-mystery-9","attempts":1,"tokens":{"input":1000,"output":70,"cache_read":0,"cache_write_5m":0,"cache_write_1h":0},"cost":{"usd":null,"unpriced_responses"`) {
				t.Errorf("the JSON lacks %s:\n%s", tc.js, js)
			}
			if !strings.Contains(js, `"unpriced_attempts":`+fmt.Sprint(len(tc.its))+`,"unpriced_tokens":`+fmt.Sprint(tc.tokens)+`},"cost_unknown_reason"`) {
				t.Errorf("extra_attempts.cost does not count unpriced attempts as attempts:\n%s", js)
			}
			if got, want := s.Tokens.Total(), 412+264+tc.tokens; got != want {
				t.Errorf("token total = %d, want %d: every attempt's tokens", got, want)
			}
			var row *ModelSpend
			for i := range s.ByModel {
				if s.ByModel[i].Model == "claude-mystery-9" {
					row = &s.ByModel[i]
				}
			}
			if row == nil || row.Tokens.Total() != tc.tokens || row.Attempts != len(tc.its) || row.Responses != 0 {
				t.Errorf("claude-mystery-9 row = %+v, want %d tokens over %d attempts", row, tc.tokens, len(tc.its))
			}
		})
	}
}

// TestExtraAttempts_AnAllDeclinedChainIsNotServed: when every model in the
// chain declines, the page says the response is the last model's refusal,
// with a fallback_message entry last. It was reported as served -- "fallback
// claude-fable-5 -> claude-opus-4-8", in fallback_served -- and the saving
// told the reader to choose with /model a model that refused too. The
// earlier attempts are declined ones; nothing was served, and no model is
// suggested.
func TestExtraAttempts_AnAllDeclinedChainIsNotServed(t *testing.T) {
	c := newConfig(t)
	c.write("proj/sess-a.jsonl", transcriptLine(t, fallbackExample, now.Add(-time.Hour), func(m map[string]any) {
		m["stop_reason"] = "refusal"
		m["stop_details"] = map[string]any{"type": "refusal", "category": "bio"}
		iterationsOf(m)[0].(map[string]any)["output_tokens"] = 50
	}))
	s := c.summary(30)
	if len(s.ExtraAttempts.Fallback) != 0 {
		t.Errorf("fallback = %+v, want none: every model declined", s.ExtraAttempts.Fallback)
	}
	if len(s.ExtraAttempts.Declined) != 1 || s.ExtraAttempts.Declined[0].Model != "claude-fable-5" {
		t.Errorf("declined = %+v, want Fable 5's attempt", s.ExtraAttempts.Declined)
	}
	for _, sv := range s.Savings {
		if sv.Hint == SavingHintServedModel {
			t.Errorf("saving %+v suggests a model that also refused", sv)
		}
	}
	txt, _ := render(t, s)
	if !strings.Contains(txt, "\nfallback      none (no response was served by a fallback model)\n") || strings.Contains(txt, "/model") {
		t.Errorf("an all-declined chain reads as served:\n%s", txt)
	}
}

// TestExtraAttempts_StickyIsOnlyAChainWithNoMessageEntry: the page tells a
// sticky-routed response by "the absence of a message entry for the
// requested model". Sticky was inferred from the first entry alone, so a
// chain whose first entry is not a "message", or whose "message" entry names
// no model, printed "sticky routing" beside a declined line for the same
// response. The model asked is the first "message" entry before the last.
func TestExtraAttempts_StickyIsOnlyAChainWithNoMessageEntry(t *testing.T) {
	for _, tc := range []struct {
		name      string
		edit      func([]any) []any
		requested string
	}{
		{"a non-message first entry", func(its []any) []any {
			return append([]any{map[string]any{"type": "something_new", "model": "claude-opus-5-5", "input_tokens": 7, "output_tokens": 0}}, its...)
		}, "claude-fable-5"},
		{"a message entry with no model", func(its []any) []any {
			delete(its[0].(map[string]any), "model")
			return its
		}, ModelNotRecorded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newConfig(t)
			c.write("proj/sess-a.jsonl", transcriptLine(t, fallbackExample, now.Add(-time.Hour), func(m map[string]any) {
				u := m["usage"].(map[string]any)
				u["iterations"] = tc.edit(iterationsOf(m))
			}))
			s := c.summary(30)
			wantR := []FallbackRoute{{Requested: tc.requested, Served: "claude-opus-4-8", Responses: 1}}
			if !reflect.DeepEqual(s.ExtraAttempts.Fallback, wantR) {
				t.Errorf("fallback = %+v, want %+v: not sticky", s.ExtraAttempts.Fallback, wantR)
			}
			txt, _ := render(t, s)
			if strings.Contains(txt, "sticky") {
				t.Errorf("text calls a chain with a message entry sticky:\n%s", txt)
			}
		})
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
// -- so it is one refusal, billed at the rates of the model that ran it.
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
			if strings.Contains(txt, "without usage") || strings.Contains(txt, "not computed") || !strings.Contains(js, `"savings_not_computed":[]`) {
				t.Errorf("the refusal is counted again as a pre-output one:\n%s\n%s", txt, js)
			}
		})
	}
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

// TestRefusals_APreOutputRefusalIsBilledByItsCategory: the page bills a
// refusal before any output only in bio, frontier_llm and
// reasoning_extraction; in cyber, general_harms or with a null category it
// "is not billed". Pre-output was read from the line's shape -- a zero-usage
// line -- so a cyber refusal with usage and no output went into the total and
// billed_refusals. It is read from output_tokens == 0: such a refusal is out
// of the total and the savings and counted as not billed; one in a category
// this read does not know shows its tokens with the cost unknown; and a
// billed one with no amount (a zero-usage line) is a billed_refusals entry of
// unknown cost, by category and model, with its lever where it has one --
// never only a count with no category, model or lever.
func TestRefusals_APreOutputRefusalIsBilledByItsCategory(t *testing.T) {
	at := now.Add(-time.Hour)
	t.Run("cyber with usage and no output", func(t *testing.T) {
		c := newConfig(t)
		c.write("proj/sess-a.jsonl",
			resp{id: "r", model: "claude-opus-5-5", at: at, in: 10, stop: "end_turn"}.line("text"),
			resp{id: "cy", model: "claude-opus-5-5", at: at, in: 10000, stop: "refusal", category: "cyber"}.line("text"))
		s := c.summary(30)
		if s.Total.Nano != 10*opusIn || s.Responses != 1 {
			t.Errorf("total = %d over %d responses, want %d over 1: an unbilled refusal is in the total", s.Total.Nano, s.Responses, 10*opusIn)
		}
		if len(s.Savings) != 0 {
			t.Errorf("savings = %+v, want none: the refusal was not billed", s.Savings)
		}
		if s.Refusals.NotBilled != 1 || s.Refusals.Responses != 0 || len(s.Refusals.ByCategory) != 1 || s.Refusals.ByCategory[0].NotBilled != 1 {
			t.Errorf("refusals = %+v, want one not billed", s.Refusals)
		}
		txt, _ := render(t, s)
		if !strings.Contains(txt, "cyber on claude-opus-5-5: 1 before any output with usage, not billed (a pre-output refusal in this category is not)\n") {
			t.Errorf("text does not say the refusal was not billed:\n%s", txt)
		}
	})
	t.Run("other with usage and no output", func(t *testing.T) {
		c := newConfig(t)
		c.write("proj/sess-a.jsonl",
			resp{id: "o", model: "claude-opus-5-5", at: at, in: 1234, stop: "refusal", category: "a_new_category"}.line("text"))
		s := c.summary(30)
		if s.Total.Priced != 0 || s.Total.Unpriced != 1 || s.Total.UnpricedTokens != 1234 {
			t.Errorf("total = %+v, want its tokens with the cost unknown", s.Total)
		}
		if len(s.SavingsNotComputed) != 0 {
			t.Errorf("savings not computed = %v, want none: its model has a rate, its billing is what is unknown", s.SavingsNotComputed)
		}
		txt, _ := render(t, s)
		if !strings.Contains(txt, "other on claude-opus-5-5: 1 response, 1,234 tokens, cost unknown") {
			t.Errorf("text does not show the tokens with the cost unknown:\n%s", txt)
		}
	})
	t.Run("reasoning_extraction without usage", func(t *testing.T) {
		c := newConfig(t)
		c.write("proj/sess-a.jsonl",
			resp{id: "r", model: "claude-opus-5-5", at: at, in: 10, stop: "end_turn"}.line("text"),
			refusalMessage("z1", "", "reasoning_extraction", at), refusalMessage("z2", "", "frontier_llm", at))
		s := c.summary(30)
		want := []Saving{
			{Kind: SavingBilledRefusals, Category: "frontier_llm", Model: ModelNotRecorded, Cost: Cost{Unpriced: 1}},
			{Kind: SavingBilledRefusals, Category: "reasoning_extraction", Model: ModelNotRecorded, Cost: Cost{Unpriced: 1},
				Hint: SavingHintReasoningInReply},
		}
		if !reflect.DeepEqual(s.Savings, want) || len(s.SavingsNotComputed) != 0 {
			t.Errorf("savings = %+v, not computed %v\nwant %+v", s.Savings, s.SavingsNotComputed, want)
		}
		txt, js := render(t, s)
		for _, line := range []string{
			"              reasoning_extraction (model not recorded): 1 without usage, billed before any output in this category; the amount is not in the transcript\n",
			"savings       1 pre-output frontier_llm refusal (model not recorded) was billed; the amount is not in the transcript\n",
			"              1 pre-output reasoning_extraction refusal (model not recorded) was billed; the amount is not in the transcript: this category is a request for the model's internal reasoning in its reply",
		} {
			if !strings.Contains(txt, line) {
				t.Errorf("text lacks %q:\n%s", line, txt)
			}
		}
		if strings.Contains(txt, " on other") || !strings.Contains(js, `"model":"not_recorded","cost":{"usd":null,"unpriced_responses":1`) {
			t.Errorf("a synthetic group reads as a model named other, or its cost as known:\n%s\n%s", txt, js)
		}
	})
	t.Run("only a billed pre-output refusal", func(t *testing.T) {
		c := newConfig(t)
		c.write("proj/sess-a.jsonl", refusalMessage("z1", "", "bio", at))
		txt, _ := render(t, c.summary(30))
		if !strings.Contains(txt, "est. $0.00 at API list prices") ||
			!strings.Contains(txt, "\n       the total leaves out 1 pre-output refusal that was billed (the amount is not in the transcript)\n") {
			t.Errorf("the header prints $0.00 beside a billed refusal with no caveat:\n%s", txt)
		}
	})
}

// TestRefusals_AreSplitByCategoryAndModel: stop_details.category was never
// read, so every refusal landed in one total and one line, and nothing told
// a classifier decline in a named category from the rest. Worse, a
// pre-output refusal in a category the API bills before any output (bio,
// frontier_llm, reasoning_extraction) was said to have billing that "cannot
// be read": it was billed, and the transcript does not hold the amount. The
// category is read as a closed word -- a null is "uncategorized", a word the
// page does not name is "other" -- and the refusals are split by category
// and model, each saying what the page says of its billing. Each of the five
// named categories is held to its billing word, so a category moved to the
// wrong side of the page's rule fails here.
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
	yes, no := true, false
	nr := ModelNotRecorded
	want := []RefusalGroup{
		{Category: "bio", Model: nr, WithoutUsage: 2, BilledBeforeOutput: &yes},
		{Category: "cyber", Model: "claude-fable-5-1", Responses: 1, Cost: Cost{Nano: 1000*10000 + 40*50000, Priced: 1}, BilledBeforeOutput: &no},
		{Category: "cyber", Model: "claude-opus-5-5", NotBilled: 1, BilledBeforeOutput: &no},
		{Category: "cyber", Model: nr, WithoutUsage: 1, BilledBeforeOutput: &no},
		{Category: "frontier_llm", Model: nr, WithoutUsage: 1, BilledBeforeOutput: &yes},
		{Category: "general_harms", Model: nr, WithoutUsage: 1, BilledBeforeOutput: &no},
		{Category: "other", Model: nr, WithoutUsage: 1},
		{Category: "reasoning_extraction", Model: nr, WithoutUsage: 1, BilledBeforeOutput: &yes},
		{Category: "uncategorized", Model: nr, WithoutUsage: 1, BilledBeforeOutput: &no},
	}
	if !reflect.DeepEqual(s.Refusals.ByCategory, want) {
		t.Errorf("by category = %+v\nwant %+v", s.Refusals.ByCategory, want)
	}
	txt, js := render(t, s)
	for _, line := range []string{
		"refusals      1 response ended in a refusal, $0.01; 1 refusal before any output with usage was not billed, and not in the total; 8 pre-output refusals were written without usage\n",
		"              bio (model not recorded): 2 without usage, billed before any output in this category; the amount is not in the transcript\n",
		"              cyber on claude-fable-5-1: 1 response, $0.01\n",
		"              cyber on claude-opus-5-5: 1 before any output with usage, not billed (a pre-output refusal in this category is not)\n",
		"              cyber (model not recorded): 1 without usage, not billed (a pre-output refusal in this category is not)\n",
		"              frontier_llm (model not recorded): 1 without usage, billed before any output in this category; the amount is not in the transcript\n",
		"              general_harms (model not recorded): 1 without usage, not billed (a pre-output refusal in this category is not)\n",
		"              other (model not recorded): 1 without usage, billing unknown (a category this read does not know)\n",
		"              reasoning_extraction (model not recorded): 1 without usage, billed before any output in this category; the amount is not in the transcript\n",
	} {
		if !strings.Contains(txt, line) {
			t.Errorf("text lacks %q:\n%s", line, txt)
		}
	}
	if !strings.Contains(js, `{"category":"bio","model":"not_recorded","responses":0,"cost":{"usd":0,"unpriced_responses":0,"unpriced_tokens":0},"not_billed":0,"without_usage":2,"billed_before_output":true}`) ||
		!strings.Contains(js, `"category":"other","model":"not_recorded","responses":0,"cost":{"usd":0,"unpriced_responses":0,"unpriced_tokens":0},"not_billed":0,"without_usage":1,"billed_before_output":null}`) {
		t.Errorf("the JSON does not split the refusals by category:\n%s", js)
	}
	if strings.Contains(txt+js, "a_new_category") {
		t.Errorf("a category outside the closed vocabulary was printed as read:\n%s\n%s", txt, js)
	}
}

// TestSavings_BilledRefusalsAndDeclinedAttemptsByCategoryAndModel: the
// savings list never read Refusals or ExtraAttempts, so a $1.00 Fable
// refusal beside a fallback-routed response printed "savings: []". Each
// billed refusal group and each model's priced declined attempts is now a
// saving with its figure, and a lever is named only where a Claude Code user
// holds one: reasoning_extraction (a request for the model's reasoning in
// its reply) and a declined model a fallback served (/model). A refusal in a
// category that names a policy area gets its figure and no advice. What was
// billed with no amount in the transcript is named as not computed.
func TestSavings_BilledRefusalsAndDeclinedAttemptsByCategoryAndModel(t *testing.T) {
	c := newConfig(t)
	at := now.Add(-time.Hour)
	c.write("proj/sess-a.jsonl",
		resp{id: "re", model: "claude-fable-5-1", at: at, in: 100000, out: 40, stop: "refusal", category: "reasoning_extraction"}.line("text"),
		// A cyber refusal partway through its output: billed at normal
		// rates. Before any output it would not have been billed.
		resp{id: "cy", model: "claude-opus-5-5", at: at, in: 10000, out: 50, stop: "refusal", category: "cyber"}.line("text"),
		refusalMessage("bio", "", "bio", at),
		transcriptLine(t, fallbackExample, at, func(m map[string]any) {
			iterationsOf(m)[0].(map[string]any)["output_tokens"] = 2000
		}),
		transcriptLine(t, fallbackExample, at, func(m map[string]any) { m["id"] = "msg_no_output" }))
	s := c.summary(30)
	want := []Saving{
		{Kind: SavingBilledRefusals, Category: "bio", Model: ModelNotRecorded, Cost: Cost{Unpriced: 1}},
		{Kind: SavingBilledRefusals, Category: "cyber", Model: "claude-opus-5-5", Cost: Cost{Nano: 10000*opusIn + 50*opusOut, Priced: 1}},
		{Kind: SavingBilledRefusals, Category: "reasoning_extraction", Model: "claude-fable-5-1", Cost: Cost{Nano: 100000*10000 + 40*50000, Priced: 1},
			Hint: SavingHintReasoningInReply},
		{Kind: SavingDeclinedAttempts, Model: "claude-fable-5", Cost: Cost{Nano: 535*fable5In + 2000*fable5Out, Priced: 1}, Hint: SavingHintServedModel},
	}
	if !reflect.DeepEqual(s.Savings, want) {
		t.Errorf("savings = %+v\nwant %+v", s.Savings, want)
	}
	if !reflect.DeepEqual(s.SavingsNotComputed, []string{SavingNotComputedAttempts}) {
		t.Errorf("savings not computed = %v", s.SavingsNotComputed)
	}
	txt, js := render(t, s)
	for _, line := range []string{
		"savings       1 pre-output bio refusal (model not recorded) was billed; the amount is not in the transcript\n",
		"              $0.04 on cyber refusals on claude-opus-5-5\n",
		"              $1.00 on reasoning_extraction refusals on claude-fable-5-1: this category is a request for the model's internal reasoning in its reply, which the model gives as thinking instead\n",
		"              $0.11 on attempts claude-fable-5 declined before a fallback served: choosing the model that served them (/model) for such work skips the declined attempt\n",
		"              not computed: 1 declined attempt with no output, billed only in some refusal categories, which the transcript does not record\n",
	} {
		if !strings.Contains(txt, line) {
			t.Errorf("text lacks %q:\n%s", line, txt)
		}
	}
	if !strings.Contains(js, `"savings_not_computed":["declined_attempts_without_output"]`) {
		t.Errorf("the JSON does not name what was not computed:\n%s", js)
	}
}

// TestSavings_BilledSpendOnAnUnpricedModelIsNamedAsNotComputed: buildSavings
// skipped every group with no priced part, so a billed refusal, or a declined
// attempt that produced output, on a model the table lacks was in neither
// savings nor savings_not_computed -- the list meant to name what could not
// be computed missed it. Each is named, with its tokens and model.
func TestSavings_BilledSpendOnAnUnpricedModelIsNamedAsNotComputed(t *testing.T) {
	c := newConfig(t)
	at := now.Add(-time.Hour)
	c.write("proj/sess-a.jsonl",
		resp{id: "cy", model: "claude-mystery-9", at: at, in: 1000, out: 50, stop: "refusal", category: "cyber"}.line("text"),
		resp{id: "fb", model: "claude-opus-4-8", at: at, in: 412, out: 264, stop: "end_turn", iters: []resp{
			{model: "claude-mystery-8", in: 2000, out: 70},
			{typ: "fallback_message", model: "claude-opus-4-8", in: 412, out: 264}}}.line("text"))
	s := c.summary(30)
	want := []string{SavingNotComputedRefusalsUnpriced, SavingNotComputedAttemptsUnpriced}
	if !reflect.DeepEqual(s.SavingsNotComputed, want) {
		t.Errorf("savings not computed = %v, want %v", s.SavingsNotComputed, want)
	}
	txt, js := render(t, s)
	for _, line := range []string{
		"savings       not computed: 1,050 tokens on claude-mystery-9 billed refusals with no known rate\n",
		"              not computed: 2,070 tokens on claude-mystery-8 declined attempts with no known rate\n",
	} {
		if !strings.Contains(txt, line) {
			t.Errorf("text lacks %q:\n%s", line, txt)
		}
	}
	if !strings.Contains(js, `"savings_not_computed":["billed_refusals_unpriced_model","declined_attempts_unpriced_model"]`) {
		t.Errorf("the JSON does not name what was not computed:\n%s", js)
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
// It also sits in the decoded fields that are printed only as closed words:
// an iteration entry's type and model, on a fallback-served response whose
// declined attempt is printed by model, and a refusal's category (and the
// explanation beside it, which is not read).
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
	walk = func(ty reflect.Type, path string) {
		for ty.Kind() == reflect.Pointer || ty.Kind() == reflect.Slice {
			if ty == raw {
				t.Errorf("%s is a json.RawMessage: it holds the bytes it spans as a value", path)
				return
			}
			ty = ty.Elem()
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
