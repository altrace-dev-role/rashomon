package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/arm"
	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/hook"
	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/learn"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// lessonEnv is the prompt path's fixture (LEARN-DELIVER1): a started store
// with the steer disclosure marker, a project directory the payloads name as
// their cwd, empty, so the request's preflight has nothing to say beside a
// lesson, and that project's ledger name under the store, as `rashomon learn`
// names it.
type lessonEnv struct {
	t         *testing.T
	root      string
	installID string
	cwd       string
	project   string
}

func newLessonEnv(t *testing.T) *lessonEnv {
	t.Helper()
	root, installID := benchStarted(t)
	if err := store.WriteSteerDisclosureMarker(root, steer.DisclosureVersion); err != nil {
		t.Fatal(err)
	}
	t.Setenv(hook.EnvHoldout, "off")
	// CI set makes the session unattended, which suppresses every note at
	// the prompt (decide's no_mirror), and a CI runner sets it. AI_AGENT is
	// cleared so a runner and a Claude Code shell take one path.
	t.Setenv("CI", "")
	t.Setenv("AI_AGENT", "")
	cwd := t.TempDir()
	key := baseline.Key(cwd)
	return &lessonEnv{t: t, root: root, installID: installID, cwd: cwd, project: strings.TrimSuffix(baseline.FileName(key), baseline.FileExt)}
}

// start is the session's SessionStart probe.
func (e *lessonEnv) start(sessionID string) {
	e.t.Helper()
	in := benchJSON(e.t, struct {
		benchHead
		Source string `json:"source"`
	}{benchHead{sessionID, benchTranscript, e.cwd, "default", "SessionStart"}, "startup"})
	var stderr bytes.Buffer
	if code := cmdProbe([]string{"start", install.Marker, e.installID}, bytes.NewReader(in), io.Discard, &stderr); code != exitOK {
		e.t.Fatalf("probe start %s: exit %d, stderr %q", sessionID, code, stderr.String())
	}
}

// call records one Bash call in the session, so the next prompt stands
// after a seq.
func (e *lessonEnv) call(sessionID, promptID, toolUseID string) {
	e.t.Helper()
	in := benchJSON(e.t, benchToolPayload{
		benchHead: benchHead{sessionID, benchTranscript, e.cwd, "default", "PreToolUse"},
		PromptID:  promptID, ToolName: "Bash", ToolInput: json.RawMessage(`{"command":"ls"}`), ToolUseID: toolUseID,
	})
	var stderr bytes.Buffer
	if code := cmdHook([]string{install.Marker, e.installID}, bytes.NewReader(in), &stderr); code != exitOK {
		e.t.Fatalf("hook %s: exit %d, stderr %q", toolUseID, code, stderr.String())
	}
}

// lessonsInto imports rows (id, text pairs) into project's ledger under
// root one second apart, so their order is the order given, and approves the
// ids named, as the learn verb does after the code is typed. The tests and
// BenchmarkRecapPending's lesson rows build their ledgers with it.
func lessonsInto(tb testing.TB, root, project string, rows [][2]string, approve ...string) {
	tb.Helper()
	base := time.UnixMilli(1_761_700_000_000)
	for i, r := range rows {
		file, err := json.Marshal(map[string]any{"lessons": []map[string]string{{"id": r[0], "text": r[1]}}})
		if err != nil {
			tb.Fatal(err)
		}
		l := ledgerAt(tb, root, project, base)
		if _, _, err := l.Import(bytes.NewReader(file), base.Add(time.Duration(i)*time.Second)); err != nil {
			tb.Fatal(err)
		}
		if err := l.Save(); err != nil {
			tb.Fatal(err)
		}
	}
	approveLessons(tb, root, project, approve...)
}

func approveLessons(tb testing.TB, root, project string, ids ...string) {
	tb.Helper()
	l := ledgerAt(tb, root, project, time.Now())
	for _, id := range ids {
		if err := l.Approve(id, time.Now()); err != nil {
			tb.Fatal(err)
		}
	}
	if err := l.Save(); err != nil {
		tb.Fatal(err)
	}
}

