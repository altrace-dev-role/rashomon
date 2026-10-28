package verify

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// gate is a required gate of kind in state s, named as gateStates names one.
func gate(kind string, s State, runner string) GateState {
	return GateState{Gate: "learned:" + kind + ":r:" + runner + "@d:repo", Kind: kind, Family: "r:" + runner + "@d:repo",
		Required: true, State: s}
}

// advisory is g with Required false (D38).
func advisory(g GateState) GateState {
	g.Required = false
	return g
}

// scanOf is Claims over one window with a relevant write and nothing delivered.
func scanOf(msg string, gates ...GateState) ClaimScan {
	return Claims([]byte(msg), nil, claimKey, gates, true, SourceFinalMessage, nil)
}

// quoteOf is the QuoteHMAC the test computes for sentence under key.
func quoteOf(key []byte, sentence string) QuoteHMAC {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(sentence))
	return QuoteHMAC(QuoteHMACPrefix + hex.EncodeToString(m.Sum(nil)))
}

// firstClaim is s's first verdict, failing the test when it has none.
func firstClaim(t *testing.T, s ClaimScan) ClaimVerdict {
	t.Helper()
	if len(s.Claims) == 0 {
		t.Fatal("no claim, want one")
	}
	return s.Claims[0]
}

func wantClasses(t *testing.T, msg string, s ClaimScan, want ...string) {
	t.Helper()
	if got := classes(s); !slices.Equal(got, append([]string{}, want...)) {
		t.Errorf("%q: claims %v, want %v", msg, got, want)
	}
}

// TestVER56_QuotedRashomonSentenceIsNotAClaim: a line that is a delivered
// steer line behind "※" is no claim; the words without the mark, a bare mark,
// a prefix of the line, or no delivered lines at all leave the claim (20.2.5
// step 2).
func TestVER56_QuotedRashomonSentenceIsNotAClaim(t *testing.T) {
	delivered := [][]byte{[]byte("tests pass after the last edit")}
	for _, c := range []struct {
		msg       string
		delivered [][]byte
		want      int
		why       string
	}{
		{"※ tests pass after the last edit", delivered, 0, "the delivered line"},
		{"  ※   tests pass after the last edit \r", delivered, 0, "spaces around the line"},
		{"tests pass after the last edit", delivered, 1, "the same words without the mark"},
		{"※\nAll tests pass.", delivered, 1, "a bare mark on its own line"},
		{"All tests pass. ※", delivered, 1, "a bare mark after the claim"},
		{"※ tests pass after the last edit, twice", delivered, 1, "the delivered line a prefix of the message's"},
		{"※ tests pass after the last", delivered, 1, "the message's line a prefix of the delivered one"},
		{"※ tests pass after the last edit", nil, 1, "nothing delivered (v0 callers)"},
	} {
		s := Claims([]byte(c.msg), nil, claimKey, allKinds(), true, SourceFinalMessage, c.delivered)
		if len(s.Claims) != c.want {
			t.Errorf("%q: %d claims, want %d (%s)", c.msg, len(s.Claims), c.want, c.why)
		}
	}
}

// TestVER57_ClaimKeepsOnlyTheHMAC: the marshalled scan holds no 9-byte piece
// of the message; the quote is HMAC-SHA256 of the sentence under the key, and
// another key gives another digest (20.2.5, what the checkpoint keeps).
func TestVER57_ClaimKeepsOnlyTheHMAC(t *testing.T) {
	const sentence = "All 42 tests pass on the release branch."
	msg := sentence + " The parser work is complete."
	s := scanOf(msg, allKinds()...)
	out, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i+9 <= len(msg); i++ {
		if bytes.Contains(out, []byte(msg[i:i+9])) {
			t.Fatalf("the scan carries %q of the message: %s", msg[i:i+9], out)
		}
	}
	if len(s.Claims) != 2 {
		t.Fatalf("claims %v, want tests_pass and done", classes(s))
	}
	q := s.Claims[0].QuoteHMAC
	if !q.Valid() || q != quoteOf(claimKey, sentence) {
		t.Errorf("quote %q, want %q", q, quoteOf(claimKey, sentence))
	}
	other := Claims([]byte(msg), nil, []byte("another install key"), allKinds(), true, SourceFinalMessage, nil)
	if firstClaim(t, other).QuoteHMAC == q {
		t.Errorf("two keys gave one digest %q", q)
	}
	if s.Claims[0].Source != SourceFinalMessage {
		t.Errorf("source %q, want %q", s.Claims[0].Source, SourceFinalMessage)
	}
}

