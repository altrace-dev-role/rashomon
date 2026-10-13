// Peak RSS is read from the child's Unix rusage (Maxrss), which Windows'
// syscall.Rusage does not have; GOOS=windows go vet ./... failed here.

//go:build !windows

package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/hookio"
	"github.com/altrace-dev-role/rashomon/test/perf/peakrss"
)

const res7Child = "RASHOMON_RES7_CHILD"

// stopPayload is a Stop payload whose final message is n bytes of text.
func stopPayload(n int) []byte {
	var b bytes.Buffer
	b.WriteString(`{"session_id":"s-res7","hook_event_name":"Stop","last_assistant_message":"`)
	line := "All 412 tests pass; the build is green and nothing was skipped.\\n"
	for b.Len() < n {
		b.WriteString(line)
	}
	b.WriteString(`"}`)
	return b.Bytes()
}

// recapMaxRSS runs recap in a child process on payload, through reaper, and
// returns its peak RSS in MiB.
func recapMaxRSS(t *testing.T, reaper *peakrss.Reaper, payload []byte) float64 {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestRES7_HugeFinalMessageIsConstant$")
	cmd.Env = append(os.Environ(), res7Child+"=1", "RASHOMON_HOME="+t.TempDir())
	cmd.Stdin = bytes.NewReader(payload)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	rss, err := reaper.Run(cmd)
	if err != nil {
		t.Fatalf("recap child: %v\n%s", err, out.String())
	}
	return rss
}

