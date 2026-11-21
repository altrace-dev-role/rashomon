package hook

import (
	"bytes"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/index"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang"
	"github.com/altrace-dev-role/rashomon/internal/jsonscan"
	"github.com/altrace-dev-role/rashomon/internal/regfile"
	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The overlay writers (W-M2.5c, 5c-2) append one overlay group to the session
// log for a file a Write, Edit or MultiEdit names that a reader of the
// language registry claims (index.Wire, LANG-REG1): at PreToolUse a group
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
//  1. the record scope is v5, the tool is Write, Edit or MultiEdit, the seq
//     the group would carry is positive (the declaration or the execution
//     landed in the ordered stream) and the session is attributed;
//  2. the session-content marker is present. Absent, the writer stops and
//     writes nothing: no group, no tally, no file in the run directory;
//  3. the input names a path, an index exists above the working directory,
//     and the file is under it;
//  4. a reader of the registry claims the file under that root
//     (lang.Registry.For, which reads nothing).
//
// Only after (4) are the file's content read, the index header read, the
// claiming reader's configuration read under the root (lang.Registry.Claim,
// one capped read, once per root for the call) and its Overlay called; the
// content read, the header read and the Overlay call each go through a
// package-level func var so a test can count them. A reader whose
// configuration under the root is refused reads no file there: no group. A
// reader admitted without overlays (an empty OverlayCarried) is not called:
// no group, and the index's facts stand for the file.
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
	// overlayResolve stands in for the claimed reader's Overlay in a test and
	// is nil in the binary (resolveOverlay). It has no initialiser: RES-2's
	// scan walks a var initialiser and every call it reaches, by name, and the
	// name Overlay reaches pylite's regexps (TestRES2_NoPackageLevelRegexpCompile).
	overlayResolve func(r lang.Resolver, rel string, content []byte, base graph.Rev, seq int64) ([]graph.Record, error)
	// overlayWire stands in for index.Wire in a test, so a test can register a
	// reader this build does not wire, and is nil in the binary
	// (wiredRegistry). It has no initialiser, for overlayResolve's reason:
	// index.Wire reaches pylite's regexps by name.
	overlayWire func() *lang.Registry
)

// wiredRegistry is a test's overlayWire when one is set, else the registry
// index.Wire builds, per call (keptGoEdit).
func wiredRegistry() *lang.Registry {
	if overlayWire != nil {
		return overlayWire()
	}
	return index.Wire()
}

// resolveOverlay is a test's overlayResolve when one is set, else r's own
// Overlay: r is the reader the registry configured for the edit's root
// (lang.Registry.Claim), so a Go file maps its imports by that root's
// go.mod, as the index does.
func resolveOverlay(r lang.Resolver, rel string, content []byte, base graph.Rev, seq int64) ([]graph.Record, error) {
	if overlayResolve != nil {
		return overlayResolve(r, rel, content, base, seq)
	}
	return r.Overlay(rel, content, base, seq)
}

// overlayContentError is a constant error, which costs no init allocation
// (RES-2) as a package-level errors.New does.
type overlayContentError string

func (e overlayContentError) Error() string { return string(e) }

const errOverlayContent overlayContentError = "hook: overlay content unreadable or over the cap"

// readOverlayFile reads a regular file of at most MaxOverlayContentBytes.
// The path is one an edit names, so a clone controls it: regfile opens it
// without blocking and checks the descriptor, so a named pipe with no
// writer, or a link to a device, is refused at once (READ-SAFE1a R9, R10)
// rather than holding the hook, and a file over the cap, or one that grows
// past it while it is read, is refused (R11).
func readOverlayFile(path string) ([]byte, error) {
	b, err := regfile.ReadCapped(path, MaxOverlayContentBytes)
	if errors.Is(err, regfile.ErrNotRegular) || errors.Is(err, regfile.ErrOverCap) {
		return nil, errOverlayContent
	}
	return b, err
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
	// path is the path the input names where the caller decoded it already
	// (the declaration's DeclaredPath), nil for the gate to decode it after
	// the marker (keptGoEdit).
	path *string
	// kind is graph.OverlayDeclared (Pre) or graph.OverlayLanded (Post), and
	// seq the seq of the record the group is placed at.
	kind string
	seq  int64
	// v5 is the call's record scope at v5, as the writer's own record read it
	// (the declaration's or the execution's gate), so the overlay reads no
	// scope of its own. Its zero value keeps nothing.
	v5 bool
	// gate, when set, receives the writer's keptGoEdit, registry included,
	// so a post's unit-brief read and conflict check take it rather than
	// evaluating the gate or a reader's configuration step again.
	gate *keptGate
}

