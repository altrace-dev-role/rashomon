package main

import (
	"bytes"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The three reasons a backtest gives when it refuses a pattern.
const (
	rh7bReplayNever = "the replay of this project's tool calls never fires it"
	rh7bStale       = "it is raised only for a gate that a learned project index or a trusted or managed spec requires, and a replay holds neither"
	rh7bReserved    = "it is reserved and not built: no code raises it, live or replayed"
)

// TestRH7b_BacktestRefusesPatternNoReplayFires: a backtest replays the tool
// calls of a transcript through PreToolUse and the post hooks with no port
// wired, against a scratch store that holds no project index and no trusted
// spec, so it can fire five patterns. Every other id it refuses before the
// store is read: exit 1, nothing on stdout, one stderr line that names the
// refused id and the five a backtest measures, and no count. The line gives
// one of three reasons. Ten ids are raised live, at an event or through a
// port the replay does not run: their line says the replay never fires them.
// stale-evidence is raised only for a gate that a learned project index or
// a trusted or managed spec requires, and a replay holds neither: its line
// says so. Four ids are reserved and no code raises them, live or replayed:
// their line says so, and never that the replay is the reason. The unknown
// sensor line and the usage screen's backtest entry name the five and no
// other id. The four lists are literal and must equal pattern.All() with no
// overlap, so a new id fails here until it is placed.
// Each of the five is proved by a run elsewhere: regression-location in
// TestP6_BacktestReproducesTheLiveFires (a replay of three sessions that
// fires it 1, 0 and 1 times), and the other four in TestRH7c_ReplayFixtures,
// each fired once through `rashomon backtest --sensor ID`; this test shows
// only that each of the five passes the refusal and reaches the store open.
func TestRH7b_BacktestRefusesPatternNoReplayFires(t *testing.T) {
	measured := []string{"regression-location", "platform-rule", "integrity",
		"ordering-rule", "masked-status"}
	replayNever := []string{"subtask-gate", "continuity-briefing", "declared-fact",
		"learned-fact", "subagent-claim", "claim-vs-record", "sequence-risk",
		"task-preflight", "blast-radius", "plan-check"}
	stale := []string{"stale-evidence"}
	reserved := []string{"identical-retry", "cred-spray", "agent-interference", "model-switch"}
	notMeasured := slices.Concat(replayNever, stale, reserved)

	t.Run("split", func(t *testing.T) {
		var all []string
		for _, id := range pattern.All() {
			all = append(all, string(id))
		}
		union := append(slices.Clone(measured), notMeasured...)
		slices.Sort(union)
		slices.Sort(all)
		if len(measured) != 5 || len(replayNever) != 10 || len(stale) != 1 || len(reserved) != 4 ||
			len(slices.Compact(slices.Clone(union))) != len(union) || !slices.Equal(union, all) {
			t.Errorf("measured %d, replay-never %d, stale %d, reserved %d, union %v; want 5, 10, 1 and 4, "+
				"no overlap, union pattern.All() %v", len(measured), len(replayNever), len(stale), len(reserved), union, all)
		}
	})

	// No store here: a refusal that ran after the store open would print the
	// store's error, not its own. The measured ids show the setup reaches it.
	t.Setenv("RASHOMON_HOME", t.TempDir())
	noStore := store.ErrNoStore.Error()
	btSensor := func(sensor string) (int, string, string) {
		var out, errOut bytes.Buffer
		code := run([]string{"backtest", "--sensor", sensor}, nil, &out, &errOut)
		return code, out.String(), errOut.String()
	}
	namesAll := func(msg string, ids []string) bool {
		for _, id := range ids {
			if !strings.Contains(msg, id) {
				return false
			}
		}
		return true
	}
	namesAny := func(msg string, ids []string, except string) bool {
		for _, id := range ids {
			if id != except && strings.Contains(msg, id) {
				return true
			}
		}
		return false
	}

	t.Run("measured", func(t *testing.T) {
		for _, id := range measured {
			t.Run(id, func(t *testing.T) {
				code, out, msg := btSensor(id)
				if code != 1 || out != "" || !strings.Contains(msg, noStore) {
					t.Errorf("backtest --sensor %s: exit %d, stdout %q, stderr %q; want exit 1 at the store open (%s)",
						id, code, out, msg, noStore)
				}
			})
		}
	})

	// refusedBy checks one refused id's line and the reason it gives: want
	// is the reason's words, other the reason it must not give.
	refusedBy := func(t *testing.T, id, want, other string) {
		t.Helper()
		code, out, msg := btSensor(id)
		if code != 1 || out != "" || strings.Count(msg, "\n") != 1 || !strings.Contains(msg, strconv.Quote(id)) ||
			!namesAll(msg, measured) || namesAny(msg, notMeasured, id) || strings.ContainsAny(msg, "0123456789") ||
			strings.Contains(msg, noStore) || !strings.Contains(msg, want) || strings.Contains(msg, other) {
			t.Errorf("backtest --sensor %s: exit %d, stdout %q, stderr %q; want exit 1, no stdout, one line "+
				"naming %q and the five measured ids, no other id, no count, before the store is read, "+
				"saying %q and not %q", id, code, out, msg, id, want, other)
		}
	}
	t.Run("replay-never", func(t *testing.T) {
		for _, id := range replayNever {
			t.Run(id, func(t *testing.T) { refusedBy(t, id, rh7bReplayNever, "reserved") })
		}
	})
	t.Run("stale", func(t *testing.T) {
		for _, id := range stale {
			t.Run(id, func(t *testing.T) { refusedBy(t, id, rh7bStale, rh7bReplayNever) })
		}
	})
	t.Run("reserved", func(t *testing.T) {
		for _, id := range reserved {
			t.Run(id, func(t *testing.T) { refusedBy(t, id, rh7bReserved, "the replay") })
		}
	})

	t.Run("unknown", func(t *testing.T) {
		code, out, msg := btSensor("no-such-pattern")
		if code != 1 || out != "" || !namesAll(msg, measured) || namesAny(msg, notMeasured, "") {
			t.Errorf("backtest --sensor no-such-pattern: exit %d, stdout %q, stderr %q; want exit 1 naming the five "+
				"measured ids and no other", code, out, msg)
		}
	})

	// The usage screen says what --sensor takes: the five, and no other id.
	t.Run("usage", func(t *testing.T) {
		if !namesAll(backtestUsage, measured) || namesAny(backtestUsage, notMeasured, "") {
			t.Errorf("the backtest usage entry %q; want it to name the five measured ids and no other", backtestUsage)
		}
	})
}
