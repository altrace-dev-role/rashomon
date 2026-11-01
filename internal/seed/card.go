package seed

import (
	"fmt"
	"strings"
)

// The card's fixed lines. Card prints nothing else but numbers and rowText,
// and Vocabulary is built from these, so the two cannot drift apart.
const (
	cardNone    = "rashomon audit: no Claude Code sessions in the last %d days on this machine.\n"
	cardHead    = "rashomon audit: the last %d days of Claude Code on this machine\n"
	cardTotals  = "  %s in %s, %s\n"
	cardStale   = "  %s older than that %s not read.\n"
	cardUnread  = "  %s could not be read and %s not counted.\n"
	cardKinds   = "  You stopped or overruled your agent %s:\n"
	cardStopped = "  %6d  stopped it mid-turn\n"
	cardSaidNo  = "  %6d  said no to a tool call\n"
	cardPushed  = "  %6d  pushed back in a reply\n"
	cardBlocked = "  Your own rules blocked it %s.\n"
	cardFailed  = "  It hit a tool error and tried the same tool again %s.\n"
	cardRows    = "  What keeps coming back (times, sessions):\n"
	cardRow     = "  %6d%6d  %s\n"
	cardNoRows  = "  Nothing came back in a second session yet.\n"
	cardFoot    = "  Counted here: nothing was sent or stored, and it cost 0 tokens.\n"
	cardLink    = "  github.com/altrace-dev-role/rashomon\n"
	cardNouns   = "session sessions project projects tool call calls time times transcript transcripts line lines or file files was were is are"
	cardMaxRows = 7
	cardRecurAt = 2
)

// Card renders s over a window of days: what was read, the corrections by
// kind, and at most cardMaxRows signatures seen in cardRecurAt or more
// sessions. Counts and our words only: no path, name, command or text of the
// person's reaches it (PF-AU2).
func Card(s Summary, days int) string {
	var b strings.Builder
	if s.Sessions == 0 {
		fmt.Fprintf(&b, cardNone, days)
		notRead(&b, s)
		return b.String()
	}
	fmt.Fprintf(&b, cardHead, days)
	fmt.Fprintf(&b, cardTotals, count(s.Sessions, "session", "sessions"), count(s.Projects, "project", "projects"),
		count(s.Calls, "tool call", "tool calls"))
	notRead(&b, s)
	b.WriteString("\n")
	fmt.Fprintf(&b, cardKinds, count(s.Interrupted+s.Rejected+s.PushedBack, "time", "times"))
	fmt.Fprintf(&b, cardStopped, s.Interrupted)
	fmt.Fprintf(&b, cardSaidNo, s.Rejected)
	fmt.Fprintf(&b, cardPushed, s.PushedBack)
	fmt.Fprintf(&b, cardBlocked, count(s.Blocked, "time", "times"))
	fmt.Fprintf(&b, cardFailed, count(s.FailedAgain, "time", "times"))
	b.WriteString("\n")
	n := 0
	for _, r := range s.Rows {
		if r.Sessions < cardRecurAt || n == cardMaxRows {
			break
		}
		if n == 0 {
			b.WriteString(cardRows)
		}
		fmt.Fprintf(&b, cardRow, r.Times, r.Sessions, r.Text)
		n++
	}
	if n == 0 {
		b.WriteString(cardNoRows)
	}
	b.WriteString("\n" + cardFoot + cardLink)
	return b.String()
}

// notRead says what the counts leave out, when anything was.
func notRead(b *strings.Builder, s Summary) {
	if s.Stale > 0 {
		fmt.Fprintf(b, cardStale, count(s.Stale, "transcript", "transcripts"), plural(s.Stale, "was", "were"))
	}
	if s.Unread > 0 {
		fmt.Fprintf(b, cardUnread, count(s.Unread, "line or file", "lines or files"), plural(s.Unread, "is", "are"))
	}
}

func count(n int, one, many string) string { return fmt.Sprintf("%d %s", n, plural(n, one, many)) }

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// Vocabulary is every word Card can print but a number: the words of its
// fixed lines and of every signature's text, for the test that holds the
// card to them.
func Vocabulary() []string {
	texts := []string{cardNone, cardHead, cardTotals, cardStale, cardUnread, cardKinds, cardStopped, cardSaidNo,
		cardPushed, cardBlocked, cardFailed, cardRows, cardRow, cardNoRows, cardFoot, cardLink, cardNouns}
	for k := kind(0); k < numKinds; k++ {
		for c := class(0); c < numClasses; c++ {
			texts = append(texts, rowText(k, c))
		}
	}
	seen := map[string]bool{}
	var out []string
	for _, t := range texts {
		for _, f := range strings.Fields(t) {
			w := strings.Trim(f, ",.:;()")
			if w == "" || strings.Contains(w, "%") || seen[w] {
				continue
			}
			seen[w] = true
			out = append(out, w)
		}
	}
	return out
}