// keptGate is keptGoEdit's result as a writer took it: the index root, the
// file's slash path under it and the registry a reader of which claims it,
// ok false where a check stopped it. The registry is the one the writer
// configures the reader through (Claim), so a later Claim of the same file
// in this call, by the post's unit-brief read or conflict check, reads no
// configuration again. unclaimed is the gate stopped by its last check
// alone: every check before it passed, no reader of reg claims rel, and
// rel's kind is no row of the kinds table (lang.NonSource); rel and reg are
// set and ok is false. The post says that to the person once per kind
// (Post.Steer, takeKindLine). unindexed is the language of the reader that
// claims rel where the writer stopped at the index's header: the reader is
// registered indexed and the header names no producer of its language, or
// one other than the string this version of the reader states
// (lang.Registry.Unindexed). The writer clears ok there, so the post reads
// no unit for the edit and runs no configuration step, and says that to the
// person once per language (takeKindLine). unread is the refusal of the
// claiming reader's configuration step under the root, where the writer
// stopped at the reader's Claim, and unreadLang that reader's language: the
// post says which file was not read and why once per file (takeKindLine).
// lastIndex is the language of the configured reader where it is admitted
// without overlays (its OverlayCarried is empty), and lastRev the index
// header's commit: the writer keeps no group, ok stays set, so the post's
// unit-brief read takes the index's facts for the file, and the post says
// once per language that those are the facts at lastRev (takeKindLine).
type keptGate struct {
	root, rel  string
	reg        *lang.Registry
	ok         bool
	unclaimed  bool
	unindexed  string
	unread     *lang.Refusal
	unreadLang string
	lastIndex  string
	lastRev    string
}

// overlayTool reports a tool whose input names a file the overlay may read.
func overlayTool(tool string) bool {
	switch tool {
	case "Write", "Edit", "MultiEdit":
		return true
	}
	return false
}

