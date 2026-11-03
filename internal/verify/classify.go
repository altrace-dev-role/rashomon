package verify

import (
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/gateout"
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
	Seq int64 // the declaration's
	// ExecSeq is the picked execution's Seq, where the run's outcome enters
	// the record; nil when the execution was spilled and carries none.
	ExecSeq   *int64
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
	// FromOutput: the run's outcome is the output's, not the exit status's
	// (outputOutcome): a masked line whose masking stage keeps the summary,
	// whose gate_output says pass, fail or empty. Beside KindRecorded, in the
	// same word of padding.
	FromOutput bool
	// Wrapped: a declared gate bound the run by its output's parser, not by
	// its line (wrapped, WRAPPER-RUN1): its outcome is the output's, and the
	// call's exit status never makes it red. Set only on the gate's copy of
	// the run; one byte in the same padding.
	Wrapped bool
	// KindRecorded: Kind is the record's runner_kind, not inferred. Beside
	// Backgrounded, so the two bools share one word of padding.
	KindRecorded bool
	// FromPrinted: the run's outcome is the exit status its line printed for
	// it (readsPrinted, STATUS-PROBE), in the same word of padding.
	FromPrinted bool
	// RunnerID and MaskedByID are the execution's runner_name and masked_by
	// (call schema 8) as their place in the binary's lists (shape.RunnerNameID,
	// shape.MaskedByID), 0 where the record has none: the runner's words in
	// its row of the lists, and the stage of the pipe that hid its status. One
	// byte each, in the padding the four flags leave, so a run of any kind
	// costs no byte more than before schema 8 (D209, D173d); the words are
	// read back where they are named (shape.RunnerNameOf, shape.MaskedByOf).
	RunnerID   uint8
	MaskedByID uint8
	// ProjectID is the project the run's declaration names, as its place
	// in the run's projectKeys plus one, 0 for none known (card
	// STALE-COVER1): a learned family's root, the files it covers. A byte in
	// the padding the flags leave, as RunnerID and MaskedByID are, so the
	// run costs no byte more (TestMO1_TheRunStructsDidNotGrow).
	ProjectID uint8
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
	Seq int64
	// decl is the call that made the write, read in place for its agent
	// (agent) and the project the write is read in (project, card
	// STALE-COVER1): a pointer into the run, so the write stays 64 bytes
	// and copies nothing. nil in a write built without one, whose agent and
	// project are not known.
	decl  *store.Declaration
	Path  *string // project-relative (relativise); nil when the target is unknown
	Class PathClass
	Tool  string
	// SHA is the sha256 the post recorded for the file a non-Bash named
	// write names, as the call left it (ContentSHA256); nil where it has
	// none, and for every other write (P5A).
	SHA *string
}

// agent is the id of the subagent whose call made w; nil for the main agent,
// or where w has no call.
func (w *write) agent() *string {
	if w.decl == nil {
		return nil
	}
	return w.decl.AgentID
}

// project is the project w is read in (projectOf); nil, not known, where w
// has no call.
func (w *write) project() *string {
	if w.decl == nil {
		return nil
	}
	return projectOf(w.decl)
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
// name is in unnamed: a shell line that may write and names no target, or
// names an incomplete or withheld list; a file tool whose path is absent or
// withheld; an MCP call, which the record cannot see into. Every family
// counts it as an unknown-target write and changes no state for it
// (gateStates); the turn's row is where it fails closed
// (TurnCounts.UnnamedWriteSeq). A declaration without an
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
	runs, writes, unnamed, rewritten, _ = classifyWith(run, pickExecutions(run))
	return runs, writes, unnamed, rewritten
}

// pickExecutions is the execution each call is judged on, by ToolUseID:
// outranks's pick among the call's records. Evaluate builds it once and hands
// it to classifyWith and to integrity, so the record is grouped one way. run
// is not nil.
func pickExecutions(run *store.Run) map[string]*store.Execution {
	picked := make(map[string]*store.Execution, len(run.Executions))
	for i := range run.Executions {
		e := &run.Executions[i]
		if cur, ok := picked[e.ToolUseID]; !ok || !outranks(cur, e) {
			picked[e.ToolUseID] = e
		}
	}
	return picked
}

