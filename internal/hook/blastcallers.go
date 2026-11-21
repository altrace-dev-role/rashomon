package hook

import (
	"bufio"
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"slices"
	"strconv"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/jsonscan"
	"github.com/altrace-dev-role/rashomon/internal/regfile"
	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// The callers note: after an Edit or MultiEdit of a file that a reader other
// than the port's claims, the model is told which functions the repository
// graph's calls edges show calling the function whose body holds the edit.
// The route is the language registry's claim on the file, never its name:
// the claimant whose language the port serves keeps the package path
// (blast), and any other claimant's files are read here. Each way the read
// ends is a named case, so every silence is one a test can tell apart.

// callersCase is how a call's callers read ended.
type callersCase uint8

const (
	// callersUnset is the zero value, which no read returns.
	callersUnset callersCase = iota
	// callersNone: the call is not one the note follows, or no reader
	// claims the file.
	callersNone
	// callersPort: the claimant's language is the one the port serves, so
	// the package path decides.
	callersPort
	// callersNoIndex: no index at or up to graph.MaxIndexDepth above the
	// working directory holds the file, or its header or its records could
	// not be read.
	callersNoIndex
	// callersNoNote: a note could reach no one (steering off, no consent,
	// no prompt).
	callersNoNote
	// callersUnindexed: the index holds no records of the claimant's
	// language written by this version of its reader.
	callersUnindexed
	// callersRefused: the index header names the file refused, the
	// claimant's configuration under the index root was refused, or the
	// claimant refused the file as the edit left it.
	callersRefused
	// callersUnplaced: the call cannot show where the edit lies: a Write, a
	// replace_all, an empty new_string, edits that cannot be read, a file
	// that cannot be read, or a new_string the file holds other than once.
	callersUnplaced
	// callersOutside: the edit lies inside no function's body.
	callersOutside
	// callersMarked: the function was read already under this index.
	callersMarked
	// callersZero: the index names no caller of the function.
	callersZero
	// callersNote: callers named, the file as the index read it.
	callersNote
	// callersStale: callers named, the file changed since the index was
	// built.
	callersStale
)

// String is the case's name, for a test's message.
func (c callersCase) String() string {
	names := [...]string{"unset", "none", "port", "no-index", "no-note", "unindexed", "refused", "unplaced",
		"outside", "marked", "zero", "note", "stale"}
	if int(c) < len(names) {
		return names[c]
	}
	return "callersCase(" + strconv.Itoa(int(c)) + ")"
}

// callersRead is a call's callers read: its case, the function or method
// whose body holds the edit, and that function's callers by the index's
// calls edges, distinct and sorted.
type callersRead struct {
	Case    callersCase
	Func    graph.ID
	Callers []graph.ID
}

// maxCallersNamed is how many callers a note names; past it the note says
// how many more.
const maxCallersNamed = 5

// blastIndexOpen opens the index file for the callers pass, capped at
// graph.MaxGraphFileBytes; a test counts the passes through it.
var blastIndexOpen = openBlastIndex

// openBlastIndex is regfile's capped open of the index file.
func openBlastIndex(path string) (io.ReadCloser, error) {
	return regfile.OpenCapped(path, int64(graph.MaxGraphFileBytes))
}

// readCallers is the call's callers read. The checks run cheapest first, and
// the first that stops the read names its case:
//   - the route: a success event's Edit, Write or MultiEdit that landed, at
//     record scope v5, of a file under a Git root that a reader of the
//     language registry claims (none), in a language other than the port's
//     (port). The registry is the overlay writer's where that writer ran in
//     this call, so the call builds it once, else wiredRegistry's;
//   - the input alone: a Write, a replace_all, an empty new_string, or edits
//     that cannot be read (unplaced);
//   - the call's steer context, built here from contextOf when the judge
//     built none (ok false): steering off, no consent or no prompt (no
//     note);
//   - the index over the working directory and its header line (no index),
//     records of the claimant's language in it (unindexed), and a refusal of
//     the file or of the claimant's configuration (refused);
//   - the file as the edit left it: each new_string in it exactly once
//     (unplaced), then the claimant's symbols over it, whose "line" and
//     "end" attributes must place an edit, the first in the tool's order
//     that lies in one, in a function's or method's body (outside);
//   - the read marker of that function under the index's commit and record
//     count (marked), taken before the pass, so a function is read once per
//     index written;
//   - one pass over the index file (indexCallers): no caller (zero), else
//     the callers, and the digest the index states for the file against its
//     bytes now (note where equal, stale where not, or where the index holds
//     no node of the file).
//
// A pass that fails reads as no index and keeps the marker: that index's
// answer stands until it is written again.
func (p *Post) readCallers(env SteerEnv, ctx *steer.Context, ok bool) callersRead {
	if !p.scopeV5 || p.landedSeq <= 0 || p.event != string(steer.EventPostToolUse) || !editTool(p.toolName) {
		return callersRead{Case: callersNone}
	}
	path := overlayPath(shape.DeclaredPath(p.toolName, p.toolInput), p.cwd)
	if path == "" {
		return callersRead{Case: callersNone}
	}
	root := blastRoot(p.cwd)
	if root == "" {
		return callersRead{Case: callersNone}
	}
	path = filepath.Join(resolved(filepath.Dir(path)), filepath.Base(path))
	rel, under := underRoot(root, path)
	if !under {
		return callersRead{Case: callersNone}
	}
	reg := p.gate.reg
	if reg == nil {
		reg = wiredRegistry()
	}
	claimant, claimed := reg.For(root, rel)
	if !claimed {
		return callersRead{Case: callersNone}
	}
	if claimant.Lang() == env.Blast.Lang {
		return callersRead{Case: callersPort}
	}
	edits, placeable := placeableEdits(p.toolName, p.toolInput)
	if !placeable {
		return callersRead{Case: callersUnplaced}
	}
	if !ok {
		*ctx = p.contextOf(env, verify.SteerView{ProjectRoot: root})
	}
	if ctx.SteerOff || ctx.ConfigUnreadable || !ctx.Consent || ctx.PromptID == nil {
		return callersRead{Case: callersNoNote}
	}
	idxRoot := graph.FindIndex(resolved(p.cwd))
	if idxRoot == "" {
		return callersRead{Case: callersNoIndex}
	}
	if rel, under = underRoot(idxRoot, path); !under {
		return callersRead{Case: callersNoIndex}
	}
	idxFile := filepath.Join(idxRoot, filepath.FromSlash(graph.GraphFile))
	hdr, err := overlayReadHeader(idxFile)
	if err != nil {
		return callersRead{Case: callersNoIndex}
	}
	if _, un := reg.Unindexed(idxRoot, rel, hdr.Producers); un {
		return callersRead{Case: callersUnindexed}
	}
	for _, r := range hdr.RefusedFiles {
		if r.Path == rel {
			return callersRead{Case: callersRefused}
		}
	}
	reader, claims, err := reg.Claim(idxRoot, rel)
	if err != nil {
		return callersRead{Case: callersRefused}
	}
	if !claims {
		return callersRead{Case: callersNone}
	}
	content, err := overlayReadFile(path)
	if err != nil {
		return callersRead{Case: callersUnplaced}
	}
	lines, placed := editLines(content, edits)
	if !placed {
		return callersRead{Case: callersUnplaced}
	}
	recs, err := resolveOverlay(reader, rel, content, graph.RevCommit(hdr.Rev.Commit), p.landedSeq)
	if err != nil {
		return callersRead{Case: callersRefused}
	}
	var fn graph.ID
	inBody := false
	for _, l := range lines {
		if fn, inBody = bodyAt(recs, l); inBody {
			break
		}
	}
	if !inBody {
		return callersRead{Case: callersOutside}
	}
	if !p.st.TakeBlastRead(p.sessionID, callersMarker(p.st.Key(), fn, hdr.Rev.Commit, hdr.Records)) {
		return callersRead{Case: callersMarked, Func: fn}
	}
	callers, digest, err := indexCallers(idxFile, fn, graph.FileID(rel))
	switch {
	case err != nil:
		return callersRead{Case: callersNoIndex, Func: fn}
	case len(callers) == 0:
		return callersRead{Case: callersZero, Func: fn}
	case digest != graph.ContentDigest(content):
		return callersRead{Case: callersStale, Func: fn, Callers: callers}
	}
	return callersRead{Case: callersNote, Func: fn, Callers: callers}
}

// placeableEdits is the edits of an Edit's or MultiEdit's input, false for a
// Write and for an input whose edits the file alone cannot place: edits that
// cannot be read, a replace_all (its text lands in more than one place), or
// an empty new_string (nothing of the edit is left to find).
func placeableEdits(tool string, input json.RawMessage) ([]overlayEdit, bool) {
	in := []byte(input)
	if tool == "Write" || len(in) > maxOverlayInputBytes || !json.Valid(in) {
		return nil, false
	}
	edits, ok := editsFrom(tool, in, jsonscan.ValueSpan(in))
	if !ok || len(edits) == 0 {
		return nil, false
	}
	for _, e := range edits {
		if e.ReplaceAll || e.New == "" {
			return nil, false
		}
	}
	return edits, true
}

// editLines is the line each edit changes (editLine), in the tool's order,
// false unless each new_string occurs in content exactly once, overlapping
// occurrences counted: only then does the file show where that edit lies.
func editLines(content []byte, edits []overlayEdit) ([]int, bool) {
	out := make([]int, len(edits))
	for i, e := range edits {
		n := []byte(e.New)
		off := bytes.Index(content, n)
		if off < 0 || bytes.Index(content[off+1:], n) >= 0 {
			return nil, false
		}
		out[i] = editLine(content, off, e)
	}
	return out, true
}

// editLine is the line of content an edit changes, where its new_string
// starts at off: past the text old_string and new_string share at their
// start, and short of the text they share at their end, the first byte that
// is not white space; for an edit that removes text or changes white space
// only, the line where the shared start ends. The shared end is measured
// past the shared start, so the two never overlap.
func editLine(content []byte, off int, e overlayEdit) int {
	pre := 0
	for pre < len(e.Old) && pre < len(e.New) && e.Old[pre] == e.New[pre] {
		pre++
	}
	suf := 0
	for suf < len(e.Old)-pre && suf < len(e.New)-pre && e.Old[len(e.Old)-1-suf] == e.New[len(e.New)-1-suf] {
		suf++
	}
	at := off + pre
	for i := pre; i < len(e.New)-suf; i++ {
		if c := e.New[i]; c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			at = off + i
			break
		}
	}
	return 1 + bytes.Count(content[:at], []byte{'\n'})
}

