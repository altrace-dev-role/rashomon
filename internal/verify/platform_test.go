package verify

import (
	"bufio"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The platform rows' known-answer set (card PL-WIRE1 c): one edit of
// pkg/a.go in the project /repo, then the runs below, judged against the
// inferred `go test ./...` the project's CI names. The states are fixed here
// in advance from what the line says, not from what the code returns: a row
// is met only where the line shows the package's tests ran after the edit.

// ciGoTest is the inferred gate the project's own CI declares.
var ciGoTest = &InferredGate{Argv: []string{"go", "test", "./..."}, Declared: true}

// pkgFamily is the read for a write of pkg/a.go: package pkg of module
// example.com/m at the root, its facts known.
var pkgFamily = PlatformRead{Families: []PlatformFamily{
	{Package: "example.com/m/pkg", Dir: "pkg", ModuleDir: ".", Facts: FactsKnown, Hash: "h0"},
}}

// plwRun is a call of Bash: the line it ran, where (project-relative, "."
// the root), how it ended.
type plwRun struct {
	line, dir, outcome string
	bg                 bool
}

// plwRecord is plwRecordAt for an edit of pkg/a.go.
func plwRecord(t *testing.T, before bool, runs ...plwRun) *store.Run {
	t.Helper()
	return plwRecordAt(t, "pkg/a.go", before, runs...)
}

// plwRecordAt is the edit of path, then each run as a Bash call recorded as
// the hook records it: the shape of its line under dgKey, its runner's name
// where the line names one (a line shape names no runner is no gate run), and
// its location, never the command, which the hook does not record; before
// puts the runs ahead of the edit.
func plwRecordAt(t *testing.T, path string, before bool, runs ...plwRun) *store.Run {
	t.Helper()
	var calls []call
	seq := int64(1)
	add := func(c call) { calls = append(calls, c); seq++ }
	if !before {
		add(edit(seq, path))
	}
	first := seq
	for _, r := range runs {
		c := gateCall(seq, "go-test", r.outcome)
		c.bg = r.bg
		add(c)
	}
	if before {
		add(edit(seq, path))
	}
	rec := runOf(t, calls...)
	ranIn(rec, "/repo", "")
	for i, r := range runs {
		s := first + int64(i)
		if namesRunner(r.line) {
			named(t, rec, s, r.line)
		} else {
			recorded(t, rec, s, r.line)
		}
		for j := range rec.Declarations {
			if d := &rec.Declarations[j]; d.ToolUseID == callID(s) {
				d.CWDDigest = shape.CWDDigest(dgKey, filepath.Join("/repo", r.dir))
			}
		}
	}
	return inPrompt(rec, "p1")
}

// namesRunner reports whether shape names a runner on line.
func namesRunner(line string) bool {
	input, _ := json.Marshal(map[string]string{"command": line})
	return shape.Derive("Bash", input, dgKey).RunnerName != nil
}

// plwStop is the Stop over r with inf and read standing in for what
// internal/platform read; got records what the Stop asked it for.
func plwStop(r *store.Run, sp *spec.Compiled, inf *InferredGate, read PlatformRead, got *[]string) StopView {
	in := StopInput{PromptID: "p1", Key: dgKey, Inferred: inf, Platform: func(root string, written []string) PlatformRead {
		if got != nil {
			*got = append([]string{root}, written...)
		}
		return read
	}}
	return EvaluateStop(r, nil, sp, Point{Kind: PointStop, Seq: lastSeqOf(r), SessionID: "s1"}, time.Time{}, in)
}

// platformRows is v's platform_impact rows.
func platformRows(v StopView) []Obligation {
	var out []Obligation
	for _, o := range v.Obligations {
		if o.Kind == PlatformImpact {
			out = append(out, o)
		}
	}
	return out
}

// plwCases is the known-answer set: true cases (met, unmet), the near
// misses a looser reading would call met, and the lines whose words the
// record cannot show, each labelled before it ran.
var plwCases = []struct {
	name   string
	before bool
	runs   []plwRun
	want   ObligationState
}{
	// Met: a green run of the gate's runner after the edit whose words name
	// pkg from the directory it ran in.
	{"./... at the root", false, []plwRun{{line: "go test ./...", dir: ".", outcome: store.ExecOK}}, ObligationMet},
	{"./pkg/...", false, []plwRun{{line: "go test ./pkg/...", dir: ".", outcome: store.ExecOK}}, ObligationMet},
	{"./pkg exactly", false, []plwRun{{line: "go test ./pkg", dir: ".", outcome: store.ExecOK}}, ObligationMet},
	{"one common flag", false, []plwRun{{line: "go test -count=1 ./pkg/...", dir: ".", outcome: store.ExecOK}}, ObligationMet},
	{"a redirect the runner's words leave out", false, []plwRun{{line: "go test ./... 2>&1 > out.txt", dir: ".", outcome: store.ExecOK}}, ObligationMet},
	{"cd pkg, local directory mode", false, []plwRun{{line: "cd pkg && go test", dir: "pkg", outcome: store.ExecOK}}, ObligationMet},
	{"cd pkg, -v .", false, []plwRun{{line: "cd pkg && go test -v .", dir: "pkg", outcome: store.ExecOK}}, ObligationMet},
	{"the last covering run decides", false, []plwRun{
		{line: "go test ./pkg", dir: ".", outcome: store.ExecFailed}, {line: "go test ./pkg", dir: ".", outcome: store.ExecOK}}, ObligationMet},
	// Unmet: the covering run is red.
	{"red ./...", false, []plwRun{{line: "go test ./...", dir: ".", outcome: store.ExecFailed}}, ObligationUnmet},
	// Open: no run of the runner after the edit.
	{"no run", false, nil, ObligationOpen},
	{"green before the edit", true, []plwRun{{line: "go test ./...", dir: ".", outcome: store.ExecOK}}, ObligationOpen},
	{"-c builds and names no runner", false, []plwRun{{line: "go test -c ./pkg", dir: ".", outcome: store.ExecOK}}, ObligationOpen},
	{"-list names no runner", false, []plwRun{{line: "go test -list . ./pkg", dir: ".", outcome: store.ExecOK}}, ObligationOpen},
	{"another runner", false, []plwRun{{line: "go build ./...", dir: ".", outcome: store.ExecOK}}, ObligationOpen},
	// Unknown: a run of the runner after the edit whose words are none of
	// the lines that name pkg: it may have tested pkg, so it is never met,
	// and never a miss either; or the covering run's outcome is not known.
	{"another package", false, []plwRun{{line: "go test ./other/...", dir: ".", outcome: store.ExecOK}}, ObligationUnknown},
	{"a package whose name pkg prefixes", false, []plwRun{{line: "go test ./pkgx/...", dir: ".", outcome: store.ExecOK}}, ObligationUnknown},
	{"./... in a directory pkg is not under", false, []plwRun{{line: "go test ./...", dir: "other", outcome: store.ExecOK}}, ObligationUnknown},
	{"-run narrows the tests", false, []plwRun{{line: "go test -run TestA ./pkg", dir: ".", outcome: store.ExecOK}}, ObligationUnknown},
	{"two flags", false, []plwRun{{line: "go test -race -count=1 ./...", dir: ".", outcome: store.ExecOK}}, ObligationUnknown},
	{"an expanding pattern", false, []plwRun{{line: "go test $PKGS", dir: ".", outcome: store.ExecOK}}, ObligationUnknown},
	{"an import path", false, []plwRun{{line: "go test example.com/m/pkg", dir: ".", outcome: store.ExecOK}}, ObligationUnknown},
	{"backgrounded", false, []plwRun{{line: "go test ./pkg", dir: ".", outcome: store.ExecOK, bg: true}}, ObligationUnknown},
	{"status hidden by a pipe", false, []plwRun{{line: "go test ./pkg | grep -v ok", dir: ".", outcome: store.ExecOK}}, ObligationUnknown},
	{"a run it cannot read after a cover", false, []plwRun{
		{line: "go test ./pkg", dir: ".", outcome: store.ExecOK}, {line: "go test -run TestA ./pkg", dir: ".", outcome: store.ExecFailed}}, ObligationUnknown},
	{"a cover after a run it cannot read", false, []plwRun{
		{line: "go test -run TestA ./pkg", dir: ".", outcome: store.ExecFailed}, {line: "go test ./pkg", dir: ".", outcome: store.ExecOK}}, ObligationMet},
}

// TestPLW1m_PlatformRowsKnownAnswers: each case's row is the state fixed for
// it, met and unmet citing the write and the covering run, open the write,
// unknown nothing. Break: count any run of the runner after the write as a
// cover, and the near misses (another package, a prefix, another directory,
// -run) read met.
func TestPLW1m_PlatformRowsKnownAnswers(t *testing.T) {
	counts := map[ObligationState]int{}
	for _, c := range plwCases {
		t.Run(c.name, func(t *testing.T) {
			r := plwRecord(t, c.before, c.runs...)
			rows := platformRows(plwStop(r, nil, ciGoTest, pkgFamily, nil))
			if len(rows) != 1 {
				t.Fatalf("rows %+v, want one platform_impact row", rows)
			}
			o := rows[0]
			counts[c.want]++
			if o.State != c.want {
				t.Errorf("state %s, want %s", o.State, c.want)
			}
			write := r.Declarations[slices.IndexFunc(r.Declarations, func(d store.Declaration) bool { return d.ToolName == "Edit" })].Seq
			switch c.want {
			case ObligationMet, ObligationUnmet:
				cover := r.Declarations[len(r.Declarations)-1].Seq
				if !slices.Equal(o.Evidence, []int64{write, cover}) {
					t.Errorf("evidence %v, want the write %d and the run %d", o.Evidence, write, cover)
				}
			case ObligationOpen:
				if !slices.Equal(o.Evidence, []int64{write}) {
					t.Errorf("evidence %v, want the write %d", o.Evidence, write)
				}
			default:
				if o.Evidence == nil || len(o.Evidence) != 0 {
					t.Errorf("evidence %v, want none (an empty list)", o.Evidence)
				}
			}
		})
	}
	t.Logf("known answers: %d met, %d unmet, %d open, %d unknown", counts[ObligationMet], counts[ObligationUnmet], counts[ObligationOpen], counts[ObligationUnknown])
}

// TestPLW1p_PlatformRowShapeAndGate: a row is platform_impact in the session
// scope, source platform, its id over the package and its facts (a hash, or
// the word for its state); the Stop asks the platform read for the project
// root and the written .go files alone; the gate is the project's declared
// one, or a trusted spec's, never a manifest's convention; an unknown family
// is unknown whatever ran; a stale one is still judged, under its own id; an
// unreadable log is one unknown row with no family; no Platform, or no Go
// write, reads nothing. Break: let a convention-only command stand as the
// gate, and a project with a bare go.mod owes rows it never declared.
func TestPLW1p_PlatformRowShapeAndGate(t *testing.T) {
	green := plwRun{line: "go test ./...", dir: ".", outcome: store.ExecOK}
	var asked []string
	v := plwStop(plwRecord(t, false, green), nil, ciGoTest, pkgFamily, &asked)
	if !slices.Equal(asked, []string{"/repo", "pkg/a.go"}) {
		t.Errorf("the Stop asked for %q, want the root and pkg/a.go", asked)
	}
	rows := platformRows(v)
	want := Obligation{ID: ObligationID(ScopeSession, PlatformImpact, "example.com/m/pkg@h0"), Scope: ScopeSession,
		Kind: PlatformImpact, Source: SourcePlatform, State: ObligationMet}
	if len(rows) != 1 || rows[0].ID != want.ID || rows[0].Scope != want.Scope || rows[0].Source != want.Source || rows[0].State != want.State {
		t.Errorf("row %+v, want %+v", rows, want)
	}
	if !slices.Equal(v.Platform.Families, pkgFamily.Families) {
		t.Errorf("view's platform %+v, want the read", v.Platform)
	}
	convention := &InferredGate{Argv: []string{"go", "test", "./..."}}
	trusted := dgSpec(spec.Trusted, dgGate("unit", "go", "test", "./..."))
	stale := PlatformRead{Families: []PlatformFamily{{Package: "example.com/m/pkg", Dir: "pkg", ModuleDir: ".", Facts: FactsStale}}}
	unknown := PlatformRead{Families: []PlatformFamily{{Package: "example.com/m/pkg", Dir: "pkg", ModuleDir: ".", Facts: FactsUnknown}}}
	yes, no := true, false
	build := dgSpec(spec.Trusted, spec.Declared{ID: "spec:b", Gate: spec.Gate{ID: "b", Kind: "build", Argv: []string{"go", "build", "./..."}, Match: "prefix", Required: &yes}})
	advisory := dgSpec(spec.Trusted, spec.Declared{ID: "spec:t", Gate: spec.Gate{ID: "t", Kind: "test", Argv: []string{"go", "test", "./..."}, Match: "prefix", Required: &no}})
	for _, c := range []struct {
		name   string
		sp     *spec.Compiled
		inf    *InferredGate
		read   PlatformRead
		runs   []plwRun
		family string
		state  ObligationState
	}{
		{"a trusted spec's gate", trusted, nil, pkgFamily, []plwRun{green}, "example.com/m/pkg@h0", ObligationMet},
		{"a trusted spec's gate, not run", trusted, nil, pkgFamily, nil, "example.com/m/pkg@h0", ObligationOpen},
		{"a manifest's convention is no gate", nil, convention, pkgFamily, nil, "example.com/m/pkg@h0", ObligationUnknown},
		{"a build gate is no test gate", build, nil, pkgFamily, nil, "example.com/m/pkg@h0", ObligationUnknown},
		{"an advisory test gate owes nothing", advisory, nil, pkgFamily, nil, "example.com/m/pkg@h0", ObligationUnknown},
		{"no gate at all", nil, nil, pkgFamily, nil, "example.com/m/pkg@h0", ObligationUnknown},
		{"stale facts are still judged", nil, ciGoTest, stale, []plwRun{green}, "example.com/m/pkg@stale", ObligationMet},
		{"stale facts are still owed", nil, ciGoTest, stale, nil, "example.com/m/pkg@stale", ObligationOpen},
		{"no log, or no such package", nil, ciGoTest, unknown, []plwRun{green}, "example.com/m/pkg@unknown", ObligationUnknown},
		{"an unreadable log", nil, ciGoTest, PlatformRead{Families: []PlatformFamily{}, Unreadable: true}, []plwRun{green}, "", ObligationUnknown},
	} {
		rows := platformRows(plwStop(plwRecord(t, false, c.runs...), c.sp, c.inf, c.read, nil))
		if len(rows) != 1 || rows[0].ID != ObligationID(ScopeSession, PlatformImpact, c.family) || rows[0].State != c.state ||
			rows[0].Kind != PlatformImpact || rows[0].Source != SourcePlatform || rows[0].Evidence == nil {
			t.Errorf("%s: rows %+v, want one %s row over %q", c.name, rows, c.state, c.family)
		}
	}
	if rows := platformRows(EvaluateStop(plwRecord(t, false, green), nil, nil, dgPoint(30), time.Time{},
		StopInput{PromptID: "p1", Key: dgKey, Inferred: ciGoTest})); len(rows) != 0 {
		t.Errorf("no Platform: rows %+v, want none", rows)
	}
	asked = nil
	noGo := runOf(t, edit(1, "pkg/a.md"))
	ranIn(noGo, "/repo", "")
	if v := plwStop(inPrompt(noGo, "p1"), nil, ciGoTest, pkgFamily, &asked); asked != nil || len(platformRows(v)) != 0 || v.Platform.Families != nil {
		t.Errorf("a write of no .go file: asked %q, rows %+v; want no read and no row", asked, platformRows(v))
	}
}

// TestPLW1q_ANestedModuleIsNotCoveredFromAbove: a package in a module nested
// under the root (sub/q, its go.mod in sub) is no package `go test ./...` at
// the root reaches; from sub it is. Break: drop the module check, and a run
// that never built the nested module meets its row.
func TestPLW1q_ANestedModuleIsNotCoveredFromAbove(t *testing.T) {
	nested := PlatformRead{Families: []PlatformFamily{{Package: "example.com/s/q", Dir: "sub/q", ModuleDir: "sub", Facts: FactsKnown, Hash: "h1"}}}
	for _, c := range []struct {
		dir  string
		want ObligationState
	}{{".", ObligationOpen}, {"sub", ObligationMet}} {
		r := plwRecordAt(t, "sub/q/q.go", false, plwRun{line: "go test ./...", dir: c.dir, outcome: store.ExecOK})
		rows := platformRows(plwStop(r, nil, ciGoTest, nested, nil))
		if len(rows) != 1 || rows[0].State != c.want {
			t.Errorf("go test ./... in %q: rows %+v, want %s", c.dir, rows, c.want)
		}
	}
}

// TestPLW1r_CandidateLinesNameThePackageFromWhereTheyRan: run at the
// package's own directory a line names it with no pattern, ".", or ./...;
// from an ancestor, with ./... and each ./d/... down to it, or its path; each
// bare or after one common flag, never two. Break: leave out the bare form,
// and `cd pkg && go test` never meets the row.
func TestPLW1r_CandidateLinesNameThePackageFromWhereTheyRan(t *testing.T) {
	lines := func(dir, pkg string) []string {
		var out []string
		for _, argv := range candidateLines(dir, pkg) {
			out = append(out, strings.Join(argv, " "))
		}
		return out
	}
	for _, c := range []struct {
		dir, pkg string
		want     []string
		not      []string
	}{
		{".", "a/b", []string{"go test ./...", "go test ./a/...", "go test ./a/b/...", "go test ./a/b", "go test -v ./a/b", "go test -count=1 ./a/..."},
			[]string{"go test", "go test .", "go test ./a", "go test -v -race ./a/b"}},
		{"a", "a/b", []string{"go test ./...", "go test ./b/...", "go test ./b", "go test -race ./b"}, []string{"go test ./a/b", "go test ."}},
		{"a/b", "a/b", []string{"go test", "go test .", "go test ./...", "go test -short ."}, []string{"go test ./b"}},
	} {
		got := lines(c.dir, c.pkg)
		for _, w := range c.want {
			if !slices.Contains(got, w) {
				t.Errorf("%s from %s: %q not among %q", c.pkg, c.dir, w, got)
			}
		}
		for _, w := range c.not {
			if slices.Contains(got, w) {
				t.Errorf("%s from %s: %q is among the lines and names no test of it alone", c.pkg, c.dir, w)
			}
		}
	}
}

// helpFlags is every flag name the go command's help lists at a tab-indented
// line start: `go help test`, `go help testflag` and `go help build`.
func helpFlags(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, topic := range []string{"test", "testflag", "build"} {
		b, err := exec.Command("go", "help", topic).Output()
		if err != nil {
			t.Fatalf("go help %s: %v", topic, err)
		}
		sc := bufio.NewScanner(strings.NewReader(string(b)))
		for sc.Scan() {
			if line := sc.Text(); strings.HasPrefix(line, "\t-") && !strings.HasPrefix(line, "\t--") {
				out = append(out, strings.Fields(line)[0][1:])
			}
		}
	}
	return out
}

// TestPLW1n_CoverFlagsAreTheGoCommands (corpus: the tool's own docs): each
// flag candidateLines puts before a pattern is one the go command's help
// lists, read from `go help` here rather than from memory, and none of them
// narrows the tests run (-run, -skip, -list, -c, -n are not among them).
// Break: add a remembered spelling the go command does not have, or -run.
func TestPLW1n_CoverFlagsAreTheGoCommands(t *testing.T) {
	help := helpFlags(t)
	if len(help) < 50 {
		t.Fatalf("read %d flags from go help; the help is not what this test reads", len(help))
	}
	for _, f := range coverFlags {
		if f == "" {
			continue
		}
		name, _, _ := strings.Cut(strings.TrimPrefix(f, "-"), "=")
		if !slices.Contains(help, name) {
			t.Errorf("coverFlags holds %s, which go help does not list", f)
		}
		if slices.Contains([]string{"run", "skip", "list", "c", "n", "C", "bench", "fuzz"}, name) {
			t.Errorf("coverFlags holds %s, which runs part of the tests, or none, or elsewhere", f)
		}
	}
}

// TestPLW1v_EachPackageOwesFromItsOwnLastWrite: pkg/a.go is written, `go
// test ./pkg` runs green, then other/o.go is written: pkg's row is met from
// its own write, and other's, written after the run, is open. Break: take
// the session's last Go write for every package, and pkg reads open.
func TestPLW1v_EachPackageOwesFromItsOwnLastWrite(t *testing.T) {
	r := runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK), edit(3, "other/o.go"))
	ranIn(r, "/repo", "")
	named(t, r, 2, "go test ./pkg")
	read := PlatformRead{Families: []PlatformFamily{
		{Package: "example.com/m/other", Dir: "other", ModuleDir: ".", Facts: FactsKnown, Hash: "h2"},
		{Package: "example.com/m/pkg", Dir: "pkg", ModuleDir: ".", Facts: FactsKnown, Hash: "h0"},
	}}
	var asked []string
	rows := platformRows(plwStop(inPrompt(r, "p1"), nil, ciGoTest, read, &asked))
	if !slices.Equal(asked, []string{"/repo", "other/o.go", "pkg/a.go"}) {
		t.Errorf("asked %q, want both written files", asked)
	}
	if len(rows) != 2 || rows[0].State != ObligationOpen || rows[1].State != ObligationMet {
		t.Errorf("rows %+v, want other open and pkg met", rows)
	}
}
