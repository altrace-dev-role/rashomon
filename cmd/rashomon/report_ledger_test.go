package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/digest"
	"github.com/altrace-dev-role/rashomon/internal/hook"
	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// lwFixture is a store at RASHOMON_HOME whose sessions are recorded through
// the hooks, as TestPG6e records one, under record scope v5 so a named
// write's post hashes its file, all run in one work tree with a .git at its
// top, which is each session's project root and its project index key, and
// the root a repository spec under it is discovered up to (spec.WalkRoot:
// with no .git and no CLAUDE_PROJECT_DIR nothing is discovered).
type lwFixture struct {
	t   *testing.T
	st  *store.Store
	cwd string
	now time.Time
}

func newLW(t *testing.T) *lwFixture {
	t.Helper()
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	t.Setenv("CLAUDE_PROJECT_DIR", "")
	t.Setenv("RASHOMON_MANAGED_SETTINGS_PATH", filepath.Join(t.TempDir(), "managed-settings.json"))
	cwd, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(cwd, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("RASHOMON_HOME", st.Root())
	if err := store.WriteRecordScopeMarker(st.Root(), store.RecordScopeV5); err != nil {
		t.Fatal(err)
	}
	f := &lwFixture{t: t, st: st, cwd: cwd, now: time.UnixMilli(1_800_000_000_000)}
	f.cover("s")
	return f
}

// cover appends session's coverage record, verified, as a hook whose entry
// and probe both resolve writes one: without it the recording is incomplete.
func (f *lwFixture) cover(session string) {
	f.t.Helper()
	if err := f.st.AppendCoverage(store.Coverage{Type: store.TypeCoverage, SchemaVersion: store.SessionSchemaVersion,
		RecordedAtMS: f.now.UnixMilli(), SessionID: session, Phase: store.PhasePost, State: store.StateVerified,
		HookEntry: store.EntryPresentSettings, Probe: store.ProbeFresh}); err != nil {
		f.t.Fatal(err)
	}
}

// file writes body to rel under the work tree, its directories made, and
// returns its path.
func (f *lwFixture) file(rel, body string) string {
	f.t.Helper()
	path := filepath.Join(f.cwd, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		f.t.Fatal(err)
	}
	return path
}

// land records one call of session "s"; landIn records it in session.
func (f *lwFixture) land(id, tool string, pre, post map[string]any) {
	f.t.Helper()
	f.landIn("s", id, tool, pre, post)
}

// landIn records one call: its PreToolUse with pre as the tool input and its
// PostToolUse with post, or pre again when post is nil (another PreToolUse
// hook may rewrite a command between the two).
func (f *lwFixture) landIn(session, id, tool string, pre, post map[string]any) {
	f.t.Helper()
	if post == nil {
		post = pre
	}
	for _, event := range []string{"PreToolUse", "PostToolUse"} {
		input := pre
		if event == "PostToolUse" {
			input = post
		}
		b, err := json.Marshal(map[string]any{"session_id": session, "tool_use_id": id, "prompt_id": "p1",
			"transcript_path": "/tmp/t.jsonl", "permission_mode": "default", "tool_name": tool, "cwd": f.cwd,
			"hook_event_name": event, "tool_input": input, "tool_response": map[string]any{"stdout": ""}})
		if err != nil {
			f.t.Fatal(err)
		}
		clock := func() time.Time { return f.now }
		if event == "PreToolUse" {
			h := hook.New(f.st, clock)
			h.Close(&hook.Signals{}, h.Capture(strings.NewReader(string(b))))
		} else if err := hook.NewPost(f.st, clock).Capture(strings.NewReader(string(b))); err != nil {
			f.t.Fatalf("post %s: %v", id, err)
		}
		f.now = f.now.Add(time.Minute)
	}
}

func lwEdit(path string) map[string]any {
	return map[string]any{"file_path": path, "old_string": "a", "new_string": "a"}
}

func lwBash(command string) map[string]any { return map[string]any{"command": command} }

// stop is the Stop hook's view of session "s"; stopIn of session. The Stop
// appends its checkpoint and writes the project index it observed.
func (f *lwFixture) stop() verify.StopView {
	f.t.Helper()
	return f.stopIn("s")
}

func (f *lwFixture) stopIn(session string) verify.StopView {
	f.t.Helper()
	run, err := digest.ReadRun(f.st, session)
	if err != nil {
		f.t.Fatal(err)
	}
	d, err := digest.BuildFrom(run, f.st, session, "", "", f.now)
	if err != nil {
		f.t.Fatal(err)
	}
	p := recapPayload{SessionID: session, Harness: harnessSignal{HookEventName: install.EventStop}, OK: true, CWD: f.cwd}
	turn, err := stopView(f.st, run, d, p, false, f.now)
	if err != nil {
		f.t.Fatal(err)
	}
	f.now = f.now.Add(time.Minute)
	return turn.view
}

// ledger runs `rashomon report --ledger` with args after it.
func (f *lwFixture) ledger(args ...string) (stdout, stderr string, code int) {
	var o, e bytes.Buffer
	code = run(append([]string{"report", "--ledger"}, args...), strings.NewReader(""), &o, &e)
	return o.String(), e.String(), code
}

// lwOut is what the verb printed for session "s": its header's "name: value"
// lines by name, and the rest, the rows or the no-row line. Tests assert on
// these, never on the exit alone; stderr is kept for their messages.
type lwOut struct {
	all, rows, stderr string
	head              map[string]string
}

func (f *lwFixture) out() lwOut {
	f.t.Helper()
	all, stderr, _ := f.ledger("--session", "s")
	o := lwOut{all: all, stderr: stderr, head: map[string]string{}}
	for _, line := range strings.SplitAfter(all, "\n") {
		name, value, ok := strings.Cut(strings.TrimSuffix(line, "\n"), ": ")
		if ok && !strings.HasPrefix(line, "ob:") && !strings.HasPrefix(line, "no obligation rows") {
			o.head[name] = value
			continue
		}
		o.rows += line
	}
	return o
}

// lwRows is v's ledger as the verb prints its rows: id, scope, kind, source,
// state, the evidence seqs joined by commas ("-" for none) and the family the
// id digests, recomputed here over v's gates (Family, or Gate for a declared
// row) and expected kinds, one space apart, one row to a line.
func lwRows(v verify.StopView) string {
	var b strings.Builder
	for _, o := range v.Obligations {
		ev := "-"
		if len(o.Evidence) > 0 {
			seqs := make([]string, len(o.Evidence))
			for i, s := range o.Evidence {
				seqs[i] = strconv.FormatInt(s, 10)
			}
			ev = strings.Join(seqs, ",")
		}
		family := "-"
		for _, g := range v.Snap.Gates {
			for _, fam := range []string{g.Family, g.Gate} {
				if verify.ObligationID(o.Scope, o.Kind, fam) == o.ID {
					family = fam
				}
			}
		}
		for _, e := range v.Expected {
			if verify.ObligationID(o.Scope, o.Kind, e.Kind) == o.ID {
				family = e.Kind
			}
		}
		fmt.Fprintf(&b, "%s %s %s %s %s %s %s\n", o.ID, o.Scope, o.Kind, o.Source, o.State, ev, family)
	}
	return b.String()
}

// lwSpec is a repository spec declaring one test gate named id.
func lwSpec(id string) string {
	return `{"rashomon_spec": 1, "gates": [{"id": "` + id + `", "kind": "test", "argv": ["go", "test", "./..."]}]}`
}

const lwSpecPath = ".claude/rashomon/spec.json"

// bind writes doc as the work tree's repository spec and binds it in the
// store's bindings file, as `rashomon spec trust` does, so discovery under
// the store reads it trusted and its gates are declared (SPEC-TRUST1,
// DECLARED-GATES1).
func (f *lwFixture) bind(doc string) {
	f.t.Helper()
	path := f.file(lwSpecPath, doc)
	b := spec.Bindings{Bindings: []spec.Binding{{ProjectRoot: hook.SpecProject(path), SpecPath: path,
		SpecDigest: digestOf(f.t, doc), BoundAtMS: 1}}}
	if err := hook.WriteSpecBindings(f.st.Root(), &b); err != nil {
		f.t.Fatal(err)
	}
}

// lwDeclared is session "s" judged at its Stop against a trusted spec that
// declares spec:unit over go test, run after an edit: the fixture, and the
// ids of the rows the Stop's ledger holds for spec:unit, each digesting the
// gate's id.
func lwDeclared(t *testing.T) (*lwFixture, map[string]bool) {
	t.Helper()
	f := newLW(t)
	f.bind(lwSpec("unit"))
	f.land("toolu_e", "Edit", lwEdit(f.file("a.go", "package a\n")), nil)
	f.land("toolu_g", "Bash", lwBash("go test ./..."), nil)
	v := f.stop()
	ids := map[string]bool{}
	for _, o := range v.Obligations {
		if o.ID == verify.ObligationID(o.Scope, o.Kind, "spec:unit") {
			ids[o.ID] = true
		}
	}
	if len(ids) == 0 {
		t.Fatalf("premise: the Stop's ledger holds spec:unit's rows; gates %+v, rows %+v", v.Snap.Gates, v.Obligations)
	}
	return f, ids
}

// TestLW1c_ReportLedgerIsByteStableAcrossTwoDerivations: the verb derives the
// ledger from the record, so two derivations of one record print the same
// bytes, among them a green run after a write's gate_after_write row, met. It
// claims nothing about the Stop's own rows (TestLW1e does).
func TestLW1c_ReportLedgerIsByteStableAcrossTwoDerivations(t *testing.T) {
	f := newLW(t)
	f.land("toolu_e", "Edit", lwEdit(f.file("a.go", "package a\n")), nil)
	f.land("toolu_g", "Bash", lwBash("go test ./..."), nil)
	f.stop()
	first, second := f.out(), f.out()
	if first.all != second.all {
		t.Errorf("two derivations differ:\n%s\n----\n%s", first.all, second.all)
	}
	if !strings.Contains(first.rows, " session gate_after_write learned met ") {
		t.Errorf("no gate_after_write row met in %q (stderr %q)", first.all, first.stderr)
	}
}

// TestLW1d_ReportLedgerNamesASpecDigestMismatch: the verb judges against the
// spec discovered now; when that is not the spec the Stop judged against, its
// spec line names both ("none" for a side with no spec), and the rows are
// printed anyway.
func TestLW1d_ReportLedgerNamesASpecDigestMismatch(t *testing.T) {
	for _, c := range []struct {
		name   string
		before string // the spec at Stop, "" for none
		after  string
	}{
		{"the spec changed", lwSpec("unit"), lwSpec("unit2")},
		{"a spec appeared", "", lwSpec("unit")},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newLW(t)
			if c.before != "" {
				f.file(lwSpecPath, c.before)
			}
			f.land("toolu_e", "Edit", lwEdit(f.file("a.go", "package a\n")), nil)
			f.land("toolu_g", "Bash", lwBash("go test ./..."), nil)
			v := f.stop()
			atStop := "none"
			if v.Snap.Spec != nil {
				atStop = v.Snap.Spec.Digest
			}
			f.file(lwSpecPath, c.after)
			pd, md := specDirs()
			sp := hook.DiscoverSpec(f.cwd, pd, md, hook.NoRun(f.st.Root()))
			if sp == nil {
				t.Fatal("no spec discovered now")
			}
			o := f.out()
			line := o.head["spec"]
			if !strings.HasPrefix(line, sp.Digest) || !strings.Contains(line, "the Stop judged against "+atStop) {
				t.Errorf("spec line %q, want one naming %s now and %s at the Stop (stderr %q)", line, sp.Digest, atStop, o.stderr)
			}
			if !strings.Contains(o.rows, " gate_after_write ") {
				t.Errorf("no rows beside the mismatch: %q", o.all)
			}
		})
	}
}

