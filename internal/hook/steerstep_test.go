package hook

import (
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/arm"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

const stSession = "sess-st"

// stFix is one session in a store under the v5 record scope with a verified
// post coverage record appended by hand (card 2.3 A2: the hook's own
// coverage record is unverified without an install), driven through Handler
// and Post on a clock the test moves a minute per record.
type stFix struct {
	t   *testing.T
	st  *store.Store
	now time.Time
	// cwd is each payload's cwd, /tmp/project unless a test moves it.
	cwd string
	// setup runs on each Post before its Capture.
	setup func(*Post)
}

func newST(t *testing.T) *stFix {
	t.Helper()
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	st := mustOpen(t)
	if err := store.WriteRecordScopeMarker(st.Root(), store.RecordScopeV5); err != nil {
		t.Fatal(err)
	}
	f := &stFix{t: t, st: st, now: time.UnixMilli(1_800_000_000_000), cwd: "/tmp/project"}
	cov := store.Coverage{Type: store.TypeCoverage, SchemaVersion: store.SessionSchemaVersion, RecordedAtMS: f.now.UnixMilli(),
		SessionID: stSession, Phase: store.PhasePost, State: store.StateVerified,
		HookEntry: store.EntryPresentSettings, Probe: store.ProbeFresh}
	if err := st.AppendCoverage(cov); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *stFix) clock() time.Time { return f.now }

func (f *stFix) payload(event, id, tool string, input, extra map[string]any) string {
	f.t.Helper()
	p := map[string]any{"session_id": stSession, "tool_use_id": id, "prompt_id": "p1", "transcript_path": "/tmp/t.jsonl",
		"permission_mode": "default", "tool_name": tool, "cwd": f.cwd, "hook_event_name": event, "tool_input": input}
	maps.Copy(p, extra)
	b, err := json.Marshal(p)
	if err != nil {
		f.t.Fatal(err)
	}
	return string(b)
}

// call lands one tool call, its PreToolUse through Handler and its post
// through Post's Capture a minute later (a failure event when exit is not
// 0), and returns the Post for Steer.
func (f *stFix) call(id, tool string, input map[string]any, exit int) *Post {
	f.t.Helper()
	h := New(f.st, f.clock)
	h.Close(&Signals{}, h.Capture(strings.NewReader(f.payload("PreToolUse", id, tool, input, nil))))
	f.now = f.now.Add(time.Minute)
	event, more := "PostToolUse", map[string]any{"tool_response": map[string]any{"stdout": ""}}
	if exit != 0 {
		event, more = FailureEvent, map[string]any{"error": "Exit code " + strconv.Itoa(exit), "is_interrupt": false, "duration_ms": 30}
	}
	p := NewPost(f.st, f.clock)
	if f.setup != nil {
		f.setup(p)
	}
	if err := p.Capture(strings.NewReader(f.payload(event, id, tool, input, more))); err != nil {
		f.t.Fatalf("post %s: %v", id, err)
	}
	f.now = f.now.Add(time.Minute)
	return p
}

var goTest = map[string]any{"command": "go test ./..."}

// red lands a green `go test ./...`, an Edit of internal/a.go under cwd and
// a red `go test ./...` under redID, the regression the step names, and
// returns the red's Post.
func (f *stFix) red(redID string) *Post {
	f.t.Helper()
	f.call("toolu_g", "Bash", goTest, 0)
	f.call("toolu_e", "Edit", map[string]any{"file_path": filepath.Join(f.cwd, "internal", "a.go"), "old_string": "a", "new_string": "b"}, 0)
	return f.call(redID, "Bash", goTest, 1)
}

// steerLines is the session's steer log, decoded.
func steerLines(t *testing.T, st *store.Store) []steer.Line {
	t.Helper()
	raw, skipped, err := st.ReadSteer(stSession)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if skipped != 0 {
		t.Fatalf("the steer log has %d unreadable lines", skipped)
	}
	out := make([]steer.Line, len(raw))
	for i, b := range raw {
		if err := json.Unmarshal(b, &out[i]); err != nil {
			t.Fatal(err)
		}
	}
	return out
}

// stEnv is what cmd builds for Claude Code 2.1.280 on the CLI, attended,
// with consent answering n.
func stEnv(n int) SteerEnv {
	return SteerEnv{HarnessID: "claude_code", Surface: "cli", MirrorInBand: true, AIAgent: "claude-code_2-1-280_harness",
		Consent: func(string) (int, error) { return n, nil }, Now: time.UnixMilli(1_800_000_600_000)}
}

// printsFlag reports b is the consent-missing flag: a systemMessage carrying
// the regression's note (as its mirror wraps it), and no model-visible key.
func printsFlag(t *testing.T, b []byte) bool {
	t.Helper()
	var out map[string]any
	if len(b) == 0 || json.Unmarshal(b, &out) != nil {
		return false
	}
	msg, _ := out["systemMessage"].(string)
	_, model := out["hookSpecificOutput"]
	return strings.Contains(msg, "rashomon record: gate 'go' exited 0 earlier in this session") && !model
}

// TestST_StepsIsTheTriggerTable: the step runs only on an execution that
// landed in the ordered stream, at a failure event that carried a runner
// (card 2.3 R3) or at either post event when the call's input names
// .claude/rashomon (card P5B, 5.7, D-B7).
func TestST_StepsIsTheTriggerTable(t *testing.T) {
	for _, tc := range []struct {
		event  string
		seq    int64
		runner bool
		store  bool
		want   bool
	}{
		{FailureEvent, 7, true, false, true},
		{"PostToolUse", 7, true, false, false},
		{FailureEvent, 0, true, false, false},
		{FailureEvent, 7, false, false, false},
		{"", 7, true, false, false},
		{"PostToolUse", 7, false, true, true},
		{FailureEvent, 7, false, true, true},
		{"PostToolUse", 0, false, true, false},
		{"", 7, false, true, false},
	} {
		if got := steps(tc.event, tc.seq, tc.runner, tc.store); got != tc.want {
			t.Errorf("steps(%q, %d, %v, %v) = %v, want %v", tc.event, tc.seq, tc.runner, tc.store, got, tc.want)
		}
	}
}

// TestST_SteerEnvBecomesTheContext: contextOf is the one place the port's
// values and the call's become Route's context (card 2.3 R4.5): CI or
// dontAsk is unattended; consent is the marker at or over the disclosure
// version, and an error or no reader is no consent; AI_AGENT gives the
// client version; an agent id is a subagent; the project is the view's root
// only, never the payload's cwd. Holdout is s1's draw under the store's
// key, which is random per run, so the want reads the draw from arm rather
// than fixing it: MS5e and MS5f check the draw, this test that the context
// carries it (card 2.5). The draw runs under EnvHoldout "on", since with
// nothing configured the share is 0 and no session is held out (card
// VERSION-GATE1, TestVG1e).
func TestST_SteerEnvBecomesTheContext(t *testing.T) {
	st := mustOpen(t)
	a, err := arm.New(st.Key())
	if err != nil {
		t.Fatal(err)
	}
	drawn, err := a.Assign(holdoutFeature, "s1", holdoutBP)
	if err != nil {
		t.Fatal(err)
	}
	base := func() (*Post, SteerEnv, verify.SteerView) {
		p := &Post{st: st, sessionID: "s1", promptID: "p1", permissionMode: "default", event: FailureEvent, cwd: "/tmp/project"}
		env := stEnv(steer.DisclosureVersion)
		env.HoldoutOn = true
		return p, env, verify.SteerView{ProjectRoot: "/repo"}
	}
	prompt := "p1"
	want := func() steer.Context {
		return steer.Context{Event: steer.EventPostToolUseFailure,
			Client: steer.Client{Version: "2.1.280", Source: steer.VersionSourceEnv, MirrorInBand: true},
			Store:  st, SessionID: "s1", PromptID: &prompt, Project: "/repo", HarnessID: "claude_code", Surface: "cli",
			NowUnixMS: 1_800_000_600_000, Consent: true, Arm: steer.ArmLive, Attendance: steer.AttendanceAttended,
			Holdout: drawn == arm.Holdout}
	}
	for _, tc := range []struct {
		name string
		edit func(p *Post, env *SteerEnv, v *verify.SteerView)
		diff func(c *steer.Context)
	}{
		{"the base", func(*Post, *SteerEnv, *verify.SteerView) {}, func(*steer.Context) {}},
		{"CI", func(_ *Post, env *SteerEnv, _ *verify.SteerView) { env.CI = true },
			func(c *steer.Context) { c.Attendance = steer.AttendanceUnattended }},
		{"dontAsk", func(p *Post, _ *SteerEnv, _ *verify.SteerView) { p.permissionMode = "dontAsk" },
			func(c *steer.Context) { c.Attendance = steer.AttendanceUnattended }},
		{"bypassPermissions", func(p *Post, _ *SteerEnv, _ *verify.SteerView) { p.permissionMode = "bypassPermissions" },
			func(*steer.Context) {}},
		{"marker 1", func(_ *Post, env *SteerEnv, _ *verify.SteerView) { *env = withConsent(*env, 1, nil) },
			func(c *steer.Context) { c.Consent = false }},
		{"marker 3", func(_ *Post, env *SteerEnv, _ *verify.SteerView) { *env = withConsent(*env, 3, nil) },
			func(*steer.Context) {}},
		{"a reader error", func(_ *Post, env *SteerEnv, _ *verify.SteerView) {
			*env = withConsent(*env, steer.DisclosureVersion, errors.New("unreadable"))
		}, func(c *steer.Context) { c.Consent = false }},
		{"no reader", func(_ *Post, env *SteerEnv, _ *verify.SteerView) { env.Consent = nil },
			func(c *steer.Context) { c.Consent = false }},
		{"the agent form of AI_AGENT", func(_ *Post, env *SteerEnv, _ *verify.SteerView) { env.AIAgent = "claude-code_2-1-280_agent" },
			func(c *steer.Context) { c.Client.Version, c.Client.Source = "", steer.VersionSourceUnknown }},
		{"a subagent", func(p *Post, _ *SteerEnv, _ *verify.SteerView) { p.agentID = "a1" },
			func(c *steer.Context) { c.AgentID, c.Client.Subagent = "a1", true }},
		{"no prompt", func(p *Post, _ *SteerEnv, _ *verify.SteerView) { p.promptID = "" },
			func(c *steer.Context) { c.PromptID = nil }},
		{"no root in the view", func(_ *Post, _ *SteerEnv, v *verify.SteerView) { v.ProjectRoot = "" },
			func(c *steer.Context) { c.Project = "" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, env, view := base()
			tc.edit(p, &env, &view)
			w := want()
			tc.diff(&w)
			if got := p.contextOf(env, view); !reflect.DeepEqual(got, w) {
				t.Errorf("contextOf =\n%+v\nwant\n%+v", got, w)
			}
		})
	}
}

func withConsent(env SteerEnv, n int, err error) SteerEnv {
	env.Consent = func(string) (int, error) { return n, err }
	return env
}

// TestST_ReadFailurePrintsNothing: a run that cannot be read after the
// capture (its directory replaced by a regular file) prints nothing: there
// is no record to cite (card 2.3 R4.3, A6). The control prints the flag.
func TestST_ReadFailurePrintsNothing(t *testing.T) {
	control := newST(t)
	if b := control.red("toolu_r").Steer(stEnv(0)); !printsFlag(t, b) {
		t.Fatalf("premise: the control printed %q, want the consent flag", b)
	}

	f := newST(t)
	p := f.red("toolu_r")
	dir := f.st.RunDir(stSession)
	if err := os.Rename(dir, dir+".moved"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if b := p.Steer(stEnv(0)); b != nil {
		t.Errorf("Steer on an unreadable run = %q, want nil", b)
	}
	if _, err := os.Stat(filepath.Join(dir+".moved", store.FileSteer)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a steer log exists beside the moved run (%v); nothing should have been decided", err)
	}
}

// TestST_OverCapIdMatchesItsRun: R11's TestST_RawIdNotTheCappedOne, renamed
// because its premise is inverted: both records store the capped id
// (CapDeclaration and CapExecution digest a tool_use_id over
// store.MaxIDBytes before the line is written), so a red call whose id is
// over the cap is matched by the capped form, p.toolUseID, and the raw
// payload id would never match its own run.
func TestST_OverCapIdMatchesItsRun(t *testing.T) {
	long := "toolu_" + strings.Repeat("x", store.MaxIDBytes)
	f := newST(t)
	p := f.red(long)
	if b := p.Steer(stEnv(0)); !printsFlag(t, b) {
		t.Errorf("Steer for a red call with a %d-byte id = %q, want the consent flag", len(long), b)
	}
	if lines := steerLines(t, f.st); len(lines) != 1 {
		t.Errorf("%d steer lines, want 1", len(lines))
	}
}
