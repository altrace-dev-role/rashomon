package steer

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// evNow is a Stop's clock for these tests: a minute after the router
// context's calls, so every delivered note has settled.
func evNow(ctx *Context) time.Time { return time.UnixMilli(ctx.NowUnixMS).Add(time.Minute) }

// bigLimits are limits no fixture here comes near.
func bigLimits(time.Time) ScanLimits { return DefaultScanLimits(time.Now()) }

// attachmentLine is a transcript record carrying a hook_additional_context
// attachment, in the form Claude Code writes: content a list of strings.
func attachmentLine(t *testing.T, typ string, content any) string {
	t.Helper()
	return attachmentAt(t, evStamp, "s1", typ, content)
}

// evStamp is the time of the records the tests' transcripts end with: hours
// after the router context's calls (07:33:20), so a transcript that holds one
// has caught up with every note.
const evStamp = "2026-10-26T12:00:00.000Z"

// attachmentAt is attachmentLine with its own timestamp and session.
func attachmentAt(t *testing.T, stamp, session, typ string, content any) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"type": "attachment", "timestamp": stamp, "sessionId": session,
		"attachment": map[string]any{"type": typ, "hookEvent": "PostToolUse", "hookName": "PostToolUse:Bash", "content": content},
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

const userLine = `{"type":"user","timestamp":"2026-10-26T00:00:00.000Z","message":{"role":"user","content":"hello"}}`

// writeTranscript writes the lines as a transcript and returns its path.
func writeTranscript(t *testing.T, lines ...string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "t.jsonl")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// deliver routes one nudge at the context's event and returns the note's text
// as the model got it.
func deliver(t *testing.T, ctx *Context, key string) string {
	t.Helper()
	out := Route(ctx, []Finding{regressionNote(t, "go test", 5, key, LevelNudge)})
	if out.AdditionalContext == "" {
		t.Fatalf("nothing delivered for %s on %s: %+v", ctx.Client.Version, ctx.Event, out)
	}
	return out.AdditionalContext
}