// TestLW1e_StopLedgerEqualsTheVerbs: the verb prints the rows the Stop view
// built, row for row, after its header. Two earlier sessions ran go vet, so
// at this session's Stop the project index has learned that family alone and
// the session's go test gate is advisory, owing no row. Between the Stop and
// the verb a further session runs go test and brings its family to
// RequiredSessions; the verb keeps the Required the checkpoint recorded, so
// neither the index on disk nor the no-index prior (every gate required)
// adds the go test rows. The session also holds a command another hook
// rewrote between the two events, which is no gate (a rewritten run has no
// location, so no family), however digest's MaskingAsRan rewrites the run.
// Then an Edit of a new file lands past the checkpoint: the verb evaluates at
// the checkpoint's point, where that record and the path the tree now names
// change nothing.
func TestLW1e_StopLedgerEqualsTheVerbs(t *testing.T) {
	f := newLW(t)
	for _, s := range []string{"p1", "p2"} {
		f.cover(s)
		f.landIn(s, "toolu_"+s, "Bash", lwBash("go vet ./..."), nil)
		f.stopIn(s)
	}
	f.land("toolu_e", "Edit", lwEdit(f.file("a.go", "package a\n")), nil)
	f.land("toolu_g", "Bash", lwBash("go test ./..."), nil)
	f.land("toolu_v", "Bash", lwBash("go vet ./..."), nil)
	f.land("toolu_r", "Bash", lwBash("go test ./..."), lwBash("go test ./... 2>&1 | tail -5"))
	v := f.stop()
	want := lwRows(v)
	var advisory *verify.GateState
	for i, g := range v.Snap.Gates {
		if !g.Required {
			advisory = &v.Snap.Gates[i]
		}
	}
	if want == "" || advisory == nil {
		t.Fatalf("premise: rows from the required gate and one advisory gate; gates %+v", v.Snap.Gates)
	}

	f.cover("s2")
	f.landIn("s2", "toolu_g2", "Bash", lwBash("go test ./..."), nil)
	f.stopIn("s2")
	idx, err := verify.ReadIndex(verify.IndexPath(f.st.Root(), baseline.Key(f.cwd)))
	if err != nil {
		t.Fatal(err)
	}
	if !idx.Required(advisory.Family, f.now.UnixMilli()) {
		t.Fatalf("premise: the index now requires %s, advisory at the Stop", advisory.Family)
	}

	if o := f.out(); o.rows != want || o.head["point"] == "" {
		t.Errorf("the verb printed %q (stderr %q)\nthe Stop built:\n%s", o.all, o.stderr, want)
	}
	f.land("toolu_b", "Edit", lwEdit(f.file("b.go", "package a\n")), nil)
	if o := f.out(); o.rows != want {
		t.Errorf("after a record past the checkpoint the verb printed %q\nthe Stop built:\n%s", o.all, want)
	}
}

