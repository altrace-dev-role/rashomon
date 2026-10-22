package harness

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/hookio"
)

// Detect decides an Identity from one already-scanned intake payload (RES-5:
// hookio.Intake's single jsonscan.Top pass) and a narrow env view. It reads
// key PRESENCE (hookio.Payload.Has, which is exactly "neither absent, null
// nor false" -- jsonscan.Span.Set's own doc names this as the bit harness
// detection reads) plus the two string VALUES hook_event_name and
// cursor_version. user_email is read as a presence bit only: p.Str(UserEmail)
// is never called, so the email never becomes a Go string here or anywhere
// downstream of this function. CURSOR_* environment variables are never
// read: a person's own Cursor terminal sets them too, and they would make a
// Claude Code session run inside one read as foreign.
//
// Order matches Technical spec 6.2.2 / Archive 60.2.3 exactly: the override
// in step 3 is checked before BOTH step 1 and step 2, not just step 2 --
// both the spec ("checked before steps 1 and 2") and the archive ("before
// steps 2 and 3", where its own step 1 is this override) place it ahead of
// Cursor's check, not only Codex's. Claude Code has never been observed to
// emit cursor_version, so this ordering has not changed any payload this
// package has been shown in practice, but a payload that does carry it
// alongside an allowlisted entrypoint and a Claude transcript follows the
// documented order rather than a narrower one this file used to carry.
//
//  1. Cursor: cursor_version present, or conversation_id and generation_id
//     both present. Subject to the same step-3 override as step 2: the
//     marker's evidence key is still recorded.
//  2. Codex: turn_id present on an event where Codex documents it on a tool
//     or turn call, or model present on any event but SessionStart.
//     EXCEPT: a payload whose Env.Entrypoint is in step 3's allowlist and
//     whose transcript_path resolves under Env.ConfigDir/projects/ stays
//     Claude Code even when one of these appears, so a field a future
//     Claude Code release adds to a subscribed event cannot stand every
//     Claude Code entry down. The marker's evidence key is still recorded.
//  3. Claude Code, positive: Env.Entrypoint is in the 2.1.280 allowlist.
//     Surface comes from the entrypoint.
//  4. Claude Code, presumed: no foreign marker (step 1 or 2 would otherwise
//     have fired) and transcript_path resolves under
//     Env.ConfigDir/projects/.
//  5. Otherwise: unknown.
//
// A payload hookio.Intake could not scan, or whose hook_event_name,
// cursor_version or transcript_path is present with a value of the wrong
// JSON type, decides nothing: Detect returns Unknown rather than guess from
// a malformed shape it has never been shown.
func Detect(p *hookio.Payload, env Env) Identity {
	if p == nil {
		return unknown(nil)
	}

	hookEventName, errEvt := p.Str(hookio.HookEventName)
	version, errVer := p.Str(hookio.CursorVersion)
	transcriptPath, errTP := p.Str(hookio.TranscriptPath)
	if errEvt != nil || errVer != nil || errTP != nil {
		return unknown(nil)
	}

	hasCursorVersion := p.Has(hookio.CursorVersion)
	hasConvGen := p.Has(hookio.ConversationID) && p.Has(hookio.GenerationID)
	hasTurnID := p.Has(hookio.TurnID)
	hasModel := p.Has(hookio.Model)

	versionPtr := checkedVersion(version, hasCursorVersion)
	transcriptUnderClaude := underProjects(transcriptPath, env.ConfigDir)
	allowedSurface, isAllowedEntrypoint := entrypointSurface(env.Entrypoint)
	claudeOverride := isAllowedEntrypoint && transcriptUnderClaude

	var evidence []Evidence

	// Step 1: Cursor. Same override as step 2: see the function doc.
	if hasCursorVersion {
		evidence = append(evidence, EvCursorVersion)
	}
	if hasConvGen {
		evidence = append(evidence, EvConversationGenID)
	}
	if (hasCursorVersion || hasConvGen) && !claudeOverride {
		return finish(Cursor, SurfaceUnknown, versionPtr, evidence)
	}

	// Step 2: Codex, with the Claude Code exception.
	turnIDMatch := hasTurnID && codexTurnIDEvents[hookEventName]
	modelMatch := hasModel && hookEventName != "SessionStart"
	if turnIDMatch {
		evidence = append(evidence, EvTurnIDOnToolEvent)
	}
	if modelMatch {
		evidence = append(evidence, EvModelOffSessionStart)
	}
	if (turnIDMatch || modelMatch) && !claudeOverride {
		return finish(Codex, SurfaceUnknown, versionPtr, evidence)
	}

	// Step 3: Claude Code, positive.
	if isAllowedEntrypoint {
		evidence = append(evidence, EvEntrypointEnv)
		if claudeOverride {
			evidence = append(evidence, EvClaudeTranscript)
		}
		return finish(ClaudeCode, allowedSurface, versionPtr, evidence)
	}

	// Step 4: Claude Code, presumed. No foreign marker fired above.
	if transcriptUnderClaude {
		return finish(ClaudeCodePresumed, SurfaceUnknown, versionPtr, []Evidence{EvClaudeTranscript})
	}

	// Step 5: unknown.
	return finish(Unknown, SurfaceUnknown, versionPtr, evidence)
}

