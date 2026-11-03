package payload

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRES28f_PayloadSizesAreExact(t *testing.T) {
	for _, n := range []int{400, 540, 4096, 4097, 40000, 1 << 20, 4 << 20, 8 << 20, 8<<20 + 1} {
		pre, err := Pre(n, "toolu_01")
		if err != nil {
			t.Fatalf("Pre(%d): %v", n, err)
		}
		if len(pre) != n || !json.Valid(pre) {
			t.Errorf("Pre(%d): %d bytes, valid %v", n, len(pre), json.Valid(pre))
		}
		if n >= 540 { // a PostToolUse envelope's fixed part is above 400 bytes
			post, err := Post(n, "toolu_01")
			if err != nil {
				t.Fatalf("Post(%d): %v", n, err)
			}
			if len(post) != n || !json.Valid(post) {
				t.Errorf("Post(%d): %d bytes, valid %v", n, len(post), json.Valid(post))
			}
		}
		var doc map[string]any
		if err := json.Unmarshal(pre, &doc); err != nil {
			t.Fatal(err)
		}
		if want := map[bool]string{true: "Bash", false: "Edit"}[n <= smallShape]; doc["tool_name"] != want {
			t.Errorf("Pre(%d) is a %v payload, want %s", n, doc["tool_name"], want)
		}
	}
	if _, err := EditPre(10, "x"); err == nil {
		t.Error("a total below the fixed part built something")
	}
	if _, err := BashPreExact(10, "x"); err == nil {
		t.Error("a total below the Bash fixed part built something")
	}
	// The size check itself: a builder that misses the total by one byte in
	// either direction is refused, so a padding rule that drifts cannot hand
	// the harness a payload one byte off the size its budget names.
	for _, off := range []int{-1, 1} {
		off := off
		_, err := exact(100, func(string) []byte { return make([]byte, 100+off) })
		if err == nil {
			t.Errorf("a builder %d byte(s) off the total was accepted", off)
		}
	}
	if out, err := exact(100, func(pad string) []byte { return []byte(pad) }); err != nil || len(out) != 100 {
		t.Errorf("an exact builder was refused: %v, %d bytes", err, len(out))
	}
	var doc map[string]any
	if err := json.Unmarshal(BashPost("git status --short", "toolu_01", " M README.md\n"), &doc); err != nil || doc["hook_event_name"] != "PostToolUse" || doc["tool_response"] == nil {
		t.Errorf("BashPost: %v %v", err, doc)
	}
	for _, p := range [][]byte{BashPre("git status", "t"), Stop("done"), Prompt("next"), SessionStart(), SessionEnd()} {
		if !json.Valid(p) {
			t.Errorf("invalid payload %s", p)
		}
	}
}

// TestBashPostExactAtIsExact: the order case's payloads, whose cwd is a
// project directory under /tmp, are exactly the class's size, and a Pre of
// the same cwd names the same directory.
func TestBashPostExactAtIsExact(t *testing.T) {
	for _, dir := range []string{"/tmp/rpo-1234567890", "/private/tmp/rpo-1234567890"} {
		p, err := BashPostExactAt(540, "toolu_00", dir)
		if err != nil || len(p) != 540 {
			t.Fatalf("cwd %s: %d bytes, %v, want 540", dir, len(p), err)
		}
		if !strings.Contains(string(p), `"cwd":"`+dir+`"`) || !strings.Contains(string(BashPreAt("git status --short", "toolu_00", dir)), `"cwd":"`+dir+`"`) {
			t.Errorf("cwd %s is not in the payloads", dir)
		}
	}
}
