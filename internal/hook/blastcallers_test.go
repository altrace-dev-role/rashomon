package hook

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/graphtest"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang/gopg"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang/pylite"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// These tests drive the callers read through a real post: the specimen
// repository at its commit, with shapes.py added, indexed by pylite into
// .rashomon/graph.jsonl, then an Edit or MultiEdit landed through Handler and
// Post. The store holds the session-content marker and the consent scope on
// file answers pylite's kept scope, so the overlay writer's gate passes for a
// pylite file as it does in a session that consented to keep its edits. Each
// test names the case it wants; a message that starts "fixture:" means the
// test's own setup is wrong, not the read.

const (
	cacheRel    = "services/auth/auth/cache.py"
	sessionsRel = "services/auth/auth/sessions.py"
	connRel     = "services/auth/auth/conn.py"
	shapesRel   = "services/auth/auth/shapes.py"
	runRel      = "services/auth/auth/bin/run"
)

// shapesPy is a module of the specimen's auth unit that the specimen does not
// hold: a class with an attribute and two methods, one calling the other
// through self; a def holding a nested def, and a function calling it; two
// functions a third calls; and a module constant below them all. Every edit a
// test makes keeps the line count, so the ends in pyEnds hold for the file as
// each edit leaves it.
const shapesPy = `class Box:
    kind = "box"

    def area(self):
        return self.w * self.h

    def report(self):
        return self.area()


def outer():
    def inner():
        return 1
    return inner()


def calls_outer():
    return outer()


def mint(user):
    return user.id


def check(token):
    return token is not None


def issue(user):
    t0 = mint(user)
    return check(t0)


LIMIT = 3
`

// runPy is a Python file whose name has no suffix, under a directory a reader
// claims whole.
const runPy = "def go_run():\n    return 1\n\n\ndef main():\n    return go_run()\n"

// authSym is a symbol of the specimen's auth unit.
func authSym(kind, qualified string) graph.ID {
	return graph.SymID(pylite.Lang, "services/auth", kind, qualified)
}

// syntheticCaller is the i'th synthetic caller's symbol, in the jobs unit:
// no file holds it.
func syntheticCaller(i int) graph.ID {
	return graph.SymID(pylite.Lang, "jobs", "func", "jobs.synthetic_caller_"+strconv.Itoa(i))
}

// syntheticCall is a calls edge no file on disk holds, at the specimen's
// commit.
func syntheticCall(src, dst graph.ID) graph.Edge {
	return graph.Edge{Src: src, Pred: graph.Calls, Dst: dst, Tier: graph.Algorithmic, Conf: 0.9,
		Rev: graph.RevCommit(graphtest.Commit), Prov: graph.Prov{Extractor: "synthetic", Version: "1"}}
}

// pyEnds is the last body line of each def in the files the tests edit, by
// its symbol: what a reader that states where a body ends writes as "end".
func pyEnds() map[graph.ID]int {
	return map[graph.ID]int{
		authSym("func", "auth.cache.put_session"):          7,
		authSym("func", "auth.sessions.create_session"):    13,
		authSym("func", "auth.sessions.is_expired"):        17,
		authSym("func", "auth.conn.connection"):            2,
		authSym("func", "auth.db.insert_session"):          6,
		authSym("func", "auth.api.create_session_handler"): 7,
		authSym("method", "auth.shapes.Box.area"):          5,
		authSym("method", "auth.shapes.Box.report"):        8,
		authSym("func", "auth.shapes.outer"):               14,
		authSym("func", "auth.shapes.calls_outer"):         18,
		authSym("func", "auth.shapes.mint"):                22,
		authSym("func", "auth.shapes.check"):               26,
		authSym("func", "auth.shapes.issue"):               31,
		authSym("func", "auth.bin.run.go_run"):             2,
		authSym("func", "auth.bin.run.main"):               6,
	}
}

// withEnds is recs with "end" set from ends on each func and method symbol
// that carries none, checked against content first; a symbol that carries
// one keeps it, and the table must agree with it.
func withEnds(t *testing.T, recs []graph.Record, content []byte, ends map[graph.ID]int) []graph.Record {
	t.Helper()
	lines := strings.Split(string(content), "\n")
	out := make([]graph.Record, len(recs))
	for i, r := range recs {
		out[i] = r
		n := r.Node
		if n == nil || n.Kind != "sym" || (n.Attrs["symkind"] != "func" && n.Attrs["symkind"] != "method") {
			continue
		}
		end, ok := ends[n.ID]
		if !ok {
			t.Errorf("fixture: no end in the table for %s", n.ID)
			continue
		}
		if own, has := n.Attrs["end"]; has {
			if own != strconv.Itoa(end) {
				t.Errorf("fixture: the reader ends %s at %s, the table at %d", n.ID, own, end)
			}
			continue
		}
		def, err := strconv.Atoi(n.Attrs["line"])
		if err != nil {
			t.Errorf("fixture: %s has line %q", n.ID, n.Attrs["line"])
			continue
		}
		checkEnd(t, lines, n.ID, def, end)
		c := *n
		c.Attrs = maps.Clone(n.Attrs)
		c.Attrs["end"] = strconv.Itoa(end)
		out[i] = graph.Record{Node: &c}
	}
	return out
}

// checkEnd fails the fixture when end is not the last line of the body of the
// def at line def: a line holding code, at or after the def, followed by the
// end of the file, a blank line or a line no deeper than the def.
func checkEnd(t *testing.T, lines []string, id graph.ID, def, end int) {
	t.Helper()
	if def < 1 || end < def || end > len(lines) {
		t.Errorf("fixture: %s spans lines %d to %d of %d", id, def, end, len(lines))
		return
	}
	depth := func(s string) int { return len(s) - len(strings.TrimLeft(s, " ")) }
	if strings.TrimSpace(lines[end-1]) == "" {
		t.Errorf("fixture: %s ends at line %d, which is blank", id, end)
	}
	if end < len(lines) {
		if next := lines[end]; strings.TrimSpace(next) != "" && depth(next) > depth(lines[def-1]) {
			t.Errorf("fixture: line %d, after the end of %s, is still in its body", end+1, id)
		}
	}
}