func ledgerAt(tb testing.TB, root, project string, now time.Time) *learn.Ledger {
	tb.Helper()
	l, err := learn.Open(root, project, now)
	if err != nil {
		tb.Fatal(err)
	}
	return l
}

func (e *lessonEnv) ledger(rows [][2]string, approve ...string) {
	e.t.Helper()
	lessonsInto(e.t, e.root, e.project, rows, approve...)
}

func (e *lessonEnv) approve(ids ...string) {
	e.t.Helper()
	approveLessons(e.t, e.root, e.project, ids...)
}

func (e *lessonEnv) open(now time.Time) *learn.Ledger {
	e.t.Helper()
	return ledgerAt(e.t, e.root, e.project, now)
}

func (e *lessonEnv) learnDir() string { return filepath.Join(e.root, learn.DirLearn, e.project) }

// promptOut is what recap pending printed for one prompt.
type promptOut struct {
	stdout, system, context string
}

// prompt runs recap pending on the session's prompt as a settings entry
// does, calling recapTurn itself so an error or a panic is seen, not
// dropped by cmdRecap's guard.
func (e *lessonEnv) prompt(sessionID, promptID, text string) promptOut {
	e.t.Helper()
	in, err := json.Marshal(map[string]any{
		"session_id": sessionID, "hook_event_name": "UserPromptSubmit", "transcript_path": benchTranscript,
		"cwd": e.cwd, "permission_mode": "default", "prompt": text, "prompt_id": promptID,
	})
	if err != nil {
		e.t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := recapTurn([]string{"pending", install.Marker, e.installID}, bytes.NewReader(in), &stdout, &stderr); err != nil {
		e.t.Fatalf("recap pending %s %s: %v (stderr %q)", sessionID, promptID, err, stderr.String())
	}
	out := promptOut{stdout: stdout.String()}
	if out.stdout == "" {
		return out
	}
	var o struct {
		SystemMessage      string `json:"systemMessage"`
		HookSpecificOutput struct {
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &o); err != nil {
		e.t.Fatalf("recap pending printed %q, not one JSON object: %v", out.stdout, err)
	}
	out.system, out.context = o.SystemMessage, o.HookSpecificOutput.AdditionalContext
	return out
}

// lessonLine is the part of a steer line these tests read.
type lessonLine struct {
	PromptID     *string `json:"prompt_id"`
	Pattern      string  `json:"pattern"`
	Kind         string  `json:"kind"`
	Key          string  `json:"key"`
	State        string  `json:"state"`
	Delivery     *string `json:"delivery"`
	SuppressedBy *string `json:"suppressed_by"`
	ModelText    string  `json:"model_text"`
	UserText     string  `json:"user_text"`
}

// lessonLines are the session's learned-fact steer lines, in order.
func (e *lessonEnv) lessonLines(sessionID string) []lessonLine {
	e.t.Helper()
	st, err := store.Open(e.root)
	if err != nil {
		e.t.Fatal(err)
	}
	raw, _, err := st.ReadSteer(sessionID)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		e.t.Fatal(err)
	}
	var out []lessonLine
	for _, b := range raw {
		var l lessonLine
		if err := json.Unmarshal(b, &l); err != nil {
			e.t.Fatal(err)
		}
		if l.Pattern == "learned-fact" {
			out = append(out, l)
		}
	}
	return out
}

func (l lessonLine) String() string {
	d, by, p := "", "", ""
	if l.Delivery != nil {
		d = *l.Delivery
	}
	if l.SuppressedBy != nil {
		by = *l.SuppressedBy
	}
	if l.PromptID != nil {
		p = *l.PromptID
	}
	return fmt.Sprintf("%s %s/%s key %s: %s %s %s", p, l.Pattern, l.Kind, l.Key, l.State, d, by)
}

const (
	cwdLesson  = "run go test from the module root"
	cwdNote    = "rashomon record: lesson go-test-cwd, approved for this project: 'run go test from the module root'."
	tamperFlag = "rashomon record: this project's lesson files fail their signature check, so no lesson was given to the agent; rashomon learn lists them."
)

// signLearnState signs project's state files under root as they are now, as
// an agent that read learn/key could (the manifest is tamper evidence, not a
// boundary): an HMAC-SHA256 over each state file's name and digest, "-" for
// one absent.
func signLearnState(tb testing.TB, root, project string) {
	tb.Helper()
	key, err := os.ReadFile(filepath.Join(root, learn.DirLearn, learn.FileKey))
	if err != nil {
		tb.Fatal(err)
	}
	dir := filepath.Join(root, learn.DirLearn, project)
	m := hmac.New(sha256.New, key)
	for _, name := range []string{learn.FileLessons, learn.FileLabels, learn.FileActive} {
		digest := "-"
		if b, err := os.ReadFile(filepath.Join(dir, name)); err == nil {
			d := sha256.Sum256(b)
			digest = hex.EncodeToString(d[:])
		}
		fmt.Fprintf(m, "%s\x00%s\n", name, digest)
	}
	if err := os.WriteFile(filepath.Join(dir, learn.FileSum), []byte(hex.EncodeToString(m.Sum(nil))+"\n"), 0o600); err != nil {
		tb.Fatal(err)
	}
}

func (e *lessonEnv) resign() {
	e.t.Helper()
	signLearnState(e.t, e.root, e.project)
}

// TestLRN17_OnlyApprovedFactsReachTheModel (STR-52's pair): a proposed
// lesson gives the model nothing and writes no line, and so does a proposed
// row in an active.json signed with the key, which Save never writes;
// approved through the ledger, the lesson arrives once: its text in quotes
// as additionalContext, the mirror to the user, and one steer line,
// learned-fact, kind local, keyed by the lesson's id, delivered.
func TestLRN17_OnlyApprovedFactsReachTheModel(t *testing.T) {
	e := newLessonEnv(t)
	e.start("s-lrn17")
	e.ledger([][2]string{{"go-test-cwd", cwdLesson}})
	if out := e.prompt("s-lrn17", "prompt-1", "continue please"); out.context != "" || strings.Contains(out.stdout, cwdLesson) {
		t.Errorf("a proposed lesson: stdout %q, want no lesson", out.stdout)
	}

	path := filepath.Join(e.learnDir(), learn.FileActive)
	forged := `{"schema":"active/1","quarantined":false,"lessons":[{"id":"go-test-cwd","tier":"proposed","text":"run go test from the module root","since_unix_ms":1761700000000}]}` + "\n"
	if err := os.WriteFile(path, []byte(forged), 0o600); err != nil {
		t.Fatal(err)
	}
	e.resign()
	if out := e.prompt("s-lrn17", "prompt-2", "continue please"); out.context != "" || strings.Contains(out.stdout, cwdLesson) {
		t.Errorf("a proposed row in a signed active.json: stdout %q, want no lesson", out.stdout)
	}
	if l := e.open(time.Now()); l.Quarantined {
		t.Fatal("premise: the re-signed state opens quarantined")
	}
	if got := e.lessonLines("s-lrn17"); len(got) != 0 {
		t.Errorf("lines before approval: %v, want none", got)
	}

	e.approve("go-test-cwd")
	out := e.prompt("s-lrn17", "prompt-3", "continue please")
	if out.context != cwdNote {
		t.Errorf("additionalContext = %q, want %q", out.context, cwdNote)
	}
	if want := `rashomon told the agent: "` + cwdNote + `"`; !strings.Contains(out.system, want) {
		t.Errorf("systemMessage = %q, want the mirror %q", out.system, want)
	}
	got := e.lessonLines("s-lrn17")
	if len(got) != 1 || got[0].Kind != "local" || got[0].Key != "go-test-cwd" || got[0].State != "delivered" ||
		got[0].PromptID == nil || *got[0].PromptID != "prompt-3" || got[0].ModelText != cwdNote {
		t.Errorf("lines after approval: %v, want one learned-fact/local line keyed go-test-cwd, delivered at prompt-3", got)
	}
}

// learnFiles is every file under the store's learn directory with its
// bytes and modification time.
func (e *lessonEnv) learnFiles() map[string]string {
	e.t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(filepath.Join(e.root, learn.DirLearn), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		out[p] = fmt.Sprintf("%x %d", sha256.Sum256(b), info.ModTime().UnixNano())
		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		e.t.Fatal(err)
	}
	return out
}

// TestLRN17b_TamperedStateDeliversNothingAndWritesNothing: active.json
// edited after Save gives the model nothing, the user the signature flag
// once in the session (kind local, key "#unchecked"), and leaves every file
// under learn/ as it was: the prompt path verifies and never quarantines.
// The flag's key is outside the id form, so a lesson whose id is
// "unchecked" is not counted as given by it once the state is repaired.
func TestLRN17b_TamperedStateDeliversNothingAndWritesNothing(t *testing.T) {
	e := newLessonEnv(t)
	e.start("s-lrn17b")
	e.ledger([][2]string{{"go-test-cwd", cwdLesson}, {"unchecked", "npm ci before npm test"}}, "go-test-cwd", "unchecked")
	path := filepath.Join(e.learnDir(), learn.FileActive)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, bytes.Replace(b, []byte(`"quarantined":false`), []byte(`"quarantined":false `), 1), 0o600); err != nil {
		t.Fatal(err)
	}
	before := e.learnFiles()

	out := e.prompt("s-lrn17b", "prompt-1", "continue please")
	if out.context != "" || !strings.Contains(out.system, tamperFlag) {
		t.Errorf("a tampered state: systemMessage %q, additionalContext %q; want the signature flag alone", out.system, out.context)
	}
	if again := e.prompt("s-lrn17b", "prompt-2", "continue please"); again.stdout != "" {
		t.Errorf("the second prompt printed %q, want the flag once per session", again.stdout)
	}
	got := e.lessonLines("s-lrn17b")
	if len(got) != 1 || got[0].Kind != "local" || got[0].Key != learn.UncheckedKey || learn.UncheckedKey != "#unchecked" || got[0].State != "delivered" || got[0].ModelText != "" {
		t.Errorf("lines: %v, want one flag line keyed #unchecked, delivered, no model text", got)
	}
	if after := e.learnFiles(); fmt.Sprint(after) != fmt.Sprint(before) || len(before) == 0 {
		t.Errorf("the prompt path changed the learn files:\nbefore %v\nafter  %v", before, after)
	}

	// Repaired at the terminal (the verb's read quarantines, a person
	// approves again): the lesson whose id is "unchecked" still arrives in
	// this session, since the flag's key is not its id.
	if l := e.open(time.Now()); !l.Quarantined {
		t.Fatal("premise: the learn verb's read did not quarantine the tampered state")
	}
	e.approve("go-test-cwd", "unchecked")
	if out := e.prompt("s-lrn17b", "prompt-3", "continue please"); out.context != cwdNote {
		t.Errorf("repaired, prompt 3: additionalContext %q, want the first lesson", out.context)
	}
	if out := e.prompt("s-lrn17b", "prompt-4", "continue please"); !strings.Contains(out.context, "lesson unchecked, approved") {
		t.Errorf("repaired, prompt 4: additionalContext %q, want the lesson whose id is unchecked", out.context)
	}
}