// TestLW1h_ReportLedgerRefusals: the ledger is one session's, so --ledger
// needs --session, and a session with no records has none; and it prints
// alone, so beside --json, --redact, --chain, --timeline or --nono-audit it is
// refused. A refusal prints nothing on stdout.
func TestLW1h_ReportLedgerRefusals(t *testing.T) {
	f := newLW(t)
	for _, c := range []struct {
		args []string
		want string
	}{
		{nil, "--ledger needs --session"},
		{[]string{"--session", "s", "--json"}, "--ledger prints alone: drop --json"},
		{[]string{"--session", "s", "--redact"}, "--ledger prints alone: drop --redact"},
		{[]string{"--session", "s", "--chain"}, "--ledger prints alone: drop --chain"},
		{[]string{"--session", "s", "--timeline"}, "--ledger prints alone: drop --timeline"},
		{[]string{"--session", "s", "--nono-audit", "trail"}, "--ledger prints alone: drop --nono-audit"},
		{[]string{"--session", "nosuch"}, `session "nosuch" has no records here`},
	} {
		out, errText, code := f.ledger(c.args...)
		if code != exitFail || out != "" || !strings.Contains(errText, c.want) {
			t.Errorf("report --ledger %v: exit %d, stdout %q, stderr %q; want exit %d, nothing printed, %q",
				c.args, code, out, errText, exitFail, c.want)
		}
	}
}

