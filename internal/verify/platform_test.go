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
var pkgFamily = PlatformRead{Log: LogRead, Families: []PlatformFamily{
	{Package: "example.com/m/pkg", Dir: "pkg", ModuleDir: ".", Facts: FactsKnown, Hash: "h0", Tests: true},
}}

// plwRun is a call of Bash: the line it ran, where (project-relative, "."
// the root), how it ended.
type plwRun struct {
	line, dir, outcome string
	bg                 bool
	// noName records the run as a record before call schema 8 does, with no
	// runner name, so its runner key is its words' digest.
	noName bool
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
		if namesRunner(r.line) && !r.noName {
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
	{"in pkg, local directory mode", false, []plwRun{{line: "go test", dir: "pkg", outcome: store.ExecOK}}, ObligationMet},
	{"in pkg, -v .", false, []plwRun{{line: "go test -v .", dir: "pkg", outcome: store.ExecOK}}, ObligationMet},
	{"the last covering run decides", false, []plwRun{
		{line: "go test ./pkg", dir: ".", outcome: store.ExecFailed}, {line: "go test ./pkg", dir: ".", outcome: store.ExecOK}}, ObligationMet},
	// Unmet: the covering run is red and names pkg alone.
	{"red ./pkg alone", false, []plwRun{{line: "go test ./pkg", dir: ".", outcome: store.ExecFailed}}, ObligationUnmet},
	{"red in pkg, local directory mode", false, []plwRun{{line: "go test", dir: "pkg", outcome: store.ExecFailed}}, ObligationUnmet},
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
	// A red run over many packages says only that one of them failed (M4).
	{"red ./... names many packages", false, []plwRun{{line: "go test ./...", dir: ".", outcome: store.ExecFailed}}, ObligationUnknown},
	{"red ./pkg/... names pkg and below", false, []plwRun{{line: "go test ./pkg/...", dir: ".", outcome: store.ExecFailed}}, ObligationUnknown},
	// A later red test run with no runner key may have run pkg's tests (M1).
	{"a red subshell after a green cover", false, []plwRun{
		{line: "go test ./...", dir: ".", outcome: store.ExecOK}, {line: "(cd pkg && go test .)", dir: ".", outcome: store.ExecFailed}}, ObligationUnknown},
	{"a red subshell with no runner name after a green cover", false, []plwRun{
		{line: "go test ./...", dir: ".", outcome: store.ExecOK}, {line: "(cd pkg && go test .)", dir: ".", outcome: store.ExecFailed, noName: true}}, ObligationUnknown},
	{"two hidden go test runs, no runner key, after a green cover", false, []plwRun{
		{line: "go test ./...", dir: ".", outcome: store.ExecOK}, {line: "go test ./pkg | tail -3; go test ./pkg | tail -3", dir: ".", outcome: store.ExecOK}}, ObligationUnknown},
	{"a red make test after a green cover", false, []plwRun{
		{line: "go test ./...", dir: ".", outcome: store.ExecOK}, {line: "make test", dir: ".", outcome: store.ExecFailed}}, ObligationUnknown},
	{"a red cd past the fold after a green cover", false, []plwRun{
		{line: "go test ./...", dir: ".", outcome: store.ExecOK}, {line: "echo x; cd pkg && go test .", dir: ".", outcome: store.ExecFailed}}, ObligationUnknown},
	// Words ahead of the runner the runner digest leaves out (M3).
	{"GOFLAGS ahead of go test", false, []plwRun{{line: "GOFLAGS=-run=TestOther go test ./...", dir: ".", outcome: store.ExecOK}}, ObligationUnknown},
	{"export GOFLAGS, then go test", false, []plwRun{{line: "export GOFLAGS=-run=TestOther; go test ./...", dir: ".", outcome: store.ExecOK}}, ObligationUnknown},
	{"a wrapper ahead of go test", false, []plwRun{{line: "timeout 60 go test ./...", dir: ".", outcome: store.ExecOK}}, ObligationUnknown},
	{"cd ahead of go test", false, []plwRun{{line: "cd pkg && go test", dir: "pkg", outcome: store.ExecOK}}, ObligationUnknown},
	{"a redirect after go test", false, []plwRun{{line: "go test ./... 2>&1 > out.txt", dir: ".", outcome: store.ExecOK}}, ObligationUnknown},
	{"-short skips tests", false, []plwRun{{line: "go test -short ./...", dir: ".", outcome: store.ExecOK}}, ObligationUnknown},
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
// the word for its state); the replay asks the platform read for the project
// root and the written .go files alone. The gate is a go test gate a trusted
// spec declares or the project's own files name, never a manifest's
// convention, a build gate, an advisory one, or a make test gate (S4). Known,
// stale and new facts are judged alike (S3); no log, or a package with no
// test file (N2), reads unknown whatever ran; an unreadable or timed-out log
// is one unknown row with no family; the packages past the cap are one
// unknown row; no Platform, or no Go write, reads nothing. Break: let a
// convention-only command stand as the gate, and a project with a bare
// go.mod owes rows it never declared.
func TestPLW1p_PlatformRowShapeAndGate(t *testing.T) {
	green := plwRun{line: "go test ./...", dir: ".", outcome: store.ExecOK}
	var asked []string
	v := plwStop(plwRecord(t, false, green), nil, ciGoTest, pkgFamily, &asked)
	if !slices.Equal(asked, []string{"/repo", "pkg/a.go"}) {
		t.Errorf("the replay asked for %q, want the root and pkg/a.go", asked)
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
	makeTest := &InferredGate{Argv: []string{"make", "test"}, Declared: true}
	trusted := dgSpec(spec.Trusted, dgGate("unit", "go", "test", "./..."))
	family := func(facts FactsState, hash string, tests bool) PlatformRead {
		return PlatformRead{Log: LogRead, Families: []PlatformFamily{
			{Package: "example.com/m/pkg", Dir: "pkg", ModuleDir: ".", Facts: facts, Hash: hash, Tests: tests}}}
	}
	yes, no := true, false
	build := dgSpec(spec.Trusted, spec.Declared{ID: "spec:b", Gate: spec.Gate{ID: "b", Kind: "build", Argv: []string{"go", "build", "./..."}, Match: "prefix", Required: &yes}})
	buildKind := dgSpec(spec.Trusted, spec.Declared{ID: "spec:k", Gate: spec.Gate{ID: "k", Kind: "build", Argv: []string{"go", "test", "./..."}, Match: "prefix", Required: &yes}})
	advisory := dgSpec(spec.Trusted, spec.Declared{ID: "spec:t", Gate: spec.Gate{ID: "t", Kind: "test", Argv: []string{"go", "test", "./..."}, Match: "prefix", Required: &no}})
	for _, c := range []struct {
		name     string
		sp       *spec.Compiled
		inf      *InferredGate
		read     PlatformRead
		runs     []plwRun
		families []string
		states   []ObligationState
	}{
		{"a trusted spec's gate", trusted, nil, pkgFamily, []plwRun{green}, []string{"example.com/m/pkg@h0"}, []ObligationState{ObligationMet}},
		{"a trusted spec's gate, not run", trusted, nil, pkgFamily, nil, []string{"example.com/m/pkg@h0"}, []ObligationState{ObligationOpen}},
		{"a manifest's convention is no gate", nil, convention, pkgFamily, nil, []string{"example.com/m/pkg@h0"}, []ObligationState{ObligationUnknown}},
		{"make test is no go test gate", nil, makeTest, pkgFamily, nil, []string{"example.com/m/pkg@h0"}, []ObligationState{ObligationUnknown}},
		{"a build gate is no test gate", build, nil, pkgFamily, nil, []string{"example.com/m/pkg@h0"}, []ObligationState{ObligationUnknown}},
		{"a build-kind gate is no test gate, go test or not", buildKind, nil, pkgFamily, nil, []string{"example.com/m/pkg@h0"}, []ObligationState{ObligationUnknown}},
		{"an advisory test gate owes nothing", advisory, nil, pkgFamily, nil, []string{"example.com/m/pkg@h0"}, []ObligationState{ObligationUnknown}},
		{"no gate at all", nil, nil, pkgFamily, nil, []string{"example.com/m/pkg@h0"}, []ObligationState{ObligationUnknown}},
		{"stale facts are judged", nil, ciGoTest, family(FactsStale, "", true), []plwRun{green}, []string{"example.com/m/pkg@stale"}, []ObligationState{ObligationMet}},
		{"stale facts are owed", nil, ciGoTest, family(FactsStale, "", true), nil, []string{"example.com/m/pkg@stale"}, []ObligationState{ObligationOpen}},
		{"a package the log does not hold is judged", nil, ciGoTest, family(FactsNew, "", true), []plwRun{green}, []string{"example.com/m/pkg@new"}, []ObligationState{ObligationMet}},
		{"a package the log does not hold is owed", nil, ciGoTest, family(FactsNew, "", true), nil, []string{"example.com/m/pkg@new"}, []ObligationState{ObligationOpen}},
		{"no log", nil, ciGoTest, PlatformRead{Log: LogNone, Families: family(FactsUnknown, "", true).Families}, []plwRun{green}, []string{"example.com/m/pkg@unknown"}, []ObligationState{ObligationUnknown}},
		{"no test file, run green", nil, ciGoTest, family(FactsKnown, "h0", false), []plwRun{green}, []string{"example.com/m/pkg@h0"}, []ObligationState{ObligationUnknown}},
		{"no test file, not run", nil, ciGoTest, family(FactsKnown, "h0", false), nil, []string{"example.com/m/pkg@h0"}, []ObligationState{ObligationUnknown}},
		{"an unreadable log", nil, ciGoTest, PlatformRead{Log: LogUnreadable, Families: []PlatformFamily{}}, []plwRun{green}, []string{""}, []ObligationState{ObligationUnknown}},
		{"a timed-out log", nil, ciGoTest, PlatformRead{Log: LogTimedOut, Families: []PlatformFamily{}}, []plwRun{green}, []string{""}, []ObligationState{ObligationUnknown}},
		{"packages past the cap", nil, ciGoTest, PlatformRead{Log: LogRead, Families: pkgFamily.Families, More: 3}, []plwRun{green},
			[]string{"example.com/m/pkg@h0", PlatformMore}, []ObligationState{ObligationMet, ObligationUnknown}},
	} {
		rows := platformRows(plwStop(plwRecord(t, false, c.runs...), c.sp, c.inf, c.read, nil))
		ok := len(rows) == len(c.families)
		for i := 0; ok && i < len(rows); i++ {
			ok = rows[i].ID == ObligationID(ScopeSession, PlatformImpact, c.families[i]) && rows[i].State == c.states[i] &&
				rows[i].Kind == PlatformImpact && rows[i].Scope == ScopeSession && rows[i].Source == SourcePlatform && rows[i].Evidence != nil
		}
		if !ok {
			t.Errorf("%s: rows %+v, want %v over %q", c.name, rows, c.states, c.families)
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
	nested := PlatformRead{Log: LogRead, Families: []PlatformFamily{{Package: "example.com/s/q", Dir: "sub/q", ModuleDir: "sub", Facts: FactsKnown, Hash: "h1", Tests: true}}}
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

// TestPLW1r_CoverLinesNameThePackageFromWhereTheyRan: run at the package's
// own directory a line names it alone with no pattern or "."; from an
// ancestor, alone with its path; and with others as ./... and each ./d/...
// down to it; each bare or after one common flag, never two, never -short.
// Break: call ./pkg/... a line that names pkg alone, and a red run of pkg and
// its subpackages reads unmet for pkg.
func TestPLW1r_CoverLinesNameThePackageFromWhereTheyRan(t *testing.T) {
	lines := func(dir, pkg string) map[string]cover {
		out := map[string]cover{}
		for _, l := range coverLines(dir, pkg) {
			out[strings.Join(l.argv, " ")] = l.cover
		}
		return out
	}
	for _, c := range []struct {
		dir, pkg string
		want     map[string]cover
		not      []string
	}{
		{".", "a/b", map[string]cover{"go test ./...": coverMany, "go test ./a/...": coverMany, "go test ./a/b/...": coverMany,
			"go test ./a/b": coverAlone, "go test -v ./a/b": coverAlone, "go test -count=1 ./a/...": coverMany},
			[]string{"go test", "go test .", "go test ./a", "go test -v -race ./a/b", "go test -short ./a/b"}},
		{"a", "a/b", map[string]cover{"go test ./...": coverMany, "go test ./b/...": coverMany, "go test ./b": coverAlone, "go test -race ./b": coverAlone},
			[]string{"go test ./a/b", "go test ."}},
		{"a/b", "a/b", map[string]cover{"go test": coverAlone, "go test .": coverAlone, "go test ./...": coverMany, "go test -json .": coverAlone},
			[]string{"go test ./b", "go test -short ."}},
	} {
		got := lines(c.dir, c.pkg)
		for w, cv := range c.want {
			if g, ok := got[w]; !ok || g != cv {
				t.Errorf("%s from %s: %q is %d (present %v), want %d", c.pkg, c.dir, w, g, ok, cv)
			}
		}
		for _, w := range c.not {
			if _, ok := got[w]; ok {
				t.Errorf("%s from %s: %q is among the lines", c.pkg, c.dir, w)
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
// flag coverLines puts before a pattern is one the go command's help
// lists, read from `go help` here rather than from memory, and none of them
// narrows the tests run (-run, -skip, -list, -short, -c, -n are not among
// them).
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
		if slices.Contains([]string{"run", "skip", "list", "short", "c", "n", "C", "bench", "fuzz"}, name) {
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
	read := PlatformRead{Log: LogRead, Families: []PlatformFamily{
		{Package: "example.com/m/other", Dir: "other", ModuleDir: ".", Facts: FactsKnown, Hash: "h2", Tests: true},
		{Package: "example.com/m/pkg", Dir: "pkg", ModuleDir: ".", Facts: FactsKnown, Hash: "h0", Tests: true},
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

// TestPLW1x_AWriteAfterTheCoverUndoesIt (M2): after `go test ./...` passed,
// a named write under pkg's directory (its testdata, a package below it) or
// to the module's go.mod or go.sum owes the row again, open; an unnamed
// write makes it unknown; a file of pkg's that reads now other than the edit
// left it makes it unknown; a write in another package leaves it met.
// Break: reset the row on pkg's own .go files alone, and an edit of go.mod
// or a golden file after the green run reads met.
func TestPLW1x_AWriteAfterTheCoverUndoesIt(t *testing.T) {
	sum := "0000000000000000000000000000000000000000000000000000000000000001"
	other := "0000000000000000000000000000000000000000000000000000000000000002"
	edited := edit(1, "pkg/a.go")
	edited.sha = sum
	for _, c := range []struct {
		name  string
		after []call
		tree  map[string]*string
		want  ObligationState
	}{
		{"nothing after", nil, nil, ObligationMet},
		{"another package", []call{edit(3, "other/b.go")}, nil, ObligationMet},
		{"go.mod", []call{edit(3, "go.mod")}, nil, ObligationOpen},
		{"go.sum", []call{edit(3, "go.sum")}, nil, ObligationOpen},
		{"pkg's testdata", []call{edit(3, "pkg/testdata/golden.txt")}, nil, ObligationOpen},
		{"a package below pkg", []call{edit(3, "pkg/sub/s.go")}, nil, ObligationOpen},
		{"an unnamed write", []call{unnamedWrite(3)}, nil, ObligationUnknown},
		{"pkg/a.go as the edit left it", nil, map[string]*string{"pkg/a.go": &sum}, ObligationMet},
		{"pkg/a.go moved since, no write recorded", nil, map[string]*string{"pkg/a.go": &other}, ObligationUnknown},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := runOf(t, append([]call{edited, gateCall(2, "go-test", store.ExecOK)}, c.after...)...)
			ranIn(r, "/repo", "")
			named(t, r, 2, "go test ./...")
			pt := Point{Kind: PointStop, Seq: lastSeqOf(r), SessionID: "s1", Tree: c.tree}
			in := StopInput{PromptID: "p1", Key: dgKey, Inferred: ciGoTest, Platform: func(string, []string) PlatformRead { return pkgFamily }}
			rows := platformRows(EvaluateStop(inPrompt(r, "p1"), nil, nil, pt, time.Time{}, in))
			var got ObligationState
			for _, o := range rows {
				if o.ID == ObligationID(ScopeSession, PlatformImpact, "example.com/m/pkg@h0") {
					got = o.State
				}
			}
			if got != c.want {
				t.Errorf("pkg's row %q, want %s (rows %+v)", got, c.want, rows)
			}
		})
	}
}

// TestPLW1o_ATestRunWithNoRunnerKeyMightHaveCovered (M1): a later run the
// record classes test but whose runner it could not key (a runner a cd ran
// past the fold, which carries no runner digest) may have run pkg's tests,
// so after a green `go test ./...` a red one leaves the row unknown, never
// met. Break: pass over a run with no runner key, and the red run is missed.
func TestPLW1o_ATestRunWithNoRunnerKeyMightHaveCovered(t *testing.T) {
	noKey := call{seq: 3, tool: "Bash", prog: "go", class: shape.VerbTest, outcome: store.ExecFailed, exit: 1, loc: "d:repo"}
	r := runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK), noKey)
	ranIn(r, "/repo", "")
	named(t, r, 2, "go test ./...")
	rows := platformRows(plwStop(inPrompt(r, "p1"), nil, ciGoTest, pkgFamily, nil))
	if len(rows) != 1 || rows[0].State != ObligationUnknown {
		t.Errorf("rows %+v, want pkg's row unknown", rows)
	}
}