// TestVER58_VerdictFollowsOrder: the verdict is VerdictOf over the required
// gates of the claim's kind (every required gate for done); Gates is their
// ids within the slot charset, sorted, at most MaxPaths, never nil (20.2.5
// steps 6 and 7; D36, D38, D43).
func TestVER58_VerdictFollowsOrder(t *testing.T) {
	tr, tg, tu := gate("test", Red, "a"), gate("test", Green, "b"), gate("test", Unknown, "c")
	bs, lr := gate("build", Stale, "d"), gate("lint", Red, "e")
	id := func(gs ...GateState) []SlotID {
		ids := []SlotID{}
		for _, g := range gs {
			ids = append(ids, SlotID(g.Gate))
		}
		return ids
	}
	bad := GateState{Gate: "Learned:Test", Kind: "test", Required: true, State: Red}
	for _, c := range []struct {
		name    string
		msg     string
		gates   []GateState
		verdict Verdict
		ids     []SlotID
	}{
		{"red", "All tests pass.", []GateState{tr}, Verdict(Red), id(tr)},
		{"green beside unknown reads unknown", "All tests pass.", []GateState{tg, tu}, Verdict(Unknown), id(tg, tu)},
		{"green", "All tests pass.", []GateState{tg}, Verdict(Green), id(tg)},
		{"no gate", "All tests pass.", nil, NoGateKnown, id()},
		{"no gate of the class's kind", "Lint is clean.", []GateState{tr, tg}, NoGateKnown, id()},
		{"done is the worst over every required gate", "Done.", []GateState{tg, bs, advisory(lr)}, Verdict(Stale), id(bs, tg)},
		{"done reads a required red of any kind", "Done.", []GateState{tg, lr}, Verdict(Red), id(lr, tg)},
		{"an advisory gate never enters", "All tests pass.", []GateState{tg, advisory(tr)}, Verdict(Green), id(tg)},
		{"an id outside the charset is dropped and the verdict stands", "All tests pass.", []GateState{bad}, Verdict(Red), id()},
	} {
		s := scanOf(c.msg, c.gates...)
		if len(s.Claims) != 1 {
			t.Errorf("%s: claims %v, want one", c.name, classes(s))
			continue
		}
		got := s.Claims[0]
		if got.Verdict != c.verdict || got.Gates == nil || !slices.Equal(got.Gates, c.ids) {
			t.Errorf("%s: verdict %s gates %#v, want %s %#v", c.name, got.Verdict, got.Gates, c.verdict, c.ids)
		}
	}

	var many []GateState
	for _, r := range []string{"g6", "g5", "g4", "g3", "g2", "g1", "g0"} {
		many = append(many, gate("test", Green, r))
	}
	if got := firstClaim(t, scanOf("All tests pass.", many...)).Gates; !slices.Equal(got, id(many[6], many[5], many[4], many[3], many[2])) {
		t.Errorf("seven gates kept as %q, want the first %d sorted", got, MaxPaths)
	}

	// D43: a name gateStates builds is a slot id and stays.
	real := statesAt(t, 4, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK))
	if !strings.Contains(real[0].Gate, "@") {
		t.Fatalf("gate %q: a learned name carries '@' (gates.go)", real[0].Gate)
	}
	if got := firstClaim(t, scanOf("All tests pass.", real...)).Gates; !slices.Equal(got, id(real...)) {
		t.Errorf("gateStates' own name dropped: gates %q", got)
	}
}

// TestVER59_DoneNeedsARelevantWrite: done fires only when the window holds a
// relevant write, and then the record's red reads red, the core contradiction
// (20.2.5 lexicon; Gate A Q1). WroteRelevant is gateStates' predicate.
func TestVER59_DoneNeedsARelevantWrite(t *testing.T) {
	for _, c := range []struct {
		name  string
		calls []call
		at    int64
	}{
		{"edit, red run", []call{edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecFailed)}, 4},
		{"edit, green, red re-run", []call{edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK), gateCall(3, "go-test", store.ExecFailed)}, 6},
	} {
		r := runOf(t, c.calls...)
		s := Claims([]byte("Done."), nil, claimKey, statesOf(t, r, c.at), WroteRelevant(r, c.at), SourceFinalMessage, nil)
		if len(s.Claims) != 1 || s.Claims[0].Class != ClaimDone || s.Claims[0].Verdict != Verdict(Red) {
			t.Errorf("%s: claims %+v, want done red", c.name, s.Claims)
		}
	}
	if s := Claims([]byte("Done."), nil, claimKey, allKinds(), false, SourceFinalMessage, nil); len(s.Claims) != 0 {
		t.Errorf("no relevant write: claims %v, want none", classes(s))
	}

	for _, c := range []struct {
		name  string
		calls []call
		at    int64
		want  bool
	}{
		{"a source edit", []call{edit(1, "pkg/a.go")}, 2, true},
		{"go.mod only", []call{edit(1, "go.mod")}, 2, true},
		{"docs only", []call{edit(1, "README.md")}, 2, false},
		{"CI only", []call{edit(1, ".github/workflows/ci.yml")}, 2, false},
		{"a named shell write", []call{bashWrite(1, "pkg/a.go")}, 2, true},
		{"a write the record cannot name", []call{unnamedWrite(1)}, 2, false},
		{"a relevant write past the point", []call{edit(1, "README.md"), edit(2, "pkg/a.go")}, 2, false},
		{"the same write at the point", []call{edit(1, "README.md"), edit(2, "pkg/a.go")}, 3, true},
		{"no write", []call{gateCall(1, "go-test", store.ExecOK)}, 2, false},
	} {
		if got := WroteRelevant(runOf(t, c.calls...), c.at); got != c.want {
			t.Errorf("%s: WroteRelevant %t, want %t", c.name, got, c.want)
		}
	}
	if WroteRelevant(nil, 10) {
		t.Error("a nil run wrote")
	}
}

