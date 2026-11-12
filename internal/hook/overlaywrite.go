package hook

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang/gopg"
	"github.com/altrace-dev-role/rashomon/internal/jsonscan"
	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The overlay writers (W-M2.5c, 5c-2) append one overlay group to the session
// log for a Go file a Write, Edit or MultiEdit names: at PreToolUse a group
// of the content the edit would leave (kind declared, at the declaration's
// seq), at PostToolUse a group of the bytes on disk after the tool (kind
// landed, at the execution's seq). A reader sees them in the next request,
// after the call; nothing here is delivered to the model, printed, or allowed
// to block a tool call. Every failure ends in "no group written", which says
// less and is never wrong.
//
// The checks run in this order and stop at the first that fails, so the work
// that costs something waits for the answer that makes it worth doing:
//
//  1. the tool is Write, Edit or MultiEdit on a path the Go resolver takes;
//  2. the seq the group would carry is positive (the declaration or the
//     execution landed in the ordered stream);
//  3. an index exists above the working directory, and the file is under it;
//  4. the session-content marker is present. Absent, the writer stops and
//     writes nothing: no group, no tally, no file in the run directory.
//
// Only after (4) are the file's content read, the index header read and the
// resolver called, each through a package-level func var so a test can count
// them.
//
// The edit's content. A Write's is its content field. An Edit's or a
// MultiEdit's is the file's bytes read once from disk with each new_string
// spliced in for its old_string, as the tool would. The splice is exact or
// nothing: an old_string that is absent, or that occurs more than once
// without replace_all, writes no group, because a guessed content would be
// wrong, not less. A MultiEdit applies its edits in order, each to the
// running result of the ones before it, not to the original bytes;
// replace_all replaces every occurrence; the bytes are those on disk, with no
// newline, byte-order-mark or encoding normalisation.
//
// Every content read, Pre and Post, is capped by MaxOverlayContentBytes: a
// file on disk, a Write's content, or a spliced result over it writes no
// group.

// MaxOverlayContentBytes caps the content an overlay writer reads, spliced or
// written (PROVISIONAL: the largest Go file of this repository must be under
// it, which the K7 measure names).
const MaxOverlayContentBytes = 1 << 20

// maxOverlayInputBytes bounds the tool_input a writer decodes. A decoded
// string is never longer than its JSON spelling, and a byte spells as six at
// the most (\u00xx), so content within the cap cannot arrive in more than
// this, with room for the keys around it.
const maxOverlayInputBytes = 6*MaxOverlayContentBytes + 64<<10

var (
	// overlayReadFile reads a file of at most MaxOverlayContentBytes.
	overlayReadFile = readOverlayFile
	// overlayReadHeader reads the index file's header line.
	overlayReadHeader = graph.ReadIndexHeader
	// overlayResolve is the Go resolver's Overlay.
	overlayResolve = func(rel string, content []byte, base graph.Rev, seq int64) ([]graph.Record, error) {
		return gopg.Resolver{}.Overlay(rel, content, base, seq)
	}
)

// overlayContentError is a constant error, which costs no init allocation
// (RES-2) as a package-level errors.New does.
type overlayContentError string

func (e overlayContentError) Error() string { return string(e) }

const errOverlayContent overlayContentError = "hook: overlay content unreadable or over the cap"