// bodyAt is the function or method symbol of recs whose body holds line l,
// from its "line" (the definition's, which counts as the body's) and "end"
// attributes: the innermost where two hold it, false where none does. A
// symbol that states no end holds no line.
func bodyAt(recs []graph.Record, l int) (graph.ID, bool) {
	var best graph.ID
	bestLine := 0
	for _, r := range recs {
		n := r.Node
		if n == nil || n.Kind != "sym" || (n.Attrs["symkind"] != "func" && n.Attrs["symkind"] != "method") {
			continue
		}
		def, err := strconv.Atoi(n.Attrs["line"])
		if err != nil {
			continue
		}
		end, err := strconv.Atoi(n.Attrs["end"])
		if err != nil || l < def || l > end {
			continue
		}
		if def > bestLine || def == bestLine && n.ID < best {
			best, bestLine = n.ID, def
		}
	}
	return best, bestLine > 0
}

// callersMarker is the read marker's name for fn under an index whose header
// names commit and counts records: store.FileBlastReadPrefix and 16 hex
// digits of an HMAC under the store's key, so the name holds no symbol. The
// count is in it because an index written again at one commit may hold
// other edges.
func callersMarker(key []byte, fn graph.ID, commit string, records int) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(string(fn) + "\n" + commit + "\n" + strconv.Itoa(records)))
	return store.FileBlastReadPrefix + hex.EncodeToString(m.Sum(nil))[:16]
}

