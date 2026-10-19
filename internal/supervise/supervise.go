// Package supervise runs a hook entry in a child process, so that nothing the
// entry does can make the hook exit non-zero or print what it should not.
//
// A Go runtime fatal error -- a concurrent map write, a stack overflow, an
// out-of-memory abort -- exits 2 whatever recover does, and exit 2 means
// something on every hook event: on PreToolUse it blocks the tool call, on
// UserPromptSubmit it blocks the prompt, on Stop it continues the turn. Only
// a parent process can turn that into silence. The parent is this same
// binary: it starts itself again as the child, streams the payload to it,
// kills it at a hard stop inside the entry's timeout, forwards what it
// printed only when it exited 0 and what it printed has the shape the event
// allows, and exits 0 whatever happened.
//
// The child is recognised by an argv token (Token) and an inherited pipe at
// fd 3 that carries the parent's clock -- both, never an environment
// variable, because a repository's settings can set the environment of
// every hook process and so could otherwise reopen the exit-2 path. The
// pipe is also how the child learns its parent has gone: end of file.
//
// The parent starts goroutines only through safe.Go and uses no maps.
package supervise

import (
	"bytes"
	"strconv"
	"strings"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/budget"
	"github.com/altrace-dev-role/rashomon/internal/jsonscan"
)

// unsupervisedClock is e's soft Clock for a process with no parent start to
// share: it starts now, with the table's budget. NewClock refuses only an
// entry past the end of the table, which has no budget; that one gets the
// zero Clock, already expired, as SoftBudget says it has.
func unsupervisedClock(e budget.DeadlineEntry) budget.Clock {
	c, err := budget.NewClock(time.Now(), e.SoftBudget())
	if err != nil {
		return budget.Clock{}
	}
	return c
}

// Token is the argv word that, with the clock pipe at fd 3, marks the
// supervised child. It comes first, before the entry's own arguments.
const Token = "--supervised-child"

// clockFD is the child's fd for the clock pipe: the first of ExtraFiles.
const clockFD = 3

// clockMagic and clockVersion open the clock record.
const (
	clockMagic   = "rashomon-supervisor"
	clockVersion = "1"
	// maxClockRecord bounds the record: the magic, the version and two
	// int64s in decimal, with their separators.
	maxClockRecord = 64
)

// Margins taken off an entry's timeout to give its hard stop. They cover the
// kill-and-trace path -- kill and reap the group, the coverage append under
// its 50 ms lock budget, one flush -- and process-start latency.
const (
	longMargin  = time.Second
	shortMargin = 100 * time.Millisecond
	// longBudget is the timeout from which the long margin applies.
	longBudget = 5 * time.Second
)

// SessionEnd's budget in Claude Code: 1.5 s by default, which a plugin
// hook's own timeout does not raise and which an environment variable can
// override. The supervisor reads the variable only to lower its stop.
const (
	eventSessionEnd      = "SessionEnd"
	sessionEndBudget     = 1500 * time.Millisecond
	SessionEndTimeoutEnv = "CLAUDE_CODE_SESSIONEND_HOOKS_TIMEOUT_MS"
)

// HardStop is how long after process start the supervisor kills an entry
// installed with timeoutS for events: the timeout less 1 s at 5 s and above,
// and less 100 ms on a shorter budget. On SessionEnd the budget is Claude
// Code's 1.5 s whatever the entry says, or sessionEndMS when that parses to
// a lower positive number of milliseconds; a higher value is ignored, so the
// variable can only shorten the stop. A budget too short for its margin
// gets half of itself. No timeout gives no stop: zero.
func HardStop(timeoutS int, events []string, sessionEndMS string) time.Duration {
	budget := time.Duration(timeoutS) * time.Second
	for _, ev := range events {
		if ev != eventSessionEnd {
			continue
		}
		budget = min(budget, sessionEndBudget)
		if ms, err := strconv.ParseInt(strings.TrimSpace(sessionEndMS), 10, 64); err == nil && ms > 0 {
			if d := time.Duration(ms) * time.Millisecond; d < budget {
				budget = d
			}
		}
	}
	switch {
	case budget <= 0:
		return 0
	case budget >= longBudget:
		return budget - longMargin
	case budget > 2*shortMargin:
		return budget - shortMargin
	}
	return budget / 2
}

// clockRecord is what the parent writes on the clock pipe before it starts
// the child: its process start and the hard stop measured from it, so both
// processes run one clock with one hard stop at the same instant.
func clockRecord(start time.Time, stop time.Duration) []byte {
	b := make([]byte, 0, maxClockRecord)
	b = append(b, clockMagic...)
	b = append(b, ' ')
	b = append(b, clockVersion...)
	b = append(b, ' ')
	b = strconv.AppendInt(b, start.UnixNano(), 10)
	b = append(b, ' ')
	b = strconv.AppendInt(b, int64(stop), 10)
	return append(b, '\n')
}

// parseClockRecord reads a whole clock record and nothing after it.
func parseClockRecord(b []byte) (start time.Time, stop time.Duration, ok bool) {
	line, rest, found := bytes.Cut(b, []byte{'\n'})
	if !found || len(rest) != 0 {
		return time.Time{}, 0, false
	}
	f := strings.Fields(string(line))
	if len(f) != 4 || f[0] != clockMagic || f[1] != clockVersion {
		return time.Time{}, 0, false
	}
	ns, err1 := strconv.ParseInt(f[2], 10, 64)
	st, err2 := strconv.ParseInt(f[3], 10, 64)
	if err1 != nil || err2 != nil || st <= 0 {
		return time.Time{}, 0, false
	}
	return time.Unix(0, ns), time.Duration(st), true
}

// headBytes is how much of the payload the parent keeps to read the ids
// from: the first 4 KiB it pumps, a fixed buffer.
const headBytes = 4 << 10

// head is what the parent reads from the payload's first 4 KiB.
type head struct {
	SessionID string
	Event     string
}

// headPaths are the top-level values scanHead reads. jsonscan passes a
// value only once it has ended, so a value the head's cut runs through is
// never read as a shorter one; and a session id's cap is the head itself,
// so one that ends inside it is never cut short either. An event name past
// its 64 bytes is cut, and names no event.
var headPaths = []jsonscan.Path{
	{Elems: []string{"session_id"}, Max: headBytes, Mode: jsonscan.CaptureHead, String: true},
	{Elems: []string{"hook_event_name"}, Max: 64, Mode: jsonscan.CaptureHead, String: true},
}

// scanHead reads the session id and event from a payload's first bytes. A
// key that repeats is read each time and the last wins.
// own read.
func scanHead(b []byte) head {
	var h head
	fn := func(path int, value []byte, _ bool) bool {
		switch path {
		case 0:
			h.SessionID = string(value)
		case 1:
			h.Event = string(value)
		}
		return true
	}
	if _, err := jsonscan.StreamOne(bytes.NewReader(b), headPaths, fn); err != nil {
		// The head is usually cut mid-value, so the scan ends in a syntax
		// error; every value it passed ended before the cut and stands.
		return h
	}
	return h
}

// eventOf is the event an invocation served: the payload's hook_event_name
// when the entry serves it, else the entry's event when it serves just one,
// else none.
func eventOf(events []string, payload string) string {
	for _, ev := range events {
		if ev == payload {
			return ev
		}
	}
	if len(events) == 1 {
		return events[0]
	}
	return ""
}
