package harness

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/hookio"
)

// intake is the test helper every case below uses: it runs the payload
// through the real hookio.Intake scan Detect is built to consume, so a test
// failure here means Detect disagrees with the product's one intake pass,
// not with a test double standing in for it.
func intake(t *testing.T, raw string) *hookio.Payload {
	t.Helper()
	p, err := hookio.Intake(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("hookio.Intake: %v", err)
	}
	return p
}

const claudeConfigDir = "/home/u/.claude"

var claudeEnv = Env{Entrypoint: "cli", ConfigDir: claudeConfigDir}

// brd1Case is one TestBRD1 table row.
type brd1Case struct {
	name    string
	raw     string
	env     Env
	wantID  ID
	wantSrf Surface
}

// brd1Cases is the table TestBRD1_DetectsEachHarnessFromDocumentedPayloads
// runs. It is a package-level var, not a literal inside the test function,
// so the test body itself stays a few lines.
var brd1Cases = []brd1Case{
	{
		name:    "claude code cli, no foreign marker",
		raw:     `{"session_id":"s","hook_event_name":"PreToolUse","tool_name":"Bash","transcript_path":"/home/u/.claude/projects/p1/s.jsonl"}`,
		env:     claudeEnv,
		wantID:  ClaudeCode,
		wantSrf: SurfaceCLI,
	},
	{
		name:    "claude code vscode surface",
		raw:     `{"session_id":"s","hook_event_name":"SessionStart","transcript_path":"/home/u/.claude/projects/p1/s.jsonl"}`,
		env:     Env{Entrypoint: "claude-vscode", ConfigDir: claudeConfigDir},
		wantID:  ClaudeCode,
		wantSrf: SurfaceIDE,
	},
	{
		name:    "claude code sdk surface",
		raw:     `{"session_id":"s","hook_event_name":"PostToolUse"}`,
		env:     Env{Entrypoint: "sdk-py", ConfigDir: claudeConfigDir},
		wantID:  ClaudeCode,
		wantSrf: SurfaceSDK,
	},
	{
		name:    "claude code remote prefix surface",
		raw:     `{"session_id":"s","hook_event_name":"PreToolUse"}`,
		env:     Env{Entrypoint: "remote-ci-runner", ConfigDir: claudeConfigDir},
		wantID:  ClaudeCode,
		wantSrf: SurfaceCloud,
	},
	{
		name:    "claude code presumed, no entrypoint",
		raw:     `{"session_id":"s","hook_event_name":"PreToolUse","transcript_path":"/home/u/.claude/projects/p1/s.jsonl"}`,
		env:     Env{Entrypoint: "", ConfigDir: claudeConfigDir},
		wantID:  ClaudeCodePresumed,
		wantSrf: SurfaceUnknown,
	},
	{
		name:    "cursor by cursor_version",
		raw:     `{"session_id":"s","hook_event_name":"preToolUse","cursor_version":"1.4.2"}`,
		env:     Env{},
		wantID:  Cursor,
		wantSrf: SurfaceUnknown,
	},
	{
		name:    "cursor by conversation_id+generation_id, no cursor_version",
		raw:     `{"session_id":"s","hook_event_name":"afterFileEdit","conversation_id":"c1","generation_id":"g1"}`,
		env:     Env{},
		wantID:  Cursor,
		wantSrf: SurfaceUnknown,
	},
	{
		name:    "codex by turn_id on PreToolUse",
		raw:     `{"session_id":"s","hook_event_name":"PreToolUse","turn_id":"t1"}`,
		env:     Env{},
		wantID:  Codex,
		wantSrf: SurfaceUnknown,
	},
	{
		name:    "codex by model off SessionStart",
		raw:     `{"session_id":"s","hook_event_name":"PostToolUse","model":"gpt-5"}`,
		env:     Env{},
		wantID:  Codex,
		wantSrf: SurfaceUnknown,
	},
	{
		name:    "model on SessionStart is not a Codex marker (Claude Code documents it there)",
		raw:     `{"session_id":"s","hook_event_name":"SessionStart","model":"claude"}`,
		env:     Env{},
		wantID:  Unknown,
		wantSrf: SurfaceUnknown,
	},
	{
		name:    "turn_id on MessageDisplay is not a Codex marker (Claude Code documents it there too)",
		raw:     `{"session_id":"s","hook_event_name":"MessageDisplay","turn_id":"t1"}`,
		env:     Env{},
		wantID:  Unknown,
		wantSrf: SurfaceUnknown,
	},
	{
		// The Claude Code override needs BOTH the allowlisted entrypoint AND
		// the transcript under <ConfigDir>/projects/. An allowlisted
		// entrypoint with no transcript_path at all must not grant the
		// override: this is still Codex.
		name:    "allowlisted entrypoint without a claude transcript does not override turn_id",
		raw:     `{"session_id":"s","hook_event_name":"PreToolUse","turn_id":"t1"}`,
		env:     claudeEnv,
		wantID:  Codex,
		wantSrf: SurfaceUnknown,
	},
	{
		name:    "no marker, no allowlisted entrypoint, no claude transcript",
		raw:     `{"session_id":"s","hook_event_name":"PreToolUse"}`,
		env:     Env{},
		wantID:  Unknown,
		wantSrf: SurfaceUnknown,
	},
	{
		name:    "null and false markers do not count as present",
		raw:     `{"session_id":"s","hook_event_name":"PreToolUse","turn_id":null,"model":false}`,
		env:     Env{},
		wantID:  Unknown,
		wantSrf: SurfaceUnknown,
	},
}