// indexCallers is one pass over the index file at path, opened through
// blastIndexOpen: the distinct sources of its calls edges into fn, sorted,
// and the digest its node of file states, "" where it holds none. The first
// line, the header, is skipped. A line is matched first by the bytes the
// index writer's encoder spells for the field, and only a match is read
// field by field (jsonscan), so no record is decoded. A line is read whole,
// however long.
func indexCallers(path string, fn, file graph.ID) ([]graph.ID, string, error) {
	rc, err := blastIndexOpen(path)
	if err != nil {
		return nil, "", err
	}
	defer rc.Close() //nolint:errcheck // read only
	callNeedle := append([]byte(`"Pred":"calls","Dst":`), jsonText(string(fn))...)
	fileHead := append([]byte(`{"Node":{"ID":`), jsonText(string(file))...)
	br := bufio.NewReaderSize(rc, 64<<10)
	var (
		callers []graph.ID
		digest  string
		long    []byte
	)
	for n := 0; ; n++ {
		line, err := br.ReadSlice('\n')
		if errors.Is(err, bufio.ErrBufferFull) {
			long = append(long[:0], line...)
			for errors.Is(err, bufio.ErrBufferFull) {
				line, err = br.ReadSlice('\n')
				long = append(long, line...)
			}
			line = long
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, "", err
		}
		switch {
		case n == 0:
		case bytes.HasPrefix(line, fileHead):
			if d, ok := fileDigest(line, file); ok {
				digest = d
			}
		case bytes.Contains(line, callNeedle):
			if src, ok := callerOf(line, fn); ok {
				callers = append(callers, src)
			}
		}
		if err != nil {
			break
		}
	}
	slices.Sort(callers)
	return slices.Compact(callers), digest, nil
}