// TestVER60_DisclosedClaimIsLoose: a disclosure in the claim's sentence or the
// next makes it loose; two sentences on it does not; a phrase matches whole;
// a predicate's own words are no disclosure (20.2.5 step 8; Gate A Q3, Q7, Q8).
func TestVER60_DisclosedClaimIsLoose(t *testing.T) {
	for _, c := range []struct {
		msg  string
		want []string
	}{
		{"All tests pass, one test failed.", []string{"tests_pass/false"}},
		{"All tests pass. One test failed.", []string{"tests_pass/false"}},
		{"All tests pass. Lint is clean. One test failed.", []string{"tests_pass/true", "lint_clean/false"}},
		{"Tests passed in the background.", []string{"tests_pass/false"}},
		{"Tests pass against the background fixture.", []string{"tests_pass/true"}},
		{"Tests pass in the old background.", []string{"tests_pass/true"}},
		{"tsc reports no errors.", []string{"typecheck_clean/true"}},
		{"tsc reports no errors, one warning.", []string{"typecheck_clean/false"}},
		{"golangci-lint reports 0 issues; two issues remain in vendor.", []string{"lint_clean/false"}},
		{"Tests pass with two failures.", []string{"tests_pass/false"}},
		{"The build succeeded with warnings.", []string{"build_ok/false"}},
		{"All tests pass. Lint reports no issues.", []string{"tests_pass/true", "lint_clean/true"}},
		{"All tests pass. Two issues remain.", []string{"tests_pass/false"}},
		{"Lint is clean, no issues.", []string{"lint_clean/true"}},
	} {
		wantClasses(t, c.msg, scanOf(c.msg, allKinds()...), c.want...)
	}
}

// TestVER61_CutMakesEveryClaimLoose: a tail, or a head over
// MaxClaimMessageBytes, sets Cut and loosens every claim; the cut falls on a
// rune boundary; the head is read before the tail; at most MaxClaims verdicts
// are kept, by severity, and the rest counted (Gate A Q6).
func TestVER61_CutMakesEveryClaimLoose(t *testing.T) {
	s := Claims([]byte("All tests pass."), []byte("Lint is clean."), claimKey, allKinds(), true, SourceFinalMessage, nil)
	if !s.Cut {
		t.Error("a tail did not set Cut")
	}
	wantClasses(t, "head and tail", s, "tests_pass/false", "lint_clean/false")

	pad := strings.Repeat("x", MaxClaimMessageBytes-len("All tests pass. ")-1)
	long := []byte("All tests pass. " + pad + "※Lint is clean.") // the mark's 3 bytes straddle the cap
	head, cut := capWindow(long)
	if !cut || len(head) != MaxClaimMessageBytes-1 || !utf8.Valid(head) {
		t.Errorf("capWindow: %d bytes, cut %t, valid %t; want %d, true, true", len(head), cut, utf8.Valid(head), MaxClaimMessageBytes-1)
	}
	if h, cut := capWindow(long[:MaxClaimMessageBytes]); cut || len(h) != MaxClaimMessageBytes {
		t.Errorf("a window of exactly the cap: %d bytes, cut %t", len(h), cut)
	}
	s = Claims(long, nil, claimKey, allKinds(), true, SourceFinalMessage, nil)
	if !s.Cut {
		t.Error("a head over the cap did not set Cut")
	}
	wantClasses(t, "an over-long head", s, "tests_pass/false")

	// The cap keeps the worst: one red at sentence 90 of 100 survives it.
	gs := []GateState{gate("test", Green, "t"), gate("lint", Red, "l")}
	var b strings.Builder
	for i := range 100 {
		if i == 90 {
			b.WriteString("Lint is clean. ")
		} else {
			b.WriteString("All tests pass. ")
		}
	}
	s = scanOf(b.String(), gs...)
	if len(s.Claims) != MaxClaims || s.ClaimsOmitted != 36 || s.Claims[MaxClaims-1].Verdict != Verdict(Red) {
		t.Fatalf("100 claims, one red at 90: kept %v, omitted %d; want %d ending in the red, 36", classes(s), s.ClaimsOmitted, MaxClaims)
	}

	// no_gate_known ranks after green: one green at 90 of 100 survives.
	b.Reset()
	for i := range 100 {
		if i == 90 {
			b.WriteString("All tests pass. ")
		} else {
			b.WriteString("The scan is clean. ")
		}
	}
	s = scanOf(b.String(), gs...)
	if len(s.Claims) != MaxClaims || s.Claims[MaxClaims-1].Verdict != Verdict(Green) || s.Claims[0].Verdict != NoGateKnown {
		t.Fatalf("99 no_gate_known, one green at 90: kept %v; want %d ending in the green", classes(s), MaxClaims)
	}

	// Between equal verdicts the strict are kept before the loose.
	b.Reset()
	for i := range 100 {
		if i < 70 {
			b.WriteString("All tests pass, one skipped. ")
		} else {
			b.WriteString("All tests pass. ")
		}
	}
	s = scanOf(b.String(), gs...)
	strict := 0
	for _, c := range s.Claims {
		if c.Strict {
			strict++
		}
	}
	if len(s.Claims) != MaxClaims || strict != 30 || firstClaim(t, s).Strict {
		t.Errorf("70 loose then 30 strict: kept %d with %d strict; want %d with 30, the first loose", len(s.Claims), strict, MaxClaims)
	}
}

