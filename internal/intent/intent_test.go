package intent_test

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/intent"
	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

const nowMS = 1_800_000_000_000

// projectTree is a small repository: a Go module with CLAUDE.md, two files
// under internal/verify, a CI workflow and test data.
func projectTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, f := range []string{
		"go.mod", "CLAUDE.md", "internal/verify/stop.go", "internal/verify/index.go",
		".github/workflows/ci.yaml", "internal/testdata/x.json",
	} {
		p := filepath.Join(root, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// specOf is a compiled spec of one source at trust: a required test gate, a
// required lint gate, a rule on internal/** (test data excluded), a rule on
// the CI workflows and a require_evidence rule with no paths.
func specOf(root string, trust spec.Trust) *spec.Compiled {
	required := true
	return &spec.Compiled{
		Sources: []spec.Source{{Path: filepath.Join(root, ".claude", "rashomon", "spec.json"), Digest: "sha256:00", Trust: trust}},
		Trust:   trust,
		Gates: []spec.Declared{
			{ID: "spec:unit", Gate: spec.Gate{ID: "unit", Kind: "test", Argv: []string{"go", "test", "./..."}, Required: &required}},
			{ID: "spec:lint", Gate: spec.Gate{ID: "lint", Kind: "lint", Argv: []string{"golangci-lint", "run"}, Required: &required}},
		},
		Rules: []spec.DeclaredRule{
			{ID: "spec:conv-errors", Rule: spec.Rule{ID: "conv-errors", Kind: "forbid_code", Why: "errors are wrapped with the package name",
				Paths: []string{"internal/**"}, ExcludePaths: []string{"internal/testdata/**"}}},
			{ID: "spec:ci-frozen", Rule: spec.Rule{ID: "ci-frozen", Kind: "protect_path", Why: "CI files belong to the platform team",
				Paths: []string{".github/workflows/**"}}},
			{ID: "spec:proof", Rule: spec.Rule{ID: "proof", Kind: "require_evidence", Why: "a done claim carries a test run"}},
		},
	}
}

// indexOf holds one required test family, one required lint family and one
// test family seen in one session only, in Family order.
func indexOf() *verify.ProjectIndex {
	return &verify.ProjectIndex{Entries: []verify.IndexEntry{
		{Family: "go-test@.", Kind: "test", Sessions: 3, LastSeen: nowMS - 1000},
		{Family: "golangci@.", Kind: "lint", Sessions: 3, LastSeen: nowMS},
		{Family: "pytest@x", Kind: "test", Sessions: 1, LastSeen: nowMS},
	}}
}

func projectOf(root string, sp *spec.Compiled, idx *verify.ProjectIndex) intent.Project {
	return intent.Project{Root: root, Spec: sp, Index: idx, NowMS: nowMS}
}

// givenBrief is a delivered set whose brief was already given, so the note
// takes the later prompts' row.
func givenBrief() intent.Delivered {
	return intent.Delivered{BriefGiven: true}
}

// noteOf resolves prompt and returns the finding's text, failing when none.
func noteOf(t *testing.T, prompt string, p intent.Project, d intent.Delivered, b *intent.Brief) string {
	t.Helper()
	f, ok := intent.Finding(intent.Resolve([]byte(prompt), false, p), d, b, 1)
	if !ok {
		t.Fatalf("no finding for %q", prompt)
	}
	if err := f.Check(); err != nil {
		t.Fatalf("finding fails its contract: %v", err)
	}
	return f.Message.Text()
}

// TestPF1_RequestNamingAPathGetsItsRulesAndGate: a request naming a file
// gets the spec rules whose paths cover it plus every require_evidence rule,
// the trusted spec's required test gate with the count of test families
// earlier sessions made required, and the fix class's expected evidence;
// nothing the project does not hold is named.
func TestPF1_RequestNamingAPathGetsItsRulesAndGate(t *testing.T) {
	root := projectTree(t)
	p := projectOf(root, specOf(root, spec.Trusted), indexOf())
	in := intent.Resolve([]byte("fix the nil check in internal/verify/stop.go before lunch"), false, p)
	if in.Verb != intent.VerbFix || !slices.Equal(in.Paths, []string{"internal/verify/stop.go"}) {
		t.Errorf("Resolve = verb %q, paths %q; want fix, the one named path", in.Verb, in.Paths)
	}
	got := noteOf(t, "fix the nil check in internal/verify/stop.go before lunch", p, givenBrief(), nil)
	want := "rashomon record: preflight for this request from the repository; the request text is not kept.\n" +
		"paths named: 'internal/verify/stop.go'\n" +
		"spec rules that apply: 'conv-errors' (trusted) 'errors are wrapped with the package name'; 'proof' (trusted) 'a done claim carries a test run'\n" +
		"gates before done: 'go test' (trusted); 1 learned from earlier sessions; evidence for a done claim (inferred from the request): judgment_disclosed\n" +
		"unknown: none"
	if got != want {
		t.Errorf("note =\n%s\nwant\n%s", got, want)
	}
}

// lineIDs numbers the steer lines this file's routes write, so no two share
// an id across calls.
var lineIDs int

// routeAt routes f as prompt promptID of session s1 on st and returns the
// Output; holdout draws the session into the field holdout.
func routeAt(t *testing.T, st *store.Store, promptID string, holdout bool, f steer.Finding) steer.Output {
	t.Helper()
	ctx := &steer.Context{
		Event: steer.EventUserPromptSubmit, Store: st, SessionID: "s1", PromptID: &promptID, Project: "/repo",
		HarnessID: "claude_code", Surface: "cli", NowUnixMS: nowMS, Consent: true, Arm: steer.ArmLive,
		Attendance: steer.AttendanceAttended, Holdout: holdout,
		NewID: func() string { lineIDs++; return fmt.Sprintf("%032d", lineIDs) },
	}
	return steer.Route(ctx, []steer.Finding{f})
}

func delivered(t *testing.T, st *store.Store) intent.Delivered {
	t.Helper()
	lines, skipped, err := st.ReadSteer("s1")
	if err != nil || skipped != 0 {
		t.Fatalf("ReadSteer: %v, %d skipped", err, skipped)
	}
	return intent.ReadDelivered(lines)
}

func mustFinding(t *testing.T, prompt string, p intent.Project, d intent.Delivered) steer.Finding {
	t.Helper()
	f, ok := intent.Finding(intent.Resolve([]byte(prompt), false, p), d, nil, 1)
	if !ok {
		t.Fatalf("no finding for %q", prompt)
	}
	return f
}

// TestPF1b_SecondPromptGetsOnlyTheNew: the session's steer log is the
// delivered set. A second request naming the delivered file and a new one
// gets the new path and its rule by name and the delivered ones as a count;
// a request naming nothing new prints nothing.
func TestPF1b_SecondPromptGetsOnlyTheNew(t *testing.T) {
	root := projectTree(t)
	p := projectOf(root, specOf(root, spec.Trusted), indexOf())
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	out := routeAt(t, st, "p1", false, mustFinding(t, "fix the nil check in internal/verify/stop.go", p, intent.Delivered{}))
	if !strings.Contains(out.AdditionalContext, "'internal/verify/stop.go'") {
		t.Fatalf("premise: the first note was not delivered: %+v", out)
	}
	d := delivered(t, st)
	if !d.Paths["internal/verify/stop.go"] || !d.Rules["conv-errors"] || !d.Gates["go test"] || !d.Evidence["judgment_disclosed"] {
		t.Fatalf("delivered set = %+v, want the first note's path, rule, gate and evidence", d)
	}

	f := mustFinding(t, "fix the same in internal/verify/stop.go and .github/workflows/ci.yaml", p, d)
	text := f.Message.Text()
	for _, want := range []string{
		"paths named: '.github/workflows/ci.yaml'; 1 given earlier\n",
		"spec rules that apply: 'ci-frozen' (trusted) 'CI files belong to the platform team'; 2 given earlier\n",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("second note lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "'conv-errors'") || strings.Contains(text, "'internal/verify/stop.go'") {
		t.Errorf("second note repeats a delivered item by name:\n%s", text)
	}
	if out := routeAt(t, st, "p2", false, f); out.AdditionalContext != text {
		t.Errorf("second note not delivered: %+v", out)
	}
	if _, ok := intent.Finding(intent.Resolve([]byte("fix it in internal/verify/stop.go"), false, p), delivered(t, st), nil, 1); ok {
		t.Error("a request naming only delivered items got a note")
	}
}

// TestPF1b_HeldOutDeliveryCountsAsGiven: a note the holdout kept from the
// model is still a delivered line (state delivered, delivery holdout), so
// the session is not offered the same items on every prompt.
func TestPF1b_HeldOutDeliveryCountsAsGiven(t *testing.T) {
	root := projectTree(t)
	p := projectOf(root, specOf(root, spec.Trusted), indexOf())
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	out := routeAt(t, st, "p1", true, mustFinding(t, "fix internal/verify/stop.go", p, intent.Delivered{}))
	if out.AdditionalContext != "" || out.SystemMessage == "" {
		t.Fatalf("premise: a held-out note goes to the user only: %+v", out)
	}
	if _, ok := intent.Finding(intent.Resolve([]byte("fix internal/verify/stop.go"), false, p), delivered(t, st), nil, 1); ok {
		t.Error("the held-out session was offered the same items again")
	}
}

// TestPF1c_AmbiguousPromptGetsNothingNew: a request that names no path and
// no identifier gets no note once the brief is given, whatever its verb.
func TestPF1c_AmbiguousPromptGetsNothingNew(t *testing.T) {
	root := projectTree(t)
	p := projectOf(root, specOf(root, spec.Trusted), indexOf())
	if _, ok := intent.Finding(intent.Resolve([]byte("fix internal/verify/stop.go"), false, p), givenBrief(), nil, 1); !ok {
		t.Fatal("premise: a request naming a file gets a note")
	}
	for _, prompt := range []string{"continue", "fix it", "ok, go ahead", "do the same for the other one", "", "deploy"} {
		if f, ok := intent.Finding(intent.Resolve([]byte(prompt), false, p), givenBrief(), nil, 1); ok {
			t.Errorf("%q got a note:\n%s", prompt, f.Message.Text())
		}
	}
}

// TestPF1c_NamesIsTheCatchUpsCheapGuard: Names answers from one scan, with
// no lookup, whether a request names a path or an identifier, found or not,
// as Resolve's Named does. The catch-up asks it before it reads the spec, the
// index or the steer log, so a request naming nothing costs nothing more.
// Break: Names false for a named request, and its preflight never runs.
func TestPF1c_NamesIsTheCatchUpsCheapGuard(t *testing.T) {
	root := projectTree(t)
	p := projectOf(root, specOf(root, spec.Trusted), indexOf())
	for prompt, want := range map[string]bool{
		"fix internal/verify/stop.go": true, "rename QuokkaMarmalade": true, "look at quokka/marmalade.go": true,
		"see record.prompt_class": true, "continue": false, "fix it": false, "": false, "deploy": false,
	} {
		got, named := intent.Names([]byte(prompt), false), intent.Resolve([]byte(prompt), false, p).Named
		if got != want || named != want {
			t.Errorf("Names(%q) = %v, Resolve's Named %v; want %v", prompt, got, named, want)
		}
	}
}

// TestPF1d_IntentKeepsNoPromptWords: nothing Resolve returns, and nothing
// the note or its slots hold, carries a word of the request that is not a
// path the project holds.
func TestPF1d_IntentKeepsNoPromptWords(t *testing.T) {
	root := projectTree(t)
	p := projectOf(root, specOf(root, spec.Trusted), indexOf())
	prompt := "fix zebracornflake in internal/verify/stop.go and quokka/marmalade.go, see QuokkaMarmalade"
	in := intent.Resolve([]byte(prompt), false, p)
	f, ok := intent.Finding(in, intent.Delivered{}, nil, 1)
	if !ok {
		t.Fatal("premise: the request names a file")
	}
	m := f.Message
	all := fmt.Sprintf("%#v %s %#v", in, m.Text(), m.Slots())
	for _, w := range []string{"zebracornflake", "quokka", "Quokka", "marmalade"} {
		if strings.Contains(all, w) {
			t.Errorf("%q survives the request:\n%s", w, all)
		}
	}
	if !strings.Contains(m.Text(), "'internal/verify/stop.go'") {
		t.Errorf("the named file is missing:\n%s", m.Text())
	}
}

// TestPF1e_UntrustedSpecIsNamedNotQuoted: an untrusted spec's rules and
// gates never reach the note as text (INV-2): the rule line counts them and
// the unknown line says the spec is present and untrusted.
func TestPF1e_UntrustedSpecIsNamedNotQuoted(t *testing.T) {
	root := projectTree(t)
	text := noteOf(t, "fix internal/verify/stop.go", projectOf(root, specOf(root, spec.Untrusted), indexOf()), givenBrief(), nil)
	for _, leak := range []string{"conv-errors", "wrapped", "proof", "go test", "golangci"} {
		if strings.Contains(text, leak) {
			t.Errorf("untrusted spec text %q in the note:\n%s", leak, text)
		}
	}
	for _, want := range []string{"spec rules that apply: 2 not shown, spec untrusted\n", "spec present, untrusted"} {
		if !strings.Contains(text, want) {
			t.Errorf("note lacks %q:\n%s", want, text)
		}
	}
}

// TestPF1f_NothingInventedWithoutASource: with no spec, no index and no such
// file, every line is in its unknown form and only the class table, which
// the line names as inferred, contributes.
func TestPF1f_NothingInventedWithoutASource(t *testing.T) {
	got := noteOf(t, "fix the parser in internal/parse/lexer.go", projectOf(t.TempDir(), nil, nil), givenBrief(), nil)
	want := "rashomon record: preflight for this request from the repository; the request text is not kept.\n" +
		"paths named: none\n" +
		"spec rules that apply: none\n" +
		"gates before done: none; evidence for a done claim (inferred from the request): judgment_disclosed\n" +
		"unknown: no spec found; no gate history for this project; no named path exists in the project"
	if got != want {
		t.Errorf("note =\n%s\nwant\n%s", got, want)
	}
}

// longID is the i-th long rule's id, 48 bytes, the spec's longest.
func longID(i int) string { return fmt.Sprintf("r%02d-", i) + strings.Repeat("x", 44) }

// longSpec is a trusted spec whose first twenty rules are on internal/**,
// each with a 48-byte id and a 160-byte why, before specOf's own.
func longSpec(root string) *spec.Compiled {
	sp := specOf(root, spec.Trusted)
	var rules []spec.DeclaredRule
	for i := range 20 {
		why := strings.Repeat("word ", 31) + "w" + strconv.Itoa(i%10) + "rds"
		rules = append(rules, spec.DeclaredRule{ID: "spec:" + longID(i),
			Rule: spec.Rule{ID: longID(i), Kind: "forbid_code", Why: why[:160], Paths: []string{"internal/**"}}})
	}
	sp.Rules = append(rules, sp.Rules...)
	return sp
}

// TestPF1g_NoteFitsTheBriefingCap: a request naming eight long paths under
// a spec with twenty long rules still gets a note, at most 1,600 bytes and
// six lines, that the router delivers with its mirror under the steer
// log's cap. The note steps down to fit: whys go first, the three rules
// still named; with long project files as well, one item per list. Break:
// drop the step-down, and a long note is refused and the prompt gets none.
func TestPF1g_NoteFitsTheBriefingCap(t *testing.T) {
	root := projectTree(t)
	var names []string
	for i := range 8 {
		rel := "internal/" + strings.Repeat("d", 120) + "/" + strings.Repeat(string(rune('a'+i)), 120) + ".go"
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		names = append(names, rel)
	}
	var instructions []string
	for i := range 3 {
		instructions = append(instructions, "docs/"+strings.Repeat("i", 120)+"/"+strings.Repeat(string(rune('a'+i)), 120)+".md")
	}
	p := projectOf(root, longSpec(root), indexOf())
	for _, c := range []struct {
		name string
		b    *intent.Brief
		want string
	}{
		{"whys dropped", &intent.Brief{Instructions: []string{"CLAUDE.md"}, Manifests: []string{"go.mod"}, Module: "example.com/m", GoDirs: 2},
			"spec rules that apply: '" + longID(0) + "' (trusted); '" + longID(1) + "' (trusted); '" + longID(2) + "' (trusted) and 19 more\n"},
		{"one item per list", &intent.Brief{Instructions: instructions, Manifests: []string{"go.mod"}, Module: "example.com/m", GoDirs: 2},
			"spec rules that apply: '" + longID(0) + "' (trusted) and 21 more\n"},
	} {
		f, ok := intent.Finding(intent.Resolve([]byte("fix "+strings.Join(names, " ")), false, p), intent.Delivered{}, c.b, 1)
		if !ok {
			t.Errorf("%s: no note for a request naming eight files", c.name)
			continue
		}
		text := f.Message.Text()
		if len(text) > render.MaxBriefingMessageBytes || strings.Count(text, "\n") > 5 || render.ValidateBriefing(text) != nil {
			t.Errorf("%s: note is %d bytes, %d lines: %v", c.name, len(text), strings.Count(text, "\n")+1, render.ValidateBriefing(text))
		}
		if !strings.Contains(text, c.want) || strings.Contains(text, "word word") {
			t.Errorf("%s: note does not step down as it should; want %q and no why:\n%s", c.name, c.want, text)
		}
		st, err := store.Open(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		if out := routeAt(t, st, "p1", false, f); out.AdditionalContext != text {
			t.Errorf("%s: the capped note was not delivered: %+v", c.name, out)
		}
	}
}

// TestPF1h_VerbClassTableIsClosed: the class list is the eight classes in
// order, the first class word in the request decides, and a request with no
// class word is "other", which never fails.
func TestPF1h_VerbClassTableIsClosed(t *testing.T) {
	want := []intent.Verb{"add", "fix", "refactor", "test", "migrate", "deploy", "document", "other"}
	if got := intent.Verbs(); !slices.Equal(got, want) {
		t.Fatalf("Verbs() = %q, want %q", got, want)
	}
	for prompt, verb := range map[string]intent.Verb{
		"Add a --json flag":            "add",
		"FIX the race in the store":    "fix",
		"refactor the parser":          "refactor",
		"write tests for the lexer":    "test",
		"migrate the schema to v6":     "migrate",
		"deploy to staging":            "deploy",
		"document the steer log":       "document",
		"continue":                     "other",
		"":                             "other",
		"add a test for the fix":       "add",
		"test the fix before you ship": "test",
		"prefix the ids":               "other",
	} {
		if got := intent.Resolve([]byte(prompt), false, intent.Project{Root: t.TempDir()}).Verb; got != verb {
			t.Errorf("Resolve(%q).Verb = %q, want %q", prompt, got, verb)
		}
	}
}

// TestPF1h_EvidenceKindsAreTheSpecs: every kind the class table can name is
// one of spec.EvidenceKinds, which the note's evidence table holds, and a
// task file's expected evidence replaces the class's, said so.
func TestPF1h_EvidenceKindsAreTheSpecs(t *testing.T) {
	for _, k := range spec.EvidenceKinds {
		if err := slot.NewEnum(slot.EnumEvidenceKinds, k).Validate(); err != nil {
			t.Errorf("evidence kind %q is not in the note's table: %v", k, err)
		}
	}
	root := projectTree(t)
	sp := specOf(root, spec.Trusted)
	sp.Task = &spec.Task{ID: "t1", ExpectedEvidence: []string{"real_data_run", "cross_check"}}
	text := noteOf(t, "fix internal/verify/stop.go", projectOf(root, sp, indexOf()), givenBrief(), nil)
	if !strings.Contains(text, "evidence for a done claim (task file): real_data_run; cross_check\n") {
		t.Errorf("note does not carry the task file's evidence:\n%s", text)
	}
}

// TestPF1i_FirstPromptCarriesTheBrief: the session's first note is the
// first-prompt row, which carries the project line, even for a request that
// names nothing; once delivered, later notes take the plain row; a project
// with nothing to brief and a request naming nothing gets no note.
func TestPF1i_FirstPromptCarriesTheBrief(t *testing.T) {
	root := projectTree(t)
	p := projectOf(root, specOf(root, spec.Trusted), indexOf())
	b := &intent.Brief{Instructions: []string{"CLAUDE.md"}, Manifests: []string{"go.mod"}, Module: "example.com/m", GoDirs: 3}
	f, ok := intent.Finding(intent.Resolve([]byte("continue"), false, p), intent.Delivered{}, b, 1)
	if !ok {
		t.Fatal("the first prompt got no brief")
	}
	if f.Pattern != pattern.TaskPreflight || f.Key != "brief" ||
		!strings.Contains(f.Message.Text(), "\nproject files: 'CLAUDE.md'; manifests: 'go.mod'; directories with Go files: 3\n") {
		t.Errorf("first note = %s/%s\n%s", f.Pattern, f.Key, f.Message.Text())
	}
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	routeAt(t, st, "p1", false, f)
	d := delivered(t, st)
	if !d.BriefGiven {
		t.Fatal("the delivered brief is not read back as given")
	}
	next := mustFinding(t, "fix internal/verify/stop.go", p, d)
	if next.Key != "note" || strings.Contains(next.Message.Text(), "project files:") {
		t.Errorf("a later note carries the brief again:\n%s", next.Message.Text())
	}
	if _, ok := intent.Finding(intent.Resolve([]byte("continue"), false, p), intent.Delivered{}, &intent.Brief{}, 1); ok {
		t.Error("an empty brief and a request naming nothing got a note")
	}
	f, _ = intent.Finding(intent.Resolve([]byte("continue"), false, p), intent.Delivered{}, &intent.Brief{GoDirs: 2000, WalkCapped: true}, 1)
	if !strings.Contains(f.Message.Text(), "package walk capped, count is a lower bound") {
		t.Errorf("a capped walk is not said so:\n%s", f.Message.Text())
	}
	f, _ = intent.Finding(intent.Resolve([]byte("continue"), false, p), intent.Delivered{}, &intent.Brief{Instructions: []string{"CLAUDE.md"}}, 1)
	if !strings.Contains(f.Message.Text(), "no go.mod, packages not counted") {
		t.Errorf("a project with no go.mod is not said so:\n%s", f.Message.Text())
	}
}

// TestPF1j_Over16KiBPromptIsTruncatedAndSaidSo: only the first 16 KiB of a
// request is read, a path named past it is not, and the note says the rest
// was not read; a request the reader already cut says so too.
func TestPF1j_Over16KiBPromptIsTruncatedAndSaidSo(t *testing.T) {
	root := projectTree(t)
	p := projectOf(root, specOf(root, spec.Trusted), indexOf())
	prompt := "fix internal/verify/stop.go " + strings.Repeat("x ", intent.MaxPromptBytes/2) + " and .github/workflows/ci.yaml"
	in := intent.Resolve([]byte(prompt), false, p)
	if !in.Truncated || slices.Contains(in.Paths, ".github/workflows/ci.yaml") || !slices.Contains(in.Paths, "internal/verify/stop.go") {
		t.Errorf("Resolve = truncated %v, paths %q; want truncated, the path past 16 KiB unread", in.Truncated, in.Paths)
	}
	f, ok := intent.Finding(in, givenBrief(), nil, 1)
	if !ok || !strings.Contains(f.Message.Text(), "request over 16 KiB, the rest not read") {
		t.Errorf("the cut is not said:\n%s", f.Message.Text())
	}
	if !intent.Resolve([]byte("fix internal/verify/stop.go"), true, p).Truncated {
		t.Error("a request the reader cut does not read as truncated")
	}
}

// TestPF1j_PathTokensAreBoundedAndStayUnderTheRoot: at most eight paths are
// kept and at most 32 tokens are looked up; a token leaving the root, an
// absolute path outside it, or a link is never followed.
func TestPF1j_PathTokensAreBoundedAndStayUnderTheRoot(t *testing.T) {
	root := projectTree(t)
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.go"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	var tokens []string
	for i := range 40 {
		tokens = append(tokens, fmt.Sprintf("pkg%d/file.go", i))
	}
	looked := 0
	p := projectOf(root, nil, nil)
	p.Lstat = func(rel string) (fs.FileInfo, error) {
		looked++
		return os.Lstat(filepath.Join(root, filepath.FromSlash(rel)))
	}
	intent.Resolve([]byte(strings.Join(tokens, " ")), false, p)
	if looked > intent.MaxLstat {
		t.Errorf("%d lookups, want at most %d", looked, intent.MaxLstat)
	}
	p.Lstat = nil
	in := intent.Resolve([]byte("fix ../"+filepath.Base(outside)+"/secret.go "+filepath.Join(outside, "secret.go")+" link/secret.go internal/verify/../verify/stop.go"), false, p)
	if !slices.Equal(in.Paths, []string{"internal/verify/stop.go"}) {
		t.Errorf("paths = %q, want only the cleaned path under the root", in.Paths)
	}
}

// TestPF1_NoRegexpInIntent: the tokenizer is a byte scanner; the package
// imports no regexp, so linking it adds no compile at init or on first use
// (RES-2).
func TestPF1_NoRegexpInIntent(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	checked := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, f, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		checked++
		for _, imp := range file.Imports {
			if imp.Path.Value == `"regexp"` {
				t.Errorf("%s imports regexp", f)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no package files found")
	}
}

// TestPF1t_FirstPromptOfAnEmptyRunIsDelivered: at a session's first prompt
// the run holds no record, so the preflight decides at after_seq 0. The
// first-prompt note is delivered there on UserPromptSubmit, brief included;
// the same finding routed on PostToolUse is refused and writes no line.
// Break: Check's after_seq floor of 1 on every event, and the brief never
// prints on prompt 1.
func TestPF1t_FirstPromptOfAnEmptyRunIsDelivered(t *testing.T) {
	root := projectTree(t)
	p := projectOf(root, specOf(root, spec.Trusted), indexOf())
	b := &intent.Brief{Instructions: []string{"CLAUDE.md"}, Manifests: []string{"go.mod"}, Module: "example.com/m", GoDirs: 3}
	f, ok := intent.Finding(intent.Resolve([]byte("fix internal/verify/stop.go"), false, p), intent.Delivered{}, b, 0)
	if !ok || f.AfterSeq != 0 || f.Key != "brief" {
		t.Fatalf("Finding at seq 0 = ok %v, after_seq %d, key %q; want the first-prompt note at 0", ok, f.AfterSeq, f.Key)
	}
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	out := routeAt(t, st, "p1", false, f)
	if !strings.Contains(out.AdditionalContext, "project files: 'CLAUDE.md'") || !delivered(t, st).BriefGiven {
		t.Errorf("prompt 1 of an empty run: additionalContext %q, want the brief delivered", out.AdditionalContext)
	}

	other, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	prompt := "p1"
	post := steer.Route(&steer.Context{
		Event: steer.EventPostToolUse, Store: other, SessionID: "s1", PromptID: &prompt, Project: "/repo",
		HarnessID: "claude_code", Surface: "cli", NowUnixMS: nowMS, Consent: true, Arm: steer.ArmLive,
		Attendance: steer.AttendanceAttended,
		NewID:      func() string { lineIDs++; return fmt.Sprintf("%032d", lineIDs) },
	}, []steer.Finding{f})
	lines, _, err := other.ReadSteer("s1")
	if err != nil {
		t.Fatal(err)
	}
	if post.AdditionalContext != "" || post.SystemMessage != "" || len(lines) != 0 {
		t.Errorf("an after_seq 0 finding on PostToolUse printed %q / %q or wrote %d lines", post.AdditionalContext, post.SystemMessage, len(lines))
	}
}