// TestLRN17c_OncePerSession: a lesson given in a session is not given again
// in it, after an earlier turn as well as at prompt 1; the next session gets
// it again. A held-out delivery and a flag-only one (no consent marker)
// count as given for the session too (PREFLIGHT1's P16 rule).
func TestLRN17c_OncePerSession(t *testing.T) {
	e := newLessonEnv(t)
	e.ledger([][2]string{{"go-test-cwd", cwdLesson}}, "go-test-cwd")

	e.start("s-once-a")
	if out := e.prompt("s-once-a", "prompt-1", "continue please"); out.context != cwdNote {
		t.Fatalf("session a, prompt 1: additionalContext %q, want the lesson", out.context)
	}
	e.call("s-once-a", "prompt-1", "toolu_once_1")
	if out := e.prompt("s-once-a", "prompt-2", "continue please"); strings.Contains(out.stdout, "go-test-cwd") {
		t.Errorf("session a, prompt 2 (after a call): stdout %q, want no lesson", out.stdout)
	}
	if got := e.lessonLines("s-once-a"); len(got) != 1 {
		t.Errorf("session a lines: %v, want the one delivery", got)
	}
	e.start("s-once-b")
	if out := e.prompt("s-once-b", "prompt-1", "continue please"); out.context != cwdNote {
		t.Errorf("session b, prompt 1: additionalContext %q, want the lesson again", out.context)
	}

	held := heldOutSession(t, e.root, "s-once-held-")
	t.Setenv(hook.EnvHoldout, "on")
	e.start(held)
	if out := e.prompt(held, "prompt-1", "continue please"); out.context != "" || !strings.Contains(out.system, "measurement holdout") || strings.Contains(out.stdout, "go-test-cwd") {
		t.Fatalf("held-out session, prompt 1: stdout %q, want the holdout notice only: no lesson text to the model or the user", out.stdout)
	}
	if out := e.prompt(held, "prompt-2", "continue please"); strings.Contains(out.stdout, "go-test-cwd") {
		t.Errorf("held-out session, prompt 2: stdout %q, want nothing: a held-out delivery is given", out.stdout)
	}
	t.Setenv(hook.EnvHoldout, "off")

	e.start("s-once-flag")
	if err := store.WriteSteerDisclosureMarker(e.root, 0); err != nil {
		t.Fatal(err)
	}
	if out := e.prompt("s-once-flag", "prompt-1", "continue please"); out.context != "" || !strings.Contains(out.system, "go-test-cwd") {
		t.Fatalf("no consent, prompt 1: stdout %q, want the flag to the user only", out.stdout)
	}
	if err := store.WriteSteerDisclosureMarker(e.root, steer.DisclosureVersion); err != nil {
		t.Fatal(err)
	}
	if out := e.prompt("s-once-flag", "prompt-2", "continue please"); strings.Contains(out.stdout, "go-test-cwd") {
		t.Errorf("consent restored, prompt 2: stdout %q, want nothing: a flag-only delivery is given", out.stdout)
	}
}

