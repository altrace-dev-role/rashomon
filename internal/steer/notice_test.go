package steer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/harness"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// below is a client on the CLI, mirror verified, one patch under the floor.
var below = Client{Version: "2.1.279", Source: VersionSourceEnv, MirrorInBand: true}

// belowNotice is channelNotice's text for below on PostToolUse.
const belowNotice = "rashomon: a note was built and not sent to the agent (Claude Code 2.1.279, PostToolUse; " +
	"delivery verified from 2.1.280): this version is older. Shown once per session."

// TestVG1g_TheChannelNoticePrintsOncePerSession: the first note of a session
// that unverified_channel shadows prints one user-only line, the channel
// notice, and nothing of the note; the next shadowed note of the session
// prints nothing; another session prints its own. Each line keeps its
// suppression, its basis (client_version, verified_floor, excluded) and,
// on the first only, the notice.
func TestVG1g_TheChannelNoticePrintsOncePerSession(t *testing.T) {
	ctx := routerContext(t)
	ctx.Client = below
	if out := Route(ctx, []Finding{regressionNote(t, "go test", 5, "a", LevelNudge)}); out != (Output{SystemMessage: belowNotice}) {
		t.Errorf("first shadowed call printed %+v, want only %q", out, belowNotice)
	}
	if out := Route(ctx, []Finding{regressionNote(t, "go vet", 6, "b", LevelNudge)}); out != (Output{}) {
		t.Errorf("second shadowed call printed %+v, want nothing", out)
	}
	lines := lastLines(t, ctx, 2)
	for i, l := range lines {
		wantLine(t, l, LineStateSuppressed, SuppressedByUnverifiedChannel, DeliveryShadow)
		if l.ClientVersion != "2.1.279" || l.VerifiedFloor != "2.1.280" || l.Excluded == nil || len(l.Excluded) != 0 ||
			l.UserText != "" || l.ModelText != "" || l.MirroredToUser {
			t.Errorf("line %d: version %q, floor %q, excluded %v, texts %q / %q, mirrored %v",
				i, l.ClientVersion, l.VerifiedFloor, l.Excluded, l.ModelText, l.UserText, l.MirroredToUser)
		}
	}
	if n := lines[0].Notice; n == nil || *n != NoticeChannel {
		t.Errorf("first line's notice = %v, want %q", n, NoticeChannel)
	}
	if n := lines[1].Notice; n != nil {
		t.Errorf("second line's notice = %q, want null", *n)
	}
	other := *ctx
	other.SessionID = "s2"
	if out := Route(&other, []Finding{regressionNote(t, "go test", 5, "a", LevelNudge)}); out != (Output{SystemMessage: belowNotice}) {
		t.Errorf("another session's first shadowed call printed %+v, want %q", out, belowNotice)
	}
	if _, err := os.Lstat(filepath.Join(ctx.Store.RunDir("s1"), store.FileNoticeChannel)); err != nil {
		t.Errorf("the channel marker: %v", err)
	}
}

// TestVG1k_AFailedAppendReturnsTheNotice: a call that took the session's
// notice marker and then could not write the line that records it prints
// nothing and returns the marker, so the next call prints the notice. The
// append is refused here by a line over the store's record cap, a project
// name of 1 MiB.
func TestVG1k_AFailedAppendReturnsTheNotice(t *testing.T) {
	ctx := routerContext(t)
	ctx.Client = below
	ctx.Project = strings.Repeat("p", store.MaxRecordBytes)
	if out := Route(ctx, []Finding{regressionNote(t, "go test", 5, "a", LevelNudge)}); out != (Output{}) {
		t.Errorf("a refused append printed %+v", out)
	}
	if _, err := os.Lstat(filepath.Join(ctx.Store.RunDir(ctx.SessionID), store.FileNoticeChannel)); err == nil {
		t.Errorf("the refused append kept the channel marker")
	}
	ctx.Project = "proj"
	if out := Route(ctx, []Finding{regressionNote(t, "go vet", 6, "b", LevelNudge)}); out != (Output{SystemMessage: belowNotice}) {
		t.Errorf("the call after the refusal printed %+v, want %q", out, belowNotice)
	}
}

// TestVG1i_TheNoticeWaitsForACallThatPrintsNothingElse: a call has one
// systemMessage, so a shadowed note beside a flag that prints leaves the
// notice to a later call: the flag prints alone, the note's line records no
// notice, and the next shadowed call prints it. A delivered note's line
// carries the basis and no notice.
func TestVG1i_TheNoticeWaitsForACallThatPrintsNothingElse(t *testing.T) {
	ctx := routerContext(t)
	ctx.Client = below
	out := Route(ctx, []Finding{regressionNote(t, "go test", 5, "a", LevelNudge), regressionNote(t, "go vet", 6, "f", LevelFlag)})
	if out.SystemMessage == "" || out.SystemMessage == belowNotice || out.AdditionalContext != "" {
		t.Fatalf("the call with a flag printed %+v, want the flag alone", out)
	}
	for _, l := range lastLines(t, ctx, 2) {
		if l.Notice != nil {
			t.Errorf("line %s/%s records notice %q on a call that printed a flag", l.Pattern, l.Key, *l.Notice)
		}
	}
	if out := Route(ctx, []Finding{regressionNote(t, "go test", 7, "c", LevelNudge)}); out != (Output{SystemMessage: belowNotice}) {
		t.Errorf("the next shadowed call printed %+v, want %q", out, belowNotice)
	}
	open := routerContext(t)
	open.Client = Client{Version: "2.1.314", Source: VersionSourceEnv, MirrorInBand: true}
	if out := Route(open, []Finding{regressionNote(t, "go test", 5, "a", LevelNudge)}); out.AdditionalContext == "" {
		t.Fatalf("2.1.314 printed %+v, want the note delivered", out)
	}
	l := lastLines(t, open, 1)[0]
	wantLine(t, l, LineStateDelivered, "", DeliveryDelivered)
	if l.ClientVersion != "2.1.314" || l.VerifiedFloor != "2.1.280" || l.Notice != nil {
		t.Errorf("delivered line: version %q, floor %q, notice %v", l.ClientVersion, l.VerifiedFloor, l.Notice)
	}
}