// TestBRD1_DetectsEachHarnessFromDocumentedPayloads is the acceptance
// requirement row BRD-1: Detect returns cursor, codex, claude_code,
// claude_code_presumed or unknown per 6.2.2/60.2.3, from key presence and
// env only. The payload shapes in brd1Cases are hand-written: this tree
// carries no captured Cursor or Codex session, and no [CUR]/[CDX]/[CC] doc
// snapshot is checked in here, so these follow the field names and the
// PreToolUse/PostToolUse shape internal/hook's own tests already use for a
// real Claude Code payload (internal/hook/rec31_test.go:127), extended with
// the Cursor/Codex-only fields the spec names.
func TestBRD1_DetectsEachHarnessFromDocumentedPayloads(t *testing.T) {
	for _, c := range brd1Cases {
		t.Run(c.name, func(t *testing.T) { runBRD1Case(t, c) })
	}
}

func runBRD1Case(t *testing.T, c brd1Case) {
	t.Helper()
	p := intake(t, c.raw)
	got := Detect(p, c.env)
	if got.ID != c.wantID || got.Surface != c.wantSrf {
		t.Fatalf("Detect() = {ID:%s Surface:%s}, want {ID:%s Surface:%s}",
			got.ID, got.Surface, c.wantID, c.wantSrf)
	}
}

// TestBRD2_NoClaudeDocExampleReadsAsForeign is requirement row BRD-2: no
// documented Claude Code payload reads as foreign, nor one with turn_id or
// model added when the entrypoint is allowlisted and the transcript is
// under <ConfigDir>/projects/.
func TestBRD2_NoClaudeDocExampleReadsAsForeign(t *testing.T) {
	// A Claude Code release that starts sending turn_id on PreToolUse, or
	// model on PostToolUse, must not stand existing Claude Code entries
	// down. This is the one case the spec calls out by name (6.2.2 step 2's
	// exception) as the reason the override exists.
	raw := `{"session_id":"s","hook_event_name":"PreToolUse","turn_id":"would-be-codex","model":"would-be-codex","transcript_path":"/home/u/.claude/projects/p1/s.jsonl"}`
	p := intake(t, raw)
	got := Detect(p, claudeEnv)
	if got.ID != ClaudeCode {
		t.Fatalf("Detect() = %+v, want ClaudeCode despite turn_id/model", got)
	}
	wantEvidence := map[Evidence]bool{
		EvEntrypointEnv:        true,
		EvClaudeTranscript:     true,
		EvTurnIDOnToolEvent:    true,
		EvModelOffSessionStart: true,
	}
	if len(got.Evidence) != len(wantEvidence) {
		t.Fatalf("evidence = %v, want exactly %v", got.Evidence, wantEvidence)
	}
	for _, e := range got.Evidence {
		if !wantEvidence[e] {
			t.Fatalf("unexpected evidence %q in %v", e, got.Evidence)
		}
	}
}

