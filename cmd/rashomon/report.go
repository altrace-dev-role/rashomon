package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/report"
	"github.com/altrace-dev-role/rashomon/internal/report/gather"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

func cmdReport(args []string, stdout io.Writer) error {
	sessionID := ""
	asJSON := false
	redact := false
	chain := false
	timeline := false
	ledger := false
	nonoTrail := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--session":
			if i+1 >= len(args) {
				return errors.New("--session needs a value")
			}
			sessionID = args[i+1]
			i++
		case "--json":
			asJSON = true
		case "--redact":
			redact = true
		case "--chain":
			chain = true
		case "--timeline":
			timeline = true
		case "--ledger":
			ledger = true
		case "--nono-audit":
			// An empty path is refused like a missing one: "" is what not
			// naming a trail means, so `--nono-audit "$NONO_TRAIL"` with the
			// variable unset would print no sandbox section and no reason.
			if i+1 >= len(args) || args[i+1] == "" {
				return errors.New("--nono-audit needs a value")
			}
			nonoTrail = args[i+1]
			i++
		default:
			return fmt.Errorf("unknown argument %q", args[i])
		}
	}
	if ledger {
		for _, f := range []struct {
			on   bool
			name string
		}{{asJSON, "--json"}, {redact, "--redact"}, {chain, "--chain"}, {timeline, "--timeline"}, {nonoTrail != "", "--nono-audit"}} {
			if f.on {
				return fmt.Errorf("--ledger prints alone: drop %s", f.name)
			}
		}
		if sessionID == "" {
			return errors.New("--ledger needs --session")
		}
		return reportLedger(sessionID, stdout)
	}
	// Opened WITHOUT creating: a command that only asks a question must not mint
	// an install identity and an HMAC key as a side effect of being asked. The
	// same rule status follows, and for the same reason.
	rep, key, err := reportOrEmpty(sessionID, nonoTrail, time.Now())
	if err != nil {
		return err
	}
	if redact {
		// Applied to the whole report before either renderer sees it, so the
		// two forms cannot disagree about what was hidden and a caller cannot
		// render the plain form from the same value by mistake.
		rep = report.Redact(rep, key)
	}
	if !asJSON {
		// --chain and --timeline expand the text listing only. The JSON
		// carries the whole structure either way: that reader is a program
		// selecting fields, not a person scrolling, and making it pass a flag
		// to receive a section would mean a consumer could parse a report and
		// silently miss one.
		opts := []report.TextOption{}
		if chain {
			opts = append(opts, report.WithChain())
		}
		if timeline {
			opts = append(opts, report.WithTimeline())
		}
		return report.Text(stdout, rep, opts...)
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(rep)
}

// reportOrEmpty builds the report, or the empty one when nothing has ever been
// recorded here.
//
// "Nothing recorded yet" is not a failure and must not read as one: it is the
// state every user is in exactly once, and it is the state in which they have
// no way to tell a broken tool from one with nothing to say. The empty report
// is the same shape as any other, so a consumer does not have to know whether
// a store exists in order to parse the answer.
// It also returns the per-install key, which --redact needs: the redaction
// digest is an HMAC under it, so a shared report cannot be dictionary-attacked
// by a recipient holding a candidate hostname. A location with no store has no
// key, and also no hosts, so the nil is never used to digest anything.
func reportOrEmpty(sessionID, nonoTrail string, now time.Time) (*report.Report, []byte, error) {
	st, err := openStoreForRead()
	if errors.Is(err, store.ErrNoStore) {
		return report.Empty(now), nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	opts := []gather.Option{gather.WithNonoTrail(nonoTrail)}
	key := st.Key()

	rep, err := gather.Build(st, sessionID, now, opts...)
	if err != nil {
		return nil, nil, err
	}
	return rep, key, nil
}
