package harness

import (
	"bufio"
	"os"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/hookio"
	"github.com/altrace-dev-role/rashomon/test/perf/payload"
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
	{
		name:    "claude code desktop surface via claude-desktop",
		raw:     `{"session_id":"s","hook_event_name":"PreToolUse"}`,
		env:     Env{Entrypoint: "claude-desktop", ConfigDir: claudeConfigDir},
		wantID:  ClaudeCode,
		wantSrf: SurfaceDesktop,
	},
	{
		name:    "claude code desktop surface via local-agent",
		raw:     `{"session_id":"s","hook_event_name":"PreToolUse"}`,
		env:     Env{Entrypoint: "local-agent", ConfigDir: claudeConfigDir},
		wantID:  ClaudeCode,
		wantSrf: SurfaceDesktop,
	},
	{
		name:    "claude code cloud surface via claude-code-github-action",
		raw:     `{"session_id":"s","hook_event_name":"PreToolUse"}`,
		env:     Env{Entrypoint: "claude-code-github-action", ConfigDir: claudeConfigDir},
		wantID:  ClaudeCode,
		wantSrf: SurfaceCloud,
	},
	{
		name:    "claude code sdk surface via sdk-cli",
		raw:     `{"session_id":"s","hook_event_name":"PreToolUse"}`,
		env:     Env{Entrypoint: "sdk-cli", ConfigDir: claudeConfigDir},
		wantID:  ClaudeCode,
		wantSrf: SurfaceSDK,
	},
	{
		name:    "claude code sdk surface via sdk-ts",
		raw:     `{"session_id":"s","hook_event_name":"PreToolUse"}`,
		env:     Env{Entrypoint: "sdk-ts", ConfigDir: claudeConfigDir},
		wantID:  ClaudeCode,
		wantSrf: SurfaceSDK,
	},
	{
		// An entrypoint outside the allowlist, with no transcript under
		// <ConfigDir>/projects/ either, must not read as any Claude Code
		// identity: the allowlist check (entrypointSurface's final return)
		// has to actually reject it, not fall through as a match.
		name:    "unlisted entrypoint with no claude transcript is unknown, not claude code",
		raw:     `{"session_id":"s","hook_event_name":"PreToolUse"}`,
		env:     Env{Entrypoint: "some-future-harness", ConfigDir: claudeConfigDir},
		wantID:  Unknown,
		wantSrf: SurfaceUnknown,
	},
	{
		// The same unlisted entrypoint, now with a transcript under
		// <ConfigDir>/projects/, falls through to step 4, presumed -- not
		// step 3, positive, which the unlisted entrypoint must not reach.
		name:    "unlisted entrypoint with a claude transcript is presumed, not positive",
		raw:     `{"session_id":"s","hook_event_name":"PreToolUse","transcript_path":"/home/u/.claude/projects/p1/s.jsonl"}`,
		env:     Env{Entrypoint: "some-future-harness", ConfigDir: claudeConfigDir},
		wantID:  ClaudeCodePresumed,
		wantSrf: SurfaceUnknown,
	},
	{
		// hook_event_name of the wrong JSON type: Detect's fail-closed
		// wrong-type branch (errEvt/errVer/errTP) must fire and return
		// Unknown rather than decide from a malformed shape.
		name:    "hook_event_name of the wrong JSON type is unknown, fail-closed",
		raw:     `{"session_id":"s","hook_event_name":5,"turn_id":"t1"}`,
		env:     Env{},
		wantID:  Unknown,
		wantSrf: SurfaceUnknown,
	},
	{
		name:    "cursor_version of the wrong JSON type is unknown, fail-closed",
		raw:     `{"session_id":"s","hook_event_name":"PreToolUse","cursor_version":1}`,
		env:     Env{},
		wantID:  Unknown,
		wantSrf: SurfaceUnknown,
	},
	{
		name:    "transcript_path of the wrong JSON type is unknown, fail-closed",
		raw:     `{"session_id":"s","hook_event_name":"PreToolUse","transcript_path":5}`,
		env:     Env{},
		wantID:  Unknown,
		wantSrf: SurfaceUnknown,
	},
}

