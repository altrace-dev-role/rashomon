package main

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// cmdForget evicts records at one end of the store's timeline.
//
// --since is the privacy form: forget what just happened. --before is the
// retention form: forget what is old. They are the two open ends of one window
// and one code path, and naming both would name an empty intersection, so
// naming both is refused rather than resolved.
func cmdForget(args []string, stdout io.Writer) error {
	var from, to *time.Time
	hostArg, bodies := "", false
	for i := 0; i < len(args); i++ {
		switch flag := args[i]; flag {
		case "--bodies":
			bodies = true
		case "--host":
			if i+1 >= len(args) {
				return errors.New("--host needs a value")
			}
			// Taken as typed here and canonicalised by forgetHost, before
			// anything is opened: the store and the baselines are asked about
			// the canonical name, and an unparsed baseline is searched for the
			// typed one as well.
			hostArg = args[i+1]
			i++
		case "--since", "--before":
			if i+1 >= len(args) {
				return fmt.Errorf("%s needs a value", flag)
			}
			t, err := parseInstant(flag, args[i+1], time.Now())
			if err != nil {
				return err
			}
			if flag == "--since" {
				from = &t
			} else {
				to = &t
			}
			i++
		default:
			return fmt.Errorf("unknown argument %q", args[i])
		}
	}
	switch {
	case forgetKinds(hostArg != "", from != nil || to != nil, bodies) > 1:
		return errors.New("--host removes by destination, --since/--before by time and --bodies this project's transcript copies; pass one kind")
	case hostArg != "":
		return forgetHost(hostArg, stdout)
	case bodies:
		return forgetBodies(stdout)
	case from != nil && to != nil:
		return errors.New("--since and --before name opposite ends of the store's timeline; pass one or the other")
	case from == nil && to == nil:
		return errors.New("forget needs --since or --before <RFC3339 time or duration such as 24h>, --host <hostname>, or --bodies")
	}

	st, err := openStoreForRead()
	if errors.Is(err, store.ErrNoStore) {
		fmt.Fprintln(stdout, nothingRecordedHere)
		return nil
	}
	if err != nil {
		return err
	}
	// A run forget could not rewrite is in the error, and every other run was
	// still forgotten (store.forgetEach): say what was done, then fail. A
	// failure before any run was touched -- store.ErrForgetIncomplete is not
	// how that one is marked -- must not be followed by a line claiming
	// otherwise, the same as forgetHost already does.
	gaps, forgetErr := st.ForgetWindow(from, to, time.Now())
	if forgetErr != nil && !errors.Is(forgetErr, store.ErrForgetIncomplete) {
		return forgetErr
	}
	total := 0
	for _, g := range gaps {
		total += g.RemovedRecords
	}
	// Exactly one bound was named; the refusal above is what makes that true.
	var bound string
	if from != nil {
		bound = "since " + from.UTC().Format(time.RFC3339)
	} else {
		bound = "before " + to.UTC().Format(time.RFC3339)
	}
	fmt.Fprintf(stdout, "rashomon: forgot %d records across %d runs %s; %d gap records written\n",
		total, len(gaps), bound, len(gaps))
	return forgetErr
}

// parseInstant accepts a duration ("24h", meaning that long ago) or an RFC3339
// timestamp.
func parseInstant(flag, v string, now time.Time) (time.Time, error) {
	if d, err := time.ParseDuration(v); err == nil {
		return now.Add(-d), nil
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("%s %q is neither a duration nor an RFC3339 time", flag, v)
}
