package verify

import (
	"slices"
	"sort"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The kinds of a gate run: the runner lists' two words, and custom where
// nothing in the record says which list the runner is on.
const (
	kindTest   = shape.RunnerTest
	kindBuild  = shape.RunnerBuild
	kindCustom = "custom"
)

// gateRun is one call that ran a gate, as the state table reads it.
type gateRun struct {
	Seq       int64
	ToolUseID string
	AgentID   *string
	// Runner is the runner digest of the line as it ran, nil where the line
	// names no one runner; Location is the declaration's CWDDigest verbatim,
	// "" where it is not known or the line was rewritten. Never VerbClass:
	// a sequenced test run is class package and the same runner.
	Runner       *string
	Location     string
	Kind         string // test | build | custom
	Program      string
	Outcome      string // the execution's outcome
	ExitCode     *int
	Masked       *string
	Output       *store.GateOutput
	Backgrounded bool
}

// familyKey is a gate's identity: the same runner at the same location.
type familyKey struct{ location, runner string }

// family is the run's family and whether it has one: only where both the
// runner and the location are known. A read location and an unread one are
// digested in two domains (shape.CWDDigest, shape.UnreadCWDDigest) and never
// equal, so comparing keys is all a reader does; there is no flag to carry.
func (g gateRun) family() (familyKey, bool) {
	if g.Runner == nil || g.Location == "" {
		return familyKey{}, false
	}
	return familyKey{location: g.Location, runner: *g.Runner}, true
}

// write is one path a call wrote, or one call that may have written a path
// the record cannot name (Path nil, no Class).
type write struct {
	Seq     int64
	AgentID *string
	Path    *string // project-relative (relativise); nil when the target is unknown
	Class   PathClass
	Tool    string
}

// classify turns a run's calls into the two things the gate state reads: the
// gate runs, each with its family, and the writes, each with its path class.
// It walks the declarations once, with the executions indexed by ToolUseID,
// and returns every slice in seq order, so its output depends neither on the
// order of the run's records nor on map iteration.
//
// A gate run is a Bash call whose line, as it ran, names a runner, hides a
// runner's status (status_masked test or build) or is class test. Its family
// is its runner and its location (gateRun.family), and a run is returned
// whether it has one or not, so a reader can count it; one without a family
// credits none:
//   - a line with two hidden runners, or with a runner a cd ran before past the
//     leading fold, has no runner digest. Where it ran is not known, so it is
//     neither a run of this project's gate nor evidence against one;
//   - a location of "" is not known either (no payload cwd, or a read fold
//     past shape.MaxLeadingCd);
//   - a line another hook rewrote after the declaration (the execution's
//     ExecutedDigest is not the declaration's shape digest) ran from a
//     location the record does not hold: its Location is "" and its seq is in
//     rewritten. A tool that is not a gate run is never in rewritten.
//
// Kind is, in this order: the runner kind of the line as it ran; else, on a
// record written before schema 6, which has a runner digest and no kind, test
// if any gate run of this run with the same runner digest is status_masked
// test or class test, build if any is status_masked build; else custom. The
// inference pools by runner digest across locations, since the kind is a
// property of the runner's words; a family's state never does. Class package
// never infers build: a test run sequenced after another command is class
// package.
//
// A write is a file tool's FilePath (class write: Write, Edit, MultiEdit,
// NotebookEdit) or each of a shell line's WrittenPaths, stored relativised
// and classed by ClassOf. A call that may have written what the record cannot
// name is in unnamed and stales every family, fail-closed: a shell line that
// may write and names no target, or names an incomplete or withheld list; a
// file tool whose path is absent or withheld; an MCP call, which the record
// cannot see into (TurnCounts.UnnamedWriteSeq). A declaration without an
// execution is still a write, since nothing says it did not write, and never
// a gate run, which has no outcome without one.
//
// Sources: classify reads a declaration's FilePath and WrittenPaths as paths,
// both capped by store.CapDeclaration before the write, and no other field:
// store.Target is cloud identity, not a path, and CWD and Project are
// prefixes, not writes.
//
// Cost: one walk over the executions into one map and one over the
// declarations; nothing is allocated per record beyond the output slices.
func classify(run *store.Run) (runs []gateRun, writes []write, unnamed []int64, rewritten []int64) {
	if run == nil {
		return nil, nil, nil, nil
	}
	picked := make(map[string]*store.Execution, len(run.Executions))
	for i := range run.Executions {
		e := &run.Executions[i]
		if cur, ok := picked[e.ToolUseID]; !ok || !outranks(cur, e) {
			picked[e.ToolUseID] = e
		}
	}

	var pending []int // indexes into runs whose kind waits on the inference
	for i := range run.Declarations {
		d := &run.Declarations[i]
		writes, unnamed = appendWrites(writes, unnamed, d)
		e := picked[d.ToolUseID]
		if e == nil || d.ToolName != "Bash" {
			continue
		}
		masked, runner, kind, schema := asRan(d, e)
		if !isGateRun(d, masked, runner) {
			continue
		}
		g := gateRun{
			Seq: d.Seq, ToolUseID: d.ToolUseID, AgentID: d.AgentID,
			Runner: runner, Location: d.CWDDigest, Masked: masked,
			Outcome: e.Outcome, ExitCode: e.ExitCode, Output: e.GateOutput, Backgrounded: e.Backgrounded,
		}
		if d.Shape.Program != nil {
			g.Program = *d.Shape.Program
		}
		if e.ExecutedDigest != "" && e.ExecutedDigest != d.Shape.Digest {
			g.Location = ""
			rewritten = append(rewritten, d.Seq)
		}
		switch {
		case kind != nil:
			g.Kind = *kind
		case runner != nil && schema < 6:
			pending = append(pending, len(runs))
		default:
			g.Kind = kindCustom
		}
		runs = append(runs, g)
	}
	inferKinds(run, picked, runs, pending)

	sort.SliceStable(runs, func(i, j int) bool { return runs[i].Seq < runs[j].Seq })
	sort.SliceStable(writes, func(i, j int) bool { return writes[i].Seq < writes[j].Seq })
	slices.Sort(unnamed)
	slices.Sort(rewritten)
	return runs, writes, unnamed, rewritten
}

// outranks reports whether cur, the execution picked so far for a call, wins
// over e, the next one read: the copy of report.outcomeRecord's rule over
// report.executionsByID's order, which is the source and must stay equal to
// this (TestVER9_ThePickAgreesWithTheReport is the oracle). A failed record
// wins over any other, then the higher seq, a record with no seq below one
// with a seq, and between two alike the later read, as a stable sort and a
// pick of the last give. Owed: one copy, lifted into store, which both read.
func outranks(cur, e *store.Execution) bool {
	cf, ef := cur.Outcome == store.ExecFailed, e.Outcome == store.ExecFailed
	if cf != ef {
		return cf
	}
	return execSeq(cur) > execSeq(e)
}

func execSeq(e *store.Execution) int64 {
	if e.Seq == nil {
		return -1
	}
	return *e.Seq
}

// asRan is a Bash call's masking and runner as the line ran, the rule
// report.MaskingAsRan applies: the execution's fields where it is at schema 4
// or later and saw a tool_input (ExecutedDigest set), else the declaration's.
// schema is the version of the record the fields came from.
func asRan(d *store.Declaration, e *store.Execution) (masked, runner, kind *string, schema int) {
	if e.SchemaVersion >= 4 && e.ExecutedDigest != "" {
		return e.StatusMasked, e.RunnerDigest, e.RunnerKind, e.SchemaVersion
	}
	return d.Shape.StatusMasked, d.Shape.RunnerDigest, d.Shape.RunnerKind, d.SchemaVersion
}

// isGateRun: the line names a runner, hides one's status, or is class test.
func isGateRun(d *store.Declaration, masked, runner *string) bool {
	return runner != nil || masked != nil && (*masked == shape.MaskedTest || *masked == shape.MaskedBuild) ||
		d.Shape.VerbClass == shape.VerbTest
}

// inferKinds sets the kind of each run at pending, a record before schema 6,
// from the gate runs of the same runner digest anywhere in the run, each read
// as it ran, as the runs themselves are.
func inferKinds(run *store.Run, picked map[string]*store.Execution, runs []gateRun, pending []int) {
	if len(pending) == 0 {
		return
	}
	test, build := map[string]bool{}, map[string]bool{}
	for i := range run.Declarations {
		d := &run.Declarations[i]
		e := picked[d.ToolUseID]
		if e == nil || d.ToolName != "Bash" {
			continue
		}
		m, r, _, _ := asRan(d, e)
		if r == nil {
			continue
		}
		if d.Shape.VerbClass == shape.VerbTest || m != nil && *m == shape.MaskedTest {
			test[*r] = true
		} else if m != nil && *m == shape.MaskedBuild {
			build[*r] = true
		}
	}
	for _, i := range pending {
		switch r := *runs[i].Runner; {
		case test[r]:
			runs[i].Kind = kindTest
		case build[r]:
			runs[i].Kind = kindBuild
		default:
			runs[i].Kind = kindCustom
		}
	}
}

// appendWrites adds d's writes, and its seq to unnamed where it may have
// written a path the record cannot name.
func appendWrites(writes []write, unnamed []int64, d *store.Declaration) ([]write, []int64) {
	named := func(p string) {
		rel := relativise(p, d.Project, d.CWD)
		writes = append(writes, write{Seq: d.Seq, AgentID: d.AgentID, Path: &rel, Class: ClassOf(rel), Tool: d.ToolName})
	}
	blind := func() {
		writes = append(writes, write{Seq: d.Seq, AgentID: d.AgentID, Tool: d.ToolName})
		unnamed = append(unnamed, d.Seq)
	}
	switch {
	case d.Shape.VerbClass == shape.VerbMCP:
		blind()
	case d.ToolName != "Bash" && d.Shape.VerbClass == shape.VerbWrite:
		if d.FilePath == nil {
			blind()
		} else {
			named(*d.FilePath)
		}
	case len(d.WrittenPaths) > 0:
		for _, p := range d.WrittenPaths {
			named(p)
		}
		if d.WrittenPathsComplete == nil || !*d.WrittenPathsComplete {
			unnamed = append(unnamed, d.Seq)
		}
	case d.Shape.MayWrite:
		blind()
	}
	return writes, unnamed
}

// relativise is a written path relative to the project, the form ClassOf's
// root rules read and the one two calls from different directories of a
// project name the same file by: the Project prefix stripped, with its
// separator, else the CWD prefix, else the path as it is (ClassOf then errs
// toward relevance), then every leading "./". Project goes first because the
// cwd moves within a project and the project does not.
//
// Pure over its three strings: no filesystem, so one stored record classifies
// the same on any machine at any later time. Project is already resolved
// (baseline.Key), and CWD and the path are both as the harness wrote them, so
// a path written under a symlinked cwd strips against CWD. Separators are
// normalised first, backslashes to slashes, so a UNC prefix becomes
// //server/share/ and strips like any other, and a drive letter compares
// without case.
func relativise(filePath string, project, cwd *string) string {
	p := slashed(filePath)
	for _, dir := range [2]*string{project, cwd} {
		if dir == nil {
			continue
		}
		if rel, ok := under(p, slashed(*dir)); ok {
			p = rel
			break
		}
	}
	for strings.HasPrefix(p, "./") {
		p = p[2:]
	}
	return p
}

func slashed(s string) string { return strings.ReplaceAll(s, `\`, "/") }

// under is p below dir, without dir and the separator, and whether it is.
// A dir of "" or "/" alone is no prefix: it would make every absolute path
// relative to the root.
func under(p, dir string) (string, bool) {
	dir = strings.TrimRight(dir, "/")
	if dir == "" || len(p) <= len(dir) || p[len(dir)] != '/' || !samePrefix(p[:len(dir)], dir) {
		return p, false
	}
	return p[len(dir)+1:], true
}

// samePrefix compares two directory strings exactly, except that a leading
// drive letter (C: against c:) compares without case.
func samePrefix(a, b string) bool {
	if len(a) >= 2 && a[1] == ':' && b[1] == ':' && isLetter(a[0]) && isLetter(b[0]) {
		return a[0]|0x20 == b[0]|0x20 && a[2:] == b[2:]
	}
	return a == b
}

func isLetter(c byte) bool { return 'a' <= c|0x20 && c|0x20 <= 'z' }