// TestDetect_CursorWinsOverClaudeOverride pins the one ordering fact the
// spec states without a named requirement row: the Claude Code override
// (6.2.2 step 2) names only turn_id and model. It does not exempt Cursor's
// markers, because Claude Code never emits a field named cursor_version.
func TestDetect_CursorWinsOverClaudeOverride(t *testing.T) {
	raw := `{"session_id":"s","hook_event_name":"PreToolUse","cursor_version":"1.0.0","transcript_path":"/home/u/.claude/projects/p1/s.jsonl"}`
	p := intake(t, raw)
	got := Detect(p, claudeEnv)
	if got.ID != Cursor {
		t.Fatalf("Detect() = %+v, want Cursor even under an allowlisted Claude entrypoint", got)
	}
}

// TestDetect_UserEmailNeverDecoded pins BRD-9's underlying mechanism at the
// Detect layer: user_email is read as a presence bit (it must not change
// the identity decision's evidence) and its value is never read, by
// construction -- there is no hookio.UserEmail call anywhere in this
// package. This test pins the observable half: a present user_email changes
// nothing about the result a payload would otherwise get.
func TestDetect_UserEmailNeverDecoded(t *testing.T) {
	without := `{"session_id":"s","hook_event_name":"PreToolUse"}`
	with := `{"session_id":"s","hook_event_name":"PreToolUse","user_email":"person@example.com"}`
	a := Detect(intake(t, without), Env{})
	b := Detect(intake(t, with), Env{})
	if a.ID != b.ID || a.Surface != b.Surface || len(a.Evidence) != len(b.Evidence) {
		t.Fatalf("user_email changed the result: without=%+v with=%+v", a, b)
	}
}

// TestDetect_VersionCharsetGating pins Version's charset gate: a
// cursor_version outside ^[0-9A-Za-z.+-]{1,32}$ is dropped from the field
// but the Cursor identity (which rests on presence, not value) still holds.
func TestDetect_VersionCharsetGating(t *testing.T) {
	cases := []struct {
		name string
		v    string
		want *string
	}{
		{"valid semver-ish", `"1.4.2-beta.1"`, strPtr("1.4.2-beta.1")},
		{"too long", `"` + strings.Repeat("9", 33) + `"`, nil},
		{"bad charset", `"1.4.2 (build)"`, nil},
		{"a space alone is outside the charset", `"1.4.2 beta"`, nil},
		{"empty string", `""`, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			raw := `{"session_id":"s","hook_event_name":"preToolUse","cursor_version":` + c.v + `}`
			got := Detect(intake(t, raw), Env{})
			if got.ID != Cursor {
				t.Fatalf("Detect() ID = %s, want Cursor", got.ID)
			}
			if !strPtrEqual(got.Version, c.want) {
				t.Fatalf("Version = %v, want %v", strDeref(got.Version), strDeref(c.want))
			}
		})
	}
}

