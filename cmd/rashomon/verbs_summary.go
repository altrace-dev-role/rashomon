package main

import (
	"errors"
	"fmt"
	"io"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// summaryUsage is the usage screen's lines for the summary verb.
const summaryUsage = `  rashomon summary [on|off] [--user|--project]
                               say whether the end-of-turn line prints its value
                               line (on unless set off), or set it for you
                               (--user, the default) or for this project
                               (--project); the done flag still prints when
                               summary is off

`

// cmdSummary is `rashomon summary [on|off] [--user|--project]` (cards
// REPORT1a, FUN-16, and STEER-CLI1 for the scopes): with no on or off it says
// the switch's state, with one it persists it in the store's config.json,
// under summary for the user and summary_projects for the project. Nothing is
// written for a bare query, and nothing is created where no store exists: a
// switch with no store to hold it would be a promise the Stop hook never
// reads (the rule pause follows).
func cmdSummary(args []string, stdout io.Writer) error {
	project, words, err := switchScope(args)
	if err != nil {
		return err
	}
	for i, a := range words {
		if i > 0 || a != "on" && a != "off" {
			return fmt.Errorf("unknown argument %q", a)
		}
	}
	st, err := openStoreForRead()
	if errors.Is(err, store.ErrNoStore) {
		if len(words) == 0 {
			_, err = fmt.Fprintln(stdout, "summary: on (nothing has been recorded here, so no switch is set)")
			return err
		}
		return errors.New("nothing has been recorded here yet, so there is no store to hold the switch")
	}
	if err != nil {
		return err
	}
	key, err := projectKey()
	if err != nil {
		return err
	}
	c, err := store.ReadConfig(st.Root())
	if err != nil {
		return err
	}
	if len(words) == 1 {
		on := words[0] == "on"
		if c, err = store.UpdateConfig(st.Root(), func(c *store.Config) {
			switch {
			case project && !on:
				if c.SummaryProjects == nil {
					c.SummaryProjects = map[string]string{}
				}
				c.SummaryProjects[key] = store.SwitchOff
			case project:
				delete(c.SummaryProjects, key)
			default:
				c.Summary = &on
			}
		}); err != nil {
			return err
		}
	}
	state := "on"
	if !c.SummaryOn() {
		state = "off"
	}
	if project {
		state = "on"
		if c.SummaryProjects[key] == store.SwitchOff {
			state = "off"
		}
		_, err = fmt.Fprintf(stdout, "summary: %s for this project (%s)\n", state, safeText(key))
		return err
	}
	_, err = fmt.Fprintf(stdout, "summary: %s\n", state)
	return err
}