// TestVG1l_TheLineCarriesItsEventsRow: a line's basis is its own event's row:
// on PostToolUseFailure the floor is 2.1.304 (#147), while the same client's
// PostToolUse line carries 2.1.280.
func TestVG1l_TheLineCarriesItsEventsRow(t *testing.T) {
	for _, tc := range []struct {
		event Event
		floor string
	}{{EventPostToolUseFailure, "2.1.304"}, {EventPostToolUse, "2.1.280"}} {
		ctx := routerContext(t)
		ctx.Event = tc.event
		Route(ctx, []Finding{regressionNote(t, "go test", 5, "a", LevelNudge)})
		if l := lastLines(t, ctx, 1)[0]; l.VerifiedFloor != tc.floor || l.ClientVersion != verifiedClient.Version {
			t.Errorf("%s: floor %q, version %q; want %q, %q", tc.event, l.VerifiedFloor, l.ClientVersion, tc.floor, verifiedClient.Version)
		}
	}
}

// TestVG1h_TheNoticeNamesVersionEventFloorAndWhy: the channel notice names the
// Claude Code version (never a malformed one), the event and its floor, says
// the note was built and not sent, and gives the reason shutBy finds, one
// text per reason. claude -p and the SDKs read as by design.
func TestVG1h_TheNoticeNamesVersionEventFloorAndWhy(t *testing.T) {
	table := map[string]channelBasis{
		string(EventPostToolUse):        {floor: "2.1.280", excluded: []string{"2.1.300"}},
		string(EventPostToolUseFailure): {floor: "2.1.304"},
	}
	const head = "rashomon: a note was built and not sent to the agent ("
	const tail = ". Shown once per session."
	at := func(v string) Client { return Client{Version: v, Source: VersionSourceEnv, MirrorInBand: true} }
	for _, tc := range []struct {
		name    string
		event   Event
		c       Client
		surface string
		want    string
	}{
		{"below the floor", EventPostToolUse, at("2.1.279"), "cli",
			"Claude Code 2.1.279, PostToolUse; delivery verified from 2.1.280): this version is older"},
		{"an excluded version", EventPostToolUse, at("2.1.300"), "cli",
			"Claude Code 2.1.300, PostToolUse; delivery verified from 2.1.280): this version failed the delivery check"},
		{"below the failure floor", EventPostToolUseFailure, at("2.1.280"), "cli",
			"Claude Code 2.1.280, PostToolUseFailure; delivery verified from 2.1.304): this version is older"},
		{"a closed event", EventSessionStart, at("2.1.314"), "cli",
			"Claude Code 2.1.314, SessionStart; delivery verified on no version yet): notes on this event wait for a release that passes the delivery check"},
		{"claude -p", EventPostToolUse, Client{Version: "2.1.314", Source: VersionSourceEnv}, "sdk",
			"Claude Code 2.1.314, PostToolUse; delivery verified from 2.1.280): claude -p and SDK sessions receive no notes by design"},
		{"an unknown surface", EventPostToolUse, Client{Version: "2.1.314", Source: VersionSourceEnv}, "unknown",
			"Claude Code 2.1.314, PostToolUse; delivery verified from 2.1.280): the session's surface could not be read, and notes go only where the user sees what the agent is told"},
		{"a subagent", EventPostToolUse, Client{Version: "2.1.314", Source: VersionSourceEnv, MirrorInBand: true, Subagent: true}, "cli",
			"Claude Code 2.1.314, PostToolUse; delivery verified from 2.1.280): notes on a subagent's calls are not sent yet"},
		{"no version", EventPostToolUse, at(""), "cli",
			"Claude Code, version unreadable, PostToolUse; delivery verified from 2.1.280): the version could not be read"},
		{"a malformed version is not printed", EventPostToolUse, at("2.1.\x1b[31m"), "cli",
			"Claude Code, version unreadable, PostToolUse; delivery verified from 2.1.280): the version could not be read"},
	} {
		if got, want := channelNotice(tc.event, tc.c, tc.surface, table), head+tc.want+tail; got != want {
			t.Errorf("%s:\n got %q\nwant %q", tc.name, got, want)
		}
	}
}

// TestVG1d_TheSDKSurfaceIsTheHarnesss: the notice's claude -p text keys on
// the harness's own value for the SDK surface, the one Caps leaves without a
// verified mirror.
func TestVG1d_TheSDKSurfaceIsTheHarnesss(t *testing.T) {
	if surfaceSDK != string(harness.SurfaceSDK) {
		t.Errorf("surfaceSDK = %q, harness.SurfaceSDK = %q", surfaceSDK, harness.SurfaceSDK)
	}
	if harness.Caps(harness.ClaudeCode, harness.SurfaceSDK)[harness.CapMirrorInBand] == harness.Yes {
		t.Errorf("the SDK surface's mirror is verified; the by-design text would be false")
	}
}