// codexTurnIDEvents are the events Codex documents turn_id on for a tool or
// turn call (Archive 60.2.3, [CDX:481-485]). MessageDisplay is excluded on
// purpose: Claude Code documents turn_id there too [CC:1477], and rashomon
// does not subscribe to it as a marker source.
var codexTurnIDEvents = map[string]bool{
	"PreToolUse":        true,
	"PostToolUse":       true,
	"PermissionRequest": true,
	"Stop":              true,
	"SubagentStop":      true,
	"UserPromptSubmit":  true,
	"PreCompact":        true,
	"PostCompact":       true,
	"Interrupt":         true,
}

// entrypointSurfaces maps the 2.1.280 CLAUDE_CODE_ENTRYPOINT allowlist to a
// Surface. remote* is a prefix, handled in entrypointSurface, not here.
var entrypointSurfaces = map[string]Surface{
	"cli":                       SurfaceCLI,
	"claude-vscode":             SurfaceIDE,
	"claude-desktop":            SurfaceDesktop,
	"local-agent":               SurfaceDesktop,
	"sdk-ts":                    SurfaceSDK,
	"sdk-py":                    SurfaceSDK,
	"sdk-cli":                   SurfaceSDK,
	"claude-code-github-action": SurfaceCloud,
}

// entrypointSurface reports the Surface for an allowlisted entrypoint and
// whether it is allowlisted at all. An empty entrypoint is never allowlisted.
func entrypointSurface(entrypoint string) (Surface, bool) {
	if entrypoint == "" {
		return SurfaceUnknown, false
	}
	if strings.HasPrefix(entrypoint, "remote") {
		return SurfaceCloud, true
	}
	if s, ok := entrypointSurfaces[entrypoint]; ok {
		return s, true
	}
	return SurfaceUnknown, false
}

// underProjects reports whether transcriptPath resolves under
// configDir/projects/, using filepath.Rel so a ".." element cannot forge
// containment the way a bare strings.HasPrefix could.
func underProjects(transcriptPath, configDir string) bool {
	if transcriptPath == "" || configDir == "" {
		return false
	}
	projects := filepath.Join(configDir, "projects")
	rel, err := filepath.Rel(projects, filepath.Clean(transcriptPath))
	if err != nil {
		return false
	}
	if rel == "." {
		return false // the transcript path names the directory itself, not a file in it
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// versionCharset is cursor_version's charset, checked byte by byte: no
// regexp compiles at package scope or at all, matching the convention
// internal/verify/finding.go documents for the same reason (no allocation on
// a hot path -- Detect's budget is the 6.4 0.2ms p99, measured on the
// payload as a whole; no BenchmarkDetect exists yet to pin it).
func versionCharset(c byte) bool {
	switch {
	case c >= '0' && c <= '9', c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z':
		return true
	case c == '.' || c == '+' || c == '-':
		return true
	}
	return false
}

// checkedVersion returns cursor_version as *string when it is present and
// matches ^[0-9A-Za-z.+-]{1,32}$, else nil. The value itself is still read
// (it is stored, unlike user_email), but a value outside the charset is
// dropped rather than carried into a record.
func checkedVersion(v string, present bool) *string {
	if !present || len(v) == 0 || len(v) > 32 {
		return nil
	}
	for i := 0; i < len(v); i++ {
		if !versionCharset(v[i]) {
			return nil
		}
	}
	return &v
}

// finish builds the Identity: evidence sorted and de-duplicated, Blind from
// Caps(id, surface) at the current CapsVersion.
func finish(id ID, surface Surface, version *string, evidence []Evidence) Identity {
	return Identity{
		ID:          id,
		Surface:     surface,
		Evidence:    sortedEvidence(evidence),
		Version:     version,
		CapsVersion: CapsVersion,
		Blind:       blindOf(Caps(id, surface)),
	}
}

// unknown builds the fail-closed Identity for a payload Detect cannot read:
// no identity decided it, so there is no evidence either.
func unknown(evidence []Evidence) Identity {
	return finish(Unknown, SurfaceUnknown, nil, evidence)
}

// sortedEvidence de-duplicates and sorts in one pass over a small, already
// bounded slice (at most 4 entries from this function's call sites).
func sortedEvidence(ev []Evidence) []Evidence {
	if len(ev) == 0 {
		return nil
	}
	seen := make(map[Evidence]bool, len(ev))
	out := make([]Evidence, 0, len(ev))
	for _, e := range ev {
		if !seen[e] {
			seen[e] = true
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
