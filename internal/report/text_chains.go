package report

import (
	"bytes"
	"fmt"
	"strings"
)

// writeChains renders the causal spine: which prompt produced which calls.
//
// THE LEGEND IS NOT DECORATION. A host on a link reads as though that call
// reached it, and it does not mean that: the hosts are the ones the call
// named, and nothing in the record observed whether it reached them. A bare
// `-> pypi.org` beside a call reads as "reached", so whenever the listing
// shows any host at all, one line says the hosts are only named.
func writeChains(b *bytes.Buffer, c Chains, expand bool) {
	extra := len(c.Unattributed) + len(c.Dropped)
	if len(c.Prompts) == 0 && extra == 0 {
		return
	}
	fmt.Fprintf(b, "  chains: %d\n", len(c.Prompts))

	// The COUNT is unconditional and the listing is not. This section is the
	// only one whose length grows with the session -- one line per tool call --
	// so on a long day it buries a report whose other sections are fixed size
	// and which a reader opens for coverage and findings. Hiding it entirely
	// behind a flag would be the opposite error: a view nobody knows exists is
	// the same as one that was never built.
	if !expand {
		if len(c.Prompts) > 0 {
			fmt.Fprintf(b, "    --chain lists the calls under each prompt\n")
		}
		writeChainTail(b, c, false)
		return
	}

	if listsAHost(c) {
		fmt.Fprintln(b, "    "+namedNotReached)
	}

	var lastPath string
	for _, ch := range c.Prompts {
		if ch.TranscriptPath != lastPath {
			fmt.Fprintf(b, "    %s\n", ch.TranscriptPath)
			lastPath = ch.TranscriptPath
		}
		fmt.Fprintf(b, "    prompt %s\n", ch.PromptID)
		for _, l := range ch.Links {
			writeLink(b, l)
		}
	}
	writeChainTail(b, c, true)
}

// listsAHost reports whether the expanded listing shows any host -- named or
// ssh -- beside any call. Dropped links carry nothing but an id.
func listsAHost(c Chains) bool {
	for _, ch := range c.Prompts {
		for _, l := range ch.Links {
			if len(l.DeclaredHosts) > 0 || len(l.SSHHosts) > 0 {
				return true
			}
		}
	}
	for _, l := range c.Unattributed {
		if len(l.DeclaredHosts) > 0 || len(l.SSHHosts) > 0 {
			return true
		}
	}
	return false
}

// writeChainTail renders the two groups that belong to no prompt.
//
// Both are announced whether or not the listing is expanded, because both are
// statements about COMPLETENESS -- how much of the session the chains above do
// not account for -- and a reader deciding whether to trust the view needs that
// without having to ask for more output.
func writeChainTail(b *bytes.Buffer, c Chains, expand bool) {
	if n := len(c.Unattributed); n > 0 {
		fmt.Fprintf(b, "    %d call%s could not be placed under a prompt "+
			"(prompt not recorded)\n", n, plural(n))
		if expand {
			for _, l := range c.Unattributed {
				writeLink(b, l)
			}
		}
	}
	if n := len(c.Dropped); n > 0 {
		// The declaration never landed, so the id is the whole of what is
		// known. Named anyway: this is a call the session made and cannot
		// describe, which is worth more to a reader than a tidy omission.
		fmt.Fprintf(b, "    %d call%s ran with no declaration recorded\n", n, plural(n))
		if expand {
			for _, l := range c.Dropped {
				fmt.Fprintf(b, "      %s  everything but the id is unknown\n", printable(l.ToolUseID))
			}
		}
	}
}

func writeLink(b *bytes.Buffer, l Link) {
	shape := l.VerbClass
	if l.Program != "" {
		shape = l.Program + ", " + l.VerbClass
	}
	outcome := l.Outcome
	// Two post records for one id. The headline is the higher-seq one and this
	// says the other existed, because a link that showed only the winner would
	// hide precisely the disagreement worth seeing.
	if l.ExecutionRecords > 1 {
		outcome = fmt.Sprintf("%s (%d records: %s)", l.Outcome,
			l.ExecutionRecords, strings.Join(l.Outcomes, ", "))
	}
	fmt.Fprintf(b, "      %d  %s (%s)  %s%s%s\n",
		l.Seq, printable(l.ToolName), shape, outcome, linkHosts(l.DeclaredHosts), linkSSH(l.SSHHosts))
}

// linkSSH renders the ssh hosts a call named, kept apart from the others
// because an ssh host is a different kind of destination.
func linkSSH(hosts []string) string {
	if len(hosts) == 0 {
		return ""
	}
	return "  ssh: " + strings.Join(hosts, ", ")
}

// linkHosts renders the hosts a call named, or nothing at all when it named
// none -- which is most calls, and a trailing "->" on every one of them would
// bury the ones that matter.
func linkHosts(hosts []string) string {
	if len(hosts) == 0 {
		return ""
	}
	return "  -> " + strings.Join(hosts, ", ")
}
