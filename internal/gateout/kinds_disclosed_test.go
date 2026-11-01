package gateout_test

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/disclose"
	"github.com/altrace-dev-role/rashomon/internal/gateout"
)

// TestFF1_EveryGateOutputKindIsDisclosed: each word gate_output.kind may hold
// (gateout.Kinds) maps to the words of the consent text's STORED paragraph
// that disclose it, so a kind added without its disclosure fails here.
func TestFF1_EveryGateOutputKindIsDisclosed(t *testing.T) {
	const printed = "the exit status the line printed for its runner as pass or fail"
	words := map[string]string{
		"summary_pass":          "a passing or failing summary",
		"summary_fail":          "a passing or failing summary",
		"fail_line":             "a failure line",
		"empty_run":             "an empty run",
		"no_summary":            "or none found",
		"unattributable":        "or none found",
		"unread":                "or none found",
		gateout.KindPrintedPass: printed,
		gateout.KindPrintedFail: printed,
	}
	inv, err := disclose.Default()
	if err != nil {
		t.Fatal(err)
	}
	var stored string
	for _, p := range inv.Paragraphs {
		if strings.HasPrefix(p.Text, disclose.StoredHeading) {
			stored = p.Text
		}
	}
	if stored == "" {
		t.Fatal("no STORED paragraph in the consent text")
	}
	for _, k := range gateout.Kinds() {
		w, ok := words[k]
		if !ok {
			t.Errorf("gate_output kind %q has no disclosure here: name it in the STORED paragraph and map it in this test", k)
			continue
		}
		if !strings.Contains(stored, w) {
			t.Errorf("gate_output kind %q: the STORED paragraph does not say %q", k, w)
		}
	}
}
