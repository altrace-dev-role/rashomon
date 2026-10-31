package main

import (
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/steer"
)

// scorecardUsage is the usage screen's lines for the scorecard verb.
const scorecardUsage = `  rashomon scorecard [--json]  what AutoSteer decided, by pattern: how many notes
                               reached the agent, how many were shown to you only,
                               held out, or not printed, and why; counts only

`

// scoreRow is one pattern's counts. Delivered and FlagOnly reached a person
// (the agent, or the user alone); Holdout was withheld on purpose; Suppressed
// printed nothing; Other is a line with no decision yet.
type scoreRow struct {
	Pattern    string `json:"pattern"`
	Decisions  int    `json:"decisions"`
	Delivered  int    `json:"delivered"`
	FlagOnly   int    `json:"flag_only"`
	Holdout    int    `json:"holdout"`
	Suppressed int    `json:"suppressed"`
	Other      int    `json:"other"`
}

// scorecard is the verb's result: a row per pattern with a line, in the
// order of the first line, and how often each reason suppressed a line.
type scorecard struct {
	Decisions    int            `json:"decisions"`
	ByPattern    []scoreRow     `json:"by_pattern"`
	SuppressedBy map[string]int `json:"suppressed_by"`
	Unreadable   int            `json:"unreadable"`
}

// score counts rows by pattern. A line counts under its delivery when it has
// one (the share counts' rule, internal/share): delivered, flag_only and
// holdout as they say, shadow and suppressed_budget as suppressed; a line
// with none counts under its state, suppressed or other.
func score(rows []steerRow) scorecard {
	sc := scorecard{ByPattern: []scoreRow{}, SuppressedBy: map[string]int{}}
	at := map[string]int{}
	for _, r := range rows {
		i, ok := at[r.Pattern]
		if !ok {
			i = len(sc.ByPattern)
			at[r.Pattern] = i
			sc.ByPattern = append(sc.ByPattern, scoreRow{Pattern: r.Pattern})
		}
		row := &sc.ByPattern[i]
		row.Decisions++
		sc.Decisions++
		switch {
		case r.Delivery == string(steer.DeliveryDelivered):
			row.Delivered++
		case r.Delivery == string(steer.DeliveryFlagOnly):
			row.FlagOnly++
		case r.Delivery == string(steer.DeliveryHoldout):
			row.Holdout++
		case r.Delivery != "-" || r.State == string(steer.LineStateSuppressed):
			row.Suppressed++
		default:
			row.Other++
		}
		if r.SuppressedBy != "-" {
			sc.SuppressedBy[r.SuppressedBy]++
		}
	}
	slices.SortStableFunc(sc.ByPattern, func(a, b scoreRow) int { return b.Decisions - a.Decisions })
	return sc
}

// cmdScorecard is `rashomon scorecard [--json]` (card STEER-CLI1): counts of
// the steer logs' decisions by pattern, read-only, nothing but closed values
// and numbers.
func cmdScorecard(args []string, stdout io.Writer) error {
	asJSON, err := jsonFlag(args)
	if err != nil {
		return err
	}
	rows, skipped, ok, err := readSteerRows(stdout)
	if !ok {
		return err
	}
	sc := score(rows)
	sc.Unreadable = skipped
	if asJSON {
		return printJSON(stdout, sc)
	}
	if sc.Decisions == 0 {
		_, err = fmt.Fprintln(stdout, "scorecard: no decisions recorded")
		return err
	}
	fmt.Fprintf(stdout, "scorecard: %d decisions\n", sc.Decisions)
	fmt.Fprintf(stdout, "%-24s %9s %9s %9s %8s %10s %6s\n", "pattern", "decisions", "delivered", "flag_only", "holdout", "suppressed", "other")
	for _, r := range sc.ByPattern {
		fmt.Fprintf(stdout, "%-24s %9d %9d %9d %8d %10d %6d\n", r.Pattern, r.Decisions, r.Delivered, r.FlagOnly, r.Holdout, r.Suppressed, r.Other)
	}
	if len(sc.SuppressedBy) > 0 {
		reasons := slices.SortedFunc(maps.Keys(sc.SuppressedBy), func(a, b string) int {
			if d := sc.SuppressedBy[b] - sc.SuppressedBy[a]; d != 0 {
				return d
			}
			return strings.Compare(a, b)
		})
		fmt.Fprint(stdout, "suppressed by:")
		for _, k := range reasons {
			fmt.Fprintf(stdout, " %s %d", k, sc.SuppressedBy[k])
		}
		fmt.Fprintln(stdout)
	}
	if skipped > 0 {
		fmt.Fprintf(stdout, "scorecard: %d line(s) could not be read\n", skipped)
	}
	return nil
}