// dirClaim is pylite claiming every file under one directory, whatever its
// name: a reader whose claim is not a suffix.
type dirClaim struct {
	pylite.Resolver
	dir string
}

func (d dirClaim) Claims(_, rel string) bool { return strings.HasPrefix(rel, d.dir) }

// pyRegistry is the wiring these tests give the hook: Go's reader, the one
// the port serves, then res registered indexed and configured as itself.
func pyRegistry(res pylite.Resolver) *lang.Registry {
	r := &lang.Registry{}
	r.Register(gopg.Resolver{})
	r.RegisterIndexed(res, func(string) (lang.Resolver, error) { return res, nil })
	return r
}

// pyFix is a post fixture over the specimen repository: its files on disk
// under a Git root that is the payloads' cwd, the hook's registry and overlay
// seams set for the test, and the index opens, the file reads and the
// consent scope's asks counted.
type pyFix struct {
	*stFix
	root string
	res  pylite.Resolver
	rels []string
	recs []graph.Record
	syn  []graph.Edge
	env  SteerEnv
	// opens is how many times the callers pass opened the index, and
	// portReads how many times the Go port's Callers ran.
	opens     int
	portReads int
	// asked is what the consent scope on file was asked, and files how many
	// times the hook read a file or an index header through its seams
	// (overlayReadFile, overlayReadHeader).
	asked *cg2Asked
	files int
}

