package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRubricTEST8_MessageCapAtNAndNPlusOne: a caller's final message is cut
// to messageCap runes before it is compared with anything: exactly that many
// are kept whole, and one more is cut back to them.
func TestRubricTEST8_MessageCapAtNAndNPlusOne(t *testing.T) {
	if messageCap != 64<<10 {
		t.Fatalf("messageCap = %d, want 64 Ki", messageCap)
	}
	for _, n := range []int{messageCap, messageCap + 1} {
		got := sanitizeMessage(strings.Repeat("é", n))
		if len([]rune(got)) != min(n, messageCap) {
			t.Errorf("%d runes in: %d kept, want %d", n, len([]rune(got)), min(n, messageCap))
		}
	}
}

// TestRubricTEST8_TranscriptLineAtNAndNPlusOne: a transcript line of maxLine
// bytes, counting its newline, is read and its tool call collected; one byte
// more stops the read with an error rather than a silent truncation.
func TestRubricTEST8_TranscriptLineAtNAndNPlusOne(t *testing.T) {
	const head, tail = `{"message":{"content":[{"type":"tool_use","id":"toolu_n"}]},"pad":"`, "\"}\n"
	if maxLine != 64<<20 {
		t.Fatalf("maxLine = %d, want 64 MiB", maxLine)
	}
	for _, tc := range []struct {
		n    int
		read bool
	}{{maxLine, true}, {maxLine + 1, false}} {
		path := filepath.Join(t.TempDir(), "t.jsonl")
		if err := os.WriteFile(path, []byte(head+strings.Repeat("x", tc.n-len(head)-len(tail))+tail), 0o600); err != nil {
			t.Fatal(err)
		}
		ids := map[string]bool{}
		_, err := collectIDs(path, ids, map[string]bool{}, map[string]bool{}, map[string]bool{})
		if read := err == nil && ids["toolu_n"]; read != tc.read {
			t.Errorf("a %d-byte line: collected %v, %v; want read %v", tc.n, ids, err, tc.read)
		}
	}
}
