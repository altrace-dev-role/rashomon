package report

import (
	"bytes"
	"fmt"
)

// writeDestinations renders what the session's calls named.
//
// The count of calls that ran differently from how they were declared comes
// first, and is always rendered, including the zero: "executed differently
// from declared: 0" tells a reader the comparison ran, where a line that
// appeared only when non-zero would leave them unable to tell a clean session
// from an unchecked one -- silence read as zero, the single failure this whole
// product exists to avoid.
//
// The host lines are an inventory and say so: these are the hosts the calls
// named, and the legend under them says nothing here observed whether any was
// reached. "none" is a fact about the record, not a verdict about the session.
func writeDestinations(b *bytes.Buffer, d Destinations) {
	fmt.Fprintf(b, "  executed differently from declared: %d\n", d.ExecutedNotAsDeclared)
	writeRewritten(b, d.Rewritten)
	if len(d.DeclaredHosts) == 0 {
		fmt.Fprintln(b, "  hosts named by tool calls: none")
	} else {
		fmt.Fprintf(b, "  hosts named by tool calls: %s\n", list(d.DeclaredHosts))
	}
	if len(d.SSHHosts) > 0 {
		fmt.Fprintf(b, "  ssh hosts named: %s\n", list(d.SSHHosts))
	}
	if len(d.DeclaredHosts) > 0 || len(d.SSHHosts) > 0 {
		fmt.Fprintln(b, "    "+namedNotReached)
	}
	writeNovelty(b, d.Novelty)
}

// namedNotReached says what a listed host is. A host under a heading or
// beside a call reads as one the session reached, and nothing in the record
// observed that, so wherever the text lists a host this line goes with it.
const namedNotReached = "hosts are those each call named; nothing here observed whether it reached them"

// writeNovelty renders which of this session's declared hosts are not yet in
// the project's baseline. It says "named", never "reached": nothing in the
// record observed whether a host was reached, and a novelty line that read
// as a destination would overstate what is known. It also never says
// "first": the baseline is seeded from whichever session happens to render
// first, not from the project's actual earliest one, so a host absent from
// the baseline is only that -- absent -- and not provably new to the
// project. A section that vanishes when it has nothing to say cannot be told
// apart from one that was never built, so every case prints a line.
func writeNovelty(b *bytes.Buffer, n Novelty) {
	switch {
	case !n.Available:
		fmt.Fprintf(b, "  not yet in this project's baseline: %s (%s)\n", unknown, n.Reason)
	case n.Established:
		// The count says which hosts it counted: loopback and the client's
		// own plane are excluded from novelty on purpose, so a bare count
		// reads as disagreeing with the list above.
		fmt.Fprintf(b, "  not yet in this project's baseline: baseline established "+
			"(%d host%s, loopback and client plane excluded)\n", n.KnownHosts, plural(n.KnownHosts))
	case len(n.Hosts) == 0:
		fmt.Fprintf(b, "  not yet in this project's baseline: none (%d known)\n", n.KnownHosts)
	default:
		fmt.Fprintf(b, "  not yet in this project's baseline: %s\n", list(n.Hosts))
		fmt.Fprintf(b, "    (added to this project's baseline by this session; %d hosts known)\n", n.KnownHosts)
	}
}
