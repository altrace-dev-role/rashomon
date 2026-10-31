package main

import "testing"

// TestSTC1p_SafeTextMapsWhatAHostileTerminalReads (card STEER-CLI1): the
// verbs print values a hand wrote (a project key, a session id), so C0 and C1
// controls and the bidi overrides and isolates never reach the terminal as
// themselves; the characters next to them, and ordinary non-ASCII text, stay.
func TestSTC1p_SafeTextMapsWhatAHostileTerminalReads(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"plain /tmp/p", "plain /tmp/p"},
		{"caf\u00e9 \u65e5\u672c", "caf\u00e9 \u65e5\u672c"},
		{"a\x00b\x1bc\x7fd", "a?b?c?d"},
		{"\u0080\u0085\u009b\u009f", "????"},
		{"\u00a0\u00a1", "\u00a0\u00a1"},
		{"\u202a\u202b\u202c\u202d\u202e", "?????"},
		{"\u2066\u2067\u2068\u2069", "????"},
		{"\u2029\u202f\u2065\u206a", "\u2029\u202f\u2065\u206a"},
	} {
		if got := safeText(c.in); got != c.want {
			t.Errorf("safeText(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
