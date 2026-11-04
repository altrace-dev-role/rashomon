package perf

// PW1b-ON-230: the plugin starts pgraph sync -hook in the background at
// SessionStart and Stop. It is a process of its own, beside the recorder, so
// DEL-7's rule (no entry gains work without a budget) is held here for it:
// every async hook in the plugin's hooks.json has a background row in
// budgets.json, a perf run owes that row's gate, and the gate compares on
// the reference platform only, as an entry row with no override does.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/budget"
)

// asyncHooks reads the plugin's hooks.json: every hook with async true,
// keyed by its command's base name and arguments ("pgraph sync -hook"), with
// the events it runs on, sorted.
func asyncHooks(t *testing.T) map[string][]string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(moduleRoot(t), "plugin", "hooks", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Command string   `json:"command"`
				Args    []string `json:"args"`
				Async   bool     `json:"async"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatal(err)
	}
	out := map[string][]string{}
	for event, groups := range doc.Hooks {
		for _, g := range groups {
			for _, h := range g.Hooks {
				if h.Async {
					key := strings.Join(append([]string{filepath.Base(h.Command)}, h.Args...), " ")
					out[key] = append(out[key], event)
				}
			}
		}
	}
	for _, evs := range out {
		slices.Sort(evs)
	}
	return out
}

// TestPW1b_EveryAsyncHookHasABackgroundRow: each async hook the plugin runs
// has a complete, measured background row naming its command and exactly
// its events, and each row names a hook the plugin runs. Break: add an async
// hook with no row, move pgraph's entry to another event, or leave a row
// unmeasured, and this fails.
func TestPW1b_EveryAsyncHookHasABackgroundRow(t *testing.T) {
	hooks := asyncHooks(t)
	if len(hooks) == 0 {
		t.Fatal("premise: the plugin runs no async hook")
	}
	rows := map[string]budget.Background{}
	for _, r := range loadBudgets(t).Background {
		rows[strings.Join(r.Command, " ")] = r
	}
	for cmd, events := range hooks {
		r, ok := rows[cmd]
		if !ok {
			t.Errorf("the plugin runs %q in the background (%v) with no background row in budgets.json", cmd, events)
			continue
		}
		evs := slices.Sorted(slices.Values(r.Events))
		if !slices.Equal(evs, events) {
			t.Errorf("background row %s names events %v; hooks.json runs it on %v", r.ID, evs, events)
		}
		if r.ID == "" || r.CurrentMaxRSSMiBP50 <= 0 || r.CurrentWallMsP50 <= 0 || !strings.HasPrefix(r.Basis, "measured ") {
			t.Errorf("background row %+v is not a complete, measured row", r)
		}
	}
	for cmd, r := range rows {
		if _, ok := hooks[cmd]; !ok {
			t.Errorf("background row %s names %q, which the plugin does not run in the background", r.ID, cmd)
		}
	}
}

// TestPW1b_APerfRunOwesEveryBackgroundGate: a required perf run that never
// compared a background row fails, as one that skipped an entry does.
// Break: leave background rows out of Ungated, and this fails.
func TestPW1b_APerfRunOwesEveryBackgroundGate(t *testing.T) {
	b := budget.Budgets{Background: []budget.Background{{ID: "pgraph.sync_hook"}}}
	if got := Ungated(b, map[string]bool{InitGate: true}); strings.Join(got, "|") != "background pgraph.sync_hook" {
		t.Errorf("a run that compared no background row: Ungated = %q", got)
	}
	if got := Ungated(b, map[string]bool{InitGate: true, BackgroundGate("pgraph.sync_hook"): true}); len(got) != 0 {
		t.Errorf("a run that compared it: Ungated = %q", got)
	}
}

// TestPW1b_ABackgroundRowGatesOnTheReferencePlatformOnly: the row is the
// reference machine's reading, so it gates there; elsewhere it reports,
// with why, since no platform has a background reading of its own and
// gating against the reference row would assume that platform reads no
// higher. Break: gate everywhere, or nowhere, and this fails.
func TestPW1b_ABackgroundRowGatesOnTheReferencePlatformOnly(t *testing.T) {
	r := budget.Background{ID: "pgraph.sync_hook", CurrentMaxRSSMiBP50: 7.5}
	if got, gated, why := backgroundBudget(referencePlatform, r); got != 7.5 || !gated || why != "" {
		t.Errorf("on %s: %v, gated %v, %q; want 7.5, gated", referencePlatform, got, gated, why)
	}
	if got, gated, why := backgroundBudget("linux/amd64", r); got != 7.5 || gated || !strings.Contains(why, "linux/amd64") {
		t.Errorf("on linux/amd64: %v, gated %v, %q; want 7.5, report-only, saying why", got, gated, why)
	}
}