// overlayPath is the absolute, clean path of p, a path a Write, Edit or
// MultiEdit names, else "" (no path, or a relative one with no working
// directory). Whether a reader takes it is the registry's to say, under the
// index root (keptGoEdit).
func overlayPath(p *string, cwd string) string {
	if p == nil || *p == "" {
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

// keptGoEdit is the file a Write, Edit or MultiEdit that landed at seq
// names, in a session whose content is kept, where a reader of the language
// registry claims it: its absolute path, and the gate (keptGate) holding the
// index root over cwd, its slash path under that root and the registry.
// Content is kept only where the record scope is v5 (v5, as the caller's
// record read it) and the session-content marker is set. The checks run
// cheapest first, so a call below v5 reads nothing and allocates nothing,
// and a session with no session-content marker pays one marker read and
// decodes no input: the scope, the tool, the seq, the session, the marker,
// the path the input names (overlayPath), an index (graph.FindIndex), the
// file under its root, and a reader's claim on it (lang.Registry.For on the
// registry index.Wire builds, which reads nothing; the registry is built
// here, per call and past every check before it, never at init: RES-2).
// A file no reader claims returns the gate unclaimed (keptGate) when its
// kind is not a kinds-table row, and the zero gate when it is.
// declared is that path where the caller decoded it already (the
// declaration's DeclaredPath), nil to decode it here, after the marker. A
// post's unit-brief read (Post.LandedGoEdit) takes the overlay writer's
// result (keptGate), so their gates cannot part and a call evaluates the
// gate once.
func keptGoEdit(st *store.Store, session, tool string, input json.RawMessage, declared *string, cwd string, seq int64, v5 bool) (string, keptGate) {
	if !v5 || !overlayTool(tool) || seq <= 0 || session == store.UnattributedSession || !store.SessionContentMarker(st.Root()) {
		return "", keptGate{}
	}
	if declared == nil {
		declared = shape.DeclaredPath(tool, input)
	}
	path := overlayPath(declared, cwd)
	if path == "" {
		return "", keptGate{}
	}
	root := graph.FindIndex(cwd)
	if root == "" {
		return "", keptGate{}
	}
	rel, ok := underRoot(root, path)
	if !ok {
		return "", keptGate{}
	}
	reg := wiredRegistry()
	if _, ok := reg.For(root, rel); !ok {
		if lang.NonSource(rel) {
			return "", keptGate{}
		}
		return "", keptGate{rel: rel, reg: reg, unclaimed: true}
	}
	return path, keptGate{root: root, rel: rel, reg: reg, ok: true}
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

// write appends the group, if every check holds, and returns it with the
// index root and the file's path under it, ok false where a check stopped
// it; a group whose append fails is still returned, since the post's
// conflict check reads the edit, not the log (card m2-seam1 D3). It never
// reports a failure: the caller has nothing to do with one, and a panic is
// recovered here, as ok false, so the hook's own work is untouched. Every
// stop returns its zero values in full, never a bare return, so no check's
// local can leak into the results. The checks run cheapest first: a
// session with no session-content marker, every session that never
// consented to it, pays one marker read on an edit and decodes no input and
// walks no directory (the hook's allocation budgets,
// test/perf/budgets.json).
func (c overlayCall) write() (og graph.OverlayGroup, root, rel string, ok bool) {
	defer func() {
		if recover() != nil {
			og, root, rel, ok = graph.OverlayGroup{}, "", "", false
		}
	}()
	path, g := keptGoEdit(c.st, c.session, c.tool, c.input, c.path, c.cwd, c.seq, c.v5)
	if c.gate != nil {
		*c.gate = g
	}
	if !g.ok {
		return graph.OverlayGroup{}, "", "", false
	}
	at, under := g.root, g.rel
	content, has := c.content(path)
	if !has {
		return graph.OverlayGroup{}, "", "", false
	}
	hdr, err := overlayReadHeader(filepath.Join(at, filepath.FromSlash(graph.GraphFile)))
	if err != nil || hdr.Rev.Commit == "" {
		return graph.OverlayGroup{}, "", "", false
	}
	if l, un := g.reg.Unindexed(at, under, hdr.Producers); un {
		if c.gate != nil {
			c.gate.ok, c.gate.unindexed = false, l
		}
		return graph.OverlayGroup{}, "", "", false
	}
	r, _, cerr := g.reg.Claim(at, under)
	if cerr != nil {
		// The claiming reader's configuration under this root is refused,
		// so the reader reads no file there and the edit writes no group;
		// the post says which file was not read and why (keepUnread).
		c.keepUnread(g, at, under, cerr)
		return graph.OverlayGroup{}, "", "", false
	}
	if len(r.OverlayCarried()) == 0 {
		// The reader is admitted without overlays (lang.Resolver): its
		// Overlay returns nothing, so no group is kept, and the facts that
		// stand for the file are the index's at the header's commit. The
		// gate stays ok for the unit-brief read; the post says so once per
		// language (takeKindLine).
		if c.gate != nil {
			c.gate.lastIndex, c.gate.lastRev = r.Lang(), hdr.Rev.Commit
		}
		return graph.OverlayGroup{}, "", "", false
	}
	if !g.reg.SessionKept(at, under) {
		// The claiming reader does not state that a session's edits of its
		// files are kept (lang.Kept): the consent text names no file of its
		// language among what a yes keeps, so no group is kept. The gate is
		// left as it stands, so the unit-brief read and every line are as
		// they were.
		return graph.OverlayGroup{}, "", "", false
	}
	from, kept := g.reg.SessionKeptFrom(at, under)
	if from > 0 && !scopeAtLeast(c.st.Root(), from) {
		// The claiming reader states the first consent scope whose screen
		// names its files (lang.KeptFrom), and no yes on file reaches it:
		// no group is kept. Only a reader that states one reads
		// consent.json, once per call; the gate is left as it stands.
		return graph.OverlayGroup{}, "", "", false
	}
	recs, err := resolveOverlay(r, under, content, graph.RevCommit(hdr.Rev.Commit), c.seq)
	if err != nil || len(recs) == 0 {
		return graph.OverlayGroup{}, "", "", false
	}
	// "" is no id: a refused git read (baseline.RepoID) captures nothing,
	// never a group under the path form, and says nothing to the user.
	rid, _ := baseline.RepoID(at)
	if rid == "" {
		return graph.OverlayGroup{}, "", "", false
	}
	group := graph.OverlayGroup{Session: c.session, ToolUseID: c.toolUseID, Kind: c.kind,
		File: graph.FileID(under), Digest: graph.ContentDigest(content), Commit: hdr.Rev.Commit, Seq: c.seq,
		Carried: r.OverlayCarried(), Recs: recs}
	// The log holds of a stating reader's records only the attributes its
	// list names (keepAttrs), filtered before the group is built, so the cut
	// and the marker are the build's own; the group returned keeps every
	// attribute.
	logged := group
	if from > 0 {
		logged.Recs = keepAttrs(recs, kept)
	}
	log := graph.OpenSessionLog(c.st.Root(), rid, graph.RevCommit(hdr.Rev.Commit), true)
	_ = log.AppendOverlayGroup(graph.BuildOverlayGroup(logged), func(n int64) error { return c.st.ReserveOverlay(c.session, n) })
	return group, at, under, true
}

// scopeAtLeast is whether the highest consent scope on file under root
// reaches from: false with no reader wired and false for a file that cannot
// be read. Its own func, so a reader left unwired is a false, never a panic
// the writer's recover would turn into the same no group.
func scopeAtLeast(root string, from int) bool {
	if ConsentScopeOnFile == nil {
		return false
	}
	scope, err := ConsentScopeOnFile(root)
	return err == nil && scope >= from
}

// keepAttrs is a copy of recs whose every node and edge holds only the
// attributes kept names for its kind (keptAttr); recs is not written.
func keepAttrs(recs []graph.Record, kept []string) []graph.Record {
	out := make([]graph.Record, 0, len(recs))
	for _, rec := range recs {
		switch {
		case rec.Node != nil:
			node := *rec.Node
			node.Attrs = keptAttrs("node", node.Kind, node.Attrs, kept)
			out = append(out, graph.Record{Node: &node})
		case rec.Edge != nil:
			edge := *rec.Edge
			edge.Attrs = keptAttrs("edge", string(edge.Pred), edge.Attrs, kept)
			out = append(out, graph.Record{Edge: &edge})
		default:
			out = append(out, rec)
		}
	}
	return out
}

// keptAttrs is a new map of the attributes of attrs that kept names for a
// record of part ("node" or "edge") and kind, nil when it names none.
func keptAttrs(part, kind string, attrs map[string]string, kept []string) map[string]string {
	var out map[string]string
	for name, val := range attrs {
		if !keptAttr(part, kind, name, kept) {
			continue
		}
		if out == nil {
			out = make(map[string]string, len(attrs))
		}
		out[name] = val
	}
	return out
}

// keptAttr is whether kept holds the entry "<part> <kind> <name>".
func keptAttr(part, kind, name string, kept []string) bool {
	for _, entry := range kept {
		head, rest, ok := strings.Cut(entry, " ")
		if !ok || head != part {
			continue
		}
		mid, last, ok := strings.Cut(rest, " ")
		if ok && mid == kind && last == name {
			return true
		}
	}
	return false
}

// keepUnread keeps cerr, the claiming reader's refused configuration under
// at, on the writer's gate, where one is set, for the post's line: the file
// and why (lang.Refusal) and the reader's language, the words for a kind
// that does not print. An error that is no Refusal names no file, and the
// gate keeps nothing of it.
func (c overlayCall) keepUnread(g keptGate, at, under string, cerr error) {
	var rf *lang.Refusal
	if c.gate == nil || !errors.As(cerr, &rf) {
		return
	}
	reader, _ := g.reg.For(at, under)
	c.gate.unread, c.gate.unreadLang = rf, reader.Lang()
}

// writeOverlay is the PreToolUse writer: the declared group, at the seq the
// declaration landed at (0 when it did not, which writes nothing), for
// declared, the path the declaration decoded, so the input is decoded once,
// and v5, the declaration's record scope at v5.
func (h *Handler) writeOverlay(p *Payload, declared *string, v5 bool) {
	overlayCall{st: h.st, session: h.sessionID, toolUseID: h.toolUseID, cwd: p.CWD, tool: p.ToolName, input: p.ToolInput,
		path: declared, kind: graph.OverlayDeclared, seq: h.declaredSeq, v5: v5}.write()
}

// writeOverlay is the PostToolUse writer: the landed group, a full one, at
// the seq the execution landed at. A failure event writes none (a
// PostToolUseFailure writer is a later card's).
// The group it builds is kept for the conflict check (LandedOverlay), and
// its gate for the unit-brief read (LandedGoEdit).
func (p *Post) writeOverlay() {
	if p.event == FailureEvent {
		return
	}
	og, root, rel, ok := overlayCall{st: p.st, session: p.sessionID, toolUseID: p.toolUseID, cwd: p.cwd, tool: p.toolName,
		input: p.toolInput, kind: graph.OverlayLanded, seq: p.landedSeq, v5: p.scopeV5, gate: &p.gate}.write()
	if ok {
		p.landed, p.landedKept = LandedGoEdit{Root: root, Rel: rel, Group: og, reg: p.gate.reg}, true
	}
}
