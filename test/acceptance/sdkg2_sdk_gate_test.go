package acceptance

import (
	"fmt"
	"testing"
)

// SDK-GATE2: a TypeScript SDK program started where CLAUDE_CODE_ENTRYPOINT is
// already set, as from a Claude Code session's Bash tool, reaches its hooks
// as sdk-cli, the entrypoint Claude Code sets for claude -p, and SDK-GATE1
// recorded it after the person answered n (live, se1/sdkg1-probe.txt Phase
// B, 22:35:33, Claude Code 2.1.280 and TS SDK 0.3.316). The SDK sets
// CLAUDE_AGENT_SDK_VERSION for the process it starts; a person's claude -p
// carries none. The DE's rule (22:4x): sdk-ts and sdk-py are SDK programs,
// and so is sdk-cli with that variable set. cli with it inherited is a
// person's own session started from inside a program, and records.

// TestSDKG2_AnSDKProgramUnderAnInheritedEntrypointIsRecordedOnlyAfterAY: on
// the watch route, a whole session as sdk-cli with CLAUDE_AGENT_SDK_VERSION
// set changes nothing on the machine after an n or with no answer on file,
// and is recorded after a y. Controls: claude -p (sdk-cli, no version) and a
// person's terminal session started from inside a program (cli, version
// set) record after an n. Break: read sdk-cli as claude -p whatever its
// environment, and the n row records, the live row; close the gate on the
// version variable alone, and the terminal row records nothing.
func TestSDKG2_AnSDKProgramUnderAnInheritedEntrypointIsRecordedOnlyAfterAY(t *testing.T) {
	const version = "CLAUDE_AGENT_SDK_VERSION=0.3.316" // the live run's
	for _, tc := range []struct {
		name, answer, entrypoint, version string
		recorded                          bool
	}{
		{"n, the TypeScript SDK under an inherited entrypoint", "n", "sdk-cli", version, false},
		{"no answer on file, the same", "", "sdk-cli", version, false},
		{"y, the same", "y", "sdk-cli", version, true},
		{"n, claude -p", "n", "sdk-cli", "", true},
		{"n, a terminal session started from inside a program", "n", "cli", version, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			e.sdkgAnswer(tc.answer)
			var extra []string
			if tc.version != "" {
				extra = append(extra, tc.version)
			}
			before := e.machine()
			e.sdkgSession(tc.entrypoint, extra...)
			what := fmt.Sprintf("a session as %s with %q after answer %q", tc.entrypoint, tc.version, tc.answer)
			if !tc.recorded {
				e.requireUnchanged(before, what)
				return
			}
			if !e.declared("toolu_e") {
				t.Errorf("%s recorded no declaration of its edit", what)
			}
		})
	}
}