// readOverlayFile reads a regular file of at most MaxOverlayContentBytes.
func readOverlayFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck // read only
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() || fi.Size() > MaxOverlayContentBytes {
		return nil, errOverlayContent
	}
	b, err := io.ReadAll(io.LimitReader(f, MaxOverlayContentBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > MaxOverlayContentBytes {
		return nil, errOverlayContent
	}
	return b, nil
}

// overlayEdit is one old_string to new_string pair of an Edit or MultiEdit.
type overlayEdit struct {
	Old, New   string
	ReplaceAll bool
}

// editFrom reads one pair out of an object span: old_string and new_string
// must be strings (a missing or null one reads as empty, as decoding did),
// replace_all a boolean when present. Spans only, no decode of the object
// (RES-5).
func editFrom(in []byte, obj jsonscan.Span) (overlayEdit, bool) {
	var e overlayEdit
	if obj.Kind != jsonscan.Object {
		return e, false
	}
	for _, f := range []struct {
		key string
		dst *string
	}{{"old_string", &e.Old}, {"new_string", &e.New}} {
		sp, err := jsonscan.Lookup(in, obj, f.key)
		if err != nil {
			return e, false
		}
		switch sp.Kind {
		case jsonscan.Absent, jsonscan.Null:
		case jsonscan.String:
			v, err := jsonscan.DecodeString(in, sp)
			if err != nil {
				return e, false
			}
			*f.dst = v
		default:
			return e, false
		}
	}
	sp, err := jsonscan.Lookup(in, obj, "replace_all")
	if err != nil {
		return e, false
	}
	switch sp.Kind {
	case jsonscan.True:
		e.ReplaceAll = true
	case jsonscan.Absent, jsonscan.Null, jsonscan.False:
	default:
		return e, false
	}
	return e, true
}

// editsFrom reads the edits of a tool_input: the one pair of an Edit, the
// edits array of a MultiEdit.
func editsFrom(tool string, in []byte, root jsonscan.Span) ([]overlayEdit, bool) {
	if tool == "Edit" {
		e, ok := editFrom(in, root)
		return []overlayEdit{e}, ok
	}
	arr, err := jsonscan.Lookup(in, root, "edits")
	if err != nil || arr.Kind != jsonscan.Array {
		return nil, false
	}
	var edits []overlayEdit
	ok := true
	if jsonscan.Elements(in, arr, func(el jsonscan.Span) bool {
		e, good := editFrom(in, el)
		edits, ok = append(edits, e), good
		return good
	}) != nil {
		return nil, false
	}
	return edits, ok
}

// spliceEdits applies edits to content in order, each to the running result,
// and reports false for any edit that is not exact (an empty or absent
// old_string, or one occurring more than once without replace_all) and for a
// result over MaxOverlayContentBytes.
func spliceEdits(content []byte, edits []overlayEdit) ([]byte, bool) {
	if len(edits) == 0 {
		return nil, false
	}
	cur := content
	for _, e := range edits {
		if e.Old == "" {
			return nil, false
		}
		old := []byte(e.Old)
		n := bytes.Count(cur, old)
		if n == 0 || n > 1 && !e.ReplaceAll {
			return nil, false
		}
		if !e.ReplaceAll {
			n = 1
		}
		if len(cur)+n*(len(e.New)-len(e.Old)) > MaxOverlayContentBytes {
			return nil, false
		}
		cur = bytes.Replace(cur, old, []byte(e.New), n)
	}
	return cur, true
}

// overlayCall is one writer's inputs.
type overlayCall struct {
	st        *store.Store
	session   string
	toolUseID string
	cwd       string
	tool      string
	input     json.RawMessage
	// kind is graph.OverlayDeclared (Pre) or graph.OverlayLanded (Post), and
	// seq the seq of the record the group is placed at.
	kind string
	seq  int64
}

// overlayPath is the absolute path of the file a Write, Edit or MultiEdit
// names when the Go resolver takes it, else "".
func overlayPath(tool string, input json.RawMessage, cwd string) string {
	switch tool {
	case "Write", "Edit", "MultiEdit":
	default:
		return ""
	}
	p := shape.DeclaredPath(tool, input)
	if p == nil || !(gopg.Resolver{}).Claims("", *p) {
		return ""
	}
	path := *p
	if !filepath.IsAbs(path) {
		if cwd == "" {
			return ""
		}
		path = filepath.Join(cwd, path)
	}
	return filepath.Clean(path)
}

// underRoot is path relative to root, slash separated, false for a path
// outside it.
func underRoot(root, path string) (string, bool) {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

// content is the file's content the group describes, false for none.
func (c overlayCall) content(path string) ([]byte, bool) {
	if c.kind == graph.OverlayLanded {
		b, err := overlayReadFile(path)
		return b, err == nil
	}
	if len(c.input) > maxOverlayInputBytes {
		return nil, false
	}
	in := []byte(c.input)
	if !json.Valid(in) {
		return nil, false
	}
	root := jsonscan.ValueSpan(in)
	if c.tool == "Write" {
		sp, err := jsonscan.Lookup(in, root, "content")
		if err != nil || sp.Kind != jsonscan.String {
			return nil, false
		}
		v, err := jsonscan.DecodeString(in, sp)
		if err != nil || len(v) > MaxOverlayContentBytes {
			return nil, false
		}
		return []byte(v), true
	}
	edits, ok := editsFrom(c.tool, in, root)
	if !ok {
		return nil, false
	}
	disk, err := overlayReadFile(path)
	if err != nil {
		return nil, false
	}
	return spliceEdits(disk, edits)
}

// write appends the group, if every check holds. It never reports a failure:
// the caller has nothing to do with one, and a panic is recovered here so the
// hook's own work is untouched.
func (c overlayCall) write() {
	defer func() { _ = recover() }()
	path := overlayPath(c.tool, c.input, c.cwd)
	if path == "" || c.seq <= 0 || c.session == store.UnattributedSession {
		return
	}
	root := graph.FindIndex(c.cwd)
	if root == "" {
		return
	}
	rel, ok := underRoot(root, path)
	if !ok {
		return
	}
	if !store.SessionContentMarker(c.st.Root()) {
		return
	}
	content, ok := c.content(path)
	if !ok {
		return
	}
	hdr, err := overlayReadHeader(filepath.Join(root, filepath.FromSlash(graph.GraphFile)))
	if err != nil || hdr.Rev.Commit == "" {
		return
	}
	recs, err := overlayResolve(rel, content, graph.RevCommit(hdr.Rev.Commit), c.seq)
	if err != nil || len(recs) == 0 {
		return
	}
	rid, _ := baseline.RepoID(root)
	if rid == "" {
		return
	}
	group := graph.BuildOverlayGroup(graph.OverlayGroup{Session: c.session, ToolUseID: c.toolUseID, Kind: c.kind,
		File: graph.FileID(rel), Digest: graph.ContentDigest(content), Commit: hdr.Rev.Commit, Seq: c.seq,
		Carried: (gopg.Resolver{}).OverlayCarried(), Recs: recs})
	log := graph.OpenSessionLog(c.st.Root(), rid, graph.RevCommit(hdr.Rev.Commit), true)
	_ = log.AppendOverlayGroup(group, func(n int64) error { return c.st.ReserveOverlay(c.session, n) })
}

// writeOverlay is the PreToolUse writer: the declared group, at the seq the
// declaration landed at (0 when it did not, which writes nothing).
func (h *Handler) writeOverlay(p *Payload) {
	overlayCall{st: h.st, session: h.sessionID, toolUseID: h.toolUseID, cwd: p.CWD, tool: p.ToolName, input: p.ToolInput,
		kind: graph.OverlayDeclared, seq: h.declaredSeq}.write()
}

// writeOverlay is the PostToolUse writer: the landed group, a full one, at
// the seq the execution landed at. A failure event writes none (a
// PostToolUseFailure writer is a later card's).
func (p *Post) writeOverlay() {
	if p.event == FailureEvent {
		return
	}
	overlayCall{st: p.st, session: p.sessionID, toolUseID: p.toolUseID, cwd: p.cwd, tool: p.toolName, input: p.toolInput,
		kind: graph.OverlayLanded, seq: p.landedSeq}.write()
}
