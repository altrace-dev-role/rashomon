package steer

import (
	"slices"
	"strings"
)

// Client is what the marshaller knows about the program reading the hook's
// stdout (tech spec :2019, MirrorVerified; D6: a value on the marshaller, no
// package-level setter). The zero value is an unknown version, an unverified
// mirror, so every gated row is shadow by default (INV-12).
type Client struct {
	// Version is the Claude Code version the running client declares, as
	// ClientVersion read it, or "" when it could not be read.
	Version string
	// Source is where Version came from: VersionSourceEnv or
	// VersionSourceUnknown (the session record's claude_code_version_source).
	Source string
	// MirrorInBand is true when the harness's in-band mirror is verified for
	// this surface: harness.Caps(id, surface)[CapMirrorInBand] == Yes, set by
	// the caller from the invocation's identity.
	MirrorInBand bool
	// Subagent is true when the call carries an agent_id. Such a call is
	// gated on every row that is not exempt: P-TRU-1 has passed on the main
	// agent only (:2019, "events with agent_id only once P-TRU-1 has their
	// row").
	Subagent bool
}

// The two values of Client.Source.
const (
	VersionSourceEnv     = "env"
	VersionSourceUnknown = "unknown"
)

// mirrorVerifiedOn is the compiled list of Claude Code versions a live
// P-TRU-1 run passed on, per event (tech spec :2019,
// OutputRule.MirrorVerifiedOn): on those versions, the event's model-visible
// field reached the model while its systemMessage mirror reached the user.
// A row with no entry, or with an empty one, is gated on every version, so
// a row added to outputKeys later fails closed. Exempt rows
// (eventKeys.exempt) are not read here. Only a release changes this list;
// test/live/p-tru-1.sh, the per-release canary, is what extends it.
var mirrorVerifiedOn = map[string][]string{
	// P-TRU-1, 2026-10-23, on 2.1.280: one PostToolUse output carrying
	// both additionalContext and systemMessage (PRD PF-ST4).
	string(EventPostToolUse): {"2.1.280"},
	// The same payload shape as PostToolUse (hook/post.go), not probed.
	string(EventPostToolUseFailure): {},
	// Not probed; empty until test/live/p-tru-1.sh runs on them (:2019).
	string(EventPostToolBatch): {},
	string(EventSessionStart):  {},
	string(EventSubagentStart): {},
	// permissionDecisionReason (G3); nonce A only, since the row has no
	// additionalContext slot for nonce B; not probed.
	string(EventPreToolUse): {},
}

// verified reports whether event's model-visible field may print for c
// under table: the row is exempt, or c is on the main agent with its mirror
// verified and runs a version table lists for the event, compared whole.
func verified(keys eventKeys, event string, c Client, table map[string][]string) bool {
	if keys.exempt {
		return true
	}
	versions := table[event]
	return c.MirrorInBand && !c.Subagent && slices.Contains(versions, c.Version)
}

// ContextAllowed reports whether AdditionalContext prints on event for c:
// the row carries it and its channel is verified for c. It reads the table
// MarshalFor reads, so a router that shadows a finding with suppressed_by
// unverified_channel where this is false cannot disagree with the marshal.
func ContextAllowed(event string, c Client) bool {
	return allowed(event, c, mirrorVerifiedOn)
}

// allowed is ContextAllowed over a given table, the seam a test reaches a
// row's printing path through while the compiled list keeps it empty.
func allowed(event string, c Client, table map[string][]string) bool {
	keys, ok := outputKeys[event]
	return ok && keys.context && verified(keys, event, c, table)
}

// The form Claude Code gives the environment its hooks inherit (read from
// the 2.1.280 binary: "claude-code_" + its version with dots made dashes +
// "_harness"; measured on a SessionStart hook). The "_agent" form is what
// its Bash tool children carry instead.
const (
	aiAgentPrefix = "claude-code_"
	aiAgentSuffix = "_harness"
	maxAIAgent    = 32
)

// ClientVersion reads the running client's version from the value of its
// AI_AGENT environment variable, which the caller passes. It accepts exactly
// the harness form, two to four dash-separated runs of digits, at most 32
// bytes, and returns the dotted version with VersionSourceEnv. Anything
// else, an empty value, the "_agent" or a slash form, another agent's value,
// one component, a letter, returns "", VersionSourceUnknown, which is
// shadow. The format is the client's and undocumented; a change reads as
// unknown, and the canary sees it.
func ClientVersion(aiAgent string) (version, source string) {
	if len(aiAgent) > maxAIAgent {
		return "", VersionSourceUnknown
	}
	v, ok := strings.CutPrefix(aiAgent, aiAgentPrefix)
	if ok {
		v, ok = strings.CutSuffix(v, aiAgentSuffix)
	}
	if !ok || !dashedDigits(v) {
		return "", VersionSourceUnknown
	}
	return strings.ReplaceAll(v, "-", "."), VersionSourceEnv
}

// dashedDigits reports whether v is two to four non-empty runs of ASCII
// digits joined by single dashes.
func dashedDigits(v string) bool {
	runs, inRun := 0, false
	for i := 0; i < len(v); i++ {
		switch b := v[i]; {
		case b >= '0' && b <= '9':
			if !inRun {
				runs++
				inRun = true
			}
		case b == '-' && inRun:
			inRun = false
		default:
			return false
		}
	}
	return inRun && runs >= 2 && runs <= 4
}
