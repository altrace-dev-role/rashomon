package main

import (
	"errors"
	"fmt"
	"io"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// summaryUsage is the usage screen's lines for the summary verb.
const summaryUsage = `  rashomon summary [on|off]      say whether the end-of-turn line prints its value
                               line (on unless set off); the done flag still
                               prints when summary is off

`

// cmdSummary is `rashomon summary [on|off]` (card REPORT1a, FUN-16): with no
// argument it says the switch's state, with one it persists it in the store's
// config.json under the key summary. Nothing is written for a bare query, and
// nothing is created where no store exists: a switch with no store to hold it
// would be a promise the Stop hook never reads (the rule pause follows).
func cmdSummary(args []string, stdout io.Writer) error {
	for i, a := range args {
		if i > 0 || a != "on" && a != "off" {
			return fmt.Errorf("unknown argument %q", a)
		}
	}
	st, err := openStoreForRead()
	if errors.Is(err, store.ErrNoStore) {
		if len(args) == 0 {
			_, err = fmt.Fprintln(stdout, "summary: on (nothing has been recorded here, so no switch is set)")
			return err
		}
		return errors.New("nothing has been recorded here yet, so there is no store to hold the switch")
	}
	if err != nil {
		return err
	}
	c, err := store.ReadConfig(st.Root())
	if err != nil {
		return err
	}
	if len(args) == 1 {
		on := args[0] == "on"
		c.Summary = &on
		if err := store.WriteConfig(st.Root(), c); err != nil {
			return err
		}
	}
	state := "on"
	if !c.SummaryOn() {
		state = "off"
	}
	_, err = fmt.Fprintf(stdout, "summary: %s\n", state)
	return err
}
