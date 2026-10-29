package main

import (
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/replay"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// replayDoc replays a synthetic document into a fresh world.
func replayDoc(t *testing.T, src io.ReadSeeker, opt replay.Options) (*replayResult, replayed) {
	t.Helper()
	if opt.TranscriptPath == "" {
		opt.TranscriptPath = tPath
	}
	w := newWorld(t)
	res, err := w.replayTranscript(src, opt, replayConfig{KeepOutput: true})
	if err != nil {
		t.Fatal(err)
	}
	return res, replayed{root: w.root, world: w, outputs: res.Outputs}
}

// lateResultDoc is two turns in which a result lands after the first turn's
// Stop. Its lines:
//
//	1 prompt
//	2 two tool_use blocks in one line: a Write (w1) and a test run (t1)
//	3 the Write's result
//	4 the turn ends, claiming the tests pass: STOP 1. t1 has no result yet.
//	5 t1's result, ok: after Stop 1.
//	6 a second prompt
//	7 a second test run (t2)
//	8 its result: a failure
//	9 the turn ends, claiming the tests pass: STOP 2.
func lateResultDoc() *tdoc {
	d := newTdoc()
	d.promptLine("p1")
	d.uses("m1", tWrite("w1", "/repo/calc.py"), tBash("t1", "python3 -m unittest"))
	d.results("", tOK("w1"))
	d.ends("m2", "I made the change and all the tests pass.")
	d.results("", tOK("t1"))
	d.promptLine("p2")
	d.uses("m3", tBash("t2", "python3 -m unittest"))
	d.results("", tFail("t2"))
	d.ends("m4", "Done, all the tests pass.")
	return d
}

// TestREPLAY13_AStopSeesOnlyTheRecordsBeforeIt: at every Stop the verdict is
// the one the recorder computed from the store as it stood, and it is the
// verdict a replay of the transcript cut at that Stop gives, so nothing after
// it, a result that lands later in the file included, can have changed it.
// Checked on both fixtures' Stops and on a synthetic transcript whose first
// Stop has a call outstanding that a later line answers: at that Stop the
// record has no gate (a declaration with no execution is no gate run) and the
// claim "the tests pass" has nothing to be compared with, as live; had the
// late result been placed before the Stop, the gate would be green and the
// claim supported.
func TestREPLAY13_AStopSeesOnlyTheRecordsBeforeIt(t *testing.T) {
	// The fixtures: the verdict at each Stop equals the captured payloads'.
	for _, version := range pairedFixtures {
		f := loadFixtureSession(t, version)
		steps, ids := f.liveSteps(t)
		a := runLive(t, steps, ids)
		_, res := runRebuilt(t, f.openTranscript(t), f.opt)
		if len(res.Stops) != len(a.verdicts) || len(res.Stops) == 0 {
			t.Fatalf("%s: %d replayed Stops, %d captured", version, len(res.Stops), len(a.verdicts))
		}
		for i, s := range res.Stops {
			if !s.Evaluated || !s.Verified {
				t.Errorf("%s: Stop %d: evaluated %v, verified %v (%s)", version, i, s.Evaluated, s.Verified, s.Downgrade)
				continue
			}
			if got, want := verdictJSON(s.Verdict), verdictJSON(a.verdicts[i]); got != want {
				t.Errorf("%s: Stop %d's verdict differs from the captured payloads':\n replayed %s\n captured %s", version, i, got, want)
			}
		}
	}

	// The synthetic transcript.
	d := lateResultDoc()
	res, rep := replayDoc(t, d.reader(), replay.Options{})
	if len(res.Stops) != 2 || !res.Stops[0].Evaluated || !res.Stops[1].Evaluated {
		t.Fatalf("stops: %+v", res.Stops)
	}
	s1, s2 := res.Stops[0], res.Stops[1]
	if s1.Pos.Line != 4 || s2.Pos.Line != 9 {
		t.Errorf("the Stops are at lines %d and %d, want 4 and 9", s1.Pos.Line, s2.Pos.Line)
	}
	snap1 := s1.Verdict.Snap
	if snap1.Recording != verify.Complete || len(snap1.Gates) != 0 || len(snap1.Claims) != 1 || snap1.Claims[0].Verdict != verify.NoGateKnown {
		t.Errorf("at Stop 1 (t1 declared, its result not yet in the file): recording %s, gates %+v, claims %+v; want a complete record with no gate and the claim unanswerable (no_gate_known): the late result leaked in",
			snap1.Recording, snap1.Gates, snap1.Claims)
	}
	snap2 := s2.Verdict.Snap
	if snap2.Recording != verify.Complete || len(snap2.Gates) != 1 || snap2.Gates[0].State != verify.Red {
		t.Errorf("at Stop 2: recording %s, gates %+v; want one red gate (t2 failed after t1 passed)", snap2.Recording, snap2.Gates)
	}
	// t1's execution is in the store, after Stop 1's point.
	run := runOf(t, rep.world, tSession)
	var landed int64
	for _, e := range run.Executions {
		if e.ToolUseID == "t1" {
			landed = *e.Seq
		}
	}
	if landed == 0 || landed <= snap1.Point.Seq {
		t.Errorf("t1's execution is seq %d, Stop 1's point is seq %d: the late result is not after the Stop", landed, snap1.Point.Seq)
	}

	// verify's own evaluation of the finished store at each Stop's point
	// (INV-11: a point sees the records by that point) agrees with the verdict
	// taken at the Stop, on the gates' families, states and reasons.
	for i, s := range res.Stops {
		post := verify.Evaluate(run, nil, nil, verify.Point{Kind: verify.PointStop, Seq: s.Verdict.Snap.Point.Seq, SessionID: tSession}, s.At)
		type gate struct{ Family, State, Reason string }
		var at, later []gate
		for _, g := range s.Verdict.Snap.Gates {
			at = append(at, gate{g.Family, string(g.State), string(g.Reason)})
		}
		for _, g := range post.Gates {
			later = append(later, gate{g.Family, string(g.State), string(g.Reason)})
		}
		if post.Recording != s.Verdict.Snap.Recording || !reflect.DeepEqual(at, later) {
			t.Errorf("Stop %d: evaluated at the Stop %s %v, and over the finished store at its point %s %v", i, s.Verdict.Snap.Recording, at, post.Recording, later)
		}
	}

	// Each Stop's verdict equals the verdict of the transcript cut at it.
	for i, s := range res.Stops {
		cut, _ := replayDoc(t, d.prefix(s.Pos.Line), replay.Options{})
		last := cut.Stops[len(cut.Stops)-1]
		if len(cut.Stops) != i+1 || !last.Evaluated || last.Pos != s.Pos {
			t.Fatalf("Stop %d: the transcript cut at line %d gives stops %+v", i, s.Pos.Line, cut.Stops)
		}
		if got, want := verdictJSON(last.Verdict), verdictJSON(s.Verdict); got != want {
			t.Errorf("Stop %d's verdict depends on what follows it in the file:\n whole file %s\n cut at it  %s", i, want, got)
		}
	}
	// And the fixtures' Stops, cut at the Stop's line.
	for _, version := range pairedFixtures {
		f := loadFixtureSession(t, version)
		raw, err := os.ReadFile(fixtureTranscript(version))
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
		_, whole := runRebuilt(t, f.openTranscript(t), f.opt)
		for i, s := range whole.Stops {
			cut, _ := replayDoc(t, strings.NewReader(strings.Join(lines[:s.Pos.Line], "\n")+"\n"), f.opt)
			if len(cut.Stops) != 1 || verdictJSON(cut.Stops[0].Verdict) != verdictJSON(s.Verdict) {
				t.Errorf("%s: Stop %d's verdict changes when the transcript is cut at its line", version, i)
			}
		}
	}
}

// TestREPLAY14_ATaintedStopIsNeverVerified: a gap at or before a Stop (an
// event the transcript cannot supply, an unreadable line, a Stop whose
// boundary is unknown) means the replay's evidence for it is incomplete, so
// the replay reports the Stop's verdict as not Verified, with the first gap
// named, and a Stop with a gap only after it stays Verified. A Stop the
// transcript does not establish is not evaluated at all.
func TestREPLAY14_ATaintedStopIsNeverVerified(t *testing.T) {
	sidechain := map[string]any{"isSidechain": true, "agentId": "ag1"}
	type tc struct {
		name  string
		build func(d *tdoc)
		// want, per Stop in file order: "verified", "unverified:<reason>" or
		// "unevaluated:<reason>".
		want []string
	}
	good := func(d *tdoc, p, m string) {
		d.promptLine(p)
		d.uses(m+"u", tBash(m+"t", "python3 -m unittest"))
		d.results("", tOK(m+"t"))
		d.ends(m+"e", "The tests pass.")
	}
	cases := []tc{
		{"no gap", func(d *tdoc) { good(d, "p1", "a"); good(d, "p2", "b") }, []string{"verified", "verified"}},
		{"an unsupported event before the Stop", func(d *tdoc) {
			d.promptLine("p1")
			d.usesWith("", sidechain, "s0", tBash("sub1", "make"))
			d.resultsWith("", sidechain, tOK("sub1"))
			d.uses("m1", tBash("t1", "python3 -m unittest"))
			d.results("", tOK("t1"))
			d.ends("m2", "The tests pass.")
			good(d, "p2", "b")
		}, []string{"unverified:sidechain_agent_type", "unverified:sidechain_agent_type"}},
		{"an unreadable line before the Stop", func(d *tdoc) {
			d.promptLine("p1")
			d.raw("{this line is not json")
			d.uses("m1", tBash("t1", "python3 -m unittest"))
			d.results("", tOK("t1"))
			d.ends("m2", "The tests pass.")
		}, []string{"unverified:unreadable_line"}},
		{"a gap after the Stop only", func(d *tdoc) {
			good(d, "p1", "a")
			d.promptLine("p2")
			d.usesWith("", sidechain, "s0", tBash("sub1", "make"))
			d.resultsWith("", sidechain, tOK("sub1"))
			d.uses("m1", tBash("t1", "python3 -m unittest"))
			d.results("", tOK("t1"))
			d.ends("m2", "The tests pass.")
		}, []string{"verified", "unverified:sidechain_agent_type"}},
		{"the first of two gaps is the one named", func(d *tdoc) {
			d.promptLine("p1")
			d.usesWith("", sidechain, "s0", tBash("sub1", "make"))
			d.resultsWith("", sidechain, tOK("sub1"))
			d.raw("{nor is this")
			d.uses("m1", tBash("t1", "python3 -m unittest"))
			d.results("", tOK("t1"))
			d.ends("m2", "The tests pass.")
		}, []string{"unverified:sidechain_agent_type"}},
		{"a turn the transcript does not show ending", func(d *tdoc) {
			d.promptLine("p1")
			d.uses("m1", tBash("t1", "python3 -m unittest"))
			d.results("", tOK("t1"))
			d.endsAs("m2", "I ran out of room", "max_tokens")
			good(d, "p2", "b")
		}, []string{"unevaluated:stop_unknown_last_message_not_end_turn", "unverified:stop_unknown_last_message_not_end_turn"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := newTdoc()
			c.build(d)
			res, rep := replayDoc(t, d.reader(), replay.Options{})
			if len(res.Stops) != len(c.want) {
				t.Fatalf("%d Stops, want %d: %+v", len(res.Stops), len(c.want), res.Stops)
			}
			for i, s := range res.Stops {
				got := "verified"
				switch {
				case !s.Evaluated:
					got = "unevaluated:" + s.Unevaluated
				case !s.Verified:
					got = "unverified:" + strings.TrimSuffix(strings.TrimPrefix(s.Downgrade, "tainted_by_"), fmt.Sprintf("_at_line_%d", s.Taint.Line))
				}
				if got != c.want[i] {
					t.Errorf("Stop %d (line %d): %s, want %s (taint %+v)", i, s.Pos.Line, got, c.want[i], s.Taint)
				}
				if s.Evaluated && s.Verdict == nil || !s.Evaluated && s.Verdict != nil {
					t.Errorf("Stop %d: evaluated %v and verdict %v", i, s.Evaluated, s.Verdict)
				}
				if s.Verified && !s.Taint.IsZero() {
					t.Errorf("Stop %d is Verified with taint %+v", i, s.Taint)
				}
			}
			_ = rep
		})
	}
}
