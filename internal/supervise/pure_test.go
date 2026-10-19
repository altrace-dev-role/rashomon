package supervise

import (
	"strings"
	"testing"
	"time"
)

// TestSTR6c_TheHardStopIsTheEntrysTimeoutLessItsMargin: every hard stop is
// derived from the installed entry's timeout: less 1 s at 5 s and above,
// less 100 ms on a short budget. SessionEnd's budget is Claude Code's 1.5 s
// default whatever the entry says, lowered -- never raised -- by
// CLAUDE_CODE_SESSIONEND_HOOKS_TIMEOUT_MS.
func TestSTR6c_TheHardStopIsTheEntrysTimeoutLessItsMargin(t *testing.T) {
	for _, c := range []struct {
		name     string
		timeoutS int
		events   []string
		env      string
		want     time.Duration
	}{
		{"hook", 5, []string{"PreToolUse"}, "", 4 * time.Second},
		{"post", 5, []string{"PostToolUse", "PostToolUseFailure"}, "", 4 * time.Second},
		{"recap", 10, []string{"Stop", "StopFailure"}, "", 9 * time.Second},
		{"probe start", 5, []string{"SessionStart"}, "", 4 * time.Second},
		{"probe end", 5, []string{"SessionEnd"}, "", 1400 * time.Millisecond},
		{"probe end, env lower", 5, []string{"SessionEnd"}, "800", 700 * time.Millisecond},
		{"probe end, env higher is ignored", 5, []string{"SessionEnd"}, "5000", 1400 * time.Millisecond},
		{"probe end, env junk is ignored", 5, []string{"SessionEnd"}, "soon", 1400 * time.Millisecond},
		{"probe end, env zero is ignored", 5, []string{"SessionEnd"}, "0", 1400 * time.Millisecond},
		{"probe end, env tiny", 5, []string{"SessionEnd"}, "150", 75 * time.Millisecond},
		{"a 2 s entry", 2, []string{"PreToolUse"}, "", 1900 * time.Millisecond},
		{"no timeout", 0, []string{"PreToolUse"}, "", 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := HardStop(c.timeoutS, c.events, c.env); got != c.want {
				t.Errorf("HardStop(%d, %v, %q) = %v, want %v", c.timeoutS, c.events, c.env, got, c.want)
			}
		})
	}
}

// TestTRU50b_TheClockRecordReadsBackOnlyWhole: the record the parent writes
// on the pipe is the one thing that turns a process into the supervised
// child, so anything but a whole, well-formed record of this version is
// refused.
func TestTRU50b_TheClockRecordReadsBackOnlyWhole(t *testing.T) {
	start := time.Unix(1_700_000_000, 123_456_789)
	rec := clockRecord(start, 4*time.Second)
	got, stop, ok := parseClockRecord(rec)
	if !ok || !got.Equal(start) || stop != 4*time.Second {
		t.Fatalf("parseClockRecord(%q) = %v, %v, %v", rec, got, stop, ok)
	}
	for _, bad := range []string{
		"",
		strings.TrimSuffix(string(rec), "\n"),
		"rashomon-supervisor 2 1 1\n",
		"rashomon-supervisor 1 1\n",
		"rashomon-supervisor 1 x 1\n",
		"rashomon-supervisor 1 1 -5\n",
		"rashomon-supervisor 1 1 0\n",
		"something-else 1 1 1\n",
		"rashomon-supervisor 1 1 1\nmore",
	} {
		if _, _, ok := parseClockRecord([]byte(bad)); ok {
			t.Errorf("parseClockRecord(%q) accepted", bad)
		}
	}
}

// TestREC55d_TheHeadScanFindsTheIDsInTheFirst4KiB: the parent reads the
// session id and event from the first 4 KiB it pumps, and only values that
// end inside it: a value the cut runs through is not read as a shorter one.
func TestREC55d_TheHeadScanFindsTheIDsInTheFirst4KiB(t *testing.T) {
	full := []byte(`{"session_id":"sess-1","hook_event_name":"PreToolUse","tool_input":{"command":"` + strings.Repeat("x", 8000) + `"}}`)
	if got := scanHead(full[:headBytes]); got.SessionID != "sess-1" || got.Event != "PreToolUse" {
		t.Errorf("scanHead = %+v", got)
	}
	cut := []byte(`{"tool_input":{"command":"` + strings.Repeat("x", 4070) + `"},"session_id":"sess-cut-through-here"}`)
	if got := scanHead(cut[:headBytes]); got.SessionID != "" {
		t.Errorf("a session id the cut runs through read as %q", got.SessionID)
	}
	nested := []byte(`{"tool_input":{"session_id":"inner","hook_event_name":"Stop"},"session_id":"outer"}`)
	if got := scanHead(nested); got.SessionID != "outer" || got.Event != "" {
		t.Errorf("nested keys read as top-level ones: %+v", got)
	}
	if got := scanHead([]byte(`not json`)); got != (head{}) {
		t.Errorf("junk read as %+v", got)
	}
	if got := scanHead([]byte(`{"session_id":7}`)); got.SessionID != "" {
		t.Errorf("a number read as a session id: %+v", got)
	}
}

// TestSTR4d_TheEventIsThePayloadsWhenTheEntryServesIt: the event the output
// gate checks against is the payload's hook_event_name when the entry serves
// it, the entry's one event when it serves only one, and none otherwise.
func TestSTR4d_TheEventIsThePayloadsWhenTheEntryServesIt(t *testing.T) {
	post := []string{"PostToolUse", "PostToolUseFailure"}
	for _, c := range []struct {
		events  []string
		payload string
		want    string
	}{
		{post, "PostToolUseFailure", "PostToolUseFailure"},
		{post, "PreToolUse", ""},
		{post, "", ""},
		{[]string{"PreToolUse"}, "", "PreToolUse"},
		{[]string{"PreToolUse"}, "Stop", "PreToolUse"},
		{nil, "Stop", ""},
	} {
		if got := eventOf(c.events, c.payload); got != c.want {
			t.Errorf("eventOf(%v, %q) = %q, want %q", c.events, c.payload, got, c.want)
		}
	}
}