// TestLRN17c2_ArmOffGivesTheLessonOnceToo: on a lab store under arm off the
// lesson is decided and withheld (state suppressed, suppressed_by arm_off),
// and that withheld line is the session's given lesson as the steer arm's
// delivered one is: the second prompt decides no second line.
func TestLRN17c2_ArmOffGivesTheLessonOnceToo(t *testing.T) {
	e := newLessonEnv(t)
	if err := store.WriteLab(e.root); err != nil {
		t.Fatal(err)
	}
	t.Setenv(hook.EnvSteerArm, "off")
	e.ledger([][2]string{{"go-test-cwd", cwdLesson}}, "go-test-cwd")

	e.start("s-off-once")
	if out := e.prompt("s-off-once", "prompt-1", "continue please"); strings.Contains(out.stdout, "go-test-cwd") {
		t.Fatalf("prompt 1 under arm off: stdout %q, want nothing printed", out.stdout)
	}
	lines := e.lessonLines("s-off-once")
	if len(lines) != 1 || lines[0].State != "suppressed" || lines[0].SuppressedBy == nil || *lines[0].SuppressedBy != "arm_off" {
		t.Fatalf("prompt 1 lines: %v, want the one lesson withheld by arm_off", lines)
	}
	e.call("s-off-once", "prompt-1", "toolu_off_1")
	e.prompt("s-off-once", "prompt-2", "continue please")
	if got := e.lessonLines("s-off-once"); len(got) != 1 {
		t.Errorf("lines after prompt 2: %v, want the one: the steer arm gives a lesson once", got)
	}
}

