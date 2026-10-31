package steer

import (
	"encoding/json"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// regressionNote is a regression-location finding with a rendered Message,
// the one pattern with a template row; gate varies its text.
func regressionNote(t *testing.T, gate string, afterSeq int64, key string, level Level) Finding {
	t.Helper()
	m, err := render.New(pattern.RegressionLocation, render.RegressionLocationBinding{
		Gate: slot.Gate(gate), Minutes: 6, Exit: 1,
		Paths: slot.NewPathList([]slot.Path{"internal/a.go"}, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	return Finding{
		Pattern: pattern.RegressionLocation, Key: key, Priority: 80, MaxLevel: level, StopForm: StopFormNone,
		Message: m, Evidence: []int64{afterSeq - 1}, AfterSeq: afterSeq,
	}
}

// routerContext is a PostToolUse call of session s1, prompt p1, on a fresh
// store, attended, consented, with a verified client and counted ids.
func routerContext(t *testing.T) *Context {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	return &Context{
		Event: EventPostToolUse, Client: verifiedClient, Store: st, SessionID: "s1", PromptID: ptr("p1"),
		Project: "proj", HarnessID: "claude_code", Surface: "cli", NowUnixMS: 1_793_000_000_000,
		NewID:   func() string { n++; return fmt.Sprintf("%032d", n) },
		Consent: true, Arm: ArmLive, Attendance: AttendanceAttended,
	}
}

// steerLines reads session s1's steer log back as Lines.
func steerLines(t *testing.T, ctx *Context) []Line {
	t.Helper()
	raw, skipped, err := ctx.Store.ReadSteer(ctx.SessionID)
	if err != nil || skipped != 0 {
		t.Fatalf("ReadSteer: %v, %d skipped", err, skipped)
	}
	lines := make([]Line, len(raw))
	for i, b := range raw {
		if err := json.Unmarshal(b, &lines[i]); err != nil {
			t.Fatal(err)
		}
	}
	return lines
}

func lastLines(t *testing.T, ctx *Context, n int) []Line {
	t.Helper()
	all := steerLines(t, ctx)
	if len(all) < n {
		t.Fatalf("the log holds %d lines, want at least %d", len(all), n)
	}
	return all[len(all)-n:]
}

func wantLine(t *testing.T, l Line, state LineState, by SuppressedBy, delivery Delivery) {
	t.Helper()
	gotBy, gotDelivery := "", ""
	if l.SuppressedBy != nil {
		gotBy = string(*l.SuppressedBy)
	}
	if l.Delivery != nil {
		gotDelivery = string(*l.Delivery)
	}
	if l.State != state || gotBy != string(by) || gotDelivery != string(delivery) {
		t.Errorf("line %s/%s = state %q, suppressed_by %q, delivery %q; want %q, %q, %q",
			l.Pattern, l.Key, l.State, gotBy, gotDelivery, state, by, delivery)
	}
}

// TestST1b_TheWinnerDeliversAndTheLoserIsSuperseded: two notes on one call,
// differing in after_seq: the lower is delivered, the other superseded by
// the one-per-batch cap (tech 4.2.5: batch losers are superseded and none
// becomes a flag; card 2.2 A6).
func TestST1b_TheWinnerDeliversAndTheLoserIsSuperseded(t *testing.T) {
	ctx := routerContext(t)
	later := regressionNote(t, "go vet", 9, "b", LevelNudge)
	first := regressionNote(t, "go test", 5, "a", LevelNudge)
	out := Route(ctx, []Finding{later, first})
	if out.AdditionalContext != first.Message.Text() {
		t.Errorf("AdditionalContext = %q, want the lower after_seq's text", out.AdditionalContext)
	}
	lines := steerLines(t, ctx)
	if len(lines) != 2 || lines[0].Key != "a" || lines[1].Key != "b" {
		t.Fatalf("lines = %+v, want a then b, in rank order", lines)
	}
	wantLine(t, lines[0], LineStateDelivered, "", DeliveryDelivered)
	wantLine(t, lines[1], LineStateSuperseded, SuppressedByBudgetBatch, DeliverySuppressedBudget)
	if lines[1].ModelText != "" || lines[1].UserText != "" || len(lines[1].Slots) == 0 {
		t.Errorf("the superseded line keeps text %q/%q and slots %v, want slots only (RES-32)", lines[1].ModelText, lines[1].UserText, lines[1].Slots)
	}
}

// TestST4d_ModelVisibleNoteIsMirrored: the delivered note's systemMessage is
// render.Mirror of its text byte for byte, and its line says so (INV-5; G3).
func TestST4d_ModelVisibleNoteIsMirrored(t *testing.T) {
	ctx := routerContext(t)
	f := regressionNote(t, "go test", 5, "a", LevelNudge)
	out := Route(ctx, []Finding{f})
	mirror, err := render.Mirror(f.Message.Text())
	if err != nil {
		t.Fatal(err)
	}
	if out.SystemMessage != mirror || out.AdditionalContext != f.Message.Text() || out.Decision != nil || out.StopContinue != "" {
		t.Errorf("Output = %+v, want the text in additionalContext and its mirror in systemMessage", out)
	}
	l := lastLines(t, ctx, 1)[0]
	if l.UserText != mirror || l.ModelText != f.Message.Text() || l.Mirror != MirrorSystemMessage || !l.MirroredToUser {
		t.Errorf("line texts %q / %q, mirror %q, mirrored %v", l.ModelText, l.UserText, l.Mirror, l.MirroredToUser)
	}
	wantLine(t, l, LineStateDelivered, "", DeliveryDelivered)
}

// TestST7_FourthNoteIsSuppressedByThePromptBudget: three calls in one prompt
// deliver; the fourth prints nothing and its line says budget_prompt (tech
// 4.2.5; REC-43). A fourth note cannot be delivered later in the prompt, so
// it is not deferred.
func TestST7_FourthNoteIsSuppressedByThePromptBudget(t *testing.T) {
	ctx := routerContext(t)
	for i := range 3 {
		if out := Route(ctx, []Finding{regressionNote(t, "go test", int64(5+i), fmt.Sprint("k", i), LevelNudge)}); out.AdditionalContext == "" {
			t.Fatalf("note %d did not print", i+1)
		}
	}
	if out := Route(ctx, []Finding{regressionNote(t, "go test", 9, "k3", LevelNudge)}); out != (Output{}) {
		t.Errorf("the fourth note printed %+v", out)
	}
	wantLine(t, lastLines(t, ctx, 1)[0], LineStateSuppressed, SuppressedByBudgetPrompt, DeliverySuppressedBudget)
	ctx.PromptID = ptr("p2")
	if out := Route(ctx, []Finding{regressionNote(t, "go test", 10, "k4", LevelNudge)}); out.AdditionalContext == "" {
		t.Error("the next prompt's first note did not print")
	}
}

// TestST7b_ASecondNoteWithTheSameKeyIsBudgetPattern: one note per (pattern,
// kind, key) per prompt, across calls and within one.
func TestST7b_ASecondNoteWithTheSameKeyIsBudgetPattern(t *testing.T) {
	ctx := routerContext(t)
	Route(ctx, []Finding{regressionNote(t, "go test", 5, "same", LevelNudge)})
	if out := Route(ctx, []Finding{regressionNote(t, "go vet", 6, "same", LevelNudge)}); out != (Output{}) {
		t.Errorf("the second note with the key printed %+v", out)
	}
	wantLine(t, lastLines(t, ctx, 1)[0], LineStateSuppressed, SuppressedByBudgetPattern, DeliverySuppressedBudget)
	out := Route(ctx, []Finding{regressionNote(t, "go test", 7, "twice", LevelNudge), regressionNote(t, "go vet", 8, "twice", LevelNudge)})
	if out.AdditionalContext == "" {
		t.Error("the call's first claim on a fresh key did not print")
	}
	wantLine(t, lastLines(t, ctx, 1)[0], LineStateSuppressed, SuppressedByBudgetPattern, DeliverySuppressedBudget)
}

// TestST7c_TheSixthFlagIsACandidate: five flags print per prompt; the sixth
// waits for the turn-end line as a candidate, delivery null (tech 4.2.5;
// card 2.2 A6: deferred is 4.2.6 step 5's word for the next prompt).
func TestST7c_TheSixthFlagIsACandidate(t *testing.T) {
	ctx := routerContext(t)
	for i := range 5 {
		if out := Route(ctx, []Finding{regressionNote(t, "go test", int64(5+i), fmt.Sprint("f", i), LevelFlag)}); out.SystemMessage == "" || out.AdditionalContext != "" {
			t.Fatalf("flag %d printed %+v", i+1, out)
		}
	}
	if out := Route(ctx, []Finding{regressionNote(t, "go test", 10, "f5", LevelFlag)}); out != (Output{}) {
		t.Errorf("the sixth flag printed %+v", out)
	}
	wantLine(t, lastLines(t, ctx, 1)[0], LineStateCandidate, "", "")
}

// TestST_AFlagWaitsBesideANote: a call has one systemMessage; when a note
// prints, the call's flag is a candidate, and two flags on a call print one.
func TestST_AFlagWaitsBesideANote(t *testing.T) {
	ctx := routerContext(t)
	note := regressionNote(t, "go test", 5, "n", LevelNudge)
	out := Route(ctx, []Finding{regressionNote(t, "go vet", 4, "f", LevelFlag), note})
	if out.AdditionalContext != note.Message.Text() {
		t.Errorf("Output = %+v, want the note", out)
	}
	lines := lastLines(t, ctx, 2)
	for _, l := range lines {
		if l.Key == "f" {
			wantLine(t, l, LineStateCandidate, "", "")
		}
	}
	flag1 := regressionNote(t, "go build", 6, "f1", LevelFlag)
	out = Route(ctx, []Finding{flag1, regressionNote(t, "go vet", 7, "f2", LevelFlag)})
	if out.SystemMessage != flag1.Message.Text() || out.AdditionalContext != "" {
		t.Errorf("two flags printed %+v, want the first flag's bare text", out)
	}
	wantLine(t, lastLines(t, ctx, 1)[0], LineStateCandidate, "", "")
}

// TestST_SubagentIsReadFromEitherSource (doubt 1): a call from a subagent
// is gated whether the caller set Client.Subagent or only AgentID, and the
// line carries the agent. Nothing of the note prints; the session's channel
// notice says why.
func TestST_SubagentIsReadFromEitherSource(t *testing.T) {
	ctx := routerContext(t)
	ctx.AgentID = "agent-7"
	want := Output{SystemMessage: "rashomon: a note was built and not sent to the agent (Claude Code 2.1.280, PostToolUse; " +
		"delivery verified from 2.1.280): notes on a subagent's calls are not sent yet. Shown once per session."}
	if out := Route(ctx, []Finding{regressionNote(t, "go test", 5, "a", LevelNudge)}); out != want {
		t.Errorf("a subagent's note printed %+v, want the channel notice alone", out)
	}
	l := lastLines(t, ctx, 1)[0]
	wantLine(t, l, LineStateSuppressed, SuppressedByUnverifiedChannel, DeliveryShadow)
	if l.AgentID == nil || *l.AgentID != "agent-7" {
		t.Errorf("agent_id = %v", l.AgentID)
	}
}

// TestR2_StopNeverContinues: Stop never sets StopContinue or a Decision in
// v0: a stop finding is a nudge without a project stop, and a nudge on Stop
// is a flag, with no reason recorded, since the event is the reason.
func TestR2_StopNeverContinues(t *testing.T) {
	ctx := routerContext(t)
	ctx.Event = EventStop
	f := regressionNote(t, "go test", 5, "a", LevelStop)
	f.StopForm = StopFormStopContinue
	out := Route(ctx, []Finding{f})
	if out.StopContinue != "" || out.Decision != nil || out.AdditionalContext != "" || out.SystemMessage != f.Message.Text() {
		t.Errorf("Stop printed %+v, want the bare text as a flag", out)
	}
	l := lastLines(t, ctx, 1)[0]
	if l.Level != "flag" || l.Channel != ChannelStop {
		t.Errorf("level %q, channel %q", l.Level, l.Channel)
	}
	wantLine(t, l, LineStateDelivered, "", DeliveryFlagOnly)
}

// TestTRU12b_UnverifiedChannelShipsTheNoteInShadow: on a client version
// below the probe's floor, the note ships in shadow, in the steer log only,
// and the session's first such call prints the channel notice and nothing of
// the note; a flag beside a later one still prints, flags being user-only.
func TestTRU12b_UnverifiedChannelShipsTheNoteInShadow(t *testing.T) {
	ctx := routerContext(t)
	ctx.Client = Client{Version: "2.1.279", Source: VersionSourceEnv, MirrorInBand: true}
	if out := Route(ctx, []Finding{regressionNote(t, "go test", 5, "a", LevelNudge)}); out != (Output{SystemMessage: belowNotice}) {
		t.Errorf("the unverified note printed %+v, want the channel notice alone", out)
	}
	wantLine(t, lastLines(t, ctx, 1)[0], LineStateSuppressed, SuppressedByUnverifiedChannel, DeliveryShadow)
	flag := regressionNote(t, "go vet", 6, "f", LevelFlag)
	out := Route(ctx, []Finding{regressionNote(t, "go test", 7, "b", LevelNudge), flag})
	if out.SystemMessage != flag.Message.Text() || out.AdditionalContext != "" {
		t.Errorf("Output = %+v, want the flag alone", out)
	}
}

// TestED9_UnattendedNudgeIsShadow: with nobody attending, a note is shadow
// (ED-9 as the PRD decided it: INV-5's mirror needs a reader), suppressed
// no_mirror; a looping session delivers.
func TestED9_UnattendedNudgeIsShadow(t *testing.T) {
	ctx := routerContext(t)
	ctx.Attendance = AttendanceUnattended
	if out := Route(ctx, []Finding{regressionNote(t, "go test", 5, "a", LevelNudge)}); out != (Output{}) {
		t.Errorf("an unattended note printed %+v", out)
	}
	l := lastLines(t, ctx, 1)[0]
	wantLine(t, l, LineStateSuppressed, SuppressedByNoMirror, DeliveryShadow)
	if l.Attendance != AttendanceUnattended {
		t.Errorf("attendance %q", l.Attendance)
	}
	ctx.Attendance = AttendanceLooping
	if out := Route(ctx, []Finding{regressionNote(t, "go test", 6, "b", LevelNudge)}); out.AdditionalContext == "" {
		t.Error("a looping session's note did not print")
	}
}

// TestST_ClampsRecordTheirReason (D50): each clamp alone turns a note into a
// delivered flag whose line names the clamp, delivery flag_only.
func TestST_ClampsRecordTheirReason(t *testing.T) {
	for _, c := range []struct {
		name string
		set  func(*Context)
		want SuppressedBy
	}{
		{"no consent", func(c *Context) { c.Consent = false }, SuppressedByConsentMissing},
		{"no prompt", func(c *Context) { c.PromptID = nil }, SuppressedByBudgetPrompt},
		{"stop hook active", func(c *Context) { c.Event, c.StopHookActive = EventStop, true }, SuppressedByStopHookActive},
	} {
		t.Run(c.name, func(t *testing.T) {
			ctx := routerContext(t)
			c.set(ctx)
			f := regressionNote(t, "go test", 5, "a", LevelNudge)
			if out := Route(ctx, []Finding{f}); out.SystemMessage != f.Message.Text() || out.AdditionalContext != "" {
				t.Errorf("Output = %+v, want the bare text as a flag", out)
			}
			l := lastLines(t, ctx, 1)[0]
			if l.Level != "flag" {
				t.Errorf("level %q", l.Level)
			}
			wantLine(t, l, LineStateDelivered, c.want, DeliveryFlagOnly)
		})
	}
}

// TestSTC1f_SteerOffSilencesNotesAndFlags (card STEER-CLI1): the user's steer
// switch prints nothing, a flag no more than a note, and logs every finding
// suppressed steer_off with delivery shadow. It is not a clamp to a flag: a
// person who turned the notes off is not shown a flag in their place.
func TestSTC1f_SteerOffSilencesNotesAndFlags(t *testing.T) {
	ctx := routerContext(t)
	ctx.SteerOff = true
	out := Route(ctx, []Finding{regressionNote(t, "go test", 5, "a", LevelNudge), regressionNote(t, "go vet", 6, "f", LevelFlag)})
	if out != (Output{}) {
		t.Errorf("steer off printed %+v", out)
	}
	for _, l := range lastLines(t, ctx, 2) {
		wantLine(t, l, LineStateSuppressed, SuppressedBySteerOff, DeliveryShadow)
	}
}

// TestST_ArmOffLogsAndPrintsNothing (step 4; STR-28): arm off evaluates and
// logs every finding, suppressed arm_off, and prints nothing, flags included.
func TestST_ArmOffLogsAndPrintsNothing(t *testing.T) {
	ctx := routerContext(t)
	ctx.Arm = ArmOff
	out := Route(ctx, []Finding{regressionNote(t, "go test", 5, "a", LevelNudge), regressionNote(t, "go vet", 6, "f", LevelFlag)})
	if out != (Output{}) {
		t.Errorf("arm off printed %+v", out)
	}
	for _, l := range lastLines(t, ctx, 2) {
		wantLine(t, l, LineStateSuppressed, SuppressedByArmOff, DeliveryShadow)
		if l.Arm != ArmOff {
			t.Errorf("arm %q", l.Arm)
		}
	}
}

// TestST_PreToolUsePrintsNothing: PreToolUse's systemMessage needs a
// Decision, and v0 issues none, so every finding is no_channel.
func TestST_PreToolUsePrintsNothing(t *testing.T) {
	ctx := routerContext(t)
	ctx.Event = EventPreToolUse
	if out := Route(ctx, []Finding{regressionNote(t, "go test", 5, "a", LevelNudge), regressionNote(t, "go vet", 6, "f", LevelFlag)}); out != (Output{}) {
		t.Errorf("PreToolUse printed %+v", out)
	}
	for _, l := range lastLines(t, ctx, 2) {
		wantLine(t, l, LineStateSuppressed, SuppressedByNoChannel, DeliveryShadow)
	}
}

// TestST_LevelOffWritesNoLine: an off finding is dropped with no line and no
// charge (step 5), and a call with nothing above off never takes the lock.
func TestST_LevelOffWritesNoLine(t *testing.T) {
	ctx := routerContext(t)
	off := regressionNote(t, "go test", 5, "off", LevelOff)
	if out := Route(ctx, []Finding{off}); out != (Output{}) {
		t.Errorf("an off finding printed %+v", out)
	}
	if lines := steerLines(t, ctx); len(lines) != 0 {
		t.Errorf("an off finding wrote %d lines", len(lines))
	}
	Route(ctx, []Finding{off, regressionNote(t, "go test", 6, "on", LevelNudge)})
	if lines := steerLines(t, ctx); len(lines) != 1 || lines[0].Key != "on" {
		t.Errorf("lines = %+v, want the on finding's alone", lines)
	}
}

// TestSTR8_NoStdoutWhenSteerLineCannotBeWritten: a call whose lines cannot
// be appended prints nothing (STR-8; tech 7.2.5 G5): the steer lock held by
// another handle past the call's lock wait is a lock timeout, counted, and
// the log is unchanged; a line over the record cap refuses the append after
// the decision to print was made; a nil store or Context prints nothing;
// and, in the fault build, a failed barrier after the append prints nothing.
func TestSTR8_NoStdoutWhenSteerLineCannotBeWritten(t *testing.T) {
	t.Run("barrier fault", str8BarrierFault)
	ctx := routerContext(t)
	other, err := store.Open(ctx.Store.Root())
	if err != nil {
		t.Fatal(err)
	}
	inside, release, done := make(chan struct{}), make(chan struct{}), make(chan error)
	go func() {
		done <- other.AppendSteer(ctx.SessionID, func(io.ReaderAt) ([]store.SteerLine, error) {
			close(inside)
			<-release
			return nil, nil
		})
	}()
	<-inside
	ctx.Store.LimitLockWaits(20 * time.Millisecond)
	before := LockTimeouts()
	out := Route(ctx, []Finding{regressionNote(t, "go test", 5, "a", LevelNudge)})
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if out != (Output{}) {
		t.Errorf("a lock timeout printed %+v", out)
	}
	if got := LockTimeouts() - before; got != 1 {
		t.Errorf("LockTimeouts rose by %d, want 1", got)
	}
	if lines := steerLines(t, ctx); len(lines) != 0 {
		t.Errorf("a lock timeout wrote %d lines", len(lines))
	}
	// A line over the store's record cap refuses the append after build
	// ran and decided to print: still nothing prints.
	huge := regressionNote(t, "go test", 200_000, "huge", LevelNudge)
	huge.Evidence = make([]int64, 200_000)
	for i := range huge.Evidence {
		huge.Evidence[i] = int64(i + 1)
	}
	fresh := routerContext(t)
	failures := AppendFailures()
	if out := Route(fresh, []Finding{huge}); out != (Output{}) {
		t.Errorf("an append refused after build printed %+v", out)
	}
	if got := AppendFailures() - failures; got != 1 {
		t.Errorf("AppendFailures rose by %d, want 1", got)
	}
	if lines := steerLines(t, fresh); len(lines) != 0 {
		t.Errorf("a refused append wrote %d lines", len(lines))
	}
	note := []Finding{regressionNote(t, "go test", 5, "a", LevelNudge)}
	noStore := routerContext(t)
	noStore.Store = nil
	if out := Route(noStore, note); out != (Output{}) {
		t.Errorf("a nil store printed %+v", out)
	}
	if out := Route(nil, note); out != (Output{}) {
		t.Errorf("a nil Context printed %+v", out)
	}
}

// TestST_RouterAndMarshallerReadOneTable: for every event and a table of
// clients, whenever decide marks a line delivered its Output marshals under
// MarshalFor without error, so what the log says printed is what can print
// (STR-10's property, read from one table; card 2.2 R4, R7).
func TestST_RouterAndMarshallerReadOneTable(t *testing.T) {
	clients := []Client{{}, verifiedClient, {Version: "2.1.279", Source: VersionSourceEnv, MirrorInBand: true}, {Version: verifiedClient.Version, Source: VersionSourceEnv, MirrorInBand: true, Subagent: true}}
	for _, e := range AllEvents() {
		for _, c := range clients {
			for _, att := range AllAttendance() {
				ctx := &Context{Event: e, SessionID: "s1", PromptID: ptr("p1"), Consent: true, Attendance: att, NewID: newID}
				findings := []Finding{regressionNote(t, "go test", 5, "a", LevelNudge), regressionNote(t, "go vet", 6, "f", LevelFlag)}
				out, lines := decide(ctx, c, findings, counts{}, newID)
				delivered := false
				for _, l := range lines {
					delivered = delivered || l.State == LineStateDelivered
				}
				if _, err := out.MarshalFor(string(e), c); delivered && err != nil {
					t.Errorf("%s, %+v, %s: decide delivered %+v and MarshalFor refused it: %v", e, c, att, out, err)
				}
				if !delivered && out != (Output{}) {
					t.Errorf("%s, %+v, %s: Output %+v with no delivered line", e, c, att, out)
				}
			}
		}
	}
}

// TestST_RenderRefusedLine: a finding with the zero Message is never
// delivered (Finding's doc): render_refused, nothing printed, as a note and
// as a flag.
func TestST_RenderRefusedLine(t *testing.T) {
	ctx := routerContext(t)
	for _, level := range []Level{LevelNudge, LevelFlag} {
		f := regressionNote(t, "go test", 5, "z"+level.String(), level)
		f.Message = render.Message{}
		if out := Route(ctx, []Finding{f}); out != (Output{}) {
			t.Errorf("a zero Message at %s printed %+v", level, out)
		}
		wantLine(t, lastLines(t, ctx, 1)[0], LineStateSuppressed, SuppressedByRenderRefused, DeliveryShadow)
	}
}

// TestST_LineCarriesTheBuildVersions (QA lens 3): a written line's
// rules_version is shape.RulesVersion and its renderer_version
// render.Version, read back through ReadSteer.
func TestST_LineCarriesTheBuildVersions(t *testing.T) {
	ctx := routerContext(t)
	Route(ctx, []Finding{regressionNote(t, "go test", 5, "a", LevelNudge)})
	l := lastLines(t, ctx, 1)[0]
	if l.RulesVersion != shape.RulesVersion || l.RendererVersion != render.Version || l.PatternVersion != 1 {
		t.Errorf("versions rules %d, renderer %d, pattern %d; want %d, %d, 1", l.RulesVersion, l.RendererVersion, l.PatternVersion, shape.RulesVersion, render.Version)
	}
}

// TestST_RouteWritesEachCallsLineToTheStore: two calls write two lines
// through the store, read back key for key (REC-42f's shape), every field
// R8 names filled from the Context and the finding.
func TestST_RouteWritesEachCallsLineToTheStore(t *testing.T) {
	ctx := routerContext(t)
	first := regressionNote(t, "go test", 5, "a", LevelNudge)
	Route(ctx, []Finding{first})
	ctx.NowUnixMS++
	Route(ctx, []Finding{regressionNote(t, "go vet", 9, "b", LevelFlag)})
	lines := steerLines(t, ctx)
	if len(lines) != 2 {
		t.Fatalf("%d lines, want 2", len(lines))
	}
	l := lines[0]
	if l.Type != LineType || l.V != LineVersion || l.ID == "" || l.SteerID != l.ID || l.SessionID != "s1" ||
		l.PromptID == nil || *l.PromptID != "p1" || l.AgentID != nil || l.Project != "proj" ||
		l.Pattern != string(pattern.RegressionLocation) || l.Key != "a" || l.Level != "nudge" ||
		l.Channel != ChannelPostToolUse || l.Event != string(EventPostToolUse) || l.Arm != ArmLive ||
		l.AfterSeq != 5 || len(l.EvidenceSeqs) != 1 || l.EvidenceSeqs[0] != 4 || string(l.Slots["gate"]) != `"go test"` ||
		l.HarnessID != "claude_code" || l.Surface != "cli" || l.AtUnixMS != 1_793_000_000_000 || l.LabelRule != nil {
		t.Errorf("the first line = %+v", l)
	}
	if lines[1].ID == l.ID || lines[1].AtUnixMS != l.AtUnixMS+1 || lines[1].Level != "flag" || lines[1].Mirror != MirrorSystemMessage {
		t.Errorf("the second line = %+v", lines[1])
	}
}