// newPyFix writes the specimen, shapes.py and extra under a fresh Git root
// and wires reg's registry (pyRegistry when nil) as the hook's. The store
// holds the session-content marker, and the consent scope on file answers
// pylite.SessionKeptScope, the first scope whose screen names pylite's
// files. Nothing is indexed until the test calls index.
func newPyFix(t *testing.T, extra map[string]string, reg func(pylite.Resolver) *lang.Registry) *pyFix {
	t.Helper()
	sp := graphtest.New(t)
	root, err := filepath.EvalSymlinks(sp.Root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	f := &pyFix{stFix: newST(t), root: root, res: sp.Res}
	f.cwd = root
	if err := store.WriteSessionContentMarker(f.st.Root()); err != nil {
		t.Fatal(err)
	}
	f.asked = cg2Scope(t, pylite.SessionKeptScope, nil)
	files := map[string]string{shapesRel: shapesPy}
	maps.Copy(files, extra)
	for rel, body := range files {
		path := f.abs(rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		f.rels = append(f.rels, rel)
	}
	for rel := range sp.Files {
		if strings.HasSuffix(rel, ".py") {
			f.rels = append(f.rels, rel)
		}
	}
	slices.Sort(f.rels)
	if reg == nil {
		reg = pyRegistry
	}
	r := reg(sp.Res)
	ends := pyEnds()
	wire0, resolve0, open0 := overlayWire, overlayResolve, blastIndexOpen
	file0, header0 := overlayReadFile, overlayReadHeader
	t.Cleanup(func() {
		overlayWire, overlayResolve, blastIndexOpen = wire0, resolve0, open0
		overlayReadFile, overlayReadHeader = file0, header0
	})
	overlayReadFile = func(p string) ([]byte, error) { f.files++; return file0(p) }
	overlayReadHeader = func(p string) (graph.Header, error) { f.files++; return header0(p) }
	overlayWire = func() *lang.Registry { return r }
	overlayResolve = func(lr lang.Resolver, rel string, content []byte, base graph.Rev, seq int64) ([]graph.Record, error) {
		recs, err := lr.Overlay(rel, content, base, seq)
		if err != nil {
			return recs, err
		}
		return withEnds(t, recs, content, ends), nil
	}
	blastIndexOpen = func(path string) (io.ReadCloser, error) {
		f.opens++
		return openBlastIndex(path)
	}
	f.env = stEnv(steer.DisclosureVersion)
	f.env.Blast = BlastPort{
		Lang: gopg.Lang,
		Locate: func(root, _ string) (BlastSite, bool) {
			return BlastSite{Root: root, Package: "example.com/m/a", Log: "log-1"}, true
		},
		Callers: func(BlastSite) ([]BlastCaller, error) {
			f.portReads++
			return []BlastCaller{{Package: "example.com/m/b", Dir: "b"}, {Package: "example.com/m/c", Dir: "c", Tests: true}}, nil
		},
	}
	return f
}

// abs is rel's path under the fixture's root.
func (f *pyFix) abs(rel string) string { return filepath.Join(f.root, filepath.FromSlash(rel)) }

// index writes the repository graph at the specimen's commit over the files
// on disk now, with syn's edges added, under a header hdr may change.
func (f *pyFix) index(hdr func(*graph.Header), syn ...graph.Edge) {
	f.t.Helper()
	recs, err := f.res.Extract(f.root, f.rels, graph.RevCommit(graphtest.Commit))
	if err != nil {
		f.t.Fatalf("fixture: extract: %v", err)
	}
	s := graph.NewStore()
	for _, r := range recs {
		if r.Node != nil {
			s.AssertNode(*r.Node)
		}
		if r.Edge != nil {
			s.AssertEdge(*r.Edge)
		}
	}
	for _, e := range syn {
		s.AssertEdge(e)
	}
	h := graph.Header{Repo: "r", Rev: graph.HeaderRev{Commit: graphtest.Commit},
		Producers: map[string]string{pylite.Lang: f.res.Producer()}}
	if hdr != nil {
		hdr(&h)
	}
	if err := os.MkdirAll(filepath.Join(f.root, ".rashomon"), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := graph.SaveFile(f.root, s, h); err != nil {
		f.t.Fatalf("fixture: save: %v", err)
	}
	f.recs, f.syn = recs, syn
}

// fixtureCallers stops the test unless the index holds id's node and its
// calls edges come from exactly want.
func (f *pyFix) fixtureCallers(id graph.ID, want ...graph.ID) {
	f.t.Helper()
	found := false
	var got []graph.ID
	for _, r := range f.recs {
		if r.Node != nil && r.Node.ID == id {
			found = true
		}
		if r.Edge != nil && r.Edge.Pred == graph.Calls && r.Edge.Dst == id {
			got = append(got, r.Edge.Src)
		}
	}
	for _, e := range f.syn {
		if e.Pred == graph.Calls && e.Dst == id {
			got = append(got, e.Src)
		}
	}
	slices.Sort(got)
	got = slices.Compact(got)
	want = slices.Clone(want)
	slices.Sort(want)
	if !found || !slices.Equal(got, want) {
		f.t.Fatalf("fixture: the index holds %s %v, with callers %q, want callers %q", id, found, got, want)
	}
}

// apply replaces old, which must occur once in rel on disk, with new and
// writes the result, as the Edit tool leaves the file.
func (f *pyFix) apply(rel, old, new string) {
	f.t.Helper()
	b, err := os.ReadFile(f.abs(rel))
	if err != nil {
		f.t.Fatal(err)
	}
	if n := strings.Count(string(b), old); n != 1 {
		f.t.Fatalf("fixture: %q occurs %d times in %s, want once", old, n, rel)
	}
	if err := os.WriteFile(f.abs(rel), []byte(strings.Replace(string(b), old, new, 1)), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

// read lands one call and returns its callers read.
func (f *pyFix) read(id, tool string, input map[string]any) callersRead {
	f.t.Helper()
	p := f.call(id, tool, input, 0)
	var ctx steer.Context
	return p.readCallers(f.env, &ctx, false)
}

// edit applies an Edit of rel on disk, lands it and reads its callers.
func (f *pyFix) edit(id, rel, old, new string) callersRead {
	f.t.Helper()
	f.apply(rel, old, new)
	return f.read(id, "Edit", map[string]any{"file_path": f.abs(rel), "old_string": old, "new_string": new})
}

// land applies an Edit of rel on disk and lands it, and returns its post
// unread, so a test can count what the read itself does.
func (f *pyFix) land(id, rel, old, new string) *Post {
	f.t.Helper()
	f.apply(rel, old, new)
	return f.call(id, "Edit", map[string]any{"file_path": f.abs(rel), "old_string": old, "new_string": new}, 0)
}

// insertLine writes rec, encoded as one JSON line, into the index file
// right after its header line.
func (f *pyFix) insertLine(rec graph.Record) {
	f.t.Helper()
	path := filepath.Join(f.root, filepath.FromSlash(graph.GraphFile))
	b, err := os.ReadFile(path)
	if err != nil {
		f.t.Fatal(err)
	}
	line, err := json.Marshal(rec)
	if err != nil {
		f.t.Fatal(err)
	}
	i := bytes.IndexByte(b, '\n')
	if i < 0 {
		f.t.Fatalf("fixture: the index file holds no header line")
	}
	out := append(append(append(slices.Clone(b[:i+1]), line...), '\n'), b[i+1:]...)
	if err := os.WriteFile(path, out, 0o644); err != nil {
		f.t.Fatal(err)
	}
}

// wantOpens fails the test unless the callers pass opened the index n times.
func (f *pyFix) wantOpens(what string, n int) {
	f.t.Helper()
	if f.opens != n {
		f.t.Errorf("%s: the index was opened %d times for callers, want %d", what, f.opens, n)
	}
}

// wantCase stops the test unless r's case is want.
func wantCase(t *testing.T, what string, r callersRead, want callersCase) {
	t.Helper()
	if r.Case != want {
		t.Fatalf("%s: case %s, want %s (func %q, callers %q)", what, r.Case, want, r.Func, r.Callers)
	}
}

// wantRead stops the test unless r has want's case, function and callers.
func wantRead(t *testing.T, what string, r, want callersRead) {
	t.Helper()
	wantCase(t, what, r, want.Case)
	if r.Func != want.Func || !slices.Equal(r.Callers, want.Callers) {
		t.Fatalf("%s: func %q callers %q, want func %q callers %q", what, r.Func, r.Callers, want.Func, want.Callers)
	}
}

// TestBlastCallers_RouteIsTheClaimingReaderNotTheSuffix: the reader that
// reads an edited file is the registry's claimant of it. A reader claiming a
// whole directory has its suffix-less file read and its function named; a
// Python file no reader claims reads nothing; a file of the port's language
// keeps the package path. Neither of the last two opens the index.
func TestBlastCallers_RouteIsTheClaimingReaderNotTheSuffix(t *testing.T) {
	t.Run("a directory claim reads a file with no suffix", func(t *testing.T) {
		f := newPyFix(t, map[string]string{runRel: runPy}, func(res pylite.Resolver) *lang.Registry {
			d := dirClaim{Resolver: res, dir: "services/auth/auth/bin/"}
			r := &lang.Registry{}
			r.RegisterIndexed(d, func(string) (lang.Resolver, error) { return d, nil })
			r.Register(gopg.Resolver{})
			r.RegisterIndexed(res, func(string) (lang.Resolver, error) { return res, nil })
			return r
		})
		f.index(nil)
		goRun, mainFn := authSym("func", "auth.bin.run.go_run"), authSym("func", "auth.bin.run.main")
		f.fixtureCallers(goRun, mainFn)
		r := f.edit("toolu_r1", runRel, "return 1", "return 2")
		if r.Case != callersNote && r.Case != callersStale {
			t.Fatalf("the claimed file: case %s, want note or stale", r.Case)
		}
		if r.Func != goRun || !slices.Equal(r.Callers, []graph.ID{mainFn}) {
			t.Fatalf("the claimed file: func %q callers %q, want %q called by %q", r.Func, r.Callers, goRun, mainFn)
		}
	})
	t.Run("a Python file no reader claims reads nothing", func(t *testing.T) {
		f := newPyFix(t, nil, func(pylite.Resolver) *lang.Registry {
			r := &lang.Registry{}
			r.Register(gopg.Resolver{})
			return r
		})
		f.index(nil)
		wantCase(t, "an unclaimed file", f.edit("toolu_r2", cacheRel, "ex=3600", "ex=7200"), callersNone)
		f.wantOpens("an unclaimed file", 0)
	})
	t.Run("a Go file keeps the package path", func(t *testing.T) {
		f := newPyFix(t, nil, nil)
		f.index(nil)
		edit := map[string]any{"file_path": filepath.Join(f.root, "a", "a.go"), "old_string": "1", "new_string": "2"}
		wantCase(t, "a Go file", f.read("toolu_r3", "Edit", edit), callersPort)
		f.wantOpens("a Go file", 0)
		if f.portReads != 0 {
			t.Errorf("the callers read ran the port %d times, want 0: the package path is blast's", f.portReads)
		}
	})
}

// TestBlastCallers_ZeroCallersPrintsNoNote: a function the index shows no
// caller of reads as zero, which no note names.
func TestBlastCallers_ZeroCallersPrintsNoNote(t *testing.T) {
	f := newPyFix(t, nil, nil)
	f.index(nil)
	isExpired := authSym("func", "auth.sessions.is_expired")
	f.fixtureCallers(isExpired)
	r := f.edit("toolu_z", sessionsRel, "< datetime.utcnow()", "<= datetime.utcnow()")
	wantRead(t, "an edit nothing calls into", r, callersRead{Case: callersZero, Func: isExpired})
}

// TestBlastCallers_OneCallerLine: an edit of put_session's body names its one
// caller, create_session; the file differs from the bytes the index read.
func TestBlastCallers_OneCallerLine(t *testing.T) {
	f := newPyFix(t, nil, nil)
	f.index(nil)
	put, create := authSym("func", "auth.cache.put_session"), authSym("func", "auth.sessions.create_session")
	f.fixtureCallers(put, create)
	r := f.edit("toolu_o", cacheRel, "ex=3600", "ex=7200")
	wantRead(t, "put_session's body", r, callersRead{Case: callersStale, Func: put, Callers: []graph.ID{create}})
}

// TestBlastCallers_ManyCallersCapAtFive: connection has three callers in the
// files and four synthetic ones; the read holds all seven, distinct and
// sorted, and its note lists the first five and says two more.
func TestBlastCallers_ManyCallersCapAtFive(t *testing.T) {
	f := newPyFix(t, nil, nil)
	conn := authSym("func", "auth.conn.connection")
	want := []graph.ID{authSym("func", "auth.db.insert_session"),
		graph.SymID(pylite.Lang, "jobs", "func", "jobs.purge_sessions.purge"),
		graph.SymID(pylite.Lang, "reports", "func", "reports.daily.active_sessions")}
	var syn []graph.Edge
	for i := 1; i <= 4; i++ {
		syn = append(syn, syntheticCall(syntheticCaller(i), conn))
		want = append(want, syntheticCaller(i))
	}
	slices.Sort(want)
	f.index(nil, syn...)
	f.fixtureCallers(conn, want...)
	r := f.edit("toolu_m", connRel, "return None", "return 0")
	wantRead(t, "connection's body", r, callersRead{Case: callersStale, Func: conn, Callers: want})
	m, _, listed, ok := callersMessage(r.Func, r.Callers, true)
	if !ok || !slices.Equal(listed, want[:maxCallersListed]) || !strings.Contains(m.Text(), "' and 2 more as calling ") {
		t.Fatalf("the note listed %q (ok %v) in %q, want %q and 2 more", listed, ok, m.Text(), want[:maxCallersListed])
	}
}

// TestBlastCallers_StaleIndexPrintsTheChangedClause: a file whose bytes
// differ from those the index read reads as stale; one whose bytes are the
// index's reads as a note.
func TestBlastCallers_StaleIndexPrintsTheChangedClause(t *testing.T) {
	put, create := authSym("func", "auth.cache.put_session"), authSym("func", "auth.sessions.create_session")
	t.Run("the index read the bytes before the edit", func(t *testing.T) {
		f := newPyFix(t, nil, nil)
		f.index(nil)
		r := f.edit("toolu_s1", cacheRel, "ex=3600", "ex=7200")
		wantRead(t, "an index older than the edit", r, callersRead{Case: callersStale, Func: put, Callers: []graph.ID{create}})
	})
	t.Run("the index read the bytes the edit left", func(t *testing.T) {
		f := newPyFix(t, nil, nil)
		f.apply(cacheRel, "ex=3600", "ex=7200")
		f.index(nil)
		r := f.read("toolu_s2", "Edit", map[string]any{"file_path": f.abs(cacheRel), "old_string": "ex=3600", "new_string": "ex=7200"})
		wantRead(t, "an index of the edited bytes", r, callersRead{Case: callersNote, Func: put, Callers: []graph.ID{create}})
	})
}

// TestBlastCallers_ALaterFunctionOfAnEditedFileStillGetsItsCallers: a second
// edit of the same file, in another function, names that function's callers.
func TestBlastCallers_ALaterFunctionOfAnEditedFileStillGetsItsCallers(t *testing.T) {
	f := newPyFix(t, nil, nil)
	f.index(nil)
	mint, check, issue := authSym("func", "auth.shapes.mint"), authSym("func", "auth.shapes.check"), authSym("func", "auth.shapes.issue")
	f.fixtureCallers(mint, issue)
	f.fixtureCallers(check, issue)
	wantRead(t, "the first edit", f.edit("toolu_l1", shapesRel, "user.id", "user.uid"),
		callersRead{Case: callersStale, Func: mint, Callers: []graph.ID{issue}})
	wantRead(t, "the second edit", f.edit("toolu_l2", shapesRel, "is not None", "is None"),
		callersRead{Case: callersStale, Func: check, Callers: []graph.ID{issue}})
}

// TestBlastCallers_RefusedFileIsNamedAndSilent: a file the index header names
// refused, and a file whose reader's configuration was refused, read as
// refused, before the index is opened for callers.
func TestBlastCallers_RefusedFileIsNamedAndSilent(t *testing.T) {
	t.Run("the header names the file", func(t *testing.T) {
		f := newPyFix(t, nil, nil)
		f.index(func(h *graph.Header) {
			h.Refused, h.RefusedFiles = 1, []graph.Refusal{{Path: cacheRel, Line: 1, Reason: "synthetic refusal"}}
		})
		wantCase(t, "a refused file", f.edit("toolu_f1", cacheRel, "ex=3600", "ex=7200"), callersRefused)
		f.wantOpens("a refused file", 0)
	})
	t.Run("the reader's configuration is refused", func(t *testing.T) {
		f := newPyFix(t, nil, func(res pylite.Resolver) *lang.Registry {
			r := &lang.Registry{}
			r.Register(gopg.Resolver{})
			r.RegisterIndexed(res, func(string) (lang.Resolver, error) { return nil, errors.New("synthetic configuration refusal") })
			return r
		})
		f.index(nil)
		wantCase(t, "a refused configuration", f.edit("toolu_f2", cacheRel, "ex=3600", "ex=7200"), callersRefused)
		f.wantOpens("a refused configuration", 0)
	})
}

// TestBlastCallers_UnindexedLanguageIsSilent: an index whose header names no
// producer of the claimant's language, or another one, reads as unindexed.
func TestBlastCallers_UnindexedLanguageIsSilent(t *testing.T) {
	for _, tc := range []struct {
		name      string
		producers map[string]string
	}{
		{"no producer of the language", map[string]string{gopg.Lang: "pgraph/1"}},
		{"an earlier producer of it", map[string]string{pylite.Lang: "pylite/0"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newPyFix(t, nil, nil)
			f.index(func(h *graph.Header) { h.Producers = tc.producers })
			wantCase(t, tc.name, f.edit("toolu_i", cacheRel, "ex=3600", "ex=7200"), callersUnindexed)
			f.wantOpens(tc.name, 0)
		})
	}
}

// TestBlastCallers_UnplacedEditIsSilent: a call that cannot show where the
// edit lies reads as unplaced and opens nothing: a Write, a replace_all, an
// empty new_string, and a new_string the file holds more than once.
func TestBlastCallers_UnplacedEditIsSilent(t *testing.T) {
	t.Run("a Write", func(t *testing.T) {
		f := newPyFix(t, nil, nil)
		f.index(nil)
		r := f.read("toolu_u1", "Write", map[string]any{"file_path": f.abs(cacheRel), "content": "LIMIT = 1\n"})
		wantCase(t, "a Write", r, callersUnplaced)
		f.wantOpens("a Write", 0)
	})
	t.Run("a replace_all", func(t *testing.T) {
		f := newPyFix(t, nil, nil)
		f.index(nil)
		f.apply(cacheRel, "ex=3600", "ex=7200")
		r := f.read("toolu_u2", "Edit", map[string]any{"file_path": f.abs(cacheRel), "old_string": "ex=3600", "new_string": "ex=7200", "replace_all": true})
		wantCase(t, "a replace_all", r, callersUnplaced)
		f.wantOpens("a replace_all", 0)
	})
	t.Run("an empty new_string", func(t *testing.T) {
		f := newPyFix(t, nil, nil)
		f.index(nil)
		wantCase(t, "an empty new_string", f.edit("toolu_u3", cacheRel, ", ex=3600", ""), callersUnplaced)
		f.wantOpens("an empty new_string", 0)
	})
	t.Run("a new_string the file holds more than once", func(t *testing.T) {
		f := newPyFix(t, nil, nil)
		f.index(nil)
		wantCase(t, "a repeated new_string", f.edit("toolu_u4", cacheRel, "ttl", "session"), callersUnplaced)
		f.wantOpens("a repeated new_string", 0)
	})
}

// TestBlastCallers_ModuleLevelEditBelowAFunctionIsSilent: an edit of a
// module constant below every def lies in no body.
func TestBlastCallers_ModuleLevelEditBelowAFunctionIsSilent(t *testing.T) {
	f := newPyFix(t, nil, nil)
	f.index(nil)
	wantCase(t, "a module constant", f.edit("toolu_x", shapesRel, "LIMIT = 3", "LIMIT = 4"), callersOutside)
	f.wantOpens("a module constant", 0)
}

// TestBlastCallers_ClassBodyOutsideEveryMethodIsSilent: an edit of a class
// attribute, in the class body but in none of its methods, names no function.
func TestBlastCallers_ClassBodyOutsideEveryMethodIsSilent(t *testing.T) {
	f := newPyFix(t, nil, nil)
	f.index(nil)
	wantCase(t, "a class attribute", f.edit("toolu_k", shapesRel, `kind = "box"`, `kind = "bag"`), callersOutside)
	f.wantOpens("a class attribute", 0)
}

// TestBlastCallers_MethodEditNamesTheMethodWithItsClass: an edit in a
// method's body names the method's symbol, which holds its class, and the
// method calling it through self.
func TestBlastCallers_MethodEditNamesTheMethodWithItsClass(t *testing.T) {
	f := newPyFix(t, nil, nil)
	f.index(nil)
	area, report := authSym("method", "auth.shapes.Box.area"), authSym("method", "auth.shapes.Box.report")
	f.fixtureCallers(area, report)
	r := f.edit("toolu_c", shapesRel, "self.w * self.h", "self.h * self.w")
	wantRead(t, "Box.area's body", r, callersRead{Case: callersStale, Func: area, Callers: []graph.ID{report}})
}

// TestBlastCallers_NestedFunctionNamesTheOuterDef: an edit inside a nested
// def names the def around it, the one symbol the index holds for that code,
// and never the nested one.
func TestBlastCallers_NestedFunctionNamesTheOuterDef(t *testing.T) {
	f := newPyFix(t, nil, nil)
	f.index(nil)
	outer, callsOuter := authSym("func", "auth.shapes.outer"), authSym("func", "auth.shapes.calls_outer")
	f.fixtureCallers(outer, callsOuter)
	r := f.edit("toolu_n", shapesRel, "        return 1", "        return 2")
	wantRead(t, "inner's body", r, callersRead{Case: callersStale, Func: outer, Callers: []graph.ID{callsOuter}})
}

// TestBlastCallers_MultiEditNamesTheFirstEditInsideABody: a MultiEdit is read
// at its first edit, in the tool's order, that lies in a body: the constant's
// edit lies in none, so mint's is the one read.
func TestBlastCallers_MultiEditNamesTheFirstEditInsideABody(t *testing.T) {
	f := newPyFix(t, nil, nil)
	f.index(nil)
	mint, issue := authSym("func", "auth.shapes.mint"), authSym("func", "auth.shapes.issue")
	edits := [][2]string{{"LIMIT = 3", "LIMIT = 4"}, {"user.id", "user.uid"}, {"is not None", "is None"}}
	var in []any
	for _, e := range edits {
		f.apply(shapesRel, e[0], e[1])
		in = append(in, map[string]any{"old_string": e[0], "new_string": e[1]})
	}
	r := f.read("toolu_me", "MultiEdit", map[string]any{"file_path": f.abs(shapesRel), "edits": in})
	wantRead(t, "the MultiEdit", r, callersRead{Case: callersStale, Func: mint, Callers: []graph.ID{issue}})
}

// TestBlastCallers_NoIndexIsSilent: in a repository with no index the
// overlay writer's gate finds none and so passes no consent scope, and the
// read stops at its scope check (none) before it opens anything.
func TestBlastCallers_NoIndexIsSilent(t *testing.T) {
	f := newPyFix(t, nil, nil)
	wantCase(t, "no index", f.edit("toolu_ni", cacheRel, "ex=3600", "ex=7200"), callersNone)
	f.wantOpens("no index", 0)
}

// TestBlastCallers_NoNoteWithoutConsent: a session whose consent is not the
// current disclosure's reads as no note, before the index is opened.
func TestBlastCallers_NoNoteWithoutConsent(t *testing.T) {
	f := newPyFix(t, nil, nil)
	f.index(nil)
	env := stEnv(0)
	env.Blast = f.env.Blast
	f.env = env
	wantCase(t, "no consent", f.edit("toolu_nc", cacheRel, "ex=3600", "ex=7200"), callersNoNote)
	f.wantOpens("no consent", 0)
}

// TestBlastCallers_RecordScopeV4IsSilent: the route stops below record
// scope v5, for the callers note as for the package note. An edit of
// put_session at record scope v4 reads as none and opens no index.
func TestBlastCallers_RecordScopeV4IsSilent(t *testing.T) {
	f := newPyFix(t, nil, nil)
	f.index(nil)
	if err := store.WriteRecordScopeMarker(f.st.Root(), store.RecordScopeV4); err != nil {
		t.Fatal(err)
	}
	wantCase(t, "record scope v4", f.edit("toolu_v4", cacheRel, "ex=3600", "ex=7200"), callersNone)
	f.wantOpens("record scope v4", 0)
}

// TestBlastCallers_NoPortWiredReadsNothing: the route stops first where no
// port is wired, ahead of the path and the Git root walk. An edit of
// put_session at record scope v5 with no port reads as none and opens no
// index.
func TestBlastCallers_NoPortWiredReadsNothing(t *testing.T) {
	f := newPyFix(t, nil, nil)
	f.index(nil)
	f.env.Blast = BlastPort{}
	wantCase(t, "no port wired", f.edit("toolu_np", cacheRel, "ex=3600", "ex=7200"), callersNone)
	f.wantOpens("no port wired", 0)
}

// TestBlastCallers_OnePassPerFunctionAndCommit: a function is read once per
// index commit and record count. The second edit of put_session reads as
// marked with no pass; an index written again at the same commit with one
// more record reads it again; another function of the file is its own pass.
func TestBlastCallers_OnePassPerFunctionAndCommit(t *testing.T) {
	f := newPyFix(t, nil, nil)
	f.index(nil)
	put := authSym("func", "auth.cache.put_session")
	create := authSym("func", "auth.sessions.create_session")
	wantCase(t, "the first edit", f.edit("toolu_p1", cacheRel, "ex=3600", "ex=7200"), callersStale)
	f.wantOpens("the first edit", 1)
	wantCase(t, "the second edit", f.edit("toolu_p2", cacheRel, "ex=7200", "ex=3600"), callersMarked)
	f.wantOpens("the second edit", 1)
	f.index(nil, syntheticCall(syntheticCaller(1), create))
	callers := []graph.ID{authSym("func", "auth.api.create_session_handler"),
		graph.SymID(pylite.Lang, "tests", "func", "tests.unit.test_sessions.test_create"), syntheticCaller(1)}
	slices.Sort(callers)
	f.fixtureCallers(create, callers...)
	r := f.edit("toolu_p3", cacheRel, "ex=3600", "ex=7200")
	if r.Case == callersMarked || r.Func != put {
		t.Fatalf("after another record count: case %s func %q, want put_session read again", r.Case, r.Func)
	}
	f.wantOpens("after another record count", 2)
	wantRead(t, "create_session", f.edit("toolu_p4", sessionsRel, "seconds=SESSION_TTL_SECONDS", "seconds=SESSION_TTL_SECONDS * 2"),
		callersRead{Case: callersStale, Func: create, Callers: callers})
	f.wantOpens("create_session", 3)
}

// TestBlastCallers_GoPackagePathUnchanged: under a registry that wires a
// second reader, a Go edit still prints the package note from the port.
func TestBlastCallers_GoPackagePathUnchanged(t *testing.T) {
	f := newPyFix(t, nil, nil)
	f.index(nil)
	edit := map[string]any{"file_path": filepath.Join(f.root, "a", "a.go"), "old_string": "1", "new_string": "2"}
	out := string(f.call("toolu_g1", "Edit", edit, 0).Steer(f.env))
	if !strings.Contains(out, blastWords) || f.portReads == 0 {
		t.Fatalf("a Go edit printed %q with the port read %d times, want the package note", out, f.portReads)
	}
	f.wantOpens("a Go edit", 0)
}

// TestBlastCallers_AScopeBelowTheReadersReadsNothing: pylite keeps a
// session's edits from pylite.SessionKeptScope on. Under a yes one below it
// the overlay writer's gate does not pass, so the read stops at its scope
// check (none): it opens no index, reads no file and asks the consent scope
// nothing of its own. The landed call asks it at least once, in the gate.
func TestBlastCallers_AScopeBelowTheReadersReadsNothing(t *testing.T) {
	f := newPyFix(t, nil, nil)
	f.asked = cg2Scope(t, pylite.SessionKeptScope-1, nil)
	f.index(nil)
	p := f.land("toolu_y8", cacheRel, "ex=3600", "ex=7200")
	asked := len(f.asked.roots)
	if asked == 0 {
		t.Fatalf("fixture: the landed call asked no consent scope, so the overlay writer's gate never ran")
	}
	f.files = 0
	var ctx steer.Context
	wantCase(t, "a scope below the reader's", p.readCallers(f.env, &ctx, false), callersNone)
	f.wantOpens("a scope below the reader's", 0)
	if f.files != 0 || len(f.asked.roots) != asked {
		t.Errorf("a scope below the reader's: the read read %d files and asked the scope %d times, want 0 and 0",
			f.files, len(f.asked.roots)-asked)
	}
}

// TestBlastCallers_TheReadersScopeReadsAndAsksNothingMore: under a yes of
// pylite.SessionKeptScope the gate passes and the read names put_session's
// caller. It takes the gate's answer, so it asks the consent scope nothing.
func TestBlastCallers_TheReadersScopeReadsAndAsksNothingMore(t *testing.T) {
	f := newPyFix(t, nil, nil)
	f.index(nil)
	put, create := authSym("func", "auth.cache.put_session"), authSym("func", "auth.sessions.create_session")
	p := f.land("toolu_y9", cacheRel, "ex=3600", "ex=7200")
	asked := len(f.asked.roots)
	var ctx steer.Context
	wantRead(t, "the reader's scope", p.readCallers(f.env, &ctx, false),
		callersRead{Case: callersStale, Func: put, Callers: []graph.ID{create}})
	if len(f.asked.roots) != asked {
		t.Errorf("the read asked the consent scope %d times, want 0", len(f.asked.roots)-asked)
	}
}

// TestBlastCallers_AGoEditAsksNoScope: the port's reader states no kept
// scope, so neither the overlay writer's gate nor the read asks the consent
// scope, and the read keeps the package path (port).
func TestBlastCallers_AGoEditAsksNoScope(t *testing.T) {
	f := newPyFix(t, nil, nil)
	f.index(nil)
	edit := map[string]any{"file_path": filepath.Join(f.root, "a", "a.go"), "old_string": "1", "new_string": "2"}
	wantCase(t, "a Go file", f.read("toolu_ga", "Edit", edit), callersPort)
	if len(f.asked.roots) != 0 {
		t.Errorf("a Go edit asked the consent scope %d times (%q), want none", len(f.asked.roots), f.asked.roots)
	}
}

// TestBlastCallers_SteerPrintsTheNoteAndRecordsCountsOnly: an edit of
// put_session prints the stale note naming create_session as its caller.
// Its steer line is keyed "callers-" and records the counts alone, no line
// of the steer log holds either name, and the step asks the consent scope
// nothing.
func TestBlastCallers_SteerPrintsTheNoteAndRecordsCountsOnly(t *testing.T) {
	f := newPyFix(t, nil, nil)
	f.index(nil)
	p := f.land("toolu_se", cacheRel, "ex=3600", "ex=7200")
	asked := len(f.asked.roots)
	out := string(p.Steer(f.env))
	const note = "rashomon record: the repository graph names 'auth.sessions.create_session' as calling " +
		"'auth.cache.put_session', which this call edited; the graph does not hold the present content of the edited file."
	if !strings.Contains(out, note) {
		t.Fatalf("the step printed %q, want it to hold %q", out, note)
	}
	if len(f.asked.roots) != asked {
		t.Errorf("the step asked the consent scope %d times, want 0", len(f.asked.roots)-asked)
	}
	const rec = "rashomon record: the repository graph names callers of the function this call edited: 1 listed, 0 more; " +
		"the graph does not hold the present content of the edited file."
	n := 0
	for _, l := range steerLines(t, f.st) {
		if l.Pattern != string(pattern.BlastRadius) || !strings.HasPrefix(l.Key, "callers-") {
			continue
		}
		n++
		if l.ModelText != rec {
			t.Errorf("the note's line records %q, want %q", l.ModelText, rec)
		}
	}
	if n != 1 {
		t.Errorf("the steer log holds %d callers lines, want 1", n)
	}
	raw, _, err := f.st.ReadSteer(stSession)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range raw {
		if bytes.Contains(b, []byte("create_session")) || bytes.Contains(b, []byte("put_session")) {
			t.Errorf("a steer line holds a name: %s", b)
		}
	}
}

// TestBlastCallers_AFunctionCallingItselfIsNotItsCaller: an index edge from
// a function into itself names no caller. put_session, called by itself and
// by create_session, reads create_session alone; is_expired, called by
// itself alone, reads as zero.
func TestBlastCallers_AFunctionCallingItselfIsNotItsCaller(t *testing.T) {
	f := newPyFix(t, nil, nil)
	put, create := authSym("func", "auth.cache.put_session"), authSym("func", "auth.sessions.create_session")
	isExpired := authSym("func", "auth.sessions.is_expired")
	f.index(nil, syntheticCall(put, put), syntheticCall(isExpired, isExpired))
	f.fixtureCallers(put, create, put)
	f.fixtureCallers(isExpired, isExpired)
	wantRead(t, "put_session", f.edit("toolu_sc1", cacheRel, "ex=3600", "ex=7200"),
		callersRead{Case: callersStale, Func: put, Callers: []graph.ID{create}})
	wantRead(t, "is_expired", f.edit("toolu_sc2", sessionsRel, "< datetime.utcnow()", "<= datetime.utcnow()"),
		callersRead{Case: callersZero, Func: isExpired})
}

// TestBlastCallers_ANoNoteReadConfiguresNothing: the route names the
// claimant by the path alone (lang.Registry.For) and the scope check reads
// the gate's answer, so a read that stops at no note (no consent to steer)
// has run no configuration step, opened no index and read no file. The
// post's registry is swapped for one whose configuration step counts.
func TestBlastCallers_ANoNoteReadConfiguresNothing(t *testing.T) {
	f := newPyFix(t, nil, nil)
	f.index(nil)
	p := f.land("toolu_cf", cacheRel, "ex=3600", "ex=7200")
	if p.gate.reg == nil || p.gate.root == "" {
		t.Fatalf("fixture: the overlay writer's gate did not pass for %s", cacheRel)
	}
	configs := 0
	reg := &lang.Registry{}
	reg.Register(gopg.Resolver{})
	reg.RegisterIndexed(f.res, func(string) (lang.Resolver, error) { configs++; return f.res, nil })
	p.gate.reg = reg
	f.opens, f.files = 0, 0
	env := stEnv(0)
	env.Blast = f.env.Blast
	var ctx steer.Context
	wantCase(t, "no consent to steer", p.readCallers(env, &ctx, false), callersNoNote)
	if configs != 0 || f.opens != 0 || f.files != 0 {
		t.Errorf("the read ran %d configuration steps, opened the index %d times and read %d files, want 0, 0 and 0",
			configs, f.opens, f.files)
	}
}

// longText is n copies of one letter; at 70 KiB it is past the callers
// pass's 64 KiB read bound.
func longText(n int) string { return strings.Repeat("y", n) }

// TestBlastCallers_ALongLineOfAnotherRecordIsPassedOver: an index line past
// the pass's read bound that is neither the edited file's node nor a calls
// edge into the function leaves the read's answer as it was.
func TestBlastCallers_ALongLineOfAnotherRecordIsPassedOver(t *testing.T) {
	f := newPyFix(t, nil, nil)
	f.index(nil)
	f.insertLine(graph.Record{Node: &graph.Node{ID: "decision:long", Kind: "decision",
		Attrs: map[string]string{"text": longText(70 << 10)}}})
	put, create := authSym("func", "auth.cache.put_session"), authSym("func", "auth.sessions.create_session")
	wantRead(t, "a long decision node", f.edit("toolu_ll", cacheRel, "ex=3600", "ex=7200"),
		callersRead{Case: callersStale, Func: put, Callers: []graph.ID{create}})
}

// TestBlastCallers_ALongCallsEdgeRefusesThePass: a calls edge into the
// function on an index line past the pass's read bound, its needle inside
// the bound's first piece (long attributes) or past it (a long source),
// refuses the pass, which reads as no index: the pass does not read that
// edge's source, and an answer without it could be short.
func TestBlastCallers_ALongCallsEdgeRefusesThePass(t *testing.T) {
	put := authSym("func", "auth.cache.put_session")
	for _, c := range []struct {
		name string
		edge graph.Edge
	}{
		{"long attributes", graph.Edge{Src: syntheticCaller(9), Pred: graph.Calls, Dst: put,
			Attrs: map[string]string{"pad": longText(70 << 10)}}},
		{"a long source", graph.Edge{Src: graph.SymID(pylite.Lang, "jobs", "func", "jobs."+longText(70<<10)),
			Pred: graph.Calls, Dst: put}},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newPyFix(t, nil, nil)
			f.index(nil)
			e := c.edge
			f.insertLine(graph.Record{Edge: &e})
			wantCase(t, c.name, f.edit("toolu_lc", cacheRel, "ex=3600", "ex=7200"), callersNoIndex)
		})
	}
}

// TestBlastCallers_ALongNodeOfTheEditedFileRefusesThePass: the edited file's
// node on an index line past the pass's read bound refuses the pass, which
// reads as no index: the pass does not read that node's digest, so it could
// not tell whether the file changed after the index was written.
func TestBlastCallers_ALongNodeOfTheEditedFileRefusesThePass(t *testing.T) {
	f := newPyFix(t, nil, nil)
	f.index(nil)
	f.insertLine(graph.Record{Node: &graph.Node{ID: graph.FileID(cacheRel), Kind: "file",
		Attrs: map[string]string{"pad": longText(70 << 10)}}})
	wantCase(t, "a long node of the edited file", f.edit("toolu_lf", cacheRel, "ex=3600", "ex=7200"), callersNoIndex)
}

// TestBlastCallers_ANeedleAcrossThePieceBoundaryRefuses: the pass reads a
// line past its 64 KiB bound in pieces. A calls needle into the function
// that ends in the first piece, starts in the next, or is split between the
// two refuses the pass (errLongLine). The first piece is the line's first
// 65536 bytes: the reader's buffer slides the line to its start before it
// fills, and reports a full buffer only then.
func TestBlastCallers_ANeedleAcrossThePieceBoundaryRefuses(t *testing.T) {
	fn := authSym("func", "auth.cache.put_session")
	needle := append([]byte(`"Pred":"calls","Dst":`), jsonText(string(fn))...)
	dir := t.TempDir()
	for s := 0; s <= len(needle); s++ {
		line := bytes.Repeat([]byte{'x'}, 70000)
		copy(line[64<<10-s:], needle)
		path := filepath.Join(dir, "g"+strconv.Itoa(s)+".jsonl")
		if err := os.WriteFile(path, append(append([]byte("hdr\n"), line...), '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, _, err := indexCallers(path, fn, graph.FileID("x.py")); !errors.Is(err, errLongLine) {
			t.Errorf("the needle starting %d bytes before the bound: err %v, want errLongLine", s, err)
		}
	}
}
