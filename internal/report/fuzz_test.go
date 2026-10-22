package report

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

// Fuzz targets over the transcript readers, which read Claude Code's file and
// whatever a session put in it. Seeded with every line of the transcript a live
// Claude Code 2.1.280 session wrote (realDir).
//
// The oracles are properties of the readers' contract rather than a second
// implementation of them:
//
//   - lines are independent: what TranscriptIDs and FinalAssistantText read
//     from a file is what they read from its lines one at a time, so a line
//     that does not parse costs that line and nothing after it;
//   - an id or text written by encoding/json reads back as written;
//   - topLevelSidechain agrees with encoding/json about a valid line's
//     top-level isSidechain.

func realLines(t testing.TB) []string {
	t.Helper()
	raw, err := os.ReadFile(realDir + "transcript.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
}

// sidechainSeed is a subagent line as the main transcript can carry one:
// isSidechain true, and not the first key. The captured session ran no
// subagent, so none of its lines is one.
const sidechainSeed = `{"parentUuid":null,"isSidechain":true,"type":"assistant","message":{"role":"assistant","content":7}}`

// maxFuzzBytes bounds what FuzzTranscriptLines checks. Each line of an input
// costs a file and two reads, so an exec here is slow, and the larger the
// input the slower. That is why CI bounds minimization (ci.yaml): at its
// one-minute default the fuzzer spends its window reducing one input, 5,584
// execs in 20 s and then 0/s (2026-10-13, an M-series Mac), and 1,330 with
// maxFuzzBytes alone. With both, the same 20 s ran 216,016 execs and kept 79 new
// inputs. The whole transcript is still checked, once, by
// TestRubricTEST1_TranscriptSeeds; the fuzzer gets its lines singly and in
// runs of three, which keep the across-line cases at a few KB.
const maxFuzzBytes = 16 << 10

func FuzzTranscriptLines(f *testing.F) {
	lines := realLines(f)
	for i, l := range lines {
		f.Add([]byte(l))
		if i+3 <= len(lines) {
			f.Add([]byte(strings.Join(lines[i:i+3], "\n")))
		}
	}
	f.Add([]byte(lines[len(lines)/2] + "\n{\"message\":\n" + lines[len(lines)/2+1]))
	f.Add([]byte(sidechainSeed))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > maxFuzzBytes {
			return
		}
		checkTranscriptLines(t, data)
	})
}

// maxFuzzLines bounds the per-line comparison, which writes one file a line.
const maxFuzzLines = 128

func checkTranscriptLines(t *testing.T, data []byte) {
	t.Helper()
	dir := t.TempDir()
	write := func(name string, b []byte) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, b, 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	whole := write("whole.jsonl", data)
	ids, results, denied, unhooked, files, _, err := transcriptIDsAt(whole)
	if err != nil || files != 1 {
		t.Fatalf("TranscriptIDs: files %d, %v", files, err)
	}
	final, finalOK := FinalAssistantText(openT(t, whole))

	lines := bytes.Split(data, []byte("\n"))
	if len(lines) > maxFuzzLines {
		return
	}
	union := [4]map[string]bool{{}, {}, {}, {}}
	var last string
	lastOK := false
	for i, l := range lines {
		checkSidechain(t, l)
		p := write("line"+strconv.Itoa(i)+".jsonl", append(l, '\n'))
		a, b, c, d, _, _, err := transcriptIDsAt(p)
		if err != nil {
			t.Fatal(err)
		}
		for j, m := range []map[string]bool{a, b, c, d} {
			for k := range m {
				union[j][k] = true
			}
		}
		if text, ok := FinalAssistantText(openT(t, p)); ok {
			last, lastOK = text, true
		}
	}
	for j, got := range []map[string]bool{ids, results, denied, unhooked} {
		if !sameSet(got, union[j]) {
			t.Fatalf("set %d read from the file is %v, from its lines one at a time %v", j, got, union[j])
		}
	}
	if final != last || finalOK != lastOK {
		t.Fatalf("FinalAssistantText of the file %q (%v), of its last line with text %q (%v)", final, finalOK, last, lastOK)
	}
}

