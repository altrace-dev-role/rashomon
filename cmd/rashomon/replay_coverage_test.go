package main

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/replay"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

func sameJSON(a, b json.RawMessage) bool {
	var x, y any
	return json.Unmarshal(a, &x) == nil && json.Unmarshal(b, &y) == nil && reflect.DeepEqual(x, y)
}

// TestREPLAY16_RewrittenInputsAreInvisibleAndTheReplayCarriesTheFlag answers
// whether any transcript field records the input a call actually executed
// with, and holds the replay to what the answer implies.
//
// The recorder compares the digest of the input a PreToolUse saw (the
// declaration's) with the digest of the input a PostToolUse saw (the
// execution's executed_digest), because a hook that rewrites an input shows
// there and nowhere else: the store's own comment on the field says "the
// transcript keeps only the pre-rewrite input". The replay builds both
// payloads from the transcript's one input, so the check cannot fire.
//
// The 2.1.312 fixture has two candidates for a second record of the input:
// the assistant lines' wireToolInputs, and the deferred_tools_record
// attachments' toolInputCopies. Neither is one:
//
//   - for every call, the input in the tool_use block, the PreToolUse payload
//     and the PostToolUse payload are the same JSON value;
//   - wireToolInputs is the input as the model wrote it, and differs from the
//     block's only for the two Edits, by the key replace_all: false, which
//     Claude Code adds as the tool's default; the two toolInputCopies entries
//     are exactly those two calls, each saying copy "wire";
//   - every assistant line's input_transformations is the empty list, and no
//     line at any depth has a key that names an updated, rewritten or
//     overridden input;
//   - the transcript has no line for a PreToolUse or PostToolUse hook having
//     run at all (the capture hook did run for every call: the payloads are
//     the proof), only the Stop hook's summary.
//
// No fixture contains a rewrite, so the fixtures cannot show where one would
// be written, and a field that has never been seen to differ is not evidence
// that nothing differed. The replay therefore says so on every session
// (flagRewriteAbsent), unconditionally.
func TestREPLAY16_RewrittenInputsAreInvisibleAndTheReplayCarriesTheFlag(t *testing.T) {
	f := loadFixtureSession(t, "2.1.312")
	pre, post := map[string]json.RawMessage{}, map[string]json.RawMessage{}
	for _, s := range f.captured {
		var m struct {
			Event string          `json:"hook_event_name"`
			ID    string          `json:"tool_use_id"`
			Input json.RawMessage `json:"tool_input"`
		}
		_ = json.Unmarshal(s.raw, &m)
		switch m.Event {
		case "PreToolUse":
			pre[m.ID] = m.Input
		case "PostToolUse", "PostToolUseFailure":
			post[m.ID] = m.Input
		}
	}
	raw, err := os.ReadFile(fixtureTranscript("2.1.312"))
	if err != nil {
		t.Fatal(err)
	}
	suspect := regexp.MustCompile(`(?i)updated.?input|rewrit|override`)
	var walk func(v any, path string)
	walk = func(v any, path string) {
		switch v := v.(type) {
		case map[string]any:
			for k, x := range v {
				if suspect.MatchString(k) {
					t.Errorf("a transcript key names a rewritten input: %s/%s", path, k)
				}
				walk(x, path+"/"+k)
			}
		case []any:
			for _, x := range v {
				walk(x, path+"[]")
			}
		}
	}
	var calls int
	copies := map[string]bool{}
	wireDiffers := map[string]bool{}
	for _, l := range strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n") {
		var line struct {
			Type       string                     `json:"type"`
			Subtype    string                     `json:"subtype"`
			Wire       map[string]json.RawMessage `json:"wireToolInputs"`
			Attachment struct {
				Type   string `json:"type"`
				Copies []struct {
					ID   string `json:"id"`
					Copy string `json:"copy"`
				} `json:"toolInputCopies"`
			} `json:"attachment"`
			Message struct {
				Content         json.RawMessage `json:"content"`
				Transformations json.RawMessage `json:"input_transformations"`
			} `json:"message"`
		}
		if err := json.Unmarshal([]byte(l), &line); err != nil {
			t.Fatal(err)
		}
		var generic any
		_ = json.Unmarshal([]byte(l), &generic)
		walk(generic, "")
		if line.Type == "system" && line.Subtype != "stop_hook_summary" {
			t.Errorf("a system line of subtype %q: the transcript records a hook other than the Stop hook", line.Subtype)
		}
		if len(line.Message.Transformations) > 0 && string(line.Message.Transformations) != "[]" {
			t.Errorf("input_transformations is %s: a fixture has a transformation", line.Message.Transformations)
		}
		for _, c := range line.Attachment.Copies {
			if c.Copy != "wire" {
				t.Errorf("toolInputCopies entry %+v is not a wire copy", c)
			}
			copies[c.ID] = true
		}
		var blocks []struct {
			Type  string          `json:"type"`
			ID    string          `json:"id"`
			Input json.RawMessage `json:"input"`
		}
		if json.Unmarshal(line.Message.Content, &blocks) != nil {
			continue
		}
		for _, b := range blocks {
			if b.Type != "tool_use" {
				continue
			}
			calls++
			if !sameJSON(pre[b.ID], b.Input) || !sameJSON(post[b.ID], b.Input) {
				t.Errorf("%s: the block's input, the PreToolUse input and the PostToolUse input are not one value:\n block %s\n pre   %s\n post  %s", b.ID, b.Input, pre[b.ID], post[b.ID])
			}
			wire, ok := line.Wire[b.ID]
			if !ok {
				t.Errorf("%s: no wireToolInputs entry", b.ID)
				continue
			}
			if sameJSON(wire, b.Input) {
				continue
			}
			wireDiffers[b.ID] = true
			var w, in map[string]any
			_ = json.Unmarshal(wire, &w)
			_ = json.Unmarshal(b.Input, &in)
			extra := map[string]any{}
			for k, v := range in {
				if wv, ok := w[k]; !ok || !reflect.DeepEqual(wv, v) {
					extra[k] = v
				}
			}
			if !reflect.DeepEqual(extra, map[string]any{"replace_all": false}) || len(w) != len(in)-1 {
				t.Errorf("%s: the wire input differs from the block's by more than replace_all: false: wire %s, block %s", b.ID, wire, b.Input)
			}
		}
	}
	if calls != 7 || len(pre) != 7 {
		t.Fatalf("%d tool_use blocks and %d PreToolUse payloads", calls, len(pre))
	}
	if !reflect.DeepEqual(copies, wireDiffers) || len(copies) != 2 {
		t.Errorf("toolInputCopies names %v, the calls whose wire input differs are %v", sortedKeys(copies), sortedKeys(wireDiffers))
	}

	// The replay: declared and executed digests agree on every call, so the
	// recorder's check cannot fire; and the flag is on every session.
	_, res := runRebuilt(t, f.openTranscript(t), f.opt)
	if len(res.Sessions) != 1 || !contains(res.Sessions[0].Flags, flagRewriteAbsent) || !reflect.DeepEqual(res.Sessions[0].Flags, replayFlags) {
		t.Fatalf("sessions: %+v", res.Sessions)
	}
	d := newTdoc()
	d.promptLine("p1")
	d.uses("m1", tBash("t1", "make"))
	d.results("", tOK("t1"))
	d.ends("m2", "done")
	syn, _ := replayDoc(t, d.reader(), replay.Options{})
	if len(syn.Sessions) != 1 || !contains(syn.Sessions[0].Flags, flagRewriteAbsent) {
		t.Fatalf("synthetic session: %+v", syn.Sessions)
	}
	rep, _ := runRebuilt(t, f.openTranscript(t), f.opt)
	dump := dumpStore(t, rep.root)
	decls, execs := recordsOf(dump, "records.ndjson", "declaration"), recordsOf(dump, "records.ndjson", "execution")
	if len(execs) == 0 {
		t.Fatal("no executions")
	}
	for _, e := range execs {
		d := byTool(decls, e["tool_use_id"].(string))
		if got, want := e["executed_digest"], d["shape"].(map[string]any)["digest"]; got != want {
			t.Errorf("%v: executed digest %v differs from the declared %v in a replay", e["tool_use_id"], got, want)
		}
	}

	// What is lost: the same check does fire on a live run whose PostToolUse
	// input differs from its PreToolUse input.
	steps, ids := f.liveSteps(t)
	for i, s := range steps {
		if s.Event == install.EventPostToolUse && ids[i] == "toolu_fixture03" {
			steps[i].Raw = bytes.Replace(s.Raw, []byte("cd sub && python3 -m unittest"), []byte("cd sub && python3 -m unittest | tail -3"), 1)
		}
	}
	live := runLive(t, steps, ids)
	ld := dumpStore(t, live.root)
	lDecls, lExecs := recordsOf(ld, "records.ndjson", "declaration"), recordsOf(ld, "records.ndjson", "execution")
	differ := 0
	for _, e := range lExecs {
		if e["executed_digest"] != byTool(lDecls, e["tool_use_id"].(string))["shape"].(map[string]any)["digest"] {
			differ++
		}
	}
	if differ != 1 {
		t.Errorf("a rewritten PostToolUse input made %d executed digests differ from their declarations, want 1: the recorder's check is not what the flag stands for", differ)
	}
}