// heldOutSession is the first session id with prefix that the store's key
// draws into the steer holdout at the share RASHOMON_HOLDOUT=on gives, as the prompt path
// draws it.
func heldOutSession(t *testing.T, root, prefix string) string {
	t.Helper()
	st, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	a, err := arm.New(st.Key())
	if err != nil {
		t.Fatal(err)
	}
	for i := range 500 {
		id := fmt.Sprintf("%s%d", prefix, i)
		if got, err := a.Assign("steer", id, 1000); err == nil && got == arm.Holdout {
			return id
		}
	}
	t.Fatal("no held-out session id in 500 draws at 10 percent")
	return ""
}

// TestLRN17m_SupersededAtPromptNArrivesAtNPlus1 (pins D170 L3): a
// task-preflight note outranks the lesson, so on a prompt that carries one
// the lesson's line is superseded (budget_batch) and not given; it arrives
// on the first prompt with no preflight note, k prompts late after a run of
// k preflight prompts.
func TestLRN17m_SupersededAtPromptNArrivesAtNPlus1(t *testing.T) {
	e := newLessonEnv(t)
	for _, f := range []string{"internal/a.go", "internal/b.go"} {
		p := filepath.Join(e.cwd, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("package internal\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	e.ledger([][2]string{{"go-test-cwd", cwdLesson}}, "go-test-cwd")
	e.start("s-lrn17c2")
	for i, prompt := range []string{"fix internal/a.go", "now fix internal/b.go"} {
		id := fmt.Sprintf("prompt-%d", i+1)
		out := e.prompt("s-lrn17c2", id, prompt)
		if out.context == "" || strings.Contains(out.context, "go-test-cwd") {
			t.Fatalf("%s names a new path: additionalContext %q, want the preflight note and no lesson", id, out.context)
		}
	}
	out := e.prompt("s-lrn17c2", "prompt-3", "continue please")
	if out.context != cwdNote {
		t.Errorf("prompt-3 carries no preflight note: additionalContext %q, want the lesson", out.context)
	}
	got := e.lessonLines("s-lrn17c2")
	want := []string{"prompt-1 superseded budget_batch", "prompt-2 superseded budget_batch", "prompt-3 delivered "}
	if len(got) != len(want) {
		t.Fatalf("lines: %v, want %v", got, want)
	}
	for i, l := range got {
		by := ""
		if l.SuppressedBy != nil {
			by = *l.SuppressedBy
		}
		if s := *l.PromptID + " " + l.State + " " + by; s != want[i] {
			t.Errorf("line %d: %q, want %q", i, s, want[i])
		}
	}
}

const (
	aGenLesson = "internal/a.go is generated from gen/a.tmpl"
	aGenRide   = "lesson a-gen, approved for this project: 'internal/a.go is generated from gen/a.tmpl'"
)

// goFilesIn writes each of files, a path under dir, as a Go file.
func goFilesIn(t *testing.T, dir string, files ...string) {
	t.Helper()
	for _, f := range files {
		p := filepath.Join(dir, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("package internal\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// rodeLessons are the session's task-preflight lines that name a lesson in
// their lesson slot, as "prompt state lesson", in order.
func (e *lessonEnv) rodeLessons(sessionID string) []string {
	e.t.Helper()
	st, err := store.Open(e.root)
	if err != nil {
		e.t.Fatal(err)
	}
	raw, _, err := st.ReadSteer(sessionID)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		e.t.Fatal(err)
	}
	var out []string
	for _, b := range raw {
		var l struct {
			PromptID *string `json:"prompt_id"`
			Pattern  string  `json:"pattern"`
			State    string  `json:"state"`
			Slots    struct {
				Lesson string `json:"lesson"`
			} `json:"slots"`
		}
		if err := json.Unmarshal(b, &l); err != nil {
			e.t.Fatal(err)
		}
		if l.Pattern == "task-preflight" && l.Slots.Lesson != "" && l.PromptID != nil {
			out = append(out, *l.PromptID+" "+l.State+" "+l.Slots.Lesson)
		}
	}
	return out
}

// TestLRN17r_ALessonRidesTheNoteOnThePromptItIsAbout (LEARN-RIDE1, R13 D1):
// a lesson whose text names a path the request names arrives on that prompt,
// as the last line of its preflight note: one note, the one the prompt gets
// with no lesson and the lesson's line after it, and one steer line, the
// note's, which names the lesson. It is given then, so a later prompt that
// names the path again carries no lesson, and no learned-fact line is
// written for it. Break: route the lesson beside the note, which supersedes
// it, and it arrives a prompt late or not at all.
func TestLRN17r_ALessonRidesTheNoteOnThePromptItIsAbout(t *testing.T) {
	e := newLessonEnv(t)
	goFilesIn(t, e.cwd, "internal/a.go", "internal/b.go")
	plain := e.prompt("s-lrn17r-plain", "prompt-1", "fix internal/a.go").context
	if strings.Count(plain, "\n") != 4 {
		t.Fatalf("premise: a request naming a path gets the five-line note, got %q", plain)
	}
	e.ledger([][2]string{{"a-gen", aGenLesson}}, "a-gen")
	e.start("s-lrn17r")
	if out := e.prompt("s-lrn17r", "prompt-1", "fix internal/a.go"); out.context != plain+"\n"+aGenRide {
		t.Errorf("prompt-1: additionalContext %q,\nwant the note it gets with no lesson and the lesson as its last line", out.context)
	}
	out := e.prompt("s-lrn17r", "prompt-2", "now fix internal/a.go and internal/b.go")
	if out.context == "" || strings.Contains(out.context, "a-gen") {
		t.Errorf("prompt-2: additionalContext %q, want a note for internal/b.go and no lesson: a-gen rode prompt-1", out.context)
	}
	if out := e.prompt("s-lrn17r", "prompt-3", "continue please"); strings.Contains(out.context, "a-gen") {
		t.Errorf("prompt-3: additionalContext %q, want no lesson: a-gen was given on prompt-1", out.context)
	}
	if got := e.lessonLines("s-lrn17r"); len(got) != 0 {
		t.Errorf("learned-fact lines %v, want none: the lesson rode the note", got)
	}
	if got, want := e.rodeLessons("s-lrn17r"), []string{"prompt-1 delivered a-gen"}; !slices.Equal(got, want) {
		t.Errorf("task-preflight lines naming a lesson: %v, want %v", got, want)
	}
}

// TestLRN17d_QuarantinedOrDemotedNeverReachesTheModel: a ledger the learn
// verb quarantined gives none; a demoted row gives none, while a proven row
// in the same active.json is given (the positive control).
func TestLRN17d_QuarantinedOrDemotedNeverReachesTheModel(t *testing.T) {
	e := newLessonEnv(t)
	e.ledger([][2]string{{"go-test-cwd", cwdLesson}, {"npm-ci", "npm ci before npm test"}}, "go-test-cwd", "npm-ci")
	l := e.open(time.Now())
	for i := range 30 {
		if err := l.AppendLabel(learn.Label{LessonID: "go-test-cwd", SessionID: fmt.Sprintf("s-%d", i), AtUnixMS: 1, Label: steer.LabelConfirmed}); err != nil {
			t.Fatal(err)
		}
	}
	for i := range 2 {
		if err := l.AppendLabel(learn.Label{LessonID: "npm-ci", SessionID: fmt.Sprintf("s-%d", i), AtUnixMS: 1, Label: steer.LabelRefuted}); err != nil {
			t.Fatal(err)
		}
	}
	tiers := map[string]learn.Tier{}
	for _, ls := range e.open(time.Now()).Lessons() {
		tiers[ls.ID] = ls.Tier
	}
	if tiers["go-test-cwd"] != learn.TierProven || tiers["npm-ci"] != learn.TierDemoted {
		t.Fatalf("premise: tiers %v, want go-test-cwd proven and npm-ci demoted", tiers)
	}
	e.start("s-lrn17d")
	for i := range 3 {
		out := e.prompt("s-lrn17d", fmt.Sprintf("prompt-%d", i+1), "continue please")
		if strings.Contains(out.stdout, "npm") {
			t.Errorf("prompt %d gave the demoted lesson: %q", i+1, out.stdout)
		}
		if i == 0 && out.context != cwdNote {
			t.Errorf("prompt 1: additionalContext %q, want the proven lesson", out.context)
		}
	}

	q := newLessonEnv(t)
	q.ledger([][2]string{{"go-test-cwd", cwdLesson}}, "go-test-cwd")
	path := filepath.Join(q.learnDir(), learn.FileLessons)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(b, ' '), 0o600); err != nil {
		t.Fatal(err)
	}
	if l := q.open(time.Now()); !l.Quarantined {
		t.Fatal("premise: the learn verb's read did not quarantine")
	}
	q.start("s-lrn17d-q")
	if out := q.prompt("s-lrn17d-q", "prompt-1", "continue please"); out.stdout != "" {
		t.Errorf("a quarantined ledger: stdout %q, want nothing", out.stdout)
	}
}

// TestLRN17e_NoLedgerPrintsNothingAndCreatesNothing: a project with no
// learn directory prints nothing and creates nothing. A guard: it passes
// before the delivery exists.
func TestLRN17e_NoLedgerPrintsNothingAndCreatesNothing(t *testing.T) {
	e := newLessonEnv(t)
	e.start("s-lrn17e")
	if out := e.prompt("s-lrn17e", "prompt-1", "continue please"); out.stdout != "" {
		t.Errorf("stdout %q, want nothing", out.stdout)
	}
	if _, err := os.Lstat(filepath.Join(e.root, learn.DirLearn)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the prompt made %s (%v)", learn.DirLearn, err)
	}
	if got := e.lessonLines("s-lrn17e"); len(got) != 0 {
		t.Errorf("lines %v, want none", got)
	}
}

// TestLRN17f_HeldOutSessionGivesTheUserOnly: in a session the draw holds
// out, the model gets nothing and the user gets the once-a-session holdout
// notice (VERSION-GATE1), not the note; the line records the delivery as
// holdout.
func TestLRN17f_HeldOutSessionGivesTheUserOnly(t *testing.T) {
	e := newLessonEnv(t)
	t.Setenv(hook.EnvHoldout, "on")
	e.ledger([][2]string{{"go-test-cwd", cwdLesson}}, "go-test-cwd")
	held := heldOutSession(t, e.root, "s-lrn17f-")
	e.start(held)
	out := e.prompt(held, "prompt-1", "continue please")
	if out.context != "" || !strings.Contains(out.system, "measurement holdout") || strings.Contains(out.stdout, "go-test-cwd") {
		t.Errorf("held out: systemMessage %q, additionalContext %q; want the once-a-session holdout notice, nothing to the model", out.system, out.context)
	}
	got := e.lessonLines(held)
	if len(got) != 1 || got[0].State != "delivered" || got[0].Delivery == nil || *got[0].Delivery != "holdout" || got[0].ModelText != "" {
		t.Errorf("lines %v, want one delivered line, delivery holdout, no model text", got)
	}
}

// TestLRN17n_LessonFindingsPassTheRouterContract: lessonFinding's two
// findings, a lesson's nudge and the unchecked flag, pass the router's
// contract at UserPromptSubmit (steer.Finding.CheckOn), at a session's first
// prompt and after a call. admit drops a finding outside it and nothing
// prints, so a field the contract holds (a stop form, a priority) left unset
// reads as no lesson at all; this names the rule.
func TestLRN17n_LessonFindingsPassTheRouterContract(t *testing.T) {
	lesson := learn.Active{Lessons: []learn.Lesson{{ID: "go-test-cwd", Tier: learn.TierProbation, Text: cwdLesson, ApprovedUnixMS: 1}}}
	for name, a := range map[string]learn.Active{"lesson": lesson, "unchecked": {Unchecked: learn.UncheckedSignature}} {
		for _, after := range []int64{0, 3} {
			f, ok := lessonFinding(a, after, nil, nil)
			if !ok {
				t.Fatalf("%s at after_seq %d: no finding", name, after)
			}
			if err := f.CheckOn(steer.EventUserPromptSubmit); err != nil {
				t.Errorf("%s at after_seq %d: %v", name, after, err)
			}
		}
	}
}