// TestLW1i_ReportLedgerWithoutACheckpointOrRows: with no checkpoint the verb
// evaluates at the record's last seq and its point line says the Stop-time
// spec is unknown; a ledger with no rows prints one line naming the point.
func TestLW1i_ReportLedgerWithoutACheckpointOrRows(t *testing.T) {
	f := newLW(t)
	f.land("toolu_e", "Edit", lwEdit(f.file("a.go", "package a\n")), nil)
	f.land("toolu_g", "Bash", lwBash("go test ./..."), nil)
	o := f.out()
	if !strings.Contains(o.head["point"], "the record's last; no checkpoint, so the Stop-time spec is unknown") ||
		!strings.Contains(o.rows, " gate_after_write ") {
		t.Errorf("without a checkpoint: %q (stderr %q)", o.all, o.stderr)
	}

	g := newLW(t)
	g.land("toolu_e", "Edit", lwEdit(g.file("a.go", "package a\n")), nil)
	v := g.stop()
	if len(v.Obligations) != 0 {
		t.Fatalf("an Edit alone owes %+v, want nothing", v.Obligations)
	}
	if o, want := g.out(), fmt.Sprintf("no obligation rows at seq %d\n", v.Snap.Point.Seq); o.rows != want {
		t.Errorf("an empty ledger printed %q, want rows %q", o.all, want)
	}
}

