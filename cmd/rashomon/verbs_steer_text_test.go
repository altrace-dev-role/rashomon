package main

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/consent"
	"github.com/altrace-dev-role/rashomon/internal/disclose"
)

// TestSTC1l_ScreenNamesTheControlsOnceTheyDispatch (D213, card STEER-CLI1):
// each command the card adds is on the consent screen, in the paragraph that
// belongs to it, and TestTRU2c_ holds that each is a dispatched verb with
// words its usage lines carry. The closing table's stop-the-notes line is the
// steer switch. The disclosure's ScopeVersion is untouched: the screen names
// controls and collects nothing new.
func TestSTC1l_ScreenNamesTheControlsOnceTheyDispatch(t *testing.T) {
	inv, err := disclose.Default()
	if err != nil {
		t.Fatal(err)
	}
	var all strings.Builder
	for _, p := range inv.Paragraphs {
		all.WriteString(p.Text + "\n")
	}
	verbs := dispatchedVerbs(t)
	for _, c := range []string{
		"`rashomon steer off --user`", "`rashomon steer off --project`", "`rashomon steer explain`", "`rashomon steer log`",
		"`rashomon scorecard`", "`rashomon summary off --user`", "`rashomon summary off --project`",
	} {
		if !strings.Contains(all.String(), c) {
			t.Errorf("the screen does not name %s", c)
		}
		if verb := strings.Fields(strings.Trim(c, "`"))[1]; !verbs[verb] {
			t.Errorf("%s is named and %q is not dispatched", c, verb)
		}
	}
	// The sentence says what steer off does for the user and no more: decide's
	// flag budget is one flag per call and five per prompt, and an event with
	// no channel prints nothing.
	for _, want := range []string{"shown to you as a flag where the call has room for one", "one flag per call, five per prompt",
		"nothing is shown on an event that has no way to carry one"} {
		if !strings.Contains(all.String(), want) {
			t.Errorf("the screen's steer-off sentence lacks %q", want)
		}
	}
	if strings.Contains(all.String(), "is still shown to you") {
		t.Error("the screen says a note is still shown to you, which the flag budget and a channel-less event make untrue")
	}
	stop := false
	for _, c := range consent.Commands {
		stop = stop || c.Does == "Stop the notes to your agent" && c.Run == "rashomon steer off --user"
	}
	if !stop {
		t.Errorf("the closing table's stop-the-notes line is not the steer switch: %v", consent.Commands)
	}
	if got := disclose.ScopeVersion; got != 5 {
		t.Errorf("ScopeVersion = %d, want 5: this card changes the text only; LEARN-DELIVER1 set 5", got)
	}
}