// classifyWith is classify over picked, pickExecutions(run), and keys, the
// projects its gate runs name (ProjectID); run is not nil.
func classifyWith(run *store.Run, picked map[string]*store.Execution) (runs []gateRun, writes []write, unnamed []int64, rewritten []int64, keys projectKeys) {
	var pending []int // indexes into runs whose kind waits on the inference
	ended := taskEnds(run)
	for i := range run.Declarations {
		d := &run.Declarations[i]
		n := len(writes)
		writes, unnamed = appendWrites(writes, unnamed, d)
		e := picked[d.ToolUseID]
		if e != nil && hashed(d) {
			writes[n].SHA = e.ContentSHA256
		}
		if e == nil || d.ToolName != "Bash" {
			continue
		}
		masked, runner, kind, schema := asRan(d, e)
		if !isGateRun(d, masked, runner) {
			continue
		}
		g := newRun(d, e, masked, runner, ended[d.ToolUseID])
		g.ProjectID = keys.id(d.Project)
		if e.ExecutedDigest != "" && e.ExecutedDigest != d.Shape.Digest {
			g.Location = ""
			rewritten = append(rewritten, d.Seq)
		}
		switch {
		case kind != nil:
			g.Kind, g.KindRecorded = *kind, true
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
	return runs, writes, unnamed, rewritten, keys
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

// newRun is the run d and e record, read as the line ran (masked and runner
// are asRan's), and judged by end, the call's outcome_join (taskEnds), where
// it was moved to the background: classify's gate runs and a declared gate's
// line runs are built here, so both read the same end (card BG-OUTCOME phase
// 1). Its location is the declaration's; the caller clears it where the line
// was rewritten. The line's digest is lineRun's, not the run's (D209).
func newRun(d *store.Declaration, e *store.Execution, masked, runner *string, end *store.OutcomeJoin) gateRun {
	g := gateRun{
		Seq: d.Seq, ExecSeq: e.Seq, ToolUseID: d.ToolUseID, AgentID: d.AgentID,
		Runner: runner, Location: d.CWDDigest, Masked: masked,
		Outcome: e.Outcome, ExitCode: e.ExitCode, Output: e.GateOutput, Backgrounded: e.Backgrounded,
	}
	if end != nil && g.Backgrounded {
		g.endedBy(end)
	}
	if d.Shape.Program != nil {
		g.Program = *d.Shape.Program
	}
	// The runner the line launched, which the call schema 8 execution names
	// in the binary's own words; an older record has none. The program the
	// gate is known by is then the table's word for that runner (pytest for
	// python3 -m pytest), as gateout keys its tables by it (D229).
	if e.SchemaVersion >= 8 {
		if e.RunnerName != nil {
			g.RunnerID = shape.RunnerNameID(*e.RunnerName)
		}
		if e.MaskedBy != nil {
			g.MaskedByID = shape.MaskedByID(*e.MaskedBy)
		}
	}
	g.Program = gateout.ProgramFor(g.Program, shape.RunnerNameOf(g.RunnerID))
	g.FromOutput = readsOutput(&g, e.RulesVersion)
	g.FromPrinted = readsPrinted(&g, e.RulesVersion)
	return g
}

// taskEnds is run's outcome_join records by the call's id, the first for an
// id (card BG-OUTCOME phase 1); nil when it has none, which allocates
// nothing.
func taskEnds(run *store.Run) map[string]*store.OutcomeJoin {
	if len(run.OutcomeJoins) == 0 {
		return nil
	}
	ended := make(map[string]*store.OutcomeJoin, len(run.OutcomeJoins))
	for i := range run.OutcomeJoins {
		if j := &run.OutcomeJoins[i]; ended[j.ToolUseID] == nil {
			ended[j.ToolUseID] = j
		}
	}
	return ended
}

// endedBy is a background run judged by its end: the join's outcome and exit
// code, as a post reads PostToolUse and PostToolUseFailure. The run keeps its
// launch's seq and location: what it tested is the tree it was launched over.
func (g *gateRun) endedBy(end *store.OutcomeJoin) {
	g.Outcome, g.ExitCode, g.Backgrounded = end.Outcome, end.ExitCode, false
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

// isGateRun: the line names a runner, hides one's status, or is class test --
// shape.GateRun over the fields as the line ran, the rule the post hook
// applies before it reads a call's output.
func isGateRun(d *store.Declaration, masked, runner *string) bool {
	return shape.GateRun(runner, masked, d.Shape.VerbClass)
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
// written a path the record cannot name. The paths it names are
// writeTargets'.
func appendWrites(writes []write, unnamed []int64, d *store.Declaration) ([]write, []int64) {
	file, shell := writeTargets(d)
	named := func(p string) {
		rel := relativise(p, d.Project, d.CWD)
		writes = append(writes, write{Seq: d.Seq, decl: d, Path: &rel, Class: ClassOf(rel), Tool: d.ToolName})
	}
	blind := func() {
		writes = append(writes, write{Seq: d.Seq, decl: d, Tool: d.ToolName})
		unnamed = append(unnamed, d.Seq)
	}
	switch {
	case d.Shape.VerbClass == shape.VerbMCP:
		blind()
	case d.ToolName != "Bash" && d.Shape.VerbClass == shape.VerbWrite:
		if file == nil {
			blind()
		} else {
			named(*file)
		}
	case len(shell) > 0:
		for _, p := range shell {
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

// writeTargets is the paths d names as written, as the call named them: a
// file tool's FilePath, or a shell call's WrittenPaths, and neither for an
// MCP call. appendWrites records them as named writes and StoreWritten
// matches them against the store root, so the trust drop and integrity read
// the same writes (D173). It allocates nothing.
func writeTargets(d *store.Declaration) (file *string, shell []string) {
	switch {
	case d.Shape.VerbClass == shape.VerbMCP:
		return nil, nil
	case d.ToolName != "Bash" && d.Shape.VerbClass == shape.VerbWrite:
		return d.FilePath, nil
	}
	return nil, d.WrittenPaths
}

// hashed reports a declaration whose post hashes the file it names (P5A): a
// non-Bash write that names its path, which appendWrites gives one named write.
func hashed(d *store.Declaration) bool {
	return d.ToolName != "Bash" && d.Shape.VerbClass == shape.VerbWrite && d.FilePath != nil
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

// RelevantUnder reports whether a path a call names as written, read as the
// classifier reads a declaration's (relativise against root, then cwd),
// lies under one of them and is of a class a gate counts (Relevant). It is
// the stale trigger's test of a write before it reads the run (card
// STALE-NUDGE1): a path outside both is not one the trigger judges.
func RelevantUnder(path, root, cwd string) bool {
	rel := relativise(path, &root, &cwd)
	if filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, "../") {
		return false
	}
	return Relevant(ClassOf(rel))
}

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
