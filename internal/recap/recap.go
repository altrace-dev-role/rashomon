// Package recap decides whether one turn's digest is worth a line on Stop,
// and renders that line.
//
// Exception-only, by the spec's own argument: a line that is the same on
// almost every turn is trained out within a week, and the whole point of
// this package is that when it DOES speak, it is worth reading. Non-goals
// carries the other half of that bargain -- "silence is not a claim" -- which
// is why this package never decides anything status cannot also see; see
// Claim and Status, which give status the fact silence itself cannot carry.
//
// It renders records, never inferences. Every sentence Line produces is
// either a bare count already sitting on the digest, or one of the store's
// own fixed vocabulary of reason codes (store.Reasons, report.ReasonGap,
// digest.ReasonNoStore/ReasonRecordsSkipped/ReasonNoSessionID) -- never a
// word this package invented and never a byte handed to it by a tool call or
// a transcript. The one caller-supplied value it prints, the session id, is
// sanitised before it is, on the same rule internal/report/account.go's
// sanitizeMessage sets for a different field: control and non-graphic runes
// are attacker-influenceable and the report renders values like this
// unquoted, which is a standing defect this package does not repeat (spec,
// "Sanitise every tool-derived value before rendering").
package recap

import (
	"fmt"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/digest"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// mark opens every line this package renders. Bytes, not a claim: a reader
// who has never seen it before still gets a coherent sentence after it.
const mark = "※" // ※ REFERENCE MARK

// prefix is measured in runes, not assumed, because the continuation line
// aligns its arrow under the first character after it -- see Line.
var prefix = mark + " rashomon: "

// Line decides whether d is worth a line and renders it if so.
//
// The five triggers are the spec's "When it speaks" list, minus one: a
// destination new for this project needs the proxy database and the
// per-project baseline (internal/report/destinations.go's buildNovelty),
// and the digest this package reads deliberately opens neither -- Part 3's
// whole reason for existing separately from report.Build is that report
// reads the proxy store and writes baseline/ on every call, which this
// path cannot afford to do once per turn without reintroducing exactly the
// side effects H-86/H-87 exist to rule out. There is therefore no honest way
// for this package to know a destination is new, and it does not pretend to;
// see the PR description for the fuller account of this gap.
//
// Coverage-unverified and digest-unknown are rendered as ONE sentence, not
// two: Unknown is defined as Recorded == 0 AND Coverage.State != verified
// (digest.Digest's own doc), so whenever it is true the coverage reasons ARE
// the explanation for the unknown count, and printing both would say the
// same thing twice in the one place this package is required to stay short.
//
// fromPlugin picks the command the line points at. /rashomon:report is a
// plugin skill and exists only where the plugin does; a settings install has
// the rashomon binary it was installed from instead, while a plugin's binary
// sits under the plugin's own directory rather than on PATH. Pointing either
// user at the other's command sends them to one that is not there.
func Line(d *digest.Digest, sessionID string, fromPlugin bool) (string, bool) {
	var sentences []string

	switch {
	case d.Unknown:
		sentences = append(sentences, fmt.Sprintf("digest unknown (%s)", reasonList(d.Coverage.Reasons)))
	case d.Coverage.State != store.StateVerified:
		sentences = append(sentences, fmt.Sprintf("coverage unverified (%s)", reasonList(d.Coverage.Reasons)))
	}
	if d.Truncated {
		sentences = append(sentences, "digest truncated (some fields were cut to stay under the size ceiling)")
	}
	if d.SilentFailures.Fires {
		sentences = append(sentences,
			fmt.Sprintf("%d recorded failure%s", d.SilentFailures.Failed, plural(d.SilentFailures.Failed)))
	}
	if n := len(d.Declarations.WithoutExecution); n > 0 {
		sentences = append(sentences, fmt.Sprintf("%d declaration%s without recorded execution", n, plural(n)))
	}

	if len(sentences) == 0 {
		return "", false
	}

	var b strings.Builder
	b.WriteString(prefix)
	b.WriteString(strings.Join(sentences, ". "))
	b.WriteString(".\n")
	b.WriteString(strings.Repeat(" ", len([]rune(prefix))))
	if fromPlugin {
		b.WriteString("→ /rashomon:report") // → RIGHTWARDS ARROW
	} else {
		b.WriteString("→ rashomon report")
	}
	// A turn that named no session (digest.Sessionless) has no session to
	// point at, so the pointer is the whole report. Not the sanitiser's
	// "unknown", which names a session nobody has, and not any real id, which
	// would be the guess the no_session_id reason exists to refuse.
	if sessionID != "" {
		b.WriteString(" --session ")
		b.WriteString(sanitizeSessionID(sessionID))
	}
	return b.String(), true
}

// reasonList joins a digest's own fixed reason vocabulary for display. There
// is deliberately no fallback to "no reason recorded": every path that sets
// d.Unknown or a non-verified Coverage.State also adds at least one reason
// (digest's buildTurnCoverage and Empty both do), so an empty list here would
// be this package's own bug, not a fact about the store, and hiding it behind
// a friendly default would make that bug quieter rather than louder.
func reasonList(reasons []string) string {
	return strings.Join(reasons, ", ")
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// sanitizeSessionID bounds and cleans the one value on this line that did not
// originate in this package's own vocabulary.
//
// session_id arrives on the Stop payload's stdin JSON, which Claude Code
// itself generates rather than a tool call or a transcript -- a materially
// different threat model from shape.program's -- but the boundary guarantee
// encoding/json gives the digest is exactly the one the spec says is not a
// defence for THIS package: whatever reaches d.SessionID gets restored to
// plain bytes the moment it is handed to fmt, so this still caps length and
// keeps only an allowlist of id-shaped runes, the same allowlist
// install.go's shellQuote uses to decide a path needs no quoting at all.
// Nothing here needs the quoting half of that pattern: an id built only from
// this allowlist can never contain a space, a newline or an escape byte to be
// quoted against.
func sanitizeSessionID(s string) string {
	const maxLen = 128
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '-' || r == '_' || r == '.' || r == ':':
		default:
			continue
		}
		b.WriteRune(r)
		if b.Len() >= maxLen {
			break
		}
	}
	if b.Len() == 0 {
		return "unknown"
	}
	return b.String()
}

// PreviousTurn marks a line as being about the turn before the one just
// started. The catch-up path prints it when the next prompt is sent, one
// turn late, and a reader who has moved on needs to know which turn it means.
func PreviousTurn(line string) string {
	previous := mark + " rashomon, previous turn: "
	line = strings.Replace(line, prefix, previous, 1)
	return strings.Replace(line,
		"\n"+strings.Repeat(" ", len([]rune(prefix))),
		"\n"+strings.Repeat(" ", len([]rune(previous))), 1)
}