// TestLW1j_ReportLedgerSaysTheMessageIsNotHeld: the record keeps no final
// message and the task file is outside spec_digest, and the header says both;
// a task naming judgment_disclosed has that row read unknown, never unmet,
// with the kind as its family.
func TestLW1j_ReportLedgerSaysTheMessageIsNotHeld(t *testing.T) {
	f := newLW(t)
	f.file(".claude/rashomon/task/current.json", `{"id": "t-1", "goal": "Ship the nightly export", `+
		`"decision": "Whether to cut the release", "expected_evidence": ["judgment_disclosed"]}`)
	f.land("toolu_e", "Edit", lwEdit(f.file("a.go", "package a\n")), nil)
	f.stop()
	o := f.out()
	if o.head["final message"] != "not in the record, so judgment_disclosed reads unknown" ||
		o.head["task"] != "outside spec_digest, so a change since the Stop is not named" ||
		o.head["tree"] != "read now, not at the Stop" {
		t.Errorf("header %v (stderr %q)", o.head, o.stderr)
	}
	id := verify.ObligationID(verify.ScopeSession, verify.ExpectedEvidence, "judgment_disclosed")
	if want := id + " session expected_evidence task unknown - judgment_disclosed\n"; o.rows != want {
		t.Errorf("rows %q, want %q", o.rows, want)
	}
}

// TestLW1m_ReportLedgerNamesGatesTheCutOmitted: a gate the checkpoint's cut
// omitted has no recorded Required, keeps the fail-closed prior, required,
// and the header counts the omitted gates.
func TestLW1m_ReportLedgerNamesGatesTheCutOmitted(t *testing.T) {
	f := newLW(t)
	f.land("toolu_e", "Edit", lwEdit(f.file("a.go", "package a\n")), nil)
	f.land("toolu_g", "Bash", lwBash("go test ./..."), nil)
	v := f.stop()
	cp := verify.Cut(v.Snap, f.now)
	cp.Gates, cp.GatesOmitted, cp.Truncated = []verify.CheckpointGate{}, len(v.Snap.Gates), true
	if err := f.st.AppendRecord("s", &cp); err != nil {
		t.Fatal(err)
	}
	o := f.out()
	if want := fmt.Sprintf("%d cut from the checkpoint read required", len(v.Snap.Gates)); o.head["gates"] != want {
		t.Errorf("gates line %q, want %q (stderr %q)", o.head["gates"], want, o.stderr)
	}
	if !strings.Contains(o.rows, " session gate_after_write learned met ") {
		t.Errorf("the omitted green gate owes no row: %q", o.all)
	}
}

