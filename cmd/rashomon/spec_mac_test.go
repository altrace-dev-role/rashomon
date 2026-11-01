package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/digest"
	"github.com/altrace-dev-role/rashomon/internal/harness"
	"github.com/altrace-dev-role/rashomon/internal/hook"
	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/selfguard"
	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// signedMAC is the bindings file's mac field as EncodeBindings writes it.
var signedMAC = regexp.MustCompile(`,"mac":"[0-9a-f]{64}"}\n$`)

// unsignedLine is the verb's report of a bindings file it did not sign.
const unsignedLine = "spec-trust.json does not carry this store's MAC, so it trusts nothing: something other than rashomon spec trust wrote it\n"

// readKey is the store's trust key, failing the test unless it is 32 bytes
// at mode 0600.
func (p specProject) readKey() []byte {
	p.t.Helper()
	path := filepath.Join(p.root, spec.TrustKeyFile)
	key, err := os.ReadFile(path)
	if err != nil {
		p.t.Fatal(err)
	}
	if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 || len(key) != spec.TrustKeyBytes {
		p.t.Fatalf("the trust key: %v, %v, %d bytes; want %d bytes at mode 0600", fi, err, len(key), spec.TrustKeyBytes)
	}
	return key
}

// TestST1n_TheVerbsOwnBindingReadsTrusted (D173): `spec trust` writes the
// bindings signed under the store's trust key, which it makes on the first
// trust (32 bytes, 0600) and reuses after: discovery reads the spec as
// trusted, and a second trust, of the edited file, keeps the key. A key file
// of another length is no key: the bindings read unsigned, and the next
// trust makes a new key and signs under it.
func TestST1n_TheVerbsOwnBindingReadsTrusted(t *testing.T) {
	p := newSpecProject(t, specUnit)
	atSpecTerminal(t, &person{answer: typedPrefix(t, specUnit)}, nil)
	if code, out, errOut := specRun(t, "", "trust"); code != 0 {
		t.Fatalf("trust: exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	data, err := os.ReadFile(filepath.Join(p.root, spec.TrustFile))
	if err != nil {
		t.Fatal(err)
	}
	if !signedMAC.Match(data) {
		t.Errorf("the bindings file %s carries no mac", data)
	}
	key := p.readKey()
	if tr := p.trust(); tr != spec.Trusted {
		t.Errorf("discovery after trust: %q, want trusted", tr)
	}
	p.edit(specVet)
	atSpecTerminal(t, &person{answer: typedPrefix(t, specVet)}, nil)
	if code, out, errOut := specRun(t, "", "trust"); code != 0 {
		t.Fatalf("trust of the edit: exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	if again := p.readKey(); !bytes.Equal(again, key) {
		t.Error("the second trust made a new key")
	}
	if tr := p.trust(); tr != spec.Trusted {
		t.Errorf("discovery after the second trust: %q, want trusted", tr)
	}
	if err := os.WriteFile(filepath.Join(p.root, spec.TrustKeyFile), key[:spec.TrustKeyBytes-1], 0o600); err != nil {
		t.Fatal(err)
	}
	if tr := p.trust(); tr != spec.Untrusted {
		t.Errorf("discovery under a %d-byte key: %q, want untrusted", spec.TrustKeyBytes-1, tr)
	}
	atSpecTerminal(t, &person{answer: typedPrefix(t, specVet)}, nil)
	if code, out, errOut := specRun(t, "", "trust"); code != 0 || !strings.HasPrefix(out, unsignedLine+"trusted "+p.path) {
		t.Fatalf("trust under a %d-byte key: exit %d, stdout %q, stderr %q", spec.TrustKeyBytes-1, code, out, errOut)
	}
	if again := p.readKey(); bytes.Equal(again, key) {
		t.Error("the trust over a short key kept the old key")
	}
	if tr := p.trust(); tr != spec.Trusted {
		t.Errorf("discovery after the trust over a short key: %q, want trusted", tr)
	}
}

// TestST1o_TrustRefusesAnAgentSession (D173): under the agent signal watch
// refuses on (selfguard.AgentSignal), `spec trust` exits 1 with the reason
// on stderr, the CLI's refusal channel, saying nothing was written as
// watch's refusal does, and writes nothing: no bindings file and no key.
// `spec untrust` is refused there too (TestAGSU1_), since an untrusted spec
// stops nudging the agent; in a terminal it removes that one binding and
// re-signs the rest, which still verify under the same key.
func TestST1o_TrustRefusesAnAgentSession(t *testing.T) {
	p := newSpecProject(t, specUnit)
	agent := &person{answer: typedPrefix(t, specUnit)}
	atSpecTerminal(t, agent, nil)
	t.Setenv(selfguard.EnvClaudeCode, "1")
	if code, out, errOut := specRun(t, "", "trust"); code != 1 || out != "" || !strings.Contains(errOut, "agent session; nothing was written") || agent.read {
		t.Errorf("trust under an agent signal: exit %d, stdout %q, stderr %q, terminal read %v", code, out, errOut, agent.read)
	}
	for _, f := range []string{spec.TrustFile, spec.TrustKeyFile} {
		if _, err := os.Lstat(filepath.Join(p.root, f)); err == nil {
			t.Errorf("trust under an agent signal wrote %s", f)
		}
	}

	t.Setenv(selfguard.EnvClaudeCode, "")
	atSpecTerminal(t, &person{answer: typedPrefix(t, specUnit)}, nil)
	if code, out, errOut := specRun(t, "", "trust"); code != 0 {
		t.Fatalf("trust: exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	b, err := spec.ReadBindings(spec.OSFS{}, p.root)
	if err != nil {
		t.Fatal(err)
	}
	other := spec.Binding{ProjectRoot: "/elsewhere", SpecPath: "/elsewhere/.claude/rashomon/spec.json", SpecDigest: digestOf(t, specVet), BoundAtMS: 1}
	b.Bind(other)
	if err := hook.WriteSpecBindings(p.root, b); err != nil {
		t.Fatal(err)
	}
	key := p.readKey()
	if code, out, errOut := specRun(t, "", "untrust"); code != 0 || !strings.HasPrefix(out, "untrusted "+p.path) {
		t.Fatalf("untrust: exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	if got := p.bindings(); len(got) != 1 || got[0] != other {
		t.Errorf("bindings after untrust %+v; want the other project's alone", got)
	}
	if again := p.readKey(); !bytes.Equal(again, key) {
		t.Error("untrust made a new key")
	}
}

// TestST1p_TheVerbReportsAnUnsignedFile (D173): a bindings file the verb did
// not sign trusts nothing, and the verb says so on stdout before its own
// line: untrust then finds nothing trusted and writes nothing, and trust
// replaces the file with one signed binding.
func TestST1p_TheVerbReportsAnUnsignedFile(t *testing.T) {
	p := newSpecProject(t, specUnit)
	forged := []byte(`{"bindings":[{"project_root":"` + p.dir + `","spec_path":"` + p.path + `","spec_digest":"` +
		digestOf(t, specUnit) + `","bound_at_unix_ms":1}]}` + "\n")
	path := filepath.Join(p.root, spec.TrustFile)
	if err := os.WriteFile(path, forged, 0o600); err != nil {
		t.Fatal(err)
	}
	if tr := p.trust(); tr != spec.Untrusted {
		t.Errorf("discovery over the unsigned file: %q, want untrusted", tr)
	}
	if code, out, errOut := specRun(t, "", "untrust"); code != 0 || out != unsignedLine+p.path+" was not trusted\n" {
		t.Errorf("untrust: exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	if data, err := os.ReadFile(path); err != nil || !bytes.Equal(data, forged) {
		t.Errorf("untrust rewrote the unsigned file: %q, %v", data, err)
	}
	atSpecTerminal(t, &person{answer: typedPrefix(t, specUnit)}, nil)
	if code, out, errOut := specRun(t, "", "trust"); code != 0 || !strings.HasPrefix(out, unsignedLine+"trusted "+p.path) {
		t.Fatalf("trust: exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	if got := p.bindings(); len(got) != 1 || got[0].SpecPath != p.path {
		t.Errorf("bindings after trust %+v; want the one signed binding", got)
	}
	if tr := p.trust(); tr != spec.Trusted {
		t.Errorf("discovery after trust: %q, want trusted", tr)
	}
}

// TestST1m_TheStopReadsTheStoreWriteBeforeTrusting (D173): the Stop reads a
// bound spec as trusted until the session writes under the store root, and
// from that write on as untrusted, the bindings file unchanged; its view's
// integrity names the write as store_touched at the path the call named.
// Each call lands whole (PreToolUse and PostToolUse, under verified
// coverage): integrity reads only a complete record.
func TestST1m_TheStopReadsTheStoreWriteBeforeTrusting(t *testing.T) {
	t.Setenv("CLAUDE_PROJECT_DIR", "")
	t.Setenv("RASHOMON_MANAGED_SETTINGS_PATH", filepath.Join(t.TempDir(), "managed-settings.json"))
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(base, ".claude", "rashomon", "spec.json")
	for _, dir := range []string{filepath.Join(base, ".git"), filepath.Dir(path)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(path, []byte(specUnit), 0o644); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	st, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.WriteRecordScopeMarker(root, store.RecordScopeV5); err != nil {
		t.Fatal(err)
	}
	bound := spec.Binding{ProjectRoot: hook.SpecProject(path), SpecPath: path, SpecDigest: digestOf(t, specUnit), BoundAtMS: 1}
	if err := hook.WriteSpecBindings(root, &spec.Bindings{Bindings: []spec.Binding{bound}}); err != nil {
		t.Fatal(err)
	}
	const session = "sess-st1m"
	now := time.Date(2026, 10, 29, 18, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	if err := st.AppendCoverage(store.Coverage{Type: store.TypeCoverage, SchemaVersion: store.SessionSchemaVersion,
		RecordedAtMS: now.UnixMilli(), SessionID: session, Phase: store.PhasePost, State: store.StateVerified,
		HookEntry: store.EntryPresentSettings, Probe: store.ProbeFresh}); err != nil {
		t.Fatal(err)
	}
	land := func(id, tool string, input map[string]any) {
		t.Helper()
		for _, event := range []string{"PreToolUse", "PostToolUse"} {
			b, err := json.Marshal(map[string]any{"session_id": session, "tool_use_id": id, "prompt_id": "p1",
				"transcript_path": "/tmp/t.jsonl", "permission_mode": "default", "tool_name": tool, "cwd": base,
				"hook_event_name": event, "tool_input": input, "tool_response": map[string]any{"stdout": ""}})
			if err != nil {
				t.Fatal(err)
			}
			if event == "PreToolUse" {
				h := hook.New(st, clock)
				h.Close(&hook.Signals{}, h.Capture(bytes.NewReader(b)))
			} else if err := hook.NewPost(st, clock).Capture(bytes.NewReader(b)); err != nil {
				t.Fatalf("post %s: %v", id, err)
			}
		}
	}
	stop := func() verify.StopView {
		t.Helper()
		run, err := digest.ReadRun(st, session)
		if err != nil {
			t.Fatal(err)
		}
		d, err := digest.BuildFrom(run, st, session, "", "done", now)
		if err != nil {
			t.Fatal(err)
		}
		p := recapPayload{SessionID: session, LastAssistantMessage: "done",
			Harness: harnessSignal{HookEventName: install.EventStop}, OK: true, CWD: base}
		turn, err := stopView(st, run, d, p, false, now)
		if err != nil {
			t.Fatal(err)
		}
		return turn.view
	}

	// The prompt's note reads the spec the same way (D203).
	preflight := func() spec.Trust {
		t.Helper()
		run, err := digest.ReadRun(st, session)
		if err != nil {
			t.Fatal(err)
		}
		if sp := preflightSpec(st, run, base, "", ""); sp != nil {
			return sp.Trust
		}
		return ""
	}

	land("toolu_r", "Read", map[string]any{"file_path": path})
	if v := stop(); v.Snap.Spec == nil || v.Snap.Spec.Trust != spec.Trusted {
		t.Fatalf("the Stop before the write: spec %+v, want trusted", v.Snap.Spec)
	}
	if got := preflight(); got != spec.Trusted {
		t.Fatalf("the preflight before the write: trust %q, want trusted", got)
	}
	trustFile := filepath.Join(root, spec.TrustFile)
	land("toolu_w", "Write", map[string]any{"file_path": trustFile, "content": "{}"})
	v := stop()
	if v.Snap.Spec == nil || v.Snap.Spec.Trust != spec.Untrusted {
		t.Errorf("the Stop after the write: spec %+v, want untrusted", v.Snap.Spec)
	}
	if got := preflight(); got != spec.Untrusted {
		t.Errorf("the preflight after the write: trust %q, want untrusted", got)
	}
	var named bool
	for _, e := range v.Snap.Integrity.Unresolved {
		named = named || e.Check == verify.StoreTouched && e.Path == filepath.ToSlash(trustFile)
	}
	if !named {
		t.Errorf("the Stop's integrity %+v; want store_touched at %s", v.Snap.Integrity.Unresolved, trustFile)
	}
}

// TestST1x_ThePromptsNoteReadsTheStoreWriteBeforeTrusting (D203): the
// prompt's note reads a bound spec through preflightSpec, so it names the
// spec's gate as trusted until the session writes under the store root, and
// from that write on names no gate and says the spec is untrusted. TestST1m
// pins preflightSpec; this pins that the note reads it. Break: discover the
// note's spec with the store root directly (hook.DiscoverSpec with
// st.Root()) and the note after the write still names the gate as trusted.
//
// Two traps in the fixture; the first hides the break, the second fakes one:
//   - The prompts name different files. A note with nothing new is not
//     given, so a second prompt naming pkg/a.go again gets no note at all,
//     and an empty note passes the after-write check with or without the
//     break.
//   - The prompts carry different prompt ids. Route's budget gives a key one
//     note a prompt (charge's budget_pattern), so under a shared id the
//     second note is suppressed and the after-write check fails with or
//     without the break.
func TestST1x_ThePromptsNoteReadsTheStoreWriteBeforeTrusting(t *testing.T) {
	t.Setenv("CLAUDE_PROJECT_DIR", "")
	t.Setenv("RASHOMON_MANAGED_SETTINGS_PATH", filepath.Join(t.TempDir(), "managed-settings.json"))
	// CI makes the session unattended, which withholds every note at the
	// prompt, and a CI runner sets it; the holdout draw could withhold it too.
	t.Setenv("CI", "")
	t.Setenv("AI_AGENT", "")
	t.Setenv(hook.EnvHoldout, "off")
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(base, ".claude", "rashomon", "spec.json")
	for _, dir := range []string{filepath.Join(base, ".git"), filepath.Dir(path), filepath.Join(base, "pkg")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{path: specUnit, filepath.Join(base, "pkg", "a.go"): "package pkg\n",
		filepath.Join(base, "pkg", "b.go"): "package pkg\n"}
	for name, body := range files {
		if err := os.WriteFile(name, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	root := t.TempDir()
	st, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.WriteRecordScopeMarker(root, store.RecordScopeV5); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteSteerDisclosureMarker(root, steer.DisclosureVersion); err != nil {
		t.Fatal(err)
	}
	bound := spec.Binding{ProjectRoot: hook.SpecProject(path), SpecPath: path, SpecDigest: digestOf(t, specUnit), BoundAtMS: 1}
	if err := hook.WriteSpecBindings(root, &spec.Bindings{Bindings: []spec.Binding{bound}}); err != nil {
		t.Fatal(err)
	}
	const session = "sess-st1x"
	now := time.Date(2026, 10, 30, 1, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	if err := st.AppendCoverage(store.Coverage{Type: store.TypeCoverage, SchemaVersion: store.SessionSchemaVersion,
		RecordedAtMS: now.UnixMilli(), SessionID: session, Phase: store.PhasePost, State: store.StateVerified,
		HookEntry: store.EntryPresentSettings, Probe: store.ProbeFresh}); err != nil {
		t.Fatal(err)
	}
	land := func(id, tool string, input map[string]any) {
		t.Helper()
		for _, event := range []string{"PreToolUse", "PostToolUse"} {
			b, err := json.Marshal(map[string]any{"session_id": session, "tool_use_id": id, "prompt_id": "p1",
				"transcript_path": "/tmp/t.jsonl", "permission_mode": "default", "tool_name": tool, "cwd": base,
				"hook_event_name": event, "tool_input": input, "tool_response": map[string]any{"stdout": ""}})
			if err != nil {
				t.Fatal(err)
			}
			if event == "PreToolUse" {
				h := hook.New(st, clock)
				h.Close(&hook.Signals{}, h.Capture(bytes.NewReader(b)))
			} else if err := hook.NewPost(st, clock).Capture(bytes.NewReader(b)); err != nil {
				t.Fatalf("post %s: %v", id, err)
			}
		}
	}
	// note is the note at prompt promptID as recapTurn routes it, flushed as
	// printTurn flushes it, so the next prompt reads what this one delivered.
	note := func(promptID, prompt string) string {
		t.Helper()
		run, err := digest.ReadRun(st, session)
		if err != nil {
			t.Fatal(err)
		}
		p := pendingPayload{recapPayload: recapPayload{SessionID: session, CWD: base}, Prompt: []byte(prompt),
			PromptID: promptID, PermissionMode: "default"}
		id := harness.Identity{ID: harness.ClaudeCode, Surface: harness.SurfaceCLI}
		out, _ := preflightNote(st, run, &p, promptProject{}, id, now)
		if err := st.Flush(); err != nil {
			t.Fatal(err)
		}
		return out.AdditionalContext + "\n" + out.SystemMessage
	}

	land("toolu_r", "Read", map[string]any{"file_path": path})
	if got := note("p2", "fix pkg/a.go"); !strings.Contains(got, "'go test' (trusted)") {
		t.Fatalf("the note before the write %q; want the spec's gate named, trusted", got)
	}
	land("toolu_w", "Write", map[string]any{"file_path": filepath.Join(root, spec.TrustFile), "content": "{}"})
	if got := note("p3", "fix pkg/b.go"); strings.Contains(got, "'go test'") || !strings.Contains(got, "spec present, untrusted") {
		t.Errorf("the note after the write %q; want no gate and the spec named untrusted", got)
	}
}
