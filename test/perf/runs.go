package perf

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/budget"
)

// Seed is the store an entry row is measured on. A recap row reads a
// session, so it is measured on the session its own benchmark row reads,
// and the RSS figure and the allocation figure describe one workload; probe
// end closes a session SessionStart opened. Every other row runs on an
// installed store with nothing in it.
type Seed struct {
	Calls      int  // a generated session of Calls calls (payload.WriteSession)
	ProbeStart bool // the session started through the binary's own `probe start`
}

// SeedFor is an entry row's seed. A recap row whose benchmark does not end
// in a call count is an error: the harness could not say what it measured.
func SeedFor(e budget.Entry) (Seed, error) {
	if len(e.Command) == 0 {
		return Seed{}, fmt.Errorf("entry %s names no command", e.ID)
	}
	switch e.Command[0] {
	case "recap":
		_, last, _ := strings.Cut(e.Benchmark, "/")
		n, err := strconv.Atoi(last)
		if err != nil || n <= 0 {
			return Seed{}, fmt.Errorf("entry %s: recap row's benchmark %q names no session size", e.ID, e.Benchmark)
		}
		return Seed{Calls: n}, nil
	case "probe":
		return Seed{ProbeStart: len(e.Command) > 1 && e.Command[1] == "end"}, nil
	}
	return Seed{}, nil
}

// Added is what one run of an entry left behind: the records.ndjson lines it
// added, the coverage.ndjson lines it added, and what it printed.
type Added struct {
	Records  int
	Coverage [][]byte
	Stdout   []byte
}

// WorkDone reports whether one run of an entry did the work its row
// budgets, so a figure measured on an early return (no store, a refused
// payload, the entry-absent path) cannot pass as the entry's cost. hook and
// post must add a record; probe exactly one coverage record of its own
// phase for session with no gap reason; recap the line naming session (the
// harness removes recap.json before each run, so every run is the turn's
// first). An entry with no check here fails.
func WorkDone(command []string, session string, a Added) error {
	if len(command) == 0 {
		return errors.New("no command, so no work-done check")
	}
	switch command[0] {
	case "hook", "post":
		if a.Records < 1 {
			return fmt.Errorf("%s added no record", command[0])
		}
		return nil
	case "probe":
		return probeDone(command, session, a.Coverage)
	case "recap":
		var out struct {
			SystemMessage string `json:"systemMessage"`
		}
		if err := json.Unmarshal(a.Stdout, &out); err != nil {
			return fmt.Errorf("recap printed no line (%q): %v", a.Stdout, err)
		}
		if !strings.Contains(out.SystemMessage, "rashomon report --session "+session) {
			return fmt.Errorf("recap's line %q does not name session %s", out.SystemMessage, session)
		}
		return nil
	}
	return fmt.Errorf("no work-done check for entry %q", strings.Join(command, " "))
}

func probeDone(command []string, session string, lines [][]byte) error {
	if len(command) < 2 || len(lines) != 1 {
		return fmt.Errorf("probe %v added %d coverage records, want 1", command[1:], len(lines))
	}
	var c struct {
		SessionID string  `json:"session_id"`
		Phase     string  `json:"phase"`
		Reason    *string `json:"reason"`
	}
	if err := json.Unmarshal(lines[0], &c); err != nil {
		return fmt.Errorf("probe's coverage record %q: %v", lines[0], err)
	}
	if c.Phase != command[1] || c.SessionID != session {
		return fmt.Errorf("probe %s wrote phase %q for session %q", command[1], c.Phase, c.SessionID)
	}
	if c.Reason != nil {
		return fmt.Errorf("probe %s took the %s path", command[1], *c.Reason)
	}
	return nil
}

// InitGate, EntryGate and CaseGate name the gates the harness runs, so a
// run can be checked for the ones it owed and skipped.
const InitGate = "init"

// EntryGate names an entry row's gate.
func EntryGate(id string) string { return "entry " + id }

// CaseGate names a RES-28 case's gate.
func CaseGate(id string) string { return "case " + id }

// CaseGates reports whether a RES-28 case gates in this tree: the 400 B and
// 40 KB cases from P-08; the 1, 4 and 8 MiB cases switch on with P-10.
func CaseGates(c budget.RSSCase) bool { return c.GatesFrom == "P-08" }

// Ungated lists, sorted, every gate b owes on this tree that ran does not
// hold: every entry row, every case CaseGates admits, and the init budget.
// A required perf run (RASHOMON_PERF_REQUIRE_ALL=1, as CI runs it) fails on
// any, because `go test` passes a run in which everything skipped.
func Ungated(b budget.Budgets, ran map[string]bool) []string {
	owed := []string{InitGate}
	for _, e := range b.Entries {
		owed = append(owed, EntryGate(e.ID))
	}
	for _, c := range b.RSSCases {
		if CaseGates(c) {
			owed = append(owed, CaseGate(c.ID))
		}
	}
	var missing []string
	for _, g := range owed {
		if !ran[g] {
			missing = append(missing, g)
		}
	}
	sort.Strings(missing)
	return missing
}