// TestLW1n_ReportLedgerKeepsATrustedSpecsDeclaredRows: the replay discovers
// the spec trusted by the store's bindings, as the Stop does, so the rows a
// trusted spec's declared gate owed at the Stop are in the verb's; discovered
// with no store root the spec reads untrusted, declares nothing, and the
// rows are gone.
func TestLW1n_ReportLedgerKeepsATrustedSpecsDeclaredRows(t *testing.T) {
	f, ids := lwDeclared(t)
	o := f.out()
	for id := range ids {
		if !strings.Contains(o.rows, id+" ") {
			t.Errorf("the Stop's row %s for spec:unit is not among the verb's: %q (stderr %q)", id, o.all, o.stderr)
		}
	}
}

// TestLW1o_ReportLedgerNamesADeclaredRowsGate: every declared gate's Family
// is "declared", so a declared row's id digests its gate id, and the family
// column names spec:unit on each of its rows, never "-".
func TestLW1o_ReportLedgerNamesADeclaredRowsGate(t *testing.T) {
	f, ids := lwDeclared(t)
	o := f.out()
	n := 0
	for _, line := range strings.SplitAfter(o.rows, "\n") {
		if id, _, _ := strings.Cut(line, " "); ids[id] {
			n++
			if !strings.HasSuffix(line, " spec:unit\n") {
				t.Errorf("declared row %q, want its family column spec:unit", line)
			}
		}
	}
	if n != len(ids) {
		t.Errorf("%d of spec:unit's %d rows printed: %q (stderr %q)", n, len(ids), o.all, o.stderr)
	}
}

// TestLW1p_ReportLedgerNamesASpecRulesRule: a spec_rule row digests its
// rule's id, so the family column names the rule on that row, never "-"
// (the way a declared row names its gate). A trusted spec with one rule, a
// call that matches it at the Stop. Break: leave SpecRule out of
// ledgerFamilies and the column reads "-".
func TestLW1p_ReportLedgerNamesASpecRulesRule(t *testing.T) {
	f := newLW(t)
	f.bind(`{"rashomon_spec": 1, "rules": [{"id": "no-force", "kind": "forbid_command", "command": {"argv": ["git", "push"], "flags": ["--force"]}, "why": "w", "hard_control": "h"}]}`)
	f.land("toolu_e", "Edit", lwEdit(f.file("a.go", "package a\n")), nil)
	f.land("toolu_g", "Bash", lwBash("go test ./..."), nil)
	v := f.stop()
	ids := map[string]string{}
	for _, o := range v.Obligations {
		if o.Kind == verify.SpecRule {
			for _, sv := range v.Snap.SpecVerdicts {
				if o.ID == verify.ObligationID(o.Scope, o.Kind, string(sv.Rule)) {
					ids[o.ID] = string(sv.Rule)
				}
			}
		}
	}
	if len(ids) == 0 {
		t.Fatalf("premise: the Stop's ledger holds a spec_rule row; rows %+v, verdicts %+v", v.Obligations, v.Snap.SpecVerdicts)
	}
	o := f.out()
	n := 0
	for _, line := range strings.SplitAfter(o.rows, "\n") {
		if id, _, _ := strings.Cut(line, " "); ids[id] != "" {
			n++
			if !strings.HasSuffix(line, " "+ids[id]+"\n") {
				t.Errorf("spec_rule row %q, want its family column %s", line, ids[id])
			}
		}
	}
	if n != len(ids) {
		t.Errorf("%d of %d spec_rule rows printed: %q (stderr %q)", n, len(ids), o.all, o.stderr)
	}
}
