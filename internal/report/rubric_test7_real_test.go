package report

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"testing"
)

// realDir is one live Claude Code 2.1.280 session, captured and redacted as
// test/acceptance/rubric_test7_real_payloads_test.go describes.
const realDir = "../../test/fixtures/claude-code/2.1.280/"

// realIDs reads, from the captured hook payloads, the ids Claude Code
// announced in PreToolUse, the ids it reported finished, and the final message
// the Stop payload carried.
func realIDs(t *testing.T) (declared, finished map[string]bool, final string) {
	t.Helper()
	f, err := os.Open(realDir + "payloads.ndjson")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	declared, finished = map[string]bool{}, map[string]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var p struct {
			Event     string `json:"hook_event_name"`
			ToolUseID string `json:"tool_use_id"`
			Final     string `json:"last_assistant_message"`
		}
		if err := json.Unmarshal(sc.Bytes(), &p); err != nil {
			t.Fatal(err)
		}
		switch p.Event {
		case "PreToolUse":
			declared[p.ToolUseID] = true
		case "PostToolUse", "PostToolUseFailure":
			finished[p.ToolUseID] = true
		case "Stop":
			final = p.Final
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return declared, finished, final
}

// TestRubricTest7_RealTranscriptAgreesWithTheHooks reads a transcript Claude
// Code wrote and checks it against the hook payloads the same session sent: two
// independent outputs of the sender, which a hand-built fixture cannot stand
// in for. The call a --disallowedTools rule refused fired PreToolUse and no
// PostToolUse; its tool_result is the only one a denial.
func TestRubricTest7_RealTranscriptAgreesWithTheHooks(t *testing.T) {
	declared, finished, final := realIDs(t)
	ids, results, denied, unhooked, files, err := TranscriptIDs(realDir + "transcript.jsonl")
	if err != nil || files != 1 {
		t.Fatalf("TranscriptIDs: files %d, %v", files, err)
	}
	if !sameSet(ids, declared) {
		t.Errorf("transcript tool_use ids %v, PreToolUse ids %v", ids, declared)
	}
	if !sameSet(results, finished) {
		t.Errorf("transcript results %v, PostToolUse ids %v", results, finished)
	}
	if len(denied) != 1 || len(unhooked) != 0 {
		t.Fatalf("denied %v unhooked %v, want the one refused call and nothing unhooked", denied, unhooked)
	}
	for id := range denied {
		if !declared[id] || finished[id] {
			t.Errorf("the denial %s is not the call that was declared and never finished", id)
		}
	}
	text, ok := FinalAssistantText(realDir + "transcript.jsonl")
	if !ok || text != final {
		t.Errorf("FinalAssistantText %q (%v), want the Stop payload's %q", text, ok, final)
	}
}

func sameSet(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

// TestRubricTest7_FixtureHoldsNoLiveRequestIDsOrSignatures: the fixture is
// in a public repository, and the API request ids and thinking signatures a
// live session carries name that account's requests to Anthropic. Nothing
// here reads either for what it is: spend matches requestIds only to each
// other, and no reader looks at a signature. Each is replaced by a stable fake
// of the same kind -- req_fixture and a two-digit number, distinct where the
// originals were, and fixture-signature and the same -- and this test holds
// every value in both files, at any depth, to that form.
func TestRubricTest7_FixtureHoldsNoLiveRequestIDsOrSignatures(t *testing.T) {
	want := map[string]*regexp.Regexp{
		"requestId": regexp.MustCompile(`^req_fixture[0-9]{2}$`),
		"signature": regexp.MustCompile(`^fixture-signature-[0-9]{2}$`),
	}
	seen := map[string]int{}
	var walk func(key string, v any)
	walk = func(key string, v any) {
		switch v := v.(type) {
		case map[string]any:
			for k, e := range v {
				walk(k, e)
			}
		case []any:
			for _, e := range v {
				walk(key, e)
			}
		default:
			re, ok := want[key]
			if !ok {
				return
			}
			seen[key]++
			if s, isString := v.(string); !isString || !re.MatchString(s) {
				shown := fmt.Sprint(v)
				if len(shown) > 12 {
					shown = shown[:12] + "..." // a live value is not printed into a public log
				}
				t.Errorf("%s %s is not a fixture fake (%s)", key, shown, re)
			}
		}
	}
	for _, name := range []string{"transcript.jsonl", "payloads.ndjson"} {
		f, err := os.Open(realDir + name)
		if err != nil {
			t.Fatal(err)
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(nil, 1<<20)
		for sc.Scan() {
			var v any
			if err := json.Unmarshal(sc.Bytes(), &v); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			walk("", v)
		}
		err = sc.Err()
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	if seen["requestId"] == 0 || seen["signature"] == 0 {
		t.Fatalf("premise: the transcript carries requestIds and signatures, found %v", seen)
	}
}
