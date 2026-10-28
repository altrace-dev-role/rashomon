package main

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestSTOP_PayloadBooleanDoesNotAbortTheRead: stop_hook_active is a JSON
// boolean, read without aborting the stream (a String path would refuse it
// and lose every field after it; QA B4), and cwd is read beside it (card 2.4
// R11). The boolean comes before the message, so an aborted read would lose
// the message.
func TestSTOP_PayloadBooleanDoesNotAbortTheRead(t *testing.T) {
	for _, active := range []bool{true, false} {
		in := fmt.Sprintf(`{"session_id":"sess-1","hook_event_name":"Stop","stop_hook_active":%t,"cwd":"/work/repo","last_assistant_message":"All 42 tests pass."}`, active)
		p := readRecapPayload(strings.NewReader(in), 5*time.Second)
		if !p.OK || p.SessionID != "sess-1" || p.LastAssistantMessage != "All 42 tests pass." || p.Harness.HookEventName != "Stop" {
			t.Fatalf("stop_hook_active %t: payload %+v, want it read whole", active, p)
		}
		if p.StopHookActive != active || p.CWD != "/work/repo" {
			t.Errorf("stop_hook_active %t: StopHookActive %t CWD %q, want %t and /work/repo", active, p.StopHookActive, p.CWD, active)
		}
	}
}
