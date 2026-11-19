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

// TestRH7b_BacktestRefusesPatternNoReplayFires: a backtest replays the tool
// calls of a transcript through PreToolUse and the post hooks with no port
// wired, so it can fire six patterns. For the other fourteen it would print a
// count of 0 that nothing measured, so it refuses them: exit 1, nothing on
// stdout, one stderr line that names the refused id and the six a backtest
// measures, prints no count, and comes before the store is read. The unknown
// sensor line names the six too, never the fourteen. Both lists are literal
// (the item 7 table), and their union must be pattern.All(), so a new id
// fails here until it is placed.
// Of the six, only regression-location is proved by a run, in
// TestP6_BacktestReproducesTheLiveFires (a replay of three sessions that
// fires it 1, 0 and 1 times). The other five, platform-rule, integrity,
// ordering-rule, masked-status and stale-evidence, are listed by reading the
// producers a replay reaches (SteerAt and RulesAtCall); this test shows only
// that each passes the refusal and reaches the store open.
func TestRH7b_BacktestRefusesPatternNoReplayFires(t *testing.T) {
	measured := []string{"regression-location", "platform-rule", "integrity",
		"ordering-rule", "masked-status", "stale-evidence"}
	refused := []string{"subtask-gate", "continuity-briefing", "declared-fact",
		"learned-fact", "identical-retry", "cred-spray", "agent-interference",
		"subagent-claim", "model-switch", "claim-vs-record", "sequence-risk",
		"task-preflight", "blast-radius", "plan-check"}

	t.Run("split", func(t *testing.T) {
		var all []string
		for _, id := range pattern.All() {
			all = append(all, string(id))
		}
		union := append(slices.Clone(measured), refused...)
		slices.Sort(union)
		slices.Sort(all)
		if len(measured) != 6 || len(refused) != 14 || len(slices.Compact(slices.Clone(union))) != len(union) ||
			!slices.Equal(union, all) {
			t.Errorf("measured %d and refused %d, union %v; want 6 and 14, no overlap, union pattern.All() %v",
				len(measured), len(refused), union, all)
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

	t.Run("refused", func(t *testing.T) {
		for _, id := range refused {
			t.Run(id, func(t *testing.T) {
				code, out, msg := btSensor(id)
				if code != 1 || out != "" || strings.Count(msg, "\n") != 1 || !strings.Contains(msg, strconv.Quote(id)) ||
					!namesAll(msg, measured) || namesAny(msg, refused, id) || strings.ContainsAny(msg, "0123456789") ||
					strings.Contains(msg, noStore) {
					t.Errorf("backtest --sensor %s: exit %d, stdout %q, stderr %q; want exit 1, no stdout, one line "+
						"naming %q and the six measured ids, no other id, no count, before the store is read",
						id, code, out, msg, id)
				}
			})
		}
	})

	t.Run("unknown", func(t *testing.T) {
		code, out, msg := btSensor("no-such-pattern")
		if code != 1 || out != "" || !namesAll(msg, measured) || namesAny(msg, refused, "") {
			t.Errorf("backtest --sensor no-such-pattern: exit %d, stdout %q, stderr %q; want exit 1 naming the six "+
				"measured ids and none of the fourteen", code, out, msg)
		}
	})
}