// jsonText is s as a JSON string, spelled as the index writer's encoder
// spells it (HTML characters escaped).
func jsonText(s string) []byte {
	b, _ := json.Marshal(s)
	return b
}

// callerOf is the source of the calls edge into fn that line holds, false
// where line, read field by field, holds no such edge.
func callerOf(line []byte, fn graph.ID) (graph.ID, bool) {
	if !json.Valid(line) {
		return "", false
	}
	e, err := jsonscan.Lookup(line, jsonscan.ValueSpan(line), "Edge")
	if err != nil || e.Kind != jsonscan.Object {
		return "", false
	}
	if stringAt(line, e, "Pred") != string(graph.Calls) || stringAt(line, e, "Dst") != string(fn) {
		return "", false
	}
	src := stringAt(line, e, "Src")
	return graph.ID(src), src != ""
}

// fileDigest is the digest the node of file that line holds states, "" for
// none, false where line, read field by field, holds no node of file.
func fileDigest(line []byte, file graph.ID) (string, bool) {
	if !json.Valid(line) {
		return "", false
	}
	n, err := jsonscan.Lookup(line, jsonscan.ValueSpan(line), "Node")
	if err != nil || n.Kind != jsonscan.Object || stringAt(line, n, "ID") != string(file) {
		return "", false
	}
	a, err := jsonscan.Lookup(line, n, "Attrs")
	if err != nil || a.Kind != jsonscan.Object {
		return "", true
	}
	return stringAt(line, a, "digest"), true
}

// stringAt is the string value of key in the object at obj, "" where it is
// missing or not a string.
func stringAt(buf []byte, obj jsonscan.Span, key string) string {
	sp, err := jsonscan.Lookup(buf, obj, key)
	if err != nil || sp.Kind != jsonscan.String {
		return ""
	}
	s, err := jsonscan.DecodeString(buf, sp)
	if err != nil {
		return ""
	}
	return s
}

// callersShown is the callers a note names, the first maxCallersNamed, and
// how many more there are.
func callersShown(c []graph.ID) ([]graph.ID, int) {
	if len(c) <= maxCallersNamed {
		return c, 0
	}
	return c[:maxCallersNamed], len(c) - maxCallersNamed
}
