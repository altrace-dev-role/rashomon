package report

import (
	"bytes"
	"strings"
	"testing"
)

// TestRP2zj_TheBaselineLineSaysWhenItsWalkWasCut (card REPORT1b review): a
// null baseline reads "no earlier checkpoint" only when the walk read the
// store; a cut walk says what it walked, and a baseline it found says a
// newer one may exist. Break: ignore the cap in the text.
func TestRP2zj_TheBaselineLineSaysWhenItsWalkWasCut(t *testing.T) {
	bl := &KnowledgeBaseline{SessionID: "base", RecordedAtUnixMS: 1_761_700_000_000, PointSeq: 9}
	for _, tc := range []struct {
		name string
		k    Knowledge
		want string
		not  string
	}{
		{"none, read the store", Knowledge{Project: "/p"}, "baseline: no earlier checkpoint of this project", "walked"},
		{"none, walk cut", Knowledge{Project: "/p", BaselineWalkCap: 1024}, "baseline: not found in the first 1024 session directories walked", "no earlier"},
		{"found, walk cut", Knowledge{Project: "/p", Baseline: bl, BaselineWalkCap: 1024}, "a newer one may exist", "the project's newest before"},
		{"found, read the store", Knowledge{Project: "/p", Baseline: bl}, "the project's newest before this session", "walked"},
	} {
		var b bytes.Buffer
		k := tc.k
		writeKnowledge(&b, &k)
		if got := strings.Join(strings.Fields(b.String()), " "); !strings.Contains(got, tc.want) || strings.Contains(got, tc.not) {
			t.Errorf("%s:\n%s", tc.name, got)
		}
	}
}
