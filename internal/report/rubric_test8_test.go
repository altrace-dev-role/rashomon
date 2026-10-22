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

// TestRubricTEST8_TranscriptLineAtNAndNPlusOne: a transcript line of
// TranscriptMaxLine bytes, counting its newline, is read and its tool call
// collected; one byte more is stepped over and counted, not read and not an
// error, and the line after it is read.
func TestRubricTEST8_TranscriptLineAtNAndNPlusOne(t *testing.T) {
	const head, tail = `{"message":{"content":[{"type":"tool_use","id":"toolu_n"}]},"pad":"`, "\"}\n"
	const next = `{"message":{"content":[{"type":"tool_use","id":"toolu_next"}]}}` + "\n"
	if TranscriptMaxLine != 64<<20 {
		t.Fatalf("TranscriptMaxLine = %d, want 64 MiB", TranscriptMaxLine)
	}
	for _, tc := range []struct {
		n    int
		read bool
	}{{TranscriptMaxLine, true}, {TranscriptMaxLine + 1, false}} {
		path := filepath.Join(t.TempDir(), "t.jsonl")
		if err := os.WriteFile(path, []byte(head+strings.Repeat("x", tc.n-len(head)-len(tail))+tail+next), 0o600); err != nil {
			t.Fatal(err)
		}
		ids := map[string]bool{}
		long, err := collectIDs(openT(t, path), ids, map[string]bool{}, map[string]bool{}, map[string]bool{})
		wantLong := 0
		if !tc.read {
			wantLong = 1
		}
		if err != nil || ids["toolu_n"] != tc.read || !ids["toolu_next"] || long != wantLong {
			t.Errorf("a %d-byte line: collected %v, %d too long, %v; want read %v, %d too long, the next line read", tc.n, ids, long, err, tc.read, wantLong)
		}
	}
}
