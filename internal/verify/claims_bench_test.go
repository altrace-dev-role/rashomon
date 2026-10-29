package verify

import "testing"

// claimsBenchHead is a full head window of some 570 summary sentences, about
// 115 bytes each, with claims of every class, a loose one, a modal one, a
// code span and a URL among them (card 1.7 item 10).
func claimsBenchHead() []byte {
	sentences := []string{
		"All 42 tests pass in the parser package on the release branch after the refactor of the tokenizer and its helpers.",
		"The project compiles cleanly with the new build tags, and the binary size stayed within two percent of the last one.",
		"golangci-lint reports 0 issues across the module, including the vendored packages that the config used to exclude.",
		"tsc reports no errors for the frontend workspace, and the generated declaration files match the committed copies.",
		"semgrep is clean on the changed files, and the dependency audit found nothing new beyond the two items from May.",
		"I moved the retry logic into its own function so the handler reads top to bottom and each branch has a single exit.",
		"The integration suite still needs a database, so I left it out of this run and noted it in the pull request body.",
		"Done with the migration of the config loader; the old flags still parse and print a deprecation warning when used.",
		"If the flaky scheduler test goes red again, rerun it under the race detector, since the fault only shows under load.",
		"See the notes in `docs/claims.md` for the lexicon, and https://example.com/ci/runs/12345 for the run behind them.",
	}
	var b []byte
	for i := 0; len(b) < MaxClaimMessageBytes; i++ {
		b = append(append(b, sentences[i%len(sentences)]...), ' ')
	}
	return b[:MaxClaimMessageBytes]
}

// claimsBenchGates is twelve gates: one green of each kind, three more test
// gates, a stale build, a red lint, and two advisory ones.
func claimsBenchGates() []GateState {
	gs := allKinds()
	gs = append(gs, gate("test", Green, "t1"), gate("test", Unknown, "t2"), gate("test", Green, "t3"),
		gate("build", Stale, "b1"), gate("lint", Red, "l1"), advisory(gate("scan", Red, "s1")), advisory(gate("test", Red, "t4")))
	return gs
}

func BenchmarkClaims(b *testing.B) {
	// A table of literal names: RES36 reads every b.Run name statically.
	for _, c := range []struct {
		name string
		head []byte
	}{{"head_64k", claimsBenchHead()}} {
		gs := claimsBenchGates()
		if s := Claims(c.head, nil, claimKey, gs, true, SourceFinalMessage, nil); len(s.Claims) != MaxClaims || s.ClaimsOmitted == 0 {
			b.Fatalf("the fixture gives %d claims, %d omitted; want the cap reached", len(s.Claims), s.ClaimsOmitted)
		}
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				Claims(c.head, nil, claimKey, gs, true, SourceFinalMessage, nil)
			}
		})
	}
}
