package verify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/spec"
)

// cpAt is the time every checkpoint test cuts at.
var cpAt = time.UnixMilli(1_761_600_000_123)

// cpGate is the i-th required learned gate in state s, last run at seq and,
// where s is Green, last green there too.
func cpGate(i int, s State, seq int64) GateState {
	run := &RunRef{Seq: seq}
	g := GateState{Gate: fmt.Sprintf("learned:test:r:go-test@d:pkg%03d", i), Kind: "test",
		Family: fmt.Sprintf("r:go-test@d:pkg%03d", i), Source: sourceLearned, Trust: "builtin", Required: true,
		State: s, LastRun: run, WritersSinceGreen: []AgentCount{}}
	if s == Green {
		g.LastGreen = run
	}
	return g
}

// cpClaim is the i-th claim; its quote HMAC is unique to i.
func cpClaim(i int, strict bool, v Verdict) ClaimVerdict {
	return ClaimVerdict{Source: SourceFinalMessage, Class: ClaimTestsPass, Strict: strict, Verdict: v,
		Gates: []SlotID{"learned:test:r:go-test@d:repo"}, QuoteHMAC: QuoteHMAC(fmt.Sprintf("%s%064x", QuoteHMACPrefix, i))}
}

func cpPractice() *Practice {
	return &Practice{Version: PracticeVersion, Score: ptr(72),
		Components:    PracticeComponents{Gates: 0.5, Claims: 1, Integrity: 1, Hygiene: 0.25},
		NotApplicable: []string{"spec"}}
}

// cpSnap is a complete snapshot at a stop point with every list empty.
func cpSnap() Snapshot {
	return Snapshot{VerifyVersion: Version, RulesVersion: RulesVersion, PracticeVersion: PracticeVersion,
		Point: Point{Kind: PointStop, Seq: 5000, SessionID: "s1"}, Recording: Complete, ProjectRoot: "/repo",
		Gates: []GateState{}, Claims: []ClaimVerdict{}, SpecVerdicts: []RuleVerdict{},
		Integrity: Integrity{Unresolved: []IntegrityEntry{}}}
}

// cpGates200 is VER51b's snapshot and BenchmarkCut's: 200 green gates last
// run in an order unrelated to their position, 20 loose claims and a
// practice score.
func cpGates200() Snapshot {
	snap := cpSnap()
	for i, s := range rand.New(rand.NewSource(1)).Perm(200) {
		snap.Gates = append(snap.Gates, cpGate(i, Green, int64(1000+s)))
	}
	for i := range 20 {
		snap.Claims = append(snap.Claims, cpClaim(i, false, Verdict(Green)))
	}
	snap.Practice = cpPractice()
	return snap
}

// written is the record's length as the store appends it: json.Marshal,
// with the widest seq in place of Cut's null.
func written(t testing.TB, cp Checkpoint) int {
	t.Helper()
	cp.Seq = ptr(int64(math.MaxInt64))
	b, err := json.Marshal(cp)
	if err != nil {
		t.Fatal(err)
	}
	return len(b)
}

