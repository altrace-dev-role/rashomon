package spend

import (
	"bytes"
	"encoding/json"
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
	sidechain          bool
	noTimestamp        bool
	noSplit            bool   // write cache_creation_input_tokens without the TTL split
	text               string // the text block's words, when not the default
}

func (r resp) usage() map[string]any {
	u := map[string]any{
		"input_tokens":                r.in,
		"output_tokens":               r.out,
		"cache_read_input_tokens":     r.read,
		"cache_creation_input_tokens": r.w5 + r.w1h,
		"service_tier":                "standard",
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
	obj := map[string]any{
		"type":        "assistant",
		"sessionId":   session,
		"isSidechain": r.sidechain,
		"cwd":         "/home/someone/secret-project-" + canary,
		"message": map[string]any{
			"id":          r.id,
			"model":       r.model,
			"role":        "assistant",
			"stop_reason": stop,
			"content":     content,
			"usage":       r.usage(),
		},
	}
	if !r.noTimestamp {
		obj["timestamp"] = r.at.UTC().Format(time.RFC3339Nano)
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

// summary reads the directory the way cmdSpend does, over `days`.
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
func TestDiscover_ASymlinkedProjectFolderIsRead(t *testing.T) {
	c := newConfig(t)
	real := filepath.Join(t.TempDir(), "elsewhere")
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
	if err := os.Symlink(real, filepath.Join(c.dir, "projects", "linked")); err != nil {
		t.Fatal(err)
	}
	rec := newRecorder(t)
	rec.transcript = realMain
	rec.call("sess-l", "p1", "toolu_l", T, T.Add(time.Second), store.ExecOK)

	s := c.summary(30)
	if s.Responses != 2 || s.Total.Nano != 120*opusIn {
		t.Errorf("responses = %d, total = %d; want 2 and %d: the linked folder was not read", s.Responses, s.Total.Nano, 120*opusIn)
	}
	if err := s.Join(rec.st); err != nil {
		t.Fatal(err)
	}
	if j := s.SilentFailureTurns; j.CoveredTranscripts != 1 || j.NotCoveredTranscripts != 0 {
		t.Errorf("covered %d, not covered %d; want 1, 0: a record naming the real path covers the linked transcript",
			j.CoveredTranscripts, j.NotCoveredTranscripts)
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
		}))
	// A subagent's user line with a numeric timestamp does not decode either,
	// but it carries no usage: it is not an unparsed usage line.
	c.write("proj/sess-a/subagents/agent-u.jsonl",
		strings.Replace(subUserLine("sess-a", "p1", now.Add(-time.Hour)), `"timestamp":"`, `"timestamp":5,"x":"`, 1))
	s := c.summary(30)
	if s.Total.Nano != 1000*opusIn || s.Responses != 1 {
		t.Errorf("total = %d over %d responses, want %d over 1: a malformed or implausible line was priced",
			s.Total.Nano, s.Responses, 1000*opusIn)
	}
	if s.Read.UnparsedUsageLines != 5 {
		t.Errorf("unparsed usage lines = %d, want 5", s.Read.UnparsedUsageLines)
	}
	if txt, _ := render(t, s); !strings.Contains(txt, "note: 5 transcript lines that may carry usage could not be read") {
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
	if want := int64(200*opusW5 + 400*opusW1h); s.CacheExpiry.Cost.Nano != want {
		t.Errorf("cold cost = %d, want %d", s.CacheExpiry.Cost.Nano, want)
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
	if s.CacheExpiry.Responses != 1 || s.CacheExpiry.Cost.Nano != 50000*opusW5 {
		t.Errorf("cold = %d responses, %d nanodollars; want b's write, 1 and %d",
			s.CacheExpiry.Responses, s.CacheExpiry.Cost.Nano, 50000*opusW5)
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

// TestRefusalsAndExtraAttempts: refusals are counted and priced; extra
// attempts are counted in TOKENS, with dollars unknown, because an iteration
// entry carries no model to price it at.
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
	if !strings.Contains(txt, "335 tokens spent on the extra attempts, cost unknown") || strings.Contains(txt, "declined") {
		t.Errorf("text does not carry the extra attempts in tokens with the cost unknown:\n%s", txt)
	}
	if !strings.Contains(js, `"cost_unknown_reason":"`+AttemptsUnpriced+`"`) {
		t.Errorf("json does not state why the attempts are unpriced:\n%s", js)
	}
}

// TestOutput_StatesTheBasisOfEveryFigure: estimated, at list prices, as of
// the snapshot date, and not what a plan subscriber was charged.
func TestOutput_StatesTheBasisOfEveryFigure(t *testing.T) {
	c := newConfig(t)
	c.write("proj/sess-a.jsonl", resp{id: "r", model: "claude-opus-5-5", at: now.Add(-time.Hour), in: 1e6, stop: "end_turn"}.line("text"))
	txt, js := render(t, c.summary(30))
	for _, want := range []string{"est. $4.00 at API list prices (2026-09-25)", "not billed per token", "not what was charged"} {
		if !strings.Contains(txt, want) {
			t.Errorf("text lacks %q:\n%s", want, txt)
		}
	}
	if !strings.Contains(js, `"snapshot":"2026-09-25"`) || !strings.Contains(js, "not billed per token") {
		t.Errorf("json lacks the snapshot or the plan note:\n%s", js)
	}
}

// TestContentNeverReachesTheOutput: the canary sits in every fixture's text,
// thinking and tool_use blocks and in cwd, and neither rendering carries it.
func TestContentNeverReachesTheOutput(t *testing.T) {
	c := newConfig(t)
	at := now.Add(-time.Hour)
	r := resp{id: "msg_1", model: "claude-opus-5-5", at: at, in: 1000, out: 500, w5: 10, stop: "tool_use"}
	c.write("proj/sess-a.jsonl", r.line("thinking"), r.line("text"), r.line("tool_use"),
		resp{id: "msg_2", model: "claude-opus-5-5", at: at.Add(10 * oneMinute), w5: 100, stop: "refusal"}.line("text"))
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
func TestContentHasNoFieldToLandIn(t *testing.T) {
	var walk func(reflect.Type, string)
	seen := map[reflect.Type]bool{}
	walk = func(ty reflect.Type, path string) {
		for ty.Kind() == reflect.Pointer || ty.Kind() == reflect.Slice {
			ty = ty.Elem()
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
				"iterations": true,
				// A subagent user line's header: a closed word, a flag and
				// the promptId key that ties a response to its turn.
				"type": true, "isMeta": true, "promptId": true,
			}
			if !f.Anonymous && !allowed[tag] {
				t.Errorf("%s.%s decodes %q, which is outside spend's read path", path, f.Name, tag)
			}
			walk(f.Type, path+"."+f.Name)
		}
	}
	walk(reflect.TypeOf(line{}), "line")
}
