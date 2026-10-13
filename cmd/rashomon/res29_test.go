package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/test/perf/pathological"
	"github.com/altrace-dev-role/rashomon/test/perf/peakrss"
)

const res29Child = "RASHOMON_RES29_CHILD"

// res29Payload is a PreToolUse payload for a Bash call whose command is
// prefix, then unit repeated as many times as fit, then suffix, so that the
// whole payload is at most size bytes. The command is encoded as Claude Code
// encodes it, & < > unescaped; a unit costs its encoded bytes, which for a
// backslash, a newline or a quote is more than its own.
func res29Payload(t *testing.T, size int, prefix, unit, suffix string) []byte {
	t.Helper()
	pre := func(cmd string) []byte {
		return benchToolCall(t, "PreToolUse", "Bash", benchJSON(t, map[string]string{"command": cmd}), nil)
	}
	per := len(pre(unit)) - len(pre(""))
	n := (size - len(pre(prefix+suffix))) / per
	body := pre(prefix + strings.Repeat(unit, n) + suffix)
	if len(body) > size || len(body) <= size-per {
		t.Fatalf("payload is %d bytes, want at most %d and within one unit of it", len(body), size)
	}
	return body
}

// TestRES29_PathologicalInputsStayBounded: each pathological input the shape
// of a call reads exits 0 inside the hook's hard stop at peak RSS of at most
// 64 MiB, and still records the call. It runs the hook entry in a child
// process, against a started session, as the installed binary runs it.
//
// The rows are RES-29's that the shape of a call owns -- 8 MiB of short words
// (the token bound, RES-9; the first row, which the RES-9 mutant turns red),
// 8 MiB of `<`, a 1 MB `cd a&&` (the leading-cd fold, RES-10) and nesting one
// past maxNest (32 levels; 17 quoted substitutions are 34) -- and every other
// 8 MiB line shape in pathological.Lines, so the hook's measured peak on each
// is pinned rather than taken by hand. The table's other rows are pinned
// beside their own entries: the 7 MB final message by
// TestRES7_HugeFinalMessageIsConstant; the 64 MiB PostToolBatch, the 187 MB
// transcript line, the 1 MiB file before a Write, the masker's bound on
// X_TOKEN assignments, the untrusted spec and the 4,096-byte ids by the parts
// that read them, which this tree does not have yet. internal/shape's
// TestRES29_ tests bound each reading of the same shapes by allocation, which
// a peak RSS cannot attribute.
func TestRES29_PathologicalInputsStayBounded(t *testing.T) {
	if args := os.Getenv(res29Child); args != "" {
		os.Exit(cmdHook(strings.Fields(args), os.Stdin, os.Stderr))
	}
	if testing.Short() {
		t.Skip("runs the hook in child processes on 8 MiB payloads")
	}
	const (
		deadline = 4 * time.Second // the hook's hard stop (85.3.10)
		rssMiB   = 64
	)
	type row struct {
		name    string
		payload func(*testing.T) []byte
	}
	var rows []row
	for _, l := range pathological.Lines() {
		rows = append(rows, row{"8 MiB of " + l.Name, func(t *testing.T) []byte {
			return res29Payload(t, 8<<20, l.Prefix, l.Unit, l.Suffix)
		}})
	}
	nested := `cd "` + strings.Repeat(`$(echo "`, 17) + "x" + strings.Repeat(`")`, 17) + `" && ls`
	rows = append(rows,
		row{"a 1 MB cd a&&", func(t *testing.T) []byte { return res29Payload(t, 1_000_000, "", "cd a&&", "") }},
		row{"nesting one past maxNest", func(t *testing.T) []byte { return res29Payload(t, len(nested)+1024, nested, " ", "") }},
	)
	if rows[0].name != "8 MiB of short words" || rows[7].name != "8 MiB of redirections" {
		t.Fatalf("premise: the table's first row is %q and its eighth %q; RES-29 names short words and `<`", rows[0].name, rows[7].name)
	}
	// Each child runs under the reaper, so its reading is its own: on Linux a
	// child's ru_maxrss is otherwise at least this test's peak, which holds
	// the 8 MiB payload (test/perf/peakrss). A platform with no ru_maxrss
	// (windows) still runs every row for its exit, deadline and record, and
	// applies no RSS bound.
	reaper, err := peakrss.NewReaper(t.TempDir())
	if err != nil && !errors.Is(err, peakrss.ErrUnavailable) {
		t.Fatal(err)
	}
	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			root, installID := benchStarted(t)
			cmd := exec.Command(os.Args[0], "-test.run=^TestRES29_PathologicalInputsStayBounded$")
			cmd.Env = append(os.Environ(), res29Child+"="+install.Marker+" "+installID)
			cmd.Stdin = bytes.NewReader(tc.payload(t))
			var out bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &out
			var (
				rss float64
				err error
			)
			start := time.Now()
			if reaper != nil {
				rss, err = reaper.Run(cmd)
			} else {
				err = cmd.Run()
			}
			took := time.Since(start)
			if err != nil || out.Len() != 0 {
				t.Fatalf("hook: %v, output %q; want exit 0 and nothing printed", err, out.String())
			}
			if took > deadline {
				t.Errorf("hook took %v, past its %v hard stop", took, deadline)
			}
			switch {
			case reaper == nil:
				t.Log("this platform reports no peak RSS for a child; the bound is not applied")
			case raceEnabled:
				t.Logf("race detector on: %.2f MiB, the %d MiB bound is not applied to an instrumented child", rss, rssMiB)
			case rss > rssMiB:
				t.Errorf("hook peaked at %.2f MiB, over %d MiB", rss, rssMiB)
			default:
				t.Logf("hook: %v, peak RSS %.2f MiB", took.Round(time.Millisecond), rss)
			}
			st, err := store.OpenExisting(root)
			if err != nil {
				t.Fatal(err)
			}
			run, err := st.ReadRun(benchSessionID)
			if err != nil {
				t.Fatal(err)
			}
			if got := benchCounts(run); got != [5]int{1, 1, 0, 1, 0} {
				t.Errorf("store holds %v declarations, terminals, executions, verified coverage and skipped lines; want the call recorded once", got)
			}
		})
	}
}
