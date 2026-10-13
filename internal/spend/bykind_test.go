package spend

import (
	"testing"
	"time"
)

// TestRubricNAMING2_EachKindIsPricedAtItsOwnRate holds every row of by_kind
// to an exact figure: one priced response with a different count of each kind
// of token, and one response of a model the table does not know. Nanodollars
// and token counts used to cross the same bare int64 signature, so a call that
// passed a kind's tokens where its price belonged compiled, passed every test
// (only input and cache-write rows were asserted) and printed a token count as
// dollars.
func TestRubricNAMING2_EachKindIsPricedAtItsOwnRate(t *testing.T) {
	c := newConfig(t)
	at := now.Add(-time.Hour)
	c.write("proj/sess-a.jsonl",
		resp{id: "a", model: "claude-opus-5-5", at: at, in: 1000, out: 300, read: 7000, w5: 50, stop: "end_turn"}.line("text"),
		resp{id: "b", model: "claude-mystery-9", at: at, in: 11, out: 13, read: 17, w5: 19, stop: "end_turn"}.line("text"))
	s := c.summary(30)
	for _, row := range []struct {
		name               string
		got                Cost
		nano, unpricedToks int64
	}{
		{"input", s.ByKind.Input, 1000 * opusIn, 11},
		{"output", s.ByKind.Output, 300 * opusOut, 13},
		{"cache_read", s.ByKind.CacheRead, 7000 * opusRead, 17},
		{"cache_write", s.ByKind.CacheWrite, 50 * opusW5, 19},
	} {
		want := Cost{Nano: row.nano, Priced: 1, Unpriced: 1, UnpricedTokens: row.unpricedToks}
		if row.got != want {
			t.Errorf("by_kind %s = %+v, want %+v", row.name, row.got, want)
		}
	}
}
