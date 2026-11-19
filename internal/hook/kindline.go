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

// The unindexed language's line: an edit the overlay writer stopped at the
// index's header, its reader registered indexed and the header naming no
// producer of the reader's language (keptGate.unindexed), is said to the
// person once per session and language. The language is the registry's,
// never the file's. The words before "files" are the edited file's kind
// where it passes printableKind, else the language's name.

// unindexedLine is the line for l, the language the index holds no records
// of, and what, the files whose edits carry no facts.
func unindexedLine(l, what string) string {
	return "rashomon: the index here holds no " + l + " records, so edits to " + what +
		" carry no unit, caller or conflict facts until rashomon graph index runs."
}

// The unread configuration's line: an edit the overlay writer stopped at
// its reader's Claim, the reader's configuration step having refused its
// file under the root (keptGate.unread), is said to the person once per
// session and file: the file as the step names it, and why, the class of
// the step's error alone (lang.Reason), never its path or text. The words
// before "files" are as the unindexed line's.

// unreadLine is the line for file, the configuration not read, why, its
// reason's words, and what, the files whose edits carry no facts.
func unreadLine(file, why, what string) string {
	return "rashomon: " + file + " was not read (" + why + "), so edits to " + what +
		" carry no unit, caller or conflict facts this session."
}

// reasonWords is the words of the reason r, one phrase for each reason of
// the closed set; a reason past it reads as a failed read.
func reasonWords(r lang.Reason) string {
	switch r {
	case lang.NotRegular:
		return "not a regular file"
	case lang.OverCap:
		return "over the size cap"
	case lang.NotJSON:
		return "not JSON"
	case lang.WrongShape:
		return "JSON of the wrong shape"
	}
	return "read failed"
}

// The last index's line: an edit the overlay writer kept no group for, its
// configured reader admitted without overlays (keptGate.lastIndex), is said
// to the person once per session and language, naming the commit of the
// index whose facts stand (the header's, keptGate.lastRev). The language is
// the registry's, never the file's. The words before "build" are as the
// unindexed line's.

// lastIndexLine is the line for what, the files whose edits build no
// overlay, and rev, the commit of the index whose facts stand for them.
func lastIndexLine(what, rev string) string {
	return "rashomon: edits to " + what + " build no new facts here; their unit, caller and conflict facts are those of the last index (" +
		rev + ")."
}

// takeKindLine is the line this post owes about the edited file's kind and
// the marker it took for it, or "" and "" when none is owed: the overlay
// writer's gate stopped neither at its last check alone (keptGate.unclaimed)
// nor at the index's header for the file's language (keptGate.unindexed)
// nor at a refused configuration (keptGate.unread), nor reached a reader
// admitted without overlays (keptGate.lastIndex), or the session has had
// the line. The gate is the writer's own result (Post.writeOverlay), so the
// line and the overlay cannot part. The states never meet: an unclaimed
// file has no reader to be unindexed or configured, the writer stops an
// unindexed one before its reader's configuration step, and a reader
// reaches the overlay check only once that step has passed.
func (p *Post) takeKindLine() (line, marker string) {
	g := p.gate
	switch {
	case g.unindexed != "":
		return p.takeUnindexedLine(g)
	case g.unread != nil:
		return p.takeUnreadLine(g)
	case g.lastIndex != "":
		return p.takeLastIndexLine(g)
	}
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

// takeUnindexedLine is takeKindLine for a gate the writer stopped for its
// language: the language's line and its marker, or "" and "" when the
// session has had it or the marker could not be made.
func (p *Post) takeUnindexedLine(g keptGate) (line, marker string) {
	marker = store.UnindexedNoticeName(g.unindexed)
	if !p.st.TakeNotice(p.sessionID, marker) {
		return "", ""
	}
	return unindexedLine(g.unindexed, kindWhat(g.rel, g.unindexed)), marker
}

// takeUnreadLine is takeKindLine for a gate the writer stopped at a refused
// configuration: the file's line and its marker, or "" and "" when the
// session has had it or the marker could not be made.
func (p *Post) takeUnreadLine(g keptGate) (line, marker string) {
	marker = store.UnreadNoticeName(g.unread.File)
	if !p.st.TakeNotice(p.sessionID, marker) {
		return "", ""
	}
	return unreadLine(g.unread.File, reasonWords(g.unread.Reason), kindWhat(g.rel, g.unreadLang)), marker
}

// takeLastIndexLine is takeKindLine for a gate whose reader builds no
// overlay: the language's line and its marker, or "" and "" when the
// session has had it or the marker could not be made.
func (p *Post) takeLastIndexLine(g keptGate) (line, marker string) {
	marker = store.LastIndexNoticeName(g.lastIndex)
	if !p.st.TakeNotice(p.sessionID, marker) {
		return "", ""
	}
	return lastIndexLine(kindWhat(g.rel, g.lastIndex), g.lastRev), marker
}

// kindWhat is the words for the files like rel: its kind where the kind
// passes printableKind, else the files of l, the language that claims rel.
func kindWhat(rel, l string) string {
	if key := lang.KindKey(rel); printableKind(key) {
		return key + " files"
	}
	return l + " files"
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
