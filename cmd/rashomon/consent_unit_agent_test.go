package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/consent"
)

// An agent session cannot withdraw a consent unit: granting one is refused by
// askConsent, but `off` asks nothing, so it carries its own refusal. Each unit
// names the command to run in a terminal and nothing is appended to
// consent.json.
func TestAGCO1_AnAgentSessionCannotWithdrawAConsentUnit(t *testing.T) {
	root := consentHere(t)
	grantBodiesHere(t, root)
	before, err := os.ReadFile(consent.Path(root))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAUDECODE", "1")
	// The units this release has a paragraph for; the others are refused
	// earlier as nothing to agree to.
	for _, unit := range []string{"bodies", "share.outcomes"} {
		var stdout, stderr bytes.Buffer
		code := run([]string{"consent", unit, "off"}, nil, &stdout, &stderr)
		if code != exitFail || stdout.Len() != 0 || !strings.Contains(stderr.String(), "agent session; nothing was written") || !strings.Contains(stderr.String(), "rashomon consent "+unit+" off") {
			t.Errorf("consent %s off in an agent session: exit %d, stdout %q, stderr %q", unit, code, stdout.String(), stderr.String())
		}
	}
	if after, _ := os.ReadFile(consent.Path(root)); !bytes.Equal(after, before) {
		t.Errorf("an agent session changed consent.json: %q, was %q", after, before)
	}
}