// TestRES7_HugeFinalMessageIsConstant: recap streams a Stop payload, so a
// 7 MB final message costs what a 2 KiB one does plus the capture buffers,
// and peak RSS stays within 12 MiB. Reading it whole cost 56 MiB. Break: read
// the Stop payload through hookio.Intake, and the message is held whole.
func TestRES7_HugeFinalMessageIsConstant(t *testing.T) {
	if os.Getenv(res7Child) == "1" {
		run([]string{"recap"}, os.Stdin, io.Discard, io.Discard)
		os.Exit(0)
	}
	if testing.Short() {
		t.Skip("runs recap in child processes")
	}
	// The reaper keeps this process's own peak out of the child's reading:
	// on Linux the child's ru_maxrss is otherwise at least this test's peak,
	// which holds the 7 MB payload (test/perf/peakrss). A platform with no
	// ru_maxrss (windows) skips by name.
	reaper, err := peakrss.NewReaper(t.TempDir())
	if errors.Is(err, peakrss.ErrUnavailable) {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	small := recapMaxRSS(t, reaper, stopPayload(2<<10))
	big := recapMaxRSS(t, reaper, stopPayload(7_000_000))
	t.Logf("recap peak RSS: %.2f MiB at a 2 KiB message, %.2f MiB at 7 MB", small, big)
	// An instrumented child's floor is over the 12 MiB bound before recap
	// does anything (measured 2026-10-13 through the reaper, linux/arm64,
	// go1.25.8 -race: 20.0 MiB at 2 KiB, 22.9 MiB at 7 MB; 4.4 and 5.5 MiB
	// without -race), so under -race only the growth bound applies.
	//
	// The growth bound itself is wider under -race than without, because the
	// race detector's own bookkeeping scales with how much the run touches,
	// not just with recap's steady-state footprint. Non-race growth has no
	// headroom problem (the 4.4/5.5 MiB floor figures above put it under 1.5
	// MiB) and keeps the original 4 MiB bound. Under -race, growth measured
	// here with go1.25.8: linux/amd64 (docker, --platform linux/amd64 on an
	// arm64 host) 2.74-3.00 MiB across 11 trials, clean and under added CPU
	// contention; linux/arm64 2.9 MiB (2026-10-13 note above); darwin/arm64
	// 3.06 MiB; darwin/amd64 under Rosetta 3.15 MiB. CI's own linux/amd64
	// runner -- the environment that actually failed, a shared GitHub-hosted
	// box -- produced 4.32, 4.04 and 4.15 MiB over three runs on 2026-10-14
	// (pr62/pr64/pr66 Test.log), higher than every figure measured directly
	// above. 6 MiB clears that observed 4.32 MiB high-water mark by about
	// 39% for the runner noise this bound exists to absorb, while staying
	// two orders of magnitude under the 86.22 MiB measured here for a recap
	// that holds the whole payload (readFinalMessage's countingReader wrapped
	// in an io.TeeReader into a buffer kept live across the read -- the
	// defect this test exists to catch), so that defect still fails it by a
	// wide margin.
	growthBound := 4.0
	if raceEnabled {
		growthBound = 6.0
	}
	switch {
	case raceEnabled:
		t.Logf("race detector on: the 12 MiB bound is not applied to an instrumented child; the growth bound is %.0f MiB", growthBound)
	case big > 12:
		t.Errorf("recap at a 7 MB final message peaked at %.2f MiB, over 12 MiB", big)
	}
	if big-small > growthBound {
		t.Errorf("a 7 MB message cost %.2f MiB more than a 2 KiB one: the payload is held", big-small)
	}
}

func TestReadFinalMessageKeepsTodaysAccountWindow(t *testing.T) {
	msg := strings.Repeat("é", 70_000) // 70,000 runes, 140,000 bytes: past 64 KiB, inside 64 Ki runes
	fm, ok := readFinalMessage(strings.NewReader(`{"session_id":"s","last_assistant_message":"`+msg+`"}`), true)
	if !ok || fm.SessionID != "s" || fm.Message != msg || fm.Tail != "" {
		t.Fatalf("ok=%v session %q head %d bytes tail %d bytes", ok, fm.SessionID, len(fm.Message), len(fm.Tail))
	}
	long := strings.Repeat("a", messageHeadBytes) + strings.Repeat("z", 100)
	fm, ok = readFinalMessage(strings.NewReader(`{"last_assistant_message":"`+long+`"}`), false)
	if !ok || len(fm.Message) != messageHeadBytes || !strings.HasSuffix(fm.Tail, strings.Repeat("z", 100)) || len(fm.Tail) != messageTailBytes {
		t.Fatalf("long message: head %d tail %d", len(fm.Message), len(fm.Tail))
	}
	for _, bad := range []string{`{"session_id":5,"last_assistant_message":"x"}`, `{"last_assistant_message":["x"]}`, `["x"]`, `{"last_assistant_message":"x"} {}`, ``} {
		if _, ok := readFinalMessage(strings.NewReader(bad), true); ok {
			t.Errorf("%q read as a payload", bad)
		}
	}
}

// TestReadFinalMessageCapIsExact: the cap readFinalMessage applies to the
// whole payload (intake.go's countingReader, cr.n) is separate from
// hookio.Read's own -- recap streams through hookio.IntakeStream, which
// never calls Read -- so it needs its own boundary proof: exactly
// hookio.MaxPayloadBytes is read, one byte more is refused.
func TestReadFinalMessageCapIsExact(t *testing.T) {
	build := func(n int) string {
		const prefix = `{"session_id":"s","last_assistant_message":"`
		const suffix = `"}`
		return prefix + strings.Repeat("a", n-len(prefix)-len(suffix)) + suffix
	}

	at := build(hookio.MaxPayloadBytes)
	if len(at) != hookio.MaxPayloadBytes {
		t.Fatalf("payload %d bytes, want exactly the %d-byte cap", len(at), hookio.MaxPayloadBytes)
	}
	if _, ok := readFinalMessage(strings.NewReader(at), true); !ok {
		t.Fatalf("exactly %d bytes refused, want accepted", hookio.MaxPayloadBytes)
	}

	over := build(hookio.MaxPayloadBytes + 1)
	if _, ok := readFinalMessage(strings.NewReader(over), true); ok {
		t.Fatalf("%d bytes read as a payload, want refused one byte over the cap", len(over))
	}
}
