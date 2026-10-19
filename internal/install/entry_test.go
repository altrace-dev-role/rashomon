package install

import (
	"reflect"
	"strings"
	"testing"
)

// TestEntryEventsAreTheInstalledEntriesEvents: EntryEvents maps the argv of
// an installed command line back to the events whose entry runs it, by the
// same subcommand table the install writes. A subcommand that is a prefix of
// another ("recap" of "recap pending") loses to the longer one, and anything
// that is not a hook entry maps to none.
func TestEntryEventsAreTheInstalledEntriesEvents(t *testing.T) {
	for _, c := range []struct {
		args []string
		want []string
	}{
		{[]string{"hook", Marker, "abc"}, []string{EventPreToolUse}},
		{[]string{"post", Marker, "abc"}, []string{EventPostToolUse, EventPostToolUseFailure}},
		{[]string{"probe", "start", Marker, "abc"}, []string{EventSessionStart}},
		{[]string{"probe", "end"}, []string{EventSessionEnd}},
		{[]string{"recap", Marker, "abc"}, []string{EventStop, EventStopFailure}},
		{[]string{"recap", "pending", Marker, "abc"}, []string{EventUserPromptSubmit}},
		{[]string{"probe"}, nil},
		{[]string{"probe", "middle"}, nil},
		{[]string{"status"}, nil},
		{nil, nil},
	} {
		if got := EntryEvents(c.args); !reflect.DeepEqual(got, c.want) {
			t.Errorf("EntryEvents(%q) = %q, want %q", c.args, got, c.want)
		}
	}
	// Every installed event maps back to itself through its own command line.
	for _, ev := range Events {
		args := append(strings.Fields(subcommand(ev)), Marker, "abc")
		found := false
		for _, got := range EntryEvents(args) {
			if got == ev {
				found = true
			}
		}
		if !found {
			t.Errorf("%s's own entry %q does not map back to it", ev, args)
		}
	}
}

// TestTimeoutForIsTheInstalledTimeout: the exported table is the one the
// installed entries carry.
func TestTimeoutForIsTheInstalledTimeout(t *testing.T) {
	spec := Spec{Executable: "/bin/rashomon", InstallID: "abc"}
	for _, ev := range Events {
		if got, want := TimeoutFor(ev), spec.group(ev).Hooks[0].Timeout; got != want {
			t.Errorf("TimeoutFor(%s) = %d, the installed entry carries %d", ev, got, want)
		}
	}
}