// TestDetect_TranscriptPathTraversalNeverCountsAsUnderProjects pins
// underProjects against a path that only textually starts with the
// projects directory: a bare strings.HasPrefix would accept
// "/home/u/.claude/projectsEVIL/x", which is a sibling, not a descendant.
func TestDetect_TranscriptPathTraversalNeverCountsAsUnderProjects(t *testing.T) {
	cases := []struct {
		name string
		path string
		want bool
	}{
		{"real descendant", "/home/u/.claude/projects/p1/s.jsonl", true},
		{"sibling with shared prefix", "/home/u/.claude/projectsEVIL/p1/s.jsonl", false},
		{"the directory itself, not a file in it", "/home/u/.claude/projects", false},
		{"escapes via dot-dot", "/home/u/.claude/projects/../escaped/s.jsonl", false},
		{"empty", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := underProjects(c.path, claudeConfigDir); got != c.want {
				t.Fatalf("underProjects(%q, %q) = %v, want %v", c.path, claudeConfigDir, got, c.want)
			}
		})
	}
}

// TestDetect_NilPayloadIsUnknown pins the fail-closed shape for a Payload
// Detect was never handed: Unknown, not a panic, not a guess.
func TestDetect_NilPayloadIsUnknown(t *testing.T) {
	got := Detect(nil, claudeEnv)
	if got.ID != Unknown {
		t.Fatalf("Detect(nil, ...) = %+v, want Unknown", got)
	}
}

// TestDetect_BlindTracksCaps pins step 7: Blind is every capability in
// Caps(id, surface) that is not "yes".
func TestDetect_BlindTracksCaps(t *testing.T) {
	got := Detect(intake(t, `{"session_id":"s","hook_event_name":"preToolUse","cursor_version":"1.0"}`), Env{})
	if got.ID != Cursor {
		t.Fatalf("setup: ID = %s, want Cursor", got.ID)
	}
	caps := Caps(Cursor, SurfaceUnknown)
	if len(got.Blind) != len(caps) {
		t.Fatalf("Blind has %d entries, Caps(cursor, unknown) has %d; cursor has no adapter, every capability should be blind", len(got.Blind), len(caps))
	}
	for c, state := range caps {
		if state == Yes {
			t.Fatalf("Caps(cursor, unknown)[%s] = yes; cursor has no adapter in this tree", c)
		}
	}

	sdk := Detect(intake(t, `{"session_id":"s","hook_event_name":"PreToolUse"}`), Env{Entrypoint: "sdk-ts", ConfigDir: claudeConfigDir})
	if sdk.ID != ClaudeCode || sdk.Surface != SurfaceSDK {
		t.Fatalf("setup: sdk = %+v", sdk)
	}
	if len(sdk.Blind) != 1 || sdk.Blind[0] != string(CapMirrorInBand) {
		t.Fatalf("Blind = %v, want exactly [%s] on the sdk surface", sdk.Blind, CapMirrorInBand)
	}
}

// TestDetect_CursorEvidenceIsSortedWhenBothMarkersFire pins the ordering:
// EvConversationGenID ("conversation_id+generation_id") sorts before
// EvCursorVersion ("cursor_version") when a payload carries both markers.
func TestDetect_CursorEvidenceIsSortedWhenBothMarkersFire(t *testing.T) {
	raw := `{"session_id":"s","hook_event_name":"preToolUse","cursor_version":"1.0.0","conversation_id":"c1","generation_id":"g1"}`
	got := Detect(intake(t, raw), Env{})
	want := []Evidence{EvConversationGenID, EvCursorVersion}
	if len(got.Evidence) != len(want) || got.Evidence[0] != want[0] || got.Evidence[1] != want[1] {
		t.Fatalf("Evidence = %v, want %v in that order", got.Evidence, want)
	}
}

// TestSortedEvidence_DedupesAndSorts is a direct unit test of the helper
// finish relies on: a duplicate entry collapses to one, and the result is
// sorted.
func TestSortedEvidence_DedupesAndSorts(t *testing.T) {
	got := sortedEvidence([]Evidence{EvCursorVersion, EvEntrypointEnv, EvCursorVersion})
	want := []Evidence{EvEntrypointEnv, EvCursorVersion}
	if len(got) != len(want) {
		t.Fatalf("sortedEvidence(...) = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sortedEvidence(...) = %v, want %v", got, want)
		}
	}
}

func strPtr(s string) *string { return &s }

func strDeref(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}

func strPtrEqual(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
