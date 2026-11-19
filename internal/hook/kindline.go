package hook

import (
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/fault"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The unclaimed kind's line (card LANG-REG1): an edit the overlay writer
// would have kept, of a file no reader here claims and whose kind is no row
// of the kinds table, is said to the person once per session and kind, as a
// system message the post prints and the model never reads. The words
// between "for" and "files" come from the edited file's name, so they print
// only when the key passes printableKind; a key that does not, and every
// kind past the store's limit of 16, takes the one line naming no kind,
// once (store.FileNoticeOtherKinds). That line is not one of the 16: its
// marker is outside the count, so an unprintable key early in a session
// still leaves sixteen named lines, and once it has printed, a kind past
// the limit prints nothing, named or not. The readers named are this
// build's registry, in its order, never a constant.

// kindKeyMax is the most bytes of a key the line prints.
const kindKeyMax = 16

// printableKind reports whether key may print inside the line: 1 to 16
// bytes of ASCII letters, digits, dot, underscore, hyphen and plus.
func printableKind(key string) bool {
	if key == "" || len(key) > kindKeyMax {
		return false
	}
	for i := 0; i < len(key); i++ {
		switch c := key[i]; {
		case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9', c == '.', c == '_', c == '-', c == '+':
		default:
			return false
		}
	}
	return true
}

// noReaderLine is the line for what, the words between "for" and the
// comma, naming the readers of reg in its order.
func noReaderLine(what string, reg *lang.Registry) string {
	return "rashomon: no reader here for " + what + ", so edits to them carry no unit, caller or conflict facts this session (readers here: " +
		strings.Join(reg.Langs(), ", ") + ")."
}

// otherKindsWhat is the fallback line's words for the kinds it does not name.
const otherKindsWhat = "some other kinds of file"

// takeKindLine is the unclaimed kind's line this post owes and the marker it
// took for it, or "" and "" when none is owed: the overlay writer's gate did
// not stop at its last check alone (keptGate.unclaimed), or the session has
// had the line. The gate is the writer's own result (Post.writeOverlay), so
// the line and the overlay cannot part.
func (p *Post) takeKindLine() (line, marker string) {
	g := p.gate
	if !g.unclaimed || g.reg == nil {
		return "", ""
	}
	key := lang.KindKey(g.rel)
	if printableKind(key) {
		marker = p.st.TakeKindNotice(p.sessionID, key)
	} else if p.st.TakeNotice(p.sessionID, store.FileNoticeOtherKinds) {
		marker = store.FileNoticeOtherKinds
	}
	switch marker {
	case "":
		return "", ""
	case store.FileNoticeOtherKinds:
		return noReaderLine(otherKindsWhat, g.reg), marker
	}
	return noReaderLine(key+" files", g.reg), marker
}

// returnKindLine gives back the marker takeKindLine took, for a line that
// did not print, so the kind's next edit says it.
func (p *Post) returnKindLine(marker string) {
	if marker != "" {
		p.st.ReturnNotice(p.sessionID, marker)
	}
}

// withKindLine is msg, a system message, with line on a line of its own
// after it: after the mirror line Route set, the person reads the line next.
func withKindLine(msg, line string) string {
	switch {
	case line == "":
		return msg
	case msg == "":
		return line
	}
	return msg + "\n" + line
}

// kindLineAlone is the bytes of a post whose call raised nothing else: the
// unclaimed kind's line alone, as one system message, or nil. The run is
// made durable before the line prints (REC-34), and the marker is given back
// when the flush or the marshal fails, so the next edit of the kind says it.
func (p *Post) kindLineAlone(c steer.Client) []byte {
	line, marker := p.takeKindLine()
	if line == "" {
		return nil
	}
	if fault.Fail(fault.PointSteerFlush) != nil || p.st.Flush() != nil {
		p.returnKindLine(marker)
		return nil
	}
	b, err := steer.Output{SystemMessage: line}.MarshalFor(p.event, c)
	if err != nil || b == nil {
		p.returnKindLine(marker)
		return nil
	}
	return b
}
