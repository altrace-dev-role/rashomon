package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/backtest"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// backtestUsage is the usage screen's backtest entry, joined as receiptUsage
// is, so usage keeps its size under Law 10.
const backtestUsage = `  rashomon backtest --sensor ID  replay this project's transcript copies
                               (rashomon bodies on) through this build's
                               hooks and print, as backtest/1 JSON lines,
                               each session's fires of steer pattern ID
                               beside the live steer log's; the store is
                               only read

`

// SchemaBacktest is backtest/1: a header line, then one line per session.
const SchemaBacktest = "backtest/1"

type backtestHeader struct {
	Schema   string `json:"schema"`
	Sensor   string `json:"sensor"`
	Sessions int    `json:"sessions"`
	Fidelity string `json:"fidelity"`
}

type backtestRow struct {
	SessionID string `json:"session_id"`
	Fires     int    `json:"fires"`
	LiveRun   bool   `json:"live_run"`
	LiveFires int    `json:"live_fires"`
	Matched   int    `json:"matched"`
}

// cmdBacktest is `rashomon backtest --sensor ID` (card P6, 6.3): each of this
// project's transcript copies replayed through the hooks against a scratch
// store, its fires of one pattern counted beside the live log's for the same
// session, matched on (pattern, call). It reads the store and writes nothing
// to it, and has no transport.
func cmdBacktest(args []string, stdout io.Writer) error {
	if len(args) > 0 && args[0] != "--sensor" {
		return fmt.Errorf("unknown argument %q", args[0])
	}
	if len(args) != 2 {
		return errors.New("backtest needs --sensor ID")
	}
	sensor := args[1]
	if !pattern.Valid(pattern.ID(sensor)) {
		var ids []string
		for _, id := range pattern.All() {
			ids = append(ids, string(id))
		}
		return fmt.Errorf("unknown sensor %q; a sensor is a steer pattern: %s", sensor, strings.Join(ids, ", "))
	}
	st, err := openStoreForRead()
	if err != nil {
		return err
	}
	_, dir, err := bodiesProject()
	if err != nil {
		return err
	}
	copies, err := st.BodyCopies(dir)
	if err != nil {
		return err
	}
	scope, _ := store.RecordScopeMarker(st.Root()) // v4 when there is none
	enc := json.NewEncoder(stdout)
	if err := enc.Encode(backtestHeader{SchemaBacktest, sensor, len(copies), backtest.Fidelity}); err != nil {
		return err
	}
	for _, path := range copies {
		row, ok, err := backtestSession(st, path, scope, sensor)
		if err != nil {
			return err
		}
		if ok {
			if err := enc.Encode(row); err != nil {
				return err
			}
		}
	}
	return nil
}

// backtestSession is one copy's row; ok is false for a copy that names no
// session.
func backtestSession(st *store.Store, path, scope, sensor string) (backtestRow, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return backtestRow{}, false, err
	}
	session, calls, err := backtest.Calls(f)
	f.Close() //nolint:errcheck // read-only
	if err != nil || session == "" {
		return backtestRow{}, false, err
	}
	replay, err := backtest.Replay(session, scope, calls)
	if err != nil {
		return backtestRow{}, false, err
	}
	live, err := backtest.Fires(st, session)
	if err != nil {
		return backtestRow{}, false, err
	}
	_, statErr := os.Stat(st.RunDir(session))
	if statErr != nil && !errors.Is(statErr, fs.ErrNotExist) {
		return backtestRow{}, false, statErr
	}
	replay, live = ofPattern(replay, sensor), ofPattern(live, sensor)
	return backtestRow{SessionID: session, Fires: len(replay), LiveRun: statErr == nil, LiveFires: len(live),
		Matched: backtest.Matched(replay, live)}, true, nil
}

func ofPattern(fires []backtest.Fire, id string) []backtest.Fire {
	var out []backtest.Fire
	for _, f := range fires {
		if f.Pattern == id {
			out = append(out, f)
		}
	}
	return out
}
