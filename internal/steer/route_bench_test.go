package steer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// benchRouteLog is a 6,000-line steer log of session s1: 120 prompts of 50
// lines each, every line the shape Route writes (slots, versions, and text
// on the delivered ones), the last prompt, p119, the one a call counts.
// Each prompt delivers two notes and three flags; the rest are suppressed.
func benchRouteLog(tb testing.TB) []byte {
	tb.Helper()
	m, err := render.New(pattern.RegressionLocation, render.RegressionLocationBinding{
		Gate: "go test", Minutes: 6, Exit: 1,
		Paths: slot.NewPathList([]slot.Path{"internal/verify/a.go", "internal/verify/b.go", "cmd/rashomon/main.go"}, 2),
	})
	if err != nil {
		tb.Fatal(err)
	}
	slots, err := slotsOf(m)
	if err != nil {
		tb.Fatal(err)
	}
	mirror, err := render.Mirror(m.Text())
	if err != nil {
		tb.Fatal(err)
	}
	var log bytes.Buffer
	for p := range 120 {
		prompt := fmt.Sprint("p", p)
		for i := range 50 {
			l := Line{
				Type: LineType, V: LineVersion, ID: fmt.Sprintf("%032x", p*50+i), AtUnixMS: int64(1_793_000_000_000 + p*50 + i),
				SessionID: "s1", PromptID: &prompt, Project: "proj", Pattern: string(pattern.RegressionLocation),
				Key: fmt.Sprint("gate|", i), PatternVersion: 1, Level: "nudge", Channel: ChannelPostToolUse,
				Event: string(EventPostToolUse), Attendance: AttendanceAttended, Arm: ArmLive, AfterSeq: int64(i + 2),
				EvidenceSeqs: []int64{int64(i + 1)}, Slots: slots, State: LineStateSuppressed,
				SuppressedBy: reason(SuppressedByBudgetPrompt), RendererVersion: render.Version, RulesVersion: 6,
				HarnessID: "claude_code", Surface: "cli", Mirror: MirrorNone,
			}
			l.SteerID = l.ID
			switch {
			case i < 2:
				l.State, l.SuppressedBy, l.ModelText, l.UserText, l.Mirror = LineStateDelivered, nil, m.Text(), mirror, MirrorSystemMessage
			case i < 5:
				l.State, l.SuppressedBy, l.Level, l.UserText, l.Mirror = LineStateDelivered, nil, "flag", m.Text(), MirrorSystemMessage
			}
			l.Delivery = DeriveDelivery(l.State, LevelNudge, l.SuppressedBy, false)
			b, err := json.Marshal(l)
			if err != nil {
				tb.Fatal(err)
			}
			log.Write(b)
			log.WriteByte('\n')
		}
	}
	return log.Bytes()
}

// BenchmarkRoute measures the router's two pure costs per routed call (card
// 2.2 premise 11): count_6000 is the reverse scan of a 6,000-line log whose
// current prompt holds the last 50 lines (it reads those 50 and the line
// before them, never the log); decide_3 is the decision over three admitted
// findings, a note, a lower note and a flag, with ids minted outside. The
// lock, the append and the sync are the store's own rows.
func BenchmarkRoute(b *testing.B) {
	log := benchRouteLog(b)
	b.Run("count_6000", func(b *testing.B) {
		ctx := &Context{SessionID: "s1", PromptID: ptr("p119")}
		r := bytes.NewReader(log)
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			c, err := countBudget(r, ctx)
			if err != nil || c.notes != 2 || c.flags != 3 {
				b.Fatalf("counts = %+v, %v", c, err)
			}
		}
	})
	b.Run("decide_3", func(b *testing.B) {
		findings := []Finding{benchNote(b, "go test", 5, "a", LevelNudge), benchNote(b, "go vet", 6, "b", LevelNudge), benchNote(b, "go build", 7, "f", LevelFlag)}
		admitted := make([]Finding, len(findings))
		ctx := &Context{Event: EventPostToolUse, SessionID: "s1", PromptID: ptr("p1"), Consent: true, NowUnixMS: 1}
		id := func() string { return "00000000000000000000000000000001" }
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			copy(admitted, findings)
			out, lines := decide(ctx, verifiedClient, admitted, counts{}, id)
			if out.AdditionalContext == "" || len(lines) != 3 {
				b.Fatalf("decide = %+v, %d lines", out, len(lines))
			}
		}
	})
}

func benchNote(tb testing.TB, gate string, afterSeq int64, key string, level Level) Finding {
	tb.Helper()
	m, err := render.New(pattern.RegressionLocation, render.RegressionLocationBinding{
		Gate: slot.Gate(gate), Minutes: 6, Exit: 1, Paths: slot.NewPathList([]slot.Path{"internal/a.go"}, 0),
	})
	if err != nil {
		tb.Fatal(err)
	}
	return Finding{Pattern: pattern.RegressionLocation, Key: key, Priority: 80, MaxLevel: level, StopForm: StopFormNone,
		Message: m, Evidence: []int64{afterSeq - 1}, AfterSeq: afterSeq}
}
