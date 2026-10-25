package report

import (
	"bytes"
	"fmt"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// writeStoreWide renders the two store-wide lines that sit above every
// session: the calls that named no session, and the sessions the harness
// detector could not place.
func writeStoreWide(b *bytes.Buffer, rep *Report) {
	writeSessionless(b, rep.CallsWithoutSessionID)
	writeHarnessUnknown(b, rep.HarnessUnknownSessions)
}

// writeHarnessUnknownIfSet is the line for a report with no session to print
// it above: "no sessions recorded" is the whole answer unless the detector has
// something to say, which is any non-zero count and any unreadable counter.
func writeHarnessUnknownIfSet(b *bytes.Buffer, n *int) {
	if n == nil || *n > 0 {
		writeHarnessUnknown(b, n)
	}
}

// writeSessionless renders Report.CallsWithoutSessionID: once, above every
// session, because it belongs to the store and not to any one of them.
//
// Three states, and none may read as another. The degraded line names where
// the calls are recorded, because in a report scoped to one session that run
// is not on screen. Its healthy twin states the zero rather than vanishing,
// so "none arrived" does not read the same as "nothing counted them". And an
// unreadable run is unknown, in a word, never none.
//
// Printed only when there is a session to print it above: with nothing
// recorded at all, "no sessions recorded" is already the whole answer.
func writeSessionless(b *bytes.Buffer, n *int) {
	switch {
	case n == nil:
		fmt.Fprintf(b, "calls without a session id: %s (session %s could not be read)\n",
			unknown, store.UnattributedSession)
	case *n == 0:
		fmt.Fprintf(b, "calls without a session id: %s\n", none)
	default:
		fmt.Fprintf(b, "%d call%s arrived without a session id (recorded under session %s)\n",
			*n, plural(*n), store.UnattributedSession)
	}
}

// writeHarnessUnknown renders Report.HarnessUnknownSessions, once, above every
// session, in the three states writeSessionless keeps apart. A non-zero count
// says what to do with it, because BRD-4's ruling (2026-10-24) is that an
// unknown-harness session is recorded and counted so that the count is
// chased: it means the detector missed a real Claude Code session. An
// unreadable counter is unknown in a word, never none.
func writeHarnessUnknown(b *bytes.Buffer, n *int) {
	switch {
	case n == nil:
		fmt.Fprintf(b, "harness unknown: %s (%s could not be read)\n", unknown, store.FileHarnessCounts)
	case *n == 0:
		fmt.Fprintf(b, "harness unknown: %s\n", none)
	default:
		fmt.Fprintf(b, "harness unknown: %d session%s in the last 30 days (each one is a detector gap to chase)\n",
			*n, plural(*n))
	}
}
