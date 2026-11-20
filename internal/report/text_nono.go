package report

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/nono"
)

// writeNono renders what the sandbox saw.
//
// ALWAYS PRINTS A LINE, including when no trail was configured. A section that
// vanishes when it has nothing to say cannot be told apart from one that was
// never built -- the rule this package states for family coverage and has now
// broken twice.
func writeNono(b *bytes.Buffer, n Nono) {
	if !n.Configured {
		// SILENT, not "not observed". A sandbox nobody asked for is not a
		// degraded observation -- it is a feature that was not in use, and
		// H-28 is right that printing a degradation marker on a healthy run
		// teaches readers to discount the word.
		return
	}
	if !n.Observed {
		// The skipped count is said here too, but ONLY when it is complete.
		// nono_audit_no_records means the reader reached end of file, so
		// Skipped counts the whole trail. nono_audit_unreadable means the
		// scan stopped at the line that broke it (an oversized line, a read
		// error): Skipped there counts only the torn lines BEFORE that
		// point, an undercount of however much of the trail follows. Two
		// torn lines followed by one line over the scanner's cap used to
		// print "2 trail records could not be read" regardless of how much
		// trail came after the point the read gave up.
		skipped := ""
		if n.Reason == nono.NotObservedNoRecords && n.Skipped > 0 {
			skipped = fmt.Sprintf("; %d trail record%s could not be read", n.Skipped, plural(n.Skipped))
		}
		fmt.Fprintf(b, "  sandbox (nono): not observed (%s%s)\n", orUnknown(n.Reason), skipped)
		return
	}
	if n.NetworkEventsInTrail == 0 {
		// Observed is true -- the trail held a session_started and a
		// session_ended record -- but the sandbox never wrote a single
		// network decision. That is a different fact from a session the
		// sandbox filtered and saw nothing on: it happens whenever the
		// active profile does not filter the network at all. Rendering it
		// as "0 allowed, 0 denied" reads as a quiet, filtered session when
		// no filtering ran -- silence read as zero, the failure this
		// package exists to prevent. Follow-up from #53, applied here.
		//
		// NOT A RETURN. This used to return right after the line below,
		// which hid the skipped/unparseable, sessions and inherited lines
		// below on exactly the trails where they are most informative: a
		// schema-drift record (an event type like log_denied) or a torn
		// line next to a session_started record both land in Skipped and
		// never touch NetworkEventsInTrail, so a reader got a confident
		// "the profile may not filter the network" on a trail this reader
		// actually failed to read. When Skipped > 0, unreadable records are
		// named as a possible cause alongside the profile, not displaced by
		// it.
		cause := "the active profile may not filter the network"
		if n.Skipped > 0 {
			cause = "the active profile may not filter the network, or the events could not be read"
		}
		fmt.Fprintf(b, "  sandbox (nono): the trail holds no network events (%s; this is "+
			"not the same as a filtered session that saw nothing)\n", cause)
	} else {
		// An open window is said ON THIS LINE: the two counts are only as
		// bounded as the window, and with no end record they run to the end
		// of the trail, where a later session's traffic on a shared trail
		// counts as this one's.
		open := ""
		if n.WindowOpen {
			open = " (no end record, so events up to the end of the trail are counted)"
		}
		fmt.Fprintf(b, "  sandbox (nono): %d allowed, %d denied in this session's window%s\n",
			len(n.Allowed), len(n.Denied), open)
		if len(n.ClientPlaneAllowed)+len(n.ClientPlaneDenied) > 0 {
			// Named, not dropped: the sandbox did allow or refuse these, and a
			// reader comparing against nono's own log should find every host and
			// what the sandbox did with it. Not counted above, because these are
			// hosts the client contacts on its own and no tool call of the session
			// named them; one that a call named is counted above, as the agent's.
			var said []string
			if len(n.ClientPlaneAllowed) > 0 {
				said = append(said, "allowed "+list(n.ClientPlaneAllowed))
			}
			if len(n.ClientPlaneDenied) > 0 {
				said = append(said, "refused "+list(n.ClientPlaneDenied))
			}
			fmt.Fprintf(b, "    the client's own traffic, not counted above: %s\n", strings.Join(said, "; "))
		}
	}
	// SAID SEPARATELY, because it is a LIFETIME count beside two windowed ones.
	// nono's session_started record carries an ISO string and no millisecond
	// instant, so there is nothing to window it on -- and joining all three in
	// one sentence rendered "0 allowed, 0 denied, across 1 session", which
	// reads as "a sandbox session ran and did nothing".
	fmt.Fprintf(b, "    the trail holds %d sandbox session%s in total (not "+
		"window-filtered: nono's session record carries no millisecond instant)\n",
		n.Sessions, plural(n.Sessions))
	if len(n.Denied) > 0 {
		// Named, because a denial is the sandbox doing its job and it is
		// evidence about the AGENT: it tried to go somewhere it could not.
		fmt.Fprintf(b, "    refused by the sandbox: %s\n", list(n.Denied))
	}
	if n.Skipped > 0 || n.UnparseableTargets > 0 {
		fmt.Fprintf(b, "    %d trail record%s could not be read, %d target%s could not be "+
			"parsed\n", n.Skipped, plural(n.Skipped),
			n.UnparseableTargets, plural(n.UnparseableTargets))
	}
	if n.UnknownModes > 0 {
		fmt.Fprintf(b, "    %d event%s carried a transport this reader does not know\n",
			n.UnknownModes, plural(n.UnknownModes))
	}
	if n.UnknownDecisions > 0 {
		verb := "were"
		if n.UnknownDecisions == 1 {
			verb = "was"
		}
		fmt.Fprintf(b, "    %d sandbox event%s carried a decision this reader does not "+
			"know, and %s counted in neither column\n",
			n.UnknownDecisions, plural(n.UnknownDecisions), verb)
	}
	if n.Inherited > 0 {
		fmt.Fprintf(b, "    %d sandbox event%s outside this session's window, excluded\n",
			n.Inherited, plural(n.Inherited))
	}
}