// TestBRD1_CodexTurnIDAcrossAllDocumentedEvents pins turn_id as a Codex
// marker on every event Codex documents it for a tool or turn call (Archive
// 60.2.3, [CDX:481-485]), not just the one event (PreToolUse) brd1Cases
// happens to use. Interrupt is included: the H-110 file this came from
// mutated it alone, and nothing but this loop pinned it.
func TestBRD1_CodexTurnIDAcrossAllDocumentedEvents(t *testing.T) {
	for event := range codexTurnIDEvents {
		t.Run(event, func(t *testing.T) {
			raw := `{"session_id":"s","hook_event_name":"` + event + `","turn_id":"t1"}`
			got := Detect(intake(t, raw), Env{})
			if got.ID != Codex {
				t.Fatalf("Detect() ID = %s on %s, want Codex", got.ID, event)
			}
		})
	}
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

// payloadConfigDir is test/perf/payload's transcript path, one directory up
// from projects/<project>/<session>.jsonl: payload.Session is the session
// id payload.go's transcript constant embeds.
const payloadConfigDir = "/Users/dev/.claude"

// realPayloads is every fixed-shape generator test/perf/payload exports: a
// real captured-shape Claude Code payload, not a hand-written one, so BRD-2
// is checked against the same bytes the perf gates run, not a stand-in for
// them (review finding: hand-written fixtures existed here before this,
// while generators the repo already carries for perf went unused).
func realPayloads(t *testing.T) map[string][]byte {
	t.Helper()
	pre, err := payload.Pre(512, "tu-1")
	if err != nil {
		t.Fatalf("payload.Pre: %v", err)
	}
	post, err := payload.Post(512, "tu-1")
	if err != nil {
		t.Fatalf("payload.Post: %v", err)
	}
	return map[string][]byte{
		"SessionStart": payload.SessionStart(),
		"SessionEnd":   payload.SessionEnd(),
		"Stop":         payload.Stop("done"),
		"Prompt":       payload.Prompt("do the thing"),
		"Pre":          pre,
		"Post":         post,
	}
}

// injectForeignMarkers adds turn_id and model to a generator payload's top
// level, the way an adversarial or buggy foreign client might, without
// hand-building a second JSON literal: raw's trailing brace is replaced
// with the two extra keys plus the brace back.
func injectForeignMarkers(raw []byte) []byte {
	s := strings.TrimSuffix(string(raw), "}")
	return []byte(s + `,"turn_id":"would-be-codex","model":"would-be-codex"}`)
}

// TestBRD2_NoRealGeneratorPayloadReadsAsForeign is requirement row BRD-2
// against test/perf/payload's generators rather than a hand-written
// fixture: every one of them, under an allowlisted entrypoint with its real
// transcript_path, decides claude_code; the same bytes with no entrypoint
// at all still decide claude_code_presumed from the transcript path alone;
// and turn_id/model injected into any of them does not change either
// answer, the same property TestBRD2_NoClaudeDocExampleReadsAsForeign pins
// by hand for one payload shape.
func TestBRD2_NoRealGeneratorPayloadReadsAsForeign(t *testing.T) {
	positive := Env{Entrypoint: "cli", ConfigDir: payloadConfigDir}
	presumed := Env{Entrypoint: "", ConfigDir: payloadConfigDir}
	for name, raw := range realPayloads(t) {
		t.Run(name, func(t *testing.T) {
			if got := Detect(intake(t, string(raw)), positive); got.ID != ClaudeCode {
				t.Fatalf("Detect() ID = %s, want ClaudeCode under an allowlisted entrypoint", got.ID)
			}
			if got := Detect(intake(t, string(raw)), presumed); got.ID != ClaudeCodePresumed {
				t.Fatalf("Detect() ID = %s, want ClaudeCodePresumed with no entrypoint", got.ID)
			}
		})
		t.Run(name+"/with turn_id and model injected", func(t *testing.T) {
			injected := injectForeignMarkers(raw)
			if got := Detect(intake(t, string(injected)), positive); got.ID != ClaudeCode {
				t.Fatalf("Detect() ID = %s, want ClaudeCode despite injected turn_id/model", got.ID)
			}
		})
	}
}

// TestDetect_ClaudeOverrideWinsOverCursorMarkers pins the ordering fact the
// spec states without a named requirement row: the step-3 override is
// checked before step 1 (Cursor) as well as step 2 (Codex) -- Technical
// spec 6.2.2 says "checked before steps 1 and 2"; Archive 60.2.3 says
// "before steps 2 and 3" where its own step 1 is this override, step 2 is
// Cursor. Neither scopes the override to turn_id/model only. An
// allowlisted entrypoint plus a transcript under <ConfigDir>/projects/
// therefore wins even over cursor_version, and the cursor evidence key is
// still recorded, same as turn_id/model in TestBRD2.
func TestDetect_ClaudeOverrideWinsOverCursorMarkers(t *testing.T) {
	raw := `{"session_id":"s","hook_event_name":"PreToolUse","cursor_version":"1.0.0","transcript_path":"/home/u/.claude/projects/p1/s.jsonl"}`
	p := intake(t, raw)
	got := Detect(p, claudeEnv)
	if got.ID != ClaudeCode {
		t.Fatalf("Detect() = %+v, want ClaudeCode under an allowlisted Claude entrypoint with a Claude transcript", got)
	}
	found := false
	for _, e := range got.Evidence {
		if e == EvCursorVersion {
			found = true
		}
	}
	if !found {
		t.Fatalf("Evidence = %v, want it to still carry %s", got.Evidence, EvCursorVersion)
	}
}

// TestDetect_CursorWinsWithoutTheOverride is the negative half: drop either
// half of the override (here, the allowlisted entrypoint) and the same
// cursor_version payload reads as Cursor, as it always has.
func TestDetect_CursorWinsWithoutTheOverride(t *testing.T) {
	raw := `{"session_id":"s","hook_event_name":"PreToolUse","cursor_version":"1.0.0","transcript_path":"/home/u/.claude/projects/p1/s.jsonl"}`
	p := intake(t, raw)
	got := Detect(p, Env{Entrypoint: "", ConfigDir: claudeConfigDir})
	if got.ID != Cursor {
		t.Fatalf("Detect() = %+v, want Cursor with no allowlisted entrypoint", got)
	}
}

// TestDetect_UserEmailPresenceDoesNotChangeTheResult pins the observable
// half of BRD-9's underlying mechanism at the Detect layer: a present
// user_email changes nothing about the result a payload would otherwise
// get. It does not and cannot pin "never decoded" by itself -- that half is
// a structural fact (no hookio.UserEmail call anywhere in this package),
// not a runtime behaviour this test's assertions could fail on; a future
// p.Str(hookio.UserEmail) call that left ID, Surface and evidence count
// unchanged would still pass here.
func TestDetect_UserEmailPresenceDoesNotChangeTheResult(t *testing.T) {
	without := `{"session_id":"s","hook_event_name":"PreToolUse"}`
	with := `{"session_id":"s","hook_event_name":"PreToolUse","user_email":"person@example.com"}`
	a := Detect(intake(t, without), Env{})
	b := Detect(intake(t, with), Env{})
	if a.ID != b.ID || a.Surface != b.Surface || len(a.Evidence) != len(b.Evidence) {
		t.Fatalf("user_email changed the result: without=%+v with=%+v", a, b)
	}
}

// TestBRD1c_VersionCharsetGating pins Version's charset gate: a
// cursor_version outside ^[0-9A-Za-z.+-]{1,32}$ is dropped from the field
// but the Cursor identity (which rests on presence, not value) still holds.
func TestBRD1c_VersionCharsetGating(t *testing.T) {
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

// TestBRD1d_TranscriptPathTraversalNeverCountsAsUnderProjects pins
// underProjects against a path that only textually starts with the
// projects directory: a bare strings.HasPrefix would accept
// "/home/u/.claude/projectsEVIL/x", which is a sibling, not a descendant.
func TestBRD1d_TranscriptPathTraversalNeverCountsAsUnderProjects(t *testing.T) {
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

// TestBRD1b_BlindTracksCaps pins step 7: Blind is every capability in
// Caps(id, surface) that is not "yes".
func TestBRD1b_BlindTracksCaps(t *testing.T) {
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
	if want := []string{string(CapMirrorInBand), string(CapPersonAtPrompt)}; !slices.Equal(sdk.Blind, want) {
		t.Fatalf("Blind = %v, want exactly %v on the sdk surface", sdk.Blind, want)
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

// TestBRD1e_SortedEvidenceDedupesAndSorts is a direct unit test of the helper
// finish relies on: a duplicate entry collapses to one, and the result is
// sorted.
func TestBRD1e_SortedEvidenceDedupesAndSorts(t *testing.T) {
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

// BRD-4 ruling, 2026-10-24: the detector gap is a bug. The Claude family was
// recognised by two signals only, so a real session with neither read as
// Unknown. The rows below admit one more signal each, and each row shows that
// signal ALONE yields the Claude family. They decide ClaudeCodePresumed, never
// ClaudeCode: only an allowlisted entrypoint is positive evidence, and none of
// these widens the step-3 override that lets Claude Code beat a foreign marker.

const sessionUUID = "00000000-0000-4000-8000-000000000001"

type brd4Case struct {
	name         string
	raw          string
	env          Env
	wantID       ID
	wantEvidence []Evidence
}

var brd4Cases = []brd4Case{
	{
		name:         "CLAUDE_PROJECT_DIR alone, no entrypoint, no transcript",
		raw:          `{"session_id":"s","hook_event_name":"PreToolUse"}`,
		env:          Env{ProjectDir: "/work/p"},
		wantID:       ClaudeCodePresumed,
		wantEvidence: []Evidence{EvClaudeProjectDirEnv},
	},
	{
		name:         "projects/<dir>/<uuid>.jsonl alone, config dir elsewhere, no entrypoint",
		raw:          `{"session_id":"` + sessionUUID + `","hook_event_name":"PreToolUse","transcript_path":"/ci/state/projects/-work-p/` + sessionUUID + `.jsonl"}`,
		env:          Env{ConfigDir: claudeConfigDir},
		wantID:       ClaudeCodePresumed,
		wantEvidence: []Evidence{EvClaudeTranscriptShape},
	},
	{
		name:         "the same shape with Windows separators",
		raw:          `{"session_id":"s","hook_event_name":"PreToolUse","transcript_path":"C:\\Users\\u\\.claude\\projects\\C--work-p\\` + sessionUUID + `.jsonl"}`,
		env:          Env{ConfigDir: "/unrelated"},
		wantID:       ClaudeCodePresumed,
		wantEvidence: []Evidence{EvClaudeTranscriptShape},
	},
	{
		name:         "both new signals name themselves",
		raw:          `{"session_id":"s","hook_event_name":"PreToolUse","transcript_path":"/ci/state/projects/-work-p/` + sessionUUID + `.jsonl"}`,
		env:          Env{ProjectDir: "/work/p"},
		wantID:       ClaudeCodePresumed,
		wantEvidence: []Evidence{EvClaudeProjectDirEnv, EvClaudeTranscriptShape},
	},
	{
		name:         "foreign evidence still wins: cursor_version beside CLAUDE_PROJECT_DIR",
		raw:          `{"session_id":"s","hook_event_name":"preToolUse","cursor_version":"1.4.2"}`,
		env:          Env{ProjectDir: "/work/p"},
		wantID:       Cursor,
		wantEvidence: []Evidence{EvCursorVersion},
	},
	{
		name:         "foreign evidence still wins: conversation_id+generation_id beside the transcript shape",
		raw:          `{"hook_event_name":"afterFileEdit","conversation_id":"c","generation_id":"g","transcript_path":"/x/projects/-w/` + sessionUUID + `.jsonl"}`,
		env:          Env{},
		wantID:       Cursor,
		wantEvidence: []Evidence{EvConversationGenID},
	},
	{
		name:         "foreign evidence still wins: Codex turn_id beside CLAUDE_PROJECT_DIR",
		raw:          `{"session_id":"s","hook_event_name":"PreToolUse","turn_id":"t1"}`,
		env:          Env{ProjectDir: "/work/p"},
		wantID:       Codex,
		wantEvidence: []Evidence{EvTurnIDOnToolEvent},
	},
	{
		name:         "foreign evidence still wins: Codex model beside the transcript shape",
		raw:          `{"session_id":"s","hook_event_name":"PostToolUse","model":"gpt-5","transcript_path":"/x/projects/-w/` + sessionUUID + `.jsonl"}`,
		env:          Env{ProjectDir: "/work/p"},
		wantID:       Codex,
		wantEvidence: []Evidence{EvModelOffSessionStart},
	},
	{
		name:         "CLAUDE_PROJECT_DIR does not widen the step-3 override: an allowlisted entrypoint with no Claude transcript still loses to turn_id",
		raw:          `{"session_id":"s","hook_event_name":"PreToolUse","turn_id":"t1"}`,
		env:          Env{Entrypoint: "cli", ConfigDir: claudeConfigDir, ProjectDir: "/work/p"},
		wantID:       Codex,
		wantEvidence: []Evidence{EvTurnIDOnToolEvent},
	},
	{
		name:   "shape needs a non-empty directory between projects and the file",
		raw:    `{"session_id":"s","hook_event_name":"PreToolUse","transcript_path":"/x/projects//` + sessionUUID + `.jsonl"}`,
		env:    Env{},
		wantID: Unknown,
	},
	{
		name:   "an empty CLAUDE_PROJECT_DIR is not a signal",
		raw:    `{"session_id":"s","hook_event_name":"PreToolUse"}`,
		env:    Env{ProjectDir: ""},
		wantID: Unknown,
	},
	{
		name:   "shape needs a uuid file name",
		raw:    `{"session_id":"s","hook_event_name":"PreToolUse","transcript_path":"/x/projects/-w/s.jsonl"}`,
		env:    Env{},
		wantID: Unknown,
	},
	{
		name:   "shape needs .jsonl",
		raw:    `{"session_id":"s","hook_event_name":"PreToolUse","transcript_path":"/x/projects/-w/` + sessionUUID + `.txt"}`,
		env:    Env{},
		wantID: Unknown,
	},
	{
		name:   "shape needs exactly one directory between projects and the file (a Cursor-style agent-transcripts layout is not it)",
		raw:    `{"session_id":"s","hook_event_name":"PreToolUse","transcript_path":"/home/u/.cursor/projects/w/agent-transcripts/` + sessionUUID + `.jsonl"}`,
		env:    Env{},
		wantID: Unknown,
	},
	{
		name:   "shape needs the directory to be named projects",
		raw:    `{"session_id":"s","hook_event_name":"PreToolUse","transcript_path":"/x/sessions/-w/` + sessionUUID + `.jsonl"}`,
		env:    Env{},
		wantID: Unknown,
	},
	{
		name:   "a transcript directly under projects/ has no project directory",
		raw:    `{"session_id":"s","hook_event_name":"PreToolUse","transcript_path":"/x/projects/` + sessionUUID + `.jsonl"}`,
		env:    Env{},
		wantID: Unknown,
	},
}

func TestBRD4_EachAdmittedSignalAloneYieldsTheClaudeFamily(t *testing.T) {
	for _, c := range brd4Cases {
		t.Run(c.name, func(t *testing.T) {
			got := Detect(intake(t, c.raw), c.env)
			if got.ID != c.wantID {
				t.Fatalf("Detect() ID = %s (evidence %v), want %s", got.ID, got.Evidence, c.wantID)
			}
			if len(got.Evidence) != len(c.wantEvidence) {
				t.Fatalf("evidence = %v, want %v", got.Evidence, c.wantEvidence)
			}
			want := append([]Evidence(nil), c.wantEvidence...)
			sort.Slice(want, func(i, j int) bool { return want[i] < want[j] })
			for i := range want {
				if got.Evidence[i] != want[i] {
					t.Fatalf("evidence = %v, want %v", got.Evidence, want)
				}
			}
		})
	}
}

// TestBRD4_CapturedSessionWithNoEntrypointAndATranscriptOutsideTheConfigDirReadsAsClaude
// is the case CI hit: a real 2.1.280 capture (test/fixtures/claude-code),
// replayed with CLAUDE_CODE_ENTRYPOINT unset, a config directory that is not
// the one the capture's transcript_path sits under, and no CLAUDE_PROJECT_DIR.
// Every one of its 25 payloads, SessionStart and SessionEnd included, used to
// read as Unknown. The payload alone now decides the Claude family.
func TestBRD4_CapturedSessionWithNoEntrypointAndATranscriptOutsideTheConfigDirReadsAsClaude(t *testing.T) {
	f, err := os.Open("../../test/fixtures/claude-code/2.1.280/payloads.ndjson")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	n := 0
	for sc.Scan() {
		n++
		raw := sc.Text()
		p := intake(t, raw)
		event, _ := p.Str(hookio.HookEventName)
		got := Detect(p, Env{Entrypoint: "", ConfigDir: "/ci/not-the-capture-config-dir"})
		if got.ID != ClaudeCodePresumed {
			t.Errorf("payload %d (%s): Detect() ID = %s, want %s", n, event, got.ID, ClaudeCodePresumed)
		}
		if len(got.Evidence) != 1 || got.Evidence[0] != EvClaudeTranscriptShape {
			t.Errorf("payload %d (%s): evidence = %v, want [%s]", n, event, got.Evidence, EvClaudeTranscriptShape)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if n != 25 {
		t.Fatalf("the capture holds %d payloads, want 25", n)
	}
}