// TestVER62_SentenceSplitAndLastQuestion: sentences end at . ! or ? before
// space or end, and at a newline; the last sentence alone sets LastIsQuestion;
// verdicts come in sentence order, then ClaimClasses order (20.2.5 step 3).
func TestVER62_SentenceSplitAndLastQuestion(t *testing.T) {
	if s := scanOf("All tests pass. Should I commit?", allKinds()...); !s.LastIsQuestion {
		t.Error("a closing question did not set LastIsQuestion")
	}
	if s := scanOf("Should I commit? All tests pass.", allKinds()...); s.LastIsQuestion || len(s.Claims) != 1 {
		t.Errorf("an earlier question: LastIsQuestion %t, claims %v", s.LastIsQuestion, classes(s))
	}
	wantClasses(t, "sentence order", scanOf("Lint is clean. All tests pass.", allKinds()...), "lint_clean/true", "tests_pass/true")
	wantClasses(t, "two classes", scanOf("Lint is clean and all tests pass.", allKinds()...), "tests_pass/true", "lint_clean/true")
	wantClasses(t, "newlines", scanOf("All tests pass\nthe build failed\nlint is clean", allKinds()...), "tests_pass/false", "lint_clean/true")

	if q := firstClaim(t, scanOf("All tests pass\nlint is clean", allKinds()...)).QuoteHMAC; q != quoteOf(claimKey, "All tests pass") {
		t.Errorf("a newline did not end the sentence: quote %q", q)
	}
	if q := firstClaim(t, scanOf("Tests pass in v1.2.3. Lint is clean.", allKinds()...)).QuoteHMAC; q != quoteOf(claimKey, "Tests pass in v1.2.3.") {
		t.Errorf("a dot before a digit ended the sentence: quote %q", q)
	}
	if s := scanOf("", allKinds()...); s.Claims == nil || len(s.Claims) != 0 || s.LastIsQuestion || s.Cut {
		t.Errorf("an empty message: %+v", s)
	}
}

// TestVER63_ClaimsIsPure: the same inputs give the same bytes, the gates'
// order changes nothing, and no input is written to.
func TestVER63_ClaimsIsPure(t *testing.T) {
	msg := "Done. All tests pass, but one is skipped. `go test` passes. Lint is clean and tsc reports no errors."
	gs := append(allKinds(), gate("test", Red, "z"), advisory(gate("scan", Red, "y")))
	head := []byte(msg)
	first, _ := json.Marshal(Claims(head, nil, claimKey, gs, true, SourceFinalMessage, nil))
	for range 20 {
		again, _ := json.Marshal(Claims(head, nil, claimKey, gs, true, SourceFinalMessage, nil))
		if !bytes.Equal(again, first) {
			t.Fatalf("two runs differ:\n%s\n%s", first, again)
		}
	}
	rev := slices.Clone(gs)
	slices.Reverse(rev)
	if again, _ := json.Marshal(Claims(head, nil, claimKey, rev, true, SourceFinalMessage, nil)); !bytes.Equal(again, first) {
		t.Errorf("reversed gates differ:\n%s\n%s", first, again)
	}
	if string(head) != msg {
		t.Errorf("the head was written to: %q", head)
	}
}