// evidenceOf is each evidence line of the session's log, by note id.
func evidenceOf(t *testing.T, ctx *Context) map[string]Evidence {
	t.Helper()
	raw, _, err := ctx.Store.ReadSteer(ctx.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Evidence{}
	for _, b := range raw {
		var l EvidenceLine
		if json.Unmarshal(b, &l) == nil && l.Type == EvidenceLineType {
			got[l.SteerID] = l.Evidence
		}
	}
	return got
}

// TestCE1_SeenAbsentUnreadable: a delivered note is seen when its text is in a
// hook_additional_context attachment, absent when the transcript was read and
// holds no such attachment, and unreadable when there is no transcript or it
// holds no record. Each outcome is a label on a line of the steer log keyed by
// the note's id, and the notes' own lines are not touched.
func TestCE1_SeenAbsentUnreadable(t *testing.T) {
	for _, tc := range []struct {
		name  string
		build func(t *testing.T, note string) string
		want  Evidence
	}{
		{"seen", func(t *testing.T, note string) string {
			return writeTranscript(t, userLine, attachmentLine(t, "hook_additional_context", []string{"unrelated", note}))
		}, EvidenceSeen},
		{"seen as a single string", func(t *testing.T, note string) string {
			return writeTranscript(t, userLine, attachmentLine(t, "hook_additional_context", "PostToolUse:Bash hook additional context: "+note))
		}, EvidenceSeen},
		{"absent", func(t *testing.T, _ string) string {
			return writeTranscript(t, userLine, attachmentLine(t, "hook_additional_context", []string{"another note"}))
		}, EvidenceAbsent},
		{"missing", func(t *testing.T, _ string) string { return filepath.Join(t.TempDir(), "gone.jsonl") }, EvidenceUnreadable},
		{"empty", func(t *testing.T, _ string) string { return writeTranscript(t) }, EvidenceUnreadable},
		{"not records", func(t *testing.T, _ string) string { return writeTranscript(t, "not json", "also not") }, EvidenceUnreadable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := routerContext(t)
			note := deliver(t, ctx, "a")
			before := steerLines(t, ctx)
			got := CheckEvidence(ctx.Store, ctx.SessionID, tc.build(t, note), evNow(ctx), bigLimits(evNow(ctx)))
			if len(got) != 1 || got[0].Evidence != tc.want || got[0].SteerID != before[0].ID || got[0].V != LineVersion {
				t.Fatalf("CheckEvidence = %+v, want one %q line for %s", got, tc.want, before[0].ID)
			}
			if ev := evidenceOf(t, ctx); len(ev) != 1 || ev[before[0].ID] != tc.want {
				t.Errorf("the log's evidence = %v, want %s on %s", ev, tc.want, before[0].ID)
			}
			raw, _, _ := ctx.Store.ReadSteer(ctx.SessionID)
			if len(raw) != 2 || string(raw[0]) != mustJSON(t, before[0]) {
				t.Errorf("the note's own line changed or the log has %d lines, want 2", len(raw))
			}
		})
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestCE1a_TheCheckKeysOnTheAttachmentAndNothingElse: the note's text in a
// user message, a hook_system_message attachment or any other record is not
// a delivered note, so none of them reads as seen. The text is compared
// decoded, so a note with quotes and a line feed is found.
func TestCE1a_TheCheckKeysOnTheAttachmentAndNothingElse(t *testing.T) {
	ctx := routerContext(t)
	note := deliver(t, ctx, "a")
	quoted := `{"type":"user","message":{"content":` + mustJSON(t, note) + `}}`
	path := writeTranscript(t, userLine, quoted,
		attachmentLine(t, "hook_system_message", []string{note}),
		attachmentLine(t, "hook_success", []string{note}),
		// A record that names the type in another field, so the cheap filter
		// passes it: only the attachment's own type counts.
		`{"type":"attachment","timestamp":"`+evStamp+`","attachment":{"type":"hook_success","about":"hook_additional_context","content":[`+mustJSON(t, note)+`]}}`)
	got := CheckEvidence(ctx.Store, ctx.SessionID, path, evNow(ctx), bigLimits(evNow(ctx)))
	if len(got) != 1 || got[0].Evidence != EvidenceAbsent {
		t.Errorf("the text outside a hook_additional_context attachment read as %+v, want absent", got)
	}

	ctx2 := routerContext(t)
	multi := "rashomon record: gate \"go test\" exited 0\nsecond line\ttabbed"
	l := Line{Type: LineType, V: LineVersion, ID: "m1", AtUnixMS: ctx2.NowUnixMS, SessionID: ctx2.SessionID, State: LineStateDelivered,
		ModelText: multi, Event: "PostToolUse", ClientVersion: "2.1.280", VerifiedFloor: "2.1.280"}
	appendLine(t, ctx2, &l)
	path = writeTranscript(t, userLine, attachmentLine(t, "hook_additional_context", []string{"x " + `rashomon record: gate "go test" exited 0`}))
	got = CheckEvidence(ctx2.Store, ctx2.SessionID, path, evNow(ctx2), bigLimits(evNow(ctx2)))
	if len(got) != 1 || got[0].Evidence != EvidenceSeen {
		t.Errorf("a first line with quotes read as %+v, want seen", got)
	}
}

// appendLine appends one steer line to the context's session.
func appendLine(t *testing.T, ctx *Context, l *Line) {
	t.Helper()
	err := ctx.Store.AppendSteer(ctx.SessionID, func(io.ReaderAt) ([]store.SteerLine, error) { return []store.SteerLine{l}, nil })
	if err != nil {
		t.Fatal(err)
	}
}

// absentTwice delivers a note, reads it absent at one Stop, delivers a second,
// and reads that absent at a later Stop, the two readings that write the
// marker. The first reading must not have written it.
func absentTwice(t *testing.T, ctx *Context) {
	t.Helper()
	path := writeTranscript(t, userLine, attachmentLine(t, "hook_additional_context", []string{"another note"}))
	deliver(t, ctx, "a")
	if got := CheckEvidence(ctx.Store, ctx.SessionID, path, evNow(ctx), bigLimits(evNow(ctx))); len(got) != 1 || got[0].Evidence != EvidenceAbsent || got[0].AbsentStreak != 1 {
		t.Fatalf("first Stop: CheckEvidence = %+v, want absent, streak 1", got)
	}
	if _, ok := ctx.Store.ChannelEvidence("PostToolUse"); ok {
		t.Fatal("one absent reading wrote the marker")
	}
	deliver(t, ctx, "b")
	if got := CheckEvidence(ctx.Store, ctx.SessionID, path, evNow(ctx), bigLimits(evNow(ctx))); len(got) != 1 || got[0].Evidence != EvidenceAbsent || got[0].AbsentStreak != 2 {
		t.Fatalf("second Stop: CheckEvidence = %+v, want absent, streak 2", got)
	}
}

// TestCE1b_AbsentShadowsNextSessionOnce: a note absent from the transcript at
// two Stops on a version the floor trusts writes the event's marker with that
// version.
// The next session's note on that event is shadowed with unverified_channel,
// and its one notice line gives the reason; its second prints nothing. A
// different event is not shadowed.
func TestCE1b_AbsentShadowsNextSessionOnce(t *testing.T) {
	ctx := routerContext(t)
	absentTwice(t, ctx)
	if v, ok := ctx.Store.ChannelEvidence("PostToolUse"); !ok || v != "2.1.280" {
		t.Fatalf("marker = (%q, %v), want (2.1.280, true)", v, ok)
	}

	next := *ctx
	next.SessionID = "s2"
	const why = "rashomon: a note was built and not sent to the agent (Claude Code 2.1.280, PostToolUse; delivery verified from 2.1.280): " +
		"a transcript did not show a delivered note for this event. Shown once per session."
	if out := Route(&next, []Finding{regressionNote(t, "go test", 5, "a", LevelNudge)}); out != (Output{SystemMessage: why}) {
		t.Errorf("the next session's first note printed %+v, want only %q", out, why)
	}
	if out := Route(&next, []Finding{regressionNote(t, "go vet", 6, "b", LevelNudge)}); out != (Output{}) {
		t.Errorf("the next session's second note printed %+v, want nothing", out)
	}
	for i, l := range lastLines(t, &next, 2) {
		wantLine(t, l, LineStateSuppressed, SuppressedByUnverifiedChannel, DeliveryShadow)
		if (i == 0) != (l.Notice != nil) {
			t.Errorf("line %d notice = %v, want it on the first only", i, l.Notice)
		}
	}

	other := *ctx
	other.SessionID, other.Event = "s3", EventPostToolUseFailure
	other.Client.Version = "2.1.304"
	deliver(t, &other, "a")
}

// TestCE1b_OnlyATrustedVersionShadows: an absent reading on a version below
// the floor, on an excluded one, or on an event with no floor writes no
// marker; it still labels the note.
func TestCE1b_OnlyATrustedVersionShadows(t *testing.T) {
	path := func(t *testing.T) string {
		return writeTranscript(t, userLine, attachmentLine(t, "hook_additional_context", []string{"another note"}))
	}
	for _, tc := range []struct {
		name, version, floor string
		excluded             []string
		event                string
		trusted              bool
	}{
		{"trusted", "2.1.280", "2.1.280", nil, "PostToolUse", true},
		{"newer than the floor", "2.1.314", "2.1.280", nil, "PostToolUse", true},
		{"below the floor", "2.1.279", "2.1.280", nil, "PostToolUse", false},
		{"excluded", "2.1.300", "2.1.280", []string{"2.1.300"}, "PostToolUse", false},
		{"no floor", "2.1.314", "", nil, "UserPromptSubmit", false},
		{"unknown version", "", "2.1.280", nil, "PostToolUse", false},
		{"malformed version", "2.1.x", "2.1.280", nil, "PostToolUse", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := routerContext(t)
			for _, id := range []string{"n1", "n2"} {
				l := Line{Type: LineType, V: LineVersion, ID: id, AtUnixMS: ctx.NowUnixMS, SessionID: ctx.SessionID, State: LineStateDelivered,
					ModelText: "rashomon record: a note", Event: tc.event, ClientVersion: tc.version, VerifiedFloor: tc.floor, Excluded: tc.excluded}
				appendLine(t, ctx, &l)
				got := CheckEvidence(ctx.Store, ctx.SessionID, path(t), evNow(ctx), bigLimits(evNow(ctx)))
				if len(got) != 1 || got[0].Evidence != EvidenceAbsent {
					t.Fatalf("CheckEvidence = %+v, want absent", got)
				}
			}
			_, shadowed := ctx.Store.ChannelEvidence(tc.event)
			if shadowed != tc.trusted {
				t.Errorf("marker for %s = %v, want %v", tc.event, shadowed, tc.trusted)
			}
		})
	}
}

// TestCE1b_AnUnreadableTranscriptChangesNoState: an unreadable reading writes
// no marker, and a transcript that shows the note writes none either.
func TestCE1b_AnUnreadableTranscriptChangesNoState(t *testing.T) {
	for name, path := range map[string]func(t *testing.T, note string) string{
		"missing": func(t *testing.T, _ string) string { return filepath.Join(t.TempDir(), "gone.jsonl") },
		"seen": func(t *testing.T, note string) string {
			return writeTranscript(t, userLine, attachmentLine(t, "hook_additional_context", []string{note}))
		},
	} {
		ctx := routerContext(t)
		note := deliver(t, ctx, "a")
		CheckEvidence(ctx.Store, ctx.SessionID, path(t, note), evNow(ctx), bigLimits(evNow(ctx)))
		if _, ok := ctx.Store.ChannelEvidence("PostToolUse"); ok {
			t.Errorf("%s: a marker was written", name)
		}
		next := *ctx
		next.SessionID = "s2"
		deliver(t, &next, "a")
	}
}

// TestCE1c_ProbePassAndNewerVersionClear: the marker shadows until it is
// removed, which is what a probe pass does, or until Claude Code is newer than
// the version that shadowed, which a routed call does for itself. The same or
// an older version stays shadowed.
func TestCE1c_ProbePassAndNewerVersionClear(t *testing.T) {
	shadow := func(t *testing.T) *Context {
		ctx := routerContext(t)
		absentTwice(t, ctx)
		if _, ok := ctx.Store.ChannelEvidence("PostToolUse"); !ok {
			t.Fatal("no marker")
		}
		next := *ctx
		next.SessionID = "s2"
		return &next
	}
	blocked := func(t *testing.T, ctx *Context) {
		t.Helper()
		if out := Route(ctx, []Finding{regressionNote(t, "go test", 5, "z", LevelNudge)}); out.AdditionalContext != "" {
			t.Errorf("%s delivered through the marker: %+v", ctx.Client.Version, out)
		}
	}

	ctx := shadow(t)
	blocked(t, ctx)
	ctx.Store.ClearChannelEvidence("PostToolUse") // the probe's removal
	ctx.SessionID = "s3"
	deliver(t, ctx, "a")

	ctx = shadow(t)
	ctx.Client.Version = "2.1.314"
	deliver(t, ctx, "a")
	if _, ok := ctx.Store.ChannelEvidence("PostToolUse"); ok {
		t.Error("a newer version delivered and left the marker")
	}

	ctx = shadow(t)
	ctx.Client.Version = "2.1.281"
	ctx.SessionID = "s4"
	deliver(t, ctx, "a")

	ctx = shadow(t)
	ctx.Client.Version = "2.1.280"
	blocked(t, ctx)
	if _, ok := ctx.Store.ChannelEvidence("PostToolUse"); !ok {
		t.Error("the same version cleared the marker")
	}
}

// TestCE1d_ANoteIsJudgedOnce: a note that has an evidence line is not read
// again, an unreadable one included; a note younger than the settle time waits
// for the next Stop; a Stop with nothing to judge opens no transcript.
func TestCE1d_ANoteIsJudgedOnce(t *testing.T) {
	ctx := routerContext(t)
	note := deliver(t, ctx, "a")
	young := time.UnixMilli(ctx.NowUnixMS).Add(time.Second)
	missing := filepath.Join(t.TempDir(), "gone.jsonl")
	if got := CheckEvidence(ctx.Store, ctx.SessionID, missing, young, bigLimits(young)); got != nil {
		t.Errorf("a note %s old was judged: %+v", time.Second, got)
	}
	if got := CheckEvidence(ctx.Store, ctx.SessionID, missing, evNow(ctx), bigLimits(evNow(ctx))); len(got) != 1 || got[0].Evidence != EvidenceUnreadable {
		t.Fatalf("settled note: %+v, want unreadable", got)
	}
	good := writeTranscript(t, userLine, attachmentLine(t, "hook_additional_context", []string{note}))
	if got := CheckEvidence(ctx.Store, ctx.SessionID, good, evNow(ctx), bigLimits(evNow(ctx))); got != nil {
		t.Errorf("a judged note was read again: %+v", got)
	}
	if raw, _, _ := ctx.Store.ReadSteer(ctx.SessionID); len(raw) != 2 {
		t.Errorf("the log holds %d lines, want the note and one evidence line", len(raw))
	}
}

// TestCE1d_ASubagentsNoteIsNotJudged: a note delivered on a subagent's call is
// in the subagent's own transcript, which the check does not read, so it
// gets no evidence line and can shadow nothing.
func TestCE1d_ASubagentsNoteIsNotJudged(t *testing.T) {
	ctx := routerContext(t)
	agent := "agent-1"
	l := Line{Type: LineType, V: LineVersion, ID: "sub1", AtUnixMS: ctx.NowUnixMS, SessionID: ctx.SessionID, AgentID: &agent,
		State: LineStateDelivered, ModelText: "rashomon record: a subagent note", Event: "PostToolUse",
		ClientVersion: "2.1.280", VerifiedFloor: "2.1.280"}
	appendLine(t, ctx, &l)
	if got := CheckEvidence(ctx.Store, ctx.SessionID, writeTranscript(t, userLine), evNow(ctx), bigLimits(evNow(ctx))); got != nil {
		t.Errorf("a subagent's note was judged: %+v", got)
	}
	if _, ok := ctx.Store.ChannelEvidence("PostToolUse"); ok {
		t.Error("a subagent's note shadowed the event")
	}
}

// TestCE1d_EvidenceLinesAreNotNotes: an evidence line carries the note's
// prompt and no state, so the router's budget still counts the prompt's
// notes alone: with three delivered and judged, a fourth is held back as
// before.
func TestCE1d_EvidenceLinesAreNotNotes(t *testing.T) {
	ctx := routerContext(t)
	for i, key := range []string{"a", "b", "c"} {
		if out := Route(ctx, []Finding{regressionNote(t, "go test", int64(5+i), key, LevelNudge)}); out.AdditionalContext == "" {
			t.Fatalf("note %s was not delivered", key)
		}
	}
	if got := CheckEvidence(ctx.Store, ctx.SessionID, writeTranscript(t, userLine), evNow(ctx), bigLimits(evNow(ctx))); len(got) != 3 {
		t.Fatalf("CheckEvidence judged %d notes, want 3", len(got))
	}
	if out := Route(ctx, []Finding{regressionNote(t, "go vet", 9, "d", LevelNudge)}); out.AdditionalContext != "" {
		t.Errorf("a fourth note in the prompt was delivered after the evidence lines: %+v", out)
	}
	if last := lastLines(t, ctx, 1)[0]; last.State == LineStateDelivered {
		t.Errorf("the fourth note's line is %q, want it held back", last.State)
	}
}

// bounded transcripts

// fillerLine is one record of about n bytes.
func fillerLine(n int, stamp string) string {
	return `{"type":"user","timestamp":"` + stamp + `","message":{"content":"` + strings.Repeat("x", n) + `"}}`
}

// TestCE1e_TheReadIsBounded: a transcript longer than the byte bound is read
// from its end. A note found there is seen; a note not found is absent only if
// the window starts at or before the note's time, and unreadable otherwise;
// the deadline makes it unreadable; a line over the line bound is stepped
// over.
func TestCE1e_TheReadIsBounded(t *testing.T) {
	noteAt := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	other := attachmentLine(t, "hook_additional_context", []string{"other"})
	filler := func(stamp string) []string {
		out := make([]string, 0, 40)
		for range 40 {
			out = append(out, fillerLine(500, stamp))
		}
		return out
	}
	lim := ScanLimits{MaxBytes: 4096, MaxLine: 1 << 20}

	covered := writeTranscript(t, append(filler("2026-10-25T00:00:00.000Z"), other)...)
	// The window is the last 4096 bytes: filler records stamped before the note.
	if _, complete := ScanTranscript(covered, "s1", []Needle{{"needle", noteAt}}, lim); !complete {
		t.Error("a window starting before the note's time was read as not covering it")
	}
	late := writeTranscript(t, append(filler("2026-10-27T00:00:00.000Z"), other)...)
	if found, complete := ScanTranscript(late, "s1", []Needle{{"needle", noteAt}}, lim); complete || found[0] {
		t.Errorf("a window starting after the note's time: found %v, complete %v; want neither", found, complete)
	}
	hit := writeTranscript(t, append(filler("2026-10-27T00:00:00.000Z"), attachmentLine(t, "hook_additional_context", []string{"a needle here"}))...)
	if found, _ := ScanTranscript(hit, "s1", []Needle{{"needle", noteAt}}, lim); !found[0] {
		t.Error("a needle inside the window was not found")
	}
	nostamp := writeTranscript(t, append(strings.Split(strings.Repeat(`{"type":"user"}`+"\n", 400), "\n"), other)...)
	if _, complete := ScanTranscript(nostamp, "s1", []Needle{{"needle", noteAt}}, ScanLimits{MaxBytes: 1024, MaxLine: 1 << 20}); complete {
		t.Error("a window with no timestamp was read as covering the note")
	}

	expired := ScanLimits{MaxBytes: 1 << 20, MaxLine: 1 << 20, Deadline: time.Now().Add(-time.Second)}
	if found, complete := ScanTranscript(covered, "s1", []Needle{{"needle", noteAt}}, expired); complete || found[0] {
		t.Errorf("an expired deadline: found %v, complete %v; want neither", found, complete)
	}

	huge := writeTranscript(t, userLine, fillerLine(3<<10, "2026-10-26T00:00:00.000Z"), attachmentLine(t, "hook_additional_context", []string{"a needle"}))
	if found, complete := ScanTranscript(huge, "s1", []Needle{{"needle", noteAt}}, ScanLimits{MaxBytes: 1 << 20, MaxLine: 2 << 10}); !found[0] || !complete {
		t.Errorf("a line over the line bound ended the read: found %v, complete %v", found, complete)
	}
	if found, complete := ScanTranscript(t.TempDir(), "s1", []Needle{{"needle", noteAt}}, lim); found[0] || complete {
		t.Errorf("a directory read as a transcript: %v %v", found, complete)
	}
}

// TestCE1e_DefaultLimitsAreTheStopsBound pins the numbers the card's budget
// row rests on.
func TestCE1e_DefaultLimitsAreTheStopsBound(t *testing.T) {
	now := time.Unix(1_793_000_000, 0)
	l := DefaultScanLimits(now)
	if l.MaxBytes != 16<<20 || l.MaxLine != 1<<20 || l.Deadline.Sub(now) != time.Second {
		t.Errorf("DefaultScanLimits = %+v", l)
	}
	if evidenceSettle != 2*time.Second {
		t.Errorf("settle = %v", evidenceSettle)
	}
}

// TestCE1f_ALaggingTranscriptIsUnreadable: a transcript whose last record is
// older than the note has not caught up with it, so a note it lacks is not
// absent: unreadable, no marker, however many Stops read it. The reviewer's
// probe: one delivered note and a transcript whose one record is from 2020.
func TestCE1f_ALaggingTranscriptIsUnreadable(t *testing.T) {
	ctx := routerContext(t)
	old := writeTranscript(t, attachmentAt(t, "2020-01-01T00:00:00.000Z", "s1", "hook_additional_context", []string{"another note"}))
	for _, key := range []string{"a", "b", "c"} {
		deliver(t, ctx, key)
		got := CheckEvidence(ctx.Store, ctx.SessionID, old, evNow(ctx), bigLimits(evNow(ctx)))
		if len(got) != 1 || got[0].Evidence != EvidenceUnreadable || got[0].AbsentStreak != 0 {
			t.Fatalf("a transcript from 2020 read as %+v, want unreadable", got)
		}
	}
	if _, ok := ctx.Store.ChannelEvidence("PostToolUse"); ok {
		t.Error("a lagging transcript wrote the marker")
	}
	// The same transcript, with its last record at the note's own instant,
	// is still short of the slack.
	at := time.UnixMilli(ctx.NowUnixMS).UTC().Format("2006-01-02T15:04:05.000Z")
	ctx2 := routerContext(t)
	deliver(t, ctx2, "a")
	edge := writeTranscript(t, attachmentAt(t, at, "s1", "hook_additional_context", []string{"another note"}))
	if got := CheckEvidence(ctx2.Store, ctx2.SessionID, edge, evNow(ctx2), bigLimits(evNow(ctx2))); len(got) != 1 || got[0].Evidence != EvidenceUnreadable {
		t.Errorf("a last record at the note's time read as %+v, want unreadable", got)
	}
}

// TestCE1f_TwoAbsentsWriteTheMarker: the first absent reading records absent
// with streak 1 and writes nothing; a second at a later Stop writes it.
func TestCE1f_TwoAbsentsWriteTheMarker(t *testing.T) {
	ctx := routerContext(t)
	absentTwice(t, ctx)
	if _, ok := ctx.Store.ChannelEvidence("PostToolUse"); !ok {
		t.Error("two absent readings wrote no marker")
	}
	raw, _, _ := ctx.Store.ReadSteer(ctx.SessionID)
	var streaks []int
	for _, b := range raw {
		var l EvidenceLine
		if json.Unmarshal(b, &l) == nil && l.Type == EvidenceLineType {
			streaks = append(streaks, l.AbsentStreak)
		}
	}
	if len(streaks) != 2 || streaks[0] != 1 || streaks[1] != 2 {
		t.Errorf("absent_streak on the log = %v, want [1 2]", streaks)
	}
}

// TestCE1f_AnUnreadableBetweenAbsentsKeepsTheCount: an unreadable reading
// changes nothing, so absent, unreadable, absent writes the marker.
func TestCE1f_AnUnreadableBetweenAbsentsKeepsTheCount(t *testing.T) {
	ctx := routerContext(t)
	read := func(key, path string) []EvidenceLine {
		deliver(t, ctx, key)
		return CheckEvidence(ctx.Store, ctx.SessionID, path, evNow(ctx), bigLimits(evNow(ctx)))
	}
	absent := writeTranscript(t, userLine, attachmentLine(t, "hook_additional_context", []string{"another note"}))
	read("a", absent)
	if got := read("b", filepath.Join(t.TempDir(), "gone.jsonl")); len(got) != 1 || got[0].Evidence != EvidenceUnreadable {
		t.Fatalf("got %+v, want unreadable", got)
	}
	read("c", absent)
	if _, ok := ctx.Store.ChannelEvidence("PostToolUse"); !ok {
		t.Error("an unreadable between two absents reset the count")
	}
}

// TestCE1f_AbsentThenSeenWritesNone: a seen reading between absents starts
// the count again, and so does a seen note in the same Stop as an absent one.
func TestCE1f_AbsentThenSeenWritesNone(t *testing.T) {
	ctx := routerContext(t)
	absent := writeTranscript(t, userLine, attachmentLine(t, "hook_additional_context", []string{"another note"}))
	deliver(t, ctx, "a")
	if got := CheckEvidence(ctx.Store, ctx.SessionID, absent, evNow(ctx), bigLimits(evNow(ctx))); len(got) != 1 || got[0].Evidence != EvidenceAbsent {
		t.Fatalf("got %+v, want absent", got)
	}
	note := deliver(t, ctx, "b")
	seen := writeTranscript(t, userLine, attachmentLine(t, "hook_additional_context", []string{note}))
	if got := CheckEvidence(ctx.Store, ctx.SessionID, seen, evNow(ctx), bigLimits(evNow(ctx))); len(got) != 1 || got[0].Evidence != EvidenceSeen {
		t.Fatalf("got %+v, want seen", got)
	}
	deliver(t, ctx, "c")
	if got := CheckEvidence(ctx.Store, ctx.SessionID, absent, evNow(ctx), bigLimits(evNow(ctx))); len(got) != 1 || got[0].Evidence != EvidenceAbsent || got[0].AbsentStreak != 1 {
		t.Fatalf("after a seen: %+v, want absent with streak 1", got)
	}
	if _, ok := ctx.Store.ChannelEvidence("PostToolUse"); ok {
		t.Error("absent, seen, absent wrote the marker")
	}
}

// TestCE1f_AnAttachmentBeforeTheNoteIsNotSeen: the first line of many notes is
// the same, so an attachment recorded before this note was delivered is
// another note's, however well its text matches.
func TestCE1f_AnAttachmentBeforeTheNoteIsNotSeen(t *testing.T) {
	ctx := routerContext(t)
	note := deliver(t, ctx, "a")
	before := attachmentAt(t, "2026-10-26T07:00:00.000Z", "s1", "hook_additional_context", []string{note}) // note is at 07:33:20
	after := attachmentAt(t, evStamp, "s1", "hook_additional_context", []string{"another note"})
	got := CheckEvidence(ctx.Store, ctx.SessionID, writeTranscript(t, userLine, before, after), evNow(ctx), bigLimits(evNow(ctx)))
	if len(got) != 1 || got[0].Evidence != EvidenceAbsent {
		t.Errorf("an attachment from before the note read as %+v, want absent", got)
	}
	ctx2 := routerContext(t)
	note = deliver(t, ctx2, "a")
	on := attachmentAt(t, "2026-10-26T07:33:21.000Z", "s1", "hook_additional_context", []string{note})
	got = CheckEvidence(ctx2.Store, ctx2.SessionID, writeTranscript(t, userLine, on, after), evNow(ctx2), bigLimits(evNow(ctx2)))
	if len(got) != 1 || got[0].Evidence != EvidenceSeen {
		t.Errorf("an attachment after the note read as %+v, want seen", got)
	}
}

// TestCE1f_AnotherSessionsTranscriptIsUnreadable: a record that names another
// session, in the middle of the file or at its end, makes the read
// unreadable, even where the note's text is in it.
func TestCE1f_AnotherSessionsTranscriptIsUnreadable(t *testing.T) {
	for name, build := range map[string]func(t *testing.T, note string) string{
		"in an attachment": func(t *testing.T, note string) string {
			return writeTranscript(t, userLine, attachmentAt(t, evStamp, "other-session", "hook_additional_context", []string{note}))
		},
		"at the end": func(t *testing.T, _ string) string {
			return writeTranscript(t, userLine, attachmentLine(t, "hook_additional_context", []string{"x"}),
				`{"type":"user","timestamp":"`+evStamp+`","sessionId":"other-session"}`)
		},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := routerContext(t)
			note := deliver(t, ctx, "a")
			got := CheckEvidence(ctx.Store, ctx.SessionID, build(t, note), evNow(ctx), bigLimits(evNow(ctx)))
			if len(got) != 1 || got[0].Evidence != EvidenceUnreadable {
				t.Errorf("another session's transcript read as %+v, want unreadable", got)
			}
		})
	}
}