// TestREPLAY17_TheReplayStartsEachSessionItself: a transcript has no
// SessionStart, and a run with no liveness marker reads
// coverage_unverified, which silences every verdict (verify.recording: no
// gates, no claims, no record green). So the replay feeds the recorder a
// SessionStart of its own at the session's first event. This shows the
// necessity (the same events without it give an incomplete record at the Stop),
// that the synthesized payload is enough (the store it leaves equals the one
// the captured SessionStart leaves, TestREPLAY10_), and which of its keys the
// probe reads.
func TestREPLAY17_TheReplayStartsEachSessionItself(t *testing.T) {
	f := loadFixtureSession(t, "2.1.312")
	steps, ids := f.liveSteps(t)

	withStart := runLive(t, steps, ids)
	withoutStart := runLive(t, steps[1:], ids[1:])
	if v := withStart.verdicts[0].Snap; v.Recording != verify.Complete || len(v.Gates) != 1 {
		t.Fatalf("with a SessionStart: recording %s, %d gates", v.Recording, len(v.Gates))
	}
	v := withoutStart.verdicts[0].Snap
	if v.Recording != verify.Incomplete || len(v.Gates) != 0 || !contains(v.Notes, "coverage_unverified") {
		t.Errorf("without a SessionStart: recording %s, %d gates, notes %v; the synthesized start would not be needed", v.Recording, len(v.Gates), v.Notes)
	}

	// The synthesized payload, key by key: the probe reads session_id and cwd;
	// the rest is unread under the pinned host guard, as are the two keys
	// Claude Code's own carries beyond them.
	_, res := runRebuilt(t, f.openTranscript(t), f.opt)
	if res.Probes != 1 {
		t.Fatalf("%d probes fed", res.Probes)
	}
	first := f.tl.Events[0]
	start, err := sessionStart(first.Payload)
	if err != nil {
		t.Fatal(err)
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(start, &keys); err != nil {
		t.Fatal(err)
	}
	var names []string
	for k := range keys {
		names = append(names, k)
	}
	sort.Strings(names)
	if want := []string{"cwd", "hook_event_name", "session_id", "transcript_path"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("the synthesized SessionStart has keys %v, want %v", names, want)
	}
	run := func(raw []byte) []storedRecord {
		st := append([]replayStep{{install.EventSessionStart, raw, first.At}}, steps[1:]...)
		return dumpStore(t, runLive(t, st, ids).root)
	}
	base := run(start)
	removed := func(keys ...string) []byte {
		m := map[string]json.RawMessage{}
		_ = json.Unmarshal(start, &m)
		for _, k := range keys {
			delete(m, k)
		}
		b, _ := json.Marshal(m)
		return b
	}
	for _, k := range []string{"session_id", "cwd"} {
		if len(storeDiffs(base, run(removed(k)), 1)) == 0 {
			t.Errorf("removing %s from the SessionStart changes nothing: the probe does not read it", k)
		}
	}
	for _, k := range []string{"hook_event_name", "transcript_path"} {
		if d := storeDiffs(base, run(removed(k)), 3); len(d) > 0 {
			t.Errorf("removing %s from the SessionStart changes the store: %v", k, d)
		}
	}
	extra := func() []byte {
		m := map[string]json.RawMessage{}
		_ = json.Unmarshal(start, &m)
		m["source"] = json.RawMessage(`"startup"`)
		m["scratchpad_dir"] = json.RawMessage(`"$TMPDIR/claude-fixture/scratchpad"`)
		b, _ := json.Marshal(m)
		return b
	}
	if d := storeDiffs(base, run(extra()), 3); len(d) > 0 {
		t.Errorf("Claude Code's own extra keys (source, scratchpad_dir) change the store: %v", d)
	}
}