// checkSidechain compares topLevelSidechain with encoding/json on a valid
// object whose top level can name isSidechain at most once: with no \u escape
// the key can only be spelled one way, and one occurrence of the word bounds
// it to one key. Duplicate keys are where the two may differ, and Claude Code
// writes none.
func checkSidechain(t *testing.T, line []byte) {
	t.Helper()
	var m map[string]any
	if json.Unmarshal(line, &m) != nil || bytes.Contains(line, []byte(`\u`)) ||
		bytes.Count(line, []byte("isSidechain")) > 1 {
		return
	}
	want := m["isSidechain"] == true
	if got := topLevelSidechain(line); got != want {
		t.Fatalf("topLevelSidechain(%q) = %v, encoding/json says %v", line, got, want)
	}
}

func FuzzTranscriptBlocks(f *testing.F) {
	f.Add("toolu_z", "All 12 tests pass.", false)
	f.Add("toolu_x", "Permission to use Bash with command rm -f out.txt has been denied.", true)
	f.Add("toolu_y", "<tool_use_error>File has not been read yet.", true)
	f.Add("", "  ", true)
	f.Add("id \"quoted\" \\  ", "line\nbreak\t\u0000", false)
	f.Fuzz(func(t *testing.T, id, text string, isError bool) { checkTranscriptBlocks(t, id, text, isError) })
}

// checkTranscriptBlocks writes one call and its result the way Claude Code
// shapes them, through encoding/json, and reads them back.
func checkTranscriptBlocks(t *testing.T, id, text string, isError bool) {
	t.Helper()
	if !utf8.ValidString(id) || !utf8.ValidString(text) {
		return // encoding/json would write U+FFFD, a different string
	}
	asst, err := json.Marshal(map[string]any{"type": "assistant", "message": map[string]any{
		"role": "assistant", "content": []any{
			map[string]any{"type": "text", "text": text},
			map[string]any{"type": "tool_use", "id": id, "name": "Bash", "input": map[string]any{}},
		}}})
	if err != nil {
		t.Fatal(err)
	}
	user, err := json.Marshal(map[string]any{"type": "user", "message": map[string]any{
		"role": "user", "content": []any{
			map[string]any{"type": "tool_result", "tool_use_id": id, "is_error": isError, "content": text},
		}}})
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "t.jsonl")
	if err := os.WriteFile(p, []byte(string(asst)+"\n"+string(user)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ids, results, denied, unhooked, _, _, err := transcriptIDsAt(p)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{}
	if id != "" {
		want[id] = true
	}
	if !sameSet(ids, want) {
		t.Fatalf("tool_use id %q reads back as %v", id, ids)
	}
	var none map[string]bool
	wantDenied, wantResult := none, want
	if isDenial(isError, text) {
		wantDenied, wantResult = want, none
	}
	if !sameSet(results, wantResult) || !sameSet(denied, wantDenied) {
		t.Fatalf("result for %q (error %v, %q): results %v denied %v", id, isError, text, results, denied)
	}
	if wantUnhooked := isError && strings.HasPrefix(text, toolUseErrorTag) && id != ""; wantUnhooked != unhooked[id] || len(unhooked) > 1 {
		t.Fatalf("unhooked %v for %q (error %v, %q)", unhooked, id, isError, text)
	}
	got, ok := FinalAssistantText(openT(t, p))
	if wantOK := strings.TrimSpace(text) != ""; ok != wantOK || (ok && got != text) {
		t.Fatalf("FinalAssistantText %q (%v), want %q (%v)", got, ok, text, wantOK)
	}
}

// TestRubricTEST1_TranscriptSeeds runs both fuzz properties over their seeds
// as a plain test, which is what a mutant names.
func TestRubricTEST1_TranscriptSeeds(t *testing.T) {
	lines := realLines(t)
	checkTranscriptLines(t, []byte(strings.Join(lines, "\n")))
	for _, l := range append(lines, sidechainSeed) {
		checkSidechain(t, []byte(l))
	}
	checkTranscriptBlocks(t, "toolu_z", "All 12 tests pass.", false)
	checkTranscriptBlocks(t, "toolu_x", "Permission to use Bash with command rm -f out.txt has been denied.", true)
	checkTranscriptBlocks(t, "toolu_y", "<tool_use_error>File has not been read yet.", true)
}