// TestCE1f_ASeenNoteInTheSameStopClearsTheCount: a Stop that reads one note of
// an event seen and another absent has shown the channel working, so it is no
// absent reading of the event: after an absent Stop, a mixed one writes no
// marker.
func TestCE1f_ASeenNoteInTheSameStopClearsTheCount(t *testing.T) {
	ctx := routerContext(t)
	absent := writeTranscript(t, userLine, attachmentLine(t, "hook_additional_context", []string{"another note"}))
	deliver(t, ctx, "a")
	CheckEvidence(ctx.Store, ctx.SessionID, absent, evNow(ctx), bigLimits(evNow(ctx)))
	seenNote := deliver(t, ctx, "b") // at 07:33:20
	ctx.NowUnixMS += 10_000
	deliver(t, ctx, "c") // at 07:33:30; the notes share their first line, so only the time tells them apart
	mixed := writeTranscript(t, userLine, attachmentAt(t, "2026-10-26T07:33:25.000Z", "s1", "hook_additional_context", []string{seenNote}),
		attachmentLine(t, "hook_additional_context", []string{"another note"}))
	got := CheckEvidence(ctx.Store, ctx.SessionID, mixed, evNow(ctx), bigLimits(evNow(ctx)))
	if len(got) != 2 {
		t.Fatalf("judged %d notes, want 2", len(got))
	}
	seen, absentN := 0, 0
	for _, l := range got {
		switch l.Evidence {
		case EvidenceSeen:
			seen++
		case EvidenceAbsent:
			absentN++
		}
	}
	if seen+absentN != 2 {
		t.Fatalf("got %+v, want one seen and one absent", got)
	}
	if _, ok := ctx.Store.ChannelEvidence("PostToolUse"); ok {
		t.Error("a Stop that saw a note of the event counted as an absent reading")
	}
}
