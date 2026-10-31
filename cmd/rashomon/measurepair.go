package main

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/hook"
	"github.com/altrace-dev-role/rashomon/internal/measure"
	"github.com/altrace-dev-role/rashomon/internal/settings"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// measurePairUsage is the usage screen's measure pair entry, joined after
// measureUsage.
const measurePairUsage = `  rashomon measure pair --session ID --arm off|steer [--home DIR]
                       [--config DIR] --session ID --arm off|steer
                       [--home DIR] [--config DIR] [--json]
                               two sessions of a paired run side by side,
                               from the record and the transcripts; each
                               --arm is the RASHOMON_STEER_ARM its session
                               ran under, --home its store and --config its
                               Claude Code directory; writes nothing

`

// measurePair is `rashomon measure pair`: two sessions, each with the arm the
// driver forced on it, read side by side from its store and the transcripts
// under its Claude Code configuration directory, by default this process's.
// It reads only.
func measurePair(args []string, stdout io.Writer) error {
	asks, asJSON, err := pairArgs(args)
	if err != nil {
		return err
	}
	for i := range asks {
		if asks[i].Root == "" {
			if asks[i].Root, err = store.DefaultRoot(); err != nil {
				return err
			}
		}
		if asks[i].ConfigDir == "" {
			if asks[i].ConfigDir, err = settings.ConfigDir(); err != nil {
				return err
			}
		}
	}
	p, err := measure.PairOf(asks[0], asks[1], time.Now())
	if errors.Is(err, store.ErrNoStore) {
		return fmt.Errorf("nothing has been recorded there, so there is no session to read: %w", err)
	}
	if err != nil {
		return err
	}
	if asJSON {
		return measure.JSON(stdout, p)
	}
	return measure.Text(stdout, p)
}

// pairArgs reads two `--session ID --arm ARM` pairs, each arm off or steer
// as SteerArm reads RASHOMON_STEER_ARM and each optionally followed by
// --home and --config for that side, and an optional --json.
func pairArgs(args []string) ([2]measure.Ask, bool, error) {
	var asks [2]measure.Ask
	n, asJSON := 0, false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			asJSON = true
		case "--session":
			if i+3 >= len(args) || args[i+2] != "--arm" {
				return asks, false, errors.New("--session needs an id and then --arm off or steer")
			}
			if n == 2 {
				return asks, false, errors.New("measure pair reads two sessions, not more")
			}
			arm := hook.SteerArm(args[i+3])
			if arm == "" {
				return asks, false, fmt.Errorf("--arm is off or steer, got %q", args[i+3])
			}
			asks[n] = measure.Ask{SessionID: args[i+1], Arm: arm}
			n++
			i += 3
		case "--home", "--config":
			if n == 0 || i+1 >= len(args) {
				return asks, false, fmt.Errorf("%s needs a directory, after the --session it is for", args[i])
			}
			dir := &asks[n-1].Root
			if args[i] == "--config" {
				dir = &asks[n-1].ConfigDir
			}
			if *dir != "" {
				return asks, false, fmt.Errorf("%s is given twice for session %s", args[i], asks[n-1].SessionID)
			}
			*dir = args[i+1]
			i++
		default:
			return asks, false, fmt.Errorf("unknown argument %q", args[i])
		}
	}
	if n != 2 {
		return asks, false, errors.New("measure pair needs two sessions, each as --session ID --arm off|steer")
	}
	return asks, asJSON, nil
}
