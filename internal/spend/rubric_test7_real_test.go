package spend

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestRubricTEST7_RealTranscriptPricesLikeClaudeCode prices a transcript
// Claude Code 2.1.280 wrote (test/fixtures/claude-code/2.1.280, captured as
// test/acceptance/rubric_test7_real_payloads_test.go describes) and compares
// the sum with the cost Claude Code itself recorded for the same model at the
// end of the session, in the transcript's cost-state line. Every line is
// read, every response is dated and priced, and the two figures agree to a
// hundredth of a cent.
func TestRubricTEST7_RealTranscriptPricesLikeClaudeCode(t *testing.T) {
	raw, err := os.ReadFile("../../test/fixtures/claude-code/2.1.280/transcript.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	config := t.TempDir()
	dir := filepath.Join(config, "projects", "-private-tmp-rashomon-fixture-wd")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "3cb822fe-8dcf-48d0-b9ca-725c8eed47f9.jsonl"), raw, 0o600); err != nil {
		t.Fatal(err)
	}

	var claudeCode float64
	for _, line := range strings.Split(string(raw), "\n") {
		var cs struct {
			Type       string `json:"type"`
			ModelUsage map[string]struct {
				CostUSD float64 `json:"costUSD"`
			} `json:"modelUsage"`
		}
		if json.Unmarshal([]byte(line), &cs) == nil && cs.Type == "cost-state" {
			claudeCode = cs.ModelUsage["claude-haiku-4-5"].CostUSD
		}
	}
	if claudeCode <= 0 {
		t.Fatal("premise: the transcript's cost-state line names no claude-haiku-4-5 cost")
	}

	found, err := Discover(config, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	sc, err := Read(found)
	if err != nil {
		t.Fatal(err)
	}
	if sc.Unreadable != 0 || sc.Unparsed != 0 || sc.Undated != 0 || len(sc.Responses) == 0 {
		t.Fatalf("read %d responses with %d unreadable files, %d unparsed lines, %d undated",
			len(sc.Responses), sc.Unreadable, sc.Unparsed, sc.Undated)
	}
	at := time.Date(2026, 10, 13, 19, 0, 0, 0, time.UTC)
	sum := Build(sc, at, 1)
	if !sum.Total.Known() || sum.Responses != len(sc.Responses) {
		t.Fatalf("priced %d of %d responses, %d unpriced", sum.Total.Priced, sum.Responses, sum.Total.Unpriced)
	}
	got := float64(sum.Total.Nano) / 1e9
	t.Logf("%d responses: spend $%.7f, Claude Code $%.7f", sum.Responses, got, claudeCode)
	if math.Abs(got-claudeCode) > 0.0001 {
		t.Errorf("spend prices the session at $%.7f, Claude Code recorded $%.7f", got, claudeCode)
	}
}
