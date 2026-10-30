package steer

import (
	"cmp"
	"strings"
	"sync"
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

// channelBasis is one event's row of the probed-version table (tech spec
// :2019, OutputRule.MirrorVerifiedOn, read as a floor): floor is the lowest
// Claude Code version a live P-TRU-1 run passed on, and every version at or
// above it is verified, compared by component (compareVersions), except the
// versions in excluded, at or above the floor, that a run failed on. A row
// with no floor is verified on no version. The cost of a floor over a list
// is a release that stops forwarding the field: it reads as verified until a
// run fails on it and its version is excluded.
type channelBasis struct {
	floor    string
	excluded []string
}

// excludes reports whether v equals an excluded version by component.
func (b channelBasis) excludes(v string) bool {
	for _, x := range b.excluded {
		if compareVersions(v, x) == 0 {
			return true
		}
	}
	return false
}

// mirrorVerifiedOn is the compiled table, per event: on a verified version
// the event's model-visible field reached the model while its systemMessage
// mirror reached the user. A row with no entry, or with no floor, is gated
// on every version, so a row added to outputKeys later fails closed. Exempt
// rows (eventKeys.exempt) are not read here. Only a release changes this
// table; test/live/p-tru-1.sh, the per-release canary, is what sets a floor
// or adds an exclusion.
func mirrorVerifiedOn() map[string]channelBasis {
	mirrorVerifiedOnOnce.Do(func() {
		mirrorVerifiedOnOnce.v = map[string]channelBasis{
			// P-TRU-1, 2026-10-23, on 2.1.280: one PostToolUse output carrying
			// both additionalContext and systemMessage (PRD PF-ST4).
			string(EventPostToolUse): {floor: "2.1.280"},
			// The same payload shape as PostToolUse (hook/post.go), which is
			// not a probe: closed until p-tru-1.sh passes on the event.
			string(EventPostToolUseFailure): {},
			// Not probed; closed until test/live/p-tru-1.sh runs on them (:2019).
			string(EventPostToolBatch): {},
			string(EventSessionStart):  {},
			string(EventSubagentStart): {},
			// permissionDecisionReason (G3); nonce A only, since the row has no
			// additionalContext slot for nonce B; not probed.
			string(EventPreToolUse): {},
		}
	})
	return mirrorVerifiedOnOnce.v
}

// mirrorVerifiedOnOnce holds what mirrorVerifiedOn returns, built on its first
// call and never at init (RES-2).
var mirrorVerifiedOnOnce struct {
	sync.Once
	v map[string]channelBasis
}

// A shutReason is why a row's model-visible field may not print for a
// client, the first that applies in shutBy's order; channelOpen is none.
type shutReason uint8

const (
	channelOpen shutReason = iota
	// shutNoMirror: the surface's in-band mirror is not verified, so the
	// user would not see what the agent was told (claude -p and the SDKs,
	// or a surface that could not be read).
	shutNoMirror
	// shutSubagent: the call carries an agent_id, and P-TRU-1 has passed on
	// the main agent only (:2019, "events with agent_id only once P-TRU-1
	// has their row").
	shutSubagent
	// shutEventClosed: the event's row has no floor.
	shutEventClosed
	// shutUnknownVersion: the client's version is empty or not two to four
	// dot-joined runs of digits.
	shutUnknownVersion
	// shutBelowFloor and shutExcluded: the version is below the row's floor,
	// or equal to one of its exclusions.
	shutBelowFloor
	shutExcluded
)

// shutBy is why event's model-visible field may not print for c under
// table, or channelOpen: the row is exempt, or c is on the main agent with
// its mirror verified and runs a version at or above the event's floor that
// no exclusion names. A malformed version is never compared.
func shutBy(keys eventKeys, event string, c Client, table map[string]channelBasis) shutReason {
	b := table[event]
	switch {
	case keys.exempt:
		return channelOpen
	case !c.MirrorInBand:
		return shutNoMirror
	case c.Subagent:
		return shutSubagent
	case b.floor == "":
		return shutEventClosed
	case !digitRuns(c.Version, '.'):
		return shutUnknownVersion
	case compareVersions(c.Version, b.floor) < 0:
		return shutBelowFloor
	case b.excludes(c.Version):
		return shutExcluded
	}
	return channelOpen
}

// verified reports whether event's model-visible field may print for c
// under table: shutBy finds no reason it may not.
func verified(keys eventKeys, event string, c Client, table map[string]channelBasis) bool {
	return shutBy(keys, event, c, table) == channelOpen
}

// compareVersions orders two dotted versions by component, numerically, and
// returns -1, 0 or 1. A component's leading zeros are dropped, the longer
// run is the larger and runs of one length compare byte by byte, so no
// component is parsed into an integer, none can overflow, and nothing is
// allocated. A missing component reads as 0, so 2.1 equals 2.1.0. Both
// arguments are digit runs joined by dots (digitRuns); compareVersions does
// not check.
func compareVersions(a, b string) int {
	for a != "" || b != "" {
		var x, y string
		x, a, _ = strings.Cut(a, ".")
		y, b, _ = strings.Cut(b, ".")
		x, y = strings.TrimLeft(x, "0"), strings.TrimLeft(y, "0")
		if len(x) != len(y) {
			return cmp.Compare(len(x), len(y))
		}
		if c := strings.Compare(x, y); c != 0 {
			return c
		}
	}
	return 0
}

// ContextAllowed reports whether AdditionalContext prints on event for c:
// the row carries it and its channel is verified for c. It reads the table
// MarshalFor reads, so a router that shadows a finding with suppressed_by
// unverified_channel where this is false cannot disagree with the marshal.
func ContextAllowed(event string, c Client) bool {
	return allowed(event, c, mirrorVerifiedOn())
}

// allowed is ContextAllowed over a given table, the seam a test reaches a
// row's printing path through while the compiled table keeps it closed.
func allowed(event string, c Client, table map[string]channelBasis) bool {
	keys, ok := outputKeys()[event]
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
	if !ok || !digitRuns(v, '-') {
		return "", VersionSourceUnknown
	}
	return strings.ReplaceAll(v, "-", "."), VersionSourceEnv
}

// digitRuns reports whether v is two to four non-empty runs of ASCII digits
// joined by single sep bytes: the harness form's version with '-', a
// Client's Version with '.'.
func digitRuns(v string, sep byte) bool {
	runs, inRun := 0, false
	for i := 0; i < len(v); i++ {
		switch b := v[i]; {
		case b >= '0' && b <= '9':
			if !inRun {
				runs++
				inRun = true
			}
		case b == sep && inRun:
			inRun = false
		default:
			return false
		}
	}
	return inRun && runs >= 2 && runs <= 4
}
