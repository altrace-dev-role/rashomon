package main

import (
	"bytes"
	"strings"
	"testing"
	"unicode"

	"github.com/altrace-dev-role/rashomon/internal/install"
)

// TestRubricNAMING4_HelpNamesEveryInstalledEvent: --help names every event
// watch installs and every subcommand those entries run. It said watch
// installed seven events while it wrote eight: the eighth, UserPromptSubmit,
// runs `recap pending` on every prompt, and neither appeared anywhere in the
// usage text. Events are matched as whole words, so PostToolUse is not
// found inside PostToolUseFailure, nor Stop inside StopFailure.
func TestRubricNAMING4_HelpNamesEveryInstalledEvent(t *testing.T) {
	var b bytes.Buffer
	usage(&b)
	help := b.String()
	// The events are named where watch is described, not merely somewhere:
	// the watch line is what a reader consults to learn what watch writes.
	start, end := strings.Index(help, "rashomon watch"), strings.Index(help, "rashomon detach")
	if start < 0 || end < start {
		t.Fatalf("--help has no watch line ahead of detach:\n%s", help)
	}
	words := map[string]bool{}
	for _, w := range strings.FieldsFunc(help[start:end], func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		words[w] = true
	}

	const id = "0123456789abcdef0123456789abcdef"
	spec := install.Spec{Executable: "rashomon", InstallID: id}
	for _, event := range install.Events() {
		if !words[event] {
			t.Errorf("--help's watch line does not name the %s event, which watch installs", event)
		}
		invocation := strings.TrimSuffix(spec.Command(event), " "+install.Marker+" "+id)
		if !strings.Contains(help, invocation) {
			t.Errorf("--help does not list %q, which the %s entry runs", invocation, event)
		}
	}
}