func cpJSON(t testing.TB, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// cpCut is Cut at cpAt, held to the rule every record keeps: Truncated says
// exactly that a list was cut or that the record as written, unflagged, is
// over CheckpointMaxBytes (the flag is decided before it is set, since
// "true" is a byte shorter than "false"); and a record over the budget as
// written has nothing left to cut.
func cpCut(t testing.TB, snap Snapshot) Checkpoint {
	t.Helper()
	cp := Cut(snap, cpAt)
	omitted := cp.GatesOmitted + cp.ClaimsOmitted + cp.SpecVerdictsOmitted + cp.IntegrityOmitted + cp.BackgroundOmitted +
		cp.PracticeOmitted
	unflagged := cp
	unflagged.Truncated = false
	if n := written(t, unflagged); cp.Truncated != (omitted > 0 || n > CheckpointMaxBytes) {
		t.Fatalf("truncated %v with %d omitted and %d bytes written unflagged", cp.Truncated, omitted, n)
	}
	if n := written(t, cp); n > CheckpointMaxBytes && (cp.Practice != nil || len(cp.Gates)+len(cp.Claims)+
		len(cp.SpecVerdicts)+len(cp.Integrity.Unresolved)+len(cp.BackgroundInFlight) > 0) {
		t.Fatalf("%d bytes written with entries left to cut", n)
	}
	return cp
}

// cpOver is snap with its ProjectRoot lengthened so that, with nothing cut,
// it is written over bytes past CheckpointMaxBytes (0: exactly at it); snap
// must fit as it is.
func cpOver(t testing.TB, snap Snapshot, over int) Snapshot {
	t.Helper()
	cp := cpCut(t, snap)
	if cp.Truncated {
		t.Fatal("the fixture is over the budget before it is padded")
	}
	snap.ProjectRoot += strings.Repeat("a", CheckpointMaxBytes-written(t, cp)+over)
	return snap
}

// cpRecordOf is g as the checkpoint keeps it, written out apart from Cut.
func cpRecordOf(g GateState) CheckpointGate {
	cg := CheckpointGate{Gate: g.Gate, Kind: g.Kind, Source: g.Source, Required: g.Required, State: g.State,
		Reason: g.Reason, WritesSinceGreen: g.WritesSinceGreen, UnknownTarget: g.UnknownTarget}
	if g.LastRun != nil {
		cg.LastRunSeq = ptr(g.LastRun.Seq)
	}
	if g.LastGreen != nil {
		cg.LastGreenSeq = ptr(g.LastGreen.Seq)
	}
	if g.LastWrite != nil {
		cg.LastWriteSeq = ptr(g.LastWrite.Seq)
	}
	return cg
}

// cpEmpty fails unless every list of cp is empty and not nil and nothing is
// omitted.
func cpEmpty(t *testing.T, cp Checkpoint) {
	t.Helper()
	if cp.Gates == nil || cp.Claims == nil || cp.SpecVerdicts == nil || cp.Integrity.Unresolved == nil ||
		cp.BackgroundInFlight == nil || len(cp.Gates)+len(cp.Claims)+len(cp.SpecVerdicts)+len(cp.Integrity.Unresolved)+
		len(cp.BackgroundInFlight) != 0 {
		t.Errorf("lists %s, want every one empty and not nil", cpJSON(t, cp))
	}
	if cp.GatesOmitted+cp.ClaimsOmitted+cp.SpecVerdictsOmitted+cp.IntegrityOmitted+cp.BackgroundOmitted+
		cp.PracticeOmitted != 0 {
		t.Errorf("omitted counts set on a record with nothing to cut: %s", cpJSON(t, cp))
	}
}

// TestVER51b_CheckpointUnderEightKiBWithOmittedCounts: 200 green gates, 20
// loose claims and a practice score are over the budget. The practice goes
// first, then the green gates with the oldest runs, and no more than that:
// the record fits, the newest gates stand in their places, and the last gate
// cut would not fit back. A header alone fits with nothing cut, and so does
// a full record at the budget. A header at the budget fits; one a byte over
// it unflagged, or over it by a ProjectRoot of 4096 escaped bytes (card 1.10
// R4), comes back whole with Truncated set. A record json cannot encode is
// over every budget: its practice goes.
func TestVER51b_CheckpointUnderEightKiBWithOmittedCounts(t *testing.T) {
	snap := cpGates200()
	cp := cpCut(t, snap)
	if n := written(t, cp); n > CheckpointMaxBytes {
		t.Fatalf("%d bytes written, want at most %d", n, CheckpointMaxBytes)
	}
	if cp.Practice != nil || cp.PracticeOmitted != 5 {
		t.Errorf("practice %v, %d omitted; want null and 5: it goes before any gate", cp.Practice, cp.PracticeOmitted)
	}
	k := cp.GatesOmitted
	if k < 1 || len(cp.Gates) < 1 || len(cp.Gates)+k != 200 || !cp.Truncated {
		t.Fatalf("%d gates kept, %d omitted, truncated %v", len(cp.Gates), k, cp.Truncated)
	}
	if len(cp.Claims) != 20 || cp.ClaimsOmitted != 0 {
		t.Errorf("%d claims kept, %d omitted: claims go after every green gate", len(cp.Claims), cp.ClaimsOmitted)
	}
	newest := func(k int) []CheckpointGate {
		out := []CheckpointGate{}
		for _, g := range snap.Gates {
			if g.LastRun.Seq >= int64(1000+k) {
				out = append(out, cpRecordOf(g))
			}
		}
		return out
	}
	if got, want := cpJSON(t, cp.Gates), cpJSON(t, newest(k)); !bytes.Equal(got, want) {
		t.Errorf("kept gates\n got %s\nwant the %d newest in place %s", got, 200-k, want)
	}
	more := cp
	more.Gates, more.GatesOmitted = newest(k-1), k-1
	if n := written(t, more); n <= CheckpointMaxBytes {
		t.Errorf("the last gate cut fits back (%d bytes): the cut went past the budget", n)
	}

	cp = cpCut(t, cpSnap())
	if cp.ProjectRoot != "/repo" || cp.Truncated {
		t.Errorf("a header alone: root %q truncated %v", cp.ProjectRoot, cp.Truncated)
	}
	cpEmpty(t, cp)
	if cp = cpCut(t, cpOver(t, cpFull(), 0)); cp.Truncated || cp.Practice == nil || len(cp.Gates) != 3 {
		t.Errorf("a full record at the budget lost something: %s", cpJSON(t, cp))
	}
	if cp = cpCut(t, cpOver(t, cpFull(), 1)); !cp.Truncated || cp.PracticeOmitted != 0 || written(t, cp) != CheckpointMaxBytes {
		t.Errorf("a full record a byte over unflagged: truncated %v, practice %d omitted, %d bytes; want it whole, "+
			"flagged, at the budget", cp.Truncated, cp.PracticeOmitted, written(t, cp))
	}
	for _, c := range []struct {
		name      string
		snap      Snapshot
		truncated bool
	}{
		{"at the budget", cpOver(t, cpSnap(), 0), false},
		{"a byte over", cpOver(t, cpSnap(), 1), true},
		{"escaped", func() Snapshot { s := cpSnap(); s.ProjectRoot = strings.Repeat("&", 4096); return s }(), true},
	} {
		cp = cpCut(t, c.snap)
		if cp.Truncated != c.truncated || cp.ProjectRoot != c.snap.ProjectRoot {
			t.Errorf("%s: truncated %v, want %v; root kept whole %v", c.name, cp.Truncated, c.truncated,
				cp.ProjectRoot == c.snap.ProjectRoot)
		}
		cpEmpty(t, cp)
	}

	nan := cpSnap()
	nan.Practice = cpPractice()
	nan.Practice.Components.Hygiene = math.NaN()
	if cp = cpCut(t, nan); cp.Practice != nil || cp.PracticeOmitted != 5 {
		t.Errorf("a NaN component: practice %v, %d omitted; want it cut", cp.Practice, cp.PracticeOmitted)
	}
}

// TestVER51c_CutOrderNeverDropsARedBeforeAGreen: every green gate goes
// before a non-green one, and every loose, supported or no_gate_known claim
// before a strict non-green one, whatever their places. Among the non-green
// gates the least severe go first, so the Red is the last gate standing, and
// a state outside the vocabulary, a defect, outlasts even the Red; within a
// state the older run goes first, a gate never run first of all (Q3). The
// non-green gates go before the strict non-green claims (D45); among those
// claims the red is the last.
func TestVER51c_CutOrderNeverDropsARedBeforeAGreen(t *testing.T) {
	snap := cpSnap()
	for i := range 200 {
		s := Green
		switch i {
		case 20:
			s = Stale
		case 150:
			s = Red
		}
		snap.Gates = append(snap.Gates, cpGate(i, s, int64(1000+i)))
	}
	cp := cpCut(t, snap)
	var bad []string
	for _, g := range cp.Gates {
		if g.State != Green {
			bad = append(bad, g.Gate)
		}
	}
	if want := []string{snap.Gates[20].Gate, snap.Gates[150].Gate}; !slices.Equal(bad, want) ||
		cp.GatesOmitted == 0 || cp.GatesOmitted != 200-len(cp.Gates) {
		t.Errorf("non-green gates kept %q, want %q; %d kept, %d omitted", bad, want, len(cp.Gates), cp.GatesOmitted)
	}

	snap = cpSnap()
	for i := range 100 {
		c := cpClaim(i, false, Verdict(Green))
		switch {
		case i == 10:
			c.Strict, c.Verdict = true, Verdict(Stale)
		case i == 90:
			c.Strict, c.Verdict = true, Verdict(Red)
		case i == 95:
			c.Strict = true // supported
		case i == 96:
			c.Strict, c.Verdict = true, NoGateKnown
		case i%3 == 0:
			c.Verdict = Verdict(Red) // loose whatever its verdict
		}
		snap.Claims = append(snap.Claims, c)
	}
	cp = cpCut(t, snap)
	want, keep := []ClaimVerdict{}, len(cp.Claims)-2
	for i, c := range snap.Claims {
		switch {
		case i == 10 || i == 90:
			want = append(want, c)
		case keep > 0:
			want, keep = append(want, c), keep-1
		}
	}
	if got := cpJSON(t, cp.Claims); !bytes.Equal(got, cpJSON(t, want)) || cp.ClaimsOmitted < 5 ||
		cp.ClaimsOmitted != 100-len(cp.Claims) {
		t.Errorf("%d claims kept, %d omitted\n got %s\nwant the strict stale and red and the first loose ones %s",
			len(cp.Claims), cp.ClaimsOmitted, got, cpJSON(t, want))
	}

	six := cpSnap()
	for i, s := range []State{Red, Stale, Unknown, MaskedInconclusive, Absent, Backgrounded} {
		six.Gates = append(six.Gates, cpGate(i, s, int64(1000+i)))
	}
	for over := 50; over <= 1500; over += 50 {
		cp = cpCut(t, cpOver(t, six, over))
		kept := map[string]bool{}
		worst := -1
		for _, g := range cp.Gates {
			kept[g.Gate], worst = true, max(worst, Rank(g.State))
		}
		for _, g := range six.Gates {
			if !kept[g.Gate] && Rank(g.State) < worst {
				t.Errorf("over by %d: a %s gate cut while one of rank %d stands", over, g.State, worst)
			}
		}
	}
	defect := cpGate(9, "bogus", 999)
	for _, c := range []struct {
		name  string
		last  GateState
		gates []GateState
	}{
		{"the Red", six.Gates[0], six.Gates},
		{"a defect", defect, append(slices.Clone(six.Gates), defect)},
	} {
		only := cpSnap()
		only.Gates = []GateState{c.last}
		all := cpOver(t, only, 0)
		all.Gates = c.gates
		cp = cpCut(t, all)
		if len(cp.Gates) != 1 || cp.Gates[0].Gate != c.last.Gate || cp.GatesOmitted != len(c.gates)-1 {
			t.Errorf("%s is not the last gate standing: %s, %d omitted", c.name, cpJSON(t, cp.Gates), cp.GatesOmitted)
		}
	}
	absent := []GateState{cpGate(0, Absent, 1000), cpGate(1, Absent, 0), cpGate(2, Absent, 900)}
	absent[1].LastRun = nil
	one := cpSnap()
	one.Gates = absent[:1]
	ties := cpOver(t, one, 0)
	ties.Gates = absent
	if cp = cpCut(t, ties); len(cp.Gates) != 1 || cp.Gates[0].Gate != absent[0].Gate {
		t.Errorf("within a state the newest run is kept, a gate never run cut first: kept %s", cpJSON(t, cp.Gates))
	}

	red := cpClaim(1, true, Verdict(Red))
	only := cpSnap()
	only.Claims = []ClaimVerdict{red}
	both := cpOver(t, only, 0)
	both.Gates = []GateState{cpGate(0, Red, 1000)}
	if cp = cpCut(t, both); len(cp.Gates) != 0 || cp.GatesOmitted != 1 || len(cp.Claims) != 1 || cp.ClaimsOmitted != 0 {
		t.Errorf("D45: %d gates kept, %d claims kept; want the red gate cut and the strict red claim kept",
			len(cp.Gates), len(cp.Claims))
	}
	three := cpOver(t, only, 0)
	three.Claims = []ClaimVerdict{cpClaim(2, true, Verdict(Stale)), red, cpClaim(3, true, Verdict(MaskedInconclusive))}
	if cp = cpCut(t, three); len(cp.Claims) != 1 || cp.Claims[0].QuoteHMAC != red.QuoteHMAC || cp.ClaimsOmitted != 2 {
		t.Errorf("strict non-green claims kept %s, want the red alone", cpJSON(t, cp.Claims))
	}
}

// cpFull is a snapshot with every part of the record set and under the
// budget: header, spec, three gates (one green, one background, one
// advisory background never run), a claim, a rule verdict with its
// hard_control text, an integrity entry and a practice score.
func cpFull() Snapshot {
	snap := cpSnap()
	snap.Point = Point{Kind: PointStop, Seq: 77, SessionID: "s1", PromptID: ptr("p1"), AgentID: ptr("a1"), TaskRef: ptr("t1")}
	snap.Spec = &SpecInfo{Path: ".rashomon/spec.yaml", Digest: "sha256:" + strings.Repeat("ab", 32), Trust: spec.Trusted,
		Errors: []string{}}
	snap.RecordGreen, snap.RecordGreenAtSeq, snap.Notes = true, ptr(int64(76)), []string{"a note"}
	green := cpGate(0, Green, 60)
	green.LastWrite, green.UnknownTarget, green.WritesSinceGreen = &WriteRef{Seq: 58, Path: ptr("a.go"), Tool: "Edit"}, 1, 2
	bg := cpGate(1, Backgrounded, 70)
	bg.Reason = BackgroundLaunch
	quiet := cpGate(2, Backgrounded, 0)
	quiet.LastRun, quiet.Required = nil, false
	snap.Gates = []GateState{green, bg, quiet}
	snap.Claims = []ClaimVerdict{cpClaim(0, true, Verdict(Green))}
	snap.SpecVerdicts = []RuleVerdict{{Rule: "no-shell-true", Kind: spec.ForbidCode, Verdict: Violation, Seqs: []int64{44},
		HardControl: "CI scanner job"}}
	snap.Integrity = Integrity{Unresolved: []IntegrityEntry{{Check: SkipAdded, Marker: "go.t_skip", Seq: 50}},
		Counts: IntegrityCounts{TestsDelta: -1, SkipsAdded: 1}, UnknownMarks: 2, UnreadBases: 3}
	snap.Practice = cpPractice()
	return snap
}

// TestVER51d_HeaderAndProjectionFromTheSnapshot: the header is R1's, from
// the snapshot's point and spec, with a null seq for the store to assign and
// no tool-use id; gates keep their refs' seqs; the rule verdict loses its
// hard_control text; background is every Backgrounded gate, required or
// not, at its launch (0 for one never run), oldest first. A nil spec gives
// a null digest and trust.
func TestVER51d_HeaderAndProjectionFromTheSnapshot(t *testing.T) {
	snap := cpFull()
	cp := cpCut(t, snap)
	g := snap.Gates
	want := Checkpoint{Type: "verify", SchemaVersion: 5, RecordedAtUnixMS: cpAt.UnixMilli(), SessionID: "s1",
		PromptID: ptr("p1"), AgentID: ptr("a1"), Point: PointStop, PointSeq: 77, TaskRef: ptr("t1"),
		VerifyVersion: Version, RulesVersion: RulesVersion, SpecDigest: ptr(snap.Spec.Digest), SpecTrust: ptr(spec.Trusted),
		Recording: Complete, ProjectRoot: "/repo",
		Gates: []CheckpointGate{
			{Gate: g[0].Gate, Kind: "test", Source: sourceLearned, Required: true, State: Green, LastRunSeq: ptr(int64(60)),
				LastGreenSeq: ptr(int64(60)), LastWriteSeq: ptr(int64(58)), WritesSinceGreen: 2, UnknownTarget: 1},
			{Gate: g[1].Gate, Kind: "test", Source: sourceLearned, Required: true, State: Backgrounded,
				Reason: BackgroundLaunch, LastRunSeq: ptr(int64(70))},
			{Gate: g[2].Gate, Kind: "test", Source: sourceLearned, State: Backgrounded},
		},
		Claims:             []ClaimVerdict{cpClaim(0, true, Verdict(Green))},
		SpecVerdicts:       []CheckpointRuleVerdict{{Rule: "no-shell-true", Kind: spec.ForbidCode, Verdict: Violation, Seqs: []int64{44}}},
		Integrity:          snap.Integrity,
		BackgroundInFlight: []BackgroundRef{{Family: g[2].Family, Seq: 0}, {Family: g[1].Family, Seq: 70}},
		Practice:           cpPractice(),
		RecordGreen:        true,
	}
	b := cpJSON(t, cp)
	if w := cpJSON(t, want); !bytes.Equal(b, w) {
		t.Errorf("record\n got %s\nwant %s", b, w)
	}
	if bytes.Contains(b, []byte("CI scanner job")) || cp.Seq != nil || cp.PointToolUseID != nil || cp.Critical {
		t.Errorf("hard_control, a seq, a tool-use id or critical in the record: %s", b)
	}
	snap.Spec = nil
	if b = cpJSON(t, cpCut(t, snap)); !bytes.Contains(b, []byte(`"spec_digest":null,"spec_trust":null,`)) {
		t.Errorf("a nil spec: %s", b)
	}
}

// cpListKeys are the keys whose value is a list anywhere in the record.
var cpListKeys = map[string]bool{"gates": true, "claims": true, "spec_verdicts": true, "unresolved": true,
	"background_in_flight": true, "seqs": true, "not_applicable": true}

// cpNullLists is the paths in b at which a list is null.
func cpNullLists(t *testing.T, b []byte) []string {
	t.Helper()
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	var out []string
	var walk func(v any, path string)
	walk = func(v any, path string) {
		switch v := v.(type) {
		case map[string]any:
			for k, e := range v {
				if e == nil && cpListKeys[k] {
					out = append(out, path+"."+k)
				}
				walk(e, path+"."+k)
			}
		case []any:
			for i, e := range v {
				walk(e, fmt.Sprintf("%s[%d]", path, i))
			}
		}
	}
	walk(v, "")
	slices.Sort(out)
	return out
}

// TestVER51e_UnderBudgetNothingOmitted: a record under the budget keeps
// everything, omits nothing and is not truncated; every list in it, nested
// ones too, is [] when empty, from a snapshot with nil lists as much as from
// one with empty ones.
func TestVER51e_UnderBudgetNothingOmitted(t *testing.T) {
	snap := cpFull()
	cp := cpCut(t, snap)
	if cp.Truncated || len(cp.Gates) != 3 || len(cp.Claims) != 1 || len(cp.SpecVerdicts) != 1 ||
		len(cp.Integrity.Unresolved) != 1 || len(cp.BackgroundInFlight) != 2 || cp.Practice == nil {
		t.Errorf("a small record lost something: %s", cpJSON(t, cp))
	}
	nils := Snapshot{Point: Point{Kind: PointStop, Seq: 9, SessionID: "s1"}, Recording: Complete, Practice: &Practice{},
		Claims: []ClaimVerdict{{Source: SourceFinalMessage, Class: ClaimDone}}, SpecVerdicts: []RuleVerdict{{Rule: "r"}}}
	for _, s := range []Snapshot{snap, nils, {}} {
		cp = cpCut(t, s)
		if cp.Truncated || cp.GatesOmitted+cp.ClaimsOmitted+cp.SpecVerdictsOmitted+cp.IntegrityOmitted+
			cp.BackgroundOmitted+cp.PracticeOmitted != 0 {
			t.Errorf("under the budget, something omitted: %s", cpJSON(t, cp))
		}
		if got := cpNullLists(t, cpJSON(t, cp)); len(got) != 0 {
			t.Errorf("null lists at %q", got)
		}
	}
	if b := cpJSON(t, cpCut(t, Snapshot{})); !bytes.Contains(b, []byte(`"gates":[],"gates_omitted":0,"claims":[],`)) {
		t.Errorf("an empty snapshot: %s", b)
	}
}

// TestVER51f_PureAndDeterministic: Cut leaves its input as it found it, a
// snapshot over the budget with every list out of order included; two cuts
// give the same bytes; and the record shares no memory with the snapshot,
// so writing through every slice and pointer of it changes nothing there.
func TestVER51f_PureAndDeterministic(t *testing.T) {
	big := cpGates200()
	rng := rand.New(rand.NewSource(3))
	for _, s := range rng.Perm(60) {
		big.Integrity.Unresolved = append(big.Integrity.Unresolved, IntegrityEntry{Check: SkipAdded, Marker: "go.t_skip",
			Seq: int64(s)})
		big.SpecVerdicts = append(big.SpecVerdicts, RuleVerdict{Rule: SlotID(fmt.Sprint("r", s)), Kind: spec.ForbidCode,
			Verdict: Violation, Seqs: []int64{int64(s + 7), int64(s)}})
	}
	for i, s := range rng.Perm(10) {
		big.Gates = append(big.Gates, cpGate(200+i, Backgrounded, int64(s)))
	}
	before := cpJSON(t, big)
	a, b := cpJSON(t, cpCut(t, big)), cpJSON(t, cpCut(t, big))
	if !bytes.Equal(before, cpJSON(t, big)) {
		t.Error("Cut changed the snapshot it was given")
	}
	if !bytes.Equal(a, b) {
		t.Errorf("two cuts differ\n%s\n%s", a, b)
	}

	snap := cpFull()
	before = cpJSON(t, snap)
	cp := cpCut(t, snap)
	*cp.PromptID, *cp.AgentID, *cp.TaskRef, *cp.SpecDigest, *cp.SpecTrust = "x", "x", "x", "x", "x"
	for i := range cp.Gates {
		for _, p := range []*int64{cp.Gates[i].LastRunSeq, cp.Gates[i].LastGreenSeq, cp.Gates[i].LastWriteSeq} {
			if p != nil {
				*p = -1
			}
		}
	}
	cp.Claims[0].Gates[0], cp.SpecVerdicts[0].Seqs[0], cp.Integrity.Unresolved[0].Seq = "x", -1, -1
	*cp.Practice.Score, cp.Practice.NotApplicable[0] = -1, "x"
	if after := cpJSON(t, snap); !bytes.Equal(before, after) {
		t.Errorf("writing through the record changed the snapshot\nbefore %s\n after %s", before, after)
	}
}

// TestVER51g_OldestFirstForBackgroundIntegrityAndSpec: background,
// integrity and spec entries built out of order are kept in Seq order and
// cut oldest first, a spec entry by its smallest seq and one with none
// before any; the three lists go in that order, each whole before the next
// is touched; an integrity cut keeps the counts whole.
func TestVER51g_OldestFirstForBackgroundIntegrityAndSpec(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	bg := cpSnap()
	for i, s := range rng.Perm(10) {
		bg.Gates = append(bg.Gates, cpGate(i, Backgrounded, int64(100+s)))
	}
	cp := cpCut(t, cpOver(t, bg, 200))
	k := cp.BackgroundOmitted
	if k == 0 || k >= 10 || len(cp.BackgroundInFlight)+k != 10 || cp.GatesOmitted != 0 {
		t.Fatalf("background: %d kept, %d omitted, %d gates omitted", len(cp.BackgroundInFlight), k, cp.GatesOmitted)
	}
	for j, r := range cp.BackgroundInFlight {
		if i := slices.IndexFunc(bg.Gates, func(g GateState) bool { return g.LastRun.Seq == r.Seq }); r.Seq != int64(100+k+j) ||
			r.Family != bg.Gates[i].Family || r.TaskRef != "" {
			t.Errorf("background %d is %+v, want the launch at %d", j, r, 100+k+j)
		}
	}

	in := cpSnap()
	in.Integrity.Counts, in.Integrity.UnknownMarks, in.Integrity.UnreadBases = IntegrityCounts{TestsDelta: 1, SkipsAdded: 2, SuppressionsAdded: 3}, 4, 5
	for _, s := range rng.Perm(40) {
		in.Integrity.Unresolved = append(in.Integrity.Unresolved, IntegrityEntry{Check: SkipAdded, Marker: "go.t_skip",
			Seq: int64(200 + s)})
	}
	cp = cpCut(t, cpOver(t, in, 300))
	k = cp.IntegrityOmitted
	if k == 0 || k >= 40 || len(cp.Integrity.Unresolved)+k != 40 {
		t.Fatalf("integrity: %d kept, %d omitted", len(cp.Integrity.Unresolved), k)
	}
	for j, e := range cp.Integrity.Unresolved {
		if e.Seq != int64(200+k+j) {
			t.Errorf("integrity %d at %d, want %d", j, e.Seq, 200+k+j)
		}
	}
	if c := cp.Integrity; c.Counts != in.Integrity.Counts || c.UnknownMarks != 4 || c.UnreadBases != 5 {
		t.Errorf("integrity counts %+v, want them whole", c)
	}

	sp := cpSnap()
	sp.SpecVerdicts = append(sp.SpecVerdicts, RuleVerdict{Rule: "none", Kind: spec.Order, Verdict: RuleUnknown})
	for _, s := range rng.Perm(40) {
		sp.SpecVerdicts = append(sp.SpecVerdicts, RuleVerdict{Rule: SlotID(fmt.Sprint("r", s)), Kind: spec.ForbidCode,
			Verdict: Violation, Seqs: []int64{int64(1000 - s), int64(300 + s), int64(400 + s)}})
	}
	cp = cpCut(t, cpOver(t, sp, 300))
	k = cp.SpecVerdictsOmitted
	if k < 2 || k >= 41 || len(cp.SpecVerdicts)+k != 41 {
		t.Fatalf("spec: %d kept, %d omitted", len(cp.SpecVerdicts), k)
	}
	for j, v := range cp.SpecVerdicts {
		s := k - 1 + j
		if want := []int64{int64(1000 - s), int64(300 + s), int64(400 + s)}; v.Rule != SlotID(fmt.Sprint("r", s)) ||
			!slices.Equal(v.Seqs, want) {
			t.Errorf("spec %d is %s %v, want r%d %v", j, v.Rule, v.Seqs, s, want)
		}
	}

	two := cpSnap()
	for i, s := range rng.Perm(5) {
		two.Gates = append(two.Gates, cpGate(i, Backgrounded, int64(100+s)))
	}
	two.Integrity.Unresolved = in.Integrity.Unresolved
	if cp = cpCut(t, cpOver(t, two, 400)); cp.BackgroundOmitted != 5 || cp.IntegrityOmitted == 0 {
		t.Errorf("background %d omitted, integrity %d: the background goes whole first", cp.BackgroundOmitted,
			cp.IntegrityOmitted)
	}
	two = cpSnap()
	two.Integrity.Unresolved = in.Integrity.Unresolved[:3]
	two.SpecVerdicts = sp.SpecVerdicts
	if cp = cpCut(t, cpOver(t, two, 400)); cp.IntegrityOmitted != 3 || cp.SpecVerdictsOmitted == 0 {
		t.Errorf("integrity %d omitted, spec %d: the integrity goes whole first", cp.IntegrityOmitted,
			cp.SpecVerdictsOmitted)
	}
}
