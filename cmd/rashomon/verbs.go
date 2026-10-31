package main

import (
	"encoding/json"
	"fmt"
	"io"
)

// verbNamed is the command a person types that run's switch does not list
// case by case, nil for none: its default branch looks a name up here, so a
// new verb costs main.go no line (Law 10's ratchet on main.go). A switch
// rather than a map, which every hook entry would build at start-up.
func verbNamed(name string) func(args []string, stdout io.Writer) error {
	switch name {
	case "status":
		return cmdStatusVerb
	case "lessons":
		return cmdLessons
	case "summary":
		return cmdSummary
	case "why":
		return cmdWhy
	}
	return nil
}

// verbUsage is the usage screen's lines for the verbs above that it does not
// already list: one const per verb, one per line of the concatenation, so a
// card adding a verb adds a const and a line and two such cards do not
// conflict on one raw string.
const verbUsage = lessonsUsage +
	summaryUsage +
	whyUsage

const lessonsUsage = `  rashomon lessons [--json]      list this project's lessons by tier, proven
                               first, each with the day it entered and its
                               fires; a ledger written by anything but
                               rashomon is said quarantined first

`

// jsonFlag reads a verb's arguments, of which --json is the only one.
func jsonFlag(args []string) (bool, error) {
	for _, a := range args {
		if a != "--json" {
			return false, fmt.Errorf("unknown argument %q", a)
		}
	}
	return len(args) > 0, nil
}

// printJSON is a verb's --json: one object, indented as report's is.
func printJSON(stdout io.Writer, v any) error {
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
