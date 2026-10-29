package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/hookio"
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

// res29PostPayload is a PostToolUse payload for `go test ./...` whose stdout
// is go test's pass lines repeated to as many bytes as the post's intake takes
// (hookio.MaxPayloadBytes; one more is refused before anything reads it): the
// largest gate run's output the post can be handed, eight times the
// gateout.MaxScanBytes it decodes.
func res29PostPayload(t *testing.T) []byte {
	t.Helper()
	post := func(stdout string) []byte {
		return benchToolCall(t, "PostToolUse", "Bash", benchJSON(t, map[string]string{"command": "go test ./..."}),
			benchJSON(t, map[string]string{"stdout": stdout, "stderr": ""}))
	}
	const unit = "--- PASS: TestX (0.00s)\n"
	per := len(post(unit)) - len(post(""))
	body := post(strings.Repeat(unit, (hookio.MaxPayloadBytes-len(post("")))/per))
	if len(body) > hookio.MaxPayloadBytes || len(body) <= hookio.MaxPayloadBytes-per {
		t.Fatalf("payload is %d bytes, want at most %d and within one unit of it", len(body), hookio.MaxPayloadBytes)
	}
	return body
}

// TestRES29_PathologicalInputsStayBounded: each pathological input the shape
// of a call reads exits 0 inside the hook's hard stop at peak RSS of at most
// rssMiB (64 on linux, 76 on darwin -- see its own comment), and still
// records the call. It runs the hook entry in a child process, against a
// started session, as the installed binary runs it.
//
// The rows are RES-29's that the shape of a call owns -- 8 MiB of short words
// (the token bound, RES-9; the first row, which the RES-9 mutant turns red),
// 8 MiB of `<`, a 1 MB `cd a&&` (the leading-cd fold, RES-10) and nesting one
// past maxNest (32 levels; 17 quoted substitutions are 34) -- and every other
// 8 MiB line shape in pathological.Lines, so the hook's measured peak on each
// is pinned rather than taken by hand -- and one row for the post entry: a
// gate run's stdout at the intake bound, which the post decodes at most 1 MiB
// of and records unread, too_large (card GATEOUT-WIRE1; its PostToolUse and
// PostToolUseFailure entries have the same 5 s timeout, so the same 4 s
// ceiling). The table's other rows are pinned
// beside their own entries: the 7 MB final message by
// TestRES7_HugeFinalMessageIsConstant; the 64 MiB PostToolBatch, the 187 MB
// transcript line, the 1 MiB file before a Write, the masker's bound on
// X_TOKEN assignments, the untrusted spec and the 4,096-byte ids by the parts
// that read them, which this tree does not have yet. internal/shape's
// TestRES29_ tests bound each reading of the same shapes by allocation, which
// a peak RSS cannot attribute.
func TestRES29_PathologicalInputsStayBounded(t *testing.T) {
	if args := os.Getenv(res29Child); args != "" {
		// The child enters through run(), as the installed binary does, so
		// the RSS budget is measured under the runtime pin run() applies to
		// every hook entry (applyHookRuntimeLimits), not whatever GC and
		// memory-limit settings the child inherited. Calling cmdHook
		// directly skipped it.
		// args name the entry first: hook for a PreToolUse row, post for the
		// gate run's output row.
		os.Exit(run(strings.Fields(args), os.Stdin, os.Stdout, os.Stderr))
	}
	if testing.Short() {
		t.Skip("runs the hook in child processes on 8 MiB payloads")
	}
	const (
		// deadline is the test's own ceiling on an uninstrumented build, not
		// a hard stop this binary enforces: 85.3.10 budgets a supervisor
		// that kills the child at this mark, but it is not built before
		// P-23 (test/perf/gate.go:246), and cmdHook has no timer of its
		// own. 4 s is the installed PreToolUse entry's 5 s Claude Code
		// timeout minus the 1 s margin 85.3.10 gives every 5 s entry
		// (plugin/hooks/hooks.json), so a hook this slow would miss that
		// external timeout too, not just this test.
		deadline = 4 * time.Second
	)
	// rssMiB is platform-specific, not one portable number: measured
	// 2026-10-18 on the git@ scheme runs row, the one that runs this test
	// closest to its ceiling, the same child binary peaks at 54.78 MiB on
	// linux/amd64 (test/perf/peakrss, docker/colima) against 65.11-65.25 MiB
	// on darwin/arm64 for the identical payload -- a ~19% gap in resident
	// memory for the same allocations, which Go's runtime costs more of per
	// allocation on darwin/arm64, not more work done. A single bound tight
	// enough to mean something on linux was flaky on darwin by design, not
	// by regression; a bound loose enough to never flake on darwin stopped
	// meaning anything on linux, where CI actually runs (-race, go1.25.8).
	// Each platform keeps its own margin instead.
	rssMiB := 64.0
	if runtime.GOOS == "darwin" {
		rssMiB = 76
	}
	// withheld marks the row whose record holds a value over its store cap:
	// the git@ line is one 8 MiB word, which is its program, and a program
	// over store.MaxProgramBytes is left out of the declaration and said on
	// the coverage record (REC-52). That run is recorded once and is not
	// verified, for that reason and no other. post marks the row that runs
	// the post entry on a gate run's output (card GATEOUT-WIRE1).
	type row struct {
		name     string
		payload  func(*testing.T) []byte
		withheld bool
		post     bool
	}
	var rows []row
	for _, l := range pathological.Lines() {
		rows = append(rows, row{"8 MiB of " + l.Name, func(t *testing.T) []byte {
			return res29Payload(t, 8<<20, l.Prefix, l.Unit, l.Suffix)
		}, l.Name == "git@ scheme runs", false})
	}
	nested := `cd "` + strings.Repeat(`$(echo "`, 17) + "x" + strings.Repeat(`")`, 17) + `" && ls`
	rows = append(rows,
		row{"a 1 MB cd a&&", func(t *testing.T) []byte { return res29Payload(t, 1_000_000, "", "cd a&&", "") }, false, false},
		row{"nesting one past maxNest", func(t *testing.T) []byte { return res29Payload(t, len(nested)+1024, nested, " ", "") }, false, false},
		row{"a gate run's stdout at the intake bound, to the post", res29PostPayload, false, true},
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
			entry := "hook"
			if tc.post {
				entry = "post"
			}
			cmd.Env = append(os.Environ(), res29Child+"="+entry+" "+install.Marker+" "+installID)
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
			// Wall time, like peak RSS below, is not a bound an instrumented
			// child can be held to: the race detector's instrumentation
			// inflates CPU time the same way its shadow memory inflates
			// RSS. Measured 2026-10-14, darwin/arm64, go1.27.2, this
			// machine under load: the backslash-newlines row took 290 ms
			// plain and 4.25-7.49 s under -race across repeated runs (an
			// observed 15-26x), while every row's non-race time stayed
			// under 400 ms. That is detector overhead on work the token
			// and masker bounds (RES-9) already cap, not this fold doing
			// more of it, which the race-aware peak-RSS case two lines
			// down establishes is the right way to read an instrumented
			// child's numbers here.
			switch {
			case raceEnabled:
				t.Logf("race detector on: hook took %v, the %v hard stop is not applied to an instrumented child", took, deadline)
			case took > deadline:
				t.Errorf("hook took %v, past its %v hard stop", took, deadline)
			}
			switch {
			case reaper == nil:
				t.Log("this platform reports no peak RSS for a child; the bound is not applied")
			case raceEnabled:
				t.Logf("race detector on: %.2f MiB, the %.0f MiB bound is not applied to an instrumented child", rss, rssMiB)
			case rss > rssMiB:
				t.Errorf("hook peaked at %.2f MiB, over %.0f MiB", rss, rssMiB)
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
			if tc.post {
				// The post decodes at most gateout's 1 MiB of the output, so
				// the run is unread, too_large, and the execution still lands.
				if len(run.Executions) != 1 {
					t.Fatalf("the post recorded %d executions, want 1", len(run.Executions))
				}
				g := run.Executions[0].GateOutput
				if g == nil || g.Kind != "unread" || g.Reason == nil || *g.Reason != "too_large" {
					t.Errorf("gate_output %+v, want unread, too_large", g)
				}
				return
			}
			want := [5]int{1, 1, 0, 1, 0}
			if tc.withheld {
				want[3] = 0
				var reasons []string
				for _, c := range run.Coverage {
					if c.Reason != nil {
						reasons = append(reasons, *c.Reason)
					}
				}
				if len(reasons) != 1 || reasons[0] != store.ReasonRecordOversize {
					t.Errorf("coverage reasons %q; want only %s", reasons, store.ReasonRecordOversize)
				}
			}
			if got := benchCounts(run); got != want {
				t.Errorf("store holds %v declarations, terminals, executions, verified coverage and skipped lines; want %v, the call recorded once", got, want)
			}
		})
	}
}
